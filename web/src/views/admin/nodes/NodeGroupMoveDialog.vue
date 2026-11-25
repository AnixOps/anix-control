<template>
  <UiConfirmDialog
    :open="open"
    :title="nodes.length === 1 ? t('admin.nodes.move.titleOne', { name: nodes[0].name }) : t('admin.nodes.move.titleMany', { count: nodes.length })"
    :message="t('admin.nodes.move.message')"
    :confirm-label="t('admin.nodes.move.action')"
    :loading="saving"
    :error="error"
    data-testid="node-group-move"
    @confirm="submit"
    @cancel="emit('update:open', false)"
  >
    <UiSelect v-model="target" size="md" :label="t('admin.nodes.move.target')" :options="options" data-testid="node-group-move-target" />
    <p v-if="relayed" class="move-note" data-testid="node-group-move-relays">{{ t('admin.nodes.move.relays') }}</p>
  </UiConfirmDialog>
</template>

<script setup>
// 移动到分组: puts the chosen nodes into a node group, or takes them out of
// every group (POST /kernel/node-groups/move). The group's join defaults are
// not applied; a group with relays changes how its nodes reach the panel,
// which the dialog says before the move.
import { computed, ref, watch } from 'vue'
import UiConfirmDialog from '@/ui/UiConfirmDialog.vue'
import UiSelect from '@/ui/UiSelect.vue'
import { useToast } from '@/ui/composables/useToast'
import { useAppI18n } from '@/composables/useAppI18n'
import { moveKernelNodesToGroup } from '@/api/kernel'
import { nodeRef } from './agentConnection'
import { readKernelError } from './nodeGroupData'

const NONE = 'none'

const props = defineProps({
  open: { type: Boolean, default: false },
  // The rows of the node list to move.
  nodes: { type: Array, default: () => [] },
  groups: { type: Array, default: () => [] }
})
const emit = defineEmits(['update:open', 'moved'])
const { t } = useAppI18n()
const toast = useToast()

const target = ref(NONE)
const saving = ref(false)
const error = ref('')

watch(() => props.open, open => {
  if (!open) return
  const current = props.nodes.length === 1 ? props.nodes[0].node_group_id : null
  target.value = current ? String(current) : NONE
  error.value = ''
})

const options = computed(() => [
  ...props.groups.map(group => ({ value: String(group.id), label: group.name })),
  { value: NONE, label: t('admin.nodes.move.none') }
])
const chosen = computed(() => props.groups.find(group => String(group.id) === target.value) || null)
const relayed = computed(() => Boolean(chosen.value?.relays?.length))

async function submit() {
  saving.value = true
  error.value = ''
  try {
    await moveKernelNodesToGroup(props.nodes.map(node => nodeRef(node.id)), chosen.value ? chosen.value.id : null)
    toast.success(chosen.value
      ? t('admin.nodes.move.moved', { count: props.nodes.length, group: chosen.value.name }, props.nodes.length)
      : t('admin.nodes.move.removed', { count: props.nodes.length }, props.nodes.length))
    emit('update:open', false)
    emit('moved')
  } catch (cause) {
    error.value = readKernelError(cause).message
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.move-note {
  margin: var(--space-2) 0 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}
</style>
