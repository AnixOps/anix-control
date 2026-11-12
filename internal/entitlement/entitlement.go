// Package entitlement licenses features to devices: the panel side of the
// AnixOps licensing (anx spec §2.4.1, FUTURE-IMPROVEMENTS F-79 in the anx
// repository).
//
// It is generic: a user is entitled to a feature (a name such as "anx")
// through the plan, or for a few days through the account's trial, and each
// device that uses the feature takes a seat out of the user's device limit.
// What a license looks like belongs to the feature: for "anx" it is the
// compact device certificate of anx spec §2.4, signed by the license CA
// whose public key the ANX nodes trust as license_ca.
//
//   - The license CA is Ed25519, its seed derived from the root key
//     (rootkey.LabelLicenseCA): nothing to store, and a root key rotation
//     rotates it.
//   - A client asks for a license when it refreshes the subscription, with
//     the header X-AnixOps-Device: "<alg>:<hex public key>". The answer
//     carries the license (X-AnixOps-Anx-License, hex), the CA's public key
//     (X-AnixOps-Anx-License-Ca) and a status (X-AnixOps-Anx-License-Status).
//   - A license lasts 7 days (LicenseValidity), never past the account's
//     expiry or the end of the trial: that is the offline grace, and
//     revoking a device stops its renewals.
//   - Seats: devices that asked within SeatWindow and are not revoked.
//
// Everything is off until the administrator turns entitlement.anx.enabled
// on: no header is answered and the ANX nodes get no license_ca.
package entitlement

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/rootkey"
	"gorm.io/gorm"
)

// FeatureANX is the feature name of the ANX protocol.
const FeatureANX = "anx"

// Settings keys (system configuration).
const (
	// KeyANXEnabled turns ANX licensing on: subscriptions answer license
	// requests and ANX nodes trust the license CA.
	KeyANXEnabled = "entitlement.anx.enabled"
	// KeyANXRequire makes the ANX nodes refuse unlicensed sessions
	// (require_license); off, nodes only accept licenses (a dry run).
	KeyANXRequire = "entitlement.anx.require"
	// KeyANXTrialDays is the account trial in days; 0 turns it off.
	KeyANXTrialDays = "entitlement.anx.trial_days"
	// KeyANXPlanIDs lists the plans (comma-separated ids) that include
	// ANX; empty means every plan.
	KeyANXPlanIDs = "entitlement.anx.plan_ids"
)

// HTTP headers of the subscription exchange.
const (
	HeaderDevice           = "X-AnixOps-Device"
	HeaderANXLicense       = "X-AnixOps-Anx-License"
	HeaderANXLicenseCA     = "X-AnixOps-Anx-License-Ca"
	HeaderANXLicenseStatus = "X-AnixOps-Anx-License-Status"
)

const (
	// LicenseValidity is how long a license lasts: the offline grace.
	LicenseValidity = 7 * 24 * time.Hour
	// DefaultTrialDays is the trial while KeyANXTrialDays is unset.
	DefaultTrialDays = 3
	// MaxTrialDays bounds KeyANXTrialDays.
	MaxTrialDays = 30
	// SeatWindow is how recently a device must have asked to take a seat.
	SeatWindow = 30 * 24 * time.Hour
	// backdate is how far a license's not_before goes back, for clients
	// whose clock is a little behind (anx spec §2.4).
	backdate = 5 * time.Minute
)

// Status is the outcome of a license request.
type Status string

// Statuses.
const (
	StatusOK          Status = "ok"
	StatusBadDevice   Status = "bad_device"
	StatusExpired     Status = "account_expired"
	StatusNotEntitled Status = "not_entitled"
	StatusNoSeat      Status = "no_seat"
	StatusRevoked     Status = "revoked"
	StatusUnavailable Status = "unavailable"
)

// ErrNoRootKey means the root key is not loaded, so there is no license CA.
var ErrNoRootKey = errors.New("entitlement: no root key")

// Settings are the ANX licensing settings in force.
type Settings struct {
	Enabled   bool
	Require   bool
	TrialDays int
	// PlanIDs are the plans that include ANX; empty is every plan.
	PlanIDs []uint
}

// LoadSettings reads the settings; an unset key takes its default (off).
func LoadSettings(db *gorm.DB) (Settings, error) {
	values := map[string]string{}
	var rows []model.SystemConfig
	if err := db.Where("key IN ?", []string{KeyANXEnabled, KeyANXRequire, KeyANXTrialDays, KeyANXPlanIDs}).Find(&rows).Error; err != nil {
		return Settings{}, err
	}
	for _, r := range rows {
		values[r.Key] = strings.TrimSpace(r.Value)
	}
	s := Settings{
		Enabled:   parseBool(values[KeyANXEnabled]),
		Require:   parseBool(values[KeyANXRequire]),
		TrialDays: DefaultTrialDays,
	}
	if v, ok := values[KeyANXTrialDays]; ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= MaxTrialDays {
			s.TrialDays = n
		}
	}
	for _, part := range strings.Split(values[KeyANXPlanIDs], ",") {
		if n, err := strconv.ParseUint(strings.TrimSpace(part), 10, 32); err == nil && n > 0 {
			s.PlanIDs = append(s.PlanIDs, uint(n))
		}
	}
	return s, nil
}

func parseBool(v string) bool {
	b, err := strconv.ParseBool(v)
	return err == nil && b
}

// includes reports whether the user's plan includes the feature.
func (s Settings) includes(user *model.User) bool {
	if user.PlanID == nil || *user.PlanID == 0 {
		return false
	}
	if len(s.PlanIDs) == 0 {
		return true
	}
	for _, id := range s.PlanIDs {
		if id == *user.PlanID {
			return true
		}
	}
	return false
}

// LicenseCA is the license CA's key, derived from the root key (the text of
// module_runtime.ca_kek, base64 or hex).
func LicenseCA(kek string) (ed25519.PrivateKey, error) {
	if strings.TrimSpace(kek) == "" {
		return nil, ErrNoRootKey
	}
	key, err := rootkey.ParseKEK(kek)
	if err != nil {
		return nil, err
	}
	return ed25519.NewKeyFromSeed(key.Derive(rootkey.LabelLicenseCA)), nil
}

// LicenseCAPublicHex is the hex of the license CA's public key, the value
// of an ANX node's license_ca.
func LicenseCAPublicHex(ca ed25519.PrivateKey) string {
	return hex.EncodeToString(ca.Public().(ed25519.PublicKey))
}

// anxPanelUserKeyLabel is the label of the panel's ANX user keys
// (service.ANXUserKey).
const anxPanelUserKeyLabel = "anx/panel-user-key/v1"

// ANXUserID is the ANX user_id of the user with uuid: the panel's user key
// is HMAC-SHA-256(uuid, "anx/panel-user-key/v1") and anx spec §2.1 takes
// user_id = HMAC-SHA-256(user_key, "anx/v1/user-id")[..8].
func ANXUserID(uuid string) ([8]byte, bool) {
	var id [8]byte
	uuid = strings.ToLower(strings.TrimSpace(uuid))
	if uuid == "" {
		return id, false
	}
	mac := hmac.New(sha256.New, []byte(uuid))
	mac.Write([]byte(anxPanelUserKeyLabel))
	userKey := mac.Sum(nil)
	mac = hmac.New(sha256.New, userKey)
	mac.Write([]byte("anx/v1/user-id"))
	copy(id[:], mac.Sum(nil))
	return id, true
}
