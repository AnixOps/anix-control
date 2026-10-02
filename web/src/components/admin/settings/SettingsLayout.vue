<template>
  <div class="settings-layout" :class="{ 'is-narrow': narrow }">
    <nav v-if="showNav" class="settings-layout__nav" :aria-label="navLabel" data-test="settings-nav">
      <ul class="settings-layout__list">
        <li v-for="section in sections" :key="section.id">
          <router-link
            :to="section.to"
            class="settings-layout__link"
            :class="{ 'is-active': section.id === current }"
            :aria-current="section.id === current ? 'page' : undefined"
            :data-settings-section="section.id"
          >
            <span class="settings-layout__icon" aria-hidden="true"><UiIcon :icon="section.icon" :size="16" /></span>
            <span class="settings-layout__label">{{ section.label }}</span>
            <UiIcon v-if="narrow" :icon="ChevronRight" class="settings-layout__chevron" />
          </router-link>
        </li>
      </ul>
    </nav>
    <div v-if="showContent" class="settings-layout__content">
      <router-link v-if="narrow" :to="backTo" class="settings-layout__back" data-test="settings-back">
        <UiIcon :icon="ChevronLeft" />
        {{ backLabel }}
      </router-link>
      <slot />
    </div>
  </div>
</template>

<script setup>
// The settings page template (plan §7.3): a list of sections on the left and
// the current section on the right. Below 834 px it works like iOS
// Settings: the page without a section shows only the list, a section shows
// only its content with a back link to the list. Sections are links, so
// the section lives in the URL and the browser's back button works.
import { computed } from 'vue'
import { ChevronLeft, ChevronRight } from '@lucide/vue'
import { NARROW_QUERY, useMediaQuery } from '@/composables/useMediaQuery'
import UiIcon from '@/ui/UiIcon.vue'

const props = defineProps({
  // [{ id, label, icon, to }]
  sections: { type: Array, required: true },
  // The id of the section shown; '' shows only the list on phones.
  current: { type: String, default: '' },
  navLabel: { type: String, required: true },
  backLabel: { type: String, required: true },
  backTo: { type: [String, Object], required: true }
})

const narrow = useMediaQuery(NARROW_QUERY)
const showNav = computed(() => !narrow.value || !props.current)
const showContent = computed(() => !narrow.value || Boolean(props.current))

defineExpose({ narrow })
</script>

<style scoped>
.settings-layout {
  display: grid;
  grid-template-columns: 220px minmax(0, 1fr);
  gap: var(--space-8);
  align-items: start;
}

.settings-layout.is-narrow {
  grid-template-columns: minmax(0, 1fr);
}

.settings-layout__nav {
  position: sticky;
  top: calc(var(--shell-topbar-height, 52px) + var(--space-4));
}

.settings-layout__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-0-5);
  margin: 0;
  padding: 0;
  list-style: none;
}

.settings-layout__link {
  display: flex;
  gap: var(--space-3);
  align-items: center;
  min-height: 36px;
  padding: var(--space-1) var(--space-2);
  border-radius: var(--radius-sm);
  color: var(--label-1);
  font-size: var(--type-body-size);
  text-decoration: none;
  transition: background-color var(--dur-micro) var(--ease-standard);
}

.settings-layout__link:hover {
  background: var(--fill-1);
}

.settings-layout__link.is-active {
  background: var(--fill-2);
  font-weight: var(--weight-medium);
}

.settings-layout__link:focus-visible {
  outline: var(--focus-ring);
  outline-offset: var(--focus-ring-offset);
}

.settings-layout__icon {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: var(--radius-xs);
  background: var(--fill-1);
  color: var(--label-2);
}

.settings-layout__link.is-active .settings-layout__icon {
  background: var(--accent-soft);
  color: var(--accent);
}

.settings-layout__label {
  flex: 1;
  min-width: 0;
}

.settings-layout__chevron {
  color: var(--label-3);
}

.settings-layout__content {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
  min-width: 0;
}

.settings-layout__back {
  display: inline-flex;
  gap: var(--space-1);
  align-items: center;
  align-self: flex-start;
  margin-bottom: calc(-1 * var(--space-4));
  border-radius: var(--radius-xs);
  color: var(--accent);
  font-size: var(--type-body-size);
  text-decoration: none;
}

.settings-layout__back:focus-visible {
  outline: var(--focus-ring);
  outline-offset: var(--focus-ring-offset);
}

/* Phones: the list is a grouped list of rows with chevrons. */
.is-narrow .settings-layout__nav {
  position: static;
}

.is-narrow .settings-layout__list {
  gap: 0;
  overflow: hidden;
  border-radius: var(--radius-md);
  background: var(--bg-elevated);
  box-shadow: 0 0 0 0.5px var(--separator);
}

.is-narrow .settings-layout__list li + li {
  border-top: 1px solid var(--separator);
}

.is-narrow .settings-layout__link {
  min-height: 52px;
  padding: var(--space-2) var(--space-4);
  border-radius: 0;
}

.is-narrow .settings-layout__link:focus-visible {
  outline-offset: -3px;
}

/* Touch: a 44 px hit area around the link without changing the look. */
@media (pointer: coarse) {
  .settings-layout__back {
    position: relative;
  }

  .settings-layout__back::after {
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
