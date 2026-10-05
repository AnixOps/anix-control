<template>
  <UiSection :title="t('admin.nodes.traffic.title')" :description="t('admin.nodes.traffic.description')">
    <NodeTrafficChart :node-id="node.id" :node-name="node.name" />
  </UiSection>
</template>

<script setup>
// 流量 section of the node page: the node's upload and download over time
// (GET /api/v4/kernel/nodes/:id/traffic). Loads when the section opens.
import UiSection from '@/ui/UiSection.vue'
import { useAppI18n } from '@/composables/useAppI18n'
import NodeTrafficChart from './NodeTrafficChart.vue'

defineProps({
  node: { type: Object, required: true }
})
const { t } = useAppI18n()
</script>
