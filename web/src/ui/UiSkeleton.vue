<template>
  <div class="ui-skeleton" :class="`ui-skeleton--${variant}`" role="status">
    <span class="visually-hidden">{{ label || t('ui.loading') }}</span>
    <template v-if="variant === 'text'">
      <span
        v-for="line in lines"
        :key="line"
        class="ui-skeleton__bone ui-skeleton__line"
        :style="{ width: lineWidth(line) }"
        aria-hidden="true"
      />
    </template>
    <template v-else-if="variant === 'card'">
      <div class="ui-skeleton__card" aria-hidden="true">
        <span class="ui-skeleton__bone ui-skeleton__line" style="width: 40%" />
        <span class="ui-skeleton__bone ui-skeleton__value" />
        <span class="ui-skeleton__bone ui-skeleton__line" style="width: 75%" />
      </div>
    </template>
    <template v-else-if="variant === 'chart'">
      <div class="ui-skeleton__chart" :style="{ height: typeof height === 'number' ? `${height}px` : height }" aria-hidden="true">
        <span
          v-for="(bar, index) in CHART_BARS"
          :key="index"
          class="ui-skeleton__bone ui-skeleton__bar"
          :style="{ height: bar }"
        />
      </div>
    </template>
    <template v-else>
      <div
        v-for="row in rows"
        :key="row"
        class="ui-skeleton__row"
        :style="{ gridTemplateColumns: columns > 1 ? `2fr repeat(${columns - 1}, 1fr)` : '1fr' }"
        aria-hidden="true"
      >
        <span
          v-for="column in columns"
          :key="column"
          class="ui-skeleton__bone ui-skeleton__line"
          :style="{ width: column === 1 ? '70%' : lineWidth(row + column) }"
        />
      </div>
    </template>
  </div>
</template>

<script setup>
// Placeholder while content loads for more than 300 ms (gate it with
// useDelayedLoading). Variants: text (lines), card (metric card), table-row
// (rows × columns). role="status" with a hidden "加载中…" announces it once;
// the bones are hidden from assistive tech. The shimmer runs three times,
// then holds still; none under reduced motion.
import { useI18n } from 'vue-i18n'

defineProps({
  variant: { type: String, default: 'text', validator: value => ['text', 'card', 'table-row', 'chart'].includes(value) },
  lines: { type: Number, default: 3 },
  rows: { type: Number, default: 3 },
  columns: { type: Number, default: 4 },
  label: { type: String, default: '' },
  // chart: the plot's height (px number or a CSS length).
  height: { type: [Number, String], default: 240 }
})

const { t } = useI18n()
const WIDTHS = ['92%', '78%', '64%', '86%', '58%']
const CHART_BARS = ['38%', '52%', '46%', '64%', '58%', '72%', '66%', '80%', '62%', '70%', '54%', '60%']

function lineWidth(index) {
  return WIDTHS[index % WIDTHS.length]
}
</script>

<style scoped>
.ui-skeleton {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  width: 100%;
}

.ui-skeleton__bone {
  display: block;
  border-radius: var(--radius-xs);
  background: linear-gradient(90deg, var(--fill-1), var(--fill-2), var(--fill-1));
  background-size: 200% 100%;
  animation: ui-shimmer 1.4s var(--ease-standard) 3;
}

.ui-skeleton__line {
  height: 14px;
}

.ui-skeleton__value {
  width: 55%;
  height: 28px;
}

.ui-skeleton__card {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-5);
  border-radius: var(--radius-md);
  background: var(--bg-elevated);
  box-shadow: var(--shadow-1), 0 0 0 0.5px var(--separator);
}

.ui-skeleton__chart {
  display: flex;
  gap: var(--space-2);
  align-items: flex-end;
  padding: var(--space-4) 0 0;
  border-bottom: 1px solid var(--separator);
}

.ui-skeleton__bar {
  flex: 1;
  border-radius: var(--radius-xs) var(--radius-xs) 0 0;
}

.ui-skeleton--table-row {
  gap: 0;
}

.ui-skeleton__row {
  display: grid;
  gap: var(--space-4);
  align-items: center;
  height: 52px;
  padding: 0 var(--space-4);
  border-bottom: 1px solid var(--separator);
}

.ui-skeleton__row:last-child {
  border-bottom: 0;
}

@keyframes ui-shimmer {
  from {
    background-position: 100% 0;
  }

  to {
    background-position: -100% 0;
  }
}

@media (prefers-reduced-motion: reduce) {
  .ui-skeleton__bone {
    animation: none;
  }
}
</style>
