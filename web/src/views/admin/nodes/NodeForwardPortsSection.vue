<template>
  <UiSection
    v-if="!unavailable"
    :title="t('nodeForwardPorts.title')"
    :description="t('nodeForwardPorts.description')"
    heading-tag="h2"
    data-testid="node-forward-ports"
  >
    <UiSkeleton v-if="loading" variant="card" :label="t('nodeForwardPorts.loading')" />
    <UiErrorState v-else-if="loadError" :title="t('nodeForwardPorts.loadFailed')" :error="loadError" @retry="load" />
    <form v-else class="node-ports" novalidate @submit.prevent="save">
      <div class="node-ports__pair">
        <UiNumberField
          v-model="form.first"
          size="md"
          :label="t('nodeForwardPorts.first')"
          :min="1"
          :max="65535"
          :error="fieldError('port_range.first') || rangeError"
          data-testid="node-forward-ports-first"
        />
        <UiNumberField
          v-model="form.last"
          size="md"
          :label="t('nodeForwardPorts.last')"
          :min="1"
          :max="65535"
          :error="fieldError('port_range.last')"
          data-testid="node-forward-ports-last"
        />
      </div>
      <p class="node-ports__hint" data-testid="node-forward-ports-source">{{ sourceHint }}</p>
      <UiTextField
        v-model="form.reserved"
        size="md"
        :label="t('nodeForwardPorts.reserved')"
        placeholder="22, 80, 443"
        :help="t('nodeForwardPorts.reservedHelp')"
        :error="reservedError || fieldError('reserved_ports')"
        data-testid="node-forward-ports-reserved"
      />
      <ul v-if="routeViolations.length" class="node-ports__warnings" role="status" data-testid="node-forward-ports-violations">
        <li v-for="(item, index) in routeViolations" :key="index">{{ violationText(t, te, item) }}</li>
      </ul>
      <p v-if="saveError" class="node-ports__error" role="alert">{{ saveError }}</p>
      <p v-if="saved" class="node-ports__hint" role="status">{{ t('nodeForwardPorts.saved') }}</p>
      <div>
        <UiButton type="submit" :loading="saving" :disabled="saving || !dirty" data-testid="node-forward-ports-save">{{ t('nodeForwardPorts.save') }}</UiButton>
      </div>
    </form>
  </UiSection>
</template>

<script setup>
// 转发端口 on the node page's 协议 tab: the port range the forward planner
// allocates route and hop listen ports from on this node, and the ports it
// never takes. Before, the range could only be set on the forwarding area's
// node page, which the node page no longer links to, so every node ran on
// the planner's built-in range. The form shows the range in force (the
// node's own, else the one the inventory reports) as values, not as a
// placeholder, and saving keeps it as the node's own.
// GET /api/v4/forward/nodes/:ref, PUT /api/v4/forward/nodes/:ref/settings;
// the node's addresses and labels are sent back unchanged. The section hides
// itself when Control has no forwarding (404 for the API or the node).
import { computed, onMounted, reactive, ref } from 'vue'
import { getNode, newIdempotencyKey, setNodeSettings } from '@/api/forwardV4'
import { forwardErrorMessage, violationText } from '@/components/forward/messages'
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
const { t, te } = useAppI18n()

const form = reactive({ first: null, last: null, reserved: '' })
const node = ref(null)
const base = ref('')
const loading = ref(true)
const loadError = ref(null)
const unavailable = ref(false)
const saving = ref(false)
const saveError = ref('')
const saved = ref(false)
const violations = ref([])
const reservedError = ref('')
let idempotencyKey = newIdempotencyKey()

const num = value => (value === undefined || value === null || value === '' ? 0 : Number(value) || 0)
const own = computed(() => node.value?.settings?.port_range || null)
const inForce = computed(() => node.value?.info?.port_range || null)

const sourceHint = computed(() => {
  if (num(own.value?.first)) return t('nodeForwardPorts.sourceOwn')
  if (num(inForce.value?.first)) {
    return t('nodeForwardPorts.sourceBuiltIn', { first: num(inForce.value.first), last: num(inForce.value.last) })
  }
  return t('nodeForwardPorts.sourceNone')
})

const rangeError = computed(() => {
  const first = num(form.first)
  const last = num(form.last)
  if (!first && !last) return ''
  if (!first || !last) return t('nodeForwardPorts.bothEnds')
  if (first > last) return t('nodeForwardPorts.reversed')
  return ''
})

function parseReserved(text) {
  const parts = String(text || '').split(/[\s,，]+/).filter(Boolean)
  const ports = parts.map(Number)
  return ports.every(port => Number.isInteger(port) && port >= 1 && port <= 65535) ? ports : null
}

function snapshot() {
  return JSON.stringify([num(form.first), num(form.last), parseReserved(form.reserved)])
}
const dirty = computed(() => Boolean(base.value) && snapshot() !== base.value)

function fill(summary) {
  node.value = summary || null
  const range = own.value || inForce.value
  form.first = num(range?.first) || null
  form.last = num(range?.last) || null
  form.reserved = (summary?.settings?.reserved_ports || []).map(num).filter(Boolean).join(', ')
  // A range shown from the inventory is not the node's own yet: saving
  // keeps it, so the form starts dirty only when the node has none.
  base.value = own.value ? snapshot() : JSON.stringify([0, 0, parseReserved(form.reserved)])
}

function fieldError(field) {
  return violations.value.filter(v => v.field === field && !v.route_id).map(v => violationText(t, te, v)).join('; ')
}
const routeViolations = computed(() => violations.value.filter(v => v.route_id || !v.field))

async function load() {
  loading.value = true
  loadError.value = null
  try {
    const answer = await getNode(props.nodeRef)
    fill(answer?.node)
  } catch (error) {
    if (error?.status === 404) {
      unavailable.value = true
    } else {
      loadError.value = new Error(forwardErrorMessage(t, error))
    }
  } finally {
    loading.value = false
  }
}

async function save() {
  saveError.value = ''
  saved.value = false
  const reserved = parseReserved(form.reserved)
  reservedError.value = reserved ? '' : t('nodeForwardPorts.reservedInvalid')
  if (!reserved || rangeError.value) return
  const current = node.value?.settings || {}
  const first = num(form.first)
  const last = num(form.last)
  const settings = {
    port_range: first && last ? { first, last } : undefined,
    reserved_ports: reserved,
    addresses: current.addresses || [],
    labels: current.labels || {}
  }
  saving.value = true
  violations.value = []
  try {
    const answer = await setNodeSettings(props.nodeRef, settings, { idempotencyKey })
    idempotencyKey = newIdempotencyKey()
    violations.value = answer?.violations || []
    if (answer?.node) fill(answer.node)
    else await load()
    saved.value = true
  } catch (error) {
    violations.value = error?.violations || []
    saveError.value = forwardErrorMessage(t, error)
  } finally {
    saving.value = false
  }
}

onMounted(load)

defineExpose({ load, form })
</script>

<style scoped>
.node-ports {
  display: grid;
  gap: var(--space-4);
}

.node-ports__pair {
  display: grid;
  gap: var(--space-4);
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
}

.node-ports__hint {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.node-ports__error {
  margin: 0;
  color: color-mix(in srgb, var(--danger) 78%, var(--label-1));
  font-size: var(--type-callout-size);
}

.node-ports__warnings {
  margin: 0;
  padding-left: var(--space-5);
  color: color-mix(in srgb, var(--warning, var(--danger)) 78%, var(--label-1));
  font-size: var(--type-callout-size);
}
</style>
