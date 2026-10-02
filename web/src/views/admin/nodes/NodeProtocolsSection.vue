<template>
  <UiSection :title="t('admin.nodes.protocols.title')" :description="t('admin.nodes.protocols.description')">
    <template #actions>
      <UiButton variant="primary" size="md" :icon="Plus" data-testid="add-protocol" @click="openEditor(null)">{{ t('admin.nodes.protocols.add') }}</UiButton>
    </template>
    <UiDataTable
      :columns="columns"
      :rows="protocols"
      :label="t('admin.nodes.protocols.tableLabel', { name: node.name })"
      :row-label="protocolLabel"
      storage-key="admin.node-protocols"
      :loading="loading"
      :error="error"
      :error-title="t('admin.nodes.protocols.loadFailed')"
      :empty-icon="Layers"
      :empty-title="t('admin.nodes.protocols.empty')"
      :empty-description="t('admin.nodes.protocols.emptyDescription')"
      :row-actions="protocolActions"
      state-heading-tag="h3"
      activatable
      @row-activate="openEditor"
      @retry="loadProtocols"
    >
      <template #empty-actions>
        <UiButton variant="primary" :icon="Plus" @click="openEditor(null)">{{ t('admin.nodes.protocols.add') }}</UiButton>
      </template>
      <template #cell-type="{ row }">
        <span class="node-protocol-type">{{ String(row.type || 'unknown').toUpperCase() }}</span>
      </template>
      <template #cell-enable="{ row }">
        <UiBadge :tone="row.enable ? 'success' : 'neutral'" :label="row.enable ? t('admin.nodes.protocols.enabled') : t('admin.nodes.protocols.disabled')" />
      </template>
    </UiDataTable>

    <NodeProtocolSheet
      v-model:open="editorOpen"
      :node="node"
      :protocol="editing"
      :templates="templates"
      @saved="loadProtocols"
    />
  </UiSection>
</template>

<script setup>
// 协议 section of the node page: the node's protocols
// (GET /admin/nodes/:id/protocols), the editor sheet, and delete with a
// confirmation (DELETE /admin/nodes/:id/protocols/:protocol_id).
import { computed, onMounted, ref } from 'vue'
import { Layers, Pencil, Plus, Trash2 } from '@lucide/vue'
import { deleteNodeProtocol, getNodeProtocols, getProtocolTemplates } from '@/api/admin'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiSection from '@/ui/UiSection.vue'
import { useConfirm } from '@/ui/composables/useConfirm'
import { useToast } from '@/ui/composables/useToast'
import { useAppI18n } from '@/composables/useAppI18n'
import NodeProtocolSheet from './NodeProtocolSheet.vue'
import { readNodeApiError, readNodeList } from './nodeData'

const props = defineProps({
  node: { type: Object, required: true }
})
const emit = defineEmits(['changed'])
const { t } = useAppI18n()
const toast = useToast()
const confirm = useConfirm()

const protocols = ref([])
const loading = ref(false)
const error = ref(null)
const templates = ref([])
const editorOpen = ref(false)
const editing = ref(null)

const TLS_KEYS = ['none', 'standard', 'reality']
const columns = computed(() => [
  { key: 'type', label: t('admin.nodes.protocols.columns.type'), primary: true, sortable: true },
  { key: 'port', label: t('admin.nodes.protocols.columns.port'), numeric: true, sortable: true, secondary: true },
  { key: 'transport', label: t('admin.nodes.protocols.columns.transport'), value: row => String(row.transport || 'tcp').toUpperCase() },
  { key: 'tls', label: t('admin.nodes.protocols.columns.tls'), value: row => (TLS_KEYS[row.tls] ? t(`admin.nodes.tlsModes.${TLS_KEYS[row.tls]}`) : '—'), breakpoint: 'md' },
  { key: 'enable', label: t('admin.nodes.protocols.columns.status'), sortValue: row => (row.enable ? 1 : 0), sortable: true },
  { key: 'show', label: t('admin.nodes.protocols.columns.show'), value: row => (row.show === 0 ? t('admin.nodes.protocols.hidden') : t('admin.nodes.protocols.listed')), breakpoint: 'lg' }
])

const protocolLabel = protocol => `${String(protocol.type || 'unknown').toUpperCase()} :${protocol.port}`

const protocolActions = protocol => [
  { key: 'edit', label: t('admin.nodes.actions.edit'), icon: Pencil, onSelect: () => openEditor(protocol) },
  { key: 'delete', label: t('admin.nodes.actions.delete'), icon: Trash2, danger: true, separatorBefore: true, onSelect: () => deleteProtocol(protocol) }
]

async function loadProtocols() {
  loading.value = true
  error.value = null
  try {
    protocols.value = readNodeList(await getNodeProtocols(props.node.id))
    emit('changed', protocols.value)
  } catch (e) {
    console.error('Failed to load protocols:', e)
    error.value = e
  } finally {
    loading.value = false
  }
}

async function loadTemplates() {
  try {
    templates.value = readNodeList(await getProtocolTemplates())
  } catch (e) {
    console.error('Failed to load templates:', e)
  }
}

function openEditor(protocol) {
  editing.value = protocol || null
  editorOpen.value = true
}

async function deleteProtocol(protocol) {
  const label = { type: String(protocol.type || 'unknown').toUpperCase(), port: protocol.port, name: props.node.name || '-' }
  const confirmed = await confirm({
    title: t('admin.nodes.confirm.deleteProtocolTitle', label),
    message: t('admin.nodes.confirm.deleteProtocolMessage', label),
    confirmLabel: t('admin.nodes.confirm.deleteProtocolAction'),
    tone: 'danger',
    onConfirm: async () => {
      try {
        await deleteNodeProtocol(props.node.id, protocol.id)
      } catch (e) {
        throw new Error(readNodeApiError(e))
      }
    }
  })
  if (!confirmed) return
  toast.success(t('admin.nodes.messages.protocolDeleted', label))
  await loadProtocols()
}

onMounted(() => {
  loadProtocols()
  loadTemplates()
})

defineExpose({ loadProtocols, openEditor, protocols })
</script>

<style scoped>
.node-protocol-type {
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
  font-weight: var(--weight-semibold);
}
</style>
