<template>
  <div class="telegram-page">
    <div class="page-header">
      <h1>Telegram Bot 管理</h1>
      <p class="text-secondary">配置 Telegram 机器人和消息通知</p>
    </div>

    <!-- 标签切换 -->
    <div class="tabs">
      <button :class="['tab', { active: activeTab === 'config' }]" @click="activeTab = 'config'">
        Bot 配置
      </button>
      <button :class="['tab', { active: activeTab === 'users' }]" @click="activeTab = 'users'">
        绑定用户
      </button>
      <button :class="['tab', { active: activeTab === 'notify' }]" @click="activeTab = 'notify'">
        发送通知
      </button>
    </div>

    <!-- Bot 配置 -->
    <div v-show="activeTab === 'config'">
      <div class="config-section">
        <h3>基础配置</h3>
        <div class="form-group">
          <label>Bot Token <span class="required">*</span></label>
          <input v-model="botConfig.token" type="text" placeholder="从 @BotFather 获取的 Token" />
        </div>
        <div class="form-group">
          <label>Webhook URL</label>
          <div class="input-group">
            <input :value="webhookUrl" readonly />
            <button class="btn-secondary" @click="setWebhook">设置 Webhook</button>
            <button class="btn-secondary" @click="deleteWebhook">删除 Webhook</button>
          </div>
        </div>
        <div class="form-group">
          <label>管理员 ID (逗号分隔)</label>
          <input v-model="adminIdsStr" type="text" placeholder="如: 123456,789012" />
        </div>
        <div class="form-group">
          <label>欢迎消息</label>
          <textarea v-model="botConfig.welcome_message" rows="3" placeholder="用户 /start 时的欢迎消息"></textarea>
        </div>
        <div class="form-actions">
          <button class="btn-primary" @click="saveBotConfig">保存配置</button>
        </div>
      </div>

      <div class="config-section">
        <h3>可用命令</h3>
        <div class="commands-list">
          <div class="command-item" v-for="cmd in commands" :key="cmd.cmd">
            <code>{{ cmd.cmd }}</code>
            <span class="command-desc">{{ cmd.desc }}</span>
          </div>
        </div>
      </div>
    </div>

    <!-- 绑定用户 -->
    <div v-show="activeTab === 'users'">
      <div class="toolbar">
        <input v-model="userSearch" type="text" placeholder="搜索用户..." class="search-input" />
        <button class="btn-secondary" @click="fetchUsers">🔍 刷新</button>
      </div>

      <div class="table-container">
        <table class="data-table">
          <thead>
            <tr>
              <th>ID</th>
              <th>Telegram ID</th>
              <th>用户ID</th>
              <th>用户邮箱</th>
              <th>绑定时间</th>
              <th>通知状态</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="user in filteredUsers" :key="user.id">
              <td>{{ user.id }}</td>
              <td>{{ user.telegram_id }}</td>
              <td>{{ user.user_id }}</td>
              <td>{{ user.user_email || '-' }}</td>
              <td>{{ formatTime(user.created_at) }}</td>
              <td>
                <span :class="['status-badge', user.notify_enabled ? 'status-active' : 'status-disabled']">
                  {{ user.notify_enabled ? '已启用' : '已禁用' }}
                </span>
              </td>
              <td>
                <div class="action-buttons">
                  <button class="btn-sm btn-ghost" @click="toggleUserNotify(user)" title="切换通知">
                    {{ user.notify_enabled ? '🔔' : '🔕' }}
                  </button>
                </div>
              </td>
            </tr>
            <tr v-if="filteredUsers.length === 0">
              <td colspan="7" class="empty-row">暂无绑定用户</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 发送通知 -->
    <div v-show="activeTab === 'notify'">
      <div class="notify-section">
        <div class="form-group">
          <label>通知类型</label>
          <select v-model="notifyForm.type">
            <option value="single">单个用户</option>
            <option value="broadcast">广播消息</option>
          </select>
        </div>

        <div class="form-group" v-if="notifyForm.type === 'single'">
          <label>Telegram ID</label>
          <input v-model="notifyForm.telegram_id" type="text" placeholder="用户 Telegram ID" />
        </div>

        <div class="form-group">
          <label>消息内容</label>
          <textarea v-model="notifyForm.message" rows="5" placeholder="支持 Markdown 格式"></textarea>
        </div>

        <div class="form-actions">
          <button class="btn-primary" @click="sendNotification">
            {{ notifyForm.type === 'broadcast' ? '广播消息' : '发送通知' }}
          </button>
        </div>
      </div>

      <div class="broadcast-tips">
        <h4>💡 提示</h4>
        <ul>
          <li>广播消息将发送给所有已绑定并启用通知的用户</li>
          <li>消息支持 Markdown 格式: *粗体* _斜体_ `代码`</li>
          <li>建议先测试单个用户再进行广播</li>
        </ul>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import {
  getTelegramBot, updateTelegramBot, setTelegramWebhook, deleteTelegramWebhook,
  getTelegramUsers, sendTelegramNotification, broadcastTelegram
} from '@/api/admin'

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

const commands = [
  { cmd: '/start', desc: '开始使用 Bot' },
  { cmd: '/bind', desc: '绑定账户' },
  { cmd: '/unbind', desc: '解绑账户' },
  { cmd: '/info', desc: '查看账户信息' },
  { cmd: '/sub', desc: '获取订阅链接' },
  { cmd: '/renew', desc: '续费套餐' },
  { cmd: '/ticket', desc: '创建工单' },
  { cmd: '/help', desc: '帮助信息' }
]

const adminIdsStr = computed({
  get: () => botConfig.value.admin_ids?.join(',') || '',
  set: (val) => {
    botConfig.value.admin_ids = val.split(',').map(id => parseInt(id.trim())).filter(id => !isNaN(id))
  }
})

const webhookUrl = computed(() => {
  if (!botConfig.value.token) return ''
  return `${window.location.origin}/api/v2/telegram/webhook`
})

const filteredUsers = computed(() => {
  if (!userSearch.value) return users.value
  const search = userSearch.value.toLowerCase()
  return users.value.filter(u =>
    u.telegram_id?.toString().includes(search) ||
    u.user_email?.toLowerCase().includes(search)
  )
})

const formatTime = (time) => {
  if (!time) return '-'
  return new Date(time).toLocaleString()
}

const fetchBotConfig = async () => {
  try {
    const res = await getTelegramBot()
    botConfig.value = res.data || { token: '', admin_ids: [], welcome_message: '' }
  } catch (err) {
    console.error('获取配置失败:', err)
  }
}

const saveBotConfig = async () => {
  try {
    await updateTelegramBot(botConfig.value)
    alert('保存成功')
  } catch (err) {
    alert('保存失败: ' + (err.response?.data?.error || err.message))
  }
}

const setWebhook = async () => {
  try {
    await setTelegramWebhook()
    alert('Webhook 设置成功')
  } catch (err) {
    alert('设置失败: ' + (err.response?.data?.error || err.message))
  }
}

const deleteWebhook = async () => {
  try {
    await deleteTelegramWebhook()
    alert('Webhook 已删除')
  } catch (err) {
    alert('删除失败')
  }
}

const fetchUsers = async () => {
  try {
    const res = await getTelegramUsers()
    users.value = res.data?.list || []
  } catch (err) {
    console.error('获取用户失败:', err)
  }
}

const toggleUserNotify = (user) => {
  user.notify_enabled = !user.notify_enabled
}

const sendNotification = async () => {
  if (!notifyForm.value.message) {
    alert('请输入消息内容')
    return
  }

  try {
    if (notifyForm.value.type === 'broadcast') {
      const res = await broadcastTelegram({ message: notifyForm.value.message })
      alert(`广播完成，成功: ${res.data?.success || 0}，失败: ${res.data?.failed || 0}`)
    } else {
      if (!notifyForm.value.telegram_id) {
        alert('请输入 Telegram ID')
        return
      }
      await sendTelegramNotification({
        telegram_id: notifyForm.value.telegram_id,
        message: notifyForm.value.message
      })
      alert('发送成功')
    }
    notifyForm.value.message = ''
  } catch (err) {
    alert('发送失败: ' + (err.response?.data?.error || err.message))
  }
}

onMounted(() => {
  fetchBotConfig()
  fetchUsers()
})
</script>

<style scoped>
.config-section, .notify-section {
  background: var(--surface-color);
  padding: 24px;
  border-radius: var(--radius-lg);
  border: 1px solid var(--border-color);
  margin-bottom: 20px;
}

.config-section h3, .notify-section h3 {
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