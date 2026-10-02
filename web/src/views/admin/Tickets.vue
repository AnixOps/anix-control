<template>
  <div class="page-shell tickets-page">
    <div class="page-toolbar">
      <div>
        <h1>{{ t('adminTickets.title') }}</h1>
        <p>{{ t('adminTickets.subtitle') }}</p>
      </div>
    </div>

    <section class="metrics-grid">
      <article class="section-panel metric-card">
        <span class="metric-code primary">OPN</span>
        <strong>{{ openCount }}</strong>
        <span>{{ t('adminTickets.stats.open') }}</span>
      </article>
      <article class="section-panel metric-card">
        <span class="metric-code success">ANS</span>
        <strong>{{ answeredCount }}</strong>
        <span>{{ t('adminTickets.stats.answered') }}</span>
      </article>
      <article class="section-panel metric-card">
        <span class="metric-code muted">CLD</span>
        <strong>{{ closedCount }}</strong>
        <span>{{ t('adminTickets.stats.closed') }}</span>
      </article>
    </section>

    <section class="section-panel data-panel">
      <div class="table-wrap">
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
                  class="btn btn-sm"
                  :title="t('adminTickets.actions.reply')"
                  :aria-label="t('adminTickets.actions.reply')"
                  @click="openReply(ticket)"
                >
                  {{ t('adminTickets.actions.reply') }}
                </button>
                <button
                  v-if="ticket.status !== 2"
                  class="btn btn-sm"
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
    </section>

    <UiSheet
      :open="showReply"
      :title="t('adminTickets.replyModal.title', { id: currentTicket?.id ?? '-' })"
      :dismissible="!replying"
      @update:open="value => { if (!value) closeReply() }"
    >
      <div class="ticket-info">
        <p><strong>{{ t('adminTickets.replyModal.subject') }}</strong>{{ currentTicket?.subject }}</p>
        <p><strong>{{ t('adminTickets.replyModal.userId') }}</strong>{{ currentTicket?.user_id }}</p>
      </div>
      <div class="form-group">
        <label for="admin-ticket-reply">{{ t('adminTickets.replyModal.content') }}</label>
        <textarea
          id="admin-ticket-reply"
          v-model="replyMessage"
          rows="5"
          data-test="ticket-reply-input"
          :placeholder="t('adminTickets.replyModal.placeholder')"
          :aria-invalid="replyError ? 'true' : undefined"
          :aria-describedby="replyError ? 'admin-ticket-reply-error' : undefined"
        ></textarea>
        <p v-if="replyError" id="admin-ticket-reply-error" class="form-error" role="alert">{{ replyError }}</p>
      </div>
      <template #footer="{ close }">
        <UiButton :disabled="replying" @click="close">{{ t('common.actions.cancel') }}</UiButton>
        <UiButton variant="primary" data-test="ticket-reply-submit" :loading="replying" @click="submitReply">{{ t('adminTickets.actions.sendReply') }}</UiButton>
      </template>
    </UiSheet>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import adminApi from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import { UiButton, UiSheet, useConfirm, useToast } from '@/ui'

const { t, formatDateTime } = useAppI18n()

const tickets = ref([])
const showReply = ref(false)
const currentTicket = ref(null)
const replyMessage = ref('')
const replyError = ref('')
const replying = ref(false)
const toast = useToast()
const confirm = useConfirm()

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
  replyError.value = ''
  showReply.value = true
}

const closeReply = () => {
  if (replying.value) return
  showReply.value = false
  currentTicket.value = null
}

const submitReply = async () => {
  if (replying.value) return
  replyError.value = ''
  if (!replyMessage.value.trim()) {
    replyError.value = t('adminTickets.messages.replyRequired')
    return
  }

  replying.value = true
  try {
    await adminApi.replyTicket({
      ticket_id: currentTicket.value.id,
      message: replyMessage.value
    })
    replying.value = false
    toast.success(t('adminTickets.messages.replySuccess'))
    closeReply()
    await load()
  } catch (error) {
    replyError.value = error.message || t('adminTickets.messages.replyFailed')
  } finally {
    replying.value = false
  }
}

const closeTicket = async (ticket) => {
  // There is no reopen endpoint: closing is final, so ask first.
  const confirmed = await confirm({
    title: t('adminTickets.confirm.closeTitle', { id: ticket.id, subject: ticket.subject || '' }),
    message: t('adminTickets.confirm.closeMessage'),
    confirmLabel: t('adminTickets.confirm.closeAction'),
    onConfirm: async () => {
      try {
        await adminApi.closeTicket(ticket.id)
      } catch (error) {
        throw new Error(error.message || t('adminTickets.messages.closeFailed'))
      }
    }
  })
  if (!confirmed) return
  toast.success(t('adminTickets.messages.closed', { id: ticket.id }))
  await load()
}
</script>

<style scoped>
.metrics-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 16px;
}

.metric-card {
  padding: 18px;
  display: grid;
  gap: 6px;
}

.metric-card strong {
  font-size: 30px;
  line-height: 1.1;
}

.metric-card span:last-child {
  color: var(--text-secondary);
  font-size: 13px;
}

.metric-code {
  width: fit-content;
  min-width: 44px;
  padding: 4px 10px;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 800;
}

.metric-code.primary { background: var(--primary-soft); color: var(--primary-color); }
.metric-code.success { background: rgba(22, 163, 74, 0.08); color: var(--success-color); }
.metric-code.muted { background: var(--surface-muted); color: var(--text-secondary); }

.data-panel {
  padding: 0;
}

.status-badge {
  display: inline-flex;
  align-items: center;
}

.level-low {
  background: rgba(22, 163, 74, 0.08);
  color: var(--success-color);
}

.level-medium {
  background: rgba(217, 119, 6, 0.08);
  color: var(--warning-color);
}

.level-high {
  background: rgba(220, 38, 38, 0.08);
  color: var(--error-color);
}

.status-open {
  background: var(--primary-soft);
  color: var(--primary-color);
}

.status-answered {
  background: rgba(22, 163, 74, 0.08);
  color: var(--success-color);
}

.status-closed {
  background: var(--surface-muted);
  color: var(--text-secondary);
}

.action-buttons {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

.empty-row {
  padding: 40px !important;
}

.ticket-info {
  background: var(--surface-muted);
  padding: 12px 16px;
  border-radius: var(--radius-md);
}

.ticket-info p {
  margin: 4px 0;
  font-size: 14px;
}

.form-group label {
  display: block;
  margin-bottom: var(--space-2);
}

.form-group textarea {
  resize: vertical;
}

.form-error {
  margin: var(--space-2) 0 0;
  color: var(--danger);
}
</style>

