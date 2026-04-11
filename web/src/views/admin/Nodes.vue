<template>
  <div class="page nodes-page">
    <div class="page-header">
      <h1>{{ t('admin.nodes.title') }}</h1>
      <div class="header-actions">
        <button class="btn btn-secondary" @click="showAuthKeys = true">
          {{ t('admin.nodes.authKeys') }}
        </button>
        <button class="btn btn-primary" @click="openCreateModal">
          + {{ t('admin.nodes.addNode') }}
        </button>
      </div>
    </div>

    <!-- Node stats -->
    <div class="stats-grid">
      <div class="stat-card">
        <div class="stat-value">{{ stats.total }}</div>
        <div class="stat-label">{{ t('admin.nodes.stats.total') }}</div>
      </div>
      <div class="stat-card online">
        <div class="stat-value">{{ stats.online }}</div>
        <div class="stat-label">{{ t('admin.nodes.stats.online') }}</div>
      </div>
      <div class="stat-card warning">
        <div class="stat-value">{{ stats.offline }}</div>
        <div class="stat-label">{{ t('admin.nodes.stats.offline') }}</div>
      </div>
      <div class="stat-card pending">
        <div class="stat-value">{{ stats.pending }}</div>
        <div class="stat-label">{{ t('admin.nodes.stats.pending') }}</div>
      </div>
    </div>

    <!-- Node list -->
    <div class="table-container">
      <table class="table">
        <thead>
          <tr>
            <th>{{ t('networkPages.nodes.table.id') }}</th>
            <th>{{ t('admin.nodes.table.name') }}</th>
            <th>{{ t('admin.nodes.table.address') }}</th>
            <th>{{ t('admin.nodes.table.status') }}</th>
            <th>{{ t('admin.nodes.table.protocols') }}</th>
            <th>{{ t('admin.nodes.table.traffic') }}</th>
            <th>{{ t('admin.nodes.table.lastHeartbeat') }}</th>
            <th>{{ t('admin.nodes.table.actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="loading">
            <td colspan="8" class="text-center">{{ t('admin.nodes.table.loading') }}</td>
          </tr>
          <tr v-else-if="nodes.length === 0">
            <td colspan="8" class="text-center">{{ t('admin.nodes.table.empty') }}</td>
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
            <td>{{ node.protocols?.length || 0 }}</td>
            <td>{{ formatBytes(node.traffic_today || 0) }}</td>
            <td>{{ formatTime(node.last_check_at) }}</td>
            <td class="actions">
              <button
                class="btn btn-sm btn-info"
                :title="t('admin.nodes.actions.manageProtocols')"
                :aria-label="t('admin.nodes.actions.manageProtocols')"
                @click="openProtocols(node)"
              >
                {{ t('admin.nodes.actions.protocols') }}
              </button>
              <button
                class="btn btn-sm btn-warning"
                :title="t('admin.nodes.actions.edit')"
                :aria-label="t('admin.nodes.actions.edit')"
                @click="openEditModal(node)"
              >
                {{ t('admin.nodes.actions.edit') }}
              </button>
              <button
                class="btn btn-sm btn-danger"
                :title="t('admin.nodes.actions.delete')"
                :aria-label="t('admin.nodes.actions.delete')"
                @click="confirmDelete(node)"
              >
                {{ t('admin.nodes.actions.delete') }}
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Pagination -->
    <div class="pagination" v-if="pagination.total > pagination.size">
      <button :disabled="pagination.page === 1" @click="changePage(pagination.page - 1)">
        {{ t('admin.nodes.pagination.previous') }}
      </button>
      <span>{{ pagination.page }} / {{ Math.ceil(pagination.total / pagination.size) }}</span>
      <button :disabled="pagination.page >= Math.ceil(pagination.total / pagination.size)" @click="changePage(pagination.page + 1)">
        {{ t('admin.nodes.pagination.next') }}
      </button>
    </div>

    <!-- Create/edit node modal -->
    <div class="modal-overlay" v-if="showNodeModal" @click.self="closeNodeModal">
      <div class="modal">
        <div class="modal-header">
          <h3>{{ editingNode ? t('admin.nodes.nodeModal.titleEdit') : t('admin.nodes.nodeModal.titleCreate') }}</h3>
          <button class="close-btn" :title="t('common.actions.close')" :aria-label="t('common.actions.close')" @click="closeNodeModal">×</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label>{{ t('admin.nodes.nodeModal.fields.name') }}</label>
            <input v-model="nodeForm.name" type="text" :placeholder="t('admin.nodes.nodeModal.placeholders.name')" />
          </div>
          <div class="form-row">
            <div class="form-group">
              <label>{{ t('admin.nodes.nodeModal.fields.address') }}</label>
              <input v-model="nodeForm.address" type="text" :placeholder="t('admin.nodes.nodeModal.placeholders.address')" />
            </div>
            <div class="form-group">
              <label>{{ t('admin.nodes.nodeModal.fields.apiPort') }}</label>
              <input v-model.number="nodeForm.api_port" type="number" :placeholder="t('admin.nodes.nodeModal.placeholders.apiPort')" />
            </div>
          </div>
          <div class="form-group">
            <label>{{ t('admin.nodes.nodeModal.fields.tags') }}</label>
            <input v-model="nodeForm.tags" type="text" :placeholder="t('admin.nodes.nodeModal.placeholders.tags')" />
          </div>
          <div class="form-row">
            <div class="form-group">
              <label>{{ t('admin.nodes.nodeModal.fields.rate') }}</label>
              <input v-model.number="nodeForm.rate" type="number" step="0.1" :placeholder="t('admin.nodes.nodeModal.placeholders.rate')" />
            </div>
            <div class="form-group">
              <label>{{ t('admin.nodes.nodeModal.fields.sort') }}</label>
              <input v-model.number="nodeForm.sort" type="number" :placeholder="t('admin.nodes.nodeModal.placeholders.sort')" />
            </div>
          </div>
          <div class="form-group" v-if="editingNode">
            <label>{{ t('admin.nodes.nodeModal.fields.status') }}</label>
            <select v-model.number="nodeForm.status">
              <option :value="0">{{ t('admin.nodes.statusText.pending') }}</option>
              <option :value="1">{{ t('admin.nodes.statusText.online') }}</option>
              <option :value="2">{{ t('admin.nodes.statusText.offline') }}</option>
              <option :value="3">{{ t('admin.nodes.statusText.disabled') }}</option>
            </select>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary" @click="closeNodeModal">{{ t('admin.nodes.actions.cancel') }}</button>
          <button class="btn btn-primary" @click="saveNode" :disabled="saving">
            {{ saving ? t('admin.nodes.actions.saving') : t('admin.nodes.actions.save') }}
          </button>
        </div>
      </div>
    </div>

    <!-- Protocol management modal -->
    <div class="modal-overlay" v-if="showProtocolModal" @click.self="closeProtocolModal">
      <div class="modal modal-lg">
        <div class="modal-header">
          <h3>{{ t('admin.nodes.protocolModal.title', { name: selectedNode?.name || "-" }) }}</h3>
          <button class="close-btn" :title="t('common.actions.close')" :aria-label="t('common.actions.close')" @click="closeProtocolModal">×</button>
        </div>
        <div class="modal-body">
          <div class="protocol-header">
            <button class="btn btn-primary btn-sm" @click="openAddProtocol">
              + {{ t('admin.nodes.protocolModal.addProtocol') }}
            </button>
          </div>
          
          <table class="table" v-if="protocols.length > 0">
            <thead>
              <tr>
                <th>{{ t('admin.nodes.protocolModal.table.type') }}</th>
                <th>{{ t('admin.nodes.protocolModal.table.port') }}</th>
                <th>{{ t('admin.nodes.protocolModal.table.status') }}</th>
                <th>{{ t('admin.nodes.protocolModal.table.actions') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="protocol in protocols" :key="protocol.id">
                <td>
                  <span class="protocol-type">{{ (protocol.type || 'unknown').toUpperCase() }}</span>
                </td>
                <td>{{ protocol.port }}</td>
                <td>
                  <span :class="['status-badge', protocol.enable ? 'status-online' : 'status-disabled']">
                    {{ protocol.enable ? t('admin.nodes.protocolModal.status.enabled') : t('admin.nodes.protocolModal.status.disabled') }}
                  </span>
                </td>
                <td>
                  <button class="btn btn-sm btn-warning" @click="editProtocol(protocol)">{{ t('admin.nodes.actions.edit') }}</button>
                  <button class="btn btn-sm btn-danger" @click="deleteProtocol(protocol)">{{ t('admin.nodes.actions.delete') }}</button>
                </td>
              </tr>
            </tbody>
          </table>
          <div v-else class="empty-message">{{ t('admin.nodes.protocolModal.empty') }}</div>
        </div>
      </div>
    </div>

    <!-- Create/edit protocol modal -->
    <div class="modal-overlay" v-if="showProtocolFormModal" @click.self="closeProtocolFormModal">
      <div class="modal modal-lg">
        <div class="modal-header">
          <h3>{{ editingProtocol ? t('admin.nodes.protocolForm.titleEdit') : t('admin.nodes.protocolForm.titleCreate') }}</h3>
          <button class="close-btn" :title="t('common.actions.close')" :aria-label="t('common.actions.close')" @click="closeProtocolFormModal">×</button>
        </div>
        <div class="modal-body">
          <div class="form-group" v-if="!editingProtocol">
            <label>{{ t('admin.nodes.protocolForm.templateLibrary') }}</label>
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
            <button :class="['tab-btn', protocolForm.mode === 'general' ? 'active' : '']" @click="protocolForm.mode = 'general'">
              {{ t('admin.nodes.protocolForm.tabs.general') }}
            </button>
            <button :class="['tab-btn', protocolForm.mode === 'custom' ? 'active' : '']" @click="protocolForm.mode = 'custom'">
              {{ t('admin.nodes.protocolForm.tabs.custom') }}
            </button>
          </div>

          <div v-if="protocolForm.mode === 'general'" class="protocol-editor">
            <div class="form-row">
              <div class="form-group">
                <label>{{ t('admin.nodes.protocolForm.fields.type') }}</label>
                <select v-model="protocolForm.type">
                  <option value="vmess">{{ t('networkPages.nodes.protocols.vmess') }}</option>
                  <option value="vless">{{ t('networkPages.nodes.protocols.vless') }}</option>
                  <option value="trojan">{{ t('networkPages.nodes.protocols.trojan') }}</option>
                  <option value="shadowsocks">{{ t('networkPages.nodes.protocols.shadowsocks') }}</option>
                  <option value="hysteria2">{{ t('networkPages.nodes.protocols.hysteria2') }}</option>
                  <option value="tuic">{{ t('networkPages.nodes.protocols.tuic') }}</option>
                </select>
              </div>
              <div class="form-group">
                <label>{{ t('admin.nodes.protocolForm.fields.port') }}</label>
                <input v-model.number="protocolForm.port" type="number" :placeholder="t('admin.nodes.protocolForm.placeholders.port')" />
              </div>
            </div>

            <div class="form-row">
              <div class="form-group">
                <label>{{ t('admin.nodes.protocolForm.fields.tls') }}</label>
                <select v-model.number="protocolForm.tls">
                  <option :value="0">{{ t('admin.nodes.tlsModes.none') }}</option>
                  <option :value="1">{{ t('admin.nodes.tlsModes.standard') }}</option>
                  <option :value="2">{{ t('admin.nodes.tlsModes.reality') }}</option>
                </select>
              </div>
              <div class="form-group">
                <label>{{ t('admin.nodes.protocolForm.fields.transport') }}</label>
                <select v-model="protocolForm.transport">
                  <option value="tcp">{{ t('admin.nodes.transports.tcp') }}</option>
                  <option value="ws">{{ t('admin.nodes.transports.ws') }}</option>
                  <option value="grpc">{{ t('admin.nodes.transports.grpc') }}</option>
                  <option value="quic">{{ t('admin.nodes.transports.quic') }}</option>
                  <option value="h2">{{ t('admin.nodes.transports.h2') }}</option>
                </select>
              </div>
            </div>

            <div class="form-group">
              <label>{{ t('admin.nodes.protocolForm.fields.settings') }}</label>
              <textarea
                v-model="protocolForm.settings"
                rows="3"
                :placeholder="t('networkPages.nodes.protocolPlaceholders.settings')"
              ></textarea>
            </div>

            <div class="form-group" v-if="protocolForm.tls > 0">
              <label>{{ t('admin.nodes.protocolForm.fields.tlsSettings') }}</label>
              <textarea
                v-model="protocolForm.tls_settings"
                rows="3"
                :placeholder="t('networkPages.nodes.protocolPlaceholders.tlsSettings')"
              ></textarea>
            </div>

            <div class="form-group" v-if="protocolForm.tls === 2">
              <label>{{ t('admin.nodes.protocolForm.fields.realitySettings') }}</label>
              <textarea
                v-model="protocolForm.reality_settings"
                rows="3"
                :placeholder="t('networkPages.nodes.protocolPlaceholders.realitySettings')"
              ></textarea>
            </div>

            <div class="form-group" v-if="protocolForm.transport !== 'tcp'">
              <label>{{ t('admin.nodes.protocolForm.fields.transportSettings') }}</label>
              <textarea
                v-model="protocolForm.transport_settings"
                rows="3"
                :placeholder="t('networkPages.nodes.protocolPlaceholders.transportSettings')"
              ></textarea>
            </div>
          </div>

          <div v-else class="protocol-editor">
            <div class="info-box">
              {{ t('admin.nodes.protocolForm.customModeHint') }}
            </div>
            <div class="form-group">
              <label>{{ t('admin.nodes.protocolForm.fields.customConfig') }}</label>
              <textarea
                v-model="protocolForm.custom_config"
                rows="15"
                :placeholder="t('networkPages.nodes.protocolPlaceholders.customConfig')"
              ></textarea>
            </div>
          </div>

          <div class="form-row">
            <div class="form-group">
              <label class="checkbox-label">
                <input type="checkbox" v-model="protocolForm.enable" :true-value="1" :false-value="0" />
                <span>{{ t('admin.nodes.protocolForm.enableHint') }}</span>
              </label>
            </div>
            <div class="form-group">
              <label class="checkbox-label">
                <input type="checkbox" v-model="protocolForm.show" :true-value="1" :false-value="0" />
                <span>{{ t('admin.nodes.protocolForm.showHint') }}</span>
              </label>
            </div>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary" @click="closeProtocolFormModal">{{ t('admin.nodes.actions.cancel') }}</button>
          <button class="btn btn-primary" @click="saveProtocol" :disabled="savingProtocol">
            {{ savingProtocol ? t('admin.nodes.actions.saving') : t('admin.nodes.actions.save') }}
          </button>
        </div>
      </div>
    </div>

    <!-- Auth key modal -->
    <div class="modal-overlay" v-if="showAuthKeys" @click.self="showAuthKeys = false">
      <div class="modal modal-lg">
        <div class="modal-header">
          <h3>{{ t('admin.nodes.authKeyModal.title') }}</h3>
          <button class="close-btn" :title="t('common.actions.close')" :aria-label="t('common.actions.close')" @click="showAuthKeys = false">×</button>
        </div>
        <div class="modal-body">
          <div class="info-box">
            <p>{{ t('admin.nodes.authKeyModal.description') }}</p>
          </div>
          
          <!-- One-time key preview -->
          <div v-if="generatedKey" class="key-display-box">
            <div class="key-display-header">
              <span>{{ t('admin.nodes.authKeyModal.oneTimeWarning') }}</span>
              <button class="close-btn" :title="t('common.actions.close')" :aria-label="t('common.actions.close')" @click="generatedKey = ''">×</button>
            </div>
            <div class="key-display-content">
              <code class="key-text-large">{{ generatedKey }}</code>
              <button class="btn btn-primary" @click="copyGeneratedKey">{{ t('admin.nodes.authKeyModal.copyAndClose') }}</button>
            </div>
          </div>

          <div class="protocol-header">
            <div class="form-inline">
              <input
                v-model="newKeyRemark"
                type="text"
                :placeholder="t('admin.nodes.authKeyModal.remarkPlaceholder')"
                style="width: 200px;"
              />
              <button class="btn btn-primary btn-sm" @click="generateKey" :disabled="generatingKey">
                {{ generatingKey ? t('admin.nodes.authKeyModal.generating') : `+ ${t('admin.nodes.authKeyModal.generateNew')}` }}
              </button>
            </div>
          </div>

          <table class="table" v-if="authKeys.length > 0">
            <thead>
              <tr>
                <th>{{ t('admin.nodes.authKeyModal.table.name') }}</th>
                <th>{{ t('admin.nodes.authKeyModal.table.status') }}</th>
                <th>{{ t('admin.nodes.authKeyModal.table.createdAt') }}</th>
                <th>{{ t('admin.nodes.authKeyModal.table.actions') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="key in authKeys" :key="key.id">
                <td>
                  <code class="key-text">{{ key.name || '-' }}</code>
                </td>
                <td>
                  <span v-if="key.used_by_node_id" class="badge badge-success">{{ t('admin.nodes.authKeyModal.status.used') }}</span>
                  <span v-else class="badge badge-info">{{ t('admin.nodes.authKeyModal.status.unused') }}</span>
                </td>
                <td>{{ formatDate(key.created_at) }}</td>
                <td>
                  <button class="btn btn-sm btn-danger" @click="removeAuthKey(key)" :disabled="key.used_by_node_id">
                    {{ t('admin.nodes.actions.delete') }}
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
          <div v-else class="empty-message">{{ t('admin.nodes.authKeyModal.empty') }}</div>
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
import { useAppI18n } from '@/composables/useAppI18n'

const { t, formatDateTime } = useAppI18n()

// State
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
  custom_config: '',
  show: 1
})
const protocolTemplates = ref([])
const selectedTemplate = ref('')

const showAuthKeys = ref(false)
const authKeys = ref([])
const newKeyRemark = ref('')
const generatedKey = ref('')

// Data loaders
const normalizeNode = (node) => ({
  ...node,
  address: node.address || node.host || '',
  api_port: node.api_port || node.port || 443,
  traffic_today: node.traffic_today || (Number(node.total_upload || 0) + Number(node.total_download || 0))
})

const loadNodes = async () => {
  loading.value = true
  try {
    const res = await getNodes({ page: pagination.page, page_size: pagination.size })
    nodes.value = (res.data.list || []).map(normalizeNode)
    pagination.total = res.data.total || 0
    return true
  } catch (e) {
    console.error('Failed to load nodes:', e)
    return false
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

// Node actions
const openCreateModal = () => {
  editingNode.value = null
  Object.assign(nodeForm, { name: '', address: '', api_port: 8080, tags: '', rate: 1.0, sort: 0, status: 0 })
  showNodeModal.value = true
}

const openEditModal = (node) => {
  editingNode.value = node
  Object.assign(nodeForm, {
    name: node.name,
    address: node.address || node.host || '',
    api_port: node.api_port || node.port || 443,
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
    alert(t('admin.nodes.messages.requiredFields'))
    return
  }
  saving.value = true
  try {
    const payload = {
      name: nodeForm.name,
      host: nodeForm.address,
      port: Number(nodeForm.api_port),
      tags: nodeForm.tags,
      rate: Number(nodeForm.rate),
      sort: Number(nodeForm.sort),
      status: Number(nodeForm.status)
    }
    if (editingNode.value) {
      await updateNode(editingNode.value.id, payload)
    } else {
      await createNode(payload)
    }
    closeNodeModal()
    loadNodes()
    loadStats()
  } catch (e) {
    alert(t('admin.nodes.messages.saveFailed', { message: e.message || e }))
  } finally {
    saving.value = false
  }
}

const confirmDelete = async (node) => {
  if (!confirm(t('admin.nodes.messages.deleteNodeConfirm', { name: node.name }))) return
  try {
    await deleteNode(node.id)
    loadNodes()
    loadStats()
  } catch (e) {
    alert(t('admin.nodes.messages.deleteFailed', { message: e.message || e }))
  }
}

// Protocol actions
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
    custom_config: '',
    show: 1
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
    custom_config: protocol.custom_config || '',
    show: protocol.show ?? 1
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
    alert(t('admin.nodes.messages.requiredFields'))
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
      custom_config: protocolForm.mode === 'custom' ? protocolForm.custom_config : null,
      show: protocolForm.show
    }
    
    if (editingProtocol.value) {
      await updateNodeProtocol(selectedNode.value.id, editingProtocol.value.id, data)
    } else {
      await createNodeProtocol(selectedNode.value.id, data)
    }
    
    closeProtocolFormModal()
    const res = await getNodeProtocols(selectedNode.value.id)
    protocols.value = res.data || []
  } catch (e) {
    alert(t('admin.nodes.messages.saveFailed', { message: e.message || e }))
  } finally {
    savingProtocol.value = false
  }
}

const deleteProtocol = async (protocol) => {
  if (!confirm(t('admin.nodes.messages.deleteProtocolConfirm'))) return
  try {
    await deleteNodeProtocol(selectedNode.value.id, protocol.id)
    const res = await getNodeProtocols(selectedNode.value.id)
    protocols.value = res.data || []
  } catch (e) {
    alert(t('admin.nodes.messages.deleteFailed', { message: e.message || e }))
  }
}

// Auth key actions
const generateKey = async () => {
  generatingKey.value = true
  try {
    const res = await generateAuthKey({ name: newKeyRemark.value })
    newKeyRemark.value = ''
    if (res.data && res.data.key) {
      generatedKey.value = res.data.key
    }
    loadAuthKeys()
  } catch (e) {
    alert(t('admin.nodes.messages.generateFailed', { message: e.message || e }))
  } finally {
    generatingKey.value = false
  }
}

const removeAuthKey = async (key) => {
  if (!confirm(t('admin.nodes.messages.deleteAuthKeyConfirm'))) return
  try {
    await deleteAuthKey(key.id)
    loadAuthKeys()
  } catch (e) {
    alert(t('admin.nodes.messages.deleteFailed', { message: e.message || e }))
  }
}

const copyKey = async (key) => {
  try {
    await navigator.clipboard.writeText(key)
    alert(t('admin.nodes.messages.copied'))
  } catch (e) {
    alert(t('admin.nodes.messages.copyFailed', { message: e.message || e }))
  }
}

const copyGeneratedKey = async () => {
  await copyKey(generatedKey.value)
  generatedKey.value = ''
}

// Pagination
const changePage = (page) => {
  pagination.page = page
  loadNodes()
}

// Helpers
const getStatusClass = (status) => {
  const classes = ['status-pending', 'status-online', 'status-offline', 'status-disabled']
  return classes[status] || 'status-pending'
}

const getStatusText = (status) => {
  const keys = ['pending', 'online', 'offline', 'disabled']
  const key = keys[status]
  return key ? t(`admin.nodes.statusText.${key}`) : t('admin.nodes.statusText.unknown')
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
  if (diff < 60) return t('admin.nodes.relativeTime.justNow')
  if (diff < 3600) return t('admin.nodes.relativeTime.minutesAgo', { count: Math.floor(diff / 60) })
  if (diff < 86400) return t('admin.nodes.relativeTime.hoursAgo', { count: Math.floor(diff / 3600) })
  return formatDateTime(timestamp)
}

const formatDate = (timestamp) => {
  if (!timestamp) return '-'
  return formatDateTime(timestamp)
}

// Init
onMounted(async () => {
  const nodesLoaded = await loadNodes()
  if (!nodesLoaded) {
    return
  }
  await Promise.all([
    loadStats(),
    loadProtocolTemplates(),
    loadAuthKeys()
  ])
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

/* Stats cards */
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

/* Table container */
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

/* Status badges */
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

/* Buttons */
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

/* Pagination */
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

/* Modal */
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

/* Form styles */
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

/* Protocol templates */
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

/* Tabs */
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

/* Responsive */
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

