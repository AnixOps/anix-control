<template>
  <section class="ui-section">
    <header v-if="title || $slots.actions" class="ui-section__header">
      <div class="ui-section__heading">
        <component :is="headingTag" v-if="title" :id="titleId" class="ui-section__title">{{ title }}</component>
        <p v-if="description" class="ui-section__description">{{ description }}</p>
      </div>
      <div v-if="$slots.actions" class="ui-section__actions">
        <slot name="actions" />
      </div>
    </header>
    <slot />
  </section>
</template>

<script setup>
// A titled block of a page (Title 2 heading, a sentence, actions on the
// right) holding cards, tables or grouped lists. Sections are 48 px apart
// in page layouts; content starts 16 px under the heading. Not a landmark
// (no accessible name): headings carry the outline.
import { useId } from 'reka-ui'

defineProps({
  title: { type: String, default: '' },
  description: { type: String, default: '' },
  headingTag: { type: String, default: 'h2' }
})

const titleId = useId(undefined, 'ui-section')
</script>

<style scoped>
.ui-section {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  min-width: 0;
}

.ui-section__header {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3);
  align-items: flex-end;
  justify-content: space-between;
}

.ui-section__heading {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  min-width: 0;
}

.ui-section__title {
  font-size: var(--type-title-2-size);
  font-weight: var(--type-title-2-weight);
  line-height: var(--type-title-2-line);
  letter-spacing: var(--type-title-2-tracking);
}

.ui-section__description {
  max-width: var(--size-content-read);
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.ui-section__actions {
  display: flex;
  gap: var(--space-2);
  align-items: center;
}
</style>
