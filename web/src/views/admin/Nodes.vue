<template>
  <div class="page nodes-page">
    <div class="page-header">
      <h1>{{ $t('admin.nodes.title', '节点管理') }}</h1>
      <div class="header-actions">
        <button class="btn btn-secondary" @click="showAuthKeys = true">
          🔑 {{ $t('admin.nodes.authKeys', '授权密钥') }}
        </button>
        <button class="btn btn-primary" @click="openCreateModal">
          + {{ $t('admin.nodes.addNode', '添加节点') }}
        </button>
      </div>
    </div>

    <!-- 节点统计 -->
    <div class="stats-grid">
      <div class="stat-card">
        <div class="stat-value">{{ stats.total }}</div>
        <div class="stat-label">{{ $t('admin.nodes.total', '总节点') }}</div>
      </div>
      <div class="stat-card online">
        <div class="stat-value">{{ stats.online }}</div>
        <div class="stat-label">{{ $t('admin.nodes.online', '在线') }}</div>
      </div>
      <div class="stat-card warning">
        <div class="stat-value">{{ stats.offline }}</div>
        <div class="stat-label">{{ $t('admin.nodes.offline', '离线') }}</div>
      </div>
      <div class="stat-card pending">
        <div class="stat-value">{{ stats.pending }}</div>
        <div class="stat-label">{{ $t('admin.nodes.pending', '待激活') }}</div>
      </div>
    </div>

    <!-- 节点列表 -->
    <div class="table-container">
      <table class="table">
        <thead>
          <tr>
            <th>ID</th>
            <th>{{ $t('admin.nodes.name', '名称') }}</th>
            <th>{{ $t('admin.nodes.address', '地址') }}</th>
            <th>{{ $t('admin.nodes.status', '状态') }}</th>
            <th>{{ $t('admin.nodes.protocols', '协议数') }}</th>
            <th>{{ $t('admin.nodes.traffic', '今日流量') }}</th>
            <th>{{ $t('admin.nodes.lastHeartbeat', '最后心跳') }}</th>
            <th>{{ $t('admin.actions', '操作') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="loading">
            <td colspan="8" class="text-center">加载中...</td>
          </tr>
          <tr v-else-if="nodes.length === 0">
            <td colspan="8" class="text-center">暂无节点</td>
          </tr>
          <tr v-for="node in nodes" :key="node.id">
            <td>{{ node.id }}</td>
            <td>
              <strong>{{ node.name }}</strong>
              <span v-if="node.tags" class="node-tags">
                <span v-for="tag in node.tags.split(',')" :key="tag" class="tag">{{ tag }}</span>
              </span>
            </td>
            <td>
              <code>{{ node.address }}:{{ node.api_port }}</code>
            </td>
            <td>
              <span :class="['status-badge', getStatusClass(node.status)]">
                {{ getStatusText(node.status) }}
              </span>
            </td>
            <td>{{ node.protocol_count || 0 }}</td>
            <td>{{ formatBytes(node.traffic_today || 0) }}</td>
            <td>{{ formatTime(node.last_heartbeat) }}</td>
            <td class="actions">
              <button class="btn btn-sm btn-info" @click="openProtocols(node)">
                📡 协议
              </button>
              <button class="btn btn-sm btn-warning" @click="openEditModal(node)">
                ✏️
              </button>
              <button class="btn btn-sm btn-danger" @click="confirmDelete(node)">
                🗑️
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- 分页 -->
    <div class="pagination" v-if="pagination.total > pagination.size">
      <button :disabled="pagination.page === 1" @click="changePage(pagination.page - 1)">上一页</button>
      <span>{{ pagination.page }} / {{ Math.ceil(pagination.total / pagination.size) }}</span>
      <button :disabled="pagination.page >= Math.ceil(pagination.total / pagination.size)" @click="changePage(pagination.page + 1)">下一页</button>
    </div>

    <!-- 创建/编辑节点模态框 -->
    <div class="modal-overlay" v-if="showNodeModal" @click.self="closeNodeModal">
      <div class="modal">
        <div class="modal-header">
          <h3>{{ editingNode ? '编辑节点' : '添加节点' }}</h3>
          <button class="close-btn" @click="closeNodeModal">×</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label>节点名称 *</label>
            <input v-model="nodeForm.name" type="text" placeholder="输入节点名称" />
          </div>
          <div class="form-row">
            <div class="form-group">
              <label>节点地址 *</label>
              <input v-model="nodeForm.address" type="text" placeholder="IP 或域名" />
            </div>
            <div class="form-group">
              <label>API 端口 *</label>
              <input v-model.number="nodeForm.api_port" type="number" placeholder="8080" />
            </div>
          </div>
          <div class="form-group">
            <label>标签 (逗号分隔)</label>
            <input v-model="nodeForm.tags" type="text" placeholder="香港,IEPL,高速" />
          </div>
          <div class="form-row">
            <div class="form-group">
              <label>节点倍率</label>
              <input v-model.number="nodeForm.rate" type="number" step="0.1" placeholder="1.0" />
            </div>
            <div class="form-group">
              <label>排序</label>
              <input v-model.number="nodeForm.sort" type="number" placeholder="0" />
            </div>
          </div>
          <div class="form-group" v-if="editingNode">
            <label>状态</label>
            <select v-model.number="nodeForm.status">
              <option :value="0">待激活</option>
              <option :value="1">在线</option>
              <option :value="2">离线</option>
              <option :value="3">禁用</option>
            </select>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary" @click="closeNodeModal">取消</button>
          <button class="btn btn-primary" @click="saveNode" :disabled="saving">
            {{ saving ? '保存中...' : '保存' }}
          </button>
        </div>
      </div>
    </div>

    <!-- 协议管理模态框 -->
    <div class="modal-overlay" v-if="showProtocolModal" @click.self="closeProtocolModal">
      <div class="modal modal-lg">
        <div class="modal-header">
          <h3>协议管理 - {{ selectedNode?.name }}</h3>
          <button class="close-btn" @click="closeProtocolModal">×</button>
        </div>
        <div class="modal-body">
          <div class="protocol-header">
            <button class="btn btn-primary btn-sm" @click="openAddProtocol">+ 添加协议</button>
          </div>
          
          <table class="table" v-if="protocols.length > 0">
            <thead>
              <tr>
                <th>协议类型</th>
                <th>端口</th>
                <th>状态</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="protocol in protocols" :key="protocol.id">
                <td>
                  <span class="protocol-type">{{ protocol.protocol_type }}</span>
                </td>
                <td>{{ protocol.port }}</td>
                <td>
                  <span :class="['status-badge', protocol.enabled ? 'status-online' : 'status-disabled']">
                    {{ protocol.enabled ? '启用' : '禁用' }}
                  </span>
                </td>
                <td>
                  <button class="btn btn-sm btn-warning" @click="editProtocol(protocol)">编辑</button>
                  <button class="btn btn-sm btn-danger" @click="deleteProtocol(protocol)">删除</button>
                </td>
              </tr>
            </tbody>
          </table>
          <div v-else class="empty-message">暂无协议配置</div>
        </div>
      </div>
    </div>

    <!-- 添加/编辑协议模态框 -->
    <div class="modal-overlay" v-if="showProtocolFormModal" @click.self="closeProtocolFormModal">
      <div class="modal">
        <div class="modal-header">
          <h3>{{ editingProtocol ? '编辑协议' : '添加协议' }}</h3>
          <button class="close-btn" @click="closeProtocolFormModal">×</button>
        </div>
        <div class="modal-body">
          <div class="form-group" v-if="!editingProtocol">
            <label>协议模板</label>
            <select v-model="selectedTemplate" @change="applyTemplate">
              <option value="">选择模板...</option>
              <option v-for="tpl in protocolTemplates" :key="tpl.type" :value="tpl.type">
                {{ tpl.type }} - {{ tpl.description }}
              </option>
            </select>
          </div>
          <div class="form-group">
            <label>协议类型 *</label>
            <input v-model="protocolForm.protocol_type" type="text" :disabled="editingProtocol" />
          </div>
          <div class="form-group">
            <label>端口 *</label>
            <input v-model.number="protocolForm.port" type="number" placeholder="443" />
          </div>
          <div class="form-group">
            <label>配置 (JSON)</label>
            <textarea v-model="protocolForm.config" rows="10" placeholder="{}"></textarea>
          </div>
          <div class="form-group">
            <label class="checkbox-label">
              <input type="checkbox" v-model="protocolForm.enabled" />
              启用协议
            </label>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary" @click="closeProtocolFormModal">取消</button>
          <button class="btn btn-primary" @click="saveProtocol" :disabled="savingProtocol">
            {{ savingProtocol ? '保存中...' : '保存' }}
          </button>
        </div>
      </div>
    </div>

    <!-- 授权密钥模态框 -->
    <div class="modal-overlay" v-if="showAuthKeys" @click.self="showAuthKeys = false">
      <div class="modal modal-lg">
        <div class="modal-header">
          <h3>🔑 授权密钥管理</h3>
          <button class="close-btn" @click="showAuthKeys = false">×</button>
        </div>
        <div class="modal-body">
          <div class="info-box">
            <p>授权密钥用于节点自动注册。将密钥配置到节点后，节点启动时会自动向面板注册。</p>
          </div>
          
          <div class="protocol-header">
            <div class="form-inline">
              <input v-model="newKeyRemark" type="text" placeholder="备注（可选）" style="width: 200px;" />
              <button class="btn btn-primary btn-sm" @click="generateKey" :disabled="generatingKey">
                {{ generatingKey ? '生成中...' : '+ 生成新密钥' }}
              </button>
            </div>
          </div>

          <table class="table" v-if="authKeys.length > 0">
            <thead>
              <tr>
                <th>密钥</th>
                <th>备注</th>
                <th>使用次数</th>
                <th>创建时间</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="key in authKeys" :key="key.id">
                <td>
                  <code class="key-text">{{ key.key }}</code>
                  <button class="btn btn-xs" @click="copyKey(key.key)">复制</button>
                </td>
                <td>{{ key.remark || '-' }}</td>
                <td>{{ key.used_count }} / {{ key.max_uses || '∞' }}</td>
                <td>{{ formatDate(key.created_at) }}</td>
                <td>
                  <button class="btn btn-sm btn-danger" @click="removeAuthKey(key)">删除</button>
                </td>
              </tr>
            </tbody>
          </table>
          <div v-else class="empty-message">暂无授权密钥</div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { 
  getNodes, getNodeStats, createNode, updateNode, deleteNode,
  getNodeProtocols, createNodeProtocol, updateNodeProtocol, deleteNodeProtocol,
  getProtocolTemplates, getAuthKeys, generateAuthKey, deleteAuthKey
} from '@/api/admin'

// 状态
const loading = ref(false)
const saving = ref(false)
const savingProtocol = ref(false)
const generatingKey = ref(false)

const nodes = ref([])
const stats = reactive({ total: 0, online: 0, offline: 0, pending: 0 })
const pagination = reactive({ page: 1, size: 20, total: 0 })

const showNodeModal = ref(false)
const editingNode = ref(null)
const nodeForm = reactive({
  name: '',
  address: '',
  api_port: 8080,
  tags: '',
  rate: 1.0,
  sort: 0,
  status: 0
})

const showProtocolModal = ref(false)
const selectedNode = ref(null)
const protocols = ref([])

const showProtocolFormModal = ref(false)
const editingProtocol = ref(null)
const protocolForm = reactive({
  protocol_type: '',
  port: 443,
  config: '{}',
  enabled: true
})
const protocolTemplates = ref([])
const selectedTemplate = ref('')

const showAuthKeys = ref(false)
const authKeys = ref([])
const newKeyRemark = ref('')

// 加载数据
const loadNodes = async () => {
  loading.value = true
  try {
    const res = await getNodes({ page: pagination.page, size: pagination.size })
    nodes.value = res.data.list || []
    pagination.total = res.data.total || 0
  } catch (e) {
    console.error('Failed to load nodes:', e)
  } finally {
    loading.value = false
  }
}

const loadStats = async () => {
  try {
    const res = await getNodeStats()
    Object.assign(stats, res.data)
  } catch (e) {
    console.error('Failed to load stats:', e)
  }
}

const loadProtocolTemplates = async () => {
  try {
    const res = await getProtocolTemplates()
    protocolTemplates.value = res.data || []
  } catch (e) {
    console.error('Failed to load templates:', e)
  }
}

const loadAuthKeys = async () => {
  try {
    const res = await getAuthKeys()
    authKeys.value = res.data || []
  } catch (e) {
    console.error('Failed to load auth keys:', e)
  }
}

// 节点操作
const openCreateModal = () => {
  editingNode.value = null
  Object.assign(nodeForm, { name: '', address: '', api_port: 8080, tags: '', rate: 1.0, sort: 0, status: 0 })
  showNodeModal.value = true
}

const openEditModal = (node) => {
  editingNode.value = node
  Object.assign(nodeForm, {
    name: node.name,
    address: node.address,
    api_port: node.api_port,
    tags: node.tags || '',
    rate: node.rate || 1.0,
    sort: node.sort || 0,
    status: node.status
  })
  showNodeModal.value = true
}

const closeNodeModal = () => {
  showNodeModal.value = false
  editingNode.value = null
}

const saveNode = async () => {
  if (!nodeForm.name || !nodeForm.address || !nodeForm.api_port) {
    alert('请填写必填字段')
    return
  }
  saving.value = true
  try {
    if (editingNode.value) {
      await updateNode(editingNode.value.id, nodeForm)
    } else {
      await createNode(nodeForm)
    }
    closeNodeModal()
    loadNodes()
    loadStats()
  } catch (e) {
    alert('保存失败: ' + (e.message || e))
  } finally {
    saving.value = false
  }
}

const confirmDelete = async (node) => {
  if (!confirm(`确定要删除节点 "${node.name}" 吗？`)) return
  try {
    await deleteNode(node.id)
    loadNodes()
    loadStats()
  } catch (e) {
    alert('删除失败: ' + (e.message || e))
  }
}

// 协议操作
const openProtocols = async (node) => {
  selectedNode.value = node
  showProtocolModal.value = true
  try {
    const res = await getNodeProtocols(node.id)
    protocols.value = res.data || []
  } catch (e) {
    console.error('Failed to load protocols:', e)
    protocols.value = []
  }
}

const closeProtocolModal = () => {
  showProtocolModal.value = false
  selectedNode.value = null
  protocols.value = []
}

const openAddProtocol = () => {
  editingProtocol.value = null
  Object.assign(protocolForm, { protocol_type: '', port: 443, config: '{}', enabled: true })
  selectedTemplate.value = ''
  showProtocolFormModal.value = true
}

const editProtocol = (protocol) => {
  editingProtocol.value = protocol
  Object.assign(protocolForm, {
    protocol_type: protocol.protocol_type,
    port: protocol.port,
    config: JSON.stringify(protocol.config || {}, null, 2),
    enabled: protocol.enabled
  })
  showProtocolFormModal.value = true
}

const closeProtocolFormModal = () => {
  showProtocolFormModal.value = false
  editingProtocol.value = null
}

const applyTemplate = () => {
  const tpl = protocolTemplates.value.find(t => t.type === selectedTemplate.value)
  if (tpl) {
    protocolForm.protocol_type = tpl.type
    protocolForm.port = tpl.default_port
    protocolForm.config = JSON.stringify(tpl.default_config || {}, null, 2)
  }
}

const saveProtocol = async () => {
  if (!protocolForm.protocol_type || !protocolForm.port) {
    alert('请填写必填字段')
    return
  }
  
  let config = {}
  try {
    config = JSON.parse(protocolForm.config || '{}')
  } catch (e) {
    alert('配置 JSON 格式错误')
    return
  }
  
  savingProtocol.value = true
  try {
    const data = {
      protocol_type: protocolForm.protocol_type,
      port: protocolForm.port,
      config: config,
      enabled: protocolForm.enabled
    }
    
    if (editingProtocol.value) {
      await updateNodeProtocol(selectedNode.value.id, editingProtocol.value.id, data)
    } else {
      await createNodeProtocol(selectedNode.value.id, data)
    }
    
    closeProtocolFormModal()
    // 刷新协议列表
    const res = await getNodeProtocols(selectedNode.value.id)
    protocols.value = res.data || []
  } catch (e) {
    alert('保存失败: ' + (e.message || e))
  } finally {
    savingProtocol.value = false
  }
}

const deleteProtocol = async (protocol) => {
  if (!confirm('确定要删除此协议吗？')) return
  try {
    await deleteNodeProtocol(selectedNode.value.id, protocol.id)
    const res = await getNodeProtocols(selectedNode.value.id)
    protocols.value = res.data || []
  } catch (e) {
    alert('删除失败: ' + (e.message || e))
  }
}

// 授权密钥操作
const generateKey = async () => {
  generatingKey.value = true
  try {
    await generateAuthKey({ remark: newKeyRemark.value })
    newKeyRemark.value = ''
    loadAuthKeys()
  } catch (e) {
    alert('生成失败: ' + (e.message || e))
  } finally {
    generatingKey.value = false
  }
}

const removeAuthKey = async (key) => {
  if (!confirm('确定要删除此授权密钥吗？')) return
  try {
    await deleteAuthKey(key.id)
    loadAuthKeys()
  } catch (e) {
    alert('删除失败: ' + (e.message || e))
  }
}

const copyKey = (key) => {
  navigator.clipboard.writeText(key)
  alert('已复制到剪贴板')
}

// 分页
const changePage = (page) => {
  pagination.page = page
  loadNodes()
}

// 工具函数
const getStatusClass = (status) => {
  const classes = ['status-pending', 'status-online', 'status-offline', 'status-disabled']
  return classes[status] || 'status-pending'
}

const getStatusText = (status) => {
  const texts = ['待激活', '在线', '离线', '禁用']
  return texts[status] || '未知'
}

const formatBytes = (bytes) => {
  if (!bytes) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  while (bytes >= 1024 && i < units.length - 1) {
    bytes /= 1024
    i++
  }
  return bytes.toFixed(2) + ' ' + units[i]
}

const formatTime = (timestamp) => {
  if (!timestamp) return '-'
  const date = new Date(timestamp * 1000)
  const now = new Date()
  const diff = (now - date) / 1000
  if (diff < 60) return '刚刚'
  if (diff < 3600) return Math.floor(diff / 60) + ' 分钟前'
  if (diff < 86400) return Math.floor(diff / 3600) + ' 小时前'
  return date.toLocaleString()
}

const formatDate = (timestamp) => {
  if (!timestamp) return '-'
  return new Date(timestamp * 1000).toLocaleString()
}

// 初始化
onMounted(() => {
  loadNodes()
  loadStats()
  loadProtocolTemplates()
  loadAuthKeys()
})
</script>

<style scoped>
.nodes-page {
  padding: 24px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
}

.page-header h1 {
  margin: 0;
  font-size: 24px;
}

.header-actions {
  display: flex;
  gap: 12px;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 16px;
  margin-bottom: 24px;
}

.stat-card {
  background: white;
  border-radius: 8px;
  padding: 20px;
  text-align: center;
  box-shadow: 0 2px 8px rgba(0,0,0,0.1);
}

.stat-card.online { border-left: 4px solid #10b981; }
.stat-card.warning { border-left: 4px solid #f59e0b; }
.stat-card.pending { border-left: 4px solid #6366f1; }

.stat-value {
  font-size: 32px;
  font-weight: bold;
  color: #1f2937;
}

.stat-label {
  font-size: 14px;
  color: #6b7280;
  margin-top: 4px;
}

.table-container {
  background: white;
  border-radius: 8px;
  box-shadow: 0 2px 8px rgba(0,0,0,0.1);
  overflow: hidden;
}

.table {
  width: 100%;
  border-collapse: collapse;
}

.table th, .table td {
  padding: 12px 16px;
  text-align: left;
  border-bottom: 1px solid #e5e7eb;
}

.table th {
  background: #f9fafb;
  font-weight: 600;
  color: #374151;
}

.table td code {
  background: #f3f4f6;
  padding: 2px 6px;
  border-radius: 4px;
  font-size: 12px;
}

.node-tags {
  display: flex;
  gap: 4px;
  margin-left: 8px;
}

.tag {
  background: #e0e7ff;
  color: #3730a3;
  padding: 2px 8px;
  border-radius: 12px;
  font-size: 11px;
}

.status-badge {
  display: inline-block;
  padding: 4px 12px;
  border-radius: 12px;
  font-size: 12px;
  font-weight: 500;
}

.status-pending { background: #e0e7ff; color: #3730a3; }
.status-online { background: #d1fae5; color: #065f46; }
.status-offline { background: #fee2e2; color: #991b1b; }
.status-disabled { background: #f3f4f6; color: #6b7280; }

.actions {
  display: flex;
  gap: 8px;
}

.btn {
  padding: 8px 16px;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  font-weight: 500;
  transition: all 0.2s;
}

.btn-primary { background: #6366f1; color: white; }
.btn-primary:hover { background: #4f46e5; }
.btn-secondary { background: #e5e7eb; color: #374151; }
.btn-secondary:hover { background: #d1d5db; }
.btn-info { background: #0ea5e9; color: white; }
.btn-warning { background: #f59e0b; color: white; }
.btn-danger { background: #ef4444; color: white; }

.btn-sm { padding: 4px 10px; font-size: 12px; }
.btn-xs { padding: 2px 6px; font-size: 11px; }

.btn:disabled { opacity: 0.5; cursor: not-allowed; }

.pagination {
  display: flex;
  justify-content: center;
  align-items: center;
  gap: 16px;
  padding: 16px;
}

.modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0,0,0,0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.modal {
  background: white;
  border-radius: 12px;
  width: 480px;
  max-width: 90%;
  max-height: 90vh;
  overflow-y: auto;
}

.modal-lg {
  width: 720px;
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 20px;
  border-bottom: 1px solid #e5e7eb;
}

.modal-header h3 {
  margin: 0;
}

.close-btn {
  background: none;
  border: none;
  font-size: 24px;
  cursor: pointer;
  color: #6b7280;
}

.modal-body {
  padding: 20px;
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  padding: 16px 20px;
  border-top: 1px solid #e5e7eb;
}

.form-group {
  margin-bottom: 16px;
}

.form-group label {
  display: block;
  margin-bottom: 6px;
  font-weight: 500;
  color: #374151;
}

.form-group input,
.form-group select,
.form-group textarea {
  width: 100%;
  padding: 10px 12px;
  border: 1px solid #d1d5db;
  border-radius: 6px;
  font-size: 14px;
}

.form-group textarea {
  font-family: monospace;
  resize: vertical;
}

.form-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
}

.checkbox-label {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
}

.checkbox-label input {
  width: auto;
}

.protocol-header {
  display: flex;
  justify-content: flex-end;
  margin-bottom: 16px;
}

.form-inline {
  display: flex;
  gap: 12px;
  align-items: center;
}

.protocol-type {
  font-weight: 600;
  color: #6366f1;
}

.empty-message {
  text-align: center;
  padding: 32px;
  color: #6b7280;
}

.info-box {
  background: #eff6ff;
  border: 1px solid #bfdbfe;
  border-radius: 8px;
  padding: 12px 16px;
  margin-bottom: 16px;
  color: #1e40af;
}

.key-text {
  font-size: 12px;
  background: #f3f4f6;
  padding: 4px 8px;
  border-radius: 4px;
  word-break: break-all;
}

.text-center {
  text-align: center;
}
</style>
