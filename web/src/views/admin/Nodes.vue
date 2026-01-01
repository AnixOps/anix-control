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
      <div class="modal modal-lg">
        <div class="modal-header">
          <h3>{{ editingProtocol ? '编辑协议' : '添加协议' }}</h3>
          <button class="close-btn" @click="closeProtocolFormModal">×</button>
        </div>
        <div class="modal-body">
          <div class="form-group" v-if="!editingProtocol">
            <label>协议模板库 (Prefab Templates)</label>
            <div class="template-grid">
              <div 
                v-for="tpl in protocolTemplates" 
                :key="tpl.type + tpl.name" 
                :class="['template-card', selectedTemplate === tpl.name ? 'active' : '']"
                @click="applyTemplate(tpl)"
              >
                <div class="tpl-name">{{ tpl.name }}</div>
                <div class="tpl-desc">{{ tpl.description }}</div>
              </div>
            </div>
          </div>

          <div class="tabs">
            <button :class="['tab-btn', protocolForm.mode === 'general' ? 'active' : '']" @click="protocolForm.mode = 'general'">基础配置</button>
            <button :class="['tab-btn', protocolForm.mode === 'custom' ? 'active' : '']" @click="protocolForm.mode = 'custom'">JSON 高级模式</button>
          </div>

          <div v-if="protocolForm.mode === 'general'" class="protocol-editor">
            <div class="form-row">
              <div class="form-group">
                <label>协议类型 *</label>
                <select v-model="protocolForm.type">
                  <option value="vmess">VMess</option>
                  <option value="vless">VLESS</option>
                  <option value="trojan">Trojan</option>
                  <option value="shadowsocks">Shadowsocks</option>
                  <option value="hysteria2">Hysteria2</option>
                  <option value="tuic">TUIC</option>
                </select>
              </div>
              <div class="form-group">
                <label>监听端口 *</label>
                <input v-model.number="protocolForm.port" type="number" placeholder="443" />
              </div>
            </div>

            <div class="form-row">
              <div class="form-group">
                <label>TLS 模式</label>
                <select v-model.number="protocolForm.tls">
                  <option :value="0">无 TLS</option>
                  <option :value="1">标准 TLS</option>
                  <option :value="2">Reality (推荐)</option>
                </select>
              </div>
              <div class="form-group">
                <label>传输层协议</label>
                <select v-model="protocolForm.transport">
                  <option value="tcp">TCP</option>
                  <option value="ws">WebSocket</option>
                  <option value="grpc">gRPC</option>
                  <option value="quic">QUIC</option>
                  <option value="h2">HTTP/2</option>
                </select>
              </div>
            </div>

            <div class="form-group">
              <label>协议设置 (JSON Settings)</label>
              <textarea v-model="protocolForm.settings" rows="3" placeholder='{"flow": "xtls-rprx-vision"}'></textarea>
            </div>

            <div class="form-group" v-if="protocolForm.tls > 0">
              <label>TLS 设置 (JSON)</label>
              <textarea v-model="protocolForm.tls_settings" rows="3" placeholder='{"server_name": "example.com"}'></textarea>
            </div>

            <div class="form-group" v-if="protocolForm.tls === 2">
              <label>Reality 设置 (JSON)</label>
              <textarea v-model="protocolForm.reality_settings" rows="3" placeholder='{"short_id": "..."}'></textarea>
            </div>

            <div class="form-group" v-if="protocolForm.transport !== 'tcp'">
              <label>传输层设置 (JSON)</label>
              <textarea v-model="protocolForm.transport_settings" rows="3" placeholder='{"path": "/ws"}'></textarea>
            </div>
          </div>

          <div v-else class="protocol-editor">
            <div class="info-box">
              高级模式将全量覆盖此协议的所有配置。请输入完整的 JSON 对象。
            </div>
            <div class="form-group">
              <label>自定义全量配置 (Custom JSON Override)</label>
              <textarea v-model="protocolForm.custom_config" rows="15" placeholder='{ "node_type": "vless", ... }'></textarea>
            </div>
          </div>

          <div class="form-group">
            <label class="checkbox-label">
              <input type="checkbox" v-model="protocolForm.enable" :true-value="1" :false-value="0" />
              启用此协议
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
          
          <!-- 一次性密钥显示 -->
          <div v-if="generatedKey" class="key-display-box">
            <div class="key-display-header">
              <span>⚠️ 请立即复制此密钥，关闭后将无法再次查看！</span>
              <button class="close-btn" @click="generatedKey = ''">×</button>
            </div>
            <div class="key-display-content">
              <code class="key-text-large">{{ generatedKey }}</code>
              <button class="btn btn-primary" @click="copyKey(generatedKey); generatedKey = ''">复制并关闭</button>
            </div>
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
                <th>名称/备注</th>
                <th>状态</th>
                <th>创建时间</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="key in authKeys" :key="key.id">
                <td>
                  <code class="key-text">{{ key.name || '-' }}</code>
                </td>
                <td>
                  <span v-if="key.used_by_node_id" class="badge badge-success">已使用</span>
                  <span v-else class="badge badge-info">未使用</span>
                </td>
                <td>{{ formatDate(key.created_at) }}</td>
                <td>
                  <button class="btn btn-sm btn-danger" @click="removeAuthKey(key)" :disabled="key.used_by_node_id">删除</button>
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
  mode: 'general', // general | custom
  type: 'vless',
  port: 443,
  enable: 1,
  tls: 0,
  transport: 'tcp',
  settings: '{}',
  tls_settings: '{}',
  transport_settings: '{}',
  reality_settings: '{}',
  custom_config: ''
})
const protocolTemplates = ref([])
const selectedTemplate = ref('')

const showAuthKeys = ref(false)
const authKeys = ref([])
const newKeyRemark = ref('')
const generatedKey = ref('')  // 一次性显示的密钥

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
  Object.assign(protocolForm, { 
    mode: 'general',
    type: 'vless', 
    port: 443, 
    enable: 1,
    tls: 0,
    transport: 'tcp',
    settings: '{}',
    tls_settings: '{}',
    transport_settings: '{}',
    reality_settings: '{}',
    custom_config: ''
  })
  selectedTemplate.value = ''
  showProtocolFormModal.value = true
}

const editProtocol = (protocol) => {
  editingProtocol.value = protocol
  Object.assign(protocolForm, {
    mode: protocol.custom_config ? 'custom' : 'general',
    type: protocol.type,
    port: protocol.port,
    enable: protocol.enable,
    tls: protocol.tls || 0,
    transport: protocol.transport || 'tcp',
    settings: protocol.settings || '{}',
    tls_settings: protocol.tls_settings || '{}',
    transport_settings: protocol.transport_settings || '{}',
    reality_settings: protocol.reality_settings || '{}',
    custom_config: protocol.custom_config || ''
  })
  showProtocolFormModal.value = true
}

const closeProtocolFormModal = () => {
  showProtocolFormModal.value = false
  editingProtocol.value = null
}

const applyTemplate = (tpl) => {
  selectedTemplate.value = tpl.name
  protocolForm.type = tpl.type
  protocolForm.port = tpl.default_port
  protocolForm.tls = tpl.tls || 0
  protocolForm.transport = tpl.transport || 'tcp'
  protocolForm.settings = tpl.settings || '{}'
  protocolForm.tls_settings = tpl.tls_settings || '{}'
  protocolForm.reality_settings = tpl.reality_settings || '{}'
  protocolForm.mode = 'general'
}

const saveProtocol = async () => {
  if (protocolForm.mode === 'general' && (!protocolForm.type || !protocolForm.port)) {
    alert('请填写必填字段')
    return
  }
  
  savingProtocol.value = true
  try {
    const data = {
      type: protocolForm.type,
      port: protocolForm.port,
      enable: protocolForm.enable,
      tls: protocolForm.tls,
      transport: protocolForm.transport,
      settings: protocolForm.settings,
      tls_settings: protocolForm.tls_settings,
      transport_settings: protocolForm.transport_settings,
      reality_settings: protocolForm.reality_settings,
      custom_config: protocolForm.mode === 'custom' ? protocolForm.custom_config : null
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
    const res = await generateAuthKey({ name: newKeyRemark.value })
    newKeyRemark.value = ''
    // 显示一次性密钥
    if (res.data && res.data.key) {
      generatedKey.value = res.data.key
    }
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
  padding: 0;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
  flex-wrap: wrap;
  gap: 16px;
}

.page-header h1 {
  margin: 0;
  font-size: 24px;
  font-weight: 600;
  color: var(--text-color);
}

.header-actions {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
}

/* 统计卡片 - 使用深色主题 */
.stats-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 16px;
  margin-bottom: 24px;
}

.stat-card {
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  padding: 20px;
  text-align: center;
  transition: var(--transition);
}

.stat-card:hover {
  border-color: var(--text-secondary);
}

.stat-card.online { border-left: 4px solid var(--success-color); }
.stat-card.warning { border-left: 4px solid var(--warning-color); }
.stat-card.pending { border-left: 4px solid var(--primary-color); }

.stat-value {
  font-size: 32px;
  font-weight: 700;
  color: var(--text-color);
}

.stat-label {
  font-size: 14px;
  color: var(--text-secondary);
  margin-top: 4px;
}

/* 表格容器 - 深色主题 */
.table-container {
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  overflow: hidden;
}

.table {
  width: 100%;
  border-collapse: collapse;
}

.table th, .table td {
  padding: 14px 16px;
  text-align: left;
  border-bottom: 1px solid var(--border-color);
}

.table th {
  background: var(--bg-color);
  font-weight: 600;
  color: var(--text-secondary);
  font-size: 13px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.table tr:hover {
  background: var(--surface-hover);
}

.table td code {
  background: var(--bg-color);
  border: 1px solid var(--border-color);
  padding: 4px 8px;
  border-radius: var(--radius-sm);
  font-size: 12px;
  color: var(--text-color);
}

.node-tags {
  display: flex;
  gap: 4px;
  margin-left: 8px;
  flex-wrap: wrap;
}

.tag {
  background: rgba(99, 102, 241, 0.2);
  color: #a5b4fc;
  padding: 2px 8px;
  border-radius: 12px;
  font-size: 11px;
}

/* 状态徽章 - 深色主题 */
.status-badge {
  display: inline-block;
  padding: 4px 12px;
  border-radius: 12px;
  font-size: 12px;
  font-weight: 500;
}

.status-pending { background: rgba(99, 102, 241, 0.2); color: #a5b4fc; }
.status-online { background: rgba(34, 197, 94, 0.2); color: var(--success-color); }
.status-offline { background: rgba(239, 68, 68, 0.2); color: var(--error-color); }
.status-disabled { background: rgba(161, 161, 170, 0.2); color: var(--text-secondary); }

.actions {
  display: flex;
  gap: 8px;
}

/* 按钮样式 - 使用主题变量 */
.btn {
  padding: 8px 16px;
  border: none;
  border-radius: var(--radius-md);
  cursor: pointer;
  font-weight: 500;
  transition: var(--transition);
  font-size: 14px;
}

.btn-primary { background: var(--primary-color); color: white; }
.btn-primary:hover { background: var(--primary-hover); transform: translateY(-1px); }
.btn-secondary { background: var(--surface-color); color: var(--text-color); border: 1px solid var(--border-color); }
.btn-secondary:hover { background: var(--surface-hover); border-color: var(--text-secondary); }
.btn-info { background: #0ea5e9; color: white; }
.btn-info:hover { background: #0284c7; }
.btn-warning { background: var(--warning-color); color: white; }
.btn-warning:hover { background: #d97706; }
.btn-danger { background: var(--error-color); color: white; }
.btn-danger:hover { background: #dc2626; }

.btn-sm { padding: 6px 12px; font-size: 12px; }
.btn-xs { padding: 4px 8px; font-size: 11px; }

.btn:disabled { opacity: 0.5; cursor: not-allowed; transform: none; }

/* 分页 */
.pagination {
  display: flex;
  justify-content: center;
  align-items: center;
  gap: 16px;
  padding: 20px;
  color: var(--text-secondary);
}

.pagination button {
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  color: var(--text-color);
}

.pagination button:disabled {
  opacity: 0.4;
}

/* 模态框 - 深色主题 */
.modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.7);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  backdrop-filter: blur(4px);
}

.modal {
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  width: 480px;
  max-width: 90%;
  max-height: 90vh;
  overflow-y: auto;
  box-shadow: var(--shadow-lg);
}

.modal-lg {
  width: 720px;
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 20px 24px;
  border-bottom: 1px solid var(--border-color);
}

.modal-header h3 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  color: var(--text-color);
}

.close-btn {
  background: none;
  border: none;
  font-size: 24px;
  cursor: pointer;
  color: var(--text-secondary);
  padding: 4px;
  line-height: 1;
  transition: var(--transition);
}

.close-btn:hover {
  color: var(--text-color);
}

.modal-body {
  padding: 24px;
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  padding: 20px 24px;
  border-top: 1px solid var(--border-color);
}

/* 表单样式 - 深色主题 */
.form-group {
  margin-bottom: 20px;
}

.form-group label {
  display: block;
  margin-bottom: 8px;
  font-weight: 500;
  color: var(--text-color);
  font-size: 14px;
}

.form-group input,
.form-group select,
.form-group textarea {
  width: 100%;
  padding: 12px 14px;
  background: var(--bg-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  font-size: 14px;
  color: var(--text-color);
  transition: var(--transition);
}

.form-group input:focus,
.form-group select:focus,
.form-group textarea:focus {
  border-color: var(--primary-color);
  outline: none;
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
}

.form-group input::placeholder,
.form-group textarea::placeholder {
  color: var(--text-secondary);
}

.form-group textarea {
  font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;
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
  color: var(--text-color);
}

.checkbox-label input {
  width: auto;
  accent-color: var(--primary-color);
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
  color: var(--primary-color);
}

.empty-message {
  text-align: center;
  padding: 40px;
  color: var(--text-secondary);
}

.info-box {
  background: rgba(59, 130, 246, 0.1);
  border: 1px solid rgba(59, 130, 246, 0.3);
  border-radius: var(--radius-md);
  padding: 14px 18px;
  margin-bottom: 20px;
  color: var(--primary-color);
  font-size: 14px;
}

.key-text {
  font-size: 12px;
  background: var(--bg-color);
  padding: 4px 8px;
  border-radius: var(--radius-sm);
  word-break: break-all;
  color: var(--text-color);
  font-family: monospace;
}

.key-display-box {
  background: rgba(245, 158, 11, 0.1);
  border: 2px solid var(--warning-color);
  border-radius: var(--radius-md);
  padding: 20px;
  margin-bottom: 20px;
}

.key-display-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
  color: var(--warning-color);
  font-weight: 600;
}

.key-display-content {
  display: flex;
  flex-direction: column;
  gap: 16px;
  align-items: center;
}

.key-text-large {
  font-size: 14px;
  font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;
  background: var(--bg-color);
  padding: 16px 20px;
  border-radius: var(--radius-md);
  word-break: break-all;
  width: 100%;
  text-align: center;
  border: 1px solid var(--warning-color);
  color: var(--text-color);
}

/* 协议模板样式 */
.template-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 12px;
  margin-bottom: 20px;
}

.template-card {
  padding: 12px;
  background: var(--bg-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: var(--transition);
}

.template-card:hover {
  border-color: var(--primary-color);
  background: var(--surface-hover);
}

.template-card.active {
  border-color: var(--primary-color);
  background: rgba(59, 130, 246, 0.1);
  box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.2);
}

.tpl-name {
  font-weight: 600;
  font-size: 13px;
  margin-bottom: 4px;
  color: var(--primary-color);
}

.tpl-desc {
  font-size: 11px;
  color: var(--text-secondary);
  line-height: 1.4;
}

/* 标签页 */
.tabs {
  display: flex;
  gap: 8px;
  margin-bottom: 20px;
  border-bottom: 1px solid var(--border-color);
  padding-bottom: 12px;
}

.tab-btn {
  background: none;
  border: none;
  color: var(--text-secondary);
  padding: 8px 16px;
  border-radius: var(--radius-md);
  cursor: pointer;
  font-size: 14px;
  transition: var(--transition);
}

.tab-btn:hover {
  color: var(--text-color);
  background: var(--bg-color);
}

.tab-btn.active {
  color: white;
  background: var(--primary-color);
}
.badge {
  display: inline-block;
  padding: 4px 10px;
  border-radius: var(--radius-sm);
  font-size: 12px;
  font-weight: 500;
}

.badge-success {
  background: rgba(34, 197, 94, 0.2);
  color: var(--success-color);
}

.badge-info {
  background: rgba(59, 130, 246, 0.2);
  color: var(--primary-color);
}

.text-center {
  text-align: center;
}

/* 响应式优化 */
@media (max-width: 768px) {
  .page-header {
    flex-direction: column;
    align-items: flex-start;
  }
  
  .header-actions {
    width: 100%;
  }
  
  .header-actions .btn {
    flex: 1;
  }
  
  .form-row {
    grid-template-columns: 1fr;
  }
  
  .table-container {
    overflow-x: auto;
  }
  
  .table {
    min-width: 800px;
  }
  
  .actions {
    flex-direction: column;
  }
  
  .modal {
    max-width: 95%;
  }
}
</style>
