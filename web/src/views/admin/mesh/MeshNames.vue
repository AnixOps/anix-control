<template>
  <UiSection :title="t('mesh.names.title')" :description="list ? t('mesh.names.description', { suffix: list.suffix }) : ''" heading-tag="h2" data-testid="mesh-names">
    <UiDataTable
      :columns="columns"
      :rows="list?.entries || []"
      row-key="key"
      :label="t('mesh.names.title')"
      :loading="!list && !loadError"
      :error="list ? null : loadError"
      :error-title="t('mesh.names.loadFailed')"
      :empty-title="t('mesh.names.empty')"
      :settings="false"
      @retry="load"
    >
      <template #cell-fqdn="{ row }">
        <span class="mesh-badges">
          <span class="mesh-mono">{{ row.fqdn }}</span>
          <UiBadge v-if="row.duplicate" tone="warning" :label="t('mesh.names.duplicate')" />
        </span>
      </template>
      <template #cell-address="{ row }"><span class="mesh-mono">{{ row.address }}</span></template>
      <template #cell-source="{ row }">{{ t(`mesh.names.sources.${row.source}`) }}</template>
    </UiDataTable>
  </UiSection>
</template>

<script setup>
// 组网 → 内网域名: every member and every named device as
// <name>.<region code>.<suffix>, the list the panel publishes.
import { computed, onMounted, ref } from 'vue'
import { getMeshNames } from '@/api/mesh'
import { useAppI18n } from '@/composables/useAppI18n'
import { UiBadge, UiDataTable, UiSection } from '@/ui'

const { t } = useAppI18n()
const list = ref(null)
const loadError = ref(null)

const columns = computed(() => [
  { key: 'fqdn', label: t('mesh.names.fqdn'), primary: true, hideable: false },
  { key: 'address', label: t('mesh.names.address') },
  { key: 'source', label: t('mesh.names.source'), breakpoint: 'md' }
])

async function load() {
  loadError.value = null
  try {
    const answer = await getMeshNames()
    list.value = { ...answer, entries: (answer?.entries || []).map(entry => ({ ...entry, key: `${entry.fqdn} ${entry.address}` })) }
  } catch (e) {
    loadError.value = e
  }
}

onMounted(load)
</script>

<style scoped src="./mesh.css"></style>
