<template>
  <UiDialog
    :open="open"
    size="sm"
    role="alertdialog"
    :title="title || t('ui.confirm.title')"
    :description="message"
    :dismissible="!loading"
    :close-on-scrim="false"
    :hide-close="true"
    :initial-focus="initialFocus"
    @update:open="onOpenChange"
  >
    <template v-if="requireText || error || $slots.default" #default>
      <slot />
      <UiTextField
        v-if="requireText"
        v-model="typed"
        class="ui-confirm__typed"
        data-confirm-typed
        autocomplete="off"
        autocapitalize="off"
        spellcheck="false"
        :disabled="loading"
        @keydown.enter.prevent="submit"
      >
        <template #label>
          <I18nT keypath="ui.confirm.typeToConfirm" tag="span" scope="global">
            <template #name>
              <code class="ui-confirm__name">{{ requireText }}</code>
            </template>
          </I18nT>
        </template>
      </UiTextField>
      <p v-if="error" class="ui-confirm__error" role="alert">{{ error }}</p>
    </template>
    <template #footer>
      <UiButton data-confirm-cancel :disabled="loading" @click="cancel">
        {{ cancelLabel || t('ui.actions.cancel') }}
      </UiButton>
      <UiButton
        data-confirm-ok
        :variant="tone === 'danger' ? 'danger' : 'primary'"
        :loading="loading"
        :disabled="!matches"
        @click="submit"
      >
        {{ confirmLabel || t('ui.actions.confirm') }}
      </UiButton>
    </template>
  </UiDialog>
</template>

<script setup>
// Confirmation for an irreversible or wide-reaching action
// (role="alertdialog"). Name the object in the title and say what happens:
// 「删除节点 hk-01？」 + 「此操作无法撤销。」; the confirm button is a verb
// (「删除节点」), never 「确定」.
// requireText: the user must type this exact text (node or user name) before
// the danger button enables. Initial focus: the typed field if any, else
// Cancel for danger, else the confirm button. Esc cancels; the scrim does not.
// Most code calls useConfirm() instead of placing this component.
import { computed, ref, watch } from 'vue'
import { Translation as I18nT, useI18n } from 'vue-i18n'
import UiDialog from './UiDialog.vue'
import UiButton from './UiButton.vue'
import UiTextField from './UiTextField.vue'

const props = defineProps({
  open: { type: Boolean, default: false },
  title: { type: String, default: '' },
  message: { type: String, default: '' },
  confirmLabel: { type: String, default: '' },
  cancelLabel: { type: String, default: '' },
  tone: { type: String, default: 'default', validator: value => ['default', 'danger'].includes(value) },
  requireText: { type: String, default: '' },
  loading: { type: Boolean, default: false },
  error: { type: String, default: '' }
})

const emit = defineEmits(['update:open', 'confirm', 'cancel'])
const { t } = useI18n()
const typed = ref('')

const matches = computed(() => !props.requireText || typed.value.trim() === props.requireText)

const initialFocus = computed(() => {
  if (props.requireText) return '[data-confirm-typed]'
  return props.tone === 'danger' ? '[data-confirm-cancel]' : '[data-confirm-ok]'
})

watch(() => props.open, (open) => {
  if (open) typed.value = ''
})

function onOpenChange(value) {
  if (!value) cancel()
}

function cancel() {
  if (props.loading) return
  emit('update:open', false)
  emit('cancel')
}

function submit() {
  if (!matches.value || props.loading) return
  emit('confirm')
}
</script>

<style scoped>
.ui-confirm__name {
  padding: 0 var(--space-1);
  border-radius: calc(var(--radius-xs) - 2px);
  background: var(--fill-1);
  font-family: var(--font-mono);
  font-weight: var(--weight-semibold);
  overflow-wrap: anywhere;
}

.ui-confirm__error {
  color: var(--danger);
  font-size: var(--type-callout-size);
}
</style>
