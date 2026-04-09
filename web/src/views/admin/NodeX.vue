<template>
  <div class="nodex-page">
    <section class="hero-card">
      <div class="hero-copy">
        <p class="eyebrow">{{ t('runtime.nodeX.heroEyebrow') }}</p>
        <h2>{{ t('runtime.nodeX.title') }}</h2>
        <p class="hero-text">
          {{ t('runtime.nodeX.heroTextPrimary') }}
        </p>
        <p class="hero-text">
          {{ t('runtime.nodeX.heroTextSecondary') }}
        </p>
      </div>
      <div class="hero-actions">
        <router-link class="btn btn-secondary" to="/admin/forward/nodes">{{ t('forwardSuite.nav.nodeXTopology') }}</router-link>
        <router-link class="btn btn-secondary" to="/admin/forward/agents">{{ t('forwardSuite.nav.nodeXAgents') }}</router-link>
        <router-link class="btn btn-secondary" to="/admin/forward/local">{{ t('forwardSuite.nav.localRuntime') }}</router-link>
        <router-link class="btn btn-secondary" to="/admin/system">{{ t('pageTitles.admin.system') }}</router-link>
        <button class="btn btn-secondary" :disabled="jobsLoading || statusLoading" @click="refreshAll">
          {{ jobsLoading || statusLoading ? t('runtime.nodeX.refreshLoading') : t('common.actions.refresh') }}
        </button>
        <button class="btn btn-primary" :disabled="saving" @click="saveNodeXConfig">
          {{ saving ? t('runtime.nodeX.saveLoading') : t('runtime.nodeX.save') }}
        </button>
      </div>
    </section>

    <section :class="['mode-banner', nodeXMode ? 'banner-success' : 'banner-warning']">
      <strong>{{ nodeXMode ? t('runtime.nodeX.enabledBannerTitle') : t('runtime.nodeX.disabledBannerTitle') }}</strong>
      <span>
        {{
          nodeXMode
            ? t('runtime.nodeX.enabledBannerText')
            : t('runtime.nodeX.disabledBannerText')
        }}
      </span>
    </section>

    <section class="panel-card">
      <div class="section-head">
        <div>
          <p class="eyebrow">{{ t('runtime.nodeX.configEyebrow') }}</p>
          <h3>{{ t('runtime.nodeX.configTitle') }}</h3>
        </div>
      </div>

      <div class="form-grid">
        <label class="toggle-card">
          <span class="toggle-copy">
            <strong>{{ t('runtime.nodeX.enableModeTitle') }}</strong>
            <span>{{ t('runtime.nodeX.enableModeHint') }}</span>
          </span>
          <input v-model="nodeXMode" type="checkbox" />
        </label>

        <div class="form-group">
          <label for="nodex-base-url">{{ t('runtime.nodeX.fields.baseUrl') }}</label>
          <input
            id="nodex-base-url"
            v-model.trim="nodeXBaseUrl"
            type="text"
            placeholder="http://127.0.0.1:18081"
          />
          <p class="hint">{{ t('runtime.nodeX.fields.baseUrlHint') }}</p>
        </div>

        <div class="form-group">
          <label for="nodex-token">{{ t('runtime.nodeX.fields.token') }}</label>
          <input
            id="nodex-token"
            v-model.trim="nodeXToken"
            type="text"
            placeholder="shared forward-api-token"
          />
          <p class="hint">{{ t('runtime.nodeX.fields.tokenHint') }}</p>
        </div>

        <div class="form-group">
          <label for="nodex-timeout">{{ t('runtime.nodeX.fields.timeout') }}</label>
          <input
            id="nodex-timeout"
            v-model.number="nodeXTimeout"
            type="number"
            min="1"
            placeholder="15"
          />
          <p class="hint">{{ t('runtime.nodeX.fields.timeoutHint') }}</p>
        </div>
      </div>

      <p v-if="validationError" class="form-error">{{ validationError }}</p>
    </section>

    <section class="panel-card">
      <div class="section-head">
        <div>
          <p class="eyebrow">{{ t('runtime.nodeX.probeEyebrow') }}</p>
          <h3>{{ t('runtime.nodeX.probeTitle') }}</h3>
          <p class="section-copy">
            {{ t('runtime.nodeX.probeCopy') }}
          </p>
        </div>
        <div class="section-actions">
          <button class="btn btn-secondary btn-sm" :disabled="statusLoading" @click="fetchNodeXStatus">
            {{ statusLoading ? t('runtime.shared.loading') : t('runtime.shared.refreshStatus') }}
          </button>
          <button class="btn btn-secondary btn-sm" :disabled="doctorRunning" @click="runNodeXDoctor">
            {{ doctorRunning ? t('runtime.shared.runningDoctor') : t('runtime.shared.runDoctor') }}
          </button>
        </div>
      </div>

      <div v-if="statusLoading" class="state-card">{{ t('runtime.nodeX.loadingStatus') }}</div>
      <div v-else-if="statusError" class="state-card state-error">{{ statusError }}</div>
      <div v-else-if="statusSummary" class="status-grid">
        <article class="status-card">
          <p class="metric-label">{{ t('runtime.shared.panelConfig') }}</p>
          <p class="metric-value">{{ statusSummary.config?.nodeXMode ? t('runtime.nodeX.cards.modeOn') : t('runtime.nodeX.cards.modeOff') }}</p>
          <p class="metric-detail">{{ t('runtime.nodeX.cards.backend') }}: {{ runtimeBackendLabel(statusSummary.config?.backend || (nodeXMode ? 'gost' : localBackend)) }}</p>
          <p class="metric-detail">{{ t('runtime.nodeX.cards.baseUrl') }}: {{ statusSummary.config?.baseUrl || '-' }}</p>
          <p class="metric-detail">{{ t('runtime.nodeX.cards.tokenConfigured') }}: {{ statusSummary.config?.tokenConfigured ? t('runtime.shared.yes') : t('runtime.shared.no') }}</p>
          <p class="metric-detail">{{ t('runtime.nodeX.cards.timeout') }}: {{ statusSummary.config?.timeoutSeconds || nodeXTimeout || 15 }}s</p>
        </article>

        <article class="status-card">
          <p class="metric-label">{{ t('runtime.shared.reachability') }}</p>
          <p class="metric-value">{{ statusSummary.reachability?.ready ? t('runtime.shared.reachable') : t('runtime.shared.notReady') }}</p>
          <p class="metric-detail">{{ translateRuntimeText(statusSummary.reachability?.reason) }}</p>
          <p class="metric-detail">{{ t('runtime.nodeX.cards.health') }}: {{ statusSummary.health?.ok ? t('runtime.shared.ready') : t('runtime.shared.unavailable') }}</p>
          <p class="metric-detail">{{ t('runtime.nodeX.cards.http') }}: {{ statusSummary.health?.statusCode || '-' }}</p>
        </article>

        <article class="status-card">
          <p class="metric-label">{{ t('runtime.shared.runtimeReady') }}</p>
          <p class="metric-value">{{ statusSummary.runtimeReady?.ready ? t('runtime.shared.ready') : t('runtime.shared.notReady') }}</p>
          <p class="metric-detail">{{ translateRuntimeText(statusSummary.runtimeReady?.reason) }}</p>
          <p class="metric-detail">{{ t('runtime.nodeX.cards.version') }}: {{ statusSummary.runtimeStatus?.version || '-' }}</p>
          <p class="metric-detail">{{ t('runtime.nodeX.cards.executePath') }}: {{ statusSummary.runtimeStatus?.executePath || '-' }}</p>
        </article>
      </div>
      <div v-else class="state-card">{{ t('runtime.nodeX.noStatus') }}</div>

      <div v-if="statusSummary?.summary" class="summary-card">
        {{ translateRuntimeText(statusSummary.summary) }}
      </div>

      <div v-if="statusSummary?.warnings?.length" class="warning-list">
        <p class="metric-label">{{ t('runtime.shared.warnings') }}</p>
        <code v-for="warning in statusSummary.warnings" :key="warning">{{ translateRuntimeText(warning) }}</code>
      </div>

      <div class="command-block">
        <p class="metric-label">{{ t('runtime.shared.powerShell') }}</p>
        <code v-for="command in displayedCommands.powerShell" :key="`ps-${command}`">{{ command }}</code>
        <p class="metric-label">{{ t('runtime.shared.bash') }}</p>
        <code v-for="command in displayedCommands.bash" :key="`bash-${command}`">{{ command }}</code>
        <p class="metric-label">{{ t('runtime.shared.bootstrapVerify') }}</p>
        <code v-for="command in displayedCommands.upgrade" :key="`verify-${command}`">{{ command }}</code>
        <p class="metric-label">{{ t('runtime.shared.references') }}</p>
        <code v-for="reference in displayedCommands.references" :key="reference">{{ reference }}</code>
      </div>

      <div class="doctor-output">
        <p class="metric-label">{{ t('runtime.shared.doctorOutput') }}</p>
        <pre>{{ doctorOutput || t('runtime.shared.doctorNotExecuted') }}</pre>
      </div>
    </section>

    <section class="panel-card">
      <div class="section-head">
        <div>
          <p class="eyebrow">{{ t('runtime.nodeX.jobsEyebrow') }}</p>
          <h3>{{ t('runtime.nodeX.jobsTitle') }}</h3>
          <p class="section-copy">{{ t('runtime.nodeX.jobsCopy') }}</p>
        </div>
      </div>

      <div v-if="jobsLoading" class="state-card">{{ t('runtime.nodeX.loadingJobs') }}</div>
      <div v-else-if="jobs.length" class="job-list">
        <article v-for="job in jobs" :key="job.id" class="job-item">
          <div class="job-main">
            <div>
              <strong>#{{ job.id }} {{ job.action }}</strong>
              <p class="job-meta">{{ t('runtime.nodeX.jobMeta', { forwardId: job.forwardId || '-', tunnelId: job.tunnelId || '-', nodeId: job.nodeId || '-' }) }}</p>
            </div>
            <div class="job-side">
              <span :class="['status-chip', `status-${job.status}`]">{{ runtimeJobStatusLabel(job.status) }}</span>
              <span class="job-time">{{ formatJobTime(job) }}</span>
            </div>
          </div>
          <code v-if="job.message" class="job-message">{{ translateRuntimeText(job.message) }}</code>
        </article>
      </div>
      <div v-else class="state-card">{{ t('runtime.nodeX.noJobs') }}</div>
    </section>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import {
  getNodeXRuntimeStatus,
  getSystemConfig,
  listForwardRuntimeJobs,
  runNodeXRuntimeDoctor,
  setSystemConfig
} from '@/api/admin'

const runtimeNodeXModeKey = 'forward.runtime.nodex_mode'
const runtimeBackendKey = 'forward.runtime_backend'
const runtimeAnsibleBackendKey = 'forward.runtime.ansible.backend'
const runtimeNodeXBaseUrlKey = 'forward.runtime.nodex.base_url'
const runtimeNodeXTokenKey = 'forward.runtime.nodex.token'
const runtimeNodeXTimeoutKey = 'forward.runtime.nodex.timeout_seconds'

const { t, formatDateTime, translateLiteral } = useAppI18n()

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
const operatorToken = computed(() => nodeXToken.value?.trim() || '<FORWARD_API_TOKEN>')

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
    'Current repo: docs/reference/runtime.md',
    'Current repo: docs/guide/forward-relay-onboarding.md',
    'NodeX repo: https://github.com/zdwtest/NodeX',
    'NodeX doc: docs/forward-runtime-relay-onboarding.md'
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
  const normalized = String(value ?? '').trim().toLowerCase()
  if (normalized === 'gost') {
    return t('runtime.nodeX.backends.gost')
  }
  if (normalized === 'nftables_ansible') {
    return t('runtime.localRuntime.backends.nftables.label')
  }
  if (normalized === 'iptables_ansible') {
    return t('runtime.localRuntime.backends.iptables.label')
  }
  return value || '-'
}

function parseRuntimeBoolean(value) {
  if (typeof value === 'boolean') {
    return value
  }
  if (typeof value === 'number') {
    return value !== 0
  }

  const normalized = String(value ?? '').trim().toLowerCase()
  if (!normalized) {
    return null
  }
  if (['1', 'true', 'yes', 'on'].includes(normalized)) {
    return true
  }
  if (['0', 'false', 'no', 'off'].includes(normalized)) {
    return false
  }
  return null
}

function extractPayload(response) {
  return response?.data?.data ?? response?.data ?? response
}

function normalizeRuntimeJob(job) {
  return {
    id: job?.id,
    action: job?.action || 'unknown',
    forwardId: job?.forwardId ?? job?.forward_id ?? null,
    tunnelId: job?.tunnelId ?? job?.tunnel_id ?? null,
    nodeId: job?.nodeId ?? job?.node_id ?? null,
    status: Number(job?.status ?? 0),
    message: String(job?.result || job?.error || '').trim(),
    completedAt: job?.completedAt ?? job?.completed_at ?? null,
    updatedAt: job?.updatedAt ?? job?.updated_at ?? null,
    createdAt: job?.createdAt ?? job?.created_at ?? null
  }
}

function runtimeJobStatusLabel(status) {
  switch (Number(status)) {
    case 0:
      return t('runtime.shared.pending')
    case 1:
      return t('runtime.shared.running')
    case 2:
      return t('runtime.shared.success')
    case 3:
      return t('runtime.shared.failed')
    default:
      return t('runtime.shared.unknown')
  }
}

function formatJobTime(job) {
  const raw = job?.completedAt || job?.updatedAt || job?.createdAt
  if (!raw) {
    return '-'
  }
  return formatDateTime(raw) || String(raw)
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
    const payload = extractPayload(res)
    const list = Array.isArray(payload?.list) ? payload.list : Array.isArray(payload) ? payload : []
    jobs.value = list.map(normalizeRuntimeJob)
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
    statusSummary.value = extractPayload(res) || null
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
    const payload = extractPayload(res) || null
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

<style scoped>
.nodex-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.hero-card,
.panel-card,
.mode-banner {
  border: 1px solid var(--border-color);
  border-radius: 20px;
  background: var(--surface-color);
}

.hero-card {
  display: flex;
  justify-content: space-between;
  gap: 20px;
  padding: 24px;
  background:
    radial-gradient(circle at top right, rgba(37, 99, 235, 0.14), transparent 28%),
    linear-gradient(180deg, rgba(15, 23, 42, 0.03), transparent 70%),
    var(--surface-color);
}

.hero-copy {
  max-width: 760px;
}

.hero-copy h2,
.section-head h3 {
  margin: 6px 0 0;
}

.hero-text,
.section-copy,
.hint,
.job-meta,
.metric-detail {
  margin: 8px 0 0;
  color: var(--text-secondary);
  line-height: 1.6;
}

.hero-actions,
.section-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  align-items: flex-start;
}

.eyebrow {
  margin: 0;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.18em;
  text-transform: uppercase;
  color: var(--text-secondary);
}

.btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  border: 1px solid transparent;
  border-radius: 12px;
  padding: 10px 16px;
  font-size: 14px;
  font-weight: 600;
  text-decoration: none;
  cursor: pointer;
  transition: 0.2s ease;
}

.btn:hover {
  transform: translateY(-1px);
}

.btn:disabled {
  opacity: 0.65;
  cursor: not-allowed;
  transform: none;
}

.btn-primary {
  background: var(--primary-color);
  color: #fff;
}

.btn-secondary {
  background: var(--surface-color);
  color: var(--text-color);
  border-color: var(--border-color);
}

.btn-sm {
  padding: 8px 12px;
  font-size: 12px;
}

.mode-banner {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  padding: 16px 18px;
}

.banner-success {
  background: rgba(16, 185, 129, 0.12);
  border-color: rgba(16, 185, 129, 0.22);
}

.banner-warning {
  background: rgba(245, 158, 11, 0.12);
  border-color: rgba(245, 158, 11, 0.22);
}

.panel-card {
  padding: 20px;
}

.section-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 18px;
}

.form-grid,
.status-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: 16px;
}

.toggle-card,
.form-group,
.status-card {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 16px;
  border: 1px solid var(--border-color);
  border-radius: 16px;
  background: rgba(15, 23, 42, 0.03);
}

.toggle-card {
  justify-content: space-between;
  cursor: pointer;
}

.toggle-card input {
  width: 20px;
  height: 20px;
}

.toggle-copy {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.toggle-copy span {
  color: var(--text-secondary);
  line-height: 1.5;
}

.form-group input {
  width: 100%;
  padding: 12px 14px;
  border: 1px solid var(--border-color);
  border-radius: 12px;
  background: var(--surface-color);
  color: var(--text-color);
}

.form-error,
.state-error {
  color: #b91c1c;
}

.metric-label {
  font-size: 12px;
  color: var(--text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.08em;
}

.metric-value {
  font-size: 20px;
  font-weight: 700;
}

.summary-card,
.state-card {
  padding: 14px 16px;
  border-radius: 14px;
  background: rgba(15, 23, 42, 0.04);
  color: var(--text-secondary);
}

.warning-list,
.command-block,
.doctor-output {
  margin-top: 18px;
}

.warning-list code,
.command-block code,
.job-message {
  display: block;
  margin-top: 8px;
  padding: 10px 12px;
  border-radius: 12px;
  background: rgba(15, 23, 42, 0.04);
  font-family: Consolas, 'Courier New', monospace;
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-word;
}

.doctor-output pre {
  margin: 8px 0 0;
  padding: 12px;
  border-radius: 12px;
  background: rgba(15, 23, 42, 0.05);
  max-height: 240px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-word;
}

.job-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.job-item {
  padding: 14px 16px;
  border: 1px solid var(--border-color);
  border-radius: 16px;
  background: rgba(15, 23, 42, 0.03);
}

.job-main,
.job-side {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.job-main {
  align-items: flex-start;
}

.job-time {
  color: var(--text-secondary);
  font-size: 12px;
}

.status-chip {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 74px;
  min-height: 28px;
  padding: 6px 10px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 700;
}

.status-0 {
  background: rgba(245, 158, 11, 0.16);
  color: #b45309;
}

.status-1 {
  background: rgba(59, 130, 246, 0.14);
  color: #1d4ed8;
}

.status-2 {
  background: rgba(16, 185, 129, 0.14);
  color: #047857;
}

.status-3 {
  background: rgba(239, 68, 68, 0.14);
  color: #b91c1c;
}

@media (max-width: 900px) {
  .hero-card,
  .section-head,
  .mode-banner,
  .job-main,
  .job-side {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>
