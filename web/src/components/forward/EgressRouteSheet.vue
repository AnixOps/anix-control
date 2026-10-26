<template>
  <UiSheet
    :open="open"
    size="md"
    :title="editing ? t('egressRoutes.sheet.editTitle', { name: route.name }) : t('egressRoutes.sheet.createTitle')"
    :description="t('egressRoutes.sheet.description')"
    :dismissible="!saving"
    @update:open="value => emit('update:open', value)"
  >
    <form class="egress-sheet" novalidate data-testid="egress-route-form" @submit.prevent="save">
      <UiTextField v-model="form.name" size="md" :label="t('egressRoutes.sheet.name')" required :placeholder="t('egressRoutes.sheet.namePlaceholder')" :error="errors.name" />
      <UiSelect v-model="form.sourceNodeId" size="md" :label="t('egressRoutes.sheet.source')" :options="nodeOptions" required :error="errors.source" data-testid="egress-route-source" />
      <UiSelect v-model="form.exitNodeId" size="md" :label="t('egressRoutes.sheet.exit')" :options="nodeOptions" required :error="errors.exit" data-testid="egress-route-exit" />
      <UiTextField v-model="form.exitHost" size="md" :label="t('egressRoutes.sheet.exitHost')" :placeholder="exitNodeHost" :help="t('egressRoutes.sheet.exitHostHelp')" />
      <UiNumberField v-model="form.relayPort" size="md" :label="t('egressRoutes.sheet.relayPort')" :min="1" :max="65535" required :help="t('egressRoutes.sheet.relayPortHelp')" :error="errors.relayPort" />
      <div class="egress-sheet__domains">
        <UiTextarea v-model="form.domains" :rows="8" :label="t('egressRoutes.sheet.domains')" required :help="t('egressRoutes.sheet.domainsHelp')" :error="errors.domains" data-testid="egress-route-domains" />
        <UiButton size="sm" variant="ghost" @click="addPreset">{{ t('egressRoutes.sheet.presetAi') }}</UiButton>
      </div>
      <UiSwitch v-model="form.enabled" :label="t('egressRoutes.sheet.enabled')" />
      <UiSwitch v-if="editing" v-model="form.rotateKey" :label="t('egressRoutes.sheet.rotateKey')" :description="t('egressRoutes.sheet.rotateKeyHelp')" />
      <p v-if="saveError" class="fwd-note is-danger" role="alert">{{ saveError }}</p>
    </form>
    <template #footer="{ close }">
      <UiButton :disabled="saving" @click="close">{{ t('forwardV4.common.cancel') }}</UiButton>
      <UiButton variant="primary" :loading="saving" data-testid="egress-route-save" @click="save">{{ editing ? t('egressRoutes.sheet.save') : t('egressRoutes.sheet.create') }}</UiButton>
    </template>
  </UiSheet>
</template>

<script setup>
// The egress route sheet: a route's two nodes, relay port and domains (one
// per line). The relay key never shows; rotate_key replaces it.
import { computed, ref, watch } from 'vue'
import { UiButton, UiNumberField, UiSelect, UiSheet, UiSwitch, UiTextarea, UiTextField, useToast } from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import { createKernelEgressRoute, updateKernelEgressRoute } from '@/api/kernel'
import { AI_DOMAINS, egressErrorMessage, egressForm, egressRequest, parseDomains } from './egressModel'
import './forward.css'

const props = defineProps({
  open: { type: Boolean, default: false },
  // null for a new route.
  route: { type: Object, default: null },
  // The proxy nodes: [{ id, name, host }].
  nodes: { type: Array, default: () => [] }
})
const emit = defineEmits(['update:open', 'saved'])
const { t } = useAppI18n()
const toast = useToast()

const editing = computed(() => Boolean(props.route?.id))
const form = ref(egressForm(null))
const errors = ref({})
const saving = ref(false)
const saveError = ref('')

watch(() => props.open, value => {
  if (!value) return
  form.value = egressForm(props.route)
  errors.value = {}
  saveError.value = ''
}, { immediate: true })

const nodeOptions = computed(() => props.nodes.map(node => ({
  value: node.id,
  label: `${node.name || `#${node.id}`}${node.host ? ` · ${node.host}` : ''}`
})))
const exitNodeHost = computed(() => props.nodes.find(node => node.id === form.value.exitNodeId)?.host || '')

function addPreset() {
  const have = new Set(parseDomains(form.value.domains))
  const added = AI_DOMAINS.filter(domain => !have.has(domain))
  form.value.domains = [...parseDomains(form.value.domains), ...added].join('\n')
}

function validate() {
  const required = t('egressRoutes.sheet.required')
  const found = {}
  if (!form.value.name.trim()) found.name = required
  if (!form.value.sourceNodeId) found.source = required
  if (!form.value.exitNodeId) found.exit = required
  else if (form.value.exitNodeId === form.value.sourceNodeId) found.exit = t('egressRoutes.sheet.sameNode')
  if (!form.value.relayPort) found.relayPort = required
  if (!parseDomains(form.value.domains).length) found.domains = required
  return found
}

async function save() {
  if (saving.value) return
  errors.value = validate()
  if (Object.keys(errors.value).length) return
  const body = egressRequest(form.value)
  saving.value = true
  saveError.value = ''
  try {
    const answer = editing.value
      ? await updateKernelEgressRoute(props.route.id, body)
      : await createKernelEgressRoute(body)
    toast.success(t(editing.value ? 'egressRoutes.toast.saved' : 'egressRoutes.toast.created', { name: body.name }))
    emit('saved', answer || null)
    emit('update:open', false)
  } catch (error) {
    saveError.value = egressErrorMessage(error)
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.egress-sheet {
  display: grid;
  gap: var(--space-4);
}

.egress-sheet__domains {
  display: grid;
  gap: var(--space-2);
  justify-items: start;
}

.egress-sheet__domains > :first-child {
  justify-self: stretch;
}
</style>
