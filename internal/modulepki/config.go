package modulepki

import (
	"errors"
	"fmt"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"gorm.io/gorm"
)

// ErrBuiltinPKIDisabled means the kernel does not run its own module CA:
// the module runtime is off or uses an external PKI.
var ErrBuiltinPKIDisabled = errors.New("the built-in module PKI is not enabled")

// FromConfig returns the built-in Authority configured by cfg.
func FromConfig(cfg config.ModuleRuntimeConfig, db *gorm.DB) (*Authority, error) {
	if !cfg.Enabled || cfg.PKIOrDefault() != config.ModulePKIBuiltin {
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
