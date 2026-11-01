// Package trafficqos is the traffic priorities and per-user class caps of
// the proxy nodes (流量与监控 → 流量分类 → QoS): each traffic category
// (internal/destinationstats) may get a priority (high or low; the rest is
// normal), each node a download capacity, and chosen destinations a cap
// per user (Steam downloads at 5 Mbit/s). The settings reach the nodes as
// the member "qos" of their desired configuration (NodeMember), which the
// Agent passes to anixcore (PUT /v1/qos, networkcore engine_inbound::qos):
// lower classes there use what the higher ones leave of the capacity, and
// a capped class holds each user's connections of it to its cap.
//
// The settings are one JSON document in v2_system_config (SettingsKey).
// Without it the defaults apply: on, downloads at low priority, Steam
// downloads capped at 5 Mbit/s per user.
package trafficqos

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/AnixOps/anix-control/v5/internal/destinationstats"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// SettingsKey is the v2_system_config key of the settings.
const SettingsKey = "traffic.qos"

// The priorities a category or a cap may have.
const (
	PriorityHigh   = "high"
	PriorityNormal = "normal"
	PriorityLow    = "low"
)

// Limits.
const (
	MaxCaps          = 32
	MaxCapDomains    = 500
	MaxEgressMbps    = 1_000_000
	MaxCapMbps       = 100_000
	DefaultMinShare  = 10
	maxCapNameLength = 64
	// MaxMemberEntries bounds the domains and networks of a node's member,
	// which travels in every configuration snapshot (8 MiB at most).
	MaxMemberEntries = 50_000
)

// ErrInvalid refuses invalid settings; the error says which field.
var ErrInvalid = errors.New("invalid QoS settings")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// Cap is a per-user cap on the download of some destinations.
type Cap struct {
	Name string `json:"name"`
	// Priority is the destinations' priority; empty is low.
	Priority string `json:"priority"`
	// LimitMbps is each user's cap, Mbit/s; 0 gives the priority alone.
	LimitMbps int `json:"limit_mbps"`
	// Domains each also match their subdomains.
	Domains []string `json:"domains"`
}

// Settings are the QoS settings.
type Settings struct {
	Enabled bool `json:"enabled"`
	// Priorities maps a category to high or low; a category left out is
	// normal. web and unknown take no priority: they are what no rule
	// names.
	Priorities map[string]string `json:"priorities"`
	// MinSharePercent is the least share of a capacity a lower class keeps
	// (1 to 90).
	MinSharePercent int `json:"min_share_percent"`
	// DefaultEgressMbps is a node's download capacity unless NodeEgressMbps
	// names it; 0 schedules within the users' speed limits only.
	DefaultEgressMbps int `json:"default_egress_mbps"`
	// NodeEgressMbps is the capacity of single nodes, by node id.
	NodeEgressMbps map[string]int `json:"node_egress_mbps"`
	Caps           []Cap          `json:"caps"`
}

// SteamDownloadDomains are the hosts Steam downloads games and updates
// from (its content servers and CDNs, including the mainland mirrors); the
// store, community and login are elsewhere and stay uncapped.
var SteamDownloadDomains = []string{
	"steamcontent.com", "steamserver.net",
	"steampipe.akamaized.net", "steampipe-kr.akamaized.net", "steampipe-partner.akamaized.net",
	"content.steamchina.com", "dl.steam.clngaa.com", "st.dl.eccdnx.com", "st.dl.bscstorage.net",
	"st.dl.pinyuncloud.com", "steam.cdn.on.net", "steam.naeu.qtlglb.com", "steam.apac.qtlglb.com",
}

// Defaults are the settings without a stored document: on, downloads at
// low priority, Steam downloads capped at 5 Mbit/s per user.
func Defaults() Settings {
	return Settings{
		Enabled:         true,
		Priorities:      map[string]string{destinationstats.CategoryDownload: PriorityLow},
		MinSharePercent: DefaultMinShare,
		NodeEgressMbps:  map[string]int{},
		Caps: []Cap{{
			Name: "Steam 下载", Priority: PriorityLow, LimitMbps: 5,
			Domains: append([]string(nil), SteamDownloadDomains...),
		}},
	}
}

// Normalize checks the settings and answers them in their stored form.
func (s Settings) Normalize() (Settings, error) {
	out := Settings{
		Enabled: s.Enabled, Priorities: map[string]string{}, MinSharePercent: s.MinSharePercent,
		DefaultEgressMbps: s.DefaultEgressMbps, NodeEgressMbps: map[string]int{}, Caps: []Cap{},
	}
	for category, priority := range s.Priorities {
		if !destinationstats.IsCategory(category) || category == destinationstats.CategoryWeb || category == destinationstats.CategoryUnknown {
			return out, invalid("priorities: %q is not a category that can take a priority", category)
		}
		switch priority {
		case PriorityHigh, PriorityLow:
			out.Priorities[category] = priority
		case PriorityNormal, "":
		default:
			return out, invalid("priorities: %q must be high, normal or low", priority)
		}
	}
	if out.MinSharePercent == 0 {
		out.MinSharePercent = DefaultMinShare
	}
	if out.MinSharePercent < 1 || out.MinSharePercent > 90 {
		return out, invalid("min_share_percent must be between 1 and 90")
	}
	if out.DefaultEgressMbps < 0 || out.DefaultEgressMbps > MaxEgressMbps {
		return out, invalid("default_egress_mbps must be between 0 and %d", MaxEgressMbps)
	}
	for node, mbps := range s.NodeEgressMbps {
		id, err := strconv.ParseUint(node, 10, 32)
		if err != nil || id == 0 {
			return out, invalid("node_egress_mbps: %q is not a node id", node)
		}
		if mbps < 0 || mbps > MaxEgressMbps {
			return out, invalid("node_egress_mbps: node %s must be between 0 and %d", node, MaxEgressMbps)
		}
		out.NodeEgressMbps[strconv.FormatUint(id, 10)] = mbps
	}
	if len(s.Caps) > MaxCaps {
		return out, invalid("at most %d caps", MaxCaps)
	}
	names := map[string]bool{}
	for _, c := range s.Caps {
		c.Name = strings.TrimSpace(c.Name)
		if c.Name == "" || len(c.Name) > maxCapNameLength || names[c.Name] {
			return out, invalid("caps: every cap needs a unique name of at most %d bytes", maxCapNameLength)
		}
		names[c.Name] = true
		switch c.Priority {
		case "":
			c.Priority = PriorityLow
		case PriorityHigh, PriorityNormal, PriorityLow:
		default:
			return out, invalid("caps: %q: priority must be high, normal or low", c.Name)
		}
		if c.LimitMbps < 0 || c.LimitMbps > MaxCapMbps {
			return out, invalid("caps: %q: limit_mbps must be between 0 and %d", c.Name, MaxCapMbps)
		}
		if len(c.Domains) == 0 || len(c.Domains) > MaxCapDomains {
			return out, invalid("caps: %q: 1 to %d domains", c.Name, MaxCapDomains)
		}
		domains := make([]string, 0, len(c.Domains))
		seen := map[string]bool{}
		for _, raw := range c.Domains {
			domain, ok := destinationstats.NormalizeDomain(raw)
			if !ok {
				return out, invalid("caps: %q: %q is not a domain", c.Name, raw)
			}
			if !seen[domain] {
				seen[domain] = true
				domains = append(domains, domain)
			}
		}
		c.Domains = domains
		out.Caps = append(out.Caps, c)
	}
	return out, nil
}

// Load reads the settings, the defaults without a stored document.
func Load(db *gorm.DB) (Settings, error) {
	var rows []model.SystemConfig
	if err := db.Where("key = ?", SettingsKey).Limit(1).Find(&rows).Error; err != nil {
		return Settings{}, err
	}
	if len(rows) == 0 || strings.TrimSpace(rows[0].Value) == "" {
		return Defaults(), nil
	}
	var stored Settings
	if err := json.Unmarshal([]byte(rows[0].Value), &stored); err != nil {
		return Defaults(), nil
	}
	settings, err := stored.Normalize()
	if err != nil {
		return Defaults(), nil
	}
	return settings, nil
}

// Save checks and stores the settings, audited, and answers them stored.
func Save(db *gorm.DB, settings Settings, actor Actor) (Settings, error) {
	normalized, err := settings.Normalize()
	if err != nil {
		return normalized, err
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return normalized, err
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		var row model.SystemConfig
		err := tx.Where("key = ?", SettingsKey).First(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = tx.Create(&model.SystemConfig{Key: SettingsKey, Value: string(encoded), Type: "json", Group: "traffic", Remark: "流量优先级与按类别限速"}).Error
		} else if err == nil {
			err = tx.Model(&row).Update("value", string(encoded)).Error
		}
		if err != nil {
			return err
		}
		var userID *uint
		if actor.UserID != 0 {
			id := actor.UserID
			userID = &id
		}
		return tx.Create(&model.OperationLog{
			UserID: userID, Username: truncate(actor.Name, 100), Action: "update", Module: "traffic_qos",
			TargetType: "traffic_qos", Content: string(encoded), IP: truncate(actor.IP, 45), Status: 1,
		}).Error
	})
	return normalized, err
}

// Actor is who changed the settings, for the audit entry.
type Actor struct {
	UserID uint
	Name   string
	IP     string
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}

// kernelProtocols are the sniffed protocols anixcore can class by.
var kernelProtocols = map[string]bool{"bittorrent": true, "http": true, "tls": true}

// NodeMember is the member "qos" of a proxy node's desired configuration:
// nil when the settings are off or name no class, so the node turns its
// priorities off. The categories' destinations come from the current
// category rules; at most MaxMemberEntries domains and networks are sent,
// the caps first.
func NodeMember(db *gorm.DB, nodeID uint) (map[string]any, error) {
	// A database without the settings table (a partial test schema) has
	// no QoS.
	if !db.Migrator().HasTable(&model.SystemConfig{}) {
		return nil, nil
	}
	settings, err := Load(db)
	if err != nil {
		return nil, err
	}
	if !settings.Enabled {
		return nil, nil
	}
	budget := MaxMemberEntries
	classes := []map[string]any{}
	for _, c := range settings.Caps {
		domains := c.Domains
		if len(domains) > budget {
			domains = domains[:budget]
		}
		budget -= len(domains)
		classes = append(classes, map[string]any{
			"name": "cap:" + c.Name, "priority": c.Priority, "limit_mbps": c.LimitMbps, "domains": domains,
		})
	}
	if len(settings.Priorities) > 0 {
		classifier, err := destinationstats.CurrentClassifier(db)
		if err != nil {
			return nil, err
		}
		for _, list := range classifier.Lists() {
			priority, ok := settings.Priorities[list.Category]
			if !ok {
				continue
			}
			domains, networks := list.Domains, list.Networks
			if len(domains) > budget {
				domains = domains[:budget]
			}
			budget -= len(domains)
			if len(networks) > budget {
				networks = networks[:budget]
			}
			budget -= len(networks)
			protocols := []string{}
			for _, protocol := range list.Protocols {
				if kernelProtocols[protocol] {
					protocols = append(protocols, protocol)
				}
			}
			if len(domains)+len(networks)+len(protocols) == 0 {
				continue
			}
			// The caps come first so that a capped domain inside a category
			// keeps its cap: the kernel takes a domain's last class, so the
			// category classes leave the capped domains out.
			classes = append(classes, map[string]any{
				"name": list.Category, "priority": priority, "limit_mbps": 0,
				"domains": withoutCapped(domains, settings.Caps), "networks": networks, "protocols": protocols,
			})
		}
	}
	if len(classes) == 0 {
		return nil, nil
	}
	egress := settings.DefaultEgressMbps
	if mbps, ok := settings.NodeEgressMbps[strconv.FormatUint(uint64(nodeID), 10)]; ok {
		egress = mbps
	}
	return map[string]any{
		"egress_mbps": egress, "min_share_percent": settings.MinSharePercent, "classes": classes,
	}, nil
}

func withoutCapped(domains []string, caps []Cap) []string {
	capped := map[string]bool{}
	for _, c := range caps {
		for _, domain := range c.Domains {
			capped[domain] = true
		}
	}
	out := make([]string, 0, len(domains))
	for _, domain := range domains {
		if !capped[domain] {
			out = append(out, domain)
		}
	}
	return out
}
