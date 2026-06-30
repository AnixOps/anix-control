<template>
  <div class="page-shell tickets-page">
    <div class="page-toolbar">
      <div>
        <h1>{{ t('user.tickets.title') }}</h1>
        <p>{{ t('user.tickets.subtitle') }}</p>
      </div>
      <button class="btn btn-primary" @click="showCreate = true">{{ t('user.tickets.submitTicket') }}</button>
    </div>

    <section class="section-panel stats-bar">
      <div class="stat-item">
        <span class="label">{{ t('user.tickets.active') }}</span>
        <span class="value">{{ activeCount }}</span>
      </div>
      <div class="stat-item">
        <span class="label">{{ t('user.tickets.resolved') }}</span>
        <span class="value">{{ resolvedCount }}</span>
      </div>
    </section>

    <div class="content-container">
      <div v-if="loading" class="loading-state">
        <div class="spinner"></div>
        <p>{{ t('user.tickets.loading') }}</p>
      </div>

      <div v-else-if="tickets.length === 0" class="empty-state">
        <div class="empty-icon">?</div>
        <p>{{ t('user.tickets.empty') }}</p>
        <button class="btn mt-4" @click="showCreate = true">{{ t('user.tickets.submitNow') }}</button>
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
              <span class="divider">/</span>
              <span class="ticket-date">{{ formatDate(ticket.updated_at) }} {{ t('user.tickets.updated') }}</span>
            </div>
          </div>
          <div class="ticket-arrow">></div>
        </div>
      </div>
    </div>

    <div v-if="showCreate" class="modal-overlay" @click.self="showCreate = false">
      <div class="modal">
        <div class="modal-header">
          <h3>{{ t('user.tickets.newTicketTitle') }}</h3>
          <button class="btn btn-ghost btn-sm close-btn normalized-close" :title="t('common.actions.close')" :aria-label="t('common.actions.close')" @click="showCreate = false">x</button>
          <button class="close-btn" @click="showCreate = false">×</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label>{{ t('common.labels.subject') }}</label>
            <input v-model="createForm.subject" type="text" :placeholder="t('common.labels.subject')">
          </div>
          <div class="form-group">
            <label>{{ t('common.labels.priority') }}</label>
            <select v-model="createForm.level">
              <option :value="0">{{ t('common.ticketPriority.low') }}</option>
              <option :value="1">{{ t('common.ticketPriority.medium') }}</option>
              <option :value="2">{{ t('common.ticketPriority.high') }}</option>
            </select>
          </div>
          <div class="form-group">
            <label>{{ t('common.labels.message') }}</label>
            <textarea v-model="createForm.message" rows="6" :placeholder="t('common.labels.message')"></textarea>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn" @click="showCreate = false">{{ t('common.actions.cancel') }}</button>
          <button class="btn btn-primary" :disabled="submitting" @click="submitCreate">
            {{ submitting ? t('common.states.loading') : t('common.actions.submit') }}
          </button>
        </div>
      </div>
    </div>

    <div v-if="showDetail" class="modal-overlay" @click.self="showDetail = false">
      <div class="modal modal-lg ticket-detail-modal">
        <div class="modal-header">
          <div class="header-top">
            <span :class="['status-badge', statusIndicator(detailTicket.status)]">{{ statusText(detailTicket.status) }}</span>
            <span class="ticket-id">{{ t('user.tickets.ticketId', { id: detailTicket.id }) }}</span>
          </div>
          <h3>{{ detailTicket.subject }}</h3>
          <button class="btn btn-ghost btn-sm close-btn normalized-close" :title="t('common.actions.close')" :aria-label="t('common.actions.close')" @click="showDetail = false">x</button>
          <button class="close-btn" @click="showDetail = false">×</button>
        </div>

        <div class="modal-body chat-container">
          <div class="messages-list">
            <div
              v-for="message in detailTicket.messages || []"
              :key="message.id"
              :class="['message-item', message.is_admin ? 'admin' : 'user']"
            >
              <div class="message-bubble">
                <div class="message-sender">{{ message.is_admin ? t('user.tickets.assistant') : t('user.tickets.me') }}</div>
                <div class="message-content">{{ message.message }}</div>
                <div class="message-time">{{ formatDateTime(message.created_at) }}</div>
              </div>
            </div>
          </div>
        </div>

        <div v-if="detailTicket.status !== 2" class="modal-footer footer-reply">
          <div class="reply-input-wrapper">
            <textarea
              v-model="replyMessage"
              rows="2"
              :placeholder="t('user.tickets.replyPlaceholder')"
              @keyup.ctrl.enter="submitReply"
            ></textarea>
            <div class="reply-actions">
              <button class="btn btn-ghost" @click="handleClose(detailTicket.id)">{{ t('common.actions.close') }}</button>
              <button class="btn btn-primary btn-sm" :disabled="replying" @click="submitReply">
                {{ replying ? t('common.states.loading') : t('common.actions.submit') }}
              </button>
            </div>
          </div>
        </div>
        <div v-else class="modal-footer">
          <div class="text-secondary">{{ t('user.tickets.closedHint') }}</div>
          <button class="btn" @click="showDetail = false">{{ t('user.tickets.closeWindow') }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { closeTicket, createTicket, getTicketDetail, getTickets, replyTicket } from '@/api/user'
import { useAppI18n } from '@/composables/useAppI18n'

const { t, formatDate, formatDateTime } = useAppI18n()
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

const activeCount = computed(() => tickets.value.filter((ticket) => ticket.status !== 2).length)
const resolvedCount = computed(() => tickets.value.filter((ticket) => ticket.status === 2).length)

async function fetchTickets() {
  loading.value = true
  try {
    const res = await getTickets()
    tickets.value = res.data || []
  } catch (err) {
    console.error('Failed to load tickets:', err)
  } finally {
    loading.value = false
  }
}

function statusText(status) {
  return [t('common.states.open'), t('common.states.answered'), t('common.states.closed')][status] || t('common.states.unknown')
}

function statusClass(status) {
  return ['status-open', 'status-answered', 'status-closed'][status] || ''
}

function statusIndicator(status) {
  return ['indicator-blue', 'indicator-green', 'indicator-gray'][status] || ''
}

async function submitCreate() {
  if (!createForm.value.subject || !createForm.value.message) {
    alert(t('common.messages.submitFailed'))
    return
  }
  submitting.value = true
  try {
    await createTicket(createForm.value)
    showCreate.value = false
    createForm.value = { subject: '', level: 1, message: '' }
    await fetchTickets()
  } catch (err) {
    alert(err.response?.data?.message || t('common.messages.submitFailed'))
  } finally {
    submitting.value = false
  }
}

async function viewDetail(ticket) {
  try {
    const res = await getTicketDetail(ticket.id)
    detailTicket.value = res.data || {}
    showDetail.value = true
  } catch {
    alert(t('common.messages.loadFailed'))
  }
}

async function submitReply() {
  if (!replyMessage.value.trim()) {
    return
  }
  replying.value = true
  try {
    await replyTicket(detailTicket.value.id, { message: replyMessage.value })
    replyMessage.value = ''
    const res = await getTicketDetail(detailTicket.value.id)
    detailTicket.value = res.data || {}
    await fetchTickets()
  } catch {
    alert(t('common.messages.submitFailed'))
  } finally {
    replying.value = false
  }
}

async function handleClose(id) {
  if (!confirm(t('common.messages.closeTicketConfirm'))) {
    return
  }
  try {
    await closeTicket(id)
    showDetail.value = false
    await fetchTickets()
  } catch {
    alert(t('common.messages.submitFailed'))
  }
}

onMounted(() => {
  fetchTickets()
})
</script>

<style scoped>
.stats-bar {
  display: flex;
  gap: 32px;
  padding: 16px 24px;
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
  box-shadow: var(--shadow-sm);
}

.ticket-card:hover {
  border-color: var(--primary-color);
  box-shadow: var(--shadow-md);
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

.status-open {
  color: var(--primary-color);
}

.status-open.status-dot {
  background: var(--primary-color);
}

.status-answered {
  color: var(--success-color);
}

.status-answered.status-dot {
  background: var(--success-color);
}

.status-closed {
  color: var(--text-secondary);
}

.status-closed.status-dot {
  background: var(--text-secondary);
}

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

.ticket-detail-modal {
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

.indicator-blue {
  background: rgba(59, 130, 246, 0.1);
  color: var(--primary-color);
}

.indicator-green {
  background: rgba(34, 197, 94, 0.1);
  color: var(--success-color);
}

.indicator-gray {
  background: rgba(161, 161, 170, 0.1);
  color: var(--text-secondary);
}

.chat-container {
  flex: 1;
  overflow-y: auto;
  padding: 20px;
  background: var(--bg-color);
  display: flex;
  flex-direction: column-reverse;
}

.messages-list {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.message-item {
  display: flex;
}

.message-item.user {
  justify-content: flex-end;
}

.message-item.admin {
  justify-content: flex-start;
}

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

.modal-lg {
  width: min(96vw, 760px);
}

.modal-header > .close-btn:not(.normalized-close) {
  display: none;
}

.loading-state,
.empty-state {
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
  to {
    transform: rotate(360deg);
  }
}

.empty-icon {
  font-size: 48px;
  margin-bottom: 16px;
}
</style>
