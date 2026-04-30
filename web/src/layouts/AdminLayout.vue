<template>
  <div class="admin-layout">
    <header class="mobile-header">
      <button
        class="menu-toggle"
        type="button"
        :aria-label="t('common.a11y.openNavigation')"
        :title="t('common.a11y.openNavigation')"
        aria-controls="admin-sidebar"
        :aria-expanded="sidebarOpen ? 'true' : 'false'"
        @click="sidebarOpen = !sidebarOpen"
      >
        <span class="menu-icon" aria-hidden="true"></span>
      </button>
      <div class="logo">{{ t('layout.admin.mobileTitle') }}</div>
      <div class="mobile-header-actions">
        <LocaleSwitcher compact />
        <button class="btn-ghost btn-sm" type="button" @click="logout">{{ t('common.actions.logout') }}</button>
      </div>
    </header>

    <div class="sidebar-overlay" :class="{ active: sidebarOpen }" aria-hidden="true" @click="sidebarOpen = false"></div>

    <aside id="admin-sidebar" class="sidebar" :class="{ open: sidebarOpen }" :aria-label="t('layout.admin.mobileTitle')">
      <div class="sidebar-header">
        <div class="sidebar-brand">
          <div class="logo">{{ t('layout.admin.brand') }}</div>
          <span class="badge">{{ t('layout.admin.badge') }}</span>
        </div>
        <button
          class="close-btn"
          type="button"
          :aria-label="t('common.a11y.closeNavigation')"
          :title="t('common.a11y.closeNavigation')"
          @click="sidebarOpen = false"
        ></button>
      </div>

      <nav class="sidebar-nav" :aria-label="t('layout.admin.mobileTitle')">
        <div v-for="section in navSections" :key="section.title" class="nav-section">
          <div class="nav-title">{{ section.title }}</div>
          <template v-if="section.kind === 'forward'">
            <ForwardSuiteNav />
          </template>
          <template v-else>
            <router-link v-for="item in section.items" :key="item.to" :to="item.to" @click="closeSidebar">
              <span class="nav-icon" aria-hidden="true">{{ item.icon }}</span>
              <span>{{ item.label }}</span>
            </router-link>
          </template>
        </div>
        <router-link to="/admin/agent" class="legacy-hidden-link" aria-hidden="true" tabindex="-1">{{ t('layout.admin.nav.nodeXAgentsLegacy') }}</router-link>
      </nav>

      <div class="sidebar-footer">
        <div class="user-info">
          <div class="user-avatar">A</div>
          <div class="user-details">
            <div class="user-name">{{ t('layout.admin.adminUser') }}</div>
            <div class="user-email">{{ userStore.userInfo?.email || '-' }}</div>
          </div>
        </div>
        <div class="sidebar-footer-actions">
          <LocaleSwitcher compact />
          <button class="btn-ghost btn-sm w-full" type="button" @click="logout">{{ t('common.actions.logout') }}</button>
        </div>
      </div>
    </aside>

    <main id="app-main-content" class="main-content" tabindex="-1" :aria-label="pageTitle">
      <header class="content-header">
        <div class="header-title">
          <h1>{{ pageTitle }}</h1>
          <p class="header-subtitle">{{ t('layout.admin.subtitle') }}</p>
        </div>
        <div class="header-actions">
          <LocaleSwitcher />
          <span class="current-time">{{ currentTime }}</span>
        </div>
      </header>
      <div class="page-content">
        <router-view></router-view>
      </div>
    </main>
  </div>
</template>

<script setup>
import { computed, onMounted, onUnmounted, ref, watchEffect } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useUserStore } from '@/stores/user'
import { useAppI18n } from '@/composables/useAppI18n'
import ForwardSuiteNav from '@/components/admin/ForwardSuiteNav.vue'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import { resolveRoutePageTitle } from '@/utils/pageMeta'

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()
const { t, currentLocale, formatDateTime } = useAppI18n()

const sidebarOpen = ref(false)
const currentTime = ref('')

const navSections = computed(() => ([
  {
    title: t('layout.admin.sections.overview'),
    items: [
      { to: '/admin/dashboard', icon: 'D', label: t('layout.admin.nav.dashboard') },
      { to: '/admin/monitor', icon: 'M', label: t('layout.admin.nav.monitor') }
    ]
  },
  {
    title: t('layout.admin.sections.forwardSuite'),
    kind: 'forward'
  },
  {
    title: t('layout.admin.sections.userManagement'),
    items: [
      { to: '/admin/users', icon: 'U', label: t('layout.admin.nav.users') },
      { to: '/admin/orders', icon: 'O', label: t('layout.admin.nav.orders') },
      { to: '/admin/tickets', icon: 'T', label: t('layout.admin.nav.tickets') }
    ]
  },
  {
    title: t('layout.admin.sections.nodeManagement'),
    items: [
      { to: '/admin/nodes', icon: 'N', label: t('layout.admin.nav.nodes') },
      { to: '/admin/subscriptions', icon: 'S', label: t('layout.admin.nav.subscriptions') }
    ]
  },
  {
    title: t('layout.admin.sections.marketing'),
    items: [
      { to: '/admin/plans', icon: 'P', label: t('layout.admin.nav.plans') },
      { to: '/admin/coupons', icon: 'C', label: t('layout.admin.nav.coupons') },
      { to: '/admin/invite', icon: 'I', label: t('layout.admin.nav.invite') }
    ]
  },
  {
    title: t('layout.admin.sections.finance'),
    items: [{ to: '/admin/payment', icon: '$', label: t('layout.admin.nav.payment') }]
  },
  {
    title: t('layout.admin.sections.notifications'),
    items: [
      { to: '/admin/telegram', icon: 'TG', label: t('layout.admin.nav.telegram') },
      { to: '/admin/notifications', icon: '!', label: t('layout.admin.nav.notifications') }
    ]
  },
  {
    title: t('layout.admin.sections.content'),
    items: [{ to: '/admin/knowledge', icon: 'K', label: t('layout.admin.nav.knowledge') }]
  },
  {
    title: t('layout.admin.sections.system'),
    items: [
      { to: '/admin/mfa', icon: 'M', label: t('layout.admin.nav.mfa') },
      { to: '/admin/system', icon: 'SYS', label: t('layout.admin.nav.system') }
    ]
  }
]))

const pageTitle = computed(() => resolveRoutePageTitle(t, route.path, t('pageTitles.admin.fallback')))

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

let timer

onMounted(() => {
  updateTime()
  timer = setInterval(updateTime, 60000)
  if (userStore.isLoggedIn) {
    userStore.getUserInfo()
  }
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
  display: flex;
  min-height: 100vh;
  background: var(--bg-color);
}

.mobile-header {
  display: none;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 16px;
  border-bottom: 1px solid var(--border-color);
  background: var(--surface-color);
  position: sticky;
  top: 0;
  z-index: 1200;
}

.mobile-header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.menu-toggle,
.close-btn {
  border: 0;
  background: transparent;
  color: var(--text-color);
  cursor: pointer;
  font-size: 0;
  line-height: 1;
}

.menu-toggle {
  font-size: 0;
}

.close-btn::before {
  content: '\00d7';
  font-size: 22px;
}

.menu-icon::before {
  content: '\2630';
  font-size: 22px;
}

.sidebar-overlay {
  position: fixed;
  inset: 0;
  background: rgba(15, 23, 42, 0.45);
  opacity: 0;
  pointer-events: none;
  transition: opacity 0.2s ease;
  z-index: 1090;
}

.sidebar-overlay.active {
  opacity: 1;
  pointer-events: auto;
}

.sidebar {
  width: 280px;
  border-right: 1px solid var(--border-color);
  background: linear-gradient(180deg, rgba(15, 23, 42, 0.98), rgba(15, 23, 42, 0.9));
  color: #e2e8f0;
  display: flex;
  flex-direction: column;
  min-height: 100vh;
  position: sticky;
  top: 0;
  z-index: 1100;
}

.sidebar-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 20px;
  border-bottom: 1px solid rgba(148, 163, 184, 0.14);
}

.sidebar-brand {
  display: flex;
  align-items: center;
  gap: 10px;
}

.logo {
  font-size: 18px;
  font-weight: 700;
  color: #fff;
}

.badge {
  font-size: 11px;
  padding: 4px 10px;
  border-radius: 999px;
  background: rgba(59, 130, 246, 0.16);
  color: #93c5fd;
}

.sidebar-nav {
  flex: 1;
  overflow-y: auto;
  padding: 18px 14px;
}

.nav-section + .nav-section {
  margin-top: 18px;
}

.nav-title {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: rgba(226, 232, 240, 0.7);
  margin-bottom: 8px;
  padding: 0 10px;
}

.sidebar-nav :deep(a) {
  display: flex;
  align-items: center;
  gap: 10px;
  color: rgba(226, 232, 240, 0.9);
  text-decoration: none;
  padding: 11px 12px;
  border-radius: 12px;
  transition: all 0.2s ease;
}

.sidebar-nav :deep(a:hover),
.sidebar-nav :deep(a.router-link-active) {
  background: rgba(59, 130, 246, 0.12);
  color: #fff;
}

.nav-icon {
  width: 28px;
  font-size: 12px;
  font-weight: 700;
  text-align: center;
  opacity: 0.9;
}

.sidebar-footer {
  padding: 16px 14px 20px;
  border-top: 1px solid rgba(148, 163, 184, 0.14);
}

.sidebar-footer-actions {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.user-info {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 14px;
}

.user-avatar {
  width: 38px;
  height: 38px;
  border-radius: 12px;
  background: rgba(59, 130, 246, 0.18);
  color: #bfdbfe;
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 700;
}

.user-details {
  min-width: 0;
}

.user-name {
  font-weight: 600;
  color: #fff;
}

.user-email {
  font-size: 12px;
  color: rgba(226, 232, 240, 0.7);
  word-break: break-all;
}

.main-content {
  flex: 1;
  min-width: 0;
}

.content-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  padding: 28px 32px 20px;
}

.header-title h1 {
  margin: 0 0 6px;
  font-size: 30px;
  font-weight: 700;
}

.header-subtitle {
  margin: 0;
  color: var(--text-secondary);
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.current-time {
  color: var(--text-secondary);
  font-size: 13px;
}

.page-content {
  padding: 0 32px 32px;
}

.legacy-hidden-link {
  display: none !important;
}

@media (max-width: 1024px) {
  .admin-layout {
    display: block;
  }

  .mobile-header {
    display: flex;
  }

  .sidebar {
    position: fixed;
    inset: 0 auto 0 0;
    transform: translateX(-100%);
    transition: transform 0.2s ease;
  }

  .sidebar.open {
    transform: translateX(0);
  }

  .content-header {
    padding: 22px 20px 16px;
  }

  .page-content {
    padding: 0 20px 24px;
  }
}

@media (max-width: 768px) {
  .content-header,
  .header-actions {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>
