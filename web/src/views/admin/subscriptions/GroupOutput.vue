<template>
  <UiSection :title="t('adminSubscriptionGroups.output.title')" :description="t('adminSubscriptionGroups.output.description')">
    <template #actions>
      <UiButton :icon="Download" :disabled="!previewContent || Boolean(previewError)" data-test="download-preview" @click="downloadPreview">{{ t('adminSubscriptionGroups.output.download') }}</UiButton>
    </template>
    <div class="output-toolbar">
      <UiSelect
        id="subscription-output-format"
        v-model="previewFormat"
        size="md"
        class="output-format"
        :label="t('adminSubscriptionGroups.output.format')"
        :options="formatOptions"
        @update:model-value="loadPreview"
      />
    </div>
    <UiSkeleton v-if="showSkeleton" variant="text" :lines="8" :label="t('adminSettings.loading')" />
    <UiErrorState
      v-else-if="previewError"
      compact
      heading-tag="h3"
      :title="t('adminSubscriptionGroups.output.loadFailed')"
      :error="previewError"
      @retry="loadPreview"
    />
    <UiEmptyState
      v-else-if="!previewLoading && !previewContent"
      compact
      heading-tag="h3"
      :icon="FileCode"
      :title="t('adminSubscriptionGroups.output.empty')"
      :description="t('adminSubscriptionGroups.output.emptyDescription')"
    />
    <UiCodeBlock
      v-else-if="!previewLoading"
      :label="formatLabel(previewFormat)"
      :code="previewContent"
      max-height="480px"
      data-test="subscription-preview"
    />
  </UiSection>
</template>

<script setup>
// 订阅输出: what a client receives for this group in a chosen format,
// rendered by the server (POST /admin/subscription/preview with group_ids
// and format, unchanged), in a code block with copy and download.
import { computed, ref, watch } from 'vue'
import { Download, FileCode } from '@lucide/vue'
import adminApi from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import UiButton from '@/ui/UiButton.vue'
import UiCodeBlock from '@/ui/UiCodeBlock.vue'
import UiEmptyState from '@/ui/UiEmptyState.vue'
import UiErrorState from '@/ui/UiErrorState.vue'
import UiSection from '@/ui/UiSection.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import { useDelayedLoading } from '@/ui/composables/useDelayedLoading'
import { PREVIEW_FORMATS, downloadText, readSubscriptionPayload, subscriptionErrorText, subscriptionFileExt } from './subscriptionShared'

const props = defineProps({
  group: { type: Object, required: true }
})

const { t } = useAppI18n()
const previewFormat = ref('v2ray')
const previewContent = ref('')
const previewLoading = ref(false)
const previewError = ref(null)
const showSkeleton = useDelayedLoading(previewLoading)

const formatOptions = computed(() => PREVIEW_FORMATS.map(value => ({ value, label: formatLabel(value) })))

function formatLabel(format) {
  return t(`adminSubscriptionGroups.formats.${format}`)
}

async function loadPreview() {
  previewLoading.value = true
  previewError.value = null
  try {
    const res = await adminApi.previewSubscription({
      group_ids: [props.group.id],
      format: previewFormat.value
    })
    const payload = readSubscriptionPayload(res, t('adminSubscriptionGroups.output.loadFailed'))
    previewContent.value = payload?.content || ''
  } catch (error) {
    previewContent.value = ''
    previewError.value = subscriptionErrorText(error) || t('adminSubscriptionGroups.output.loadFailed')
  } finally {
    previewLoading.value = false
  }
}

function downloadPreview() {
  downloadText(previewContent.value, `subscription.${subscriptionFileExt(previewFormat.value)}`)
}

watch(() => props.group?.id, id => { if (id) loadPreview() }, { immediate: true })
</script>

<style scoped>
.output-toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3);
  align-items: flex-end;
}

.output-format {
  width: 260px;
  max-width: 100%;
}
</style>
