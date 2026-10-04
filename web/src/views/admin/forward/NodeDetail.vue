<template>
  <section class="fwd-page" data-testid="forward-node-detail">
    <UiErrorState v-if="loadError && !node" :title="t('forwardV4.nodeDetail.loadFailed')" :error="loadError" @retry="refresh" />
    <UiSkeleton v-else-if="!node" variant="card" />

    <template v-else>
      <UiPageHeader :title="node.name || node.node_ref">
        <template #back>
          <RouterLink class="fwd-back" to="/admin/forward/inventory"><ChevronLeft :size="16" aria-hidden="true" /> {{ t('forwardV4.nav.nodes') }}</RouterLink>
        </template>
        <template #meta>
          <UiBadge :status="nodeState(node)" />
          <UiBadge :tone="isProxy ? 'info' : 'neutral'" :dot="false" :label="isProxy ? t('forwardV4.nodes.proxy') : t('forwardV4.nodes.forward')" />
          <span class="fwd-mono node-ref">{{ [node.node_ref, node.host].filter(Boolean).join(' · ') }}</span>
        </template>
        <template #actions>
          <span class="fwd-muted node-updated">{{ secondsAgo !== null && secondsAgo < 5 ? t('forwardV4.updated.now') : t('forwardV4.updated.ago', { n: secondsAgo ?? 0 }) }}</span>
          <UiButton v-if="!ansible" :icon="Terminal" data-testid="forward-node-install" @click="installOpen = true">{{ t('forwardV4.nodeDetail.install') }}</UiButton>
          <UiButton :icon="RefreshCw" :disabled="loading" @click="refresh">{{ t('forwardV4.updated.refresh') }}</UiButton>
        </template>
      </UiPageHeader>

      <div class="node-layout">
        <div class="node-col">
          <UiCard :title="t('forwardV4.nodeDetail.health')" :description="healthDescription">
            <div class="node-health">
              <div class="node-health__item">
                <span class="fwd-muted">{{ t('forwardV4.nodeDetail.generation') }}</span>
                <span class="node-health__value">{{ node.reported ? num(node.reported_generation) : '—' }} / {{ num(node.desired_generation) }}</span>
                <UiBadge v-if="!node.reported" tone="neutral" :label="t('forwardV4.nodes.neverReported')" />
                <UiBadge v-else :tone="lag ? 'info' : 'success'" :label="lag ? t('forwardV4.nodes.lag', { n: lag }) : t('forwardV4.nodes.converged')" />
              </div>
              <div class="node-health__item">
                <span class="fwd-muted">{{ t('forwardV4.nodeDetail.stateHash') }}</span>
                <span class="node-health__value fwd-mono">{{ short(node.reported_state_hash) || '—' }}</span>
                <span class="fwd-muted">{{ !node.reported ? '' : (node.reported_state_hash === node.desired_state_hash ? t('forwardV4.nodeDetail.hashMatches') : t('forwardV4.nodeDetail.hashExpected', { hash: short(node.desired_state_hash) })) }}</span>
              </div>
              <div class="node-health__item">
                <span class="fwd-muted">{{ t('forwardV4.nodeDetail.hops') }}</span>
                <span class="node-health__value">{{ num(node.desired_hops) }}</span>
                <UiBadge :tone="errors.length ? 'danger' : 'success'" :label="errors.length ? t('forwardV4.nodes.hopErrors', { n: errors.length }) : t('forwardV4.nodeDetail.noErrors')" />
              </div>
              <div class="node-health__item">
                <span class="fwd-muted">{{ t('forwardV4.nodeDetail.upstreams') }}</span>
                <span class="node-health__value">{{ healthyUpstreams }} / {{ upstreams.length }}</span>
                <span class="fwd-muted">{{ t('forwardV4.nodeDetail.healthyTotal') }}</span>
              </div>
            </div>
            <ul v-if="errors.length" class="fwd-list node-errors">
              <li v-for="(error, index) in errors" :key="index" class="fwd-list__item">
                <span class="fwd-cell-stack">
                  <span><AlertTriangle :size="14" class="node-errors__icon" aria-hidden="true" /> {{ routeName(error.route_id) }} · {{ t('forwardV4.diagnose.hop', { n: num(error.hop_index) + 1 }) }}</span>
                  <span class="fwd-mono node-errors__msg">{{ error.message }}</span>
                </span>
                <UiButton size="sm" @click="router.push(`/admin/forward/routes/${error.route_id}`)">{{ t('forwardV4.overview.viewRoute') }}</UiButton>
              </li>
            </ul>
          </UiCard>

          <UiCard :title="t('forwardV4.nodeDetail.hostedHops')" :description="t('forwardV4.nodeDetail.hostedHopsDescription')">
            <UiDataTable :columns="hopColumns" :rows="hops" :label="t('forwardV4.nodeDetail.hostedHops')" row-key="key" flat :settings="false" :sticky-header="false" :empty-title="t('forwardV4.nodeDetail.noHops')">
              <template #cell-route="{ row }">
                <RouterLink class="fwd-link" :to="`/admin/forward/routes/${row.route_id}`">{{ row.route }}</RouterLink>
              </template>
              <template #cell-engine="{ row }"><EngineChip :engine="row.engine" /></template>
              <template #cell-state="{ row }">
                <UiBadge :tone="row.paused ? 'neutral' : 'success'" :label="row.paused ? t('forwardV4.status.paused') : t('forwardV4.nodeDetail.running')" />
              </template>
            </UiDataTable>
          </UiCard>

          <UiCard :title="t('forwardV4.nodeDetail.engines')" :description="enginesDescription">
            <p v-if="!engines.length" class="fwd-muted">{{ t('forwardV4.nodeDetail.noEngines') }}</p>
            <ul v-else class="fwd-list">
              <li v-for="engine in engines" :key="engine.engine" class="fwd-list__item">
                <span class="fwd-cell-stack">
                  <span class="node-engine-head"><EngineChip :engine="engine.engine" :unavailable="!engine.available" /> <span class="fwd-mono">{{ engine.version }}</span></span>
                  <span class="fwd-muted">{{ t('forwardV4.nodeDetail.links', { list: (engine.link_securities || []).map(s => SECURITIES[s] || s).join(' / ') || '—' }) }}</span>
                  <span class="fwd-muted">{{ t('forwardV4.nodeDetail.strategies', { list: (engine.strategies || []).map(s => t(`forwardV4.strategy.${s}`)).join(', ') || '—' }) }}</span>
                </span>
                <span class="node-caps">
                  <UiBadge v-if="!engine.available" tone="danger" :label="engine.unavailable_reason || t('forwardV4.engine.unavailable')" />
                  <template v-else>
                    <UiBadge v-for="cap in capsOf(engine)" :key="cap" tone="neutral" :dot="false" :label="cap" />
                  </template>
                </span>
              </li>
            </ul>
          </UiCard>
        </div>

        <div class="node-col">
          <UiCard :title="t('forwardV4.nodeDetail.settings')" :description="t('forwardV4.nodeDetail.settingsDescription')">
            <form class="node-form" novalidate data-testid="forward-node-settings" @submit.prevent="saveSettings">
              <div class="node-form__pair">
                <UiNumberField v-model="form.first" size="md" :label="t('forwardV4.nodeDetail.rangeFirst')" :min="1" :max="65535" placeholder="30000" :error="settingsError('port_range.first')" />
                <UiNumberField v-model="form.last" size="md" :label="t('forwardV4.nodeDetail.rangeLast')" :min="1" :max="65535" placeholder="39999" :error="settingsError('port_range.last')" />
              </div>
              <UiTextField v-model="form.reserved" size="md" :label="t('forwardV4.nodeDetail.reserved')" placeholder="22, 80, 443" :help="t('forwardV4.nodeDetail.reservedHelp')" :error="reservedError || settingsError('reserved_ports')" />
              <UiTextarea v-model="form.addresses" :label="t('forwardV4.nodeDetail.addresses')" :rows="3" :help="t('forwardV4.nodeDetail.addressesHelp')" :error="settingsError('addresses')" />
              <UiField :label="t('forwardV4.nodeDetail.labels')" label-tag="span" :help="t('forwardV4.nodeDetail.labelsHelp')">
                <template #default="{ labelId }">
                  <div class="node-labels" role="group" :aria-labelledby="labelId">
                    <span v-for="(item, index) in form.labels" :key="item._key" class="node-label">
                      <UiTextField v-model="item.key" size="sm" :aria-label="t('forwardV4.editor.labelKey', { n: index + 1 })" placeholder="key" />
                      <span aria-hidden="true">=</span>
                      <UiTextField v-model="item.value" size="sm" :aria-label="t('forwardV4.editor.labelValue', { n: index + 1 })" placeholder="value" />
                      <UiIconButton size="sm" :icon="X" :label="t('forwardV4.editor.removeLabel', { n: index + 1 })" @click="form.labels.splice(index, 1)" />
                    </span>
                    <UiButton size="sm" variant="tertiary" :icon="Plus" @click="form.labels.push({ _key: `l${Date.now()}${form.labels.length}`, key: '', value: '' })">{{ t('forwardV4.editor.addLabel') }}</UiButton>
                  </div>
                </template>
              </UiField>
              <p class="fwd-note">{{ t('forwardV4.nodeDetail.replanNote') }}</p>
              <div v-if="settingsViolations.length" class="fwd-note is-warning" role="status" data-testid="forward-settings-violations">
                <p class="node-violations__title">{{ t('forwardV4.nodeDetail.replanWarnings', { n: settingsViolations.length }) }}</p>
                <ul class="node-violations">
                  <li v-for="(item, index) in settingsViolations" :key="index">
                    <RouterLink v-if="item.route_id" class="fwd-link" :to="`/admin/forward/routes/${item.route_id}`">{{ routeName(item.route_id) }}</RouterLink>
                    <span class="fwd-mono">{{ item.code }}</span> {{ violationText(t, te, item) }}
                  </li>
                </ul>
              </div>
              <p v-if="settingsFailure" class="fwd-note is-danger" role="alert">{{ settingsFailure }}</p>
              <div class="node-form__actions">
                <UiButton :disabled="!settingsDirty || savingSettings" @click="resetForm">{{ t('forwardV4.nodeDetail.discard') }}</UiButton>
                <UiButton variant="primary" type="submit" :loading="savingSettings" :disabled="!settingsDirty" data-testid="forward-node-settings-save">{{ t('forwardV4.nodeDetail.save') }}</UiButton>
              </div>
            </form>
          </UiCard>

          <UiCard :title="t('forwardV4.nodeDetail.agent')">
            <UiGroupedList>
              <UiGroupedListRow :label="t('forwardV4.nodeDetail.transport')" :value="ansible ? t('forwardV4.nodes.ansible') : t('forwardV4.nodes.agent')" />
              <UiGroupedListRow :label="t('forwardV4.nodeDetail.version')" :value="node.capabilities?.agent_version || '—'" />
              <UiGroupedListRow :label="t('forwardV4.nodeDetail.negotiated')" :value="node.negotiated ? t('forwardV4.nodeDetail.negotiatedYes') : t('forwardV4.nodeDetail.negotiatedNo')" />
            </UiGroupedList>
            <template v-if="!ansible">
              <p class="fwd-muted node-install">{{ t('forwardV4.nodeDetail.installHelp') }}</p>
              <UiButton :icon="Terminal" @click="installOpen = true">{{ t('forwardV4.nodeDetail.installCommand') }}</UiButton>
            </template>
          </UiCard>

          <UiCard v-if="isProxy" :title="t('forwardV4.nodeDetail.proxyTitle')">
            <p class="fwd-muted">{{ t('forwardV4.nodeDetail.proxyHelp') }}</p>
            <RouterLink v-if="proxyId" class="fwd-link" :to="`/admin/nodes/${proxyId}`">{{ t('forwardV4.nodeDetail.proxyLink') }}</RouterLink>
          </UiCard>

          <UiCard v-else :title="t('forwardV4.nodeDetail.danger')">
            <p class="fwd-note">{{ usedBy.length ? t('forwardV4.nodeDetail.inUse', { name: node.name, n: usedBy.length, routes: usedBy.map(item => item.route.name).join(', ') }) : t('forwardV4.nodeDetail.notInUse') }}</p>
            <div class="node-danger">
              <UiButton variant="danger-soft" :icon="Power" :disabled="node.enabled !== false && usedBy.length > 0" @click="toggle">{{ node.enabled === false ? t('forwardV4.nodes.enable') : t('forwardV4.nodeDetail.disable') }}</UiButton>
              <UiButton variant="danger-soft" :icon="Trash2" :disabled="usedBy.length > 0 || canDelete === false" @click="remove">{{ canDelete === false ? t('forwardV4.actions.deleteSuperOnly') : t('forwardV4.nodeDetail.delete') }}</UiButton>
            </div>
          </UiCard>
        </div>
      </div>
    </template>

    <AgentInstallSheet v-if="node" v-model:open="installOpen" :node="node.node_ref" :node-label="node.name" />
  </section>
</template>

<script setup>
// 节点详情 (F5b): the node's health (applied / desired generation, state
// hash, hop errors, upstreams), the hops it hosts, its engines, and its
// forwarding settings (PUT /nodes/{ref}/settings). A proxy node shows only
// its forwarding settings; its record belongs to the proxy node page (D15).
import { computed, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { AlertTriangle, ChevronLeft, Plus, Power, RefreshCw, Terminal, Trash2, X } from '@lucide/vue'
import {
  UiBadge, UiButton, UiCard, UiDataTable, UiErrorState, UiField, UiGroupedList, UiGroupedListRow, UiIconButton, UiNumberField,
  UiPageHeader, UiSkeleton, UiTextarea, UiTextField, useConfirm, useFormat, useToast
} from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import { useUnsavedChanges } from '@/composables/useUnsavedChanges'
import AgentInstallSheet from '@/components/admin/AgentInstallSheet.vue'
import { deleteNode, getNode, listRoutes, newIdempotencyKey, setNodeSettings, settingsBody, toggleNode } from '@/api/forwardV4'
import EngineChip from '@/components/forward/EngineChip.vue'
import { forwardErrorMessage, violationText } from '@/components/forward/messages'
import { usePolling } from '@/components/forward/usePolling'
import { SECURITIES, nodeLag, nodeState, num, roleOf } from '@/components/forward/routeModel'
import '@/components/forward/forward.css'

const props = defineProps({
  nodeRef: { type: String, required: true }
})

const router = useRouter()
const { t, te } = useAppI18n()
const fmt = useFormat()
const toast = useToast()
const confirm = useConfirm()

const view = ref(null)
const routes = ref([])
const canDelete = ref(null)
const loadError = ref('')
const installOpen = ref(false)

const { loading, secondsAgo, refresh } = usePolling(async () => {
  try {
    const [answer, routeAnswer] = await Promise.all([getNode(props.nodeRef), listRoutes({ nodeRef: props.nodeRef })])
    view.value = answer
    routes.value = routeAnswer.routes
    canDelete.value = routeAnswer.canDelete
    loadError.value = ''
    if (!settingsDirty.value) fillForm()
  } catch (error) {
    loadError.value = forwardErrorMessage(t, error)
    throw error
  }
}, { interval: 15_000 })

const node = computed(() => view.value?.node || null)
const report = computed(() => view.value?.report || null)
const isProxy = computed(() => node.value?.kind === 'proxy')
const proxyId = computed(() => (isProxy.value ? String(node.value.node_ref).replace(/^proxy-/, '') : ''))
const ansible = computed(() => node.value?.record?.transport === 'NODE_TRANSPORT_ANSIBLE')
const lag = computed(() => nodeLag(node.value))
const errors = computed(() => report.value?.errors || [])
const upstreams = computed(() => report.value?.health || [])
const healthyUpstreams = computed(() => upstreams.value.filter(item => item.state === 'HEALTH_STATE_HEALTHY').length)
const engines = computed(() => node.value?.capabilities?.engines || node.value?.info?.engines || [])
const usedBy = computed(() => routes.value)
const routeName = id => routes.value.find(item => item.route?.id === id)?.route?.name || id
const short = hash => (hash ? String(hash).slice(0, 12) : '')

const healthDescription = computed(() => (node.value?.reported
  ? t('forwardV4.nodeDetail.lastReport', { when: fmt.relativeTime(num(node.value.reported_at_unix_ms)), at: fmt.dateTime(num(node.value.reported_at_unix_ms)) })
  : t('forwardV4.nodes.neverReported')))
const enginesDescription = computed(() => {
  const caps = node.value?.capabilities || {}
  return t('forwardV4.nodeDetail.enginesDescription', { agent: caps.agent_version || '—', kernel: caps.kernel_version || '—', cgroup: caps.cgroup || '—' })
})

function capsOf(engine) {
  return [
    engine.udp && 'UDP',
    engine.ipv6 && 'IPv6',
    engine.bandwidth_limit && t('forwardV4.nodeDetail.capBandwidth'),
    engine.quota && t('forwardV4.nodeDetail.capQuota'),
    engine.max_conns && t('forwardV4.nodeDetail.capConns')
  ].filter(Boolean)
}

const hops = computed(() => (view.value?.state?.hops || []).map(hop => {
  const route = routes.value.find(item => item.route?.id === hop.route_id)?.route
  const index = num(hop.hop_index)
  return {
    key: `${hop.route_id}-${index}`,
    route_id: hop.route_id,
    route: route?.name || hop.route_id,
    role: t('forwardV4.nodeDetail.roleAt', { role: t(`forwardV4.role.${hop.role || roleOf(index, route?.hops?.length || 1)}`), n: index + 1 }),
    engine: hop.engine,
    port: num(hop.listen?.port),
    mark: num(hop.mark),
    paused: Boolean(hop.paused)
  }
}))
const hopColumns = computed(() => [
  { key: 'route', label: t('forwardV4.nodeDetail.columns.route'), primary: true },
  { key: 'role', label: t('forwardV4.nodeDetail.columns.role'), secondary: true },
  { key: 'engine', label: t('forwardV4.nodeDetail.columns.engine') },
  { key: 'port', label: t('forwardV4.nodeDetail.columns.port'), align: 'end', numeric: true },
  { key: 'mark', label: t('forwardV4.nodeDetail.columns.mark'), align: 'end', numeric: true, breakpoint: 'md' },
  { key: 'state', label: t('forwardV4.nodeDetail.columns.state') }
])

// ---------------------------------------------------------------------------
// Settings
// ---------------------------------------------------------------------------

const form = reactive({ first: null, last: null, reserved: '', addresses: '', labels: [] })
const baseSettings = ref('')
const savingSettings = ref(false)
const settingsViolations = ref([])
const settingsFailure = ref('')
const reservedError = ref('')
let settingsKey = newIdempotencyKey()
let settingsAttempt = null

function fillForm() {
  const settings = node.value?.settings || {}
  form.first = num(settings.port_range?.first) || null
  form.last = num(settings.port_range?.last) || null
  form.reserved = (settings.reserved_ports || []).join(', ')
  form.addresses = (settings.addresses || []).join('\n')
  form.labels = Object.entries(settings.labels || {}).map(([key, value], index) => ({ _key: `s${index}`, key, value }))
  baseSettings.value = JSON.stringify(settingsBody(formSettings()))
}

function parseReserved(text) {
  const parts = String(text || '').split(/[\s,，]+/).filter(Boolean)
  const ports = parts.map(Number)
  return ports.every(port => Number.isInteger(port) && port >= 1 && port <= 65535) ? ports : null
}

function formSettings() {
  const labels = {}
  for (const item of form.labels) {
    const key = String(item.key || '').trim()
    if (key) labels[key] = String(item.value ?? '').trim()
  }
  return {
    port_range: form.first || form.last ? { first: form.first || 0, last: form.last || 0 } : undefined,
    reserved_ports: parseReserved(form.reserved) || [],
    addresses: String(form.addresses || '').split(/\n+/).map(line => line.trim()).filter(Boolean),
    labels
  }
}

const settingsDirty = computed(() => Boolean(node.value) && JSON.stringify(settingsBody(formSettings())) !== baseSettings.value)
useUnsavedChanges(settingsDirty, { discard: () => {} })

function settingsError(field) {
  const hit = settingsViolations.value.find(item => item.field === field && !item.route_id)
  return hit ? violationText(t, te, hit) : ''
}

function resetForm() {
  settingsViolations.value = []
  settingsFailure.value = ''
  reservedError.value = ''
  fillForm()
}

async function saveSettings() {
  reservedError.value = parseReserved(form.reserved) ? '' : t('forwardV4.nodeDetail.reservedInvalid')
  if (reservedError.value) return
  const settings = formSettings()
  const body = JSON.stringify(settingsBody(settings))
  if (settingsAttempt !== null && settingsAttempt !== body) settingsKey = newIdempotencyKey()
  settingsAttempt = body
  savingSettings.value = true
  settingsFailure.value = ''
  try {
    const answer = await setNodeSettings(props.nodeRef, settings, { idempotencyKey: settingsKey })
    settingsKey = newIdempotencyKey()
    settingsAttempt = null
    settingsViolations.value = answer?.violations || []
    if (answer?.node) view.value = { ...view.value, node: answer.node }
    fillForm()
    toast.success(settingsViolations.value.length ? t('forwardV4.toast.settingsSavedWarnings', { n: settingsViolations.value.length }) : t('forwardV4.toast.settingsSaved'))
  } catch (error) {
    settingsViolations.value = error.violations || []
    settingsFailure.value = forwardErrorMessage(t, error)
  } finally {
    savingSettings.value = false
  }
}

// ---------------------------------------------------------------------------
// Danger zone (forward nodes)
// ---------------------------------------------------------------------------

async function toggle() {
  const enable = node.value.enabled === false
  const run = async () => {
    try {
      await toggleNode(props.nodeRef, enable, { idempotencyKey: newIdempotencyKey() })
    } catch (error) {
      throw new Error(forwardErrorMessage(t, error))
    }
    toast.success(t(enable ? 'forwardV4.toast.nodeEnabled' : 'forwardV4.toast.nodeDisabled', { name: node.value.name }))
    await refresh()
  }
  if (enable) {
    await run().catch(error => toast.error(error.message))
    return
  }
  await confirm({
    title: t('forwardV4.nodes.disableTitle', { name: node.value.name }),
    message: t('forwardV4.nodes.disableMessage'),
    confirmLabel: t('forwardV4.nodes.disable'),
    tone: 'danger',
    onConfirm: run
  })
}

async function remove() {
  const name = node.value.name || props.nodeRef
  await confirm({
    title: t('forwardV4.nodes.deleteTitle', { name }),
    message: t('forwardV4.nodes.deleteMessage'),
    confirmLabel: t('forwardV4.nodes.delete'),
    tone: 'danger',
    requireText: name,
    onConfirm: async () => {
      try {
        await deleteNode(props.nodeRef, { idempotencyKey: newIdempotencyKey() })
      } catch (error) {
        throw new Error(forwardErrorMessage(t, error))
      }
      toast.success(t('forwardV4.toast.nodeDeleted', { name }))
      router.push('/admin/forward/inventory')
    }
  })
}
</script>

<style scoped>
.node-ref {
  color: var(--label-3);
}

.node-updated {
  align-self: center;
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
  display: grid;
  gap: var(--space-2);
  justify-items: start;
}

.node-label {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr) auto;
  gap: var(--space-1);
  align-items: center;
  width: 100%;
  color: var(--label-2);
}

.node-violations__title {
  margin: 0 0 var(--space-1);
  font-weight: var(--weight-semibold);
}

.node-violations {
  display: grid;
  gap: var(--space-1);
  margin: 0;
  padding-left: var(--space-4);
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
