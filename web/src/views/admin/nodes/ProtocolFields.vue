<template>
  <div class="protocol-fields" data-testid="protocol-fields">
    <section v-for="group in groups" :key="group.name" class="dialog-section" :data-testid="`protocol-fields-${group.name}`">
      <h3 class="dialog-section__title">{{ t(`${K}.groups.${group.name}`) }}</h3>
      <div class="form-grid">
        <template v-for="field in group.fields" :key="`${field.section}.${field.key}`">
          <UiSelect
            v-if="field.kind === 'select'"
            :model-value="selectValue(field)"
            size="md"
            :label="labelOf(field)"
            :options="optionsOf(field)"
            :data-testid="`protocol-field-${field.key}`"
            @update:model-value="value => update(field, value === NONE ? '' : value)"
          />
          <UiNumberField
            v-else-if="field.kind === 'number'"
            :model-value="readField(modelValue, field)"
            size="md"
            :min="field.min"
            :max="field.max"
            :label="labelOf(field)"
            :data-testid="`protocol-field-${field.key}`"
            @update:model-value="value => update(field, value)"
          />
          <UiSwitch
            v-else-if="field.kind === 'switch'"
            :model-value="Boolean(readField(modelValue, field))"
            class="form-grid__full"
            :label="labelOf(field)"
            :data-testid="`protocol-field-${field.key}`"
            @update:model-value="value => update(field, value)"
          />
          <UiTextField
            v-else-if="field.kind === 'list'"
            :model-value="listText(field)"
            size="md"
            :label="labelOf(field)"
            :help="helpOf(field)"
            :placeholder="field.placeholder || ''"
            :data-testid="`protocol-field-${field.key}`"
            @update:model-value="value => update(field, splitList(value))"
          />
          <UiPasswordField
            v-else-if="field.kind === 'password'"
            :model-value="String(readField(modelValue, field))"
            size="md"
            autocomplete="off"
            :label="labelOf(field)"
            :help="helpOf(field)"
            :data-testid="`protocol-field-${field.key}`"
            @update:model-value="value => update(field, String(value).trim())"
          />
          <div v-else-if="field.action === 'shortId'" class="protocol-fields__with-action">
            <UiTextField
              :model-value="String(readField(modelValue, field))"
              size="md"
              :label="labelOf(field)"
              :data-testid="`protocol-field-${field.key}`"
              @update:model-value="value => update(field, String(value).trim())"
            />
            <UiButton size="md" :icon="Dices" :data-testid="`protocol-generate-${field.key}`" @click="update(field, randomShortId())">{{ t(`${K}.generate`) }}</UiButton>
          </div>
          <UiTextField
            v-else
            :model-value="String(readField(modelValue, field))"
            size="md"
            :label="labelOf(field)"
            :help="helpOf(field)"
            :placeholder="field.placeholder || ''"
            :data-testid="`protocol-field-${field.key}`"
            @update:model-value="value => update(field, String(value).trim())"
          />
        </template>
      </div>
    </section>
  </div>
</template>

<script setup>
// The 3x-ui style fields of a protocol: shows the fields registered for the
// protocol's type, TLS mode and transport (protocolFields.js) and edits them
// in place. v-model is the protocol with its four section objects parsed
// ({ type, tls, transport, settings, tls_settings, reality_settings,
// transport_settings }); other keys of the sections are left alone.
// Reusable wherever a protocol is edited (node, node group template,
// subscription group).
import { computed } from 'vue'
import { Dices } from '@lucide/vue'
import UiButton from '@/ui/UiButton.vue'
import UiNumberField from '@/ui/UiNumberField.vue'
import UiPasswordField from '@/ui/UiPasswordField.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiSwitch from '@/ui/UiSwitch.vue'
import UiTextField from '@/ui/UiTextField.vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { FIELD_GROUPS, PROTOCOL_SECTIONS, fieldsFor, randomShortId, readField, writeField } from './protocolFields'

const props = defineProps({
  modelValue: { type: Object, required: true },
  // Limit the form to some sections (a node group template keeps its own
  // Reality and TLS fields).
  sections: { type: Array, default: () => PROTOCOL_SECTIONS }
})
const emit = defineEmits(['update:modelValue'])
const { t } = useAppI18n()
const K = 'admin.nodes.protocolFields'
// Select items cannot carry an empty value: '' travels as this.
const NONE = '__none__'

const GROUP_ORDER = ['protocol', 'transport', 'security']
const groups = computed(() => {
  const fields = fieldsFor(props.modelValue).filter(field => props.sections.includes(field.section))
  return GROUP_ORDER
    .map(name => ({ name, fields: fields.filter(field => FIELD_GROUPS[field.section] === name) }))
    .filter(group => group.fields.length)
})

const i18nKey = field => field.key.replace(/-/g, '_')
const labelOf = field => t(`${K}.labels.${i18nKey(field)}`)
const helpOf = field => (field.hintKey ? t(`${K}.hints.${field.hintKey}`) : '')
const selectValue = (field) => {
  const value = readField(props.modelValue, field)
  return value === '' ? NONE : value
}
const splitList = text => String(text || '').split(/[,\s]+/).filter(Boolean)
const listText = field => [].concat(readField(props.modelValue, field)).join(',')
const optionsOf = field => field.options.map(value => ({
  value: value === '' ? NONE : value,
  label: value === '' ? t(`${K}.none`) : value
}))

function update(field, value) {
  emit('update:modelValue', writeField(props.modelValue, field, value))
}
</script>

<style scoped>
.protocol-fields {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
}

.protocol-fields__with-action {
  display: flex;
  align-items: flex-end;
  gap: var(--space-2);
}

.protocol-fields__with-action > :first-child {
  flex: 1 1 auto;
}
</style>
