<template>
  <div class="user-shell" :class="{ 'is-narrow': narrow }">
    <header class="user-bar">
      <div class="user-bar__inner">
        <router-link to="/user/dashboard" class="user-bar__brand">
          <BrandLockup size="sm" />
        </router-link>

        <nav v-if="!narrow" class="user-bar__links" :aria-label="t('shell.user.navLabel')" data-user-nav="top">
          <router-link
            v-for="item in items"
            :key="item.id"
            :to="item.to"
            class="user-bar__link"
            :class="{ 'is-active': item.id === activeId }"
            active-class=""
            exact-active-class=""
            :aria-current="item.id === activeId ? 'page' : undefined"
            :data-nav-item="item.id"
          >
            {{ item.label }}
          </router-link>
        </nav>

        <div class="user-bar__account">
          <AccountMenu
            variant="avatar"
            role="user"
            :account-path="USER_ACCOUNT_PATH"
            :extra-links="narrow ? overflowItems : []"
          />
        </div>
      </div>
    </header>

    <main id="app-main-content" class="user-main" tabindex="-1" :aria-label="pageTitle">
      <ShellRouterView />
    </main>

    <nav v-if="narrow" class="user-tabbar" :aria-label="t('shell.user.navLabel')" data-user-nav="tabs">
      <ul class="user-tabbar__list">
        <li v-for="item in tabItems" :key="item.id">
          <router-link
            :to="item.to"
            class="user-tab"
            :class="{ 'is-active': item.id === activeId }"
            active-class=""
            exact-active-class=""
            :aria-current="item.id === activeId ? 'page' : undefined"
            :data-nav-item="item.id"
          >
            <UiIcon :icon="navIcon(item.icon)" :size="24" />
            <span class="user-tab__label">{{ item.shortLabel }}</span>
          </router-link>
        </li>
      </ul>
    </nav>
  </div>
</template>

<script setup>
// User shell (plan §8.3, D4). Wide screens: a 48 px frosted bar with the
// lockup, centred links and the account menu. Below 834 px: the bar keeps
// the lockup and the avatar, and a bottom tab bar (概览 / 订阅 / 帮助 / 工单 /
// 账户) with safe-area padding takes the navigation; commercial pages that
// do not fit move into the account menu. Theme and language are in the
// account menu. Items come from navigation/menu.js.
import { computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { useUserStore } from '@/stores/user'
import { useAppI18n } from '@/composables/useAppI18n'
import { useMediaQuery, NARROW_QUERY } from '@/composables/useMediaQuery'
import { loadEdition } from '@/composables/useEdition'
import { resolveRoutePageTitle } from '@/utils/pageMeta'
import { useUserMenu } from '@/navigation/useNavigation'
import { USER_ACCOUNT_PATH } from '@/navigation/menu'
import { navIcon } from '@/navigation/icons'
import BrandLockup from '@/components/common/BrandLockup.vue'
import AccountMenu from '@/components/shell/AccountMenu.vue'
import ShellRouterView from '@/components/shell/ShellRouterView.vue'
import UiIcon from '@/ui/UiIcon.vue'

const route = useRoute()
const userStore = useUserStore()
const { t } = useAppI18n()
const narrow = useMediaQuery(NARROW_QUERY)
const { items, activeId } = useUserMenu()

const tabItems = computed(() => items.value.filter(item => item.tab))
const overflowItems = computed(() => items.value.filter(item => !item.tab))
const pageTitle = computed(() => resolveRoutePageTitle(t, route.path, t('layout.user.brand')))

onMounted(() => {
  void loadEdition()
  if (userStore.isLoggedIn) {
    userStore.getUserInfo()
  }
})
</script>

<style scoped>
.user-shell {
  display: flex;
  flex-direction: column;
  min-height: 100vh;
  min-height: 100dvh;
  background: var(--bg);
}

.user-bar {
  position: sticky;
  top: 0;
  z-index: var(--z-sticky);
  border-bottom: 1px solid var(--separator);
  background: var(--bg);
}

@supports ((backdrop-filter: blur(1px)) or (-webkit-backdrop-filter: blur(1px))) {
  .user-bar,
  .user-tabbar {
    background: var(--material);
    -webkit-backdrop-filter: saturate(180%) blur(20px);
    backdrop-filter: saturate(180%) blur(20px);
  }
}

@media (prefers-reduced-transparency: reduce) {
  .user-bar,
  .user-tabbar {
    background: var(--bg);
  }
}

.user-bar__inner {
  display: flex;
  gap: var(--space-8);
  align-items: center;
  max-width: var(--size-content-user);
  height: 48px;
  margin: 0 auto;
  padding: 0 var(--space-6);
}

.user-bar__brand {
  display: inline-flex;
  flex: none;
  color: inherit;
  text-decoration: none;
}

.user-bar__links {
  display: flex;
  gap: var(--space-6);
  margin: 0 auto;
}

.user-bar__link {
  color: var(--label-2);
  font-size: var(--type-callout-size);
  text-decoration: none;
  white-space: nowrap;
  transition: color var(--dur-micro) var(--ease-standard);
}

.user-bar__link:hover,
.user-bar__link.is-active {
  color: var(--label-1);
}

.user-bar__link.is-active {
  font-weight: var(--weight-medium);
}

.user-bar__account {
  display: flex;
  flex: none;
}

.user-main {
  display: flex;
  flex: 1;
  flex-direction: column;
  width: 100%;
  max-width: var(--size-content-user);
  margin: 0 auto;
  padding: var(--space-12) var(--space-6) var(--space-16);
}

.user-main:focus {
  outline: none;
}

.user-tabbar {
  position: fixed;
  right: 0;
  bottom: 0;
  left: 0;
  z-index: var(--z-sticky);
  padding-bottom: env(safe-area-inset-bottom, 0px);
  border-top: 1px solid var(--separator);
  background: var(--bg);
}

.user-tabbar__list {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(0, 1fr));
  list-style: none;
}

.user-tab {
  display: flex;
  flex-direction: column;
  gap: var(--space-0-5);
  align-items: center;
  justify-content: center;
  min-height: 52px;
  padding: var(--space-1) 0 var(--space-2);
  color: var(--label-2);
  font-size: var(--type-caption-size);
  font-weight: var(--weight-medium);
  line-height: 1.2;
  text-decoration: none;
}

.user-tab.is-active {
  color: var(--accent);
}

.user-tab:focus-visible {
  outline-offset: -4px;
}

.user-tab__label {
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.user-shell.is-narrow .user-bar__inner {
  justify-content: space-between;
  padding: 0 var(--space-4);
}

.user-shell.is-narrow .user-main {
  padding: var(--space-6) var(--space-4) calc(var(--space-10) + 60px + env(safe-area-inset-bottom, 0px));
}

@media (forced-colors: active) {
  .user-bar__link.is-active,
  .user-tab.is-active {
    text-decoration: underline;
  }
}
</style>
