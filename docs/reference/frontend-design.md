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
