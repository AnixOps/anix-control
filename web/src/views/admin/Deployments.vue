<template>
  <section class="deployment-center" :aria-busy="loading ? 'true' : 'false'">
    <header class="page-header">
      <div>
        <h1>{{ t('pageTitles.admin.deployments') }}</h1>
        <p class="page-subtitle">{{ t('control.subtitle') }}</p>
      </div>
      <button
        class="icon-button"
        data-testid="refresh-deployments"
        type="button"
        :aria-label="t('control.actions.refresh')"
        :title="t('control.actions.refresh')"
        :disabled="loading"
        @click="loadInitial()"
      >
        <RefreshCw :class="{ spinning: loading }" :size="18" aria-hidden="true" />
        <span class="sr-only">{{ t('control.actions.refresh') }}</span>
      </button>
    </header>

    <nav class="view-switcher" role="tablist" :aria-label="t('pageTitles.admin.deployments')">
      <button
        class="view-tab"
        data-testid="deployment-topologies"
        type="button"
        role="tab"
        :aria-selected="viewMode === 'topologies'"
        :tabindex="viewMode === 'topologies' ? 0 : -1"
        @click="setViewMode('topologies')"
      >
        {{ t('control.tabs.topologies') }}
      </button>
      <button
        class="view-tab"
        data-testid="deployment-targets"
        type="button"
        role="tab"
        :aria-selected="viewMode === 'targets'"
        :tabindex="viewMode === 'targets' ? 0 : -1"
        @click="setViewMode('targets')"
      >
        {{ t('control.tabs.assignments') }}
      </button>
    </nav>

    <p v-if="error" class="error-message" role="alert">{{ error }}</p>
    <p v-if="notice" class="notice-message" role="status">{{ notice }}</p>

    <section
      v-show="viewMode === 'topologies'"
      class="topologies-panel"
      data-testid="deployment-topology-panel"
      role="tabpanel"
      :aria-labelledby="'deployment-topologies'"
    >
      <div class="panel-toolbar">
        <div>
          <h2>{{ t('control.tabs.topologies') }}</h2>
          <p>{{ t('control.topology.select') }}</p>
        </div>
        <button
          class="btn btn-primary"
          data-testid="new-topology"
          type="button"
          :disabled="scopes.length === 0"
          @click="openNewTopology"
        >
          {{ t('control.topology.new') }}
        </button>
      </div>

      <div class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <th>{{ t('control.table.topology') }}</th>
              <th>{{ t('control.table.scope') }}</th>
              <th>{{ t('control.table.activeRevision') }}</th>
              <th>{{ t('control.table.deployment') }}</th>
              <th>{{ t('control.table.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="loading && !loaded"><td colspan="5" class="state-row">{{ t('control.states.loading') }}</td></tr>
            <tr v-for="topology in topologies" v-else :key="topology.id">
              <td>
                <strong>{{ topology.name || `#${topology.id}` }}</strong>
                <code>#{{ topology.id }}</code>
              </td>
              <td><code>{{ topology.service_scope || '-' }}</code></td>
              <td><code>{{ topology.active_revision_id || '-' }}</code></td>
              <td>
                <span v-if="latestDeploymentFor(topology)" :class="['state-badge', stateClass(latestDeploymentFor(topology).state)]">
                  {{ latestDeploymentFor(topology).state }}
                </span>
                <span v-else class="muted">{{ t('control.topology.noDeployment') }}</span>
              </td>
              <td>
                <div class="row-actions">
                  <button class="btn" :data-testid="`edit-topology-${topology.id}`" type="button" @click="openTopologyEditor(topology)">
                    {{ t('control.topology.edit') }}
                  </button>
                  <button
                    v-if="latestDeploymentFor(topology)"
                    class="btn"
                    :data-testid="`view-deployment-${latestDeploymentFor(topology).id}`"
                    type="button"
                    @click="openDeploymentStatus(latestDeploymentFor(topology))"
                  >
                    {{ t('control.topology.status') }}
                  </button>
                </div>
              </td>
            </tr>
            <tr v-if="loaded && !loading && topologies.length === 0 && !error"><td colspan="5" class="state-row">{{ t('control.empty.topologies') }}</td></tr>
          </tbody>
        </table>
      </div>
    </section>

    <section
      v-show="viewMode === 'targets'"
      class="targets-panel"
      data-testid="deployment-target-panel"
      role="tabpanel"
      :aria-labelledby="'deployment-targets'"
    >
      <div class="panel-toolbar">
        <label class="node-picker" for="assignment-node-filter">
          <span>{{ t('control.assignments.node') }}</span>
          <select
            id="assignment-node-filter"
            v-model.number="selectedNodeID"
            :disabled="nodes.length === 0 || assignmentsLoading"
            @change="loadAssignments(selectedNodeID)"
          >
            <option v-if="nodes.length === 0" :value="0">{{ t('control.assignments.noNodes') }}</option>
            <option v-for="node in nodes" :key="node.id" :value="node.id">{{ node.name || node.host || `#${node.id}` }} (#{{ node.id }})</option>
          </select>
        </label>
        <div class="row-actions">
          <button class="btn" type="button" :disabled="!selectedNodeID || assignmentsLoading" @click="loadAssignments(selectedNodeID)">
            {{ assignmentsLoading ? t('control.actions.refreshing') : t('control.actions.refresh') }}
          </button>
          <button class="btn btn-primary" data-testid="new-assignment" type="button" :disabled="!selectedNodeID" @click="openAssignmentDrawer()">
            {{ t('control.actions.newAssignment') }}
          </button>
        </div>
      </div>

      <div class="table-wrap">
        <table class="data-table assignments-table">
          <thead>
            <tr>
              <th>{{ t('control.table.plugin') }}</th>
              <th>{{ t('control.table.scope') }}</th>
              <th>{{ t('control.table.role') }}</th>
              <th>{{ t('control.table.version') }}</th>
              <th>{{ t('control.table.configRevision') }}</th>
              <th>{{ t('control.table.rolloutGroup') }}</th>
              <th>{{ t('control.table.state') }}</th>
              <th>{{ t('control.table.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="assignmentsLoading"><td colspan="8" class="state-row">{{ t('control.assignments.loading') }}</td></tr>
            <tr v-for="assignment in assignments" v-else :key="assignment.id">
              <td><strong>{{ pluginName(assignment.plugin_id) }}</strong><code>{{ assignment.plugin_id }}</code></td>
              <td><code>{{ assignment.service_scope }}</code></td>
              <td><code>{{ assignment.role }}</code></td>
              <td><code>{{ assignment.desired_version || '-' }}</code></td>
              <td>{{ assignment.desired_config_revision ?? 0 }}</td>
              <td>{{ assignment.rollout_group || '-' }}</td>
              <td><span :class="['state-badge', assignment.enabled ? 'state-active' : 'state-error']">{{ assignment.enabled ? t('control.states.enabled') : t('control.states.disabled') }}</span></td>
              <td>
                <div class="row-actions">
                  <button class="btn" :data-testid="`edit-assignment-${assignment.id}`" type="button" :disabled="isAssignmentBusy(assignment)" @click="openAssignmentDrawer(assignment)">
                    {{ t('common.actions.edit') }}
                  </button>
                  <button
                    :class="['btn', assignment.enabled ? 'btn-danger' : 'btn-primary']"
                    :data-testid="`toggle-assignment-${assignment.id}`"
                    type="button"
                    :disabled="isAssignmentBusy(assignment)"
                    @click="toggleAssignment(assignment)"
                  >
                    {{ assignment.enabled ? t('control.actions.disable') : t('control.actions.enable') }}
                  </button>
                  <button class="btn btn-danger" :data-testid="`delete-assignment-${assignment.id}`" type="button" :disabled="isAssignmentBusy(assignment)" @click="removeAssignment(assignment)">
                    {{ t('common.actions.delete') }}
                  </button>
                </div>
              </td>
            </tr>
            <tr v-if="!assignmentsLoading && selectedNodeID && assignments.length === 0"><td colspan="8" class="state-row">{{ t('control.empty.assignments') }}</td></tr>
            <tr v-if="!assignmentsLoading && !selectedNodeID"><td colspan="8" class="state-row">{{ t('control.assignments.noNodes') }}</td></tr>
          </tbody>
        </table>
      </div>
    </section>

    <section class="activity-panel" data-testid="deployment-activity">
      <OperationTimeline
        :operations="operations"
        :scoped-operation-i-ds="scopedOperationIDs"
        :busy-operation-i-d="operationBusy"
        @cancel="cancelOperation"
      />
    </section>

    <TopologyWorkspace
      :open="topologyEditor.open"
      :topology="topologyEditor.topology"
      :scopes="scopes"
      :revisions="topologyEditor.revisions"
      :revision-i-d="topologyEditor.revisionID"
      :revision-detail="topologyEditor.revisionDetail"
      :deployment-i-d="topologyEditor.deploymentID"
      :deployment-status="topologyEditor.deploymentStatus"
      :validation="topologyEditor.validation"
      :preview="topologyEditor.preview"
      :loading="topologyEditor.loading"
      :saving="topologyEditor.saving"
      :validating="topologyEditor.validating"
      :error="topologyEditor.error"
      @close="closeTopologyEditor"
      @create-topology="createTopology"
      @select-revision="selectTopologyRevision"
      @diagnose="diagnoseTopology"
      @save-revision="saveTopologyRevision"
      @preview="previewTopology"
      @plan="planTopology"
      @apply="applyDeployment"
      @rollback="rollbackDeployment"
    />

    <AssignmentDrawer
      :open="assignmentEditor.open"
      :assignment="assignmentEditor.assignment"
      :selected-node-i-d="selectedNodeID"
      :nodes="nodes"
      :plugins="assignmentResources.plugins"
      :releases="assignmentResources.releases"
      :installations="assignmentResources.installations"
      :scopes="scopes"
      :saving="assignmentEditor.saving"
      :error="assignmentEditor.error"
      @close="closeAssignmentDrawer"
      @save="saveAssignment"
    />
  </section>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { RefreshCw } from '@lucide/vue'
import AssignmentDrawer from '@/components/admin/AssignmentDrawer.vue'
import OperationTimeline from '@/components/admin/OperationTimeline.vue'
import TopologyWorkspace from '@/components/admin/TopologyWorkspace.vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { extractNodes } from '@/composables/useKernelDeployments'
import { getNodes } from '@/api/admin'
import {
  applyKernelDeployment,
  cancelKernelOperation,
  createKernelTopology,
  createKernelTopologyRevision,
  deleteKernelNodeAssignment,
  diagnoseKernelTopologyDeployment,
  getKernelDeploymentStatus,
  getKernelDeployments,
  getKernelInstallations,
  getKernelNodeAssignments,
  getKernelOperations,
  getKernelPluginReleases,
  getKernelPlugins,
  getKernelScopes,
  getKernelTopologies,
  getKernelTopologyRevision,
  getKernelTopologyRevisions,
  planKernelDeployment,
  previewKernelTopologyDeployment,
  rollbackKernelDeployment,
  upsertKernelNodeAssignment,
  validateKernelTopology,
} from '@/api/kernel'

const TERMINAL_OPERATION_STATES = new Set(['succeeded', 'completed', 'failed', 'superseded', 'cancelled', 'timed_out', 'expired', 'rolled_back'])
const ACTIVE_DEPLOYMENT_STATES = new Set(['applying', 'rollback_requested'])
const POLL_INTERVAL_MS = 2000

const { t } = useAppI18n()
const viewMode = ref('topologies')
const loading = ref(false)
const loaded = ref(false)
const error = ref('')
const notice = ref('')
const topologies = ref([])
const deployments = ref([])
const scopes = ref([])
const nodes = ref([])
const operations = ref([])
const selectedNodeID = ref(0)
const assignments = ref([])
const assignmentsLoading = ref(false)
const assignmentBusy = ref({})
const operationBusy = ref('')
const trackedOperationIDs = ref(new Set())
const topologyEditor = reactive({
  open: false,
  topology: null,
  revisions: [],
  revisionID: 0,
  revisionDetail: null,
  deploymentID: 0,
  deploymentStatus: null,
  validation: null,
  preview: null,
  loading: false,
  saving: false,
  validating: false,
  error: '',
})
const assignmentEditor = reactive({ open: false, assignment: null, saving: false, error: '' })
const assignmentResources = reactive({
  loaded: false,
  loading: false,
  plugins: [],
  releases: [],
  installations: [],
})

let pollTimer = null
let pollRequestRunning = false
let disposed = false
let topologyRequestID = 0

const scopedOperationIDs = computed(() => {
  const ids = new Set(trackedOperationIDs.value)
  const deploymentID = Number(topologyEditor.deploymentID)
  const nodeID = Number(selectedNodeID.value)
  for (const operation of operations.value) {
    if (deploymentID && operationDeploymentID(operation) === deploymentID) ids.add(String(operation.id))
    if (nodeID && operationNodeID(operation) === nodeID) ids.add(String(operation.id))
  }
  return [...ids]
})

function errorMessage(cause, fallbackKey) {
  const response = cause?.response?.data
  return response?.error?.message || (typeof response?.error === 'string' ? response.error : '') || response?.message || response?.msg || cause?.message || t(fallbackKey)
}

function rows(value) {
  return Array.isArray(value) ? value : []
}

function pluginName(pluginID) {
  return assignmentResources.plugins.find(plugin => plugin?.id === pluginID)?.name || pluginID
}

function latestDeploymentFor(topology) {
  return deployments.value
    .filter(deployment => Number(deployment?.topology_id) === Number(topology?.id))
    .sort((left, right) => Number(right?.id || 0) - Number(left?.id || 0))[0] || null
}

function stateClass(state) {
  if (state === 'healthy' || state === 'enabled' || state === 'succeeded' || state === 'completed') return 'state-active'
  if (state === 'failed' || state === 'superseded' || state === 'disabled' || state === 'cancel_requested' || state === 'cancelled' || state === 'timed_out') return 'state-error'
  return 'state-pending'
}

function operationDeploymentID(operation) {
  return Number(operation?.topology_deployment_id || operation?.deployment_id || operation?.deployment?.id || operation?.subject_id || 0)
}

function operationNodeID(operation) {
  return Number(operation?.node_id || operation?.node?.id || operation?.payload?.node_id || 0)
}

function deploymentState() {
  return topologyEditor.deploymentStatus?.deployment?.state || topologyEditor.deploymentStatus?.state || ''
}

function addTrackedOperation(result) {
  const operationID = result?.operation?.id || result?.data?.operation?.id
  if (!operationID) return
  trackedOperationIDs.value = new Set([...trackedOperationIDs.value, String(operationID)])
}

function reconcileTrackedOperations() {
  const activeIDs = [...trackedOperationIDs.value].filter(operationID => {
    const operation = operations.value.find(item => String(item?.id) === String(operationID))
    return operation && !TERMINAL_OPERATION_STATES.has(operation.state)
  })
  trackedOperationIDs.value = new Set(activeIDs)
}

function trackNodeOperations(nodeID) {
  const matchingOperationIDs = operations.value
    .filter(operation => operationNodeID(operation) === Number(nodeID) && !TERMINAL_OPERATION_STATES.has(operation.state))
    .map(operation => String(operation.id))
  if (!matchingOperationIDs.length) return
  trackedOperationIDs.value = new Set([...trackedOperationIDs.value, ...matchingOperationIDs])
}

function updateDeployment(deployment) {
  if (!deployment?.id) return
  const deploymentID = Number(deployment.id)
  deployments.value = [deployment, ...deployments.value.filter(item => Number(item?.id) !== deploymentID)]
}

async function loadInitial({ silent = false } = {}) {
  if (!silent) {
    loading.value = true
    error.value = ''
    notice.value = ''
  }
  try {
    const [topologyRows, deploymentRows, operationRows, scopeRows, nodeResponse] = await Promise.all([
      getKernelTopologies(),
      getKernelDeployments(),
      getKernelOperations(),
      getKernelScopes(),
      getNodes({ page: 1, page_size: 200 }),
    ])
    topologies.value = rows(topologyRows)
    deployments.value = rows(deploymentRows)
    operations.value = rows(operationRows)
    scopes.value = rows(scopeRows)
    nodes.value = extractNodes(nodeResponse, t('control.errors.nodesLoad'))
    if (!selectedNodeID.value || !nodes.value.some(node => Number(node.id) === Number(selectedNodeID.value))) {
      selectedNodeID.value = Number(nodes.value[0]?.id || 0)
    }
    loaded.value = true
    reconcileTrackedOperations()
    updatePolling()
    return true
  } catch (cause) {
    error.value = errorMessage(cause, 'control.errors.load')
    if (!loaded.value) {
      topologies.value = []
      deployments.value = []
      operations.value = []
      scopes.value = []
      nodes.value = []
    }
    return false
  } finally {
    if (!silent) loading.value = false
  }
}

async function refreshTopologies() {
  const topologyRows = await getKernelTopologies()
  topologies.value = rows(topologyRows)
}

async function refreshOperationState() {
  try {
    operations.value = rows(await getKernelOperations())
    reconcileTrackedOperations()
    updatePolling()
    return true
  } catch (cause) {
    error.value = errorMessage(cause, 'control.errors.poll')
    return false
  }
}

async function setViewMode(mode) {
  viewMode.value = mode
  if (mode === 'targets') await loadAssignments(selectedNodeID.value)
}

async function loadAssignments(nodeID = selectedNodeID.value) {
  const requestedNodeID = Number(nodeID)
  if (requestedNodeID <= 0) {
    assignments.value = []
    return false
  }
  assignmentsLoading.value = true
  error.value = ''
  try {
    assignments.value = rows(await getKernelNodeAssignments(requestedNodeID))
    return true
  } catch (cause) {
    assignments.value = []
    error.value = errorMessage(cause, 'control.errors.assignmentsLoad')
    return false
  } finally {
    assignmentsLoading.value = false
  }
}

async function loadAssignmentResources() {
  if (assignmentResources.loaded) return true
  assignmentResources.loading = true
  try {
    const [pluginRows, releaseRows, installationRows] = await Promise.all([
      getKernelPlugins(),
      getKernelPluginReleases(),
      getKernelInstallations(),
    ])
    assignmentResources.plugins = rows(pluginRows)
    assignmentResources.releases = rows(releaseRows)
    assignmentResources.installations = rows(installationRows)
    assignmentResources.loaded = true
    return true
  } catch (cause) {
    error.value = errorMessage(cause, 'control.errors.load')
    return false
  } finally {
    assignmentResources.loading = false
  }
}

async function openAssignmentDrawer(assignment = null) {
  if (!assignment && Number(selectedNodeID.value) <= 0) return
  error.value = ''
  if (!await loadAssignmentResources()) return
  Object.assign(assignmentEditor, { open: true, assignment, saving: false, error: '' })
}

function closeAssignmentDrawer() {
  if (!assignmentEditor.saving) assignmentEditor.open = false
}

async function refreshSelectedAssignmentActivity(nodeID = selectedNodeID.value) {
  const [assignmentResult, operationResult] = await Promise.all([
    loadAssignments(nodeID),
    refreshOperationState(),
  ])
  if (operationResult) trackNodeOperations(nodeID)
  updatePolling()
  return assignmentResult && operationResult
}

async function saveAssignment({ nodeID, payload }) {
  const targetNodeID = Number(nodeID)
  if (targetNodeID <= 0) return
  assignmentEditor.saving = true
  assignmentEditor.error = ''
  error.value = ''
  notice.value = ''
  try {
    const result = await upsertKernelNodeAssignment(targetNodeID, payload)
    addTrackedOperation(result)
    selectedNodeID.value = targetNodeID
    if (!await refreshSelectedAssignmentActivity(targetNodeID)) throw new Error(error.value || t('control.errors.assignmentsLoad'))
    assignmentEditor.open = false
    notice.value = t('control.messages.assignmentSaved', { plugin: pluginName(payload.plugin_id) })
  } catch (cause) {
    assignmentEditor.error = errorMessage(cause, 'control.errors.assignmentSave')
  } finally {
    assignmentEditor.saving = false
  }
}

function isAssignmentBusy(assignment) {
  return Boolean(assignmentBusy.value[assignment.id])
}

function setAssignmentBusy(assignment, action = '') {
  assignmentBusy.value = { ...assignmentBusy.value, [assignment.id]: action }
  if (!action) delete assignmentBusy.value[assignment.id]
}

async function toggleAssignment(assignment) {
  setAssignmentBusy(assignment, 'toggle')
  error.value = ''
  notice.value = ''
  try {
    const result = await upsertKernelNodeAssignment(Number(selectedNodeID.value), {
      service_scope: assignment.service_scope,
      plugin_id: assignment.plugin_id,
      role: assignment.role,
      desired_version: assignment.desired_version || '',
      desired_config_revision: Math.max(0, Number(assignment.desired_config_revision ?? 0)),
      enabled: !assignment.enabled,
      rollout_group: assignment.rollout_group || '',
    })
    addTrackedOperation(result)
    await refreshSelectedAssignmentActivity(Number(selectedNodeID.value))
    notice.value = t('control.messages.assignmentStateSaved', { plugin: pluginName(assignment.plugin_id) })
  } catch (cause) {
    error.value = errorMessage(cause, 'control.errors.assignmentSave')
  } finally {
    setAssignmentBusy(assignment)
  }
}

async function removeAssignment(assignment) {
  setAssignmentBusy(assignment, 'delete')
  error.value = ''
  notice.value = ''
  try {
    await deleteKernelNodeAssignment(Number(selectedNodeID.value), assignment.id)
    await refreshSelectedAssignmentActivity(Number(selectedNodeID.value))
    notice.value = t('control.messages.assignmentDeleted', { plugin: pluginName(assignment.plugin_id) })
  } catch (cause) {
    error.value = errorMessage(cause, 'control.errors.assignmentDelete')
  } finally {
    setAssignmentBusy(assignment)
  }
}

function resetTopologyEditor() {
  Object.assign(topologyEditor, {
    topology: null,
    revisions: [],
    revisionID: 0,
    revisionDetail: null,
    deploymentID: 0,
    deploymentStatus: null,
    validation: null,
    preview: null,
    loading: false,
    saving: false,
    validating: false,
    error: '',
  })
}

function openNewTopology() {
  resetTopologyEditor()
  topologyEditor.open = true
}

function closeTopologyEditor() {
  if (!topologyEditor.saving && !topologyEditor.validating) topologyEditor.open = false
}

async function createTopology(input) {
  topologyEditor.saving = true
  topologyEditor.error = ''
  try {
    const topology = await createKernelTopology(input)
    topologyEditor.topology = topology
    topologyEditor.revisions = []
    topologyEditor.revisionID = 0
    topologyEditor.revisionDetail = null
    topologyEditor.validation = null
    topologyEditor.preview = null
    topologies.value = [topology, ...topologies.value.filter(item => Number(item?.id) !== Number(topology?.id))]
    notice.value = t('control.messages.topologyCreated', { name: topology?.name || input.name })
    try {
      await refreshTopologies()
    } catch (cause) {
      topologyEditor.error = errorMessage(cause, 'control.errors.topologyLoad')
    }
  } catch (cause) {
    topologyEditor.error = errorMessage(cause, 'control.errors.topologyCreate')
  } finally {
    topologyEditor.saving = false
  }
}

async function openTopologyEditor(topology) {
  const nextTopology = topology || topologies.value.find(item => Number(item.id) === Number(topologyEditor.topology?.id))
  if (!nextTopology?.id) return
  const requestID = ++topologyRequestID
  resetTopologyEditor()
  Object.assign(topologyEditor, { open: true, topology: nextTopology, loading: true })
  try {
    const revisions = rows(await getKernelTopologyRevisions(nextTopology.id))
    if (requestID !== topologyRequestID || !topologyEditor.open) return
    topologyEditor.revisions = revisions
    const revisionID = Number(nextTopology.active_revision_id || revisions[0]?.id || 0)
    topologyEditor.revisionID = revisionID
    if (revisionID) await loadTopologyRevisionDetail(nextTopology.id, revisionID, requestID)
  } catch (cause) {
    if (requestID === topologyRequestID) topologyEditor.error = errorMessage(cause, 'control.errors.topologyLoad')
  } finally {
    if (requestID === topologyRequestID) topologyEditor.loading = false
  }
}

async function loadTopologyRevisionDetail(topologyID, revisionID, requestID = ++topologyRequestID) {
  const detail = await getKernelTopologyRevision(topologyID, revisionID)
  if (requestID !== topologyRequestID || !topologyEditor.open || Number(topologyEditor.topology?.id) !== Number(topologyID)) return false
  topologyEditor.revisionID = Number(revisionID)
  topologyEditor.revisionDetail = detail
  return true
}

async function selectTopologyRevision({ topologyID, revisionID }) {
  const requestID = ++topologyRequestID
  topologyEditor.loading = true
  topologyEditor.error = ''
  try {
    await loadTopologyRevisionDetail(topologyID, revisionID, requestID)
    topologyEditor.validation = null
    topologyEditor.preview = null
    topologyEditor.deploymentID = 0
    topologyEditor.deploymentStatus = null
  } catch (cause) {
    if (requestID === topologyRequestID) topologyEditor.error = errorMessage(cause, 'control.errors.topologyLoad')
  } finally {
    if (requestID === topologyRequestID) topologyEditor.loading = false
  }
}

async function diagnoseTopology({ topologyID, revisionID, input, options }) {
  topologyEditor.validating = true
  topologyEditor.error = ''
  try {
    const localValidation = await validateKernelTopology(input)
    topologyEditor.validation = localValidation
    if (!localValidation?.valid) return false
    const diagnosis = await diagnoseKernelTopologyDeployment(topologyID, revisionID, options)
    topologyEditor.validation = diagnosis
    return Boolean(diagnosis?.valid)
  } catch (cause) {
    topologyEditor.error = errorMessage(cause, 'control.errors.topologyValidate')
    return false
  } finally {
    topologyEditor.validating = false
  }
}

async function saveTopologyRevision({ topologyID, input }) {
  topologyEditor.saving = true
  topologyEditor.error = ''
  try {
    const validation = await validateKernelTopology(input)
    topologyEditor.validation = validation
    if (!validation?.valid) throw new Error(t('control.topology.invalid'))
    const revision = await createKernelTopologyRevision(topologyID, input)
    topologyEditor.revisions = rows(await getKernelTopologyRevisions(topologyID))
    if (!topologyEditor.revisions.some(item => Number(item.id) === Number(revision?.id))) topologyEditor.revisions = [revision, ...topologyEditor.revisions]
    const revisionID = Number(revision?.id || topologyEditor.revisions[0]?.id || 0)
    topologyEditor.revisionID = revisionID
    const requestID = ++topologyRequestID
    if (revisionID) await loadTopologyRevisionDetail(topologyID, revisionID, requestID)
    topologyEditor.preview = null
    topologyEditor.deploymentID = 0
    topologyEditor.deploymentStatus = null
    await refreshTopologies()
    notice.value = t('control.messages.topologyRevisionSaved', { revision: revision?.revision || '-' })
    return true
  } catch (cause) {
    topologyEditor.error = errorMessage(cause, 'control.errors.topologySave')
    return false
  } finally {
    topologyEditor.saving = false
  }
}

async function previewTopology({ topologyID, revisionID, options }) {
  topologyEditor.validating = true
  topologyEditor.error = ''
  try {
    const preview = await previewKernelTopologyDeployment(topologyID, revisionID, options)
    topologyEditor.preview = preview
    topologyEditor.validation = preview
    return Boolean(preview?.valid)
  } catch (cause) {
    topologyEditor.error = errorMessage(cause, 'control.errors.topologyPreview')
    return false
  } finally {
    topologyEditor.validating = false
  }
}

async function planTopology({ topologyID, revisionID, options }) {
  topologyEditor.saving = true
  topologyEditor.error = ''
  notice.value = ''
  try {
    const diagnosis = await diagnoseKernelTopologyDeployment(topologyID, revisionID, options)
    topologyEditor.validation = diagnosis
    if (!diagnosis?.valid) throw new Error(t('control.topology.invalid'))
    const preview = await previewKernelTopologyDeployment(topologyID, revisionID, options)
    topologyEditor.preview = preview
    topologyEditor.validation = preview
    if (!preview?.valid) throw new Error(t('control.topology.invalid'))
    const result = await planKernelDeployment({
      topology_id: Number(topologyID),
      revision_id: Number(revisionID),
      rollout_group: options.rolloutGroup?.trim() || '',
      failure_policy: options.failurePolicy || 'stop_and_rollback',
    })
    const deployment = result?.deployment || result
    const deploymentID = Number(deployment?.id || 0)
    if (!deploymentID) throw new Error(t('control.errors.topologyPlan'))
    updateDeployment(deployment)
    topologyEditor.deploymentID = deploymentID
    await refreshDeploymentStatus(deploymentID)
    await refreshOperationState()
    notice.value = t('control.messages.topologyPlanned', { id: deploymentID })
    return true
  } catch (cause) {
    topologyEditor.error = errorMessage(cause, 'control.errors.topologyPlan')
    return false
  } finally {
    topologyEditor.saving = false
  }
}

async function refreshDeploymentStatus(deploymentID = topologyEditor.deploymentID) {
  const selectedDeploymentID = Number(deploymentID)
  if (!selectedDeploymentID) return null
  const status = await getKernelDeploymentStatus(selectedDeploymentID)
  topologyEditor.deploymentID = selectedDeploymentID
  topologyEditor.deploymentStatus = status
  updateDeployment(status?.deployment || status)
  updatePolling()
  return status
}

async function openDeploymentStatus(deployment) {
  const topology = topologies.value.find(item => Number(item.id) === Number(deployment?.topology_id))
  if (!topology) return
  await openTopologyEditor(topology)
  if (!deployment?.id) return
  try {
    await refreshDeploymentStatus(deployment.id)
  } catch (cause) {
    topologyEditor.error = errorMessage(cause, 'control.errors.topologyStatus')
  }
}

async function applyDeployment({ deploymentID }) {
  topologyEditor.saving = true
  topologyEditor.error = ''
  notice.value = ''
  try {
    await applyKernelDeployment(deploymentID)
    await refreshDeploymentStatus(deploymentID)
    await refreshTopologies()
    await refreshOperationState()
    notice.value = t('control.messages.topologyApplyRequested')
  } catch (cause) {
    topologyEditor.error = errorMessage(cause, 'control.errors.topologyApply')
  } finally {
    topologyEditor.saving = false
  }
}

async function rollbackDeployment({ deploymentID }) {
  topologyEditor.saving = true
  topologyEditor.error = ''
  notice.value = ''
  try {
    await rollbackKernelDeployment(deploymentID)
    await refreshDeploymentStatus(deploymentID)
    await refreshTopologies()
    await refreshOperationState()
    notice.value = t('control.messages.topologyRollbackRequested')
  } catch (cause) {
    topologyEditor.error = errorMessage(cause, 'control.errors.topologyRollback')
  } finally {
    topologyEditor.saving = false
  }
}

async function cancelOperation(operationID) {
  operationBusy.value = operationID
  error.value = ''
  notice.value = ''
  try {
    const result = await cancelKernelOperation(operationID)
    addTrackedOperation(result?.operation?.id ? result : { operation: { id: operationID } })
    await refreshOperationState()
    notice.value = t('control.messages.cancelRequested')
  } catch (cause) {
    error.value = errorMessage(cause, 'control.errors.cancel')
  } finally {
    operationBusy.value = ''
  }
}

function selectedDeploymentIsActive() {
  const state = deploymentState()
  return Boolean(topologyEditor.open && topologyEditor.deploymentID && ACTIVE_DEPLOYMENT_STATES.has(state))
}

function hasActiveScopedWork() {
  if (selectedDeploymentIsActive()) return true
  return [...trackedOperationIDs.value].some(operationID => {
    const operation = operations.value.find(item => String(item?.id) === String(operationID))
    return operation && !TERMINAL_OPERATION_STATES.has(operation.state)
  })
}

function updatePolling() {
  if (disposed) {
    if (pollTimer) clearInterval(pollTimer)
    pollTimer = null
    return
  }
  const shouldPoll = hasActiveScopedWork()
  if (shouldPoll && !pollTimer) {
    pollTimer = setInterval(pollActivity, POLL_INTERVAL_MS)
  } else if (!shouldPoll && pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

async function pollActivity() {
  if (pollRequestRunning || disposed) return
  pollRequestRunning = true
  try {
    const operationRequest = getKernelOperations()
    const deploymentRequest = selectedDeploymentIsActive()
      ? getKernelDeploymentStatus(topologyEditor.deploymentID)
      : null
    const [operationRows, status] = await Promise.all([operationRequest, deploymentRequest])
    operations.value = rows(operationRows)
    if (status) {
      topologyEditor.deploymentStatus = status
      updateDeployment(status?.deployment || status)
    }
    reconcileTrackedOperations()
  } catch (cause) {
    error.value = errorMessage(cause, 'control.errors.poll')
  } finally {
    pollRequestRunning = false
  }
  updatePolling()
}

onMounted(() => {
  disposed = false
  void loadInitial()
})

onBeforeUnmount(() => {
  disposed = true
  if (pollTimer) clearInterval(pollTimer)
  pollTimer = null
})
</script>

<style scoped>
.deployment-center { display: grid; gap: 16px; min-width: 0; }
.page-header, .panel-toolbar, .row-actions { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.page-header h1, .panel-toolbar h2 { margin: 0; }
.page-header h1 { font-size: 24px; line-height: 1.25; }
.panel-toolbar h2 { font-size: 16px; }
.page-subtitle, .panel-toolbar p, .muted, td code { color: var(--text-secondary); }
.page-subtitle, .panel-toolbar p { margin: 5px 0 0; font-size: 13px; }
.icon-button, .btn { min-height: 36px; border: 1px solid var(--border-color); border-radius: 6px; background: var(--surface-color); color: var(--text-color); cursor: pointer; }
.icon-button { display: inline-grid; width: 36px; place-items: center; padding: 0; }
.btn { padding: 8px 12px; }
.btn-primary { border-color: var(--primary-color); background: var(--primary-color); color: #fff; }
.btn-danger { border-color: var(--error-color); color: var(--error-color); }
.icon-button:disabled, .btn:disabled, select:disabled { cursor: not-allowed; opacity: .55; }
.spinning { animation: deployment-spin .8s linear infinite; }
@keyframes deployment-spin { to { transform: rotate(360deg); } }
.view-switcher { display: flex; gap: 4px; border-bottom: 1px solid var(--border-color); overflow-x: auto; }
.view-tab { min-height: 40px; border: 0; border-bottom: 2px solid transparent; border-radius: 0; background: transparent; padding: 9px 12px; color: var(--text-secondary); cursor: pointer; white-space: nowrap; }
.view-tab[aria-selected="true"] { border-bottom-color: var(--primary-color); color: var(--text-color); font-weight: 600; }
.topologies-panel, .targets-panel, .activity-panel { display: grid; gap: 12px; min-width: 0; }
.panel-toolbar { align-items: end; }
.node-picker { display: grid; min-width: min(100%, 300px); gap: 6px; color: var(--text-secondary); font-size: 12px; font-weight: 700; }
.node-picker select { min-height: 38px; max-width: 100%; border: 1px solid var(--border-color); border-radius: 6px; background: var(--surface-color); color: var(--text-color); padding: 8px 10px; }
.table-wrap { min-width: 0; overflow-x: auto; border: 1px solid var(--border-color); border-radius: 6px; }
.data-table { width: 100%; min-width: 720px; border-collapse: collapse; }
.assignments-table { min-width: 1000px; }
.data-table th, .data-table td { padding: 10px 12px; border-bottom: 1px solid var(--border-color); text-align: left; vertical-align: top; }
.data-table th { color: var(--text-secondary); font-size: 11px; font-weight: 700; text-transform: uppercase; }
.data-table tbody tr:last-child td { border-bottom: 0; }
.data-table td strong, .data-table td code { display: block; overflow-wrap: anywhere; }
.state-row { color: var(--text-secondary); text-align: center !important; }
.state-badge { display: inline-flex; border-radius: 6px; padding: 3px 7px; font-size: 12px; }
.state-active { color: var(--success-color); background: rgba(22, 163, 74, .1); }
.state-error { color: var(--error-color); background: rgba(220, 38, 38, .1); }
.state-pending { color: var(--warning-color); background: rgba(217, 119, 6, .1); }
.error-message, .notice-message { margin: 0; overflow-wrap: anywhere; }
.error-message { color: var(--error-color); }
.notice-message { color: var(--success-color); }
.sr-only { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip: rect(0, 0, 0, 0); white-space: nowrap; border: 0; }
@media (max-width: 720px) {
  .deployment-center { gap: 12px; }
  .page-header h1 { font-size: 20px; }
  .panel-toolbar > div:first-child { width: 100%; }
  .panel-toolbar .row-actions { width: 100%; }
  .panel-toolbar .row-actions .btn { flex: 1; }
}
</style>
