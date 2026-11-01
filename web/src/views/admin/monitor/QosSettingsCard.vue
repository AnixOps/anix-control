<template>
  <UiCard :title="t('adminMonitor.categories.qos.title')" :description="t('adminMonitor.categories.qos.description')" as="section" data-qos-settings>
    <LoadError v-if="loadError" :title="t('adminMonitor.categories.qos.loadFailed')" :error="loadError" compact @retry="load" />
    <form v-else class="qos-form" @submit.prevent="save">
      <UiSwitch v-model="form.enabled" :label="t('adminMonitor.categories.qos.enabled')" :description="t('adminMonitor.categories.qos.enabledHint')" :disabled="!loaded" data-qos-enabled />

      <fieldset class="qos-form__group" :disabled="!loaded">
        <legend class="qos-form__legend">{{ t('adminMonitor.categories.qos.priorities') }}</legend>
        <div class="qos-form__grid">
          <UiSelect
            v-for="category in PRIORITY_CATEGORIES"
            :key="category"
            v-model="form.priorities[category]"
            :label="categoryLabel(category)"
            :options="priorityOptions"
            size="md"
            :data-qos-priority="category"
          />
        </div>
      </fieldset>

      <div class="qos-form__grid">
        <UiNumberField v-model="form.default_egress_mbps" :label="t('adminMonitor.categories.qos.defaultEgress')" :help="t('adminMonitor.categories.qos.defaultEgressHelp')" unit="Mbps" :min="0" :max="1000000" size="md" :disabled="!loaded" />
        <UiNumberField v-model="form.min_share_percent" :label="t('adminMonitor.categories.qos.minShare')" :help="t('adminMonitor.categories.qos.minShareHelp')" unit="%" :min="1" :max="90" size="md" :disabled="!loaded" />
      </div>

      <fieldset v-if="nodes.length" class="qos-form__group" :disabled="!loaded">
        <legend class="qos-form__legend">{{ t('adminMonitor.categories.qos.nodeEgress') }}</legend>
        <p class="qos-form__hint">{{ t('adminMonitor.categories.qos.nodeEgressHelp') }}</p>
        <div class="qos-form__grid">
          <UiTextField
            v-for="node in nodes"
            :key="node.id"
            v-model="form.node_egress[node.id]"
            :label="node.name || `#${node.id}`"
            :placeholder="t('adminMonitor.categories.qos.useDefault')"
            suffix="Mbps"
            inputmode="numeric"
            size="md"
          />
        </div>
      </fieldset>

      <fieldset class="qos-form__group" :disabled="!loaded">
        <legend class="qos-form__legend">{{ t('adminMonitor.categories.qos.caps') }}</legend>
        <p class="qos-form__hint">{{ t('adminMonitor.categories.qos.capsHelp') }}</p>
        <div v-for="(cap, index) in form.caps" :key="cap.key" class="qos-cap" data-qos-cap>
          <div class="qos-form__grid">
            <UiTextField v-model="cap.name" :label="t('adminMonitor.categories.qos.capName')" size="md" required />
            <UiNumberField v-model="cap.limit_mbps" :label="t('adminMonitor.categories.qos.capLimit')" :help="t('adminMonitor.categories.qos.capLimitHelp')" unit="Mbps" :min="0" :max="100000" size="md" data-qos-cap-limit />
            <UiSelect v-model="cap.priority" :label="t('adminMonitor.categories.qos.capPriority')" :options="priorityOptions" size="md" />
          </div>
          <UiTextarea v-model="cap.domains" :label="t('adminMonitor.categories.qos.capDomains')" :help="t('adminMonitor.categories.qos.capDomainsHelp')" :rows="3" />
          <UiButton size="sm" variant="ghost" :icon="Trash2" @click="form.caps.splice(index, 1)">{{ t('adminMonitor.categories.qos.removeCap') }}</UiButton>
        </div>
        <UiButton size="sm" :icon="Plus" @click="addCap">{{ t('adminMonitor.categories.qos.addCap') }}</UiButton>
      </fieldset>

      <div>
        <UiButton type="submit" variant="primary" :loading="saving" :disabled="!loaded" data-qos-save>{{ t('adminMonitor.categories.save') }}</UiButton>
      </div>
    </form>
  </UiCard>
</template>

<script setup>
// QoS of 流量分类: the traffic priorities and per-user class caps the proxy
// nodes get in their configuration (GET and PUT /api/v4/admin/traffic/qos,
// internal/trafficqos): on or off, a priority for each category, the
// nodes' download capacity (one default and per node) and caps such as
// Steam downloads at 5 Mbit/s per user.
import { computed, onMounted, reactive, ref } from 'vue'
import { Plus, Trash2 } from '@lucide/vue'
import { getTrafficQos, updateTrafficQos } from '@/api/admin'
import { getKernelAdminNodes } from '@/api/kernel'
import { useAppI18n } from '@/composables/useAppI18n'
import LoadError from '@/components/common/LoadError.vue'
import UiButton from '@/ui/UiButton.vue'
import UiCard from '@/ui/UiCard.vue'
import UiNumberField from '@/ui/UiNumberField.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiSwitch from '@/ui/UiSwitch.vue'
import UiTextField from '@/ui/UiTextField.vue'
import UiTextarea from '@/ui/UiTextarea.vue'
import { useToast } from '@/ui/composables/useToast'
import { errorText } from './trafficData'

const PRIORITY_CATEGORIES = ['video', 'music', 'messaging', 'social', 'gaming', 'ai', 'download']

const { t, te } = useAppI18n()
const toast = useToast()

const loaded = ref(false)
const loadError = ref(null)
const saving = ref(false)
const nodes = ref([])
const form = reactive({ enabled: false, priorities: {}, min_share_percent: 10, default_egress_mbps: 0, node_egress: {}, caps: [] })
let capKey = 0

const priorityOptions = computed(() => ['high', 'normal', 'low'].map(value => ({ value, label: t(`adminMonitor.categories.qos.priority.${value}`) })))

function categoryLabel(category) {
  const key = `portal.destinationStats.category.${category}`
  return te(key) ? t(key) : category
}

function apply(settings) {
  form.enabled = Boolean(settings?.enabled)
  form.priorities = Object.fromEntries(PRIORITY_CATEGORIES.map(category => [category, settings?.priorities?.[category] || 'normal']))
  form.min_share_percent = Number(settings?.min_share_percent) || 10
  form.default_egress_mbps = Number(settings?.default_egress_mbps) || 0
  form.node_egress = Object.fromEntries(Object.entries(settings?.node_egress_mbps || {}).map(([id, mbps]) => [id, String(mbps)]))
  form.caps = (Array.isArray(settings?.caps) ? settings.caps : []).map(cap => ({
    key: ++capKey,
    name: cap.name || '',
    priority: cap.priority || 'low',
    limit_mbps: Number(cap.limit_mbps) || 0,
    domains: (cap.domains || []).join('\n')
  }))
}

function addCap() {
  form.caps.push({ key: ++capKey, name: '', priority: 'low', limit_mbps: 5, domains: '' })
}

function body() {
  const priorities = {}
  for (const [category, priority] of Object.entries(form.priorities)) {
    if (priority !== 'normal') priorities[category] = priority
  }
  const nodeEgress = {}
  for (const [id, raw] of Object.entries(form.node_egress)) {
    const text = String(raw ?? '').trim()
    if (text !== '' && Number.isFinite(Number(text))) nodeEgress[id] = Math.max(0, Math.round(Number(text)))
  }
  return {
    enabled: form.enabled,
    priorities,
    min_share_percent: Number(form.min_share_percent) || 10,
    default_egress_mbps: Number(form.default_egress_mbps) || 0,
    node_egress_mbps: nodeEgress,
    caps: form.caps.map(cap => ({
      name: cap.name.trim(),
      priority: cap.priority,
      limit_mbps: Number(cap.limit_mbps) || 0,
      domains: cap.domains.split(/[\s,]+/).map(domain => domain.trim()).filter(Boolean)
    }))
  }
}

async function load() {
  loadError.value = null
  try {
    apply(await getTrafficQos())
    loaded.value = true
  } catch (err) {
    loadError.value = err
  }
  try {
    const answer = await getKernelAdminNodes({ page: 1, page_size: 500 })
    nodes.value = Array.isArray(answer?.list) ? answer.list : []
  } catch {
    // The per-node capacities stay hidden without the node list.
  }
}

async function save() {
  saving.value = true
  try {
    apply(await updateTrafficQos(body()))
    toast.success(t('adminMonitor.categories.qos.saved'))
  } catch (err) {
    toast.error(err?.response?.data?.error?.message || errorText(err, t('adminMonitor.categories.qos.saveFailed')))
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  void load()
})

defineExpose({ load, body })
</script>

<style scoped>
.qos-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
}

.qos-form__group {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  margin: 0;
  padding: 0;
  border: 0;
}

.qos-form__legend {
  padding: 0;
  font-weight: var(--weight-medium);
}

.qos-form__hint {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.qos-form__grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
  gap: var(--space-3) var(--space-4);
}

.qos-cap {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--space-3);
  padding: var(--space-4);
  border: 1px solid var(--separator);
  border-radius: var(--radius-md);
}

.qos-cap > :deep(*) {
  align-self: stretch;
}

.qos-cap > button {
  align-self: flex-start;
}
</style>
