<template>
  <div class="tickets-page">
    <div class="page-header">
      <h1>工单管理</h1>
      <p class="text-secondary">查看和回复用户提交的工单</p>
    </div>

    <!-- 统计卡片 -->
    <div class="stats-grid">
      <div class="stat-card">
        <div class="stat-icon">📬</div>
        <div class="stat-info">
          <div class="stat-value">{{ openCount }}</div>
          <div class="stat-label">待处理</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon">✅</div>
        <div class="stat-info">
          <div class="stat-value">{{ answeredCount }}</div>
          <div class="stat-label">已回复</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon">🔒</div>
        <div class="stat-info">
          <div class="stat-value">{{ closedCount }}</div>
          <div class="stat-label">已关闭</div>
        </div>
      </div>
    </div>

    <!-- 工单列表 -->
    <div class="table-container">
      <table class="data-table">
        <thead>
          <tr>
            <th>ID</th>
            <th>用户ID</th>
            <th>主题</th>
            <th>优先级</th>
            <th>状态</th>
            <th>创建时间</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="t in tickets" :key="t.id">
            <td>{{ t.id }}</td>
            <td>{{ t.user_id }}</td>
            <td>{{ t.subject }}</td>
            <td>
              <span :class="['status-badge', levelClass(t.level)]">{{ levelText(t.level) }}</span>
            </td>
            <td>
              <span :class="['status-badge', statusClass(t.status)]">{{ statusText(t.status) }}</span>
            </td>
            <td>{{ formatDate(t.created_at) }}</td>
            <td>
              <div class="action-buttons">
                <button class="btn-sm btn-ghost" @click="openReply(t)" title="回复">💬</button>
                <button class="btn-sm btn-ghost" @click="close(t)" v-if="t.status !== 2" title="关闭">🔒</button>
              </div>
            </td>
          </tr>
          <tr v-if="tickets.length === 0">
            <td colspan="7" class="empty-row">暂无工单</td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- 回复弹窗 -->
    <div v-if="showReply" class="modal-overlay" @click.self="closeReply">
      <div class="modal">
        <div class="modal-header">
          <h3>回复工单 #{{ currentTicket?.id }}</h3>
          <button class="close-btn" @click="closeReply">✕</button>
        </div>
        <div class="modal-body">
          <div class="ticket-info">
            <p><strong>主题：</strong>{{ currentTicket?.subject }}</p>
            <p><strong>用户ID：</strong>{{ currentTicket?.user_id }}</p>
          </div>
          <div class="form-group">
            <label>回复内容</label>
            <textarea v-model="replyMessage" rows="5" placeholder="输入回复内容..."></textarea>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn-secondary" @click="closeReply">取消</button>
          <button @click="submitReply">发送回复</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import adminApi from '@/api/admin'

const tickets = ref([])
const showReply = ref(false)
const currentTicket = ref(null)
const replyMessage = ref('')

const openCount = computed(() => tickets.value.filter(t => t.status === 0).length)
const answeredCount = computed(() => tickets.value.filter(t => t.status === 1).length)
const closedCount = computed(() => tickets.value.filter(t => t.status === 2).length)

const load = async () => {
  try {
    const res = await adminApi.getTickets()
    tickets.value = res.data || []
  } catch (e) {
    console.error('加载失败:', e)
  }
}

onMounted(() => { load() })

const levelText = (level) => ['低', '中', '高'][level] || '未知'
const levelClass = (level) => ['level-low', 'level-medium', 'level-high'][level] || ''
const statusText = (status) => ['待处理', '已回复', '已关闭'][status] || '未知'
const statusClass = (status) => ['status-open', 'status-answered', 'status-closed'][status] || ''

const formatDate = (ts) => {
  if (!ts) return '-'
  return new Date(ts * 1000).toLocaleString('zh-CN')
}

const openReply = (t) => {
  currentTicket.value = t
  replyMessage.value = ''
  showReply.value = true
}

const closeReply = () => {
  showReply.value = false
  currentTicket.value = null
}

const submitReply = async () => {
  if (!replyMessage.value.trim()) {
    alert('请输入回复内容')
    return
  }
  try {
    await adminApi.replyTicket({
      ticket_id: currentTicket.value.id,
      message: replyMessage.value
    })
    alert('回复成功')
    closeReply()
    await load()
  } catch (e) {
    alert(e.message || '回复失败')
  }
}

const close = async (t) => {
  if (!confirm('确定关闭此工单？')) return
  try {
    await adminApi.closeTicket(t.id)
    await load()
  } catch (e) {
    alert(e.message || '关闭失败')
  }
}
</script>

<style scoped>
.tickets-page {
  max-width: 1400px;
}

.page-header {
  margin-bottom: 24px;
}

.page-header h1 {
  font-size: 24px;
  margin-bottom: 4px;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 16px;
  margin-bottom: 24px;
}

@media (max-width: 768px) {
  .stats-grid {
    grid-template-columns: 1fr;
  }
}

.stat-card {
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  padding: 20px;
  display: flex;
  align-items: center;
  gap: 16px;
}

.stat-icon {
  font-size: 32px;
}

.stat-value {
  font-size: 24px;
  font-weight: 700;
}

.stat-label {
  font-size: 13px;
  color: var(--text-secondary);
}

.table-container {
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  overflow-x: auto;
}

.data-table {
  width: 100%;
  border-collapse: collapse;
}

.data-table th,
.data-table td {
  padding: 14px 16px;
  text-align: left;
  border-bottom: 1px solid var(--border-color);
}

.data-table th {
  background: var(--bg-color);
  font-weight: 600;
  font-size: 13px;
  color: var(--text-secondary);
  text-transform: uppercase;
}

.data-table tr:hover {
  background: var(--bg-color);
}

.status-badge {
  font-size: 12px;
  padding: 4px 10px;
  border-radius: 20px;
  font-weight: 500;
}

.level-low {
  background: rgba(34, 197, 94, 0.15);
  color: var(--success-color);
}

.level-medium {
  background: rgba(245, 158, 11, 0.15);
  color: var(--warning-color);
}

.level-high {
  background: rgba(239, 68, 68, 0.15);
  color: var(--error-color);
}

.status-open {
  background: rgba(59, 130, 246, 0.15);
  color: var(--primary-color);
}

.status-answered {
  background: rgba(34, 197, 94, 0.15);
  color: var(--success-color);
}

.status-closed {
  background: rgba(161, 161, 170, 0.15);
  color: var(--text-secondary);
}

.action-buttons {
  display: flex;
  gap: 4px;
}

.empty-row {
  text-align: center;
  color: var(--text-secondary);
  padding: 40px !important;
}

/* 弹窗样式 */
.modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.7);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  padding: 20px;
}

.modal {
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  width: 100%;
  max-width: 500px;
  max-height: 90vh;
  overflow-y: auto;
}

.modal-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 20px;
  border-bottom: 1px solid var(--border-color);
}

.modal-header h3 {
  font-size: 18px;
  font-weight: 600;
}

.close-btn {
  background: transparent;
  border: none;
  font-size: 18px;
  color: var(--text-secondary);
  cursor: pointer;
  padding: 4px;
}

.modal-body {
  padding: 20px;
}

.ticket-info {
  background: var(--bg-color);
  padding: 12px 16px;
  border-radius: var(--radius-md);
  margin-bottom: 16px;
}

.ticket-info p {
  margin: 4px 0;
  font-size: 14px;
}

.modal-body .form-group {
  margin-bottom: 16px;
}

.modal-body label {
  display: block;
  margin-bottom: 6px;
  font-size: 14px;
  font-weight: 500;
}

.modal-body textarea {
  resize: vertical;
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  padding: 16px 20px;
  border-top: 1px solid var(--border-color);
}

.modal-footer button {
  min-width: 80px;
}
</style>
