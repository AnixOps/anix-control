import { nextTick, watch } from 'vue'

const TRANSLATABLE_ATTRIBUTES = ['placeholder', 'title', 'aria-label']
const ATTRIBUTE_SOURCE_MAP = new WeakMap()
const TEXT_SOURCE_MAP = new WeakMap()

function getLegacyMap(i18n) {
  return i18n.global.getLocaleMessage(i18n.global.locale.value)?.legacy || {}
}

function translateLiteral(i18n, value) {
  if (typeof value !== 'string') {
    return value
  }
  const trimmed = value.trim()
  if (!trimmed) {
    return value
  }
  const translated = getLegacyMap(i18n)[trimmed]
  if (!translated) {
    return value
  }
  const leading = value.match(/^\s*/)?.[0] || ''
  const trailing = value.match(/\s*$/)?.[0] || ''
  return `${leading}${translated}${trailing}`
}

function shouldSkipTextNode(node) {
  const text = node?.nodeValue
  if (typeof text !== 'string' || !text.trim()) {
    return true
  }
  const parent = node.parentElement
  if (!parent) {
    return true
  }
  return ['SCRIPT', 'STYLE', 'TEXTAREA', 'PRE', 'CODE'].includes(parent.tagName)
}

function translateTextNode(i18n, node) {
  if (shouldSkipTextNode(node)) {
    return
  }
  const source = TEXT_SOURCE_MAP.get(node) ?? node.nodeValue
  TEXT_SOURCE_MAP.set(node, source)
  const translated = translateLiteral(i18n, source)
  if (translated !== node.nodeValue) {
    node.nodeValue = translated
  }
}

function translateAttributes(i18n, root) {
  if (!(root instanceof Element)) {
    return
  }
  const selector = TRANSLATABLE_ATTRIBUTES.map((attr) => `[${attr}]`).join(',')
  const elements = [root, ...root.querySelectorAll(selector)]

  for (const element of elements) {
    const sourceMap = ATTRIBUTE_SOURCE_MAP.get(element) || {}
    for (const attr of TRANSLATABLE_ATTRIBUTES) {
      if (!element.hasAttribute(attr)) {
        continue
      }
      if (!(attr in sourceMap)) {
        sourceMap[attr] = element.getAttribute(attr)
      }
      const source = sourceMap[attr]
      const translated = translateLiteral(i18n, source)
      if (translated !== element.getAttribute(attr)) {
        element.setAttribute(attr, translated)
      }
    }
    ATTRIBUTE_SOURCE_MAP.set(element, sourceMap)
  }
}

function translateTree(i18n, root) {
  if (!root || typeof document === 'undefined') {
    return
  }
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT)
  let current = walker.nextNode()
  while (current) {
    translateTextNode(i18n, current)
    current = walker.nextNode()
  }
  translateAttributes(i18n, root)
}

export function mountLegacyI18n(i18n, root) {
  if (!root || typeof MutationObserver === 'undefined') {
    return () => {}
  }

  const originalAlert = typeof window !== 'undefined' ? window.alert?.bind(window) : null
  const originalConfirm = typeof window !== 'undefined' ? window.confirm?.bind(window) : null
  const originalPrompt = typeof window !== 'undefined' ? window.prompt?.bind(window) : null

  if (typeof window !== 'undefined') {
    window.alert = (message) => originalAlert?.(translateLiteral(i18n, String(message)))
    window.confirm = (message) => originalConfirm?.(translateLiteral(i18n, String(message)))
    window.prompt = (message, defaultValue) => originalPrompt?.(translateLiteral(i18n, String(message)), defaultValue)
  }

  let scheduled = false
  const scheduleTranslate = () => {
    if (scheduled) {
      return
    }
    scheduled = true
    queueMicrotask(() => {
      scheduled = false
      translateTree(i18n, root)
    })
  }

  const observer = new MutationObserver(scheduleTranslate)
  observer.observe(root, {
    subtree: true,
    childList: true,
    characterData: true
  })

  const stopWatch = watch(
    () => i18n.global.locale.value,
    async () => {
      await nextTick()
      translateTree(i18n, root)
    }
  )

  nextTick(() => translateTree(i18n, root))

  return () => {
    observer.disconnect()
    stopWatch()
    if (typeof window !== 'undefined') {
      if (originalAlert) {
        window.alert = originalAlert
      }
      if (originalConfirm) {
        window.confirm = originalConfirm
      }
      if (originalPrompt) {
        window.prompt = originalPrompt
      }
    }
  }
}
