import { ref } from 'vue'

const STORAGE_KEY = 'v2board-theme'
const THEME_LIGHT = 'light'
const THEME_DARK = 'dark'
const DARK_QUERY = '(prefers-color-scheme: dark)'

// 全局单例状态，跨组件共享当前（已解析的）主题。
const currentTheme = ref(THEME_LIGHT)

let systemQuery = null

function readStoredTheme() {
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    return stored === THEME_DARK || stored === THEME_LIGHT ? stored : ''
  } catch {
    return ''
  }
}

function systemTheme() {
  if (typeof window !== 'undefined' && typeof window.matchMedia === 'function' && window.matchMedia(DARK_QUERY).matches) {
    return THEME_DARK
  }
  return THEME_LIGHT
}

// The browser chrome colour follows the page background of the active theme.
function syncThemeColor() {
  if (typeof document === 'undefined' || typeof getComputedStyle !== 'function') return
  const background = getComputedStyle(document.documentElement).getPropertyValue('--bg').trim()
  if (!background) return
  for (const meta of document.head.querySelectorAll('meta[name="theme-color"]')) {
    meta.setAttribute('content', background)
  }
}

// applyTheme 把主题写到 html[data-theme]。设计变量（src/design/tokens.css）在没有
// data-theme 时跟随系统，"light" / "dark" 则强制覆盖；旧页面里的
// [data-theme='dark'] 选择器也依赖这个属性，所以两种主题都显式写入。
function applyTheme(theme) {
  const resolved = theme === THEME_DARK ? THEME_DARK : THEME_LIGHT
  document.documentElement.setAttribute('data-theme', resolved)
  currentTheme.value = resolved
  syncThemeColor()
}

function handleSystemChange() {
  if (!readStoredTheme()) {
    applyTheme(systemTheme())
  }
}

// initTheme 在应用启动时调用。优先级：用户选择 > 系统偏好 > 亮色；
// 没有用户选择时继续跟随系统切换。
export function initTheme() {
  applyTheme(readStoredTheme() || systemTheme())
  if (!systemQuery && typeof window !== 'undefined' && typeof window.matchMedia === 'function') {
    systemQuery = window.matchMedia(DARK_QUERY)
    if (typeof systemQuery.addEventListener === 'function') {
      systemQuery.addEventListener('change', handleSystemChange)
    } else if (typeof systemQuery.addListener === 'function') {
      systemQuery.addListener(handleSystemChange)
    }
  }
}

export function useTheme() {
  function setTheme(theme) {
    applyTheme(theme)
    try {
      localStorage.setItem(STORAGE_KEY, currentTheme.value)
    } catch {
      // Blocked storage only loses the preference; the theme still applies.
    }
  }

  function toggleTheme() {
    setTheme(currentTheme.value === THEME_DARK ? THEME_LIGHT : THEME_DARK)
  }

  function isDark() {
    return currentTheme.value === THEME_DARK
  }

  return { currentTheme, setTheme, toggleTheme, isDark }
}
