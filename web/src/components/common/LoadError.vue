<template>
  <UiErrorState
    class="load-error"
    data-load-error
    :title="title"
    :error="error"
    :heading-tag="headingTag"
    :compact="compact"
    @retry="emit('retry')"
  />
</template>

<script setup>
// The error state of a user page or a section (plan §9). Since U6 it is the
// component library's UiErrorState (重试, 复制错误详情 with status, request id,
// page and time), which the admin list pages use through UiDataTable too.
import UiErrorState from '@/ui/UiErrorState.vue'

defineProps({
  title: { type: String, required: true },
  error: { type: [Object, String, null], default: null },
  headingTag: { type: String, default: 'h2' },
  compact: { type: Boolean, default: false }
})

const emit = defineEmits(['retry'])
</script>
