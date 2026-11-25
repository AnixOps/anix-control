// Package relayroute is the region relay route of a node: the relays
// (anix-relay, docs/design/region-relay) it reaches Control through, never
// directly, and their release file caches. A node group holds the route
// (v5_node_group.relays, release_caches); the group's install command
// carries it, and every node of the group gets it in its desired
// configuration as the member "control_route", so a change also reaches the
// nodes that already joined.
package relayroute

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// Member is the desired configuration member that carries a route.
const Member = "control_route"

// Limits of a route.
const (
	MaxRelays = 8
	MaxCaches = 8
	// maxEntry bounds one host:port or URL.
	maxEntry = 255
)

// ErrInvalid wraps every validation error of Check.
var ErrInvalid = errors.New("invalid relay route")

// Route is a node's relays and release caches. A route without relays
// reaches Control directly; caches alone are allowed (downloads only).
type Route struct {
	Relays        []string `json:"relays"`
	ReleaseCaches []string `json:"release_caches"`
}

// Empty reports a route with neither relays nor caches.
func (r Route) Empty() bool { return len(r.Relays) == 0 && len(r.ReleaseCaches) == 0 }

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// CleanRelays checks relay addresses (host:port, a hostname or an IP
// literal, port 1-65535), trims them and drops repeats, in order.
func CleanRelays(relays []string) ([]string, error) {
	clean := []string{}
	seen := map[string]bool{}
	for _, relay := range relays {
		relay = strings.TrimSpace(relay)
		if relay == "" {
			continue
		}
		host, port, err := net.SplitHostPort(relay)
		number, portErr := strconv.Atoi(port)
		if err != nil || len(relay) > maxEntry || !validHost(host) || portErr != nil || number < 1 || number > 65535 {
			return nil, invalid("relay %q is not host:port", relay)
		}
		if !seen[relay] {
			seen[relay] = true
			clean = append(clean, relay)
		}
	}
	if len(clean) > MaxRelays {
		return nil, invalid("at most %d relays", MaxRelays)
	}
	return clean, nil
}

// CleanCaches checks release cache addresses: https URLs without user,
// query or fragment, such as https://relay-1.example.com:8443. A trailing
// slash is dropped.
func CleanCaches(caches []string) ([]string, error) {
	clean := []string{}
	seen := map[string]bool{}
	for _, raw := range caches {
		raw = strings.TrimRight(strings.TrimSpace(raw), "/")
		if raw == "" {
			continue
		}
		parsed, err := url.Parse(raw)
		if err != nil || len(raw) > maxEntry || parsed.Scheme != "https" || parsed.User != nil ||
			parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" || !validHost(parsed.Hostname()) {
			return nil, invalid("release cache %q is not an https URL", raw)
		}
		// The installer writes the URL into the command and the Agent's
		// configuration: only plain path characters.
		if strings.Trim(parsed.EscapedPath(), "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._~/-") != "" {
			return nil, invalid("release cache %q has a path with other characters than letters, digits and ._~/-", raw)
		}
		if port := parsed.Port(); port != "" {
			if number, err := strconv.Atoi(port); err != nil || number < 1 || number > 65535 {
				return nil, invalid("release cache %q has no valid port", raw)
			}
		}
		if !seen[raw] {
			seen[raw] = true
			clean = append(clean, raw)
		}
	}
	if len(clean) > MaxCaches {
		return nil, invalid("at most %d release caches", MaxCaches)
	}
	return clean, nil
}

// validHost accepts an IP literal or a DNS name of letters, digits, dashes
// and dots; nothing a shell or a URL would read otherwise.
func validHost(host string) bool {
	if host == "" {
		return false
	}
	if net.ParseIP(host) != nil {
		return true
	}
	if len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(strings.TrimSuffix(host, "."), ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
	}
	return true
}

// Encode stores a cleaned list as a JSON array, "" for none.
func Encode(list []string) string {
	if len(list) == 0 {
		return ""
	}
	encoded, _ := json.Marshal(list)
	return string(encoded)
}

// Decode reads a stored list; a malformed one reads as none.
func Decode(stored string) []string {
	list := []string{}
	if stored != "" {
		_ = json.Unmarshal([]byte(stored), &list)
	}
	return list
}

// OfGroup is a group's route as stored.
func OfGroup(group model.NodeGroup) Route {
	return Route{Relays: Decode(group.Relays), ReleaseCaches: Decode(group.ReleaseCaches)}
}

// ForNode answers the route of node: its group's, or an empty route for a
// node without a group or of a deleted group.
func ForNode(db *gorm.DB, node agentcontrol.AgentNode) (Route, error) {
	// NullInt64: a node without a group has NULL, which a *uint pluck
	// cannot scan on every driver.
	var ids []sql.NullInt64
	query := db.Model(&model.Node{})
	if node.Kind == agentcontrol.NodeKindForward {
		query = db.Model(&model.ForwardNode{})
	} else if node.Kind != agentcontrol.NodeKindProxy {
		return Route{}, nil
	}
	if err := query.Where("id = ?", node.ID).Limit(1).Pluck("node_group_id", &ids).Error; err != nil {
		return Route{}, err
	}
	if len(ids) == 0 || !ids[0].Valid {
		return Route{}, nil
	}
	var groups []model.NodeGroup
	if err := db.Where("id = ?", ids[0].Int64).Limit(1).Find(&groups).Error; err != nil {
		return Route{}, err
	}
	if len(groups) == 0 {
		return Route{}, nil
	}
	return OfGroup(groups[0]), nil
}

// DesiredMember is the "control_route" member of node's desired
// configuration, nil when its route is empty: the member is additive, and an
// Agent without it keeps the route its own configuration has.
func DesiredMember(db *gorm.DB, node agentcontrol.AgentNode) (map[string]any, error) {
	route, err := ForNode(db, node)
	if err != nil || route.Empty() {
		return nil, err
	}
	return map[string]any{"relays": route.Relays, "release_caches": route.ReleaseCaches}, nil
}
