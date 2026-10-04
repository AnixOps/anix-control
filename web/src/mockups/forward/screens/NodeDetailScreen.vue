<template>
  <section class="fwd-screen">
    <UiPageHeader :title="node.name">
      <template #back>
        <RouterLink class="node-back" :to="`${BASE}/nodes`"><ChevronLeft :size="16" /> 节点</RouterLink>
      </template>
      <template #meta>
        <UiBadge :status="node.enabled ? (node._online ? 'online' : 'offline') : 'disabled'" />
        <UiBadge tone="neutral" :label="node.kind === 'proxy' ? '代理节点' : '转发节点'" />
        <span class="fwd-mono node-ref">{{ node.node_ref }} · {{ node.host }}</span>
      </template>
      <template #actions>
        <UiButton :icon="Terminal" @click="installOpen = true">安装 Agent</UiButton>
        <UiButton variant="primary" :icon="RefreshCw">刷新</UiButton>
      </template>
    </UiPageHeader>

    <div class="node-layout">
      <div class="node-col">
        <UiCard title="健康" :description="`最新上报 ${fmt.relativeTime(Number(node.reported_at_unix_ms), { now: MOCK_NOW })}（${fmt.dateTime(Number(node.reported_at_unix_ms))}）`">
          <div class="node-health">
            <div class="node-health__item">
              <span class="fwd-muted">已应用 / 期望代</span>
              <span class="node-health__value">{{ node.reported_generation }} / {{ node.desired_generation }}</span>
              <UiBadge :tone="lag ? 'info' : 'success'" :label="lag ? `落后 ${lag} 代` : '已收敛'" />
            </div>
            <div class="node-health__item">
              <span class="fwd-muted">状态哈希</span>
              <span class="node-health__value fwd-mono">{{ node.reported_state_hash }}</span>
              <span class="fwd-muted">{{ node.reported_state_hash === node.desired_state_hash ? '与期望一致' : `期望 ${node.desired_state_hash}` }}</span>
            </div>
            <div class="node-health__item">
              <span class="fwd-muted">跳</span>
              <span class="node-health__value">{{ node.desired_hops }}</span>
              <UiBadge :tone="node.hop_errors ? 'danger' : 'success'" :label="node.hop_errors ? `${node.hop_errors} 个错误` : '无错误'" />
            </div>
            <div class="node-health__item">
              <span class="fwd-muted">上游</span>
              <span class="node-health__value">6 / 7</span>
              <span class="fwd-muted">健康 / 总数</span>
            </div>
          </div>
          <ul v-if="errors.length" class="fwd-list node-errors">
            <li v-for="(error, index) in errors" :key="index" class="fwd-list__item">
              <span class="fwd-cell-stack">
                <span><AlertTriangle :size="14" class="node-errors__icon" /> {{ routeName(error.route_id) }} · 第 {{ error.hop_index + 1 }} 跳</span>
                <span class="fwd-mono node-errors__msg">{{ error.message }}</span>
              </span>
              <UiButton size="sm" @click="router.push(`${BASE}/route?id=${error.route_id}`)">查看路由</UiButton>
            </li>
          </ul>
        </UiCard>

        <UiCard title="承载的跳" description="节点期望状态里的跳（GET /nodes/{ref} 的 state.hops）。">
          <UiDataTable :columns="hopColumns" :rows="hops" label="承载的跳" row-key="key" flat :settings="false" :sticky-header="false">
            <template #cell-route_name="{ row }">
              <RouterLink class="node-link" :to="`${BASE}/route?id=${row.route_id}`">{{ row.route_name }}</RouterLink>
            </template>
            <template #cell-engine="{ row }"><EngineChip :engine="row.engine" /></template>
            <template #cell-state="{ row }">
              <UiBadge :tone="row.paused ? 'neutral' : 'success'" :label="row.paused ? '已暂停' : '运行'" />
            </template>
          </UiDataTable>
        </UiCard>

        <UiCard title="引擎能力" :description="`Agent ${node.capabilities.agent_version || '—'} 在连接时上报 · 内核 ${node.capabilities.kernel_version} · cgroup ${node.capabilities.cgroup}`">
          <ul class="fwd-list">
            <li v-for="engine in node.info.engines" :key="engine.engine" class="fwd-list__item">
              <span class="fwd-cell-stack">
                <span class="node-engine-head"><EngineChip :engine="engine.engine" :unavailable="!engine.available" /> <span class="fwd-mono">{{ engine.version }}</span></span>
                <span class="fwd-muted">链路：{{ engine.link_securities.map(s => SECURITIES[s]).join(' / ') }}{{ engine.engine === 'ENGINE_GOST' ? ' · 可多路复用' : '' }}</span>
                <span class="fwd-muted">均衡：{{ engine.strategies.map(s => STRATEGIES[s].label).join('、') }}</span>
              </span>
              <span class="node-caps">
                <UiBadge v-if="!engine.available" tone="danger" :label="engine.unavailable_reason" />
                <template v-else>
                  <UiBadge v-for="cap in capsOf(engine)" :key="cap" tone="neutral" :dot="false" :label="cap" />
                </template>
              </span>
            </li>
          </ul>
        </UiCard>
      </div>

      <div class="node-col">
        <UiCard title="转发设置" description="规划器从这里分配端口和地址。修改后所有路由重新规划；无法规划的路由会列出原因，节点保持当前代。">
          <form class="node-form" @submit.prevent>
            <div class="node-form__pair">
              <UiNumberField v-model="rangeFirst" size="md" label="端口范围起" :min="1" :max="65535" />
              <UiNumberField v-model="rangeLast" size="md" label="端口范围止" :min="1" :max="65535" />
            </div>
            <UiTextField v-model="reserved" size="md" label="保留端口" help="逗号分隔；规划器永不分配，路由也不能指定。" />
            <UiTextarea v-model="addresses" label="地址" :rows="2" help="每行一个 IP，按顺序作为上一跳拨号的地址；空则用节点主机地址。" />
            <UiField label="标签" help="例如 link=iepl 标记可信专线；路由筛选和规则可以引用。">
              <template #default>
                <span class="node-labels">
                  <span v-for="(value, key) in node.settings.labels" :key="key" class="node-label">{{ key }}={{ value }}<X :size="12" aria-hidden="true" /></span>
                  <UiButton size="sm" variant="tertiary" :icon="Plus">添加标签</UiButton>
                </span>
              </template>
            </UiField>
            <p class="fwd-note">保存前会重新规划并列出受影响的跳：范围缩小或新增保留端口时，已分配的端口可能被迫移动（规划警告），节点生成新的一代。</p>
            <div class="node-form__actions">
              <UiButton>放弃</UiButton>
              <UiButton variant="primary" type="submit">保存设置</UiButton>
            </div>
          </form>
        </UiCard>

        <UiCard title="Agent">
          <UiGroupedList>
            <UiGroupedListRow label="通道" :value="node._transport === 'NODE_TRANSPORT_ANSIBLE' ? 'Ansible（无 Agent）' : 'Agent · mTLS'" />
            <UiGroupedListRow label="版本" :value="node.capabilities.agent_version || '—'" />
            <UiGroupedListRow label="forward.v1 协商" :value="node.negotiated ? '已协商' : '未协商'" />
          </UiGroupedList>
          <p class="fwd-muted node-install">安装、重装或迁移节点：生成一次性安装命令（与节点页的安装面板相同）。</p>
          <UiButton :icon="Terminal" @click="installOpen = true">生成安装命令</UiButton>
        </UiCard>

        <UiCard title="危险操作">
          <p class="fwd-note">{{ node.name }} 被 {{ usedBy.length }} 条路由使用（{{ usedBy.map(item => item.route.name).join('、') }}），无法禁用或删除。先把这些路由迁到其他节点。</p>
          <div class="node-danger">
            <UiButton variant="danger-soft" :icon="Power" disabled>禁用节点…</UiButton>
            <UiButton variant="danger-soft" :icon="Trash2" disabled>删除节点…</UiButton>
          </div>
        </UiCard>
      </div>
    </div>

    <AgentInstallSheet v-model:open="installOpen" :node="node.node_ref" :node-label="node.name" />
  </section>
</template>

<script setup>
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { AlertTriangle, ChevronLeft, Plus, Power, RefreshCw, Terminal, Trash2, X } from '@lucide/vue'
import { UiBadge, UiButton, UiCard, UiDataTable, UiField, UiGroupedList, UiGroupedListRow, UiNumberField, UiPageHeader, UiTextarea, UiTextField, useFormat } from '@/ui'
import AgentInstallSheet from '@/components/admin/AgentInstallSheet.vue'
import EngineChip from '../parts/EngineChip.vue'
import { HOP_ERRORS, MOCK_NOW, NODES, SECURITIES, STRATEGIES, hopsOnNode, nodeByRef, routeById, routesUsingNode } from '../mockData'

const BASE = '/admin/__mockups/forward'
const router = useRouter()
const fmt = useFormat()
const node = nodeByRef(useRoute().query.ref) || NODES[2]
const installOpen = ref(false)
const lag = Number(node.desired_generation) - Number(node.reported_generation || 0)
const errors = HOP_ERRORS[node.node_ref] || []
const usedBy = routesUsingNode(node.node_ref)

const rangeFirst = ref(node.settings.port_range.first)
const rangeLast = ref(node.settings.port_range.last)
const reserved = ref(node.settings.reserved_ports.join(', '))
const addresses = ref(node.settings.addresses.join('\n'))

const ROLE = { HOP_ROLE_ENTRY: '入口', HOP_ROLE_RELAY: '中转', HOP_ROLE_EXIT: '出口' }
const hops = hopsOnNode(node.node_ref).map(hop => ({ ...hop, key: `${hop.route_id}-${hop.hop_index}`, roleText: `${ROLE[hop.role]}（第 ${hop.hop_index + 1} 跳）` }))
const hopColumns = [
  { key: 'route_name', label: '路由', primary: true },
  { key: 'roleText', label: '角色', secondary: true },
  { key: 'engine', label: '引擎' },
  { key: 'port', label: '端口', align: 'end', numeric: true },
  { key: 'mark', label: '标记', align: 'end', numeric: true, breakpoint: 'md' },
  { key: 'state', label: '状态' }
]

function routeName(id) {
  return routeById(id)?.route.name || id
}

function capsOf(engine) {
  return [
    engine.udp && 'UDP',
    engine.ipv6 && 'IPv6',
    engine.bandwidth_limit && '限速',
    engine.quota && '配额',
    engine.max_conns && '连接数'
  ].filter(Boolean)
}
</script>

<style scoped>
.node-back,
.node-link {
  display: inline-flex;
  gap: var(--space-1);
  align-items: center;
  color: var(--accent);
  text-decoration: none;
}

.node-ref {
  color: var(--label-3);
}

.node-layout {
  display: grid;
  grid-template-columns: minmax(0, 3fr) minmax(0, 2fr);
  gap: var(--space-4);
  align-items: start;
}

.node-col {
  display: grid;
  gap: var(--space-4);
  min-width: 0;
}

.node-health {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: var(--space-3);
}

.node-health__item {
  display: grid;
  justify-items: start;
  gap: var(--space-1);
  padding: var(--space-3);
  border-radius: var(--radius-sm);
  background: var(--fill-1);
}

.node-health__value {
  font-size: var(--type-title-3-size);
  font-weight: var(--weight-semibold);
  font-variant-numeric: tabular-nums;
}

.node-errors {
  margin-top: var(--space-3);
}

.node-errors__icon {
  color: var(--danger);
  vertical-align: -2px;
}

.node-errors__msg {
  overflow-wrap: anywhere;
  color: var(--label-2);
}

.node-engine-head {
  display: inline-flex;
  gap: var(--space-2);
  align-items: center;
}

.node-caps {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1);
  justify-content: flex-end;
  max-width: 50%;
}

.node-form {
  display: grid;
  gap: var(--space-4);
}

.node-form__pair {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--space-3);
}

.node-form__actions,
.node-danger {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  justify-content: flex-end;
}

.node-labels {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
}

.node-label {
  display: inline-flex;
  gap: var(--space-1);
  align-items: center;
  height: 24px;
  padding: 0 var(--space-2);
  border-radius: var(--radius-pill);
  background: var(--fill-1);
  font-family: var(--font-mono);
  font-size: var(--type-caption-size);
}

.node-install {
  margin: var(--space-3) 0;
}

@media (max-width: 1099.98px) {
  .node-layout {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (max-width: 639.98px) {
  .node-caps {
    justify-content: flex-start;
    max-width: none;
  }

  .fwd-list__item {
    flex-direction: column;
  }
}
</style>
