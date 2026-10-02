// Node actions shared by the list and the node page: sync / reload
// (POST /admin/nodes/:id/sync, the kernel's node.sync operation) and delete
// (DELETE /admin/nodes/:id) after the node's name is typed.
import { reactive } from 'vue'
import { deleteNode, syncNodeProtocol } from '@/api/admin'
import { useConfirm } from '@/ui/composables/useConfirm'
import { useToast } from '@/ui/composables/useToast'
import { useAppI18n } from '@/composables/useAppI18n'
import { readNodeApiError } from './nodeData'

export function useNodeActions() {
  const { t } = useAppI18n()
  const toast = useToast()
  const confirm = useConfirm()
  const syncingNodeIds = reactive(new Set())

  async function syncNode(node) {
    if (syncingNodeIds.has(node.id)) return
    syncingNodeIds.add(node.id)
    try {
      await syncNodeProtocol(node.id)
      toast.success(t('admin.nodes.messages.syncSuccess', { name: node.name }))
    } catch (e) {
      toast.error(t('admin.nodes.messages.syncFailed', { message: readNodeApiError(e) }))
    } finally {
      syncingNodeIds.delete(node.id)
    }
  }

  // Deleting a node removes its protocols and stops every subscription
  // that lists it: type the node name to confirm. Resolves true once deleted.
  async function confirmDelete(node) {
    const confirmed = await confirm({
      title: t('admin.nodes.confirm.deleteNodeTitle', { name: node.name }),
      message: t('admin.nodes.confirm.deleteNodeMessage'),
      confirmLabel: t('admin.nodes.confirm.deleteNodeAction'),
      tone: 'danger',
      requireText: String(node.name || node.id),
      onConfirm: async () => {
        try {
          await deleteNode(node.id)
        } catch (e) {
          throw new Error(readNodeApiError(e))
        }
      }
    })
    if (!confirmed) return false
    toast.success(t('admin.nodes.messages.nodeDeleted', { name: node.name }))
    return true
  }

  return { syncingNodeIds, syncNode, confirmDelete }
}
