import { FIXED_NOW_MS } from './clock.js'
const GIB = 1024 ** 3
const now = Math.floor(FIXED_NOW_MS / 1000)
const NAMES = [
  ['hk-01', 'hk1.edge.example.net', 'HK,IEPL'], ['hk-02', 'hk2.edge.example.net', 'HK'], ['jp-tokyo-01', 'tyo1.edge.example.net', 'JP,Premium'],
  ['sg-01', '203.0.113.24', 'SG'], ['us-lax-01', 'lax1.edge.example.net', 'US'], ['de-fra-01', 'fra1.edge.example.net', 'DE'],
  ['kr-sel-01', 'sel1.edge.example.net', ''], ['tw-01', '198.51.100.7', 'TW,IEPL'], ['uk-lon-01', 'lon1.edge.example.net', 'UK'],
  ['relay-sh-01', 'sh-relay.example.net', 'Relay'], ['relay-gz-01', 'gz-relay.example.net', 'Relay'], ['au-syd-01', 'syd1.edge.example.net', '']
]
export const NODES = NAMES.map(([name, host, tags], index) => {
  const status = index === 6 ? 3 : index === 11 ? 0 : index === 4 ? 2 : 1
  const online = status === 1
  return {
    id: 101 + index,
    name,
    host,
    port: 443,
    tags: tags || null,
    status,
    parent_id: index >= 9 && index <= 10 ? 101 + (index - 9) : null,
    rate: index % 4 === 0 ? 1.5 : 1,
    sort: index,
    show: 1,
    auto_register: index % 2,
    monthly_limit: index % 3 === 0 ? 2000 * GIB : null,
    monthly_upload: (index * 97 % 900) * GIB,
    monthly_download: (index * 131 % 1400) * GIB,
    monthly_reset_day: 1,
    server_ip: `203.0.113.${10 + index}`,
    server_version: status === 0 ? null : (index % 3 ? 'v1.4.2' : 'v1.5.0-rc.1'),
    server_os: status === 0 ? null : 'Debian GNU/Linux 12 (bookworm)',
    cpu_usage: online ? (index * 13) % 80 + 4 : 0,
    memory_usage: online ? (index * 17) % 70 + 20 : 0,
    disk_usage: online ? (index * 7) % 60 + 10 : 0,
    uptime: online ? 86400 * (index + 3) + 3600 * index : 0,
    online_users: online ? (index * 11) % 90 : 0,
    runtime_healthy: index !== 7,
    runtime_error: index === 7 ? 'gost relay exited: listen udp :8421: address already in use' : '',
    runtime_checked_at: index % 2 === 1 ? now - 120 : null,
    total_upload: (index * 311 % 4000) * GIB,
    total_download: (index * 523 % 9000) * GIB,
    last_check_at: status === 0 ? null : (online ? now - (index * 23) % 240 : now - 3600 * (index + 2)),
    created_at: new Date(Date.UTC(2026, 3, 1 + index)).toISOString(),
    updated_at: new Date(Date.UTC(2026, 8, 1 + index)).toISOString(),
    protocols: Array.from({ length: (index % 4) + 1 }, (_, i) => ({ id: index * 10 + i }))
  }
})

export const PROTOCOLS = [
  { id: 1, type: 'vless', port: 443, tls: 2, transport: 'tcp', enable: 1, show: 1, settings: '{"flow":"xtls-rprx-vision"}', reality_settings: '{"private_key":"********","public_key":"pPk2","short_id":"ab"}' },
  { id: 2, type: 'hysteria2', port: 8443, tls: 1, transport: 'quic', enable: 1, show: 1, settings: '{"password":"********"}' },
  { id: 3, type: 'trojan', port: 2053, tls: 1, transport: 'ws', enable: 0, show: 0, settings: '{}' },
  { id: 4, type: 'wireguard', port: 51820, tls: 0, transport: 'udp', enable: 1, show: 1, settings: '{"cidr":"10.66.0.0/24","relay":{"role":"entry","mode":"relay+quic","server":"203.0.113.80","server_port":8443}}' }
]

export const TEMPLATES = [
  { name: 'VLESS Reality', type: 'vless', default_port: 443, tls: 2, transport: 'tcp', description: 'VLESS + XTLS Vision over Reality', settings: '{"flow":"xtls-rprx-vision"}' },
  { name: 'Hysteria2', type: 'hysteria2', default_port: 8443, tls: 1, transport: 'quic', description: 'UDP, fast on lossy links' },
  { name: 'Trojan WS', type: 'trojan', default_port: 2053, tls: 1, transport: 'ws', description: 'Trojan over WebSocket + TLS' },
  { name: 'Shadowsocks 2022', type: 'shadowsocks', default_port: 8388, tls: 0, transport: 'tcp', description: '2022-blake3-aes-128-gcm' },
  { name: 'WireGuard relay', type: 'wireguard', default_port: 51820, tls: 0, transport: 'udp', description: 'Entry/exit relay over GOST' }
]

const LEVELS = ['info', 'info', 'warning', 'error', 'debug', 'info']
const SOURCES = ['xray', 'agent', 'gost', 'agent.control', 'xray', 'sync']
const MESSAGES = [
  'core started: 3 inbounds, 1 outbound',
  'config pushed by control plane (revision 42)',
  'heartbeat latency 812ms above threshold 500ms',
  'listen udp :8421: address already in use',
  'reloading routing rules',
  'node.sync accepted: 4 protocols'
]
export const LOGS = Array.from({ length: 20 }, (_, index) => ({
  id: 900 - index,
  level: LEVELS[index % LEVELS.length],
  source: SOURCES[index % SOURCES.length],
  message: MESSAGES[index % MESSAGES.length],
  trace_id: index % 3 === 0 ? `7f3a${index}c0de91b2` : '',
  fields_json: index % 4 === 0 ? JSON.stringify({ inbound: 'vless-443', users: 37 }) : '',
  created_at: now - index * 420
}))

export function envelope(data) {
  return { code: 0, msg: 'ok', data }
}
