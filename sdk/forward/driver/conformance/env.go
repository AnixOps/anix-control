package conformance

import (
	"testing"

	"github.com/AnixOps/anix-control/sdk/forward/driver"
)

// Env is one isolated host for one scenario: an in-memory host for the
// fake driver, a set of network namespaces for the nftables and gost
// drivers (F2d). Factory makes a fresh one for every scenario; the
// factory registers its own teardown with t.Cleanup.
type Env interface {
	// NewDriver answers a driver bound to the host. Calling it again
	// simulates an Agent restart: the new instance shares the host and
	// nothing else.
	NewDriver(t testing.TB) driver.Driver
	// Owned lists the driver-owned objects now on the host, as stable
	// strings in a stable order (an nft listing of "inet anixops_fwd"
	// without counter values, the gost services). Two hosts running the
	// same artifact list the same objects; a host the driver owns nothing
	// on lists none.
	Owned(t testing.TB) []string
	// PlantForeign creates objects the driver does not own and must never
	// change (other tables, chains with the driver's chain names in other
	// tables, other services). Foreign lists them, as stable strings.
	PlantForeign(t testing.TB)
	Foreign(t testing.TB) []string
}

// Factory makes a fresh Env for one scenario.
type Factory func(t *testing.T) Env

// The optional Env interfaces below unlock scenarios; a scenario that needs
// one the Env lacks is skipped with the interface's name.

// Damager breaks the driver's owned state partially, as a crash halfway
// through a non-atomic apply (nft applied, tc not) or an operator deleting
// one object would. It must leave at least one owned object damaged or
// missing.
type Damager interface {
	Damage(t testing.TB)
}

// TrafficSource makes traffic flow through one applied hop in both
// directions, so its counters grow.
type TrafficSource interface {
	Traffic(t testing.TB, hop driver.HopKey)
}

// ApplyFaulter makes the next Apply fail inside the engine (nft -f refuses
// the transaction, gost refuses the config) after the driver started it.
type ApplyFaulter interface {
	FailNextApply(t testing.TB)
}

// ConflictPlanter makes a foreign object hold a listen port: a rule in
// another table that DNATs it, or another process bound to it.
type ConflictPlanter interface {
	PlantConflict(t testing.TB, port uint32)
}

// ImpostorPlanter creates an object with the driver's name but without its
// ownership mark (a table "inet anixops_fwd" someone else made). It is
// called before anything is applied. It must appear in Foreign.
type ImpostorPlanter interface {
	PlantImpostor(t testing.TB)
}

// ApplyCounter counts the applies that changed the host. With it the suite
// also proves that SetUpstreams and no-op applies do not run a full apply.
type ApplyCounter interface {
	Applies(t testing.TB) int
}
