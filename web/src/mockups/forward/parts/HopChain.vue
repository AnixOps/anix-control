<template>
  <ol class="hop-chain" :class="{ 'is-compact': compact }" :aria-label="ariaLabel">
    <template v-for="(hop, index) in route.hops" :key="index">
      <li v-if="index > 0" class="hop-chain__link" aria-hidden="true">
        <ArrowRight :size="12" :stroke-width="1.75" />
        <span v-if="!compact || linkLabel(hop) !== 'RAW'" class="hop-chain__security">{{ linkLabel(hop) }}</span>
      </li>
      <li class="hop-chain__hop">
        <EngineChip :engine="hop.engine" :suffix="hop.node_refs.length > 1 ? `×${hop.node_refs.length}` : ''" />
        <span v-if="!compact" class="hop-chain__nodes">{{ hop.node_refs.map(nodeName).join(' / ') }}</span>
      </li>
    </template>
    <template v-if="!compact">
      <li class="hop-chain__link" aria-hidden="true"><ArrowRight :size="12" :stroke-width="1.75" /></li>
      <li class="hop-chain__targets">{{ route.targets.length }} 个目标</li>
    </template>
  </ol>
</template>

<script setup>
// The hop chain in one line: engine chips joined by the link security of
// the next hop's ingress (RAW is left out in the compact form).
import { computed } from 'vue'
import { ArrowRight } from '@lucide/vue'
import EngineChip from './EngineChip.vue'
import { SECURITIES, ENGINES, nodeName } from '../mockData'

const props = defineProps({
  route: { type: Object, required: true },
  compact: { type: Boolean, default: false }
})

function linkLabel(hop) {
  const security = SECURITIES[hop.ingress?.security || 'LINK_SECURITY_RAW']
  return hop.ingress?.mux ? `${security}·mux` : security
}

const ariaLabel = computed(() => props.route.hops.map((hop, index) => {
  const engine = ENGINES[hop.engine]?.label || hop.engine
  const link = index > 0 ? `经 ${linkLabel(hop)} ` : ''
  return `${link}${engine} ${hop.node_refs.map(nodeName).join('、')}`
}).join('，') + `，${props.route.targets.length} 个目标`)
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
