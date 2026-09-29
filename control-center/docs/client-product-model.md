# Client Product Model

This document defines the maintainable client product model for the AnixOps Control Center repository. It is the durable reference for how the client surfaces should be understood, configured, and kept in sync.

## Purpose

The repository contains more than one client surface. To keep future work maintainable, every client change should be planned against a shared product model instead of being treated as a one-off UI feature.

The model exists to:
- keep web and Flutter behavior aligned
- make backend targeting explicit
- separate core workflow surfaces from secondary or experimental surfaces
- keep auth, session, error, and realtime behavior consistent
- give future client work a stable reference point before implementation

## Current client surfaces

| Surface | Stack | Primary use | Platforms | Backend target | Status |
|---|---|---|---|---|---|
| Web control center | Vue 3, Vite, Pinia, Vue Router | Browser-first operator and admin console | Desktop browser | Configurable API base | Active |
| Flutter client | Flutter, Riverpod, GoRouter, Dio | Operator app for daily workflow and response tasks | Mobile, desktop, and other Flutter targets | Configurable API base | Active |
| CLI / TUI | Go-based CLI/TUI | Power-user and local operator workflows | Terminal | Local or remote API mode | Supported |

The important rule is that these are separate client surfaces, not interchangeable views of the same implementation.

## What the current code already shows

The repository already contains evidence that the client product is split across multiple shells and stacks:

- `web/src/App.vue` uses an authenticated shell with a sidebar layout for signed-in users.
- `web/src/views/Login.vue` shows the browser login flow is email/password based.
- `mobile/lib/core/services/api_client.dart` centralizes the Flutter API client, base URL, and auth header handling.
- `mobile/lib/core/router/app_router.dart` defines the Flutter route surface across dashboard, nodes, playbooks, tasks, schedules, plugins, users, logs, settings, notifications, AI, and Web3.
- `mobile/lib/features/auth/presentation/providers/auth_provider.dart` persists auth state locally and reconnects realtime after login.
- `DEVELOPMENT.md` and `docs/cloudflare-integration-plan.md` show that environment and deployment mode are part of the client contract, not just backend concerns.

## Product principles

### 1. The API is the source of truth

Clients should present, filter, and orchestrate state, but not invent canonical domain state locally.

Rules:
- do not duplicate business logic in multiple clients
- do not infer permissions from labels or navigation alone
- refresh authoritative server state after important actions
- treat local caches as temporary convenience, not canonical storage

### 2. Backend targeting must be explicit

The repository shows both local development and cloud-targeted operation. Client code should never assume a single hardcoded target.

The product model should always document:
- local development base URL
- production base URL
- whether a client can target both modes
- how auth tokens are issued and refreshed in each mode
- how realtime endpoints are reached in each mode

### 3. Auth and session behavior must be consistent across surfaces

Every client surface should follow the same high-level session rules:
- login creates an authenticated session
- tokens are stored using platform-appropriate secure storage
- logout clears local auth state
- expired sessions require reauthentication or refresh
- 401/403 responses should be handled consistently

The storage mechanism may differ by platform, but the behavior should not.

### 4. Realtime is helpful, not authoritative

Realtime delivery should improve responsiveness, but the server remains the source of truth.

Client rules:
- use realtime to hint at a refresh or a small UI update
- never assume realtime ordering is complete unless the client explicitly guarantees it
- always reconcile important state with a fresh API read
- remain functional if realtime is unavailable

### 5. Feature parity is tracked by domain, not by screen count

A feature is not “done” just because a page exists in one client. It is only complete when the domain behavior is documented and implemented for the intended surfaces.

The core domains that should stay aligned are:
- auth and session
- dashboard
- nodes
- playbooks
- tasks
- schedules
- notifications
- logs
- settings and admin flows
- AI assistant
- Web3 dashboard

Treat AI and Web3 as secondary until the core operational domains are stable and aligned.

## Recommended backend integration model

The client product model should assume two primary backend modes.

### Local development mode

Used for development, debugging, and offline validation.

Characteristics:
- local API endpoint
- fast iteration
- developer-controlled auth state
- direct visibility into backend errors

### Cloud production mode

Used for deployed and shared environments.

Characteristics:
- versioned remote API endpoint
- explicit auth and refresh behavior
- stricter environment control
- production-safe logging and retries

The client docs should always state which modes are supported and how the app chooses between them.

## Environment and configuration model

Each client surface should have a clear configuration contract.

### Web

Document:
- API base URL source
- development proxy behavior
- authentication persistence strategy
- environment-specific flags
- how route guards behave when auth is missing

### Flutter

Document:
- API base URL source
- secure token storage strategy
- SSE or realtime endpoint configuration
- platform-specific shell differences
- how auth refresh and logout work across app restarts

### CLI / TUI

Document:
- local vs remote mode selection
- auth method
- output and error handling
- whether it shares the same API contract as the web and Flutter clients

## State ownership

The client model should distinguish three kinds of state:

### Canonical state

State that lives on the server and must be re-read after major actions.

Examples:
- auth status
- nodes
- playbooks
- tasks
- schedules
- permissions
- notifications

### Ephemeral UI state

State that belongs only to the current view or session.

Examples:
- dialog visibility
- loading indicators
- selected tab
- temporary filters

### Local persistence state

State stored on the client for convenience only.

Examples:
- auth tokens
- refresh tokens
- saved preferences
- last-used filters if the UI intentionally persists them

The rule is simple: local persistence must never override canonical server truth.

## Feature ownership by domain

| Domain | Product rule |
|---|---|
| Auth | Shared session behavior across all surfaces |
| Dashboard | Shared summary data and loading/error behavior |
| Nodes | Core operational feature; must stay aligned across clients |
| Playbooks | Core operational feature; execution and inspection should match |
| Tasks | Core operational feature; logs and retry behavior should be consistent |
| Schedules | Core operational feature; editing and execution rules should match |
| Notifications | Shared prioritization surface; not the source of truth |
| Logs | Shared operational visibility surface |
| Settings | Surface-specific controls with shared auth rules |
| AI | Advisory only; never bypass workflow or auth rules |
| Web3 | Secondary/experimental unless explicitly promoted |

## Maintenance rules

When any of these change, the client product model must be updated:
- a new client surface is added
- the API base URL changes
- auth or token storage changes
- route names or navigation structure changes
- realtime channels are added or removed
- a core domain changes behavior
- a feature becomes production-ready or is marked experimental

## Phased maintenance plan

### Phase 1: lock the surface inventory
- keep the current client surface list current
- mark which surfaces are primary, secondary, or experimental
- avoid feature duplication across surfaces

### Phase 2: normalize integration and auth
- keep backend targeting documented
- keep login, logout, refresh, and 401 handling aligned
- keep secure storage rules platform-appropriate

### Phase 3: keep parity visible
- maintain a feature parity matrix
- record which domain owns which screen or route
- distinguish implemented, partial, and planned work

### Phase 4: keep docs synced
- update this model whenever a route, auth rule, or environment rule changes
- link new client docs back to this file
- treat this file as the starting point for new client work

## Related documentation

- `docs/ARCHITECTURE.md`
- `docs/cloudflare-integration-plan.md`
- `DEVELOPMENT.md`
- `README.md`
