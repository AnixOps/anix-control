package service

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newPluginSecretTestStore(t *testing.T, active string, keys map[string][]byte) (*PluginSecretStore, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(model.KernelModels()...))
	encoded := make(map[string]string, len(keys))
	for id, key := range keys {
		encoded[id] = base64.StdEncoding.EncodeToString(key)
	}
	store, err := NewPluginSecretStore(db, config.PluginSecretEncryptionConfig{ActiveKeyID: active, Keys: encoded})
	require.NoError(t, err)
	return store, db
}

func TestPluginSecretStoreEncryptsVersionsAndRotatesKeys(t *testing.T) {
	oldKey := []byte("0123456789abcdef0123456789abcdef")
	newKey := []byte("abcdef0123456789abcdef0123456789")
	store, db := newPluginSecretTestStore(t, "old", map[string][]byte{"old": oldKey})
	plaintext := []byte("private-key-material-must-not-appear")
	created, err := store.Create(PluginSecretCreateInput{
		ID: "mesh-edge", Name: "Mesh edge", ActorID: 7,
		Files: []PluginSecretFileInput{{Name: "client.key", Content: plaintext}},
	})
	require.NoError(t, err)
	require.Equal(t, uint64(1), created.ActiveVersion)

	var material model.PluginSecretMaterial
	require.NoError(t, db.First(&material).Error)
	require.NotContains(t, string(material.Ciphertext), string(plaintext))
	require.NotContains(t, string(material.Nonce), string(plaintext))
	var count int64
	require.NoError(t, db.Model(&model.PluginSecretAudit{}).Where("detail LIKE ?", "%private-key-material%").Count(&count).Error)
	require.Zero(t, count)

	rotated, err := NewPluginSecretStore(db, config.PluginSecretEncryptionConfig{
		ActiveKeyID: "new",
		Keys: map[string]string{
			"old": base64.StdEncoding.EncodeToString(oldKey),
			"new": base64.StdEncoding.EncodeToString(newKey),
		},
	})
	require.NoError(t, err)
	_, err = rotated.AddVersion("mesh-edge", []PluginSecretFileInput{{Name: "client.key", Content: []byte("replacement")}}, 8)
	require.NoError(t, err)

	materials, err := rotated.ResolveConfig(`{"key_ref":"secret://mesh-edge@1/client.key","next_ref":"secret://mesh-edge@2/client.key"}`)
	require.NoError(t, err)
	require.Len(t, materials, 2)
	decoded, err := base64.StdEncoding.DecodeString(materials[0].ContentBase64)
	require.NoError(t, err)
	require.Equal(t, plaintext, decoded)
	metadata, err := rotated.Get("mesh-edge")
	require.NoError(t, err)
	require.Equal(t, uint64(2), metadata.ActiveVersion)
	require.Equal(t, "new", metadata.Versions[0].KeyID)
	require.Equal(t, "old", metadata.Versions[1].KeyID)
}

func TestPluginSecretStoreRejectsTamperingAndMissingRetiredKey(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	store, db := newPluginSecretTestStore(t, "primary", map[string][]byte{"primary": key})
	_, err := store.Create(PluginSecretCreateInput{
		ID: "mesh-tamper", Name: "Mesh", ActorID: 1,
		Files: []PluginSecretFileInput{{Name: "ca.pem", Content: []byte("certificate")}},
	})
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.PluginSecretMaterial{}).Where("name = ?", "ca.pem").Update("ciphertext", []byte("tampered")).Error)
	_, err = store.ResolveConfig(`{"ca_ref":"secret://mesh-tamper@1/ca.pem"}`)
	require.ErrorIs(t, err, ErrPluginSecretIntegrity)

	missing, err := NewPluginSecretStore(db, config.PluginSecretEncryptionConfig{
		ActiveKeyID: "replacement",
		Keys:        map[string]string{"replacement": base64.StdEncoding.EncodeToString([]byte("abcdef0123456789abcdef0123456789"))},
	})
	require.NoError(t, err)
	_, err = missing.ResolveConfig(`{"ca_ref":"secret://mesh-tamper@1/ca.pem"}`)
	require.ErrorIs(t, err, ErrPluginSecretKeyringUnavailable)
}

func TestParsePluginSecretReferencesIsStrictAndDeterministic(t *testing.T) {
	refs, err := ParsePluginSecretReferences(`{"b":"secret://mesh@2/key.pem","nested":["secret://mesh@1/ca.pem","secret://mesh@2/key.pem"]}`)
	require.NoError(t, err)
	require.Equal(t, []string{"secret://mesh@1/ca.pem", "secret://mesh@2/key.pem"}, []string{refs[0].String(), refs[1].String()})

	invalid := []string{
		`{"x":"secret://mesh/key.pem"}`,
		`{"x":"secret://mesh@0/key.pem"}`,
		`{"x":"secret://Mesh@1/../key.pem"}`,
		`{"x":"secret://mesh@01/key.pem"}`,
	}
	for _, document := range invalid {
		_, err := ParsePluginSecretReferences(document)
		require.Error(t, err, document)
	}
}

func TestParsePluginSecretVersionReferenceIsStrict(t *testing.T) {
	reference, err := ParsePluginSecretVersionReference("secret://mesh-edge@42")
	require.NoError(t, err)
	require.Equal(t, PluginSecretVersionReference{SecretID: "mesh-edge", Version: 42}, reference)
	for _, invalid := range []string{
		"mesh-edge", "secret://mesh-edge", "secret://mesh-edge@0", "secret://mesh-edge@01",
		"secret://Mesh@1", "secret://mesh-edge@1/ca.pem", " secret://mesh-edge@1",
	} {
		_, err := ParsePluginSecretVersionReference(invalid)
		require.Error(t, err, invalid)
	}
}

func TestPluginSecretDeletionFailsClosedForActiveAndReferencedVersions(t *testing.T) {
	store, db := newPluginSecretTestStore(t, "primary", map[string][]byte{"primary": []byte("0123456789abcdef0123456789abcdef")})
	_, err := store.Create(PluginSecretCreateInput{
		ID: "mesh-delete", Name: "Mesh", ActorID: 1,
		Files: []PluginSecretFileInput{{Name: "ca.pem", Content: []byte("ca-v1")}},
	})
	require.NoError(t, err)
	_, err = store.AddVersion("mesh-delete", []PluginSecretFileInput{{Name: "ca.pem", Content: []byte("ca-v2")}}, 2)
	require.NoError(t, err)
	require.ErrorIs(t, store.DeleteVersion("mesh-delete", 2, 2), ErrPluginSecretVersionActive)

	topology := model.Topology{Name: "referenced", ServiceScope: "forward"}
	require.NoError(t, db.Create(&topology).Error)
	revision := model.TopologyRevision{TopologyID: topology.ID, Revision: 1, State: "draft", ContentHash: strings.Repeat("a", 64), CreatedBy: 1}
	require.NoError(t, db.Create(&revision).Error)
	require.NoError(t, db.Create(&model.TopologyVertex{
		RevisionID: revision.ID, Key: "entry", Kind: "plugin", PluginID: "gost-mesh", Role: "entry",
		ConfigJSON: `{"tls":{"ca_ref":"secret://mesh-delete@1/ca.pem"}}`,
	}).Error)
	require.ErrorIs(t, store.DeleteVersion("mesh-delete", 1, 2), ErrPluginSecretReferenced)
	require.ErrorIs(t, store.Delete("mesh-delete", 2), ErrPluginSecretReferenced)

	require.NoError(t, db.Delete(&model.TopologyVertex{}, "revision_id = ?", revision.ID).Error)
	require.NoError(t, store.DeleteVersion("mesh-delete", 1, 2))
	require.NoError(t, store.Delete("mesh-delete", 2))
	_, err = store.Get("mesh-delete")
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound))
}

func TestNewPluginSecretStoreRejectsInvalidKeyrings(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	_, err = NewPluginSecretStore(db, config.PluginSecretEncryptionConfig{})
	require.ErrorIs(t, err, ErrPluginSecretKeyringUnavailable)
	_, err = NewPluginSecretStore(db, config.PluginSecretEncryptionConfig{ActiveKeyID: "primary", Keys: map[string]string{"primary": base64.StdEncoding.EncodeToString([]byte("short"))}})
	require.ErrorIs(t, err, ErrPluginSecretKeyringUnavailable)
}
