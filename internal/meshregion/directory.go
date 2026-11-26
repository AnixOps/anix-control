package meshregion

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/netip"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// The region directory (appendix D.2 of the mesh protocol): the regions
// with the probe addresses and static public keys of their border nodes'
// probe responders, which a node measures to choose or change its region.
// A device reads it over HTTPS (DeviceDirectory); an Agent node gets it
// inside the member "mesh" (region.directory, regionSection).

// DefaultResponderPort is the UDP port of a border node's probe responder
// until an administrator saves another (Settings.ResponderPort).
const DefaultResponderPort = 11013

// The directory's bounds (D.2.2).
const (
	DirectoryVersion = 1
	maxDirRegions    = 64
	maxDirBorders    = 16
)

// Directory is the directory object (D.2.2).
type Directory struct {
	Version int `json:"version"`
	// Preferred is the code of the preferred region, null for none; only
	// for display, the panel applies the factor (D.5).
	Preferred *string           `json:"preferred"`
	Regions   []DirectoryRegion `json:"regions"`
}

// DirectoryRegion is an enabled region with at least one border node that
// has a probe key.
type DirectoryRegion struct {
	Code    string            `json:"code"`
	Subnet  string            `json:"subnet"`
	Borders []DirectoryBorder `json:"borders"`
}

// DirectoryBorder is a border node's probe responder.
type DirectoryBorder struct {
	// Name is a DNS label of the node, unique in its region.
	Name string `json:"name"`
	// Probe is the node's address literal and the responder port.
	Probe string `json:"probe"`
	// Key is the responder's X25519 public key, standard base64.
	Key string `json:"key"`
}

// probeKey reads a responder public key as the kernel states it: 32 bytes
// in standard base64 with padding (44 characters), not all zero.
func probeKey(raw string) (string, bool) {
	if len(raw) != 44 {
		return "", false
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(decoded) != 32 || base64.StdEncoding.EncodeToString(decoded) != raw {
		return "", false
	}
	for _, b := range decoded {
		if b != 0 {
			return raw, true
		}
	}
	return "", false
}

// reportedProbe is the part of a "mesh.status" payload the directory and
// the automatic region choice read: its region_probe (appendix D.9.2).
type reportedProbe struct {
	RegionProbe *struct {
		Responder *struct {
			PublicKey string `json:"public_key"`
		} `json:"responder"`
		Regions []struct {
			Region string   `json:"region"`
			RTT    *float64 `json:"rtt_ms"`
		} `json:"regions"`
		Suggestion json.RawMessage `json:"suggestion"`
	} `json:"region_probe"`
}

// responderKey answers the probe key a status payload reports, if valid.
func responderKey(payload string) (string, bool) {
	var shape reportedProbe
	if json.Unmarshal([]byte(payload), &shape) != nil || shape.RegionProbe == nil || shape.RegionProbe.Responder == nil {
		return "", false
	}
	return probeKey(shape.RegionProbe.Responder.PublicKey)
}

// nodeProbeAddr answers the address a node's probe responder is reached
// at: the first of its server address, the public address it joined from
// and its host that is an IP literal and a unicast address (no loopback,
// link-local, multicast, broadcast or unspecified address; a private
// address is fine, D.2.2). false when it has none: a host name is not
// resolved.
func nodeProbeAddr(db *gorm.DB, ref string) (netip.Addr, bool, error) {
	node, err := agentcontrol.ParseAgentNode(ref)
	if err != nil {
		return netip.Addr{}, false, nil
	}
	var candidates []string
	if node.Kind == agentcontrol.NodeKindForward {
		var rows []struct{ LastPublicAddress, Host string }
		if err := db.Model(&model.ForwardNode{}).Select("last_public_address", "host").Where("id = ?", node.ID).Limit(1).Find(&rows).Error; err != nil {
			return netip.Addr{}, false, err
		}
		for _, row := range rows {
			candidates = []string{row.LastPublicAddress, row.Host}
		}
	} else {
		var rows []struct {
			ServerIP          *string
			LastPublicAddress string
			Host              string
		}
		if err := db.Model(&model.Node{}).Select("server_ip", "last_public_address", "host").Where("id = ?", node.ID).Limit(1).Find(&rows).Error; err != nil {
			return netip.Addr{}, false, err
		}
		for _, row := range rows {
			server := ""
			if row.ServerIP != nil {
				server = *row.ServerIP
			}
			candidates = []string{server, row.LastPublicAddress, row.Host}
		}
	}
	for _, raw := range candidates {
		if address, ok := probeLiteral(raw); ok {
			return address, true, nil
		}
	}
	return netip.Addr{}, false, nil
}

// probeLiteral reads raw as an IP literal (with or without a port, an IPv6
// one with or without brackets) of a unicast address.
func probeLiteral(raw string) (netip.Addr, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return netip.Addr{}, false
	}
	address, err := netip.ParseAddr(strings.Trim(raw, "[]"))
	if err != nil {
		port, err := netip.ParseAddrPort(raw)
		if err != nil {
			return netip.Addr{}, false
		}
		address = port.Addr()
	}
	address = address.Unmap()
	if address.Zone() != "" || !address.IsGlobalUnicast() {
		return netip.Addr{}, false
	}
	return address, true
}

// borderName answers the border's display name: the node's name as an
// intranet name (a DNS label), else node-<ref>; a name another border of
// the region has gets the node's ref after it.
func borderName(name, ref string, taken map[string]bool) string {
	label := normalizeName(name)
	if label == "" || len(label) > 63 || !labelPattern.MatchString(label) {
		label = "node-" + ref
	}
	if taken[label] {
		suffix := "-" + ref
		label = strings.TrimRight(label[:min(len(label), 63-len(suffix))], "-") + suffix
	}
	taken[label] = true
	return label
}

// BuildDirectory answers the directory: the enabled regions in order
// (sort, then id) with the border nodes (the home membership of a boundary
// node) whose latest report names a valid probe key and that have an
// address to probe; a region with none is left out. preferredID is the
// preferred region (0: none).
func BuildDirectory(db *gorm.DB, preferredID uint) (Directory, error) {
	directory := Directory{Version: DirectoryVersion, Regions: []DirectoryRegion{}}
	settings, err := loadSettings(db)
	if err != nil {
		return directory, err
	}
	var regions []model.MeshRegion
	if err := db.Order("sort ASC, id ASC").Find(&regions).Error; err != nil {
		return directory, err
	}
	enabled := []uint{}
	for _, region := range regions {
		if region.ID == preferredID && preferredID != 0 {
			code := region.Code
			directory.Preferred = &code
		}
		if region.Enabled {
			enabled = append(enabled, region.ID)
		}
	}
	if len(enabled) == 0 {
		return directory, nil
	}
	var borders []model.MeshMember
	if err := db.Where("region_id IN ? AND home = ? AND boundary = ?", enabled, true, true).Order("id ASC").Find(&borders).Error; err != nil {
		return directory, err
	}
	refs := make([]string, 0, len(borders))
	for _, border := range borders {
		refs = append(refs, border.NodeRef)
	}
	var statuses []model.MeshStatus
	if len(refs) > 0 {
		if err := db.Where("node_ref IN ?", refs).Find(&statuses).Error; err != nil {
			return directory, err
		}
	}
	keys := map[string]string{}
	for _, status := range statuses {
		if key, ok := responderKey(status.Payload); ok {
			keys[status.NodeRef] = key
		}
	}
	byRegion := map[uint][]model.MeshMember{}
	for _, border := range borders {
		byRegion[border.RegionID] = append(byRegion[border.RegionID], border)
	}
	for _, region := range regions {
		if !region.Enabled || len(directory.Regions) == maxDirRegions {
			continue
		}
		taken := map[string]bool{}
		listed := []DirectoryBorder{}
		for _, border := range byRegion[region.ID] {
			key, ok := keys[border.NodeRef]
			if !ok || len(listed) == maxDirBorders {
				continue
			}
			address, ok, err := nodeProbeAddr(db, border.NodeRef)
			if err != nil {
				return directory, err
			}
			if !ok {
				continue
			}
			info, err := lookupNode(db, border.NodeRef)
			if err != nil {
				continue
			}
			listed = append(listed, DirectoryBorder{
				Name:  borderName(info.name, border.NodeRef, taken),
				Probe: netip.AddrPortFrom(address, uint16(settings.ResponderPort)).String(), // #nosec G115 -- 1 to 65535, checked on save.
				Key:   key,
			})
		}
		if len(listed) > 0 {
			directory.Regions = append(directory.Regions, DirectoryRegion{Code: region.Code, Subnet: region.CIDR, Borders: listed})
		}
	}
	return directory, nil
}

// DeviceDirectory answers the directory to the holder of token, an
// enrollment credential (mse_) or a device token (msd_). A credential is
// not used up and nothing is written for it: it counts while it is
// unexpired and not withdrawn, and either unused or used no longer ago
// than EnrollRetryWindow (as a retry of its enrollment would). The
// preferred region is the credential's, else the settings' default; a
// device token has the default. ErrUnauthorized otherwise.
func DeviceDirectory(ctx context.Context, db *gorm.DB, token string, now time.Time) (Directory, error) {
	db = db.WithContext(ctx)
	var preferred *uint
	switch {
	case strings.HasPrefix(token, enrollmentPrefix):
		var rows []model.MeshEnrollment
		if err := db.Where("token_hash = ? AND expires_at > ?", hashToken(token), now).Limit(1).Find(&rows).Error; err != nil {
			return Directory{}, err
		}
		if len(rows) == 0 || (rows[0].UsedAt != nil && now.Sub(*rows[0].UsedAt) > EnrollRetryWindow) {
			return Directory{}, ErrUnauthorized
		}
		preferred = rows[0].PreferredRegionID
	case strings.HasPrefix(token, deviceTokenPrefix):
		if _, err := Authenticate(ctx, db, token, now); err != nil {
			return Directory{}, err
		}
	default:
		return Directory{}, ErrUnauthorized
	}
	id, err := preferredRegionID(db, preferred)
	if err != nil {
		return Directory{}, err
	}
	return BuildDirectory(db, id)
}
