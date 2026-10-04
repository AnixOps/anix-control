<template>
  <UiCard :title="t('forwardV4.preview.title')" heading-tag="h2" data-testid="forward-preview">
    <template #actions>
      <UiIconButton :icon="RefreshCw" :label="t('forwardV4.preview.refresh')" size="sm" :disabled="loading" @click="emit('refresh')" />
    </template>
    <div class="preview" :aria-busy="loading ? 'true' : 'false'">
      <p class="preview__meta" role="status">
        <UiBadge v-if="loading" tone="info" :label="t('forwardV4.preview.running')" />
        <UiBadge v-else-if="skipped" tone="neutral" :label="t('forwardV4.preview.waiting')" />
        <UiBadge v-else-if="error" tone="danger" :label="t('forwardV4.preview.failed')" />
        <UiBadge v-else-if="violations.length" tone="danger" :label="t('forwardV4.preview.problems', { n: violations.length })" />
        <UiBadge v-else-if="result" tone="success" :label="t('forwardV4.preview.ok')" />
        <span class="fwd-muted">{{ t('forwardV4.preview.cadence') }}</span>
      </p>

      <p v-if="skipped" class="fwd-muted">{{ t('forwardV4.preview.skippedHelp') }}</p>
      <p v-else-if="error" class="fwd-note is-danger">{{ errorMessage }}</p>

      <template v-else-if="violations.length">
        <p class="fwd-muted">{{ t('forwardV4.preview.refusedHelp') }}</p>
        <ul class="preview__violations">
          <li v-for="(item, index) in violations" :key="index">
            <span class="fwd-mono">{{ item.code }}</span>
            <span class="fwd-muted">{{ item.message }}</span>
          </li>
        </ul>
      </template>

      <template v-else-if="result">
        <dl class="preview__totals">
          <div><dt>{{ t('forwardV4.preview.nodes') }}</dt><dd>{{ cards.nodeCount }}</dd></div>
          <div><dt>{{ t('forwardV4.preview.ports') }}</dt><dd>{{ result.allocations.length }}</dd></div>
          <div><dt>{{ t('forwardV4.preview.generations') }}</dt><dd>{{ t('forwardV4.preview.generationsValue', { n: cards.nodeCount }) }}</dd></div>
        </dl>
        <ol class="preview__states">
          <li v-for="card in cards.items" :key="card.key" class="preview__state">
            <div class="preview__state-head">
              <span class="preview__node">{{ nodeName(card.nodeRef) }}</span>
              <span class="fwd-muted">{{ t('forwardV4.preview.hopRole', { n: card.hopIndex + 1, role: t(`forwardV4.role.${card.role}`) }) }}</span>
              <EngineChip :engine="card.engine" />
            </div>
            <dl class="preview__fields">
              <dt>{{ t('forwardV4.preview.listen') }}</dt>
              <dd class="fwd-mono">{{ card.listen }}<template v-if="card.hopIndex > 0"> · {{ card.ingress }}</template></dd>
              <dt>{{ t('forwardV4.preview.upstreams') }}</dt>
              <dd>
                <span v-for="(up, upIndex) in card.upstreams" :key="upIndex" class="preview__up fwd-mono">{{ up }}</span>
              </dd>
              <dt>{{ t('forwardV4.preview.markGeneration') }}</dt>
              <dd class="fwd-mono">{{ card.mark }} · {{ card.generation }}</dd>
            </dl>
          </li>
        </ol>
      </template>

      <div v-if="warnings.length" class="preview__warnings">
        <p v-for="warning in warnings" :key="warning" class="fwd-note is-warning">{{ warning }}</p>
      </div>

      <details class="preview__json">
        <summary>{{ t('forwardV4.preview.json') }}</summary>
        <UiCodeBlock :code="json" :label="t('forwardV4.preview.jsonLabel')" max-height="280px" />
      </details>
    </div>
  </UiCard>
</template>

<script setup>
// 规划预览: what POST /routes/preview answers for the draft (D4). With
// violations nothing is planned; otherwise one card per node and hop of
// this route, and the generation each node moves to.
import { computed } from 'vue'
import { RefreshCw } from '@lucide/vue'
import { UiBadge, UiCard, UiCodeBlock, UiIconButton } from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import { routeBody } from '@/api/forwardV4'
import EngineChip from './EngineChip.vue'
import { SECURITIES, num } from './routeModel'

const props = defineProps({
  result: { type: Object, default: null },
  loading: { type: Boolean, default: false },
  skipped: { type: Boolean, default: false },
  error: { type: Object, default: null },
  // The Route the preview was asked for (request JSON).
  route: { type: Object, default: null },
  routeId: { type: String, default: '' },
  nodes: { type: Array, default: () => [] },
  // Warnings the UI adds (LEAST_CONN on nftables, entry HA without a name).
  extraWarnings: { type: Array, default: () => [] },
  errorMessageFor: { type: Function, default: error => error?.message || '' }
})
const emit = defineEmits(['refresh'])
const { t } = useAppI18n()

const byRef = computed(() => new Map(props.nodes.map(node => [node.node_ref, node])))
function nodeName(ref) {
  return byRef.value.get(ref)?.name || ref
}

const violations = computed(() => props.result?.violations || [])
const warnings = computed(() => Array.from(new Set([...(props.result?.warnings || []), ...props.extraWarnings])))
const errorMessage = computed(() => props.errorMessageFor(props.error))

const cards = computed(() => {
  const states = props.result?.states || []
  const allocations = props.result?.allocations || []
  const routeIds = new Set(allocations.map(item => item.route_id || ''))
  if (props.routeId) routeIds.add(props.routeId)
  const items = []
  const nodeRefs = new Set()
  for (const state of states) {
    const current = num(byRef.value.get(state.node_ref)?.desired_generation)
    for (const hop of state.hops || []) {
      if (!routeIds.has(hop.route_id || '')) continue
      nodeRefs.add(state.node_ref)
      const hopIndex = num(hop.hop_index)
      items.push({
        key: `${state.node_ref}-${hopIndex}`,
        nodeRef: state.node_ref,
        hopIndex,
        role: hop.role || 'HOP_ROLE_ENTRY',
        engine: hop.engine,
        listen: `${hop.listen?.address || '0.0.0.0'}:${num(hop.listen?.port)}`,
        ingress: SECURITIES[hop.ingress?.security || 'LINK_SECURITY_RAW'],
        upstreams: (hop.upstreams || []).map(up => {
          const name = up.node_ref ? `${nodeName(up.node_ref)} :${num(up.port)}` : `${up.address}:${num(up.port)}`
          const link = up.egress?.security && up.egress.security !== 'LINK_SECURITY_RAW' ? ` · ${SECURITIES[up.egress.security] || up.egress.security}` : ''
          return `P${num(up.priority)} ${name}${link}`
        }),
        mark: num(hop.mark),
        generation: `${current} → ${num(state.generation)}`
      })
    }
  }
  items.sort((a, b) => a.hopIndex - b.hopIndex || a.nodeRef.localeCompare(b.nodeRef))
  return { items, nodeCount: nodeRefs.size }
})

const json = computed(() => JSON.stringify({ route: props.route ? routeBody(props.route) : {} }, null, 2))
</script>

<style scoped>
.preview {
  display: grid;
  gap: var(--space-3);
}

.preview__meta {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
  margin: 0;
}

.preview__violations,
.preview__states {
  display: grid;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}

.preview__violations li {
  display: grid;
  gap: 2px;
  padding: var(--space-2);
  border-left: 2px solid var(--danger);
  background: var(--fill-1);
  overflow-wrap: anywhere;
}

.preview__totals {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: var(--space-2);
  margin: 0;
}

.preview__totals div {
  padding: var(--space-2);
  border-radius: var(--radius-xs);
  background: var(--fill-1);
}

.preview__totals dt {
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

.preview__totals dd {
  margin: 0;
  font-weight: var(--weight-semibold);
}

.preview__state {
  display: grid;
  gap: var(--space-2);
  padding: var(--space-3);
  border: 1px solid var(--separator);
  border-radius: var(--radius-sm);
}

.preview__state-head {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
}

.preview__node {
  font-weight: var(--weight-semibold);
}

.preview__fields {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  gap: var(--space-1) var(--space-3);
  margin: 0;
}

.preview__fields dt {
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

.preview__fields dd {
  display: grid;
  gap: 2px;
  margin: 0;
  overflow-wrap: anywhere;
}

.preview__warnings {
  display: grid;
  gap: var(--space-2);
}

.preview__json summary {
  color: var(--accent);
  cursor: pointer;
}

.preview__json[open] summary {
  margin-bottom: var(--space-2);
}
</style>
