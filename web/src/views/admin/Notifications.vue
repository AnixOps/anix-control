<template>
  <div class="notification-page">
    <div class="page-header">
      <h1>{{ t('adminNotifications.title') }}</h1>
      <p class="text-secondary">{{ t('adminNotifications.subtitle') }}</p>
    </div>

    <div class="tabs">
      <button :class="['tab', { active: activeTab === 'templates' }]" @click="activeTab = 'templates'">
        {{ t('adminNotifications.tabs.templates') }}
      </button>
      <button :class="['tab', { active: activeTab === 'email' }]" @click="activeTab = 'email'">
        {{ t('adminNotifications.tabs.email') }}
      </button>
      <button :class="['tab', { active: activeTab === 'logs' }]" @click="activeTab = 'logs'">
        {{ t('adminNotifications.tabs.logs') }}
      </button>
    </div>

    <div v-show="activeTab === 'templates'">
      <div class="toolbar">
        <button class="btn-primary" @click="openTemplateModal()">
          + {{ t('adminNotifications.actions.createTemplate') }}
        </button>
      </div>

      <div class="table-container">
        <table class="data-table">
          <thead>
            <tr>
              <th>{{ t('adminNotifications.templates.table.id') }}</th>
              <th>{{ t('adminNotifications.templates.table.name') }}</th>
              <th>{{ t('adminNotifications.templates.table.type') }}</th>
              <th>{{ t('adminNotifications.templates.table.event') }}</th>
              <th>{{ t('adminNotifications.templates.table.status') }}</th>
              <th>{{ t('adminNotifications.templates.table.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="template in templates" :key="template.id">
              <td>{{ template.id }}</td>
              <td>{{ template.name || '-' }}</td>
              <td>
                <span :class="['type-badge', template.type]">
                  {{ getTypeLabel(template.type) }}
                </span>
              </td>
              <td>{{ getEventLabel(template.event) }}</td>
              <td>
                <span :class="['status-badge', template.enabled ? 'status-active' : 'status-disabled']">
                  {{ template.enabled ? t('adminNotifications.status.enabled') : t('adminNotifications.status.disabled') }}
                </span>
              </td>
              <td>
                <div class="action-buttons">
                  <button
                    class="btn-sm btn-ghost"
                    :title="t('adminNotifications.actions.edit')"
                    :aria-label="t('adminNotifications.actions.edit')"
                    @click="openTemplateModal(template)"
                  >
                    {{ t('adminNotifications.actions.edit') }}
                  </button>
                  <button
                    class="btn-sm btn-danger"
                    :title="t('adminNotifications.actions.delete')"
                    :aria-label="t('adminNotifications.actions.delete')"
                    @click="deleteTemplateItem(template)"
                  >
                    {{ t('adminNotifications.actions.delete') }}
                  </button>
                </div>
              </td>
            </tr>
            <tr v-if="templates.length === 0">
              <td colspan="6" class="empty-row">{{ t('adminNotifications.templates.empty') }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <div v-show="activeTab === 'email'">
      <div class="config-section">
        <h3>{{ t('adminNotifications.email.title') }}</h3>
        <div class="form-row">
          <div class="form-group">
            <label>{{ t('adminNotifications.email.fields.host') }}</label>
            <input
              v-model="emailConfig.host"
              type="text"
              :placeholder="t('adminNotifications.email.placeholders.host')"
            />
          </div>
          <div class="form-group">
            <label>{{ t('adminNotifications.email.fields.port') }}</label>
            <input
              v-model.number="emailConfig.port"
              type="number"
              :placeholder="t('adminNotifications.email.placeholders.port')"
            />
          </div>
        </div>
        <div class="form-row">
          <div class="form-group">
            <label>{{ t('adminNotifications.email.fields.username') }}</label>
            <input
              v-model="emailConfig.username"
              type="text"
              :placeholder="t('adminNotifications.email.placeholders.username')"
            />
          </div>
          <div class="form-group">
            <label>{{ t('adminNotifications.email.fields.password') }}</label>
            <input
              v-model="emailConfig.password"
              type="password"
              :placeholder="t('adminNotifications.email.placeholders.password')"
            />
          </div>
        </div>
        <div class="form-row">
          <div class="form-group">
            <label>{{ t('adminNotifications.email.fields.fromName') }}</label>
            <input
              v-model="emailConfig.from_name"
              type="text"
              :placeholder="t('adminNotifications.email.placeholders.fromName')"
            />
          </div>
          <div class="form-group">
            <label>{{ t('adminNotifications.email.fields.fromAddress') }}</label>
            <input
              v-model="emailConfig.from_address"
              type="email"
              :placeholder="t('adminNotifications.email.placeholders.fromAddress')"
            />
          </div>
        </div>
        <div class="form-group">
          <label class="checkbox-label">
            <input v-model="emailConfig.encryption" type="checkbox" />
            <span>{{ t('adminNotifications.email.fields.encryption') }}</span>
          </label>
        </div>
        <div class="form-actions">
          <button class="btn-primary" @click="saveEmailSettings">{{ t('common.actions.save') }}</button>
          <button class="btn-secondary" @click="openTestModal">{{ t('adminNotifications.actions.sendTest') }}</button>
        </div>
      </div>
    </div>

    <div v-show="activeTab === 'logs'">
      <div class="toolbar">
        <select v-model="logFilter.type">
          <option value="">{{ t('adminNotifications.logs.filters.allTypes') }}</option>
          <option v-for="type in notificationTypes" :key="type" :value="type">
            {{ getTypeLabel(type) }}
          </option>
        </select>
        <select v-model="logFilter.status">
          <option value="">{{ t('adminNotifications.logs.filters.allStatuses') }}</option>
          <option v-for="status in logStatuses" :key="status" :value="status">
            {{ getLogStatusLabel(status) }}
          </option>
        </select>
        <button class="btn-secondary" @click="fetchLogs">{{ t('adminNotifications.actions.search') }}</button>
      </div>

      <div class="table-container">
        <table class="data-table">
          <thead>
            <tr>
              <th>{{ t('adminNotifications.logs.table.id') }}</th>
              <th>{{ t('adminNotifications.logs.table.type') }}</th>
              <th>{{ t('adminNotifications.logs.table.recipient') }}</th>
              <th>{{ t('adminNotifications.logs.table.title') }}</th>
              <th>{{ t('adminNotifications.logs.table.status') }}</th>
              <th>{{ t('adminNotifications.logs.table.sentAt') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="log in logs" :key="log.id">
              <td>{{ log.id }}</td>
              <td>
                <span :class="['type-badge', log.type]">{{ getTypeLabel(log.type) }}</span>
              </td>
              <td>{{ log.recipient || '-' }}</td>
              <td>{{ log.title || '-' }}</td>
              <td>
                <span :class="['status-badge', getLogStatusClass(log.status)]">
                  {{ getLogStatusLabel(log.status) }}
                </span>
              </td>
              <td>{{ formatTime(log.created_at) }}</td>
            </tr>
            <tr v-if="logs.length === 0">
              <td colspan="6" class="empty-row">{{ t('adminNotifications.logs.empty') }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <div v-if="showTemplateModal" class="modal-overlay" @click.self="showTemplateModal = false">
      <div class="modal">
        <div class="modal-header">
          <h3>{{ editingTemplate ? t('adminNotifications.modal.editTitle') : t('adminNotifications.modal.createTitle') }}</h3>
          <button class="close-btn" :title="t('common.actions.close')" :aria-label="t('common.actions.close')" @click="showTemplateModal = false">×</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label>{{ t('adminNotifications.modal.fields.name') }} <span class="required">*</span></label>
            <input
              v-model="templateForm.name"
              type="text"
              :placeholder="t('adminNotifications.modal.placeholders.name')"
            />
          </div>
          <div class="form-row">
            <div class="form-group">
              <label>{{ t('adminNotifications.modal.fields.type') }}</label>
              <select v-model="templateForm.type">
                <option v-for="type in notificationTypes" :key="type" :value="type">
                  {{ getTypeLabel(type) }}
                </option>
              </select>
            </div>
            <div class="form-group">
              <label>{{ t('adminNotifications.modal.fields.event') }}</label>
              <select v-model="templateForm.event">
                <option v-for="event in notificationEvents" :key="event" :value="event">
                  {{ getEventLabel(event) }}
                </option>
              </select>
            </div>
          </div>
          <div class="form-group">
            <label>{{ t('adminNotifications.modal.fields.title') }}</label>
            <input
              v-model="templateForm.title"
              type="text"
              :placeholder="t('adminNotifications.modal.placeholders.title')"
            />
          </div>
          <div class="form-group">
            <label>{{ t('adminNotifications.modal.fields.content') }}</label>
            <textarea
              v-model="templateForm.content"
              rows="5"
              :placeholder="t('adminNotifications.modal.placeholders.content')"
            ></textarea>
          </div>
          <div class="form-group">
            <label class="checkbox-label">
              <input v-model="templateForm.enabled" type="checkbox" />
              <span>{{ t('adminNotifications.modal.fields.enabled') }}</span>
            </label>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn-secondary" @click="showTemplateModal = false">{{ t('common.actions.cancel') }}</button>
          <button @click="saveTemplate">{{ t('common.actions.save') }}</button>
        </div>
      </div>
    </div>

    <div v-if="showTestModal" class="modal-overlay" @click.self="showTestModal = false">
      <div class="modal modal-sm">
        <div class="modal-header">
          <h3>{{ t('adminNotifications.testModal.title') }}</h3>
          <button class="close-btn" :title="t('common.actions.close')" :aria-label="t('common.actions.close')" @click="showTestModal = false">×</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label>{{ t('adminNotifications.testModal.fields.recipient') }}</label>
            <input
              v-model="testEmail"
              type="email"
              :placeholder="t('adminNotifications.testModal.placeholders.recipient')"
            />
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn-secondary" @click="showTestModal = false">{{ t('common.actions.cancel') }}</button>
          <button @click="sendTestEmail">{{ t('adminNotifications.testModal.actions.send') }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import {
  createNotificationTemplate,
  deleteNotificationTemplate,
  getEmailConfig,
  getNotificationLogs,
  getNotificationTemplates,
  sendTestNotification,
  updateEmailConfig,
  updateNotificationTemplate
} from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'

const { t, formatDateTime } = useAppI18n()

const notificationTypes = ['email', 'telegram', 'webhook']
const notificationEvents = [
  'user.register',
  'user.login',
  'user.expire',
  'user.traffic_low',
  'order.paid',
  'ticket.reply',
  'node.offline',
  'node.online'
]
const logStatuses = ['pending', 'success', 'failed']

const activeTab = ref('templates')
const templates = ref([])
const logs = ref([])
const logFilter = ref({ type: '', status: '' })

const showTemplateModal = ref(false)
const editingTemplate = ref(null)
const templateForm = ref(createTemplateForm())

const showTestModal = ref(false)
const testEmail = ref('')

const emailConfig = ref({
  host: '',
  port: 465,
  username: '',
  password: '',
  from_name: '',
  from_address: '',
  encryption: true
})

function createTemplateForm(source = {}) {
  return {
    name: '',
    type: 'email',
    event: 'user.register',
    title: '',
    content: '',
    enabled: true,
    ...source
  }
}

const resolveApiError = (error, fallbackKey) => (
  error?.response?.data?.error ||
  error?.response?.data?.msg ||
  error?.message ||
  t(fallbackKey)
)

const typeKeyMap = {
  email: 'adminNotifications.types.email',
  telegram: 'adminNotifications.types.telegram',
  webhook: 'adminNotifications.types.webhook'
}

const eventKeyMap = {
  'user.register': 'adminNotifications.events.userRegister',
  'user.login': 'adminNotifications.events.userLogin',
  'user.expire': 'adminNotifications.events.userExpire',
  'user.traffic_low': 'adminNotifications.events.userTrafficLow',
  'order.paid': 'adminNotifications.events.orderPaid',
  'ticket.reply': 'adminNotifications.events.ticketReply',
  'node.offline': 'adminNotifications.events.nodeOffline',
  'node.online': 'adminNotifications.events.nodeOnline'
}

const logStatusKeyMap = {
  pending: 'adminNotifications.status.pending',
  success: 'adminNotifications.status.success',
  failed: 'adminNotifications.status.failed'
}

const getTypeLabel = (type) => (typeKeyMap[type] ? t(typeKeyMap[type]) : type || '-')
const getEventLabel = (event) => (eventKeyMap[event] ? t(eventKeyMap[event]) : event || '-')

const getLogStatusClass = (status) => {
  if (status === 'success') return 'status-success'
  if (status === 'pending') return 'status-pending'
  return 'status-failed'
}

const getLogStatusLabel = (status) => (logStatusKeyMap[status] ? t(logStatusKeyMap[status]) : status || '-')

const formatTime = (value) => {
  if (!value) return '-'
  return formatDateTime(value)
}

const fetchTemplates = async () => {
  try {
    const res = await getNotificationTemplates()
    templates.value = res.data?.list || []
  } catch (error) {
    console.error(t('adminNotifications.messages.fetchTemplatesFailed'), error)
  }
}

const fetchLogs = async () => {
  try {
    const res = await getNotificationLogs(logFilter.value)
    logs.value = res.data?.list || []
  } catch (error) {
    console.error(t('adminNotifications.messages.fetchLogsFailed'), error)
  }
}

const fetchEmailSettings = async () => {
  try {
    const res = await getEmailConfig()
    if (res.data) {
      emailConfig.value = { ...emailConfig.value, ...res.data }
    }
  } catch (error) {
    console.error(t('adminNotifications.messages.fetchEmailConfigFailed'), error)
  }
}

const openTemplateModal = (template = null) => {
  editingTemplate.value = template
  templateForm.value = createTemplateForm(template || {})
  showTemplateModal.value = true
}

const saveTemplate = async () => {
  try {
    if (editingTemplate.value) {
      await updateNotificationTemplate(editingTemplate.value.id, templateForm.value)
    } else {
      await createNotificationTemplate(templateForm.value)
    }
    window.alert(t('adminNotifications.messages.templateSaveSuccess'))
    showTemplateModal.value = false
    await fetchTemplates()
  } catch (error) {
    window.alert(
      t('adminNotifications.messages.templateSaveFailed', {
        message: resolveApiError(error, 'adminNotifications.messages.templateSaveFailedShort')
      })
    )
  }
}

const deleteTemplateItem = async (template) => {
  if (!window.confirm(t('adminNotifications.messages.deleteConfirm', { name: template.name }))) return
  try {
    await deleteNotificationTemplate(template.id)
    await fetchTemplates()
  } catch (error) {
    window.alert(
      t('adminNotifications.messages.deleteFailed', {
        message: resolveApiError(error, 'adminNotifications.messages.deleteFailedShort')
      })
    )
  }
}

const saveEmailSettings = async () => {
  try {
    await updateEmailConfig(emailConfig.value)
    window.alert(t('adminNotifications.messages.emailSaveSuccess'))
  } catch (error) {
    window.alert(
      t('adminNotifications.messages.emailSaveFailed', {
        message: resolveApiError(error, 'adminNotifications.messages.emailSaveFailedShort')
      })
    )
  }
}

const openTestModal = () => {
  showTestModal.value = true
}

const sendTestEmail = async () => {
  if (!testEmail.value.trim()) {
    window.alert(t('adminNotifications.messages.testRecipientRequired'))
    return
  }

  try {
    await sendTestNotification({
      type: 'email',
      recipient: testEmail.value.trim(),
      subject: t('adminNotifications.testPayload.subject'),
      content: t('adminNotifications.testPayload.content')
    })
    window.alert(t('adminNotifications.messages.testSendSuccess'))
    showTestModal.value = false
    testEmail.value = ''
  } catch (error) {
    window.alert(
      t('adminNotifications.messages.testSendFailed', {
        message: resolveApiError(error, 'adminNotifications.messages.testSendFailedShort')
      })
    )
  }
}

onMounted(() => {
  fetchTemplates()
  fetchLogs()
  fetchEmailSettings()
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

.type-badge.email {
  background: rgba(59, 130, 246, 0.15);
  color: #3b82f6;
}

.type-badge.telegram {
  background: rgba(0, 136, 204, 0.15);
  color: #0088cc;
}

.type-badge.webhook {
  background: rgba(168, 85, 247, 0.15);
  color: #a855f7;
}

.status-success {
  background: rgba(34, 197, 94, 0.15);
  color: #22c55e;
}

.status-pending {
  background: rgba(245, 158, 11, 0.15);
  color: #f59e0b;
}

.status-failed {
  background: rgba(239, 68, 68, 0.15);
  color: #ef4444;
}

.checkbox-label {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
}

.checkbox-label input[type='checkbox'] {
  width: 18px;
  height: 18px;
}

.action-buttons {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.btn-danger {
  background: rgba(239, 68, 68, 0.1);
  color: #ef4444;
  border: 1px solid rgba(239, 68, 68, 0.2);
}

.btn-danger:hover {
  background: rgba(239, 68, 68, 0.18);
}

.modal-sm {
  max-width: 400px;
}
</style>
