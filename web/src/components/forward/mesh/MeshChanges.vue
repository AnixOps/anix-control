<template>
  <UiSection :title="t('meshAddr.changes.title')" :description="t('meshAddr.changes.description')" heading-tag="h2" data-testid="mesh-changes">
    <div class="mesh-toolbar">
      <UiSegmentedControl v-model="status" size="sm" :options="statusOptions" :aria-label="t('meshAddr.changes.status')" />
    </div>
    <UiDataTable
      :columns="columns"
      :rows="changes || []"
      row-key="id"
      :label="t('meshAddr.changes.title')"
      :row-label="row => memberName(row)"
      :row-actions="rowActions"
      :loading="!changes && !loadError"
      :error="changes ? null : loadError"
      :error-title="t('meshAddr.changes.loadFailed')"
      :empty-title="t('meshAddr.changes.empty')"
      data-testid="mesh-change-table"
      @retry="load"
    >
      <template #cell-member="{ row }">
        <span class="mesh-cell">
          <span>{{ memberName(row) }}</span>
          <span class="mesh-muted">{{ t(`meshAddr.changes.sources.${row.source}`) }} · {{ format.dateTime(row.created_at) }}</span>
        </span>
      </template>
      <template #cell-move="{ row }">
        <span class="mesh-cell">
          <span>{{ regionName(row.from_region_id, row.from_region_code) }} → {{ regionName(row.to_region_id, row.to_region_code) }}</span>
          <span class="mesh-muted mesh-mono">{{ row.old_address || '—' }} → {{ row.new_address || '—' }}</span>
        </span>
      </template>
      <template #cell-status="{ row }">
        <span class="mesh-cell">
          <UiBadge :tone="TONES[row.status]" :label="t(`meshAddr.changes.statuses.${row.status}`)" />
          <span v-if="row.note" class="mesh-muted">{{ row.note }}</span>
        </span>
      </template>
    </UiDataTable>

    <UiDialog :open="Boolean(decision)" :title="decision ? t(decision.approve ? 'meshAddr.changes.approveTitle' : 'meshAddr.changes.rejectTitle') : ''" :description="decision?.approve ? t('meshAddr.changes.approveDescription') : ''" @update:open="value => { if (!value) decision = null }">
      <form v-if="decision" class="mesh-form" novalidate @submit.prevent="decide">
        <UiTextField v-model="decision.note" size="md" :label="t('meshAddr.changes.note')" />
        <p v-if="decisionError" class="mesh-error" role="alert">{{ decisionError }}</p>
      </form>
      <template #footer>
        <UiButton :disabled="deciding" @click="decision = null">{{ t('meshAddr.common.cancel') }}</UiButton>
        <UiButton :variant="decision?.approve ? 'primary' : 'danger'" :loading="deciding" :disabled="deciding" data-testid="mesh-change-decide" @click="decide">
          {{ decision?.approve ? t('meshAddr.changes.confirmApprove') : t('meshAddr.changes.confirmReject') }}
        </UiButton>
      </template>
    </UiDialog>
  </UiSection>
</template>

<script setup>
// 组网 → 地区变更: a member asks to move to another region, which changes its
// address; an administrator approves or rejects (R20, R24).
import { computed, onMounted, ref, watch } from 'vue'
import { Check, X } from '@lucide/vue'
import { UiBadge, UiButton, UiDataTable, UiDialog, UiSection, UiSegmentedControl, UiTextField, useFormat, useToast } from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import { decideKernelMeshChange, listKernelMeshChanges } from '@/api/kernel'
import { meshErrorMessage } from '../meshModel'

const props = defineProps({
  regions: { type: Array, default: () => [] },
  members: { type: Array, default: () => [] }
})
const emit = defineEmits(['changed'])
const { t } = useAppI18n()
const format = useFormat()
const toast = useToast()

const TONES = { pending: 'info', approved: 'success', rejected: 'neutral' }
const status = ref('pending')
const changes = ref(null)
const loadError = ref(null)
const decision = ref(null)
const decisionError = ref('')
const deciding = ref(false)

const statusOptions = computed(() => ['pending', 'approved', 'rejected', 'all'].map(value => ({ value, label: t(`meshAddr.changes.statuses.${value}`) })))
const columns = computed(() => [
  { key: 'member', label: t('meshAddr.changes.member'), primary: true },
  { key: 'move', label: t('meshAddr.changes.move'), secondary: true },
  { key: 'reason', label: t('meshAddr.changes.reason'), breakpoint: 'md' },
  { key: 'status', label: t('meshAddr.changes.status') }
])

function memberName(row) {
  if (row.node_ref) return props.members.find(member => member.node_ref === row.node_ref)?.node_name || row.node_ref
  return t('meshAddr.changes.device', { id: row.device_id })
}

function regionName(id, code) {
  return props.regions.find(region => region.id === id)?.name || code || '—'
}

function rowActions(row) {
  if (row.status !== 'pending') return []
  return [
    { key: 'approve', label: t('meshAddr.changes.approve'), icon: Check, onSelect: () => openDecision(row, true) },
    { key: 'reject', label: t('meshAddr.changes.reject'), icon: X, danger: true, onSelect: () => openDecision(row, false) }
  ]
}

async function load() {
  loadError.value = null
  changes.value = null
  try {
    changes.value = (await listKernelMeshChanges(status.value === 'all' ? '' : status.value)) || []
  } catch (e) {
    loadError.value = { message: meshErrorMessage(e) }
  }
}

function openDecision(row, approve) {
  decisionError.value = ''
  decision.value = { row, approve, note: '' }
}

async function decide() {
  if (!decision.value || deciding.value) return
  deciding.value = true
  decisionError.value = ''
  const { row, approve, note } = decision.value
  try {
    await decideKernelMeshChange(row.id, approve, note)
    toast.success(t(approve ? 'meshAddr.changes.approved' : 'meshAddr.changes.rejected'))
    decision.value = null
    await load()
    emit('changed')
  } catch (e) {
    decisionError.value = meshErrorMessage(e)
  } finally {
    deciding.value = false
  }
}

watch(status, load)
onMounted(load)
</script>

<style scoped src="./meshAddr.css"></style>
