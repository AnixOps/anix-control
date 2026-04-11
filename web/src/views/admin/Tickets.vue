<template>
  <div class="tickets-page">
    <div class="page-header">
      <h1>{{ t('adminTickets.title') }}</h1>
      <p class="text-secondary">{{ t('adminTickets.subtitle') }}</p>
    </div>

    <div class="stats-grid">
      <div class="stat-card">
        <div class="stat-icon" aria-hidden="true">{{ t('adminTickets.icons.open') }}</div>
        <div class="stat-info">
          <div class="stat-value">{{ openCount }}</div>
          <div class="stat-label">{{ t('adminTickets.stats.open') }}</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon" aria-hidden="true">{{ t('adminTickets.icons.answered') }}</div>
        <div class="stat-info">
          <div class="stat-value">{{ answeredCount }}</div>
          <div class="stat-label">{{ t('adminTickets.stats.answered') }}</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon" aria-hidden="true">{{ t('adminTickets.icons.closed') }}</div>
        <div class="stat-info">
          <div class="stat-value">{{ closedCount }}</div>
          <div class="stat-label">{{ t('adminTickets.stats.closed') }}</div>
        </div>
      </div>
    </div>

    <div class="table-container">
      <table class="data-table">
        <thead>
          <tr>
            <th>{{ t('adminTickets.table.id') }}</th>
            <th>{{ t('adminTickets.table.userId') }}</th>
            <th>{{ t('adminTickets.table.subject') }}</th>
            <th>{{ t('adminTickets.table.priority') }}</th>
            <th>{{ t('adminTickets.table.status') }}</th>
            <th>{{ t('adminTickets.table.createdAt') }}</th>
            <th>{{ t('adminTickets.table.actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="ticket in tickets" :key="ticket.id">
            <td>{{ ticket.id }}</td>
            <td>{{ ticket.user_id }}</td>
            <td>{{ ticket.subject }}</td>
            <td>
              <span :class="['status-badge', levelClass(ticket.level)]">
                {{ levelText(ticket.level) }}
              </span>
            </td>
            <td>
              <span :class="['status-badge', statusClass(ticket.status)]">
                {{ statusText(ticket.status) }}
              </span>
            </td>
            <td>{{ formatTicketDate(ticket.created_at) }}</td>
            <td>
              <div class="action-buttons">
                <button
                  class="btn-sm btn-ghost"
                  :title="t('adminTickets.actions.reply')"
                  :aria-label="t('adminTickets.actions.reply')"
                  @click="openReply(ticket)"
                >
                  {{ t('adminTickets.actions.reply') }}
                </button>
                <button
                  v-if="ticket.status !== 2"
                  class="btn-sm btn-ghost"
                  :title="t('adminTickets.actions.closeTicket')"
                  :aria-label="t('adminTickets.actions.closeTicket')"
                  @click="closeTicket(ticket)"
                >
                  {{ t('adminTickets.actions.closeTicket') }}
                </button>
              </div>
            </td>
          </tr>
          <tr v-if="tickets.length === 0">
            <td colspan="7" class="empty-row">{{ t('adminTickets.empty.noData') }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <div v-if="showReply" class="modal-overlay" @click.self="closeReply">
      <div class="modal">
        <div class="modal-header">
          <h3>{{ t('adminTickets.replyModal.title', { id: currentTicket?.id ?? '-' }) }}</h3>
          <button
            class="close-btn"
            :aria-label="t('common.actions.close')"
            :title="t('common.actions.close')"
            @click="closeReply"
          >
            ×
          </button>
        </div>
        <div class="modal-body">
          <div class="ticket-info">
            <p><strong>{{ t('adminTickets.replyModal.subject') }}</strong>{{ currentTicket?.subject }}</p>
            <p><strong>{{ t('adminTickets.replyModal.userId') }}</strong>{{ currentTicket?.user_id }}</p>
          </div>
          <div class="form-group">
            <label>{{ t('adminTickets.replyModal.content') }}</label>
            <textarea
              v-model="replyMessage"
              rows="5"
              :placeholder="t('adminTickets.replyModal.placeholder')"
            ></textarea>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn-secondary" @click="closeReply">{{ t('common.actions.cancel') }}</button>
          <button @click="submitReply">{{ t('adminTickets.actions.sendReply') }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import adminApi from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'

const { t, formatDateTime } = useAppI18n()

const tickets = ref([])
const showReply = ref(false)
const currentTicket = ref(null)
const replyMessage = ref('')

const openCount = computed(() => tickets.value.filter((ticket) => ticket.status === 0).length)
const answeredCount = computed(() => tickets.value.filter((ticket) => ticket.status === 1).length)
const closedCount = computed(() => tickets.value.filter((ticket) => ticket.status === 2).length)

const load = async () => {
  try {
    const res = await adminApi.getTickets()
    tickets.value = res.data || []
  } catch (error) {
    console.error(t('adminTickets.messages.fetchFailed'), error)
  }
}

onMounted(() => {
  load()
})

const levelText = (level) => {
  switch (Number(level)) {
    case 0:
      return t('adminTickets.levels.low')
    case 1:
      return t('adminTickets.levels.medium')
    case 2:
      return t('adminTickets.levels.high')
    default:
      return t('adminTickets.levels.unknown')
  }
}

const levelClass = (level) => ['level-low', 'level-medium', 'level-high'][level] || ''

const statusText = (status) => {
  switch (Number(status)) {
    case 0:
      return t('adminTickets.status.open')
    case 1:
      return t('adminTickets.status.answered')
    case 2:
      return t('adminTickets.status.closed')
    default:
      return t('adminTickets.status.unknown')
  }
}

const statusClass = (status) => ['status-open', 'status-answered', 'status-closed'][status] || ''

const formatTicketDate = (ts) => {
  if (!ts) return '-'
  return formatDateTime(ts)
}

const openReply = (ticket) => {
  currentTicket.value = ticket
  replyMessage.value = ''
  showReply.value = true
}

const closeReply = () => {
  showReply.value = false
  currentTicket.value = null
}

const submitReply = async () => {
  if (!replyMessage.value.trim()) {
    window.alert(t('adminTickets.messages.replyRequired'))
    return
  }

  try {
    await adminApi.replyTicket({
      ticket_id: currentTicket.value.id,
      message: replyMessage.value
    })
    window.alert(t('adminTickets.messages.replySuccess'))
    closeReply()
    await load()
  } catch (error) {
    window.alert(error.message || t('adminTickets.messages.replyFailed'))
  }
}

const closeTicket = async (ticket) => {
  if (!window.confirm(t('adminTickets.messages.closeConfirm'))) return

  try {
    await adminApi.closeTicket(ticket.id)
    await load()
  } catch (error) {
    window.alert(error.message || t('adminTickets.messages.closeFailed'))
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

