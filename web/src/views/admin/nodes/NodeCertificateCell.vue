<template>
  <span v-if="status === 'error'" class="node-cert-muted" data-testid="node-certificate-unavailable">{{ t('admin.nodes.agent.unavailable') }}</span>
  <span v-else-if="entry" class="node-cert" data-testid="node-certificate" :data-certificate="certificate.state">
    <UiBadge :tone="certificate.tone" :dot="false" :label="stateLabel" />
    <time v-if="dateText" class="node-cert__date" :datetime="dateValue" :title="format.dateTime(dateValue)">{{ dateText }}</time>
  </span>
  <span v-else class="node-cert-muted" data-testid="node-certificate-empty">
    <span aria-hidden="true">—</span><span v-if="status === 'loading'" class="visually-hidden">{{ t('admin.nodes.agent.loading') }}</span>
  </span>
</template>

<script setup>
// The certificate column of the node list: its state as a chip (valid,
// renewal overdue, revoked, expired, none) and the day that matters (when it
// ends, or was revoked). The state is a word, never only a colour.
import { computed } from 'vue'
import UiBadge from '@/ui/UiBadge.vue'
import { useFormat } from '@/ui/composables/useFormat'
import { useAppI18n } from '@/composables/useAppI18n'
import { certificateOf } from './agentConnection'

const props = defineProps({
  entry: { type: Object, default: null },
  status: { type: String, default: 'idle' }
})
const { t } = useAppI18n()
const format = useFormat()
const certificate = computed(() => certificateOf(props.entry))
const stateLabel = computed(() => t(`admin.nodes.agent.certificate.${certificate.value.overdue ? 'overdue' : certificate.value.state}`))
// The day that matters for the state: when it ends (valid, expired) or was
// revoked.
const dateValue = computed(() => (certificate.value.state === 'revoked' ? (certificate.value.revokedAt || certificate.value.notAfter) : certificate.value.notAfter))
const dateText = computed(() => {
  const { state } = certificate.value
  if (!dateValue.value || state === 'none') return ''
  const key = { valid: 'until', expired: 'expiredOn', revoked: 'revokedOn' }[state]
  return t(`admin.nodes.agent.certificate.${key}`, { date: format.date(dateValue.value) })
})
</script>

<style scoped>
.node-cert {
  display: inline-flex;
  flex-wrap: wrap;
  gap: var(--space-1) var(--space-2);
  align-items: center;
}

.node-cert__date,
.node-cert-muted {
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

.node-cert-muted {
  font-size: inherit;
}
</style>
