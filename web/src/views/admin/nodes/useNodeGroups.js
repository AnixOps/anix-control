// The node groups of the 节点 page: the list (GET /kernel/node-groups), the
// approval switch of a row (a PUT of the group as it was listed, with the one
// field changed) and deleting a group after a confirmation. The count of
// nodes waiting for approval across every group feeds the page's banner.
import { computed, ref } from 'vue'
import { deleteKernelNodeGroup, listKernelNodeGroups, updateKernelNodeGroup } from '@/api/kernel'
import { useConfirm } from '@/ui/composables/useConfirm'
import { useToast } from '@/ui/composables/useToast'
import { useAppI18n } from '@/composables/useAppI18n'
import { inputFromGroup, readKernelError, readNodeGroups } from './nodeGroupData'

export function useNodeGroups() {
  const { t } = useAppI18n()
  const toast = useToast()
  const confirm = useConfirm()

  const groups = ref([])
  const loading = ref(false)
  const error = ref(null)
  const busy = ref([])
  let sequence = 0

  const pendingTotal = computed(() => groups.value.reduce((sum, group) => sum + Number(group.nodes?.pending || 0), 0))

  async function load() {
    const mine = ++sequence
    loading.value = true
    error.value = null
    try {
      const answer = readNodeGroups(await listKernelNodeGroups())
      if (mine !== sequence) return true
      groups.value = answer
      return true
    } catch (cause) {
      if (mine !== sequence) return false
      console.error('Failed to load node groups:', cause)
      error.value = cause
      groups.value = []
      return false
    } finally {
      if (mine === sequence) loading.value = false
    }
  }

  // Replaces every field of the group, so the group is sent back as listed
  // with the approval flag changed. A group whose token has ended cannot be
  // sent as it is (the API refuses a past expiry): its sheet sets a new one.
  async function toggleApproval(group, value) {
    if (busy.value.includes(group.id)) return
    if (group.token_expired) {
      toast.error(t('admin.nodes.groups.tokenExpiredEdit'))
      return
    }
    busy.value = [...busy.value, group.id]
    try {
      await updateKernelNodeGroup(group.id, inputFromGroup(group, { require_approval: Boolean(value) }))
      toast.success(t(value ? 'admin.nodes.groups.approvalOn' : 'admin.nodes.groups.approvalOff', { name: group.name }))
      await load()
    } catch (cause) {
      toast.error(t('admin.nodes.groupForm.saveFailed', { message: readKernelError(cause).message }))
      await load()
    } finally {
      busy.value = busy.value.filter(id => id !== group.id)
    }
  }

  // Resolves true once deleted. A group with nodes answers 409
  // node_group_in_use, which the confirmation shows as "组内还有节点".
  async function confirmDelete(group) {
    const confirmed = await confirm({
      title: t('admin.nodes.groups.deleteTitle', { name: group.name }),
      message: t('admin.nodes.groups.deleteMessage'),
      confirmLabel: t('admin.nodes.groups.deleteAction'),
      tone: 'danger',
      onConfirm: async () => {
        try {
          await deleteKernelNodeGroup(group.id)
        } catch (cause) {
          const problem = readKernelError(cause)
          if (problem.status === 409 || problem.code === 'node_group_in_use') throw new Error(t('admin.nodes.groups.deleteInUse'))
          throw new Error(problem.message)
        }
      }
    })
    if (!confirmed) return false
    toast.success(t('admin.nodes.groups.deleted', { name: group.name }))
    return true
  }

  return { groups, loading, error, busy, pendingTotal, load, toggleApproval, confirmDelete }
}
