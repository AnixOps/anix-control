package relay

import (
	"fmt"
	"time"
)

// Defaults of the timers (anixops-protocol.md sections 2.5 and 4.6).
const (
	// DefaultPingInterval is how long a carrier may receive nothing before
	// it sends a PING (L4).
	DefaultPingInterval = 10 * time.Second
	// DefaultIdleTimeout is how long a carrier may receive nothing before it
	// is closed (L4); a write that makes no progress for as long fails the
	// carrier too, the application-level counterpart of TCP_USER_TIMEOUT.
	DefaultIdleTimeout = 30 * time.Second
	// DefaultResultTimeout is how long an OPEN may go unanswered on a
	// carrier that answers pings before the carrier is retired (L5).
	DefaultResultTimeout = 5 * time.Second
	// DefaultHandshakeTimeout bounds the SETTINGS exchange (and, with the
	// TLS carrier, the TLS handshake before it).
	DefaultHandshakeTimeout = 10 * time.Second
	// DefaultDrainTimeout is how long a closing listener lets the carriers
	// it accepted drain (L1).
	DefaultDrainTimeout = 5 * time.Second

	// DefaultAcceptQueue is how many streams the peer may have opened that
	// the application has not taken yet.
	DefaultAcceptQueue = 128
	// DefaultSendBuffer is how many bytes a stream buffers ahead of its
	// credit before Write blocks.
	DefaultSendBuffer = 64 * 1024

	// closeGrace is how long a failing carrier tries to flush its final
	// frames before the connection is closed regardless.
	closeGrace = time.Second
)

// Config configures a Carrier. The zero value of every field means its
// default, so Config{} is the documented configuration.
type Config struct {
	// MaxStreams is the number of concurrent streams the listening end
	// accepts. It is ignored on the dialling end, which accepts none.
	// Default 1024, at most 65536.
	MaxStreams uint32
	// MaxFrame is the largest frame payload this end accepts: 1024 to
	// 65535, default 16 KiB.
	MaxFrame uint32
	// StreamWindow is the credit granted for each stream: 4 KiB to 16 MiB,
	// default 256 KiB.
	StreamWindow uint32
	// CarrierWindow is the credit granted for the carrier, which bounds the
	// bytes it buffers: 4 KiB to 64 MiB, default 1 MiB.
	CarrierWindow uint32

	// AcceptQueue bounds the streams opened by the peer and not yet taken
	// with Accept; further OPENs are refused. Default 128.
	AcceptQueue int
	// SendBuffer bounds the bytes one stream buffers ahead of its credit.
	// Default 64 KiB.
	SendBuffer int

	// HandshakeTimeout, PingInterval, IdleTimeout, ResultTimeout and
	// DrainTimeout: see the Default constants. A negative ResultTimeout
	// turns L5 off.
	HandshakeTimeout time.Duration
	PingInterval     time.Duration
	IdleTimeout      time.Duration
	ResultTimeout    time.Duration
	DrainTimeout     time.Duration
}

// withDefaults fills the zero fields and validates the result. role fixes
// the stream limit: a dialling end accepts none.
func (c Config) withDefaults(role Role) (Config, error) {
	if c.MaxStreams == 0 {
		c.MaxStreams = DefaultMaxStreams
	}
	if c.MaxFrame == 0 {
		c.MaxFrame = DefaultMaxFrame
	}
	if c.StreamWindow == 0 {
		c.StreamWindow = DefaultStreamWindow
	}
	if c.CarrierWindow == 0 {
		c.CarrierWindow = DefaultCarrierWindow
	}
	if c.AcceptQueue == 0 {
		c.AcceptQueue = DefaultAcceptQueue
	}
	if c.SendBuffer == 0 {
		c.SendBuffer = DefaultSendBuffer
	}
	if c.HandshakeTimeout == 0 {
		c.HandshakeTimeout = DefaultHandshakeTimeout
	}
	if c.PingInterval == 0 {
		c.PingInterval = DefaultPingInterval
	}
	if c.IdleTimeout == 0 {
		c.IdleTimeout = DefaultIdleTimeout
	}
	if c.ResultTimeout == 0 {
		c.ResultTimeout = DefaultResultTimeout
	}
	if c.DrainTimeout == 0 {
		c.DrainTimeout = DefaultDrainTimeout
	}
	if role != RoleDialer && role != RoleAcceptor {
		return c, fmt.Errorf("relay: invalid role %d", role)
	}
	if err := c.settings(role).Validate(); err != nil {
		return c, fmt.Errorf("relay: config: %w", err)
	}
	switch {
	case c.AcceptQueue < 0 || c.SendBuffer < 0:
		return c, fmt.Errorf("relay: config: queue and buffer sizes cannot be negative")
	case c.HandshakeTimeout < 0 || c.PingInterval < 0 || c.IdleTimeout < 0 || c.DrainTimeout < 0:
		return c, fmt.Errorf("relay: config: timeouts cannot be negative")
	case c.PingInterval >= c.IdleTimeout:
		return c, fmt.Errorf("relay: config: ping interval %s must be shorter than the idle timeout %s", c.PingInterval, c.IdleTimeout)
	}
	return c, nil
}

// settings returns what this end announces in SETTINGS.
func (c Config) settings(role Role) Settings {
	s := Settings{MaxStreams: c.MaxStreams, MaxFrame: c.MaxFrame, StreamWindow: c.StreamWindow, CarrierWindow: c.CarrierWindow}
	if role == RoleDialer {
		s.MaxStreams = 0
	}
	return s
}
