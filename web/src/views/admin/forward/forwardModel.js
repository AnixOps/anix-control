// Pure helpers of the forward rules page (flux-panel forward.tsx clone).
// They were part of Forward.vue; UI U7 moved them here unchanged so the page
// keeps only its state, its API calls and its template.

export function splitLines(value) {
  return String(value || '')
    .split('\n')
    .map(item => item.trim())
    .filter(Boolean)
}

export function splitRemoteAddresses(value) {
  return String(value || '')
    .split(',')
    .map(item => item.trim())
    .filter(Boolean)
}

export function arrayMove(list, fromIndex, toIndex) {
  const next = [...list]
  const [item] = next.splice(fromIndex, 1)
  next.splice(toIndex, 0, item)
  return next
}

export function normalizeTunnel(raw) {
  return {
    ...raw,
    inIp: raw.inIp ?? raw.in_ip ?? '',
    inNodePortSta: raw.inNodePortSta ?? raw.in_node_port_sta ?? null,
    inNodePortEnd: raw.inNodePortEnd ?? raw.in_node_port_end ?? null
  }
}

export function normalizeForward(raw) {
  const runtimeStatus = Number(raw.runtimeStatus ?? raw.runtime_status ?? 0)
  const runtimeBackend = String(raw.runtimeBackend ?? raw.runtime_backend ?? '').trim()
  const runtimeMessage = String(raw.runtimeMessage ?? raw.runtime_message ?? '').trim()
  const lastRuntimeSyncTime = Number(raw.lastRuntimeSyncTime ?? raw.last_runtime_sync_time ?? 0)
  const hasTrackedRuntime = Boolean(runtimeBackend || runtimeMessage || lastRuntimeSyncTime > 0 || runtimeStatus === 2 || runtimeStatus === 3)
  const serviceRunning = hasTrackedRuntime ? Number(raw.status) === 1 && runtimeStatus === 2 : Number(raw.status) === 1
  return {
    ...raw,
    id: Number(raw.id),
    tunnelId: Number(raw.tunnelId),
    inPort: Number(raw.inPort ?? 0),
    status: Number(raw.status ?? 0),
    runtimeStatus,
    runtimeBackend,
    runtimeMessage,
    lastRuntimeSyncTime,
    hasTrackedRuntime,
    inx: raw.inx == null ? 0 : Number(raw.inx),
    userId: raw.userId == null ? null : Number(raw.userId),
    serviceRunning
  }
}

export function isNodeXOnlyTunnel(tunnel) {
  return Number(tunnel?.type ?? 0) === 2
}

export function isForwardRuntimeBusy(forward) {
  return Boolean(forward?.hasTrackedRuntime) && [0, 1].includes(Number(forward?.runtimeStatus))
}

export function isForwardToggleDisabled(forward) {
  if (Number(forward?.status) !== 0 && Number(forward?.status) !== 1) {
    return true
  }
  return isForwardRuntimeBusy(forward)
}

// The status filter of the direct view: running, paused or error.
export function getDirectFilterStatus(forward) {
  const status = Number(forward?.status)
  if (status !== 0 && status !== 1) {
    return 'error'
  }
  if (Number(forward?.runtimeStatus) === 3) {
    return 'error'
  }
  return forward?.serviceRunning ? 'running' : 'paused'
}

export function hasValidInx(forward) {
  return forward?.inx !== undefined && forward?.inx !== null && Number(forward.inx) !== 0
}

export function normalizeInboundAddress(ip, port) {
  if (String(ip).includes(':') && !String(ip).startsWith('[')) {
    return `[${ip}]:${port}`
  }
  return `${ip}:${port}`
}

export function formatInAddress(ipString, port) {
  if (!ipString || !port) {
    return ''
  }
  const ips = splitRemoteAddresses(ipString)
  if (!ips.length) {
    return ''
  }
  if (ips.length === 1) {
    return normalizeInboundAddress(ips[0], port)
  }
  return `${normalizeInboundAddress(ips[0], port)} (+${ips.length - 1})`
}

export function formatRemoteAddress(addressString) {
  const addresses = splitRemoteAddresses(addressString)
  if (!addresses.length) {
    return ''
  }
  if (addresses.length === 1) {
    return addresses[0]
  }
  return `${addresses[0]} (+${addresses.length - 1})`
}

const IPV4_PATTERN = /^(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?):\d+$/
const IPV6_FULL_PATTERN = /^\[((([0-9a-fA-F]{1,4}:){7}([0-9a-fA-F]{1,4}|:))|(([0-9a-fA-F]{1,4}:){6}(:[0-9a-fA-F]{1,4}|((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3})|:))|(([0-9a-fA-F]{1,4}:){5}(((:[0-9a-fA-F]{1,4}){1,2})|:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3})|:))|(([0-9a-fA-F]{1,4}:){4}(((:[0-9a-fA-F]{1,4}){1,3})|((:[0-9a-fA-F]{1,4})?:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:))|(([0-9a-fA-F]{1,4}:){3}(((:[0-9a-fA-F]{1,4}){1,4})|((:[0-9a-fA-F]{1,4}){0,2}:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:))|(([0-9a-fA-F]{1,4}:){2}(((:[0-9a-fA-F]{1,4}){1,5})|((:[0-9a-fA-F]{1,4}){0,3}:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:))|(([0-9a-fA-F]{1,4}:){1}(((:[0-9a-fA-F]{1,4}){1,6})|((:[0-9a-fA-F]{1,4}){0,4}:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:))|(:(((:[0-9a-fA-F]{1,4}){1,7})|((:[0-9a-fA-F]{1,4}){0,5}:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:)))\]:\d+$/
const DOMAIN_PATTERN = /^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*:\d+$/

// Index (0-based) of the first target line that is not IPv4:port,
// [IPv6]:port or domain:port, or -1 when every line is valid.
export function findInvalidTargetLine(remoteAddr) {
  const addresses = splitLines(remoteAddr)
  return addresses.findIndex(address => (
    !IPV4_PATTERN.test(address) && !IPV6_FULL_PATTERN.test(address) && !DOMAIN_PATTERN.test(address)
  ))
}

// relay-panel compatible export: [{ dest: ["host:port"], listen_port, name }].
export function buildExportData(items) {
  const rules = items.map(item => ({
    dest: splitRemoteAddresses(item.remoteAddr),
    listen_port: Number(item.inPort) || null,
    name: item.name || ''
  }))
  return JSON.stringify(rules, null, 2)
}

function formatImportEntryLabel(entry) {
  if (entry?.source) {
    return entry.source
  }
  if (entry?.name) {
    return entry.name
  }
  return JSON.stringify(entry)
}

function normalizeImportEntry(raw, source = '') {
  const dest = Array.isArray(raw?.dest) ? raw.dest : []
  const remoteAddr = dest
    .map(item => String(item || '').trim())
    .filter(Boolean)
    .join(',')
  const name = String(raw?.name || '').trim()
  const listenPort = raw?.listen_port ?? raw?.listenPort ?? raw?.inPort ?? raw?.in_port ?? ''
  return {
    source: source || formatImportEntryLabel(raw),
    remoteAddr,
    name,
    inPortRaw: listenPort === null || listenPort === undefined ? '' : String(listenPort).trim()
  }
}

// relay-panel JSON, or the legacy `remoteAddr|name|inPort` lines.
export function parseImportEntries(rawText) {
  const raw = rawText.trim()
  if (!raw) {
    return []
  }

  if (raw.startsWith('[') || raw.startsWith('{')) {
    const parsed = JSON.parse(raw)
    const list = Array.isArray(parsed) ? parsed : [parsed]
    return list.map(item => normalizeImportEntry(item))
  }

  return raw
    .split('\n')
    .map(item => item.trim())
    .filter(Boolean)
    .map(line => {
      const parts = line.split('|')
      return {
        source: line,
        remoteAddr: String(parts[0] || '').trim(),
        name: String(parts[1] || '').trim(),
        inPortRaw: String(parts[2] || '').trim(),
        legacyParts: parts.length
      }
    })
}

export function formatFlow(value) {
  const size = Number(value || 0)
  if (size === 0) return '0 B'
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(2)} KB`
  if (size < 1024 * 1024 * 1024) return `${(size / (1024 * 1024)).toFixed(2)} MB`
  return `${(size / (1024 * 1024 * 1024)).toFixed(2)} GB`
}

// The quality word of a diagnosis result (same thresholds as flux-panel).
export function qualityKey(averageTime, packetLoss) {
  if (averageTime == null || packetLoss == null) return 'unknown'
  if (averageTime < 30 && packetLoss === 0) return 'excellent'
  if (averageTime < 50 && packetLoss === 0) return 'veryGood'
  if (averageTime < 100 && packetLoss < 1) return 'good'
  if (averageTime < 150 && packetLoss < 2) return 'fair'
  if (averageTime < 200 && packetLoss < 5) return 'poor'
  return 'veryPoor'
}

// Status, runtime and strategy words with a UiBadge tone.
export function createForwardPresenters(t, translateLiteral) {
  function statusMeta(status) {
    switch (Number(status)) {
      case 1:
        return { text: t('runtime.forward.status.normal'), tone: 'success' }
      case 0:
        return { text: t('runtime.forward.status.paused'), tone: 'warning' }
      case -1:
        return { text: t('runtime.forward.status.error'), tone: 'danger' }
      default:
        return { text: t('runtime.forward.status.unknown'), tone: 'neutral' }
    }
  }

  function runtimeMeta(forward) {
    if (!forward?.hasTrackedRuntime) {
      return null
    }
    switch (Number(forward.runtimeStatus)) {
      case 0:
        return { text: t('runtime.forward.runtimeStatus.pending'), tone: 'warning' }
      case 1:
        return { text: t('runtime.forward.runtimeStatus.running'), tone: 'info' }
      case 2:
        return {
          text: forward.runtimeBackend === 'gost' ? t('runtime.forward.runtimeStatus.synced') : t('runtime.forward.runtimeStatus.applied'),
          tone: 'success'
        }
      case 3:
        return { text: t('runtime.forward.runtimeStatus.failed'), tone: 'danger' }
      default:
        return null
    }
  }

  function runtimeSummary(forward) {
    if (!forward?.hasTrackedRuntime) {
      return ''
    }
    if (forward.runtimeMessage) {
      return translateLiteral(forward.runtimeMessage) || forward.runtimeMessage
    }
    switch (Number(forward.runtimeStatus)) {
      case 0:
        return t('runtime.forward.runtimeStatus.queuedSummary')
      case 1:
        return t('runtime.forward.runtimeStatus.runningSummary')
      default:
        return ''
    }
  }

  function strategyText(strategy) {
    return ['fifo', 'round', 'rand', 'hash'].includes(strategy)
      ? t(`runtime.forward.strategy.${strategy}`)
      : t('runtime.forward.strategy.unknown')
  }

  return { statusMeta, runtimeMeta, runtimeSummary, strategyText }
}
