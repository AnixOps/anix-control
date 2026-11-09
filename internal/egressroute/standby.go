package egressroute

import (
	"strings"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// StandbyIDBase offsets the ids of the cross-border standby channels
// (model.L2Standby, internal/l2standby) in the member "egress", so they
// never share an id with an egress route. The kernel tries routes in id
// order: a standby's catch-all route comes after every domain route.
const StandbyIDBase = 1_000_000_000

// StandbyCatchAll is the matcher of a standby route: every domain.
const StandbyCatchAll = "regexp:."

// StandbyFlow is the VLESS flow of the reality tier.
const StandbyFlow = "xtls-rprx-vision"

// StandbyXHTTPMode is the XHTTP mode of the xhttp tier: numbered POSTs,
// which pass through any HTTP front unbuffered or not.
const StandbyXHTTPMode = "packet-up"

// standbyMember adds the active standby channels node takes part in to
// routes (as the source node) and relays (as the exit node), in the
// relay protocol vocabulary of the member "egress": protocol "vless" with
// the VLESS outbound's settings on the route and the VLESS inbound's on the
// relay (networkcore node-runtime egress.rs).
func standbyMember(db *gorm.DB, nodeID uint, routes, relays []map[string]any) ([]map[string]any, []map[string]any, error) {
	if !db.Migrator().HasTable(&model.L2Standby{}) {
		return routes, relays, nil
	}
	var rows []model.L2Standby
	if err := db.Where("active = ? AND (source_node_id = ? OR exit_node_id = ?)", true, nodeID, nodeID).
		Order("id ASC").Find(&rows).Error; err != nil {
		return nil, nil, err
	}
	for _, row := range rows {
		id := StandbyIDBase + row.ID
		if row.ExitNodeID == nodeID {
			relays = append(relays, StandbyRelay(row, id))
		}
		if row.SourceNodeID != nodeID {
			continue
		}
		host, port := row.XHTTPDomain, 443
		if row.Tier == model.L2StandbyTierReality {
			host, port = row.ExitHost, row.RelayPort
			if host == "" {
				exit, err := loadNode(db, row.ExitNodeID)
				if err != nil {
					return nil, nil, err
				}
				if exit == nil || strings.TrimSpace(exit.Host) == "" {
					continue
				}
				host = strings.TrimSpace(exit.Host)
			}
		}
		if host == "" {
			continue
		}
		routes = append(routes, map[string]any{
			"id": id, "domains": []string{StandbyCatchAll},
			"server": map[string]any{"protocol": "vless", "host": host, "port": port, "settings": StandbyRouteSettings(row)},
		})
	}
	return routes, relays, nil
}

// StandbyRelay is the exit node's relay of a standby channel.
func StandbyRelay(row model.L2Standby, id uint) map[string]any {
	user := map[string]any{"name": "standby-" + row.TransportID, "uuid": row.UUID}
	settings := map[string]any{"users": []any{user}}
	relay := map[string]any{"id": id, "protocol": "vless", "port": row.RelayPort}
	switch row.Tier {
	case model.L2StandbyTierReality:
		user["flow"] = StandbyFlow
		host, port := splitDest(row.RealityDest)
		settings["reality"] = map[string]any{
			"private_key": row.RealityPrivateKey, "short_ids": []string{row.RealityShortID},
			"server_names": []string{row.RealityServerName},
			"handshake":    map[string]any{"server": host, "server_port": port},
		}
	case model.L2StandbyTierXHTTP:
		// Behind the node's TLS front: loopback only.
		relay["listen"] = "127.0.0.1"
		settings["transport"] = map[string]any{"type": "xhttp", "path": row.XHTTPPath, "mode": StandbyXHTTPMode}
	}
	relay["settings"] = settings
	return relay
}

// StandbyRouteSettings is the VLESS outbound of the source node's route.
func StandbyRouteSettings(row model.L2Standby) map[string]any {
	if row.Tier == model.L2StandbyTierReality {
		return map[string]any{
			"uuid": row.UUID, "flow": StandbyFlow,
			"reality": map[string]any{"server_name": row.RealityServerName, "public_key": row.RealityPublicKey,
				"short_id": row.RealityShortID, "fingerprint": "chrome"},
		}
	}
	return map[string]any{
		"uuid": row.UUID,
		"tls":  map[string]any{"server_name": row.XHTTPDomain},
		"transport": map[string]any{"type": "xhttp", "path": row.XHTTPPath, "host": row.XHTTPDomain,
			"mode": StandbyXHTTPMode},
	}
}

// splitDest splits "host:port" (443 when the port is missing).
func splitDest(dest string) (string, int) {
	dest = strings.TrimSpace(dest)
	if i := strings.LastIndex(dest, ":"); i > 0 && !strings.HasSuffix(dest, "]") {
		port := 0
		for _, r := range dest[i+1:] {
			if r < '0' || r > '9' {
				return dest, 443
			}
			port = port*10 + int(r-'0')
		}
		if port > 0 && port < 65536 {
			return strings.Trim(dest[:i], "[]"), port
		}
	}
	return strings.Trim(dest, "[]"), 443
}
