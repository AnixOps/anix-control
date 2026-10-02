import { afterEach, describe, expect, it } from 'vitest'
import { ref } from 'vue'
import { mountLegacyI18n } from '@/utils/legacyI18n'

// A stand-in for the vue-i18n instance: the legacy map of the current locale.
function fakeI18n(locale = 'en') {
  const messages = { en: { legacy: { '刷新': 'Refresh', '保存': 'Save' } }, 'zh-CN': { legacy: {} } }
  const current = ref(locale)
  return { global: { locale: current, getLocaleMessage: name => messages[name] } }
}

function settle() {
  return new Promise(resolve => setTimeout(resolve, 0))
}

let stop = () => {}
afterEach(() => {
  stop()
  document.body.innerHTML = ''
})

describe('legacy literal translation', () => {
  it('translates hard-coded literals and follows the locale', async () => {
    const i18n = fakeI18n()
    document.body.innerHTML = '<div id="root"><button title="保存">刷新</button></div>'
    const root = document.getElementById('root')
    stop = mountLegacyI18n(i18n, root)
    await settle()
    expect(root.querySelector('button').textContent).toBe('Refresh')
    expect(root.querySelector('button').getAttribute('title')).toBe('Save')
    i18n.global.locale.value = 'zh-CN'
    await settle()
    await settle()
    expect(root.querySelector('button').textContent).toBe('刷新')
  })

  // Vue updates text nodes in place; the translator must not write the first
  // value it saw back over the update (a "复制" button that turns into
  // "已复制", a heading that follows a mode).
  it('keeps text and attributes the app changes after the first pass', async () => {
    const i18n = fakeI18n()
    document.body.innerHTML = '<div id="root"><p>Sign in</p><button aria-label="Copy">刷新</button></div>'
    const root = document.getElementById('root')
    stop = mountLegacyI18n(i18n, root)
    await settle()
    const text = root.querySelector('p').firstChild
    text.nodeValue = 'Create your account'
    root.querySelector('button').setAttribute('aria-label', 'Copied')
    await settle()
    await settle()
    expect(root.querySelector('p').textContent).toBe('Create your account')
    expect(root.querySelector('button').getAttribute('aria-label')).toBe('Copied')
    // A new literal written later is still translated.
    root.querySelector('button').firstChild.nodeValue = '保存'
    await settle()
    await settle()
    expect(root.querySelector('button').textContent).toBe('Save')
  })
})
