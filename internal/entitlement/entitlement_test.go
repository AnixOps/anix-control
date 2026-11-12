package entitlement

import (
	"crypto/ed25519"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// testKEK is a fixed root key (hex of 32 bytes).
var testKEK = strings.Repeat("42", 32)

func openDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "control.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.SystemConfig{}, &model.EntitlementDevice{}))
	return db
}

func set(t *testing.T, db *gorm.DB, key, value string) {
	t.Helper()
	require.NoError(t, db.Where("key = ?", key).Delete(&model.SystemConfig{}).Error)
	require.NoError(t, db.Create(&model.SystemConfig{Key: key, Value: value}).Error)
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newService(t *testing.T) (*Service, *clock) {
	db := openDB(t)
	c := &clock{t: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)}
	return &Service{DB: db, KEK: func() string { return testKEK }, Now: c.now}, c
}

func planUser(id uint, plan uint) *model.User {
	u := &model.User{ID: id, UUID: fmt.Sprintf("9b3e6c1e-6f1d-4c55-9a43-0d5b0c9f4a%02d", id)}
	if plan > 0 {
		u.PlanID = &plan
	}
	return u
}

func edDevice(t *testing.T, seed byte) string {
	t.Helper()
	priv := ed25519.NewKeyFromSeed(append(make([]byte, 31), seed))
	return "ed25519:" + hex.EncodeToString(priv.Public().(ed25519.PublicKey))
}

// decodeCert checks a certificate's layout and signature and answers its
// user_id, device public key and validity.
func decodeCert(t *testing.T, ca string, cert []byte) (userID [8]byte, pk []byte, notBefore, notAfter uint32) {
	t.Helper()
	require.Equal(t, byte(1), cert[0])
	require.Equal(t, AlgEd25519, cert[17])
	n := int(cert[18])
	pk = cert[19 : 19+n]
	copy(userID[:], cert[9:17])
	notBefore = binary.BigEndian.Uint32(cert[19+n:])
	notAfter = binary.BigEndian.Uint32(cert[23+n:])
	require.Len(t, cert, 27+n+64)
	caKey, err := hex.DecodeString(ca)
	require.NoError(t, err)
	msg := append([]byte(certInfo), cert[:27+n]...)
	require.True(t, ed25519.Verify(caKey, msg, cert[27+n:]))
	id := Device{Alg: AlgEd25519, PublicKey: pk}.ID()
	require.Equal(t, id[:], cert[1:9])
	return userID, pk, notBefore, notAfter
}

func TestOffByDefault(t *testing.T) {
	svc, _ := newService(t)
	grant, err := svc.IssueANX(planUser(1, 1), edDevice(t, 1))
	require.NoError(t, err)
	require.Nil(t, grant)
	ca, required, err := svc.ANXNodeLicense()
	require.NoError(t, err)
	require.Empty(t, ca)
	require.False(t, required)
}

func TestIssueToAPlanUser(t *testing.T) {
	svc, c := newService(t)
	set(t, svc.DB, KeyANXEnabled, "true")
	user := planUser(1, 7)
	grant, err := svc.IssueANX(user, edDevice(t, 1))
	require.NoError(t, err)
	require.Equal(t, StatusOK, grant.Status)
	require.False(t, grant.Trial)
	userID, _, nb, na := decodeCert(t, grant.CA, grant.License)
	want, _ := ANXUserID(user.UUID)
	require.Equal(t, want, userID)
	require.EqualValues(t, c.t.Add(-backdate).Unix(), nb)
	require.EqualValues(t, c.t.Add(LicenseValidity).Unix(), na)

	// The node gets the same CA; require_license only when asked for.
	ca, req, err := svc.ANXNodeLicense()
	require.NoError(t, err)
	require.Equal(t, grant.CA, ca)
	require.False(t, req)
	set(t, svc.DB, KeyANXRequire, "1")
	_, req, _ = svc.ANXNodeLicense()
	require.True(t, req)

	// Never past the account's expiry.
	expiry := c.t.Add(48 * time.Hour).Unix()
	user.ExpiredAt = &expiry
	grant, err = svc.IssueANX(user, edDevice(t, 1))
	require.NoError(t, err)
	_, _, _, na = decodeCert(t, grant.CA, grant.License)
	require.EqualValues(t, expiry, na)

	// An expired account gets nothing.
	past := c.t.Add(-time.Hour).Unix()
	user.ExpiredAt = &past
	grant, err = svc.IssueANX(user, edDevice(t, 1))
	require.NoError(t, err)
	require.Equal(t, StatusExpired, grant.Status)
	require.Empty(t, grant.License)
}

func TestPlansThatIncludeANX(t *testing.T) {
	svc, _ := newService(t)
	set(t, svc.DB, KeyANXEnabled, "true")
	set(t, svc.DB, KeyANXPlanIDs, "3, 5")
	set(t, svc.DB, KeyANXTrialDays, "0")
	grant, err := svc.IssueANX(planUser(1, 5), edDevice(t, 1))
	require.NoError(t, err)
	require.Equal(t, StatusOK, grant.Status)
	grant, err = svc.IssueANX(planUser(2, 4), edDevice(t, 2))
	require.NoError(t, err)
	require.Equal(t, StatusNotEntitled, grant.Status)
}

func TestTrialIsOncePerAccount(t *testing.T) {
	svc, c := newService(t)
	set(t, svc.DB, KeyANXEnabled, "true")
	user := planUser(1, 0)
	start := c.t
	grant, err := svc.IssueANX(user, edDevice(t, 1))
	require.NoError(t, err)
	require.Equal(t, StatusOK, grant.Status)
	require.True(t, grant.Trial)
	_, _, _, na := decodeCert(t, grant.CA, grant.License)
	trialEnd := start.Add(DefaultTrialDays * 24 * time.Hour)
	require.EqualValues(t, trialEnd.Unix(), na)

	// A second device (or a reinstall) on day 2 shares the same trial.
	c.t = start.Add(48 * time.Hour)
	grant, err = svc.IssueANX(user, edDevice(t, 2))
	require.NoError(t, err)
	require.Equal(t, StatusOK, grant.Status)
	_, _, _, na = decodeCert(t, grant.CA, grant.License)
	require.EqualValues(t, trialEnd.Unix(), na)

	// After it, nothing until the plan includes ANX.
	c.t = trialEnd
	grant, err = svc.IssueANX(user, edDevice(t, 3))
	require.NoError(t, err)
	require.Equal(t, StatusNotEntitled, grant.Status)
	plan := uint(1)
	user.PlanID = &plan
	grant, err = svc.IssueANX(user, edDevice(t, 3))
	require.NoError(t, err)
	require.Equal(t, StatusOK, grant.Status)
	require.False(t, grant.Trial)
}

func TestSeatsAndRevocation(t *testing.T) {
	svc, c := newService(t)
	set(t, svc.DB, KeyANXEnabled, "true")
	user := planUser(1, 1)
	limit := 2
	user.DeviceLimit = &limit
	for seed := byte(1); seed <= 2; seed++ {
		grant, err := svc.IssueANX(user, edDevice(t, seed))
		require.NoError(t, err)
		require.Equal(t, StatusOK, grant.Status)
	}
	grant, err := svc.IssueANX(user, edDevice(t, 3))
	require.NoError(t, err)
	require.Equal(t, StatusNoSeat, grant.Status)
	// A device already seated renews.
	grant, err = svc.IssueANX(user, edDevice(t, 2))
	require.NoError(t, err)
	require.Equal(t, StatusOK, grant.Status)

	// Revoking one frees its seat; the revoked device gets nothing.
	devices, err := svc.Devices(user.ID, FeatureANX)
	require.NoError(t, err)
	require.Len(t, devices, 2)
	d1, _ := ParseDevice(edDevice(t, 1))
	id := d1.ID()
	ok, err := svc.Revoke(user.ID, FeatureANX, hex.EncodeToString(id[:]))
	require.NoError(t, err)
	require.True(t, ok)
	grant, err = svc.IssueANX(user, edDevice(t, 1))
	require.NoError(t, err)
	require.Equal(t, StatusRevoked, grant.Status)
	grant, err = svc.IssueANX(user, edDevice(t, 3))
	require.NoError(t, err)
	require.Equal(t, StatusOK, grant.Status)

	// A device silent for the seat window frees its seat as well.
	c.t = c.t.Add(SeatWindow + time.Hour)
	grant, err = svc.IssueANX(user, edDevice(t, 4))
	require.NoError(t, err)
	require.Equal(t, StatusOK, grant.Status)

	ok, err = svc.Restore(user.ID, FeatureANX, hex.EncodeToString(id[:]))
	require.NoError(t, err)
	require.True(t, ok)
}

func TestBadDeviceAndNoRootKey(t *testing.T) {
	svc, _ := newService(t)
	set(t, svc.DB, KeyANXEnabled, "true")
	for _, header := range []string{"", "ed25519", "ed25519:zz", "ed25519:" + strings.Repeat("00", 31), "p256:" + strings.Repeat("04", 65), "rsa:00"} {
		grant, err := svc.IssueANX(planUser(1, 1), header)
		require.NoError(t, err)
		require.Equal(t, StatusBadDevice, grant.Status, header)
	}
	svc.KEK = func() string { return "" }
	grant, err := svc.IssueANX(planUser(1, 1), edDevice(t, 1))
	require.NoError(t, err)
	require.Equal(t, StatusUnavailable, grant.Status)
	ca, _, err := svc.ANXNodeLicense()
	require.NoError(t, err)
	require.Empty(t, ca)
}

func TestParseP256Device(t *testing.T) {
	// The P-256 generator point, uncompressed.
	g := "046b17d1f2e12c4247f8bce6e563a440f277037d812deb33a0f4a13945d898c2964fe342e2fe1a7f9b8ee7eb4a7c0f9e162bce33576b315ececbb6406837bf51f5"
	d, err := ParseDevice("P256:" + g)
	require.NoError(t, err)
	require.Equal(t, AlgP256, d.Alg)
	require.Equal(t, "p256", d.AlgName())
}

// TestANXCertVector pins one certificate byte for byte. anx-core checks the
// same hex (crates/anx-core/tests/panel_license_vector.rs in the anx
// repository): decode, verify against the CA and compare the user_id with
// the one the ANX user key of the UUID gives.
func TestANXCertVector(t *testing.T) {
	ca, err := LicenseCA(testKEK)
	require.NoError(t, err)
	device := Device{Alg: AlgEd25519, PublicKey: ed25519.NewKeyFromSeed(make([]byte, 32)).Public().(ed25519.PublicKey)}
	userID, ok := ANXUserID("00000000-0000-4000-8000-000000000001")
	require.True(t, ok)
	cert := ANXCert(ca, userID, device, 1_800_000_000, 1_800_604_800)
	t.Logf("ca %s", LicenseCAPublicHex(ca))
	t.Logf("cert %s", hex.EncodeToString(cert))
	require.Len(t, cert, 123)
	require.Equal(t, vectorCA, LicenseCAPublicHex(ca))
	require.Equal(t, vectorCert, hex.EncodeToString(cert))
}
