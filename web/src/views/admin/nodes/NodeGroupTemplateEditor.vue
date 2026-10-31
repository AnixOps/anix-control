<template>
  <div class="template-editor" data-testid="template-editor">
    <p v-if="modelValue.length === 0" class="template-editor__empty" data-testid="template-empty">{{ t('admin.nodes.groupForm.template.empty') }}</p>

    <fieldset v-for="(protocol, index) in modelValue" :key="protocol.key" class="template-card" :data-testid="`template-protocol-${index}`">
      <legend class="template-card__legend">
        {{ t('admin.nodes.groupForm.template.protocol', { n: index + 1 }) }}
      </legend>
      <div class="form-grid">
        <UiTextField
          :model-value="protocol.name"
          size="md"
          :label="t('admin.nodes.groupForm.template.name')"
          :placeholder="t('admin.nodes.groupForm.template.namePlaceholder')"
          @update:model-value="value => patch(index, { name: value })"
        />
        <UiSelect
          :model-value="protocol.type"
          size="md"
          :label="t('admin.nodes.groupForm.template.type')"
          :options="typeOptions"
          @update:model-value="value => patch(index, { type: value }, true)"
        />
        <UiSelect
          :model-value="protocol.transport"
          size="md"
          :label="t('admin.nodes.groupForm.template.transport')"
          :options="transportOptions(protocol)"
          @update:model-value="value => patch(index, { transport: value }, true)"
        />
        <UiSelect
          :model-value="protocol.tls"
          size="md"
          :label="t('admin.nodes.groupForm.template.tls')"
          :options="tlsOptions(protocol)"
          @update:model-value="value => patch(index, { tls: value }, true)"
        />
        <UiSegmentedControl
          class="form-grid__full"
          :model-value="protocol.portMode"
          :aria-label="t('admin.nodes.groupForm.template.portMode')"
          :options="portModeOptions"
          @update:model-value="value => patch(index, { portMode: value })"
        />
        <UiNumberField
          v-if="protocol.portMode === 'fixed'"
          :model-value="Number(protocol.port)"
          size="md"
          :min="1"
          :max="65535"
          :label="t('admin.nodes.groupForm.template.port')"
          :error="portError(protocol)"
          @update:model-value="value => patch(index, { port: value })"
        />
        <template v-else>
          <UiNumberField
            :model-value="Number(protocol.portFrom)"
            size="md"
            :min="1"
            :max="65535"
            :label="t('admin.nodes.groupForm.template.portFrom')"
            :error="portError(protocol)"
            @update:model-value="value => patch(index, { portFrom: value })"
          />
          <UiNumberField
            :model-value="Number(protocol.portTo)"
            size="md"
            :min="1"
            :max="65535"
            :label="t('admin.nodes.groupForm.template.portTo')"
            @update:model-value="value => patch(index, { portTo: value })"
          />
        </template>
        <template v-if="protocol.tls === 'reality'">
          <UiTextField
            :model-value="protocol.serverName"
            size="md"
            required
            :label="t('admin.nodes.groupForm.template.serverName')"
            :help="t('admin.nodes.groupForm.template.serverNameHelp')"
            :error="problemsOf(protocol).serverName ? t('admin.nodes.form.required') : ''"
            @update:model-value="value => patch(index, { serverName: value })"
          />
          <UiTextField
            :model-value="protocol.dest"
            size="md"
            :label="t('admin.nodes.groupForm.template.dest')"
            :placeholder="`${protocol.serverName || 'example.com'}:443`"
            :help="t('admin.nodes.groupForm.template.destHelp')"
            @update:model-value="value => patch(index, { dest: value })"
          />
        </template>
      </div>
      <ProtocolFields
        v-if="protocol.type !== 'wireguard'"
        :model-value="fieldModel(protocol)"
        :sections="['settings', 'transport_settings']"
        :data-testid="`template-fields-${index}`"
        @update:model-value="value => patch(index, fieldChanges(value))"
      />
      <details class="template-card__advanced">
        <summary>{{ t('admin.nodes.groupForm.template.advanced') }}</summary>
        <UiTextarea
          :model-value="protocol.settingsText"
          :label="t('admin.nodes.groupForm.template.settings')"
          :rows="3"
          :error="jsonError(problemsOf(protocol).settings)"
          placeholder="{}"
          spellcheck="false"
          @update:model-value="value => patch(index, { settingsText: value })"
        />
        <UiTextarea
          :model-value="protocol.transportSettingsText"
          :label="t('admin.nodes.groupForm.template.transportSettings')"
          :rows="3"
          :error="jsonError(problemsOf(protocol).transportSettings)"
          placeholder="{}"
          spellcheck="false"
          @update:model-value="value => patch(index, { transportSettingsText: value })"
        />
      </details>
      <div class="template-card__actions">
        <UiButton size="sm" variant="danger-soft" :icon="Trash2" data-testid="template-remove" @click="remove(index)">
          {{ t('admin.nodes.groupForm.template.remove') }}
        </UiButton>
      </div>
    </fieldset>

    <div class="template-editor__add">
      <UiButton :icon="Plus" :disabled="modelValue.length >= MAX_PROTOCOLS" data-testid="template-add" @click="add">
        {{ t('admin.nodes.groupForm.template.add') }}
      </UiButton>
    </div>
  </div>
</template>

<script setup>
// The protocols every new node of a proxy group gets: type, name, transport,
// TLS (none, or Reality for VLESS), a fixed port or a range (a random port
// per node), and the Reality site. Settings stay a JSON object under
// "高级". The server validates it all again and its errors show in the sheet.
import { Plus, Trash2 } from '@lucide/vue'
import { computed } from 'vue'
import UiButton from '@/ui/UiButton.vue'
import UiNumberField from '@/ui/UiNumberField.vue'
import UiSegmentedControl from '@/ui/UiSegmentedControl.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiTextarea from '@/ui/UiTextarea.vue'
import UiTextField from '@/ui/UiTextField.vue'
import ProtocolFields from './ProtocolFields.vue'
import { useAppI18n } from '@/composables/useAppI18n'
import {
  REALITY_TRANSPORTS, TEMPLATE_TRANSPORTS, TEMPLATE_TYPES, adaptProtocol, blankProtocol, protocolProblems
} from './nodeGroupData'

const MAX_PROTOCOLS = 16

const props = defineProps({
  modelValue: { type: Array, default: () => [] },
  // Show what is wrong with each protocol (after a failed save).
  validate: { type: Boolean, default: false }
})
const emit = defineEmits(['update:modelValue'])
const { t } = useAppI18n()

const typeOptions = computed(() => TEMPLATE_TYPES.map(value => ({ value, label: t(`admin.nodes.groupForm.template.types.${value}`) })))
const portModeOptions = computed(() => ['range', 'fixed'].map(value => ({ value, label: t(`admin.nodes.groupForm.template.portModes.${value}`) })))

function transportOptions(protocol) {
  const allowed = TEMPLATE_TRANSPORTS[protocol.type] || []
  const usable = protocol.tls === 'reality' ? allowed.filter(value => REALITY_TRANSPORTS.includes(value)) : allowed
  return usable.map(value => ({ value, label: value }))
}

function tlsOptions(protocol) {
  const options = [{ value: 'none', label: t('admin.nodes.tlsModes.none') }]
  if (protocol.type === 'vless') options.push({ value: 'reality', label: t('admin.nodes.tlsModes.reality') })
  return options
}

function problemsOf(protocol) {
  return props.validate ? protocolProblems(protocol) : { port: '', serverName: '', settings: '', transportSettings: '' }
}

function portError(protocol) {
  const problem = problemsOf(protocol).port
  return problem ? t(`admin.nodes.groupForm.template.portErrors.${problem}`) : ''
}

function jsonError(problem) {
  return problem ? t(`admin.nodes.groupForm.template.jsonErrors.${problem}`) : ''
}

// ProtocolFields edits the parsed settings and transport settings of a
// template protocol; the JSON under 高级 stays the same text.
function parseObject(text) {
  try {
    const value = JSON.parse(text || '{}')
    return value && typeof value === 'object' && !Array.isArray(value) ? value : {}
  } catch {
    return {}
  }
}
function fieldModel(protocol) {
  return {
    type: protocol.type,
    tls: protocol.tls === 'reality' ? 2 : 0,
    transport: protocol.transport,
    settings: parseObject(protocol.settingsText),
    transport_settings: parseObject(protocol.transportSettingsText)
  }
}
const textOf = value => (Object.keys(value || {}).length ? JSON.stringify(value) : '')
function fieldChanges(value) {
  return { settingsText: textOf(value.settings), transportSettingsText: textOf(value.transport_settings) }
}

// `adapt` re-reads the pieces that depend on each other (the transports of a
// type, Reality being VLESS only).
function patch(index, changes, adapt = false) {
  const next = props.modelValue.map((protocol, i) => {
    if (i !== index) return protocol
    const updated = { ...protocol, ...changes }
    if (changes.type) {
      // A new type starts from its own defaults: settings, TLS and transport.
      const fresh = blankProtocol(changes.type)
      updated.settingsText = fresh.settingsText
      updated.transportSettingsText = ''
      updated.tls = fresh.tls
      updated.transport = fresh.transport
    }
    return adapt ? adaptProtocol(updated) : updated
  })
  emit('update:modelValue', next)
}

function add() {
  emit('update:modelValue', [...props.modelValue, blankProtocol(props.modelValue.length ? 'shadowsocks' : 'vless')])
}

function remove(index) {
  emit('update:modelValue', props.modelValue.filter((_, i) => i !== index))
}
</script>

<style scoped>
.template-editor {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.template-editor__empty {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.template-card {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  margin: 0;
  padding: var(--space-4);
  border: 1px solid var(--separator);
  border-radius: var(--radius-md);
}

.template-card__legend {
  padding: 0 var(--space-2);
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

.template-card__advanced summary {
  cursor: pointer;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.template-card__advanced {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.template-card__actions {
  display: flex;
  justify-content: flex-end;
}
</style>
