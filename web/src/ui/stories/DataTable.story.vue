<script setup>
import { computed, ref } from 'vue'
import { Ban, Pencil, Plus, Trash2, Users } from '@lucide/vue'
import UiDataTable from '../UiDataTable.vue'
import UiPageHeader from '../UiPageHeader.vue'
import UiSearchField from '../UiSearchField.vue'
import UiFilterChips from '../UiFilterChips.vue'
import UiSheet from '../UiSheet.vue'
import UiButton from '../UiButton.vue'
import UiBadge from '../UiBadge.vue'
import UiUsageBar from '../UiUsageBar.vue'
import UiGroupedList from '../UiGroupedList.vue'
import UiGroupedListRow from '../UiGroupedListRow.vue'
import { formatBytes } from '../composables/useFormat'

const GIB = 1024 ** 3
const NAMES = ['lin.xiao', 'wang.fang', 'chen.jie', 'zhao.lei', 'sun.li', 'zhou.min', 'wu.hao', 'zheng.yu', 'feng.yi', 'he.ming', 'luo.qi', 'gao.yan']
const ROWS = NAMES.map((name, index) => ({
  id: index + 1,
  email: `${name}@example.com`,
  status: index % 5 === 3 ? 'banned' : (index % 4 === 2 ? 'expired' : 'active'),
  plan: ['标准', '高级', '基础'][index % 3],
  used: ((index * 37) % 200) * GIB,
  total: 200 * GIB,
  expires: `2026-${String((index % 12) + 1).padStart(2, '0')}-28`
}))
const STATUS = { active: { status: 'online', label: '正常' }, banned: { status: 'error', label: '已封禁' }, expired: { status: 'pending', label: '已到期' } }

const columns = [
  { key: 'email', label: '邮箱', sortable: true, primary: true },
  { key: 'status', label: '状态', secondary: true, sortable: true },
  { key: 'plan', label: '订阅模板', breakpoint: 'md' },
  { key: 'used', label: '已用 / 总流量', sortable: true, firstDirection: 'desc' },
  { key: 'expires', label: '到期', sortable: true, numeric: true }
]

const search = ref('')
const filter = ref('')
const selected = ref([])
const open = ref(false)
const current = ref(null)
const rows = computed(() => ROWS.filter(row => (!filter.value || row.status === filter.value) && row.email.includes(search.value.trim())))
const chips = [{ value: 'banned', label: '已封禁' }, { value: 'expired', label: '已到期' }]

function show(row) {
  current.value = row
  open.value = true
}

function actions(row) {
  return [
    { key: 'edit', label: '编辑', icon: Pencil, onSelect: () => show(row) },
    { key: 'ban', label: row.status === 'banned' ? '解封' : '封禁', icon: Ban, onSelect: () => {} },
    { key: 'delete', label: '删除', icon: Trash2, danger: true, separatorBefore: true, onSelect: () => {} }
  ]
}

function clearFilters() {
  search.value = ''
  filter.value = ''
}
</script>

<template>
  <Story title="DataTable" group="data" :layout="{ type: 'single', iframe: true }">
    <Variant title="List page template">
      <div class="story" style="max-width: 1100px">
        <UiPageHeader title="用户" description="管理账户、订阅和流量。">
          <template #actions>
            <UiButton variant="primary" :icon="Plus">新建用户</UiButton>
          </template>
        </UiPageHeader>
        <UiDataTable
          v-model:selected="selected"
          :columns="columns"
          :rows="rows"
          label="用户"
          storage-key="story.users"
          :page-size="8"
          :filtered="Boolean(search || filter)"
          selectable
          activatable
          :row-actions="actions"
          :row-label="row => row.email"
          @row-activate="show"
          @clear-filters="clearFilters"
        >
          <template #toolbar>
            <UiSearchField v-model="search" label="搜索邮箱" style="max-width: 280px" />
            <UiFilterChips v-model="filter" label="按状态筛选" :options="chips" />
          </template>
          <template #cell-status="{ row }">
            <UiBadge :status="STATUS[row.status].status" :label="STATUS[row.status].label" />
          </template>
          <template #cell-used="{ row }">
            <UiUsageBar :value="row.used" :max="row.total" :text="`${formatBytes(row.used)} / ${formatBytes(row.total)}`" />
          </template>
          <template #bulk-actions>
            <UiButton size="sm" :icon="Ban">封禁</UiButton>
            <UiButton size="sm">解封</UiButton>
          </template>
        </UiDataTable>
        <UiSheet v-model:open="open" :title="current?.email || ''" description="用户详情" grouped>
          <UiGroupedList v-if="current" title="订阅">
            <UiGroupedListRow label="订阅模板" :value="current.plan" />
            <UiGroupedListRow label="已用流量" :value="`${formatBytes(current.used)} / ${formatBytes(current.total)}`" />
            <UiGroupedListRow label="到期" :value="current.expires" />
          </UiGroupedList>
        </UiSheet>
      </div>
    </Variant>

    <Variant title="Loading">
      <div class="story" style="max-width: 1100px">
        <UiDataTable :columns="columns" :rows="[]" label="用户" loading />
      </div>
    </Variant>

    <Variant title="Empty">
      <div class="story" style="max-width: 1100px">
        <UiDataTable :columns="columns" :rows="[]" label="用户" :empty-icon="Users" empty-title="还没有用户" empty-description="添加第一个用户后，就能给他分配订阅。">
          <template #empty-actions>
            <UiButton variant="primary" :icon="Plus">新建用户</UiButton>
          </template>
        </UiDataTable>
      </div>
    </Variant>

    <Variant title="Error">
      <div class="story" style="max-width: 1100px">
        <UiDataTable :columns="columns" :rows="[]" label="用户" error="服务器没有响应（HTTP 502）" error-title="用户列表没有加载出来" />
      </div>
    </Variant>
  </Story>
</template>

<docs lang="md">
# DataTable

The list of a list page (plan §7.1): column definitions drive the header,
cells, phone cards, sorting and the column settings. Client-side sorting and
pagination by default; `manualSort` / `manualPagination` + `total` for the
server. Selection shows a floating bulk bar; `rowActions` adds a "…" menu;
`activatable` rows open a Sheet with click or Enter (↑/↓ move). States:
error with 重试 / 复制错误详情, skeleton after 300 ms, empty, and "no
results" with 清除筛选 when `filtered`. Hidden columns and density are
remembered per `storageKey`. Below 640 px each row is a card.
</docs>
