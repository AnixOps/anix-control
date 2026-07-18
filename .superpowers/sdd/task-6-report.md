# Task 6 Report: Deployment Center Migration

## Scope

Delivered the Deployment Center route at `/admin/deployments`, migrated the assignment fixture browser flow, retained the Task 1 legacy `/admin/control` redirect, and removed the legacy Control view and test once replacement coverage was in place.

## RED / GREEN

### RED

Command:

```bash
npm --prefix web run test -- src/__tests__/Deployments.test.js
```

Result before implementation: `9 failed`. The authorized `Deployments.vue` stub rendered only `data-testid="admin-route-placeholder"`; the new tests failed on absent deployment selectors and zero API calls. This established that the migrated tests exercised the replacement route rather than legacy Control behavior.

### GREEN

After implementing the route coordinator:

```bash
npm --prefix web run test -- src/__tests__/Deployments.test.js
```

Result: `1 passed`, `9 passed`.

The route tests cover:

- Initial loading limited to topology, deployment, operation, scope, and node resources.
- Deferred node-assignment loading and deferred plugin/release/installation metadata until the assignment drawer opens.
- Assignment create, edit, toggle, delete, and drawer-local server error visibility.
- Topology creation, revision loading, validation/diagnose, revision saving, preview/plan staging, snake_case plan payload mapping, apply refresh, and guarded rollback.
- Scoped operation cancellation/polling and an initial backend error without an empty-state flash.

### Review Regression Cycle

Independent review found three scoped-lifecycle gaps and one recovery edge case. Each received a focused RED/GREEN cycle in `Deployments.test.js`:

- Assignment mutation refreshes initially could not start polling because the shared Axios response interceptor returns `response.data` and drops the backend's `X-AnixOps-Operation-ID` header. The route now uses its existing post-mutation `GET /operations` refresh, tracks active rows matching the selected `node_id`, and reconciles those tracked IDs when the rows become terminal or disappear. This keeps the API wrapper and backend unchanged.
- Deployment activity now recognizes the server projection field `topology_deployment_id`.
- Planned deployments are inert and now do not start a polling interval; only `applying` and `rollback_requested` selected deployments poll.
- A successful topology POST now enters the revision editor before its follow-up list refresh; a list-refresh error remains visible without making a retry create a duplicate.

Final focused command:

```bash
npm --prefix web run test -- src/__tests__/Deployments.test.js
```

Result: `1 passed`, `13 passed`. The assignment-operation regression asserts polling starts on a selected-node operation, stops on its terminal state, and does not reload topology or deployment collections.

## Verification

Focused domain command:

```bash
npm --prefix web run test -- src/__tests__/Plugins.test.js src/__tests__/Deployments.test.js src/__tests__/useKernelPlugins.test.js src/__tests__/useKernelDeployments.test.js src/__tests__/AdminLayout.test.js src/__tests__/adminRoutes.test.js
```

Result: `6 passed`, `55 passed`.

Post-deletion full unit command:

```bash
npm --prefix web run test
```

Result: `56 passed`, `329 passed`.

Build command:

```bash
npm --prefix web run build
```

Result: passed. Vite emitted its existing large-chunk advisory only.

Fixture browser command after the parent-authorized bare-Control redirect correction:

```bash
npm --prefix web run test:e2e -- e2e/control-assignments.spec.js e2e/plugin-webui.spec.js
```

Result: `4 passed`.

- `control-assignments.spec.js` passed, including the narrow viewport no-overflow assertion and the new target/drawer selectors.
- All Plugin WebUI cases passed after the parent-authorized bare-Control fallback updates to `/admin/plugins` and the matching `Plugins` heading.

Initial browser execution could not launch because Chromium was absent. `npx --prefix web playwright install chromium` installed the declared Playwright runtime, after which the application-level fixture result above was obtained.

Live-control browser command:

```bash
npm --prefix web run test:e2e -- e2e/live-control-machine-telemetry.spec.js
```

Result: unavailable local-backend baseline before any test assertion. Vite's `/api/v2/login` proxy failed with `AggregateError [ECONNREFUSED]`, producing Axios 502. The updated post-revocation fallback assertion remains `/admin/plugins`; it was not weakened.

## Changed Files

- Modified `web/src/views/admin/Deployments.vue`.
  - Uses `TopologyWorkspace`, `AssignmentDrawer`, and `OperationTimeline` from Task 5.
  - Starts in Topologies mode; Targets is the only other primary view.
  - Maps `rolloutGroup` and `failurePolicy` to `rollout_group` and `failure_policy` only at the plan request boundary.
  - Polls only the selected active deployment or tracked assignment-related operation, and clears the timer on unmount.
- Added `web/src/__tests__/Deployments.test.js`.
- Modified `web/e2e/control-assignments.spec.js` for `/admin/deployments` and stable target/drawer selectors.
- Modified `web/e2e/live-control-machine-telemetry.spec.js` only at its requested fallback expectation.
- Modified `web/e2e/plugin-webui.spec.js` only at the two parent-authorized stale bare-Control fallback assertions (URL and resulting heading).
- Deleted `web/src/views/admin/Control.vue`.
- Deleted `web/src/__tests__/Control.test.js`.

`adminRoutes.test.js` already verifies both required Task 1 redirects: deployment-related legacy tabs route to `/admin/deployments`, and a bare `/admin/control` routes to `/admin/plugins`; no change was required there.

## Self-Review

- Confirmed the initial route load does not call assignments, plugins, releases, or installations.
- Confirmed the assignment drawer loads its form metadata lazily and errors remain at the action surface.
- Confirmed staged plan flow performs diagnose, preview, validity gating, then plan and status refresh.
- Confirmed apply and rollback refresh selected deployment status and topology rows.
- Confirmed assignment mutation activity derives from the public `/operations` projection after headers are intentionally unwrapped, stays node-scoped, and stops polling on a terminal row without broad reloads.
- Confirmed legacy Control files have no remaining imports and `/admin/control` remains a router redirect.
- Ran `git diff --check` with no whitespace errors.

## Independent Review

Read-only review against `4ec7054` found and drove the scoped-lifecycle and create-refresh regressions above. Final assessment: approved with no Critical, Important, or Minor findings.

## Concerns

- The live-control browser test requires a local backend that is not running in this workspace; its failure is pre-assertion `ECONNREFUSED`, not a UI regression.

## Fix Round Task 6

### Formal Review Fixes

The formal review identified five migration-scope regressions. No backend,
API-wrapper, or authorization behavior changed.

- Assignment deletion now asks for the existing localized destructive
  confirmation before calling the delete API. Focused coverage proves cancel
  makes no delete request and confirm deletes the selected assignment. The
  assignment fixture browser flow explicitly accepts this native confirmation.
- Assignment target mutations capture their initiating node ID. The selector,
  refresh action, and create action are disabled while an assignment mutation
  is pending. Assignment responses are request-ordered and only write rows for
  the current Targets selection; an early Targets entry waits for the initial
  node load and then loads the default node's assignments.
- Polling now treats any nonterminal operation row scoped to the selected
  Targets node as active work, in addition to explicitly tracked operation
  IDs. A pre-existing selected-node operation starts polling and the timer
  stops after its row becomes terminal or disappears.
- Deployment status reads carry the current topology-editor session plus the
  expected topology and deployment IDs. Closing, opening, or switching the
  editor invalidates prior status responses; poll-driven status responses use
  the same guard.
- Plugin Center now renders nonempty `adminExtensionErrors` in an accessible
  alert band, including the originating plugin ID and runtime error message.

### RED / GREEN Evidence

Focused RED cases were added before their corresponding implementation:

- A selected-target operation already present in the initial operation list
  did not start the polling timer (`getKernelOperations` remained at one call).
- A deferred deployment-status response for topology A attached deployment
  `#44` to topology B after the editor switched.
- A populated extension runtime error list had no Plugin Center alert band.
- The existing assignment browser fixture auto-dismissed the restored native
  delete confirmation and therefore never deleted its fixture row.

Final focused unit command:

```bash
npm --prefix web run test -- src/__tests__/Plugins.test.js src/__tests__/Deployments.test.js
```

Result: `2 passed`, `33 passed`.

### Verification

Full web unit command:

```bash
npm --prefix web run test
```

Result: `56 passed`, `336 passed`.

Build command:

```bash
npm --prefix web run build
```

Result: passed. Vite emitted only its existing large-chunk advisory.

Fixture browser command:

```bash
npm --prefix web run test:e2e -- e2e/control-assignments.spec.js e2e/plugin-webui.spec.js
```

Result: `4 passed`. The WebUI fixture still emits its expected router warning
for the intentionally rejected tampered extension route.

### Changed Files

- Modified `web/src/views/admin/Deployments.vue` and
  `web/src/__tests__/Deployments.test.js` for destructive confirmation,
  race-safe target activity, scoped operation polling, and topology-editor
  status-session isolation.
- Modified `web/src/views/admin/Plugins.vue` and
  `web/src/__tests__/Plugins.test.js` for extension runtime error visibility.
- Modified `web/e2e/control-assignments.spec.js` to accept the restored native
  delete confirmation before asserting the fixture deletion.
