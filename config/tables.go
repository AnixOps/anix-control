// Package config embeds the kernel-owned tables of this directory that the
// kernel reads at run time. Packages cannot change them: they ship in the
// kernel binary.
package config

import _ "embed"

// NodeSecretFields is config/node-secret-fields.json: per v2 route, the
// request and answer fields that carry node secrets, which the gateway
// seals (docs/architecture/node-ops-service.md section 3.7).
//
//go:embed node-secret-fields.json
var NodeSecretFields []byte
