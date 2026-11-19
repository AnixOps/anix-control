<template>
  <UiSection :title="t('nodeReach.title')" :description="t('nodeReach.description')" heading-tag="h2" data-testid="node-reach-section">
    <UiSkeleton v-if="status === 'idle' || status === 'loading'" variant="card" :label="t('nodeReach.loading')" />
    <UiErrorState v-else-if="status === 'unavailable'" :title="t('nodeReach.loadFailed')" @retry="load" />
    <div v-else class="node-reach-section">
      <UiRadioGroup
        :model-value="role"
        :label="t('nodeReach.role')"
        :help="roleHint"
        :options="roleOptions"
        orientation="horizontal"
        :disabled="saving"
        data-testid="node-reach-role"
        @update:model-value="setRole"
      />
      <p class="node-reach-section__text" data-testid="node-reach-views">
        <template v-if="entry">
          {{ t(`nodeReach.status.${entry.status}`) }} · {{ regionText('domestic') }} · {{ regionText('overseas') }}
        </template>
        <template v-else>{{ t('nodeReach.noAnswer') }}</template>
      </p>
      <p v-if="saveError" class="node-reach-section__error" role="alert">{{ saveError }}</p>
    </div>
  </UiSection>
</template>

<script setup>
// 可达性探测 on the node page's 协议 tab: whether this node is a vantage
// (its Agent probes the other nodes from its region every 10 minutes), and
// what the vantages found about this node. GET /api/v4/kernel/reach,
// PUT /api/v4/kernel/reach/vantages.
import { computed, onMounted, ref } from 'vue'
import { setKernelReachVantages } from '@/api/kernel'
import { useAppI18n } from '@/composables/useAppI18n'
import { useFormat } from '@/ui/composables/useFormat'
import UiErrorState from '@/ui/UiErrorState.vue'
import UiSection from '@/ui/UiSection.vue'
import UiRadioGroup from '@/ui/UiRadioGroup.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import { useNodeReach } from './useNodeReach'

const props = defineProps({
  nodeId: { type: [Number, String], required: true }
})
const { t } = useAppI18n()
const format = useFormat()
const { nodes, vantages, status, load, apply } = useNodeReach()
const saving = ref(false)
const saveError = ref('')

const id = computed(() => Number(props.nodeId))
const entry = computed(() => nodes.value.get(id.value))
const vantage = computed(() => vantages.value.get(id.value))
const role = computed(() => vantage.value?.region || 'none')
const roleOptions = computed(() => ['none', 'domestic', 'overseas'].map(value => ({ value, label: t(`nodeReach.roles.${value}`) })))
const roleHint = computed(() => {
  if (role.value === 'none') return t('nodeReach.roleHint.none')
  return vantage.value?.capable ? t('nodeReach.roleHint.active') : t('nodeReach.roleHint.waiting')
})

const regionText = region => {
  const view = entry.value?.[region]
  const reachable = view?.reachable
  const text = t(`nodeReach.view.${reachable === true ? 'yes' : reachable === false ? 'no' : 'none'}`, { region: t(`nodeReach.region.${region}`) })
  return view?.probed_at ? `${text}（${format.relativeTime(view.probed_at)}）` : text
}

async function setRole(value) {
  saving.value = true
  saveError.value = ''
  const list = [...vantages.value.values()]
    .filter(v => Number(v.node_id) !== id.value)
    .map(v => ({ node_id: Number(v.node_id), region: v.region }))
  if (value !== 'none') list.push({ node_id: id.value, region: value })
  try {
    apply(await setKernelReachVantages(list))
  } catch (e) {
    saveError.value = e?.response?.data?.error?.message || e?.message || t('nodeReach.saveFailed')
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.node-reach-section {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.node-reach-section__text,
.node-reach-section__error {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.node-reach-section__error {
  color: var(--danger);
}
</style>
