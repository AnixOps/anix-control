<template>
  <div class="schema-field">
    <template v-if="fieldKind === 'enum'">
      <label :for="fieldID">{{ name }}<span v-if="required" class="required"> *</span></label>
      <select :id="fieldID" :value="enumIndex" @change="updateEnum">
        <option value="">{{ t('control.config.selectValue') }}</option>
        <option v-for="(option, index) in schema.enum" :key="`${path}-option-${index}`" :value="String(index)">
          {{ enumLabel(option) }}
        </option>
      </select>
      <p v-if="schema.description" class="field-help">{{ schema.description }}</p>
    </template>

    <template v-else-if="fieldKind === 'string'">
      <label :for="fieldID">{{ name }}<span v-if="required" class="required"> *</span></label>
      <input
        :id="fieldID"
        :value="modelValue ?? ''"
        type="text"
        :placeholder="schema.default === undefined ? '' : String(schema.default)"
        @input="emit('update:modelValue', $event.target.value)"
      />
      <p v-if="schema.description" class="field-help">{{ schema.description }}</p>
    </template>

    <template v-else-if="fieldKind === 'number' || fieldKind === 'integer'">
      <label :for="fieldID">{{ name }}<span v-if="required" class="required"> *</span></label>
      <input
        :id="fieldID"
        :value="modelValue ?? ''"
        type="number"
        :step="fieldKind === 'integer' ? 1 : 'any'"
        :min="schema.minimum"
        :max="schema.maximum"
        @input="updateNumber"
      />
      <p v-if="schema.description" class="field-help">{{ schema.description }}</p>
    </template>

    <template v-else-if="fieldKind === 'boolean'">
      <label class="boolean-field" :for="fieldID">
        <input :id="fieldID" type="checkbox" :checked="modelValue === true" @change="emit('update:modelValue', $event.target.checked)" />
        <span>{{ name }}<span v-if="required" class="required"> *</span></span>
      </label>
      <p v-if="schema.description" class="field-help">{{ schema.description }}</p>
    </template>

    <fieldset v-else-if="fieldKind === 'object'" class="schema-group">
      <legend>{{ name }}<span v-if="required" class="required"> *</span></legend>
      <p v-if="schema.description" class="field-help">{{ schema.description }}</p>
      <div class="schema-children">
        <PluginSchemaField
          v-for="(childSchema, key) in schema.properties"
          :key="`${path}.${key}`"
          :schema="childSchema"
          :model-value="objectValue[key]"
          :name="childSchema.title || key"
          :path="`${path}.${key}`"
          :required="requiredFields.has(key)"
          @update:model-value="updateObject(key, $event)"
          @validity="emit('validity', $event)"
        />
      </div>
    </fieldset>

    <fieldset v-else-if="fieldKind === 'array'" class="schema-group">
      <div class="array-heading">
        <span class="group-label">{{ name }}<span v-if="required" class="required"> *</span></span>
        <button class="btn btn-sm icon-button" type="button" :title="t('control.config.addItem')" :aria-label="t('control.config.addItem')" @click="addArrayItem">+</button>
      </div>
      <p v-if="schema.description" class="field-help">{{ schema.description }}</p>
      <p v-if="arrayValue.length === 0" class="array-empty">{{ t('control.config.emptyArray') }}</p>
      <div v-for="(item, index) in arrayValue" :key="`${path}.${index}`" class="array-item">
        <PluginSchemaField
          :schema="schema.items"
          :model-value="item"
          :name="`${t('control.config.item')} ${index + 1}`"
          :path="`${path}.${index}`"
          required
          @update:model-value="updateArrayItem(index, $event)"
          @validity="emit('validity', $event)"
        />
        <button class="btn btn-sm btn-danger icon-button remove-item" type="button" :title="t('control.config.removeItem')" :aria-label="t('control.config.removeItem')" @click="removeArrayItem(index)">x</button>
      </div>
    </fieldset>

    <template v-else>
      <label :for="fieldID">{{ name }}<span v-if="required" class="required"> *</span></label>
      <textarea :id="fieldID" v-model="jsonText" rows="6" spellcheck="false" class="json-editor" @input="updateJSON"></textarea>
      <p v-if="schema.description" class="field-help">{{ schema.description }}</p>
      <p v-if="jsonError" class="field-error" role="alert">{{ jsonError }}</p>
    </template>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'

defineOptions({ name: 'PluginSchemaField' })

const props = defineProps({
  schema: { type: Object, default: () => ({}) },
  modelValue: { default: undefined },
  name: { type: String, required: true },
  path: { type: String, required: true },
  required: { type: Boolean, default: false }
})
const emit = defineEmits(['update:modelValue', 'validity'])
const { t } = useAppI18n()
const jsonText = ref('')
const jsonError = ref('')

const complexKeywords = ['$ref', 'const', 'allOf', 'anyOf', 'oneOf', 'not', 'if', 'then', 'else', 'prefixItems', 'patternProperties', 'dependentSchemas']
const hasComplexKeywords = computed(() => complexKeywords.some(key => props.schema[key] !== undefined))
const inferredType = computed(() => props.schema.type || (props.schema.properties ? 'object' : props.schema.items ? 'array' : 'string'))
const fieldKind = computed(() => {
  if (Array.isArray(props.schema.enum) && props.schema.enum.length > 0) return 'enum'
  if (hasComplexKeywords.value || Array.isArray(props.schema.type)) return 'json'
  const type = inferredType.value
  if (type === 'object') {
    return props.schema.properties && typeof props.schema.properties === 'object' && !Array.isArray(props.schema.properties) ? 'object' : 'json'
  }
  if (type === 'array') {
    return props.schema.items && typeof props.schema.items === 'object' && !Array.isArray(props.schema.items) ? 'array' : 'json'
  }
  return ['string', 'number', 'integer', 'boolean'].includes(type) ? type : 'json'
})
const fieldID = computed(() => `plugin-config-${props.path.replace(/[^A-Za-z0-9_-]/g, '-')}`)
const objectValue = computed(() => props.modelValue && typeof props.modelValue === 'object' && !Array.isArray(props.modelValue) ? props.modelValue : {})
const arrayValue = computed(() => Array.isArray(props.modelValue) ? props.modelValue : [])
const requiredFields = computed(() => new Set(Array.isArray(props.schema.required) ? props.schema.required : []))
const enumIndex = computed(() => {
  const index = props.schema.enum?.findIndex(option => Object.is(option, props.modelValue)) ?? -1
  return index < 0 ? '' : String(index)
})

watch(() => props.modelValue, value => {
  if (fieldKind.value !== 'json') return
  jsonText.value = JSON.stringify(value ?? defaultValue(props.schema), null, 2)
  jsonError.value = ''
  emit('validity', { path: props.path, valid: true })
}, { immediate: true, deep: true })

function defaultValue(schema) {
  if (schema.default !== undefined) return structuredCloneSafe(schema.default)
  if (Array.isArray(schema.enum) && schema.enum.length > 0) return structuredCloneSafe(schema.enum[0])
  const type = schema.type || (schema.properties ? 'object' : schema.items ? 'array' : 'string')
  if (type === 'object') return {}
  if (type === 'array') return []
  if (type === 'boolean') return false
  if (type === 'number' || type === 'integer') return 0
  return ''
}

function structuredCloneSafe(value) {
  if (value === undefined) return undefined
  return JSON.parse(JSON.stringify(value))
}

function enumLabel(value) {
  return typeof value === 'string' ? value : JSON.stringify(value)
}

function updateEnum(event) {
  const index = Number.parseInt(event.target.value, 10)
  if (!Number.isInteger(index) || index < 0) return
  emit('update:modelValue', structuredCloneSafe(props.schema.enum[index]))
}

function updateNumber(event) {
  if (event.target.value === '') {
    emit('update:modelValue', undefined)
    return
  }
  const value = props.schema.type === 'integer' ? Number.parseInt(event.target.value, 10) : Number(event.target.value)
  emit('update:modelValue', value)
}

function updateObject(key, value) {
  emit('update:modelValue', { ...objectValue.value, [key]: value })
}

function addArrayItem() {
  emit('update:modelValue', [...arrayValue.value, defaultValue(props.schema.items)])
}

function updateArrayItem(index, value) {
  const next = [...arrayValue.value]
  next[index] = value
  emit('update:modelValue', next)
}

function removeArrayItem(index) {
  emit('update:modelValue', arrayValue.value.filter((_, itemIndex) => itemIndex !== index))
}

function updateJSON() {
  try {
    const parsed = JSON.parse(jsonText.value)
    jsonError.value = ''
    emit('validity', { path: props.path, valid: true })
    emit('update:modelValue', parsed)
  } catch (error) {
    jsonError.value = error instanceof Error ? error.message : t('control.config.invalidJSON')
    emit('validity', { path: props.path, valid: false })
  }
}
</script>

<style scoped>
.schema-field { min-width: 0; }
.schema-field > label, .schema-group > legend, .group-label { display: block; margin-bottom: 6px; color: var(--text-secondary); font-size: 13px; font-weight: 700; }
.schema-field input:not([type='checkbox']), .schema-field select, .schema-field textarea { width: 100%; }
.boolean-field { display: inline-flex !important; align-items: center; gap: 9px; color: var(--text-color) !important; }
.boolean-field input { width: 18px; height: 18px; margin: 0; }
.field-help, .field-error, .array-empty { margin: 6px 0 0; font-size: 12px; line-height: 1.45; }
.field-help, .array-empty { color: var(--text-secondary); }
.field-error { color: var(--error-color); overflow-wrap: anywhere; }
.schema-group { min-width: 0; margin: 0; padding: 12px 0 0 14px; border: 0; border-left: 2px solid var(--border-color); }
.schema-children { display: grid; gap: 14px; }
.array-heading { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.array-heading .group-label { margin: 0; }
.array-item { position: relative; min-width: 0; padding: 12px 44px 12px 0; border-top: 1px solid var(--border-color); }
.remove-item { position: absolute; top: 10px; right: 0; }
.icon-button { width: 34px; height: 34px; min-height: 34px; padding: 0; }
.json-editor { min-height: 120px; font: 12px/1.5 ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-weight: 500; resize: vertical; }
</style>
