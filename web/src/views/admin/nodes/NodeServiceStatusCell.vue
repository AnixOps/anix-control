<template>
  <span
    v-if="state === 'none'"
    class="node-service-muted"
    :title="emptyHint || undefined"
    data-testid="node-service-status-empty"
    :data-service-status="status === 'ready' ? 'none' : status"
  >
    <span aria-hidden="true">—</span><span v-if="emptyHint" class="visually-hidden">{{ emptyHint }}</span>
  </span>
  <UiBadge
    v-else
    :tone="tone"
    :dot="false"
    :label="label"
    :title="hint"
    data-testid="node-service-status"
    :data-service-status="state"
  />
</template>

<script setup>
// The 服务状态 cell of the node list: the node's systemd services as the
// machine-telemetry package summarizes them (useNodeServiceStatus). A word,
// never only a colour; a dash when there is nothing to show.
import { computed } from 'vue'
import UiBadge from '@/ui/UiBadge.vue'
import { useFormat } from '@/ui/composables/useFormat'
import { useAppI18n } from '@/composables/useAppI18n'
import { serviceStatusState } from './useNodeServiceStatus'

const props = defineProps({
  entry: { type: Object, default: null },
  status: { type: String, default: 'idle' }
})
const { t } = useAppI18n()
const format = useFormat()

const state = computed(() => serviceStatusState(props.entry))
const counts = computed(() => {
  const summary = props.entry?.summary || {}
  return { total: Number(summary.total || 0), active: Number(summary.active || 0), failed: Number(summary.failed || 0) }
})

const TONES = { waiting: 'neutral', unsupported: 'neutral', stale: 'warning', failed: 'danger', partial: 'warning', empty: 'neutral', healthy: 'success' }
const tone = computed(() => TONES[state.value] || 'neutral')
const label = computed(() => t(`admin.nodes.serviceStatus.states.${state.value}`, counts.value))

const hint = computed(() => {
  const entry = props.entry
  if (state.value === 'unsupported') {
    return entry?.unsupported_reason ? t('admin.nodes.services.unsupportedReason', { reason: entry.unsupported_reason }) : t('admin.nodes.services.unsupportedNoReason')
  }
  if (state.value === 'waiting') return t('admin.nodes.services.waiting')
  const parts = [t('admin.nodes.serviceStatus.counts', counts.value)]
  if (entry?.observed_at) parts.push(t('admin.nodes.services.observedAt', { time: format.relativeTime(entry.observed_at) }))
  return parts.join(' · ')
})

// Why the cell is a dash: the column could not load, or the node does not
// collect its services.
const emptyHint = computed(() => {
  if (props.status === 'loading') return t('admin.nodes.serviceStatus.loading')
  if (props.status === 'unavailable') return t('admin.nodes.serviceStatus.unavailable')
  if (props.status === 'ready') return t('admin.nodes.serviceStatus.notEnabled')
  return ''
})
</script>

<style scoped>
.node-service-muted {
  color: var(--label-2);
}
</style>
