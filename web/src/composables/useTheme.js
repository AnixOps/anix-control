import { ref } from 'vue'

const STORAGE_KEY = 'v2board-theme'
const THEME_LIGHT = 'light'
const THEME_DARK = 'dark'

// 全局单例状态，跨组件共享当前主题。
const currentTheme = ref(THEME_LIGHT)

// applyTheme 同步主题到 DOM：Arco 用 body[arco-theme]，
// 自建样式用 html[data-theme]，两者一起切换以保持一致。
function applyTheme(theme) {
  const isDark = theme === THEME_DARK
  if (isDark) {
    document.body.setAttribute('arco-theme', 'dark')
    document.documentElement.setAttribute('data-theme', 'dark')
  } else {
    document.body.removeAttribute('arco-theme')
    document.documentElement.removeAttribute('data-theme')
  }
  document.documentElement.style.colorScheme = isDark ? 'dark' : 'light'
  currentTheme.value = theme
}

// resolveInitialTheme 优先级：用户存储 > 系统偏好 > 亮色。
function resolveInitialTheme() {
  const stored = localStorage.getItem(STORAGE_KEY)
  if (stored === THEME_DARK || stored === THEME_LIGHT) {
    return stored
  }
  if (window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches) {
    return THEME_DARK
  }
  return THEME_LIGHT
}

// initTheme 在应用启动时调用，应用初始主题。
export function initTheme() {
  applyTheme(resolveInitialTheme())
}

export function useTheme() {
  function setTheme(theme) {
    applyTheme(theme)
    localStorage.setItem(STORAGE_KEY, theme)
  }

  function toggleTheme() {
    setTheme(currentTheme.value === THEME_DARK ? THEME_LIGHT : THEME_DARK)
  }

  function isDark() {
    return currentTheme.value === THEME_DARK
  }

  return { currentTheme, setTheme, toggleTheme, isDark }
}
