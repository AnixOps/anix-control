<template>
  <div class="list-page">
    <UiPageHeader :title="t('adminOrders.title')" :description="t('adminOrders.subtitle')" />

    <dl class="list-page__summary order-summary" data-test="order-summary">
      <div><dt>{{ t('adminOrders.stats.totalOrders') }}</dt><dd><strong>{{ format.number(stats.total_orders || 0) }}</strong></dd></div>
      <div><dt>{{ t('adminOrders.stats.pendingOrders') }}</dt><dd><strong>{{ format.number(stats.pending_orders || 0) }}</strong></dd></div>
      <div><dt>{{ t('adminOrders.stats.totalRevenue') }}</dt><dd><strong>{{ formatMoney(stats.total_revenue) }}</strong></dd></div>
      <div><dt>{{ t('adminOrders.stats.todayRevenue') }}</dt><dd><strong>{{ formatMoney(stats.today_revenue) }}</strong></dd></div>
    </dl>

    <UiDataTable
      :columns="columns"
      :rows="orders"
      :label="t('adminOrders.table.label')"
      :row-label="order => order.trade_no"
      storage-key="admin.orders"
      manual-pagination
      :page="page"
      :page-size="pageSize"
      :total="total"
      :loading="listLoading"
      :error="listError"
      :error-title="t('adminOrders.messages.fetchOrdersFailed')"
      :filtered="Boolean(filters.trade_no || filters.email || filters.status)"
      :empty-icon="Receipt"
      :empty-title="t('adminOrders.empty.title')"
      :empty-description="t('adminOrders.empty.description')"
      activatable
      :row-actions="orderActions"
      @update:page="goToPage"
      @row-activate="viewDetail"
      @retry="fetchOrders"
      @clear-filters="clearFilters"
    >
      <template #toolbar>
        <UiSearchField
          v-model="filters.trade_no"
          class="list-page__search"
          :label="t('adminOrders.filters.tradeNo')"
          data-test="order-search-trade-no"
          @update:model-value="scheduleSearch"
          @submit="searchNow"
        />
        <UiSearchField
          v-model="filters.email"
          class="list-page__search"
          :label="t('adminOrders.filters.email')"
          :shortcut="false"
          data-test="order-search-email"
          @update:model-value="scheduleSearch"
          @submit="searchNow"
        />
        <UiFilterChips v-model="filters.status" :label="t('adminOrders.filters.label')" :options="statusChips" />
      </template>
      <template #cell-trade_no="{ value }">
        <span class="order-no">{{ value }}</span>
      </template>
      <template #cell-status="{ row }">
        <UiBadge :tone="statusTone(row.status)" :label="getStatusText(row.status)" />
      </template>
    </UiDataTable>

    <UiSheet
      v-model:open="showDetailModal"
      grouped
      :title="t('adminOrders.detailModal.title')"
      :description="selectedOrder.trade_no || ''"
    >
      <template #header-actions>
        <UiBadge v-if="selectedOrder.trade_no" :tone="statusTone(selectedOrder.status)" :label="getStatusText(selectedOrder.status)" />
      </template>
      <div class="order-detail" data-test="order-detail">
        <UiGroupedList :title="t('adminOrders.detailModal.orderSection')">
          <UiGroupedListRow :label="t('adminOrders.detailModal.tradeNo')" :value="selectedOrder.trade_no || '—'" />
          <UiGroupedListRow :label="t('adminOrders.detailModal.userEmail')" :value="selectedOrder.user?.email || '—'" />
          <UiGroupedListRow :label="t('adminOrders.detailModal.plan')" :value="selectedOrder.plan?.name || '—'" />
          <UiGroupedListRow :label="t('adminOrders.detailModal.period')" :value="getPeriodText(selectedOrder.period)" />
          <UiGroupedListRow :label="t('adminOrders.detailModal.type')" :value="getTypeText(selectedOrder.type)" />
          <UiGroupedListRow :label="t('adminOrders.detailModal.amount')" :value="formatMoney(selectedOrder.total_amount)" />
        </UiGroupedList>
        <UiGroupedList :title="t('adminOrders.detailModal.paymentSection')">
          <UiGroupedListRow :label="t('adminOrders.detailModal.createdAt')" :value="formatTimestamp(selectedOrder.created_at)" />
          <UiGroupedListRow :label="t('adminOrders.detailModal.paidAt')" :value="selectedOrder.paid_at ? formatPaidAt(selectedOrder.paid_at) : '—'" />
          <UiGroupedListRow v-if="selectedOrder.callback_no" :label="t('adminOrders.detailModal.callbackNo')" :value="selectedOrder.callback_no" />
        </UiGroupedList>
      </div>
      <template v-if="Number(selectedOrder.status) === 0" #footer>
        <UiButton variant="danger-soft" data-test="order-detail-cancel" @click="handleCancel(selectedOrder)">{{ t('adminOrders.confirm.cancelAction') }}</UiButton>
        <UiButton variant="primary" data-test="order-detail-mark-paid" @click="handleMarkPaid(selectedOrder)">{{ t('adminOrders.actions.markPaid') }}</UiButton>
      </template>
    </UiSheet>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { CircleCheck, CircleX, Eye, Receipt } from '@lucide/vue'
import { cancelOrder, getOrderList, getOrderStats, markOrderPaid } from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiFilterChips from '@/ui/UiFilterChips.vue'
import UiGroupedList from '@/ui/UiGroupedList.vue'
import UiGroupedListRow from '@/ui/UiGroupedListRow.vue'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import UiSearchField from '@/ui/UiSearchField.vue'
import UiSheet from '@/ui/UiSheet.vue'
import { useConfirm } from '@/ui/composables/useConfirm'
import { useFormat } from '@/ui/composables/useFormat'
import { useToast } from '@/ui/composables/useToast'

const { t } = useAppI18n()
const format = useFormat()

const orders = ref([])
const stats = ref({})
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const filters = ref({
  trade_no: '',
  email: '',
  status: ''
})
const showDetailModal = ref(false)
const toast = useToast()
const confirm = useConfirm()
const selectedOrder = ref({})
const listLoading = ref(false)
const listError = ref(null)

const columns = computed(() => [
  { key: 'trade_no', label: t('adminOrders.table.tradeNo'), primary: true, hideable: false },
  { key: 'status', label: t('adminOrders.table.status'), secondary: true, sortable: true, sortValue: order => Number(order.status) },
  { key: 'user', label: t('adminOrders.table.user'), value: order => order.user?.email, sortable: true },
  { key: 'plan', label: t('adminOrders.table.plan'), value: order => order.plan?.name, sortable: true },
  { key: 'period', label: t('adminOrders.table.period'), value: order => getPeriodText(order.period), breakpoint: 'lg' },
  { key: 'total_amount', label: t('adminOrders.table.amount'), numeric: true, align: 'end', sortable: true, firstDirection: 'desc', format: value => formatMoney(value), nowrap: true },
  { key: 'created_at', label: t('adminOrders.table.createdAt'), numeric: true, nowrap: true, sortable: true, firstDirection: 'desc', format: value => formatTimestamp(value), sortValue: order => String(order.created_at || '') }
])
// The order list filters by status on the server (0 待支付 … 3 已完成).
const statusChips = computed(() => [
  { value: '0', label: t('adminOrders.status.pending'), count: stats.value.pending_orders },
  { value: '1', label: t('adminOrders.status.paid') },
  { value: '2', label: t('adminOrders.status.cancelled') },
  { value: '3', label: t('adminOrders.status.completed') }
])
const statusTone = (status) => ({ 0: 'warning', 1: 'info', 2: 'neutral', 3: 'success' }[Number(status)] || 'neutral')
const orderActions = order => [
  { key: 'details', label: t('common.actions.details'), icon: Eye, onSelect: () => viewDetail(order) },
  { key: 'mark-paid', label: t('adminOrders.actions.markPaid'), icon: CircleCheck, hidden: Number(order.status) !== 0, onSelect: () => handleMarkPaid(order) },
  { key: 'cancel', label: t('adminOrders.confirm.cancelAction'), icon: CircleX, danger: true, separatorBefore: true, hidden: Number(order.status) !== 0, onSelect: () => handleCancel(order) }
]

const goToPage = (value) => {
  page.value = value
  fetchOrders()
}
let searchTimer = null
const searchNow = () => {
  clearTimeout(searchTimer)
  page.value = 1
  fetchOrders()
}
const scheduleSearch = () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(searchNow, 300)
}
onBeforeUnmount(() => clearTimeout(searchTimer))
const clearFilters = () => {
  const statusChanged = filters.value.status !== ''
  filters.value.trade_no = ''
  filters.value.email = ''
  filters.value.status = ''
  // A status change searches through the watcher below.
  if (!statusChanged) searchNow()
}
watch(() => filters.value.status, () => searchNow())

const resolveApiError = (error, fallbackKey) => (
  error?.response?.data?.error ||
  error?.response?.data?.message ||
  error?.response?.data?.msg ||
  error?.message ||
  t(fallbackKey)
)

const ensureOrderSuccess = (res, fallbackKey) => {
  if (res && typeof res === 'object' && typeof res.code === 'number' && res.code !== 0) {
    throw new Error(res.msg || t(fallbackKey))
  }
  return res
}

const fetchOrders = async () => {
  listLoading.value = true
  try {
    const res = ensureOrderSuccess(
      await getOrderList({
        page: page.value,
        page_size: pageSize.value,
        trade_no: filters.value.trade_no,
        email: filters.value.email,
        status: filters.value.status || undefined
      }),
      'adminOrders.messages.fetchOrdersFailed'
    )
    const payload = readOrderPage(res)
    orders.value = payload.list || []
    total.value = payload.total || 0
    listError.value = null
    if (showDetailModal.value && selectedOrder.value.id) {
      selectedOrder.value = orders.value.find(order => order.id === selectedOrder.value.id) || selectedOrder.value
    }
  } catch (error) {
    listError.value = error
  } finally {
    listLoading.value = false
  }
}

const fetchStats = async () => {
  try {
    const res = ensureOrderSuccess(
      await getOrderStats(),
      'adminOrders.messages.fetchStatsFailed'
    )
    stats.value = readOrderStats(res)
  } catch (error) {
    console.error(t('adminOrders.messages.fetchStatsFailed'), error)
  }
}

const readOrderPayload = (res) => {
  if (!res || typeof res !== 'object') {
    return null
  }
  if (Object.prototype.hasOwnProperty.call(res, 'code')) {
    return res.data ?? null
  }
  if (
    res.data &&
    typeof res.data === 'object' &&
    Object.prototype.hasOwnProperty.call(res.data, 'data')
  ) {
    return res.data.data ?? null
  }
  return res.data ?? res
}

const readOrderPage = (res) => {
  const payload = readOrderPayload(res)
  return payload && typeof payload === 'object' ? payload : {}
}

const readOrderStats = (res) => {
  const payload = readOrderPayload(res)
  return payload && typeof payload === 'object' ? payload : {}
}

const viewDetail = (order) => {
  selectedOrder.value = order
  showDetailModal.value = true
}

// Marking paid and cancelling are final for the order: ask first and keep a
// failure inside the confirmation.
const handleMarkPaid = async (order) => {
  const confirmed = await confirm({
    title: t('adminOrders.confirm.markPaidTitle', { tradeNo: order.trade_no }),
    message: t('adminOrders.confirm.markPaidMessage', { amount: formatMoney(order.total_amount) }),
    confirmLabel: t('adminOrders.actions.markPaid'),
    onConfirm: async () => {
      try {
        ensureOrderSuccess(
          await markOrderPaid(order.id),
          'adminOrders.messages.markPaidFailedShort'
        )
      } catch (error) {
        throw new Error(resolveApiError(error, 'adminOrders.messages.markPaidFailedShort'))
      }
    }
  })
  if (!confirmed) return
  toast.success(t('adminOrders.messages.markPaidSuccess'))
  await fetchOrders()
  await fetchStats()
}

const handleCancel = async (order) => {
  const confirmed = await confirm({
    title: t('adminOrders.confirm.cancelTitle', { tradeNo: order.trade_no }),
    message: t('adminOrders.confirm.cancelMessage'),
    confirmLabel: t('adminOrders.confirm.cancelAction'),
    cancelLabel: t('adminOrders.confirm.keepOrder'),
    tone: 'danger',
    onConfirm: async () => {
      try {
        ensureOrderSuccess(
          await cancelOrder(order.id),
          'adminOrders.messages.cancelFailedShort'
        )
      } catch (error) {
        throw new Error(resolveApiError(error, 'adminOrders.messages.cancelFailedShort'))
      }
    }
  })
  if (!confirmed) return
  toast.success(t('adminOrders.messages.cancelSuccess', { tradeNo: order.trade_no }))
  await fetchOrders()
  await fetchStats()
}

const getStatusText = (status) => {
  switch (Number(status)) {
    case 0:
      return t('adminOrders.status.pending')
    case 1:
      return t('adminOrders.status.paid')
    case 2:
      return t('adminOrders.status.cancelled')
    case 3:
      return t('adminOrders.status.completed')
    default:
      return t('adminOrders.status.unknown')
  }
}

const getTypeText = (type) => {
  switch (Number(type)) {
    case 1:
      return t('adminOrders.types.new')
    case 2:
      return t('adminOrders.types.renew')
    case 3:
      return t('adminOrders.types.upgrade')
    case 4:
      return t('adminOrders.types.resetTraffic')
    default:
      return t('adminOrders.types.unknown')
  }
}

const getPeriodText = (period) => {
  switch (period) {
    case 'month':
      return t('common.periods.month')
    case 'quarter':
      return t('common.periods.quarter')
    case 'half_year':
      return t('common.periods.halfYear')
    case 'year':
      return t('common.periods.year')
    case 'two_year':
      return t('common.periods.twoYear')
    case 'three_year':
      return t('common.periods.threeYear')
    case 'onetime':
      return t('common.periods.onetime')
    default:
      return period || '—'
  }
}

const formatMoney = cents => format.money(Number(cents || 0))

const formatTimestamp = value => format.dateTime(value)

const formatPaidAt = (value) => {
  const numericValue = Number(value)
  if (!value) return '—'
  if (Number.isFinite(numericValue) && numericValue < 1e12) {
    return formatTimestamp(numericValue * 1000)
  }
  return formatTimestamp(value)
}

onMounted(() => {
  fetchOrders()
  fetchStats()
})
</script>

<style scoped>
.order-summary {
  gap: var(--space-2) var(--space-8);
}

.order-summary div {
  display: flex;
  gap: var(--space-2);
  align-items: baseline;
}

.order-summary dd {
  margin: 0;
}

.order-no {
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
}

.order-detail {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
}
</style>
