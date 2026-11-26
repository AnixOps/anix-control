<template>
  <UiSection :title="t('meshAddr.names.title')" :description="t('meshAddr.names.description', { suffix: list?.suffix || '' })" heading-tag="h2" data-testid="mesh-names">
    <p v-if="list?.suffix_invalid" class="mesh-error" role="alert" data-testid="mesh-names-suffix-invalid">{{ t('meshAddr.names.suffixInvalid') }}</p>
    <UiDataTable
      :columns="columns"
      :rows="rows"
      row-key="key"
      :label="t('meshAddr.names.title')"
      :loading="!list && !loadError"
      :error="list ? null : loadError"
      :error-title="t('meshAddr.names.loadFailed')"
      :empty-title="t('meshAddr.names.empty')"
      @retry="load"
    >
      <template #cell-fqdn="{ row }">
        <span class="mesh-badges">
          <span class="mesh-mono">{{ row.fqdn }}</span>
          <UiBadge v-if="row.duplicate" tone="warning" :label="t('meshAddr.names.duplicate')" />
        </span>
      </template>
      <template #cell-address="{ row }"><span class="mesh-mono">{{ row.address }}</span></template>
      <template #cell-source="{ row }">{{ t(`meshAddr.names.sources.${row.source}`) }}</template>
    </UiDataTable>
    <p v-if="list?.unnamed?.length" class="mesh-muted" data-testid="mesh-names-unnamed">
      {{ t('meshAddr.names.unnamed', { names: list.unnamed.map(node => node.node_name || node.node_ref).join('、') }) }}
    </p>
  </UiSection>
</template>

<script setup>
// 组网 → 内网域名: every node, device and published name as
// name.region-code.suffix; duplicates are all listed (R28).
import { computed, onMounted, ref } from 'vue'
import { UiBadge, UiDataTable, UiSection } from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import { getKernelMeshNames } from '@/api/kernel'
import { meshErrorMessage } from '../meshModel'

const { t } = useAppI18n()
const list = ref(null)
const loadError = ref(null)

const rows = computed(() => (list.value?.entries || []).map(entry => ({ ...entry, key: `${entry.source} ${entry.ref} ${entry.fqdn}` })))
const columns = computed(() => [
  { key: 'fqdn', label: t('meshAddr.names.fqdn'), primary: true },
  { key: 'address', label: t('meshAddr.common.address'), secondary: true },
  { key: 'source', label: t('meshAddr.names.source'), breakpoint: 'md' }
])

async function load() {
  loadError.value = null
  try {
    list.value = await getKernelMeshNames()
  } catch (e) {
    loadError.value = { message: meshErrorMessage(e) }
  }
}

onMounted(load)
defineExpose({ load })
</script>

<style scoped src="./meshAddr.css"></style>
