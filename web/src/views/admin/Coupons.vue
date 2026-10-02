<template>
  <div class="list-page">
    <UiPageHeader :title="t('adminCoupons.title')" :description="t('adminCoupons.subtitle')">
      <template #actions>
        <UiButton variant="primary" :icon="Plus" @click="showCreate = true">{{ t('adminCoupons.actions.createCoupon') }}</UiButton>
      </template>
    </UiPageHeader>

    <UiDataTable
      :columns="columns"
      :rows="visibleCoupons"
      :label="t('adminCoupons.table.label')"
      :row-label="coupon => coupon.code"
      storage-key="admin.coupons"
      :page-size="20"
      :loading="loading"
      :error="loadError"
      :error-title="t('adminCoupons.messages.fetchFailed')"
      :filtered="Boolean(search || typeFilter)"
      :empty-icon="TicketPercent"
      :empty-title="t('adminCoupons.empty.title')"
      :empty-description="t('adminCoupons.empty.description')"
      :row-actions="couponActions"
      @retry="load"
      @clear-filters="clearFilters"
    >
      <template #toolbar>
        <UiSearchField v-model="search" class="list-page__search" :label="t('adminCoupons.filters.search')" />
        <UiFilterChips v-model="typeFilter" :label="t('adminCoupons.filters.type')" :options="typeChips" />
      </template>
      <template #empty-actions>
        <UiButton variant="primary" :icon="Plus" @click="showCreate = true">{{ t('adminCoupons.actions.createCoupon') }}</UiButton>
      </template>
      <template #cell-code="{ value }">
        <code class="coupon-code">{{ value }}</code>
      </template>
      <template #cell-type="{ row }">
        <UiBadge :tone="row.type === 1 ? 'success' : 'info'" :dot="false" :label="row.type === 1 ? t('adminCoupons.types.discount') : t('adminCoupons.types.fixed')" />
      </template>
      <template #cell-usage="{ row }">
        <UiUsageBar :value="Number(row.use_count || 0)" :max="row.limit_use === -1 ? 0 : Number(row.limit_use || 0)" :text="usageText(row)" :warn-at="80" :danger-at="100" />
      </template>
    </UiDataTable>

    <UiDialog
      :open="showCreate"
      :title="t('adminCoupons.modal.title')"
      :dismissible="!creating"
      @update:open="value => { if (!value) closeModal() }"
    >
      <div class="form-grid">
        <UiTextField
          v-model="form.code"
          required
          :label="t('adminCoupons.fields.code')"
          :placeholder="t('adminCoupons.placeholders.code')"
          :help="t('adminCoupons.help.code')"
          :error="fieldErrors.code"
          data-test="coupon-code"
        />
        <UiTextField
          v-model="form.name"
          required
          :label="t('adminCoupons.fields.name')"
          :placeholder="t('adminCoupons.placeholders.name')"
          :error="fieldErrors.name"
          data-test="coupon-name"
        />
        <UiSelect v-model="form.type" :label="t('adminCoupons.fields.type')" :options="typeOptions" />
        <UiTextField
          v-model.number="form.value"
          type="number"
          min="0"
          :label="form.type === 1 ? t('adminCoupons.fields.discountValue') : t('adminCoupons.fields.fixedValue')"
          :suffix="form.type === 1 ? '%' : t('adminCoupons.units.cents')"
          :help="form.type === 1 ? '' : formatCouponValue({ type: 2, value: Number(form.value || 0) })"
        />
        <UiTextField v-model="form.started_at" type="datetime-local" :label="t('adminCoupons.fields.startTime')" :help="t('adminCoupons.help.startTime')" />
        <UiTextField v-model="form.ended_at" type="datetime-local" :label="t('adminCoupons.fields.endTime')" :help="t('adminCoupons.help.endTime')" />
        <UiTextField v-model.number="form.limit_use" class="form-grid__full" type="number" :label="t('adminCoupons.fields.limitUse')" :help="t('adminCoupons.help.limitUse')" />
      </div>
      <p v-if="createError" class="form-error" role="alert" data-test="coupon-create-error">{{ createError }}</p>
      <template #footer="{ close }">
        <UiButton :disabled="creating" @click="close">{{ t('common.actions.cancel') }}</UiButton>
        <UiButton variant="primary" data-test="coupon-create" :loading="creating" @click="createCoupon">{{ t('adminCoupons.actions.create') }}</UiButton>
      </template>
    </UiDialog>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { Plus, TicketPercent, Trash2 } from '@lucide/vue'
import adminApi from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiDialog from '@/ui/UiDialog.vue'
import UiFilterChips from '@/ui/UiFilterChips.vue'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import UiSearchField from '@/ui/UiSearchField.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiTextField from '@/ui/UiTextField.vue'
import UiUsageBar from '@/ui/UiUsageBar.vue'
import { useConfirm } from '@/ui/composables/useConfirm'
import { useFormat } from '@/ui/composables/useFormat'
import { useToast } from '@/ui/composables/useToast'

const { t } = useAppI18n()
const format = useFormat()
const toast = useToast()
const confirm = useConfirm()

const coupons = ref([])
const loading = ref(false)
const loadError = ref(null)
// Every coupon is loaded at once: search and the type chips filter in the page.
const search = ref('')
const typeFilter = ref('')
const visibleCoupons = computed(() => {
  const query = search.value.trim().toLowerCase()
  return coupons.value.filter(coupon => (
    (!typeFilter.value || String(coupon.type) === typeFilter.value) &&
    (!query || String(coupon.code || '').toLowerCase().includes(query) || String(coupon.name || '').toLowerCase().includes(query))
  ))
})
const clearFilters = () => {
  search.value = ''
  typeFilter.value = ''
}
const typeChips = computed(() => [
  { value: '1', label: t('adminCoupons.types.discount') },
  { value: '2', label: t('adminCoupons.types.fixed') }
])
const typeOptions = computed(() => [
  { value: 1, label: t('adminCoupons.types.discountPercent') },
  { value: 2, label: t('adminCoupons.types.fixedCents') }
])
const usageText = coupon => `${coupon.use_count ?? 0} / ${coupon.limit_use === -1 ? t('adminCoupons.table.unlimited') : (coupon.limit_use ?? 0)}`
const columns = computed(() => [
  { key: 'code', label: t('adminCoupons.table.code'), primary: true, sortable: true },
  { key: 'name', label: t('adminCoupons.table.name'), secondary: true, sortable: true },
  { key: 'type', label: t('adminCoupons.table.type'), sortable: true },
  { key: 'value', label: t('adminCoupons.table.value'), numeric: true, sortable: true, value: coupon => formatCouponValue(coupon), sortValue: coupon => Number(coupon.value || 0) },
  { key: 'usage', label: t('adminCoupons.table.usageCount'), sortable: true, sortValue: coupon => Number(coupon.use_count || 0) },
  { key: 'validity', label: t('adminCoupons.table.validity'), nowrap: true, value: coupon => `${formatCouponDate(coupon.started_at)} – ${formatCouponDate(coupon.ended_at)}`, sortable: true, sortValue: coupon => Number(coupon.ended_at || 0) },
  { key: 'id', label: t('adminCoupons.table.id'), numeric: true, sortable: true, hidden: true }
])
const couponActions = coupon => [
  { key: 'delete', label: t('adminCoupons.confirm.deleteAction'), icon: Trash2, danger: true, onSelect: () => removeCoupon(coupon) }
]
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
  loading.value = true
  try {
    const res = await adminApi.getCoupons()
    coupons.value = res.data || []
    loadError.value = null
  } catch (error) {
    loadError.value = error
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  load()
})

const formatCouponDate = ts => format.date(ts)

const formatCouponValue = (coupon) => {
  if (!coupon) return ''
  return coupon.type === 1 ? `${coupon.value}%` : format.money(Number(coupon.value || 0))
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
.coupon-code {
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
  font-weight: var(--weight-semibold);
}
</style>
