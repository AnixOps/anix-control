// F5b mockups: a stand-in for POST /routes/preview. The real answer comes
// from sdk/forward/validate and the planner; this mirrors a few rules so the
// editor can show violations mapped to fields by their stable codes.
import { MOCK_NOW, nodeByRef, H21_DEFAULTS } from './mockData'

// What the UI says for each validate code (the API message is English and
// is shown as the detail). Codes: sdk/forward/validate/violation.go.
export const CODE_TEXT = {
  required: '必填',
  invalid_format: '格式不正确',
  out_of_range: '超出允许范围',
  too_many: '数量超过上限',
  duplicate: '重复',
  invalid_role: '跳的角色顺序不正确',
  engine_not_enabled: '该引擎未启用',
  link_unsupported: '上一跳的引擎无法发起这种链路，或本跳无法终止它',
  not_applicable: '此设置在当前组合下无效',
  requires_single_node: '只有一个节点时才能指定拨号地址',
  forbidden: '不允许',
  target_not_allowed: '目标策略不允许此地址',
  invalid_relation: '取值之间互相矛盾',
  unused_hops: '强制直连时不能有后续跳',
  expired: '到期时间必须晚于现在',
  unknown_node: '节点不存在或未加入转发清单',
  engine_not_advertised: '节点没有上报这个引擎',
  engine_unavailable: '节点上的引擎当前不可用',
  capability_missing: '节点的引擎缺少所需能力',
  port_out_of_range: '端口不在节点的端口范围内',
  port_reserved: '端口是节点的保留端口',
  port_in_use: '端口已被其他路由占用',
  port_exhausted: '节点的端口范围已用尽',
  no_port_range: '节点没有配置端口范围',
  mark_exhausted: '节点的连接标记已用尽',
  no_address: '节点没有可用地址',
  node_in_use: '节点正被路由使用'
}

function caps(ref, engine) {
  return nodeByRef(ref)?.info.engines.find(item => item.engine === engine)
}

function isPrivate(host) {
  return /^(10\.|192\.168\.|172\.(1[6-9]|2\d|3[01])\.|127\.)/.test(host)
}

// validateDraft answers [{field, code, message}] like the API's violations.
export function validateDraft(route, { onCreate = true } = {}) {
  const out = []
  const add = (field, code, message) => out.push({ field, code, message, route_id: '' })
  if (!route.name?.trim()) add('name', 'required', 'name is required')
  const listenPort = route.listen?.port || 0
  if (listenPort < 0 || listenPort > 65535) add('listen.port', 'out_of_range', 'port must be 1..65535')
  if (!route.hops.length) add('hops', 'required', 'a route needs at least one hop')

  route.hops.forEach((item, index) => {
    const path = `hops[${index}]`
    if (!item.node_refs.length) add(`${path}.node_refs`, 'required', 'a hop needs at least one node')
    for (const ref of item.node_refs) {
      const node = nodeByRef(ref)
      if (!node || !node.enabled) {
        add(`${path}.node_refs`, 'unknown_node', `node ${ref} is not in the inventory`)
        continue
      }
      const engine = caps(ref, item.engine)
      if (!engine) add(`${path}.engine`, 'engine_not_advertised', `${ref} does not advertise ${item.engine}`)
      else if (!engine.available) add(`${path}.engine`, 'engine_unavailable', `${ref}: ${engine.unavailable_reason}`)
      else if (route.listen.protocol !== 'L4_PROTOCOL_TCP' && !engine.udp) add(`${path}.engine`, 'capability_missing', `${ref} ${item.engine} has no UDP`)
      if (index === 0 && listenPort) {
        const range = node.settings.port_range
        if (node.reserved_ports.includes(listenPort)) add('listen.port', 'port_reserved', `port ${listenPort} is reserved on ${ref}`)
        else if (listenPort < range.first || listenPort > range.last) add('listen.port', 'port_out_of_range', `port ${listenPort} is outside ${range.first}-${range.last} on ${ref}`)
      }
    }
    if (item.dial_address && item.node_refs.length > 1) add(`${path}.dial_address`, 'requires_single_node', 'dial_address needs exactly one node')
    if (index > 0) {
      const security = item.ingress?.security || 'LINK_SECURITY_RAW'
      const previous = route.hops[index - 1]
      const originates = previous.node_refs.every(ref => caps(ref, previous.engine)?.link_securities.includes(security))
      const terminates = item.node_refs.every(ref => caps(ref, item.engine)?.link_securities.includes(security))
      if (previous.node_refs.length && item.node_refs.length && (!originates || !terminates)) {
        add(`${path}.ingress.security`, 'link_unsupported', `${previous.engine} cannot originate ${security}`)
      }
      if (item.ingress?.mux && security === 'LINK_SECURITY_RAW') add(`${path}.ingress.mux`, 'not_applicable', 'mux needs an encrypted link')
    }
  })

  if (route.policy?.direct === 'DIRECT_MODE_FORCED' && route.hops.length > 1) add('policy.direct', 'unused_hops', 'FORCED direct mode leaves later hops unused')
  if (!route.targets.length) add('targets', 'required', 'a route needs at least one target')
  route.targets.forEach((target, index) => {
    if (!target.host?.trim()) add(`targets[${index}].host`, 'required', 'host is required')
    else if (isPrivate(target.host) && route.policy?.target_policy !== 'TARGET_POLICY_ALLOW_PRIVATE') add(`targets[${index}].host`, 'target_not_allowed', `${target.host} is private and the target policy is PUBLIC_ONLY`)
    if (!target.port || target.port > 65535) add(`targets[${index}].port`, 'out_of_range', 'port must be 1..65535')
  })
  const expires = Number(route.limits?.expires_at_unix_ms || 0)
  if (onCreate && expires && expires <= MOCK_NOW) add('limits.expires_at_unix_ms', 'expired', 'expires_at must be in the future')
  return out
}

// warningsFor answers the preview's warnings (strings in the API).
export function warningsFor(route) {
  const out = []
  route.hops.forEach((item, index) => {
    const strategy = index === route.hops.length - 1 ? route.policy?.target : route.policy?.next_hop
    if (strategy === 'BALANCE_STRATEGY_LEAST_CONN' && item.engine === 'ENGINE_NFTABLES') {
      out.push(`第 ${index + 1} 跳：nftables 上的“最少连接”在 Agent 提供 conntrack 计数前按权重随机分配。`)
    }
  })
  if (route.hops[0]?.node_refs.length > 1 && !route.listen.entry_hostname) {
    out.push('入口有多个节点但没有入口域名：客户端只能直连某一个入口。')
  }
  return out
}

// previewDraft answers {states, allocations, violations, warnings}, the
// shape of POST /routes/preview, flattened for the panel.
export function previewDraft(route, options = {}) {
  const violations = validateDraft(route, options)
  const warnings = warningsFor(route)
  if (violations.length) return { states: [], allocations: [], violations, warnings }
  const allocations = []
  const states = []
  route.hops.forEach((item, index) => {
    item.node_refs.forEach((ref, position) => {
      const node = nodeByRef(ref)
      const port = index === 0
        ? (route.listen.port || node.settings.port_range.first + 17)
        : (item.port || node.settings.port_range.first + 21 + position)
      const mark = 40 + index * 3 + position
      allocations.push({ route_id: '', hop_index: index, node_ref: ref, port, mark })
      const next = route.hops[index + 1]
      const upstreams = next
        ? next.node_refs.map((nextRef, priority) => ({ label: `${nodeByRef(nextRef)?.name} :${next.port || nodeByRef(nextRef).settings.port_range.first + 21 + priority}`, priority, security: next.ingress?.security || 'LINK_SECURITY_RAW' }))
        : route.targets.map(target => ({ label: `${target.host}:${target.port}`, priority: target.priority || 0, security: 'LINK_SECURITY_RAW' }))
      states.push({
        node_ref: ref,
        name: node.name,
        hop_index: index,
        role: item.role,
        engine: item.engine,
        listen: `${index === 0 ? route.listen.address || '0.0.0.0' : item.dial_address || node.settings.addresses[0]}:${port}`,
        ingress: index === 0 ? 'LINK_SECURITY_RAW' : (item.ingress?.security || 'LINK_SECURITY_RAW'),
        upstreams,
        mark,
        generation: `${node.desired_generation} → ${Number(node.desired_generation) + 1}`
      })
    })
  })
  return { states, allocations, violations, warnings }
}

export function healthDefaults(policy) {
  return {
    interval_ms: policy?.health?.interval_ms || H21_DEFAULTS.interval_ms,
    timeout_ms: policy?.health?.timeout_ms || H21_DEFAULTS.timeout_ms,
    failure_threshold: policy?.circuit_breaker?.failure_threshold || H21_DEFAULTS.failure_threshold,
    open_ms: policy?.circuit_breaker?.open_ms || H21_DEFAULTS.open_ms
  }
}
