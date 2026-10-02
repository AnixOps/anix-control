// Copy to the clipboard and say how it went (copyText falls back to
// execCommand('copy') on plain HTTP).
import { useAppI18n } from '@/composables/useAppI18n'
import { copyText } from '@/ui/composables/useClipboard'
import { useToast } from '@/ui/composables/useToast'

export function useNodeCopy() {
  const { t } = useAppI18n()
  const toast = useToast()
  return async function copyWithToast(text) {
    if (await copyText(text)) toast.success(t('admin.nodes.messages.copied'))
    else toast.error(t('admin.nodes.messages.copyFailedManual'))
  }
}
