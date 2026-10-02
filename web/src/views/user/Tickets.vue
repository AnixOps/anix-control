<template>
  <div class="page-shell tickets-page">
    <div class="page-toolbar">
      <div>
        <h1>{{ t('user.tickets.title') }}</h1>
        <p>{{ t('user.tickets.subtitle') }}</p>
      </div>
      <button class="btn btn-primary" data-test="ticket-create-button" @click="showCreate = true">{{ t('user.tickets.submitTicket') }}</button>
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
        <button class="btn mt-4" data-test="ticket-create-button" @click="showCreate = true">{{ t('user.tickets.submitNow') }}</button>
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

    <UiSheet v-model:open="showCreate" :title="t('user.tickets.newTicketTitle')" :dismissible="!submitting">
      <div class="ticket-form">
        <div class="form-group">
          <label for="ticket-subject">{{ t('common.labels.subject') }}</label>
          <input id="ticket-subject" v-model="createForm.subject" type="text" :placeholder="t('common.labels.subject')">
        </div>
        <div class="form-group">
          <label for="ticket-level">{{ t('common.labels.priority') }}</label>
          <select id="ticket-level" v-model="createForm.level">
            <option :value="0">{{ t('common.ticketPriority.low') }}</option>
            <option :value="1">{{ t('common.ticketPriority.medium') }}</option>
            <option :value="2">{{ t('common.ticketPriority.high') }}</option>
          </select>
        </div>
        <div class="form-group">
          <label for="ticket-message">{{ t('common.labels.message') }}</label>
          <textarea id="ticket-message" v-model="createForm.message" rows="6" :placeholder="t('common.labels.message')"></textarea>
        </div>
        <p v-if="createError" class="form-error" role="alert" data-test="ticket-create-error">{{ createError }}</p>
      </div>
      <template #footer="{ close }">
        <UiButton :disabled="submitting" @click="close">{{ t('common.actions.cancel') }}</UiButton>
        <UiButton variant="primary" data-test="ticket-submit-button" :loading="submitting" @click="submitCreate">
          {{ t('common.actions.submit') }}
        </UiButton>
      </template>
    </UiSheet>

    <UiSheet v-model:open="showDetail" size="lg" class="ticket-detail-sheet" :title="detailTicket.subject || ''">
      <template #description>
        <span class="header-top">
          <span :class="['status-badge', statusIndicator(detailTicket.status)]">{{ statusText(detailTicket.status) }}</span>
          <span class="ticket-id">{{ t('user.tickets.ticketId', { id: detailTicket.id }) }}</span>
        </span>
      </template>

      <div class="chat-container">
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

      <template #footer="{ close }">
        <div v-if="detailTicket.status !== 2" class="reply-input-wrapper">
          <label class="visually-hidden" for="ticket-reply">{{ t('user.tickets.replyLabel') }}</label>
          <textarea
            id="ticket-reply"
            v-model="replyMessage"
            rows="2"
            :placeholder="t('user.tickets.replyPlaceholder')"
            @keyup.ctrl.enter="submitReply"
          ></textarea>
          <p v-if="replyError" class="form-error" role="alert" data-test="ticket-reply-error">{{ replyError }}</p>
          <div class="reply-actions">
            <UiButton variant="tertiary" data-test="ticket-close-button" @click="handleClose(detailTicket)">{{ t('user.tickets.closeTicket') }}</UiButton>
            <UiButton variant="primary" data-test="ticket-reply-button" :loading="replying" @click="submitReply">
              {{ t('user.tickets.sendReply') }}
            </UiButton>
          </div>
        </div>
        <div v-else class="closed-footer">
          <div class="text-secondary">{{ t('user.tickets.closedHint') }}</div>
          <UiButton @click="close">{{ t('user.tickets.closeWindow') }}</UiButton>
        </div>
      </template>
    </UiSheet>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { closeTicket, createTicket, getTicketDetail, getTickets, replyTicket } from '@/api/user'
import { useAppI18n } from '@/composables/useAppI18n'
import { UiButton, UiSheet, useConfirm, useToast } from '@/ui'

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
const createError = ref('')
const replyError = ref('')
const toast = useToast()
const confirm = useConfirm()

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
  createError.value = ''
  if (!createForm.value.subject || !createForm.value.message) {
    createError.value = t('user.tickets.fillSubjectMessage')
    return
  }
  submitting.value = true
  try {
    await createTicket(createForm.value)
    showCreate.value = false
    createForm.value = { subject: '', level: 1, message: '' }
    toast.success(t('user.tickets.created'))
    await fetchTickets()
  } catch (err) {
    createError.value = err.response?.data?.message || t('common.messages.submitFailed')
  } finally {
    submitting.value = false
  }
}

async function viewDetail(ticket) {
  try {
    const res = await getTicketDetail(ticket.id)
    detailTicket.value = res.data || {}
    replyError.value = ''
    showDetail.value = true
  } catch {
    toast.error(t('user.tickets.loadDetailFailed'))
  }
}

async function submitReply() {
  if (!replyMessage.value.trim()) {
    return
  }
  replying.value = true
  replyError.value = ''
  try {
    await replyTicket(detailTicket.value.id, { message: replyMessage.value })
    replyMessage.value = ''
    const res = await getTicketDetail(detailTicket.value.id)
    detailTicket.value = res.data || {}
    await fetchTickets()
  } catch (err) {
    replyError.value = err?.response?.data?.message || t('common.messages.submitFailed')
  } finally {
    replying.value = false
  }
}

async function handleClose(ticket) {
  const closed = await confirm({
    title: t('user.tickets.closeConfirmTitle', { subject: ticket.subject || `#${ticket.id}` }),
    message: t('user.tickets.closeConfirmMessage'),
    confirmLabel: t('user.tickets.closeTicket'),
    onConfirm: async () => {
      try {
        await closeTicket(ticket.id)
      } catch (err) {
        throw new Error(err?.response?.data?.message || t('common.messages.submitFailed'))
      }
    }
  })
  if (!closed) return
  showDetail.value = false
  toast.success(t('user.tickets.closed'))
  await fetchTickets()
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

.ticket-form {
  display: grid;
}

.form-error {
  margin: 0;
  color: var(--danger);
  font-size: var(--type-callout-size);
}

.header-top {
  display: flex;
  align-items: center;
  gap: var(--space-3);
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
  display: flex;
  flex: 1;
  flex-direction: column-reverse;
  margin: calc(var(--space-6) * -1);
  padding: var(--space-5);
  background: var(--bg-grouped);
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
  background: var(--accent-fill);
  color: var(--on-accent);
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

.reply-actions,
.closed-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: var(--space-3);
  flex-wrap: wrap;
}

.closed-footer {
  width: 100%;
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
