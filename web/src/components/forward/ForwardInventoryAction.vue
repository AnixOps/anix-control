<template>
  <UiButton v-if="state === 'member'" :icon="Waypoints" data-testid="forward-inventory-link" @click="router.push(`/admin/forward/inventory/${nodeRef}`)">{{ t('forwardV4.join.settings') }}</UiButton>
  <UiButton v-else-if="state === 'outside'" :icon="Waypoints" :loading="joining" data-testid="forward-inventory-join" @click="join">{{ t('forwardV4.join.action') }}</UiButton>
</template>

<script setup>
// 加入转发清单… on a proxy node's page (D15): a proxy node joins the
// forwarding inventory through its forwarding settings (PUT
// /nodes/proxy-<id>/settings with the defaults). Once it is in, the button
// leads to its 转发设置.
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Waypoints } from '@lucide/vue'
import { UiButton, useConfirm, useToast } from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import { listNodes, newIdempotencyKey, setNodeSettings } from '@/api/forwardV4'
import { forwardErrorMessage } from './messages'

const props = defineProps({
  nodeRef: { type: String, required: true },
  name: { type: String, default: '' }
})
const router = useRouter()
const { t } = useAppI18n()
const confirm = useConfirm()
const toast = useToast()
// '' while unknown, then 'member' or 'outside'.
const state = ref('')
const joining = ref(false)

onMounted(async () => {
  try {
    const { nodes } = await listNodes({ kind: 'proxy' })
    const node = nodes.find(item => item.node_ref === props.nodeRef)
    state.value = node && node.in_inventory !== false ? 'member' : 'outside'
  } catch {
    state.value = ''
  }
})

async function join() {
  await confirm({
    title: t('forwardV4.join.title', { name: props.name || props.nodeRef }),
    message: t('forwardV4.join.message'),
    confirmLabel: t('forwardV4.join.confirm'),
    onConfirm: async () => {
      joining.value = true
      try {
        await setNodeSettings(props.nodeRef, {}, { idempotencyKey: newIdempotencyKey() })
      } catch (error) {
        throw new Error(forwardErrorMessage(t, error))
      } finally {
        joining.value = false
      }
      toast.success(t('forwardV4.join.done', { name: props.name || props.nodeRef }))
      router.push(`/admin/forward/inventory/${props.nodeRef}`)
    }
  })
}
</script>
