package main

import "github.com/AnixOps/anix-control/v4/internal/config"

// logSecretStrength logs one WARNING per weak shared secret at server start
// (jwt.secret, app.api_token, grpc.api_token; see config.SecretStrengthWarnings).
// It never stops the start: an installation with a short secret keeps working.
func logSecretStrength(logf func(format string, args ...any), cfg *config.Config) {
	for _, warning := range cfg.SecretStrengthWarnings() {
		logf("WARNING: %s", warning)
	}
}
