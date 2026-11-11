<template>
  <UiSheet
    :open="open"
    size="lg"
    :title="protocol ? t(`${K}.titleEdit`) : t(`${K}.titleCreate`)"
    :description="t(`${K}.description`, { name: node?.name || '—' })"
    :dismissible="!saving"
    data-testid="protocol-sheet"
    @update:open="value => { if (!value) close() }"
  >
    <!-- Step 1 (adding only): pick a starting point. -->
    <section v-if="step === 'pick'" class="dialog-section" data-testid="protocol-picker">
      <p class="node-protocol__lead">{{ t(`${K}.pickHint`) }}</p>
      <ul class="proto-list" role="list">
        <li v-for="tpl in choices" :key="tpl.key">
          <button type="button" class="proto-option" :data-testid="`protocol-template-${tpl.key}`" @click="pick(tpl)">
            <span class="proto-mark" aria-hidden="true">{{ tpl.mark }}</span>
            <span class="proto-option__body">
              <span class="proto-option__title">
                {{ tpl.title }}
                <span v-if="tpl.recommended" class="proto-chip proto-chip--accent">{{ t(`${K}.recommended`) }}</span>
              </span>
              <span class="proto-option__desc">{{ tpl.description }}</span>
              <span v-if="tpl.specs.length" class="proto-option__specs">
                <span v-for="spec in tpl.specs" :key="spec" class="proto-chip">{{ spec }}</span>
              </span>
            </span>
            <UiIcon :icon="ChevronRight" :size="18" class="proto-option__go" />
          </button>
        </li>
      </ul>
    </section>

    <!-- Step 2: configure. -->
    <template v-else>
      <div class="proto-summary">
        <span class="proto-mark" aria-hidden="true">{{ markOf(form.type) }}</span>
        <span class="proto-summary__body">
          <span class="proto-summary__title">{{ summaryTitle }}</span>
          <span class="proto-summary__specs">{{ summarySpecs }}</span>
        </span>
        <UiButton v-if="!protocol && templates.length" size="sm" variant="tertiary" data-testid="protocol-change" @click="step = 'pick'">{{ t(`${K}.change`) }}</UiButton>
        <UiSegmentedControl
          v-model="form.mode"
          size="sm"
          :aria-label="t(`${K}.modeLabel`)"
          :options="modeOptions"
          data-testid="protocol-mode"
        />
      </div>

      <p v-if="protocol" class="node-protocol__hint" data-testid="protocol-masked-hint">{{ t(`${K}.maskedSecretsHint`) }}</p>

      <section v-if="form.mode === 'json'" class="dialog-section">
        <div class="node-protocol__toolbar">
          <UiBadge :tone="editor.jsonValid.value ? 'success' : 'danger'" :label="editor.jsonValid.value ? t(`${K}.jsonStatus.valid`) : t(`${K}.jsonStatus.invalid`)" />
          <span class="node-protocol__spacer" />
          <UiButton size="sm" :disabled="!editor.jsonValid.value" @click="editor.formatJson()">{{ t(`${K}.jsonActions.format`) }}</UiButton>
          <UiButton size="sm" :icon="Copy" @click="copyWithToast(editor.jsonEditorContent.value)">{{ t(`${K}.jsonActions.copy`) }}</UiButton>
        </div>
        <UiTextarea
          v-model="editor.jsonEditorContent.value"
          class="node-protocol__json"
          :label="t(`${K}.jsonLabel`)"
          :rows="22"
          :placeholder="JSON_PLACEHOLDER"
          :error="editor.jsonValid.value ? '' : t('admin.nodes.messages.invalidJsonDetail', { message: editor.jsonParseError.value })"
          spellcheck="false"
          data-testid="protocol-json"
        />
      </section>

      <div v-else class="proto-form">
        <section class="dialog-section">
          <h3 class="dialog-section__title">{{ t(`${K}.sections.basics`) }}</h3>
          <div class="form-grid">
            <UiSelect v-model="form.type" size="md" :label="t(`${K}.fields.type`)" :options="typeOptions" required />
            <UiNumberField v-model="form.port" size="md" :min="1" :max="65535" :label="t(`${K}.fields.port`)" required />
          </div>
        </section>

        <!-- A type with one TLS mode and one transport has nothing to choose: the summary bar names them. -->
        <section v-if="tlsOptions.length > 1 || transportOptions.length > 1" class="dialog-section">
          <h3 class="dialog-section__title">{{ t(`${K}.sections.connection`) }}</h3>
          <div class="proto-row">
            <span class="proto-row__label">{{ t(`${K}.fields.tls`) }}</span>
            <UiSegmentedControl
              v-if="tlsOptions.length > 1"
              v-model="form.tls"
              block
              :aria-label="t(`${K}.fields.tls`)"
              :options="tlsOptions"
              data-testid="protocol-tls"
            />
            <span v-else class="proto-row__fixed" data-testid="protocol-tls">{{ t(`${K}.fixed`, { value: tlsOptions[0]?.label }) }}</span>
          </div>
          <div class="proto-row">
            <span class="proto-row__label">{{ t(`${K}.fields.transport`) }}</span>
            <UiSelect v-if="transportOptions.length > 1" v-model="form.transport" size="md" :aria-label="t(`${K}.fields.transport`)" :options="transportOptions" data-testid="protocol-transport" />
            <span v-else class="proto-row__fixed" data-testid="protocol-transport">{{ t(`${K}.fixed`, { value: transportOptions[0]?.label }) }}</span>
          </div>
        </section>

        <NodeWireGuardFields v-if="form.type === 'wireguard'" :wg="editor.wireGuardForm" @generate-keypair="createWireGuardKeypair" />
        <NodeIwanFields v-else-if="form.type === 'iwan'" :iwan="editor.iwanForm" />
        <ProtocolFields v-else v-model="editor.protocolModel.value" />

        <section class="dialog-section">
          <h3 class="dialog-section__title">{{ t(`${K}.sections.publish`) }}</h3>
          <div class="proto-switches">
            <UiSwitch v-model="enabled" :label="t(`${K}.enable`)" :description="t(`${K}.enableHint`)" />
            <UiSwitch v-model="shown" :label="t(`${K}.show`)" :description="t(`${K}.showHint`)" />
          </div>
        </section>
      </div>
    </template>

    <p v-if="formError" class="form-error" role="alert" data-testid="protocol-form-error">{{ formError }}</p>

    <template #footer>
      <UiButton :disabled="saving" @click="close">{{ t('common.actions.cancel') }}</UiButton>
      <UiButton v-if="step === 'form'" variant="primary" data-testid="save-protocol" :loading="saving" @click="save">
        {{ protocol ? t(`${K}.save`) : t(`${K}.create`) }}
      </UiButton>
    </template>
  </UiSheet>
</template>

<script setup>
// Add or edit one protocol (inbound) of a node. Adding starts with a list of
// starting points (the protocol templates and a blank one), then the form:
// basics, TLS and transport, the type's own fields, and whether it runs and
// shows in subscriptions. The whole protocol is editable as JSON from the
// summary bar. Saves with POST /admin/nodes/:id/protocols or
// PUT …/protocols/:protocol_id (useProtocolEditor builds the body).
import { computed, ref, watch } from 'vue'
import { ChevronRight, Copy } from '@lucide/vue'
import { createNodeProtocol, generateWireGuardKeypair, updateNodeProtocol } from '@/api/admin'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiIcon from '@/ui/UiIcon.vue'
import UiNumberField from '@/ui/UiNumberField.vue'
import UiSegmentedControl from '@/ui/UiSegmentedControl.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiSheet from '@/ui/UiSheet.vue'
import UiSwitch from '@/ui/UiSwitch.vue'
import UiTextarea from '@/ui/UiTextarea.vue'
import { useToast } from '@/ui/composables/useToast'
import { useAppI18n } from '@/composables/useAppI18n'
import NodeIwanFields from './NodeIwanFields.vue'
import NodeWireGuardFields from './NodeWireGuardFields.vue'
import ProtocolFields from './ProtocolFields.vue'
import { readNodeApiError } from './nodeData'
import { useNodeCopy } from './useNodeCopy'
import { capabilitiesOf, transportsFor } from './protocolCapabilities'
import { JSON_PLACEHOLDER, PROTOCOL_TYPES, useProtocolEditor } from './useProtocolEditor'

const props = defineProps({
  open: { type: Boolean, default: false },
  node: { type: Object, default: null },
  // The protocol to edit; null adds one.
  protocol: { type: Object, default: null },
  templates: { type: Array, default: () => [] }
})
const emit = defineEmits(['update:open', 'saved'])
const K = 'admin.nodes.protocolForm'
const { t } = useAppI18n()
const toast = useToast()
const copyWithToast = useNodeCopy()
const editor = useProtocolEditor()
const form = editor.protocolForm

const step = ref('form') // pick | form
const saving = ref(false)
const formError = ref('')

const MARKS = { vless: 'VL', vmess: 'VM', trojan: 'TJ', shadowsocks: 'SS', hysteria2: 'HY', tuic: 'TU', wireguard: 'WG', iwan: 'iW', anx: 'AX' }
const markOf = type => MARKS[type] || '+'
const typeLabel = type => ({ wireguard: 'WireGuard', iwan: 'Panabit iWAN', anx: 'ANX' }[type] || t(`networkPages.nodes.protocols.${type}`))
const typeOptions = computed(() => PROTOCOL_TYPES.map(type => ({ value: type, label: typeLabel(type) })))

// TLS modes and transports follow the protocol type (protocolCapabilities):
// Shadowsocks is TCP without TLS, Hysteria2 and TUIC UDP with TLS.
const TLS_MODE_KEYS = ['none', 'standard', 'reality']
const TRANSPORT_KEYS = ['tcp', 'ws', 'grpc', 'quic', 'h2', 'httpupgrade', 'udp']
// ANX brings its own security, the A2 jacket, instead of the node's TLS.
const tlsLabel = (type, value) => t(`admin.nodes.tlsModes.${type === 'anx' ? 'anxA2' : (TLS_MODE_KEYS[value] || 'none')}`)
const transportLabel = value => (TRANSPORT_KEYS.includes(value) ? t(`admin.nodes.transports.${value}`) : value)
const tlsOptions = computed(() => capabilitiesOf(form.type).tls.map(value => ({ value, label: tlsLabel(form.type, value).replace(RECOMMENDED, '') })))
const transportOptions = computed(() => transportsFor(form.type, form.tls, form.transport).map(value => ({ value, label: transportLabel(value) })))
const specsOf = (type, tls, transport, port) => [
  transportLabel(transport),
  Number(tls) || type === 'anx' ? tlsLabel(type, Number(tls)).replace(RECOMMENDED, '') : '',
  port ? t(`${K}.portSpec`, { port }) : ''
].filter(Boolean)

// The templates name their recommended one "… (推荐)": shown as a chip.
const RECOMMENDED = /\s*[(（](推荐|recommended)[)）]/i
const modeOptions = computed(() => [
  { value: 'visual', label: t(`${K}.tabs.visual`) },
  { value: 'json', label: t(`${K}.tabs.json`) }
])
const enabled = computed({ get: () => Boolean(form.enable), set: value => { form.enable = value ? 1 : 0 } })
const shown = computed({ get: () => Boolean(form.show), set: value => { form.show = value ? 1 : 0 } })

const choices = computed(() => [
  ...props.templates.map((tpl, index) => ({
    key: index,
    tpl,
    mark: markOf(tpl.type),
    title: tpl.name.replace(RECOMMENDED, ''),
    recommended: RECOMMENDED.test(tpl.name),
    description: tpl.description,
    specs: specsOf(tpl.type, tpl.tls, tpl.transport, tpl.default_port)
  })),
  { key: 'blank', tpl: null, mark: '+', title: t(`${K}.blank.name`), recommended: false, description: t(`${K}.blank.description`), specs: [] }
])

const summaryTitle = computed(() => typeLabel(form.type))
const summarySpecs = computed(() => specsOf(form.type, form.tls, form.transport, form.port).join(' · '))

watch(() => props.open, (open) => {
  if (!open) return
  formError.value = ''
  if (props.protocol) editor.startEdit(props.protocol)
  else editor.startCreate()
  step.value = props.protocol || !props.templates.length ? 'form' : 'pick'
}, { immediate: true })

watch(editor.jsonEditorContent, () => editor.validateJson())

function pick(choice) {
  formError.value = ''
  if (choice.tpl) editor.applyTemplate(choice.tpl)
  else editor.startCreate()
  step.value = 'form'
}

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
  if (built.error === 'unsupported') {
    const { field, type, value } = built.problem
    const label = field === 'tls' ? t(`admin.nodes.tlsModes.${TLS_MODE_KEYS[value] || 'none'}`) : transportLabel(value)
    formError.value = t(`admin.nodes.messages.${field}Unsupported`, { type: typeLabel(type), value: label })
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
    if (editing) await updateNodeProtocol(props.node.id, props.protocol.id, payload)
    else await createNodeProtocol(props.node.id, payload)
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

defineExpose({ editor })
</script>

<style scoped>
.node-protocol__lead {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
}

/* Starting points: one per row, so long descriptions wrap instead of
   running into the next card. */
.proto-list {
  display: flex;
  flex-direction: column;
  margin: 0;
  padding: 0;
  overflow: hidden;
  border: 1px solid var(--separator);
  border-radius: var(--radius-md);
  background: var(--bg-elevated);
  list-style: none;
}

.proto-list li + li {
  border-top: 1px solid var(--separator);
}

.proto-option {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  width: 100%;
  padding: var(--space-3) var(--space-4);
  border: 0;
  border-radius: 0;
  background: transparent;
  color: var(--label-1);
  font: inherit;
  text-align: left;
  white-space: normal;
  cursor: pointer;
  transition: background-color var(--dur-micro) var(--ease-standard);
}

.proto-option:hover {
  background: var(--fill-1);
}

.proto-option:focus-visible {
  outline: var(--focus-ring);
  outline-offset: calc(var(--focus-ring-offset) * -1);
}

.proto-option__body {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  gap: var(--space-1);
  min-width: 0;
}

.proto-option__title {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2);
  font-size: var(--type-body-size);
  font-weight: var(--weight-semibold);
}

.proto-option__desc {
  color: var(--label-2);
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
}

.proto-option__specs {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1);
  margin-top: var(--space-0-5);
}

.proto-option__go {
  flex: none;
  color: var(--label-3);
}

.proto-chip {
  padding: 0 var(--space-2);
  border-radius: var(--radius-pill);
  background: var(--fill-2);
  color: var(--label-2);
  font-size: var(--type-caption-size);
  font-weight: var(--weight-medium);
  line-height: 20px;
  white-space: nowrap;
}

.proto-chip--accent {
  background: var(--accent-soft);
  color: var(--accent);
}

.proto-mark {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  border-radius: var(--radius-sm);
  background: var(--accent-soft);
  color: var(--accent);
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
  font-weight: var(--weight-bold);
}

/* Step 2 */
.proto-summary {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-3);
  margin-bottom: var(--space-5);
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-md);
  background: var(--bg-grouped);
}

.proto-summary__body {
  display: flex;
  flex: 1 1 160px;
  flex-direction: column;
  min-width: 0;
}

.proto-summary__title {
  font-size: var(--type-body-size);
  font-weight: var(--weight-semibold);
}

.proto-summary__specs {
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

.node-protocol__hint {
  margin: 0 0 var(--space-6);
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-sm);
  background: var(--bg-grouped);
  color: var(--label-2);
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
}

.proto-row {
  display: grid;
  grid-template-columns: 72px minmax(0, 1fr);
  align-items: center;
  gap: var(--space-3);
}

@media (max-width: 640px) {
  .proto-row {
    grid-template-columns: minmax(0, 1fr);
    gap: var(--space-1);
  }
}

.proto-row__label {
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.proto-row__fixed {
  color: var(--label-1);
  font-size: var(--type-callout-size);
}

.proto-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
}

.proto-form > .dialog-section + .dialog-section {
  margin-top: 0;
}

.proto-switches {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
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
</style>
