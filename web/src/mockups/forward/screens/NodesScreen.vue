<template>
  <section class="fwd-screen">
    <UiPageHeader title="节点" description="转发清单里的节点：转发节点和加入转发的代理节点。角色属于跳，同一个节点可以是一条路由的入口、另一条的出口。">
      <template #actions>
        <UiButton :icon="Terminal" @click="installOpen = true">安装 Agent</UiButton>
        <UiButton variant="primary" :icon="Plus">添加转发节点</UiButton>
      </template>
    </UiPageHeader>

    <UiDataTable
      :columns="columns"
      :rows="rows"
      row-key="node_ref"
      label="节点"
      :row-label="row => row.name"
      :row-actions="rowActions"
      :filtered="filtered"
      activatable
      :card-fields="4"
      @row-activate="row => router.push(`${BASE}/node?ref=${row.node_ref}`)"
      @clear-filters="kind = ''; flag = ''"
    >
      <template #toolbar>
        <div class="nodes-toolbar">
          <UiFilterChips v-model="kind" label="类型" :options="kindOptions" />
          <UiFilterChips v-model="flag" label="需要处理" :options="flagOptions" />
        </div>
      </template>

      <template #cell-name="{ row }">
        <span class="fwd-cell-stack">
          <span class="nodes-name">{{ row.name }}</span>
          <span class="fwd-muted">{{ row.node_ref }} · {{ row._region }}</span>
        </span>
      </template>
      <template #cell-status="{ row }">
        <UiBadge :status="row.statusKey" />
      </template>
      <template #cell-kind="{ row }">
        <span class="fwd-cell-stack">
          <span>{{ row.kind === 'proxy' ? '代理节点' : '转发节点' }}</span>
          <span class="fwd-muted">{{ row._transport === 'NODE_TRANSPORT_ANSIBLE' ? 'Ansible（无 Agent）' : 'Agent · mTLS' }}</span>
        </span>
      </template>
      <template #cell-agent="{ row }">
        <span class="fwd-cell-stack">
          <span class="fwd-mono">{{ row.capabilities.agent_version || '—' }}</span>
          <span v-if="row.capabilities.agent_version && row.capabilities.agent_version !== '4.2.0'" class="nodes-warn">可升级到 4.2.0</span>
        </span>
      </template>
      <template #cell-engines="{ row }">
        <span class="nodes-engines">
          <EngineChip
            v-for="engine in row.info.engines"
            :key="engine.engine"
            :engine="engine.engine"
            :unavailable="!engine.available"
            :title="engine.available ? `${engine.version}：${engine.link_securities.map(s => SECURITIES[s]).join(' / ')}` : engine.unavailable_reason"
          />
        </span>
      </template>
      <template #cell-generation="{ row }">
        <span class="fwd-cell-stack nodes-end">
          <span class="fwd-mono">{{ row.reported_generation || '—' }} / {{ row.desired_generation }}</span>
          <UiBadge v-if="row.lag > 0" tone="info" :label="`落后 ${row.lag} 代`" />
          <span v-else-if="row.reported" class="fwd-muted">已收敛</span>
          <span v-else class="fwd-muted">未上报</span>
        </span>
      </template>
      <template #cell-errors="{ row }">
        <UiBadge v-if="row.hop_errors" tone="danger" :label="`${row.hop_errors} 个跳错误`" />
        <span v-else class="fwd-muted">—</span>
      </template>
    </UiDataTable>

    <p class="list-page__note">代 = 节点已应用的状态代 / Control 期望的代。节点每次上报时更新；本页每 15 秒刷新一次。</p>

    <AgentInstallSheet v-model:open="installOpen" node="forward-81" node-label="fra-exit-01" />
  </section>
</template>

<script setup>
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Plus, Terminal, Settings2, Power, Trash2, RotateCcw } from '@lucide/vue'
import { UiBadge, UiButton, UiDataTable, UiFilterChips, UiPageHeader } from '@/ui'
import AgentInstallSheet from '@/components/admin/AgentInstallSheet.vue'
import EngineChip from '../parts/EngineChip.vue'
import { NODES, SECURITIES } from '../mockData'

const BASE = '/admin/__mockups/forward'
const router = useRouter()
const installOpen = ref(false)
const kind = ref('')
const flag = ref('')

const allRows = NODES.map(node => ({
  ...node,
  statusKey: !node.enabled ? 'disabled' : (node._online ? 'online' : 'offline'),
  lag: node.reported ? Number(node.desired_generation) - Number(node.reported_generation) : 0
}))

const rows = computed(() => allRows.filter(row => {
  if (kind.value && row.kind !== kind.value) return false
  if (flag.value === 'lagging' && row.lag <= 0) return false
  if (flag.value === 'errors' && !row.hop_errors) return false
  return true
}))
const filtered = computed(() => Boolean(kind.value || flag.value))

const kindOptions = [
  { value: 'forward', label: '转发节点', count: allRows.filter(row => row.kind === 'forward').length },
  { value: 'proxy', label: '代理节点', count: allRows.filter(row => row.kind === 'proxy').length }
]
const flagOptions = [
  { value: 'lagging', label: '同步落后', count: allRows.filter(row => row.lag > 0).length },
  { value: 'errors', label: '有跳错误', count: allRows.filter(row => row.hop_errors).length }
]

const columns = [
  { key: 'name', label: '节点', primary: true, sortable: true },
  { key: 'status', label: '状态', secondary: true },
  { key: 'kind', label: '类型与通道' },
  { key: 'agent', label: 'Agent', breakpoint: 'md' },
  { key: 'engines', label: '引擎' },
  { key: 'desired_hops', label: '跳数', align: 'end', numeric: true, sortable: true, breakpoint: 'lg' },
  { key: 'generation', label: '代（已应用 / 期望）', align: 'end', sortable: true, sortValue: row => row.lag },
  { key: 'errors', label: '跳错误', sortable: true, sortValue: row => row.hop_errors }
]

function rowActions(row) {
  return [
    { key: 'settings', label: '转发设置', icon: Settings2, onSelect: () => router.push(`${BASE}/node?ref=${row.node_ref}`) },
    { key: 'install', label: '安装或重装 Agent', icon: Terminal, onSelect: () => { installOpen.value = true } },
    { key: 'reset', label: '重置节点状态…', icon: RotateCcw, onSelect: () => {} },
    { key: 'toggle', label: row.enabled ? '禁用…' : '启用', icon: Power, separatorBefore: true, onSelect: () => {} },
    { key: 'delete', label: '删除节点…', icon: Trash2, danger: true, onSelect: () => {} }
  ]
}
</script>

<style scoped>
.nodes-toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-4);
  align-items: center;
}

.nodes-name {
  font-weight: var(--weight-medium);
}

.nodes-engines {
  display: inline-flex;
  flex-wrap: wrap;
  gap: var(--space-1);
}

.nodes-end {
  justify-items: end;
}

.nodes-warn {
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

@media (max-width: 639.98px) {
  .nodes-end {
    justify-items: start;
  }
}
</style>
