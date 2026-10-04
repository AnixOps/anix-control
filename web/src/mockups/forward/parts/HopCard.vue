<template>
  <article class="hop-card" :class="{ 'has-error': hasError }" :aria-labelledby="`${idBase}-title`">
    <header class="hop-card__head">
      <span class="hop-card__index" aria-hidden="true">{{ index + 1 }}</span>
      <h4 :id="`${idBase}-title`" class="hop-card__title">第 {{ index + 1 }} 跳 · {{ ROLE[hop.role] }}</h4>
      <span class="fwd-muted hop-card__role-hint">角色由位置决定</span>
      <span class="hop-card__tools">
        <UiIconButton size="sm" :icon="ArrowUp" label="上移" :disabled="index === 0" @click="emit('move', -1)" />
        <UiIconButton size="sm" :icon="ArrowDown" label="下移" :disabled="last" @click="emit('move', 1)" />
        <UiIconButton size="sm" :icon="Trash2" label="删除这一跳" :disabled="total === 1" @click="emit('remove')" />
      </span>
    </header>

    <div class="hop-card__body">
      <UiField :id="fid(`${path}.node_refs`)" label="节点" :error="errorFor(`${path}.node_refs`)" required label-tag="span" :help="nodeHelp">
        <template #default>
          <div class="hop-card__nodes">
            <span v-for="(ref, position) in hop.node_refs" :key="ref" class="hop-node">
              <span class="hop-node__prio" :title="index === 0 ? '入口节点' : '故障转移优先级'">{{ index === 0 ? '入口' : `P${position}` }}</span>
              <span class="hop-node__name">{{ nodeName(ref) }}</span>
              <span class="hop-node__engines">
                <EngineChip v-for="engine in enginesOf(ref)" :key="engine.engine" :engine="engine.engine" :unavailable="!engine.available" :title="engine.available ? engine.version : engine.unavailable_reason" />
              </span>
              <UiIconButton size="sm" :icon="X" :label="`移除 ${nodeName(ref)}`" @click="removeNode(ref)" />
            </span>
            <UiCombobox
              v-model="adding"
              class="hop-card__add"
              size="md"
              aria-label="添加节点"
              placeholder="添加节点…"
              empty-text="没有可用节点"
              :options="nodeOptions"
            />
          </div>
        </template>
      </UiField>

      <div class="hop-card__row">
        <UiSelect
          :id="fid(`${path}.engine`)"
          v-model="hop.engine"
          size="md"
          label="引擎"
          :options="engineOptions"
          :error="errorFor(`${path}.engine`)"
        />
        <template v-if="index > 0">
          <UiSelect
            :id="fid(`${path}.ingress.security`)"
            :model-value="hop.ingress?.security || 'LINK_SECURITY_RAW'"
            size="md"
            label="接入链路"
            :options="securityOptions"
            :error="errorFor(`${path}.ingress.security`)"
            help="上一跳到本跳的链路：上一跳发起，本跳终止。"
            @update:model-value="value => setIngress({ security: value, mux: value === 'LINK_SECURITY_RAW' ? false : hop.ingress?.mux })"
          />
        </template>
        <p v-else class="fwd-muted hop-card__entry-note">入口监听的地址和端口在“入口监听”里设置；限额只作用于入口。</p>
      </div>

      <div v-if="linkError" class="fwd-note is-danger hop-card__fix" role="note">
        <span>{{ previousLabel }} 无法发起 {{ SECURITIES[hop.ingress?.security] }}。可以：</span>
        <span class="hop-card__fix-actions">
          <UiButton size="sm" :icon="Plus" @click="emit('insert-relay')">在前面插入 gost 中转</UiButton>
          <UiButton size="sm" @click="setIngress({ security: 'LINK_SECURITY_RAW', mux: false })">改为 RAW</UiButton>
        </span>
      </div>

      <div v-if="index > 0" class="hop-card__row">
        <UiSwitch
          :id="fid(`${path}.ingress.mux`)"
          :model-value="Boolean(hop.ingress?.mux)"
          label="多路复用"
          description="多个连接共用一条加密链路；RAW 不可用。"
          :disabled="(hop.ingress?.security || 'LINK_SECURITY_RAW') === 'LINK_SECURITY_RAW'"
          @update:model-value="value => setIngress({ mux: value })"
        />
        <UiTextField
          v-if="['LINK_SECURITY_TLS', 'LINK_SECURITY_WSS', 'LINK_SECURITY_GRPC', 'LINK_SECURITY_QUIC'].includes(hop.ingress?.security)"
          :model-value="hop.ingress?.server_name || ''"
          size="md"
          label="服务器名称（可选）"
          placeholder="默认用节点身份，如 forward-61"
          @update:model-value="value => setIngress({ server_name: value })"
        />
      </div>

      <div v-if="index > 0" class="hop-card__row">
        <UiField label="监听端口" label-tag="span" help="自动：规划器在节点端口范围内分配并保持不变。">
          <template #default>
            <span class="hop-card__port">
              <UiSegmentedControl v-model="portMode" size="sm" aria-label="端口方式" :options="[{ value: 'auto', label: '自动' }, { value: 'explicit', label: '指定' }]" />
              <UiNumberField v-if="portMode === 'explicit'" v-model="hop.port" size="md" aria-label="端口" :min="1" :max="65535" placeholder="30000" />
            </span>
          </template>
        </UiField>
        <UiTextField
          :id="fid(`${path}.dial_address`)"
          v-model="hop.dial_address"
          size="md"
          label="拨号地址（可选）"
          placeholder="默认用节点的第一个地址"
          :disabled="hop.node_refs.length > 1 && !hop.dial_address"
          :error="errorFor(`${path}.dial_address`)"
          :help="hop.node_refs.length > 1 ? '多个节点时不能指定（各节点用自己的地址）。' : '上一跳拨这个地址，例如专线内网 IP。'"
        />
      </div>
    </div>
  </article>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { ArrowDown, ArrowUp, Plus, Trash2, X } from '@lucide/vue'
import { UiButton, UiCombobox, UiField, UiIconButton, UiNumberField, UiSegmentedControl, UiSelect, UiSwitch, UiTextField } from '@/ui'
import EngineChip from './EngineChip.vue'
import { ENGINES, NODES, SECURITIES, nodeByRef, nodeName } from '../mockData'

const props = defineProps({
  hop: { type: Object, required: true },
  previous: { type: Object, default: null },
  index: { type: Number, required: true },
  total: { type: Number, required: true },
  violations: { type: Array, default: () => [] },
  errorFor: { type: Function, required: true },
  fid: { type: Function, required: true }
})
const emit = defineEmits(['move', 'remove', 'insert-relay'])

const ROLE = { HOP_ROLE_ENTRY: '入口', HOP_ROLE_RELAY: '中转', HOP_ROLE_EXIT: '出口' }
const path = computed(() => `hops[${props.index}]`)
const idBase = computed(() => `hop-${props.index}`)
const last = computed(() => props.index === props.total - 1)
const hasError = computed(() => props.violations.some(item => item.field.startsWith(path.value)))
const linkError = computed(() => props.violations.some(item => item.field === `${path.value}.ingress.security` && item.code === 'link_unsupported'))
const previousLabel = computed(() => (props.previous ? `第 ${props.index} 跳的 ${ENGINES[props.previous.engine].label}` : ''))
const portMode = ref(props.hop.port ? 'explicit' : 'auto')

function enginesOf(ref) {
  return nodeByRef(ref)?.info.engines || []
}

function caps(ref, engine) {
  return enginesOf(ref).find(item => item.engine === engine)
}

const nodeHelp = computed(() => {
  if (props.index === 0) return '多个入口节点共享一个端口，配合入口域名做入口高可用。'
  return '多个节点时，上一跳按“下一跳策略”在它们之间均衡；顺序即故障转移优先级。'
})

const adding = ref(undefined)
watch(adding, value => {
  if (!value) return
  props.hop.node_refs.push(value)
  adding.value = undefined
})

const nodeOptions = computed(() => NODES.filter(node => !props.hop.node_refs.includes(node.node_ref)).map(node => ({
  value: node.node_ref,
  label: node.name,
  disabled: !node.enabled,
  description: node.enabled
    ? `${node.info.engines.filter(engine => engine.available).map(engine => ENGINES[engine.engine].short).join(' + ')} · ${node._region} · 端口 ${node.settings.port_range.first}–${node.settings.port_range.last}`
    : '已禁用，不在转发清单'
})))

function removeNode(ref) {
  props.hop.node_refs.splice(props.hop.node_refs.indexOf(ref), 1)
}

const engineOptions = computed(() => ['ENGINE_NFTABLES', 'ENGINE_GOST'].map(engine => {
  const missing = props.hop.node_refs.filter(ref => !caps(ref, engine)?.available)
  return {
    value: engine,
    label: ENGINES[engine].label,
    description: missing.length
      ? `${missing.map(nodeName).join('、')} 不支持或不可用`
      : (engine === 'ENGINE_NFTABLES' ? '内核 DNAT，几乎不占 CPU；只有 RAW 链路' : '支持 TLS / WSS / QUIC / gRPC 与多路复用'),
    disabled: missing.length > 0 && props.hop.engine !== engine
  }
}))

const securityOptions = computed(() => Object.keys(SECURITIES).filter(key => key !== 'LINK_SECURITY_ANIXOPS').map(security => {
  const originates = props.previous?.node_refs.every(ref => caps(ref, props.previous.engine)?.link_securities.includes(security)) ?? true
  const terminates = props.hop.node_refs.every(ref => caps(ref, props.hop.engine)?.link_securities.includes(security))
  let description = ''
  if (!originates) description = `${ENGINES[props.previous.engine].label} 不能发起`
  else if (!terminates) description = `${ENGINES[props.hop.engine].label} 不能终止`
  return { value: security, label: SECURITIES[security], description, disabled: Boolean(description) && props.hop.ingress?.security !== security }
}))

function setIngress(patch) {
  props.hop.ingress = { ...(props.hop.ingress || {}), ...patch }
}
</script>

<style scoped>
.hop-card {
  display: grid;
  gap: var(--space-3);
  padding: var(--space-4);
  border: 1px solid var(--separator);
  border-radius: var(--radius-md);
  background: var(--bg-elevated);
}

.hop-card.has-error {
  border-color: var(--danger);
}

.hop-card__head {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
}

.hop-card__index {
  display: inline-grid;
  place-items: center;
  width: 24px;
  height: 24px;
  border-radius: 50%;
  background: var(--accent-soft);
  color: var(--accent);
  font-size: var(--type-caption-size);
  font-weight: var(--weight-semibold);
}

.hop-card__title {
  margin: 0;
  font-size: var(--type-body-size);
  font-weight: var(--weight-semibold);
}

.hop-card__tools {
  display: inline-flex;
  margin-left: auto;
}

.hop-card__body {
  display: grid;
  gap: var(--space-3);
}

.hop-card__row {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: var(--space-3);
  align-items: start;
}

.hop-card__nodes {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
}

.hop-node {
  display: inline-flex;
  gap: var(--space-2);
  align-items: center;
  min-height: 36px;
  padding: 0 var(--space-1) 0 var(--space-2);
  border-radius: var(--radius-sm);
  background: var(--fill-1);
}

.hop-node__prio {
  color: var(--label-3);
  font-family: var(--font-mono);
  font-size: var(--type-caption-size);
}

.hop-node__name {
  font-weight: var(--weight-medium);
}

.hop-node__engines {
  display: inline-flex;
  gap: 2px;
}

.hop-card__add {
  min-width: 180px;
}

.hop-card__entry-note {
  align-self: end;
  margin: 0;
}

.hop-card__fix {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
  justify-content: space-between;
}

.hop-card__fix-actions {
  display: inline-flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}

.hop-card__port {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
}
</style>
