<template>
  <DropdownMenuRoot v-model:open="open" :modal="false">
    <DropdownMenuTrigger
      class="account-trigger"
      :class="`account-trigger--${variant}`"
      :aria-label="t('shell.account.menuLabel', { name: displayName })"
      :title="variant === 'row' && !compact ? undefined : displayName"
      data-account-menu-trigger
    >
      <ShellAvatar :name="avatarSource" :size="variant === 'avatar' ? 'sm' : 'md'" />
      <span v-if="variant === 'row' && !compact" class="account-trigger__who">
        <span class="account-trigger__name">{{ displayName }}</span>
        <span class="account-trigger__email">{{ email || '—' }}</span>
      </span>
      <UiIcon v-if="variant === 'row' && !compact" :icon="ChevronsUpDown" class="account-trigger__chevron" />
    </DropdownMenuTrigger>

    <DropdownMenuPortal :to="layer">
      <DropdownMenuContent
        class="shell-menu"
        :side="variant === 'row' ? 'top' : 'bottom'"
        :align="variant === 'row' ? 'start' : 'end'"
        :side-offset="6"
        :collision-padding="12"
        data-account-menu
      >
        <DropdownMenuLabel class="shell-menu__header">
          <span class="shell-menu__header-name">{{ displayName }}</span>
          <span v-if="email && email !== displayName" class="shell-menu__header-meta">{{ email }}</span>
        </DropdownMenuLabel>
        <DropdownMenuSeparator class="shell-menu__separator" />

        <DropdownMenuItem as-child>
          <router-link class="shell-menu__item" :to="accountPath" data-menu-item="account">
            <UiIcon :icon="UserRound" />
            <span class="shell-menu__label">{{ t('shell.account.account') }}</span>
          </router-link>
        </DropdownMenuItem>

        <DropdownMenuItem v-for="link in extraLinks" :key="link.to" as-child>
          <router-link class="shell-menu__item" :to="link.to" :data-menu-item="link.id">
            <UiIcon :icon="navIcon(link.icon)" />
            <span class="shell-menu__label">{{ link.label }}</span>
          </router-link>
        </DropdownMenuItem>

        <!-- Phones: the choices sit in the menu itself (no flyout submenus). -->
        <template v-if="inlineChoices">
          <DropdownMenuSeparator class="shell-menu__separator" />
          <DropdownMenuLabel class="shell-menu__group-label">{{ t('shell.account.appearance') }}</DropdownMenuLabel>
          <DropdownMenuRadioGroup :model-value="themePreference" @update:model-value="setThemePreference">
            <DropdownMenuRadioItem
              v-for="option in themeOptions"
              :key="option.value"
              class="shell-menu__item"
              :value="option.value"
              :data-theme-option="option.value"
            >
              <UiIcon :icon="option.icon" />
              <span class="shell-menu__label">{{ option.label }}</span>
              <DropdownMenuItemIndicator class="shell-menu__check">
                <UiIcon :icon="Check" />
              </DropdownMenuItemIndicator>
            </DropdownMenuRadioItem>
          </DropdownMenuRadioGroup>
          <DropdownMenuSeparator class="shell-menu__separator" />
          <DropdownMenuLabel class="shell-menu__group-label">{{ t('shell.account.language') }}</DropdownMenuLabel>
          <DropdownMenuRadioGroup :model-value="currentLocale" @update:model-value="switchLocale">
            <DropdownMenuRadioItem
              v-for="option in localeOptions"
              :key="option.value"
              class="shell-menu__item"
              :value="option.value"
              :lang="option.value"
              :data-locale-option="option.value"
            >
              <span class="shell-menu__label">{{ option.nativeLabel }}</span>
              <DropdownMenuItemIndicator class="shell-menu__check">
                <UiIcon :icon="Check" />
              </DropdownMenuItemIndicator>
            </DropdownMenuRadioItem>
          </DropdownMenuRadioGroup>
          <DropdownMenuSeparator v-if="showAbout" class="shell-menu__separator" />
        </template>
        <template v-else>
          <DropdownMenuSub>
            <DropdownMenuSubTrigger class="shell-menu__item" data-menu-item="appearance">
              <UiIcon :icon="SunMoon" />
              <span class="shell-menu__label">{{ t('shell.account.appearance') }}</span>
              <span class="shell-menu__value">{{ t(`shell.account.themes.${themePreference}`) }}</span>
              <UiIcon :icon="ChevronRight" />
            </DropdownMenuSubTrigger>
            <DropdownMenuPortal :to="layer">
              <DropdownMenuSubContent class="shell-menu shell-menu--sub" :side-offset="4" :collision-padding="12">
                <DropdownMenuRadioGroup :model-value="themePreference" @update:model-value="setThemePreference">
                  <DropdownMenuRadioItem
                    v-for="option in themeOptions"
                    :key="option.value"
                    class="shell-menu__item"
                    :value="option.value"
                    :data-theme-option="option.value"
                  >
                    <UiIcon :icon="option.icon" />
                    <span class="shell-menu__label">{{ option.label }}</span>
                    <DropdownMenuItemIndicator class="shell-menu__check">
                      <UiIcon :icon="Check" />
                    </DropdownMenuItemIndicator>
                  </DropdownMenuRadioItem>
                </DropdownMenuRadioGroup>
              </DropdownMenuSubContent>
            </DropdownMenuPortal>
          </DropdownMenuSub>

          <DropdownMenuSub>
            <DropdownMenuSubTrigger class="shell-menu__item" data-menu-item="language">
              <UiIcon :icon="Languages" />
              <span class="shell-menu__label">{{ t('shell.account.language') }}</span>
              <span class="shell-menu__value">{{ currentLocaleLabel }}</span>
              <UiIcon :icon="ChevronRight" />
            </DropdownMenuSubTrigger>
            <DropdownMenuPortal :to="layer">
              <DropdownMenuSubContent class="shell-menu shell-menu--sub" :side-offset="4" :collision-padding="12">
                <DropdownMenuRadioGroup :model-value="currentLocale" @update:model-value="switchLocale">
                  <DropdownMenuRadioItem
                    v-for="option in localeOptions"
                    :key="option.value"
                    class="shell-menu__item"
                    :value="option.value"
                    :lang="option.value"
                    :data-locale-option="option.value"
                  >
                    <span class="shell-menu__label">{{ option.nativeLabel }}</span>
                    <DropdownMenuItemIndicator class="shell-menu__check">
                      <UiIcon :icon="Check" />
                    </DropdownMenuItemIndicator>
                  </DropdownMenuRadioItem>
                </DropdownMenuRadioGroup>
              </DropdownMenuSubContent>
            </DropdownMenuPortal>
          </DropdownMenuSub>

        </template>

        <DropdownMenuItem v-if="showAbout" class="shell-menu__item" data-menu-item="about" @select="emit('about')">
          <UiIcon :icon="Info" />
          <span class="shell-menu__label">{{ t('shell.account.about') }}</span>
        </DropdownMenuItem>

        <DropdownMenuSeparator class="shell-menu__separator" />
        <DropdownMenuItem class="shell-menu__item" data-menu-item="logout" @select="logout">
          <UiIcon :icon="LogOut" />
          <span class="shell-menu__label">{{ t('shell.account.logout') }}</span>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenuPortal>
  </DropdownMenuRoot>
</template>

<script setup>
// Account menu (plan §8.3): avatar → 账户, 外观, 语言, (关于), 退出登录.
// Reka DropdownMenu: Enter/Space/↓ open it, arrows move, → opens a
// submenu and ← closes it (below 834 px the choices are inline instead), typing jumps to an item, Esc closes and returns
// focus to the avatar. The theme and language submenus are radio groups.
// variant 'row' is the admin sidebar footer (avatar, name and email; just
// the avatar when the sidebar is a rail); 'avatar' is a top-bar button.
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  DropdownMenuContent, DropdownMenuItem, DropdownMenuItemIndicator, DropdownMenuLabel, DropdownMenuPortal,
  DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuRoot, DropdownMenuSeparator, DropdownMenuSub,
  DropdownMenuSubContent, DropdownMenuSubTrigger, DropdownMenuTrigger
} from 'reka-ui'
import { Check, ChevronRight, ChevronsUpDown, Info, Languages, LogOut, Monitor, Moon, Sun, SunMoon, UserRound } from '@lucide/vue'
import { useUserStore } from '@/stores/user'
import { useAppI18n } from '@/composables/useAppI18n'
import { useTheme } from '@/composables/useTheme'
import { useMediaQuery, NARROW_QUERY } from '@/composables/useMediaQuery'
import { navIcon } from '@/navigation/icons'
import UiIcon from '@/ui/UiIcon.vue'
import { useMenuLayer } from '@/ui/composables/useMenuLayer'
import ShellAvatar from './ShellAvatar.vue'

const props = defineProps({
  variant: { type: String, default: 'avatar', validator: value => ['row', 'avatar'].includes(value) },
  // The sidebar is an icon rail: show only the avatar.
  compact: { type: Boolean, default: false },
  role: { type: String, default: 'user', validator: value => ['admin', 'user'].includes(value) },
  accountPath: { type: String, required: true },
  showAbout: { type: Boolean, default: false },
  // Pages that have no other entry at this width (user plans and orders on
  // phones, where the tab bar holds five items).
  extraLinks: { type: Array, default: () => [] }
})

const emit = defineEmits(['about'])
const router = useRouter()
const userStore = useUserStore()
const { t, currentLocale, localeOptions, switchLocale } = useAppI18n()
const { themePreference, setThemePreference } = useTheme()
const open = ref(false)
// The open menu and its submenus sit in one labelled region, not on <body>.
const layer = useMenuLayer(open, () => t('shell.account.menuLabel', { name: displayName.value }))
// Flyout submenus are awkward on a phone: show the choices inline there.
const inlineChoices = useMediaQuery(NARROW_QUERY)

const email = computed(() => userStore.userInfo?.email || '')
const displayName = computed(() => (props.role === 'admin' ? t('shell.account.roleAdmin') : (email.value || t('shell.account.roleUser'))))
const avatarSource = computed(() => email.value || displayName.value)

const themeOptions = computed(() => [
  { value: 'system', label: t('shell.account.themes.system'), icon: Monitor },
  { value: 'light', label: t('shell.account.themes.light'), icon: Sun },
  { value: 'dark', label: t('shell.account.themes.dark'), icon: Moon }
])

const currentLocaleLabel = computed(() => (
  localeOptions.value.find(option => option.value === currentLocale.value)?.nativeLabel || currentLocale.value
))

function logout() {
  userStore.logout()
  router.push('/login')
}
</script>

<style src="./menu.css"></style>

<style scoped>
.account-trigger {
  display: inline-flex;
  gap: var(--space-3);
  align-items: center;
  min-width: 0;
  border: 0;
  background: transparent;
  color: var(--label-1);
  font: inherit;
  text-align: left;
  cursor: pointer;
  transition: background-color var(--dur-micro) var(--ease-standard);
}

.account-trigger--row {
  width: 100%;
  min-height: 44px;
  padding: var(--space-1) var(--space-2);
  border-radius: var(--radius-sm);
}

/* Hover lifts the row onto an elevated surface (label-2 on a fill-1 row is
   under 4.5:1 in light mode). */
.account-trigger--row:hover,
.account-trigger--row[data-state='open'] {
  background: var(--bg-elevated);
  box-shadow: var(--shadow-1), 0 0 0 0.5px var(--separator);
}

.account-trigger--avatar {
  justify-content: center;
  width: var(--size-control-md);
  height: var(--size-control-md);
  border-radius: 50%;
}

.account-trigger--avatar:hover :deep(.shell-avatar),
.account-trigger--avatar[data-state='open'] :deep(.shell-avatar) {
  background: var(--fill-3);
}

.account-trigger__who {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-width: 0;
  line-height: var(--type-callout-line);
}

.account-trigger__name {
  overflow: hidden;
  font-size: var(--type-callout-size);
  font-weight: var(--weight-semibold);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.account-trigger__email {
  overflow: hidden;
  color: var(--label-2);
  font-size: var(--type-caption-size);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.account-trigger__chevron {
  color: var(--label-3);
}

@media (pointer: coarse) {
  .account-trigger--avatar {
    width: 44px;
    height: 44px;
  }
}
</style>
