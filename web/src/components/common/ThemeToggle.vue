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
  border: 1px solid var(--border-color);
  border-radius: 999px;
  background: var(--surface-color);
  color: var(--text-secondary);
  cursor: pointer;
  font-size: 12px;
  font-weight: 700;
  box-shadow: var(--shadow-sm);
  transition: var(--transition);
}

.theme-toggle:hover {
  background: var(--surface-hover);
  color: var(--text-color);
}

.theme-toggle.compact {
  min-width: 40px;
  justify-content: center;
  padding: 6px 10px;
}

.theme-icon {
  font-size: 14px;
  line-height: 1;
}
</style>
