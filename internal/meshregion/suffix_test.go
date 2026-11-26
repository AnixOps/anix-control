package meshregion

import (
	"context"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
)

// The intranet domain suffix follows spec section 15.9.2 item 1: under
// internal with at least two labels, compared without one final dot and
// in lowercase.
func TestNameSuffixRule(t *testing.T) {
	longest := strings.Repeat(strings.Repeat("a", 63)+".", 2) + strings.Repeat("a", 35) + ".internal"
	require.Len(t, longest, maxSuffix)
	for _, good := range []string{"mesh.internal", "Mesh.Internal.", "MESH.INTERNAL", " lab.mesh.internal ", "a-1.internal", "xn--fiqs8s.internal", "lab.xn--bcher-kva.internal", strings.Repeat("a", 63) + ".internal", longest} {
		require.NoError(t, checkSuffix(normalizeSuffix(good)), good)
	}
	for _, bad := range []string{
		"", ".", "internal", "internal.", "local", "com", "mesh",
		"arpa", "home.arpa", "mesh.home.arpa", "local.internal.arpa",
		"mesh.local", "local.local", "corp.example", "mesh.internal.com", "internal.mesh",
		"xinternal", "mesh.xinternal", "mesh.internalx", "mesh-internal",
		// Only ASCII letters are lowercased: no Unicode mapping makes these
		// ASCII (U+0130 to i, the Kelvin sign to k).
		"MESH.\u0130NTERNAL", "\u212Aube.internal", "中国.internal", "mesh.ınternal",
		".mesh.internal", "mesh..internal", "mesh.internal..", "-mesh.internal", "mesh-.internal", "mesh_lab.internal", "mesh lab.internal",
		strings.Repeat("a", 64) + ".internal", "a" + longest,
	} {
		require.ErrorIs(t, checkSuffix(normalizeSuffix(bad)), ErrInvalid, "%q", bad)
	}
}

// A suffix saved before the rule (home.arpa, say) is never delivered: the
// settings carry a warning, nodes and enrolling devices get the dns
// section disabled with no names and no resolver address, and the name
// list and the device view mark the names as not delivered, until a valid
// suffix is saved.
func TestSavedSuffixBreakingTheRule(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	cn := region(t, db, "cn", "10.150.0.0/24")
	_, err := AddNode(ctx, db, cn.ID, proxy(1), false, now)
	require.NoError(t, err)
	_, err = CreateDevice(ctx, db, cn.ID, DeviceInput{Hostname: "phone"}, now)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.MeshSettings{ID: settingsID, NameSuffix: "mesh.home.arpa", HostGroupSupernet: DefaultHostGroupSupernet, HostGroupPrefix: DefaultHostGroupPrefix, UpdatedAt: now}).Error)

	nodeDNS := func() map[string]any {
		t.Helper()
		doc, err := NodeMember(db, proxy(1))
		require.NoError(t, err)
		return doc["networks"].([]map[string]any)[0]["dns"].(map[string]any)
	}
	features := append([]string{FeatureResolverAddress}, meshFeatures...)
	enroll := func(name string, key byte) map[string]any {
		t.Helper()
		issued, err := CreateEnrollment(ctx, db, EnrollmentInput{Hostname: name, RegionID: cn.ID, Role: RoleFull}, nil, now)
		require.NoError(t, err)
		enrolled, err := Enroll(ctx, db, EnrollRequest{Features: features, Token: issued.Token, DeviceKey: strings.Repeat(string(rune(key)), 32)}, now)
		require.NoError(t, err)
		return enrolled.Config["networks"].([]map[string]any)[0]["dns"].(map[string]any)
	}
	fqdns := func() (map[string]string, bool) {
		t.Helper()
		views, err := Devices(ctx, db, cn.ID, now)
		require.NoError(t, err)
		answer := map[string]string{}
		for _, view := range views {
			answer[view.Hostname] = view.FQDN
		}
		list, err := Names(ctx, db)
		require.NoError(t, err)
		return answer, list.SuffixInvalid
	}

	settings, err := LoadSettings(ctx, db)
	require.NoError(t, err)
	require.Contains(t, settings.Warnings, WarningNameSuffixInvalid)
	disabled := map[string]any{"enabled": false, "names": []map[string]any{}}
	require.Equal(t, disabled, nodeDNS(), "no suffix, no names, no resolver address")
	require.Equal(t, disabled, enroll("router", 'a'), "the enroll response as well")
	names, invalid := fqdns()
	require.True(t, invalid)
	require.Equal(t, map[string]string{"phone": "", "router": ""}, names)

	_, err = SaveSettings(ctx, db, SettingsInput{NameSuffix: "mesh.home.arpa", HostGroupSupernet: DefaultHostGroupSupernet, HostGroupPrefix: DefaultHostGroupPrefix}, now)
	require.ErrorIs(t, err, ErrInvalid, "the old suffix is not saved again")
	settings, err = SaveSettings(ctx, db, SettingsInput{NameSuffix: "mesh.internal", HostGroupSupernet: DefaultHostGroupSupernet, HostGroupPrefix: DefaultHostGroupPrefix}, now)
	require.NoError(t, err)
	require.NotContains(t, settings.Warnings, WarningNameSuffixInvalid)
	dns := nodeDNS()
	require.Equal(t, true, dns["enabled"])
	require.Equal(t, "mesh.internal", dns["suffix"])
	require.Equal(t, "10.150.0.254", dns["resolver_address"])
	dns = enroll("office", 'b')
	require.Equal(t, true, dns["enabled"])
	require.Equal(t, "10.150.0.254", dns["resolver_address"])
	names, invalid = fqdns()
	require.False(t, invalid)
	require.Equal(t, "phone.cn.mesh.internal", names["phone"])
}
