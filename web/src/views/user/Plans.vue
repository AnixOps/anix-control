<template>
  <div class="page-shell plans-page">
    <div class="page-toolbar">
      <div>
        <h1>{{ t('user.plans.title') }}</h1>
        <p>{{ t('user.plans.subtitle') }}</p>
      </div>
    </div>

    <section v-if="loading" class="section-panel loading-state">
      <div class="spinner"></div>
      <p>{{ t('user.plans.loading') }}</p>
    </section>

    <div v-else class="plans-grid">
      <div v-for="plan in plans" :key="plan.id" class="plan-card">
        <div v-if="plan.onetime_price" class="plan-badge">{{ t('user.plans.permanentBadge') }}</div>
        <h3 class="plan-name">{{ plan.name }}</h3>
        <div class="plan-price">
          <span class="currency">¥</span>
          <span class="amount">{{ formatPrice(getDisplayPrice(plan)) }}</span>
          <span class="period">/ {{ displayPeriodLabel(plan) }}</span>
        </div>

        <div class="plan-features">
          <div class="feature-item">
            <span class="icon">T</span>
            <span>{{ t('user.plans.trafficFeature', { value: formatBytes(plan.transfer_enable * 1024 * 1024 * 1024) }) }}</span>
          </div>
          <div v-if="plan.speed_limit" class="feature-item">
            <span class="icon">S</span>
            <span>{{ t('user.plans.speedLimitFeature', { value: plan.speed_limit }) }}</span>
          </div>
          <div v-if="plan.device_limit" class="feature-item">
            <span class="icon">D</span>
            <span>{{ t('user.plans.deviceLimitFeature', { value: plan.device_limit }) }}</span>
          </div>
          <div class="feature-item">
            <span class="icon">N</span>
            <span>{{ t('user.plans.unlimitedFeature') }}</span>
          </div>
        </div>

        <button class="btn btn-primary w-full" @click="openPurchase(plan)">{{ t('common.actions.buyNow') }}</button>
      </div>
    </div>

    <div v-if="showPurchase" class="modal-overlay" @click.self="closePurchase">
      <div class="modal modal-md">
        <div class="modal-header">
          <h3>{{ t('user.plans.confirmOrder') }}</h3>
          <button class="btn btn-ghost btn-sm close-btn normalized-close" :title="t('common.actions.close')" :aria-label="t('common.actions.close')" @click="closePurchase">x</button>
          <button class="close-btn" @click="closePurchase">×</button>
        </div>
        <div class="modal-body">
          <div class="order-summary">
            <div class="summary-item">
              <span class="label">{{ t('user.plans.selectedPlan') }}</span>
              <span class="value">{{ selectedPlan?.name }}</span>
            </div>

            <div class="form-group mt-4">
              <label>{{ t('user.plans.choosePeriod') }}</label>
              <div class="period-selector">
                <button
                  v-for="item in availablePeriods"
                  :key="item.key"
                  :class="['period-btn', { active: selectedPeriod === item.key }]"
                  @click="selectedPeriod = item.key"
                >
                  <span class="period-name">{{ item.label }}</span>
                  <span class="period-price normalized-amount">{{ formatCurrency(item.price) }}</span>
                  <span class="period-price">¥{{ formatPrice(item.price) }}</span>
                </button>
              </div>
            </div>

            <div class="coupon-section mt-4">
              <label>{{ t('user.plans.optionalCoupon') }}</label>
              <div class="coupon-input-group">
                <input
                  v-model.trim="couponCode"
                  type="text"
                  :placeholder="t('user.plans.couponPlaceholder')"
                  :disabled="couponApplied"
                >
                <button
                  v-if="!couponApplied"
                  class="btn"
                  :disabled="checkingCoupon"
                  @click="applyCoupon"
                >
                  {{ checkingCoupon ? t('common.actions.refresh') : t('common.actions.verify') }}
                </button>
                <button v-else class="btn btn-ghost text-error" @click="removeCoupon">{{ t('common.actions.remove') }}</button>
              </div>
              <p v-if="couponError" class="coupon-tip text-error">{{ couponError }}</p>
              <p v-if="couponApplied" class="coupon-tip text-success">
                {{ t('user.plans.couponApplied', { name: couponData.name, value: formatPrice(discountAmount) }) }}
              </p>
            </div>
          </div>

          <div class="order-total mt-6">
            <div class="total-row">
              <span>{{ t('user.plans.totalAmount') }}</span>
              <span class="total-price normalized-amount">{{ formatCurrency(finalPrice) }}</span>
              <span class="total-price">¥{{ formatPrice(finalPrice) }}</span>
            </div>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn btn-ghost" @click="closePurchase">{{ t('user.plans.backToEdit') }}</button>
          <button class="btn btn-primary" :disabled="creatingOrder" @click="submitOrder">
            {{ creatingOrder ? t('user.plans.creatingOrder') : t('common.actions.submit') }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { checkCoupon, getPlans, saveOrder } from '@/api/user'
import { useAppI18n } from '@/composables/useAppI18n'

const router = useRouter()
const { t } = useAppI18n()

const plans = ref([])
const loading = ref(true)
const showPurchase = ref(false)
const selectedPlan = ref(null)
const selectedPeriod = ref('month')
const couponCode = ref('')
const couponApplied = ref(false)
const couponData = ref(null)
const checkingCoupon = ref(false)
const couponError = ref('')
const creatingOrder = ref(false)

async function loadPlans() {
  loading.value = true
  try {
    const res = await getPlans()
    plans.value = res.data || []
  } catch (err) {
    console.error('Failed to load plans:', err)
  } finally {
    loading.value = false
  }
}

function getDisplayPrice(plan) {
  if (plan.month_price) return plan.month_price
  if (plan.onetime_price) return plan.onetime_price
  return 0
}

function displayPeriodLabel(plan) {
  return plan.month_price ? t('common.periods.month') : t('common.periods.onetime')
}

const availablePeriods = computed(() => {
  if (!selectedPlan.value) return []
  const plan = selectedPlan.value
  const list = []
  if (plan.month_price) list.push({ key: 'month', label: t('common.periods.month'), price: plan.month_price })
  if (plan.quarter_price) list.push({ key: 'quarter', label: t('common.periods.quarter'), price: plan.quarter_price })
  if (plan.half_year_price) list.push({ key: 'half_year', label: t('common.periods.halfYear'), price: plan.half_year_price })
  if (plan.year_price) list.push({ key: 'year', label: t('common.periods.year'), price: plan.year_price })
  if (plan.two_year_price) list.push({ key: 'two_year', label: t('common.periods.twoYear'), price: plan.two_year_price })
  if (plan.three_year_price) list.push({ key: 'three_year', label: t('common.periods.threeYear'), price: plan.three_year_price })
  if (plan.onetime_price) list.push({ key: 'onetime', label: t('common.periods.onetime'), price: plan.onetime_price })
  return list
})

const currentPeriodPrice = computed(() => availablePeriods.value.find((item) => item.key === selectedPeriod.value)?.price || 0)

const discountAmount = computed(() => {
  if (!couponApplied.value || !couponData.value) return 0
  if (couponData.value.type === 1) {
    return currentPeriodPrice.value * couponData.value.value / 100
  }
  return couponData.value.value
})

const finalPrice = computed(() => Math.max(0, currentPeriodPrice.value - discountAmount.value))

function formatPrice(amount) {
  return ((amount || 0) / 100).toFixed(2)
}

function formatCurrency(amount) {
  return `\u00a5${formatPrice(amount)}`
}

function formatBytes(bytes) {
  if (!bytes) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let value = bytes
  let index = 0
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024
    index += 1
  }
  return `${value.toFixed(2)} ${units[index]}`
}

function openPurchase(plan) {
  selectedPlan.value = plan
  selectedPeriod.value = availablePeriods.value[0]?.key || 'month'
  showPurchase.value = true
}

function closePurchase() {
  showPurchase.value = false
  removeCoupon()
}

async function applyCoupon() {
  if (!couponCode.value || !selectedPlan.value) {
    return
  }
  checkingCoupon.value = true
  couponError.value = ''
  try {
    const res = await checkCoupon({
      code: couponCode.value,
      plan_id: selectedPlan.value.id
    })
    couponData.value = res.data
    couponApplied.value = true
  } catch (err) {
    couponError.value = err.response?.data?.message || t('common.messages.invalidCoupon')
  } finally {
    checkingCoupon.value = false
  }
}

function removeCoupon() {
  couponCode.value = ''
  couponApplied.value = false
  couponData.value = null
  couponError.value = ''
}

async function submitOrder() {
  creatingOrder.value = true
  try {
    await saveOrder({
      plan_id: selectedPlan.value.id,
      period: selectedPeriod.value,
      coupon_id: couponApplied.value ? couponData.value.id : null
    })
    router.push('/user/orders')
  } catch (err) {
    alert(err.response?.data?.message || t('common.messages.submitFailed'))
  } finally {
    creatingOrder.value = false
  }
}

onMounted(() => {
  loadPlans()
})
</script>

<style scoped>
.plans-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 16px;
}

.plan-card {
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  padding: 32px;
  position: relative;
  transition: var(--transition);
  display: flex;
  flex-direction: column;
  box-shadow: var(--shadow-sm);
}

.plan-card:hover {
  border-color: var(--primary-color);
  box-shadow: var(--shadow-md);
}

.plan-badge {
  position: absolute;
  top: 16px;
  right: 16px;
  background: var(--primary-color);
  color: white;
  padding: 4px 12px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 700;
}

.plan-name {
  font-size: 20px;
  font-weight: 700;
  margin-bottom: 16px;
}

.plan-price {
  margin-bottom: 24px;
  display: flex;
  align-items: baseline;
  gap: 4px;
}

.currency {
  font-size: 18px;
  font-weight: 600;
}

.amount {
  font-size: 36px;
  font-weight: 800;
  color: var(--primary-color);
}

.period {
  font-size: 14px;
  color: var(--text-secondary);
}

.plan-features {
  margin-bottom: 32px;
  flex: 1;
}

.feature-item {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
  font-size: 14px;
}

.feature-item .icon {
  font-size: 14px;
  width: 24px;
  display: inline-flex;
  justify-content: center;
  font-weight: 700;
}

.order-summary {
  background: var(--surface-muted);
  padding: 20px;
  border-radius: var(--radius-md);
}

.summary-item {
  display: flex;
  justify-content: space-between;
  margin-bottom: 8px;
}

.summary-item .label {
  color: var(--text-secondary);
}

.summary-item .value {
  font-weight: 600;
}

.period-selector {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 10px;
  margin-top: 10px;
}

.period-btn {
  padding: 12px;
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  cursor: pointer;
  display: flex;
  flex-direction: column;
  align-items: center;
  transition: var(--transition);
}

.period-btn:hover {
  border-color: var(--primary-color);
}

.period-btn.active {
  background: var(--primary-soft);
  border-color: var(--primary-color);
}

.period-name {
  font-size: 13px;
  font-weight: 600;
}

.period-price {
  font-size: 11px;
  color: var(--text-secondary);
}

.coupon-input-group {
  display: flex;
  gap: 8px;
  margin-top: 8px;
}

.coupon-input-group input {
  flex: 1;
}

.coupon-tip {
  font-size: 12px;
  margin-top: 4px;
}

.order-total {
  padding: 16px 0;
  border-top: 1px dashed var(--border-color);
}

.total-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.total-price {
  font-size: 24px;
  font-weight: 800;
  color: var(--primary-color);
}

.period-name + .normalized-amount + .period-price,
.total-row .normalized-amount + .total-price,
.modal-header > .close-btn:not(.normalized-close) {
  display: none;
}

.text-error {
  color: var(--error-color);
}

.text-success {
  color: var(--success-color);
}

.loading-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 100px 0;
}

.spinner {
  width: 40px;
  height: 40px;
  border: 4px solid rgba(59, 130, 246, 0.1);
  border-top-color: var(--primary-color);
  border-radius: 50%;
  animation: rotate 1s linear infinite;
  margin-bottom: 16px;
}

@keyframes rotate {
  to {
    transform: rotate(360deg);
  }
}
</style>
