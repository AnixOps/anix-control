<template>
  <UiSection
    class="reality-targets"
    :title="t('realityTargets.title')"
    :description="t('realityTargets.description')"
    data-testid="reality-targets"
    :aria-busy="loading || probing ? 'true' : 'false'"
  >
    <div class="reality-targets__bar">
      <p v-if="support" class="reality-targets__agent" data-testid="reality-agent">
        <UiBadge
          :tone="support.connected ? 'success' : 'neutral'"
          :label="support.connected ? t('realityTargets.online') : t('realityTargets.notConnected')"
        />
        <span v-if="support.connected && support.agent_version">{{ t('realityTargets.agentVersion', { version: support.agent_version }) }}</span>
        <span v-if="report && report.node_address" class="reality-targets__meta">{{ t('realityTargets.nodeAddress', { address: report.node_address }) }}</span>
      </p>
      <UiIconButton
        class="reality-targets__refresh"
        variant="secondary"
        :icon="RefreshCw"
        :label="t('realityTargets.refresh')"
        :disabled="loading || probing"
        data-testid="reality-refresh"
        @click="load"
      />
    </div>

    <UiSkeleton v-if="loading && !report" variant="card" :label="t('realityTargets.loading')" />
    <UiErrorState v-else-if="loadError && !report" :title="t('realityTargets.loadFailed')" :error="loadError" @retry="load" />

    <template v-else-if="report">
      <div class="reality-targets__group" role="group" :aria-label="t('realityTargets.current.title')">
        <h3 class="reality-targets__heading">{{ t('realityTargets.current.title') }}</h3>
        <p v-if="!report.current.length" class="reality-targets__hint" data-testid="reality-no-protocols">{{ t('realityTargets.current.none') }}</p>
        <ul v-else class="reality-targets__list" data-testid="reality-current">
          <li v-for="item in report.current" :key="item.protocol_id" class="reality-targets__row">
            <span class="reality-targets__name">{{ item.name || `#${item.protocol_id}` }}</span>
            <code>{{ item.server_name || '—' }}</code>
            <span v-if="item.dest" class="reality-targets__meta">{{ t('realityTargets.current.dest', { dest: item.dest }) }}</span>
          </li>
        </ul>
      </div>

      <div class="reality-targets__group" role="group" :aria-label="t('realityTargets.auto.title')" data-testid="reality-auto">
        <UiSwitch
          :model-value="Boolean(auto && auto.enabled)"
          :label="t('realityTargets.auto.title')"
          :description="t('realityTargets.auto.description')"
          :disabled="savingAuto || !report.current.length"
          data-testid="reality-auto-switch"
          @update:model-value="setAuto"
        />
        <ul v-if="auto && (auto.enabled || auto.last_run_at)" class="reality-targets__facts">
          <li v-if="auto.enabled && auto.next_run_at">{{ t('realityTargets.auto.nextRun', { time: format.dateTime(auto.next_run_at) }) }}</li>
          <li v-else-if="auto.enabled">{{ t('realityTargets.auto.nextRunSoon') }}</li>
          <li v-if="auto.last_run_at">{{ t('realityTargets.auto.lastRun', { time: format.dateTime(auto.last_run_at) }) }}</li>
          <li v-if="auto.last_outcome" data-testid="reality-auto-outcome">{{ t('realityTargets.auto.outcome', { outcome: auto.last_outcome }) }}</li>
          <li v-if="auto.last_switch_at">{{ t('realityTargets.auto.lastSwitch', { time: format.dateTime(auto.last_switch_at) }) }}</li>
        </ul>
        <p v-if="autoError" class="reality-targets__error" role="alert">{{ autoError }}</p>
      </div>

      <div class="reality-targets__group" role="group" :aria-label="t('realityTargets.probe.title')">
        <h3 class="reality-targets__heading">{{ t('realityTargets.probe.title') }}</h3>
        <p v-if="!support || !support.connected" class="reality-targets__hint" data-testid="reality-offline">{{ t('realityTargets.probe.offline') }}</p>
        <p v-else-if="!support.capable" class="reality-targets__hint" data-testid="reality-unsupported">
          {{ support.agent_version ? t('realityTargets.probe.unsupportedVersion', { version: support.agent_version }) : t('realityTargets.probe.unsupported') }}
        </p>
        <template v-else>
          <UiTextarea
            v-model="extra"
            :label="t('realityTargets.probe.candidates')"
            :help="t('realityTargets.probe.candidatesHint')"
            :rows="2"
            :disabled="probing"
            data-testid="reality-candidates"
          />
          <UiCheckbox
            v-model="withDefaults"
            :label="t('realityTargets.probe.withDefaults', { count: defaultCount })"
            :disabled="probing"
            data-testid="reality-defaults"
          />
          <div class="reality-targets__actions">
            <UiButton :icon="Radar" :loading="probing" :disabled="probing || (!withDefaults && !extraList.length)" data-testid="reality-probe" @click="probe">
              {{ t('realityTargets.probe.run') }}
            </UiButton>
            <span v-if="probing" class="reality-targets__meta" role="status">{{ t('realityTargets.probe.running') }}</span>
          </div>
        </template>
        <p v-if="probeError" class="reality-targets__error" role="alert" data-testid="reality-probe-error">{{ probeError }}</p>
      </div>

      <div v-if="report.recommended" class="reality-targets__group reality-targets__group--accent" data-testid="reality-recommended">
        <h3 class="reality-targets__heading">{{ t('realityTargets.recommended.title') }}</h3>
        <p class="reality-targets__row">
          <code>{{ targetLabel(report.recommended) }}</code>
          <span class="reality-targets__meta tabular-nums">{{ t('realityTargets.score', { score: report.recommended.score }) }}</span>
        </p>
      </div>

      <section v-if="report.targets.length" class="reality-targets__results" data-testid="reality-results" :aria-label="t('realityTargets.results.title')">
        <h3 class="reality-targets__heading">{{ t('realityTargets.results.title') }}</h3>
        <ul class="reality-targets__targets">
          <li v-for="target in report.targets" :key="targetLabel(target)" class="reality-targets__target" :data-testid="`reality-target-${target.server_name}`">
            <div class="reality-targets__row">
              <UiBadge :tone="verdictTone(target.verdict)" :dot="false" :label="verdictLabel(target.verdict)" />
              <code class="reality-targets__host">{{ targetLabel(target) }}</code>
              <span v-if="target.verdict === 'ok'" class="reality-targets__meta tabular-nums">{{ t('realityTargets.score', { score: target.score }) }}</span>
              <span v-if="target.result && target.result.handshake_ms != null && target.verdict === 'ok'" class="reality-targets__meta tabular-nums">
                {{ t('realityTargets.results.handshake', { ms: target.result.handshake_ms }) }}
              </span>
              <span v-if="target.result && target.result.curve" class="reality-targets__meta">{{ target.result.curve }}</span>
              <UiBadge v-if="target.result && target.result.cdn" tone="warning" :dot="false" :label="t('realityTargets.results.cdn', { cdn: target.result.cdn })" />
              <span v-if="target.used_by" class="reality-targets__meta">{{ t('realityTargets.results.usedBy', { count: target.used_by }) }}</span>
              <span v-if="target.probed_at" class="reality-targets__meta">{{ format.dateTime(target.probed_at) }}</span>
              <span v-if="target.verdict === 'ok' && report.current.length" class="reality-targets__apply">
                <UiButton
                  v-for="item in report.current"
                  :key="item.protocol_id"
                  size="sm"
                  variant="secondary"
                  :disabled="Boolean(applying) || isCurrent(item, target)"
                  :loading="applying === applyKey(item, target)"
                  :data-testid="`reality-apply-${item.protocol_id}-${target.server_name}`"
                  @click="apply(item, target)"
                >{{ isCurrent(item, target) ? t('realityTargets.apply.inUse') : applyLabel(item) }}</UiButton>
              </span>
            </div>
            <p v-if="target.verdict !== 'ok'" class="reality-targets__meta">
              {{ failureSummary(target) }}
            </p>
          </li>
        </ul>
      </section>
      <p v-else class="reality-targets__hint" data-testid="reality-no-results">{{ t('realityTargets.results.none') }}</p>
      <p v-if="applyMessage" class="reality-targets__hint" role="status" data-testid="reality-applied">{{ applyMessage }}</p>
    </template>
  </UiSection>
</template>

<script setup>
// 目标站 of a proxy node (Reality target probing): the node's Agent probes
// candidate REALITY targets (dest and SNI) from the node's own network
// (reality.probe), Control ranks them; the administrator applies one to a
// Reality protocol of the node (confirmed: clients need a subscription
// refresh), or turns on automatic selection, which rechecks the node every
// 6 hours and replaces a target that failed twice in a row (confirmed for
// the same reason). GET/POST /api/v4/kernel/nodes/:id/reality-targets
// {,/probe,/apply}, PUT .../auto.
import { computed, onMounted, ref, watch } from 'vue'
import { Radar, RefreshCw } from '@lucide/vue'
import { applyKernelRealityTarget, getKernelRealityTargets, probeKernelRealityTargets, setKernelRealityTargetAuto } from '@/api/kernel'
import { useAppI18n } from '@/composables/useAppI18n'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiCheckbox from '@/ui/UiCheckbox.vue'
import UiErrorState from '@/ui/UiErrorState.vue'
import UiIconButton from '@/ui/UiIconButton.vue'
import UiSection from '@/ui/UiSection.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import UiSwitch from '@/ui/UiSwitch.vue'
import UiTextarea from '@/ui/UiTextarea.vue'
import { useConfirm } from '@/ui/composables/useConfirm'
import { useFormat } from '@/ui/composables/useFormat'

const props = defineProps({
  // proxy-<id>.
  nodeRef: { type: String, required: true },
  name: { type: String, default: '' }
})

const VERDICT_TONE = { ok: 'success', failed: 'danger', error: 'warning' }

const { t, te } = useAppI18n()
const format = useFormat()
const confirm = useConfirm()

const report = ref(null)
const support = ref(null)
const auto = ref(null)
const savingAuto = ref(false)
const autoError = ref('')
const defaultCount = ref(0)
const loading = ref(false)
const loadError = ref(null)
const probing = ref(false)
const probeError = ref('')
const applying = ref('')
const applyMessage = ref('')
const extra = ref('')
const withDefaults = ref(true)
let generation = 0

const extraList = computed(() => extra.value.split(/[\s,，]+/).map(item => item.trim()).filter(Boolean))

function normalizeReport(value) {
  const source = value && typeof value === 'object' ? value : {}
  return {
    node_address: source.node_address || '',
    current: Array.isArray(source.current) ? source.current : [],
    recommended: source.recommended || null,
    targets: Array.isArray(source.targets) ? source.targets : []
  }
}

function targetLabel(target) {
  return Number(target.port) && Number(target.port) !== 443 ? `${target.server_name}:${target.port}` : target.server_name
}

function verdictTone(verdict) {
  return VERDICT_TONE[verdict] || 'neutral'
}

function verdictLabel(verdict) {
  const key = `realityTargets.verdicts.${verdict}`
  return te(key) ? t(key) : String(verdict || '')
}

function failureSummary(target) {
  if (target.verdict === 'error') return target.error || t('realityTargets.errors.unknown')
  const failures = Array.isArray(target.result?.failures) ? target.result.failures : []
  const names = failures.map((code) => {
    const key = `realityTargets.failures.${code}`
    return te(key) ? t(key) : code
  }).join(t('realityTargets.separator'))
  return names || target.result?.error || ''
}

function applyKey(item, target) {
  return `${item.protocol_id}#${targetLabel(target)}`
}

function applyLabel(item) {
  return report.value.current.length > 1 ? t('realityTargets.apply.toProtocol', { name: item.name || `#${item.protocol_id}` }) : t('realityTargets.apply.button')
}

function isCurrent(item, target) {
  const dest = `${target.server_name}:${Number(target.port) || 443}`
  return item.server_name === target.server_name && (!item.dest || item.dest === dest)
}

function failureText(cause) {
  const response = cause?.response?.data
  const code = response?.error?.code || ''
  const message = response?.error?.message || (typeof response?.error === 'string' ? response.error : '')
  const key = `realityTargets.errors.${code}`
  if (code && te(key)) return t(key, { message })
  return message || cause?.message || t('realityTargets.errors.unknown')
}

function take(answer) {
  report.value = normalizeReport(answer?.report)
  if (answer?.support) support.value = answer.support
  if (answer?.auto) auto.value = answer.auto
  if (Array.isArray(answer?.default_candidates)) defaultCount.value = answer.default_candidates.length
}

async function load() {
  const current = ++generation
  loading.value = true
  try {
    const answer = await getKernelRealityTargets(props.nodeRef)
    if (current !== generation) return
    take(answer)
    loadError.value = null
  } catch (cause) {
    if (current !== generation) return
    loadError.value = cause
  } finally {
    if (current === generation) loading.value = false
  }
}

async function probe() {
  if (probing.value) return
  probing.value = true
  probeError.value = ''
  applyMessage.value = ''
  try {
    take(await probeKernelRealityTargets(props.nodeRef, { candidates: extraList.value, includeDefaults: withDefaults.value }))
  } catch (cause) {
    probeError.value = failureText(cause)
  } finally {
    probing.value = false
  }
}

async function apply(item, target) {
  const label = targetLabel(target)
  const confirmed = await confirm({
    title: t('realityTargets.apply.title', { target: label, name: item.name || `#${item.protocol_id}` }),
    message: t('realityTargets.apply.message', { from: item.server_name || '—', target: label }),
    confirmLabel: t('realityTargets.apply.confirm')
  })
  if (!confirmed) return
  applying.value = applyKey(item, target)
  applyMessage.value = ''
  probeError.value = ''
  try {
    await applyKernelRealityTarget(props.nodeRef, { protocolId: item.protocol_id, serverName: target.server_name, port: Number(target.port) || 443 })
    applyMessage.value = t('realityTargets.apply.done', { target: label, name: item.name || `#${item.protocol_id}` })
  } catch (cause) {
    probeError.value = failureText(cause)
  } finally {
    applying.value = ''
  }
  await load()
}

async function setAuto(enabled) {
  if (savingAuto.value) return
  if (enabled) {
    const confirmed = await confirm({
      title: t('realityTargets.auto.confirmTitle'),
      message: t('realityTargets.auto.confirmMessage'),
      confirmLabel: t('realityTargets.auto.confirm')
    })
    if (!confirmed) return
  }
  savingAuto.value = true
  autoError.value = ''
  try {
    auto.value = await setKernelRealityTargetAuto(props.nodeRef, enabled)
  } catch (cause) {
    autoError.value = failureText(cause)
  } finally {
    savingAuto.value = false
  }
}

watch(() => props.nodeRef, () => {
  report.value = null
  support.value = null
  auto.value = null
  autoError.value = ''
  probeError.value = ''
  applyMessage.value = ''
  load()
})

onMounted(load)

defineExpose({ load, probe, report })
</script>

<style scoped>
.reality-targets {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  min-width: 0;
}

.reality-targets__bar {
  display: flex;
  gap: var(--space-3);
  align-items: flex-start;
  justify-content: space-between;
}

.reality-targets__refresh {
  flex: none;
  margin-left: auto;
}

.reality-targets__agent {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2) var(--space-3);
  align-items: center;
  min-width: 0;
  margin: 0;
  color: var(--label-1);
  font-size: var(--type-callout-size);
}

.reality-targets__meta,
.reality-targets__hint {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.reality-targets__error {
  margin: 0;
  color: color-mix(in srgb, var(--danger) 78%, var(--label-1));
  font-size: var(--type-callout-size);
}

.reality-targets__group {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4);
  border-radius: var(--radius-md);
  background: var(--bg-elevated);
  box-shadow: 0 0 0 0.5px var(--separator);
}

.reality-targets__group--accent {
  box-shadow: 0 0 0 1px var(--accent);
}

.reality-targets__heading {
  margin: 0;
  color: var(--label-1);
  font-size: var(--type-body-size);
  font-weight: var(--weight-semibold);
}

.reality-targets__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
}

.reality-targets__facts {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  margin: 0;
  padding: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
  list-style: none;
}

.reality-targets__list,
.reality-targets__targets {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}

.reality-targets__row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2) var(--space-3);
  align-items: center;
  min-width: 0;
  margin: 0;
}

.reality-targets__target {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  padding: var(--space-3) 0;
  border-bottom: 0.5px solid var(--separator);
}

.reality-targets__name {
  color: var(--label-1);
  font-weight: var(--weight-semibold);
}

.reality-targets__host {
  overflow-wrap: anywhere;
}

.reality-targets__apply {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  margin-left: auto;
}
</style>
