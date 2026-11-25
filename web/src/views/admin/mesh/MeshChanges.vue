<template>
  <UiSection :title="t('mesh.changes.title')" :description="t('mesh.changes.description')" heading-tag="h2" data-testid="mesh-changes">
    <div class="mesh-toolbar">
      <UiSegmentedControl v-model="status" size="sm" :options="statusOptions" :aria-label="t('mesh.changes.status')" />
    </div>
    <UiDataTable
      :columns="columns"
      :rows="changes || []"
      :label="t('mesh.changes.title')"
      :loading="!changes && !loadError"
      :error="changes ? null : loadError"
      :error-title="t('mesh.changes.loadFailed')"
      :empty-title="t('mesh.changes.empty')"
      :settings="false"
      :row-actions="rowActions"
      @retry="load"
    >
      <template #cell-hostname="{ row }">
        <span class="mesh-cell">
          <span>{{ row.hostname }}</span>
          <span class="mesh-muted">{{ t(`mesh.changes.sources.${row.source}`) }} · {{ format.dateTime(row.created_at) }}</span>
        </span>
      </template>
      <template #cell-move="{ row }">
        <span class="mesh-cell">
          <span>{{ regionName(row.from_region_id, row.from_code) }} → {{ regionName(row.to_region_id, row.to_code) }}</span>
          <span v-if="row.new_address" class="mesh-muted mesh-mono">{{ row.old_address || '—' }} → {{ row.new_address }}</span>
        </span>
      </template>
      <template #cell-status="{ row }">
        <UiBadge :tone="tones[row.status]" :label="t(`mesh.changes.statuses.${row.status}`)" />
      </template>
    </UiDataTable>

    <UiDialog :open="Boolean(decision)" :title="decision ? t(decision.approve ? 'mesh.changes.approveTitle' : 'mesh.changes.rejectTitle', { name: decision.row.hostname }) : ''" :description="decision?.approve ? t('mesh.changes.approveDescription') : ''" @update:open="decision = null">
      <form v-if="decision" class="mesh-form" novalidate @submit.prevent="decide">
        <UiTextField v-if="decision.approve" v-model="decision.address" size="md" :label="t('mesh.members.address')" :help="t('mesh.changes.addressHelp')" />
        <UiTextField v-model="decision.note" size="md" :label="t('mesh.note')" />
        <p v-if="decisionError" class="mesh-error" role="alert">{{ decisionError }}</p>
      </form>
      <template #footer>
        <UiButton @click="decision = null">{{ t('mesh.cancel') }}</UiButton>
        <UiButton :variant="decision?.approve ? 'primary' : 'danger'" :loading="deciding" :disabled="deciding" data-testid="mesh-change-decide" @click="decide">
          {{ decision?.approve ? t('mesh.changes.approve') : t('mesh.changes.reject') }}
        </UiButton>
      </template>
    </UiDialog>
  </UiSection>
</template>

<script setup>
// 组网 → 变更申请: members ask to move to another region; an administrator
// approves (the member gets a new address) or rejects.
import { computed, onMounted, ref, watch } from 'vue'
import { approveMeshRegionChange, listMeshRegionChanges, meshErrorMessage, rejectMeshRegionChange } from '@/api/mesh'
import { useAppI18n } from '@/composables/useAppI18n'
import { UiBadge, UiButton, UiDataTable, UiDialog, UiSection, UiSegmentedControl, UiTextField, useFormat, useToast } from '@/ui'

const props = defineProps({
  regions: { type: Array, required: true }
})
const emit = defineEmits(['changed'])
const { t } = useAppI18n()
const format = useFormat()
const toast = useToast()

const status = ref('pending')
const changes = ref(null)
const loadError = ref(null)
const decision = ref(null)
const decisionError = ref('')
const deciding = ref(false)

const tones = { pending: 'info', approved: 'success', rejected: 'neutral' }
const statusOptions = computed(() => ['pending', 'approved', 'rejected', 'all'].map(value => ({ value, label: t(`mesh.changes.statuses.${value}`) })))

const columns = computed(() => [
  { key: 'hostname', label: t('mesh.changes.member'), primary: true, hideable: false },
  { key: 'move', label: t('mesh.changes.move') },
  { key: 'reason', label: t('mesh.changes.reason'), breakpoint: 'md', truncate: true },
  { key: 'status', label: t('mesh.changes.status') }
])

function regionName(id, code) {
  return props.regions.find(region => region.id === id)?.name || code || '—'
}

function rowActions(row) {
  if (row.status !== 'pending') return []
  return [
    { key: 'approve', label: t('mesh.changes.approve'), onSelect: () => openDecision(row, true) },
    { key: 'reject', label: t('mesh.changes.reject'), danger: true, onSelect: () => openDecision(row, false) }
  ]
}

async function load() {
  loadError.value = null
  changes.value = null
  try {
    changes.value = (await listMeshRegionChanges(status.value === 'all' ? '' : status.value)) || []
  } catch (e) {
    loadError.value = e
  }
}

function openDecision(row, approve) {
  decisionError.value = ''
  decision.value = { row, approve, address: '', note: '' }
}

async function decide() {
  if (!decision.value || deciding.value) return
  deciding.value = true
  decisionError.value = ''
  const { row, approve, address, note } = decision.value
  try {
    if (approve) await approveMeshRegionChange(row.id, { address, note })
    else await rejectMeshRegionChange(row.id, { note })
    toast.success(t(approve ? 'mesh.changes.approved' : 'mesh.changes.rejected'))
    decision.value = null
    await load()
    emit('changed')
  } catch (e) {
    decisionError.value = meshErrorMessage(e, t('mesh.saveFailed'))
  } finally {
    deciding.value = false
  }
}

watch(status, load)
onMounted(load)
</script>

<style scoped src="./mesh.css"></style>
