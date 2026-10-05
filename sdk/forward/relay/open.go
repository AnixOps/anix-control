package relay

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"net/netip"
)

// StreamKind is the kind of connection a stream carries.
type StreamKind uint8

const (
	// StreamTCP is one client TCP connection: DATA frames in both
	// directions with half-close.
	StreamTCP StreamKind = 1
	// StreamUDP is one client UDP association: DATAGRAM frames, one per UDP
	// datagram, boundaries preserved (anixops-protocol.md section 4.8).
	StreamUDP StreamKind = 2
)

// String returns the name used in logs.
func (k StreamKind) String() string {
	switch k {
	case StreamTCP:
		return "tcp"
	case StreamUDP:
		return "udp"
	}
	return fmt.Sprintf("kind(%d)", uint8(k))
}

// MaxRouteIDLen bounds OpenParams.RouteID. Route ids are ULIDs (26
// characters); the wire format leaves room for other id shapes.
const MaxRouteIDLen = 64

// OpenParams are the parameters of an OPEN frame
// (anixops-protocol.md section 4.3). There is deliberately no destination:
// the listener sends a stream only to the upstreams its own hop names, so a
// peer cannot make it dial anything.
type OpenParams struct {
	Kind StreamKind
	// RouteID and HopIndex name the route and hop the dialler believes it
	// is feeding. The listener answers ResultRouteMismatch when they are
	// not its own, which catches a stale or crossed state.
	RouteID  string
	HopIndex uint32
	// Client is the original client address as the entry saw it, for
	// IP_HASH at later hops, PROXY protocol at the exit and diagnosis. The
	// zero value means unknown. An IPv4-mapped IPv6 address is carried as
	// IPv4 and a zone is refused.
	Client netip.AddrPort
}

// Client address families on the wire.
const (
	addrNone byte = 0
	addr4    byte = 4
	addr6    byte = 6
)

// ErrInvalidOpen reports an OPEN payload or OpenParams that cannot be sent
// or accepted.
var ErrInvalidOpen = errors.New("relay: invalid OPEN parameters")

// validate checks the fields that Marshal and Unmarshal share.
func (p OpenParams) validate() error {
	if p.Kind != StreamTCP && p.Kind != StreamUDP {
		return fmt.Errorf("%w: stream kind %d", ErrInvalidOpen, uint8(p.Kind))
	}
	if n := len(p.RouteID); n == 0 || n > MaxRouteIDLen {
		return fmt.Errorf("%w: route id of %d bytes", ErrInvalidOpen, n)
	}
	for i := range len(p.RouteID) {
		if c := p.RouteID[i]; c < 0x21 || c > 0x7e {
			return fmt.Errorf("%w: route id has a byte outside printable ASCII", ErrInvalidOpen)
		}
	}
	return nil
}

// MarshalBinary returns the OPEN payload:
//
//	| kind (8) | hop index (32) | route id length (8) | route id | family (8) | address | port (16) |
//
// family is 0 (no address or port follow), 4 (4 address bytes) or 6 (16).
func (p OpenParams) MarshalBinary() ([]byte, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	b := make([]byte, 0, 1+4+1+len(p.RouteID)+1+16+2)
	b = append(b, byte(p.Kind))
	b = binary.BigEndian.AppendUint32(b, p.HopIndex)
	n := len(p.RouteID)
	if n > math.MaxUint8 { // validate bounds it by MaxRouteIDLen already
		return nil, fmt.Errorf("%w: route id of %d bytes", ErrInvalidOpen, n)
	}
	b = append(b, byte(n))
	b = append(b, p.RouteID...)
	if !p.Client.IsValid() {
		return append(b, addrNone), nil
	}
	a := p.Client.Addr()
	if a.Zone() != "" {
		return nil, fmt.Errorf("%w: client address has a zone", ErrInvalidOpen)
	}
	if a.Is4In6() {
		a = a.Unmap()
	}
	if a.Is4() {
		a4 := a.As4()
		b = append(b, addr4)
		b = append(b, a4[:]...)
	} else {
		a16 := a.As16()
		b = append(b, addr6)
		b = append(b, a16[:]...)
	}
	return binary.BigEndian.AppendUint16(b, p.Client.Port()), nil
}

// UnmarshalBinary decodes an OPEN payload. It accepts exactly the payloads
// MarshalBinary produces: any other encoding of the same parameters, and any
// trailing byte, is an error.
func (p *OpenParams) UnmarshalBinary(b []byte) error {
	var q OpenParams
	if len(b) < 1+4+1 {
		return fmt.Errorf("%w: %d bytes", ErrInvalidOpen, len(b))
	}
	q.Kind = StreamKind(b[0])
	q.HopIndex = binary.BigEndian.Uint32(b[1:])
	n := int(b[5])
	b = b[6:]
	if len(b) < n+1 {
		return fmt.Errorf("%w: truncated", ErrInvalidOpen)
	}
	q.RouteID, b = string(b[:n]), b[n:]
	if err := q.validate(); err != nil {
		return err
	}
	family := b[0]
	b = b[1:]
	switch family {
	case addrNone:
	case addr4, addr6:
		size := 4
		if family == addr6 {
			size = 16
		}
		if len(b) != size+2 {
			return fmt.Errorf("%w: client address of %d bytes", ErrInvalidOpen, len(b))
		}
		var a netip.Addr
		if family == addr4 {
			a = netip.AddrFrom4([4]byte(b[:4]))
		} else {
			a = netip.AddrFrom16([16]byte(b[:16]))
			if a.Is4In6() {
				return fmt.Errorf("%w: IPv4-mapped client address sent as IPv6", ErrInvalidOpen)
			}
		}
		q.Client = netip.AddrPortFrom(a, binary.BigEndian.Uint16(b[size:]))
		b = nil
	default:
		return fmt.Errorf("%w: address family %d", ErrInvalidOpen, family)
	}
	if len(b) != 0 {
		return fmt.Errorf("%w: %d trailing bytes", ErrInvalidOpen, len(b))
	}
	*p = q
	return nil
}
