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
            <th>{{ t('adminUsers.table.id') }}</th><th>{{ t('adminUsers.table.email') }}</th><th data-test="user-plan-column">{{ isCommercial ? t('adminUsers.table.plan') : t('adminUsers.table.subscriptionTemplate') }}</th><th>{{ t('adminUsers.table.traffic') }}</th><th>{{ t('adminUsers.table.limits') }}</th>
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
              <button class="btn btn-sm" @click="openTrafficModal(user)" :title="t('adminUsers.actions.viewTraffic')">{{ t('adminUsers.actions.viewTrafficShort') }}</button>
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

    <UiDialog v-model:open="showEditModal" :title="t('adminUsers.editModal.title')">
      <div class="dialog-form">
        <label>{{ t('adminUsers.editModal.fields.email') }}<input v-model="editingUser.email" type="email" /></label>
        <label v-if="isCommercial" data-test="user-balance-field">{{ t('adminUsers.editModal.fields.balance') }}<input v-model.number="editingUser.balance" type="number" /></label>
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
        <p v-if="editError" class="error" role="alert" data-test="user-save-error">{{ editError }}</p>
      </div>
      <template #footer="{ close }">
        <UiButton :disabled="editSaving" @click="close">{{ t('common.actions.cancel') }}</UiButton>
        <UiButton variant="primary" data-test="user-save-button" :loading="editSaving" @click="saveUser">{{ t('common.actions.save') }}</UiButton>
      </template>
    </UiDialog>

    <UiDialog v-model:open="showCreateModal" :title="t('adminUsers.createModal.title')">
      <div class="dialog-form">
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
        <p v-if="createError" class="error" role="alert">{{ createError }}</p>
      </div>
      <template #footer="{ close }">
        <UiButton :disabled="createLoading" @click="close">{{ t('common.actions.cancel') }}</UiButton>
        <UiButton variant="primary" data-test="user-create-button" :loading="createLoading" @click="handleCreateUser">{{ t('common.actions.create') }}</UiButton>
      </template>
    </UiDialog>

    <UiDialog :open="showTunnelModal" size="lg" :title="t('adminUsers.tunnelModal.title', { email: tunnelUser?.email || '-' })" @update:open="value => { if (!value) closeTunnelModal() }">
      <section class="dialog-section">
        <h4>{{ t('adminUsers.tunnelModal.sections.form') }}</h4>
        <div class="grid dialog-form">
          <label>{{ t('adminUsers.tunnelModal.fields.tunnel') }}{{ editingTunnelId ? t('adminUsers.tunnelModal.fields.tunnelReadonlyHint') : '' }}
            <select v-model="tunnelForm.tunnelId" :disabled="Boolean(editingTunnelId)" :aria-invalid="tunnelFormError ? 'true' : undefined" :aria-describedby="tunnelFormError ? 'user-tunnel-form-error' : undefined" @change="handleTunnelChange">
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
          <p v-if="tunnelFormError" id="user-tunnel-form-error" class="error grid-full" role="alert">{{ tunnelFormError }}</p>
          <div class="row grid-full">
            <UiButton v-if="editingTunnelId" @click="resetTunnelForm">{{ t('adminUsers.tunnelModal.actions.cancelEdit') }}</UiButton>
            <UiButton variant="primary" :loading="tunnelLoading" data-test="user-tunnel-submit" @click="submitTunnelForm">{{ editingTunnelId ? t('adminUsers.tunnelModal.actions.updateGrant') : t('adminUsers.tunnelModal.actions.addGrant') }}</UiButton>
          </div>
        </div>
      </section>

      <section class="dialog-section">
        <h4>{{ t('adminUsers.tunnelModal.sections.list') }}</h4>
        <div class="table-wrap">
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
        </div>
      </section>
      <template #footer="{ close }">
        <UiButton @click="close">{{ t('common.actions.close') }}</UiButton>
      </template>
    </UiDialog>

    <UiConfirmDialog
      :open="showResetFlowModal"
      tone="danger"
      :title="resetFlowTitle"
      :message="resetFlowMessage"
      :confirm-label="t('adminUsers.resetFlow.confirmAction')"
      :loading="resetFlowLoading"
      :error="resetFlowError"
      @confirm="confirmResetFlow"
      @cancel="closeResetFlowModal()"
    >
      <dl class="reset-flow-facts">
        <div><dt>{{ t('adminUsers.resetFlow.usedFlow') }}</dt><dd>{{ resetFlowUsedFlow }}</dd></div>
        <div v-if="resetFlowQuota"><dt>{{ t('adminUsers.resetFlow.quota') }}</dt><dd>{{ resetFlowQuota }}</dd></div>
      </dl>
    </UiConfirmDialog>

    <UiSheet
      :open="showTrafficModal"
      size="lg"
      :title="t('adminUsers.trafficModal.title', { email: trafficUser?.email || '-' })"
      :description="t('adminUsers.trafficModal.subtitle')"
      :dismissible="!trafficLoading"
      @update:open="value => { if (!value) closeTrafficModal() }"
    >
      <template #header-actions>
        <UiButton size="sm" :loading="trafficLoading" @click="loadTrafficDetail">{{ t('adminUsers.trafficModal.refresh') }}</UiButton>
      </template>
      <p v-if="trafficError" class="error" role="alert">{{ trafficError }}</p>
      <div class="traffic-summary-grid">
        <article class="traffic-summary-card">
          <span>{{ t('adminUsers.trafficModal.summary.total30d') }}</span>
          <strong>{{ formatBytes(trafficTotal30d) }}</strong>
        </article>
        <article class="traffic-summary-card">
          <span>{{ t('adminUsers.trafficModal.summary.dailyPeak') }}</span>
          <strong>{{ formatBytes(dailyPeak?.traffic || 0) }}</strong>
          <small>{{ dailyPeak?.date || '-' }}</small>
        </article>
        <article class="traffic-summary-card">
          <span>{{ t('adminUsers.trafficModal.summary.hourlyPeak') }}</span>
          <strong>{{ formatBytes(hourlyPeak?.traffic || 0) }}</strong>
          <small>{{ hourlyPeak ? formatHourTs(hourlyPeak.hour_ts) : '-' }}</small>
        </article>
      </div>
      <section class="traffic-section">
        <h4 id="user-traffic-dailyTitle">{{ t('adminUsers.trafficModal.dailyTitle') }}</h4>
        <div class="traffic-table-wrap" tabindex="0" role="region" aria-labelledby="user-traffic-dailyTitle">
          <table class="data-table compact-table">
            <thead>
              <tr><th>{{ t('adminUsers.trafficModal.table.date') }}</th><th>{{ t('adminUsers.trafficModal.table.traffic') }}</th></tr>
            </thead>
            <tbody>
              <tr v-if="trafficLoading"><td colspan="2" class="empty-row">{{ t('common.states.loading') }}</td></tr>
              <tr v-for="item in dailyTrafficRows" :key="item.date">
                <td>{{ item.date }}</td>
                <td>{{ formatBytes(item.traffic) }}</td>
              </tr>
              <tr v-if="!trafficLoading && dailyTrafficRows.length === 0"><td colspan="2" class="empty-row">{{ t('adminUsers.trafficModal.empty') }}</td></tr>
            </tbody>
          </table>
        </div>
      </section>
      <section class="traffic-section">
        <h4 id="user-traffic-hourlyTitle">{{ t('adminUsers.trafficModal.hourlyTitle') }}</h4>
        <div class="traffic-table-wrap" tabindex="0" role="region" aria-labelledby="user-traffic-hourlyTitle">
          <table class="data-table compact-table">
            <thead>
              <tr><th>{{ t('adminUsers.trafficModal.table.hour') }}</th><th>{{ t('adminUsers.trafficModal.table.traffic') }}</th></tr>
            </thead>
            <tbody>
              <tr v-if="trafficLoading"><td colspan="2" class="empty-row">{{ t('common.states.loading') }}</td></tr>
              <tr v-for="item in hourlyTrafficRows" :key="item.hour_ts">
                <td>{{ formatHourTs(item.hour_ts) }}</td>
                <td>{{ formatBytes(item.traffic) }}</td>
              </tr>
              <tr v-if="!trafficLoading && hourlyTrafficRows.length === 0"><td colspan="2" class="empty-row">{{ t('adminUsers.trafficModal.empty') }}</td></tr>
            </tbody>
          </table>
        </div>
      </section>
    </UiSheet>

    <UiDialog v-model:open="showCopyManual" size="sm" :title="t('adminUsers.copyDialog.title')" :description="t('adminUsers.copyDialog.description')">
      <UiCopyField :value="copyManualUrl" :label="t('adminUsers.copyDialog.label')" stacked />
      <template #footer="{ close }">
        <UiButton @click="close">{{ t('common.actions.close') }}</UiButton>
      </template>
    </UiDialog>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { useEdition } from '@/composables/useEdition'
import { useRouteIntent } from '@/composables/useRouteIntent'
import {
  assignAdminUserTunnel, banUser, createUser, getAdminUserTunnelList, getForwardTunnels, getSpeedLimitList,
  getAdminUser, getTrafficHourly,
  getSubscriptionSettings,
  getUserList, getUserStats, removeAdminUserTunnel, resetUserSubscribe, resetUserTraffic, resetUserTunnelTraffic,
  unbanUser, updateAdminUserTunnel, updateUser
} from '@/api/admin'
import { getSubscriptionGroups } from '@/api/admin'
import { UiButton, UiConfirmDialog, UiCopyField, UiDialog, UiSheet, copyText, useConfirm, useToast } from '@/ui'

const { t, formatDate: i18nFormatDate, formatDateTime: i18nFormatDateTime } = useAppI18n()
// The balance is commercial: the community edition hides the field and
// sends the stored value back unchanged.
const { isCommercial } = useEdition()
const users = ref([])
const stats = ref({})
const subscriptionGroups = ref([])
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
// The command palette opens this page with ?email= (a user search result)
// or ?create=1 (添加用户).
const routeIntent = useRouteIntent(['email', 'create'], intent => {
  if (intent.email !== undefined) {
    filters.value.email = intent.email
    page.value = 1
    fetchUsers()
  }
  if (intent.create === '1') showCreateModal.value = true
})
const filters = ref({ email: routeIntent.email || '', status: '' })
const showEditModal = ref(false)
const editingUser = ref({})
const editSaving = ref(false)
const editError = ref('')
const showCreateModal = ref(routeIntent.create === '1')
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
const tunnelFormError = ref('')
const showResetFlowModal = ref(false)
const resetFlowLoading = ref(false)
const resetFlowTarget = ref(null)
const resetFlowTitle = ref('')
const resetFlowMessage = ref('')
const resetFlowUsedFlow = ref('')
const resetFlowQuota = ref('')
const resetFlowError = ref('')
const showTrafficModal = ref(false)
const trafficUser = ref(null)
const trafficLoading = ref(false)
const trafficError = ref('')
const hourlyTrafficRows = ref([])
const showCopyManual = ref(false)
const copyManualUrl = ref('')
const newTunnelForm = () => ({ tunnelId: '', flow: 0, num: 0, expTime: '', flowResetTime: 0, speedId: null, status: 1 })
const tunnelForm = ref(newTunnelForm())

const totalPages = computed(() => Math.ceil(total.value / pageSize.value) || 1)
const assignedTunnelIds = computed(() => new Set(userTunnels.value.map(item => Number(item?.tunnelId || 0)).filter(id => id > 0)))
const availableTunnelOptions = computed(() => editingTunnelId.value ? tunnelOptions.value : tunnelOptions.value.filter(item => !assignedTunnelIds.value.has(Number(item?.id || 0))))
const availableSpeedLimitOptions = computed(() => {
  const targetTunnelId = Number(tunnelForm.value.tunnelId || 0)
  return targetTunnelId ? speedLimitOptions.value.filter(item => Number(item?.tunnelId || 0) === targetTunnelId) : []
})
const trafficTotal30d = computed(() => hourlyTrafficRows.value.reduce((sum, item) => sum + Number(item?.traffic || 0), 0))
const dailyTrafficRows = computed(() => {
  const buckets = new Map()
  for (const item of hourlyTrafficRows.value) {
    const date = formatDayTs(Number(item?.hour_ts || 0))
    if (!date) continue
    buckets.set(date, (buckets.get(date) || 0) + Number(item?.traffic || 0))
  }
  return Array.from(buckets.entries())
    .map(([date, traffic]) => ({ date, traffic }))
    .sort((a, b) => b.date.localeCompare(a.date))
    .slice(0, 30)
})
const dailyPeak = computed(() => dailyTrafficRows.value.reduce((peak, item) => (item.traffic > (peak?.traffic || 0) ? item : peak), null))
const hourlyPeak = computed(() => hourlyTrafficRows.value.reduce((peak, item) => (Number(item?.traffic || 0) > Number(peak?.traffic || 0) ? item : peak), null))
const toast = useToast()
const confirm = useConfirm()

const assertCompatSuccess = (res, fallback = t('adminUsers.messages.actionFailed')) => {
  if (res && typeof res.code === 'number' && res.code !== 0) throw new Error(res.msg || fallback)
  return res
}
const readApiError = (err, fallback = t('adminUsers.messages.actionFailed')) => (
  err?.response?.data?.msg ||
  err?.response?.data?.message ||
  err?.message ||
  fallback
)
const getResData = (res, fallback = t('adminUsers.messages.actionFailed')) => {
  if (!res) return null
  if (typeof res.code === 'number') {
    if (res.code !== 0) throw new Error(res.msg || fallback)
    return res.data
  }
  if (
    res.data &&
    typeof res.data === 'object' &&
    Object.prototype.hasOwnProperty.call(res.data, 'data')
  ) {
    return res.data.data
  }
  return res.data ?? res
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
    assertCompatSuccess(await createUser(newUser.value), t('adminUsers.messages.createFailed'))
    showCreateModal.value = false
    newUser.value = newBlankUser()
    fetchUsers()
    fetchStats()
    toast.success(t('adminUsers.messages.userCreated'))
  } catch (err) {
    createError.value = readApiError(err, t('adminUsers.messages.createFailed'))
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
// fetchUserDetail reads one user's whole row (remark, subscription token)
// from the user detail; the list carries only the account and subscription
// summary.
const fetchUserDetail = async (id) => {
  const detail = getResData(await getAdminUser(id), t('adminUsers.messages.fetchUserFailed'))
  return detail && typeof detail === 'object' ? detail : {}
}
const editUser = async (user) => {
  let detail = {}
  try {
    detail = await fetchUserDetail(user.id)
  } catch (err) {
    // The list row still fills the form; a remark not loaded is not sent.
    console.error(t('adminUsers.messages.fetchUserFailed'), err)
  }
  const merged = { ...user, ...detail }
  editingUser.value = {
    ...merged,
    flowResetTime: Number(merged?.flowResetTime || 0),
    speed_limit: Number(merged?.speed_limit || 0),
    device_limit: Number(merged?.device_limit || 0)
  }
  editError.value = ''
  showEditModal.value = true
}
const saveUser = async () => {
  if (editSaving.value) return
  editSaving.value = true
  editError.value = ''
  try {
    assertCompatSuccess(await updateUser(editingUser.value.id, {
      email: editingUser.value.email, balance: editingUser.value.balance, transfer_enable: editingUser.value.transfer_enable,
      speed_limit: Number(editingUser.value.speed_limit || 0), device_limit: Number(editingUser.value.device_limit || 0),
      expired_at: editingUser.value.expired_at, flowResetTime: Number(editingUser.value.flowResetTime || 0), remark_content: editingUser.value.remark_content,
      group_id: editingUser.value.group_id || null
    }), t('adminUsers.messages.actionFailed'))
    showEditModal.value = false
    toast.success(t('adminUsers.messages.userSaved', { email: editingUser.value.email || '' }))
    fetchUsers()
  } catch (err) {
    editError.value = t('adminUsers.messages.saveFailed', { message: readApiError(err) })
  } finally {
    editSaving.value = false
  }
}

// Ban and unban undo each other, so neither asks first: the toast offers
// 撤销 for 5 s instead (redesign plan §9).
const setBanned = async (user, banned) => {
  try {
    assertCompatSuccess(await (banned ? banUser(user.id) : unbanUser(user.id)))
    fetchUsers()
    fetchStats()
    return true
  } catch (err) {
    toast.error(readApiError(err))
    return false
  }
}
const handleBan = async (user) => {
  if (!(await setBanned(user, true))) return
  toast.success(t('adminUsers.messages.userBanned', { email: user.email }), { undo: () => setBanned(user, false) })
}
const handleUnban = async (user) => {
  if (!(await setBanned(user, false))) return
  toast.success(t('adminUsers.messages.userUnbanned', { email: user.email }), { undo: () => setBanned(user, true) })
}

const openResetUserDialog = (user) => {
  resetFlowTarget.value = { type: 'user', id: user.id }
  resetFlowTitle.value = t('adminUsers.resetFlow.userTitle', { email: user.email })
  resetFlowMessage.value = t('adminUsers.resetFlow.userMessage', { email: user.email })
  resetFlowUsedFlow.value = formatBytes((user.u || 0) + (user.d || 0))
  resetFlowQuota.value = user.transfer_enable ? formatBytes(user.transfer_enable) : ''
  resetFlowError.value = ''
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

// 复制到剪贴板：copyText 在 HTTP + IP 直连时用 execCommand('copy') 兜底。
const copyToClipboard = text => copyText(text)

const readSubscriptionSettings = (res) => {
  const payload = getResData(res) || {}
  if (!payload || typeof payload !== 'object') return { subscribe_path: '/s', subscribe_domains: [] }
  return {
    subscribe_path: payload.subscribe_path || '/s',
    subscribe_domains: Array.isArray(payload.subscribe_domains) ? payload.subscribe_domains : []
  }
}

const copySubscribe = async (user) => {
  let token = ''
  try {
    token = (await fetchUserDetail(user.id)).token || ''
  } catch (err) {
    toast.error(readApiError(err, t('adminUsers.messages.fetchUserFailed')))
    return
  }
  if (!token) {
    toast.error(t('adminUsers.messages.noToken'))
    return
  }
  const url = buildSubscribeUrl({ token })
  if (await copyToClipboard(url)) {
    toast.success(t('adminUsers.messages.subscribeCopied'))
  } else {
    // 兜底也失败：在对话框里显示链接，让管理员手动复制。
    copyManualUrl.value = url
    showCopyManual.value = true
  }
}

const loadSubscriptionSettings = async () => {
  try {
    const res = await getSubscriptionSettings()
    subscriptionSettings.value = readSubscriptionSettings(res)
  } catch {
    subscriptionSettings.value = { subscribe_path: '/s', subscribe_domains: [] }
  }
}

const resetSubscribe = async (user) => {
  const confirmed = await confirm({
    title: t('adminUsers.confirm.resetSubscribeTitle', { email: user.email }),
    message: t('adminUsers.confirm.resetSubscribeMessage'),
    confirmLabel: t('adminUsers.confirm.resetSubscribeAction'),
    tone: 'danger',
    onConfirm: async () => {
      assertCompatSuccess(await resetUserSubscribe(user.id), t('adminUsers.messages.resetSubscribeFailed'))
    }
  })
  if (!confirmed) return
  toast.success(t('adminUsers.messages.resetSubscribeSuccess'))
  await fetchUsers()
}

const normalizeSpeedLimitList = items => Array.isArray(items) ? items.map(item => {
  const id = Number(item?.id || 0)
  if (!Number.isFinite(id) || id <= 0) return null
  return { id, name: item?.name || `Rule-${id}`, speed: Number(item?.speed || 0), tunnelId: Number(item?.tunnelId ?? item?.tunnel_id ?? 0) }
}).filter(Boolean) : []

const loadTunnelOptions = async () => {
  try { tunnelOptions.value = getResData(await getForwardTunnels(), t('adminUsers.messages.fetchTunnelListFailed')) || [] } catch (err) {
    toast.error(err.response?.data?.msg || err.response?.data?.message || err.message || t('adminUsers.messages.fetchTunnelListFailed')); tunnelOptions.value = []
  }
}
const loadSpeedLimitOptions = async () => {
  speedLimitLoading.value = true
  try {
    const payload = getResData(await getSpeedLimitList(), t('adminUsers.messages.fetchSpeedLimitFailed'))
    const list = Array.isArray(payload) ? payload : (payload?.list || [])
    speedLimitOptions.value = normalizeSpeedLimitList(list)
  } catch (err) {
    toast.error(err.response?.data?.msg || err.response?.data?.message || err.message || t('adminUsers.messages.fetchSpeedLimitFailed')); speedLimitOptions.value = []
  } finally {
    speedLimitLoading.value = false
  }
}
const loadUserTunnels = async (userId) => {
  tunnelListLoading.value = true
  try { userTunnels.value = getResData(await getAdminUserTunnelList({ userId }), t('adminUsers.messages.fetchTunnelGrantFailed')) || [] } catch (err) {
    toast.error(err.response?.data?.msg || err.response?.data?.message || err.message || t('adminUsers.messages.fetchTunnelGrantFailed'))
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
const resetTunnelForm = () => { editingTunnelId.value = null; tunnelForm.value = newTunnelForm(); tunnelFormError.value = '' }

const submitTunnelForm = async () => {
  if (!tunnelUser.value) return
  tunnelFormError.value = ''
  if (!tunnelForm.value.tunnelId) return (tunnelFormError.value = t('adminUsers.messages.selectTunnelFirst'))
  if (!editingTunnelId.value && assignedTunnelIds.value.has(Number(tunnelForm.value.tunnelId))) return (tunnelFormError.value = t('adminUsers.messages.tunnelAlreadyAssigned'))
  tunnelLoading.value = true
  try {
    const payload = {
      tunnelId: Number(tunnelForm.value.tunnelId), flow: Number(tunnelForm.value.flow || 0), num: Number(tunnelForm.value.num || 0),
      expTime: fromDateTimeLocal(tunnelForm.value.expTime), flowResetTime: Number(tunnelForm.value.flowResetTime || 0),
      speedId: normalizeSpeedId(tunnelForm.value.speedId, tunnelForm.value.tunnelId), status: Number(tunnelForm.value.status || 1)
    }
    if (editingTunnelId.value) {
      assertCompatSuccess(await updateAdminUserTunnel({ id: Number(editingTunnelId.value), ...payload }), t('adminUsers.messages.grantUpdateFailed'))
      toast.success(t('adminUsers.messages.grantUpdated'))
    } else {
      assertCompatSuccess(await assignAdminUserTunnel({ userId: Number(tunnelUser.value.id), ...payload }), t('adminUsers.messages.grantCreateFailed'))
      toast.success(t('adminUsers.messages.grantCreated'))
    }
    await loadUserTunnels(tunnelUser.value.id)
    resetTunnelForm()
  } catch (err) {
    tunnelFormError.value = err.response?.data?.msg || err.response?.data?.message || t('adminUsers.messages.grantActionFailed')
  } finally {
    tunnelLoading.value = false
  }
}
const editTunnelGrant = (item) => {
  editingTunnelId.value = item.id
  tunnelFormError.value = ''
  tunnelForm.value = { tunnelId: item.tunnelId || '', flow: item.flow ?? 0, num: item.num ?? 0, expTime: toDateTimeLocal(item.expTime), flowResetTime: item.flowResetTime ?? 0, speedId: normalizeSpeedId(item.speedId ?? null, item.tunnelId || ''), status: item.status ?? 1 }
}
const removeTunnelGrant = async (item) => {
  const confirmed = await confirm({
    title: t('adminUsers.confirm.deleteGrantTitle', { id: item.id }),
    message: t('adminUsers.confirm.deleteGrantMessage', { tunnel: item.tunnelName || item.tunnelId || item.id }),
    confirmLabel: t('adminUsers.confirm.deleteGrantAction'),
    tone: 'danger',
    onConfirm: async () => {
      try {
        assertCompatSuccess(await removeAdminUserTunnel({ id: item.id }), t('adminUsers.messages.grantDeleteFailed'))
      } catch (err) {
        throw new Error(err.response?.data?.msg || err.response?.data?.message || err.message || t('adminUsers.messages.grantDeleteFailed'))
      }
    }
  })
  if (!confirmed) return
  toast.success(t('adminUsers.messages.grantDeleted', { id: item.id }))
  if (tunnelUser.value) await loadUserTunnels(tunnelUser.value.id)
  if (editingTunnelId.value === item.id) resetTunnelForm()
}
const calculateTunnelUsedFlow = item => Number(item?.inFlow || 0) + Number(item?.outFlow || 0)

const openResetTunnelDialog = (item) => {
  if (!item?.id) return
  resetFlowTarget.value = { type: 'tunnel', id: item.id }
  resetFlowTitle.value = t('adminUsers.resetFlow.tunnelTitle', { id: item.id })
  resetFlowMessage.value = t('adminUsers.resetFlow.tunnelMessage', { id: item.id })
  resetFlowUsedFlow.value = formatBytes(calculateTunnelUsedFlow(item))
  resetFlowQuota.value = Number(item?.flow || 0) > 0 ? `${item.flow} GB` : ''
  resetFlowError.value = ''
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
  resetFlowError.value = ''
}
const openTrafficModal = async (user) => {
  trafficUser.value = user
  hourlyTrafficRows.value = []
  trafficError.value = ''
  showTrafficModal.value = true
  await loadTrafficDetail()
}
const closeTrafficModal = () => {
  if (trafficLoading.value) return
  showTrafficModal.value = false
  trafficUser.value = null
  hourlyTrafficRows.value = []
  trafficError.value = ''
}
const loadTrafficDetail = async () => {
  if (!trafficUser.value?.id) return
  trafficLoading.value = true
  trafficError.value = ''
  try {
    const res = await getTrafficHourly(720, trafficUser.value.id)
    const payload = getResData(res, t('adminUsers.trafficModal.fetchFailed'))
    const list = Array.isArray(payload) ? payload : []
    hourlyTrafficRows.value = list
      .map(item => ({ hour_ts: Number(item?.hour_ts || 0), traffic: Number(item?.traffic || 0) }))
      .filter(item => item.hour_ts > 0)
      .sort((a, b) => b.hour_ts - a.hour_ts)
  } catch (err) {
    trafficError.value = err.response?.data?.message || err.message || t('adminUsers.trafficModal.fetchFailed')
    hourlyTrafficRows.value = []
  } finally {
    trafficLoading.value = false
  }
}
const confirmResetFlow = async () => {
  if (!resetFlowTarget.value?.id) return
  resetFlowLoading.value = true
  resetFlowError.value = ''
  try {
    if (resetFlowTarget.value.type === 'user') {
      assertCompatSuccess(await resetUserTraffic(resetFlowTarget.value.id), t('adminUsers.messages.resetFailed'))
      await fetchUsers()
      toast.success(t('adminUsers.messages.userFlowReset'))
    } else {
      assertCompatSuccess(await resetUserTunnelTraffic(resetFlowTarget.value.id), t('adminUsers.messages.resetFailed'))
      if (tunnelUser.value) await loadUserTunnels(tunnelUser.value.id)
      toast.success(t('adminUsers.messages.tunnelFlowReset'))
    }
    closeResetFlowModal(true)
  } catch (err) {
    resetFlowError.value = err.response?.data?.msg || err.response?.data?.message || err.message || t('adminUsers.messages.resetFailed')
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
const formatDayTs = (timestamp) => {
  if (!timestamp) return ''
  const date = new Date(timestamp * 1000)
  if (Number.isNaN(date.getTime())) return ''
  return i18nFormatDate(date) || date.toISOString().slice(0, 10)
}
const formatHourTs = (timestamp) => {
  if (!timestamp) return '-'
  const date = new Date(timestamp * 1000)
  if (Number.isNaN(date.getTime())) return '-'
  return i18nFormatDateTime(date) || date.toLocaleString()
}

onMounted(() => { fetchUsers(); fetchStats(); loadSubscriptionGroups(); loadSubscriptionSettings() })

const loadSubscriptionGroups = async () => {
  try {
    const res = await getSubscriptionGroups()
    const payload = getResData(res) || []
    subscriptionGroups.value = Array.isArray(payload) ? payload : []
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

.dialog-form {
  display: grid;
  gap: var(--space-3);
}

.dialog-section {
  display: grid;
  gap: var(--space-3);
}

.dialog-section h4 {
  margin: 0;
}

.grid-full {
  grid-column: 1 / -1;
}

.reset-flow-facts {
  display: grid;
  gap: var(--space-1);
  margin: 0;
}

.reset-flow-facts div {
  display: flex;
  gap: var(--space-2);
}

.reset-flow-facts dt {
  color: var(--label-2);
}

.reset-flow-facts dd {
  margin: 0;
  font-weight: var(--weight-semibold);
}

.traffic-summary-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}

.traffic-summary-card {
  border: 1px solid var(--border-color);
  border-radius: 8px;
  padding: 12px;
  display: grid;
  gap: 4px;
}

.traffic-summary-card span,
.traffic-summary-card small {
  color: var(--text-secondary);
  font-size: 12px;
}

.traffic-summary-card strong {
  font-size: 20px;
}

.traffic-section h4 {
  margin: 0 0 var(--space-2);
}

.traffic-table-wrap:focus-visible {
  outline: var(--focus-ring);
  outline-offset: var(--focus-ring-offset);
}

.traffic-table-wrap {
  max-height: 420px;
  overflow: auto;
  border: 1px solid var(--border-color);
  border-radius: 8px;
}

.compact-table th,
.compact-table td {
  padding: 10px;
}

.dialog-form label {
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

  .traffic-summary-grid {
    grid-template-columns: 1fr;
  }
}
</style>
