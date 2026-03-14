<template>
  <div class="notification-page">
    <div class="page-header">
      <h1>通知管理</h1>
      <p class="text-secondary">配置通知模板和邮件设置</p>
    </div>

    <!-- 标签切换 -->
    <div class="tabs">
      <button :class="['tab', { active: activeTab === 'templates' }]" @click="activeTab = 'templates'">
        通知模板
      </button>
      <button :class="['tab', { active: activeTab === 'email' }]" @click="activeTab = 'email'">
        邮件配置
      </button>
      <button :class="['tab', { active: activeTab === 'logs' }]" @click="activeTab = 'logs'">
        发送日志
      </button>
    </div>

    <!-- 通知模板 -->
    <div v-show="activeTab === 'templates'">
      <div class="toolbar">
        <button class="btn-primary" @click="openTemplateModal()">➕ 新增模板</button>
      </div>

      <div class="table-container">
        <table class="data-table">
          <thead>
            <tr>
              <th>ID</th>
              <th>名称</th>
              <th>类型</th>
              <th>触发事件</th>
              <th>状态</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="template in templates" :key="template.id">
              <td>{{ template.id }}</td>
              <td>{{ template.name }}</td>
              <td>
                <span :class="['type-badge', template.type]">
                  {{ getTypeLabel(template.type) }}
                </span>
              </td>
              <td>{{ getEventLabel(template.event) }}</td>
              <td>
                <span :class="['status-badge', template.enabled ? 'status-active' : 'status-disabled']">
                  {{ template.enabled ? '启用' : '禁用' }}
                </span>
              </td>
              <td>
                <div class="action-buttons">
                  <button class="btn-sm btn-ghost" @click="openTemplateModal(template)" title="编辑">✏️</button>
                  <button class="btn-sm btn-ghost" @click="deleteTemplate(template)" title="删除">🗑️</button>
                </div>
              </td>
            </tr>
            <tr v-if="templates.length === 0">
              <td colspan="6" class="empty-row">暂无模板数据</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 邮件配置 -->
    <div v-show="activeTab === 'email'">
      <div class="config-section">
        <h3>SMTP 配置</h3>
        <div class="form-row">
          <div class="form-group">
            <label>SMTP 服务器</label>
            <input v-model="emailConfig.host" type="text" placeholder="smtp.example.com" />
          </div>
          <div class="form-group">
            <label>端口</label>
            <input v-model.number="emailConfig.port" type="number" placeholder="465" />
          </div>
        </div>
        <div class="form-row">
          <div class="form-group">
            <label>用户名</label>
            <input v-model="emailConfig.username" type="text" placeholder="your@email.com" />
          </div>
          <div class="form-group">
            <label>密码</label>
            <input v-model="emailConfig.password" type="password" placeholder="••••••••" />
          </div>
        </div>
        <div class="form-row">
          <div class="form-group">
            <label>发件人名称</label>
            <input v-model="emailConfig.from_name" type="text" placeholder="V2Board" />
          </div>
          <div class="form-group">
            <label>发件人地址</label>
            <input v-model="emailConfig.from_address" type="email" placeholder="noreply@example.com" />
          </div>
        </div>
        <div class="form-group">
          <label class="checkbox-label">
            <input type="checkbox" v-model="emailConfig.encryption" />
            <span>启用 TLS 加密</span>
          </label>
        </div>
        <div class="form-actions">
          <button class="btn-primary" @click="saveEmailConfig">保存配置</button>
          <button class="btn-secondary" @click="openTestModal">发送测试邮件</button>
        </div>
      </div>
    </div>

    <!-- 发送日志 -->
    <div v-show="activeTab === 'logs'">
      <div class="toolbar">
        <select v-model="logFilter.type">
          <option value="">全部类型</option>
          <option value="email">邮件</option>
          <option value="telegram">Telegram</option>
          <option value="webhook">Webhook</option>
        </select>
        <select v-model="logFilter.status">
          <option value="">全部状态</option>
          <option value="success">成功</option>
          <option value="failed">失败</option>
        </select>
        <button class="btn-secondary" @click="fetchLogs">🔍 搜索</button>
      </div>

      <div class="table-container">
        <table class="data-table">
          <thead>
            <tr>
              <th>ID</th>
              <th>类型</th>
              <th>接收者</th>
              <th>标题</th>
              <th>状态</th>
              <th>发送时间</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="log in logs" :key="log.id">
              <td>{{ log.id }}</td>
              <td>
                <span :class="['type-badge', log.type]">{{ getTypeLabel(log.type) }}</span>
              </td>
              <td>{{ log.recipient }}</td>
              <td>{{ log.title || '-' }}</td>
              <td>
                <span :class="['status-badge', log.status === 'success' ? 'status-active' : 'status-failed']">
                  {{ log.status === 'success' ? '成功' : '失败' }}
                </span>
              </td>
              <td>{{ formatTime(log.created_at) }}</td>
            </tr>
            <tr v-if="logs.length === 0">
              <td colspan="6" class="empty-row">暂无日志数据</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 模板弹窗 -->
    <div v-if="showTemplateModal" class="modal-overlay" @click.self="showTemplateModal = false">
      <div class="modal">
        <div class="modal-header">
          <h3>{{ editingTemplate ? '编辑模板' : '新增模板' }}</h3>
          <button class="close-btn" @click="showTemplateModal = false">✕</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label>名称 <span class="required">*</span></label>
            <input v-model="templateForm.name" type="text" placeholder="模板名称" />
          </div>
          <div class="form-row">
            <div class="form-group">
              <label>类型</label>
              <select v-model="templateForm.type">
                <option value="email">邮件</option>
                <option value="telegram">Telegram</option>
                <option value="webhook">Webhook</option>
              </select>
            </div>
            <div class="form-group">
              <label>触发事件</label>
              <select v-model="templateForm.event">
                <option value="user.register">用户注册</option>
                <option value="user.login">用户登录</option>
                <option value="user.expire">用户到期</option>
                <option value="user.traffic_low">流量不足</option>
                <option value="order.paid">订单支付</option>
                <option value="ticket.reply">工单回复</option>
                <option value="node.offline">节点离线</option>
                <option value="node.online">节点上线</option>
              </select>
            </div>
          </div>
          <div class="form-group">
            <label>标题模板</label>
            <input v-model="templateForm.title" type="text" placeholder="支持变量: {username}, {site_name}" />
          </div>
          <div class="form-group">
            <label>内容模板</label>
            <textarea v-model="templateForm.content" rows="5" placeholder="支持变量: {username}, {email}, {expire_time}"></textarea>
          </div>
          <div class="form-group">
            <label class="checkbox-label">
              <input type="checkbox" v-model="templateForm.enabled" />
              <span>启用</span>
            </label>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn-secondary" @click="showTemplateModal = false">取消</button>
          <button @click="saveTemplate">保存</button>
        </div>
      </div>
    </div>

    <!-- 测试邮件弹窗 -->
    <div v-if="showTestModal" class="modal-overlay" @click.self="showTestModal = false">
      <div class="modal modal-sm">
        <div class="modal-header">
          <h3>发送测试邮件</h3>
          <button class="close-btn" @click="showTestModal = false">✕</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label>收件人地址</label>
            <input v-model="testEmail" type="email" placeholder="test@example.com" />
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn-secondary" @click="showTestModal = false">取消</button>
          <button @click="sendTestEmail">发送</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import {
  getNotificationTemplates, createNotificationTemplate, updateNotificationTemplate,
  deleteNotificationTemplate, getNotificationLogs, sendTestNotification,
  getEmailConfig, updateEmailConfig
} from '@/api/admin'

const activeTab = ref('templates')
const templates = ref([])
const logs = ref([])

const logFilter = ref({ type: '', status: '' })

const showTemplateModal = ref(false)
const editingTemplate = ref(null)
const templateForm = ref({
  name: '', type: 'email', event: 'user.register', title: '', content: '', enabled: true
})

const showTestModal = ref(false)
const testEmail = ref('')

const emailConfig = ref({
  host: '', port: 465, username: '', password: '',
  from_name: '', from_address: '', encryption: true
})

const typeLabels = {
  email: '邮件',
  telegram: 'Telegram',
  webhook: 'Webhook'
}

const eventLabels = {
  'user.register': '用户注册',
  'user.login': '用户登录',
  'user.expire': '用户到期',
  'user.traffic_low': '流量不足',
  'order.paid': '订单支付',
  'ticket.reply': '工单回复',
  'node.offline': '节点离线',
  'node.online': '节点上线'
}

const getTypeLabel = (type) => typeLabels[type] || type
const getEventLabel = (event) => eventLabels[event] || event

const formatTime = (time) => {
  if (!time) return '-'
  return new Date(time).toLocaleString()
}

const fetchTemplates = async () => {
  try {
    const res = await getNotificationTemplates()
    templates.value = res.data?.list || []
  } catch (err) {
    console.error('获取模板失败:', err)
  }
}

const fetchLogs = async () => {
  try {
    const res = await getNotificationLogs(logFilter.value)
    logs.value = res.data?.list || []
  } catch (err) {
    console.error('获取日志失败:', err)
  }
}

const fetchEmailConfig = async () => {
  try {
    const res = await getEmailConfig()
    if (res.data) {
      emailConfig.value = { ...emailConfig.value, ...res.data }
    }
  } catch (err) {
    console.error('获取邮件配置失败:', err)
  }
}

const openTemplateModal = (template = null) => {
  if (template) {
    editingTemplate.value = template
    templateForm.value = { ...template }
  } else {
    editingTemplate.value = null
    templateForm.value = {
      name: '', type: 'email', event: 'user.register', title: '', content: '', enabled: true
    }
  }
  showTemplateModal.value = true
}

const saveTemplate = async () => {
  try {
    if (editingTemplate.value) {
      await updateNotificationTemplate(editingTemplate.value.id, templateForm.value)
    } else {
      await createNotificationTemplate(templateForm.value)
    }
    showTemplateModal.value = false
    fetchTemplates()
  } catch (err) {
    alert('保存失败: ' + (err.response?.data?.error || err.message))
  }
}

const deleteTemplate = async (template) => {
  if (!confirm(`确定删除模板 ${template.name}?`)) return
  try {
    await deleteNotificationTemplate(template.id)
    fetchTemplates()
  } catch (err) {
    alert('删除失败')
  }
}

const saveEmailConfig = async () => {
  try {
    await updateEmailConfig(emailConfig.value)
    alert('保存成功')
  } catch (err) {
    alert('保存失败: ' + (err.response?.data?.error || err.message))
  }
}

const openTestModal = () => {
  showTestModal.value = true
}

const sendTestEmail = async () => {
  if (!testEmail.value) {
    alert('请输入收件人地址')
    return
  }
  try {
    await sendTestNotification({
      type: 'email',
      recipient: testEmail.value,
      subject: '测试邮件',
      content: '这是一封测试邮件，如果您收到此邮件，说明邮件配置正确。'
    })
    alert('发送成功')
    showTestModal.value = false
  } catch (err) {
    alert('发送失败: ' + (err.response?.data?.error || err.message))
  }
}

onMounted(() => {
  fetchTemplates()
  fetchLogs()
  fetchEmailConfig()
})
</script>

<style scoped>
.config-section {
  background: var(--surface-color);
  padding: 24px;
  border-radius: var(--radius-lg);
  border: 1px solid var(--border-color);
}

.config-section h3 {
  margin-bottom: 16px;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--border-color);
}

.type-badge {
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 4px;
}

.type-badge.email { background: rgba(59, 130, 246, 0.15); color: #3b82f6; }
.type-badge.telegram { background: rgba(0, 136, 204, 0.15); color: #0088cc; }
.type-badge.webhook { background: rgba(168, 85, 247, 0.15); color: #a855f7; }

.status-failed { background: rgba(239, 68, 68, 0.15); color: #ef4444; }

.checkbox-label {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
}

.checkbox-label input[type="checkbox"] {
  width: 18px;
  height: 18px;
}

.modal-sm {
  max-width: 400px;
}
</style>