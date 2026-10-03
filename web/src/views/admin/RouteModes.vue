<template>
  <section class="list-page route-modes" :aria-busy="loading ? 'true' : 'false'">
    <UiPageHeader :title="t('pageTitles.admin.routeModes')" :description="t('routeModes.subtitle')">
      <template #actions>
        <UiIconButton
          :icon="RefreshCw"
          variant="secondary"
          data-testid="refresh-route-modes"
          :label="t('control.actions.refresh')"
          :disabled="loading"
          @click="load()"
        />
        <UiButton :icon="ArrowLeft" data-testid="route-modes-back" @click="router.push('/admin/plugins')">
          {{ t('routeModes.back') }}
        </UiButton>
      </template>
    </UiPageHeader>

    <p v-if="loaded && !canSwitch" class="route-modes__banner" data-testid="route-modes-readonly" role="status">
      {{ t('routeModes.superAdminOnly') }}
    </p>

    <UiErrorState
      v-if="loadError && !packages.length && !loading"
      :title="t('routeModes.loadFailed')"
      :error="loadError"
      @retry="load()"
    />
    <UiEmptyState
      v-else-if="loaded && !packages.length"
      :icon="Waypoints"
      heading-tag="h2"
      :title="t('routeModes.empty.title')"
      :description="t('routeModes.empty.description')"
    />
    <template v-else-if="selectedPackage">
      <div class="route-modes__toolbar">
        <UiSelect
          v-model="selectedPackageID"
          class="route-modes__package"
          size="md"
          data-testid="route-modes-package"
          :label="t('routeModes.package')"
          :options="packageOptions"
          :help="t('routeModes.packageMeta', { version: selectedPackage.version || '—', revision: selectedPackage.config_revision ?? 0 })"
        />
        <div class="route-modes__bulk">
          <UiSelect
            v-model="packageMode"
            class="route-modes__mode"
            size="md"
            data-testid="route-modes-package-mode"
            :label="t('routeModes.packageMode')"
            :options="modeOptions(MODES)"
            :disabled="!canSwitch"
          />
          <UiButton
            variant="primary"
            data-testid="route-modes-set-package"
            :disabled="!canSwitch"
            @click="openSwitch(packageMode, null)"
          >
            {{ t('routeModes.setPackage') }}
          </UiButton>
          <UiButton
            variant="danger-soft"
            :icon="Undo2"
            data-testid="route-modes-rollback"
            :disabled="!canSwitch"
            @click="openRollback()"
          >
            {{ t('routeModes.rollback') }}
          </UiButton>
        </div>
      </div>

      <p v-if="!selectedPackage.enabled" class="route-modes__banner" role="status">
        <UiBadge tone="neutral" :label="t('routeModes.packageDisabled')" />
      </p>
      <p v-if="selectedPackage.error" class="route-modes__banner is-error" role="alert">{{ selectedPackage.error }}</p>
      <p v-if="defaultsNotice" class="route-modes__banner" role="status" data-testid="route-modes-defaults">{{ defaultsNotice }}</p>
      <p v-if="loadError" class="route-modes__banner is-error" role="alert">{{ loadError }}</p>

      <UiDataTable
        :columns="routeColumns"
        :rows="selectedPackage.routes || []"
        row-key="route_id"
        :row-label="route => route.route_id"
        :label="t('routeModes.routesLabel', { package: selectedPackage.package_id })"
        storage-key="admin.routeModes"
        :loading="loading && !loaded"
        :card-fields="5"
        :empty-icon="Waypoints"
        :empty-title="t('routeModes.empty.title')"
        data-testid="route-modes-table"
      >
        <template #cell-route="{ row }">
          <code class="route-modes__id">{{ row.route_id }}</code>
        </template>
        <template #cell-endpoint="{ row }">
          <span class="route-modes__endpoint">
            <strong>{{ row.method }}</strong> <code>{{ row.path }}</code>
            <UiBadge v-if="row.transport === 'websocket'" tone="info" :dot="false" label="WebSocket" />
          </span>
        </template>
        <template #cell-catalog="{ row }">
          <UiBadge :tone="catalogTone(row.catalog)" :dot="false" :label="catalogLabel(row.catalog)" />
        </template>
        <template #cell-configured="{ row }">
          <UiBadge :tone="modeTone(row.configured)" :label="modeLabel(row.configured)" />
        </template>
        <template #cell-effective="{ row }">
          <span class="route-modes__effective" :data-route-effective="row.route_id">
            <UiBadge
              :tone="row.effective !== row.configured ? 'warning' : modeTone(row.effective)"
              :label="row.source === 'default' ? t('routeModes.modeDefault', { mode: modeLabel(row.effective) }) : modeLabel(row.effective)"
              :title="row.source ? t(`routeModes.sources.${row.source}`) : undefined"
              :data-route-source="row.source"
            />
            <span v-if="row.effective !== row.configured" class="route-modes__drift">{{ t('routeModes.effectiveDiffers') }}</span>
            <span v-if="hostDiffers(row)" class="route-modes__drift" data-host-drift>
              {{ t('routeModes.hostEffective', { mode: modeLabel(row.host.effective) }) }}
            </span>
          </span>
        </template>
        <template #cell-shadow="{ row }">
          <span v-if="row.host" class="route-modes__counters">
            {{ row.host.shadow_total ?? 0 }} /
            <span :class="{ 'is-warning': row.host.shadow_mismatch > 0 }">{{ row.host.shadow_mismatch ?? 0 }}</span> /
            <span :class="{ 'is-error': row.host.shadow_errors > 0 }">{{ row.host.shadow_errors ?? 0 }}</span>
          </span>
          <span v-else>—</span>
        </template>
        <template #cell-mismatch="{ row }">
          <span class="route-modes__mismatch" :data-route-mismatch="row.route_id">
            <span
              v-if="row.host && row.host.shadow_total > 0"
              class="route-modes__rate"
              :class="{ 'is-warning': row.host.mismatch_rate > 0 }"
            >
              {{ mismatchRate(row.host.mismatch_rate) }}
            </span>
            <span v-else class="route-modes__hint">—</span>
            <span v-if="lastMismatch(row)" class="route-modes__hint" :title="format.dateTime(lastMismatch(row))">
              {{ t('routeModes.mismatches.lastSeen', { time: format.relativeTime(lastMismatch(row)) }) }}
            </span>
            <UiButton
              v-if="row.mismatch_samples?.stored || row.host?.shadow_mismatch"
              size="sm"
              variant="secondary"
              :icon="FileDiff"
              :data-route-samples="row.route_id"
              @click="openMismatches(row)"
            >
              {{ t('routeModes.mismatches.open', { count: row.mismatch_samples?.stored ?? 0 }) }}
            </UiButton>
          </span>
        </template>
        <template #cell-mode="{ row }">
          <div class="route-modes__row-mode">
            <UiSelect
              :model-value="row.configured"
              size="md"
              :aria-label="t('routeModes.routeMode', { route: row.route_id })"
              :options="modeOptions(rowModes(row))"
              :disabled="!routeSwitchable(row)"
              :data-route-mode="row.route_id"
              @update:model-value="mode => onRowMode(row, mode)"
            />
            <span v-if="row.locked" class="route-modes__hint" :data-route-locked="row.route_id">
              {{ row.locked_reason || lockedLabel(row.locked) }}
            </span>
          </div>
        </template>
      </UiDataTable>

      <UiSection :title="t('routeModes.revisions.title')">
        <UiDataTable
          :columns="revisionColumns"
          :rows="revisions"
          :label="t('routeModes.revisions.label')"
          :row-label="revision => revision.route_id || t('routeModes.wholePackage')"
          storage-key="admin.routeModes.revisions"
          :page-size="20"
          :loading="revisionsLoading"
          :error="revisionsError"
          :error-title="t('routeModes.errors.revisions')"
          :empty-icon="History"
          :empty-title="t('routeModes.revisions.empty')"
          data-testid="route-mode-revisions"
          @retry="loadRevisions()"
        >
          <template #cell-route="{ row }">
            <code v-if="row.route_id">{{ row.route_id }}</code>
            <span v-else>{{ t('routeModes.wholePackage') }}</span>
          </template>
          <template #cell-change="{ row }">
            <span class="route-modes__change">
              <UiBadge :tone="row.action === 'rollback' ? 'warning' : 'neutral'" :dot="false" :label="actionLabel(row.action)" />
              {{ modeLabel(row.from_mode) }} → {{ modeLabel(row.to_mode) }}
            </span>
          </template>
        </UiDataTable>
      </UiSection>
    </template>

    <UiDialog
      :open="dialog.open"
      size="sm"
      data-testid="route-mode-dialog"
      :title="dialogTitle"
      :description="dialogMessage"
      :dismissible="!dialog.saving"
      :close-on-scrim="false"
      @update:open="value => { if (!value) closeDialog() }"
    >
      <UiTextarea
        v-model="dialog.reason"
        data-testid="route-mode-reason"
        :rows="3"
        :label="t('routeModes.dialog.reason')"
        :required="reasonRequired"
        :help="reasonRequired ? t('routeModes.dialog.reasonRequired') : t('routeModes.dialog.reasonHelp')"
        :disabled="dialog.saving"
      />
      <p v-if="dialog.error" class="form-error" role="alert" data-testid="route-mode-error">{{ dialog.error }}</p>
      <template #footer>
        <UiButton :disabled="dialog.saving" @click="closeDialog">{{ t('common.actions.cancel') }}</UiButton>
        <UiButton
          :variant="dialog.kind === 'rollback' ? 'danger' : 'primary'"
          data-testid="route-mode-submit"
          :loading="dialog.saving"
          :disabled="!dialogReady"
          @click="submitDialog"
        >
          {{ dialogConfirmLabel }}
        </UiButton>
      </template>
    </UiDialog>

    <RouteMismatchSheet
      v-if="mismatchSheet.loaded"
      v-model:open="mismatchSheet.open"
      :package-id="mismatchSheet.packageID"
      :route="mismatchSheet.route"
    />
  </section>
</template>

<script setup>
import { computed, defineAsyncComponent, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ArrowLeft, FileDiff, History, RefreshCw, Undo2, Waypoints } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiDialog from '@/ui/UiDialog.vue'
import UiEmptyState from '@/ui/UiEmptyState.vue'
import UiErrorState from '@/ui/UiErrorState.vue'
import UiIconButton from '@/ui/UiIconButton.vue'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import UiSection from '@/ui/UiSection.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiTextarea from '@/ui/UiTextarea.vue'
import { useFormat } from '@/ui/composables/useFormat'
import { useToast } from '@/ui/composables/useToast'
import {
  getKernelRouteModeRevisions,
  getKernelRouteModes,
  rollbackKernelRouteModes,
  setKernelRouteModes,
} from '@/api/kernel'

// The samples sheet is its own chunk, loaded when it is first opened.
const RouteMismatchSheet = defineAsyncComponent(() => import('@/components/admin/RouteMismatchSheet.vue'))

const MODES = ['legacy', 'shadow', 'native']
const REVISION_LIMIT = 100

const { t } = useAppI18n()
const router = useRouter()
const toast = useToast()
const format = useFormat()

const canSwitch = ref(false)
const packages = ref([])
const selectedPackageID = ref('')
const packageMode = ref('shadow')
const loading = ref(false)
const loaded = ref(false)
const loadError = ref('')
const revisions = ref([])
const revisionsLoading = ref(false)
const revisionsError = ref('')
const dialog = reactive({ open: false, kind: 'set', mode: '', routes: null, reason: '', saving: false, error: '' })
const mismatchSheet = reactive({ loaded: false, open: false, packageID: '', route: null })

let revisionsRequest = 0

const selectedPackage = computed(() => packages.value.find(item => item.package_id === selectedPackageID.value) || null)
const packageOptions = computed(() => packages.value.map(item => ({
  value: item.package_id,
  label: item.enabled === false ? `${item.package_id} (${t('routeModes.packageDisabled')})` : item.package_id
})))

const routeColumns = computed(() => [
  { key: 'route', label: t('routeModes.columns.route'), primary: true, value: row => row.route_id, sortable: true, nowrap: true },
  { key: 'endpoint', label: t('routeModes.columns.endpoint'), secondary: true, value: row => `${row.method} ${row.path}`, sortable: true },
  { key: 'catalog', label: t('routeModes.columns.catalog'), value: row => row.catalog, sortable: true, nowrap: true },
  { key: 'configured', label: t('routeModes.columns.configured'), value: row => row.configured, sortable: true, nowrap: true },
  { key: 'effective', label: t('routeModes.columns.effective'), value: row => row.effective, sortable: true },
  { key: 'shadow', label: t('routeModes.columns.shadow'), value: row => row.host?.shadow_total ?? -1, numeric: true, nowrap: true, breakpoint: 'lg' },
  { key: 'mismatch', label: t('routeModes.columns.mismatch'), value: row => (row.host?.shadow_total ? row.host.mismatch_rate ?? 0 : -1), sortable: true, firstDirection: 'desc' },
  { key: 'mode', label: t('routeModes.columns.mode'), hideable: false, minWidth: '150px' }
])

const revisionColumns = computed(() => [
  { key: 'time', label: t('routeModes.revisions.time'), value: row => row.created_at, format: value => format.dateTime(value), sortable: true, firstDirection: 'desc', nowrap: true },
  { key: 'route', label: t('routeModes.revisions.route'), primary: true, value: row => row.route_id || '' },
  { key: 'change', label: t('routeModes.revisions.change'), value: row => `${row.from_mode} ${row.to_mode}`, nowrap: true },
  { key: 'actor', label: t('routeModes.revisions.actor'), value: row => row.actor || (row.actor_user_id ? `#${row.actor_user_id}` : '—') },
  { key: 'reason', label: t('routeModes.revisions.reason'), value: row => row.reason || '—', truncate: true, maxWidth: '280px' },
  { key: 'revision', label: t('routeModes.revisions.revision'), value: row => row.config_revision, numeric: true, breakpoint: 'lg' }
])

const reasonRequired = computed(() => dialog.kind === 'set' && dialog.mode === 'native')
const dialogReady = computed(() => !reasonRequired.value || dialog.reason.trim() !== '')
const dialogTitle = computed(() => {
  const packageID = selectedPackage.value?.package_id || ''
  if (dialog.kind === 'rollback') return t('routeModes.dialog.rollbackTitle', { package: packageID })
  const target = dialog.routes?.length === 1 ? dialog.routes[0] : t('routeModes.wholePackageTarget', { package: packageID })
  return t('routeModes.dialog.setTitle', { target, mode: modeLabel(dialog.mode) })
})
const dialogMessage = computed(() => {
  if (dialog.kind === 'rollback') return t('routeModes.dialog.rollbackMessage')
  return dialog.mode === 'native' ? t('routeModes.dialog.nativeMessage') : t('routeModes.dialog.setMessage')
})
const dialogConfirmLabel = computed(() => {
  if (dialog.kind === 'rollback') return t('routeModes.dialog.confirmRollback')
  return dialog.mode === 'native' ? t('routeModes.dialog.confirmNative') : t('routeModes.dialog.confirmSet')
})

const defaultsNotice = computed(() => {
  const defaults = selectedPackage.value?.defaults
  if (defaults?.source === 'kill-switch') return t('routeModes.defaults.kill-switch')
  if (defaults?.source === 'package-too-old') {
    return t('routeModes.defaults.package-too-old', { version: selectedPackage.value.version || '—', min: defaults.min_version })
  }
  return ''
})

function modeLabel(mode) {
  return MODES.includes(mode) ? t(`routeModes.modes.${mode}`) : (mode || '—')
}

function modeTone(mode) {
  if (mode === 'native') return 'success'
  if (mode === 'shadow') return 'info'
  return 'neutral'
}

function modeOptions(modes) {
  return modes.map(mode => ({ value: mode, label: modeLabel(mode) }))
}

function catalogLabel(catalog) {
  return ['native-flagged', 'bridged', 'kernel-owned', 'native'].includes(catalog)
    ? t(`routeModes.catalog.${catalog}`)
    : t('routeModes.catalog.none')
}

function catalogTone(catalog) {
  if (catalog === 'native-flagged' || catalog === 'native') return 'success'
  if (catalog === 'bridged') return 'info'
  return 'neutral'
}

function lockedLabel(locked) {
  return ['kernel_owned', 'identity_group_a', 'websocket', 'not_declared'].includes(locked) ? t(`routeModes.locked.${locked}`) : locked
}

function actionLabel(action) {
  return action === 'rollback' ? t('routeModes.revisions.actions.rollback') : t('routeModes.revisions.actions.set')
}

function hostDiffers(row) {
  return Boolean(row.host?.effective) && row.host.effective !== row.effective
}

// Small non-zero rates keep enough digits not to read as 0%.
function mismatchRate(rate) {
  const value = Number(rate) || 0
  const precision = value === 0 || value >= 0.1 ? 0 : value >= 0.001 ? 1 : 2
  return format.percent(value, { precision })
}

function lastMismatch(row) {
  const times = [row.host?.last_mismatch_at, row.mismatch_samples?.last_observed_at]
    .filter(Boolean)
    .map(value => new Date(value))
    .filter(value => !Number.isNaN(value.getTime()) && value.getTime() > 0)
  if (!times.length) return null
  return new Date(Math.max(...times.map(value => value.getTime())))
}

function openMismatches(row) {
  if (!selectedPackage.value) return
  Object.assign(mismatchSheet, {
    loaded: true,
    open: true,
    packageID: selectedPackage.value.package_id,
    route: { route_id: row.route_id, method: row.method, path: row.path }
  })
}

function rowModes(row) {
  const allowed = Array.isArray(row.allowed_modes) ? row.allowed_modes : []
  // The current mode stays listed so the select can show it.
  return MODES.filter(mode => allowed.includes(mode) || mode === row.configured)
}

function routeSwitchable(row) {
  return canSwitch.value && !row.locked && Array.isArray(row.allowed_modes) && row.allowed_modes.length > 0
}

function errorMessage(cause, fallbackKey) {
  const response = cause?.response?.data
  return response?.error?.message || (typeof response?.error === 'string' ? response.error : '') || response?.message || cause?.message || t(fallbackKey)
}

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    const data = await getKernelRouteModes()
    canSwitch.value = data?.can_switch === true
    packages.value = Array.isArray(data?.packages) ? data.packages : []
    if (!packages.value.some(item => item.package_id === selectedPackageID.value)) {
      selectedPackageID.value = packages.value[0]?.package_id || ''
    }
    loaded.value = true
  } catch (cause) {
    loadError.value = errorMessage(cause, 'routeModes.loadFailed')
  } finally {
    loading.value = false
  }
}

async function loadRevisions() {
  const packageID = selectedPackageID.value
  const request = ++revisionsRequest
  if (!packageID) {
    revisions.value = []
    return
  }
  revisionsLoading.value = true
  revisionsError.value = ''
  try {
    const data = await getKernelRouteModeRevisions(packageID, REVISION_LIMIT)
    if (request !== revisionsRequest) return
    revisions.value = Array.isArray(data?.revisions) ? data.revisions : []
  } catch (cause) {
    if (request !== revisionsRequest) return
    revisionsError.value = errorMessage(cause, 'routeModes.errors.revisions')
  } finally {
    if (request === revisionsRequest) revisionsLoading.value = false
  }
}

watch(selectedPackageID, () => loadRevisions())

function openSwitch(mode, routes) {
  if (!canSwitch.value || !selectedPackage.value || !MODES.includes(mode)) return
  Object.assign(dialog, { open: true, kind: 'set', mode, routes, reason: '', saving: false, error: '' })
}

function onRowMode(row, mode) {
  if (!mode || mode === row.configured || !routeSwitchable(row)) return
  openSwitch(mode, [row.route_id])
}

function openRollback() {
  if (!canSwitch.value || !selectedPackage.value) return
  Object.assign(dialog, { open: true, kind: 'rollback', mode: 'legacy', routes: null, reason: '', saving: false, error: '' })
}

function closeDialog() {
  if (!dialog.saving) dialog.open = false
}

async function submitDialog() {
  if (!dialogReady.value || dialog.saving || !selectedPackage.value) return
  const packageID = selectedPackage.value.package_id
  const reason = dialog.reason.trim()
  dialog.saving = true
  dialog.error = ''
  try {
    let result
    if (dialog.kind === 'rollback') {
      result = await rollbackKernelRouteModes(packageID, reason)
    } else {
      const input = { package_id: packageID, mode: dialog.mode }
      if (dialog.routes?.length) input.routes = [...dialog.routes]
      if (reason) input.reason = reason
      if (dialog.mode === 'native') input.confirm = true
      result = await setKernelRouteModes(input)
    }
    dialog.saving = false
    dialog.open = false
    toast.success(t('routeModes.result', {
      changed: result?.changes?.length ?? 0,
      skipped: result?.skipped?.length ?? 0,
      revision: result?.config_revision ?? '—'
    }))
    await Promise.all([load(), loadRevisions()])
  } catch (cause) {
    dialog.saving = false
    dialog.error = errorMessage(cause, dialog.kind === 'rollback' ? 'routeModes.errors.rollback' : 'routeModes.errors.switch')
    // A concurrent change: refresh what is shown behind the dialog.
    if (cause?.response?.data?.error?.code === 'configuration_revision_conflict') load()
  }
}

onMounted(() => load())

defineExpose({ load })
</script>

<style scoped>
.route-modes__toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-4);
  align-items: flex-end;
  justify-content: space-between;
}

.route-modes__package {
  flex: 1 1 240px;
  max-width: 360px;
}

.route-modes__bulk {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: flex-end;
}

.route-modes__mode {
  min-width: 160px;
}

.route-modes__banner {
  margin: 0;
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-sm);
  background: var(--fill-1);
  color: var(--label-1);
  font-size: var(--type-callout-size);
  overflow-wrap: anywhere;
}

.route-modes__banner.is-error {
  background: var(--danger-soft);
  color: color-mix(in srgb, var(--danger) 78%, var(--label-1));
}

.route-modes__id {
  overflow-wrap: anywhere;
}

.route-modes__endpoint,
.route-modes__effective,
.route-modes__change {
  display: inline-flex;
  flex-wrap: wrap;
  gap: var(--space-1) var(--space-2);
  align-items: center;
  min-width: 0;
}

.route-modes__endpoint code {
  overflow-wrap: anywhere;
}

.route-modes__drift {
  color: var(--warning);
  font-size: var(--type-caption-size);
}

.route-modes__counters {
  font-variant-numeric: tabular-nums;
}

.route-modes__counters .is-warning {
  color: var(--warning);
}

.route-modes__counters .is-error {
  color: var(--danger);
}

.route-modes__mismatch {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  align-items: flex-start;
  min-width: 0;
}

.route-modes__rate {
  font-variant-numeric: tabular-nums;
}

.route-modes__rate.is-warning {
  color: var(--warning);
}

.route-modes__row-mode {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  min-width: 0;
}

.route-modes__hint {
  color: var(--label-2);
  font-size: var(--type-caption-size);
  overflow-wrap: anywhere;
}

@media (max-width: 639.98px) {
  .route-modes__package {
    max-width: none;
  }

  .route-modes__bulk {
    width: 100%;
  }

  .route-modes__mode {
    flex: 1 1 160px;
  }
}
</style>
