# Admin Workbench Redesign

**Date:** 2026-07-18  
**Status:** Design approved; awaiting specification review  
**Scope:** `anix-control` administrator frontend

## Context

The existing administrator shell exposes many independent sections in a long
sidebar. The control kernel is a single large `Control.vue` page with five
tabs: plugins, assignments, scopes, topologies, and operations. It loads every
resource together and repeats a plugin once for each installation target. This
makes the common work of checking a plugin, updating it, or publishing a
topology harder to scan than it needs to be.

The redesign separates plugin lifecycle work from deployment work while making
the entire administrator shell more compact. It preserves every existing
administrator capability, kernel API, permission check, and extension contract.

## Goals

- Make the administrator UI feel like a focused operations workbench rather
  than a collection of unrelated panels.
- Make plugin health, version state, configuration, and lifecycle actions
  quickly scannable and available without duplicate rows.
- Put assignment, topology, release, rollout, rollback, and activity work in
  one deployment-oriented flow.
- Preserve deep links, translations, light/dark themes, keyboard access,
  responsive behavior, extension menus, and server-side behavior.
- Use a restrained, natural, Apple-inspired visual direction: content first,
  calm hierarchy, generous but purposeful space, simple surfaces, and
  immediate feedback. Do not copy Apple branding, assets, layouts, or text.

## Non-Goals

- No change to kernel API endpoints, data models, authorization semantics, or
  agent behavior.
- No user-portal redesign.
- No removal of existing administrator functions or routes; infrequent routes
  may be grouped behind an explicit `More` disclosure.
- No marketing-style hero layout, decorative gradients, or visual-only cards.

## Information Architecture

### Administrator shell

The desktop shell has a compact, fixed navigation rail that can collapse to an
icon rail. The mobile shell remains a full overlay drawer. The header contains
the page title, a concise breadcrumb, and global tools only. Page-specific
actions appear beside the page title, so the same action is never repeated in
the sidebar and page body.

Navigation has five primary groups:

1. **Overview:** dashboard, monitor, and traffic.
2. **Business:** users, orders, tickets, subscriptions, plans, and related
   commercial/content routes. Less-frequent entries use a `More` disclosure.
3. **Network:** nodes and the existing Forward Suite. Its established core and
   advanced grouping is retained, but rendered in the compact shell style.
4. **Control Center:** Plugins, Deployments, then authorized dynamic extension
   entries. Extension entries remain permission-gated and retain their current
   runtime registration behavior.
5. **System:** MFA, access groups, system configuration, and administrative
   communications/settings. Less-frequent entries use `More` rather than being
   removed.

Every current route remains reachable. Each route is placed in its closest
primary group; `More` is an explicit disclosure, not a hidden or permissionless
shortcut.

### Routes and compatibility

New primary routes are:

- `/admin/plugins` - plugin catalog and installation lifecycle.
- `/admin/deployments` - topology, assignment, rollout, and activity work.

`/admin/control` remains a compatibility route. Its legacy tab query maps as
follows while preserving unrelated query parameters and hashes:

| Legacy tab | Destination |
| --- | --- |
| `plugins` or absent | `/admin/plugins` |
| `assignments`, `scopes`, `topologies`, `operations` | `/admin/deployments` |

Unauthorized-extension fallbacks point to `/admin/plugins`. Existing extension
route discovery, `menuRegistry` parents, and permission checks remain intact.

## Plugin Center

The Plugin Center is a single compact, searchable list with one row per
plugin, not one row per plugin target. Its fixed data hierarchy is:

1. Plugin name, identifier, and short description.
2. Installed target summary.
3. Current and available release version.
4. Health state, including a concise visible error summary when attention is
   required.
5. Contextual actions.

The page header offers search, health/target filtering, refresh, and one
primary action: import release. A small state summary reports healthy,
attention-needed, and catalog-only counts without turning the page into a
dashboard.

Selecting a plugin opens a right-side detail drawer. It lists each installation
target with its desired and observed version, enabled state, and latest error.
The drawer is the only place for target-scoped actions: install, configure,
enable/disable, upgrade, and rollback. Installation and release-import dialogs
remain explicit when a version must be selected.

The existing schema-driven `PluginConfigForm` remains the configuration editor.
It continues to support structured fields, JSON fallback, validation, defaults,
and `expected_revision` conflict protection. A conflict or API error stays next
to the affected action and preserves draft configuration so the operator can
resolve it without re-entering data.

## Deployment Center

The Deployment Center opens in a topology workbench:

- A narrow topology list identifies the selected scope, active revision, and
  attention state.
- The main workspace shows the selected revision, validation/diagnostic result,
  preview, planned rollout, apply action, and rollback action.
- Activity is a concise timeline scoped to the selected topology or target.
  An explicit `All activity` expansion exposes the global operations list only
  for troubleshooting.

The same center provides a `Targets` view for node assignments. Operators can
filter by node, scope, or plugin and create, edit, enable/disable, or delete an
assignment in a side drawer. Assignment identity fields remain immutable during
edit, matching current backend behavior. Scopes are shown and selected in
assignment/topology context; there is no separate scopes page because it has no
independent operator workflow.

Publishing is intentionally staged: validate/diagnose, preview, plan, apply,
then observe. Rollback, destructive deletion, and applying a rollout use a
clear confirmation that names the affected topology or target. Polling is
limited to active, relevant operations and stops when the page unmounts or the
operation reaches a terminal state.

## Component and Data Boundaries

The monolithic control page is replaced with small route-level units:

- `AdminLayout` and a dedicated compact navigation component for shell state.
- `Plugins` route, plugin list, plugin detail drawer, installation dialog, and
  release-import dialog.
- `Deployments` route, topology workspace, assignment panel/drawer, and
  activity timeline.
- Focused composables for plugin catalog state, deployment state, and shared
  operation feedback.

The Plugin route loads plugins, releases, installations, and extension refresh
state. The Deployment route loads topologies, deployments, operations, nodes,
and scopes, then loads assignments only for the selected target. Refreshes are
scoped to the affected resource rather than reloading the complete kernel
surface after every action.

No backend API contract changes are required. Existing functions in
`web/src/api/kernel.js` remain the transport boundary.

## Visual and Interaction System

- Use a neutral, solid work surface with 1px dividers, subtle shadows, and
  corners no larger than 8px. Remove global and sidebar gradients in the admin
  shell.
- Use one clear primary action per page. Secondary actions are quiet; familiar
  icon-only controls include visible tooltips and accessible labels.
- Use a supported icon library rather than text-initial icons. The selected
  icon must be stable in width so labels and rows do not shift.
- Keep typography compact in dense tools. Titles describe the current work;
  helper copy appears only when it changes an operator decision.
- Maintain light and dark semantic color tokens. Health is conveyed with text
  and icons in addition to color.
- On smaller screens, tables become stacked scan rows and detail drawers become
  full-screen panels. No critical interaction requires horizontal scrolling.
- Preserve semantic headings, keyboard navigation, focus restoration after
  drawers/dialogs close, live status announcements, and clear loading/empty/
  error states.

## Testing and Acceptance

The implementation must add or update focused tests for:

- Navigation grouping, collapse behavior, permissions, extension entries, and
  mobile drawer behavior.
- Legacy `/admin/control` links and tab queries redirecting to the correct new
  route.
- Plugin search/filter, target detail, installation lifecycle, configuration
  save/conflict handling, and error visibility.
- Assignment create/edit/toggle/delete behavior, topology validation/preview/
  apply/rollback behavior, and operation polling lifecycle.
- Chinese and English labels, keyboard focus behavior, loading/empty/error
  states, and responsive layout at desktop and mobile viewports.

Acceptance requires the frontend unit suite and production build to pass. A
local development server must be checked interactively and with browser
screenshots at desktop and mobile widths. Existing unrelated end-to-end
failures, if reproduced unchanged on the base commit, must be documented rather
than hidden by this change.

## Delivery Sequence

1. Create the new route and navigation shell while retaining legacy redirects.
2. Extract plugin lifecycle into the Plugin Center and verify API behavior.
3. Extract topology, assignments, and activity into the Deployment Center.
4. Apply the compact visual system across the administrator shell.
5. Run unit, build, route, responsive browser, and compatibility verification.
