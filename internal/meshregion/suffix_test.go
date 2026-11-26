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
	for _, good := range []string{"mesh.internal", "Mesh.Internal.", " lab.mesh.internal ", "a-1.internal", strings.Repeat("a", 63) + ".internal", longest} {
		require.NoError(t, checkSuffix(normalizeSuffix(good)), good)
	}
	for _, bad := range []string{
		"", ".", "internal", "internal.", "local", "com", "mesh",
		"arpa", "home.arpa", "mesh.home.arpa", "local.internal.arpa",
		"mesh.local", "local.local", "corp.example", "mesh.internal.com", "internal.mesh",
		".mesh.internal", "mesh..internal", "mesh.internal..", "-mesh.internal", "mesh-.internal", "mesh_lab.internal", "mesh lab.internal",
		strings.Repeat("a", 64) + ".internal", "a" + longest,
	} {
		require.ErrorIs(t, checkSuffix(normalizeSuffix(bad)), ErrInvalid, "%q", bad)
	}
}

// A suffix saved before the rule (home.arpa, say) is never delivered: the
// settings carry a warning and nodes get the dns section disabled with no
// names until a valid suffix is saved.
func TestSavedSuffixBreakingTheRule(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	require.NoError(t, db.Create(&model.MeshSettings{ID: settingsID, NameSuffix: "mesh.home.arpa", HostGroupSupernet: DefaultHostGroupSupernet, HostGroupPrefix: DefaultHostGroupPrefix, UpdatedAt: now}).Error)
	settings, err := LoadSettings(ctx, db)
	require.NoError(t, err)
	require.Contains(t, settings.Warnings, WarningNameSuffixInvalid)
	dns, err := dnsSection(db.WithContext(ctx))
	require.NoError(t, err)
	require.Equal(t, map[string]any{"enabled": false, "names": []map[string]any{}}, dns)

	_, err = SaveSettings(ctx, db, SettingsInput{NameSuffix: "mesh.home.arpa", HostGroupSupernet: DefaultHostGroupSupernet, HostGroupPrefix: DefaultHostGroupPrefix}, now)
	require.ErrorIs(t, err, ErrInvalid, "the old suffix is not saved again")
	settings, err = SaveSettings(ctx, db, SettingsInput{NameSuffix: "mesh.internal", HostGroupSupernet: DefaultHostGroupSupernet, HostGroupPrefix: DefaultHostGroupPrefix}, now)
	require.NoError(t, err)
	require.NotContains(t, settings.Warnings, WarningNameSuffixInvalid)
	dns, err = dnsSection(db.WithContext(ctx))
	require.NoError(t, err)
	require.Equal(t, true, dns["enabled"])
	require.Equal(t, "mesh.internal", dns["suffix"])
}
