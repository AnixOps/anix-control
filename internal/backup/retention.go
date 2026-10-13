package backup

import (
	"fmt"
	"sort"
	"time"
)

// The retention (docs/reference/backups.md, "Retention"). The numbers are
// built in.
const (
	// KeepDaily daily bundles, the newest, are kept.
	KeepDaily = 7
	// KeepWeekly: of each of this many ISO weeks before the current one
	// (the most recent weeks that have a daily bundle), the newest daily
	// bundle is kept.
	KeepWeekly = 4
	// KeepPreMigration: every pre-migration bundle of this period is kept.
	KeepPreMigration = 30 * 24 * time.Hour
	// KeepManual manual bundles, the newest, are kept.
	KeepManual = 3
	// KeepVerify verify bundles, the newest, are kept.
	KeepVerify = 1
)

// Entry is a bundle as the retention sees it.
type Entry struct {
	Name      string
	Type      string
	CreatedAt time.Time
	// Verified is true when the bundle's latest verification passed.
	Verified bool
}

// Decision is the retention's answer for a bundle.
type Decision struct {
	Entry
	Keep bool
	// Reason says why it is kept, or why not.
	Reason string
}

// Plan decides which bundles to keep at now. It never drops the newest
// verified bundle, whatever its type and age. Bundles keep their order.
func Plan(entries []Entry, now time.Time) []Decision {
	decisions := make([]Decision, len(entries))
	for i, entry := range entries {
		decisions[i] = Decision{Entry: entry}
	}
	byType := map[string][]int{}
	for i, entry := range entries {
		byType[entry.Type] = append(byType[entry.Type], i)
	}
	for _, indexes := range byType {
		sort.SliceStable(indexes, func(a, b int) bool {
			return entries[indexes[a]].CreatedAt.After(entries[indexes[b]].CreatedAt)
		})
	}
	keep := func(i int, reason string) {
		if !decisions[i].Keep {
			decisions[i].Keep, decisions[i].Reason = true, reason
		}
	}
	keepNewest := func(t string, count int, reason string) {
		for rank, i := range byType[t] {
			if rank < count {
				keep(i, reason)
			}
		}
	}

	keepNewest(TypeDaily, KeepDaily, fmt.Sprintf("one of the newest %d daily backups", KeepDaily))
	// Weekly: the newest daily of each ISO week before now's, for the
	// KeepWeekly most recent such weeks that have one.
	currentYear, currentWeek := now.UTC().ISOWeek()
	weeks := 0
	lastWeek := ""
	for _, i := range byType[TypeDaily] {
		year, week := entries[i].CreatedAt.UTC().ISOWeek()
		if year > currentYear || (year == currentYear && week >= currentWeek) {
			continue
		}
		label := fmt.Sprintf("%d-W%02d", year, week)
		if label == lastWeek {
			continue
		}
		lastWeek = label
		if weeks >= KeepWeekly {
			break
		}
		weeks++
		keep(i, "the newest daily backup of week "+label)
	}
	for _, i := range byType[TypePreMigration] {
		if now.Sub(entries[i].CreatedAt) <= KeepPreMigration {
			keep(i, "a pre-migration backup of the last 30 days")
		}
	}
	keepNewest(TypeManual, KeepManual, fmt.Sprintf("one of the newest %d manual backups", KeepManual))
	keepNewest(TypeVerify, KeepVerify, "the newest verify backup")

	newestVerified := -1
	for i, entry := range entries {
		if entry.Verified && (newestVerified < 0 || entry.CreatedAt.After(entries[newestVerified].CreatedAt)) {
			newestVerified = i
		}
	}
	if newestVerified >= 0 {
		keep(newestVerified, "the newest verified backup")
	}
	for i := range decisions {
		if !decisions[i].Keep {
			decisions[i].Reason = "outside the retention"
		}
	}
	return decisions
}
