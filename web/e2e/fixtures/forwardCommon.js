export function systemConfig(path, nodex = false) {
  const m = path.match(/^\/api\/v2\/admin\/system\/configs\/(.+)$/)
  if (!m) return undefined
  const key = decodeURIComponent(m[1])
  if (key === 'forward.runtime.nodex_mode') return { code: 0, data: { value: nodex ? 'true' : 'false' } }
  if (key === 'forward.runtime_backend') return { code: 0, data: { value: nodex ? 'gost' : 'nftables_ansible' } }
  if (key === 'forward.runtime.nodex.base_url') return { code: 0, data: { value: '' } }
  if (key === 'forward.runtime.nodex.token') return { code: 0, data: { value: '' } }
  return { code: 0, data: { value: '' } }
}
export const TUNNELS = [
  { id: 1, name: 'hk-port-01', type: 1, inNodeId: 0, outNodeId: 11, inIp: '203.0.113.10', outIp: '203.0.113.10', flow: 1, trafficRatio: 1, status: 1, inNodePortSta: 10000, inNodePortEnd: 20000, tcpListenAddr: '[::]', udpListenAddr: '[::]' },
  { id: 2, name: 'jp-port-02', type: 1, inNodeId: 0, outNodeId: 12, inIp: '198.51.100.24,198.51.100.25', outIp: '198.51.100.24', flow: 2, trafficRatio: 1.5, status: 1, inNodePortSta: 20000, inNodePortEnd: 30000, tcpListenAddr: '[::]', udpListenAddr: '[::]' },
  { id: 3, name: 'sg-tls-tunnel', type: 2, inNodeId: 21, outNodeId: 22, inIp: '192.0.2.7', outIp: '192.0.2.8', flow: 1, trafficRatio: 2, status: 0, protocol: 'tls', tcpListenAddr: '[::]', udpListenAddr: '[::]' }
]
