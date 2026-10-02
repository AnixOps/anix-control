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
`design/tokens.css`, `styles/base.css`, then the legacy `style.css`.

- **`styles/base.css`**: reset; `body` on `--bg` / `--label-1` with the
  token font stack (system, Inter, then Chinese system fonts) and the Body
  step (line height 1.6 for Chinese); the 2 px `--accent` focus ring with a
  2 px offset; `.tabular-nums`; `.type-*` utilities for the seven type
  steps; scrollbars per theme; `prefers-reduced-motion` (no movement, short
  fades) and `prefers-contrast: more` (stronger separators). It also defines
  two values the tokens do not have yet: `--scrim` (dialog and drawer
  backdrop) and `--focus-ring`.
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

### Legacy variable bridge

Until every page is migrated, `web/src/style.css` keeps the pre-redesign
variable names as aliases of tokens, so old pages follow the brand and both
themes:

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

The global classes `.btn` (`-primary`, `-secondary`, `-ghost`, `-danger`,
`-sm`, `-lg`), `.card`, `.section-panel`, `.table-container`, `.data-table`,
`.tabs`/`.tab` (segmented control), `.status-badge`, `.modal*`, `.form-group`
and the form fields are restyled through tokens: pill buttons, 36 px controls,
10/14/20 px radii, hairline separators and the token shadows. The aliases and
classes are removed when the last page stops using them.

## Brand in the UI

`web/src/components/common/BrandLockup.vue` renders the lockup from
`brand.md` §2.4: the inline glyph (from the vendored `mark-glyph.svg`), then
"AnixOps" in 600 and the product word in 400. `tile` puts the glyph on the
brand-gradient tile (login page); `inverse` sets the text in white for the
slate backdrop; `size` is `sm`, `md` or `lg`. Page titles use "AnixOps
Control".

## Charts

`web/src/composables/useChartTheme.js` resolves the chart palette
(`--chart-1` … `--chart-8`) and the neutral roles from the document,
registers an ECharts theme per mode (`anixops-light`, `anixops-dark`), and
re-themes live charts with `chart.setTheme()` when the theme changes. The G6
topology takes node, label and edge colours from the same tokens.

## Lint rules

Warnings for now; the redesign makes them errors as pages migrate (colour
literals and native dialogs in U4, the type and radius scale in U9).

| Command | Rule | Warnings at U1 |
|---|---|---|
| `npm run lint:styles` (stylelint, `web/stylelint.config.mjs`) | colour literals (hex, named colours, `rgb()`/`hsl()`…) outside `src/design/` | 512 |
| | `font-size` off the scale (12/13/15/19/24/32/48 px, phone 16/28/34 px, or a variable) | 131 |
| | `border-radius` off the scale (6/10/14/20/980 px, 0, 50 %, or a variable) | 132 |
| `npm run lint` (ESLint, `web/eslint.config.js`) | `no-alert`: `alert`, `confirm`, `prompt` | 117 |

Neither command fails on warnings, and CI does not run them yet.

## Components

`web/src/ui/` is the component library (phase U2). It is built on
[Reka UI](https://reka-ui.com) 2.10.5 (headless primitives with the
keyboard, focus and ARIA behaviour; pinned) and our own scoped CSS that uses
only tokens (stylelint reports zero warnings for `src/ui/`). Pages do not
use it yet; U4 onwards migrates them. Names carry a `Ui` prefix so they never
shadow HTML elements: `import { UiButton, useToast } from '@/ui'`.

**Stories.** Every component has a Histoire story (`src/ui/stories/`):
`npm run story:dev`, `npm run story:build` (output in `web/.histoire/`).
The preview loads the vendored tokens and `base.css`; the toolbar's
dark-mode switch maps to `<html data-theme="dark">`, and `?lang=en` on the
preview URL switches the built-in strings to English. Histoire declares
Vite ^7; `package.json` overrides its `vite` peer to the project's Vite 8.

**Mount once.** `<UiHost />` (toast region and confirmation host) goes in
`App.vue` when the first page uses `useToast()` or `useConfirm()` (U4).

**Strings.** Built-in text (close, cancel, copied, undo, loading, status
words, units) comes from `ui.*` in `src/locales/modules/{zh-CN,en}/ui.js`,
written separately for each language. Page text is passed in as props.

### Which component

| Need | Use | Notes |
|---|---|---|
| An action | `UiButton` | One `primary` per view, rightmost; `secondary` otherwise; `tertiary` for low-emphasis text actions; `danger` only to confirm a destructive action, `danger-soft` for the button that starts one ("删除节点…"); `ghost` in toolbars. Sizes `sm` 28 (tables), `md` 36 (default), `lg` 44. Labels are verbs. |
| An icon-only action | `UiIconButton` | `label` is required (aria-label and tooltip); toggles pass `pressed`. |
| Text, numbers, passwords | `UiTextField`, `UiNumberField`, `UiPasswordField`, `UiTextarea` | Top label, help, error; units as `suffix`/`unit`. Height 44 by default (`md` 36 in toolbars, `sm` 28 in tables). |
| A custom control in a form | `UiField` | Gives the control its id, label, help and error wiring. |
| One of a short list | `UiSelect` | Up to ~10 options. |
| One of a long list, or search | `UiCombobox` | Filters as you type. |
| A setting that applies at once | `UiSwitch` | On/off; label left, switch right. |
| A choice applied on submit | `UiCheckbox` | `'indeterminate'` for "select all". |
| One of a few visible options | `UiRadioGroup` | When the options need descriptions. |
| A value that changes the view in place | `UiSegmentedControl` | 2–5 values (time range, list or grouped view). |
| Switching content panels | `UiTabs` | `underline` for detail sections; `variant="segmented"` for the pill look at the top of a page (转发规则 / 隧道 / 限速). |
| A focused task | `UiDialog` | sm 420 / md 560 / lg 760; full screen below 834 px for md and lg. |
| Confirming an irreversible action | `useConfirm()` / `UiConfirmDialog` | `requireText` for deleting nodes and users. |
| Details or an editor next to a list | `UiSheet` | Right drawer; bottom sheet below 834 px. |
| The result of an action | `useToast()` | See the rules below. |
| A status | `UiBadge`, `UiStatusDot` | `status` online, offline, disabled, pending, error; always a word. |
| Nothing to show, or a failed load | `UiEmptyState` | What will appear and the first action; for errors, retry and "复制错误详情". |
| Loading | `UiSkeleton` + `useDelayedLoading()` | text, card, table-row. |
| A link, token or command to copy | `UiCopyField` | `secret` masks it. |
| Page title | `UiPageHeader` | The only H1, one sentence, actions with the primary last. |
| Surfaces and settings rows | `UiCard`, `UiSection`, `UiGroupedList` + `UiGroupedListRow` | System Settings style rows: label left; value, control or chevron right. |
| Numbers, bytes, rates, money, dates | `useFormat()` | One implementation on Intl and the current locale. |

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

### Accessibility built in

| Component | Behaviour |
|---|---|
| Fields | `<label for>`; help, units and error in `aria-describedby` (error first); `aria-invalid`; errors in a polite live region; native `required` with a visual-only asterisk. |
| Select / Combobox | `role="combobox"` + listbox; Space/Enter/↓ open, arrows move, typing jumps or filters, Enter selects, Esc closes and returns focus. |
| NumberField | `role="spinbutton"` with min/max; ↑/↓ step, PageUp/PageDown ×10, Home/End; the wheel does not change it. |
| Switch / Checkbox / Radio | `role="switch"` / `checkbox` (`mixed`) / `radiogroup` named by its label; boundaries at 3:1 or more. |
| SegmentedControl | Named `group` of `aria-pressed` buttons with roving focus; one selection always. |
| Tabs | `tablist` / `tab` / `tabpanel` with `aria-controls`; arrows move and activate. |
| Dialog / Sheet | `role="dialog"`, `aria-modal`, labelled by the title and described by the description; focus trapped and returned; Esc; page scroll locked; the close button (Lucide x) is named "关闭" and comes last in tab order. A sheet focuses its first text field, else the close button. |
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
  change. `{ digits: 1 }` gives `128.5 GB`.
- `rate`: decimal bits per second from bytes/s (`{ input: 'bits' | 'mbps' }`).
- `duration`: the two largest units, 「3 天 4 小时」 / "3d 4h".
- `money`: from cents, `¥` in both languages.
- `date` `2026-11-30`, `dateTime` `2026-11-30 14:05` (24-hour, local time);
  `relativeTime` 「3 分钟前」 (show `dateTime` on hover).
- Missing values render as `—` (option `empty`).

### Bundle

Reka UI and its helpers (`@floating-ui`, `@vueuse`, …) build into a
`ui-vendor` chunk, kept out of `vue-vendor`. With every component imported
it is about 45 KB gzip (the components add 14 KB JS and 7 KB CSS); the set
the app shell will need first — `UiHost` and `UiButton` — costs about 9 KB
of it, against the 250 KB admin shell budget (plan §13). Import components
from the pages that use them so routes stay lazy.
