package kernelforward

import (
	"crypto/rand"
	"time"
)

// crockford is the ULID alphabet (Crockford's base32).
const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// newULID answers a ULID for now: 48 bits of Unix milliseconds and 80
// random bits, 26 characters. Route ids are never reused.
func newULID(now time.Time) string {
	var id [16]byte
	ms := uint64(now.UnixMilli()) // #nosec G115 -- a time after 1970.
	for i := 5; i >= 0; i-- {
		id[i] = byte(ms)
		ms >>= 8
	}
	if _, err := rand.Read(id[6:]); err != nil {
		panic("kernel forward: no randomness for a ULID: " + err.Error())
	}
	return encodeULID(id)
}

// encodeULID writes 128 bits as 26 base32 characters, the first holding
// the top 3 bits.
func encodeULID(id [16]byte) string {
	out := make([]byte, 26)
	// Treat the 128 bits as a big number and emit 5 bits at a time from the
	// least significant end; the leading character holds 3 bits.
	var bits uint
	var acc uint32
	pos := 25
	for i := 15; i >= 0; i-- {
		acc |= uint32(id[i]) << bits
		bits += 8
		for bits >= 5 && pos >= 0 {
			out[pos] = crockford[acc&0x1f]
			acc >>= 5
			bits -= 5
			pos--
		}
	}
	if pos >= 0 {
		out[pos] = crockford[acc&0x1f]
	}
	return string(out)
}
