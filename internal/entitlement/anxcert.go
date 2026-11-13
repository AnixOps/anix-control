package entitlement

import (
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strings"
)

// Device key algorithms (anx spec §2.4).
const (
	AlgP256    byte = 1
	AlgEd25519 byte = 2
)

const (
	certVersion = 1
	certInfo    = "anx/v1/device-cert"
)

// ErrBadDevice is a malformed X-AnixOps-Device header.
var ErrBadDevice = errors.New("entitlement: device must be ed25519:<64 hex> or p256:<130 hex>")

// Device is a device's public key.
type Device struct {
	Alg       byte
	PublicKey []byte
}

// ParseDevice parses "<alg>:<hex public key>": ed25519 with a 32-byte key,
// p256 with the 65-byte uncompressed SEC1 point.
func ParseDevice(text string) (Device, error) {
	name, keyHex, ok := strings.Cut(strings.TrimSpace(text), ":")
	if !ok {
		return Device{}, ErrBadDevice
	}
	pk, err := hex.DecodeString(strings.TrimSpace(keyHex))
	if err != nil {
		return Device{}, ErrBadDevice
	}
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "ed25519":
		if len(pk) != ed25519.PublicKeySize {
			return Device{}, ErrBadDevice
		}
		return Device{Alg: AlgEd25519, PublicKey: pk}, nil
	case "p256":
		if len(pk) != 65 {
			return Device{}, ErrBadDevice
		}
		if _, err := ecdh.P256().NewPublicKey(pk); err != nil {
			return Device{}, ErrBadDevice
		}
		return Device{Alg: AlgP256, PublicKey: pk}, nil
	}
	return Device{}, ErrBadDevice
}

// AlgName is the device's algorithm as the header names it.
func (d Device) AlgName() string {
	if d.Alg == AlgP256 {
		return "p256"
	}
	return "ed25519"
}

// ID is the device_id: SHA-256(alg ‖ public key)[..8].
func (d Device) ID() [8]byte {
	h := sha256.New()
	h.Write([]byte{d.Alg})
	h.Write(d.PublicKey)
	var id [8]byte
	copy(id[:], h.Sum(nil))
	return id
}

// ANXCert builds and signs the compact device certificate of anx spec §2.4:
//
//	v(1)=1 ‖ device_id(8) ‖ user_id(8) ‖ alg(1) ‖ pk_len(1) ‖ pk ‖
//	not_before(4) ‖ not_after(4) ‖ ca_sig(64)
//
// with ca_sig = Ed25519(ca, "anx/v1/device-cert" ‖ everything before it).
func ANXCert(ca ed25519.PrivateKey, userID [8]byte, d Device, notBefore, notAfter uint32) []byte {
	id := d.ID()
	tbs := make([]byte, 0, 1+8+8+2+len(d.PublicKey)+8+ed25519.SignatureSize)
	tbs = append(tbs, certVersion)
	tbs = append(tbs, id[:]...)
	tbs = append(tbs, userID[:]...)
	tbs = append(tbs, d.Alg, byte(len(d.PublicKey))) // #nosec G115 -- ParseDevice admits 32- or 65-byte keys only.
	tbs = append(tbs, d.PublicKey...)
	tbs = binary.BigEndian.AppendUint32(tbs, notBefore)
	tbs = binary.BigEndian.AppendUint32(tbs, notAfter)
	msg := append([]byte(certInfo), tbs...)
	return append(tbs, ed25519.Sign(ca, msg)...)
}
