<template>
  <UiSheet
    :open="open"
    size="md"
    :title="t('adminSubscriptionGroups.nodeEdit.title')"
    :description="t('adminSubscriptionGroups.nodeEdit.description', { group: group.name })"
    :dismissible="!saving"
    @update:open="value => emit('update:open', value)"
  >
    <p v-if="loading" class="node-edit__note">{{ t('common.states.loading') }}</p>
    <p v-else-if="!rows.length" class="node-edit__note">{{ t('adminSubscriptionGroups.nodeEdit.empty') }}</p>
    <form v-else class="node-edit" novalidate data-test="node-edit-form" @submit.prevent="save">
      <section v-for="row in rows" :key="row.key" class="node-edit__item" :data-test="`node-edit-${row.key || 'base'}`">
        <header class="node-edit__head">
          <strong>{{ row.title }}</strong>
          <UiBadge v-if="isEdited(row)" tone="info" :dot="false" :label="t('adminSubscriptionGroups.nodeEdit.edited')" />
          <UiButton size="sm" :disabled="!isEdited(row)" @click="resetRow(row)">{{ t('adminSubscriptionGroups.nodeEdit.resetNode') }}</UiButton>
        </header>
        <UiTextField v-model="row.fields.name" size="sm" :label="t('adminSubscriptionGroups.nodeEdit.name')" :placeholder="show(row, 'name')" />
        <UiTextField v-model="row.fields.server" size="sm" :label="t('adminSubscriptionGroups.nodeEdit.server')" :placeholder="show(row, 'server')" :help="hint(row, 'server')" />
        <UiTextField v-model="row.fields.port" size="sm" inputmode="numeric" :label="t('adminSubscriptionGroups.nodeEdit.port')" :placeholder="show(row, 'port')" :error="portError(row)" />
        <UiTextField v-model="row.fields.server_name" size="sm" :label="t('adminSubscriptionGroups.nodeEdit.serverName')" :placeholder="show(row, 'server_name')" />
        <UiTextarea v-model="row.advanced" :rows="4" :label="t('adminSubscriptionGroups.nodeEdit.advanced')" :error="advancedError(row)" />
      </section>
      <p v-if="saveError" class="node-edit__error" role="alert">{{ saveError }}</p>
    </form>
    <template #footer="{ close }">
      <UiButton :disabled="saving" @click="close">{{ t('common.actions.cancel') }}</UiButton>
      <UiButton variant="primary" :loading="saving" :disabled="!rows.length" data-test="save-node-edit" @click="save">{{ t('adminSubscriptionGroups.nodeEdit.save') }}</UiButton>
    </template>
  </UiSheet>
</template>

<script setup>
// 编辑节点: the group's own edits of the nodes one protocol produces (one
// per entry). The source protocol is never changed; an empty field keeps the
// source value and "重置此节点" drops the node's edits. Saves
// PUT /api/v4/admin/subscription-groups/:id/protocols/:protocolId/nodes.
import { ref, watch } from 'vue'
import { getGroupProtocolNodes, saveGroupProtocolNodes } from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import { UiBadge, UiButton, UiSheet, UiTextField, UiTextarea, useToast } from '@/ui'

// The fields with a form input; any other edited field lives in the JSON.
const FORM_FIELDS = ['name', 'server', 'port', 'server_name']

const props = defineProps({
  open: { type: Boolean, default: false },
  group: { type: Object, required: true },
  protocol: { type: Object, default: null }
})
const emit = defineEmits(['update:open', 'saved'])
const { t } = useAppI18n()
const toast = useToast()

const rows = ref([])
const loading = ref(false)
const saving = ref(false)
const saveError = ref('')

watch(() => [props.open, props.protocol?.id], async ([open]) => {
  if (!open || !props.protocol) return
  loading.value = true
  saveError.value = ''
  rows.value = []
  try {
    const answer = await getGroupProtocolNodes(props.group.id, props.protocol.id)
    rows.value = (answer?.nodes || []).map(toRow)
  } catch (error) {
    saveError.value = messageOf(error)
  } finally {
    loading.value = false
  }
}, { immediate: true })

function toRow(node) {
  const overrides = node.overrides || {}
  const fields = {}
  for (const name of FORM_FIELDS) fields[name] = overrides[name] == null ? '' : String(overrides[name])
  const rest = Object.fromEntries(Object.entries(overrides).filter(([name]) => !FORM_FIELDS.includes(name)))
  return {
    key: node.key,
    title: node.name || props.protocol?.name || node.key,
    defaults: node.defaults || {},
    fields,
    advanced: Object.keys(rest).length ? JSON.stringify(rest, null, 2) : ''
  }
}

function show(row, name) {
  const value = row.defaults[name]
  return value == null ? '' : String(value)
}

function hint(row, name) {
  const value = show(row, name)
  return value ? t('adminSubscriptionGroups.nodeEdit.default', { value }) : ''
}

function portError(row) {
  const value = String(row.fields.port || '').trim()
  if (!value) return ''
  const port = Number(value)
  return Number.isInteger(port) && port >= 1 && port <= 65535 ? '' : t('adminSubscriptionGroups.nodeEdit.badPort')
}

function parseAdvanced(row) {
  const text = row.advanced.trim()
  if (!text) return {}
  const parsed = JSON.parse(text)
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) throw new Error('not an object')
  return parsed
}

function advancedError(row) {
  try {
    parseAdvanced(row)
    return ''
  } catch {
    return t('adminSubscriptionGroups.nodeEdit.badJson')
  }
}

// The row's edits as the API takes them: form fields set, plus the JSON.
function overridesOf(row) {
  const edits = { ...parseAdvanced(row) }
  for (const name of FORM_FIELDS) {
    const value = String(row.fields[name] ?? '').trim()
    if (value === '') continue
    edits[name] = name === 'port' ? Number(value) : value
  }
  return edits
}

function isEdited(row) {
  try {
    return Object.keys(overridesOf(row)).length > 0
  } catch {
    return true
  }
}

function resetRow(row) {
  for (const name of FORM_FIELDS) row.fields[name] = ''
  row.advanced = ''
}

function messageOf(error) {
  const answer = error?.response?.data
  return answer?.error?.message || answer?.message || error?.message || t('adminSubscriptionGroups.nodeEdit.saveFailed')
}

async function save() {
  if (saving.value || rows.value.some(row => portError(row) || advancedError(row))) return
  saving.value = true
  saveError.value = ''
  try {
    const nodes = {}
    for (const row of rows.value) nodes[row.key] = overridesOf(row)
    await saveGroupProtocolNodes(props.group.id, props.protocol.id, nodes)
    toast.success(t('adminSubscriptionGroups.nodeEdit.saved'))
    emit('saved')
    emit('update:open', false)
  } catch (error) {
    saveError.value = messageOf(error)
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.node-edit {
  display: grid;
  gap: var(--space-4);
}

.node-edit__item {
  display: grid;
  gap: var(--space-2);
  padding: var(--space-3);
  border: 1px solid var(--separator);
  border-radius: var(--radius-md);
  min-width: 0;
}

.node-edit__head {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
  justify-content: space-between;
  overflow-wrap: anywhere;
}

.node-edit__note {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.node-edit__error {
  margin: 0;
  color: var(--red, var(--danger));
}
</style>
