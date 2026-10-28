<template>
  <div class="groups-panel" data-testid="node-groups-panel">
    <UiDataTable
      :columns="columns"
      :rows="groups"
      :label="t('admin.nodes.groups.table.label')"
      :row-label="group => group.name"
      storage-key="admin.nodeGroups"
      :loading="loading"
      :error="error"
      :error-title="t('admin.nodes.groups.loadFailed')"
      :empty-icon="Boxes"
      :empty-title="t('admin.nodes.groups.empty')"
      :empty-description="t('admin.nodes.groups.emptyDescription')"
      :row-actions="rowActions"
      @retry="emit('reload')"
    >
      <template #toolbar>
        <p class="groups-panel__summary">{{ t('admin.nodes.groups.summary') }}</p>
      </template>
      <template #toolbar-end>
        <UiButton :icon="Plus" data-testid="add-node-group" @click="emit('create')">{{ t('admin.nodes.groups.add') }}</UiButton>
      </template>
      <template #empty-actions>
        <UiButton variant="primary" :icon="Terminal" data-testid="groups-empty-join" @click="emit('join', null)">{{ t('admin.nodes.join.title') }}</UiButton>
      </template>
      <template #cell-name="{ row, card }">
        <span class="group-name">
          <span class="group-name__text">{{ row.name }}</span>
          <UiBadge v-if="!card" tone="neutral" :dot="false" :label="t(`admin.nodes.groups.kinds.${groupKind(row)}`)" />
        </span>
      </template>
      <template #cell-nodes="{ row }">
        <span class="group-nodes" data-testid="group-nodes">
          <span>{{ t('admin.nodes.groups.nodesLive', { count: row.nodes?.live || 0 }) }}</span>
          <UiBadge v-if="row.nodes?.pending" tone="warning" :dot="false" :label="t('admin.nodes.groups.nodesPending', { count: row.nodes.pending })" />
        </span>
      </template>
      <template #cell-requireApproval="{ row }">
        <UiSwitch
          :model-value="Boolean(row.require_approval)"
          :disabled="busy.includes(row.id)"
          :aria-label="t('admin.nodes.groups.toggleApproval', { name: row.name })"
          :data-testid="`group-approval-${row.id}`"
          @update:model-value="value => emit('toggle-approval', row, value)"
        />
      </template>
      <template #cell-token="{ row }">
        <UiBadge v-if="row.token_expired" tone="danger" :dot="false" :label="t('admin.nodes.groups.tokenExpired')" />
        <span v-else-if="row.token_expires_at" :title="format.dateTime(row.token_expires_at)">{{ t('admin.nodes.groups.tokenUntil', { date: format.date(row.token_expires_at) }) }}</span>
        <span v-else>{{ t('admin.nodes.groups.tokenNever') }}</span>
      </template>
    </UiDataTable>
  </div>
</template>

<script setup>
// 节点组: the groups with how many machines they hold (and how many wait for
// approval), the approval switch, when the join token ends, and the row
// actions 接入节点, 编辑 and 删除. The page owns the data and the dialogs.
import { computed } from 'vue'
import { Boxes, Pencil, Plus, Terminal, Trash2 } from '@lucide/vue'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiSwitch from '@/ui/UiSwitch.vue'
import { useFormat } from '@/ui/composables/useFormat'
import { useAppI18n } from '@/composables/useAppI18n'
import { groupKind } from './nodeGroupData'

defineProps({
  groups: { type: Array, default: () => [] },
  loading: { type: Boolean, default: false },
  error: { type: [Object, String], default: null },
  // Ids of groups with a change in flight.
  busy: { type: Array, default: () => [] }
})
const emit = defineEmits(['join', 'edit', 'create', 'delete', 'reload', 'toggle-approval'])
const { t } = useAppI18n()
const format = useFormat()

const columns = computed(() => [
  { key: 'name', label: t('admin.nodes.groups.table.name'), primary: true, hideable: false },
  { key: 'nodes', label: t('admin.nodes.groups.table.nodes'), nowrap: true, value: group => group.nodes?.live || 0 },
  { key: 'requireApproval', label: t('admin.nodes.groups.table.requireApproval'), nowrap: true, value: group => (group.require_approval ? 1 : 0) },
  { key: 'token', label: t('admin.nodes.groups.table.token'), nowrap: true, value: group => group.token_expires_at || '' },
  { key: 'rate', label: t('admin.nodes.groups.table.rate'), numeric: true, align: 'end', nowrap: true, hidden: true, value: group => group.rate },
  { key: 'sort', label: t('admin.nodes.groups.table.sort'), numeric: true, align: 'end', nowrap: true, hidden: true, value: group => group.sort }
])

const rowActions = group => [
  { key: 'join', label: t('admin.nodes.join.title'), icon: Terminal, onSelect: () => emit('join', group) },
  { key: 'edit', label: t('admin.nodes.actions.edit'), icon: Pencil, onSelect: () => emit('edit', group) },
  { key: 'delete', label: t('admin.nodes.actions.delete'), icon: Trash2, danger: true, separatorBefore: true, onSelect: () => emit('delete', group) }
]
</script>

<style scoped>
.groups-panel__summary {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.group-name {
  display: inline-flex;
  flex-wrap: wrap;
  gap: var(--space-1) var(--space-2);
  align-items: center;
  min-width: 0;
}

.group-name__text {
  font-weight: var(--weight-medium);
}

.group-nodes {
  display: inline-flex;
  flex-wrap: wrap;
  gap: var(--space-1) var(--space-2);
  align-items: center;
}
</style>
