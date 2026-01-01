<template>
  <div class="coupons-page">
    <div class="page-header">
      <h1>优惠券管理</h1>
      <p class="text-secondary">创建和管理优惠券</p>
    </div>

    <!-- 筛选栏 -->
    <div class="filter-bar">
      <button @click="showCreate = true">➕ 新建优惠券</button>
    </div>

    <!-- 优惠券列表 -->
    <div class="table-container">
      <table class="data-table">
        <thead>
          <tr>
            <th>ID</th>
            <th>优惠码</th>
            <th>名称</th>
            <th>类型</th>
            <th>优惠值</th>
            <th>使用次数</th>
            <th>有效期</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="c in coupons" :key="c.id">
            <td>{{ c.id }}</td>
            <td><code class="coupon-code">{{ c.code }}</code></td>
            <td>{{ c.name }}</td>
            <td>
              <span :class="['status-badge', c.type === 1 ? 'type-percent' : 'type-fixed']">
                {{ c.type === 1 ? '折扣' : '固定金额' }}
              </span>
            </td>
            <td>{{ c.type === 1 ? c.value + '%' : '¥' + (c.value / 100).toFixed(2) }}</td>
            <td>{{ c.use_count }} / {{ c.limit_use === -1 ? '∞' : c.limit_use }}</td>
            <td class="date-range">{{ formatDate(c.started_at) }} ~ {{ formatDate(c.ended_at) }}</td>
            <td>
              <div class="action-buttons">
                <button class="btn-sm btn-ghost" @click="remove(c)" title="删除">🗑️</button>
              </div>
            </td>
          </tr>
          <tr v-if="coupons.length === 0">
            <td colspan="8" class="empty-row">暂无优惠券</td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- 创建弹窗 -->
    <div v-if="showCreate" class="modal-overlay" @click.self="closeModal">
      <div class="modal">
        <div class="modal-header">
          <h3>新建优惠券</h3>
          <button class="close-btn" @click="closeModal">✕</button>
        </div>
        <div class="modal-body">
          <div class="form-row">
            <div class="form-group">
              <label>优惠码 <span class="required">*</span></label>
              <input v-model="form.code" type="text" placeholder="如: NEWYEAR2026">
            </div>
            <div class="form-group">
              <label>名称 <span class="required">*</span></label>
              <input v-model="form.name" type="text" placeholder="如: 新年特惠">
            </div>
          </div>
          <div class="form-row">
            <div class="form-group">
              <label>类型</label>
              <select v-model="form.type">
                <option :value="1">折扣（百分比）</option>
                <option :value="2">固定金额（分）</option>
              </select>
            </div>
            <div class="form-group">
              <label>{{ form.type === 1 ? '折扣百分比 (0-100)' : '优惠金额 (分)' }}</label>
              <input v-model.number="form.value" type="number" min="0">
            </div>
          </div>
          <div class="form-row">
            <div class="form-group">
              <label>开始时间</label>
              <input v-model="form.started_at" type="datetime-local">
            </div>
            <div class="form-group">
              <label>结束时间</label>
              <input v-model="form.ended_at" type="datetime-local">
            </div>
          </div>
          <div class="form-group">
            <label>使用次数限制 (-1 表示无限)</label>
            <input v-model.number="form.limit_use" type="number">
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn-secondary" @click="closeModal">取消</button>
          <button @click="create">创建</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import adminApi from '@/api/admin'

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

const load = async () => {
  try {
    const res = await adminApi.getCoupons()
    coupons.value = res.data || []
  } catch (e) {
    console.error('加载失败:', e)
  }
}

onMounted(() => { load() })

const formatDate = (ts) => {
  if (!ts) return '-'
  return new Date(ts * 1000).toLocaleDateString('zh-CN')
}

const closeModal = () => {
  showCreate.value = false
  form.code = ''
  form.name = ''
  form.type = 1
  form.value = 10
  form.limit_use = 100
  form.started_at = ''
  form.ended_at = ''
}

const create = async () => {
  if (!form.code || !form.name) {
    alert('请填写优惠码和名称')
    return
  }
  try {
    const payload = {
      code: form.code.toUpperCase(),
      name: form.name,
      type: form.type,
      value: form.value,
      limit_use: form.limit_use,
      started_at: form.started_at ? Math.floor(new Date(form.started_at).getTime() / 1000) : Math.floor(Date.now() / 1000),
      ended_at: form.ended_at ? Math.floor(new Date(form.ended_at).getTime() / 1000) : Math.floor(Date.now() / 1000) + 30 * 24 * 3600
    }
    await adminApi.createCoupon(payload)
    alert('创建成功')
    closeModal()
    await load()
  } catch (e) {
    alert(e.message || '创建失败')
  }
}

const remove = async (c) => {
  if (!confirm(`确定删除优惠券 "${c.code}"？`)) return
  try {
    await adminApi.deleteCoupon(c.id)
    await load()
  } catch (e) {
    alert(e.message || '删除失败')
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
