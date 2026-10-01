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

// Editions is config/editions.json: the packages and v2 routes that only the
// commercial edition (app.edition) serves. The community edition answers
// their routes as undeclared, and its release leaves the packages out
// (packages/shared/build_package.py --edition).
//
//go:embed editions.json
var Editions []byte

// PackageExtraction is config/package-extraction.json: the owning package
// and route id of every /api/v2 route, which the edition filter keys on.
//
//go:embed package-extraction.json
var PackageExtraction []byte
