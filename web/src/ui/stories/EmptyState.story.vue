<script setup>
import { CloudOff, Plus, RefreshCw, Server } from '@lucide/vue'
import UiEmptyState from '../UiEmptyState.vue'
import UiButton from '../UiButton.vue'
import UiCard from '../UiCard.vue'
</script>

<template>
  <Story title="EmptyState" group="display" :layout="{ type: 'single', iframe: true }">
    <Variant title="Empty and error">
      <div class="story story-grouped">
        <UiCard :padded="false">
          <UiEmptyState :icon="Server" title="还没有节点" description="添加第一个节点后，用户就能使用订阅。">
            <template #actions>
              <UiButton variant="primary" :icon="Plus">添加节点</UiButton>
            </template>
          </UiEmptyState>
        </UiCard>
        <UiCard :padded="false">
          <UiEmptyState :icon="CloudOff" title="无法加载节点" description="请求超时（request id 7f3k9q2m）。检查网络后重试。" compact>
            <template #actions>
              <UiButton :icon="RefreshCw">重试</UiButton>
              <UiButton variant="tertiary">复制错误详情</UiButton>
            </template>
          </UiEmptyState>
        </UiCard>
      </div>
    </Variant>
  </Story>
</template>

<docs lang="md">
# EmptyState

Say what will appear here and offer the first action. For a failed load,
explain what happened, offer "重试" and "复制错误详情" (with the request id).
</docs>
