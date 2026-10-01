// Style rules for the AnixOps Design migration (docs/reference/frontend-design.md).
// Everything warns for now; the redesign plan turns them into errors once the
// pages are migrated (colour literals in U4, the type and radius scales in U9).
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

export default {
  defaultSeverity: 'warning',
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
    ]
  }
}
