// Package identityplatform carries the identity-platform package's own
// files that its control host needs at run time.
package identityplatform

import "embed"

// Migrations is the package's migrations/ directory, as it appears in the
// package artifact, for the host's storage migrations.
//
//go:embed migrations
var Migrations embed.FS

// MigrationIndex is the index path within Migrations.
const MigrationIndex = "migrations/index.json"
