export function topologyGraphValue(value) {
  if (typeof value === 'string') {
    try {
      return JSON.parse(value || '{}')
    } catch {
      return {}
    }
  }
  return value && typeof value === 'object' ? value : {}
}

export function topologyInputFromJSON(json, message, invalidJSONMessage) {
  let value
  try {
    value = JSON.parse(json || '{}')
  } catch {
    throw new Error(invalidJSONMessage)
  }

  const vertices = Array.isArray(value?.vertices) ? value.vertices.map(vertex => ({
    key: String(vertex?.key || '').trim(),
    kind: String(vertex?.kind || '').trim(),
    node_id: vertex?.node_id === null || vertex?.node_id === undefined || vertex?.node_id === '' ? null : Number(vertex.node_id),
    plugin_id: String(vertex?.plugin_id || '').trim(),
    role: String(vertex?.role || '').trim(),
    config: JSON.stringify(topologyGraphValue(vertex?.config ?? vertex?.config_json)),
  })) : []
  const edges = Array.isArray(value?.edges) ? value.edges.map(edge => ({
    source_key: String(edge?.source_key || '').trim(),
    target_key: String(edge?.target_key || '').trim(),
    protocol: String(edge?.protocol || '').trim(),
    secret_id: String(edge?.secret_id || '').trim(),
    config: JSON.stringify(topologyGraphValue(edge?.config ?? edge?.config_json)),
  })) : []

  return { message: String(message || '').trim(), vertices, edges }
}

export function topologyJSONFromDetail(detail) {
  const vertices = Array.isArray(detail?.vertices) ? detail.vertices.map(vertex => ({
    key: vertex.key,
    kind: vertex.kind,
    ...(vertex.node_id ? { node_id: vertex.node_id } : {}),
    ...(vertex.plugin_id ? { plugin_id: vertex.plugin_id } : {}),
    ...(vertex.role ? { role: vertex.role } : {}),
    config: topologyGraphValue(vertex.config),
  })) : []
  const edges = Array.isArray(detail?.edges) ? detail.edges.map(edge => ({
    source_key: edge.source_key,
    target_key: edge.target_key,
    protocol: edge.protocol,
    ...(edge.secret_id ? { secret_id: edge.secret_id } : {}),
    config: topologyGraphValue(edge.config),
  })) : []

  return JSON.stringify({ vertices, edges }, null, 2)
}

export function extractNodes(response, fallbackMessage = 'Unable to load nodes') {
  if (response?.code !== undefined && response.code !== 0) {
    throw new Error(response.msg || fallbackMessage)
  }
  const payload = response?.code !== undefined ? response.data : response?.data?.data ?? response?.data ?? response
  const rows = Array.isArray(payload) ? payload : payload?.list
  return Array.isArray(rows)
    ? rows.map(node => ({ ...node, id: Number(node.id) })).filter(node => node.id > 0)
    : []
}

export function assignmentPayload(source, overrides = {}) {
  return {
    service_scope: source.service_scope,
    plugin_id: source.plugin_id,
    role: source.role,
    desired_version: source.desired_version || '',
    desired_config_revision: Math.max(0, Number(source.desired_config_revision ?? 0)),
    enabled: Boolean(source.enabled),
    rollout_group: source.rollout_group || '',
    ...overrides,
  }
}
