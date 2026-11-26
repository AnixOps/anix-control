package meshregion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// Custom paths (docs/design/mesh-regions): an administrator defines, for
// one source member of a region, the path its traffic to a peer (another
// member of the region) or to a subnet takes: through 1 to 6 waypoint
// members, in order. Control stores members by identity (node:<node ref>,
// device:<device id>) and resolves their addresses when it delivers the
// paths in the network's routing.paths; a path whose member no longer
// resolves is left out of the delivery and the view warns.

// FeaturePaths is the kernel capability of custom paths; a device without
// an Agent gets paths only when its kernel lists it (an Agent withholds
// them from a kernel that does not).
const FeaturePaths = "mesh_paths"

// Path target kinds.
const (
	PathTargetPeer   = "peer"
	PathTargetSubnet = "subnet"
)

// maxPathVia is the most waypoints of one path.
const maxPathVia = 6

// Path warnings.
const (
	// WarningPathMemberMissing: the source, the target or a waypoint does
	// not resolve to an address in the source's region (it left the
	// region, was deleted, is no full node any more or has no address), so
	// the path is not delivered.
	WarningPathMemberMissing = "path_member_missing"
	// WarningPathUnsupported: the source is a device without an Agent
	// whose latest report does not list mesh_paths, so it is given no
	// paths.
	WarningPathUnsupported = "path_unsupported"
)

// PathInput creates or changes a custom path.
type PathInput struct {
	// Source is the member the path starts at: node:<node ref> or
	// device:<device id> of a full node; it never changes on an update
	// (it may then be left out).
	Source string `json:"source"`
	// Target is a member of the source's region (node:<node ref> or
	// device:<device id>, any role) or a private IPv4 subnet in CIDR form.
	Target string `json:"target"`
	// Via are the waypoints in order: 1 to 6 full nodes of the region
	// (node:<node ref> or device:<device id> of a full node).
	Via []string `json:"via"`
}

// PathMemberView is a waypoint with its name and current address.
type PathMemberView struct {
	Entry string `json:"entry"`
	Name  string `json:"name"`
	// Address is empty while the member does not resolve.
	Address string `json:"address"`
}

// PathStatus is what the source's latest report says of the path: its
// state, reason and unreachable waypoint, as the kernel reported them.
type PathStatus struct {
	State       string `json:"state"`
	Reason      any    `json:"reason"`
	Unreachable any    `json:"unreachable"`
}

// PathView is a custom path with the names and addresses its entries
// currently resolve to.
type PathView struct {
	ID         uint             `json:"id"`
	Source     string           `json:"source"`
	SourceName string           `json:"source_name"`
	RegionID   uint             `json:"region_id"`
	RegionCode string           `json:"region_code"`
	TargetKind string           `json:"target_kind"`
	Target     string           `json:"target"`
	TargetName string           `json:"target_name"`
	Via        []PathMemberView `json:"via"`
	CreatedAt  time.Time        `json:"created_at"`
	UpdatedAt  time.Time        `json:"updated_at"`
	// Warnings: WarningPathMemberMissing, WarningPathUnsupported.
	Warnings []string `json:"warnings"`
	// Status is from the source's latest mesh.status report; nil when it
	// has none for this path.
	Status *PathStatus `json:"status"`
}

// parsePathMember reads a member entry: node:<node ref> or
// device:<device id>.
func parsePathMember(field, raw string) (kind, ref, entry string, err error) {
	bad := invalid("%s: %q is not node:<node> or device:<id>", field, strings.TrimSpace(raw))
	kind, ref, ok := strings.Cut(strings.TrimSpace(raw), ":")
	if !ok {
		return "", "", "", bad
	}
	switch kind {
	case AllowNode:
		node, err := agentcontrol.ParseAgentNode(ref)
		if err != nil {
			return "", "", "", bad
		}
		return kind, node.String(), kind + ":" + node.String(), nil
	case AllowDevice:
		id, err := strconv.ParseUint(ref, 10, 32)
		if err != nil || id == 0 {
			return "", "", "", bad
		}
		ref = strconv.FormatUint(id, 10)
		return kind, ref, kind + ":" + ref, nil
	}
	return "", "", "", bad
}

// pathMember is a member entry as it resolves in one region.
type pathMember struct {
	entry string
	name  string
	// found: the member is in the region.
	found bool
	// address is empty while the member has none.
	address string
	// relay: the member forwards for others (a node of the mesh, or a full
	// node device).
	relay bool
}

// resolved tells whether the member has an address in the region.
func (m pathMember) resolved() bool { return m.found && m.address != "" }

// resolvePathMember answers entry as it is in region regionID. The name is
// filled whenever the node or device exists.
func resolvePathMember(tx *gorm.DB, regionID uint, entry string) (pathMember, error) {
	answer := pathMember{entry: entry}
	kind, ref, _, err := parsePathMember("entry", entry)
	if err != nil {
		return answer, nil
	}
	switch kind {
	case AllowNode:
		info, err := lookupNode(tx, ref)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return answer, err
		}
		answer.name = info.name
		var rows []model.MeshMember
		if err := tx.Where("region_id = ? AND node_ref = ?", regionID, ref).Limit(1).Find(&rows).Error; err != nil {
			return answer, err
		}
		if len(rows) > 0 {
			answer.found, answer.relay, answer.address = true, true, rows[0].Address
		}
	case AllowDevice:
		id, _ := strconv.ParseUint(ref, 10, 32)
		var rows []model.MeshDevice
		if err := tx.Where("id = ?", id).Limit(1).Find(&rows).Error; err != nil {
			return answer, err
		}
		if len(rows) == 0 {
			return answer, nil
		}
		answer.name = rows[0].Hostname
		if rows[0].RegionID != regionID {
			return answer, nil
		}
		answer.found = true
		if rows[0].Address != nil {
			answer.address = *rows[0].Address
		}
		_, full, err := isFullNode(tx, rows[0].ID)
		if err != nil {
			return answer, err
		}
		answer.relay = full
	}
	return answer, nil
}

// pathSource answers the region of the path source entry (0 when it is in
// none), its name, and whether it may be a source: a node of the mesh, or
// a device that is a full node.
func pathSource(tx *gorm.DB, entry string) (regionID uint, name string, source bool, err error) {
	kind, ref, _, err := parsePathMember("source", entry)
	if err != nil {
		return 0, "", false, nil
	}
	switch kind {
	case AllowNode:
		info, err := lookupNode(tx, ref)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return 0, "", false, err
		}
		var rows []model.MeshMember
		if err := tx.Where("node_ref = ? AND home = ?", ref, true).Limit(1).Find(&rows).Error; err != nil {
			return 0, "", false, err
		}
		if len(rows) == 0 {
			return 0, info.name, false, nil
		}
		return rows[0].RegionID, info.name, true, nil
	default:
		id, _ := strconv.ParseUint(ref, 10, 32)
		var rows []model.MeshDevice
		if err := tx.Where("id = ?", id).Limit(1).Find(&rows).Error; err != nil || len(rows) == 0 {
			return 0, "", false, err
		}
		_, full, err := isFullNode(tx, rows[0].ID)
		return rows[0].RegionID, rows[0].Hostname, full, err
	}
}

// checkPathTarget reads the target of a path of source in region: a member
// entry of the region, or a private subnet outside the region's range and
// not a part of a host group's segment. It answers the normalized target
// and its kind.
func checkPathTarget(tx *gorm.DB, region netip.Prefix, regionID uint, source, raw string) (target, kind string, err error) {
	raw = strings.TrimSpace(raw)
	if strings.Contains(raw, "/") {
		prefix, err := parseRange("target", raw)
		if err != nil {
			return "", "", err
		}
		if !isPrivate(prefix) {
			return "", "", invalid("target: %s is not a private network", prefix)
		}
		if prefix.Overlaps(region) {
			return "", "", invalid("target: %s overlaps the region's range %s", prefix, region)
		}
		var groups []model.MeshHostGroup
		if err := tx.Find(&groups).Error; err != nil {
			return "", "", err
		}
		for _, group := range groups {
			segment, err := netip.ParsePrefix(group.Segment)
			if err == nil && segment.Overlaps(prefix) && prefix.Bits() > segment.Bits() {
				return "", "", invalid("target: %s lies inside the host group segment %s; the members of a host group follow their gateway's path, target the whole segment", prefix, segment)
			}
		}
		return prefix.String(), PathTargetSubnet, nil
	}
	_, _, entry, err := parsePathMember("target", raw)
	if err != nil {
		return "", "", err
	}
	if entry == source {
		return "", "", invalid("target: a path does not lead to its own source")
	}
	member, err := resolvePathMember(tx, regionID, entry)
	if err != nil {
		return "", "", err
	}
	if !member.found {
		return "", "", invalid("target: %s is not in the source's region", entry)
	}
	return entry, PathTargetPeer, nil
}

// checkPathVia reads the waypoints of a path: 1 to 6 distinct full nodes of
// the region, none of them the source or a peer target.
func checkPathVia(tx *gorm.DB, regionID uint, source, target, kind string, raw []string) ([]string, error) {
	entries := []string{}
	seen := map[string]bool{}
	for _, item := range raw {
		if strings.TrimSpace(item) == "" {
			continue
		}
		_, _, entry, err := parsePathMember("via", item)
		if err != nil {
			return nil, err
		}
		if seen[entry] {
			return nil, invalid("via: %s is listed twice", entry)
		}
		seen[entry] = true
		if entry == source {
			return nil, invalid("via: %s is the path's source", entry)
		}
		if kind == PathTargetPeer && entry == target {
			return nil, invalid("via: %s is the path's target", entry)
		}
		member, err := resolvePathMember(tx, regionID, entry)
		if err != nil {
			return nil, err
		}
		if !member.found {
			return nil, invalid("via: %s is not in the source's region", entry)
		}
		if !member.relay {
			return nil, invalid("via: %s is not a full node", entry)
		}
		entries = append(entries, entry)
	}
	if len(entries) == 0 || len(entries) > maxPathVia {
		return nil, invalid("via: 1 to %d waypoints", maxPathVia)
	}
	return entries, nil
}

// savePath checks input and writes the path row (id 0 creates it).
func savePath(tx *gorm.DB, id uint, input PathInput, now time.Time) (model.MeshPath, error) {
	var row model.MeshPath
	if id != 0 {
		var rows []model.MeshPath
		if err := forUpdate(tx).Where("id = ?", id).Limit(1).Find(&rows).Error; err != nil {
			return row, err
		}
		if len(rows) == 0 {
			return row, fmt.Errorf("%w: path %d", ErrNotFound, id)
		}
		row = rows[0]
	}
	source := row.SourceRef
	if id == 0 || strings.TrimSpace(input.Source) != "" {
		_, _, entry, err := parsePathMember("source", input.Source)
		if err != nil {
			return row, err
		}
		if id != 0 && entry != row.SourceRef {
			return row, invalid("source: the source of a path never changes")
		}
		source = entry
	}
	kind, ref, _, _ := parsePathMember("source", source)
	// The source is locked as removing a node locks it: a node leaving at
	// the same time does not get a path.
	var regionID uint
	switch kind {
	case AllowNode:
		home, err := homeOf(forUpdate(tx), ref)
		if err != nil {
			return row, err
		}
		regionID = home.RegionID
	default:
		deviceID, _ := strconv.ParseUint(ref, 10, 32)
		var device model.MeshDevice
		if err := findDevice(forUpdate(tx), uint(deviceID), &device); err != nil {
			if errors.Is(err, ErrNotFound) {
				return row, invalid("source: %s does not exist", source)
			}
			return row, err
		}
		_, full, err := isFullNode(tx, device.ID)
		if err != nil {
			return row, err
		}
		if !full {
			return row, invalid("source: %s is an access device; only a node or a full node device has paths", source)
		}
		regionID = device.RegionID
	}
	var region model.MeshRegion
	if err := findRegion(tx, regionID, &region); err != nil {
		return row, err
	}
	prefix, err := netip.ParsePrefix(region.CIDR)
	if err != nil {
		return row, fmt.Errorf("region %q: %w", region.Code, err)
	}
	target, targetKind, err := checkPathTarget(tx, prefix, regionID, source, input.Target)
	if err != nil {
		return row, err
	}
	via, err := checkPathVia(tx, regionID, source, target, targetKind, input.Via)
	if err != nil {
		return row, err
	}
	var count int64
	if err := tx.Model(&model.MeshPath{}).Where("source_ref = ? AND target = ? AND id <> ?", source, target, id).Count(&count).Error; err != nil {
		return row, err
	}
	if count > 0 {
		return row, conflict("%s has a path to %s already", source, target)
	}
	if id == 0 {
		row = model.MeshPath{CreatedAt: now}
	}
	row.SourceRef, row.TargetKind, row.Target, row.Via, row.UpdatedAt = source, targetKind, target, strings.Join(via, ","), now
	if id == 0 {
		return row, tx.Create(&row).Error
	}
	// Only its own columns, with the given time: saving the whole row would
	// stamp updated_at with the clock instead.
	return row, updateExisting(tx, &model.MeshPath{}, row.ID, map[string]any{"target_kind": row.TargetKind, "target": row.Target, "via": row.Via, "updated_at": now}, "path")
}

// CreatePath adds a custom path.
func CreatePath(ctx context.Context, db *gorm.DB, input PathInput, now time.Time) (model.MeshPath, error) {
	var row model.MeshPath
	err := change(ctx, db, func(tx *gorm.DB) error {
		var err error
		row, err = savePath(tx, 0, input, now)
		return err
	})
	return row, err
}

// UpdatePath changes a path's target and waypoints; its source never
// changes.
func UpdatePath(ctx context.Context, db *gorm.DB, id uint, input PathInput, now time.Time) (model.MeshPath, error) {
	var row model.MeshPath
	err := change(ctx, db, func(tx *gorm.DB) error {
		var err error
		row, err = savePath(tx, id, input, now)
		return err
	})
	return row, err
}

// DeletePath removes a custom path.
func DeletePath(ctx context.Context, db *gorm.DB, id uint) error {
	return change(ctx, db, func(tx *gorm.DB) error {
		result := tx.Where("id = ?", id).Delete(&model.MeshPath{})
		if result.Error == nil && result.RowsAffected == 0 {
			return fmt.Errorf("%w: path %d", ErrNotFound, id)
		}
		return result.Error
	})
}

// deleteSourcePaths deletes the paths of the source entry: a node that
// left its region, or a device that was deleted or moved, has none. Paths
// that only name it as target or waypoint stay and warn.
func deleteSourcePaths(tx *gorm.DB, source string) error {
	return tx.Where("source_ref = ?", source).Delete(&model.MeshPath{}).Error
}

// deviceEntry is the path entry of a device.
func deviceEntry(id uint) string { return AllowDevice + ":" + strconv.FormatUint(uint64(id), 10) }

// resolvePath answers the kernel entry of row in region regionID, false
// when a member of it does not resolve to an address there.
func resolvePath(tx *gorm.DB, regionID uint, row model.MeshPath) (map[string]any, bool, error) {
	entry := map[string]any{}
	if row.TargetKind == PathTargetSubnet {
		entry["to_subnet"] = row.Target
	} else {
		target, err := resolvePathMember(tx, regionID, row.Target)
		if err != nil {
			return nil, false, err
		}
		if !target.resolved() {
			return nil, false, nil
		}
		entry["to_peer"] = target.address
	}
	via := []string{}
	for _, item := range splitSubnets(row.Via) {
		member, err := resolvePathMember(tx, regionID, item)
		if err != nil {
			return nil, false, err
		}
		if !member.resolved() || !member.relay {
			return nil, false, nil
		}
		via = append(via, member.address)
	}
	entry["via"] = via
	return entry, true, nil
}

// deliveredPaths answers the kernel entries of the paths of the source
// entry in region regionID, in id order, without the paths that do not
// resolve.
func deliveredPaths(tx *gorm.DB, source string, regionID uint) ([]map[string]any, error) {
	var rows []model.MeshPath
	if err := tx.Where("source_ref = ?", source).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	answer := []map[string]any{}
	for _, row := range rows {
		entry, ok, err := resolvePath(tx, regionID, row)
		if err != nil {
			return nil, err
		}
		if ok {
			answer = append(answer, entry)
		}
	}
	return answer, nil
}

// pathsValue is the routing.paths value of a network: the entries, or an
// empty list.
func pathsValue(entries []map[string]any) any {
	if len(entries) == 0 {
		return []any{}
	}
	return entries
}

// reportedPath answers the path id's entry in network code of a
// "mesh.status" payload, nil when it has none.
func reportedPath(payload, code, id string) *PathStatus {
	var shape struct {
		Networks []struct {
			ID    string `json:"id"`
			Paths []struct {
				ID          string `json:"id"`
				State       string `json:"state"`
				Reason      any    `json:"reason"`
				Unreachable any    `json:"unreachable"`
			} `json:"paths"`
		} `json:"networks"`
	}
	if json.Unmarshal([]byte(payload), &shape) != nil {
		return nil
	}
	for _, network := range shape.Networks {
		if network.ID != code {
			continue
		}
		for _, path := range network.Paths {
			if path.ID == id {
				return &PathStatus{State: path.State, Reason: path.Reason, Unreachable: path.Unreachable}
			}
		}
	}
	return nil
}

// statusRef is the key of the source entry's status: the node's ref, or
// the device's status key.
func statusRef(entry string) string {
	kind, ref, _, err := parsePathMember("source", entry)
	if err != nil {
		return ""
	}
	if kind == AllowDevice {
		id, _ := strconv.ParseUint(ref, 10, 32)
		return DeviceRef(uint(id))
	}
	return ref
}

// Paths answers every custom path with the names and addresses of its
// members, warnings and the state its source last reported.
func Paths(ctx context.Context, db *gorm.DB) ([]PathView, error) {
	db = db.WithContext(ctx)
	var rows []model.MeshPath
	if err := db.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	codes, err := regionCodes(db)
	if err != nil {
		return nil, err
	}
	statuses := map[string]model.MeshStatus{}
	if len(rows) > 0 {
		refs := make([]string, 0, len(rows))
		for _, row := range rows {
			refs = append(refs, statusRef(row.SourceRef))
		}
		var found []model.MeshStatus
		if err := db.Where("node_ref IN ?", refs).Find(&found).Error; err != nil {
			return nil, err
		}
		for _, status := range found {
			statuses[status.NodeRef] = status
		}
	}
	answer := make([]PathView, 0, len(rows))
	for _, row := range rows {
		view := PathView{ID: row.ID, Source: row.SourceRef, TargetKind: row.TargetKind, Target: row.Target,
			Via: []PathMemberView{}, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, Warnings: []string{}}
		regionID, name, isSource, err := pathSource(db, row.SourceRef)
		if err != nil {
			return nil, err
		}
		view.SourceName, view.RegionID, view.RegionCode = name, regionID, codes[regionID]
		missing := !isSource
		view.TargetName = row.Target
		targetAddress := ""
		if row.TargetKind == PathTargetPeer {
			target, err := resolvePathMember(db, regionID, row.Target)
			if err != nil {
				return nil, err
			}
			view.TargetName, targetAddress = target.name, target.address
			missing = missing || !target.resolved()
		}
		for _, item := range splitSubnets(row.Via) {
			member, err := resolvePathMember(db, regionID, item)
			if err != nil {
				return nil, err
			}
			view.Via = append(view.Via, PathMemberView{Entry: item, Name: member.name, Address: member.address})
			missing = missing || !member.resolved() || !member.relay
		}
		if missing {
			view.Warnings = append(view.Warnings, WarningPathMemberMissing)
		}
		status, hasStatus := statuses[statusRef(row.SourceRef)]
		if strings.HasPrefix(row.SourceRef, AllowDevice+":") {
			features := map[string]bool{}
			if hasStatus {
				features = reportedFeatures(status.Payload)
			}
			if !features[FeaturePaths] {
				view.Warnings = append(view.Warnings, WarningPathUnsupported)
			}
		}
		if hasStatus && !missing {
			id := "subnet:" + row.Target
			if row.TargetKind == PathTargetPeer {
				id = "peer:" + targetAddress
			}
			view.Status = reportedPath(status.Payload, view.RegionCode, id)
		}
		answer = append(answer, view)
	}
	return answer, nil
}
