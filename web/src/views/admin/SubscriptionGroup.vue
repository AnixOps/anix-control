<template>
  <div class="detail-page">
    <UiPageHeader :title="group?.name || t('adminSubscriptionGroups.detail.fallbackTitle')" :description="group?.description || ''">
      <template #back>
        <UiButton variant="tertiary" size="sm" :icon="ChevronLeft" as="router-link" to="/admin/subscriptions" data-test="back-to-groups">{{ t('adminSubscriptionGroups.title') }}</UiButton>
      </template>
      <template v-if="group" #meta>
        <UiBadge :tone="group.enable === 1 ? 'success' : 'neutral'" :label="group.enable === 1 ? t('adminSubscriptionGroups.status.enabled') : t('adminSubscriptionGroups.status.disabled')" />
      </template>
      <template v-if="group" #actions>
        <UiMenu :label="t('adminSubscriptionGroups.detail.more')" :items="moreActions" size="md" />
        <UiButton :icon="Pencil" data-test="edit-group" @click="groupDialogOpen = true">{{ t('adminSubscriptionGroups.actions.edit') }}</UiButton>
        <UiButton variant="primary" :icon="Link2" data-test="open-links" @click="linksOpen = true">{{ t('adminSubscriptionGroups.actions.links') }}</UiButton>
      </template>
    </UiPageHeader>

    <UiSkeleton v-if="showSkeleton" variant="card" :label="t('adminSettings.loading')" />
    <UiErrorState v-else-if="loadError" :title="t('adminSubscriptionGroups.messages.loadFailed')" :error="loadError" @retry="load" />
    <UiEmptyState
      v-else-if="!loading && !group"
      heading-tag="h2"
      :icon="Layers"
      :title="t('adminSubscriptionGroups.detail.notFound')"
      :description="t('adminSubscriptionGroups.detail.notFoundDescription')"
    >
      <template #actions>
        <UiButton variant="primary" as="router-link" to="/admin/subscriptions">{{ t('adminSubscriptionGroups.detail.backToList') }}</UiButton>
      </template>
    </UiEmptyState>
    <UiTabs
      v-else-if="group"
      variant="segmented"
      :model-value="section"
      :items="tabs"
      :aria-label="t('adminSubscriptionGroups.detail.sections')"
      @update:model-value="goToSection"
    >
      <template #overview>
        <div class="detail-sections">
          <UiGroupedList :title="t('adminSubscriptionGroups.overview.title')" heading-tag="h2">
            <UiGroupedListRow :label="t('adminSubscriptionGroups.table.status')">
              <UiBadge :tone="group.enable === 1 ? 'success' : 'neutral'" :label="group.enable === 1 ? t('adminSubscriptionGroups.status.enabled') : t('adminSubscriptionGroups.status.disabled')" />
            </UiGroupedListRow>
            <UiGroupedListRow :label="t('adminSubscriptionGroups.table.priority')" :description="t('adminSubscriptionGroups.groupDialog.priorityHelp')" :value="String(group.priority ?? 0)" />
            <UiGroupedListRow :label="t('adminSubscriptionGroups.table.templates')" :value="format.number(stat.template_count ?? group.template_count ?? 0)" @click="goToSection('templates')" />
            <UiGroupedListRow :label="t('adminSubscriptionGroups.table.protocols')" :value="format.number(stat.protocol_count || 0)" @click="goToSection('protocols')" />
            <UiGroupedListRow :label="t('adminSubscriptionGroups.table.onlineNodes')" :value="format.number(stat.online_nodes || 0)" />
            <UiGroupedListRow :label="t('adminSubscriptionGroups.table.traffic')" :value="format.bytes(stat.total_traffic || 0)" />
          </UiGroupedList>
          <UiGroupedList :title="t('adminSubscriptionGroups.overview.share')" heading-tag="h2" :footer="t('adminSubscriptionGroups.overview.shareHelp')">
            <UiGroupedListRow :label="t('adminSubscriptionGroups.actions.links')" @click="linksOpen = true" />
            <UiGroupedListRow :label="t('adminSubscriptionGroups.actions.copyCombined')" @click="copyGroupCombined(group)" />
            <UiGroupedListRow :label="t('adminSubscriptionGroups.output.title')" @click="goToSection('output')" />
          </UiGroupedList>
        </div>
      </template>
      <template #templates>
        <GroupTemplates :group="group" @changed="loadStats" />
      </template>
      <template #protocols>
        <GroupProtocols :group="group" @changed="loadStats" />
      </template>
      <template #members>
        <GroupMembers :group="group" :stat="stat" />
      </template>
      <template #output>
        <GroupOutput :group="group" />
      </template>
    </UiTabs>

    <SubscriptionGroupDialog v-model:open="groupDialogOpen" :group="group" @saved="load" />
    <SubscriptionLinksDialog v-model:open="linksOpen" :group="group" />
  </div>
</template>

<script setup>
// 订阅分组 → one group (plan §7.2 detail template, §8.2): back link, name,
// state and actions, then sections in the URL (/admin/subscriptions/:id/
// :section): 概览, 模板, 节点协议, 成员, 订阅输出. The group comes from the
// group list and its numbers from the stats endpoint; 成员 pages the users
// granted the group directly (GET /api/v4/admin/subscription-groups/:id/
// members), see subscriptions/GroupMembers.vue.
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ChevronLeft, Copy, Layers, Link2, Pencil, Trash2 } from '@lucide/vue'
import adminApi, { getSubscriptionStats } from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiEmptyState from '@/ui/UiEmptyState.vue'
import UiErrorState from '@/ui/UiErrorState.vue'
import UiGroupedList from '@/ui/UiGroupedList.vue'
import UiGroupedListRow from '@/ui/UiGroupedListRow.vue'
import UiMenu from '@/ui/UiMenu.vue'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import UiTabs from '@/ui/UiTabs.vue'
import { useDelayedLoading } from '@/ui/composables/useDelayedLoading'
import { useFormat } from '@/ui/composables/useFormat'
import GroupMembers from './subscriptions/GroupMembers.vue'
import GroupOutput from './subscriptions/GroupOutput.vue'
import GroupProtocols from './subscriptions/GroupProtocols.vue'
import GroupTemplates from './subscriptions/GroupTemplates.vue'
import SubscriptionGroupDialog from './subscriptions/SubscriptionGroupDialog.vue'
import SubscriptionLinksDialog from './subscriptions/SubscriptionLinksDialog.vue'
import { readSubscriptionList, subscriptionErrorText } from './subscriptions/subscriptionShared'
import { useGroupActions } from './subscriptions/useGroupActions'

const SECTIONS = ['overview', 'templates', 'protocols', 'members', 'output']

const { t } = useAppI18n()
const format = useFormat()
const route = useRoute()
const router = useRouter()
const { copyGroupCombined, deleteGroup } = useGroupActions()

const group = ref(null)
const stat = ref({})
const loading = ref(false)
const loadError = ref(null)
const groupDialogOpen = ref(false)
const linksOpen = ref(false)
const showSkeleton = useDelayedLoading(loading)

const groupId = computed(() => Number(route.params.id))
const section = computed(() => (SECTIONS.includes(route.params.section) ? route.params.section : 'overview'))
const tabs = computed(() => SECTIONS.map(value => ({ value, label: t(`adminSubscriptionGroups.detail.tabs.${value}`) })))
const moreActions = computed(() => [
  { key: 'combined', label: t('adminSubscriptionGroups.actions.copyCombined'), icon: Copy, onSelect: () => copyGroupCombined(group.value) },
  { key: 'delete', label: t('adminSubscriptionGroups.actions.delete'), icon: Trash2, danger: true, separatorBefore: true, onSelect: removeGroup }
])

function goToSection(value) {
  const path = value === 'overview' ? `/admin/subscriptions/${groupId.value}` : `/admin/subscriptions/${groupId.value}/${value}`
  if (route.path !== path) router.replace(path)
}

async function loadStats() {
  try {
    const res = await getSubscriptionStats()
    const list = res && typeof res === 'object' ? readSubscriptionList(res, t('adminSubscriptionGroups.messages.loadFailed')) : []
    stat.value = list.find(item => item.group_id === groupId.value) || {}
  } catch (error) {
    stat.value = {}
    console.error('Failed to load subscription stats:', error)
  }
}

async function load() {
  loading.value = true
  loadError.value = null
  try {
    const res = await adminApi.getSubscriptionGroups()
    const groups = readSubscriptionList(res, t('adminSubscriptionGroups.messages.loadFailed'))
    group.value = groups.find(item => item.id === groupId.value) || null
    if (group.value) await loadStats()
  } catch (error) {
    group.value = null
    loadError.value = subscriptionErrorText(error) || t('adminSubscriptionGroups.messages.loadFailed')
  } finally {
    loading.value = false
  }
}

async function removeGroup() {
  if (await deleteGroup(group.value)) router.push('/admin/subscriptions')
}

// An unknown section in the URL goes to 概览.
watch(() => route.params.section, value => {
  if (value && !SECTIONS.includes(value) && Number.isFinite(groupId.value)) router.replace(`/admin/subscriptions/${groupId.value}`)
}, { immediate: true })
watch(groupId, (next, previous) => { if (next !== previous) load() })
onMounted(load)
</script>

<style scoped>
.detail-page {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
  min-width: 0;
}

.detail-page :deep(.ui-tabs__list) {
  max-width: 100%;
}

.detail-page :deep(.ui-tabs__panel) {
  padding-top: var(--space-6);
}

.detail-sections {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
}
</style>
