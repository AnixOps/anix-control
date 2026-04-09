<template>
  <div class="locale-switcher" :class="{ compact }" :aria-label="t('common.locale.label')">
    <button
      v-for="option in localeOptions"
      :key="option.value"
      type="button"
      class="locale-option"
      :class="{ active: currentLocale === option.value }"
      :aria-pressed="currentLocale === option.value"
      :title="`${t('common.locale.switch')}: ${option.label}`"
      @click="switchLocale(option.value)"
    >
      {{ compact ? option.shortLabel : option.label }}
    </button>
  </div>
</template>

<script setup>
import { useAppI18n } from '@/composables/useAppI18n'

defineProps({
  compact: {
    type: Boolean,
    default: false
  }
})

const { t, currentLocale, localeOptions, switchLocale } = useAppI18n()
</script>

<style scoped>
.locale-switcher {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 4px;
  border: 1px solid var(--border-color);
  border-radius: 999px;
  background: rgba(148, 163, 184, 0.08);
}

.locale-option {
  border: 0;
  background: transparent;
  color: var(--text-secondary);
  padding: 6px 10px;
  border-radius: 999px;
  cursor: pointer;
  font-size: 12px;
  font-weight: 600;
  transition: var(--transition);
}

.locale-option.active {
  background: var(--primary-color);
  color: #fff;
}

.locale-switcher.compact .locale-option {
  min-width: 40px;
  padding: 6px 8px;
}
</style>
