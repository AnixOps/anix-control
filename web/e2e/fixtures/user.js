import { FIXED_NOW_MS } from './clock.js'
const GIB = 1024 ** 3
const NOW = Math.floor(FIXED_NOW_MS / 1000)
const ok = data => ({ code: 0, msg: 'ok', data })

const SUMMARY = {
  user_id: 7, email: 'lin.xiao@example.com', plan_id: 3, plan_name: '标准', transfer_enable: 200 * GIB, used_traffic: 71.6 * GIB,
  upload_traffic: 3.2 * GIB, download_traffic: 68.4 * GIB, expired_at: Date.UTC(2026, 10, 30, 12) / 1000, is_expired: false,
  days_remaining: 59, usage_percent: 35.8, subscribe_path: '/s', cached_at: new Date(FIXED_NOW_MS - 90e3).toISOString()
}
const ARTICLES = [
  { id: 4, category: '公告', title: '10 月 8 日凌晨 2:00–3:00 维护香港节点', body: '维护期间自动切换到东京节点，无需操作。', updated_at: NOW - 2 * 86400 },
  { id: 5, category: '公告', title: '新增新加坡 2 号节点', body: '重新导入订阅即可看到。', updated_at: NOW - 10 * 86400 },
  { id: 6, category: '客户端', title: 'Shadowrocket 导入失败怎么办', body: '# 先检查订阅\n确认订阅链接可以在浏览器中打开，并且**没有过期**。\n\n## 重新导入\n1. 打开 Shadowrocket，删除旧的订阅。\n2. 回到「订阅」页，点「一键导入」。\n3. 在 Shadowrocket 中点「更新」。\n\n## 仍然失败\n- 检查手机时间是否准确\n- 切换 Wi-Fi 与蜂窝网络后重试\n- 提交工单，附上截图\n\n> 订阅链接等同于密码，请勿发给他人。\n\n```\nhttps://panel.example.com/s/••••\n```', updated_at: NOW - 30 * 86400 },
  { id: 7, category: '客户端', title: '在 Windows 上使用 Clash Verge', body: '下载 Clash Verge，导入订阅后选择「规则」模式。', updated_at: NOW - 40 * 86400 },
  { id: 8, category: '客户端', title: '在 Android 上使用 sing-box', body: '安装 sing-box，点「一键导入」。', updated_at: NOW - 41 * 86400 },
  { id: 9, category: '账户', title: '开启两步验证', body: '打开「账户」，点「开启两步验证」。', updated_at: NOW - 50 * 86400 },
  { id: 10, category: '常见问题', title: '为什么晚高峰速度变慢', body: '晚高峰时段国际出口拥堵，可以切换到其他节点。', updated_at: NOW - 60 * 86400 },
]
const TICKETS = [
  { id: 2041, subject: '晚高峰东京节点速度慢', status: 1, level: 1, updated_at: NOW - 2 * 3600 },
  { id: 1987, subject: '如何在路由器上使用订阅', status: 2, level: 0, updated_at: NOW - 14 * 86400 },
  { id: 1960, subject: '续费后流量没有更新', status: 0, level: 2, updated_at: NOW - 20 * 86400 },
]
const TICKET = { ...TICKETS[0], messages: [
  { id: 1, is_admin: 0, message: '这两天晚上 9 点到 11 点，东京 2 号节点很慢，测速只有 2 Mbps。', created_at: new Date((NOW - 26 * 3600) * 1000).toISOString() },
  { id: 2, is_admin: 1, message: '你好，我们已经调整了晚高峰的路由。请重新导入订阅后再试一次，如果仍然慢，请告诉我们你所在的城市和运营商。', created_at: new Date((NOW - 3 * 3600) * 1000).toISOString() },
  { id: 3, is_admin: 0, message: '好的，我今晚再试。', created_at: new Date((NOW - 2 * 3600) * 1000).toISOString() },
] }
const PLANS = [
  { id: 1, name: '轻量', transfer_enable: 100, device_limit: 2, month_price: 1500, quarter_price: 4200, year_price: 15000, content: '全部地区节点\n适合日常浏览' },
  { id: 2, name: '标准', transfer_enable: 200, speed_limit: 500, device_limit: 3, month_price: 2500, quarter_price: 7000, year_price: 25000, content: '全部地区节点\n晚高峰优先线路\n工单 24 小时内回复' },
  { id: 3, name: '专业', transfer_enable: 1000, speed_limit: 1000, device_limit: 6, month_price: 6800, year_price: 68000, content: '全部地区节点\n专属线路\n工单 2 小时内回复' },
]
const ORDERS = [
  { id: 31, trade_no: '2026100112000031', plan: { id: 2, name: '标准' }, period: 'quarter', total_amount: 7000, status: 0, created_at: NOW - 3600 },
  { id: 22, trade_no: '2026070112000022', plan: { id: 2, name: '标准' }, period: 'quarter', total_amount: 6300, discount_amount: 700, status: 3, created_at: NOW - 92 * 86400, paid_at: NOW - 92 * 86400 + 300 },
  { id: 15, trade_no: '2026040112000015', plan: { id: 1, name: '轻量' }, period: 'month', total_amount: 1500, status: 2, created_at: NOW - 183 * 86400 },
]

export function baseApi(p, method, { commercial = false } = {}) {
  if (p.endsWith('/public/config')) return { edition: commercial ? 'commercial' : 'community', hidden_packages: commercial ? [] : ['affiliate', 'order', 'payment'], registration: { enabled: true, require_invite: true } }
  if (p.endsWith('/user/profile')) return ok({ id: 7, email: 'lin.xiao@example.com', token: '7f3k9q2m8x4v2c6b', is_admin: false })
  if (p.endsWith('/user/subscription')) return ok(SUMMARY)
  if (p.endsWith('/user/knowledge')) return ok(ARTICLES)
  if (p.endsWith('/user/ticket') && method === 'GET') return ok(TICKETS)
  if (/\/user\/ticket\/\d+$/.test(p)) return ok(TICKET)
  if (p.endsWith('/user/mfa/status')) return ok({ enabled: false, has_backup_codes: false, remaining_codes: 0 })
  if (p.endsWith('/user/mfa/totp/setup')) return ok({ secret: 'JBSWY3DPEHPK3PXP', url: 'otpauth://totp/AnixOps:lin.xiao%40example.com?secret=JBSWY3DPEHPK3PXP&issuer=AnixOps', backup_codes: [] })
  if (p.endsWith('/user/plan')) return ok(PLANS)
  if (p.endsWith('/user/order')) return ok({ list: ORDERS, total: 3 })
  if (/\/user\/order\/\d+$/.test(p)) return ok(ORDERS[1])
  if (p.endsWith('/s/7f3k9q2m8x4v2c6b')) return 'x'
  return ok([])
}


const paths = {
  login: '/login', home: '/user/dashboard', subscribe: '/user/subscribe', help: '/user/knowledge', article: '/user/knowledge?article=6',
  tickets: '/user/tickets', ticket: '/user/tickets?ticket=2041', account: '/user/account', plans: '/user/plans', orders: '/user/orders'
}
const noop = async () => {}
export default {
  user: true,
  signedOut: s => s === 'login',
  pathFor: s => paths[s],
  api(p, { method, scenario }) {
    if (p.startsWith('/s/')) return { __text: 'vless://uuid@hk-01.example.com:443#HK 01' }
    return baseApi(p, method, { commercial: scenario === 'plans' || scenario === 'orders' })
  },
  scenarios: Object.fromEntries(Object.keys(paths).map(k => [k, noop]))
}
