<template>
  <div class="subscriptions-page">
    <div class="page-header">
      <h1>{{ $t('admin.subscriptions.title') }}</h1>
      <p class="subtitle">{{ $t('admin.subscriptions.subtitle') }}</p>
    </div>

    <!-- 分组列表 -->
    <div class="groups-section">
      <div class="section-header">
        <h2>{{ $t('admin.subscriptions.groups') }}</h2>
        <button class="btn btn-primary" @click="showCreateGroupModal = true">
          <svg class="icon" viewBox="0 0 24 24" width="16" height="16" aria-hidden="true"><path fill="currentColor" d="M19 11h-6V5h-2v6H5v2h6v6h2v-6h6v-2z"/></svg>
          {{ $t('admin.subscriptions.createGroup') }}
        </button>
      </div>

      <div class="groups-grid">
        <div 
          v-for="group in groups" 
          :key="group.id" 
          class="group-card"
          :class="{ active: selectedGroup?.id === group.id }"
          @click="selectGroup(group)"
        >
          <div class="group-header">
            <h3>{{ group.name }}</h3>
            <span class="badge" :class="group.enable ? 'badge-success' : 'badge-secondary'">
              {{ group.enable ? $t('common.enabled') : $t('common.disabled') }}
            </span>
          </div>
          <p class="group-desc">{{ group.description || $t('admin.subscriptions.noDescription') }}</p>
          <div class="group-meta">
            <span>{{ $t('admin.subscriptions.priority') }}: {{ group.priority }}</span>
            <span>{{ $t('admin.subscriptions.templates') }}: {{ group.template_count || 0 }}</span>
          </div>
          <div class="group-actions">
            <button class="btn btn-sm btn-outline" @click.stop="copyGroupSubscription(group)">
              <svg class="icon" viewBox="0 0 24 24" width="16" height="16" aria-hidden="true"><path fill="currentColor" d="M16 1H4a2 2 0 0 0-2 2v12h2V3h12V1zM20 5H8a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7a2 2 0 0 0-2-2zm-1 15H9V8h10v12z"/></svg>
              {{ $t('admin.subscriptions.copySubscription') }}
            </button>
            <button class="btn btn-sm btn-outline" @click.stop="copyGroupCombined(group)">
              <svg class="icon" viewBox="0 0 24 24" width="16" height="16" aria-hidden="true"><path fill="currentColor" d="M16 1H4a2 2 0 0 0-2 2v12h2V3h12V1zM20 5H8a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7a2 2 0 0 0-2-2zm-1 15H9V8h10v12z"/></svg>
              {{ $t('admin.subscriptions.copyCombinedSubscription') }}
            </button>
            <button class="btn btn-sm btn-outline" @click.stop="editGroup(group)">
              <svg class="icon" viewBox="0 0 24 24" width="14" height="14" aria-hidden="true"><path fill="currentColor" d="M3 17.25V21h3.75L17.81 9.94l-3.75-3.75L3 17.25zM20.71 7.04a1 1 0 0 0 0-1.41l-2.34-2.34a1 1 0 0 0-1.41 0l-1.83 1.83 3.75 3.75 1.83-1.83z"/></svg>
            </button>
            <button class="btn btn-sm btn-outline btn-danger" @click.stop="deleteGroup(group)">
              <svg class="icon" viewBox="0 0 24 24" width="14" height="14" aria-hidden="true"><path fill="currentColor" d="M6 19a2 2 0 0 0 2 2h8a2 2 0 0 0 2-2V7H6v12zM19 4h-3.5l-1-1h-5l-1 1H5v2h14V4z"/></svg>
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- 模板列表 -->
    <div class="templates-section" v-if="selectedGroup">
      <div class="section-header">
        <h2>{{ $t('admin.subscriptions.templatesFor') }} {{ selectedGroup.name }}</h2>
        <div class="header-actions">
          <button class="btn btn-secondary" @click="previewSubscription">
              <svg class="icon" viewBox="0 0 24 24" width="16" height="16" aria-hidden="true"><path fill="currentColor" d="M12 5c-7 0-11 7-11 7s4 7 11 7 11-7 11-7-4-7-11-7zm0 12a5 5 0 1 1 0-10 5 5 0 0 1 0 10zM12 9a3 3 0 1 0 .001 6.001A3 3 0 0 0 12 9z"/></svg>
              {{ $t('admin.subscriptions.preview') }}
          </button>
            <button class="btn btn-primary" @click="showCreateTemplateModal = true">
            <svg class="icon" viewBox="0 0 24 24" width="16" height="16" aria-hidden="true"><path fill="currentColor" d="M19 11h-6V5h-2v6H5v2h6v6h2v-6h6v-2z"/></svg>
            {{ $t('admin.subscriptions.createTemplate') }}
          </button>
        </div>
      </div>

      <div class="templates-table">
        <table>
          <thead>
            <tr>
              <th>{{ $t('admin.subscriptions.nodeName') }}</th>
              <th>{{ $t('admin.subscriptions.protocol') }}</th>
              <th>{{ $t('admin.subscriptions.server') }}</th>
              <th>{{ $t('admin.subscriptions.port') }}</th>
              <th>{{ $t('admin.subscriptions.tls') }}</th>
              <th>{{ $t('admin.subscriptions.status') }}</th>
              <th>{{ $t('common.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="template in templates" :key="template.id">
              <td>{{ template.name }}</td>
              <td>
                <span class="protocol-badge" :class="'protocol-' + template.type">
                  {{ template.type.toUpperCase() }}
                </span>
              </td>
              <td>{{ template.server }}</td>
              <td>{{ template.port }}</td>
              <td>
                <span class="tls-badge" :class="getTLSClass(template.tls)">
                  {{ getTLSLabel(template.tls) }}
                </span>
              </td>
              <td>
                <label class="switch">
                  <input type="checkbox" :checked="template.enable === 1" @change="toggleTemplate(template)">
                  <span class="slider"></span>
                </label>
              </td>
              <td>
                <button class="btn btn-sm btn-outline" @click="copyTemplateLink(template)">
                  <svg class="icon" viewBox="0 0 24 24" width="14" height="14" aria-hidden="true"><path fill="currentColor" d="M10.59 13.41L9.17 12l4.24-4.24 1.41 1.41L10.59 13.41zM6 18h12v2H6v-2z"/></svg>
                </button>
                <button class="btn btn-sm btn-outline" @click="editTemplate(template)">
                  <svg class="icon" viewBox="0 0 24 24" width="14" height="14" aria-hidden="true"><path fill="currentColor" d="M3 17.25V21h3.75L17.81 9.94l-3.75-3.75L3 17.25zM20.71 7.04a1 1 0 0 0 0-1.41l-2.34-2.34a1 1 0 0 0-1.41 0l-1.83 1.83 3.75 3.75 1.83-1.83z"/></svg>
                </button>
                <button class="btn btn-sm btn-outline btn-danger" @click="deleteTemplate(template)">
                  <svg class="icon" viewBox="0 0 24 24" width="14" height="14" aria-hidden="true"><path fill="currentColor" d="M6 19a2 2 0 0 0 2 2h8a2 2 0 0 0 2-2V7H6v12zM19 4h-3.5l-1-1h-5l-1 1H5v2h14V4z"/></svg>
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 生产节点列表 (物理节点协议) -->
    <div class="templates-section" v-if="selectedGroup">
      <div class="section-header">
        <h2>{{ $t('admin.subscriptions.productionNodes') || '物理节点协议' }}</h2>
        <div class="header-actions">
           <button class="btn btn-secondary" @click="openManageProtocolsModal">
              <svg class="icon" viewBox="0 0 24 24" width="16" height="16" aria-hidden="true"><path fill="currentColor" d="M12 22C6.477 22 2 17.523 2 12S6.477 2 12 2s10 4.477 10 10-4.477 10-10 10zm0-2a8 8 0 1 0 0-16 8 8 0 0 0 0 16zm-1-7h2v2h-2v-2zm0-4h2v2h-2V9z"/></svg>
              管理关联
           </button>
           <span class="info-badge">已关联到此分组的协议</span>
        </div>
      </div>

      <div class="templates-table">
        <table v-if="protocols.length > 0">
          <thead>
            <tr>
              <th>节点</th>
              <th>协议</th>
              <th>名称</th>
              <th>端口</th>
              <th>可见</th>
              <th>状态</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="protocol in protocols" :key="protocol.id">
              <td>{{ protocol.node?.name || 'ID: ' + protocol.node_id }}</td>
              <td>
                <span class="protocol-badge" :class="'protocol-' + protocol.type">
                  {{ (protocol.type || '').toUpperCase() }}
                </span>
              </td>
              <td>{{ protocol.name }}</td>
              <td>{{ protocol.port }}</td>
              <td>
                <span :class="['status-badge', protocol.show ? 'status-online' : 'status-disabled']">
                  {{ protocol.show ? '显示' : '隐藏' }}
                </span>
              </td>
              <td>
                <span :class="['status-badge', protocol.enable ? 'status-online' : 'status-disabled']">
                  {{ protocol.enable ? '在线' : '下线' }}
                </span>
              </td>
              <td>
                <button class="btn btn-sm btn-outline" @click="goToNode(protocol.node_id)" title="前往管理">
                  <svg class="icon" viewBox="0 0 24 24" width="14" height="14" aria-hidden="true"><path fill="currentColor" d="M19 19H5V5h7V3H5c-1.11 0-2 .9-2 2v14c0 1.1.89 2 2 2h14c1.1 0 2-.9 2-2v-7h-2v7zM14 3v2h3.59l-9.83 9.83 1.41 1.41L19 6.41V10h2V3h-7z"/></svg>
                </button>
              </td>
            </tr>
          </tbody>
        </table>
        <div v-else class="empty-state">
          <p>此分组暂无关联的物理节点协议</p>
        </div>
      </div>
    </div>

    <!-- 管理关联协议弹窗 -->
    <div class="modal" v-if="showManageProtocolsModal" @click.self="showManageProtocolsModal = false">
      <div class="modal-content modal-lg">
        <div class="modal-header">
          <h3>管理物理节点协议关联</h3>
          <button class="close-btn" @click="showManageProtocolsModal = false">&times;</button>
        </div>
        <div class="modal-body">
          <p class="subtitle mb-4">勾选要包含在 <strong>{{ selectedGroup?.name }}</strong> 分组中的节点协议。只有在“节点管理”中开启了“显示在订阅中”的协议才会出现在此处。</p>
          
          <div class="protocol-pool-table">
            <table>
              <thead>
                <tr>
                  <th width="40"><input type="checkbox" @change="toggleAllAvailable" :checked="isAllSelected"></th>
                  <th>节点</th>
                  <th>协议/名称</th>
                  <th>端口</th>
                  <th>已关联分组</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="p in availableProtocols" :key="p.id" @click="toggleProtocolSelection(p.id)" class="clickable-row">
                  <td><input type="checkbox" :checked="selectedProtocolIds.includes(p.id)" @click.stop></td>
                  <td>{{ p.node?.name || '未知节点' }}</td>
                  <td>
                    <span class="protocol-badge" :class="'protocol-' + p.type">{{ p.type.toUpperCase() }}</span>
                    <span class="ml-2">{{ p.name }}</span>
                  </td>
                  <td>{{ p.port }}</td>
                  <td>
                    <div class="group-badges">
                      <span v-for="g in p.subscription_groups" :key="g.id" class="mini-badge">
                        {{ g.name }}
                      </span>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary" @click="showManageProtocolsModal = false">取消</button>
          <button class="btn btn-primary" @click="saveGroupProtocols">确认保存</button>
        </div>
      </div>
    </div>

    <!-- 订阅内容查看弹窗 -->
    <div class="modal" v-if="showPreviewModal" @click.self="showPreviewModal = false">
      <div class="modal-content modal-lg">
        <div class="modal-header">
          <h3>{{ $t('admin.subscriptions.previewTitle') }}</h3>
          <button class="close-btn" @click="showPreviewModal = false">&times;</button>
        </div>
        <div class="modal-body">
          <div class="preview-format-selector">
            <label>{{ $t('admin.subscriptions.format') }}:</label>
            <select v-model="previewFormat" @change="loadPreview">
              <option value="v2ray">V2Ray (Base64)</option>
              <option value="clash">Clash (YAML)</option>
              <option value="json">JSON</option>
              <option value="base64json">Base64 JSON</option>
            </select>
          </div>
          <div class="preview-content">
            <pre>{{ previewContent }}</pre>
          </div>
          <div class="preview-actions">
            <button class="btn btn-primary" @click="copyPreviewContent">
              <svg class="icon" viewBox="0 0 24 24" width="16" height="16" aria-hidden="true"><path fill="currentColor" d="M16 1H4a2 2 0 0 0-2 2v12h2V3h12V1zM20 5H8a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7a2 2 0 0 0-2-2zm-1 15H9V8h10v12z"/></svg>
              {{ $t('admin.subscriptions.copyContent') }}
            </button>
            <button class="btn btn-secondary" @click="downloadPreview">
              <svg class="icon" viewBox="0 0 24 24" width="16" height="16" aria-hidden="true"><path fill="currentColor" d="M5 20h14v-2H5v2zm7-18L5.33 9h3.67v6h6V9h3.67L12 2z"/></svg>
              {{ $t('admin.subscriptions.download') }}
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- 订阅链接弹窗 -->
    <div class="modal" v-if="showSubscriptionModal" @click.self="showSubscriptionModal = false">
      <div class="modal-content">
        <div class="modal-header">
          <h3>{{ $t('admin.subscriptions.subscriptionLinks') }}</h3>
          <button class="close-btn" @click="showSubscriptionModal = false">&times;</button>
        </div>
        <div class="modal-body">
          <div class="subscription-links">
            <div class="link-item" v-for="format in subscriptionFormats" :key="format.value">
              <label>{{ format.label }}:</label>
              <div class="link-input-group">
                <input type="text" readonly :value="getSubscriptionUrl(format.value)">
                <button class="btn btn-sm" @click="copyToClipboard(getSubscriptionUrl(format.value))">
                  <i class="icon-copy"></i>
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 创建/编辑分组弹窗 -->
    <div class="modal" v-if="showCreateGroupModal || showEditGroupModal" @click.self="closeGroupModal">
      <div class="modal-content">
        <div class="modal-header">
          <h3>{{ showEditGroupModal ? $t('admin.subscriptions.editGroup') : $t('admin.subscriptions.createGroup') }}</h3>
          <button class="close-btn" @click="closeGroupModal">&times;</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label>{{ $t('admin.subscriptions.groupName') }} *</label>
            <input type="text" v-model="groupForm.name" :placeholder="$t('admin.subscriptions.groupNamePlaceholder')">
          </div>
          <div class="form-group">
            <label>{{ $t('admin.subscriptions.description') }}</label>
            <textarea v-model="groupForm.description" rows="3"></textarea>
          </div>
          <div class="form-row">
            <div class="form-group">
              <label>{{ $t('admin.subscriptions.priority') }}</label>
              <input type="number" v-model.number="groupForm.priority">
            </div>
            <div class="form-group">
              <label>{{ $t('admin.subscriptions.enabled') }}</label>
              <label class="switch">
                <input type="checkbox" v-model="groupForm.enable">
                <span class="slider"></span>
              </label>
            </div>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary" @click="closeGroupModal">{{ $t('common.cancel') }}</button>
          <button class="btn btn-primary" @click="saveGroup">{{ $t('common.save') }}</button>
        </div>
      </div>
    </div>

    <!-- 创建/编辑模板弹窗 -->
    <div class="modal" v-if="showCreateTemplateModal || showEditTemplateModal" @click.self="closeTemplateModal">
      <div class="modal-content modal-lg">
        <div class="modal-header">
          <h3>{{ showEditTemplateModal ? $t('admin.subscriptions.editTemplate') : $t('admin.subscriptions.createTemplate') }}</h3>
          <button class="close-btn" @click="closeTemplateModal">&times;</button>
        </div>
        <div class="modal-body">
          <div class="form-row">
            <div class="form-group">
              <label>{{ $t('admin.subscriptions.nodeName') }} *</label>
              <input type="text" v-model="templateForm.name" placeholder="🇺🇸 美国节点">
            </div>
            <div class="form-group">
              <label>{{ $t('admin.subscriptions.protocol') }} *</label>
              <select v-model="templateForm.type">
                <option value="vless">VLESS</option>
                <option value="vmess">VMess</option>
                <option value="trojan">Trojan</option>
                <option value="shadowsocks">Shadowsocks</option>
                <option value="hysteria2">Hysteria2</option>
                <option value="tuic">TUIC</option>
              </select>
            </div>
          </div>
          <div class="form-row">
            <div class="form-group">
              <label>{{ $t('admin.subscriptions.server') }} *</label>
              <input type="text" v-model="templateForm.server" placeholder="us.example.com">
            </div>
            <div class="form-group">
              <label>{{ $t('admin.subscriptions.port') }} *</label>
              <input type="number" v-model.number="templateForm.port" placeholder="443">
            </div>
          </div>
          <div class="form-row">
            <div class="form-group">
              <label>{{ $t('admin.subscriptions.tls') }}</label>
              <select v-model.number="templateForm.tls">
                <option :value="0">{{ $t('admin.subscriptions.tlsNone') }}</option>
                <option :value="1">TLS</option>
                <option :value="2">Reality</option>
              </select>
            </div>
            <div class="form-group">
              <label>{{ $t('admin.subscriptions.transport') }}</label>
              <select v-model="templateForm.transport">
                <option value="tcp">TCP</option>
                <option value="ws">WebSocket</option>
                <option value="grpc">gRPC</option>
                <option value="h2">HTTP/2</option>
                <option value="quic">QUIC</option>
              </select>
            </div>
          </div>
          <div class="form-group" v-if="templateForm.tls > 0">
            <label>SNI (Server Name)</label>
            <input type="text" v-model="templateForm.server_name" placeholder="www.example.com">
          </div>
          <div class="form-row" v-if="templateForm.tls === 2">
            <div class="form-group">
              <label>Reality Public Key</label>
              <input type="text" v-model="templateForm.reality_public_key">
            </div>
            <div class="form-group">
              <label>Reality Short ID</label>
              <input type="text" v-model="templateForm.reality_short_id">
            </div>
          </div>
          <div class="form-group" v-if="templateForm.tls > 0">
            <label>TLS Fingerprint</label>
            <select v-model="templateForm.tls_fingerprint">
              <option value="">{{ $t('common.default') }}</option>
              <option value="chrome">Chrome</option>
              <option value="firefox">Firefox</option>
              <option value="safari">Safari</option>
              <option value="edge">Edge</option>
              <option value="random">Random</option>
            </select>
          </div>
          <div class="form-group" v-if="templateForm.transport === 'ws'">
            <label>WebSocket Path</label>
            <input type="text" v-model="wsPath" placeholder="/ws">
          </div>
          <div class="form-group" v-if="templateForm.type === 'vless' && templateForm.tls === 2">
            <label>Flow</label>
            <select v-model="vlessFlow">
              <option value="">None</option>
              <option value="xtls-rprx-vision">xtls-rprx-vision</option>
            </select>
          </div>
          <div class="form-group">
            <label>{{ $t('admin.subscriptions.enabled') }}</label>
            <label class="switch">
              <input type="checkbox" v-model="templateForm.enable">
              <span class="slider"></span>
            </label>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary" @click="closeTemplateModal">{{ $t('common.cancel') }}</button>
          <button class="btn btn-primary" @click="saveTemplate">{{ $t('common.save') }}</button>
        </div>
      </div>
    </div>

    <!-- Toast 消息 -->
    <div class="toast" v-if="toastMessage" :class="toastType">
      {{ toastMessage }}
    </div>
  </div>
</template>

<script>
import { ref, reactive, onMounted, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import adminApi from '@/api/admin'

export default {
  name: 'Subscriptions',
  setup() {
    const { t } = useI18n()
    
    // Data
    const groups = ref([])
    const templates = ref([])
    const protocols = ref([])
    const selectedGroup = ref(null)
    
    // Modals
    const showPreviewModal = ref(false)
    const showSubscriptionModal = ref(false)
    const showCreateGroupModal = ref(false)
    const showEditGroupModal = ref(false)
    const showCreateTemplateModal = ref(false)
    const showEditTemplateModal = ref(false)
    const showManageProtocolsModal = ref(false)
    
    // Protocol Pool Data
    const availableProtocols = ref([])
    const selectedProtocolIds = ref([])
    
    // Forms
    const groupForm = reactive({
      id: null,
      name: '',
      description: '',
      priority: 0,
      enable: true
    })
    
    const templateForm = reactive({
      id: null,
      name: '',
      type: 'vless',
      server: '',
      port: 443,
      server_name: '',
      tls: 1,
      tls_fingerprint: 'chrome',
      transport: 'tcp',
      reality_public_key: '',
      reality_short_id: '',
      enable: true
    })
    
    const wsPath = ref('/ws')
    const vlessFlow = ref('xtls-rprx-vision')
    // Preview
    const previewFormat = ref('v2ray')
    const previewContent = ref('')
    
    // Toast
    const toastMessage = ref('')
    const toastType = ref('success')
    
    // Subscription formats
    const subscriptionFormats = [
      { value: 'v2ray', label: 'V2Ray (Base64)' },
      { value: 'clash', label: 'Clash (YAML)' },
      { value: 'json', label: 'JSON' },
      { value: 'base64json', label: 'Base64 JSON' }
    ]
    
    // Methods
    const loadGroups = async () => {
      try {
        const res = await adminApi.getSubscriptionGroups()
        groups.value = res.data || []
        // 自动选中第一个分组
        if (groups.value.length > 0 && !selectedGroup.value) {
          selectGroup(groups.value[0])
        }
      } catch (error) {
        showToast(t('admin.subscriptions.loadError'), 'error')
      }
    }
    
    const loadTemplates = async (groupId) => {
      try {
        const res = await adminApi.getSubscriptionTemplates(groupId)
        templates.value = res.data || []
      } catch (error) {
        showToast(t('admin.subscriptions.loadError'), 'error')
      }
    }

    const loadProtocols = async (groupId) => {
      try {
        const res = await adminApi.getSubscriptionProtocols(groupId)
        protocols.value = res.data || []
      } catch (error) {
        console.error('Failed to load protocols:', error)
      }
    }

    const openManageProtocolsModal = async () => {
      if (!selectedGroup.value) return
      await loadAvailableProtocols()
      // 设置当前已选中的协议
      selectedProtocolIds.value = protocols.value.map(p => p.id)
      showManageProtocolsModal.value = true
    }

    const loadAvailableProtocols = async () => {
      try {
        const res = await adminApi.getAvailableProtocols()
        availableProtocols.value = res.data || []
      } catch (error) {
        showToast('获取可用协议失败', 'error')
      }
    }

    const toggleProtocolSelection = (id) => {
      const index = selectedProtocolIds.value.indexOf(id)
      if (index > -1) {
        selectedProtocolIds.value.splice(index, 1)
      } else {
        selectedProtocolIds.value.push(id)
      }
    }

    const isAllSelected = computed(() => {
      return availableProtocols.value.length > 0 && selectedProtocolIds.value.length === availableProtocols.value.length
    })

    const toggleAllAvailable = () => {
      if (isAllSelected.value) {
        selectedProtocolIds.value = []
      } else {
        selectedProtocolIds.value = availableProtocols.value.map(p => p.id)
      }
    }

    const saveGroupProtocols = async () => {
      try {
        await adminApi.updateGroupProtocols(selectedGroup.value.id, selectedProtocolIds.value)
        showToast('更新关联成功', 'success')
        showManageProtocolsModal.value = false
        loadProtocols(selectedGroup.value.id) // 重新加载当前分组的协议列表
      } catch (error) {
        showToast('更新失败', 'error')
      }
    }
    
    const selectGroup = (group) => {
      selectedGroup.value = group
      loadTemplates(group.id)
      loadProtocols(group.id)
    }
    
    const copyGroupSubscription = (group) => {
      selectedGroup.value = group
      showSubscriptionModal.value = true
    }

    // 复制分组合并订阅内容（服务端已按组合并并返回 base64 编码的内容）
    const copyGroupCombined = async (group) => {
      selectedGroup.value = group
      if (!confirm(t('admin.subscriptions.copyCombinedConfirm'))) return

      // 优先尝试后端生成合并结果
      try {
        const res = await adminApi.previewSubscription({ group_ids: [group.id], format: 'v2ray' })
        const content = res.data?.content || ''
        if (content) {
          await copyToClipboard(content)
          showToast(t('admin.subscriptions.copied'), 'success')
          return
        }
      } catch (e) {
        // 继续走前端回退逻辑
      }

      // 后端不可用或返回空，使用前端合并（仅基于模板，不包含内部节点）
      try {
        const tplRes = await adminApi.getSubscriptionTemplates(group.id)
        const tplList = tplRes.data || []
        const lines = []
        for (const tpl of tplList) {
          const link = generateNodeLink(tpl)
          if (link) lines.push(link)
        }
        if (lines.length === 0) {
          showToast(t('admin.subscriptions.copyError'), 'error')
          return
        }
        const combined = lines.join('\n')
        const encoded = btoa(combined)
        await copyToClipboard(encoded)
        showToast(t('admin.subscriptions.copyFallbackNotice'), 'success')
      } catch (e) {
        showToast(t('admin.subscriptions.copyError'), 'error')
      }
    }
    
    const getSubscriptionUrl = (format) => {
      // 使用测试用户 token (管理员预览用)
      const baseUrl = window.location.origin
      return `${baseUrl}/s/admin-preview?type=${format}&groups=${selectedGroup.value?.id || ''}`
    }
    
    const copyToClipboard = async (text) => {
      try {
        await navigator.clipboard.writeText(text)
        showToast(t('admin.subscriptions.copied'), 'success')
      } catch (error) {
        showToast(t('admin.subscriptions.copyError'), 'error')
      }
    }
    
    const previewSubscription = async () => {
      showPreviewModal.value = true
      await loadPreview()
    }
    
    const loadPreview = async () => {
      try {
        const res = await adminApi.previewSubscription({
          group_ids: [selectedGroup.value.id],
          format: previewFormat.value
        })
        previewContent.value = res.data?.content || ''
      } catch (error) {
        previewContent.value = t('admin.subscriptions.previewError')
      }
    }
    
    const copyPreviewContent = () => {
      copyToClipboard(previewContent.value)
    }
    
    const downloadPreview = () => {
      const ext = previewFormat.value === 'clash' ? 'yaml' : 
                  previewFormat.value === 'json' ? 'json' : 'txt'
      const blob = new Blob([previewContent.value], { type: 'text/plain' })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `subscription.${ext}`
      a.click()
      URL.revokeObjectURL(url)
    }
    
    const editGroup = (group) => {
      Object.assign(groupForm, {
        id: group.id,
        name: group.name,
        description: group.description || '',
        priority: group.priority,
        enable: group.enable === 1
      })
      showEditGroupModal.value = true
    }
    
    const deleteGroup = async (group) => {
      if (!confirm(t('admin.subscriptions.confirmDeleteGroup'))) return
      try {
        await adminApi.deleteSubscriptionGroup(group.id)
        showToast(t('admin.subscriptions.groupDeleted'), 'success')
        loadGroups()
        if (selectedGroup.value?.id === group.id) {
          selectedGroup.value = null
          templates.value = []
        }
      } catch (error) {
        showToast(t('admin.subscriptions.deleteError'), 'error')
      }
    }
    
    const closeGroupModal = () => {
      showCreateGroupModal.value = false
      showEditGroupModal.value = false
      Object.assign(groupForm, { id: null, name: '', description: '', priority: 0, enable: true })
    }
    
    const saveGroup = async () => {
      try {
        const data = {
          name: groupForm.name,
          description: groupForm.description,
          priority: groupForm.priority,
          enable: groupForm.enable ? 1 : 0
        }
        
        if (groupForm.id) {
          await adminApi.updateSubscriptionGroup(groupForm.id, data)
        } else {
          await adminApi.createSubscriptionGroup(data)
        }
        
        showToast(t('admin.subscriptions.groupSaved'), 'success')
        closeGroupModal()
        loadGroups()
      } catch (error) {
        showToast(t('admin.subscriptions.saveError'), 'error')
      }
    }
    
    const editTemplate = (template) => {
      Object.assign(templateForm, {
        id: template.id,
        name: template.name,
        type: template.type,
        server: template.server,
        port: template.port,
        server_name: template.server_name || '',
        tls: template.tls,
        tls_fingerprint: template.tls_fingerprint || '',
        transport: template.transport || 'tcp',
        reality_public_key: template.reality_public_key || '',
        reality_short_id: template.reality_short_id || '',
        enable: template.enable === 1
      })
      
      // 解析 transport_settings
      if (template.transport_settings) {
        try {
          const settings = JSON.parse(template.transport_settings)
          wsPath.value = settings.path || '/ws'
        } catch (e) {}
      }
      
      // 解析 protocol_settings
      if (template.protocol_settings) {
        try {
          const settings = JSON.parse(template.protocol_settings)
          vlessFlow.value = settings.flow || ''
        } catch (e) {}
      }
      
      showEditTemplateModal.value = true
    }
    
    const deleteTemplate = async (template) => {
      if (!confirm(t('admin.subscriptions.confirmDeleteTemplate'))) return
      try {
        await adminApi.deleteSubscriptionTemplate(template.id)
        showToast(t('admin.subscriptions.templateDeleted'), 'success')
        loadTemplates(selectedGroup.value.id)
      } catch (error) {
        showToast(t('admin.subscriptions.deleteError'), 'error')
      }
    }
    
    const toggleTemplate = async (template) => {
      try {
        await adminApi.updateSubscriptionTemplate(template.id, {
          enable: template.enable === 1 ? 0 : 1
        })
        loadTemplates(selectedGroup.value.id)
      } catch (error) {
        showToast(t('admin.subscriptions.updateError'), 'error')
      }
    }
    
    const closeTemplateModal = () => {
      showCreateTemplateModal.value = false
      showEditTemplateModal.value = false
      Object.assign(templateForm, {
        id: null, name: '', type: 'vless', server: '', port: 443,
        server_name: '', tls: 1, tls_fingerprint: 'chrome', transport: 'tcp',
        reality_public_key: '', reality_short_id: '', enable: true
      })
      wsPath.value = '/ws'
      vlessFlow.value = 'xtls-rprx-vision'
    }
    
    const saveTemplate = async () => {
      try {
        const data = {
          name: templateForm.name,
          type: templateForm.type,
          server: templateForm.server,
          port: templateForm.port,
          server_name: templateForm.server_name || null,
          tls: templateForm.tls,
          tls_fingerprint: templateForm.tls_fingerprint || null,
          transport: templateForm.transport,
          enable: templateForm.enable ? 1 : 0
        }
        
        // Reality settings
        if (templateForm.tls === 2) {
          data.reality_public_key = templateForm.reality_public_key
          data.reality_short_id = templateForm.reality_short_id
        }
        
        // Transport settings
        if (templateForm.transport === 'ws') {
          data.transport_settings = JSON.stringify({ path: wsPath.value, host: templateForm.server_name || templateForm.server })
        }
        
        // Protocol settings
        if (templateForm.type === 'vless' && vlessFlow.value) {
          data.protocol_settings = JSON.stringify({ flow: vlessFlow.value })
        }
        
        if (templateForm.id) {
          await adminApi.updateSubscriptionTemplate(templateForm.id, data)
        } else {
          await adminApi.createSubscriptionTemplate(selectedGroup.value.id, data)
        }
        
        showToast(t('admin.subscriptions.templateSaved'), 'success')
        closeTemplateModal()
        loadTemplates(selectedGroup.value.id)
      } catch (error) {
        showToast(t('admin.subscriptions.saveError'), 'error')
      }
    }
    
    const copyTemplateLink = async (template) => {
      // 生成单个节点的链接
      const link = generateNodeLink(template)
      copyToClipboard(link)
    }
    
    const generateNodeLink = (template) => {
      const uuid = 'test-uuid-1234-5678-abcdef'
      
      if (template.type === 'vless') {
        let params = [`encryption=none`]
        
        if (template.tls === 2) {
          params.push('security=reality')
          if (template.reality_public_key) params.push(`pbk=${template.reality_public_key}`)
          if (template.reality_short_id) params.push(`sid=${template.reality_short_id}`)
          if (template.server_name) params.push(`sni=${template.server_name}`)
          if (template.tls_fingerprint) params.push(`fp=${template.tls_fingerprint}`)
        } else if (template.tls === 1) {
          params.push('security=tls')
          if (template.server_name) params.push(`sni=${template.server_name}`)
        } else {
          params.push('security=none')
        }
        
        params.push(`type=${template.transport || 'tcp'}`)
        
        if (template.protocol_settings) {
          try {
            const settings = JSON.parse(template.protocol_settings)
            if (settings.flow) params.push(`flow=${settings.flow}`)
          } catch (e) {}
        }
        
        if (template.transport === 'tcp') {
          params.push('headerType=none')
        }
        
        return `vless://${uuid}@${template.server}:${template.port}?${params.join('&')}#${encodeURIComponent(template.name)}`
      }
      
      if (template.type === 'vmess') {
        const config = {
          v: '2',
          ps: template.name,
          add: template.server,
          port: template.port,
          id: uuid,
          aid: 0,
          scy: 'auto',
          net: template.transport || 'tcp',
          type: 'none',
          tls: template.tls === 1 ? 'tls' : ''
        }
        return `vmess://${btoa(JSON.stringify(config))}`
      }
      
      if (template.type === 'trojan') {
        let params = []
        if (template.server_name) params.push(`sni=${template.server_name}`)
        params.push(`type=${template.transport || 'tcp'}`)
        const paramStr = params.length ? `?${params.join('&')}` : ''
        return `trojan://${uuid}@${template.server}:${template.port}${paramStr}#${encodeURIComponent(template.name)}`
      }
      
      if (template.type === 'hysteria2' || template.type === 'hy2') {
        let params = []
        if (template.server_name) params.push(`sni=${template.server_name}`)
        const paramStr = params.length ? `?${params.join('&')}` : ''
        return `hy2://${uuid}@${template.server}:${template.port}${paramStr}#${encodeURIComponent(template.name)}`
      }
      
      return ''
    }
    
    const getTLSClass = (tls) => {
      return tls === 2 ? 'tls-reality' : tls === 1 ? 'tls-enabled' : 'tls-none'
    }
    
    const getTLSLabel = (tls) => {
      return tls === 2 ? 'Reality' : tls === 1 ? 'TLS' : t('admin.subscriptions.tlsNone')
    }
    
    const showToast = (message, type = 'success') => {
      toastMessage.value = message
      toastType.value = type
      setTimeout(() => {
        toastMessage.value = ''
      }, 3000)
    }
    
    const goToNode = (nodeId) => {
      window.location.hash = `#/admin/nodes?id=${nodeId}`
    }

    // Lifecycle
    onMounted(() => {
      loadGroups()
    })
    
    return {
      groups,
      templates,
      protocols,
      selectedGroup,
      showPreviewModal,
      showSubscriptionModal,
      showCreateGroupModal,
      showEditGroupModal,
      showCreateTemplateModal,
      showEditTemplateModal,
      showManageProtocolsModal,
      availableProtocols,
      selectedProtocolIds,
      isAllSelected,
      groupForm,
      templateForm,
      wsPath,
      vlessFlow,
      previewFormat,
      previewContent,
      subscriptionFormats,
      toastMessage,
      toastType,
      loadGroups,
      selectGroup,
      copyGroupSubscription,
      getSubscriptionUrl,
      copyToClipboard,
      previewSubscription,
      loadPreview,
      copyPreviewContent,
      downloadPreview,
      editGroup,
      deleteGroup,
      closeGroupModal,
      saveGroup,
      editTemplate,
      deleteTemplate,
      toggleTemplate,
      closeTemplateModal,
      saveTemplate,
      copyTemplateLink,
      copyGroupCombined,
      getTLSClass,
      getTLSLabel,
      goToNode,
      openManageProtocolsModal,
      toggleProtocolSelection,
      toggleAllAvailable,
      saveGroupProtocols
    }
  }
}
</script>

<style scoped>
.subscriptions-page { padding: 24px; }

.page-header { margin-bottom: 24px; }
.page-header h1 { margin: 0; font-size: 24px; }
.subtitle { color: var(--text-secondary); margin-top: 8px; }

.section-header { display:flex; justify-content:space-between; align-items:center; margin-bottom:16px; }
.section-header h2 { margin:0; font-size:18px; }
.header-actions { display:flex; gap:8px; }

.groups-grid { display:grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap:16px; margin-bottom:32px; }

.group-card {
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: 16px;
  cursor: pointer;
  transition: var(--transition);
}
.group-card:hover { border-color: var(--primary-color); box-shadow: var(--shadow-sm); }
.group-card.active { border-color: var(--primary-color); background: color-mix(in srgb, var(--surface-color) 80%, var(--primary-color) 8%); }

.group-header { display:flex; justify-content:space-between; align-items:center; margin-bottom:8px; }
.group-header h3 { margin:0; font-size:16px; color: var(--text-color); }

.badge { padding:2px 8px; border-radius:4px; font-size:12px; }
.badge-success { background: rgba(34,197,94,0.12); color: var(--success-color); }
.badge-secondary { background: rgba(255,255,255,0.02); color: var(--text-secondary); }

.group-desc { color: var(--text-secondary); font-size:14px; margin-bottom:8px; }
.group-meta { display:flex; gap:16px; font-size:12px; color: var(--text-secondary); margin-bottom:12px; }
.group-actions { display:flex; gap:8px; }

.templates-section { background: var(--surface-color); border:1px solid var(--border-color); border-radius: var(--radius-md); padding:16px; margin-bottom: 24px; }
.info-badge { font-size: 12px; color: var(--text-secondary); background: rgba(255,255,255,0.04); padding: 4px 12px; border-radius: 20px; }
.status-badge { padding: 2px 8px; border-radius: 4px; font-size: 12px; }
.status-online { background: rgba(34,197,94,0.1); color: var(--success-color); }
.status-offline { background: rgba(244,63,94,0.1); color: var(--error-color); }
.status-disabled { background: rgba(255,255,255,0.05); color: var(--text-secondary); }
.templates-table { overflow-x:auto; }
.templates-table table { width:100%; border-collapse:collapse; }
.templates-table th, .templates-table td { padding:12px; text-align:left; border-bottom:1px solid var(--border-color); }
.templates-table th { background: rgba(255,255,255,0.02); font-weight:500; color: var(--text-secondary); }

.protocol-badge { padding:2px 8px; border-radius:4px; font-size:12px; font-weight:500; }
.protocol-vless { background: rgba(59,130,246,0.08); color: var(--primary-color); }
.protocol-vmess { background: rgba(124,58,237,0.06); color: #7b1fa2; }
.protocol-trojan { background: rgba(245,158,11,0.06); color: #e65100; }
.protocol-shadowsocks { background: rgba(34,197,94,0.06); color: var(--success-color); }
.protocol-hysteria2 { background: rgba(194,24,91,0.06); color: #c2185b; }
.protocol-tuic { background: rgba(0,131,143,0.06); color: #00838f; }

.tls-badge { padding:2px 8px; border-radius:4px; font-size:12px; }
.tls-reality { background: rgba(34,197,94,0.06); color: var(--success-color); }
.tls-enabled { background: rgba(59,130,246,0.06); color: var(--primary-color); }
.tls-none { background: rgba(244,63,94,0.06); color: #c62828; }

.switch { position:relative; display:inline-block; width:40px; height:20px; }
.switch input { opacity:0; width:0; height:0; }
.slider { position:absolute; cursor:pointer; top:0; left:0; right:0; bottom:0; background-color: rgba(255,255,255,0.06); transition:0.4s; border-radius:20px; }
.slider:before { position:absolute; content:""; height:16px; width:16px; left:2px; bottom:2px; background-color: var(--surface-color); transition:0.4s; border-radius:50%; }
input:checked + .slider { background-color: var(--primary-color); }
input:checked + .slider:before { transform: translateX(20px); }

/* Modal */
.modal { position:fixed; inset:0; background: rgba(0,0,0,0.6); display:flex; align-items:center; justify-content:center; z-index:1000; }
.modal-content { background: var(--surface-color); border-radius:8px; width:90%; max-width:700px; max-height:90vh; overflow-y:auto; border:1px solid var(--border-color); }
.modal-header { display:flex; justify-content:space-between; align-items:center; padding:16px; border-bottom:1px solid var(--border-color); }
.close-btn { background:none; border:none; font-size:24px; cursor:pointer; color: var(--text-secondary); }
.modal-body { padding:16px; }
.modal-footer { padding:16px; border-top:1px solid var(--border-color); display:flex; justify-content:flex-end; gap:8px; }

.form-group { margin-bottom:16px; }
.form-group label { display:block; margin-bottom:4px; font-weight:500; color: var(--text-color); }
.form-group input, .form-group select, .form-group textarea { width:100%; padding:8px 12px; background:var(--surface-color); border:1px solid var(--border-color); border-radius:4px; font-size:14px; color:var(--text-color); }
.form-row { display:flex; gap:16px; }
.form-row .form-group { flex:1; }

.preview-format-selector { margin-bottom:16px; display:flex; align-items:center; gap:8px; }
.preview-format-selector select { padding:8px 12px; border:1px solid var(--border-color); border-radius:4px; background:var(--surface-color); color:var(--text-color); }

.preview-content { background: rgba(255,255,255,0.02); border:1px solid var(--border-color); border-radius:4px; padding:16px; max-height:400px; overflow:auto; color: var(--text-color); }
.preview-content pre { margin:0; white-space:pre-wrap; word-break:break-all; font-size:12px; }
.preview-actions { margin-top:16px; display:flex; gap:8px; }

.subscription-links { display:flex; flex-direction:column; gap:12px; }
.link-item label { display:block; margin-bottom:4px; font-weight:500; color:var(--text-color); }
.link-input-group { display:flex; gap:8px; }
.link-input-group input { flex:1; padding:8px 12px; border:1px solid var(--border-color); border-radius:4px; font-size:12px; background:var(--surface-color); color:var(--text-color); }

/* Buttons */
.btn { padding:8px 16px; border:none; border-radius:4px; cursor:pointer; font-size:14px; display:inline-flex; align-items:center; gap:4px; }
.btn-primary { background:var(--primary-color); color:white; }
.btn-secondary { background: rgba(255,255,255,0.02); color: var(--text-color); border:1px solid var(--border-color); }
.btn-outline {
  background: rgba(255,255,255,0.02);
  border: 1px solid rgba(255,255,255,0.04);
  color: var(--text-color);
}
.btn-outline:hover { background: rgba(255,255,255,0.04); }

/* Ensure icon visibility and proper sizing for small icon-only buttons */
.btn-sm { padding:4px 8px; font-size:12px; min-width:36px; height:34px; display:inline-flex; align-items:center; justify-content:center; }
.btn-sm i { font-size:14px; color: var(--text-secondary); }
.btn-sm:hover i { color: var(--text-color); }
/* svg icon color */
.icon { color: var(--text-secondary); display:inline-block; vertical-align:middle; }
.btn:hover .icon { color: var(--text-color); }
.btn-danger { color: var(--error-color); }
.btn-sm { padding:4px 8px; font-size:12px; }

/* Toast */
.toast { position:fixed; bottom:24px; right:24px; padding:12px 24px; border-radius:4px; color:white; z-index:2000; }
.toast.error { background: var(--error-color); }

.modal-lg { max-width: 900px; }
.mb-4 { margin-bottom: 24px; }
.ml-2 { margin-left: 8px; }
.clickable-row { cursor: pointer; transition: background 0.2s; }
.clickable-row:hover { background: rgba(255,255,255,0.02); }

.protocol-pool-table { border: 1px solid var(--border-color); border-radius: 4px; overflow: hidden; }
.protocol-pool-table table { width: 100%; border-collapse: collapse; }
.protocol-pool-table th, .protocol-pool-table td { padding: 12px; text-align: left; border-bottom: 1px solid var(--border-color); }
.protocol-pool-table th { background: rgba(255,255,255,0.02); font-weight: 500; font-size: 13px; }

.group-badges { display: flex; flex-wrap: wrap; gap: 4px; }
.mini-badge { padding: 1px 6px; background: rgba(59,130,246,0.08); color: var(--primary-color); border-radius: 4px; font-size: 11px; }
</style>
