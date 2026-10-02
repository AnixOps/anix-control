<template>
  <div class="settings-section" data-settings-panel="about">
    <UiSection :title="t('adminSettings.about.title')" :description="t('adminSettings.about.description')">
      <template #actions>
        <UiButton :icon="Copy" :disabled="!rows.length" data-test="about-copy" @click="copyDetails">{{ t('adminSettings.about.copy') }}</UiButton>
      </template>
      <UiSkeleton v-if="showSkeleton" variant="card" :label="t('adminSettings.loading')" />
      <UiErrorState
        v-else-if="loadError && !rows.length"
        compact
        heading-tag="h3"
        :title="t('shell.about.unavailable')"
        :error="loadError"
        @retry="load"
      />
      <UiGroupedList v-else-if="!loading" data-about-list>
        <UiGroupedListRow :label="t('adminSettings.about.product')" :value="CONTROL_NAME" data-about-row="product" />
        <UiGroupedListRow v-for="row in rows" :key="row.key" :label="row.label" :data-about-row="row.key">
          <template #value><span class="about-value">{{ row.value }}</span></template>
        </UiGroupedListRow>
      </UiGroupedList>
    </UiSection>
  </div>
</template>

<script setup>
// 系统设置 → 关于: version and build details, the same rows as the account
// menu's 关于 dialog (utils/systemInfo.js aboutRows). GET /admin/system/info.
import { computed, onMounted, ref } from 'vue'
import { Copy } from '@lucide/vue'
import { getSystemInfo } from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import { CONTROL_NAME } from '@/constants/brand'
import { FRONTEND_BUILD, aboutRows, readSystemInfo } from '@/utils/systemInfo'
import { panelErrorMessage } from '@/utils/panelResponse'
import UiButton from '@/ui/UiButton.vue'
import UiErrorState from '@/ui/UiErrorState.vue'
import UiGroupedList from '@/ui/UiGroupedList.vue'
import UiGroupedListRow from '@/ui/UiGroupedListRow.vue'
import UiSection from '@/ui/UiSection.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import { copyText } from '@/ui/composables/useClipboard'
import { useDelayedLoading } from '@/ui/composables/useDelayedLoading'
import { useToast } from '@/ui/composables/useToast'

const { t } = useAppI18n()
const toast = useToast()
const info = ref(null)
const loading = ref(false)
const loadError = ref(null)
const showSkeleton = useDelayedLoading(loading)

const rows = computed(() => aboutRows(t, info.value, FRONTEND_BUILD))

async function load() {
  loading.value = true
  loadError.value = null
  try {
    const res = await getSystemInfo()
    if (res && typeof res === 'object' && Object.prototype.hasOwnProperty.call(res, 'code') && Number(res.code) !== 0) {
      throw new Error(res.msg || t('shell.about.unavailable'))
    }
    info.value = readSystemInfo(res)
  } catch (err) {
    loadError.value = panelErrorMessage(err, t('shell.about.unavailable'))
  } finally {
    loading.value = false
  }
}

async function copyDetails() {
  const text = [`${t('adminSettings.about.product')}: ${CONTROL_NAME}`, ...rows.value.map(row => `${row.label}: ${row.value}`)].join('\n')
  if (await copyText(text)) toast.success(t('adminSettings.about.copied'))
  else toast.error(t('ui.copy.failed'))
}

onMounted(load)
</script>

<style scoped>
.settings-section {
  display: flex;
  flex-direction: column;
  gap: var(--space-12);
  min-width: 0;
}

.about-value {
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
}
</style>
