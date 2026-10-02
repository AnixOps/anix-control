<template>
  <UiSection :title="t('adminSubscriptionGroups.protocols.title')" :description="t('adminSubscriptionGroups.protocols.description')">
    <template #actions>
      <UiButton :icon="Link2" :loading="availableLoading" data-test="manage-protocols" @click="openManageProtocolsModal">{{ t('adminSubscriptionGroups.protocols.manage') }}</UiButton>
    </template>
    <UiDataTable
      :columns="columns"
      :rows="protocols"
      :label="t('adminSubscriptionGroups.protocols.title')"
      :row-label="protocol => protocol.name"
      storage-key="admin.subscriptionProtocols"
      :page-size="20"
      :loading="protocolsLoading"
      :error="protocolsLoadError"
      :error-title="t('adminSubscriptionGroups.protocols.loadFailed')"
      :empty-icon="Server"
      :empty-title="t('adminSubscriptionGroups.protocols.empty')"
      :empty-description="t('adminSubscriptionGroups.protocols.emptyDescription')"
      :row-actions="protocolActions"
      @retry="loadProtocols"
    >
      <template #cell-type="{ row }">
        <UiBadge tone="info" :dot="false" :label="String(row.type || '').toUpperCase()" />
      </template>
      <template #cell-show="{ row }">
        <UiBadge :tone="row.show ? 'success' : 'neutral'" :label="row.show ? t('adminSubscriptionGroups.protocols.shown') : t('adminSubscriptionGroups.protocols.hidden')" />
      </template>
      <template #cell-enable="{ row }">
        <UiBadge :status="row.enable ? 'online' : 'offline'" :label="row.enable ? t('adminSubscriptionGroups.protocols.online') : t('adminSubscriptionGroups.protocols.offline')" />
      </template>
      <template #empty-actions>
        <UiButton variant="primary" :icon="Link2" @click="openManageProtocolsModal">{{ t('adminSubscriptionGroups.protocols.manage') }}</UiButton>
      </template>
    </UiDataTable>

    <UiDialog
      v-model:open="showManageProtocolsModal"
      size="lg"
      :title="t('adminSubscriptionGroups.protocols.manageTitle')"
      :description="t('adminSubscriptionGroups.protocols.manageDescription', { group: group.name })"
      :dismissible="!protocolsSaving"
    >
      <p v-if="!availableProtocols.length" class="pool-empty">{{ t('adminSubscriptionGroups.protocols.poolEmpty') }}</p>
      <template v-else>
        <div class="pool-head">
          <UiCheckbox
            :model-value="allSelected ? true : selectedProtocolIds.length ? 'indeterminate' : false"
            :label="t('adminSubscriptionGroups.protocols.selectAll')"
            data-test="pool-select-all"
            @update:model-value="toggleAllAvailable"
          />
          <span class="pool-count" aria-live="polite">{{ t('adminSubscriptionGroups.protocols.selectedCount', { count: selectedProtocolIds.length }) }}</span>
        </div>
        <ul class="pool-list" :aria-label="t('adminSubscriptionGroups.protocols.pool')">
          <li v-for="protocol in availableProtocols" :key="protocol.id" class="pool-item">
            <UiCheckbox
              :model-value="selectedProtocolIds.includes(protocol.id)"
              :label="`${protocol.node?.name || t('adminSubscriptionGroups.protocols.unknownNode')} · ${String(protocol.type || '').toUpperCase()} ${protocol.name} :${protocol.port}`"
              :description="poolGroups(protocol)"
              @update:model-value="toggleProtocolSelection(protocol.id)"
            />
          </li>
        </ul>
      </template>
      <p v-if="protocolsError" class="form-error" role="alert">{{ protocolsError }}</p>
      <template #footer="{ close }">
        <UiButton :disabled="protocolsSaving" @click="close">{{ t('common.actions.cancel') }}</UiButton>
        <UiButton variant="primary" data-test="save-group-protocols" :loading="protocolsSaving" @click="saveGroupProtocols">{{ t('adminSubscriptionGroups.protocols.save') }}</UiButton>
      </template>
    </UiDialog>
  </UiSection>
</template>

<script setup>
// The production node protocols linked to one subscription group, and the
// dialog that picks them from every visible node protocol. Endpoints
// unchanged: GET/POST /admin/subscription/groups/:id/protocols,
// GET /admin/subscription/protocols/available.
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { Link2, Server, SquareArrowOutUpRight } from '@lucide/vue'
import adminApi from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiCheckbox from '@/ui/UiCheckbox.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiDialog from '@/ui/UiDialog.vue'
import UiSection from '@/ui/UiSection.vue'
import { useToast } from '@/ui/composables/useToast'
import { ensureSubscriptionSuccess, readSubscriptionList, subscriptionErrorText } from './subscriptionShared'

const props = defineProps({
  group: { type: Object, required: true }
})
const emit = defineEmits(['changed'])

const { t } = useAppI18n()
const toast = useToast()
const router = useRouter()

const protocols = ref([])
const protocolsLoading = ref(false)
const protocolsLoadError = ref(null)
const availableProtocols = ref([])
const availableLoading = ref(false)
const selectedProtocolIds = ref([])
const showManageProtocolsModal = ref(false)
const protocolsSaving = ref(false)
const protocolsError = ref('')

const columns = computed(() => [
  { key: 'node', label: t('adminSubscriptionGroups.protocols.node'), primary: true, sortable: true, value: protocol => protocol.node?.name || t('adminSubscriptionGroups.protocols.nodeIdFallback', { id: protocol.node_id }) },
  { key: 'name', label: t('adminSubscriptionGroups.protocols.name'), secondary: true, sortable: true },
  { key: 'type', label: t('adminSubscriptionGroups.protocols.protocol') },
  { key: 'port', label: t('adminSubscriptionGroups.protocols.port'), numeric: true, align: 'end' },
  { key: 'show', label: t('adminSubscriptionGroups.protocols.visibility'), breakpoint: 'md' },
  { key: 'enable', label: t('adminSubscriptionGroups.protocols.status') }
])

const allSelected = computed(() => availableProtocols.value.length > 0 && selectedProtocolIds.value.length === availableProtocols.value.length)

function poolGroups(protocol) {
  const names = (protocol.subscription_groups || []).map(item => item.name).filter(Boolean)
  return names.length ? t('adminSubscriptionGroups.protocols.linkedTo', { groups: names.join(', ') }) : ''
}

function toggleProtocolSelection(id) {
  const index = selectedProtocolIds.value.indexOf(id)
  if (index > -1) selectedProtocolIds.value.splice(index, 1)
  else selectedProtocolIds.value.push(id)
}

function toggleAllAvailable() {
  selectedProtocolIds.value = allSelected.value ? [] : availableProtocols.value.map(protocol => protocol.id)
}

const protocolActions = protocol => [
  { key: 'node', label: t('adminSubscriptionGroups.protocols.goToNode'), icon: SquareArrowOutUpRight, onSelect: () => goToNode(protocol.node_id) }
]

async function loadProtocols() {
  protocolsLoading.value = true
  protocolsLoadError.value = null
  try {
    const res = await adminApi.getSubscriptionProtocols(props.group.id)
    protocols.value = readSubscriptionList(res, t('adminSubscriptionGroups.protocols.loadFailed'))
  } catch (error) {
    protocols.value = []
    protocolsLoadError.value = subscriptionErrorText(error) || t('adminSubscriptionGroups.protocols.loadFailed')
  } finally {
    protocolsLoading.value = false
  }
}

async function loadAvailableProtocols() {
  availableLoading.value = true
  try {
    const res = await adminApi.getAvailableProtocols()
    availableProtocols.value = readSubscriptionList(res, t('adminSubscriptionGroups.protocols.availableLoadFailed'))
    return true
  } catch {
    availableProtocols.value = []
    toast.error(t('adminSubscriptionGroups.protocols.availableLoadFailed'))
    return false
  } finally {
    availableLoading.value = false
  }
}

async function openManageProtocolsModal() {
  if (!props.group) return
  await loadAvailableProtocols()
  selectedProtocolIds.value = protocols.value.map(protocol => protocol.id)
  protocolsError.value = ''
  showManageProtocolsModal.value = true
}

async function saveGroupProtocols() {
  if (protocolsSaving.value) return
  protocolsSaving.value = true
  protocolsError.value = ''
  try {
    ensureSubscriptionSuccess(
      await adminApi.updateGroupProtocols(props.group.id, selectedProtocolIds.value),
      t('adminSubscriptionGroups.protocols.saveFailed')
    )
    toast.success(t('adminSubscriptionGroups.protocols.saved'))
    showManageProtocolsModal.value = false
    loadProtocols()
    emit('changed')
  } catch {
    protocolsError.value = t('adminSubscriptionGroups.protocols.saveFailed')
  } finally {
    protocolsSaving.value = false
  }
}

function goToNode(nodeId) {
  router.push({ path: '/admin/nodes', query: { id: String(nodeId) } })
}

watch(() => props.group?.id, id => { if (id) loadProtocols() }, { immediate: true })
</script>

<style scoped>
.pool-head {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2) var(--space-4);
  align-items: center;
  justify-content: space-between;
  padding: 0 0 var(--space-3);
  border-bottom: 1px solid var(--separator);
}

.pool-count,
.pool-empty {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.pool-list {
  margin: 0;
  padding: 0;
  list-style: none;
}

.pool-item {
  padding: var(--space-3) 0;
}

.pool-item + .pool-item {
  border-top: 1px solid var(--separator);
}
</style>
