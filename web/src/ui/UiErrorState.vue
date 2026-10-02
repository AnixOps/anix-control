<template>
  <div class="ui-error-state" role="alert" data-error-state>
    <UiEmptyState :icon="CloudAlert" :title="title" :description="message" :heading-tag="headingTag" :compact="compact">
      <template #actions>
        <UiButton :icon="RotateCw" data-error-retry @click="emit('retry')">{{ t('ui.error.retry') }}</UiButton>
        <UiButton variant="tertiary" :icon="copied ? Check : Copy" data-error-copy @click="copyDetails">
          {{ copied ? t('ui.error.detailsCopied') : t('ui.error.copyDetails') }}
        </UiButton>
      </template>
    </UiEmptyState>
  </div>
</template>

<script setup>
// Error state of a list, a page or a section (plan §9): what failed in one
// sentence, the server's message, 重试 and 复制错误详情 (the message, HTTP
// status, request id when the server sent one, page and time), so the
// details can go into a ticket.
import { computed, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Check, CloudAlert, Copy, RotateCw } from '@lucide/vue'
import UiButton from './UiButton.vue'
import UiEmptyState from './UiEmptyState.vue'
import { copyText } from './composables/useClipboard'

const props = defineProps({
  title: { type: String, required: true },
  // An axios error, an Error, or a message.
  error: { type: [Object, String, null], default: null },
  headingTag: { type: String, default: 'h2' },
  compact: { type: Boolean, default: false }
})

const emit = defineEmits(['retry'])
const { t } = useI18n()
const copied = ref(false)
let timer = null

const message = computed(() => errorMessage(props.error))

function errorMessage(error) {
  if (!error) return ''
  if (typeof error === 'string') return error
  const data = error.response?.data
  return data?.msg || data?.message || data?.error || error.message || ''
}

async function copyDetails() {
  const response = props.error && typeof props.error === 'object' ? props.error.response : null
  const lines = [
    props.title,
    message.value,
    response?.status ? `HTTP ${response.status}` : '',
    response?.headers?.['x-request-id'] ? `Request ID: ${response.headers['x-request-id']}` : '',
    typeof window !== 'undefined' ? `Page: ${window.location.pathname}` : '',
    `Time: ${new Date().toISOString()}`
  ].filter(Boolean)
  copied.value = await copyText(lines.join('\n'))
  if (timer) clearTimeout(timer)
  if (copied.value) timer = setTimeout(() => { copied.value = false }, 2000)
}

onBeforeUnmount(() => { if (timer) clearTimeout(timer) })
</script>

<style scoped>
.ui-error-state {
  display: flex;
  justify-content: center;
}
</style>
