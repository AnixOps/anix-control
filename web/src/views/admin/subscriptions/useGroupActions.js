// Group actions shared by the 订阅分组 list and the group page: copy the
// merged subscription, delete a group. Endpoints unchanged.
import adminApi from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import { copyText } from '@/ui/composables/useClipboard'
import { useConfirm } from '@/ui/composables/useConfirm'
import { useToast } from '@/ui/composables/useToast'
import { ensureSubscriptionSuccess, generateNodeLink, readSubscriptionList, readSubscriptionPayload } from './subscriptionShared'

export function useGroupActions() {
  const { t } = useAppI18n()
  const toast = useToast()
  const confirm = useConfirm()

  async function copyToClipboard(text) {
    if (!text) {
      toast.error(t('adminSubscriptionGroups.messages.copyFailed'))
      return false
    }
    if (await copyText(text)) {
      toast.success(t('adminSubscriptionGroups.messages.copied'))
      return true
    }
    toast.error(t('adminSubscriptionGroups.messages.copyFailed'))
    return false
  }

  // The server's merged result first; without it, the templates' links
  // merged here (base64), as before.
  async function copyGroupCombined(group) {
    const loadFailed = t('adminSubscriptionGroups.messages.loadFailed')
    try {
      const res = await adminApi.previewSubscription({ group_ids: [group.id], format: 'v2ray' })
      const payload = readSubscriptionPayload(res, loadFailed)
      const content = payload?.content || ''
      if (content) {
        await copyToClipboard(content)
        return
      }
    } catch {
      // Fall through to the panel-side fallback merge.
    }

    try {
      const tplRes = await adminApi.getSubscriptionTemplates(group.id)
      const lines = readSubscriptionList(tplRes, loadFailed).map(generateNodeLink).filter(Boolean)
      if (lines.length === 0) {
        toast.error(t('adminSubscriptionGroups.messages.copyFailed'))
        return
      }
      if (await copyText(btoa(lines.join('\n')))) {
        toast.success(t('adminSubscriptionGroups.messages.copyFallback'))
      } else {
        toast.error(t('adminSubscriptionGroups.messages.copyFailed'))
      }
    } catch {
      toast.error(t('adminSubscriptionGroups.messages.copyFailed'))
    }
  }

  // Resolves true when the group was deleted.
  async function deleteGroup(group) {
    const confirmed = await confirm({
      tone: 'danger',
      title: t('adminSubscriptionGroups.confirm.deleteGroupTitle', { name: group.name }),
      message: t('adminSubscriptionGroups.confirm.deleteGroupMessage'),
      confirmLabel: t('adminSubscriptionGroups.confirm.deleteGroupAction'),
      onConfirm: async () => {
        try {
          ensureSubscriptionSuccess(await adminApi.deleteSubscriptionGroup(group.id), t('adminSubscriptionGroups.messages.deleteFailed'))
        } catch (error) {
          throw new Error(error?.message || t('adminSubscriptionGroups.messages.deleteFailed'))
        }
      }
    })
    if (!confirmed) return false
    toast.success(t('adminSubscriptionGroups.messages.groupDeleted'))
    return true
  }

  return { copyToClipboard, copyGroupCombined, deleteGroup }
}
