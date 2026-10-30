<template>
  <UiSection :title="t('nodeNetwork.title')" :description="t('nodeNetwork.description')" heading-tag="h2" data-testid="node-network">
    <UiSkeleton v-if="loading" variant="card" :label="t('nodeNetwork.loading')" />
    <UiErrorState v-else-if="loadError" :title="t('nodeNetwork.loadFailed')" :error="loadError" @retry="load" />
    <form v-else class="node-network" @submit.prevent="save">
      <UiTextField
        v-model="form.interface"
        :label="t('nodeNetwork.interface')"
        :help="t('nodeNetwork.interfaceHint')"
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
// GET/PUT /api/v4/kernel/nodes/:id/network
import { onMounted, reactive, ref } from 'vue'
import { getKernelNodeNetwork, setKernelNodeNetwork } from '@/api/kernel'
import { useAppI18n } from '@/composables/useAppI18n'
import UiButton from '@/ui/UiButton.vue'
import UiErrorState from '@/ui/UiErrorState.vue'
import UiNumberField from '@/ui/UiNumberField.vue'
import UiSection from '@/ui/UiSection.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import UiTextField from '@/ui/UiTextField.vue'

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

function fill(settings) {
  for (const key of ['interface', 'source_v4', 'gateway_v4', 'source_v6', 'gateway_v6']) form[key] = settings?.[key] || ''
  form.hop_port = settings?.hop_port || null
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
    fill(await setKernelNodeNetwork(props.nodeRef, { ...form, hop_port: form.hop_port || 0 }))
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
