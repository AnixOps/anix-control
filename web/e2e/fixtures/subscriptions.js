import { FIXED_NOW_MS } from './clock.js'

const GROUPS = [
  { id: 1, name: '亚洲', description: '香港、日本、新加坡节点', priority: 10, enable: 1 },
  { id: 2, name: '欧美', description: '美国与德国', priority: 5, enable: 1 },
  { id: 3, name: '内测', description: '', priority: 0, enable: 0 }
]
const STATS = [
  { group_id: 1, group_name: '亚洲', user_count: 128, enabled_users: 117, template_count: 6, protocol_count: 9, online_nodes: 7, total_traffic: 3.2e12, plan_count: 3 },
  { group_id: 2, group_name: '欧美', user_count: 64, enabled_users: 60, template_count: 3, protocol_count: 4, online_nodes: 2, total_traffic: 8.4e11, plan_count: 2 },
  { group_id: 3, group_name: '内测', user_count: 4, enabled_users: 4, template_count: 1, protocol_count: 0, online_nodes: 0, total_traffic: 1.2e9, plan_count: 1 }
]
const TEMPLATES = [
  { id: 11, name: '香港 01', type: 'vless', server: 'hk-01.example.com', port: 443, tls: 2, transport: 'tcp', enable: 1, reality_public_key: 'abc', server_name: 'www.example.com' },
  { id: 12, name: '日本 02', type: 'trojan', server: 'jp-02.example.com', port: 8443, tls: 1, transport: 'ws', enable: 1 },
  { id: 13, name: '新加坡 03', type: 'hysteria2', server: 'sg-03.example.com', port: 443, tls: 1, transport: 'quic', enable: 0 }
]
const PROTOCOLS = [
  { id: 21, node_id: 5, node: { name: 'hk-hkg-01' }, type: 'vless', name: 'VLESS Reality', port: 443, show: true, enable: true },
  { id: 22, node_id: 6, node: { name: 'jp-tyo-02' }, type: 'trojan', name: 'Trojan WS', port: 8443, show: true, enable: false }
]
const AVAILABLE = [...PROTOCOLS.map(p => ({ ...p, subscription_groups: [{ id: 1, name: '亚洲' }] })), { id: 23, node_id: 7, node: { name: 'sg-sin-03' }, type: 'hysteria2', name: 'Hy2', port: 443, subscription_groups: [] }]
const CLASH = `proxies:
  - name: 香港 01
    type: vless
    server: hk-01.example.com
    port: 443
    uuid: 00000000-0000-0000-0000-000000000000
    tls: true
    servername: www.example.com
    reality-opts:
      public-key: abc
  - name: 日本 02
    type: trojan
    server: jp-02.example.com
    port: 8443
    password: 00000000-0000-0000-0000-000000000000
    network: ws
proxy-groups:
  - name: Proxy
    type: select
    proxies: [香港 01, 日本 02]`
// GET /api/v4/admin/subscription-groups/1/members: the users granted the group
// directly, newest grant first (no credential in any field).
const MEMBER_NAMES = ['lin.xiao', 'wang.fang', 'chen.jie', 'zhao.lei', 'sun.li', 'zhou.min', 'wu.hao', 'zheng.yu', 'feng.yi', 'he.ming', 'luo.qi', 'gao.yan', 'xu.ning', 'ma.chao', 'tang.yu', 'han.mei', 'liu.bo', 'deng.xin', 'yan.ru', 'shi.kai', 'jiang.lu', 'fan.hui']
const MEMBER_DAY = 86400
const MEMBERS = MEMBER_NAMES.map((name, index) => {
  const expired = index % 5 === 3
  const forever = index % 4 === 1
  return {
    user_id: 300 + index,
    email: `${name}@example.com`,
    banned: index === 6 ? 1 : 0,
    plan_id: index % 3 === 0 ? null : 1 + (index % 3),
    expire_at: forever ? null : Math.floor(FIXED_NOW_MS / 1000) + (expired ? -MEMBER_DAY * (3 + index) : MEMBER_DAY * (30 + index * 3)),
    transfer_enable: index % 6 === 2 ? 500 * 1024 ** 3 : null,
    next_renew_price: index % 4 === 0 ? 3000 : null,
    created_at: new Date(FIXED_NOW_MS - index * 36 * 3600 * 1000).toISOString(),
    active: !expired
  }
})

const PATHS = {
  list: '/admin/subscriptions', empty: '/admin/subscriptions', error: '/admin/subscriptions', groupDialog: '/admin/subscriptions', links: '/admin/subscriptions',
  overview: '/admin/subscriptions/1', templates: '/admin/subscriptions/1/templates', templateDialog: '/admin/subscriptions/1/templates',
  protocols: '/admin/subscriptions/1/protocols', protocolsDialog: '/admin/subscriptions/1/protocols', members: '/admin/subscriptions/1/members',
  output: '/admin/subscriptions/1/output', outputError: '/admin/subscriptions/1/output', notFound: '/admin/subscriptions/99', templatesEmpty: '/admin/subscriptions/3/templates'
}
export default {
  pathFor: scenario => PATHS[scenario],
  api(path, { scenario, query }) {
    if (/^\/api\/v4\/admin\/subscription-groups\/\d+\/members$/.test(path)) {
      let list = MEMBERS
      if (query.q) list = list.filter(member => member.email.includes(query.q))
      if (query.status === 'active') list = list.filter(member => member.active)
      if (query.status === 'expired') list = list.filter(member => !member.active)
      const size = Number(query.page_size) || 20
      const page = Number(query.page) || 1
      return { data: { total: list.length, page, page_size: size, members: list.slice((page - 1) * size, page * size) } }
    }
    if (path === '/api/v2/admin/subscription/groups') {
      if (scenario === 'error') return { code: 500, msg: '订阅服务没有响应', data: null }
      return { code: 0, data: scenario === 'empty' ? [] : GROUPS }
    }
    if (path === '/api/v2/admin/subscription/stats') return { code: 0, data: scenario === 'empty' ? [] : STATS }
    if (/groups\/\d+\/templates$/.test(path)) return { code: 0, data: scenario === 'templatesEmpty' ? [] : TEMPLATES }
    if (/groups\/\d+\/protocols$/.test(path)) return { code: 0, data: PROTOCOLS }
    if (path === '/api/v2/admin/subscription/protocols/available') return { code: 0, data: AVAILABLE }
    if (path === '/api/v2/admin/subscription/preview') {
      if (scenario === 'outputError') return { code: 500, msg: '没有可输出的节点', data: null }
      return { code: 0, data: { content: CLASH } }
    }
    if (path === '/api/v2/user/profile') return { code: 0, data: { id: 1, email: 'admin@example.com', is_admin: true, token: 'f3k9q2m8x4v7' } }
    return undefined
  },
  viewportOnly: ['groupDialog', 'links', 'templateDialog', 'protocolsDialog'],
  scenarios: {
    list: async () => {},
    empty: async () => {},
    error: async () => {},
    groupDialog: async (page) => {
      await page.getByRole('button', { name: '新建分组' }).first().click()
      await page.getByRole('dialog').getByRole('button', { name: '保存' }).click()
    },
    links: async (page) => {
      await page.getByRole('button', { name: '亚洲 的操作' }).first().click()
      await page.getByRole('menuitem', { name: '订阅链接' }).click()
    },
    overview: async () => {},
    templates: async () => {},
    templatesEmpty: async () => {},
    templateDialog: async (page) => {
      await page.getByRole('button', { name: '添加节点模板' }).first().click()
      await page.getByRole('dialog').getByRole('button', { name: '保存' }).click()
    },
    protocols: async () => {},
    protocolsDialog: async (page) => { await page.getByRole('button', { name: '管理关联' }).first().click() },
    members: async () => {},
    output: async () => {},
    outputError: async () => {},
    notFound: async () => {}
  }
}
