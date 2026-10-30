<template>
  <div class="hosts-panel" data-testid="forward-hosts-panel">
    <UiDataTable
      :columns="columns"
      :rows="rows"
      row-key="node_ref"
      :label="t('forwardV4.nodes.title')"
      :row-label="row => row.name"
      :row-actions="rowActions"
      :loading="inventory.loading.value && !inventory.available.value"
      :filtered="filtered"
      :empty-icon="Server"
      :empty-title="t('forwardV4.nodes.emptyTitle')"
      :empty-description="t('forwardV4.nodes.emptyDescription')"
      activatable
      :card-fields="4"
      @row-activate="row => router.push(`/admin/forward/inventory/${row.node_ref}`)"
      @clear-filters="flag = ''"
    >
      <template #toolbar>
        <p class="hosts-panel__summary">{{ t('forwardV4.nodes.description') }}</p>
        <UiFilterChips v-if="allRows.length" v-model="flag" :label="t('forwardV4.nodes.flagFilter')" :options="flagOptions" />
      </template>
      <template #toolbar-end>
        <UiButton :icon="Plus" data-testid="forward-add-node" @click="openCreate">{{ t('forwardV4.nodes.add') }}</UiButton>
      </template>
      <template #empty-actions>
        <UiButton :icon="Plus" @click="openCreate">{{ t('forwardV4.nodes.add') }}</UiButton>
      </template>

      <template #cell-name="{ row }">
        <span class="fwd-cell-stack">
          <span class="nodes-name">{{ row.name }}</span>
          <span class="fwd-muted">{{ [row.node_ref, row.record?.region].filter(Boolean).join(' · ') }}</span>
        </span>
      </template>
      <template #cell-status="{ row }">
        <UiBadge :status="row.state" />
      </template>
      <template #cell-kind="{ row }">
        <UiBadge v-if="row.ansible" tone="neutral" :dot="false" :label="t('forwardV4.nodes.ansible')" />
        <UiBadge v-else tone="warning" :dot="false" :label="t('forwardV4.nodes.unconverted')" :title="t('forwardV4.nodes.unconvertedHint')" />
      </template>
      <template #cell-agent="{ row }">
        <span class="fwd-mono">{{ row.capabilities?.agent_version || '—' }}</span>
      </template>
      <template #cell-engines="{ row }">
        <span class="nodes-engines">
          <EngineChip
            v-for="engine in row.info?.engines || []"
            :key="engine.engine"
            :engine="engine.engine"
            :unavailable="!engine.available"
            :title="engine.available ? `${engine.version || ''} ${(engine.link_securities || []).map(s => SECURITIES[s] || s).join(' / ')}` : (engine.unavailable_reason || '')"
          />
          <span v-if="!(row.info?.engines || []).length" class="fwd-muted">—</span>
        </span>
      </template>
      <template #cell-generation="{ row }">
        <span class="fwd-cell-stack nodes-end">
          <span class="fwd-mono">{{ row.reported ? num(row.reported_generation) : '—' }} / {{ num(row.desired_generation) }}</span>
          <UiBadge v-if="row.lag > 0" tone="info" :label="t('forwardV4.nodes.lag', { n: row.lag })" />
          <span v-else-if="row.reported" class="fwd-muted">{{ t('forwardV4.nodes.converged') }}</span>
          <span v-else class="fwd-muted">{{ t('forwardV4.nodes.neverReported') }}</span>
        </span>
      </template>
      <template #cell-errors="{ row }">
        <UiBadge v-if="num(row.hop_errors)" tone="danger" :label="t('forwardV4.nodes.hopErrors', { n: num(row.hop_errors) })" />
        <span v-else class="fwd-muted">—</span>
      </template>
    </UiDataTable>

    <p v-if="allRows.length" class="list-page__note">{{ t('forwardV4.nodes.note') }}</p>

    <AgentInstallSheet v-if="installNode" v-model:open="installOpen" :node="installNode.node_ref" :node-label="installNode.name" />

    <UiDialog v-model:open="createOpen" :title="t('forwardV4.nodes.createTitle')" :description="t('forwardV4.nodes.createDescription')">
      <form class="nodes-form" novalidate @submit.prevent="create">
        <UiTextField v-model="form.name" size="md" :label="t('forwardV4.nodes.name')" required :error="formErrors.name" />
        <UiTextField v-model="form.host" size="md" :label="t('forwardV4.nodes.host')" required placeholder="203.0.113.10" :error="formErrors.host" />
        <UiTextField v-model="form.region" size="md" :label="t('forwardV4.nodes.region')" />
        <div class="nodes-form__pair">
          <UiNumberField v-model="form.first" size="md" :label="t('forwardV4.nodeDetail.rangeFirst')" :min="1" :max="65535" placeholder="30000" />
          <UiNumberField v-model="form.last" size="md" :label="t('forwardV4.nodeDetail.rangeLast')" :min="1" :max="65535" placeholder="39999" />
        </div>
        <p v-if="createError" class="fwd-note is-danger" role="alert">{{ createError }}</p>
      </form>
      <template #footer="{ close }">
        <UiButton @click="close">{{ t('forwardV4.common.cancel') }}</UiButton>
        <UiButton variant="primary" :loading="creating" @click="create">{{ t('forwardV4.nodes.createRun') }}</UiButton>
      </template>
    </UiDialog>
  </div>
</template>

<script setup>
// 转发主机 on the node page (docs/design/node-unification): the machines of
// the forwarding inventory that are not regular nodes, Ansible hosts (no
// Agent) and Agent forward nodes not yet converted (转为常规节点). Regular
// nodes, relays included, are in the 节点 tab with their relay role. The
// node page owns the inventory (useForwardInventory) and reloads it after
// every change here. Generations are applied / desired.
import { computed, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ArrowRightLeft, Plus, Power, Server, Settings2, Terminal, Trash2 } from '@lucide/vue'
import { UiBadge, UiButton, UiDataTable, UiDialog, UiFilterChips, UiNumberField, UiTextField, useConfirm, useToast } from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import AgentInstallSheet from '@/components/admin/AgentInstallSheet.vue'
import { createNode, deleteNode, newIdempotencyKey, toggleNode } from '@/api/forwardV4'
import { convertForwardNode } from '@/api/kernel'
import EngineChip from '@/components/forward/EngineChip.vue'
import { forwardErrorMessage } from '@/components/forward/messages'
import { SECURITIES, nodeLag, nodeState, num } from '@/components/forward/routeModel'
import '@/components/forward/forward.css'

const props = defineProps({
  // useForwardInventory() of the node page.
  inventory: { type: Object, required: true }
})
const emit = defineEmits(['changed'])

const router = useRouter()
const { t } = useAppI18n()
const toast = useToast()
const confirm = useConfirm()

const canDelete = computed(() => props.inventory.canDelete.value)
const refresh = () => emit('changed')

const flag = ref('')

function usedBy(ref) {
  return props.inventory.routes.value.filter(item => (item.route?.hops || []).some(hop => (hop.node_refs || []).includes(ref)))
}

const allRows = computed(() => props.inventory.hosts.value.map(node => ({
  ...node,
  name: node.name || node.node_ref,
  state: nodeState(node),
  ansible: node.record?.transport === 'NODE_TRANSPORT_ANSIBLE',
  lag: nodeLag(node),
  routes: usedBy(node.node_ref)
})))

const rows = computed(() => allRows.value.filter(row => {
  if (flag.value === 'lagging' && row.lag <= 0) return false
  if (flag.value === 'errors' && !num(row.hop_errors)) return false
  return true
}))
const filtered = computed(() => Boolean(flag.value))

const flagOptions = computed(() => [
  { value: 'lagging', label: t('forwardV4.nodes.lagging'), count: allRows.value.filter(row => row.lag > 0).length },
  { value: 'errors', label: t('forwardV4.nodes.withErrors'), count: allRows.value.filter(row => num(row.hop_errors)).length }
])

const columns = computed(() => [
  { key: 'name', label: t('forwardV4.nodes.columns.node'), primary: true, sortable: true },
  { key: 'status', label: t('forwardV4.nodes.columns.status'), secondary: true },
  { key: 'kind', label: t('forwardV4.nodes.columns.kind'), breakpoint: 'lg' },
  { key: 'agent', label: t('forwardV4.nodes.columns.agent'), breakpoint: 'lg' },
  { key: 'engines', label: t('forwardV4.nodes.columns.engines') },
  { key: 'desired_hops', label: t('forwardV4.nodes.columns.hops'), align: 'end', numeric: true, sortable: true, breakpoint: 'lg', sortValue: row => num(row.desired_hops) },
  { key: 'generation', label: t('forwardV4.nodes.columns.generation'), align: 'end', sortable: true, sortValue: row => row.lag },
  { key: 'errors', label: t('forwardV4.nodes.columns.errors'), sortable: true, breakpoint: 'md', sortValue: row => num(row.hop_errors) }
])

// ---------------------------------------------------------------------------
// Row actions
// ---------------------------------------------------------------------------

const installOpen = ref(false)
const installNode = ref(null)

function rowActions(row) {
  const inUse = row.routes.length > 0
  return [
    { key: 'settings', label: t('forwardV4.nodes.settings'), icon: Settings2, onSelect: () => router.push(`/admin/forward/inventory/${row.node_ref}`) },
    { key: 'install', label: t('forwardV4.nodes.install'), icon: Terminal, hidden: row.ansible, onSelect: () => { installNode.value = row; installOpen.value = true } },
    {
      key: 'toggle',
      label: row.enabled === false ? t('forwardV4.nodes.enable') : (inUse ? t('forwardV4.nodes.disableInUse') : t('forwardV4.nodes.disable')),
      icon: Power,
      separatorBefore: true,
      disabled: row.enabled !== false && inUse,
      onSelect: () => toggle(row)
    },
    {
      key: 'convert',
      label: canDelete.value === false ? t('forwardV4.nodes.convertSuperOnly') : t('forwardV4.nodes.convert'),
      icon: ArrowRightLeft,
      hidden: row.ansible,
      disabled: canDelete.value === false,
      onSelect: () => convert(row)
    },
    {
      key: 'delete',
      label: canDelete.value === false ? t('forwardV4.actions.deleteSuperOnly') : (inUse ? t('forwardV4.nodes.deleteInUse') : t('forwardV4.nodes.delete')),
      icon: Trash2,
      danger: true,
      disabled: inUse || canDelete.value === false,
      onSelect: () => remove(row)
    }
  ]
}

async function toggle(row) {
  const enable = row.enabled === false
  const run = async () => {
    try {
      await toggleNode(row.node_ref, enable, { idempotencyKey: newIdempotencyKey() })
    } catch (error) {
      throw new Error(forwardErrorMessage(t, error))
    }
    toast.success(t(enable ? 'forwardV4.toast.nodeEnabled' : 'forwardV4.toast.nodeDisabled', { name: row.name }))
    await refresh()
  }
  if (enable) {
    await run().catch(error => toast.error(error.message))
    return
  }
  await confirm({
    title: t('forwardV4.nodes.disableTitle', { name: row.name }),
    message: t('forwardV4.nodes.disableMessage'),
    confirmLabel: t('forwardV4.nodes.disable'),
    tone: 'danger',
    onConfirm: run
  })
}

async function remove(row) {
  await confirm({
    title: t('forwardV4.nodes.deleteTitle', { name: row.name }),
    message: t('forwardV4.nodes.deleteMessage'),
    confirmLabel: t('forwardV4.nodes.delete'),
    tone: 'danger',
    requireText: row.name,
    onConfirm: async () => {
      try {
        await deleteNode(row.node_ref, { idempotencyKey: newIdempotencyKey() })
      } catch (error) {
        throw new Error(forwardErrorMessage(t, error))
      }
      toast.success(t('forwardV4.toast.nodeDeleted', { name: row.name }))
      await refresh()
    }
  })
}

// A forward node becomes a regular node without protocols
// (docs/design/node-unification): a dry run names the new node, then the
// conversion runs and the new node's install command opens.
async function convert(row) {
  let preview
  try {
    preview = await convertForwardNode(row.node_ref, true)
  } catch (error) {
    toast.error(forwardErrorMessage(t, error))
    return
  }
  await confirm({
    title: t('forwardV4.nodes.convertTitle', { name: row.name }),
    message: t('forwardV4.nodes.convertMessage', { to: preview?.to || '', n: preview?.routes?.length || 0 }),
    confirmLabel: t('forwardV4.nodes.convertRun'),
    tone: 'danger',
    requireText: row.name,
    onConfirm: async () => {
      let answer
      try {
        answer = await convertForwardNode(row.node_ref, false)
      } catch (error) {
        throw new Error(forwardErrorMessage(t, error))
      }
      toast.success(t('forwardV4.toast.nodeConverted', { name: row.name, to: answer?.to || '' }))
      await refresh()
      if (answer?.to) {
        installNode.value = { node_ref: answer.to, name: row.name }
        installOpen.value = true
      }
    }
  })
}

// ---------------------------------------------------------------------------
// Add an Ansible host (a machine that runs the Agent is a regular node)
// ---------------------------------------------------------------------------

const createOpen = ref(false)
const creating = ref(false)
const createError = ref('')
const form = reactive({ name: '', host: '', transport: 'NODE_TRANSPORT_ANSIBLE', region: '', first: null, last: null })
const formErrors = reactive({ name: '', host: '' })
let createKey = newIdempotencyKey()

function openCreate() {
  Object.assign(form, { name: '', host: '', transport: 'NODE_TRANSPORT_ANSIBLE', region: '', first: null, last: null })
  createError.value = ''
  formErrors.name = ''
  formErrors.host = ''
  createKey = newIdempotencyKey()
  createOpen.value = true
}

async function create() {
  formErrors.name = form.name.trim() ? '' : t('forwardV4.codes.required')
  formErrors.host = form.host.trim() ? '' : t('forwardV4.codes.required')
  if (formErrors.name || formErrors.host) return
  creating.value = true
  createError.value = ''
  try {
    const settings = form.first && form.last ? { port_range: { first: form.first, last: form.last } } : null
    const answer = await createNode({ name: form.name, host: form.host, transport: form.transport, region: form.region, enabled: true }, settings, { idempotencyKey: createKey })
    createOpen.value = false
    toast.success(t('forwardV4.toast.nodeCreated', { name: form.name }))
    const ref = answer?.node?.node_ref
    refresh()
    if (ref) router.push(`/admin/forward/inventory/${ref}`)
  } catch (error) {
    createError.value = forwardErrorMessage(t, error)
  } finally {
    creating.value = false
  }
}
</script>

<style scoped>
.hosts-panel__summary {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
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

.nodes-form {
  display: grid;
  gap: var(--space-3);
}

.nodes-form__pair {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--space-3);
}

@media (max-width: 639.98px) {
  .nodes-end {
    justify-items: start;
  }
}
</style>
