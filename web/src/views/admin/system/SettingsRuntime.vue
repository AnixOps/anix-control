<template>
  <div class="settings-section" data-settings-panel="runtime">
    <UiSection :title="t('adminSettings.runtime.modes.title')" :description="t('adminSettings.runtime.modes.description')">
      <div class="runtime-modes">
        <UiCard :class="['runtime-mode', { 'is-active': !runtimeNodeXMode }]" heading-tag="h3" :title="t('runtime.workbench.localCard.title')" :description="t('runtime.workbench.localCard.description')">
          <UiGroupedList>
            <UiGroupedListRow :label="t('runtime.workbench.localCard.currentState')">
              <UiBadge :tone="runtimeNodeXMode ? 'neutral' : 'success'" :label="runtimeNodeXMode ? t('runtime.workbench.state.standby') : t('runtime.workbench.state.activeBackend')" />
            </UiGroupedListRow>
            <UiGroupedListRow :label="t('runtime.localRuntime.fields.inventory')" :value="runtimeAnsibleForm.inventory || '—'" />
            <UiGroupedListRow :label="t('runtime.localRuntime.fields.command')" :value="runtimeAnsibleForm.command || defaultRuntimeAnsibleConfig.command" />
          </UiGroupedList>
        </UiCard>
        <UiCard :class="['runtime-mode', { 'is-active': runtimeNodeXMode }]" heading-tag="h3" :title="t('runtime.workbench.nodeXCard.title')" :description="t('runtime.workbench.nodeXCard.description')">
          <UiGroupedList>
            <UiGroupedListRow :label="t('runtime.workbench.localCard.currentState')">
              <UiBadge :tone="runtimeNodeXMode ? 'success' : 'neutral'" :label="runtimeNodeXMode ? t('runtime.workbench.state.activeBackend') : t('runtime.workbench.state.standby')" />
            </UiGroupedListRow>
            <UiGroupedListRow :label="t('runtime.nodeX.cards.baseUrl')" :value="runtimeNodeXBaseUrl || '—'" />
            <UiGroupedListRow :label="t('runtime.nodeX.cards.tokenConfigured')" :value="runtimeNodeXToken ? t('runtime.shared.yes') : t('runtime.shared.no')" />
          </UiGroupedList>
        </UiCard>
      </div>
    </UiSection>

    <UiSection :title="t('runtime.workbench.doctor.title')" :description="t('runtime.workbench.doctor.description')">
      <template #actions>
        <UiButton :icon="RotateCw" :loading="runtimeStatusLoading" data-test="runtime-refresh-status" @click="fetchRuntimeStatusSafe">{{ t('runtime.shared.refreshStatus') }}</UiButton>
        <UiButton variant="primary" :icon="Stethoscope" :loading="runtimeDoctorRunning" data-test="runtime-run-doctor" @click="runRuntimeDoctorCheckSafe">{{ t('runtime.workbench.actions.runDoctorActiveRuntime') }}</UiButton>
      </template>
      <UiSkeleton v-if="showStatusSkeleton" variant="card" :label="t('runtime.workbench.doctor.loadingStatus')" />
      <UiErrorState
        v-else-if="runtimeStatusError"
        compact
        heading-tag="h3"
        :title="t('runtime.workbench.errors.fetchStatusFailed')"
        :error="runtimeStatusError"
        @retry="fetchRuntimeStatusSafe"
      />
      <div v-else-if="!runtimeStatusLoading" class="runtime-status">
        <UiGroupedList :title="t('runtime.workbench.cards.backend')">
          <UiGroupedListRow :label="t('runtime.workbench.cards.backend')" :value="runtimeBackendLabel(runtimeStatus?.config?.backend || (runtimeNodeXMode ? 'gost' : runtimeBackend))" />
          <UiGroupedListRow :label="t('runtime.workbench.cards.nodeXMode')" :value="runtimeStatus?.config?.nodeXMode ? t('runtime.shared.enabled') : t('runtime.shared.disabled')" />
          <UiGroupedListRow :label="t('runtime.workbench.cards.attachment')" :value="runtimeStatus?.attachment?.model || '—'" :description="translateRuntimeText(runtimeStatus?.attachment?.description, '')" />
        </UiGroupedList>
        <UiGroupedList :title="t('runtime.workbench.cards.panelVerdict')">
          <UiGroupedListRow :label="t('runtime.workbench.cards.panelVerdict')" :description="translateRuntimeText(runtimeStatus?.runtimeReady?.reason, '')">
            <UiBadge :tone="runtimeStatus?.runtimeReady?.ready ? 'success' : 'warning'" :label="runtimeStatus?.runtimeReady?.ready ? t('runtime.shared.ready') : t('runtime.shared.notReady')" />
          </UiGroupedListRow>
          <UiGroupedListRow :label="t('runtime.shared.reachability')" :description="translateRuntimeText(runtimeStatus?.reachability?.reason, '')">
            <UiBadge :tone="runtimeStatus?.reachability?.ready ? 'success' : 'warning'" :label="runtimeStatus?.reachability?.ready ? t('runtime.shared.ready') : t('runtime.shared.notReady')" />
          </UiGroupedListRow>
        </UiGroupedList>
        <UiGroupedList v-if="runtimeStatus?.config?.nodeXMode" :title="t('runtime.workbench.cards.nodeXSnapshot')">
          <UiGroupedListRow :label="t('runtime.nodeX.cards.baseUrl')" :value="runtimeStatus?.config?.baseUrl || '—'" />
          <UiGroupedListRow :label="t('runtime.workbench.cards.baseUrlConfigured')" :value="yesNo(runtimeStatus?.config?.baseUrlConfigured)" />
          <UiGroupedListRow :label="t('runtime.nodeX.cards.tokenConfigured')" :value="yesNo(runtimeStatus?.config?.tokenConfigured)" />
          <UiGroupedListRow :label="t('runtime.nodeX.cards.health')" :value="runtimeStatus?.health?.ok ? t('runtime.shared.ready') : t('runtime.shared.unavailable')" />
          <UiGroupedListRow :label="t('runtime.workbench.cards.runtimeVersion')" :value="runtimeStatus?.runtimeStatus?.version || '—'" />
        </UiGroupedList>
        <UiGroupedList v-else :title="t('runtime.workbench.cards.localAnsible')">
          <UiGroupedListRow :label="t('runtime.localRuntime.fields.command')" :value="runtimeStatus?.localAnsible?.command || defaultRuntimeAnsibleConfig.command" />
          <UiGroupedListRow :label="t('runtime.localRuntime.cards.commandFound')" :value="yesNo(runtimeStatus?.localAnsible?.commandFound)" />
          <UiGroupedListRow :label="t('runtime.localRuntime.fields.inventory')" :value="runtimeStatus?.localAnsible?.inventoryExists ? t('runtime.shared.present') : t('runtime.shared.missing')" />
          <UiGroupedListRow :label="t('runtime.workbench.cards.playbooks')" :value="runtimeStatus?.localAnsible?.applyPlaybookExists && runtimeStatus?.localAnsible?.removePlaybookExists ? t('runtime.shared.ready') : t('runtime.shared.missing')" />
          <UiGroupedListRow :label="t('runtime.localRuntime.fields.workingDir')" :value="runtimeStatus?.localAnsible?.workingDirExists ? t('runtime.shared.present') : t('runtime.shared.missing')" />
        </UiGroupedList>
        <UiGroupedList v-if="runtimeStatus?.warnings?.length" :title="t('runtime.shared.warnings')" class="runtime-status__wide">
          <UiGroupedListRow v-for="warning in runtimeStatus.warnings" :key="warning" :label="translateRuntimeText(warning)">
            <template #leading><UiIcon :icon="TriangleAlert" class="runtime-warning-icon" /></template>
          </UiGroupedListRow>
        </UiGroupedList>
      </div>
      <p class="settings-note">{{ t('runtime.workbench.doctor.note') }}</p>
    </UiSection>

    <UiSection :title="t('adminSettings.runtime.commands.title')" :description="t('runtime.workbench.doctor.summary')">
      <div class="runtime-commands">
        <UiCodeBlock :label="t('runtime.shared.bash')" :code="runtimeDisplayedCommands.bash.join('\n')" wrap max-height="" />
        <UiCodeBlock :label="t('runtime.shared.powerShell')" :code="runtimeDisplayedCommands.powerShell.join('\n')" wrap max-height="" />
        <UiCodeBlock :label="t('runtime.shared.bootstrapVerify')" :code="runtimeDisplayedCommands.upgrade.join('\n')" wrap max-height="" />
        <UiCodeBlock :label="t('runtime.shared.references')" :code="runtimeDisplayedCommands.references.join('\n')" wrap max-height="" />
      </div>
      <UiCodeBlock
        :label="t('runtime.shared.doctorOutput')"
        :code="runtimeDoctorOutput || t('runtime.shared.doctorNotExecuted')"
        :copyable="Boolean(runtimeDoctorOutput)"
        data-test="runtime-doctor-output"
      />
    </UiSection>
  </div>
</template>

<script setup>
// 系统设置 → 转发运行时: the v4.1 flux forwarding runtime, read-only: the
// active backend (NodeX or local Ansible, read from the system config
// keys), the runtime doctor and the operator commands. The flux pages that
// edited it and the runtime job list were removed in v4.2 (F5d); the
// runtime itself goes with the legacy cleanup (F5c).
import { computed, onMounted, ref } from 'vue'
import { RotateCw, Stethoscope, TriangleAlert } from '@lucide/vue'
import { getForwardRuntimeStatus, getSystemConfig, runForwardRuntimeDoctor } from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import { humanizeForwardRuntimeBackend } from '@/utils/forwardRuntime'
import { isMaskedSecret } from '@/constants/secrets'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiCard from '@/ui/UiCard.vue'
import UiCodeBlock from '@/ui/UiCodeBlock.vue'
import UiErrorState from '@/ui/UiErrorState.vue'
import UiGroupedList from '@/ui/UiGroupedList.vue'
import UiGroupedListRow from '@/ui/UiGroupedListRow.vue'
import UiIcon from '@/ui/UiIcon.vue'
import UiSection from '@/ui/UiSection.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import { useDelayedLoading } from '@/ui/composables/useDelayedLoading'

const { t, translateLiteral } = useAppI18n()

const runtimeNodeXModeKey = 'forward.runtime.nodex_mode'
const runtimeBackendKey = 'forward.runtime_backend'
const runtimeAnsibleBackendKey = 'forward.runtime.ansible.backend'
const runtimeAnsibleConfigKey = 'forward.runtime.ansible.config'
const runtimeLegacyAnsibleConfigKey = 'forward.runtime.iptables_ansible.config'
const runtimeNodeXBaseUrlKey = 'forward.runtime.nodex.base_url'
const runtimeNodeXTokenKey = 'forward.runtime.nodex.token'
const runtimeNodeXTimeoutKey = 'forward.runtime.nodex.timeout_seconds'
const defaultRuntimeAnsibleConfig = Object.freeze({
  inventory: 'config/deploy/ansible/inventory.ini',
  playbookApply: 'config/deploy/ansible/playbooks/forward_apply_nftables.yml',
  playbookRemove: 'config/deploy/ansible/playbooks/forward_remove_nftables.yml',
  command: 'ansible-playbook',
  workingDir: 'config/deploy/ansible',
  targetPattern: '{{node.host}}',
  timeoutSeconds: 120,
  ansibleConfig: 'config/deploy/ansible/ansible.cfg',
  become: false
})
const runtimeBackend = ref('nftables_ansible')
const runtimeNodeXMode = ref(false)
const runtimeNodeXBaseUrl = ref('')
const runtimeNodeXToken = ref('')
const runtimeNodeXTimeout = ref(15)
const runtimeAnsibleForm = ref(createRuntimeAnsibleForm())
const runtimeStatus = ref(null)
const runtimeStatusLoading = ref(false)
const runtimeStatusError = ref('')
const runtimeDoctorOutput = ref('')
const runtimeDoctorRunning = ref(false)
const runtimeDoctorSummary = ref(null)
const showStatusSkeleton = useDelayedLoading(runtimeStatusLoading)

const defaultNodeXBaseUrl = 'http://127.0.0.1:18081'
const runtimeOperatorBaseUrl = computed(() => runtimeNodeXBaseUrl.value?.trim() || defaultNodeXBaseUrl)
// A stored token reads MASKED_SECRET; the commands show a placeholder then.
const runtimeOperatorToken = computed(() => {
  const token = runtimeNodeXToken.value?.trim() || ''
  return token && !isMaskedSecret(token) ? token : '<FORWARD_API_TOKEN>'
})
const runtimeOperatorReferences = computed(() => ([
  t('runtime.workbench.references.panelRuntimeDoc'),
  t('runtime.workbench.references.panelRelayOnboarding'),
  t('runtime.workbench.references.nodeXRepo'),
  t('runtime.workbench.references.panelNodeXOnboarding')
]))
const runtimeDisplayedCommands = computed(() => {
  const source = runtimeDoctorSummary.value?.commands
  if (source) {
    return {
      powerShell: Array.isArray(source.powerShell) ? source.powerShell : [],
      bash: Array.isArray(source.bash) ? source.bash : [],
      upgrade: Array.isArray(source.upgrade) ? source.upgrade : [],
      references: Array.isArray(source.references) ? source.references : []
    }
  }

  if (runtimeNodeXMode.value) {
    return {
      powerShell: [
        `Invoke-WebRequest '${runtimeOperatorBaseUrl.value}/health' | Select-Object -ExpandProperty Content`,
        `Invoke-WebRequest '${runtimeOperatorBaseUrl.value}/api/v2/internal/forward/runtime/status' -Headers @{ Authorization = 'Bearer ${runtimeOperatorToken.value}' } | Select-Object -ExpandProperty Content`
      ],
      bash: [
        `curl -fsSL '${runtimeOperatorBaseUrl.value}/health'`,
        `curl -fsSL -H 'Authorization: Bearer ${runtimeOperatorToken.value}' '${runtimeOperatorBaseUrl.value}/api/v2/internal/forward/runtime/status'`
      ],
      upgrade: [
        'git clone https://github.com/zdwtest/NodeX.git',
        'cd NodeX/control-plane && go run ./cmd/control-plane --version',
        'cd NodeX/control-plane && go run ./cmd/control-plane --config ../deploy/config/control-plane.yaml --addr :18081 --forward-api-token <FORWARD_API_TOKEN>'
      ],
      references: runtimeOperatorReferences.value
    }
  }

  return {
    powerShell: ['Get-Command ansible-playbook', 'ansible-playbook --version'],
    bash: ['command -v ansible-playbook', 'ansible-playbook --version'],
    upgrade: ['git pull --ff-only', 'go test ./internal/service/... -run ForwardRuntime'],
    references: [
      'docs/guide/forward-relay-onboarding.md',
      'docs/guide/forward-tunnel-runtime-ops.md',
      'docs/guide/forward-tunnel-smoke-test.md'
    ]
  }
})

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

function createRuntimeAnsibleForm(source = {}) {
  const extraVars = source.extraVars && typeof source.extraVars === 'object' && !Array.isArray(source.extraVars)
    ? source.extraVars
    : {}
  const rawEnvironment = source.environment && typeof source.environment === 'object' && !Array.isArray(source.environment)
    ? source.environment
    : {}
  const environment = { ...rawEnvironment }
  const ansibleConfig = String(source.ansibleConfig ?? environment.ANSIBLE_CONFIG ?? defaultRuntimeAnsibleConfig.ansibleConfig).trim() ||
    defaultRuntimeAnsibleConfig.ansibleConfig
  delete environment.ANSIBLE_CONFIG

  return {
    inventory: String(source.inventory ?? defaultRuntimeAnsibleConfig.inventory).trim() || defaultRuntimeAnsibleConfig.inventory,
    playbookApply: String(source.playbookApply ?? source.applyPlaybook ?? defaultRuntimeAnsibleConfig.playbookApply).trim() || defaultRuntimeAnsibleConfig.playbookApply,
    playbookRemove: String(source.playbookRemove ?? source.removePlaybook ?? defaultRuntimeAnsibleConfig.playbookRemove).trim() || defaultRuntimeAnsibleConfig.playbookRemove,
    command: String(source.command ?? defaultRuntimeAnsibleConfig.command).trim() || defaultRuntimeAnsibleConfig.command,
    workingDir: String(source.workingDir ?? defaultRuntimeAnsibleConfig.workingDir).trim() || defaultRuntimeAnsibleConfig.workingDir,
    targetPattern: String(source.targetPattern ?? defaultRuntimeAnsibleConfig.targetPattern).trim() || defaultRuntimeAnsibleConfig.targetPattern,
    timeoutSeconds: Number.isFinite(Number(source.timeoutSeconds)) && Number(source.timeoutSeconds) > 0
      ? Number(source.timeoutSeconds)
      : defaultRuntimeAnsibleConfig.timeoutSeconds,
    ansibleConfig,
    become: parseRuntimeBoolean(source.become) ?? defaultRuntimeAnsibleConfig.become,
    extraVarsJson: Object.keys(extraVars).length ? JSON.stringify(extraVars, null, 2) : '',
    environmentJson: Object.keys(environment).length ? JSON.stringify(environment, null, 2) : ''
  }
}

function normalizeRuntimeAnsibleForm(source = {}) {
  const form = createRuntimeAnsibleForm(source)
  return {
    ...form,
    extraVarsJson: normalizeJsonObjectText(source.extraVarsJson ?? form.extraVarsJson),
    environmentJson: normalizeJsonObjectText(source.environmentJson ?? form.environmentJson)
  }
}

const translateRuntimeText = (value, fallback = '-') => {
  const text = String(value ?? '').trim()
  if (!text) return fallback
  return translateLiteral(text)
}

const resolveRuntimeError = (error, fallbackKey) => (
  translateRuntimeText(error?.response?.data?.msg || error?.message, t(fallbackKey))
)

const yesNo = value => (value ? t('runtime.shared.yes') : t('runtime.shared.no'))
const runtimeBackendLabel = value => humanizeForwardRuntimeBackend(t, value)

const fetchRuntimeStatusSafe = async () => {
  runtimeStatusLoading.value = true
  runtimeStatusError.value = ''
  try {
    const res = await getForwardRuntimeStatus()
    runtimeStatus.value = res.data?.data || res.data || null
    runtimeDoctorSummary.value = null
  } catch (err) {
    runtimeStatusError.value = resolveRuntimeError(err, 'runtime.workbench.errors.fetchStatusFailed')
    runtimeStatus.value = null
  } finally {
    runtimeStatusLoading.value = false
  }
}

const runRuntimeDoctorCheckSafe = async () => {
  runtimeDoctorRunning.value = true
  runtimeDoctorOutput.value = ''
  try {
    const res = await runForwardRuntimeDoctor()
    const payload = res.data?.data || res.data || res
    runtimeDoctorSummary.value = payload
    runtimeDoctorOutput.value = JSON.stringify(payload, null, 2)
  } catch (err) {
    runtimeDoctorOutput.value = resolveRuntimeError(err, 'runtime.workbench.errors.doctorFailed')
  } finally {
    runtimeDoctorRunning.value = false
  }
}

const fetchForwardRuntimeConfig = async () => {
  let explicitNodeXMode = null
  runtimeAnsibleForm.value = createRuntimeAnsibleForm()
  try {
    const nodeXModeRes = await getSystemConfig(runtimeNodeXModeKey)
    explicitNodeXMode = parseRuntimeBoolean(nodeXModeRes.data?.value)
  } catch (err) {
    console.error('get forward runtime NodeX mode config failed:', err)
  }

  try {
    const backendRes = await getSystemConfig(runtimeBackendKey)
    const backendValue = String(backendRes.data?.value || 'nftables_ansible').trim().toLowerCase()
    runtimeBackend.value = backendValue || 'nftables_ansible'
  } catch (err) {
    console.error('get forward runtime backend config failed:', err)
  }
  runtimeNodeXMode.value = explicitNodeXMode === null
    ? runtimeBackend.value === 'gost'
    : explicitNodeXMode
  if (runtimeNodeXMode.value) {
    runtimeBackend.value = 'gost'
  } else if (runtimeBackend.value === 'gost') {
    try {
      const localBackendRes = await getSystemConfig(runtimeAnsibleBackendKey)
      runtimeBackend.value = String(localBackendRes.data?.value || 'nftables_ansible').trim().toLowerCase() || 'nftables_ansible'
    } catch {
      runtimeBackend.value = 'nftables_ansible'
    }
  }

  try {
    const configRes = await getSystemConfig(runtimeAnsibleConfigKey)
    let rawValue = configRes.data?.value || ''
    if (!rawValue) {
      try {
        const legacyConfigRes = await getSystemConfig(runtimeLegacyAnsibleConfigKey)
        rawValue = legacyConfigRes.data?.value || ''
      } catch {
        rawValue = ''
      }
    }
    if (rawValue) {
      try {
        const parsed = JSON.parse(rawValue)
        if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') {
          throw new Error('saved payload must be a JSON object')
        }
        runtimeAnsibleForm.value = normalizeRuntimeAnsibleForm(parsed)
      } catch {
        runtimeAnsibleForm.value = createRuntimeAnsibleForm()
      }
    }
  } catch (err) {
    console.error('get forward runtime ansible config failed:', err)
  }
  try {
    const baseUrlRes = await getSystemConfig(runtimeNodeXBaseUrlKey)
    runtimeNodeXBaseUrl.value = baseUrlRes.data?.value || ''
  } catch (err) {
    console.error('get forward runtime NodeX base URL config failed:', err)
  }
  try {
    const tokenRes = await getSystemConfig(runtimeNodeXTokenKey)
    runtimeNodeXToken.value = tokenRes.data?.value || ''
  } catch (err) {
    console.error('get forward runtime NodeX token config failed:', err)
  }
  try {
    const timeoutRes = await getSystemConfig(runtimeNodeXTimeoutKey)
    const timeoutValue = Number(timeoutRes.data?.value)
    runtimeNodeXTimeout.value = Number.isFinite(timeoutValue) && timeoutValue > 0 ? timeoutValue : 15
  } catch (err) {
    console.error('get forward runtime NodeX timeout config failed:', err)
  }
}

onMounted(async () => {
  await fetchForwardRuntimeConfig()
  fetchRuntimeStatusSafe()
})
</script>

<style scoped>
.settings-section {
  display: flex;
  flex-direction: column;
  gap: var(--space-12);
  min-width: 0;
}

.runtime-modes {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--space-4);
}

.runtime-mode {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.runtime-mode.is-active {
  box-shadow: 0 0 0 1.5px var(--accent);
}

.runtime-mode :deep(.ui-glist__list) {
  background: var(--bg-grouped);
  box-shadow: none;
}

.runtime-status {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--space-6) var(--space-4);
}

.runtime-status__wide {
  grid-column: 1 / -1;
}

.runtime-warning-icon {
  color: var(--warning);
}

.runtime-commands {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--space-4);
}

.settings-note {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

@media (max-width: 1023.98px) {
  .runtime-modes,
  .runtime-status,
  .runtime-commands {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
