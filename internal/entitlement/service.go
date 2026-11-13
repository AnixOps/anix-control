package entitlement

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// Service issues and revokes licenses.
type Service struct {
	DB *gorm.DB
	// KEK answers the root key's text; nil reads module_runtime.ca_kek
	// from the loaded configuration.
	KEK func() string
	// Now is the clock; nil is time.Now.
	Now func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *Service) ca() (ed25519.PrivateKey, error) {
	var raw string
	if s.KEK != nil {
		raw = s.KEK()
	} else if cfg := config.Get(); cfg != nil {
		raw = cfg.ModuleRuntime.CAKEK
	}
	return LicenseCA(raw)
}

// Grant is the answer to a license request.
type Grant struct {
	Status Status
	// License is the signed license (for anx, the device certificate);
	// empty unless Status is StatusOK.
	License []byte
	// CA is the hex of the license CA's public key.
	CA string
	// NotAfter is when the license ends.
	NotAfter time.Time
	// Trial marks a license from the account's trial.
	Trial bool
}

// ANXEnabled reports whether ANX licensing is on.
func (s *Service) ANXEnabled() (bool, error) {
	settings, err := LoadSettings(s.DB)
	return settings.Enabled, err
}

// IssueANX answers the ANX license request of user's device (the value of
// X-AnixOps-Device). It returns nil while ANX licensing is off.
func (s *Service) IssueANX(user *model.User, deviceHeader string) (*Grant, error) {
	settings, err := LoadSettings(s.DB)
	if err != nil || !settings.Enabled {
		return nil, err
	}
	ca, err := s.ca()
	if err != nil {
		// Without a root key there is no CA; the client learns why.
		return &Grant{Status: StatusUnavailable}, nil
	}
	grant := &Grant{CA: LicenseCAPublicHex(ca)}
	device, err := ParseDevice(deviceHeader)
	if err != nil {
		grant.Status = StatusBadDevice
		return grant, nil
	}
	now := s.now()
	userID, ok := ANXUserID(user.UUID)
	if !ok || user.Banned == 1 || (user.ExpiredAt != nil && *user.ExpiredAt <= now.Unix()) {
		grant.Status = StatusExpired
		return grant, nil
	}
	deviceID := device.ID()
	idHex := hex.EncodeToString(deviceID[:])

	var row model.EntitlementDevice
	err = s.DB.Where("user_id = ? AND feature = ? AND device_id = ?", user.ID, FeatureANX, idHex).First(&row).Error
	known := err == nil
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if known && row.RevokedAt != nil {
		grant.Status = StatusRevoked
		return grant, nil
	}

	notAfter := now.Add(LicenseValidity)
	trial := !settings.includes(user)
	if trial {
		end, err := s.trialEnd(user.ID, settings.TrialDays, now)
		if err != nil {
			return nil, err
		}
		if !now.Before(end) {
			grant.Status = StatusNotEntitled
			return grant, nil
		}
		if end.Before(notAfter) {
			notAfter = end
		}
	}
	if user.ExpiredAt != nil {
		if end := time.Unix(*user.ExpiredAt, 0).UTC(); end.Before(notAfter) {
			notAfter = end
		}
	}

	if limit := user.GetDeviceLimit(); limit > 0 {
		var others int64
		if err := s.DB.Model(&model.EntitlementDevice{}).
			Where("user_id = ? AND feature = ? AND device_id <> ? AND revoked_at IS NULL AND last_seen_at >= ?",
				user.ID, FeatureANX, idHex, now.Add(-SeatWindow)).
			Count(&others).Error; err != nil {
			return nil, err
		}
		if others >= int64(limit) {
			grant.Status = StatusNoSeat
			return grant, nil
		}
	}

	notBefore := now.Add(-backdate)
	grant.License = ANXCert(ca, userID, device, uint32(notBefore.Unix()), uint32(notAfter.Unix())) // #nosec G115 -- Unix seconds fit in uint32 until 2106, the spec's field.
	grant.NotAfter = notAfter
	grant.Trial = trial
	grant.Status = StatusOK

	if known {
		err = s.DB.Model(&row).Updates(map[string]any{
			"trial": trial, "not_after": notAfter.Unix(), "last_seen_at": now,
		}).Error
	} else {
		row = model.EntitlementDevice{
			UserID: user.ID, Feature: FeatureANX, DeviceID: idHex, Alg: device.AlgName(),
			PublicKey: hex.EncodeToString(device.PublicKey), Trial: trial, NotAfter: notAfter.Unix(),
			CreatedAt: now, LastSeenAt: now,
		}
		err = s.DB.Create(&row).Error
	}
	if err != nil {
		return nil, err
	}
	return grant, nil
}

// trialEnd is when the account's trial ends: trialDays after its first
// trial device, or after now if it has none yet. One trial per account,
// however many devices or reinstalls.
func (s *Service) trialEnd(userID uint, trialDays int, now time.Time) (time.Time, error) {
	if trialDays <= 0 {
		return now, nil
	}
	var first model.EntitlementDevice
	err := s.DB.Where("user_id = ? AND feature = ? AND trial = ?", userID, FeatureANX, true).
		Order("created_at").First(&first).Error
	start := now
	switch {
	case err == nil:
		start = first.CreatedAt.UTC()
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return time.Time{}, err
	}
	return start.Add(time.Duration(trialDays) * 24 * time.Hour), nil
}

// Devices lists a user's devices for feature, newest first.
func (s *Service) Devices(userID uint, feature string) ([]model.EntitlementDevice, error) {
	var rows []model.EntitlementDevice
	err := s.DB.Where("user_id = ? AND feature = ?", userID, feature).Order("last_seen_at DESC").Find(&rows).Error
	return rows, err
}

// Revoke revokes a user's device: it gets no new license and frees its
// seat. The license it holds runs out within LicenseValidity. It reports
// whether the device was found.
func (s *Service) Revoke(userID uint, feature, deviceID string) (bool, error) {
	res := s.DB.Model(&model.EntitlementDevice{}).
		Where("user_id = ? AND feature = ? AND device_id = ? AND revoked_at IS NULL", userID, feature, deviceID).
		Update("revoked_at", s.now())
	return res.RowsAffected > 0, res.Error
}

// Restore lifts a revocation.
func (s *Service) Restore(userID uint, feature, deviceID string) (bool, error) {
	res := s.DB.Model(&model.EntitlementDevice{}).
		Where("user_id = ? AND feature = ? AND device_id = ?", userID, feature, deviceID).
		Update("revoked_at", nil)
	return res.RowsAffected > 0, res.Error
}

// ANXNodeLicense is what an ANX node's inbound gets: the license CA it
// trusts and whether it refuses unlicensed sessions. ca is "" while ANX
// licensing is off or the root key is missing.
func (s *Service) ANXNodeLicense() (ca string, require bool, err error) {
	settings, err := LoadSettings(s.DB)
	if err != nil || !settings.Enabled {
		return "", false, err
	}
	key, err := s.ca()
	if err != nil {
		// Without a root key nodes keep running unlicensed.
		return "", false, nil
	}
	return LicenseCAPublicHex(key), settings.Require, nil
}

// LicenseCAHex answers the hex of the license CA's public key;
// ErrNoRootKey without a root key.
func (s *Service) LicenseCAHex() (string, error) {
	key, err := s.ca()
	if err != nil {
		return "", err
	}
	return LicenseCAPublicHex(key), nil
}
