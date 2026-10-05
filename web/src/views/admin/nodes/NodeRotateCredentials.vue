<template>
  <span class="node-rotate" data-testid="node-rotate">
    <UiButton
      v-if="rotation.superAdmin.value !== false"
      ref="triggerRef"
      :size="size"
      variant="danger-soft"
      :icon="KeyRound"
      :disabled="disabled"
      data-testid="rotate-credentials"
      @click="rotation.open"
    >{{ t('admin.nodes.rotate.action') }}</UiButton>
    <span v-else class="node-rotate__muted" data-testid="rotate-super-only">{{ t('admin.nodes.rotate.superOnly') }}</span>

    <UiConfirmDialog
      :open="rotation.stage.value === 'confirm'"
      :title="t('admin.nodes.rotate.confirm.title', { name: nodeLabel || node })"
      :message="t('admin.nodes.rotate.confirm.message')"
      :confirm-label="t('admin.nodes.rotate.confirm.action')"
      tone="danger"
      :loading="rotation.busy.value"
      :error="rotation.error.value"
      data-testid="rotate-confirm"
      @confirm="rotation.submit"
      @cancel="rotation.cancel"
    >
      <div class="node-rotate__body">
        <div>
          <h3 class="node-rotate__heading">{{ t('admin.nodes.rotate.confirm.impact') }}</h3>
          <ul class="node-rotate__list">
            <li>{{ t('admin.nodes.rotate.confirm.revoke') }}</li>
            <li>{{ t('admin.nodes.rotate.confirm.issue') }}</li>
            <li>{{ t('admin.nodes.rotate.confirm.offline') }}</li>
          </ul>
        </div>

        <UiSelect
          v-model="rotation.form.ttlSeconds"
          size="md"
          :label="t('admin.nodes.rotate.confirm.ttl')"
          :options="ttlOptions"
          :disabled="rotation.busy.value"
          data-testid="rotate-ttl"
        />
        <UiTextarea
          v-model="rotation.form.reason"
          :rows="2"
          :label="t('admin.nodes.rotate.confirm.reason')"
          :placeholder="t('admin.nodes.rotate.confirm.reasonPlaceholder')"
          :help="t('admin.nodes.rotate.confirm.reasonHelp', { count: rotation.reasonBytes.value, max: ROTATE_REASON_MAX })"
          :error="rotation.reasonTooLong.value ? t('admin.nodes.rotate.confirm.reasonTooLong', { count: rotation.reasonBytes.value, max: ROTATE_REASON_MAX }) : ''"
          :disabled="rotation.busy.value"
          autocomplete="off"
          data-testid="rotate-reason"
        />
        <template v-if="rotation.kind.value === 'proxy'">
          <UiCheckbox
            v-model="rotation.form.rotateApiKey"
            :label="t('admin.nodes.rotate.confirm.rotateKey')"
            :description="t('admin.nodes.rotate.confirm.rotateKeyHelp')"
            :disabled="rotation.busy.value"
            data-testid="rotate-api-key"
          />
          <NodeNotice v-if="rotation.form.rotateApiKey" tone="danger" data-testid="rotate-api-key-notice">
            {{ t('admin.nodes.rotate.confirm.rotateKeyNotice') }}
          </NodeNotice>
        </template>
      </div>
    </UiConfirmDialog>

    <UiDialog
      :open="rotation.stage.value === 'result'"
      size="md"
      :title="t('admin.nodes.rotate.result.title', { name: nodeLabel || node })"
      :description="t('admin.nodes.rotate.result.description')"
      :close-on-scrim="false"
      initial-focus=".ui-copy__button"
      data-testid="rotate-result"
      @update:open="onResultOpenChange"
      @close-auto-focus="restoreFocus"
    >
      <div v-if="result" class="node-rotate__body">
        <NodeNotice tone="warning" data-testid="rotate-warning">{{ t('admin.nodes.rotate.result.warning') }}</NodeNotice>
        <UiCopyField
          :value="result.credential"
          :label="t('admin.nodes.rotate.result.label')"
          :copy-label="t('admin.nodes.rotate.result.copy')"
          secret
          size="md"
          data-testid="rotate-credential"
        />
        <NodeNotice v-if="expired" tone="danger" data-testid="rotate-expired">{{ t('admin.nodes.rotate.result.expired') }}</NodeNotice>
        <p v-else class="node-rotate__text" data-testid="rotate-expiry">
          <time :datetime="expiresIso">{{ t('admin.nodes.rotate.result.expires', { time: format.dateTime(result.expiresAt) }) }}</time>
          ({{ t('admin.nodes.rotate.result.remaining', { remaining: format.duration(remainingSeconds) }) }}).
          {{ t('admin.nodes.rotate.result.single') }}
        </p>

        <div>
          <h3 class="node-rotate__heading">{{ t('admin.nodes.rotate.result.revoked') }}</h3>
          <dl class="node-rotate__facts" data-testid="rotate-revoked">
            <div>
              <dt>{{ t('admin.nodes.rotate.result.certificates') }}</dt>
              <dd>{{ format.number(result.revoked.certificates) }}</dd>
            </div>
            <div>
              <dt>{{ t('admin.nodes.rotate.result.enrollments') }}</dt>
              <dd>{{ format.number(result.revoked.enrollments) }}</dd>
            </div>
            <div>
              <dt>{{ t('admin.nodes.rotate.result.linkCertificates') }}</dt>
              <dd>{{ format.number(result.revoked.linkCertificates) }}</dd>
            </div>
          </dl>
        </div>
        <p class="node-rotate__text" data-testid="rotate-api-key-result">
          {{ result.apiKeyRotated ? t('admin.nodes.rotate.result.apiKeyReplaced') : t('admin.nodes.rotate.result.apiKeyKept') }}
        </p>
        <p class="node-rotate__text">
          {{ t('admin.nodes.rotate.result.next') }}
          <a class="node-rotate__link" :href="GUIDE_URL" target="_blank" rel="noopener noreferrer" data-testid="rotate-guide">{{ t('admin.nodes.rotate.result.guide') }}</a>
        </p>
      </div>
      <template #footer="{ close }">
        <UiButton variant="primary" data-testid="rotate-done" @click="close">{{ t('admin.nodes.rotate.result.done') }}</UiButton>
      </template>
    </UiDialog>
  </span>
</template>

<script setup>
// "Rotate Agent credentials" for a node (proxy node page, forward node page):
// a danger confirmation that says what is revoked and lets the operator give
// a reason, the credential lifetime and (proxy nodes) whether to replace the
// API key too, then a dialog that shows the new one-time `anixagt_`
// credential once. POST /api/v4/kernel/agents/rotate-credentials is for super
// administrators; the UI does not know the role, so a 403 turns the button
// into a sentence (the forward page knows it from can_delete and hides it).
//
// The credential lives in useCredentialRotation's `result` while the result
// dialog is open and is cleared when it closes (and when this component
// goes): it is never stored, put in the URL or logged. The page is not told
// the secret either: `rotated` carries only the node and whether the API key
// was replaced.
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { KeyRound } from '@lucide/vue'
import UiButton from '@/ui/UiButton.vue'
import UiCheckbox from '@/ui/UiCheckbox.vue'
import UiConfirmDialog from '@/ui/UiConfirmDialog.vue'
import UiCopyField from '@/ui/UiCopyField.vue'
import UiDialog from '@/ui/UiDialog.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiTextarea from '@/ui/UiTextarea.vue'
import { useFormat } from '@/ui/composables/useFormat'
import { useAppI18n } from '@/composables/useAppI18n'
import NodeNotice from './NodeNotice.vue'
import { ROTATE_REASON_MAX, ROTATE_TTL_SECONDS, useCredentialRotation } from './useCredentialRotation'

const GUIDE_URL = 'https://github.com/AnixOps/anix-control/blob/go_dev/docs/guide/agent-onboarding.md#rotating-a-nodes-credentials'
const TTL_LABELS = { 3600: 'hour', 21600: 'sixHours', 86400: 'day', 604800: 'week' }

const props = defineProps({
  // The Agent node name: "proxy-<id>" or "forward-<id>".
  node: { type: String, required: true },
  nodeLabel: { type: String, default: '' },
  disabled: { type: Boolean, default: false },
  // The trigger's size: sm in a grouped list row, md next to other buttons.
  size: { type: String, default: 'sm' }
})
const emit = defineEmits(['rotated'])
const { t, te } = useAppI18n()
const format = useFormat()

const rotation = useCredentialRotation(() => props.node, { t, te, onRotated: info => emit('rotated', info) })
const result = computed(() => rotation.result.value)

const ttlOptions = computed(() => ROTATE_TTL_SECONDS.map(value => ({ value, label: t(`admin.nodes.install.ttlOptions.${TTL_LABELS[value]}`) })))

// The countdown to the credential's expiry, once a second while it is shown.
const now = ref(Date.now())
let timer = null
function stopTimer() {
  if (timer) clearInterval(timer)
  timer = null
}
watch(result, (value) => {
  stopTimer()
  if (!value) return
  now.value = Date.now()
  timer = setInterval(() => { now.value = Date.now() }, 1000)
}, { immediate: true })
onBeforeUnmount(stopTimer)

const remainingSeconds = computed(() => Math.max(0, Math.ceil(((result.value?.expiresAt || 0) - now.value) / 1000)))
const expired = computed(() => Boolean(result.value) && remainingSeconds.value <= 0)
const expiresIso = computed(() => (result.value ? new Date(result.value.expiresAt).toISOString() : ''))

function onResultOpenChange(open) {
  if (!open) rotation.dismiss()
}

// The result dialog opened from the confirmation, which is gone by then, so
// the browser has nothing to return focus to: give it back to the button.
const triggerRef = ref(null)
function restoreFocus(event) {
  const button = triggerRef.value?.$el
  if (button && typeof button.focus === 'function') {
    event.preventDefault()
    button.focus()
  }
}
</script>

<style scoped>
.node-rotate__muted {
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.node-rotate__body {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.node-rotate__heading {
  margin: 0 0 var(--space-2);
  font-size: var(--type-callout-size);
  font-weight: var(--weight-semibold);
}

.node-rotate__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding-left: var(--space-5);
  color: var(--label-2);
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
}

.node-rotate__text {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
}

.node-rotate__facts {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: var(--space-3);
  margin: 0;
}

.node-rotate__facts div {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  padding: var(--space-3);
  border-radius: var(--radius-sm);
  background: var(--bg-grouped);
}

.node-rotate__facts dt {
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

.node-rotate__facts dd {
  margin: 0;
  color: var(--label-1);
  font-size: var(--type-body-size);
  font-weight: var(--weight-semibold);
  font-variant-numeric: tabular-nums;
}

.node-rotate__link {
  color: var(--accent);
}
</style>
