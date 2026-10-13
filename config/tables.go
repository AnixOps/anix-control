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

// PackageExtraction is config/package-extraction.json: the owning package
// and route id of every /api/v2 route.
//
//go:embed package-extraction.json
var PackageExtraction []byte

// PackageRouteDefaults is config/package-route-defaults.json: the routes
// that run natively by default (package_routes.default_mode: rehearsed) once
// they passed the staging rehearsal, with the package release each needs.
// The list is explicit: a route that becomes native-flagged in
// package-extraction.json does not default to native until it is added here.
//
//go:embed package-route-defaults.json
var PackageRouteDefaults []byte
