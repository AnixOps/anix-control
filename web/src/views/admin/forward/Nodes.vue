<template>
  <section class="fwd-page" data-testid="forward-nodes">
    <ForwardAreaNav area="nodes" :seconds-ago="secondsAgo" :loading="loading" refreshable @refresh="refresh" />

    <UiPageHeader :title="t('forwardV4.nodes.title')" :description="t('forwardV4.nodes.description')">
      <template #actions>
        <UiButton variant="primary" :icon="Plus" data-testid="forward-add-node" @click="openCreate">{{ t('forwardV4.nodes.add') }}</UiButton>
      </template>
    </UiPageHeader>

    <UiDataTable
      :columns="columns"
      :rows="rows"
      row-key="node_ref"
      :label="t('forwardV4.nodes.title')"
      :row-label="row => row.name"
      :row-actions="rowActions"
      :loading="loading && !loaded"
      :error="loadError"
      :error-title="t('forwardV4.nodes.loadFailed')"
      :filtered="filtered"
      :empty-icon="Server"
      :empty-title="t('forwardV4.nodes.emptyTitle')"
      :empty-description="t('forwardV4.nodes.emptyDescription')"
      activatable
      :card-fields="4"
      @row-activate="row => router.push(`/admin/forward/inventory/${row.node_ref}`)"
      @clear-filters="kind = ''; flag = ''"
      @retry="refresh"
    >
      <template v-if="allRows.length" #toolbar>
        <div class="nodes-toolbar">
          <UiFilterChips v-model="kind" :label="t('forwardV4.nodes.kindFilter')" :options="kindOptions" />
          <UiFilterChips v-model="flag" :label="t('forwardV4.nodes.flagFilter')" :options="flagOptions" />
        </div>
      </template>
      <template #empty-actions>
        <UiButton variant="primary" :icon="Plus" @click="openCreate">{{ t('forwardV4.nodes.add') }}</UiButton>
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
        <span class="fwd-cell-stack">
          <span><UiBadge v-if="row.kind === 'proxy'" tone="info" :dot="false" :label="t('forwardV4.nodes.proxy')" /><template v-else>{{ t('forwardV4.nodes.forward') }}</template></span>
          <span class="fwd-muted">{{ row.ansible ? t('forwardV4.nodes.ansible') : t('forwardV4.nodes.agent') }}</span>
        </span>
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
        <UiRadioGroup v-model="form.transport" :label="t('forwardV4.nodes.transport')" :options="transportOptions" />
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
  </section>
</template>

<script setup>
// 节点 (F5b): the forwarding inventory, forward nodes and proxy nodes that
// joined it (D15, 「代理节点」). Generations are applied / desired; the page
// polls every 15 s while visible (D5).
import { computed, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Plus, Power, Server, Settings2, Terminal, Trash2 } from '@lucide/vue'
import { UiBadge, UiButton, UiDataTable, UiDialog, UiFilterChips, UiNumberField, UiPageHeader, UiRadioGroup, UiTextField, useConfirm, useToast } from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import AgentInstallSheet from '@/components/admin/AgentInstallSheet.vue'
import { createNode, deleteNode, listNodes, listRoutes, newIdempotencyKey, toggleNode } from '@/api/forwardV4'
import EngineChip from '@/components/forward/EngineChip.vue'
import ForwardAreaNav from '@/components/forward/ForwardAreaNav.vue'
import { forwardErrorMessage } from '@/components/forward/messages'
import { usePolling } from '@/components/forward/usePolling'
import { SECURITIES, nodeLag, nodeState, num } from '@/components/forward/routeModel'
import '@/components/forward/forward.css'

const router = useRouter()
const { t } = useAppI18n()
const toast = useToast()
const confirm = useConfirm()

const nodes = ref([])
const routes = ref([])
const canDelete = ref(null)
const loaded = ref(false)
const loadError = ref(null)
const { loading, secondsAgo, refresh } = usePolling(async () => {
  try {
    const [nodeAnswer, routeAnswer] = await Promise.all([listNodes(), listRoutes()])
    nodes.value = nodeAnswer.nodes
    canDelete.value = nodeAnswer.canDelete
    routes.value = routeAnswer.routes
    loaded.value = true
    loadError.value = null
  } catch (error) {
    loadError.value = { message: forwardErrorMessage(t, error) }
    throw error
  }
}, { interval: 15_000 })

const kind = ref('')
const flag = ref('')

function usedBy(ref) {
  return routes.value.filter(item => (item.route?.hops || []).some(hop => (hop.node_refs || []).includes(ref)))
}

const allRows = computed(() => nodes.value.map(node => ({
  ...node,
  name: node.name || node.node_ref,
  state: nodeState(node),
  ansible: node.record?.transport === 'NODE_TRANSPORT_ANSIBLE',
  lag: nodeLag(node),
  routes: usedBy(node.node_ref)
})))

const rows = computed(() => allRows.value.filter(row => {
  if (kind.value && row.kind !== kind.value) return false
  if (flag.value === 'lagging' && row.lag <= 0) return false
  if (flag.value === 'errors' && !num(row.hop_errors)) return false
  return true
}))
const filtered = computed(() => Boolean(kind.value || flag.value))

const kindOptions = computed(() => [
  { value: 'forward', label: t('forwardV4.nodes.forward'), count: allRows.value.filter(row => row.kind === 'forward').length },
  { value: 'proxy', label: t('forwardV4.nodes.proxy'), count: allRows.value.filter(row => row.kind === 'proxy').length }
])
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
  const actions = [
    { key: 'settings', label: t('forwardV4.nodes.settings'), icon: Settings2, onSelect: () => router.push(`/admin/forward/inventory/${row.node_ref}`) },
    { key: 'install', label: t('forwardV4.nodes.install'), icon: Terminal, hidden: row.ansible, onSelect: () => { installNode.value = row; installOpen.value = true } }
  ]
  if (row.kind === 'forward') {
    actions.push(
      {
        key: 'toggle',
        label: row.enabled === false ? t('forwardV4.nodes.enable') : (inUse ? t('forwardV4.nodes.disableInUse') : t('forwardV4.nodes.disable')),
        icon: Power,
        separatorBefore: true,
        disabled: row.enabled !== false && inUse,
        onSelect: () => toggle(row)
      },
      {
        key: 'delete',
        label: canDelete.value === false ? t('forwardV4.actions.deleteSuperOnly') : (inUse ? t('forwardV4.nodes.deleteInUse') : t('forwardV4.nodes.delete')),
        icon: Trash2,
        danger: true,
        disabled: inUse || canDelete.value === false,
        onSelect: () => remove(row)
      }
    )
  }
  return actions
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

// ---------------------------------------------------------------------------
// Add a forward node
// ---------------------------------------------------------------------------

const createOpen = ref(false)
const creating = ref(false)
const createError = ref('')
const form = reactive({ name: '', host: '', transport: 'NODE_TRANSPORT_AGENT', region: '', first: null, last: null })
const formErrors = reactive({ name: '', host: '' })
let createKey = newIdempotencyKey()
const transportOptions = computed(() => [
  { value: 'NODE_TRANSPORT_AGENT', label: t('forwardV4.nodes.agent'), description: t('forwardV4.nodes.agentHelp') },
  { value: 'NODE_TRANSPORT_ANSIBLE', label: t('forwardV4.nodes.ansible'), description: t('forwardV4.nodes.ansibleHelp') }
])

function openCreate() {
  Object.assign(form, { name: '', host: '', transport: 'NODE_TRANSPORT_AGENT', region: '', first: null, last: null })
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
    if (ref) router.push(`/admin/forward/inventory/${ref}`)
    else await refresh()
  } catch (error) {
    createError.value = forwardErrorMessage(t, error)
  } finally {
    creating.value = false
  }
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
