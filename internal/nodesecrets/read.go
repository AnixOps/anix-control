package nodesecrets

import (
	"errors"
	"strconv"
	"strings"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// This file is the read side of the split (section 4.3). Every kernel
// reader of a moved column reads through it, from the new tables only: the
// legacy columns hold tombstones and redacted documents, and a literal
// secret a JSON column holds anyway (a row written past the kernel's
// writers) is ignored, its position left at the placeholder. No reader
// takes a tombstone or the placeholder for a secret.

func newSession(db *gorm.DB) *gorm.DB {
	return db.Session(&gorm.Session{NewDB: true})
}

// --- Protocol secrets (v2_node_protocol, v2_node.raw_config, v2_wireguard_peer) ---

// ResolveProtocols gives the secret positions of the protocols' JSON
// columns their values from the new table, in place, where the kernel
// builds a node's or a user's configuration from them. A column is
// rewritten only where a position takes a value its legacy document does
// not hold; otherwise it stays byte for byte as stored.
func ResolveProtocols(db *gorm.DB, protocols []model.NodeProtocol) {
	if len(protocols) == 0 {
		return
	}
	pointers := make([]*model.NodeProtocol, len(protocols))
	for i := range protocols {
		pointers[i] = &protocols[i]
	}
	resolveProtocols(db, pointers)
}

// ResolveProtocol is ResolveProtocols for one protocol.
func ResolveProtocol(db *gorm.DB, protocol *model.NodeProtocol) {
	if protocol == nil {
		return
	}
	resolveProtocols(db, []*model.NodeProtocol{protocol})
}

func resolveProtocols(db *gorm.DB, protocols []*model.NodeProtocol) {
	ids := make([]uint64, 0, len(protocols))
	for _, protocol := range protocols {
		ids = append(ids, uint64(protocol.ID))
	}
	rows, err := secretRows(db, ScopeNodeProtocol, ids)
	for _, protocol := range protocols {
		owner := uint64(protocol.ID)
		for _, column := range []struct {
			name  string
			value **string
		}{
			{"settings", &protocol.Settings},
			{"tls_settings", &protocol.TLSSettings},
			{"transport_settings", &protocol.TransportSettings},
			{"reality_settings", &protocol.RealitySettings},
			{"custom_config", &protocol.CustomConfig},
		} {
			if *column.value == nil {
				continue
			}
			resolved := resolveDocument(**column.value, rows[secretOwner{owner, column.name}], err)
			if resolved != **column.value {
				*column.value = &resolved
			}
		}
	}
}

// ResolveNodeRawConfig gives the secret positions of node's raw
// configuration their values from the new table, in place.
func ResolveNodeRawConfig(db *gorm.DB, node *model.Node) {
	if node == nil || node.RawConfig == nil {
		return
	}
	owner := uint64(node.ID)
	rows, err := secretRows(db, ScopeNodeRawConfig, []uint64{owner})
	resolved := resolveDocument(*node.RawConfig, rows[secretOwner{owner, "raw_config"}], err)
	if resolved != *node.RawConfig {
		node.RawConfig = &resolved
	}
}

// ResolveWireGuardPeers gives the peers' private and preshared keys their
// values from the new table, in place: a user's subscription and the
// node's runtime user list. A peer without a row there has no key.
func ResolveWireGuardPeers(db *gorm.DB, peers ...*model.WireGuardPeer) {
	if len(peers) == 0 {
		return
	}
	ids := make([]uint64, 0, len(peers))
	for _, peer := range peers {
		if peer != nil {
			ids = append(ids, uint64(peer.ID))
		}
	}
	rows, err := secretRows(db, ScopeWireGuardPeer, ids)
	for _, peer := range peers {
		if peer == nil {
			continue
		}
		owner := uint64(peer.ID)
		peer.PrivateKey = peerKey(rows[secretOwner{owner, "private_key"}], err)
		peer.PresharedKey = peerKey(rows[secretOwner{owner, "preshared_key"}], err)
	}
}

func peerKey(rows map[string]string, err error) string {
	if err != nil {
		return ""
	}
	return rows[""]
}

type secretOwner struct {
	owner  uint64
	column string
}

// secretRows loads the new-table secrets of the owners of one scope, by
// owner and column, then pointer.
func secretRows(db *gorm.DB, scope string, owners []uint64) (map[secretOwner]map[string]string, error) {
	out := make(map[secretOwner]map[string]string)
	for start := 0; start < len(owners); start += syncBatch {
		end := min(start+syncBatch, len(owners))
		var rows []model.ProtocolSecret
		if err := newSession(db).Select("owner_id", "column_name", "json_pointer", "value").
			Where("scope = ? AND owner_id IN ?", scope, owners[start:end]).Find(&rows).Error; err != nil {
			return out, err
		}
		for _, row := range rows {
			key := secretOwner{row.OwnerID, row.ColumnName}
			if out[key] == nil {
				out[key] = make(map[string]string)
			}
			out[key][row.JSONPointer] = row.Value
		}
	}
	return out, nil
}

// resolveDocument gives each secret position of one legacy JSON column the
// new table's value. A position the new table does not hold is left at the
// placeholder, whatever the legacy document holds there.
func resolveDocument(document string, rows map[string]string, readErr error) string {
	positions := SecretPositions(document)
	if len(positions) == 0 {
		return document
	}
	replacements := map[string]string{}
	placeholder := encodeJSON(Placeholder)
	for _, position := range positions {
		value, found := rows[position.Pointer]
		switch {
		case readErr == nil && found:
			if value != position.Value {
				replacements[position.Pointer] = value
			}
		case !position.Moved:
			if position.Pointer == "" {
				replacements[""] = Placeholder
			} else {
				replacements[position.Pointer] = placeholder
			}
		}
	}
	if len(replacements) == 0 {
		return document
	}
	resolved, err := replacePositions(document, replacements)
	if err != nil {
		return Redact(document)
	}
	return resolved
}

// ReplacePositions returns document with each pointer set to its
// JSON-encoded value (an empty pointer stands for the whole text). A
// document that is not JSON, a value that is not, or a pointer whose parent
// is not in the document is an error.
func ReplacePositions(document string, replacements map[string]string) (string, error) {
	if len(replacements) == 0 {
		return document, nil
	}
	return replacePositions(document, replacements)
}

// replacePositions sets each pointer of document to its JSON-encoded value.
// An empty pointer stands for the whole column, whose value is the text
// itself.
func replacePositions(document string, replacements map[string]string) (string, error) {
	if value, whole := replacements[""]; whole {
		return value, nil
	}
	root, ok := decodeJSON(document)
	if !ok {
		return "", errors.New("nodesecrets: the document is not JSON")
	}
	for pointer, encoded := range replacements {
		value, ok := decodeJSON(encoded)
		if !ok {
			return "", errors.New("nodesecrets: a stored secret is not JSON")
		}
		if !setPointer(root, pointer, value) {
			return "", errors.New("nodesecrets: a secret position is not in the document")
		}
	}
	encoded := encodeJSON(root)
	if encoded == "" {
		return "", errors.New("nodesecrets: the document cannot be encoded")
	}
	return encoded, nil
}

// setPointer sets the value at an RFC 6901 pointer whose parent exists.
func setPointer(root any, pointer string, value any) bool {
	tokens := strings.Split(pointer, "/")[1:]
	parent := root
	for i, token := range tokens {
		token = strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")
		last := i == len(tokens)-1
		switch typed := parent.(type) {
		case map[string]any:
			if last {
				typed[token] = value
				return true
			}
			parent = typed[token]
		case []any:
			index, err := strconv.Atoi(token)
			if err != nil || index < 0 || index >= len(typed) {
				return false
			}
			if last {
				typed[index] = value
				return true
			}
			parent = typed[index]
		default:
			return false
		}
	}
	return false
}
