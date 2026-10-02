<template>
  <UiSheet
    :open="open"
    size="lg"
    :title="protocol ? t('admin.nodes.protocolForm.titleEdit') : t('admin.nodes.protocolForm.titleCreate')"
    :description="t('admin.nodes.protocolForm.description', { name: node?.name || '—' })"
    :dismissible="!saving"
    data-testid="protocol-sheet"
    @update:open="value => { if (!value) close() }"
  >
    <section v-if="!protocol && templates.length" class="dialog-section">
      <h3 :id="templateHeadingId" class="dialog-section__title">{{ t('admin.nodes.protocolForm.templateLibrary') }}</h3>
      <div class="node-templates" role="group" :aria-labelledby="templateHeadingId">
        <button
          v-for="tpl in templates"
          :key="tpl.type + tpl.name"
          type="button"
          class="node-templates__card"
          :aria-pressed="editor.selectedTemplate.value === tpl.name ? 'true' : 'false'"
          @click="editor.applyTemplate(tpl)"
        >
          <span class="node-templates__name">{{ tpl.name }}</span>
          <span v-if="tpl.description" class="node-templates__desc">{{ tpl.description }}</span>
        </button>
      </div>
    </section>

    <p v-if="protocol" class="node-protocol__hint" data-testid="protocol-masked-hint">{{ t('admin.nodes.protocolForm.maskedSecretsHint') }}</p>

    <section class="dialog-section">
      <UiSegmentedControl
        v-model="form.mode"
        :aria-label="t('admin.nodes.protocolForm.modeLabel')"
        :options="modeOptions"
        class="node-protocol__mode"
        data-testid="protocol-mode"
      />

      <div v-if="form.mode === 'json'" class="node-protocol__editor">
        <div class="node-protocol__toolbar">
          <UiBadge :tone="editor.jsonValid.value ? 'success' : 'danger'" :label="editor.jsonValid.value ? t('admin.nodes.protocolForm.jsonStatus.valid') : t('admin.nodes.protocolForm.jsonStatus.invalid')" />
          <span class="node-protocol__spacer" />
          <UiButton size="sm" :disabled="!editor.jsonValid.value" @click="editor.formatJson()">{{ t('admin.nodes.protocolForm.jsonActions.format') }}</UiButton>
          <UiButton size="sm" :icon="Copy" @click="copyWithToast(editor.jsonEditorContent.value)">{{ t('admin.nodes.protocolForm.jsonActions.copy') }}</UiButton>
          <UiButton v-if="templates.length" size="sm" @click="openTemplatePicker">{{ t('admin.nodes.protocolForm.jsonActions.fromTemplate') }}</UiButton>
        </div>
        <UiTextarea
          v-model="editor.jsonEditorContent.value"
          class="node-protocol__json"
          :label="t('admin.nodes.protocolForm.jsonLabel')"
          :rows="20"
          :placeholder="JSON_PLACEHOLDER"
          :error="editor.jsonValid.value ? '' : t('admin.nodes.messages.invalidJsonDetail', { message: editor.jsonParseError.value })"
          spellcheck="false"
          data-testid="protocol-json"
        />
      </div>

      <div v-else class="node-protocol__editor">
        <div class="form-grid">
          <UiSelect v-model="form.type" size="md" :label="t('admin.nodes.protocolForm.fields.type')" :options="typeOptions" required />
          <UiNumberField v-model="form.port" size="md" :min="1" :max="65535" :label="t('admin.nodes.protocolForm.fields.port')" required />
          <UiSelect v-model="form.tls" size="md" :label="t('admin.nodes.protocolForm.fields.tls')" :options="tlsOptions" />
          <UiSelect v-model="form.transport" size="md" :label="t('admin.nodes.protocolForm.fields.transport')" :options="transportOptions" />
          <UiCheckbox v-model="enabled" :label="t('admin.nodes.protocolForm.enable')" />
          <UiCheckbox v-model="shown" :label="t('admin.nodes.protocolForm.show')" />
        </div>

        <NodeWireGuardFields
          v-if="form.type === 'wireguard'"
          :wg="editor.wireGuardForm"
          @generate-keypair="createWireGuardKeypair"
          @add-path="editor.addWireGuardNetworkPath()"
          @remove-path="index => editor.removeWireGuardNetworkPath(index)"
        />

        <details v-if="form.type !== 'wireguard'" class="node-protocol__advanced">
          <summary>{{ t('admin.nodes.protocolForm.fields.settings') }}</summary>
          <UiTextarea v-model="form.settings" class="node-protocol__json" :aria-label="t('admin.nodes.protocolForm.fields.settings')" :rows="4" placeholder="{}" spellcheck="false" />
        </details>
        <details v-if="form.tls > 0" class="node-protocol__advanced">
          <summary>{{ t('admin.nodes.protocolForm.fields.tlsSettings') }}</summary>
          <UiTextarea v-model="form.tls_settings" class="node-protocol__json" :aria-label="t('admin.nodes.protocolForm.fields.tlsSettings')" :rows="4" placeholder="{}" spellcheck="false" />
        </details>
        <details v-if="form.tls === 2" class="node-protocol__advanced">
          <summary>{{ t('admin.nodes.protocolForm.fields.realitySettings') }}</summary>
          <UiTextarea v-model="form.reality_settings" class="node-protocol__json" :aria-label="t('admin.nodes.protocolForm.fields.realitySettings')" :rows="4" placeholder="{}" spellcheck="false" />
        </details>
        <details v-if="form.transport !== 'tcp'" class="node-protocol__advanced">
          <summary>{{ t('admin.nodes.protocolForm.fields.transportSettings') }}</summary>
          <UiTextarea v-model="form.transport_settings" class="node-protocol__json" :aria-label="t('admin.nodes.protocolForm.fields.transportSettings')" :rows="4" placeholder="{}" spellcheck="false" />
        </details>
      </div>
    </section>
    <p v-if="formError" class="form-error" role="alert" data-testid="protocol-form-error">{{ formError }}</p>

    <template #footer>
      <UiButton :disabled="saving" @click="close">{{ t('common.actions.cancel') }}</UiButton>
      <UiButton variant="primary" data-testid="save-protocol" :loading="saving" @click="save">
        {{ protocol ? t('admin.nodes.protocolForm.save') : t('admin.nodes.protocolForm.create') }}
      </UiButton>
    </template>
  </UiSheet>

  <!-- Template picker for the JSON editor -->
  <UiDialog
    v-model:open="pickerOpen"
    size="sm"
    :title="t('admin.nodes.protocolForm.templatePicker.title')"
    :description="t('admin.nodes.protocolForm.templatePicker.description')"
  >
    <UiSelect
      v-model="pickIndex"
      data-testid="template-picker-select"
      :label="t('admin.nodes.protocolForm.templatePicker.label')"
      :placeholder="t('admin.nodes.protocolForm.templatePicker.placeholder')"
      :options="pickerOptions"
      :error="pickError"
      required
    />
    <template #footer="{ close: closePicker }">
      <UiButton @click="closePicker">{{ t('common.actions.cancel') }}</UiButton>
      <UiButton variant="primary" data-testid="template-picker-apply" @click="applyPickedTemplate">{{ t('admin.nodes.protocolForm.templatePicker.apply') }}</UiButton>
    </template>
  </UiDialog>
</template>

<script setup>
// Add or edit one protocol of a node. JSON first (the whole protocol as one
// object), with a visual form for the common fields and the WireGuard relay.
// Saves with POST /admin/nodes/:id/protocols or PUT …/protocols/:protocol_id
// (useProtocolEditor builds the same body as before the redesign).
import { computed, ref, watch } from 'vue'
import { useId } from 'reka-ui'
import { Copy } from '@lucide/vue'
import { createNodeProtocol, generateWireGuardKeypair, updateNodeProtocol } from '@/api/admin'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiCheckbox from '@/ui/UiCheckbox.vue'
import UiDialog from '@/ui/UiDialog.vue'
import UiNumberField from '@/ui/UiNumberField.vue'
import UiSegmentedControl from '@/ui/UiSegmentedControl.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiSheet from '@/ui/UiSheet.vue'
import UiTextarea from '@/ui/UiTextarea.vue'
import { useToast } from '@/ui/composables/useToast'
import { useAppI18n } from '@/composables/useAppI18n'
import NodeWireGuardFields from './NodeWireGuardFields.vue'
import { readNodeApiError } from './nodeData'
import { useNodeCopy } from './useNodeCopy'
import { JSON_PLACEHOLDER, PROTOCOL_TYPES, useProtocolEditor } from './useProtocolEditor'

const props = defineProps({
  open: { type: Boolean, default: false },
  node: { type: Object, default: null },
  // The protocol to edit; null adds one.
  protocol: { type: Object, default: null },
  templates: { type: Array, default: () => [] }
})
const emit = defineEmits(['update:open', 'saved'])
const { t } = useAppI18n()
const toast = useToast()
const copyWithToast = useNodeCopy()
const editor = useProtocolEditor()
const form = editor.protocolForm
const templateHeadingId = useId(undefined, 'node-templates')

const saving = ref(false)
const formError = ref('')

const modeOptions = computed(() => [
  { value: 'json', label: 'JSON' },
  { value: 'visual', label: t('admin.nodes.protocolForm.tabs.visual') }
])
const typeOptions = computed(() => PROTOCOL_TYPES.map(type => ({
  value: type,
  label: type === 'wireguard' ? 'WireGuard' : t(`networkPages.nodes.protocols.${type}`)
})))
const tlsOptions = computed(() => [
  { value: 0, label: t('admin.nodes.tlsModes.none') },
  { value: 1, label: t('admin.nodes.tlsModes.standard') },
  { value: 2, label: t('admin.nodes.tlsModes.reality') }
])
const transportOptions = computed(() => {
  const options = ['tcp', 'ws', 'grpc', 'quic', 'h2'].map(value => ({ value, label: t(`admin.nodes.transports.${value}`) }))
  // WireGuard runs over UDP; show it when the protocol says so.
  if (form.transport === 'udp') options.push({ value: 'udp', label: 'UDP' })
  return options
})
const enabled = computed({ get: () => Boolean(form.enable), set: value => { form.enable = value ? 1 : 0 } })
const shown = computed({ get: () => Boolean(form.show), set: value => { form.show = value ? 1 : 0 } })

watch(() => props.open, (open) => {
  if (!open) return
  formError.value = ''
  if (props.protocol) editor.startEdit(props.protocol)
  else editor.startCreate()
}, { immediate: true })

watch(editor.jsonEditorContent, () => editor.validateJson())

function close() {
  if (saving.value) return
  emit('update:open', false)
}

async function createWireGuardKeypair() {
  try {
    const response = await generateWireGuardKeypair()
    const payload = response?.data?.data || response?.data || response
    editor.wireGuardForm.serverPrivateKey = payload?.private_key || ''
    editor.wireGuardForm.serverPublicKey = payload?.public_key || ''
  } catch (error) {
    formError.value = t('admin.nodes.messages.generateFailed', { message: readNodeApiError(error) })
  }
}

async function save() {
  if (saving.value) return
  formError.value = ''
  const built = editor.buildPayload()
  if (built.error === 'invalidJson') {
    formError.value = t('admin.nodes.messages.invalidJsonDetail', { message: built.message })
    return
  }
  if (built.error) {
    formError.value = t('admin.nodes.messages.requiredFields')
    return
  }
  const { payload } = built
  const editing = Boolean(props.protocol)
  saving.value = true
  try {
    if (editing) {
      await updateNodeProtocol(props.node.id, props.protocol.id, payload)
    } else {
      await createNodeProtocol(props.node.id, payload)
    }
    saving.value = false
    emit('update:open', false)
    toast.success(t(editing ? 'admin.nodes.messages.protocolSaved' : 'admin.nodes.messages.protocolCreated', { type: String(payload.type || '').toUpperCase(), port: payload.port }))
    emit('saved', payload)
  } catch (e) {
    formError.value = t('admin.nodes.messages.saveFailed', { message: readNodeApiError(e) })
  } finally {
    saving.value = false
  }
}

// Template picker ----------------------------------------------------------
const pickerOpen = ref(false)
const pickIndex = ref(undefined)
const pickError = ref('')
const pickerOptions = computed(() => props.templates.map((tpl, index) => ({ value: index, label: tpl.name })))
watch(pickIndex, () => { pickError.value = '' })

function openTemplatePicker() {
  pickIndex.value = undefined
  pickError.value = ''
  pickerOpen.value = true
}

function applyPickedTemplate() {
  const idx = Number(pickIndex.value)
  if (pickIndex.value === undefined || pickIndex.value === null || Number.isNaN(idx) || idx < 0 || idx >= props.templates.length) {
    pickError.value = t('admin.nodes.protocolForm.templatePicker.required')
    return
  }
  pickerOpen.value = false
  editor.loadTemplateJson(props.templates[idx])
}

defineExpose({ editor })
</script>

<style scoped>
.node-protocol__hint {
  margin: 0 0 var(--space-6);
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-sm);
  background: var(--bg-grouped);
  color: var(--label-2);
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
}

.node-protocol__mode {
  align-self: flex-start;
}

.node-protocol__editor {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.node-protocol__toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
}

.node-protocol__spacer {
  flex: 1 1 auto;
}

.node-protocol__json :deep(textarea) {
  font-family: var(--font-mono);
  font-size: var(--type-caption-size);
  line-height: 1.6;
}

.node-protocol__advanced {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.node-protocol__advanced summary {
  padding: var(--space-1) 0;
  color: var(--label-1);
  font-size: var(--type-callout-size);
  font-weight: var(--weight-medium);
  cursor: pointer;
}

.node-protocol__advanced summary:focus-visible {
  border-radius: var(--radius-xs);
  outline: var(--focus-ring);
  outline-offset: var(--focus-ring-offset);
}

.node-templates {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
  gap: var(--space-2);
}

.node-templates__card {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  min-height: 0;
  padding: var(--space-3);
  border: 1px solid var(--separator);
  border-radius: var(--radius-sm);
  background: var(--bg-elevated);
  color: var(--label-1);
  font: inherit;
  text-align: left;
  cursor: pointer;
  transition: border-color var(--dur-micro) var(--ease-standard), background-color var(--dur-micro) var(--ease-standard);
}

.node-templates__card:hover {
  background: var(--fill-1);
}

.node-templates__card[aria-pressed='true'] {
  border-color: var(--accent);
  background: var(--accent-soft);
}

.node-templates__card:focus-visible {
  outline: var(--focus-ring);
  outline-offset: var(--focus-ring-offset);
}

.node-templates__name {
  font-size: var(--type-callout-size);
  font-weight: var(--weight-semibold);
}

.node-templates__desc {
  color: var(--label-2);
  font-size: var(--type-caption-size);
  line-height: 1.4;
}
</style>
