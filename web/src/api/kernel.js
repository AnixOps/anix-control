import request from '@/utils/request'

const KERNEL_API_BASE_URL = '/api/v3'

function v3(config) {
  return request({ ...config, baseURL: KERNEL_API_BASE_URL })
}

function unwrap(response) {
  return response?.data ?? response
}

export async function getKernelPlugins() {
  return unwrap(await v3({ url: '/plugins', method: 'get' }))
}

export async function getKernelPluginReleases(pluginID) {
  if (pluginID) {
    return unwrap(await v3({ url: '/plugin-releases', method: 'get', params: { plugin_id: pluginID } }))
  }
  return unwrap(await v3({ url: '/plugin-releases', method: 'get' }))
}

export async function registerKernelPluginRelease(manifest, signature) {
  return unwrap(await v3({
    url: '/plugin-releases',
    method: 'post',
    data: { manifest, signature }
  }))
}

export async function getKernelPluginReleaseArtifact(releaseID) {
  return unwrap(await v3({ url: `/plugin-releases/${releaseID}/artifact`, method: 'get' }))
}

export async function uploadKernelPluginReleaseArtifact(releaseID, artifactBase64) {
  return unwrap(await v3({
    url: `/plugin-releases/${releaseID}/artifact`,
    method: 'post',
    data: { artifact_base64: artifactBase64 },
    timeout: 120_000
  }))
}

export async function getKernelInstallations() {
  return unwrap(await v3({ url: '/plugin-installations', method: 'get' }))
}

export async function getKernelNodeAssignments(nodeID) {
  return unwrap(await v3({ url: `/nodes/${nodeID}/assignments`, method: 'get' }))
}

export async function upsertKernelNodeAssignment(nodeID, assignment) {
  return unwrap(await v3({
    url: `/nodes/${nodeID}/assignments`,
    method: 'put',
    data: assignment
  }))
}

export async function deleteKernelNodeAssignment(nodeID, assignmentID) {
  return unwrap(await v3({
    url: `/nodes/${nodeID}/assignments/${assignmentID}`,
    method: 'delete'
  }))
}

export async function upsertKernelInstallation(installation) {
  return unwrap(await v3({
    url: '/plugin-installations',
    method: 'put',
    data: installation
  }))
}

export async function runKernelInstallationAction(installationID, action, options = {}) {
  const data = {
    action,
    idempotency_key: options.idempotencyKey
  }
  if (options.targetVersion) {
    data.target_version = options.targetVersion
  }
  return unwrap(await v3({
    url: `/plugin-installations/${installationID}/actions`,
    method: 'post',
    data
  }))
}

export async function getKernelInstallationConfig(installationID) {
  return unwrap(await v3({ url: `/plugin-installations/${installationID}/config`, method: 'get' }))
}

export async function updateKernelInstallationConfig(installationID, config, expectedRevision) {
  const data = { config }
  if (expectedRevision !== undefined && expectedRevision !== null) {
    data.expected_revision = expectedRevision
  }
  return unwrap(await v3({ url: `/plugin-installations/${installationID}/config`, method: 'put', data }))
}

export async function getKernelScopes() {
  return unwrap(await v3({ url: '/service-scopes', method: 'get' }))
}

export async function getKernelTopologies() {
  return unwrap(await v3({ url: '/topologies', method: 'get' }))
}

export async function createKernelTopology(input) {
  return unwrap(await v3({ url: '/topologies', method: 'post', data: input }))
}

export async function getKernelTopologyRevisions(topologyID) {
  return unwrap(await v3({ url: `/topologies/${topologyID}/revisions`, method: 'get' }))
}

export async function getKernelTopologyRevision(topologyID, revisionID) {
  return unwrap(await v3({ url: `/topologies/${topologyID}/revisions/${revisionID}`, method: 'get' }))
}

export async function validateKernelTopology(input) {
  return unwrap(await v3({ url: '/topologies/validate', method: 'post', data: input }))
}

export async function createKernelTopologyRevision(topologyID, input) {
  return unwrap(await v3({ url: `/topologies/${topologyID}/revisions`, method: 'post', data: input }))
}

export async function getKernelDeployments() {
  return unwrap(await v3({ url: '/deployments', method: 'get' }))
}

export async function planKernelDeployment(input) {
  return unwrap(await v3({ url: '/deployments', method: 'post', data: input }))
}

export async function getKernelDeploymentStatus(deploymentID) {
  return unwrap(await v3({ url: `/deployments/${deploymentID}`, method: 'get' }))
}

export async function applyKernelDeployment(deploymentID) {
  return unwrap(await v3({ url: `/deployments/${deploymentID}/apply`, method: 'post' }))
}

export async function rollbackKernelDeployment(deploymentID) {
  return unwrap(await v3({ url: `/deployments/${deploymentID}/rollback`, method: 'post' }))
}

export async function previewKernelTopologyDeployment(topologyID, revisionID, options = {}) {
  return unwrap(await v3({
    url: `/topologies/${topologyID}/revisions/${revisionID}/preview`,
    method: 'post',
    data: {
      rollout_group: options.rolloutGroup || '',
      failure_policy: options.failurePolicy || 'stop_and_rollback'
    }
  }))
}

export async function diagnoseKernelTopologyDeployment(topologyID, revisionID, options = {}) {
  return unwrap(await v3({
    url: `/topologies/${topologyID}/revisions/${revisionID}/diagnose`,
    method: 'post',
    data: {
      rollout_group: options.rolloutGroup || '',
      failure_policy: options.failurePolicy || 'stop_and_rollback'
    }
  }))
}

// For compatibility with early alpha clients, an object-only call remains a
// graph validator. The normal two-ID form is the server-side read-only
// diagnose endpoint, including assignments, releases and rollout checks.
export async function diagnoseKernelTopology(topologyID, revisionID, options = {}) {
  if (topologyID && typeof topologyID === 'object' && revisionID === undefined) {
    return validateKernelTopology(topologyID)
  }
  return diagnoseKernelTopologyDeployment(topologyID, revisionID, options)
}

export async function getKernelOperations() {
  return unwrap(await v3({ url: '/operations', method: 'get' }))
}

export async function cancelKernelOperation(operationID) {
  return unwrap(await v3({ url: `/operations/${operationID}/cancel`, method: 'post' }))
}

export async function getKernelExtensions() {
  return unwrap(await v3({ url: '/extensions', method: 'get' }))
}
