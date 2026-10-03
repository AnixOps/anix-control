<template>
  <div class="node-page">
    <UiPageHeader :title="headerTitle" :description="node ? node.address : ''">
      <template #back>
        <RouterLink class="node-page__back" :to="backTo" data-testid="node-back">
          <UiIcon :icon="ChevronLeft" :size="16" />
          <span>{{ t('admin.nodes.detail.back') }}</span>
        </RouterLink>
      </template>
      <template v-if="node" #meta>
        <UiBadge :status="statusName(status)" :label="t(`admin.nodes.statusText.${statusName(status)}`)" data-testid="node-status" />
      </template>
      <template v-if="node" #actions>
        <UiButton :icon="Pencil" data-testid="edit-node" @click="openEdit">{{ t('admin.nodes.actions.edit') }}</UiButton>
        <UiButton
          variant="primary"
          :icon="RefreshCcw"
          :loading="syncingNodeIds.has(node.id)"
          data-testid="sync-node"
          @click="syncNode(node)"
        >{{ t('admin.nodes.actions.syncReload') }}</UiButton>
      </template>
    </UiPageHeader>

    <UiSkeleton v-if="showSkeleton" variant="card" :label="t('admin.nodes.detail.loading')" />
    <UiEmptyState
      v-else-if="notFound"
      :icon="ServerOff"
      heading-tag="h2"
      :title="t('admin.nodes.detail.notFound', { id: nodeId })"
      :description="t('admin.nodes.detail.notFoundHint')"
    >
      <template #actions>
        <UiButton variant="primary" @click="router.push(backTo)">{{ t('admin.nodes.detail.back') }}</UiButton>
      </template>
    </UiEmptyState>
    <UiErrorState v-else-if="error" :title="t('admin.nodes.detail.loadFailed')" :error="error" @retry="load" />

    <UiTabs
      v-else-if="node"
      v-model="section"
      variant="segmented"
      :aria-label="t('admin.nodes.detail.sections')"
      :items="sectionItems"
      class="node-page__tabs"
    >
      <template #overview>
        <NodeOverviewSection :node="node" :protocol-count="protocolCount" :parent-name="parentName" />
      </template>
      <template #protocols>
        <NodeProtocolsSection :node="node" @changed="list => { protocolCount = list.length }" />
      </template>
      <template #credentials>
        <NodeCredentialsSection :node="node" @open-protocols="section = 'protocols'" />
      </template>
      <template #deploy>
        <NodeDeploySection :node="node" :deploy="deploy" />
      </template>
      <template #logs>
        <NodeLogsSection :node="node" />
      </template>
      <template v-if="hasServices" #services>
        <NodeServicesSection :node="node" />
      </template>
      <template #danger>
        <NodeDangerSection :node="node" @changed="load" @deleted="router.push(backTo)" />
      </template>
    </UiTabs>

    <NodeFormSheet v-model:open="editOpen" :node="node" :candidates="candidates" @saved="load" />
  </div>
</template>

<script setup>
// The node page (plan §7.2, §8.2): back link, name, status and the main
// actions, then sections — 概览 / 协议 / 凭据 / 部署 / 日志 / 服务 / 危险区 —
// kept in the URL (?section=protocols). Reads GET /admin/nodes/:id; every
// action calls the same endpoints as the node list. 服务 shows only when the
// node's machine-telemetry release reports a services table, and loads its
// code when opened.
import { computed, defineAsyncComponent, reactive, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { ChevronLeft, Pencil, RefreshCcw, ServerOff } from '@lucide/vue'
import { getNode, getNodes } from '@/api/admin'
import { nodeHasServicesTable } from '@/api/machineTelemetry'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiEmptyState from '@/ui/UiEmptyState.vue'
import UiErrorState from '@/ui/UiErrorState.vue'
import UiIcon from '@/ui/UiIcon.vue'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import UiTabs from '@/ui/UiTabs.vue'
import { useDelayedLoading } from '@/ui/composables/useDelayedLoading'
import { useAppI18n } from '@/composables/useAppI18n'
import NodeCredentialsSection from './nodes/NodeCredentialsSection.vue'
import NodeDangerSection from './nodes/NodeDangerSection.vue'
import NodeDeploySection from './nodes/NodeDeploySection.vue'
import NodeFormSheet from './nodes/NodeFormSheet.vue'
import NodeLogsSection from './nodes/NodeLogsSection.vue'
import NodeOverviewSection from './nodes/NodeOverviewSection.vue'
import NodeProtocolsSection from './nodes/NodeProtocolsSection.vue'
import {
  NODE_SECTIONS, OPTIONAL_NODE_SECTIONS, displayStatus, knownNodeName, listQuery, normalizeNode, readNodeList, readNodePayload, statusName
} from './nodes/nodeData'
import { useNodeActions } from './nodes/useNodeActions'
import { useNodeDeploy } from './nodes/useNodeDeploy'

const NodeServicesSection = defineAsyncComponent(() => import('./nodes/NodeServicesSection.vue'))

const { t } = useAppI18n()
const route = useRoute()
const router = useRouter()
const { syncingNodeIds, syncNode } = useNodeActions()
// Page-level, so a key generated in 部署 survives switching sections.
const deploy = reactive(useNodeDeploy())

const node = ref(null)
const loading = ref(false)
const error = ref(null)
const notFound = ref(false)
const protocolCount = ref(0)
const editOpen = ref(false)
const candidates = ref([])
const showSkeleton = useDelayedLoading(computed(() => loading.value && !node.value))

const nodeId = computed(() => String(route.params.id || ''))
const backTo = computed(() => ({ path: '/admin/nodes', query: listQuery() }))
const status = computed(() => (node.value ? displayStatus(node.value) : 0))
const headerTitle = computed(() => node.value?.name || (notFound.value || error.value ? t('admin.nodes.detail.title') : t('admin.nodes.detail.loading')))
const parentName = computed(() => (node.value?.parent_id ? knownNodeName(node.value.parent_id) : ''))

// Whether 服务 applies: null while unknown, then true or false.
const servicesAvailable = ref(null)
const hasServices = computed(() => servicesAvailable.value === true)
const sections = computed(() => NODE_SECTIONS.filter(value => !OPTIONAL_NODE_SECTIONS.includes(value) || (value === 'services' && hasServices.value)))
const sectionItems = computed(() => sections.value.map(value => ({ value, label: t(`admin.nodes.detail.sectionNames.${value}`) })))

// ?section= in the URL; 概览 is the default and leaves the query clean. A
// link to 服务 waits for the capability check instead of falling back.
const section = computed({
  get: () => {
    const value = String(route.query.section || '')
    if (value === 'services' && servicesAvailable.value === null) return value
    return sections.value.includes(value) ? value : 'overview'
  },
  set: (value) => {
    const query = { ...route.query }
    if (value === 'overview') delete query.section
    else query.section = value
    router.replace({ path: route.path, query })
  }
})

async function load() {
  const id = nodeId.value
  if (!id) return
  loading.value = true
  error.value = null
  notFound.value = false
  try {
    const payload = readNodePayload(await getNode(id))
    if (!payload || typeof payload !== 'object' || !payload.id) {
      throw Object.assign(new Error('not found'), { response: { status: 404 } })
    }
    if (nodeId.value !== id) return
    node.value = normalizeNode(payload)
    protocolCount.value = Array.isArray(payload.protocols) ? payload.protocols.length : 0
  } catch (e) {
    if (nodeId.value !== id) return
    node.value = null
    if (e?.response?.status === 404) notFound.value = true
    else error.value = e
  } finally {
    loading.value = false
  }
}

// The parent list of the edit form: up to 100 nodes (the list endpoint's
// largest page).
async function openEdit() {
  editOpen.value = true
  try {
    candidates.value = readNodeList(await getNodes({ page: 1, page_size: 100 })).map(normalizeNode)
  } catch (e) {
    console.error('Failed to load parent candidates:', e)
  }
}

// The services capability is looked up apart from the node: a failure
// only hides 服务.
async function checkServices() {
  const id = nodeId.value
  servicesAvailable.value = null
  let available = false
  try {
    available = id ? await nodeHasServicesTable(id) : false
  } catch {
    available = false
  }
  if (nodeId.value === id) servicesAvailable.value = available
}

watch(nodeId, () => {
  node.value = null
  load()
  checkServices()
}, { immediate: true })

defineExpose({ node, load, section })
</script>

<style scoped>
.node-page {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
  min-width: 0;
}

.node-page__back {
  display: inline-flex;
  gap: var(--space-1);
  align-items: center;
  min-height: 28px;
  margin-left: calc(-1 * var(--space-1));
  padding: 0 var(--space-1);
  border-radius: var(--radius-xs);
  color: var(--accent);
  font-size: var(--type-callout-size);
  text-decoration: none;
}

.node-page__back:hover {
  text-decoration: underline;
}

.node-page__back:focus-visible {
  outline: var(--focus-ring);
  outline-offset: var(--focus-ring-offset);
}

.node-page__tabs :deep(.ui-tabs__list) {
  max-width: 100%;
}

/* Touch: a 44 px hit area around the link without changing the look. */
@media (pointer: coarse) {
  .node-page__back {
    position: relative;
  }

  .node-page__back::after {
    position: absolute;
    top: 50%;
    left: 50%;
    width: max(100%, var(--size-control-lg));
    height: max(100%, var(--size-control-lg));
    transform: translate(-50%, -50%);
    content: '';
  }
}
</style>
