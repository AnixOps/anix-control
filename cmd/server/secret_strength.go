package main

import "github.com/AnixOps/anix-control/v5/internal/config"

// logSecretStrength logs one WARNING per weak shared secret at server start
// (jwt.secret; see config.SecretStrengthWarnings).
// It never stops the start: an installation with a short secret keeps working.
func logSecretStrength(logf func(format string, args ...any), cfg *config.Config) {
	for _, warning := range cfg.SecretStrengthWarnings() {
		logf("WARNING: %s", warning)
	}
}
