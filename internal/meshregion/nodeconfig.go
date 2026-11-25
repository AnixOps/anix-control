package meshregion

import (
	"encoding/json"
	"net/netip"
	"regexp"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// The sections of a region network that carry the intranet names (R28)
// and the host groups (R21, R25) to the node (agent-contract.md section
// 2). The Agent leaves out a section its kernel does not take yet.

// instanceIDPattern is the node instance id a status may carry: the
// kernel's id of the node, kept across restarts and upgrades.
// 1 to 64 letters, digits or hyphens, the first a letter or digit, as the
// kernel and the Agent check it.
var instanceIDPattern = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z-]{0,63}$`)

// statusInstanceID answers the instance id of a "mesh.status" payload, ""
// when it has none.
func statusInstanceID(payload string) string {
	var shape struct {
		InstanceID string `json:"instance_id"`
	}
	if json.Unmarshal([]byte(payload), &shape) != nil || !instanceIDPattern.MatchString(shape.InstanceID) {
		return ""
	}
	return shape.InstanceID
}

// instanceIDs answers the instance ids the nodes last reported.
func instanceIDs(db *gorm.DB, refs []string) (map[string]string, error) {
	ids := map[string]string{}
	if len(refs) == 0 {
		return ids, nil
	}
	var rows []model.MeshStatus
	if err := db.Where("node_ref IN ?", refs).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		if id := statusInstanceID(row.Payload); id != "" {
			ids[row.NodeRef] = id
		}
	}
	return ids, nil
}

// dnsSection is the section "dns": the suffix and every intranet name as
// <name>.<region code> with its address. A name more than one entry has
// goes with each of its addresses (R28): the kernel answers them all and
// marks the name, as the panel does.
func dnsSection(db *gorm.DB) (map[string]any, error) {
	list, err := Names(db.Statement.Context, db)
	if err != nil {
		return nil, err
	}
	names := []map[string]any{}
	for _, entry := range list.Entries {
		if entry.RegionCode == "" {
			continue
		}
		names = append(names, map[string]any{"name": entry.Name + "." + entry.RegionCode, "address": entry.Address})
	}
	return map[string]any{"enabled": true, "suffix": list.Suffix, "names": names}, nil
}

// resolverAddresses answers, by region id, the resolver address each of
// regions delivers as dns.resolver_address: none while a device still
// holds it from before it was reserved (spec section 15.9.2, item 8,
// transition), and none for a range it cannot be read from.
func resolverAddresses(db *gorm.DB, regions []model.MeshRegion) (map[uint]string, error) {
	answer := map[uint]string{}
	ids := make([]uint, 0, len(regions))
	for _, region := range regions {
		if prefix, err := netip.ParsePrefix(region.CIDR); err == nil && prefix.Addr().Is4() {
			answer[region.ID] = ResolverAddress(prefix).String()
			ids = append(ids, region.ID)
		}
	}
	if len(ids) == 0 {
		return answer, nil
	}
	var holders []struct {
		RegionID uint
		Address  string
	}
	if err := db.Model(&model.MeshDevice{}).Select("region_id", "address").Where("region_id IN ? AND address IS NOT NULL", ids).Scan(&holders).Error; err != nil {
		return nil, err
	}
	for _, holder := range holders {
		if answer[holder.RegionID] == holder.Address {
			delete(answer, holder.RegionID)
		}
	}
	return answer, nil
}

// regionDNS answers the section "dns" of one network: dns with the
// network's resolver address, when it has one to deliver.
func regionDNS(dns map[string]any, resolver string) map[string]any {
	if resolver == "" {
		return dns
	}
	section := make(map[string]any, len(dns)+1)
	for key, value := range dns {
		section[key] = value
	}
	section["resolver_address"] = resolver
	return section
}

// hostGroupSections answers the section "host_groups" of each region: the
// groups whose gateway's home is the region. A group whose gateway has
// not reported its instance id yet is left out, and so is a standby that
// has not.
func hostGroupSections(db *gorm.DB) (map[uint][]map[string]any, error) {
	var groups []model.MeshHostGroup
	if err := db.Order("id ASC").Find(&groups).Error; err != nil {
		return nil, err
	}
	sections := map[uint][]map[string]any{}
	if len(groups) == 0 {
		return sections, nil
	}
	refs := []string{}
	for _, group := range groups {
		refs = append(refs, group.GatewayRef)
		if group.StandbyRef != nil {
			refs = append(refs, *group.StandbyRef)
		}
	}
	ids, err := instanceIDs(db, refs)
	if err != nil {
		return nil, err
	}
	var homes []model.MeshMember
	if err := db.Where("home = ? AND node_ref IN ?", true, refs).Find(&homes).Error; err != nil {
		return nil, err
	}
	region := map[string]uint{}
	for _, home := range homes {
		region[home.NodeRef] = home.RegionID
	}
	for _, group := range groups {
		home, inMesh := region[group.GatewayRef]
		gateway := ids[group.GatewayRef]
		if !inMesh || gateway == "" {
			continue
		}
		section := map[string]any{"cidr": group.Segment, "gateway": gateway}
		if group.StandbyRef != nil && ids[*group.StandbyRef] != "" {
			section["backup_gateway"] = ids[*group.StandbyRef]
		}
		sections[home] = append(sections[home], section)
	}
	return sections, nil
}

// isGateway tells whether nodeRef is the gateway or the standby of a host
// group.
func isGateway(db *gorm.DB, nodeRef string) (bool, error) {
	var count int64
	err := db.Model(&model.MeshHostGroup{}).Where("gateway_ref = ? OR standby_ref = ?", nodeRef, nodeRef).Count(&count).Error
	return count > 0, err
}
