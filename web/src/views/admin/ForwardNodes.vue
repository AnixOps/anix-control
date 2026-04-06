<template>
  <div class="forward-nodes-page">
    <div class="toolbar">
      <div class="toolbar-copy">
        <p class="eyebrow">Flux Compatible</p>
        <h2>中转节点管理</h2>
        <p class="toolbar-subtitle">统一管理 relay / exit 节点，支持 NodeX 与 Ansible 双运行时</p>
      </div>
      <div class="toolbar-actions">
        <button class="btn btn-secondary" :disabled="loading" @click="refreshNodes">刷新</button>
        <button class="btn btn-primary" @click="openEditor">新增节点</button>
      </div>
    </div>
    <ForwardSuiteNav />

    <div class="filters">
      <div class="filter-row">
        <label>节点类型</label>
        <select v-model="typeFilter">
          <option value="">全部</option>
          <option value="relay">Relay</option>
          <option value="exit">Exit</option>
        </select>
      </div>
      <div class="filter-row">
        <label>状态</label>
        <select v-model="statusFilter">
          <option value="all">全部</option>
          <option value="1">在线</option>
          <option value="0">离线</option>
        </select>
      </div>
      <div class="filter-row pagination">
        <button class="btn btn-ghost btn-sm" :disabled="page === 1" @click="changePage(page - 1)">上一页</button>
        <span>第 {{ page }} / {{ pageCount }} 页</span>
        <button class="btn btn-ghost btn-sm" :disabled="page >= pageCount" @click="changePage(page + 1)">下一页</button>
      </div>
    </div>

    <div v-if="feedback.message" :class="['feedback', `feedback-${feedback.type}`]">
      <span>{{ feedback.message }}</span>
      <button class="feedback-close" @click="clearFeedback">关闭</button>
    </div>

    <div v-if="loading" class="loading-state">
      <div class="spinner"></div>
      <span>正在加载中转节点...</span>
    </div>

    <section v-else-if="!nodes.length" class="empty-state">
      <h3>暂无中转节点</h3>
      <p>新增 relay / exit 节点后即可开始建模流量通道。</p>
    </section>

    <section v-else class="nodes-grid">
      <article v-for="node in nodes" :key="node.id" class="node-card">
        <div class="node-header">
          <div>
            <p class="eyebrow">节点 ID {{ node.id }}</p>
            <h3>{{ node.name }}</h3>
            <p class="meta">{{ node.typeLabel }} · {{ node.host }}:{{ node.port }}</p>
          </div>
          <div class="status-group">
            <span :class="['tag', `tag-${node.type}`]">{{ node.typeLabel }}</span>
            <span :class="['tag', node.enabled ? 'tag-success' : 'tag-muted']">
              {{ node.enabled ? '启用' : '禁用' }}
            </span>
          </div>
        </div>

        <div class="meta-grid">
          <div>
            <span class="meta-label">API 入口</span>
            <code>{{ node.host }}:{{ node.apiPort || '-' }}</code>
          </div>
          <div>
            <span class="meta-label">最后检测</span>
            <strong>{{ formatTime(node.lastCheck) }}</strong>
          </div>
          <div>
            <span class="meta-label">延迟</span>
            <strong>{{ node.latency ? `${node.latency} ms` : '-' }}</strong>
          </div>
          <div>
            <span class="meta-label">Region / ISP</span>
            <strong>{{ node.region || '-' }} · {{ node.isp || '-' }}</strong>
          </div>
          <div>
            <span class="meta-label">带宽 / 最大连接</span>
            <strong>{{ node.bandwidthDisplay }} / {{ node.maxConn || '-' }}</strong>
          </div>
          <div>
            <span class="meta-label">权重</span>
            <strong>{{ node.weight }}</strong>
          </div>
        </div>

        <div v-if="checkResults[node.id]" class="check-result">
          <span :class="['tag', checkResults[node.id].success ? 'tag-success' : 'tag-danger']">
            {{ checkResults[node.id].success ? '检测通过' : '检测失败' }}
          </span>
          <p class="check-message">{{ checkResults[node.id].message }}</p>
        </div>

        <div class="node-actions">
          <button class="btn btn-secondary btn-sm" @click="openEditor(node)">编辑</button>
          <button
            class="btn btn-secondary btn-sm"
            :disabled="nodeChecking === node.id"
            @click="runCheck(node)"
          >
            {{ nodeChecking === node.id ? '检测中...' : '检测' }}
          </button>
          <button class="btn btn-secondary btn-sm" @click="handleToggle(node)">
            {{ node.enabled ? '禁用' : '启用' }}
          </button>
          <button class="btn btn-danger btn-sm" @click="openDelete(node)">删除</button>
        </div>
      </article>
    </section>

    <div v-if="modalOpen" class="modal-overlay" @click.self="closeEditor">
      <div class="modal modal-lg">
        <div class="modal-header">
          <h3>{{ isEditMode ? '编辑中转节点' : '新增中转节点' }}</h3>
          <button class="modal-close" @click="closeEditor">×</button>
        </div>
        <div class="modal-body">
          <div class="form-grid">
            <label>
              <span>节点名称</span>
              <input v-model.trim="form.name" type="text" placeholder="例如 relay-hk-01" />
              <small v-if="formErrors.name">{{ formErrors.name }}</small>
            </label>
            <label>
              <span>类型</span>
              <select v-model="form.type">
                <option value="relay">Relay</option>
                <option value="exit">Exit</option>
              </select>
            </label>
          </div>
          <div class="form-grid">
            <label>
              <span>主机地址</span>
              <input v-model.trim="form.host" type="text" placeholder="1.2.3.4" />
              <small v-if="formErrors.host">{{ formErrors.host }}</small>
            </label>
            <label>
              <span>端口</span>
              <input v-model.number="form.port" type="number" min="1" max="65535" />
              <small v-if="formErrors.port">{{ formErrors.port }}</small>
            </label>
          </div>
          <div class="form-grid">
            <label>
              <span>API 端口</span>
              <input v-model.number="form.apiPort" type="number" min="1" max="65535" />
            </label>
            <label>
              <span>API Token</span>
              <input v-model.trim="form.apiToken" type="text" placeholder="不填则自动生成" />
            </label>
          </div>
          <div class="form-grid">
            <label>
              <span>带宽 (Mbps)</span>
              <input v-model.number="form.bandwidth" type="number" min="0" />
            </label>
            <label>
              <span>最大连接</span>
              <input v-model.number="form.maxConn" type="number" min="0" />
            </label>
          </div>
          <div class="form-grid">
            <label>
              <span>权重</span>
              <input v-model.number="form.weight" type="number" min="1" />
            </label>
            <label class="dual-input">
              <span>Region / ISP</span>
              <input v-model.trim="form.region" type="text" placeholder="HK" />
              <input v-model.trim="form.isp" type="text" placeholder="ISP" />
            </label>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary" :disabled="editorLoading" @click="closeEditor">取消</button>
          <button class="btn btn-primary" :disabled="editorLoading" @click="submitForm">
            {{ editorLoading ? '保存中...' : isEditMode ? '保存修改' : '创建节点' }}
          </button>
        </div>
      </div>
    </div>

    <div v-if="deleteModalOpen" class="modal-overlay" @click.self="closeDelete">
      <div class="modal modal-sm">
        <div class="modal-header">
          <h3>确认删除</h3>
          <button class="modal-close" @click="closeDelete">×</button>
        </div>
        <div class="modal-body">
          <p>确认删除节点 {{ deleteTarget?.name }}？此操作无法撤销。</p>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary btn-sm" :disabled="deleteLoading" @click="closeDelete">取消</button>
          <button class="btn btn-danger btn-sm" :disabled="deleteLoading" @click="confirmDelete">
            {{ deleteLoading ? '删除中...' : '确认删除' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import ForwardSuiteNav from '@/components/admin/ForwardSuiteNav.vue'
import {
  checkForwardNode,
  createForwardNode,
  deleteForwardNode,
  getForwardNodes,
  toggleForwardNode,
  updateForwardNode
} from '@/api/admin'

const loading = ref(false)
const nodes = ref([])
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const typeFilter = ref('')
const statusFilter = ref('all')
const feedback = reactive({ type: 'info', message: '' })
const modalOpen = ref(false)
const isEditMode = ref(false)
const editorLoading = ref(false)
const deleteModalOpen = ref(false)
const deleteLoading = ref(false)
const deleteTarget = ref(null)
const nodeChecking = ref(null)
const checkResults = reactive({})
const form = reactive({
  id: null,
  name: '',
  type: 'relay',
  host: '',
  port: '',
  apiPort: '',
  apiToken: '',
  region: '',
  isp: '',
  bandwidth: '',
  weight: 1,
  maxConn: ''
})
const formErrors = reactive({
  name: '',
  host: '',
  port: ''
})

const typeLabels = {
  relay: 'Relay',
  exit: 'Exit'
}

const pageCount = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)))

const clearFeedback = () => {
  feedback.message = ''
}

const setFeedback = (type, message) => {
  feedback.type = type
  feedback.message = message
}

const changePage = newPage => {
  if (newPage < 1 || newPage > pageCount.value) return
  page.value = newPage
  loadNodes()
}

const refreshNodes = () => {
  loadNodes()
}

const resetForm = () => {
  form.id = null
  form.name = ''
  form.type = 'relay'
  form.host = ''
  form.port = ''
  form.apiPort = ''
  form.apiToken = ''
  form.region = ''
  form.isp = ''
  form.bandwidth = ''
  form.weight = 1
  form.maxConn = ''
  formErrors.name = ''
  formErrors.host = ''
  formErrors.port = ''
}

const openEditor = node => {
  if (node) {
    form.id = node.id
    form.name = node.name
    form.type = node.type
    form.host = node.host
    form.port = node.port
    form.apiPort = node.apiPort
    form.apiToken = node.apiToken || ''
    form.region = node.region || ''
    form.isp = node.isp || ''
    form.bandwidth = node.bandwidth || ''
    form.weight = node.weight || 1
    form.maxConn = node.maxConn || ''
    isEditMode.value = true
  } else {
    resetForm()
    isEditMode.value = false
  }
  modalOpen.value = true
}

const closeEditor = () => {
  if (editorLoading.value) return
  modalOpen.value = false
  resetForm()
}

const validateForm = () => {
  formErrors.name = ''
  formErrors.host = ''
  formErrors.port = ''
  if (!form.name.trim()) {
    formErrors.name = '节点名称不能为空'
  }
  if (!form.host.trim()) {
    formErrors.host = '主机地址不能为空'
  }
  if (!form.port || form.port <= 0 || form.port > 65535) {
    formErrors.port = '端口必须在 1～65535 范围'
  }
  return !formErrors.name && !formErrors.host && !formErrors.port
}

const submitForm = async () => {
  if (!validateForm()) {
    return
  }
  editorLoading.value = true
  try {
    const payload = {
      name: form.name.trim(),
      type: form.type,
      host: form.host.trim(),
      port: Number(form.port),
      weight: Number(form.weight) || 1,
      api_port: form.apiPort ? Number(form.apiPort) : undefined,
      api_token: form.apiToken?.trim(),
      region: form.region?.trim(),
      isp: form.isp?.trim(),
      bandwidth: form.bandwidth ? Number(form.bandwidth) : undefined,
      max_conn: form.maxConn ? Number(form.maxConn) : undefined
    }

    if (isEditMode.value && form.id) {
      await updateForwardNode(form.id, payload)
      setFeedback('success', '节点信息已更新')
    } else {
      await createForwardNode(payload)
      setFeedback('success', '节点创建成功')
    }
    closeEditor()
    loadNodes()
  } catch (error) {
    setFeedback('error', extractErrorMessage(error, '保存失败'))
  } finally {
    editorLoading.value = false
  }
}

const openDelete = node => {
  deleteTarget.value = node
  deleteModalOpen.value = true
}

const closeDelete = () => {
  if (deleteLoading.value) {
    return
  }
  deleteModalOpen.value = false
  deleteTarget.value = null
}

const confirmDelete = async () => {
  if (!deleteTarget.value) {
    return
  }
  deleteLoading.value = true
  try {
    await deleteForwardNode(deleteTarget.value.id)
    setFeedback('success', '节点已删除')
    loadNodes()
    closeDelete()
  } catch (error) {
    setFeedback('error', extractErrorMessage(error, '删除失败'))
  } finally {
    deleteLoading.value = false
  }
}

const handleToggle = async node => {
  try {
    await toggleForwardNode(node.id, { enabled: !node.enabled })
    setFeedback('success', `${node.name} 已${node.enabled ? '禁用' : '启用'}`)
    loadNodes()
  } catch (error) {
    setFeedback('error', extractErrorMessage(error, '操作失败'))
  }
}

const runCheck = async node => {
  nodeChecking.value = node.id
  try {
    const response = await checkForwardNode(node.id)
    const payload = unwrapResponse(response)
    const message = payload?.message || (payload?.data?.message || '检测完成')
    const success = payload?.success ?? payload?.data?.success ?? true
    checkResults[node.id] = {
      success,
      message
    }
    setFeedback('success', `节点 ${node.name} 检测结果: ${message}`)
  } catch (error) {
    checkResults[node.id] = {
      success: false,
      message: extractErrorMessage(error, '检测失败')
    }
    setFeedback('error', `节点 ${node.name} 检测失败`)
  } finally {
    nodeChecking.value = null
  }
}

const unwrapResponse = (response = {}) => {
  if (!response) {
    return {}
  }
  if (typeof response.code === 'number') {
    if (response.code !== 0) {
      throw new Error(response.msg || '请求失败')
    }
    return response.data ?? response
  }
  return response.data ? response.data : response
}

const extractErrorMessage = (error, fallback = '请求失败') => {
  return (
    error?.response?.data?.error ||
    error?.response?.data?.message ||
    error?.message ||
    fallback
  )
}

const normalizeNodesPayload = payload => {
  const list = Array.isArray(payload?.list) ? payload.list : []
  return list.map(item => {
    const node = {
      ...item,
      apiPort: Number(item.api_port ?? item.apiPort ?? 0) || undefined,
      port: Number(item.port ?? 0),
      type: item.type || 'relay',
      typeLabel: typeLabels[item.type] || 'Relay',
      region: item.region || '',
      isp: item.isp || '',
      latency: Number(item.latency ?? 0) || undefined,
      lastCheck: item.last_check ?? item.lastCheck ?? null,
      bandwidth: Number(item.bandwidth ?? 0) || undefined,
      bandwidthDisplay: item.bandwidth ? `${item.bandwidth} Mbps` : '-',
      maxConn: Number(item.max_conn ?? item.maxConn ?? 0) || undefined,
      weight: Number(item.weight ?? 1) || 1,
      enabled: typeof item.enabled === 'boolean' ? item.enabled : item.enabled === 1,
      id: Number(item.id ?? 0) || 0
    }
    node.name = item.name || `node-${node.id}`
    node.host = item.host || '-'
    node.apiToken = item.api_token ?? item.apiToken ?? ''
    return node
  })
}

const loadNodes = async () => {
  loading.value = true
  try {
    const params = {
      page: page.value,
      page_size: pageSize.value
    }
    if (typeFilter.value) {
      params.type = typeFilter.value
    }
    if (statusFilter.value !== 'all') {
      params.status = Number(statusFilter.value)
    }
    const response = await getForwardNodes(params)
    const payload = unwrapResponse(response)
    const list = normalizeNodesPayload(payload)
    nodes.value = list
    total.value = Number(payload?.total ?? payload?.list?.length ?? list.length ?? 0)
  } catch (error) {
    setFeedback('error', extractErrorMessage(error, '加载失败'))
  } finally {
    loading.value = false
  }
}

watch([typeFilter, statusFilter], () => {
  page.value = 1
  loadNodes()
})

onMounted(() => {
  loadNodes()
})

const formatTime = value => {
  if (!value) {
    return '-'
  }
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return '-'
  }
  return date.toLocaleString('zh-CN')
}
</script>

<style scoped>
.forward-nodes-page {
  display: flex;
  flex-direction: column;
  gap: 18px;
  min-height: calc(100vh - 200px);
}

.toolbar {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  padding: 20px 24px;
  border-radius: 18px;
  border: 1px solid var(--border-color);
  background:
    radial-gradient(circle at top right, rgba(37, 99, 235, 0.15), transparent 35%),
    var(--surface-color);
  box-shadow: var(--shadow-sm);
  gap: 16px;
}

.toolbar-copy h2 {
  margin: 6px 0 4px;
  font-size: 26px;
}

.toolbar-subtitle {
  color: var(--text-secondary);
  margin: 0;
}

.toolbar-actions {
  display: flex;
  gap: 10px;
}

.filters {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  padding: 14px 20px;
  border-radius: 16px;
  border: 1px solid var(--border-color);
  background: var(--surface-color);
}

.filter-row {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 160px;
}

.filter-row label {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-secondary);
}

.filter-row select {
  padding: 8px 10px;
  border-radius: 10px;
  border: 1px solid var(--border-color);
  background: var(--bg-color);
}

.pagination {
  flex: 1;
  flex-direction: row;
  align-items: center;
  justify-content: flex-end;
}

.feedback {
  padding: 12px 16px;
  border-radius: 12px;
  border: 1px solid transparent;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.nodes-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
  gap: 16px;
}

.node-card {
  border: 1px solid var(--border-color);
  border-radius: 20px;
  padding: 18px;
  background: var(--surface-color);
  display: flex;
  flex-direction: column;
  gap: 16px;
  box-shadow: 0 18px 40px rgba(15, 23, 42, 0.08);
}

.node-header {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  align-items: flex-start;
}

.status-group {
  display: flex;
  gap: 6px;
  align-items: center;
}

.meta-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: 12px;
}

.meta-label {
  font-size: 12px;
  color: var(--text-secondary);
  display: block;
}

.node-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

.check-result {
  border-radius: 12px;
  padding: 12px 14px;
  background: rgba(37, 99, 235, 0.05);
  border: 1px solid rgba(37, 99, 235, 0.25);
}

.check-message {
  margin: 4px 0 0;
  font-size: 13px;
}

.modal-body .form-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 12px;
}

.modal-body input,
.modal-body select {
  width: 100%;
  padding: 10px 12px;
  border-radius: 10px;
  border: 1px solid var(--border-color);
  background: var(--bg-color);
}

.modal-body small {
  color: var(--error-color);
  display: block;
  margin-top: 4px;
}

.dual-input input {
  margin-top: 6px;
}

.modal-overlay {
  position: fixed;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
  background: rgba(15, 23, 42, 0.65);
  z-index: 1000;
}

.tag {
  padding: 4px 10px;
  border-radius: 999px;
  font-size: 12px;
}

.tag-relay {
  background: rgba(37, 99, 235, 0.12);
  color: #2563eb;
}

.tag-exit {
  background: rgba(220, 38, 38, 0.12);
  color: #dc2626;
}

.tag-success {
  background: rgba(16, 185, 129, 0.12);
  color: #047857;
}

.tag-muted {
  background: rgba(148, 163, 184, 0.12);
  color: var(--text-secondary);
}

.node-actions button {
  flex: 1 1 auto;
}

.modal-sm .modal-body {
  padding: 18px 24px;
}

.eyebrow {
  margin: 0;
  text-transform: uppercase;
  font-size: 11px;
  letter-spacing: 0.2em;
  color: var(--text-secondary);
}

.loading-state,
.empty-state {
  border: 1px solid var(--border-color);
  border-radius: 18px;
  padding: 36px;
  display: flex;
  align-items: center;
  justify-content: center;
  text-align: center;
  gap: 12px;
  min-height: 200px;
}

.spinner {
  width: 28px;
  height: 28px;
  border: 3px solid rgba(37, 99, 235, 0.3);
  border-top-color: #2563eb;
  border-radius: 50%;
  animation: spin 0.9s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
