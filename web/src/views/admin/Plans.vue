<template>
  <div class="page-shell">
    <div class="page-toolbar">
      <div>
        <h1>{{ t('adminPlans.title') }}</h1>
        <p>{{ t('adminPlans.subtitle') }}</p>
      </div>
      <button class="btn btn-primary" @click="openCreateModal">{{ t('adminPlans.actions.create') }}</button>
    </div>

    <section class="section-panel data-panel">
      <table class="data-table">
        <thead>
          <tr>
            <th>{{ t('miscPages.shared.id') }}</th>
            <th>{{ t('adminPlans.table.name') }}</th>
            <th>{{ t('adminPlans.table.transfer') }}</th>
            <th>{{ t('adminPlans.table.limits') }}</th>
            <th>{{ t('adminPlans.table.monthPrice') }}</th>
            <th>{{ t('adminPlans.table.subscriptionGroups') }}</th>
            <th>{{ t('adminPlans.table.actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="plan in plans" :key="plan.id">
            <td>{{ plan.id }}</td>
            <td>{{ plan.name }}</td>
            <td>{{ plan.transfer_enable }}</td>
            <td>{{ formatPlanLimits(plan) }}</td>
            <td>{{ plan.month_price ?? '-' }}</td>
            <td>
              <div class="group-tags">
                <span v-for="group in (planGroups[plan.id] || [])" :key="group.id" class="group-tag">
                  {{ group.name }}
                  <button
                    class="tag-remove"
                    :title="t('adminPlans.actions.removeGroup')"
                    :aria-label="t('adminPlans.actions.removeGroup')"
                    @click="removeGroup(plan.id, group.id)"
                  >
                    x
                  </button>
                </span>
                <button
                  class="btn btn-sm"
                  :title="t('adminPlans.actions.manageGroups')"
                  :aria-label="t('adminPlans.actions.manageGroups')"
                  @click="openGroupModal(plan)"
                >
                  {{ t('adminPlans.actions.manageGroups') }}
                </button>
              </div>
            </td>
            <td>
              <div class="action-buttons">
                <button
                  class="btn btn-sm"
                  :title="t('adminPlans.actions.edit')"
                  :aria-label="t('adminPlans.actions.edit')"
                  @click="edit(plan)"
                >
                  {{ t('adminPlans.actions.edit') }}
                </button>
                <button
                  class="btn btn-sm"
                  :title="t('adminPlans.actions.delete')"
                  :aria-label="t('adminPlans.actions.delete')"
                  @click="remove(plan)"
                >
                  {{ t('adminPlans.actions.delete') }}
                </button>
                <button
                  class="btn btn-sm"
                  :title="t('adminPlans.actions.assign')"
                  :aria-label="t('adminPlans.actions.assign')"
                  @click="openAssign(plan)"
                >
                  {{ t('adminPlans.actions.assign') }}
                </button>
              </div>
            </td>
          </tr>
          <tr v-if="plans.length === 0">
            <td colspan="7" class="empty-row">{{ t('adminPlans.empty.noData') }}</td>
          </tr>
        </tbody>
      </table>
    </section>

    <div v-if="showPlanModal" class="modal-overlay" @click.self="closePlanModal">
      <div class="modal">
        <div class="modal-header">
          <h3>{{ editingPlanId ? t('adminPlans.planModal.editTitle') : t('adminPlans.planModal.createTitle') }}</h3>
          <button class="btn btn-ghost btn-sm close-btn" :title="t('common.actions.close')" :aria-label="t('common.actions.close')" @click="closePlanModal">x</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label>{{ t('adminPlans.planModal.fields.name') }} <span class="required">*</span></label>
            <input v-model="form.name" type="text" :placeholder="t('adminPlans.planModal.placeholders.name')" />
          </div>
          <div class="form-group">
            <label>{{ t('adminPlans.planModal.fields.transfer') }}</label>
            <input v-model.number="form.transfer_enable" type="number" min="0" />
          </div>
          <div class="form-group">
            <label>{{ t('adminPlans.planModal.fields.speedLimit') }}</label>
            <input v-model.number="form.speed_limit" data-test="plan-speed-limit-input" type="number" min="0" />
          </div>
          <div class="form-group">
            <label>{{ t('adminPlans.planModal.fields.deviceLimit') }}</label>
            <input v-model.number="form.device_limit" data-test="plan-device-limit-input" type="number" min="0" />
          </div>
          <div class="form-group">
            <label>{{ t('adminPlans.planModal.fields.monthPrice') }}</label>
            <input v-model.number="form.month_price" type="number" min="0" />
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn" @click="closePlanModal">{{ t('common.actions.cancel') }}</button>
          <button class="btn btn-primary" data-test="plan-save-button" @click="save">{{ t('common.actions.save') }}</button>
        </div>
      </div>
    </div>

    <div v-if="showAssign" class="modal-overlay" @click.self="closeAssign">
      <div class="modal">
        <div class="modal-header">
          <h3>{{ t('adminPlans.assignModal.title') }}</h3>
          <button class="btn btn-ghost btn-sm close-btn" :title="t('common.actions.close')" :aria-label="t('common.actions.close')" @click="closeAssign">x</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label>{{ t('adminPlans.assignModal.fields.userId') }} <span class="required">*</span></label>
            <input
              v-model.number="assignForm.user_id"
              type="number"
              :placeholder="t('adminPlans.assignModal.placeholders.userId')"
            />
          </div>
          <div class="form-group">
            <label>{{ t('adminPlans.assignModal.fields.expireAt') }}</label>
            <input v-model.number="assignForm.expire_at" type="number" />
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn" @click="closeAssign">{{ t('common.actions.cancel') }}</button>
          <button class="btn btn-primary" @click="assign">{{ t('adminPlans.actions.assign') }}</button>
        </div>
      </div>
    </div>

    <div v-if="showGroupModal" class="modal-overlay" @click.self="closeGroupModal">
      <div class="modal">
        <div class="modal-header">
          <h3>{{ t('adminPlans.groupModal.title', { name: currentPlan?.name || '' }) }}</h3>
          <button class="btn btn-ghost btn-sm close-btn" :title="t('common.actions.close')" :aria-label="t('common.actions.close')" @click="closeGroupModal">x</button>
        </div>
        <div class="modal-body">
          <p class="group-description">{{ t('adminPlans.groupModal.description') }}</p>

          <div v-if="allGroups.length === 0" class="empty-msg">
            {{ t('adminPlans.groupModal.empty') }}
          </div>

          <div v-else class="group-list">
            <div
              v-for="group in allGroups"
              :key="group.id"
              class="group-item"
              :class="{ selected: isGroupSelected(group.id) }"
              @click="toggleGroup(group)"
            >
              <div class="group-info">
                <div class="group-name">{{ group.name }}</div>
                <div class="group-desc">{{ group.description || t('adminPlans.groupModal.noDescription') }}</div>
              </div>
              <div class="group-check">
                <span>{{ isGroupSelected(group.id) ? t('adminPlans.groupModal.selectedShort') : '' }}</span>
              </div>
            </div>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn" @click="closeGroupModal">{{ t('common.actions.close') }}</button>
        </div>
      </div>
    </div>
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

const { t } = useAppI18n()

const plans = ref([])
const allGroups = ref([])
const planGroups = ref({})

const showPlanModal = ref(false)
const showAssign = ref(false)
const showGroupModal = ref(false)

const editingPlanId = ref(null)
const currentPlan = ref(null)

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

const resetPlanForm = () => {
  editingPlanId.value = null
  form.name = ''
  form.transfer_enable = 0
  form.speed_limit = 0
  form.device_limit = 0
  form.month_price = null
}

const resetAssignForm = () => {
  assignForm.plan_id = null
  assignForm.user_id = null
  assignForm.expire_at = null
}

const loadPlanGroups = async (planId) => {
  try {
    const res = await getPlanGroups(planId)
    planGroups.value[planId] = readPlanList(res)
  } catch {
    planGroups.value[planId] = []
  }
}

const load = async () => {
  try {
    const res = await getPlans()
    plans.value = readPlanList(res)
    await Promise.all(plans.value.map((plan) => loadPlanGroups(plan.id)))
  } catch (error) {
    console.error(t('adminPlans.messages.loadFailed'), error)
  }
}

const loadAllGroups = async () => {
  try {
    const res = await getSubscriptionGroups()
    allGroups.value = readPlanList(res)
  } catch (error) {
    console.error(t('adminPlans.messages.loadGroupsFailed'), error)
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
  showPlanModal.value = true
}

const remove = async (plan) => {
  if (!window.confirm(t('adminPlans.messages.deleteConfirm'))) {
    return
  }

  try {
    await deletePlan(plan.id)
    await load()
  } catch (error) {
    window.alert(t('adminPlans.messages.deleteFailed', {
      message: resolveApiError(error, 'adminPlans.messages.deleteFailedShort')
    }))
  }
}

const closePlanModal = () => {
  showPlanModal.value = false
  resetPlanForm()
}

const save = async () => {
  if (!form.name || form.name.trim() === '') {
    window.alert(t('adminPlans.messages.nameRequired'))
    return
  }

  const payload = {
    name: form.name.trim(),
    transfer_enable: form.transfer_enable,
    speed_limit: Number(form.speed_limit || 0),
    device_limit: Number(form.device_limit || 0),
    month_price: form.month_price
  }

  try {
    if (editingPlanId.value) {
      await updatePlan(editingPlanId.value, payload)
    } else {
      await createPlan(payload)
    }
    closePlanModal()
    await load()
  } catch (error) {
    window.alert(t('adminPlans.messages.saveFailed', {
      message: resolveApiError(error, 'adminPlans.messages.saveFailedShort')
    }))
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
  if (!assignForm.user_id) {
    window.alert(t('adminPlans.messages.userIdRequired'))
    return
  }

  try {
    await assignPlanToUser(assignForm.plan_id, {
      user_id: assignForm.user_id,
      expire_at: assignForm.expire_at
    })
    closeAssign()
    window.alert(t('adminPlans.messages.assignSuccess'))
  } catch (error) {
    window.alert(t('adminPlans.messages.assignFailed', {
      message: resolveApiError(error, 'adminPlans.messages.assignFailedShort')
    }))
  }
}

const openGroupModal = (plan) => {
  currentPlan.value = plan
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

  try {
    if (isGroupSelected(group.id)) {
      await removeGroupFromPlan(planId, group.id)
    } else {
      await addGroupToPlan(planId, group.id)
    }
    await loadPlanGroups(planId)
  } catch (error) {
    window.alert(t('adminPlans.messages.toggleGroupFailed', {
      message: resolveApiError(error, 'adminPlans.messages.toggleGroupFailedShort')
    }))
  }
}

const removeGroup = async (planId, groupId) => {
  if (!window.confirm(t('adminPlans.messages.removeGroupConfirm'))) {
    return
  }

  try {
    await removeGroupFromPlan(planId, groupId)
    await loadPlanGroups(planId)
  } catch (error) {
    window.alert(t('adminPlans.messages.removeGroupFailed', {
      message: resolveApiError(error, 'adminPlans.messages.removeGroupFailedShort')
    }))
  }
}

const formatPlanLimits = (plan) => {
  const speedLimit = Number(plan?.speed_limit || 0)
  const deviceLimit = Number(plan?.device_limit || 0)
  const speedText = speedLimit > 0 ? t('adminPlans.labels.speedLimitMbps', { value: speedLimit }) : t('adminPlans.labels.noSpeedLimit')
  const deviceText = deviceLimit > 0 ? t('adminPlans.labels.deviceLimitCount', { value: deviceLimit }) : t('adminPlans.labels.noDeviceLimit')
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

.modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(15, 23, 42, 0.42);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  padding: 20px;
}

.modal {
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: 8px;
  width: 100%;
  max-width: 500px;
  max-height: 90vh;
  overflow-y: auto;
  box-shadow: var(--shadow-lg);
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
  min-width: 36px;
  font-size: 18px;
}

.modal-body {
  padding: 20px;
}

.modal-body .form-group {
  margin-bottom: 16px;
}

.modal-body label {
  display: block;
  margin-bottom: 6px;
  font-size: 13px;
  font-weight: 700;
  color: var(--text-secondary);
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
  flex-wrap: wrap;
}

.empty-msg {
  text-align: center;
  color: var(--text-secondary);
  padding: 30px 20px;
}

.group-description {
  margin-bottom: 16px;
  color: var(--text-secondary);
}

.group-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.group-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  background: var(--surface-muted);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: var(--transition);
}

.group-item:hover {
  border-color: var(--border-strong);
}

.group-item.selected {
  background: rgba(0, 100, 250, 0.06);
  border-color: var(--primary-color);
}

.group-info {
  flex: 1;
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
  background: var(--primary-color);
  color: white;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 700;
}

.group-item:not(.selected) .group-check {
  background: var(--border-color);
  color: transparent;
}
</style>
