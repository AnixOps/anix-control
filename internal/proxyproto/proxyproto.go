// Package proxyproto writes and reads the binary PROXY protocol header,
// version 2 (HAProxy's "PROXY protocol" specification, section 2.2), for
// TCP over IPv4 and IPv6.
//
// The region relay (cmd/anix-relay, docs/design/region-relay) forwards a
// node's TCP connection to Control byte for byte and puts this header in
// front of it, so Control still sees the node's own address. Control reads
// the header only on connections from the relays it is configured to trust
// (grpc.relay_sources): from any other peer a header would be a forged
// address, so those connections are never parsed.
package proxyproto

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
)

// signature opens every version 2 header.
var signature = []byte{0x0D, 0x0A, 0x0D, 0x0A, 0x00, 0x0D, 0x0A, 0x51, 0x55, 0x49, 0x54, 0x0A}

const (
	// headerLength is the signature, the version and command, the family
	// and the length of the address block.
	headerLength = 16

	versionCommandLocal = 0x20
	versionCommandProxy = 0x21

	familyTCP4 = 0x11
	familyTCP6 = 0x21

	addressLengthTCP4 = 12
	addressLengthTCP6 = 36

	// MaxAddressBlock bounds the address block a reader accepts: the
	// addresses plus any TLVs. A relay sends none; a peer that claims more
	// is refused rather than read into memory.
	MaxAddressBlock = 512
)

// ErrNoHeader is returned when the stream does not open with a version 2
// header.
var ErrNoHeader = errors.New("proxyproto: the connection does not start with a PROXY v2 header")

// Header is a decoded header. Local is a LOCAL command (the relay's own
// connection, for example a health check): it carries no addresses, and the
// connection's own addresses stand.
type Header struct {
	Local       bool
	Source      *net.TCPAddr
	Destination *net.TCPAddr
}

// Encode returns the header that announces a TCP connection from source to
// destination. Both must be of the same family; an IPv4-mapped IPv6 address
// counts as IPv4.
func Encode(source, destination *net.TCPAddr) ([]byte, error) {
	if source == nil || destination == nil {
		return nil, errors.New("proxyproto: source and destination are required")
	}
	if !validPort(source.Port) || !validPort(destination.Port) {
		return nil, fmt.Errorf("proxyproto: port out of range")
	}
	out := make([]byte, 0, headerLength+addressLengthTCP6)
	out = append(out, signature...)
	out = append(out, versionCommandProxy)
	source4, destination4 := source.IP.To4(), destination.IP.To4()
	switch {
	case source4 != nil && destination4 != nil:
		out = append(out, familyTCP4)
		out = binary.BigEndian.AppendUint16(out, addressLengthTCP4)
		out = append(out, source4...)
		out = append(out, destination4...)
	case source4 == nil && destination4 == nil && source.IP.To16() != nil && destination.IP.To16() != nil:
		out = append(out, familyTCP6)
		out = binary.BigEndian.AppendUint16(out, addressLengthTCP6)
		out = append(out, source.IP.To16()...)
		out = append(out, destination.IP.To16()...)
	default:
		return nil, fmt.Errorf("proxyproto: %s and %s are not of one address family", source.IP, destination.IP)
	}
	out = binary.BigEndian.AppendUint16(out, uint16(source.Port))      // #nosec G115 -- validPort bounds it to 16 bits above.
	out = binary.BigEndian.AppendUint16(out, uint16(destination.Port)) // #nosec G115 -- validPort bounds it to 16 bits above.
	return out, nil
}

// EncodeLocal returns a LOCAL header: the connection is the sender's own.
func EncodeLocal() []byte {
	out := append([]byte(nil), signature...)
	out = append(out, versionCommandLocal, 0x00, 0x00, 0x00)
	return out
}

func validPort(port int) bool { return port >= 0 && port <= 0xFFFF }

// Read reads one header from r and nothing after it. It refuses anything
// that is not a well-formed version 2 header of a TCP connection or a LOCAL
// command: a relay that announces something else is misconfigured.
func Read(r io.Reader) (Header, error) {
	fixed := make([]byte, headerLength)
	if _, err := io.ReadFull(r, fixed); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
			return Header{}, ErrNoHeader
		}
		return Header{}, err
	}
	if !bytes.Equal(fixed[:len(signature)], signature) {
		return Header{}, ErrNoHeader
	}
	length := int(binary.BigEndian.Uint16(fixed[14:16]))
	if length > MaxAddressBlock {
		return Header{}, fmt.Errorf("proxyproto: address block of %d bytes is over %d", length, MaxAddressBlock)
	}
	block := make([]byte, length)
	if _, err := io.ReadFull(r, block); err != nil {
		return Header{}, fmt.Errorf("proxyproto: truncated address block: %w", err)
	}
	switch fixed[12] {
	case versionCommandLocal:
		return Header{Local: true}, nil
	case versionCommandProxy:
	default:
		return Header{}, fmt.Errorf("proxyproto: version and command 0x%02x is not v2 LOCAL or PROXY", fixed[12])
	}
	switch fixed[13] {
	case familyTCP4:
		if length < addressLengthTCP4 {
			return Header{}, fmt.Errorf("proxyproto: IPv4 address block of %d bytes", length)
		}
		return Header{
			Source:      &net.TCPAddr{IP: net.IP(append([]byte(nil), block[0:4]...)), Port: int(binary.BigEndian.Uint16(block[8:10]))},
			Destination: &net.TCPAddr{IP: net.IP(append([]byte(nil), block[4:8]...)), Port: int(binary.BigEndian.Uint16(block[10:12]))},
		}, nil
	case familyTCP6:
		if length < addressLengthTCP6 {
			return Header{}, fmt.Errorf("proxyproto: IPv6 address block of %d bytes", length)
		}
		return Header{
			Source:      &net.TCPAddr{IP: net.IP(append([]byte(nil), block[0:16]...)), Port: int(binary.BigEndian.Uint16(block[32:34]))},
			Destination: &net.TCPAddr{IP: net.IP(append([]byte(nil), block[16:32]...)), Port: int(binary.BigEndian.Uint16(block[34:36]))},
		}, nil
	default:
		return Header{}, fmt.Errorf("proxyproto: family 0x%02x is not TCP over IPv4 or IPv6", fixed[13])
	}
}
