<template>
  <div class="dns-picker" data-testid="forward-dns-picker" data-field="dns">
    <div class="dns-picker__head">
      <UiSwitch
        v-if="!stored"
        :model-value="modelValue.enabled"
        :label="t('forwardDns.binding.enable')"
        :description="t('forwardDns.binding.enableHelp')"
        data-testid="forward-dns-enable"
        @update:model-value="enable"
      />
      <template v-else>
        <span class="dns-picker__title">{{ t('forwardDns.binding.bound') }}</span>
        <UiBadge v-if="modelValue.paused" tone="neutral" :label="t('forwardDns.state.paused')" />
      </template>
    </div>

    <template v-if="modelValue.enabled">
      <p v-if="loadError" class="fwd-note is-danger" role="alert">{{ loadError }}</p>
      <p v-else-if="loaded && !providers.length && !stored" class="fwd-note" role="status" data-testid="forward-dns-no-providers">
        {{ t('forwardDns.binding.noProviders') }}
        <RouterLink class="fwd-link" to="/admin/forward/dns">{{ t('forwardDns.binding.manageProviders') }}</RouterLink>
      </p>

      <UiGroupedList v-if="stored" class="dns-picker__summary" data-testid="forward-dns-identity">
        <UiGroupedListRow :label="t('forwardDns.binding.provider')" :value="providerLabel" />
        <UiGroupedListRow :label="t('forwardDns.binding.zone')" :value="stored.zone || '—'" />
        <UiGroupedListRow :label="t('forwardDns.binding.mode')" :value="t(`forwardDns.mode.${stored.mode || 'DNS_BINDING_MODE_DDNS'}`)" />
        <UiGroupedListRow :label="cname ? t('forwardDns.binding.recordName') : t('forwardDns.binding.record')" :value="stored.record_name || '—'" />
      </UiGroupedList>
      <template v-else>
        <div class="dns-picker__grid">
          <UiSelect
            :id="fieldId('dns.provider_id')"
            :model-value="modelValue.provider_id"
            size="md"
            :label="t('forwardDns.binding.provider')"
            :placeholder="t('forwardDns.binding.providerPlaceholder')"
            :options="providerOptions"
            :error="errors.provider_id"
            required
            @update:model-value="value => patch({ provider_id: value })"
          />
          <UiTextField
            :id="fieldId('dns.zone')"
            :model-value="modelValue.zone"
            size="md"
            :label="t('forwardDns.binding.zone')"
            placeholder="example.com"
            :help="t('forwardDns.binding.zoneHelp')"
            :error="errors.zone"
            required
            @update:model-value="value => patch({ zone: value })"
          />
        </div>

        <UiRadioGroup
          :id="fieldId('dns.mode')"
          :model-value="modelValue.mode"
          :label="t('forwardDns.binding.mode')"
          orientation="horizontal"
          :options="modeOptions"
          @update:model-value="value => patch({ mode: value })"
        />

        <UiTextField
          v-if="cname"
          :id="fieldId('dns.record_name')"
          :model-value="modelValue.record_name"
          size="md"
          :label="t('forwardDns.binding.recordName')"
          :placeholder="t('forwardDns.binding.recordNamePlaceholder')"
          :help="t('forwardDns.binding.recordNameHelp')"
          :error="errors.record_name"
          required
          @update:model-value="value => patch({ record_name: value })"
        />
        <p v-else class="fwd-muted dns-picker__line">
          {{ t('forwardDns.binding.ddnsRecord') }} <span class="fwd-mono">{{ recordName || '—' }}</span>
        </p>
      </template>

      <div class="dns-picker__grid">
        <UiField :label="t('forwardDns.binding.recordTypes')" label-tag="span" :error="errors.record_types" :help="t('forwardDns.binding.recordTypesHelp')" data-field="dns.record_types">
          <template #default="{ labelId }">
            <span class="dns-picker__types" role="group" :aria-labelledby="labelId">
              <UiCheckbox
                v-for="type in RECORD_TYPES"
                :key="type"
                :model-value="modelValue.record_types.includes(type)"
                :label="recordTypeLabel(type)"
                :description="t(`forwardDns.binding.type.${recordTypeLabel(type)}`)"
                @update:model-value="value => toggleType(type, value)"
              />
            </span>
          </template>
        </UiField>
        <UiNumberField
          :id="fieldId('dns.ttl')"
          :model-value="modelValue.ttl"
          size="md"
          :label="t('forwardDns.binding.ttl')"
          :unit="t('forwardDns.binding.seconds')"
          :min="1"
          :max="MAX_TTL"
          :placeholder="String(DEFAULT_TTL)"
          :help="t('forwardDns.binding.ttlHelp')"
          :error="errors.ttl"
          @update:model-value="value => patch({ ttl: value })"
        />
      </div>

      <UiSwitch
        :model-value="modelValue.paused"
        :label="t('forwardDns.binding.paused')"
        :description="t('forwardDns.binding.pausedHelp')"
        @update:model-value="value => patch({ paused: value })"
      />

      <div v-if="cname && modelValue.record_name" class="dns-picker__cname" data-testid="forward-dns-cname">
        <p class="dns-picker__line">{{ t('forwardDns.binding.cnameInstruction', { host: hostname || '—' }) }}</p>
        <UiCopyField :value="String(modelValue.record_name).trim()" size="md" :label="t('forwardDns.binding.cnameTarget')" :copy-label="t('forwardDns.binding.copy')" />
      </div>

      <p v-if="stored" class="fwd-muted dns-picker__line">{{ t('forwardDns.binding.immutable') }}</p>
      <p v-if="mismatch" class="fwd-note is-warning" role="status" data-testid="forward-dns-mismatch">{{ t('forwardDns.binding.mismatch', { name: stored.record_name }) }}</p>
      <p v-if="errors.hostname" class="fwd-note is-warning" role="status">{{ errors.hostname }}</p>
      <p class="fwd-muted dns-picker__line">
        <RouterLink class="fwd-link" to="/admin/forward/dns">{{ t('forwardDns.binding.manageProviders') }}</RouterLink>
      </p>
    </template>
  </div>
</template>

<script setup>
// The entry hostname's DNS binding in the route editor (L2, D14): with a
// provider, Control keeps the hostname's A/AAAA records on the healthy
// entries. The editor writes it after the route is saved (the binding
// needs the stored route and its entry_hostname): POST for a new one, the
// whole binding on PUT for a change. Provider, zone, name and mode cannot
// change once bound (delete with purge and bind again, on the route page).
import { computed, onMounted, ref } from 'vue'
import { UiBadge, UiCheckbox, UiCopyField, UiField, UiGroupedList, UiGroupedListRow, UiNumberField, UiRadioGroup, UiSelect, UiSwitch, UiTextField } from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import { listDnsKinds, listDnsProviders } from '@/api/forwardV4'
import { DEFAULT_TTL, DNS_MODES, MAX_TTL, RECORD_TYPES, kindName, recordTypeLabel, zoneOf } from './dnsModel'
import { fieldId } from './routeModel'
import { forwardErrorMessage } from './messages'
import './forward.css'

const props = defineProps({
  modelValue: { type: Object, required: true },
  // The stored binding, or null.
  stored: { type: Object, default: null },
  hostname: { type: String, default: '' },
  errors: { type: Object, default: () => ({}) }
})
const emit = defineEmits(['update:modelValue'])
const { t } = useAppI18n()

const providers = ref([])
const kinds = ref([])
const loaded = ref(false)
const loadError = ref('')

onMounted(async () => {
  try {
    const [providerList, kindList] = await Promise.all([listDnsProviders(), listDnsKinds().catch(() => [])])
    providers.value = providerList
    kinds.value = kindList
  } catch (error) {
    loadError.value = forwardErrorMessage(t, error)
  } finally {
    loaded.value = true
  }
})

const cname = computed(() => props.modelValue.mode === 'DNS_BINDING_MODE_CNAME')
const recordName = computed(() => String(props.hostname || '').trim())
const providerOptions = computed(() => {
  const options = providers.value.map(provider => ({ value: String(provider.id), label: `${provider.name} · ${kindName(kinds.value, provider.kind)}` }))
  const id = String(props.stored?.provider_id || '')
  if (id && !options.some(option => option.value === id)) options.push({ value: id, label: `#${id}` })
  return options
})
const providerLabel = computed(() => providerOptions.value.find(option => option.value === String(props.stored?.provider_id || ''))?.label || '—')
const modeOptions = computed(() => DNS_MODES.map(value => ({ value, label: t(`forwardDns.mode.${value}`), description: t(`forwardDns.modeHint.${value}`) })))
const mismatch = computed(() => Boolean(props.stored) && props.stored.mode !== 'DNS_BINDING_MODE_CNAME' &&
  recordName.value.replace(/\.$/, '').toLowerCase() !== String(props.stored.record_name || '').replace(/\.$/, '').toLowerCase())

function patch(change) {
  emit('update:modelValue', { ...props.modelValue, ...change })
}

// Turning the binding on guesses the zone from the hostname.
function enable(value) {
  const change = { enabled: value }
  if (value && !String(props.modelValue.zone || '').trim()) change.zone = zoneOf(props.hostname)
  patch(change)
}

function toggleType(type, on) {
  const set = new Set(props.modelValue.record_types)
  if (on) set.add(type)
  else set.delete(type)
  patch({ record_types: RECORD_TYPES.filter(item => set.has(item)) })
}
</script>

<style scoped>
.dns-picker {
  display: grid;
  gap: var(--space-3);
  min-width: 0;
  margin-top: var(--space-3);
  padding: var(--space-3) var(--space-4);
  border: 1px solid var(--separator);
  border-radius: var(--radius-sm);
  background: var(--bg-grouped);
}

.dns-picker__head {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
}

.dns-picker__title {
  font-weight: var(--weight-semibold);
}

.dns-picker__grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--space-3);
  align-items: start;
}

.dns-picker__types {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2) var(--space-5);
}

.dns-picker__line {
  margin: 0;
  overflow-wrap: anywhere;
}

.dns-picker__cname {
  display: grid;
  gap: var(--space-2);
  padding: var(--space-3);
  border-radius: var(--radius-xs);
  background: var(--fill-1);
}

@media (max-width: 639.98px) {
  .dns-picker__grid {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
