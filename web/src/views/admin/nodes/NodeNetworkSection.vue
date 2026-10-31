<template>
  <UiSection :title="t('nodeNetwork.title')" :description="t('nodeNetwork.description')" heading-tag="h2" data-testid="node-network">
    <UiSkeleton v-if="loading" variant="card" :label="t('nodeNetwork.loading')" />
    <UiErrorState v-else-if="loadError" :title="t('nodeNetwork.loadFailed')" :error="loadError" @retry="load" />
    <form v-else class="node-network" @submit.prevent="save">
      <template v-if="picking">
        <UiSelect
          :model-value="form.interface || NONE"
          :options="interfaceOptions"
          :label="t('nodeNetwork.interface')"
          :help="detectedHint"
          :error="fieldError('interface')"
          data-testid="node-network-interface"
          @update:model-value="pickInterface"
        />
        <div v-if="form.interface" class="node-network__pair">
          <UiSelect v-model="sourceV4" :options="choices('addresses_v4', form.source_v4)" :label="t('nodeNetwork.sourceV4')" :error="fieldError('source_v4')" data-testid="node-network-source-v4" />
          <UiSelect v-model="gatewayV4" :options="choices('gateway_v4', form.gateway_v4)" :label="t('nodeNetwork.gatewayV4')" :error="fieldError('gateway_v4')" data-testid="node-network-gateway-v4" />
        </div>
        <div v-if="form.interface" class="node-network__pair">
          <UiSelect v-model="sourceV6" :options="choices('addresses_v6', form.source_v6)" :label="t('nodeNetwork.sourceV6')" :error="fieldError('source_v6')" data-testid="node-network-source-v6" />
          <UiSelect v-model="gatewayV6" :options="choices('gateway_v6', form.gateway_v6)" :label="t('nodeNetwork.gatewayV6')" :error="fieldError('gateway_v6')" data-testid="node-network-gateway-v6" />
        </div>
        <div>
          <UiButton variant="tertiary" size="sm" type="button" data-testid="node-network-manual" @click="manual = true">{{ t('nodeNetwork.manual') }}</UiButton>
        </div>
      </template>
      <template v-else>
        <UiTextField
          v-model="form.interface"
          :label="t('nodeNetwork.interface')"
          :help="detected.length ? t('nodeNetwork.interfaceHint') : t('nodeNetwork.notDetected')"
          placeholder="eth0"
          :error="fieldError('interface')"
          data-testid="node-network-interface"
        />
        <div class="node-network__pair">
          <UiTextField v-model="form.source_v4" :label="t('nodeNetwork.sourceV4')" placeholder="10.0.0.7" :error="fieldError('source_v4')" data-testid="node-network-source-v4" />
          <UiTextField v-model="form.gateway_v4" :label="t('nodeNetwork.gatewayV4')" placeholder="10.0.0.1" :error="fieldError('gateway_v4')" data-testid="node-network-gateway-v4" />
        </div>
        <div class="node-network__pair">
          <UiTextField v-model="form.source_v6" :label="t('nodeNetwork.sourceV6')" :error="fieldError('source_v6')" data-testid="node-network-source-v6" />
          <UiTextField v-model="form.gateway_v6" :label="t('nodeNetwork.gatewayV6')" :error="fieldError('gateway_v6')" data-testid="node-network-gateway-v6" />
        </div>
        <div v-if="detected.length">
          <UiButton variant="tertiary" size="sm" type="button" data-testid="node-network-pick" @click="manual = false">{{ t('nodeNetwork.pick') }}</UiButton>
        </div>
      </template>
      <UiNumberField
        v-model="form.hop_port"
        :label="t('nodeNetwork.hopPort')"
        :help="t('nodeNetwork.hopPortHint')"
        :min="0"
        :max="65535"
        placeholder="24443"
        :error="fieldError('hop_port')"
        data-testid="node-network-hop-port"
      />
      <NodeUplinksEditor v-model="uplinks" :hops="hops" :detected="detected" :field-error="fieldError" />
      <p v-if="saveError" class="node-network__error" role="alert">{{ saveError }}</p>
      <p v-if="saved" class="node-network__hint" role="status">{{ t('nodeNetwork.saved') }}</p>
      <div>
        <UiButton type="submit" :loading="saving" :disabled="saving" data-testid="node-network-save">{{ t('nodeNetwork.save') }}</UiButton>
      </div>
    </form>
  </UiSection>
</template>

<script setup>
// 网络 on the node page's 协议 tab: which interface, source address and
// gateway the network model's automatic egresses of the node leave by, and
// the port its automatic hop ingress listens on, for NAT and multi-homed
// machines where the default route is wrong. A NAT node is reached at the
// NAT entry (入口) whose port map covers the hop port.
// The interfaces the node's Agent reported (detected) are offered as
// choices: picking one fills its first address and its gateways; 手动填写
// switches to free text, which is all there is before the Agent reports.
// 上联网卡 (NodeUplinksEditor) lists the NICs of a dual-NIC node and which
// one each next node of its routes gets, with each NIC's latency to it.
// GET/PUT /api/v4/kernel/nodes/:id/network
import { computed, onMounted, reactive, ref } from 'vue'
import { getKernelNodeNetwork, setKernelNodeNetwork } from '@/api/kernel'
import { useAppI18n } from '@/composables/useAppI18n'
import UiButton from '@/ui/UiButton.vue'
import UiErrorState from '@/ui/UiErrorState.vue'
import UiNumberField from '@/ui/UiNumberField.vue'
import UiSection from '@/ui/UiSection.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import UiTextField from '@/ui/UiTextField.vue'
import NodeUplinksEditor from './NodeUplinksEditor.vue'

const props = defineProps({
  nodeRef: { type: String, required: true }
})
const { t } = useAppI18n()

const form = reactive({ interface: '', source_v4: '', gateway_v4: '', source_v6: '', gateway_v6: '', hop_port: null })
const loading = ref(true)
const loadError = ref(null)
const saving = ref(false)
const saveError = ref('')
const saved = ref(false)
const violations = ref([])
const detected = ref([])
const detectedAt = ref('')
const manual = ref(false)
const uplinks = ref({ uplinks: [], uplink_default: '', uplink_choices: {} })
const hops = ref([])

// NONE stands for an empty field in a select, which cannot hold ''.
const NONE = '__none__'
const FIELDS = ['interface', 'source_v4', 'gateway_v4', 'source_v6', 'gateway_v6']

const picking = computed(() => detected.value.length > 0 && !manual.value)
const current = computed(() => detected.value.find(i => i.name === form.interface))
const detectedHint = computed(() => detectedAt.value
  ? t('nodeNetwork.detectedAt', { time: new Date(detectedAt.value).toLocaleString() })
  : t('nodeNetwork.interfaceHint'))

const interfaceOptions = computed(() => {
  const options = [{ value: NONE, label: t('nodeNetwork.defaultRoute') }]
  for (const iface of detected.value) {
    options.push({ value: iface.name, label: iface.name, description: [...iface.addresses_v4, ...iface.addresses_v6].join(', ') })
  }
  if (form.interface && !current.value) options.push({ value: form.interface, label: form.interface })
  return options
})

// choices answers a source or gateway select's options: none, what the
// chosen interface has, and the saved value when it is not among them.
function choices(key, value) {
  const found = current.value?.[key]
  const values = Array.isArray(found) ? [...found] : (found ? [found] : [])
  if (value && !values.includes(value)) values.push(value)
  return [{ value: NONE, label: t('nodeNetwork.none') }, ...values]
}

function field(key) {
  return computed({
    get: () => form[key] || NONE,
    set: value => { form[key] = value === NONE ? '' : value }
  })
}
const sourceV4 = field('source_v4')
const gatewayV4 = field('gateway_v4')
const sourceV6 = field('source_v6')
const gatewayV6 = field('gateway_v6')

function pickInterface(value) {
  const iface = detected.value.find(i => i.name === value)
  form.interface = value === NONE ? '' : value
  form.source_v4 = iface?.addresses_v4[0] || ''
  form.gateway_v4 = (form.source_v4 && iface?.gateway_v4) || ''
  form.source_v6 = iface?.addresses_v6[0] || ''
  form.gateway_v6 = (form.source_v6 && iface?.gateway_v6) || ''
}

function fill(view) {
  for (const key of FIELDS) form[key] = view?.[key] || ''
  form.hop_port = view?.hop_port || null
  detected.value = Array.isArray(view?.detected) ? view.detected : []
  detectedAt.value = view?.detected_at || ''
  uplinks.value = {
    uplinks: Array.isArray(view?.uplinks) ? view.uplinks.map(u => ({ label: '', source_v4: '', gateway_v4: '', source_v6: '', gateway_v6: '', ...u })) : [],
    uplink_default: view?.uplink_default || '',
    uplink_choices: view?.uplink_choices || {}
  }
  hops.value = Array.isArray(view?.uplink_hops) ? view.uplink_hops : []
}

function fieldError(field) {
  return violations.value.filter(v => v.field === field).map(v => v.message).join('; ')
}

async function load() {
  loading.value = true
  loadError.value = null
  try {
    fill(await getKernelNodeNetwork(props.nodeRef))
  } catch (e) {
    console.error('Failed to load node network settings:', e)
    loadError.value = e
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  saveError.value = ''
  saved.value = false
  violations.value = []
  try {
    const settings = Object.fromEntries(FIELDS.map(key => [key, form[key]]))
    fill(await setKernelNodeNetwork(props.nodeRef, { ...settings, hop_port: form.hop_port || 0, ...uplinks.value }))
    saved.value = true
  } catch (e) {
    const error = e?.response?.data?.error
    violations.value = Array.isArray(error?.violations) ? error.violations : []
    saveError.value = violations.value.length ? t('nodeNetwork.invalid') : (error?.message || e?.message || t('nodeNetwork.saveFailed'))
  } finally {
    saving.value = false
  }
}

onMounted(load)

defineExpose({ load, form })
</script>

<style scoped>
.node-network {
  display: grid;
  gap: var(--space-4);
}

.node-network__pair {
  display: grid;
  gap: var(--space-4);
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
}

.node-network__error {
  margin: 0;
  color: color-mix(in srgb, var(--danger) 78%, var(--label-1));
  font-size: var(--type-callout-size);
}

.node-network__hint {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}
</style>
