<template>
  <UiSheet
    v-model:open="open"
    size="lg"
    :title="t('agentUpgradeSheet.title')"
    :description="t('agentUpgradeSheet.description')"
    data-testid="agent-upgrade-sheet"
  >
    <div class="upgrade-sheet">
      <UiSection :title="t('agentUpgradeSheet.start.title')" :aria-busy="loading ? 'true' : 'false'" heading-tag="h3">
        <UiSkeleton v-if="loading && !plan" variant="card" :label="t('agentUpgradeSheet.loading')" />
        <UiErrorState v-else-if="loadError && !plan" :title="t('agentUpgradeSheet.loadFailed')" :error="loadError" @retry="loadPlan" />

        <form v-else-if="plan" class="upgrade-sheet__form" novalidate data-testid="agent-upgrade-form" @submit.prevent="start">
          <p v-if="!plan.configured" class="upgrade-sheet__notice is-warning" role="status" data-testid="agent-upgrade-unconfigured">
            {{ t('agentUpgradeSheet.start.unconfigured', { reason: plan.unconfigured_reason }) }}
          </p>
          <p v-else-if="plan.active_campaign_id" class="upgrade-sheet__notice" role="status" data-testid="agent-upgrade-active">
            {{ t('agentUpgradeSheet.start.active') }}
          </p>
          <p v-else-if="!plan.can_start" class="upgrade-sheet__notice" role="status" data-testid="agent-upgrade-super-only">
            {{ t('agentUpgradeSheet.start.superOnly') }}
          </p>

          <UiTextField
            v-model.trim="form.target"
            size="md"
            :label="t('agentUpgradeSheet.start.target')"
            :help="t('agentUpgradeSheet.start.targetHelp', { control: plan.control_version })"
            :error="targetError"
            :disabled="disabled"
            autocomplete="off"
            data-testid="agent-upgrade-target"
          />

          <UiTextField
            v-model="form.batches"
            size="md"
            :label="t('agentUpgradeSheet.start.batches')"
            :help="t('agentUpgradeSheet.start.batchesHelp', { first: plan.first_batch_max_percent })"
            :error="batchesError"
            :disabled="disabled"
            autocomplete="off"
            inputmode="numeric"
            data-testid="agent-upgrade-batches"
          />

          <dl class="upgrade-sheet__facts">
            <div>
              <dt>{{ t('agentUpgradeSheet.start.minimum') }}</dt>
              <dd data-testid="agent-upgrade-minimum">{{ t('agentUpgradeSheet.start.minimumValue', { minutes: minutes(plan.min_batch_seconds) }) }}</dd>
            </div>
            <div>
              <dt>{{ t('agentUpgradeSheet.start.eligible') }}</dt>
              <dd data-testid="agent-upgrade-eligible">{{ t('agentUpgradeSheet.start.eligibleValue', { n: plan.eligible_nodes }, plan.eligible_nodes) }}</dd>
            </div>
          </dl>
          <p class="upgrade-sheet__rules">
            {{ t('agentUpgradeSheet.start.rules', { percent: plan.failure_threshold_percent, minutes: minutes(plan.reconnect_timeout_seconds) }) }}
          </p>

          <UiTextField
            v-model.trim="form.reason"
            size="md"
            :label="t('agentUpgradeSheet.start.reason')"
            :placeholder="t('agentUpgradeSheet.start.reasonPlaceholder')"
            :disabled="disabled"
            maxlength="512"
            data-testid="agent-upgrade-reason-field"
          />

          <p v-if="startError" class="upgrade-sheet__notice is-danger" role="alert" data-testid="agent-upgrade-start-error">{{ startError }}</p>

          <div class="upgrade-sheet__actions">
            <UiButton
              variant="primary"
              type="submit"
              :icon="Rocket"
              :loading="starting"
              :disabled="!canSubmit"
              data-testid="agent-upgrade-start"
            >{{ t('agentUpgradeSheet.start.submit') }}</UiButton>
            <span v-if="plan.configured && !plan.active_campaign_id && plan.can_start && plan.eligible_nodes === 0" class="upgrade-sheet__hint" data-testid="agent-upgrade-none">
              {{ t('agentUpgradeSheet.start.noneEligible') }}
            </span>
          </div>
        </form>
      </UiSection>

      <AgentUpgradeCampaign ref="campaign" :show-command="false" />
    </div>
  </UiSheet>
</template>

<script setup>
// 升级节点: the start form of a staged Agent upgrade (O4, H19) above the
// campaign card. The release defaults to the Agent version this Control
// pins (GET /api/v4/kernel/agents/upgrade-plan), the batches to 5%, 25%
// and 100% with the 30 minute minimum shown, not editable. Starting asks
// for a confirmation that names how many nodes are included, then calls
// POST /api/v4/kernel/agents/upgrades (super administrators). Opened with
// openAgentUpgrade() (composables/useAgentUpgradeSheet.js); the admin
// shell mounts it once.
import { computed, reactive, ref, watch } from 'vue'
import { Rocket } from '@lucide/vue'
import { getKernelAgentUpgradePlan, startKernelAgentUpgrade } from '@/api/kernel'
import { useAppI18n } from '@/composables/useAppI18n'
import { useAgentUpgradeSheet } from '@/composables/useAgentUpgradeSheet'
import AgentUpgradeCampaign from '@/components/admin/AgentUpgradeCampaign.vue'
import UiButton from '@/ui/UiButton.vue'
import UiErrorState from '@/ui/UiErrorState.vue'
import UiSection from '@/ui/UiSection.vue'
import UiSheet from '@/ui/UiSheet.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import UiTextField from '@/ui/UiTextField.vue'
import { useConfirm } from '@/ui/composables/useConfirm'
import { useToast } from '@/ui/composables/useToast'

const RELEASE_TAG = /^v\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/
const MAX_BATCHES = 10

const { t, te } = useAppI18n()
const confirm = useConfirm()
const toast = useToast()
const { open } = useAgentUpgradeSheet()

const plan = ref(null)
const loading = ref(false)
const loadError = ref(null)
const starting = ref(false)
const startError = ref('')
const campaign = ref(null)
const form = reactive({ target: '', batches: '', reason: '' })

const disabled = computed(() => !plan.value || !plan.value.configured || Boolean(plan.value.active_campaign_id) || !plan.value.can_start || starting.value)

// "5, 25, 100" → [5, 25, 100]; null when it is not a list of integers.
function parseBatches(text) {
  const parts = String(text || '').split(/[\s,，]+/).filter(Boolean)
  if (!parts.length) return null
  const values = parts.map(part => (/^\d{1,3}$/.test(part) ? Number(part) : NaN))
  return values.every(Number.isInteger) ? values : null
}

const batches = computed(() => parseBatches(form.batches))
const targetError = computed(() => (form.target && !RELEASE_TAG.test(form.target) ? t('agentUpgradeSheet.errors.target') : ''))
const batchesError = computed(() => {
  const values = batches.value
  if (!plan.value) return ''
  if (!values) return t('agentUpgradeSheet.errors.batchesFormat')
  if (values.length > MAX_BATCHES) return t('agentUpgradeSheet.errors.batchesCount', { max: MAX_BATCHES })
  if (values.some((value, index) => value <= (values[index - 1] || 0) || value > 100)) return t('agentUpgradeSheet.errors.batchesOrder')
  if (values[0] > plan.value.first_batch_max_percent) return t('agentUpgradeSheet.errors.batchesFirst', { max: plan.value.first_batch_max_percent })
  if (values[values.length - 1] !== 100) return t('agentUpgradeSheet.errors.batchesLast')
  return ''
})
const canSubmit = computed(() => Boolean(plan.value) && !disabled.value && Boolean(form.target) && !targetError.value && !batchesError.value &&
  plan.value.eligible_nodes > 0)

function minutes(seconds) {
  return Math.round(Number(seconds || 0) / 60)
}

function errorText(cause) {
  const response = cause?.response?.data
  const code = response?.error?.code || ''
  const message = response?.error?.message || (typeof response?.error === 'string' ? response.error : '') || cause?.message || ''
  const key = `agentUpgradeSheet.errors.${code}`
  if (code && te(key)) return t(key, { message })
  return message || t('agentUpgradeSheet.errors.unknown')
}

async function loadPlan() {
  loading.value = true
  loadError.value = null
  try {
    const answer = await getKernelAgentUpgradePlan()
    plan.value = answer
    if (!form.target) form.target = answer?.target_version || ''
    if (!form.batches) form.batches = (Array.isArray(answer?.batches) ? answer.batches : []).map(batch => batch.percent).join(', ')
  } catch (cause) {
    loadError.value = cause
  } finally {
    loading.value = false
  }
}

async function start() {
  if (!canSubmit.value || starting.value) return
  startError.value = ''
  const percents = batches.value
  const target = form.target
  const count = plan.value.eligible_nodes
  const confirmed = await confirm({
    title: t('agentUpgradeSheet.confirm.title', { n: count, version: target }, count),
    message: t('agentUpgradeSheet.confirm.message', {
      batches: percents.map(percent => `${percent}%`).join(' → '),
      minutes: minutes(plan.value.min_batch_seconds)
    }),
    confirmLabel: t('agentUpgradeSheet.start.submit'),
    tone: 'danger'
  })
  if (!confirmed) return
  starting.value = true
  try {
    const input = {
      target_version: target,
      batches: percents.map(percent => ({ percent, min_duration_seconds: plan.value.min_batch_seconds })),
    }
    if (form.reason) input.reason = form.reason
    await startKernelAgentUpgrade(input)
    toast.success(t('agentUpgradeSheet.started', { version: target }))
    form.reason = ''
    await Promise.all([loadPlan(), campaign.value?.load?.()])
  } catch (cause) {
    startError.value = errorText(cause)
    // A campaign that started meanwhile, or nodes that left, change the plan.
    await loadPlan()
  } finally {
    starting.value = false
  }
}

// The plan is read each time the sheet opens: nodes come and go, and a
// campaign may have started elsewhere.
watch(open, (value) => {
  if (!value) return
  startError.value = ''
  loadPlan()
  campaign.value?.load?.()
}, { immediate: true })

defineExpose({ loadPlan, plan, form })
</script>

<style scoped>
.upgrade-sheet {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
  min-width: 0;
}

.upgrade-sheet__form {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.upgrade-sheet__facts {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: var(--space-3);
  margin: 0;
}

.upgrade-sheet__facts div {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-sm);
  background: var(--fill-1);
}

.upgrade-sheet__facts dt {
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

.upgrade-sheet__facts dd {
  margin: 0;
  color: var(--label-1);
  font-size: var(--type-callout-size);
  font-weight: var(--weight-semibold);
}

.upgrade-sheet__rules,
.upgrade-sheet__hint {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.upgrade-sheet__notice {
  margin: 0;
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-sm);
  background: var(--fill-1);
  color: var(--label-1);
  font-size: var(--type-callout-size);
  overflow-wrap: anywhere;
}

.upgrade-sheet__notice.is-warning {
  background: var(--warning-soft);
}

.upgrade-sheet__notice.is-danger {
  background: var(--danger-soft);
  color: color-mix(in srgb, var(--danger) 78%, var(--label-1));
}

.upgrade-sheet__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3);
  align-items: center;
}
</style>
