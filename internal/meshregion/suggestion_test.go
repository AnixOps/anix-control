package meshregion

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const goodSuggestion = `{"region":"eu","current_median_ms":212,"suggested_median_ms":48,"since":"2026-11-24T08:00:00Z"}`

func TestRegionSuggestionIsKeptAndShown(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	cn := region(t, db, "cn", "10.150.0.0/16")
	_, err := AddNode(ctx, db, cn.ID, proxy(1), false, now)
	require.NoError(t, err)

	// An Agent node's status is kept as it is, and its suggestion shown.
	report := `{"networks":[],"region_probe":{"measured_at":null,"regions":[],"suggestion":` + goodSuggestion + `}}`
	require.NoError(t, RecordStatus(ctx, db, proxy(1), []byte(report), now, now))
	regions, err := List(ctx, db)
	require.NoError(t, err)
	member := regions[0].Members[0]
	require.JSONEq(t, report, string(member.Status))
	require.Equal(t, &RegionSuggestion{Region: "eu", CurrentMedianMS: 212, SuggestedMedianMS: 48, Since: time.Date(2026, 11, 24, 8, 0, 0, 0, time.UTC)}, member.RegionSuggestion)
	require.NoError(t, RecordStatus(ctx, db, proxy(1), []byte(`{"networks":[],"region_probe":{"suggestion":null}}`), now.Add(time.Hour), now.Add(time.Hour)))
	regions, err = List(ctx, db)
	require.NoError(t, err)
	require.Nil(t, regions[0].Members[0].RegionSuggestion)

	// A device's report: unknown top-level fields stay, a bad suggestion is
	// refused.
	issued, err := CreateEnrollment(ctx, db, EnrollmentInput{Hostname: "laptop", RegionID: cn.ID}, nil, now)
	require.NoError(t, err)
	enrolled, err := Enroll(ctx, db, EnrollRequest{Features: meshFeatures, Token: issued.Token, DeviceKey: laptopKey}, now)
	require.NoError(t, err)
	device, err := Authenticate(ctx, db, enrolled.DeviceToken, now)
	require.NoError(t, err)
	send := func(body string) error {
		return RecordDeviceStatus(ctx, db, device, enrolled.DeviceToken, []byte(body), now)
	}
	for _, bad := range []string{
		`{"networks":[],"region_probe":"x"}`,
		`{"networks":[],"region_probe":{"suggestion":"eu"}}`,
		`{"networks":[],"region_probe":{"suggestion":{"region":"EU","current_median_ms":1,"suggested_median_ms":1,"since":"2026-11-24T08:00:00Z"}}}`,
		`{"networks":[],"region_probe":{"suggestion":{"region":"eu","current_median_ms":-1,"suggested_median_ms":1,"since":"2026-11-24T08:00:00Z"}}}`,
		`{"networks":[],"region_probe":{"suggestion":{"region":"eu","current_median_ms":1.5,"suggested_median_ms":1,"since":"2026-11-24T08:00:00Z"}}}`,
		`{"networks":[],"region_probe":{"suggestion":{"region":"eu","current_median_ms":1,"suggested_median_ms":"1","since":"2026-11-24T08:00:00Z"}}}`,
		`{"networks":[],"region_probe":{"suggestion":{"region":"eu","current_median_ms":1,"suggested_median_ms":1,"since":"yesterday"}}}`,
		`{"networks":[],"region_probe":{"suggestion":{"region":"eu","current_median_ms":1,"suggested_median_ms":1}}}`,
	} {
		require.ErrorIs(t, send(bad), ErrInvalid, bad)
	}
	require.NoError(t, send(`{"networks":[],"future_field":{"a":1},"region_probe":null}`))
	require.NoError(t, send(`{"networks":[],"region_probe":{"regions":[{"region":"eu","rtt_ms":18.4}],"suggestion":`+goodSuggestion+`}}`))
	views, err := Devices(ctx, db, 0, now)
	require.NoError(t, err)
	require.Len(t, views, 1)
	require.Equal(t, "eu", views[0].RegionSuggestion.Region)
	require.EqualValues(t, 48, views[0].RegionSuggestion.SuggestedMedianMS)
	require.NoError(t, send(`{"networks":[]}`))
	views, err = Devices(ctx, db, 0, now)
	require.NoError(t, err)
	require.Nil(t, views[0].RegionSuggestion)
}
