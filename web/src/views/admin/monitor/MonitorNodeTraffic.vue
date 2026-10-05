<template>
  <UiSheet
    :open="open"
    size="lg"
    :title="t('adminMonitor.live.traffic.title', { name: label })"
    :description="t('adminMonitor.live.traffic.description')"
    data-testid="monitor-node-traffic"
    @update:open="value => emit('update:open', value)"
  >
    <NodeTrafficChart v-if="open" :key="node.id" :node-id="node.id" :node-name="label" :chart-height="260" />
    <template #footer="{ close }">
      <UiButton :as="RouterLink" :to="`/admin/nodes/${node.id}?section=traffic`" :icon="SquareArrowOutUpRight" data-testid="monitor-node-open">
        {{ t('adminMonitor.live.traffic.openNode') }}
      </UiButton>
      <UiButton variant="primary" @click="close">{{ t('common.actions.close') }}</UiButton>
    </template>
  </UiSheet>
</template>

<script setup>
// The traffic of one node of 实时节点, in a sheet next to the live table: the
// node's upload and download over time (the node page's chart, from
// GET /api/v4/kernel/nodes/:id/traffic). The live table only has the node's
// load now; this is its history. Loads when the first row asks for it.
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import { SquareArrowOutUpRight } from '@lucide/vue'
import UiButton from '@/ui/UiButton.vue'
import UiSheet from '@/ui/UiSheet.vue'
import { useAppI18n } from '@/composables/useAppI18n'
import NodeTrafficChart from '../nodes/NodeTrafficChart.vue'

const props = defineProps({
  open: { type: Boolean, default: false },
  // A row of the live table: { id, name, host }.
  node: { type: Object, required: true }
})
const emit = defineEmits(['update:open'])
const { t } = useAppI18n()

const label = computed(() => props.node.name || `#${props.node.id}`)
</script>
