<template>
  <section class="fwd-page" data-testid="mesh-regions">
    <ForwardAreaNav area="mesh" :seconds-ago="secondsAgo" :loading="loading" refreshable @refresh="refresh" />

    <UiPageHeader :title="t('mesh.page.title')" :description="t('mesh.page.description')">
      <template v-if="!unavailable" #actions>
        <UiButton :icon="Plus" data-testid="mesh-region-add" @click="openRegion(null)">{{ t('mesh.page.addRegion') }}</UiButton>
        <UiButton :icon="KeyRound" :disabled="!regions.length" data-testid="mesh-enrollment-add" @click="enrollmentSheetOpen = true">{{ t('mesh.page.issueEnrollment') }}</UiButton>
        <UiButton variant="primary" :icon="Plus" :disabled="!regions.length" data-testid="mesh-member-add" @click="openMember(null)">{{ t('mesh.page.addMember') }}</UiButton>
      </template>
    </UiPageHeader>

    <UiEmptyState v-if="unavailable" :icon="Network" heading-tag="h2" :title="t('mesh.page.unavailable')" />
    <template v-else>
      <h2 class="mesh-heading">{{ t('mesh.page.regions') }}</h2>
      <UiDataTable
        :columns="regionColumns"
        :rows="regions"
        row-key="id"
        :label="t('mesh.page.regions')"
        :row-label="row => row.name"
        :row-actions="regionActions"
        :loading="loading && !loaded"
        :error="loadError"
        :error-title="t('mesh.page.loadFailed')"
        :empty-icon="Network"
        :empty-title="t('mesh.page.emptyRegions')"
        :card-fields="3"
        data-testid="mesh-region-table"
        @retry="refresh"
      >
        <template #cell-region="{ row }">
          <span class="fwd-cell-stack">
            <span class="mesh-name">{{ row.name }}</span>
            <span class="fwd-muted fwd-mono">{{ row.code }}</span>
          </span>
        </template>
        <template #cell-cidr="{ row }">
          <span class="fwd-mono">{{ row.cidr }}</span>
        </template>
        <template #cell-members="{ row }">
          <span class="fwd-cell-stack">
            <span>{{ t('mesh.page.memberCount', { n: homeCount(row) }, homeCount(row)) }}</span>
            <span class="fwd-muted">{{ t('mesh.page.boundaryCount', { n: boundaryCount(row) }, boundaryCount(row)) }}</span>
          </span>
        </template>
        <template #cell-state="{ row }">
          <span class="mesh-badges">
            <UiBadge :tone="row.enabled ? 'success' : 'neutral'" :label="row.enabled ? t('mesh.page.enabled') : t('mesh.page.disabled')" />
            <UiBadge v-if="row.enabled && boundaryCount(row) < 2" tone="warning" :label="t('mesh.page.fewBoundaries')" />
          </span>
        </template>
      </UiDataTable>

      <h2 class="mesh-heading">{{ t('mesh.page.members') }}</h2>
      <UiDataTable
        :columns="memberColumns"
        :rows="members"
        row-key="id"
        :label="t('mesh.page.members')"
        :row-label="row => row.node_name || row.node_ref"
        :row-actions="memberActions"
        :loading="loading && !loaded"
        :empty-icon="Network"
        :empty-title="t('mesh.page.emptyMembers')"
        :empty-description="t('mesh.page.emptyMembersDescription')"
        :card-fields="4"
        data-testid="mesh-member-table"
      >
        <template #cell-node="{ row }">
          <span class="fwd-cell-stack">
            <span class="mesh-name">{{ row.node_name || row.node_ref }}</span>
            <span class="fwd-muted">{{ row.node_host || row.node_ref }}</span>
          </span>
        </template>
        <template #cell-address="{ row }">
          <span class="fwd-cell-stack">
            <span class="fwd-mono">{{ row.region.name }} · {{ row.address }}</span>
            <span v-for="other in row.others" :key="other.region.id" class="fwd-muted fwd-mono">{{ other.region.name }} · {{ other.address }}</span>
          </span>
        </template>
        <template #cell-role="{ row }">
          {{ row.boundary ? t('mesh.page.boundary') : t('mesh.page.member') }}
        </template>
        <template #cell-status="{ row }">
          <span class="mesh-badges">
            <UiBadge :tone="STATE_TONES[summary(row).state]" :label="t(`mesh.page.status.${summary(row).state}`)" />
            <UiBadge v-for="code in summary(row).alerts" :key="code" tone="danger" :label="alertLabel(code)" />
          </span>
        </template>
      </UiDataTable>

      <template v-if="pendingChanges.length">
        <h2 class="mesh-heading">{{ t('mesh.page.regionChanges') }}</h2>
        <UiDataTable
          :columns="changeColumns"
          :rows="pendingChanges"
          row-key="id"
          :label="t('mesh.page.regionChanges')"
          :row-label="row => deviceName(row.device_id)"
          :row-actions="changeActions"
          :card-fields="3"
          data-testid="mesh-change-table"
        >
          <template #cell-device="{ row }">{{ deviceName(row.device_id) }}</template>
          <template #cell-move="{ row }">{{ regionName(row.from_region_id) }} → {{ regionName(row.to_region_id) }}</template>
          <template #cell-reason="{ row }"><span class="fwd-muted">{{ row.reason || '—' }}</span></template>
        </UiDataTable>
      </template>

      <h2 class="mesh-heading">{{ t('mesh.page.devices') }}</h2>
      <UiDataTable
        :columns="deviceColumns"
        :rows="devices"
        row-key="id"
        :label="t('mesh.page.devices')"
        :row-label="row => row.name"
        :row-actions="deviceActions"
        :loading="loading && !loaded"
        :empty-icon="Laptop"
        :empty-title="t('mesh.page.emptyDevices')"
        :empty-description="t('mesh.page.emptyDevicesDescription')"
        :card-fields="4"
        data-testid="mesh-device-table"
      >
        <template #cell-device="{ row }">
          <span class="fwd-cell-stack">
            <span class="mesh-name">{{ row.name }}</span>
            <span class="fwd-muted">{{ t(`mesh.device.kinds.${row.kind}`) }}</span>
          </span>
        </template>
        <template #cell-address="{ row }">
          <span class="fwd-mono">{{ row.region.name }} · {{ row.address || t('mesh.page.noAddress') }}</span>
        </template>
        <template #cell-status="{ row }">
          <span class="mesh-badges">
            <UiBadge :tone="STATE_TONES[summary(row).state]" :label="t(`mesh.page.status.${summary(row).state}`)" />
            <UiBadge v-for="code in summary(row).alerts" :key="code" tone="danger" :label="alertLabel(code)" />
          </span>
        </template>
      </UiDataTable>

      <template v-if="enrollments.length">
        <h2 class="mesh-heading">{{ t('mesh.page.enrollments') }}</h2>
        <UiDataTable
          :columns="enrollmentColumns"
          :rows="enrollments"
          row-key="id"
          :label="t('mesh.page.enrollments')"
          :row-label="row => row.name"
          :row-actions="enrollmentActions"
          :card-fields="3"
          data-testid="mesh-enrollment-table"
        >
          <template #cell-name="{ row }">
            <span class="fwd-cell-stack">
              <span class="mesh-name">{{ row.name }}</span>
              <span class="fwd-muted">{{ t(`mesh.device.kinds.${row.kind}`) }}</span>
            </span>
          </template>
          <template #cell-region="{ row }">{{ row.region_id ? regionName(row.region_id) : t('mesh.enrollment.anyRegion') }}</template>
          <template #cell-state="{ row }">
            <UiBadge :tone="ENROLLMENT_TONES[enrollmentState(row)]" :label="t(`mesh.enrollment.states.${enrollmentState(row)}`)" />
          </template>
        </UiDataTable>
      </template>

      <p class="list-page__note">{{ t('mesh.page.note') }}</p>
    </template>

    <MeshRegionSheet v-model:open="regionSheetOpen" :region="editingRegion" @saved="refresh" />
    <MeshEnrollmentSheet v-model:open="enrollmentSheetOpen" :regions="regions" @saved="refresh" />
    <MeshMemberSheet v-model:open="memberSheetOpen" :member="editingMember" :regions="regions" :nodes="freeNodes" @saved="refresh" />
  </section>
</template>

<script setup>
// 组网: the mesh regions (internal/meshregion) and the nodes in them, with
// the status each node's Agent reports. A change reaches the nodes at once.
import { computed, ref } from 'vue'
import { Ban, Check, KeyRound, Laptop, MoveRight, Network, Pencil, Plus, Power, Shield, Trash2, X } from '@lucide/vue'
import { UiBadge, UiButton, UiDataTable, UiEmptyState, UiPageHeader, useConfirm, useToast } from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import {
  decideKernelMeshRegionChange, deleteKernelMeshEnrollment, deleteKernelMeshRegion, getKernelAdminNodes, listKernelMeshEnrollments, listKernelMeshRegionChanges,
  listKernelMeshRegions, moveKernelMeshDevice, removeKernelMeshMember, revokeKernelMeshDevice, saveKernelMeshRegion, updateKernelMeshMember
} from '@/api/kernel'
import ForwardAreaNav from '@/components/forward/ForwardAreaNav.vue'
import MeshEnrollmentSheet from '@/components/forward/MeshEnrollmentSheet.vue'
import MeshMemberSheet from '@/components/forward/MeshMemberSheet.vue'
import MeshRegionSheet from '@/components/forward/MeshRegionSheet.vue'
import { boundaryCount, deviceRows, enrollmentState, isFeatureDisabled, memberRows, meshErrorMessage, statusSummary } from '@/components/forward/meshModel'
import { usePolling } from '@/components/forward/usePolling'
import { readNodePage } from '@/views/admin/nodes/nodeData'
import '@/components/forward/forward.css'

const { t, te } = useAppI18n()
const toast = useToast()
const confirm = useConfirm()

const regions = ref([])
const enrollments = ref([])
const changes = ref([])
const nodes = ref([])
const loaded = ref(false)
const loadError = ref(null)
const unavailable = ref(false)

const { loading, secondsAgo, refresh } = usePolling(async () => {
  try {
    const [answer, enrollmentList, changeList, nodePage] = await Promise.all([
      listKernelMeshRegions(),
      listKernelMeshEnrollments(),
      listKernelMeshRegionChanges(),
      nodes.value.length ? Promise.resolve(null) : getKernelAdminNodes({ page: 1, page_size: 500 })
    ])
    regions.value = (answer || []).map(region => ({ ...region, members: region.members || [], devices: region.devices || [] }))
    enrollments.value = enrollmentList || []
    changes.value = changeList || []
    if (nodePage) nodes.value = (readNodePage(nodePage).list || []).map(node => ({ ref: `proxy-${node.id}`, name: node.name, host: node.host }))
    loaded.value = true
    loadError.value = null
  } catch (error) {
    if (isFeatureDisabled(error)) {
      unavailable.value = true
      return
    }
    loadError.value = { message: meshErrorMessage(error) }
    throw error
  }
}, { interval: 15000 })

const members = computed(() => memberRows(regions.value))
const freeNodes = computed(() => {
  const taken = new Set(members.value.map(member => member.node_ref))
  return nodes.value.filter(node => !taken.has(node.ref))
})
const devices = computed(() => deviceRows(regions.value))
const pendingChanges = computed(() => changes.value.filter(change => change.state === 'pending'))
const regionName = id => regions.value.find(region => region.id === id)?.name || `#${id}`
const deviceName = id => devices.value.find(device => device.id === id)?.name || `#${id}`
const ENROLLMENT_TONES = { valid: 'success', used: 'neutral', expired: 'warning' }
const homeCount = region => region.members.filter(member => member.home).length
const summary = member => statusSummary(member)
const STATE_TONES = { none: 'neutral', running: 'success', stopped: 'warning', lost: 'danger' }
const alertLabel = code => (te(`mesh.page.alerts.${code}`) ? t(`mesh.page.alerts.${code}`) : code)

const regionColumns = computed(() => [
  { key: 'region', label: t('mesh.page.columns.region'), primary: true },
  { key: 'cidr', label: t('mesh.page.columns.cidr'), secondary: true },
  { key: 'members', label: t('mesh.page.columns.members'), breakpoint: 'md' },
  { key: 'state', label: t('mesh.page.columns.state') }
])
const memberColumns = computed(() => [
  { key: 'node', label: t('mesh.page.columns.node'), primary: true },
  { key: 'address', label: t('mesh.page.columns.address'), secondary: true },
  { key: 'role', label: t('mesh.page.columns.role'), breakpoint: 'md' },
  { key: 'status', label: t('mesh.page.columns.status') }
])

const deviceColumns = computed(() => [
  { key: 'device', label: t('mesh.page.columns.device'), primary: true },
  { key: 'address', label: t('mesh.page.columns.address'), secondary: true },
  { key: 'status', label: t('mesh.page.columns.status') }
])
const changeColumns = computed(() => [
  { key: 'device', label: t('mesh.page.columns.device'), primary: true },
  { key: 'move', label: t('mesh.page.columns.move'), secondary: true },
  { key: 'reason', label: t('mesh.page.columns.reason') }
])
const enrollmentColumns = computed(() => [
  { key: 'name', label: t('mesh.page.columns.name'), primary: true },
  { key: 'region', label: t('mesh.page.columns.region'), secondary: true },
  { key: 'state', label: t('mesh.page.columns.state') }
])

const enrollmentSheetOpen = ref(false)
const regionSheetOpen = ref(false)
const editingRegion = ref(null)
function openRegion(region) {
  editingRegion.value = region
  regionSheetOpen.value = true
}
const memberSheetOpen = ref(false)
const editingMember = ref(null)
function openMember(member) {
  editingMember.value = member
  memberSheetOpen.value = true
}

async function run(action) {
  try {
    await action()
    toast.success(t('mesh.toast.saved'))
  } catch (error) {
    toast.error(meshErrorMessage(error))
  }
  await refresh()
}

function regionActions(row) {
  return [
    { key: 'edit', label: t('mesh.page.edit'), icon: Pencil, onSelect: () => openRegion(row) },
    { key: 'toggle', label: row.enabled ? t('mesh.page.disable') : t('mesh.page.enable'), icon: Power, onSelect: () => run(() => saveKernelMeshRegion(row.id, { ...row, enabled: !row.enabled })) },
    {
      key: 'delete',
      label: t('mesh.page.delete'),
      icon: Trash2,
      danger: true,
      separatorBefore: true,
      onSelect: () => confirm({
        title: t('mesh.delete.regionTitle', { name: row.name }),
        message: t('mesh.delete.regionMessage'),
        confirmLabel: t('mesh.delete.confirm'),
        tone: 'danger',
        onConfirm: async () => {
          try {
            await deleteKernelMeshRegion(row.id)
          } catch (error) {
            throw new Error(meshErrorMessage(error))
          }
          toast.success(t('mesh.toast.deleted'))
          await refresh()
        }
      })
    }
  ]
}

function memberActions(row) {
  const name = row.node_name || row.node_ref
  return [
    { key: 'move', label: t('mesh.page.move'), icon: MoveRight, onSelect: () => openMember(row) },
    {
      key: 'boundary',
      label: row.boundary ? t('mesh.page.unsetBoundary') : t('mesh.page.setBoundary'),
      icon: Shield,
      onSelect: () => run(() => updateKernelMeshMember(row.id, { regionId: 0, boundary: !row.boundary }))
    },
    {
      key: 'remove',
      label: t('mesh.page.remove'),
      icon: Trash2,
      danger: true,
      separatorBefore: true,
      onSelect: () => confirm({
        title: t('mesh.delete.memberTitle', { name }),
        message: t('mesh.delete.memberMessage'),
        confirmLabel: t('mesh.delete.confirmRemove'),
        tone: 'danger',
        onConfirm: async () => {
          try {
            await removeKernelMeshMember(row.id)
          } catch (error) {
            throw new Error(meshErrorMessage(error))
          }
          toast.success(t('mesh.toast.removed'))
          await refresh()
        }
      })
    }
  ]
}

function deviceActions(row) {
  return [
    ...regions.value.filter(region => region.id !== row.region_id).map(region => ({
      key: `move-${region.id}`,
      label: t('mesh.page.moveTo', { name: region.name }),
      icon: MoveRight,
      onSelect: () => run(() => moveKernelMeshDevice(row.id, region.id))
    })),
    {
      key: 'revoke',
      label: t('mesh.page.revoke'),
      icon: Ban,
      danger: true,
      separatorBefore: true,
      onSelect: () => confirm({
        title: t('mesh.delete.deviceTitle', { name: row.name }),
        message: t('mesh.delete.deviceMessage'),
        confirmLabel: t('mesh.delete.confirmRevoke'),
        tone: 'danger',
        onConfirm: async () => {
          try {
            await revokeKernelMeshDevice(row.id)
          } catch (error) {
            throw new Error(meshErrorMessage(error))
          }
          toast.success(t('mesh.toast.revoked'))
          await refresh()
        }
      })
    }
  ]
}

function changeActions(row) {
  return [
    { key: 'approve', label: t('mesh.page.approve'), icon: Check, onSelect: () => run(() => decideKernelMeshRegionChange(row.id, true)) },
    { key: 'reject', label: t('mesh.page.reject'), icon: X, danger: true, onSelect: () => run(() => decideKernelMeshRegionChange(row.id, false)) }
  ]
}

function enrollmentActions(row) {
  return [
    { key: 'delete', label: t('mesh.page.withdraw'), icon: Trash2, danger: true, onSelect: () => run(() => deleteKernelMeshEnrollment(row.id)) }
  ]
}
</script>

<style scoped>
.mesh-heading {
  margin: var(--space-4) 0 0;
  font-size: var(--type-body-size);
  font-weight: var(--weight-semibold);
}

.mesh-name {
  font-weight: var(--weight-medium);
}

.mesh-badges {
  display: inline-flex;
  flex-wrap: wrap;
  gap: var(--space-1);
}
</style>
