<template>
  <figure class="ui-code" :class="{ 'is-wrapped': wrap }">
    <figcaption v-if="label || $slots.actions || copyable" class="ui-code__bar">
      <span :id="labelId" class="ui-code__label">{{ label }}</span>
      <span class="ui-code__actions">
        <slot name="actions" />
        <UiButton
          v-if="copyable"
          size="sm"
          variant="ghost"
          :icon="copied ? Check : Copy"
          :disabled="!code"
          :aria-describedby="label ? labelId : undefined"
          data-test="code-copy"
          @click="copy"
        >
          {{ copied ? t('ui.actions.copied') : t('ui.actions.copy') }}
        </UiButton>
      </span>
    </figcaption>
    <pre
      ref="preRef"
      class="ui-code__pre"
      :style="maxHeight ? { maxHeight } : undefined"
      tabindex="0"
      :aria-labelledby="label ? labelId : undefined"
    ><code class="ui-code__code"><slot>{{ code }}</slot></code></pre>
    <span class="visually-hidden" role="status">{{ statusText }}</span>
  </figure>
</template>

<script setup>
// A block of code or generated text (subscription output, commands, doctor
// output): monospace, scrolls inside itself (both ways, or wraps with
// `wrap`), a label and a copy button. The <pre> is focusable so keyboard
// users can scroll it. Copying falls back like UiCopyField; when it fails
// the text is selected and the status line says how to copy it by hand.
import { computed, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useId } from 'reka-ui'
import { Check, Copy } from '@lucide/vue'
import UiButton from './UiButton.vue'
import { copyText } from './composables/useClipboard'

const props = defineProps({
  code: { type: String, default: '' },
  label: { type: String, default: '' },
  copyable: { type: Boolean, default: true },
  // CSS length; the block scrolls beyond it. Empty: no limit.
  maxHeight: { type: String, default: '360px' },
  wrap: { type: Boolean, default: false }
})

const emit = defineEmits(['copy'])
const { t } = useI18n()
const labelId = useId(undefined, 'ui-code')
const preRef = ref(null)
const copied = ref(false)
const failed = ref(false)
let timer = null

const statusText = computed(() => {
  if (copied.value) return t('ui.actions.copied')
  if (failed.value) return t('ui.copy.failed')
  return ''
})

async function copy() {
  failed.value = false
  const ok = await copyText(props.code)
  if (ok) {
    copied.value = true
    emit('copy')
    clearTimeout(timer)
    timer = setTimeout(() => { copied.value = false }, 2000)
    return
  }
  failed.value = true
  selectContents()
}

function selectContents() {
  const node = preRef.value
  if (!node || typeof window === 'undefined' || !window.getSelection) return
  const range = document.createRange()
  range.selectNodeContents(node)
  const selection = window.getSelection()
  selection.removeAllRanges()
  selection.addRange(range)
}

onBeforeUnmount(() => clearTimeout(timer))
</script>

<style scoped>
.ui-code {
  display: flex;
  flex-direction: column;
  min-width: 0;
  margin: 0;
  overflow: hidden;
  border-radius: var(--radius-md);
  background: var(--bg-grouped);
  box-shadow: 0 0 0 0.5px var(--separator);
}

.ui-code__bar {
  display: flex;
  gap: var(--space-2);
  align-items: center;
  justify-content: space-between;
  min-height: 40px;
  padding: var(--space-1) var(--space-2) var(--space-1) var(--space-4);
  border-bottom: 1px solid var(--separator);
}

.ui-code__label {
  min-width: 0;
  overflow: hidden;
  color: var(--label-2);
  font-size: var(--type-caption-size);
  font-weight: var(--weight-medium);
  line-height: var(--type-caption-line);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ui-code__actions {
  display: flex;
  flex: none;
  gap: var(--space-1);
  align-items: center;
}

.ui-code__pre {
  min-width: 0;
  margin: 0;
  padding: var(--space-3) var(--space-4);
  overflow: auto;
  color: var(--label-1);
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
  line-height: 1.55;
  white-space: pre;
  tab-size: 2;
}

.is-wrapped .ui-code__pre {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.ui-code__pre:focus-visible {
  outline: var(--focus-ring);
  outline-offset: -2px;
  border-radius: 0 0 var(--radius-md) var(--radius-md);
}

.ui-code__code {
  font: inherit;
}
</style>
