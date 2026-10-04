<template>
  <ol class="hop-chain" :class="{ 'is-compact': compact }" :aria-label="ariaLabel">
    <template v-for="(hop, index) in hops" :key="index">
      <li v-if="index > 0" class="hop-chain__link" aria-hidden="true">
        <ArrowRight :size="12" :stroke-width="1.75" />
        <span v-if="!compact || linkLabel(hop) !== 'RAW'" class="hop-chain__security">{{ linkLabel(hop) }}</span>
      </li>
      <li class="hop-chain__hop" aria-hidden="true">
        <EngineChip :engine="hop.engine" :suffix="(hop.node_refs || []).length > 1 ? `×${hop.node_refs.length}` : ''" />
        <span v-if="!compact" class="hop-chain__nodes">{{ (hop.node_refs || []).map(nodeName).join(' / ') }}</span>
      </li>
    </template>
    <template v-if="!compact">
      <li class="hop-chain__link" aria-hidden="true"><ArrowRight :size="12" :stroke-width="1.75" /></li>
      <li class="hop-chain__targets" aria-hidden="true">{{ t('forwardV4.chain.targets', { n: targetCount }) }}</li>
    </template>
  </ol>
</template>

<script setup>
// The hop chain in one line: engine chips joined by the link security of
// the next hop's ingress (RAW is left out in the compact form). Screen
// readers get one sentence instead of the chips.
import { computed } from 'vue'
import { ArrowRight } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'
import EngineChip from './EngineChip.vue'
import { ENGINES, linkLabel } from './routeModel'

const props = defineProps({
  route: { type: Object, required: true },
  compact: { type: Boolean, default: false },
  nodeName: { type: Function, default: ref => ref }
})
const { t } = useAppI18n()
const hops = computed(() => props.route?.hops || [])
const targetCount = computed(() => (props.route?.targets || []).length)

const ariaLabel = computed(() => hops.value.map((hop, index) => {
  const engine = ENGINES[hop.engine]?.label || hop.engine
  const nodes = (hop.node_refs || []).map(props.nodeName).join(', ')
  return index > 0
    ? t('forwardV4.chain.hopVia', { link: linkLabel(hop), engine, nodes })
    : t('forwardV4.chain.hop', { engine, nodes })
}).join('; ') + `; ${t('forwardV4.chain.targets', { n: targetCount.value })}`)
</script>

<style scoped>
.hop-chain {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1);
  align-items: center;
  margin: 0;
  padding: 0;
  list-style: none;
}

.hop-chain__hop {
  display: inline-flex;
  gap: var(--space-1);
  align-items: center;
}

.hop-chain__nodes,
.hop-chain__targets {
  color: var(--label-2);
  font-size: var(--type-caption-size);
  white-space: nowrap;
}

.hop-chain__link {
  display: inline-flex;
  gap: 2px;
  align-items: center;
  color: var(--label-3);
}

.hop-chain__security {
  color: var(--label-2);
  font-family: var(--font-mono);
  font-size: var(--type-caption-size);
}
</style>
