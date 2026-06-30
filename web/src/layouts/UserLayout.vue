<template>
  <div class="user-layout">
    <header class="user-header">
      <div class="user-header-main">
        <button
          class="btn btn-ghost btn-sm menu-toggle"
          type="button"
          :aria-label="t('common.a11y.openNavigation')"
          :title="t('common.a11y.openNavigation')"
          aria-controls="user-sidebar"
          :aria-expanded="sidebarOpen ? 'true' : 'false'"
          @click="sidebarOpen = !sidebarOpen"
        >
          ☰
        </button>
        <div class="brand-block">
          <div class="brand-mark">AO</div>
          <div>
            <div class="brand-name">{{ t('layout.user.brand') }}</div>
            <div class="brand-subtitle">{{ pageTitle }}</div>
          </div>
        </div>
      </div>

      <nav class="desktop-nav" :aria-label="t('layout.user.brand')">
        <router-link v-for="item in navItems" :key="item.to" :to="item.to">{{ item.label }}</router-link>
      </nav>

      <div class="user-actions">
        <ThemeToggle compact />
        <LocaleSwitcher compact />
        <div class="user-chip">
          <span class="user-chip-label">{{ userStore.userInfo?.email || '-' }}</span>
        </div>
        <button class="btn" type="button" @click="logout">{{ t('common.actions.logout') }}</button>
      </div>
    </header>

    <div class="sidebar-overlay" :class="{ active: sidebarOpen }" aria-hidden="true" @click="sidebarOpen = false"></div>

    <aside id="user-sidebar" class="mobile-sidebar" :class="{ open: sidebarOpen }" :aria-label="t('layout.user.brand')">
      <div class="mobile-sidebar-header">
        <div class="brand-block">
          <div class="brand-mark">AO</div>
          <div class="brand-name">{{ t('layout.user.brand') }}</div>
        </div>
        <button
          class="btn btn-ghost btn-sm"
          type="button"
          :aria-label="t('common.a11y.closeNavigation')"
          :title="t('common.a11y.closeNavigation')"
          @click="sidebarOpen = false"
        >
          x
        </button>
      </div>
      <nav class="sidebar-nav" :aria-label="t('layout.user.brand')">
        <router-link v-for="item in navItems" :key="item.to" :to="item.to" class="sidebar-link" @click="sidebarOpen = false">
          <span class="sidebar-link-icon">{{ item.icon }}</span>
          <span>{{ item.label }}</span>
        </router-link>
      </nav>
      <div class="mobile-sidebar-footer">
        <LocaleSwitcher />
        <button class="btn w-full" type="button" @click="logout">{{ t('common.actions.logout') }}</button>
      </div>
    </aside>

    <main id="app-main-content" class="main-content" tabindex="-1" :aria-label="pageTitle">
      <div class="container content-shell">
        <router-view></router-view>
      </div>
    </main>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useUserStore } from '@/stores/user'
import { useAppI18n } from '@/composables/useAppI18n'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import ThemeToggle from '@/components/common/ThemeToggle.vue'
import { resolveRoutePageTitle } from '@/utils/pageMeta'

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()
const { t } = useAppI18n()
const sidebarOpen = ref(false)

const navItems = computed(() => ([
  { to: '/user/dashboard', label: t('layout.user.nav.dashboard'), icon: 'DB' },
  { to: '/user/subscribe', label: t('layout.user.nav.subscribe'), icon: 'SB' },
  { to: '/user/knowledge', label: t('layout.user.nav.knowledge'), icon: 'KB' },
  { to: '/user/tickets', label: t('layout.user.nav.tickets'), icon: 'TK' },
  { to: '/user/plans', label: t('layout.user.nav.plans'), icon: 'PL' },
  { to: '/user/orders', label: t('layout.user.nav.orders'), icon: 'OR' }
]))

const pageTitle = computed(() => resolveRoutePageTitle(t, route.path, t('layout.user.brand')))

function logout() {
  userStore.logout()
  router.push('/login')
}

onMounted(() => {
  if (userStore.isLoggedIn) {
    userStore.getUserInfo()
  }
})
</script>

<style scoped>
.user-layout {
  min-height: 100vh;
}

.user-header,
.user-header-main,
.brand-block,
.user-actions {
  display: flex;
  align-items: center;
}

.user-header {
  min-height: var(--header-height);
  justify-content: space-between;
  gap: 18px;
  padding: 14px 20px;
  border-bottom: 1px solid rgba(220, 227, 240, 0.9);
  background: rgba(247, 249, 252, 0.82);
  backdrop-filter: blur(10px);
  position: sticky;
  top: 0;
  z-index: 100;
}

.user-header-main {
  gap: 12px;
}

.menu-toggle {
  display: none;
}

.brand-block {
  gap: 12px;
}

.brand-mark,
.sidebar-link-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
}

.brand-mark {
  width: 40px;
  height: 40px;
  background: var(--primary-soft);
  color: var(--primary-color);
  font-weight: 800;
  font-size: 13px;
}

.brand-name {
  font-size: 16px;
  font-weight: 700;
}

.brand-subtitle {
  font-size: 12px;
  color: var(--text-secondary);
}

.desktop-nav {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 1;
}

.desktop-nav a,
.sidebar-link {
  color: var(--text-secondary);
  text-decoration: none;
  border-radius: 8px;
  transition: all 0.2s ease;
}

.desktop-nav a {
  padding: 10px 12px;
  font-size: 14px;
  font-weight: 600;
}

.desktop-nav a:hover,
.desktop-nav a.router-link-active {
  background: var(--surface-color);
  color: var(--text-color);
  box-shadow: var(--shadow-sm);
}

.user-actions {
  gap: 10px;
}

.user-chip {
  display: inline-flex;
  align-items: center;
  min-height: 40px;
  padding: 0 12px;
  background: rgba(255, 255, 255, 0.86);
  border: 1px solid var(--border-color);
  border-radius: 999px;
  box-shadow: var(--shadow-sm);
}

.user-chip-label {
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
  color: var(--text-secondary);
}

.sidebar-overlay {
  position: fixed;
  inset: 0;
  background: rgba(15, 23, 42, 0.3);
  opacity: 0;
  pointer-events: none;
  transition: opacity 0.2s ease;
  z-index: 190;
}

.sidebar-overlay.active {
  opacity: 1;
  pointer-events: auto;
}

.mobile-sidebar {
  position: fixed;
  inset: 0 auto 0 0;
  width: 300px;
  max-width: 88vw;
  background: #ffffff;
  border-right: 1px solid var(--border-color);
  z-index: 200;
  transform: translateX(-100%);
  transition: transform 0.2s ease;
  display: flex;
  flex-direction: column;
}

.mobile-sidebar.open {
  transform: translateX(0);
}

.mobile-sidebar-header,
.mobile-sidebar-footer {
  padding: 18px 16px;
}

.mobile-sidebar-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid var(--border-color);
}

.sidebar-nav {
  flex: 1;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.sidebar-link {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 42px;
  padding: 10px 12px;
}

.sidebar-link:hover,
.sidebar-link.router-link-active {
  background: var(--surface-hover);
  color: var(--text-color);
}

.sidebar-link-icon {
  width: 28px;
  height: 28px;
  background: var(--surface-muted);
  color: var(--primary-color);
  font-size: 11px;
  font-weight: 700;
}

.mobile-sidebar-footer {
  border-top: 1px solid var(--border-color);
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.main-content {
  padding: 24px 0 32px;
}

.content-shell {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

@media (max-width: 1024px) {
  .menu-toggle {
    display: inline-flex;
  }

  .desktop-nav,
  .user-chip {
    display: none;
  }
}

@media (max-width: 768px) {
  .user-header {
    padding: 12px 16px;
  }

  .main-content {
    padding-top: 18px;
  }
}
</style>
