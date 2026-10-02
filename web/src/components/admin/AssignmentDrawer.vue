<template>
  <UiSheet
    :open="open"
    size="lg"
    data-testid="assignment-drawer"
    :title="editing ? t('control.assignments.editTitle') : t('control.assignments.createTitle')"
    :dismissible="canClose"
    @update:open="value => { if (!value) requestClose() }"
  >
    <form class="assignment-form-grid" @submit.prevent="save">
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
    <template #footer>
      <UiButton :disabled="saving" @click="requestClose">{{ t('common.actions.cancel') }}</UiButton>
      <UiButton variant="primary" data-testid="save-assignment" :loading="saving" :disabled="!valid" @click="save">
        {{ t('common.actions.save') }}
      </UiButton>
    </template>
  </UiSheet>
</template>

<script setup>
// Create or edit a node assignment in a side sheet (UiSheet: focus trap,
// Esc, focus return); it cannot be dismissed while saving.
import { computed, reactive, watch } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { UiButton, UiSheet } from '@/ui'
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
function requestClose() {
  if (canClose.value) emit('close')
}

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
.assignment-form-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--space-4); align-content: start; }
.form-group { display: grid; min-width: 0; gap: var(--space-2); }
.form-group label, .enabled-field { font-size: var(--type-callout-size); font-weight: var(--weight-semibold); }
.form-group input, .form-group select { box-sizing: border-box; width: 100%; }
.assignment-rollout-field { grid-column: 1 / 2; }
.enabled-field { display: flex; align-items: center; gap: var(--space-2); align-self: end; min-height: var(--size-control-md); font-weight: var(--weight-regular); }
.enabled-field input { width: 18px; height: 18px; margin: 0; }
.drawer-error { grid-column: 1 / -1; margin: 0; color: var(--danger); overflow-wrap: anywhere; }
@media (max-width: 640px) {
  .assignment-form-grid { grid-template-columns: 1fr; }
  .assignment-rollout-field { grid-column: auto; }
}
</style>
