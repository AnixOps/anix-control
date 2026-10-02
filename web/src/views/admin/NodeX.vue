<template>
  <div class="runtime-page nodex-page">
    <UiPageHeader :title="t('forwardNodesPage.title')" :description="t('forwardNodesPage.descriptions.nodexRuntime')">
      <template #meta>
        <UiBadge :tone="nodeXMode ? 'success' : 'warning'" :label="nodeXMode ? t('runtime.nodeX.cards.modeOn') : t('runtime.nodeX.cards.modeOff')" />
      </template>
      <template #actions>
        <UiIconButton variant="secondary" :icon="RefreshCw" :label="t('common.actions.refresh')" :disabled="jobsLoading || statusLoading" data-test="nodex-refresh" @click="refreshAll" />
        <UiButton variant="primary" :loading="saving" data-test="nodex-save" @click="saveNodeXConfig">{{ t('runtime.nodeX.save') }}</UiButton>
      </template>
    </UiPageHeader>

    <ForwardNodesModeNav current="nodexRuntime" />

    <div :class="['runtime-banner', nodeXMode ? 'is-on' : 'is-off']" role="status" data-test="nodex-banner">
      <UiIcon :icon="nodeXMode ? CircleCheck : CircleAlert" :size="18" />
      <div>
        <strong>{{ nodeXMode ? t('runtime.nodeX.enabledBannerTitle') : t('runtime.nodeX.disabledBannerTitle') }}</strong>
        <p>{{ nodeXMode ? t('runtime.nodeX.enabledBannerText') : t('runtime.nodeX.disabledBannerText') }}</p>
        <p>{{ t('runtime.nodeX.heroTextSecondary') }}</p>
      </div>
    </div>

    <UiSection :title="t('runtime.nodeX.configTitle')" :description="t('runtime.nodeX.heroTextPrimary')">
      <div class="runtime-form">
        <UiSwitch v-model="nodeXMode" :label="t('runtime.nodeX.enableModeTitle')" :description="t('runtime.nodeX.enableModeHint')" data-test="nodex-mode" />
        <div class="form-grid">
          <UiTextField
            id="nodex-base-url"
            v-model.trim="nodeXBaseUrl"
            class="form-grid__full"
            :required="nodeXMode"
            placeholder="http://127.0.0.1:18081"
            :label="t('runtime.nodeX.fields.baseUrl')"
            :help="t('runtime.nodeX.fields.baseUrlHint')"
          />
          <UiTextField
            id="nodex-token"
            v-model.trim="nodeXToken"
            :required="nodeXMode"
            autocomplete="off"
            spellcheck="false"
            :placeholder="t('runtimePages.nodeX.placeholders.token')"
            :label="t('runtime.nodeX.fields.token')"
            :help="t('runtime.nodeX.fields.tokenHint')"
          />
          <UiTextField
            id="nodex-timeout"
            v-model.number="nodeXTimeout"
            type="number"
            min="1"
            placeholder="15"
            suffix="s"
            :label="t('runtime.nodeX.fields.timeout')"
            :help="t('runtime.nodeX.fields.timeoutHint')"
          />
        </div>
        <p v-if="validationError" class="form-error" role="alert">{{ validationError }}</p>
      </div>
    </UiSection>

    <RuntimeStatusPanel
      :title="t('runtime.nodeX.probeTitle')"
      :description="t('runtime.nodeX.probeCopy')"
      :loading="statusLoading"
      :error="statusError"
      :has-status="Boolean(statusSummary)"
      :empty-text="t('runtime.nodeX.noStatus')"
      :summary="statusSummary?.summary ? translateRuntimeText(statusSummary.summary) : ''"
      :warnings="(statusSummary?.warnings || []).map(warning => translateRuntimeText(warning))"
      :commands="displayedCommands"
      :doctor-running="doctorRunning"
      :doctor-output="doctorOutput"
      @refresh="fetchNodeXStatus"
      @doctor="runNodeXDoctor"
    >
      <UiGroupedList :title="t('runtime.shared.panelConfig')" heading-tag="h3">
        <UiGroupedListRow :label="t('runtime.nodeX.enableModeTitle')" :value="statusSummary.config?.nodeXMode ? t('runtime.nodeX.cards.modeOn') : t('runtime.nodeX.cards.modeOff')" />
        <UiGroupedListRow :label="t('runtime.nodeX.cards.backend')" :value="runtimeBackendLabel(statusSummary.config?.backend || (nodeXMode ? 'gost' : localBackend))" />
        <UiGroupedListRow :label="t('runtime.nodeX.cards.baseUrl')" :description="statusSummary.config?.baseUrl || '-'" />
        <UiGroupedListRow :label="t('runtime.nodeX.cards.tokenConfigured')" :value="statusSummary.config?.tokenConfigured ? t('runtime.shared.yes') : t('runtime.shared.no')" />
        <UiGroupedListRow :label="t('runtime.nodeX.cards.timeout')" :value="`${statusSummary.config?.timeoutSeconds || nodeXTimeout || 15}s`" />
      </UiGroupedList>
      <UiGroupedList :title="t('runtime.shared.reachability')" heading-tag="h3">
        <UiGroupedListRow :label="t('runtime.shared.reachability')" :description="translateRuntimeText(statusSummary.reachability?.reason)">
          <template #value><UiBadge :status="statusSummary.reachability?.ready ? 'online' : 'offline'" :label="statusSummary.reachability?.ready ? t('runtime.shared.reachable') : t('runtime.shared.notReady')" /></template>
        </UiGroupedListRow>
        <UiGroupedListRow :label="t('runtime.nodeX.cards.health')" :value="statusSummary.health?.ok ? t('runtime.shared.ready') : t('runtime.shared.unavailable')" />
        <UiGroupedListRow :label="t('runtime.nodeX.cards.http')" :value="statusSummary.health?.statusCode || '-'" />
      </UiGroupedList>
      <UiGroupedList :title="t('runtime.shared.runtimeReady')" heading-tag="h3">
        <UiGroupedListRow :label="t('runtime.shared.runtimeReady')" :description="translateRuntimeText(statusSummary.runtimeReady?.reason)">
          <template #value><UiBadge :status="statusSummary.runtimeReady?.ready ? 'online' : 'offline'" :label="statusSummary.runtimeReady?.ready ? t('runtime.shared.ready') : t('runtime.shared.notReady')" /></template>
        </UiGroupedListRow>
        <UiGroupedListRow :label="t('runtime.nodeX.cards.version')" :value="statusSummary.runtimeStatus?.version || '-'" />
        <UiGroupedListRow :label="t('runtime.nodeX.cards.executePath')" :description="statusSummary.runtimeStatus?.executePath || '-'" />
      </UiGroupedList>
    </RuntimeStatusPanel>

    <RuntimeJobsTable
      :title="t('runtime.nodeX.jobsTitle')"
      :description="t('runtime.nodeX.jobsCopy')"
      :jobs="jobs"
      :loading="jobsLoading"
      :empty-text="t('runtime.nodeX.noJobs')"
      storage-key="admin.nodex-jobs"
    />
  </div>
</template>

<script setup>
// 转发节点 › NodeX 运行时 (/admin/forward/nodex): the NodeX control-plane
// settings (system config keys), its probes and the latest gost jobs.
// Execution plane: kept apart from the flux-panel forward pages (AGENTS.md).
// UI U7 restyled it; config keys, calls and payloads are unchanged.
import { computed, onMounted, ref } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import {
  getNodeXRuntimeStatus,
  getSystemConfig,
  listForwardRuntimeJobs,
  runNodeXRuntimeDoctor,
  setSystemConfig
} from '@/api/admin'
import { humanizeForwardRuntimeBackend } from '@/utils/forwardRuntime'
import { isMaskedSecret } from '@/constants/secrets'
import { CircleAlert, CircleCheck, RefreshCw } from '@lucide/vue'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiGroupedList from '@/ui/UiGroupedList.vue'
import UiGroupedListRow from '@/ui/UiGroupedListRow.vue'
import UiIcon from '@/ui/UiIcon.vue'
import UiIconButton from '@/ui/UiIconButton.vue'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import UiSection from '@/ui/UiSection.vue'
import UiSwitch from '@/ui/UiSwitch.vue'
import UiTextField from '@/ui/UiTextField.vue'
import ForwardNodesModeNav from './forward-nodes/ForwardNodesModeNav.vue'
import RuntimeJobsTable from './forward-nodes/RuntimeJobsTable.vue'
import RuntimeStatusPanel from './forward-nodes/RuntimeStatusPanel.vue'
import { normalizeRuntimeJob as normalizeJob, parseRuntimeBoolean, unwrapPayload } from './forward-nodes/forwardNodeModel'

const runtimeNodeXModeKey = 'forward.runtime.nodex_mode'
const runtimeBackendKey = 'forward.runtime_backend'
const runtimeAnsibleBackendKey = 'forward.runtime.ansible.backend'
const runtimeNodeXBaseUrlKey = 'forward.runtime.nodex.base_url'
const runtimeNodeXTokenKey = 'forward.runtime.nodex.token'
const runtimeNodeXTimeoutKey = 'forward.runtime.nodex.timeout_seconds'

const { t, translateLiteral } = useAppI18n()

const nodeXMode = ref(false)
const localBackend = ref('nftables_ansible')
const nodeXBaseUrl = ref('')
const nodeXToken = ref('')
const nodeXTimeout = ref(15)
const saving = ref(false)
const validationError = ref('')

const jobs = ref([])
const jobsLoading = ref(false)
const statusSummary = ref(null)
const statusLoading = ref(false)
const statusError = ref('')
const doctorRunning = ref(false)
const doctorSummary = ref(null)
const doctorOutput = ref('')

const operatorBaseUrl = computed(() => nodeXBaseUrl.value?.trim() || 'http://127.0.0.1:18081')
// A stored token reads MASKED_SECRET: the field keeps it, so saving keeps the
// stored token, and the commands show a placeholder instead.
const operatorToken = computed(() => {
  const token = nodeXToken.value?.trim() || ''
  return token && !isMaskedSecret(token) ? token : '<FORWARD_API_TOKEN>'
})

const fallbackCommands = computed(() => ({
  powerShell: [
    `Invoke-WebRequest '${operatorBaseUrl.value}/health' | Select-Object -ExpandProperty Content`,
    `Invoke-WebRequest '${operatorBaseUrl.value}/api/v2/internal/forward/runtime/status' -Headers @{ Authorization = 'Bearer ${operatorToken.value}' } | Select-Object -ExpandProperty Content`,
    "Invoke-WebRequest 'http://<RELAY_HOST>:<API_PORT>/api/config/services' -Headers @{ Authorization = 'Basic <BASE64(admin:RELAY_API_TOKEN)>' } | Select-Object -ExpandProperty Content"
  ],
  bash: [
    `curl -fsSL '${operatorBaseUrl.value}/health'`,
    `curl -fsSL -H 'Authorization: Bearer ${operatorToken.value}' '${operatorBaseUrl.value}/api/v2/internal/forward/runtime/status'`,
    "curl -fsSL -u 'admin:<RELAY_API_TOKEN>' 'http://<RELAY_HOST>:<API_PORT>/api/config/services'"
  ],
  upgrade: [
    'git clone https://github.com/zdwtest/NodeX.git',
    'cd NodeX/control-plane && go run ./cmd/control-plane --version',
    'cd NodeX/control-plane && go run ./cmd/control-plane --config ../deploy/config/control-plane.yaml --addr :18081 --forward-api-token <FORWARD_API_TOKEN>'
  ],
  references: [
    t('runtimePages.nodeX.references.currentRepoRuntime'),
    t('runtimePages.nodeX.references.currentRepoOnboarding'),
    t('runtimePages.nodeX.references.nodeXRepo'),
    t('runtimePages.nodeX.references.nodeXDoc')
  ]
}))

const displayedCommands = computed(() => {
  const commands = doctorSummary.value?.commands
  if (!commands) {
    return fallbackCommands.value
  }
  return {
    powerShell: Array.isArray(commands.powerShell) ? commands.powerShell : [],
    bash: Array.isArray(commands.bash) ? commands.bash : [],
    upgrade: Array.isArray(commands.upgrade) ? commands.upgrade : [],
    references: Array.isArray(commands.references) ? commands.references : []
  }
})

function translateRuntimeText(value, fallback = '-') {
  const text = String(value ?? '').trim()
  if (!text) {
    return fallback
  }
  return translateLiteral(text)
}

function resolveRuntimeError(error, fallbackKey) {
  return translateRuntimeText(error?.response?.data?.msg || error?.message, t(fallbackKey))
}

function runtimeBackendLabel(value) {
  return humanizeForwardRuntimeBackend(t, value)
}

async function fetchNodeXConfig() {
  validationError.value = ''

  let explicitMode = null
  try {
    const res = await getSystemConfig(runtimeNodeXModeKey)
    explicitMode = parseRuntimeBoolean(res.data?.value)
  } catch (error) {
    console.error('get NodeX mode config failed:', error)
  }

  let backend = 'gost'
  try {
    const res = await getSystemConfig(runtimeBackendKey)
    backend = String(res.data?.value || 'gost').trim().toLowerCase() || 'gost'
  } catch (error) {
    console.error('get forward runtime backend failed:', error)
  }

  try {
    const res = await getSystemConfig(runtimeAnsibleBackendKey)
    localBackend.value = String(res.data?.value || 'nftables_ansible').trim().toLowerCase() || 'nftables_ansible'
  } catch (error) {
    console.error('get local ansible backend failed:', error)
    localBackend.value = 'nftables_ansible'
  }

  nodeXMode.value = explicitMode === null ? backend === 'gost' : explicitMode

  try {
    const res = await getSystemConfig(runtimeNodeXBaseUrlKey)
    nodeXBaseUrl.value = res.data?.value || ''
  } catch (error) {
    console.error('get NodeX base URL failed:', error)
  }

  try {
    const res = await getSystemConfig(runtimeNodeXTokenKey)
    nodeXToken.value = res.data?.value || ''
  } catch (error) {
    console.error('get NodeX token failed:', error)
  }

  try {
    const res = await getSystemConfig(runtimeNodeXTimeoutKey)
    const value = Number(res.data?.value)
    nodeXTimeout.value = Number.isFinite(value) && value > 0 ? value : 15
  } catch (error) {
    console.error('get NodeX timeout failed:', error)
  }
}

async function saveNodeXConfig() {
  validationError.value = ''

  const trimmedBaseUrl = nodeXBaseUrl.value?.trim() || ''
  const trimmedToken = nodeXToken.value?.trim() || ''
  const timeout = Number(nodeXTimeout.value)

  if (nodeXMode.value && !trimmedBaseUrl) {
    validationError.value = t('runtime.nodeX.errors.baseUrlRequired')
    return
  }
  if (nodeXMode.value && !trimmedToken) {
    validationError.value = t('runtime.nodeX.errors.tokenRequired')
    return
  }

  saving.value = true
  try {
    await Promise.all([
      setSystemConfig(runtimeNodeXModeKey, {
        value: nodeXMode.value,
        type: 'bool',
        group: 'forward',
        description: 'Enable NodeX forward runtime mode'
      }),
      setSystemConfig(runtimeBackendKey, {
        value: nodeXMode.value ? 'gost' : localBackend.value,
        type: 'string',
        group: 'forward',
        description: 'Forward runtime backend'
      }),
      setSystemConfig(runtimeNodeXBaseUrlKey, {
        value: trimmedBaseUrl,
        type: 'string',
        group: 'forward',
        description: 'Forward runtime NodeX base URL'
      }),
      setSystemConfig(runtimeNodeXTokenKey, {
        value: trimmedToken,
        type: 'string',
        group: 'forward',
        description: 'Forward runtime NodeX token'
      }),
      setSystemConfig(runtimeNodeXTimeoutKey, {
        value: Number.isFinite(timeout) && timeout > 0 ? timeout : 15,
        type: 'number',
        group: 'forward',
        description: 'Forward runtime NodeX timeout'
      })
    ])
    await fetchNodeXConfig()
    await fetchNodeXStatus()
    await fetchJobs()
  } catch (error) {
    validationError.value = resolveRuntimeError(error, 'runtime.nodeX.errors.saveFailed')
  } finally {
    saving.value = false
  }
}

async function fetchJobs() {
  jobsLoading.value = true
  try {
    const res = await listForwardRuntimeJobs({ backend: 'gost', limit: 10 })
    const payload = unwrapPayload(res)
    const list = Array.isArray(payload?.list) ? payload.list : Array.isArray(payload) ? payload : []
    jobs.value = list.map(job => normalizeJob(job, t('runtime.shared.unknown')))
  } catch (error) {
    console.error('get NodeX runtime jobs failed:', error)
    jobs.value = []
  } finally {
    jobsLoading.value = false
  }
}

async function fetchNodeXStatus() {
  statusLoading.value = true
  statusError.value = ''
  try {
    const res = await getNodeXRuntimeStatus()
    statusSummary.value = unwrapPayload(res) || null
  } catch (error) {
    statusError.value = resolveRuntimeError(error, 'runtime.nodeX.errors.fetchStatusFailed')
    statusSummary.value = null
  } finally {
    statusLoading.value = false
  }
}

async function runNodeXDoctor() {
  doctorRunning.value = true
  doctorOutput.value = ''
  try {
    const res = await runNodeXRuntimeDoctor()
    const payload = unwrapPayload(res) || null
    doctorSummary.value = payload
    statusSummary.value = payload
    doctorOutput.value = JSON.stringify(payload, null, 2)
  } catch (error) {
    doctorOutput.value = resolveRuntimeError(error, 'runtime.nodeX.errors.doctorFailed')
  } finally {
    doctorRunning.value = false
  }
}

async function refreshAll() {
  await Promise.all([fetchNodeXConfig(), fetchNodeXStatus(), fetchJobs()])
}

onMounted(async () => {
  await refreshAll()
})
</script>

<style scoped src="./forward-nodes/runtime-page.css"></style>
