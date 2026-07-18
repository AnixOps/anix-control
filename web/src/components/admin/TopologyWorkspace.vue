<template>
  <div v-if="open" class="topology-backdrop" @click.self="requestClose">
    <section
      ref="modal"
      class="topology-workspace"
      data-testid="topology-workspace"
      role="dialog"
      aria-modal="true"
      :aria-labelledby="createMode ? 'new-topology-title' : 'topology-editor-title'"
      @keydown="handleKeydown"
    >
      <template v-if="createMode">
        <header class="workspace-header">
          <h2 id="new-topology-title">{{ t('control.topology.newTitle') }}</h2>
          <button
            ref="closeButton"
            class="icon-button"
            type="button"
            :aria-label="t('common.actions.close')"
            :title="t('common.actions.close')"
            :disabled="saving"
            @click="requestClose"
          >
            <X :size="20" aria-hidden="true" />
            <span class="sr-only">{{ t('common.actions.close') }}</span>
          </button>
        </header>
        <form class="workspace-body creation-form" @submit.prevent="createTopology">
          <div class="form-group">
            <label for="new-topology-name">{{ t('control.topology.name') }}</label>
            <input id="new-topology-name" v-model.trim="newTopology.name" type="text" maxlength="160" :disabled="saving" />
          </div>
          <div class="form-group">
            <label for="new-topology-scope">{{ t('control.table.scope') }}</label>
            <select id="new-topology-scope" v-model="newTopology.serviceScope" :disabled="saving">
              <option v-for="scope in scopes" :key="scope.id" :value="scope.id">{{ scope.name || scope.id }} ({{ scope.id }})</option>
            </select>
          </div>
          <div class="form-group">
            <label for="new-topology-description">{{ t('control.table.description') }}</label>
            <textarea id="new-topology-description" v-model.trim="newTopology.description" rows="3" :disabled="saving"></textarea>
          </div>
          <p v-if="visibleError" class="workspace-error" role="alert">{{ visibleError }}</p>
        </form>
        <footer class="workspace-footer">
          <button class="btn" type="button" :disabled="saving" @click="requestClose">{{ t('common.actions.cancel') }}</button>
          <button id="create-topology" class="btn btn-primary" type="button" :disabled="saving || !newTopology.name || !newTopology.serviceScope" @click="createTopology">
            {{ saving ? t('control.actions.saving') : t('control.topology.create') }}
          </button>
        </footer>
      </template>

      <template v-else>
        <header class="workspace-header">
          <div>
            <h2 id="topology-editor-title">{{ t('control.topology.editorTitle') }}</h2>
            <p class="workspace-meta">{{ topology?.name || topology?.id }} / {{ t('control.topology.revision') }} {{ selectedRevisionID || '-' }}</p>
          </div>
          <button
            ref="closeButton"
            class="icon-button"
            type="button"
            :aria-label="t('common.actions.close')"
            :title="t('common.actions.close')"
            :disabled="saving || validating"
            @click="requestClose"
          >
            <X :size="20" aria-hidden="true" />
            <span class="sr-only">{{ t('common.actions.close') }}</span>
          </button>
        </header>

        <div class="workspace-body" :aria-busy="loading ? 'true' : undefined">
          <div class="editor-toolbar">
            <div class="form-group">
              <label for="topology-revision-selector">{{ t('control.topology.revision') }}</label>
              <select id="topology-revision-selector" v-model.number="selectedRevisionID" :disabled="loading || saving" @change="selectRevision">
                <option v-if="revisions.length === 0" :value="0">{{ t('control.topology.noRevisions') }}</option>
                <option v-for="revision in revisions" :key="revision.id" :value="Number(revision.id)">r{{ revision.revision }} (#{{ revision.id }})</option>
              </select>
            </div>
            <div class="form-group">
              <label for="topology-rollout-group">{{ t('control.table.rolloutGroup') }}</label>
              <input id="topology-rollout-group" v-model.trim="rolloutGroup" type="text" autocomplete="off" :disabled="saving" />
            </div>
            <div class="form-group">
              <label for="topology-failure-policy">{{ t('control.topology.failurePolicy') }}</label>
              <select id="topology-failure-policy" v-model="failurePolicy" :disabled="saving">
                <option value="stop_and_rollback">{{ t('control.topology.stopAndRollback') }}</option>
              </select>
            </div>
          </div>

          <div class="form-group">
            <label for="topology-revision-message">{{ t('control.topology.message') }}</label>
            <input id="topology-revision-message" v-model.trim="message" type="text" maxlength="500" :disabled="saving" />
          </div>
          <div class="form-group">
            <label for="topology-editor-json">{{ t('control.topology.graphJSON') }}</label>
            <textarea id="topology-editor-json" v-model="json" rows="16" spellcheck="false" class="json-textarea" :disabled="saving"></textarea>
            <p class="field-help">{{ t('control.topology.graphHelp') }}</p>
          </div>

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

        <footer class="workspace-footer editor-footer">
          <button class="btn" type="button" :disabled="saving || loading" @click="requestClose">{{ t('common.actions.cancel') }}</button>
          <button id="topology-diagnose" class="btn" type="button" :disabled="validating || loading" @click="diagnose">
            {{ validating ? t('control.topology.validating') : t('control.topology.diagnose') }}
          </button>
          <button id="topology-preview" class="btn" type="button" :disabled="validating || loading || dirty || !selectedRevisionID" @click="previewRevision">{{ t('control.topology.previewAction') }}</button>
          <button id="topology-save-revision" class="btn btn-primary" type="button" :disabled="saving || loading" @click="saveRevision">{{ saving ? t('control.actions.saving') : t('control.topology.saveRevision') }}</button>
          <button id="topology-plan" class="btn" type="button" :disabled="saving || dirty || !selectedRevisionID" @click="plan">{{ t('control.topology.plan') }}</button>
          <button id="topology-apply" class="btn btn-primary" type="button" :disabled="saving || dirty || !canApply || applyConfirming || rollbackConfirming" @click="requestApply">{{ t('control.topology.apply') }}</button>
          <button id="topology-rollback" class="btn btn-danger" type="button" :disabled="saving || dirty || !canRollback || applyConfirming || rollbackConfirming" @click="requestRollback">{{ t('control.topology.rollback') }}</button>
          <section v-if="applyConfirming" class="action-confirmation apply-confirmation" data-testid="apply-confirmation" role="alertdialog" :aria-label="t('control.topology.apply')">
            <p>{{ t('control.topology.applyConfirm', { topology: topologyLabel, deployment: deploymentID }) }}</p>
            <div class="confirmation-actions">
              <button class="btn" type="button" :disabled="saving" @click="applyConfirming = false">{{ t('common.actions.cancel') }}</button>
              <button class="btn btn-primary" data-testid="confirm-apply" type="button" :disabled="saving || dirty || !canApply" @click="apply">{{ t('control.topology.apply') }}</button>
            </div>
          </section>
          <section v-if="rollbackConfirming" class="action-confirmation rollback-confirmation" data-testid="rollback-confirmation" role="alertdialog" :aria-label="t('control.topology.rollback')">
            <p>{{ t('control.topology.rollbackConfirm', { topology: topologyLabel, deployment: deploymentID }) }}</p>
            <div class="confirmation-actions">
              <button class="btn" type="button" :disabled="saving" @click="rollbackConfirming = false">{{ t('common.actions.cancel') }}</button>
              <button class="btn btn-danger" data-testid="confirm-rollback" type="button" :disabled="saving || dirty" @click="rollback">{{ t('control.topology.rollback') }}</button>
            </div>
          </section>
        </footer>
      </template>
    </section>
  </div>
</template>

<script setup>
import { computed, reactive, ref, watch } from 'vue'
import { X } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { topologyInputFromJSON, topologyJSONFromDetail } from '@/composables/useKernelDeployments'
import { useModalFocus } from '@/composables/useModalFocus'

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
const closeButton = ref(null)
const modal = ref(null)
const json = ref(emptyTopologyJSON())
const baselineJSON = ref(emptyTopologyJSON())
const message = ref('')
const baselineMessage = ref('')
const selectedRevisionID = ref(0)
const rolloutGroup = ref('')
const failurePolicy = ref('stop_and_rollback')
const applyConfirming = ref(false)
const rollbackConfirming = ref(false)
const localError = ref('')
const newTopology = reactive({ name: '', serviceScope: '', description: '' })
const baselineTopologyID = ref(0)
const baselineRevisionID = ref(0)

const createMode = computed(() => !props.topology?.id)
const openState = computed(() => props.open)
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
const visibleError = computed(() => localError.value || props.error)
const { handleKeydown, requestClose } = useModalFocus({
  open: openState,
  canClose,
  container: modal,
  initialFocus: closeButton,
  close: () => emit('close'),
})

watch([() => props.open, () => props.topology?.id, () => props.revisionID, () => props.revisionDetail], ([isOpen, topologyID, revisionID, detail]) => {
  if (!isOpen) {
    baselineTopologyID.value = 0
    baselineRevisionID.value = 0
    return
  }
  localError.value = ''
  applyConfirming.value = false
  rollbackConfirming.value = false
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
    applyConfirming.value = false
    rollbackConfirming.value = false
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

function selectRevision() {
  applyConfirming.value = false
  rollbackConfirming.value = false
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
  if (props.saving || dirty.value || !props.topology?.id || !selectedRevisionID.value) return
  emit('plan', { topologyID: Number(props.topology.id), revisionID: selectedRevisionID.value, options: options() })
}

function requestApply() {
  if (props.saving || dirty.value || !canApply.value || applyConfirming.value) return
  rollbackConfirming.value = false
  applyConfirming.value = true
}

function apply() {
  if (props.saving || dirty.value || !applyConfirming.value || !canApply.value) return
  applyConfirming.value = false
  emit('apply', { deploymentID: Number(props.deploymentID) })
}

function requestRollback() {
  if (props.saving || dirty.value || !canRollback.value || rollbackConfirming.value) return
  applyConfirming.value = false
  rollbackConfirming.value = true
}

function rollback() {
  if (props.saving || dirty.value || !rollbackConfirming.value || !canRollback.value) return
  applyConfirming.value = false
  rollbackConfirming.value = false
  emit('rollback', { deploymentID: Number(props.deploymentID) })
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
.topology-backdrop { position: fixed; inset: 0; z-index: 1200; display: flex; align-items: center; justify-content: center; padding: 20px; background: rgba(15, 23, 42, .62); }
.topology-workspace { display: flex; width: min(100%, 980px); max-height: calc(100dvh - 40px); flex-direction: column; overflow-y: auto; border: 1px solid var(--border-color); border-radius: 8px; background: var(--surface-color); color: var(--text-color); box-shadow: 0 18px 32px rgba(15, 23, 42, .2); }
.workspace-header, .workspace-footer { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; padding: 18px 20px; border-bottom: 1px solid var(--border-color); }
.workspace-header h2 { margin: 0; font-size: 18px; line-height: 1.35; }
.workspace-meta { margin: 4px 0 0; color: var(--text-secondary); font-size: 13px; }
.workspace-body { display: grid; gap: 16px; padding: 20px; }
.creation-form { min-width: min(100%, 480px); }
.editor-toolbar { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; }
.form-group { display: grid; min-width: 0; gap: 8px; }
.form-group label { color: var(--text-secondary); font-size: 12px; font-weight: 700; }
.form-group input, .form-group select, .form-group textarea { box-sizing: border-box; min-height: 38px; width: 100%; border: 1px solid var(--border-color); border-radius: 6px; background: var(--bg-color); color: var(--text-color); padding: 8px 10px; }
.form-group textarea { resize: vertical; }
.json-textarea { font: 12px/1.5 ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-weight: 500; }
.field-help, .workspace-error { margin: 0; overflow-wrap: anywhere; }
.field-help { color: var(--text-secondary); font-size: 12px; }
.workspace-error { color: var(--error-color); }
.topology-dirty { margin: 0; padding: 9px 12px; border-left: 3px solid var(--warning-color); color: var(--warning-color); background: rgba(217, 119, 6, .08); font-size: 12px; }
.topology-validation, .topology-preview, .deployment-status { padding: 10px 12px; border-left: 3px solid var(--warning-color); background: rgba(217, 119, 6, .08); }
.topology-validation.is-valid { border-left-color: var(--success-color); background: rgba(22, 163, 74, .08); }
.topology-validation.is-invalid { border-left-color: var(--error-color); background: rgba(220, 38, 38, .08); }
.topology-preview { border-left-color: var(--primary-color); background: rgba(37, 99, 235, .06); }
.topology-validation ul, .preview-steps, .deployment-timeline { margin: 8px 0 0; padding-left: 20px; }
.topology-checks, .status-line, .deployment-timeline li { display: flex; align-items: center; gap: 7px; flex-wrap: wrap; }
.topology-checks { margin-top: 9px; }
.check-pill { display: inline-flex; padding: 3px 7px; border-radius: 6px; font-size: 11px; }
.check-passed { color: var(--success-color); background: rgba(22, 163, 74, .1); }
.check-failed { color: var(--error-color); background: rgba(220, 38, 38, .1); }
.secondary-cell, .row-error { display: block; }
.secondary-cell { margin-top: 3px; color: var(--text-secondary); font-size: 11px; }
.row-error { color: var(--error-color); overflow-wrap: anywhere; }
.preview-steps, .deployment-timeline { max-height: 180px; overflow: auto; }
.deployment-timeline li + li { margin-top: 6px; }
.status-line strong { margin-right: auto; }
.state-badge { display: inline-flex; border-radius: 6px; padding: 3px 7px; font-size: 12px; }
.state-active { color: var(--success-color); background: rgba(22, 163, 74, .1); }
.state-error { color: var(--error-color); background: rgba(220, 38, 38, .1); }
.state-pending { color: var(--warning-color); background: rgba(217, 119, 6, .1); }
.workspace-footer { position: relative; align-items: center; justify-content: flex-end; flex-wrap: wrap; border-top: 1px solid var(--border-color); border-bottom: 0; }
.editor-footer { padding-right: 20px; }
.action-confirmation { width: 100%; border: 1px solid var(--border-color); border-radius: 6px; padding: 10px 12px; }
.apply-confirmation { border-color: var(--primary-color); background: rgba(37, 99, 235, .06); }
.rollback-confirmation { border-color: var(--error-color); background: rgba(220, 38, 38, .06); }
.action-confirmation p { margin: 0 0 10px; }
.confirmation-actions { display: flex; justify-content: flex-end; gap: 8px; }
.btn, .icon-button { min-height: 36px; border: 1px solid var(--border-color); border-radius: 6px; background: var(--surface-color); color: var(--text-color); cursor: pointer; }
.btn { padding: 8px 12px; }
.icon-button { display: inline-grid; width: 36px; place-items: center; padding: 0; }
.btn-primary { border-color: var(--primary-color); background: var(--primary-color); color: #fff; }
.btn-danger { border-color: var(--error-color); color: var(--error-color); }
.btn:disabled, .icon-button:disabled, input:disabled, select:disabled, textarea:disabled { cursor: not-allowed; opacity: .55; }
.sr-only { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip: rect(0, 0, 0, 0); white-space: nowrap; border: 0; }
@media (max-width: 720px) {
  .topology-backdrop { padding: 0; }
  .topology-workspace { width: 100vw; max-height: none; min-height: 100dvh; border: 0; border-radius: 0; }
  .editor-toolbar { grid-template-columns: 1fr; }
}
</style>
