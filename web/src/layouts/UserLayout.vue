<template>
  <div class="user-layout">
    <header class="header">
      <button
        class="menu-toggle"
        type="button"
        :aria-label="t('common.a11y.openNavigation')"
        :title="t('common.a11y.openNavigation')"
        aria-controls="user-sidebar"
        :aria-expanded="sidebarOpen ? 'true' : 'false'"
        @click="sidebarOpen = !sidebarOpen"
      >
        <span class="menu-icon" aria-hidden="true"></span>
      </button>
      <div class="logo">{{ t('layout.user.brand') }}</div>
      <nav class="desktop-nav" :aria-label="t('layout.user.brand')">
        <router-link v-for="item in navItems" :key="item.to" :to="item.to">{{ item.label }}</router-link>
      </nav>
      <div class="user-actions">
        <LocaleSwitcher compact />
        <span class="user-email">{{ userStore.userInfo?.email }}</span>
        <button class="btn-ghost btn-sm" type="button" @click="logout">{{ t('common.actions.logout') }}</button>
      </div>
    </header>

    <div class="sidebar-overlay" :class="{ active: sidebarOpen }" aria-hidden="true" @click="sidebarOpen = false"></div>

    <aside id="user-sidebar" class="mobile-sidebar" :class="{ open: sidebarOpen }" :aria-label="t('layout.user.brand')">
      <div class="sidebar-header">
        <div class="logo">{{ t('layout.user.brand') }}</div>
        <button
          class="close-btn"
          type="button"
          :aria-label="t('common.a11y.closeNavigation')"
          :title="t('common.a11y.closeNavigation')"
          @click="sidebarOpen = false"
        ></button>
      </div>
      <nav class="sidebar-nav" :aria-label="t('layout.user.brand')">
        <router-link v-for="item in navItems" :key="item.to" :to="item.to" @click="sidebarOpen = false">
          <span class="nav-icon" aria-hidden="true">{{ item.icon }}</span>
          <span>{{ item.label }}</span>
        </router-link>
      </nav>
      <div class="sidebar-footer">
        <LocaleSwitcher />
        <button class="btn-secondary w-full" type="button" @click="logout">{{ t('common.actions.logout') }}</button>
      </div>
    </aside>

    <main id="app-main-content" class="main-content" tabindex="-1" :aria-label="pageTitle">
      <div class="container">
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
import { resolveRoutePageTitle } from '@/utils/pageMeta'

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()
const { t } = useAppI18n()
const sidebarOpen = ref(false)

const navItems = computed(() => ([
  { to: '/user/dashboard', label: t('layout.user.nav.dashboard'), icon: 'D' },
  { to: '/user/subscribe', label: t('layout.user.nav.subscribe'), icon: 'S' },
  { to: '/user/knowledge', label: t('layout.user.nav.knowledge'), icon: 'K' },
  { to: '/user/tickets', label: t('layout.user.nav.tickets'), icon: 'T' },
  { to: '/user/plans', label: t('layout.user.nav.plans'), icon: 'P' },
  { to: '/user/orders', label: t('layout.user.nav.orders'), icon: 'O' }
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
  display: flex;
  flex-direction: column;
}

.header {
  height: var(--header-height);
  border-bottom: 1px solid var(--border-color);
  display: flex;
  align-items: center;
  padding: 0 16px;
  background: var(--surface-color);
  position: sticky;
  top: 0;
  z-index: 100;
  gap: 16px;
}

.menu-toggle {
  display: flex;
  padding: 8px;
  background: transparent;
  border: none;
  font-size: 0;
  color: var(--text-color);
}

.menu-icon {
  line-height: 1;
}

.menu-icon::before {
  content: '\2630';
  font-size: 20px;
}

.logo {
  font-weight: 700;
  font-size: 18px;
  background: linear-gradient(135deg, #3b82f6, #8b5cf6);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.desktop-nav {
  display: none;
  gap: 24px;
  flex: 1;
  margin-left: 32px;
}

.desktop-nav a {
  color: var(--text-secondary);
  text-decoration: none;
  font-size: 14px;
  font-weight: 500;
  padding: 8px 0;
  transition: var(--transition);
}

.desktop-nav a:hover,
.desktop-nav a.router-link-active {
  color: var(--text-color);
}

.user-actions {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-left: auto;
}

.user-email {
  display: none;
  font-size: 14px;
  color: var(--text-secondary);
}

.sidebar-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.5);
  z-index: 200;
  opacity: 0;
  visibility: hidden;
  transition: var(--transition);
}

.sidebar-overlay.active {
  opacity: 1;
  visibility: visible;
}

.mobile-sidebar {
  position: fixed;
  top: 0;
  left: 0;
  width: 280px;
  max-width: 85vw;
  height: 100vh;
  background: var(--surface-color);
  border-right: 1px solid var(--border-color);
  z-index: 300;
  transform: translateX(-100%);
  transition: transform 0.3s ease;
  display: flex;
  flex-direction: column;
}

.mobile-sidebar.open {
  transform: translateX(0);
}

.sidebar-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 20px;
  border-bottom: 1px solid var(--border-color);
}

.close-btn {
  padding: 8px;
  background: transparent;
  border: none;
  font-size: 0;
  color: var(--text-secondary);
}

.close-btn::before {
  content: '\00d7';
  font-size: 20px;
}

.sidebar-nav {
  flex: 1;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.sidebar-nav a {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px 16px;
  color: var(--text-secondary);
  text-decoration: none;
  border-radius: var(--radius-md);
  font-size: 15px;
  transition: var(--transition);
}

.sidebar-nav a:hover,
.sidebar-nav a.router-link-active {
  background: var(--bg-color);
  color: var(--text-color);
}

.nav-icon {
  width: 24px;
  text-align: center;
  font-weight: 700;
}

.sidebar-footer {
  padding: 16px;
  border-top: 1px solid var(--border-color);
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.main-content {
  flex: 1;
  padding: 24px 0;
}

@media (min-width: 768px) {
  .menu-toggle {
    display: none;
  }

  .desktop-nav {
    display: flex;
  }

  .user-email {
    display: block;
  }

  .mobile-sidebar,
  .sidebar-overlay {
    display: none;
  }

  .header {
    padding: 0 24px;
  }
}
</style>
