package meshregion

import (
	"context"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
)

func TestChooseRegionScores(t *testing.T) {
	regions := []model.MeshRegion{
		{ID: 1, Code: "cn", Sort: 1},
		{ID: 2, Code: "eu", Sort: 2},
		{ID: 3, Code: "na", Sort: 3},
	}
	for _, test := range []struct {
		name      string
		latencies map[string]float64
		preferred uint
		want      string
	}{
		{"no preference: the lowest latency", map[string]float64{"cn": 40, "eu": 35}, 0, "eu"},
		{"preferred wins when 0.8 times its latency is lower (40 against 35)", map[string]float64{"cn": 40, "eu": 35}, 1, "cn"},
		{"preferred loses when 0.8 times its latency is not lower (45 against 35)", map[string]float64{"cn": 45, "eu": 35}, 1, "eu"},
		{"preferred without a latency is ignored", map[string]float64{"eu": 35, "na": 50}, 1, "eu"},
		{"equal scores: the lower sort", map[string]float64{"cn": 30, "eu": 30, "na": 30}, 0, "cn"},
		{"equal scores after the factor: the lower sort", map[string]float64{"cn": 25, "eu": 20}, 2, "cn"},
		{"a zero latency counts", map[string]float64{"cn": 0, "eu": 1}, 0, "cn"},
		{"a negative latency is ignored", map[string]float64{"cn": -1, "eu": 90}, 0, "eu"},
		{"a code that is no region is ignored", map[string]float64{"xx": 1, "na": 90}, 0, "na"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, ok := chooseRegion(regions, test.latencies, test.preferred)
			require.True(t, ok)
			require.Equal(t, test.want, got.Code)
		})
	}
	_, ok := chooseRegion(regions, map[string]float64{"xx": 1}, 1)
	require.False(t, ok, "no region has a latency")
}

func TestChooseRegionTiesFollowSortThenID(t *testing.T) {
	// Handed over in the wrong order: the rule is explicit, not the
	// order of the list.
	regions := []model.MeshRegion{
		{ID: 5, Code: "d", Sort: 2},
		{ID: 4, Code: "c", Sort: 2},
		{ID: 9, Code: "z", Sort: 1},
		{ID: 1, Code: "a", Sort: 3},
	}
	latencies := map[string]float64{"a": 10, "c": 10, "d": 10, "z": 10}
	got, ok := chooseRegion(regions, latencies, 0)
	require.True(t, ok)
	require.Equal(t, "z", got.Code, "the lowest sort")
	got, _ = chooseRegion(regions[:2], latencies, 0)
	require.Equal(t, "c", got.Code, "the same sort: the lowest id")
}

func TestEnrollmentPrefersARegion(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	cn := region(t, db, "cn", "10.150.0.0/16")
	eu := region(t, db, "eu", "10.152.0.0/16")
	off := region(t, db, "na", "10.154.0.0/16")
	_, err := UpdateRegion(ctx, db, off.ID, RegionInput{Code: "na", Name: "na", CIDR: off.CIDR, Enabled: false, Sort: 0}, now)
	require.NoError(t, err)

	_, err = CreateEnrollment(ctx, db, EnrollmentInput{Hostname: "a", PreferredRegion: "xx"}, nil, now)
	require.ErrorIs(t, err, ErrInvalid, "an unknown preferred region")
	_, err = CreateEnrollment(ctx, db, EnrollmentInput{Hostname: "a", PreferredRegion: "cn", RegionID: eu.ID}, nil, now)
	require.ErrorIs(t, err, ErrInvalid, "a credential with a region has none preferred")

	enroll := func(host, key string, input EnrollmentInput, latencies map[string]float64) string {
		t.Helper()
		input.Hostname = host
		issued, err := CreateEnrollment(ctx, db, input, nil, now)
		require.NoError(t, err)
		enrolled, err := Enroll(ctx, db, EnrollRequest{Features: meshFeatures, Token: issued.Token, DeviceKey: key + "-device-key-padding", Latencies: latencies}, now)
		require.NoError(t, err)
		return enrolled.Config["networks"].([]map[string]any)[0]["id"].(string)
	}
	// The credential's preferred region wins at 40 against 35, not at 45;
	// a disabled region never counts, however low its latency.
	require.Equal(t, "cn", enroll("one", "one", EnrollmentInput{PreferredRegion: "cn"}, map[string]float64{"cn": 40, "eu": 35, "na": 1}))
	require.Equal(t, "eu", enroll("two", "two", EnrollmentInput{PreferredRegion: "cn"}, map[string]float64{"cn": 45, "eu": 35}))
	require.Equal(t, "eu", enroll("three", "three", EnrollmentInput{}, map[string]float64{"cn": 40, "eu": 35}), "none preferred")

	// The settings' default applies without the credential's.
	settings, err := SaveSettings(ctx, db, SettingsInput{NameSuffix: DefaultNameSuffix, HostGroupSupernet: DefaultHostGroupSupernet, HostGroupPrefix: DefaultHostGroupPrefix, PreferredRegion: "cn"}, now)
	require.NoError(t, err)
	require.Equal(t, "cn", settings.PreferredRegion)
	require.Equal(t, cn.ID, settings.PreferredRegionID)
	require.Equal(t, "cn", enroll("four", "four", EnrollmentInput{}, map[string]float64{"cn": 40, "eu": 35}))
	require.Equal(t, "eu", enroll("five", "five", EnrollmentInput{PreferredRegion: "eu"}, map[string]float64{"cn": 30, "eu": 35}), "the credential's overrides the default")
	_, err = SaveSettings(ctx, db, SettingsInput{NameSuffix: DefaultNameSuffix, HostGroupSupernet: DefaultHostGroupSupernet, HostGroupPrefix: DefaultHostGroupPrefix, PreferredRegion: "xx"}, now)
	require.ErrorIs(t, err, ErrInvalid)
	settings, err = SaveSettings(ctx, db, SettingsInput{NameSuffix: DefaultNameSuffix, HostGroupSupernet: DefaultHostGroupSupernet, HostGroupPrefix: DefaultHostGroupPrefix}, now)
	require.NoError(t, err)
	require.Empty(t, settings.PreferredRegion, "saved empty: none")

	views, err := Enrollments(ctx, db)
	require.NoError(t, err)
	codes := map[string]string{}
	for _, view := range views {
		codes[view.Hostname] = view.PreferredRegion
	}
	require.Equal(t, map[string]string{"one": "cn", "two": "cn", "three": "", "four": "", "five": "eu"}, codes)
}
