// Row and detail actions of an Ansible machine: TCP reachability check,
// stats sync, enable/disable and delete (typed name). Same calls and
// messages as the U6 list page; results stay next to the machine.
import { reactive, ref } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { checkAnsibleMachine, deleteAnsibleMachine, syncAnsibleMachineStats, toggleAnsibleMachine } from '@/api/admin'
import { useConfirm } from '@/ui/composables/useConfirm'
import { useToast } from '@/ui/composables/useToast'
import { unwrapPayload } from './forwardNodeModel'

export function useAnsibleMachineActions({ onChanged = () => {} } = {}) {
  const { t, translateLiteral } = useAppI18n()
  const toast = useToast()
  const confirm = useConfirm()

  const pendingAction = ref('')
  const results = reactive({})

  function translateRuntimeText(value, fallback = '-') {
    const text = String(value ?? '').trim()
    return text ? translateLiteral(text) : fallback
  }

  function resolveRuntimeError(error, fallbackKey) {
    return translateRuntimeText(error?.response?.data?.msg || error?.message, t(fallbackKey))
  }

  function isPending(id) {
    return pendingAction.value.startsWith(`${id}:`)
  }

  function pendingLabel(machine) {
    const action = pendingAction.value.split(':')[1]
    if (action === 'check') return t('runtime.ansibleMachines.actions.checking')
    if (action === 'sync') return t('runtime.ansibleMachines.actions.syncing')
    return t('runtime.ansibleMachines.actions.updating', { name: machine.name })
  }

  async function checkMachine(machine) {
    pendingAction.value = `${machine.id}:check`
    try {
      const payload = unwrapPayload(await checkAnsibleMachine(machine.id))
      const success = Number(payload?.status ?? 0) === 1 && !payload?.error
      results[machine.id] = {
        success,
        message: success
          ? (payload?.latency
            ? t('runtime.ansibleMachines.results.latency', { value: payload.latency })
            : t('runtime.ansibleMachines.results.reachable'))
          : translateRuntimeText(payload?.error, t('runtime.ansibleMachines.results.unavailable'))
      }
      await onChanged('check')
    } catch (error) {
      results[machine.id] = { success: false, message: resolveRuntimeError(error, 'runtime.ansibleMachines.errors.checkFailed') }
    } finally {
      pendingAction.value = ''
    }
  }

  async function syncMachine(machine) {
    pendingAction.value = `${machine.id}:sync`
    try {
      const payload = unwrapPayload(await syncAnsibleMachineStats(machine.id))
      results[machine.id] = { success: true, message: translateRuntimeText(payload?.message, t('runtime.ansibleMachines.results.synced')) }
      await onChanged('sync')
    } catch (error) {
      results[machine.id] = { success: false, message: resolveRuntimeError(error, 'runtime.ansibleMachines.errors.syncFailed') }
    } finally {
      pendingAction.value = ''
    }
  }

  async function toggleMachine(machine) {
    pendingAction.value = `${machine.id}:toggle`
    try {
      await toggleAnsibleMachine(machine.id, !machine.enabled)
      await onChanged('toggle')
    } catch (error) {
      results[machine.id] = { success: false, message: resolveRuntimeError(error, 'runtime.ansibleMachines.errors.toggleFailed') }
    } finally {
      pendingAction.value = ''
    }
  }

  // An execution machine is deleted only after its name is typed.
  async function deleteMachine(machine) {
    if (!machine?.id) return false
    const name = machine.name && machine.name !== '-' ? machine.name : `#${machine.id}`
    const confirmed = await confirm({
      title: t('runtime.ansibleMachines.modal.deleteTitle', { name }),
      message: t('runtime.ansibleMachines.modal.deleteConfirm'),
      confirmLabel: t('runtime.ansibleMachines.modal.deleteAction'),
      tone: 'danger',
      requireText: name,
      onConfirm: async () => {
        try {
          await deleteAnsibleMachine(machine.id)
        } catch (error) {
          throw new Error(resolveRuntimeError(error, 'runtime.ansibleMachines.errors.deleteFailed'))
        }
      }
    })
    if (!confirmed) return false
    toast.success(t('runtime.ansibleMachines.messages.deleted', { name }))
    await onChanged('delete')
    return true
  }

  return { pendingAction, results, isPending, pendingLabel, checkMachine, syncMachine, toggleMachine, deleteMachine, resolveRuntimeError }
}
