<template>
  <UiSheet
    :open="open"
    size="lg"
    data-testid="assignment-drawer"
    :title="editing ? t('control.assignments.editTitle') : t('control.assignments.createTitle')"
    :dismissible="canClose"
    @update:open="value => { if (!value) requestClose() }"
  >
    <form id="assignment-form" class="form-grid" @submit.prevent="save">
      <UiSelect
        id="assignment-node"
        v-model="draft.nodeID"
        size="md"
        :label="t('control.assignments.node')"
        :options="nodeOptions"
        :disabled="editing || saving"
      />
      <UiSelect
        id="assignment-plugin"
        v-model="draft.pluginID"
        size="md"
        :label="t('control.assignments.agentPlugin')"
        :options="pluginOptions"
        :disabled="editing || saving"
      />
      <UiSelect
        id="assignment-scope"
        v-model="draft.serviceScope"
        size="md"
        :label="t('control.table.scope')"
        :options="scopeOptions"
        :disabled="editing || saving"
      />
      <UiTextField
        id="assignment-role"
        v-model.trim="draft.role"
        size="md"
        :label="t('control.table.role')"
        :help="roleSuggestions.length ? t('control.assignments.roleHelp', { roles: roleSuggestions.join(' / ') }) : ''"
        list="assignment-role-options"
        autocomplete="off"
        :disabled="editing || saving"
      />
      <datalist id="assignment-role-options">
        <option v-for="role in roleSuggestions" :key="role" :value="role"></option>
      </datalist>
      <UiSelect
        id="assignment-version"
        v-model="draft.desiredVersion"
        size="md"
        :label="t('control.install.version')"
        :options="versionOptions"
        :disabled="saving"
      />
      <UiTextField
        id="assignment-config-revision"
        :model-value="draft.desiredConfigRevision"
        size="md"
        type="number"
        inputmode="numeric"
        min="0"
        step="1"
        :label="t('control.table.configRevision')"
        :disabled="saving"
        @update:model-value="value => { draft.desiredConfigRevision = value === '' ? '' : Number(value) }"
      />
      <UiTextField
        id="assignment-rollout-group"
        v-model.trim="draft.rolloutGroup"
        size="md"
        :label="t('control.table.rolloutGroup')"
        autocomplete="off"
        :disabled="saving"
      />
      <UiCheckbox
        id="assignment-enabled"
        v-model="draft.enabled"
        class="assignment-enabled-field"
        :label="t('control.assignments.enabled')"
        :disabled="saving"
      />
      <p v-if="error" class="form-error form-grid__full" role="alert">{{ error }}</p>
    </form>
    <template #footer>
      <UiButton :disabled="saving" @click="requestClose">{{ t('common.actions.cancel') }}</UiButton>
      <UiButton variant="primary" type="submit" form="assignment-form" data-testid="save-assignment" :loading="saving" :disabled="!valid" @click.prevent="save">
        {{ t('common.actions.save') }}
      </UiButton>
    </template>
  </UiSheet>
</template>

<script setup>
// Create or edit a node assignment in a side sheet (UiSheet: focus trap,
// Esc, focus return); it cannot be dismissed while saving. A standard sheet
// form (UI U8): top-labelled Ui fields in a .form-grid; the node, plugin,
// scope and role are fixed when editing. Enter in a field saves.
import { computed, reactive, watch } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import UiButton from '@/ui/UiButton.vue'
import UiCheckbox from '@/ui/UiCheckbox.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiSheet from '@/ui/UiSheet.vue'
import UiTextField from '@/ui/UiTextField.vue'
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
// Options of the selects; a value the lists no longer have (an edited
// assignment's old release) stays selectable, as before.
const nodeOptions = computed(() => props.nodes.map(node => ({
  value: Number(node.id),
  label: `${node.name || node.host || `#${node.id}`} (#${node.id})`,
})))
const pluginOptions = computed(() => {
  const options = agentPluginOptions.value.map(option => ({ value: option.pluginID, label: `${option.name} (${option.pluginID})` }))
  if (draft.pluginID && !options.some(option => option.value === draft.pluginID)) {
    options.push({ value: draft.pluginID, label: `${pluginName(draft.pluginID)} (${draft.pluginID})` })
  }
  return options
})
const scopeOptions = computed(() => {
  const options = props.scopes.map(scope => ({ value: scope.id, label: `${scope.name || scope.id} (${scope.id})` }))
  if (draft.serviceScope && !options.some(option => option.value === draft.serviceScope)) {
    options.push({ value: draft.serviceScope, label: draft.serviceScope })
  }
  return options
})
const versionOptions = computed(() => {
  const options = compatibleReleases.value.map(release => ({ value: release.version, label: release.version }))
  if (draft.desiredVersion && !options.some(option => option.value === draft.desiredVersion)) {
    options.push({ value: draft.desiredVersion, label: draft.desiredVersion })
  }
  return options
})
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
.assignment-enabled-field {
  align-self: end;
  min-height: var(--size-control-md);
}
</style>
