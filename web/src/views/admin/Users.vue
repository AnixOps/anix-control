<template>
  <div class="list-page">
    <UiPageHeader :title="t('adminUsers.title')" :description="t('adminUsers.subtitle')">
      <template #actions>
        <UiButton variant="primary" :icon="Plus" data-test="user-add-button" @click="showCreateModal = true">{{ t('adminUsers.actions.addUser') }}</UiButton>
      </template>
    </UiPageHeader>

    <UiDataTable
      v-model:selected="selectedIds"
      :columns="columns"
      :rows="visibleUsers"
      :label="t('adminUsers.table.label')"
      :row-label="user => user.email"
      storage-key="admin.users"
      manual-pagination
      :page="page"
      :page-size="pageSize"
      :total="statusFilter === 'exhausted' ? visibleUsers.length : total"
      :loading="listLoading"
      :error="listError"
      :error-title="t('adminUsers.messages.fetchUsersFailed')"
      :filtered="Boolean(filters.email || statusFilter)"
      :empty-icon="UsersIcon"
      :empty-title="t('adminUsers.empty.title')"
      :empty-description="t('adminUsers.empty.description')"
      selectable
      activatable
      manual-sort
      :sort="sortState"
      :card-fields="4"
      :row-actions="userActions"
      @update:sort="changeSort"
      @update:page="goToPage"
      @row-activate="openDetail"
      @retry="fetchUsers"
      @clear-filters="clearFilters"
    >
      <template #toolbar>
        <UiSearchField
          v-model="filters.email"
          class="list-page__search"
          :label="t('adminUsers.filters.searchEmail')"
          data-test="user-search"
          @update:model-value="scheduleSearch"
          @submit="searchNow"
        />
        <UiFilterChips v-model="statusFilter" :label="t('adminUsers.filters.label')" :options="statusChips" />
      </template>
      <template #empty-actions>
        <UiButton variant="primary" :icon="Plus" @click="showCreateModal = true">{{ t('adminUsers.actions.addUser') }}</UiButton>
      </template>
      <template #cell-email="{ row }">
        <span class="user-email">
          <span class="user-email__text">{{ row.email }}</span>
          <UiBadge v-if="row.is_admin === 1" tone="info" :dot="false" :label="t('adminUsers.labels.admin')" />
        </span>
      </template>
      <template #cell-status="{ row }">
        <UiBadge :tone="statusOf(row).tone" :label="statusOf(row).label" />
      </template>
      <template #cell-traffic="{ row }">
        <UiUsageBar :value="usedOf(row)" :max="Number(row.transfer_enable || 0)" :text="trafficText(row)" />
      </template>
      <template #cell-last_online="{ row }">
        <UserLastOnline :value="activity.lastOnlineOf(row.id)" :failed="activity.failed.value" />
      </template>
      <template #bulk-actions="{ rows: chosen }">
        <UiButton size="sm" :icon="Ban" :loading="bulkBusy" data-test="bulk-ban" @click="bulkSetBanned(chosen, true)">{{ t('adminUsers.actions.ban') }}</UiButton>
        <UiButton size="sm" :icon="ShieldCheck" :disabled="bulkBusy" data-test="bulk-unban" @click="bulkSetBanned(chosen, false)">{{ t('adminUsers.actions.unban') }}</UiButton>
        <UiButton size="sm" variant="danger-soft" :icon="RotateCcw" :disabled="bulkBusy" data-test="bulk-reset-traffic" @click="bulkResetTraffic(chosen)">{{ t('adminUsers.actions.resetTraffic') }}</UiButton>
      </template>
    </UiDataTable>
    <p v-if="statusFilter === 'exhausted'" class="list-page__note" data-test="user-exhausted-note">{{ t('adminUsers.filters.exhaustedHint') }}</p>

    <UiSheet
      :open="Boolean(detailUser)"
      size="md"
      grouped
      :title="detailUser?.email || ''"
      :description="detailUser ? t('adminUsers.detail.description', { id: detailUser.id, date: formatDateTime(detailUser.created_at) }) : ''"
      data-test="user-detail-sheet"
      @update:open="value => { if (!value) detailUser = null }"
    >
      <template #header-actions>
        <UiBadge v-if="detailUser" :tone="statusOf(detailUser).tone" :label="statusOf(detailUser).label" />
      </template>
      <template v-if="detailUser">
        <UiGroupedList :title="t('adminUsers.detail.subscription')">
          <UiGroupedListRow :label="isCommercial ? t('adminUsers.table.plan') : t('adminUsers.table.subscriptionTemplate')" :value="detailUser.plan?.name || '—'" />
          <UiGroupedListRow :label="t('adminUsers.table.traffic')">
            <UiUsageBar :value="usedOf(detailUser)" :max="Number(detailUser.transfer_enable || 0)" :text="trafficText(detailUser)" />
          </UiGroupedListRow>
          <UiGroupedListRow :label="t('adminUsers.table.expireAt')" :value="formatDate(detailUser.expired_at)" />
          <UiGroupedListRow :label="t('adminUsers.detail.flowReset')" :value="formatFlowResetDay(detailUser.flowResetTime)" />
          <UiGroupedListRow :label="t('adminUsers.table.limits')" :value="formatUserLimits(detailUser)" />
          <UiGroupedListRow v-if="isCommercial" :label="t('adminUsers.editModal.fields.balance')" :value="String(detailUser.balance ?? 0)" />
        </UiGroupedList>
        <UiGroupedList :title="t('adminUsers.detail.activity')">
          <UiGroupedListRow :label="t('adminUsers.table.lastOnline')" data-testid="user-last-online">
            <template #value><UserLastOnline :value="activity.lastOnlineOf(detailUser.id)" :failed="activity.failed.value" /></template>
          </UiGroupedListRow>
        </UiGroupedList>
        <UiGroupedList :title="t('adminUsers.detail.actions')">
          <UiGroupedListRow :label="t('adminUsers.actions.editUser')" @click="runFromDetail(editUser)" />
          <UiGroupedListRow :label="t('adminUsers.actions.viewTraffic')" @click="runFromDetail(openTrafficModal)" />
          <UiGroupedListRow :label="t('adminUsers.actions.copySubscribe')" @click="copySubscribe(detailUser)" />
        </UiGroupedList>
        <UiGroupedList :title="t('adminUsers.detail.danger')" :footer="t('adminUsers.detail.dangerFooter')">
          <UiGroupedListRow v-if="detailUser.banned === 0" :label="t('adminUsers.actions.ban')" @click="handleBan(detailUser)" />
          <UiGroupedListRow v-else :label="t('adminUsers.actions.unban')" @click="handleUnban(detailUser)" />
          <UiGroupedListRow :label="t('adminUsers.actions.resetTraffic')" @click="openResetUserDialog(detailUser)" />
          <UiGroupedListRow :label="t('adminUsers.actions.resetSubscribe')" @click="resetSubscribe(detailUser)" />
        </UiGroupedList>
      </template>
    </UiSheet>

    <UiDialog v-model:open="showEditModal" :title="t('adminUsers.editModal.title')" :description="editingUser.email || ''">
      <div class="form-grid">
        <UiTextField v-model="editingUser.email" class="form-grid__full" type="email" :label="t('adminUsers.editModal.fields.email')" />
        <UiTextField v-if="isCommercial" v-model.number="editingUser.balance" type="number" :label="t('adminUsers.editModal.fields.balance')" data-test="user-balance-field" />
        <UiTextField v-model.number="editingUser.transfer_enable" type="number" :label="t('adminUsers.editModal.fields.transfer')" :help="formatBytes(editingUser.transfer_enable || 0)" />
        <UiTextField v-model.number="editingUser.speed_limit" type="number" min="0" :label="t('adminUsers.editModal.fields.speedLimit')" data-test="user-speed-limit-input" />
        <UiTextField v-model.number="editingUser.device_limit" type="number" min="0" :label="t('adminUsers.editModal.fields.deviceLimit')" data-test="user-device-limit-input" />
        <UiSelect v-model="editGroup" :label="t('adminUsers.editModal.fields.groupId')" :options="groupOptions" />
        <UiTextField v-model.number="editingUser.flowResetTime" type="number" min="0" max="31" :label="t('adminUsers.editModal.fields.flowResetTime')" />
        <UiTextField v-model.number="editingUser.expired_at" class="form-grid__full" type="number" :label="t('adminUsers.editModal.fields.expiredAt')" :help="editingUser.expired_at ? formatDateTime(Number(editingUser.expired_at) * 1000) : t('adminUsers.labels.permanent')" />
        <UiTextarea v-model="editingUser.remark_content" class="form-grid__full" :rows="3" :label="t('adminUsers.editModal.fields.remark')" />
      </div>
      <p v-if="editError" class="form-error" role="alert" data-test="user-save-error">{{ editError }}</p>
      <template #footer="{ close }">
        <UiButton :disabled="editSaving" @click="close">{{ t('common.actions.cancel') }}</UiButton>
        <UiButton variant="primary" data-test="user-save-button" :loading="editSaving" @click="saveUser">{{ t('common.actions.save') }}</UiButton>
      </template>
    </UiDialog>

    <UiDialog v-model:open="showCreateModal" :title="t('adminUsers.createModal.title')">
      <div class="form-grid">
        <UiTextField v-model="newUser.email" class="form-grid__full" type="email" required :label="t('adminUsers.createModal.fields.email')" :placeholder="t('adminUsers.createModal.placeholders.email')" />
        <UiPasswordField v-model="newUser.password" class="form-grid__full" required :label="t('adminUsers.createModal.fields.password')" :help="t('adminUsers.createModal.placeholders.password')" />
        <UiSelect v-model="newUser.is_admin" :label="t('adminUsers.createModal.fields.userType')" :options="userTypeOptions" />
        <UiSelect v-model="newGroup" :label="t('adminUsers.createModal.fields.groupId')" :options="groupOptions" />
        <UiTextField v-model.number="newUser.transfer_enable" type="number" min="0" :label="t('adminUsers.createModal.fields.transferEnable')" :help="t('adminUsers.createModal.placeholders.transferEnable')" />
        <UiTextField v-model.number="newUser.speed_limit" type="number" min="0" :label="t('adminUsers.createModal.fields.speedLimit')" :help="t('adminUsers.createModal.placeholders.speedLimit')" data-test="new-user-speed-limit-input" />
        <UiTextField v-model.number="newUser.device_limit" type="number" min="0" :label="t('adminUsers.createModal.fields.deviceLimit')" :help="t('adminUsers.createModal.placeholders.deviceLimit')" data-test="new-user-device-limit-input" />
        <UiTextField v-model.number="newUser.flowResetTime" type="number" min="0" max="31" :label="t('adminUsers.createModal.fields.flowResetTime')" />
      </div>
      <p v-if="createError" class="form-error" role="alert">{{ createError }}</p>
      <template #footer="{ close }">
        <UiButton :disabled="createLoading" @click="close">{{ t('common.actions.cancel') }}</UiButton>
        <UiButton variant="primary" data-test="user-create-button" :loading="createLoading" @click="handleCreateUser">{{ t('adminUsers.createModal.submit') }}</UiButton>
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
        <UiButton size="sm" :icon="RotateCw" :loading="trafficLoading" @click="loadTrafficDetail">{{ t('adminUsers.trafficModal.refresh') }}</UiButton>
      </template>
      <UiErrorState v-if="trafficError" compact heading-tag="h3" :title="t('adminUsers.trafficModal.fetchFailed')" :error="trafficError" @retry="loadTrafficDetail" />
      <template v-else>
        <div class="traffic-summary">
          <div class="traffic-summary__item">
            <span>{{ t('adminUsers.trafficModal.summary.total30d') }}</span>
            <strong class="tabular-nums">{{ formatBytes(trafficTotal30d) }}</strong>
          </div>
          <div class="traffic-summary__item">
            <span>{{ t('adminUsers.trafficModal.summary.dailyPeak') }}</span>
            <strong class="tabular-nums">{{ formatBytes(dailyPeak?.traffic || 0) }}</strong>
            <small>{{ dailyPeak?.date || '—' }}</small>
          </div>
          <div class="traffic-summary__item">
            <span>{{ t('adminUsers.trafficModal.summary.hourlyPeak') }}</span>
            <strong class="tabular-nums">{{ formatBytes(hourlyPeak?.traffic || 0) }}</strong>
            <small>{{ hourlyPeak ? formatHourTs(hourlyPeak.hour_ts) : '—' }}</small>
          </div>
        </div>
        <section class="dialog-section" aria-labelledby="user-traffic-daily">
          <h3 id="user-traffic-daily" class="dialog-section__title">{{ t('adminUsers.trafficModal.dailyTitle') }}</h3>
          <UiDataTable
            :columns="dailyColumns"
            :rows="dailyTrafficRows"
            row-key="date"
            :label="t('adminUsers.trafficModal.dailyTitle')"
            :loading="trafficLoading"
            :empty-title="t('adminUsers.trafficModal.empty')"
            :settings="false"
            :sticky-header="false"
            :page-size="10"
            state-heading-tag="h4"
            default-density="compact"
          />
        </section>
        <section class="dialog-section" aria-labelledby="user-traffic-hourly">
          <h3 id="user-traffic-hourly" class="dialog-section__title">{{ t('adminUsers.trafficModal.hourlyTitle') }}</h3>
          <UiDataTable
            :columns="hourlyColumns"
            :rows="hourlyTrafficRows"
            row-key="hour_ts"
            :label="t('adminUsers.trafficModal.hourlyTitle')"
            :loading="trafficLoading"
            :empty-title="t('adminUsers.trafficModal.empty')"
            :settings="false"
            :sticky-header="false"
            :page-size="24"
            state-heading-tag="h4"
            default-density="compact"
          />
        </section>
      </template>
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
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Ban, Copy, Gauge, KeyRound, Pencil, Plus, RotateCcw, RotateCw, ShieldCheck, Users as UsersIcon } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { useEdition } from '@/composables/useEdition'
import { useListQuery } from '@/composables/useListQuery'
import { useBulkReport } from '@/composables/useBulkReport'
import { useRouteIntent } from '@/composables/useRouteIntent'
import {
  banUser, bulkUsers, createUser,
  getAdminUser, getTrafficHourly,
  getSubscriptionSettings,
  getUserList, getUserStats, resetUserSubscribe, resetUserTraffic,
  unbanUser, updateUser
} from '@/api/admin'
import { getSubscriptionGroups } from '@/api/admin'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiConfirmDialog from '@/ui/UiConfirmDialog.vue'
import UiCopyField from '@/ui/UiCopyField.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiDialog from '@/ui/UiDialog.vue'
import UiErrorState from '@/ui/UiErrorState.vue'
import UiFilterChips from '@/ui/UiFilterChips.vue'
import UiGroupedList from '@/ui/UiGroupedList.vue'
import UiGroupedListRow from '@/ui/UiGroupedListRow.vue'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import UiPasswordField from '@/ui/UiPasswordField.vue'
import UiSearchField from '@/ui/UiSearchField.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiSheet from '@/ui/UiSheet.vue'
import UiTextField from '@/ui/UiTextField.vue'
import UiTextarea from '@/ui/UiTextarea.vue'
import UiUsageBar from '@/ui/UiUsageBar.vue'
import { copyText } from '@/ui/composables/useClipboard'
import { useConfirm } from '@/ui/composables/useConfirm'
import { useFormat } from '@/ui/composables/useFormat'
import { useToast } from '@/ui/composables/useToast'
import { adminV4ErrorMessage, newBulkIdempotencyKey } from '@/utils/adminV4'
import { readBulkResult } from '@/utils/bulkResult'
import { SORT_ORDERS, createListSort } from '@/utils/listSort'
import UserLastOnline from './users/UserLastOnline.vue'
import { useUserActivity } from './users/useUserActivity'

const { t } = useAppI18n()
// The balance is commercial: the community edition hides the field and
// sends the stored value back unchanged.
const { isCommercial } = useEdition()
const users = ref([])
const stats = ref({})
const subscriptionGroups = ref([])
// The list state (q, status, page) lives in the URL query (plan §9).
const listQuery = useListQuery({ omit: ['email', 'create'] })
const USER_STATUS_FILTERS = ['active', 'expired', 'banned', 'exhausted']
// Sorting happens on the server (GET /admin/users sort and order), so a
// header sorts the whole list: only the columns the API can sort by are
// sortable, keyed by the table's column key. The status is derived and the
// plan's name is another table's, and sorting the 20 rows of one page says
// nothing about the list, so those two headers stay plain.
const USER_SORT = createListSort({ id: 'id', email: 'email', traffic: 'traffic', expired_at: 'expired_at', created_at: 'created_at' })
const sortState = ref(USER_SORT.fromQuery(
  listQuery.read('sort', { values: USER_SORT.values }),
  listQuery.read('order', { values: SORT_ORDERS })
))
const activity = useUserActivity()
const bulkReport = useBulkReport()
const page = ref(listQuery.readPage())
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
// 已封禁 / 已到期 / 正常 are the list's server-side status filter;
// 流量用尽 has no server filter, so it narrows the loaded page only.
const statusFilter = ref(listQuery.read('status', { values: USER_STATUS_FILTERS }))
const filters = ref({
  email: routeIntent.email || listQuery.read('q'),
  status: statusFilter.value === 'exhausted' ? '' : statusFilter.value
})
if (routeIntent.email) page.value = 1
const listLoading = ref(false)
const listError = ref(null)
const selectedIds = ref([])
const detailUser = ref(null)
const bulkBusy = ref(false)
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

const usedOf = user => Number(user?.u || 0) + Number(user?.d || 0)
const isExhausted = user => Number(user?.transfer_enable || 0) > 0 && usedOf(user) >= Number(user.transfer_enable)
const isExpired = user => Boolean(user?.expired_at) && user.expired_at < Date.now() / 1000
const visibleUsers = computed(() => (statusFilter.value === 'exhausted' ? users.value.filter(isExhausted) : users.value))
const statusOf = (user) => {
  if (user.banned === 1) return { tone: 'danger', label: t('adminUsers.status.banned') }
  if (isExpired(user)) return { tone: 'warning', label: t('adminUsers.status.expired') }
  if (isExhausted(user)) return { tone: 'warning', label: t('adminUsers.status.exhausted') }
  return { tone: 'success', label: t('adminUsers.status.active') }
}
const trafficText = user => (Number(user?.transfer_enable || 0) > 0
  ? `${formatBytes(usedOf(user))} / ${formatBytes(user.transfer_enable)}`
  : t('adminUsers.labels.trafficUnlimited', { used: formatBytes(usedOf(user)) }))

// Last online takes the room 注册时间 had at 1440 px (it is in the detail
// sheet's description, and one click away in the table settings).
const columns = computed(() => [
  { key: 'email', label: t('adminUsers.table.email'), primary: true },
  { key: 'status', label: t('adminUsers.table.status'), secondary: true },
  { key: 'plan', label: isCommercial.value ? t('adminUsers.table.plan') : t('adminUsers.table.subscriptionTemplate'), value: user => user.plan?.name },
  { key: 'traffic', label: t('adminUsers.table.traffic'), firstDirection: 'desc' },
  { key: 'expired_at', label: t('adminUsers.table.expireAt'), numeric: true, nowrap: true, format: value => formatDate(value) },
  { key: 'last_online', label: t('adminUsers.table.lastOnline'), nowrap: true, breakpoint: 'lg' },
  { key: 'limits', label: t('adminUsers.table.limits'), value: user => formatUserLimits(user), breakpoint: 'lg', hidden: true },
  { key: 'created_at', label: t('adminUsers.table.createdAt'), numeric: true, nowrap: true, format: value => formatDateTime(value), breakpoint: 'lg', hidden: true },
  { key: 'id', label: t('adminUsers.table.id'), numeric: true, hidden: true }
].map(column => (USER_SORT.isSortable(column.key) ? { ...column, sortable: true } : column)))
const statusChips = computed(() => [
  { value: 'active', label: t('adminUsers.status.active'), count: stats.value.active_users },
  { value: 'expired', label: t('adminUsers.status.expired'), count: stats.value.expired_users },
  { value: 'banned', label: t('adminUsers.status.banned'), count: stats.value.banned_users },
  { value: 'exhausted', label: t('adminUsers.status.exhausted') }
])
const groupOptions = computed(() => [
  { value: 'none', label: t('adminUsers.editModal.groupOptions.unassigned') },
  ...subscriptionGroups.value.map(group => ({ value: group.id, label: group.name }))
])
const userTypeOptions = computed(() => [
  { value: 0, label: t('adminUsers.createModal.userTypes.normal') },
  { value: 1, label: t('adminUsers.createModal.userTypes.admin') }
])
const editGroup = computed({
  get: () => editingUser.value.group_id ?? 'none',
  set: (value) => { editingUser.value.group_id = value === 'none' ? null : value }
})
const newGroup = computed({
  get: () => newUser.value.group_id ?? 'none',
  set: (value) => { newUser.value.group_id = value === 'none' ? null : value }
})

const userActions = user => [
  { key: 'edit', label: t('adminUsers.actions.editUser'), icon: Pencil, onSelect: () => editUser(user) },
  { key: 'traffic', label: t('adminUsers.actions.viewTraffic'), icon: Gauge, onSelect: () => openTrafficModal(user) },
  { key: 'copy', label: t('adminUsers.actions.copySubscribe'), icon: Copy, onSelect: () => copySubscribe(user) },
  user.banned === 0
    ? { key: 'ban', label: t('adminUsers.actions.ban'), icon: Ban, separatorBefore: true, onSelect: () => handleBan(user) }
    : { key: 'unban', label: t('adminUsers.actions.unban'), icon: ShieldCheck, separatorBefore: true, onSelect: () => handleUnban(user) },
  { key: 'reset-traffic', label: t('adminUsers.actions.resetTraffic'), icon: RotateCcw, danger: true, onSelect: () => openResetUserDialog(user) },
  { key: 'reset-subscribe', label: t('adminUsers.actions.resetSubscribe'), icon: KeyRound, danger: true, onSelect: () => resetSubscribe(user) }
]

const openDetail = (user) => { detailUser.value = user }
// A header click sorts the whole list on the server, from the first page.
const changeSort = (next) => {
  sortState.value = next
  page.value = 1
  selectedIds.value = []
  fetchUsers()
}
const goToPage = (value) => {
  page.value = value
  selectedIds.value = []
  fetchUsers()
}
const searchNow = () => {
  clearTimeout(searchTimer)
  page.value = 1
  fetchUsers()
}
let searchTimer = null
const scheduleSearch = () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(searchNow, 300)
}
onBeforeUnmount(() => clearTimeout(searchTimer))
const clearFilters = () => {
  filters.value.email = ''
  statusFilter.value = ''
  searchNow()
}
watch(statusFilter, (value, previous) => {
  const next = value === 'exhausted' ? '' : value
  const before = previous === 'exhausted' ? '' : previous
  filters.value.status = next
  if (next !== before) {
    page.value = 1
    fetchUsers()
  }
})

// Bulk ban / unban / reset traffic: one request for the whole selection
// (POST /api/v4/admin/users/bulk), answered per user. The users that could not
// be changed stay selected, and the toast says why (a count per cause) and
// offers 重试 for the ones worth another try.
const runUserBulk = async (action, ids, key = '') => (
  readBulkResult(await bulkUsers(action, ids, key ? { idempotencyKey: key } : {}), ids)
)
const afterBulk = (outcome) => {
  selectedIds.value = outcome.failedIds
  fetchUsers()
  fetchStats()
}

const bulkSetBanned = async (chosen, banned) => {
  const targets = chosen.filter(user => (banned ? user.banned === 0 : user.banned === 1))
  if (!targets.length || bulkBusy.value) return
  await applyBulkBan(banned, targets.map(user => user.id))
}
const applyBulkBan = async (banned, ids) => {
  const action = banned ? 'ban' : 'unban'
  bulkBusy.value = true
  let outcome
  try {
    outcome = await runUserBulk(action, ids)
  } catch (err) {
    toast.error(t('adminUsers.bulk.requestFailed', { message: adminV4ErrorMessage(err, t('adminUsers.messages.actionFailed')) }))
    return
  } finally {
    bulkBusy.value = false
  }
  afterBulk(outcome)
  bulkReport.report(outcome, {
    success: t(banned ? 'adminUsers.messages.bulkBanned' : 'adminUsers.messages.bulkUnbanned', { count: outcome.done.length }),
    partial: t(`adminUsers.bulk.${action}Partial`, { done: outcome.done.length, total: ids.length }),
    none: t(`adminUsers.bulk.${action}None`),
    undo: () => undoBulkBan(banned, outcome.done),
    retry: () => applyBulkBan(banned, outcome.retryable)
  })
}
// 撤销 is the inverse action for the users that were changed.
const undoBulkBan = async (banned, ids) => {
  try {
    const outcome = await runUserBulk(banned ? 'unban' : 'ban', ids)
    if (outcome.failed.length) toast.error(t('adminUsers.bulk.undoPartial', { count: outcome.failed.length }))
  } catch (err) {
    toast.error(t('adminUsers.bulk.requestFailed', { message: adminV4ErrorMessage(err, t('adminUsers.messages.actionFailed')) }))
  }
  fetchUsers()
  fetchStats()
}

// Reset traffic can't be undone: ask first, once for the selection. Every
// click of the button makes one Idempotency-Key and the server derives one
// per user from it, so asking again (a failed request, 重试 for the users that
// failed) never resets a counter twice.
const bulkResetTraffic = async (chosen) => {
  if (!chosen.length || bulkBusy.value) return
  const ids = chosen.map(user => user.id)
  const key = newBulkIdempotencyKey()
  let outcome = null
  const confirmed = await confirm({
    title: t('adminUsers.bulk.resetTitle', { count: ids.length }),
    message: t('adminUsers.bulk.resetMessage'),
    confirmLabel: t('adminUsers.resetFlow.confirmAction'),
    tone: 'danger',
    onConfirm: async () => {
      bulkBusy.value = true
      try {
        outcome = await runUserBulk('reset_traffic', ids, key)
      } catch (err) {
        throw new Error(adminV4ErrorMessage(err, t('adminUsers.messages.resetFailed')))
      } finally {
        bulkBusy.value = false
      }
    }
  })
  if (!confirmed || !outcome) return
  reportBulkReset(outcome, ids.length, key)
}
const reportBulkReset = (outcome, total, key) => {
  afterBulk(outcome)
  bulkReport.report(outcome, {
    success: t('adminUsers.bulk.resetDone', { count: outcome.done.length }),
    partial: t('adminUsers.bulk.resetPartial', { done: outcome.done.length, total }),
    none: t('adminUsers.bulk.resetNone'),
    retry: () => retryBulkReset(outcome.retryable, key)
  })
}
const retryBulkReset = async (ids, key) => {
  bulkBusy.value = true
  try {
    reportBulkReset(await runUserBulk('reset_traffic', ids, key), ids.length, key)
  } catch (err) {
    toast.error(t('adminUsers.bulk.requestFailed', { message: adminV4ErrorMessage(err, t('adminUsers.messages.resetFailed')) }))
  } finally {
    bulkBusy.value = false
  }
}

const dailyColumns = computed(() => [
  { key: 'date', label: t('adminUsers.trafficModal.table.date'), primary: true, sortable: true, firstDirection: 'desc' },
  { key: 'traffic', label: t('adminUsers.trafficModal.table.traffic'), numeric: true, align: 'end', sortable: true, firstDirection: 'desc', format: value => formatBytes(value) }
])
const hourlyColumns = computed(() => [
  { key: 'hour_ts', label: t('adminUsers.trafficModal.table.hour'), primary: true, sortable: true, firstDirection: 'desc', format: value => formatHourTs(value) },
  { key: 'traffic', label: t('adminUsers.trafficModal.table.traffic'), numeric: true, align: 'end', sortable: true, firstDirection: 'desc', format: value => formatBytes(value) }
])
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
const format = useFormat()

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
  listLoading.value = true
  listQuery.write({ q: filters.value.email.trim(), status: statusFilter.value, page: page.value, ...USER_SORT.toQuery(sortState.value) })
  try {
    const res = await getUserList({ page: page.value, page_size: pageSize.value, email: filters.value.email, status: filters.value.status, ...USER_SORT.toParams(sortState.value) })
    const payload = getResData(res) || {}
    users.value = payload.list || []
    total.value = payload.total || 0
    listError.value = null
    // When the users of this page were last seen: asked after the list is
    // shown, for this page's ids only.
    void activity.load(users.value.map(user => user.id))
    // The open detail follows the reloaded row (ban, edit, reset).
    if (detailUser.value) detailUser.value = users.value.find(item => item.id === detailUser.value.id) || detailUser.value
  } catch (err) {
    listError.value = err
    activity.reset()
  } finally {
    listLoading.value = false
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
const formatBytes = (bytes) => {
  if (!bytes) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let value = Number(bytes)
  while (value >= 1024 && i < units.length - 1) { value /= 1024; i += 1 }
  return `${value.toFixed(2)} ${units[i]}`
}
// One date style on the page (useFormat: 2026-11-30, 2026-11-30 14:05).
const formatDate = (timestamp) => (!timestamp ? t('adminUsers.labels.permanent') : format.date(Number(timestamp)))
const formatDateTime = (datetime) => format.dateTime(datetime)
const formatDayTs = (timestamp) => {
  if (!timestamp) return ''
  const date = new Date(timestamp * 1000)
  if (Number.isNaN(date.getTime())) return ''
  return format.date(date, { empty: '' }) || date.toISOString().slice(0, 10)
}
const formatHourTs = (timestamp) => {
  if (!timestamp) return '-'
  const date = new Date(timestamp * 1000)
  if (Number.isNaN(date.getTime())) return '-'
  return format.dateTime(date)
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
.user-email {
  display: inline-flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
  min-width: 0;
}

.user-email__text {
  overflow-wrap: anywhere;
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

.traffic-summary {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: var(--space-3);
}

.traffic-summary__item {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-md);
  background: var(--bg-grouped);
}

.traffic-summary__item span,
.traffic-summary__item small {
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

.traffic-summary__item strong {
  font-size: var(--type-title-3-size);
  font-weight: var(--weight-semibold);
}

@media (max-width: 639.98px) {
  .traffic-summary {
    grid-template-columns: 1fr;
  }
}
</style>
