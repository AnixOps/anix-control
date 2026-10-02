<template>
  <nav class="forward-suite-nav" :aria-label="t('shell.forwardNav.label')" data-forward-suite-nav>
    <ul class="forward-suite-row">
      <li v-for="link in leadingLinks" :key="link.to" class="forward-suite-item">
        <router-link
          :to="link.to"
          class="forward-suite-tool forward-suite-tool--accent"
          active-class=""
          exact-active-class="is-active"
          :title="link.hint || undefined"
        >
          <UiIcon :icon="navIcon(link.icon)" :size="16" />
          <span>{{ link.label }}</span>
        </router-link>
      </li>
      <li class="forward-suite-item">
        <ul class="forward-suite-segments">
          <li v-for="link in segmentLinks" :key="link.to" class="forward-suite-item">
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
        </ul>
      </li>
      <li v-for="link in trailingLinks" :key="link.to" class="forward-suite-item">
        <router-link
          :to="link.to"
          class="forward-suite-tool"
          active-class=""
          exact-active-class="is-active"
          :title="link.hint || undefined"
        >
          <UiIcon :icon="navIcon(link.icon)" :size="16" />
          <span>{{ link.label }}</span>
        </router-link>
      </li>
      <li class="forward-suite-item">
        <DropdownMenuRoot :modal="false">
          <DropdownMenuTrigger
            class="forward-suite-tool forward-suite-more"
            :class="{ 'is-active': activeAdvanced }"
            :aria-label="activeAdvanced ? `${t('shell.forwardNav.moreLabel')}: ${activeAdvanced.label}` : t('shell.forwardNav.moreLabel')"
            data-forward-more
          >
            <span>{{ activeAdvanced ? activeAdvanced.label : t('shell.forwardNav.more') }}</span>
            <UiIcon :icon="ChevronDown" :size="16" />
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
// UI U7 (plan §8.2): the three flux pages (流量转发 / 隧道 / 限速) form the
// segmented control; the wizard, NodeX 拓扑 and 更多 sit around it as tools.
// Same links, same order, same routes.
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

const SEGMENT_IDS = ['forward-rules', 'forward-tunnel', 'forward-limit']
const coreLinks = computed(() => FORWARD_SUITE_LINKS.core.map(present))
const firstSegment = computed(() => coreLinks.value.findIndex(link => SEGMENT_IDS.includes(link.id)))
const segmentLinks = computed(() => coreLinks.value.filter(link => SEGMENT_IDS.includes(link.id)))
const leadingLinks = computed(() => coreLinks.value.filter((link, index) => !SEGMENT_IDS.includes(link.id) && index < firstSegment.value))
const trailingLinks = computed(() => coreLinks.value.filter((link, index) => !SEGMENT_IDS.includes(link.id) && index > firstSegment.value))
const advancedLinks = computed(() => FORWARD_SUITE_LINKS.advanced.map(present))
const activeAdvanced = computed(() => advancedLinks.value.find(link => link.to === currentPath.value) || null)
</script>

<style src="../shell/menu.css"></style>

<style scoped>
.forward-suite-nav {
  max-width: 100%;
}

.forward-suite-row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2) var(--space-3);
  align-items: center;
  margin: 0;
  padding: 0;
  list-style: none;
}

.forward-suite-item {
  display: flex;
  flex: none;
  min-width: 0;
}

/* The segmented control: 流量转发 / 隧道 / 限速. */
.forward-suite-segments {
  display: inline-flex;
  gap: var(--space-0-5);
  margin: 0;
  padding: var(--space-0-5);
  border-radius: var(--radius-pill);
  background: var(--fill-1);
  list-style: none;
}

.forward-suite-link {
  display: inline-flex;
  align-items: center;
  height: 30px;
  padding: 0 var(--space-4);
  border-radius: var(--radius-pill);
  /* label-1: label-2 on the fill-1 track is under 4.5:1 in light mode. The
     selected segment is the raised pill. */
  color: var(--label-1);
  font-size: var(--type-callout-size);
  font-weight: var(--weight-regular);
  text-decoration: none;
  white-space: nowrap;
  transition:
    background-color var(--dur-toggle) var(--ease-standard),
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

/* Tools around it: the wizard, NodeX 拓扑, 更多. */
.forward-suite-tool {
  display: inline-flex;
  gap: var(--space-1);
  align-items: center;
  height: 34px;
  padding: 0 var(--space-3);
  border: 0;
  border-radius: var(--radius-pill);
  background: transparent;
  color: var(--label-1);
  font: inherit;
  font-size: var(--type-callout-size);
  text-decoration: none;
  white-space: nowrap;
  cursor: pointer;
  transition: background-color var(--dur-toggle) var(--ease-standard);
}

.forward-suite-tool:hover,
.forward-suite-tool.is-active {
  background: var(--fill-1);
}

.forward-suite-tool.is-active {
  font-weight: var(--weight-medium);
}

.forward-suite-tool--accent {
  background: var(--accent-soft);
  /* As the badges: the accent mixed towards the label colour keeps 4.5:1
     on --accent-soft in both themes. */
  color: color-mix(in srgb, var(--accent) 78%, var(--label-1));
  font-weight: var(--weight-medium);
}

.forward-suite-tool--accent:hover,
.forward-suite-tool--accent.is-active {
  background: var(--accent-soft);
  box-shadow: inset 0 0 0 1px var(--accent);
}

.forward-suite-link:focus-visible,
.forward-suite-tool:focus-visible {
  outline: var(--focus-ring);
  outline-offset: var(--focus-ring-offset);
}

.forward-suite-more {
  padding-right: var(--space-2);
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

/* Phones: the segmented control takes a row of its own, full width. */
@media (max-width: 639.98px) {
  .forward-suite-item:has(> .forward-suite-segments) {
    flex: 1 1 100%;
  }

  .forward-suite-segments {
    display: flex;
    width: 100%;
  }

  .forward-suite-segments > .forward-suite-item {
    flex: 1 1 0;
  }

  .forward-suite-link {
    justify-content: center;
    width: 100%;
    padding: 0 var(--space-2);
  }
}

@media (pointer: coarse) {
  .forward-suite-link {
    height: 40px;
  }

  .forward-suite-tool {
    height: 44px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .forward-suite-link,
  .forward-suite-tool {
    transition: none;
  }
}

@media (forced-colors: active) {
  .forward-suite-link.is-active,
  .forward-suite-tool.is-active {
    outline: 2px solid transparent;
    text-decoration: underline;
  }
}

/* Touch: a 44 px hit area around the link without changing the look. */
@media (pointer: coarse) {
  .forward-suite-link {
    position: relative;
  }

  .forward-suite-link::after {
    position: absolute;
    top: 50%;
    left: 50%;
    width: max(100%, var(--size-control-lg));
    height: max(100%, var(--size-control-lg));
    transform: translate(-50%, -50%);
    content: '';
  }
}
</style>
