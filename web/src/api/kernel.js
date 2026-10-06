import request from '@/utils/request'

const KERNEL_API_BASE_URL = '/api/v3'
const KERNEL_V4_API_BASE_URL = '/api/v4'

function v3(config) {
  return request({ ...config, baseURL: KERNEL_API_BASE_URL })
}

function v4(config) {
  return request({ ...config, baseURL: KERNEL_V4_API_BASE_URL })
}

function unwrap(response) {
  return response?.data ?? response
}

function operationResult(response) {
  const result = response?.data?.data ?? response?.data ?? response
  if (!result || typeof result !== 'object' || Array.isArray(result)) return result
  return {
    ...result,
    operation_id: response?.headers?.['x-anixops-operation-id'] || '',
    operation_chain: response?.headers?.['x-anixops-operation-chain'] || '',
  }
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
  return operationResult(await v3({
    url: '/plugin-installations',
    method: 'put',
    data: installation,
    rawResponse: true,
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
  return operationResult(await v3({
    url: `/plugin-installations/${installationID}/actions`,
    method: 'post',
    data,
    rawResponse: true,
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
  return operationResult(await v3({
    url: `/plugin-installations/${installationID}/config`,
    method: 'put',
    data,
    rawResponse: true,
  }))
}

export async function getKernelScopes() {
  return unwrap(await v3({ url: '/service-scopes', method: 'get' }))
}

export async function getKernelAccessGroups(scopeID) {
  const config = { url: '/access-groups', method: 'get' }
  if (scopeID) {
    config.params = { scope_id: scopeID }
  }
  return unwrap(await v3(config))
}

export async function getKernelAccessGroupDetail(groupID) {
  return unwrap(await v3({ url: `/access-groups/${groupID}`, method: 'get' }))
}

export async function createKernelAccessGroup(group) {
  return unwrap(await v3({ url: '/access-groups', method: 'post', data: group }))
}

export async function updateKernelAccessGroup(groupID, group) {
  return unwrap(await v3({ url: `/access-groups/${groupID}`, method: 'put', data: group }))
}

export async function deleteKernelAccessGroup(groupID) {
  return unwrap(await v3({ url: `/access-groups/${groupID}`, method: 'delete' }))
}

export async function addKernelAccessGroupUser(groupID, userID) {
  return unwrap(await v3({ url: `/access-groups/${groupID}/users`, method: 'post', data: { user_id: userID } }))
}

export async function removeKernelAccessGroupUser(groupID, userID) {
  return unwrap(await v3({ url: `/access-groups/${groupID}/users/${userID}`, method: 'delete' }))
}

export async function addKernelAccessGroupPlan(groupID, planID) {
  return unwrap(await v3({ url: `/access-groups/${groupID}/plans`, method: 'post', data: { plan_id: planID } }))
}

export async function removeKernelAccessGroupPlan(groupID, planID) {
  return unwrap(await v3({ url: `/access-groups/${groupID}/plans/${planID}`, method: 'delete' }))
}

export async function createKernelResourceGrant(grant) {
  return unwrap(await v3({ url: '/resource-grants', method: 'post', data: grant }))
}

export async function deleteKernelResourceGrant(grantID) {
  return unwrap(await v3({ url: `/resource-grants/${grantID}`, method: 'delete' }))
}

export async function upsertKernelQuotaPolicy(policy) {
  return unwrap(await v3({ url: '/quota-policies', method: 'put', data: policy }))
}

export async function deleteKernelQuotaPolicy(policyID) {
  return unwrap(await v3({ url: `/quota-policies/${policyID}`, method: 'delete' }))
}

export async function resolveKernelAccess({ userID, scopeID, planID }) {
  const params = { user_id: userID, scope_id: scopeID }
  if (planID) {
    params.plan_id = planID
  }
  return unwrap(await v3({ url: '/access-groups/resolve', method: 'get', params }))
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

export async function getKernelOperations() {
  return unwrap(await v3({ url: '/operations', method: 'get' }))
}

export async function cancelKernelOperation(operationID) {
  return operationResult(await v3({
    url: `/operations/${operationID}/cancel`,
    method: 'post',
    rawResponse: true,
  }))
}

export async function getKernelExtensions() {
  return unwrap(await v3({ url: '/extensions', method: 'get' }))
}

// Per-package v2 route modes (legacy / shadow / native), /api/v4.
export async function getKernelRouteModes(packageID) {
  const config = { url: '/kernel/route-modes', method: 'get' }
  if (packageID) config.params = { package_id: packageID }
  return unwrap(await v4(config))
}

// input: { package_id, routes?, mode, reason?, confirm? }; switching to
// native needs confirm: true and a reason.
export async function setKernelRouteModes(input) {
  return unwrap(await v4({ url: '/kernel/route-modes', method: 'post', data: input }))
}

export async function rollbackKernelRouteModes(packageID, reason = '') {
  const data = { package_id: packageID }
  if (reason) data.reason = reason
  return unwrap(await v4({ url: '/kernel/route-modes/rollback', method: 'post', data }))
}

export async function getKernelRouteModeRevisions(packageID, limit) {
  const params = {}
  if (packageID) params.package_id = packageID
  if (limit) params.limit = limit
  return unwrap(await v4({ url: '/kernel/route-modes/revisions', method: 'get', params }))
}

// The agent transport inventory (A2-6), /api/v4: each node's last agent
// transport, agent version, certificate (state, last_certificate with
// not_after / renew_after / revoked_at / revoke_reason) and connection type.
// The argument is legacyOnly (a boolean: the nodes agent_control.mtls:
// required would refuse) or { legacyOnly, nodes }, where nodes are
// "proxy-<id>" / "forward-<id>" names (at most 200, one comma separated
// `node` value) and the answer keeps those nodes only.
export async function getKernelAgentTransports(options = false) {
  const { legacyOnly = false, nodes = [] } = typeof options === 'object' && options !== null ? options : { legacyOnly: Boolean(options) }
  const config = { url: '/kernel/agents/transports', method: 'get' }
  const params = {}
  if (legacyOnly) params.legacy_only = true
  const names = [...new Set((Array.isArray(nodes) ? nodes : []).map(String).filter(Boolean))]
  if (names.length) params.node = names.join(',')
  if (Object.keys(params).length) config.params = params
  return unwrap(await v4(config))
}

// Staged Agent upgrades (O4, H19): campaigns push an Agent release in canary
// batches and roll a batch back when more than 5% of it fails. Reads are for
// administrators; start, pause, resume and abort for super administrators.
export async function listKernelAgentUpgrades(limit) {
  const config = { url: '/kernel/agents/upgrades', method: 'get' }
  if (limit) config.params = { limit }
  return unwrap(await v4(config))
}

export async function getKernelAgentUpgrade(id) {
  return unwrap(await v4({ url: `/kernel/agents/upgrades/${encodeURIComponent(id)}`, method: 'get' }))
}

export async function startKernelAgentUpgrade(input = {}) {
  return unwrap(await v4({ url: '/kernel/agents/upgrades', method: 'post', data: input }))
}

export async function pauseKernelAgentUpgrade(id) {
  return unwrap(await v4({ url: `/kernel/agents/upgrades/${encodeURIComponent(id)}/pause`, method: 'post' }))
}

export async function resumeKernelAgentUpgrade(id) {
  return unwrap(await v4({ url: `/kernel/agents/upgrades/${encodeURIComponent(id)}/resume`, method: 'post' }))
}

export async function abortKernelAgentUpgrade(id, rollback = false) {
  return unwrap(await v4({ url: `/kernel/agents/upgrades/${encodeURIComponent(id)}/abort`, method: 'post', data: { rollback } }))
}

// One-command node onboarding: a single-use enrollment token bound to the
// node ("proxy-<id>" or "forward-<id>") and the install command for every
// mirror. Super administrators only; the token is shown once.
export async function createKernelAgentInstallToken(node, ttlSeconds) {
  const data = { node }
  if (ttlSeconds) data.ttl_seconds = ttlSeconds
  return unwrap(await v4({ url: '/kernel/agents/install-tokens', method: 'post', data }))
}

// Rotate a node's Agent credentials (administrators with the super
// administrator right only). Revokes every Agent certificate, enrollment and
// forward link certificate of the node, optionally replaces a proxy node's API
// key (`rotateApiKey`; a forward node answers 400) and issues a fresh
// single-use `anixagt_` credential, valid for `ttlSeconds` (60 to 604800,
// default 3600). `reason` (at most 200 characters) goes to the audit entry.
// The answer carries the credential once ({ node, revoked, api_key_rotated,
// enrollment, expires_at, credential }) with Cache-Control: no-store: the
// caller keeps it only as long as it shows it and never writes it anywhere.
// Refusals: 400 invalid_request, 403 super_admin_required, 404 node_not_found,
// 409 node_disabled / agent_pki_disabled.
export async function rotateKernelAgentCredentials({ node, rotateApiKey = false, ttlSeconds, reason = '' }) {
  const data = { node }
  if (rotateApiKey) data.rotate_api_key = true
  if (ttlSeconds) data.ttl_seconds = ttlSeconds
  const why = String(reason ?? '').trim()
  if (why) data.reason = why
  return unwrap(await v4({ url: '/kernel/agents/rotate-credentials', method: 'post', data, timeout: 30_000 }))
}

// One proxy node's traffic over time (administrators, v4), for the node page's
// chart: { node_id, granularity, since_unix_ms, until_unix_ms, points:
// [{ start_unix_ms, up_bytes, down_bytes }], total: { up_bytes, down_bytes } },
// ascending, buckets without traffic as zeros. `hour` (UTC hours, from the raw
// traffic log; at most 720 buckets) or `day` (the Control host's calendar
// days, from the daily statistics; at most 366). `since` / `until` are Unix
// milliseconds, rounded out to whole buckets; a longer window is 400
// invalid_request and an unknown node 404 not_found.
export async function getKernelNodeTraffic(nodeId, { granularity, since, until } = {}) {
  const params = {}
  if (granularity) params.granularity = granularity
  if (since !== undefined && since !== null) params.since = since
  if (until !== undefined && until !== null) params.until = until
  const config = { url: `/kernel/nodes/${encodeURIComponent(nodeId)}/traffic`, method: 'get' }
  if (Object.keys(params).length) config.params = params
  return unwrap(await v4(config))
}

// Admin API tokens (docs/reference/admin-api-tokens.md): an administrator's
// personal access tokens for automation. All three routes need the signed-in
// session, which the console always has.
//
// List: the caller's active tokens, `includeInactive` adds revoked and expired
// ones; a super administrator may pass `all` (everyone's) or `userId` (one
// administrator's), anyone else gets 403 super_admin_required. Rows are
// { id, user_id, name, scope, hint, expires_at, last_used_at, last_used_ip,
// created_at, revoked_at, revoke_reason }, never a secret.
export async function listKernelApiTokens({ all = false, userId, includeInactive = false } = {}) {
  const params = {}
  if (includeInactive) params.include_inactive = true
  if (all) params.all = true
  else if (userId) params.user_id = userId
  const config = { url: '/kernel/api-tokens', method: 'get' }
  if (Object.keys(params).length) config.params = params
  return unwrap(await v4(config))
}

// Create: `name` (1 to 100 characters), `scope` ('read' or 'admin'), optional
// `expiresInDays` (1 to 730; none or 0 never expires) and the administrator's
// re-authentication: `password`, or with two-step verification on `code` and
// `method` ('totp' or 'backup'). Only the fields that are set are sent. The
// answer is { token, api_token } with Cache-Control: no-store: `token` is the
// `anixadm_` secret, shown once, and the caller keeps it only as long as it
// shows it and never writes it anywhere. The request is marked `sensitive`
// (utils/request.js), so a failed one logs no body. Refusals: 400
// invalid_request, 403 step_up_required (the message says password or MFA code),
// step_up_sign_in_stale (identity holds the credential and the sign-in is older
// than 10 minutes; older Controls said step_up_required with "sign in again"), step_up_failed,
// not_an_administrator, 409 too_many_tokens (25 active), 429
// step_up_rate_limited with Retry-After.
export async function createKernelApiToken({ name, scope, expiresInDays, password, code, method }) {
  const data = { name: String(name ?? '').trim(), scope }
  if (expiresInDays) data.expires_in_days = expiresInDays
  if (password) data.password = password
  if (code) {
    data.code = code
    data.method = method || 'totp'
  }
  return unwrap(await v4({ url: '/kernel/api-tokens', method: 'post', data, timeout: 30_000, sensitive: true }))
}

// Revoke: ends a token at once. The owner revokes their own, a super
// administrator anyone's; a token that is not yours or does not exist is 404.
// The answer is { api_token, changed } (changed false: it was revoked already).
export async function revokeKernelApiToken(id) {
  return unwrap(await v4({ url: `/kernel/api-tokens/${encodeURIComponent(id)}`, method: 'delete' }))
}

// Sanitized shadow-mode mismatch samples (newest first, at most 100):
// { samples, retention_days, max_per_route }.
export async function getKernelRouteModeMismatches({ packageID, routeID, limit } = {}) {
  const params = {}
  if (packageID) params.package_id = packageID
  if (routeID) params.route_id = routeID
  if (limit) params.limit = limit
  return unwrap(await v4({ url: '/kernel/route-modes/mismatches', method: 'get', params }))
}
