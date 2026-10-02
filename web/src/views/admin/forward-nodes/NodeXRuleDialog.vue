<template>
  <UiDialog
    :open="open"
    size="lg"
    :title="editing ? t('runtime.nodeXTopology.ruleModal.titleEdit') : t('runtime.nodeXTopology.ruleModal.titleAdd')"
    :dismissible="!saving"
    @update:open="value => { if (!value) close() }"
  >
    <div v-if="loading" class="fn-dialog-loading" role="status">{{ t('runtime.nodeXTopology.ruleModal.loading') }}</div>
    <template v-else>
      <div class="dialog-section">
        <div class="form-grid">
          <UiTextField
            v-model.trim="form.name"
            required
            :label="t('runtime.nodeXTopology.ruleModal.fields.name')"
            :placeholder="t('runtime.nodeXTopology.ruleModal.placeholders.name')"
            :error="errors.name"
          />
          <UiSelect
            v-model="form.protocol"
            :label="t('runtime.nodeXTopology.ruleModal.fields.protocol')"
            :options="protocolOptions"
          />
          <UiSelect
            :model-value="form.relayNodeId || undefined"
            required
            :label="t('runtime.nodeXTopology.ruleModal.fields.relayNode')"
            :placeholder="t('runtime.nodeXTopology.ruleModal.placeholders.relayNode')"
            :options="relayOptions"
            :error="errors.relayNodeId"
            @update:model-value="value => { form.relayNodeId = String(value ?? '') }"
          />
          <UiTextField
            v-model.trim="form.listenPort"
            required
            type="number"
            min="1"
            max="65535"
            :label="t('runtime.nodeXTopology.ruleModal.fields.listenPort')"
            :error="errors.listenPort"
          />
          <UiSelect
            :model-value="form.exitNodeId || undefined"
            required
            :label="t('runtime.nodeXTopology.ruleModal.fields.exitNode')"
            :placeholder="t('runtime.nodeXTopology.ruleModal.placeholders.exitNode')"
            :options="exitOptions"
            :error="errors.exitNodeId"
            @update:model-value="value => { form.exitNodeId = String(value ?? '') }"
          />
          <UiTextField
            v-model.trim="form.targetPort"
            required
            type="number"
            min="1"
            max="65535"
            :label="t('runtime.nodeXTopology.ruleModal.fields.targetPort')"
            :error="errors.targetPort"
          />
          <UiTextField
            v-model.trim="form.targetHost"
            class="form-grid__full"
            required
            :label="t('runtime.nodeXTopology.ruleModal.fields.targetHost')"
            :placeholder="t('runtime.nodeXTopology.ruleModal.placeholders.targetHost')"
            :error="errors.targetHost"
          />
        </div>
      </div>
      <div class="dialog-section">
        <div class="form-grid">
          <UiTextField
            v-model.trim="form.userId"
            inputmode="numeric"
            :label="t('runtime.nodeXTopology.ruleModal.fields.userId')"
            :placeholder="t('runtime.nodeXTopology.ruleModal.placeholders.userId')"
            :disabled="editing"
          />
          <UiTextField
            v-model.trim="form.userGroupId"
            inputmode="numeric"
            :label="t('runtime.nodeXTopology.ruleModal.fields.userGroupId')"
            :placeholder="t('runtime.nodeXTopology.ruleModal.placeholders.userGroupId')"
            :disabled="editing"
          />
        </div>
        <p v-if="editing" class="fn-dialog-hint">{{ t('runtime.nodeXTopology.ruleModal.ownerReadOnlyHint') }}</p>
        <p v-if="errors.owner" class="form-error" role="alert">{{ errors.owner }}</p>
      </div>
      <div class="dialog-section">
        <div class="form-grid">
          <UiTextField
            v-model.trim="form.speedLimit"
            type="number"
            min="0"
            :label="t('runtime.nodeXTopology.ruleModal.fields.speedLimit')"
          />
          <UiTextField
            v-model.trim="form.trafficLimit"
            type="number"
            min="0"
            :label="t('runtime.nodeXTopology.ruleModal.fields.trafficLimit')"
          />
          <UiTextField
            v-model="form.expireTime"
            type="datetime-local"
            :label="t('runtime.nodeXTopology.ruleModal.fields.expireTime')"
          />
          <UiTextarea
            v-model="form.remark"
            class="form-grid__full"
            :rows="3"
            :label="t('runtime.nodeXTopology.ruleModal.fields.remark')"
            :placeholder="t('runtime.nodeXTopology.ruleModal.placeholders.remark')"
          />
        </div>
      </div>
    </template>
    <template #footer>
      <UiButton :disabled="saving" @click="close">{{ t('runtime.nodeXTopology.actions.cancel') }}</UiButton>
      <UiButton variant="primary" data-test="forward-rule-submit" :loading="saving" :disabled="loading" @click="submit">
        {{ editing ? t('runtime.nodeXTopology.actions.saveChanges') : t('runtime.nodeXTopology.actions.createRule') }}
      </UiButton>
    </template>
  </UiDialog>
</template>

<script setup>
// Add or edit a legacy forward rule (/admin/forward/rules). The relay and
// exit pickers list the NodeX nodes (one page of 500, as before U7); the
// owner fields cannot change after creation (backend update API).
import { computed, reactive, ref, watch } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { createForwardRule, getForwardNodes, getForwardRule, updateForwardRule } from '@/api/admin'
import UiButton from '@/ui/UiButton.vue'
import UiDialog from '@/ui/UiDialog.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiTextField from '@/ui/UiTextField.vue'
import UiTextarea from '@/ui/UiTextarea.vue'
import { useToast } from '@/ui/composables/useToast'
import {
  errorMessage,
  isPort,
  listOf,
  normalizeNode,
  normalizeRule,
  parseNonNegativeInt,
  parsePositiveInt,
  toDateTimeLocal,
  toISOStringOrNull,
  unwrapForwardResponse
} from './forwardNodeModel'

const props = defineProps({
  open: { type: Boolean, default: false },
  ruleId: { type: Number, default: null }
})
const emit = defineEmits(['update:open', 'saved'])

const { t, translateLiteral } = useAppI18n()
const toast = useToast()

const loading = ref(false)
const saving = ref(false)
const nodeOptions = ref([])
const editing = computed(() => Boolean(props.ruleId))
const form = reactive(emptyForm())
const errors = reactive(emptyErrors())

const protocolOptions = computed(() => ['tcp', 'udp', 'both'].map(value => ({ value, label: t(`runtime.nodeXTopology.protocols.${value}`) })))
const relayOptions = computed(() => nodeOptions.value.filter(node => node.type === 'relay').map(nodeOption))
const exitOptions = computed(() => nodeOptions.value.filter(node => node.type === 'exit').map(nodeOption))

function nodeOption(node) {
  return { value: String(node.id), label: `${node.name} (${node.host})` }
}

function emptyForm() {
  return {
    id: null,
    name: '',
    relayNodeId: '',
    exitNodeId: '',
    listenPort: '',
    protocol: 'tcp',
    targetHost: '',
    targetPort: '',
    userId: '',
    userGroupId: '',
    speedLimit: '',
    trafficLimit: '',
    expireTime: '',
    remark: ''
  }
}

function emptyErrors() {
  return { name: '', relayNodeId: '', exitNodeId: '', listenPort: '', targetHost: '', targetPort: '', owner: '' }
}

function failure(error, key) {
  return errorMessage(error, t(key), translateLiteral)
}

async function loadNodeOptions() {
  try {
    const payload = unwrapForwardResponse(await getForwardNodes({ page: 1, page_size: 500, scope: 'nodex' }), t('runtime.nodeXTopology.validation.requestFailed'))
    nodeOptions.value = listOf(payload).map(normalizeNode).sort((left, right) => (
      left.type === right.type ? left.name.localeCompare(right.name) : left.type.localeCompare(right.type)
    ))
  } catch (error) {
    nodeOptions.value = []
    toast.error(failure(error, 'runtime.nodeXTopology.messages.loadNodeOptionsFailed'))
  }
}

function fill(rule) {
  Object.assign(form, {
    id: rule.id,
    name: rule.name,
    relayNodeId: rule.relayNodeId ? String(rule.relayNodeId) : '',
    exitNodeId: rule.exitNodeId ? String(rule.exitNodeId) : '',
    listenPort: rule.listenPort ? String(rule.listenPort) : '',
    protocol: rule.protocol,
    targetHost: rule.targetHost,
    targetPort: rule.targetPort ? String(rule.targetPort) : '',
    userId: rule.userId ? String(rule.userId) : '',
    userGroupId: rule.userGroupId ? String(rule.userGroupId) : '',
    speedLimit: rule.speedLimit !== null && rule.speedLimit !== undefined ? String(rule.speedLimit) : '',
    trafficLimit: rule.trafficLimit !== null && rule.trafficLimit !== undefined ? String(rule.trafficLimit) : '',
    expireTime: toDateTimeLocal(rule.expireTime),
    remark: rule.remark || ''
  })
}

watch(() => [props.open, props.ruleId], async ([open, ruleId]) => {
  if (!open) return
  Object.assign(form, emptyForm())
  Object.assign(errors, emptyErrors())
  loading.value = true
  try {
    await loadNodeOptions()
    if (ruleId) fill(normalizeRule(unwrapForwardResponse(await getForwardRule(ruleId), t('runtime.nodeXTopology.validation.requestFailed'))))
  } catch (error) {
    toast.error(failure(error, 'runtime.nodeXTopology.messages.loadRuleDetailFailed'))
    close(true)
  } finally {
    loading.value = false
  }
}, { immediate: true })

function close(force = false) {
  if (!force && saving.value) return
  emit('update:open', false)
}

function validate() {
  Object.assign(errors, emptyErrors())
  if (!form.name.trim()) errors.name = t('runtime.nodeXTopology.validation.ruleNameRequired')
  if (!parsePositiveInt(form.relayNodeId)) errors.relayNodeId = t('runtime.nodeXTopology.validation.relayNodeRequired')
  if (!parsePositiveInt(form.exitNodeId)) errors.exitNodeId = t('runtime.nodeXTopology.validation.exitNodeRequired')
  if (!isPort(form.listenPort)) errors.listenPort = t('runtime.nodeXTopology.validation.listenPortRange')
  if (!form.targetHost.trim()) errors.targetHost = t('runtime.nodeXTopology.validation.targetHostRequired')
  if (!isPort(form.targetPort)) errors.targetPort = t('runtime.nodeXTopology.validation.targetPortRange')
  if (form.userId.trim() && form.userGroupId.trim()) errors.owner = t('runtime.nodeXTopology.validation.ownerConflict')
  return Object.values(errors).every(value => !value)
}

async function submit() {
  if (!validate()) return
  saving.value = true
  try {
    const payload = {
      name: form.name.trim(),
      relay_node_id: parsePositiveInt(form.relayNodeId),
      exit_node_id: parsePositiveInt(form.exitNodeId),
      listen_port: parsePositiveInt(form.listenPort),
      protocol: form.protocol,
      target_host: form.targetHost.trim(),
      target_port: parsePositiveInt(form.targetPort),
      remark: form.remark.trim()
    }
    const speedLimit = parseNonNegativeInt(form.speedLimit)
    const trafficLimit = parseNonNegativeInt(form.trafficLimit)
    const expireTime = toISOStringOrNull(form.expireTime)
    if (speedLimit !== null) payload.speed_limit = speedLimit
    if (trafficLimit !== null) payload.traffic_limit = trafficLimit
    if (expireTime) payload.expire_time = expireTime

    if (editing.value && form.id) {
      await updateForwardRule(form.id, payload)
      toast.success(t('runtime.nodeXTopology.messages.ruleUpdated'))
    } else {
      const userId = parsePositiveInt(form.userId)
      const userGroupId = parsePositiveInt(form.userGroupId)
      if (userId) payload.user_id = userId
      if (userGroupId) payload.user_group_id = userGroupId
      await createForwardRule(payload)
      toast.success(t('runtime.nodeXTopology.messages.ruleCreated'))
    }
    saving.value = false
    close(true)
    emit('saved')
  } catch (error) {
    toast.error(failure(error, 'runtime.nodeXTopology.messages.saveRuleFailed'))
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.fn-dialog-loading {
  padding: var(--space-6) 0;
  color: var(--label-2);
  text-align: center;
}

.fn-dialog-hint {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}
</style>
