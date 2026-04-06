<template>
  <div class="limit-page">
    <div class="page-header">
      <div>
        <h1>限速管理</h1>
        <p class="text-secondary">按隧道维护限速规则，保持 Flux 风格的独立规则页。</p>
      </div>
      <div class="header-actions">
        <button class="btn-secondary" :disabled="loading" @click="refreshAll">刷新</button>
        <button @click="openCreateModal">新增</button>
      </div>
    </div>
    <ForwardSuiteNav />

    <div v-if="loading" class="loading-state">
      <div class="spinner"></div>
      <span>正在加载...</span>
    </div>

    <div v-else-if="limits.length === 0" class="empty-state">
      <div class="empty-icon">⏱</div>
      <h3>暂无限速规则</h3>
      <p class="text-secondary">还没有创建任何限速规则，点击上方按钮开始创建。</p>
      <button @click="openCreateModal">立即创建</button>
    </div>

    <div v-else class="card-grid">
      <article v-for="item in limits" :key="item.id" class="limit-card">
        <div class="card-head">
          <div class="title-wrap">
            <h3>{{ item.name || `Rule-${item.id}` }}</h3>
            <span :class="['status-pill', item.status === 1 ? 'status-active' : 'status-disabled']">
              {{ item.status === 1 ? '运行' : '异常' }}
            </span>
          </div>
          <div class="card-actions">
            <button class="btn-sm btn-ghost" @click="openEditModal(item)">编辑</button>
            <button class="btn-sm btn-ghost btn-danger" @click="openDeleteModal(item)">删除</button>
          </div>
        </div>

        <div class="kv-list">
          <div class="kv-item">
            <span class="k">速度限制</span>
            <span class="v">{{ formatSpeed(item.speed) }}</span>
          </div>
          <div class="kv-item">
            <span class="k">绑定隧道</span>
            <span class="v">{{ formatTunnel(item) }}</span>
          </div>
          <div class="kv-item">
            <span class="k">更新时间</span>
            <span class="v">{{ formatTime(item.updatedTime || item.createdTime) }}</span>
          </div>
        </div>
      </article>
    </div>

    <div v-if="showFormModal" class="modal-overlay" @click.self="closeFormModal">
      <div class="modal">
        <div class="modal-header">
          <h3>{{ isEditMode ? '编辑限速规则' : '新增限速规则' }}</h3>
          <button class="close-btn" @click="closeFormModal">脳</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label>规则名称</label>
            <input v-model.trim="form.name" type="text" placeholder="请输入限速规则名称" />
          </div>
          <div class="form-group">
            <label>速度限制</label>
            <input v-model.number="form.speed" type="number" min="1" step="1" placeholder="请输入速度限制（Mbps）" />
          </div>
          <div class="form-group">
            <label>绑定隧道</label>
            <select v-model.number="form.tunnelId">
              <option :value="0">请选择要绑定的隧道</option>
              <option v-for="item in tunnels" :key="item.id" :value="item.id">
                {{ item.name || `Tunnel-${item.id}` }}
              </option>
            </select>
          </div>
          <p v-if="formError" class="error-msg">{{ formError }}</p>
        </div>
        <div class="modal-footer">
          <button class="btn-secondary" :disabled="formSubmitting" @click="closeFormModal">取消</button>
          <button :disabled="formSubmitting" @click="submitForm">
            {{ formSubmitting ? '提交中...' : (isEditMode ? '保存修改' : '创建规则') }}
          </button>
        </div>
      </div>
    </div>

    <div v-if="showDeleteModal" class="modal-overlay" @click.self="closeDeleteModal">
      <div class="modal modal-sm">
        <div class="modal-header">
          <h3>确认删除</h3>
          <button class="close-btn" @click="closeDeleteModal">脳</button>
        </div>
        <div class="modal-body">
          <p class="delete-copy">
            确定要删除限速规则 <strong>{{ deletingItem?.name || `#${deletingItem?.id || '-'}` }}</strong> 吗？
          </p>
          <p class="text-secondary">此操作无法撤销，删除后该规则将永久消失。</p>
        </div>
        <div class="modal-footer">
          <button class="btn-secondary" :disabled="deleteSubmitting" @click="closeDeleteModal">取消</button>
          <button class="btn-danger" :disabled="deleteSubmitting" @click="confirmDelete">
            {{ deleteSubmitting ? '删除中...' : '确认删除' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import {
  createSpeedLimit,
  deleteSpeedLimit,
  getSpeedLimitList,
  getSpeedLimitTunnels,
  updateSpeedLimit
} from '@/api/admin'
import ForwardSuiteNav from '@/components/admin/ForwardSuiteNav.vue'

const loading = ref(false)
const limits = ref([])
const tunnels = ref([])

const showFormModal = ref(false)
const formSubmitting = ref(false)
const formError = ref('')
const form = ref(newForm())

const showDeleteModal = ref(false)
const deleteSubmitting = ref(false)
const deletingItem = ref(null)

function newForm() {
  return {
    id: null,
    name: '',
    speed: 100,
    tunnelId: 0
  }
}

const isEditMode = computed(() => Number(form.value.id || 0) > 0)
const selectedTunnel = computed(() => {
  return tunnels.value.find(item => Number(item.id) === Number(form.value.tunnelId || 0)) || null
})

const assertCompat = (res, fallbackMsg) => {
  if (res && typeof res.code === 'number' && res.code !== 0) {
    throw new Error(res.msg || fallbackMsg)
  }
  return res
}

const unwrapData = (res, fallbackMsg) => {
  if (!res) return null
  if (typeof res.code === 'number') {
    if (res.code !== 0) {
      throw new Error(res.msg || fallbackMsg)
    }
    return res.data
  }
  return res.data
}

const normalizeTunnels = (input) => {
  if (!Array.isArray(input)) return []
  return input
    .map(item => {
      const id = Number(item?.id || 0)
      if (!id) return null
      return {
        id,
        name: item?.name || item?.tunnelName || ''
      }
    })
    .filter(Boolean)
}

const normalizeLimits = (input) => {
  if (!Array.isArray(input)) return []
  return input
    .map(item => {
      const id = Number(item?.id || 0)
      if (!id) return null
      return {
        id,
        name: item?.name || '',
        speed: Number(item?.speed || 0),
        tunnelId: Number(item?.tunnelId ?? item?.tunnel_id ?? 0),
        tunnelName: item?.tunnelName || item?.tunnel_name || '',
        status: Number(item?.status ?? 1),
        createdTime: item?.createdTime ?? item?.created_at ?? item?.createdAt ?? 0,
        updatedTime: item?.updatedTime ?? item?.updated_at ?? item?.updatedAt ?? 0
      }
    })
    .filter(Boolean)
}

const hydrateLimitTunnelName = (items) => {
  if (!Array.isArray(items) || items.length === 0) return []
  const tunnelMap = new Map(tunnels.value.map(item => [Number(item.id), item.name || `#${item.id}`]))
  return items.map(item => ({
    ...item,
    tunnelName: item.tunnelName || tunnelMap.get(Number(item.tunnelId)) || ''
  }))
}

const fetchTunnels = async () => {
  const res = await getSpeedLimitTunnels()
  const payload = unwrapData(res, '获取隧道列表失败')
  const list = Array.isArray(payload) ? payload : (payload?.list || [])
  tunnels.value = normalizeTunnels(list)
}

const fetchLimits = async () => {
  const res = await getSpeedLimitList()
  const payload = unwrapData(res, '获取限速规则失败')
  const list = Array.isArray(payload) ? payload : (payload?.list || [])
  limits.value = hydrateLimitTunnelName(normalizeLimits(list))
}

const refreshAll = async () => {
  loading.value = true
  try {
    await fetchTunnels()
    await fetchLimits()
  } catch (err) {
    alert(err?.message || '加载失败')
  } finally {
    loading.value = false
  }
}

const openCreateModal = () => {
  form.value = newForm()
  formError.value = ''
  showFormModal.value = true
}

const openEditModal = (item) => {
  form.value = {
    id: item.id,
    name: item.name || '',
    speed: Number(item.speed || 0),
    tunnelId: Number(item.tunnelId || 0)
  }
  formError.value = ''
  showFormModal.value = true
}

const closeFormModal = () => {
  if (formSubmitting.value) return
  showFormModal.value = false
  formError.value = ''
}

const validateForm = () => {
  if (!form.value.name) return '规则名称不能为空'
  if (form.value.name.length < 2 || form.value.name.length > 50) return '规则名称长度应在 2-50 个字符之间'
  if (!Number.isFinite(Number(form.value.speed)) || Number(form.value.speed) <= 0) return '请输入有效的速度限制（≥1 Mbps）'
  if (!Number.isFinite(Number(form.value.tunnelId)) || Number(form.value.tunnelId) <= 0) return '请选择要绑定的隧道'
  if (!selectedTunnel.value?.name) return '隧道名称不存在，请刷新后重试'
  return ''
}

const submitForm = async () => {
  const invalid = validateForm()
  if (invalid) {
    formError.value = invalid
    return
  }

  formSubmitting.value = true
  formError.value = ''
  try {
    const payload = {
      name: form.value.name,
      speed: Number(form.value.speed),
      tunnelId: Number(form.value.tunnelId),
      tunnelName: selectedTunnel.value.name
    }
    if (isEditMode.value) {
      assertCompat(await updateSpeedLimit({ id: Number(form.value.id), ...payload }), '更新限速规则失败')
    } else {
      assertCompat(await createSpeedLimit(payload), '创建限速规则失败')
    }
    await fetchLimits()
    showFormModal.value = false
  } catch (err) {
    formError.value = err?.message || '提交失败'
  } finally {
    formSubmitting.value = false
  }
}

const openDeleteModal = (item) => {
  deletingItem.value = item
  showDeleteModal.value = true
}

const closeDeleteModal = () => {
  if (deleteSubmitting.value) return
  showDeleteModal.value = false
  deletingItem.value = null
}

const confirmDelete = async () => {
  if (!deletingItem.value?.id) return
  deleteSubmitting.value = true
  try {
    assertCompat(await deleteSpeedLimit(Number(deletingItem.value.id)), '删除限速规则失败')
    await fetchLimits()
    closeDeleteModal()
  } catch (err) {
    alert(err?.message || '删除失败')
  } finally {
    deleteSubmitting.value = false
  }
}

const formatSpeed = (value) => {
  const speed = Number(value || 0)
  if (!Number.isFinite(speed) || speed <= 0) return '不限速'
  return `${speed} Mbps`
}

const formatTunnel = (item) => {
  if (item.tunnelName) {
    return item.tunnelName
  }
  if (item.tunnelId) {
    return `Tunnel #${item.tunnelId}`
  }
  return '-'
}

const formatTime = (value) => {
  const numeric = Number(value || 0)
  if (!Number.isFinite(numeric) || numeric <= 0) return '-'
  const parsed = new Date(numeric)
  if (Number.isNaN(parsed.getTime())) return '-'
  return parsed.toLocaleString('zh-CN')
}

onMounted(() => {
  refreshAll()
})
</script>

<style scoped>
.limit-page {
  max-width: 1400px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
  margin-bottom: 20px;
  flex-wrap: wrap;
}

.page-header h1 {
  margin: 0 0 6px;
  font-size: 24px;
}

.text-secondary {
  color: var(--text-secondary);
}

.header-actions {
  display: flex;
  gap: 10px;
}

.loading-state {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  min-height: 240px;
  color: var(--text-secondary);
}

.spinner {
  width: 18px;
  height: 18px;
  border: 2px solid rgba(148, 163, 184, 0.35);
  border-top-color: var(--primary-color);
  border-radius: 999px;
  animation: spin 0.8s linear infinite;
}

.card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 14px;
}

.limit-card {
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  padding: 16px;
  box-shadow: var(--shadow-sm);
}

.card-head {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}

.title-wrap {
  display: flex;
  align-items: center;
  gap: 8px;
}

.title-wrap h3 {
  margin: 0;
  font-size: 16px;
}

.status-pill {
  padding: 2px 10px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 600;
}

.status-active {
  background: rgba(34, 197, 94, 0.15);
  color: var(--success-color);
}

.status-disabled {
  background: rgba(239, 68, 68, 0.15);
  color: var(--error-color);
}

.card-actions {
  display: flex;
  gap: 6px;
}

.kv-list {
  display: grid;
  gap: 8px;
}

.kv-item {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  font-size: 13px;
}

.kv-item .k {
  color: var(--text-secondary);
}

.kv-item .v {
  color: var(--text-color);
  text-align: right;
}

.empty-state {
  border: 1px dashed var(--border-color);
  border-radius: var(--radius-lg);
  padding: 36px 16px;
  text-align: center;
  background: var(--surface-color);
}

.empty-icon {
  font-size: 36px;
  margin-bottom: 8px;
}

.modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.7);
  display: flex;
  justify-content: center;
  align-items: center;
  z-index: 1000;
  padding: 20px;
}

.modal {
  width: 100%;
  max-width: 560px;
  background: var(--surface-color);
  border-radius: var(--radius-lg);
  border: 1px solid var(--border-color);
}

.modal-sm {
  max-width: 460px;
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 20px;
  border-bottom: 1px solid var(--border-color);
}

.modal-body {
  padding: 18px 20px;
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  padding: 14px 20px 18px;
  border-top: 1px solid var(--border-color);
}

.form-group {
  margin-bottom: 14px;
}

.form-group label {
  display: block;
  margin-bottom: 6px;
  font-size: 14px;
  font-weight: 600;
}

.form-group input,
.form-group select {
  width: 100%;
}

.error-msg {
  margin: 0;
  color: var(--error-color);
  font-size: 13px;
}

.close-btn {
  border: none;
  background: transparent;
  color: var(--text-secondary);
  font-size: 20px;
  cursor: pointer;
}

.btn-danger {
  background: var(--error-color);
  color: #fff;
}

.delete-copy {
  margin: 0 0 10px;
  line-height: 1.7;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

@media (max-width: 768px) {
  .card-grid {
    grid-template-columns: 1fr;
  }
}
</style>
