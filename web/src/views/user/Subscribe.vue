<template>
  <div class="subscribe">
    <UiPageHeader :title="t('portal.subscribe.title')" :description="t('portal.subscribe.description')" />

    <div v-if="sub.loading.value && !sub.summary.value" class="subscribe__slot">
      <div v-if="showSkeleton" class="subscribe-card" data-subscribe-skeleton>
        <UiSkeleton variant="text" :lines="4" />
        <span class="subscribe-card__qr-bone" aria-hidden="true" />
      </div>
    </div>

    <LoadError v-else-if="sub.error.value" :title="t('portal.subscribe.loadFailed')" :error="sub.error.value" @retry="sub.load()" />

    <UiEmptyState
      v-else-if="!sub.link.value"
      :icon="Link2Off"
      :title="t('portal.subscribe.missing')"
      :description="isCommercial ? t('portal.home.next.commercial') : t('portal.home.next.community')"
      heading-tag="h2"
      data-subscribe-missing
    >
      <template v-if="isCommercial" #actions>
        <UiButton :as="RouterLink" to="/user/plans" variant="primary">{{ t('portal.home.buyPlan') }}</UiButton>
      </template>
    </UiEmptyState>

    <template v-else>
      <section class="subscribe-card" :aria-label="t('portal.subscribe.link')" data-subscribe-link>
        <div class="subscribe-card__main">
          <UiSelect
            v-if="sub.domains.value.length > 1"
            v-model="sub.selectedDomain.value"
            :label="t('portal.subscribe.domain')"
            :options="sub.domains.value"
            size="md"
            class="subscribe-card__domain"
            data-subscribe-domain
          />
          <UiCopyField
            id="subscribe-link"
            :value="sub.link.value"
            :label="t('portal.subscribe.link')"
            :help="t('portal.subscribe.linkHelp')"
            :copy-label="t('portal.subscribe.copy')"
            variant="primary"
            class="subscribe-card__copy"
          />
          <dl class="subscribe-card__facts">
            <div class="subscribe-fact">
              <dt>{{ t('portal.subscribe.facts.remaining') }}</dt>
              <dd>{{ shortBytes(sub.remaining.value) }}</dd>
            </div>
            <div class="subscribe-fact">
              <dt>{{ t('portal.subscribe.facts.used') }}</dt>
              <dd>{{ shortBytes(sub.used.value) }}</dd>
            </div>
            <div class="subscribe-fact">
              <dt>{{ t('portal.subscribe.facts.expires') }}</dt>
              <dd>{{ sub.expiresAt.value > 0 ? format.date(sub.expiresAt.value) : t('portal.home.facts.never') }}</dd>
            </div>
          </dl>
        </div>
        <UiQrCode :value="sub.link.value" :label="t('portal.subscribe.qr.label')" :caption="t('portal.subscribe.qr.caption')" data-subscribe-qr />
      </section>

      <section class="subscribe-section" aria-labelledby="subscribe-clients-title" data-subscribe-clients>
        <div class="subscribe-section__head">
          <h2 id="subscribe-clients-title" class="subscribe-section__title">{{ t('portal.subscribe.clients.title') }}</h2>
          <span class="subscribe-section__hint">{{ t('portal.subscribe.clients.hint') }}</span>
        </div>
        <ul class="clients">
          <li v-for="client in clients" :key="client.id" class="client" :data-client="client.id">
            <span class="client__glyph" aria-hidden="true">{{ client.glyph }}</span>
            <div class="client__text">
              <span class="client__name">{{ client.name }}</span>
              <span class="client__platforms">{{ client.platformsKey ? t(client.platformsKey) : client.platforms }}</span>
            </div>
            <UiButton
              v-if="client.href"
              :href="client.href"
              size="sm"
              block
              :aria-label="t('portal.subscribe.clients.importLabel', { name: client.name })"
              data-client-import
            >
              {{ t('portal.subscribe.clients.import') }}
            </UiButton>
            <UiButton
              v-else
              size="sm"
              block
              :icon="copiedClient === client.id ? Check : Copy"
              :aria-label="t('portal.subscribe.clients.copyLabel', { name: client.name })"
              data-client-copy
              @click="copyClientLink(client)"
            >
              {{ copiedClient === client.id ? t('ui.actions.copied') : t('portal.subscribe.clients.copy') }}
            </UiButton>
          </li>
        </ul>
      </section>

      <section class="subscribe-section" aria-labelledby="subscribe-formats-title" data-subscribe-formats>
        <div class="subscribe-section__head">
          <h2 id="subscribe-formats-title" class="subscribe-section__title">{{ t('portal.subscribe.formats.title') }}</h2>
          <span class="subscribe-section__hint">{{ t('portal.subscribe.formats.description') }}</span>
        </div>
        <div class="formats">
          <UiSelect
            v-model="selectedFormat"
            :label="t('portal.subscribe.formats.format')"
            :options="formatOptions"
            size="md"
            class="formats__select"
            data-subscribe-format
          />
          <UiCopyField :value="formatLink" :label="t('portal.subscribe.link')" size="md" class="formats__link" data-subscribe-format-link />
          <div>
            <UiButton :icon="FileText" data-subscribe-preview @click="openPreview">{{ t('portal.subscribe.formats.preview') }}</UiButton>
          </div>
        </div>
      </section>

      <section class="subscribe-section" aria-labelledby="subscribe-danger-title" data-subscribe-danger>
        <h2 id="subscribe-danger-title" class="subscribe-section__title subscribe-section__title--danger">{{ t('portal.subscribe.danger.title') }}</h2>
        <div class="danger-zone">
          <div class="danger-zone__row">
            <div class="danger-zone__text">
              <h3 class="danger-zone__title">{{ t('portal.subscribe.danger.resetTitle') }}</h3>
              <p class="danger-zone__description">{{ t('portal.subscribe.danger.resetDescription') }}</p>
            </div>
            <UiButton
              v-if="!reset.confirming"
              ref="resetOpenRef"
              variant="danger-soft"
              :icon="RefreshCcw"
              data-reset-open
              @click="openReset"
            >
              {{ t('portal.subscribe.danger.resetAction') }}
            </UiButton>
          </div>
          <div v-if="reset.confirming" class="danger-zone__confirm" role="group" aria-labelledby="reset-question" data-reset-confirm @keydown.esc="cancelReset">
            <p id="reset-question" class="danger-zone__question">{{ t('portal.subscribe.danger.confirmTitle') }}</p>
            <p class="danger-zone__description">{{ t('portal.subscribe.danger.confirmDescription') }}</p>
            <p v-if="reset.error" class="danger-zone__error" role="alert">{{ reset.error }}</p>
            <div class="danger-zone__actions">
              <UiButton ref="resetCancelRef" :disabled="reset.busy" data-reset-cancel @click="cancelReset">{{ t('portal.subscribe.danger.cancel') }}</UiButton>
              <UiButton variant="danger" :loading="reset.busy" data-reset-confirm-button @click="submitReset">{{ t('portal.subscribe.danger.confirm') }}</UiButton>
            </div>
          </div>
          <p v-if="reset.ticketId" class="danger-zone__done" role="status" data-reset-done>
            <UiIcon :icon="CircleCheck" :size="16" />
            <span>{{ t('portal.subscribe.danger.submitted') }}</span>
            <RouterLink :to="{ path: '/user/tickets', query: { ticket: String(reset.ticketId) } }">{{ t('portal.subscribe.danger.viewTicket') }}</RouterLink>
          </p>
        </div>
      </section>
    </template>

    <UiSheet v-model:open="preview.open" size="lg" :title="t('portal.subscribe.formats.previewTitle', { format: formatName(preview.format) })" :description="t('portal.subscribe.formats.previewDescription')" data-subscribe-preview-sheet>
      <UiSkeleton v-if="preview.loading" variant="text" :lines="8" />
      <p v-else-if="preview.error" class="preview__error" role="alert">{{ preview.error }}</p>
      <p v-else-if="!preview.content" class="preview__empty">{{ t('portal.subscribe.formats.previewEmpty') }}</p>
      <pre v-else class="preview__code" tabindex="0">{{ preview.content }}</pre>
      <template #footer="{ close }">
        <UiButton :disabled="!preview.content" :icon="Download" @click="downloadPreview">{{ t('portal.subscribe.formats.download') }}</UiButton>
        <UiButton :disabled="!preview.content" :icon="preview.copied ? Check : Copy" @click="copyPreview">
          {{ preview.copied ? t('ui.actions.copied') : t('portal.subscribe.formats.copyContent') }}
        </UiButton>
        <UiButton variant="primary" @click="close">{{ t('ui.actions.close') }}</UiButton>
      </template>
    </UiSheet>
  </div>
</template>

<script setup>
// 订阅 (plan §8.1): the link with copy and its QR code, one-click import for
// the clients the kernel serves a format for (subscriptionClients.js), every
// other format with copy and preview behind 「其他格式」, and the danger zone.
// Users cannot reset their own link (only POST /admin/users/:id/reset-subscribe
// exists), so 「申请重置」 confirms in place and files a ticket for the
// administrator through POST /user/ticket.
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { Check, CircleCheck, Copy, Download, FileText, Link2Off, RefreshCcw } from '@lucide/vue'
import { createTicket } from '@/api/user'
import { useAppI18n } from '@/composables/useAppI18n'
import { useEdition } from '@/composables/useEdition'
import { useUserSubscription } from '@/composables/useUserSubscription'
import { panelErrorMessage, unwrapPanel } from '@/utils/panelResponse'
import LoadError from '@/components/common/LoadError.vue'
import { SUBSCRIPTION_CLIENTS, SUBSCRIPTION_FORMATS } from './subscriptionClients'
import UiButton from '@/ui/UiButton.vue'
import UiCopyField from '@/ui/UiCopyField.vue'
import UiEmptyState from '@/ui/UiEmptyState.vue'
import UiIcon from '@/ui/UiIcon.vue'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import UiQrCode from '@/ui/UiQrCode.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiSheet from '@/ui/UiSheet.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import { copyText } from '@/ui/composables/useClipboard'
import { useDelayedLoading } from '@/ui/composables/useDelayedLoading'
import { useFormat } from '@/ui/composables/useFormat'
import { useToast } from '@/ui/composables/useToast'

const { t } = useAppI18n()
const format = useFormat()
const toast = useToast()
const { isCommercial } = useEdition()
const sub = useUserSubscription()
const showSkeleton = useDelayedLoading(sub.loading)

// One decimal, without a trailing ".0" (200 GB, not 200.0 GB).
function shortBytes(value) {
  return format.bytes(value, { precision: 1, empty: '0 B' }).replace(/[.,]0(?= )/, '')
}

// Clients ------------------------------------------------------------------
const profileName = computed(() => (typeof window !== 'undefined' ? window.location.host : 'AnixOps'))
const clients = computed(() => SUBSCRIPTION_CLIENTS.map(client => ({
  ...client,
  href: client.importUrl ? client.importUrl(sub.linkFor(client.format), profileName.value) : ''
})))

const copiedClient = ref('')
let copiedTimer = null

async function copyClientLink(client) {
  if (await copyText(sub.linkFor(client.format))) {
    copiedClient.value = client.id
    toast.success(t('portal.subscribe.clients.copied', { name: client.name }))
    if (copiedTimer) clearTimeout(copiedTimer)
    copiedTimer = setTimeout(() => { copiedClient.value = '' }, 2000)
  } else {
    toast.error(t('portal.subscribe.clients.copyFailed'))
  }
}

// Other formats -------------------------------------------------------------
const selectedFormat = ref('auto')
const formatOptions = computed(() => SUBSCRIPTION_FORMATS.map(item => ({ value: item.value, label: t(`portal.subscribe.formats.names.${item.nameKey}`) })))
const formatLink = computed(() => sub.linkFor(selectedFormat.value))

function formatName(value) {
  const item = SUBSCRIPTION_FORMATS.find(entry => entry.value === value)
  return item ? t(`portal.subscribe.formats.names.${item.nameKey}`) : value
}

const preview = reactive({ open: false, loading: false, format: '', content: '', error: '', copied: false })

// The preview reads the subscription from this page's own origin (a
// configured subscription domain may not allow this page to fetch it).
async function openPreview() {
  Object.assign(preview, { open: true, loading: true, format: selectedFormat.value, content: '', error: '', copied: false })
  try {
    const host = typeof window !== 'undefined' ? window.location.host : ''
    const response = await fetch(sub.linkFor(selectedFormat.value, host))
    if (!response.ok) {
      preview.error = t('portal.subscribe.formats.previewFailed', { reason: `HTTP ${response.status}` })
      return
    }
    preview.content = await response.text()
  } catch (error) {
    preview.error = t('portal.subscribe.formats.previewFailed', { reason: panelErrorMessage(error) })
  } finally {
    preview.loading = false
  }
}

async function copyPreview() {
  preview.copied = await copyText(preview.content)
  if (preview.copied) setTimeout(() => { preview.copied = false }, 2000)
}

function downloadPreview() {
  const ext = SUBSCRIPTION_FORMATS.find(item => item.value === preview.format)?.ext || 'txt'
  const url = URL.createObjectURL(new Blob([preview.content], { type: 'text/plain' }))
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = `subscription-${preview.format || 'auto'}.${ext}`
  anchor.click()
  URL.revokeObjectURL(url)
}

// Danger zone ---------------------------------------------------------------
const reset = reactive({ confirming: false, busy: false, error: '', ticketId: null })
const resetOpenRef = ref(null)
const resetCancelRef = ref(null)

function focusComponent(component) {
  const el = component?.$el
  ;(el?.matches?.('button, a') ? el : el?.querySelector?.('button, a'))?.focus()
}

function openReset() {
  reset.confirming = true
  reset.error = ''
  nextTick(() => focusComponent(resetCancelRef.value))
}

function cancelReset() {
  if (reset.busy) return
  reset.confirming = false
  reset.error = ''
  nextTick(() => focusComponent(resetOpenRef.value))
}

async function submitReset() {
  reset.busy = true
  reset.error = ''
  try {
    const ticket = unwrapPanel(await createTicket({
      subject: t('portal.subscribe.danger.ticketSubject'),
      level: 2,
      message: t('portal.subscribe.danger.ticketMessage')
    }))
    reset.ticketId = ticket?.id || ticket?.ID || 0
    reset.confirming = false
    nextTick(() => focusComponent(resetOpenRef.value))
  } catch (error) {
    reset.error = t('portal.subscribe.danger.failed', { message: panelErrorMessage(error) })
  } finally {
    reset.busy = false
  }
}

onMounted(() => { sub.load() })
onBeforeUnmount(() => { if (copiedTimer) clearTimeout(copiedTimer) })
</script>

<style scoped>
.subscribe {
  display: flex;
  flex-direction: column;
  gap: var(--space-12);
}

.subscribe__slot {
  min-height: 240px;
}

/* ---- link card --------------------------------------------------------------- */
.subscribe-card {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: var(--space-8);
  align-items: start;
  padding: var(--space-8);
  border-radius: var(--radius-lg);
  background: var(--bg-elevated);
  box-shadow: 0 0 0 0.5px var(--separator), var(--shadow-1);
}

.subscribe-card__main {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
  min-width: 0;
}

.subscribe-card__domain {
  max-width: 360px;
}

.subscribe-card__copy :deep(input) {
  color: var(--label-2);
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
  text-overflow: ellipsis;
}

.subscribe-card__qr-bone {
  width: 168px;
  height: 168px;
  border-radius: var(--radius-md);
  background: var(--fill-1);
}

.subscribe-card__facts {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: var(--space-4);
  margin: 0;
}

.subscribe-fact dt {
  margin-bottom: var(--space-1);
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.subscribe-fact dd {
  margin: 0;
  font-size: var(--type-title-3-size);
  font-weight: var(--weight-semibold);
  font-variant-numeric: tabular-nums;
}

/* ---- sections --------------------------------------------------------------- */
.subscribe-section__head {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1) var(--space-3);
  align-items: baseline;
  justify-content: space-between;
  margin-bottom: var(--space-4);
}

.subscribe-section__title {
  font-size: var(--type-title-3-size);
  font-weight: var(--type-title-3-weight);
  line-height: var(--type-title-3-line);
}

.subscribe-section__title--danger {
  margin-bottom: var(--space-4);
  color: var(--danger);
}

.subscribe-section__hint {
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

/* ---- clients ---------------------------------------------------------------- */
.clients {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: var(--space-4);
  list-style: none;
}

.client {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  align-items: stretch;
  padding: var(--space-5);
  border-radius: var(--radius-md);
  background: var(--bg-elevated);
  box-shadow: 0 0 0 0.5px var(--separator), var(--shadow-1);
  transition:
    box-shadow var(--dur-toggle) var(--ease-standard),
    transform var(--dur-toggle) var(--ease-standard);
}

.client:hover {
  box-shadow: 0 0 0 0.5px var(--separator), var(--shadow-2);
  transform: translateY(-1px);
}

.client__glyph {
  display: grid;
  align-self: flex-start;
  width: 40px;
  height: 40px;
  place-items: center;
  border-radius: var(--radius-sm);
  background: var(--fill-1);
  color: var(--label-1);
  font-size: var(--type-body-size);
  font-weight: var(--weight-bold);
}

.client__text {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-0-5);
}

.client__name {
  font-weight: var(--weight-semibold);
}

.client__platforms {
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

/* ---- other formats ---------------------------------------------------------- */
.formats {
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
  padding: var(--space-6);
  border-radius: var(--radius-md);
  background: var(--bg-elevated);
  box-shadow: 0 0 0 0.5px var(--separator), var(--shadow-1);
}

.formats__select {
  max-width: 320px;
}

.formats__link :deep(input) {
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
}

/* ---- danger zone -------------------------------------------------------------- */
.danger-zone {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  padding: var(--space-6);
  border-radius: var(--radius-md);
  background: var(--bg-elevated);
  box-shadow: 0 0 0 1px var(--danger-soft), var(--shadow-1);
}

.danger-zone__row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-6);
  align-items: center;
  justify-content: space-between;
}

.danger-zone__text {
  flex: 1 1 280px;
}

.danger-zone__title {
  font-size: var(--type-body-size);
  font-weight: var(--weight-semibold);
}

.danger-zone__description {
  margin-top: var(--space-1);
  color: var(--label-2);
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
}

.danger-zone__confirm {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-4);
  border-radius: var(--radius-sm);
  background: var(--danger-soft);
  animation: danger-in var(--dur-toggle) var(--ease-standard);
}

.danger-zone__question {
  font-weight: var(--weight-semibold);
}

/* On the tinted fill, secondary text needs to be darker to keep 4.5:1. */
.danger-zone__confirm .danger-zone__description {
  color: color-mix(in srgb, var(--label-2) 60%, var(--label-1));
}

.danger-zone__error {
  color: color-mix(in srgb, var(--danger) 80%, var(--label-1));
  font-size: var(--type-callout-size);
}

.danger-zone__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3);
  justify-content: flex-end;
  margin-top: var(--space-2);
}

.danger-zone__done {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
  color: var(--label-1);
  font-size: var(--type-callout-size);
}

.danger-zone__done :deep(.ui-icon) {
  color: var(--success);
}

/* ---- preview ------------------------------------------------------------------ */
.preview__code {
  overflow: auto;
  max-height: 60vh;
  margin: 0;
  padding: var(--space-4);
  border-radius: var(--radius-sm);
  background: var(--bg-grouped);
  font-family: var(--font-mono);
  font-size: var(--type-caption-size);
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-all;
}

.preview__code:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: 2px;
}

.preview__empty {
  color: var(--label-2);
}

.preview__error {
  color: var(--danger);
}

@keyframes danger-in {
  from {
    opacity: 0;
  }
}

@media (prefers-reduced-motion: reduce) {
  .client {
    transition: none;
  }

  .client:hover {
    transform: none;
  }

  .danger-zone__confirm {
    animation: none;
  }
}

@media (max-width: 833px) {
  .subscribe {
    gap: var(--space-8);
  }

  .subscribe-card {
    grid-template-columns: minmax(0, 1fr);
    justify-items: center;
    padding: var(--space-6);
  }

  .subscribe-card__main {
    width: 100%;
  }

  .subscribe-card__facts {
    gap: var(--space-3);
  }

  .subscribe-fact dd {
    font-size: var(--type-body-size);
    white-space: nowrap;
  }

  .clients {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--space-3);
  }

  .client {
    padding: var(--space-4);
  }

  .client:hover {
    transform: none;
  }

  .danger-zone__actions :deep(.ui-button) {
    flex: 1 1 auto;
  }
}
</style>
