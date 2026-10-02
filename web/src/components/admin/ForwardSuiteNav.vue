<template>
  <nav class="forward-suite-nav" :aria-label="t('shell.forwardNav.label')" data-forward-suite-nav>
    <ul class="forward-suite-list">
      <li v-for="link in coreLinks" :key="link.to" class="forward-suite-item">
        <router-link
          :to="link.to"
          class="forward-suite-link"
          active-class=""
          exact-active-class="is-active"
          :title="link.hint || undefined"
        >
          {{ link.label }}
        </router-link>
      </li>
      <li class="forward-suite-item">
        <DropdownMenuRoot :modal="false">
          <DropdownMenuTrigger
            class="forward-suite-link forward-suite-more"
            :class="{ 'is-active': activeAdvanced }"
            :aria-label="activeAdvanced ? `${t('shell.forwardNav.moreLabel')}: ${activeAdvanced.label}` : t('shell.forwardNav.moreLabel')"
            data-forward-more
          >
            <span>{{ activeAdvanced ? activeAdvanced.label : t('shell.forwardNav.more') }}</span>
            <UiIcon :icon="ChevronDown" />
          </DropdownMenuTrigger>
          <DropdownMenuPortal>
            <DropdownMenuContent class="shell-menu" align="end" :side-offset="6" :collision-padding="12">
              <DropdownMenuItem v-for="link in advancedLinks" :key="link.to" as-child>
                <router-link
                  :to="link.to"
                  class="shell-menu__item forward-suite-menu-item"
                  :class="{ 'is-current': link.to === currentPath }"
                  active-class=""
                  exact-active-class=""
                  :aria-current="link.to === currentPath ? 'page' : undefined"
                >
                  <UiIcon :icon="navIcon(link.icon)" />
                  <span class="forward-suite-menu-text">
                    <span class="shell-menu__label">{{ link.label }}</span>
                    <span v-if="link.hint" class="forward-suite-menu-hint">{{ link.hint }}</span>
                  </span>
                </router-link>
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenuPortal>
        </DropdownMenuRoot>
      </li>
    </ul>
  </nav>
</template>

<script setup>
// The forward suite's own navigation (flux-panel clone): 快速配置向导,
// 流量转发, 隧道, 限速, NodeX 拓扑, and "更多" for the runtime tools. The
// admin shell renders it once, above every /admin/forward* page; the pages
// and the sidebar no longer repeat it. Links are real links (aria-current on
// the current page); "更多" is a Reka DropdownMenu (arrows, Esc) that shows
// the current page's name when it is one of its items.
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { DropdownMenuContent, DropdownMenuItem, DropdownMenuPortal, DropdownMenuRoot, DropdownMenuTrigger } from 'reka-ui'
import { ChevronDown } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { FORWARD_SUITE_LINKS } from '@/navigation/menu'
import { navIcon } from '@/navigation/icons'
import UiIcon from '@/ui/UiIcon.vue'

const { t } = useAppI18n()
const route = useRoute()
const currentPath = computed(() => route?.path || '')

function present(link) {
  return { ...link, label: t(link.labelKey), hint: link.hintKey ? t(link.hintKey) : '' }
}

const coreLinks = computed(() => FORWARD_SUITE_LINKS.core.map(present))
const advancedLinks = computed(() => FORWARD_SUITE_LINKS.advanced.map(present))
const activeAdvanced = computed(() => advancedLinks.value.find(link => link.to === currentPath.value) || null)
</script>

<style src="../shell/menu.css"></style>

<style scoped>
.forward-suite-nav {
  max-width: 100%;
  overflow-x: auto;
  scrollbar-width: none;
}

.forward-suite-nav::-webkit-scrollbar {
  display: none;
}

.forward-suite-list {
  display: inline-flex;
  gap: var(--space-0-5);
  padding: var(--space-0-5);
  border-radius: var(--radius-pill);
  background: var(--fill-1);
  list-style: none;
}

.forward-suite-item {
  display: flex;
  flex: none;
}

.forward-suite-link {
  display: inline-flex;
  gap: var(--space-1);
  align-items: center;
  height: 30px;
  padding: 0 var(--space-4);
  border: 0;
  border-radius: var(--radius-pill);
  background: transparent;
  /* label-1: label-2 on the fill-1 track is under 4.5:1 in light mode. The
     selected segment is the raised pill. */
  color: var(--label-1);
  font: inherit;
  font-size: var(--type-callout-size);
  font-weight: var(--weight-regular);
  text-decoration: none;
  white-space: nowrap;
  cursor: pointer;
  transition:
    background-color var(--dur-toggle) var(--ease-standard),
    color var(--dur-toggle) var(--ease-standard),
    box-shadow var(--dur-toggle) var(--ease-standard);
}

.forward-suite-link:hover:not(.is-active) {
  background: var(--fill-1);
}

.forward-suite-link.is-active {
  background: var(--bg-elevated);
  font-weight: var(--weight-medium);
  box-shadow: var(--shadow-1), 0 0 0 0.5px var(--separator);
}

.forward-suite-link:focus-visible {
  outline: var(--focus-ring);
  outline-offset: 0;
}

.forward-suite-more {
  padding-right: var(--space-3);
}

.forward-suite-menu-item {
  min-height: 44px;
  padding-block: var(--space-1);
}

.forward-suite-menu-text {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-width: 0;
}

.forward-suite-menu-hint {
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

.forward-suite-menu-item[data-highlighted] .forward-suite-menu-hint {
  color: var(--on-accent);
}

@media (pointer: coarse) {
  .forward-suite-link {
    height: 40px;
  }
}

@media (forced-colors: active) {
  .forward-suite-link.is-active {
    outline: 2px solid transparent;
    text-decoration: underline;
  }
}
</style>
