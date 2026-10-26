<template>
  <div class="monitor-section" data-monitor-destinations>
    <UiCard :title="t('adminMonitor.destinations.settings.title')" :description="t('adminMonitor.destinations.settings.description')" as="section" data-destination-settings>
      <LoadError v-if="settingsError" :title="t('adminMonitor.destinations.settings.loadFailed')" :error="settingsError" compact @retry="loadSettings" />
      <form v-else class="destination-settings" @submit.prevent="saveSettings">
        <UiSwitch v-model="form.enabled" :label="t('adminMonitor.destinations.settings.enabled')" :description="t('adminMonitor.destinations.settings.enabledHint')" :disabled="!settingsLoaded" data-destination-enabled />
        <UiNumberField
          v-model="form.retention_days"
          class="destination-settings__retention"
          :label="t('adminMonitor.destinations.settings.retention')"
          :unit="t('adminMonitor.destinations.settings.retentionUnit')"
          :help="t('adminMonitor.destinations.settings.retentionHelp')"
          :min="7"
          :max="180"
          :disabled="!settingsLoaded"
          size="md"
          data-destination-retention
        />
        <UiButton type="submit" variant="primary" :loading="saving" :disabled="!settingsLoaded || !dirty || !retentionValid" data-destination-save>{{ t('adminMonitor.destinations.settings.save') }}</UiButton>
      </form>
    </UiCard>

    <div class="monitor-section__toolbar">
      <UiSegmentedControl v-model="range" :options="rangeOptions" :aria-label="t('adminMonitor.range.label')" data-monitor-range />
      <UiCombobox
        class="monitor-section__picker"
        :model-value="selectedUserId"
        :options="userOptions"
        :aria-label="t('adminMonitor.destinations.user')"
        size="md"
        data-destination-user
        @update:model-value="value => selectUser(value)"
      />
      <UiButton class="monitor-section__end" :icon="RotateCw" :loading="loading" data-monitor-refresh @click="refreshAll">{{ t('adminMonitor.refresh') }}</UiButton>
    </div>

    <p v-if="settingsLoaded && !saved.enabled" class="destination-settings__off" role="status" data-destination-off>{{ t('adminMonitor.destinations.off') }}</p>

    <DestinationStatsView :summary="summary" :loading="loading" :error="errorMessage" storage-key="admin.monitor.destinations" @retry="fetchSummary" />
  </div>
</template>

<script setup>
// 访问统计 of 流量与监控: each user's traffic by destination domain and
// category (GET /api/v4/admin/traffic/destinations?hours=&user_id=), for all
// users or one picked from the traffic ranking of the range, and the
// statistics' settings (GET and PUT /api/v4/admin/traffic/destinations/
// settings): on or off, and how many days are kept. Late answers of an
// earlier request are ignored.
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { RotateCw } from '@lucide/vue'
import { getDestinationSettings, getTrafficDestinations, getUserTrafficRanking, updateDestinationSettings } from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import LoadError from '@/components/common/LoadError.vue'
import DestinationStatsView from '@/components/traffic/DestinationStatsView.vue'
import UiButton from '@/ui/UiButton.vue'
import UiCard from '@/ui/UiCard.vue'
import UiCombobox from '@/ui/UiCombobox.vue'
import UiNumberField from '@/ui/UiNumberField.vue'
import UiSegmentedControl from '@/ui/UiSegmentedControl.vue'
import UiSwitch from '@/ui/UiSwitch.vue'
import { useToast } from '@/ui/composables/useToast'
import { errorText, readRankingRows } from './trafficData'
import { MONITOR_RANGES, useMonitorRange } from './useMonitorRange'

const { t } = useAppI18n()
const toast = useToast()
const { range, hours } = useMonitorRange()

const selectedUserId = ref(0)
const ranking = ref([])
const summary = ref(null)
const loading = ref(false)
const errorMessage = ref('')

const form = reactive({ enabled: false, retention_days: 30 })
const saved = reactive({ enabled: false, retention_days: 30 })
const settingsLoaded = ref(false)
const settingsError = ref(null)
const saving = ref(false)

let isActive = true
let summarySeq = 0

const rangeOptions = computed(() => Object.keys(MONITOR_RANGES).map(value => ({ value, label: t(`adminMonitor.range.${value}`) })))
const userOptions = computed(() => [
  { value: 0, label: t('adminMonitor.destinations.allUsers') },
  ...ranking.value.map(row => ({ value: row.user_id, label: row.email || `#${row.user_id}` }))
])
const retentionValid = computed(() => Number.isInteger(form.retention_days) && form.retention_days >= 7 && form.retention_days <= 180)
const dirty = computed(() => form.enabled !== saved.enabled || form.retention_days !== saved.retention_days)

function applySettings(settings) {
  saved.enabled = Boolean(settings?.enabled)
  saved.retention_days = Number(settings?.retention_days) || 30
  form.enabled = saved.enabled
  form.retention_days = saved.retention_days
}

async function loadSettings() {
  settingsError.value = null
  try {
    const settings = await getDestinationSettings()
    if (!isActive) return
    applySettings(settings)
    settingsLoaded.value = true
  } catch (err) {
    if (!isActive) return
    settingsError.value = err
  }
}

async function saveSettings() {
  if (!retentionValid.value) return
  saving.value = true
  try {
    applySettings(await updateDestinationSettings({ enabled: form.enabled, retention_days: form.retention_days }))
    toast.success(t('adminMonitor.destinations.settings.saved'))
  } catch (err) {
    toast.error(errorText(err, t('adminMonitor.destinations.settings.saveFailed')))
  } finally {
    saving.value = false
  }
}

async function fetchSummary() {
  const requestSeq = ++summarySeq
  loading.value = true
  errorMessage.value = ''
  try {
    const answer = await getTrafficDestinations({ hours: hours.value, userId: selectedUserId.value, limit: 200 })
    if (!isActive || requestSeq !== summarySeq) return
    summary.value = answer
  } catch (err) {
    if (!isActive || requestSeq !== summarySeq) return
    errorMessage.value = err?.response?.data?.error?.message || errorText(err, t('portal.destinationStats.loadFailed'))
    summary.value = null
  } finally {
    if (isActive && requestSeq === summarySeq) loading.value = false
  }
}

async function fetchRanking() {
  try {
    const res = await getUserTrafficRanking(hours.value, 1000, false)
    if (!isActive) return
    ranking.value = readRankingRows(res)
  } catch {
    // The picker keeps 全部用户 and the users it had.
  }
}

function selectUser(userId) {
  selectedUserId.value = Number(userId) || 0
  void fetchSummary()
}

async function refreshAll() {
  await Promise.all([fetchRanking(), fetchSummary()])
}

watch(hours, () => { void refreshAll() })

onMounted(() => {
  void loadSettings()
  void refreshAll()
})

onUnmounted(() => {
  isActive = false
})
</script>

<style scoped>
.destination-settings {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  gap: var(--space-4) var(--space-6);
}

.destination-settings__retention {
  width: 200px;
}

.destination-settings__off {
  margin: 0;
  color: var(--label-2);
}
</style>
