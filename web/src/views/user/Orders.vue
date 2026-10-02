<template>
  <div class="orders">
    <UiPageHeader :title="t('portal.orders.title')" :description="t('portal.orders.description')">
      <template v-if="state === 'ready' && orders.length" #actions>
        <UiButton :as="RouterLink" to="/user/plans">{{ t('portal.orders.browse') }}</UiButton>
      </template>
    </UiPageHeader>

    <div v-if="state === 'loading'">
      <UiSkeleton v-if="showSkeleton" variant="table-row" :rows="4" :columns="4" />
    </div>
    <LoadError v-else-if="state === 'failed'" :title="t('portal.orders.loadFailed')" :error="loadError" @retry="load" />
    <UiEmptyState v-else-if="!orders.length" :icon="Receipt" :title="t('portal.orders.empty')" :description="t('portal.orders.emptyHint')" heading-tag="h2" data-orders-empty>
      <template #actions>
        <UiButton :as="RouterLink" to="/user/plans" variant="primary">{{ t('portal.orders.browse') }}</UiButton>
      </template>
    </UiEmptyState>

    <ul v-else class="order-list" data-order-list>
      <li v-for="order in orders" :key="order.id" class="order-row">
        <button type="button" class="order-row__main" :aria-label="`${t('portal.orders.details')}: ${planName(order)} ${format.money(order.total_amount)}`" data-order-detail @click="openDetail(order)">
          <span class="order-row__plan">
            <span class="order-row__name">{{ planName(order) }}</span>
            <span class="order-row__meta">{{ periodLabel(order.period) }} · <span :title="format.dateTime(order.created_at)">{{ format.date(order.created_at) }}</span></span>
          </span>
          <span class="order-row__amount">{{ format.money(order.total_amount) }}</span>
          <UiBadge :tone="statusTone(order.status)" :label="statusLabel(order.status)" class="order-row__status" />
          <UiIcon :icon="ChevronRight" :size="20" class="order-row__chevron" />
        </button>
        <UiButton v-if="order.status === 0" variant="primary" size="sm" class="order-row__pay" data-order-pay @click="pay">{{ t('portal.orders.pay') }}</UiButton>
      </li>
    </ul>

    <UiSheet v-model:open="detail.open" :title="t('portal.orders.detail.title')" grouped data-order-sheet>
      <UiSkeleton v-if="detail.loading" variant="text" :lines="6" />
      <p v-else-if="detail.error" class="orders__error" role="alert">{{ detail.error }}</p>
      <UiGroupedList v-else-if="detail.order">
        <UiGroupedListRow :label="t('portal.orders.detail.tradeNo')">
          <template #value><span class="orders__mono">{{ detail.order.trade_no || '—' }}</span></template>
        </UiGroupedListRow>
        <UiGroupedListRow :label="t('portal.orders.detail.status')">
          <template #value><UiBadge :tone="statusTone(detail.order.status)" :label="statusLabel(detail.order.status)" /></template>
        </UiGroupedListRow>
        <UiGroupedListRow :label="t('portal.orders.detail.plan')" :value="planName(detail.order)" />
        <UiGroupedListRow :label="t('portal.orders.detail.period')" :value="periodLabel(detail.order.period)" />
        <UiGroupedListRow v-if="detail.order.discount_amount" :label="t('portal.orders.detail.subtotal')" :value="format.money(Number(detail.order.total_amount || 0) + Number(detail.order.discount_amount || 0))" />
        <UiGroupedListRow v-if="detail.order.discount_amount" :label="t('portal.orders.detail.discount')" :value="`−${format.money(detail.order.discount_amount)}`" />
        <UiGroupedListRow :label="t('portal.orders.detail.total')" :value="format.money(detail.order.total_amount)" />
        <UiGroupedListRow :label="t('portal.orders.detail.createdAt')" :value="format.dateTime(detail.order.created_at)" />
        <UiGroupedListRow v-if="detail.order.paid_at" :label="t('portal.orders.detail.paidAt')" :value="format.dateTime(detail.order.paid_at)" />
      </UiGroupedList>
      <template #footer="{ close }">
        <UiButton v-if="detail.order?.status === 0" variant="primary" @click="pay">{{ t('portal.orders.pay') }}</UiButton>
        <UiButton @click="close">{{ t('ui.actions.close') }}</UiButton>
      </template>
    </UiSheet>
  </div>
</template>

<script setup>
// 订单 (commercial edition only; plan §8.1): the orders as a list (plan,
// period, date, amount, status) and a details Sheet. Online payment is a
// separate line of work (plan §8.1), so 去支付 says how to pay for now.
// Data: GET /user/order (first 50), GET /user/order/:id.
import { computed, onMounted, reactive, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { ChevronRight, Receipt } from '@lucide/vue'
import { getOrderDetail, getOrders } from '@/api/user'
import { useAppI18n } from '@/composables/useAppI18n'
import { listOf, panelErrorMessage, unwrapPanel } from '@/utils/panelResponse'
import LoadError from '@/components/common/LoadError.vue'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiEmptyState from '@/ui/UiEmptyState.vue'
import UiGroupedList from '@/ui/UiGroupedList.vue'
import UiGroupedListRow from '@/ui/UiGroupedListRow.vue'
import UiIcon from '@/ui/UiIcon.vue'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import UiSheet from '@/ui/UiSheet.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import { useDelayedLoading } from '@/ui/composables/useDelayedLoading'
import { useFormat } from '@/ui/composables/useFormat'
import { useToast } from '@/ui/composables/useToast'

const { t } = useAppI18n()
const format = useFormat()
const toast = useToast()

const STATUS = ['pending', 'paid', 'cancelled', 'completed', 'discounted']
const TONES = ['warning', 'success', 'neutral', 'success', 'info']

const orders = ref([])
const state = ref('loading')
const loadError = ref(null)
const showSkeleton = useDelayedLoading(computed(() => state.value === 'loading'))

async function load() {
  state.value = 'loading'
  loadError.value = null
  try {
    orders.value = listOf(unwrapPanel(await getOrders({ page: 1, page_size: 50 })))
    state.value = 'ready'
  } catch (error) {
    loadError.value = error
    state.value = 'failed'
  }
}

function statusLabel(status) {
  return t(`portal.orders.status.${STATUS[status] || 'unknown'}`)
}

function statusTone(status) {
  return TONES[status] || 'neutral'
}

function periodLabel(period) {
  return period ? t(`portal.plans.periods.${period}`) : '—'
}

function planName(order) {
  return order?.plan?.name || t('portal.orders.unknownPlan')
}

const detail = reactive({ open: false, loading: false, order: null, error: '' })

async function openDetail(order) {
  Object.assign(detail, { open: true, loading: true, order: null, error: '' })
  try {
    detail.order = unwrapPanel(await getOrderDetail(order.id)) || order
  } catch (error) {
    detail.error = `${t('portal.orders.detailFailed')} ${panelErrorMessage(error)}`
  } finally {
    detail.loading = false
  }
}

function pay() {
  toast.info(t('portal.orders.payPending'))
}

onMounted(load)
</script>

<style scoped>
.orders {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
}

.order-list {
  overflow: hidden;
  border-radius: var(--radius-md);
  background: var(--bg-elevated);
  box-shadow: 0 0 0 0.5px var(--separator), var(--shadow-1);
  list-style: none;
}

.order-row {
  display: flex;
  gap: var(--space-3);
  align-items: center;
  padding-right: var(--space-4);
}

.order-row + .order-row {
  border-top: 1px solid var(--separator);
}

.order-row__main {
  display: grid;
  justify-content: stretch;
  border-radius: 0;
  font-weight: inherit;
  line-height: inherit;
  white-space: normal;
  user-select: auto;
  flex: 1;
  grid-template-columns: minmax(0, 1fr) auto 96px 20px;
  gap: var(--space-4);
  align-items: center;
  min-width: 0;
  padding: var(--space-4) 0 var(--space-4) var(--space-6);
  border: 0;
  background: transparent;
  color: var(--label-1);
  font: inherit;
  text-align: left;
  cursor: pointer;
}

.order-row__main:hover {
  background: var(--fill-1);
}

.order-row__main:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: -2px;
}

.order-row__plan {
  display: flex;
  flex-direction: column;
  gap: var(--space-0-5);
  min-width: 0;
}

.order-row__name {
  font-weight: var(--weight-semibold);
  overflow-wrap: anywhere;
}

.order-row__meta {
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.order-row__amount {
  font-weight: var(--weight-semibold);
  font-variant-numeric: tabular-nums;
  text-align: right;
}

.order-row__status {
  justify-self: start;
}

.order-row__chevron {
  color: var(--label-3);
}

.orders__mono {
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
  overflow-wrap: anywhere;
}

.orders__error {
  color: var(--danger);
}

@media (max-width: 833px) {
  .order-row {
    flex-wrap: wrap;
    padding: 0 0 var(--space-3);
  }

  .order-row__main {
    grid-template-columns: minmax(0, 1fr) auto;
    padding: var(--space-4) var(--space-4) var(--space-1);
  }

  .order-row__chevron {
    display: none;
  }

  .order-row__status {
    grid-column: 1;
  }

  .order-row__pay {
    margin-left: var(--space-4);
  }
}
</style>
