<template>
  <div class="plans-page">
    <div class="page-header">
      <h1>套餐管理</h1>
      <p class="text-secondary">创建、编辑、删除套餐并管理订阅分组关联</p>
    </div>

    <!-- 筛选栏 -->
    <div class="filter-bar">
      <button @click="showCreate = true">➕ 新建套餐</button>
    </div>

    <!-- 套餐列表 -->
    <div class="table-container">
      <table class="data-table">
        <thead>
          <tr>
            <th>ID</th>
            <th>名称</th>
            <th>流量(GB)</th>
            <th>月价(分)</th>
            <th>关联订阅分组</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="p in plans" :key="p.id">
            <td>{{ p.id }}</td>
            <td>{{ p.name }}</td>
            <td>{{ p.transfer_enable }}</td>
            <td>{{ p.month_price || '-' }}</td>
            <td>
              <div class="group-tags">
                <span v-for="g in (planGroups[p.id] || [])" :key="g.id" class="group-tag">
                  {{ g.name }}
                  <button class="tag-remove" @click="removeGroup(p.id, g.id)" title="移除">×</button>
                </span>
                <button class="btn-sm btn-ghost" @click="openGroupModal(p)" title="管理分组">➕</button>
              </div>
            </td>
            <td>
              <div class="action-buttons">
                <button class="btn-sm btn-ghost" @click="edit(p)" title="编辑">✏️</button>
                <button class="btn-sm btn-ghost" @click="remove(p)" title="删除">🗑️</button>
                <button class="btn-sm btn-ghost" @click="openAssign(p)" title="分配给用户">👤</button>
              </div>
            </td>
          </tr>
          <tr v-if="plans.length === 0">
            <td colspan="6" class="empty-row">暂无套餐</td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Create/Edit Modal -->
    <div v-if="showCreate || showEdit" class="modal-overlay" @click.self="closeModal">
      <div class="modal">
        <div class="modal-header">
          <h3>{{ showEdit ? '编辑套餐' : '新建套餐' }}</h3>
          <button class="close-btn" @click="closeModal">✕</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label>名称 <span class="required">*</span></label>
            <input v-model="form.name" type="text" placeholder="套餐名称">
          </div>
          <div class="form-group">
            <label>流量(GB)</label>
            <input v-model.number="form.transfer_enable" type="number" min="0">
          </div>
          <div class="form-group">
            <label>月价(分)</label>
            <input v-model.number="form.month_price" type="number" min="0">
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn-secondary" @click="closeModal">取消</button>
          <button @click="save">保存</button>
        </div>
      </div>
    </div>

    <!-- Assign to User Modal -->
    <div v-if="showAssign" class="modal-overlay" @click.self="closeAssign">
      <div class="modal">
        <div class="modal-header">
          <h3>将套餐分配给用户</h3>
          <button class="close-btn" @click="closeAssign">✕</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label>用户 ID <span class="required">*</span></label>
            <input v-model.number="assignForm.user_id" type="number" placeholder="输入用户ID">
          </div>
          <div class="form-group">
            <label>过期时间 (Unix 秒，留空表示不变)</label>
            <input v-model.number="assignForm.expire_at" type="number">
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn-secondary" @click="closeAssign">取消</button>
          <button @click="assign">分配</button>
        </div>
      </div>
    </div>

    <!-- Manage Groups Modal -->
    <div v-if="showGroupModal" class="modal-overlay" @click.self="closeGroupModal">
      <div class="modal">
        <div class="modal-header">
          <h3>管理订阅分组 - {{ currentPlan?.name }}</h3>
          <button class="close-btn" @click="closeGroupModal">✕</button>
        </div>
        <div class="modal-body">
          <p class="text-secondary" style="margin-bottom: 16px;">选择要关联到此套餐的订阅分组，用户购买此套餐后将自动获得所选分组的订阅权限。</p>
          
          <div v-if="allGroups.length === 0" class="empty-msg">
            暂无可用的订阅分组，请先在"订阅管理"中创建分组。
          </div>
          
          <div v-else class="group-list">
            <div 
              v-for="g in allGroups" 
              :key="g.id" 
              class="group-item"
              :class="{ selected: isGroupSelected(g.id) }"
              @click="toggleGroup(g)"
            >
              <div class="group-info">
                <div class="group-name">{{ g.name }}</div>
                <div class="group-desc text-secondary">{{ g.description || '无描述' }}</div>
              </div>
              <div class="group-check">
                <span v-if="isGroupSelected(g.id)">✓</span>
              </div>
            </div>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn-secondary" @click="closeGroupModal">关闭</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import adminApi from '@/api/admin'

const plans = ref([])
const allGroups = ref([])
const planGroups = ref({}) // { planId: [groups] }

const showCreate = ref(false)
const showEdit = ref(false)
const showAssign = ref(false)
const showGroupModal = ref(false)

const currentPlan = ref(null)
const form = reactive({ id: null, name: '', transfer_enable: 0, month_price: null })
const assignForm = reactive({ user_id: null, expire_at: null, plan_id: null })

const load = async () => {
  try {
    const res = await adminApi.getPlans()
    plans.value = res.data || []
    // 加载每个套餐关联的分组
    for (const plan of plans.value) {
      await loadPlanGroups(plan.id)
    }
  } catch (e) {
    console.error('加载失败:', e)
  }
}

const loadAllGroups = async () => {
  try {
    const res = await adminApi.getSubscriptionGroups()
    allGroups.value = res.data || []
  } catch (e) {
    console.error('加载订阅分组失败:', e)
  }
}

const loadPlanGroups = async (planId) => {
  try {
    const res = await adminApi.getPlanGroups(planId)
    planGroups.value[planId] = res.data || []
  } catch (e) {
    planGroups.value[planId] = []
  }
}

onMounted(() => {
  load()
  loadAllGroups()
})

const edit = (p) => {
  form.id = p.id
  form.name = p.name
  form.transfer_enable = p.transfer_enable
  form.month_price = p.month_price
  showEdit.value = true
}

const remove = async (p) => {
  if (!confirm('确认删除该套餐？')) return
  try {
    await adminApi.deletePlan(p.id)
    await load()
  } catch (e) {
    alert(e.message || '删除失败')
  }
}

const closeModal = () => {
  showCreate.value = false
  showEdit.value = false
  form.id = null
  form.name = ''
  form.transfer_enable = 0
  form.month_price = null
}

const save = async () => {
  if (!form.name || form.name.trim() === '') {
    alert('请输入名称')
    return
  }
  const payload = {
    name: form.name.trim(),
    transfer_enable: form.transfer_enable,
    month_price: form.month_price
  }
  try {
    if (showEdit.value) {
      await adminApi.updatePlan(form.id, payload)
    } else {
      await adminApi.createPlan(payload)
    }
    closeModal()
    await load()
  } catch (e) {
    alert(e.message || '保存失败')
  }
}

const openAssign = (p) => {
  assignForm.plan_id = p.id
  assignForm.user_id = null
  assignForm.expire_at = null
  showAssign.value = true
}

const closeAssign = () => {
  showAssign.value = false
}

const assign = async () => {
  if (!assignForm.user_id) {
    alert('请输入用户ID')
    return
  }
  try {
    await adminApi.assignPlanToUser(assignForm.plan_id, {
      user_id: assignForm.user_id,
      expire_at: assignForm.expire_at
    })
    closeAssign()
    alert('分配成功')
  } catch (e) {
    alert(e.message || '分配失败')
  }
}

// 订阅分组管理
const openGroupModal = (p) => {
  currentPlan.value = p
  showGroupModal.value = true
}

const closeGroupModal = () => {
  showGroupModal.value = false
  currentPlan.value = null
}

const isGroupSelected = (groupId) => {
  if (!currentPlan.value) return false
  const groups = planGroups.value[currentPlan.value.id] || []
  return groups.some(g => g.id === groupId)
}

const toggleGroup = async (group) => {
  if (!currentPlan.value) return
  const planId = currentPlan.value.id
  
  try {
    if (isGroupSelected(group.id)) {
      await adminApi.removeGroupFromPlan(planId, group.id)
    } else {
      await adminApi.addGroupToPlan(planId, group.id)
    }
    await loadPlanGroups(planId)
  } catch (e) {
    alert(e.message || '操作失败')
  }
}

const removeGroup = async (planId, groupId) => {
  if (!confirm('确定移除此订阅分组关联？')) return
  try {
    await adminApi.removeGroupFromPlan(planId, groupId)
    await loadPlanGroups(planId)
  } catch (e) {
    alert(e.message || '移除失败')
  }
}
</script>

<style scoped>
.plans-page {
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
  background: var(--primary-color);
  color: white;
  padding: 3px 10px;
  border-radius: 12px;
  font-size: 12px;
  font-weight: 500;
}

.tag-remove {
  background: transparent;
  border: none;
  color: rgba(255,255,255,0.7);
  cursor: pointer;
  padding: 0;
  font-size: 14px;
  line-height: 1;
  margin-left: 2px;
}

.tag-remove:hover {
  color: white;
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

/* 弹窗样式 */
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
  max-width: 500px;
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

.close-btn:hover {
  color: var(--text-color);
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

/* 分组列表样式 */
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
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  background: var(--bg-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: var(--transition);
}

.group-item:hover {
  border-color: var(--primary-color);
}

.group-item.selected {
  background: rgba(59, 130, 246, 0.1);
  border-color: var(--primary-color);
}

.group-info {
  flex: 1;
}

.group-name {
  font-weight: 500;
  margin-bottom: 2px;
}

.group-desc {
  font-size: 12px;
}

.group-check {
  width: 24px;
  height: 24px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--primary-color);
  color: white;
  border-radius: 50%;
  font-size: 14px;
  font-weight: bold;
}

.group-item:not(.selected) .group-check {
  background: var(--border-color);
  color: transparent;
}
</style>
