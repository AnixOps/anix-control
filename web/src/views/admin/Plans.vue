<template>
  <div class="page-shell">
    <div class="page-toolbar">
      <div>
        <h1>{{ pt('title') }}</h1>
        <p>{{ pt('subtitle') }}</p>
      </div>
      <button class="btn btn-primary" @click="openCreateModal">{{ pt('actions.create') }}</button>
    </div>

    <section class="section-panel data-panel">
      <table class="data-table">
        <thead>
          <tr>
            <th>{{ t('miscPages.shared.id') }}</th>
            <th>{{ pt('table.name') }}</th>
            <th>{{ pt('table.transfer') }}</th>
            <th>{{ pt('table.limits') }}</th>
            <th v-if="isCommercial">{{ pt('table.monthPrice') }}</th>
            <th>{{ pt('table.subscriptionGroups') }}</th>
            <th>{{ pt('table.actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="plan in plans" :key="plan.id">
            <td>{{ plan.id }}</td>
            <td>{{ plan.name }}</td>
            <td>{{ plan.transfer_enable }}</td>
            <td>{{ formatPlanLimits(plan) }}</td>
            <td v-if="isCommercial">{{ plan.month_price ?? '-' }}</td>
            <td>
              <div class="group-tags">
                <span v-for="group in (planGroups[plan.id] || [])" :key="group.id" class="group-tag">
                  {{ group.name }}
                  <button
                    class="tag-remove"
                    :title="pt('actions.removeGroup')"
                    :aria-label="pt('actions.removeGroup')"
                    @click="removeGroup(plan, group)"
                  >
                    ×
                  </button>
                </span>
                <button
                  class="btn btn-sm"
                  :title="pt('actions.manageGroups')"
                  :aria-label="pt('actions.manageGroups')"
                  @click="openGroupModal(plan)"
                >
                  {{ pt('actions.manageGroups') }}
                </button>
              </div>
            </td>
            <td>
              <div class="action-buttons">
                <button
                  class="btn btn-sm"
                  :title="pt('actions.edit')"
                  :aria-label="pt('actions.edit')"
                  @click="edit(plan)"
                >
                  {{ pt('actions.edit') }}
                </button>
                <button
                  class="btn btn-sm"
                  :title="pt('actions.delete')"
                  :aria-label="pt('actions.delete')"
                  @click="remove(plan)"
                >
                  {{ pt('actions.delete') }}
                </button>
                <button
                  class="btn btn-sm"
                  :title="pt('actions.assign')"
                  :aria-label="pt('actions.assign')"
                  @click="openAssign(plan)"
                >
                  {{ pt('actions.assign') }}
                </button>
              </div>
            </td>
          </tr>
          <tr v-if="plans.length === 0">
            <td :colspan="isCommercial ? 7 : 6" class="empty-row">{{ pt('empty.noData') }}</td>
          </tr>
        </tbody>
      </table>
    </section>

    <UiDialog
      :open="showPlanModal"
      size="sm"
      :title="editingPlanId ? pt('planModal.editTitle') : pt('planModal.createTitle')"
      :dismissible="!planSaving"
      @update:open="value => { if (!value) closePlanModal() }"
    >
      <div class="dialog-form">
        <div class="form-group">
          <label for="plan-name">{{ pt('planModal.fields.name') }} <span class="required">*</span></label>
          <input
            id="plan-name"
            v-model="form.name"
            type="text"
            data-test="plan-name-input"
            :placeholder="pt('planModal.placeholders.name')"
            :aria-invalid="nameError ? 'true' : undefined"
            :aria-describedby="nameError ? 'plan-name-error' : undefined"
          />
          <p v-if="nameError" id="plan-name-error" class="form-error" role="alert">{{ nameError }}</p>
        </div>
        <div class="form-group">
          <label for="plan-transfer">{{ pt('planModal.fields.transfer') }}</label>
          <input id="plan-transfer" v-model.number="form.transfer_enable" type="number" min="0" />
        </div>
        <div class="form-group">
          <label for="plan-speed-limit">{{ pt('planModal.fields.speedLimit') }}</label>
          <input id="plan-speed-limit" v-model.number="form.speed_limit" data-test="plan-speed-limit-input" type="number" min="0" />
        </div>
        <div class="form-group">
          <label for="plan-device-limit">{{ pt('planModal.fields.deviceLimit') }}</label>
          <input id="plan-device-limit" v-model.number="form.device_limit" data-test="plan-device-limit-input" type="number" min="0" />
        </div>
        <div v-if="isCommercial" class="form-group" data-test="plan-month-price-field">
          <label for="plan-month-price">{{ pt('planModal.fields.monthPrice') }}</label>
          <input id="plan-month-price" v-model.number="form.month_price" type="number" min="0" />
        </div>
        <p v-if="planError" class="form-error" role="alert" data-test="plan-save-error">{{ planError }}</p>
      </div>
      <template #footer="{ close }">
        <UiButton :disabled="planSaving" @click="close">{{ t('common.actions.cancel') }}</UiButton>
        <UiButton variant="primary" data-test="plan-save-button" :loading="planSaving" @click="save">{{ t('common.actions.save') }}</UiButton>
      </template>
    </UiDialog>

    <UiDialog
      :open="showAssign"
      size="sm"
      :title="pt('assignModal.title')"
      :dismissible="!assigning"
      @update:open="value => { if (!value) closeAssign() }"
    >
      <div class="dialog-form">
        <div class="form-group">
          <label for="plan-assign-user">{{ pt('assignModal.fields.userId') }} <span class="required">*</span></label>
          <input
            id="plan-assign-user"
            v-model.number="assignForm.user_id"
            type="number"
            data-test="plan-assign-user"
            :placeholder="pt('assignModal.placeholders.userId')"
            :aria-invalid="assignUserError ? 'true' : undefined"
            :aria-describedby="assignUserError ? 'plan-assign-user-error' : undefined"
          />
          <p v-if="assignUserError" id="plan-assign-user-error" class="form-error" role="alert">{{ assignUserError }}</p>
        </div>
        <div class="form-group">
          <label for="plan-assign-expire">{{ pt('assignModal.fields.expireAt') }}</label>
          <input id="plan-assign-expire" v-model.number="assignForm.expire_at" type="number" />
        </div>
        <p v-if="assignError" class="form-error" role="alert" data-test="plan-assign-error">{{ assignError }}</p>
      </div>
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
      <div v-if="allGroups.length === 0" class="empty-msg">
        {{ pt('groupModal.empty') }}
      </div>

      <div v-else class="group-list">
        <button
          v-for="group in allGroups"
          :key="group.id"
          type="button"
          class="group-item"
          :class="{ selected: isGroupSelected(group.id) }"
          :aria-pressed="isGroupSelected(group.id) ? 'true' : 'false'"
          :disabled="togglingGroupId === group.id"
          @click="toggleGroup(group)"
        >
          <span class="group-info">
            <span class="group-name">{{ group.name }}</span>
            <span class="group-desc">{{ group.description || pt('groupModal.noDescription') }}</span>
          </span>
          <span class="group-check" aria-hidden="true">
            <span>{{ isGroupSelected(group.id) ? pt('groupModal.selectedShort') : '' }}</span>
          </span>
        </button>
      </div>
      <p v-if="groupError" class="form-error" role="alert" data-test="plan-group-error">{{ groupError }}</p>
      <template #footer="{ close }">
        <UiButton @click="close">{{ t('common.actions.close') }}</UiButton>
      </template>
    </UiDialog>
  </div>
</template>

<script setup>
import { onMounted, reactive, ref } from 'vue'
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
import { UiButton, UiDialog, useConfirm, useToast } from '@/ui'

const { t, te } = useAppI18n()
// The community edition calls plans subscription templates: no price, no
// purchase. Copy comes from adminTemplates where it differs, and a stored
// price is sent back unchanged.
const { isCommercial } = useEdition()
const toast = useToast()
const confirm = useConfirm()
const pt = (key, params) => (
  !isCommercial.value && te(`adminTemplates.${key}`)
    ? t(`adminTemplates.${key}`, params)
    : t(`adminPlans.${key}`, params)
)

const plans = ref([])
const allGroups = ref([])
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
  try {
    const res = ensurePlanSuccess(
      await getPlans(),
      'adminPlans.messages.loadFailed'
    )
    plans.value = readPlanList(res)
    await Promise.all(plans.value.map((plan) => loadPlanGroups(plan.id)))
  } catch (error) {
    console.error(pt('messages.loadFailed'), error)
  }
}

const loadAllGroups = async () => {
  try {
    const res = ensurePlanSuccess(
      await getSubscriptionGroups(),
      'adminPlans.messages.loadGroupsFailed'
    )
    allGroups.value = readPlanList(res)
  } catch (error) {
    console.error(pt('messages.loadGroupsFailed'), error)
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
.data-panel {
  overflow-x: auto;
  padding: 0;
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
  vertical-align: top;
}

.data-table th {
  font-weight: 600;
  font-size: 13px;
  color: var(--text-secondary);
  text-transform: uppercase;
}

.group-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
}

.group-tag {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  background: var(--primary-soft);
  color: var(--primary-color);
  padding: 4px 10px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 700;
}

.tag-remove {
  background: transparent;
  border: none;
  color: inherit;
  cursor: pointer;
  padding: 0;
  font-size: 14px;
  line-height: 1;
  margin-left: 2px;
  box-shadow: none;
}

.action-buttons {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

.empty-row {
  text-align: center;
  color: var(--text-secondary);
  padding: 40px !important;
}

.dialog-form .form-group:last-child {
  margin-bottom: 0;
}

.form-error {
  margin: var(--space-1) 0 0;
  color: var(--danger);
  font-size: var(--type-callout-size);
}

.empty-msg {
  text-align: center;
  color: var(--text-secondary);
  padding: 30px 20px;
}

.group-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.group-item {
  display: flex;
  width: 100%;
  min-height: auto;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  color: var(--label-1);
  font: inherit;
  text-align: left;
  padding: 12px 16px;
  background: var(--surface-muted);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: var(--transition);
}

.group-item:focus-visible {
  outline: var(--focus-ring);
  outline-offset: var(--focus-ring-offset);
}

.group-item:hover {
  border-color: var(--border-strong);
}

.group-item.selected {
  background: rgba(0, 100, 250, 0.06);
  border-color: var(--primary-color);
}

.group-info {
  display: flex;
  flex: 1;
  flex-direction: column;
}

.group-name {
  font-weight: 600;
  margin-bottom: 2px;
}

.group-desc {
  font-size: 12px;
  color: var(--text-secondary);
}

.group-check {
  min-width: 48px;
  height: 24px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--accent-fill);
  color: var(--on-accent);
  border-radius: 999px;
  font-size: 12px;
  font-weight: 700;
}

.group-item:not(.selected) .group-check {
  background: var(--border-color);
  color: transparent;
}
</style>
