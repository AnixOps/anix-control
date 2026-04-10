<template>
  <div class="local-runtime-page">
    <section class="hero-card">
      <div class="hero-copy">
        <p class="eyebrow">{{ t('runtime.localRuntime.heroEyebrow') }}</p>
        <h2>{{ t('runtime.localRuntime.title') }}</h2>
        <p class="hero-text">
          {{ t('runtime.localRuntime.heroTextPrimary') }}
        </p>
        <p class="hero-text">
          {{ t('runtime.localRuntime.heroTextSecondary') }}
        </p>
      </div>
      <div class="hero-actions">
        <router-link class="btn btn-secondary" to="/admin/forward/ansible-machines">{{ t('forwardSuite.nav.ansibleMachines') }}</router-link>
        <router-link class="btn btn-secondary" to="/admin/forward/nodex">{{ t('forwardSuite.nav.nodeXRuntime') }}</router-link>
        <router-link class="btn btn-secondary" to="/admin/forward/agents">{{ t('forwardSuite.nav.nodeXAgents') }}</router-link>
        <button class="btn btn-secondary" :disabled="jobsLoading || statusLoading" @click="refreshAll">
          {{ jobsLoading || statusLoading ? t('runtime.localRuntime.refreshLoading') : t('common.actions.refresh') }}
        </button>
        <button class="btn btn-primary" :disabled="saving" @click="saveLocalConfig">
          {{ saving ? t('runtime.localRuntime.saveLoading') : t('runtime.localRuntime.saveActivate') }}
        </button>
      </div>
    </section>

    <section :class="['mode-banner', localModeActive ? 'banner-success' : 'banner-warning']">
      <strong>{{ localModeActive ? t('runtime.localRuntime.activeBannerTitle') : t('runtime.localRuntime.standbyBannerTitle') }}</strong>
      <span>
        {{
          localModeActive
            ? t('runtime.localRuntime.activeBannerText', { backend: localBackendLabel(selectedLocalBackend) })
            : t('runtime.localRuntime.standbyBannerText')
        }}
      </span>
    </section>

    <section class="panel-card">
      <div class="section-head">
        <div>
          <p class="eyebrow">{{ t('runtime.localRuntime.configEyebrow') }}</p>
          <h3>{{ t('runtime.localRuntime.configTitle') }}</h3>
          <p class="section-copy">
            {{ t('runtime.localRuntime.configCopy') }}
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
              {{ option.recommended ? t('runtime.localRuntime.recommended') : t('runtime.localRuntime.legacy') }}
            </span>
          </div>
          <p>{{ option.description }}</p>
          <code>{{ option.applyPlaybook }}</code>
        </button>
      </div>

      <div class="runtime-local-head">
        <div>
          <p class="eyebrow">{{ t('runtime.localRuntime.executorEyebrow') }}</p>
          <h4>{{ localBackendLabel(selectedLocalBackend) }}</h4>
          <p class="hint">
            {{ t('runtime.localRuntime.executorHint', { backend: localBackendLabel(selectedLocalBackend), backendKey: selectedLocalBackend }) }}
          </p>
        </div>
        <button class="btn btn-secondary btn-sm" :disabled="saving" @click="applyDefaultRuntimeAnsibleConfig">
          {{ t('runtime.localRuntime.defaultsAction') }}
        </button>
      </div>

      <div class="form-grid ansible-form-grid">
        <div class="form-group">
          <label for="ansible-inventory">{{ t('runtime.localRuntime.fields.inventory') }}</label>
          <input id="ansible-inventory" v-model.trim="runtimeAnsibleForm.inventory" type="text" placeholder="config/deploy/ansible/inventory.ini" />
        </div>
        <div class="form-group">
          <label for="ansible-apply-playbook">{{ t('runtime.localRuntime.fields.applyPlaybook') }}</label>
          <input id="ansible-apply-playbook" v-model.trim="runtimeAnsibleForm.playbookApply" type="text" :placeholder="defaultRuntimeAnsibleConfig.playbookApply" />
        </div>
        <div class="form-group">
          <label for="ansible-remove-playbook">{{ t('runtime.localRuntime.fields.removePlaybook') }}</label>
          <input id="ansible-remove-playbook" v-model.trim="runtimeAnsibleForm.playbookRemove" type="text" :placeholder="defaultRuntimeAnsibleConfig.playbookRemove" />
        </div>
        <div class="form-group">
          <label for="ansible-command">{{ t('runtime.localRuntime.fields.command') }}</label>
          <input id="ansible-command" v-model.trim="runtimeAnsibleForm.command" type="text" placeholder="ansible-playbook" />
        </div>
        <div class="form-group">
          <label for="ansible-working-dir">{{ t('runtime.localRuntime.fields.workingDir') }}</label>
          <input id="ansible-working-dir" v-model.trim="runtimeAnsibleForm.workingDir" type="text" placeholder="config/deploy/ansible" />
        </div>
        <div class="form-group">
          <label for="ansible-target-pattern">{{ t('runtime.localRuntime.fields.targetPattern') }}</label>
          <input id="ansible-target-pattern" v-model.trim="runtimeAnsibleForm.targetPattern" type="text" placeholder="{{node.host}}" />
        </div>
        <div class="form-group">
          <label for="ansible-timeout-seconds">{{ t('runtime.localRuntime.fields.timeoutSeconds') }}</label>
          <input id="ansible-timeout-seconds" v-model.number="runtimeAnsibleForm.timeoutSeconds" type="number" min="1" placeholder="120" />
        </div>
        <div class="form-group">
          <label for="ansible-config-path">{{ t('runtime.localRuntime.fields.ansibleConfig') }}</label>
          <input id="ansible-config-path" v-model.trim="runtimeAnsibleForm.ansibleConfig" type="text" placeholder="config/deploy/ansible/ansible.cfg" />
        </div>
      </div>

      <div class="form-group checkbox-group">
        <label class="checkbox-label">
          <input v-model="runtimeAnsibleForm.become" type="checkbox" />
          <span>{{ t('runtime.localRuntime.fields.useBecome') }}</span>
        </label>
      </div>

      <div class="form-grid ansible-form-grid ansible-json-grid">
        <div class="form-group">
          <label for="ansible-extra-vars-json">{{ t('runtime.localRuntime.fields.extraVarsJson') }}</label>
          <textarea id="ansible-extra-vars-json" v-model="runtimeAnsibleForm.extraVarsJson" rows="6" placeholder='{"change_window":"maintenance"}'></textarea>
          <p class="hint">{{ t('runtime.localRuntime.extraVarsHint') }}</p>
        </div>
        <div class="form-group">
          <label for="ansible-environment-json">{{ t('runtime.localRuntime.fields.environmentJson') }}</label>
          <textarea id="ansible-environment-json" v-model="runtimeAnsibleForm.environmentJson" rows="6" placeholder='{"ANSIBLE_HOST_KEY_CHECKING":"False"}'></textarea>
          <p class="hint">{{ t('runtime.localRuntime.environmentHint') }}</p>
        </div>
      </div>

      <div class="form-group">
        <label>{{ t('runtime.localRuntime.fields.generatedJson') }}</label>
        <textarea :value="runtimeConfigPreview" rows="8" class="runtime-config-preview" readonly></textarea>
        <p class="hint">{{ t('runtime.localRuntime.generatedHint') }}</p>
      </div>

      <p v-if="validationError" class="form-error">{{ validationError }}</p>
    </section>

    <section class="panel-card">
      <div class="section-head">
        <div>
          <p class="eyebrow">{{ t('runtime.localRuntime.probeEyebrow') }}</p>
          <h3>{{ t('runtime.localRuntime.probeTitle') }}</h3>
        </div>
        <div class="section-actions">
          <button class="btn btn-secondary btn-sm" :disabled="statusLoading" @click="fetchLocalStatus">
            {{ statusLoading ? t('runtime.shared.loading') : t('runtime.shared.refreshStatus') }}
          </button>
          <button class="btn btn-secondary btn-sm" :disabled="doctorRunning" @click="runLocalDoctor">
            {{ doctorRunning ? t('runtime.shared.runningDoctor') : t('runtime.shared.runDoctor') }}
          </button>
        </div>
      </div>

      <div v-if="statusLoading" class="state-card">{{ t('runtime.localRuntime.loadingStatus') }}</div>
      <div v-else-if="statusError" class="state-card state-error">{{ statusError }}</div>
      <div v-else-if="statusSummary" class="status-grid">
        <article class="status-card">
          <p class="metric-label">{{ t('runtime.shared.panelConfig') }}</p>
          <p class="metric-value">{{ localModeActive ? t('runtime.localRuntime.cards.localActiveValue') : t('runtime.localRuntime.cards.standbyValue') }}</p>
          <p class="metric-detail">{{ t('runtime.localRuntime.cards.backend') }}: {{ localBackendLabel(actualBackend) }}</p>
          <p class="metric-detail">{{ t('runtime.localRuntime.cards.preferredLocalBackend') }}: {{ localBackendLabel(selectedLocalBackend) }}</p>
          <p class="metric-detail">{{ t('runtime.localRuntime.cards.attachment') }}: {{ statusSummary.attachment?.model || '-' }}</p>
        </article>
        <article class="status-card">
          <p class="metric-label">{{ t('runtime.shared.reachability') }}</p>
          <p class="metric-value">{{ statusSummary.reachability?.ready ? t('runtime.shared.reachable') : t('runtime.shared.notReady') }}</p>
          <p class="metric-detail">{{ translateRuntimeText(statusSummary.reachability?.reason) }}</p>
          <p class="metric-detail">{{ t('runtime.localRuntime.cards.runtimeReady') }}: {{ statusSummary.runtimeReady?.ready ? t('runtime.shared.yes') : t('runtime.shared.no') }}</p>
          <p class="metric-detail">{{ translateRuntimeText(statusSummary.runtimeReady?.reason) }}</p>
        </article>
        <article class="status-card">
          <p class="metric-label">{{ t('runtime.shared.executor') }}</p>
          <p class="metric-value">{{ statusSummary.localAnsible?.command || runtimeAnsibleForm.command || 'ansible-playbook' }}</p>
          <p class="metric-detail">{{ t('runtime.localRuntime.cards.firewallDriver') }}: {{ statusSummary.localAnsible?.firewallDriver || firewallDriverLabel(selectedLocalBackend) }}</p>
          <p class="metric-detail">{{ t('runtime.localRuntime.cards.commandFound') }}: {{ statusSummary.localAnsible?.commandFound ? t('runtime.shared.yes') : t('runtime.shared.no') }}</p>
          <p class="metric-detail">{{ t('runtime.localRuntime.cards.become') }}: {{ statusSummary.localAnsible?.become ? t('runtime.shared.yes') : t('runtime.shared.no') }}</p>
        </article>
        <article class="status-card">
          <p class="metric-label">{{ t('runtime.shared.files') }}</p>
          <p class="metric-detail">{{ t('runtime.localRuntime.cards.inventory') }}: {{ statusSummary.localAnsible?.inventoryExists ? t('runtime.shared.present') : t('runtime.shared.missing') }}</p>
          <p class="metric-detail">{{ t('runtime.localRuntime.cards.applyPlaybook') }}: {{ statusSummary.localAnsible?.applyPlaybookExists ? t('runtime.shared.present') : t('runtime.shared.missing') }}</p>
          <p class="metric-detail">{{ t('runtime.localRuntime.cards.removePlaybook') }}: {{ statusSummary.localAnsible?.removePlaybookExists ? t('runtime.shared.present') : t('runtime.shared.missing') }}</p>
          <p class="metric-detail">{{ t('runtime.localRuntime.cards.workingDir') }}: {{ statusSummary.localAnsible?.workingDirExists ? t('runtime.shared.present') : t('runtime.shared.missing') }}</p>
        </article>
      </div>
      <div v-else class="state-card">{{ t('runtime.localRuntime.noStatus') }}</div>

      <div v-if="statusSummary?.summary" class="summary-card">{{ translateRuntimeText(statusSummary.summary) }}</div>
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
          <p class="eyebrow">{{ t('runtime.localRuntime.jobsEyebrow') }}</p>
          <h3>{{ t('runtime.localRuntime.latestJobs', { backend: localBackendLabel(selectedLocalBackend) }) }}</h3>
        </div>
      </div>

      <div v-if="jobsLoading" class="state-card">{{ t('runtime.localRuntime.loadingJobs') }}</div>
      <div v-else-if="jobs.length" class="job-list">
        <article v-for="job in jobs" :key="job.id" class="job-item">
          <div class="job-main">
            <div>
              <strong>#{{ job.id }} {{ job.action }}</strong>
              <p class="job-meta">{{ t('runtime.localRuntime.jobMeta', { forwardId: job.forwardId || '-', tunnelId: job.tunnelId || '-', nodeId: job.nodeId || '-' }) }}</p>
            </div>
            <div class="job-side">
              <span :class="['status-chip', `status-${job.status}`]">{{ runtimeJobStatusLabel(job.status) }}</span>
              <span class="job-time">{{ formatJobTime(job) }}</span>
            </div>
          </div>
          <code v-if="job.message" class="job-message">{{ translateRuntimeText(job.message) }}</code>
        </article>
      </div>
      <div v-else class="state-card">{{ t('runtime.localRuntime.noJobs') }}</div>
    </section>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
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

const { t, formatDateTime, translateLiteral } = useAppI18n()

const localBackendOptions = computed(() => ([
  {
    value: 'nftables_ansible',
    label: t('runtime.localRuntime.backends.nftables.label'),
    description: t('runtime.localRuntime.backends.nftables.description'),
    applyPlaybook: 'config/deploy/ansible/playbooks/forward_apply_nftables.yml',
    removePlaybook: 'config/deploy/ansible/playbooks/forward_remove_nftables.yml',
    recommended: true
  },
  {
    value: 'iptables_ansible',
    label: t('runtime.localRuntime.backends.iptables.label'),
    description: t('runtime.localRuntime.backends.iptables.description'),
    applyPlaybook: 'config/deploy/ansible/playbooks/forward_apply.yml',
    removePlaybook: 'config/deploy/ansible/playbooks/forward_remove.yml',
    recommended: false
  }
]))

function normalizeLocalBackend(value) {
  const normalized = String(value ?? '').trim().toLowerCase()
  return localBackendOptions.value.some(option => option.value === normalized) ? normalized : 'nftables_ansible'
}

function isLocalBackend(value) {
  return localBackendOptions.value.some(option => option.value === String(value ?? '').trim().toLowerCase())
}

function getLocalBackendMeta(value) {
  return localBackendOptions.value.find(option => option.value === normalizeLocalBackend(value)) || localBackendOptions.value[0]
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

function translateRuntimeText(value, fallback = '-') {
  const text = String(value ?? '').trim()
  if (!text) return fallback
  return translateLiteral(text)
}

function resolveRuntimeError(error, fallbackKey) {
  return translateRuntimeText(error?.response?.data?.msg || error?.message, t(fallbackKey))
}

function parseRuntimeJsonObject(value, labelKey) {
  const trimmed = String(value ?? '').trim()
  if (!trimmed) return {}
  const label = t(labelKey)
  let parsed
  try {
    parsed = JSON.parse(trimmed)
  } catch {
    throw new Error(t('runtime.localRuntime.errors.invalidJson', { label }))
  }
  if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') {
    throw new Error(t('runtime.localRuntime.errors.invalidObject', { label }))
  }
  return parsed
}

function buildRuntimeAnsiblePayload(source = runtimeAnsibleForm.value) {
  const form = normalizeRuntimeAnsibleForm(source)
  const extraVars = parseRuntimeJsonObject(form.extraVarsJson, 'runtime.localRuntime.fields.extraVarsJson')
  const environmentOverrides = parseRuntimeJsonObject(form.environmentJson, 'runtime.localRuntime.fields.environmentJson')
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
    case 0: return t('runtime.shared.pending')
    case 1: return t('runtime.shared.running')
    case 2: return t('runtime.shared.success')
    case 3: return t('runtime.shared.failed')
    default: return t('runtime.shared.unknown')
  }
}

function formatJobTime(job) {
  const raw = job?.completedAt || job?.updatedAt || job?.createdAt
  if (!raw) return '-'
  return formatDateTime(raw) || String(raw)
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
    return t('runtime.localRuntime.errors.invalidPreview', { message: error.message })
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
      validationError.value = t('runtime.localRuntime.errors.savedConfigInvalid')
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
    validationError.value = error.message || t('runtime.localRuntime.errors.invalidRuntimeJson')
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
    validationError.value = resolveRuntimeError(error, 'runtime.localRuntime.errors.saveFailed')
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
    statusError.value = resolveRuntimeError(error, 'runtime.localRuntime.errors.fetchStatusFailed')
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
    doctorOutput.value = resolveRuntimeError(error, 'runtime.localRuntime.errors.doctorFailed')
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
