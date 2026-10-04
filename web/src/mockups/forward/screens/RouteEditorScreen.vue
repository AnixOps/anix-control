<template>
  <section class="fwd-screen">
    <UiPageHeader :title="editing ? `编辑 ${draft.name}` : '新建路由'">
      <template #back>
        <RouterLink class="editor-back" :to="`${BASE}/routes`"><ChevronLeft :size="16" /> 路由</RouterLink>
      </template>
      <template #meta>
        <UiBadge v-if="editing" tone="neutral" :label="`修订 ${draft.revision}`" />
      </template>
      <template #actions>
        <UiButton @click="router.push(`${BASE}/routes`)">取消</UiButton>
        <UiButton variant="primary" :icon="Check" :disabled="violations.length > 0">{{ editing ? '保存' : '创建路由' }}</UiButton>
      </template>
    </UiPageHeader>

    <p v-if="enforcedNote" class="fwd-note is-warning" role="status">{{ enforcedNote }}</p>

    <div v-if="violations.length" class="editor-summary" role="alert">
      <p class="editor-summary__title"><AlertCircle :size="16" /> {{ violations.length }} 个问题需要处理，处理后才能保存</p>
      <ul class="editor-summary__list">
        <li v-for="(item, index) in violations" :key="index">
          <button type="button" class="editor-summary__item" @click="focusField(item.field)">
            <span class="fwd-mono editor-summary__field">{{ item.field }}</span>
            <span>{{ CODE_TEXT[item.code] }}</span>
            <span class="fwd-mono editor-summary__code">{{ item.code }}</span>
          </button>
        </li>
      </ul>
    </div>

    <div class="editor-layout">
      <form class="editor-form" @submit.prevent>
        <UiCard title="基本信息" heading-tag="h2">
          <div class="editor-grid">
            <UiTextField :id="fid('name')" v-model="draft.name" size="md" label="名称" required :error="errorFor('name')" />
            <UiField label="标签" label-tag="span" help="key=value，用于筛选和批量操作。">
              <template #default>
                <span class="editor-labels">
                  <span v-for="(value, key) in draft.labels" :key="key" class="editor-label">{{ key }}={{ value }}<X :size="12" aria-hidden="true" /></span>
                  <UiButton size="sm" variant="tertiary" :icon="Plus">添加</UiButton>
                </span>
              </template>
            </UiField>
          </div>
        </UiCard>

        <UiCard title="入口监听" heading-tag="h2" description="客户端连接的地址和端口，在第 1 跳的所有节点上相同。">
          <div class="editor-grid editor-grid--3">
            <UiTextField v-model="draft.listen.address" size="md" label="监听地址" placeholder="0.0.0.0" />
            <UiField :id="fid('listen.port')" label="端口" label-tag="span" :error="errorFor('listen.port')" :help="listenHelp">
              <template #default>
                <span class="editor-port">
                  <UiSegmentedControl v-model="listenMode" size="sm" aria-label="端口方式" :options="[{ value: 'auto', label: '自动' }, { value: 'explicit', label: '指定' }]" />
                  <UiNumberField v-if="listenMode === 'explicit'" v-model="draft.listen.port" size="md" aria-label="端口" :min="1" :max="65535" />
                </span>
              </template>
            </UiField>
            <UiSelect v-model="draft.listen.protocol" size="md" label="协议" :options="protocolOptions" />
          </div>
          <UiTextField
            v-if="draft.hops[0]?.node_refs.length > 1"
            v-model="draft.listen.entry_hostname"
            class="editor-mt"
            size="md"
            label="入口域名"
            placeholder="edge.example.net"
            help="多个入口节点时，Control 通过 DNS 把它指向健康的入口。"
          />
        </UiCard>

        <UiCard title="跳链" heading-tag="h2" description="第 1 跳是入口，最后一跳是出口，中间为中转。只有 1 跳时入口直接连接目标。anixops 引擎（实验）在系统设置开启后才出现在引擎列表里。">
          <ol class="editor-chain">
            <template v-for="(hop, index) in draft.hops" :key="hop._key">
              <li v-if="index > 0" class="editor-link" aria-hidden="true">
                <ArrowDown :size="16" />
                <span class="fwd-mono">{{ linkText(hop) }}</span>
              </li>
              <li>
                <HopCard
                  :hop="hop"
                  :previous="draft.hops[index - 1] || null"
                  :index="index"
                  :total="draft.hops.length"
                  :violations="violations"
                  :error-for="errorFor"
                  :fid="fid"
                  @move="delta => moveHop(index, delta)"
                  @remove="removeHop(index)"
                  @insert-relay="insertRelay(index)"
                />
              </li>
            </template>
            <li class="editor-link" aria-hidden="true"><ArrowDown :size="16" /><span class="fwd-mono">RAW</span></li>
            <li class="editor-targets-anchor fwd-muted">目标（见下方）</li>
          </ol>
          <UiButton class="editor-mt" :icon="Plus" @click="addHop">添加一跳</UiButton>
        </UiCard>

        <UiCard title="目标" heading-tag="h2" description="由最后一跳拨号；域名在该节点上解析并定期重新解析。">
          <div class="editor-targets">
            <div class="editor-targets__head" aria-hidden="true">
              <span>主机</span><span>端口</span><span>权重</span><span>优先级</span><span />
            </div>
            <div v-for="(target, index) in draft.targets" :key="index" class="editor-target">
              <UiTextField :id="fid(`targets[${index}].host`)" v-model="target.host" size="md" :aria-label="`目标 ${index + 1} 主机`" placeholder="host 或 IP" :error="errorFor(`targets[${index}].host`)" />
              <UiNumberField :id="fid(`targets[${index}].port`)" v-model="target.port" size="md" :aria-label="`目标 ${index + 1} 端口`" :min="1" :max="65535" :error="errorFor(`targets[${index}].port`)" />
              <UiNumberField v-model="target.weight" size="md" :aria-label="`目标 ${index + 1} 权重`" :min="0" placeholder="1" />
              <UiNumberField v-model="target.priority" size="md" :aria-label="`目标 ${index + 1} 优先级`" :min="0" placeholder="0" />
              <UiIconButton :icon="Trash2" :label="`删除目标 ${index + 1}`" :disabled="draft.targets.length === 1" @click="draft.targets.splice(index, 1)" />
            </div>
          </div>
          <div class="editor-grid editor-mt">
            <UiButton :icon="Plus" @click="draft.targets.push({ host: '', port: 443 })">添加目标</UiButton>
            <UiSelect v-model="draft.policy.target_policy" size="md" label="目标策略" :options="targetPolicyOptions" />
          </div>
        </UiCard>

        <UiCard title="负载均衡与故障转移" heading-tag="h2">
          <div class="editor-grid">
            <UiSelect v-model="draft.policy.next_hop" size="md" label="下一跳策略" :options="strategyOptions" help="在有多个节点的中转 / 出口之间。" />
            <UiSelect v-model="draft.policy.target" size="md" label="目标策略" :options="strategyOptions" :help="leastConnHint" />
          </div>
          <UiRadioGroup :id="fid('policy.direct')" v-model="draft.policy.direct" class="editor-mt" label="直连模式" orientation="horizontal" :options="directOptions" :error="errorFor('policy.direct')" />
          <div class="editor-subhead">
            <h3 class="editor-subhead__title">健康检查与熔断</h3>
            <span class="fwd-muted">留空使用默认值（H21）。每个节点自己检查上游。</span>
          </div>
          <div class="editor-grid editor-grid--4">
            <UiNumberField v-model="draft.policy.health.interval_ms" size="md" label="检查间隔" unit="ms" :placeholder="String(H21_DEFAULTS.interval_ms)" help="默认 5000" />
            <UiNumberField v-model="draft.policy.health.timeout_ms" size="md" label="超时" unit="ms" :placeholder="String(H21_DEFAULTS.timeout_ms)" help="默认 2000" />
            <UiNumberField v-model="draft.policy.circuit_breaker.failure_threshold" size="md" label="熔断阈值" unit="次" :placeholder="String(H21_DEFAULTS.failure_threshold)" help="连续失败，默认 3" />
            <UiNumberField v-model="draft.policy.circuit_breaker.open_ms" size="md" label="熔断时长" unit="ms" :placeholder="String(H21_DEFAULTS.open_ms)" help="之后放一次试探，默认 30000" />
          </div>
          <UiSwitch v-model="healthDisabled" class="editor-mt" label="关闭主动健康检查" description="只靠真实连接失败判断（不推荐）。" />
        </UiCard>

        <UiCard id="limits" title="限额" heading-tag="h2" description="只作用于入口。带宽和连接数按每个入口节点计算；配额为全局，由 Control 汇总并在用尽时暂停路由。">
          <div class="editor-grid editor-grid--4">
            <UiNumberField v-model="limits.bandwidth" size="md" label="带宽" unit="Mbps" placeholder="不限" />
            <UiNumberField v-model="limits.quota" size="md" label="配额" unit="GB" placeholder="不限" />
            <UiNumberField v-model="draft.limits.max_conns" size="md" label="最大连接" placeholder="不限" />
            <UiTextField :id="fid('limits.expires_at_unix_ms')" v-model="limits.expires" size="md" label="到期时间" placeholder="不限" :error="errorFor('limits.expires_at_unix_ms')" help="到期后 Control 暂停路由" />
          </div>
        </UiCard>
      </form>

      <aside id="preview" class="editor-preview" aria-label="规划预览">
        <RoutePreview :preview="preview" :draft="draft" />
      </aside>
    </div>

    <div class="editor-phonebar">
      <UiBadge v-if="violations.length" tone="danger" :label="`${violations.length} 个问题`" />
      <UiBadge v-else tone="success" label="可以规划" />
      <a class="editor-phonebar__link" href="#preview">查看预览</a>
      <UiButton size="sm" variant="primary" :disabled="violations.length > 0">{{ editing ? '保存' : '创建' }}</UiButton>
    </div>
  </section>
</template>

<script setup>
import { computed, nextTick, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { AlertCircle, ArrowDown, Check, ChevronLeft, Plus, Trash2, X } from '@lucide/vue'
import { UiBadge, UiButton, UiCard, UiField, UiIconButton, UiNumberField, UiPageHeader, UiRadioGroup, UiSegmentedControl, UiSelect, UiSwitch, UiTextField } from '@/ui'
import HopCard from '../parts/HopCard.vue'
import RoutePreview from '../parts/RoutePreview.vue'
import { DIRECT_MODES, H21_DEFAULTS, ROUTES, SECURITIES, STRATEGIES, routeById } from '../mockData'
import { CODE_TEXT, previewDraft } from '../mockPlanner'

const BASE = '/admin/__mockups/forward'
const router = useRouter()
const query = useRoute().query
const source = routeById(query.id) || (query.state === 'valid' ? ROUTES[0] : null)
const editing = Boolean(source)

let keySeq = 0
function withKeys(hops) {
  return hops.map(hop => ({ dial_address: '', port: 0, ...hop, _key: keySeq++ }))
}

// A new route as an operator might first write it: nftables entry straight
// into a TLS exit (link_unsupported), a reserved port and a private target
// under the default public-only policy.
const NEW_DRAFT = {
  name: 'game-sha-tyo',
  listen: { address: '0.0.0.0', port: 443, protocol: 'L4_PROTOCOL_TCP' },
  hops: [
    { role: 'HOP_ROLE_ENTRY', engine: 'ENGINE_NFTABLES', node_refs: ['forward-71'] },
    { role: 'HOP_ROLE_EXIT', engine: 'ENGINE_GOST', node_refs: ['forward-61', 'forward-62'], ingress: { security: 'LINK_SECURITY_TLS', mux: true } }
  ],
  targets: [{ host: '10.0.3.8', port: 7777, weight: 1 }],
  policy: { next_hop: 'BALANCE_STRATEGY_FAILOVER', target: 'BALANCE_STRATEGY_ROUND_ROBIN', direct: 'DIRECT_MODE_OFF', target_policy: 'TARGET_POLICY_PUBLIC_ONLY' },
  limits: {},
  labels: { team: 'game' }
}

const base = JSON.parse(JSON.stringify(source ? source.route : NEW_DRAFT))
const draft = reactive({
  ...base,
  hops: withKeys(base.hops),
  policy: { direct: 'DIRECT_MODE_OFF', target_policy: 'TARGET_POLICY_PUBLIC_ONLY', ...base.policy, health: { ...(base.policy?.health || {}) }, circuit_breaker: { ...(base.policy?.circuit_breaker || {}) } },
  limits: { ...(base.limits || {}) },
  labels: { ...(base.labels || {}) }
})
const listenMode = ref(draft.listen.port ? 'explicit' : 'auto')
const healthDisabled = ref(false)
const GB = 1024 ** 3
const limits = reactive({
  bandwidth: draft.limits.bandwidth_bps ? Number(draft.limits.bandwidth_bps) / 1e6 : null,
  quota: draft.limits.quota_bytes ? Number(draft.limits.quota_bytes) / GB : null,
  expires: draft.limits.expires_at_unix_ms ? new Date(Number(draft.limits.expires_at_unix_ms) + 8 * 3600_000).toISOString().slice(0, 16).replace('T', ' ') : ''
})

const enforcedNote = computed(() => {
  const enforced = source && ROUTES.find(item => item.route.id === source.route.id)?.enforced
  if (enforced === 'quota') return '此路由因配额用尽被 Control 强制暂停。提高或清空配额后，下一次规划自动恢复；“恢复”按钮对它无效。'
  if (enforced === 'expired') return '此路由已到期，被 Control 强制暂停。延长或清空到期时间后，下一次规划自动恢复。'
  return ''
})

// Roles follow positions; the preview reads the route as the API would get it.
function syncRoles() {
  draft.hops.forEach((hop, index) => {
    hop.role = index === 0 ? 'HOP_ROLE_ENTRY' : (index === draft.hops.length - 1 ? 'HOP_ROLE_EXIT' : 'HOP_ROLE_RELAY')
  })
}

const routeForApi = computed(() => ({
  ...draft,
  listen: { ...draft.listen, port: listenMode.value === 'auto' ? 0 : draft.listen.port },
  limits: {
    ...draft.limits,
    expires_at_unix_ms: draft.limits.expires_at_unix_ms
  }
}))
const preview = computed(() => previewDraft(routeForApi.value, { onCreate: !editing }))
const violations = computed(() => preview.value.violations)

function fid(field) {
  return `f-${field.replace(/[^a-z0-9]+/gi, '-')}`
}
function errorFor(field) {
  const hit = violations.value.find(item => item.field === field)
  return hit ? `${CODE_TEXT[hit.code] || hit.message}` : ''
}
async function focusField(field) {
  await nextTick()
  const el = document.getElementById(fid(field)) || document.querySelector(`[id^="${fid(field)}"]`)
  el?.scrollIntoView({ block: 'center' })
  el?.focus?.()
}

function linkText(hop) {
  const security = SECURITIES[hop.ingress?.security || 'LINK_SECURITY_RAW']
  return hop.ingress?.mux ? `${security} · mux` : security
}
function moveHop(index, delta) {
  const [hop] = draft.hops.splice(index, 1)
  draft.hops.splice(index + delta, 0, hop)
  syncRoles()
}
function removeHop(index) {
  draft.hops.splice(index, 1)
  syncRoles()
}
function addHop() {
  draft.hops.push({ role: 'HOP_ROLE_EXIT', engine: 'ENGINE_GOST', node_refs: [], ingress: { security: 'LINK_SECURITY_TLS' }, port: 0, dial_address: '', _key: keySeq++ })
  syncRoles()
}
function insertRelay(index) {
  draft.hops.splice(index, 0, { role: 'HOP_ROLE_RELAY', engine: 'ENGINE_GOST', node_refs: ['forward-41'], ingress: { security: 'LINK_SECURITY_RAW' }, port: 0, dial_address: '', _key: keySeq++ })
  syncRoles()
}

const listenHelp = computed(() => (listenMode.value === 'auto'
  ? '自动：取所有入口节点共同的最小空闲端口。'
  : '须在入口节点的端口范围内，且不是保留端口。'))
const protocolOptions = [
  { value: 'L4_PROTOCOL_TCP', label: 'TCP' },
  { value: 'L4_PROTOCOL_UDP', label: 'UDP' },
  { value: 'L4_PROTOCOL_TCP_UDP', label: 'TCP + UDP' }
]
const strategyOptions = Object.entries(STRATEGIES).map(([value, item]) => ({ value, label: item.label, description: item.hint }))
const directOptions = Object.entries(DIRECT_MODES).map(([value, item]) => ({ value, label: item.label, description: item.hint }))
const targetPolicyOptions = [
  { value: 'TARGET_POLICY_PUBLIC_ONLY', label: '只允许公网地址', description: '默认；防止路由被用来访问内网' },
  { value: 'TARGET_POLICY_ALLOW_PRIVATE', label: '允许内网地址', description: '专线、同机房内网' }
]
const leastConnHint = computed(() => {
  const lastHop = draft.hops[draft.hops.length - 1]
  if (draft.policy.target === 'BALANCE_STRATEGY_LEAST_CONN' && lastHop?.engine === 'ENGINE_NFTABLES') return 'nftables 上为近似：在 Agent 提供 conntrack 计数前按权重随机。'
  return '在最后一跳的目标之间。'
})

</script>

<style scoped>
.editor-back {
  display: inline-flex;
  gap: var(--space-1);
  align-items: center;
  color: var(--accent);
  text-decoration: none;
}

.editor-summary {
  padding: var(--space-3) var(--space-4);
  border: 1px solid var(--danger);
  border-radius: var(--radius-sm);
  background: var(--danger-soft);
}

.editor-summary__title {
  display: flex;
  gap: var(--space-2);
  align-items: center;
  margin: 0 0 var(--space-2);
  font-weight: var(--weight-semibold);
}

.editor-summary__list {
  display: grid;
  gap: var(--space-1);
  margin: 0;
  padding: 0;
  list-style: none;
}

.editor-summary__item {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1) var(--space-3);
  align-items: baseline;
  padding: var(--space-1) 0;
  border: 0;
  background: none;
  color: var(--label-1);
  text-align: left;
  cursor: pointer;
}

.editor-summary__item:hover span:nth-child(2) {
  text-decoration: underline;
}

.editor-summary__field {
  color: var(--label-2);
}

.editor-summary__code {
  color: var(--label-3);
}

.editor-layout {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(320px, 400px);
  gap: var(--space-4);
  align-items: start;
}

.editor-form {
  display: grid;
  gap: var(--space-4);
  min-width: 0;
}

.editor-preview {
  position: sticky;
  top: calc(var(--shell-topbar-height, 52px) + var(--space-4));
  min-width: 0;
}

.editor-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: var(--space-3);
  align-items: start;
}

.editor-grid--3 {
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
}

.editor-grid--4 {
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
}

.editor-mt {
  margin-top: var(--space-3);
}

.editor-port {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
}

.editor-labels {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
  min-height: 36px;
}

.editor-label {
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

.editor-chain {
  display: grid;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}

.editor-link {
  display: flex;
  gap: var(--space-2);
  align-items: center;
  padding-left: var(--space-4);
  color: var(--label-2);
}

.editor-targets-anchor {
  padding-left: var(--space-4);
}

.editor-targets {
  display: grid;
  gap: var(--space-2);
}

.editor-targets__head,
.editor-target {
  display: grid;
  grid-template-columns: minmax(0, 3fr) minmax(0, 1.2fr) minmax(0, 1fr) minmax(0, 1fr) 36px;
  gap: var(--space-2);
  align-items: start;
}

.editor-targets__head {
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

.editor-subhead {
  display: grid;
  gap: var(--space-1);
  margin: var(--space-5) 0 var(--space-3);
}

.editor-subhead__title {
  margin: 0;
  font-size: var(--type-body-size);
  font-weight: var(--weight-semibold);
}

@media (max-width: 1099.98px) {
  .editor-layout {
    grid-template-columns: minmax(0, 1fr);
  }

  .editor-preview {
    position: static;
  }
}

.editor-phonebar {
  display: none;
}

@media (max-width: 639.98px) {
  .editor-phonebar {
    position: sticky;
    bottom: 0;
    z-index: var(--z-sticky);
    display: flex;
    gap: var(--space-3);
    align-items: center;
    margin: 0 calc(var(--space-4) * -1);
    padding: var(--space-2) var(--space-4) calc(var(--space-2) + env(safe-area-inset-bottom));
    border-top: 1px solid var(--separator);
    background: var(--material);
    backdrop-filter: blur(20px);
  }

  .editor-phonebar__link {
    margin-left: auto;
    color: var(--accent);
    text-decoration: none;
  }

  .editor-targets__head {
    display: none;
  }

  .editor-target {
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr) minmax(0, 1fr) 36px;
    padding-bottom: var(--space-2);
    border-bottom: 1px solid var(--separator);
  }

  .editor-target > :first-child {
    grid-column: 1 / -1;
  }
}
</style>
