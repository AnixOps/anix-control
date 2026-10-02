<template>
  <div class="ui-glist">
    <component :is="headingTag" v-if="title" :id="titleId" class="ui-glist__title">{{ title }}</component>
    <ul class="ui-glist__list" :aria-labelledby="title ? titleId : undefined">
      <slot />
    </ul>
    <p v-if="footer || $slots.footer" class="ui-glist__footer">
      <slot name="footer">{{ footer }}</slot>
    </p>
  </div>
</template>

<script setup>
// Settings-style grouped list (System Settings / iCloud): a small group
// title, rows on an elevated rounded surface separated by inset hairlines,
// and an optional footnote. Rows are UiGroupedListRow. Put it on
// --bg-grouped (a page section or a grouped UiSheet).
import { useId } from 'reka-ui'

defineProps({
  title: { type: String, default: '' },
  footer: { type: String, default: '' },
  headingTag: { type: String, default: 'h3' }
})

const titleId = useId(undefined, 'ui-glist')
</script>

<style scoped>
.ui-glist {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.ui-glist__title {
  padding: 0 var(--space-4) var(--space-2);
  color: var(--label-2);
  font-size: var(--type-callout-size);
  font-weight: var(--weight-medium);
  line-height: var(--type-callout-line);
}

.ui-glist__list {
  margin: 0;
  padding: 0;
  overflow: hidden;
  border-radius: var(--radius-md);
  background: var(--bg-elevated);
  box-shadow: 0 0 0 0.5px var(--separator);
  list-style: none;
}

.ui-glist__footer {
  padding: var(--space-2) var(--space-4) 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
}
</style>
