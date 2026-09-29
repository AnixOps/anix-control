# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- Moved the Control Center into `control-center/` of `AnixOps/anix-control`
  (snapshot of `master` merged with `production`); the old
  `AnixOps/Anixops-control-center` repository is archived. The Go module path
  is now `github.com/AnixOps/anix-control/control-center`, and the Cloudflare
  Workers API is back in `workers/`, trimmed to the routes these clients use.
- Removed the AI assistant, Web3/IPFS/vector pages and the unrouted
  ELK/log-search/metrics/monitoring/resilience/service-discovery/tracing mock
  pages from the web and Flutter clients, because the trimmed Workers API no
  longer serves them.

### Added

- Added a Control kernel plugin lifecycle page with an independent Control
  administrator login, MFA challenge, signed catalog discovery, revisioned
  configuration, Control/Agent actions, and operation status.
- Added a CI web test job and browser coverage for the Control login-to-catalog
  flow, session isolation, lifecycle actions, error states, and mobile layout.

### Changed

- Cloudflare Workers API moved to the standalone `anixops-control-center-workers` repository.
- Development branch remains the place for post-v2.5.0 work.

### Fixed

- Enabled Tailwind processing for the web build and made navigation usable on
  mobile viewports.

## [v2.5.0] - 2026-03-23

### Added

- **Realtime foundation across services and clients**
  - Workers SSE and WebSocket smoke coverage for authenticated realtime endpoints.
  - Dedicated handler tests for SSE and WebSocket protocol behavior.
  - Web realtime lifecycle coverage for nodes and tasks flows.
  - Mobile SSE parsing, reconnect, and provider-side realtime update coverage.

- **Workers AI and Vectorize release-readiness**
  - Executable service-level tests for AI text generation, embeddings, log analysis, query translation, and assistant chat flows.
  - Handler-level coverage for AI chat, log analysis, embeddings, and NL query endpoints.
  - Executable Vectorize service and handler tests for insert, search, delete, filtering, and anomaly helpers.
  - End-to-end AI and vector flows covering embedding, vector insert/search, chat, and natural language query translation.

- **Web3 and IPFS production candidate flow**
  - Executable Workers tests for SIWE challenge generation, DID handling, IPFS-backed storage, and on-chain audit persistence.
  - End-to-end Web3 and IPFS flows covering challenge, verify, upload, fetch, and audit endpoints.
  - Web and mobile client request alignment for Web3 verification payloads.

- **Mobile release stabilization**
  - Android integration test execution verified on physical Android device.
  - Android NDK configuration aligned with Flutter integration_test requirements.
  - Mobile navigation now exposes AI and Web3 entry points directly.

### Changed

- **Web realtime behavior**
  - SSE connection handling was consolidated to avoid duplicate connections and duplicate event consumption.
  - Nodes and tasks views now subscribe and clean up predictably during route transitions.

- **Mobile realtime behavior**
  - SSE reconnect and channel subscription handling were tightened for nodes and tasks updates.
  - Task log and status updates are now consumed through provider-level realtime hooks.

- **Release consistency**
  - Project version markers were aligned for the v2.5.0 final release line.

### Fixed

- WebSocket and SSE testability issues under Node/Vitest environments.
- Mobile AI/Web3 page import issues blocking Android integration builds.
- Flutter AppBar usage incompatible with current framework API.
- Mobile Android build configuration mismatch caused by outdated NDK pin.
- Web3 verification payload mismatch between clients and Workers endpoint expectations.

### Security

- Authenticated realtime endpoints continue to require bearer-token protection.
- Web3 challenge/verify and audit flows are validated through release test coverage.

### Quality

- Workers full test suite passed at release-prep time.
- Web full unit test suite passed at release-prep time.
- Mobile full unit test suite passed at release-prep time.
- Mobile integration tests passed on a connected Android 14 physical device.

## [v0.1.0] - 2026-03-15

### Added

- **Core Plugin System**
  - Plugin interface with lifecycle management
  - Plugin manager for registration and execution
  - Plugin registry for factory pattern

- **Plugins**
  - Ansible plugin for playbook execution
  - v2board plugin for panel management
  - V2bX plugin for proxy node management
  - AnixOps-agent plugin for remote control

- **API Layer**
  - REST API server with Gin framework
  - WebSocket hub for real-time communication
  - JWT authentication
  - RBAC authorization (admin/operator/viewer)

- **User Interfaces**
  - TUI with Bubble Tea framework
  - Web GUI with Vue 3 and Tailwind CSS

- **Security**
  - JWT token management
  - Password hashing with bcrypt
  - Role-based access control

- **Infrastructure**
  - SQLite database support
  - Event bus for pub/sub
  - Task scheduler with cron

- **Deployment**
  - Docker support
  - systemd service file
  - Multi-platform builds (Windows/macOS/Linux)

- **Testing**
  - Unit tests for core modules
  - Coverage reporting

### Security

- JWT-based authentication
- RBAC with three default roles
- Password hashing with bcrypt

---

## Version Naming Convention

- **Major (X.0.0)**: Breaking changes
- **Minor (0.X.0)**: New features, backward compatible
- **Patch (0.0.X)**: Bug fixes, backward compatible

## Branch Strategy

- **dev**: Development and pre-release validation branch
- **production**: Stable releases
- **master**: Final release line for source of truth in this repository
- **vX.X.X**: Frozen version branches (protected)

---

[Unreleased]: https://github.com/AnixOps/anixops-control-center/compare/v2.5.0...HEAD
[v2.5.0]: https://github.com/AnixOps/anixops-control-center/releases/tag/v2.5.0
[v0.1.0]: https://github.com/AnixOps/anixops-control-center/releases/tag/v0.1.0
