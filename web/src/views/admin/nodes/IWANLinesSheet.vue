<template>
  <UiSheet
    :open="open"
    size="md"
    :title="t('nodeEntries.lines.title')"
    :description="t('nodeEntries.lines.description')"
    :dismissible="!saving"
    @update:open="value => emit('update:open', value)"
  >
    <div class="iwan-lines-sheet" data-testid="iwan-lines">
      <p v-if="!lines.length && !form" class="iwan-lines-sheet__empty">{{ t('nodeEntries.lines.empty') }}</p>
      <ul v-if="lines.length" class="iwan-lines-sheet__list">
        <li v-for="line in lines" :key="line.id" class="iwan-lines-sheet__line" :data-testid="`iwan-line-${line.id}`">
          <div class="iwan-lines-sheet__info">
            <span class="iwan-lines-sheet__name">{{ line.name }}</span>
            <span class="iwan-lines-sheet__address">{{ address(line) }}</span>
            <span class="iwan-lines-sheet__used">{{ t('nodeEntries.lines.used', { n: line.entries || 0 }) }}</span>
          </div>
          <UiIconButton :icon="Pencil" size="sm" :label="t('nodeEntries.lines.edit', { name: line.name })" @click="edit(line)" />
          <UiIconButton :icon="Trash2" size="sm" :label="t('nodeEntries.lines.remove', { name: line.name })" :disabled="line.entries > 0" @click="remove(line)" />
        </li>
      </ul>

      <form v-if="form" class="iwan-lines-sheet__form" novalidate data-testid="iwan-line-form" @submit.prevent="save">
        <UiTextField v-model="form.name" size="md" :label="t('nodeEntries.lines.name')" required placeholder="HK iWAN" :help="t('nodeEntries.lines.nameHelp')" :error="errors.name" />
        <UiTextField v-model="form.host" size="md" :label="t('nodeEntries.lines.host')" required placeholder="203.0.113.10" :error="errors.host" />
        <UiNumberField v-model="form.port" size="md" :label="t('nodeEntries.lines.port')" required :min="1" :max="65535" :error="errors.port" />
        <UiTextField v-model="form.username" size="md" :label="t('nodeEntries.lines.username')" required autocomplete="off" :error="errors.username" />
        <UiPasswordField
          v-model="form.password"
          size="md"
          :label="t('nodeEntries.lines.password')"
          :required="form.id === null"
          autocomplete="new-password"
          :help="form.id === null ? '' : t('nodeEntries.lines.passwordKeep')"
          :error="errors.password"
        />
        <UiNumberField v-model="form.mtu" size="md" :label="t('nodeEntries.lines.mtu')" :min="0" :max="1600" placeholder="1400" :help="t('nodeEntries.lines.mtuHelp')" :error="errors.mtu" />
        <UiSwitch v-model="form.encrypt" :label="t('nodeEntries.lines.encrypt')" :description="t('nodeEntries.lines.encryptHelp')" />
        <p v-if="saveError" class="iwan-lines-sheet__error" role="alert">{{ saveError }}</p>
      </form>
      <UiButton v-else size="sm" variant="ghost" :icon="Plus" data-testid="add-iwan-line" @click="edit(null)">{{ t('nodeEntries.lines.add') }}</UiButton>
    </div>
    <template #footer="{ close }">
      <template v-if="form">
        <UiButton :disabled="saving" @click="form = null">{{ t('nodeEntries.sheet.cancel') }}</UiButton>
        <UiButton variant="primary" :loading="saving" data-testid="iwan-line-save" @click="save">{{ t('nodeEntries.sheet.save') }}</UiButton>
      </template>
      <UiButton v-else @click="close">{{ t('nodeEntries.lines.close') }}</UiButton>
    </template>
  </UiSheet>
</template>

<script setup>
// The Panabit iWAN lines (GET/POST/PUT/DELETE /api/v4/kernel/iwan-lines):
// third-party iWAN ingresses AnixOps Net logs into with each line's shared
// account. A node's iwan entry reaches the node through one of them.
import { ref, watch } from 'vue'
import { Pencil, Plus, Trash2 } from '@lucide/vue'
import { UiButton, UiIconButton, UiNumberField, UiPasswordField, UiSheet, UiSwitch, UiTextField, useConfirm, useToast } from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import { createIWANLine, deleteIWANLine, updateIWANLine } from '@/api/kernel'
import { lineForm, lineRequest, validateLine } from './nodeEntriesModel'

defineProps({
  open: { type: Boolean, default: false },
  lines: { type: Array, default: () => [] }
})
const emit = defineEmits(['update:open', 'changed'])
const { t } = useAppI18n()
const toast = useToast()
const confirm = useConfirm()

const form = ref(null)
const errors = ref({})
const saving = ref(false)
const saveError = ref('')

watch(() => form.value, () => {
  errors.value = {}
  saveError.value = ''
})

function address(line) {
  const host = line.host.includes(':') ? `[${line.host}]` : line.host
  return `${host}:${line.port}`
}

function edit(line) {
  form.value = lineForm(line)
}

function failure(error) {
  const answer = error?.response?.data
  return answer?.error?.message || answer?.message || error?.message || String(error)
}

async function save() {
  if (saving.value || !form.value) return
  errors.value = validateLine(form.value, {
    required: t('nodeEntries.sheet.required'),
    noPipe: t('nodeEntries.sheet.noPipe'),
    port: t('nodeEntries.lines.badPort'),
    mtu: t('nodeEntries.lines.badMtu')
  })
  if (Object.keys(errors.value).length) return
  saving.value = true
  saveError.value = ''
  try {
    const request = lineRequest(form.value)
    if (form.value.id === null) await createIWANLine(request)
    else await updateIWANLine(form.value.id, request)
    toast.success(t('nodeEntries.lines.saved', { name: request.name }))
    form.value = null
    emit('changed')
  } catch (error) {
    saveError.value = failure(error)
  } finally {
    saving.value = false
  }
}

async function remove(line) {
  await confirm({
    title: t('nodeEntries.lines.removeTitle', { name: line.name }),
    message: t('nodeEntries.lines.removeMessage'),
    confirmLabel: t('nodeEntries.lines.removeAction'),
    tone: 'danger',
    onConfirm: async () => {
      await deleteIWANLine(line.id)
      emit('changed')
    }
  })
}
</script>

<style scoped>
.iwan-lines-sheet {
  display: grid;
  gap: var(--space-4);
  justify-items: stretch;
}

.iwan-lines-sheet > .ui-button {
  justify-self: start;
}

.iwan-lines-sheet__list {
  display: grid;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}

.iwan-lines-sheet__line {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-4);
  border: 1px solid var(--separator);
  border-radius: var(--radius-md);
}

.iwan-lines-sheet__info {
  display: grid;
  flex: 1;
  min-width: 0;
}

.iwan-lines-sheet__name {
  font-weight: var(--weight-semibold);
  overflow-wrap: anywhere;
}

.iwan-lines-sheet__address {
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
  overflow-wrap: anywhere;
}

.iwan-lines-sheet__used {
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.iwan-lines-sheet__form {
  display: grid;
  gap: var(--space-3);
}

.iwan-lines-sheet__empty {
  margin: 0;
  color: var(--label-2);
}

.iwan-lines-sheet__error {
  margin: 0;
  color: var(--danger);
}
</style>
