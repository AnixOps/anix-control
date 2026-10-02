<template>
  <div class="settings-section" data-settings-panel="balancer">
    <UiSection :title="t('adminSettings.balancer.title')" :description="t('adminSettings.balancer.description')">
      <template #actions>
        <UiButton :icon="Plus" data-test="balancer-create" @click="openBalancerModal()">{{ t('adminSettings.balancer.create') }}</UiButton>
      </template>
      <UiDataTable
        :columns="balancerColumns"
        :rows="balancers"
        :label="t('adminSettings.balancer.title')"
        :row-label="lb => lb.name"
        storage-key="admin.system.balancers"
        :page-size="20"
        :loading="balancersLoading"
        :error="balancersError"
        :error-title="t('adminSettings.balancer.loadFailed')"
        :empty-icon="Scale"
        :empty-title="t('adminSettings.balancer.empty')"
        :empty-description="t('adminSettings.balancer.emptyDescription')"
        state-heading-tag="h3"
        :row-actions="balancerActions"
        @retry="fetchBalancers"
      >
        <template #cell-strategy="{ row }">
          <UiBadge tone="info" :dot="false" :label="getStrategyLabel(row.strategy)" />
        </template>
        <template #cell-health_check="{ row }">
          <UiBadge :tone="row.health_check ? 'success' : 'neutral'" :label="row.health_check ? t('adminSettings.balancer.on') : t('adminSettings.balancer.off')" />
        </template>
        <template #cell-enabled="{ row }">
          <UiBadge :tone="row.enabled ? 'success' : 'neutral'" :label="row.enabled ? t('adminSettings.balancer.enabled') : t('adminSettings.balancer.disabled')" />
        </template>
        <template #empty-actions>
          <UiButton variant="primary" :icon="Plus" @click="openBalancerModal()">{{ t('adminSettings.balancer.create') }}</UiButton>
        </template>
      </UiDataTable>
    </UiSection>

    <UiDialog
      v-model:open="showBalancerModal"
      :title="editingBalancer ? t('adminSettings.balancer.editTitle') : t('adminSettings.balancer.createTitle')"
      :dismissible="!balancerSaving"
    >
      <form id="system-balancer-form" class="form-grid" data-test="system-balancer-form" novalidate @submit.prevent="saveBalancer">
        <UiTextField
          id="system-balancer-name"
          v-model="balancerForm.name"
          class="form-grid__full"
          required
          :label="t('adminSettings.balancer.name')"
          :placeholder="t('adminSettings.balancer.namePlaceholder')"
          :error="nameTouched ? nameError : ''"
          @blur="nameTouched = true"
        />
        <UiNumberField
          id="system-balancer-group"
          v-model="balancerForm.group_id"
          :min="0"
          :label="t('adminSettings.balancer.groupId')"
          :help="t('adminSettings.balancer.groupIdHelp')"
        />
        <UiSelect
          id="system-balancer-strategy"
          v-model="balancerForm.strategy"
          :label="t('adminSettings.balancer.strategy')"
          :options="strategyOptions"
        />
        <div class="form-grid__full balancer-health">
          <UiSwitch v-model="balancerForm.health_check" :label="t('adminSettings.balancer.healthCheck')" />
        </div>
        <UiNumberField
          id="system-balancer-interval"
          v-model="balancerForm.check_interval"
          :min="10"
          :disabled="!balancerForm.health_check"
          :unit="t('adminSettings.balancer.seconds')"
          :label="t('adminSettings.balancer.checkInterval')"
        />
        <UiTextarea
          id="system-balancer-weights"
          v-model="balancerForm.weights_json"
          class="form-grid__full balancer-weights"
          :rows="3"
          :label="t('adminSettings.balancer.weights')"
          :placeholder="WEIGHTS_PLACEHOLDER"
          :help="t('adminSettings.balancer.weightsHelp')"
          :error="weightsError"
        />
        <p v-if="balancerError" class="form-error form-grid__full" role="alert" data-test="system-balancer-error">{{ balancerError }}</p>
      </form>
      <template #footer="{ close }">
        <UiButton :disabled="balancerSaving" @click="close">{{ t('common.actions.cancel') }}</UiButton>
        <UiButton variant="primary" type="submit" form="system-balancer-form" data-test="system-balancer-save" :loading="balancerSaving">{{ t('common.actions.save') }}</UiButton>
      </template>
    </UiDialog>
  </div>
</template>

<script setup>
// 系统设置 → 负载均衡: node group load balancers (a table, a dialog to
// create or edit, a health check and delete in the row menu). Endpoints
// unchanged: GET/POST /admin/load-balancers, PUT/DELETE
// /admin/load-balancers/:id, POST /admin/load-balancers/:id/health-check.
import { computed, onMounted, ref } from 'vue'
import { Activity, Pencil, Plus, Scale, Trash2 } from '@lucide/vue'
import { createLoadBalancer, deleteLoadBalancer, getLoadBalancers, runHealthCheck, updateLoadBalancer } from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiDialog from '@/ui/UiDialog.vue'
import UiNumberField from '@/ui/UiNumberField.vue'
import UiSection from '@/ui/UiSection.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiSwitch from '@/ui/UiSwitch.vue'
import UiTextField from '@/ui/UiTextField.vue'
import UiTextarea from '@/ui/UiTextarea.vue'
import { useConfirm } from '@/ui/composables/useConfirm'
import { useToast } from '@/ui/composables/useToast'
import { ensureSystemMutation, ensureSystemSuccess, readSystemPayload, systemErrorText } from './systemResponse'

const { t, translateLiteral } = useAppI18n()
const toast = useToast()
const confirm = useConfirm()

const WEIGHTS_PLACEHOLDER = '{"1": 10, "2": 5}'
const STRATEGIES = ['round-robin', 'least-load', 'latency', 'weight', 'random']
const STRATEGY_KEYS = {
  'round-robin': 'roundRobin',
  'least-load': 'leastLoad',
  latency: 'latency',
  weight: 'weight',
  random: 'random'
}

const balancers = ref([])
const balancersLoading = ref(false)
const balancersError = ref(null)
const showBalancerModal = ref(false)
const balancerSaving = ref(false)
const balancerError = ref('')
const editingBalancer = ref(null)
const nameTouched = ref(false)
const balancerForm = ref(emptyBalancerForm())

function emptyBalancerForm() {
  return { name: '', group_id: 0, strategy: 'round-robin', health_check: true, check_interval: 60, weights_json: '' }
}

const strategyOptions = computed(() => STRATEGIES.map(value => ({ value, label: getStrategyLabel(value) })))
const nameError = computed(() => (String(balancerForm.value.name || '').trim() ? '' : t('adminSettings.balancer.nameRequired')))
const weightsError = computed(() => {
  const text = String(balancerForm.value.weights_json || '').trim()
  if (!text) return ''
  try {
    JSON.parse(text)
    return ''
  } catch {
    return t('adminSettings.balancer.weightsInvalid')
  }
})

const translateText = (value, fallback) => {
  const text = String(value ?? '').trim()
  return text ? translateLiteral(text) : fallback
}
const resolveSystemError = (error, fallbackKey) => translateText(systemErrorText(error), t(fallbackKey))

function getStrategyLabel(strategy) {
  return STRATEGY_KEYS[strategy] ? t(`adminSettings.balancer.strategies.${STRATEGY_KEYS[strategy]}`) : (strategy || '—')
}

const balancerColumns = computed(() => [
  { key: 'name', label: t('adminSettings.balancer.name'), primary: true, sortable: true, hideable: false, truncate: true, minWidth: 120, maxWidth: 260 },
  { key: 'group_name', label: t('adminSettings.balancer.group'), secondary: true, nowrap: true, value: lb => lb.group_name || '—' },
  { key: 'strategy', label: t('adminSettings.balancer.strategy'), nowrap: true },
  { key: 'health_check', label: t('adminSettings.balancer.healthCheckColumn'), nowrap: true },
  { key: 'enabled', label: t('adminSettings.balancer.state'), nowrap: true },
  { key: 'id', label: 'ID', numeric: true, hidden: true }
])

const balancerActions = lb => [
  { key: 'health', label: t('adminSettings.balancer.runHealthCheck'), icon: Activity, onSelect: () => runHealthCheckRequest(lb) },
  { key: 'edit', label: t('common.actions.edit'), icon: Pencil, onSelect: () => openBalancerModal(lb) },
  { key: 'delete', label: t('common.actions.delete'), icon: Trash2, danger: true, separatorBefore: true, onSelect: () => deleteBalancer(lb) }
]

const fetchBalancers = async () => {
  balancersLoading.value = true
  balancersError.value = null
  try {
    const payload = readSystemPayload(await getLoadBalancers(), t('adminSettings.balancer.loadFailed'))
    balancers.value = Array.isArray(payload.list) ? payload.list : []
  } catch (err) {
    balancers.value = []
    balancersError.value = resolveSystemError(err, 'adminSettings.balancer.loadFailed')
  } finally {
    balancersLoading.value = false
  }
}

const openBalancerModal = (lb = null) => {
  if (lb) {
    editingBalancer.value = lb
    balancerForm.value = {
      name: lb.name,
      group_id: lb.group_id,
      strategy: lb.strategy,
      health_check: lb.health_check,
      check_interval: lb.check_interval,
      weights_json: typeof lb.weights === 'string' ? lb.weights : JSON.stringify(lb.weights || {})
    }
  } else {
    editingBalancer.value = null
    balancerForm.value = emptyBalancerForm()
  }
  nameTouched.value = false
  balancerError.value = ''
  showBalancerModal.value = true
}

const saveBalancer = async () => {
  if (balancerSaving.value) return
  balancerError.value = ''
  nameTouched.value = true
  if (nameError.value) return
  const data = { ...balancerForm.value }
  if (data.weights_json) {
    try {
      data.weights = JSON.parse(data.weights_json)
    } catch {
      balancerError.value = t('adminSettings.balancer.weightsInvalid')
      return
    }
  }
  balancerSaving.value = true
  try {
    delete data.weights_json
    const failed = t('adminSettings.balancer.saveFailedShort')
    if (editingBalancer.value) {
      await ensureSystemMutation(updateLoadBalancer(editingBalancer.value.id, data), failed)
    } else {
      await ensureSystemMutation(createLoadBalancer(data), failed)
    }
    showBalancerModal.value = false
    toast.success(t('adminSettings.balancer.saved', { name: data.name }))
    fetchBalancers()
  } catch (err) {
    balancerError.value = t('adminSettings.balancer.saveFailed', {
      message: resolveSystemError(err, 'adminSettings.balancer.saveFailedShort')
    })
  } finally {
    balancerSaving.value = false
  }
}

const deleteBalancer = async (lb) => {
  const confirmed = await confirm({
    tone: 'danger',
    title: t('adminSettings.balancer.deleteTitle', { name: lb.name }),
    message: t('adminSettings.balancer.deleteMessage'),
    confirmLabel: t('adminSettings.balancer.deleteAction'),
    onConfirm: async () => {
      try {
        ensureSystemSuccess(await deleteLoadBalancer(lb.id), t('adminSettings.balancer.deleteFailed'))
      } catch (err) {
        throw new Error(resolveSystemError(err, 'adminSettings.balancer.deleteFailed'))
      }
    }
  })
  if (!confirmed) return
  toast.success(t('adminSettings.balancer.deleted', { name: lb.name }))
  fetchBalancers()
}

const runHealthCheckRequest = async (lb) => {
  try {
    await ensureSystemMutation(runHealthCheck(lb.id), t('adminSettings.balancer.healthCheckFailed'))
    toast.success(t('adminSettings.balancer.healthCheckDone', { name: lb.name }))
    fetchBalancers()
  } catch (err) {
    toast.error(resolveSystemError(err, 'adminSettings.balancer.healthCheckFailed'))
  }
}

onMounted(fetchBalancers)
</script>

<style scoped>
.settings-section {
  display: flex;
  flex-direction: column;
  gap: var(--space-12);
  min-width: 0;
}

.balancer-health {
  display: flex;
  align-items: center;
}

.balancer-weights :deep(textarea) {
  font-family: var(--font-mono);
}
</style>
