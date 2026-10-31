<template>
  <div class="settings-section" data-settings-panel="data">
    <UiSection :title="t('adminSettings.data.title')" :description="t('adminSettings.data.description')">
      <UiSkeleton v-if="showSkeleton" variant="card" :label="t('adminSettings.loading')" />
      <UiErrorState
        v-else-if="loadError"
        compact
        heading-tag="h3"
        :title="t('adminMonitor.destinations.settings.loadFailed')"
        :error="loadError"
        @retry="load"
      />
      <template v-else-if="loaded">
        <UiGroupedList :footer="t('adminSettings.data.footer')">
          <UiGroupedListRow stacked>
            <UiSwitch
              v-model="form.enabled"
              :label="t('adminMonitor.destinations.settings.enabled')"
              :description="t('adminMonitor.destinations.settings.enabledHint')"
              data-test="data-enabled"
            />
          </UiGroupedListRow>
          <UiGroupedListRow stacked>
            <UiSelect
              id="settings-retention"
              v-model="retentionChoice"
              :label="t('adminMonitor.destinations.settings.retention')"
              :help="t('adminMonitor.destinations.settings.retentionHelp')"
              :options="retentionOptions"
              data-test="data-retention"
            />
          </UiGroupedListRow>
        </UiGroupedList>
      </template>
    </UiSection>

    <SettingsSaveBar
      :visible="dirty"
      :saving="saving"
      @save="save"
      @discard="discard"
    />
  </div>
</template>

<script setup>
// 系统设置 → 数据与保留: the access statistics switch and how long they are
// kept, as a switch and a list of presets. The same settings (and the
// statistics themselves) are on 监控 → 目的地;
// GET/PUT /admin/traffic/destinations/settings.
import { computed, onMounted, reactive, ref } from 'vue'
import { getDestinationSettings, updateDestinationSettings } from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import { useUnsavedChanges } from '@/composables/useUnsavedChanges'
import SettingsSaveBar from '@/components/admin/settings/SettingsSaveBar.vue'
import UiErrorState from '@/ui/UiErrorState.vue'
import UiGroupedList from '@/ui/UiGroupedList.vue'
import UiGroupedListRow from '@/ui/UiGroupedListRow.vue'
import UiSection from '@/ui/UiSection.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import UiSwitch from '@/ui/UiSwitch.vue'
import { useDelayedLoading } from '@/ui/composables/useDelayedLoading'
import { useToast } from '@/ui/composables/useToast'
import { systemErrorText } from './systemResponse'

const { t } = useAppI18n()
const toast = useToast()

const RETENTION_PRESETS = [7, 30, 60, 90, 180]

const loading = ref(false)
const loaded = ref(false)
const loadError = ref(null)
const saving = ref(false)
const showSkeleton = useDelayedLoading(loading)
const saved = reactive({ enabled: false, retention_days: 30 })
const form = reactive({ enabled: false, retention_days: 30 })

const retentionChoice = computed({
  get: () => String(form.retention_days),
  set: value => { form.retention_days = Number(value) }
})
// A value the console did not offer (set through the API) stays selectable.
const retentionOptions = computed(() => {
  const days = new Set([...RETENTION_PRESETS, form.retention_days, saved.retention_days])
  return [...days].sort((a, b) => a - b).map(day => ({
    value: String(day),
    label: t('adminSettings.data.days', { days: day })
  }))
})
const dirty = computed(() => form.enabled !== saved.enabled || form.retention_days !== saved.retention_days)

function apply(settings) {
  saved.enabled = Boolean(settings?.enabled)
  saved.retention_days = Number(settings?.retention_days) || 30
  form.enabled = saved.enabled
  form.retention_days = saved.retention_days
}

async function load() {
  loading.value = true
  loadError.value = null
  try {
    apply(await getDestinationSettings())
    loaded.value = true
  } catch (err) {
    loadError.value = err
  } finally {
    loading.value = false
  }
}

async function save() {
  if (saving.value) return
  saving.value = true
  try {
    apply(await updateDestinationSettings({ enabled: form.enabled, retention_days: form.retention_days }))
    toast.success(t('adminMonitor.destinations.settings.saved'))
  } catch (err) {
    toast.error(systemErrorText(err) || t('adminMonitor.destinations.settings.saveFailed'))
  } finally {
    saving.value = false
  }
}

function discard() {
  form.enabled = saved.enabled
  form.retention_days = saved.retention_days
}

useUnsavedChanges(dirty, { discard })

onMounted(load)
</script>

<style scoped>
.settings-section {
  display: flex;
  flex-direction: column;
  gap: var(--space-12);
  min-width: 0;
}

.settings-section :deep(.ui-section) {
  gap: var(--space-5);
}
</style>
