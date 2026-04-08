<template>
  <div class="nodex-page">
    <section class="hero-card">
      <div class="hero-copy">
        <p class="eyebrow">Private Runtime</p>
        <h2>NodeX Runtime</h2>
        <p class="hero-text">
          Dedicated operator entry for the stateful NodeX/gost path. This page probes the configured NodeX control
          plane directly, even when the current global runtime backend is still a local Ansible backend.
        </p>
        <p class="hero-text">
          Local Ansible execution now lives under Local Runtime and Ansible Machines. Node "online" still means TCP
          reachability only and is not proof that NodeX or the relay gost API is already attached.
        </p>
      </div>
      <div class="hero-actions">
        <router-link class="btn btn-secondary" to="/admin/forward/nodes">NodeX Topology</router-link>
        <router-link class="btn btn-secondary" to="/admin/forward/agents">NodeX Agents</router-link>
        <router-link class="btn btn-secondary" to="/admin/forward/local">Local Runtime</router-link>
        <router-link class="btn btn-secondary" to="/admin/system">System Overview</router-link>
        <button class="btn btn-secondary" :disabled="jobsLoading || statusLoading" @click="refreshAll">
          {{ jobsLoading || statusLoading ? 'Refreshing...' : 'Refresh' }}
        </button>
        <button class="btn btn-primary" :disabled="saving" @click="saveNodeXConfig">
          {{ saving ? 'Saving...' : 'Save NodeX Config' }}
        </button>
      </div>
    </section>

    <section :class="['mode-banner', nodeXMode ? 'banner-success' : 'banner-warning']">
      <strong>{{ nodeXMode ? 'NodeX Mode is enabled' : 'NodeX Mode is disabled' }}</strong>
      <span>
        {{
          nodeXMode
            ? 'Panel forward jobs can route through NodeX/gost, but each runtime job still has to succeed before relay attachment is real.'
            : 'You can validate the configured NodeX control plane here first, then switch the global backend when you are ready.'
        }}
      </span>
    </section>

    <section class="panel-card">
      <div class="section-head">
        <div>
          <p class="eyebrow">Configuration</p>
          <h3>NodeX Control Plane</h3>
        </div>
      </div>

      <div class="form-grid">
        <label class="toggle-card">
          <span class="toggle-copy">
            <strong>Enable NodeX Mode</strong>
            <span>Writes `forward.runtime.nodex_mode=true` and `forward.runtime_backend=gost`.</span>
          </span>
          <input v-model="nodeXMode" type="checkbox" />
        </label>

        <div class="form-group">
          <label for="nodex-base-url">NodeX Base URL</label>
          <input
            id="nodex-base-url"
            v-model.trim="nodeXBaseUrl"
            type="text"
            placeholder="http://127.0.0.1:18081"
          />
          <p class="hint">This must point to the NodeX control-plane, not directly to the relay gost API.</p>
        </div>

        <div class="form-group">
          <label for="nodex-token">NodeX Token</label>
          <input
            id="nodex-token"
            v-model.trim="nodeXToken"
            type="text"
            placeholder="shared forward-api-token"
          />
          <p class="hint">Matches the NodeX control-plane `--forward-api-token` value.</p>
        </div>

        <div class="form-group">
          <label for="nodex-timeout">Timeout (seconds)</label>
          <input
            id="nodex-timeout"
            v-model.number="nodeXTimeout"
            type="number"
            min="1"
            placeholder="15"
          />
          <p class="hint">Used by the panel when probing or executing NodeX runtime requests.</p>
        </div>
      </div>

      <p v-if="validationError" class="form-error">{{ validationError }}</p>
    </section>

    <section class="panel-card">
      <div class="section-head">
        <div>
          <p class="eyebrow">NodeX Probe</p>
          <h3>Health And Runtime Status</h3>
          <p class="section-copy">
            These checks always target the configured NodeX control plane. They do not depend on the currently active
            runtime backend.
          </p>
        </div>
        <div class="section-actions">
          <button class="btn btn-secondary btn-sm" :disabled="statusLoading" @click="fetchNodeXStatus">
            {{ statusLoading ? 'Loading...' : 'Refresh status' }}
          </button>
          <button class="btn btn-secondary btn-sm" :disabled="doctorRunning" @click="runNodeXDoctor">
            {{ doctorRunning ? 'Running...' : 'Run doctor' }}
          </button>
        </div>
      </div>

      <div v-if="statusLoading" class="state-card">Loading NodeX runtime status...</div>
      <div v-else-if="statusError" class="state-card state-error">{{ statusError }}</div>
      <div v-else-if="statusSummary" class="status-grid">
        <article class="status-card">
          <p class="metric-label">Panel Config</p>
          <p class="metric-value">{{ statusSummary.config?.nodeXMode ? 'NodeX mode on' : 'NodeX mode off' }}</p>
          <p class="metric-detail">Backend: {{ statusSummary.config?.backend || (nodeXMode ? 'gost' : localBackend) }}</p>
          <p class="metric-detail">Base URL: {{ statusSummary.config?.baseUrl || '-' }}</p>
          <p class="metric-detail">Token configured: {{ statusSummary.config?.tokenConfigured ? 'Yes' : 'No' }}</p>
          <p class="metric-detail">Timeout: {{ statusSummary.config?.timeoutSeconds || nodeXTimeout || 15 }}s</p>
        </article>

        <article class="status-card">
          <p class="metric-label">Reachability</p>
          <p class="metric-value">{{ statusSummary.reachability?.ready ? 'Reachable' : 'Not ready' }}</p>
          <p class="metric-detail">{{ statusSummary.reachability?.reason || '-' }}</p>
          <p class="metric-detail">Health: {{ statusSummary.health?.ok ? 'OK' : 'Unavailable' }}</p>
          <p class="metric-detail">HTTP: {{ statusSummary.health?.statusCode || '-' }}</p>
        </article>

        <article class="status-card">
          <p class="metric-label">Runtime Ready</p>
          <p class="metric-value">{{ statusSummary.runtimeReady?.ready ? 'Ready' : 'Not ready' }}</p>
          <p class="metric-detail">{{ statusSummary.runtimeReady?.reason || '-' }}</p>
          <p class="metric-detail">Version: {{ statusSummary.runtimeStatus?.version || '-' }}</p>
          <p class="metric-detail">Execute path: {{ statusSummary.runtimeStatus?.executePath || '-' }}</p>
        </article>
      </div>
      <div v-else class="state-card">No NodeX runtime status loaded yet.</div>

      <div v-if="statusSummary?.summary" class="summary-card">
        {{ statusSummary.summary }}
      </div>

      <div v-if="statusSummary?.warnings?.length" class="warning-list">
        <p class="metric-label">Warnings</p>
        <code v-for="warning in statusSummary.warnings" :key="warning">{{ warning }}</code>
      </div>

      <div class="command-block">
        <p class="metric-label">PowerShell</p>
        <code v-for="command in displayedCommands.powerShell" :key="`ps-${command}`">{{ command }}</code>
        <p class="metric-label">Bash</p>
        <code v-for="command in displayedCommands.bash" :key="`bash-${command}`">{{ command }}</code>
        <p class="metric-label">Bootstrap / Verify</p>
        <code v-for="command in displayedCommands.upgrade" :key="`verify-${command}`">{{ command }}</code>
        <p class="metric-label">References</p>
        <code v-for="reference in displayedCommands.references" :key="reference">{{ reference }}</code>
      </div>

      <div class="doctor-output">
        <p class="metric-label">Doctor Output</p>
        <pre>{{ doctorOutput || 'Doctor has not been executed yet.' }}</pre>
      </div>
    </section>

    <section class="panel-card">
      <div class="section-head">
        <div>
          <p class="eyebrow">Runtime Jobs</p>
          <h3>Latest gost Jobs</h3>
          <p class="section-copy">Recent panel-side runtime audit rows filtered to the `gost` backend.</p>
        </div>
      </div>

      <div v-if="jobsLoading" class="state-card">Loading runtime jobs...</div>
      <div v-else-if="jobs.length" class="job-list">
        <article v-for="job in jobs" :key="job.id" class="job-item">
          <div class="job-main">
            <div>
              <strong>#{{ job.id }} {{ job.action }}</strong>
              <p class="job-meta">forward {{ job.forwardId || '-' }} / tunnel {{ job.tunnelId || '-' }} / node {{ job.nodeId || '-' }}</p>
            </div>
            <div class="job-side">
              <span :class="['status-chip', `status-${job.status}`]">{{ runtimeJobStatusLabel(job.status) }}</span>
              <span class="job-time">{{ formatJobTime(job) }}</span>
            </div>
          </div>
          <code v-if="job.message" class="job-message">{{ job.message }}</code>
        </article>
      </div>
      <div v-else class="state-card">No gost runtime jobs yet.</div>
    </section>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
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
      return 'Pending'
    case 1:
      return 'Running'
    case 2:
      return 'Success'
    case 3:
      return 'Failed'
    default:
      return 'Unknown'
  }
}

function formatJobTime(job) {
  const raw = job?.completedAt || job?.updatedAt || job?.createdAt
  if (!raw) {
    return '-'
  }
  const time = new Date(raw)
  if (Number.isNaN(time.getTime())) {
    return String(raw)
  }
  return time.toLocaleString()
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
    validationError.value = 'NodeX base URL is required when NodeX mode is enabled'
    return
  }
  if (nodeXMode.value && !trimmedToken) {
    validationError.value = 'NodeX token is required when NodeX mode is enabled'
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
    validationError.value = error.response?.data?.msg || error.message || 'Failed to save NodeX config'
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
    statusError.value = error.response?.data?.msg || error.message || 'Failed to fetch NodeX runtime status'
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
    doctorOutput.value = error.response?.data?.msg || error.message || 'NodeX runtime doctor failed'
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
