<template>
  <div class="admin-layout">
    <div class="sidebar-overlay" :class="{ active: sidebarOpen }" aria-hidden="true" @click="sidebarOpen = false"></div>

    <aside id="admin-sidebar" class="sidebar" :class="{ open: sidebarOpen }" :aria-label="t('layout.admin.mobileTitle')">
      <div class="sidebar-top">
        <div class="brand-block">
          <div class="brand-mark">AO</div>
          <div>
            <div class="brand-name">{{ t('layout.admin.brand') }}</div>
            <div class="brand-meta">{{ t('layout.admin.badge') }}</div>
          </div>
        </div>
        <button
          class="btn-ghost btn-sm close-button md:hidden"
          type="button"
          :aria-label="t('common.a11y.closeNavigation')"
          :title="t('common.a11y.closeNavigation')"
          @click="sidebarOpen = false"
        >
          x
        </button>
      </div>

      <nav class="sidebar-nav" :aria-label="t('layout.admin.mobileTitle')">
        <section
          v-for="section in navSections"
          :key="section.id || section.title"
          class="nav-section"
          :data-extension-parent="section.kind === 'extension' ? section.parent : undefined"
        >
          <div class="nav-section-title">{{ section.title }}</div>
          <template v-if="section.kind === 'forward'">
            <ForwardSuiteNav />
          </template>
          <template v-else>
            <router-link v-for="item in section.items" :key="item.to" :to="item.to" class="nav-link" @click="closeSidebar">
              <span class="nav-link-icon">{{ item.icon }}</span>
              <span class="nav-link-label">{{ item.label }}</span>
            </router-link>
          </template>
        </section>
        <router-link to="/admin/agent" class="legacy-hidden-link" aria-hidden="true" tabindex="-1">{{ t('layout.admin.nav.nodeXAgentsLegacy') }}</router-link>
      </nav>

      <div class="sidebar-footer">
        <div class="operator-card">
          <div class="operator-avatar">A</div>
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
          <button class="btn w-full" type="button" @click="logout">{{ t('common.actions.logout') }}</button>
        </div>
      </div>
    </aside>

    <div class="workspace">
      <header class="topbar">
        <div class="topbar-primary">
          <button
            class="btn btn-ghost btn-sm menu-button"
            type="button"
            :aria-label="t('common.a11y.openNavigation')"
            :title="t('common.a11y.openNavigation')"
            aria-controls="admin-sidebar"
            :aria-expanded="sidebarOpen ? 'true' : 'false'"
            @click="sidebarOpen = !sidebarOpen"
          >
            ☰
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
import { computed, onMounted, onUnmounted, ref, watchEffect } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useUserStore } from '@/stores/user'
import { useAppI18n } from '@/composables/useAppI18n'
import ForwardSuiteNav from '@/components/admin/ForwardSuiteNav.vue'
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

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()
const { t, currentLocale, formatDateTime } = useAppI18n()

const sidebarOpen = ref(false)
const currentTime = ref('')
const systemVersion = ref('')
const systemBuildCode = ref(import.meta.env.VITE_APP_BUILD_CODE || '')
const systemBuildTime = ref('')
const systemCommit = ref('')
const frontendBuildCode = import.meta.env.VITE_APP_BUILD_CODE || ''
const frontendBuildTime = import.meta.env.VITE_APP_BUILD_TIME || ''

const extensionSectionTitleKeys = {
  services: 'layout.admin.sections.extensionServices',
  operations: 'layout.admin.sections.extensionOperations',
  system: 'layout.admin.sections.extensionSystem',
  [WEBUI_MENU_FALLBACK_PARENT]: 'layout.admin.sections.extensions'
}

function compareExtensionMenus(left, right) {
  const leftOrder = Number.isSafeInteger(left.order) ? left.order : 1000
  const rightOrder = Number.isSafeInteger(right.order) ? right.order : 1000
  return leftOrder - rightOrder ||
    String(left.label || '').localeCompare(String(right.label || '')) ||
    String(left.id || '').localeCompare(String(right.id || ''))
}

const navSections = computed(() => {
  const sections = [
  {
    title: t('layout.admin.sections.overview'),
    items: [
      { to: '/admin/dashboard', icon: 'DB', label: t('layout.admin.nav.dashboard') },
      { to: '/admin/monitor', icon: 'MT', label: t('layout.admin.nav.monitor') },
      { to: '/admin/traffic-hourly', icon: 'TH', label: t('layout.admin.nav.trafficHourly') }
    ]
  },
  {
    title: t('layout.admin.sections.forwardSuite'),
    kind: 'forward'
  },
  {
    title: t('layout.admin.sections.userManagement'),
    items: [
      { to: '/admin/users', icon: 'US', label: t('layout.admin.nav.users') },
      { to: '/admin/orders', icon: 'OR', label: t('layout.admin.nav.orders') },
      { to: '/admin/tickets', icon: 'TK', label: t('layout.admin.nav.tickets') }
    ]
  },
  {
    title: t('layout.admin.sections.nodeManagement'),
    items: [
      { to: '/admin/nodes', icon: 'ND', label: t('layout.admin.nav.nodes') },
      { to: '/admin/subscriptions', icon: 'SB', label: t('layout.admin.nav.subscriptions') }
    ]
  },
  {
    title: t('layout.admin.sections.marketing'),
    items: [
      { to: '/admin/plans', icon: 'PL', label: t('layout.admin.nav.plans') },
      { to: '/admin/coupons', icon: 'CP', label: t('layout.admin.nav.coupons') },
      { to: '/admin/invite', icon: 'IV', label: t('layout.admin.nav.invite') }
    ]
  },
  {
    title: t('layout.admin.sections.finance'),
    items: [{ to: '/admin/payment', icon: 'PY', label: t('layout.admin.nav.payment') }]
  },
  {
    title: t('layout.admin.sections.notifications'),
    items: [
      { to: '/admin/telegram', icon: 'TG', label: t('layout.admin.nav.telegram') },
      { to: '/admin/notifications', icon: 'NT', label: t('layout.admin.nav.notifications') }
    ]
  },
  {
    title: t('layout.admin.sections.content'),
    items: [{ to: '/admin/knowledge', icon: 'KB', label: t('layout.admin.nav.knowledge') }]
  },
  {
    title: t('layout.admin.sections.system'),
    items: [
      { to: '/admin/mfa', icon: 'MF', label: t('layout.admin.nav.mfa') },
      { to: '/admin/control', icon: 'CT', label: t('layout.admin.nav.control') },
      { to: '/admin/system', icon: 'SY', label: t('layout.admin.nav.system') }
    ]
  }
  ]
  const extensionMenusByParent = new Map(WEBUI_MENU_PARENT_REGISTRY.map(parent => [parent, []]))
  for (const item of adminExtensionMenus.value.filter(menu => userStore.hasPermission(menu.permission))) {
    const parent = normalizeWebUIMenuParent(item.parent)
    extensionMenusByParent.get(parent).push({ ...item, parent })
  }
  const extensionSections = WEBUI_MENU_PARENT_REGISTRY.flatMap(parent => {
    const items = extensionMenusByParent.get(parent).sort(compareExtensionMenus)
    if (items.length === 0) {
      return []
    }
    return [{
      id: `extension:${parent}`,
      kind: 'extension',
      parent,
      title: t(extensionSectionTitleKeys[parent]),
      items
    }]
  })
  if (extensionSections.length > 0) {
    sections.splice(sections.length - 1, 0, ...extensionSections)
  }
  return sections
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
  sidebarOpen.value = false
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

async function loadSystemInfo() {
  try {
    const res = await getSystemInfo()
    const info = readSystemInfo(res)
    systemVersion.value = info.version || ''
    systemBuildCode.value = info.build_code || frontendBuildCode
    systemBuildTime.value = info.build_time || ''
    systemCommit.value = info.commit || ''
  } catch {
    // ignore layout metadata failures
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

onMounted(() => {
  updateTime()
  timer = setInterval(updateTime, 60000)
  if (userStore.isLoggedIn) {
    userStore.getUserInfo()
  }
  loadSystemInfo()
})

onUnmounted(() => {
  clearInterval(timer)
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
  background: transparent;
}

.sidebar-overlay {
  position: fixed;
  inset: 0;
  background: rgba(15, 23, 42, 0.32);
  opacity: 0;
  pointer-events: none;
  transition: opacity 0.2s ease;
  z-index: 1000;
}

.sidebar-overlay.active {
  opacity: 1;
  pointer-events: auto;
}

.sidebar {
  width: var(--sidebar-width);
  background: linear-gradient(180deg, #0f172a 0%, #172033 100%);
  color: #d8e1f0;
  padding: 20px 16px 16px;
  display: flex;
  flex-direction: column;
  gap: 18px;
  position: sticky;
  top: 0;
  height: 100vh;
  border-right: 1px solid rgba(148, 163, 184, 0.18);
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
  gap: 12px;
}

.brand-block {
  gap: 12px;
}

.brand-mark,
.nav-link-icon,
.operator-avatar {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
}

.brand-mark {
  width: 40px;
  height: 40px;
  background: rgba(0, 100, 250, 0.16);
  color: #fff;
  font-size: 13px;
  font-weight: 800;
}

.brand-name {
  font-size: 16px;
  font-weight: 700;
  color: #fff;
}

.brand-meta {
  font-size: 12px;
  color: rgba(216, 225, 240, 0.72);
}

.sidebar-nav {
  flex: 1;
  overflow-y: auto;
  padding-right: 4px;
}

.nav-section + .nav-section {
  margin-top: 18px;
}

.nav-section-title {
  margin-bottom: 8px;
  padding: 0 8px;
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
  color: rgba(216, 225, 240, 0.62);
}

.nav-link,
.sidebar :deep(.forward-suite-link) {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 40px;
  padding: 9px 10px;
  color: rgba(216, 225, 240, 0.86);
  text-decoration: none;
  border-radius: 8px;
  transition: all 0.2s ease;
}

.nav-link:hover,
.nav-link.router-link-active,
.sidebar :deep(.forward-suite-link:hover),
.sidebar :deep(.forward-suite-link.router-link-active),
.sidebar :deep(.forward-suite-link-active) {
  background: rgba(255, 255, 255, 0.09);
  color: #fff;
}

.nav-link-icon {
  width: 28px;
  height: 28px;
  background: rgba(255, 255, 255, 0.08);
  font-size: 11px;
  font-weight: 700;
}

.nav-link-label {
  min-width: 0;
}

.sidebar-footer {
  border-top: 1px solid rgba(148, 163, 184, 0.16);
  padding-top: 14px;
}

.operator-card {
  gap: 12px;
  padding: 12px;
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.05);
}

.operator-avatar {
  width: 36px;
  height: 36px;
  background: rgba(0, 100, 250, 0.18);
  color: #fff;
  font-weight: 700;
}

.operator-name {
  color: #fff;
  font-weight: 600;
}

.operator-email {
  font-size: 12px;
  color: rgba(216, 225, 240, 0.72);
  word-break: break-all;
}

.version-line {
  margin: 10px 0;
  font-size: 12px;
  color: rgba(216, 225, 240, 0.56);
}

.sidebar-actions {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.workspace {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.topbar {
  justify-content: space-between;
  gap: 16px;
  padding: 18px 24px;
  border-bottom: 1px solid rgba(220, 227, 240, 0.9);
  backdrop-filter: blur(8px);
}

.topbar-primary {
  gap: 12px;
}

.menu-button {
  display: none;
}

.topbar-title {
  font-size: 22px;
  font-weight: 700;
  line-height: 1.2;
}

.topbar-subtitle {
  font-size: 13px;
  color: var(--text-secondary);
}

.topbar-actions {
  gap: 12px;
}

.current-time {
  font-size: 13px;
  color: var(--text-secondary);
}

.content {
  flex: 1;
  padding: 24px;
}

.legacy-hidden-link {
  display: none !important;
}

@media (max-width: 1024px) {
  .sidebar {
    position: fixed;
    inset: 0 auto 0 0;
    z-index: 1010;
    transform: translateX(-100%);
    transition: transform 0.2s ease;
  }

  .sidebar.open {
    transform: translateX(0);
  }

  .menu-button {
    display: inline-flex;
  }
}

@media (max-width: 768px) {
  .topbar,
  .topbar-actions {
    align-items: flex-start;
    flex-direction: column;
  }

  .content {
    padding: 18px;
  }
}
</style>
