package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	maxPluginSecretFiles     = 16
	maxPluginSecretFileBytes = 1 << 20
	maxPluginSecretTotal     = 4 << 20
	pluginSecretAADVersion   = "anixops.plugin-secret/v1"
)

var (
	ErrPluginSecretKeyringUnavailable = errors.New("plugin secret encryption keyring is unavailable")
	ErrPluginSecretConflict           = errors.New("plugin secret already exists")
	ErrPluginSecretDeleted            = errors.New("plugin secret is deleted")
	ErrPluginSecretVersionActive      = errors.New("active plugin secret version cannot be deleted")
	ErrPluginSecretReferenced         = errors.New("plugin secret is still referenced")
	ErrPluginSecretIntegrity          = errors.New("plugin secret material failed integrity verification")
)

type PluginSecretFileInput struct {
	Name    string
	Content []byte
}

type PluginSecretCreateInput struct {
	ID          string
	Name        string
	Description string
	Files       []PluginSecretFileInput
	ActorID     uint
}

type PluginSecretFileMetadata struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type PluginSecretVersionMetadata struct {
	Version   uint64                     `json:"version"`
	KeyID     string                     `json:"key_id"`
	CreatedBy uint                       `json:"created_by"`
	CreatedAt time.Time                  `json:"created_at"`
	Files     []PluginSecretFileMetadata `json:"files"`
}

type PluginSecretMetadata struct {
	ID            string                        `json:"id"`
	Name          string                        `json:"name"`
	Description   string                        `json:"description"`
	ActiveVersion uint64                        `json:"active_version"`
	CreatedBy     uint                          `json:"created_by"`
	CreatedAt     time.Time                     `json:"created_at"`
	UpdatedAt     time.Time                     `json:"updated_at"`
	Versions      []PluginSecretVersionMetadata `json:"versions,omitempty"`
}

// PluginSecretDispatchMaterial is safe to serialize only inside one live,
// authenticated Agent operation. Callers must not persist or log ContentBase64.
type PluginSecretDispatchMaterial struct {
	Reference     string `json:"reference"`
	SHA256        string `json:"sha256"`
	ContentBase64 string `json:"content_base64"`
}

type PluginSecretReference struct {
	SecretID string
	Version  uint64
	Name     string
}

// PluginSecretVersionReference identifies one immutable bundle without
// selecting a file. Topology edges use this form to bind both endpoint
// configurations to the same credential version.
type PluginSecretVersionReference struct {
	SecretID string
	Version  uint64
}

func (r PluginSecretVersionReference) String() string {
	return fmt.Sprintf("secret://%s@%d", r.SecretID, r.Version)
}

func (r PluginSecretReference) String() string {
	return fmt.Sprintf("secret://%s@%d/%s", r.SecretID, r.Version, r.Name)
}

type PluginSecretStore struct {
	db          *gorm.DB
	activeKeyID string
	keys        map[string][]byte
	now         func() time.Time
}

func NewPluginSecretStore(db *gorm.DB, encryption config.PluginSecretEncryptionConfig) (*PluginSecretStore, error) {
	if db == nil {
		return nil, errors.New("plugin secret store requires a database")
	}
	activeKeyID := strings.TrimSpace(encryption.ActiveKeyID)
	if !validPluginSecretKeyID(activeKeyID) || len(encryption.Keys) == 0 {
		return nil, ErrPluginSecretKeyringUnavailable
	}
	keys := make(map[string][]byte, len(encryption.Keys))
	for rawID, encoded := range encryption.Keys {
		keyID := strings.TrimSpace(rawID)
		if !validPluginSecretKeyID(keyID) || keyID != rawID {
			return nil, fmt.Errorf("%w: invalid key ID", ErrPluginSecretKeyringUnavailable)
		}
		key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
		if err != nil || len(key) != 32 {
			return nil, fmt.Errorf("%w: key %q must contain 32 base64-encoded bytes", ErrPluginSecretKeyringUnavailable, keyID)
		}
		keys[keyID] = append([]byte(nil), key...)
	}
	if _, ok := keys[activeKeyID]; !ok {
		return nil, fmt.Errorf("%w: active key is not present", ErrPluginSecretKeyringUnavailable)
	}
	return &PluginSecretStore{db: db, activeKeyID: activeKeyID, keys: keys, now: time.Now}, nil
}

func (s *PluginSecretStore) Create(input PluginSecretCreateInput) (*PluginSecretMetadata, error) {
	input.ID = strings.TrimSpace(input.ID)
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	if !validPluginSecretID(input.ID) {
		return nil, errors.New("plugin secret id must be a lowercase package identifier")
	}
	if input.Name == "" || len(input.Name) > 160 || strings.ContainsAny(input.Name, "\x00\r\n") {
		return nil, errors.New("plugin secret name is invalid")
	}
	if len(input.Description) > 2000 || strings.ContainsRune(input.Description, '\x00') {
		return nil, errors.New("plugin secret description is invalid")
	}
	files, err := validatePluginSecretFiles(input.Files)
	if err != nil {
		return nil, err
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&model.PluginSecret{}).Where("id = ?", input.ID).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return ErrPluginSecretConflict
		}
		secret := model.PluginSecret{
			ID: input.ID, Name: input.Name, Description: input.Description,
			CreatedBy: input.ActorID, ActiveVersion: 0,
		}
		if err := tx.Create(&secret).Error; err != nil {
			return err
		}
		if _, err := s.createVersionTx(tx, &secret, files, input.ActorID); err != nil {
			return err
		}
		return s.auditTx(tx, secret.ID, secret.ActiveVersion, "create", input.ActorID, "succeeded", fmt.Sprintf("files=%d", len(files)))
	})
	if err != nil {
		return nil, err
	}
	return s.Get(input.ID)
}

func (s *PluginSecretStore) AddVersion(secretID string, files []PluginSecretFileInput, actorID uint) (*PluginSecretMetadata, error) {
	secretID = strings.TrimSpace(secretID)
	if !validPluginSecretID(secretID) {
		return nil, errors.New("plugin secret id is invalid")
	}
	checked, err := validatePluginSecretFiles(files)
	if err != nil {
		return nil, err
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var secret model.PluginSecret
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&secret, "id = ?", secretID).Error; err != nil {
			return err
		}
		if secret.DeletedAt != nil {
			return ErrPluginSecretDeleted
		}
		version, err := s.createVersionTx(tx, &secret, checked, actorID)
		if err != nil {
			return err
		}
		return s.auditTx(tx, secret.ID, version.Version, "create_version", actorID, "succeeded", fmt.Sprintf("files=%d", len(checked)))
	})
	if err != nil {
		return nil, err
	}
	return s.Get(secretID)
}

func (s *PluginSecretStore) List() ([]PluginSecretMetadata, error) {
	var rows []model.PluginSecret
	if err := s.db.Where("deleted_at IS NULL").Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]PluginSecretMetadata, 0, len(rows))
	for _, row := range rows {
		result = append(result, pluginSecretMetadata(row))
	}
	return result, nil
}

func (s *PluginSecretStore) Get(secretID string) (*PluginSecretMetadata, error) {
	var secret model.PluginSecret
	if err := s.db.First(&secret, "id = ? AND deleted_at IS NULL", strings.TrimSpace(secretID)).Error; err != nil {
		return nil, err
	}
	metadata := pluginSecretMetadata(secret)
	var versions []model.PluginSecretVersion
	if err := s.db.Where("secret_id = ?", secret.ID).Order("version DESC").Find(&versions).Error; err != nil {
		return nil, err
	}
	metadata.Versions = make([]PluginSecretVersionMetadata, 0, len(versions))
	for _, version := range versions {
		var materials []model.PluginSecretMaterial
		if err := s.db.Select("name", "sha256", "size").Where("secret_version_id = ?", version.ID).Order("name").Find(&materials).Error; err != nil {
			return nil, err
		}
		entry := PluginSecretVersionMetadata{
			Version: version.Version, KeyID: version.KeyID, CreatedBy: version.CreatedBy, CreatedAt: version.CreatedAt,
			Files: make([]PluginSecretFileMetadata, 0, len(materials)),
		}
		for _, material := range materials {
			entry.Files = append(entry.Files, PluginSecretFileMetadata{Name: material.Name, SHA256: material.SHA256, Size: material.Size})
		}
		metadata.Versions = append(metadata.Versions, entry)
	}
	return &metadata, nil
}

func (s *PluginSecretStore) ResolveConfig(configJSON string) ([]PluginSecretDispatchMaterial, error) {
	references, err := ParsePluginSecretReferences(configJSON)
	if err != nil {
		return nil, err
	}
	result := make([]PluginSecretDispatchMaterial, 0, len(references))
	for _, reference := range references {
		material, err := s.resolve(reference)
		if err != nil {
			return nil, fmt.Errorf("resolve plugin secret reference %q: %w", reference.String(), err)
		}
		result = append(result, material)
	}
	return result, nil
}

func (s *PluginSecretStore) resolve(reference PluginSecretReference) (PluginSecretDispatchMaterial, error) {
	var secret model.PluginSecret
	if err := s.db.Select("id", "deleted_at").First(&secret, "id = ?", reference.SecretID).Error; err != nil {
		return PluginSecretDispatchMaterial{}, err
	}
	if secret.DeletedAt != nil {
		return PluginSecretDispatchMaterial{}, ErrPluginSecretDeleted
	}
	var version model.PluginSecretVersion
	if err := s.db.First(&version, "secret_id = ? AND version = ?", reference.SecretID, reference.Version).Error; err != nil {
		return PluginSecretDispatchMaterial{}, err
	}
	key, ok := s.keys[version.KeyID]
	if !ok {
		return PluginSecretDispatchMaterial{}, fmt.Errorf("%w: decryption key %q is not configured", ErrPluginSecretKeyringUnavailable, version.KeyID)
	}
	var material model.PluginSecretMaterial
	if err := s.db.First(&material, "secret_version_id = ? AND name = ?", version.ID, reference.Name).Error; err != nil {
		return PluginSecretDispatchMaterial{}, err
	}
	plaintext, err := decryptPluginSecretMaterial(key, reference, material.Nonce, material.Ciphertext)
	if err != nil {
		return PluginSecretDispatchMaterial{}, ErrPluginSecretIntegrity
	}
	digest := sha256.Sum256(plaintext)
	if int64(len(plaintext)) != material.Size || !strings.EqualFold(hex.EncodeToString(digest[:]), material.SHA256) {
		return PluginSecretDispatchMaterial{}, ErrPluginSecretIntegrity
	}
	return PluginSecretDispatchMaterial{
		Reference: reference.String(), SHA256: material.SHA256,
		ContentBase64: base64.StdEncoding.EncodeToString(plaintext),
	}, nil
}

func (s *PluginSecretStore) DeleteVersion(secretID string, version uint64, actorID uint) error {
	if !validPluginSecretID(strings.TrimSpace(secretID)) || version == 0 {
		return errors.New("plugin secret id and version are required")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		var secret model.PluginSecret
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&secret, "id = ? AND deleted_at IS NULL", secretID).Error; err != nil {
			return err
		}
		if secret.ActiveVersion == version {
			return ErrPluginSecretVersionActive
		}
		referenced, err := pluginSecretVersionReferenced(tx, secretID, version)
		if err != nil {
			return err
		}
		if referenced {
			return ErrPluginSecretReferenced
		}
		var row model.PluginSecretVersion
		if err := tx.First(&row, "secret_id = ? AND version = ?", secretID, version).Error; err != nil {
			return err
		}
		if err := tx.Where("secret_version_id = ?", row.ID).Delete(&model.PluginSecretMaterial{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&row).Error; err != nil {
			return err
		}
		return s.auditTx(tx, secretID, version, "delete_version", actorID, "succeeded", "encrypted material removed")
	})
}

func (s *PluginSecretStore) Delete(secretID string, actorID uint) error {
	secretID = strings.TrimSpace(secretID)
	if !validPluginSecretID(secretID) {
		return errors.New("plugin secret id is invalid")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		var secret model.PluginSecret
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&secret, "id = ? AND deleted_at IS NULL", secretID).Error; err != nil {
			return err
		}
		referenced, err := pluginSecretReferenced(tx, secretID)
		if err != nil {
			return err
		}
		if referenced {
			return ErrPluginSecretReferenced
		}
		var versionIDs []uint
		if err := tx.Model(&model.PluginSecretVersion{}).Where("secret_id = ?", secretID).Pluck("id", &versionIDs).Error; err != nil {
			return err
		}
		if len(versionIDs) > 0 {
			if err := tx.Where("secret_version_id IN ?", versionIDs).Delete(&model.PluginSecretMaterial{}).Error; err != nil {
				return err
			}
		}
		now := s.now()
		if err := tx.Model(&secret).Updates(map[string]any{"deleted_at": now, "active_version": 0, "updated_at": now}).Error; err != nil {
			return err
		}
		return s.auditTx(tx, secretID, secret.ActiveVersion, "delete", actorID, "succeeded", "all encrypted material removed")
	})
}

func (s *PluginSecretStore) createVersionTx(tx *gorm.DB, secret *model.PluginSecret, files []PluginSecretFileInput, actorID uint) (*model.PluginSecretVersion, error) {
	versionNumber := secret.ActiveVersion + 1
	version := model.PluginSecretVersion{
		SecretID: secret.ID, Version: versionNumber, KeyID: s.activeKeyID,
		FileCount: len(files), CreatedBy: actorID,
	}
	if err := tx.Create(&version).Error; err != nil {
		return nil, err
	}
	key := s.keys[s.activeKeyID]
	for _, file := range files {
		reference := PluginSecretReference{SecretID: secret.ID, Version: versionNumber, Name: file.Name}
		nonce, ciphertext, err := encryptPluginSecretMaterial(key, reference, file.Content)
		if err != nil {
			return nil, err
		}
		digest := sha256.Sum256(file.Content)
		row := model.PluginSecretMaterial{
			SecretVersionID: version.ID, Name: file.Name, Ciphertext: ciphertext, Nonce: nonce,
			SHA256: hex.EncodeToString(digest[:]), Size: int64(len(file.Content)),
		}
		if err := tx.Create(&row).Error; err != nil {
			return nil, err
		}
	}
	secret.ActiveVersion = versionNumber
	if err := tx.Model(secret).Updates(map[string]any{"active_version": versionNumber, "updated_at": s.now()}).Error; err != nil {
		return nil, err
	}
	return &version, nil
}

func (s *PluginSecretStore) auditTx(tx *gorm.DB, secretID string, version uint64, action string, actorID uint, outcome, detail string) error {
	return tx.Create(&model.PluginSecretAudit{
		SecretID: secretID, Version: version, Action: action, ActorID: actorID,
		Outcome: outcome, Detail: detail, CreatedAt: s.now(),
	}).Error
}

// RecordPluginSecretDispatchAudit stores metadata for a dispatch attempt and
// never decrypts, persists, or logs material bytes. Replays update the same
// operation/node/version row so the audit remains bounded and deterministic.
func RecordPluginSecretDispatchAudit(db *gorm.DB, configJSON, operationID string, nodeID uint, outcome string, now time.Time) error {
	if db == nil || strings.TrimSpace(operationID) == "" || nodeID == 0 {
		return errors.New("plugin secret dispatch audit identity is incomplete")
	}
	switch outcome {
	case "prepared", "accepted", "rejected", "retryable_error", "material_error":
	default:
		return errors.New("plugin secret dispatch audit outcome is invalid")
	}
	references, err := ParsePluginSecretReferences(configJSON)
	if err != nil {
		return err
	}
	counts := make(map[PluginSecretVersionReference]int)
	for _, reference := range references {
		counts[PluginSecretVersionReference{SecretID: reference.SecretID, Version: reference.Version}]++
	}
	keys := make([]PluginSecretVersionReference, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
	return db.Transaction(func(tx *gorm.DB) error {
		for _, key := range keys {
			row := model.PluginSecretAudit{
				SecretID: key.SecretID, Version: key.Version, Action: "dispatch", ActorID: 0,
				OperationID: operationID, NodeID: &nodeID, Outcome: outcome,
				Detail: fmt.Sprintf("files=%d", counts[key]), CreatedAt: now,
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "secret_id"}, {Name: "version"}, {Name: "action"}, {Name: "operation_id"}, {Name: "node_id"}},
				DoUpdates: clause.Assignments(map[string]any{"outcome": row.Outcome, "detail": row.Detail}),
			}).Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func ParsePluginSecretReferences(configJSON string) ([]PluginSecretReference, error) {
	decoder := json.NewDecoder(strings.NewReader(configJSON))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("decode plugin config for secret references: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("plugin config must contain one JSON value")
	}
	unique := make(map[string]PluginSecretReference)
	var walk func(any) error
	walk = func(current any) error {
		switch typed := current.(type) {
		case map[string]any:
			for _, child := range typed {
				if err := walk(child); err != nil {
					return err
				}
			}
		case []any:
			for _, child := range typed {
				if err := walk(child); err != nil {
					return err
				}
			}
		case string:
			if !strings.HasPrefix(typed, "secret://") {
				return nil
			}
			reference, err := ParsePluginSecretReference(typed)
			if err != nil {
				return err
			}
			unique[reference.String()] = reference
		}
		return nil
	}
	if err := walk(value); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(unique))
	for key := range unique {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]PluginSecretReference, 0, len(keys))
	for _, key := range keys {
		result = append(result, unique[key])
	}
	return result, nil
}

func ParsePluginSecretReference(raw string) (PluginSecretReference, error) {
	if raw != strings.TrimSpace(raw) || !strings.HasPrefix(raw, "secret://") {
		return PluginSecretReference{}, errors.New("plugin secret reference is invalid")
	}
	identity, name, found := strings.Cut(strings.TrimPrefix(raw, "secret://"), "/")
	if !found || strings.Contains(name, "/") {
		return PluginSecretReference{}, errors.New("plugin secret reference must be secret://id@version/file")
	}
	secretID, versionText, found := strings.Cut(identity, "@")
	if !found || strings.Contains(versionText, "@") || !validPluginSecretID(secretID) || !validPluginSecretFileName(name) {
		return PluginSecretReference{}, errors.New("plugin secret reference is invalid")
	}
	version, err := strconv.ParseUint(versionText, 10, 64)
	if err != nil || version == 0 || strconv.FormatUint(version, 10) != versionText {
		return PluginSecretReference{}, errors.New("plugin secret reference version is invalid")
	}
	reference := PluginSecretReference{SecretID: secretID, Version: version, Name: name}
	if reference.String() != raw {
		return PluginSecretReference{}, errors.New("plugin secret reference is not canonical")
	}
	return reference, nil
}

func ParsePluginSecretVersionReference(raw string) (PluginSecretVersionReference, error) {
	if raw != strings.TrimSpace(raw) || !strings.HasPrefix(raw, "secret://") || strings.Contains(strings.TrimPrefix(raw, "secret://"), "/") {
		return PluginSecretVersionReference{}, errors.New("plugin secret version reference must be secret://id@version")
	}
	secretID, versionText, found := strings.Cut(strings.TrimPrefix(raw, "secret://"), "@")
	if !found || strings.Contains(versionText, "@") || !validPluginSecretID(secretID) {
		return PluginSecretVersionReference{}, errors.New("plugin secret version reference is invalid")
	}
	version, err := strconv.ParseUint(versionText, 10, 64)
	if err != nil || version == 0 || strconv.FormatUint(version, 10) != versionText {
		return PluginSecretVersionReference{}, errors.New("plugin secret version reference is invalid")
	}
	reference := PluginSecretVersionReference{SecretID: secretID, Version: version}
	if reference.String() != raw {
		return PluginSecretVersionReference{}, errors.New("plugin secret version reference is not canonical")
	}
	return reference, nil
}

func encryptPluginSecretMaterial(key []byte, reference PluginSecretReference, plaintext []byte) ([]byte, []byte, error) {
	aead, err := pluginSecretAEAD(key)
	if err != nil {
		return nil, nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, fmt.Errorf("generate plugin secret nonce: %w", err)
	}
	ciphertext := aead.Seal(nil, nonce, plaintext, pluginSecretAAD(reference))
	return nonce, ciphertext, nil
}

func decryptPluginSecretMaterial(key []byte, reference PluginSecretReference, nonce, ciphertext []byte) ([]byte, error) {
	aead, err := pluginSecretAEAD(key)
	if err != nil {
		return nil, err
	}
	if len(nonce) != aead.NonceSize() {
		return nil, ErrPluginSecretIntegrity
	}
	return aead.Open(nil, nonce, ciphertext, pluginSecretAAD(reference))
}

func pluginSecretAEAD(key []byte) (cipher.AEAD, error) {
	if len(key) != 32 {
		return nil, ErrPluginSecretKeyringUnavailable
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func pluginSecretAAD(reference PluginSecretReference) []byte {
	return []byte(pluginSecretAADVersion + "\x00" + reference.SecretID + "\x00" + strconv.FormatUint(reference.Version, 10) + "\x00" + reference.Name)
}

func validatePluginSecretFiles(input []PluginSecretFileInput) ([]PluginSecretFileInput, error) {
	if len(input) == 0 || len(input) > maxPluginSecretFiles {
		return nil, fmt.Errorf("plugin secret must contain between 1 and %d files", maxPluginSecretFiles)
	}
	seen := make(map[string]struct{}, len(input))
	total := 0
	files := make([]PluginSecretFileInput, len(input))
	for index, file := range input {
		file.Name = strings.TrimSpace(file.Name)
		if !validPluginSecretFileName(file.Name) {
			return nil, fmt.Errorf("plugin secret file %d name is invalid", index)
		}
		if _, duplicate := seen[file.Name]; duplicate {
			return nil, fmt.Errorf("plugin secret file %q is duplicated", file.Name)
		}
		seen[file.Name] = struct{}{}
		if len(file.Content) == 0 || len(file.Content) > maxPluginSecretFileBytes {
			return nil, fmt.Errorf("plugin secret file %q size is invalid", file.Name)
		}
		total += len(file.Content)
		if total > maxPluginSecretTotal {
			return nil, fmt.Errorf("plugin secret content exceeds %d bytes", maxPluginSecretTotal)
		}
		files[index] = PluginSecretFileInput{Name: file.Name, Content: append([]byte(nil), file.Content...)}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	return files, nil
}

func pluginSecretMetadata(row model.PluginSecret) PluginSecretMetadata {
	return PluginSecretMetadata{
		ID: row.ID, Name: row.Name, Description: row.Description, ActiveVersion: row.ActiveVersion,
		CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func validPluginSecretID(value string) bool {
	if len(value) == 0 || len(value) > 120 || value != strings.ToLower(value) || !asciiLetterOrDigit(value[0]) || !asciiLetterOrDigit(value[len(value)-1]) || strings.Contains(value, "..") {
		return false
	}
	for index := range value {
		if !asciiLetterOrDigit(value[index]) && !strings.ContainsRune("._-", rune(value[index])) {
			return false
		}
	}
	return true
}

func validPluginSecretKeyID(value string) bool {
	return len(value) <= 80 && validPluginSecretID(value)
}

func validPluginSecretFileName(value string) bool {
	if len(value) == 0 || len(value) > 120 || value == "." || value == ".." || strings.ContainsAny(value, "/\\\x00\r\n") {
		return false
	}
	for index := range value {
		if !asciiLetterOrDigit(value[index]) && !strings.ContainsRune("._-", rune(value[index])) {
			return false
		}
	}
	return true
}

func asciiLetterOrDigit(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9'
}

func pluginSecretReferenced(db *gorm.DB, secretID string) (bool, error) {
	var versions []uint64
	if err := db.Model(&model.PluginSecretVersion{}).Where("secret_id = ?", secretID).Order("version").Pluck("version", &versions).Error; err != nil {
		return false, err
	}
	for _, version := range versions {
		referenced, err := pluginSecretVersionReferenced(db, secretID, version)
		if err != nil || referenced {
			return referenced, err
		}
	}
	return false, nil
}

func pluginSecretVersionReferenced(db *gorm.DB, secretID string, version uint64) (bool, error) {
	prefix := fmt.Sprintf("secret://%s@%d/", secretID, version)
	type configRow struct{ ConfigJSON string }
	queries := []struct {
		model any
		cols  string
	}{
		{&model.TopologyVertex{}, "config_json"},
		{&model.TopologyEdge{}, "config_json"},
		{&model.PluginConfiguration{}, "config_json"},
		{&model.KernelOperation{}, "config_json"},
		{&model.TopologyDeploymentStep{}, "config_json"},
		{&model.TopologyDeploymentStep{}, "rollback_config_json"},
	}
	for _, query := range queries {
		if !db.Migrator().HasTable(query.model) {
			continue
		}
		var rows []configRow
		if err := db.Model(query.model).Select(query.cols+" AS config_json").Where(query.cols+" LIKE ?", "%"+prefix+"%").Find(&rows).Error; err != nil {
			return false, err
		}
		for _, row := range rows {
			references, err := ParsePluginSecretReferences(row.ConfigJSON)
			if err != nil {
				return false, err
			}
			for _, reference := range references {
				if reference.SecretID == secretID && reference.Version == version {
					return true, nil
				}
			}
		}
	}
	if db.Migrator().HasTable(&model.TopologyEdge{}) {
		var count int64
		versionIdentity := fmt.Sprintf("secret://%s@%d", secretID, version)
		if err := db.Model(&model.TopologyEdge{}).
			Where("secret_id = ? OR secret_id = ? OR secret_id LIKE ?", secretID, versionIdentity, versionIdentity+"/%").
			Count(&count).Error; err != nil {
			return false, err
		}
		if count > 0 {
			return true, nil
		}
	}
	return false, nil
}
