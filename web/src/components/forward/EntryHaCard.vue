<template>
  <UiCard :title="t('forwardDns.card.title')" :description="t('forwardDns.card.description')" data-testid="forward-entry-ha">
    <template #actions>
      <span class="ha-actions">
        <UiBadge v-if="status" :tone="stateTone(state)" :label="stateLabel" data-testid="forward-entry-ha-state" />
        <UiButton v-if="status?.binding" size="sm" :icon="Pencil" @click="router.push(editPath)">{{ t('forwardDns.card.edit') }}</UiButton>
        <UiButton
          v-if="status?.binding"
          size="sm"
          variant="danger-soft"
          :icon="Unlink"
          :disabled="canDelete === false"
          :title="canDelete === false ? t('forwardDns.card.unbindSuperOnly') : undefined"
          data-testid="forward-entry-ha-unbind"
          @click="openUnbind"
        >
          {{ t('forwardDns.card.unbind') }}
        </UiButton>
      </span>
    </template>

    <UiErrorState v-if="error && !status" compact :title="t('forwardDns.card.loadFailed')" :error="error" @retry="refresh" />
    <UiSkeleton v-else-if="!status" variant="text" :lines="3" />

    <template v-else-if="state === 'unbound'">
      <div class="ha-empty">
        <p class="ha-text">{{ status.entry_hostname ? t('forwardDns.card.unboundHost', { host: status.entry_hostname }) : t('forwardDns.card.unboundNoHost') }}</p>
        <UiButton size="sm" variant="primary" :icon="Link2" @click="router.push(editPath)">{{ status.entry_hostname ? t('forwardDns.card.bind') : t('forwardDns.card.setHostname') }}</UiButton>
      </div>
    </template>

    <template v-else>
      <p class="ha-text" :class="{ 'fwd-note is-warning': warn }" role="status">{{ stateText }}</p>

      <div v-if="status.last_error" class="fwd-note is-danger ha-error" role="alert" data-testid="forward-entry-ha-error">
        <span class="ha-error__head"><AlertCircle :size="16" aria-hidden="true" /> {{ t('forwardDns.card.lastError') }}<span v-if="lastErrorAt" class="fwd-muted"> · {{ fmt.relativeTime(lastErrorAt) }}</span></span>
        <code class="ha-error__text">{{ status.last_error }}</code>
        <span v-if="nextAttempt" class="fwd-muted">{{ t('forwardDns.card.nextAttempt', { when: fmt.relativeTime(nextAttempt) }) }}</span>
      </div>
      <p v-else-if="nextAttempt" class="fwd-muted ha-text">{{ t('forwardDns.card.nextAttempt', { when: fmt.relativeTime(nextAttempt) }) }}</p>

      <div class="ha-layout">
        <UiGroupedList>
          <UiGroupedListRow :label="t('forwardDns.card.hostname')" :value="status.entry_hostname || '—'" />
          <UiGroupedListRow :label="t('forwardDns.card.mode')" :value="modeText" />
          <UiGroupedListRow :label="t('forwardDns.card.zone')" :value="binding.zone || '—'" />
          <UiGroupedListRow :label="t('forwardDns.card.ttl')" :value="t('forwardDns.card.ttlValue', { n: num(binding.ttl) || DEFAULT_TTL })" />
          <UiGroupedListRow :label="t('forwardDns.card.published')" :value="publishedAt ? fmt.relativeTime(publishedAt) : t('forwardDns.card.never')" />
          <UiGroupedListRow :label="t('forwardDns.card.evaluated')" :value="evaluatedAt ? fmt.relativeTime(evaluatedAt) : '—'" />
        </UiGroupedList>

        <div class="ha-records">
          <h3 class="ha-subhead">{{ t('forwardDns.card.records') }}</h3>
          <table class="ha-table">
            <thead>
              <tr>
                <th scope="col">{{ t('forwardDns.card.type') }}</th>
                <th scope="col">{{ t('forwardDns.card.publishedValues') }}</th>
                <th scope="col">{{ t('forwardDns.card.desiredValues') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="record in records" :key="record.type">
                <th scope="row" class="fwd-mono">{{ recordTypeLabel(record.type) }}</th>
                <td>
                  <span v-if="record.published.length" class="ha-values">
                    <span v-for="value in record.published" :key="value" class="fwd-mono" :class="{ 'ha-stale': !record.desired.includes(value) && record.desired.length }">{{ value }}</span>
                  </span>
                  <span v-else class="fwd-muted">{{ t('forwardDns.card.none') }}</span>
                </td>
                <td>
                  <span v-if="record.desired.length" class="ha-values">
                    <span v-for="value in record.desired" :key="value" class="fwd-mono">{{ value }}</span>
                  </span>
                  <span v-else class="fwd-muted">{{ t('forwardDns.card.keepPublished') }}</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <div v-if="cnameTarget" class="ha-cname" data-testid="forward-entry-ha-cname">
        <p class="ha-text">{{ t('forwardDns.binding.cnameInstruction', { host: status.entry_hostname || '—' }) }}</p>
        <UiCopyField :value="cnameTarget" size="md" :label="t('forwardDns.binding.cnameTarget')" :copy-label="t('forwardDns.binding.copy')" />
      </div>

      <div v-if="nodes.length" class="ha-nodes">
        <h3 class="ha-subhead">{{ t('forwardDns.card.entries') }}</h3>
        <ul class="fwd-list">
          <li v-for="node in nodes" :key="node.node_ref" class="fwd-list__item ha-node" :data-node-ref="node.node_ref">
            <span class="fwd-cell-stack">
              <span class="ha-node__name">{{ nodeName(node.node_ref) }}</span>
              <span class="fwd-mono ha-wrap">{{ (node.addresses || []).join(', ') || t('forwardDns.card.noAddress') }}</span>
            </span>
            <span class="ha-chips">
              <UiBadge :tone="reasonTone(node.reason)" :label="reasonLabel(node.reason)" />
              <UiBadge :tone="node.in_rotation ? 'success' : 'neutral'" :dot="false" :label="node.in_rotation ? t('forwardDns.card.inRotation') : t('forwardDns.card.outOfRotation')" />
              <span v-if="streak(node)" class="fwd-muted">{{ streak(node) }}</span>
            </span>
          </li>
        </ul>
      </div>
      <p class="fwd-muted ha-text">{{ t('forwardDns.card.timing') }}</p>
    </template>

    <UiDialog v-model:open="unbindOpen" :title="t('forwardDns.unbind.title', { host: binding.record_name || status?.entry_hostname || '' })" :description="t('forwardDns.unbind.description')" :dismissible="!unbinding">
      <UiCheckbox v-model="purge" :label="t('forwardDns.unbind.purge')" :description="t('forwardDns.unbind.purgeHelp')" data-testid="forward-entry-ha-purge" />
      <p v-if="unbindError" class="fwd-note is-danger ha-mt" role="alert">{{ unbindError }}</p>
      <template #footer="{ close }">
        <UiButton :disabled="unbinding" @click="close">{{ t('forwardV4.common.cancel') }}</UiButton>
        <UiButton variant="danger" :loading="unbinding" data-testid="forward-entry-ha-unbind-confirm" @click="unbind">{{ t('forwardDns.unbind.confirm') }}</UiButton>
      </template>
    </UiDialog>
  </UiCard>
</template>

<script setup>
// 入口高可用 on the route page (L2): GET /routes/{id}/dns, every 30 s while
// the tab is visible (the D5 overview pace: Control evaluates every 10 s
// and a node moves after 3 evaluations). The state, published against
// desired records, each entry with its reason, the last error and the next
// attempt. Unbinding needs a super administrator (can_delete, D7).
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { AlertCircle, Link2, Pencil, Unlink } from '@lucide/vue'
import { UiBadge, UiButton, UiCard, UiCheckbox, UiCopyField, UiDialog, UiErrorState, UiGroupedList, UiGroupedListRow, UiSkeleton, useFormat, useToast } from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import { deleteDnsBinding, newIdempotencyKey, routeDns } from '@/api/forwardV4'
import { DEFAULT_TTL, dnsErrorMessage, reasonTone, recordTypeLabel, stateTone } from './dnsModel'
import { forwardErrorMessage } from './messages'
import { usePolling } from './usePolling'
import { num } from './routeModel'
import './forward.css'

const props = defineProps({
  routeId: { type: String, required: true },
  canDelete: { type: Boolean, default: null },
  nodeName: { type: Function, default: ref => ref }
})

const router = useRouter()
const { t, te } = useAppI18n()
const fmt = useFormat()
const toast = useToast()

const status = ref(null)
const error = ref('')
const { refresh } = usePolling(async () => {
  try {
    status.value = await routeDns(props.routeId)
    error.value = ''
  } catch (cause) {
    error.value = forwardErrorMessage(t, cause)
    throw cause
  }
}, { interval: 30_000 })

const editPath = computed(() => `/admin/forward/routes/${props.routeId}/edit`)
const state = computed(() => status.value?.state || 'unbound')
const binding = computed(() => status.value?.binding || {})
const stateLabel = computed(() => (te(`forwardDns.state.${state.value}`) ? t(`forwardDns.state.${state.value}`) : state.value))
const stateText = computed(() => (te(`forwardDns.stateHelp.${state.value}`) ? t(`forwardDns.stateHelp.${state.value}`) : ''))
const warn = computed(() => ['degraded', 'rate_limited', 'hostname_mismatch', 'route_missing'].includes(state.value))
const cnameTarget = computed(() => (binding.value.mode === 'DNS_BINDING_MODE_CNAME' ? (status.value?.cname_target || binding.value.record_name || '') : ''))
const modeText = computed(() => {
  const mode = binding.value.mode || 'DNS_BINDING_MODE_DDNS'
  const label = t(`forwardDns.mode.${mode}`)
  return mode === 'DNS_BINDING_MODE_CNAME' ? `${label} → ${binding.value.record_name || '—'}` : label
})
const publishedAt = computed(() => num(status.value?.published_at_unix_ms))
const evaluatedAt = computed(() => num(status.value?.evaluated_at_unix_ms))
const lastErrorAt = computed(() => num(status.value?.last_error_at_unix_ms))
const nextAttempt = computed(() => num(status.value?.next_attempt_at_unix_ms))
const nodes = computed(() => status.value?.nodes || [])

// Every bound record type has a row, even before anything is published.
const records = computed(() => {
  const listed = status.value?.records || []
  const types = binding.value.record_types?.length ? binding.value.record_types : ['DNS_RECORD_TYPE_A']
  const all = [...new Set([...types, ...listed.map(record => record.type)])]
  return all.map(type => {
    const record = listed.find(item => item.type === type) || {}
    return { type, published: record.published || [], desired: record.desired || [] }
  })
})

function reasonLabel(reason) {
  const key = `forwardDns.reason.${reason}`
  return te(key) ? t(key) : (reason || '—')
}

function streak(node) {
  if (node.healthy && !node.in_rotation && num(node.good_streak)) return t('forwardDns.card.joining', { n: num(node.good_streak) })
  if (!node.healthy && node.in_rotation && num(node.bad_streak)) return t('forwardDns.card.leaving', { n: num(node.bad_streak) })
  return ''
}

// ---------------------------------------------------------------------------
// Unbind (super administrator): purge deletes the published records first.
// ---------------------------------------------------------------------------

const unbindOpen = ref(false)
const unbinding = ref(false)
const unbindError = ref('')
const purge = ref(true)
let unbindKey = newIdempotencyKey()

function openUnbind() {
  purge.value = true
  unbindError.value = ''
  unbindKey = newIdempotencyKey()
  unbindOpen.value = true
}

async function unbind() {
  unbinding.value = true
  unbindError.value = ''
  try {
    await deleteDnsBinding(binding.value.id, { purge: purge.value, idempotencyKey: `${unbindKey}:${purge.value ? 'purge' : 'keep'}` })
    unbindOpen.value = false
    toast.success(t('forwardDns.toast.unbound', { host: status.value?.entry_hostname || '' }))
    await refresh()
  } catch (cause) {
    unbindError.value = cause.code === 'dns_purge_failed'
      ? `${dnsErrorMessage(t, te, cause)} ${t('forwardDns.unbind.purgeFailedHint')}`
      : dnsErrorMessage(t, te, cause)
  } finally {
    unbinding.value = false
  }
}

defineExpose({ refresh })
</script>

<style scoped>
.ha-actions {
  display: inline-flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
}

.ha-text {
  margin: 0 0 var(--space-3);
}

.ha-empty {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3);
  align-items: center;
  justify-content: space-between;
}

.ha-empty .ha-text {
  margin: 0;
}

.ha-error {
  display: grid;
  gap: var(--space-1);
  margin-bottom: var(--space-3);
}

.ha-error__head {
  display: inline-flex;
  gap: var(--space-1);
  align-items: center;
  font-weight: var(--weight-semibold);
}

.ha-error__text {
  font-family: var(--font-mono);
  font-size: var(--type-caption-size);
  overflow-wrap: anywhere;
}

.ha-layout {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  gap: var(--space-4);
  align-items: start;
}

.ha-subhead {
  margin: 0 0 var(--space-2);
  font-size: var(--type-callout-size);
  font-weight: var(--weight-semibold);
}

.ha-table {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--type-callout-size);
}

.ha-table th,
.ha-table td {
  padding: var(--space-2);
  border-bottom: 1px solid var(--separator);
  text-align: left;
  vertical-align: top;
}

.ha-table thead th {
  color: var(--label-2);
  font-size: var(--type-caption-size);
  font-weight: var(--weight-medium);
}

.ha-values {
  display: grid;
  gap: 2px;
}

.ha-stale {
  color: var(--label-2);
  text-decoration: line-through;
}

.ha-cname {
  display: grid;
  gap: var(--space-2);
  margin-top: var(--space-4);
  padding: var(--space-3);
  border-radius: var(--radius-xs);
  background: var(--fill-1);
}

.ha-cname .ha-text {
  margin: 0;
}

.ha-nodes {
  margin-top: var(--space-4);
}

.ha-node__name {
  font-weight: var(--weight-medium);
}

.ha-wrap {
  overflow-wrap: anywhere;
}

.ha-chips {
  display: inline-flex;
  flex-wrap: wrap;
  gap: var(--space-1) var(--space-2);
  align-items: center;
  justify-content: flex-end;
}

.ha-mt {
  margin-top: var(--space-3);
}

.ha-nodes + .ha-text {
  margin: var(--space-3) 0 0;
}

@media (max-width: 833.98px) {
  .ha-layout {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (max-width: 639.98px) {
  .ha-node {
    flex-direction: column;
  }

  .ha-chips {
    justify-content: flex-start;
  }
}
</style>
