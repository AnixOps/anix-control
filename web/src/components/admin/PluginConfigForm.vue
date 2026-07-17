<template>
  <div class="plugin-config-form">
    <div class="config-mode" role="group" :aria-label="t('control.config.mode')">
      <button v-if="structuredSupported" class="btn btn-sm" :class="{ 'btn-primary': mode === 'form' }" type="button" @click="mode = 'form'">
        {{ t('control.config.formMode') }}
      </button>
      <button class="btn btn-sm" :class="{ 'btn-primary': mode === 'json' }" type="button" @click="openJSONMode">
        {{ t('control.config.jsonMode') }}
      </button>
    </div>

    <div v-if="mode === 'form'" class="structured-fields">
      <PluginSchemaField
        v-for="(fieldSchema, key) in schema.properties"
        :key="key"
        :schema="fieldSchema"
        :model-value="objectValue[key]"
        :name="fieldSchema.title || key"
        :path="key"
        :required="requiredFields.has(key)"
        @update:model-value="updateProperty(key, $event)"
        @validity="updateFieldValidity"
      />
    </div>

    <div v-else class="json-mode">
      <textarea v-model="jsonText" rows="18" spellcheck="false" class="full-json-editor" :aria-label="t('control.config.jsonMode')" @input="updateJSON"></textarea>
      <p v-if="jsonError" class="field-error" role="alert">{{ jsonError }}</p>
    </div>

    <p v-if="schemaValidationError" class="field-error" role="alert">{{ schemaValidationError }}</p>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import PluginSchemaField from './PluginSchemaField.vue'

const props = defineProps({
  schema: { type: Object, default: () => ({}) },
  modelValue: { default: () => ({}) }
})
const emit = defineEmits(['update:modelValue', 'validity'])
const { t } = useAppI18n()
const mode = ref('form')
const jsonText = ref('{}')
const jsonError = ref('')
const fieldValidity = ref({})

const complexKeywords = ['allOf', 'anyOf', 'oneOf', 'not', 'if', 'then', 'else', 'patternProperties']
const structuredSupported = computed(() => {
  if (!props.schema || typeof props.schema !== 'object' || Array.isArray(props.schema)) return false
  if (complexKeywords.some(key => props.schema[key] !== undefined)) return false
  const type = props.schema.type || (props.schema.properties ? 'object' : '')
  return type === 'object' && props.schema.properties && typeof props.schema.properties === 'object' && Object.keys(props.schema.properties).length > 0
})
const objectValue = computed(() => props.modelValue && typeof props.modelValue === 'object' && !Array.isArray(props.modelValue) ? props.modelValue : {})
const requiredFields = computed(() => new Set(Array.isArray(props.schema.required) ? props.schema.required : []))
const schemaValidationError = computed(() => validateSchemaValue(props.schema, props.modelValue, '$'))
const fallbackFieldsValid = computed(() => Object.values(fieldValidity.value).every(Boolean))
const valid = computed(() => !jsonError.value && !schemaValidationError.value && fallbackFieldsValid.value)

watch(structuredSupported, supported => {
  if (!supported) mode.value = 'json'
}, { immediate: true })

watch(() => props.modelValue, value => {
  if (mode.value !== 'json') return
  jsonText.value = JSON.stringify(value ?? {}, null, 2)
  jsonError.value = ''
}, { immediate: true, deep: true })

watch([() => props.schema, () => props.modelValue], ([schema, value]) => {
  const next = applyDefaults(schema, value)
  if (JSON.stringify(next) !== JSON.stringify(value)) emit('update:modelValue', next)
}, { immediate: true, deep: true })

watch(valid, value => emit('validity', value), { immediate: true })

function openJSONMode() {
  jsonText.value = JSON.stringify(props.modelValue ?? {}, null, 2)
  jsonError.value = ''
  mode.value = 'json'
}

function updateProperty(key, value) {
  emit('update:modelValue', { ...objectValue.value, [key]: value })
}

function updateFieldValidity({ path, valid: fieldValid }) {
  fieldValidity.value = { ...fieldValidity.value, [path]: fieldValid }
}

function updateJSON() {
  try {
    const parsed = JSON.parse(jsonText.value)
    jsonError.value = ''
    emit('update:modelValue', parsed)
  } catch (error) {
    jsonError.value = error instanceof Error ? error.message : t('control.config.invalidJSON')
  }
}

function applyDefaults(schema, value) {
  if (!schema || typeof schema !== 'object') return value
  let next = value
  if (next === undefined && schema.default !== undefined) next = JSON.parse(JSON.stringify(schema.default))
  const type = schema.type || (schema.properties ? 'object' : schema.items ? 'array' : '')
  if (type === 'object') {
    const source = next && typeof next === 'object' && !Array.isArray(next) ? next : {}
    const result = { ...source }
    for (const [key, childSchema] of Object.entries(schema.properties || {})) {
      const child = applyDefaults(childSchema, source[key])
      if (child !== undefined) result[key] = child
    }
    return result
  }
  if (type === 'array' && Array.isArray(next)) return next.map(item => applyDefaults(schema.items || {}, item))
  return next
}

function validateSchemaValue(schema, value, path) {
  if (!schema || typeof schema !== 'object' || Object.keys(schema).length === 0) return ''
  if (value === undefined || value === null) return ''
  if (Array.isArray(schema.enum) && !schema.enum.some(option => JSON.stringify(option) === JSON.stringify(value))) {
    return t('control.config.schemaError', { path })
  }
  const type = schema.type || (schema.properties ? 'object' : schema.items ? 'array' : '')
  if (type === 'object') {
    if (typeof value !== 'object' || Array.isArray(value)) return t('control.config.schemaError', { path })
    for (const requiredKey of schema.required || []) {
      if (value[requiredKey] === undefined || value[requiredKey] === null || value[requiredKey] === '') {
        return t('control.config.requiredError', { path: `${path}.${requiredKey}` })
      }
    }
    for (const [key, childSchema] of Object.entries(schema.properties || {})) {
      const error = validateSchemaValue(childSchema, value[key], `${path}.${key}`)
      if (error) return error
    }
  } else if (type === 'array') {
    if (!Array.isArray(value)) return t('control.config.schemaError', { path })
    if (schema.minItems !== undefined && value.length < schema.minItems) return t('control.config.schemaError', { path })
    if (schema.maxItems !== undefined && value.length > schema.maxItems) return t('control.config.schemaError', { path })
    for (let index = 0; index < value.length; index += 1) {
      const error = validateSchemaValue(schema.items || {}, value[index], `${path}[${index}]`)
      if (error) return error
    }
  } else if (type === 'string') {
    if (typeof value !== 'string') return t('control.config.schemaError', { path })
    if (schema.minLength !== undefined && value.length < schema.minLength) return t('control.config.schemaError', { path })
    if (schema.maxLength !== undefined && value.length > schema.maxLength) return t('control.config.schemaError', { path })
    if (schema.pattern !== undefined) {
      try {
        if (!new RegExp(schema.pattern).test(value)) return t('control.config.schemaError', { path })
      } catch {
        return t('control.config.schemaError', { path })
      }
    }
  } else if (type === 'number' || type === 'integer') {
    if (typeof value !== 'number' || !Number.isFinite(value) || (type === 'integer' && !Number.isInteger(value))) return t('control.config.schemaError', { path })
    if (schema.minimum !== undefined && value < schema.minimum) return t('control.config.schemaError', { path })
    if (schema.maximum !== undefined && value > schema.maximum) return t('control.config.schemaError', { path })
  } else if (type === 'boolean' && typeof value !== 'boolean') {
    return t('control.config.schemaError', { path })
  }
  return ''
}
</script>

<style scoped>
.plugin-config-form { display: grid; gap: 16px; }
.config-mode { display: flex; gap: 6px; flex-wrap: wrap; }
.structured-fields { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 18px 20px; }
.full-json-editor { min-height: 320px; font: 12px/1.5 ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-weight: 500; resize: vertical; }
.field-error { margin: 0; color: var(--error-color); font-size: 12px; overflow-wrap: anywhere; }
@media (max-width: 720px) { .structured-fields { grid-template-columns: 1fr; } }
</style>
