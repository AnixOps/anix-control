<template>
  <span class="engine-chip" :class="[`engine-chip--${meta.short}`, { 'is-off': unavailable }]" :title="title">
    <span class="engine-chip__dot" aria-hidden="true" />{{ meta.short }}<span v-if="suffix" class="engine-chip__suffix">{{ suffix }}</span>
  </span>
</template>

<script setup>
// An engine as a compact chip: a token-coloured dot and the short name.
// The name carries the meaning; the dot only helps scanning.
import { computed } from 'vue'
import { ENGINES } from '../mockData'

const props = defineProps({
  engine: { type: String, required: true },
  suffix: { type: String, default: '' },
  unavailable: { type: Boolean, default: false },
  title: { type: String, default: '' }
})
const meta = computed(() => ENGINES[props.engine] || { short: props.engine, label: props.engine })
</script>

<style scoped>
.engine-chip {
  display: inline-flex;
  gap: var(--space-1);
  align-items: center;
  height: 20px;
  padding: 0 var(--space-2);
  border-radius: var(--radius-pill);
  background: var(--fill-1);
  color: var(--label-1);
  font-family: var(--font-mono);
  font-size: var(--type-caption-size);
  line-height: 1;
  white-space: nowrap;
}

.engine-chip__dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--chart-1);
}

.engine-chip--gost .engine-chip__dot { background: var(--chart-3); }
.engine-chip--anixops .engine-chip__dot { background: var(--chart-5); }

.engine-chip__suffix {
  color: var(--label-2);
}

.engine-chip.is-off {
  color: var(--label-3);
  text-decoration: line-through;
}

.engine-chip.is-off .engine-chip__dot {
  background: var(--label-3);
}
</style>
