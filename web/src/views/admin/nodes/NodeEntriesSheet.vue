<template>
  <UiSheet
    :open="open"
    size="md"
    :title="t('nodeEntries.sheet.title', { name: node.name })"
    :description="t('nodeEntries.sheet.description')"
    :dismissible="!saving"
    @update:open="value => emit('update:open', value)"
  >
    <form class="node-entries-sheet" novalidate data-testid="node-entries-form" @submit.prevent="save">
      <p v-if="!forms.length" class="node-entries-sheet__empty">{{ t('nodeEntries.sheet.empty', { host: node.host || '—' }) }}</p>
      <fieldset v-for="(form, index) in forms" :key="form.key" class="node-entries-sheet__entry" :data-testid="`node-entry-${index}`">
        <legend class="node-entries-sheet__legend">
          <span>{{ form.label.trim() || t('nodeEntries.sheet.entryN', { n: index + 1 }) }}</span>
          <UiIconButton :icon="Trash2" size="sm" :label="t('nodeEntries.sheet.remove')" @click="remove(index)" />
        </legend>
        <UiTextField v-model="form.label" size="md" :label="t('nodeEntries.sheet.label')" required :placeholder="t('nodeEntries.sheet.labelPlaceholder')" :help="t('nodeEntries.sheet.labelHelp')" :error="errors[form.key]?.label" />
        <UiSelect v-model="form.kind" size="md" :label="t('nodeEntries.sheet.kind')" :options="kindOptions" :help="t(`nodeEntries.sheet.kindHelp.${form.kind}`)" />
        <UiSelect
          v-if="form.kind === 'iwan'"
          v-model="form.lineId"
          size="md"
          :label="t('nodeEntries.sheet.line')"
          required
          :options="lineOptions"
          :placeholder="lines.length ? t('nodeEntries.sheet.linePlaceholder') : t('nodeEntries.sheet.noLines')"
          :help="t('nodeEntries.sheet.lineHelp')"
          :error="errors[form.key]?.line"
        />
        <UiTextField
          v-model="form.host"
          size="md"
          :label="t('nodeEntries.sheet.host')"
          required
          :placeholder="form.kind === 'nat' ? '203.0.113.7' : form.kind === 'iwan' ? (node.host || '198.51.100.20') : '2001:db8::10'"
          :help="form.kind === 'iwan' ? t('nodeEntries.sheet.iwanHostHelp') : ''"
          :error="errors[form.key]?.host"
        />
        <UiTextarea
          v-if="form.kind === 'nat'"
          v-model="form.portMap"
          :rows="3"
          :label="t('nodeEntries.sheet.portMap')"
          required
          :placeholder="'21000-21099 -> 30000-30099\n20443 -> 443'"
          :help="t('nodeEntries.sheet.portMapHelp')"
          :error="errors[form.key]?.portMap"
        />
        <UiSwitch v-model="form.enabled" :label="t('nodeEntries.sheet.enabled')" />
      </fieldset>
      <UiButton size="sm" variant="ghost" :icon="Plus" :disabled="forms.length >= MAX_ENTRIES" data-testid="add-node-entry" @click="add">{{ t('nodeEntries.add') }}</UiButton>
      <p v-if="saveError" class="node-entries-sheet__error" role="alert">{{ saveError }}</p>
    </form>
    <template #footer="{ close }">
      <UiButton :disabled="saving" @click="close">{{ t('nodeEntries.sheet.cancel') }}</UiButton>
      <UiButton variant="primary" :loading="saving" data-testid="node-entries-save" @click="save">{{ t('nodeEntries.sheet.save') }}</UiButton>
    </template>
  </UiSheet>
</template>

<script setup>
// The 入口 sheet: edits the whole list of a node's entries and replaces it
// in one request (PUT /api/v4/kernel/nodes/proxy-<id>/entries).
import { computed, ref, watch } from 'vue'
import { Plus, Trash2 } from '@lucide/vue'
import { UiButton, UiIconButton, UiSelect, UiSheet, UiSwitch, UiTextarea, UiTextField, useToast } from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import { replaceKernelNodeEntries } from '@/api/kernel'
import { entriesRequest, entryForm, validateEntries } from './nodeEntriesModel'

const MAX_ENTRIES = 32

const props = defineProps({
  open: { type: Boolean, default: false },
  node: { type: Object, required: true },
  entries: { type: Array, default: () => [] },
  // Panabit iWAN lines ({ id, name }) an iwan entry can name.
  lines: { type: Array, default: () => [] }
})
const emit = defineEmits(['update:open', 'saved'])
const { t } = useAppI18n()
const toast = useToast()

const forms = ref([])
const errors = ref({})
const saving = ref(false)
const saveError = ref('')

watch(() => props.open, value => {
  if (!value) return
  forms.value = props.entries.map(entry => entryForm(entry))
  errors.value = {}
  saveError.value = ''
}, { immediate: true })

const kindOptions = computed(() => [
  { value: 'direct', label: t('nodeEntries.kinds.direct') },
  { value: 'nat', label: t('nodeEntries.kinds.nat') },
  { value: 'iwan', label: t('nodeEntries.kinds.iwan') }
])

const lineOptions = computed(() => props.lines.map(line => ({ value: line.id, label: line.name })))

function add() {
  forms.value.push(entryForm(null))
}

function remove(index) {
  forms.value.splice(index, 1)
}

async function save() {
  if (saving.value) return
  errors.value = validateEntries(forms.value, {
    required: t('nodeEntries.sheet.required'),
    noPipe: t('nodeEntries.sheet.noPipe'),
    duplicate: t('nodeEntries.sheet.duplicate'),
    badLine: line => t('nodeEntries.sheet.badLine', { line })
  })
  if (Object.keys(errors.value).length) return
  saving.value = true
  saveError.value = ''
  try {
    const answer = await replaceKernelNodeEntries(`proxy-${props.node.id}`, entriesRequest(forms.value))
    toast.success(t('nodeEntries.saved', { name: props.node.name }))
    emit('saved', answer || null)
    emit('update:open', false)
  } catch (error) {
    const answer = error?.response?.data
    saveError.value = answer?.error?.message || answer?.message || error?.message || String(error)
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.node-entries-sheet {
  display: grid;
  gap: var(--space-4);
  justify-items: stretch;
}

.node-entries-sheet > .ui-button {
  justify-self: start;
}

.node-entries-sheet__entry {
  display: grid;
  gap: var(--space-3);
  margin: 0;
  padding: var(--space-3) var(--space-4) var(--space-4);
  border: 1px solid var(--separator);
  border-radius: var(--radius-md);
  min-width: 0;
}

.node-entries-sheet__legend {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: 0 var(--space-1);
  font-weight: var(--weight-semibold);
}

.node-entries-sheet__empty {
  margin: 0;
  color: var(--label-2);
}

.node-entries-sheet__error {
  margin: 0;
  color: var(--danger);
}
</style>
