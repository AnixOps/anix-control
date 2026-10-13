<template>
  <NodeNotice
    v-if="notice"
    :tone="notice.tone"
    class="certificate-banner"
    :role="notice.tone === 'danger' ? 'alert' : 'status'"
    data-testid="panel-certificate-banner"
    :data-state="notice.state"
  >
    <strong class="certificate-banner__title">{{ notice.title }}</strong>
    <span v-if="notice.reason" class="certificate-banner__line" data-testid="panel-certificate-reason">{{ notice.reason }}</span>
    <span v-if="notice.fix" class="certificate-banner__line" data-testid="panel-certificate-fix">{{ notice.fix }}</span>
    <span v-if="retryNote" class="certificate-banner__line" data-testid="panel-certificate-retry-note">{{ retryNote }}</span>
    <template v-if="notice.retry" #actions>
      <UiButton size="sm" :loading="retrying" data-testid="panel-certificate-retry" @click="retry">
        {{ t('shell.admin.certificate.retry') }}
      </UiButton>
    </template>
  </NodeNotice>
</template>

<script setup>
// The banner of the admin shell for the certificates Control serves itself
// (GET /api/v5/panel/status): it shows only while one is failing (Control
// serves its self-signed fallback, so nodes cannot join and browsers warn),
// still being requested, failing to renew, or not publicly trusted, with the
// reason and the fix. Nothing shows while every certificate is valid, when
// Control manages none (TLS in front of it), or when the status cannot be
// read.
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { getPanelStatus, retryPanelCertificates } from '@/api/kernel'
import { useAppI18n } from '@/composables/useAppI18n'
import { useFormat } from '@/ui/composables/useFormat'
import UiButton from '@/ui/UiButton.vue'
import NodeNotice from '@/views/admin/nodes/NodeNotice.vue'

// How often the status is read while a banner shows.
const REFRESH_MS = 60_000

const { t, te } = useAppI18n()
const format = useFormat()
const status = ref(null)
const retrying = ref(false)
const retryNote = ref('')
let timer = null

const PROBLEMS = new Set([
  'dns_unresolved', 'dns_elsewhere', 'unreachable', 'challenge_failed', 'rate_limited', 'ca_unreachable',
  'ca_error', 'file_unreadable', 'expired', 'name_mismatch', 'untrusted', 'quota'
])

// The certificate that needs attention most: failed, then pending, then a
// failing renewal or one that is not publicly trusted.
function worst(certificates) {
  const rank = certificate => {
    if (certificate.state === 'failed') return 4
    if (certificate.state === 'pending') return 3
    if (certificate.renewal_failing) return 2
    if (certificate.state === 'valid' && certificate.publicly_trusted === false) return 1
    return 0
  }
  return [...certificates].sort((a, b) => rank(b) - rank(a)).find(certificate => rank(certificate) > 0) || null
}

function fixText(certificate) {
  const key = `shell.admin.certificate.fixes.${certificate.problem}`
  if (PROBLEMS.has(certificate.problem) && te(key)) return t(key, { name: certificate.name })
  return certificate.fix || ''
}

const notice = computed(() => {
  const certificates = status.value?.certificates
  if (!Array.isArray(certificates) || certificates.length === 0) return null
  const certificate = worst(certificates)
  if (!certificate) return null
  const name = certificate.name || status.value.domain || ''
  const base = 'shell.admin.certificate'
  if (certificate.state === 'pending') {
    return { state: 'pending', tone: 'info', title: t(`${base}.pending`, { name }), reason: '', fix: '', retry: false }
  }
  const reason = certificate.last_error ? t(`${base}.reason`, { reason: certificate.last_error }) : ''
  if (certificate.state === 'failed') {
    return { state: 'failed', tone: 'danger', title: t(`${base}.failed`, { name }), reason, fix: fixText(certificate), retry: true }
  }
  if (certificate.renewal_failing) {
    const date = certificate.not_after ? format.dateTime(certificate.not_after) : ''
    return { state: 'renewal_failing', tone: 'warning', title: t(`${base}.renewalFailing`, { name, date }), reason, fix: fixText(certificate), retry: true }
  }
  // A CA's certificate that does not verify (a staging directory) would only
  // come back the same; files are read again.
  return { state: 'untrusted', tone: 'warning', title: t(`${base}.untrusted`, { name }), reason, fix: fixText(certificate), retry: certificate.source === 'files' }
})

async function load() {
  try {
    const answer = await getPanelStatus()
    status.value = answer?.certificate || null
  } catch {
    // An older Control, a staff account or a network error: no banner.
    status.value = null
  }
  schedule()
}

function schedule() {
  clearTimeout(timer)
  timer = null
  if (notice.value) timer = setTimeout(load, REFRESH_MS)
}

async function retry() {
  retrying.value = true
  retryNote.value = ''
  try {
    const answer = await retryPanelCertificates()
    const waiting = Object.values(answer?.retry?.waiting || {})
    if (waiting.length > 0) {
      retryNote.value = t('shell.admin.certificate.retryWaiting', { date: format.dateTime(waiting[0]) })
    } else {
      retryNote.value = t('shell.admin.certificate.retryStarted')
    }
    if (answer?.certificate) status.value = answer.certificate
  } catch {
    retryNote.value = t('shell.admin.certificate.retryFailed')
  } finally {
    retrying.value = false
  }
  clearTimeout(timer)
  timer = setTimeout(load, 5000)
}

onMounted(load)
onBeforeUnmount(() => clearTimeout(timer))
</script>

<style scoped>
.certificate-banner {
  margin-bottom: var(--space-4);
}

.certificate-banner :deep(.node-notice__body) {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}

.certificate-banner__line {
  color: var(--label-2);
  overflow-wrap: anywhere;
}
</style>
