<template>
  <form id="wizard-step-form" class="step-form" novalidate @submit.prevent="handleConfirm">
    <p class="step-intro">{{ t('forwardWizard.steps.mode.intro') }}</p>

    <UiSkeleton v-if="loadingCurrent" variant="text" :lines="4" :label="t('forwardWizard.loading')" />

    <template v-else>
      <div class="mode-options">
        <UiRadioGroup
          v-model="selectedKey"
          :label="t('forwardWizard.steps.mode.chooseLabel')"
          :options="modeOptions"
          required
          @update:model-value="selectCard"
        />
      </div>

      <section v-if="needsNodeXSetup" class="nodex-setup" aria-labelledby="wizard-nodex-setup">
        <p id="wizard-nodex-setup" class="nodex-setup__hint">{{ t('forwardWizard.steps.mode.nodeXSetupHint') }}</p>
        <div class="form-grid">
          <UiTextField
            v-model.trim="nodeXBaseUrl"
            :label="t('runtime.nodeX.fields.baseUrl')"
            placeholder="https://nodex.example.com"
            :help="t('runtime.nodeX.fields.baseUrlHint')"
            type="url"
            required
            size="md"
          />
          <UiPasswordField
            v-model.trim="nodeXToken"
            :label="t('runtime.nodeX.fields.token')"
            :help="t('runtime.nodeX.fields.tokenHint')"
            autocomplete="off"
            required
            size="md"
          />
        </div>
        <p v-if="errors.nodeX" class="form-error" role="alert">{{ errors.nodeX }}</p>
      </section>

      <p v-if="submitError" class="form-error" role="alert">{{ submitError }}</p>
    </template>
  </form>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { getSystemConfig, setSystemConfig } from '@/api/admin'
import UiPasswordField from '@/ui/UiPasswordField.vue'
import UiRadioGroup from '@/ui/UiRadioGroup.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import UiTextField from '@/ui/UiTextField.vue'

const emit = defineEmits(['selected'])

const { t } = useAppI18n()

const runtimeNodeXModeKey = 'forward.runtime.nodex_mode'
const runtimeBackendKey = 'forward.runtime_backend'
const runtimeNodeXBaseUrlKey = 'forward.runtime.nodex.base_url'
const runtimeNodeXTokenKey = 'forward.runtime.nodex.token'
const runtimeNodeXTimeoutKey = 'forward.runtime.nodex.timeout_seconds'

const CARD_LOCAL = 'local'
const CARD_GOST_SINGLE = 'gostSingle'
const CARD_GOST_TUNNEL = 'gostTunnel'

const loadingCurrent = ref(true)
const submitLoading = ref(false)
const submitError = ref('')
const currentKey = ref(CARD_LOCAL)
const selectedKey = ref('')

const nodeXBaseUrlConfigured = ref(false)
const nodeXTokenConfigured = ref(false)
const nodeXBaseUrl = ref('')
const nodeXToken = ref('')
const nodeXTimeout = ref(15)

const errors = reactive({ nodeX: '' })

const cards = computed(() => ([
  {
    key: CARD_LOCAL,
    label: t('forwardWizard.steps.mode.cards.local.label'),
    description: t('forwardWizard.steps.mode.cards.local.description')
  },
  {
    key: CARD_GOST_SINGLE,
    label: t('forwardWizard.steps.mode.cards.gostSingle.label'),
    description: t('forwardWizard.steps.mode.cards.gostSingle.description')
  },
  {
    key: CARD_GOST_TUNNEL,
    label: t('forwardWizard.steps.mode.cards.gostTunnel.label'),
    description: t('forwardWizard.steps.mode.cards.gostTunnel.description')
  }
]))

// The wizard footer reads these (UI U7: 上一步 / 下一步 at the bottom).
const modeOptions = computed(() => cards.value.map(card => ({
  value: card.key,
  label: currentKey.value === card.key ? `${card.label} · ${t('forwardWizard.steps.mode.currentBadge')}` : card.label,
  description: card.description
})))
const primaryLabel = computed(() => t('forwardWizard.steps.mode.confirmAndContinue'))
const canSubmit = computed(() => Boolean(selectedKey.value) && !loadingCurrent.value)
defineExpose({ submit: handleConfirm, busy: submitLoading, canSubmit, primaryLabel })

const needsNodeXSetup = computed(() =>
  selectedKey.value !== CARD_LOCAL && (!nodeXBaseUrlConfigured.value || !nodeXTokenConfigured.value)
)

function parseBooleanConfig(value) {
  if (value === undefined || value === null) return null
  if (typeof value === 'boolean') return value
  const normalized = String(value).trim().toLowerCase()
  if (!normalized) return null
  return ['1', 'true', 'yes', 'on', 'enabled'].includes(normalized)
}

function selectCard(key) {
  selectedKey.value = key
  submitError.value = ''
  errors.nodeX = ''
}

async function loadCurrentMode() {
  let explicitMode = null
  try {
    const res = await getSystemConfig(runtimeNodeXModeKey)
    explicitMode = parseBooleanConfig(res.data?.value)
  } catch {
    // ignore, fall back to backend-derived mode
  }

  let backend = 'nftables_ansible'
  try {
    const res = await getSystemConfig(runtimeBackendKey)
    backend = String(res.data?.value || 'nftables_ansible').trim().toLowerCase() || 'nftables_ansible'
  } catch {
    // keep default
  }

  const nodeXMode = explicitMode === null ? backend === 'gost' : explicitMode
  currentKey.value = nodeXMode ? CARD_GOST_SINGLE : CARD_LOCAL

  try {
    const res = await getSystemConfig(runtimeNodeXBaseUrlKey)
    nodeXBaseUrl.value = res.data?.value || ''
    nodeXBaseUrlConfigured.value = Boolean(nodeXBaseUrl.value)
  } catch {
    nodeXBaseUrlConfigured.value = false
  }

  try {
    const res = await getSystemConfig(runtimeNodeXTokenKey)
    nodeXToken.value = res.data?.value || ''
    nodeXTokenConfigured.value = Boolean(nodeXToken.value)
  } catch {
    nodeXTokenConfigured.value = false
  }

  try {
    const res = await getSystemConfig(runtimeNodeXTimeoutKey)
    const value = Number(res.data?.value)
    nodeXTimeout.value = Number.isFinite(value) && value > 0 ? value : 15
  } catch {
    nodeXTimeout.value = 15
  }
}

async function handleConfirm() {
  submitError.value = ''
  errors.nodeX = ''
  if (!selectedKey.value) return

  const useNodeX = selectedKey.value !== CARD_LOCAL
  const tunnelType = selectedKey.value === CARD_GOST_TUNNEL ? 2 : 1

  const trimmedBaseUrl = nodeXBaseUrl.value?.trim() || ''
  const trimmedToken = nodeXToken.value?.trim() || ''

  if (useNodeX && !trimmedBaseUrl) {
    errors.nodeX = t('runtime.nodeX.errors.baseUrlRequired')
    return
  }
  if (useNodeX && !trimmedToken) {
    errors.nodeX = t('runtime.nodeX.errors.tokenRequired')
    return
  }

  submitLoading.value = true
  try {
    const tasks = [
      setSystemConfig(runtimeNodeXModeKey, { value: useNodeX, type: 'bool', group: 'forward', description: 'Enable NodeX forward runtime mode' }),
      setSystemConfig(runtimeBackendKey, { value: useNodeX ? 'gost' : 'nftables_ansible', type: 'string', group: 'forward', description: 'Forward runtime backend' })
    ]

    if (useNodeX && (trimmedBaseUrl !== nodeXBaseUrl.value || !nodeXBaseUrlConfigured.value)) {
      tasks.push(setSystemConfig(runtimeNodeXBaseUrlKey, { value: trimmedBaseUrl, type: 'string', group: 'forward', description: 'Forward runtime NodeX base URL' }))
    }
    if (useNodeX && (trimmedToken !== nodeXToken.value || !nodeXTokenConfigured.value)) {
      tasks.push(setSystemConfig(runtimeNodeXTokenKey, { value: trimmedToken, type: 'string', group: 'forward', description: 'Forward runtime NodeX token' }))
    }
    if (useNodeX) {
      const timeout = Number(nodeXTimeout.value)
      tasks.push(setSystemConfig(runtimeNodeXTimeoutKey, { value: Number.isFinite(timeout) && timeout > 0 ? timeout : 15, type: 'number', group: 'forward', description: 'Forward runtime NodeX timeout' }))
    }

    await Promise.all(tasks)
    emit('selected', { nodeXMode: useNodeX, tunnelType })
  } catch {
    submitError.value = t('runtime.nodeX.errors.saveFailed')
  } finally {
    submitLoading.value = false
  }
}

onMounted(async () => {
  await loadCurrentMode()
  loadingCurrent.value = false
})
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

/* The three ways as selectable cards (a radio group underneath). */
.mode-options :deep(.ui-radio-group__items) {
  gap: var(--space-3);
}

.mode-options :deep(.ui-radio) {
  padding: var(--space-4);
  border-radius: var(--radius-md);
  background: var(--bg-elevated);
  box-shadow: 0 0 0 1px var(--separator);
  transition: box-shadow var(--dur-toggle) var(--ease-standard);
}

.mode-options :deep(.ui-radio:has([data-state='checked'])) {
  box-shadow: 0 0 0 2px var(--accent);
}

.nodex-setup {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  padding: var(--space-4);
  border-radius: var(--radius-md);
  background: var(--fill-1);
}

.nodex-setup__hint {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.form-error {
  margin: 0;
}

@media (prefers-reduced-motion: reduce) {
  .mode-options :deep(.ui-radio) {
    transition: none;
  }
}
</style>
