<template>
  <UiSection :title="title" :description="description" data-test="runtime-status">
    <template #actions>
      <UiButton size="sm" :icon="RotateCw" :loading="loading" data-test="runtime-status-refresh" @click="emit('refresh')">{{ t('runtime.shared.refreshStatus') }}</UiButton>
      <UiButton size="sm" :icon="Stethoscope" :loading="doctorRunning" data-test="runtime-doctor" @click="emit('doctor')">{{ t('runtime.shared.runDoctor') }}</UiButton>
    </template>

    <UiSkeleton v-if="showSkeleton" variant="card" :lines="4" />
    <UiErrorState v-else-if="error" compact heading-tag="h3" :title="t('forwardNodesPage.runtimePage.statusLoadFailed')" :error="error" @retry="emit('refresh')" />
    <div v-else-if="hasStatus" class="rt-status-grid">
      <slot />
    </div>
    <UiEmptyState v-else-if="!loading" compact heading-tag="h3" :icon="Activity" :title="t('forwardNodesPage.runtimePage.emptyStatus')" :description="emptyText" />

    <p v-if="summary" class="rt-summary" data-test="runtime-summary">{{ summary }}</p>

    <div v-if="warnings.length" class="rt-warnings" role="note">
      <h3 class="rt-heading">{{ t('runtime.shared.warnings') }}</h3>
      <ul>
        <li v-for="warning in warnings" :key="warning">{{ warning }}</li>
      </ul>
    </div>

    <div class="rt-block">
      <h3 class="rt-heading">{{ t('forwardNodesPage.runtimePage.commandsTitle') }}</h3>
      <div class="rt-commands">
        <div v-for="group in commandGroups" :key="group.key" class="rt-command-group">
          <div class="rt-command-head">
            <span class="rt-command-label">{{ group.label }}</span>
            <UiIconButton
              v-if="group.lines.length"
              size="sm"
              :icon="copiedKey === group.key ? Check : Copy"
              :label="copiedKey === group.key ? t('forwardNodesPage.runtimePage.copied') : t('forwardNodesPage.runtimePage.copyGroup', { label: group.label })"
              @click="copyGroup(group)"
            />
          </div>
          <pre class="rt-code" tabindex="0" :aria-label="group.label"><code>{{ group.lines.join('\n') || '—' }}</code></pre>
        </div>
      </div>
    </div>

    <div class="rt-block">
      <h3 class="rt-heading">{{ t('forwardNodesPage.runtimePage.doctorTitle') }}</h3>
      <pre class="rt-code rt-doctor" tabindex="0" :aria-label="t('runtime.shared.doctorOutput')" data-test="runtime-doctor-output">{{ doctorOutput || t('runtime.shared.doctorNotExecuted') }}</pre>
    </div>
  </UiSection>
</template>

<script setup>
// Probe section of the NodeX and local runtime pages (UI U7): refresh the
// status and run Doctor (same endpoints as before), the status as grouped
// lists (default slot), the summary and warnings, the troubleshooting
// commands (copyable) and the raw Doctor output.
import { computed, onBeforeUnmount, ref, toRef } from 'vue'
import { Activity, Check, Copy, RotateCw, Stethoscope } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'
import UiButton from '@/ui/UiButton.vue'
import UiEmptyState from '@/ui/UiEmptyState.vue'
import UiErrorState from '@/ui/UiErrorState.vue'
import UiIconButton from '@/ui/UiIconButton.vue'
import UiSection from '@/ui/UiSection.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import { copyText } from '@/ui/composables/useClipboard'
import { useDelayedLoading } from '@/ui/composables/useDelayedLoading'
import { useToast } from '@/ui/composables/useToast'

const props = defineProps({
  title: { type: String, required: true },
  description: { type: String, default: '' },
  loading: { type: Boolean, default: false },
  error: { type: String, default: '' },
  hasStatus: { type: Boolean, default: false },
  emptyText: { type: String, default: '' },
  summary: { type: String, default: '' },
  warnings: { type: Array, default: () => [] },
  // { powerShell: [], bash: [], upgrade: [], references: [] }
  commands: { type: Object, required: true },
  doctorRunning: { type: Boolean, default: false },
  doctorOutput: { type: String, default: '' }
})
const emit = defineEmits(['refresh', 'doctor'])

const { t } = useAppI18n()
const toast = useToast()
const showSkeleton = useDelayedLoading(toRef(() => props.loading))
const copiedKey = ref('')
let timer = null

const commandGroups = computed(() => [
  { key: 'powerShell', label: t('runtime.shared.powerShell'), lines: props.commands.powerShell || [] },
  { key: 'bash', label: t('runtime.shared.bash'), lines: props.commands.bash || [] },
  { key: 'upgrade', label: t('runtime.shared.bootstrapVerify'), lines: props.commands.upgrade || [] },
  { key: 'references', label: t('runtime.shared.references'), lines: props.commands.references || [] }
])

async function copyGroup(group) {
  const ok = await copyText(group.lines.join('\n'))
  if (!ok) {
    toast.error(t('ui.copy.failed'))
    return
  }
  copiedKey.value = group.key
  clearTimeout(timer)
  timer = setTimeout(() => { copiedKey.value = '' }, 2000)
}

onBeforeUnmount(() => clearTimeout(timer))
</script>

<style scoped>
.rt-status-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(260px, 100%), 1fr));
  gap: var(--space-4);
  align-items: start;
}

.rt-summary {
  margin: var(--space-4) 0 0;
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-md);
  background: var(--bg-grouped);
  color: var(--label-1);
  font-size: var(--type-callout-size);
}

.rt-warnings {
  margin-top: var(--space-4);
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-md);
  background: var(--warning-soft);
  color: var(--label-1);
  font-size: var(--type-callout-size);
}

.rt-warnings ul {
  margin: var(--space-2) 0 0;
  padding-left: var(--space-5);
}

.rt-block {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  margin-top: var(--space-6);
}

.rt-heading {
  margin: 0;
  color: var(--label-1);
  font-size: var(--type-body-size);
  font-weight: var(--weight-semibold);
}

.rt-commands {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--space-4);
}

.rt-command-group {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
}

.rt-command-head {
  display: flex;
  gap: var(--space-2);
  align-items: center;
  justify-content: space-between;
  min-height: 28px;
}

.rt-command-label {
  color: var(--label-2);
  font-size: var(--type-callout-size);
  font-weight: var(--weight-medium);
}

.rt-code {
  max-height: 260px;
  margin: 0;
  padding: var(--space-3) var(--space-4);
  overflow: auto;
  border-radius: var(--radius-md);
  background: var(--bg-grouped);
  color: var(--label-1);
  font-family: var(--font-mono);
  font-size: var(--type-caption-size);
  line-height: 1.6;
  white-space: pre;
}

.rt-code:focus-visible {
  outline: var(--focus-ring);
  outline-offset: var(--focus-ring-offset);
}

.rt-doctor {
  max-height: 360px;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

@media (max-width: 833.98px) {
  .rt-commands {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
