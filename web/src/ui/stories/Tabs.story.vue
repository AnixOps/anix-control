<script setup>
import { ref } from 'vue'
import UiTabs from '../UiTabs.vue'

const section = ref('overview')
const suite = ref('rules')
const items = [
  { value: 'overview', label: '概览' },
  { value: 'protocols', label: '协议', count: 4 },
  { value: 'credentials', label: '凭据' },
  { value: 'logs', label: '日志' },
  { value: 'danger', label: '危险操作' }
]
</script>

<template>
  <Story title="Tabs" group="layout" :layout="{ type: 'single', iframe: true }">
    <Variant title="Underline">
      <div class="story">
        <UiTabs v-model="section" aria-label="节点详情" :items="items">
          <template #overview><p>健康状态、流量与最近心跳。</p></template>
          <template #protocols><p>4 个协议。</p></template>
          <template #credentials><p>凭据默认遮罩。</p></template>
          <template #logs><p>最近 200 行日志。</p></template>
          <template #danger><p>禁用或删除节点。</p></template>
        </UiTabs>
      </div>
    </Variant>
    <Variant title="Segmented">
      <div class="story">
        <UiTabs v-model="suite" variant="segmented" aria-label="转发" :items="[{ value: 'rules', label: '转发规则' }, { value: 'tunnels', label: '隧道' }, { value: 'limits', label: '限速' }]">
          <template #rules><p>转发规则列表。</p></template>
          <template #tunnels><p>隧道列表。</p></template>
          <template #limits><p>限速规则。</p></template>
        </UiTabs>
      </div>
    </Variant>
  </Story>
</template>

<docs lang="md">
# Tabs

Switch between panels of one page. Reka Tabs: `tablist` / `tab` /
`tabpanel` with `aria-selected` and `aria-controls`; ←/→ move and activate,
Home/End jump, Tab moves into the panel. `underline` for detail-page
sections, `segmented` for the pill look at the top of a page. Panels are
slots named after each value.
</docs>
