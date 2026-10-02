<template>
  <div class="ui-search" :class="[$attrs.class, { 'has-value': Boolean(modelValue) }]" :style="$attrs.style" role="search" :aria-label="label">
    <UiIcon class="ui-search__icon" :icon="Search" :size="16" />
    <input
      ref="inputRef"
      class="ui-search__input"
      type="search"
      enterkeyhint="search"
      autocomplete="off"
      :value="modelValue"
      :placeholder="placeholder || label"
      :aria-label="label"
      :aria-keyshortcuts="shortcut ? '/' : undefined"
      v-bind="inputAttrs"
      @input="onInput"
      @keydown.enter.prevent="emit('submit', modelValue)"
      @keydown.esc="onEscape"
    />
    <kbd v-if="shortcut && !modelValue" class="ui-search__kbd" aria-hidden="true">/</kbd>
    <button
      v-if="modelValue"
      type="button"
      class="ui-search__clear"
      :aria-label="t('ui.search.clear')"
      @click="clear"
    >
      <UiIcon :icon="X" :size="14" />
    </button>
  </div>
</template>

<script setup>
// Search box of a list toolbar: a named search landmark, 36 px, magnifier,
// clear button, Esc clears. `/` focuses it from anywhere on the page
// (plan §9) unless `shortcut` is false. Emits update:modelValue on input
// (debounce in the page when it calls the server) and submit on Enter.
import { computed, onBeforeUnmount, onMounted, ref, useAttrs } from 'vue'
import { useI18n } from 'vue-i18n'
import { Search, X } from '@lucide/vue'
import UiIcon from './UiIcon.vue'

defineOptions({ inheritAttrs: false })

const props = defineProps({
  modelValue: { type: String, default: '' },
  label: { type: String, required: true },
  placeholder: { type: String, default: '' },
  shortcut: { type: Boolean, default: true }
})

const emit = defineEmits(['update:modelValue', 'submit'])
const { t } = useI18n()
const inputRef = ref(null)
// class and style size the box; everything else (data-*, aria-*) is the input's.
const attrs = useAttrs()
const inputAttrs = computed(() => {
  const { class: _class, style: _style, ...rest } = attrs
  return rest
})

function onInput(event) {
  emit('update:modelValue', event.target.value)
}

function clear() {
  emit('update:modelValue', '')
  emit('submit', '')
  inputRef.value?.focus()
}

function onEscape(event) {
  if (!props.modelValue) return
  event.stopPropagation()
  clear()
}

function isTyping(target) {
  if (!target || !(target instanceof Element)) return false
  return Boolean(target.closest('input, textarea, select, [contenteditable=""], [contenteditable="true"], [role="dialog"], [role="menu"], [role="listbox"]'))
}

function onKeydown(event) {
  if (!props.shortcut || event.key !== '/' || event.metaKey || event.ctrlKey || event.altKey) return
  if (isTyping(event.target)) return
  event.preventDefault()
  inputRef.value?.focus()
}

onMounted(() => document.addEventListener('keydown', onKeydown))
onBeforeUnmount(() => document.removeEventListener('keydown', onKeydown))

defineExpose({ focus: () => inputRef.value?.focus() })
</script>

<style scoped>
.ui-search {
  position: relative;
  display: flex;
  align-items: center;
  width: 100%;
  min-width: 0;
  height: var(--size-control-md);
  /* Same boundary as the text fields: 1 px --label-3 reaches 3:1 in both
     themes (WCAG 1.4.11). */
  border: 1px solid var(--label-3);
  border-radius: var(--radius-sm);
  background: var(--bg-elevated);
  transition: box-shadow var(--dur-micro) var(--ease-standard);
}

.ui-search:focus-within {
  border-color: var(--accent);
  box-shadow: 0 0 0 1px var(--accent), 0 0 0 4px var(--accent-soft);
}

.ui-search__icon {
  position: absolute;
  left: var(--space-3);
  color: var(--label-2);
  pointer-events: none;
}

.ui-search__input {
  flex: 1;
  min-width: 0;
  height: 100%;
  padding: 0 var(--space-8) 0 var(--space-8);
  border: 0;
  background: transparent;
  color: var(--label-1);
  font: inherit;
  font-size: var(--type-body-size);
  outline: none;
}

.ui-search__input::placeholder {
  color: var(--label-2);
}

.ui-search__input::-webkit-search-cancel-button {
  display: none;
}

.ui-search__kbd {
  position: absolute;
  right: var(--space-2);
  display: grid;
  place-items: center;
  min-width: 20px;
  height: 20px;
  border: 1px solid var(--separator-strong);
  border-radius: var(--radius-xs);
  color: var(--label-2);
  font-family: var(--font-sans);
  font-size: var(--type-caption-size);
  pointer-events: none;
}

.ui-search__clear {
  position: absolute;
  right: var(--space-1);
  display: grid;
  place-items: center;
  width: var(--size-control-sm);
  height: var(--size-control-sm);
  min-height: 0;
  padding: 0;
  border: 0;
  border-radius: var(--radius-pill);
  background: transparent;
  color: var(--label-2);
  cursor: pointer;
}

.ui-search__clear:hover {
  background: var(--fill-2);
  color: var(--label-1);
}

.ui-search__clear:focus-visible {
  outline: var(--focus-ring);
  outline-offset: 0;
}

@media (max-width: 639.98px) {
  .ui-search__input {
    font-size: 16px;
  }

  .ui-search__kbd {
    display: none;
  }
}

@media (pointer: coarse) {
  .ui-search {
    height: 44px;
  }

  /* A 44 px hit area around the 28 px clear button. */
  .ui-search__clear::after {
    position: absolute;
    top: 50%;
    left: 50%;
    width: var(--size-control-lg);
    height: var(--size-control-lg);
    transform: translate(-50%, -50%);
    content: '';
  }
}
</style>
