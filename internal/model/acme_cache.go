package model

import "time"

// ACMECacheEntry is one entry of the panel certificate store
// (internal/panelcert): the ACME account key, issued certificates with
// their keys, challenge answers in flight, the self-signed fallback and the
// certificate status Control publishes for the command line. An entry that
// holds a private key is sealed with the root key (module_runtime.ca_kek,
// AES-256-GCM): Sealed tells which. The table travels with the database, so
// a restore or a move to another machine does not request the certificates
// again.
//
// The schema migration 0003_acme_cache (internal/database/migrations)
// creates the table; the model is not one of the kernel models, which
// describe the 0001_baseline schema.
type ACMECacheEntry struct {
	// Key is the cache key: an autocert key under a per-directory prefix,
	// or a Control entry ("anixops:...").
	Key       string    `gorm:"column:cache_key;primaryKey;size:255" json:"key"`
	Data      []byte    `gorm:"not null" json:"-"`
	Sealed    bool      `gorm:"not null;default:false" json:"sealed"`
	UpdatedAt time.Time `gorm:"not null" json:"updated_at"`
}

// TableName keeps the 5.0 kernel prefix.
func (ACMECacheEntry) TableName() string { return "v5_acme_cache" }
