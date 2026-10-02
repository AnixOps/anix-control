// Style rules for AnixOps Design (docs/reference/frontend-design.md).
// Every page is migrated, so since U9 the rules are errors (they warned
// during the migration) and CI runs `npm run lint:styles`.
//
// Colours come from the tokens in src/design/tokens.css (var(--label-1),
// var(--accent), ...). Font sizes and radii must be on the token scale.

// Literal values must be on the scale; values built from custom properties
// (var(--type-body-size), calc(var(--radius-sm) - 2px), a component's own
// --size variable) pass, because the tokens are checked where they are defined.
// 16px is allowed for phone inputs (iOS zoom), 28/34px are the phone sizes
// of Title 1 and Display.
const FONT_SIZES = /(var\(--)|^(inherit|initial|unset|(12|13|15|16|19|24|28|32|34|48)px)$/
const RADIUS = /(var\(--)|^((0|50%|inherit|(6|10|14|20|980)px)(\s+|$)){1,4}$/

// The pre-redesign aliases removed from style.css in U9. They resolve to
// nothing now, so a copy-pasted var(--text-color) would render unstyled.
const LEGACY_VARIABLES = /var\(--(bg-color|bg-accent|surface-(color|muted|hover)|border-(color|strong)|text-(color|secondary|tertiary)|primary-(color|hover|soft)|(success|warning|error)-color|admin-sidebar-(surface|accent|hover|divider|text|text-strong|muted)|shadow-(sm|md|lg)|transition|color-(text-[123]|bg-[12]|border)|background-color|bg-secondary|sidebar-(width|collapsed-width)|header-height)\)/

export default {
  defaultSeverity: 'error',
  ignoreFiles: ['src/design/**', 'node_modules/**', 'public/**', 'coverage/**'],
  overrides: [
    {
      files: ['**/*.vue', '**/*.html'],
      customSyntax: 'postcss-html'
    }
  ],
  rules: {
    'color-no-hex': [true, { message: 'Use a colour token (var(--label-1), var(--accent), ...) instead of a hex literal' }],
    'color-named': ['never', { message: 'Use a colour token instead of a named colour' }],
    'function-disallowed-list': [
      ['rgb', 'rgba', 'hsl', 'hsla', 'hwb', 'lab', 'lch', 'oklab', 'oklch', 'color'],
      { message: 'Use a colour token (or a -soft token) instead of a colour function' }
    ],
    'declaration-property-value-allowed-list': [
      {
        'font-size': [FONT_SIZES],
        '/^border(-(top|bottom|start|end)-(left|right|start|end))?-radius$/': [RADIUS]
      },
      { message: 'Use the token scale: font-size 12/13/15/19/24/32/48 px (var(--type-*-size)), radius var(--radius-xs|sm|md|lg|pill)' }
    ],
    'declaration-property-value-disallowed-list': [
      { '/.*/': [LEGACY_VARIABLES] },
      { message: 'Legacy variable: use the design token (frontend-design.md, "Legacy variable map")' }
    ]
  }
}
