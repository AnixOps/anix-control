<template>
  <UiSection :title="t('adminSubscriptionGroups.templates.title')" :description="t('adminSubscriptionGroups.templates.description')">
    <template #actions>
      <UiButton variant="primary" :icon="Plus" data-test="create-template" @click="openTemplateDialog()">{{ t('adminSubscriptionGroups.templates.create') }}</UiButton>
    </template>
    <UiDataTable
      :columns="columns"
      :rows="templates"
      :label="t('adminSubscriptionGroups.templates.title')"
      :row-label="template => template.name"
      storage-key="admin.subscriptionTemplates"
      :page-size="20"
      :loading="templatesLoading"
      :error="templatesError"
      :error-title="t('adminSubscriptionGroups.templates.loadFailed')"
      :empty-icon="LayoutTemplate"
      :empty-title="t('adminSubscriptionGroups.templates.empty')"
      :empty-description="t('adminSubscriptionGroups.templates.emptyDescription')"
      :row-actions="templateActions"
      @retry="loadTemplates"
    >
      <template #cell-type="{ row }">
        <UiBadge tone="info" :dot="false" :label="String(row.type || '').toUpperCase()" />
      </template>
      <template #cell-tls="{ row }">
        <UiBadge :tone="row.tls === 2 ? 'success' : row.tls === 1 ? 'info' : 'neutral'" :dot="false" :label="tlsLabel(row.tls)" />
      </template>
      <template #cell-enable="{ row }">
        <UiSwitch
          :model-value="row.enable === 1"
          :aria-label="t('adminSubscriptionGroups.templates.enableNamed', { name: row.name })"
          @update:model-value="toggleTemplate(row)"
        />
      </template>
      <template #empty-actions>
        <UiButton variant="primary" :icon="Plus" @click="openTemplateDialog()">{{ t('adminSubscriptionGroups.templates.create') }}</UiButton>
      </template>
    </UiDataTable>

    <UiDialog
      :open="templateDialogOpen"
      size="lg"
      :title="templateForm.id ? t('adminSubscriptionGroups.templates.editTitle') : t('adminSubscriptionGroups.templates.createTitle')"
      :dismissible="!templateSaving"
      @update:open="value => { if (!value) closeTemplateModal() }"
    >
      <form id="subscription-template-form" class="form-grid" novalidate @submit.prevent="saveTemplate">
        <UiTextField
          id="subscription-template-name"
          v-model="templateForm.name"
          required
          :label="t('adminSubscriptionGroups.templates.name')"
          :placeholder="t('adminSubscriptionGroups.templates.namePlaceholder')"
          :error="touched.name ? errors.name : ''"
          @blur="touched.name = true"
        />
        <UiSelect id="subscription-template-type" v-model="templateForm.type" required :label="t('adminSubscriptionGroups.templates.protocol')" :options="protocolOptions" />
        <UiTextField
          id="subscription-template-server"
          v-model="templateForm.server"
          required
          :label="t('adminSubscriptionGroups.templates.server')"
          placeholder="us.example.com"
          :error="touched.server ? errors.server : ''"
          @blur="touched.server = true"
        />
        <UiNumberField
          id="subscription-template-port"
          v-model="templateForm.port"
          required
          :min="1"
          :max="65535"
          :label="t('adminSubscriptionGroups.templates.port')"
          :error="errors.port"
        />
        <UiSelect id="subscription-template-tls" v-model="templateForm.tls" :label="t('adminSubscriptionGroups.templates.tls')" :options="tlsOptions" />
        <UiSelect id="subscription-template-transport" v-model="templateForm.transport" :label="t('adminSubscriptionGroups.templates.transport')" :options="transportOptions" />
        <UiTextField
          v-if="templateForm.tls > 0"
          id="subscription-template-sni"
          v-model="templateForm.server_name"
          :label="t('adminSubscriptionGroups.templates.sni')"
          placeholder="www.example.com"
        />
        <UiSelect
          v-if="templateForm.tls > 0"
          id="subscription-template-fingerprint"
          v-model="fingerprintChoice"
          :label="t('adminSubscriptionGroups.templates.fingerprint')"
          :options="fingerprintOptions"
        />
        <template v-if="templateForm.tls === 2">
          <UiTextField id="subscription-template-pbk" v-model="templateForm.reality_public_key" class="template-mono" :label="t('adminSubscriptionGroups.templates.realityPublicKey')" />
          <UiTextField id="subscription-template-sid" v-model="templateForm.reality_short_id" class="template-mono" :label="t('adminSubscriptionGroups.templates.realityShortId')" />
        </template>
        <UiTextField
          v-if="templateForm.transport === 'ws'"
          id="subscription-template-ws-path"
          v-model="wsPath"
          :label="t('adminSubscriptionGroups.templates.wsPath')"
          placeholder="/ws"
        />
        <UiSelect
          v-if="templateForm.type === 'vless' && templateForm.tls === 2"
          id="subscription-template-flow"
          v-model="flowChoice"
          :label="t('adminSubscriptionGroups.templates.flow')"
          :options="flowOptions"
        />
        <div class="form-grid__full">
          <UiSwitch v-model="templateForm.enable" :label="t('adminSubscriptionGroups.templates.enabled')" />
        </div>
        <p v-if="templateFormError" class="form-error form-grid__full" role="alert" data-test="template-form-error">{{ templateFormError }}</p>
      </form>
      <template #footer>
        <UiButton :disabled="templateSaving" @click="closeTemplateModal">{{ t('common.actions.cancel') }}</UiButton>
        <UiButton variant="primary" type="submit" form="subscription-template-form" data-test="save-template" :loading="templateSaving">{{ t('common.actions.save') }}</UiButton>
      </template>
    </UiDialog>
  </UiSection>
</template>

<script setup>
// The templates (nodes) of one subscription group: a table with an inline
// on/off switch, copy link, edit (dialog) and delete. Endpoints and fields
// unchanged: GET/POST /admin/subscription/groups/:id/templates,
// PUT/DELETE /admin/subscription/templates/:id.
import { computed, reactive, ref, watch } from 'vue'
import { Link, LayoutTemplate, Pencil, Plus, Trash2 } from '@lucide/vue'
import adminApi from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiDialog from '@/ui/UiDialog.vue'
import UiNumberField from '@/ui/UiNumberField.vue'
import UiSection from '@/ui/UiSection.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiSwitch from '@/ui/UiSwitch.vue'
import UiTextField from '@/ui/UiTextField.vue'
import { useConfirm } from '@/ui/composables/useConfirm'
import { useToast } from '@/ui/composables/useToast'
import {
  TEMPLATE_PROTOCOLS,
  TEMPLATE_TRANSPORTS,
  TLS_FINGERPRINTS,
  ensureSubscriptionSuccess,
  generateNodeLink,
  readSubscriptionList,
  subscriptionErrorText
} from './subscriptionShared'
import { useGroupActions } from './useGroupActions'

const props = defineProps({
  group: { type: Object, required: true }
})
const emit = defineEmits(['changed'])

const { t } = useAppI18n()
const toast = useToast()
const confirm = useConfirm()
const { copyToClipboard } = useGroupActions()

const templates = ref([])
const templatesLoading = ref(false)
const templatesError = ref(null)
const templateDialogOpen = ref(false)
const templateSaving = ref(false)
const templateFormError = ref('')
const touched = reactive({ name: false, server: false })
const templateForm = reactive(emptyTemplateForm())
const wsPath = ref('/ws')
const vlessFlow = ref('xtls-rprx-vision')

function emptyTemplateForm() {
  return {
    id: null,
    name: '',
    type: 'vless',
    server: '',
    port: 443,
    server_name: '',
    tls: 1,
    tls_fingerprint: 'chrome',
    transport: 'tcp',
    reality_public_key: '',
    reality_short_id: '',
    enable: true
  }
}

const PROTOCOL_LABELS = { vless: 'VLESS', vmess: 'VMess', trojan: 'Trojan', shadowsocks: 'Shadowsocks', hysteria2: 'Hysteria2', tuic: 'TUIC' }
const TRANSPORT_LABELS = { tcp: 'TCP', ws: 'WebSocket', grpc: 'gRPC', h2: 'HTTP/2', quic: 'QUIC' }
const FINGERPRINT_LABELS = { chrome: 'Chrome', firefox: 'Firefox', safari: 'Safari', edge: 'Edge', random: 'Random' }

const protocolOptions = TEMPLATE_PROTOCOLS.map(value => ({ value, label: PROTOCOL_LABELS[value] }))
const transportOptions = TEMPLATE_TRANSPORTS.map(value => ({ value, label: TRANSPORT_LABELS[value] }))
const tlsOptions = computed(() => [
  { value: 0, label: t('adminSubscriptionGroups.templates.tlsNone') },
  { value: 1, label: 'TLS' },
  { value: 2, label: 'Reality' }
])
// The API keeps "default" and "none" as an empty string; a select option
// cannot be empty, so these choices stand in for it.
const DEFAULT_CHOICE = '__default'
const NONE_CHOICE = '__none'
const fingerprintChoice = computed({
  get: () => templateForm.tls_fingerprint || DEFAULT_CHOICE,
  set: value => { templateForm.tls_fingerprint = value === DEFAULT_CHOICE ? '' : value }
})
const flowChoice = computed({
  get: () => vlessFlow.value || NONE_CHOICE,
  set: value => { vlessFlow.value = value === NONE_CHOICE ? '' : value }
})
const fingerprintOptions = computed(() => [
  { value: DEFAULT_CHOICE, label: t('adminSubscriptionGroups.templates.defaultOption') },
  ...TLS_FINGERPRINTS.map(value => ({ value, label: FINGERPRINT_LABELS[value] }))
])
const flowOptions = computed(() => [
  { value: NONE_CHOICE, label: t('adminSubscriptionGroups.templates.noneOption') },
  { value: 'xtls-rprx-vision', label: 'xtls-rprx-vision' }
])

const errors = computed(() => ({
  name: String(templateForm.name || '').trim() ? '' : t('adminSubscriptionGroups.templates.nameRequired'),
  server: String(templateForm.server || '').trim() ? '' : t('adminSubscriptionGroups.templates.serverRequired'),
  port: Number.isInteger(Number(templateForm.port)) && Number(templateForm.port) >= 1 && Number(templateForm.port) <= 65535
    ? ''
    : t('adminSubscriptionGroups.templates.portInvalid')
}))

const columns = computed(() => [
  { key: 'name', label: t('adminSubscriptionGroups.templates.name'), primary: true, sortable: true, hideable: false },
  { key: 'type', label: t('adminSubscriptionGroups.templates.protocol'), secondary: true, sortable: true },
  { key: 'server', label: t('adminSubscriptionGroups.templates.server'), breakpoint: 'md' },
  { key: 'port', label: t('adminSubscriptionGroups.templates.port'), numeric: true, align: 'end' },
  { key: 'tls', label: t('adminSubscriptionGroups.templates.tls') },
  { key: 'enable', label: t('adminSubscriptionGroups.templates.enabled'), card: false }
])

const templateActions = template => [
  { key: 'copy', label: t('adminSubscriptionGroups.templates.copyLink'), icon: Link, onSelect: () => copyTemplateLink(template) },
  { key: 'edit', label: t('common.actions.edit'), icon: Pencil, onSelect: () => editTemplate(template) },
  { key: 'delete', label: t('common.actions.delete'), icon: Trash2, danger: true, separatorBefore: true, onSelect: () => deleteTemplate(template) }
]

function tlsLabel(tls) {
  return tls === 2 ? 'Reality' : tls === 1 ? 'TLS' : t('adminSubscriptionGroups.templates.tlsNone')
}

async function loadTemplates() {
  templatesLoading.value = true
  templatesError.value = null
  try {
    const res = await adminApi.getSubscriptionTemplates(props.group.id)
    templates.value = readSubscriptionList(res, t('adminSubscriptionGroups.templates.loadFailed'))
  } catch (error) {
    templates.value = []
    templatesError.value = subscriptionErrorText(error) || t('adminSubscriptionGroups.templates.loadFailed')
  } finally {
    templatesLoading.value = false
  }
}

function resetForm() {
  Object.assign(templateForm, emptyTemplateForm())
  wsPath.value = '/ws'
  vlessFlow.value = 'xtls-rprx-vision'
  touched.name = false
  touched.server = false
  templateFormError.value = ''
}

function openTemplateDialog() {
  resetForm()
  templateDialogOpen.value = true
}

function editTemplate(template) {
  resetForm()
  Object.assign(templateForm, {
    id: template.id,
    name: template.name,
    type: template.type,
    server: template.server,
    port: template.port,
    server_name: template.server_name || '',
    tls: template.tls,
    tls_fingerprint: template.tls_fingerprint || '',
    transport: template.transport || 'tcp',
    reality_public_key: template.reality_public_key || '',
    reality_short_id: template.reality_short_id || '',
    enable: template.enable === 1
  })
  if (template.transport_settings) {
    try {
      wsPath.value = JSON.parse(template.transport_settings).path || '/ws'
    } catch {
      // Keep the default path.
    }
  }
  if (template.protocol_settings) {
    try {
      vlessFlow.value = JSON.parse(template.protocol_settings).flow || ''
    } catch {
      // Keep the default flow.
    }
  }
  templateDialogOpen.value = true
}

function closeTemplateModal() {
  if (templateSaving.value) return
  templateDialogOpen.value = false
}

async function saveTemplate() {
  if (templateSaving.value) return
  touched.name = true
  touched.server = true
  if (errors.value.name || errors.value.server || errors.value.port) return
  templateSaving.value = true
  templateFormError.value = ''
  const failed = t('adminSubscriptionGroups.messages.saveFailed')
  try {
    const data = {
      name: templateForm.name,
      type: templateForm.type,
      server: templateForm.server,
      port: templateForm.port,
      server_name: templateForm.server_name || null,
      tls: templateForm.tls,
      tls_fingerprint: templateForm.tls_fingerprint || null,
      transport: templateForm.transport,
      enable: templateForm.enable ? 1 : 0
    }
    if (templateForm.tls === 2) {
      data.reality_public_key = templateForm.reality_public_key
      data.reality_short_id = templateForm.reality_short_id
    }
    if (templateForm.transport === 'ws') {
      data.transport_settings = JSON.stringify({ path: wsPath.value, host: templateForm.server_name || templateForm.server })
    }
    if (templateForm.type === 'vless' && vlessFlow.value) {
      data.protocol_settings = JSON.stringify({ flow: vlessFlow.value })
    }

    if (templateForm.id) {
      ensureSubscriptionSuccess(await adminApi.updateSubscriptionTemplate(templateForm.id, data), failed)
    } else {
      ensureSubscriptionSuccess(await adminApi.createSubscriptionTemplate(props.group.id, data), failed)
    }
    templateSaving.value = false
    toast.success(t('adminSubscriptionGroups.messages.templateSaved'))
    templateDialogOpen.value = false
    loadTemplates()
    emit('changed')
  } catch {
    templateFormError.value = failed
  } finally {
    templateSaving.value = false
  }
}

async function deleteTemplate(template) {
  const confirmed = await confirm({
    tone: 'danger',
    title: t('adminSubscriptionGroups.confirm.deleteTemplateTitle', { name: template.name }),
    message: t('adminSubscriptionGroups.confirm.deleteTemplateMessage'),
    confirmLabel: t('adminSubscriptionGroups.confirm.deleteTemplateAction'),
    onConfirm: async () => {
      try {
        ensureSubscriptionSuccess(await adminApi.deleteSubscriptionTemplate(template.id), t('adminSubscriptionGroups.messages.deleteFailed'))
      } catch (error) {
        throw new Error(error?.message || t('adminSubscriptionGroups.messages.deleteFailed'))
      }
    }
  })
  if (!confirmed) return
  toast.success(t('adminSubscriptionGroups.messages.templateDeleted'))
  loadTemplates()
  emit('changed')
}

async function toggleTemplate(template) {
  try {
    ensureSubscriptionSuccess(
      await adminApi.updateSubscriptionTemplate(template.id, { enable: template.enable === 1 ? 0 : 1 }),
      t('adminSubscriptionGroups.messages.updateFailed')
    )
    loadTemplates()
  } catch {
    toast.error(t('adminSubscriptionGroups.messages.updateFailed'))
  }
}

function copyTemplateLink(template) {
  copyToClipboard(generateNodeLink(template))
}

watch(() => props.group?.id, id => { if (id) loadTemplates() }, { immediate: true })
</script>

<style scoped>
.template-mono :deep(input) {
  font-family: var(--font-mono);
}
</style>
