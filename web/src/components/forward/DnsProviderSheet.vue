<template>
  <UiSheet
    :open="open"
    size="md"
    :title="editing ? t('forwardDns.sheet.editTitle', { name: provider.name }) : t('forwardDns.sheet.createTitle')"
    :description="t('forwardDns.sheet.description')"
    :dismissible="!saving"
    @update:open="value => emit('update:open', value)"
  >
    <form class="dns-sheet" novalidate data-testid="forward-dns-provider-form" @submit.prevent="save">
      <UiTextField v-model="form.name" size="md" :label="t('forwardDns.sheet.name')" required :placeholder="t('forwardDns.sheet.namePlaceholder')" :error="errors.name" />
      <UiRadioGroup
        :model-value="form.kind"
        :label="t('forwardDns.sheet.kind')"
        :options="kindOptions"
        :disabled="editing"
        :help="editing ? t('forwardDns.sheet.kindFixed') : ''"
        :error="errors.kind"
        required
        data-testid="forward-dns-kind"
        @update:model-value="changeKind"
      />

      <fieldset v-if="spec?.config?.length" class="dns-sheet__group">
        <legend class="dns-sheet__legend">{{ t('forwardDns.sheet.settings') }}</legend>
        <UiTextField
          v-for="field in spec.config"
          :key="`config-${field}`"
          v-model="form.config[field]"
          size="md"
          :data-config-field="field"
          :label="fieldLabel('config', field)"
          :required="required.has(field)"
          :placeholder="fieldPlaceholder(field)"
          :help="fieldHelp(field)"
          :error="errors[`config.${field}`]"
        />
      </fieldset>

      <fieldset v-if="spec?.credentials?.length" class="dns-sheet__group">
        <legend class="dns-sheet__legend">{{ t('forwardDns.sheet.credentials') }}</legend>
        <p class="fwd-muted dns-sheet__note">{{ editing ? t('forwardDns.sheet.credentialsKeep') : t('forwardDns.sheet.credentialsHelp') }}</p>
        <UiPasswordField
          v-for="field in spec.credentials"
          :key="`credential-${field}`"
          v-model="form.credentials[field]"
          size="md"
          autocomplete="new-password"
          :data-credential-field="field"
          :label="fieldLabel('credentials', field)"
          :required="!isStored(field)"
          :help="isStored(field) ? t('forwardDns.sheet.stored') : ''"
          :error="errors[`credentials.${field}`]"
          @focus="clearPlaceholder(field)"
          @blur="restorePlaceholder(field)"
        />
      </fieldset>

      <p v-if="forbidden" class="fwd-note is-warning" role="alert" data-testid="forward-dns-forbidden">{{ t('forwardDns.superOnly') }}</p>
      <p v-else-if="saveError" class="fwd-note is-danger" role="alert">{{ saveError }}</p>
    </form>
    <template #footer="{ close }">
      <UiButton :disabled="saving" @click="close">{{ t('forwardV4.common.cancel') }}</UiButton>
      <UiButton variant="primary" :loading="saving" :disabled="forbidden" data-testid="forward-dns-provider-save" @click="save">{{ editing ? t('forwardDns.sheet.save') : t('forwardDns.sheet.create') }}</UiButton>
    </template>
  </UiSheet>
</template>

<script setup>
// The provider sheet (L2): its fields come from GET /dns/kinds (config,
// required_config, credentials). Credentials are write-only: a stored one
// shows as ******** and is left out of the write unless replaced, so it
// keeps its value. One Idempotency-Key per attempt, reused for a retry of
// the same body. Writes need a super administrator (403 shown inline).
import { computed, ref, watch } from 'vue'
import { UiButton, UiPasswordField, UiRadioGroup, UiSheet, UiTextField, useToast } from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import { createDnsProvider, newIdempotencyKey, updateDnsProvider } from '@/api/forwardV4'
import { CREDENTIAL_PLACEHOLDER, dnsErrorMessage, providerErrors, providerForm, providerRequest, switchKind } from './dnsModel'
import './forward.css'

const props = defineProps({
  open: { type: Boolean, default: false },
  // null for a new provider.
  provider: { type: Object, default: null },
  kinds: { type: Array, default: () => [] }
})
const emit = defineEmits(['update:open', 'saved', 'forbidden'])
const { t, te } = useAppI18n()
const toast = useToast()

const editing = computed(() => Boolean(props.provider?.id))
const form = ref(providerForm(null, props.kinds))
const errors = ref({})
const saving = ref(false)
const saveError = ref('')
const forbidden = ref(false)
const attempt = { key: newIdempotencyKey(), body: null }

watch(() => props.open, value => {
  if (!value) return
  form.value = providerForm(props.provider, props.kinds)
  errors.value = {}
  saveError.value = ''
  forbidden.value = false
  attempt.key = newIdempotencyKey()
  attempt.body = null
}, { immediate: true })

const spec = computed(() => props.kinds.find(item => item.kind === form.value.kind) || null)
const required = computed(() => new Set(spec.value?.required_config || []))
// Every kind with what its credential needs: the choice is made once.
const kindOptions = computed(() => props.kinds.map(item => {
  const key = `forwardDns.kindHelp.${item.kind}`
  return { value: item.kind, label: item.name || item.kind, description: te(key) ? t(key) : '' }
}))

function changeKind(kind) {
  form.value = switchKind(form.value, kind, props.kinds)
  errors.value = {}
}

function fieldLabel(group, field) {
  const key = `forwardDns.${group === 'config' ? 'config' : 'credential'}.${field}`
  return te(key) ? t(key) : field
}
function fieldHelp(field) {
  const key = `forwardDns.configHelp.${field}`
  return te(key) ? t(key) : ''
}
function fieldPlaceholder(field) {
  const key = `forwardDns.configPlaceholder.${form.value.kind}.${field}`
  return te(key) ? t(key) : ''
}

const isStored = field => editing.value && form.value.stored.includes(field)

// Focusing a stored credential clears the placeholder so a new value is
// typed fresh; leaving it empty puts the placeholder back (keep).
function clearPlaceholder(field) {
  if (form.value.credentials[field] === CREDENTIAL_PLACEHOLDER) form.value.credentials[field] = ''
}
function restorePlaceholder(field) {
  if (isStored(field) && !form.value.credentials[field]) form.value.credentials[field] = CREDENTIAL_PLACEHOLDER
}

async function save() {
  if (saving.value || forbidden.value) return
  errors.value = providerErrors(form.value, props.kinds, t('forwardV4.codes.required'))
  if (Object.keys(errors.value).length) return
  const { provider, credentials } = providerRequest(form.value, props.kinds)
  const body = JSON.stringify({ provider, credentials })
  if (attempt.body !== null && attempt.body !== body) attempt.key = newIdempotencyKey()
  attempt.body = body
  saving.value = true
  saveError.value = ''
  try {
    const answer = editing.value
      ? await updateDnsProvider(props.provider.id, provider, credentials, { idempotencyKey: attempt.key })
      : await createDnsProvider(provider, credentials, { idempotencyKey: attempt.key })
    attempt.key = newIdempotencyKey()
    attempt.body = null
    toast.success(t(editing.value ? 'forwardDns.toast.saved' : 'forwardDns.toast.created', { name: provider.name }))
    emit('saved', answer?.provider || null)
    emit('update:open', false)
  } catch (error) {
    if (error.status === 403 || error.code === 'super_admin_required') {
      forbidden.value = true
      emit('forbidden')
    } else {
      saveError.value = dnsErrorMessage(t, te, error)
      for (const violation of error.violations || []) {
        const field = String(violation.field || '').replace(/^provider\./, '')
        if (field && !errors.value[field]) errors.value = { ...errors.value, [field]: violation.message || violation.code }
      }
    }
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.dns-sheet {
  display: grid;
  gap: var(--space-4);
}

.dns-sheet__group {
  display: grid;
  gap: var(--space-3);
  min-width: 0;
  margin: 0;
  padding: 0;
  border: 0;
}

.dns-sheet__legend {
  margin-bottom: var(--space-1);
  padding: 0;
  color: var(--label-1);
  font-size: var(--type-callout-size);
  font-weight: var(--weight-semibold);
}

.dns-sheet__note {
  margin: 0;
}
</style>
