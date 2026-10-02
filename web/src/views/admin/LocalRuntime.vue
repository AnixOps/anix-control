<template>
  <div class="runtime-page local-runtime-page">
    <UiPageHeader :title="t('forwardNodesPage.title')" :description="t('forwardNodesPage.descriptions.local')">
      <template #meta>
        <UiBadge :tone="localModeActive ? 'success' : 'warning'" :label="localModeActive ? t('runtime.localRuntime.cards.localActiveValue') : t('runtime.localRuntime.cards.standbyValue')" />
      </template>
      <template #actions>
        <UiIconButton variant="secondary" :icon="RefreshCw" :label="t('common.actions.refresh')" :disabled="jobsLoading || statusLoading" data-test="local-refresh" @click="refreshAll" />
        <UiButton variant="primary" :loading="saving" data-test="local-save" @click="saveLocalConfig">{{ t('runtime.localRuntime.saveActivate') }}</UiButton>
      </template>
    </UiPageHeader>

    <ForwardNodesModeNav current="local" />

    <div :class="['runtime-banner', localModeActive ? 'is-on' : 'is-off']" role="status" data-test="local-banner">
      <UiIcon :icon="localModeActive ? CircleCheck : CircleAlert" :size="18" />
      <div>
        <strong>{{ localModeActive ? t('runtime.localRuntime.activeBannerTitle') : t('runtime.localRuntime.standbyBannerTitle') }}</strong>
        <p>
          {{
            localModeActive
              ? t('runtime.localRuntime.activeBannerText', { backend: localBackendLabel(selectedLocalBackend) })
              : t('runtime.localRuntime.standbyBannerText')
          }}
        </p>
        <p data-test="local-backend-note">{{ t('runtime.localRuntime.heroTextSecondary') }}</p>
      </div>
    </div>

    <UiSection :title="t('runtime.localRuntime.configTitle')" :description="t('runtime.localRuntime.configCopy')">
      <div class="runtime-form">
        <div class="local-backend">
          <UiRadioGroup
            :model-value="selectedLocalBackend"
            :label="t('runtime.localRuntime.executorEyebrow')"
            :options="backendRadioOptions"
            data-test="local-backend"
            @update:model-value="selectLocalBackend"
          />
        </div>
        <div class="local-executor">
          <p class="local-executor__hint" data-test="local-executor-hint">
            {{ t('runtime.localRuntime.executorHint', { backend: localBackendLabel(selectedLocalBackend), backendKey: selectedLocalBackend }) }}
          </p>
          <UiButton size="sm" :disabled="saving" data-test="local-defaults" @click="applyDefaultRuntimeAnsibleConfig">
            {{ t('runtime.localRuntime.defaultsAction') }}
          </UiButton>
        </div>

        <div class="form-grid">
          <UiTextField id="ansible-inventory" v-model.trim="runtimeAnsibleForm.inventory" placeholder="config/deploy/ansible/inventory.ini" :label="t('runtime.localRuntime.fields.inventory')" />
          <UiTextField id="ansible-apply-playbook" v-model.trim="runtimeAnsibleForm.playbookApply" :placeholder="defaultRuntimeAnsibleConfig.playbookApply" :label="t('runtime.localRuntime.fields.applyPlaybook')" />
          <UiTextField id="ansible-remove-playbook" v-model.trim="runtimeAnsibleForm.playbookRemove" :placeholder="defaultRuntimeAnsibleConfig.playbookRemove" :label="t('runtime.localRuntime.fields.removePlaybook')" />
          <UiTextField id="ansible-command" v-model.trim="runtimeAnsibleForm.command" :placeholder="t('runtimePages.localRuntime.placeholders.command')" :label="t('runtime.localRuntime.fields.command')" />
          <UiTextField id="ansible-working-dir" v-model.trim="runtimeAnsibleForm.workingDir" placeholder="config/deploy/ansible" :label="t('runtime.localRuntime.fields.workingDir')" />
          <UiTextField id="ansible-target-pattern" v-model.trim="runtimeAnsibleForm.targetPattern" placeholder="{{node.host}}" :label="t('runtime.localRuntime.fields.targetPattern')" />
          <UiTextField id="ansible-timeout-seconds" v-model.number="runtimeAnsibleForm.timeoutSeconds" type="number" min="1" placeholder="120" suffix="s" :label="t('runtime.localRuntime.fields.timeoutSeconds')" />
          <UiTextField id="ansible-config-path" v-model.trim="runtimeAnsibleForm.ansibleConfig" placeholder="config/deploy/ansible/ansible.cfg" :label="t('runtime.localRuntime.fields.ansibleConfig')" />
        </div>

        <UiSwitch id="ansible-become" v-model="runtimeAnsibleForm.become" :label="t('runtime.localRuntime.fields.useBecome')" />

        <div class="form-grid">
          <UiTextarea id="ansible-extra-vars-json" v-model="runtimeAnsibleForm.extraVarsJson" class="local-json" :rows="6" placeholder='{"change_window":"maintenance"}' :label="t('runtime.localRuntime.fields.extraVarsJson')" :help="t('runtime.localRuntime.extraVarsHint')" />
          <UiTextarea id="ansible-environment-json" v-model="runtimeAnsibleForm.environmentJson" class="local-json" :rows="6" placeholder='{"ANSIBLE_HOST_KEY_CHECKING":"False"}' :label="t('runtime.localRuntime.fields.environmentJson')" :help="t('runtime.localRuntime.environmentHint')" />
        </div>

        <UiTextarea id="ansible-runtime-preview" class="local-json" :model-value="runtimeConfigPreview" :rows="8" readonly :label="t('runtime.localRuntime.fields.generatedJson')" :help="t('runtime.localRuntime.generatedHint')" />

        <p v-if="validationError" class="form-error" role="alert">{{ validationError }}</p>
      </div>
    </UiSection>

    <RuntimeStatusPanel
      :title="t('runtime.localRuntime.probeTitle')"
      :description="t('runtime.localRuntime.heroTextPrimary')"
      :loading="statusLoading"
      :error="statusError"
      :has-status="Boolean(statusSummary)"
      :empty-text="t('runtime.localRuntime.noStatus')"
      :summary="statusSummary?.summary ? translateRuntimeText(statusSummary.summary) : ''"
      :warnings="(statusSummary?.warnings || []).map(warning => translateRuntimeText(warning))"
      :commands="displayedCommands"
      :doctor-running="doctorRunning"
      :doctor-output="doctorOutput"
      @refresh="fetchLocalStatus"
      @doctor="runLocalDoctor"
    >
      <UiGroupedList :title="t('runtime.shared.panelConfig')" heading-tag="h3">
        <UiGroupedListRow :label="t('runtime.shared.panelConfig')" :value="localModeActive ? t('runtime.localRuntime.cards.localActiveValue') : t('runtime.localRuntime.cards.standbyValue')" />
        <UiGroupedListRow :label="t('runtime.localRuntime.cards.backend')" :value="localBackendLabel(actualBackend)" />
        <UiGroupedListRow :label="t('runtime.localRuntime.cards.preferredLocalBackend')" :value="localBackendLabel(selectedLocalBackend)" />
        <UiGroupedListRow :label="t('runtime.localRuntime.cards.attachment')" :description="statusSummary.attachment?.model || '-'" />
      </UiGroupedList>
      <UiGroupedList :title="t('runtime.shared.reachability')" heading-tag="h3">
        <UiGroupedListRow :label="t('runtime.shared.reachability')" :description="translateRuntimeText(statusSummary.reachability?.reason)">
          <template #value><UiBadge :status="statusSummary.reachability?.ready ? 'online' : 'offline'" :label="statusSummary.reachability?.ready ? t('runtime.shared.reachable') : t('runtime.shared.notReady')" /></template>
        </UiGroupedListRow>
        <UiGroupedListRow :label="t('runtime.localRuntime.cards.runtimeReady')" :description="translateRuntimeText(statusSummary.runtimeReady?.reason)" :value="statusSummary.runtimeReady?.ready ? t('runtime.shared.yes') : t('runtime.shared.no')" />
      </UiGroupedList>
      <UiGroupedList :title="t('runtime.shared.executor')" heading-tag="h3">
        <UiGroupedListRow :label="t('runtime.localRuntime.fields.command')" :value="statusSummary.localAnsible?.command || runtimeAnsibleForm.command || t('runtimePages.localRuntime.defaults.command')" />
        <UiGroupedListRow :label="t('runtime.localRuntime.cards.firewallDriver')" :value="statusSummary.localAnsible?.firewallDriver || firewallDriverLabel(selectedLocalBackend)" />
        <UiGroupedListRow :label="t('runtime.localRuntime.cards.commandFound')" :value="statusSummary.localAnsible?.commandFound ? t('runtime.shared.yes') : t('runtime.shared.no')" />
        <UiGroupedListRow :label="t('runtime.localRuntime.cards.become')" :value="statusSummary.localAnsible?.become ? t('runtime.shared.yes') : t('runtime.shared.no')" />
      </UiGroupedList>
      <UiGroupedList :title="t('runtime.shared.files')" heading-tag="h3">
        <UiGroupedListRow :label="t('runtime.localRuntime.cards.inventory')" :value="statusSummary.localAnsible?.inventoryExists ? t('runtime.shared.present') : t('runtime.shared.missing')" />
        <UiGroupedListRow :label="t('runtime.localRuntime.cards.applyPlaybook')" :value="statusSummary.localAnsible?.applyPlaybookExists ? t('runtime.shared.present') : t('runtime.shared.missing')" />
        <UiGroupedListRow :label="t('runtime.localRuntime.cards.removePlaybook')" :value="statusSummary.localAnsible?.removePlaybookExists ? t('runtime.shared.present') : t('runtime.shared.missing')" />
        <UiGroupedListRow :label="t('runtime.localRuntime.cards.workingDir')" :value="statusSummary.localAnsible?.workingDirExists ? t('runtime.shared.present') : t('runtime.shared.missing')" />
      </UiGroupedList>
    </RuntimeStatusPanel>

    <RuntimeJobsTable
      :title="t('runtime.localRuntime.latestJobs', { backend: localBackendLabel(selectedLocalBackend) })"
      :jobs="jobs"
      :loading="jobsLoading"
      :empty-text="t('runtime.localRuntime.noJobs')"
      storage-key="admin.local-runtime-jobs"
    />
  </div>
</template>

<script setup>
// 转发节点 › 本地运行时 (/admin/forward/local): the Ansible executor on the
// panel host (stateless; system config keys), its probes and the latest
// local jobs. Execution plane, apart from the flux-panel forward pages.
// UI U7 restyled it; config keys, calls and payloads are unchanged.
import { computed, onMounted, ref } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { getLocalRuntimeStatus, getSystemConfig, listForwardRuntimeJobs, runLocalRuntimeDoctor, setSystemConfig } from '@/api/admin'
import { CircleAlert, CircleCheck, RefreshCw } from '@lucide/vue'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiGroupedList from '@/ui/UiGroupedList.vue'
import UiGroupedListRow from '@/ui/UiGroupedListRow.vue'
import UiIcon from '@/ui/UiIcon.vue'
import UiIconButton from '@/ui/UiIconButton.vue'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import UiRadioGroup from '@/ui/UiRadioGroup.vue'
import UiSection from '@/ui/UiSection.vue'
import UiSwitch from '@/ui/UiSwitch.vue'
import UiTextarea from '@/ui/UiTextarea.vue'
import UiTextField from '@/ui/UiTextField.vue'
import ForwardNodesModeNav from './forward-nodes/ForwardNodesModeNav.vue'
import RuntimeJobsTable from './forward-nodes/RuntimeJobsTable.vue'
import RuntimeStatusPanel from './forward-nodes/RuntimeStatusPanel.vue'
import { normalizeRuntimeJob as normalizeJob, parseRuntimeBoolean, unwrapPayload } from './forward-nodes/forwardNodeModel'

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

const { t, translateLiteral } = useAppI18n()

const localBackendOptions = computed(() => ([
  {
    value: 'nftables_ansible',
    label: t('runtime.localRuntime.backends.nftables.label'),
    description: t('runtime.localRuntime.backends.nftables.description'),
    applyPlaybook: 'config/deploy/ansible/playbooks/forward_apply_nftables.yml',
    removePlaybook: 'config/deploy/ansible/playbooks/forward_remove_nftables.yml',
    recommended: true
  }
]))

const backendRadioOptions = computed(() => localBackendOptions.value.map(option => ({
  value: option.value,
  label: `${option.label} · ${option.recommended ? t('runtime.localRuntime.recommended') : t('runtime.localRuntime.legacy')}`,
  description: `${option.description} ${option.applyPlaybook}`
})))

function normalizeLocalBackend(value) {
  const normalized = String(value ?? '').trim().toLowerCase()
  return localBackendOptions.value.some(option => option.value === normalized) ? normalized : 'nftables_ansible'
}

function isLocalBackend(value) {
  // iptables 已下线: 归一化为 nftables, 旧值仍识别为本地后端
  const normalized = String(value ?? '').trim().toLowerCase()
  return normalized === 'iptables_ansible' || localBackendOptions.value.some(option => option.value === normalized)
}

function getLocalBackendMeta(value) {
  return localBackendOptions.value.find(option => option.value === normalizeLocalBackend(value)) || localBackendOptions.value[0]
}

function firewallDriverLabel(value) {
  return 'nftables'
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
  const base = {
    powerShell: ['Get-Command ansible-playbook', 'ansible-playbook --version', 'Get-Content .\\config\\deploy\\ansible\\inventory.ini'],
    bash: ['command -v ansible-playbook', 'ansible-playbook --version', 'cat ./config/deploy/ansible/inventory.ini'],
    upgrade: ['git pull --ff-only', 'go test ./internal/service/... -run ForwardRuntime'],
    references: ['docs/guide/forward-relay-onboarding.md', 'docs/guide/forward-tunnel-runtime-ops.md', 'docs/reference/runtime.md']
  }
  // iptables 已下线: 本地后端统一为 nftables
  base.powerShell.push('wsl nft --version')
  base.bash.push('nft --version', 'nft list tables')
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
    const payload = unwrapPayload(res)
    const list = Array.isArray(payload?.list) ? payload.list : Array.isArray(payload) ? payload : []
    jobs.value = list.map(job => normalizeJob(job, t('runtime.shared.unknown')))
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
    statusSummary.value = unwrapPayload(res) || null
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
    const payload = unwrapPayload(res) || null
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

<style scoped src="./forward-nodes/runtime-page.css"></style>

<style scoped>
.local-executor {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3);
  align-items: flex-start;
  justify-content: space-between;
}

.local-executor__hint {
  flex: 1 1 320px;
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
  overflow-wrap: anywhere;
}

.local-backend {
  min-width: 0;
}

.local-backend :deep(.ui-radio),
.local-backend :deep(.ui-radio__text) {
  min-width: 0;
}

.local-backend :deep(.ui-radio__desc),
.local-backend :deep(.ui-radio__label) {
  overflow-wrap: anywhere;
}

.local-json :deep(textarea) {
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
}
</style>
