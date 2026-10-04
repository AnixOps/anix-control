<template>
  <UiSection
    class="agent-upgrade"
    :title="t('agentUpgrades.title')"
    :description="t('agentUpgrades.description')"
    data-testid="agent-upgrade"
    :aria-busy="loading ? 'true' : 'false'"
  >
    <template v-if="campaign && active" #actions>
      <UiButton
        v-if="campaign.status === 'running'"
        :icon="Pause"
        variant="secondary"
        data-testid="agent-upgrade-pause"
        :disabled="busy"
        @click="act('pause')"
      >
        {{ t('agentUpgrades.actions.pause') }}
      </UiButton>
      <UiButton
        v-if="campaign.status === 'paused'"
        :icon="Play"
        variant="secondary"
        data-testid="agent-upgrade-resume"
        :disabled="busy"
        @click="act('resume')"
      >
        {{ t('agentUpgrades.actions.resume') }}
      </UiButton>
      <UiButton
        v-if="campaign.status !== 'rolling_back'"
        :icon="Undo2"
        variant="secondary"
        data-testid="agent-upgrade-rollback"
        :disabled="busy"
        @click="abort(true)"
      >
        {{ t('agentUpgrades.actions.rollback') }}
      </UiButton>
      <UiButton :icon="Square" variant="danger" data-testid="agent-upgrade-abort" :disabled="busy" @click="abort(false)">
        {{ t('agentUpgrades.actions.abort') }}
      </UiButton>
    </template>

    <p v-if="error" class="agent-upgrade__error" role="alert">{{ error }}</p>
    <div v-if="!campaign && !loading" class="agent-upgrade__empty" data-testid="agent-upgrade-empty">
      <p>{{ t('agentUpgrades.empty') }}</p>
      <code>anix-control agent upgrade start</code>
    </div>

    <div v-if="campaign" class="agent-upgrade__campaign" data-testid="agent-upgrade-campaign">
      <p class="agent-upgrade__summary">
        <UiBadge :tone="statusTone(campaign.status)" :label="statusLabel(campaign.status)" />
        <strong>{{ t('agentUpgrades.target', { version: campaign.target_version }) }}</strong>
        <span v-if="active">{{ t('agentUpgrades.batchOf', { batch: campaign.current_batch + 1, total: batches.length }) }}</span>
        <span v-if="active && campaign.batch_ends_at" class="agent-upgrade__muted">
          {{ t('agentUpgrades.batchEnds', { time: format.dateTime(campaign.batch_ends_at) }) }}
        </span>
        <span v-if="campaign.finished_at" class="agent-upgrade__muted">
          {{ t('agentUpgrades.finished', { time: format.dateTime(campaign.finished_at) }) }}
        </span>
      </p>
      <p v-if="campaign.status_reason" class="agent-upgrade__reason" data-testid="agent-upgrade-reason">
        <code v-if="campaign.error_code">{{ campaign.error_code }}</code>
        {{ campaign.status_reason }}
      </p>
      <ol class="agent-upgrade__batches">
        <li
          v-for="batch in batches"
          :key="batch.index"
          class="agent-upgrade__batch"
          :data-current="active && batch.index === campaign.current_batch ? 'true' : 'false'"
          :data-testid="`agent-upgrade-batch-${batch.index}`"
        >
          <span class="agent-upgrade__batch-name">
            {{ t('agentUpgrades.batch', { batch: batch.index + 1, percent: batch.percent }) }}
          </span>
          <UiUsageBar
            :value="done(batch)"
            :max="batch.nodes"
            :text="t('agentUpgrades.progress', { done: done(batch), total: batch.nodes })"
            :warn-at="101"
            :danger-at="101"
          />
          <span class="agent-upgrade__states">
            <UiBadge
              v-for="state in statesOf(batch)"
              :key="state"
              :tone="stateTone(state)"
              :dot="false"
              :label="`${stateLabel(state)} ${batch.states[state]}`"
            />
          </span>
        </li>
      </ol>
    </div>
  </UiSection>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { Pause, Play, Square, Undo2 } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiSection from '@/ui/UiSection.vue'
import UiUsageBar from '@/ui/UiUsageBar.vue'
import { useConfirm } from '@/ui/composables/useConfirm'
import { useFormat } from '@/ui/composables/useFormat'
import {
  abortKernelAgentUpgrade,
  listKernelAgentUpgrades,
  pauseKernelAgentUpgrade,
  resumeKernelAgentUpgrade
} from '@/api/kernel'

const STATUSES = ['running', 'paused', 'rolling_back', 'succeeded', 'rolled_back', 'aborted']
const STATES = ['pending', 'offered', 'upgrading', 'succeeded', 'failed', 'rolled_back', 'skipped']
const ACTIVE = ['running', 'paused', 'rolling_back']

const { t } = useAppI18n()
const format = useFormat()
const confirm = useConfirm()

const campaign = ref(null)
const loading = ref(false)
const busy = ref(false)
const error = ref('')

const active = computed(() => ACTIVE.includes(campaign.value?.status))
const batches = computed(() => (Array.isArray(campaign.value?.batches) ? campaign.value.batches : []).map(batch => ({
  ...batch,
  states: batch.states || {}
})))

function done(batch) {
  return ['succeeded', 'failed', 'rolled_back', 'skipped'].reduce((sum, state) => sum + (batch.states[state] || 0), 0)
}

function statesOf(batch) {
  return STATES.filter(state => batch.states[state] > 0)
}

function statusLabel(status) {
  return STATUSES.includes(status) ? t(`agentUpgrades.statuses.${status}`) : (status || '—')
}

function statusTone(status) {
  if (status === 'succeeded') return 'success'
  if (status === 'running') return 'info'
  if (status === 'rolled_back' || status === 'rolling_back') return 'danger'
  if (status === 'paused' || status === 'aborted') return 'warning'
  return 'neutral'
}

function stateLabel(state) {
  return STATES.includes(state) ? t(`agentUpgrades.states.${state}`) : state
}

function stateTone(state) {
  if (state === 'succeeded') return 'success'
  if (state === 'failed' || state === 'rolled_back') return 'danger'
  if (state === 'offered' || state === 'upgrading') return 'info'
  if (state === 'skipped') return 'warning'
  return 'neutral'
}

function errorMessage(cause) {
  const response = cause?.response?.data
  return response?.error?.message || (typeof response?.error === 'string' ? response.error : '') || cause?.message || t('agentUpgrades.loadFailed')
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const answer = await listKernelAgentUpgrades(1)
    campaign.value = Array.isArray(answer?.campaigns) && answer.campaigns.length ? answer.campaigns[0] : null
  } catch (cause) {
    error.value = errorMessage(cause)
  } finally {
    loading.value = false
  }
}

async function act(action) {
  busy.value = true
  error.value = ''
  try {
    campaign.value = action === 'pause'
      ? await pauseKernelAgentUpgrade(campaign.value.id)
      : await resumeKernelAgentUpgrade(campaign.value.id)
  } catch (cause) {
    error.value = errorMessage(cause)
  } finally {
    busy.value = false
  }
}

async function abort(rollback) {
  const id = campaign.value.id
  await confirm({
    title: t(rollback ? 'agentUpgrades.confirm.rollbackTitle' : 'agentUpgrades.confirm.abortTitle', { version: campaign.value.target_version }),
    message: t(rollback ? 'agentUpgrades.confirm.rollback' : 'agentUpgrades.confirm.abort'),
    confirmLabel: t(rollback ? 'agentUpgrades.actions.rollback' : 'agentUpgrades.actions.abort'),
    tone: 'danger',
    onConfirm: async () => {
      campaign.value = await abortKernelAgentUpgrade(id, rollback)
    }
  })
}

onMounted(() => load())

defineExpose({ load })
</script>

<style scoped>
.agent-upgrade__error {
  margin: 0;
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-sm);
  background: var(--danger-soft);
  color: color-mix(in srgb, var(--danger) 78%, var(--label-1));
  font-size: var(--type-callout-size);
}

.agent-upgrade__empty,
.agent-upgrade__campaign {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-sm);
  background: var(--fill-1);
  color: var(--label-1);
  font-size: var(--type-callout-size);
}

.agent-upgrade__empty p,
.agent-upgrade__summary,
.agent-upgrade__reason {
  margin: 0;
}

.agent-upgrade__empty code,
.agent-upgrade__reason code {
  overflow-wrap: anywhere;
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

.agent-upgrade__summary {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1) var(--space-2);
  align-items: center;
}

.agent-upgrade__muted {
  color: var(--label-2);
}

.agent-upgrade__batches {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}

.agent-upgrade__batch {
  display: grid;
  grid-template-columns: minmax(96px, 160px) minmax(96px, 1fr);
  gap: var(--space-1) var(--space-3);
  align-items: center;
}

.agent-upgrade__batch[data-current='true'] .agent-upgrade__batch-name {
  font-weight: 600;
}

.agent-upgrade__states {
  display: inline-flex;
  flex-wrap: wrap;
  grid-column: 1 / -1;
  gap: var(--space-1);
}
</style>
