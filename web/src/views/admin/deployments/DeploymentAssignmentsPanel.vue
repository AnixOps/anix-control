<template>
  <section
    class="deployment-panel"
    data-testid="deployment-target-panel"
  >
    <UiDataTable
      :columns="columns"
      :rows="selectedNodeID ? assignments : []"
      :label="t('control.tabs.assignments')"
      :row-label="row => `${pluginName(row.plugin_id)} / ${row.role}`"
      :loading="loading"
      :error="error"
      :error-title="t('control.errors.load')"
      :empty-icon="Boxes"
      :empty-title="selectedNodeID ? t('control.empty.assignments') : t('control.assignments.noNodes')"
      :empty-description="selectedNodeID ? t('control.assignments.emptyHint') : ''"
      :settings="false"
      :card-fields="8"
      @retry="emit('retry')"
    >
      <template #toolbar>
        <UiSelect
          id="assignment-node-filter"
          class="deployment-panel__picker"
          size="md"
          :label="t('control.assignments.node')"
          :model-value="selectedNodeID || undefined"
          :options="nodeOptions"
          :placeholder="t('control.assignments.noNodes')"
          :disabled="nodes.length === 0 || mutationPending"
          @update:model-value="value => emit('select-node', value)"
        />
      </template>
      <template #toolbar-end>
        <UiButton
          :icon="RotateCw"
          :loading="loading && Boolean(selectedNodeID)"
          :disabled="!selectedNodeID || mutationPending"
          @click="emit('refresh')"
        >
          {{ t('control.actions.refresh') }}
        </UiButton>
        <UiButton
          variant="primary"
          :icon="Plus"
          data-testid="new-assignment"
          :disabled="!selectedNodeID || mutationPending"
          @click="emit('create')"
        >
          {{ t('control.actions.newAssignment') }}
        </UiButton>
      </template>
      <template #cell-plugin="{ row }">
        <span class="deployment-panel__name">
          <strong>{{ pluginName(row.plugin_id) }}</strong>
          <code>{{ row.plugin_id }}</code>
        </span>
      </template>
      <template #cell-scope="{ row }"><code>{{ row.service_scope }}</code></template>
      <template #cell-role="{ row }"><code>{{ row.role }}</code></template>
      <template #cell-version="{ row }"><code>{{ row.desired_version || '-' }}</code></template>
      <template #cell-state="{ row }">
        <UiBadge
          :status="row.enabled ? 'online' : 'disabled'"
          :label="row.enabled ? t('control.states.enabled') : t('control.states.disabled')"
        />
      </template>
      <template #cell-actions="{ row }">
        <span class="deployment-panel__actions">
          <UiButton size="sm" :data-testid="`edit-assignment-${row.id}`" :disabled="isBusy(row)" @click="emit('edit', row)">
            {{ t('common.actions.edit') }}
          </UiButton>
          <UiButton
            size="sm"
            :variant="row.enabled ? 'secondary' : 'primary'"
            :data-testid="`toggle-assignment-${row.id}`"
            :disabled="isBusy(row)"
            @click="emit('toggle', row)"
          >
            {{ row.enabled ? t('control.actions.disable') : t('control.actions.enable') }}
          </UiButton>
          <UiButton
            size="sm"
            variant="danger-soft"
            :data-testid="`delete-assignment-${row.id}`"
            :disabled="isBusy(row)"
            @click="emit('remove', row)"
          >
            {{ t('common.actions.delete') }}
          </UiButton>
        </span>
      </template>
    </UiDataTable>
  </section>
</template>

<script setup>
// 部署编排 · 节点角色 (UI U8): the plugin roles assigned to one node. The
// node picker, refresh and 新建角色 sit in the table toolbar; each row keeps
// 编辑 / 启用·禁用 / 删除 as visible buttons (on phone cards too).
import { computed } from 'vue'
import { Boxes, Plus, RotateCw } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiSelect from '@/ui/UiSelect.vue'

const props = defineProps({
  nodes: { type: Array, default: () => [] },
  selectedNodeID: { type: Number, default: 0 },
  assignments: { type: Array, default: () => [] },
  loading: { type: Boolean, default: false },
  error: { type: [String, Object, null], default: null },
  mutationPending: { type: Boolean, default: false },
  pluginName: { type: Function, required: true },
  isBusy: { type: Function, required: true }
})
const emit = defineEmits(['select-node', 'refresh', 'create', 'edit', 'toggle', 'remove', 'retry'])
const { t } = useAppI18n()

const nodeOptions = computed(() => props.nodes.map(node => ({
  value: Number(node.id),
  label: `${node.name || node.host || `#${node.id}`} (#${node.id})`
})))

const columns = computed(() => [
  { key: 'plugin', label: t('control.table.plugin'), primary: true, value: row => props.pluginName(row.plugin_id) },
  { key: 'state', label: t('control.table.state'), secondary: true, nowrap: true },
  { key: 'scope', label: t('control.table.scope'), value: row => row.service_scope },
  { key: 'role', label: t('control.table.role'), value: row => row.role },
  { key: 'version', label: t('control.table.version'), value: row => row.desired_version || '-' },
  { key: 'config', label: t('control.table.configRevision'), numeric: true, value: row => row.desired_config_revision ?? 0 },
  { key: 'rollout', label: t('control.table.rolloutGroup'), value: row => row.rollout_group || '-' },
  { key: 'actions', label: t('control.table.actions'), hideable: false }
])
</script>

<style scoped src="./deploymentPanel.css"></style>
