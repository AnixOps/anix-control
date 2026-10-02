<template>
  <fieldset class="ui-otp" :class="{ 'is-invalid': Boolean(error), 'is-disabled': disabled }" :aria-describedby="describedBy">
    <legend class="ui-otp__legend" :class="{ 'visually-hidden': hideLabel }">{{ label }}</legend>
    <div class="ui-otp__boxes" :style="{ '--ui-otp-count': length }">
      <input
        v-for="(digit, index) in digits"
        :key="index"
        :ref="element => setInput(element, index)"
        class="ui-otp__box"
        type="text"
        inputmode="numeric"
        pattern="[0-9]*"
        :autocomplete="index === 0 ? 'one-time-code' : 'off'"
        :maxlength="index === 0 ? length : 1"
        :value="digit"
        :disabled="disabled"
        :aria-label="t('ui.otp.digit', { n: index + 1, total: length })"
        :aria-invalid="error ? 'true' : undefined"
        :aria-describedby="describedBy"
        :data-otp-index="index"
        @input="onInput($event, index)"
        @keydown="onKeydown($event, index)"
        @paste="onPaste($event, index)"
        @focus="onFocus($event)"
      >
    </div>
    <p v-if="help" :id="helpId" class="ui-otp__help">{{ help }}</p>
    <div :id="errorId" class="ui-otp__error" aria-live="polite">
      <template v-if="error">
        <UiIcon :icon="CircleAlert" :size="16" />
        <span>{{ error }}</span>
      </template>
    </div>
  </fieldset>
</template>

<script setup>
// One-time code: a row of single-digit boxes (6 by default) inside a
// fieldset whose legend names the code. Typing a digit moves to the next
// box; Backspace clears the box, or the one before when it is empty; the
// arrow keys, Home and End move between boxes; a paste (or an SMS/keychain
// autofill into the first box, which takes the whole code) spreads the
// digits from the box it lands in. Anything that is not a digit is ignored.
// `complete` fires once every box is filled. Boxes are 52 px high so they
// are comfortable touch targets, and use 16 px+ text so iOS does not zoom.
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useId } from 'reka-ui'
import { CircleAlert } from '@lucide/vue'
import UiIcon from './UiIcon.vue'

const props = defineProps({
  modelValue: { type: String, default: '' },
  length: { type: Number, default: 6 },
  label: { type: String, required: true },
  // Keep the legend for assistive tech only, when a heading above says it.
  hideLabel: { type: Boolean, default: false },
  help: { type: String, default: '' },
  error: { type: String, default: '' },
  disabled: { type: Boolean, default: false },
  id: { type: String, default: '' }
})

const emit = defineEmits(['update:modelValue', 'complete'])
const { t } = useI18n()
const fallbackId = useId(undefined, 'ui-otp')
const baseId = computed(() => props.id || fallbackId)
const helpId = computed(() => `${baseId.value}-help`)
const errorId = computed(() => `${baseId.value}-error`)
const describedBy = computed(() => [props.error ? errorId.value : '', props.help ? helpId.value : ''].filter(Boolean).join(' ') || undefined)

const inputs = []
function setInput(element, index) {
  inputs[index] = element
}

function normalize(value) {
  return String(value || '').replace(/\D/g, '').slice(0, props.length)
}

const digits = ref(Array.from({ length: props.length }, (_, index) => normalize(props.modelValue)[index] || ''))

watch(() => props.modelValue, (value) => {
  const clean = normalize(value)
  if (clean === digits.value.join('')) return
  digits.value = Array.from({ length: props.length }, (_, index) => clean[index] || '')
})

function commit() {
  const value = digits.value.join('')
  emit('update:modelValue', value)
  if (value.length === props.length && digits.value.every(Boolean)) emit('complete', value)
}

function focusBox(index) {
  const target = inputs[Math.max(0, Math.min(props.length - 1, index))]
  if (target) {
    target.focus()
    target.select?.()
  }
}

// Spread `text` from box `start`; returns the index after the last digit.
function spread(text, start) {
  const clean = String(text || '').replace(/\D/g, '')
  if (!clean) return start
  // A whole code arriving anywhere fills from the first box.
  const from = clean.length >= props.length ? 0 : start
  const next = [...digits.value]
  let index = from
  for (const char of clean) {
    if (index >= props.length) break
    next[index] = char
    index += 1
  }
  digits.value = next
  return index
}

function onInput(event, index) {
  const raw = event.target.value
  const clean = raw.replace(/\D/g, '')
  if (!clean) {
    // Not a digit (or cleared): keep the box as it was, minus non-digits.
    const next = [...digits.value]
    next[index] = ''
    digits.value = next
    event.target.value = ''
    commit()
    return
  }
  let end
  if (clean.length === 1 || (clean.length === 2 && digits.value[index])) {
    // One new digit, or a digit typed into a filled box: keep the newest.
    const next = [...digits.value]
    next[index] = clean.slice(-1)
    digits.value = next
    end = index + 1
  } else {
    end = spread(clean, index)
  }
  event.target.value = digits.value[index]
  commit()
  nextTick(() => focusBox(Math.min(end, props.length - 1)))
}

function onPaste(event, index) {
  const text = event.clipboardData?.getData('text') || ''
  event.preventDefault()
  const end = spread(text, index)
  commit()
  nextTick(() => focusBox(Math.min(end, props.length - 1)))
}

function onKeydown(event, index) {
  if (event.key === 'Backspace') {
    event.preventDefault()
    const next = [...digits.value]
    if (next[index]) {
      next[index] = ''
      digits.value = next
      commit()
      return
    }
    if (index > 0) {
      next[index - 1] = ''
      digits.value = next
      commit()
      focusBox(index - 1)
    }
    return
  }
  if (event.key === 'Delete') {
    event.preventDefault()
    const next = [...digits.value]
    next[index] = ''
    digits.value = next
    commit()
    return
  }
  const moves = { ArrowLeft: index - 1, ArrowRight: index + 1, Home: 0, End: props.length - 1 }
  if (event.key in moves) {
    event.preventDefault()
    focusBox(moves[event.key])
  }
}

function onFocus(event) {
  event.target.select?.()
}

defineExpose({
  // Focus the first empty box (or the last one when the code is complete).
  focus: () => {
    const empty = digits.value.findIndex(digit => !digit)
    focusBox(empty === -1 ? props.length - 1 : empty)
  },
  clear: () => {
    digits.value = Array.from({ length: props.length }, () => '')
    commit()
    nextTick(() => focusBox(0))
  }
})
</script>

<style scoped>
.ui-otp {
  min-width: 0;
  margin: 0;
  padding: 0;
  border: 0;
}

.ui-otp__legend {
  margin-bottom: var(--space-2);
  padding: 0;
  color: var(--label-1);
  font-size: var(--type-callout-size);
  font-weight: var(--weight-medium);
  line-height: var(--type-callout-line);
}

.ui-otp__boxes {
  display: grid;
  grid-template-columns: repeat(var(--ui-otp-count), minmax(0, 1fr));
  gap: var(--space-2);
}

.ui-otp__box {
  width: 100%;
  min-width: 0;
  height: 52px;
  padding: 0;
  border: 1px solid var(--label-3);
  border-radius: var(--radius-sm);
  background: var(--bg-elevated);
  color: var(--label-1);
  font: inherit;
  font-size: var(--type-title-3-size);
  font-weight: var(--weight-semibold);
  font-variant-numeric: tabular-nums;
  text-align: center;
  caret-color: var(--accent);
  outline: none;
  appearance: none;
  transition:
    border-color var(--dur-micro) var(--ease-standard),
    box-shadow var(--dur-micro) var(--ease-standard);
}

.ui-otp__box:hover:not(:disabled) {
  border-color: var(--label-2);
}

/* A transparent outline keeps a visible ring in forced-colors mode. */
.ui-otp__box:focus {
  border-color: var(--accent);
  outline: 2px solid transparent;
  box-shadow: 0 0 0 1px var(--accent), 0 0 0 4px var(--accent-soft);
}

.is-invalid .ui-otp__box {
  border-color: var(--danger);
}

.is-invalid .ui-otp__box:focus {
  box-shadow: 0 0 0 1px var(--danger), 0 0 0 4px var(--danger-soft);
}

.ui-otp__box:disabled {
  border-color: var(--separator-strong);
  background: var(--fill-1);
  color: var(--label-2);
  cursor: not-allowed;
}

.ui-otp__help {
  margin-top: var(--space-1);
  color: var(--label-2);
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
}

.ui-otp__error {
  display: flex;
  gap: var(--space-1);
  align-items: flex-start;
  color: var(--danger);
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
}

.ui-otp__error:not(:empty) {
  margin-top: var(--space-2);
}

.ui-otp__error :deep(.ui-icon) {
  margin-top: 1px;
}
</style>
