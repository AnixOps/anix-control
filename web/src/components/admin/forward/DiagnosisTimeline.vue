<template>
  <div class="diagnosis-timeline">
    <p v-if="summary" class="diagnosis-timeline__summary" data-test="diagnosis-summary">
      <UiBadge :tone="failedCount ? 'danger' : 'success'" :label="summary" />
      <span v-if="checkedAt" class="diagnosis-timeline__time">{{ checkedAt }}</span>
    </p>
    <ol class="diagnosis-timeline__list" :aria-label="label">
      <li
        v-for="(step, index) in steps"
        :key="step.key ?? index"
        class="diagnosis-step"
        :class="step.success ? 'is-ok' : 'is-fail'"
        data-test="diagnosis-step"
      >
        <span class="diagnosis-step__marker" aria-hidden="true">
          <UiIcon :icon="step.success ? Check : X" :size="14" />
        </span>
        <div class="diagnosis-step__body">
          <div class="diagnosis-step__head">
            <h3 class="diagnosis-step__title">{{ step.title }}</h3>
            <UiBadge :tone="step.success ? 'success' : 'danger'" :label="step.statusLabel" />
          </div>
          <p v-if="step.meta" class="diagnosis-step__meta">{{ step.meta }}</p>
          <dl v-if="step.fields?.length" class="diagnosis-step__fields">
            <div v-for="field in step.fields" :key="field.label" class="diagnosis-step__field">
              <dt>{{ field.label }}</dt>
              <dd :class="{ 'is-mono': field.mono, 'tabular-nums': field.numeric }">{{ field.value }}</dd>
            </div>
          </dl>
          <p v-if="step.message" class="diagnosis-step__message" :class="{ 'is-error': !step.success }">{{ step.message }}</p>
        </div>
      </li>
    </ol>
  </div>
</template>

<script setup>
// Diagnosis results of a forward rule or a tunnel as a timeline (UI U7,
// plan §8.2). Presentational only: each step is one item of the
// diagnose response's `results[]` (flux-panel `forward/diagnose`,
// `tunnel/diagnose`) that the page has already turned into text.
//   steps: [{ key, title, meta, success, statusLabel, fields: [{ label, value, mono, numeric }], message }]
import { computed } from 'vue'
import { Check, X } from '@lucide/vue'
import UiBadge from '@/ui/UiBadge.vue'
import UiIcon from '@/ui/UiIcon.vue'

const props = defineProps({
  steps: { type: Array, default: () => [] },
  label: { type: String, required: true },
  // "2 / 3 passed": built by the page, which knows the words.
  summary: { type: String, default: '' },
  checkedAt: { type: String, default: '' }
})

const failedCount = computed(() => props.steps.filter(step => !step.success).length)
</script>

<style scoped>
.diagnosis-timeline {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.diagnosis-timeline__summary {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2) var(--space-3);
  align-items: center;
  margin: 0;
}

.diagnosis-timeline__time {
  color: var(--label-2);
  font-size: var(--type-caption-size);
  font-variant-numeric: tabular-nums;
}

.diagnosis-timeline__list {
  display: flex;
  flex-direction: column;
  margin: 0;
  padding: 0;
  list-style: none;
}

.diagnosis-step {
  position: relative;
  display: grid;
  grid-template-columns: 24px minmax(0, 1fr);
  gap: var(--space-3);
  padding-bottom: var(--space-5);
}

.diagnosis-step:last-child {
  padding-bottom: 0;
}

/* The rail between two markers. */
.diagnosis-step:not(:last-child)::before {
  position: absolute;
  top: 28px;
  bottom: 4px;
  left: 11px;
  width: 2px;
  border-radius: var(--radius-pill);
  background: var(--separator);
  content: '';
}

.diagnosis-step__marker {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  margin-top: 1px;
  border-radius: 50%;
  color: var(--on-accent);
}

.diagnosis-step.is-ok .diagnosis-step__marker {
  background: var(--success);
}

.diagnosis-step.is-fail .diagnosis-step__marker {
  background: var(--danger);
}

.diagnosis-step__body {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
}

.diagnosis-step__head {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
  justify-content: space-between;
}

.diagnosis-step__title {
  margin: 0;
  color: var(--label-1);
  font-size: var(--type-body-size);
  font-weight: var(--weight-semibold);
  overflow-wrap: anywhere;
}

.diagnosis-step__meta {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-caption-size);
  overflow-wrap: anywhere;
}

.diagnosis-step__fields {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
  gap: var(--space-2) var(--space-4);
  margin: 0;
  padding: var(--space-3);
  border-radius: var(--radius-sm);
  background: var(--fill-1);
}

.diagnosis-step__field {
  display: flex;
  flex-direction: column;
  gap: var(--space-0-5);
  min-width: 0;
}

.diagnosis-step__field dt {
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

.diagnosis-step__field dd {
  margin: 0;
  color: var(--label-1);
  font-weight: var(--weight-medium);
  overflow-wrap: anywhere;
}

.diagnosis-step__field dd.is-mono {
  font-family: var(--font-mono);
  font-size: var(--type-caption-size);
  font-weight: var(--weight-regular);
}

.diagnosis-step__message {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
  overflow-wrap: anywhere;
}

.diagnosis-step__message.is-error {
  color: var(--danger);
}

@media (forced-colors: active) {
  .diagnosis-step__marker {
    border: 1px solid CanvasText;
  }
}
</style>
