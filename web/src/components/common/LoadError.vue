<template>
  <div class="load-error" role="alert" data-load-error>
    <UiEmptyState :icon="CloudAlert" :title="title" :description="message" :heading-tag="headingTag" :compact="compact">
      <template #actions>
        <UiButton :icon="RotateCw" data-load-error-retry @click="emit('retry')">{{ t('portal.state.retry') }}</UiButton>
        <UiButton variant="tertiary" :icon="copied ? Check : Copy" data-load-error-copy @click="copyDetails">
          {{ copied ? t('portal.state.detailsCopied') : t('portal.state.copyDetails') }}
        </UiButton>
      </template>
    </UiEmptyState>
  </div>
</template>

<script setup>
// The error state of a page or a section (plan §9): what failed in one
// sentence, the server's message, 重试, and 复制错误详情 (status, request id
// when the server sent one, the page and the time) for a ticket.
import { computed, ref } from 'vue'
import { Check, CloudAlert, Copy, RotateCw } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { panelErrorMessage } from '@/utils/panelResponse'
import UiButton from '@/ui/UiButton.vue'
import UiEmptyState from '@/ui/UiEmptyState.vue'
import { copyText } from '@/ui/composables/useClipboard'

const props = defineProps({
  title: { type: String, required: true },
  error: { type: [Object, String, null], default: null },
  headingTag: { type: String, default: 'h2' },
  compact: { type: Boolean, default: false }
})

const emit = defineEmits(['retry'])
const { t } = useAppI18n()
const copied = ref(false)

const message = computed(() => (props.error ? panelErrorMessage(props.error) : ''))

async function copyDetails() {
  const response = props.error?.response
  const lines = [
    props.title,
    message.value,
    response?.status ? `HTTP ${response.status}` : '',
    response?.headers?.['x-request-id'] ? `Request ID: ${response.headers['x-request-id']}` : '',
    typeof window !== 'undefined' ? `Page: ${window.location.pathname}` : '',
    `Time: ${new Date().toISOString()}`
  ].filter(Boolean)
  copied.value = await copyText(lines.join('\n'))
  if (copied.value) setTimeout(() => { copied.value = false }, 2000)
}
</script>

<style scoped>
.load-error {
  display: flex;
  justify-content: center;
}
</style>
