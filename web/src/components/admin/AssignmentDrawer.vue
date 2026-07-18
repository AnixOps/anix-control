<template>
  <div v-if="open" class="assignment-backdrop" @click.self="requestClose">
    <section
      ref="modal"
      class="assignment-drawer"
      data-testid="assignment-drawer"
      role="dialog"
      aria-modal="true"
      aria-labelledby="assignment-editor-title"
      @keydown="handleKeydown"
    >
      <header class="drawer-header">
        <h2 id="assignment-editor-title">
          {{ editing ? t('control.assignments.editTitle') : t('control.assignments.createTitle') }}
        </h2>
        <button
          ref="closeButton"
          class="icon-button"
          data-testid="assignment-close"
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

      <form class="drawer-body assignment-form-grid" @submit.prevent="save">
        <div class="form-group">
          <label for="assignment-node">{{ t('control.assignments.node') }}</label>
          <select id="assignment-node" v-model.number="draft.nodeID" :disabled="editing || saving">
            <option v-for="node in nodes" :key="node.id" :value="Number(node.id)">
              {{ node.name || node.host || `#${node.id}` }} (#{{ node.id }})
            </option>
          </select>
        </div>
        <div class="form-group">
          <label for="assignment-plugin">{{ t('control.assignments.agentPlugin') }}</label>
          <select id="assignment-plugin" v-model="draft.pluginID" :disabled="editing || saving">
            <option v-for="option in agentPluginOptions" :key="option.pluginID" :value="option.pluginID">
              {{ option.name }} ({{ option.pluginID }})
            </option>
            <option v-if="draft.pluginID && !agentPluginOptions.some(option => option.pluginID === draft.pluginID)" :value="draft.pluginID">
              {{ pluginName(draft.pluginID) }} ({{ draft.pluginID }})
            </option>
          </select>
        </div>
        <div class="form-group">
          <label for="assignment-scope">{{ t('control.table.scope') }}</label>
          <select id="assignment-scope" v-model="draft.serviceScope" :disabled="editing || saving">
            <option v-for="scope in scopes" :key="scope.id" :value="scope.id">{{ scope.name || scope.id }} ({{ scope.id }})</option>
            <option v-if="draft.serviceScope && !scopes.some(scope => scope.id === draft.serviceScope)" :value="draft.serviceScope">{{ draft.serviceScope }}</option>
          </select>
        </div>
        <div class="form-group">
          <label for="assignment-role">{{ t('control.table.role') }}</label>
          <input id="assignment-role" v-model.trim="draft.role" type="text" list="assignment-role-options" :disabled="editing || saving" autocomplete="off" />
          <datalist id="assignment-role-options">
            <option v-for="role in roleSuggestions" :key="role" :value="role"></option>
          </datalist>
        </div>
        <div class="form-group">
          <label for="assignment-version">{{ t('control.install.version') }}</label>
          <select id="assignment-version" v-model="draft.desiredVersion" :disabled="saving">
            <option v-for="release in compatibleReleases" :key="release.id" :value="release.version">{{ release.version }}</option>
            <option v-if="draft.desiredVersion && !compatibleReleases.some(release => release.version === draft.desiredVersion)" :value="draft.desiredVersion">
              {{ draft.desiredVersion }}
            </option>
          </select>
        </div>
        <div class="form-group">
          <label for="assignment-config-revision">{{ t('control.table.configRevision') }}</label>
          <input id="assignment-config-revision" v-model.number="draft.desiredConfigRevision" type="number" min="0" step="1" :disabled="saving" />
        </div>
        <div class="form-group assignment-rollout-field">
          <label for="assignment-rollout-group">{{ t('control.table.rolloutGroup') }}</label>
          <input id="assignment-rollout-group" v-model.trim="draft.rolloutGroup" type="text" autocomplete="off" :disabled="saving" />
        </div>
        <label class="enabled-field assignment-enabled-field" for="assignment-enabled">
          <input id="assignment-enabled" v-model="draft.enabled" type="checkbox" :disabled="saving" />
          <span>{{ t('control.assignments.enabled') }}</span>
        </label>
        <p v-if="error" class="drawer-error" role="alert">{{ error }}</p>
      </form>

      <footer class="drawer-footer">
        <button class="btn" type="button" :disabled="saving" @click="requestClose">{{ t('common.actions.cancel') }}</button>
        <button class="btn btn-primary" data-testid="save-assignment" type="button" :disabled="saving || !valid" @click="save">
          {{ saving ? t('control.actions.saving') : t('common.actions.save') }}
        </button>
      </footer>
    </section>
  </div>
</template>

<script setup>
import { computed, reactive, ref, watch } from 'vue'
import { X } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { useModalFocus } from '@/composables/useModalFocus'
import { assignmentPayload } from '@/composables/useKernelDeployments'
import { releaseTargets } from '@/utils/kernelPluginRelease'

const OFFICIAL_PLUGIN_ROLES = Object.freeze({
  'machine-telemetry': ['telemetry'],
  'nftables-forward': ['cn_dedicated_nftables', 'entry'],
  'nat-egress': ['nat_egress', 'overseas_exit'],
  'gost-mesh': ['relay', 'tunnel_entry', 'tunnel_exit', 'cn_standard_tunnel_entry'],
})

const props = defineProps({
  open: { type: Boolean, default: false },
  assignment: { type: Object, default: null },
  selectedNodeID: { type: Number, default: 0 },
  nodes: { type: Array, default: () => [] },
  plugins: { type: Array, default: () => [] },
  releases: { type: Array, default: () => [] },
  installations: { type: Array, default: () => [] },
  scopes: { type: Array, default: () => [] },
  saving: { type: Boolean, default: false },
  error: { type: String, default: '' },
})
const emit = defineEmits(['close', 'save'])
const { t } = useAppI18n()
const closeButton = ref(null)
const modal = ref(null)
const draft = reactive({
  nodeID: 0,
  pluginID: '',
  serviceScope: '',
  role: '',
  desiredVersion: '',
  desiredConfigRevision: 0,
  rolloutGroup: '',
  enabled: true,
})

const editing = computed(() => Boolean(props.assignment))
const openState = computed(() => props.open)
const canClose = computed(() => !props.saving)
const agentPluginOptions = computed(() => props.installations
  .filter(installation => installation?.target === 'agent')
  .map(installation => ({
    pluginID: installation.plugin_id,
    name: pluginName(installation.plugin_id),
    installation,
  }))
  .sort((left, right) => left.name.localeCompare(right.name)))
const compatibleReleases = computed(() => props.releases
  .filter(release => release?.plugin_id === draft.pluginID && releaseTargets(release).includes('agent')))
const roleSuggestions = computed(() => OFFICIAL_PLUGIN_ROLES[draft.pluginID] || [])
const valid = computed(() => Boolean(
  draft.nodeID &&
  draft.pluginID &&
  draft.serviceScope &&
  draft.role &&
  draft.desiredVersion &&
  Number.isInteger(Number(draft.desiredConfigRevision)) &&
  Number(draft.desiredConfigRevision) >= 0,
))
const { handleKeydown, requestClose } = useModalFocus({
  open: openState,
  canClose,
  container: modal,
  initialFocus: closeButton,
  close: () => emit('close'),
})

watch(() => props.open, isOpen => {
  if (isOpen) resetDraft()
}, { immediate: true })
watch(() => props.assignment, () => {
  if (props.open) resetDraft()
})
watch(() => props.selectedNodeID, () => {
  if (props.open && !editing.value) resetDraft()
})
watch(agentPluginOptions, options => {
  if (props.open && !editing.value && !draft.pluginID && options.length) selectPluginDefaults(options[0].pluginID)
})
watch(() => draft.pluginID, (pluginID, previousPluginID) => {
  if (props.open && !editing.value && pluginID && pluginID !== previousPluginID) selectPluginDefaults(pluginID)
})

function pluginName(pluginID) {
  return props.plugins.find(plugin => plugin?.id === pluginID)?.name || pluginID
}

function agentInstallationFor(pluginID) {
  return props.installations.find(installation => installation?.plugin_id === pluginID && installation.target === 'agent')
}

function defaultScopeFor(pluginID) {
  const preferredScope = pluginID === 'machine-telemetry' ? ['monitoring', 'system'] : ['forward']
  return preferredScope.find(scopeID => props.scopes.some(scope => scope?.id === scopeID)) || props.scopes[0]?.id || ''
}

function selectPluginDefaults(pluginID) {
  const installation = agentInstallationFor(pluginID)
  const releases = props.releases.filter(release => release?.plugin_id === pluginID && releaseTargets(release).includes('agent'))
  draft.pluginID = pluginID
  draft.desiredVersion = installation?.desired_version || releases[0]?.version || ''
  draft.desiredConfigRevision = Number(installation?.config_revision ?? 0)
  draft.serviceScope = defaultScopeFor(pluginID)
  draft.role = (OFFICIAL_PLUGIN_ROLES[pluginID] || [])[0] || ''
}

function resetDraft() {
  if (props.assignment) {
    Object.assign(draft, {
      nodeID: Number(props.assignment.node_id || props.selectedNodeID),
      pluginID: props.assignment.plugin_id || '',
      serviceScope: props.assignment.service_scope || '',
      role: props.assignment.role || '',
      desiredVersion: props.assignment.desired_version || '',
      desiredConfigRevision: Number(props.assignment.desired_config_revision ?? 0),
      rolloutGroup: props.assignment.rollout_group || '',
      enabled: Boolean(props.assignment.enabled),
    })
    return
  }

  Object.assign(draft, {
    nodeID: Number(props.selectedNodeID),
    pluginID: '',
    serviceScope: '',
    role: '',
    desiredVersion: '',
    desiredConfigRevision: 0,
    rolloutGroup: '',
    enabled: true,
  })
  const firstPlugin = agentPluginOptions.value[0]?.pluginID || ''
  if (firstPlugin) selectPluginDefaults(firstPlugin)
}

function save() {
  if (props.saving || !valid.value) return
  emit('save', {
    nodeID: Number(draft.nodeID),
    payload: assignmentPayload({
      service_scope: draft.serviceScope,
      plugin_id: draft.pluginID,
      role: draft.role,
      desired_version: draft.desiredVersion,
      desired_config_revision: draft.desiredConfigRevision,
      enabled: draft.enabled,
      rollout_group: draft.rolloutGroup,
    }),
  })
}
</script>

<style scoped>
.assignment-backdrop { position: fixed; inset: 0; z-index: 1200; display: flex; justify-content: flex-end; background: rgba(15, 23, 42, .62); }
.assignment-drawer { display: flex; width: min(100%, 620px); min-height: 100dvh; max-height: 100dvh; flex-direction: column; overflow-y: auto; border-left: 1px solid var(--border-color); background: var(--surface-color); color: var(--text-color); box-shadow: -18px 0 32px rgba(15, 23, 42, .2); }
.drawer-header, .drawer-footer { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 18px 20px; border-bottom: 1px solid var(--border-color); }
.drawer-header h2 { margin: 0; font-size: 18px; line-height: 1.35; }
.drawer-body { flex: 1; align-content: start; }
.assignment-form-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; padding: 20px; }
.form-group { display: grid; min-width: 0; gap: 8px; }
.form-group label, .enabled-field { font-size: 14px; font-weight: 600; }
.form-group input, .form-group select { box-sizing: border-box; min-height: 38px; width: 100%; border: 1px solid var(--border-color); border-radius: 6px; background: var(--bg-color); color: var(--text-color); padding: 8px 10px; }
.assignment-rollout-field { grid-column: 1 / 2; }
.enabled-field { display: flex; align-items: center; gap: 9px; align-self: end; min-height: 38px; font-weight: 400; }
.enabled-field input { width: 18px; height: 18px; margin: 0; }
.drawer-error { grid-column: 1 / -1; margin: 0; color: var(--error-color); overflow-wrap: anywhere; }
.drawer-footer { justify-content: flex-end; border-top: 1px solid var(--border-color); border-bottom: 0; }
.btn, .icon-button { min-height: 36px; border: 1px solid var(--border-color); border-radius: 6px; background: var(--surface-color); color: var(--text-color); cursor: pointer; }
.btn { padding: 8px 12px; }
.icon-button { display: inline-grid; width: 36px; place-items: center; padding: 0; }
.btn-primary { border-color: var(--primary-color); background: var(--primary-color); color: #fff; }
.btn:disabled, .icon-button:disabled, input:disabled, select:disabled { cursor: not-allowed; opacity: .55; }
.sr-only { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip: rect(0, 0, 0, 0); white-space: nowrap; border: 0; }
@media (max-width: 640px) {
  .assignment-drawer { width: 100vw; }
  .assignment-form-grid { grid-template-columns: 1fr; }
  .assignment-rollout-field { grid-column: auto; }
}
</style>
