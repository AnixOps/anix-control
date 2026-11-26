package meshregion

import (
	"bytes"
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func probeKeyOf(b byte) string { return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{b}, 32)) }

// reportProbe stores a status of node ref whose region_probe has the given
// responder key.
func reportProbe(t *testing.T, db *gorm.DB, ref, key string) {
	t.Helper()
	payload := `{"networks":[],"region_probe":{"responder":{"public_key":"` + key + `"}}}`
	require.NoError(t, db.Create(&model.MeshStatus{NodeRef: ref, Payload: payload, ObservedAt: now, ReceivedAt: now}).Error)
}

func setAddress(t *testing.T, db *gorm.DB, id uint, address string) {
	t.Helper()
	require.NoError(t, db.Model(&model.Node{}).Where("id = ?", id).Update("last_public_address", address).Error)
}

func TestDirectoryListsBorderNodesWithProbeKeys(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	cn := region(t, db, "cn", "10.150.0.0/16")
	eu := region(t, db, "eu", "10.152.0.0/16")
	region(t, db, "na", "10.154.0.0/16")
	off := region(t, db, "ap", "10.156.0.0/16")
	for ref, home := range map[uint]uint{1: cn.ID, 2: eu.ID, 3: cn.ID, 4: cn.ID} {
		_, err := AddNode(ctx, db, home, proxy(uint32(ref)), ref != 3, now)
		require.NoError(t, err)
	}
	// openDB has nodes 1 to 4.
	require.NoError(t, db.Create(&model.Node{ID: 5, Name: "e", Host: "e.example.com"}).Error)
	_, err := AddNode(ctx, db, off.ID, proxy(5), true, now)
	require.NoError(t, err)
	_, err = UpdateRegion(ctx, db, off.ID, RegionInput{Code: "ap", Name: "ap", CIDR: off.CIDR, Enabled: false}, now)
	require.NoError(t, err)
	setAddress(t, db, 1, "203.0.113.7")
	setAddress(t, db, 2, "2001:db8::2")
	setAddress(t, db, 3, "198.51.100.3")
	setAddress(t, db, 4, "198.51.100.4")
	setAddress(t, db, 5, "198.51.100.5")
	reportProbe(t, db, "proxy-1", probeKeyOf(1))
	reportProbe(t, db, "proxy-2", probeKeyOf(2))
	reportProbe(t, db, "proxy-3", probeKeyOf(3)) // not a border node
	reportProbe(t, db, "proxy-4", "not-a-key")   // an invalid key
	reportProbe(t, db, "proxy-5", probeKeyOf(5)) // in a disabled region
	require.NoError(t, db.Create(&model.MeshStatus{NodeRef: "proxy-9", Payload: `{"networks":[]}`, ObservedAt: now, ReceivedAt: now}).Error)

	directory, err := BuildDirectory(db, cn.ID)
	require.NoError(t, err)
	require.Equal(t, 1, directory.Version)
	require.NotNil(t, directory.Preferred)
	require.Equal(t, "cn", *directory.Preferred)
	require.Equal(t, []DirectoryRegion{
		{Code: "cn", Subnet: "10.150.0.0/16", Borders: []DirectoryBorder{{Name: "a", Probe: "203.0.113.7:11013", Key: probeKeyOf(1)}}},
		{Code: "eu", Subnet: "10.152.0.0/16", Borders: []DirectoryBorder{{Name: "b", Probe: "[2001:db8::2]:11013", Key: probeKeyOf(2)}}},
	}, directory.Regions, "no region without a border with a key (na), none disabled (ap)")

	// A border without an address literal (a host name) is not listed, and
	// the responder port is a setting.
	require.NoError(t, db.Model(&model.Node{}).Where("id = ?", 1).Update("last_public_address", "").Error)
	_, err = SaveSettings(ctx, db, SettingsInput{NameSuffix: DefaultNameSuffix, HostGroupSupernet: DefaultHostGroupSupernet, HostGroupPrefix: DefaultHostGroupPrefix, ResponderPort: 12000}, now)
	require.NoError(t, err)
	directory, err = BuildDirectory(db, 0)
	require.NoError(t, err)
	require.Nil(t, directory.Preferred)
	require.Len(t, directory.Regions, 1)
	require.Equal(t, "[2001:db8::2]:12000", directory.Regions[0].Borders[0].Probe)
	for _, port := range []int{-1, 65536, Port} {
		_, err = SaveSettings(ctx, db, SettingsInput{NameSuffix: DefaultNameSuffix, HostGroupSupernet: DefaultHostGroupSupernet, HostGroupPrefix: DefaultHostGroupPrefix, ResponderPort: port}, now)
		require.ErrorIs(t, err, ErrInvalid, "port %d", port)
	}
}

func TestProbeKeysAndAddresses(t *testing.T) {
	for raw, ok := range map[string]bool{
		probeKeyOf(1):            true,
		probeKeyOf(0):            false,
		"":                       false,
		probeKeyOf(1)[:43]:       false,
		probeKeyOf(1)[:43] + "!": false,
		base64.URLEncoding.EncodeToString(bytes.Repeat([]byte{0xfb}, 32)): false,
	} {
		_, got := probeKey(raw)
		require.Equal(t, ok, got, raw)
	}
	for raw, want := range map[string]string{
		"203.0.113.7": "203.0.113.7", "203.0.113.7:443": "203.0.113.7", "[2001:db8::1]": "2001:db8::1", "2001:db8::1": "2001:db8::1",
		"[2001:db8::1]:443": "2001:db8::1", "10.1.2.3": "10.1.2.3", "::ffff:198.51.100.1": "198.51.100.1",
		"": "", "example.com": "", "127.0.0.1": "", "169.254.1.1": "", "224.0.0.1": "", "0.0.0.0": "", "0.1.2.3": "", "255.255.255.255": "", "::1": "", "fe80::1": "", "fe80::1%eth0": "",
	} {
		address, ok := probeLiteral(raw)
		if want == "" {
			require.False(t, ok, raw)
			continue
		}
		require.True(t, ok, raw)
		require.Equal(t, want, address.String(), raw)
	}
	taken := map[string]bool{}
	require.Equal(t, "fra-1", borderName("Fra 1", "proxy-1", taken))
	require.Equal(t, "fra-1-proxy-2", borderName("fra-1", "proxy-2", taken), "unique in the region")
	require.Equal(t, "node-proxy-3", borderName("!!!", "proxy-3", taken), "a name with no label")
}

func TestDirectoryAuthentication(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	region(t, db, "cn", "10.150.0.0/16")
	eu := region(t, db, "eu", "10.152.0.0/16")
	_, err := AddNode(ctx, db, eu.ID, proxy(1), true, now)
	require.NoError(t, err)
	setAddress(t, db, 1, "203.0.113.7")
	reportProbe(t, db, "proxy-1", probeKeyOf(1))
	_, err = SaveSettings(ctx, db, SettingsInput{NameSuffix: DefaultNameSuffix, HostGroupSupernet: DefaultHostGroupSupernet, HostGroupPrefix: DefaultHostGroupPrefix, PreferredRegion: "eu"}, now)
	require.NoError(t, err)

	issued, err := CreateEnrollment(ctx, db, EnrollmentInput{Hostname: "laptop", PreferredRegion: "cn"}, nil, now)
	require.NoError(t, err)
	// An unused credential reads the directory and is not used up.
	directory, err := DeviceDirectory(ctx, db, issued.Token, now)
	require.NoError(t, err)
	require.Equal(t, "cn", *directory.Preferred, "the credential's preferred region")
	require.Len(t, directory.Regions, 1)
	var row model.MeshEnrollment
	require.NoError(t, db.First(&row, issued.ID).Error)
	require.Nil(t, row.UsedAt)
	require.Nil(t, row.DeviceID)
	enrolled, err := Enroll(ctx, db, EnrollRequest{Features: meshFeatures, Token: issued.Token, DeviceKey: laptopKey, Latencies: map[string]float64{"cn": 5}}, now)
	require.NoError(t, err, "the credential was still unused")

	// Used: within the retry window it reads, after it does not.
	_, err = DeviceDirectory(ctx, db, issued.Token, now.Add(EnrollRetryWindow))
	require.NoError(t, err)
	_, err = DeviceDirectory(ctx, db, issued.Token, now.Add(EnrollRetryWindow+time.Second))
	require.ErrorIs(t, err, ErrUnauthorized)
	require.NoError(t, db.First(&row, issued.ID).Error)
	require.True(t, row.UsedAt.Equal(now), "reading changes nothing")

	// A device token reads with the default preferred region.
	directory, err = DeviceDirectory(ctx, db, enrolled.DeviceToken, now.Add(time.Hour))
	require.NoError(t, err)
	require.Equal(t, "eu", *directory.Preferred)
	require.NoError(t, RevokeDevice(ctx, db, enrolled.DeviceID))
	_, err = DeviceDirectory(ctx, db, enrolled.DeviceToken, now)
	require.ErrorIs(t, err, ErrUnauthorized, "a revoked token")

	// Expired, withdrawn and unknown credentials, other tokens.
	expiring, err := CreateEnrollment(ctx, db, EnrollmentInput{Hostname: "tablet", Hours: 1}, nil, now)
	require.NoError(t, err)
	_, err = DeviceDirectory(ctx, db, expiring.Token, now.Add(time.Hour))
	require.ErrorIs(t, err, ErrUnauthorized, "expired")
	require.NoError(t, DeleteEnrollment(ctx, db, expiring.ID))
	_, err = DeviceDirectory(ctx, db, expiring.Token, now)
	require.ErrorIs(t, err, ErrUnauthorized, "withdrawn")
	for _, token := range []string{"", "mse_unknown", "msd_unknown", "Bearer x", "plain"} {
		_, err = DeviceDirectory(ctx, db, token, now)
		require.ErrorIs(t, err, ErrUnauthorized, token)
	}
}
