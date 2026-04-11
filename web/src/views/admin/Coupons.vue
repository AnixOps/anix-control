<template>
  <div class="coupons-page">
    <div class="page-header">
      <h1>{{ t('adminCoupons.title') }}</h1>
      <p class="text-secondary">{{ t('adminCoupons.subtitle') }}</p>
    </div>

    <div class="filter-bar">
      <button @click="showCreate = true">{{ t('adminCoupons.actions.createCoupon') }}</button>
    </div>

    <div class="table-container">
      <table class="data-table">
        <thead>
          <tr>
            <th>{{ t('adminCoupons.table.id') }}</th>
            <th>{{ t('adminCoupons.table.code') }}</th>
            <th>{{ t('adminCoupons.table.name') }}</th>
            <th>{{ t('adminCoupons.table.type') }}</th>
            <th>{{ t('adminCoupons.table.value') }}</th>
            <th>{{ t('adminCoupons.table.usageCount') }}</th>
            <th>{{ t('adminCoupons.table.validity') }}</th>
            <th>{{ t('adminCoupons.table.actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="coupon in coupons" :key="coupon.id">
            <td>{{ coupon.id }}</td>
            <td><code class="coupon-code">{{ coupon.code }}</code></td>
            <td>{{ coupon.name }}</td>
            <td>
              <span :class="['status-badge', coupon.type === 1 ? 'type-percent' : 'type-fixed']">
                {{ coupon.type === 1 ? t('adminCoupons.types.discount') : t('adminCoupons.types.fixed') }}
              </span>
            </td>
            <td>{{ formatCouponValue(coupon) }}</td>
            <td>{{ coupon.use_count }} / {{ coupon.limit_use === -1 ? t('adminCoupons.table.unlimited') : coupon.limit_use }}</td>
            <td class="date-range">{{ formatCouponDate(coupon.started_at) }} ~ {{ formatCouponDate(coupon.ended_at) }}</td>
            <td>
              <div class="action-buttons">
                <button
                  class="btn-sm btn-ghost"
                  :title="t('common.actions.delete')"
                  :aria-label="t('common.actions.delete')"
                  @click="removeCoupon(coupon)"
                >
                  {{ t('common.actions.delete') }}
                </button>
              </div>
            </td>
          </tr>
          <tr v-if="coupons.length === 0">
            <td colspan="8" class="empty-row">{{ t('adminCoupons.empty.noData') }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <div v-if="showCreate" class="modal-overlay" @click.self="closeModal">
      <div class="modal">
        <div class="modal-header">
          <h3>{{ t('adminCoupons.modal.title') }}</h3>
          <button
            class="close-btn"
            :aria-label="t('common.actions.close')"
            :title="t('common.actions.close')"
            @click="closeModal"
          >
            {{ t('common.actions.close') }}
          </button>
        </div>
        <div class="modal-body">
          <div class="form-row">
            <div class="form-group">
              <label>{{ t('adminCoupons.fields.code') }} <span class="required">*</span></label>
              <input v-model="form.code" type="text" :placeholder="t('adminCoupons.placeholders.code')" />
            </div>
            <div class="form-group">
              <label>{{ t('adminCoupons.fields.name') }} <span class="required">*</span></label>
              <input v-model="form.name" type="text" :placeholder="t('adminCoupons.placeholders.name')" />
            </div>
          </div>
          <div class="form-row">
            <div class="form-group">
              <label>{{ t('adminCoupons.fields.type') }}</label>
              <select v-model="form.type">
                <option :value="1">{{ t('adminCoupons.types.discountPercent') }}</option>
                <option :value="2">{{ t('adminCoupons.types.fixedCents') }}</option>
              </select>
            </div>
            <div class="form-group">
              <label>{{ form.type === 1 ? t('adminCoupons.fields.discountValue') : t('adminCoupons.fields.fixedValue') }}</label>
              <input v-model.number="form.value" type="number" min="0" />
            </div>
          </div>
          <div class="form-row">
            <div class="form-group">
              <label>{{ t('adminCoupons.fields.startTime') }}</label>
              <input v-model="form.started_at" type="datetime-local" />
            </div>
            <div class="form-group">
              <label>{{ t('adminCoupons.fields.endTime') }}</label>
              <input v-model="form.ended_at" type="datetime-local" />
            </div>
          </div>
          <div class="form-group">
            <label>{{ t('adminCoupons.fields.limitUse') }}</label>
            <input v-model.number="form.limit_use" type="number" />
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn-secondary" @click="closeModal">{{ t('common.actions.cancel') }}</button>
          <button @click="createCoupon">{{ t('common.actions.create') }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { onMounted, reactive, ref } from 'vue'
import adminApi from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'

const { t, formatDate } = useAppI18n()

const coupons = ref([])
const showCreate = ref(false)
const form = reactive({
  code: '',
  name: '',
  type: 1,
  value: 10,
  limit_use: 100,
  started_at: '',
  ended_at: ''
})

const resetForm = () => {
  form.code = ''
  form.name = ''
  form.type = 1
  form.value = 10
  form.limit_use = 100
  form.started_at = ''
  form.ended_at = ''
}

const load = async () => {
  try {
    const res = await adminApi.getCoupons()
    coupons.value = res.data || []
  } catch (error) {
    console.error(t('adminCoupons.messages.fetchFailed'), error)
  }
}

onMounted(() => {
  load()
})

const formatCouponDate = (ts) => {
  if (!ts) return '-'
  return formatDate(ts)
}

const formatCouponValue = (coupon) => {
  if (!coupon) return ''
  const currencySymbol = t('adminCoupons.currencySymbol')
  return coupon.type === 1
    ? `${coupon.value}%`
    : `${currencySymbol}${(coupon.value / 100).toFixed(2)}`
}

const closeModal = () => {
  showCreate.value = false
  resetForm()
}

const createCoupon = async () => {
  if (!form.code || !form.name) {
    window.alert(t('adminCoupons.messages.requiredFields'))
    return
  }

  try {
    const payload = {
      code: form.code.toUpperCase(),
      name: form.name,
      type: form.type,
      value: form.value,
      limit_use: form.limit_use,
      started_at: form.started_at
        ? Math.floor(new Date(form.started_at).getTime() / 1000)
        : Math.floor(Date.now() / 1000),
      ended_at: form.ended_at
        ? Math.floor(new Date(form.ended_at).getTime() / 1000)
        : Math.floor(Date.now() / 1000) + 30 * 24 * 3600
    }

    await adminApi.createCoupon(payload)
    window.alert(t('adminCoupons.messages.createSuccess'))
    closeModal()
    await load()
  } catch (error) {
    window.alert(error.message || t('adminCoupons.messages.createFailed'))
  }
}

const removeCoupon = async (coupon) => {
  if (!window.confirm(t('adminCoupons.messages.deleteConfirm', { code: coupon.code }))) return

  try {
    await adminApi.deleteCoupon(coupon.id)
    await load()
  } catch (error) {
    window.alert(error.message || t('adminCoupons.messages.deleteFailed'))
  }
}
</script>

<style scoped>
.coupons-page {
  max-width: 1400px;
}

.page-header {
  margin-bottom: 24px;
}

.page-header h1 {
  font-size: 24px;
  margin-bottom: 4px;
}

.filter-bar {
  display: flex;
  gap: 12px;
  margin-bottom: 16px;
  flex-wrap: wrap;
}

.table-container {
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  overflow-x: auto;
}

.data-table {
  width: 100%;
  border-collapse: collapse;
}

.data-table th,
.data-table td {
  padding: 14px 16px;
  text-align: left;
  border-bottom: 1px solid var(--border-color);
}

.data-table th {
  background: var(--bg-color);
  font-weight: 600;
  font-size: 13px;
  color: var(--text-secondary);
  text-transform: uppercase;
}

.data-table tr:hover {
  background: var(--bg-color);
}

.coupon-code {
  background: rgba(59, 130, 246, 0.15);
  color: var(--primary-color);
  padding: 4px 8px;
  border-radius: 4px;
  font-family: 'Consolas', 'Monaco', monospace;
  font-weight: 600;
}

.status-badge {
  font-size: 12px;
  padding: 4px 10px;
  border-radius: 20px;
  font-weight: 500;
}

.type-percent {
  background: rgba(34, 197, 94, 0.15);
  color: var(--success-color);
}

.type-fixed {
  background: rgba(59, 130, 246, 0.15);
  color: var(--primary-color);
}

.date-range {
  font-size: 13px;
  color: var(--text-secondary);
}

.action-buttons {
  display: flex;
  gap: 4px;
}

.empty-row {
  text-align: center;
  color: var(--text-secondary);
  padding: 40px !important;
}

.modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.7);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  padding: 20px;
}

.modal {
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  width: 100%;
  max-width: 550px;
  max-height: 90vh;
  overflow-y: auto;
}

.modal-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 20px;
  border-bottom: 1px solid var(--border-color);
}

.modal-header h3 {
  font-size: 18px;
  font-weight: 600;
}

.close-btn {
  background: transparent;
  border: none;
  font-size: 18px;
  color: var(--text-secondary);
  cursor: pointer;
  padding: 4px;
}

.modal-body {
  padding: 20px;
}

.form-row {
  display: flex;
  gap: 16px;
}

@media (max-width: 640px) {
  .form-row {
    flex-direction: column;
    gap: 0;
  }
}

.form-row .form-group {
  flex: 1;
}

.modal-body .form-group {
  margin-bottom: 16px;
}

.modal-body label {
  display: block;
  margin-bottom: 6px;
  font-size: 14px;
  font-weight: 500;
}

.modal-body .required {
  color: var(--error-color);
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  padding: 16px 20px;
  border-top: 1px solid var(--border-color);
}

.modal-footer button {
  min-width: 80px;
}
</style>
