package modulepki

import (
	"errors"
	"fmt"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/config"
	"gorm.io/gorm"
)

// ErrBuiltinPKIDisabled means the kernel does not run its own CA: no
// module_runtime.ca_kek is configured, or the PKI is external.
var ErrBuiltinPKIDisabled = errors.New("the built-in module PKI is not enabled")

// FromConfig returns the built-in Authority configured by cfg: with pki
// builtin, module_runtime.ca_kek configures the kernel CA that signs agent
// and kernel certificates (config.ModuleRuntimeConfig.BuiltinCA).
func FromConfig(cfg config.ModuleRuntimeConfig, db *gorm.DB) (*Authority, error) {
	if !cfg.BuiltinCA() {
		return nil, ErrBuiltinPKIDisabled
	}
	kek, err := ParseKEK(cfg.CAKEK)
	if err != nil {
		return nil, err
	}
	lifetime, err := time.ParseDuration(cfg.CertLifetimeOrDefault())
	if err != nil {
		return nil, fmt.Errorf("invalid module_runtime.cert_lifetime: %w", err)
	}
	return New(Options{DB: db, Cluster: cfg.ClusterOrDefault(), KEK: kek, Lifetime: lifetime})
}
