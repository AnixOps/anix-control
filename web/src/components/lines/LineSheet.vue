<template>
  <UiSheet
    :open="open"
    size="md"
    :title="editing ? t('lines.form.editTitle', { name: line.name }) : t('lines.form.createTitle')"
    :description="t('lines.form.description')"
    :dismissible="!saving"
    data-testid="line-form"
    @update:open="value => emit('update:open', value)"
  >
    <form class="line-form" novalidate @submit.prevent="save">
      <div class="line-form__preview">
        <h3 class="line-form__sub">{{ t('lines.form.preview') }}</h3>
        <LinePath v-if="preview" :line="preview" />
        <span v-else class="fwd-muted">{{ t('lines.form.previewEmpty') }}</span>
      </div>

      <UiField :label="t('lines.form.entries')" :help="t('lines.form.entriesHelp')" :error="message('entries')" label-tag="span" required>
        <template #default="{ labelId }">
          <span class="line-form__pick" role="group" :aria-labelledby="labelId" data-testid="line-form-entries">
            <button v-for="device in devices" :key="device.ref" type="button" :aria-pressed="form.entries.includes(device.ref)" @click="toggle('entries', device.ref)">{{ device.name }}</button>
          </span>
        </template>
      </UiField>
      <UiField :label="t('lines.form.via')" :help="t('lines.form.viaHelp')" label-tag="span">
        <template #default="{ labelId }">
          <span class="line-form__pick" role="group" :aria-labelledby="labelId" data-testid="line-form-via">
            <button v-for="device in devices" :key="device.ref" type="button" :aria-pressed="form.via.includes(device.ref)" @click="toggle('via', device.ref)">
              <span v-if="form.via.includes(device.ref)" class="line-form__order">{{ form.via.indexOf(device.ref) + 1 }}</span>{{ device.name }}
            </button>
          </span>
        </template>
      </UiField>
      <UiSelect v-model="form.exit" :options="exitOptions" :label="t('lines.form.exit')" :placeholder="t('lines.form.choose')" :error="message('exit')" required data-testid="line-form-exit" />
      <UiSelect v-model="form.protocol" :options="protocolOptions" :label="t('lines.form.protocol')" :placeholder="t('lines.form.choose')" :help="form.exit && protocolOptions.length === 1 ? t('lines.form.noProtocols') : ''" :error="message('protocol')" required data-testid="line-form-protocol" />
      <UiTextarea v-if="form.protocol === FIXED" v-model="form.targets" :rows="3" :label="t('lines.form.targets')" :help="t('lines.form.targetsHelp')" placeholder="game.example.jp:7777" :error="message('targets')" required />
      <UiTextField v-model="form.name" :label="t('lines.form.name')" :placeholder="t('lines.form.namePlaceholder')" :error="message('name')" required data-testid="line-form-name" />
      <UiNumberField v-model="form.port" :label="t('lines.form.port')" :help="t('lines.form.portHelp')" :min="1" :max="65535" :error="message('port')" required data-testid="line-form-port" />
      <UiField :label="t('lines.form.l4')" :error="message('l4')" label-tag="span">
        <span class="line-form__row">
          <UiCheckbox v-model="form.tcp" label="TCP" />
          <UiCheckbox v-model="form.udp" label="UDP" />
        </span>
      </UiField>
      <UiSwitch v-model="form.enabled" :label="t('lines.form.enabled')" />
      <UiSwitch v-if="form.protocol !== FIXED" v-model="form.show" :label="t('lines.form.show')" :description="t('lines.form.showHelp')" />

      <div class="line-form__more">
        <button type="button" class="line-form__toggle" :aria-expanded="advanced" aria-controls="line-form-advanced" data-testid="line-form-advanced" @click="advanced = !advanced">
          <ChevronRight class="line-form__chevron" :size="16" aria-hidden="true" />{{ t('lines.form.advanced') }}
        </button>
        <div v-show="advanced" id="line-form-advanced" class="line-form__advanced">
          <h3 class="line-form__sub">{{ t('lines.form.limits') }}</h3>
          <div class="line-form__grid">
            <UiNumberField v-model="form.bandwidth" :label="t('lines.form.bandwidth')" unit="Mbps" :min="0" :step="0.1" :placeholder="t('lines.form.unlimited')" />
            <UiNumberField v-model="form.quota" :label="t('lines.form.quota')" unit="GB" :min="0" :step="0.1" :placeholder="t('lines.form.unlimited')" />
            <UiNumberField v-model="form.maxConns" :label="t('lines.form.maxConns')" :min="0" :placeholder="t('lines.form.unlimited')" />
            <UiTextField v-model="form.expires" type="datetime-local" :label="t('lines.form.expires')" :help="t('lines.form.expiresHelp')" />
          </div>
          <h3 class="line-form__sub">{{ t('lines.form.forwarding') }}</h3>
          <UiSelect v-model="form.proxyProtocol" :options="proxyOptions" :label="t('lines.form.proxyProtocol')" :help="t('lines.form.proxyProtocolHelp')" data-testid="line-form-proxy" />
          <UiSelect v-if="form.protocol === FIXED" v-model="form.balance" :options="balanceOptions" :label="t('lines.form.balance')" :help="t('lines.form.balanceHelp')" />
          <UiSwitch v-model="form.healthOff" :label="t('lines.form.healthOff')" :description="t('lines.form.healthOffHelp')" />
          <div class="line-form__grid">
            <UiNumberField v-model="form.interval" :label="t('lines.form.interval')" :unit="t('lines.form.seconds')" :min="0" placeholder="5" :disabled="form.healthOff" />
            <UiNumberField v-model="form.timeout" :label="t('lines.form.timeout')" :unit="t('lines.form.seconds')" :min="0" placeholder="2" :disabled="form.healthOff" :error="message('timeout')" />
            <UiNumberField v-model="form.threshold" :label="t('lines.form.threshold')" :unit="t('lines.form.times')" :min="0" placeholder="3" :help="t('lines.form.thresholdHelp')" />
            <UiNumberField v-model="form.openFor" :label="t('lines.form.openFor')" :unit="t('lines.form.seconds')" :min="0" placeholder="30" :help="t('lines.form.openForHelp')" />
          </div>
        </div>
      </div>
      <div v-if="saveError" class="fwd-note is-danger" role="alert">
        <p>{{ saveError }}</p>
        <ul v-if="violations.length"><li v-for="(item, index) in violations" :key="index">{{ item.message }}</li></ul>
      </div>
    </form>
    <template #footer="{ close }">
      <UiButton :disabled="saving" @click="close">{{ t('forwardV4.common.cancel') }}</UiButton>
      <UiButton variant="primary" :loading="saving" data-testid="line-form-save" @click="save">{{ editing ? t('lines.form.save') : t('lines.form.create') }}</UiButton>
    </template>
  </UiSheet>
</template>

<script setup>
// The line sheet: create a line, or edit or copy one (POST/PUT
// /api/v5/net/lines). Pick the entries, the devices it passes in order and
// the exit, then the exit's protocol or fixed targets; the preview follows.
// A device on a cloud that scans its machines forwards in the kernel (its
// network settings), which the preview's dashed links show.
import { computed, ref, watch } from 'vue'
import { ChevronRight } from '@lucide/vue'
import { UiButton, UiCheckbox, UiField, UiNumberField, UiSelect, UiSheet, UiSwitch, UiTextarea, UiTextField, useToast } from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import { createNetLine, updateNetLine } from '@/api/kernel'
import LinePath from './LinePath.vue'
import { buildLines } from './linesModel'
import { BALANCES, FIXED, PROXY_PROTOCOLS, hasPolicy, lineDraft, lineForm, parseTargets, protocolsOn, validate } from './lineForm'
import '@/components/forward/forward.css'

const props = defineProps({
  open: { type: Boolean, default: false },
  // The line to edit (GET /api/v4/kernel/network/lines), a copy without an
  // id, or null for a new one.
  line: { type: Object, default: null },
  // The network as lines: its devices and lines.
  network: { type: Object, default: null }
})
const emit = defineEmits(['update:open', 'saved'])
const { t } = useAppI18n()
const toast = useToast()

const editing = computed(() => Boolean(props.line?.id))
const form = ref(lineForm(null))
const errors = ref({})
const saving = ref(false)
const saveError = ref('')
const violations = ref([])
// advanced shows the policy and limits; open when the line has any.
const advanced = ref(false)

watch(() => props.open, (value) => {
  if (!value) return
  form.value = lineForm(props.line)
  advanced.value = hasPolicy(form.value)
  errors.value = {}
  saveError.value = ''
  violations.value = []
}, { immediate: true })

const devices = computed(() => props.network?.devices || [])
const exitOptions = computed(() => devices.value.map(device => ({ value: device.ref, label: device.name, description: device.host })))
const protocols = computed(() => protocolsOn(props.network, form.value.exit))
const protocolOptions = computed(() => [
  ...protocols.value.map(item => ({ value: item.id, label: item.type, description: t('lines.form.protocolPort', { port: item.port }) })),
  { value: FIXED, label: t('lines.form.fixed') }
])

const proxyOptions = computed(() => PROXY_PROTOCOLS.map(value => ({ value, label: t(`lines.form.proxy.${value}`) })))
const balanceOptions = computed(() => BALANCES.map(value => ({ value, label: t(`lines.form.balances.${value}`) })))

// A new exit keeps the protocol only while the exit serves it; a protocol
// fills an empty port with its own.
watch(() => form.value.exit, () => {
  if (form.value.protocol !== FIXED && !protocols.value.some(item => item.id === form.value.protocol)) form.value.protocol = ''
})
watch(() => form.value.protocol, (id) => {
  const found = protocols.value.find(item => item.id === id)
  if (found && !form.value.port) form.value.port = found.port
})

function toggle(key, ref) {
  const list = form.value[key]
  form.value[key] = list.includes(ref) ? list.filter(item => item !== ref) : [...list, ref]
}

const preview = computed(() => {
  const draft = form.value
  if (!draft.entries.length || !draft.exit) return null
  const body = lineDraft(draft)
  const protocol = protocols.value.find(item => item.id === draft.protocol)
  return buildLines({
    network: {
      devices: devices.value,
      lines: [{ ...body, id: 'draft', protocol: protocol ? { ...protocol, device: draft.exit } : null, targets: draft.protocol === FIXED ? parseTargets(draft.targets) : [] }]
    }
  })[0]
})

const message = key => (errors.value[key] ? t(`lines.form.errors.${errors.value[key]}`) : '')

async function save() {
  if (saving.value) return
  errors.value = validate(form.value)
  if (errors.value.timeout) advanced.value = true
  if (Object.keys(errors.value).length) return
  const body = lineDraft(form.value, editing.value ? props.line.revision : 0)
  saving.value = true
  saveError.value = ''
  violations.value = []
  try {
    const answer = editing.value ? await updateNetLine(props.line.id, body) : await createNetLine(body)
    toast.success(t(editing.value ? 'lines.toast.saved' : 'lines.toast.created', { name: body.name }))
    emit('saved', answer?.id || props.line?.id || '')
    emit('update:open', false)
  } catch (error) {
    const answer = error?.response?.data?.error
    violations.value = Array.isArray(answer?.violations) ? answer.violations : []
    saveError.value = answer?.code === 'revision_conflict' ? t('lines.form.conflict') : violations.value.length ? t('lines.form.refused') : answer?.message || error?.message || String(error)
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.line-form {
  display: grid;
  gap: var(--space-4);
}

.line-form__preview {
  display: grid;
  gap: var(--space-2);
  padding: var(--space-3);
  border: 1px solid var(--separator);
  border-radius: var(--radius-md);
}

.line-form__sub {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-caption-size);
  font-weight: var(--weight-semibold);
}

.line-form__pick,
.line-form__row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}

.line-form__row {
  gap: var(--space-5);
}

.line-form__pick button {
  display: inline-flex;
  gap: var(--space-1);
  align-items: center;
  min-height: 32px;
  padding: 0 var(--space-3);
  border: 1px solid var(--separator-strong);
  border-radius: var(--radius-pill);
  background: var(--bg-elevated);
  color: var(--label-1);
  font: inherit;
  font-size: var(--type-callout-size);
  cursor: pointer;
}

.line-form__pick button[aria-pressed="true"] {
  border-color: var(--accent);
  background: var(--accent-soft);
  color: var(--label-1);
}

.line-form__pick button:focus-visible {
  outline: var(--focus-ring);
  outline-offset: var(--focus-ring-offset);
}

.line-form__more {
  display: grid;
  gap: var(--space-3);
  padding-top: var(--space-3);
  border-top: 1px solid var(--separator);
}

.line-form__toggle {
  display: inline-flex;
  gap: var(--space-1);
  align-items: center;
  justify-self: start;
  padding: 0;
  border: 0;
  background: none;
  color: var(--label-1);
  font: inherit;
  font-weight: var(--weight-semibold);
  cursor: pointer;
}

.line-form__toggle:focus-visible {
  outline: var(--focus-ring);
  outline-offset: var(--focus-ring-offset);
}

.line-form__chevron {
  transition: transform var(--duration-fast, 120ms) ease;
}

.line-form__toggle[aria-expanded="true"] .line-form__chevron {
  transform: rotate(90deg);
}

.line-form__advanced {
  display: grid;
  gap: var(--space-4);
}

.line-form__grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: var(--space-4);
}

.line-form__order {
  display: inline-grid;
  place-items: center;
  width: 18px;
  height: 18px;
  border-radius: 50%;
  background: var(--accent);
  color: var(--on-accent);
  font-size: var(--type-caption-size);
}
</style>
