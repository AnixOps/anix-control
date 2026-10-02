<template>
  <div class="list-page">
    <UiPageHeader :title="pt('title')" :description="pt('subtitle')">
      <template #actions>
        <UiButton variant="primary" :icon="Plus" data-test="plan-create-button" @click="openCreateModal">{{ pt('actions.create') }}</UiButton>
      </template>
    </UiPageHeader>

    <UiDataTable
      :columns="columns"
      :rows="filteredPlans"
      :label="pt('table.label')"
      :row-label="plan => plan.name"
      storage-key="admin.plans"
      :page-size="20"
      :loading="listLoading"
      :error="listError"
      :error-title="pt('messages.loadFailed')"
      :filtered="Boolean(search.trim())"
      :empty-icon="Package"
      :empty-title="pt('empty.title')"
      :empty-description="pt('empty.description')"
      activatable
      :row-actions="planActions"
      @row-activate="openDetail"
      @retry="load"
      @clear-filters="search = ''"
    >
      <template #toolbar>
        <UiSearchField v-model="search" class="list-page__search" :label="pt('filters.search')" />
      </template>
      <template #empty-actions>
        <UiButton variant="primary" :icon="Plus" @click="openCreateModal">{{ pt('actions.create') }}</UiButton>
      </template>
      <template #cell-groups="{ row }">
        <span v-if="(planGroups[row.id] || []).length" class="plan-groups">
          <UiBadge v-for="group in planGroups[row.id]" :key="group.id" tone="neutral" :dot="false" :label="group.name" />
        </span>
        <span v-else class="plan-groups__none">{{ pt('labels.noGroups') }}</span>
      </template>
    </UiDataTable>

    <UiSheet
      :open="Boolean(detailPlan)"
      grouped
      :title="detailPlan?.name || ''"
      :description="detailPlan ? pt('detail.description', { id: detailPlan.id }) : ''"
      data-test="plan-detail-sheet"
      @update:open="value => { if (!value) detailPlan = null }"
    >
      <template v-if="detailPlan">
        <UiGroupedList :title="pt('detail.limits')">
          <UiGroupedListRow :label="pt('table.transfer')" :value="formatTransfer(detailPlan.transfer_enable)" />
          <UiGroupedListRow :label="pt('table.limits')" :value="formatPlanLimits(detailPlan)" />
          <UiGroupedListRow v-if="isCommercial" :label="pt('table.monthPrice')" :value="formatPrice(detailPlan.month_price)" />
        </UiGroupedList>
        <section class="plan-sheet-groups" aria-labelledby="plan-sheet-groups-title">
          <div class="plan-sheet-groups__head">
            <h3 id="plan-sheet-groups-title" class="plan-sheet-groups__title">{{ pt('table.subscriptionGroups') }}</h3>
            <UiButton size="sm" @click="openGroupModal(detailPlan)">{{ pt('actions.manageGroups') }}</UiButton>
          </div>
          <ul v-if="(planGroups[detailPlan.id] || []).length" class="plan-sheet-groups__list">
            <li v-for="group in planGroups[detailPlan.id]" :key="group.id" class="plan-sheet-groups__item">
              <span>{{ group.name }}</span>
              <UiIconButton :icon="X" size="sm" :label="pt('actions.removeGroupNamed', { name: group.name })" @click="removeGroup(detailPlan, group)" />
            </li>
          </ul>
          <p v-else class="plan-sheet-groups__empty">{{ pt('labels.noGroupsHint') }}</p>
        </section>
        <UiGroupedList :title="pt('detail.actions')">
          <UiGroupedListRow :label="pt('actions.edit')" @click="edit(detailPlan)" />
          <UiGroupedListRow :label="pt('actions.assign')" @click="openAssign(detailPlan)" />
          <UiGroupedListRow :label="pt('confirm.deleteAction')" @click="remove(detailPlan)" />
        </UiGroupedList>
      </template>
    </UiSheet>

    <UiDialog
      :open="showPlanModal"
      size="md"
      :title="editingPlanId ? pt('planModal.editTitle') : pt('planModal.createTitle')"
      :dismissible="!planSaving"
      @update:open="value => { if (!value) closePlanModal() }"
    >
      <div class="form-grid">
        <UiTextField
          id="plan-name"
          v-model="form.name"
          class="form-grid__full"
          required
          data-test="plan-name-input"
          :label="pt('planModal.fields.name')"
          :placeholder="pt('planModal.placeholders.name')"
          :error="nameError"
        />
        <UiTextField v-model.number="form.transfer_enable" type="number" min="0" suffix="GB" :label="pt('planModal.fields.transfer')" />
        <UiTextField v-if="isCommercial" v-model.number="form.month_price" type="number" min="0" :label="pt('planModal.fields.monthPrice')" :help="formatPrice(form.month_price)" data-test="plan-month-price-field" />
        <UiTextField v-model.number="form.speed_limit" type="number" min="0" suffix="Mbps" :label="pt('planModal.fields.speedLimit')" :help="pt('planModal.help.zeroUnlimited')" data-test="plan-speed-limit-input" />
        <UiTextField v-model.number="form.device_limit" type="number" min="0" :label="pt('planModal.fields.deviceLimit')" :help="pt('planModal.help.zeroUnlimited')" data-test="plan-device-limit-input" />
      </div>
      <p v-if="planError" class="form-error" role="alert" data-test="plan-save-error">{{ planError }}</p>
      <template #footer="{ close }">
        <UiButton :disabled="planSaving" @click="close">{{ t('common.actions.cancel') }}</UiButton>
        <UiButton variant="primary" data-test="plan-save-button" :loading="planSaving" @click="save">{{ t('common.actions.save') }}</UiButton>
      </template>
    </UiDialog>

    <UiDialog
      :open="showAssign"
      size="sm"
      :title="pt('assignModal.title')"
      :description="assignPlanName"
      :dismissible="!assigning"
      @update:open="value => { if (!value) closeAssign() }"
    >
      <div class="form-grid">
        <UiTextField
          id="plan-assign-user"
          v-model.number="assignForm.user_id"
          class="form-grid__full"
          type="number"
          required
          data-test="plan-assign-user"
          :label="pt('assignModal.fields.userId')"
          :placeholder="pt('assignModal.placeholders.userId')"
          :error="assignUserError"
        />
        <UiTextField
          v-model.number="assignForm.expire_at"
          class="form-grid__full"
          type="number"
          :label="pt('assignModal.fields.expireAt')"
          :help="assignForm.expire_at ? format.dateTime(Number(assignForm.expire_at)) : pt('assignModal.help.expireAt')"
        />
      </div>
      <p v-if="assignError" class="form-error" role="alert" data-test="plan-assign-error">{{ assignError }}</p>
      <template #footer="{ close }">
        <UiButton :disabled="assigning" @click="close">{{ t('common.actions.cancel') }}</UiButton>
        <UiButton variant="primary" data-test="plan-assign-button" :loading="assigning" @click="assign">{{ pt('actions.assign') }}</UiButton>
      </template>
    </UiDialog>

    <UiDialog
      :open="showGroupModal"
      :title="pt('groupModal.title', { name: currentPlan?.name || '' })"
      :description="pt('groupModal.description')"
      @update:open="value => { if (!value) closeGroupModal() }"
    >
      <UiErrorState v-if="allGroupsError" compact heading-tag="h3" :title="pt('messages.loadGroupsFailed')" :error="allGroupsError" @retry="loadAllGroups" />
      <UiEmptyState v-else-if="allGroups.length === 0" compact :icon="Layers" :title="pt('groupModal.empty')" />
      <ul v-else class="group-list">
        <li v-for="group in allGroups" :key="group.id">
          <button
            type="button"
            class="group-item"
            :class="{ 'is-selected': isGroupSelected(group.id) }"
            :aria-pressed="isGroupSelected(group.id) ? 'true' : 'false'"
            :disabled="togglingGroupId === group.id"
            @click="toggleGroup(group)"
          >
            <span class="group-item__text">
              <span class="group-item__name">{{ group.name }}</span>
              <span class="group-item__desc">{{ group.description || pt('groupModal.noDescription') }}</span>
            </span>
            <UiIcon v-if="isGroupSelected(group.id)" class="group-item__check" :icon="Check" :size="18" />
          </button>
        </li>
      </ul>
      <p v-if="groupError" class="form-error" role="alert" data-test="plan-group-error">{{ groupError }}</p>
      <template #footer="{ close }">
        <UiButton @click="close">{{ t('common.actions.close') }}</UiButton>
      </template>
    </UiDialog>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { Check, Layers, Package, Pencil, Plus, Trash2, UserPlus, X } from '@lucide/vue'
import {
  addGroupToPlan,
  assignPlanToUser,
  createPlan,
  deletePlan,
  getPlanGroups,
  getPlans,
  getSubscriptionGroups,
  removeGroupFromPlan,
  updatePlan
} from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import { useEdition } from '@/composables/useEdition'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiDialog from '@/ui/UiDialog.vue'
import UiEmptyState from '@/ui/UiEmptyState.vue'
import UiErrorState from '@/ui/UiErrorState.vue'
import UiGroupedList from '@/ui/UiGroupedList.vue'
import UiGroupedListRow from '@/ui/UiGroupedListRow.vue'
import UiIcon from '@/ui/UiIcon.vue'
import UiIconButton from '@/ui/UiIconButton.vue'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import UiSearchField from '@/ui/UiSearchField.vue'
import UiSheet from '@/ui/UiSheet.vue'
import UiTextField from '@/ui/UiTextField.vue'
import { useConfirm } from '@/ui/composables/useConfirm'
import { useFormat } from '@/ui/composables/useFormat'
import { useToast } from '@/ui/composables/useToast'

const { t, te } = useAppI18n()
// The community edition calls plans subscription templates: no price, no
// purchase. Copy comes from adminTemplates where it differs, and a stored
// price is sent back unchanged.
const { isCommercial } = useEdition()
const toast = useToast()
const confirm = useConfirm()
const format = useFormat()
const pt = (key, params) => (
  !isCommercial.value && te(`adminTemplates.${key}`)
    ? t(`adminTemplates.${key}`, params)
    : t(`adminPlans.${key}`, params)
)

const plans = ref([])
const listLoading = ref(false)
const listError = ref(null)
const search = ref('')
const detailPlan = ref(null)
const allGroups = ref([])
const allGroupsError = ref(null)
const planGroups = ref({})

const showPlanModal = ref(false)
const showAssign = ref(false)
const showGroupModal = ref(false)

const editingPlanId = ref(null)
const currentPlan = ref(null)
const planSaving = ref(false)
const planError = ref('')
const nameError = ref('')
const assigning = ref(false)
const assignError = ref('')
const assignUserError = ref('')
const groupError = ref('')
const togglingGroupId = ref(null)

const form = reactive({
  name: '',
  transfer_enable: 0,
  speed_limit: 0,
  device_limit: 0,
  month_price: null
})

const assignForm = reactive({
  user_id: null,
  expire_at: null,
  plan_id: null
})

const resolveApiError = (error, fallbackKey) => (
  error?.response?.data?.error ||
  error?.response?.data?.message ||
  error?.response?.data?.msg ||
  error?.message ||
  t(fallbackKey)
)

const readPlanPayload = (res) => {
  if (!res || typeof res !== 'object') return null
  if (Object.prototype.hasOwnProperty.call(res, 'code')) return res.data ?? null
  if (
    res.data &&
    typeof res.data === 'object' &&
    Object.prototype.hasOwnProperty.call(res.data, 'data')
  ) {
    return res.data.data ?? null
  }
  return res.data ?? res
}

const readPlanList = (res) => {
  const payload = readPlanPayload(res)
  return Array.isArray(payload) ? payload : []
}

const ensurePlanSuccess = (res, fallbackKey) => {
  if (res && typeof res === 'object' && typeof res.code === 'number' && res.code !== 0) {
    throw new Error(res.msg || t(fallbackKey))
  }
  return res
}

const resetPlanForm = () => {
  editingPlanId.value = null
  form.name = ''
  form.transfer_enable = 0
  form.speed_limit = 0
  form.device_limit = 0
  form.month_price = null
  planError.value = ''
  nameError.value = ''
}

const resetAssignForm = () => {
  assignForm.plan_id = null
  assignForm.user_id = null
  assignForm.expire_at = null
  assignError.value = ''
  assignUserError.value = ''
}

const loadPlanGroups = async (planId) => {
  try {
    const res = ensurePlanSuccess(
      await getPlanGroups(planId),
      'adminPlans.messages.loadGroupsFailed'
    )
    planGroups.value[planId] = readPlanList(res)
  } catch {
    planGroups.value[planId] = []
  }
}

const load = async () => {
  listLoading.value = true
  try {
    const res = ensurePlanSuccess(
      await getPlans(),
      'adminPlans.messages.loadFailed'
    )
    plans.value = readPlanList(res)
    listError.value = null
    await Promise.all(plans.value.map((plan) => loadPlanGroups(plan.id)))
    // The open details follow the reloaded row.
    if (detailPlan.value) detailPlan.value = plans.value.find(plan => plan.id === detailPlan.value.id) || null
  } catch (error) {
    listError.value = error
  } finally {
    listLoading.value = false
  }
}

const formatTransfer = value => `${format.number(Number(value || 0))} GB`
const formatPrice = value => (value === null || value === undefined || value === '' ? '—' : format.money(value))

const filteredPlans = computed(() => {
  const query = search.value.trim().toLowerCase()
  if (!query) return plans.value
  return plans.value.filter(plan => String(plan.name || '').toLowerCase().includes(query) || String(plan.id) === query)
})

const columns = computed(() => [
  { key: 'name', label: pt('table.name'), primary: true, sortable: true },
  { key: 'transfer_enable', label: pt('table.transfer'), sortable: true, numeric: true, firstDirection: 'desc', format: value => formatTransfer(value) },
  { key: 'limits', label: pt('table.limits'), value: plan => formatPlanLimits(plan) },
  ...(isCommercial.value
    ? [{ key: 'month_price', label: pt('table.monthPrice'), sortable: true, numeric: true, align: 'end', format: value => formatPrice(value) }]
    : []),
  { key: 'groups', label: pt('table.subscriptionGroups'), secondary: true, sortValue: plan => (planGroups.value[plan.id] || []).length },
  { key: 'id', label: t('miscPages.shared.id'), numeric: true, sortable: true, hidden: true }
])

const planActions = plan => [
  { key: 'edit', label: pt('actions.edit'), icon: Pencil, onSelect: () => edit(plan) },
  { key: 'groups', label: pt('actions.manageGroups'), icon: Layers, onSelect: () => openGroupModal(plan) },
  { key: 'assign', label: pt('actions.assign'), icon: UserPlus, onSelect: () => openAssign(plan) },
  { key: 'delete', label: pt('actions.delete'), icon: Trash2, danger: true, separatorBefore: true, onSelect: () => remove(plan) }
]

const openDetail = (plan) => { detailPlan.value = plan }
const assignPlanName = computed(() => plans.value.find(plan => plan.id === assignForm.plan_id)?.name || '')

const loadAllGroups = async () => {
  try {
    const res = ensurePlanSuccess(
      await getSubscriptionGroups(),
      'adminPlans.messages.loadGroupsFailed'
    )
    allGroups.value = readPlanList(res)
    allGroupsError.value = null
  } catch (error) {
    allGroupsError.value = error
  }
}

onMounted(() => {
  load()
  loadAllGroups()
})

const openCreateModal = () => {
  resetPlanForm()
  showPlanModal.value = true
}

const edit = (plan) => {
  editingPlanId.value = plan.id
  form.name = plan.name || ''
  form.transfer_enable = plan.transfer_enable || 0
  form.speed_limit = Number(plan.speed_limit || 0)
  form.device_limit = Number(plan.device_limit || 0)
  form.month_price = plan.month_price
  planError.value = ''
  nameError.value = ''
  showPlanModal.value = true
}

const remove = async (plan) => {
  const confirmed = await confirm({
    title: pt('confirm.deleteTitle', { name: plan.name }),
    message: pt('confirm.deleteMessage'),
    confirmLabel: pt('confirm.deleteAction'),
    tone: 'danger',
    onConfirm: async () => {
      try {
        ensurePlanSuccess(
          await deletePlan(plan.id),
          'adminPlans.messages.deleteFailedShort'
        )
      } catch (error) {
        throw new Error(resolveApiError(error, 'adminPlans.messages.deleteFailedShort'))
      }
    }
  })
  if (!confirmed) return
  toast.success(pt('messages.deleted', { name: plan.name }))
  await load()
}

const closePlanModal = () => {
  showPlanModal.value = false
  resetPlanForm()
}

const save = async () => {
  if (planSaving.value) return
  planError.value = ''
  nameError.value = ''
  if (!form.name || form.name.trim() === '') {
    nameError.value = pt('messages.nameRequired')
    return
  }

  const payload = {
    name: form.name.trim(),
    transfer_enable: form.transfer_enable,
    speed_limit: Number(form.speed_limit || 0),
    device_limit: Number(form.device_limit || 0),
    month_price: form.month_price
  }

  planSaving.value = true
  try {
    if (editingPlanId.value) {
      ensurePlanSuccess(
        await updatePlan(editingPlanId.value, payload),
        'adminPlans.messages.saveFailedShort'
      )
    } else {
      ensurePlanSuccess(
        await createPlan(payload),
        'adminPlans.messages.saveFailedShort'
      )
    }
    closePlanModal()
    toast.success(pt('messages.saved', { name: payload.name }))
    await load()
  } catch (error) {
    planError.value = pt('messages.saveFailed', {
      message: resolveApiError(error, 'adminPlans.messages.saveFailedShort')
    })
  } finally {
    planSaving.value = false
  }
}

const openAssign = (plan) => {
  resetAssignForm()
  assignForm.plan_id = plan.id
  showAssign.value = true
}

const closeAssign = () => {
  showAssign.value = false
  resetAssignForm()
}

const assign = async () => {
  if (assigning.value) return
  assignError.value = ''
  assignUserError.value = ''
  if (!assignForm.user_id) {
    assignUserError.value = pt('messages.userIdRequired')
    return
  }

  assigning.value = true
  try {
    ensurePlanSuccess(
      await assignPlanToUser(assignForm.plan_id, {
        user_id: assignForm.user_id,
        expire_at: assignForm.expire_at
      }),
      'adminPlans.messages.assignFailedShort'
    )
    closeAssign()
    toast.success(pt('messages.assignSuccess'))
  } catch (error) {
    assignError.value = pt('messages.assignFailed', {
      message: resolveApiError(error, 'adminPlans.messages.assignFailedShort')
    })
  } finally {
    assigning.value = false
  }
}

const openGroupModal = (plan) => {
  currentPlan.value = plan
  groupError.value = ''
  showGroupModal.value = true
}

const closeGroupModal = () => {
  showGroupModal.value = false
  currentPlan.value = null
}

const isGroupSelected = (groupId) => {
  if (!currentPlan.value) return false
  const groups = planGroups.value[currentPlan.value.id] || []
  return groups.some((group) => group.id === groupId)
}

const toggleGroup = async (group) => {
  if (!currentPlan.value) return

  const planId = currentPlan.value.id
  groupError.value = ''
  togglingGroupId.value = group.id

  try {
    if (isGroupSelected(group.id)) {
      ensurePlanSuccess(
        await removeGroupFromPlan(planId, group.id),
        'adminPlans.messages.toggleGroupFailedShort'
      )
    } else {
      ensurePlanSuccess(
        await addGroupToPlan(planId, group.id),
        'adminPlans.messages.toggleGroupFailedShort'
      )
    }
    await loadPlanGroups(planId)
  } catch (error) {
    groupError.value = pt('messages.toggleGroupFailed', {
      message: resolveApiError(error, 'adminPlans.messages.toggleGroupFailedShort')
    })
  } finally {
    togglingGroupId.value = null
  }
}

// Removing a group from a plan is undone by adding it back: no
// confirmation, 撤销 in the toast (redesign plan §9).
const removeGroup = async (plan, group) => {
  try {
    ensurePlanSuccess(
      await removeGroupFromPlan(plan.id, group.id),
      'adminPlans.messages.removeGroupFailedShort'
    )
    await loadPlanGroups(plan.id)
  } catch (error) {
    toast.error(pt('messages.removeGroupFailed', {
      message: resolveApiError(error, 'adminPlans.messages.removeGroupFailedShort')
    }))
    return
  }
  toast.success(pt('messages.groupRemoved', { group: group.name, plan: plan.name }), {
    undo: async () => {
      try {
        ensurePlanSuccess(
          await addGroupToPlan(plan.id, group.id),
          'adminPlans.messages.toggleGroupFailedShort'
        )
      } catch (error) {
        toast.error(pt('messages.toggleGroupFailed', {
          message: resolveApiError(error, 'adminPlans.messages.toggleGroupFailedShort')
        }))
      }
      await loadPlanGroups(plan.id)
    }
  })
}

const formatPlanLimits = (plan) => {
  const speedLimit = Number(plan?.speed_limit || 0)
  const deviceLimit = Number(plan?.device_limit || 0)
  const speedText = speedLimit > 0 ? pt('labels.speedLimitMbps', { value: speedLimit }) : pt('labels.noSpeedLimit')
  const deviceText = deviceLimit > 0 ? pt('labels.deviceLimitCount', { value: deviceLimit }) : pt('labels.noDeviceLimit')
  return `${speedText} / ${deviceText}`
}
</script>

<style scoped>
.plan-groups {
  display: inline-flex;
  flex-wrap: wrap;
  gap: var(--space-1);
}

.plan-groups__none,
.plan-sheet-groups__empty {
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.plan-sheet-groups {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.plan-sheet-groups__head {
  display: flex;
  gap: var(--space-2);
  align-items: center;
  justify-content: space-between;
  padding: 0 var(--space-4);
}

.plan-sheet-groups__title {
  color: var(--label-2);
  font-size: var(--type-callout-size);
  font-weight: var(--weight-regular);
}

.plan-sheet-groups__list {
  display: flex;
  flex-direction: column;
  padding: 0;
  margin: 0;
  border-radius: var(--radius-md);
  background: var(--bg-elevated);
  list-style: none;
}

.plan-sheet-groups__item {
  display: flex;
  gap: var(--space-2);
  align-items: center;
  justify-content: space-between;
  min-height: 44px;
  padding: var(--space-1) var(--space-2) var(--space-1) var(--space-4);
  border-bottom: 1px solid var(--separator);
}

.plan-sheet-groups__item:last-child {
  border-bottom: 0;
}

.plan-sheet-groups__empty {
  padding: 0 var(--space-4);
}

.group-list {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: 0;
  margin: 0;
  list-style: none;
}

.group-item {
  display: flex;
  gap: var(--space-3);
  align-items: center;
  justify-content: space-between;
  width: 100%;
  min-height: 52px;
  padding: var(--space-2) var(--space-4);
  border: 1px solid var(--separator-strong);
  border-radius: var(--radius-sm);
  background: var(--bg-elevated);
  color: var(--label-1);
  font: inherit;
  text-align: left;
  white-space: normal;
  cursor: pointer;
}

.group-item:hover {
  background: var(--fill-1);
}

.group-item:focus-visible {
  outline: var(--focus-ring);
  outline-offset: var(--focus-ring-offset);
}

.group-item.is-selected {
  border-color: var(--accent);
  background: var(--accent-soft);
}

.group-item__text {
  display: flex;
  flex-direction: column;
  gap: var(--space-0-5);
  min-width: 0;
}

.group-item__name {
  font-weight: var(--weight-semibold);
}

.group-item__desc {
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.group-item.is-selected .group-item__desc {
  /* --label-2 on the accent tint stays at 4.5:1. */
  color: color-mix(in srgb, var(--label-2) 70%, var(--label-1));
}

.group-item__check {
  color: var(--accent);
}
</style>
