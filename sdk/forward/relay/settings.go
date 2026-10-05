package relay

import (
	"encoding/binary"
	"fmt"
)

// SETTINGS keys. A SETTINGS payload is a list of (u16 key, u32 value)
// pairs; keys this version does not know are ignored, so optional features
// are added without a new wire version (anixops-protocol.md section 6.7).
const (
	// SettingMaxStreams is the number of concurrent streams the sender
	// accepts from its peer. A dialler never accepts streams, so it sends 0.
	SettingMaxStreams uint16 = 0x1
	// SettingMaxFrame is the largest frame payload the sender accepts.
	SettingMaxFrame uint16 = 0x2
	// SettingStreamWindow is the credit the sender grants its peer for each
	// new stream: how many DATA bytes may be in flight before a WINDOW.
	SettingStreamWindow uint16 = 0x3
	// SettingCarrierWindow is the credit the sender grants its peer for the
	// whole carrier, shared by all streams.
	SettingCarrierWindow uint16 = 0x4
)

// Defaults and hard bounds of the SETTINGS values
// (anixops-protocol.md sections 4.4 and 4.5).
const (
	DefaultMaxStreams    = 1024
	DefaultStreamWindow  = 256 * 1024
	DefaultCarrierWindow = 1024 * 1024

	// MaxMaxStreams caps SETTINGS_MAX_STREAMS: the node-wide cap of open
	// streams (section 4.5) is also 65536.
	MaxMaxStreams = 65536
	// MinWindow is the smallest window a receiver may grant.
	MinWindow = 4096
	// MaxStreamWindow and MaxCarrierWindow are the largest windows: the
	// sizes the document grows them to when the bandwidth-delay product
	// needs it.
	MaxStreamWindow  = 16 * 1024 * 1024
	MaxCarrierWindow = 64 * 1024 * 1024

	// maxCreditValue is the largest credit a sender may hold for a stream
	// or the carrier; a WINDOW that would take it past this is a
	// flow-control error (as in HTTP/2).
	maxCreditValue = 1<<31 - 1
)

// Settings are the receive-side limits one end of a carrier announces in its
// first frame. Both ends know the other's before the first stream opens, so
// nothing is assumed about a peer.
type Settings struct {
	MaxStreams    uint32
	MaxFrame      uint32
	StreamWindow  uint32
	CarrierWindow uint32
}

// DefaultSettings returns the defaults of anixops-protocol.md section 4.5.
func DefaultSettings() Settings {
	return Settings{
		MaxStreams:    DefaultMaxStreams,
		MaxFrame:      DefaultMaxFrame,
		StreamWindow:  DefaultStreamWindow,
		CarrierWindow: DefaultCarrierWindow,
	}
}

// Validate reports a value outside its hard bounds.
func (s Settings) Validate() error {
	switch {
	case s.MaxStreams > MaxMaxStreams:
		return fmt.Errorf("max streams %d exceeds %d", s.MaxStreams, MaxMaxStreams)
	case s.MaxFrame < MinMaxFrame || s.MaxFrame > MaxMaxFrame:
		return fmt.Errorf("max frame %d is not in %d..%d", s.MaxFrame, MinMaxFrame, MaxMaxFrame)
	case s.StreamWindow < MinWindow || s.StreamWindow > MaxStreamWindow:
		return fmt.Errorf("stream window %d is not in %d..%d", s.StreamWindow, MinWindow, MaxStreamWindow)
	case s.CarrierWindow < MinWindow || s.CarrierWindow > MaxCarrierWindow:
		return fmt.Errorf("carrier window %d is not in %d..%d", s.CarrierWindow, MinWindow, MaxCarrierWindow)
	}
	return nil
}

// Marshal returns the SETTINGS payload: every key, in key order.
func (s Settings) Marshal() []byte {
	b := make([]byte, 0, 4*6)
	for _, kv := range []struct {
		key uint16
		val uint32
	}{
		{SettingMaxStreams, s.MaxStreams},
		{SettingMaxFrame, s.MaxFrame},
		{SettingStreamWindow, s.StreamWindow},
		{SettingCarrierWindow, s.CarrierWindow},
	} {
		b = binary.BigEndian.AppendUint16(b, kv.key)
		b = binary.BigEndian.AppendUint32(b, kv.val)
	}
	return b
}

// ParseSettings decodes a SETTINGS payload. A key that is missing keeps its
// default, a key this version does not know is ignored, a known key given
// twice or a value out of its bounds is an error, and so is a payload that
// is not a whole number of pairs.
func ParseSettings(b []byte) (Settings, error) {
	if len(b)%6 != 0 {
		return Settings{}, fmt.Errorf("settings payload of %d bytes is not a whole number of pairs", len(b))
	}
	s := DefaultSettings()
	var seen [5]bool
	for ; len(b) > 0; b = b[6:] {
		key, val := binary.BigEndian.Uint16(b), binary.BigEndian.Uint32(b[2:])
		if key >= 1 && int(key) < len(seen) {
			if seen[key] {
				return Settings{}, fmt.Errorf("setting 0x%x given twice", key)
			}
			seen[key] = true
		}
		switch key {
		case SettingMaxStreams:
			s.MaxStreams = val
		case SettingMaxFrame:
			s.MaxFrame = val
		case SettingStreamWindow:
			s.StreamWindow = val
		case SettingCarrierWindow:
			s.CarrierWindow = val
		}
	}
	if err := s.Validate(); err != nil {
		return Settings{}, err
	}
	return s, nil
}
