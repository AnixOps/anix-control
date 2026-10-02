<template>
  <div class="list-page tickets">
    <UiPageHeader v-if="!(narrow && selected)" :title="t('adminTickets.title')" :description="t('adminTickets.subtitle')" />

    <div v-if="loading && !loaded" class="tickets__pending">
      <UiSkeleton v-if="showSkeleton" variant="table-row" :rows="5" :columns="2" :label="t('adminTickets.loading')" />
    </div>
    <LoadError v-else-if="loadError && !tickets.length" :title="t('adminTickets.messages.fetchFailed')" :error="loadError" @retry="load" />
    <UiEmptyState
      v-else-if="!tickets.length"
      :icon="Inbox"
      :title="t('adminTickets.empty.title')"
      :description="t('adminTickets.empty.description')"
      heading-tag="h2"
      data-tickets-empty
    />

    <template v-else>
      <div v-if="!(narrow && selected)" class="tickets__toolbar">
        <UiSearchField
          v-model="search"
          class="list-page__search"
          :label="t('adminTickets.filters.search')"
          data-test="ticket-search"
        />
        <UiFilterChips v-model="statusFilter" :label="t('adminTickets.filters.label')" :options="statusChips" />
      </div>

      <div class="tickets-split" :class="{ 'is-narrow': narrow }">
        <nav v-if="!narrow || !selected" class="ticket-list" :aria-label="t('adminTickets.list')" data-ticket-list>
          <ul v-if="visibleTickets.length">
            <li v-for="ticket in visibleTickets" :key="ticket.id">
              <button
                type="button"
                class="ticket-list__item"
                :class="{ 'is-selected': ticket.id === selectedId }"
                :aria-current="ticket.id === selectedId ? 'true' : undefined"
                :data-ticket-id="ticket.id"
                @click="select(ticket)"
              >
                <span class="ticket-list__top">
                  <span class="ticket-list__subject">{{ ticket.subject || t('adminTickets.noSubject') }}</span>
                  <UiBadge :tone="statusTone(ticket.status)" :label="statusText(ticket.status)" />
                </span>
                <span class="ticket-list__meta">
                  {{ t('adminTickets.meta', { id: ticket.id, user: ticket.user_id, level: levelText(ticket.level) }) }} ·
                  <span :title="format.dateTime(ticket.updated_at || ticket.created_at)">{{ format.relativeTime(ticket.updated_at || ticket.created_at) }}</span>
                </span>
              </button>
            </li>
          </ul>
          <UiEmptyState
            v-else
            compact
            :icon="SearchX"
            :title="t('adminTickets.empty.noMatches')"
            heading-tag="h2"
          >
            <template #actions>
              <UiButton size="sm" data-clear-filters @click="clearFilters">{{ t('adminTickets.filters.clear') }}</UiButton>
            </template>
          </UiEmptyState>
        </nav>

        <section
          v-if="!narrow || selected"
          class="conversation"
          :aria-labelledby="selected ? 'admin-ticket-title' : undefined"
          data-ticket-conversation
        >
          <div v-if="!selected" class="conversation__placeholder">
            <UiIcon :icon="MessagesSquare" :size="48" />
            <p>{{ t('adminTickets.select') }}</p>
          </div>

          <template v-else>
            <header class="conversation__header">
              <UiButton v-if="narrow" variant="tertiary" size="sm" :icon="ChevronLeft" class="conversation__back" data-ticket-back @click="selectedId = null">
                {{ t('adminTickets.backToList') }}
              </UiButton>
              <div class="conversation__heading">
                <component :is="narrow ? 'h1' : 'h2'" id="admin-ticket-title" ref="titleRef" class="conversation__title" tabindex="-1">{{ selected.subject || t('adminTickets.noSubject') }}</component>
                <p class="conversation__meta">
                  <UiBadge :tone="statusTone(selected.status)" :label="statusText(selected.status)" />
                  <UiBadge :tone="levelTone(selected.level)" :dot="false" :label="t('adminTickets.priority', { level: levelText(selected.level) })" />
                </p>
              </div>
              <UiButton v-if="selected.status !== 2" variant="danger-soft" size="sm" :icon="Lock" data-ticket-close @click="closeTicket(selected)">{{ t('adminTickets.actions.closeTicket') }}</UiButton>
            </header>

            <div class="conversation__body">
              <dl class="ticket-facts">
                <div><dt>{{ t('adminTickets.facts.number') }}</dt><dd class="tabular-nums">#{{ selected.id }}</dd></div>
                <div><dt>{{ t('adminTickets.facts.user') }}</dt><dd class="tabular-nums">{{ selected.user_id }}</dd></div>
                <div><dt>{{ t('adminTickets.facts.created') }}</dt><dd>{{ format.dateTime(selected.created_at) }}</dd></div>
                <div><dt>{{ t('adminTickets.facts.updated') }}</dt><dd>{{ format.dateTime(selected.updated_at) }}</dd></div>
              </dl>
              <p class="conversation__note">
                <UiIcon :icon="Info" :size="16" />
                <span>{{ t('adminTickets.threadNote') }}</span>
              </p>
            </div>

            <p v-if="selected.status === 2" class="conversation__closed" data-ticket-closed>
              <UiIcon :icon="Lock" :size="16" />
              <span>{{ t('adminTickets.closedNote') }}</span>
            </p>
            <form v-else class="composer" novalidate data-ticket-reply-form @submit.prevent="submitReply">
              <div class="composer__quick" role="group" :aria-label="t('adminTickets.quick.label')">
                <UiButton
                  v-for="item in quickReplies"
                  :key="item.key"
                  size="sm"
                  variant="ghost"
                  :data-quick-reply="item.key"
                  @click="useQuickReply(item.text)"
                >{{ item.label }}</UiButton>
              </div>
              <div class="composer__row">
                <UiTextarea
                  v-model="replyMessage"
                  class="composer__field"
                  :rows="3"
                  :label="t('adminTickets.reply.content')"
                  :placeholder="t('adminTickets.reply.placeholder')"
                  :help="t('adminTickets.reply.hint')"
                  data-test="ticket-reply-input"
                  @keydown.enter.ctrl.prevent="submitReply"
                  @keydown.enter.meta.prevent="submitReply"
                />
                <UiButton type="submit" variant="primary" :icon="Send" class="composer__send" data-test="ticket-reply-submit" :loading="replying">{{ t('adminTickets.actions.sendReply') }}</UiButton>
              </div>
              <p v-if="replyError" class="form-error" role="alert">{{ replyError }}</p>
            </form>
          </template>
        </section>
      </div>
    </template>
  </div>
</template>

<script setup>
// 工单 (plan §8.2): an inbox — the queue on the left (search, status chips),
// the selected ticket on the right with quick replies and a close action;
// on phones the queue, then the ticket full width with a way back. Same
// endpoints as before: GET /admin/ticket, POST /admin/ticket/reply,
// POST /admin/ticket/:id/close. The admin API returns no message thread, so
// the panel shows the ticket's facts and the reply box.
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { ChevronLeft, Inbox, Info, Lock, MessagesSquare, SearchX, Send } from '@lucide/vue'
import adminApi from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import { useListQuery } from '@/composables/useListQuery'
import { NARROW_QUERY, useMediaQuery } from '@/composables/useMediaQuery'
import LoadError from '@/components/common/LoadError.vue'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiEmptyState from '@/ui/UiEmptyState.vue'
import UiFilterChips from '@/ui/UiFilterChips.vue'
import UiIcon from '@/ui/UiIcon.vue'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import UiSearchField from '@/ui/UiSearchField.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import UiTextarea from '@/ui/UiTextarea.vue'
import { useConfirm } from '@/ui/composables/useConfirm'
import { useDelayedLoading } from '@/ui/composables/useDelayedLoading'
import { useFormat } from '@/ui/composables/useFormat'
import { useToast } from '@/ui/composables/useToast'

const { t } = useAppI18n()
const format = useFormat()
const toast = useToast()
const confirm = useConfirm()
const narrow = useMediaQuery(NARROW_QUERY)

const tickets = ref([])
const loading = ref(false)
const loaded = ref(false)
const loadError = ref(null)
const showSkeleton = useDelayedLoading(loading)
// The queue's search and status chip live in the URL query (plan §9).
const listQuery = useListQuery()
const search = ref(listQuery.read('q'))
const statusFilter = ref(listQuery.read('status', { values: ['open', 'answered', 'closed'] }))
watch([search, statusFilter], () => listQuery.write({ q: search.value.trim(), status: statusFilter.value }))
const selectedId = ref(null)
const titleRef = ref(null)
const replyMessage = ref('')
const replyError = ref('')
const replying = ref(false)

const selected = computed(() => tickets.value.find(ticket => ticket.id === selectedId.value) || null)
const countOf = status => tickets.value.filter(ticket => Number(ticket.status) === status).length
const statusChips = computed(() => [
  { value: 'open', label: t('adminTickets.status.open'), count: countOf(0) },
  { value: 'answered', label: t('adminTickets.status.answered'), count: countOf(1) },
  { value: 'closed', label: t('adminTickets.status.closed'), count: countOf(2) }
])
const STATUS_BY_FILTER = { open: 0, answered: 1, closed: 2 }
// Open tickets first, then the most recently updated.
const visibleTickets = computed(() => {
  const needle = search.value.trim().toLowerCase().replace(/^#/, '')
  return tickets.value
    .filter(ticket => !statusFilter.value || Number(ticket.status) === STATUS_BY_FILTER[statusFilter.value])
    .filter(ticket => !needle
      || String(ticket.subject || '').toLowerCase().includes(needle)
      || String(ticket.id) === needle
      || String(ticket.user_id) === needle)
    .sort((a, b) => (Number(a.status === 2) - Number(b.status === 2)) || (Number(b.updated_at || b.created_at || 0) - Number(a.updated_at || a.created_at || 0)))
})

const quickReplies = computed(() => [
  { key: 'received', label: t('adminTickets.quick.receivedLabel'), text: t('adminTickets.quick.received') },
  { key: 'details', label: t('adminTickets.quick.detailsLabel'), text: t('adminTickets.quick.details') },
  { key: 'fixed', label: t('adminTickets.quick.fixedLabel'), text: t('adminTickets.quick.fixed') }
])

const readList = (res) => {
  if (res && typeof res.code === 'number' && res.code !== 0) throw new Error(res.msg || t('adminTickets.messages.fetchFailed'))
  return Array.isArray(res?.data) ? res.data : []
}

const load = async () => {
  loading.value = true
  try {
    tickets.value = readList(await adminApi.getTickets())
    loadError.value = null
  } catch (error) {
    loadError.value = error
    if (tickets.value.length) toast.error(error?.message || t('adminTickets.messages.fetchFailed'))
  } finally {
    loading.value = false
    loaded.value = true
  }
}

onMounted(load)

const levelText = (level) => {
  switch (Number(level)) {
    case 0: return t('adminTickets.levels.low')
    case 1: return t('adminTickets.levels.medium')
    case 2: return t('adminTickets.levels.high')
    default: return t('adminTickets.levels.unknown')
  }
}
const levelTone = level => (Number(level) === 2 ? 'danger' : (Number(level) === 1 ? 'warning' : 'neutral'))

const statusText = (status) => {
  switch (Number(status)) {
    case 0: return t('adminTickets.status.open')
    case 1: return t('adminTickets.status.answered')
    case 2: return t('adminTickets.status.closed')
    default: return t('adminTickets.status.unknown')
  }
}
const statusTone = status => ({ 0: 'info', 1: 'success', 2: 'neutral' })[Number(status)] || 'neutral'

const select = async (ticket) => {
  if (selectedId.value !== ticket.id) {
    replyMessage.value = ''
    replyError.value = ''
  }
  selectedId.value = ticket.id
  if (narrow.value) {
    await nextTick()
    titleRef.value?.focus?.()
  }
}

const clearFilters = () => {
  search.value = ''
  statusFilter.value = ''
}

const useQuickReply = (text) => {
  replyMessage.value = replyMessage.value.trim() ? `${replyMessage.value.trimEnd()}\n\n${text}` : text
  replyError.value = ''
}

const submitReply = async () => {
  if (replying.value || !selected.value) return
  replyError.value = ''
  if (!replyMessage.value.trim()) {
    replyError.value = t('adminTickets.messages.replyRequired')
    return
  }
  replying.value = true
  const ticketId = selected.value.id
  try {
    await adminApi.replyTicket({
      ticket_id: ticketId,
      message: replyMessage.value
    })
    replyMessage.value = ''
    toast.success(t('adminTickets.messages.replySuccess', { id: ticketId }))
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

defineExpose({ load, select, submitReply, closeTicket })
</script>

<style scoped>
.tickets__pending {
  min-height: 240px;
}

.tickets__toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3);
  align-items: center;
  margin-bottom: calc(-1 * var(--space-2));
}

/* ---- split view (same look as the user tickets page, U5) ----------------- */
.tickets-split {
  display: grid;
  grid-template-columns: 340px minmax(0, 1fr);
  height: min(720px, calc(100dvh - 300px));
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
  width: 100%;
  min-height: 0;
  padding: var(--space-4) var(--space-5);
  border: 0;
  border-radius: 0;
  background: transparent;
  color: var(--label-1);
  font: inherit;
  font-weight: inherit;
  line-height: inherit;
  text-align: left;
  white-space: normal;
  cursor: pointer;
  user-select: auto;
}

.ticket-list__item:hover {
  background: var(--fill-1);
}

.ticket-list__item:active {
  background: var(--fill-2);
}

.ticket-list__item:focus-visible {
  outline: var(--focus-ring);
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

/* ---- the selected ticket -------------------------------------------------- */
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
}

.conversation__body {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-5);
  min-height: 0;
  overflow-y: auto;
  padding: var(--space-6);
}

.ticket-facts {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(130px, 1fr));
  gap: var(--space-4);
  margin: 0;
  padding: var(--space-4) var(--space-5);
  border-radius: var(--radius-md);
  background: var(--bg-grouped);
}

.ticket-facts div {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}

.ticket-facts dt {
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

.ticket-facts dd {
  margin: 0;
}

.conversation__note,
.conversation__closed {
  display: flex;
  gap: var(--space-2);
  align-items: flex-start;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.conversation__note :deep(.ui-icon),
.conversation__closed :deep(.ui-icon) {
  margin-top: 2px;
}

.conversation__closed {
  padding: var(--space-4) var(--space-6);
  border-top: 1px solid var(--separator);
}

.composer {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4) var(--space-6);
  border-top: 1px solid var(--separator);
}

.composer__quick {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}

.composer__row {
  display: flex;
  gap: var(--space-3);
  align-items: flex-end;
}

.composer__field {
  flex: 1;
  min-width: 0;
}

.composer__field :deep(textarea) {
  max-height: 200px;
}

.composer__send {
  flex: none;
  margin-bottom: var(--space-6);
}

.composer .form-error {
  margin: 0;
}

@media (max-width: 833.98px) {
  .conversation__header,
  .conversation__body,
  .composer,
  .conversation__closed {
    padding-right: var(--space-4);
    padding-left: var(--space-4);
  }

  .composer__row {
    flex-direction: column;
    align-items: stretch;
  }

  .composer__send {
    margin-bottom: 0;
  }
}
</style>
