<template>
  <UiDialog
    :open="open"
    size="lg"
    data-testid="topology-workspace"
    :description="createMode ? '' : `${topology?.name || topology?.id || ''} / ${t('control.topology.revision')} ${selectedRevisionID || '-'}`"
    :dismissible="canClose"
    @update:open="value => { if (!value) requestClose() }"
  >
    <template #title>
      <span :id="createMode ? 'new-topology-title' : 'topology-editor-title'">{{ createMode ? t('control.topology.newTitle') : t('control.topology.editorTitle') }}</span>
    </template>
    <template v-if="createMode">
      <form class="creation-form" @submit.prevent="createTopology">
        <UiTextField
          id="new-topology-name"
          v-model.trim="newTopology.name"
          size="md"
          :label="t('control.topology.name')"
          maxlength="160"
          :disabled="saving"
        />
        <UiSelect
          id="new-topology-scope"
          v-model="newTopology.serviceScope"
          class="form-select"
          size="md"
          :label="t('control.table.scope')"
          :options="scopeOptions"
          :disabled="saving"
        />
        <UiTextarea
          id="new-topology-description"
          v-model.trim="newTopology.description"
          :label="t('control.table.description')"
          :rows="3"
          :disabled="saving"
        />
        <p v-if="visibleError" class="workspace-error" role="alert">{{ visibleError }}</p>
      </form>
    </template>
    <template v-else>
      <div class="workspace-body" :aria-busy="loading ? 'true' : undefined">
        <div class="editor-toolbar">
          <UiSelect
            id="topology-revision-selector"
            class="form-select"
            size="md"
            :model-value="selectedRevisionID"
            :label="t('control.topology.revision')"
            :options="revisionOptions"
            :disabled="loading || saving || revisions.length === 0"
            @update:model-value="selectRevision"
          />
          <UiTextField
            id="topology-rollout-group"
            v-model.trim="rolloutGroup"
            size="md"
            :label="t('control.table.rolloutGroup')"
            autocomplete="off"
            :disabled="saving"
          />
          <UiSelect
            id="topology-failure-policy"
            v-model="failurePolicy"
            class="form-select"
            size="md"
            :label="t('control.topology.failurePolicy')"
            :options="failurePolicyOptions"
            :disabled="saving"
          />
        </div>

        <UiTextField
          id="topology-revision-message"
          v-model.trim="message"
          size="md"
          :label="t('control.topology.message')"
          maxlength="500"
          :disabled="saving"
        />
        <UiSegmentedControl
          v-model="graphView"
          class="graph-view-switch"
          size="sm"
          :aria-label="t('control.topology.viewLabel')"
          :options="[{ value: 'json', label: t('control.topology.viewJSON') }, { value: 'graph', label: t('control.topology.viewGraph') }]"
          data-testid="topology-view-switch"
        />
        <UiTextarea
          v-show="graphView === 'json'"
          id="topology-editor-json"
          v-model="json"
          class="json-textarea"
          :label="t('control.topology.graphJSON')"
          :help="t('control.topology.graphHelp')"
          :rows="16"
          spellcheck="false"
          :disabled="saving"
        />
        <TopologyGraph
          v-if="graphView === 'graph'"
          data-testid="topology-graph-preview"
          layout="dagre"
          :height="360"
          :nodes="graphPreview.nodes"
          :edges="graphPreview.edges"
          :label="t('control.topology.graphLabel')"
          :summary="t('control.topology.graphSummary', { nodes: graphPreview.nodes.length, edges: graphPreview.edges.length })"
          :empty-title="graphPreview.invalid ? t('control.topology.invalidJSON') : t('control.topology.graphEmpty')"
          :empty-description="graphPreview.invalid ? t('control.topology.graphInvalidHint') : t('control.topology.graphEmptyHint')"
          :error-title="t('control.topology.graphFailed')"
        />

        <p v-if="dirty" class="topology-dirty" role="status">{{ t('control.topology.unsavedChanges') }}</p>
        <section v-if="validation" class="topology-validation" :class="validation.valid ? 'is-valid' : 'is-invalid'" role="status">
          <strong>{{ validation.valid ? t('control.topology.valid') : t('control.topology.invalid') }}</strong>
          <ul v-if="validation.issues?.length">
            <li v-for="(issue, index) in validation.issues" :key="`${issue.code || 'issue'}-${index}`">{{ issue.message || issue.code }}</li>
          </ul>
          <div v-if="validation.checks?.length" class="topology-checks">
            <span v-for="check in validation.checks" :key="check.name" :class="['check-pill', check.status === 'passed' ? 'check-passed' : 'check-failed']">
              {{ check.name }}: {{ check.status }}
            </span>
          </div>
        </section>
        <section v-if="preview" class="topology-preview" role="status">
          <strong>{{ t('control.topology.preview') }}</strong>
          <span class="secondary-cell">{{ t('control.topology.previewSteps', { count: preview.steps?.length || 0 }) }}</span>
          <ol v-if="preview.steps?.length" class="preview-steps">
            <li v-for="step in preview.steps" :key="`${step.order}-${step.vertex_key}`">
              <code>{{ step.vertex_key }}</code> / {{ step.apply_action }} / {{ step.rollback_mode }}<span v-if="step.config_hash"> / {{ shortHash(step.config_hash) }}</span>
            </li>
          </ol>
        </section>
        <section v-if="deploymentStatus" class="deployment-status" role="status">
          <div class="status-line">
            <strong>{{ t('control.topology.deployment') }}</strong>
            <code>#{{ deploymentID }}</code>
            <span :class="['state-badge', stateClass(deploymentState)]">{{ deploymentState || '-' }}</span>
          </div>
          <p v-if="deploymentStatus.deployment?.last_error" class="row-error">{{ deploymentStatus.deployment.last_error }}</p>
          <ul v-if="deploymentStatus.operations?.length" class="deployment-timeline">
            <li v-for="operation in deploymentStatus.operations.slice(-8)" :key="operation.operation_id || operation.id">
              <code>{{ operation.kind }}</code>
              <span :class="['state-badge', stateClass(operation.state)]">{{ operation.state }}</span>
              <span v-if="operation.last_error" class="row-error">{{ operation.last_error }}</span>
            </li>
          </ul>
        </section>
        <p v-if="visibleError" class="workspace-error" role="alert">{{ visibleError }}</p>
      </div>
    </template>

    <template #footer>
      <template v-if="createMode">
        <UiButton :disabled="saving" @click="requestClose">{{ t('common.actions.cancel') }}</UiButton>
        <UiButton id="create-topology" variant="primary" :loading="saving" :disabled="!newTopology.name || !newTopology.serviceScope" @click="createTopology">
          {{ t('control.topology.create') }}
        </UiButton>
      </template>
      <template v-else>
        <UiButton :disabled="saving || loading" @click="requestClose">{{ t('common.actions.cancel') }}</UiButton>
        <UiButton id="topology-diagnose" :disabled="validating || loading" @click="diagnose">
          {{ validating ? t('control.topology.validating') : t('control.topology.diagnose') }}
        </UiButton>
        <UiButton id="topology-preview" :disabled="validating || loading || dirty || !selectedRevisionID" @click="previewRevision">{{ t('control.topology.previewAction') }}</UiButton>
        <UiButton id="topology-save-revision" variant="primary" :disabled="saving || loading" @click="saveRevision">{{ saving ? t('control.actions.saving') : t('control.topology.saveRevision') }}</UiButton>
        <UiButton id="topology-plan" :disabled="saving || dirty || !selectedRevisionID || confirmationActive" @click="plan">{{ t('control.topology.plan') }}</UiButton>
        <UiButton id="topology-apply" variant="primary" :disabled="saving || dirty || !canApply || applyConfirming || rollbackConfirming" @click="requestApply">{{ t('control.topology.apply') }}</UiButton>
        <UiButton id="topology-rollback" variant="danger-soft" :disabled="saving || dirty || !canRollback || applyConfirming || rollbackConfirming" @click="requestRollback">{{ t('control.topology.rollback') }}</UiButton>
        <section v-if="applyConfirming" class="action-confirmation apply-confirmation" data-testid="apply-confirmation" role="alertdialog" :aria-label="t('control.topology.apply')">
          <p>{{ t('control.topology.applyConfirm', { topology: confirmedTopologyLabel, deployment: confirmedDeploymentID }) }}</p>
          <div class="confirmation-actions">
            <UiButton size="sm" :disabled="saving" @click="clearConfirmation">{{ t('common.actions.cancel') }}</UiButton>
            <UiButton size="sm" variant="primary" data-testid="confirm-apply" :disabled="saving || dirty || !canApply || !confirmationMatchesCurrentDeployment" @click="apply">{{ t('control.topology.apply') }}</UiButton>
          </div>
        </section>
        <section v-if="rollbackConfirming" class="action-confirmation rollback-confirmation" data-testid="rollback-confirmation" role="alertdialog" :aria-label="t('control.topology.rollback')">
          <p>{{ t('control.topology.rollbackConfirm', { topology: confirmedTopologyLabel, deployment: confirmedDeploymentID }) }}</p>
          <div class="confirmation-actions">
            <UiButton size="sm" :disabled="saving" @click="clearConfirmation">{{ t('common.actions.cancel') }}</UiButton>
            <UiButton size="sm" variant="danger" data-testid="confirm-rollback" :disabled="saving || dirty || !confirmationMatchesCurrentDeployment" @click="rollback">{{ t('control.topology.rollback') }}</UiButton>
          </div>
        </section>
      </template>
    </template>
  </UiDialog>
</template>

<script setup>
// Topology create / revision editor (UiDialog: focus trap, Esc, focus
// return). It cannot be dismissed while saving or validating. JSON / 图示
// switches the editor for a read-only G6 preview of the JSON being edited
// (UI U8): G6 loads only when the preview opens, and the graph follows the
// light / dark theme.
import { computed, reactive, ref, watch } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { topologyInputFromJSON, topologyJSONFromDetail } from '@/composables/useKernelDeployments'
import UiButton from '@/ui/UiButton.vue'
import UiDialog from '@/ui/UiDialog.vue'
import UiSegmentedControl from '@/ui/UiSegmentedControl.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiTextarea from '@/ui/UiTextarea.vue'
import UiTextField from '@/ui/UiTextField.vue'
import TopologyGraph from './TopologyGraph.vue'

const props = defineProps({
  open: { type: Boolean, default: false },
  topology: { type: Object, default: null },
  scopes: { type: Array, default: () => [] },
  revisions: { type: Array, default: () => [] },
  revisionID: { type: Number, default: 0 },
  revisionDetail: { type: Object, default: null },
  deploymentID: { type: Number, default: 0 },
  deploymentStatus: { type: Object, default: null },
  validation: { type: Object, default: null },
  preview: { type: Object, default: null },
  loading: { type: Boolean, default: false },
  saving: { type: Boolean, default: false },
  validating: { type: Boolean, default: false },
  error: { type: String, default: '' },
})
const emit = defineEmits(['close', 'create-topology', 'save-revision', 'diagnose', 'preview', 'plan', 'apply', 'rollback', 'select-revision'])
const { t } = useAppI18n()
const json = ref(emptyTopologyJSON())
const baselineJSON = ref(emptyTopologyJSON())
const message = ref('')
const baselineMessage = ref('')
const selectedRevisionID = ref(0)
const rolloutGroup = ref('')
const failurePolicy = ref('stop_and_rollback')
const scopeOptions = computed(() => props.scopes.map(scope => ({ value: scope.id, label: `${scope.name || scope.id} (${scope.id})` })))
// With no revision yet the select shows 「暂无 revision」 as its only choice.
const revisionOptions = computed(() => (props.revisions.length === 0
  ? [{ value: 0, label: t('control.topology.noRevisions') }]
  : props.revisions.map(revision => ({ value: Number(revision.id), label: `r${revision.revision} (#${revision.id})` }))))
const failurePolicyOptions = computed(() => [{ value: 'stop_and_rollback', label: t('control.topology.stopAndRollback') }])
const applyConfirming = ref(false)
const rollbackConfirming = ref(false)
const confirmedDeploymentID = ref(0)
const confirmedTopologyLabel = ref('')
const localError = ref('')
const newTopology = reactive({ name: '', serviceScope: '', description: '' })
const baselineTopologyID = ref(0)
const baselineRevisionID = ref(0)

const createMode = computed(() => !props.topology?.id)
const canClose = computed(() => !props.saving && !props.validating)
const dirty = computed(() => !createMode.value && props.open && (
  json.value !== baselineJSON.value || message.value.trim() !== baselineMessage.value.trim()
))
const deploymentState = computed(() => props.deploymentStatus?.deployment?.state || props.deploymentStatus?.state || '')
const canApply = computed(() => Boolean(props.deploymentID) && ['planned', 'applying'].includes(deploymentState.value))
const canRollback = computed(() => Boolean(props.deploymentID) && ![
  '', 'rollback_requested', 'rollback_configuring', 'rollback_enabling', 'rollback_disabling', 'rolled_back',
].includes(deploymentState.value))
const topologyLabel = computed(() => props.topology?.name || `#${props.topology?.id || props.deploymentID}`)
const confirmationActive = computed(() => applyConfirming.value || rollbackConfirming.value)
const confirmationMatchesCurrentDeployment = computed(() => (
  confirmedDeploymentID.value > 0 && Number(props.deploymentID) === confirmedDeploymentID.value
))
const visibleError = computed(() => localError.value || props.error)
const graphView = ref('json')
// Vertices → nodes (coloured by kind, a stable palette slot per kind),
// edges → links labelled with their protocol. Invalid JSON → empty state.
const graphPreview = computed(() => {
  let value
  try {
    value = JSON.parse(json.value || '{}')
  } catch {
    return { nodes: [], edges: [], invalid: true }
  }
  const kinds = []
  const nodes = (Array.isArray(value?.vertices) ? value.vertices : [])
    .filter(vertex => vertex && String(vertex.key || '').trim())
    .map(vertex => {
      const kind = String(vertex.kind || '')
      if (!kinds.includes(kind)) kinds.push(kind)
      return {
        id: String(vertex.key).trim(),
        label: String(vertex.key).trim(),
        detail: vertex.role || vertex.plugin_id || kind,
        tone: `chart-${(kinds.indexOf(kind) % 7) + 1}`,
      }
    })
  const edges = (Array.isArray(value?.edges) ? value.edges : []).map((edge, index) => ({
    id: `${edge?.source_key}-${edge?.target_key}-${index}`,
    source: String(edge?.source_key || '').trim(),
    target: String(edge?.target_key || '').trim(),
    label: edge?.protocol || '',
  }))
  return { nodes, edges, invalid: false }
})
function requestClose() {
  if (canClose.value) emit('close')
}

watch([
  () => props.open,
  () => props.topology?.id,
  () => props.revisionID,
  () => props.revisionDetail,
  () => props.deploymentID,
  () => props.deploymentStatus?.deployment?.id,
  () => deploymentState.value,
], ([isOpen, topologyID, revisionID, detail]) => {
  if (!isOpen) {
    graphView.value = 'json'
    clearConfirmation()
    baselineTopologyID.value = 0
    baselineRevisionID.value = 0
    return
  }
  localError.value = ''
  clearConfirmation()
  if (!topologyID) {
    Object.assign(newTopology, { name: '', serviceScope: props.scopes[0]?.id || '', description: '' })
    baselineTopologyID.value = 0
    baselineRevisionID.value = 0
    return
  }

  const nextTopologyID = Number(topologyID)
  const nextRevisionID = Number(revisionID || detail?.revision?.id || 0)
  const detailRevisionID = Number(detail?.revision?.id || 0)
  if (detail && nextRevisionID && detailRevisionID && nextRevisionID !== detailRevisionID) return

  selectedRevisionID.value = nextRevisionID
  const topologyChanged = baselineTopologyID.value !== nextTopologyID
  const revisionChanged = baselineRevisionID.value !== nextRevisionID
  if (topologyChanged || revisionChanged) {
    clearConfirmation()
    rolloutGroup.value = ''
    failurePolicy.value = 'stop_and_rollback'
    if (detail) {
      setRevisionDetail(detail)
      baselineTopologyID.value = nextTopologyID
      baselineRevisionID.value = nextRevisionID
    } else if (topologyChanged || baselineTopologyID.value === 0) {
      setRevisionDetail(null)
      baselineTopologyID.value = nextTopologyID
      baselineRevisionID.value = nextRevisionID
    }
    return
  }

  if (!dirty.value && detail) setRevisionDetail(detail)
}, { immediate: true })

function emptyTopologyJSON() {
  return '{\n  "vertices": [],\n  "edges": []\n}'
}

function setRevisionDetail(detail) {
  json.value = detail ? topologyJSONFromDetail(detail) : emptyTopologyJSON()
  message.value = detail?.revision?.message || ''
  baselineJSON.value = json.value
  baselineMessage.value = message.value
}

function input() {
  localError.value = ''
  try {
    return topologyInputFromJSON(json.value, message.value, t('control.topology.invalidJSON'))
  } catch (cause) {
    localError.value = cause.message || t('control.topology.invalidJSON')
    return null
  }
}

function options() {
  return {
    rolloutGroup: rolloutGroup.value,
    failurePolicy: failurePolicy.value,
  }
}

function selectRevision(value) {
  if (value !== undefined) selectedRevisionID.value = Number(value) || 0
  clearConfirmation()
  emit('select-revision', { topologyID: Number(props.topology.id), revisionID: selectedRevisionID.value })
}

function createTopology() {
  if (props.saving || !newTopology.name || !newTopology.serviceScope) return
  emit('create-topology', {
    name: newTopology.name,
    service_scope: newTopology.serviceScope,
    description: newTopology.description,
  })
}

function saveRevision() {
  if (props.saving || props.loading || !props.topology?.id) return
  const revisionInput = input()
  if (!revisionInput) return
  emit('save-revision', { topologyID: Number(props.topology.id), input: revisionInput })
}

function diagnose() {
  if (props.validating || props.loading || !props.topology?.id) return
  const revisionInput = input()
  if (!revisionInput) return
  emit('diagnose', {
    topologyID: Number(props.topology.id),
    revisionID: selectedRevisionID.value,
    input: revisionInput,
    options: options(),
  })
}

function previewRevision() {
  if (props.validating || props.loading || dirty.value || !props.topology?.id || !selectedRevisionID.value) return
  emit('preview', { topologyID: Number(props.topology.id), revisionID: selectedRevisionID.value, options: options() })
}

function plan() {
  if (props.saving || dirty.value || confirmationActive.value || !props.topology?.id || !selectedRevisionID.value) return
  emit('plan', { topologyID: Number(props.topology.id), revisionID: selectedRevisionID.value, options: options() })
}

function requestApply() {
  if (props.saving || dirty.value || !canApply.value || confirmationActive.value) return
  beginConfirmation('apply')
}

function apply() {
  if (props.saving || dirty.value || !applyConfirming.value || !canApply.value || !confirmationMatchesCurrentDeployment.value) {
    clearConfirmation()
    return
  }
  const deploymentID = confirmedDeploymentID.value
  clearConfirmation()
  emit('apply', { deploymentID })
}

function requestRollback() {
  if (props.saving || dirty.value || !canRollback.value || confirmationActive.value) return
  beginConfirmation('rollback')
}

function rollback() {
  if (props.saving || dirty.value || !rollbackConfirming.value || !canRollback.value || !confirmationMatchesCurrentDeployment.value) {
    clearConfirmation()
    return
  }
  const deploymentID = confirmedDeploymentID.value
  clearConfirmation()
  emit('rollback', { deploymentID })
}

function beginConfirmation(action) {
  const deploymentID = Number(props.deploymentID)
  if (!deploymentID) return
  clearConfirmation()
  confirmedDeploymentID.value = deploymentID
  confirmedTopologyLabel.value = topologyLabel.value
  applyConfirming.value = action === 'apply'
  rollbackConfirming.value = action === 'rollback'
}

function clearConfirmation() {
  applyConfirming.value = false
  rollbackConfirming.value = false
  confirmedDeploymentID.value = 0
  confirmedTopologyLabel.value = ''
}

function shortHash(value) {
  return typeof value === 'string' && value.length > 12 ? value.slice(0, 12) : value || '-'
}

function stateClass(state) {
  if (state === 'healthy' || state === 'enabled' || state === 'succeeded' || state === 'completed') return 'state-active'
  if (state === 'failed' || state === 'superseded' || state === 'disabled' || state === 'cancel_requested' || state === 'cancelled' || state === 'timed_out') return 'state-error'
  return 'state-pending'
}
</script>

<style scoped>
.workspace-body { display: grid; gap: var(--space-4); }
.creation-form { display: grid; gap: var(--space-4); }
.editor-toolbar { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: var(--space-3); }
.form-select { min-width: 0; }
.json-textarea :deep(textarea) { font-family: var(--font-mono); font-size: var(--type-caption-size); }
.graph-view-switch { justify-self: start; }
.workspace-error { margin: 0; color: var(--danger); overflow-wrap: anywhere; }
.topology-dirty { margin: 0; padding: var(--space-2) var(--space-3); border-left: 3px solid var(--warning); color: var(--warning); background: var(--warning-soft); font-size: var(--type-caption-size); }
.topology-validation, .topology-preview, .deployment-status { padding: var(--space-3); border-left: 3px solid var(--warning); background: var(--warning-soft); }
.topology-validation.is-valid { border-left-color: var(--success); background: var(--success-soft); }
.topology-validation.is-invalid { border-left-color: var(--danger); background: var(--danger-soft); }
.topology-preview { border-left-color: var(--accent); background: var(--accent-soft); }
.topology-validation ul, .preview-steps, .deployment-timeline { margin: var(--space-2) 0 0; padding-left: var(--space-5); }
.topology-checks, .status-line, .deployment-timeline li { display: flex; align-items: center; gap: var(--space-2); flex-wrap: wrap; }
.topology-checks { margin-top: var(--space-2); }
.check-pill { display: inline-flex; padding: var(--space-1) var(--space-2); border-radius: var(--radius-xs); font-size: var(--type-caption-size); }
.check-passed { color: var(--success); background: var(--success-soft); }
.check-failed { color: var(--danger); background: var(--danger-soft); }
.secondary-cell, .row-error { display: block; }
.secondary-cell { margin-top: var(--space-1); color: var(--label-2); font-size: var(--type-caption-size); }
.row-error { color: var(--danger); overflow-wrap: anywhere; }
.preview-steps, .deployment-timeline { max-height: 180px; overflow: auto; }
.deployment-timeline li + li { margin-top: var(--space-2); }
.status-line strong { margin-right: auto; }
.state-badge { display: inline-flex; border-radius: var(--radius-xs); padding: var(--space-1) var(--space-2); font-size: var(--type-caption-size); }
.state-active { color: var(--success); background: var(--success-soft); }
.state-error { color: var(--danger); background: var(--danger-soft); }
.state-pending { color: var(--warning); background: var(--warning-soft); }
.action-confirmation { width: 100%; border: 1px solid var(--separator); border-radius: var(--radius-sm); padding: var(--space-3); }
.apply-confirmation { border-color: var(--accent); background: var(--accent-soft); }
.rollback-confirmation { border-color: var(--danger); background: var(--danger-soft); }
.action-confirmation p { margin: 0 0 var(--space-3); }
.confirmation-actions { display: flex; justify-content: flex-end; gap: var(--space-2); }
@media (max-width: 720px) {
  .editor-toolbar { grid-template-columns: 1fr; }
}
</style>
