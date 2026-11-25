<template>
  <UiSection :title="t('mesh.regions.title')" :description="t('mesh.regions.description')" heading-tag="h2" data-testid="mesh-regions">
    <template #actions>
      <UiButton variant="primary" size="sm" :icon="Plus" data-testid="mesh-region-create" @click="openForm(null)">{{ t('mesh.regions.create') }}</UiButton>
    </template>
    <UiDataTable
      :columns="columns"
      :rows="regions"
      :label="t('mesh.regions.title')"
      :empty-title="t('mesh.regions.empty')"
      :settings="false"
      :row-actions="rowActions"
    >
      <template #cell-name="{ row }">
        <span class="mesh-cell">
          <span>{{ row.name }}</span>
          <span class="mesh-muted mesh-mono">{{ row.code }} · {{ row.network_name }}</span>
        </span>
      </template>
      <template #cell-subnet="{ row }">
        <span class="mesh-cell mesh-mono">
          <span>{{ row.subnet }}</span>
          <span class="mesh-muted">{{ t('mesh.regions.ranges', { server: row.server_range, pool: row.device_pool }) }}</span>
        </span>
      </template>
      <template #cell-pool="{ row }">{{ t('mesh.regions.poolUsage', { used: row.pool_used, size: row.pool_size }) }}</template>
      <template #cell-status="{ row }">
        <span class="mesh-badges">
          <UiBadge tone="success" :label="t('mesh.regions.forwardSecrecy')" />
          <UiBadge v-for="warning in row.warnings" :key="warning" tone="warning" :label="t(`mesh.regions.warnings.${warning}`)" />
        </span>
      </template>
    </UiDataTable>

    <UiDialog :open="Boolean(form)" :title="form?.id ? t('mesh.regions.editTitle') : t('mesh.regions.createTitle')" @update:open="form = null">
      <form v-if="form" class="mesh-form" novalidate @submit.prevent="save">
        <div class="mesh-pair">
          <UiTextField v-model="form.code" size="md" :label="t('mesh.regions.code')" :help="t('mesh.regions.codeHelp')" :disabled="Boolean(form.id)" data-testid="mesh-region-code" />
          <UiTextField v-model="form.name" size="md" :label="t('mesh.regions.name')" data-testid="mesh-region-name" />
        </div>
        <UiTextField v-model="form.subnet" size="md" :label="t('mesh.regions.subnet')" placeholder="10.125.0.0/16" :help="t('mesh.regions.subnetHelp')" data-testid="mesh-region-subnet" />
        <div class="mesh-pair">
          <UiTextField v-model="form.server_range" size="md" :label="t('mesh.regions.serverRange')" placeholder="10.125.0.0/20" data-testid="mesh-region-server" />
          <UiTextField v-model="form.device_pool" size="md" :label="t('mesh.regions.devicePool')" placeholder="10.125.64.0/18" data-testid="mesh-region-pool" />
        </div>
        <UiNumberField v-model="form.sort_order" size="md" :label="t('mesh.regions.sortOrder')" :min="0" :max="10000" />
        <p v-if="error" class="mesh-error" role="alert">{{ error }}</p>
      </form>
      <template #footer>
        <UiButton @click="form = null">{{ t('mesh.cancel') }}</UiButton>
        <UiButton variant="primary" :loading="saving" :disabled="saving" data-testid="mesh-region-save" @click="save">{{ t('mesh.save') }}</UiButton>
      </template>
    </UiDialog>
  </UiSection>
</template>

<script setup>
// 组网 → 地区: each region is an independent network with forward secrecy
// and a subnet split into a server range and a device pool.
import { computed, ref } from 'vue'
import { Plus } from '@lucide/vue'
import { createMeshRegion, deleteMeshRegion, meshErrorMessage, updateMeshRegion } from '@/api/mesh'
import { useAppI18n } from '@/composables/useAppI18n'
import { UiBadge, UiButton, UiDataTable, UiDialog, UiNumberField, UiSection, UiTextField, useConfirm, useToast } from '@/ui'

defineProps({
  regions: { type: Array, required: true }
})
const emit = defineEmits(['changed'])
const { t } = useAppI18n()
const confirm = useConfirm()
const toast = useToast()

const form = ref(null)
const saving = ref(false)
const error = ref('')

const columns = computed(() => [
  { key: 'name', label: t('mesh.regions.name'), primary: true, hideable: false },
  { key: 'subnet', label: t('mesh.regions.subnet') },
  { key: 'border_nodes', label: t('mesh.regions.borderNodes'), align: 'end', numeric: true },
  { key: 'members', label: t('mesh.regions.members'), align: 'end', numeric: true, breakpoint: 'md' },
  { key: 'pool', label: t('mesh.regions.pool'), breakpoint: 'md' },
  { key: 'status', label: t('mesh.regions.status') }
])

function rowActions(row) {
  return [
    { key: 'edit', label: t('mesh.edit'), onSelect: () => openForm(row) },
    { key: 'delete', label: t('mesh.delete'), danger: true, separatorBefore: true, onSelect: () => remove(row) }
  ]
}

function openForm(row) {
  error.value = ''
  form.value = row
    ? { id: row.id, code: row.code, name: row.name, subnet: row.subnet, server_range: row.server_range, device_pool: row.device_pool, sort_order: row.sort_order }
    : { id: 0, code: '', name: '', subnet: '', server_range: '', device_pool: '', sort_order: 0 }
}

async function save() {
  if (!form.value || saving.value) return
  saving.value = true
  error.value = ''
  const { id, ...body } = form.value
  body.sort_order = Number(body.sort_order) || 0
  try {
    if (id) await updateMeshRegion(id, body)
    else await createMeshRegion(body)
    toast.success(t('mesh.saved'))
    form.value = null
    emit('changed')
  } catch (e) {
    error.value = meshErrorMessage(e, t('mesh.saveFailed'))
  } finally {
    saving.value = false
  }
}

async function remove(row) {
  const ok = await confirm({
    title: t('mesh.regions.deleteTitle', { name: row.name }),
    message: t('mesh.regions.deleteMessage'),
    confirmLabel: t('mesh.delete'),
    tone: 'danger',
    onConfirm: () => deleteMeshRegion(row.id)
  })
  if (!ok) return
  toast.success(t('mesh.deleted'))
  emit('changed')
}
</script>

<style scoped src="./mesh.css"></style>
