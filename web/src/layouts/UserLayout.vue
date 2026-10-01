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
          <BrandLockup />
          <div class="brand-subtitle">{{ pageTitle }}</div>
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
        <BrandLockup size="sm" />
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
import BrandLockup from '@/components/common/BrandLockup.vue'
import { resolveRoutePageTitle } from '@/utils/pageMeta'
import { filterByEdition, loadEdition } from '@/composables/useEdition'

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()
const { t } = useAppI18n()
const sidebarOpen = ref(false)

const navItems = computed(() => filterByEdition([
  { to: '/user/dashboard', label: t('layout.user.nav.dashboard'), icon: 'DB' },
  { to: '/user/subscribe', label: t('layout.user.nav.subscribe'), icon: 'SB' },
  { to: '/user/knowledge', label: t('layout.user.nav.knowledge'), icon: 'KB' },
  { to: '/user/tickets', label: t('layout.user.nav.tickets'), icon: 'TK' },
  { to: '/user/plans', label: t('layout.user.nav.plans'), icon: 'PL', edition: 'commercial' },
  { to: '/user/orders', label: t('layout.user.nav.orders'), icon: 'OR', edition: 'commercial' }
]))

const pageTitle = computed(() => resolveRoutePageTitle(t, route.path, t('layout.user.brand')))

function logout() {
  userStore.logout()
  router.push('/login')
}

onMounted(() => {
  void loadEdition()
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
.user-actions {
  display: flex;
  align-items: center;
}

.user-header {
  min-height: var(--header-height);
  justify-content: space-between;
  gap: 18px;
  padding: 14px 20px;
  border-bottom: 1px solid var(--separator);
  background: var(--material);
  -webkit-backdrop-filter: saturate(180%) blur(20px);
  backdrop-filter: saturate(180%) blur(20px);
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
  display: flex;
  min-width: 0;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--space-0-5);
}

.sidebar-link-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-xs);
}

/* Lines up with the lockup text: mark (26 px) plus a third of it. */
.brand-subtitle {
  padding-left: calc(26px * 4 / 3);
  font-size: var(--type-caption-size);
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
  border-radius: var(--radius-sm);
  transition: all 0.2s ease;
}

.desktop-nav a {
  padding: 10px 12px;
  font-size: var(--type-body-size);
  font-weight: var(--weight-medium);
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
  background: var(--bg-elevated);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-pill);
  box-shadow: var(--shadow-sm);
}

.user-chip-label {
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--type-callout-size);
  color: var(--text-secondary);
}

.sidebar-overlay {
  position: fixed;
  inset: 0;
  background: var(--scrim);
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
  background: var(--bg-elevated);
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
  font-size: var(--type-caption-size);
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

  /* Phones: language and logout live in the navigation drawer, so the
     header keeps only the menu, the brand and the theme switch. */
  .brand-subtitle,
  .user-actions > .locale-switcher,
  .user-actions > .btn {
    display: none;
  }

  .main-content {
    padding-top: 18px;
  }
}
</style>
