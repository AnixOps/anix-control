<template>
  <div class="group-members">
    <UiGroupedList :title="t('adminSubscriptionGroups.members.title')" heading-tag="h2" :footer="t('adminSubscriptionGroups.members.help')">
      <UiGroupedListRow :label="t('adminSubscriptionGroups.members.users')" :value="format.number(stat.user_count || 0)" />
      <UiGroupedListRow :label="t('adminSubscriptionGroups.members.enabledUsers')" :value="format.number(stat.enabled_users || 0)" />
      <UiGroupedListRow :label="t('adminSubscriptionGroups.members.plans')" :value="format.number(stat.plan_count || 0)" />
    </UiGroupedList>

    <UiSection :title="t('adminSubscriptionGroups.members.listTitle')" :description="t('adminSubscriptionGroups.members.listDescription')">
      <UiDataTable
        :columns="columns"
        :rows="members"
        row-key="user_id"
        :label="t('adminSubscriptionGroups.members.table.label')"
        :row-label="member => member.email"
        storage-key="admin.subscriptionGroupMembers"
        manual-pagination
        :page="page"
        :page-size="pageSize"
        :total="total"
        :loading="loading"
        :error="loadError"
        :error-title="t('adminSubscriptionGroups.members.loadFailed')"
        :filtered="Boolean(query || status)"
        :empty-icon="Users"
        :empty-title="t('adminSubscriptionGroups.members.empty.title')"
        :empty-description="t('adminSubscriptionGroups.members.empty.description')"
        :row-actions="memberActions"
        :card-fields="4"
        state-heading-tag="h3"
        data-testid="group-members-table"
        @update:page="goToPage"
        @retry="load"
        @clear-filters="clearFilters"
      >
        <template #toolbar>
          <UiSearchField
            v-model="query"
            class="list-page__search"
            :label="t('adminSubscriptionGroups.members.search')"
            data-test="member-search"
            @update:model-value="scheduleSearch"
            @submit="searchNow"
          />
          <UiFilterChips v-model="status" :label="t('adminSubscriptionGroups.members.filterLabel')" :options="statusChips" data-testid="member-status-filter" />
        </template>
        <template #cell-email="{ row }">
          <span class="group-members__email">
            <span class="group-members__email-text">{{ row.email }}</span>
            <UiBadge v-if="row.banned === 1" tone="danger" :dot="false" :label="t('adminSubscriptionGroups.members.banned')" />
          </span>
        </template>
        <template #cell-active="{ row }">
          <UiBadge :tone="row.active ? 'success' : 'warning'" :label="row.active ? t('adminSubscriptionGroups.members.status.active') : t('adminSubscriptionGroups.members.status.expired')" />
        </template>
      </UiDataTable>
    </UiSection>

    <UiGroupedList :title="t('adminSubscriptionGroups.members.manage')" heading-tag="h2">
      <UiGroupedListRow :label="isCommercial ? t('adminSubscriptionGroups.members.openPlansCommercial') : t('adminSubscriptionGroups.members.openPlans')" :description="t('adminSubscriptionGroups.members.openPlansHelp')" href="/admin/plans" @click.prevent="router.push('/admin/plans')" />
      <UiGroupedListRow :label="t('adminSubscriptionGroups.members.openUsers')" :description="t('adminSubscriptionGroups.members.openUsersHelp')" href="/admin/users" @click.prevent="router.push('/admin/users')" />
    </UiGroupedList>
  </div>
</template>

<script setup>
// 成员 of one subscription group: the counts, the users granted the group
// directly as a paged, searchable table (GET /api/v4/admin/subscription-groups
// /:id/members: e-mail, membership end, quota override, renewal price, no
// credential), and where the other ways in (plans, primary group) are
// managed. Users the group reaches through a plan or their primary group are
// not members of this list, as the counts above do not count them. Search and
// status go to the server; the table keeps its own state (the group page's
// URL names the section only).
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { UserSearch, Users } from '@lucide/vue'
import { getSubscriptionGroupMembers } from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import { useEdition } from '@/composables/useEdition'
import UiBadge from '@/ui/UiBadge.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiFilterChips from '@/ui/UiFilterChips.vue'
import UiGroupedList from '@/ui/UiGroupedList.vue'
import UiGroupedListRow from '@/ui/UiGroupedListRow.vue'
import UiSearchField from '@/ui/UiSearchField.vue'
import UiSection from '@/ui/UiSection.vue'
import { useFormat } from '@/ui/composables/useFormat'
import { adminV4ErrorMessage } from '@/utils/adminV4'

const props = defineProps({
  group: { type: Object, required: true },
  stat: { type: Object, default: () => ({}) }
})

const { t } = useAppI18n()
const format = useFormat()
const router = useRouter()
const { isCommercial } = useEdition()

const pageSize = 20
const members = ref([])
const total = ref(0)
const page = ref(1)
const query = ref('')
const status = ref('')
const loading = ref(false)
const loadError = ref(null)
let sequence = 0

const columns = computed(() => [
  { key: 'email', label: t('adminSubscriptionGroups.members.table.email'), primary: true, hideable: false },
  { key: 'active', label: t('adminSubscriptionGroups.members.table.membership'), secondary: true },
  { key: 'plan_id', label: isCommercial.value ? t('adminSubscriptionGroups.members.table.plan') : t('adminSubscriptionGroups.members.table.template'), nowrap: true, breakpoint: 'lg', format: value => (value ? `#${value}` : '—') },
  { key: 'expire_at', label: t('adminSubscriptionGroups.members.table.expireAt'), numeric: true, nowrap: true, format: value => (value ? format.date(Number(value)) : t('adminSubscriptionGroups.members.never')) },
  { key: 'transfer_enable', label: t('adminSubscriptionGroups.members.table.quota'), numeric: true, nowrap: true, breakpoint: 'lg', format: value => (value ? format.bytes(value) : '—') },
  { key: 'next_renew_price', label: t('adminSubscriptionGroups.members.table.renewPrice'), numeric: true, nowrap: true, hidden: true, card: false, format: value => (value ? format.money(value) : '—') },
  { key: 'created_at', label: t('adminSubscriptionGroups.members.table.grantedAt'), numeric: true, nowrap: true, breakpoint: 'lg', format: value => format.dateTime(value) }
])
const statusChips = computed(() => [
  { value: 'active', label: t('adminSubscriptionGroups.members.status.active') },
  { value: 'expired', label: t('adminSubscriptionGroups.members.status.expired') }
])
const memberActions = member => [
  { key: 'find', label: t('adminSubscriptionGroups.members.findInUsers'), icon: UserSearch, onSelect: () => router.push({ path: '/admin/users', query: { email: member.email } }) }
]

async function load() {
  const mine = ++sequence
  const id = props.group?.id
  if (!id) return
  loading.value = true
  try {
    const data = await getSubscriptionGroupMembers(id, { page: page.value, pageSize, q: query.value.trim(), status: status.value })
    if (mine !== sequence) return
    members.value = Array.isArray(data?.members) ? data.members : []
    total.value = Number(data?.total || 0)
    loadError.value = null
  } catch (error) {
    if (mine !== sequence) return
    console.error('Failed to load the group members:', error)
    members.value = []
    total.value = 0
    // The table's error state shows the message and the request id.
    loadError.value = error?.response ? error : adminV4ErrorMessage(error, t('adminSubscriptionGroups.members.loadFailed'))
  } finally {
    if (mine === sequence) loading.value = false
  }
}

function goToPage(value) {
  page.value = value
  load()
}

let searchTimer = null
function searchNow() {
  clearTimeout(searchTimer)
  page.value = 1
  load()
}
function scheduleSearch() {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(searchNow, 300)
}
function clearFilters() {
  query.value = ''
  // Clearing the chip reloads through the watcher.
  if (status.value) status.value = ''
  else searchNow()
}
watch(status, () => searchNow())
// Another group (the route's id changed under the page): start over.
watch(() => props.group?.id, () => {
  query.value = ''
  status.value = ''
  page.value = 1
  load()
})
onBeforeUnmount(() => clearTimeout(searchTimer))

load()

defineExpose({ load })
</script>

<style scoped>
.group-members {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
  min-width: 0;
}

.group-members__email {
  display: inline-flex;
  flex-wrap: wrap;
  gap: var(--space-1) var(--space-2);
  align-items: center;
  min-width: 0;
}

.group-members__email-text {
  overflow-wrap: anywhere;
}
</style>
