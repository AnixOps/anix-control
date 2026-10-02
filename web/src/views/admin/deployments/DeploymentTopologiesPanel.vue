<template>
  <section
    id="deployment-panel-topologies"
    class="deployment-panel"
    data-testid="deployment-topology-panel"
    role="tabpanel"
    aria-labelledby="deployment-tab-topologies"
  >
    <UiDataTable
      :columns="columns"
      :rows="topologies"
      :label="t('control.tabs.topologies')"
      :row-label="topology => topology.name || `#${topology.id}`"
      :loading="loading"
      :error="error"
      :error-title="t('control.errors.load')"
      :empty-icon="Waypoints"
      :empty-title="t('control.empty.topologies')"
      :empty-description="t('control.topology.emptyHint')"
      :settings="false"
      :card-fields="8"
      @retry="emit('retry')"
    >
      <template #toolbar>
        <p class="deployment-panel__hint">{{ t('control.topology.select') }}</p>
      </template>
      <template #toolbar-end>
        <UiButton
          variant="primary"
          :icon="Plus"
          data-testid="new-topology"
          :disabled="!canCreate"
          @click="emit('create')"
        >
          {{ t('control.topology.new') }}
        </UiButton>
      </template>
      <template #cell-topology="{ row }">
        <span class="deployment-panel__name">
          <strong>{{ row.name || `#${row.id}` }}</strong>
          <code>#{{ row.id }}</code>
        </span>
      </template>
      <template #cell-scope="{ row }">
        <code>{{ row.service_scope || '-' }}</code>
      </template>
      <template #cell-revision="{ row }">
        <code>{{ row.active_revision_id || '-' }}</code>
      </template>
      <template #cell-deployment="{ row }">
        <UiBadge
          v-if="latestDeploymentFor(row)"
          :tone="deploymentStateTone(latestDeploymentFor(row).state)"
          :label="latestDeploymentFor(row).state || '-'"
        />
        <span v-else class="deployment-panel__muted">{{ t('control.topology.noDeployment') }}</span>
      </template>
      <template #cell-actions="{ row }">
        <span class="deployment-panel__actions">
          <UiButton size="sm" :data-testid="`edit-topology-${row.id}`" @click="emit('edit', row)">
            {{ t('control.topology.edit') }}
          </UiButton>
          <UiButton
            v-if="latestDeploymentFor(row)"
            size="sm"
            :data-testid="`view-deployment-${latestDeploymentFor(row).id}`"
            @click="emit('view-deployment', latestDeploymentFor(row))"
          >
            {{ t('control.topology.status') }}
          </UiButton>
        </span>
      </template>
    </UiDataTable>
  </section>
</template>

<script setup>
// 部署编排 · 拓扑 (UI U8): the topologies with their latest deployment.
// Editing opens the topology workspace; 查看状态 opens it on the deployment.
import { computed } from 'vue'
import { Plus, Waypoints } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import { deploymentStateTone } from './useDeploymentCenter'

defineProps({
  topologies: { type: Array, default: () => [] },
  latestDeploymentFor: { type: Function, required: true },
  loading: { type: Boolean, default: false },
  error: { type: [String, Object, null], default: null },
  canCreate: { type: Boolean, default: false }
})
const emit = defineEmits(['create', 'edit', 'view-deployment', 'retry'])
const { t } = useAppI18n()

const columns = computed(() => [
  { key: 'topology', label: t('control.table.topology'), primary: true, value: row => row.name || `#${row.id}` },
  { key: 'deployment', label: t('control.table.deployment'), secondary: true, nowrap: true },
  { key: 'scope', label: t('control.table.scope'), value: row => row.service_scope || '-' },
  { key: 'revision', label: t('control.table.activeRevision'), value: row => row.active_revision_id || '-' },
  { key: 'actions', label: t('control.table.actions'), hideable: false }
])
</script>

<style scoped src="./deploymentPanel.css"></style>
