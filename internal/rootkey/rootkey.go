// Package rootkey holds Control's root key: 32 random bytes in the file
// root.key of the data directory, the one secret every other key of Control
// is sealed with or derived from (docs/reference/configuration.md, "The root
// key").
//
//   - It seals the module CA's and the forward link CA's private keys, the
//     DNS provider credentials and, through HKDF-SHA256 with their own
//     labels, the node group join tokens ("anixops node-group join token v1")
//     and the ACME account and certificate keys ("anixops acme cache v1"). In
//     memory it stands where module_runtime.ca_kek stood, so every reader of
//     that key keeps working unchanged.
//   - The JWT secret and the identity signing key KEK are derived from it
//     with HKDF-SHA256 (LabelJWT, LabelIdentityKEK), unless an earlier
//     configuration set them: such a value is adopted, kept in root.key and
//     used until a rotation.
//
// The file is text, versioned by its first field:
//
//	# AnixOps Control root key. Keep it secret; back it up with the database.
//	anixops-root-key: v1
//	key: <the 32 bytes, standard base64>
//	adopted-jwt-secret: <the jwt.secret it adopted, standard base64 of its bytes>
//	adopted-identity-kek: <the identity.kek it adopted, standard base64 of its 32 bytes>
//
// The two adopted fields are optional. Lines starting with # and empty lines
// are ignored; any other field, a repeated field or another version is
// refused. The file is mode 0600 in a 0700 directory, owned by the user
// Control runs as. Nothing here ever logs or prints a key: Key formats as
// "[root key]".
package rootkey

import (
	"bufio"
	"bytes"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// Size is the length of the root key in bytes.
const Size = 32

// FileName is the root key's file in the data directory.
const FileName = "root.key"

// Version is the format version this Control writes and reads.
const Version = "v1"

// Derivation labels (HKDF-SHA256 info, no salt, 32 bytes). They are part of
// the stored data's format: changing one makes everything derived with it
// unreadable, so a new label gets a new version suffix instead.
const (
	// LabelJWT derives the kernel's HS256 JWT secret.
	LabelJWT = "anixops jwt v1"
	// LabelIdentityKEK derives the KEK that seals the identity signing keys.
	LabelIdentityKEK = "anixops identity kek v1"
	// LabelJoinToken derives the key that seals node group join tokens
	// (internal/nodegroup, unchanged since it was keyed by
	// module_runtime.ca_kek).
	LabelJoinToken = "anixops node-group join token v1" // #nosec G101 -- an HKDF label, not a credential.
	// LabelACMECache derives the key that seals the ACME cache entries
	// holding a private key (internal/panelcert).
	LabelACMECache = "anixops acme cache v1"
)

const (
	fieldVersion     = "anixops-root-key"
	fieldKey         = "key"
	fieldJWTSecret   = "adopted-jwt-secret"
	fieldIdentityKEK = "adopted-identity-kek"
)

// Key is a root key. Its formatting never shows the bytes.
type Key struct {
	bytes [Size]byte
}

// String hides the key.
func (Key) String() string { return "[root key]" }

// GoString hides the key.
func (Key) GoString() string { return "rootkey.Key{[root key]}" }

// Bytes returns a copy of the key.
func (k Key) Bytes() []byte {
	out := make([]byte, Size)
	copy(out, k.bytes[:])
	return out
}

// Base64 is the key as module_runtime.ca_kek held it (standard base64), the
// form every existing reader of that key parses.
func (k Key) Base64() string { return base64.StdEncoding.EncodeToString(k.bytes[:]) }

// IsZero tells whether k is the zero key (no key).
func (k Key) IsZero() bool {
	var zero [Size]byte
	return subtle.ConstantTimeCompare(k.bytes[:], zero[:]) == 1
}

// Equal compares two keys in constant time.
func (k Key) Equal(other Key) bool {
	return subtle.ConstantTimeCompare(k.bytes[:], other.bytes[:]) == 1
}

// Derive returns the 32 bytes HKDF-SHA256 derives from k for label.
func (k Key) Derive(label string) []byte {
	derived, err := hkdf.Key(sha256.New, k.bytes[:], nil, label, Size)
	if err != nil {
		// hkdf.Key fails only for lengths above 255 hash blocks.
		panic(fmt.Sprintf("rootkey: derive %q: %v", label, err))
	}
	return derived
}

// DerivedJWTSecret is the JWT secret derived from k: the standard base64 of
// the LabelJWT derivation (44 characters).
func (k Key) DerivedJWTSecret() string {
	return base64.StdEncoding.EncodeToString(k.Derive(LabelJWT))
}

// DerivedIdentityKEK is the identity signing key KEK derived from k, in the
// form identity.kek takes (standard base64 of 32 bytes).
func (k Key) DerivedIdentityKEK() string {
	return base64.StdEncoding.EncodeToString(k.Derive(LabelIdentityKEK))
}

// Generate returns a new random root key.
func Generate() (Key, error) {
	var key Key
	if _, err := rand.Read(key.bytes[:]); err != nil {
		return Key{}, fmt.Errorf("generate the root key: %w", err)
	}
	return key, nil
}

// FromBytes returns the key of 32 bytes.
func FromBytes(raw []byte) (Key, error) {
	if len(raw) != Size {
		return Key{}, fmt.Errorf("a root key is %d bytes, not %d", Size, len(raw))
	}
	var key Key
	copy(key.bytes[:], raw)
	if key.IsZero() {
		return Key{}, errors.New("a root key of zero bytes is not a key")
	}
	return key, nil
}

// ParseKEK reads a 32-byte key given as standard base64 or hex, the forms
// module_runtime.ca_kek and identity.kek accept.
func ParseKEK(raw string) (Key, error) {
	raw = strings.TrimSpace(raw)
	for _, decode := range []func(string) ([]byte, error){base64.StdEncoding.DecodeString, hex.DecodeString} {
		if decoded, err := decode(raw); err == nil && len(decoded) == Size {
			return FromBytes(decoded)
		}
	}
	return Key{}, errors.New("the key must be 32 bytes, base64 or hex encoded")
}

// File is the content of root.key.
type File struct {
	Key Key
	// AdoptedJWTSecret is the jwt.secret an earlier configuration set, kept
	// until a rotation; empty when the secret is derived.
	AdoptedJWTSecret string
	// AdoptedIdentityKEK is the identity.kek an earlier configuration set, in
	// standard base64; empty when the KEK is derived.
	AdoptedIdentityKEK string
}

// String hides the secrets.
func (File) String() string { return "[root key file]" }

// GoString hides the secrets.
func (File) GoString() string { return "rootkey.File{[root key file]}" }

// JWTSecret is the JWT secret the file stands for: the adopted one, else the
// derived one.
func (f File) JWTSecret() string {
	if f.AdoptedJWTSecret != "" {
		return f.AdoptedJWTSecret
	}
	return f.Key.DerivedJWTSecret()
}

// IdentityKEK is the identity KEK the file stands for: the adopted one, else
// the derived one.
func (f File) IdentityKEK() string {
	if f.AdoptedIdentityKEK != "" {
		return f.AdoptedIdentityKEK
	}
	return f.Key.DerivedIdentityKEK()
}

// Marshal encodes the file.
func (f File) Marshal() []byte {
	var buffer bytes.Buffer
	buffer.WriteString("# AnixOps Control root key. Keep it secret; back it up with the database.\n")
	buffer.WriteString("# Format: docs/reference/configuration.md, \"The root key\".\n")
	fmt.Fprintf(&buffer, "%s: %s\n", fieldVersion, Version)
	fmt.Fprintf(&buffer, "%s: %s\n", fieldKey, f.Key.Base64())
	if f.AdoptedJWTSecret != "" {
		fmt.Fprintf(&buffer, "%s: %s\n", fieldJWTSecret, base64.StdEncoding.EncodeToString([]byte(f.AdoptedJWTSecret)))
	}
	if f.AdoptedIdentityKEK != "" {
		fmt.Fprintf(&buffer, "%s: %s\n", fieldIdentityKEK, f.AdoptedIdentityKEK)
	}
	return buffer.Bytes()
}

// Parse decodes a root.key file. The errors never contain key material.
func Parse(data []byte) (File, error) {
	var file File
	seen := map[string]bool{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	line := 0
	versioned := false
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		name, value, ok := strings.Cut(text, ":")
		name, value = strings.TrimSpace(name), strings.TrimSpace(value)
		if !ok || name == "" {
			return File{}, fmt.Errorf("line %d is not a \"name: value\" field", line)
		}
		if !versioned {
			if name != fieldVersion {
				return File{}, fmt.Errorf("line %d: the file must start with %q", line, fieldVersion+": "+Version)
			}
			if value != Version {
				return File{}, fmt.Errorf("format %q is not %s: the file was written by another Control version", value, Version)
			}
			versioned = true
			continue
		}
		if seen[name] {
			return File{}, fmt.Errorf("line %d: %s is given twice", line, name)
		}
		seen[name] = true
		switch name {
		case fieldKey:
			raw, err := base64.StdEncoding.DecodeString(value)
			if err != nil {
				return File{}, fmt.Errorf("line %d: the key is not standard base64", line)
			}
			key, err := FromBytes(raw)
			if err != nil {
				return File{}, fmt.Errorf("line %d: %v", line, err)
			}
			file.Key = key
		case fieldJWTSecret:
			raw, err := base64.StdEncoding.DecodeString(value)
			if err != nil || len(raw) == 0 {
				return File{}, fmt.Errorf("line %d: the adopted JWT secret is not standard base64", line)
			}
			file.AdoptedJWTSecret = string(raw)
		case fieldIdentityKEK:
			kek, err := ParseKEK(value)
			if err != nil {
				return File{}, fmt.Errorf("line %d: the adopted identity KEK: %v", line, err)
			}
			file.AdoptedIdentityKEK = kek.Base64()
		default:
			return File{}, fmt.Errorf("line %d: unknown field %q", line, name)
		}
	}
	if err := scanner.Err(); err != nil {
		return File{}, err
	}
	if !versioned {
		return File{}, fmt.Errorf("the file is empty: it must start with %q", fieldVersion+": "+Version)
	}
	if !seen[fieldKey] {
		return File{}, errors.New("the file has no key field")
	}
	return file, nil
}
