<template>
  <div class="admin-shell" :class="{ 'is-rail': railActive, 'is-drawer': drawerMode }">
    <div class="admin-scrim" :class="{ 'is-open': sidebarOpen }" aria-hidden="true" @click="closeSidebar"></div>

    <!-- Not a landmark itself: the <nav> inside is. As a drawer it is a modal dialog. -->
    <div
      id="admin-sidebar"
      ref="sidebarElement"
      class="admin-sidebar"
      :class="{ 'is-open': sidebarOpen }"
      :role="drawerMode ? 'dialog' : undefined"
      :aria-modal="drawerMode && sidebarOpen ? 'true' : undefined"
      :aria-label="drawerMode ? t('shell.admin.navLabel') : undefined"
      :inert="drawerInactive"
      :aria-hidden="drawerInactive ? 'true' : undefined"
      @keydown="handleDrawerKeydown"
    >
      <div class="admin-sidebar__top">
        <router-link to="/admin/dashboard" class="admin-sidebar__brand" :title="railActive ? 'AnixOps Control' : undefined">
          <BrandLockup size="sm" tile :mark-only="railActive" />
          <span v-if="railActive" class="visually-hidden">AnixOps Control</span>
        </router-link>
        <UiIconButton
          v-if="drawerMode"
          ref="closeButton"
          class="admin-sidebar__close"
          :label="t('common.a11y.closeNavigation')"
          :icon="X"
          @click="closeSidebar"
        />
      </div>

      <AdminNavigation
        class="admin-sidebar__nav"
        :groups="groups"
        :active-id="active?.item.id || ''"
        :rail="railActive"
        @navigate="onNavigate"
      />

      <div class="admin-sidebar__footer">
        <AccountMenu
          variant="row"
          role="admin"
          :compact="railActive"
          :account-path="ADMIN_ACCOUNT_PATH"
          show-about
          @about="aboutOpen = true"
        />
      </div>
    </div>

    <div class="admin-workspace">
      <header class="admin-topbar">
        <UiIconButton
          ref="menuButton"
          class="admin-topbar__toggle"
          :label="toggleLabel"
          :icon="toggleIcon"
          aria-controls="admin-sidebar"
          :aria-expanded="drawerMode ? String(sidebarOpen) : String(!sidebarCollapsed)"
          data-sidebar-toggle
          @click="toggleSidebar"
        />

        <nav class="admin-crumbs" :aria-label="t('shell.admin.breadcrumb')">
          <ol>
            <li v-for="(crumb, index) in crumbs" :key="index" :class="{ 'is-current': index === crumbs.length - 1 }">
              <router-link v-if="crumb.to && index < crumbs.length - 1" :to="crumb.to">{{ crumb.label }}</router-link>
              <span v-else :aria-current="index === crumbs.length - 1 ? 'page' : undefined">{{ crumb.label }}</span>
            </li>
          </ol>
        </nav>

        <button
          type="button"
          class="admin-search"
          :aria-label="t('shell.search.label', { shortcut: shortcutLabel })"
          :aria-keyshortcuts="'Meta+K Control+K'"
          aria-haspopup="dialog"
          data-palette-trigger
          @click="openPalette"
        >
          <UiIcon :icon="Search" />
          <span class="admin-search__text">{{ t('shell.search.button') }}</span>
          <kbd class="admin-search__kbd">{{ shortcutLabel }}</kbd>
        </button>

        <div v-if="drawerMode" class="admin-topbar__account">
          <AccountMenu
            variant="avatar"
            role="admin"
            :account-path="ADMIN_ACCOUNT_PATH"
            show-about
            @about="aboutOpen = true"
          />
        </div>
      </header>

      <main id="app-main-content" class="admin-main" tabindex="-1" :aria-label="pageTitle">
        <div class="admin-content" :class="{ 'is-wide': route.meta?.layout === 'wide' }">
          <ForwardSuiteNav v-if="forwardSuite" class="admin-content__suite-nav" />
          <ShellRouterView />
        </div>
      </main>
    </div>

    <CommandPalette :groups="groups" />
    <AboutDialog v-model:open="aboutOpen" />
  </div>
</template>

<script setup>
// Admin shell (plan §8.3, D3). Light frosted sidebar (dark in dark mode)
// with the lockup, the menu groups from navigation/menu.js and the account
// menu; a top bar with the sidebar toggle, the breadcrumb, the ⌘K search
// button (and the avatar when the sidebar is a drawer). On wide screens the
// toggle switches the sidebar between full width and an icon rail
// (remembered); below 834 px the sidebar is a modal drawer with its own
// focus loop, Esc and focus return. Content keeps to --size-content-admin
// unless the route asks for meta.layout = 'wide'.
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Menu, PanelLeftClose, PanelLeftOpen, Search, X } from '@lucide/vue'
import { useRoute } from 'vue-router'
import { useUserStore } from '@/stores/user'
import { useAppI18n } from '@/composables/useAppI18n'
import { useMediaQuery, NARROW_QUERY } from '@/composables/useMediaQuery'
import { loadEdition } from '@/composables/useEdition'
import { resolveRoutePageTitle } from '@/utils/pageMeta'
import { adminExtensionMenus } from '@/extensions/runtime'
import { useAdminMenu } from '@/navigation/useNavigation'
import { ADMIN_ACCOUNT_PATH, isForwardSuitePath } from '@/navigation/menu'
import AdminNavigation from '@/components/admin/AdminNavigation.vue'
import ForwardSuiteNav from '@/components/admin/ForwardSuiteNav.vue'
import BrandLockup from '@/components/common/BrandLockup.vue'
import AccountMenu from '@/components/shell/AccountMenu.vue'
import AboutDialog from '@/components/shell/AboutDialog.vue'
import CommandPalette from '@/components/shell/CommandPalette.vue'
import ShellRouterView from '@/components/shell/ShellRouterView.vue'
import { isPaletteShortcut, paletteShortcutLabel, usePalette } from '@/components/shell/usePalette'
import UiIcon from '@/ui/UiIcon.vue'
import UiIconButton from '@/ui/UiIconButton.vue'

const SIDEBAR_RAIL_KEY = 'admin.sidebar.collapsed'

const route = useRoute()
const userStore = useUserStore()
const { t } = useAppI18n()
const { groups, active } = useAdminMenu()
const { open: paletteOpen, openPalette } = usePalette()

const drawerMode = useMediaQuery(NARROW_QUERY)
const sidebarOpen = ref(false)
const sidebarCollapsed = ref(readRailPreference())
const aboutOpen = ref(false)
const menuButton = ref(null)
const closeButton = ref(null)
const sidebarElement = ref(null)
const shortcutLabel = paletteShortcutLabel()

const railActive = computed(() => !drawerMode.value && sidebarCollapsed.value)
const drawerInactive = computed(() => drawerMode.value && !sidebarOpen.value)
const forwardSuite = computed(() => isForwardSuitePath(route.path))

const toggleIcon = computed(() => {
  if (drawerMode.value) return Menu
  return sidebarCollapsed.value ? PanelLeftOpen : PanelLeftClose
})
const toggleLabel = computed(() => {
  if (drawerMode.value) return t('common.a11y.openNavigation')
  return sidebarCollapsed.value ? t('shell.admin.expand') : t('shell.admin.collapse')
})

const pageTitle = computed(() => {
  if (route.meta?.titleKey) return t(route.meta.titleKey)
  const extensionMenu = adminExtensionMenus.value.find(item => item.to === route.path && userStore.hasPermission(item.permission))
  return extensionMenu?.label ||
    resolveRoutePageTitle(t, route.path, '') ||
    active.value?.item.label ||
    t('pageTitles.admin.fallback')
})

// group › menu item (when the page is below it) › page title.
const crumbs = computed(() => {
  const out = []
  const hit = active.value
  if (hit) {
    out.push({ label: hit.group.label })
    if (hit.item.to !== route.path && hit.item.label !== pageTitle.value) {
      out.push({ label: hit.item.label, to: hit.item.to })
    }
  }
  out.push({ label: pageTitle.value })
  return out
})

function readRailPreference() {
  try {
    return localStorage.getItem(SIDEBAR_RAIL_KEY) === 'true'
  } catch {
    return false
  }
}

function focusElement(target) {
  const element = target?.$el || target
  element?.focus?.()
}

function closeSidebar() {
  const restoreFocus = drawerMode.value && sidebarOpen.value
  sidebarOpen.value = false
  if (restoreFocus) {
    nextTick(() => focusElement(menuButton.value))
  }
}

function openSidebar() {
  sidebarOpen.value = true
  nextTick(() => focusElement(closeButton.value))
}

function toggleSidebar() {
  if (drawerMode.value) {
    if (sidebarOpen.value) closeSidebar()
    else openSidebar()
    return
  }
  sidebarCollapsed.value = !sidebarCollapsed.value
  try {
    localStorage.setItem(SIDEBAR_RAIL_KEY, String(sidebarCollapsed.value))
  } catch {
    // Blocked storage only forgets the preference.
  }
}

function onNavigate() {
  if (drawerMode.value) closeSidebar()
}

function drawerFocusables() {
  const selector = 'a[href], button:not([disabled]), input:not([disabled]), [tabindex]:not([tabindex="-1"])'
  return [...(sidebarElement.value?.querySelectorAll(selector) || [])].filter(element => (
    !element.hasAttribute('hidden') &&
    element.getAttribute('aria-hidden') !== 'true' &&
    !element.closest('[inert]')
  ))
}

// The open drawer is modal: Esc closes it, Tab and Shift+Tab stay inside.
function handleDrawerKeydown(event) {
  if (!drawerMode.value || !sidebarOpen.value) return
  if (event.key === 'Escape') {
    event.preventDefault()
    closeSidebar()
    return
  }
  if (event.key !== 'Tab') return
  const focusables = drawerFocusables()
  if (focusables.length === 0) return
  const first = focusables[0]
  const last = focusables.at(-1)
  if (event.shiftKey && document.activeElement === first) {
    event.preventDefault()
    last.focus()
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault()
    first.focus()
  }
}

watch(drawerMode, isDrawer => {
  if (!isDrawer && sidebarOpen.value) sidebarOpen.value = false
})

watch(() => route.path, () => {
  if (drawerMode.value && sidebarOpen.value) sidebarOpen.value = false
})

function onGlobalKeydown(event) {
  if (isPaletteShortcut(event)) {
    event.preventDefault()
    paletteOpen.value = !paletteOpen.value
  }
}

onMounted(() => {
  void loadEdition()
  window.addEventListener('keydown', onGlobalKeydown)
  if (userStore.isLoggedIn) {
    userStore.getUserInfo()
  }
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onGlobalKeydown)
  paletteOpen.value = false
})
</script>

<style scoped>
.admin-shell {
  --admin-sidebar-width: 248px;
  --admin-rail-width: 64px;
  /* Sticky table headers (UiDataTable) stop under the top bar. */
  --shell-topbar-height: 52px;

  display: grid;
  grid-template-columns: var(--admin-sidebar-width) minmax(0, 1fr);
  min-height: 100vh;
  min-height: 100dvh;
  background: var(--bg);
}

.admin-shell.is-rail {
  grid-template-columns: var(--admin-rail-width) minmax(0, 1fr);
}

.admin-shell.is-drawer {
  grid-template-columns: minmax(0, 1fr);
}

/* Sidebar: light frosted material (dark in dark mode); opaque where
   backdrop-filter is missing or less transparency is asked for. */
.admin-sidebar {
  position: sticky;
  top: 0;
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  height: 100vh;
  height: 100dvh;
  padding: var(--space-4) var(--space-3) var(--space-3);
  overflow: hidden;
  border-right: 1px solid var(--separator);
  background: var(--bg-grouped);
}

@supports ((backdrop-filter: blur(1px)) or (-webkit-backdrop-filter: blur(1px))) {
  .admin-sidebar {
    background: var(--material-sidebar);
    -webkit-backdrop-filter: saturate(180%) blur(20px);
    backdrop-filter: saturate(180%) blur(20px);
  }

  .admin-topbar {
    background: var(--material);
    -webkit-backdrop-filter: saturate(180%) blur(20px);
    backdrop-filter: saturate(180%) blur(20px);
  }
}

@media (prefers-reduced-transparency: reduce) {
  .admin-sidebar {
    background: var(--bg-grouped);
  }

  .admin-topbar {
    background: var(--bg);
  }
}

.admin-sidebar__top {
  display: flex;
  gap: var(--space-2);
  align-items: center;
  justify-content: space-between;
  min-height: 36px;
  padding: 0 var(--space-3);
}

.admin-sidebar__brand {
  display: inline-flex;
  min-width: 0;
  color: inherit;
  text-decoration: none;
  border-radius: var(--radius-xs);
}

.admin-sidebar__nav {
  flex: 1;
  min-height: 0;
  margin: 0 calc(var(--space-1) * -1);
  padding: 0 var(--space-1);
  overflow-y: auto;
  overscroll-behavior: contain;
  scrollbar-width: thin;
}

.admin-sidebar__footer {
  padding-top: var(--space-2);
  border-top: 1px solid var(--separator);
}

.admin-shell.is-rail .admin-sidebar {
  padding-inline: var(--space-2);
}

.admin-shell.is-rail .admin-sidebar__top {
  justify-content: center;
  padding: 0;
}

.admin-shell.is-rail .admin-sidebar__footer :deep(.account-trigger) {
  justify-content: center;
  padding-inline: 0;
}

/* Workspace and top bar */
.admin-workspace {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.admin-topbar {
  position: sticky;
  top: 0;
  z-index: var(--z-sticky);
  display: flex;
  gap: var(--space-3);
  align-items: center;
  height: var(--shell-topbar-height);
  padding: 0 var(--space-6) 0 var(--space-4);
  border-bottom: 1px solid var(--separator);
  background: var(--bg);
}

.admin-topbar__toggle {
  flex: none;
}

.admin-crumbs {
  min-width: 0;
}

.admin-crumbs ol {
  display: flex;
  gap: var(--space-2);
  align-items: center;
  min-width: 0;
  list-style: none;
  color: var(--label-2);
  font-size: var(--type-body-size);
  white-space: nowrap;
}

.admin-crumbs li {
  display: flex;
  gap: var(--space-2);
  align-items: center;
  min-width: 0;
}

.admin-crumbs li + li::before {
  content: '›';
  color: var(--label-3);
}

.admin-crumbs li.is-current {
  overflow: hidden;
  color: var(--label-1);
  font-weight: var(--weight-medium);
}

.admin-crumbs li.is-current span {
  overflow: hidden;
  text-overflow: ellipsis;
}

.admin-crumbs a {
  color: inherit;
  text-decoration: none;
}

.admin-crumbs a:hover {
  color: var(--label-1);
}

.admin-search {
  display: inline-flex;
  flex: none;
  gap: var(--space-2);
  align-items: center;
  width: 260px;
  height: 32px;
  margin-left: auto;
  padding: 0 var(--space-2) 0 var(--space-3);
  border: 0;
  border-radius: var(--radius-sm);
  /* An elevated field with a hairline: label-2 on a fill-1 field is under
     4.5:1 in light mode. */
  background: var(--bg-elevated);
  box-shadow: inset 0 0 0 1px var(--separator);
  color: var(--label-2);
  font: inherit;
  font-size: var(--type-callout-size);
  cursor: pointer;
  transition: background-color var(--dur-micro) var(--ease-standard);
}

.admin-search:hover {
  box-shadow: inset 0 0 0 1px var(--separator-strong);
}

.admin-search__text {
  flex: 1;
  text-align: left;
}

.admin-search__kbd {
  display: inline-grid;
  place-items: center;
  min-width: 20px;
  height: 20px;
  padding: 0 var(--space-1);
  border: 1px solid var(--separator);
  border-radius: var(--radius-xs);
  color: var(--label-2);
  font-family: var(--font-sans);
  font-size: var(--type-caption-size);
}

.admin-topbar__account {
  display: flex;
  flex: none;
}

/* Content */
.admin-main {
  flex: 1;
}

.admin-main:focus {
  outline: none;
}

.admin-content {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
  width: 100%;
  max-width: var(--size-content-admin);
  margin: 0 auto;
  padding: var(--space-8) var(--space-8) var(--space-16);
}

.admin-content.is-wide {
  max-width: var(--size-content-wide);
}

.admin-content__suite-nav {
  align-self: flex-start;
}

/* Drawer below 834 px */
.admin-scrim {
  display: none;
}

.admin-shell.is-drawer .admin-scrim {
  position: fixed;
  inset: 0;
  z-index: var(--z-drawer);
  display: block;
  background: var(--scrim);
  opacity: 0;
  pointer-events: none;
  transition: opacity var(--dur-overlay) var(--ease-standard);
}

.admin-shell.is-drawer .admin-scrim.is-open {
  opacity: 1;
  pointer-events: auto;
}

.admin-shell.is-drawer .admin-sidebar {
  position: fixed;
  inset: 0 auto 0 0;
  z-index: calc(var(--z-drawer) + 1);
  width: min(88vw, 300px);
  padding-bottom: calc(var(--space-3) + env(safe-area-inset-bottom, 0px));
  box-shadow: var(--shadow-3);
  transform: translateX(-100%);
  visibility: hidden;
  transition:
    transform var(--dur-overlay) var(--ease-emphasized),
    visibility 0s linear var(--dur-overlay);
}

.admin-shell.is-drawer .admin-sidebar.is-open {
  transform: none;
  visibility: visible;
  transition: transform var(--dur-overlay) var(--ease-emphasized);
}

.admin-shell.is-drawer .admin-topbar {
  gap: var(--space-2);
  padding: 0 var(--space-3);
}

.admin-shell.is-drawer .admin-crumbs li:not(.is-current) {
  display: none;
}

.admin-shell.is-drawer .admin-crumbs li.is-current::before {
  content: none;
}

.admin-shell.is-drawer .admin-search {
  justify-content: center;
  width: var(--size-control-md);
  height: var(--size-control-md);
  padding: 0;
  border-radius: 50%;
  background: transparent;
  box-shadow: none;
}

.admin-shell.is-drawer .admin-search:hover {
  background: var(--fill-1);
}

.admin-shell.is-drawer .admin-search__text,
.admin-shell.is-drawer .admin-search__kbd {
  display: none;
}

.admin-shell.is-drawer .admin-content {
  padding: var(--space-6) var(--space-4) var(--space-12);
}

@media (max-width: 1067.98px) {
  .admin-search {
    width: 200px;
  }
}

@media (pointer: coarse) {
  .admin-shell.is-drawer .admin-search {
    width: 44px;
    height: 44px;
  }
}
</style>
