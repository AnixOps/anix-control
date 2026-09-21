<template>
  <main class="maintenance-page">
    <header class="page-header">
      <div><p class="eyebrow">AnixOps · Operations</p><h1>运维工作台</h1><p>机器监控故障、自愈记录、人工处理与版本审批。</p></div>
      <div class="actions"><router-link to="/user/dashboard">返回工作区</router-link><button class="btn" :disabled="busy" @click="reload">刷新</button></div>
    </header>
    <p v-if="error" class="alert-error" role="alert">{{ error }}</p>
    <p v-if="message" class="alert-success" role="status">{{ message }}</p>
    <p v-if="loading">正在加载运维数据…</p>
    <template v-if="loaded">
      <nav class="tabs" aria-label="运维栏目">
        <button v-if="role !== 'bootstrap'" class="btn" :class="{ active: tab === 'tickets' }" @click="tab = 'tickets'">故障工单 {{ tickets.length }}</button>
        <button v-if="role !== 'bootstrap'" class="btn" :class="{ active: tab === 'changes' }" @click="tab = 'changes'">插件操作与审批</button>
        <button class="btn" :class="{ active: tab === 'settings' }" @click="tab = 'settings'">运维设置</button>
      </nav>
      <section v-if="tab === 'tickets'" class="ticket-grid">
        <div class="panel">
          <div class="section-heading"><h2>故障工单</h2><label>状态 <select v-model="filter"><option value="">全部</option><option value="open">未恢复</option><option value="recovered">已恢复</option><option value="closed">已关闭</option></select></label></div>
          <p v-if="!filteredTickets.length" class="muted">当前没有符合条件的运维故障。</p>
          <button v-for="ticket in filteredTickets" :key="ticket.id" class="ticket-row" :class="{ selected: detail?.ticket.id === ticket.id }" @click="openTicket(ticket.id)">
            <span><strong>#{{ ticket.id }} {{ ticket.title }}</strong><small>节点 {{ ticket.node_id }} · {{ ticket.plugin_id }} {{ ticket.plugin_version }}</small></span>
            <span class="badge">{{ statusName(ticket.status) }} · {{ ticket.severity }}</span>
            <small>首次失败 {{ date(ticket.first_failed_at) }} · {{ ticket.claimed_by ? `处理人 #${ticket.claimed_by}` : '待认领' }}</small>
          </button>
        </div>
        <article v-if="detail" class="panel detail" aria-label="故障详情">
          <h2>#{{ detail.ticket.id }} {{ detail.ticket.title }}</h2>
          <dl><dt>节点 / 实例</dt><dd>{{ detail.ticket.node_id }} / {{ detail.ticket.instance_id }}</dd><dt>插件 / Agent / 配置版本</dt><dd>{{ detail.ticket.plugin_version }} / {{ detail.ticket.agent_version || '—' }} / {{ detail.ticket.config_version || '—' }}</dd><dt>首次失败</dt><dd>{{ date(detail.ticket.first_failed_at) }}</dd><dt>恢复时间</dt><dd>{{ date(detail.ticket.recovered_at) }}</dd><dt>处理人</dt><dd>{{ detail.ticket.claimed_by || '待认领' }}</dd><dt>历史工单</dt><dd><button v-if="detail.ticket.previous_ticket_id" class="link" @click="openTicket(detail.ticket.previous_ticket_id)">#{{ detail.ticket.previous_ticket_id }}</button><span v-else>—</span></dd></dl>
          <div class="actions"><button v-if="!detail.ticket.claimed_by && detail.ticket.status !== 'closed'" class="btn btn-primary" :disabled="busy" @click="claim">认领工单</button><button class="btn" :disabled="busy || detail.ticket.status === 'closed'" @click="prepareRepair('restart')">重启此节点插件</button><button class="btn" :disabled="busy || detail.ticket.status === 'closed'" @click="prepareRepair('rollback')">恢复已验证版本</button></div>
          <h3>诊断时间线</h3>
          <ol class="timeline"><li v-for="event in detail.events || []" :key="event.event_id"><strong>{{ event.error_code }} · {{ event.status }}</strong><p>{{ date(event.occurred_at) }} · 连续失败 {{ event.consecutive_failures || 0 }} 次</p><p>自愈：{{ event.self_heal_action || 'none' }} / {{ event.self_heal_result || 'not_attempted' }}</p></li></ol>
          <h3>处理记录</h3><ol class="timeline"><li v-for="record in detail.records || []" :key="record.id"><strong>{{ record.kind }} · #{{ record.actor_id }}</strong><p>{{ record.note }}</p><small>{{ date(record.created_at) }}</small></li></ol>
          <form v-if="detail.ticket.status !== 'closed'" @submit.prevent="saveNote"><label>处理记录<textarea v-model="note" maxlength="2000" required rows="3" placeholder="记录诊断、操作和恢复验证结果"></textarea></label><div class="actions"><button class="btn" :disabled="busy || !note.trim()">保存记录</button><button type="button" class="btn" :disabled="busy || detail.ticket.status !== 'recovered' || !note.trim()" @click="close">填写记录并关闭</button></div><p class="muted">持续健康 5 分钟后可标记恢复；关闭必须填写处理记录。</p></form>
          <h3>通知投递</h3><div class="table-scroll"><table><thead><tr><th>渠道 / 接收人</th><th>原因</th><th>结果</th></tr></thead><tbody><tr v-for="delivery in detail.deliveries || []" :key="delivery.id"><td>{{ delivery.channel }} / #{{ delivery.recipient_id }}</td><td>{{ delivery.reason }}</td><td>{{ delivery.status }} · {{ delivery.attempts }} 次 <span v-if="delivery.last_error">({{ delivery.last_error }})</span></td></tr></tbody></table></div>
        </article>
        <div v-else class="panel muted">选择工单查看节点版本、处理时间线和投递结果。</div>
      </section>
      <section v-if="tab === 'changes'" class="panel">
        <h2>机器监控插件操作</h2><p>技术员可重启单节点、恢复该节点曾成功运行的版本。其他操作及批量变更需负责人批准，批准绑定节点、动作、版本与配置。</p>
        <form class="change-form" @submit.prevent="submitChange">
          <label>选择节点（可多选，最多 50 个）<select v-model="selectedNodeIDs" multiple required size="5"><option v-for="node in catalog.nodes" :key="node.id" :value="node.id">#{{ node.id }} {{ node.name }} · 当前 {{ node.active_version || '未运行' }} / 目标 {{ node.desired_version || '未安装' }}</option></select></label>
          <button v-if="catalog.next_node_id" type="button" class="btn" :disabled="busy" @click="loadMoreNodes">载入更多节点</button>
          <label>操作<select v-model="change.kind"><option value="restart">重启</option><option value="rollback">人工恢复已验证版本</option><option value="install">安装并启动</option><option value="enable">启动</option><option value="disable">停止</option><option value="update">升级并启动</option><option value="configure">修改插件配置</option></select></label>
          <label>具体版本<select v-model="change.target_version" required :disabled="!selectedNodeIDs.length"><option value="" disabled>选择验证通过的版本</option><option v-for="version in eligibleVersions" :key="version" :value="version">{{ version }}</option></select></label>
          <p v-if="selectedNodeIDs.length && !eligibleVersions.length" class="muted">所选节点没有共同可用的版本。人工恢复要求该节点已记录健康状态及匹配的历史配置。</p>
          <label v-if="['install', 'update', 'configure'].includes(change.kind)">配置 JSON<textarea v-model="change.config" rows="3" spellcheck="false" required></textarea></label>
          <button class="btn btn-primary" :disabled="busy || !change.target_version || !selectedNodeIDs.length">提交操作请求</button>
        </form>
        <p v-if="!catalog.nodes.length" class="muted">尚无机器监控节点。请由负责人在 Control 插件目录登记签名包，并为节点建立 machine-telemetry 分配。</p>
        <p class="muted">签名发布包在 Control 插件目录登记；安装会验证签名与包摘要。操作排队后，运行结果由 Agent 回报。</p>
        <article v-for="item in changes" :key="item.change.id" class="change-card">
          <div class="section-heading"><h3>#{{ item.change.id }} {{ item.change.kind }} · {{ item.change.target_version }}</h3><span class="badge">{{ item.change.status }}</span></div>
          <p>节点 {{ item.change.node_ids_json }} · {{ item.change.plugin_id }} · 请求人 #{{ item.change.requested_by }}<span v-if="item.change.approved_by"> · 批准人 #{{ item.change.approved_by }}</span></p>
          <details><summary>查看本次具体操作与审计</summary><p class="hash">配置摘要：{{ item.change.config_hash }}</p><p class="hash">审批绑定：{{ item.change.binding_hash }}</p><pre>{{ JSON.stringify(item.config || {}, null, 2) }}</pre><p class="hash">执行任务：{{ item.change.operation_ids_json || '尚未排队' }}</p><ol><li v-for="entry in item.audit || []" :key="entry.id">{{ date(entry.created_at) }} · #{{ entry.actor_id }} {{ entry.action }}</li></ol></details>
          <ul v-if="item.operations?.length" class="timeline"><li v-for="operation in item.operations" :key="operation.id">节点 {{ operation.node_id }} · {{ operation.kind }} {{ operation.target_version }} · {{ operation.state }} · {{ date(operation.updated_at) }}</li></ul>
          <div class="actions"><button v-if="isOwner && item.change.status === 'pending'" class="btn" :disabled="busy" @click="approve(item.change)">批准此具体操作</button><button v-if="item.change.status === 'approved'" class="btn btn-primary" :disabled="busy" @click="execute(item.change)">执行已批准操作</button></div>
        </article>
      </section>
      <section v-if="tab === 'settings'" class="panel">
        <h2>运维负责人及通知设置</h2><p>普通故障通知技术员；没有技术员时由负责人接单。重大故障与超时升级同时通知负责人。</p>
        <form @submit.prevent="saveSettings"><fieldset :disabled="!canConfigure || busy"><div class="settings-grid"><label>负责人用户 ID<input v-model.number="settings.owner_id" type="number" min="1" required></label><label>技术员用户 ID（逗号分隔）<input v-model="technicianIDs" placeholder="2, 3"></label></div>
          <h3>接收渠道</h3><div v-for="(contact, index) in settings.contacts" :key="index" class="contact-row"><label>用户 ID<input v-model.number="contact.user_id" type="number" min="1" required></label><label>邮件地址<input v-model="contact.email" type="email" maxlength="320"></label><label>Telegram chat ID<input v-model="contact.telegram_chat_id" maxlength="128"></label><button type="button" class="btn" @click="settings.contacts.splice(index, 1)">移除</button></div><button type="button" class="btn" @click="settings.contacts.push({user_id: 0, email: '', telegram_chat_id: ''})">添加接收人</button>
          <label class="enable-switch"><input v-model="settings.enabled" type="checkbox">正式启用运维通知</label><p class="muted">负责人邮件和 Telegram 均需验证后才能正式启用。更换负责人或渠道会清除对应验证结果。</p><button class="btn btn-primary">保存设置</button></fieldset></form>
        <h3>负责人渠道验证</h3><div v-for="channel in channels" :key="channel.id" class="verification"><strong>{{ channel.label }}</strong><span>{{ settings[channel.verifiedField] ? `已验证 · ${date(settings[channel.verifiedField])}` : '未验证' }}</span><template v-if="canConfigure"><button class="btn" :disabled="busy" @click="verify(channel.id)">发送验证码</button><label>收到的验证码<input v-model="codes[channel.id]" maxlength="32" autocomplete="one-time-code"></label><button class="btn" :disabled="busy || !codes[channel.id]" @click="confirm(channel.id)">确认验证码</button></template></div>
      </section>
    </template>
  </main>
</template>

<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import * as api from '@/api/maintenance'

const loading = ref(true), loaded = ref(false), busy = ref(false), error = ref(''), message = ref(''), tab = ref('tickets')
const role = ref(''), tickets = ref([]), changes = ref([]), detail = ref(null), filter = ref(''), note = ref('')
const settings = ref({ owner_id: 0, technician_ids: [], contacts: [], enabled: false }), technicianIDs = ref('')
const codes = reactive({ email: '', telegram: '' })
const channels = [{ id: 'email', label: '邮件', verifiedField: 'owner_email_verified_at' }, { id: 'telegram', label: 'Telegram', verifiedField: 'owner_telegram_verified_at' }]
const change = reactive({ nodeIDs: '', kind: 'restart', target_version: '', config: '{}' })
const catalog = ref({ nodes: [], releases: [], next_node_id: 0 })
const selectedNodeIDs = computed({ get: () => ids(change.nodeIDs), set: values => { change.nodeIDs = values.join(', ') } })
const eligibleVersions = computed(() => {
  const nodes = catalog.value.nodes.filter(node => selectedNodeIDs.value.includes(node.id))
  if (!nodes.length) return []
  if (['install', 'update'].includes(change.kind)) return catalog.value.releases.map(release => release.version)
  const choices = node => change.kind === 'rollback' ? node.restore_versions || [] : node.active_version && node.active_version === node.desired_version ? [node.active_version] : []
  return choices(nodes[0]).filter(version => nodes.every(node => choices(node).includes(version)))
})
watch(eligibleVersions, versions => {
  if (!versions.includes(change.target_version)) change.target_version = versions.length === 1 && change.kind !== 'rollback' ? versions[0] : ''
})
const isOwner = computed(() => role.value === 'owner')
const canConfigure = computed(() => isOwner.value || role.value === 'bootstrap' || role.value === 'admin')
const filteredTickets = computed(() => tickets.value.filter(ticket => !filter.value || ticket.status === filter.value))
const date = value => value ? new Date(value).toLocaleString() : '—'
const statusName = value => ({ open: '未恢复', recovered: '已恢复', closed: '已关闭' }[value] || value)
function failure(err) { return err?.response?.data?.error?.message || err?.response?.data?.message || (typeof err?.response?.data?.error === 'string' ? err.response.data.error : '') || err?.message || '操作失败，请稍后重试。' }
async function perform(action, success) {
  if (busy.value) return
  busy.value = true; error.value = ''; message.value = ''
  try { await action(); message.value = success || '' } catch (err) { error.value = failure(err) } finally { busy.value = false }
}
async function reload() {
  await perform(async () => {
    const current = await api.getMaintenanceRole()
    role.value = typeof current === 'string' ? current : current.role
    const bootstrap = role.value === 'bootstrap'
    const results = await Promise.all([api.getMaintenanceSettings(), bootstrap ? [] : api.getMaintenanceTickets(), bootstrap ? [] : api.getMaintenanceChanges(), bootstrap ? { nodes: [], releases: [], next_node_id: 0 } : api.getMaintenanceCatalog()])
    if (bootstrap) tab.value = 'settings'
    settings.value = { ...results[0], contacts: results[0].contacts || [] }
    technicianIDs.value = (settings.value.technician_ids || []).join(', ')
    tickets.value = results[1] || []; changes.value = results[2] || []; catalog.value = results[3]; loaded.value = true
    if (detail.value) detail.value = await api.getMaintenanceTicket(detail.value.ticket.id)
  })
  loading.value = false
}
async function openTicket(id) { await perform(async () => { detail.value = await api.getMaintenanceTicket(id); note.value = '' }) }
async function refreshTicket() { const id = detail.value.ticket.id; [detail.value, tickets.value] = await Promise.all([api.getMaintenanceTicket(id), api.getMaintenanceTickets()]) }
async function claim() { await perform(async () => { await api.claimMaintenanceTicket(detail.value.ticket.id); await refreshTicket() }, '工单已认领。') }
async function saveNote() { await perform(async () => { await api.addMaintenanceNote(detail.value.ticket.id, note.value.trim()); note.value = ''; await refreshTicket() }, '处理记录已保存。') }
async function close() { await perform(async () => { await api.closeMaintenanceTicket(detail.value.ticket.id, note.value.trim()); note.value = ''; await refreshTicket() }, '工单已关闭。') }
async function prepareRepair(kind) {
  await perform(async () => {
    const nodeID = Number(detail.value.ticket.node_id)
    if (!catalog.value.nodes.some(node => node.id === nodeID)) {
      const page = await api.getMaintenanceCatalog(Math.max(0, nodeID - 1))
      const node = page.nodes.find(item => item.id === nodeID)
      if (node) catalog.value.nodes.push(node)
    }
    const current = catalog.value.nodes.find(node => node.id === nodeID)
    change.nodeIDs = String(nodeID); change.kind = kind
    change.target_version = kind === 'restart' ? current?.active_version || '' : ''
    change.config = '{}'; tab.value = 'changes'
  })
}
async function loadMoreNodes() {
  await perform(async () => {
    const page = await api.getMaintenanceCatalog(catalog.value.next_node_id)
    const existing = new Set(catalog.value.nodes.map(node => node.id))
    catalog.value.nodes.push(...page.nodes.filter(node => !existing.has(node.id)))
    catalog.value.next_node_id = page.next_node_id
  })
}
function ids(value) { return value.trim() ? value.split(/[,，\s]+/).map(Number) : [] }
async function submitChange() {
  await perform(async () => {
    const nodeIDs = ids(change.nodeIDs)
    if (!nodeIDs.length || nodeIDs.some(id => !Number.isSafeInteger(id) || id < 1)) throw new Error('请填写有效的节点 ID。')
    const config = ['install', 'update', 'configure'].includes(change.kind) ? JSON.parse(change.config) : {}
    await api.createMaintenanceChange({ request_key: crypto.randomUUID(), node_ids: nodeIDs, plugin_id: 'machine-telemetry', target_version: change.target_version.trim(), kind: change.kind, config })
    changes.value = await api.getMaintenanceChanges()
  }, '操作请求已保存，请查看审批和执行状态。')
}
async function approve(item) { await perform(async () => { await api.approveMaintenanceChange(item.id, item.binding_hash); changes.value = await api.getMaintenanceChanges() }, '具体操作已批准。') }
async function execute(item) { await perform(async () => { await api.executeMaintenanceChange(item.id); changes.value = await api.getMaintenanceChanges() }, '操作已持久化排队，等待 Agent 执行回报。') }
async function saveSettings() { await perform(async () => { settings.value = await api.saveMaintenanceSettings({ id: settings.value.id || 1, enabled: settings.value.enabled, owner_id: settings.value.owner_id, technician_ids: ids(technicianIDs.value), contacts: settings.value.contacts, revision: settings.value.revision || 0 }); technicianIDs.value = (settings.value.technician_ids || []).join(', ') }, '运维设置已保存。') }
async function verify(channel) { await perform(() => api.verifyMaintenanceChannel(channel), '验证码已加入发送队列，请检查负责人渠道。') }
async function confirm(channel) { await perform(async () => { await api.confirmMaintenanceChannel(channel, codes[channel]); settings.value = await api.getMaintenanceSettings(); codes[channel] = '' }, '渠道已验证。') }
onMounted(reload)
</script>

<style scoped>
.maintenance-page { max-width: 1500px; margin: 0 auto; padding: 28px; color: var(--text-primary, #182235); }
.page-header,.section-heading,.actions,.verification,.tabs { display:flex; align-items:center; justify-content:space-between; gap:12px; flex-wrap:wrap; }
.page-header { margin-bottom:24px; }.eyebrow { font-size:12px; letter-spacing:.1em; color:var(--primary, #286be6); }.page-header h1 { margin:6px 0; }.page-header p,.muted { color:var(--text-secondary, #627086); }.actions { justify-content:flex-start; }.tabs { justify-content:flex-start; margin-bottom:18px; }.active { outline:2px solid var(--primary, #286be6); }
.panel { background:var(--bg-card, #fff); border:1px solid var(--border-color, #dce3ef); border-radius:12px; padding:22px; min-width:0; }.panel h2 { margin-top:0; }.panel h3 { margin-top:24px; }.ticket-grid { display:grid; grid-template-columns:minmax(310px, .9fr) minmax(380px, 1.2fr); gap:20px; }.ticket-row { display:flex; width:100%; text-align:left; flex-wrap:wrap; gap:10px; padding:17px 8px; border:0; border-bottom:1px solid var(--border-color, #dce3ef); color:inherit; background:transparent; cursor:pointer; }.ticket-row.selected { background:var(--bg-secondary, #edf4ff); }.ticket-row span:first-child { flex:1 1 100%; }.ticket-row small { display:block; margin-top:5px; }.badge { border-radius:12px; padding:3px 9px; background:var(--bg-secondary, #edf4ff); font-size:12px; }.detail dl { display:grid; grid-template-columns:1fr 1.4fr; gap:9px; }.detail dd { margin:0; overflow-wrap:anywhere; }.detail dt { color:var(--text-secondary, #627086); }.timeline { padding-left:22px; }.timeline li { padding:7px 0; }.timeline p { margin:5px 0; white-space:pre-wrap; overflow-wrap:anywhere; }.table-scroll { overflow-x:auto; }table { width:100%; border-collapse:collapse; text-align:left; }th,td { padding:9px; border-bottom:1px solid var(--border-color, #dce3ef); }label { display:flex; flex-direction:column; gap:7px; margin:10px 0; }input,select,textarea { padding:9px; border:1px solid var(--border-color, #c8d3e5); border-radius:6px; background:var(--bg-card, #fff); color:inherit; font:inherit; min-width:0; }textarea { width:100%; box-sizing:border-box; }.change-form,.settings-grid { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:14px; align-items:end; }.change-card { border-top:1px solid var(--border-color, #dce3ef); margin-top:22px; padding-top:8px; }.contact-row { display:grid; grid-template-columns:100px 1fr 1fr auto; gap:12px; align-items:end; }fieldset { border:0; padding:0; }.enable-switch { flex-direction:row; margin-top:20px; }.verification { justify-content:flex-start; padding:15px 0; border-bottom:1px solid var(--border-color, #dce3ef); }.hash,pre { overflow-wrap:anywhere; white-space:pre-wrap; font-size:12px; }.alert-error,.alert-success { padding:12px; border-radius:8px; }.alert-error { background:#fee2e2; color:#8f1919; }.alert-success { background:#dcfce7; color:#166534; }.link { border:0; background:none; color:var(--primary, #286be6); cursor:pointer; }
@media(max-width:850px) { .maintenance-page { padding:16px; }.ticket-grid,.change-form,.settings-grid { grid-template-columns:1fr; }.contact-row { grid-template-columns:1fr 1fr; }.detail dl { grid-template-columns:1fr; }.panel { padding:16px; } }
</style>
