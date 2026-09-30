package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/identity/keystore"
	"github.com/AnixOps/anix-control/identity/server"
	"github.com/AnixOps/anix-control/identity/signingkey"
	identityv1 "github.com/AnixOps/anix-control/sdk/api/identity/v1"
	"github.com/AnixOps/anix-control/sdk/packagestoresdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"google.golang.org/grpc"
)

// Control's identity tokens: the kernel verifies iss and aud.
const (
	tokenIssuer   = "anixops-identity"
	tokenAudience = "anix-control"
)

const (
	envKEK           = "ANIX_IDENTITY_KEK"
	envKEKFile       = "ANIX_IDENTITY_KEK_FILE"
	envTokenLifetime = "ANIX_IDENTITY_TOKEN_LIFETIME"
	// keyMaintenanceInterval is how often the host applies key rotation.
	keyMaintenanceInterval = time.Minute
)

// identityHost is the identity-platform package: the v2 route router, which
// still bridges every route to the kernel, plus the identity core's
// IdentityService and signing keys in the package's own storage.
type identityHost struct {
	*pluginhostsdk.Router
	identity *server.Server
	keys     *storedKeys
	logf     func(string, ...any)
}

var _ interface {
	pluginhostsdk.Package
	RegisterServices(grpc.ServiceRegistrar)
} = (*identityHost)(nil)

// RegisterServices serves IdentityService next to the host protocol.
func (h *identityHost) RegisterServices(registrar grpc.ServiceRegistrar) {
	identityv1.RegisterIdentityServiceServer(registrar, h.identity)
}

// Run polls the route configuration and, with signing keys configured,
// applies key rotation, for the life of the host.
func (h *identityHost) Run(ctx context.Context) {
	if h.keys == nil {
		h.Router.Run(ctx)
		return
	}
	var wait sync.WaitGroup
	wait.Add(1)
	go func() {
		defer wait.Done()
		h.Router.Run(ctx)
	}()
	h.keys.maintain(ctx, keyMaintenanceInterval, h.logf)
	wait.Wait()
}

// keyPolicy rotates monthly, publishes a key 15 minutes (three kernel key
// refreshes) before it signs, and keeps a retired key until every token it
// signed has expired.
func keyPolicy(getenv func(string) string) (signingkey.Policy, error) {
	lifetime := 24 * time.Hour
	if raw := strings.TrimSpace(getenv(envTokenLifetime)); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed <= 0 {
			return signingkey.Policy{}, errors.New(envTokenLifetime + " must be a positive duration")
		}
		lifetime = parsed
	}
	return signingkey.Policy{RotateAfter: 30 * 24 * time.Hour, PublishAhead: 15 * time.Minute, VerifyAfterRetire: lifetime + time.Hour}, nil
}

// loadKEK reads the signing key KEK from ANIX_IDENTITY_KEK or the file named
// by ANIX_IDENTITY_KEK_FILE. No KEK means no signing keys.
func loadKEK(getenv func(string) string) ([]byte, error) {
	value := strings.TrimSpace(getenv(envKEK))
	if value == "" {
		if path := strings.TrimSpace(getenv(envKEKFile)); path != "" {
			raw, err := os.ReadFile(path) // #nosec G304 -- the operator names the secret file.
			if err != nil {
				return nil, err
			}
			value = strings.TrimSpace(string(raw))
		}
	}
	if value == "" {
		return nil, nil
	}
	return signingkey.ParseKEK(value)
}

// storedKeys opens the key store in the package storage on first use.
type storedKeys struct {
	open   packagestoresdk.Opener
	sealer *signingkey.Sealer
	policy signingkey.Policy

	mu    sync.Mutex
	store *keystore.Store
}

func (k *storedKeys) keyStore(ctx context.Context) (*keystore.Store, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.store != nil {
		return k.store, nil
	}
	storage, err := k.open(ctx)
	if err != nil {
		return nil, err
	}
	k.store = &keystore.Store{DB: storage.DB, Table: storage.Table("signing_key"), Sealer: k.sealer, Policy: k.policy}
	return k.store, nil
}

// Load returns the keys, for IdentityService.
func (k *storedKeys) Load(ctx context.Context) ([]signingkey.Key, error) {
	store, err := k.keyStore(ctx)
	if err != nil {
		return nil, err
	}
	return store.Load(ctx)
}

// maintain applies rotation now and every interval. Failures (storage not
// leased or not migrated yet) are logged when they change.
func (k *storedKeys) maintain(ctx context.Context, interval time.Duration, logf func(string, ...any)) {
	lastError := ""
	for {
		message := ""
		store, err := k.keyStore(ctx)
		if err == nil {
			_, err = store.Advance(ctx)
		}
		if err != nil && ctx.Err() == nil {
			message = err.Error()
		}
		if message != lastError {
			if message != "" {
				logf("identity signing keys: %s", message)
			} else if lastError != "" {
				logf("identity signing keys: rotation recovered")
			}
			lastError = message
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}
