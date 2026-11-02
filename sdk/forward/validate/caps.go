package validate

import (
	"time"
)

// Caps and bounds. The design fixes only the port range (1..65535); the
// other numbers are PROPOSED here and are the one place to change them (the
// defaults of model are decided, owner decision H21).
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

// Health-check and circuit-breaker bounds for values that are set
// (proposed); 0 means the default of model (H21).
const (
	MinHealthInterval = 500 * time.Millisecond
	MaxHealthInterval = time.Hour
	MinHealthTimeout  = 50 * time.Millisecond
	MaxHealthTimeout  = time.Minute
	MinBreakerOpen    = time.Second
	MaxBreakerOpen    = time.Hour
)
