<template>
  <div class="control-page" :aria-busy="loading ? 'true' : 'false'">
    <div class="page-header">
      <div>
        <h1>{{ t('pageTitles.admin.control') }}</h1>
        <p class="page-subtitle">{{ t('control.subtitle') }}</p>
      </div>
      <div class="header-actions">
        <button class="btn btn-primary btn-sm" type="button" @click="openReleaseImport">{{ t('control.actions.importRelease') }}</button>
        <button class="btn btn-sm" type="button" :disabled="loading" @click="load()">
          {{ loading ? t('control.actions.refreshing') : t('control.actions.refresh') }}
        </button>
      </div>
    </div>

    <div class="tabs" role="tablist" :aria-label="t('pageTitles.admin.control')">
      <button
        v-for="(item, index) in tabs"
        :id="`control-tab-${item.key}`"
        :key="item.key"
        class="tab"
        :class="{ active: tab === item.key }"
        type="button"
        role="tab"
        :aria-controls="`control-panel-${item.key}`"
        :aria-selected="tab === item.key"
        :tabindex="tab === item.key ? 0 : -1"
        @click="tab = item.key"
        @keydown="moveTab($event, index)"
      >
        {{ item.label }}
        <span v-if="item.key === 'operations' && polling" class="polling-dot" :title="t('control.states.polling')"></span>
      </button>
    </div>

    <p v-if="error" class="error-message" role="alert">{{ error }}</p>
    <p v-if="notice" class="notice-message" role="status">{{ notice }}</p>

    <section v-if="adminExtensionErrors.length" class="extension-error-band" role="alert">
      <strong>{{ t('control.extensions.errorsTitle') }}</strong>
      <ul>
        <li v-for="(extensionError, index) in adminExtensionErrors" :key="`${extensionError.plugin_id || 'catalog'}-${index}`">
          <code v-if="extensionError.plugin_id">{{ extensionError.plugin_id }}</code>
          {{ extensionError.message }}
        </li>
      </ul>
    </section>

    <div
      v-show="tab === 'plugins'"
      id="control-panel-plugins"
      class="table-container"
      role="tabpanel"
      aria-labelledby="control-tab-plugins"
      tabindex="0"
    >
      <table class="data-table plugins-table">
        <thead>
          <tr>
            <th>{{ t('control.table.plugin') }}</th>
            <th>{{ t('control.table.release') }}</th>
            <th>{{ t('control.table.installation') }}</th>
            <th>{{ t('control.table.version') }}</th>
            <th>{{ t('control.table.state') }}</th>
            <th>{{ t('control.table.actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="loading && !loaded" class="state-row"><td colspan="6">{{ t('control.states.loading') }}</td></tr>
          <tr v-for="row in pluginRows" v-else :key="row.key">
            <td>
              <span class="primary-cell">{{ row.plugin.name || row.plugin.id }}</span>
              <code class="secondary-cell">{{ row.plugin.id }}</code>
              <span v-if="row.plugin.description" class="plugin-description">{{ row.plugin.description }}</span>
            </td>
            <td>
              <span>{{ row.latestRelease?.version || '-' }}</span>
              <span class="secondary-cell">{{ t('control.labels.releases', { count: row.releases.length }) }}</span>
            </td>
            <td><code>{{ row.target || '-' }}</code></td>
            <td>
              <span class="version-line"><span>{{ t('control.labels.desired') }}</span><code>{{ row.installation?.desired_version || '-' }}</code></span>
              <span class="version-line"><span>{{ t('control.labels.observed') }}</span><code>{{ row.installation?.observed_version || '-' }}</code></span>
            </td>
            <td>
              <span :class="['status-badge', stateClass(row.installation?.state)]">{{ row.installation?.state || t('control.states.catalogued') }}</span>
              <span v-if="row.installation?.last_error" class="row-error">{{ row.installation.last_error }}</span>
            </td>
            <td>
              <div class="row-actions">
                <button
                  v-if="!row.installation"
                  class="btn btn-primary btn-sm"
                  type="button"
                  :disabled="row.releases.length === 0 || isBusy(row)"
                  @click="openInstall(row)"
                >{{ t('control.actions.install') }}</button>
                <template v-else>
                  <button class="btn btn-sm" type="button" :disabled="isBusy(row)" @click="openConfig(row)">{{ t('control.actions.configure') }}</button>
                  <button v-if="!row.installation.enabled" class="btn btn-primary btn-sm" type="button" :disabled="isBusy(row)" @click="runLifecycle(row, 'enable')">{{ t('control.actions.enable') }}</button>
                  <button v-else class="btn btn-sm btn-danger" type="button" :disabled="isBusy(row)" @click="runLifecycle(row, 'disable')">{{ t('control.actions.disable') }}</button>
                  <button v-if="row.upgradeRelease" class="btn btn-sm" type="button" :disabled="isBusy(row)" @click="openUpgrade(row)">{{ t('control.actions.upgrade') }}</button>
                  <button v-if="row.installation.previous_version" class="btn btn-sm" type="button" :disabled="isBusy(row)" @click="runLifecycle(row, 'rollback')">{{ t('control.actions.rollback') }}</button>
                </template>
              </div>
            </td>
          </tr>
          <tr v-if="loaded && pluginRows.length === 0"><td colspan="6" class="empty-row">{{ t('control.empty.plugins') }}</td></tr>
        </tbody>
      </table>
    </div>

    <section
      v-show="tab === 'assignments'"
      id="control-panel-assignments"
      class="assignments-panel"
      role="tabpanel"
      aria-labelledby="control-tab-assignments"
      tabindex="0"
    >
      <div class="assignments-toolbar">
        <div class="assignment-node-picker">
          <label for="assignment-node-filter">{{ t('control.assignments.node') }}</label>
          <select id="assignment-node-filter" v-model.number="selectedNodeID" :disabled="nodes.length === 0 || assignmentsLoading" @change="loadAssignments()">
            <option v-if="nodes.length === 0" :value="0">{{ t('control.assignments.noNodes') }}</option>
            <option v-for="node in nodes" :key="node.id" :value="node.id">{{ node.name || node.host || `#${node.id}` }} (#{{ node.id }})</option>
          </select>
        </div>
        <div class="assignments-toolbar-actions">
          <button class="btn btn-sm" type="button" :disabled="!selectedNodeID || assignmentsLoading" @click="loadAssignments()">
            {{ assignmentsLoading ? t('control.actions.refreshing') : t('control.actions.refresh') }}
          </button>
          <button id="new-assignment" class="btn btn-primary btn-sm" type="button" :disabled="!selectedNodeID || agentPluginOptions.length === 0 || scopes.length === 0" @click="openAssignmentEditor()">
            {{ t('control.actions.newAssignment') }}
          </button>
        </div>
      </div>

      <div class="table-container">
        <table class="data-table assignments-table">
          <thead>
            <tr>
              <th>{{ t('control.table.plugin') }}</th>
              <th>{{ t('control.table.scope') }}</th>
              <th>{{ t('control.table.role') }}</th>
              <th>{{ t('control.table.version') }}</th>
              <th>{{ t('control.table.configRevision') }}</th>
              <th>{{ t('control.table.rolloutGroup') }}</th>
              <th>{{ t('control.table.state') }}</th>
              <th>{{ t('control.table.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="assignmentsLoading" class="state-row"><td colspan="8">{{ t('control.assignments.loading') }}</td></tr>
            <tr v-for="assignment in assignments" v-else :key="assignment.id">
              <td>
                <span class="primary-cell">{{ pluginName(assignment.plugin_id) }}</span>
                <code class="secondary-cell">{{ assignment.plugin_id }}</code>
              </td>
              <td><code>{{ assignment.service_scope }}</code></td>
              <td><code>{{ assignment.role }}</code></td>
              <td><code>{{ assignment.desired_version || '-' }}</code></td>
              <td>{{ assignment.desired_config_revision ?? 0 }}</td>
              <td>{{ assignment.rollout_group || '-' }}</td>
              <td><span :class="['status-badge', assignment.enabled ? 'status-active' : 'status-error']">{{ assignment.enabled ? t('control.states.enabled') : t('control.states.disabled') }}</span></td>
              <td>
                <div class="row-actions">
                  <button class="btn btn-sm" type="button" :disabled="isAssignmentBusy(assignment)" @click="openAssignmentEditor(assignment)">{{ t('common.actions.edit') }}</button>
                  <button
                    :class="['btn', 'btn-sm', assignment.enabled ? 'btn-danger' : 'btn-primary']"
                    type="button"
                    :disabled="isAssignmentBusy(assignment)"
                    @click="toggleAssignment(assignment)"
                  >{{ assignment.enabled ? t('control.actions.disable') : t('control.actions.enable') }}</button>
                  <button class="btn btn-sm btn-danger" type="button" :disabled="isAssignmentBusy(assignment)" @click="removeAssignment(assignment)">{{ t('common.actions.delete') }}</button>
                </div>
              </td>
            </tr>
            <tr v-if="!assignmentsLoading && selectedNodeID && assignments.length === 0"><td colspan="8" class="empty-row">{{ t('control.empty.assignments') }}</td></tr>
            <tr v-if="!assignmentsLoading && !selectedNodeID"><td colspan="8" class="empty-row">{{ t('control.assignments.noNodes') }}</td></tr>
          </tbody>
        </table>
      </div>
    </section>

    <div
      v-show="tab === 'scopes'"
      id="control-panel-scopes"
      class="table-container"
      role="tabpanel"
      aria-labelledby="control-tab-scopes"
      tabindex="0"
    >
      <table class="data-table scopes-table">
        <thead><tr><th>{{ t('control.table.scope') }}</th><th>{{ t('control.table.owner') }}</th><th>{{ t('control.table.description') }}</th></tr></thead>
        <tbody>
          <tr v-if="loading && !loaded" class="state-row"><td colspan="3">{{ t('control.states.loading') }}</td></tr>
          <tr v-for="scope in scopes" v-else :key="scope.id">
            <td><span class="primary-cell">{{ scope.name || scope.id }}</span><code class="secondary-cell">{{ scope.id }}</code></td>
            <td>{{ scope.plugin_id || '-' }}</td>
            <td class="description-cell">{{ scope.description || '-' }}</td>
          </tr>
          <tr v-if="loaded && scopes.length === 0"><td colspan="3" class="empty-row">{{ t('control.empty.scopes') }}</td></tr>
        </tbody>
      </table>
    </div>

    <section
      v-show="tab === 'topologies'"
      id="control-panel-topologies"
      class="topology-panel"
      role="tabpanel"
      aria-labelledby="control-tab-topologies"
      tabindex="0"
    >
      <div class="topology-toolbar">
        <div class="topology-selection">
          <label for="topology-selector">{{ t('control.topology.select') }}</label>
          <select id="topology-selector" v-model.number="selectedTopologyID">
            <option v-if="topologies.length === 0" :value="0">{{ t('control.empty.topologies') }}</option>
            <option v-for="topology in topologies" :key="topology.id" :value="topology.id">{{ topology.name || `#${topology.id}` }} (#{{ topology.id }})</option>
          </select>
        </div>
        <div class="row-actions">
          <button class="btn btn-sm" type="button" :disabled="topologyLoading" @click="loadTopologies">
            {{ topologyLoading ? t('control.actions.refreshing') : t('control.actions.refresh') }}
          </button>
          <button id="edit-topology" class="btn btn-primary btn-sm" type="button" :disabled="!selectedTopologyID || topologyLoading" @click="openTopologyEditor()">
            {{ t('control.topology.edit') }}
          </button>
          <button id="new-topology" class="btn btn-sm" type="button" :disabled="scopes.length === 0" @click="openNewTopology">{{ t('control.topology.new') }}</button>
        </div>
      </div>
      <div class="table-container">
        <table class="data-table topologies-table">
          <thead><tr><th>{{ t('control.table.topology') }}</th><th>{{ t('control.table.scope') }}</th><th>{{ t('control.table.activeRevision') }}</th><th>{{ t('control.table.deployment') }}</th><th>{{ t('control.table.description') }}</th><th>{{ t('control.table.actions') }}</th></tr></thead>
          <tbody>
            <tr v-if="loading && !loaded" class="state-row"><td colspan="6">{{ t('control.states.loading') }}</td></tr>
            <tr v-for="topology in topologies" v-else :key="topology.id">
              <td><span class="primary-cell">{{ topology.name || '-' }}</span><code class="secondary-cell">#{{ topology.id }}</code></td>
              <td><code>{{ topology.service_scope || '-' }}</code></td>
              <td><code>{{ topology.active_revision_id || '-' }}</code></td>
              <td>
                <span v-if="latestDeploymentFor(topology)" :class="['status-badge', stateClass(latestDeploymentFor(topology).state)]">{{ latestDeploymentFor(topology).state }}</span>
                <span v-else class="secondary-cell">{{ t('control.topology.noDeployment') }}</span>
              </td>
              <td class="description-cell">{{ topology.description || '-' }}</td>
              <td>
                <div class="row-actions">
                  <button class="btn btn-sm" type="button" @click="openTopologyEditor(topology)">{{ t('control.topology.edit') }}</button>
                  <button v-if="latestDeploymentFor(topology)" class="btn btn-sm" type="button" @click="openDeploymentStatus(latestDeploymentFor(topology))">{{ t('control.topology.status') }}</button>
                </div>
              </td>
            </tr>
            <tr v-if="loaded && topologies.length === 0"><td colspan="6" class="empty-row">{{ t('control.empty.topologies') }}</td></tr>
          </tbody>
        </table>
      </div>
    </section>

    <div
      v-show="tab === 'operations'"
      id="control-panel-operations"
      class="table-container"
      role="tabpanel"
      aria-labelledby="control-tab-operations"
      tabindex="0"
    >
      <table class="data-table operations-table">
        <thead><tr><th>{{ t('control.table.operation') }}</th><th>{{ t('control.table.plugin') }}</th><th>{{ t('control.table.revision') }}</th><th>{{ t('control.table.deadline') }}</th><th>{{ t('control.table.state') }}</th><th>{{ t('control.table.actions') }}</th></tr></thead>
        <tbody>
          <tr v-if="loading && !loaded" class="state-row"><td colspan="6">{{ t('control.states.loading') }}</td></tr>
          <tr v-for="operation in operations" v-else :key="operation.id">
            <td><code class="identifier">{{ operation.kind || '-' }}</code><code class="secondary-cell operation-id">{{ operation.id }}</code></td>
            <td class="identifier">{{ operation.plugin_id || '-' }}</td><td>{{ operation.revision ?? '-' }}</td><td>{{ formatDate(operation.deadline_at) }}</td>
            <td>
              <span :class="['status-badge', stateClass(operation.state)]">{{ operation.state || '-' }}</span>
              <span v-if="operation.last_error" class="row-error">{{ operation.last_error }}</span>
            </td>
            <td><button v-if="isCancellable(operation)" class="btn btn-sm btn-danger" type="button" :disabled="operationBusy === operation.id" @click="cancelOperation(operation)">{{ t('control.actions.cancel') }}</button></td>
          </tr>
          <tr v-if="loaded && operations.length === 0"><td colspan="6" class="empty-row">{{ t('control.empty.operations') }}</td></tr>
        </tbody>
      </table>
    </div>

    <div v-if="assignmentEditor.open" class="modal-overlay" @click.self="closeAssignmentEditor">
      <section class="modal modal-lg" role="dialog" aria-modal="true" aria-labelledby="assignment-editor-title">
        <div class="modal-header">
          <h3 id="assignment-editor-title">{{ assignmentEditor.mode === 'edit' ? t('control.assignments.editTitle') : t('control.assignments.createTitle') }}</h3>
          <button class="btn btn-ghost close-btn" type="button" :aria-label="t('common.actions.close')" @click="closeAssignmentEditor">x</button>
        </div>
        <div class="modal-body assignment-form-grid">
          <div class="form-group">
            <label for="assignment-node">{{ t('control.assignments.node') }}</label>
            <select id="assignment-node" v-model.number="assignmentEditor.nodeID" :disabled="assignmentEditor.mode === 'edit'">
              <option v-for="node in nodes" :key="node.id" :value="node.id">{{ node.name || node.host || `#${node.id}` }} (#{{ node.id }})</option>
            </select>
          </div>
          <div class="form-group">
            <label for="assignment-plugin">{{ t('control.assignments.agentPlugin') }}</label>
            <select id="assignment-plugin" v-model="assignmentEditor.pluginID" :disabled="assignmentEditor.mode === 'edit'" @change="selectAssignmentPluginDefaults">
              <option v-for="option in agentPluginOptions" :key="option.pluginID" :value="option.pluginID">{{ option.name }} ({{ option.pluginID }})</option>
              <option v-if="assignmentEditor.pluginID && !agentPluginOptions.some(option => option.pluginID === assignmentEditor.pluginID)" :value="assignmentEditor.pluginID">{{ pluginName(assignmentEditor.pluginID) }} ({{ assignmentEditor.pluginID }})</option>
            </select>
          </div>
          <div class="form-group">
            <label for="assignment-scope">{{ t('control.table.scope') }}</label>
            <select id="assignment-scope" v-model="assignmentEditor.serviceScope" :disabled="assignmentEditor.mode === 'edit'">
              <option v-for="scope in scopes" :key="scope.id" :value="scope.id">{{ scope.name || scope.id }} ({{ scope.id }})</option>
              <option v-if="assignmentEditor.serviceScope && !scopes.some(scope => scope.id === assignmentEditor.serviceScope)" :value="assignmentEditor.serviceScope">{{ assignmentEditor.serviceScope }}</option>
            </select>
          </div>
          <div class="form-group">
            <label for="assignment-role">{{ t('control.table.role') }}</label>
            <input id="assignment-role" v-model.trim="assignmentEditor.role" type="text" list="assignment-role-options" :disabled="assignmentEditor.mode === 'edit'" autocomplete="off" />
            <datalist id="assignment-role-options">
              <option v-for="role in assignmentRoleSuggestions" :key="role" :value="role"></option>
            </datalist>
          </div>
          <div class="form-group">
            <label for="assignment-version">{{ t('control.install.version') }}</label>
            <select id="assignment-version" v-model="assignmentEditor.desiredVersion">
              <option v-for="release in assignmentReleaseOptions" :key="release.id" :value="release.version">{{ release.version }}</option>
              <option v-if="assignmentEditor.desiredVersion && !assignmentReleaseOptions.some(release => release.version === assignmentEditor.desiredVersion)" :value="assignmentEditor.desiredVersion">{{ assignmentEditor.desiredVersion }}</option>
            </select>
          </div>
          <div class="form-group">
            <label for="assignment-config-revision">{{ t('control.table.configRevision') }}</label>
            <input id="assignment-config-revision" v-model.number="assignmentEditor.desiredConfigRevision" type="number" min="0" step="1" />
          </div>
          <div class="form-group assignment-rollout-field">
            <label for="assignment-rollout-group">{{ t('control.table.rolloutGroup') }}</label>
            <input id="assignment-rollout-group" v-model.trim="assignmentEditor.rolloutGroup" type="text" autocomplete="off" />
          </div>
          <label class="enable-after-install assignment-enabled-field">
            <input v-model="assignmentEditor.enabled" type="checkbox" />
            <span>{{ t('control.assignments.enabled') }}</span>
          </label>
        </div>
        <div class="modal-footer">
          <button class="btn" type="button" :disabled="assignmentEditor.saving" @click="closeAssignmentEditor">{{ t('common.actions.cancel') }}</button>
          <button class="btn btn-primary" type="button" :disabled="assignmentEditor.saving || !assignmentEditorValid" @click="saveAssignment">
            {{ assignmentEditor.saving ? t('control.actions.saving') : t('common.actions.save') }}
          </button>
        </div>
      </section>
    </div>

    <div v-if="installationEditor.open" class="modal-overlay" @click.self="closeInstallationEditor">
      <section class="modal" role="dialog" aria-modal="true" :aria-labelledby="'plugin-installation-title'">
        <div class="modal-header">
          <h3 id="plugin-installation-title">{{ installationEditor.mode === 'install' ? t('control.install.title') : t('control.update.title') }}</h3>
          <button class="btn btn-ghost close-btn" type="button" :aria-label="t('common.actions.close')" @click="closeInstallationEditor">x</button>
        </div>
        <div class="modal-body">
          <p class="modal-plugin-name">{{ installationEditor.plugin?.name || installationEditor.plugin?.id }}</p>
          <div class="form-group">
            <label for="plugin-install-target">{{ t('control.install.target') }}</label>
            <select id="plugin-install-target" v-model="installationEditor.target" :disabled="installationEditor.mode === 'update'" @change="selectDefaultEditorVersion">
              <option v-for="target in installationEditor.targets" :key="target" :value="target">{{ target }}</option>
            </select>
          </div>
          <div class="form-group">
            <label for="plugin-install-version">{{ t('control.install.version') }}</label>
            <select id="plugin-install-version" v-model="installationEditor.version">
              <option v-for="release in editorReleases" :key="release.id" :value="release.version">{{ release.version }}</option>
            </select>
          </div>
          <label v-if="installationEditor.mode === 'install'" class="enable-after-install">
            <input v-model="installationEditor.enabled" type="checkbox" />
            <span>{{ t('control.install.enableAfterInstall') }}</span>
          </label>
        </div>
        <div class="modal-footer">
          <button class="btn" type="button" :disabled="installationEditor.saving" @click="closeInstallationEditor">{{ t('common.actions.cancel') }}</button>
          <button class="btn btn-primary" type="button" :disabled="installationEditor.saving || !installationEditor.version || !installationEditor.target" @click="saveInstallation">
            {{ installationEditor.saving ? t('control.actions.saving') : installationEditor.mode === 'install' ? t('control.actions.install') : t('control.actions.upgrade') }}
          </button>
        </div>
      </section>
    </div>

    <div v-if="configEditor.open" class="modal-overlay" @click.self="closeConfigEditor">
      <section class="modal modal-lg" role="dialog" aria-modal="true" aria-labelledby="plugin-config-title">
        <div class="modal-header">
          <div>
            <h3 id="plugin-config-title">{{ t('control.config.title') }}</h3>
            <p class="modal-meta">{{ configEditor.row?.plugin.name || configEditor.row?.plugin.id }} / {{ configEditor.row?.target }}</p>
          </div>
          <button class="btn btn-ghost close-btn" type="button" :aria-label="t('common.actions.close')" @click="closeConfigEditor">x</button>
        </div>
        <div class="modal-body">
          <p v-if="configEditor.loading" class="state-message">{{ t('control.config.loading') }}</p>
          <PluginConfigForm
            v-else
            v-model="configEditor.value"
            :schema="configEditor.schema"
            @validity="configEditor.valid = $event"
          />
        </div>
        <div class="modal-footer">
          <span class="revision-label">{{ t('control.config.revision', { revision: configEditor.revision }) }}</span>
          <button class="btn" type="button" :disabled="configEditor.saving" @click="closeConfigEditor">{{ t('common.actions.cancel') }}</button>
          <button class="btn btn-primary" type="button" :disabled="configEditor.loading || configEditor.saving || !configEditor.valid" @click="saveConfig">
            {{ configEditor.saving ? t('control.actions.saving') : t('common.actions.save') }}
          </button>
        </div>
      </section>
    </div>

    <div v-if="releaseImport.open" class="modal-overlay" @click.self="closeReleaseImport">
      <section class="modal modal-lg" role="dialog" aria-modal="true" aria-labelledby="plugin-release-title">
        <div class="modal-header">
          <h3 id="plugin-release-title">{{ t('control.releaseImport.title') }}</h3>
          <button class="btn btn-ghost close-btn" type="button" :aria-label="t('common.actions.close')" @click="closeReleaseImport">x</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label for="plugin-release-manifest">{{ t('control.releaseImport.manifest') }}</label>
            <input type="file" accept="application/json,.json" @change="readManifestFile" />
            <textarea id="plugin-release-manifest" v-model="releaseImport.manifest" rows="10" spellcheck="false" class="json-textarea"></textarea>
          </div>
          <div class="form-group">
            <label for="plugin-release-signature">{{ t('control.releaseImport.signature') }}</label>
            <input type="file" accept="text/plain,.sig" @change="readSignatureFile" />
            <textarea id="plugin-release-signature" v-model="releaseImport.signature" rows="3" spellcheck="false"></textarea>
          </div>
          <div class="form-group">
            <label for="plugin-release-artifact">{{ t('control.releaseImport.artifact') }}</label>
            <input id="plugin-release-artifact" type="file" @change="readArtifactFile" />
            <p class="field-help">{{ releaseImport.artifactName || t('control.releaseImport.artifactOptional') }}</p>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn" type="button" :disabled="releaseImport.saving" @click="closeReleaseImport">{{ t('common.actions.cancel') }}</button>
          <button class="btn btn-primary" type="button" :disabled="releaseImport.saving || !releaseImport.manifest.trim() || !releaseImport.signature.trim()" @click="importRelease">
            {{ releaseImport.saving ? t('control.actions.importing') : t('control.actions.importRelease') }}
          </button>
        </div>
      </section>
    </div>

    <div v-if="topologyEditor.open" class="modal-overlay" @click.self="closeTopologyEditor">
      <section class="modal modal-xl" role="dialog" aria-modal="true" aria-labelledby="topology-editor-title">
        <div class="modal-header">
          <div>
            <h3 id="topology-editor-title">{{ t('control.topology.editorTitle') }}</h3>
            <p class="modal-meta">{{ topologyEditor.topology?.name || topologyEditor.topology?.id }} / {{ t('control.topology.revision') }} {{ topologyEditor.revisionID || '-' }}</p>
          </div>
          <button class="btn btn-ghost close-btn" type="button" :aria-label="t('common.actions.close')" @click="closeTopologyEditor">x</button>
        </div>
        <div class="modal-body topology-editor-body">
          <div class="topology-editor-toolbar">
            <div class="form-group">
              <label for="topology-revision-selector">{{ t('control.topology.revision') }}</label>
              <select id="topology-revision-selector" v-model.number="topologyEditor.revisionID" :disabled="topologyEditor.loading || topologyEditor.saving" @change="loadTopologyRevisionDetail">
                <option v-if="topologyRevisions.length === 0" :value="0">{{ t('control.topology.noRevisions') }}</option>
                <option v-for="revision in topologyRevisions" :key="revision.id" :value="revision.id">r{{ revision.revision }} (#{{ revision.id }})</option>
              </select>
            </div>
            <div class="form-group">
              <label for="topology-rollout-group">{{ t('control.table.rolloutGroup') }}</label>
              <input id="topology-rollout-group" v-model.trim="topologyEditor.rolloutGroup" type="text" autocomplete="off" />
            </div>
            <div class="form-group">
              <label for="topology-failure-policy">{{ t('control.topology.failurePolicy') }}</label>
              <select id="topology-failure-policy" v-model="topologyEditor.failurePolicy">
                <option value="stop_and_rollback">{{ t('control.topology.stopAndRollback') }}</option>
              </select>
            </div>
          </div>
          <div class="form-group">
            <label for="topology-revision-message">{{ t('control.topology.message') }}</label>
            <input id="topology-revision-message" v-model.trim="topologyEditor.message" type="text" maxlength="500" />
          </div>
          <div class="form-group">
            <label for="topology-editor-json">{{ t('control.topology.graphJSON') }}</label>
            <textarea id="topology-editor-json" v-model="topologyEditor.json" rows="16" spellcheck="false" class="json-textarea"></textarea>
            <p class="field-help">{{ t('control.topology.graphHelp') }}</p>
          </div>
          <p v-if="topologyEditorDirty" class="topology-dirty" role="status">{{ t('control.topology.unsavedChanges') }}</p>
          <div v-if="topologyEditor.validation" class="topology-validation" :class="topologyEditor.validation.valid ? 'is-valid' : 'is-invalid'" role="status">
            <strong>{{ topologyEditor.validation.valid ? t('control.topology.valid') : t('control.topology.invalid') }}</strong>
            <ul v-if="topologyEditor.validation.issues?.length">
              <li v-for="(issue, index) in topologyEditor.validation.issues" :key="`${issue.code || 'issue'}-${index}`">{{ issue.message || issue.code }}</li>
            </ul>
            <div v-if="topologyEditor.validation.checks?.length" class="topology-checks">
              <span v-for="check in topologyEditor.validation.checks" :key="check.name" :class="['check-pill', check.status === 'passed' ? 'check-passed' : 'check-failed']">{{ check.name }}: {{ check.status }}</span>
            </div>
          </div>
          <div v-if="topologyEditor.preview" class="topology-preview" role="status">
            <strong>{{ t('control.topology.preview') }}</strong>
            <span class="secondary-cell">{{ t('control.topology.previewSteps', { count: topologyEditor.preview.steps?.length || 0 }) }}</span>
            <ol v-if="topologyEditor.preview.steps?.length" class="preview-steps">
              <li v-for="step in topologyEditor.preview.steps" :key="`${step.order}-${step.vertex_key}`"><code>{{ step.vertex_key }}</code> / {{ step.apply_action }} / {{ step.rollback_mode }}<span v-if="step.config_hash"> / {{ shortHash(step.config_hash) }}</span></li>
            </ol>
          </div>
          <div v-if="topologyEditor.deploymentStatus" class="topology-deployment-status" role="status">
            <div class="status-line"><strong>{{ t('control.topology.deployment') }}</strong><code>#{{ topologyEditor.deploymentID }}</code><span :class="['status-badge', stateClass(topologyEditor.deploymentStatus.deployment?.state || topologyEditor.deploymentStatus.state)]">{{ topologyEditor.deploymentStatus.deployment?.state || topologyEditor.deploymentStatus.state || '-' }}</span></div>
            <p v-if="topologyEditor.deploymentStatus.deployment?.last_error" class="row-error">{{ topologyEditor.deploymentStatus.deployment.last_error }}</p>
            <div v-if="topologyEditor.deploymentStatus.steps?.length" class="secondary-cell">{{ t('control.topology.steps', { count: topologyEditor.deploymentStatus.steps.length }) }}</div>
            <ul v-if="topologyEditor.deploymentStatus.operations?.length" class="deployment-timeline">
              <li v-for="operation in topologyEditor.deploymentStatus.operations.slice(-8)" :key="operation.operation_id">
                <code>{{ operation.kind }}</code> <span :class="['status-badge', stateClass(operation.state)]">{{ operation.state }}</span>
                <span v-if="operation.last_error" class="row-error">{{ operation.last_error }}</span>
              </li>
            </ul>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn" type="button" :disabled="topologyEditor.saving || topologyEditor.loading" @click="closeTopologyEditor">{{ t('common.actions.cancel') }}</button>
          <button id="topology-diagnose" class="btn" type="button" :disabled="topologyEditor.validating || topologyEditor.loading" @click="diagnoseTopology">
            {{ topologyEditor.validating ? t('control.topology.validating') : t('control.topology.diagnose') }}
          </button>
          <button id="topology-preview" class="btn" type="button" :disabled="topologyEditor.validating || topologyEditor.loading || topologyEditorDirty || !topologyEditor.revisionID" @click="previewTopology">{{ t('control.topology.previewAction') }}</button>
          <button id="topology-save-revision" class="btn btn-primary" type="button" :disabled="topologyEditor.saving || topologyEditor.loading" @click="saveTopologyRevision">{{ topologyEditor.saving ? t('control.actions.saving') : t('control.topology.saveRevision') }}</button>
          <button id="topology-plan" class="btn" type="button" :disabled="topologyEditor.saving || topologyEditorDirty || !topologyEditor.revisionID" @click="planTopology">{{ t('control.topology.plan') }}</button>
          <button id="topology-apply" class="btn btn-primary" type="button" :disabled="topologyEditor.saving || topologyEditorDirty || !topologyEditor.deploymentID || !canApplyDeployment(topologyEditor.deploymentStatus)" @click="applyDeployment">{{ t('control.topology.apply') }}</button>
          <button id="topology-rollback" class="btn btn-danger" type="button" :disabled="topologyEditor.saving || topologyEditorDirty || !topologyEditor.deploymentID || !canRollbackDeployment(topologyEditor.deploymentStatus)" @click="rollbackDeployment">{{ t('control.topology.rollback') }}</button>
        </div>
      </section>
    </div>

    <div v-if="newTopologyEditor.open" class="modal-overlay" @click.self="closeNewTopology">
      <section class="modal" role="dialog" aria-modal="true" aria-labelledby="new-topology-title">
        <div class="modal-header">
          <h3 id="new-topology-title">{{ t('control.topology.newTitle') }}</h3>
          <button class="btn btn-ghost close-btn" type="button" :aria-label="t('common.actions.close')" @click="closeNewTopology">x</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label for="new-topology-name">{{ t('control.topology.name') }}</label>
            <input id="new-topology-name" v-model.trim="newTopologyEditor.name" type="text" maxlength="160" />
          </div>
          <div class="form-group">
            <label for="new-topology-scope">{{ t('control.table.scope') }}</label>
            <select id="new-topology-scope" v-model="newTopologyEditor.serviceScope">
              <option v-for="scope in scopes" :key="scope.id" :value="scope.id">{{ scope.name || scope.id }} ({{ scope.id }})</option>
            </select>
          </div>
          <div class="form-group">
            <label for="new-topology-description">{{ t('control.table.description') }}</label>
            <textarea id="new-topology-description" v-model.trim="newTopologyEditor.description" rows="3"></textarea>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn" type="button" :disabled="newTopologyEditor.saving" @click="closeNewTopology">{{ t('common.actions.cancel') }}</button>
          <button id="create-topology" class="btn btn-primary" type="button" :disabled="newTopologyEditor.saving || !newTopologyEditor.name || !newTopologyEditor.serviceScope" @click="createTopology">
            {{ newTopologyEditor.saving ? t('control.actions.saving') : t('control.topology.create') }}
          </button>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAppI18n } from '@/composables/useAppI18n'
import PluginConfigForm from '@/components/admin/PluginConfigForm.vue'
import { getNodes } from '@/api/admin'
import {
  cancelKernelOperation,
  createKernelTopology,
  applyKernelDeployment,
  createKernelTopologyRevision,
  diagnoseKernelTopology,
  deleteKernelNodeAssignment,
  getKernelDeploymentStatus,
  getKernelDeployments,
  getKernelInstallationConfig,
  getKernelInstallations,
  getKernelNodeAssignments,
  getKernelOperations,
  getKernelPluginReleases,
  getKernelPlugins,
  getKernelScopes,
  getKernelTopologyRevision,
  getKernelTopologyRevisions,
  getKernelTopologies,
  planKernelDeployment,
  previewKernelTopologyDeployment,
  registerKernelPluginRelease,
  rollbackKernelDeployment,
  runKernelInstallationAction,
  updateKernelInstallationConfig,
  uploadKernelPluginReleaseArtifact,
  upsertKernelNodeAssignment,
  upsertKernelInstallation,
  validateKernelTopology
} from '@/api/kernel'
import { adminExtensionErrors, adminExtensions, refreshAdminExtensions } from '@/extensions/runtime'

const TERMINAL_OPERATION_STATES = new Set(['succeeded', 'completed', 'failed', 'superseded', 'cancelled', 'timed_out', 'expired', 'rolled_back'])
const CANCELLABLE_OPERATION_STATES = new Set(['pending', 'dispatching', 'running'])
const POLL_INTERVAL_MS = 2000
const OFFICIAL_PLUGIN_ROLES = Object.freeze({
  'machine-telemetry': ['telemetry'],
  'nftables-forward': ['cn_dedicated_nftables', 'entry'],
  'nat-egress': ['nat_egress', 'overseas_exit'],
  'gost-mesh': ['relay', 'tunnel_entry', 'tunnel_exit', 'cn_standard_tunnel_entry']
})

const { t, formatDateTime } = useAppI18n()
const router = useRouter()
const tab = ref('plugins')
const loading = ref(false)
const loaded = ref(false)
const error = ref('')
const notice = ref('')
const plugins = ref([])
const releases = ref([])
const installations = ref([])
const scopes = ref([])
const topologies = ref([])
const deployments = ref([])
const operations = ref([])
const nodes = ref([])
const selectedNodeID = ref(0)
const assignments = ref([])
const assignmentsLoading = ref(false)
const assignmentBusy = ref({})
const rowBusy = ref({})
const operationBusy = ref('')
const trackedOperationIDs = new Set()
const polling = ref(false)
const selectedTopologyID = ref(0)
const topologyLoading = ref(false)
const topologyRevisions = ref([])
const topologyEditor = reactive({
  open: false,
  topology: null,
  revisionID: 0,
  message: '',
  json: '{\n  "vertices": [],\n  "edges": []\n}',
  baselineJSON: '{\n  "vertices": [],\n  "edges": []\n}',
  baselineMessage: '',
  rolloutGroup: '',
  failurePolicy: 'stop_and_rollback',
  validation: null,
  deploymentID: 0,
  deploymentStatus: null,
  preview: null,
  loading: false,
  validating: false,
  saving: false
})
const newTopologyEditor = reactive({ open: false, name: '', serviceScope: '', description: '', saving: false })
let pollTimer = null
let pollRequestRunning = false
let disposed = false

const installationEditor = reactive({ open: false, mode: 'install', plugin: null, row: null, target: 'control', targets: [], version: '', enabled: true, saving: false })
const configEditor = reactive({ open: false, row: null, schema: {}, value: {}, revision: 0, valid: true, loading: false, saving: false })
const releaseImport = reactive({ open: false, manifest: '', signature: '', artifactBase64: '', artifactName: '', saving: false })
const assignmentEditor = reactive({
  open: false,
  mode: 'create',
  assignmentID: 0,
  nodeID: 0,
  pluginID: '',
  serviceScope: '',
  role: '',
  desiredVersion: '',
  desiredConfigRevision: 0,
  rolloutGroup: '',
  enabled: true,
  saving: false
})

const tabs = computed(() => [
  { key: 'plugins', label: t('control.tabs.plugins') },
  { key: 'assignments', label: t('control.tabs.assignments') },
  { key: 'scopes', label: t('control.tabs.scopes') },
  { key: 'topologies', label: t('control.tabs.topologies') },
  { key: 'operations', label: t('control.tabs.operations') }
])

const pluginRows = computed(() => plugins.value.flatMap(plugin => {
  const pluginReleases = releasesForPlugin(plugin.id)
  const targetSet = new Set(pluginReleases.flatMap(releaseTargets))
  const matchingInstallations = installations.value.filter(item => item.plugin_id === plugin.id)
  for (const installation of matchingInstallations) targetSet.add(installation.target)
  if (targetSet.size === 0) targetSet.add('')
  return [...targetSet].sort().map(target => {
    const targetReleases = pluginReleases.filter(release => !target || releaseTargets(release).includes(target))
    const installation = matchingInstallations.find(item => item.target === target) || null
    const latestRelease = targetReleases[0] || null
    return {
      key: `${plugin.id}:${target || 'catalogue'}`,
      plugin,
      target,
      releases: targetReleases,
      latestRelease,
      installation,
      upgradeRelease: installation && latestRelease?.version !== installation.desired_version ? latestRelease : null
    }
  })
}))

const editorReleases = computed(() => releasesForPlugin(installationEditor.plugin?.id).filter(release => releaseTargets(release).includes(installationEditor.target)))
const agentPluginOptions = computed(() => installations.value
  .filter(installation => installation.target === 'agent')
  .map(installation => ({
    pluginID: installation.plugin_id,
    name: pluginName(installation.plugin_id),
    installation
  }))
  .sort((left, right) => left.name.localeCompare(right.name)))
const assignmentReleaseOptions = computed(() => releasesForPlugin(assignmentEditor.pluginID).filter(release => releaseTargets(release).includes('agent')))
const assignmentRoleSuggestions = computed(() => OFFICIAL_PLUGIN_ROLES[assignmentEditor.pluginID] || [])
const assignmentEditorValid = computed(() => Boolean(
  assignmentEditor.nodeID &&
  assignmentEditor.pluginID &&
  assignmentEditor.serviceScope &&
  assignmentEditor.role &&
  assignmentEditor.desiredVersion &&
  Number.isInteger(Number(assignmentEditor.desiredConfigRevision)) &&
  Number(assignmentEditor.desiredConfigRevision) >= 0
))

const topologyDeploymentRows = computed(() => deployments.value.slice().sort((left, right) => {
  const leftTime = Date.parse(left?.created_at || '') || 0
  const rightTime = Date.parse(right?.created_at || '') || 0
  return rightTime - leftTime || Number(right?.id || 0) - Number(left?.id || 0)
}))
const topologyEditorDirty = computed(() => topologyEditor.open && (
  topologyEditor.json !== topologyEditor.baselineJSON || topologyEditor.message.trim() !== topologyEditor.baselineMessage.trim()
))

function parseManifest(release) {
  try {
    return typeof release?.manifest === 'string' ? JSON.parse(release.manifest) : release?.manifest || {}
  } catch {
    return {}
  }
}

function releaseTargets(release) {
  const targets = parseManifest(release).targets
  return Array.isArray(targets) ? targets.filter(target => target === 'control' || target === 'agent') : []
}

function releasesForPlugin(pluginID) {
  return releases.value.filter(release => release.plugin_id === pluginID)
}

function pluginName(pluginID) {
  return plugins.value.find(plugin => plugin.id === pluginID)?.name || pluginID
}

function shortHash(value) {
  return typeof value === 'string' && value.length > 12 ? value.slice(0, 12) : value || '-'
}

function latestDeploymentFor(topology) {
  return topologyDeploymentRows.value.find(deployment => Number(deployment.topology_id) === Number(topology?.id)) || null
}

function topologyGraphValue(value) {
  if (typeof value === 'string') {
    try {
      return JSON.parse(value || '{}')
    } catch {
      return {}
    }
  }
  return value && typeof value === 'object' ? value : {}
}

function topologyInputFromJSON() {
  let value
  try {
    value = JSON.parse(topologyEditor.json || '{}')
  } catch {
    throw new Error(t('control.topology.invalidJSON'))
  }
  const vertices = Array.isArray(value?.vertices) ? value.vertices.map(vertex => ({
    key: String(vertex?.key || '').trim(),
    kind: String(vertex?.kind || '').trim(),
    node_id: vertex?.node_id === null || vertex?.node_id === undefined || vertex?.node_id === '' ? null : Number(vertex.node_id),
    plugin_id: String(vertex?.plugin_id || '').trim(),
    role: String(vertex?.role || '').trim(),
    config: JSON.stringify(topologyGraphValue(vertex?.config ?? vertex?.config_json))
  })) : []
  const edges = Array.isArray(value?.edges) ? value.edges.map(edge => ({
    source_key: String(edge?.source_key || '').trim(),
    target_key: String(edge?.target_key || '').trim(),
    protocol: String(edge?.protocol || '').trim(),
    secret_id: String(edge?.secret_id || '').trim(),
    config: JSON.stringify(topologyGraphValue(edge?.config ?? edge?.config_json))
  })) : []
  return { message: topologyEditor.message.trim(), vertices, edges }
}

function topologyJSONFromDetail(detail) {
  const vertices = Array.isArray(detail?.vertices) ? detail.vertices.map(vertex => ({
    key: vertex.key,
    kind: vertex.kind,
    ...(vertex.node_id ? { node_id: vertex.node_id } : {}),
    ...(vertex.plugin_id ? { plugin_id: vertex.plugin_id } : {}),
    ...(vertex.role ? { role: vertex.role } : {}),
    config: topologyGraphValue(vertex.config)
  })) : []
  const edges = Array.isArray(detail?.edges) ? detail.edges.map(edge => ({
    source_key: edge.source_key,
    target_key: edge.target_key,
    protocol: edge.protocol,
    ...(edge.secret_id ? { secret_id: edge.secret_id } : {}),
    config: topologyGraphValue(edge.config)
  })) : []
  return JSON.stringify({ vertices, edges }, null, 2)
}

function extractNodes(response) {
  if (response?.code !== undefined && response.code !== 0) throw new Error(response.msg || t('control.errors.nodesLoad'))
  const payload = response?.code !== undefined ? response.data : response?.data?.data ?? response?.data ?? response
  const rows = Array.isArray(payload) ? payload : payload?.list
  return Array.isArray(rows) ? rows.map(node => ({ ...node, id: Number(node.id) })).filter(node => node.id > 0) : []
}

function agentInstallationFor(pluginID) {
  return installations.value.find(installation => installation.plugin_id === pluginID && installation.target === 'agent')
}

function defaultScopeFor(pluginID) {
  const preferredScope = pluginID === 'machine-telemetry' ? ['monitoring', 'system'] : ['forward']
  return preferredScope.find(scopeID => scopes.value.some(scope => scope.id === scopeID)) || scopes.value[0]?.id || ''
}

function schemaFor(row) {
  const runtimeExtension = adminExtensions.value.find(extension => extension.installationID === row.installation?.id)
  if (runtimeExtension?.configSchema) return runtimeExtension.configSchema
  const release = row.releases.find(item => item.version === row.installation?.desired_version)
  const schema = parseManifest(release).config_schema
  return schema && typeof schema === 'object' && !Array.isArray(schema) ? schema : {}
}

function stateClass(state) {
  if (state === 'healthy' || state === 'enabled' || state === 'succeeded' || state === 'completed') return 'status-active'
  if (state === 'failed' || state === 'superseded' || state === 'disabled' || state === 'cancel_requested' || state === 'cancelled' || state === 'timed_out') return 'status-error'
  return 'status-pending'
}

function formatDate(value) {
  if (!value) return '-'
  return formatDateTime(value, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', hour12: false }) || '-'
}

function moveTab(event, index) {
  let nextIndex = index
  if (event.key === 'ArrowRight') nextIndex = (index + 1) % tabs.value.length
  else if (event.key === 'ArrowLeft') nextIndex = (index - 1 + tabs.value.length) % tabs.value.length
  else if (event.key === 'Home') nextIndex = 0
  else if (event.key === 'End') nextIndex = tabs.value.length - 1
  else return
  event.preventDefault()
  tab.value = tabs.value[nextIndex].key
  document.getElementById(`control-tab-${tab.value}`)?.focus()
}

function errorMessage(cause, fallbackKey) {
  const response = cause?.response?.data
  return response?.error?.message || (typeof response?.error === 'string' ? response.error : '') || response?.message || response?.msg || cause?.message || t(fallbackKey)
}

async function load(options = {}) {
  if (!options.silent) loading.value = true
  if (!options.silent) error.value = ''
  try {
    const [pluginRowsValue, releaseRows, installationRows, scopeRows, topologyRows, deploymentRows, operationRows, nodeRows] = await Promise.all([
      getKernelPlugins(), getKernelPluginReleases(), getKernelInstallations(), getKernelScopes(), getKernelTopologies(), getKernelDeployments(), getKernelOperations(), getNodes({ page: 1, page_size: 200 })
    ])
    plugins.value = Array.isArray(pluginRowsValue) ? pluginRowsValue : []
    releases.value = Array.isArray(releaseRows) ? releaseRows : []
    installations.value = Array.isArray(installationRows) ? installationRows : []
    scopes.value = Array.isArray(scopeRows) ? scopeRows : []
    topologies.value = Array.isArray(topologyRows) ? topologyRows : []
    deployments.value = Array.isArray(deploymentRows) ? deploymentRows : []
    operations.value = Array.isArray(operationRows) ? operationRows : []
    if (!topologies.value.some(topology => Number(topology.id) === Number(selectedTopologyID.value))) selectedTopologyID.value = Number(topologies.value[0]?.id || 0)
    nodes.value = extractNodes(nodeRows)
    if (!nodes.value.some(node => node.id === Number(selectedNodeID.value))) selectedNodeID.value = nodes.value[0]?.id || 0
    if (selectedNodeID.value) {
      assignmentsLoading.value = true
      const assignmentRows = await getKernelNodeAssignments(selectedNodeID.value)
      assignments.value = Array.isArray(assignmentRows) ? assignmentRows : []
    } else {
      assignments.value = []
    }
    loaded.value = true
    updatePolling()
  } catch (cause) {
    error.value = errorMessage(cause, 'control.errors.load')
  } finally {
    assignmentsLoading.value = false
    if (!options.silent) loading.value = false
  }
}

async function loadTopologies() {
  topologyLoading.value = true
  error.value = ''
  try {
    const [topologyRows, deploymentRows] = await Promise.all([getKernelTopologies(), getKernelDeployments()])
    topologies.value = Array.isArray(topologyRows) ? topologyRows : []
    deployments.value = Array.isArray(deploymentRows) ? deploymentRows : []
    if (!topologies.value.some(topology => Number(topology.id) === Number(selectedTopologyID.value))) selectedTopologyID.value = Number(topologies.value[0]?.id || 0)
  } catch (cause) {
    error.value = errorMessage(cause, 'control.errors.topologyLoad')
  } finally {
    topologyLoading.value = false
  }
}

function openNewTopology() {
  Object.assign(newTopologyEditor, {
    open: true,
    name: '',
    serviceScope: scopes.value[0]?.id || '',
    description: '',
    saving: false
  })
}

function closeNewTopology() {
  if (!newTopologyEditor.saving) newTopologyEditor.open = false
}

async function createTopology() {
  if (!newTopologyEditor.name || !newTopologyEditor.serviceScope) return
  newTopologyEditor.saving = true
  error.value = ''
  notice.value = ''
  try {
    const topology = await createKernelTopology({
      name: newTopologyEditor.name,
      service_scope: newTopologyEditor.serviceScope,
      description: newTopologyEditor.description
    })
    newTopologyEditor.open = false
    await loadTopologies()
    selectedTopologyID.value = Number(topology?.id || selectedTopologyID.value)
    notice.value = t('control.messages.topologyCreated', { name: topology?.name || newTopologyEditor.name })
    if (topology?.id) await openTopologyEditor(topology)
  } catch (cause) {
    error.value = errorMessage(cause, 'control.errors.topologyCreate')
  } finally {
    newTopologyEditor.saving = false
  }
}

async function openTopologyEditor(topology = null) {
  const selected = topology || topologies.value.find(row => Number(row.id) === Number(selectedTopologyID.value))
  if (!selected) return
  selectedTopologyID.value = Number(selected.id)
  Object.assign(topologyEditor, {
    open: true,
    topology: selected,
    revisionID: 0,
    message: '',
    json: '{\n  "vertices": [],\n  "edges": []\n}',
    baselineJSON: '{\n  "vertices": [],\n  "edges": []\n}',
    baselineMessage: '',
    rolloutGroup: '',
    failurePolicy: 'stop_and_rollback',
    validation: null,
    deploymentID: 0,
    deploymentStatus: null,
    preview: null,
    loading: true,
    validating: false,
    saving: false
  })
  try {
    const rows = await getKernelTopologyRevisions(selected.id)
    topologyRevisions.value = Array.isArray(rows) ? rows : []
    const active = topologyRevisions.value.find(row => Number(row.id) === Number(selected.active_revision_id))
    topologyEditor.revisionID = Number(active?.id || topologyRevisions.value[0]?.id || 0)
    if (topologyEditor.revisionID) await loadTopologyRevisionDetail()
    const deployment = latestDeploymentFor(selected)
    if (deployment) {
      topologyEditor.deploymentID = Number(deployment.id)
      await refreshDeploymentStatus(deployment.id)
    }
  } catch (cause) {
    error.value = errorMessage(cause, 'control.errors.topologyLoad')
  } finally {
    topologyEditor.loading = false
  }
}

async function loadTopologyRevisionDetail() {
  if (!topologyEditor.topology?.id || !topologyEditor.revisionID) return
  topologyEditor.loading = true
  error.value = ''
  try {
    const detail = await getKernelTopologyRevision(topologyEditor.topology.id, topologyEditor.revisionID)
    topologyEditor.json = topologyJSONFromDetail(detail)
    topologyEditor.message = detail?.revision?.message || ''
    topologyEditor.baselineJSON = topologyEditor.json
    topologyEditor.baselineMessage = topologyEditor.message
    topologyEditor.validation = null
    topologyEditor.preview = null
    topologyEditor.deploymentID = 0
    topologyEditor.deploymentStatus = null
  } catch (cause) {
    error.value = errorMessage(cause, 'control.errors.topologyLoad')
  } finally {
    topologyEditor.loading = false
  }
}

function closeTopologyEditor() {
  if (!topologyEditor.saving && !topologyEditor.validating) topologyEditor.open = false
}

async function diagnoseTopology() {
  topologyEditor.validating = true
  error.value = ''
  notice.value = ''
  try {
    const input = topologyInputFromJSON()
    const graphResult = await validateKernelTopology(input)
    let result = {
      valid: Boolean(graphResult?.valid),
      issues: Array.isArray(graphResult?.issues) ? graphResult.issues : [],
      checks: []
    }
    if (topologyEditor.topology?.id && topologyEditor.revisionID) {
      const serverResult = await diagnoseKernelTopology(topologyEditor.topology.id, topologyEditor.revisionID, {
        rolloutGroup: topologyEditor.rolloutGroup,
        failurePolicy: topologyEditor.failurePolicy
      })
      result = {
        ...serverResult,
        valid: result.valid && Boolean(serverResult?.valid),
        issues: [...result.issues, ...(Array.isArray(serverResult?.issues) ? serverResult.issues : [])],
        checks: Array.isArray(serverResult?.checks) ? serverResult.checks : []
      }
      topologyEditor.preview = serverResult
    }
    topologyEditor.validation = result
    if (!topologyEditor.validation.valid) throw new Error(t('control.topology.invalid'))
    notice.value = t('control.messages.topologyValidated')
    return true
  } catch (cause) {
    if (cause?.message === t('control.topology.invalid')) {
      // Validation issues are already rendered in the modal; do not replace
      // them with a generic request error.
      return false
    }
    error.value = errorMessage(cause, 'control.errors.topologyValidate')
    return false
  } finally {
    topologyEditor.validating = false
  }
}

async function saveTopologyRevision() {
  if (!topologyEditor.topology?.id) return false
  topologyEditor.saving = true
  error.value = ''
  notice.value = ''
  try {
    const input = topologyInputFromJSON()
    const validation = await validateKernelTopology(input)
    topologyEditor.validation = {
      valid: Boolean(validation?.valid),
      issues: Array.isArray(validation?.issues) ? validation.issues : []
    }
    if (!topologyEditor.validation.valid) throw new Error(t('control.topology.invalid'))
    const revision = await createKernelTopologyRevision(topologyEditor.topology.id, input)
    const rows = await getKernelTopologyRevisions(topologyEditor.topology.id)
    topologyRevisions.value = Array.isArray(rows) ? rows : [...topologyRevisions.value, revision]
    topologyEditor.revisionID = Number(revision?.id || topologyRevisions.value[0]?.id || 0)
    topologyEditor.message = revision?.message || input.message
    topologyEditor.baselineJSON = topologyEditor.json
    topologyEditor.baselineMessage = topologyEditor.message
    topologyEditor.preview = null
    topologyEditor.deploymentID = 0
    topologyEditor.deploymentStatus = null
    await loadTopologies()
    notice.value = t('control.messages.topologyRevisionSaved', { revision: revision?.revision || '-' })
    return true
  } catch (cause) {
    error.value = errorMessage(cause, 'control.errors.topologySave')
    return false
  } finally {
    topologyEditor.saving = false
  }
}

async function previewTopology() {
  if (!topologyEditor.topology?.id || !topologyEditor.revisionID) return false
  topologyEditor.validating = true
  error.value = ''
  try {
    const preview = await previewKernelTopologyDeployment(topologyEditor.topology.id, topologyEditor.revisionID, {
      rolloutGroup: topologyEditor.rolloutGroup,
      failurePolicy: topologyEditor.failurePolicy
    })
    topologyEditor.preview = preview
    topologyEditor.validation = preview
    return Boolean(preview?.valid)
  } catch (cause) {
    error.value = errorMessage(cause, 'control.errors.topologyPreview')
    return false
  } finally {
    topologyEditor.validating = false
  }
}

async function planTopology() {
  if (!topologyEditor.topology?.id || !topologyEditor.revisionID || topologyEditorDirty.value) return false
  topologyEditor.saving = true
  error.value = ''
  notice.value = ''
  try {
    const preview = await previewKernelTopologyDeployment(topologyEditor.topology.id, topologyEditor.revisionID, {
      rolloutGroup: topologyEditor.rolloutGroup,
      failurePolicy: topologyEditor.failurePolicy
    })
    topologyEditor.preview = preview
    topologyEditor.validation = preview
    if (!preview?.valid) throw new Error(t('control.topology.invalid'))
    const result = await planKernelDeployment({
      topology_id: Number(topologyEditor.topology.id),
      revision_id: Number(topologyEditor.revisionID),
      rollout_group: topologyEditor.rolloutGroup.trim(),
      failure_policy: topologyEditor.failurePolicy
    })
    const deployment = result?.deployment || result
    topologyEditor.deploymentID = Number(deployment?.id || 0)
    if (!topologyEditor.deploymentID) throw new Error(t('control.errors.topologyPlan'))
    deployments.value = [deployment, ...deployments.value.filter(row => Number(row.id) !== topologyEditor.deploymentID)]
    await refreshDeploymentStatus(topologyEditor.deploymentID)
    notice.value = t('control.messages.topologyPlanned', { id: topologyEditor.deploymentID })
    return true
  } catch (cause) {
    error.value = errorMessage(cause, 'control.errors.topologyPlan')
    return false
  } finally {
    topologyEditor.saving = false
  }
}

async function refreshDeploymentStatus(deploymentID = topologyEditor.deploymentID) {
  if (!deploymentID) return null
  const status = await getKernelDeploymentStatus(deploymentID)
  topologyEditor.deploymentID = Number(deploymentID)
  topologyEditor.deploymentStatus = status
  const deployment = status?.deployment || status
  if (deployment?.id) deployments.value = [deployment, ...deployments.value.filter(row => Number(row.id) !== Number(deployment.id))]
  return status
}

async function openDeploymentStatus(deployment) {
  const topology = topologies.value.find(row => Number(row.id) === Number(deployment?.topology_id))
  await openTopologyEditor(topology)
  if (deployment?.id) {
    try {
      await refreshDeploymentStatus(deployment.id)
    } catch (cause) {
      error.value = errorMessage(cause, 'control.errors.topologyStatus')
    }
  }
}

function deploymentState(status) {
  return status?.deployment?.state || status?.state || ''
}

function canApplyDeployment(status) {
  return ['planned', 'applying'].includes(deploymentState(status))
}

function canRollbackDeployment(status) {
  return !['', 'rollback_requested', 'rollback_configuring', 'rollback_enabling', 'rollback_disabling', 'rolled_back'].includes(deploymentState(status))
}

async function applyDeployment() {
  if (!topologyEditor.deploymentID || topologyEditorDirty.value) return
  topologyEditor.saving = true
  error.value = ''
  try {
    await applyKernelDeployment(topologyEditor.deploymentID)
    await refreshDeploymentStatus()
    await loadTopologies()
    notice.value = t('control.messages.topologyApplyRequested')
  } catch (cause) {
    error.value = errorMessage(cause, 'control.errors.topologyApply')
  } finally {
    topologyEditor.saving = false
  }
}

async function rollbackDeployment() {
  if (!topologyEditor.deploymentID || topologyEditorDirty.value) return
  if (!confirm(t('control.topology.rollbackConfirm'))) return
  topologyEditor.saving = true
  error.value = ''
  try {
    await rollbackKernelDeployment(topologyEditor.deploymentID)
    await refreshDeploymentStatus()
    await loadTopologies()
    notice.value = t('control.messages.topologyRollbackRequested')
  } catch (cause) {
    error.value = errorMessage(cause, 'control.errors.topologyRollback')
  } finally {
    topologyEditor.saving = false
  }
}

async function loadAssignments() {
  error.value = ''
  notice.value = ''
  if (!selectedNodeID.value) {
    assignments.value = []
    return
  }
  assignmentsLoading.value = true
  try {
    const rows = await getKernelNodeAssignments(selectedNodeID.value)
    assignments.value = Array.isArray(rows) ? rows : []
  } catch (cause) {
    error.value = errorMessage(cause, 'control.errors.assignmentsLoad')
  } finally {
    assignmentsLoading.value = false
  }
}

function selectAssignmentPluginDefaults() {
  const installation = agentInstallationFor(assignmentEditor.pluginID)
  const compatibleReleases = releasesForPlugin(assignmentEditor.pluginID).filter(release => releaseTargets(release).includes('agent'))
  assignmentEditor.desiredVersion = installation?.desired_version || compatibleReleases[0]?.version || ''
  assignmentEditor.desiredConfigRevision = Number(installation?.config_revision ?? 0)
  assignmentEditor.serviceScope = defaultScopeFor(assignmentEditor.pluginID)
  assignmentEditor.role = assignmentRoleSuggestions.value[0] || ''
}

function openAssignmentEditor(assignment = null) {
  if (assignment) {
    Object.assign(assignmentEditor, {
      open: true,
      mode: 'edit',
      assignmentID: assignment.id,
      nodeID: Number(assignment.node_id || selectedNodeID.value),
      pluginID: assignment.plugin_id,
      serviceScope: assignment.service_scope,
      role: assignment.role,
      desiredVersion: assignment.desired_version || '',
      desiredConfigRevision: Number(assignment.desired_config_revision ?? 0),
      rolloutGroup: assignment.rollout_group || '',
      enabled: Boolean(assignment.enabled),
      saving: false
    })
    return
  }

  const firstPlugin = agentPluginOptions.value[0]?.pluginID || ''
  Object.assign(assignmentEditor, {
    open: true,
    mode: 'create',
    assignmentID: 0,
    nodeID: Number(selectedNodeID.value),
    pluginID: firstPlugin,
    serviceScope: '',
    role: '',
    desiredVersion: '',
    desiredConfigRevision: 0,
    rolloutGroup: '',
    enabled: true,
    saving: false
  })
  selectAssignmentPluginDefaults()
}

function closeAssignmentEditor() {
  if (!assignmentEditor.saving) assignmentEditor.open = false
}

function assignmentPayload(source, overrides = {}) {
  return {
    service_scope: source.service_scope,
    plugin_id: source.plugin_id,
    role: source.role,
    desired_version: source.desired_version || '',
    desired_config_revision: Math.max(0, Number(source.desired_config_revision ?? 0)),
    enabled: Boolean(source.enabled),
    rollout_group: source.rollout_group || '',
    ...overrides
  }
}

async function saveAssignment() {
  if (!assignmentEditorValid.value) return
  assignmentEditor.saving = true
  error.value = ''
  notice.value = ''
  const nodeID = Number(assignmentEditor.nodeID)
  try {
    await upsertKernelNodeAssignment(nodeID, assignmentPayload({
      service_scope: assignmentEditor.serviceScope,
      plugin_id: assignmentEditor.pluginID,
      role: assignmentEditor.role,
      desired_version: assignmentEditor.desiredVersion,
      desired_config_revision: assignmentEditor.desiredConfigRevision,
      enabled: assignmentEditor.enabled,
      rollout_group: assignmentEditor.rolloutGroup
    }))
    selectedNodeID.value = nodeID
    assignmentEditor.open = false
    await afterMutation(t('control.messages.assignmentSaved', { plugin: pluginName(assignmentEditor.pluginID) }))
  } catch (cause) {
    error.value = errorMessage(cause, 'control.errors.assignmentSave')
  } finally {
    assignmentEditor.saving = false
  }
}

function isAssignmentBusy(assignment) {
  return Boolean(assignmentBusy.value[assignment.id])
}

function setAssignmentBusy(assignment, action = '') {
  assignmentBusy.value = { ...assignmentBusy.value, [assignment.id]: action }
  if (!action) delete assignmentBusy.value[assignment.id]
}

async function toggleAssignment(assignment) {
  setAssignmentBusy(assignment, 'toggle')
  error.value = ''
  notice.value = ''
  try {
    await upsertKernelNodeAssignment(selectedNodeID.value, assignmentPayload(assignment, { enabled: !assignment.enabled }))
    await afterMutation(t('control.messages.assignmentStateSaved', { plugin: pluginName(assignment.plugin_id) }))
  } catch (cause) {
    error.value = errorMessage(cause, 'control.errors.assignmentSave')
  } finally {
    setAssignmentBusy(assignment)
  }
}

async function removeAssignment(assignment) {
  if (!confirm(t('control.assignments.deleteConfirm', { plugin: pluginName(assignment.plugin_id), role: assignment.role }))) return
  setAssignmentBusy(assignment, 'delete')
  error.value = ''
  notice.value = ''
  try {
    await deleteKernelNodeAssignment(selectedNodeID.value, assignment.id)
    await afterMutation(t('control.messages.assignmentDeleted', { plugin: pluginName(assignment.plugin_id) }))
  } catch (cause) {
    error.value = errorMessage(cause, 'control.errors.assignmentDelete')
  } finally {
    setAssignmentBusy(assignment)
  }
}

function isBusy(row) {
  return Boolean(rowBusy.value[row.key])
}

function setRowBusy(row, action = '') {
  rowBusy.value = { ...rowBusy.value, [row.key]: action }
  if (!action) delete rowBusy.value[row.key]
}

function idempotencyKey(action, installationID) {
  const random = globalThis.crypto?.randomUUID?.() || `${Date.now()}-${Math.random().toString(16).slice(2)}`
  return `webui:${installationID}:${action}:${random}`
}

function trackOperation(result) {
  const operationID = result?.operation?.id
  if (operationID) trackedOperationIDs.add(operationID)
}

async function refreshExtensions() {
  if (!router) return
  await refreshAdminExtensions(router)
}

async function afterMutation(message) {
  notice.value = message
  await load({ silent: true })
  await refreshExtensions()
  updatePolling()
}

async function runLifecycle(row, action, targetVersion = '') {
  setRowBusy(row, action)
  error.value = ''
  notice.value = ''
  try {
    if (row.target === 'control') {
      const result = await runKernelInstallationAction(row.installation.id, action, {
        targetVersion,
        idempotencyKey: idempotencyKey(action, row.installation.id)
      })
      trackOperation(result)
    } else {
      const enabled = action === 'disable' ? false : true
      const desiredVersion = action === 'rollback' ? row.installation.previous_version : targetVersion || row.installation.desired_version
      await upsertKernelInstallation({ plugin_id: row.plugin.id, target: row.target, desired_version: desiredVersion, enabled })
    }
    await afterMutation(t('control.messages.actionQueued', { action: t(`control.actions.${action}`), plugin: row.plugin.name || row.plugin.id }))
    return true
  } catch (cause) {
    error.value = errorMessage(cause, 'control.errors.action')
    return false
  } finally {
    setRowBusy(row)
  }
}

function openInstall(row) {
  const targets = [...new Set(releasesForPlugin(row.plugin.id).flatMap(releaseTargets))]
  Object.assign(installationEditor, {
    open: true,
    mode: 'install',
    plugin: row.plugin,
    row,
    target: row.target || targets[0] || 'control',
    targets,
    version: row.latestRelease?.version || '',
    enabled: true,
    saving: false
  })
  selectDefaultEditorVersion()
}

function openUpgrade(row) {
  Object.assign(installationEditor, {
    open: true,
    mode: 'update',
    plugin: row.plugin,
    row,
    target: row.target,
    targets: [row.target],
    version: row.upgradeRelease?.version || '',
    enabled: true,
    saving: false
  })
}

function selectDefaultEditorVersion() {
  const candidates = releasesForPlugin(installationEditor.plugin?.id).filter(release => releaseTargets(release).includes(installationEditor.target))
  if (!candidates.some(release => release.version === installationEditor.version)) installationEditor.version = candidates[0]?.version || ''
}

function closeInstallationEditor() {
  if (!installationEditor.saving) installationEditor.open = false
}

async function saveInstallation() {
  installationEditor.saving = true
  error.value = ''
  notice.value = ''
  try {
    if (installationEditor.mode === 'update' && installationEditor.row?.installation) {
      const succeeded = await runLifecycle(installationEditor.row, 'update', installationEditor.version)
      if (!succeeded) return
    } else {
      await upsertKernelInstallation({
        plugin_id: installationEditor.plugin.id,
        target: installationEditor.target,
        desired_version: installationEditor.version,
        enabled: installationEditor.enabled
      })
      await afterMutation(t('control.messages.installed', { plugin: installationEditor.plugin.name || installationEditor.plugin.id }))
    }
    installationEditor.open = false
  } catch (cause) {
    error.value = errorMessage(cause, 'control.errors.install')
  } finally {
    installationEditor.saving = false
  }
}

async function openConfig(row) {
  Object.assign(configEditor, { open: true, row, schema: schemaFor(row), value: {}, revision: row.installation.config_revision || 0, valid: true, loading: true, saving: false })
  error.value = ''
  try {
    const configuration = await getKernelInstallationConfig(row.installation.id)
    const rawConfig = configuration?.config ?? {}
    configEditor.value = typeof rawConfig === 'string' ? JSON.parse(rawConfig || '{}') : rawConfig
    configEditor.revision = configuration?.revision ?? row.installation.config_revision ?? 0
  } catch (cause) {
    error.value = errorMessage(cause, 'control.errors.configLoad')
    configEditor.open = false
  } finally {
    configEditor.loading = false
  }
}

function closeConfigEditor() {
  if (!configEditor.saving) configEditor.open = false
}

async function saveConfig() {
  configEditor.saving = true
  error.value = ''
  notice.value = ''
  try {
    const configuration = await updateKernelInstallationConfig(configEditor.row.installation.id, configEditor.value, configEditor.revision)
    configEditor.revision = configuration?.revision ?? configEditor.revision
    await afterMutation(t('control.messages.configSaved', { plugin: configEditor.row.plugin.name || configEditor.row.plugin.id }))
    configEditor.open = false
  } catch (cause) {
    error.value = errorMessage(cause, 'control.errors.configSave')
  } finally {
    configEditor.saving = false
  }
}

function openReleaseImport() {
  Object.assign(releaseImport, { open: true, manifest: '', signature: '', artifactBase64: '', artifactName: '', saving: false })
}

function closeReleaseImport() {
  if (!releaseImport.saving) releaseImport.open = false
}

async function readTextFile(event, field) {
  const file = event.target.files?.[0]
  if (file) releaseImport[field] = await file.text()
}

function readManifestFile(event) { return readTextFile(event, 'manifest') }
function readSignatureFile(event) { return readTextFile(event, 'signature') }

async function readArtifactFile(event) {
  const file = event.target.files?.[0]
  if (!file) return
  releaseImport.artifactName = `${file.name} (${file.size} B)`
  const bytes = new Uint8Array(await file.arrayBuffer())
  let binary = ''
  const chunkSize = 0x8000
  for (let offset = 0; offset < bytes.length; offset += chunkSize) {
    binary += String.fromCharCode(...bytes.subarray(offset, offset + chunkSize))
  }
  releaseImport.artifactBase64 = btoa(binary)
}

async function importRelease() {
  releaseImport.saving = true
  error.value = ''
  notice.value = ''
  try {
    JSON.parse(releaseImport.manifest)
    const release = await registerKernelPluginRelease(releaseImport.manifest, releaseImport.signature.trim())
    if (releaseImport.artifactBase64) await uploadKernelPluginReleaseArtifact(release.id, releaseImport.artifactBase64)
    await afterMutation(t('control.messages.releaseImported', { plugin: release.plugin_id, version: release.version }))
    releaseImport.open = false
  } catch (cause) {
    error.value = errorMessage(cause, 'control.errors.releaseImport')
  } finally {
    releaseImport.saving = false
  }
}

function isCancellable(operation) {
  return CANCELLABLE_OPERATION_STATES.has(operation.state)
}

async function cancelOperation(operation) {
  operationBusy.value = operation.id
  error.value = ''
  notice.value = ''
  try {
    await cancelKernelOperation(operation.id)
    trackedOperationIDs.add(operation.id)
    await afterMutation(t('control.messages.cancelRequested'))
  } catch (cause) {
    error.value = errorMessage(cause, 'control.errors.cancel')
  } finally {
    operationBusy.value = ''
  }
}

function hasActiveOperations() {
  for (const operationID of trackedOperationIDs) {
    const operation = operations.value.find(item => item.id === operationID)
    if (!operation || !TERMINAL_OPERATION_STATES.has(operation.state)) return true
  }
  return operations.value.some(operation => {
    const pluginLifecycle = typeof operation.kind === 'string' && operation.kind.startsWith('plugin.')
    const tracked = trackedOperationIDs.has(operation.id)
    return (pluginLifecycle || tracked) && !TERMINAL_OPERATION_STATES.has(operation.state)
  })
}

function updatePolling() {
  if (disposed) {
    polling.value = false
    if (pollTimer) clearInterval(pollTimer)
    pollTimer = null
    return
  }
  const shouldPoll = hasActiveOperations()
  polling.value = shouldPoll
  if (shouldPoll && !pollTimer) {
    pollTimer = setInterval(pollOperations, POLL_INTERVAL_MS)
  } else if (!shouldPoll && pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
    void refreshExtensions()
  }
}

async function pollOperations() {
  if (pollRequestRunning) return
  pollRequestRunning = true
  try {
    const [operationRows, installationRows] = await Promise.all([getKernelOperations(), getKernelInstallations()])
    operations.value = Array.isArray(operationRows) ? operationRows : []
    installations.value = Array.isArray(installationRows) ? installationRows : []
    for (const operationID of [...trackedOperationIDs]) {
      const operation = operations.value.find(item => item.id === operationID)
      if (operation && TERMINAL_OPERATION_STATES.has(operation.state)) trackedOperationIDs.delete(operationID)
    }
  } catch (cause) {
    error.value = errorMessage(cause, 'control.errors.poll')
  } finally {
    pollRequestRunning = false
  }
  updatePolling()
}

onMounted(() => {
  disposed = false
  load()
})
onBeforeUnmount(() => {
  disposed = true
  if (pollTimer) clearInterval(pollTimer)
  pollTimer = null
})
</script>

<style scoped>
.control-page { display: grid; gap: 16px; min-width: 0; }
.page-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; flex-wrap: wrap; }
.page-header h1 { margin: 0; font-size: 24px; line-height: 1.25; }
.page-subtitle, .modal-meta { margin: 5px 0 0; color: var(--text-secondary); font-size: 13px; }
.header-actions, .row-actions { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.tabs { display: flex; gap: 4px; border-bottom: 1px solid var(--border-color, #d7dde7); overflow-x: auto; }
.tab { position: relative; min-height: 40px; border: 0; border-bottom: 2px solid transparent; border-radius: 0; background: transparent; padding: 9px 12px; color: var(--text-secondary, #596579); cursor: pointer; white-space: nowrap; }
.tab:hover { background: var(--surface-hover); }
.tab.active { border-bottom-color: var(--primary-color, #2563eb); color: var(--text-color, #172033); font-weight: 600; }
.polling-dot { display: inline-block; width: 7px; height: 7px; border-radius: 50%; background: var(--warning-color); }
.error-message, .notice-message { margin: 0; overflow-wrap: anywhere; }
.error-message { color: var(--error-color, #b42318); }
.notice-message { color: var(--success-color); }
.extension-error-band { padding: 12px 14px; border-left: 3px solid var(--error-color); background: rgba(220, 38, 38, 0.06); }
.extension-error-band ul { margin: 8px 0 0; padding-left: 20px; }
.extension-error-band li + li { margin-top: 4px; }
.plugins-table { min-width: 1080px; }
.assignments-panel { display: grid; gap: 12px; min-width: 0; }
.assignments-toolbar { display: flex; align-items: end; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.assignment-node-picker { display: grid; gap: 5px; min-width: min(320px, 100%); }
.assignment-node-picker label { color: var(--text-secondary); font-size: 12px; font-weight: 700; }
.assignment-node-picker select { min-height: 38px; }
.assignments-toolbar-actions { display: flex; gap: 8px; flex-wrap: wrap; }
.assignments-table { min-width: 1040px; }
.scopes-table { min-width: 620px; }
.topologies-table { min-width: 980px; }
.operations-table { min-width: 920px; }
.topology-panel { display: grid; gap: 12px; min-width: 0; }
.topology-toolbar { display: flex; align-items: end; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.topology-selection { display: grid; gap: 5px; min-width: min(360px, 100%); }
.topology-selection label, .topology-editor-toolbar label { color: var(--text-secondary); font-size: 12px; font-weight: 700; }
.topology-selection select { min-height: 38px; }
.topology-editor-body { display: grid; gap: 14px; }
.topology-editor-toolbar { display: grid; grid-template-columns: minmax(0, 1.2fr) minmax(0, 1fr) minmax(0, 1fr); gap: 12px; }
.topology-editor-toolbar .form-group { margin: 0; }
.topology-validation, .topology-deployment-status { padding: 10px 12px; border-left: 3px solid var(--warning-color); background: rgba(217, 119, 6, 0.08); }
.topology-validation.is-valid { border-left-color: var(--success-color); background: rgba(22, 163, 74, 0.08); }
.topology-validation.is-invalid { border-left-color: var(--error-color); background: rgba(220, 38, 38, 0.08); }
.topology-dirty { margin: 0; padding: 9px 12px; border-left: 3px solid var(--warning-color); color: var(--warning-color); background: rgba(217, 119, 6, 0.08); font-size: 12px; }
.topology-validation ul { margin: 7px 0 0; padding-left: 20px; }
.topology-validation li + li { margin-top: 4px; }
.topology-checks { display: flex; gap: 6px; flex-wrap: wrap; margin-top: 9px; }
.check-pill { display: inline-flex; padding: 3px 7px; border-radius: 999px; font-size: 11px; }
.check-passed { color: var(--success-color); background: rgba(22, 163, 74, 0.1); }
.check-failed { color: var(--error-color); background: rgba(220, 38, 38, 0.1); }
.topology-preview { padding: 10px 12px; border-left: 3px solid var(--primary-color); background: rgba(37, 99, 235, 0.06); }
.preview-steps { max-height: 160px; margin: 8px 0 0; padding-left: 21px; overflow: auto; }
.preview-steps li + li { margin-top: 4px; }
.deployment-timeline { max-height: 180px; margin: 8px 0 0; padding-left: 20px; overflow: auto; }
.deployment-timeline li { display: flex; align-items: center; gap: 7px; flex-wrap: wrap; }
.deployment-timeline li + li { margin-top: 6px; }
.status-line { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.status-line strong { margin-right: auto; }
.modal-xl { width: min(980px, 100%); }
.primary-cell, .secondary-cell, .plugin-description, .row-error, .version-line { display: block; }
.secondary-cell { width: fit-content; margin-top: 3px; color: var(--text-secondary); font-size: 11px; }
.plugin-description { max-width: 260px; margin-top: 5px; color: var(--text-secondary); font-size: 12px; line-height: 1.4; }
.version-line { display: flex; align-items: center; gap: 7px; font-size: 12px; }
.version-line + .version-line { margin-top: 4px; }
.version-line span { color: var(--text-secondary); }
.row-error { max-width: 280px; margin-top: 6px; color: var(--error-color); font-size: 11px; line-height: 1.4; overflow-wrap: anywhere; }
.description-cell { min-width: 220px; max-width: 520px; overflow-wrap: anywhere; }
.identifier, .operation-id { overflow-wrap: anywhere; }
.state-row td, .state-message { color: var(--text-secondary); text-align: center; }
.status-active { background: rgba(22, 163, 74, 0.1); color: var(--success-color); }
.status-error { background: rgba(220, 38, 38, 0.1); color: var(--error-color); }
.status-pending { background: rgba(217, 119, 6, 0.1); color: var(--warning-color); }
.modal-plugin-name { margin: 0 0 18px; font-size: 16px; font-weight: 700; }
.enable-after-install { display: inline-flex; align-items: center; gap: 9px; font-size: 13px; font-weight: 700; }
.enable-after-install input { width: 18px; height: 18px; margin: 0; }
.assignment-form-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; }
.assignment-form-grid .form-group { min-width: 0; margin: 0; }
.assignment-rollout-field { grid-column: 1 / 2; }
.assignment-enabled-field { align-self: end; min-height: 42px; }
.revision-label { margin-right: auto; color: var(--text-secondary); font-size: 12px; }
.json-textarea { font: 12px/1.5 ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-weight: 500; }
.field-help { margin: 6px 0 0; color: var(--text-secondary); font-size: 12px; }
@media (max-width: 640px) {
  .control-page { gap: 12px; }
  .page-header h1 { font-size: 20px; }
  .header-actions { width: 100%; }
  .header-actions .btn { flex: 1; }
  .assignments-toolbar, .assignment-node-picker, .assignments-toolbar-actions { width: 100%; }
  .assignments-toolbar-actions .btn { flex: 1; }
  .assignment-form-grid { grid-template-columns: 1fr; }
  .assignment-rollout-field { grid-column: auto; }
  .topology-editor-toolbar { grid-template-columns: 1fr; }
  .tab { padding-inline: 10px; }
  .modal-overlay { padding: 10px; }
}
</style>
