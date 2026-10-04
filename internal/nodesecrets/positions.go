package nodesecrets

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/AnixOps/anix-control/sdk/v2compat"
)

// Placeholder stands for a node secret in administrators' answers
// (service.NodeSecretPlaceholder). A finalized JSON column keeps it at every
// secret position.
const Placeholder = v2compat.NodeSecretPlaceholder

// IsSecretKey reports whether a key of a node protocol's settings, or of a
// node's raw configuration, names a secret: Reality's and TLS's
// private_key (privateKey), WireGuard's server_private_key and
// preshared_key, Shadowsocks' server_key, psk and password, the Hysteria2
// obfs-password and auth, and any other *_key, password, passwd, pass,
// secret, token, credential or seed. Public keys, Reality's short_id and the
// paths of key files (key_file, wss_key_file) are not secrets.
func IsSecretKey(key string) bool {
	return v2compat.IsNodeSecretKey(key)
}

// Position is one secret of a column: its RFC 6901 JSON pointer and the
// JSON encoding of its value, or, with an empty pointer, the whole text of a
// column that is not JSON.
type Position struct {
	Pointer string
	Value   string
	// Moved is set when the column holds the placeholder there: the secret
	// is kept in the new table only, so the writer leaves its row as it is.
	Moved bool
}

// SecretPositions returns the secrets of a node protocol setting or a raw
// configuration, ordered by pointer. They are exactly the values that
// service.RedactNodeSecretsJSON replaces with the placeholder: a non-empty
// string or array under a secret key (objects under one are searched
// instead), in objects at any depth and in arrays; and the whole text when
// it is not JSON. A blank column has none.
func SecretPositions(document string) []Position {
	if strings.TrimSpace(document) == "" {
		return nil
	}
	value, ok := decodeJSON(document)
	if !ok {
		return []Position{{Value: document, Moved: document == Placeholder}}
	}
	var positions []Position
	collectPositions(value, "", &positions)
	sort.Slice(positions, func(i, j int) bool { return positions[i].Pointer < positions[j].Pointer })
	return positions
}

func collectPositions(value any, pointer string, positions *[]Position) {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			child := pointer + "/" + escapePointerToken(key)
			if object, ok := item.(map[string]any); ok {
				collectPositions(object, child, positions)
				continue
			}
			if IsSecretKey(key) && hasSecretValue(item) {
				*positions = append(*positions, Position{Pointer: child, Value: encodeJSON(item), Moved: item == Placeholder})
				continue
			}
			collectPositions(item, child, positions)
		}
	case []any:
		for index, item := range typed {
			collectPositions(item, pointer+"/"+strconv.Itoa(index), positions)
		}
	}
}

// Redact replaces the secret values of a node protocol setting or a raw
// configuration (JSON text) with Placeholder: a string or array under a
// secret key is replaced whole, an object's own keys are checked, and so are
// the objects in arrays. A document without a secret is returned unchanged,
// and one that is not JSON is replaced whole, since it cannot be told apart.
// These are exactly the positions SecretPositions answers. It is the
// administrators' masked answer (service.RedactNodeSecretsJSON) and the
// document a finalized JSON column keeps (section 4.4).
func Redact(document string) string {
	return v2compat.RedactNodeSecrets(document)
}

// hasSecretValue reports whether a value under a secret key holds a secret:
// a non-empty string or array. Empty values, null, numbers and booleans are
// not secrets.
func hasSecretValue(value any) bool {
	return v2compat.HasNodeSecretValue(value)
}

// escapePointerToken escapes one reference token of a JSON pointer.
func escapePointerToken(token string) string {
	return strings.ReplaceAll(strings.ReplaceAll(token, "~", "~0"), "/", "~1")
}

// decodeJSON decodes exactly one JSON value, keeping numbers as written.
func decodeJSON(document string) (any, bool) {
	decoder := json.NewDecoder(strings.NewReader(document))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, false
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, false
	}
	return value, true
}

func encodeJSON(value any) string {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		// Decoded JSON always encodes again.
		return ""
	}
	return strings.TrimSuffix(buffer.String(), "\n")
}
