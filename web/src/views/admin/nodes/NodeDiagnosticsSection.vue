<template>
  <component
    :is="embedded ? 'div' : UiSection"
    class="node-diagnostics"
    v-bind="rootProps"
    data-testid="node-diagnostics"
    :aria-busy="loading || running ? 'true' : 'false'"
  >
    <div class="node-diagnostics__bar">
      <p v-if="info" class="node-diagnostics__agent" data-testid="diagnostics-agent">
        <UiBadge
          :tone="info.connected ? 'success' : 'neutral'"
          :label="info.connected ? t('nodeDiagnostics.online') : t('nodeDiagnostics.notConnected')"
        />
        <span v-if="info.connected && info.agent_version">{{ t('nodeDiagnostics.agentVersion', { version: info.agent_version }) }}</span>
        <span v-if="supportedNames" class="node-diagnostics__supports">{{ t('nodeDiagnostics.supports', { list: supportedNames }) }}</span>
      </p>
      <UiIconButton
        class="node-diagnostics__refresh"
        variant="secondary"
        :icon="RefreshCw"
        :label="t('nodeDiagnostics.refresh')"
        :disabled="loading || Boolean(running)"
        data-testid="diagnostics-refresh"
        @click="load"
      />
    </div>

    <UiSkeleton v-if="loading && !info" variant="card" :label="t('nodeDiagnostics.loading')" />
    <UiErrorState v-else-if="loadError && !info" :title="t('nodeDiagnostics.loadFailed')" :error="loadError" @retry="load" />

    <template v-else-if="info">
      <UiEmptyState
        v-if="!info.connected"
        :icon="WifiOff"
        heading-tag="h3"
        :title="t('nodeDiagnostics.offline.title')"
        :description="t('nodeDiagnostics.offline.description')"
        data-testid="diagnostics-offline"
      />
      <UiEmptyState
        v-else-if="!info.diagnostics"
        :icon="CircleSlash"
        heading-tag="h3"
        :title="t('nodeDiagnostics.unsupported.title')"
        :description="info.agent_version
          ? t('nodeDiagnostics.unsupported.descriptionVersion', { version: info.agent_version })
          : t('nodeDiagnostics.unsupported.description')"
        data-testid="diagnostics-unsupported"
      />

      <template v-else>
        <div class="node-diagnostics__group" role="group" :aria-label="t('nodeDiagnostics.service.title', { service: serviceName })">
          <h3 class="node-diagnostics__heading">{{ t('nodeDiagnostics.service.title', { service: serviceName }) }}</h3>
          <div class="node-diagnostics__actions">
            <UiSelect
              v-if="serviceOptions.length > 1"
              v-model="service"
              size="md"
              class="node-diagnostics__service"
              :aria-label="t('nodeDiagnostics.service.label')"
              :options="serviceOptions"
              :disabled="Boolean(running)"
              data-testid="diagnostics-service"
            />
            <UiButton
              v-if="supports('service_status')"
              :icon="Activity"
              :loading="running === 'service_status'"
              :disabled="Boolean(running)"
              data-testid="diagnostics-status"
              @click="run('service_status', serviceParams())"
            >{{ t('nodeDiagnostics.actions.service_status') }}</UiButton>
            <UiButton
              v-if="supports('service_restart')"
              variant="danger-soft"
              :icon="RotateCcw"
              :loading="running === 'service_restart'"
              :disabled="Boolean(running) || !info.can_restart"
              data-testid="diagnostics-restart"
              @click="restart"
            >{{ t('nodeDiagnostics.actions.service_restart') }}</UiButton>
            <span v-if="supports('log_tail')" class="node-diagnostics__logs">
              <UiSelect
                v-model="lines"
                size="md"
                class="node-diagnostics__lines"
                :aria-label="t('nodeDiagnostics.lines.label')"
                :options="lineOptions"
                :disabled="Boolean(running)"
                data-testid="diagnostics-lines"
              />
              <UiButton
                :icon="ScrollText"
                :loading="running === 'log_tail'"
                :disabled="Boolean(running)"
                data-testid="diagnostics-logs"
                @click="run('log_tail', { ...serviceParams(), lines: Number(lines) })"
              >{{ t('nodeDiagnostics.actions.log_tail') }}</UiButton>
            </span>
          </div>
          <p v-if="supports('service_restart') && !info.can_restart" class="node-diagnostics__hint" data-testid="diagnostics-restart-hint">
            {{ t('nodeDiagnostics.restartSuperOnly') }}
          </p>
        </div>

        <div v-if="showChecks" class="node-diagnostics__group" role="group" :aria-label="t('nodeDiagnostics.forward.title')">
          <h3 class="node-diagnostics__heading">{{ t('nodeDiagnostics.forward.title') }}</h3>
          <p v-if="!hops.length" class="node-diagnostics__hint" data-testid="diagnostics-no-hops">{{ t('nodeDiagnostics.forward.noHops') }}</p>
          <template v-else>
            <UiSelect
              v-model="hop"
              size="md"
              class="node-diagnostics__hop"
              :label="t('nodeDiagnostics.forward.hop')"
              :options="hopOptions"
              :disabled="Boolean(running)"
              data-testid="diagnostics-hop"
            />
            <div class="node-diagnostics__actions">
              <UiButton
                v-for="check in forwardChecks"
                :key="check"
                :loading="running === check"
                :disabled="Boolean(running) || !selectedHop"
                :data-testid="`diagnostics-${actionKey(check)}`"
                @click="run(check, hopParams())"
              >{{ actionLabel(check) }}</UiButton>
            </div>
          </template>
        </div>
        <p v-else-if="info.diagnostics && isForward && !info.forward_checks" class="node-diagnostics__hint" data-testid="diagnostics-no-checks">
          {{ t('nodeDiagnostics.forward.unsupported') }}
        </p>
      </template>

      <section v-if="running" class="node-diagnostics__running" role="status" data-testid="diagnostics-running">
        <UiSpinner />
        <span>{{ t('nodeDiagnostics.running', { action: actionLabel(running), seconds: waitSeconds(running) }) }}</span>
      </section>

      <p v-else-if="runError" class="node-diagnostics__error" role="alert" data-testid="diagnostics-error">{{ runError }}</p>

      <section v-else-if="result" class="node-diagnostics__result" data-testid="diagnostics-result" :aria-label="t('nodeDiagnostics.result.title')">
        <header class="node-diagnostics__result-head">
          <UiBadge
            :tone="result.success ? 'success' : 'danger'"
            :label="result.success ? t('nodeDiagnostics.result.success') : t('nodeDiagnostics.result.failed')"
            data-testid="diagnostics-result-status"
          />
          <strong>{{ actionLabel(result.action) }}</strong>
          <span class="node-diagnostics__meta tabular-nums">{{ t('nodeDiagnostics.result.duration', { ms: result.duration_ms }) }}</span>
          <span v-if="result.at" class="node-diagnostics__meta">{{ format.dateTime(result.at) }}</span>
        </header>
        <p v-if="result.error" class="node-diagnostics__failure" data-testid="diagnostics-result-error">{{ result.error }}</p>
        <ul v-if="result.items.length" class="node-diagnostics__items" data-testid="diagnostics-items">
          <li v-for="(item, index) in result.items" :key="index" class="node-diagnostics__item">
            <code>{{ item.target }}</code>
            <span class="node-diagnostics__meta">{{ item.protocol }}</span>
            <UiBadge :tone="itemTone(item.status)" :dot="false" :label="itemLabel(item.status)" />
            <span v-if="item.rtt_us" class="node-diagnostics__meta tabular-nums">{{ t('nodeDiagnostics.result.rtt', { ms: (item.rtt_us / 1000).toFixed(1) }) }}</span>
            <span v-if="item.message" class="node-diagnostics__meta">{{ item.message }}</span>
          </li>
        </ul>
        <UiCodeBlock
          :label="t('nodeDiagnostics.result.output')"
          :code="result.output || t('nodeDiagnostics.result.noOutput')"
          :copy-disabled="!result.output"
          max-height="320px"
          data-testid="diagnostics-output"
        />
      </section>

      <section v-if="info.tasks && info.tasks.length" class="node-diagnostics__history" data-testid="diagnostics-history">
        <h3 class="node-diagnostics__heading">{{ t('nodeDiagnostics.history.title') }}</h3>
        <ul class="node-diagnostics__tasks">
          <li v-for="task in info.tasks" :key="task.task_id" class="node-diagnostics__task">
            <UiBadge :tone="taskTone(task)" :label="taskLabel(task)" />
            <span class="node-diagnostics__task-action">{{ actionLabel(task.action) }}</span>
            <span class="node-diagnostics__meta">{{ format.dateTime(task.updated_at || task.created_at) }}</span>
            <span class="node-diagnostics__meta tabular-nums">{{ t('nodeDiagnostics.result.duration', { ms: task.duration_ms || 0 }) }}</span>
            <UiButton size="sm" variant="ghost" :data-testid="`diagnostics-show-${task.task_id}`" @click="showTask(task)">{{ t('nodeDiagnostics.history.show') }}</UiButton>
          </li>
        </ul>
      </section>
      <p v-else-if="info.history === false && info.connected" class="node-diagnostics__hint" data-testid="diagnostics-no-history">
        {{ t('nodeDiagnostics.history.none') }}
      </p>
    </template>
  </component>
</template>

<script setup>
// 诊断 of a node (5.0, design D7): the whitelisted actions of the Agent's
// agent.diagnostic operation, run over the Agent Control stream — service
// status, restart (super administrators, confirmed), log tail, and, for a
// forward node, the forward checks of one hop. The result is answered in
// the request (GET/POST /api/v4/kernel/nodes/:id/diagnostics); the panel
// shows what the node's Agent session runs, the latest result with its
// output (copyable), and a proxy node's newest recorded tasks.
import { computed, onMounted, ref, watch } from 'vue'
import { Activity, CircleSlash, RefreshCw, RotateCcw, ScrollText, WifiOff } from '@lucide/vue'
import { getKernelNodeDiagnostics, runKernelNodeDiagnostic } from '@/api/kernel'
import { useAppI18n } from '@/composables/useAppI18n'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiCodeBlock from '@/ui/UiCodeBlock.vue'
import UiEmptyState from '@/ui/UiEmptyState.vue'
import UiErrorState from '@/ui/UiErrorState.vue'
import UiIconButton from '@/ui/UiIconButton.vue'
import UiSection from '@/ui/UiSection.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import UiSpinner from '@/ui/UiSpinner.vue'
import { useConfirm } from '@/ui/composables/useConfirm'
import { useFormat } from '@/ui/composables/useFormat'

const props = defineProps({
  // proxy-<id> or forward-<id>.
  nodeRef: { type: String, required: true },
  name: { type: String, default: '' },
  // The hops the node hosts, for the forward checks: [{ route_id,
  // hop_index, label }].
  hops: { type: Array, default: () => [] },
  // Inside a card that has its own title (the forwarding node page).
  embedded: { type: Boolean, default: false }
})

const FORWARD_CHECKS = ['forward.listen', 'forward.port_conflict', 'forward.connect', 'forward.udp_probe']
const GENERIC = ['service_status', 'service_restart', 'log_tail']
const LOG_LINES = [100, 500, 1000]
// The wait, in seconds, a request grants its action: a restart may take a
// while, a probe is short.
const WAIT = { service_restart: 60 }
const DEFAULT_WAIT = 30
const CHECK_TONE = { ok: 'success', failed: 'danger', inconclusive: 'warning', skipped: 'neutral' }

const { t, te } = useAppI18n()
const format = useFormat()
const confirm = useConfirm()

const info = ref(null)
const loading = ref(false)
const loadError = ref(null)
const running = ref('')
const runError = ref('')
const result = ref(null)
const lines = ref(String(LOG_LINES[0]))
const hop = ref('')
let generation = 0

const rootProps = computed(() => (props.embedded ? {} : { title: t('nodeDiagnostics.title'), description: t('nodeDiagnostics.description') }))
const isForward = computed(() => props.nodeRef.startsWith('forward-'))
const actions = computed(() => (Array.isArray(info.value?.actions) ? info.value.actions : []))
const services = computed(() => (Array.isArray(info.value?.services) && info.value.services.length ? info.value.services : ['gost']))
const service = ref('')
const serviceName = computed(() => (services.value.includes(service.value) ? service.value : services.value[0]))
const serviceOptions = computed(() => services.value.map(name => ({ value: name, label: name })))
const forwardChecks = computed(() => FORWARD_CHECKS.filter(check => supports(check)))
const showChecks = computed(() => forwardChecks.value.length > 0 && (isForward.value || props.hops.length > 0))
const lineOptions = computed(() => LOG_LINES
  .filter(count => !info.value?.log_tail_max || count <= info.value.log_tail_max)
  .map(count => ({ value: String(count), label: t('nodeDiagnostics.lines.option', { count }) })))
const hopOptions = computed(() => props.hops.map(item => ({ value: hopKey(item), label: item.label || `${item.route_id} · ${item.hop_index}` })))
const selectedHop = computed(() => props.hops.find(item => hopKey(item) === hop.value) || null)
const supportedNames = computed(() => (info.value?.connected ? actions.value : [])
  .map(action => actionLabel(action.action))
  .join(t('nodeDiagnostics.separator')))

function hopKey(item) {
  return `${item.route_id}#${item.hop_index}`
}

function supports(action) {
  return actions.value.some(item => item.action === action)
}

// forward.listen is read as forward_listen: a dot would be a path.
function actionKey(action) {
  return String(action || '').replace(/\./g, '_')
}

function actionLabel(action) {
  const key = `nodeDiagnostics.actions.${actionKey(action)}`
  return te(key) ? t(key) : String(action || '')
}

function waitSeconds(action) {
  return WAIT[action] || DEFAULT_WAIT
}

function serviceParams() {
  return { service: serviceName.value }
}

function hopParams() {
  return selectedHop.value ? { route_id: selectedHop.value.route_id, hop_index: Number(selectedHop.value.hop_index) } : {}
}

function itemTone(status) {
  return CHECK_TONE[status] || 'neutral'
}

function itemLabel(status) {
  const key = `nodeDiagnostics.checkStates.${status}`
  return te(key) ? t(key) : String(status || '')
}

function taskTone(task) {
  if (task.status === 'completed' && task.success) return 'success'
  if (task.status === 'pending' || task.status === 'dispatched') return 'info'
  return 'danger'
}

function taskLabel(task) {
  if (task.status === 'pending' || task.status === 'dispatched') return t('nodeDiagnostics.history.pending')
  return task.success ? t('nodeDiagnostics.result.success') : t('nodeDiagnostics.result.failed')
}

// The refusal as the administrator reads it: a message per code, the
// server's text when the code is not known.
function failureText(cause, action) {
  const response = cause?.response?.data
  const code = response?.error?.code || ''
  const message = response?.error?.message || (typeof response?.error === 'string' ? response.error : '')
  const key = `nodeDiagnostics.errors.${code}`
  if (code && te(key)) return t(key, { seconds: waitSeconds(action), message })
  return message || cause?.message || t('nodeDiagnostics.errors.unknown')
}

function normalizeResult(answer) {
  const checkResult = answer?.result && typeof answer.result === 'object' ? answer.result : null
  return {
    action: answer?.action || '',
    success: Boolean(answer?.success),
    output: typeof answer?.output === 'string' ? answer.output : '',
    error: typeof answer?.error === 'string' ? answer.error : '',
    duration_ms: Number(answer?.duration_ms) || 0,
    items: Array.isArray(checkResult?.items) ? checkResult.items : [],
    at: new Date().toISOString()
  }
}

async function load() {
  const current = ++generation
  loading.value = true
  try {
    const answer = await getKernelNodeDiagnostics(props.nodeRef)
    if (current !== generation) return
    info.value = answer
    loadError.value = null
  } catch (cause) {
    if (current !== generation) return
    loadError.value = cause
  } finally {
    if (current === generation) loading.value = false
  }
}

async function run(action, params) {
  if (running.value) return
  running.value = action
  runError.value = ''
  try {
    const answer = await runKernelNodeDiagnostic(props.nodeRef, { action, params, timeoutSeconds: waitSeconds(action) })
    result.value = normalizeResult(answer)
  } catch (cause) {
    result.value = null
    runError.value = failureText(cause, action)
  } finally {
    running.value = ''
  }
  // The newest tasks, and whether the Agent still runs it.
  await load()
}

async function restart() {
  const confirmed = await confirm({
    title: t('nodeDiagnostics.restart.title', { service: serviceName.value, name: props.name || props.nodeRef }),
    message: t('nodeDiagnostics.restart.message', { service: serviceName.value }),
    confirmLabel: t('nodeDiagnostics.restart.confirm'),
    tone: 'danger'
  })
  if (confirmed) await run('service_restart', serviceParams())
}

function showTask(task) {
  runError.value = ''
  result.value = {
    action: task.action,
    success: Boolean(task.success),
    output: typeof task.output === 'string' ? task.output : '',
    error: typeof task.error === 'string' ? task.error : '',
    duration_ms: Number(task.duration_ms) || 0,
    items: [],
    at: task.updated_at || task.created_at || ''
  }
}

watch(() => props.hops, (hops) => {
  if (!hops.some(item => hopKey(item) === hop.value)) hop.value = hops.length ? hopKey(hops[0]) : ''
}, { immediate: true })

watch(() => props.nodeRef, () => {
  info.value = null
  service.value = ''
  result.value = null
  runError.value = ''
  load()
})

onMounted(load)

defineExpose({ load, run, info, result })
</script>

<style scoped>
.node-diagnostics {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  min-width: 0;
}

.node-diagnostics__bar {
  display: flex;
  gap: var(--space-3);
  align-items: flex-start;
  justify-content: space-between;
}

.node-diagnostics__refresh {
  flex: none;
  margin-left: auto;
}

.node-diagnostics__agent {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2) var(--space-3);
  align-items: center;
  min-width: 0;
  margin: 0;
  color: var(--label-1);
  font-size: var(--type-callout-size);
}

.node-diagnostics__supports,
.node-diagnostics__meta,
.node-diagnostics__hint {
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.node-diagnostics__hint {
  margin: 0;
}

.node-diagnostics__group {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4);
  border-radius: var(--radius-md);
  background: var(--bg-elevated);
  box-shadow: 0 0 0 0.5px var(--separator);
}

.node-diagnostics__heading {
  margin: 0;
  color: var(--label-1);
  font-size: var(--type-body-size);
  font-weight: var(--weight-semibold);
}

.node-diagnostics__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
}

.node-diagnostics__logs {
  display: inline-flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
}

.node-diagnostics__lines {
  width: 160px;
}

.node-diagnostics__hop {
  max-width: 420px;
}

.node-diagnostics__running {
  display: flex;
  gap: var(--space-3);
  align-items: center;
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-sm);
  background: var(--fill-1);
  color: var(--label-1);
  font-size: var(--type-callout-size);
}

.node-diagnostics__error,
.node-diagnostics__failure {
  margin: 0;
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-sm);
  background: var(--danger-soft);
  color: color-mix(in srgb, var(--danger) 78%, var(--label-1));
  font-size: var(--type-callout-size);
  overflow-wrap: anywhere;
}

.node-diagnostics__result,
.node-diagnostics__history {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  min-width: 0;
}

.node-diagnostics__result-head {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2) var(--space-3);
  align-items: center;
}

.node-diagnostics__items,
.node-diagnostics__tasks {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}

.node-diagnostics__item,
.node-diagnostics__task {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1) var(--space-3);
  align-items: center;
  min-width: 0;
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-sm);
  background: var(--fill-1);
  font-size: var(--type-callout-size);
}

.node-diagnostics__item code {
  overflow-wrap: anywhere;
}

/* Secondary text on the --fill-1 rows stays at 4.5:1. */
.node-diagnostics__item .node-diagnostics__meta,
.node-diagnostics__task .node-diagnostics__meta {
  color: color-mix(in srgb, var(--label-2) 70%, var(--label-1));
}

.node-diagnostics__task-action {
  flex: 1 1 140px;
  color: var(--label-1);
}

@media (max-width: 639.98px) {
  .node-diagnostics__lines,
  .node-diagnostics__hop {
    width: 100%;
    max-width: none;
  }
}
</style>
