<template>
  <div class="access-groups-page" :aria-busy="loading ? 'true' : 'false'">
    <div class="page-header">
      <div>
        <h1>{{ t('pageTitles.admin.accessGroups') }}</h1>
        <p class="page-subtitle">{{ t('accessGroups.subtitle') }}</p>
      </div>
      <div class="header-actions">
        <button class="btn btn-primary btn-sm" type="button" :disabled="scopes.length === 0" @click="openGroupEditor()">
          {{ t('accessGroups.actions.newGroup') }}
        </button>
        <button class="btn btn-sm" type="button" :disabled="loading" @click="refresh()">
          {{ loading ? t('accessGroups.actions.refreshing') : t('accessGroups.actions.refresh') }}
        </button>
      </div>
    </div>

    <p v-if="error" class="error-message" role="alert">{{ error }}</p>
    <p v-if="notice" class="notice-message" role="status">{{ notice }}</p>

    <section class="access-toolbar" :aria-label="t('accessGroups.filters.title')">
      <div class="form-group">
        <label for="access-scope-filter">{{ t('accessGroups.filters.scope') }}</label>
        <select id="access-scope-filter" v-model="scopeFilter" :disabled="loading" @change="loadGroups()">
          <option value="">{{ t('accessGroups.filters.allScopes') }}</option>
          <option v-for="scope in scopes" :key="scope.id" :value="scope.id">{{ scope.name || scope.id }} ({{ scope.id }})</option>
        </select>
      </div>
      <div class="scope-context">
        <strong>{{ activeScope?.name || t('accessGroups.filters.allScopes') }}</strong>
        <code v-if="activeScope">{{ activeScope.id }}</code>
        <span v-if="activeScope?.description">{{ activeScope.description }}</span>
      </div>
    </section>

    <div class="access-layout">
      <section class="access-list" :aria-labelledby="'access-groups-list-title'">
        <div class="section-heading">
          <h2 id="access-groups-list-title">{{ t('accessGroups.groups.title') }}</h2>
          <span class="secondary-cell">{{ t('accessGroups.groups.count', { count: groups.length }) }}</span>
        </div>
        <div class="table-container">
          <table class="data-table access-groups-table">
            <thead>
              <tr>
                <th>{{ t('accessGroups.table.group') }}</th>
                <th>{{ t('accessGroups.table.scope') }}</th>
                <th>{{ t('accessGroups.table.state') }}</th>
                <th>{{ t('accessGroups.table.actions') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-if="loading && groups.length === 0" class="state-row"><td colspan="4">{{ t('common.states.loading') }}</td></tr>
              <tr v-for="group in groups" :key="group.id" :class="{ selected: selectedGroupID === group.id }">
                <td>
                  <strong>{{ group.name }}</strong>
                  <span v-if="group.description" class="secondary-cell">{{ group.description }}</span>
                </td>
                <td><code>{{ group.scope_id }}</code></td>
                <td><span :class="['status-badge', group.enabled ? 'status-active' : 'status-error']">{{ group.enabled ? t('accessGroups.states.enabled') : t('accessGroups.states.disabled') }}</span></td>
                <td><button class="btn btn-sm" type="button" @click="selectGroup(group.id)">{{ t('accessGroups.actions.open') }}</button></td>
              </tr>
              <tr v-if="!loading && groups.length === 0"><td colspan="4" class="empty-row">{{ t('accessGroups.groups.empty') }}</td></tr>
            </tbody>
          </table>
        </div>
      </section>

      <section class="access-detail" :aria-live="detailLoading ? 'polite' : 'off'">
        <template v-if="detail">
          <div class="section-heading detail-heading">
            <div>
              <h2>{{ detail.group.name }}</h2>
              <p>{{ detail.group.description || t('accessGroups.groups.noDescription') }}</p>
            </div>
            <div class="row-actions">
              <button class="btn btn-sm" type="button" :disabled="detailLoading" @click="openGroupEditor(detail.group)">{{ t('common.actions.edit') }}</button>
              <button :class="['btn', 'btn-sm', detail.group.enabled ? 'btn-danger' : 'btn-primary']" type="button" :disabled="detailLoading" @click="toggleGroup()">
                {{ detail.group.enabled ? t('accessGroups.actions.disable') : t('accessGroups.actions.enable') }}
              </button>
              <button class="btn btn-sm btn-danger" type="button" :disabled="detailLoading" @click="removeGroup()">{{ t('common.actions.delete') }}</button>
            </div>
          </div>
          <div class="detail-meta"><span>{{ t('accessGroups.table.scope') }} <code>{{ detail.group.scope_id }}</code></span><span>{{ t('accessGroups.groups.directUnion') }}</span></div>

          <div class="policy-grid">
            <section class="policy-section" aria-labelledby="access-members-title">
              <div class="policy-heading"><h3 id="access-members-title">{{ t('accessGroups.members.title') }}</h3><span>{{ detail.users.length }}</span></div>
              <form class="compact-form" @submit.prevent="addMember">
                <label for="access-member-id">{{ t('accessGroups.members.userID') }}</label>
                <input id="access-member-id" v-model.trim="memberID" inputmode="numeric" type="number" min="1" required />
                <button class="btn btn-sm" type="submit" :disabled="mutation">{{ t('accessGroups.actions.add') }}</button>
              </form>
              <ul class="entity-list">
                <li v-for="user in detail.users" :key="user.id"><span><strong>{{ user.email || `#${user.id}` }}</strong><code>#{{ user.id }}</code></span><button class="btn btn-sm btn-danger" type="button" :disabled="mutation" @click="removeMember(user)">{{ t('common.actions.remove') }}</button></li>
                <li v-if="detail.users.length === 0" class="empty-list">{{ t('accessGroups.members.empty') }}</li>
              </ul>
            </section>

            <section class="policy-section" aria-labelledby="access-plans-title">
              <div class="policy-heading"><h3 id="access-plans-title">{{ t('accessGroups.plans.title') }}</h3><span>{{ detail.plans.length }}</span></div>
              <form class="compact-form" @submit.prevent="addPlan">
                <label for="access-plan-id">{{ t('accessGroups.plans.planID') }}</label>
                <input id="access-plan-id" v-model.trim="planID" inputmode="numeric" type="number" min="1" required />
                <button class="btn btn-sm" type="submit" :disabled="mutation">{{ t('accessGroups.actions.add') }}</button>
              </form>
              <ul class="entity-list">
                <li v-for="plan in detail.plans" :key="plan.id"><span><strong>{{ plan.name || `#${plan.id}` }}</strong><code>#{{ plan.id }}</code></span><button class="btn btn-sm btn-danger" type="button" :disabled="mutation" @click="removePlan(plan)">{{ t('common.actions.remove') }}</button></li>
                <li v-if="detail.plans.length === 0" class="empty-list">{{ t('accessGroups.plans.empty') }}</li>
              </ul>
            </section>

            <section class="policy-section policy-section-wide" aria-labelledby="access-grants-title">
              <div class="policy-heading"><h3 id="access-grants-title">{{ t('accessGroups.grants.title') }}</h3><span>{{ detail.resource_grants.length }}</span></div>
              <form class="grant-form" @submit.prevent="addGrant">
                <div class="form-group"><label for="access-grant-type">{{ t('accessGroups.grants.resourceType') }}</label><input id="access-grant-type" v-model.trim="grantEditor.resourceType" required maxlength="64" placeholder="plugin_api" /></div>
                <div class="form-group"><label for="access-grant-id">{{ t('accessGroups.grants.resourceID') }}</label><input id="access-grant-id" v-model.trim="grantEditor.resourceID" required maxlength="160" placeholder="machine-telemetry" /></div>
                <div class="form-group grant-permissions"><label for="access-grant-permissions">{{ t('accessGroups.grants.permissions') }}</label><input id="access-grant-permissions" v-model.trim="grantEditor.permissions" required spellcheck="false" /></div>
                <button class="btn btn-sm" type="submit" :disabled="mutation">{{ t('accessGroups.actions.addGrant') }}</button>
              </form>
              <div class="table-container">
                <table class="data-table compact-table"><thead><tr><th>{{ t('accessGroups.grants.resourceType') }}</th><th>{{ t('accessGroups.grants.resourceID') }}</th><th>{{ t('accessGroups.grants.permissions') }}</th><th>{{ t('accessGroups.table.actions') }}</th></tr></thead><tbody>
                  <tr v-for="grant in detail.resource_grants" :key="grant.id"><td><code>{{ grant.resource_type }}</code></td><td><code>{{ grant.resource_id }}</code></td><td><code class="json-value">{{ grant.permissions }}</code></td><td><button class="btn btn-sm btn-danger" type="button" :disabled="mutation" @click="removeGrant(grant)">{{ t('common.actions.remove') }}</button></td></tr>
                  <tr v-if="detail.resource_grants.length === 0"><td colspan="4" class="empty-row">{{ t('accessGroups.grants.empty') }}</td></tr>
                </tbody></table>
              </div>
            </section>

            <section class="policy-section policy-section-wide" aria-labelledby="access-quotas-title">
              <div class="policy-heading"><h3 id="access-quotas-title">{{ t('accessGroups.quotas.title') }}</h3><span>{{ detail.quota_policies.length }}</span></div>
              <form class="quota-form" @submit.prevent="saveQuota">
                <div class="form-group"><label for="access-quota-key">{{ t('accessGroups.quotas.key') }}</label><input id="access-quota-key" v-model.trim="quotaEditor.key" required maxlength="80" placeholder="machine-telemetry.rate" /></div>
                <div class="form-group quota-policy"><label for="access-quota-policy">{{ t('accessGroups.quotas.policy') }}</label><textarea id="access-quota-policy" v-model="quotaEditor.policy" required rows="2" spellcheck="false"></textarea></div>
                <button class="btn btn-sm" type="submit" :disabled="mutation">{{ t('accessGroups.actions.saveQuota') }}</button>
              </form>
              <div class="table-container">
                <table class="data-table compact-table"><thead><tr><th>{{ t('accessGroups.quotas.key') }}</th><th>{{ t('accessGroups.quotas.policy') }}</th><th>{{ t('accessGroups.table.actions') }}</th></tr></thead><tbody>
                  <tr v-for="policy in detail.quota_policies" :key="policy.id"><td><code>{{ policy.key }}</code></td><td><code class="json-value">{{ policy.policy }}</code></td><td><button class="btn btn-sm btn-danger" type="button" :disabled="mutation" @click="removeQuota(policy)">{{ t('common.actions.remove') }}</button></td></tr>
                  <tr v-if="detail.quota_policies.length === 0"><td colspan="3" class="empty-row">{{ t('accessGroups.quotas.empty') }}</td></tr>
                </tbody></table>
              </div>
            </section>
          </div>
        </template>
        <p v-else-if="detailLoading" class="empty-state">{{ t('accessGroups.detail.loading') }}</p>
        <p v-else class="empty-state">{{ t('accessGroups.detail.empty') }}</p>
      </section>
    </div>

    <section class="access-resolver" :aria-labelledby="'access-resolver-title'">
      <div class="section-heading"><div><h2 id="access-resolver-title">{{ t('accessGroups.resolver.title') }}</h2><p>{{ t('accessGroups.resolver.description') }}</p></div></div>
      <form class="resolver-form" @submit.prevent="resolveAccess">
        <div class="form-group"><label for="access-resolve-user">{{ t('accessGroups.resolver.userID') }}</label><input id="access-resolve-user" v-model.trim="resolver.userID" inputmode="numeric" type="number" min="1" required /></div>
        <div class="form-group"><label for="access-resolve-plan">{{ t('accessGroups.resolver.planID') }}</label><input id="access-resolve-plan" v-model.trim="resolver.planID" inputmode="numeric" type="number" min="1" /></div>
        <div class="form-group"><label for="access-resolve-scope">{{ t('accessGroups.resolver.scope') }}</label><select id="access-resolve-scope" v-model="resolver.scopeID" required><option v-for="scope in scopes" :key="scope.id" :value="scope.id">{{ scope.name || scope.id }} ({{ scope.id }})</option></select></div>
        <button class="btn btn-primary btn-sm" type="submit" :disabled="resolving">{{ resolving ? t('accessGroups.actions.resolving') : t('accessGroups.actions.resolve') }}</button>
      </form>
      <div v-if="resolvedAccess" class="resolver-result">
        <strong>{{ t('accessGroups.resolver.result', { count: resolvedAccess.groups?.length || 0 }) }}</strong>
        <div class="resolver-chips"><span v-for="group in resolvedAccess.groups || []" :key="group.id">{{ group.name }} <code>#{{ group.id }}</code></span><span v-if="(resolvedAccess.groups || []).length === 0" class="secondary-cell">{{ t('accessGroups.resolver.none') }}</span></div>
        <p>{{ t('accessGroups.resolver.policySummary', { grants: resolvedAccess.grants?.length || 0, quotas: resolvedAccess.quotas?.length || 0 }) }}</p>
      </div>
    </section>

    <div v-if="groupEditor.open" class="modal-overlay" @click.self="closeGroupEditor">
      <section class="modal" role="dialog" aria-modal="true" aria-labelledby="access-group-editor-title">
        <div class="modal-header"><h3 id="access-group-editor-title">{{ groupEditor.mode === 'create' ? t('accessGroups.editor.createTitle') : t('accessGroups.editor.editTitle') }}</h3><button class="btn btn-ghost close-btn" type="button" :aria-label="t('common.actions.close')" @click="closeGroupEditor">x</button></div>
        <form @submit.prevent="saveGroup">
          <div class="modal-body form-grid">
            <div class="form-group"><label for="access-group-scope">{{ t('accessGroups.table.scope') }}</label><select id="access-group-scope" v-model="groupEditor.scopeID" :disabled="groupEditor.mode === 'edit'" required><option v-for="scope in scopes" :key="scope.id" :value="scope.id">{{ scope.name || scope.id }} ({{ scope.id }})</option></select></div>
            <div class="form-group"><label for="access-group-name">{{ t('accessGroups.editor.name') }}</label><input id="access-group-name" v-model.trim="groupEditor.name" required maxlength="120" /></div>
            <div class="form-group form-group-wide"><label for="access-group-description">{{ t('accessGroups.editor.description') }}</label><textarea id="access-group-description" v-model="groupEditor.description" rows="3" maxlength="4000"></textarea></div>
            <label class="checkbox-row"><input v-model="groupEditor.enabled" type="checkbox" />{{ t('accessGroups.editor.enabled') }}</label>
          </div>
          <div class="modal-footer"><button class="btn" type="button" @click="closeGroupEditor">{{ t('common.actions.cancel') }}</button><button class="btn btn-primary" type="submit" :disabled="mutation">{{ mutation ? t('accessGroups.actions.saving') : t('common.actions.save') }}</button></div>
        </form>
      </section>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
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
const loading = ref(false)
const detailLoading = ref(false)
const mutation = ref(false)
const resolving = ref(false)
const error = ref('')
const notice = ref('')
const scopes = ref([])
const groups = ref([])
const scopeFilter = ref('')
const selectedGroupID = ref(0)
const detail = ref(null)
const memberID = ref('')
const planID = ref('')
const resolvedAccess = ref(null)
const resolver = reactive({ userID: '', planID: '', scopeID: '' })
const groupEditor = reactive({ open: false, mode: 'create', id: 0, scopeID: '', name: '', description: '', enabled: true })
const grantEditor = reactive({ resourceType: 'plugin_api', resourceID: '', permissions: '[]' })
const quotaEditor = reactive({ key: '', policy: '{}' })

const activeScope = computed(() => scopes.value.find(scope => scope.id === scopeFilter.value) || null)

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
    error.value = apiErrorMessage(requestError, t('accessGroups.errors.load'))
  } finally {
    loading.value = false
  }
}

async function loadGroups() {
  error.value = ''
  try {
    groups.value = await getKernelAccessGroups(scopeFilter.value || undefined)
    const current = groups.value.find(group => group.id === selectedGroupID.value)
    const next = current || groups.value[0]
    if (next) {
      await selectGroup(next.id)
    } else {
      selectedGroupID.value = 0
      detail.value = null
    }
  } catch (requestError) {
    error.value = apiErrorMessage(requestError, t('accessGroups.errors.loadGroups'))
  }
}

async function selectGroup(groupID) {
  selectedGroupID.value = Number(groupID)
  detailLoading.value = true
  error.value = ''
  try {
    detail.value = await getKernelAccessGroupDetail(selectedGroupID.value)
  } catch (requestError) {
    detail.value = null
    error.value = apiErrorMessage(requestError, t('accessGroups.errors.loadDetail'))
  } finally {
    detailLoading.value = false
  }
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
}

function closeGroupEditor() {
  groupEditor.open = false
}

async function saveGroup() {
  if (!groupEditor.scopeID || !groupEditor.name) {
    error.value = t('accessGroups.errors.groupRequired')
    return
  }
  mutation.value = true
  error.value = ''
  try {
    if (groupEditor.mode === 'create') {
      const created = await createKernelAccessGroup({ scope_id: groupEditor.scopeID, name: groupEditor.name, description: groupEditor.description, enabled: groupEditor.enabled })
      selectedGroupID.value = created.id
      notice.value = t('accessGroups.messages.groupCreated', { name: created.name })
    } else {
      await updateKernelAccessGroup(groupEditor.id, { name: groupEditor.name, description: groupEditor.description, enabled: groupEditor.enabled })
      selectedGroupID.value = groupEditor.id
      notice.value = t('accessGroups.messages.groupSaved', { name: groupEditor.name })
    }
    closeGroupEditor()
    await loadGroups()
  } catch (requestError) {
    error.value = apiErrorMessage(requestError, t('accessGroups.errors.saveGroup'))
  } finally {
    mutation.value = false
  }
}

async function toggleGroup() {
  if (!detail.value) return
  const group = detail.value.group
  mutation.value = true
  error.value = ''
  try {
    await updateKernelAccessGroup(group.id, { name: group.name, description: group.description, enabled: !group.enabled })
    notice.value = !group.enabled ? t('accessGroups.messages.groupEnabled', { name: group.name }) : t('accessGroups.messages.groupDisabled', { name: group.name })
    await loadGroups()
  } catch (requestError) {
    error.value = apiErrorMessage(requestError, t('accessGroups.errors.saveGroup'))
  } finally {
    mutation.value = false
  }
}

async function removeGroup() {
  if (!detail.value || !confirm(t('accessGroups.confirm.deleteGroup', { name: detail.value.group.name }))) return
  mutation.value = true
  error.value = ''
  try {
    const name = detail.value.group.name
    await deleteKernelAccessGroup(detail.value.group.id)
    selectedGroupID.value = 0
    detail.value = null
    notice.value = t('accessGroups.messages.groupDeleted', { name })
    await loadGroups()
  } catch (requestError) {
    error.value = apiErrorMessage(requestError, t('accessGroups.errors.deleteGroup'))
  } finally {
    mutation.value = false
  }
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
    notice.value = t('accessGroups.messages.memberAdded', { id })
    await refreshSelectedDetail()
  } catch (requestError) {
    error.value = apiErrorMessage(requestError, t('accessGroups.errors.member'))
  } finally {
    mutation.value = false
  }
}

async function removeMember(user) {
  if (!detail.value || !confirm(t('accessGroups.confirm.removeMember', { id: user.id }))) return
  mutation.value = true
  try {
    await removeKernelAccessGroupUser(detail.value.group.id, user.id)
    notice.value = t('accessGroups.messages.memberRemoved', { id: user.id })
    await refreshSelectedDetail()
  } catch (requestError) {
    error.value = apiErrorMessage(requestError, t('accessGroups.errors.member'))
  } finally {
    mutation.value = false
  }
}

async function addPlan() {
  if (!detail.value) return
  try {
    const id = positiveID(planID.value, t('accessGroups.plans.planID'))
    mutation.value = true
    await addKernelAccessGroupPlan(detail.value.group.id, id)
    planID.value = ''
    notice.value = t('accessGroups.messages.planAdded', { id })
    await refreshSelectedDetail()
  } catch (requestError) {
    error.value = apiErrorMessage(requestError, t('accessGroups.errors.plan'))
  } finally {
    mutation.value = false
  }
}

async function removePlan(plan) {
  if (!detail.value || !confirm(t('accessGroups.confirm.removePlan', { id: plan.id }))) return
  mutation.value = true
  try {
    await removeKernelAccessGroupPlan(detail.value.group.id, plan.id)
    notice.value = t('accessGroups.messages.planRemoved', { id: plan.id })
    await refreshSelectedDetail()
  } catch (requestError) {
    error.value = apiErrorMessage(requestError, t('accessGroups.errors.plan'))
  } finally {
    mutation.value = false
  }
}

async function addGrant() {
  if (!detail.value) return
  try {
    ensureJSON(grantEditor.permissions, t('accessGroups.grants.permissions'))
    mutation.value = true
    await createKernelResourceGrant({ group_id: detail.value.group.id, resource_type: grantEditor.resourceType, resource_id: grantEditor.resourceID, permissions: grantEditor.permissions })
    notice.value = t('accessGroups.messages.grantAdded')
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
  if (!confirm(t('accessGroups.confirm.removeGrant', { id: grant.id }))) return
  mutation.value = true
  try {
    await deleteKernelResourceGrant(grant.id)
    notice.value = t('accessGroups.messages.grantRemoved')
    await refreshSelectedDetail()
  } catch (requestError) {
    error.value = apiErrorMessage(requestError, t('accessGroups.errors.grant'))
  } finally {
    mutation.value = false
  }
}

async function saveQuota() {
  if (!detail.value) return
  try {
    ensureJSON(quotaEditor.policy, t('accessGroups.quotas.policy'))
    mutation.value = true
    await upsertKernelQuotaPolicy({ group_id: detail.value.group.id, key: quotaEditor.key, policy: quotaEditor.policy })
    notice.value = t('accessGroups.messages.quotaSaved')
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
  if (!confirm(t('accessGroups.confirm.removeQuota', { key: policy.key }))) return
  mutation.value = true
  try {
    await deleteKernelQuotaPolicy(policy.id)
    notice.value = t('accessGroups.messages.quotaRemoved')
    await refreshSelectedDetail()
  } catch (requestError) {
    error.value = apiErrorMessage(requestError, t('accessGroups.errors.quota'))
  } finally {
    mutation.value = false
  }
}

async function resolveAccess() {
  try {
    const userID = positiveID(resolver.userID, t('accessGroups.resolver.userID'))
    const planIDValue = resolver.planID ? positiveID(resolver.planID, t('accessGroups.resolver.planID')) : undefined
    if (!resolver.scopeID) throw new Error(t('accessGroups.errors.scopeRequired'))
    resolving.value = true
    resolvedAccess.value = await resolveKernelAccess({ userID, planID: planIDValue, scopeID: resolver.scopeID })
  } catch (requestError) {
    error.value = apiErrorMessage(requestError, t('accessGroups.errors.resolve'))
  } finally {
    resolving.value = false
  }
}

onMounted(refresh)
</script>

<style scoped>
.access-groups-page { display: grid; gap: 20px; }
.header-actions, .row-actions, .detail-meta, .resolver-chips { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.access-toolbar { display: flex; align-items: end; justify-content: space-between; gap: 20px; padding: 14px 0; border-block: 1px solid var(--border-color); }
.access-toolbar .form-group { min-width: min(100%, 280px); margin: 0; }
.scope-context { display: grid; gap: 2px; color: var(--text-secondary); font-size: 13px; }
.scope-context strong { color: var(--text-color); }
.access-layout { display: grid; grid-template-columns: minmax(300px, .8fr) minmax(0, 1.6fr); gap: 24px; align-items: start; }
.access-list, .access-detail, .access-resolver { min-width: 0; border-top: 2px solid var(--border-color); padding-top: 14px; }
.section-heading { display: flex; justify-content: space-between; align-items: start; gap: 16px; margin-bottom: 12px; }
.section-heading h2, .policy-heading h3 { margin: 0; font-size: 17px; }
.section-heading p { margin: 5px 0 0; color: var(--text-secondary); font-size: 13px; }
.access-groups-table tr.selected td { background: color-mix(in srgb, var(--primary-color) 9%, transparent); }
.secondary-cell { display: block; color: var(--text-secondary); font-size: 12px; margin-top: 3px; }
.detail-meta { color: var(--text-secondary); font-size: 12px; padding-bottom: 14px; border-bottom: 1px solid var(--border-color); }
.policy-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 20px; margin-top: 18px; }
.policy-section { min-width: 0; border-top: 1px solid var(--border-color); padding-top: 12px; }
.policy-section-wide { grid-column: 1 / -1; }
.policy-heading { display: flex; justify-content: space-between; align-items: center; gap: 12px; margin-bottom: 10px; }
.policy-heading > span { min-width: 26px; text-align: center; padding: 2px 7px; background: var(--bg-secondary); font-size: 12px; }
.compact-form, .grant-form, .quota-form, .resolver-form { display: grid; gap: 8px; align-items: end; margin-bottom: 12px; }
.compact-form { grid-template-columns: 1fr 92px; }
.compact-form label { grid-column: 1 / -1; font-size: 12px; color: var(--text-secondary); }
.grant-form { grid-template-columns: minmax(120px, .8fr) minmax(160px, 1fr) minmax(180px, 1.4fr) auto; }
.quota-form { grid-template-columns: minmax(160px, .8fr) minmax(240px, 1.4fr) auto; }
.grant-form .form-group, .quota-form .form-group, .resolver-form .form-group { margin: 0; }
.grant-form label, .quota-form label { font-size: 12px; }
.entity-list { list-style: none; padding: 0; margin: 0; display: grid; gap: 7px; }
.entity-list li { display: flex; align-items: center; justify-content: space-between; gap: 10px; padding: 8px 0; border-top: 1px solid var(--border-color); }
.entity-list li span { min-width: 0; display: grid; gap: 2px; overflow-wrap: anywhere; }
.entity-list li code { color: var(--text-secondary); }
.empty-list { color: var(--text-secondary); font-size: 13px; }
.compact-table { min-width: 640px; }
.json-value { display: block; max-width: 360px; white-space: pre-wrap; overflow-wrap: anywhere; font-size: 12px; }
.access-resolver { padding-top: 16px; }
.resolver-form { grid-template-columns: minmax(130px, .65fr) minmax(130px, .65fr) minmax(180px, 1fr) auto; }
.resolver-form .form-group { min-width: 0; }
.resolver-result { margin-top: 10px; padding-top: 12px; border-top: 1px solid var(--border-color); }
.resolver-result p { margin: 8px 0 0; color: var(--text-secondary); font-size: 13px; }
.resolver-chips { margin-top: 8px; }
.resolver-chips > span { padding: 4px 8px; border: 1px solid var(--border-color); font-size: 13px; }
.form-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px; }
.form-group-wide { grid-column: 1 / -1; }
.checkbox-row { display: flex; align-items: center; gap: 8px; font-size: 14px; }
@media (max-width: 980px) { .access-layout, .policy-grid { grid-template-columns: 1fr; } .policy-section-wide { grid-column: auto; } .grant-form, .quota-form, .resolver-form { grid-template-columns: 1fr; } .grant-form .btn, .quota-form .btn, .resolver-form .btn { justify-self: start; } }
@media (max-width: 620px) { .access-toolbar { align-items: stretch; flex-direction: column; } .detail-heading { flex-direction: column; } .form-grid { grid-template-columns: 1fr; } .form-group-wide { grid-column: auto; } }
</style>
