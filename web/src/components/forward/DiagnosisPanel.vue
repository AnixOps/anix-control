<template>
  <div class="diag" data-testid="forward-diagnosis" :aria-busy="loading ? 'true' : 'false'">
    <p v-if="loading" class="diag__running" role="status">
      <UiSpinner :size="16" /> {{ t('forwardV4.diagnose.running') }}
    </p>
    <p v-else-if="error" class="fwd-note is-danger" role="alert">{{ errorMessage }}</p>

    <template v-else-if="result">
      <div class="diag__summary" role="status">
        <UiBadge :tone="result.ok ? 'success' : 'danger'" :label="result.ok ? t('forwardV4.diagnose.ok') : t('forwardV4.diagnose.failed')" />
        <span class="fwd-muted">{{ t('forwardV4.diagnose.counts', counts) }}</span>
        <span v-if="duration !== null" class="fwd-muted">{{ t('forwardV4.diagnose.duration', { s: duration }) }}</span>
        <UiBadge v-if="result.cached" tone="neutral" :dot="false" :label="t('forwardV4.diagnose.cached')" />
      </div>

      <section v-if="(result.nodes || []).length" class="diag__section">
        <h3 class="diag__title">{{ t('forwardV4.diagnose.nodes') }}</h3>
        <ul class="diag__nodes">
          <li v-for="node in result.nodes" :key="node.node_ref" class="diag__node">
            <UiStatusDot :tone="node.node_vantage ? 'success' : (node.connected ? 'warning' : 'neutral')" :label="nodeName(node.node_ref)" />
            <span class="fwd-muted">{{ node.node_vantage ? t('forwardV4.diagnose.vantage') : (node.connected ? t('forwardV4.diagnose.noVantage') : t('forwardV4.diagnose.offline')) }}</span>
            <span v-if="node.note" class="fwd-muted">{{ node.note }}</span>
          </li>
        </ul>
      </section>

      <section v-for="(stage, stageIndex) in stages" :key="stage.key" class="diag__section">
        <h3 class="diag__title">{{ stageIndex + 1 }}. {{ t(`forwardV4.diagnose.stages.${stage.key}`) }}</h3>
        <p v-if="!stage.steps.length" class="fwd-muted">{{ t('forwardV4.diagnose.noSteps') }}</p>
        <ol v-else class="diag__steps">
          <li v-for="(step, index) in stage.steps" :key="index" class="diag__step" :data-status="probeStatus(step)">
            <UiBadge :tone="PROBE_TONES[probeStatus(step)]" :label="t(`forwardV4.diagnose.status.${probeStatus(step)}`)" />
            <span class="diag__step-main">
              <span class="diag__step-head">
                <span>{{ t(`forwardV4.diagnose.kinds.${step.kind || 'PROBE_KIND_UNSPECIFIED'}`) }}</span>
                <span v-if="step.node_ref" class="fwd-muted">{{ nodeName(step.node_ref) }}<template v-if="step.hop_index !== undefined || step.node_ref"> · {{ t('forwardV4.diagnose.hop', { n: num(step.hop_index) + 1 }) }}</template></span>
                <span v-if="step.target" class="fwd-mono">→ {{ step.target }}</span>
              </span>
              <span v-if="step.result?.message" class="fwd-muted diag__message">{{ step.result.message }}</span>
            </span>
            <span class="diag__step-end">
              <span v-if="step.result?.code" class="fwd-mono diag__code">{{ step.result.code }}</span>
              <span v-if="num(step.result?.rtt_us)" class="fwd-muted">{{ (num(step.result.rtt_us) / 1000).toFixed(1) }} ms</span>
            </span>
          </li>
        </ol>
      </section>
    </template>
  </div>
</template>

<script setup>
// The staged answer of POST /routes/{id}/diagnose (F3c): Control's own
// records, the probes the route's nodes ran, then Control's dials. Every
// step shows its status word (OK / FAILED / INCONCLUSIVE / SKIPPED) and
// its stable code.
import { computed } from 'vue'
import { UiBadge, UiSpinner, UiStatusDot } from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import { PROBE_TONES, diagnosisStages, num, probeStatus } from './routeModel'

const props = defineProps({
  result: { type: Object, default: null },
  loading: { type: Boolean, default: false },
  error: { type: Object, default: null },
  nodes: { type: Array, default: () => [] },
  errorMessageFor: { type: Function, default: error => error?.message || '' }
})
const { t } = useAppI18n()

const byRef = computed(() => new Map(props.nodes.map(node => [node.node_ref, node])))
function nodeName(ref) {
  return byRef.value.get(ref)?.name || ref
}

const stages = computed(() => diagnosisStages(props.result))
const errorMessage = computed(() => props.errorMessageFor(props.error))
const counts = computed(() => {
  const out = { ok: 0, failed: 0, inconclusive: 0, skipped: 0 }
  for (const step of props.result?.steps || []) {
    const status = probeStatus(step)
    if (status === 'PROBE_STATUS_OK') out.ok++
    else if (status === 'PROBE_STATUS_FAILED') out.failed++
    else if (status === 'PROBE_STATUS_INCONCLUSIVE') out.inconclusive++
    else out.skipped++
  }
  return out
})
const duration = computed(() => {
  const started = num(props.result?.started_at_unix_ms)
  const finished = num(props.result?.finished_at_unix_ms)
  return started && finished >= started ? ((finished - started) / 1000).toFixed(1) : null
})
</script>

<style scoped>
.diag {
  display: grid;
  gap: var(--space-4);
}

.diag__running {
  display: flex;
  gap: var(--space-2);
  align-items: center;
  margin: 0;
}

.diag__summary {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
}

.diag__section {
  display: grid;
  gap: var(--space-2);
}

.diag__title {
  margin: 0;
  font-size: var(--type-body-size);
  font-weight: var(--weight-semibold);
}

.diag__nodes,
.diag__steps {
  display: grid;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}

.diag__node {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
}

.diag__step {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  gap: var(--space-2) var(--space-3);
  align-items: start;
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--separator);
  border-radius: var(--radius-sm);
}

.diag__step[data-status='PROBE_STATUS_FAILED'] {
  box-shadow: inset 2px 0 0 var(--danger);
}

.diag__step-main,
.diag__step-end {
  display: grid;
  gap: 2px;
  min-width: 0;
}

.diag__step-end {
  justify-items: end;
}

.diag__step-head {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1) var(--space-2);
  align-items: baseline;
}

.diag__message {
  overflow-wrap: anywhere;
}

.diag__code {
  color: var(--label-2);
}

@media (max-width: 639.98px) {
  .diag__step {
    grid-template-columns: minmax(0, 1fr);
  }

  .diag__step-end {
    justify-items: start;
  }
}
</style>
