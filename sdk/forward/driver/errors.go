package driver

import (
	"errors"
	"fmt"
	"strings"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
)

// Sentinel errors. Drivers wrap them with detail; callers test with
// errors.Is.
var (
	// ErrUnsupported: the hop or call needs a feature this driver or host
	// does not offer (a link security, a strategy, UDP, IPv6, a limit, an
	// unknown enum value).
	ErrUnsupported = errors.New("forward driver: unsupported")
	// ErrInvalidState: the state is malformed in a way validation should
	// have caught (a nil state, a hop twice, a missing listen port).
	ErrInvalidState = errors.New("forward driver: invalid state")
	// ErrInvalidArtifact: the artifact's digest does not match its content,
	// or its content cannot be read.
	ErrInvalidArtifact = errors.New("forward driver: invalid artifact")
	// ErrEngineMismatch: the artifact was rendered by another engine's
	// driver.
	ErrEngineMismatch = errors.New("forward driver: artifact engine mismatch")
	// ErrStaleGeneration: the artifact's generation is older than the one
	// the host runs. The contract's FAILED_PRECONDITION.
	ErrStaleGeneration = errors.New("forward driver: stale generation")
	// ErrGenerationConflict: the artifact has the generation the host runs
	// but a different digest. Generations never repeat with other content,
	// so this is a bug upstream; the host is left alone.
	ErrGenerationConflict = errors.New("forward driver: same generation, different content")
	// ErrConflict: applying would collide with an object the driver does
	// not own (another table's rule or another process holding a listen
	// port). Neither side is changed.
	ErrConflict = errors.New("forward driver: conflict with a foreign object")
	// ErrNotOwned: an object has the driver's name but not its ownership
	// mark (a table "inet anixops_fwd" someone else created). Apply refuses
	// to touch it; Remove leaves it in place and succeeds.
	ErrNotOwned = errors.New("forward driver: object not owned by the driver")
	// ErrNotFound: SetUpstreams named a hop the host does not run, or an
	// upstream the hop was not rendered with.
	ErrNotFound = errors.New("forward driver: not found")
	// ErrInvalidArgument: a call's arguments are malformed (SetUpstreams
	// with no upstream, or one twice).
	ErrInvalidArgument = errors.New("forward driver: invalid argument")
	// ErrDuplicateEngine: a Registry already has a driver for the engine.
	ErrDuplicateEngine = errors.New("forward driver: engine already registered")
)

// HopError is one hop a driver could not render or run. Err wraps one of the
// sentinel errors.
type HopError struct {
	Key    HopKey
	Engine forwardv1.Engine
	Err    error
}

func (e *HopError) Error() string {
	return fmt.Sprintf("route %s hop %d (%s): %v", e.Key.RouteID, e.Key.HopIndex, e.Engine, e.Err)
}

func (e *HopError) Unwrap() error { return e.Err }

// ToProto converts the error for NodeForwardReport and ApplyResult.
func (e *HopError) ToProto() *forwardv1.HopError {
	return &forwardv1.HopError{
		RouteId:  e.Key.RouteID,
		HopIndex: e.Key.HopIndex,
		Engine:   e.Engine,
		Message:  e.Err.Error(),
	}
}

// RenderError is the error Render returns next to a usable artifact when
// it left some hops out. errors.Is sees through it to every hop's error.
type RenderError struct {
	Hops []*HopError
}

func (e *RenderError) Error() string {
	parts := make([]string, len(e.Hops))
	for i, h := range e.Hops {
		parts[i] = h.Error()
	}
	return fmt.Sprintf("forward driver: %d hop(s) not rendered: %s", len(e.Hops), strings.Join(parts, "; "))
}

// Unwrap answers the hop errors for errors.Is and errors.As.
func (e *RenderError) Unwrap() []error {
	errs := make([]error, len(e.Hops))
	for i, h := range e.Hops {
		errs[i] = h
	}
	return errs
}

// ToProto converts every hop error.
func (e *RenderError) ToProto() []*forwardv1.HopError {
	out := make([]*forwardv1.HopError, len(e.Hops))
	for i, h := range e.Hops {
		out[i] = h.ToProto()
	}
	return out
}

// NewRenderError answers nil for no hop errors (so a nil *RenderError never
// becomes a non-nil error), and a *RenderError otherwise.
func NewRenderError(hops []*HopError) error {
	if len(hops) == 0 {
		return nil
	}
	return &RenderError{Hops: hops}
}
