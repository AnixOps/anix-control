# Admin Workbench Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the monolithic Control Kernel page with a compact Plugin Center and Deployment Center, and rebuild the administrator shell into a restrained, high-frequency operations workbench.

**Architecture:** Keep the existing Go/kernel APIs untouched. Add two route-level Vue views backed by focused composables: the Plugin Center loads only catalog/install resources, while the Deployment Center loads topology/target/activity resources and fetches assignments only for the selected node. The compact navigation shell owns grouping, collapse state, responsive behavior, permissions, and extension menu placement; domain views own loading, mutation, and polling lifecycles.

**Tech Stack:** Vue 3 Composition API, Vue Router 4, Pinia, vue-i18n, Vitest, Vue Test Utils, Playwright, Vite, and `lucide-vue-next`.

## Global Constraints

- Do not change any endpoint, request shape, response shape, authorization semantic, or anix-agent behavior.
- Keep every administrator route reachable. Use explicit `More` disclosures for low-frequency routes; do not silently hide a core route.
- Preserve dynamic extension discovery, permission filtering, registered parent ordering, and fail-closed extension routing.
- `/admin/control` must continue to work and map legacy `tab` query values to the new routes.
- Reuse `PluginConfigForm.vue` for schema-driven and JSON configuration editing, including defaults, validation, and `expected_revision` protection.
- Use solid surfaces, 1px dividers, shallow shadows, and corners no larger than 8px. Do not introduce gradients, marketing heroes, decorative card grids, manually drawn SVG icons, or text-initial icons.
- Use Lucide icons with accessible labels and tooltips for icon-only controls. Health state must be conveyed by text/icon as well as color.
- Keep light/dark themes, English/Chinese translations, keyboard operation, focus restoration, mobile drawer behavior, and no-horizontal-overflow behavior.
- Work in a fresh isolated worktree when executing this plan; do not implement directly in the primary `go_dev` worktree.

## File Map

| Path | Responsibility |
| --- | --- |
| `web/src/router/controlLegacy.js` | Pure legacy Control-tab redirect mapping. |
| `web/src/router/index.js` | Plugins/Deployments routes and legacy redirect registration. |
| `web/src/components/admin/AdminNavIcon.vue` | Safe Lucide mapping for core and dynamic navigation entries. |
| `web/src/components/admin/AdminNavigation.vue` | Five-group compact navigation, `More`, mobile close, and collapse presentation. |
| `web/src/layouts/AdminLayout.vue` | Shell state, navigation data, title/breadcrumb, version/logout controls. |
| `web/src/utils/kernelPluginRelease.js` | Shared manifest parsing and release-target extraction. |
| `web/src/composables/useKernelPlugins.js` | Catalog loading and one-row-per-plugin data. |
| `web/src/views/admin/Plugins.vue` | Plugin Center filters, mutations, feedback, and scoped polling. |
| `web/src/components/admin/PluginDetailDrawer.vue` | Target-scoped plugin details and actions. |
| `web/src/components/admin/PluginInstallationDialog.vue` | Install/upgrade version selection. |
| `web/src/components/admin/PluginReleaseImportDialog.vue` | Manifest/signature/artifact import. |
| `web/src/composables/useKernelDeployments.js` | Deployment loading, topology conversion, assignment payloads, polling helpers. |
| `web/src/views/admin/Deployments.vue` | Topology/target/activity route orchestration. |
| `web/src/components/admin/TopologyWorkspace.vue` | Revision edit, diagnose, preview, plan, apply, rollback. |
| `web/src/components/admin/AssignmentDrawer.vue` | Create/edit target assignment. |
| `web/src/components/admin/OperationTimeline.vue` | Scoped activity timeline and all-history expansion. |
| `web/src/views/admin/Control.vue` | Delete only after the two replacement routes cover its behavior. |

## Task 1: Establish Route, Copy, Icon, and Legacy-Redirect Contracts

**Files:**
- Create: `web/src/router/controlLegacy.js`
- Create: `web/src/__tests__/controlLegacy.test.js`
- Create: `web/src/views/admin/Plugins.vue` (temporary route stub; Task 4 replaces it)
- Create: `web/src/views/admin/Deployments.vue` (temporary route stub; Task 6 replaces it)
- Modify: `web/package.json`
- Modify: `web/package-lock.json`
- Modify: `web/src/router/index.js`
- Modify: `web/src/utils/pageMeta.js`
- Modify: `web/src/locales/en.js`
- Modify: `web/src/locales/zh-CN.js`
- Modify: `web/src/__tests__/adminRoutes.test.js`
- Modify: `web/src/__tests__/pageMeta.test.js`

**Interfaces:**
- `resolveLegacyControlRedirect(to)` accepts `query` and `hash` and returns `{ path, query, hash }`.
- `LEGACY_DEPLOYMENT_TABS` is a `Set` containing `assignments`, `scopes`, `topologies`, and `operations`.
- The router exposes `/admin/plugins`, `/admin/deployments`, and `/admin/control`; the last path uses the redirect helper.
- The two temporary views exist solely so Vite can statically resolve lazy
  imports while route-contract tests run; they contain no product UI and are
  replaced by their named route tasks.

- [ ] **Step 1: Write failing redirect and metadata tests.**

```js
// web/src/__tests__/controlLegacy.test.js
import { describe, expect, it } from 'vitest'
import { resolveLegacyControlRedirect } from '@/router/controlLegacy'

describe('legacy Control redirects', () => {
  it.each([
    [undefined, '/admin/plugins'],
    ['plugins', '/admin/plugins'],
    ['assignments', '/admin/deployments'],
    ['scopes', '/admin/deployments'],
    ['topologies', '/admin/deployments'],
    ['operations', '/admin/deployments'],
    ['unknown', '/admin/plugins'],
  ])('maps tab %s to %s', (tab, path) => {
    const query = tab === undefined ? { keep: 'yes' } : { tab, keep: 'yes' }
    expect(resolveLegacyControlRedirect({ query, hash: '#activity' })).toEqual({
      path,
      query: { keep: 'yes' },
      hash: '#activity',
    })
  })
})
```

Extend `pageMeta.test.js` with `/admin/plugins` and `/admin/deployments` title
expectations. Extend `adminRoutes.test.js` to include both new paths and to
expect denied extension routes to land on `/admin/plugins`.

- [ ] **Step 2: Run the focused test set and verify it fails.**

Run:

```bash
npm --prefix web run test -- src/__tests__/controlLegacy.test.js src/__tests__/adminRoutes.test.js src/__tests__/pageMeta.test.js
```

Expected: FAIL because the helper, routes, metadata, and translation keys do
not exist.

- [ ] **Step 3: Implement the route contract and labels.**

Run:

```bash
npm --prefix web install lucide-vue-next
```

Create `web/src/router/controlLegacy.js`:

```js
export const LEGACY_DEPLOYMENT_TABS = new Set([
  'assignments',
  'scopes',
  'topologies',
  'operations',
])

export function resolveLegacyControlRedirect(to) {
  const query = { ...(to?.query || {}) }
  const tab = typeof query.tab === 'string' ? query.tab : ''
  delete query.tab
  return {
    path: LEGACY_DEPLOYMENT_TABS.has(tab) ? '/admin/deployments' : '/admin/plugins',
    query,
    hash: to?.hash || '',
  }
}
```

In `router/index.js`, import the helper; replace the Control lazy import with
lazy imports for `Plugins.vue` and `Deployments.vue`; register `path: 'plugins'`
and `path: 'deployments'`; set `path: 'control'` to
`redirect: resolveLegacyControlRedirect`; and change every extension fallback
from `'/admin/control'` to `'/admin/plugins'`.

Add `plugins`/`deployments` page title and nav keys to both locales, with
`Plugins`/`Deployments` in English and `插件`/`部署` in Chinese. Add section keys
`business`, `network`, `controlCenter`, and `more`. Map both paths in
`pageMeta.js`. Retain the existing `control` copy for compatibility.

Create the two temporary route files with no data fetching or actions:

```vue
<template><section data-testid="admin-route-placeholder"></section></template>
```

They are a Vite import bridge only. Task 4 and Task 6 replace the respective
files with complete route implementations before any production build or
visual acceptance run.

- [ ] **Step 4: Run the focused route set and verify it passes.**

Run:

```bash
npm --prefix web run test -- src/__tests__/controlLegacy.test.js src/__tests__/adminRoutes.test.js src/__tests__/pageMeta.test.js
```

Expected: PASS. In the route test, assert `/admin/control?tab=assignments`
resolves to `/admin/deployments` and `/admin/control` resolves to
`/admin/plugins`.

- [ ] **Step 5: Commit the compatibility foundation.**

```bash
git add web/package.json web/package-lock.json web/src/router/controlLegacy.js web/src/router/index.js web/src/utils/pageMeta.js web/src/locales/en.js web/src/locales/zh-CN.js web/src/views/admin/Plugins.vue web/src/views/admin/Deployments.vue web/src/__tests__/controlLegacy.test.js web/src/__tests__/adminRoutes.test.js web/src/__tests__/pageMeta.test.js
git commit -m "feat(admin): add plugin and deployment routes"
```

## Task 2: Build the Compact Administrator Shell and Navigation

**Files:**
- Create: `web/src/components/admin/AdminNavIcon.vue`
- Create: `web/src/components/admin/AdminNavigation.vue`
- Modify: `web/src/layouts/AdminLayout.vue`
- Modify: `web/src/components/admin/ForwardSuiteNav.vue`
- Modify: `web/src/style.css`
- Modify: `web/src/__tests__/AdminLayout.test.js`
- Modify: `web/src/__tests__/ForwardSuiteNav.test.js`

**Interfaces:**
- `AdminNavIcon` accepts `name` and optional `size`; it renders a fixed-size
  Lucide icon, with `Box` as fallback for unknown extension icon names.
- `AdminNavigation` accepts `sections`, `collapsed`, and `mobile`; it emits
  `navigate` and `toggle-collapse`.
- A section is `{ id, label, items, advancedItems?, kind? }`; an item is
  `{ to, icon, label, hint? }`.

- [ ] **Step 1: Write failing shell behavior tests.**

Add tests to `AdminLayout.test.js` that assert the new groups and collapse
contract:

```js
expect(wrapper.find('[data-nav-group="overview"]').text()).toContain('Dashboard')
expect(wrapper.find('[data-nav-group="control-center"] a[data-to="/admin/plugins"]').exists()).toBe(true)
expect(wrapper.find('[data-nav-group="control-center"] a[data-to="/admin/deployments"]').exists()).toBe(true)
await wrapper.get('[aria-label="Collapse navigation"]').trigger('click')
expect(localStorage.setItem).toHaveBeenCalledWith('admin.sidebar.collapsed', 'true')
expect(wrapper.find('.admin-layout').classes()).toContain('navigation-collapsed')
```

Move existing extension sorting assertions under
`[data-nav-group="control-center"]`. Add a mobile test that opens the drawer,
clicks a route, and asserts the menu button returns to `aria-expanded="false"`.

- [ ] **Step 2: Run the shell test set and verify it fails.**

Run:

```bash
npm --prefix web run test -- src/__tests__/AdminLayout.test.js src/__tests__/ForwardSuiteNav.test.js
```

Expected: FAIL because the new groups, controls, data attributes, and collapse
state do not exist.

- [ ] **Step 3: Implement the navigation primitives and shell composition.**

Use a closed icon mapping in `AdminNavIcon.vue` so extension data cannot select
an arbitrary component:

```js
import { Box, ChartNoAxesCombined, Gauge, LayoutDashboard, Network, Package, Rocket, Settings, Users } from 'lucide-vue-next'

const icons = Object.freeze({
  dashboard: LayoutDashboard,
  monitor: Gauge,
  traffic: ChartNoAxesCombined,
  users: Users,
  network: Network,
  plugins: Package,
  deployments: Rocket,
  system: Settings,
})

const icon = computed(() => icons[props.name] || Box)
```

Move navigation markup from `AdminLayout.vue` into `AdminNavigation.vue`. Build
five groups with these route placements:

```js
[
  { id: 'overview', items: ['/admin/dashboard', '/admin/monitor', '/admin/traffic-hourly'] },
  { id: 'business', items: ['/admin/users', '/admin/orders', '/admin/tickets'], advancedItems: ['/admin/subscriptions', '/admin/plans', '/admin/coupons', '/admin/invite', '/admin/payment', '/admin/knowledge'] },
  { id: 'network', items: ['/admin/nodes'], kind: 'forward' },
  { id: 'control-center', items: ['/admin/plugins', '/admin/deployments'], extensionItems: true },
  { id: 'system', items: ['/admin/mfa', '/admin/access-groups', '/admin/system'], advancedItems: ['/admin/telegram', '/admin/notifications'] },
]
```

Render `advancedItems` in native `<details>` with translated `More` text. Keep
dynamic extensions permission-gated and sorted by their registered parent, but
render them inside Control Center. Preserve the Forward Suite's current core
and advanced route grouping while replacing text-initial glyphs with icon
components.

Persist the desktop preference under `admin.sidebar.collapsed`; apply
`navigation-collapsed` to the root. In collapsed desktop mode hide labels but
keep `title` and `aria-label`; below the tablet breakpoint retain the full
drawer. Keep version, theme/locale, logout, page title, breadcrumb, and main
landmark behavior.

Replace the global `--body-gradient` and the sidebar gradient with solid
semantic surfaces. Use 1px dividers, shallow shadows, and no corner above 8px.

- [ ] **Step 4: Run the shell-focused regression set and verify it passes.**

Run:

```bash
npm --prefix web run test -- src/__tests__/AdminLayout.test.js src/__tests__/ForwardSuiteNav.test.js src/__tests__/adminRoutes.test.js
```

Expected: PASS. All old routes are present in a primary group or `More`,
extension menus remain sorted/permission-gated, and the compact rail has stable
accessible names.

- [ ] **Step 5: Commit the shell redesign.**

```bash
git add web/src/components/admin/AdminNavIcon.vue web/src/components/admin/AdminNavigation.vue web/src/layouts/AdminLayout.vue web/src/components/admin/ForwardSuiteNav.vue web/src/style.css web/src/__tests__/AdminLayout.test.js web/src/__tests__/ForwardSuiteNav.test.js
git commit -m "feat(admin): compact the administrator navigation"
```

## Task 3: Extract a Single-Row Plugin Catalog and Target Detail Components

**Files:**
- Create: `web/src/utils/kernelPluginRelease.js`
- Create: `web/src/composables/useKernelPlugins.js`
- Create: `web/src/components/admin/PluginDetailDrawer.vue`
- Create: `web/src/components/admin/PluginInstallationDialog.vue`
- Create: `web/src/components/admin/PluginReleaseImportDialog.vue`
- Create: `web/src/__tests__/useKernelPlugins.test.js`
- Create: `web/src/__tests__/PluginDetailDrawer.test.js`

**Interfaces:**
- `parseManifest(release)` returns a manifest object or `{}` when the stored
  manifest is malformed.
- `releaseTargets(release)` returns only `control` and `agent` target names.
- `buildPluginRows(plugins, releases, installations)` returns exactly one row
  per plugin with `{ key, plugin, releases, installations, targets, latestRelease, health }`.
- `useKernelPlugins()` returns `plugins`, `releases`, `installations`, `rows`,
  `loading`, `loaded`, `error`, and `load({ silent })`.
- `PluginDetailDrawer` receives `row`, `busyTarget`, and `open`; it emits
  `close`, `install`, `configure`, and `lifecycle` with `{ installation, action }`.
- `PluginInstallationDialog` emits `save` with `{ target, version, enabled }`.
- `PluginReleaseImportDialog` emits `save` with `{ manifest, signature, artifactBase64 }`.

- [ ] **Step 1: Write failing derived-row and drawer tests.**

Use fixtures with one plugin, control/agent releases, and two installations.
The derived-row test must prove there is one row and two target summaries:

```js
const rows = buildPluginRows([plugin], [controlRelease, agentRelease], [controlInstallation, agentInstallation])
expect(rows).toHaveLength(1)
expect(rows[0].targets.map(target => target.target)).toEqual(['agent', 'control'])
expect(rows[0].health).toEqual({ state: 'attention', error: 'agent failed' })
```

Mount `PluginDetailDrawer` and assert that selecting a target exposes desired
and observed versions, emits `configure` for an installed target, and emits
`install` for a catalog-only target. Assert `role="dialog"`,
`aria-modal="true"`, and route-level focus restoration after close.

- [ ] **Step 2: Run the component tests and verify they fail.**

Run:

```bash
npm --prefix web run test -- src/__tests__/useKernelPlugins.test.js src/__tests__/PluginDetailDrawer.test.js
```

Expected: FAIL because the composable and drawer components do not exist.

- [ ] **Step 3: Implement the catalog composable and presentational components.**

Implement the shared release helper and target collection without duplicating
plugin rows:

```js
// web/src/utils/kernelPluginRelease.js
export function parseManifest(release) {
  try {
    return typeof release?.manifest === 'string' ? JSON.parse(release.manifest) : release?.manifest || {}
  } catch {
    return {}
  }
}

export function releaseTargets(release) {
  const targets = parseManifest(release).targets
  return Array.isArray(targets) ? targets.filter(target => target === 'control' || target === 'agent') : []
}

// web/src/composables/useKernelPlugins.js
export function buildPluginRows(plugins, releases, installations) {
  return plugins.map(plugin => {
    const pluginReleases = releases.filter(release => release.plugin_id === plugin.id)
    const targetIDs = new Set(pluginReleases.flatMap(releaseTargets))
    const pluginInstallations = installations.filter(item => item.plugin_id === plugin.id)
    for (const installation of pluginInstallations) targetIDs.add(installation.target)
    const targets = [...targetIDs].sort().map(target => {
      const targetReleases = pluginReleases.filter(release => releaseTargets(release).includes(target))
      const installation = pluginInstallations.find(item => item.target === target) || null
      return { target, releases: targetReleases, installation, latestRelease: targetReleases[0] || null }
    })
    const failed = targets.find(item => item.installation?.last_error)
    const unhealthy = targets.find(item => item.installation && item.installation.state !== 'healthy')
    return {
      key: plugin.id,
      plugin,
      releases: pluginReleases,
      installations: pluginInstallations,
      targets,
      latestRelease: pluginReleases[0] || null,
      health: failed ? { state: 'attention', error: failed.installation.last_error } : unhealthy ? { state: 'attention', error: '' } : { state: pluginInstallations.length ? 'healthy' : 'catalogued', error: '' },
    }
  })
}
```

`useKernelPlugins.load()` calls only `getKernelPlugins`,
`getKernelPluginReleases`, and `getKernelInstallations` in one `Promise.all`.
It preserves existing rows during a silent refresh and clears rows only after a
non-silent initial-load failure. Reuse `PluginConfigForm` in the route/dialog
layer; do not duplicate schema validation.

Implement drawers/dialogs with a focusable close control, Escape-close when
not saving, click-outside close only when not saving, mobile full-screen styles,
and visible close/error labels. Use Lucide icons with tooltips for compact
controls.

- [ ] **Step 4: Run the component regression set and verify it passes.**

Run:

```bash
npm --prefix web run test -- src/__tests__/useKernelPlugins.test.js src/__tests__/PluginDetailDrawer.test.js src/__tests__/PluginConfigForm.test.js
```

Expected: PASS. One plugin appears once regardless of target count, target
details remain accessible, and the existing config form still passes.

- [ ] **Step 5: Commit the plugin-domain building blocks.**

```bash
git add web/src/utils/kernelPluginRelease.js web/src/composables/useKernelPlugins.js web/src/components/admin/PluginDetailDrawer.vue web/src/components/admin/PluginInstallationDialog.vue web/src/components/admin/PluginReleaseImportDialog.vue web/src/__tests__/useKernelPlugins.test.js web/src/__tests__/PluginDetailDrawer.test.js
git commit -m "feat(admin): add grouped plugin catalog components"
```

## Task 4: Deliver the Plugin Center Route and Preserve Lifecycle Semantics

**Files:**
- Modify: `web/src/views/admin/Plugins.vue`
- Create: `web/src/__tests__/Plugins.test.js`
- Modify: `web/src/components/admin/PluginDetailDrawer.vue`
- Modify: `web/src/components/admin/PluginInstallationDialog.vue`
- Modify: `web/src/components/admin/PluginReleaseImportDialog.vue`
- Modify: `web/src/__tests__/kernelApi.test.js` only if an existing API mock needs a new asserted invocation

**Interfaces:**
- `Plugins.vue` uses `data-testid="plugin-list"`, `data-testid="plugin-detail-drawer"`, and
  `data-testid="plugin-row-<plugin-id>"` for stable browser checks.
- The view supplies `runLifecycle(target, action, targetVersion)`,
  `saveInstallation(input)`, `saveConfig(input)`, and `importRelease(input)`.
- Successful plugin mutations refresh only plugin resources and call
  `refreshAdminExtensions(router)` after the resource refresh completes.

- [ ] **Step 1: Write failing Plugin Center integration tests.**

Move lifecycle coverage out of `Control.test.js` and assert route-specific
loading and grouped output:

```js
expect(kernelApi.getKernelTopologies).not.toHaveBeenCalled()
expect(wrapper.findAll('[data-testid^="plugin-row-"]')).toHaveLength(1)
await wrapper.get('[data-testid="plugin-row-protocol-runtime"]').trigger('click')
expect(wrapper.get('[data-testid="plugin-detail-drawer"]').text()).toContain('agent')
```

Port catalog installation, control-target idempotency, agent-target upsert
lifecycle, optimistic config revision, release import, and active-operation
cancellation tests. Add a failure assertion that an
`updateKernelInstallationConfig` rejection leaves the drawer open and retains
the edited configuration model.

- [ ] **Step 2: Run the Plugin Center test file and verify it fails.**

Run:

```bash
npm --prefix web run test -- src/__tests__/Plugins.test.js
```

Expected: FAIL because `Plugins.vue` does not exist.

- [ ] **Step 3: Implement the route using scoped data and current lifecycle rules.**

Render a page header with search, health filter, target filter, refresh icon
button, and one primary `Import release` action. Render one compact row per
`buildPluginRows()` result. Keep the selected row in a `ref`, record the trigger
element before opening the drawer, and focus it again after close.

Preserve the existing control-vs-agent mutation contract:

```js
async function runLifecycle(target, action, targetVersion = '') {
  if (target.target === 'control') {
    const result = await runKernelInstallationAction(target.installation.id, action, {
      targetVersion,
      idempotencyKey: `webui:${target.installation.id}:${action}:${createIdempotencyToken()}`,
    })
    if (result?.operation?.id) trackedOperationIDs.add(result.operation.id)
  } else {
    await upsertKernelInstallation({
      plugin_id: selectedRow.value.plugin.id,
      target: target.target,
      desired_version: action === 'rollback' ? target.installation.previous_version : targetVersion || target.installation.desired_version,
      enabled: action !== 'disable',
    })
  }
  await refreshPluginResources({ silent: true })
  await refreshAdminExtensions(router)
}
```

Define the token helper in the same route module:

```js
function createIdempotencyToken() {
  return globalThis.crypto?.randomUUID?.() || `${Date.now()}-${Math.random().toString(16).slice(2)}`
}
```

Keep release artifact conversion chunked exactly as in `Control.vue`. Poll only
while a tracked or plugin-lifecycle operation is active; poll
`getKernelOperations` and `getKernelInstallations`, stop at terminal states,
and clear the timer on unmount. Do not fetch topologies, deployments, nodes,
scopes, or assignments from this route.

- [ ] **Step 4: Run Plugin Center and related regression tests.**

Run:

```bash
npm --prefix web run test -- src/__tests__/Plugins.test.js src/__tests__/useKernelPlugins.test.js src/__tests__/PluginDetailDrawer.test.js src/__tests__/PluginConfigForm.test.js src/__tests__/kernelApi.test.js
```

Expected: PASS. Confirm a config conflict retains the draft, a release import
refreshes the catalog, and extension refresh runs after successful lifecycle
mutation.

- [ ] **Step 5: Commit the Plugin Center route.**

```bash
git add web/src/views/admin/Plugins.vue web/src/components/admin/PluginDetailDrawer.vue web/src/components/admin/PluginInstallationDialog.vue web/src/components/admin/PluginReleaseImportDialog.vue web/src/__tests__/Plugins.test.js web/src/__tests__/kernelApi.test.js
git commit -m "feat(admin): add compact plugin center"
```

## Task 5: Extract Deployment Data, Topology, Assignment, and Activity Components

**Files:**
- Create: `web/src/composables/useKernelDeployments.js`
- Create: `web/src/components/admin/TopologyWorkspace.vue`
- Create: `web/src/components/admin/AssignmentDrawer.vue`
- Create: `web/src/components/admin/OperationTimeline.vue`
- Create: `web/src/__tests__/useKernelDeployments.test.js`
- Create: `web/src/__tests__/TopologyWorkspace.test.js`
- Create: `web/src/__tests__/AssignmentDrawer.test.js`

**Interfaces:**
- `topologyInputFromJSON(json, message, invalidJSONMessage)` returns `{ message, vertices, edges }` or throws `new Error(invalidJSONMessage)`.
- `topologyJSONFromDetail(detail)` returns canonical, indented revision JSON.
- `assignmentPayload(source, overrides)` returns `{ service_scope, plugin_id, role, desired_version, desired_config_revision, enabled, rollout_group }`.
- `TopologyWorkspace` emits `create-topology`, `save-revision`, `diagnose`, `preview`, `plan`, `apply`, and `rollback`.
- `AssignmentDrawer` emits `save` with `{ nodeID, payload }` and disables identity controls in edit mode.
- `OperationTimeline` emits `cancel` only for cancellable operations.

- [ ] **Step 1: Write failing pure-data and component tests.**

Add a pure assignment payload test:

```js
expect(assignmentPayload({
  service_scope: 'forward', plugin_id: 'gost-mesh', role: 'relay',
  desired_version: '1.0.0', desired_config_revision: -2, enabled: 1,
  rollout_group: 'canary-a',
})).toEqual({
  service_scope: 'forward', plugin_id: 'gost-mesh', role: 'relay',
  desired_version: '1.0.0', desired_config_revision: 0, enabled: true,
  rollout_group: 'canary-a',
})
```

Add topology JSON round-trip coverage with vertices, edges, optional node IDs,
and config objects. Mount `AssignmentDrawer` in edit mode and assert node,
plugin, scope, and role are disabled while version, revision, rollout group,
and enabled state are editable. Mount `OperationTimeline` with a running and a
completed operation; only the running operation exposes cancel.

- [ ] **Step 2: Run the deployment component tests and verify they fail.**

Run:

```bash
npm --prefix web run test -- src/__tests__/useKernelDeployments.test.js src/__tests__/TopologyWorkspace.test.js src/__tests__/AssignmentDrawer.test.js
```

Expected: FAIL because the composable and components do not exist.

- [ ] **Step 3: Implement backend-compatible extraction.**

Move `extractNodes`, `topologyGraphValue`, `topologyInputFromJSON`,
`topologyJSONFromDetail`, and `assignmentPayload` out of `Control.vue` without
changing any request shape. Import `releaseTargets` from
`@/utils/kernelPluginRelease` rather than duplicating manifest parsing. Retain
this normalization:

```js
export function assignmentPayload(source, overrides = {}) {
  return {
    service_scope: source.service_scope,
    plugin_id: source.plugin_id,
    role: source.role,
    desired_version: source.desired_version || '',
    desired_config_revision: Math.max(0, Number(source.desired_config_revision ?? 0)),
    enabled: Boolean(source.enabled),
    rollout_group: source.rollout_group || '',
    ...overrides,
  }
}
```

`TopologyWorkspace` keeps a revision JSON/message baseline, disables
plan/apply/rollback while dirty, puts validation/preview results beside their
actions, and renders an explicit rollback confirmation. `AssignmentDrawer`
chooses the first installed agent plugin for a new assignment and derives its
default version/config revision/scope using current `Control.vue` rules.
`OperationTimeline` sorts newest first, displays state/error text, and exposes
global history only through an explicit expansion.

- [ ] **Step 4: Run extracted-component tests and verify they pass.**

Run:

```bash
npm --prefix web run test -- src/__tests__/useKernelDeployments.test.js src/__tests__/TopologyWorkspace.test.js src/__tests__/AssignmentDrawer.test.js src/__tests__/PluginConfigForm.test.js
```

Expected: PASS. Verify canonical topology JSON, immutable assignment identity,
and terminal-operation cancellation behavior.

- [ ] **Step 5: Commit the deployment building blocks.**

```bash
git add web/src/composables/useKernelDeployments.js web/src/components/admin/TopologyWorkspace.vue web/src/components/admin/AssignmentDrawer.vue web/src/components/admin/OperationTimeline.vue web/src/__tests__/useKernelDeployments.test.js web/src/__tests__/TopologyWorkspace.test.js web/src/__tests__/AssignmentDrawer.test.js
git commit -m "feat(admin): extract deployment workspace components"
```

## Task 6: Deliver Deployment Center, Migrate Browser Coverage, and Remove the Monolith

**Files:**
- Modify: `web/src/views/admin/Deployments.vue`
- Create: `web/src/__tests__/Deployments.test.js`
- Modify: `web/e2e/control-assignments.spec.js`
- Modify: `web/e2e/live-control-machine-telemetry.spec.js`
- Modify: `web/src/__tests__/adminRoutes.test.js`
- Delete: `web/src/views/admin/Control.vue`
- Delete: `web/src/__tests__/Control.test.js`

**Interfaces:**
- `Deployments.vue` has `data-testid="deployment-topologies"`,
  `data-testid="deployment-targets"`, `data-testid="assignment-drawer"`, and
  `data-testid="deployment-activity"`.
- Its local view mode is only `topologies` or `targets`; global activity is an
  explicit expansion, not another primary page tab.
- `loadAssignments(nodeID)` calls `getKernelNodeAssignments(nodeID)` only when
  `nodeID > 0` and clears rows otherwise.

- [ ] **Step 1: Create failing Deployment Center tests from old Control coverage.**

Port topology creation, revision load/validation, preview/plan/apply, guarded
rollback, deployment status refresh, assignment create/edit/toggle/delete,
backend error visibility, and operation cancellation into `Deployments.test.js`.
Use the stable target view/drawer contract:

```js
await wrapper.get('[data-testid="deployment-targets"]').trigger('click')
expect(kernelApi.getKernelNodeAssignments).toHaveBeenCalledWith(11)
await wrapper.get('[data-testid="new-assignment"]').trigger('click')
expect(wrapper.get('[data-testid="assignment-drawer"]').exists()).toBe(true)
```

Add an initial-load assertion that `getKernelPluginReleases` is not called until
the assignment drawer is opened.

- [ ] **Step 2: Run the route test and verify it fails.**

Run:

```bash
npm --prefix web run test -- src/__tests__/Deployments.test.js
```

Expected: FAIL because `Deployments.vue` does not exist.

- [ ] **Step 3: Implement the Deployment Center route.**

Start in `topologies` mode. Initial load calls `getKernelTopologies`,
`getKernelDeployments`, `getKernelOperations`, `getKernelScopes`, and
`getNodes({ page: 1, page_size: 200 })`. Fetch assignment rows only after a
node selection or entering Targets mode. Load plugin/release/installation data
lazily when the assignment drawer opens, because only that form needs agent
defaults.

Preserve staged publishing:

```js
await diagnoseKernelTopologyDeployment(topologyID, revisionID, options)
const preview = await previewKernelTopologyDeployment(topologyID, revisionID, options)
if (!preview?.valid) throw new Error(t('control.topology.invalid'))
const deployment = await planKernelDeployment({ topology_id: topologyID, revision_id: revisionID, ...options })
await getKernelDeploymentStatus(deployment.id)
```

After apply/rollback, refresh selected deployment status and the topology list.
After an assignment mutation, refresh only selected-node assignments and
relevant activity. Keep server error text adjacent to the action. Poll only a
selected active deployment or assignment-related operation; stop on terminal
state and route unmount.

Delete `Control.vue` and `Control.test.js` only after the Plugin Center and
Deployment Center suites cover every old test case named in this plan. Keep
`/admin/control` as the redirect from Task 1.

- [ ] **Step 4: Migrate fixture-based browser tests to the new routes.**

In `web/e2e/control-assignments.spec.js`, navigate to
`/admin/deployments`, activate `data-testid="deployment-targets"`, and replace
legacy tab/panel selectors with the stable target/drawer selectors. Retain:

```js
expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true)
```

In `live-control-machine-telemetry.spec.js`, change only the post-revocation
fallback expectation from `/admin/control` to `/admin/plugins`; retain all
signed-bundle and revocation assertions.

- [ ] **Step 5: Run migrated domain and browser tests.**

Run:

```bash
npm --prefix web run test -- src/__tests__/Plugins.test.js src/__tests__/Deployments.test.js src/__tests__/useKernelPlugins.test.js src/__tests__/useKernelDeployments.test.js src/__tests__/AdminLayout.test.js src/__tests__/adminRoutes.test.js
npm --prefix web run test:e2e -- e2e/control-assignments.spec.js e2e/plugin-webui.spec.js
```

Expected: PASS. If the live-control suite reproduces an existing base-commit
failure, record its exact command and failure separately; do not label it a new
UI regression without a changed failure signature.

- [ ] **Step 6: Commit the completed route migration.**

```bash
git add web/src/views/admin/Deployments.vue web/src/__tests__/Deployments.test.js web/e2e/control-assignments.spec.js web/e2e/live-control-machine-telemetry.spec.js web/src/__tests__/adminRoutes.test.js
git rm web/src/views/admin/Control.vue web/src/__tests__/Control.test.js
git commit -m "feat(admin): split deployments from plugin operations"
```

## Task 7: Run Full Verification and Perform Visual Acceptance

**Files:**
- Modify only files required to repair a demonstrated test, build, accessibility, or responsive failure from this task.
- Create: `docs/audit/admin-workbench-verification-2026-07-18.md` only if an unchanged baseline failure must be documented.

**Interfaces:**
- The feature worktree is clean after verification except for a narrowly scoped repair commit.
- Desktop (1440x1024) and mobile (390x844) evidence shows nonblank Plugin and Deployment pages, readable text, no accidental overlap, no horizontal overflow, and usable drawer/dialog controls.

- [ ] **Step 1: Run the complete frontend unit suite and production build.**

Run:

```bash
npm --prefix web run test
npm --prefix web run build
```

Expected: PASS. The build emits the new lazy route chunks and has no unresolved Lucide imports. A passing focused subset is not proof of the broader administrator change.

- [ ] **Step 2: Run fixture-based Playwright coverage and capture screenshots.**

Run:

```bash
npm --prefix web run test:e2e -- e2e/control-assignments.spec.js e2e/plugin-webui.spec.js
```

Start the development server from `web` on an unused local port. Use Playwright with the fixture API routing used by the browser tests to inspect `/admin/plugins` and `/admin/deployments` at 1440x1024 and 390x844. Save four screenshots under Playwright output and inspect them. Assert every viewport:

```js
expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true)
await expect(page.locator('main')).toBeVisible()
await expect(page.locator('main')).not.toBeEmpty()
```

Expected: plugin rows are grouped per plugin, drawers are fully visible, topology/target controls do not collide, and compact navigation works at both widths.

- [ ] **Step 3: Verify backward compatibility and extension fail-closed behavior.**

Run:

```bash
npm --prefix web run test -- src/__tests__/controlLegacy.test.js src/__tests__/adminRoutes.test.js src/__tests__/AdminLayout.test.js
```

Expected: all legacy tab mappings resolve correctly, denied extension routes redirect to `/admin/plugins`, and extension menus remain absent without their required permission.

- [ ] **Step 4: Run source and worktree hygiene checks.**

Run:

```bash
git diff --check
git status --short --branch
```

Then inspect `web/src/style.css` and `web/src/layouts/AdminLayout.vue` for `body-gradient`, `linear-gradient`, and `radial-gradient` with the repository search tool.

Expected: no whitespace errors, no unintended files, and no gradient remains in the redesigned global/admin shell. A gradient in an unrelated user-facing component must be listed in the verification note rather than silently changed.

- [ ] **Step 5: Commit a baseline-failure note only when it is needed.**

If an unchanged baseline failure is reproduced, create the named audit file and
commit only that evidence:

```bash
git add docs/audit/admin-workbench-verification-2026-07-18.md
git commit -m "docs: record admin workbench verification baseline"
```

If a source verification failure occurs, return to its owning task, add the
concrete affected files to that task's commit, and repeat this task from Step 1.
If no repair or baseline note is needed, do not create an empty commit. The
final handoff must report exact unit, build, browser, viewport, and compatibility
commands with their observed results.
