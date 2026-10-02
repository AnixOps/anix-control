<template>
  <NodeDetailLayout
    v-model:tab="tab"
    :title="machine ? machine.name : t('forwardNodesPage.detail.sections')"
    :description="machine ? `${machine.host}:${machine.port}` : ''"
    back-to="/admin/forward/ansible-machines"
    :back-label="t('forwardSuite.nav.ansibleMachines')"
    :sections-label="t('forwardNodesPage.detail.sections')"
    :tabs="tabs"
    :loading="loading"
    :loaded="Boolean(machine)"
    :error="error"
    :error-title="t('forwardNodesPage.ansibleDetail.loadFailed')"
    :not-found="notFound"
    :not-found-text="t('forwardNodesPage.ansibleDetail.notFound')"
    @retry="load"
  >
    <template v-if="machine" #meta>
      <UiBadge :status="machine.status === 1 ? 'online' : 'offline'" :label="machine.status === 1 ? t('runtime.shared.online') : t('runtime.shared.offline')" />
      <UiBadge v-if="!machine.enabled" status="disabled" :label="t('runtime.shared.disabled')" />
    </template>
    <template #actions>
      <UiButton :icon="Stethoscope" :loading="isPending(machine.id)" data-test="ansible-detail-check" @click="runIfIdle(checkMachine)">{{ t('runtime.ansibleMachines.actions.check') }}</UiButton>
      <UiButton variant="primary" :icon="Pencil" data-test="ansible-detail-edit" @click="editorOpen = true">{{ t('forwardNodesPage.detail.editConfig') }}</UiButton>
    </template>

    <template #overview>
      <UiGroupedList :title="t('forwardNodesPage.detail.runtime')" :footer="t('runtime.ansibleMachines.fields.reachabilityHelp')">
        <UiGroupedListRow :label="t('runtime.ansibleMachines.table.reachability')">
          <template #value>
            <UiBadge :status="machine.status === 1 ? 'online' : 'offline'" :label="machine.status === 1 ? t('runtime.shared.online') : t('runtime.shared.offline')" />
          </template>
        </UiGroupedListRow>
        <UiGroupedListRow :label="t('runtime.ansibleMachines.meta.currentConn')" :value="String(machine.currentConn)" />
        <UiGroupedListRow :label="t('runtime.nodeXTopology.meta.upload')" :value="format.bytes(machine.totalUpload)" />
        <UiGroupedListRow :label="t('runtime.nodeXTopology.meta.download')" :value="format.bytes(machine.totalDownload)" />
        <UiGroupedListRow :label="t('runtime.ansibleMachines.table.lastResult')">
          <template #value>
            <span v-if="isPending(machine.id)">{{ pendingLabel(machine) }}</span>
            <span v-else-if="results[machine.id]" :class="['fn-result', results[machine.id].success ? 'is-ok' : 'is-fail']" data-test="ansible-detail-result">{{ results[machine.id].message }}</span>
            <span v-else>{{ t('forwardNodesPage.detail.noResult') }}</span>
          </template>
        </UiGroupedListRow>
      </UiGroupedList>
      <UiGroupedList :title="t('forwardNodesPage.detail.actions')">
        <UiGroupedListRow :label="t('runtime.ansibleMachines.actions.sync')" data-test="ansible-detail-sync" @click="runIfIdle(syncMachine)" />
      </UiGroupedList>
    </template>

    <template #config>
      <UiGroupedList :title="t('forwardNodesPage.detail.identity')" :footer="t('runtime.ansibleMachines.inventoryHint')">
        <UiGroupedListRow :label="t('runtime.ansibleMachines.fields.name')" :value="machine.name" />
        <UiGroupedListRow :label="t('runtime.ansibleMachines.fields.host')">
          <template #value><code class="fn-mono">{{ machine.host }}</code></template>
        </UiGroupedListRow>
        <UiGroupedListRow :label="t('runtime.ansibleMachines.fields.reachabilityPort')" :value="machine.port || '—'" />
        <UiGroupedListRow :label="t('runtime.ansibleMachines.fields.weight')" :value="machine.weight" />
        <UiGroupedListRow :label="t('runtime.ansibleMachines.fields.region')" :value="machine.region || '—'" />
        <UiGroupedListRow :label="t('runtime.ansibleMachines.fields.isp')" :value="machine.isp || '—'" />
      </UiGroupedList>
      <UiGroupedList>
        <UiGroupedListRow :label="t('forwardNodesPage.detail.editConfig')" @click="editorOpen = true" />
      </UiGroupedList>
    </template>

    <template #danger>
      <UiGroupedList :footer="t('forwardNodesPage.detail.disableFooter')">
        <UiGroupedListRow
          :label="machine.enabled ? t('runtime.ansibleMachines.actions.disable') : t('runtime.ansibleMachines.actions.enable')"
          data-test="ansible-detail-toggle"
          @click="runIfIdle(toggleMachine)"
        />
      </UiGroupedList>
      <UiGroupedList :footer="t('forwardNodesPage.ansibleDetail.dangerFooter')">
        <UiGroupedListRow class="fn-danger-row" :label="t('runtime.ansibleMachines.modal.deleteAction')" data-test="ansible-detail-delete" @click="deleteMachine(machine)" />
      </UiGroupedList>
    </template>
  </NodeDetailLayout>

  <AnsibleMachineDialog v-if="machine" v-model:open="editorOpen" :machine-id="machine.id" @saved="onSaved" />
</template>

<script setup>
// 转发节点 › Ansible 机器 › detail (/admin/forward/ansible-machines/:id,
// UI U7): one execution machine in the detail template. Reads
// GET /admin/forward/ansible-machines/:id; actions as on the list.
import { computed, inject, ref, watch } from 'vue'
import { routerKey } from 'vue-router'
import { Pencil, Stethoscope } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { getAnsibleMachine } from '@/api/admin'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiGroupedList from '@/ui/UiGroupedList.vue'
import UiGroupedListRow from '@/ui/UiGroupedListRow.vue'
import { useFormat } from '@/ui/composables/useFormat'
import { useToast } from '@/ui/composables/useToast'
import AnsibleMachineDialog from './AnsibleMachineDialog.vue'
import NodeDetailLayout from './NodeDetailLayout.vue'
import { normalizeMachine, unwrapPayload } from './forwardNodeModel'
import { useAnsibleMachineActions } from './useAnsibleMachineActions'
import { useDetailTab } from './useDetailTab'

const props = defineProps({
  id: { type: Number, required: true }
})

const { t } = useAppI18n()
const format = useFormat()
const toast = useToast()
const router = inject(routerKey, null)

const machine = ref(null)
const loading = ref(false)
const error = ref(null)
const notFound = ref(false)
const editorOpen = ref(false)

const tab = useDetailTab(['overview', 'config', 'danger'])
const tabs = computed(() => [
  { value: 'overview', label: t('forwardNodesPage.detail.tabs.overview') },
  { value: 'config', label: t('forwardNodesPage.detail.tabs.config') },
  { value: 'danger', label: t('forwardNodesPage.detail.tabs.danger') }
])

const {
  results,
  isPending,
  pendingLabel,
  checkMachine,
  syncMachine,
  toggleMachine,
  deleteMachine,
  resolveRuntimeError
} = useAnsibleMachineActions({
  onChanged: kind => (kind === 'delete' ? router?.push('/admin/forward/ansible-machines') : load())
})

function runIfIdle(action) {
  if (machine.value && !isPending(machine.value.id)) action(machine.value)
}

async function onSaved(name) {
  await load()
  toast.success(t('runtime.ansibleMachines.messages.saved', { name }))
}

async function load() {
  loading.value = true
  error.value = null
  notFound.value = false
  try {
    const payload = unwrapPayload(await getAnsibleMachine(props.id))
    if (!payload || !payload.id) {
      machine.value = null
      notFound.value = true
    } else {
      machine.value = normalizeMachine(payload)
    }
  } catch (err) {
    if (err?.response?.status === 404) {
      machine.value = null
      notFound.value = true
    } else if (!machine.value) {
      error.value = resolveRuntimeError(err, 'forwardNodesPage.ansibleDetail.loadFailed')
    }
  } finally {
    loading.value = false
  }
}

watch(() => props.id, () => {
  machine.value = null
  load()
}, { immediate: true })
</script>

<style scoped>
.fn-mono {
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
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
