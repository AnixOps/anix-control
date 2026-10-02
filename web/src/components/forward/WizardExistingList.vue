<template>
  <section class="existing" :aria-labelledby="headingId">
    <h3 :id="headingId" class="existing__title">{{ t('forwardWizard.shared.existingLabel') }}</h3>
    <ul class="existing__list">
      <li v-for="item in items" :key="item.id" class="existing__item">
        <span class="existing__text">
          <span class="existing__name">{{ item.title }}</span>
          <span v-if="item.meta" class="existing__meta">{{ item.meta }}</span>
        </span>
        <UiButton size="sm" :aria-label="`${t('forwardWizard.shared.useExisting')} ${item.title}`" @click="emit('use', item)">
          {{ t('forwardWizard.shared.useExisting') }}
        </UiButton>
      </li>
    </ul>
  </section>
</template>

<script setup>
// Records a wizard step can reuse instead of creating a new one (machines,
// NodeX nodes, tunnels): "使用现有" continues with that record.
import { useId } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import UiButton from '@/ui/UiButton.vue'

defineProps({
  // [{ id, title, meta }]
  items: { type: Array, default: () => [] }
})

const emit = defineEmits(['use'])
const { t } = useAppI18n()
const headingId = `wizard-existing-${useId()}`
</script>

<style scoped>
.existing {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.existing__title {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-caption-size);
  font-weight: var(--weight-semibold);
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.existing__list {
  display: flex;
  flex-direction: column;
  max-height: 240px;
  margin: 0;
  padding: 0;
  overflow-y: auto;
  border-radius: var(--radius-md);
  background: var(--fill-1);
  list-style: none;
}

.existing__item {
  display: flex;
  gap: var(--space-3);
  align-items: center;
  justify-content: space-between;
  min-height: 52px;
  padding: var(--space-2) var(--space-3);
}

.existing__item + .existing__item {
  border-top: 0.5px solid var(--separator);
}

.existing__text {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.existing__name {
  color: var(--label-1);
  font-weight: var(--weight-medium);
  overflow-wrap: anywhere;
}

.existing__meta {
  color: var(--label-2);
  font-family: var(--font-mono);
  font-size: var(--type-caption-size);
  overflow-wrap: anywhere;
}
</style>
