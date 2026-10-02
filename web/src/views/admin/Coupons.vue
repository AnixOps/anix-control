<template>
  <div class="page-shell coupons-page">
    <div class="page-toolbar">
      <div>
        <h1>{{ t('adminCoupons.title') }}</h1>
        <p>{{ t('adminCoupons.subtitle') }}</p>
      </div>
      <button class="btn btn-primary" @click="showCreate = true">
        {{ t('adminCoupons.actions.createCoupon') }}
      </button>
    </div>

    <section class="section-panel data-panel">
      <div class="table-wrap">
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
                  class="btn btn-sm btn-danger"
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
    </section>

    <UiDialog
      :open="showCreate"
      :title="t('adminCoupons.modal.title')"
      :dismissible="!creating"
      @update:open="value => { if (!value) closeModal() }"
    >
      <div class="dialog-form">
        <div class="form-row">
          <div class="form-group">
            <label for="coupon-code">{{ t('adminCoupons.fields.code') }} <span class="required">*</span></label>
            <input
              id="coupon-code"
              v-model="form.code"
              type="text"
              data-test="coupon-code"
              :placeholder="t('adminCoupons.placeholders.code')"
              :aria-invalid="fieldErrors.code ? 'true' : undefined"
              :aria-describedby="fieldErrors.code ? 'coupon-code-error' : undefined"
            />
            <p v-if="fieldErrors.code" id="coupon-code-error" class="form-error" role="alert">{{ fieldErrors.code }}</p>
          </div>
          <div class="form-group">
            <label for="coupon-name">{{ t('adminCoupons.fields.name') }} <span class="required">*</span></label>
            <input
              id="coupon-name"
              v-model="form.name"
              type="text"
              data-test="coupon-name"
              :placeholder="t('adminCoupons.placeholders.name')"
              :aria-invalid="fieldErrors.name ? 'true' : undefined"
              :aria-describedby="fieldErrors.name ? 'coupon-name-error' : undefined"
            />
            <p v-if="fieldErrors.name" id="coupon-name-error" class="form-error" role="alert">{{ fieldErrors.name }}</p>
          </div>
        </div>
        <div class="form-row">
          <div class="form-group">
            <label for="coupon-type">{{ t('adminCoupons.fields.type') }}</label>
            <select id="coupon-type" v-model="form.type">
              <option :value="1">{{ t('adminCoupons.types.discountPercent') }}</option>
              <option :value="2">{{ t('adminCoupons.types.fixedCents') }}</option>
            </select>
          </div>
          <div class="form-group">
            <label for="coupon-value">{{ form.type === 1 ? t('adminCoupons.fields.discountValue') : t('adminCoupons.fields.fixedValue') }}</label>
            <input id="coupon-value" v-model.number="form.value" type="number" min="0" />
          </div>
        </div>
        <div class="form-row">
          <div class="form-group">
            <label for="coupon-start">{{ t('adminCoupons.fields.startTime') }}</label>
            <input id="coupon-start" v-model="form.started_at" type="datetime-local" />
          </div>
          <div class="form-group">
            <label for="coupon-end">{{ t('adminCoupons.fields.endTime') }}</label>
            <input id="coupon-end" v-model="form.ended_at" type="datetime-local" />
          </div>
        </div>
        <div class="form-group">
          <label for="coupon-limit">{{ t('adminCoupons.fields.limitUse') }}</label>
          <input id="coupon-limit" v-model.number="form.limit_use" type="number" />
        </div>
        <p v-if="createError" class="form-error" role="alert" data-test="coupon-create-error">{{ createError }}</p>
      </div>
      <template #footer="{ close }">
        <UiButton :disabled="creating" @click="close">{{ t('common.actions.cancel') }}</UiButton>
        <UiButton variant="primary" data-test="coupon-create" :loading="creating" @click="createCoupon">{{ t('common.actions.create') }}</UiButton>
      </template>
    </UiDialog>
  </div>
</template>

<script setup>
import { onMounted, reactive, ref } from 'vue'
import adminApi from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import { UiButton, UiDialog, useConfirm, useToast } from '@/ui'

const { t, formatDate } = useAppI18n()
const toast = useToast()
const confirm = useConfirm()

const coupons = ref([])
const showCreate = ref(false)
const creating = ref(false)
const createError = ref('')
const fieldErrors = reactive({ code: '', name: '' })
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
  createError.value = ''
  fieldErrors.code = ''
  fieldErrors.name = ''
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
  if (creating.value) return
  createError.value = ''
  fieldErrors.code = form.code ? '' : t('adminCoupons.messages.codeRequired')
  fieldErrors.name = form.name ? '' : t('adminCoupons.messages.nameRequired')
  if (fieldErrors.code || fieldErrors.name) return

  creating.value = true
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
    toast.success(t('adminCoupons.messages.createSuccess'))
    closeModal()
    await load()
  } catch (error) {
    createError.value = error.message || t('adminCoupons.messages.createFailed')
  } finally {
    creating.value = false
  }
}

const removeCoupon = async (coupon) => {
  const confirmed = await confirm({
    title: t('adminCoupons.confirm.deleteTitle', { code: coupon.code }),
    message: t('adminCoupons.confirm.deleteMessage'),
    confirmLabel: t('adminCoupons.confirm.deleteAction'),
    tone: 'danger',
    onConfirm: async () => {
      try {
        await adminApi.deleteCoupon(coupon.id)
      } catch (error) {
        throw new Error(error.message || t('adminCoupons.messages.deleteFailed'))
      }
    }
  })
  if (!confirmed) return
  toast.success(t('adminCoupons.messages.deleted', { code: coupon.code }))
  await load()
}
</script>

<style scoped>
.data-panel {
  padding: 0;
}

.coupon-code {
  display: inline-flex;
  background: var(--primary-soft);
  color: var(--primary-color);
  padding: 4px 8px;
  border-radius: var(--radius-sm);
  font-family: 'Consolas', 'Monaco', monospace;
  font-size: 12px;
  font-weight: 700;
}

.type-percent {
  background: rgba(22, 163, 74, 0.08);
  color: var(--success-color);
}

.type-fixed {
  background: var(--primary-soft);
  color: var(--primary-color);
}

.date-range {
  font-size: 13px;
  color: var(--text-secondary);
}

.action-buttons {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

.empty-row {
  padding: 40px !important;
}

.dialog-form > .form-group:last-child {
  margin-bottom: 0;
}

.form-error {
  margin: var(--space-1) 0 0;
  color: var(--danger);
  font-size: var(--type-callout-size);
}
</style>
