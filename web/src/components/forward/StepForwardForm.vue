<template>
  <div class="step-form">
    <p class="step-intro">{{ t('forwardWizard.steps.forward.intro') }}</p>

    <div class="form-grid">
      <label class="form-group">
        <span>{{ t('runtime.forward.editor.fields.name') }}</span>
        <input v-model.trim="form.name" type="text" maxlength="50" :placeholder="t('runtime.forward.editor.placeholders.name')" />
        <p v-if="errors.name" class="form-error">{{ errors.name }}</p>
      </label>
      <label class="form-group">
        <span>{{ t('runtime.forward.editor.fields.tunnel') }}</span>
        <select v-model.number="form.tunnelId" disabled>
          <option :value="prefillTunnelId">{{ prefillTunnelLabel }}</option>
        </select>
        <p class="hint">{{ t('forwardWizard.steps.forward.inheritedTunnelHint') }}</p>
      </label>
    </div>

    <div class="form-grid">
      <label class="form-group">
        <span>{{ t('runtime.forward.editor.fields.ingressPort') }}</span>
        <input v-model="portInput" type="number" min="1" max="65535" :placeholder="t('runtime.forward.editor.placeholders.ingressPort')" />
        <p v-if="errors.inPort" class="form-error">{{ errors.inPort }}</p>
      </label>
      <label class="form-group">
        <span>{{ t('runtime.forward.editor.fields.interfaceName') }}</span>
        <input v-model.trim="form.interfaceName" type="text" :placeholder="t('runtime.forward.editor.placeholders.interfaceName')" />
      </label>
    </div>

    <label class="form-group">
      <span>{{ t('runtime.forward.editor.fields.remoteAddress') }}</span>
      <textarea v-model="form.remoteAddr" rows="6" :placeholder="t('runtime.forward.editor.placeholders.remoteAddress')"></textarea>
      <p class="hint">{{ t('runtime.forward.editor.remoteHint') }}</p>
      <p v-if="errors.remoteAddr" class="form-error">{{ errors.remoteAddr }}</p>
    </label>

    <label v-if="addressLineCount > 1" class="form-group">
      <span>{{ t('runtime.forward.editor.fields.strategy') }}</span>
      <select v-model="form.strategy">
        <option value="fifo">{{ t('runtime.forward.strategy.fifo') }}</option>
        <option value="round">{{ t('runtime.forward.strategy.round') }}</option>
        <option value="rand">{{ t('runtime.forward.strategy.rand') }}</option>
        <option value="hash">{{ t('runtime.forward.strategy.hash') }}</option>
      </select>
    </label>

    <p v-if="submitError" class="form-error">{{ submitError }}</p>

    <div class="step-actions">
      <button class="btn btn-primary" :disabled="submitLoading" @click="handleSubmit">
        {{ submitLoading ? t('runtime.forward.modal.submitLoading') : t('forwardWizard.steps.forward.createAndFinish') }}
      </button>
    </div>
  </div>
</template>

<script setup>
import { computed, reactive, ref, watch } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { createForward } from '@/api/admin'

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

function splitLines(value) {
  return String(value || '')
    .split('\n')
    .map(item => item.trim())
    .filter(Boolean)
}

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
    const addresses = splitLines(form.remoteAddr)
    const ipv4Pattern = /^(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?):\d+$/
    const ipv6FullPattern = /^\[((([0-9a-fA-F]{1,4}:){7}([0-9a-fA-F]{1,4}|:))|(([0-9a-fA-F]{1,4}:){6}(:[0-9a-fA-F]{1,4}|((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3})|:))|(([0-9a-fA-F]{1,4}:){5}(((:[0-9a-fA-F]{1,4}){1,2})|:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3})|:))|(([0-9a-fA-F]{1,4}:){4}(((:[0-9a-fA-F]{1,4}){1,3})|((:[0-9a-fA-F]{1,4})?:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:))|(([0-9a-fA-F]{1,4}:){3}(((:[0-9a-fA-F]{1,4}){1,4})|((:[0-9a-fA-F]{1,4}){0,2}:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:))|(([0-9a-fA-F]{1,4}:){2}(((:[0-9a-fA-F]{1,4}){1,5})|((:[0-9a-fA-F]{1,4}){0,3}:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:))|(([0-9a-fA-F]{1,4}:){1}(((:[0-9a-fA-F]{1,4}){1,6})|((:[0-9a-fA-F]{1,4}){0,4}:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:))|(:(((:[0-9a-fA-F]{1,4}){1,7})|((:[0-9a-fA-F]{1,4}){0,5}:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:)))\]:\d+$/
    const domainPattern = /^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*:\d+$/

    for (let index = 0; index < addresses.length; index += 1) {
      const address = addresses[index]
      if (!ipv4Pattern.test(address) && !ipv6FullPattern.test(address) && !domainPattern.test(address)) {
        errors.remoteAddr = t('runtime.forward.messages.remoteAddrLineInvalid', { line: index + 1 })
        break
      }
    }
  }

  if (form.inPort !== null && (form.inPort < 1 || form.inPort > 65535)) {
    errors.inPort = t('runtime.forward.messages.portRange')
  }

  return !errors.name && !errors.remoteAddr && !errors.inPort
}

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
.step-form { display: flex; flex-direction: column; gap: 16px; }
.step-intro { margin: 0; color: var(--text-secondary); line-height: 1.6; }
.form-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; }
.form-group { display: flex; flex-direction: column; gap: 8px; }
.form-group input, .form-group select, .form-group textarea { border: 1px solid var(--border-color); border-radius: 12px; background: var(--bg-color); color: var(--text-color); padding: 12px 14px; font-family: inherit; }
.hint { margin: 0; font-size: 12px; color: var(--text-secondary); }
.form-error { margin: 0; color: #b91c1c; }
.step-actions { display: flex; justify-content: flex-end; }
.btn { display: inline-flex; align-items: center; justify-content: center; border: 1px solid transparent; border-radius: 12px; padding: 10px 16px; cursor: pointer; }
.btn-primary { background: var(--primary-color); color: #fff; }
@media (max-width: 720px) {
  .form-grid { grid-template-columns: 1fr; }
}
</style>
