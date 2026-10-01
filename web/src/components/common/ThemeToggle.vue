<template>
  <button
    type="button"
    class="theme-toggle"
    :class="{ compact }"
    role="switch"
    :aria-checked="dark"
    :aria-label="label"
    :title="label"
    @click="toggleTheme"
  >
    <span class="theme-icon" aria-hidden="true">{{ dark ? '☾' : '☀' }}</span>
    <span v-if="!compact" class="theme-text">{{ label }}</span>
  </button>
</template>

<script setup>
import { computed } from 'vue'
import { useTheme } from '@/composables/useTheme'
import { useAppI18n } from '@/composables/useAppI18n'

defineProps({
  compact: {
    type: Boolean,
    default: false
  }
})

const { t } = useAppI18n()
const { currentTheme, toggleTheme } = useTheme()

const dark = computed(() => currentTheme.value === 'dark')
const label = computed(() =>
  dark.value ? t('common.theme.switchToLight') : t('common.theme.switchToDark')
)
</script>

<style scoped>
.theme-toggle {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  border: 0;
  border-radius: var(--radius-pill);
  background: var(--fill-1);
  color: var(--text-secondary);
  cursor: pointer;
  font-size: var(--type-caption-size);
  font-weight: var(--weight-semibold);
  box-shadow: none;
  transition: var(--transition);
}

.theme-toggle:hover {
  background: var(--fill-2);
  color: var(--text-color);
}

.theme-toggle.compact {
  min-width: 40px;
  justify-content: center;
  padding: 6px 10px;
}

.theme-icon {
  font-size: var(--type-body-size);
  line-height: 1;
}
</style>
