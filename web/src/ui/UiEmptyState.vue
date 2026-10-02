<template>
  <div class="ui-empty" :class="{ 'is-compact': compact }">
    <UiIcon v-if="icon" class="ui-empty__icon" :icon="icon" :size="compact ? 32 : 48" />
    <div class="ui-empty__text">
      <component :is="headingTag" class="ui-empty__title">{{ title }}</component>
      <p v-if="description" class="ui-empty__description">{{ description }}</p>
    </div>
    <div v-if="$slots.actions" class="ui-empty__actions">
      <slot name="actions" />
    </div>
  </div>
</template>

<script setup>
// Empty state: what will appear here and the first action
// (「还没有节点」 + 「添加第一个节点后，用户就能使用订阅。」 + 「添加节点」).
// For a failed load pass the error as description and a retry action.
import UiIcon from './UiIcon.vue'

defineProps({
  title: { type: String, required: true },
  description: { type: String, default: '' },
  icon: { type: [Object, Function], default: null },
  // Keep the heading outline continuous: h2 on a page, h3 inside a card.
  headingTag: { type: String, default: 'h3' },
  compact: { type: Boolean, default: false }
})
</script>

<style scoped>
.ui-empty {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  align-items: center;
  padding: var(--space-12) var(--space-6);
  text-align: center;
}

.ui-empty.is-compact {
  gap: var(--space-3);
  padding: var(--space-6) var(--space-4);
}

.ui-empty__icon {
  color: var(--label-3);
}

.ui-empty__text {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  max-width: 420px;
}

.ui-empty__title {
  color: var(--label-1);
  font-size: var(--type-body-size);
  font-weight: var(--weight-semibold);
  line-height: var(--type-body-line);
}

.ui-empty__description {
  color: var(--label-2);
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
}

.ui-empty__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  justify-content: center;
}
</style>
