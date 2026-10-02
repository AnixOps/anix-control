<template>
  <div class="ui-usage" :class="`is-${tone}`">
    <span class="ui-usage__text tabular-nums">{{ text }}</span>
    <span v-if="max > 0" class="ui-usage__track" aria-hidden="true">
      <span class="ui-usage__fill" :style="{ width: `${percent}%` }" />
    </span>
  </div>
</template>

<script setup>
// Used / total in a cell or a card: the text ("71.6 GB / 200 GB") carries
// the meaning; the thin bar under it is decorative and turns amber at 75 %
// and red at 90 % (or at `warnAt` / `dangerAt`). No bar without a total.
import { computed } from 'vue'

const props = defineProps({
  value: { type: Number, default: 0 },
  max: { type: Number, default: 0 },
  text: { type: String, required: true },
  warnAt: { type: Number, default: 75 },
  dangerAt: { type: Number, default: 90 }
})

const percent = computed(() => (props.max > 0 ? Math.min(100, Math.max(0, (props.value / props.max) * 100)) : 0))
const tone = computed(() => {
  if (props.max <= 0) return 'none'
  if (percent.value >= props.dangerAt) return 'danger'
  if (percent.value >= props.warnAt) return 'warning'
  return 'normal'
})
</script>

<style scoped>
.ui-usage {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  min-width: 96px;
}

.ui-usage__text {
  white-space: nowrap;
}

.ui-usage__track {
  display: block;
  height: 4px;
  overflow: hidden;
  border-radius: var(--radius-pill);
  background: var(--fill-2);
}

.ui-usage__fill {
  display: block;
  height: 100%;
  border-radius: inherit;
  background: var(--accent);
  transition: width var(--dur-toggle) var(--ease-standard);
}

.ui-usage.is-warning .ui-usage__fill {
  background: var(--warning);
}

.ui-usage.is-danger .ui-usage__fill {
  background: var(--danger);
}
</style>
