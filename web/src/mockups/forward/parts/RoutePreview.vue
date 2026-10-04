<template>
  <UiCard title="规划预览" heading-tag="h2">
    <template #actions>
      <UiIconButton :icon="RefreshCw" label="重新预览" size="sm" />
    </template>
    <div class="preview">
      <p class="preview__meta">
        <UiBadge v-if="preview.violations.length" tone="danger" :label="`${preview.violations.length} 个问题 · 未规划`" />
        <UiBadge v-else tone="success" label="可以规划" />
        <span class="fwd-muted">POST /routes/preview · 停止输入 1 秒后自动预览</span>
      </p>

      <template v-if="preview.violations.length">
        <p class="fwd-muted">规划器拒绝整条路由时不分配任何端口，节点保持当前代。处理左侧标红的问题后这里显示每个节点将收到的状态。</p>
        <ul class="preview__violations">
          <li v-for="(item, index) in preview.violations" :key="index">
            <span class="fwd-mono">{{ item.code }}</span>
            <span class="fwd-muted">{{ item.message }}</span>
          </li>
        </ul>
      </template>

      <template v-else>
        <dl class="preview__totals">
          <div><dt>节点</dt><dd>{{ nodeCount }}</dd></div>
          <div><dt>端口</dt><dd>{{ preview.allocations.length }}</dd></div>
          <div><dt>代变化</dt><dd>{{ nodeCount }} 个节点 +1</dd></div>
        </dl>
        <ol class="preview__states">
          <li v-for="state in preview.states" :key="`${state.node_ref}-${state.hop_index}`" class="preview__state">
            <div class="preview__state-head">
              <span class="preview__node">{{ state.name }}</span>
              <span class="fwd-muted">第 {{ state.hop_index + 1 }} 跳 · {{ ROLE[state.role] }}</span>
              <EngineChip :engine="state.engine" />
            </div>
            <dl class="preview__fields">
              <dt>监听</dt><dd class="fwd-mono">{{ state.listen }}<template v-if="state.hop_index > 0"> · {{ SECURITIES[state.ingress] }}</template></dd>
              <dt>上游</dt>
              <dd>
                <span v-for="up in state.upstreams" :key="up.label" class="preview__up fwd-mono">P{{ up.priority }} {{ up.label }}<template v-if="up.security !== 'LINK_SECURITY_RAW'"> · {{ SECURITIES[up.security] }}</template></span>
              </dd>
              <dt>标记 / 代</dt><dd class="fwd-mono">{{ state.mark }} · {{ state.generation }}</dd>
            </dl>
          </li>
        </ol>
      </template>

      <div v-if="preview.warnings.length" class="preview__warnings">
        <p v-for="warning in preview.warnings" :key="warning" class="fwd-note is-warning">{{ warning }}</p>
      </div>

      <details class="preview__json">
        <summary>请求 JSON（protojson）</summary>
        <UiCodeBlock :code="json" label="PlanRouteRequest.route" max-height="280px" />
      </details>
    </div>
  </UiCard>
</template>

<script setup>
import { computed } from 'vue'
import { RefreshCw } from '@lucide/vue'
import { UiBadge, UiCard, UiCodeBlock, UiIconButton } from '@/ui'
import EngineChip from './EngineChip.vue'
import { SECURITIES } from '../mockData'

const props = defineProps({
  preview: { type: Object, required: true },
  draft: { type: Object, required: true }
})

const ROLE = { HOP_ROLE_ENTRY: '入口', HOP_ROLE_RELAY: '中转', HOP_ROLE_EXIT: '出口' }
const nodeCount = computed(() => new Set(props.preview.states.map(state => state.node_ref)).size)

// The request body, without UI-only keys and with unset fields left out.
const json = computed(() => JSON.stringify(props.draft, (key, value) => {
  if (key.startsWith('_') || key === 'id' || key === 'created_at_unix_ms' || key === 'updated_at_unix_ms') return undefined
  if (value === '' || value === 0 || value === null) return undefined
  if (value && typeof value === 'object' && !Array.isArray(value) && Object.keys(value).length === 0) return undefined
  return value
}, 2))
</script>

<style scoped>
.preview {
  display: grid;
  gap: var(--space-3);
}

.preview__meta {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
  margin: 0;
}

.preview__violations,
.preview__states {
  display: grid;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}

.preview__violations li {
  display: grid;
  gap: 2px;
  padding: var(--space-2);
  border-left: 2px solid var(--danger);
  background: var(--fill-1);
  overflow-wrap: anywhere;
}

.preview__totals {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: var(--space-2);
  margin: 0;
}

.preview__totals div {
  padding: var(--space-2);
  border-radius: var(--radius-xs);
  background: var(--fill-1);
}

.preview__totals dt {
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

.preview__totals dd {
  margin: 0;
  font-weight: var(--weight-semibold);
}

.preview__state {
  display: grid;
  gap: var(--space-2);
  padding: var(--space-3);
  border: 1px solid var(--separator);
  border-radius: var(--radius-sm);
}

.preview__state-head {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
}

.preview__node {
  font-weight: var(--weight-semibold);
}

.preview__fields {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  gap: var(--space-1) var(--space-3);
  margin: 0;
}

.preview__fields dt {
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

.preview__fields dd {
  display: grid;
  gap: 2px;
  margin: 0;
  overflow-wrap: anywhere;
}

.preview__warnings {
  display: grid;
  gap: var(--space-2);
}

.preview__json summary {
  color: var(--accent);
  cursor: pointer;
}

.preview__json[open] summary {
  margin-bottom: var(--space-2);
}
</style>
