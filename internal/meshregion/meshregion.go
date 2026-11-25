// Package meshregion keeps the regions of the intranet mesh (组网;
// docs/design/mesh-regions): each region is a network of its own with a
// virtual IPv4 range that no other region overlaps. Its lower half is the
// server segment, from which Control gives servers and boundary nodes a
// fixed address; the upper half is the device pool (allocated to devices
// by a later step).
//
// A node has one home region. A boundary node joins every region: it has a
// fixed address in each, the other boundary nodes of a region are the
// region's initial peers, and in each region it advertises the ranges of the
// other regions, so traffic between regions goes through it.
//
// The mesh runs in the node's kernel; Control only plans it. The member
// "mesh" of a node's desired configuration (NodeMember) lists the networks
// of the node's enabled regions, and the node's Agent reports their status
// as the package report "mesh.status" (RecordStatus). The contract with the
// Agent is docs/design/mesh-regions/agent-contract.md. The whole feature
// is the gated feature "mesh" (internal/features): off in releases until the
// kernel runs the mesh.
package meshregion

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/features"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Feature is the gated feature of the mesh.
const Feature = "mesh"

// The desired configuration member and the status report.
const (
	Member         = "mesh"
	StatusPluginID = "mesh"
	StatusKind     = "mesh.status"
	StatusVersion  = "v1"
)

// Network settings Control fixes for every region network.
const (
	// Port is the TCP and UDP port the nodes listen on.
	Port   = 11010
	cipher = "aes-gcm"
	policy = "latency"
)

var (
	ErrNotFound = errors.New("not found")
	ErrInvalid  = errors.New("invalid mesh request")
	// ErrConflict: the change collides with another region or member.
	ErrConflict = errors.New("mesh conflict")
	// ErrFull: the region's server segment has no free address.
	ErrFull = errors.New("the region's server segment is full")
	// ErrNotMember: a report of a node that is in no region.
	ErrNotMember = errors.New("the node is not in the mesh")
)

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

func conflict(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrConflict, fmt.Sprintf(format, args...))
}

var codePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,14}[a-z0-9]$`)

// privateRanges are the ranges a region's range must fall in.
var privateRanges = []netip.Prefix{
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("100.64.0.0/10"),
}

// RegionInput creates or changes a region.
type RegionInput struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	CIDR    string `json:"cidr"`
	Enabled bool   `json:"enabled"`
	Sort    int    `json:"sort"`
}

// Region is a region with its members, for the administrator.
type Region struct {
	model.MeshRegion
	Members []MemberView `json:"members"`
	// ResolverAddress is the region's fixed resolver address, which no
	// node is given (ResolverAddress).
	ResolverAddress string `json:"resolver_address"`
	// Warnings: WarningFewBoundaryNodes when the region is enabled with
	// fewer than two boundary nodes (R19); WarningSharedAddressSpace when
	// its range lies in 100.64.0.0/10; WarningResolverAddressHeld while a
	// device holds its resolver address, which is then not delivered.
	Warnings []string `json:"warnings"`
}

// WarningFewBoundaryNodes: an enabled region has fewer than two boundary
// nodes, so a failing one cuts it off from the other regions.
const WarningFewBoundaryNodes = "few_boundary_nodes"

// WarningResolverAddressHeld: a device holds the region's resolver
// address from before it was reserved, so the nodes are not given it
// (dns.resolver_address) until the device takes another address.
const WarningResolverAddressHeld = "resolver_address_held"

// MemberView is a membership with its node and the node's last status.
type MemberView struct {
	model.MeshMember
	NodeName string `json:"node_name"`
	NodeHost string `json:"node_host"`
	// Status is the node's last report as the Agent sent it, nil before
	// the first.
	Status           json.RawMessage `json:"status" swaggertype:"object"`
	StatusObservedAt *time.Time      `json:"status_observed_at"`
	StatusReceivedAt *time.Time      `json:"status_received_at"`
	// ExternalBridge: the node is an external network bridge of its home
	// region (R36).
	ExternalBridge bool `json:"external_bridge"`
	// Warnings: WarningStaticKeyTaken when the node's static key in
	// this region is a device's or another node's.
	Warnings []string `json:"warnings"`
}

// List answers every region in order with its members.
func List(ctx context.Context, db *gorm.DB) ([]Region, error) {
	db = db.WithContext(ctx)
	var regions []model.MeshRegion
	if err := db.Order("sort ASC, id ASC").Find(&regions).Error; err != nil {
		return nil, err
	}
	var members []model.MeshMember
	if err := db.Order("home DESC, address ASC").Find(&members).Error; err != nil {
		return nil, err
	}
	var statuses []model.MeshStatus
	if err := db.Find(&statuses).Error; err != nil {
		return nil, err
	}
	byNode := map[string]model.MeshStatus{}
	for _, status := range statuses {
		byNode[status.NodeRef] = status
	}
	infos := map[string]nodeInfo{}
	for _, member := range members {
		if _, done := infos[member.NodeRef]; done {
			continue
		}
		info, err := lookupNode(db, member.NodeRef)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, err
		}
		infos[member.NodeRef] = info
	}
	payloads := make(map[string]string, len(statuses))
	for _, status := range statuses {
		payloads[status.NodeRef] = status.Payload
	}
	codes := make(map[uint]string, len(regions))
	for _, region := range regions {
		codes[region.ID] = region.Code
	}
	keyWarnings, err := memberKeyWarnings(db, members, payloads, codes)
	if err != nil {
		return nil, err
	}
	resolvers, err := resolverAddresses(db, regions)
	if err != nil {
		return nil, err
	}
	var bridgeRefs []string
	if err := db.Model(&model.MeshExternalBridge{}).Pluck("node_ref", &bridgeRefs).Error; err != nil {
		return nil, err
	}
	bridges := make(map[string]bool, len(bridgeRefs))
	for _, ref := range bridgeRefs {
		bridges[ref] = true
	}
	answer := make([]Region, len(regions))
	index := map[uint]int{}
	for i, region := range regions {
		answer[i] = Region{MeshRegion: region, Members: []MemberView{}, Warnings: []string{}}
		if prefix, err := netip.ParsePrefix(region.CIDR); err == nil {
			answer[i].ResolverAddress = ResolverAddress(prefix).String()
		}
		if inSharedSpace(region.CIDR) {
			answer[i].Warnings = append(answer[i].Warnings, WarningSharedAddressSpace)
		}
		if answer[i].ResolverAddress != "" && resolvers[region.ID] == "" {
			answer[i].Warnings = append(answer[i].Warnings, WarningResolverAddressHeld)
		}
		index[region.ID] = i
	}
	for _, member := range members {
		i, ok := index[member.RegionID]
		if !ok {
			continue
		}
		view := MemberView{MeshMember: member, NodeName: infos[member.NodeRef].name, NodeHost: infos[member.NodeRef].host, ExternalBridge: member.Home && bridges[member.NodeRef], Warnings: []string{}}
		if warnings, ok := keyWarnings[member.ID]; ok {
			view.Warnings = warnings
		}
		if status, ok := byNode[member.NodeRef]; ok {
			view.Status = json.RawMessage(status.Payload)
			view.StatusObservedAt, view.StatusReceivedAt = &status.ObservedAt, &status.ReceivedAt
		}
		answer[i].Members = append(answer[i].Members, view)
	}
	for i := range answer {
		boundaries := 0
		for _, member := range answer[i].Members {
			if !member.Home || member.Boundary {
				boundaries++
			}
		}
		if answer[i].Enabled && boundaries < 2 {
			answer[i].Warnings = append(answer[i].Warnings, WarningFewBoundaryNodes)
		}
	}
	return answer, nil
}

// normalize checks input and answers its canonical range.
func normalize(input *RegionInput) (netip.Prefix, error) {
	if !codePattern.MatchString(input.Code) {
		return netip.Prefix{}, invalid("code: 2 to 16 lowercase letters, digits or dashes, starting with a letter and ending with a letter or digit")
	}
	if n := utf8.RuneCountInString(input.Name); n == 0 || n > 32 {
		return netip.Prefix{}, invalid("name: 1 to 32 characters")
	}
	prefix, err := netip.ParsePrefix(input.CIDR)
	if err != nil || !prefix.Addr().Is4() || prefix.Masked() != prefix {
		return netip.Prefix{}, invalid("cidr: an IPv4 network such as 10.150.0.0/16")
	}
	if prefix.Bits() < 8 || prefix.Bits() > 24 {
		return netip.Prefix{}, invalid("cidr: a prefix length of 8 to 24")
	}
	inside := false
	for _, private := range privateRanges {
		inside = inside || (private.Bits() <= prefix.Bits() && private.Contains(prefix.Addr()))
	}
	if !inside {
		return netip.Prefix{}, invalid("cidr: inside 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16 or 100.64.0.0/10")
	}
	input.CIDR = prefix.String()
	return prefix, nil
}

// checkRegion refuses a code or range another region (not id) has, and a
// prefix length other than the other regions': every region network has
// the same one, or nodes of one region drop traffic for another.
func checkRegion(tx *gorm.DB, id uint, input RegionInput, prefix netip.Prefix) error {
	var others []model.MeshRegion
	if err := tx.Where("id <> ?", id).Find(&others).Error; err != nil {
		return err
	}
	for _, other := range others {
		if other.Code == input.Code {
			return conflict("another region has the code %q", input.Code)
		}
		otherPrefix, err := netip.ParsePrefix(other.CIDR)
		if err != nil {
			continue
		}
		if otherPrefix.Overlaps(prefix) {
			return conflict("the range overlaps region %q (%s)", other.Code, other.CIDR)
		}
		if otherPrefix.Bits() != prefix.Bits() {
			return conflict("every region has the same prefix length: /%d like region %q", otherPrefix.Bits(), other.Code)
		}
	}
	if err := checkOutsideSettings(tx, prefix); err != nil {
		return err
	}
	if err := checkBridgeRoutes(tx, "cidr", prefix); err != nil {
		return err
	}
	return checkFullNodeSubnets(tx, "cidr", prefix)
}

// CreateRegion creates a region; every boundary node gets an address in it.
func CreateRegion(ctx context.Context, db *gorm.DB, input RegionInput, now time.Time) (model.MeshRegion, error) {
	prefix, err := normalize(&input)
	if err != nil {
		return model.MeshRegion{}, err
	}
	row := model.MeshRegion{Code: input.Code, Name: input.Name, CIDR: input.CIDR, Enabled: input.Enabled, Sort: input.Sort, CreatedAt: now, UpdatedAt: now}
	err = change(ctx, db, func(tx *gorm.DB) error {
		if err := lockAddressPlan(tx); err != nil {
			return err
		}
		if err := checkRegion(tx, 0, input, prefix); err != nil {
			return err
		}
		row.Secret = rand.Text()
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		var boundaries []model.MeshMember
		if err := tx.Where("home = ? AND boundary = ?", true, true).Order("id ASC").Find(&boundaries).Error; err != nil {
			return err
		}
		for _, boundary := range boundaries {
			if _, err := join(tx, row, boundary.NodeRef, false, false, now); err != nil {
				return err
			}
		}
		return nil
	})
	return row, err
}

// UpdateRegion changes a region. Its range changes only while it has no
// members; enabling it gives it a network key when it has none.
func UpdateRegion(ctx context.Context, db *gorm.DB, id uint, input RegionInput, now time.Time) (model.MeshRegion, error) {
	prefix, err := normalize(&input)
	if err != nil {
		return model.MeshRegion{}, err
	}
	var row model.MeshRegion
	err = change(ctx, db, func(tx *gorm.DB) error {
		if err := lockAddressPlan(tx); err != nil {
			return err
		}
		// Locked first, so that no member or device joins the region
		// between the checks below and the change.
		if err := findLockedRegion(tx, id, &row); err != nil {
			return err
		}
		if err := checkRegion(tx, id, input, prefix); err != nil {
			return err
		}
		// The code names the network and every name in the region.
		if input.CIDR != row.CIDR || input.Code != row.Code {
			var count int64
			if err := tx.Model(&model.MeshMember{}).Where("region_id = ?", id).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return conflict("the range and the code of a region with members cannot change")
			}
			if err := regionHasDevices(tx, id); err != nil {
				return err
			}
		}
		if input.CIDR != row.CIDR {
			// A full node credential not yet used takes the lowest free
			// address of the new range.
			if err := tx.Model(&model.MeshFullNode{}).Where("device_id IS NULL AND enrollment_id IN (?)", unusedEnrollments(tx, id)).
				Updates(map[string]any{"address": "", "updated_at": now}).Error; err != nil {
				return err
			}
		}
		if row.Secret == "" {
			row.Secret = rand.Text()
		}
		row.Code, row.Name, row.CIDR, row.Enabled, row.Sort, row.UpdatedAt = input.Code, input.Name, input.CIDR, input.Enabled, input.Sort, now
		return saveExisting(tx, &row, "region")
	})
	return row, err
}

// RotateRegionSecret gives a region a new network key, so that a device
// whose token was revoked or that was deleted no longer joins with the key
// it kept. Every member gets the new key at once; devices read it with
// their next configuration.
func RotateRegionSecret(ctx context.Context, db *gorm.DB, id uint, now time.Time) error {
	return change(ctx, db, func(tx *gorm.DB) error {
		var row model.MeshRegion
		if err := findLockedRegion(tx, id, &row); err != nil {
			return err
		}
		return tx.Model(&row).Updates(map[string]any{"secret": rand.Text(), "updated_at": now}).Error
	})
}

// DeleteRegion deletes a region that is no node's home region; the boundary
// nodes leave it.
func DeleteRegion(ctx context.Context, db *gorm.DB, id uint) error {
	return change(ctx, db, func(tx *gorm.DB) error {
		// Locked first, so that no member or device joins the region
		// between the checks below and the delete.
		var row model.MeshRegion
		if err := findLockedRegion(tx, id, &row); err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&model.MeshMember{}).Where("region_id = ? AND home = ?", id, true).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return conflict("the region is the home region of %d nodes; move or remove them first", count)
		}
		if err := regionHasDevices(tx, id); err != nil {
			return err
		}
		if err := regionHasPendingChanges(tx, id); err != nil {
			return err
		}
		if err := tx.Where("region_id = ?", id).Delete(&model.MeshMember{}).Error; err != nil {
			return err
		}
		// Unused credentials for the region can no longer be used.
		if err := tx.Where("device_id IS NULL AND enrollment_id IN (?)", unusedEnrollments(tx, id)).Delete(&model.MeshFullNode{}).Error; err != nil {
			return err
		}
		if err := tx.Where("region_id = ? AND used_at IS NULL", id).Delete(&model.MeshEnrollment{}).Error; err != nil {
			return err
		}
		return tx.Delete(&row).Error
	})
}

// AddNode makes regionID the home region of node, with a fixed address
// from its server segment; a boundary node joins every other region too.
func AddNode(ctx context.Context, db *gorm.DB, regionID uint, node agentcontrol.AgentNode, boundary bool, now time.Time) (model.MeshMember, error) {
	var row model.MeshMember
	err := change(ctx, db, func(tx *gorm.DB) error {
		if boundary {
			if err := lockAllRegions(tx); err != nil {
				return err
			}
		}
		if _, err := lookupNode(tx, node.String()); err != nil {
			return err
		}
		var region model.MeshRegion
		if err := findRegion(tx, regionID, &region); err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&model.MeshMember{}).Where("node_ref = ?", node.String()).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return conflict("the node is already in a region")
		}
		var err error
		if row, err = join(tx, region, node.String(), true, boundary, now); err != nil {
			return err
		}
		if boundary {
			return joinOthers(tx, node.String(), regionID, now)
		}
		return nil
	})
	return row, err
}

// UpdateMember changes the node of the home membership memberID: it moves
// to the region regionID (0 keeps its region), where it gets a new fixed
// address, and becomes a boundary node (an address in every other region)
// or an ordinary member. It answers the node's home membership. actor is
// the administrator recorded on a move.
func UpdateMember(ctx context.Context, db *gorm.DB, memberID, regionID uint, boundary bool, actor *uint, now time.Time) (model.MeshMember, error) {
	var row model.MeshMember
	err := change(ctx, db, func(tx *gorm.DB) error {
		// A move or a boundary node chooses addresses in more than one
		// region.
		if err := lockAllRegions(tx); err != nil {
			return err
		}
		home, err := findHome(tx, memberID)
		if err != nil {
			return err
		}
		row = home
		if regionID != 0 && regionID != home.RegionID {
			if row, err = moveNode(tx, home, regionID, boundary, now); err != nil {
				return err
			}
			if err := recordMove(tx, &home.NodeRef, nil, home.RegionID, regionID, home.Address, row.Address, actor, now); err != nil {
				return err
			}
		} else if home.Boundary != boundary {
			if err := tx.Model(&row).Update("boundary", boundary).Error; err != nil {
				return err
			}
		}
		if boundary {
			return joinOthers(tx, home.NodeRef, row.RegionID, now)
		}
		return tx.Where("node_ref = ? AND home = ?", home.NodeRef, false).Delete(&model.MeshMember{}).Error
	})
	return row, err
}

// moveNode makes regionID the home region of the node of home, with a new
// fixed address there; the caller rejoins the other regions of a boundary
// node.
func moveNode(tx *gorm.DB, home model.MeshMember, regionID uint, boundary bool, now time.Time) (model.MeshMember, error) {
	if err := checkGatewayStays(tx, home.NodeRef); err != nil {
		return model.MeshMember{}, err
	}
	var region model.MeshRegion
	if err := findRegion(tx, regionID, &region); err != nil {
		return model.MeshMember{}, err
	}
	result := tx.Where("node_ref = ? AND region_id IN ?", home.NodeRef, []uint{home.RegionID, regionID}).Delete(&model.MeshMember{})
	if result.Error != nil {
		return model.MeshMember{}, result.Error
	}
	if result.RowsAffected == 0 {
		return model.MeshMember{}, conflict("the node left the mesh meanwhile")
	}
	return join(tx, region, home.NodeRef, true, boundary, now)
}

// RemoveNode takes the node of the home membership memberID out of the
// mesh.
func RemoveNode(ctx context.Context, db *gorm.DB, memberID uint) error {
	return change(ctx, db, func(tx *gorm.DB) error {
		home, err := findHome(tx, memberID)
		if err != nil {
			return err
		}
		if err := checkGatewayStays(tx, home.NodeRef); err != nil {
			return err
		}
		// The node's member rows before its changes, in the order an
		// approval of a change takes them.
		if err := lockRows(tx, &model.MeshMember{}, "node_ref = ?", home.NodeRef); err != nil {
			return err
		}
		if err := rejectPendingOf(tx, home.NodeRef, "the node left the mesh", time.Now().UTC()); err != nil {
			return err
		}
		return tx.Where("node_ref = ?", home.NodeRef).Delete(&model.MeshMember{}).Error
	})
}

// findRegion reads a region a change is about to use, after locking its
// row (findLockedRegion): a range or a state read before the lock could
// change before the transaction ends.
func findRegion(tx *gorm.DB, id uint, row *model.MeshRegion) error {
	return findLockedRegion(tx, id, row)
}

// forUpdate makes tx's next read lock the rows it reads until the
// transaction ends (PostgreSQL).
func forUpdate(tx *gorm.DB) *gorm.DB {
	if tx.Name() != "postgres" {
		return tx
	}
	return tx.Clauses(clause.Locking{Strength: "UPDATE"})
}

// saveExisting writes every field of row, which must still exist. Unlike
// Save, which inserts a row when its update matches none, it refuses a row
// a change at the same time deleted, instead of bringing it back.
func saveExisting(tx *gorm.DB, row any, what string) error {
	result := tx.Model(row).Select("*").Updates(row)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return conflict("the %s was removed meanwhile", what)
	}
	return nil
}

// updateExisting writes only the given columns of the row of table with
// id, which must still exist. A change writes the columns it owns, so
// that it does not write back another change's columns from a row read
// before that change.
func updateExisting(tx *gorm.DB, table any, id uint, columns map[string]any, what string) error {
	result := tx.Model(table).Where("id = ?", id).Updates(columns)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return conflict("the %s was removed meanwhile", what)
	}
	return nil
}

// lockRows locks the rows of table that match the condition until the
// transaction ends (PostgreSQL).
func lockRows(tx *gorm.DB, table any, query string, args ...any) error {
	if tx.Name() != "postgres" {
		return nil
	}
	var ids []uint
	return tx.Model(table).Clauses(clause.Locking{Strength: "UPDATE"}).Where(query, args...).Order("id ASC").Pluck("id", &ids).Error
}

// readRegion reads a region without locking it, for an answer that
// changes nothing.
func readRegion(db *gorm.DB, id uint, row *model.MeshRegion) error {
	var rows []model.MeshRegion
	if err := db.Where("id = ?", id).Limit(1).Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return fmt.Errorf("%w: region %d", ErrNotFound, id)
	}
	*row = rows[0]
	return nil
}

func findHome(tx *gorm.DB, memberID uint) (model.MeshMember, error) {
	var rows []model.MeshMember
	// Locked: a node removed or moved at the same time is not changed
	// again from a row read before.
	if err := forUpdate(tx).Where("id = ? AND home = ?", memberID, true).Limit(1).Find(&rows).Error; err != nil {
		return model.MeshMember{}, err
	}
	if len(rows) == 0 {
		return model.MeshMember{}, fmt.Errorf("%w: member %d", ErrNotFound, memberID)
	}
	return rows[0], nil
}

// joinOthers gives a boundary node an address in every region but home
// where it has none.
func joinOthers(tx *gorm.DB, nodeRef string, home uint, now time.Time) error {
	var regions []model.MeshRegion
	if err := tx.Where("id <> ? AND id NOT IN (?)", home,
		tx.Model(&model.MeshMember{}).Select("region_id").Where("node_ref = ?", nodeRef)).Order("id ASC").Find(&regions).Error; err != nil {
		return err
	}
	for _, region := range regions {
		if _, err := join(tx, region, nodeRef, false, false, now); err != nil {
			return err
		}
	}
	return nil
}

// join gives nodeRef the lowest free address of region's server segment.
func join(tx *gorm.DB, region model.MeshRegion, nodeRef string, home, boundary bool, now time.Time) (model.MeshMember, error) {
	prefix, err := netip.ParsePrefix(region.CIDR)
	if err != nil {
		return model.MeshMember{}, fmt.Errorf("region %q: %w", region.Code, err)
	}
	used, err := usedServerAddresses(tx, region.ID, 0, now)
	if err != nil {
		return model.MeshMember{}, err
	}
	address, ok := freeServerAddress(prefix, used)
	if !ok {
		return model.MeshMember{}, fmt.Errorf("%w: %s", ErrFull, region.Code)
	}
	row := model.MeshMember{RegionID: region.ID, NodeRef: nodeRef, Address: address.String(), Home: home, Boundary: boundary, CreatedAt: now}
	return row, tx.Create(&row).Error
}

// freeServerAddress is the lowest address of the lower half of prefix, after
// the network address, that used does not hold.
func freeServerAddress(prefix netip.Prefix, used []string) (netip.Addr, bool) {
	taken := make(map[string]bool, len(used))
	for _, address := range used {
		taken[address] = true
	}
	half := uint32(1) << (31 - prefix.Bits())
	base := prefix.Addr().As4()
	start := binary.BigEndian.Uint32(base[:])
	for offset := uint32(1); offset < half; offset++ {
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], start+offset)
		address := netip.AddrFrom4(b)
		if !taken[address.String()] {
			return address, true
		}
	}
	return netip.Addr{}, false
}

type nodeInfo struct{ name, host string }

// lookupNode answers a node's name and host; ErrNotFound when it does not
// exist.
func lookupNode(tx *gorm.DB, ref string) (nodeInfo, error) {
	node, err := agentcontrol.ParseAgentNode(ref)
	if err != nil {
		return nodeInfo{}, invalid("node: proxy-<id> or forward-<id>")
	}
	var table any = &model.Node{}
	if node.Kind == agentcontrol.NodeKindForward {
		table = &model.ForwardNode{}
	}
	var rows []struct{ Name, Host string }
	if err := tx.Model(table).Select("name", "host").Where("id = ?", node.ID).Limit(1).Find(&rows).Error; err != nil {
		return nodeInfo{}, err
	}
	if len(rows) == 0 {
		return nodeInfo{}, fmt.Errorf("%w: node %s", ErrNotFound, ref)
	}
	return nodeInfo{name: rows[0].Name, host: rows[0].Host}, nil
}

// NodeMember is the member "mesh" of node's desired configuration: the
// networks of its enabled regions (agent-contract.md section 2), nil when
// it is in none, when the feature is off, or before the tables exist.
func NodeMember(db *gorm.DB, node agentcontrol.AgentNode) (map[string]any, error) {
	if !features.Enabled(Feature) || !db.Migrator().HasTable(&model.MeshMember{}) {
		return nil, nil
	}
	var own []model.MeshMember
	if err := db.Where("node_ref = ?", node.String()).Find(&own).Error; err != nil || len(own) == 0 {
		return nil, err
	}
	boundary := false
	ids := make([]uint, 0, len(own))
	for _, member := range own {
		boundary = boundary || (member.Home && member.Boundary)
		ids = append(ids, member.RegionID)
	}
	var regions []model.MeshRegion
	if err := db.Where("enabled = ?", true).Order("sort ASC, id ASC").Find(&regions).Error; err != nil {
		return nil, err
	}
	mine := map[uint]model.MeshMember{}
	for _, member := range own {
		mine[member.RegionID] = member
	}
	// The boundary nodes of each region: a home row with the flag, or a
	// row that is not a home row.
	var boundaries []model.MeshMember
	if err := db.Where("region_id IN ? AND node_ref <> ? AND (home = ? OR boundary = ?)", ids, node.String(), false, true).
		Order("id ASC").Find(&boundaries).Error; err != nil {
		return nil, err
	}
	peers := map[uint][]map[string]any{}
	hosts := map[string]string{}
	for _, peer := range boundaries {
		host, done := hosts[peer.NodeRef]
		if !done {
			info, err := lookupNode(db, peer.NodeRef)
			if err != nil && !errors.Is(err, ErrNotFound) {
				return nil, err
			}
			host, hosts[peer.NodeRef] = info.host, info.host
		}
		if host != "" {
			peers[peer.RegionID] = append(peers[peer.RegionID], map[string]any{"server": host, "port": Port, "transport": "tcp"})
		}
	}
	dns, err := dnsSection(db)
	if err != nil {
		return nil, err
	}
	hostGroups, err := hostGroupSections(db)
	if err != nil {
		return nil, err
	}
	keys, err := nativeKeys(db, ids)
	if err != nil {
		return nil, err
	}
	resolvers, err := resolverAddresses(db, regions)
	if err != nil {
		return nil, err
	}
	networks := []map[string]any{}
	for _, region := range regions {
		member, ok := mine[region.ID]
		if !ok {
			continue
		}
		prefix, err := netip.ParsePrefix(region.CIDR)
		if err != nil {
			return nil, fmt.Errorf("region %q: %w", region.Code, err)
		}
		advertise := []string{}
		if boundary {
			for _, other := range regions {
				if other.ID != region.ID {
					advertise = append(advertise, other.CIDR)
				}
			}
		}
		regionPeers := peers[region.ID]
		if regionPeers == nil {
			regionPeers = []map[string]any{}
		}
		groups := hostGroups[region.ID]
		if groups == nil {
			groups = []map[string]any{}
		}
		network := map[string]any{
			"id": region.Code, "name": "mesh-" + region.Code, "kind": KindManaged, "native_keys": regionKeys(keys, region.ID),
			"secret": region.Secret, "secure_mode": true, "cipher": cipher,
			"address": fmt.Sprintf("%s/%d", member.Address, prefix.Bits()), "role": "full",
			"listen":      []map[string]any{{"transport": "tcp", "port": Port}, {"transport": "udp", "port": Port}},
			"peers":       regionPeers,
			"subnets":     map[string]any{"accept": []string{}, "advertise": advertise},
			"routing":     map[string]any{"policy": policy, "paths": []any{}},
			"dns":         regionDNS(dns, resolvers[region.ID]),
			"host_groups": groups,
		}
		// An external network bridge bridges in its home region only
		// (spec section 15.12).
		if member.Home {
			bridge, err := bridgeSection(db, node.String(), region.ID)
			if err != nil {
				return nil, err
			}
			if bridge != nil {
				network["external_bridge"] = bridge
			}
		}
		networks = append(networks, network)
	}
	if len(networks) == 0 {
		return nil, nil
	}
	return map[string]any{"networks": networks}, nil
}

// RecordStatus keeps payload, a "mesh.status" report of node, as the
// node's latest status. payload must be a JSON object with a "networks"
// array; its "instance_id", when valid, names the node in the host groups.
func RecordStatus(ctx context.Context, db *gorm.DB, node agentcontrol.AgentNode, payload []byte, observedAt, receivedAt time.Time) error {
	var shape struct {
		Networks []json.RawMessage `json:"networks"`
	}
	if err := json.Unmarshal(payload, &shape); err != nil || shape.Networks == nil {
		return invalid("payload: a JSON object with a networks array")
	}
	db = db.WithContext(ctx)
	var members int64
	if err := db.Model(&model.MeshMember{}).Where("node_ref = ?", node.String()).Count(&members).Error; err != nil {
		return err
	}
	if members == 0 {
		return ErrNotMember
	}
	before, err := instanceIDs(db, []string{node.String()})
	if err != nil {
		return err
	}
	// Reading the node's report before this one, writing this one and
	// releasing holds happen under a lock on the node's row (PostgreSQL):
	// a hold is released by two consecutive reports, and a report observed
	// before the stored one (late, or out of order between two
	// connections) is dropped.
	dropped := false
	err = db.Transaction(func(tx *gorm.DB) error {
		var previous []model.MeshStatus
		if err := forUpdate(tx).Where("node_ref = ?", node.String()).Limit(1).Find(&previous).Error; err != nil {
			return err
		}
		var last *model.MeshStatus
		if len(previous) > 0 {
			last = &previous[0]
			if observedAt.Before(last.ObservedAt) {
				dropped = true
				return nil
			}
		}
		row := model.MeshStatus{NodeRef: node.String(), Payload: string(payload), ObservedAt: observedAt, ReceivedAt: receivedAt}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "node_ref"}},
			DoUpdates: clause.AssignmentColumns([]string{"payload", "observed_at", "received_at"}),
		}).Create(&row).Error; err != nil {
			return err
		}
		if err := touchDevices(tx, payload, observedAt); err != nil {
			return err
		}
		// The allow list the node reports applied releases the
		// addresses held for it (spec section 15.12 item 4).
		return releaseHolds(tx, node.String(), last, row.Payload, receivedAt)
	})
	if err != nil || dropped {
		return err
	}
	// A new or changed static key of the node changes its regions'
	// native_keys.
	if err := recordMemberKeys(ctx, db, node.String(), payload); err != nil {
		return err
	}
	// A host group names its gateways by instance id: a new or changed id
	// of a gateway changes the configuration of its region's nodes.
	if statusInstanceID(string(payload)) == before[node.String()] {
		return nil
	}
	gateway, err := isGateway(db, node.String())
	if err != nil || !gateway {
		return err
	}
	return change(ctx, db, func(*gorm.DB) error { return nil })
}

// change runs write in a transaction and then tells the listeners every
// node that was or is a member, whose configurations may have changed.
// Addresses are taken by reading the free ones and inserting, guarded by
// unique indexes: a write that collides with a concurrent one runs again,
// up to three times, and is then a conflict.
func change(ctx context.Context, db *gorm.DB, write func(tx *gorm.DB) error) error {
	return changeLast(ctx, db, "", write)
}

// changeLast is change that tells the listeners about the node last,
// after every other node, when last is not empty: a node's new static key
// reaches the other members before the node itself starts to use it.
func changeLast(ctx context.Context, db *gorm.DB, last string, write func(tx *gorm.DB) error) error {
	db = db.WithContext(ctx)
	before, err := memberNodes(db)
	if err != nil {
		return err
	}
	for attempt := 1; ; attempt++ {
		err = db.Transaction(func(tx *gorm.DB) error { return withBridgeHolds(tx, write) })
		if err == nil || !isUniqueViolation(err) && !isRetryable(err) {
			break
		}
		changeRetries.Add(1)
		if attempt == 3 {
			return conflict("a concurrent change conflicted with this one; try again")
		}
		// A short, growing and random pause, so that the transactions
		// that collided do not collide again at once.
		var jitter [1]byte
		_, _ = rand.Read(jitter[:])
		time.Sleep(time.Duration(attempt)*10*time.Millisecond + time.Duration(jitter[0]%10)*time.Millisecond)
	}
	// The only address to reclaim is in an external bridge's allow
	// list: it is taken from its device first (releaseFirst).
	var release *releaseFirst
	if errors.As(err, &release) {
		return release.release(ctx, db)
	}
	if err != nil {
		return err
	}
	after, err := memberNodes(db)
	if err != nil {
		return err
	}
	for ref := range before {
		after[ref] = true
	}
	nodes := make([]agentcontrol.AgentNode, 0, len(after))
	for ref := range after {
		if node, err := agentcontrol.ParseAgentNode(ref); err == nil {
			nodes = append(nodes, node)
		}
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].String() < nodes[j].String() })
	if last == "" {
		notify(nodes)
		return nil
	}
	others := make([]agentcontrol.AgentNode, 0, len(nodes))
	var self []agentcontrol.AgentNode
	for _, node := range nodes {
		if node.String() == last {
			self = append(self, node)
		} else {
			others = append(others, node)
		}
	}
	notify(others)
	notify(self)
	return nil
}

func memberNodes(db *gorm.DB) (map[string]bool, error) {
	var refs []string
	if err := db.Model(&model.MeshMember{}).Distinct("node_ref").Pluck("node_ref", &refs).Error; err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(refs))
	for _, ref := range refs {
		set[ref] = true
	}
	return set, nil
}

// Listener is told the nodes whose mesh configuration may have changed.
type Listener func(nodes []agentcontrol.AgentNode)

var (
	listenersMu sync.Mutex
	listeners   = map[int]Listener{}
	nextID      int
)

// OnChange registers listener for every change; cancel removes it.
func OnChange(listener Listener) (cancel func()) {
	listenersMu.Lock()
	defer listenersMu.Unlock()
	nextID++
	id := nextID
	listeners[id] = listener
	return func() {
		listenersMu.Lock()
		delete(listeners, id)
		listenersMu.Unlock()
	}
}

func notify(nodes []agentcontrol.AgentNode) {
	if len(nodes) == 0 {
		return
	}
	listenersMu.Lock()
	called := make([]Listener, 0, len(listeners))
	for _, listener := range listeners {
		called = append(called, listener)
	}
	listenersMu.Unlock()
	for _, listener := range called {
		listener(nodes)
	}
}

// changeRetries counts the transactions change ran again, which the
// PostgreSQL tests read: with the regions locked in one order, changes at
// the same time wait for each other instead of colliding.
var changeRetries atomic.Int64

// isRetryable tells whether PostgreSQL ended the transaction for a
// deadlock (40P01) or a serialization failure (40001), after which the
// whole transaction can run again.
func isRetryable(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "40P01" || pgErr.Code == "40001"
	}
	text := err.Error()
	return strings.Contains(text, "SQLSTATE 40P01") || strings.Contains(text, "SQLSTATE 40001")
}

// isUniqueViolation tells whether err is a unique index violation.
func isUniqueViolation(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "unique constraint") || strings.Contains(text, "duplicate key")
}
