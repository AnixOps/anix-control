<template>
  <div class="step-form">
    <p class="step-intro">{{ t('forwardWizard.steps.mode.intro') }}</p>

    <div v-if="loadingCurrent" class="mode-loading">{{ t('forwardWizard.loading') }}</div>

    <template v-else>
      <div class="mode-cards">
        <button
          v-for="card in cards"
          :key="card.key"
          type="button"
          class="mode-card"
          :class="{ 'mode-card-active': selectedKey === card.key }"
          @click="selectCard(card.key)"
        >
          <div class="mode-card-head">
            <span class="mode-card-label">{{ card.label }}</span>
            <span v-if="currentKey === card.key" class="mode-card-badge">{{ t('forwardWizard.steps.mode.currentBadge') }}</span>
          </div>
          <p class="mode-card-description">{{ card.description }}</p>
        </button>
      </div>

      <div v-if="needsNodeXSetup" class="nodex-setup">
        <p class="nodex-setup-hint">{{ t('forwardWizard.steps.mode.nodeXSetupHint') }}</p>
        <div class="form-grid">
          <label class="form-group">
            <span>{{ t('runtime.nodeX.fields.baseUrl') }}</span>
            <input v-model.trim="nodeXBaseUrl" type="text" placeholder="https://nodex.example.com" />
            <p class="hint">{{ t('runtime.nodeX.fields.baseUrlHint') }}</p>
          </label>
          <label class="form-group">
            <span>{{ t('runtime.nodeX.fields.token') }}</span>
            <input v-model.trim="nodeXToken" type="password" />
            <p class="hint">{{ t('runtime.nodeX.fields.tokenHint') }}</p>
          </label>
        </div>
        <p v-if="errors.nodeX" class="form-error">{{ errors.nodeX }}</p>
      </div>

      <p v-if="submitError" class="form-error">{{ submitError }}</p>

      <div class="step-actions">
        <button class="btn btn-primary" :disabled="!selectedKey || submitLoading" @click="handleConfirm">
          {{ submitLoading ? t('runtime.nodeX.saveLoading') : t('forwardWizard.steps.mode.confirmAndContinue') }}
        </button>
      </div>
    </template>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { getSystemConfig, setSystemConfig } from '@/api/admin'

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
.step-form { display: flex; flex-direction: column; gap: 16px; }
.step-intro { margin: 0; color: var(--text-secondary); line-height: 1.6; }
.mode-loading { color: var(--text-secondary); text-align: center; padding: 40px 0; }
.mode-cards { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 16px; }
.mode-card {
  display: flex;
  flex-direction: column;
  gap: 8px;
  text-align: left;
  border: 1px solid var(--border-color);
  border-radius: 14px;
  padding: 16px;
  background: var(--bg-color);
  color: var(--text-color);
  cursor: pointer;
}
.mode-card-active { border-color: var(--primary-color); box-shadow: 0 0 0 2px var(--primary-color) inset; }
.mode-card-head { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.mode-card-label { font-weight: 600; }
.mode-card-badge { font-size: 11px; padding: 2px 8px; border-radius: 999px; background: var(--primary-color); color: #fff; }
.mode-card-description { margin: 0; font-size: 13px; color: var(--text-secondary); line-height: 1.5; }
.nodex-setup { border: 1px solid var(--border-color); border-radius: 14px; padding: 14px 16px; background: var(--bg-color); display: flex; flex-direction: column; gap: 12px; }
.nodex-setup-hint { margin: 0; color: var(--text-secondary); font-size: 13px; }
.form-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; }
.form-group { display: flex; flex-direction: column; gap: 8px; }
.form-group input { border: 1px solid var(--border-color); border-radius: 12px; background: var(--bg-color); color: var(--text-color); padding: 12px 14px; }
.hint { margin: 0; font-size: 12px; color: var(--text-secondary); }
.form-error { margin: 0; color: #b91c1c; }
.step-actions { display: flex; justify-content: flex-end; }
.btn { display: inline-flex; align-items: center; justify-content: center; border: 1px solid transparent; border-radius: 12px; padding: 10px 16px; cursor: pointer; }
.btn-primary { background: var(--primary-color); color: #fff; }
@media (max-width: 720px) {
  .mode-cards { grid-template-columns: 1fr; }
  .form-grid { grid-template-columns: 1fr; }
}
</style>
