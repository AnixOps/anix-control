<template>
  <div class="plans">
    <UiPageHeader :title="t('portal.plans.title')" :description="t('portal.plans.description')" />

    <div v-if="state === 'loading'">
      <div v-if="showSkeleton" class="plans__grid">
        <div v-for="index in 3" :key="index" class="plan-card"><UiSkeleton variant="card" /></div>
      </div>
    </div>
    <LoadError v-else-if="state === 'failed'" :title="t('portal.plans.loadFailed')" :error="loadError" @retry="load" />
    <UiEmptyState v-else-if="!plans.length" :icon="Package" :title="t('portal.plans.empty')" :description="t('portal.plans.emptyHint')" heading-tag="h2" data-plans-empty />

    <template v-else>
      <div v-if="periodOptions.length > 1" class="plans__periods">
        <UiSegmentedControl v-model="period" :options="periodOptions" :aria-label="t('portal.plans.period')" data-plans-period />
      </div>

      <ul class="plans__grid">
        <li v-for="plan in plans" :key="plan.id" class="plan-card" :class="{ 'is-unavailable': priceOf(plan, period) === null }" data-plan-card>
          <h2 class="plan-card__name">{{ plan.name }}</h2>
          <p class="plan-card__price">
            <template v-if="priceOf(plan, period) !== null">
              <span class="plan-card__amount">{{ format.money(priceOf(plan, period)) }}</span>
              <span class="plan-card__per">{{ t(`portal.plans.per.${period}`) }}</span>
            </template>
            <span v-else class="plan-card__none">{{ t('portal.plans.notOffered') }}</span>
          </p>
          <ul class="plan-card__features">
            <li v-if="plan.transfer_enable > 0">
              <UiIcon :icon="Gauge" :size="16" />
              <span>{{ t('portal.plans.traffic', { value: format.bytes(plan.transfer_enable * GIB, { precision: 0 }) }) }}</span>
            </li>
            <li v-if="plan.speed_limit">
              <UiIcon :icon="Zap" :size="16" />
              <span>{{ t('portal.plans.speed', { value: plan.speed_limit }) }}</span>
            </li>
            <li v-if="plan.device_limit">
              <UiIcon :icon="MonitorSmartphone" :size="16" />
              <span>{{ t('portal.plans.devices', { n: plan.device_limit }, plan.device_limit) }}</span>
            </li>
            <li v-for="(line, index) in contentLines(plan)" :key="index">
              <UiIcon :icon="Check" :size="16" />
              <span>{{ line }}</span>
            </li>
          </ul>
          <UiButton
            variant="primary"
            block
            :disabled="priceOf(plan, period) === null"
            :aria-label="`${t('portal.plans.buy')} ${plan.name}`"
            data-plan-buy
            @click="openCheckout(plan)"
          >
            {{ t('portal.plans.buy') }}
          </UiButton>
        </li>
      </ul>
    </template>

    <UiDialog
      :open="checkout.open"
      :title="t('portal.plans.checkout.title')"
      :dismissible="!checkout.busy"
      data-checkout
      @update:open="value => { if (!value) closeCheckout() }"
    >
      <form v-if="checkout.plan" id="checkout-form" class="checkout" novalidate @submit.prevent="submitOrder">
        <dl class="checkout__summary">
          <div>
            <dt>{{ t('portal.plans.checkout.plan') }}</dt>
            <dd>{{ checkout.plan.name }}</dd>
          </div>
        </dl>
        <UiField :label="t('portal.plans.checkout.period')" label-tag="span">
          <UiSegmentedControl
            v-model="checkout.period"
            :options="checkoutPeriods"
            :aria-label="t('portal.plans.checkout.period')"
            block
            data-checkout-period
          />
        </UiField>
        <div class="checkout__coupon">
          <UiTextField
            v-model.trim="checkout.coupon"
            :label="t('portal.plans.checkout.coupon')"
            :placeholder="t('portal.plans.checkout.couponPlaceholder')"
            :error="checkout.couponError"
            :readonly="Boolean(checkout.couponData)"
            autocomplete="off"
            class="checkout__coupon-field"
            data-coupon-code
            @keydown.enter.prevent="applyCoupon"
          />
          <UiButton v-if="!checkout.couponData" size="lg" :loading="checkout.checking" :disabled="!checkout.coupon" data-coupon-apply @click="applyCoupon">{{ t('portal.plans.checkout.apply') }}</UiButton>
          <UiButton v-else variant="tertiary" size="lg" data-coupon-remove @click="removeCoupon">{{ t('portal.plans.checkout.remove') }}</UiButton>
        </div>
        <p v-if="checkout.couponData" class="checkout__applied" role="status">
          <UiIcon :icon="BadgePercent" :size="16" />
          {{ t('portal.plans.checkout.couponApplied', { name: checkout.couponData.name || checkout.coupon }) }}
        </p>
        <dl class="checkout__totals">
          <div>
            <dt>{{ t('portal.plans.checkout.subtotal') }}</dt>
            <dd>{{ format.money(subtotal) }}</dd>
          </div>
          <div v-if="discount > 0">
            <dt>{{ t('portal.plans.checkout.discount') }}</dt>
            <dd>−{{ format.money(discount) }}</dd>
          </div>
          <div class="checkout__total">
            <dt>{{ t('portal.plans.checkout.total') }}</dt>
            <dd data-checkout-total>{{ format.money(total) }}</dd>
          </div>
        </dl>
        <p v-if="checkout.error" class="checkout__error" role="alert" data-checkout-error>{{ checkout.error }}</p>
      </form>
      <template #footer>
        <UiButton :disabled="checkout.busy" @click="closeCheckout">{{ t('portal.plans.checkout.back') }}</UiButton>
        <UiButton type="submit" form="checkout-form" variant="primary" :loading="checkout.busy" data-checkout-submit>{{ t('portal.plans.checkout.submit') }}</UiButton>
      </template>
    </UiDialog>
  </div>
</template>

<script setup>
// 套餐 (commercial edition only; plan §8.1): store-style plan cards with
// the price large, a billing-period segmented control, the plan's limits and
// its description lines, and a checkout dialog with a coupon. Data: GET
// /user/plan, POST /user/coupon/check, POST /user/order/save. Prices are in
// fen and go through useFormat().money().
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { BadgePercent, Check, Gauge, MonitorSmartphone, Package, Zap } from '@lucide/vue'
import { checkCoupon, getPlans, saveOrder } from '@/api/user'
import { useAppI18n } from '@/composables/useAppI18n'
import { listOf, panelErrorMessage, unwrapPanel } from '@/utils/panelResponse'
import LoadError from '@/components/common/LoadError.vue'
import UiButton from '@/ui/UiButton.vue'
import UiDialog from '@/ui/UiDialog.vue'
import UiEmptyState from '@/ui/UiEmptyState.vue'
import UiField from '@/ui/UiField.vue'
import UiIcon from '@/ui/UiIcon.vue'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import UiSegmentedControl from '@/ui/UiSegmentedControl.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import UiTextField from '@/ui/UiTextField.vue'
import { useDelayedLoading } from '@/ui/composables/useDelayedLoading'
import { useFormat } from '@/ui/composables/useFormat'
import { useToast } from '@/ui/composables/useToast'

const GIB = 1024 ** 3
const PERIODS = [
  ['month', 'month_price'],
  ['quarter', 'quarter_price'],
  ['half_year', 'half_year_price'],
  ['year', 'year_price'],
  ['two_year', 'two_year_price'],
  ['three_year', 'three_year_price'],
  ['onetime', 'onetime_price']
]

const { t } = useAppI18n()
const format = useFormat()
const toast = useToast()
const router = useRouter()

const plans = ref([])
const state = ref('loading')
const loadError = ref(null)
const showSkeleton = useDelayedLoading(computed(() => state.value === 'loading'))
const period = ref('month')

function priceOf(plan, key) {
  const field = PERIODS.find(([name]) => name === key)?.[1]
  const value = plan?.[field]
  return value === null || value === undefined || value === '' || Number(value) <= 0 ? null : Number(value)
}

const periodOptions = computed(() => PERIODS
  .filter(([key]) => plans.value.some(plan => priceOf(plan, key) !== null))
  .map(([key]) => ({ value: key, label: t(`portal.plans.periods.${key}`) })))

async function load() {
  state.value = 'loading'
  loadError.value = null
  try {
    plans.value = listOf(unwrapPanel(await getPlans()))
    period.value = periodOptions.value[0]?.value || 'month'
    state.value = 'ready'
  } catch (error) {
    loadError.value = error
    state.value = 'failed'
  }
}

// The plan's description, one feature per line, without markup.
function contentLines(plan) {
  return String(plan?.content || '')
    .replace(/<br\s*\/?>/gi, '\n')
    .replace(/<[^>]*>/g, '')
    .split('\n')
    .map(line => line.replace(/^[\s\-*•·]+/, '').trim())
    .filter(Boolean)
    .slice(0, 6)
}

// --- checkout --------------------------------------------------------------------
const checkout = reactive({
  open: false, plan: null, period: 'month', coupon: '', couponData: null, couponError: '', checking: false, busy: false, error: ''
})

const checkoutPeriods = computed(() => PERIODS
  .filter(([key]) => priceOf(checkout.plan, key) !== null)
  .map(([key]) => ({ value: key, label: t(`portal.plans.periods.${key}`) })))

const subtotal = computed(() => priceOf(checkout.plan, checkout.period) || 0)
const discount = computed(() => {
  const coupon = checkout.couponData
  if (!coupon) return 0
  const value = Number(coupon.value) || 0
  return Math.min(subtotal.value, Number(coupon.type) === 1 ? Math.round(subtotal.value * value / 100) : value)
})
const total = computed(() => Math.max(0, subtotal.value - discount.value))

function openCheckout(plan) {
  Object.assign(checkout, { plan, coupon: '', couponData: null, couponError: '', checking: false, busy: false, error: '' })
  checkout.period = priceOf(plan, period.value) !== null ? period.value : (checkoutPeriods.value[0]?.value || 'month')
  checkout.open = true
}

function closeCheckout() {
  if (checkout.busy) return
  checkout.open = false
}

async function applyCoupon() {
  if (!checkout.coupon || checkout.couponData) return
  checkout.checking = true
  checkout.couponError = ''
  try {
    const data = unwrapPanel(await checkCoupon({ code: checkout.coupon, plan_id: checkout.plan.id }))
    if (!data) throw new Error(t('portal.plans.checkout.couponInvalid'))
    checkout.couponData = data
  } catch (error) {
    checkout.couponError = panelErrorMessage(error) || t('portal.plans.checkout.couponInvalid')
  } finally {
    checkout.checking = false
  }
}

function removeCoupon() {
  checkout.coupon = ''
  checkout.couponData = null
  checkout.couponError = ''
}

async function submitOrder() {
  if (checkout.busy) return
  checkout.busy = true
  checkout.error = ''
  try {
    unwrapPanel(await saveOrder({
      plan_id: checkout.plan.id,
      period: checkout.period,
      coupon_id: checkout.couponData ? checkout.couponData.id : null
    }))
    checkout.busy = false
    checkout.open = false
    toast.success(t('portal.plans.checkout.created'))
    router.push('/user/orders')
  } catch (error) {
    checkout.error = t('portal.plans.checkout.failed', { message: panelErrorMessage(error) })
  } finally {
    checkout.busy = false
  }
}

onMounted(load)
</script>

<style scoped>
.plans {
  display: flex;
  flex-direction: column;
  gap: var(--space-10);
}

.plans__periods {
  display: flex;
  justify-content: center;
}

.plans__grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
  gap: var(--space-5);
  list-style: none;
}

.plan-card {
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
  padding: var(--space-8) var(--space-6);
  border-radius: var(--radius-lg);
  background: var(--bg-elevated);
  box-shadow: 0 0 0 0.5px var(--separator), var(--shadow-1);
}

.plan-card__name {
  font-size: var(--type-title-3-size);
  font-weight: var(--type-title-3-weight);
  line-height: var(--type-title-3-line);
}

.plan-card__price {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1) var(--space-2);
  align-items: baseline;
  min-height: 48px;
}

.plan-card__amount {
  font-size: var(--type-title-1-size);
  font-weight: var(--weight-bold);
  font-variant-numeric: tabular-nums;
  letter-spacing: var(--type-title-1-tracking);
}

.plan-card__per,
.plan-card__none {
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.plan-card__features {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-3);
  padding-top: var(--space-5);
  border-top: 1px solid var(--separator);
  list-style: none;
}

.plan-card__features li {
  display: flex;
  gap: var(--space-2);
  align-items: flex-start;
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
}

.plan-card__features :deep(.ui-icon) {
  flex: none;
  margin-top: 1px;
  color: var(--accent);
}

.plan-card.is-unavailable .plan-card__features {
  color: var(--label-2);
}

/* ---- checkout ------------------------------------------------------------------ */
.checkout {
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
}

.checkout__summary div,
.checkout__totals div {
  display: flex;
  gap: var(--space-4);
  justify-content: space-between;
}

.checkout__summary dt,
.checkout__totals dt {
  color: var(--label-2);
}

.checkout__summary dd,
.checkout__totals dd {
  margin: 0;
  font-weight: var(--weight-semibold);
  font-variant-numeric: tabular-nums;
}

.checkout__coupon {
  display: flex;
  gap: var(--space-2);
  align-items: flex-start;
}

.checkout__coupon-field {
  flex: 1;
  min-width: 0;
}

/* Level with the input (below the field's label), even when the field
   shows an error under it. */
.checkout__coupon > :deep(.ui-button) {
  margin-top: calc(var(--type-callout-size) * var(--type-callout-line) + var(--space-2));
}

.checkout__applied {
  display: flex;
  gap: var(--space-2);
  align-items: center;
  margin-top: calc(var(--space-3) * -1);
  color: var(--success);
  font-size: var(--type-callout-size);
}

.checkout__totals {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding-top: var(--space-4);
  border-top: 1px solid var(--separator);
}

.checkout__total {
  padding-top: var(--space-2);
  font-size: var(--type-title-3-size);
}

.checkout__total dt {
  color: var(--label-1);
  font-weight: var(--weight-semibold);
}

.checkout__error {
  color: var(--danger);
  font-size: var(--type-callout-size);
}

@media (max-width: 833px) {
  .plans {
    gap: var(--space-8);
  }

  .plans__periods {
    overflow-x: auto;
    justify-content: flex-start;
  }
}
</style>
