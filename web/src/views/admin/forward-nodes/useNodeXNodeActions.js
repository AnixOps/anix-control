// Row and detail actions of a NodeX forward node: health check, stats sync,
// enable/disable and delete. Same calls, scope and messages as before U7;
// the list page and the detail page share them.
import { reactive, ref } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { checkForwardNode, deleteForwardNode, syncForwardNodeStats, toggleForwardNode } from '@/api/admin'
import { useConfirm } from '@/ui/composables/useConfirm'
import { useToast } from '@/ui/composables/useToast'
import { NODEX_SCOPE, errorMessage, unwrapForwardResponse } from './forwardNodeModel'

/**
 * @param {{ onChanged?: (kind: 'check'|'sync'|'toggle'|'delete') => unknown }} options
 *   onChanged runs after a successful action (reload what the page shows).
 */
export function useNodeXNodeActions({ onChanged = () => {} } = {}) {
  const { t, translateLiteral } = useAppI18n()
  const toast = useToast()
  const confirm = useConfirm()

  const pending = ref('')
  // Last check or sync result per node id: { success, message }.
  const results = reactive({})

  const failure = (error, key) => errorMessage(error, t(key), translateLiteral)
  const unwrap = response => unwrapForwardResponse(response, t('runtime.nodeXTopology.validation.requestFailed'))

  function isPending(id, action) {
    return action ? pending.value === `${id}:${action}` : pending.value.startsWith(`${id}:`)
  }

  async function check(node) {
    pending.value = `${node.id}:check`
    try {
      const payload = unwrap(await checkForwardNode(node.id, NODEX_SCOPE))
      const success = Number(payload.status ?? 0) === 1 && !payload.error
      const latency = Number(payload.latency ?? 0)
      const message = success
        ? latency > 0
          ? t('runtime.nodeXTopology.messages.latency', { value: latency })
          : t('runtime.nodeXTopology.messages.nodeReachable')
        : translateLiteral(payload.error || t('runtime.nodeXTopology.messages.nodeUnavailable'))
      results[node.id] = { success, message }
      if (success) toast.success(`${node.name}: ${message}`)
      else toast.error(`${node.name}: ${message}`)
      await onChanged('check')
    } catch (error) {
      const message = failure(error, 'runtime.nodeXTopology.messages.nodeCheckFailed')
      results[node.id] = { success: false, message }
      toast.error(`${node.name}: ${message}`)
    } finally {
      pending.value = ''
    }
  }

  async function sync(node) {
    pending.value = `${node.id}:sync`
    try {
      const payload = unwrap(await syncForwardNodeStats(node.id, NODEX_SCOPE))
      const stats = payload?.stats || null
      const base = translateLiteral(payload?.message || t('runtime.nodeXTopology.messages.syncSuccess'))
      const suffix = stats?.current_conn !== undefined
        ? t('runtime.nodeXTopology.messages.currentConnectionsSuffix', { value: stats.current_conn })
        : ''
      results[node.id] = { success: true, message: `${base}${suffix}` }
      toast.success(`${node.name}: ${base}`)
      await onChanged('sync')
    } catch (error) {
      const message = failure(error, 'runtime.nodeXTopology.messages.nodeSyncFailed')
      results[node.id] = { success: false, message }
      toast.error(`${node.name}: ${message}`)
    } finally {
      pending.value = ''
    }
  }

  async function toggle(node) {
    pending.value = `${node.id}:toggle`
    try {
      await toggleForwardNode(node.id, !node.enabled, NODEX_SCOPE)
      toast.success(t(node.enabled ? 'runtime.nodeXTopology.messages.nodeDisabled' : 'runtime.nodeXTopology.messages.nodeEnabled', { name: node.name }))
      await onChanged('toggle')
    } catch (error) {
      toast.error(failure(error, 'runtime.nodeXTopology.messages.nodeToggleFailed'))
    } finally {
      pending.value = ''
    }
  }

  // A forward node carries rules and runtime state: its name must be typed.
  async function remove(node) {
    if (!node?.id) return false
    const name = node.name || `#${node.id}`
    const confirmed = await confirm({
      title: t('runtime.nodeXTopology.deleteModal.titleNode', { name }),
      message: t('runtime.nodeXTopology.deleteModal.warning'),
      confirmLabel: t('runtime.nodeXTopology.deleteModal.deleteNode'),
      tone: 'danger',
      requireText: name,
      onConfirm: async () => {
        try {
          await deleteForwardNode(node.id, NODEX_SCOPE)
        } catch (error) {
          throw new Error(failure(error, 'runtime.nodeXTopology.messages.deleteFailed'))
        }
      }
    })
    if (!confirmed) return false
    toast.success(t('runtime.nodeXTopology.messages.nodeDeleted'))
    await onChanged('delete')
    return true
  }

  return { pending, results, isPending, check, sync, toggle, remove }
}
