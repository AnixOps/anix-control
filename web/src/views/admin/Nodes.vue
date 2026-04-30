<template>
  <div class="page nodes-page">
    <div class="page-header">
      <h1>{{ t('admin.nodes.title') }}</h1>
      <div class="header-actions">
        <button class="btn btn-secondary" @click="openAuthKeyModal">
          {{ t('admin.nodes.actions.authKey') }}
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
              <code>{{ node.address }}</code>
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

    <!-- Auth Key modal -->
    <div class="modal-overlay" v-if="showAuthKeyModal" @click.self="closeAuthKeyModal">
      <div class="modal">
        <div class="modal-header">
          <h3>{{ t('admin.nodes.authKeyModal.title') }}</h3>
          <button class="close-btn" :title="t('common.actions.close')" :aria-label="t('common.actions.close')" @click="closeAuthKeyModal">×</button>
        </div>
        <div class="modal-body">
          <p class="auth-key-hint">{{ t('admin.nodes.authKeyModal.hint') }}</p>
          <div class="auth-key-display">
            <code class="auth-key-value">{{ authKey || t('admin.nodes.authKeyModal.noKey') }}</code>
            <button class="btn btn-sm" @click="copyAuthKey" :disabled="!authKey">
              {{ t('admin.nodes.authKeyModal.copy') }}
            </button>
          </div>
          <div class="auth-key-usage" v-if="authKeyUsed > 0">
            {{ t('admin.nodes.authKeyModal.registeredCount', { count: authKeyUsed }) }}
          </div>
          <div class="auth-key-config">
            <label>{{ t('admin.nodes.authKeyModal.configHint') }}</label>
            <pre class="config-block">{{ configSnippet }}</pre>
            <button class="btn btn-sm" @click="copyConfig">{{ t('admin.nodes.authKeyModal.copyConfig') }}</button>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary" @click="closeAuthKeyModal">{{ t('common.actions.close') }}</button>
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

    <!-- Create/edit protocol modal (JSON-first) -->
    <div class="modal-overlay" v-if="showProtocolFormModal" @click.self="closeProtocolFormModal">
      <div class="modal modal-lg">
        <div class="modal-header">
          <h3>{{ editingProtocol ? t('admin.nodes.protocolForm.titleEdit') : t('admin.nodes.protocolForm.titleCreate') }}</h3>
          <button class="close-btn" :title="t('common.actions.close')" :aria-label="t('common.actions.close')" @click="closeProtocolFormModal">×</button>
        </div>
        <div class="modal-body">
          <!-- Template quick-select (only when creating) -->
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

          <!-- Mode tabs -->
          <div class="tabs">
            <button :class="['tab-btn', protocolForm.mode === 'json' ? 'active' : '']" @click="protocolForm.mode = 'json'">
              JSON
            </button>
            <button :class="['tab-btn', protocolForm.mode === 'visual' ? 'active' : '']" @click="protocolForm.mode = 'visual'">
              {{ t('admin.nodes.protocolForm.tabs.visual') }}
            </button>
          </div>

          <!-- JSON mode (primary) -->
          <div v-if="protocolForm.mode === 'json'" class="protocol-editor">
            <div class="json-editor-actions">
              <button class="btn btn-sm btn-secondary" @click="formatJson" :disabled="!jsonValid">
                {{ t('admin.nodes.protocolForm.jsonActions.format') }}
              </button>
              <button class="btn btn-sm btn-secondary" @click="copyJson">
                {{ t('admin.nodes.protocolForm.jsonActions.copy') }}
              </button>
              <button class="btn btn-sm btn-secondary" @click="loadTemplateAsJson">
                {{ t('admin.nodes.protocolForm.jsonActions.fromTemplate') }}
              </button>
              <span :class="['json-status', jsonValid ? 'valid' : 'invalid']">
                {{ jsonValid ? t('admin.nodes.protocolForm.jsonStatus.valid') : t('admin.nodes.protocolForm.jsonStatus.invalid') }}
              </span>
            </div>
            <textarea
              v-model="jsonEditorContent"
              class="json-textarea"
              rows="22"
              spellcheck="false"
              :placeholder="jsonPlaceholder"
              @input="onJsonInput"
            ></textarea>
          </div>

          <!-- Visual mode (helper) -->
          <div v-else class="protocol-editor">
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

            <div class="form-row">
              <div class="form-group">
                <label class="checkbox-label">
                  <input type="checkbox" v-model="protocolForm.enable" :true-value="1" :false-value="0" />
                  <span>{{ t('admin.nodes.protocolForm.enable') }}</span>
                </label>
              </div>
              <div class="form-group">
                <label class="checkbox-label">
                  <input type="checkbox" v-model="protocolForm.show" :true-value="1" :false-value="0" />
                  <span>{{ t('admin.nodes.protocolForm.show') }}</span>
                </label>
              </div>
            </div>

            <!-- JSON sub-editors for advanced fields -->
            <details class="advanced-details">
              <summary>{{ t('admin.nodes.protocolForm.fields.settings') }}</summary>
              <textarea
                v-model="protocolForm.settings"
                class="json-textarea"
                rows="4"
                spellcheck="false"
                placeholder='{}'
              ></textarea>
            </details>

            <details class="advanced-details" v-if="protocolForm.tls > 0">
              <summary>{{ t('admin.nodes.protocolForm.fields.tlsSettings') }}</summary>
              <textarea
                v-model="protocolForm.tls_settings"
                class="json-textarea"
                rows="4"
                spellcheck="false"
                placeholder='{}'
              ></textarea>
            </details>

            <details class="advanced-details" v-if="protocolForm.tls === 2">
              <summary>{{ t('admin.nodes.protocolForm.fields.realitySettings') }}</summary>
              <textarea
                v-model="protocolForm.reality_settings"
                class="json-textarea"
                rows="4"
                spellcheck="false"
                placeholder='{}'
              ></textarea>
            </details>

            <details class="advanced-details" v-if="protocolForm.transport !== 'tcp'">
              <summary>{{ t('admin.nodes.protocolForm.fields.transportSettings') }}</summary>
              <textarea
                v-model="protocolForm.transport_settings"
                class="json-textarea"
                rows="4"
                spellcheck="false"
                placeholder='{}'
              ></textarea>
            </details>
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
  </div>
</template>

<script setup>
import { ref, reactive, computed, watch, onMounted } from 'vue'
import {
  getNodes, getNodeStats, createNode, updateNode, deleteNode,
  getNodeProtocols, createNodeProtocol, updateNodeProtocol, deleteNodeProtocol,
  getProtocolTemplates,
  getAuthKeys
} from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'

const { t, formatDateTime } = useAppI18n()

// State
const loading = ref(false)
const saving = ref(false)
const savingProtocol = ref(false)

const nodes = ref([])
const stats = reactive({ total: 0, online: 0, offline: 0, pending: 0 })
const pagination = reactive({ page: 1, size: 20, total: 0 })

const showNodeModal = ref(false)
const editingNode = ref(null)
const nodeForm = reactive({
  name: '',
  address: '',
  tags: '',
  rate: 1.0,
  sort: 0,
  status: 0
})

const showAuthKeyModal = ref(false)
const authKey = ref('')
const authKeyUsed = ref(0)

const showProtocolModal = ref(false)
const selectedNode = ref(null)
const protocols = ref([])

const showProtocolFormModal = ref(false)
const editingProtocol = ref(null)
const protocolForm = reactive({
  mode: 'json', // json | visual
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

// JSON editor state
const jsonEditorContent = ref('')
const jsonParseError = ref('')
const jsonValid = computed(() => jsonParseError.value === '')

const jsonPlaceholder = `{
  "type": "vless",
  "port": 443,
  "tls": 0,
  "transport": "tcp",
  "enable": 1,
  "show": 1,
  "settings": {},
  "tls_settings": {},
  "transport_settings": {},
  "reality_settings": {}
}`

// Convert visual form to JSON object
function visualToJson() {
  const obj = {
    type: protocolForm.type,
    port: protocolForm.port,
    tls: protocolForm.tls,
    transport: protocolForm.transport,
    enable: protocolForm.enable,
    show: protocolForm.show,
  }
  try { obj.settings = JSON.parse(protocolForm.settings || '{}') } catch { obj.settings = {} }
  try { obj.tls_settings = JSON.parse(protocolForm.tls_settings || '{}') } catch { obj.tls_settings = {} }
  try { obj.transport_settings = JSON.parse(protocolForm.transport_settings || '{}') } catch { obj.transport_settings = {} }
  try { obj.reality_settings = JSON.parse(protocolForm.reality_settings || '{}') } catch { obj.reality_settings = {} }
  return obj
}

// Convert JSON object to visual form fields
function jsonToVisual(json) {
  protocolForm.type = json.type || 'vless'
  protocolForm.port = json.port || 443
  protocolForm.tls = json.tls ?? 0
  protocolForm.transport = json.transport || 'tcp'
  protocolForm.enable = json.enable ?? 1
  protocolForm.show = json.show ?? 1
  protocolForm.settings = json.settings ? (typeof json.settings === 'string' ? json.settings : JSON.stringify(json.settings, null, 2)) : '{}'
  protocolForm.tls_settings = json.tls_settings ? (typeof json.tls_settings === 'string' ? json.tls_settings : JSON.stringify(json.tls_settings, null, 2)) : '{}'
  protocolForm.transport_settings = json.transport_settings ? (typeof json.transport_settings === 'string' ? json.transport_settings : JSON.stringify(json.transport_settings, null, 2)) : '{}'
  protocolForm.reality_settings = json.reality_settings ? (typeof json.reality_settings === 'string' ? json.reality_settings : JSON.stringify(json.reality_settings, null, 2)) : '{}'
}

// When switching to JSON mode, sync from visual form
watch(() => protocolForm.mode, (newMode) => {
  if (newMode === 'json' && !editingProtocol.value) {
    const json = visualToJson()
    jsonEditorContent.value = JSON.stringify(json, null, 2)
    jsonParseError.value = ''
  }
})

// When switching to visual mode, sync from JSON
watch(() => protocolForm.mode, (newMode) => {
  if (newMode === 'visual') {
    try {
      const json = JSON.parse(jsonEditorContent.value)
      jsonToVisual(json)
    } catch {
      // keep existing visual values if JSON is invalid
    }
  }
})

function onJsonInput() {
  try {
    JSON.parse(jsonEditorContent.value)
    jsonParseError.value = ''
  } catch (e) {
    jsonParseError.value = e.message
  }
}

function formatJson() {
  try {
    const parsed = JSON.parse(jsonEditorContent.value)
    jsonEditorContent.value = JSON.stringify(parsed, null, 2)
    jsonParseError.value = ''
  } catch (e) {
    jsonParseError.value = e.message
  }
}

async function copyJson() {
  try {
    await navigator.clipboard.writeText(jsonEditorContent.value)
  } catch (e) {
    alert(t('admin.nodes.messages.copyFailed') + ': ' + (e.message || e))
  }
}

function loadTemplateAsJson() {
  if (protocolTemplates.value.length === 0) return
  // Show a simple prompt to pick template
  const names = protocolTemplates.value.map((tpl, i) => `${i + 1}. ${tpl.name}`).join('\n')
  const pick = prompt(`Select template number:\n${names}`)
  const idx = parseInt(pick) - 1
  if (isNaN(idx) || idx < 0 || idx >= protocolTemplates.value.length) return
  const tpl = protocolTemplates.value[idx]
  const json = {
    type: tpl.type,
    port: tpl.default_port,
    tls: tpl.tls || 0,
    transport: tpl.transport || 'tcp',
    enable: 1,
    show: 1,
  }
  try { json.settings = JSON.parse(tpl.settings || '{}') } catch { json.settings = {} }
  try { json.tls_settings = JSON.parse(tpl.tls_settings || '{}') } catch { json.tls_settings = {} }
  try { json.transport_settings = JSON.parse(tpl.transport_settings || '{}') } catch { json.transport_settings = {} }
  try { json.reality_settings = JSON.parse(tpl.reality_settings || '{}') } catch { json.reality_settings = {} }
  jsonEditorContent.value = JSON.stringify(json, null, 2)
  jsonParseError.value = ''
}

// Data loaders
const normalizeNode = (node) => ({
  ...node,
  address: node.address || node.host || '',
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

// Node actions
const openAuthKeyModal = async () => {
  try {
    const res = await getAuthKeys()
    const keys = res.data || []
    if (keys.length > 0) {
      const first = keys[0]
      authKey.value = first.key
      authKeyUsed.value = first.used || 0
    } else {
      authKey.value = ''
      authKeyUsed.value = 0
    }
  } catch (e) {
    console.error('Failed to load auth keys:', e)
    authKey.value = ''
    authKeyUsed.value = 0
  }
  showAuthKeyModal.value = true
}

const closeAuthKeyModal = () => {
  showAuthKeyModal.value = false
}

const copyAuthKey = async () => {
  if (!authKey.value) return
  try {
    await navigator.clipboard.writeText(authKey.value)
    alert(t('admin.nodes.messages.copied'))
  } catch {
    alert(t('admin.nodes.messages.copyFailed'))
  }
}

const copyConfig = async () => {
  try {
    await navigator.clipboard.writeText(configSnippet.value)
    alert(t('admin.nodes.messages.copied'))
  } catch {
    alert(t('admin.nodes.messages.copyFailed'))
  }
}

const configSnippet = computed(() => {
  const host = window.location.origin
  return `# V2bX config example
{
  "Nodes": [
    {
      "Type": "v2board",
      "ApiHost": "${host}",
      "AuthKey": "${authKey.value || '<your-auth-key>'}",
      "NodeID": 0,
      "AutoRegister": true,
      "Rate": 1.0
    }
  ]
}`
})

const openEditModal = (node) => {
  editingNode.value = node
  Object.assign(nodeForm, {
    name: node.name,
    address: node.address || node.host || '',
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
  if (!nodeForm.name || !nodeForm.address) {
    alert(t('admin.nodes.messages.requiredFields'))
    return
  }
  saving.value = true
  try {
    const payload = {
      name: nodeForm.name,
      host: nodeForm.address,
      tags: nodeForm.tags,
      rate: Number(nodeForm.rate),
      sort: Number(nodeForm.sort),
      status: Number(nodeForm.status)
    }
    await updateNode(editingNode.value.id, payload)
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
  selectedTemplate.value = ''
  Object.assign(protocolForm, {
    mode: 'json',
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
  jsonEditorContent.value = JSON.stringify({
    type: 'vless',
    port: 443,
    tls: 0,
    transport: 'tcp',
    enable: 1,
    show: 1,
    settings: {},
    tls_settings: {},
    transport_settings: {},
    reality_settings: {}
  }, null, 2)
  jsonParseError.value = ''
  showProtocolFormModal.value = true
}

const editProtocol = (protocol) => {
  editingProtocol.value = protocol

  // Parse existing protocol data safely
  const settings = protocol.settings || '{}'
  const tlsSettings = protocol.tls_settings || '{}'
  const transportSettings = protocol.transport_settings || '{}'
  const realitySettings = protocol.reality_settings || '{}'

  // Build complete JSON for the editor
  const json = {
    type: protocol.type || 'vless',
    port: protocol.port || 443,
    tls: protocol.tls ?? 0,
    transport: protocol.transport || 'tcp',
    enable: protocol.enable ?? 1,
    show: protocol.show ?? 1,
  }
  try { json.settings = typeof settings === 'string' ? JSON.parse(settings) : settings } catch { json.settings = {} }
  try { json.tls_settings = typeof tlsSettings === 'string' ? JSON.parse(tlsSettings) : tlsSettings } catch { json.tls_settings = {} }
  try { json.transport_settings = typeof transportSettings === 'string' ? JSON.parse(transportSettings) : transportSettings } catch { json.transport_settings = {} }
  try { json.reality_settings = typeof realitySettings === 'string' ? JSON.parse(realitySettings) : realitySettings } catch { json.reality_settings = {} }
  if (protocol.custom_config) {
    try { json.custom_config = typeof protocol.custom_config === 'string' ? JSON.parse(protocol.custom_config) : protocol.custom_config } catch { json.custom_config = {} }
  }

  // Sync to visual form
  jsonToVisual(json)

  // Set JSON editor content
  jsonEditorContent.value = JSON.stringify(json, null, 2)
  jsonParseError.value = ''

  // Start in JSON mode
  protocolForm.mode = 'json'
  showProtocolFormModal.value = true
}

const closeProtocolFormModal = () => {
  showProtocolFormModal.value = false
  editingProtocol.value = null
}

const applyTemplate = (tpl) => {
  selectedTemplate.value = tpl.name

  const json = {
    type: tpl.type,
    port: tpl.default_port,
    tls: tpl.tls || 0,
    transport: tpl.transport || 'tcp',
    enable: 1,
    show: 1,
  }
  try { json.settings = JSON.parse(tpl.settings || '{}') } catch { json.settings = {} }
  try { json.tls_settings = JSON.parse(tpl.tls_settings || '{}') } catch { json.tls_settings = {} }
  try { json.transport_settings = JSON.parse(tpl.transport_settings || '{}') } catch { json.transport_settings = {} }
  try { json.reality_settings = JSON.parse(tpl.reality_settings || '{}') } catch { json.reality_settings = {} }

  jsonEditorContent.value = JSON.stringify(json, null, 2)
  jsonParseError.value = ''

  // Also sync to visual
  jsonToVisual(json)
  protocolForm.mode = 'json'
}

const saveProtocol = async () => {
  let payload

  if (protocolForm.mode === 'json') {
    // Parse from JSON editor
    try {
      const json = JSON.parse(jsonEditorContent.value)
      payload = {
        type: json.type || 'vless',
        port: json.port || 443,
        enable: json.enable ?? 1,
        tls: json.tls ?? 0,
        transport: json.transport || 'tcp',
        settings: JSON.stringify(json.settings || {}),
        tls_settings: JSON.stringify(json.tls_settings || {}),
        transport_settings: JSON.stringify(json.transport_settings || {}),
        reality_settings: JSON.stringify(json.reality_settings || {}),
        show: json.show ?? 1,
      }
      if (json.custom_config) {
        payload.custom_config = typeof json.custom_config === 'string' ? json.custom_config : JSON.stringify(json.custom_config)
      }
    } catch (e) {
      alert(t('admin.nodes.messages.invalidJson') + ': ' + e.message)
      return
    }
  } else {
    // Parse from visual form
    if (!protocolForm.type || !protocolForm.port) {
      alert(t('admin.nodes.messages.requiredFields'))
      return
    }
    payload = {
      type: protocolForm.type,
      port: protocolForm.port,
      enable: protocolForm.enable,
      tls: protocolForm.tls,
      transport: protocolForm.transport,
      settings: protocolForm.settings,
      tls_settings: protocolForm.tls_settings,
      transport_settings: protocolForm.transport_settings,
      reality_settings: protocolForm.reality_settings,
      show: protocolForm.show,
    }
  }

  savingProtocol.value = true
  try {
    if (editingProtocol.value) {
      await updateNodeProtocol(selectedNode.value.id, editingProtocol.value.id, payload)
    } else {
      await createNodeProtocol(selectedNode.value.id, payload)
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

// Init
onMounted(async () => {
  const nodesLoaded = await loadNodes()
  if (!nodesLoaded) {
    return
  }
  await Promise.all([
    loadStats(),
    loadProtocolTemplates()
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

/* JSON editor */
.json-editor-actions {
  display: flex;
  gap: 8px;
  align-items: center;
  margin-bottom: 12px;
}

.json-textarea {
  font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', 'Consolas', monospace !important;
  font-size: 13px;
  line-height: 1.5;
  background: var(--bg-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: 14px;
  color: var(--text-color);
  width: 100%;
  resize: vertical;
  tab-size: 2;
}

.json-textarea:focus {
  border-color: var(--primary-color);
  outline: none;
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
}

.json-status {
  margin-left: auto;
  font-size: 12px;
  font-weight: 500;
  padding: 4px 10px;
  border-radius: var(--radius-sm);
}

.json-status.valid {
  background: rgba(34, 197, 94, 0.15);
  color: var(--success-color);
}

.json-status.invalid {
  background: rgba(239, 68, 68, 0.15);
  color: var(--error-color);
}

/* Advanced details */
.advanced-details {
  margin-bottom: 16px;
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  overflow: hidden;
}

.advanced-details summary {
  padding: 12px 16px;
  background: var(--bg-color);
  cursor: pointer;
  font-weight: 500;
  font-size: 13px;
  color: var(--text-secondary);
  user-select: none;
}

.advanced-details summary:hover {
  background: var(--surface-hover);
}

.advanced-details > .json-textarea {
  border: none;
  border-radius: 0;
  border-top: 1px solid var(--border-color);
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

/* Auth Key Modal */
.auth-key-hint {
  color: var(--text-secondary);
  margin-bottom: 16px;
  font-size: 14px;
}

.auth-key-display {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}

.auth-key-value {
  flex: 1;
  background: var(--bg-color);
  padding: 12px 16px;
  border-radius: var(--radius-md);
  font-family: var(--font-mono);
  font-size: 14px;
  word-break: break-all;
  border: 1px solid var(--border-color);
}

.auth-key-usage {
  color: var(--text-secondary);
  font-size: 13px;
  margin-bottom: 16px;
}

.auth-key-config {
  margin-top: 20px;
}

.auth-key-config label {
  display: block;
  color: var(--text-secondary);
  font-size: 13px;
  margin-bottom: 8px;
}

.config-block {
  background: var(--bg-color);
  padding: 16px;
  border-radius: var(--radius-md);
  font-family: var(--font-mono);
  font-size: 13px;
  white-space: pre-wrap;
  word-break: break-all;
  border: 1px solid var(--border-color);
  max-height: 300px;
  overflow-y: auto;
  margin-bottom: 12px;
}
</style>
