<template>
  <div class="page-shell">
    <div class="page-toolbar">
      <div>
        <h1>{{ t('adminUsers.title') }}</h1>
        <p>{{ t('adminUsers.subtitle') }}</p>
      </div>
      <button class="btn btn-primary" @click="showCreateModal = true">{{ t('adminUsers.actions.addUser') }}</button>
    </div>

    <section class="metrics-grid">
      <article class="section-panel metric-card">
        <span class="metric-code primary">USR</span>
        <strong>{{ stats.total_users || 0 }}</strong>
        <span>{{ t('adminUsers.stats.totalUsers') }}</span>
      </article>
      <article class="section-panel metric-card">
        <span class="metric-code success">ACT</span>
        <strong>{{ stats.active_users || 0 }}</strong>
        <span>{{ t('adminUsers.stats.activeUsers') }}</span>
      </article>
      <article class="section-panel metric-card">
        <span class="metric-code warning">EXP</span>
        <strong>{{ stats.expired_users || 0 }}</strong>
        <span>{{ t('adminUsers.stats.expiredUsers') }}</span>
      </article>
      <article class="section-panel metric-card">
        <span class="metric-code danger">BAN</span>
        <strong>{{ stats.banned_users || 0 }}</strong>
        <span>{{ t('adminUsers.stats.bannedUsers') }}</span>
      </article>
    </section>

    <section class="section-panel filter-panel">
      <div class="filter-row">
        <input v-model="filters.email" type="text" :placeholder="t('adminUsers.filters.searchEmail')" @keyup.enter="fetchUsers" />
        <select v-model="filters.status" @change="fetchUsers">
          <option value="">{{ t('adminUsers.filters.allStatus') }}</option>
          <option value="active">{{ t('adminUsers.status.active') }}</option>
          <option value="expired">{{ t('adminUsers.status.expired') }}</option>
          <option value="banned">{{ t('adminUsers.status.banned') }}</option>
        </select>
        <button class="btn" @click="fetchUsers">{{ t('adminUsers.actions.search') }}</button>
      </div>
    </section>

    <section class="section-panel data-panel">
      <div class="table-wrap">
        <table class="data-table">
        <thead>
          <tr>
            <th>{{ t('adminUsers.table.id') }}</th><th>{{ t('adminUsers.table.email') }}</th><th>{{ t('adminUsers.table.plan') }}</th><th>{{ t('adminUsers.table.traffic') }}</th><th>{{ t('adminUsers.table.limits') }}</th>
            <th>{{ t('adminUsers.table.expireAt') }}</th><th>{{ t('adminUsers.table.status') }}</th><th>{{ t('adminUsers.table.createdAt') }}</th><th>{{ t('adminUsers.table.actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="user in users" :key="user.id">
            <td>{{ user.id }}</td>
            <td><span v-if="user.is_admin === 1" class="admin-badge">{{ t('adminUsers.labels.admin') }}</span> {{ user.email }}</td>
            <td>{{ user.plan?.name || '-' }}</td>
            <td>{{ formatBytes((user.u || 0) + (user.d || 0)) }} / {{ formatBytes(user.transfer_enable || 0) }}</td>
            <td>{{ formatUserLimits(user) }}</td>
            <td>{{ formatDate(user.expired_at) }}</td>
            <td><span :class="['status-badge', getStatusClass(user)]">{{ getStatusText(user) }}</span></td>
            <td>{{ formatDateTime(user.created_at) }}</td>
            <td class="actions">
              <button class="btn btn-sm" @click="editUser(user)" :title="t('adminUsers.actions.editUser')">{{ t('common.actions.edit') }}</button>
              <button class="btn btn-sm" @click="openTunnelModal(user)" :title="t('adminUsers.actions.manageTunnel')">{{ t('adminUsers.actions.manageTunnelShort') }}</button>
              <button v-if="user.banned === 0" class="btn btn-sm" @click="handleBan(user)" :title="t('adminUsers.actions.ban')">{{ t('adminUsers.actions.ban') }}</button>
              <button v-else class="btn btn-sm" @click="handleUnban(user)" :title="t('adminUsers.actions.unban')">{{ t('adminUsers.actions.unban') }}</button>
              <button class="btn btn-sm" @click="copySubscribe(user)" :title="t('adminUsers.actions.copySubscribe')">{{ t('adminUsers.actions.copySubscribeShort') }}</button>
              <button class="btn btn-sm" @click="resetSubscribe(user)" :title="t('adminUsers.actions.resetSubscribe')">{{ t('adminUsers.actions.resetSubscribeShort') }}</button>
              <button class="btn btn-sm" @click="openResetUserDialog(user)" :title="t('adminUsers.actions.resetTraffic')">{{ t('adminUsers.actions.resetShort') }}</button>
            </td>
          </tr>
          <tr v-if="users.length === 0"><td colspan="9" class="empty-row">{{ t('adminUsers.empty.noData') }}</td></tr>
        </tbody>
        </table>
      </div>

      <div class="pagination">
        <button class="btn" :disabled="page <= 1" @click="page--; fetchUsers()">{{ t('adminUsers.pagination.prev') }}</button>
        <span>{{ t('adminUsers.pagination.info', { page, totalPages }) }}</span>
        <button class="btn" :disabled="page >= totalPages" @click="page++; fetchUsers()">{{ t('adminUsers.pagination.next') }}</button>
      </div>
    </section>

    <div v-if="showEditModal" class="modal-overlay" @click.self="showEditModal = false">
      <div class="modal">
        <h3>{{ t('adminUsers.editModal.title') }}</h3>
        <label>{{ t('adminUsers.editModal.fields.email') }}<input v-model="editingUser.email" type="email" /></label>
        <label>{{ t('adminUsers.editModal.fields.balance') }}<input v-model.number="editingUser.balance" type="number" /></label>
        <label>{{ t('adminUsers.editModal.fields.transfer') }}<input v-model.number="editingUser.transfer_enable" type="number" /></label>
        <label>{{ t('adminUsers.editModal.fields.speedLimit') }}<input v-model.number="editingUser.speed_limit" data-test="user-speed-limit-input" type="number" min="0" /></label>
        <label>{{ t('adminUsers.editModal.fields.deviceLimit') }}<input v-model.number="editingUser.device_limit" data-test="user-device-limit-input" type="number" min="0" /></label>
        <label>{{ t('adminUsers.editModal.fields.groupId') }}
          <select v-model="editingUser.group_id">
            <option :value="null">{{ t('adminUsers.editModal.groupOptions.unassigned') }}</option>
            <option v-for="g in subscriptionGroups" :key="g.id" :value="g.id">{{ g.name }}</option>
          </select>
        </label>
        <label>{{ t('adminUsers.editModal.fields.expiredAt') }}<input v-model.number="editingUser.expired_at" type="number" /></label>
        <label>{{ t('adminUsers.editModal.fields.flowResetTime') }}<input v-model.number="editingUser.flowResetTime" type="number" min="0" max="31" /></label>
        <label>{{ t('adminUsers.editModal.fields.remark') }}<textarea v-model="editingUser.remark_content" rows="3"></textarea></label>
        <div class="row"><button class="btn" @click="showEditModal = false">{{ t('common.actions.cancel') }}</button><button class="btn btn-primary" data-test="user-save-button" @click="saveUser">{{ t('common.actions.save') }}</button></div>
      </div>
    </div>

    <div v-if="showCreateModal" class="modal-overlay" @click.self="showCreateModal = false">
      <div class="modal">
        <h3>{{ t('adminUsers.createModal.title') }}</h3>
        <label>{{ t('adminUsers.createModal.fields.email') }} *<input v-model="newUser.email" type="email" :placeholder="t('adminUsers.createModal.placeholders.email')" /></label>
        <label>{{ t('adminUsers.createModal.fields.password') }} *<input v-model="newUser.password" type="password" :placeholder="t('adminUsers.createModal.placeholders.password')" /></label>
        <label>{{ t('adminUsers.createModal.fields.userType') }}
          <select v-model="newUser.is_admin">
            <option :value="0">{{ t('adminUsers.createModal.userTypes.normal') }}</option>
            <option :value="1">{{ t('adminUsers.createModal.userTypes.admin') }}</option>
          </select>
        </label>
        <label>{{ t('adminUsers.createModal.fields.groupId') }}
          <select v-model="newUser.group_id">
            <option :value="null">{{ t('adminUsers.createModal.groupOptions.unassigned') }}</option>
            <option v-for="g in subscriptionGroups" :key="g.id" :value="g.id">{{ g.name }}</option>
          </select>
        </label>
        <label>{{ t('adminUsers.createModal.fields.transferEnable') }}<input v-model.number="newUser.transfer_enable" type="number" min="0" :placeholder="t('adminUsers.createModal.placeholders.transferEnable')" /></label>
        <label>{{ t('adminUsers.createModal.fields.speedLimit') }}<input v-model.number="newUser.speed_limit" data-test="new-user-speed-limit-input" type="number" min="0" :placeholder="t('adminUsers.createModal.placeholders.speedLimit')" /></label>
        <label>{{ t('adminUsers.createModal.fields.deviceLimit') }}<input v-model.number="newUser.device_limit" data-test="new-user-device-limit-input" type="number" min="0" :placeholder="t('adminUsers.createModal.placeholders.deviceLimit')" /></label>
        <label>{{ t('adminUsers.createModal.fields.flowResetTime') }}<input v-model.number="newUser.flowResetTime" type="number" min="0" max="31" /></label>
        <p v-if="createError" class="error">{{ createError }}</p>
        <div class="row"><button class="btn" @click="showCreateModal = false">{{ t('common.actions.cancel') }}</button><button class="btn btn-primary" @click="handleCreateUser" :disabled="createLoading">{{ createLoading ? t('adminUsers.createModal.creating') : t('common.actions.create') }}</button></div>
      </div>
    </div>

    <div v-if="showTunnelModal" class="modal-overlay" @click.self="closeTunnelModal">
      <div class="modal modal-wide">
        <h3>{{ t('adminUsers.tunnelModal.title', { email: tunnelUser?.email || '-' }) }}</h3>
        <h4>{{ t('adminUsers.tunnelModal.sections.form') }}</h4>
        <div class="grid">
          <label>{{ t('adminUsers.tunnelModal.fields.tunnel') }}{{ editingTunnelId ? t('adminUsers.tunnelModal.fields.tunnelReadonlyHint') : '' }}
            <select v-model="tunnelForm.tunnelId" :disabled="Boolean(editingTunnelId)" @change="handleTunnelChange">
              <option value="">{{ availableTunnelOptions.length === 0 && !editingTunnelId ? t('adminUsers.tunnelModal.options.noAssignableTunnel') : t('adminUsers.tunnelModal.options.selectTunnel') }}</option>
              <option v-for="item in availableTunnelOptions" :key="item.id" :value="item.id">{{ item.name }} (ID: {{ item.id }})</option>
            </select>
          </label>
          <label v-if="editingTunnelId">{{ t('adminUsers.tunnelModal.fields.status') }}
            <select v-model.number="tunnelForm.status"><option :value="1">{{ t('runtime.shared.enabled') }}</option><option :value="0">{{ t('runtime.shared.disabled') }}</option></select>
          </label>
          <label>{{ t('adminUsers.tunnelModal.fields.flowQuota') }}<input v-model.number="tunnelForm.flow" type="number" min="0" /></label>
          <label>{{ t('adminUsers.tunnelModal.fields.numQuota') }}<input v-model.number="tunnelForm.num" type="number" min="0" /></label>
          <label>{{ t('adminUsers.tunnelModal.fields.expTime') }}<input v-model="tunnelForm.expTime" type="datetime-local" /></label>
          <label>{{ t('adminUsers.tunnelModal.fields.flowResetTime') }}<input v-model.number="tunnelForm.flowResetTime" type="number" min="0" /></label>
          <label>{{ t('adminUsers.tunnelModal.fields.rateLimit') }}
            <select v-model="tunnelForm.speedId" :disabled="speedLimitLoading || !tunnelForm.tunnelId" @change="handleSpeedLimitChange">
              <option :value="null">{{ t('adminUsers.labels.noLimit') }}</option>
              <option v-if="!tunnelForm.tunnelId" disabled value="">{{ t('adminUsers.tunnelModal.options.selectTunnelFirst') }}</option>
              <option v-else-if="!speedLimitLoading && availableSpeedLimitOptions.length === 0" disabled value="">{{ t('adminUsers.tunnelModal.options.noRateLimitRules') }}</option>
              <option v-for="item in availableSpeedLimitOptions" :key="item.id" :value="item.id">{{ formatSpeedLimitOptionLabel(item) }}</option>
            </select>
          </label>
          <div class="row">
            <button v-if="editingTunnelId" class="btn" @click="resetTunnelForm">{{ t('adminUsers.tunnelModal.actions.cancelEdit') }}</button>
            <button class="btn btn-primary" :disabled="tunnelLoading" @click="submitTunnelForm">{{ tunnelLoading ? t('adminUsers.tunnelModal.actions.submitting') : (editingTunnelId ? t('adminUsers.tunnelModal.actions.updateGrant') : t('adminUsers.tunnelModal.actions.addGrant')) }}</button>
          </div>
        </div>

        <h4>{{ t('adminUsers.tunnelModal.sections.list') }}</h4>
        <table class="data-table">
          <thead>
            <tr><th>{{ t('adminUsers.tunnelModal.table.id') }}</th><th>{{ t('adminUsers.tunnelModal.table.tunnel') }}</th><th>{{ t('adminUsers.tunnelModal.table.status') }}</th><th>{{ t('adminUsers.tunnelModal.table.flow') }}</th><th>{{ t('adminUsers.tunnelModal.table.num') }}</th><th>{{ t('adminUsers.tunnelModal.table.expireAt') }}</th><th>{{ t('adminUsers.tunnelModal.table.reset') }}</th><th>{{ t('adminUsers.tunnelModal.table.usedFlow') }}</th><th>{{ t('adminUsers.tunnelModal.table.rateLimit') }}</th><th>{{ t('adminUsers.table.actions') }}</th></tr>
          </thead>
          <tbody>
            <tr v-if="tunnelListLoading"><td colspan="10" class="empty-row">{{ t('common.states.loading') }}</td></tr>
            <tr v-for="item in userTunnels" :key="item.id">
              <td>{{ item.id }}</td><td>{{ item.tunnelName || item.tunnelId }}</td>
              <td><span :class="['status-badge', item.status === 1 ? 'status-active' : 'status-banned']">{{ item.status === 1 ? t('runtime.shared.enabled') : t('runtime.shared.disabled') }}</span></td>
              <td>{{ item.flow ?? 0 }}</td><td>{{ item.num ?? 0 }}</td><td>{{ formatTunnelExpire(item.expTime) }}</td><td>{{ formatFlowResetDay(item.flowResetTime) }}</td><td>{{ formatBytes(calculateTunnelUsedFlow(item)) }}</td><td>{{ formatTunnelRateLimit(item) }}</td>
              <td class="actions"><button class="btn btn-sm" @click="editTunnelGrant(item)">{{ t('common.actions.edit') }}</button><button class="btn btn-sm" @click="openResetTunnelDialog(item)">{{ t('adminUsers.actions.resetTraffic') }}</button><button class="btn btn-sm" @click="removeTunnelGrant(item)">{{ t('common.actions.delete') }}</button></td>
            </tr>
            <tr v-if="!tunnelListLoading && userTunnels.length === 0"><td colspan="10" class="empty-row">{{ t('adminUsers.tunnelModal.empty') }}</td></tr>
          </tbody>
        </table>
        <div class="row"><button class="btn" @click="closeTunnelModal">{{ t('common.actions.close') }}</button></div>
      </div>
    </div>

    <div v-if="showResetFlowModal" class="modal-overlay" @click.self="closeResetFlowModal">
      <div class="modal">
        <h3>{{ resetFlowTitle }}</h3>
        <p>{{ resetFlowMessage }}</p>
        <div><strong>{{ t('adminUsers.resetFlow.usedFlow') }}</strong> {{ resetFlowUsedFlow }}</div>
        <div v-if="resetFlowQuota"><strong>{{ t('adminUsers.resetFlow.quota') }}</strong> {{ resetFlowQuota }}</div>
        <div class="row">
          <button class="btn" :disabled="resetFlowLoading" @click="closeResetFlowModal">{{ t('common.actions.cancel') }}</button>
          <button class="btn btn-primary" :disabled="resetFlowLoading" @click="confirmResetFlow">{{ resetFlowLoading ? t('adminUsers.resetFlow.resetting') : t('adminUsers.resetFlow.confirmAction') }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import {
  assignAdminUserTunnel, banUser, createUser, getAdminUserTunnelList, getForwardTunnels, getSpeedLimitList,
  getSubscriptionSettings,
  getUserList, getUserStats, removeAdminUserTunnel, resetUserSubscribe, resetUserTraffic, resetUserTunnelTraffic,
  unbanUser, updateAdminUserTunnel, updateUser
} from '@/api/admin'
import { getSubscriptionGroups } from '@/api/admin'

const { t, formatDate: i18nFormatDate, formatDateTime: i18nFormatDateTime } = useAppI18n()
const users = ref([])
const stats = ref({})
const subscriptionGroups = ref([])
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const filters = ref({ email: '', status: '' })
const showEditModal = ref(false)
const editingUser = ref({})
const showCreateModal = ref(false)
const createLoading = ref(false)
const createError = ref('')
const subscriptionSettings = ref({ subscribe_path: '/s', subscribe_domains: [] })
const newBlankUser = () => ({ email: '', password: '', is_admin: 0, flowResetTime: 0, group_id: null, transfer_enable: 0, speed_limit: 0, device_limit: 0 })
const newUser = ref(newBlankUser())
const showTunnelModal = ref(false)
const tunnelUser = ref(null)
const tunnelOptions = ref([])
const speedLimitOptions = ref([])
const userTunnels = ref([])
const tunnelListLoading = ref(false)
const tunnelLoading = ref(false)
const speedLimitLoading = ref(false)
const editingTunnelId = ref(null)
const showResetFlowModal = ref(false)
const resetFlowLoading = ref(false)
const resetFlowTarget = ref(null)
const resetFlowTitle = ref('')
const resetFlowMessage = ref('')
const resetFlowUsedFlow = ref('')
const resetFlowQuota = ref('')
const newTunnelForm = () => ({ tunnelId: '', flow: 0, num: 0, expTime: '', flowResetTime: 0, speedId: null, status: 1 })
const tunnelForm = ref(newTunnelForm())

const totalPages = computed(() => Math.ceil(total.value / pageSize.value) || 1)
const assignedTunnelIds = computed(() => new Set(userTunnels.value.map(item => Number(item?.tunnelId || 0)).filter(id => id > 0)))
const availableTunnelOptions = computed(() => editingTunnelId.value ? tunnelOptions.value : tunnelOptions.value.filter(item => !assignedTunnelIds.value.has(Number(item?.id || 0))))
const availableSpeedLimitOptions = computed(() => {
  const targetTunnelId = Number(tunnelForm.value.tunnelId || 0)
  return targetTunnelId ? speedLimitOptions.value.filter(item => Number(item?.tunnelId || 0) === targetTunnelId) : []
})
const notify = (message) => window.alert(message)
const confirmAction = (message) => window.confirm(message)

const assertCompatSuccess = (res, fallback = t('adminUsers.messages.actionFailed')) => {
  if (res && typeof res.code === 'number' && res.code !== 0) throw new Error(res.msg || fallback)
  return res
}
const getResData = (res, fallback = t('adminUsers.messages.actionFailed')) => {
  if (!res) return null
  if (typeof res.code === 'number') {
    if (res.code !== 0) throw new Error(res.msg || fallback)
    return res.data
  }
  return res.data
}

const toDateTimeLocal = (timestamp) => {
  if (!timestamp) return ''
  let value = Number(timestamp)
  if (!Number.isFinite(value) || value <= 0) return ''
  if (value < 1000000000000) value *= 1000
  const date = new Date(value)
  const pad = n => String(n).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`
}
const fromDateTimeLocal = value => (!value ? 0 : (Number.isNaN(Date.parse(value)) ? 0 : Date.parse(value)))

const handleCreateUser = async () => {
  if (!newUser.value.email || !newUser.value.password) return (createError.value = t('adminUsers.messages.fillEmailPassword'))
  if (newUser.value.password.length < 6) return (createError.value = t('adminUsers.messages.passwordTooShort'))
  createLoading.value = true
  createError.value = ''
  try {
    await createUser(newUser.value)
    showCreateModal.value = false
    newUser.value = newBlankUser()
    fetchUsers()
    fetchStats()
    notify(t('adminUsers.messages.userCreated'))
  } catch (err) {
    createError.value = err.response?.data?.message || t('adminUsers.messages.createFailed')
  } finally {
    createLoading.value = false
  }
}

const fetchUsers = async () => {
  try {
    const res = await getUserList({ page: page.value, page_size: pageSize.value, email: filters.value.email, status: filters.value.status })
    const payload = getResData(res) || {}
    users.value = payload.list || []
    total.value = payload.total || 0
  } catch (err) {
    console.error(t('adminUsers.messages.fetchUsersFailed'), err)
  }
}
const fetchStats = async () => {
  try { stats.value = getResData(await getUserStats()) || {} } catch (err) { console.error(t('adminUsers.messages.fetchStatsFailed'), err) }
}
const editUser = (user) => {
  editingUser.value = {
    ...user,
    flowResetTime: Number(user?.flowResetTime || 0),
    speed_limit: Number(user?.speed_limit || 0),
    device_limit: Number(user?.device_limit || 0)
  }
  showEditModal.value = true
}
const saveUser = async () => {
  try {
    await updateUser(editingUser.value.id, {
      email: editingUser.value.email, balance: editingUser.value.balance, transfer_enable: editingUser.value.transfer_enable,
      speed_limit: Number(editingUser.value.speed_limit || 0), device_limit: Number(editingUser.value.device_limit || 0),
      expired_at: editingUser.value.expired_at, flowResetTime: Number(editingUser.value.flowResetTime || 0), remark_content: editingUser.value.remark_content,
      group_id: editingUser.value.group_id || null
    })
    showEditModal.value = false
    fetchUsers()
  } catch (err) {
    notify(t('adminUsers.messages.saveFailed', { message: err.response?.data?.message || err.message }))
  }
}

const handleBan = async (user) => {
  if (!confirmAction(t('adminUsers.messages.confirmBan', { email: user.email }))) return
  try { await banUser(user.id); fetchUsers(); fetchStats() } catch (err) { notify(t('adminUsers.messages.actionFailed')) }
}
const handleUnban = async (user) => {
  if (!confirmAction(t('adminUsers.messages.confirmUnban', { email: user.email }))) return
  try { await unbanUser(user.id); fetchUsers(); fetchStats() } catch (err) { notify(t('adminUsers.messages.actionFailed')) }
}

const openResetUserDialog = (user) => {
  resetFlowTarget.value = { type: 'user', id: user.id }
  resetFlowTitle.value = t('adminUsers.resetFlow.userTitle')
  resetFlowMessage.value = t('adminUsers.resetFlow.userMessage', { email: user.email })
  resetFlowUsedFlow.value = formatBytes((user.u || 0) + (user.d || 0))
  resetFlowQuota.value = user.transfer_enable ? formatBytes(user.transfer_enable) : ''
  showResetFlowModal.value = true
}

// 拼某用户的订阅链接。订阅路径固定为 /s (与 user/Subscribe.vue 一致)。
const preferredSubscribeOrigin = computed(() => {
  const domains = Array.isArray(subscriptionSettings.value.subscribe_domains) ? subscriptionSettings.value.subscribe_domains : []
  const currentHost = window.location.host
  const host = domains.includes(currentHost) ? currentHost : (domains[0] || currentHost)
  return `${window.location.protocol}//${host}`
})

const buildSubscribeUrl = (user) => `${preferredSubscribeOrigin.value}${subscriptionSettings.value.subscribe_path || '/s'}/${user.token}`

// 复制到剪贴板。navigator.clipboard 只在 HTTPS/localhost 可用, HTTP+IP 直连时
// 用隐藏 textarea + execCommand('copy') 兜底, 让明文 HTTP 也能复制。
const copyToClipboard = async (text) => {
  if (navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text)
      return true
    } catch {
      // 落到下面的兜底
    }
  }
  try {
    const ta = document.createElement('textarea')
    ta.value = text
    ta.style.position = 'fixed'
    ta.style.top = '-9999px'
    document.body.appendChild(ta)
    ta.focus()
    ta.select()
    const ok = document.execCommand('copy')
    document.body.removeChild(ta)
    return ok
  } catch {
    return false
  }
}

const copySubscribe = async (user) => {
  if (!user.token) {
    notify(t('adminUsers.messages.noToken'))
    return
  }
  const url = buildSubscribeUrl(user)
  if (await copyToClipboard(url)) {
    notify(t('adminUsers.messages.subscribeCopied'))
  } else {
    // 兜底也失败: 把链接直接弹出来让用户手动复制
    window.prompt(t('adminUsers.messages.copyManual'), url)
  }
}

const loadSubscriptionSettings = async () => {
  try {
    const res = await getSubscriptionSettings()
    subscriptionSettings.value = res.data || { subscribe_path: '/s', subscribe_domains: [] }
  } catch {
    subscriptionSettings.value = { subscribe_path: '/s', subscribe_domains: [] }
  }
}

const resetSubscribe = async (user) => {
  if (!confirm(t('adminUsers.messages.resetSubscribeConfirm', { email: user.email }))) return
  try {
    await resetUserSubscribe(user.id)
    notify(t('adminUsers.messages.resetSubscribeSuccess'))
    await fetchUsers()
  } catch (err) {
    notify(err.response?.data?.message || err.message || t('adminUsers.messages.resetSubscribeFailed'))
  }
}

const normalizeSpeedLimitList = items => Array.isArray(items) ? items.map(item => {
  const id = Number(item?.id || 0)
  if (!Number.isFinite(id) || id <= 0) return null
  return { id, name: item?.name || `Rule-${id}`, speed: Number(item?.speed || 0), tunnelId: Number(item?.tunnelId ?? item?.tunnel_id ?? 0) }
}).filter(Boolean) : []

const loadTunnelOptions = async () => {
  try { tunnelOptions.value = getResData(await getForwardTunnels(), t('adminUsers.messages.fetchTunnelListFailed')) || [] } catch (err) {
    notify(err.response?.data?.msg || err.response?.data?.message || err.message || t('adminUsers.messages.fetchTunnelListFailed')); tunnelOptions.value = []
  }
}
const loadSpeedLimitOptions = async () => {
  speedLimitLoading.value = true
  try {
    const payload = getResData(await getSpeedLimitList(), t('adminUsers.messages.fetchSpeedLimitFailed'))
    const list = Array.isArray(payload) ? payload : (payload?.list || [])
    speedLimitOptions.value = normalizeSpeedLimitList(list)
  } catch (err) {
    notify(err.response?.data?.msg || err.response?.data?.message || err.message || t('adminUsers.messages.fetchSpeedLimitFailed')); speedLimitOptions.value = []
  } finally {
    speedLimitLoading.value = false
  }
}
const loadUserTunnels = async (userId) => {
  tunnelListLoading.value = true
  try { userTunnels.value = getResData(await getAdminUserTunnelList({ userId }), t('adminUsers.messages.fetchTunnelGrantFailed')) || [] } catch (err) {
    notify(err.response?.data?.msg || err.response?.data?.message || err.message || t('adminUsers.messages.fetchTunnelGrantFailed'))
  } finally {
    tunnelListLoading.value = false
  }
}

const normalizeSpeedId = (speedId, tunnelId) => {
  if (speedId === null || speedId === '' || typeof speedId === 'undefined') return null
  const value = Number(speedId)
  if (!Number.isFinite(value) || value <= 0) return null
  const currentTunnelId = Number(tunnelId || 0)
  if (!currentTunnelId) return null
  return speedLimitOptions.value.some(item => Number(item.id) === value && Number(item.tunnelId || 0) === currentTunnelId) ? value : null
}
const handleSpeedLimitChange = () => { tunnelForm.value.speedId = normalizeSpeedId(tunnelForm.value.speedId, tunnelForm.value.tunnelId) }
const handleTunnelChange = () => { tunnelForm.value.speedId = normalizeSpeedId(tunnelForm.value.speedId, tunnelForm.value.tunnelId) }
const formatSpeedLimitOptionLabel = item => (item?.name || `Rule-${item?.id}`)
const formatTunnelRateLimit = (item) => {
  if (item?.speedLimitName) return item.speedLimitName
  const speedId = Number(item?.speedId || 0)
  if (speedId > 0) {
    const found = speedLimitOptions.value.find(limit => Number(limit?.id || 0) === speedId)
    if (found) return formatSpeedLimitOptionLabel(found)
    return speedId
  }
  return t('adminUsers.labels.noLimit')
}

const openTunnelModal = async (user) => {
  tunnelUser.value = user
  resetTunnelForm()
  showTunnelModal.value = true
  await Promise.all([loadTunnelOptions(), loadSpeedLimitOptions(), loadUserTunnels(user.id)])
}
const closeTunnelModal = () => {
  showTunnelModal.value = false
  tunnelUser.value = null
  userTunnels.value = []
  resetTunnelForm()
}
const resetTunnelForm = () => { editingTunnelId.value = null; tunnelForm.value = newTunnelForm() }

const submitTunnelForm = async () => {
  if (!tunnelUser.value) return
  if (!tunnelForm.value.tunnelId) return notify(t('adminUsers.messages.selectTunnelFirst'))
  if (!editingTunnelId.value && assignedTunnelIds.value.has(Number(tunnelForm.value.tunnelId))) return notify(t('adminUsers.messages.tunnelAlreadyAssigned'))
  tunnelLoading.value = true
  try {
    const payload = {
      tunnelId: Number(tunnelForm.value.tunnelId), flow: Number(tunnelForm.value.flow || 0), num: Number(tunnelForm.value.num || 0),
      expTime: fromDateTimeLocal(tunnelForm.value.expTime), flowResetTime: Number(tunnelForm.value.flowResetTime || 0),
      speedId: normalizeSpeedId(tunnelForm.value.speedId, tunnelForm.value.tunnelId), status: Number(tunnelForm.value.status || 1)
    }
    if (editingTunnelId.value) {
      assertCompatSuccess(await updateAdminUserTunnel({ id: Number(editingTunnelId.value), ...payload }), t('adminUsers.messages.grantUpdateFailed'))
      notify(t('adminUsers.messages.grantUpdated'))
    } else {
      assertCompatSuccess(await assignAdminUserTunnel({ userId: Number(tunnelUser.value.id), ...payload }), t('adminUsers.messages.grantCreateFailed'))
      notify(t('adminUsers.messages.grantCreated'))
    }
    await loadUserTunnels(tunnelUser.value.id)
    resetTunnelForm()
  } catch (err) {
    notify(err.response?.data?.msg || err.response?.data?.message || t('adminUsers.messages.grantActionFailed'))
  } finally {
    tunnelLoading.value = false
  }
}
const editTunnelGrant = (item) => {
  editingTunnelId.value = item.id
  tunnelForm.value = { tunnelId: item.tunnelId || '', flow: item.flow ?? 0, num: item.num ?? 0, expTime: toDateTimeLocal(item.expTime), flowResetTime: item.flowResetTime ?? 0, speedId: normalizeSpeedId(item.speedId ?? null, item.tunnelId || ''), status: item.status ?? 1 }
}
const removeTunnelGrant = async (item) => {
  if (!confirmAction(t('adminUsers.messages.confirmDeleteGrant', { id: item.id }))) return
  try {
    assertCompatSuccess(await removeAdminUserTunnel({ id: item.id }), t('adminUsers.messages.grantDeleteFailed'))
    if (tunnelUser.value) await loadUserTunnels(tunnelUser.value.id)
    if (editingTunnelId.value === item.id) resetTunnelForm()
  } catch (err) {
    notify(err.response?.data?.msg || err.response?.data?.message || t('adminUsers.messages.grantDeleteFailed'))
  }
}
const calculateTunnelUsedFlow = item => Number(item?.inFlow || 0) + Number(item?.outFlow || 0)

const openResetTunnelDialog = (item) => {
  if (!item?.id) return
  resetFlowTarget.value = { type: 'tunnel', id: item.id }
  resetFlowTitle.value = t('adminUsers.resetFlow.tunnelTitle')
  resetFlowMessage.value = t('adminUsers.resetFlow.tunnelMessage', { id: item.id })
  resetFlowUsedFlow.value = formatBytes(calculateTunnelUsedFlow(item))
  resetFlowQuota.value = Number(item?.flow || 0) > 0 ? `${item.flow} GB` : ''
  showResetFlowModal.value = true
}
const closeResetFlowModal = (force = false) => {
  if (resetFlowLoading.value && !force) return
  showResetFlowModal.value = false
  resetFlowTarget.value = null
  resetFlowTitle.value = ''
  resetFlowMessage.value = ''
  resetFlowUsedFlow.value = ''
  resetFlowQuota.value = ''
}
const confirmResetFlow = async () => {
  if (!resetFlowTarget.value?.id) return
  resetFlowLoading.value = true
  try {
    if (resetFlowTarget.value.type === 'user') {
      assertCompatSuccess(await resetUserTraffic(resetFlowTarget.value.id), t('adminUsers.messages.resetFailed'))
      await fetchUsers()
      notify(t('adminUsers.messages.userFlowReset'))
    } else {
      assertCompatSuccess(await resetUserTunnelTraffic(resetFlowTarget.value.id), t('adminUsers.messages.resetFailed'))
      if (tunnelUser.value) await loadUserTunnels(tunnelUser.value.id)
      notify(t('adminUsers.messages.tunnelFlowReset'))
    }
    closeResetFlowModal(true)
  } catch (err) {
    notify(err.response?.data?.msg || err.response?.data?.message || err.message || t('adminUsers.messages.resetFailed'))
  } finally {
    resetFlowLoading.value = false
  }
}

const formatFlowResetDay = (value) => {
  const day = Number(value || 0)
  if (!day) return t('adminUsers.labels.noReset')
  return t('adminUsers.labels.monthlyDay', { day })
}
const formatUserLimits = (user) => {
  const speedLimit = Number(user?.speed_limit || 0)
  const deviceLimit = Number(user?.device_limit || 0)
  const speedText = speedLimit > 0 ? t('adminUsers.labels.speedLimitMbps', { value: speedLimit }) : t('adminUsers.labels.noSpeedLimit')
  const deviceText = deviceLimit > 0 ? t('adminUsers.labels.deviceLimitCount', { value: deviceLimit }) : t('adminUsers.labels.noDeviceLimit')
  return `${speedText} / ${deviceText}`
}
const formatTunnelExpire = (value) => {
  if (!value) return t('adminUsers.labels.permanent')
  const date = new Date(value < 1000000000000 ? value * 1000 : value)
  if (Number.isNaN(date.getTime())) return '-'
  return i18nFormatDateTime(date) || '-'
}
const getStatusClass = (user) => {
  if (user.banned === 1) return 'status-banned'
  if (user.expired_at && user.expired_at < Date.now() / 1000) return 'status-expired'
  return 'status-active'
}
const getStatusText = (user) => {
  if (user.banned === 1) return t('adminUsers.status.banned')
  if (user.expired_at && user.expired_at < Date.now() / 1000) return t('adminUsers.status.expired')
  return t('adminUsers.status.active')
}
const formatBytes = (bytes) => {
  if (!bytes) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let value = Number(bytes)
  while (value >= 1024 && i < units.length - 1) { value /= 1024; i += 1 }
  return `${value.toFixed(2)} ${units[i]}`
}
const formatDate = (timestamp) => (!timestamp ? t('adminUsers.labels.permanent') : (i18nFormatDate(Number(timestamp) * 1000) || '-'))
const formatDateTime = (datetime) => (!datetime ? '-' : (i18nFormatDateTime(datetime) || '-'))

onMounted(() => { fetchUsers(); fetchStats(); loadSubscriptionGroups(); loadSubscriptionSettings() })

const loadSubscriptionGroups = async () => {
  try {
    const res = await getSubscriptionGroups()
    subscriptionGroups.value = res?.data || []
  } catch (err) {
    console.error('Failed to load subscription groups', err)
  }
}
</script>

<style scoped>
.metrics-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 16px;
}

.metric-card {
  padding: 18px;
  display: grid;
  gap: 6px;
}

.metric-card strong {
  font-size: 30px;
  line-height: 1.1;
}

.metric-card span:last-child {
  color: var(--text-secondary);
  font-size: 13px;
}

.metric-code {
  width: fit-content;
  min-width: 44px;
  padding: 4px 10px;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 800;
}

.metric-code.primary { background: rgba(0, 100, 250, 0.08); color: var(--primary-color); }
.metric-code.success { background: rgba(22, 163, 74, 0.08); color: var(--success-color); }
.metric-code.warning { background: rgba(217, 119, 6, 0.08); color: var(--warning-color); }
.metric-code.danger { background: rgba(220, 38, 38, 0.08); color: var(--error-color); }

.filter-panel,
.data-panel {
  padding: 18px;
}

.filter-row {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
}

.filter-row input {
  flex: 1;
  min-width: 260px;
}

.filter-row select {
  width: 180px;
}

.table-wrap {
  overflow-x: auto;
}

.data-table {
  width: 100%;
  border-collapse: collapse;
}

.data-table th,
.data-table td {
  padding: 14px 10px;
  border-bottom: 1px solid var(--border-color);
  text-align: left;
  white-space: nowrap;
  vertical-align: top;
}

.data-table th {
  font-size: 12px;
  text-transform: uppercase;
  color: var(--text-secondary);
}

.admin-badge {
  display: inline-flex;
  align-items: center;
  min-height: 20px;
  padding: 0 8px;
  border-radius: 999px;
  background: var(--primary-soft);
  color: var(--primary-color);
  margin-right: 6px;
  font-size: 10px;
  font-weight: 700;
}

.status-badge {
  display: inline-flex;
  align-items: center;
  min-height: 24px;
  padding: 0 10px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 700;
}

.status-active { background: rgba(22, 163, 74, 0.08); color: var(--success-color); }
.status-expired { background: rgba(217, 119, 6, 0.08); color: var(--warning-color); }
.status-banned { background: rgba(220, 38, 38, 0.08); color: var(--error-color); }

.actions {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

.empty-row {
  text-align: center;
  color: var(--text-secondary);
}

.pagination {
  margin-top: 16px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  flex-wrap: wrap;
}

.modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(15, 23, 42, 0.42);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
  z-index: 1000;
}

.modal {
  width: min(96vw, 560px);
  max-height: 90vh;
  overflow: auto;
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: 8px;
  padding: 18px;
  display: grid;
  gap: 12px;
  box-shadow: var(--shadow-lg);
}

.modal-wide {
  width: min(96vw, 1100px);
}

.modal label {
  display: grid;
  gap: 6px;
  font-size: 13px;
  font-weight: 700;
  color: var(--text-secondary);
}

.grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.row {
  display: flex;
  gap: 8px;
  justify-content: flex-end;
  align-items: center;
  flex-wrap: wrap;
}

.error {
  color: var(--error-color);
  margin: 0;
}

@media (max-width: 900px) {
  .grid {
    grid-template-columns: 1fr;
  }
}
</style>
