import { useAppI18n } from '@/composables/useAppI18n'
import { useToast } from '@/ui/composables/useToast'
import { countBulkFailures } from '@/utils/bulkResult'

// The result of a bulk action as a toast, the summary pattern of the list
// pages (the forwarding bulk bar does the same):
//
//   all done      success toast (with 撤销 when the action has an inverse)
//   some failed   warning toast: what was done, why the rest was not (a count
//                 per cause), and 重试 for the failed ids worth another try
//   none done     error toast with the same detail
//
// The failed rows stay selected in the table, so which ones they are can be
// read from the list too.
export function useBulkReport() {
  const { t } = useAppI18n()
  const toast = useToast()

  // "2 not found, 1 is your own account": a count per cause. A failure the
  // console has no wording for ends with the server's message.
  function failureText(failed) {
    const details = countBulkFailures(failed)
      .map(({ code, count, message }) => {
        const text = t(`adminBulk.errors.${code}`, { count })
        return code === 'failed' && message ? `${text} (${message})` : text
      })
      .join(t('adminBulk.separator'))
    return details ? t('adminBulk.list', { details }) : ''
  }

  // report({ done, failed, retryable }, { success, partial, none, undo, retry })
  // success / partial / none are the leading sentences (partial and none get
  // the failure detail appended); undo and retry are callbacks.
  function report(outcome, { success, partial, none, undo, retry }) {
    const { done, failed, retryable } = outcome
    if (!failed.length) {
      toast.success(success, undo && done.length ? { undo } : undefined)
      return
    }
    const detail = failureText(failed)
    const action = retry && retryable.length ? { label: t('adminBulk.retry', { count: retryable.length }), onAction: retry } : undefined
    toast.show({
      tone: done.length ? 'warning' : 'error',
      message: `${done.length ? partial : none} ${detail}`.trim(),
      duration: 0,
      action
    })
  }

  return { failureText, report }
}
