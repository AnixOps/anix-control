<template>
  <form id="wizard-step-form" class="step-form" novalidate @submit.prevent="handleSubmit">
    <p class="step-intro">{{ t('forwardWizard.steps.forward.intro') }}</p>

    <div class="form-grid">
      <UiTextField v-model.trim="form.name" :label="t('runtime.forward.editor.fields.name')" :placeholder="t('runtime.forward.editor.placeholders.name')" :error="errors.name" maxlength="50" required size="md" />
      <UiTextField
        :model-value="prefillTunnelLabel"
        :label="t('runtime.forward.editor.fields.tunnel')"
        :help="t('forwardWizard.steps.forward.inheritedTunnelHint')"
        readonly
        size="md"
      />
      <UiTextField
        v-model="portInput"
        type="number"
        min="1"
        max="65535"
        inputmode="numeric"
        :label="t('runtime.forward.editor.fields.ingressPort')"
        :placeholder="t('runtime.forward.editor.placeholders.ingressPort')"
        :error="errors.inPort"
        size="md"
      />
      <UiTextField v-model.trim="form.interfaceName" :label="t('runtime.forward.editor.fields.interfaceName')" :placeholder="t('runtime.forward.editor.placeholders.interfaceName')" size="md" />
    </div>
    <UiTextarea
      v-model="form.remoteAddr"
      class="targets"
      :rows="6"
      :label="t('runtime.forward.editor.fields.remoteAddress')"
      :placeholder="t('runtime.forward.editor.placeholders.remoteAddress')"
      :help="t('runtime.forward.editor.remoteHint')"
      :error="errors.remoteAddr"
      required
    />
    <UiSelect
      v-if="addressLineCount > 1"
      v-model="form.strategy"
      :label="t('runtime.forward.editor.fields.strategy')"
      :options="strategyOptions"
      size="md"
    />
    <p v-if="submitError" class="form-error" role="alert">{{ submitError }}</p>
  </form>
</template>

<script setup>
import { computed, reactive, ref, watch } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { createForward } from '@/api/admin'
import { findInvalidTargetLine, splitLines } from '@/views/admin/forward/forwardModel'
import UiSelect from '@/ui/UiSelect.vue'
import UiTextField from '@/ui/UiTextField.vue'
import UiTextarea from '@/ui/UiTextarea.vue'

const props = defineProps({
  prefillTunnelId: { type: Number, required: true },
  prefillTunnelLabel: { type: String, default: '' }
})

const emit = defineEmits(['created'])

const { t, translateLiteral } = useAppI18n()

const submitLoading = ref(false)
const submitError = ref('')
const portInput = ref('')

const form = reactive({
  name: '',
  tunnelId: props.prefillTunnelId,
  inPort: null,
  remoteAddr: '',
  interfaceName: '',
  strategy: 'fifo'
})
const errors = reactive({ name: '', remoteAddr: '', inPort: '' })

const addressLineCount = computed(() => splitLines(form.remoteAddr).length)

watch(portInput, value => {
  if (value === '' || value === null) {
    form.inPort = null
    return
  }
  const parsed = Number(value)
  form.inPort = Number.isFinite(parsed) ? parsed : null
})


function translateMessage(value, fallback) {
  const text = String(value ?? '').trim()
  if (!text) return fallback
  return translateLiteral(text)
}

function validateForm() {
  errors.name = ''
  errors.remoteAddr = ''
  errors.inPort = ''

  if (!form.name.trim()) {
    errors.name = t('runtime.forward.messages.nameRequired')
  } else if (form.name.length < 2 || form.name.length > 50) {
    errors.name = t('runtime.forward.messages.nameLength')
  }

  if (!form.remoteAddr.trim()) {
    errors.remoteAddr = t('runtime.forward.messages.remoteAddrRequired')
  } else {
    const invalidLine = findInvalidTargetLine(form.remoteAddr)
    if (invalidLine >= 0) {
      errors.remoteAddr = t('runtime.forward.messages.remoteAddrLineInvalid', { line: invalidLine + 1 })
    }
  }

  if (form.inPort !== null && (form.inPort < 1 || form.inPort > 65535)) {
    errors.inPort = t('runtime.forward.messages.portRange')
  }

  return !errors.name && !errors.remoteAddr && !errors.inPort
}

const strategyOptions = computed(() => ['fifo', 'round', 'rand', 'hash'].map(value => ({ value, label: t(`runtime.forward.strategy.${value}`) })))

// The wizard footer reads these (UI U7: 上一步 / 下一步 at the bottom).
defineExpose({ submit: handleSubmit, busy: submitLoading, canSubmit: true, primaryLabel: computed(() => t('forwardWizard.steps.forward.createAndFinish')) })

async function handleSubmit() {
  submitError.value = ''
  if (!validateForm()) return

  submitLoading.value = true
  try {
    const processedRemoteAddr = splitLines(form.remoteAddr).join(',')
    const addressCount = processedRemoteAddr.split(',').map(item => item.trim()).filter(Boolean).length
    const payload = {
      name: form.name,
      tunnelId: form.tunnelId,
      inPort: form.inPort,
      remoteAddr: processedRemoteAddr,
      interfaceName: form.interfaceName,
      strategy: addressCount > 1 ? form.strategy : 'fifo'
    }

    const response = await createForward(payload)
    if (response.code !== 0) {
      submitError.value = translateMessage(response.msg, t('runtime.forward.messages.actionFailed'))
      return
    }
    emit('created', { id: Number(response.data?.id || 0), name: payload.name })
  } catch {
    submitError.value = t('runtime.forward.messages.actionFailed')
  } finally {
    submitLoading.value = false
  }
}
</script>

<style scoped>
.step-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
}

.step-intro {
  margin: 0;
  color: var(--label-2);
}

.targets :deep(textarea) {
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
}

.form-error {
  margin: 0;
}
</style>
