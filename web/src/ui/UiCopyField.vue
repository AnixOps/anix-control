<template>
  <UiField
    :class="['ui-copy', { 'is-stacked': stacked }]"
    :label="label"
    :help="help"
    :error="failed ? t('ui.copy.failed') : ''"
    :id="baseId"
  >
    <template #default="field">
      <div class="ui-copy__row">
        <InputBox :size="size" readonly>
          <input
            :id="field.id"
            ref="inputRef"
            class="ui-copy__input"
            :type="masked ? 'password' : 'text'"
            :value="value"
            readonly
            autocomplete="off"
            spellcheck="false"
            :aria-describedby="field.describedBy"
            @focus="selectAll"
          >
          <span v-if="secret" class="ui-box__end">
            <UiIconButton
              :label="t('ui.secret.show')"
              :icon="masked ? Eye : EyeOff"
              :pressed="!masked"
              :aria-controls="field.id"
              :tooltip="false"
              size="sm"
              variant="plain"
              @click="masked = !masked"
            />
          </span>
        </InputBox>
        <UiButton
          class="ui-copy__button"
          :class="{ 'is-copied': copied }"
          :size="size"
          :variant="variant"
          :icon="copied ? Check : Copy"
          :aria-describedby="label ? field.labelId : undefined"
          @click="copy"
        >
          {{ copied ? t('ui.actions.copied') : (copyLabel || t('ui.actions.copy')) }}
        </UiButton>
      </div>
      <span class="visually-hidden" role="status">{{ copied ? t('ui.actions.copied') : '' }}</span>
    </template>
  </UiField>
</template>

<script setup>
// Read-only value with a copy button: subscription links, tokens, install
// commands. "复制" turns into "已复制" (announced through a status region)
// for 2 s. With `secret` the value is masked (a readonly password field, so
// a screen reader says it is hidden) with a pressed/unpressed reveal toggle;
// copying never needs revealing. If the browser refuses (plain HTTP without
// the clipboard API and no execCommand), the text is selected and the field
// explains how to copy it by hand.
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useId } from 'reka-ui'
import { Check, Copy, Eye, EyeOff } from '@lucide/vue'
import UiField from './UiField.vue'
import UiButton from './UiButton.vue'
import UiIconButton from './UiIconButton.vue'
import InputBox from './internal/InputBox.vue'
import { copyText } from './composables/useClipboard'

const props = defineProps({
  value: { type: String, default: '' },
  label: { type: String, default: '' },
  help: { type: String, default: '' },
  secret: { type: Boolean, default: false },
  copyLabel: { type: String, default: '' },
  variant: { type: String, default: 'secondary' },
  size: { type: String, default: 'lg' },
  // Put the button under the field (narrow cards); also automatic below 640 px.
  stacked: { type: Boolean, default: false },
  id: { type: String, default: '' }
})

const emit = defineEmits(['copy'])
const { t } = useI18n()
const fallbackId = useId(undefined, 'ui-copy')
const baseId = computed(() => props.id || fallbackId)
const inputRef = ref(null)
const masked = ref(props.secret)
const copied = ref(false)
const failed = ref(false)
let resetTimer = null

watch(() => props.secret, (secret) => { masked.value = secret })
watch(() => props.value, () => {
  copied.value = false
  failed.value = false
})

function selectAll() {
  inputRef.value?.select()
}

async function copy() {
  failed.value = false
  const ok = await copyText(props.value)
  if (resetTimer) clearTimeout(resetTimer)
  if (ok) {
    copied.value = true
    emit('copy', props.value)
    resetTimer = setTimeout(() => { copied.value = false }, 2000)
    return
  }
  // Let the user copy by hand: show the text and select it.
  failed.value = true
  masked.value = false
  inputRef.value?.focus()
  inputRef.value?.select()
}

onBeforeUnmount(() => {
  if (resetTimer) clearTimeout(resetTimer)
})
</script>

<style scoped>
.ui-copy__row {
  display: flex;
  gap: var(--space-2);
  align-items: stretch;
}

.ui-copy__input {
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
  text-overflow: ellipsis;
}

.ui-copy__button {
  flex: none;
  min-width: 112px;
}

.ui-copy__button.is-copied {
  --ui-button-bg: var(--success-soft);
  --ui-button-bg-hover: var(--success-soft);
  --ui-button-fg: var(--success);
}

.ui-copy.is-stacked .ui-copy__row {
  flex-direction: column;
}

@media (max-width: 639px) {
  .ui-copy__row {
    flex-direction: column;
  }
}

/* Phones: inputs at 16 px (iOS zoom). */
@media (max-width: 833px) {
  .ui-copy__input {
    font-size: 16px;
  }
}
</style>
