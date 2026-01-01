<template>
  <div class="tickets-page">
    <div class="page-header">
      <div class="header-content">
        <h1>🎫 我的工单</h1>
        <p class="text-secondary">提交反馈或寻求技术支持</p>
      </div>
      <button class="btn-primary" @click="showCreate = true">➕ 提交工单</button>
    </div>

    <!-- 筛选和统计 -->
    <div class="stats-bar">
      <div class="stat-item">
        <span class="label">进行中</span>
        <span class="value">{{ activeCount }}</span>
      </div>
      <div class="stat-item">
        <span class="label">已处理</span>
        <span class="value">{{ resolvedCount }}</span>
      </div>
    </div>

    <!-- 工单列表 -->
    <div class="content-container">
      <div v-if="loading" class="loading-state">
        <div class="spinner"></div>
        <p>加载中...</p>
      </div>

      <div v-else-if="tickets.length === 0" class="empty-state">
        <div class="empty-icon">🎟️</div>
        <p>暂无任何工单，如有疑问欢迎提交反馈</p>
        <button class="btn-secondary mt-4" @click="showCreate = true">立即提交</button>
      </div>

      <div v-else class="tickets-list">
        <div 
          v-for="ticket in tickets" 
          :key="ticket.id" 
          class="ticket-card"
          @click="viewDetail(ticket)"
        >
          <div class="ticket-status">
            <span :class="['status-dot', statusClass(ticket.status)]"></span>
            <span :class="['status-text', statusClass(ticket.status)]">{{ statusText(ticket.status) }}</span>
          </div>
          <div class="ticket-info">
            <h3 class="ticket-subject">{{ ticket.subject }}</h3>
            <div class="ticket-meta">
              <span class="ticket-id">#{{ ticket.id }}</span>
              <span class="divider">·</span>
              <span class="ticket-date">{{ formatDate(ticket.updated_at) }} 更新</span>
            </div>
          </div>
          <div class="ticket-arrow">→</div>
        </div>
      </div>
    </div>

    <!-- 提交工单弹窗 -->
    <div v-if="showCreate" class="modal-overlay" @click.self="showCreate = false">
      <div class="modal">
        <div class="modal-header">
          <h3>提交新工单</h3>
          <button class="close-btn" @click="showCreate = false">✕</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label>主题 <span class="required">*</span></label>
            <input v-model="createForm.subject" type="text" placeholder="简述您的问题">
          </div>
          <div class="form-group">
            <label>优先级</label>
            <select v-model="createForm.level">
              <option :value="0">低 (一般建议)</option>
              <option :value="1">中 (使用遇到困难)</option>
              <option :value="2">高 (无法使用/紧急故障)</option>
            </select>
          </div>
          <div class="form-group">
            <label>描述内容 <span class="required">*</span></label>
            <textarea v-model="createForm.message" rows="6" placeholder="请详细描述您遇到的问题..."></textarea>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn-secondary" @click="showCreate = false">取消</button>
          <button class="btn-primary" @click="submitCreate" :disabled="submitting">
            {{ submitting ? '提交中...' : '提交反馈' }}
          </button>
        </div>
      </div>
    </div>

    <!-- 工单详情弹窗 -->
    <div v-if="showDetail" class="modal-overlay" @click.self="showDetail = false">
      <div class="modal modal-lg ticket-detail-modal">
        <div class="modal-header">
          <div class="header-top">
            <span :class="['status-badge', statusIndicator(detailTicket.status)]">
              {{ statusText(detailTicket.status) }}
            </span>
            <span class="ticket-id">工单 #{{ detailTicket.id }}</span>
          </div>
          <h3>{{ detailTicket.subject }}</h3>
          <button class="close-btn" @click="showDetail = false">✕</button>
        </div>
        
        <div class="modal-body chat-container">
          <div class="messages-list">
            <div 
              v-for="msg in detailTicket.messages" 
              :key="msg.id" 
              :class="['message-item', msg.is_admin ? 'admin' : 'user']"
            >
              <div class="message-bubble">
                <div class="message-sender">{{ msg.is_admin ? '客服助手' : '我' }}</div>
                <div class="message-content">{{ msg.message }}</div>
                <div class="message-time">{{ formatTime(msg.created_at) }}</div>
              </div>
            </div>
          </div>
        </div>

        <div class="modal-footer footer-reply" v-if="detailTicket.status !== 2">
          <div class="reply-input-wrapper">
            <textarea 
              v-model="replyMessage" 
              rows="2" 
              placeholder="回复内容..."
              @keyup.ctrl.enter="submitReply"
            ></textarea>
            <div class="reply-actions">
              <button class="btn-ghost" @click="handleClose(detailTicket.id)">关闭工单</button>
              <button class="btn-primary btn-sm" @click="submitReply" :disabled="replying">
                {{ replying ? '发送中' : '发送回复' }}
              </button>
            </div>
          </div>
        </div>
        <div class="modal-footer" v-else>
          <div class="text-secondary">此工单已关闭，如需进一步支持请提交新工单</div>
          <button class="btn-secondary" @click="showDetail = false">关闭窗口</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { getTickets, createTicket, getTicketDetail, replyTicket, closeTicket } from '@/api/user'

const tickets = ref([])
const loading = ref(true)
const showCreate = ref(false)
const showDetail = ref(false)
const submitting = ref(false)
const replying = ref(false)

const createForm = ref({
  subject: '',
  level: 1,
  message: ''
})

const detailTicket = ref({})
const replyMessage = ref('')

const activeCount = computed(() => tickets.value.filter(t => t.status !== 2).length)
const resolvedCount = computed(() => tickets.value.filter(t => t.status === 2).length)

const fetchTickets = async () => {
  loading.value = true
  try {
    const res = await getTickets()
    tickets.value = res.data || []
  } catch (err) {
    console.error('获取工单失败:', err)
  } finally {
    loading.value = false
  }
}

const statusText = (s) => ['待处理', '已回复', '已关闭'][s] || '未知'
const statusClass = (s) => ['status-open', 'status-answered', 'status-closed'][s] || ''
const statusIndicator = (s) => ['indicator-blue', 'indicator-green', 'indicator-gray'][s] || ''

const formatDate = (ts) => {
  if (!ts) return '-'
  return new Date(ts * 1000).toLocaleDateString('zh-CN')
}

const formatTime = (iso) => {
  return new Date(iso).toLocaleString('zh-CN', { 
    month: 'long', 
    day: 'numeric', 
    hour: '2-digit', 
    minute: '2-digit' 
  })
}

const submitCreate = async () => {
  if (!createForm.value.subject || !createForm.value.message) {
    alert('请填写主题和内容')
    return
  }
  submitting.value = true
  try {
    await createTicket(createForm.value)
    showCreate.value = false
    createForm.value = { subject: '', level: 1, message: '' }
    await fetchTickets()
  } catch (err) {
    alert(err.response?.data?.message || '提交失败')
  } finally {
    submitting.value = false
  }
}

const viewDetail = async (ticket) => {
  try {
    const res = await getTicketDetail(ticket.id)
    detailTicket.value = res.data
    showDetail.value = true
  } catch (err) {
    alert('加载详情失败')
  }
}

const submitReply = async () => {
  if (!replyMessage.value.trim()) return
  replying.value = true
  try {
    await replyTicket(detailTicket.value.id, { message: replyMessage.value })
    replyMessage.value = ''
    // 刷新详情
    const res = await getTicketDetail(detailTicket.value.id)
    detailTicket.value = res.data
    // 刷新列表
    await fetchTickets()
  } catch (err) {
    alert('发送失败')
  } finally {
    replying.value = false
  }
}

const handleClose = async (id) => {
  if (!confirm('确定要关闭此工单吗？')) return
  try {
    await closeTicket(id)
    showDetail.value = false
    await fetchTickets()
  } catch (err) {
    alert('关闭失败')
  }
}

onMounted(() => {
  fetchTickets()
})
</script>

<style scoped>
.tickets-page {
  max-width: 1000px;
  margin: 0 auto;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 32px;
}

.page-header h1 {
  font-size: 28px;
  margin-bottom: 4px;
}

.stats-bar {
  display: flex;
  gap: 32px;
  margin-bottom: 24px;
  padding: 16px 24px;
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
}

.stat-item {
  display: flex;
  flex-direction: column;
}

.stat-item .label {
  font-size: 13px;
  color: var(--text-secondary);
  margin-bottom: 4px;
}

.stat-item .value {
  font-size: 20px;
  font-weight: 700;
}

.tickets-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.ticket-card {
  display: flex;
  align-items: center;
  padding: 16px 20px;
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: var(--transition);
}

.ticket-card:hover {
  border-color: var(--primary-color);
  transform: translateX(4px);
}

.ticket-status {
  display: flex;
  flex-direction: column;
  align-items: center;
  min-width: 80px;
  margin-right: 20px;
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  margin-bottom: 4px;
}

.status-text {
  font-size: 12px;
  font-weight: 500;
}

.status-open { color: var(--primary-color); }
.status-open.status-dot { background: var(--primary-color); }

.status-answered { color: var(--success-color); }
.status-answered.status-dot { background: var(--success-color); }

.status-closed { color: var(--text-secondary); }
.status-closed.status-dot { background: var(--text-secondary); }

.ticket-info {
  flex: 1;
}

.ticket-subject {
  font-size: 16px;
  font-weight: 600;
  margin-bottom: 4px;
}

.ticket-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  color: var(--text-secondary);
}

.ticket-arrow {
  font-size: 18px;
  color: var(--border-color);
  margin-left: 12px;
}

/* 详情弹窗聊天样式 */
.ticket-detail-modal .modal {
  display: flex;
  flex-direction: column;
  height: 80vh;
}

.header-top {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 8px;
}

.status-badge {
  font-size: 11px;
  font-weight: 600;
  padding: 2px 8px;
  border-radius: 4px;
}

.indicator-blue { background: rgba(59, 130, 246, 0.1); color: var(--primary-color); }
.indicator-green { background: rgba(34, 197, 94, 0.1); color: var(--success-color); }
.indicator-gray { background: rgba(161, 161, 170, 0.1); color: var(--text-secondary); }

.chat-container {
  flex: 1;
  overflow-y: auto;
  padding: 20px;
  background: var(--bg-color);
  display: flex;
  flex-direction: column-reverse; /* 最新的在下面，自适应滚动 */
}

.messages-list {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.message-item {
  display: flex;
}

.message-item.user { justify-content: flex-end; }
.message-item.admin { justify-content: flex-start; }

.message-bubble {
  max-width: 80%;
  padding: 12px 16px;
  border-radius: var(--radius-lg);
  position: relative;
}

.user .message-bubble {
  background: var(--primary-color);
  color: white;
  border-bottom-right-radius: 2px;
}

.admin .message-bubble {
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-bottom-left-radius: 2px;
}

.message-sender {
  font-size: 11px;
  font-weight: 700;
  margin-bottom: 4px;
  opacity: 0.8;
}

.message-content {
  font-size: 14px;
  line-height: 1.6;
  white-space: pre-wrap;
}

.message-time {
  font-size: 10px;
  margin-top: 6px;
  opacity: 0.6;
}

.footer-reply {
  padding: 16px 20px;
  border-top: 1px solid var(--border-color);
}

.reply-input-wrapper {
  display: flex;
  flex-direction: column;
  gap: 12px;
  width: 100%;
}

.reply-input-wrapper textarea {
  background: var(--bg-color);
  border-radius: var(--radius-md);
  padding: 12px;
}

.reply-actions {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

/* 弹窗通用样式 */
.modal-lg { max-width: 700px; }

/* 状态样式 */
.loading-state, .empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 80px 20px;
  color: var(--text-secondary);
}

.spinner {
  width: 40px;
  height: 40px;
  border: 3px solid rgba(59, 130, 246, 0.1);
  border-top-color: var(--primary-color);
  border-radius: 50%;
  animation: spin 1s linear infinite;
  margin-bottom: 16px;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.empty-icon {
  font-size: 48px;
  margin-bottom: 16px;
}
</style>
