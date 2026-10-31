import request from '@/utils/request'

const KERNEL_API_BASE_URL = '/api/v3'
const KERNEL_V4_API_BASE_URL = '/api/v4'
const PANEL_API_BASE_URL = '/api/v5'

function v3(config) {
  return request({ ...config, baseURL: KERNEL_API_BASE_URL })
}

function v4(config) {
  return request({ ...config, baseURL: KERNEL_V4_API_BASE_URL })
}

function v5(config) {
  return request({ ...config, baseURL: PANEL_API_BASE_URL })
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

export async function getKernelInstallations() {
  return unwrap(await v3({ url: '/plugin-installations', method: 'get' }))
}

export async function getKernelNodeAssignments(nodeID) {
  return unwrap(await v3({ url: `/nodes/${nodeID}/assignments`, method: 'get' }))
}

// Restarts a built-in package's host: the only installation action (the
// built-in packages are installed, upgraded and enabled at every start).
export async function restartKernelInstallation(installationID, idempotencyKey) {
  return operationResult(await v3({
    url: `/plugin-installations/${installationID}/actions`,
    method: 'post',
    data: { action: 'restart', idempotency_key: idempotencyKey },
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

export async function getKernelExtensions() {
  return unwrap(await v3({ url: '/extensions', method: 'get' }))
}

// The agent transport inventory (A2-6), /api/v4: each node's last agent
// transport, agent version, certificate (state, last_certificate with
// not_after / renew_after / revoked_at / revoke_reason) and connection type.
// The argument is { nodes }, where nodes are "proxy-<id>" / "forward-<id>"
// names (at most 200, one comma separated `node` value) and the answer keeps
// those nodes only.
export async function getKernelAgentTransports({ nodes = [] } = {}) {
  const config = { url: '/kernel/agents/transports', method: 'get' }
  const params = {}
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

// What an upgrade would do now: the Agent release a campaign pushes by
// default (target_version, "" when Control's https address is not set),
// this Control's release, the nodes it would include (eligible_nodes), the
// default batches with the H19 minimums, the active campaign's id, and
// can_start (super administrators).
export async function getKernelAgentUpgradePlan() {
  return unwrap(await v4({ url: '/kernel/agents/upgrade-plan', method: 'get' }))
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

// Node diagnostics (5.0 node page, 诊断), `node` is "proxy-<id>" or
// "forward-<id>". The listing says what the node's Agent session runs
// (connected, diagnostics, forward_checks, actions, services, can_restart)
// and, for a proxy node, its newest recorded tasks.
export async function getKernelNodeDiagnostics(node, limit) {
  const config = { url: `/kernel/nodes/${encodeURIComponent(node)}/diagnostics`, method: 'get' }
  if (limit) config.params = { limit }
  return unwrap(await v4(config))
}

// Runs one whitelisted action on the node's Agent and waits for its result
// (the request lasts up to timeoutSeconds, 1 to 120, default 30):
// service_status, service_restart (super administrators), log_tail
// ({ service, lines }) and the forward checks forward.listen,
// forward.port_conflict, forward.connect, forward.udp_probe ({ route_id,
// hop_index }). The answer is { task_id, node, action, params, success,
// output, error, duration_ms, result? }; success false is an answer too.
// Refusals carry error.code: node_offline, diagnostics_unsupported,
// node_pending_approval, diagnostic_busy (409), super_admin_required (403),
// action_not_allowed, invalid_params (400), diagnostic_timeout (504),
// agent_error (502).
export async function runKernelNodeDiagnostic(node, { action, params = {}, timeoutSeconds } = {}) {
  const data = { action, params }
  const seconds = Number(timeoutSeconds) > 0 ? Number(timeoutSeconds) : 30
  if (timeoutSeconds) data.timeout_seconds = seconds
  return unwrap(await v4({
    url: `/kernel/nodes/${encodeURIComponent(node)}/diagnostics`,
    method: 'post',
    data,
    timeout: (seconds + 10) * 1000
  }))
}

// Reality target probing (目标站探测), `node` is "proxy-<id>". The report
// is { report: { node_id, node_address, current: [{ protocol_id, name,
// server_name, dest }], recommended, targets: [{ server_name, port, verdict
// (ok | failed | error), score, used_by, probed_at, error, result }] },
// support: { connected, capable, agent_version }, auto: { enabled,
// next_run_at, last_run_at, last_switch_at, fail_streak, last_outcome },
// default_candidates }.
export async function getKernelRealityTargets(node) {
  return unwrap(await v4({ url: `/kernel/nodes/${encodeURIComponent(node)}/reality-targets`, method: 'get' }))
}

// The node's Agent probes the candidates (host or host:port) and, unless
// includeDefaults is false, the built-in list; a run takes up to 90 s.
// Refusals carry error.code: node_offline, reality_probe_unsupported,
// probe_busy (409), invalid_params (400).
export async function probeKernelRealityTargets(node, { candidates = [], includeDefaults = true } = {}) {
  return unwrap(await v4({
    url: `/kernel/nodes/${encodeURIComponent(node)}/reality-targets/probe`,
    method: 'post',
    data: { candidates, include_defaults: includeDefaults },
    timeout: 120 * 1000
  }))
}

// Writes a target into one Reality protocol of the node: { previous,
// applied }. Refusals: target_not_probed (409, unless force),
// protocol_not_found (404), invalid_params (400).
export async function applyKernelRealityTarget(node, { protocolId, serverName, port = 443, force = false }) {
  return unwrap(await v4({
    url: `/kernel/nodes/${encodeURIComponent(node)}/reality-targets/apply`,
    method: 'post',
    data: { protocol_id: protocolId, server_name: serverName, port, force }
  }))
}

// Turns the node's automatic Reality target selection on or off; answers
// the auto state.
export async function setKernelRealityTargetAuto(node, enabled) {
  return unwrap(await v4({
    url: `/kernel/nodes/${encodeURIComponent(node)}/reality-targets/auto`,
    method: 'put',
    data: { enabled: Boolean(enabled) }
  }))
}

// A proxy node's entries (入口), `node` is "proxy-<id>": { node_id, entries:
// [{ id, label, kind (direct | nat | iwan), host, family (ipv4 | ipv6 |
// domain), port_map: [{ from, to, external }], enabled, sort, iwan_line_id }] }. A node with
// enabled entries is offered in subscriptions once per entry and protocol.
export async function getKernelNodeEntries(node) {
  return unwrap(await v4({ url: `/kernel/nodes/${encodeURIComponent(node)}/entries`, method: 'get' }))
}

// Replaces the node's entries with `entries` ([{ label, kind, host,
// port_map, enabled }], in order); an empty list removes them all.
// Refusals: invalid_request (400) with the reason, not_found (404).
export async function replaceKernelNodeEntries(node, entries) {
  return unwrap(await v4({
    url: `/kernel/nodes/${encodeURIComponent(node)}/entries`,
    method: 'put',
    data: { entries }
  }))
}

// A node's network settings, `node` is "proxy-<id>" or "forward-<id>": {
// interface, source_v4, gateway_v4, source_v6, gateway_v6, hop_port }. The
// network model's automatic egresses of the node leave by the interface
// (empty: the default route); its automatic hop ingress listens on hop_port
// (0: 24443), reached through a NAT entry whose port map covers it.
export async function getKernelNodeNetwork(node) {
  return unwrap(await v4({ url: `/kernel/nodes/${encodeURIComponent(node)}/network`, method: 'get' }))
}

// Replaces the node's network settings and replans the routes through it.
// Refusals: invalid_request (400) with violations, not_found (404).
export async function setKernelNodeNetwork(node, settings) {
  return unwrap(await v4({
    url: `/kernel/nodes/${encodeURIComponent(node)}/network`,
    method: 'put',
    data: settings
  }))
}

// Panabit iWAN lines: { lines: [{ id, name, host, port, username, mtu,
// encrypt, entries }] }, never a password. An iwan node entry reaches its
// node through a line (iwan_line_id).
export async function listIWANLines() {
  return unwrap(await v4({ url: '/kernel/iwan-lines', method: 'get' }))
}

// Adds a line ({ name, host, port, username, password, mtu, encrypt }):
// { line }. Refusals: invalid_request (400) with the reason.
export async function createIWANLine(line) {
  return unwrap(await v4({ url: '/kernel/iwan-lines', method: 'post', data: line }))
}

// Changes a line; an empty password keeps the stored one: { line }.
export async function updateIWANLine(id, line) {
  return unwrap(await v4({ url: `/kernel/iwan-lines/${encodeURIComponent(id)}`, method: 'put', data: line }))
}

// Deletes a line; in_use (409) while node entries use it.
export async function deleteIWANLine(id) {
  return unwrap(await v4({ url: `/kernel/iwan-lines/${encodeURIComponent(id)}`, method: 'delete' }))
}

// A forward route in subscriptions (加入订阅, Phase 10): { route_id,
// route_name, published, display_name, protocol: { id, node_id, node_name,
// name, type, port } | null, entries: [{ node_ref, node_name, host, port }],
// reason: '' | no_protocol | paused | l4_mismatch | no_entry, message }.
export async function getRouteSubscription(routeId) {
  return unwrap(await v4({ url: `/kernel/forward-routes/${encodeURIComponent(routeId)}/subscription`, method: 'get' }))
}

// Publishes (published true) or withdraws the route; refusals are
// invalid_request (400) with the reason and not_found (404).
export async function setRouteSubscription(routeId, published, displayName = '') {
  return unwrap(await v4({
    url: `/kernel/forward-routes/${encodeURIComponent(routeId)}/subscription`,
    method: 'put',
    data: { published, display_name: displayName }
  }))
}

// One-command node onboarding: a single-use enrollment token bound to the
// node ("proxy-<id>" or "forward-<id>") and the install command for every
// mirror. Super administrators only; the token is shown once.
export async function createKernelAgentInstallToken(node, ttlSeconds) {
  const data = { node }
  if (ttlSeconds) data.ttl_seconds = ttlSeconds
  return unwrap(await v4({ url: '/kernel/agents/install-tokens', method: 'post', data }))
}

// What an install command needs from this Control, checked without a network
// call (any administrator): { ready, checks: [{ id, ok, level, message, fix }] }.
// `ready` is false while a check of level "error" fails; a failed "warning"
// does not block the command. Ids: public_url, panel_certificate, grpc,
// grpc_certificate, ca, agent_release, script_signature.
export async function getKernelInstallReadiness() {
  return unwrap(await v4({ url: '/kernel/agents/install-readiness', method: 'get' }))
}

// Node groups (5.0 node onboarding). A group's join token makes any machine
// that runs its install command a node of the group.
//
// The list answers { groups: [...] }: each group with its defaults,
// token_reset_at, token_expires_at, token_expired and nodes { live, pending }. Never a token.
export async function listKernelNodeGroups() {
  return unwrap(await v4({ url: '/kernel/node-groups', method: 'get' }))
}

// The first group of a panel needs no input: this creates the default group
// (默认设置，接入的节点保持干净，没有协议) when none exists and
// answers the first group otherwise: { group, created }. 409 ca_kek_missing
// without module_runtime.ca_kek.
export async function ensureKernelDefaultNodeGroup() {
  return unwrap(await v4({ url: '/kernel/node-groups/default', method: 'post' }))
}

// input: { name, max_nodes, require_approval,
// rate, tags, monthly_limit_bytes, reset_day, sort,
// token_expires_at }. Errors: 400 invalid_request
// (the message names the field), 409
// name_taken, 409 ca_kek_missing.
export async function createKernelNodeGroup(input) {
  return unwrap(await v4({ url: '/kernel/node-groups', method: 'post', data: input }))
}

// Replaces every field of the group but its token.
export async function updateKernelNodeGroup(id, input) {
  return unwrap(await v4({ url: `/kernel/node-groups/${encodeURIComponent(id)}`, method: 'put', data: input }))
}

// 409 node_group_in_use while the group has live nodes.
export async function deleteKernelNodeGroup(id) {
  return unwrap(await v4({ url: `/kernel/node-groups/${encodeURIComponent(id)}`, method: 'delete' }))
}

// The group's install commands (one per download source), join token, expiry
// and the uninstall command. Super administrators only (403
// super_admin_required); every read is audited.
export async function getKernelNodeGroupCommand(id) {
  return unwrap(await v4({ url: `/kernel/node-groups/${encodeURIComponent(id)}/command`, method: 'get' }))
}

// Replaces the group's token: every earlier command stops working. Super
// administrators only.
export async function resetKernelNodeGroupToken(id) {
  return unwrap(await v4({ url: `/kernel/node-groups/${encodeURIComponent(id)}/reset-token`, method: 'post' }))
}

// The group's live nodes, newest first: { nodes: [{ id, node, kind, name,
// host, approval, online, last_seen, agent_version, created_at }] }.
export async function getKernelNodeGroupNodes(id) {
  return unwrap(await v4({ url: `/kernel/node-groups/${encodeURIComponent(id)}/nodes`, method: 'get' }))
}

// Approve or reject a node that joined a group requiring approval. `node` is
// "proxy-<id>" or "forward-<id>". 409 node_not_pending for any other node.
export async function approveKernelNode(node) {
  return unwrap(await v4({ url: `/kernel/nodes/${encodeURIComponent(node)}/approve`, method: 'post' }))
}

export async function rejectKernelNode(node) {
  return unwrap(await v4({ url: `/kernel/nodes/${encodeURIComponent(node)}/reject`, method: 'post' }))
}

// The console's node list: GET /api/v2/admin/nodes' rows (same query: page,
// page_size, search, status, group_id, sort, order) with each node's
// `approval` (approved, pending or rejected), `node_group_id` and
// `node_group_name`; `approval` and `node_group_id` also filter. Answers
// { total, list }.
export async function getKernelAdminNodes(params = {}) {
  const config = { url: '/admin/nodes', method: 'get' }
  if (Object.keys(params).length) config.params = params
  return unwrap(await v4(config))
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

// The certificates Control serves itself (GET /api/v5/panel/status, any
// administrator): { certificate: { mode (acme|files|external), state
// (not_configured|pending|valid|failed), domain, grpc_name, certificates:
// [{ name, roles, source, state, serving (acme|file|self_signed),
// publicly_trusted, not_after, renew_at, renewal_failing, last_error,
// problem, fix, next_attempt_at }] } }.
export async function getPanelStatus() {
  return unwrap(await v5({ url: '/panel/status', method: 'get' }))
}

// The options of 系统设置 (GET /api/v5/settings/catalog, any administrator):
// { items: [{ key, section (general|data), kind (domains|switch|choice|link),
// choices, min, max, default, value, link, sensitive }] }.
export async function getSettingsCatalog() {
  return unwrap(await v5({ url: '/settings/catalog', method: 'get' }))
}

// Asks Control to request the failing certificates again now, within its
// limit of three requests per name and hour. Answers { retry: { retrying,
// waiting, healthy }, certificate }.
export async function retryPanelCertificates() {
  return unwrap(await v5({ url: '/panel/certificates/retry', method: 'post' }))
}

// The panel's backups (GET /api/v5/backups, any administrator): { supported,
// reason (why not, on PostgreSQL), backups: [{ id, file, type
// (daily|pre-migration|manual|verify), size, sha256, created_at, verified_at,
// verify_status (pending|passed|failed), verify_error, program_version,
// schema_version }], summary: { last_verified (a backup or null), stale,
// stale_after_hours, next_daily_at, directory }, retention: { daily, weekly,
// pre_migration_days, manual, verify } }.
export async function getPanelBackups() {
  return unwrap(await v5({ url: '/backups', method: 'get' }))
}

// Back up now: a manual bundle (POST /api/v5/backups, any administrator).
// Answers { backup } pending verification, which follows in the background.
// 409 backups_need_sqlite or backup_in_progress.
export async function createPanelBackup() {
  return unwrap(await v5({ url: '/backups', method: 'post', timeout: 300_000 }))
}

// Downloads a bundle (GET /api/v5/backups/:id/download). It holds the root
// key: a super administrator only (403 super_admin_required), and audited.
// Answers { blob, filename }.
export async function downloadPanelBackup(id) {
  const response = await v5({
    url: `/backups/${encodeURIComponent(id)}/download`, method: 'get', responseType: 'blob', rawResponse: true, timeout: 0
  })
  const disposition = String(response?.headers?.['content-disposition'] || '')
  const match = /filename="([^"]+)"/.exec(disposition)
  return { blob: response.data, filename: match ? match[1] : `backup-${id}.tar` }
}

// Egress routes: domains a proxy node's users open that leave through
// another proxy node. { routes: [{ id, name, source_node, exit_node (each
// { id, name, host, exists }), exit_host, relay_port, method, domains,
// enabled, source_state, exit_state (each { state: applied | pending |
// failed, error? }) }] }. Never the relay key.
export async function listKernelEgressRoutes() {
  return unwrap(await v4({ url: '/kernel/egress-routes', method: 'get' }))
}

// input: { name, source_node_id, exit_node_id, exit_host, relay_port,
// domains: [], enabled }. Errors: 400 invalid_request (the message names the
// field), 409 port_taken.
export async function createKernelEgressRoute(input) {
  return unwrap(await v4({ url: '/kernel/egress-routes', method: 'post', data: input }))
}

// Replaces every field; rotate_key: true replaces the relay key too.
export async function updateKernelEgressRoute(id, input) {
  return unwrap(await v4({ url: `/kernel/egress-routes/${encodeURIComponent(id)}`, method: 'put', data: input }))
}

export async function deleteKernelEgressRoute(id) {
  return unwrap(await v4({ url: `/kernel/egress-routes/${encodeURIComponent(id)}`, method: 'delete' }))
}

// The device network model (/api/v4/kernel/network): { revision, document:
// { lines, ingresses, egresses, transports, paths }, pending, devices }.
export async function getNetworkModel() {
  return unwrap(await v4({ url: '/kernel/network', method: 'get' }))
}
