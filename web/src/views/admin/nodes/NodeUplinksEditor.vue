<template>
  <div class="node-uplinks" data-testid="node-uplinks">
    <div class="node-uplinks__head">
      <h3 class="node-uplinks__title">{{ t('nodeNetwork.uplinks.title') }}</h3>
      <p class="node-uplinks__hint">{{ t('nodeNetwork.uplinks.description') }}</p>
    </div>

    <div v-for="(uplink, index) in modelValue.uplinks" :key="index" class="node-uplinks__card" :data-testid="`node-uplink-${index}`">
      <div class="node-uplinks__pair">
        <UiTextField
          :model-value="uplink.label"
          :label="t('nodeNetwork.uplinks.label')"
          placeholder="CN2"
          :error="fieldError(`uplinks[${index}].label`)"
          :data-testid="`node-uplink-${index}-label`"
          @update:model-value="value => edit(index, { label: value })"
        />
        <UiTextField
          :model-value="uplink.name"
          :label="t('nodeNetwork.uplinks.name')"
          :help="t('nodeNetwork.uplinks.nameHint')"
          placeholder="cn2"
          :error="fieldError(`uplinks[${index}].name`) || fieldError(`uplinks[${index}]`)"
          :data-testid="`node-uplink-${index}-name`"
          @update:model-value="value => edit(index, { name: value })"
        />
      </div>
      <UiTextField
        :model-value="uplink.interface"
        :label="t('nodeNetwork.interface')"
        placeholder="eth1"
        :error="fieldError(`uplinks[${index}].interface`)"
        :data-testid="`node-uplink-${index}-interface`"
        @update:model-value="value => edit(index, { interface: value })"
      />
      <div class="node-uplinks__pair">
        <UiTextField :model-value="uplink.source_v4" :label="t('nodeNetwork.sourceV4')" :error="fieldError(`uplinks[${index}].source_v4`)" :data-testid="`node-uplink-${index}-source-v4`" @update:model-value="value => edit(index, { source_v4: value })" />
        <UiTextField :model-value="uplink.gateway_v4" :label="t('nodeNetwork.gatewayV4')" :error="fieldError(`uplinks[${index}].gateway_v4`)" :data-testid="`node-uplink-${index}-gateway-v4`" @update:model-value="value => edit(index, { gateway_v4: value })" />
      </div>
      <div class="node-uplinks__pair">
        <UiTextField :model-value="uplink.source_v6" :label="t('nodeNetwork.sourceV6')" :error="fieldError(`uplinks[${index}].source_v6`)" :data-testid="`node-uplink-${index}-source-v6`" @update:model-value="value => edit(index, { source_v6: value })" />
        <UiTextField :model-value="uplink.gateway_v6" :label="t('nodeNetwork.gatewayV6')" :error="fieldError(`uplinks[${index}].gateway_v6`)" :data-testid="`node-uplink-${index}-gateway-v6`" @update:model-value="value => edit(index, { gateway_v6: value })" />
      </div>
      <div>
        <UiButton variant="tertiary" size="sm" type="button" :data-testid="`node-uplink-${index}-remove`" @click="remove(index)">{{ t('nodeNetwork.uplinks.remove') }}</UiButton>
      </div>
    </div>

    <div class="node-uplinks__actions">
      <UiButton
        v-for="iface in addable"
        :key="iface.name"
        variant="secondary"
        size="sm"
        type="button"
        :data-testid="`node-uplink-add-${iface.name}`"
        @click="addDetected(iface)"
      >
        {{ t('nodeNetwork.uplinks.addDetected', { name: iface.name }) }}
      </UiButton>
      <UiButton variant="tertiary" size="sm" type="button" data-testid="node-uplink-add" @click="add({})">{{ t('nodeNetwork.uplinks.add') }}</UiButton>
    </div>

    <UiSelect
      v-if="modelValue.uplinks.length"
      :model-value="modelValue.uplink_default || AUTO"
      :options="defaultOptions"
      :label="t('nodeNetwork.uplinks.default')"
      :help="t('nodeNetwork.uplinks.defaultHint')"
      :error="fieldError('uplink_default')"
      data-testid="node-uplink-default"
      @update:model-value="value => emit('update:modelValue', { ...modelValue, uplink_default: value === AUTO ? '' : value })"
    />

    <div v-if="hops.length" class="node-uplinks__hops">
      <h3 class="node-uplinks__title">{{ t('nodeNetwork.uplinks.hops') }}</h3>
      <div v-for="hop in hops" :key="`${hop.to}/${hop.family}`" class="node-uplinks__card" :data-testid="`node-uplink-hop-${hop.to}`">
        <div class="node-uplinks__hop-head">
          <strong>{{ hop.to_name || hop.to }}</strong>
          <span class="node-uplinks__hint">{{ hop.target }}{{ hop.family === 'v6' ? ' · IPv6' : '' }}</span>
        </div>
        <ul class="node-uplinks__probes">
          <li v-for="probe in hop.probes" :key="probe.uplink" class="node-uplinks__probe" :data-testid="`node-uplink-probe-${hop.to}-${probe.uplink}`">
            <span class="node-uplinks__probe-name">{{ labelOf(probe.uplink) }}</span>
            <UiBadge :tone="probeTone(probe)" :label="probeText(probe)" />
            <UiBadge v-if="hop.active === probe.uplink" tone="info" :dot="false" :label="t('nodeNetwork.uplinks.inUse')" />
          </li>
        </ul>
        <p v-if="hop.choice === 'auto' && hop.reason" class="node-uplinks__hint">
          {{ t('nodeNetwork.uplinks.reason', { reason: hop.reason, time: hop.since ? new Date(hop.since).toLocaleString() : '' }) }}
        </p>
        <UiSelect
          :model-value="modelValue.uplink_choices?.[hop.to] || FOLLOW"
          :options="choiceOptions"
          :label="t('nodeNetwork.uplinks.choice')"
          :error="fieldError(`uplink_choices.${hop.to}`)"
          :data-testid="`node-uplink-choice-${hop.to}`"
          @update:model-value="value => choose(hop.to, value)"
        />
      </div>
    </div>
  </div>
</template>

<script setup>
// 上联网卡 in the node's 网络 settings: the NICs traffic to the next node
// of a route may leave by (dual-NIC entries such as CN2 + 9929), which one
// each next node gets (自动: the lowest latency, or a pinned NIC), and each
// NIC's round trip to each next node as the node last probed it. The
// parent saves it with the rest of the settings.
import { computed } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiTextField from '@/ui/UiTextField.vue'

const props = defineProps({
  modelValue: { type: Object, required: true },
  hops: { type: Array, default: () => [] },
  detected: { type: Array, default: () => [] },
  fieldError: { type: Function, default: () => '' }
})
const emit = defineEmits(['update:modelValue'])
const { t } = useAppI18n()

const AUTO = 'auto'
// FOLLOW stands for "no choice of its own" in a select, which cannot hold ''.
const FOLLOW = '__default__'

const addable = computed(() => props.detected.filter(iface =>
  !props.modelValue.uplinks.some(u => u.interface === iface.name) && (iface.addresses_v4.length || iface.addresses_v6.length)))

const uplinkOptions = computed(() => props.modelValue.uplinks
  .filter(u => u.name)
  .map(u => ({ value: u.name, label: u.label || u.name, description: u.interface })))

const defaultOptions = computed(() => [{ value: AUTO, label: t('nodeNetwork.uplinks.auto') }, ...uplinkOptions.value])
const choiceOptions = computed(() => [
  { value: FOLLOW, label: t('nodeNetwork.uplinks.follow') },
  { value: AUTO, label: t('nodeNetwork.uplinks.auto') },
  ...uplinkOptions.value
])

function update(uplinks) {
  emit('update:modelValue', { ...props.modelValue, uplinks })
}

function edit(index, change) {
  update(props.modelValue.uplinks.map((u, i) => (i === index ? { ...u, ...change } : u)))
}

function remove(index) {
  update(props.modelValue.uplinks.filter((_, i) => i !== index))
}

function add(uplink) {
  update([...props.modelValue.uplinks, { name: '', label: '', interface: '', source_v4: '', gateway_v4: '', source_v6: '', gateway_v6: '', ...uplink }])
}

// addDetected adds a detected interface with its first addresses and its
// gateways; the name is the interface name without what an uplink name
// cannot hold.
function addDetected(iface) {
  const source4 = iface.addresses_v4[0] || ''
  const source6 = iface.addresses_v6[0] || ''
  add({
    name: iface.name.toLowerCase().replace(/[^a-z0-9]/g, '').slice(0, 12),
    label: iface.name,
    interface: iface.name,
    source_v4: source4,
    gateway_v4: (source4 && iface.gateway_v4) || '',
    source_v6: source6,
    gateway_v6: (source6 && iface.gateway_v6) || ''
  })
}

function choose(to, value) {
  const choices = { ...(props.modelValue.uplink_choices || {}) }
  if (value === FOLLOW) delete choices[to]
  else choices[to] = value
  emit('update:modelValue', { ...props.modelValue, uplink_choices: choices })
}

function labelOf(name) {
  const uplink = props.modelValue.uplinks.find(u => u.name === name)
  return uplink?.label || name
}

function probeTone(probe) {
  if (probe.state === 'up') return 'success'
  if (probe.state === 'down') return 'danger'
  return 'neutral'
}

function probeText(probe) {
  if (probe.state === 'up') return t('nodeNetwork.uplinks.rtt', { ms: probe.rtt_ms })
  if (probe.state === 'down') return t('nodeNetwork.uplinks.down')
  return t('nodeNetwork.uplinks.waiting')
}
</script>

<style scoped>
.node-uplinks {
  display: grid;
  gap: var(--space-4);
}

.node-uplinks__head,
.node-uplinks__hops {
  display: grid;
  gap: var(--space-2);
}

.node-uplinks__title {
  margin: 0;
  font-size: var(--type-body-size);
  font-weight: 600;
}

.node-uplinks__hint {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.node-uplinks__card {
  display: grid;
  gap: var(--space-3);
  padding: var(--space-4);
  border-radius: var(--radius-md);
  background: var(--fill-1);
}

.node-uplinks__pair {
  display: grid;
  gap: var(--space-4);
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
}

.node-uplinks__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}

.node-uplinks__hop-head {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: baseline;
}

.node-uplinks__probes {
  display: grid;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}

.node-uplinks__probe {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
}

.node-uplinks__probe-name {
  min-width: 80px;
}
</style>
