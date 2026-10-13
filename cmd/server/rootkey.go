package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/rootkey"
	"github.com/AnixOps/anix-control/v5/internal/rootkeycheck"
	"gorm.io/gorm"
)

// activeRootKey is the root key this process resolved (resolveRootKey);
// nil when it has none (a command without root.key or a legacy key).
var activeRootKey *rootkey.Resolved

// deferRootKeyCheck lets bootstrapDatabase skip the start-up check that the
// root key opens the stored keys: `doctor` runs it as one of its checks and
// reports a mismatch instead of stopping.
var deferRootKeyCheck bool

// resolveRootKey finds the root key in data_dir (rootkey.Resolve) and puts
// it where every reader looks: module_runtime.ca_kek (the CA, the ACME
// cache, DNS credentials and join tokens), jwt.secret and identity.kek. The
// server (write) creates root.key, adopting a configured
// module_runtime.ca_kek; commands only read it. It logs where each value
// comes from and never a value.
func resolveRootKey(cfg *config.Config, write bool, logf func(string, ...any)) (*rootkey.Resolved, error) {
	jwtSecret := cfg.JWT.Secret
	if config.PlaceholderJWTSecret(jwtSecret) {
		logf("WARNING: jwt.secret is a template value and is not used: the root key derives the JWT secret (every user signs in again once)")
		jwtSecret = ""
	}
	resolved, err := rootkey.Resolve(rootkey.Options{
		DataDir:           cfg.DataDir,
		Write:             write,
		LegacyCAKEK:       cfg.ModuleRuntime.CAKEK,
		LegacyJWTSecret:   jwtSecret,
		LegacyIdentityKEK: cfg.Identity.KEK,
	})
	if err != nil {
		return nil, err
	}
	for _, note := range resolvedNotes(resolved) {
		logf("%s", note)
	}
	if resolved == nil {
		if strings.TrimSpace(jwtSecret) == "" {
			cfg.JWT.Secret = ""
		}
		return nil, nil
	}
	cfg.ModuleRuntime.CAKEK = resolved.Key.Base64()
	cfg.JWT.Secret = resolved.JWTSecret
	cfg.Identity.KEK = resolved.IdentityKEK
	logf("Root key: %s (from %s); JWT secret: %s; identity KEK: %s", resolved.Path, resolved.KeySource, resolved.JWTSource, resolved.IdentitySource)
	return resolved, nil
}

func resolvedNotes(resolved *rootkey.Resolved) []string {
	if resolved == nil {
		return nil
	}
	return resolved.Notes
}

// rootKeyName names the root key in messages: its file, or the
// configuration key it came from.
func rootKeyName() string {
	switch {
	case activeRootKey == nil:
		return "the root key"
	case activeRootKey.Stored:
		return "the root key " + activeRootKey.Path
	default:
		return "the root key (module_runtime.ca_kek of the configuration)"
	}
}

// verifyRootKey checks, before anything uses the key, that the root key
// opens what the database holds sealed with it (rootkeycheck.Check): every
// current and next module CA and forward link CA. A database without a CA
// must open its sealed ACME cache entry and join token instead (each checked
// once, cheaply). With a CA, those two only warn: resetting a join token or
// requesting a certificate again heals them. Without a root key there is
// nothing to check.
func verifyRootKey(ctx context.Context, cfg *config.Config, db *gorm.DB, logf func(string, ...any)) error {
	raw := strings.TrimSpace(cfg.ModuleRuntime.CAKEK)
	if raw == "" || db == nil {
		return nil
	}
	key, err := rootkey.ParseKEK(raw)
	if err != nil {
		return fmt.Errorf("%s: %w", rootKeyName(), err)
	}
	err = rootkeycheck.Check(ctx, db, key.Bytes(), func(what string, cause error) {
		logf("WARNING: %s opens the CA keys but not a stored %s (%v); reset the node group's token or request the certificate again", rootKeyName(), what, cause)
	})
	var mismatch *rootkeycheck.Mismatch
	if errors.As(err, &mismatch) {
		return fmt.Errorf("%s does not open the %s stored in this database (%v). "+
			"The database was sealed with another root key: restore the root.key that belongs to this database "+
			"(from the backup taken with it) into %s, or start with the database that belongs to this key. "+
			"Control does not start with a root key that cannot open its stored keys", rootKeyName(), mismatch.What, mismatch.Err, rootkey.Path(cfg.DataDir))
	}
	return err
}
