<template>
  <div class="locale-switcher" :class="{ compact }" role="radiogroup" :aria-label="t('common.locale.label')">
    <button
      v-for="option in localeOptions"
      :key="option.value"
      :ref="(el) => setOptionRef(el, option.value)"
      type="button"
      class="locale-option"
      :class="{ active: currentLocale === option.value }"
      role="radio"
      :tabindex="currentLocale === option.value ? 0 : -1"
      :aria-checked="currentLocale === option.value"
      :aria-label="`${t('common.locale.switch')}: ${option.label}`"
      :title="`${t('common.locale.switch')}: ${option.label}`"
      @click="activateLocale(option.value)"
      @keydown="onOptionKeydown($event, option.value)"
    >
      {{ compact ? option.shortLabel : option.label }}
    </button>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'

defineProps({
  compact: {
    type: Boolean,
    default: false
  }
})

const { t, currentLocale, localeOptions, switchLocale } = useAppI18n()
const optionRefs = ref({})

function setOptionRef(el, key) {
  if (el) {
    optionRefs.value[key] = el
  }
}

function focusOptionByValue(value) {
  const element = optionRefs.value[value]
  if (element && typeof element.focus === 'function') {
    element.focus()
  }
}

function activateLocale(value) {
  switchLocale(value)
}

function onOptionKeydown(event, value) {
  const options = localeOptions.value || []
  const currentIndex = options.findIndex(item => item.value === value)
  if (currentIndex < 0) return

  const move = (nextIndex) => {
    const normalizedIndex = (nextIndex + options.length) % options.length
    const nextValue = options[normalizedIndex]?.value
    if (!nextValue) return
    activateLocale(nextValue)
    focusOptionByValue(nextValue)
  }

  switch (event.key) {
    case 'ArrowRight':
    case 'ArrowDown':
      event.preventDefault()
      move(currentIndex + 1)
      break
    case 'ArrowLeft':
    case 'ArrowUp':
      event.preventDefault()
      move(currentIndex - 1)
      break
    case 'Home':
      event.preventDefault()
      move(0)
      break
    case 'End':
      event.preventDefault()
      move(options.length - 1)
      break
    default:
      break
  }
}
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
