<template>
  <header class="ui-page-header">
    <div v-if="$slots.back" class="ui-page-header__back">
      <slot name="back" />
    </div>
    <div class="ui-page-header__row">
      <div class="ui-page-header__text">
        <div class="ui-page-header__title-row">
          <h1 class="ui-page-header__title">
            <slot name="title">{{ title }}</slot>
          </h1>
          <slot name="meta" />
        </div>
        <p v-if="description || $slots.description" class="ui-page-header__description">
          <slot name="description">{{ description }}</slot>
        </p>
      </div>
      <div v-if="$slots.actions" class="ui-page-header__actions">
        <slot name="actions" />
      </div>
    </div>
  </header>
</template>

<script setup>
// Page header: the page's single H1 (Title 1), one sentence of description,
// and the actions (secondary first, the primary action last, so it sits on
// the right). Slots: back (a back link), meta (a status badge next to the
// title), description, actions.
defineProps({
  title: { type: String, default: '' },
  description: { type: String, default: '' }
})
</script>

<style scoped>
.ui-page-header {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.ui-page-header__back {
  display: flex;
}

.ui-page-header__row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-4);
  align-items: flex-end;
  justify-content: space-between;
}

.ui-page-header__text {
  display: flex;
  flex: 1 1 320px;
  flex-direction: column;
  gap: var(--space-1);
  min-width: 0;
}

.ui-page-header__title-row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3);
  align-items: center;
}

.ui-page-header__title {
  min-width: 0;
  color: var(--label-1);
  font-size: var(--type-title-1-size);
  font-weight: var(--type-title-1-weight);
  line-height: var(--type-title-1-line);
  letter-spacing: var(--type-title-1-tracking);
  overflow-wrap: anywhere;
}

.ui-page-header__description {
  max-width: var(--size-content-read);
  color: var(--label-2);
}

.ui-page-header__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
}

@media (max-width: 639px) {
  .ui-page-header__actions {
    width: 100%;
  }
}
</style>
