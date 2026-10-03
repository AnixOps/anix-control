<template>
  <UiSection :title="t('admin.nodes.services.title')" :description="t('admin.nodes.services.description')">
    <template #actions>
      <UiButton v-if="data" :icon="Settings2" data-testid="services-settings" @click="openSettings">{{ t('admin.nodes.services.settings') }}</UiButton>
      <UiIconButton variant="secondary" :icon="RefreshCw" :label="t('admin.nodes.services.refresh')" :disabled="loading" data-testid="refresh-services" @click="load" />
    </template>

    <UiEmptyState
      v-if="data && !data.enabled"
      :icon="Power"
      heading-tag="h3"
      :title="t('admin.nodes.services.disabled.title')"
      :description="t('admin.nodes.services.disabled.description')"
      data-testid="services-disabled"
    >
      <template #actions>
        <UiButton variant="primary" :loading="saving" data-testid="enable-services" @click="enable">{{ t('admin.nodes.services.disabled.enable') }}</UiButton>
        <UiButton @click="openSettings">{{ t('admin.nodes.services.settings') }}</UiButton>
      </template>
    </UiEmptyState>
    <UiEmptyState
      v-else-if="data && data.reported && !data.supported"
      :icon="CircleSlash"
      heading-tag="h3"
      :title="t('admin.nodes.services.unsupported')"
      :description="data.unsupported_reason ? t('admin.nodes.services.unsupportedReason', { reason: data.unsupported_reason }) : t('admin.nodes.services.unsupportedNoReason')"
      data-testid="services-unsupported"
    />
    <UiEmptyState
      v-else-if="data && !data.reported"
      :icon="Hourglass"
      heading-tag="h3"
      :title="t('admin.nodes.services.waiting')"
      :description="t('admin.nodes.services.waitingDescription')"
      data-testid="services-waiting"
    />
    <template v-else>
      <NodeNotice v-if="data?.stale" data-testid="services-stale">
        {{ t('admin.nodes.services.stale', { time: format.relativeTime(data.observed_at) }) }}
      </NodeNotice>
      <UiDataTable
        :columns="columns"
        :rows="visibleUnits"
        row-key="name"
        :label="t('admin.nodes.services.tableLabel', { name: node.name })"
        storage-key="admin.node-services"
        :sort="sort"
        :loading="loading"
        :error="error"
        :error-title="t('admin.nodes.services.loadFailed')"
        :filtered="Boolean(search || stateFilter)"
        :empty-icon="ServerCog"
        :empty-title="t('admin.nodes.services.empty')"
        :empty-description="t('admin.nodes.services.emptyDescription')"
        state-heading-tag="h3"
        default-density="compact"
        @update:sort="value => { sort = value }"
        @retry="load"
        @clear-filters="clearFilters"
      >
        <template #toolbar>
          <UiSearchField
            v-model="search"
            class="list-page__search"
            :label="t('admin.nodes.services.filters.search')"
            :placeholder="t('admin.nodes.services.filters.searchPlaceholder')"
            :shortcut="false"
          />
          <UiFilterChips v-model="stateFilter" :label="t('admin.nodes.services.filters.label')" :options="stateChips" />
        </template>
        <template #cell-state="{ row }">
          <span class="node-services__state">
            <UiBadge :tone="stateTone(row.active_state)" :label="stateLabel(row.active_state)" />
            <span class="node-services__sub">{{ row.sub_state }}</span>
          </span>
        </template>
      </UiDataTable>
      <p v-if="data" class="node-services__totals tabular-nums" data-testid="services-totals">
        <span>{{ t('admin.nodes.services.totals', { total: data.summary.total, failed: data.summary.failed }) }}</span>
        <span v-if="data.observed_at" class="node-services__observed" :title="format.dateTime(data.observed_at)">{{ t('admin.nodes.services.observedAt', { time: format.relativeTime(data.observed_at) }) }}</span>
      </p>
    </template>

    <UiSheet
      v-model:open="settingsOpen"
      :title="t('admin.nodes.services.form.title')"
      :description="t('admin.nodes.services.form.description', { name: node.name })"
    >
      <form class="node-services__form" data-testid="services-form" @submit.prevent="saveSettings">
        <UiSwitch
          v-model="form.enabled"
          :label="t('admin.nodes.services.form.enabled')"
          :description="t('admin.nodes.services.form.enabledHelp')"
          data-testid="services-enabled"
        />
        <UiTextarea
          v-model="form.include"
          :label="t('admin.nodes.services.form.include')"
          :help="t('admin.nodes.services.form.includeHelp')"
          :error="includeError"
          placeholder="nginx*.service"
          :rows="4"
          data-testid="services-include"
        />
        <UiTextarea
          v-model="form.exclude"
          :label="t('admin.nodes.services.form.exclude')"
          :help="t('admin.nodes.services.form.excludeHelp')"
          :error="excludeError"
          :rows="4"
          data-testid="services-exclude"
        />
        <p v-if="saveError" class="node-services__save-error" role="alert">{{ saveError }}</p>
      </form>
      <template #footer>
        <UiButton :disabled="saving" @click="settingsOpen = false">{{ t('common.actions.cancel') }}</UiButton>
        <UiButton
          variant="primary"
          :loading="saving"
          :disabled="Boolean(includeError || excludeError)"
          data-testid="save-services"
          @click="saveSettings"
        >{{ t('admin.nodes.services.form.save') }}</UiButton>
      </template>
    </UiSheet>
  </UiSection>
</template>

<script setup>
// 服务 section of the node page: the node's systemd services as the
// machine-telemetry package reports them (GET
// /api/v3/plugins/machine-telemetry/nodes/:id/services). Read-only: there is
// no start, stop or restart. Collection is off on every node until an
// administrator enables it here; the switch and the include / exclude
// patterns are saved in the package's Agent installation configuration.
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { CircleSlash, Hourglass, Power, RefreshCw, ServerCog, Settings2 } from '@lucide/vue'
import { MAX_GLOBS, getNodeServices, saveNodeServicesSettings, validGlob } from '@/api/machineTelemetry'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiEmptyState from '@/ui/UiEmptyState.vue'
import UiFilterChips from '@/ui/UiFilterChips.vue'
import UiIconButton from '@/ui/UiIconButton.vue'
import UiSearchField from '@/ui/UiSearchField.vue'
import UiSection from '@/ui/UiSection.vue'
import UiSheet from '@/ui/UiSheet.vue'
import UiSwitch from '@/ui/UiSwitch.vue'
import UiTextarea from '@/ui/UiTextarea.vue'
import { useFormat } from '@/ui/composables/useFormat'
import { useToast } from '@/ui/composables/useToast'
import { useAppI18n } from '@/composables/useAppI18n'
import NodeNotice from './NodeNotice.vue'
import { readNodeApiError } from './nodeData'

const props = defineProps({
  node: { type: Object, required: true }
})
const { t } = useAppI18n()
const format = useFormat()
const toast = useToast()

const STATES = ['active', 'failed', 'inactive']
const KNOWN_STATES = [...STATES, 'activating', 'deactivating', 'reloading']

const data = ref(null)
const loading = ref(false)
const error = ref(null)
const search = ref('')
const stateFilter = ref('')
const sort = ref({ key: 'name', direction: 'asc' })
const settingsOpen = ref(false)
const saving = ref(false)
const saveError = ref('')
const form = reactive({ enabled: false, include: '', exclude: '' })

const units = computed(() => (Array.isArray(data.value?.units) ? data.value.units : []))
const visibleUnits = computed(() => {
  const needle = search.value.trim().toLowerCase()
  return units.value.filter(unit => (!needle || unit.name.toLowerCase().includes(needle)) &&
    (!stateFilter.value || unit.active_state === stateFilter.value))
})

const stateChips = computed(() => {
  const summary = data.value?.summary || {}
  return [
    { value: '', label: t('admin.nodes.services.filters.all'), count: summary.total ?? 0 },
    ...STATES.map(state => ({ value: state, label: t(`admin.nodes.services.states.${state}`), count: summary[state] ?? 0 }))
  ]
})

const cpu = value => format.percent(Number(value || 0) / 100, { precision: 1 })
const memory = value => format.bytes(value, { precision: 1, empty: '0 B' })

const columns = computed(() => [
  { key: 'name', label: t('admin.nodes.services.columns.name'), primary: true, hideable: false, sortable: true, truncate: true, maxWidth: 320 },
  {
    key: 'state', label: t('admin.nodes.services.columns.state'), secondary: true, sortable: true, nowrap: true,
    value: unit => unit.active_state, sortValue: unit => `${stateRank(unit.active_state)}${unit.active_state}/${unit.sub_state}`
  },
  { key: 'cpu_avg_percent', label: t('admin.nodes.services.columns.cpuAvg'), sortable: true, firstDirection: 'desc', align: 'end', numeric: true, nowrap: true, format: cpu },
  { key: 'cpu_peak_percent', label: t('admin.nodes.services.columns.cpuPeak'), sortable: true, firstDirection: 'desc', align: 'end', numeric: true, nowrap: true, format: cpu, breakpoint: 'md' },
  { key: 'memory_bytes', label: t('admin.nodes.services.columns.memory'), sortable: true, firstDirection: 'desc', align: 'end', numeric: true, nowrap: true, format: memory },
  { key: 'memory_peak_bytes', label: t('admin.nodes.services.columns.memoryPeak'), sortable: true, firstDirection: 'desc', align: 'end', numeric: true, nowrap: true, format: memory, breakpoint: 'md' }
])

// Failed first when sorting by state, then the others.
function stateRank(state) {
  const index = ['failed', 'activating', 'deactivating', 'reloading', 'active', 'inactive'].indexOf(state)
  return index < 0 ? 9 : index
}

function stateLabel(state) {
  return KNOWN_STATES.includes(state) ? t(`admin.nodes.services.states.${state}`) : state
}

function stateTone(state) {
  if (state === 'active') return 'success'
  if (state === 'failed') return 'danger'
  if (state === 'inactive') return 'neutral'
  return 'warning'
}

function clearFilters() {
  search.value = ''
  stateFilter.value = ''
}

async function load() {
  const nodeID = props.node.id
  loading.value = true
  error.value = null
  try {
    const answer = await getNodeServices(nodeID)
    if (props.node.id !== nodeID) return
    data.value = answer && typeof answer === 'object' ? answer : null
  } catch (e) {
    if (props.node.id !== nodeID) return
    error.value = e
  } finally {
    loading.value = false
  }
}

const lines = text => text.split('\n').map(line => line.trim()).filter(Boolean)

function globError(text) {
  const globs = lines(text)
  if (globs.length > MAX_GLOBS) return t('admin.nodes.services.form.tooManyGlobs', { max: MAX_GLOBS })
  const invalid = globs.find(glob => !validGlob(glob))
  return invalid ? t('admin.nodes.services.form.invalidGlob', { glob: invalid }) : ''
}

const includeError = computed(() => globError(form.include))
const excludeError = computed(() => globError(form.exclude))

function openSettings() {
  form.enabled = Boolean(data.value?.enabled)
  form.include = (data.value?.include || []).join('\n')
  form.exclude = (data.value?.exclude || []).join('\n')
  saveError.value = ''
  settingsOpen.value = true
}

function saveMessage(e) {
  if (e?.code === 'no_installation') return t('admin.nodes.services.form.noInstallation')
  if (e?.response?.status === 409) return t('admin.nodes.services.form.conflict')
  const message = e?.response?.data?.error?.message || readNodeApiError(e)
  return `${t('admin.nodes.services.form.saveFailed')}: ${message}`
}

async function save(settings) {
  saving.value = true
  saveError.value = ''
  try {
    await saveNodeServicesSettings(props.node.id, settings)
    toast.success(t('admin.nodes.services.form.saved'))
    settingsOpen.value = false
    await load()
    return true
  } catch (e) {
    saveError.value = saveMessage(e)
    if (!settingsOpen.value) toast.error(saveError.value)
    if (e?.response?.status === 409) await load()
    return false
  } finally {
    saving.value = false
  }
}

function enable() {
  return save({ enabled: true, include: data.value?.include || [], exclude: data.value?.exclude || [] })
}

function saveSettings() {
  if (includeError.value || excludeError.value) return
  return save({ enabled: form.enabled, include: lines(form.include), exclude: lines(form.exclude) })
}

watch(() => props.node.id, () => {
  data.value = null
  load()
})
onMounted(load)

defineExpose({ load })
</script>

<style scoped>
.node-services__state {
  display: inline-flex;
  gap: var(--space-2);
  align-items: center;
}

.node-services__sub {
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

.node-services__totals {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2) var(--space-4);
  justify-content: space-between;
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

.node-services__form {
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
}

.node-services__save-error {
  margin: 0;
  color: var(--danger);
  font-size: var(--type-caption-size);
}
</style>
