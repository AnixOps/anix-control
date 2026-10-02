<template>
  <UiSection :title="t('runtime.nodeXTopology.legacy.title')" :description="t('runtime.nodeXTopology.legacyText')" class="fn-rules">
    <template #actions>
      <UiButton :icon="Plus" data-test="forward-rule-add" @click="openEditor()">{{ t('runtime.nodeXTopology.actions.addRule') }}</UiButton>
    </template>
    <UiDataTable
      :columns="columns"
      :rows="rules"
      :label="t('forwardNodesPage.rules.tableLabel')"
      :row-label="rule => rule.name"
      storage-key="admin.forward-rules"
      :loading="loading"
      :error="error"
      :error-title="t('runtime.nodeXTopology.messages.loadRulesFailed')"
      :filtered="Boolean(userFilter)"
      :empty-icon="Route"
      :empty-title="t('runtime.nodeXTopology.legacy.emptyTitle')"
      :empty-description="t('runtime.nodeXTopology.legacy.emptyText')"
      :row-actions="ruleActions"
      :page="page"
      :page-size="pageSize"
      :total="total"
      manual-pagination
      state-heading-tag="h3"
      @update:page="changePage"
      @retry="load"
      @clear-filters="clearFilter"
    >
      <template #toolbar>
        <form class="fn-rules__filter" role="search" @submit.prevent="applyFilter">
          <UiTextField
            v-model.trim="userFilterInput"
            size="md"
            inputmode="numeric"
            :label="t('forwardNodesPage.rules.userFilterLabel')"
            :placeholder="t('runtime.nodeXTopology.filters.userIdPlaceholder')"
            :error="filterError"
          />
          <UiButton type="submit" size="md">{{ t('forwardNodesPage.rules.apply') }}</UiButton>
          <UiButton v-if="userFilter || userFilterInput" variant="tertiary" size="md" @click="clearFilter">{{ t('runtime.nodeXTopology.actions.clear') }}</UiButton>
        </form>
      </template>
      <template #empty-actions>
        <UiButton variant="primary" :icon="Plus" @click="openEditor()">{{ t('runtime.nodeXTopology.actions.addRule') }}</UiButton>
      </template>
      <template #cell-name="{ row }">
        <span class="fn-stack">
          <span>{{ row.name }}</span>
          <span class="fn-sub">#{{ row.id }} · {{ formatTime(row.updatedAt) }}</span>
        </span>
      </template>
      <template #cell-ingress="{ row }">
        <span class="fn-stack">
          <span>{{ row.relayName }}</span>
          <span class="fn-sub">{{ protocolLabel(row.protocol) }} / :{{ row.listenPort }}</span>
        </span>
      </template>
      <template #cell-egress="{ row }">
        <span class="fn-stack">
          <span>{{ row.exitName }}</span>
          <code class="fn-sub">{{ row.targetHost }}:{{ row.targetPort }}</code>
        </span>
      </template>
      <template #cell-limits="{ row }">
        <span class="fn-stack fn-sub">
          <span>{{ t('runtime.nodeXTopology.meta.rateLimit') }} {{ formatSpeedLimit(row.speedLimit) }}</span>
          <span>{{ t('runtime.nodeXTopology.meta.trafficLimit') }} {{ formatTrafficLimit(row.trafficLimit) }}</span>
          <span>{{ t('runtime.nodeXTopology.meta.expire') }} {{ formatExpireTime(row.expireTime) }}</span>
        </span>
      </template>
      <template #cell-traffic="{ row }">
        <span class="fn-stack fn-sub">
          <span>{{ t('runtime.nodeXTopology.meta.upload') }} {{ format.bytes(row.upload) }}</span>
          <span>{{ t('runtime.nodeXTopology.meta.download') }} {{ format.bytes(row.download) }}</span>
          <span>{{ t('runtime.nodeXTopology.meta.connections') }} {{ row.connections }}</span>
        </span>
      </template>
      <template #cell-status="{ row }">
        <span class="fn-badges">
          <UiBadge :status="row.enabled ? 'online' : 'disabled'" :label="row.enabled ? t('runtime.nodeXTopology.status.enabled') : t('runtime.nodeXTopology.status.disabled')" />
          <UiBadge tone="neutral" :dot="false" :label="protocolLabel(row.protocol)" />
        </span>
      </template>
    </UiDataTable>

    <NodeXRuleDialog v-model:open="editorOpen" :rule-id="editingId" @saved="onSaved" />
  </UiSection>
</template>

<script setup>
// 旧版规则 (/admin/forward/rules*): the legacy-rules compatibility layer of
// the NodeX nodes page. Server pages of 10, filtered by user ID; edit,
// enable/disable and delete (plain confirmation) as before U7.
import { computed, onMounted, ref } from 'vue'
import { Pencil, Plus, Power, Route, Trash2 } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { deleteForwardRule, getForwardRules, toggleForwardRule } from '@/api/admin'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiSection from '@/ui/UiSection.vue'
import UiTextField from '@/ui/UiTextField.vue'
import { useConfirm } from '@/ui/composables/useConfirm'
import { useFormat } from '@/ui/composables/useFormat'
import { useToast } from '@/ui/composables/useToast'
import NodeXRuleDialog from './NodeXRuleDialog.vue'
import { errorMessage, listOf, normalizeRule, parsePositiveInt, unwrapForwardResponse } from './forwardNodeModel'

const emit = defineEmits(['changed'])

const { t, translateLiteral } = useAppI18n()
const toast = useToast()
const confirm = useConfirm()
const format = useFormat()

const rules = ref([])
const loading = ref(false)
const error = ref(null)
const page = ref(1)
const pageSize = 10
const total = ref(0)
const userFilter = ref('')
const userFilterInput = ref('')
const filterError = ref('')
const pending = ref('')
const editorOpen = ref(false)
const editingId = ref(null)

const columns = computed(() => [
  { key: 'name', label: t('forwardNodesPage.rules.columns.name'), primary: true },
  { key: 'ingress', label: t('forwardNodesPage.rules.columns.ingress'), secondary: true },
  { key: 'egress', label: t('forwardNodesPage.rules.columns.egress') },
  { key: 'owner', label: t('forwardNodesPage.rules.columns.owner'), value: rule => ownerLabel(rule) },
  { key: 'limits', label: t('forwardNodesPage.rules.columns.limits'), breakpoint: 'md' },
  { key: 'traffic', label: t('forwardNodesPage.rules.columns.traffic'), breakpoint: 'lg' },
  { key: 'status', label: t('forwardNodesPage.rules.columns.status') }
])

function ruleActions(rule) {
  return [
    { key: 'edit', label: t('runtime.nodeXTopology.actions.edit'), icon: Pencil, onSelect: () => openEditor(rule) },
    { key: 'toggle', label: rule.enabled ? t('runtime.nodeXTopology.actions.disable') : t('runtime.nodeXTopology.actions.enable'), icon: Power, disabled: pending.value === `${rule.id}:toggle`, onSelect: () => toggle(rule) },
    { key: 'delete', label: t('runtime.nodeXTopology.actions.delete'), icon: Trash2, danger: true, separatorBefore: true, onSelect: () => remove(rule) }
  ]
}

function failure(err, key) {
  return errorMessage(err, t(key), translateLiteral)
}

function formatTime(value) {
  return value ? format.dateTime(value) : '—'
}

function protocolLabel(value) {
  const protocol = String(value || '').toLowerCase()
  if (['tcp', 'udp', 'both'].includes(protocol)) return t(`runtime.nodeXTopology.protocols.${protocol}`)
  return protocol ? protocol.toUpperCase() : '-'
}

function ownerLabel(rule) {
  if (rule.userId) return t('runtime.nodeXTopology.owner.user', { id: rule.userId })
  if (rule.userGroupId) return t('runtime.nodeXTopology.owner.userGroup', { id: rule.userGroupId })
  return t('runtime.nodeXTopology.owner.public')
}

function isUnset(value) {
  return value === null || value === undefined || value === ''
}

function formatSpeedLimit(value) {
  return isUnset(value) ? t('runtime.nodeXTopology.labels.none') : `${value} KB/s`
}

function formatTrafficLimit(value) {
  return isUnset(value) ? t('runtime.nodeXTopology.labels.none') : format.bytes(Number(value))
}

function formatExpireTime(value) {
  return value ? format.dateTime(value) : t('runtime.nodeXTopology.labels.neverExpires')
}

async function load() {
  loading.value = true
  error.value = null
  try {
    const params = { page: page.value, page_size: pageSize }
    const userId = parsePositiveInt(userFilter.value)
    if (userId) params.user_id = userId
    const payload = unwrapForwardResponse(await getForwardRules(params), t('runtime.nodeXTopology.validation.requestFailed'))
    const list = listOf(payload)
    rules.value = list.map(normalizeRule)
    total.value = Number(payload?.total ?? list.length)
  } catch (err) {
    rules.value = []
    total.value = 0
    error.value = failure(err, 'runtime.nodeXTopology.messages.loadRulesFailed')
  } finally {
    loading.value = false
  }
}

function changePage(next) {
  page.value = next
  load()
}

function applyFilter() {
  filterError.value = ''
  if (userFilterInput.value.trim() && !parsePositiveInt(userFilterInput.value)) {
    filterError.value = t('runtime.nodeXTopology.validation.userIdPositive')
    return
  }
  userFilter.value = userFilterInput.value.trim()
  page.value = 1
  load()
}

function clearFilter() {
  filterError.value = ''
  userFilter.value = ''
  userFilterInput.value = ''
  page.value = 1
  load()
}

function openEditor(rule = null) {
  editingId.value = rule?.id || null
  editorOpen.value = true
}

function onSaved() {
  load()
  emit('changed')
}

async function toggle(rule) {
  pending.value = `${rule.id}:toggle`
  try {
    await toggleForwardRule(rule.id, !rule.enabled)
    toast.success(t(rule.enabled ? 'runtime.nodeXTopology.messages.ruleDisabled' : 'runtime.nodeXTopology.messages.ruleEnabled', { name: rule.name }))
    await load()
  } catch (err) {
    toast.error(failure(err, 'runtime.nodeXTopology.messages.ruleToggleFailed'))
  } finally {
    pending.value = ''
  }
}

// A rule asks for a plain confirmation (a node asks for its name).
async function remove(rule) {
  if (!rule?.id) return
  const name = rule.name || `#${rule.id}`
  const confirmed = await confirm({
    title: t('runtime.nodeXTopology.deleteModal.titleRule', { name }),
    message: t('runtime.nodeXTopology.deleteModal.warning'),
    confirmLabel: t('runtime.nodeXTopology.deleteModal.deleteRule'),
    tone: 'danger',
    onConfirm: async () => {
      try {
        await deleteForwardRule(rule.id)
      } catch (err) {
        throw new Error(failure(err, 'runtime.nodeXTopology.messages.deleteFailed'))
      }
    }
  })
  if (!confirmed) return
  toast.success(t('runtime.nodeXTopology.messages.ruleDeleted'))
  await load()
}

defineExpose({ load, openEditor })

onMounted(load)
</script>

<style scoped>
.fn-rules__filter {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: flex-end;
}

.fn-rules__filter > :first-child {
  flex: 0 1 240px;
}

.fn-stack {
  display: inline-flex;
  flex-direction: column;
  gap: var(--space-0-5);
  min-width: 0;
}

.fn-sub {
  color: color-mix(in srgb, var(--label-2) 70%, var(--label-1));
  font-size: var(--type-caption-size);
}

code.fn-sub {
  font-family: var(--font-mono);
}

.fn-badges {
  display: inline-flex;
  flex-wrap: wrap;
  gap: var(--space-1);
}
</style>
