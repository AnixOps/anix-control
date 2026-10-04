package kernelforward

// Entry high availability through DNS (forward-sdk.md section 7.4, L2):
// DNS provider accounts, the routes' bindings to a provider zone and the
// route DNS status that ForwardControl serves. The controller that keeps
// the records on the healthy entries is dns_controller.go.

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/forwardddns"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/modulepki"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"gorm.io/gorm"
)

// Binding defaults and bounds.
const (
	DNSDefaultTTL = 60
	DNSMaxTTL     = 86400
	// dnsMaxName bounds a provider's name.
	dnsMaxName = 128
	// dnsAdditionalData binds sealed credentials to the DNS provider
	// store and the row: a sealed module or link CA key never opens as
	// credentials, nor one provider's credentials as another's.
	dnsAdditionalData = "anixops-forward-dns-provider:"
)

// Binding modes and record types as stored.
const (
	DNSModeDDNS  = "ddns"
	DNSModeCNAME = "cname"
)

// Violation codes of the DNS writes.
const (
	CodeSecretStoreUnavailable = "secret_store_unavailable"
	CodeProviderInUse          = "provider_in_use"
	CodeBindingExists          = "binding_exists"
	CodeNameTaken              = "name_taken"
	CodeUnknownRoute           = "unknown_route"
	CodeUnknownProvider        = "unknown_provider"
	CodeEntryHostnameRequired  = "entry_hostname_required"
	CodeHostnameMismatch       = "hostname_mismatch"
	CodeImmutable              = "immutable"
	CodeInvalidFormat          = "invalid_format"
	CodeRequired               = "required"
	CodeUnknownField           = "unknown_field"
)

// methods of the request ledger.
const (
	methodCreateDNSProvider = "create_dns_provider"
	methodUpdateDNSProvider = "update_dns_provider"
	methodDeleteDNSProvider = "delete_dns_provider"
	methodCreateDNSBinding  = "create_dns_binding"
	methodUpdateDNSBinding  = "update_dns_binding"
	methodDeleteDNSBinding  = "delete_dns_binding"
)

var (
	// ErrDNSProviderNotFound: no DNS provider with that id.
	ErrDNSProviderNotFound = errors.New("forward dns provider not found")
	// ErrDNSBindingNotFound: no DNS binding with that id.
	ErrDNSBindingNotFound = errors.New("forward dns binding not found")
	// ErrDNSPurge: a binding's published records could not be deleted.
	ErrDNSPurge = errors.New("forward dns: the provider did not delete the published records")
	// errNoKEK: Control has no key-encryption key to seal credentials.
	errNoKEK = errors.New("set module_runtime.ca_kek: DNS provider credentials are sealed with Control's key-encryption key")
)

// DNSRefusedError is a DNS write refused with violations; Precondition is
// true when the request is well formed but cannot be applied now.
type DNSRefusedError struct {
	Violations   []*forwardv1.Violation
	Precondition bool
}

func (e *DNSRefusedError) Error() string {
	if len(e.Violations) == 0 {
		return "forward dns write refused"
	}
	first := e.Violations[0]
	return fmt.Sprintf("forward dns write refused: %s %s (%s)", first.GetField(), first.GetMessage(), first.GetCode())
}

func dnsRefusal(precondition bool, violations ...*forwardv1.Violation) error {
	return &DNSRefusedError{Violations: violations, Precondition: precondition}
}

func violation(field, code, message string) *forwardv1.Violation {
	return &forwardv1.Violation{Field: field, Code: code, Message: message}
}

// ---------------------------------------------------------------------------
// Kinds

var kindNames = map[forwardv1.DnsProviderKind]string{
	forwardv1.DnsProviderKind_DNS_PROVIDER_KIND_CLOUDFLARE:  forwardddns.KindCloudflare,
	forwardv1.DnsProviderKind_DNS_PROVIDER_KIND_ALIDNS:      forwardddns.KindAliDNS,
	forwardv1.DnsProviderKind_DNS_PROVIDER_KIND_DNSPOD:      forwardddns.KindDNSPod,
	forwardv1.DnsProviderKind_DNS_PROVIDER_KIND_HUAWEICLOUD: forwardddns.KindHuaweiCloud,
	forwardv1.DnsProviderKind_DNS_PROVIDER_KIND_WEBHOOK:     forwardddns.KindWebhook,
}

func kindOf(name string) forwardv1.DnsProviderKind {
	for kind, n := range kindNames {
		if n == name {
			return kind
		}
	}
	return forwardv1.DnsProviderKind_DNS_PROVIDER_KIND_UNSPECIFIED
}

func recordTypeName(t forwardv1.DnsRecordType) string {
	switch t {
	case forwardv1.DnsRecordType_DNS_RECORD_TYPE_A:
		return forwardddns.TypeA
	case forwardv1.DnsRecordType_DNS_RECORD_TYPE_AAAA:
		return forwardddns.TypeAAAA
	}
	return ""
}

func recordTypeOf(name string) forwardv1.DnsRecordType {
	switch name {
	case forwardddns.TypeA:
		return forwardv1.DnsRecordType_DNS_RECORD_TYPE_A
	case forwardddns.TypeAAAA:
		return forwardv1.DnsRecordType_DNS_RECORD_TYPE_AAAA
	}
	return forwardv1.DnsRecordType_DNS_RECORD_TYPE_UNSPECIFIED
}

// ---------------------------------------------------------------------------
// Sealing

// credentialSealer seals provider credentials with AES-256-GCM under the
// key-encryption key module_runtime.ca_kek (as the module and forward link
// CAs seal their keys), with additional data naming the provider row.
type credentialSealer struct {
	aead cipher.AEAD
	kek  []byte
}

func (s *Service) credentialSealer() (*credentialSealer, error) {
	var raw string
	if s.DNSKEK != nil {
		raw = s.DNSKEK()
	} else if cfg := config.Get(); cfg != nil {
		raw = cfg.ModuleRuntime.CAKEK
	}
	if strings.TrimSpace(raw) == "" {
		return nil, errNoKEK
	}
	kek, err := modulepki.ParseKEK(raw)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &credentialSealer{aead: aead, kek: kek}, nil
}

func additionalData(providerID uint64) []byte {
	return []byte(dnsAdditionalData + strconv.FormatUint(providerID, 10))
}

// fingerprints replaces each credential value with its HMAC under the
// key-encryption key, so the request ledger's hash of a write never
// derives from a plain credential.
func (c *credentialSealer) fingerprints(credentials map[string]string) map[string]string {
	out := make(map[string]string, len(credentials))
	for name, value := range credentials {
		mac := hmac.New(sha256.New, c.kek)
		mac.Write([]byte("anixops-forward-dns-credential:" + name + "\x00" + value))
		out[name] = hex.EncodeToString(mac.Sum(nil))
	}
	return out
}

func (c *credentialSealer) seal(providerID uint64, credentials map[string]string) (string, error) {
	plain, err := json.Marshal(credentials)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(c.aead.Seal(nonce, nonce, plain, additionalData(providerID))), nil
}

func (c *credentialSealer) open(row model.KernelForwardDNSProvider) (map[string]string, error) {
	sealed, err := base64.StdEncoding.DecodeString(row.SealedCredentials)
	if err != nil || len(sealed) < c.aead.NonceSize() {
		return nil, fmt.Errorf("forward dns provider %d: sealed credentials are corrupt", row.ID)
	}
	nonce, ciphertext := sealed[:c.aead.NonceSize()], sealed[c.aead.NonceSize():]
	plain, err := c.aead.Open(nil, nonce, ciphertext, additionalData(row.ID))
	if err != nil {
		return nil, fmt.Errorf("forward dns provider %d: credentials cannot be unsealed: wrong key-encryption key", row.ID)
	}
	credentials := map[string]string{}
	if err := json.Unmarshal(plain, &credentials); err != nil {
		return nil, fmt.Errorf("forward dns provider %d: sealed credentials are corrupt", row.ID)
	}
	return credentials, nil
}

// ---------------------------------------------------------------------------
// Providers

func decodeStrings(raw string) []string {
	var out []string
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &out)
	}
	return out
}

func decodeMap(raw string) map[string]string {
	out := map[string]string{}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &out)
	}
	return out
}

func encodeJSON(value any) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func providerView(row model.KernelForwardDNSProvider, bindings int64) *forwardv1.DnsProvider {
	return &forwardv1.DnsProvider{
		Id: row.ID, Name: row.Name, Kind: kindOf(row.Kind), Config: decodeMap(row.ConfigJSON),
		CredentialNames: decodeStrings(row.CredentialNames), Bindings: uint32(min(bindings, 1<<31)), // #nosec G115 -- bounded above.
		CreatedAtUnixMs: row.CreatedAt.UnixMilli(), UpdatedAtUnixMs: row.UpdatedAt.UnixMilli(),
	}
}

func bindingCounts(tx *gorm.DB) (map[uint64]int64, error) {
	var rows []struct {
		ProviderID uint64
		Count      int64
	}
	if err := tx.Model(&model.KernelForwardDNSBinding{}).Select("provider_id, count(*) as count").Group("provider_id").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("kernel forward: count dns bindings: %w", err)
	}
	out := map[uint64]int64{}
	for _, row := range rows {
		out[row.ProviderID] = row.Count
	}
	return out, nil
}

// ListDNSProviders answers every DNS provider without credentials.
func (s *Service) ListDNSProviders(ctx context.Context) ([]*forwardv1.DnsProvider, error) {
	db, err := s.db(ctx)
	if err != nil {
		return nil, err
	}
	var rows []model.KernelForwardDNSProvider
	if err := db.Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("kernel forward: list dns providers: %w", err)
	}
	counts, err := bindingCounts(db)
	if err != nil {
		return nil, err
	}
	out := make([]*forwardv1.DnsProvider, 0, len(rows))
	for _, row := range rows {
		out = append(out, providerView(row, counts[row.ID]))
	}
	return out, nil
}

func loadProvider(tx *gorm.DB, id uint64) (model.KernelForwardDNSProvider, error) {
	var rows []model.KernelForwardDNSProvider
	if err := tx.Where("id = ?", id).Limit(1).Find(&rows).Error; err != nil {
		return model.KernelForwardDNSProvider{}, fmt.Errorf("kernel forward: load dns provider: %w", err)
	}
	if len(rows) == 0 {
		return model.KernelForwardDNSProvider{}, ErrDNSProviderNotFound
	}
	return rows[0], nil
}

// GetDNSProvider answers one provider without credentials.
func (s *Service) GetDNSProvider(ctx context.Context, id uint64) (*forwardv1.DnsProvider, error) {
	db, err := s.db(ctx)
	if err != nil {
		return nil, err
	}
	row, err := loadProvider(db, id)
	if err != nil {
		return nil, err
	}
	var count int64
	if err := db.Model(&model.KernelForwardDNSBinding{}).Where("provider_id = ?", id).Count(&count).Error; err != nil {
		return nil, err
	}
	return providerView(row, count), nil
}

// checkProvider validates a provider's fields and its complete
// credentials.
func checkProvider(name, kind string, cfg, credentials map[string]string) []*forwardv1.Violation {
	var out []*forwardv1.Violation
	if name == "" || len(name) > dnsMaxName {
		out = append(out, violation("provider.name", CodeRequired, fmt.Sprintf("is required, at most %d bytes", dnsMaxName)))
	}
	if kind == "" {
		return append(out, violation("provider.kind", CodeRequired, "is required"))
	}
	for _, problem := range forwardddns.Validate(kind, cfg, credentials) {
		field := problem.Field
		if strings.HasPrefix(field, "config.") || field == "kind" {
			field = "provider." + field
		}
		out = append(out, violation(field, problem.Code, problem.Message))
	}
	return out
}

func trimMap(values map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out[strings.TrimSpace(key)] = value
		}
	}
	return out
}

func credentialNames(credentials map[string]string) []string {
	out := make([]string, 0, len(credentials))
	for name := range credentials {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// CreateDNSProvider stores a provider with its credentials sealed.
func (s *Service) CreateDNSProvider(ctx context.Context, requestID string, provider *forwardv1.DnsProvider, credentials map[string]string) (*forwardv1.DnsProvider, error) {
	requestID, err := checkRequestID(requestID)
	if err != nil {
		return nil, err
	}
	if provider == nil {
		return nil, fmt.Errorf("%w: provider is required", ErrInvalidRequest)
	}
	name, kind := strings.TrimSpace(provider.GetName()), kindNames[provider.GetKind()]
	cfg, credentials := trimMap(provider.GetConfig()), trimMap(credentials)
	if violations := checkProvider(name, kind, cfg, credentials); len(violations) > 0 {
		return nil, dnsRefusal(false, violations...)
	}
	sealer, err := s.credentialSealer()
	if err != nil {
		return nil, dnsRefusal(true, violation("credentials", CodeSecretStoreUnavailable, err.Error()))
	}
	db, err := s.db(ctx)
	if err != nil {
		return nil, err
	}
	hash := requestHash(methodCreateDNSProvider, &forwardv1.CreateDnsProviderRequest{Provider: provider, Credentials: sealer.fingerprints(credentials)})
	answer := &forwardv1.CreateDnsProviderResponse{}
	err = db.Transaction(func(tx *gorm.DB) error {
		now := s.now()
		if found, err := replay(tx, requestID, methodCreateDNSProvider, hash, answer); err != nil || found {
			return err
		}
		var taken int64
		if err := tx.Model(&model.KernelForwardDNSProvider{}).Where("name = ?", name).Count(&taken).Error; err != nil {
			return err
		}
		if taken > 0 {
			return dnsRefusal(true, violation("provider.name", CodeNameTaken, "another provider has this name"))
		}
		row := model.KernelForwardDNSProvider{
			Name: name, Kind: kind, ConfigJSON: encodeJSON(cfg), CredentialNames: encodeJSON(credentialNames(credentials)),
			CreatedAt: now, UpdatedAt: now,
		}
		if err := tx.Create(&row).Error; err != nil {
			return fmt.Errorf("kernel forward: create dns provider: %w", err)
		}
		sealed, err := sealer.seal(row.ID, credentials)
		if err != nil {
			return err
		}
		if err := tx.Model(&row).UpdateColumn("sealed_credentials", sealed).Error; err != nil {
			return err
		}
		answer.Provider = providerView(row, 0)
		return record(tx, requestID, methodCreateDNSProvider, "", hash, answer, now)
	})
	if err != nil {
		return nil, err
	}
	return answer.GetProvider(), nil
}

// UpdateDNSProvider replaces a provider's name and configuration, and the
// credentials sent; a credential left out or sent as the placeholder keeps
// its stored value.
func (s *Service) UpdateDNSProvider(ctx context.Context, requestID string, provider *forwardv1.DnsProvider, credentials map[string]string) (*forwardv1.DnsProvider, error) {
	requestID, err := checkRequestID(requestID)
	if err != nil {
		return nil, err
	}
	if provider == nil || provider.GetId() == 0 {
		return nil, fmt.Errorf("%w: provider.id is required", ErrInvalidRequest)
	}
	sealer, err := s.credentialSealer()
	if err != nil {
		return nil, dnsRefusal(true, violation("credentials", CodeSecretStoreUnavailable, err.Error()))
	}
	db, err := s.db(ctx)
	if err != nil {
		return nil, err
	}
	hash := requestHash(methodUpdateDNSProvider, &forwardv1.UpdateDnsProviderRequest{Provider: provider, Credentials: sealer.fingerprints(credentials)})
	answer := &forwardv1.UpdateDnsProviderResponse{}
	err = db.Transaction(func(tx *gorm.DB) error {
		now := s.now()
		if found, err := replay(tx, requestID, methodUpdateDNSProvider, hash, answer); err != nil || found {
			return err
		}
		row, err := loadProvider(tx, provider.GetId())
		if err != nil {
			return err
		}
		if provider.GetKind() != forwardv1.DnsProviderKind_DNS_PROVIDER_KIND_UNSPECIFIED && kindNames[provider.GetKind()] != row.Kind {
			return dnsRefusal(false, violation("provider.kind", CodeImmutable, "a provider's kind cannot change"))
		}
		merged, err := sealer.open(row)
		if err != nil {
			// Sealed under another key (a database restored with a new
			// ca_kek): the update must then send every credential again.
			merged = map[string]string{}
		}
		for key, value := range credentials {
			key, value = strings.TrimSpace(key), strings.TrimSpace(value)
			if value == "" || value == service.SensitiveSystemConfigPlaceholder {
				continue
			}
			merged[key] = value
		}
		name, cfg := strings.TrimSpace(provider.GetName()), trimMap(provider.GetConfig())
		if violations := checkProvider(name, row.Kind, cfg, merged); len(violations) > 0 {
			return dnsRefusal(false, violations...)
		}
		var taken int64
		if err := tx.Model(&model.KernelForwardDNSProvider{}).Where("name = ? AND id <> ?", name, row.ID).Count(&taken).Error; err != nil {
			return err
		}
		if taken > 0 {
			return dnsRefusal(true, violation("provider.name", CodeNameTaken, "another provider has this name"))
		}
		sealed, err := sealer.seal(row.ID, merged)
		if err != nil {
			return err
		}
		row.Name, row.ConfigJSON, row.CredentialNames, row.SealedCredentials, row.UpdatedAt = name, encodeJSON(cfg), encodeJSON(credentialNames(merged)), sealed, now
		if err := tx.Save(&row).Error; err != nil {
			return fmt.Errorf("kernel forward: update dns provider: %w", err)
		}
		var count int64
		if err := tx.Model(&model.KernelForwardDNSBinding{}).Where("provider_id = ?", row.ID).Count(&count).Error; err != nil {
			return err
		}
		answer.Provider = providerView(row, count)
		return record(tx, requestID, methodUpdateDNSProvider, "", hash, answer, now)
	})
	if err != nil {
		return nil, err
	}
	return answer.GetProvider(), nil
}

// DeleteDNSProvider removes a provider no binding uses.
func (s *Service) DeleteDNSProvider(ctx context.Context, requestID string, id uint64) error {
	requestID, err := checkRequestID(requestID)
	if err != nil {
		return err
	}
	db, err := s.db(ctx)
	if err != nil {
		return err
	}
	hash := requestHash(methodDeleteDNSProvider, &forwardv1.DeleteDnsProviderRequest{Id: id})
	answer := &forwardv1.DeleteDnsProviderResponse{}
	err = db.Transaction(func(tx *gorm.DB) error {
		now := s.now()
		if found, err := replay(tx, requestID, methodDeleteDNSProvider, hash, answer); err != nil || found {
			return err
		}
		if _, err := loadProvider(tx, id); err != nil {
			return err
		}
		var bindings []model.KernelForwardDNSBinding
		if err := tx.Where("provider_id = ?", id).Order("id").Find(&bindings).Error; err != nil {
			return err
		}
		if len(bindings) > 0 {
			violations := make([]*forwardv1.Violation, 0, len(bindings))
			for _, binding := range bindings {
				v := violation(fmt.Sprintf("bindings[%d]", binding.ID), CodeProviderInUse, "the binding of route "+binding.RouteID+" uses the provider")
				v.RouteId = binding.RouteID
				violations = append(violations, v)
			}
			return dnsRefusal(true, violations...)
		}
		if err := tx.Delete(&model.KernelForwardDNSProvider{}, "id = ?", id).Error; err != nil {
			return fmt.Errorf("kernel forward: delete dns provider: %w", err)
		}
		return record(tx, requestID, methodDeleteDNSProvider, "", hash, answer, now)
	})
	return err
}

// ---------------------------------------------------------------------------
// Bindings

func bindingTypes(row model.KernelForwardDNSBinding) []string {
	return strings.Split(row.RecordTypes, ",")
}

func bindingView(row model.KernelForwardDNSBinding) *forwardv1.DnsBinding {
	mode := forwardv1.DnsBindingMode_DNS_BINDING_MODE_DDNS
	if row.Mode == DNSModeCNAME {
		mode = forwardv1.DnsBindingMode_DNS_BINDING_MODE_CNAME
	}
	var types []forwardv1.DnsRecordType
	for _, name := range bindingTypes(row) {
		types = append(types, recordTypeOf(name))
	}
	return &forwardv1.DnsBinding{
		Id: row.ID, RouteId: row.RouteID, ProviderId: row.ProviderID, Zone: row.Zone, RecordName: row.RecordName,
		Mode: mode, RecordTypes: types, Ttl: row.TTL, Paused: row.Paused,
		CreatedAtUnixMs: row.CreatedAt.UnixMilli(), UpdatedAtUnixMs: row.UpdatedAt.UnixMilli(),
	}
}

// normalizeTypes answers the binding's record types as stored ("A",
// "A,AAAA"), A when none.
func normalizeTypes(types []forwardv1.DnsRecordType) (string, *forwardv1.Violation) {
	seen := map[string]bool{}
	for i, t := range types {
		name := recordTypeName(t)
		if name == "" {
			return "", violation(fmt.Sprintf("binding.record_types[%d]", i), CodeInvalidFormat, "must be A or AAAA")
		}
		seen[name] = true
	}
	switch {
	case seen[forwardddns.TypeA] && seen[forwardddns.TypeAAAA]:
		return "A,AAAA", nil
	case seen[forwardddns.TypeAAAA]:
		return "AAAA", nil
	}
	return "A", nil
}

func checkTTL(ttl uint32) (uint32, *forwardv1.Violation) {
	if ttl == 0 {
		return DNSDefaultTTL, nil
	}
	if ttl > DNSMaxTTL {
		return 0, violation("binding.ttl", CodeInvalidFormat, fmt.Sprintf("must be at most %d seconds", DNSMaxTTL))
	}
	return ttl, nil
}

// routeEntryHostname answers a stored route's entry_hostname.
func routeEntryHostname(tx *gorm.DB, routeID string) (string, bool, error) {
	var rows []model.KernelForwardRoute
	if err := tx.Where("id = ?", routeID).Limit(1).Find(&rows).Error; err != nil {
		return "", false, err
	}
	if len(rows) == 0 {
		return "", false, nil
	}
	route, err := decodeRoute(rows[0])
	if err != nil {
		return "", false, err
	}
	return forwardddns.NormalizeName(route.GetListen().GetEntryHostname()), true, nil
}

// CreateDNSBinding binds a route to a provider zone.
func (s *Service) CreateDNSBinding(ctx context.Context, requestID string, binding *forwardv1.DnsBinding) (*forwardv1.DnsBinding, error) {
	requestID, err := checkRequestID(requestID)
	if err != nil {
		return nil, err
	}
	if binding == nil {
		return nil, fmt.Errorf("%w: binding is required", ErrInvalidRequest)
	}
	db, err := s.db(ctx)
	if err != nil {
		return nil, err
	}
	hash := requestHash(methodCreateDNSBinding, binding)
	answer := &forwardv1.CreateDnsBindingResponse{}
	err = db.Transaction(func(tx *gorm.DB) error {
		now := s.now()
		if found, err := replay(tx, requestID, methodCreateDNSBinding, hash, answer); err != nil || found {
			return err
		}
		row, err := checkNewBinding(tx, binding)
		if err != nil {
			return err
		}
		row.CreatedAt, row.UpdatedAt, row.State = now, now, dnsStatePending
		if err := tx.Create(&row).Error; err != nil {
			return fmt.Errorf("kernel forward: create dns binding: %w", err)
		}
		answer.Binding = bindingView(row)
		return record(tx, requestID, methodCreateDNSBinding, row.RouteID, hash, answer, now)
	})
	if err != nil {
		return nil, err
	}
	return answer.GetBinding(), nil
}

func checkNewBinding(tx *gorm.DB, binding *forwardv1.DnsBinding) (model.KernelForwardDNSBinding, error) {
	var violations []*forwardv1.Violation
	precondition := false
	routeID := strings.TrimSpace(binding.GetRouteId())
	hostname, found, err := routeEntryHostname(tx, routeID)
	if err != nil {
		return model.KernelForwardDNSBinding{}, err
	}
	switch {
	case routeID == "" || !found:
		violations = append(violations, violation("binding.route_id", CodeUnknownRoute, "no stored route has this id"))
	case hostname == "":
		violations = append(violations, violation("binding.route_id", CodeEntryHostnameRequired, "the route has no listen.entry_hostname"))
	}
	if _, err := loadProvider(tx, binding.GetProviderId()); errors.Is(err, ErrDNSProviderNotFound) {
		violations = append(violations, violation("binding.provider_id", CodeUnknownProvider, "no DNS provider has this id"))
	} else if err != nil {
		return model.KernelForwardDNSBinding{}, err
	}
	zone := forwardddns.NormalizeName(binding.GetZone())
	if !forwardddns.ValidName(zone) {
		violations = append(violations, violation("binding.zone", CodeInvalidFormat, "must be a DNS zone name"))
	}
	name := forwardddns.NormalizeName(binding.GetRecordName())
	mode := ""
	switch binding.GetMode() {
	case forwardv1.DnsBindingMode_DNS_BINDING_MODE_DDNS, forwardv1.DnsBindingMode_DNS_BINDING_MODE_UNSPECIFIED:
		mode = DNSModeDDNS
		if name == "" {
			name = hostname
		}
		if hostname != "" && name != hostname {
			violations = append(violations, violation("binding.record_name", CodeHostnameMismatch, "in DDNS mode the record is the route's entry_hostname"))
		}
	case forwardv1.DnsBindingMode_DNS_BINDING_MODE_CNAME:
		mode = DNSModeCNAME
		if name != "" && name == hostname {
			violations = append(violations, violation("binding.record_name", CodeInvalidFormat, "in CNAME mode the record is a Control-managed name, not entry_hostname"))
		}
	default:
		violations = append(violations, violation("binding.mode", CodeInvalidFormat, "must be DDNS or CNAME"))
	}
	if !forwardddns.ValidName(name) {
		violations = append(violations, violation("binding.record_name", CodeInvalidFormat, "must be a DNS name"))
	} else if forwardddns.ValidName(zone) && !forwardddns.InZone(name, zone) {
		violations = append(violations, violation("binding.record_name", CodeInvalidFormat, "must be inside the zone"))
	}
	types, problem := normalizeTypes(binding.GetRecordTypes())
	if problem != nil {
		violations = append(violations, problem)
	}
	ttl, problem := checkTTL(binding.GetTtl())
	if problem != nil {
		violations = append(violations, problem)
	}
	if len(violations) == 0 {
		var taken []model.KernelForwardDNSBinding
		if err := tx.Where("route_id = ? OR (provider_id = ? AND zone = ? AND record_name = ?)", routeID, binding.GetProviderId(), zone, name).Find(&taken).Error; err != nil {
			return model.KernelForwardDNSBinding{}, err
		}
		for _, other := range taken {
			field := "binding.record_name"
			if other.RouteID == routeID {
				field = "binding.route_id"
			}
			v := violation(field, CodeBindingExists, fmt.Sprintf("binding %d already uses it", other.ID))
			v.RouteId = other.RouteID
			violations = append(violations, v)
			precondition = true
		}
	}
	if len(violations) > 0 {
		return model.KernelForwardDNSBinding{}, dnsRefusal(precondition, violations...)
	}
	return model.KernelForwardDNSBinding{
		RouteID: routeID, ProviderID: binding.GetProviderId(), Zone: zone, RecordName: name, Mode: mode,
		RecordTypes: types, TTL: ttl, Paused: binding.GetPaused(),
	}, nil
}

func loadBinding(tx *gorm.DB, id uint64) (model.KernelForwardDNSBinding, error) {
	var rows []model.KernelForwardDNSBinding
	if err := tx.Where("id = ?", id).Limit(1).Find(&rows).Error; err != nil {
		return model.KernelForwardDNSBinding{}, fmt.Errorf("kernel forward: load dns binding: %w", err)
	}
	if len(rows) == 0 {
		return model.KernelForwardDNSBinding{}, ErrDNSBindingNotFound
	}
	return rows[0], nil
}

// UpdateDNSBinding replaces a binding's record types, TTL and paused flag.
func (s *Service) UpdateDNSBinding(ctx context.Context, requestID string, binding *forwardv1.DnsBinding) (*forwardv1.DnsBinding, error) {
	requestID, err := checkRequestID(requestID)
	if err != nil {
		return nil, err
	}
	if binding == nil || binding.GetId() == 0 {
		return nil, fmt.Errorf("%w: binding.id is required", ErrInvalidRequest)
	}
	db, err := s.db(ctx)
	if err != nil {
		return nil, err
	}
	hash := requestHash(methodUpdateDNSBinding, binding)
	answer := &forwardv1.UpdateDnsBindingResponse{}
	err = db.Transaction(func(tx *gorm.DB) error {
		now := s.now()
		if found, err := replay(tx, requestID, methodUpdateDNSBinding, hash, answer); err != nil || found {
			return err
		}
		row, err := loadBinding(tx, binding.GetId())
		if err != nil {
			return err
		}
		current := bindingView(row)
		var violations []*forwardv1.Violation
		immutable := func(field string, changed bool) {
			if changed {
				violations = append(violations, violation("binding."+field, CodeImmutable, "cannot change; delete the binding (with purge) and create another"))
			}
		}
		immutable("route_id", binding.GetRouteId() != "" && binding.GetRouteId() != current.GetRouteId())
		immutable("provider_id", binding.GetProviderId() != 0 && binding.GetProviderId() != current.GetProviderId())
		immutable("zone", binding.GetZone() != "" && forwardddns.NormalizeName(binding.GetZone()) != current.GetZone())
		immutable("record_name", binding.GetRecordName() != "" && forwardddns.NormalizeName(binding.GetRecordName()) != current.GetRecordName())
		immutable("mode", binding.GetMode() != forwardv1.DnsBindingMode_DNS_BINDING_MODE_UNSPECIFIED && binding.GetMode() != current.GetMode())
		types, problem := normalizeTypes(binding.GetRecordTypes())
		if problem != nil {
			violations = append(violations, problem)
		}
		ttl, problem := checkTTL(binding.GetTtl())
		if problem != nil {
			violations = append(violations, problem)
		}
		if len(violations) > 0 {
			return dnsRefusal(false, violations...)
		}
		row.RecordTypes, row.TTL, row.Paused, row.UpdatedAt = types, ttl, binding.GetPaused(), now
		if err := tx.Model(&row).Select("record_types", "ttl", "paused", "updated_at").Updates(&row).Error; err != nil {
			return fmt.Errorf("kernel forward: update dns binding: %w", err)
		}
		answer.Binding = bindingView(row)
		return record(tx, requestID, methodUpdateDNSBinding, row.RouteID, hash, answer, now)
	})
	if err != nil {
		return nil, err
	}
	return answer.GetBinding(), nil
}

// DeleteDNSBinding removes a binding; with purge it first deletes the
// records Control published (ErrDNSPurge when the provider fails, and the
// binding stays).
func (s *Service) DeleteDNSBinding(ctx context.Context, requestID string, id uint64, purge bool) error {
	requestID, err := checkRequestID(requestID)
	if err != nil {
		return err
	}
	db, err := s.db(ctx)
	if err != nil {
		return err
	}
	hash := requestHash(methodDeleteDNSBinding, &forwardv1.DeleteDnsBindingRequest{Id: id, Purge: purge})
	answer := &forwardv1.DeleteDnsBindingResponse{}
	if found, err := replay(db, requestID, methodDeleteDNSBinding, hash, answer); err != nil || found {
		return err
	}
	row, err := loadBinding(db, id)
	if err != nil {
		return err
	}
	if purge {
		if err := s.purgeBinding(ctx, row); err != nil {
			return err
		}
	}
	return db.Transaction(func(tx *gorm.DB) error {
		now := s.now()
		if found, err := replay(tx, requestID, methodDeleteDNSBinding, hash, answer); err != nil || found {
			return err
		}
		if err := tx.Delete(&model.KernelForwardDNSNode{}, "binding_id = ?", id).Error; err != nil {
			return err
		}
		if err := tx.Delete(&model.KernelForwardDNSBinding{}, "id = ?", id).Error; err != nil {
			return fmt.Errorf("kernel forward: delete dns binding: %w", err)
		}
		if err := record(tx, requestID, methodDeleteDNSBinding, row.RouteID, hash, answer, now); err != nil {
			return err
		}
		return auditDNS(tx, "forward.dns_binding_delete", row, map[string]any{"purge": purge, "published": decodePublished(row.PublishedJSON)})
	})
}

// purgeBinding deletes every record type the binding published.
func (s *Service) purgeBinding(ctx context.Context, row model.KernelForwardDNSBinding) error {
	published := decodePublished(row.PublishedJSON)
	if len(published) == 0 {
		return nil
	}
	db, err := s.db(ctx)
	if err != nil {
		return err
	}
	provider, err := s.dnsProvider(db, row.ProviderID)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrDNSPurge, err.Error())
	}
	ctx, cancel := context.WithTimeout(ctx, dnsCallTimeout)
	defer cancel()
	for _, rtype := range sortedTypes(published) {
		if err := provider.DeleteRecords(ctx, row.Zone, row.RecordName, rtype); err != nil {
			dnsMetrics.add(provider.Kind(), "error")
			return fmt.Errorf("%w: %s", ErrDNSPurge, err.Error())
		}
		dnsMetrics.add(provider.Kind(), "deleted")
	}
	return nil
}

// ListDNSBindings answers the bindings, filtered by route and provider.
func (s *Service) ListDNSBindings(ctx context.Context, routeID string, providerID uint64) ([]*forwardv1.DnsBinding, error) {
	db, err := s.db(ctx)
	if err != nil {
		return nil, err
	}
	query := db.Order("id")
	if routeID != "" {
		query = query.Where("route_id = ?", routeID)
	}
	if providerID != 0 {
		query = query.Where("provider_id = ?", providerID)
	}
	var rows []model.KernelForwardDNSBinding
	if err := query.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("kernel forward: list dns bindings: %w", err)
	}
	out := make([]*forwardv1.DnsBinding, 0, len(rows))
	for _, row := range rows {
		out = append(out, bindingView(row))
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Status

func decodePublished(raw string) map[string][]string {
	out := map[string][]string{}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &out)
	}
	for key, values := range out {
		if len(values) == 0 {
			delete(out, key)
		}
	}
	return out
}

func sortedTypes(values map[string][]string) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func unixMilli(t *time.Time) int64 {
	if t == nil {
		return 0
	}
	return t.UnixMilli()
}

// RouteDNS answers a route's entry HA status; ErrNotFound for an unknown
// route.
func (s *Service) RouteDNS(ctx context.Context, routeID string) (*forwardv1.RouteDnsStatus, error) {
	db, err := s.db(ctx)
	if err != nil {
		return nil, err
	}
	hostname, found, err := routeEntryHostname(db, routeID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrNotFound
	}
	status := &forwardv1.RouteDnsStatus{RouteId: routeID, EntryHostname: hostname, State: dnsStateUnbound}
	var rows []model.KernelForwardDNSBinding
	if err := db.Where("route_id = ?", routeID).Limit(1).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return status, nil
	}
	row := rows[0]
	status.Binding = bindingView(row)
	if row.Mode == DNSModeCNAME {
		status.CnameTarget = row.RecordName
	}
	status.State = row.State
	if row.Paused {
		status.State = dnsStatePaused
	}
	if status.State == "" {
		status.State = dnsStatePending
	}
	status.PublishedAtUnixMs, status.EvaluatedAtUnixMs = unixMilli(row.PublishedAt), unixMilli(row.EvaluatedAt)
	status.LastError, status.LastErrorAtUnixMs, status.NextAttemptAtUnixMs = row.LastError, unixMilli(row.LastErrorAt), unixMilli(row.NextAttemptAt)
	published, desired := decodePublished(row.PublishedJSON), decodePublished(row.DesiredJSON)
	types := map[string]bool{}
	for _, t := range bindingTypes(row) {
		types[t] = true
	}
	for t := range published {
		types[t] = true
	}
	for _, t := range []string{forwardddns.TypeA, forwardddns.TypeAAAA} {
		if types[t] {
			status.Records = append(status.Records, &forwardv1.DnsRecordStatus{Type: recordTypeOf(t), Published: published[t], Desired: desired[t]})
		}
	}
	var nodes []model.KernelForwardDNSNode
	if err := db.Where("binding_id = ?", row.ID).Order("node_ref").Find(&nodes).Error; err != nil {
		return nil, err
	}
	inv, err := loadInventory(db)
	if err != nil {
		return nil, err
	}
	addresses := map[string][]string{}
	for _, info := range inv.nodes {
		addresses[info.GetNodeRef()] = publicAddresses(info.GetAddresses(), bindingTypes(row))
	}
	for _, node := range nodes {
		status.Nodes = append(status.Nodes, &forwardv1.DnsEntryNode{
			NodeRef: node.NodeRef, Addresses: addresses[node.NodeRef], Healthy: node.Healthy, InRotation: node.InRotation,
			GoodStreak: node.GoodStreak, BadStreak: node.BadStreak, Reason: node.Reason,
		})
	}
	return status, nil
}
