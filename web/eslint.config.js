// Minimal ESLint setup for the redesign rules (docs/reference/frontend-design.md).
// Since U4 every alert, confirm and prompt is a toast, a ConfirmDialog or a
// UiDialog, and every overlay is UiDialog or UiSheet: no-alert and the
// legacy modal classes are errors (src/__tests__/nativeDialogs.test.js
// checks the same in CI).
import pluginVue from 'eslint-plugin-vue'
import globals from 'globals'
import { DATA_TABLE_PAGES } from './scripts/data-table-pages.mjs'

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
      'no-alert': 'error',
      'vue/no-restricted-class': ['error', 'modal-overlay', 'modal', 'modal-lg', 'modal-header', 'modal-body', 'modal-footer']
    }
  },
  {
    // Migrated list pages use UiDataTable (UI U6).
    files: DATA_TABLE_PAGES,
    rules: {
      'vue/no-restricted-html-elements': ['error', { element: 'table', message: 'Use UiDataTable (frontend-design.md, "List pages").' }]
    }
  }
]
