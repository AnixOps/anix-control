<template>
  <UiSection :title="t('admin.nodes.danger.title')" :description="t('admin.nodes.danger.description')">
    <UiGroupedList class="node-danger">
      <UiGroupedListRow
        v-if="disabled"
        :label="t('admin.nodes.danger.enable')"
        :description="t('admin.nodes.danger.enableHint')"
      >
        <UiButton size="sm" :icon="Power" :loading="busy" data-testid="enable-node" @click="enable">{{ t('admin.nodes.danger.enableAction') }}</UiButton>
      </UiGroupedListRow>
      <UiGroupedListRow
        v-else
        :label="t('admin.nodes.danger.disable')"
        :description="t('admin.nodes.danger.disableHint')"
      >
        <UiButton size="sm" variant="danger-soft" :icon="PowerOff" :loading="busy" data-testid="disable-node" @click="disable">{{ t('admin.nodes.danger.disableAction') }}</UiButton>
      </UiGroupedListRow>
      <UiGroupedListRow
        :label="t('admin.nodes.danger.delete')"
        :description="t('admin.nodes.danger.deleteHint')"
      >
        <UiButton size="sm" variant="danger-soft" :icon="Trash2" data-testid="delete-node" @click="remove">{{ t('admin.nodes.danger.deleteAction') }}</UiButton>
      </UiGroupedListRow>
    </UiGroupedList>
  </UiSection>
</template>

<script setup>
// 危险区 of the node page: disable or enable (PUT /admin/nodes/:id with the
// edit form's body and status 3 / 0, as the status field of the form
// always did) and delete (DELETE /admin/nodes/:id) after the name is typed.
import { computed, ref } from 'vue'
import { Power, PowerOff, Trash2 } from '@lucide/vue'
import { updateNode } from '@/api/admin'
import UiButton from '@/ui/UiButton.vue'
import UiGroupedList from '@/ui/UiGroupedList.vue'
import UiGroupedListRow from '@/ui/UiGroupedListRow.vue'
import UiSection from '@/ui/UiSection.vue'
import { useConfirm } from '@/ui/composables/useConfirm'
import { useToast } from '@/ui/composables/useToast'
import { useAppI18n } from '@/composables/useAppI18n'
import { NODE_STATUS, buildNodePayload, nodeFormFrom, readNodeApiError } from './nodeData'
import { useNodeActions } from './useNodeActions'

const props = defineProps({
  node: { type: Object, required: true }
})
const emit = defineEmits(['changed', 'deleted'])
const { t } = useAppI18n()
const toast = useToast()
const confirm = useConfirm()
const { confirmDelete } = useNodeActions()

const busy = ref(false)
const disabled = computed(() => Number(props.node.status) === NODE_STATUS.disabled)

async function setStatus(status) {
  const payload = buildNodePayload({ ...nodeFormFrom(props.node), status }, { editing: true })
  await updateNode(props.node.id, payload)
}

async function disable() {
  const confirmed = await confirm({
    title: t('admin.nodes.confirm.disableNodeTitle', { name: props.node.name }),
    message: t('admin.nodes.confirm.disableNodeMessage'),
    confirmLabel: t('admin.nodes.confirm.disableNodeAction'),
    tone: 'danger',
    onConfirm: async () => {
      try {
        await setStatus(NODE_STATUS.disabled)
      } catch (e) {
        throw new Error(readNodeApiError(e))
      }
    }
  })
  if (!confirmed) return
  toast.success(t('admin.nodes.messages.nodeDisabled', { name: props.node.name }))
  emit('changed')
}

async function enable() {
  if (busy.value) return
  busy.value = true
  try {
    await setStatus(NODE_STATUS.pending)
    toast.success(t('admin.nodes.messages.nodeEnabled', { name: props.node.name }))
    emit('changed')
  } catch (e) {
    toast.error(t('admin.nodes.messages.saveFailed', { message: readNodeApiError(e) }))
  } finally {
    busy.value = false
  }
}

async function remove() {
  if (await confirmDelete(props.node)) emit('deleted')
}
</script>

<style scoped>
.node-danger {
  max-width: 760px;
}
</style>
