<template>
  <div class="admin-layout" :class="{ 'navigation-collapsed': sidebarCollapsed }">
    <div class="sidebar-overlay" :class="{ active: sidebarOpen }" aria-hidden="true" @click="closeSidebar"></div>

    <aside
      id="admin-sidebar"
      class="sidebar"
      :class="{ open: sidebarOpen }"
      :inert="drawerInactive"
      :aria-hidden="drawerInactive ? 'true' : undefined"
      :aria-label="t('layout.admin.mobileTitle')"
    >
      <div class="sidebar-top">
        <div class="brand-block">
          <div class="brand-mark" aria-hidden="true"><PanelsTopLeft :size="20" /></div>
          <div class="brand-copy">
            <div class="brand-name">{{ t('layout.admin.brand') }}</div>
            <div class="brand-meta">{{ t('layout.admin.badge') }}</div>
          </div>
        </div>
        <button
          class="btn-ghost close-button"
          ref="closeButton"
          type="button"
          :aria-label="t('common.a11y.closeNavigation')"
          :title="t('common.a11y.closeNavigation')"
          @click="closeSidebar"
        >
          <X :size="18" aria-hidden="true" />
        </button>
      </div>

      <AdminNavigation
        class="sidebar-nav"
        :sections="navSections"
        :collapsed="sidebarCollapsed"
        :mobile="isTabletViewport"
        @navigate="closeSidebar"
        @toggle-collapse="toggleSidebarCollapse"
      />

      <div class="sidebar-footer">
        <div class="operator-card">
          <div class="operator-avatar" aria-hidden="true"><Users :size="18" /></div>
          <div class="operator-copy">
            <div class="operator-name">{{ t('layout.admin.adminUser') }}</div>
            <div class="operator-email">{{ userStore.userInfo?.email || '-' }}</div>
          </div>
        </div>
        <div v-if="systemVersionDisplay" class="version-line" :title="systemVersionTitle">
          {{ systemVersionDisplay }}
        </div>
        <div class="sidebar-actions">
          <LocaleSwitcher compact />
          <button class="btn logout-button" type="button" :title="t('common.actions.logout')" @click="logout">
            <LogOut :size="17" aria-hidden="true" />
            <span class="logout-label">{{ t('common.actions.logout') }}</span>
          </button>
        </div>
      </div>
    </aside>

    <div class="workspace">
      <header class="topbar">
        <div class="topbar-primary">
          <button
            class="btn btn-ghost menu-button"
            ref="menuButton"
            type="button"
            :aria-label="t('common.a11y.openNavigation')"
            :title="t('common.a11y.openNavigation')"
            aria-controls="admin-sidebar"
            :aria-expanded="sidebarOpen ? 'true' : 'false'"
            @click="toggleSidebar"
          >
            <Menu :size="20" aria-hidden="true" />
          </button>
          <div>
            <div class="topbar-title">{{ pageTitle }}</div>
            <div class="topbar-subtitle">{{ t('layout.admin.subtitle') }}</div>
          </div>
        </div>
        <div class="topbar-actions">
          <span class="current-time">{{ currentTime }}</span>
          <ThemeToggle compact />
          <LocaleSwitcher />
        </div>
      </header>

      <main id="app-main-content" class="content" tabindex="-1" :aria-label="pageTitle">
        <router-view></router-view>
      </main>
    </div>
  </div>
</template>

<script setup>
import { computed, nextTick, onMounted, onUnmounted, ref, watchEffect } from 'vue'
import { LogOut, Menu, PanelsTopLeft, Users, X } from '@lucide/vue'
import { useRoute, useRouter } from 'vue-router'
import { useUserStore } from '@/stores/user'
import { useAppI18n } from '@/composables/useAppI18n'
import AdminNavigation from '@/components/admin/AdminNavigation.vue'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import ThemeToggle from '@/components/common/ThemeToggle.vue'
import { resolveRoutePageTitle } from '@/utils/pageMeta'
import { getSystemInfo } from '@/api/admin'
import { adminExtensionMenus } from '@/extensions/runtime'
import {
  WEBUI_MENU_FALLBACK_PARENT,
  WEBUI_MENU_PARENT_REGISTRY,
  normalizeWebUIMenuParent
} from '@/extensions/menuRegistry'

const DESKTOP_SIDEBAR_KEY = 'admin.sidebar.collapsed'
const TABLET_BREAKPOINT = '(max-width: 1024px)'

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()
const { t, currentLocale, formatDateTime } = useAppI18n()

const sidebarOpen = ref(false)
const sidebarCollapsed = ref(readSidebarCollapsePreference())
const isTabletViewport = ref(readTabletViewport())
const menuButton = ref(null)
const closeButton = ref(null)
const currentTime = ref('')
const systemVersion = ref('')
const systemBuildCode = ref(import.meta.env.VITE_APP_BUILD_CODE || '')
const systemBuildTime = ref('')
const systemCommit = ref('')
const frontendBuildCode = import.meta.env.VITE_APP_BUILD_CODE || ''
const frontendBuildTime = import.meta.env.VITE_APP_BUILD_TIME || ''

const drawerInactive = computed(() => isTabletViewport.value && !sidebarOpen.value)

const extensionSectionTitleKeys = {
  services: 'layout.admin.sections.extensionServices',
  operations: 'layout.admin.sections.extensionOperations',
  system: 'layout.admin.sections.extensionSystem',
  [WEBUI_MENU_FALLBACK_PARENT]: 'layout.admin.sections.extensions'
}

function readSidebarCollapsePreference() {
  try {
    return localStorage.getItem(DESKTOP_SIDEBAR_KEY) === 'true'
  } catch {
    return false
  }
}

function readTabletViewport() {
  if (typeof window === 'undefined') {
    return false
  }
  if (typeof window.matchMedia === 'function') {
    return window.matchMedia(TABLET_BREAKPOINT).matches
  }
  return window.innerWidth <= 1024
}

function compareExtensionMenus(left, right) {
  const leftOrder = Number.isSafeInteger(left.order) ? left.order : 1000
  const rightOrder = Number.isSafeInteger(right.order) ? right.order : 1000
  return leftOrder - rightOrder ||
    String(left.label || '').localeCompare(String(right.label || '')) ||
    String(left.id || '').localeCompare(String(right.id || ''))
}

const navSections = computed(() => {
  const extensionMenusByParent = new Map(WEBUI_MENU_PARENT_REGISTRY.map(parent => [parent, []]))
  for (const item of adminExtensionMenus.value.filter(menu => userStore.hasPermission(menu.permission))) {
    const parent = normalizeWebUIMenuParent(item.parent)
    extensionMenusByParent.get(parent).push({ ...item, parent })
  }

  const extensionGroups = WEBUI_MENU_PARENT_REGISTRY.flatMap(parent => {
    const items = extensionMenusByParent.get(parent).sort(compareExtensionMenus)
    return items.length > 0
      ? [{ parent, label: t(extensionSectionTitleKeys[parent]), items }]
      : []
  })

  return [
    {
      id: 'overview',
      label: t('layout.admin.sections.overview'),
      items: [
        { to: '/admin/dashboard', icon: 'dashboard', label: t('layout.admin.nav.dashboard') },
        { to: '/admin/monitor', icon: 'monitor', label: t('layout.admin.nav.monitor') },
        { to: '/admin/traffic-hourly', icon: 'traffic', label: t('layout.admin.nav.trafficHourly') }
      ]
    },
    {
      id: 'business',
      label: t('layout.admin.sections.business'),
      items: [
        { to: '/admin/users', icon: 'users', label: t('layout.admin.nav.users') },
        { to: '/admin/orders', icon: 'orders', label: t('layout.admin.nav.orders') },
        { to: '/admin/tickets', icon: 'tickets', label: t('layout.admin.nav.tickets') }
      ],
      advancedItems: [
        { to: '/admin/subscriptions', icon: 'subscriptions', label: t('layout.admin.nav.subscriptions') },
        { to: '/admin/plans', icon: 'plans', label: t('layout.admin.nav.plans') },
        { to: '/admin/coupons', icon: 'coupons', label: t('layout.admin.nav.coupons') },
        { to: '/admin/invite', icon: 'invite', label: t('layout.admin.nav.invite') },
        { to: '/admin/payment', icon: 'payment', label: t('layout.admin.nav.payment') },
        { to: '/admin/knowledge', icon: 'knowledge', label: t('layout.admin.nav.knowledge') }
      ]
    },
    {
      id: 'network',
      label: t('layout.admin.sections.network'),
      kind: 'forward',
      forwardLabel: t('layout.admin.sections.forwardSuite'),
      items: [
        { to: '/admin/nodes', icon: 'nodes', label: t('layout.admin.nav.nodes') }
      ],
      advancedItems: [
        { to: '/admin/agent', icon: 'agents', label: t('layout.admin.nav.nodeXAgentsLegacy') }
      ]
    },
    {
      id: 'control-center',
      label: t('layout.admin.sections.controlCenter'),
      items: [
        { to: '/admin/plugins', icon: 'plugins', label: t('layout.admin.nav.plugins') },
        { to: '/admin/deployments', icon: 'deployments', label: t('layout.admin.nav.deployments') }
      ],
      advancedItems: [
        { to: '/admin/control', icon: 'control', label: t('layout.admin.nav.control') }
      ],
      extensionGroups
    },
    {
      id: 'system',
      label: t('layout.admin.sections.system'),
      items: [
        { to: '/admin/mfa', icon: 'mfa', label: t('layout.admin.nav.mfa') },
        { to: '/admin/access-groups', icon: 'access-groups', label: t('layout.admin.nav.accessGroups') },
        { to: '/admin/system', icon: 'system', label: t('layout.admin.nav.system') }
      ],
      advancedItems: [
        { to: '/admin/telegram', icon: 'system', label: t('layout.admin.nav.telegram') },
        { to: '/admin/notifications', icon: 'system', label: t('layout.admin.nav.notifications') }
      ]
    }
  ]
})

const pageTitle = computed(() => {
  const extensionMenu = adminExtensionMenus.value.find(item => item.to === route.path && userStore.hasPermission(item.permission))
  return extensionMenu?.label || resolveRoutePageTitle(t, route.path, t('pageTitles.admin.fallback'))
})

const systemVersionDisplay = computed(() => {
  if (!systemVersion.value) {
    return ''
  }
  if (systemVersion.value.includes('#')) {
    return `AnixOps v${systemVersion.value}`
  }
  return systemBuildCode.value
    ? `AnixOps v${systemVersion.value} #${systemBuildCode.value}`
    : `AnixOps v${systemVersion.value}`
})

const systemVersionTitle = computed(() => {
  const rows = []
  if (systemBuildCode.value) {
    rows.push(`Build code: ${systemBuildCode.value}`)
  }
  if (systemBuildTime.value) {
    rows.push(`Backend build: ${systemBuildTime.value}`)
  }
  if (systemCommit.value && systemCommit.value !== 'unknown') {
    rows.push(`Commit: ${systemCommit.value}`)
  }
  if (frontendBuildCode) {
    rows.push(`Frontend build: ${frontendBuildCode}`)
  }
  if (frontendBuildTime) {
    rows.push(`Frontend time: ${frontendBuildTime}`)
  }
  return rows.join('\n')
})

function closeSidebar() {
  const restoreFocus = isTabletViewport.value && sidebarOpen.value
  sidebarOpen.value = false
  if (restoreFocus) {
    nextTick(() => menuButton.value?.focus?.())
  }
}

function openSidebar() {
  sidebarOpen.value = true
  if (isTabletViewport.value) {
    nextTick(() => closeButton.value?.focus?.())
  }
}

function toggleSidebar() {
  if (sidebarOpen.value) {
    closeSidebar()
  } else {
    openSidebar()
  }
}

function toggleSidebarCollapse() {
  sidebarCollapsed.value = !sidebarCollapsed.value
  try {
    localStorage.setItem(DESKTOP_SIDEBAR_KEY, String(sidebarCollapsed.value))
  } catch {
    // A blocked storage implementation should not prevent navigation from working.
  }
}

function logout() {
  userStore.logout()
  router.push('/login')
}

function updateTime() {
  currentTime.value = formatDateTime(new Date(), {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false
  })
}

function updateTabletViewport(event) {
  if (typeof event?.matches === 'boolean') {
    isTabletViewport.value = event.matches
    return
  }
  isTabletViewport.value = readTabletViewport()
}

async function loadSystemInfo() {
  try {
    const res = await getSystemInfo()
    const info = readSystemInfo(res)
    systemVersion.value = info.version || ''
    systemBuildCode.value = info.build_code || frontendBuildCode
    systemBuildTime.value = info.build_time || ''
    systemCommit.value = info.commit || ''
  } catch {
    // Ignore layout metadata failures so a transient version lookup cannot block routes.
  }
}

function readSystemInfo(res) {
  if (!res || typeof res !== 'object') {
    return {}
  }
  const payload = Object.prototype.hasOwnProperty.call(res, 'code') ? res.data : (res.data ?? res)
  return payload && typeof payload === 'object' ? payload : {}
}

let timer
let tabletMediaQuery
let useWindowResizeListener = false

onMounted(() => {
  updateTime()
  timer = setInterval(updateTime, 60000)
  if (typeof window !== 'undefined' && window.matchMedia) {
    tabletMediaQuery = window.matchMedia(TABLET_BREAKPOINT)
    updateTabletViewport(tabletMediaQuery)
    if (typeof tabletMediaQuery.addEventListener === 'function') {
      tabletMediaQuery.addEventListener('change', updateTabletViewport)
    } else if (typeof tabletMediaQuery.addListener === 'function') {
      tabletMediaQuery.addListener(updateTabletViewport)
    }
  } else if (typeof window !== 'undefined') {
    useWindowResizeListener = true
    window.addEventListener('resize', updateTabletViewport)
  }
  if (userStore.isLoggedIn) {
    userStore.getUserInfo()
  }
  loadSystemInfo()
})

onUnmounted(() => {
  clearInterval(timer)
  if (typeof tabletMediaQuery?.removeEventListener === 'function') {
    tabletMediaQuery.removeEventListener('change', updateTabletViewport)
  } else if (typeof tabletMediaQuery?.removeListener === 'function') {
    tabletMediaQuery.removeListener(updateTabletViewport)
  }
  if (useWindowResizeListener && typeof window !== 'undefined') {
    window.removeEventListener('resize', updateTabletViewport)
  }
})

watchEffect(() => {
  currentLocale.value
  updateTime()
})
</script>

<style scoped>
.admin-layout {
  min-height: 100vh;
  display: flex;
  background: var(--bg-color);
}

.sidebar-overlay {
  position: fixed;
  inset: 0;
  z-index: 1000;
  background: rgba(15, 23, 42, 0.42);
  opacity: 0;
  pointer-events: none;
  transition: opacity 0.2s ease;
}

.sidebar-overlay.active {
  opacity: 1;
  pointer-events: auto;
}

.sidebar {
  position: sticky;
  top: 0;
  display: flex;
  width: var(--sidebar-width);
  height: 100vh;
  flex: 0 0 var(--sidebar-width);
  flex-direction: column;
  gap: 16px;
  padding: 16px;
  overflow: hidden;
  border-right: 1px solid var(--admin-sidebar-divider);
  background: var(--admin-sidebar-surface);
  color: var(--admin-sidebar-text);
  transition: width 0.2s ease, flex-basis 0.2s ease, padding 0.2s ease;
}

.navigation-collapsed .sidebar {
  width: var(--sidebar-collapsed-width);
  flex-basis: var(--sidebar-collapsed-width);
  padding: 16px 12px;
}

.sidebar-top,
.brand-block,
.operator-card,
.topbar,
.topbar-primary,
.topbar-actions {
  display: flex;
  align-items: center;
}

.sidebar-top {
  justify-content: space-between;
  gap: 8px;
}

.brand-block {
  min-width: 0;
  gap: 10px;
}

.brand-mark,
.operator-avatar {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  border: 1px solid var(--admin-sidebar-divider);
  border-radius: var(--radius-md);
  background: var(--admin-sidebar-accent);
  color: var(--admin-sidebar-text-strong);
}

.brand-mark {
  width: 38px;
  height: 38px;
}

.brand-copy,
.operator-copy {
  min-width: 0;
}

.brand-name,
.operator-name {
  overflow: hidden;
  color: var(--admin-sidebar-text-strong);
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.brand-name {
  font-size: 15px;
}

.brand-meta,
.operator-email,
.version-line {
  color: var(--admin-sidebar-muted);
  font-size: 12px;
}

.close-button {
  display: none;
  width: 40px;
  min-height: 40px;
  padding: 0;
  border-radius: var(--radius-md);
  color: var(--admin-sidebar-text);
}

.sidebar-nav {
  min-height: 0;
}

.sidebar-footer {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding-top: 14px;
  border-top: 1px solid var(--admin-sidebar-divider);
}

.operator-card {
  gap: 10px;
}

.operator-avatar {
  width: 34px;
  height: 34px;
}

.operator-email {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.version-line {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sidebar-actions {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.logout-button {
  justify-content: flex-start;
  width: 100%;
}

.workspace {
  display: flex;
  min-width: 0;
  flex: 1;
  flex-direction: column;
}

.topbar {
  justify-content: space-between;
  gap: 16px;
  min-height: var(--header-height);
  padding: 14px 24px;
  border-bottom: 1px solid var(--border-color);
  background: var(--surface-color);
  box-shadow: var(--shadow-sm);
}

.topbar-primary {
  min-width: 0;
  gap: 12px;
}

.menu-button {
  display: none;
  width: 40px;
  min-height: 40px;
  padding: 0;
  border-radius: var(--radius-md);
}

.topbar-title {
  overflow: hidden;
  color: var(--text-color);
  font-size: 20px;
  font-weight: 700;
  line-height: 1.2;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.topbar-subtitle {
  max-width: 760px;
  margin-top: 3px;
  color: var(--text-secondary);
  font-size: 13px;
}

.topbar-actions {
  flex: 0 0 auto;
  gap: 10px;
}

.current-time {
  color: var(--text-secondary);
  font-size: 13px;
  white-space: nowrap;
}

.content {
  flex: 1;
  padding: 24px;
}

.sidebar :deep(.locale-switcher),
.topbar :deep(.locale-switcher),
.topbar :deep(.theme-toggle) {
  border-radius: var(--radius-md);
  background: var(--surface-muted);
}

.sidebar :deep(.locale-option),
.topbar :deep(.locale-option) {
  border-radius: var(--radius-sm);
}

.navigation-collapsed .brand-copy,
.navigation-collapsed .operator-copy,
.navigation-collapsed .version-line,
.navigation-collapsed .sidebar-actions :deep(.locale-switcher),
.navigation-collapsed .logout-label {
  display: none;
}

.navigation-collapsed .sidebar-top,
.navigation-collapsed .sidebar-footer,
.navigation-collapsed .operator-card,
.navigation-collapsed .sidebar-actions {
  align-items: center;
}

.navigation-collapsed .logout-button {
  justify-content: center;
  width: 40px;
  min-height: 40px;
  padding: 0;
}

@media (max-width: 1024px) {
  .sidebar,
  .navigation-collapsed .sidebar {
    position: fixed;
    inset: 0 auto 0 0;
    z-index: 1010;
    width: min(88vw, var(--sidebar-width));
    height: 100dvh;
    flex-basis: min(88vw, var(--sidebar-width));
    padding: 16px;
    transform: translateX(-100%);
    transition: transform 0.2s ease;
  }

  .sidebar.open {
    transform: translateX(0);
  }

  .menu-button,
  .close-button {
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }

  .navigation-collapsed .brand-copy,
  .navigation-collapsed .operator-copy,
  .navigation-collapsed .version-line,
  .navigation-collapsed .logout-label {
    display: block;
  }

  .navigation-collapsed .sidebar-actions :deep(.locale-switcher) {
    display: inline-flex;
  }

  .navigation-collapsed .sidebar-top,
  .navigation-collapsed .sidebar-footer,
  .navigation-collapsed .operator-card,
  .navigation-collapsed .sidebar-actions {
    align-items: stretch;
  }

  .navigation-collapsed .logout-button {
    justify-content: flex-start;
    width: 100%;
    padding: 10px 16px;
  }
}

@media (max-width: 768px) {
  .topbar {
    align-items: flex-start;
    flex-direction: column;
    padding: 14px 18px;
  }

  .topbar-actions {
    flex-wrap: wrap;
  }

  .current-time {
    display: none;
  }

  .topbar-subtitle {
    display: none;
  }

  .content {
    padding: 18px;
  }
}
</style>
