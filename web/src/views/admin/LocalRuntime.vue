<template>
  <div class="local-runtime-page">
    <section class="hero-card">
      <div class="hero-copy">
        <p class="eyebrow">Stateless Runtime</p>
        <h2>Local Runtime / Ansible</h2>
        <p class="hero-text">
          This page owns the panel-host Ansible executor only. It is the stateless runtime path for panel-side
          forwarding and does not require a persistent NodeX control-plane or Node-Agent connection.
        </p>
        <p class="hero-text">
          The recommended backend is <code>nftables_ansible</code>. <code>iptables_ansible</code> remains available as a
          legacy compatibility path.
        </p>
      </div>
      <div class="hero-actions">
        <router-link class="btn btn-secondary" to="/admin/forward/ansible-machines">Ansible Machines</router-link>
        <router-link class="btn btn-secondary" to="/admin/forward/nodex">NodeX Runtime</router-link>
        <router-link class="btn btn-secondary" to="/admin/forward/agents">NodeX Agents</router-link>
        <button class="btn btn-secondary" :disabled="jobsLoading || statusLoading" @click="refreshAll">
          {{ jobsLoading || statusLoading ? 'Refreshing...' : 'Refresh' }}
        </button>
        <button class="btn btn-primary" :disabled="saving" @click="saveLocalConfig">
          {{ saving ? 'Saving...' : 'Save And Activate Local Runtime' }}
        </button>
      </div>
    </section>

    <section :class="['mode-banner', localModeActive ? 'banner-success' : 'banner-warning']">
      <strong>{{ localModeActive ? 'Local runtime is active' : 'Local runtime is configured as standby' }}</strong>
      <span>
        {{
          localModeActive
            ? `Forward jobs currently use ${localBackendLabel(selectedLocalBackend)}. SSH transport and privilege escalation are resolved from this Ansible runtime config.`
            : 'NodeX/gost remains active globally. You can still stage and validate the local Ansible runtime here before switching back.'
        }}
      </span>
    </section>

    <section class="panel-card">
      <div class="section-head">
        <div>
          <p class="eyebrow">Configuration</p>
          <h3>Panel-Host Ansible Executor</h3>
          <p class="section-copy">
            Ansible mode is stateless: the panel stores execution-node identity on tunnel and forward records, while
            inventory, playbooks, sudo and SSH behavior live here.
          </p>
        </div>
      </div>

      <div class="backend-grid">
        <button
          v-for="option in localBackendOptions"
          :key="option.value"
          type="button"
          :class="['backend-card', { active: selectedLocalBackend === option.value }]"
          @click="selectLocalBackend(option.value)"
        >
          <div class="backend-head">
            <strong>{{ option.label }}</strong>
            <span :class="['backend-chip', option.recommended ? 'backend-chip-primary' : 'backend-chip-muted']">
              {{ option.recommended ? 'Recommended' : 'Legacy' }}
            </span>
          </div>
          <p>{{ option.description }}</p>
          <code>{{ option.applyPlaybook }}</code>
        </button>
      </div>

      <div class="runtime-local-head">
        <div>
          <p class="eyebrow">Executor</p>
          <h4>{{ localBackendLabel(selectedLocalBackend) }}</h4>
          <p class="hint">
            Saving here writes <code>forward.runtime_backend={{ selectedLocalBackend }}</code>,
            <code>forward.runtime.ansible.backend={{ selectedLocalBackend }}</code> and
            <code>forward.runtime.nodex_mode=false</code>.
          </p>
        </div>
        <button class="btn btn-secondary btn-sm" :disabled="saving" @click="applyDefaultRuntimeAnsibleConfig">
          Use backend defaults
        </button>
      </div>

      <div class="form-grid ansible-form-grid">
        <div class="form-group">
          <label for="ansible-inventory">Inventory</label>
          <input id="ansible-inventory" v-model.trim="runtimeAnsibleForm.inventory" type="text" placeholder="config/deploy/ansible/inventory.ini" />
        </div>
        <div class="form-group">
          <label for="ansible-apply-playbook">Apply playbook</label>
          <input id="ansible-apply-playbook" v-model.trim="runtimeAnsibleForm.playbookApply" type="text" :placeholder="defaultRuntimeAnsibleConfig.playbookApply" />
        </div>
        <div class="form-group">
          <label for="ansible-remove-playbook">Remove playbook</label>
          <input id="ansible-remove-playbook" v-model.trim="runtimeAnsibleForm.playbookRemove" type="text" :placeholder="defaultRuntimeAnsibleConfig.playbookRemove" />
        </div>
        <div class="form-group">
          <label for="ansible-command">Command</label>
          <input id="ansible-command" v-model.trim="runtimeAnsibleForm.command" type="text" placeholder="ansible-playbook" />
        </div>
        <div class="form-group">
          <label for="ansible-working-dir">Working dir</label>
          <input id="ansible-working-dir" v-model.trim="runtimeAnsibleForm.workingDir" type="text" placeholder="config/deploy/ansible" />
        </div>
        <div class="form-group">
          <label for="ansible-target-pattern">Target pattern</label>
          <input id="ansible-target-pattern" v-model.trim="runtimeAnsibleForm.targetPattern" type="text" placeholder="{{node.host}}" />
        </div>
        <div class="form-group">
          <label for="ansible-timeout-seconds">Timeout (seconds)</label>
          <input id="ansible-timeout-seconds" v-model.number="runtimeAnsibleForm.timeoutSeconds" type="number" min="1" placeholder="120" />
        </div>
        <div class="form-group">
          <label for="ansible-config-path">ANSIBLE_CONFIG</label>
          <input id="ansible-config-path" v-model.trim="runtimeAnsibleForm.ansibleConfig" type="text" placeholder="config/deploy/ansible/ansible.cfg" />
        </div>
      </div>

      <div class="form-group checkbox-group">
        <label class="checkbox-label">
          <input v-model="runtimeAnsibleForm.become" type="checkbox" />
          <span>Use sudo / become on the execution node</span>
        </label>
      </div>

      <div class="form-grid ansible-form-grid ansible-json-grid">
        <div class="form-group">
          <label for="ansible-extra-vars-json">Extra vars JSON</label>
          <textarea id="ansible-extra-vars-json" v-model="runtimeAnsibleForm.extraVarsJson" rows="6" placeholder='{"change_window":"maintenance"}'></textarea>
          <p class="hint">Backend-specific fields such as firewall driver are injected automatically by the backend.</p>
        </div>
        <div class="form-group">
          <label for="ansible-environment-json">Environment JSON</label>
          <textarea id="ansible-environment-json" v-model="runtimeAnsibleForm.environmentJson" rows="6" placeholder='{"ANSIBLE_HOST_KEY_CHECKING":"False"}'></textarea>
          <p class="hint">Extra process environment variables for the panel-host executor.</p>
        </div>
      </div>

      <div class="form-group">
        <label>Generated runtime JSON</label>
        <textarea :value="runtimeConfigPreview" rows="8" class="runtime-config-preview" readonly></textarea>
        <p class="hint">The JSON payload is generated from the structured fields above and stored in <code>forward.runtime.ansible.config</code>.</p>
      </div>

      <p v-if="validationError" class="form-error">{{ validationError }}</p>
    </section>

    <section class="panel-card">
      <div class="section-head">
        <div>
          <p class="eyebrow">Local Probe</p>
          <h3>Executor Reachability And Runtime Readiness</h3>
        </div>
        <div class="section-actions">
          <button class="btn btn-secondary btn-sm" :disabled="statusLoading" @click="fetchLocalStatus">
            {{ statusLoading ? 'Loading...' : 'Refresh status' }}
          </button>
          <button class="btn btn-secondary btn-sm" :disabled="doctorRunning" @click="runLocalDoctor">
            {{ doctorRunning ? 'Running...' : 'Run doctor' }}
          </button>
        </div>
      </div>

      <div v-if="statusLoading" class="state-card">Loading local runtime status...</div>
      <div v-else-if="statusError" class="state-card state-error">{{ statusError }}</div>
      <div v-else-if="statusSummary" class="status-grid">
        <article class="status-card">
          <p class="metric-label">Panel Config</p>
          <p class="metric-value">{{ localModeActive ? 'Local runtime active' : 'Standby config' }}</p>
          <p class="metric-detail">Backend: {{ localBackendLabel(actualBackend) }}</p>
          <p class="metric-detail">Preferred local backend: {{ localBackendLabel(selectedLocalBackend) }}</p>
          <p class="metric-detail">Attachment: {{ statusSummary.attachment?.model || '-' }}</p>
        </article>
        <article class="status-card">
          <p class="metric-label">Reachability</p>
          <p class="metric-value">{{ statusSummary.reachability?.ready ? 'Reachable' : 'Not ready' }}</p>
          <p class="metric-detail">{{ statusSummary.reachability?.reason || '-' }}</p>
          <p class="metric-detail">Runtime ready: {{ statusSummary.runtimeReady?.ready ? 'Yes' : 'No' }}</p>
          <p class="metric-detail">{{ statusSummary.runtimeReady?.reason || '-' }}</p>
        </article>
        <article class="status-card">
          <p class="metric-label">Executor</p>
          <p class="metric-value">{{ statusSummary.localAnsible?.command || runtimeAnsibleForm.command || 'ansible-playbook' }}</p>
          <p class="metric-detail">Firewall driver: {{ statusSummary.localAnsible?.firewallDriver || firewallDriverLabel(selectedLocalBackend) }}</p>
          <p class="metric-detail">Command found: {{ statusSummary.localAnsible?.commandFound ? 'Yes' : 'No' }}</p>
          <p class="metric-detail">Become: {{ statusSummary.localAnsible?.become ? 'Yes' : 'No' }}</p>
        </article>
        <article class="status-card">
          <p class="metric-label">Files</p>
          <p class="metric-detail">Inventory: {{ statusSummary.localAnsible?.inventoryExists ? 'Present' : 'Missing' }}</p>
          <p class="metric-detail">Apply playbook: {{ statusSummary.localAnsible?.applyPlaybookExists ? 'Present' : 'Missing' }}</p>
          <p class="metric-detail">Remove playbook: {{ statusSummary.localAnsible?.removePlaybookExists ? 'Present' : 'Missing' }}</p>
          <p class="metric-detail">Working dir: {{ statusSummary.localAnsible?.workingDirExists ? 'Present' : 'Missing' }}</p>
        </article>
      </div>
      <div v-else class="state-card">No local runtime status loaded yet.</div>

      <div v-if="statusSummary?.summary" class="summary-card">{{ statusSummary.summary }}</div>
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
          <h3>Latest {{ selectedLocalBackend }} Jobs</h3>
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
      <div v-else class="state-card">No local runtime jobs yet.</div>
    </section>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { getLocalRuntimeStatus, getSystemConfig, listForwardRuntimeJobs, runLocalRuntimeDoctor, setSystemConfig } from '@/api/admin'

const runtimeNodeXModeKey = 'forward.runtime.nodex_mode'
const runtimeBackendKey = 'forward.runtime_backend'
const runtimeAnsibleBackendKey = 'forward.runtime.ansible.backend'
const runtimeAnsibleConfigKey = 'forward.runtime.ansible.config'
const runtimeLegacyAnsibleConfigKey = 'forward.runtime.iptables_ansible.config'
const runtimeAnsibleInventoryKey = 'forward.runtime.ansible.inventory'
const runtimeAnsibleApplyPlaybookKey = 'forward.runtime.ansible.apply_playbook'
const runtimeAnsibleRemovePlaybookKey = 'forward.runtime.ansible.remove_playbook'
const runtimeAnsibleBecomeKey = 'forward.runtime.ansible.become'
const runtimeAnsibleExtraVarsKey = 'forward.runtime.ansible.extra_vars_json'
const runtimeLegacyAnsibleInventoryKey = 'forward.ansible.inventory'
const runtimeLegacyAnsibleApplyPlaybookKey = 'forward.ansible.playbook_apply'
const runtimeLegacyAnsibleRemovePlaybookKey = 'forward.ansible.playbook_remove'
const runtimeLegacyAnsibleBecomeKey = 'forward.ansible.become'
const runtimeLegacyAnsibleExtraVarsKey = 'forward.ansible.extra_vars_json'

const localBackendOptions = Object.freeze([
  {
    value: 'nftables_ansible',
    label: 'nftables / Ansible',
    description: 'Modern Linux hosts should prefer nftables.',
    applyPlaybook: 'config/deploy/ansible/playbooks/forward_apply_nftables.yml',
    removePlaybook: 'config/deploy/ansible/playbooks/forward_remove_nftables.yml',
    recommended: true
  },
  {
    value: 'iptables_ansible',
    label: 'iptables / Ansible',
    description: 'Legacy compatibility for existing playbooks.',
    applyPlaybook: 'config/deploy/ansible/playbooks/forward_apply.yml',
    removePlaybook: 'config/deploy/ansible/playbooks/forward_remove.yml',
    recommended: false
  }
])

function normalizeLocalBackend(value) {
  const normalized = String(value ?? '').trim().toLowerCase()
  return localBackendOptions.some(option => option.value === normalized) ? normalized : 'nftables_ansible'
}

function isLocalBackend(value) {
  return localBackendOptions.some(option => option.value === String(value ?? '').trim().toLowerCase())
}

function getLocalBackendMeta(value) {
  return localBackendOptions.find(option => option.value === normalizeLocalBackend(value)) || localBackendOptions[0]
}

function firewallDriverLabel(value) {
  return normalizeLocalBackend(value) === 'iptables_ansible' ? 'iptables' : 'nftables'
}

function localBackendLabel(value) {
  return getLocalBackendMeta(value).label
}

function defaultRuntimeAnsibleConfigForBackend(backend) {
  const meta = getLocalBackendMeta(backend)
  return {
    inventory: 'config/deploy/ansible/inventory.ini',
    playbookApply: meta.applyPlaybook,
    playbookRemove: meta.removePlaybook,
    command: 'ansible-playbook',
    workingDir: 'config/deploy/ansible',
    targetPattern: '{{node.host}}',
    timeoutSeconds: 120,
    ansibleConfig: 'config/deploy/ansible/ansible.cfg',
    become: false
  }
}

const selectedLocalBackend = ref('nftables_ansible')
const defaultRuntimeAnsibleConfig = computed(() => defaultRuntimeAnsibleConfigForBackend(selectedLocalBackend.value))
const actualBackend = ref('nftables_ansible')
const localModeActive = ref(false)
const runtimeAnsibleForm = ref(createRuntimeAnsibleForm())
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

function parseRuntimeBoolean(value) {
  if (typeof value === 'boolean') return value
  if (typeof value === 'number') return value !== 0
  const normalized = String(value ?? '').trim().toLowerCase()
  if (!normalized) return null
  if (['1', 'true', 'yes', 'on'].includes(normalized)) return true
  if (['0', 'false', 'no', 'off'].includes(normalized)) return false
  return null
}

function normalizeJsonObjectText(value) {
  const trimmed = String(value ?? '').trim()
  if (!trimmed) return ''
  try {
    const parsed = JSON.parse(trimmed)
    if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') return trimmed
    return JSON.stringify(parsed, null, 2)
  } catch {
    return trimmed
  }
}

function createRuntimeAnsibleForm(source = {}, backend = selectedLocalBackend.value) {
  const defaults = defaultRuntimeAnsibleConfigForBackend(backend)
  const extraVars = source.extraVars && typeof source.extraVars === 'object' && !Array.isArray(source.extraVars) ? source.extraVars : {}
  const rawEnvironment = source.environment && typeof source.environment === 'object' && !Array.isArray(source.environment) ? source.environment : {}
  const environment = { ...rawEnvironment }
  const ansibleConfig = String(source.ansibleConfig ?? environment.ANSIBLE_CONFIG ?? defaults.ansibleConfig).trim() || defaults.ansibleConfig
  delete environment.ANSIBLE_CONFIG
  return {
    inventory: String(source.inventory ?? defaults.inventory).trim() || defaults.inventory,
    playbookApply: String(source.playbookApply ?? source.applyPlaybook ?? defaults.playbookApply).trim() || defaults.playbookApply,
    playbookRemove: String(source.playbookRemove ?? source.removePlaybook ?? defaults.playbookRemove).trim() || defaults.playbookRemove,
    command: String(source.command ?? defaults.command).trim() || defaults.command,
    workingDir: String(source.workingDir ?? defaults.workingDir).trim() || defaults.workingDir,
    targetPattern: String(source.targetPattern ?? defaults.targetPattern).trim() || defaults.targetPattern,
    timeoutSeconds: Number.isFinite(Number(source.timeoutSeconds)) && Number(source.timeoutSeconds) > 0 ? Number(source.timeoutSeconds) : defaults.timeoutSeconds,
    ansibleConfig,
    become: parseRuntimeBoolean(source.become) ?? defaults.become,
    extraVarsJson: Object.keys(extraVars).length ? JSON.stringify(extraVars, null, 2) : '',
    environmentJson: Object.keys(environment).length ? JSON.stringify(environment, null, 2) : ''
  }
}

function normalizeRuntimeAnsibleForm(source = {}) {
  const form = createRuntimeAnsibleForm(source, selectedLocalBackend.value)
  return {
    ...form,
    extraVarsJson: normalizeJsonObjectText(source.extraVarsJson ?? form.extraVarsJson),
    environmentJson: normalizeJsonObjectText(source.environmentJson ?? form.environmentJson)
  }
}

function parseRuntimeJsonObject(value, label) {
  const trimmed = String(value ?? '').trim()
  if (!trimmed) return {}
  let parsed
  try {
    parsed = JSON.parse(trimmed)
  } catch {
    throw new Error(`${label} must be valid JSON`)
  }
  if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') {
    throw new Error(`${label} must be a JSON object`)
  }
  return parsed
}

function buildRuntimeAnsiblePayload(source = runtimeAnsibleForm.value) {
  const form = normalizeRuntimeAnsibleForm(source)
  const extraVars = parseRuntimeJsonObject(form.extraVarsJson, 'Extra vars JSON')
  const environmentOverrides = parseRuntimeJsonObject(form.environmentJson, 'Environment JSON')
  const environment = {}
  if (form.ansibleConfig) environment.ANSIBLE_CONFIG = form.ansibleConfig
  Object.entries(environmentOverrides).forEach(([key, value]) => {
    const normalizedKey = String(key ?? '').trim()
    if (!normalizedKey) return
    environment[normalizedKey] = value == null ? '' : String(value)
  })
  const normalizedExtraVars = {}
  Object.entries(extraVars).forEach(([key, value]) => {
    const normalizedKey = String(key ?? '').trim()
    if (!normalizedKey) return
    normalizedExtraVars[normalizedKey] = value
  })
  return {
    inventory: form.inventory,
    playbookApply: form.playbookApply,
    playbookRemove: form.playbookRemove,
    become: !!form.become,
    extraVars: normalizedExtraVars,
    command: form.command,
    workingDir: form.workingDir,
    targetPattern: form.targetPattern,
    environment,
    timeoutSeconds: Number.isFinite(Number(form.timeoutSeconds)) && Number(form.timeoutSeconds) > 0 ? Number(form.timeoutSeconds) : defaultRuntimeAnsibleConfig.value.timeoutSeconds
  }
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
    case 0: return 'Pending'
    case 1: return 'Running'
    case 2: return 'Success'
    case 3: return 'Failed'
    default: return 'Unknown'
  }
}

function formatJobTime(job) {
  const raw = job?.completedAt || job?.updatedAt || job?.createdAt
  if (!raw) return '-'
  const time = new Date(raw)
  if (Number.isNaN(time.getTime())) return String(raw)
  return time.toLocaleString()
}

function safeParseObject(value) {
  const trimmed = String(value ?? '').trim()
  if (!trimmed) return {}
  try {
    const parsed = JSON.parse(trimmed)
    return parsed && !Array.isArray(parsed) && typeof parsed === 'object' ? parsed : {}
  } catch {
    return {}
  }
}

const fallbackCommands = computed(() => {
  const driver = firewallDriverLabel(selectedLocalBackend.value)
  const base = {
    powerShell: ['Get-Command ansible-playbook', 'ansible-playbook --version', 'Get-Content .\\config\\deploy\\ansible\\inventory.ini'],
    bash: ['command -v ansible-playbook', 'ansible-playbook --version', 'cat ./config/deploy/ansible/inventory.ini'],
    upgrade: ['git pull --ff-only', 'go test ./internal/service/... -run ForwardRuntime'],
    references: ['docs/guide/forward-relay-onboarding.md', 'docs/guide/forward-tunnel-runtime-ops.md', 'docs/reference/runtime.md']
  }
  if (driver === 'nftables') {
    base.powerShell.push('wsl nft --version')
    base.bash.push('nft --version', 'nft list tables')
  } else {
    base.powerShell.push('Get-Command iptables')
    base.bash.push('iptables --version', 'iptables -t nat -S')
  }
  return base
})

const displayedCommands = computed(() => {
  const commands = doctorSummary.value?.commands
  if (!commands) return fallbackCommands.value
  return {
    powerShell: Array.isArray(commands.powerShell) ? commands.powerShell : [],
    bash: Array.isArray(commands.bash) ? commands.bash : [],
    upgrade: Array.isArray(commands.upgrade) ? commands.upgrade : [],
    references: Array.isArray(commands.references) ? commands.references : []
  }
})

const runtimeConfigPreview = computed(() => {
  try {
    return JSON.stringify(buildRuntimeAnsiblePayload(runtimeAnsibleForm.value), null, 2)
  } catch (error) {
    return `Invalid runtime config: ${error.message}`
  }
})

function selectLocalBackend(nextBackend) {
  const normalized = normalizeLocalBackend(nextBackend)
  if (normalized === selectedLocalBackend.value) return
  const previousDefaults = defaultRuntimeAnsibleConfigForBackend(selectedLocalBackend.value)
  const nextDefaults = defaultRuntimeAnsibleConfigForBackend(normalized)
  const nextForm = { ...runtimeAnsibleForm.value }
  if (!nextForm.playbookApply || nextForm.playbookApply === previousDefaults.playbookApply) {
    nextForm.playbookApply = nextDefaults.playbookApply
  }
  if (!nextForm.playbookRemove || nextForm.playbookRemove === previousDefaults.playbookRemove) {
    nextForm.playbookRemove = nextDefaults.playbookRemove
  }
  selectedLocalBackend.value = normalized
  runtimeAnsibleForm.value = normalizeRuntimeAnsibleForm(nextForm)
  fetchJobs()
}

function applyDefaultRuntimeAnsibleConfig() {
  validationError.value = ''
  runtimeAnsibleForm.value = createRuntimeAnsibleForm({}, selectedLocalBackend.value)
}

async function getConfigValue(key) {
  try {
    const res = await getSystemConfig(key)
    return res?.data?.value
  } catch {
    return ''
  }
}

async function fetchLocalConfig() {
  validationError.value = ''
  const values = await Promise.all([
    getConfigValue(runtimeNodeXModeKey),
    getConfigValue(runtimeBackendKey),
    getConfigValue(runtimeAnsibleBackendKey),
    getConfigValue(runtimeAnsibleConfigKey),
    getConfigValue(runtimeLegacyAnsibleConfigKey),
    getConfigValue(runtimeAnsibleInventoryKey),
    getConfigValue(runtimeLegacyAnsibleInventoryKey),
    getConfigValue(runtimeAnsibleApplyPlaybookKey),
    getConfigValue(runtimeLegacyAnsibleApplyPlaybookKey),
    getConfigValue(runtimeAnsibleRemovePlaybookKey),
    getConfigValue(runtimeLegacyAnsibleRemovePlaybookKey),
    getConfigValue(runtimeAnsibleBecomeKey),
    getConfigValue(runtimeLegacyAnsibleBecomeKey),
    getConfigValue(runtimeAnsibleExtraVarsKey),
    getConfigValue(runtimeLegacyAnsibleExtraVarsKey)
  ])

  const explicitNodeXMode = parseRuntimeBoolean(values[0])
  const resolvedRuntimeBackend = String(values[1] || '').trim().toLowerCase()
  const resolvedLocalBackend = normalizeLocalBackend(
    values[2] || (isLocalBackend(resolvedRuntimeBackend) ? resolvedRuntimeBackend : 'nftables_ansible')
  )

  selectedLocalBackend.value = resolvedLocalBackend
  actualBackend.value = isLocalBackend(resolvedRuntimeBackend) ? resolvedRuntimeBackend : resolvedLocalBackend
  localModeActive.value = explicitNodeXMode === null ? isLocalBackend(resolvedRuntimeBackend) : !explicitNodeXMode

  let parsedPayload = null
  const rawConfig = String(values[3] || values[4] || '').trim()
  if (rawConfig) {
    try {
      const parsed = JSON.parse(rawConfig)
      if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') throw new Error('invalid')
      parsedPayload = parsed
    } catch {
      validationError.value = 'Saved local runtime config is invalid. Defaults were loaded; save again to repair it.'
    }
  }

  const fallbackPayload = {
    inventory: values[5] || values[6] || '',
    playbookApply: values[7] || values[8] || '',
    playbookRemove: values[9] || values[10] || '',
    become: parseRuntimeBoolean(values[11]) ?? parseRuntimeBoolean(values[12]) ?? false,
    extraVars: safeParseObject(values[13] || values[14])
  }

  runtimeAnsibleForm.value = normalizeRuntimeAnsibleForm(parsedPayload || fallbackPayload)
}

async function saveLocalConfig() {
  validationError.value = ''
  let ansiblePayload = null
  try {
    runtimeAnsibleForm.value = normalizeRuntimeAnsibleForm(runtimeAnsibleForm.value)
    ansiblePayload = buildRuntimeAnsiblePayload(runtimeAnsibleForm.value)
  } catch (error) {
    validationError.value = error.message || 'Local runtime JSON is invalid'
    return
  }

  saving.value = true
  try {
    await Promise.all([
      setSystemConfig(runtimeNodeXModeKey, { value: false, type: 'bool', group: 'forward', description: 'Enable NodeX forward runtime mode' }),
      setSystemConfig(runtimeBackendKey, { value: selectedLocalBackend.value, type: 'string', group: 'forward', description: 'Forward runtime backend' }),
      setSystemConfig(runtimeAnsibleBackendKey, { value: selectedLocalBackend.value, type: 'string', group: 'forward', description: 'Preferred local ansible backend' }),
      setSystemConfig(runtimeAnsibleConfigKey, { value: JSON.stringify(ansiblePayload), type: 'json', group: 'forward', description: 'Forward runtime ansible config' }),
      setSystemConfig(runtimeAnsibleInventoryKey, { value: ansiblePayload.inventory || '', type: 'string', group: 'forward', description: 'Forward ansible inventory path' }),
      setSystemConfig(runtimeAnsibleApplyPlaybookKey, { value: ansiblePayload.playbookApply || '', type: 'string', group: 'forward', description: 'Forward ansible apply playbook path' }),
      setSystemConfig(runtimeAnsibleRemovePlaybookKey, { value: ansiblePayload.playbookRemove || '', type: 'string', group: 'forward', description: 'Forward ansible remove playbook path' }),
      setSystemConfig(runtimeAnsibleBecomeKey, { value: ansiblePayload.become || false, type: 'bool', group: 'forward', description: 'Forward ansible become flag' }),
      setSystemConfig(runtimeAnsibleExtraVarsKey, { value: JSON.stringify(ansiblePayload.extraVars || {}), type: 'json', group: 'forward', description: 'Forward ansible extra vars JSON' })
    ])
    actualBackend.value = selectedLocalBackend.value
    localModeActive.value = true
    await refreshAll()
  } catch (error) {
    validationError.value = error.response?.data?.msg || error.message || 'Failed to save local runtime config'
  } finally {
    saving.value = false
  }
}

async function fetchJobs() {
  jobsLoading.value = true
  try {
    const res = await listForwardRuntimeJobs({ backend: selectedLocalBackend.value, limit: 10 })
    const payload = extractPayload(res)
    const list = Array.isArray(payload?.list) ? payload.list : Array.isArray(payload) ? payload : []
    jobs.value = list.map(normalizeRuntimeJob)
  } catch (error) {
    console.error('get local runtime jobs failed:', error)
    jobs.value = []
  } finally {
    jobsLoading.value = false
  }
}

async function fetchLocalStatus() {
  statusLoading.value = true
  statusError.value = ''
  try {
    const res = await getLocalRuntimeStatus()
    statusSummary.value = extractPayload(res) || null
  } catch (error) {
    statusError.value = error.response?.data?.msg || error.message || 'Failed to fetch local runtime status'
    statusSummary.value = null
  } finally {
    statusLoading.value = false
  }
}

async function runLocalDoctor() {
  doctorRunning.value = true
  doctorOutput.value = ''
  try {
    const res = await runLocalRuntimeDoctor()
    const payload = extractPayload(res) || null
    doctorSummary.value = payload
    statusSummary.value = payload
    doctorOutput.value = JSON.stringify(payload, null, 2)
  } catch (error) {
    doctorOutput.value = error.response?.data?.msg || error.message || 'Local runtime doctor failed'
  } finally {
    doctorRunning.value = false
  }
}

async function refreshAll() {
  await fetchLocalConfig()
  await Promise.all([fetchLocalStatus(), fetchJobs()])
}

onMounted(async () => {
  await refreshAll()
})
</script>

<style scoped>
.local-runtime-page {
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
    radial-gradient(circle at top right, rgba(16, 185, 129, 0.14), transparent 28%),
    linear-gradient(180deg, rgba(15, 23, 42, 0.03), transparent 70%),
    var(--surface-color);
}

.hero-copy {
  max-width: 760px;
}

.hero-copy h2 {
  margin: 6px 0 12px;
  font-size: 30px;
}

.hero-text,
.section-copy,
.hint,
.metric-detail,
.job-meta,
.job-time {
  color: var(--text-secondary);
  line-height: 1.6;
}

.hero-actions,
.section-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

.hero-actions {
  align-items: flex-start;
  justify-content: flex-end;
  min-width: 320px;
}

.btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  border: 1px solid transparent;
  border-radius: 12px;
  padding: 10px 16px;
  cursor: pointer;
  text-decoration: none;
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
  flex-wrap: wrap;
  gap: 12px;
  align-items: center;
  padding: 16px 18px;
}

.banner-success {
  border-color: rgba(16, 185, 129, 0.35);
  background: rgba(16, 185, 129, 0.08);
}

.banner-warning {
  border-color: rgba(245, 158, 11, 0.35);
  background: rgba(245, 158, 11, 0.08);
}

.panel-card {
  padding: 22px;
}

.section-head,
.runtime-local-head,
.job-main {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  align-items: flex-start;
}

.section-head,
.runtime-local-head {
  margin-bottom: 18px;
}

.section-head h3,
.runtime-local-head h4 {
  margin: 6px 0 0;
}

.eyebrow,
.metric-label {
  margin: 0;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  font-size: 12px;
  color: var(--text-secondary);
}

.backend-grid,
.form-grid,
.status-grid {
  display: grid;
  gap: 16px;
}

.backend-grid {
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
  margin-bottom: 18px;
}

.backend-card {
  display: flex;
  flex-direction: column;
  gap: 10px;
  border: 1px solid var(--border-color);
  border-radius: 18px;
  background: var(--bg-color);
  padding: 18px;
  text-align: left;
  cursor: pointer;
}

.backend-card.active {
  border-color: var(--primary-color);
  box-shadow: 0 12px 26px rgba(59, 130, 246, 0.12);
}

.backend-card p,
.backend-card code {
  margin: 0;
  color: var(--text-secondary);
}

.backend-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.backend-chip {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 24px;
  padding: 4px 10px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 700;
}

.backend-chip-primary {
  background: rgba(59, 130, 246, 0.12);
  color: #1d4ed8;
}

.backend-chip-muted {
  background: rgba(148, 163, 184, 0.16);
  color: #475569;
}

.ansible-form-grid {
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
}

.form-group {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.form-group input,
.form-group textarea {
  width: 100%;
  border: 1px solid var(--border-color);
  border-radius: 12px;
  background: var(--bg-color);
  color: var(--text-color);
  padding: 12px 14px;
}

.form-group textarea {
  resize: vertical;
}

.checkbox-group {
  margin: 18px 0 10px;
}

.checkbox-label {
  display: inline-flex;
  align-items: center;
  gap: 10px;
}

.form-error,
.state-error {
  color: #dc2626;
}

.state-card,
.summary-card,
.doctor-output,
.command-block,
.warning-list,
.status-card,
.job-item {
  border: 1px solid var(--border-color);
  border-radius: 16px;
  background: var(--bg-color);
}

.state-card,
.summary-card,
.doctor-output,
.command-block,
.warning-list,
.status-card {
  padding: 16px;
}

.status-grid {
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
}

.metric-value {
  margin: 8px 0;
  font-size: 24px;
  font-weight: 700;
}

.warning-list,
.command-block,
.doctor-output {
  margin-top: 16px;
}

.warning-list code,
.command-block code,
.job-message {
  display: block;
  margin-top: 8px;
  border-radius: 10px;
  padding: 10px 12px;
  background: rgba(15, 23, 42, 0.06);
  white-space: pre-wrap;
  word-break: break-word;
}

.doctor-output pre {
  margin: 8px 0 0;
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
}

.job-side {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 6px;
}

.status-chip {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 72px;
  border-radius: 999px;
  padding: 4px 10px;
  font-size: 12px;
  font-weight: 700;
}

.status-0 {
  background: rgba(148, 163, 184, 0.18);
  color: #475569;
}

.status-1 {
  background: rgba(59, 130, 246, 0.18);
  color: #1d4ed8;
}

.status-2 {
  background: rgba(16, 185, 129, 0.18);
  color: #047857;
}

.status-3 {
  background: rgba(239, 68, 68, 0.18);
  color: #b91c1c;
}

@media (max-width: 900px) {
  .hero-card,
  .section-head,
  .runtime-local-head,
  .job-main {
    flex-direction: column;
  }

  .hero-actions,
  .job-side {
    justify-content: flex-start;
    align-items: stretch;
  }
}
</style>
