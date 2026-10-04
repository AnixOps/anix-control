// Messages for the admin shell and the dashboard: navigation, ⌘K, the
// settings save bar and the dashboard page. src/i18n.js loads them when
// an admin signs in or an admin route opens; the other admin pages read
// src/locales/zh-CN.adminPages.js, loaded before any of them opens.
// The rest of the app reads src/locales/zh-CN.js only.
import adminDashboard from './modules/zh-CN/adminDashboard'
import adminMonitor from './modules/zh-CN/adminMonitor'
import adminSettingsPages from './modules/zh-CN/adminSettingsPages'

export default {
  ...adminSettingsPages,
  ...adminDashboard,
  ...adminMonitor
}
