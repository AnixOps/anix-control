<template>
  <span class="node-reach">
    <UiBadge
      v-if="entry"
      :tone="REACH_TONES[entry.status] || 'neutral'"
      :dot="false"
      :label="t(`nodeReach.status.${entry.status}`)"
      :title="hint"
      data-testid="node-reach"
      :data-reach-status="entry.status"
    />
    <span v-else class="node-reach__muted" data-testid="node-reach-empty">—</span>
    <span v-if="vantage" class="node-reach__muted" data-testid="node-reach-vantage">{{ t(`nodeReach.vantage.${vantage.region}`) }}</span>
  </span>
</template>

<script setup>
// The 可达性 cell of the node list: whether the node answers from each
// region (useNodeReach), and whether it is a vantage itself.
import { computed } from 'vue'
import UiBadge from '@/ui/UiBadge.vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { REACH_TONES } from './useNodeReach'

const props = defineProps({
  entry: { type: Object, default: null },
  vantage: { type: Object, default: null }
})
const { t } = useAppI18n()

const view = region => {
  const reachable = props.entry?.[region]?.reachable
  return t(`nodeReach.view.${reachable === true ? 'yes' : reachable === false ? 'no' : 'none'}`, { region: t(`nodeReach.region.${region}`) })
}
const hint = computed(() => `${view('domestic')} · ${view('overseas')}`)
</script>

<style scoped>
.node-reach {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
}

.node-reach__muted {
  color: var(--label-2);
  font-size: var(--type-footnote-size);
}
</style>
