<template>
  <UiSkeleton v-if="showSkeleton" variant="card" :label="t('ui.chart.loading', { label })" class="ui-metric-card__skeleton" />
  <div v-else class="ui-metric-card" :class="{ 'has-sparkline': hasSparkline }" data-metric-card>
    <p class="ui-metric-card__label">
      <UiIcon v-if="icon" :icon="icon" :size="16" class="ui-metric-card__icon" />
      <span>{{ label }}</span>
    </p>
    <p class="ui-metric-card__value tabular-nums" data-metric-value>
      <span>{{ displayValue }}</span>
      <span v-if="unit" class="ui-metric-card__unit">{{ unit }}</span>
    </p>
    <p v-if="trend || detail || $slots.detail" class="ui-metric-card__foot">
      <span v-if="trend" class="ui-metric-card__trend" :class="`is-${trendTone}`" data-metric-trend>
        <UiIcon :icon="trendIcon" :size="14" />
        <span>{{ trend }}</span>
      </span>
      <span v-if="detail || $slots.detail" class="ui-metric-card__detail"><slot name="detail">{{ detail }}</slot></span>
    </p>
    <svg
      v-if="hasSparkline"
      class="ui-metric-card__sparkline"
      viewBox="0 0 100 32"
      preserveAspectRatio="none"
      aria-hidden="true"
      focusable="false"
      data-metric-sparkline
    >
      <path class="ui-metric-card__area" :d="sparkPaths.area" />
      <path class="ui-metric-card__line" :d="sparkPaths.line" />
    </svg>
  </div>
</template>

<script setup>
// Metric card (plan §6 Stat / MetricCard): a label, one big number, an
// optional trend ("+3 今日", with an arrow and a word, never colour alone)
// and a detail line, and an optional sparkline under it. The sparkline is a
// small inline SVG (no chart library) drawn with the accent token, so it
// follows the theme; it is decorative, the numbers carry the meaning.
// `loading` shows a card skeleton after 300 ms. Put the card in a link when
// it opens a page; the whole card is then the link text.
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Minus, TrendingDown, TrendingUp } from '@lucide/vue'
import UiIcon from './UiIcon.vue'
import UiSkeleton from './UiSkeleton.vue'
import { useDelayedLoading } from './composables/useDelayedLoading'

const props = defineProps({
  label: { type: String, required: true },
  value: { type: [String, Number], default: '' },
  unit: { type: String, default: '' },
  icon: { type: [Object, Function], default: null },
  detail: { type: String, default: '' },
  trend: { type: String, default: '' },
  // 'up' | 'down' | 'flat': the arrow.
  trendDirection: { type: String, default: 'flat', validator: value => ['up', 'down', 'flat'].includes(value) },
  // 'positive' | 'negative' | 'neutral': the colour (more tickets is bad news).
  trendTone: { type: String, default: 'neutral', validator: value => ['positive', 'negative', 'neutral'].includes(value) },
  sparkline: { type: Array, default: () => [] },
  loading: { type: Boolean, default: false }
})

const { t } = useI18n()
const showSkeleton = useDelayedLoading(() => props.loading)

const displayValue = computed(() => (props.value === '' || props.value === null || props.value === undefined ? '—' : props.value))
const trendIcon = computed(() => ({ up: TrendingUp, down: TrendingDown }[props.trendDirection] || Minus))
const points = computed(() => props.sparkline.map(Number).filter(Number.isFinite))
const hasSparkline = computed(() => points.value.length > 1)

// Paths in a 100 × 32 box (stretched to the card); 2 units of padding keep
// the stroke inside at the extremes.
const sparkPaths = computed(() => {
  const values = points.value
  if (values.length < 2) return { line: '', area: '' }
  const max = Math.max(...values)
  const min = Math.min(...values, 0)
  const span = max - min || 1
  const step = 100 / (values.length - 1)
  const coords = values.map((value, index) => [index * step, 30 - ((value - min) / span) * 28])
  const line = coords.map(([x, y], index) => `${index ? 'L' : 'M'}${x.toFixed(2)} ${y.toFixed(2)}`).join(' ')
  return { line, area: `${line} L100 32 L0 32 Z` }
})
</script>

<style scoped>
.ui-metric-card {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
  height: 100%;
  padding: var(--space-5);
  overflow: hidden;
  border-radius: var(--radius-md);
  background: var(--bg-elevated);
  box-shadow: var(--shadow-1), 0 0 0 0.5px var(--separator);
}

.ui-metric-card.has-sparkline {
  padding-bottom: calc(var(--space-5) + 36px);
}

.ui-metric-card__label {
  display: flex;
  gap: var(--space-2);
  align-items: center;
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
  font-weight: var(--weight-medium);
  line-height: var(--type-callout-line);
}

.ui-metric-card__icon {
  flex: none;
  color: var(--label-3);
}

.ui-metric-card__value {
  display: flex;
  gap: var(--space-1);
  align-items: baseline;
  margin: 0;
  color: var(--label-1);
  font-size: var(--type-title-1-size);
  font-weight: var(--weight-semibold);
  line-height: var(--type-title-1-line);
  overflow-wrap: anywhere;
}

.ui-metric-card__unit {
  color: var(--label-2);
  font-size: var(--type-body-size);
  font-weight: var(--weight-medium);
}

.ui-metric-card__foot {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1) var(--space-3);
  align-items: center;
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-caption-size);
  line-height: var(--type-caption-line);
}

.ui-metric-card__trend {
  display: inline-flex;
  gap: var(--space-1);
  align-items: center;
  font-weight: var(--weight-semibold);
}

.ui-metric-card__trend.is-positive {
  color: var(--success);
}

.ui-metric-card__trend.is-negative {
  color: var(--danger);
}

.ui-metric-card__sparkline {
  position: absolute;
  right: 0;
  bottom: 0;
  left: 0;
  width: 100%;
  height: 36px;
}

.ui-metric-card__line {
  fill: none;
  stroke: var(--accent);
  stroke-width: 1.5;
  stroke-linejoin: round;
  vector-effect: non-scaling-stroke;
}

.ui-metric-card__area {
  fill: var(--accent-soft);
  stroke: none;
}

.ui-metric-card__skeleton :deep(.ui-skeleton__card) {
  height: 100%;
}
</style>
