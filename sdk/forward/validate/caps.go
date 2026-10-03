package validate

import (
	"time"

	"github.com/AnixOps/anix-control/sdk/forward/model"
)

// Caps and bounds. The design fixes only the port range (1..65535); the
// other numbers are PROPOSED here (with the defaults of model, owner
// decision H21) and are the one place to change them.
const (
	// MaxPort is the largest port.
	MaxPort = 65535
	// MaxRouteBytes caps a route's protobuf encoding (proposed).
	MaxRouteBytes = 64 << 10
	// MaxHops caps a route's hops: entry, up to six relays, exit (proposed).
	MaxHops = 8
	// MaxNodesPerHop caps a hop's nodes (proposed).
	MaxNodesPerHop = 16
	// MaxTargets caps a route's targets (proposed).
	MaxTargets = 64
	// MaxWeight caps a target's weight; the nftables driver turns weights
	// into intervals of a numgen map (proposed).
	MaxWeight = 1000
	// MaxNameRunes caps Route.name (proposed).
	MaxNameRunes = 128
	// MaxLabels caps Route.labels (proposed).
	MaxLabels = 32
	// MaxLabelKeyBytes and MaxLabelValueBytes cap a label (proposed).
	MaxLabelKeyBytes   = 63
	MaxLabelValueBytes = 255
	// MaxPathBytes caps LinkTransport.path (proposed).
	MaxPathBytes = 256
	// MaxHostnameBytes is the longest DNS name (RFC 1035).
	MaxHostnameBytes = 253
	// MaxFailureThreshold caps CircuitBreaker.failure_threshold (proposed).
	MaxFailureThreshold = 100
)

// Health-check and circuit-breaker bounds for values that are set; 0 means
// the default of model (proposed, H21).
const (
	MinHealthInterval = 500 * time.Millisecond
	MaxHealthInterval = time.Hour
	MinHealthTimeout  = 50 * time.Millisecond
	MaxHealthTimeout  = time.Minute
	MinBreakerOpen    = time.Second
	MaxBreakerOpen    = time.Hour
)

// engineLinks are the link securities each engine can originate and
// terminate (forward-sdk.md section 4.2).
var engineLinks = map[model.Engine][]model.LinkSecurity{
	model.EngineNFTables: {model.LinkSecurityRaw},
	model.EngineGost: {
		model.LinkSecurityRaw, model.LinkSecurityTLS, model.LinkSecurityWSS,
		model.LinkSecurityQUIC, model.LinkSecurityGRPC,
	},
	model.EngineAnixOps: {model.LinkSecurityAnixOps},
}

// EngineLinks answers the link securities engine e can originate and
// terminate; none for an unknown engine.
func EngineLinks(e model.Engine) []model.LinkSecurity {
	return append([]model.LinkSecurity(nil), engineLinks[e]...)
}

// CanCarry reports whether engine e can originate and terminate a link with
// security s.
func CanCarry(e model.Engine, s model.LinkSecurity) bool {
	for _, l := range engineLinks[e] {
		if l == s {
			return true
		}
	}
	return false
}

// CanMux reports whether engine e can multiplex a link: gost (mtls, mwss
// and friends) and the AnixOps protocol; nftables cannot.
func CanMux(e model.Engine) bool {
	return e == model.EngineGost || e == model.EngineAnixOps
}
