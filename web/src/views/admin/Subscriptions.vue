<template>
  <div class="list-page">
    <UiPageHeader :title="t('adminSubscriptionGroups.title')" :description="t('adminSubscriptionGroups.subtitle')">
      <template #actions>
        <UiButton variant="primary" :icon="Plus" data-test="create-group" @click="openGroupDialog()">{{ t('adminSubscriptionGroups.createGroup') }}</UiButton>
      </template>
    </UiPageHeader>

    <dl v-if="rows.length" class="list-page__summary overview-stats" data-test="subscription-summary">
      <div class="stat-card"><dt>{{ t('adminSubscriptionGroups.summary.groups') }}</dt><dd class="stat-value"><strong>{{ format.number(groups.length) }}</strong></dd></div>
      <div class="stat-card"><dt>{{ t('adminSubscriptionGroups.summary.users') }}</dt><dd class="stat-value"><strong>{{ format.number(totalUsers) }}</strong></dd></div>
      <div class="stat-card"><dt>{{ t('adminSubscriptionGroups.summary.templates') }}</dt><dd class="stat-value"><strong>{{ format.number(totalTemplates) }}</strong></dd></div>
      <div class="stat-card"><dt>{{ t('adminSubscriptionGroups.summary.traffic') }}</dt><dd class="stat-value"><strong>{{ format.bytes(totalTraffic) }}</strong></dd></div>
    </dl>

    <UiDataTable
      :columns="columns"
      :rows="filteredRows"
      :label="t('adminSubscriptionGroups.table.label')"
      :row-label="group => group.name"
      storage-key="admin.subscriptionGroups"
      :page-size="20"
      :loading="loading"
      :error="loadError"
      :error-title="t('adminSubscriptionGroups.messages.loadFailed')"
      :filtered="Boolean(search || statusFilter)"
      :empty-icon="Layers"
      :empty-title="t('adminSubscriptionGroups.empty.title')"
      :empty-description="t('adminSubscriptionGroups.empty.description')"
      activatable
      :row-actions="groupActions"
      @row-activate="openGroup"
      @retry="loadAll"
      @clear-filters="clearFilters"
    >
      <template #toolbar>
        <UiSearchField v-model="search" class="list-page__search" :label="t('adminSubscriptionGroups.search')" />
        <UiFilterChips v-model="statusFilter" :label="t('adminSubscriptionGroups.filters.label')" :options="statusChips" />
      </template>
      <template #cell-name="{ row, card }">
        <span class="group-cell">
          <span class="group-cell__name">{{ row.name }}</span>
          <span v-if="!card && row.description" class="group-cell__desc">{{ row.description }}</span>
        </span>
      </template>
      <template #cell-enable="{ row }">
        <UiBadge :tone="row.enable === 1 ? 'success' : 'neutral'" :label="row.enable === 1 ? t('adminSubscriptionGroups.status.enabled') : t('adminSubscriptionGroups.status.disabled')" />
      </template>
      <template #cell-online_nodes="{ row }">
        <span :class="['node-count', row.online_nodes > 0 ? 'is-online' : 'is-offline']">{{ format.number(row.online_nodes) }}</span>
      </template>
      <template #empty-actions>
        <UiButton variant="primary" :icon="Plus" @click="openGroupDialog()">{{ t('adminSubscriptionGroups.createGroup') }}</UiButton>
      </template>
    </UiDataTable>

    <SubscriptionGroupDialog v-model:open="groupDialogOpen" :group="editingGroup" @saved="loadAll" />
    <SubscriptionLinksDialog v-model:open="linksOpen" :group="linksGroup" />
  </div>
</template>

<script setup>
// 订阅分组 (plan §8.2 "订阅模板与分组", list template §7.1): the groups with
// their usage (users, templates, protocols, online nodes, traffic, plans)
// in one table; a row opens the group page (/admin/subscriptions/:id) with
// its templates, node protocols, members and subscription output. The "…"
// menu keeps the old card actions. Endpoints unchanged:
// GET /admin/subscription/groups, GET /admin/subscription/stats.
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Copy, Layers, Link2, Pencil, Plus, SquareArrowOutUpRight, Trash2 } from '@lucide/vue'
import adminApi, { getSubscriptionStats } from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiFilterChips from '@/ui/UiFilterChips.vue'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import UiSearchField from '@/ui/UiSearchField.vue'
import { useFormat } from '@/ui/composables/useFormat'
import SubscriptionGroupDialog from './subscriptions/SubscriptionGroupDialog.vue'
import SubscriptionLinksDialog from './subscriptions/SubscriptionLinksDialog.vue'
import { readSubscriptionList, subscriptionErrorText } from './subscriptions/subscriptionShared'
import { useGroupActions } from './subscriptions/useGroupActions'

const { t } = useAppI18n()
const format = useFormat()
const router = useRouter()
const { copyGroupCombined, deleteGroup: confirmDeleteGroup } = useGroupActions()

const groups = ref([])
const groupStats = ref([])
const loading = ref(false)
const loadError = ref(null)
const search = ref('')
const statusFilter = ref('')
const groupDialogOpen = ref(false)
const editingGroup = ref(null)
const linksOpen = ref(false)
const linksGroup = ref(null)

const statsById = computed(() => new Map(groupStats.value.map(stat => [stat.group_id, stat])))
const rows = computed(() => groups.value.map(group => {
  const stat = statsById.value.get(group.id) || {}
  return {
    ...group,
    user_count: Number(stat.user_count) || 0,
    enabled_users: Number(stat.enabled_users) || 0,
    template_count: Number(stat.template_count ?? group.template_count) || 0,
    protocol_count: Number(stat.protocol_count) || 0,
    online_nodes: Number(stat.online_nodes) || 0,
    total_traffic: Number(stat.total_traffic) || 0,
    plan_count: Number(stat.plan_count) || 0
  }
}))
const filteredRows = computed(() => {
  const query = search.value.trim().toLowerCase()
  return rows.value.filter(group => {
    if (statusFilter.value === 'enabled' && group.enable !== 1) return false
    if (statusFilter.value === 'disabled' && group.enable === 1) return false
    if (!query) return true
    return String(group.name || '').toLowerCase().includes(query) || String(group.description || '').toLowerCase().includes(query)
  })
})

const totalUsers = computed(() => groupStats.value.reduce((sum, s) => sum + (Number(s.user_count) || 0), 0))
const totalTemplates = computed(() => groupStats.value.reduce((sum, s) => sum + (Number(s.template_count) || 0), 0))
const totalTraffic = computed(() => groupStats.value.reduce((sum, s) => sum + (Number(s.total_traffic) || 0), 0))

const statusChips = computed(() => [
  { value: 'enabled', label: t('adminSubscriptionGroups.status.enabled'), count: rows.value.filter(group => group.enable === 1).length },
  { value: 'disabled', label: t('adminSubscriptionGroups.status.disabled'), count: rows.value.filter(group => group.enable !== 1).length }
])

const columns = computed(() => [
  { key: 'name', label: t('adminSubscriptionGroups.table.name'), primary: true, sortable: true, hideable: false },
  { key: 'enable', label: t('adminSubscriptionGroups.table.status'), secondary: true, sortable: true },
  { key: 'user_count', label: t('adminSubscriptionGroups.table.users'), numeric: true, align: 'end', sortable: true, firstDirection: 'desc', format: value => format.number(value) },
  { key: 'template_count', label: t('adminSubscriptionGroups.table.templates'), numeric: true, align: 'end', sortable: true, format: value => format.number(value) },
  { key: 'protocol_count', label: t('adminSubscriptionGroups.table.protocols'), numeric: true, align: 'end', sortable: true, breakpoint: 'md', format: value => format.number(value) },
  { key: 'online_nodes', label: t('adminSubscriptionGroups.table.onlineNodes'), numeric: true, align: 'end', sortable: true, breakpoint: 'md' },
  { key: 'total_traffic', label: t('adminSubscriptionGroups.table.traffic'), numeric: true, align: 'end', sortable: true, firstDirection: 'desc', breakpoint: 'lg', format: value => format.bytes(value) },
  { key: 'plan_count', label: t('adminSubscriptionGroups.table.plans'), numeric: true, align: 'end', sortable: true, hidden: true, format: value => format.number(value) },
  { key: 'priority', label: t('adminSubscriptionGroups.table.priority'), numeric: true, align: 'end', sortable: true, hidden: true },
  { key: 'enabled_users', label: t('adminSubscriptionGroups.table.enabledUsers'), numeric: true, align: 'end', sortable: true, hidden: true, format: value => format.number(value) }
])

const groupActions = group => [
  { key: 'open', label: t('adminSubscriptionGroups.actions.open'), icon: SquareArrowOutUpRight, onSelect: () => openGroup(group) },
  { key: 'links', label: t('adminSubscriptionGroups.actions.links'), icon: Link2, onSelect: () => openLinks(group) },
  { key: 'combined', label: t('adminSubscriptionGroups.actions.copyCombined'), icon: Copy, onSelect: () => copyGroupCombined(group) },
  { key: 'edit', label: t('adminSubscriptionGroups.actions.edit'), icon: Pencil, onSelect: () => openGroupDialog(group) },
  { key: 'delete', label: t('adminSubscriptionGroups.actions.delete'), icon: Trash2, danger: true, separatorBefore: true, onSelect: () => deleteGroup(group) }
]

async function loadGroups() {
  const res = await adminApi.getSubscriptionGroups()
  groups.value = readSubscriptionList(res, t('adminSubscriptionGroups.messages.loadFailed'))
}

async function loadStats() {
  try {
    const res = await getSubscriptionStats()
    groupStats.value = res && typeof res === 'object' ? readSubscriptionList(res, t('adminSubscriptionGroups.messages.loadFailed')) : []
  } catch (error) {
    // The usage columns stay at zero; the groups still list.
    groupStats.value = []
    console.error('Failed to load subscription stats:', error)
  }
}

async function loadAll() {
  loading.value = true
  loadError.value = null
  try {
    await Promise.all([loadGroups(), loadStats()])
  } catch (error) {
    groups.value = []
    loadError.value = subscriptionErrorText(error) || t('adminSubscriptionGroups.messages.loadFailed')
  } finally {
    loading.value = false
  }
}

function openGroup(group) {
  router.push(`/admin/subscriptions/${group.id}`)
}

function openGroupDialog(group = null) {
  editingGroup.value = group
  groupDialogOpen.value = true
}

function openLinks(group) {
  linksGroup.value = group
  linksOpen.value = true
}

async function deleteGroup(group) {
  if (await confirmDeleteGroup(group)) loadAll()
}

function clearFilters() {
  search.value = ''
  statusFilter.value = ''
}

onMounted(loadAll)
</script>

<style scoped>
.group-cell {
  display: flex;
  flex-direction: column;
  gap: var(--space-0-5);
  min-width: 0;
}

.group-cell__name {
  font-weight: var(--weight-medium);
}

.group-cell__desc {
  overflow: hidden;
  color: var(--label-2);
  font-size: var(--type-callout-size);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.overview-stats .stat-card {
  display: flex;
  gap: var(--space-1);
}

.overview-stats dd {
  margin: 0;
}

.node-count.is-online {
  color: var(--success);
}

.node-count.is-offline {
  color: var(--label-2);
}
</style>
