// Minimal ESLint setup for the redesign rules (docs/reference/frontend-design.md).
// no-alert warns for now; the redesign plan (U4) replaces every alert,
// confirm and prompt with the Toast and ConfirmDialog components and then
// makes it an error.
import pluginVue from 'eslint-plugin-vue'
import globals from 'globals'

export default [
  { ignores: ['public/**', 'coverage/**', 'node_modules/**', 'playwright-report/**', 'test-results/**'] },
  ...pluginVue.configs['flat/base'],
  {
    files: ['src/**/*.{js,vue}'],
    languageOptions: {
      ecmaVersion: 'latest',
      sourceType: 'module',
      globals: { ...globals.browser }
    },
    rules: {
      'no-alert': 'warn'
    }
  }
]
