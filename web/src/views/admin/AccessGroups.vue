<template>
  <div class="list-page">
    <UiPageHeader :title="t('pageTitles.admin.accessGroups')" :description="t('accessGroups.subtitle')">
      <template #actions>
        <UiButton :icon="RotateCw" :loading="loading" data-test="access-refresh" @click="refresh()">{{ t('accessGroups.actions.refresh') }}</UiButton>
        <UiButton variant="primary" :icon="Plus" :disabled="scopes.length === 0" data-test="access-new-group" @click="openGroupEditor()">{{ t('accessGroups.actions.newGroup') }}</UiButton>
      </template>
    </UiPageHeader>

    <UiDataTable
      :columns="columns"
      :rows="groups"
      :label="t('accessGroups.groups.title')"
      :row-label="group => group.name"
      storage-key="admin.accessGroups"
      :page-size="20"
      :loading="loading"
      :error="listError"
      :error-title="t('accessGroups.errors.loadGroups')"
      :filtered="Boolean(scopeFilter)"
      :empty-icon="ShieldCheck"
      :empty-title="scopeFilter ? t('accessGroups.groups.empty') : t('accessGroups.groups.emptyAll')"
      :empty-description="t('accessGroups.groups.emptyDescription')"
      activatable
      :row-actions="groupActions"
      class="access-groups-table"
      @row-activate="group => openDetail(group.id)"
      @retry="refresh"
      @clear-filters="setScope('')"
    >
      <template #toolbar>
        <UiSelect
          class="access-scope"
          size="md"
          :model-value="scopeFilter || ALL"
          :aria-label="t('accessGroups.filters.label')"
          :options="scopeOptions"
          :disabled="loading"
          data-test="access-scope-filter"
          @update:model-value="value => setScope(value === ALL ? '' : value)"
        />
        <span v-if="activeScope?.description" class="access-scope__note">{{ activeScope.description }}</span>
      </template>
      <template #cell-name="{ row }">
        <span class="access-group-name">
          <span class="access-group-name__title">{{ row.name }}</span>
          <span v-if="row.description" class="access-group-name__desc">{{ row.description }}</span>
        </span>
      </template>
      <template #cell-scope_id="{ value }">
        <code>{{ value }}</code>
      </template>
      <template #cell-enabled="{ row }">
        <UiBadge :tone="row.enabled ? 'success' : 'neutral'" :label="row.enabled ? t('accessGroups.states.enabled') : t('accessGroups.states.disabled')" />
      </template>
    </UiDataTable>

    <UiCard as="section" :title="t('accessGroups.resolver.title')" :description="t('accessGroups.resolver.description')" class="access-resolver">
      <form class="access-resolver__form" @submit.prevent="resolveAccess">
        <UiTextField id="access-resolve-user" v-model.trim="resolver.userID" size="md" type="number" min="1" inputmode="numeric" required :label="t('accessGroups.resolver.userID')" />
        <UiTextField id="access-resolve-plan" v-model.trim="resolver.planID" size="md" type="number" min="1" inputmode="numeric" :label="t('accessGroups.resolver.planID')" />
        <UiSelect id="access-resolve-scope" v-model="resolver.scopeID" size="md" required :label="t('accessGroups.resolver.scope')" :options="resolverScopeOptions" data-test="access-resolve-scope" />
        <div class="access-resolver__submit">
          <UiButton variant="primary" type="submit" :loading="resolving">{{ t('accessGroups.actions.resolve') }}</UiButton>
        </div>
      </form>
      <p v-if="resolverError" class="form-error" role="alert">{{ resolverError }}</p>
      <div v-if="resolvedAccess" class="resolver-result" role="status">
        <strong>{{ t('accessGroups.resolver.result', { count: resolvedAccess.groups?.length || 0 }) }}</strong>
        <div class="resolver-result__chips">
          <UiBadge v-for="item in resolvedAccess.groups || []" :key="item.id" tone="info" :dot="false" :label="`${item.name} #${item.id}`" />
          <span v-if="(resolvedAccess.groups || []).length === 0" class="resolver-result__none">{{ t('accessGroups.resolver.none') }}</span>
        </div>
        <p>{{ t('accessGroups.resolver.policySummary', { grants: resolvedAccess.grants?.length || 0, quotas: resolvedAccess.quotas?.length || 0 }) }}</p>
      </div>
    </UiCard>

    <UiSheet
      :open="detailOpen"
      size="lg"
      grouped
      :title="detail?.group.name || selectedGroup?.name || t('accessGroups.detail.title')"
      :description="detail ? (detail.group.description || t('accessGroups.groups.noDescription')) : ''"
      data-test="access-detail-sheet"
      @update:open="value => { if (!value) closeDetail() }"
    >
      <UiErrorState v-if="detailLoadError" compact heading-tag="h3" :title="t('accessGroups.errors.loadDetail')" :error="detailLoadError" @retry="refreshSelectedDetail" />
      <UiSkeleton v-else-if="!detail && showDetailSkeleton" variant="text" :lines="8" :label="t('accessGroups.detail.loading')" />
      <template v-else-if="detail">
        <p v-if="error" class="form-error error-message" role="alert">{{ error }}</p>

        <UiGroupedList :title="t('accessGroups.detail.group')" :footer="t('accessGroups.groups.directUnion')">
          <UiGroupedListRow :label="t('accessGroups.table.scope')">
            <code>{{ detail.group.scope_id }}</code>
          </UiGroupedListRow>
          <UiGroupedListRow :label="t('accessGroups.editor.enabled')" label-for="access-detail-enabled">
            <UiSwitch id="access-detail-enabled" :model-value="detail.group.enabled" :disabled="mutation" @update:model-value="toggleGroup()" />
          </UiGroupedListRow>
          <UiGroupedListRow :label="t('accessGroups.actions.editGroup')" @click="openGroupEditor(detail.group)" />
        </UiGroupedList>

        <section class="policy-section" aria-labelledby="access-members-title">
          <h3 id="access-members-title" class="policy-section__title">{{ t('accessGroups.members.title') }} <span class="policy-section__count">{{ detail.users.length }}</span></h3>
          <form class="policy-form" @submit.prevent="addMember">
            <UiTextField id="access-member-id" v-model.trim="memberID" size="md" type="number" min="1" inputmode="numeric" required :label="t('accessGroups.members.userID')" />
            <UiButton type="submit" :icon="Plus" :disabled="mutation">{{ t('accessGroups.actions.add') }}</UiButton>
          </form>
          <ul v-if="detail.users.length" class="entity-list">
            <li v-for="user in detail.users" :key="user.id" class="entity-list__item">
              <span class="entity-list__text"><strong>{{ user.email || `#${user.id}` }}</strong><code>#{{ user.id }}</code></span>
              <UiIconButton :icon="X" size="sm" :label="t('accessGroups.actions.removeNamed', { name: user.email || `#${user.id}` })" :disabled="mutation" @click="removeMember(user)" />
            </li>
          </ul>
          <p v-else class="policy-section__empty">{{ t('accessGroups.members.empty') }}</p>
        </section>

        <section class="policy-section" aria-labelledby="access-plans-title">
          <h3 id="access-plans-title" class="policy-section__title">{{ t('accessGroups.plans.title') }} <span class="policy-section__count">{{ detail.plans.length }}</span></h3>
          <form class="policy-form" @submit.prevent="addPlan">
            <UiTextField id="access-plan-id" v-model.trim="planID" size="md" type="number" min="1" inputmode="numeric" required :label="t('accessGroups.plans.planID')" />
            <UiButton type="submit" :icon="Plus" :disabled="mutation">{{ t('accessGroups.actions.add') }}</UiButton>
          </form>
          <ul v-if="detail.plans.length" class="entity-list">
            <li v-for="plan in detail.plans" :key="plan.id" class="entity-list__item">
              <span class="entity-list__text"><strong>{{ plan.name || `#${plan.id}` }}</strong><code>#{{ plan.id }}</code></span>
              <UiIconButton :icon="X" size="sm" :label="t('accessGroups.actions.removeNamed', { name: plan.name || `#${plan.id}` })" :disabled="mutation" @click="removePlan(plan)" />
            </li>
          </ul>
          <p v-else class="policy-section__empty">{{ t('accessGroups.plans.empty') }}</p>
        </section>

        <section class="policy-section" aria-labelledby="access-grants-title">
          <h3 id="access-grants-title" class="policy-section__title">{{ t('accessGroups.grants.title') }} <span class="policy-section__count">{{ detail.resource_grants.length }}</span></h3>
          <form class="policy-form policy-form--wide" @submit.prevent="addGrant">
            <UiTextField id="access-grant-type" v-model.trim="grantEditor.resourceType" size="md" required maxlength="64" placeholder="plugin_api" :label="t('accessGroups.grants.resourceType')" />
            <UiTextField id="access-grant-id" v-model.trim="grantEditor.resourceID" size="md" required maxlength="160" placeholder="machine-telemetry" :label="t('accessGroups.grants.resourceID')" />
            <UiTextField id="access-grant-permissions" v-model.trim="grantEditor.permissions" class="policy-form__full" size="md" required spellcheck="false" :label="t('accessGroups.grants.permissions')" />
            <UiButton type="submit" :icon="Plus" :disabled="mutation">{{ t('accessGroups.actions.addGrant') }}</UiButton>
          </form>
          <UiDataTable
            :columns="grantColumns"
            :rows="detail.resource_grants"
            :label="t('accessGroups.grants.title')"
            :row-label="grant => `${grant.resource_type}/${grant.resource_id}`"
            :empty-title="t('accessGroups.grants.empty')"
            :row-actions="grant => [{ key: 'remove', label: t('common.actions.remove'), icon: Trash2, danger: true, disabled: mutation, onSelect: () => removeGrant(grant) }]"
            :settings="false"
            :sticky-header="false"
            state-heading-tag="h4"
            default-density="compact"
            class="access-grants-table"
          >
            <template #cell-resource_type="{ value }"><code>{{ value }}</code></template>
            <template #cell-resource_id="{ value }"><code>{{ value }}</code></template>
            <template #cell-permissions="{ value }"><code class="json-value">{{ value }}</code></template>
          </UiDataTable>
        </section>

        <section class="policy-section" aria-labelledby="access-quotas-title">
          <h3 id="access-quotas-title" class="policy-section__title">{{ t('accessGroups.quotas.title') }} <span class="policy-section__count">{{ detail.quota_policies.length }}</span></h3>
          <form class="policy-form policy-form--wide" @submit.prevent="saveQuota">
            <UiTextField id="access-quota-key" v-model.trim="quotaEditor.key" class="policy-form__full" size="md" required maxlength="80" placeholder="machine-telemetry.rate" :label="t('accessGroups.quotas.key')" />
            <UiTextarea id="access-quota-policy" v-model="quotaEditor.policy" class="policy-form__full" :rows="2" required spellcheck="false" :label="t('accessGroups.quotas.policy')" />
            <UiButton type="submit" :disabled="mutation">{{ t('accessGroups.actions.saveQuota') }}</UiButton>
          </form>
          <UiDataTable
            :columns="quotaColumns"
            :rows="detail.quota_policies"
            :label="t('accessGroups.quotas.title')"
            :row-label="policy => policy.key"
            :empty-title="t('accessGroups.quotas.empty')"
            :row-actions="policy => [{ key: 'remove', label: t('common.actions.remove'), icon: Trash2, danger: true, disabled: mutation, onSelect: () => removeQuota(policy) }]"
            :settings="false"
            :sticky-header="false"
            state-heading-tag="h4"
            default-density="compact"
          >
            <template #cell-key="{ value }"><code>{{ value }}</code></template>
            <template #cell-policy="{ value }"><code class="json-value">{{ value }}</code></template>
          </UiDataTable>
        </section>

        <UiGroupedList :title="t('accessGroups.detail.danger')">
          <UiGroupedListRow :label="t('accessGroups.confirm.deleteGroupAction')" data-test="access-delete-group" @click="removeGroup()" />
        </UiGroupedList>
      </template>
    </UiSheet>

    <UiDialog
      :open="groupEditor.open"
      :title="groupEditor.mode === 'create' ? t('accessGroups.editor.createTitle') : t('accessGroups.editor.editTitle')"
      :dismissible="!mutation"
      @update:open="value => { if (!value) closeGroupEditor() }"
    >
      <form id="access-group-editor-form" class="form-grid" data-test="access-group-editor" @submit.prevent="saveGroup">
        <UiSelect
          id="access-group-scope"
          v-model="groupEditor.scopeID"
          class="form-grid__full"
          required
          :disabled="groupEditor.mode === 'edit'"
          :label="t('accessGroups.table.scope')"
          :help="groupEditor.mode === 'edit' ? t('accessGroups.editor.scopeFixed') : ''"
          :options="resolverScopeOptions"
        />
        <UiTextField id="access-group-name" v-model.trim="groupEditor.name" class="form-grid__full" required maxlength="120" :label="t('accessGroups.editor.name')" />
        <UiTextarea id="access-group-description" v-model="groupEditor.description" class="form-grid__full" :rows="3" maxlength="4000" :label="t('accessGroups.editor.description')" />
        <div class="form-grid__full">
          <UiSwitch v-model="groupEditor.enabled" :label="t('accessGroups.editor.enabled')" />
        </div>
        <p v-if="groupEditor.error" class="form-error error-message form-grid__full" role="alert">{{ groupEditor.error }}</p>
      </form>
      <template #footer="{ close }">
        <UiButton :disabled="mutation" @click="close">{{ t('common.actions.cancel') }}</UiButton>
        <UiButton variant="primary" type="submit" form="access-group-editor-form" :loading="mutation">{{ t('common.actions.save') }}</UiButton>
      </template>
    </UiDialog>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { Pencil, Plus, Power, RotateCw, ShieldCheck, Trash2, X } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiCard from '@/ui/UiCard.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiDialog from '@/ui/UiDialog.vue'
import UiErrorState from '@/ui/UiErrorState.vue'
import UiGroupedList from '@/ui/UiGroupedList.vue'
import UiGroupedListRow from '@/ui/UiGroupedListRow.vue'
import UiIconButton from '@/ui/UiIconButton.vue'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiSheet from '@/ui/UiSheet.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import UiSwitch from '@/ui/UiSwitch.vue'
import UiTextField from '@/ui/UiTextField.vue'
import UiTextarea from '@/ui/UiTextarea.vue'
import { useConfirm } from '@/ui/composables/useConfirm'
import { useDelayedLoading } from '@/ui/composables/useDelayedLoading'
import { useToast } from '@/ui/composables/useToast'
import {
  addKernelAccessGroupPlan,
  addKernelAccessGroupUser,
  createKernelAccessGroup,
  createKernelResourceGrant,
  deleteKernelAccessGroup,
  deleteKernelQuotaPolicy,
  deleteKernelResourceGrant,
  getKernelAccessGroupDetail,
  getKernelAccessGroups,
  getKernelScopes,
  removeKernelAccessGroupPlan,
  removeKernelAccessGroupUser,
  resolveKernelAccess,
  updateKernelAccessGroup,
  upsertKernelQuotaPolicy,
} from '@/api/kernel'

const { t } = useAppI18n()
const toast = useToast()
const confirm = useConfirm()
const loading = ref(false)
const detailLoading = ref(false)
const mutation = ref(false)
const resolving = ref(false)
// Errors of an action inside the open group (adding a member, a grant…)
// stay in its sheet; a failed list load is the table's error state.
const error = ref('')
const listError = ref(null)
const detailLoadError = ref(null)
const resolverError = ref('')
const detailOpen = ref(false)
const showDetailSkeleton = useDelayedLoading(detailLoading)
const ALL = '__all__'
const scopes = ref([])
const groups = ref([])
const scopeFilter = ref('')
const selectedGroupID = ref(0)
const detail = ref(null)
const memberID = ref('')
const planID = ref('')
const resolvedAccess = ref(null)
const resolver = reactive({ userID: '', planID: '', scopeID: '' })
const groupEditor = reactive({ open: false, mode: 'create', id: 0, scopeID: '', name: '', description: '', enabled: true, error: '' })
const grantEditor = reactive({ resourceType: 'plugin_api', resourceID: '', permissions: '[]' })
const quotaEditor = reactive({ key: '', policy: '{}' })

const activeScope = computed(() => scopes.value.find(scope => scope.id === scopeFilter.value) || null)
const selectedGroup = computed(() => groups.value.find(group => group.id === selectedGroupID.value) || null)
const resolverScopeOptions = computed(() => scopes.value.map(scope => ({ value: scope.id, label: `${scope.name || scope.id} (${scope.id})` })))
const scopeOptions = computed(() => [{ value: ALL, label: t('accessGroups.filters.allScopes') }, ...resolverScopeOptions.value])
const columns = computed(() => [
  { key: 'name', label: t('accessGroups.table.group'), primary: true, sortable: true },
  { key: 'enabled', label: t('accessGroups.table.state'), secondary: true, sortable: true },
  { key: 'scope_id', label: t('accessGroups.table.scope'), sortable: true },
  { key: 'id', label: 'ID', numeric: true, sortable: true, hidden: true }
])
const grantColumns = computed(() => [
  { key: 'resource_type', label: t('accessGroups.grants.resourceType'), primary: true },
  { key: 'resource_id', label: t('accessGroups.grants.resourceID'), secondary: true },
  { key: 'permissions', label: t('accessGroups.grants.permissions') }
])
const quotaColumns = computed(() => [
  { key: 'key', label: t('accessGroups.quotas.key'), primary: true },
  { key: 'policy', label: t('accessGroups.quotas.policy') }
])
const groupActions = group => [
  { key: 'open', label: t('accessGroups.actions.open'), icon: ShieldCheck, onSelect: () => openDetail(group.id) },
  { key: 'edit', label: t('accessGroups.actions.editGroup'), icon: Pencil, onSelect: () => openGroupEditor(group) },
  { key: 'toggle', label: group.enabled ? t('accessGroups.actions.disable') : t('accessGroups.actions.enable'), icon: Power, onSelect: () => toggleGroup(group) },
  { key: 'delete', label: t('common.actions.delete'), icon: Trash2, danger: true, separatorBefore: true, onSelect: () => removeGroup(group) }
]

function setScope(value) {
  scopeFilter.value = value
  loadGroups()
}

function apiErrorMessage(requestError, fallback) {
  return requestError?.response?.data?.error?.message || requestError?.message || fallback
}

function positiveID(value, label) {
  const parsed = Number(value)
  if (!Number.isSafeInteger(parsed) || parsed <= 0) {
    throw new Error(t('accessGroups.errors.invalidID', { label }))
  }
  return parsed
}

function ensureJSON(value, label) {
  try {
    const parsed = JSON.parse(value)
    if (!parsed || typeof parsed !== 'object') {
      throw new Error('not object')
    }
  } catch {
    throw new Error(t('accessGroups.errors.invalidJSON', { label }))
  }
}

async function refresh() {
  loading.value = true
  error.value = ''
  listError.value = null
  try {
    scopes.value = await getKernelScopes()
    if (!resolver.scopeID || !scopes.value.some(scope => scope.id === resolver.scopeID)) {
      resolver.scopeID = scopes.value[0]?.id || ''
    }
    if (scopeFilter.value && !scopes.value.some(scope => scope.id === scopeFilter.value)) {
      scopeFilter.value = ''
    }
    await loadGroups()
  } catch (requestError) {
    listError.value = new Error(apiErrorMessage(requestError, t('accessGroups.errors.load')))
  } finally {
    loading.value = false
  }
}

async function loadGroups() {
  listError.value = null
  try {
    groups.value = await getKernelAccessGroups(scopeFilter.value || undefined)
    // The open group follows the reload; a group that is gone closes.
    if (detailOpen.value && selectedGroupID.value) {
      if (groups.value.some(group => group.id === selectedGroupID.value)) await selectGroup(selectedGroupID.value)
      else closeDetail()
    }
  } catch (requestError) {
    listError.value = new Error(apiErrorMessage(requestError, t('accessGroups.errors.loadGroups')))
  }
}

async function selectGroup(groupID) {
  selectedGroupID.value = Number(groupID)
  detailLoading.value = true
  detailLoadError.value = null
  try {
    detail.value = await getKernelAccessGroupDetail(selectedGroupID.value)
  } catch (requestError) {
    detail.value = null
    detailLoadError.value = new Error(apiErrorMessage(requestError, t('accessGroups.errors.loadDetail')))
  } finally {
    detailLoading.value = false
  }
}

// A row opens its group in a sheet: memberships, grants and quotas.
function openDetail(groupID) {
  if (Number(groupID) !== selectedGroupID.value) detail.value = null
  error.value = ''
  detailOpen.value = true
  return selectGroup(groupID)
}

function closeDetail() {
  detailOpen.value = false
  error.value = ''
}

function openGroupEditor(group) {
  const source = group || { scope_id: scopeFilter.value || scopes.value[0]?.id || '', name: '', description: '', enabled: true }
  groupEditor.open = true
  groupEditor.mode = group ? 'edit' : 'create'
  groupEditor.id = group?.id || 0
  groupEditor.scopeID = source.scope_id
  groupEditor.name = source.name || ''
  groupEditor.description = source.description || ''
  groupEditor.enabled = source.enabled !== false
  groupEditor.error = ''
}

function closeGroupEditor() {
  if (mutation.value) return
  groupEditor.open = false
}

async function saveGroup() {
  if (!groupEditor.scopeID || !groupEditor.name) {
    groupEditor.error = t('accessGroups.errors.groupRequired')
    return
  }
  mutation.value = true
  groupEditor.error = ''
  try {
    if (groupEditor.mode === 'create') {
      const created = await createKernelAccessGroup({ scope_id: groupEditor.scopeID, name: groupEditor.name, description: groupEditor.description, enabled: groupEditor.enabled })
      selectedGroupID.value = created.id
      toast.success(t('accessGroups.messages.groupCreated', { name: created.name }))
    } else {
      await updateKernelAccessGroup(groupEditor.id, { name: groupEditor.name, description: groupEditor.description, enabled: groupEditor.enabled })
      selectedGroupID.value = groupEditor.id
      toast.success(t('accessGroups.messages.groupSaved', { name: groupEditor.name }))
    }
    mutation.value = false
    closeGroupEditor()
    await loadGroups()
  } catch (requestError) {
    groupEditor.error = apiErrorMessage(requestError, t('accessGroups.errors.saveGroup'))
  } finally {
    mutation.value = false
  }
}

async function toggleGroup(target) {
  const group = target || detail.value?.group
  if (!group) return
  mutation.value = true
  error.value = ''
  try {
    await updateKernelAccessGroup(group.id, { name: group.name, description: group.description, enabled: !group.enabled })
    toast.success(!group.enabled ? t('accessGroups.messages.groupEnabled', { name: group.name }) : t('accessGroups.messages.groupDisabled', { name: group.name }))
    await loadGroups()
  } catch (requestError) {
    const message = apiErrorMessage(requestError, t('accessGroups.errors.saveGroup'))
    if (detailOpen.value) error.value = message
    else toast.error(message)
  } finally {
    mutation.value = false
  }
}

async function removeGroup(target) {
  const group = target || detail.value?.group
  if (!group) return
  const { id, name } = group
  const confirmed = await confirm({
    title: t('accessGroups.confirm.deleteGroupTitle', { name }),
    message: t('accessGroups.confirm.deleteGroup'),
    confirmLabel: t('accessGroups.confirm.deleteGroupAction'),
    tone: 'danger',
    onConfirm: async () => {
      try {
        await deleteKernelAccessGroup(id)
      } catch (requestError) {
        throw new Error(apiErrorMessage(requestError, t('accessGroups.errors.deleteGroup')))
      }
    }
  })
  if (!confirmed) return
  error.value = ''
  if (selectedGroupID.value === id) {
    closeDetail()
    selectedGroupID.value = 0
    detail.value = null
  }
  toast.success(t('accessGroups.messages.groupDeleted', { name }))
  await loadGroups()
}

async function refreshSelectedDetail() {
  if (selectedGroupID.value) await selectGroup(selectedGroupID.value)
}

async function addMember() {
  if (!detail.value) return
  try {
    const id = positiveID(memberID.value, t('accessGroups.members.userID'))
    mutation.value = true
    await addKernelAccessGroupUser(detail.value.group.id, id)
    memberID.value = ''
    toast.success(t('accessGroups.messages.memberAdded', { id }))
    await refreshSelectedDetail()
  } catch (requestError) {
    error.value = apiErrorMessage(requestError, t('accessGroups.errors.member'))
  } finally {
    mutation.value = false
  }
}

// Removing a member or a plan is undone by adding it back, so neither asks
// first: the toast offers 撤销 instead (redesign plan §9).
async function changeMembership(action, errorKey) {
  mutation.value = true
  error.value = ''
  try {
    await action()
    await refreshSelectedDetail()
    return true
  } catch (requestError) {
    error.value = apiErrorMessage(requestError, t(errorKey))
    return false
  } finally {
    mutation.value = false
  }
}

async function removeMember(user) {
  if (!detail.value) return
  const groupID = detail.value.group.id
  if (!(await changeMembership(() => removeKernelAccessGroupUser(groupID, user.id), 'accessGroups.errors.member'))) return
  toast.success(t('accessGroups.messages.memberRemoved', { id: user.id }), {
    undo: () => changeMembership(() => addKernelAccessGroupUser(groupID, user.id), 'accessGroups.errors.member')
  })
}

async function addPlan() {
  if (!detail.value) return
  try {
    const id = positiveID(planID.value, t('accessGroups.plans.planID'))
    mutation.value = true
    await addKernelAccessGroupPlan(detail.value.group.id, id)
    planID.value = ''
    toast.success(t('accessGroups.messages.planAdded', { id }))
    await refreshSelectedDetail()
  } catch (requestError) {
    error.value = apiErrorMessage(requestError, t('accessGroups.errors.plan'))
  } finally {
    mutation.value = false
  }
}

async function removePlan(plan) {
  if (!detail.value) return
  const groupID = detail.value.group.id
  if (!(await changeMembership(() => removeKernelAccessGroupPlan(groupID, plan.id), 'accessGroups.errors.plan'))) return
  toast.success(t('accessGroups.messages.planRemoved', { id: plan.id }), {
    undo: () => changeMembership(() => addKernelAccessGroupPlan(groupID, plan.id), 'accessGroups.errors.plan')
  })
}

async function addGrant() {
  if (!detail.value) return
  try {
    ensureJSON(grantEditor.permissions, t('accessGroups.grants.permissions'))
    mutation.value = true
    await createKernelResourceGrant({ group_id: detail.value.group.id, resource_type: grantEditor.resourceType, resource_id: grantEditor.resourceID, permissions: grantEditor.permissions })
    toast.success(t('accessGroups.messages.grantAdded'))
    grantEditor.resourceID = ''
    grantEditor.permissions = '[]'
    await refreshSelectedDetail()
  } catch (requestError) {
    error.value = apiErrorMessage(requestError, t('accessGroups.errors.grant'))
  } finally {
    mutation.value = false
  }
}

async function removeGrant(grant) {
  const confirmed = await confirm({
    title: t('accessGroups.confirm.removeGrantTitle', { id: grant.id }),
    message: t('accessGroups.confirm.removeGrant', { resource: `${grant.resource_type}/${grant.resource_id}` }),
    confirmLabel: t('accessGroups.confirm.removeGrantAction'),
    tone: 'danger',
    onConfirm: async () => {
      try {
        await deleteKernelResourceGrant(grant.id)
      } catch (requestError) {
        throw new Error(apiErrorMessage(requestError, t('accessGroups.errors.grant')))
      }
    }
  })
  if (!confirmed) return
  error.value = ''
  toast.success(t('accessGroups.messages.grantRemoved'))
  await refreshSelectedDetail()
}

async function saveQuota() {
  if (!detail.value) return
  try {
    ensureJSON(quotaEditor.policy, t('accessGroups.quotas.policy'))
    mutation.value = true
    await upsertKernelQuotaPolicy({ group_id: detail.value.group.id, key: quotaEditor.key, policy: quotaEditor.policy })
    toast.success(t('accessGroups.messages.quotaSaved'))
    quotaEditor.key = ''
    quotaEditor.policy = '{}'
    await refreshSelectedDetail()
  } catch (requestError) {
    error.value = apiErrorMessage(requestError, t('accessGroups.errors.quota'))
  } finally {
    mutation.value = false
  }
}

async function removeQuota(policy) {
  const confirmed = await confirm({
    title: t('accessGroups.confirm.removeQuotaTitle', { key: policy.key }),
    message: t('accessGroups.confirm.removeQuota'),
    confirmLabel: t('accessGroups.confirm.removeQuotaAction'),
    tone: 'danger',
    onConfirm: async () => {
      try {
        await deleteKernelQuotaPolicy(policy.id)
      } catch (requestError) {
        throw new Error(apiErrorMessage(requestError, t('accessGroups.errors.quota')))
      }
    }
  })
  if (!confirmed) return
  error.value = ''
  toast.success(t('accessGroups.messages.quotaRemoved'))
  await refreshSelectedDetail()
}

async function resolveAccess() {
  resolverError.value = ''
  try {
    const userID = positiveID(resolver.userID, t('accessGroups.resolver.userID'))
    const planIDValue = resolver.planID ? positiveID(resolver.planID, t('accessGroups.resolver.planID')) : undefined
    if (!resolver.scopeID) throw new Error(t('accessGroups.errors.scopeRequired'))
    resolving.value = true
    resolvedAccess.value = await resolveKernelAccess({ userID, planID: planIDValue, scopeID: resolver.scopeID })
  } catch (requestError) {
    resolverError.value = apiErrorMessage(requestError, t('accessGroups.errors.resolve'))
  } finally {
    resolving.value = false
  }
}

onMounted(refresh)
</script>

<style scoped>
.access-scope {
  width: min(100%, 280px);
}

.access-scope__note {
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.access-group-name {
  display: flex;
  flex-direction: column;
  gap: var(--space-0-5);
  min-width: 0;
}

.access-group-name__title {
  font-weight: var(--weight-medium);
}

.access-group-name__desc {
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

code {
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
  overflow-wrap: anywhere;
}

.access-resolver__form {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr)) auto;
  gap: var(--space-4);
  align-items: end;
}

.resolver-result {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-4);
  margin-top: var(--space-4);
  border-radius: var(--radius-sm);
  background: var(--bg-grouped);
}

.resolver-result__chips {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}

.resolver-result__none,
.policy-section__empty {
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.policy-section {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.policy-section__title {
  display: flex;
  gap: var(--space-2);
  align-items: baseline;
  padding: 0 var(--space-4);
  color: var(--label-2);
  font-size: var(--type-callout-size);
  font-weight: var(--weight-regular);
}

.policy-section__count {
  color: var(--label-2);
  font-variant-numeric: tabular-nums;
}

.policy-form {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: var(--space-3);
  align-items: end;
  padding: var(--space-4);
  border-radius: var(--radius-md);
  background: var(--bg-elevated);
}

.policy-form--wide {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.policy-form__full {
  grid-column: 1 / -1;
}

.policy-form--wide > :last-child {
  grid-column: 1 / -1;
  justify-self: end;
}

.entity-list {
  display: flex;
  flex-direction: column;
  padding: 0;
  margin: 0;
  border-radius: var(--radius-md);
  background: var(--bg-elevated);
  list-style: none;
}

.entity-list__item {
  display: flex;
  gap: var(--space-2);
  align-items: center;
  justify-content: space-between;
  min-height: 44px;
  padding: var(--space-1) var(--space-2) var(--space-1) var(--space-4);
  border-bottom: 1px solid var(--separator);
}

.entity-list__item:last-child {
  border-bottom: 0;
}

.entity-list__text {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: baseline;
  min-width: 0;
  overflow-wrap: anywhere;
}

.entity-list__text code {
  color: var(--label-2);
}

.json-value {
  white-space: pre-wrap;
}

@media (max-width: 833.98px) {
  .access-resolver__form {
    grid-template-columns: 1fr;
  }

  .access-resolver__submit {
    justify-self: end;
  }
}

@media (max-width: 639.98px) {
  .policy-form--wide {
    grid-template-columns: 1fr;
  }
}
</style>
