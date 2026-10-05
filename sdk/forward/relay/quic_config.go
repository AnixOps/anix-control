package relay

import (
	"fmt"

	"github.com/quic-go/quic-go"
)

// QUICConfig configures a QUIC carrier. The embedded Config means what it
// means for the carrier over a net.Conn, mapped onto QUIC's own mechanisms:
//
//	MaxStreams        MaxIncomingStreams of the listening end (the dialling end
//	                  accepts none)
//	MaxFrame          the largest UDP datagram carried on a stream (QUIC has no
//	                  frames of its own to bound)
//	StreamWindow      the initial stream flow control window
//	CarrierWindow     the initial connection flow control window
//	PingInterval      the QUIC keepalive period (L4)
//	IdleTimeout       the QUIC idle timeout (L4)
//	HandshakeTimeout  the deadline of the QUIC handshake and of the SETTINGS
//	                  exchange after it, and the time a peer has to send OPEN on
//	                  a stream it opened
//	ResultTimeout     L5, as for the TCP carrier
//	DrainTimeout      L1, as for the TCP carrier
//	AcceptQueue       as for the TCP carrier
//	SendBuffer        the bytes a UDP association queues for its stream (the
//	                  QUIC stream buffers TCP data itself)
//
// The windows grow with the measured bandwidth-delay product, which is QUIC's
// own auto-tuning (anixops-protocol.md section 4.4): from the configured
// initial windows up to the ceilings below.
type QUICConfig struct {
	Config

	// StreamWindowCeiling and CarrierWindowCeiling are the largest windows
	// QUIC may grow to. They default to the document's 16 MiB and 64 MiB, and
	// are never below the initial windows. The carrier window ceiling bounds
	// what a carrier buffers (section 4.5); set both to the initial windows
	// for fixed windows.
	StreamWindowCeiling  uint32
	CarrierWindowCeiling uint32

	// DisableDatagrams makes UDP ride the association's stream, as it does on
	// a QUIC connection whose peer does not support DATAGRAM frames (section
	// 4.8). The default uses native datagrams when both ends enabled them.
	DisableDatagrams bool

	// StatelessResetKey is used by listeners only: see
	// link.QUICListenerConfig.StatelessResetKey. The caller keeps it in the
	// relay's state directory; this library never writes one.
	StatelessResetKey *[32]byte
}

// withDefaults fills the zero fields and validates the result for role.
func (c QUICConfig) withDefaults(role Role) (QUICConfig, error) {
	cfg, err := c.Config.withDefaults(role)
	if err != nil {
		return c, err
	}
	c.Config = cfg
	if c.StreamWindowCeiling == 0 {
		c.StreamWindowCeiling = MaxStreamWindow
	}
	if c.CarrierWindowCeiling == 0 {
		c.CarrierWindowCeiling = MaxCarrierWindow
	}
	c.StreamWindowCeiling = max(c.StreamWindowCeiling, c.StreamWindow)
	c.CarrierWindowCeiling = max(c.CarrierWindowCeiling, c.CarrierWindow)
	if c.StreamWindowCeiling > MaxStreamWindow || c.CarrierWindowCeiling > MaxCarrierWindow {
		return c, fmt.Errorf("relay: config: window ceilings %d and %d exceed %d and %d",
			c.StreamWindowCeiling, c.CarrierWindowCeiling, MaxStreamWindow, MaxCarrierWindow)
	}
	return c, nil
}

// Transport returns the quic.Config a connection for role must be made with:
// the limits and timers of the carrier mapped onto QUIC's transport
// parameters, datagrams enabled unless disabled, and one unidirectional
// stream allowed from the peer, which is its control stream. The link layer
// forces everything that is a security decision (QUIC version 1, no 0-RTT).
// DialQUIC and ListenQUIC use it; it is exported for callers that make the
// QUIC connection themselves and run NewQUICCarrier on it.
func (c QUICConfig) Transport(role Role) (*quic.Config, error) {
	cfg, err := c.withDefaults(role)
	if err != nil {
		return nil, err
	}
	maxStreams := int64(cfg.MaxStreams)
	if role == RoleDialer {
		maxStreams = -1 // a dialler accepts no streams: only it opens them
	}
	return &quic.Config{
		MaxIdleTimeout:                 cfg.IdleTimeout,
		KeepAlivePeriod:                cfg.PingInterval,
		InitialStreamReceiveWindow:     uint64(cfg.StreamWindow),
		MaxStreamReceiveWindow:         uint64(cfg.StreamWindowCeiling),
		InitialConnectionReceiveWindow: uint64(cfg.CarrierWindow),
		MaxConnectionReceiveWindow:     uint64(cfg.CarrierWindowCeiling),
		MaxIncomingStreams:             maxStreams,
		MaxIncomingUniStreams:          1, // the peer's control stream
		EnableDatagrams:                !cfg.DisableDatagrams,
	}, nil
}
