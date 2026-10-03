// Histoire preview setup: the same CSS stack as the app (main.js), vue-i18n
// with both locales, and the theme. Histoire marks dark mode with
// <html class="dark">; the tokens follow <html data-theme>, so mirror it.
// Locale: zh-CN by default, ?lang=en on the preview URL for English.
import { defineSetupVue3 } from '@histoire/plugin-vue'
import '../design/fonts/inter/inter.css'
import '../design/tokens.css'
import '../styles/base.css'
import './stories/story.css'
import i18n, { loadMessageGroup, setLocale } from '../i18n'

function syncTheme() {
  const root = document.documentElement
  root.setAttribute('data-theme', root.classList.contains('dark') ? 'dark' : 'light')
}

if (typeof document !== 'undefined') {
  syncTheme()
  new MutationObserver(syncTheme).observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
}

export const setupVue3 = defineSetupVue3(async ({ app }) => {
  app.use(i18n)
  const params = typeof location !== 'undefined' ? new URLSearchParams(location.search) : null
  const lang = params?.get('lang') === 'en' ? 'en' : 'zh-CN'
  await loadMessageGroup('admin')
  await loadMessageGroup('adminPages')
  await setLocale(lang)
})
