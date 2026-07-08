<template>
  <div class="telegram-page">
    <div class="page-header">
      <h1>{{ t('adminTelegram.title') }}</h1>
      <p class="text-secondary">{{ t('adminTelegram.subtitle') }}</p>
    </div>

    <div class="tabs">
      <button :class="['tab', { active: activeTab === 'config' }]" @click="activeTab = 'config'">
        {{ t('adminTelegram.tabs.config') }}
      </button>
      <button :class="['tab', { active: activeTab === 'users' }]" @click="activeTab = 'users'">
        {{ t('adminTelegram.tabs.users') }}
      </button>
      <button :class="['tab', { active: activeTab === 'notify' }]" @click="activeTab = 'notify'">
        {{ t('adminTelegram.tabs.notify') }}
      </button>
    </div>

    <div v-show="activeTab === 'config'">
      <div class="config-section">
        <h3>{{ t('adminTelegram.config.basicTitle') }}</h3>
        <div class="form-group">
          <label>{{ t('adminTelegram.config.token') }} <span class="required">*</span></label>
          <input
            v-model="botConfig.token"
            type="text"
            :placeholder="t('adminTelegram.placeholders.token')"
          />
        </div>
        <div class="form-group">
          <label>{{ t('adminTelegram.config.webhookUrl') }}</label>
          <div class="input-group">
            <input :value="webhookUrl" readonly />
            <button class="btn-secondary" @click="setWebhookConfig">
              {{ t('adminTelegram.actions.setWebhook') }}
            </button>
            <button class="btn-secondary" @click="deleteWebhookConfig">
              {{ t('adminTelegram.actions.deleteWebhook') }}
            </button>
          </div>
        </div>
        <div class="form-group">
          <label>{{ t('adminTelegram.config.adminIds') }}</label>
          <input
            v-model="adminIdsStr"
            type="text"
            :placeholder="t('adminTelegram.placeholders.adminIds')"
          />
        </div>
        <div class="form-group">
          <label>{{ t('adminTelegram.config.welcomeMessage') }}</label>
          <textarea
            v-model="botConfig.welcome_message"
            rows="3"
            :placeholder="t('adminTelegram.placeholders.welcomeMessage')"
          ></textarea>
        </div>
        <div class="form-actions">
          <button class="btn-primary" @click="saveBotConfig">{{ t('common.actions.save') }}</button>
        </div>
      </div>

      <div class="config-section">
        <h3>{{ t('adminTelegram.commands.title') }}</h3>
        <div class="commands-list">
          <div v-for="command in commands" :key="command.cmd" class="command-item">
            <code>{{ command.cmd }}</code>
            <span class="command-desc">{{ command.desc }}</span>
          </div>
        </div>
      </div>
    </div>

    <div v-show="activeTab === 'users'">
      <div class="toolbar">
        <input
          v-model="userSearch"
          type="text"
          class="search-input"
          :placeholder="t('adminTelegram.users.searchPlaceholder')"
        />
        <button class="btn-secondary" @click="fetchUsers">
          {{ t('adminTelegram.actions.refreshUsers') }}
        </button>
      </div>

      <div class="table-container">
        <table class="data-table">
          <thead>
            <tr>
              <th>{{ t('adminTelegram.users.table.id') }}</th>
              <th>{{ t('adminTelegram.users.table.telegramId') }}</th>
              <th>{{ t('adminTelegram.users.table.userId') }}</th>
              <th>{{ t('adminTelegram.users.table.userEmail') }}</th>
              <th>{{ t('adminTelegram.users.table.boundAt') }}</th>
              <th>{{ t('adminTelegram.users.table.notifyStatus') }}</th>
              <th>{{ t('adminTelegram.users.table.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="user in filteredUsers" :key="user.id">
              <td>{{ user.id }}</td>
              <td>{{ user.telegram_id }}</td>
              <td>{{ user.user_id }}</td>
              <td>{{ user.user_email || '-' }}</td>
              <td>{{ formatBoundTime(user.created_at) }}</td>
              <td>
                <span :class="['status-badge', user.notify_enabled ? 'status-active' : 'status-disabled']">
                  {{ user.notify_enabled ? t('adminTelegram.status.enabled') : t('adminTelegram.status.disabled') }}
                </span>
              </td>
              <td>
                <div class="action-buttons">
                  <button
                    class="btn-sm btn-ghost"
                    :title="t('adminTelegram.actions.toggleNotify')"
                    :aria-label="t('adminTelegram.actions.toggleNotify')"
                    @click="toggleUserNotify(user)"
                  >
                    {{ user.notify_enabled ? t('adminTelegram.actions.disableNotifyLabel') : t('adminTelegram.actions.enableNotifyLabel') }}
                  </button>
                </div>
              </td>
            </tr>
            <tr v-if="filteredUsers.length === 0">
              <td colspan="7" class="empty-row">{{ t('adminTelegram.users.empty') }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <div v-show="activeTab === 'notify'">
      <div class="notify-section">
        <div class="form-group">
          <label>{{ t('adminTelegram.notify.type') }}</label>
          <select v-model="notifyForm.type">
            <option value="single">{{ t('adminTelegram.notify.types.single') }}</option>
            <option value="broadcast">{{ t('adminTelegram.notify.types.broadcast') }}</option>
          </select>
        </div>

        <div v-if="notifyForm.type === 'single'" class="form-group">
          <label>{{ t('adminTelegram.notify.telegramId') }}</label>
          <input
            v-model="notifyForm.telegram_id"
            type="text"
            :placeholder="t('adminTelegram.placeholders.telegramId')"
          />
        </div>

        <div class="form-group">
          <label>{{ t('adminTelegram.notify.message') }}</label>
          <textarea
            v-model="notifyForm.message"
            rows="5"
            :placeholder="t('adminTelegram.placeholders.message')"
          ></textarea>
        </div>

        <div class="form-actions">
          <button class="btn-primary" @click="sendNotification">
            {{ notifyForm.type === 'broadcast' ? t('adminTelegram.actions.broadcast') : t('adminTelegram.actions.send') }}
          </button>
        </div>
      </div>

      <div class="broadcast-tips">
        <h4>{{ t('adminTelegram.tips.title') }}</h4>
        <ul>
          <li>{{ t('adminTelegram.tips.broadcastScope') }}</li>
          <li>{{ t('adminTelegram.tips.markdown') }}</li>
          <li>{{ t('adminTelegram.tips.singleFirst') }}</li>
        </ul>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import {
  broadcastTelegram,
  deleteTelegramWebhook,
  getTelegramBot,
  getTelegramUsers,
  sendTelegramNotification,
  setTelegramWebhook,
  updateTelegramBot,
  updateTelegramUserNotify
} from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'

const { t, formatDateTime } = useAppI18n()

const activeTab = ref('config')
const userSearch = ref('')
const users = ref([])

const botConfig = ref({
  token: '',
  admin_ids: [],
  welcome_message: ''
})

const notifyForm = ref({
  type: 'single',
  telegram_id: '',
  message: ''
})

const commands = computed(() => ([
  { cmd: '/start', desc: t('adminTelegram.commands.items.start') },
  { cmd: '/bind', desc: t('adminTelegram.commands.items.bind') },
  { cmd: '/unbind', desc: t('adminTelegram.commands.items.unbind') },
  { cmd: '/info', desc: t('adminTelegram.commands.items.info') },
  { cmd: '/sub', desc: t('adminTelegram.commands.items.sub') },
  { cmd: '/renew', desc: t('adminTelegram.commands.items.renew') },
  { cmd: '/ticket', desc: t('adminTelegram.commands.items.ticket') },
  { cmd: '/help', desc: t('adminTelegram.commands.items.help') }
]))

const adminIdsStr = computed({
  get: () => botConfig.value.admin_ids?.join(',') || '',
  set: (value) => {
    botConfig.value.admin_ids = value
      .split(',')
      .map((id) => parseInt(id.trim(), 10))
      .filter((id) => !Number.isNaN(id))
  }
})

const webhookUrl = computed(() => {
  if (!botConfig.value.token) return ''
  return `${window.location.origin}/api/v2/telegram/webhook`
})

const filteredUsers = computed(() => {
  if (!userSearch.value) return users.value
  const search = userSearch.value.toLowerCase()
  return users.value.filter((user) => (
    user.telegram_id?.toString().includes(search) ||
    user.user_email?.toLowerCase().includes(search)
  ))
})

const resolveApiError = (error, fallbackKey) => (
  error?.response?.data?.error ||
  error?.response?.data?.msg ||
  error?.message ||
  t(fallbackKey)
)

const readTelegramPayload = (res) => {
  if (!res || typeof res !== 'object') return {}
  if (Object.prototype.hasOwnProperty.call(res, 'code')) {
    return res.data && typeof res.data === 'object' ? res.data : {}
  }
  if (res.data && typeof res.data === 'object' && Object.prototype.hasOwnProperty.call(res.data, 'data')) {
    return res.data.data && typeof res.data.data === 'object' ? res.data.data : {}
  }
  return res.data && typeof res.data === 'object' ? res.data : res
}

const formatBoundTime = (time) => {
  if (!time) return '-'
  return formatDateTime(time)
}

const fetchBotConfig = async () => {
  try {
    const res = await getTelegramBot()
    const payload = readTelegramPayload(res)
    botConfig.value = Object.keys(payload).length > 0 ? payload : { token: '', admin_ids: [], welcome_message: '' }
  } catch (error) {
    console.error(t('adminTelegram.messages.fetchConfigFailed'), error)
  }
}

const saveBotConfig = async () => {
  try {
    await updateTelegramBot(botConfig.value)
    window.alert(t('adminTelegram.messages.saveSuccess'))
  } catch (error) {
    window.alert(t('adminTelegram.messages.saveFailed', { message: resolveApiError(error, 'adminTelegram.messages.saveFailedShort') }))
  }
}

const setWebhookConfig = async () => {
  try {
    await setTelegramWebhook(webhookUrl.value || undefined)
    window.alert(t('adminTelegram.messages.webhookSetSuccess'))
  } catch (error) {
    window.alert(t('adminTelegram.messages.webhookSetFailed', { message: resolveApiError(error, 'adminTelegram.messages.webhookSetFailedShort') }))
  }
}

const deleteWebhookConfig = async () => {
  try {
    await deleteTelegramWebhook()
    window.alert(t('adminTelegram.messages.webhookDeleteSuccess'))
  } catch (error) {
    window.alert(t('adminTelegram.messages.webhookDeleteFailed', { message: resolveApiError(error, 'adminTelegram.messages.webhookDeleteFailedShort') }))
  }
}

const fetchUsers = async () => {
  try {
    const res = await getTelegramUsers({ all: true })
    const payload = readTelegramPayload(res)
    users.value = payload.list || (Array.isArray(payload) ? payload : [])
  } catch (error) {
    console.error(t('adminTelegram.messages.fetchUsersFailed'), error)
  }
}

const toggleUserNotify = async (user) => {
  try {
    const nextEnabled = !user.notify_enabled
    const res = await updateTelegramUserNotify(user.id, { notify_enabled: nextEnabled })
    const updated = readTelegramPayload(res)
    user.notify_enabled = !!updated.notify_enabled
    user.notify_expire = !!updated.notify_expire
    user.notify_traffic = !!updated.notify_traffic
    user.notify_ticket = !!updated.notify_ticket
  } catch (error) {
    window.alert(t('adminTelegram.messages.toggleNotifyFailed', { message: resolveApiError(error, 'adminTelegram.messages.toggleNotifyFailedShort') }))
  }
}

const sendNotification = async () => {
  if (!notifyForm.value.message) {
    window.alert(t('adminTelegram.messages.messageRequired'))
    return
  }

  try {
    if (notifyForm.value.type === 'broadcast') {
      const res = await broadcastTelegram(notifyForm.value.message)
      const payload = readTelegramPayload(res)
      window.alert(t('adminTelegram.messages.broadcastComplete', {
        success: payload.success || 0,
        failed: payload.failed || 0
      }))
    } else {
      if (!notifyForm.value.telegram_id) {
        window.alert(t('adminTelegram.messages.telegramIdRequired'))
        return
      }
      await sendTelegramNotification({
        telegram_id: notifyForm.value.telegram_id,
        message: notifyForm.value.message
      })
      window.alert(t('adminTelegram.messages.sendSuccess'))
    }
    notifyForm.value.message = ''
  } catch (error) {
    window.alert(t('adminTelegram.messages.sendFailed', { message: resolveApiError(error, 'adminTelegram.messages.sendFailedShort') }))
  }
}

onMounted(() => {
  fetchBotConfig()
  fetchUsers()
})
</script>

<style scoped>
.config-section,
.notify-section {
  background: var(--surface-color);
  padding: 24px;
  border-radius: var(--radius-lg);
  border: 1px solid var(--border-color);
  margin-bottom: 20px;
}

.config-section h3,
.notify-section h3 {
  margin-bottom: 16px;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--border-color);
}

.input-group {
  display: flex;
  gap: 8px;
}

.input-group input {
  flex: 1;
}

.commands-list {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 12px;
}

.command-item {
  display: flex;
  align-items: center;
  gap: 8px;
}

.command-item code {
  background: var(--background-color);
  padding: 4px 8px;
  border-radius: 4px;
  font-size: 13px;
}

.command-desc {
  color: var(--text-secondary);
  font-size: 13px;
}

.search-input {
  min-width: 200px;
}

.broadcast-tips {
  background: rgba(59, 130, 246, 0.1);
  padding: 16px;
  border-radius: var(--radius-md);
  border: 1px solid rgba(59, 130, 246, 0.2);
}

.broadcast-tips h4 {
  margin-bottom: 8px;
}

.broadcast-tips ul {
  margin: 0;
  padding-left: 20px;
  color: var(--text-secondary);
  font-size: 13px;
}

.broadcast-tips li {
  margin-bottom: 4px;
}
</style>
