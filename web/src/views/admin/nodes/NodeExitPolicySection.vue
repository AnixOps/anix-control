<template>
  <UiSection :title="t('nodeExitPolicy.title')" :description="t('nodeExitPolicy.description')" heading-tag="h2" data-testid="node-exit-policy">
    <UiSkeleton v-if="loading" variant="card" :label="t('nodeExitPolicy.loading')" />
    <UiErrorState v-else-if="loadError" :title="t('nodeExitPolicy.loadFailed')" :error="loadError" @retry="load" />
    <div v-else class="node-exit">
      <UiSwitch
        :model-value="policy.block_mainland"
        :label="t('nodeExitPolicy.blockMainland')"
        :description="t('nodeExitPolicy.blockMainlandHelp')"
        :disabled="saving"
        data-testid="node-exit-policy-switch"
        @update:model-value="toggle"
      />
      <p class="node-exit__hint" data-testid="node-exit-policy-count">
        {{ policy.mainland_blocked_at
          ? t('nodeExitPolicy.blocked', { n: policy.mainland_blocked_total, date: format.dateTime(policy.mainland_blocked_at) })
          : t('nodeExitPolicy.none') }}
      </p>
      <p v-if="saveError" class="node-exit__error" role="alert">{{ saveError }}</p>
    </div>
  </UiSection>
</template>

<script setup>
// 出口策略 on the node page's 协议 tab: whether the node, as an exit, refuses
// mainland-China destinations (off by default), and how many connections its
// Agent reported refusing. It needs an Agent and kernel of 5.0.2 or later.
// GET/PUT /api/v4/kernel/nodes/:id/exit-policy
import { onMounted, ref } from 'vue'
import { getKernelNodeExitPolicy, setKernelNodeExitPolicy } from '@/api/kernel'
import { useAppI18n } from '@/composables/useAppI18n'
import { useFormat } from '@/ui/composables/useFormat'
import UiErrorState from '@/ui/UiErrorState.vue'
import UiSection from '@/ui/UiSection.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import UiSwitch from '@/ui/UiSwitch.vue'

const props = defineProps({
  nodeRef: { type: String, required: true }
})
const { t } = useAppI18n()
const format = useFormat()

const policy = ref({ block_mainland: false, mainland_blocked_total: 0, mainland_blocked_at: null })
const loading = ref(true)
const loadError = ref(null)
const saving = ref(false)
const saveError = ref('')

async function load() {
  loading.value = true
  loadError.value = null
  try {
    policy.value = await getKernelNodeExitPolicy(props.nodeRef)
  } catch (e) {
    loadError.value = e
  } finally {
    loading.value = false
  }
}

async function toggle(value) {
  saving.value = true
  saveError.value = ''
  try {
    policy.value = await setKernelNodeExitPolicy(props.nodeRef, value)
  } catch (e) {
    saveError.value = e?.response?.data?.error?.message || e?.message || t('nodeExitPolicy.saveFailed')
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.node-exit {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.node-exit__hint,
.node-exit__error {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.node-exit__error {
  color: var(--danger);
}
</style>
