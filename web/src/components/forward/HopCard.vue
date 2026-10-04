<template>
  <article
    class="hop-card"
    :class="{ 'has-error': hasError }"
    :aria-labelledby="`${idBase}-title`"
    :data-hop-index="index"
    :data-field="path"
    @keydown.alt.up.prevent="index > 0 && emit('move', -1)"
    @keydown.alt.down.prevent="!last && emit('move', 1)"
  >
    <header class="hop-card__head">
      <span class="hop-card__index" aria-hidden="true">{{ index + 1 }}</span>
      <h3 :id="`${idBase}-title`" class="hop-card__title">{{ t('forwardV4.editor.hopTitle', { n: index + 1, role: t(`forwardV4.role.${role}`) }) }}</h3>
      <span class="hop-card__hint">{{ t('forwardV4.editor.roleHint') }}</span>
      <span class="hop-card__tools">
        <UiIconButton
          :ref="el => setTool('up', el)"
          size="sm"
          :icon="ArrowUp"
          :label="t('forwardV4.editor.moveUp', { n: index + 1 })"
          :disabled="index === 0"
          data-hop-tool="up"
          @click="emit('move', -1)"
        />
        <UiIconButton
          size="sm"
          :icon="ArrowDown"
          :label="t('forwardV4.editor.moveDown', { n: index + 1 })"
          :disabled="last"
          data-hop-tool="down"
          @click="emit('move', 1)"
        />
        <UiIconButton
          size="sm"
          :icon="Trash2"
          :label="t('forwardV4.editor.removeHop', { n: index + 1 })"
          :disabled="total === 1"
          data-hop-tool="remove"
          @click="emit('remove')"
        />
      </span>
    </header>

    <div class="hop-card__body">
      <UiField
        :label="t('forwardV4.editor.nodes')"
        label-tag="span"
        required
        :error="errorText(`${path}.node_refs`)"
        :help="index === 0 ? t('forwardV4.editor.nodesEntryHelp') : t('forwardV4.editor.nodesHelp')"
      >
        <template #default="{ labelId, describedBy }">
          <div class="hop-card__nodes" role="group" :aria-labelledby="labelId" :aria-describedby="describedBy">
            <span v-for="(ref, position) in hop.node_refs" :key="ref" class="hop-node" :data-node-ref="ref">
              <span class="hop-node__prio" :title="index === 0 ? t('forwardV4.editor.entryNode') : t('forwardV4.editor.priority')">{{ index === 0 ? t('forwardV4.editor.entryShort') : `P${position}` }}</span>
              <span class="hop-node__name">{{ nodeName(ref) }}</span>
              <span class="hop-node__engines">
                <EngineChip
                  v-for="engine in enginesOf(ref)"
                  :key="engine.engine"
                  :engine="engine.engine"
                  :unavailable="!engine.available"
                  :title="engine.available ? (engine.version || '') : (engine.unavailable_reason || '')"
                />
              </span>
              <UiIconButton size="sm" :icon="X" :label="t('forwardV4.editor.removeNode', { name: nodeName(ref) })" @click="emit('remove-node', ref)" />
            </span>
            <UiCombobox
              :id="fieldId(`${path}.node_refs`)"
              :model-value="adding"
              class="hop-card__add"
              size="md"
              :aria-label="t('forwardV4.editor.addNode', { n: index + 1 })"
              :placeholder="t('forwardV4.editor.addNodePlaceholder')"
              :empty-text="t('forwardV4.editor.noNodes')"
              :options="nodeOptions"
              @update:model-value="addNode"
            />
          </div>
        </template>
      </UiField>

      <div class="hop-card__row">
        <UiSelect
          :id="fieldId(`${path}.engine`)"
          :model-value="hop.engine"
          size="md"
          :label="t('forwardV4.editor.engine')"
          :options="engineOptions"
          :error="errorText(`${path}.engine`)"
          @update:model-value="value => emit('update', { engine: value })"
        />
        <UiSelect
          v-if="index > 0"
          :id="fieldId(`${path}.ingress.security`)"
          :model-value="security"
          size="md"
          :label="t('forwardV4.editor.ingress')"
          :options="securityOptions"
          :error="errorText(`${path}.ingress.security`)"
          :help="t('forwardV4.editor.ingressHelp')"
          @update:model-value="value => setIngress({ security: value, mux: value === 'LINK_SECURITY_RAW' ? false : hop.ingress?.mux })"
        />
        <p v-else class="hop-card__note">{{ t('forwardV4.editor.entryNote') }}</p>
      </div>

      <div v-if="linkError" class="fwd-note is-danger hop-card__fix" role="note">
        <span>{{ t('forwardV4.editor.linkFix', { prev: previousLabel, link: SECURITIES[security] || security }) }}</span>
        <span class="hop-card__fix-actions">
          <UiButton size="sm" :icon="Plus" @click="emit('insert-relay')">{{ t('forwardV4.editor.insertRelay') }}</UiButton>
          <UiButton size="sm" @click="setIngress({ security: 'LINK_SECURITY_RAW', mux: false })">{{ t('forwardV4.editor.useRaw') }}</UiButton>
        </span>
      </div>

      <div v-if="index > 0" class="hop-card__row">
        <UiSwitch
          :id="fieldId(`${path}.ingress.mux`)"
          :model-value="Boolean(hop.ingress?.mux)"
          :label="t('forwardV4.editor.mux')"
          :description="t('forwardV4.editor.muxHelp')"
          :disabled="security === 'LINK_SECURITY_RAW'"
          @update:model-value="value => setIngress({ mux: value })"
        />
        <UiTextField
          v-if="security !== 'LINK_SECURITY_RAW'"
          :id="fieldId(`${path}.ingress.server_name`)"
          :model-value="hop.ingress?.server_name || ''"
          size="md"
          :label="t('forwardV4.editor.serverName')"
          :placeholder="t('forwardV4.editor.serverNamePlaceholder')"
          :error="errorText(`${path}.ingress.server_name`)"
          @update:model-value="value => setIngress({ server_name: value })"
        />
        <UiTextField
          v-if="security === 'LINK_SECURITY_WSS'"
          :id="fieldId(`${path}.ingress.path`)"
          :model-value="hop.ingress?.path || ''"
          size="md"
          :label="t('forwardV4.editor.wsPath')"
          placeholder="/ws"
          :error="errorText(`${path}.ingress.path`)"
          @update:model-value="value => setIngress({ path: value })"
        />
      </div>

      <div v-if="index > 0" class="hop-card__row">
        <UiField :id="fieldId(`${path}.port`)" :label="t('forwardV4.editor.listenPort')" label-tag="span" :error="errorText(`${path}.port`)" :help="t('forwardV4.editor.hopPortHelp')">
          <template #default="{ labelId }">
            <span class="hop-card__port" role="group" :aria-labelledby="labelId">
              <UiSegmentedControl
                :model-value="hop.portMode"
                size="sm"
                :aria-label="t('forwardV4.editor.portMode')"
                :options="[{ value: 'auto', label: t('forwardV4.editor.auto') }, { value: 'explicit', label: t('forwardV4.editor.explicit') }]"
                @update:model-value="value => emit('update', { portMode: value })"
              />
              <UiNumberField
                v-if="hop.portMode === 'explicit'"
                :model-value="hop.port || null"
                size="md"
                :aria-label="t('forwardV4.editor.portValue', { n: index + 1 })"
                :min="1"
                :max="65535"
                placeholder="30000"
                @update:model-value="value => emit('update', { port: value || 0 })"
              />
            </span>
          </template>
        </UiField>
        <UiTextField
          :id="fieldId(`${path}.dial_address`)"
          :model-value="hop.dial_address"
          size="md"
          :label="t('forwardV4.editor.dialAddress')"
          :placeholder="t('forwardV4.editor.dialPlaceholder')"
          :disabled="hop.node_refs.length > 1 && !hop.dial_address"
          :error="errorText(`${path}.dial_address`)"
          :help="hop.node_refs.length > 1 ? t('forwardV4.editor.dialMulti') : t('forwardV4.editor.dialHelp')"
          @update:model-value="value => emit('update', { dial_address: value })"
        />
      </div>
    </div>
  </article>
</template>

<script setup>
// One hop of the route editor's chain (D2). It never changes the hop
// itself: every edit is an event the editor applies to its draft. Role is
// set by position; ↑ / ↓ (or Alt+↑ / Alt+↓ inside the card) reorder.
import { computed, ref } from 'vue'
import { ArrowDown, ArrowUp, Plus, Trash2, X } from '@lucide/vue'
import { UiButton, UiCombobox, UiField, UiIconButton, UiNumberField, UiSegmentedControl, UiSelect, UiSwitch, UiTextField } from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import EngineChip from './EngineChip.vue'
import { BASE_ENGINES, ENGINES, SECURITIES, fieldId, roleOf, securityOf, underField } from './routeModel'

const props = defineProps({
  hop: { type: Object, required: true },
  previous: { type: Object, default: null },
  index: { type: Number, required: true },
  total: { type: Number, required: true },
  nodes: { type: Array, default: () => [] },
  violations: { type: Array, default: () => [] },
  errorText: { type: Function, required: true },
  enableAnixOps: { type: Boolean, default: false }
})
const emit = defineEmits(['update', 'add-node', 'remove-node', 'move', 'remove', 'insert-relay'])
const { t } = useAppI18n()

const path = computed(() => `hops[${props.index}]`)
const idBase = computed(() => `fwd-hop-${props.index}`)
const role = computed(() => roleOf(props.index, props.total))
const last = computed(() => props.index === props.total - 1)
const security = computed(() => securityOf(props.hop))
const hasError = computed(() => underField(props.violations, path.value))
const linkError = computed(() => props.violations.some(item => item.field === `${path.value}.ingress.security` && item.code === 'link_unsupported'))
const previousLabel = computed(() => (props.previous
  ? t('forwardV4.editor.previousEngine', { n: props.index, engine: ENGINES[props.previous.engine]?.label || props.previous.engine })
  : ''))

const tools = {}
function setTool(name, el) {
  tools[name] = el
}
defineExpose({ focusTool: name => (tools[name]?.$el || tools[name])?.focus?.() })

const byRef = computed(() => new Map(props.nodes.map(node => [node.node_ref, node])))
function nodeName(ref) {
  return byRef.value.get(ref)?.name || ref
}
function enginesOf(ref) {
  return byRef.value.get(ref)?.info?.engines || byRef.value.get(ref)?.capabilities?.engines || []
}
function caps(ref, engine) {
  return enginesOf(ref).find(item => item.engine === engine)
}

const adding = ref(undefined)
function addNode(value) {
  if (!value) return
  emit('add-node', value)
  adding.value = null
  queueMicrotask(() => { adding.value = undefined })
}

const nodeOptions = computed(() => props.nodes
  .filter(node => !props.hop.node_refs.includes(node.node_ref))
  .map(node => {
    const usable = node.enabled !== false && node.in_inventory !== false
    const engines = (node.info?.engines || []).filter(engine => engine.available).map(engine => ENGINES[engine.engine]?.short || engine.engine).join(' + ')
    const range = node.settings?.port_range || node.info?.port_range
    return {
      value: node.node_ref,
      label: node.name || node.node_ref,
      disabled: !usable,
      description: usable
        ? [engines, node.record?.region, range ? t('forwardV4.editor.portRange', { first: range.first, last: range.last }) : ''].filter(Boolean).join(' · ')
        : t('forwardV4.editor.nodeUnusable')
    }
  }))

const engineOptions = computed(() => {
  const engines = [...BASE_ENGINES]
  if (props.enableAnixOps || props.hop.engine === 'ENGINE_ANIXOPS') engines.push('ENGINE_ANIXOPS')
  return engines.map(engine => {
    const missing = props.hop.node_refs.filter(ref => !caps(ref, engine)?.available)
    let description = t(`forwardV4.engine.${engine}`)
    if (missing.length) description = t('forwardV4.editor.engineMissing', { nodes: missing.map(nodeName).join(', ') })
    return {
      value: engine,
      label: engine === 'ENGINE_ANIXOPS' ? t('forwardV4.engine.anixopsLabel') : ENGINES[engine].label,
      description,
      disabled: missing.length > 0 && props.hop.engine !== engine
    }
  })
})

const securityOptions = computed(() => Object.keys(SECURITIES)
  .filter(key => key !== 'LINK_SECURITY_ANIXOPS' || props.enableAnixOps || security.value === key)
  .map(key => {
    const originates = props.previous?.node_refs.every(ref => caps(ref, props.previous.engine)?.link_securities?.includes(key)) ?? true
    const terminates = props.hop.node_refs.every(ref => caps(ref, props.hop.engine)?.link_securities?.includes(key))
    let description = ''
    if (!originates) description = t('forwardV4.editor.cannotOriginate', { engine: ENGINES[props.previous.engine]?.label || props.previous.engine })
    else if (!terminates) description = t('forwardV4.editor.cannotTerminate', { engine: ENGINES[props.hop.engine]?.label || props.hop.engine })
    return {
      value: key,
      label: key === 'LINK_SECURITY_ANIXOPS' ? t('forwardV4.engine.anixopsLabel') : SECURITIES[key],
      description,
      disabled: Boolean(description) && security.value !== key
    }
  }))

function setIngress(patch) {
  emit('update', { ingress: { ...(props.hop.ingress || {}), ...patch } })
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

.hop-card__hint,
.hop-card__note {
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

.hop-card__note {
  align-self: end;
  margin: 0;
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
  color: var(--label-2);
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
