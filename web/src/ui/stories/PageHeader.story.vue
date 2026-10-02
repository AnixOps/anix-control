<script setup>
import { ChevronLeft, Download, Plus } from '@lucide/vue'
import UiPageHeader from '../UiPageHeader.vue'
import UiButton from '../UiButton.vue'
import UiBadge from '../UiBadge.vue'
</script>

<template>
  <Story title="PageHeader" group="layout" :layout="{ type: 'single', iframe: true }">
    <Variant title="List page">
      <div class="story">
        <UiPageHeader title="节点" description="代理节点与它们的协议、凭据和部署状态。">
          <template #actions>
            <UiButton :icon="Download">导出</UiButton>
            <UiButton variant="primary" :icon="Plus">添加节点</UiButton>
          </template>
        </UiPageHeader>
      </div>
    </Variant>
    <Variant title="Detail page">
      <div class="story">
        <UiPageHeader title="hk-hkg-01" description="香港 · 203.0.113.10 · Agent 4.1.0">
          <template #back>
            <UiButton variant="tertiary" size="sm" :icon="ChevronLeft" href="#">节点</UiButton>
          </template>
          <template #meta>
            <UiBadge status="online" />
          </template>
          <template #actions>
            <UiButton>诊断</UiButton>
            <UiButton variant="primary">编辑</UiButton>
          </template>
        </UiPageHeader>
      </div>
    </Variant>
  </Story>
</template>

<docs lang="md">
# PageHeader

The page's only H1 (Title 1), one sentence, and the actions — secondary
first, primary last (rightmost). Slots: `back`, `meta` (status badge next to
the title), `description`, `actions`. Actions wrap under the title on
phones.
</docs>
