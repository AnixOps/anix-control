# Frontend Design System (AnixOps Design)

The web UI (`web/`) is built on **AnixOps Design**, the shared brand and
design-token repository
[`AnixOps/AnixOps-design`](https://github.com/AnixOps/AnixOps-design). This
page covers how Control vendors it, how the theme works, how the pre-redesign
styles are bridged to it, and the lint rules that guard it. It is phase U1 of
the UI redesign; later phases replace pages with a component library.

## Vendored files

`web/src/design/` is a verbatim copy of the pinned tag (currently **v1.0.2**,
in `web/src/design/VERSION`). Never edit these files here: change the design
repository, tag a release, and sync.

| Vendored path (`web/src/design/`) | Source in AnixOps-design | Use |
|---|---|---|
| `tokens.css` | `tokens/css/tokens.css` | All colour, type, spacing, radius, shadow, motion and z-index variables, light and dark |
| `brand/mark-glyph.svg` | `brand/assets/mark-glyph.svg` | Inline mark (`currentColor`) in the brand lockup |
| `brand/wordmark.svg`, `brand/wordmark-on-dark.svg` | `brand/assets/` | Outlined lockups for places where HTML text cannot be used |
| `brand/favicon.svg`, `brand/favicon/*` | `brand/assets/favicon.svg`, `brand/assets/favicon/` | Browser tab, iOS home screen, web app manifest icons |
| `fonts/inter/` | `fonts/inter/` | Inter variable, Latin subset, `font-display: swap`, with `OFL.txt` |
| `LICENSE`, `LICENSE-BRAND.md` | repository root | Licences (below) |

The sync also writes `VERSION`, `NOTICE.md` and `manifest.json` (path, source
path, size and SHA-256 of every file).

**Licences.** `tokens.css` is MIT. Inter is under the SIL Open Font License
1.1. The AnixOps name, mark, glyph, wordmarks, favicon and app icons are
**not** MIT: all rights reserved, for official AnixOps products only
(`LICENSE-BRAND.md`). A fork or redistribution that is not an official
AnixOps release must replace `web/src/design/brand/` with its own assets.

### Sync and check

```bash
cd web
npm run design:sync -- --tag v1.0.3                       # from GitHub raw files at the tag
npm run design:sync -- --tag v1.0.3 --source ../AnixOps-design   # from a local clone (reads the tag with git show)
npm run design:check                                      # offline: files == manifest.json
node scripts/sync-design.mjs --check --upstream           # also compare manifest.json with the tag on GitHub
```

`design:check` runs in the Frontend Build CI job. It fails when a vendored
file differs from the manifest, a file is missing or unexpected, `VERSION`
or `NOTICE.md` disagree with the manifest, or the file list in
`web/scripts/sync-design.mjs` and the manifest differ. The sync refuses a
source whose `tokens.css` header names a different version than the tag.

After a sync: run `npm test`, `npm run build` and look at the UI in both
themes; record the new version in `CHANGELOG.md`.

### Favicon, app icons and manifest

`web/public/` is the build output (Vite `publicDir` is off) and the Go
frontend server only serves `/assets/*` and `/favicon.ico`. The Vite plugin
`web/scripts/vite-brand-icons.mjs` therefore emits the icons and
`manifest.webmanifest` under `assets/` with content-hashed names
(immutable caching), `favicon.ico` at the root, and adds the `<link>` and
`theme-color` tags to `index.html`. The manifest name is "AnixOps Control";
its colours and the `theme-color` values are read from `tokens.css`.

## Styles

Load order (`web/src/main.js`): `design/fonts/inter/inter.css`,
`design/tokens.css`, `styles/base.css`, `styles/pages.css`, then
`style.css`.

- **`styles/base.css`**: reset; `body` on `--bg` / `--label-1` with the
  token font stack (system, Inter, then Chinese system fonts) and the Body
  step (line height 1.6 for Chinese); the 2 px `--accent` focus ring with a
  2 px offset; `.tabular-nums`; `.type-*` utilities for the seven type
  steps; scrollbars per theme; `prefers-reduced-motion` (no movement, short
  fades) and `prefers-contrast: more` (stronger separators). It also defines
  three values the tokens do not have yet: `--scrim` (dialog and drawer
  backdrop), `--focus-ring` and `--z-popover` (see "Layer order").
- **`style.css`**: what is left of the pre-redesign global layer (U9): the
  baseline for bare `<button>`, `<input>`, `<textarea>` and `<select>`, the
  skip link, and the classes that signed plugin WebUI bundles
  (`packages/<name>/webui`, `anixops.webui/v1`) render: `.btn`,
  `.btn-secondary`, `.table-container`, `.data-table`, `.empty-state`. The
  plugin configuration forms also use `.btn-primary`, `.btn-sm`,
  `.btn-danger`, `.form-group` and `.required`. Plugins ship separately, so
  these classes stay while the WebUI contract does; nothing new goes there.
  The five WebUI classes are a stable contract ("WebUI CSS Classes" in
  `docs/architecture/plugin-kernel-contract.md`), guarded by
  `src/__tests__/pluginWebuiClasses.test.js`.
  On touch screens `.btn` and the bare fields keep a 44 px minimum height;
  bare buttons size themselves (the old rule that set every button to
  40 / 44 px on phones is gone).
- **New code uses the tokens directly**: `var(--label-1)`, `var(--accent)`,
  `var(--accent-fill)` with `var(--on-accent)` for filled controls,
  `var(--radius-sm)`, `var(--type-body-size)`, `var(--space-4)`, and so on.
  `--accent` is for text, links and icons; filled buttons use
  `--accent-fill` (in dark mode white on `--accent` fails contrast).

### Theme

`tokens.css` follows `prefers-color-scheme` while `<html>` has no
`data-theme`, and `data-theme="light"` or `"dark"` overrides it.
`web/src/composables/useTheme.js` resolves the theme (stored choice, else the
system preference), always writes it to `data-theme` (older page styles use
`[data-theme='dark']` selectors), keeps following the system until the user
picks a theme with the toggle, and points the `theme-color` meta tags at the
active background.

### Layer order

Layers stack by the `--z-*` tokens, lowest first: `--z-sticky` (10, sticky
headers and bars), `--z-dropdown` (100), `--z-drawer` (200, `UiSheet` and the
phone navigation drawer), `--z-modal` (300, `UiDialog`, `UiConfirmDialog`,
the command palette), `--z-popover`, `--z-toast` (400) and `--z-tooltip`
(500, the skip link, which has to be reachable over everything; there is no
tooltip component, hints are `title` attributes).

AnixOps Design 1.0.2 has no layer for what opens from a trigger and can be
opened from inside a dialog, a sheet or the navigation drawer: a row "…"
menu in a sheet, the account menu in the phone drawer, a Select or Combobox
list in a dialog. All of these are portalled to `<body>` (`useMenuLayer`), so
only z-index decides what covers them, and `--z-dropdown` is below the
drawers and the dialogs. `styles/base.css` therefore defines
`--z-popover: calc(var(--z-modal) + 1)`: above every dialog and drawer, under
the toasts, which stay on top of a menu. `.ui-menu`, `.shell-menu` and
`.ui-listbox` use it with `position: relative`: Reka copies the content's
z-index to its fixed popper wrapper, and a static element can compute to
`auto`. A new floating layer uses `var(--z-popover)` the same way. The
vendored `tokens.css` is not edited for this. The token belongs in the design
repository (next to `--z-modal`); until a tag carries it, the line in
`base.css` is its definition, and it goes when the sync brings it.

No component writes a z-index number: stylelint accepts `var(--z-*)`, a token
plus or minus 1 or 2 (`calc(var(--z-drawer) + 1)`), and `0`, `1`, `-1` and
`auto` for ordering siblings inside one component. `e2e/layers.spec.js` opens
a row menu in a sheet, a Select list in a dialog and the account menu in the
phone drawer and asks the browser (`elementFromPoint`) whether each is what
the pointer reaches at its centre, and `src/__tests__/layerOrder.test.js`
pins the scale.

### Legacy variable map

The pre-redesign variable names were aliases of tokens in `style.css`
until every page used the tokens; U9 removed them, and stylelint rejects
them (`declaration-property-value-disallowed-list`). Code copied from an
old page or a plugin translates like this:

| Legacy | Token |
|---|---|
| `--bg-color`, `--bg-accent` | `--bg`, `--bg-grouped` |
| `--surface-color`, `--surface-muted`, `--surface-hover` | `--bg-elevated`, `--bg-grouped`, `--fill-1` |
| `--border-color`, `--border-strong` | `--separator`, `--separator-strong` |
| `--text-color`, `--text-secondary`, `--text-tertiary` | `--label-1`, `--label-2`, `--label-3` |
| `--primary-color`, `--primary-hover`, `--primary-soft` | `--accent`, `--accent-hover`, `--accent-soft` |
| `--success-color`, `--warning-color`, `--error-color` | `--success`, `--warning`, `--danger` |
| `--admin-sidebar-surface` | `--material-sidebar` (frosted, with `backdrop-filter`) |
| `--admin-sidebar-accent`, `--admin-sidebar-hover`, `--admin-sidebar-divider` | `--fill-2`, `--fill-1`, `--separator` |
| `--admin-sidebar-text`, `-text-strong`, `-muted` | `--label-1`, `--label-1`, `--label-2` |
| `--shadow-sm`, `--shadow-md`, `--shadow-lg` | `--shadow-1`, `--shadow-2`, `--shadow-3` |
| `--transition` | `all var(--dur-toggle) var(--ease-standard)` |
| `--radius-sm`, `--radius-md`, `--radius-lg` | now the token values 10, 14, 20 px (were 6, 8, 8 px) |
| Arco-era `--color-text-1/2/3` | `--label-1/2/3` |
| Arco-era `--color-bg-1`, `--color-bg-2`, `--color-border` | `--bg-elevated`, `--bg-elevated`, `--separator` |
| `--background-color`, `--bg-secondary` | `--bg-grouped`, `--fill-1` |

The global classes `.card`, `.section-panel`, `.tabs`/`.tab`,
`.status-badge`, `.form-row`, `.btn-ghost`, `.btn-lg`, the layout and
spacing utilities (`.grid-*`, `.flex`, `.mt-4`, ...) and `.container` went
in U9, unused; ESLint (`vue/no-restricted-class`) rejects the distinctive
ones. `.modal*` and `.close-btn` went in U4, when every overlay moved to
`UiDialog`/`UiSheet`.

## Brand in the UI

`web/src/components/common/BrandLockup.vue` renders the lockup from
`brand.md` §2.4: the inline glyph (from the vendored `mark-glyph.svg`), then
"AnixOps" in 600 and the product word in 400. `tile` puts the glyph on the
brand-gradient tile (login page, admin sidebar); `inverse` sets the text in
white for the slate backdrop; `size` is `sm`, `md` or `lg`; `markOnly`
drops the name (the sidebar's icon rail). Page titles use "AnixOps
Control".

## Charts

Pages draw every chart with `UiChart` (`web/src/ui/UiChart.vue`) and every
headline number with `UiMetricCard`; neither imports ECharts in the page.

- **Engine.** `src/ui/internal/echarts.js` is the tree-shaken ECharts
  build (`echarts/core` with line and bar series, grid, tooltip, legend,
  canvas renderer). `UiChart` imports it on first use, so it lands in the
  lazy `echarts` chunk (about 180 KB gzip) and never in the entry chunk or a
  page without a chart. A new series type is imported and listed in `use()`
  there.
- **Theme.** `web/src/composables/useChartTheme.js` resolves the chart
  palette (`--chart-1` … `--chart-8`) and the neutral roles from the
  document, registers an ECharts theme per mode (`anixops-light`,
  `anixops-dark`), and `watchDocumentTheme()` re-themes live charts with
  `chart.setTheme()` when `<html data-theme>` changes. Options passed to
  `UiChart` carry no colours. The G6 topology (`components/admin/
  TopologyGraph.vue`) takes node, label and edge colours from the same
  tokens (`buildGraphColors`) and redraws on the same signal.
- **Size.** The chart resizes with its box (`ResizeObserver`), not only the
  window; `height` sets the plot height.
- **States.** `loading` shows the chart skeleton (`UiSkeleton
  variant="chart"`) after 300 ms on the first load and dims the plot on a
  reload; `empty` shows `UiEmptyState` (`emptyTitle`, `emptyDescription`);
  `error` shows `UiErrorState` with 重试 (`@retry`). A failure to load
  ECharts shows the error state too.
- **Accessibility.** The plot is `role="img"` named by `label` and
  `summary` (one sentence: range, total, peak). `table`
  (`{ columns: [{ key, label, numeric, format }], rows }`) adds the data as
  a table, visually hidden but read by screen readers, and 以表格查看 shows
  it (plan §11).
- **Metric cards.** `UiMetricCard`: `label`, `value`, `icon`, `trend` with
  `trendDirection` and `trendTone` (an arrow and a word, never colour
  alone), `detail`, an optional `sparkline` (number array, an inline SVG in
  the accent token, decorative) and `loading`. Wrap it in a link when it
  opens a page.

## Lint rules

Every rule is an error, and the Frontend Build job runs both linters
(`npm run lint && npm run lint:styles`). The style rules warned while the
pages migrated and became errors in U9, when the count reached 0.

| Command | Rule | At U1 | At U4 | At U9 |
|---|---|---|---|---|
| `npm run lint:styles` (stylelint, `web/stylelint.config.mjs`) | colour literals (hex, named colours, `rgb()`/`hsl()`…) outside `src/design/` | 512 warnings | 646 warnings for the three rules together (774 before U4) | error, 0 |
| | `font-size` off the scale (12/13/15/19/24/32/48 px, phone 16/28/34 px, or a variable) | 131 warnings | | error, 0 |
| | `border-radius` off the scale (6/10/14/20/980 px, 0, 50 %, or a variable) | 132 warnings | | error, 0 |
| | `z-index` that is not a `--z-*` token (alone or ±1/±2), `0`, `1`, `-1` or `auto` | — | — | error, 0 |
| | removed legacy variables (`var(--text-color)`, …, see "Legacy variable map") | — | — | error, 0 |
| `npm run lint` (ESLint, `web/eslint.config.js`) | `no-alert`: `alert`, `confirm`, `prompt` | 117 warnings | error, 0 | error, 0 |
| | `vue/no-restricted-class`: `.modal*` (U4) and the removed global classes (U9) | — | error, 0 | error, 0 |
| | `vue/no-restricted-html-elements`: `<table>` in the pages listed in `web/scripts/data-table-pages.mjs` (U6) | — | error, 0 | error, 0 |

`web/src/__tests__/nativeDialogs.test.js` (part of `npm test`) also fails
on `window.alert/confirm/prompt` or `.modal-overlay` in the app sources, and
the test setup makes a native dialog throw. Likewise
`web/src/__tests__/dataTableGuard.test.js` fails when a page listed in
`web/scripts/data-table-pages.mjs` (the migrated list pages) has a bare
`<table>` or the legacy `.data-table` class again.

## Components

`web/src/ui/` is the component library (phase U2). It is built on
[Reka UI](https://reka-ui.com) 2.10.5 (headless primitives with the
keyboard, focus and ARIA behaviour; pinned) and our own scoped CSS that uses
only tokens (stylelint reports zero warnings for `src/ui/`). The app shells
(U3) and the account page use it; since U4 every page's feedback and
dialogs do too (see "Feedback and dialogs in pages"); U5 onwards migrates
the page bodies. Names carry a `Ui` prefix so they never
shadow HTML elements: `import { UiButton, useToast } from '@/ui'`.

**Stories.** Every component has a Histoire story (`src/ui/stories/`):
`npm run story:dev`, `npm run story:build` (output in `web/.histoire/`).
The preview loads the vendored tokens and `base.css`; the toolbar's
dark-mode switch maps to `<html data-theme="dark">`, and `?lang=en` on the
preview URL switches the built-in strings to English.

> Histoire is pinned at 1.0.0-beta.1, which declares Vite ^7, so
> `package.json` has `overrides: { "vite": "$vite" }` to run it on the
> project's Vite 8; drop the override once a stable Histoire supports Vite 8.


**Mount once.** `<UiHost />` (toast region and confirmation host) is in
`App.vue` (an async component, so the login page does not load it); any
page can call `useToast()` and `useConfirm()`.

**Strings.** Built-in text (close, cancel, copied, undo, loading, status
words, units) comes from `ui.*` in `src/locales/modules/{zh-CN,en}/ui.js`,
written separately for each language. Page text is passed in as props.

### Which component

| Need | Use | Notes |
|---|---|---|
| An action | `UiButton` | One `primary` per view, rightmost; `secondary` otherwise; `tertiary` for low-emphasis text actions; `danger` only to confirm a destructive action, `danger-soft` for the button that starts one ("删除节点…"); `ghost` in toolbars. Sizes `sm` 28 (tables), `md` 36 (default), `lg` 44. Labels are verbs. |
| An icon-only action | `UiIconButton` | `label` is required (aria-label and tooltip); toggles pass `pressed`. |
| Text, numbers, passwords | `UiTextField`, `UiNumberField`, `UiPasswordField`, `UiTextarea` | Top label, help, error; units as `suffix`/`unit`. Height 44 (`lg`) by default in forms and dialogs; `size="md"` (36) in toolbars, tables and sheets; `sm` 28 only inside dense table cells. Same sizes for `UiSelect` and `UiCombobox`. |
| A custom control in a form | `UiField` | Gives the control its id, label, help and error wiring. |
| One of a short list | `UiSelect` | Up to ~10 options. |
| One of a long list, or search | `UiCombobox` | Filters as you type. |
| A setting that applies at once | `UiSwitch` | On/off; label left, switch right. |
| A choice applied on submit | `UiCheckbox` | `'indeterminate'` for "select all". |
| One of a few visible options | `UiRadioGroup` | When the options need descriptions. |
| A value that changes the view in place | `UiSegmentedControl` | 2–5 values (time range, list or grouped view). |
| Switching content panels | `UiTabs` | `underline` for detail sections; `variant="segmented"` for the pill look at the top of a page (转发规则 / 隧道 / 限速). A row of tabs wider than the screen scrolls sideways, fades out on the clipped edge and keeps the active tab in view. |
| A focused task | `UiDialog` | sm 420 / md 560 / lg 760; full screen below 834 px for md and lg. |
| Confirming an irreversible action | `useConfirm()` / `UiConfirmDialog` | `requireText` for deleting nodes and users. |
| Details or an editor next to a list | `UiSheet` | Right drawer; bottom sheet below 834 px. |
| The result of an action | `useToast()` | See the rules below. |
| A status | `UiBadge`, `UiStatusDot` | `status` online, offline, disabled, pending, error; always a word. |
| Nothing to show, or a failed load | `UiEmptyState` | What will appear and the first action; for errors, retry and "复制错误详情". |
| Loading | `UiSkeleton` + `useDelayedLoading()` | text, card, table-row. |
| A link, token or command to copy | `UiCopyField` | `secret` masks it. |
| Generated text, output, commands | `UiCodeBlock` | Monospace, scrolls inside itself (`maxHeight`; `wrap` wraps instead), label and 复制 (`copyLabel` for a specific verb, `copyDisabled` while the text is not usable yet); focusable for keyboard scrolling. |
| Page title | `UiPageHeader` | The only H1, one sentence, actions with the primary last. |
| Surfaces and settings rows | `UiCard`, `UiSection`, `UiGroupedList` + `UiGroupedListRow` | System Settings style rows: label left; value, control or chevron right. |
| Numbers, bytes, rates, money, dates | `useFormat()` | One implementation on Intl and the current locale. |
| A list of records | `UiDataTable` | Column definitions; sorting, pagination, selection + bulk bar, row "…" menu, states, phone cards. See "List pages". |
| Pages of a long list | `UiPagination` | Inside `UiDataTable` when `pageSize` is set; on its own for card grids. |
| Searching a list | `UiSearchField` | A search landmark; `/` focuses it; Esc clears. |
| Filtering a list | `UiFilterChips` | Toggle chips (已封禁 / 已到期); one at a time, or `multiple`. |
| More actions on a row or toolbar | `UiMenu` | "…" trigger with `label`; danger items last after a separator. |
| A failed load | `UiErrorState` | What failed, the message, 重试 and 复制错误详情 (status, request id, page, time). `LoadError` of the user pages wraps it. |
| Used of a total | `UiUsageBar` | The text carries the value; the bar is decorative, amber at 75 %, red at 90 %. |
| A chart | `UiChart` | ECharts with the token theme, resize, states and a data table; see "Charts". |
| A headline number | `UiMetricCard` | Label, value, trend word, detail, optional sparkline. |

### Interaction rules (redesign plan §9)

- **Feedback.** Success: a toast that disappears after 3 s. An action that
  can be undone (delete a forward rule, ban a user): a toast with "撤销" for
  5 s instead of a confirmation. Errors: inline in the form first; otherwise
  an error toast that stays until dismissed; system-wide problems use a
  banner (later phase). At most three toasts; timers pause on hover, on
  focus and while the page is hidden.
- **Confirmation.** Only for irreversible or wide-reaching actions. Name the
  object and the consequence: 「删除节点 hk-01？」 + 「此操作无法撤销。」; the
  button says what happens (「删除节点」, never 「确定」). Deleting a node or a
  user requires typing its name. With `onConfirm` the dialog shows the
  button as busy, closes on success and shows the error inline on failure.
- **Loading.** Nothing for the first 300 ms, then a skeleton
  (`useDelayedLoading`). A submitting button shows `loading`: it keeps its
  width and name, sets `aria-busy` and ignores further clicks.
- **Empty and error states** say what will appear here and offer the first
  action; errors add retry and "复制错误详情" (with the request id).
- **Forms.** Top labels; validate on blur and on submit; mark required
  fields; units as a suffix; help for risky fields (ports, keys); secrets
  masked with reveal and copy.
- **Keyboard.** Everything is reachable with Tab and shows the 2 px accent
  ring with a 2 px offset; Esc closes overlays and returns focus; arrows move
  within selects, radio groups, segmented controls and tabs; F8 jumps to the
  toasts.
- **Copy.** Short, active, about the result; Chinese and English written
  separately; a space between a number and its unit (`128.4 GB`, `3 分钟前`).
- **Lists.** Search, filter chips, sort and page live in the URL query, so a
  filtered list can be shared and survives a reload or 返回
  (`composables/useListQuery.js`: `read`, `readPage`, `write` with
  `router.replace`; defaults stay out of the URL). Nodes, users, orders,
  tickets, plugins, agents, coupons and invite codes do this.

### Feedback and dialogs in pages (U4)

How the rules above were applied when U4 replaced every `alert`, `confirm`
and `prompt` (117 calls plus the `notify` wrappers on Users, Agent and
System) and every hand-rolled overlay (about 50 in 25 views and
components):

- **Results** are `toast.success`. Page-local banners and toasts that did
  the same job (Forward, Tunnel, Limit, NodeX forward nodes, Subscriptions)
  now call `useToast()`; a message that repeats (auto-refresh) replaces its
  toast instead of stacking.
- **Errors** of an open form stay in it: a field error, or a
  `<p class="error" role="alert">` above the footer; the dialog stays open.
  Errors with no form (a load or a row action) are persistent
  `toast.error`.
- **Undo instead of a confirmation** only where the API has a real inverse:
  ban/unban a user, enable/disable a payment gateway, delete the Telegram
  webhook (sets it again), remove a group from a plan, remove a member or
  plan from an access group. Nothing else offers 撤销.
- **ConfirmDialog** (`useConfirm`, danger tone, title naming the object,
  verb button) for irreversible actions, with `onConfirm` so a failure shows
  inside it. **Typed name** for deleting a node, a NodeX forward node and an
  Ansible machine (the user list has no delete). Forward keeps its second
  confirmation before a force delete (flux clone flow); a few non-danger
  confirmations (mark an order paid, close a ticket, send a Telegram
  broadcast) use the default tone.
- **`prompt()`** became a small dialog: the node template picker (a select
  with validation) and the "copy manually" fallback (a `UiCopyField`).
- **Containers**: forms are `UiDialog` (sm/md/lg by content; the inner
  markup was kept and is restyled in U5–U7); details and side tasks are
  `UiSheet`: user traffic, order and payment-record details, user tickets
  (new ticket and conversation), the admin ticket reply, a node's protocol
  list, plugin details and deployment assignments. Busy dialogs set
  `:dismissible="false"`.
- **Tests**: `src/__tests__/helpers/feedback.js` gives `answerConfirms()`
  (answers `useConfirm`, running `onConfirm`), `toastMessages()` and
  `inBody()` (dialogs render in `document.body`: mount with
  `attachTo: document.body`). Flows that matter (typed name, Esc, focus
  return, inline errors) are tested with `<UiHost />` and Testing Library,
  e.g. `adminUsers.dialogs.test.js`.

### Accessibility built in

| Component | Behaviour |
|---|---|
| Fields | A 1 px `--label-3` border (3:1 or more in both themes, WCAG 1.4.11); `<label for>`; help, units and error in `aria-describedby` (error first); `aria-invalid`; errors in a polite live region; native `required` with a visual-only asterisk. |
| Select / Combobox | `role="combobox"` + listbox; Space/Enter/↓ open, arrows move, typing jumps or filters, Enter selects, Esc closes and returns focus. The open list is a region named "<label> options"; a Select makes the page behind it inert while open. |
| NumberField | `role="spinbutton"` with min/max; ↑/↓ step, PageUp/PageDown ×10, Home/End; the wheel does not change it. |
| Switch / Checkbox / Radio | `role="switch"` / `checkbox` (`mixed`) / `radiogroup` named by its label; boundaries at 3:1 or more. |
| SegmentedControl | Named `group` of `aria-pressed` buttons with roving focus; one selection always. |
| Tabs | `tablist` / `tab` / `tabpanel` with `aria-controls`; arrows move and activate. |
| Dialog / Sheet | `role="dialog"`, `aria-modal`, labelled by the title and described by the description; focus trapped and returned; Esc; page scroll locked; the close button (Lucide x) is named "关闭" and comes last in tab order. A sheet focuses its first text field, else the close button. A body that scrolls with nothing focusable inside takes `tabindex="0"` so the keyboard can scroll it. Below 834 px wide content scrolls inside the dialog, never past the screen. |
| ConfirmDialog | `role="alertdialog"`; focus on the typed field, else Cancel for danger, else the confirm button; the scrim does not close it. |
| Toast | One polite `role="status"` region announces each toast (with "按 F8 前往通知" when it has an action); the toasts are a named region. |
| Badge / StatusDot | Always a word next to the colour. |
| Skeleton | `role="status"` with a hidden "加载中…"; bones hidden; shimmer runs three times, none under reduced motion. |
| CopyField | "已复制" announced; a masked secret is a read-only password field; reveal is a pressed toggle. |

Motion uses the token durations and easings, only `opacity` and
`transform`; under `prefers-reduced-motion` overlays fade instead of
scaling or sliding. Touch: controls smaller than 44 px get a 44 px hit area
on coarse pointers; inputs are 16 px on phones.

### `useFormat()`

`useFormat()` returns `bytes`, `rate`, `number`, `percent`, `money`,
`duration`, `date`, `dateTime` and `relativeTime`; each reads the current
locale when called, so templates follow a locale switch. `createFormatter(locale)`
gives a fixed-locale set; `formatBytes()` is a plain helper.

- `bytes`: binary steps, B/KB/MB/GB/TB, two decimals by default — the same
  output as the `formatBytes` copies in the pages (a test compares them over
  2 000 values in both locales), so U4+ can swap them without visible
  change. `{ precision: 1 }` gives `128.5 GB`: use it for hero and summary
  numbers (user home, dashboard and metric cards) when those pages
  migrate; tables and details keep the default. `rate` and `percent` take
  `precision` too.
- `rate`: decimal bits per second from bytes/s (`{ input: 'bits' | 'mbps' }`).
- `duration`: the two largest units, 「3 天 4 小时」 / "3d 4h".
- `money`: from cents, `¥` in both languages.
- `date` `2026-11-30`, `dateTime` `2026-11-30 14:05` (24-hour, local time);
  `relativeTime` 「3 分钟前」 (show `dateTime` on hover).
- Missing values render as `—` (option `empty`).

## Accessibility and visual regression tests (U9)

Both run against mocked screens: `web/e2e/support/screens.js` names each
screen (fixture from `web/e2e/fixtures/` plus a scenario such as `empty` or
`error`) and `openScreen(page, name, { theme, locale, clock, scenario })` opens it
with the API answered from the fixture. `scenario` answers with another
scenario of the screen's fixture (a refusal, a state the sweeps do not need a
screen for): a spec uses it for the variants of a flow without adding a screen to
the sweeps. A fixture answer may carry `headers` (`Retry-After` of a 429).

- **Accessibility** (`web/e2e/a11y.spec.js`, part of `npm run test:e2e` in
  the Frontend Build job): `@axe-core/playwright` with the WCAG 2.2 AA and
  best-practice rules on 51 admin and user screens, light and dark, 1440 and
  390 px. Serious and critical findings fail. It also checks the skip link,
  the landmarks, one `h1` per page and focus returning to the opener when a
  dialog closes, an open row menu and the account menu (no finding of any
  impact), the plugin drawer's tabs (WAI-ARIA keys) and the topology
  workspace's field names. Add a screen to `SCREENS` when a page is added.
- **Visual regression** (`web/e2e/visual/visual.spec.js`,
  `playwright.visual.config.js`): 100 full-page screenshots of 41 screens
  (sign-in, user home, subscription, dashboard, users, nodes with their Agent
  connection, node detail, a node's traffic (and its empty state), the
  rotate-credentials confirmation and result, a subscription group's members,
  the forwarding pages, the API tokens list (also with ended and everyone's
  tokens, and empty), its create form and one-time token dialog, settings,
  monitoring (and a node's traffic in its sheet), plugin center, an empty and
  an error state), light and dark at 1440 px and a few at 390 px. Deterministic by design:
  mocked data with fixed timestamps, `Date.now()` fixed with
  `page.clock.setFixedTime`, UTC, English (text in the bundled Inter),
  reduced motion and `animations: 'disabled'` (charts and graphs turn their
  animation off under reduced motion), and anything marked
  `data-visual-mask` masked. Baselines live in
  `web/e2e/visual/__screenshots__/` and are made in the official Playwright
  image, the same one the **Frontend Visual Regression** CI job uses, so
  fonts and rasterizing match:

  ```bash
  cd web
  npm run test:visual            # compare (docker, mcr.microsoft.com/playwright:v<@playwright/test>-noble)
  npm run test:visual:update     # rewrite the baselines after an intended change
  sh scripts/visual-docker.sh -g "admin-users"   # extra Playwright arguments
  ```

  Without docker, run the **Frontend Visual Baselines** workflow
  (`workflow_dispatch`) on the branch and commit the PNGs from its artifact.
  On a failure the CI job uploads the expected, actual and diff images
  (`frontend-visual-diffs`). Bumping `@playwright/test` means updating the
  image tag in `ci.yml` and `frontend-visual-baselines.yml` and
  regenerating the baselines. The image (about 2.4 GB) is not kept on
  developer machines: `scripts/visual-docker.sh` pulls it on first use, and
  `docker rmi mcr.microsoft.com/playwright:v<version>-noble` frees the
  space again.

**Asserting a toast.** A toast's text is in the page twice by design: in the toast
list (a region named "Notifications") and, 100 ms later, in the single polite
`role="status"` live region that announces it. A bare `page.getByText(message)`
therefore matches two elements and fails with a strict-mode violation as soon as
the announcement arrives. Use `expectToast(page, message)` from
`e2e/support/toasts.js`: it asserts the toast once, scoped to the region, and the
announcement separately (`toastLocator()` and `announcementLocator()` for a
single check). Do not use `.first()` to get around it.

**The screens' fixed clock and Vue's events.** `openScreen(..., { clock: true })`
uses `page.clock.setFixedTime`, which freezes `Date.now()`. Vue skips an event
handler that was attached at or after the event's own timestamp, so with a
frozen clock every handler after the first one on an event's path is skipped.
Most controls have only one (a button's `click`), but a Reka Select trigger's
`pointerdown` sits under the dialog's own capture listener, and the keys of an
open list under the list's: the Select of a dialog neither opens with the
mouse nor moves with the arrows. A flow that works such a control (the
rotation confirmation's lifetime) runs on the real clock, and the fixture
reads `ctx.now` (the time the page's clock reads) where it dates a value.

Touch targets: on coarse pointers every control is at least 44 × 44 px,
through its own size or an `::after` hit area that keeps the look; medium
fields grow to 44 px; inputs are 16 px on phones.

Safe areas: `index.html` sets `viewport-fit=cover`, so the page draws under
the notch and the home indicator and each bar pads itself with
`env(safe-area-inset-*)`: the admin top bar (`--shell-topbar-height`
includes the top inset, so sticky offsets follow) and drawer, the user bar
and tab bar, Sheet header (side sheet) and body or footer, full-screen
dialogs, the settings save bar, the forward wizard footer and toasts; `body`
takes the left and right insets in landscape. Checked on an emulated
iPhone 13 with a 47 px notch and a 34 px home indicator
(`Emulation.setSafeAreaInsetsOverride`).

Open menus: Reka renders a drop-down menu in a portal on `<body>`, outside
every landmark, so axe reported its items as "region" content (moderate).
Moving the portal into the page's landmark is not an option (a
fixed-position menu would sit under the top bar's `backdrop-filter` or a
dialog's transform), so `UiMenu` and the account menu portal into their own
layer, made by `ui/composables/useMenuLayer.js`: a `region` on `<body>`
named after the menu (the trigger's `label`, "Menu" without one), attached
when the menu opens and removed when it closes. It exists only while the menu
is open, so there is no empty landmark to list and Reka's hide-others pass
for a modal dialog never marks it `aria-hidden`; a menu opened inside a
dialog stays readable. `e2e/a11y.spec.js` opens a row menu and the account
menu and expects no axe finding at all.

Open Select and Combobox lists: the same layer (`useMenuLayer`), named
"<field label> options" ("Options" without a name), so the list is not axe
"region" content either. The layer counts a role (`listbox`, `menu`) as
content, because a closed Reka Select keeps an empty placeholder `<div>` in
its portal. A `UiSelect` is modal in Reka (outside pointer events are off,
Tab is swallowed, and every other part of the page gets `aria-hidden`, with
no prop to turn that off), and axe reports `aria-hidden-focus` (serious) for
hidden controls that can still be focused. So while its list is open the page
behind it is `inert` (`useInertBehind`): the same siblings Reka hides (an
element with an `aria-live` attribute and its ancestors are the one
exemption, as in Reka's hide-others pass), set once focus is inside the list
and taken off synchronously when it closes, before Reka returns focus to the
trigger (an earlier `inert` would blur the trigger with no `relatedTarget`).
`UiCombobox` keeps focus in its input and hides nothing, so it only gets the
layer. The one finding left while a Select list is open is
`page-has-heading-one` (moderate): the heading is hidden on purpose and axe
cannot tell a listbox is modal, so the e2e check filters that one rule.
`listbox.css` is global (the components import it as a module): scoped, its
`.ui-listbox` rules never reached the portalled root, which then had no
background or radius and, with `z-index` on a static element computing to
`auto`, sat under the scrim of a dialog it was opened in. It is now
`position: relative` with `z-index: var(--z-popover)`, above the dialog.
`UiMenu` and the account menu had the same fault with `--z-dropdown`, which is
below a sheet (200), the phone navigation drawer (201) and a dialog (300): a
row menu in a sheet or the account menu in the drawer opened under the scrim.
They use `--z-popover` and `position: relative` too ("Layer order").

Plugin detail drawer: the install targets are `UiTabs variant="segmented"`
(Left/Right, Home/End, one Tab stop), and the topology workspace's text
fields are `UiTextField` / `UiTextarea` with real labels; the JSON help is
the field's description.

## App shell

Phase U3 (plan §4, §8.3, §9–§11). Two shells, both lazy-loaded by the
router, so the login page loads neither.

### Admin shell (`web/src/layouts/AdminLayout.vue`)

- **Sidebar** (248 px): light frosted material, dark in dark mode
  (`--material-sidebar`, opaque `--bg-grouped` without `backdrop-filter` or
  under `prefers-reduced-transparency`). The lockup on the brand tile, the
  menu groups, and the account menu at the bottom. The selected item is a
  `--fill-2` rounded fill with medium weight and an `--accent` icon; hover is
  `--fill-1`. Every link is in the Tab order; ↑/↓, Home and End also move
  between links (`components/admin/AdminNavigation.vue`).
- **Icon rail.** The top-bar toggle collapses the sidebar to a 64 px rail
  and back; the choice is stored (`admin.sidebar.collapsed`). In the rail the
  labels stay in the accessibility tree (visually hidden) and show as
  tooltips.
- **Below 834 px** the sidebar is a drawer: a modal dialog with a scrim,
  focus on its close button, Tab kept inside, Esc and the scrim close it and
  focus returns to the toggle; choosing a page closes it. The top bar then
  holds the account avatar too.
- **Top bar** (52 px, frosted): the sidebar toggle, the breadcrumb (group ›
  menu item › page; just the page below 834 px), the search button that
  opens the command palette (`⌘K` on Apple platforms, `Ctrl K` elsewhere,
  both accepted). No clock, no subtitle; no notification bell, because no
  admin notification feed exists (the bell comes with one).
- **Content** keeps to `--size-content-admin` (1280 px); routes with
  `meta.layout: 'wide'` (users, nodes, orders, deployments, forward rules,
  tunnels, limits, forward nodes, Ansible machines) use
  `--size-content-wide` (1440 px).
- **Forward suite.** On `/admin/forward*` the shell renders
  `ForwardSuiteNav` once above the page: the flux-panel sub-navigation as
  links: 快速配置向导 (an accent pill), 流量转发 / 隧道 / 限速 as a segmented
  control (U7; its own row on phones), NodeX 拓扑
  and a 更多 menu (Ansible 机器, 本地运行时, NodeX 运行时, NodeX Agents,
  可观测性) whose button names the current page when it is one of them. The
  pages and the sidebar no longer repeat it; routes and the seven legacy
  redirects are unchanged.

### User shell (`web/src/layouts/UserLayout.vue`)

- 834 px and wider: a 48 px frosted bar with the lockup, centred links
  (概览, 订阅, 帮助中心, 工单, 账户; 套餐 and 订单 in the commercial edition)
  and the account avatar; content keeps to `--size-content-user` (980 px).
- Below 834 px: the bar keeps the lockup and the avatar, and a bottom tab bar
  (概览, 订阅, 帮助, 工单, 账户; 24 px icons, the current tab in `--accent`)
  with `env(safe-area-inset-bottom)` padding takes the navigation.
  Commercial pages that do not fit move into the account menu.

### Menu config (`web/src/navigation/menu.js`)

The one place that says what the shells show. `ADMIN_MENU` lists the groups
(概览, 用户, 网络, 扩展, 系统, 商业) and their items; `buildAdminMenu()`
merges, in this order:

1. the built-in items (`id`, `to`, `icon`, `labelKey`, optional `match`
   paths that also select the item);
2. the edition (`edition: 'commercial' | 'community'`; 商业 only in the
   commercial edition, 订阅模板 under 用户 only in the community edition);
3. permissions (`permission`, checked with `userStore.hasPermission`);
4. the plugin menus from `extensions/menuRegistry.js`: `services` and
   `operations` go to 扩展, `system` to 系统, unknown parents to 扩展,
   sorted by `order` then label; each needs its permission and must not
   belong to a package the edition hides.

Items marked `optional` appear only when the router has their route (a page
whose route lands in another pull request). Empty
groups are dropped. `activeMenuItem()` selects exactly one item per path (an
exact `to`, then a `match` entry, then a sub-path). `Node` (节点,
`/admin/nodes`) and `ForwardNode` (转发节点, `/admin/forward/nodes`) are
separate items. `useAdminMenu()` / `useUserMenu()`
(`navigation/useNavigation.js`) wire it to the stores; the sidebar,
breadcrumb, palette and user navigation all render from it. Icons come from
the closed map in `navigation/icons.js`.

To add a page: add the route, then one item in `ADMIN_MENU` (or
`FORWARD_SUITE_LINKS` for a forward suite page, or `USER_MENU`) with keys in
`locales/modules/{zh-CN,en}/shell.js`.

### Command palette (`components/shell/CommandPalette.vue`)

A Reka `Dialog` around a Reka `Listbox` driven from its filter input, which
has the combobox role (`aria-controls` the listbox, `aria-activedescendant`
the highlighted option). Type to filter, ↑/↓ move, Enter opens, Esc closes and
focus returns to where it was. It lists:

- **Pages**: every page the admin can see (the menu config, the forward
  suite pages placed where the sidebar puts them, the account page);
  matching is case-insensitive on the label, the group and the route words,
  so "tunnel" also finds 隧道管理.
- **Actions**: the existing create flows of visible pages: 添加节点
  (`/admin/nodes?create=1`), 添加用户 (`/admin/users?create=1`) and 转发快速向导
  (`/admin/forward/setup`); appearance and language switches. The pages read
  the one-shot query with `useRouteIntent()` and remove it from the URL.
- **Users** (two characters or more, 250 ms debounce): the email filter of
  `GET /admin/users?email=`, five results, opening the user list filtered
  to that email. Nodes and forward rules have no list endpoint with a search
  parameter, so the palette does not search them.

### Account menu, account page, status pages

- **Account menu** (`components/shell/AccountMenu.vue`, Reka DropdownMenu):
  avatar → 账户, 外观 (跟随系统 / 浅色 / 深色), 语言 (简体中文 / English, each
  in its own language), 关于 (admin: version, build and commit, the line that
  used to sit at the bottom of the sidebar; Settings → 关于 takes it over in
  U7: 系统设置 → 关于 shows the same rows, `aboutRows`), 退出登录. Submenus on wide screens, inline radio groups on phones.
  `useTheme().setThemePreference('system')` forgets the stored theme and
  follows the system again.
- **账户** (`/user/account`, `/admin/account`, `views/Account.vue`): the
  profile (read-only, `GET /user/profile`), two-factor authentication
  (`/user/mfa/*`: status; turn on by scanning a QR code of the `otpauth://`
  key, or typing the key, then entering the first code in `UiOtpField`;
  recovery codes shown once, with copy and download; new recovery codes;
  turn off with the current password),
  language and appearance. Password change and signed-in devices have no
  endpoint, so they are not shown.
- **404 and 无权限** (`views/StatusPage.vue`, `UiEmptyState` with the page's
  H1): unknown paths under `/admin` and `/user` render inside their shell;
  other unknown paths, and an admin page opened by a signed-in user who is
  not an administrator, render a standalone page with the lockup. The URL
  stays as typed. Signed-out visitors still go to the login page.

### Motion and scrolling

- Pages fade in over 240 ms while rising 8 px (`ShellRouterView.vue`); the
  old page leaves at once. Under `prefers-reduced-motion` only the fade
  remains.
- `router/scroll.js`: a new page starts at the top, back and forward restore
  the position (waiting up to 1.5 s for the list to load), and a change of
  only the query keeps the position.

## Sign-in and user pages (U5)

All of them read the API through `utils/panelResponse.js` (`unwrapPanel`
throws on a `{ code != 0 }` envelope, so an error never renders as an empty
page) and have the same four states: nothing for 300 ms, then a skeleton
(`useDelayedLoading`); the content; an empty state that says what will
appear and offers the first action; and `components/common/LoadError.vue`
(what failed, the server's message, 重试, 复制错误详情 with status, request id,
page and time). No new endpoints or routes: page state lives in the query.

- **登录** (`views/Login.vue`): one 400 px card on the `--bg` backdrop with
  two faint brand-gradient washes (a brand moment). Steps, each with its own
  H1 that takes focus when it is not a field: email and password
  (`UiPasswordField` reveal) → 两步验证, six `UiOtpField` boxes (typing
  advances, Backspace steps back, paste or one-time-code autofill spreads,
  the sixth digit submits; `mfa_method: "totp"`) → or 使用恢复码, offered only
  when the server lists the `backup` method (`XXXX-XXXX`, upper-cased,
  `mfa_method: "backup"`). `mfa_enrollment_required` has no token and the
  `/user/mfa/*` setup endpoints need one, so the card explains what to do
  (the administrator pauses the policy, the user turns it on in 账户). No
  "忘记密码": there is no password-reset endpoint. Registration appears only
  when the public config enables it; the invite code field only with
  `require_invite`. Server errors show in one `role="alert"` above the
  button; field errors sit under their fields. No required asterisks on this
  form (`aria-required` instead).
- **概览** (`views/user/Dashboard.vue`): 你好，{邮箱前缀}, one sentence on the
  state, and the hero card: remaining traffic in a 200 px ring (brand
  gradient stroke; the number in the gradient, its first stop mixed 30 %
  towards indigo so the lightest edge keeps 3:1 on white), status, expiry,
  used (upload and download hidden on phones), the plan (订阅模板 in the
  community edition, 套餐 in the commercial one), 复制订阅链接 (primary),
  导入到客户端, and 数据更新于 … with 刷新 (`?refresh=true`). Below: the first
  three help articles in the administrator's order, and the three latest
  tickets. The commercial edition adds a card with 选购套餐 and 我的订单. The
  summary has no traffic-reset date and no balance, so neither is shown.
- **订阅** (`views/user/Subscribe.vue`, `composables/useUserSubscription.js`):
  the link (`UiCopyField`, the token from the profile, the domain picker when
  several are configured), its QR code (`UiQrCode`), and one-click import
  for the clients the kernel serves a format for
  (`views/user/subscriptionClients.js`: Clash Verge, Shadowrocket, sing-box,
  Stash, Surge, Quantumult X and Loon open their own URL scheme with the
  format's link; v2rayN copies its link). 其他格式 lists every format with
  copy and a preview Sheet (fetched from the page's own origin) with
  download. The danger zone's 重置链接… opens a `UiDialog` that asks for the
  current password, or with two-step verification on (`/user/mfa/status`,
  or the server's `mfa code required`) the `UiOtpField` code or a recovery
  code, and calls `POST /user/subscription/reset`; it then shows the new
  link with its QR code, keeps the new token in the user store and reloads
  the page data.
- **帮助中心** (`views/user/Knowledge.vue`): a centred search (`/` focuses
  it; `?q=`), category cards with counts (`?category=`), and the list. An
  article opens in place (`?article=`) at `--size-content-read` (692 px)
  with a table of contents (three headings or more), previous / next within
  the list the reader came from, and 提交工单. Bodies are rendered by
  `utils/articleMarkup.js` + `ArticleBody.vue` as elements, never HTML:
  headings, paragraphs, lists, quotes, fenced code, bold, inline code and
  http(s)/mailto links.
- **工单** (`views/user/Tickets.vue`): the list and the conversation side by
  side from 834 px (`?ticket=`); below it the list, then the conversation
  full width with 我的工单 to go back (its subject becomes the page's H1).
  Replies send with the button or Ctrl/⌘ + Enter; 关闭工单 confirms. 新建工单
  opens a Sheet (subject, priority, details; `?new=1` opens it from other
  pages) and selects the new ticket.
- **套餐 / 订单** (commercial edition only): store-style plan cards (price in
  Title 1 through `useFormat().money`, a billing-period segmented control,
  the plan's limits and description lines), a checkout dialog with a coupon,
  and the orders as a list with a details Sheet. 去支付 says that online
  payment is not available yet.

New components: `UiOtpField` (a fieldset of one-digit boxes) and `UiQrCode`
(the `uqr` encoder, MIT, 3.8 KB gzip in its own `qr` chunk, loaded on first
use; dark modules on a light tile in both themes, because many scanners
cannot read an inverted code).

## List pages (U6)

Every list in the admin console uses one template (plan §7.1) and one
component, `UiDataTable`. Shared layout classes are in
`web/src/styles/pages.css` (`.list-page`, `.list-page__search`,
`.list-page__note`, `.form-grid`, `.dialog-section`, `.form-error`).

```
UiPageHeader      title (the nav name) · one sentence · primary "新建…" on the right
UiDataTable       #toolbar: UiSearchField + UiFilterChips · table settings (columns, row height)
                  rows → click / Enter opens a UiSheet quick view (or the detail page)
                  "…" row menu (UiMenu) · selection → bulk bar floating up from the bottom
UiSheet           details as grouped lists, actions, then a danger zone
UiDialog          create / edit forms with Ui fields in a .form-grid
```

**`UiDataTable`** (`web/src/ui/UiDataTable.vue`, story "DataTable"):

- **Columns** are objects: `key`, `label`, `sortable` (`firstDirection:
  'desc'` for dates and amounts), `align: 'end'`, `numeric` (tabular
  numbers), `width`, `value(row)`, `format(value, row)`, `sortValue(row)`,
  `hidden` (off by default), `hideable: false`, `nowrap` (one line), `truncate` (one line with an ellipsis and the full text as the cell's `title`; set `maxWidth`), `minWidth` / `maxWidth`, `breakpoint: 'md' | 'lg'`
  (left out of the table below that width), `primary` / `secondary` (title
  and subtitle of the phone card), `card: false`. Cells render text (empty is
  `—`) or the slot `#cell-<key>="{ row, value, card }"`.
- **Sorting and pages**: client-side by default (`pageSize` turns pages on);
  `manualSort` and `manualPagination` + `total` for the server, with
  `update:sort` / `update:page`. Header buttons cycle none → ascending →
  descending and set `aria-sort`. `manualSort` is table-wide (the table sorts
  no column itself), so a server-sorted list makes only the columns the API
  sorts by `sortable`: see "Server-side sort" below.
- **Selection**: `selectable` + `v-model:selected` (row keys, `rowKey`
  default `id`). A header checkbox selects the page (mixed when partly
  selected); the bulk bar (slot `#bulk-actions="{ rows, clear }"`) floats
  up from the bottom (under the overlays; it fades out while a modal Dialog
  or Sheet is open) with the count, "全选所有 N 条" when every row is loaded,
  and 取消选择 (Esc too). Only offer bulk actions an endpoint supports. The
  users (封禁 / 解封 / 重置流量) and invite codes (撤销) have bulk endpoints
  (`POST /api/v4/admin/users/bulk`, `/api/v4/admin/invite-codes/bulk`) and
  make one request for the selection; see "Bulk actions" below. A bulk action
  without a bulk endpoint calls the per-row endpoint for each row and offers
  撤销 where there is an inverse.
- **Rows**: `rowActions(row)` gives the "…" menu items (`{ key, label, icon,
  danger, separatorBefore, onSelect }`; the action runs after focus is back
  on the trigger, so a dialog it opens returns focus there). `activatable`
  rows emit `row-activate` on click (not on controls inside) and Enter; ↑/↓,
  Home and End move between rows (one tab stop); Space toggles selection.
  `rowLabel(row)` names the row for "选择 {name}" and "{name} 的操作".
- **States**: `error` (+ `errorTitle`, `retry` event) shows `UiErrorState`
  (it reads the message of an axios error from `msg`, `message`, the kernel's
  `{ error: { code, message } }` or the error itself);
  `loading` with no rows shows skeleton rows after 300 ms, with rows it dims
  them; no rows shows the empty state (`emptyTitle`, `emptyDescription`,
  `emptyIcon`, slot `#empty-actions`), or "没有结果" + 清除筛选
  (`clear-filters`) when `filtered`.
- **View**: the header sticks under the admin top bar
  (`--shell-topbar-height`); comfortable (52 px rows) or compact (40 px);
  hidden columns and row height are remembered per `storageKey` in
  `localStorage` (`anix.table.<key>`, guarded: blocked storage just means
  defaults). `flat` drops the surface for tables inside dialogs and sheets.
- **Phones** (< 640 px): a list of cards instead of the table (no sideways
  scrolling): checkbox, title (a button when `activatable`), subtitle, the
  first three visible fields with their labels, and the "…" menu.
- **Accessibility**: a real `<table>` with a hidden caption (`label`),
  `scope="col"`, sortable headers as buttons with `aria-sort`, named
  checkboxes and menus, a polite live region for the selection count, and
  the bulk bar as a named region.

**Server-side sort.** The user, order and node lists pass the sort to the
server (`sort` and `order` of `GET /api/v2/admin/users|orders|nodes`, the
whitelists in `docs/guide/api-reference.md`). A page makes the table
`manual-sort` with `:sort` and `@update:sort`, and keeps the mapping from its
column keys to the API's names in `createListSort()`
(`web/src/utils/listSort.js`):

| List | Sortable columns (table key → `sort`) |
|---|---|
| 用户 | 邮箱 `email`, 已用 / 总流量 `traffic` (used, newest first), 到期时间 `expired_at`, 注册时间 `created_at`, ID `id` |
| 订单 | 订单号 `trade_no`, 状态 `status`, 金额 `total_amount`, 创建时间 `created_at` |
| 节点 | 名称 `name`, 地址 `host`, 负载 `cpu_usage`, 最后心跳 `last_check_at`, ID `id` |

A header click sets the sort, goes back to page 1 (and drops the selection)
and writes `?sort=<API name>&order=asc|desc` next to the other filters (an
unknown value in the URL is ignored); the third click is no sort and sends
neither parameter, so the API's own order applies. A column the API cannot
sort by (the derived status, a plan name, the buyer's e-mail, the node's
protocol count and total traffic, the Agent columns) is **not** sortable
rather than sorted over the loaded page: sorting 20 rows of a long list says
nothing about the list, and the status and order chips already filter on the
server.

**Bulk actions.** `useBulkReport()` (`web/src/composables/useBulkReport.js`)
and `readBulkResult()` (`web/src/utils/bulkResult.js`) read the per-item
answer of the bulk endpoints (`{ results: [{ id, ok } | { id, ok: false,
error: { code, message } }] }`; `not_found`, `conflict`, `forbidden_self`,
`not_attempted`, `failed` or a gateway code). The result is the toast
pattern of the forwarding bulk bar: all done is a success toast (with 撤销
where there is an inverse: ban ↔ unban, applied to the users that changed);
some failed is a warning that stays until dismissed, saying what was done and
why the rest was not (a count per cause, with the server's text for an
unnamed code) with 重试 for the ids a second attempt can change (`not_found`,
`conflict` and `forbidden_self` are final, so never retried); none done is
the same as an error. The rows that failed stay selected, so which ones they
are can be read from the table. A request that fails as a whole is a toast,
or inline in the confirmation when the action has one. The bulk requests have
a 30 s timeout (the server's budget is 25 s). 重置流量 asks first, makes one
`Idempotency-Key` per click (the server derives one per user) and sends it
again for a failed request and for 重试, so a counter is never reset twice.

**Filters and search.** Use the API's filters (the users' `status`, the
orders' `status`) as chips; a filter the API lacks narrows the loaded page
only and says so under the table (Users: 流量用尽). Search calls the server
300 ms after typing stops and on Enter.

Migrated in U6 (also listed in `web/scripts/data-table-pages.mjs`):
- **用户** (`Users.vue`): search by email; chips 正常 / 已到期 / 已封禁 (the
  API's `status`, with the counts from the stats) and 流量用尽 (loaded page
  only); used / total as a `UiUsageBar`; 最近在线 (below); the row opens a
  detail Sheet (subscription, activity, actions, danger zone); sorting on the
  server; bulk 封禁 / 解封 / 重置流量 are one request each (see "Bulk
  actions"). The limits, ID and 注册时间 columns start hidden (at 1440 px
  最近在线 takes the room 注册时间 had; the registration time is in the detail
  sheet's description).
  **最近在线** is `GET /api/v4/admin/users/activity?ids=…` for the ids of the
  page, asked for after the list is shown (`views/admin/users/useUserActivity.js`;
  an answer for an earlier page is dropped) and never holding it up: a
  relative time with the exact time on hover (`useFormat().relativeTime`),
  "从未" for `null`, a dash while loading, "暂不可用" when it fails
  (`UserLastOnline.vue`, also the 活动 row of the detail). Not sortable: the
  API has no such sort.
- **工单** (`Tickets.vue`): an inbox like the user tickets page, the queue on
  the left and the ticket with quick replies on the right (full width on
  phones, with a back button). The admin list has no message thread, so the
  panel shows the ticket and the reply box.
- **帮助中心内容** (`Knowledge.vue`): articles in a table with category chips;
  the editor is a wide dialog with the Markdown source and a live preview
  (U5's `parseArticle` + `ArticleBody`: elements, never HTML), a Write /
  Preview switch below 834 px; show / hide from the row menu with 撤销.
- **插件中心** (`Plugins.vue`): an App Store style card grid (icon, name,
  publisher, versions, health) with search and health / target chips; the
  details Sheet and the install dialog keep their behaviour and test ids.
- **NodeX Agents** (`Agent.vue`), **Ansible 机器** (`AnsibleMachines.vue`,
  execution plane: visuals only), **邀请码** (`InviteCodes.vue`, bulk copy
  and revoke: one `POST /api/v4/admin/invite-codes/bulk`), **访问组** (`AccessGroups.vue`, the group opens in a large
  Sheet with members, plans, grants, quotas and a danger zone).
- Commercial: **订单** (details Sheet, server status chips), **优惠券**,
  **套餐** (details Sheet with groups), **支付** (gateways / records / stats
  tabs), **邀请返佣** (withdrawals / stats / rules). Edition gating is
  unchanged.

**转发节点 (U7).** The execution-plane pages keep their own routes and
share a header and a run-mode switch
(`views/admin/forward-nodes/ForwardNodesModeNav.vue`: real links styled as
a segmented control, `aria-current="page"` on the current one). A NodeX
node and an Ansible machine open a detail page built on
`forward-nodes/NodeDetailLayout.vue`, the detail template of plan §7.2:
back link, title, status badges and actions, then `UiTabs` sections
(概览 / 配置 / 危险操作) of `UiGroupedList`s, with a delayed skeleton, an
error state with 重试 and a "no longer exists" state. The open section is
in the query (`?tab=config`, `useDetailTab`). The NodeX and local runtime
pages use `RuntimeStatusPanel` (probes, commands, Doctor output) and
`RuntimeJobsTable`.

The NodeX and local runtime settings use the settings template's sticky
保存 / 放弃 bar (`SettingsSaveBar`) and ask before leaving with unsaved
changes; the local runtime keeps 保存并启用 in the header while it is on
standby.

Library buttons, checkboxes, switches and chips set `min-height: 0` and get
their 44 px touch area from `::after` on coarse pointers.

## Node pages (U7)

The proxy nodes (`Node`, `/admin/nodes`; not `ForwardNode`) use the list
template and the first detail page (plan §7.2). Page-local parts live in
`web/src/views/admin/nodes/`.

```
/admin/nodes            UiPageHeader (注册密钥 · 部署父节点 · 添加节点)
                        UiDataTable: search + status chips + sort (?q= &status= &sort= &order= &page=)
                        row → node page; "…" → 打开 / 协议 / 日志 / 同步 / 编辑 / 删除
/admin/nodes/:id        back link · name · status badge · 编辑 · 同步并重载 (primary)
  ?section=             UiTabs variant="segmented":
                        overview | traffic | protocols | credentials | deploy | logs | danger
```

- **Detail page template.** `UiPageHeader` with the `#back` slot (a link
  back to the list that keeps the list's query), the status in `#meta`, one
  primary action; sections are `UiTabs` panels named in `?section=` (the
  default section leaves the query clean, `router.replace`); each section is
  a `UiSection` with grouped lists, cards or a `UiDataTable`. Panels mount
  when shown, so a section loads its data the first time it opens; state that
  must survive switching (a just-generated registration key) belongs to the
  page. The route sets `meta.titleKey`, so the breadcrumb reads
  网络 › 节点 › 节点详情.
- **Status.** The list endpoint reports online/offline from the last
  heartbeat; `GET /admin/nodes/:id` returns the stored column, so
  `nodeData.displayStatus()` applies the same five-minute rule.
- **Agent connection.** The list and the overview show how each node's Agent
  reaches Control and the state of its certificate, from the transport
  inventory (`GET /api/v4/kernel/agents/transports?node=proxy-<id>,…`, at
  most 200; the nodes are joined by `proxy-<id>`, never `forward-<id>`).
  The list calls it once per page after the list has loaded
  (`nodes/useNodeTransports.js`: a late answer for another page is dropped, a
  failure only turns the two cells into "暂不可用") and has two columns: 连接方式,
  a `UiBadge` chip (mTLS 流 success, API 密钥流 info, 旧版 warning, 第三方
  and 离线 neutral: the node's own status badge already says offline), and 证书,
  a chip (有效 success, 续期逾期 warning, 已吊销 and 已过期 danger, 无证书
  neutral) with the day that matters (until / expired / revoked). The overview's
  *Agent connection* group (`NodeAgentConnection.vue`) loads the node's own row
  and lists the connection, its transport, when it was last seen, the
  certificate state, `not_after`, `renew_after` and, for a revoked one,
  `revoked_at` and `revoke_reason`. A valid certificate past `renew_after`
  is "续期逾期" with a notice (the Agent did not renew in time); a revoked
  one says the Agent has to enroll again. Neither call holds the page up.
  The columns are not sortable: the API has no sort for them. To make room
  at 1440 px 版本 and 负载 start hidden (the node page has both; the table
  settings bring them back).
- **Traffic.** 流量 (`nodes/NodeTrafficSection.vue`, a lazy chunk, loaded
  when the section opens) draws `GET /api/v4/kernel/nodes/:id/traffic`
  through `nodes/NodeTrafficChart.vue`, which the live monitor's sheet reuses.
  The range switch is 24 h, 7 d and 30 d by the hour (whole UTC hours: 24,
  168 and 720 buckets, the route's maximum) and 90 d and 1 y by the day (the
  host's calendar days; `since` is `buckets - 1` days back and `until` is left
  out, 90 and 365 buckets, so a host midnight a day earlier than the browser's
  stays inside the 366 the route allows); `nodeTraffic.js` has the windows
  (`trafficQuery`), the reader of the answer and the chart option, and
  `useNodeTraffic.js` the state (a late answer to an earlier range or node is
  dropped; the series of the previous range stays on screen, dimmed, while the
  next loads). The metric cards are the range's upload, download and total
  (`format.bytes`, one decimal); the chart is two `UiChart` lines (download with
  a soft area) on one axis in the unit of the peak (`axisUnitFor`, shared with
  the user-traffic chart), its tooltip uses `format.bytes`, and the plot is
  named with a summary sentence (range, totals, busiest hour or day) and has
  the numbers as the chart's data table. An hour is labelled in the viewer's
  time zone; a day by its noon (`bucketLabelTime`), so its date is right for a
  browser up to twelve hours from the host. The loading skeleton (after 300 ms),
  the empty state (nothing moved: it says hourly history ends where the traffic
  log was purged) and the error state (the route's own message, 重试) are
  `UiChart`'s; a note under the chart says whether the numbers are hours from
  the log or days from the daily statistics. The range is local state, not in
  the URL.
- **Rotating Agent credentials.** 凭据 has an *Agent credentials* group with
  `nodes/NodeRotateCredentials.vue` (also on the forwarding node page's Agent
  card, for an Agent node that is not a proxy node, hidden when the forward
  API reports `can_delete: false`; the forward page passes `size="md"`). The
  button opens a danger `UiConfirmDialog` (its default slot holds the fields,
  which `useConfirm()` cannot carry) that names the node, lists what happens,
  and takes the credential's lifetime (a `UiSelect`: 1 h, 6 h, 24 h, 7 d), a
  reason (a `UiTextarea`, counted in UTF-8 bytes with its 200 limit because the
  server counts bytes) and, for a proxy node only, a `UiCheckbox` to also
  replace the API key, with a danger notice when it is ticked. The result is a
  `UiDialog` (the scrim does not close it; focus starts on the copy button and
  returns to the trigger) with a warning, the credential in a masked
  `UiCopyField` (copy with the library's fallback that selects the text), the
  expiry with a once-a-second countdown (a `<time>`, not a live region) or an
  "expired" notice, what was revoked, whether the API key was replaced, and a
  link to `docs/guide/agent-onboarding.md#rotating-a-nodes-credentials`. It
  shows no install command: the `--reset` variant the guide describes does not
  exist yet. `useCredentialRotation.js` keeps the state (`stage`, `form`,
  `busy`, `error`, `result`): the credential is in `result` only, `dismiss`,
  `cancel` and the end of the scope clear it, a request that answers after that
  is dropped, and `rotated` carries the node and whether the key was replaced,
  never the secret. The console does not know who is a super administrator
  (the forwarding pages learn it from `can_delete`), so the proxy node page
  offers the action to every administrator; a 403 shows the reason inside the
  confirmation and turns the button into a sentence, and a disabled node's
  button is off with its reason (the route answers 409 `node_disabled`).
- **Secrets.** 凭据 calls `GET /admin/nodes/:id/credentials` only on 读取 API
  密钥 (the server audits every read) and shows the API key in a masked
  `UiCopyField`; the shared secret is never shown. Registration keys are
  shown once, when generated. Protocol secrets stay `********` and are sent
  back as such.
- **Pieces.** `useNodeDeploy` (registration key, connection settings,
  config.json, Ansible inventory), `useProtocolEditor` (JSON / form editor and
  request bodies), `useNodeActions` (sync, typed-name delete), `nodeData`
  (payload readers, request bodies, status), and one component per sheet or
  section. `NodeCodeBlock` shows generated files with a copy button.

## Forward suite and the wizard template (U7)

**Removed in v4.2 (F5d).** The pages below were deleted with the flux v2
forwarding API; the section is kept as a record. The flux-panel clone pages
(`docs/archive/flux-panel-clone.md`) moved to the
list page template with visual and interaction changes only: same fields,
endpoints and flows.

- **流量转发** (`views/admin/Forward.vue`, the route entry, with page-local
  components in `views/admin/forward/`): `ForwardRulesTable` (a
  `UiDataTable` with the service `UiSwitch` and one status badge (the
  worst state; the detail in a Reka tooltip, a second line on cards), addresses that
  copy or open the address list, the row menu 编辑 / 诊断 / 上移 / 下移 / 删除,
  and a drag handle in the name cell), `ForwardGroupedView` (per user, per
  tunnel, flat tables), and the import, export and address dialogs. The
  toolbar has the search, the tunnel `UiSelect`, status chips and the
  直连 / 分组 `UiSegmentedControl`; 导入 / 导出 sit in the header's "…" menu.
  The editor is a `UiSheet` whose form the footer button submits (`form`
  attribute), so Enter in a field saves too.
- **隧道** and **限速**: `UiDataTable` + editor `UiSheet`.
- **Diagnosis timeline** (`components/admin/forward/DiagnosisTimeline.vue`):
  an ordered list of steps (green check or red cross marker, title, node,
  a field grid, the message) under a "2/3 项通过" badge and the check time.
  Presentational: the page maps the diagnose response to `steps`.
- **Wizard template** (plan §7.5, `views/admin/ForwardWizard.vue`): a
  240 px step list on the left (number, check when done, `aria-current="step"`),
  the step in a card on the right with "第 2 步，共 5 步" and its title (which
  takes focus when the step changes), and a sticky footer with 上一步 and
  the primary action. Below 834 px the steps become a row of five above the
  card; below 640 px only their numbers show. Each step component keeps its
  fields and API calls, renders `<form id="wizard-step-form">` and exposes
  `submit`, `busy`, `canSubmit` and `primaryLabel` for the footer. Records
  a step can reuse are a `WizardExistingList` with "使用现有".

### v4.2 forwarding mockups (F5b, H16)

The proposed v4.2 forwarding screens are dev-only prototypes under
`web/src/mockups/forward/`, at `/admin/__mockups/forward/<screen>`. They use
the `Ui*` components with mocked `/api/v4/forward` data and call nothing.

- The route is registered only when `import.meta.env.DEV` is set, so
  `vite build` drops it and its chunk. Bundle budgets, visual baselines and
  the menu are unchanged.
- The copy is inline zh-CN mockup text, with no i18n keys.
- The screens, the screenshots and the open design questions are in
  [`docs/design/forward-ui/README.md`](../design/forward-ui/README.md).
- The final F5b pages replace this directory.

## Settings pages (U7)

Settings-style admin pages use the settings template (plan §7.3):
`components/admin/settings/SettingsLayout.vue` (section list on the left;
below 834 px the page without a section shows only the list and a section
shows a back link) and `SettingsSaveBar.vue` (保存 / 放弃, floats up at the
bottom while a form differs from what was loaded; 保存 is disabled while a
field is invalid). `composables/useUnsavedChanges.js` asks 「放弃未保存的更改？」
before the route or its path changes (a query change never asks) and lets
the browser prompt on reload. Sections are routes (`/admin/<page>/:section`)
and render their own component, which loads its own data; the sections are
listed once in `navigation/menu.js` (`ADMIN_PAGE_SECTIONS`), which feeds the
section list, the page titles and breadcrumb (`utils/pageMeta.js`) and the
command palette.

| Page | Path | Sections |
|---|---|---|
| 系统设置 | `/admin/system/:section` | 通用 (subscription domains, configuration keys), 转发运行时 (backend status, doctor, jobs, operator commands), 备份, 负载均衡, 审计日志, 关于 |
| 安全 | `/admin/security/:section` | 两步验证 (`MFA.vue`), 访问组 (`AccessGroups.vue` with `embedded`: its header becomes an H2), API 令牌 (`security/ApiTokens.vue`, a lazy chunk: see "API tokens") |
| 通知 | `/admin/notifications/:channel` | 邮件, Telegram, 模板, 发送记录 as segmented tabs (`UiTabs variant="segmented"`) |

Old URLs redirect: `/admin/mfa`, `/admin/access-groups`, `/admin/telegram`.
订阅分组 is a list page with a detail page (plan §7.2,
`/admin/subscriptions/:id/:section`: 概览, 节点模板, 节点协议, 成员, 订阅输出).
Its sections are segmented `UiTabs` bound to the path, like the node and forward-node detail pages.
成员 (`views/admin/subscriptions/GroupMembers.vue`) shows the counts, then a
paged `UiDataTable` of the users granted the group directly
(`GET /api/v4/admin/subscription-groups/:id/members`: search by e-mail, a
有效 / 已过期 chip, 20 per page, membership end, quota override, renewal price,
no credential) with its loading, empty and error states and cards on a phone;
the table keeps its own state (the page's URL names the section only). Users
who reach the group through a plan or their primary group are not members of
this list, as the counts do not count them.

### API tokens (安全 → API 令牌)

The administrators' personal access tokens
([guide](../guide/admin-api-tokens.md), [reference](admin-api-tokens.md)):
`GET`, `POST` and `DELETE /api/v4/kernel/api-tokens`. It sits in 安全 next to the
two-factor policy because creating a token needs the same re-authentication; the
section is `defineAsyncComponent` in `Security.vue`, so its code (and the
`adminApiTokens` messages, in the `adminPages` group; only the section's label is
in `adminSecurity`, which the palette reads) loads when it is opened. The pieces
are in `web/src/views/admin/security/`:

| File | What it holds |
|---|---|
| `ApiTokens.vue` | The section: `UiSection` with *Create token*, the quota line, a `UiDataTable` (cards on a phone), the revoke confirmation |
| `ApiTokenCreate.vue` | The button, the form dialog and the one-time token dialog |
| `useApiTokens.js` | List state: the two chips, a stale answer dropped, the count against 25, revoking |
| `useApiTokenCreate.js` | Create state: the form, the re-authentication step, the refusals, the token |
| `apiTokens.js` | Plain readers and rules: the row, the state of a token, the name and expiry checks, `classifyTokenRefusal` |

- **The table.** Name with `anixadm_…<last four>` and the creation date (the
  date only, the exact time on hover: seven columns do not fit the 876 px a
  1440 px screen leaves the section), scope badge (Read info, Admin warning),
  status (`tokenState`: Active, Expires soon within 7 days, Expired, Revoked; the
  two ended states are neutral and the words tell them apart), expiry as a
  relative time with the exact one under it ("No expiry" for none; "Revoked 20
  days ago" with the `revoke_reason` for a revoked one), last use as a relative
  time with `last_used_ip` ("Never used"), and the row menu: *Copy token ID* (the
  id the operation log carries) and, for an active token, a danger *Revoke
  token…* `UiConfirmDialog` that names the token and says that its next request is
  refused (`onConfirm`: a failure, or a 404 "gone or not yours", shows inside it).
  The chips *Revoked and expired* (`include_inactive=true`) and *All
  administrators* (`all=true`, adds the Owner column) ask the route again.
  The list is newest first, at most 500 rows.
- **Who is a super administrator.** The profile has no such field, so (as for
  rotating credentials) *All administrators* is offered to everyone and a
  `403 super_admin_required` turns it off for the page's lifetime and shows a
  sentence. The rows carry only `user_id`, so another administrator's token reads
  "Administrator #7" and the quota line counts only the caller's own active
  tokens (`ownActive`); at 25, *Create token* is disabled with that line saying why.
- **The form.** Name (1 to 100 characters, counted as characters), scope as a
  `UiRadioGroup` whose option texts say what each can do (Read: `GET` and `HEAD`,
  never the two reads that answer a secret in clear, a node's credentials and the
  Telegram bot token; Admin: what you may do, except manage tokens), expiry as a
  `UiSelect` (30, 90 (recommended, the default), 180, 365, 730 days, *Custom…*
  with a 1 to 730 field, *No expiry* with a warning) and the re-authentication,
  which follows the subscription reset in `Subscribe.vue`: `getMfaStatus()` picks
  the password or a six-digit `UiOtpField` (sent as `{ code, method: 'totp' }`),
  with a switch to a recovery code (`{ code: 'XXXX-XXXX', method: 'backup' }`). The
  form is `novalidate`: the same checks run in `useApiTokenCreate.submit` and the
  field to focus comes back.
- **Refusals** (`classifyTokenRefusal` reads the kernel's `{ error: { code,
  message } }`): `step_up_failed` on the credential field (and the credential is
  cleared), `step_up_required` split by its message (`password is required`
  switches to the password, `an MFA code is required` to the code, `sign in again
  and retry within 10 minutes`, which is what the kernel says once identity holds
  the credentials, is a form-level alert with a *Sign in again* button that signs
  out and goes to `/login`), `409 too_many_tokens`, `429` (`step_up_rate_limited`,
  or any 429; the wait from `Retry-After` in whole minutes), `400
  invalid_request`, and anything else with the route's own message.
- **The token is shown once** in a `UiDialog` whose scrim does not close it (Esc,
  × and Done do): a warning that it will not be shown again, the token in a
  masked `UiCopyField` (reveal toggle, copy with the clipboard fallback that
  selects the text), the name, scope and expiry, and a usage hint, a
  `UiCodeBlock` with `curl … -H "Authorization: Bearer $ANIXOPS_TOKEN"` on this
  origin and the rule never to put it in a URL. The hint never contains the
  token, so the secret is in one place on screen. `useApiTokenCreate` keeps it in
  `result` (a `shallowRef`) only: `dismiss`, `cancel`, `open` and the end of the
  scope clear it, a request that answers after that is dropped, `onCreated`
  carries the stored record (id, name, scope, `hint`) and never the token, and
  the form's password and codes are cleared on success, on a refused credential
  and on close. `src/__tests__/apiTokenCreate.test.js` and the e2e spec check the
  DOM (markup, text and field values), storage, the URL, the console and every
  later request after each way of closing.
- **No credential in the console.** `utils/request.js` logs the whole axios error
  of a failed request, and that error holds the request body. A request marked
  `sensitive: true` (`createKernelApiToken`) logs only its method, URL and status
  and has its `data` removed from the error. Other requests are logged as before
  (the subscription reset's password still is: mark it when that page is next
  changed).
- **Fixtures.** `e2e/fixtures/apiTokens.js` answers the three routes from a little
  per-screen state (tokens created and revoked in it), accepts one password, one
  code and one recovery code, and has scenarios for the refusals (`mfa`,
  `cutover`, `tooMany`, `rateLimited`, `notSuper`, `full`, `revokeFails`,
  `revokeGone`). The create flows work a Select inside a dialog, so they run on the
  real clock (see "The screens' fixed clock and Vue's events").

### Telegram test message (通知 → Telegram → 测试消息)

`views/admin/notifications/TelegramTestPanel.vue`, a section of the Telegram
channel above 发送消息. The button posts `{}` to `POST /api/v4/kernel/notifications/telegram/test`
(`testTelegramBot()` in `api/admin.js`, the v4 base with `sensitive: true`, so a
failed request is logged by method, URL and status only) and shows
`data.class` in the tone of `utils/telegramTest.js`: success for `ok`, warning for
`chat_not_found`, `bot_blocked`, `rate_limited` and `not_configured`, danger for
`invalid_token`, `network_error` and `unknown` (a class the server adds later is
`unknown`). The result is a `role="status"` block with a badge (the class in
words), the server's fixed `message` and a fix hint; for `network_error` the hint
is the `reason` (timeout, dns, tls, connect, canceled, other).

- 409 `telegram_not_bound`: "Telegram not linked" with the way to link it
  (the bot's `/bind` command; the console has no binding page of its own).
- 429: the button is disabled and shows "Try again in N s" from `Retry-After`
  (seconds or a date; 60 s when missing; at most an hour). The countdown counts
  timer ticks, not `Date.now()`, so a frozen clock cannot stall it.
- The button is `loading` during the request and ignores a second click.
- An unsaved bot form gets a note: the test uses the saved settings.
- The token is never read, shown or logged here; the route neither takes nor
  returns it. Tests: `TelegramTestPanel.test.js`, `telegramTest.test.js`, the
  `admin-telegram*` screens and `e2e/admin-wired-apis-4.spec.js`. The Telegram
  flows run on the real clock (the countdown is a timer).

## Dashboards and monitoring (U8)

The dashboard template (plan §7.4): 3–4 metric cards → one main chart →
two columns (what needs attention, recent activity). Each block loads,
fails and retries on its own, so one failing endpoint never blanks the page.

| Page | Path | Content |
|---|---|---|
| 仪表盘 | `/admin/dashboard` | 用户, 在线节点 / 总数, 今日流量, 待处理工单 cards; 24 h traffic (`/admin/traffic/hourly`); 需要处理 (offline nodes, tickets waiting for a reply, stalled traffic reports, pending orders in the commercial edition, and the kernel's alerts: see "Kernel alerts in 需要处理"); 最近操作 (audit log) |
| 流量与监控 | `/admin/monitor/:section` | 实时节点 (default, `/admin/monitor`: the monitor WebSocket in a `UiDataTable`), `traffic` 用户流量, `latency` 节点延迟, `forward` 转发 (topology, ingress comparison, runtime jobs) |
| 部署编排 | `/admin/deployments` | Topologies and node roles (tabs), the operation timeline, the topology workspace (dialog) and the node-role sheet |

- 流量与监控's sections are segmented `UiTabs` bound to the path and listed in
  `ADMIN_PAGE_SECTIONS.monitor` (menu match, ⌘K). The time range
  (`views/admin/monitor/useMonitorRange.js`: 1h, 24h, 7d, 30d) is in
  `?range=` (24h leaves it out) and is offered only by the sections whose
  API takes one (用户流量, 节点延迟). Only 实时节点 opens the WebSocket.
- 实时节点 has no history of a node, only its load now, so a row (Enter on a
  focused row too, or 查看流量 in its "…" menu, which also has 打开节点页面)
  opens a `UiSheet` (`monitor/MonitorNodeTraffic.vue`, a lazy chunk) with the
  node page's traffic chart (`GET /api/v4/kernel/nodes/:id/traffic` by the
  node's id from the WebSocket) and a link to the node page's 流量 section.
  The user-traffic section keeps `/admin/traffic/hourly`.
- Old URLs redirect: `/admin/traffic-hourly` → `/admin/monitor/traffic`,
  `/admin/forward/observability` → `/admin/monitor/forward` (query kept).
- `components/admin/OperationTimeline.vue` (部署编排 and 插件中心) is an
  ordered timeline list with a state badge per operation; it is listed in
  `scripts/data-table-pages.mjs` so no bare table comes back.
- 部署编排 keeps its state and API calls in
  `views/admin/deployments/useDeploymentCenter.js`; the panels next to it
  only render.

### Kernel alerts in 需要处理

`DashboardAlerts.vue` merges `GET /api/v4/kernel/alerts` (`getKernelAlerts()`;
`docs/reference/kernel-alerts.md`) into the items it builds in the browser.
`Dashboard.vue` loads them with the other blocks (and on 刷新) but apart from
them: they never keep the page busy, and a failure is one item with 重试 (a 404,
a server without the route, shows none).

- **Tone and order.** `critical` is danger, everything else warning; danger
  items (offline nodes, critical alerts) come first, otherwise the order is the
  server's (critical, then the soonest to end).
- **Text** comes from `kind` and `detail` in `views/admin/dashboard/kernelAlerts.js`
  (`alerts.kinds.*` in both locale files): Agent, forward link and module
  certificates (about to expire or expired, with the end date), the module and
  forward link CAs (and whether a next CA is staged), the node credential split
  (stalled in a phase, or a finalize that was interrupted), the identity import
  and the cutover. The server's `message` is English, for notifications, and
  is only the fallback for a kind this build does not know. Dates use `useFormat`.
- **Links.** Node subjects `proxy-<id>` go to `/admin/nodes/<id>`, `forward-<id>`
  to `/admin/forward/inventory/forward-<id>`; the other subjects have no page.
- **Badge** from `summary`: "N active" (warning), "N active, M critical" (danger);
  the summary counts every active alert whatever the list shows.
- **Resolved history.** `UiSegmentedControl` "Alert status": Active (default) and
  Resolved (`status=resolved&limit=30`, the server keeps 30 days), each item
  saying when it resolved; the browser-built alerts belong to Active. A reply
  that arrives after the view changed is dropped.
- **Phones.** The toggle and badge wrap above the list; items wrap like the
  others and keep their 44 px touch area.
- **Tests.** `AdminDashboard.test.js`, `kernelAlerts.test.js`, the
  `admin-dashboard*` screens and `e2e/admin-wired-apis-4.spec.js` (axe,
  light and dark, desktop and phone).

## Bundle

### Performance budget (plan §13, U9)

`npm run bundle:budget` (`web/scripts/check-bundle-budget.mjs`) runs after
`npm run build`; CI runs it in the Frontend Build job. The build writes the
chunk graph (which chunk imports which, the CSS each pulls in, the source
modules in each) to `web/bundle-reports/chunk-graph.json`
(`web/scripts/vite-chunk-graph.mjs`, outside the served `web/public`). The
script adds up gzip sizes from it and fails when a number is over
`web/bundle-budget.json`:

- **Routes, first visit.** *Initial* is what the route needs before it
  renders: the entry with its static imports and CSS, the default locale's
  core messages (`boot`), and the route's layout and page chunks with
  theirs (for the admin route also the `admin` message group, which the
  router awaits; the `adminPages` group of the other admin pages is not
  part of it, see below). *With deferred* adds what loads right after (`after`: axios,
  with the first request).
- **Lazy chunks.** Every chunk except the entry, against one limit;
  `echarts` and `g6` are excluded (they load only with the pages that draw
  charts and graphs).

| Budget (gzip) | Measured at U9 | Limit | Why |
|---|---:|---:|---|
| Sign-in, initial | 115 KB | 120 KB | plan §13 |
| Sign-in, with axios | 134 KB | 141 KB | measured + 5 % |
| User home, initial | 168 KB | 178 KB | measured + 5 % |
| User home, with axios | 186 KB | 197 KB | measured + 5 % |
| Admin shell + dashboard, initial | 237 KB | 250 KB | plan §13 |
| Admin shell + dashboard, with axios | 255 KB | 269 KB | measured + 5 % |
| Largest lazy chunk (`vue-vendor`) | 42 KB | 80 KB | plan §13, per route chunk |

Before U9 the same measurement gave 278, 289 and 307 KB. After the
v4.1.0 follow-ups split the admin messages (below) the admin route
measures 210 KB initial and 228 KB with axios, against the same limits.

Admin messages come in two groups (`web/src/i18n.js`): `admin`
(`src/locales/<locale>.admin.js`) holds only what the shell and the
dashboard read (navigation and ⌘K labels, the settings save bar, the
dashboard), and `adminPages` (`<locale>.adminPages.js`) every other admin
page and the forward suite. The router awaits `adminPages` before the
first admin page other than the dashboard opens and otherwise loads it
when the browser is idle. A new admin namespace goes in `adminPages`
unless the shell reads it; `localeParity.test.js` pins the `admin` group's
namespaces.

When a change
needs more, raise the limit in `bundle-budget.json` in the same PR and say
why in the CHANGELOG; `--verbose` lists every file of every route by size.

### What keeps the sign-in page small

- **Locale messages** (`src/i18n.js`) come in two groups per language:
  `core` (`src/locales/<lang>.js`: sign-in, user pages, shell, components)
  loads at startup, `admin` (`src/locales/<lang>.admin.js`: the admin
  console and the forward suite) when an administrator is signed in (the
  router guard calls `loadMessageGroup('admin')`). Only the active
  language loads; switching loads the new language's active groups.
  `localeParity.test.js` keeps en and zh-CN on the same keys (so the
  fallback locale is not fetched) and the groups' namespaces disjoint. A new
  namespace for user or sign-in pages goes in the core file; one for admin
  pages goes in the admin file. Tests and Histoire load both groups.
- **App code is not grouped.** `vite.config.js` groups only vendor code
  (`chunkGroups`: `vue-vendor` with every `@vue/*` package, `router`,
  `i18n`, `axios`, `echarts`, `g6`, `qr`). API modules, composables and
  messages split automatically with the routes that use them; a manual
  group would also take every module its members import.
- **Reka UI has no group**: automatic splitting shares each primitive only
  between the routes that render it (one `ui-vendor` chunk cost the sign-in
  page 43 KB).
- **axios** loads with the first request (`utils/request.js`
  `loadRequestClient()`, `api/public.js`), the **extension runtime** with
  the first admin route, and vue-i18n is built without its legacy API
  (`__VUE_I18N_LEGACY_API__`).
- Import components from the pages that use them so routes stay lazy: the
  `@/ui` barrel pulls every component into the page that imports it, so
  pages import `@/ui/UiButton.vue` and `@/ui/composables/…` directly.
