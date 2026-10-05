<template>
  <UiSection :title="t('adminNotify.telegram.test.title')" :description="t('adminNotify.telegram.test.description')">
    <div class="telegram-test" data-telegram-test>
      <p v-if="dirty" class="telegram-test__note" data-testid="telegram-test-dirty">{{ t('adminNotify.telegram.test.dirty') }}</p>
      <div class="telegram-test__actions">
        <UiButton
          :icon="SendHorizontal"
          :loading="busy"
          :disabled="waiting"
          data-testid="telegram-test-send"
          @click="send"
        >
          {{ t('adminNotify.telegram.test.send') }}
        </UiButton>
        <span v-if="waiting" class="telegram-test__wait tabular-nums" data-testid="telegram-test-wait">
          {{ t('adminNotify.telegram.test.wait', { seconds: remaining }) }}
        </span>
      </div>
      <div v-if="result" class="telegram-test__result" :class="`is-${result.tone}`" role="status" data-testid="telegram-test-result" :data-tone="result.tone" :data-class="result.cls">
        <UiBadge :tone="result.tone" :label="result.label" />
        <p class="telegram-test__message">{{ result.message }}</p>
        <p v-if="result.hint" class="telegram-test__hint">{{ result.hint }}</p>
      </div>
    </div>
  </UiSection>
</template>

<script setup>
// 通知 → Telegram → 发送测试消息: POST /api/v4/kernel/notifications/telegram/test
// with {} (the administrator's own bound chat). It answers what Telegram said
// as a result class, shown as a tone (success, warning, danger) with the
// server's fixed sentence. 409 means the administrator has not bound
// Telegram; 429 carries Retry-After and the button counts it down (ticks,
// not Date.now(), so a frozen clock cannot stall it). The bot token is never
// read, shown or logged here: the route does not take or return it.
import { computed, onBeforeUnmount, ref } from 'vue'
import { SendHorizontal } from '@lucide/vue'
import { testTelegramBot } from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import { adminV4ErrorMessage } from '@/utils/adminV4'
import { TELEGRAM_TEST_NETWORK_REASONS, readTelegramTestRefusal, telegramTestClass, telegramTestTone } from '@/utils/telegramTest'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiSection from '@/ui/UiSection.vue'

defineProps({
  // The bot form has unsaved changes: the test uses the saved ones.
  dirty: { type: Boolean, default: false }
})

const { t, te } = useAppI18n()

const busy = ref(false)
const remaining = ref(0)
const result = ref(null)
let timer = null

const waiting = computed(() => remaining.value > 0)

function stopTimer() {
  if (timer) clearInterval(timer)
  timer = null
}

function startCountdown(seconds) {
  stopTimer()
  remaining.value = seconds
  timer = setInterval(() => {
    remaining.value -= 1
    if (remaining.value <= 0) {
      remaining.value = 0
      stopTimer()
    }
  }, 1000)
}

function describe(answer) {
  const cls = telegramTestClass(answer?.class)
  const reason = TELEGRAM_TEST_NETWORK_REASONS.includes(answer?.reason) ? answer.reason : ''
  const hintKey = cls === 'network_error' && reason ? `adminNotify.telegram.test.reasons.${reason}` : `adminNotify.telegram.test.hints.${cls}`
  return {
    cls,
    tone: telegramTestTone(cls),
    label: t(`adminNotify.telegram.test.classes.${cls}`),
    // The server's own sentence (fixed per class); ours when it sent none.
    message: typeof answer?.message === 'string' && answer.message ? answer.message : t(`adminNotify.telegram.test.classes.${cls}`),
    hint: te(hintKey) ? t(hintKey) : ''
  }
}

function describeRefusal(error) {
  const refusal = readTelegramTestRefusal(error)
  if (refusal.kind === 'rate_limited') {
    startCountdown(refusal.retryAfter)
    return { cls: 'rate_limited', tone: 'warning', label: t('adminNotify.telegram.test.tooMany'), message: t('adminNotify.telegram.test.tooManyMessage'), hint: '' }
  }
  if (refusal.kind === 'not_bound') {
    return { cls: 'not_bound', tone: 'warning', label: t('adminNotify.telegram.test.notBound'), message: t('adminNotify.telegram.test.notBoundMessage'), hint: t('adminNotify.telegram.test.notBoundHint') }
  }
  if (refusal.kind === 'not_allowed') {
    return { cls: 'not_allowed', tone: 'danger', label: t('adminNotify.telegram.test.notAllowed'), message: t('adminNotify.telegram.test.notAllowedMessage'), hint: '' }
  }
  return {
    cls: 'failed',
    tone: 'danger',
    label: t('adminNotify.telegram.test.failed'),
    message: adminV4ErrorMessage(error, t('adminNotify.telegram.test.failedShort')),
    hint: ''
  }
}

async function send() {
  if (busy.value || waiting.value) return
  busy.value = true
  try {
    result.value = describe(await testTelegramBot())
  } catch (error) {
    result.value = describeRefusal(error)
  } finally {
    busy.value = false
  }
}

onBeforeUnmount(stopTimer)
</script>

<style scoped>
.telegram-test {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  max-width: 640px;
}

.telegram-test__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3);
  align-items: center;
}

.telegram-test__wait,
.telegram-test__note,
.telegram-test__hint {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.telegram-test__result {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  align-items: flex-start;
  padding: var(--space-3) var(--space-4);
  border: 1px solid var(--separator);
  border-left-width: 4px;
  border-radius: var(--radius-sm);
  background: var(--bg-elevated);
}

.telegram-test__result.is-success {
  border-left-color: var(--success);
}

.telegram-test__result.is-warning {
  border-left-color: var(--warning);
}

.telegram-test__result.is-danger {
  border-left-color: var(--danger);
}

.telegram-test__message {
  margin: 0;
  color: var(--label-1);
  overflow-wrap: anywhere;
}
</style>
