<template>
  <div class="plans-page">
    <div class="page-header">
      <h1>💰 订阅计划</h1>
      <p class="text-secondary">选择最适合您的流量套餐，随时开启高速网络体验</p>
    </div>

    <div v-if="loading" class="loading-state">
      <div class="spinner"></div>
      <p>正在加载精品套餐...</p>
    </div>

    <!-- 套餐列表 -->
    <div v-else class="plans-grid">
      <div v-for="plan in plans" :key="plan.id" class="plan-card">
        <div class="plan-badge" v-if="plan.onetime_price">永久</div>
        <h3 class="plan-name">{{ plan.name }}</h3>
        <div class="plan-price">
          <span class="currency">¥</span>
          <span class="amount">{{ formatPrice(getDisplayPrice(plan)) }}</span>
          <span class="period">/ {{ periodText }}</span>
        </div>
        
        <div class="plan-features">
          <div class="feature-item">
            <span class="icon">🚀</span>
            <span>{{ formatBytes(plan.transfer_enable * 1024 * 1024 * 1024) }} 流量</span>
          </div>
          <div class="feature-item" v-if="plan.speed_limit">
            <span class="icon">⚡</span>
            <span>{{ plan.speed_limit }}Mbps 速率限制</span>
          </div>
          <div class="feature-item" v-if="plan.device_limit">
            <span class="icon">📱</span>
            <span>{{ plan.device_limit }} 台设备同时在线</span>
          </div>
          <div class="feature-item">
            <span class="icon">🌍</span>
            <span>多国节点全协议支持</span>
          </div>
        </div>

        <button class="btn-primary w-full" @click="openPurchase(plan)">
          立即选购
        </button>
      </div>
    </div>

    <!-- 结算弹窗 -->
    <div v-if="showPurchase" class="modal-overlay" @click.self="closePurchase">
      <div class="modal modal-md">
        <div class="modal-header">
          <h3>确认订单</h3>
          <button class="close-btn" @click="closePurchase">✕</button>
        </div>
        <div class="modal-body">
          <div class="order-summary">
            <div class="summary-item">
              <span class="label">所选套餐</span>
              <span class="value">{{ selectedPlan.name }}</span>
            </div>
            
            <div class="form-group mt-4">
              <label>选择支付周期</label>
              <div class="period-selector">
                <button 
                  v-for="p in availablePeriods" 
                  :key="p.key"
                  :class="['period-btn', { active: selectedPeriod === p.key }]"
                  @click="selectedPeriod = p.key"
                >
                  <span class="period-name">{{ p.label }}</span>
                  <span class="period-price">¥{{ formatPrice(p.price) }}</span>
                </button>
              </div>
            </div>

            <!-- 优惠券部分 -->
            <div class="coupon-section mt-4">
              <label>使用优惠码 (可选)</label>
              <div class="coupon-input-group">
                <input 
                  v-model="couponCode" 
                  type="text" 
                  placeholder="输入优惠码"
                  :disabled="couponApplied"
                >
                <button 
                  v-if="!couponApplied" 
                  class="btn-secondary" 
                  @click="applyCoupon"
                  :disabled="checkingCoupon"
                >
                  {{ checkingCoupon ? '检查中' : '验证' }}
                </button>
                <button 
                  v-else 
                  class="btn-ghost text-error" 
                  @click="removeCoupon"
                >
                  移除
                </button>
              </div>
              <p v-if="couponError" class="coupon-tip text-error">{{ couponError }}</p>
              <p v-if="couponApplied" class="coupon-tip text-success">
                已应用优惠: {{ couponData.name }} (-¥{{ formatPrice(discountAmount) }})
              </p>
            </div>
          </div>

          <div class="order-total mt-6">
            <div class="total-row">
              <span>应付总额</span>
              <span class="total-price">¥{{ formatPrice(finalPrice) }}</span>
            </div>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn-ghost" @click="closePurchase">返回重选</button>
          <button class="btn-primary" @click="submitOrder" :disabled="creatingOrder">
            {{ creatingOrder ? '正在下单' : '提交订单' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { getPlans, checkCoupon, saveOrder } from '@/api/user'

const router = useRouter()
const plans = ref([])
const loading = ref(true)
const showPurchase = ref(false)
const selectedPlan = ref(null)
const selectedPeriod = ref('month')

// 优惠券相关
const couponCode = ref('')
const couponApplied = ref(false)
const couponData = ref(null)
const checkingCoupon = ref(false)
const couponError = ref('')

// 下单相关
const creatingOrder = ref(false)

const loadPlans = async () => {
  loading.value = true
  try {
    const res = await getPlans()
    plans.value = res.data || []
  } catch (err) {
    console.error('加载套餐失败:', err)
  } finally {
    loading.value = false
  }
}

const getDisplayPrice = (plan) => {
  if (plan.month_price) return plan.month_price
  if (plan.onetime_price) return plan.onetime_price
  return 0
}

const periodText = '月' // 简化显示

const availablePeriods = computed(() => {
  if (!selectedPlan.value) return []
  const p = selectedPlan.value
  const list = []
  if (p.month_price) list.push({ key: 'month', label: '月付', price: p.month_price })
  if (p.quarter_price) list.push({ key: 'quarter', label: '季付', price: p.quarter_price })
  if (p.half_year_price) list.push({ key: 'half_year', label: '半年付', price: p.half_year_price })
  if (p.year_price) list.push({ key: 'year', label: '年付', price: p.year_price })
  if (p.onetime_price) list.push({ key: 'onetime', label: '一次性', price: p.onetime_price })
  return list
})

const currentPeriodPrice = computed(() => {
  const period = availablePeriods.value.find(p => p.key === selectedPeriod.value)
  return period ? period.price : 0
})

const discountAmount = computed(() => {
  if (!couponApplied.value || !couponData.value) return 0
  const price = currentPeriodPrice.value
  if (couponData.value.type === 1) { // 百分比
    return price * couponData.value.value / 100
  } else { // 固定金额
    return couponData.value.value
  }
})

const finalPrice = computed(() => {
  return Math.max(0, currentPeriodPrice.value - discountAmount.value)
})

const formatPrice = (cent) => {
  return (cent / 100).toFixed(2)
}

const formatBytes = (bytes) => {
  if (!bytes) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  while (bytes >= 1024 && i < units.length - 1) {
    bytes /= 1024
    i++
  }
  return bytes.toFixed(2) + ' ' + units[i]
}

const openPurchase = (plan) => {
  selectedPlan.value = plan
  // 默认选择第一个可用的周期
  const periods = availablePeriods.value
  if (periods.length > 0) {
    selectedPeriod.value = periods[0].key
  }
  showPurchase.value = true
}

const closePurchase = () => {
  showPurchase.value = false
  removeCoupon()
}

const applyCoupon = async () => {
  if (!couponCode.value.trim()) return
  checkingCoupon.value = true
  couponError.value = ''
  try {
    const res = await checkCoupon({ 
      code: couponCode.value.trim(),
      plan_id: selectedPlan.value.id 
    })
    couponData.value = res.data
    couponApplied.value = true
  } catch (err) {
    couponError.value = err.response?.data?.message || '无效的优惠码'
  } finally {
    checkingCoupon.value = false
  }
}

const removeCoupon = () => {
  couponCode.value = ''
  couponApplied.value = false
  couponData.value = null
  couponError.value = ''
}

const submitOrder = async () => {
  creatingOrder.value = true
  try {
    const res = await saveOrder({
      plan_id: selectedPlan.value.id,
      period: selectedPeriod.value,
      coupon_id: couponApplied.value ? couponData.value.id : null
    })
    // 下单成功后跳转到订单列表（或支付页面）
    router.push('/user/orders')
  } catch (err) {
    alert(err.response?.data?.message || '下单失败')
  } finally {
    creatingOrder.value = false
  }
}

onMounted(() => {
  loadPlans()
})
</script>

<style scoped>
.plans-page {
  max-width: 1200px;
  margin: 0 auto;
}

.page-header {
  margin-bottom: 32px;
  text-align: center;
}

.page-header h1 {
  font-size: 32px;
  margin-bottom: 8px;
}

.plans-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 24px;
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
}

.plan-card:hover {
  border-color: var(--primary-color);
  transform: translateY(-4px);
  box-shadow: var(--shadow-lg);
}

.plan-badge {
  position: absolute;
  top: 16px;
  right: 16px;
  background: var(--primary-color);
  color: white;
  padding: 4px 12px;
  border-radius: 20px;
  font-size: 12px;
  font-weight: 600;
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
  font-size: 18px;
  width: 24px;
  display: inline-flex;
  justify-content: center;
}

/* 结算弹窗 */
.order-summary {
  background: var(--bg-color);
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
  background: rgba(59, 130, 246, 0.1);
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

.text-error { color: var(--error-color); }
.text-success { color: var(--success-color); }

/* 状态样式 */
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
  to { transform: rotate(360deg); }
}
</style>
