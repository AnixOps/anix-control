// 部署编排 (UI U8): the page's state and requests, moved out of
// Deployments.vue unchanged (the same kernel calls, payloads, request
// generations and 2 s polling while scoped work is active). The page and its
// panels only render what this returns.
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { useConfirm } from '@/ui/composables/useConfirm'
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

export const VIEW_MODES = ['topologies', 'targets']

export function deploymentStateTone(state) {
  if (state === 'healthy' || state === 'enabled' || state === 'succeeded' || state === 'completed') return 'success'
  if (state === 'failed' || state === 'superseded' || state === 'disabled' || state === 'cancel_requested' || state === 'cancelled' || state === 'timed_out') return 'danger'
  return 'warning'
}

export function useDeploymentCenter() {
  const TERMINAL_OPERATION_STATES = new Set(['succeeded', 'completed', 'failed', 'superseded', 'cancelled', 'timed_out', 'expired', 'rolled_back'])
  const ACTIVE_DEPLOYMENT_STATES = new Set(['applying', 'rollback_requested'])
  const POLL_INTERVAL_MS = 2000

  const { t } = useAppI18n()
  const confirm = useConfirm()
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
  const targetMutationPending = computed(() => assignmentEditor.saving || Object.values(assignmentBusy.value).some(Boolean))

  let pollTimer = null
  let pollRequestRunning = false
  let disposed = false
  let topologyRequestID = 0
  let topologySessionID = 0
  let assignmentLoadRequestID = 0
  let operationRequestGeneration = 0

  const scopedOperationIDs = computed(() => {
    const ids = new Set(trackedOperationIDs.value)
    for (const operation of operations.value) {
      if (isScopedOperation(operation)) ids.add(String(operation.id))
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
    if (!isCurrentTarget(nodeID)) return
    const matchingOperationIDs = operations.value
      .filter(operation => operationNodeID(operation) === Number(nodeID) && !TERMINAL_OPERATION_STATES.has(operation.state))
      .map(operation => String(operation.id))
    if (!matchingOperationIDs.length) return
    trackedOperationIDs.value = new Set([...trackedOperationIDs.value, ...matchingOperationIDs])
  }

  function isCurrentTarget(nodeID) {
    return viewMode.value === 'targets' && Number(nodeID) > 0 && Number(selectedNodeID.value) === Number(nodeID)
  }

  function beginTopologyEditorSession() {
    topologySessionID += 1
    return topologySessionID
  }

  function isCurrentTopologySession(sessionID, topologyID) {
    return sessionID === topologySessionID && topologyEditor.open && Number(topologyEditor.topology?.id) === Number(topologyID)
  }

  function isCurrentDeploymentStatusSession(sessionID, topologyID, deploymentID) {
    return isCurrentTopologySession(sessionID, topologyID) && Number(topologyEditor.deploymentID) === Number(deploymentID)
  }

  function isScopedOperation(operation) {
    const deploymentID = Number(topologyEditor.deploymentID)
    return Boolean(
      (topologyEditor.open && deploymentID && operationDeploymentID(operation) === deploymentID) ||
      isCurrentTarget(operationNodeID(operation)),
    )
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
    const operationRequestGenerationAtStart = ++operationRequestGeneration
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
      if (operationRequestGenerationAtStart === operationRequestGeneration) {
        operations.value = rows(operationRows)
        reconcileTrackedOperations()
      }
      scopes.value = rows(scopeRows)
      nodes.value = extractNodes(nodeResponse, t('control.errors.nodesLoad'))
      if (!selectedNodeID.value || !nodes.value.some(node => Number(node.id) === Number(selectedNodeID.value))) {
        selectedNodeID.value = Number(nodes.value[0]?.id || 0)
      }
      loaded.value = true
      if (viewMode.value === 'targets' && selectedNodeID.value > 0) await loadAssignments(selectedNodeID.value)
      updatePolling()
      return true
    } catch (cause) {
      error.value = errorMessage(cause, 'control.errors.load')
      if (!loaded.value) {
        topologies.value = []
        deployments.value = []
        if (operationRequestGenerationAtStart === operationRequestGeneration) operations.value = []
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
    const requestGeneration = ++operationRequestGeneration
    try {
      const operationRows = rows(await getKernelOperations())
      if (requestGeneration !== operationRequestGeneration) return true
      operations.value = operationRows
      reconcileTrackedOperations()
      updatePolling()
      return true
    } catch (cause) {
      if (requestGeneration !== operationRequestGeneration) return true
      error.value = errorMessage(cause, 'control.errors.poll')
      return false
    }
  }

  async function setViewMode(mode) {
    viewMode.value = mode
    if (mode === 'targets' && selectedNodeID.value > 0) await loadAssignments(selectedNodeID.value)
    updatePolling()
  }

  async function loadAssignments(nodeID = selectedNodeID.value) {
    const requestedNodeID = Number(nodeID)
    const requestID = ++assignmentLoadRequestID
    if (requestedNodeID <= 0) {
      if (requestID === assignmentLoadRequestID) assignments.value = []
      return false
    }
    assignmentsLoading.value = true
    error.value = ''
    try {
      const assignmentRows = rows(await getKernelNodeAssignments(requestedNodeID))
      if (requestID !== assignmentLoadRequestID || !isCurrentTarget(requestedNodeID)) return false
      assignments.value = assignmentRows
      return true
    } catch (cause) {
      if (requestID !== assignmentLoadRequestID || !isCurrentTarget(requestedNodeID)) return false
      assignments.value = []
      error.value = errorMessage(cause, 'control.errors.assignmentsLoad')
      return false
    } finally {
      if (requestID === assignmentLoadRequestID) assignmentsLoading.value = false
    }
  }

  async function selectAssignmentTarget(nodeID) {
    selectedNodeID.value = Number(nodeID)
    await loadAssignments(selectedNodeID.value)
    updatePolling()
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
    if (!isCurrentTarget(nodeID)) return true
    const [assignmentResult, operationResult] = await Promise.all([
      loadAssignments(nodeID),
      refreshOperationState(),
    ])
    if (!isCurrentTarget(nodeID)) return true
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
      selectedNodeID.value = targetNodeID
      addTrackedOperation(result)
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
    const targetNodeID = Number(assignment.node_id || selectedNodeID.value)
    setAssignmentBusy(assignment, 'toggle')
    error.value = ''
    notice.value = ''
    try {
      const result = await upsertKernelNodeAssignment(targetNodeID, {
        service_scope: assignment.service_scope,
        plugin_id: assignment.plugin_id,
        role: assignment.role,
        desired_version: assignment.desired_version || '',
        desired_config_revision: Math.max(0, Number(assignment.desired_config_revision ?? 0)),
        enabled: !assignment.enabled,
        rollout_group: assignment.rollout_group || '',
      })
      if (isCurrentTarget(targetNodeID)) addTrackedOperation(result)
      await refreshSelectedAssignmentActivity(targetNodeID)
      notice.value = t('control.messages.assignmentStateSaved', { plugin: pluginName(assignment.plugin_id) })
    } catch (cause) {
      error.value = errorMessage(cause, 'control.errors.assignmentSave')
    } finally {
      setAssignmentBusy(assignment)
    }
  }

  async function removeAssignment(assignment) {
    const confirmed = await confirm({
      title: t('control.assignments.deleteTitle', { plugin: pluginName(assignment.plugin_id), role: assignment.role }),
      message: t('control.assignments.deleteMessage'),
      confirmLabel: t('control.assignments.deleteAction'),
      tone: 'danger'
    })
    if (!confirmed) return
    const targetNodeID = Number(assignment.node_id || selectedNodeID.value)
    setAssignmentBusy(assignment, 'delete')
    error.value = ''
    notice.value = ''
    try {
      await deleteKernelNodeAssignment(targetNodeID, assignment.id)
      await refreshSelectedAssignmentActivity(targetNodeID)
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
    beginTopologyEditorSession()
    resetTopologyEditor()
    topologyEditor.open = true
  }

  function closeTopologyEditor() {
    if (!topologyEditor.saving && !topologyEditor.validating) {
      beginTopologyEditorSession()
      topologyEditor.open = false
      updatePolling()
    }
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
    if (!nextTopology?.id) return 0
    const requestID = ++topologyRequestID
    const sessionID = beginTopologyEditorSession()
    resetTopologyEditor()
    Object.assign(topologyEditor, { open: true, topology: nextTopology, loading: true })
    try {
      const revisions = rows(await getKernelTopologyRevisions(nextTopology.id))
      if (requestID !== topologyRequestID || !isCurrentTopologySession(sessionID, nextTopology.id)) return 0
      topologyEditor.revisions = revisions
      const revisionID = Number(nextTopology.active_revision_id || revisions[0]?.id || 0)
      topologyEditor.revisionID = revisionID
      if (revisionID) await loadTopologyRevisionDetail(nextTopology.id, revisionID, requestID)
      return isCurrentTopologySession(sessionID, nextTopology.id) ? sessionID : 0
    } catch (cause) {
      if (requestID === topologyRequestID && isCurrentTopologySession(sessionID, nextTopology.id)) topologyEditor.error = errorMessage(cause, 'control.errors.topologyLoad')
      return 0
    } finally {
      if (requestID === topologyRequestID && isCurrentTopologySession(sessionID, nextTopology.id)) topologyEditor.loading = false
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

  async function refreshDeploymentStatus(deploymentID = topologyEditor.deploymentID, {
    sessionID = topologySessionID,
    topologyID = topologyEditor.topology?.id,
  } = {}) {
    const selectedDeploymentID = Number(deploymentID)
    if (!selectedDeploymentID) return null
    try {
      const status = await getKernelDeploymentStatus(selectedDeploymentID)
      if (!isCurrentDeploymentStatusSession(sessionID, topologyID, selectedDeploymentID)) return null
      topologyEditor.deploymentStatus = status
      updateDeployment(status?.deployment || status)
      updatePolling()
      return status
    } catch (cause) {
      if (!isCurrentDeploymentStatusSession(sessionID, topologyID, selectedDeploymentID)) return null
      throw cause
    }
  }

  async function openDeploymentStatus(deployment) {
    const topology = topologies.value.find(item => Number(item.id) === Number(deployment?.topology_id))
    if (!topology) return
    const sessionID = await openTopologyEditor(topology)
    if (!deployment?.id || !isCurrentTopologySession(sessionID, topology.id)) return
    topologyEditor.deploymentID = Number(deployment.id)
    try {
      await refreshDeploymentStatus(deployment.id, { sessionID, topologyID: topology.id })
    } catch (cause) {
      if (isCurrentDeploymentStatusSession(sessionID, topology.id, deployment.id)) topologyEditor.error = errorMessage(cause, 'control.errors.topologyStatus')
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
    if (operations.value.some(operation => isScopedOperation(operation) && !TERMINAL_OPERATION_STATES.has(operation.state))) return true
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
    const operationRequestGenerationAtStart = ++operationRequestGeneration
    const sessionID = topologySessionID
    const topologyID = topologyEditor.topology?.id
    const deploymentID = Number(topologyEditor.deploymentID)
    const shouldRefreshDeployment = selectedDeploymentIsActive()
    try {
      const operationRequest = getKernelOperations()
      const deploymentRequest = shouldRefreshDeployment
        ? getKernelDeploymentStatus(deploymentID)
        : null
      const [operationRows, status] = await Promise.all([operationRequest, deploymentRequest])
      if (operationRequestGenerationAtStart === operationRequestGeneration) {
        operations.value = rows(operationRows)
        reconcileTrackedOperations()
      }
      if (status && isCurrentDeploymentStatusSession(sessionID, topologyID, deploymentID)) {
        topologyEditor.deploymentStatus = status
        updateDeployment(status?.deployment || status)
      }
    } catch (cause) {
      if (operationRequestGenerationAtStart === operationRequestGeneration) error.value = errorMessage(cause, 'control.errors.poll')
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

  return {
    t,
    viewMode,
    loading,
    loaded,
    error,
    notice,
    topologies,
    scopes,
    nodes,
    operations,
    selectedNodeID,
    assignments,
    assignmentsLoading,
    operationBusy,
    topologyEditor,
    assignmentEditor,
    assignmentResources,
    targetMutationPending,
    scopedOperationIDs,
    pluginName,
    latestDeploymentFor,
    loadInitial,
    setViewMode,
    loadAssignments,
    selectAssignmentTarget,
    openAssignmentDrawer,
    closeAssignmentDrawer,
    saveAssignment,
    isAssignmentBusy,
    toggleAssignment,
    removeAssignment,
    openNewTopology,
    closeTopologyEditor,
    createTopology,
    openTopologyEditor,
    selectTopologyRevision,
    diagnoseTopology,
    saveTopologyRevision,
    previewTopology,
    planTopology,
    openDeploymentStatus,
    applyDeployment,
    rollbackDeployment,
    cancelOperation,
  }
}
