<template>
  <component :is="as" class="ui-card" :class="{ 'is-padded': padded && !hasHeader, 'is-interactive': interactive }" :aria-labelledby="title && as === 'section' ? titleId : undefined">
    <header v-if="hasHeader" class="ui-card__header">
      <div class="ui-card__heading">
        <component :is="headingTag" v-if="title" :id="titleId" class="ui-card__title">{{ title }}</component>
        <p v-if="description" class="ui-card__description">{{ description }}</p>
      </div>
      <div v-if="$slots.actions" class="ui-card__actions">
        <slot name="actions" />
      </div>
    </header>
    <div v-if="hasHeader" class="ui-card__body" :class="{ 'is-padded': padded }">
      <slot />
    </div>
    <slot v-else />
  </component>
</template>

<script setup>
// Elevated surface: radius-md, shadow-1 and a hairline edge (a 1 px
// highlight in dark mode comes from the shadow token). With `title` it gets
// a header row (title, description, actions) above a padded body.
import { computed, useSlots } from 'vue'
import { useId } from 'reka-ui'

const props = defineProps({
  title: { type: String, default: '' },
  description: { type: String, default: '' },
  headingTag: { type: String, default: 'h2' },
  padded: { type: Boolean, default: true },
  interactive: { type: Boolean, default: false },
  // 'section' makes it a named region; use it for a few top-level cards only.
  as: { type: String, default: 'div' }
})

const slots = useSlots()
const titleId = useId(undefined, 'ui-card')
const hasHeader = computed(() => Boolean(props.title || slots.actions))
</script>

<style scoped>
.ui-card {
  display: flex;
  flex-direction: column;
  min-width: 0;
  border-radius: var(--radius-md);
  background: var(--bg-elevated);
  box-shadow: var(--shadow-1), 0 0 0 0.5px var(--separator);
  color: var(--label-1);
}

.ui-card.is-padded,
.ui-card__body.is-padded {
  padding: var(--space-6);
}

.ui-card.is-interactive {
  transition: box-shadow var(--dur-micro) var(--ease-standard);
}

.ui-card.is-interactive:hover {
  box-shadow: var(--shadow-2), 0 0 0 0.5px var(--separator);
}

.ui-card__header {
  display: flex;
  gap: var(--space-3);
  align-items: center;
  justify-content: space-between;
  padding: var(--space-4) var(--space-6);
  border-bottom: 1px solid var(--separator);
}

.ui-card__heading {
  display: flex;
  flex-direction: column;
  gap: var(--space-0-5);
  min-width: 0;
}

.ui-card__title {
  font-size: var(--type-body-size);
  font-weight: var(--weight-semibold);
  line-height: var(--type-body-line);
}

.ui-card__description {
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.ui-card__actions {
  display: flex;
  flex: none;
  gap: var(--space-2);
  align-items: center;
}

@media (max-width: 639px) {
  .ui-card.is-padded,
  .ui-card__body.is-padded {
    padding: var(--space-4);
  }

  .ui-card__header {
    padding: var(--space-3) var(--space-4);
  }
}
</style>
