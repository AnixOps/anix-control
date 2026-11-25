package meshregion

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// External network bridges (R36, spec section 15.12): a Linux node of ours
// that is also on another overlay network (the external network) advertises
// chosen external addresses and subnets in its home region, and lets only
// the members an administrator allowed reach them, translating their source
// address to its own on the external interface. Control lists the allowed
// members by identity and gives the node their current addresses
// (expandAllow); the node's kernel checks the source address only.
//
// An address or segment that leaves a bridge node's allow list is held
// (model.MeshBridgeHold): it is not given to another member until the node
// reports a list without it applied (spec section 15.12 item 4), so a
// former member's access never passes to the next holder of its address.

// FeatureExternalBridge is the kernel capability of external network
// bridges; the Agent gives the section external_bridge only to a kernel
// that lists it.
const FeatureExternalBridge = "mesh_external_bridge"

// Bridge limits.
const (
	maxBridgeRoutes = 64
	maxBridgeAllow  = 256
)

// Allow list entry kinds: a node of the mesh (its address in the bridge's
// region), a device (its address, while it holds an address and a static
// key) or a host group (its segment, while its gateway's home is the
// bridge's region).
const (
	AllowNode      = "node"
	AllowDevice    = "device"
	AllowHostGroup = "host_group"
)

// Bridge warnings.
const (
	// WarningBridgeUnconfirmed: the bridge node has not reported the allow
	// list it was given in force (its digest), or addresses wait for it to
	// (it may be offline); addresses that left the list stay held until it
	// does.
	WarningBridgeUnconfirmed = "bridge_unconfirmed"
	// WarningBridgeUnsupported: the node's kernel does not list
	// mesh_external_bridge, so the Agent leaves the bridge out.
	WarningBridgeUnsupported = "bridge_unsupported"
	// WarningAllowOutsideRegion: an allowed member, device or host group
	// is not in the bridge's region (or no longer exists) and is left
	// out of the list.
	WarningAllowOutsideRegion = "allow_outside_region"
)

// SharedAddressSpaceV6 is the IPv6 range other overlay software commonly
// takes over whole (spec section 15.12 item 1).
var SharedAddressSpaceV6 = netip.MustParsePrefix("fd7a:115c:a1e0::/48")

// privateRangeV6 is the IPv6 range of the spec's private address whitelist
// (section 15.4).
var privateRangeV6 = netip.MustParsePrefix("fc00::/7")

// interfacePattern is a Linux network interface name: 1 to 15 bytes, no
// slash, colon or white space.
var interfacePattern = regexp.MustCompile(`^[A-Za-z0-9_.@-]{1,15}$`)

// BridgeInput creates or changes an external network bridge.
type BridgeInput struct {
	// Node is the bridge node (proxy-<id>, forward-<id>) in the mesh; it
	// never changes on an update.
	Node string `json:"node"`
	// Interface is the node's network interface on the external network.
	Interface string `json:"interface"`
	// Routes are the external addresses (IPv4 /32, IPv6 /128, also inside
	// 100.64.0.0/10 and fd7a:115c:a1e0::/48) and subnets (inside the
	// private ranges, outside those two) the node advertises; at most 64.
	Routes []string `json:"routes"`
	// Allow are the sources that may reach them: node:<node ref>,
	// device:<device id> or host_group:<host group id>; at most 256.
	Allow []string `json:"allow"`
}

// BridgeAllowView is an allow list entry with what it currently expands
// to.
type BridgeAllowView struct {
	Entry string `json:"entry"`
	Kind  string `json:"kind"`
	Name  string `json:"name"`
	// Prefixes are the entry's addresses in the bridge's region: empty
	// while it has none there.
	Prefixes []string `json:"prefixes"`
}

// BridgeView is an external network bridge with its region and state.
type BridgeView struct {
	model.MeshExternalBridge
	NodeName   string            `json:"node_name"`
	RegionID   uint              `json:"region_id"`
	RegionCode string            `json:"region_code"`
	Routes     []string          `json:"routes"`
	Allow      []BridgeAllowView `json:"allow"`
	// Expanded is the allow list the node is given, and Digest its
	// digest (AllowDigest).
	Expanded []string `json:"expanded"`
	Digest   string   `json:"digest"`
	// AppliedDigest and AppliedAt are the digest of the allow list the
	// node last reported in force and since when (its status's
	// externalBridge), nil before it reported one; Confirmed: it equals
	// Digest.
	AppliedDigest *string    `json:"applied_digest"`
	AppliedAt     *time.Time `json:"applied_at"`
	Confirmed     bool       `json:"confirmed"`
	// Held are the addresses and segments that left the node's list and
	// are not given out until it reports a list without them.
	Held []string `json:"held"`
	// Warnings: WarningBridgeUnconfirmed, WarningBridgeUnsupported,
	// WarningAllowOutsideRegion.
	Warnings []string `json:"warnings"`
}

// parseRoute reads a route: a network, or a single address as /32 or
// /128.
func parseRoute(raw string) (netip.Prefix, error) {
	raw = strings.TrimSpace(raw)
	var prefix netip.Prefix
	if strings.Contains(raw, "/") {
		parsed, err := netip.ParsePrefix(raw)
		if err != nil {
			return prefix, invalid("routes: %q is not an address or a network", raw)
		}
		prefix = parsed
	} else {
		address, err := netip.ParseAddr(raw)
		if err != nil {
			return prefix, invalid("routes: %q is not an address or a network", raw)
		}
		prefix = netip.PrefixFrom(address, address.BitLen())
	}
	if prefix.Addr().Zone() != "" || prefix.Addr().Is4In6() {
		return prefix, invalid("routes: %q is not an address or a network", raw)
	}
	if prefix.Masked() != prefix {
		return prefix, invalid("routes: %s is not a network address (%s)", prefix, prefix.Masked())
	}
	return prefix, nil
}

// checkRoutes reads the routes of a bridge (spec section 15.12 item 1):
// inside the private ranges, never the default route; a subnet does not
// overlap 100.64.0.0/10 or fd7a:115c:a1e0::/48, where only single addresses
// go. It answers them without duplicates, in order.
func checkRoutes(raw []string) ([]netip.Prefix, error) {
	routes := []netip.Prefix{}
	seen := map[netip.Prefix]bool{}
	for _, entry := range raw {
		if strings.TrimSpace(entry) == "" {
			continue
		}
		prefix, err := parseRoute(entry)
		if err != nil {
			return nil, err
		}
		private := isPrivate(prefix)
		if prefix.Addr().Is6() {
			private = privateRangeV6.Bits() <= prefix.Bits() && privateRangeV6.Contains(prefix.Addr())
		}
		if !private {
			return nil, invalid("routes: %s is not inside the private ranges (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, 100.64.0.0/10, fc00::/7)", prefix)
		}
		if !prefix.IsSingleIP() && (prefix.Overlaps(SharedAddressSpace) || prefix.Overlaps(SharedAddressSpaceV6)) {
			return nil, invalid("routes: %s overlaps %s or %s, where only single addresses are advertised", prefix, SharedAddressSpace, SharedAddressSpaceV6)
		}
		if !seen[prefix] {
			seen[prefix] = true
			routes = append(routes, prefix)
		}
	}
	if len(routes) > maxBridgeRoutes {
		return nil, invalid("routes: at most %d", maxBridgeRoutes)
	}
	return routes, nil
}

// parseAllowEntry reads an allow list entry: node:<node ref>,
// device:<id> or host_group:<id>.
func parseAllowEntry(raw string) (kind, ref string, err error) {
	kind, ref, ok := strings.Cut(strings.TrimSpace(raw), ":")
	if !ok {
		return "", "", invalid("allow: %q is not node:<node>, device:<id> or host_group:<id>", raw)
	}
	switch kind {
	case AllowNode:
		node, err := agentcontrol.ParseAgentNode(ref)
		if err != nil {
			return "", "", invalid("allow: %q is not node:<node>, device:<id> or host_group:<id>", raw)
		}
		return kind, node.String(), nil
	case AllowDevice, AllowHostGroup:
		id, err := strconv.ParseUint(ref, 10, 32)
		if err != nil || id == 0 {
			return "", "", invalid("allow: %q is not node:<node>, device:<id> or host_group:<id>", raw)
		}
		return kind, strconv.FormatUint(id, 10), nil
	}
	return "", "", invalid("allow: %q is not node:<node>, device:<id> or host_group:<id>", raw)
}

// checkAllow reads the allow list of a bridge in region regionID: every
// entry names a member, device or host group of that region.
func checkAllow(tx *gorm.DB, regionID uint, raw []string) ([]string, error) {
	entries := []string{}
	seen := map[string]bool{}
	for _, item := range raw {
		if strings.TrimSpace(item) == "" {
			continue
		}
		kind, ref, err := parseAllowEntry(item)
		if err != nil {
			return nil, err
		}
		entry := kind + ":" + ref
		if seen[entry] {
			continue
		}
		seen[entry] = true
		entries = append(entries, entry)
	}
	if len(entries) > maxBridgeAllow {
		return nil, invalid("allow: at most %d", maxBridgeAllow)
	}
	expanded, err := expandEntries(tx, regionID, entries)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if _, ok := expanded.found[entry]; !ok {
			return nil, invalid("allow: %s is not in the bridge's region", entry)
		}
	}
	return entries, nil
}

// expansion is what allow list entries expand to: each entry found in the
// region with its prefixes (possibly none, as for a device without an
// address) and its name.
type expansion struct {
	found map[string][]netip.Prefix
	names map[string]string
}

// expandEntries expands allow list entries in region regionID: a node to
// its address there, a device of the region to its address while it holds
// one and a static key (a revoked device has none), a host group whose
// gateway's home is the region to its segment.
func expandEntries(tx *gorm.DB, regionID uint, entries []string) (expansion, error) {
	answer := expansion{found: map[string][]netip.Prefix{}, names: map[string]string{}}
	var nodes []string
	var devices, groups []uint
	for _, entry := range entries {
		kind, ref, err := parseAllowEntry(entry)
		if err != nil {
			continue
		}
		switch kind {
		case AllowNode:
			nodes = append(nodes, ref)
		case AllowDevice:
			id, _ := strconv.ParseUint(ref, 10, 32)
			devices = append(devices, uint(id))
		case AllowHostGroup:
			id, _ := strconv.ParseUint(ref, 10, 32)
			groups = append(groups, uint(id))
		}
	}
	if len(nodes) > 0 {
		var members []model.MeshMember
		if err := tx.Where("region_id = ? AND node_ref IN ?", regionID, nodes).Find(&members).Error; err != nil {
			return answer, err
		}
		for _, member := range members {
			entry := AllowNode + ":" + member.NodeRef
			answer.found[entry] = hostPrefixes(member.Address)
			info, err := lookupNode(tx, member.NodeRef)
			if err != nil && !errors.Is(err, ErrNotFound) {
				return answer, err
			}
			answer.names[entry] = info.name
		}
	}
	if len(devices) > 0 {
		var rows []model.MeshDevice
		if err := tx.Where("region_id = ? AND id IN ?", regionID, devices).Find(&rows).Error; err != nil {
			return answer, err
		}
		for _, row := range rows {
			entry := AllowDevice + ":" + strconv.FormatUint(uint64(row.ID), 10)
			answer.found[entry] = nil
			answer.names[entry] = row.Hostname
			if row.Address != nil && row.StaticKey != nil {
				answer.found[entry] = hostPrefixes(*row.Address)
			}
		}
	}
	if len(groups) > 0 {
		var rows []model.MeshHostGroup
		if err := tx.Where("id IN ?", groups).Find(&rows).Error; err != nil {
			return answer, err
		}
		for _, row := range rows {
			home, err := homeOf(tx, row.GatewayRef)
			if err != nil || home.RegionID != regionID {
				continue
			}
			entry := AllowHostGroup + ":" + strconv.FormatUint(uint64(row.ID), 10)
			answer.names[entry] = row.Name
			if segment, err := netip.ParsePrefix(row.Segment); err == nil {
				answer.found[entry] = []netip.Prefix{segment.Masked()}
			} else {
				answer.found[entry] = nil
			}
		}
	}
	return answer, nil
}

// hostPrefixes answers address as a host prefix, none when it is not one.
func hostPrefixes(raw string) []netip.Prefix {
	address, err := netip.ParseAddr(raw)
	if err != nil {
		return nil
	}
	return []netip.Prefix{netip.PrefixFrom(address, address.BitLen())}
}

// sortedPrefixes answers the prefixes in address order, without
// duplicates, as strings.
func sortedPrefixes(set map[netip.Prefix]bool) []string {
	list := make([]netip.Prefix, 0, len(set))
	for prefix := range set {
		list = append(list, prefix)
	}
	sort.Slice(list, func(i, j int) bool {
		if c := list[i].Addr().Compare(list[j].Addr()); c != 0 {
			return c < 0
		}
		return list[i].Bits() < list[j].Bits()
	})
	answer := make([]string, len(list))
	for i, prefix := range list {
		answer[i] = prefix.String()
	}
	return answer
}

// bridgeKey is a bridge node in a region.
type bridgeKey struct {
	node   string
	region uint
}

// bridgeHome answers the bridge's region: its node's home region, 0 when
// the node is not in the mesh.
func bridgeHome(tx *gorm.DB, bridge model.MeshExternalBridge) (uint, error) {
	var rows []model.MeshMember
	if err := tx.Where("node_ref = ? AND home = ?", bridge.NodeRef, true).Limit(1).Find(&rows).Error; err != nil || len(rows) == 0 {
		return 0, err
	}
	return rows[0].RegionID, nil
}

// expandBridge answers the allow list the bridge's node is given in its
// region, as a set.
func expandBridge(tx *gorm.DB, bridge model.MeshExternalBridge, regionID uint) (map[netip.Prefix]bool, expansion, error) {
	entries := splitSubnets(bridge.Allow)
	expanded, err := expandEntries(tx, regionID, entries)
	if err != nil {
		return nil, expanded, err
	}
	set := map[netip.Prefix]bool{}
	for _, entry := range entries {
		for _, prefix := range expanded.found[entry] {
			set[prefix] = true
		}
	}
	return set, expanded, nil
}

// desiredAllows answers the allow list each bridge node is given, by node
// and region.
func desiredAllows(tx *gorm.DB) (map[bridgeKey]map[netip.Prefix]bool, error) {
	answer := map[bridgeKey]map[netip.Prefix]bool{}
	var bridges []model.MeshExternalBridge
	if err := tx.Find(&bridges).Error; err != nil {
		return nil, err
	}
	for _, bridge := range bridges {
		region, err := bridgeHome(tx, bridge)
		if err != nil {
			return nil, err
		}
		if region == 0 {
			continue
		}
		set, _, err := expandBridge(tx, bridge, region)
		if err != nil {
			return nil, err
		}
		answer[bridgeKey{bridge.NodeRef, region}] = set
	}
	return answer, nil
}

// bridgeLock is the PostgreSQL advisory lock key of the bridge allow lists
// ("mesh_br" in ASCII): every change takes it first, so that what an allow
// list held before a change (withBridgeHolds) is not changed by another
// change at the same time.
const bridgeLock int64 = 0x6d6573685f6272

// withBridgeHolds runs write and then holds every address and segment
// that left a bridge node's allow list, for a node still in that region.
func withBridgeHolds(tx *gorm.DB, write func(tx *gorm.DB) error) error {
	if tx.Name() == "postgres" {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", bridgeLock).Error; err != nil {
			return err
		}
	}
	before, err := desiredAllows(tx)
	if err != nil {
		return err
	}
	if err := write(tx); err != nil {
		return err
	}
	// A node that left a region leaves its network, and its static key
	// the region's native_keys: what was held for it there is released.
	if err := tx.Where("NOT EXISTS (?)", tx.Session(&gorm.Session{NewDB: true}).Model(&model.MeshMember{}).Select("1").
		Where("v5_mesh_member.node_ref = v5_mesh_bridge_hold.node_ref AND v5_mesh_member.region_id = v5_mesh_bridge_hold.region_id")).
		Delete(&model.MeshBridgeHold{}).Error; err != nil {
		return err
	}
	if len(before) == 0 {
		return nil
	}
	after, err := desiredAllows(tx)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	holds := []model.MeshBridgeHold{}
	for key, set := range before {
		var count int64
		if err := tx.Model(&model.MeshMember{}).Where("node_ref = ? AND region_id = ?", key.node, key.region).Count(&count).Error; err != nil {
			return err
		}
		// A node that left the region leaves its network, and its
		// static key the region's native_keys.
		if count == 0 {
			continue
		}
		for prefix := range set {
			if !after[key][prefix] {
				holds = append(holds, model.MeshBridgeHold{NodeRef: key.node, RegionID: key.region, Prefix: prefix.String(), CreatedAt: now})
			}
		}
	}
	if len(holds) == 0 {
		return nil
	}
	// A prefix removed again starts its wait again: a report received
	// before this removal confirms nothing about it.
	return tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "node_ref"}, {Name: "region_id"}, {Name: "prefix"}},
		DoUpdates: clause.AssignmentColumns([]string{"created_at"}),
	}).Create(&holds).Error
}

// heldPrefixes answers the prefixes held back in region regionID, or in
// every region for 0. A caller reads them after the addresses in use, so
// that an address freed and held by one commit is seen as either.
func heldPrefixes(tx *gorm.DB, regionID uint) ([]netip.Prefix, error) {
	query := tx.Model(&model.MeshBridgeHold{})
	if regionID != 0 {
		query = query.Where("region_id = ?", regionID)
	}
	var raw []string
	if err := query.Pluck("prefix", &raw).Error; err != nil {
		return nil, err
	}
	answer := make([]netip.Prefix, 0, len(raw))
	for _, entry := range raw {
		if prefix, err := netip.ParsePrefix(entry); err == nil {
			answer = append(answer, prefix)
		}
	}
	return answer, nil
}

// heldAddresses answers the single addresses held back in region regionID.
func heldAddresses(tx *gorm.DB, regionID uint) (map[string]bool, error) {
	prefixes, err := heldPrefixes(tx, regionID)
	if err != nil {
		return nil, err
	}
	answer := map[string]bool{}
	for _, prefix := range prefixes {
		if prefix.IsSingleIP() {
			answer[prefix.Addr().String()] = true
		}
	}
	return answer, nil
}

// bridgedAddresses answers the single addresses the allow lists of the
// bridges in region regionID hold now.
func bridgedAddresses(tx *gorm.DB, regionID uint) (map[string]bool, error) {
	allows, err := desiredAllows(tx)
	if err != nil {
		return nil, err
	}
	answer := map[string]bool{}
	for key, set := range allows {
		if key.region != regionID {
			continue
		}
		for prefix := range set {
			if prefix.IsSingleIP() {
				answer[prefix.Addr().String()] = true
			}
		}
	}
	return answer, nil
}

// releaseFirst is the answer of allocate when the only address it could
// reclaim is in a bridge node's allow list: the change is undone, and the
// address is taken from its device in a change of its own, after which
// the bridge nodes are given lists without it; it is given out once they
// confirm (spec section 15.12 item 4).
type releaseFirst struct {
	region   string
	deviceID uint
	address  string
}

func (r *releaseFirst) Error() string {
	return fmt.Sprintf("%s: %s", ErrPoolFull, r.region)
}

func (r *releaseFirst) Unwrap() error { return ErrPoolFull }

// release takes the address from its device, if it still holds it, as a
// change of its own (withBridgeHolds holds it), and answers the error the
// request that needed an address ends with.
func (r *releaseFirst) release(ctx context.Context, db *gorm.DB) error {
	err := change(ctx, db, func(tx *gorm.DB) error {
		return tx.Model(&model.MeshDevice{}).Where("id = ? AND address = ?", r.deviceID, r.address).
			Updates(map[string]any{"address": nil, "address_used_at": nil, "updated_at": time.Now().UTC()}).Error
	})
	if err != nil {
		return err
	}
	return fmt.Errorf("%w: %s; the address %s unused longest is released and is given out once the external network bridges confirm it", ErrPoolFull, r.region, r.address)
}

// AllowDigest is the digest of an allow list both sides compute (spec
// mesh-status.md, externalBridge): each entry as CIDR text (netip's form:
// dotted IPv4, RFC 5952 IPv6, always with the prefix length), without
// duplicates, sorted bytewise, joined by "\n" (none after the last), SHA-256
// in lower-case hex.
func AllowDigest(set map[netip.Prefix]bool) string {
	list := make([]string, 0, len(set))
	for prefix := range set {
		list = append(list, prefix.Masked().String())
	}
	sort.Strings(list)
	sum := sha256.Sum256([]byte(strings.Join(list, "\n")))
	return hex.EncodeToString(sum[:])
}

// appliedBridge is what a bridge node reports in force in one network.
type appliedBridge struct {
	AllowDigest string    `json:"allowDigest"`
	AllowCount  int       `json:"allowCount"`
	AppliedAt   time.Time `json:"appliedAt"`
}

// AlertBridgeUnsupported is the alert the Agent reports in a network whose
// external_bridge section it withholds from a kernel without
// mesh_external_bridge.
const AlertBridgeUnsupported = "external_bridge_unsupported"

// networkAlert tells whether a "mesh.status" payload reports the alert
// code in the network id.
func networkAlert(payload, id, code string) bool {
	var shape struct {
		Networks []struct {
			ID     string `json:"id"`
			Alerts []struct {
				Code string `json:"code"`
			} `json:"alerts"`
		} `json:"networks"`
	}
	if json.Unmarshal([]byte(payload), &shape) != nil {
		return false
	}
	for _, network := range shape.Networks {
		if network.ID != id {
			continue
		}
		for _, alert := range network.Alerts {
			if alert.Code == code {
				return true
			}
		}
	}
	return false
}

// reportedBridge is what a "mesh.status" payload says of the bridge in one
// network: Running with the configuration in force (Applied), Stopped for
// an explicit null externalBridge or, without one, the alert
// external_bridge_unsupported (the Agent never gives that kernel the
// section, so the node runs no bridge there), or
// neither when the key is missing or unreadable (the state is unknown: the
// configuration may not have been applied or the kernel not read).
type reportedBridge struct {
	Running bool
	Stopped bool
	Applied appliedBridge
}

// reportedBridges answers, by network id (region code), the bridge state a
// "mesh.status" payload reports: the network's externalBridge as the
// kernel status has it (the Agent passes it on as it is, and null only
// when it knows the kernel runs no bridge there).
func reportedBridges(payload string) map[string]reportedBridge {
	var shape struct {
		Networks []struct {
			ID             string          `json:"id"`
			ExternalBridge json.RawMessage `json:"externalBridge"`
			Alerts         []struct {
				Code string `json:"code"`
			} `json:"alerts"`
		} `json:"networks"`
	}
	answer := map[string]reportedBridge{}
	if json.Unmarshal([]byte(payload), &shape) != nil {
		return answer
	}
	for _, network := range shape.Networks {
		raw := bytes.TrimSpace(network.ExternalBridge)
		unsupported := false
		for _, alert := range network.Alerts {
			unsupported = unsupported || alert.Code == AlertBridgeUnsupported
		}
		switch {
		case len(raw) == 0 && unsupported:
			answer[network.ID] = reportedBridge{Stopped: true}
		case len(raw) == 0:
			// Missing: unknown.
		case bytes.Equal(raw, []byte("null")):
			answer[network.ID] = reportedBridge{Stopped: true}
		default:
			var applied appliedBridge
			if json.Unmarshal(raw, &applied) == nil && applied.AllowDigest != "" && !applied.AppliedAt.IsZero() {
				answer[network.ID] = reportedBridge{Running: true, Applied: applied}
			}
		}
	}
	return answer
}

// desiredOf answers the allow list nodeRef is given in region regionID,
// and whether it bridges there at all.
func desiredOf(db *gorm.DB, nodeRef string, regionID uint) (map[netip.Prefix]bool, bool, error) {
	var rows []model.MeshExternalBridge
	if err := db.Where("node_ref = ?", nodeRef).Limit(1).Find(&rows).Error; err != nil || len(rows) == 0 {
		return nil, false, err
	}
	home, err := bridgeHome(db, rows[0])
	if err != nil || home != regionID {
		return nil, false, err
	}
	set, _, err := expandBridge(db, rows[0], regionID)
	return set, err == nil, err
}

// releaseHolds deletes the holds of nodeRef its report, received at
// receivedAt, shows released (spec section 15.12 item 4): where the node
// bridges, a report received after the hold whose digest of the list in
// force equals the digest of the list it is given now, which no longer has
// the prefix; where it runs no bridge, a report received after the hold
// without a bridge in that network (an explicit null, or the alert
// external_bridge_unsupported without externalBridge; a missing
// externalBridge is unknown and releases nothing). Both times are the
// panel's: the node's appliedAt, by another clock, is only shown.
func releaseHolds(db *gorm.DB, nodeRef, payload string, receivedAt time.Time) error {
	var holds []model.MeshBridgeHold
	if err := db.Where("node_ref = ?", nodeRef).Find(&holds).Error; err != nil || len(holds) == 0 {
		return err
	}
	codes, err := regionCodes(db)
	if err != nil {
		return err
	}
	reported := reportedBridges(payload)
	type desired struct {
		set    map[netip.Prefix]bool
		bridge bool
	}
	byRegion := map[uint]desired{}
	released := []uint{}
	for _, hold := range holds {
		prefix, err := netip.ParsePrefix(hold.Prefix)
		if err != nil {
			continue
		}
		want, done := byRegion[hold.RegionID]
		if !done {
			set, bridge, err := desiredOf(db, nodeRef, hold.RegionID)
			if err != nil {
				return err
			}
			want = desired{set, bridge}
			byRegion[hold.RegionID] = want
		}
		report := reported[codes[hold.RegionID]]
		switch {
		case want.bridge && report.Running:
			if receivedAt.After(hold.CreatedAt) && !want.set[prefix] && report.Applied.AllowDigest == AllowDigest(want.set) {
				released = append(released, hold.ID)
			}
		case report.Stopped:
			// The node runs no bridge in the network: nothing it held
			// back is reachable through it, whether or not it is still
			// meant to bridge.
			if receivedAt.After(hold.CreatedAt) {
				released = append(released, hold.ID)
			}
		}
	}
	if len(released) == 0 {
		return nil
	}
	return db.Where("id IN ?", released).Delete(&model.MeshBridgeHold{}).Error
}

// bridgeSection answers the section external_bridge of the network of
// region regionID for nodeRef, nil when the node is not a bridge there.
func bridgeSection(db *gorm.DB, nodeRef string, regionID uint) (map[string]any, error) {
	var rows []model.MeshExternalBridge
	if err := db.Where("node_ref = ?", nodeRef).Limit(1).Find(&rows).Error; err != nil || len(rows) == 0 {
		return nil, err
	}
	home, err := bridgeHome(db, rows[0])
	if err != nil || home != regionID {
		return nil, err
	}
	set, _, err := expandBridge(db, rows[0], regionID)
	if err != nil {
		return nil, err
	}
	routes := splitSubnets(rows[0].Routes)
	return map[string]any{"interface": rows[0].Interface, "routes": routes, "allow": sortedPrefixes(set)}, nil
}

// checkBridgeRoutes refuses ranges of our own (a region's range, the host
// group supernet, a full node's subnets) that overlap a bridge's routes.
func checkBridgeRoutes(tx *gorm.DB, field string, ranges ...netip.Prefix) error {
	var rows []model.MeshExternalBridge
	if err := tx.Where("routes <> ''").Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		for _, raw := range splitSubnets(row.Routes) {
			route, err := netip.ParsePrefix(raw)
			if err != nil {
				continue
			}
			for _, prefix := range ranges {
				if prefix.Overlaps(route) {
					return conflict("%s: %s overlaps the external route %s of a bridge", field, prefix, raw)
				}
			}
		}
	}
	return nil
}

// checkRoutesOutside refuses routes that overlap our own ranges: the
// regions' ranges, the host group supernet and the full nodes' subnets.
func checkRoutesOutside(tx *gorm.DB, routes []netip.Prefix) error {
	var regions []model.MeshRegion
	if err := tx.Find(&regions).Error; err != nil {
		return err
	}
	settings, err := loadSettings(tx)
	if err != nil {
		return err
	}
	ours := []string{settings.HostGroupSupernet}
	for _, region := range regions {
		ours = append(ours, region.CIDR)
	}
	var fullNodes []model.MeshFullNode
	if err := tx.Where("subnets <> ''").Find(&fullNodes).Error; err != nil {
		return err
	}
	for _, row := range fullNodes {
		ours = append(ours, splitSubnets(row.Subnets)...)
	}
	for _, route := range routes {
		for _, raw := range ours {
			if other, err := netip.ParsePrefix(raw); err == nil && other.Overlaps(route) {
				return conflict("routes: %s overlaps %s of the mesh", route, raw)
			}
		}
	}
	return nil
}

// saveBridge checks input and writes the bridge row (id 0 creates it).
func saveBridge(tx *gorm.DB, id uint, input BridgeInput, now time.Time) (model.MeshExternalBridge, error) {
	var row model.MeshExternalBridge
	iface := strings.TrimSpace(input.Interface)
	if !interfacePattern.MatchString(iface) || iface == "." || iface == ".." {
		return row, invalid("interface: a network interface name of 1 to 15 letters, digits, '_', '.', '@' or '-'")
	}
	routes, err := checkRoutes(input.Routes)
	if err != nil {
		return row, err
	}
	if len(routes) == 0 {
		return row, invalid("routes: at least one external address or subnet")
	}
	if err := lockAddressPlan(tx); err != nil {
		return row, err
	}
	if id == 0 {
		node, err := agentcontrol.ParseAgentNode(strings.TrimSpace(input.Node))
		if err != nil {
			return row, invalid("node: proxy-<id> or forward-<id>")
		}
		row = model.MeshExternalBridge{NodeRef: node.String(), CreatedAt: now}
		var count int64
		if err := tx.Model(&model.MeshExternalBridge{}).Where("node_ref = ?", row.NodeRef).Count(&count).Error; err != nil {
			return row, err
		}
		if count > 0 {
			return row, conflict("the node is a bridge already")
		}
	} else {
		var rows []model.MeshExternalBridge
		if err := forUpdate(tx).Where("id = ?", id).Limit(1).Find(&rows).Error; err != nil {
			return row, err
		}
		if len(rows) == 0 {
			return row, fmt.Errorf("%w: bridge %d", ErrNotFound, id)
		}
		row = rows[0]
	}
	// Locked, as removing a node locks it: a node leaving at the same
	// time is not made a bridge.
	home, err := homeOf(forUpdate(tx), row.NodeRef)
	if err != nil {
		return row, err
	}
	if err := checkRoutesOutside(tx, routes); err != nil {
		return row, err
	}
	allow, err := checkAllow(tx, home.RegionID, input.Allow)
	if err != nil {
		return row, err
	}
	list := make([]string, len(routes))
	for i, route := range routes {
		list[i] = route.String()
	}
	row.Interface, row.Routes, row.Allow, row.UpdatedAt = iface, strings.Join(list, ","), strings.Join(allow, ","), now
	if id == 0 {
		return row, tx.Create(&row).Error
	}
	return row, saveExisting(tx, &row, "bridge")
}

// CreateBridge makes a node of the mesh an external network bridge of its
// home region.
func CreateBridge(ctx context.Context, db *gorm.DB, input BridgeInput, now time.Time) (model.MeshExternalBridge, error) {
	var row model.MeshExternalBridge
	err := change(ctx, db, func(tx *gorm.DB) error {
		var err error
		row, err = saveBridge(tx, 0, input, now)
		return err
	})
	return row, err
}

// UpdateBridge changes a bridge's interface, routes and allow list; its
// node never changes.
func UpdateBridge(ctx context.Context, db *gorm.DB, id uint, input BridgeInput, now time.Time) (model.MeshExternalBridge, error) {
	var row model.MeshExternalBridge
	err := change(ctx, db, func(tx *gorm.DB) error {
		var err error
		row, err = saveBridge(tx, id, input, now)
		return err
	})
	return row, err
}

// DeleteBridge makes the node an ordinary member again. The addresses of
// its allow list stay held until it reports the bridge gone.
func DeleteBridge(ctx context.Context, db *gorm.DB, id uint) error {
	return change(ctx, db, func(tx *gorm.DB) error {
		result := tx.Where("id = ?", id).Delete(&model.MeshExternalBridge{})
		if result.Error == nil && result.RowsAffected == 0 {
			return fmt.Errorf("%w: bridge %d", ErrNotFound, id)
		}
		return result.Error
	})
}

// isBridge tells whether nodeRef is an external network bridge.
func isBridge(tx *gorm.DB, nodeRef string) (bool, error) {
	var count int64
	err := tx.Model(&model.MeshExternalBridge{}).Where("node_ref = ?", nodeRef).Count(&count).Error
	return count > 0, err
}

// Bridges answers every external network bridge with its allow list, the
// list its node last reported applied and the prefixes held back for it.
func Bridges(ctx context.Context, db *gorm.DB) ([]BridgeView, error) {
	db = db.WithContext(ctx)
	var rows []model.MeshExternalBridge
	if err := db.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	codes, err := regionCodes(db)
	if err != nil {
		return nil, err
	}
	answer := make([]BridgeView, 0, len(rows))
	for _, row := range rows {
		view := BridgeView{MeshExternalBridge: row, Routes: splitSubnets(row.Routes), Allow: []BridgeAllowView{}, Expanded: []string{}, Held: []string{}, Warnings: []string{}}
		info, err := lookupNode(db, row.NodeRef)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, err
		}
		view.NodeName = info.name
		region, err := bridgeHome(db, row)
		if err != nil {
			return nil, err
		}
		view.RegionID, view.RegionCode = region, codes[region]
		set, expanded, err := expandBridge(db, row, region)
		if err != nil {
			return nil, err
		}
		view.Expanded = sortedPrefixes(set)
		outside := false
		for _, entry := range splitSubnets(row.Allow) {
			kind, _, _ := strings.Cut(entry, ":")
			prefixes, found := expanded.found[entry]
			item := BridgeAllowView{Entry: entry, Kind: kind, Name: expanded.names[entry], Prefixes: []string{}}
			for _, prefix := range prefixes {
				item.Prefixes = append(item.Prefixes, prefix.String())
			}
			outside = outside || !found
			view.Allow = append(view.Allow, item)
		}
		var holds []model.MeshBridgeHold
		if err := db.Where("node_ref = ?", row.NodeRef).Order("prefix ASC").Find(&holds).Error; err != nil {
			return nil, err
		}
		for _, hold := range holds {
			view.Held = append(view.Held, hold.Prefix)
		}
		var statuses []model.MeshStatus
		if err := db.Where("node_ref = ?", row.NodeRef).Limit(1).Find(&statuses).Error; err != nil {
			return nil, err
		}
		view.Digest = AllowDigest(set)
		if len(statuses) > 0 {
			if report := reportedBridges(statuses[0].Payload)[view.RegionCode]; report.Running {
				applied := report.Applied
				view.AppliedDigest, view.AppliedAt = &applied.AllowDigest, &applied.AppliedAt
				view.Confirmed = applied.AllowDigest == view.Digest
			}
			// The Agent withholds the section from a kernel without the
			// capability and reports the alert; a report's features say
			// so too.
			if features := reportedFeatures(statuses[0].Payload); (len(features) > 0 && !features[FeatureExternalBridge]) || networkAlert(statuses[0].Payload, view.RegionCode, AlertBridgeUnsupported) {
				view.Warnings = append(view.Warnings, WarningBridgeUnsupported)
			}
		}
		if !view.Confirmed || len(view.Held) > 0 {
			view.Warnings = append(view.Warnings, WarningBridgeUnconfirmed)
		}
		if outside {
			view.Warnings = append(view.Warnings, WarningAllowOutsideRegion)
		}
		answer = append(answer, view)
	}
	return answer, nil
}
