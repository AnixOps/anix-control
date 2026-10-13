package backup

import (
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// now is Tuesday 2026-10-13 12:00 UTC, in ISO week 2026-W42.
var retentionNow = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)

func day(month time.Month, d int) time.Time {
	return time.Date(2026, month, d, 3, 30, 0, 0, time.UTC)
}

func entry(t string, at time.Time) Entry { return Entry{Name: FileName(at, t), Type: t, CreatedAt: at} }

func verified(e Entry) Entry {
	e.Verified = true
	return e
}

func TestPlan(t *testing.T) {
	cases := []struct {
		name    string
		entries []Entry
		// removed are the names Plan drops; every other is kept.
		removed []string
		reasons map[string]string
	}{
		{
			name:    "nothing",
			entries: nil,
		},
		{
			// Oct 3..13: the newest 7 are Oct 7..13. W41 (Oct 5-11)'s
			// newest is Oct 11 (already kept), W40 (Sep 28-Oct 4)'s is
			// Oct 4. Oct 3, 5 and 6 go.
			name: "eleven days of daily backups",
			entries: func() []Entry {
				var list []Entry
				for d := 3; d <= 13; d++ {
					list = append(list, entry(TypeDaily, day(time.October, d)))
				}
				return list
			}(),
			removed: []string{FileName(day(time.October, 3), TypeDaily), FileName(day(time.October, 5), TypeDaily), FileName(day(time.October, 6), TypeDaily)},
			reasons: map[string]string{
				FileName(day(time.October, 13), TypeDaily): "one of the newest 7 daily backups",
				FileName(day(time.October, 4), TypeDaily):  "the newest daily backup of week 2026-W40",
			},
		},
		{
			// The newest 7 cover W41 and W42; the weeks before that have
			// one daily each, with a gap at W40: W41 (Oct 11), W39, W38
			// and W37 are the four weeks kept; W36, W35, W34 go.
			name: "four weekly backups beyond the dailies",
			entries: func() []Entry {
				var list []Entry
				for d := 7; d <= 13; d++ {
					list = append(list, entry(TypeDaily, day(time.October, d)))
				}
				for _, at := range []time.Time{day(time.September, 27), day(time.September, 20), day(time.September, 13), day(time.September, 6), day(time.August, 30), day(time.August, 23)} {
					list = append(list, entry(TypeDaily, at))
				}
				return list
			}(),
			removed: []string{FileName(day(time.September, 6), TypeDaily), FileName(day(time.August, 30), TypeDaily), FileName(day(time.August, 23), TypeDaily)},
			reasons: map[string]string{
				FileName(day(time.September, 27), TypeDaily): "the newest daily backup of week 2026-W39",
				FileName(day(time.September, 13), TypeDaily): "the newest daily backup of week 2026-W37",
			},
		},
		{
			name: "the current week is not a weekly one",
			entries: []Entry{
				entry(TypeDaily, day(time.October, 12)), entry(TypeDaily, day(time.October, 13)),
			},
		},
		{
			name: "pre-migration backups of the last 30 days",
			entries: []Entry{
				entry(TypePreMigration, retentionNow.Add(-time.Hour)),
				entry(TypePreMigration, retentionNow.Add(-29*24*time.Hour)),
				entry(TypePreMigration, retentionNow.Add(-30*24*time.Hour)),
				entry(TypePreMigration, retentionNow.Add(-31*24*time.Hour)),
				entry(TypePreMigration, retentionNow.Add(-90*24*time.Hour)),
			},
			removed: []string{FileName(retentionNow.Add(-31*24*time.Hour), TypePreMigration), FileName(retentionNow.Add(-90*24*time.Hour), TypePreMigration)},
		},
		{
			name: "the newest three manual and the newest verify backup",
			entries: []Entry{
				entry(TypeManual, day(time.October, 1)), entry(TypeManual, day(time.October, 2)), entry(TypeManual, day(time.October, 3)),
				entry(TypeManual, day(time.October, 4)), entry(TypeManual, day(time.October, 5)),
				entry(TypeVerify, day(time.October, 6)), entry(TypeVerify, day(time.October, 7)),
			},
			removed: []string{FileName(day(time.October, 1), TypeManual), FileName(day(time.October, 2), TypeManual), FileName(day(time.October, 6), TypeVerify)},
		},
		{
			// Only an old pre-migration bundle passed its verification:
			// it stays, whatever its age.
			name: "the newest verified bundle is never dropped",
			entries: []Entry{
				verified(entry(TypePreMigration, retentionNow.Add(-60*24*time.Hour))),
				entry(TypePreMigration, retentionNow.Add(-70*24*time.Hour)),
				verified(entry(TypePreMigration, retentionNow.Add(-80*24*time.Hour))),
			},
			removed: []string{FileName(retentionNow.Add(-70*24*time.Hour), TypePreMigration), FileName(retentionNow.Add(-80*24*time.Hour), TypePreMigration)},
			reasons: map[string]string{FileName(retentionNow.Add(-60*24*time.Hour), TypePreMigration): "the newest verified backup"},
		},
		{
			name:    "types are kept apart",
			entries: []Entry{entry(TypeManual, day(time.January, 2)), entry(TypePreMigration, day(time.October, 12))},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decisions := Plan(tc.entries, retentionNow)
			assert.Len(t, decisions, len(tc.entries))
			var removed []string
			for i, decision := range decisions {
				assert.Equal(t, tc.entries[i], decision.Entry, "decisions keep the order")
				if !decision.Keep {
					removed = append(removed, decision.Name)
					assert.Equal(t, "outside the retention", decision.Reason)
				} else {
					assert.NotEmpty(t, decision.Reason)
				}
				if want, ok := tc.reasons[decision.Name]; ok {
					assert.Equal(t, want, decision.Reason, decision.Name)
				}
			}
			sort.Strings(removed)
			want := append([]string(nil), tc.removed...)
			sort.Strings(want)
			assert.Equal(t, want, removed)
		})
	}
}
