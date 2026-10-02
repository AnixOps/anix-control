<template>
  <NodeDetailLayout
    v-model:tab="tab"
    :title="node ? node.name : t('forwardNodesPage.detail.sections')"
    :description="node ? `${typeLabel(node.type)} · ${node.host}:${node.port}` : ''"
    back-to="/admin/forward/nodes"
    :back-label="t('forwardSuite.nav.nodeXTopology')"
    :sections-label="t('forwardNodesPage.detail.sections')"
    :tabs="tabs"
    :loading="loading"
    :loaded="Boolean(node)"
    :error="error"
    :error-title="t('forwardNodesPage.detail.loadFailed')"
    :not-found="notFound"
    :not-found-text="t('forwardNodesPage.detail.notFound')"
    @retry="load"
  >
    <template v-if="node" #meta>
      <UiBadge :status="node.status === 1 ? 'online' : 'offline'" :label="node.status === 1 ? t('runtime.nodeXTopology.status.online') : t('runtime.nodeXTopology.status.offline')" />
      <UiBadge v-if="!node.enabled" status="disabled" :label="t('runtime.nodeXTopology.status.disabled')" />
    </template>
    <template #actions>
      <UiButton :icon="Stethoscope" :loading="actions.isPending(node.id, 'check')" data-test="node-detail-check" @click="actions.check(node)">{{ t('runtime.nodeXTopology.actions.healthCheck') }}</UiButton>
      <UiButton variant="primary" :icon="Pencil" data-test="node-detail-edit" @click="editorOpen = true">{{ t('forwardNodesPage.detail.editConfig') }}</UiButton>
    </template>

    <template #overview>
      <UiGroupedList heading-tag="h2" :title="t('forwardNodesPage.detail.runtime')" :footer="t('forwardNodesPage.nodex.onlineNote')">
        <UiGroupedListRow :label="t('forwardNodesPage.nodex.columns.status')">
          <template #value>
            <UiBadge :status="node.status === 1 ? 'online' : 'offline'" :label="node.status === 1 ? t('runtime.nodeXTopology.status.online') : t('runtime.nodeXTopology.status.offline')" />
          </template>
        </UiGroupedListRow>
        <UiGroupedListRow :label="t('runtime.nodeXTopology.meta.latency')" :value="node.latency ? `${node.latency} ms` : '—'" />
        <UiGroupedListRow :label="t('runtime.nodeXTopology.meta.currentConnections')" :value="String(node.currentConn)" />
        <UiGroupedListRow :label="t('runtime.nodeXTopology.meta.upload')" :value="format.bytes(node.totalUpload)" />
        <UiGroupedListRow :label="t('runtime.nodeXTopology.meta.download')" :value="format.bytes(node.totalDownload)" />
        <UiGroupedListRow :label="t('runtime.nodeXTopology.meta.uptime')" :value="`${node.uptime.toFixed(1)}%`" />
        <UiGroupedListRow :label="t('runtime.nodeXTopology.meta.lastCheck')" :value="node.lastCheck ? format.dateTime(node.lastCheck) : '—'" />
        <UiGroupedListRow :label="t('forwardNodesPage.detail.lastResult')">
          <template #value>
            <span v-if="lastResult" :class="['fn-result', lastResult.success ? 'is-ok' : 'is-fail']" data-test="node-detail-result">{{ lastResult.message }}</span>
            <span v-else>{{ t('forwardNodesPage.detail.noResult') }}</span>
          </template>
        </UiGroupedListRow>
      </UiGroupedList>
      <UiGroupedList heading-tag="h2" :title="t('forwardNodesPage.detail.actions')">
        <UiGroupedListRow :label="actions.isPending(node.id, 'sync') ? t('runtime.nodeXTopology.actions.syncing') : t('runtime.nodeXTopology.actions.syncStats')" data-test="node-detail-sync" @click="runIfIdle(actions.sync)" />
        <UiGroupedListRow :label="t('runtime.nodeXTopology.actions.testConnection')" data-test="node-detail-test" @click="connectionOpen = true" />
      </UiGroupedList>
    </template>

    <template #config>
      <UiGroupedList heading-tag="h2" :title="t('forwardNodesPage.detail.identity')">
        <UiGroupedListRow :label="t('runtime.nodeXTopology.nodeModal.fields.name')" :value="node.name" />
        <UiGroupedListRow :label="t('runtime.nodeXTopology.nodeModal.fields.type')" :value="typeLabel(node.type)" />
        <UiGroupedListRow :label="t('runtime.nodeXTopology.nodeModal.fields.region')" :value="node.region || '—'" />
        <UiGroupedListRow :label="t('runtime.nodeXTopology.nodeModal.fields.isp')" :value="node.isp || '—'" />
      </UiGroupedList>
      <UiGroupedList heading-tag="h2" :title="t('forwardNodesPage.detail.endpoints')">
        <UiGroupedListRow :label="t('runtime.nodeXTopology.nodeModal.fields.host')">
          <template #value><code class="fn-mono">{{ node.host }}</code></template>
        </UiGroupedListRow>
        <UiGroupedListRow :label="t('runtime.nodeXTopology.nodeModal.fields.servicePort')" :value="node.port || '—'" />
        <UiGroupedListRow :label="t('runtime.nodeXTopology.nodeModal.fields.apiPort')" :value="node.apiPort || '—'" />
        <UiGroupedListRow :label="t('runtime.nodeXTopology.nodeModal.fields.apiToken')" :value="tokenLabel" />
        <UiGroupedListRow :label="t('runtime.nodeXTopology.nodeModal.fields.metricsPort')" :value="node.metricsPort || '—'" />
      </UiGroupedList>
      <UiGroupedList heading-tag="h2" :title="t('forwardNodesPage.detail.capacity')">
        <UiGroupedListRow :label="t('runtime.nodeXTopology.nodeModal.fields.bandwidth')" :value="node.bandwidth || '—'" />
        <UiGroupedListRow :label="t('runtime.nodeXTopology.nodeModal.fields.maxConnections')" :value="node.maxConn || '—'" />
        <UiGroupedListRow :label="t('runtime.nodeXTopology.nodeModal.fields.weight')" :value="node.weight" />
      </UiGroupedList>
      <UiGroupedList>
        <UiGroupedListRow :label="t('forwardNodesPage.detail.editConfig')" @click="editorOpen = true" />
      </UiGroupedList>
    </template>

    <template #danger>
      <UiGroupedList :footer="t('forwardNodesPage.detail.disableFooter')">
        <UiGroupedListRow
          :label="node.enabled ? t('runtime.nodeXTopology.actions.disable') : t('runtime.nodeXTopology.actions.enable')"
          data-test="node-detail-toggle"
          @click="runIfIdle(actions.toggle)"
        />
      </UiGroupedList>
      <UiGroupedList :footer="t('forwardNodesPage.detail.dangerFooter')">
        <UiGroupedListRow class="fn-danger-row" :label="t('runtime.nodeXTopology.deleteModal.deleteNode')" data-test="node-detail-delete" @click="removeNode" />
      </UiGroupedList>
    </template>
  </NodeDetailLayout>

  <NodeXNodeDialog v-if="node" v-model:open="editorOpen" :node-id="node.id" @saved="load" />
  <NodeXConnectionDialog v-model:open="connectionOpen" :node="node" />
</template>

<script setup>
// 转发节点 › NodeX 节点 › detail (/admin/forward/nodes/:id, UI U7): one NodeX
// forward node in the detail template (概览 / 配置 / 危险操作). Reads
// GET /admin/forward/nodes/:id?scope=nodex; the actions are the list's.
import { computed, inject, reactive, ref, watch } from 'vue'
import { routerKey } from 'vue-router'
import { Pencil, Stethoscope } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { getForwardNode } from '@/api/admin'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiGroupedList from '@/ui/UiGroupedList.vue'
import UiGroupedListRow from '@/ui/UiGroupedListRow.vue'
import { useFormat } from '@/ui/composables/useFormat'
import NodeDetailLayout from './NodeDetailLayout.vue'
import NodeXConnectionDialog from './NodeXConnectionDialog.vue'
import NodeXNodeDialog from './NodeXNodeDialog.vue'
import { NODEX_SCOPE, errorMessage, normalizeNode, unwrapForwardResponse } from './forwardNodeModel'
import { useNodeXNodeActions } from './useNodeXNodeActions'
import { useDetailTab } from './useDetailTab'

const props = defineProps({
  id: { type: Number, required: true }
})

const { t, translateLiteral } = useAppI18n()
const format = useFormat()
const router = inject(routerKey, null)

const node = ref(null)
const loading = ref(false)
const error = ref(null)
const notFound = ref(false)
const editorOpen = ref(false)
const connectionOpen = ref(false)

const tab = useDetailTab(['overview', 'config', 'danger'])
const tabs = computed(() => [
  { value: 'overview', label: t('forwardNodesPage.detail.tabs.overview') },
  { value: 'config', label: t('forwardNodesPage.detail.tabs.config') },
  { value: 'danger', label: t('forwardNodesPage.detail.tabs.danger') }
])

const actions = reactive(useNodeXNodeActions({
  onChanged: kind => (kind === 'delete' ? router?.push('/admin/forward/nodes') : load())
}))
const lastResult = computed(() => (node.value ? actions.results[node.value.id] : null))
// The answer masks a stored token; the page never shows a token.
const tokenLabel = computed(() => (node.value?.apiToken ? t('forwardNodesPage.detail.tokenHidden') : t('forwardNodesPage.detail.tokenNone')))

function typeLabel(value) {
  return value === 'relay' || value === 'exit' ? t(`runtime.nodeXTopology.filters.${value}`) : (value || '-')
}

function runIfIdle(action) {
  if (node.value && !actions.isPending(node.value.id)) action(node.value)
}

function removeNode() {
  if (node.value) actions.remove(node.value)
}

async function load() {
  loading.value = true
  error.value = null
  notFound.value = false
  try {
    const payload = unwrapForwardResponse(await getForwardNode(props.id, NODEX_SCOPE), t('runtime.nodeXTopology.validation.requestFailed'))
    if (!payload || !payload.id) {
      node.value = null
      notFound.value = true
    } else {
      node.value = normalizeNode(payload)
    }
  } catch (err) {
    if (err?.response?.status === 404) {
      node.value = null
      notFound.value = true
    } else if (!node.value) {
      error.value = errorMessage(err, t('forwardNodesPage.detail.loadFailed'), translateLiteral)
    }
  } finally {
    loading.value = false
  }
}

watch(() => props.id, () => {
  node.value = null
  load()
}, { immediate: true })
</script>

<style scoped>
.fn-mono {
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
}

.fn-result {
  color: color-mix(in srgb, var(--label-2) 70%, var(--label-1));
}

.fn-result.is-ok {
  color: color-mix(in srgb, var(--success) 78%, var(--label-1));
}

.fn-result.is-fail {
  color: color-mix(in srgb, var(--danger) 78%, var(--label-1));
}

.fn-danger-row :deep(.ui-row__label) {
  color: var(--danger);
}
</style>
