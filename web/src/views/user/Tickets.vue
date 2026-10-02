<template>
  <div class="tickets">
    <UiPageHeader v-if="!(narrow && selectedId)" :title="t('portal.tickets.title')" :description="t('portal.tickets.description')">
      <template v-if="list.state === 'ready' && list.items.length" #actions>
        <UiButton variant="primary" :icon="Plus" data-ticket-new @click="openCreate">{{ t('portal.tickets.new') }}</UiButton>
      </template>
    </UiPageHeader>

    <div v-if="list.state === 'loading'">
      <UiSkeleton v-if="showSkeleton" variant="table-row" :rows="4" :columns="2" />
    </div>
    <LoadError v-else-if="list.state === 'failed'" :title="t('portal.tickets.errors.load')" :error="list.error" @retry="loadList" />
    <UiEmptyState
      v-else-if="!list.items.length"
      :icon="MessagesSquare"
      :title="t('portal.tickets.empty')"
      :description="t('portal.tickets.emptyHint')"
      heading-tag="h2"
      data-tickets-empty
    >
      <template #actions>
        <UiButton variant="primary" :icon="Plus" data-ticket-new @click="openCreate">{{ t('portal.tickets.new') }}</UiButton>
      </template>
    </UiEmptyState>

    <div v-else class="tickets-split" :class="{ 'is-narrow': narrow }">
      <!-- The list (always on wide screens; on phones until a ticket is open). -->
      <nav v-if="!narrow || !selectedId" class="ticket-list" :aria-label="t('portal.tickets.list')" data-ticket-list>
        <ul>
          <li v-for="ticket in list.items" :key="ticket.id">
            <button
              type="button"
              class="ticket-list__item"
              :class="{ 'is-selected': String(ticket.id) === selectedId }"
              :aria-current="String(ticket.id) === selectedId ? 'true' : undefined"
              :data-ticket-id="ticket.id"
              @click="select(ticket.id)"
            >
              <span class="ticket-list__top">
                <span class="ticket-list__subject">{{ ticket.subject }}</span>
                <UiBadge :tone="statusTone(ticket.status)" :label="statusLabel(ticket.status)" />
              </span>
              <span class="ticket-list__meta">
                {{ t('portal.tickets.number', { id: ticket.id }) }} ·
                <span :title="format.dateTime(ticket.updated_at)">{{ t('portal.tickets.updated', { time: format.relativeTime(ticket.updated_at) }) }}</span>
              </span>
            </button>
          </li>
        </ul>
      </nav>

      <!-- The conversation. -->
      <section v-if="!narrow || selectedId" class="conversation" :aria-labelledby="selectedId ? 'conversation-title' : undefined" data-ticket-conversation>
        <div v-if="!selectedId" class="conversation__placeholder">
          <UiIcon :icon="MessagesSquare" :size="48" />
          <p>{{ t('portal.tickets.select') }}</p>
        </div>

        <template v-else>
          <div v-if="detail.state === 'loading'" class="conversation__loading">
            <UiSkeleton v-if="showDetailSkeleton" variant="text" :lines="6" />
          </div>
          <LoadError v-else-if="detail.state === 'failed'" :title="t('portal.tickets.errors.detail')" :error="detail.error" compact heading-tag="h2" @retry="loadDetail(selectedId)" />

          <template v-else-if="detail.ticket">
            <header class="conversation__header">
              <UiButton v-if="narrow" variant="tertiary" size="sm" :icon="ChevronLeft" class="conversation__back" data-ticket-back @click="select('')">
                {{ t('portal.tickets.backToList') }}
              </UiButton>
              <div class="conversation__heading">
                <component :is="narrow ? 'h1' : 'h2'" id="conversation-title" ref="conversationTitleRef" class="conversation__title" tabindex="-1">{{ detail.ticket.subject }}</component>
                <p class="conversation__meta">
                  <UiBadge :tone="statusTone(detail.ticket.status)" :label="statusLabel(detail.ticket.status)" />
                  <span>{{ t('portal.tickets.number', { id: detail.ticket.id }) }} · {{ priorityLabel(detail.ticket.level) }}</span>
                </p>
              </div>
              <UiButton v-if="!isClosed" variant="tertiary" size="sm" data-ticket-close @click="closeCurrent">{{ t('portal.tickets.close') }}</UiButton>
            </header>

            <ol ref="messagesRef" class="conversation__messages" tabindex="0" :aria-label="detail.ticket.subject" data-ticket-messages>
              <li
                v-for="message in detail.ticket.messages || []"
                :key="message.id"
                class="message"
                :class="message.is_admin ? 'message--support' : 'message--me'"
              >
                <span class="message__author">{{ message.is_admin ? t('portal.tickets.support') : t('portal.tickets.me') }}</span>
                <p class="message__bubble">{{ message.message }}</p>
                <time class="message__time" :datetime="isoTime(message.created_at)" :title="format.dateTime(message.created_at)">{{ format.relativeTime(message.created_at) }}</time>
              </li>
            </ol>

            <p v-if="isClosed" class="conversation__closed" data-ticket-closed>
              <UiIcon :icon="Lock" :size="16" />
              <span>{{ t('portal.tickets.closedNote') }}</span>
            </p>
            <form v-else class="composer" novalidate data-ticket-reply-form @submit.prevent="sendReply">
              <UiTextarea
                v-model="reply.text"
                :rows="2"
                :placeholder="t('portal.tickets.replyPlaceholder')"
                :aria-label="t('portal.tickets.reply')"
                :error="reply.error"
                :help="t('portal.tickets.sendHint')"
                class="composer__field"
                data-ticket-reply
                @keydown.enter.ctrl.prevent="sendReply"
                @keydown.enter.meta.prevent="sendReply"
              />
              <UiButton type="submit" variant="primary" :icon="Send" :loading="reply.busy" class="composer__send" data-ticket-send>{{ t('portal.tickets.send') }}</UiButton>
            </form>
          </template>
        </template>
      </section>
    </div>

    <UiSheet v-model:open="create.open" :title="t('portal.tickets.new')" :dismissible="!create.busy" data-ticket-create>
      <form id="ticket-create-form" class="create-form" novalidate @submit.prevent="submitCreate">
        <UiTextField
          v-model="create.subject"
          :label="t('portal.tickets.subject')"
          :placeholder="t('portal.tickets.subjectPlaceholder')"
          :error="create.errors.subject"
          maxlength="255"
          required
          data-ticket-subject
        />
        <UiField :label="t('portal.tickets.priority.label')" label-tag="span">
          <UiSegmentedControl v-model="create.level" :options="priorityOptions" :aria-label="t('portal.tickets.priority.label')" block data-ticket-priority />
        </UiField>
        <UiTextarea
          v-model="create.message"
          :label="t('portal.tickets.message')"
          :help="t('portal.tickets.messageHelp')"
          :error="create.errors.message"
          :rows="6"
          required
          data-ticket-message
        />
        <p v-if="create.error" class="create-form__error" role="alert" data-ticket-create-error>{{ create.error }}</p>
      </form>
      <template #footer="{ close }">
        <UiButton :disabled="create.busy" @click="close">{{ t('common.actions.cancel') }}</UiButton>
        <UiButton type="submit" form="ticket-create-form" variant="primary" :loading="create.busy" data-ticket-submit>{{ t('portal.tickets.submit') }}</UiButton>
      </template>
    </UiSheet>
  </div>
</template>

<script setup>
// 工单 (plan §8.1): the list with the conversation beside it on wide
// screens; on phones the list, then the conversation full width with a way
// back. A new ticket opens in a Sheet. ?ticket=<id> selects a ticket and
// ?new=1 opens the Sheet (links from 概览, 帮助中心 and 订阅). Data: GET/POST
// /user/ticket, GET /user/ticket/:id, POST …/reply and …/close.
import { computed, nextTick, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ChevronLeft, Lock, MessagesSquare, Plus, Send } from '@lucide/vue'
import { closeTicket, createTicket, getTicketDetail, getTickets, replyTicket } from '@/api/user'
import { useAppI18n } from '@/composables/useAppI18n'
import { useMediaQuery, NARROW_QUERY } from '@/composables/useMediaQuery'
import { listOf, panelErrorMessage, unwrapPanel } from '@/utils/panelResponse'
import LoadError from '@/components/common/LoadError.vue'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiEmptyState from '@/ui/UiEmptyState.vue'
import UiField from '@/ui/UiField.vue'
import UiIcon from '@/ui/UiIcon.vue'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import UiSegmentedControl from '@/ui/UiSegmentedControl.vue'
import UiSheet from '@/ui/UiSheet.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import UiTextField from '@/ui/UiTextField.vue'
import UiTextarea from '@/ui/UiTextarea.vue'
import { useConfirm } from '@/ui/composables/useConfirm'
import { useDelayedLoading } from '@/ui/composables/useDelayedLoading'
import { useFormat } from '@/ui/composables/useFormat'
import { useToast } from '@/ui/composables/useToast'

const { t } = useAppI18n()
const format = useFormat()
const toast = useToast()
const confirm = useConfirm()
const route = useRoute()
const router = useRouter()
const narrow = useMediaQuery(NARROW_QUERY)

const STATUS = ['open', 'answered', 'closed']

function statusLabel(status) {
  return t(`portal.tickets.status.${STATUS[status] || 'open'}`)
}

function statusTone(status) {
  return ['info', 'success', 'neutral'][status] || 'neutral'
}

function priorityLabel(level) {
  return t(`portal.tickets.priority.${['low', 'medium', 'high'][level] || 'medium'}`)
}

const priorityOptions = computed(() => [
  { value: 0, label: t('portal.tickets.priority.low') },
  { value: 1, label: t('portal.tickets.priority.medium') },
  { value: 2, label: t('portal.tickets.priority.high') }
])

function timeOf(value) {
  const n = typeof value === 'number' ? (value > 1e12 ? value : value * 1000) : Date.parse(value)
  return Number.isFinite(n) ? n : 0
}

function isoTime(value) {
  const n = timeOf(value)
  return n ? new Date(n).toISOString() : undefined
}

// --- list ----------------------------------------------------------------------
const list = reactive({ state: 'loading', items: [], error: null })
const showSkeleton = useDelayedLoading(computed(() => list.state === 'loading'))

async function loadList({ quiet = false } = {}) {
  if (!quiet) list.state = 'loading'
  try {
    list.items = [...listOf(unwrapPanel(await getTickets()))].sort((a, b) => timeOf(b.updated_at) - timeOf(a.updated_at))
    list.state = 'ready'
    list.error = null
  } catch (error) {
    if (quiet && list.state === 'ready') return
    list.error = error
    list.state = 'failed'
  }
}

// --- selection (?ticket=) -------------------------------------------------------
const selectedId = computed(() => String(route.query.ticket || ''))

function select(id) {
  router.replace({ query: { ...route.query, ticket: id ? String(id) : undefined } })
}

const detail = reactive({ state: 'idle', ticket: null, error: null })
const reply = reactive({ text: '', busy: false, error: '' })
const showDetailSkeleton = useDelayedLoading(computed(() => detail.state === 'loading'))
const isClosed = computed(() => detail.ticket?.status === 2)
const messagesRef = ref(null)
const conversationTitleRef = ref(null)

function scrollToEnd() {
  nextTick(() => {
    const el = messagesRef.value
    if (el) el.scrollTop = el.scrollHeight
  })
}

async function loadDetail(id, { quiet = false } = {}) {
  if (!id) {
    detail.state = 'idle'
    detail.ticket = null
    return
  }
  if (!quiet) detail.state = 'loading'
  try {
    detail.ticket = unwrapPanel(await getTicketDetail(id)) || null
    detail.state = 'ready'
    scrollToEnd()
  } catch (error) {
    detail.error = error
    detail.state = 'failed'
  }
}

watch(selectedId, async (id, previous) => {
  reply.text = ''
  reply.error = ''
  await loadDetail(id)
  // Opened by the user (not on first load): move focus to the conversation.
  if (id && previous !== undefined) nextTick(() => conversationTitleRef.value?.$el?.focus?.() || conversationTitleRef.value?.focus?.())
}, { immediate: true })

// --- reply ---------------------------------------------------------------------

async function sendReply() {
  if (reply.busy) return
  if (!reply.text.trim()) {
    reply.error = t('portal.tickets.errors.reply')
    return
  }
  reply.busy = true
  reply.error = ''
  try {
    unwrapPanel(await replyTicket(detail.ticket.id, { message: reply.text }))
    reply.text = ''
    await Promise.all([loadDetail(selectedId.value, { quiet: true }), loadList({ quiet: true })])
  } catch (error) {
    reply.error = t('portal.tickets.errors.send', { message: panelErrorMessage(error) })
  } finally {
    reply.busy = false
  }
}

async function closeCurrent() {
  const ticket = detail.ticket
  const closed = await confirm({
    title: t('portal.tickets.closeTitle', { subject: ticket.subject || `#${ticket.id}` }),
    message: t('portal.tickets.closeMessage'),
    confirmLabel: t('portal.tickets.closeConfirm'),
    onConfirm: async () => {
      try {
        unwrapPanel(await closeTicket(ticket.id))
      } catch (error) {
        throw new Error(panelErrorMessage(error))
      }
    }
  })
  if (!closed) return
  toast.success(t('portal.tickets.closed'))
  await Promise.all([loadDetail(selectedId.value, { quiet: true }), loadList({ quiet: true })])
}

// --- new ticket (?new=1) --------------------------------------------------------
const create = reactive({ open: false, busy: false, subject: '', level: 1, message: '', error: '', errors: { subject: '', message: '' } })

function openCreate() {
  Object.assign(create, { open: true, busy: false, error: '', errors: { subject: '', message: '' } })
}

watch(() => create.open, (open) => {
  if (!open && route.query.new) router.replace({ query: { ...route.query, new: undefined } })
})

async function submitCreate() {
  create.errors.subject = create.subject.trim() ? '' : t('portal.tickets.errors.subject')
  create.errors.message = create.message.trim() ? '' : t('portal.tickets.errors.message')
  if (create.errors.subject || create.errors.message) return
  create.busy = true
  create.error = ''
  try {
    const ticket = unwrapPanel(await createTicket({ subject: create.subject.trim(), level: create.level, message: create.message }))
    create.open = false
    create.subject = ''
    create.message = ''
    create.level = 1
    toast.success(t('portal.tickets.created'))
    await loadList({ quiet: true })
    if (ticket?.id) select(ticket.id)
  } catch (error) {
    create.error = t('portal.tickets.errors.create', { message: panelErrorMessage(error) })
  } finally {
    create.busy = false
  }
}

onMounted(async () => {
  await loadList()
  if (route.query.new) openCreate()
})
</script>

<style scoped>
.tickets {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
}

/* ---- split view --------------------------------------------------------------- */
.tickets-split {
  display: grid;
  grid-template-columns: 320px minmax(0, 1fr);
  height: min(720px, calc(100dvh - 260px));
  min-height: 480px;
  overflow: hidden;
  border-radius: var(--radius-lg);
  background: var(--bg-elevated);
  box-shadow: 0 0 0 0.5px var(--separator), var(--shadow-1);
}

.tickets-split.is-narrow {
  display: block;
  height: auto;
  min-height: 0;
  overflow: visible;
  border-radius: var(--radius-md);
}

.ticket-list {
  overflow-y: auto;
  border-right: 1px solid var(--separator);
  background: var(--bg-grouped);
}

.is-narrow .ticket-list {
  border-right: 0;
  border-radius: inherit;
  background: var(--bg-elevated);
}

.ticket-list ul {
  list-style: none;
}

.ticket-list li + li {
  border-top: 1px solid var(--separator);
}

.ticket-list__item {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  align-items: stretch;
  /* Undo the global button rule (style.css): rows, not pills. */
  justify-content: flex-start;
  border-radius: 0;
  font-weight: inherit;
  line-height: inherit;
  white-space: normal;
  user-select: auto;

  width: 100%;
  padding: var(--space-4) var(--space-5);
  border: 0;
  background: transparent;
  color: var(--label-1);
  font: inherit;
  text-align: left;
  cursor: pointer;
}

.ticket-list__item:hover {
  background: var(--fill-1);
}

.ticket-list__item:active {
  background: var(--fill-2);
}

.ticket-list__item:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: -2px;
}

.ticket-list__item.is-selected {
  background: var(--accent-soft);
}

.ticket-list__item.is-selected .ticket-list__meta {
  color: color-mix(in srgb, var(--label-2) 70%, var(--label-1));
}

.ticket-list__top {
  display: flex;
  gap: var(--space-2);
  align-items: flex-start;
  justify-content: space-between;
}

.ticket-list__subject {
  min-width: 0;
  font-weight: var(--weight-semibold);
  overflow-wrap: anywhere;
}

.ticket-list__meta {
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

/* ---- conversation ------------------------------------------------------------- */
.conversation {
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
}

.is-narrow .conversation {
  min-height: 60vh;
}

.conversation__placeholder {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-3);
  align-items: center;
  justify-content: center;
  padding: var(--space-8);
  color: var(--label-2);
  text-align: center;
}

.conversation__placeholder :deep(.ui-icon) {
  color: var(--label-3);
}

.conversation__loading {
  padding: var(--space-6);
}

.conversation__header {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2) var(--space-4);
  align-items: flex-start;
  justify-content: space-between;
  padding: var(--space-5) var(--space-6);
  border-bottom: 1px solid var(--separator);
}

.conversation__back {
  flex-basis: 100%;
  justify-content: flex-start;
  margin-left: calc(var(--space-2) * -1);
}

.conversation__heading {
  flex: 1 1 240px;
  min-width: 0;
}

.conversation__title {
  font-size: var(--type-title-3-size);
  font-weight: var(--type-title-3-weight);
  line-height: var(--type-title-3-line);
  overflow-wrap: anywhere;
}

.conversation__title:focus {
  outline: none;
}

.conversation__meta {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
  margin-top: var(--space-2);
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.conversation__messages {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-5);
  min-height: 0;
  overflow-y: auto;
  padding: var(--space-6);
  list-style: none;
}

.conversation__messages:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: -2px;
}

.message {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  max-width: min(560px, 85%);
}

.message--me {
  align-self: flex-end;
  align-items: flex-end;
}

.message__author,
.message__time {
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

.message__bubble {
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-md);
  line-height: var(--type-body-line);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.message--support .message__bubble {
  border-top-left-radius: var(--radius-xs);
  background: var(--fill-1);
}

.message--me .message__bubble {
  border-top-right-radius: var(--radius-xs);
  background: var(--accent-soft);
}

.conversation__closed {
  display: flex;
  gap: var(--space-2);
  align-items: center;
  padding: var(--space-4) var(--space-6);
  border-top: 1px solid var(--separator);
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.composer {
  display: flex;
  gap: var(--space-3);
  align-items: flex-start;
  padding: var(--space-4) var(--space-6);
  border-top: 1px solid var(--separator);
}

.composer__field {
  flex: 1;
  min-width: 0;
}

.composer__field :deep(textarea) {
  min-height: 44px;
  max-height: 160px;
}

.composer__send {
  flex: none;
}

/* ---- new ticket ----------------------------------------------------------------- */
.create-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
}

.create-form__error {
  color: var(--danger);
  font-size: var(--type-callout-size);
}

@media (max-width: 833px) {
  .tickets {
    gap: var(--space-6);
  }

  .conversation__header,
  .conversation__messages,
  .composer,
  .conversation__closed {
    padding-right: var(--space-4);
    padding-left: var(--space-4);
  }

  .composer {
    flex-direction: column;
    align-items: stretch;
  }

  .message {
    max-width: 92%;
  }
}
</style>
