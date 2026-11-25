<template>
  <section v-if="!unavailable" class="network-groups" data-testid="node-network-groups">
    <div class="network-groups__head">
      <div>
        <h2 class="network-groups__title">{{ t('admin.nodes.networkGroups.title') }}</h2>
        <p class="network-groups__summary">{{ t('admin.nodes.networkGroups.summary') }}</p>
      </div>
      <UiButton variant="tertiary" :icon-end="ChevronRight" :as="RouterLink" to="/admin/forward/mesh">{{ t('admin.nodes.networkGroups.manage') }}</UiButton>
    </div>
    <UiDataTable
      :columns="columns"
      :rows="regions"
      :label="t('admin.nodes.networkGroups.title')"
      :row-label="region => region.name"
      :loading="loading"
      :error="error"
      :error-title="t('admin.nodes.networkGroups.loadFailed')"
      :empty-icon="Network"
      :empty-title="t('admin.nodes.networkGroups.empty')"
      activatable
      @row-activate="() => router.push('/admin/forward/mesh')"
      @retry="load"
    >
      <template #cell-name="{ row }">
        <span>{{ row.name }}</span>
        <span class="network-groups__code">{{ row.code }}</span>
      </template>
      <template #cell-state="{ row }">
        <UiBadge :tone="row.enabled ? 'success' : 'neutral'" :label="row.enabled ? t('mesh.page.enabled') : t('mesh.page.disabled')" />
      </template>
    </UiDataTable>
  </section>
</template>

<script setup>
// 网络分组: the regions of the intranet mesh (组网), read only. A node is in
// one node group and, independently, in one mesh region (boundary nodes in
// every region); the regions are managed on the mesh page. Hidden in builds
// without the mesh feature.
import { computed, onMounted, ref } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import { ChevronRight, Network } from '@lucide/vue'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { listKernelMeshRegions } from '@/api/kernel'
import { isFeatureDisabled } from '@/components/forward/meshModel'

const { t } = useAppI18n()
const router = useRouter()
const regions = ref([])
const loading = ref(false)
const error = ref(null)
const unavailable = ref(false)

const homeCount = region => (region.members || []).filter(member => member.home).length

const columns = computed(() => [
  { key: 'name', label: t('admin.nodes.networkGroups.columns.name'), primary: true },
  { key: 'cidr', label: t('admin.nodes.networkGroups.columns.cidr'), nowrap: true },
  { key: 'nodes', label: t('admin.nodes.networkGroups.columns.nodes'), numeric: true, align: 'end', value: homeCount },
  { key: 'state', label: t('admin.nodes.networkGroups.columns.state') }
])

async function load() {
  loading.value = true
  error.value = null
  try {
    regions.value = (await listKernelMeshRegions()) || []
  } catch (cause) {
    if (isFeatureDisabled(cause)) unavailable.value = true
    else error.value = cause
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.network-groups {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  margin-top: var(--space-6);
}

.network-groups__head {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: flex-start;
  justify-content: space-between;
}

.network-groups__title {
  margin: 0;
  font-size: var(--type-body-size);
  font-weight: var(--weight-semibold);
}

.network-groups__summary {
  margin: var(--space-1) 0 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.network-groups__code {
  margin-left: var(--space-2);
  color: var(--label-2);
  font-family: var(--font-mono);
}
</style>
