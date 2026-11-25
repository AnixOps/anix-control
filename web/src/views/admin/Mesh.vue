<template>
  <section class="mesh-page" data-testid="mesh">
    <UiPageHeader :title="t('mesh.title')" :description="t('mesh.description')">
      <template #actions>
        <UiIconButton :icon="RefreshCw" :label="t('mesh.refresh')" :disabled="loading" data-testid="mesh-refresh" @click="load" />
      </template>
    </UiPageHeader>

    <UiSegmentedControl v-model="tab" :options="tabs" :aria-label="t('mesh.title')" data-testid="mesh-tabs" />

    <UiSkeleton v-if="loading && !overview" variant="card" :label="t('mesh.loading')" />
    <UiErrorState v-else-if="loadError && !overview" :title="t('mesh.loadFailed')" :error="loadError" @retry="load" />
    <template v-else-if="overview">
      <MeshRegions v-if="tab === 'regions'" :regions="overview.regions" @changed="load" />
      <MeshMembers v-else-if="tab === 'members'" :regions="overview.regions" @changed="load" />
      <MeshHostGroups v-else-if="tab === 'host-groups'" :regions="overview.regions" @changed="load" />
      <MeshNames v-else-if="tab === 'names'" />
      <MeshChanges v-else-if="tab === 'changes'" :regions="overview.regions" @changed="load" />
      <MeshSettings v-else-if="tab === 'settings'" :settings="overview.settings" @changed="load" />
    </template>
  </section>
</template>

<script setup>
// 网络 → 组网: the intranet mesh's regions, members and their addresses,
// host groups and device names, the intranet names, region changes and the
// settings (/api/v5/mesh, docs/design/mesh-regions).
import { computed, onMounted, ref } from 'vue'
import { RefreshCw } from '@lucide/vue'
import { getMesh } from '@/api/mesh'
import { useAppI18n } from '@/composables/useAppI18n'
import UiErrorState from '@/ui/UiErrorState.vue'
import UiIconButton from '@/ui/UiIconButton.vue'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import UiSegmentedControl from '@/ui/UiSegmentedControl.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import MeshChanges from './mesh/MeshChanges.vue'
import MeshHostGroups from './mesh/MeshHostGroups.vue'
import MeshMembers from './mesh/MeshMembers.vue'
import MeshNames from './mesh/MeshNames.vue'
import MeshRegions from './mesh/MeshRegions.vue'
import MeshSettings from './mesh/MeshSettings.vue'

const { t } = useAppI18n()
const overview = ref(null)
const loading = ref(true)
const loadError = ref(null)
const tab = ref('regions')

const tabs = computed(() => [
  { value: 'regions', label: t('mesh.tabs.regions') },
  { value: 'members', label: t('mesh.tabs.members') },
  { value: 'host-groups', label: t('mesh.tabs.hostGroups') },
  { value: 'names', label: t('mesh.tabs.names') },
  {
    value: 'changes',
    label: overview.value?.pending_changes
      ? t('mesh.tabs.changesPending', { n: overview.value.pending_changes })
      : t('mesh.tabs.changes')
  },
  { value: 'settings', label: t('mesh.tabs.settings') }
])

async function load() {
  loading.value = true
  loadError.value = null
  try {
    overview.value = await getMesh()
  } catch (e) {
    loadError.value = e
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.mesh-page {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}
</style>
