<template>
  <TabsRoot
    class="ui-tabs"
    :class="`ui-tabs--${variant}`"
    :model-value="modelValue"
    :activation-mode="activationMode"
    :unmount-on-hide="unmountOnHide"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <TabsList class="ui-tabs__list" :aria-label="ariaLabel || undefined" loop>
      <TabsTrigger
        v-for="item in normalizedItems"
        :key="String(item.value)"
        class="ui-tabs__tab"
        :value="item.value"
        :disabled="item.disabled"
      >
        <UiIcon v-if="item.icon" :icon="item.icon" />
        <span>{{ item.label }}</span>
        <span v-if="item.count !== undefined && item.count !== null" class="ui-tabs__count">{{ item.count }}</span>
      </TabsTrigger>
      <TabsIndicator v-if="variant === 'underline'" class="ui-tabs__indicator" />
    </TabsList>
    <TabsContent
      v-for="item in normalizedItems"
      :key="String(item.value)"
      class="ui-tabs__panel"
      :value="item.value"
    >
      <slot :name="String(item.value)" :item="item" />
    </TabsContent>
  </TabsRoot>
</template>

<script setup>
// Tabs switch between panels of one page (Reka Tabs: tablist / tab /
// tabpanel, aria-selected and aria-controls; ←/→ move and activate, Home/End
// jump, Tab moves into the panel).
// variant 'underline' (default) for long content sections; 'segmented' for the
// pill look of the reviewed prototype when the panels are page sections
// (转发规则 / 隧道 / 限速). Panels come from slots named after each value:
//   <UiTabs v-model="tab" :items="[{ value: 'rules', label: '规则' }]">
//     <template #rules>…</template>
//   </UiTabs>
import { computed } from 'vue'
import { TabsContent, TabsIndicator, TabsList, TabsRoot, TabsTrigger } from 'reka-ui'
import UiIcon from './UiIcon.vue'

const props = defineProps({
  modelValue: { type: [String, Number], default: undefined },
  items: { type: Array, default: () => [] },
  ariaLabel: { type: String, default: '' },
  variant: { type: String, default: 'underline', validator: value => ['underline', 'segmented'].includes(value) },
  activationMode: { type: String, default: 'automatic' },
  // Keep hidden panels mounted (forms keep their state) when false.
  unmountOnHide: { type: Boolean, default: true }
})

const emit = defineEmits(['update:modelValue'])

const normalizedItems = computed(() => props.items.map(item => (
  typeof item === 'object' && item !== null
    ? { ...item, label: String(item.label ?? item.value) }
    : { value: item, label: String(item) }
)))
</script>

<style scoped>
.ui-tabs {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
  min-width: 0;
}

.ui-tabs__list {
  position: relative;
  display: flex;
  max-width: 100%;
  overflow-x: auto;
  scrollbar-width: none;
}

.ui-tabs__list::-webkit-scrollbar {
  display: none;
}

.ui-tabs__tab {
  display: inline-flex;
  flex: none;
  gap: var(--space-2);
  align-items: center;
  border: 0;
  background: transparent;
  color: var(--label-2);
  font-family: inherit;
  font-weight: var(--weight-medium);
  white-space: nowrap;
  cursor: pointer;
  transition: color var(--dur-toggle) var(--ease-standard);
}

.ui-tabs__tab:hover:not([data-disabled]) {
  color: var(--label-1);
}

.ui-tabs__tab[data-state='active'] {
  color: var(--label-1);
}

.ui-tabs__tab[data-disabled] {
  opacity: 0.4;
  cursor: not-allowed;
}

.ui-tabs__count {
  min-width: 20px;
  padding: 0 var(--space-1);
  border-radius: var(--radius-pill);
  background: var(--fill-2);
  color: var(--label-2);
  font-size: var(--type-caption-size);
  font-variant-numeric: tabular-nums;
  line-height: 18px;
  text-align: center;
}

.ui-tabs__panel:focus-visible {
  border-radius: var(--radius-xs);
  outline: var(--focus-ring);
  outline-offset: var(--focus-ring-offset);
}

/* Underline */
.ui-tabs--underline .ui-tabs__list {
  gap: var(--space-6);
  border-bottom: 1px solid var(--separator);
}

.ui-tabs--underline .ui-tabs__tab {
  height: var(--size-control-lg);
  padding: 0;
  font-size: var(--type-body-size);
}

.ui-tabs--underline .ui-tabs__tab:focus-visible {
  border-radius: var(--radius-xs);
  outline: var(--focus-ring);
  outline-offset: calc(var(--focus-ring-offset) * -1);
}

.ui-tabs__indicator {
  position: absolute;
  bottom: -1px;
  left: 0;
  width: var(--reka-tabs-indicator-size);
  height: 2px;
  border-radius: var(--radius-pill);
  background: var(--accent);
  transform: translateX(var(--reka-tabs-indicator-position));
  transition: transform var(--dur-toggle) var(--ease-standard);
}

/* Segmented */
.ui-tabs--segmented .ui-tabs__list {
  display: inline-flex;
  gap: var(--space-0-5);
  align-self: flex-start;
  padding: var(--space-0-5);
  border-radius: var(--radius-pill);
  background: var(--fill-1);
}

.ui-tabs--segmented .ui-tabs__tab {
  height: 30px;
  padding: 0 var(--space-4);
  border-radius: var(--radius-pill);
  font-size: var(--type-callout-size);
  /* 4.5:1 on the track's fill over any page background (as UiSegmentedControl). */
  color: color-mix(in srgb, var(--label-2) 85%, var(--label-1));
  transition:
    background-color var(--dur-toggle) var(--ease-standard),
    color var(--dur-toggle) var(--ease-standard);
}

.ui-tabs--segmented .ui-tabs__tab[data-state='active'] {
  background: var(--bg-elevated);
  color: var(--label-1);
  box-shadow: var(--shadow-1), 0 0 0 0.5px var(--separator);
}

.ui-tabs--segmented .ui-tabs__tab:focus-visible {
  outline: var(--focus-ring);
  outline-offset: 0;
}

@media (pointer: coarse) {
  .ui-tabs--segmented .ui-tabs__tab {
    height: 40px;
  }
}

@media (forced-colors: active) {
  .ui-tabs__tab[data-state='active'] {
    text-decoration: underline;
  }
}
</style>
