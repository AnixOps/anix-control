<template>
  <section class="deployment-center" :aria-busy="loading ? 'true' : 'false'">
    <UiPageHeader :title="t('pageTitles.admin.deployments')" :description="t('control.subtitle')">
      <template #actions>
        <UiIconButton
          data-testid="refresh-deployments"
          variant="secondary"
          :icon="RefreshCw"
          :label="t('control.actions.refresh')"
          :disabled="loading"
          :class="{ 'is-spinning': loading }"
          @click="loadInitial()"
        />
      </template>
    </UiPageHeader>

    <p v-if="error && loaded" class="page-message is-error" role="alert">{{ error }}</p>
    <p v-if="notice" class="page-message is-notice" role="status">{{ notice }}</p>

    <UiTabs
      :model-value="viewMode"
      :items="viewTabs"
      :aria-label="t('pageTitles.admin.deployments')"
      variant="segmented"
      :unmount-on-hide="false"
      data-testid="deployment-tabs"
      @update:model-value="setViewMode"
    >
      <template #topologies>
        <DeploymentTopologiesPanel
          :topologies="topologies"
          :latest-deployment-for="latestDeploymentFor"
          :loading="loading && !loaded"
          :error="loaded ? null : error"
          :can-create="scopes.length > 0"
          @create="openNewTopology"
          @edit="openTopologyEditor"
          @view-deployment="openDeploymentStatus"
          @retry="loadInitial()"
        />
      </template>
      <template #targets>
        <DeploymentAssignmentsPanel
          :nodes="nodes"
          :selected-node-i-d="selectedNodeID"
          :assignments="assignments"
          :loading="assignmentsLoading || (loading && !loaded)"
          :error="loaded ? null : error"
          :mutation-pending="targetMutationPending"
          :plugin-name="pluginName"
          :is-busy="isAssignmentBusy"
          @select-node="selectAssignmentTarget"
          @refresh="loadAssignments(selectedNodeID)"
          @create="openAssignmentDrawer()"
          @edit="openAssignmentDrawer"
          @toggle="toggleAssignment"
          @remove="removeAssignment"
          @retry="loadInitial()"
        />
      </template>
    </UiTabs>

    <section class="activity-panel" data-testid="deployment-activity">
      <OperationTimeline
        :operations="operations"
        :scoped-operation-i-ds="scopedOperationIDs"
        :busy-operation-i-d="operationBusy"
        @cancel="cancelOperation"
      />
    </section>

    <TopologyWorkspace
      :open="topologyEditor.open"
      :topology="topologyEditor.topology"
      :scopes="scopes"
      :revisions="topologyEditor.revisions"
      :revision-i-d="topologyEditor.revisionID"
      :revision-detail="topologyEditor.revisionDetail"
      :deployment-i-d="topologyEditor.deploymentID"
      :deployment-status="topologyEditor.deploymentStatus"
      :validation="topologyEditor.validation"
      :preview="topologyEditor.preview"
      :loading="topologyEditor.loading"
      :saving="topologyEditor.saving"
      :validating="topologyEditor.validating"
      :error="topologyEditor.error"
      @close="closeTopologyEditor"
      @create-topology="createTopology"
      @select-revision="selectTopologyRevision"
      @diagnose="diagnoseTopology"
      @save-revision="saveTopologyRevision"
      @preview="previewTopology"
      @plan="planTopology"
      @apply="applyDeployment"
      @rollback="rollbackDeployment"
    />

    <AssignmentDrawer
      :open="assignmentEditor.open"
      :assignment="assignmentEditor.assignment"
      :selected-node-i-d="selectedNodeID"
      :nodes="nodes"
      :plugins="assignmentResources.plugins"
      :releases="assignmentResources.releases"
      :installations="assignmentResources.installations"
      :scopes="scopes"
      :saving="assignmentEditor.saving"
      :error="assignmentEditor.error"
      @close="closeAssignmentDrawer"
      @save="saveAssignment"
    />
  </section>
</template>

<script setup>
// 部署编排 (plan §8.2, UI U8): topologies and node roles as two tabs, the
// operation timeline under them, the topology workspace (UiDialog, with a
// graph preview) and the node-role sheet (UiSheet). The state and every
// kernel request live in deployments/useDeploymentCenter.js; the tables are
// page-local panels on UiDataTable.
import { computed } from 'vue'
import { RefreshCw } from '@lucide/vue'
import AssignmentDrawer from '@/components/admin/AssignmentDrawer.vue'
import OperationTimeline from '@/components/admin/OperationTimeline.vue'
import TopologyWorkspace from '@/components/admin/TopologyWorkspace.vue'
import UiIconButton from '@/ui/UiIconButton.vue'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import UiTabs from '@/ui/UiTabs.vue'
import DeploymentAssignmentsPanel from './deployments/DeploymentAssignmentsPanel.vue'
import DeploymentTopologiesPanel from './deployments/DeploymentTopologiesPanel.vue'
import { useDeploymentCenter, VIEW_MODES } from './deployments/useDeploymentCenter'

const {
  t,
  viewMode,
  loading,
  loaded,
  error,
  notice,
  topologies,
  scopes,
  nodes,
  operations,
  selectedNodeID,
  assignments,
  assignmentsLoading,
  operationBusy,
  topologyEditor,
  assignmentEditor,
  assignmentResources,
  targetMutationPending,
  scopedOperationIDs,
  pluginName,
  latestDeploymentFor,
  loadInitial,
  setViewMode,
  loadAssignments,
  selectAssignmentTarget,
  openAssignmentDrawer,
  closeAssignmentDrawer,
  saveAssignment,
  isAssignmentBusy,
  toggleAssignment,
  removeAssignment,
  openNewTopology,
  closeTopologyEditor,
  createTopology,
  openTopologyEditor,
  selectTopologyRevision,
  diagnoseTopology,
  saveTopologyRevision,
  previewTopology,
  planTopology,
  openDeploymentStatus,
  applyDeployment,
  rollbackDeployment,
  cancelOperation,
} = useDeploymentCenter()

const viewTabs = computed(() => VIEW_MODES.map(mode => ({
  value: mode,
  label: mode === 'topologies' ? t('control.tabs.topologies') : t('control.tabs.assignments')
})))
</script>

<style scoped>
.deployment-center {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
  min-width: 0;
}

.page-message {
  margin: 0;
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-sm);
  font-size: var(--type-callout-size);
  overflow-wrap: anywhere;
}

.page-message.is-error {
  background: var(--danger-soft);
  color: var(--danger);
}

.page-message.is-notice {
  background: var(--success-soft);
  color: var(--success);
}

.activity-panel {
  min-width: 0;
}

.is-spinning :deep(svg) {
  animation: deployment-spin 0.8s linear infinite;
}

@keyframes deployment-spin {
  to {
    transform: rotate(360deg);
  }
}

@media (prefers-reduced-motion: reduce) {
  .is-spinning :deep(svg) {
    animation: none;
  }
}
</style>
