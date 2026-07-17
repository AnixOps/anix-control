<template>
  <div class="page nodes-page">
    <div class="page-header">
      <h1>{{ t('admin.nodes.title') }}</h1>
      <div class="header-actions">
        <button class="btn btn-primary" @click="openCreateModal">
          {{ t('admin.nodes.addNode') }}
        </button>
        <button class="btn btn-secondary" @click="openDeployModal">
          {{ t('admin.nodes.actions.deployParents') }}
        </button>
        <button class="btn btn-secondary" @click="openAuthKeyModal">
          {{ t('admin.nodes.actions.authKey') }}
        </button>
      </div>
    </div>

    <!-- Over-quota banner -->
    <div class="quota-banner" v-if="overQuotaNodes.length > 0">
      {{ t('admin.nodes.messages.quotaExceededBanner', { count: overQuotaNodes.length }) }}
    </div>

    <!-- Node stats -->
    <div class="stats-grid">
      <div class="stat-card">
        <div class="stat-value">{{ stats.total }}</div>
        <div class="stat-label">{{ t('admin.nodes.stats.total') }}</div>
      </div>
      <div class="stat-card online">
        <div class="stat-value">{{ stats.online }}</div>
        <div class="stat-label">{{ t('admin.nodes.stats.online') }}</div>
      </div>
      <div class="stat-card warning">
        <div class="stat-value">{{ stats.offline }}</div>
        <div class="stat-label">{{ t('admin.nodes.stats.offline') }}</div>
      </div>
      <div class="stat-card pending">
        <div class="stat-value">{{ stats.pending }}</div>
        <div class="stat-label">{{ t('admin.nodes.stats.pending') }}</div>
      </div>
    </div>

    <!-- Node list -->
    <div class="table-container">
      <table class="table">
        <thead>
          <tr>
            <th>{{ t('networkPages.nodes.table.id') }}</th>
            <th>{{ t('admin.nodes.table.name') }}</th>
            <th>{{ t('admin.nodes.table.address') }}</th>
            <th>{{ t('admin.nodes.table.status') }}</th>
            <th>{{ t('admin.nodes.table.parent') }}</th>
            <th>{{ t('admin.nodes.table.protocols') }}</th>
            <th>{{ t('admin.nodes.table.traffic') }}</th>
            <th>{{ t('admin.nodes.table.monthlyQuota') }}</th>
            <th>{{ t('admin.nodes.table.lastHeartbeat') }}</th>
            <th>{{ t('admin.nodes.table.actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="loading">
            <td colspan="10" class="text-center">{{ t('admin.nodes.table.loading') }}</td>
          </tr>
          <tr v-else-if="nodes.length === 0">
            <td colspan="10" class="text-center">{{ t('admin.nodes.table.empty') }}</td>
          </tr>
          <tr v-for="node in nodes" :key="node.id" :class="{ 'row-over-quota': isOverQuota(node) }">
            <td>{{ node.id }}</td>
            <td>
              <strong>{{ node.name }}</strong>
              <span v-if="node.tags" class="node-tags">
                <span v-for="tag in node.tags.split(',')" :key="tag" class="tag">{{ tag }}</span>
              </span>
            </td>
            <td>
              <code>{{ node.address }}</code>
            </td>
            <td>
              <span :class="['status-badge', getStatusClass(node.status)]">
                {{ getStatusText(node.status) }}
              </span>
              <span
                v-if="node.runtime_checked_at"
                :class="['runtime-health-badge', node.runtime_healthy ? 'runtime-healthy' : 'runtime-unhealthy']"
                :title="node.runtime_healthy ? t('admin.nodes.table.runtimeHealthy') : (node.runtime_error || t('admin.nodes.table.runtimeUnhealthy'))"
              >
                {{ node.runtime_healthy ? t('admin.nodes.table.runtimeHealthy') : t('admin.nodes.table.runtimeUnhealthy') }}
              </span>
            </td>
            <td>{{ getNodeName(node.parent_id) || '-' }}</td>
            <td>{{ node.protocols?.length || 0 }}</td>
            <td>{{ formatBytes(node.traffic_today || 0) }}</td>
            <td>
              <span v-if="!node.monthly_limit">-</span>
              <span v-else :class="['quota-text', isOverQuota(node) ? 'quota-exceeded' : '']">
                {{ formatBytes((node.monthly_upload || 0) + (node.monthly_download || 0)) }} / {{ formatBytes(node.monthly_limit) }}
                <span v-if="isOverQuota(node)" class="quota-tag">{{ t('admin.nodes.table.quotaExceeded') }}</span>
              </span>
            </td>
            <td>{{ formatTime(node.last_check_at) }}</td>
            <td class="actions">
              <button
                class="btn btn-sm btn-info"
                :title="t('admin.nodes.actions.manageProtocols')"
                :aria-label="t('admin.nodes.actions.manageProtocols')"
                @click="openProtocols(node)"
              >
                {{ t('admin.nodes.actions.protocols') }}
              </button>
              <button
                class="btn btn-sm btn-secondary sync-node-btn"
                :disabled="syncingNodeIds.has(node.id)"
                :title="t('admin.nodes.actions.syncReload')"
                :aria-label="t('admin.nodes.actions.syncReload')"
                @click="syncNode(node)"
              >
                {{ syncingNodeIds.has(node.id) ? t('admin.nodes.actions.syncing') : t('admin.nodes.actions.syncReload') }}
              </button>
              <button
                class="btn btn-sm btn-secondary"
                :title="t('admin.nodes.actions.logs')"
                :aria-label="t('admin.nodes.actions.logs')"
                @click="openLogModal(node)"
              >
                {{ t('admin.nodes.actions.logs') }}
              </button>
              <button
                class="btn btn-sm btn-warning"
                :title="t('admin.nodes.actions.edit')"
                :aria-label="t('admin.nodes.actions.edit')"
                @click="openEditModal(node)"
              >
                {{ t('admin.nodes.actions.edit') }}
              </button>
              <button
                class="btn btn-sm btn-danger"
                :title="t('admin.nodes.actions.delete')"
                :aria-label="t('admin.nodes.actions.delete')"
                @click="confirmDelete(node)"
              >
                {{ t('admin.nodes.actions.delete') }}
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Pagination -->
    <div class="pagination" v-if="pagination.total > pagination.size">
      <button :disabled="pagination.page === 1" @click="changePage(pagination.page - 1)">
        {{ t('admin.nodes.pagination.previous') }}
      </button>
      <span>{{ pagination.page }} / {{ Math.ceil(pagination.total / pagination.size) }}</span>
      <button :disabled="pagination.page >= Math.ceil(pagination.total / pagination.size)" @click="changePage(pagination.page + 1)">
        {{ t('admin.nodes.pagination.next') }}
      </button>
    </div>

    <!-- Create/edit node modal -->
    <div class="modal-overlay" v-if="showNodeModal" @click.self="closeNodeModal">
      <div class="modal">
        <div class="modal-header">
          <h3>{{ editingNode ? t('admin.nodes.nodeModal.titleEdit') : t('admin.nodes.nodeModal.titleCreate') }}</h3>
          <button class="close-btn" :title="t('common.actions.close')" :aria-label="t('common.actions.close')" @click="closeNodeModal">×</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label>{{ t('admin.nodes.nodeModal.fields.name') }}</label>
            <input v-model="nodeForm.name" type="text" :placeholder="t('admin.nodes.nodeModal.placeholders.name')" />
          </div>
          <div class="form-group">
            <label>{{ t('admin.nodes.nodeModal.fields.address') }}</label>
            <input v-model="nodeForm.address" type="text" :placeholder="t('admin.nodes.nodeModal.placeholders.address')" />
          </div>
          <div class="form-row">
            <div class="form-group">
              <label>{{ t('admin.nodes.nodeModal.fields.rate') }}</label>
              <input v-model.number="nodeForm.rate" type="number" step="0.1" min="0" :placeholder="t('admin.nodes.nodeModal.placeholders.rate')" />
            </div>
            <div class="form-group">
              <label>{{ t('admin.nodes.nodeModal.fields.sort') }}</label>
              <input v-model.number="nodeForm.sort" type="number" :placeholder="t('admin.nodes.nodeModal.placeholders.sort')" />
            </div>
          </div>
          <div class="form-group">
            <label>{{ t('admin.nodes.nodeModal.fields.tags') }}</label>
            <input v-model="nodeForm.tags" type="text" :placeholder="t('admin.nodes.nodeModal.placeholders.tags')" />
          </div>

          <!-- 中转链路: 父节点 (落地节点为根, 转发节点为子, 支持多级) -->
          <div class="form-group">
            <label>{{ t('admin.nodes.nodeModal.fields.parent') }}</label>
            <select v-model="nodeForm.parent_id">
              <option :value="null">{{ t('admin.nodes.nodeModal.parentNone') }}</option>
              <option v-for="candidate in parentCandidates" :key="candidate.id" :value="candidate.id">
                {{ candidate.name }} ({{ candidate.address || candidate.host }})
              </option>
            </select>
            <p class="field-hint">{{ t('admin.nodes.nodeModal.parentHint') }}</p>
          </div>

          <!-- 月流量限额: 每个节点独立统计, 超限只做标记不自动限制 -->
          <div class="form-row">
            <div class="form-group">
              <label>{{ t('admin.nodes.nodeModal.fields.monthlyLimit') }}</label>
              <input v-model.number="nodeForm.monthly_limit_gb" type="number" min="0" step="0.1" :placeholder="t('admin.nodes.nodeModal.placeholders.monthlyLimit')" />
            </div>
            <div class="form-group">
              <label>{{ t('admin.nodes.nodeModal.fields.monthlyResetDay') }}</label>
              <input v-model.number="nodeForm.monthly_reset_day" type="number" min="1" max="28" :placeholder="t('admin.nodes.nodeModal.placeholders.monthlyResetDay')" />
            </div>
          </div>

          <div class="form-group" v-if="editingNode">
            <label>{{ t('admin.nodes.nodeModal.fields.status') }}</label>
            <select v-model.number="nodeForm.status">
              <option :value="0">{{ t('admin.nodes.statusText.pending') }}</option>
              <option :value="1">{{ t('admin.nodes.statusText.online') }}</option>
              <option :value="2">{{ t('admin.nodes.statusText.offline') }}</option>
              <option :value="3">{{ t('admin.nodes.statusText.disabled') }}</option>
            </select>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary" @click="closeNodeModal">{{ t('admin.nodes.actions.cancel') }}</button>
          <button class="btn btn-primary" @click="saveNode" :disabled="saving">
            {{ saving ? t('admin.nodes.actions.saving') : t('admin.nodes.actions.save') }}
          </button>
        </div>
      </div>
    </div>

    <!-- Auth Key modal -->
    <div class="modal-overlay" v-if="showAuthKeyModal" @click.self="closeAuthKeyModal">
      <div class="modal">
        <div class="modal-header">
          <h3>{{ t('admin.nodes.authKeyModal.title') }}</h3>
          <button class="close-btn" :title="t('common.actions.close')" :aria-label="t('common.actions.close')" @click="closeAuthKeyModal">×</button>
        </div>
        <div class="modal-body">
          <p class="auth-key-hint">{{ t('admin.nodes.authKeyModal.hint') }}</p>
          <div class="auth-key-display">
            <code class="auth-key-value">{{ authKey || t('admin.nodes.authKeyModal.noKey') }}</code>
            <button class="btn btn-sm" @click="copyAuthKey" :disabled="!authKey">
              {{ t('admin.nodes.authKeyModal.copy') }}
            </button>
          </div>
          <div class="auth-key-usage" v-if="authKeyUsed > 0">
            {{ t('admin.nodes.authKeyModal.registeredCount', { count: authKeyUsed }) }}
          </div>
          <div class="auth-key-config">
            <label>{{ t('admin.nodes.authKeyModal.configHint') }}</label>
            <pre class="config-block">{{ configSnippet }}</pre>
            <button class="btn btn-sm" :disabled="!pluginSupervisorCanaryReady" @click="copyConfig">{{ t('admin.nodes.authKeyModal.copyConfig') }}</button>
            <p v-if="deploySettings.pluginSupervisorEnabled && !pluginSupervisorCanaryReady" class="field-hint">
              {{ pluginSupervisorCanaryError }}
            </p>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary" @click="closeAuthKeyModal">{{ t('common.actions.close') }}</button>
        </div>
      </div>
    </div>

    <div class="modal-overlay" v-if="showDeployModal" @click.self="closeDeployModal">
      <div class="modal modal-xl">
        <div class="modal-header">
          <h3>{{ t('admin.nodes.deployModal.title') }}</h3>
          <button class="close-btn" :title="t('common.actions.close')" :aria-label="t('common.actions.close')" @click="closeDeployModal">×</button>
        </div>
        <div class="modal-body">
          <div class="deploy-summary">
            <div>
              <p class="eyebrow">{{ t('admin.nodes.deployModal.summaryEyebrow') }}</p>
              <p class="deploy-summary-title">{{ t('admin.nodes.deployModal.summaryTitle', { count: parentNodes.length }) }}</p>
              <p class="text-secondary">{{ t('admin.nodes.deployModal.summaryText') }}</p>
            </div>
            <p class="text-secondary deploy-warning">{{ t('admin.nodes.deployModal.warning') }}</p>
          </div>

          <div class="deploy-settings-grid">
            <div class="form-group">
              <label>{{ t('admin.nodes.deployModal.fields.panelApiHost') }}</label>
              <input v-model.trim="deploySettings.panelApiHost" type="text" />
            </div>
            <div class="form-group">
              <label>{{ t('admin.nodes.deployModal.fields.grpcHost') }}</label>
              <input v-model.trim="deploySettings.grpcHost" type="text" />
            </div>
            <div class="form-group">
              <label>{{ t('admin.nodes.deployModal.fields.grpcServerName') }}</label>
              <input v-model.trim="deploySettings.grpcServerName" type="text" />
            </div>
            <div class="form-group">
              <label>{{ t('admin.nodes.deployModal.fields.amd64BinaryPath') }}</label>
              <input v-model.trim="deploySettings.amd64BinaryPath" type="text" />
            </div>
            <div class="form-group">
              <label>{{ t('admin.nodes.deployModal.fields.arm64BinaryPath') }}</label>
              <input v-model.trim="deploySettings.arm64BinaryPath" type="text" />
            </div>
            <div class="form-group">
              <label>{{ t('admin.nodes.deployModal.fields.coreType') }}</label>
              <select v-model="deploySettings.coreType">
                <option value="xray">xray</option>
                <option value="sing">sing</option>
              </select>
            </div>
            <div class="form-group">
              <label class="checkbox-label">
                <input v-model="deploySettings.grpcUseTLS" type="checkbox" />
                <span>{{ t('admin.nodes.deployModal.fields.grpcUseTLS') }}</span>
              </label>
            </div>
            <div class="form-group deploy-plugin-supervisor-toggle">
              <label class="checkbox-label">
                <input
                  v-model="deploySettings.pluginSupervisorEnabled"
                  data-testid="plugin-supervisor-enabled"
                  type="checkbox"
                />
                <span>{{ t('admin.nodes.deployModal.fields.pluginSupervisorEnabled') }}</span>
              </label>
              <p class="field-hint">{{ t('admin.nodes.deployModal.pluginSupervisorHint') }}</p>
            </div>
            <template v-if="deploySettings.pluginSupervisorEnabled">
              <div class="form-group">
                <label>{{ t('admin.nodes.deployModal.fields.pluginRoot') }}</label>
                <input v-model.trim="deploySettings.pluginRoot" data-testid="plugin-root" type="text" />
              </div>
              <div class="form-group">
                <label>{{ t('admin.nodes.deployModal.fields.pluginSocketDir') }}</label>
                <input v-model.trim="deploySettings.pluginSocketDir" data-testid="plugin-socket-dir" type="text" />
              </div>
              <div class="form-group deploy-plugin-public-key">
                <label>{{ t('admin.nodes.deployModal.fields.pluginOfficialPublicKey') }}</label>
                <input
                  v-model.trim="deploySettings.pluginOfficialPublicKey"
                  data-testid="plugin-official-public-key"
                  type="text"
                  autocomplete="off"
                />
              </div>
              <div v-if="!pluginSupervisorCanaryReady" class="form-error deploy-plugin-supervisor-error">
                {{ pluginSupervisorCanaryError }}
              </div>
            </template>
          </div>

          <div v-if="deployError" class="form-error">{{ deployError }}</div>
          <div v-if="deployLoading" class="empty-message">{{ t('admin.nodes.deployModal.loading') }}</div>

          <table v-else class="table deploy-table">
            <thead>
              <tr>
                <th>{{ t('admin.nodes.deployModal.table.alias') }}</th>
                <th>{{ t('admin.nodes.deployModal.table.node') }}</th>
                <th>{{ t('admin.nodes.deployModal.table.sshHost') }}</th>
                <th>{{ t('admin.nodes.deployModal.table.port') }}</th>
                <th>{{ t('admin.nodes.deployModal.table.user') }}</th>
                <th>{{ t('admin.nodes.deployModal.table.arch') }}</th>
                <th>{{ t('admin.nodes.deployModal.table.authMode') }}</th>
                <th>{{ t('admin.nodes.deployModal.table.authValue') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="row in deployRows" :key="row.id">
                <td><input v-model.trim="row.alias" type="text" /></td>
                <td>
                  <strong>{{ row.name }}</strong>
                  <div class="deploy-meta">ID {{ row.nodeId }}</div>
                </td>
                <td><input v-model.trim="row.host" type="text" /></td>
                <td><input v-model.number="row.sshPort" type="number" min="1" max="65535" /></td>
                <td><input v-model.trim="row.sshUser" type="text" /></td>
                <td>
                  <select v-model="row.arch">
                    <option value="amd64">amd64</option>
                    <option value="arm64">arm64</option>
                  </select>
                </td>
                <td>
                  <select v-model="row.authMode">
                    <option value="password">{{ t('admin.nodes.deployModal.authModes.password') }}</option>
                    <option value="key">{{ t('admin.nodes.deployModal.authModes.key') }}</option>
                  </select>
                </td>
                <td>
                  <input
                    v-model.trim="row.authValue"
                    :type="row.authMode === 'password' ? 'password' : 'text'"
                    :placeholder="row.authMode === 'password' ? t('admin.nodes.deployModal.placeholders.password') : t('admin.nodes.deployModal.placeholders.privateKey')"
                  />
                </td>
              </tr>
              <tr v-if="deployRows.length === 0">
                <td colspan="8" class="empty-row">{{ t('admin.nodes.deployModal.empty') }}</td>
              </tr>
            </tbody>
          </table>

          <div class="deploy-output-grid">
            <div class="deploy-output-panel">
              <div class="deploy-output-head">
                <strong>inventory.ini</strong>
                <button class="btn btn-sm btn-secondary" @click="copyDeployText(deployInventoryPreview)">{{ t('common.actions.copy') }}</button>
              </div>
              <textarea readonly rows="9" :value="deployInventoryPreview"></textarea>
            </div>
            <div class="deploy-output-panel">
              <div class="deploy-output-head">
                <strong>group_vars/all.yml</strong>
                <button class="btn btn-sm btn-secondary" @click="copyDeployText(deployGroupVarsPreview)">{{ t('common.actions.copy') }}</button>
              </div>
              <textarea readonly rows="9" :value="deployGroupVarsPreview"></textarea>
            </div>
          </div>

          <div class="deploy-output-panel deploy-output-panel-full">
            <div class="deploy-output-head">
              <strong>{{ t('admin.nodes.deployModal.commandsLabel') }}</strong>
              <button class="btn btn-sm btn-secondary" @click="copyDeployText(deployCommandsPreview)">{{ t('common.actions.copy') }}</button>
            </div>
            <textarea readonly rows="6" :value="deployCommandsPreview"></textarea>
          </div>
        </div>
      </div>
    </div>

    <!-- Node log modal -->
    <div class="modal-overlay" v-if="showLogModal" @click.self="closeLogModal">
      <div class="modal modal-xl">
        <div class="modal-header">
          <h3>{{ t('admin.nodes.logModal.title', { name: logNode?.name || '-' }) }}</h3>
          <button class="close-btn" :title="t('common.actions.close')" :aria-label="t('common.actions.close')" @click="closeLogModal">×</button>
        </div>
        <div class="modal-body">
          <div class="log-toolbar">
            <select v-model="logFilter.level">
              <option value="">{{ t('admin.nodes.logModal.filters.allLevels') }}</option>
              <option v-for="level in logLevels" :key="level" :value="level">
                {{ getLogLevelLabel(level) }}
              </option>
            </select>
            <input
              v-model.trim="logFilter.source"
              type="text"
              :placeholder="t('admin.nodes.logModal.filters.sourcePlaceholder')"
            />
            <input
              v-model.trim="logFilter.search"
              type="text"
              :placeholder="t('admin.nodes.logModal.filters.searchPlaceholder')"
              @keyup.enter="refreshLogs"
            />
            <button class="btn btn-secondary btn-sm" :disabled="logLoading" @click="refreshLogs">
              {{ logLoading ? t('admin.nodes.actions.loadingLogs') : t('admin.nodes.actions.refreshLogs') }}
            </button>
          </div>

          <div v-if="logLoading" class="empty-message">{{ t('admin.nodes.logModal.loading') }}</div>

          <table v-else-if="nodeLogs.length > 0" class="table node-log-table">
            <thead>
              <tr>
                <th>{{ t('admin.nodes.logModal.table.time') }}</th>
                <th>{{ t('admin.nodes.logModal.table.level') }}</th>
                <th>{{ t('admin.nodes.logModal.table.source') }}</th>
                <th>{{ t('admin.nodes.logModal.table.message') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="log in nodeLogs" :key="log.id">
                <td class="log-time">{{ formatLogTime(log) }}</td>
                <td>
                  <span :class="['log-level-badge', `log-level-${normalizeLogLevel(log.level)}`]">
                    {{ getLogLevelLabel(log.level) }}
                  </span>
                </td>
                <td class="log-source">{{ log.source || '-' }}</td>
                <td class="log-message-cell">
                  <div class="log-message">{{ log.message }}</div>
                  <div v-if="log.trace_id" class="log-meta">
                    trace: <code>{{ log.trace_id }}</code>
                  </div>
                  <details v-if="log.fields_json" class="log-fields">
                    <summary>{{ t('admin.nodes.logModal.table.fields') }}</summary>
                    <pre>{{ formatLogFields(log.fields, log.fields_json) }}</pre>
                  </details>
                </td>
              </tr>
            </tbody>
          </table>

          <div v-else class="empty-message">{{ t('admin.nodes.logModal.empty') }}</div>

          <div class="pagination modal-pagination" v-if="logPagination.total > logPagination.size">
            <button :disabled="logPagination.page === 1 || logLoading" @click="changeLogPage(logPagination.page - 1)">
              {{ t('admin.nodes.pagination.previous') }}
            </button>
            <span>{{ logPagination.page }} / {{ Math.ceil(logPagination.total / logPagination.size) }}</span>
            <button
              :disabled="logPagination.page >= Math.ceil(logPagination.total / logPagination.size) || logLoading"
              @click="changeLogPage(logPagination.page + 1)"
            >
              {{ t('admin.nodes.pagination.next') }}
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- Protocol management modal -->
    <div class="modal-overlay" v-if="showProtocolModal" @click.self="closeProtocolModal">
      <div class="modal modal-lg">
        <div class="modal-header">
          <h3>{{ t('admin.nodes.protocolModal.title', { name: selectedNode?.name || "-" }) }}</h3>
          <button class="close-btn" :title="t('common.actions.close')" :aria-label="t('common.actions.close')" @click="closeProtocolModal">×</button>
        </div>
        <div class="modal-body">
          <div class="protocol-header">
            <button class="btn btn-primary btn-sm" @click="openAddProtocol">
              + {{ t('admin.nodes.protocolModal.addProtocol') }}
            </button>
          </div>

          <table class="table" v-if="protocols.length > 0">
            <thead>
              <tr>
                <th>{{ t('admin.nodes.protocolModal.table.type') }}</th>
                <th>{{ t('admin.nodes.protocolModal.table.port') }}</th>
                <th>{{ t('admin.nodes.protocolModal.table.status') }}</th>
                <th>{{ t('admin.nodes.protocolModal.table.actions') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="protocol in protocols" :key="protocol.id">
                <td>
                  <span class="protocol-type">{{ (protocol.type || 'unknown').toUpperCase() }}</span>
                </td>
                <td>{{ protocol.port }}</td>
                <td>
                  <span :class="['status-badge', protocol.enable ? 'status-online' : 'status-disabled']">
                    {{ protocol.enable ? t('admin.nodes.protocolModal.status.enabled') : t('admin.nodes.protocolModal.status.disabled') }}
                  </span>
                </td>
                <td>
                  <button class="btn btn-sm btn-warning" @click="editProtocol(protocol)">{{ t('admin.nodes.actions.edit') }}</button>
                  <button class="btn btn-sm btn-danger" @click="deleteProtocol(protocol)">{{ t('admin.nodes.actions.delete') }}</button>
                </td>
              </tr>
            </tbody>
          </table>
          <div v-else class="empty-message">{{ t('admin.nodes.protocolModal.empty') }}</div>
        </div>
      </div>
    </div>

    <!-- Create/edit protocol modal (JSON-first) -->
    <div class="modal-overlay" v-if="showProtocolFormModal" @click.self="closeProtocolFormModal">
      <div class="modal modal-lg">
        <div class="modal-header">
          <h3>{{ editingProtocol ? t('admin.nodes.protocolForm.titleEdit') : t('admin.nodes.protocolForm.titleCreate') }}</h3>
          <button class="close-btn" :title="t('common.actions.close')" :aria-label="t('common.actions.close')" @click="closeProtocolFormModal">×</button>
        </div>
        <div class="modal-body">
          <!-- Template quick-select (only when creating) -->
          <div class="form-group" v-if="!editingProtocol">
            <label>{{ t('admin.nodes.protocolForm.templateLibrary') }}</label>
            <div class="template-grid">
              <div
                v-for="tpl in protocolTemplates"
                :key="tpl.type + tpl.name"
                :class="['template-card', selectedTemplate === tpl.name ? 'active' : '']"
                @click="applyTemplate(tpl)"
              >
                <div class="tpl-name">{{ tpl.name }}</div>
                <div class="tpl-desc">{{ tpl.description }}</div>
              </div>
            </div>
          </div>

          <!-- Mode tabs -->
          <div class="tabs">
            <button :class="['tab-btn', protocolForm.mode === 'json' ? 'active' : '']" @click="protocolForm.mode = 'json'">
              JSON
            </button>
            <button :class="['tab-btn', protocolForm.mode === 'visual' ? 'active' : '']" @click="protocolForm.mode = 'visual'">
              {{ t('admin.nodes.protocolForm.tabs.visual') }}
            </button>
          </div>

          <!-- JSON mode (primary) -->
          <div v-if="protocolForm.mode === 'json'" class="protocol-editor">
            <div class="json-editor-actions">
              <button class="btn btn-sm btn-secondary" @click="formatJson" :disabled="!jsonValid">
                {{ t('admin.nodes.protocolForm.jsonActions.format') }}
              </button>
              <button class="btn btn-sm btn-secondary" @click="copyJson">
                {{ t('admin.nodes.protocolForm.jsonActions.copy') }}
              </button>
              <button class="btn btn-sm btn-secondary" @click="loadTemplateAsJson">
                {{ t('admin.nodes.protocolForm.jsonActions.fromTemplate') }}
              </button>
              <span :class="['json-status', jsonValid ? 'valid' : 'invalid']">
                {{ jsonValid ? t('admin.nodes.protocolForm.jsonStatus.valid') : t('admin.nodes.protocolForm.jsonStatus.invalid') }}
              </span>
            </div>
            <textarea
              v-model="jsonEditorContent"
              class="json-textarea"
              rows="22"
              spellcheck="false"
              :placeholder="jsonPlaceholder"
              @input="onJsonInput"
            ></textarea>
          </div>

          <!-- Visual mode (helper) -->
          <div v-else class="protocol-editor">
            <div class="form-row">
              <div class="form-group">
                <label>{{ t('admin.nodes.protocolForm.fields.type') }}</label>
                <select v-model="protocolForm.type">
                  <option value="vmess">{{ t('networkPages.nodes.protocols.vmess') }}</option>
                  <option value="vless">{{ t('networkPages.nodes.protocols.vless') }}</option>
                  <option value="trojan">{{ t('networkPages.nodes.protocols.trojan') }}</option>
                  <option value="shadowsocks">{{ t('networkPages.nodes.protocols.shadowsocks') }}</option>
                  <option value="hysteria2">{{ t('networkPages.nodes.protocols.hysteria2') }}</option>
                  <option value="tuic">{{ t('networkPages.nodes.protocols.tuic') }}</option>
                  <option value="wireguard">WireGuard</option>
                </select>
              </div>
              <div class="form-group">
                <label>{{ t('admin.nodes.protocolForm.fields.port') }}</label>
                <input v-model.number="protocolForm.port" type="number" :placeholder="t('admin.nodes.protocolForm.placeholders.port')" />
              </div>
            </div>

            <div class="form-row">
              <div class="form-group">
                <label>{{ t('admin.nodes.protocolForm.fields.tls') }}</label>
                <select v-model.number="protocolForm.tls">
                  <option :value="0">{{ t('admin.nodes.tlsModes.none') }}</option>
                  <option :value="1">{{ t('admin.nodes.tlsModes.standard') }}</option>
                  <option :value="2">{{ t('admin.nodes.tlsModes.reality') }}</option>
                </select>
              </div>
              <div class="form-group">
                <label>{{ t('admin.nodes.protocolForm.fields.transport') }}</label>
                <select v-model="protocolForm.transport">
                  <option value="tcp">{{ t('admin.nodes.transports.tcp') }}</option>
                  <option value="ws">{{ t('admin.nodes.transports.ws') }}</option>
                  <option value="grpc">{{ t('admin.nodes.transports.grpc') }}</option>
                  <option value="quic">{{ t('admin.nodes.transports.quic') }}</option>
                  <option value="h2">{{ t('admin.nodes.transports.h2') }}</option>
                </select>
              </div>
            </div>

            <div class="form-row">
              <div class="form-group">
                <label class="checkbox-label">
                  <input type="checkbox" v-model="protocolForm.enable" :true-value="1" :false-value="0" />
                  <span>{{ t('admin.nodes.protocolForm.enable') }}</span>
                </label>
              </div>
              <div class="form-group">
                <label class="checkbox-label">
                  <input type="checkbox" v-model="protocolForm.show" :true-value="1" :false-value="0" />
                  <span>{{ t('admin.nodes.protocolForm.show') }}</span>
                </label>
              </div>
            </div>

            <div v-if="protocolForm.type === 'wireguard'" class="wireguard-editor">
              <div class="section-title">{{ t('admin.nodes.protocolForm.wireguard.sections.access') }}</div>
              <div class="form-row">
                <div class="form-group">
                  <label>{{ t('admin.nodes.protocolForm.wireguard.fields.cidr') }}</label>
                  <input v-model.trim="wireGuardForm.cidr" type="text" />
                </div>
                <div class="form-group" v-if="wireGuardForm.role !== 'exit'">
                  <label>{{ t('admin.nodes.protocolForm.wireguard.fields.serverAddress') }}</label>
                  <input v-model.trim="wireGuardForm.serverAddress" type="text" />
                </div>
              </div>
              <div class="form-row" v-if="wireGuardForm.role !== 'exit'">
                <div class="form-group">
                  <label class="field-label-row">
                    <span>{{ t('admin.nodes.protocolForm.wireguard.fields.serverPrivateKey') }}</span>
                    <button type="button" class="btn btn-secondary btn-sm" @click="createWireGuardKeypair">
                      {{ t('admin.nodes.protocolForm.wireguard.actions.generateKeypair') }}
                    </button>
                  </label>
                  <input v-model.trim="wireGuardForm.serverPrivateKey" type="password" autocomplete="off" />
                </div>
                <div class="form-group">
                  <label>{{ t('admin.nodes.protocolForm.wireguard.fields.serverPublicKey') }}</label>
                  <input v-model.trim="wireGuardForm.serverPublicKey" type="text" />
                </div>
              </div>
              <div class="form-row">
                <div class="form-group">
                  <label>{{ t('admin.nodes.protocolForm.wireguard.fields.mtu') }}</label>
                  <input v-model.number="wireGuardForm.mtu" type="number" min="576" max="1500" />
                </div>
                <div class="form-group" v-if="wireGuardForm.role !== 'exit'">
                  <label>{{ t('admin.nodes.protocolForm.wireguard.fields.dns') }}</label>
                  <input v-model.trim="wireGuardForm.dns" type="text" />
                </div>
              </div>
              <div class="form-group" v-if="wireGuardForm.role !== 'exit'">
                <label>{{ t('admin.nodes.protocolForm.wireguard.fields.allowedIps') }}</label>
                <input v-model.trim="wireGuardForm.allowedIps" type="text" />
              </div>

              <div class="section-title">{{ t('admin.nodes.protocolForm.wireguard.sections.relay') }}</div>
              <div class="form-row">
                <div class="form-group">
                  <label>{{ t('admin.nodes.protocolForm.wireguard.fields.role') }}</label>
                  <select v-model="wireGuardForm.role">
                    <option value="entry">{{ t('admin.nodes.protocolForm.wireguard.values.entry') }}</option>
                    <option value="exit">{{ t('admin.nodes.protocolForm.wireguard.values.exit') }}</option>
                  </select>
                </div>
                <div class="form-group">
                  <label>{{ t('admin.nodes.protocolForm.wireguard.fields.tunnelType') }}</label>
                  <select v-model="wireGuardForm.tunnelType">
                    <option value="quic">GOST relay+QUIC</option>
                    <option value="wss">GOST relay+WSS</option>
                  </select>
                </div>
              </div>
              <div class="form-group">
                <label class="checkbox-label">
                  <input v-model="wireGuardForm.wssCompat" type="checkbox" />
                  <span>{{ t('admin.nodes.protocolForm.wireguard.fields.wssCompat') }}</span>
                </label>
                <p class="field-hint">{{ t('admin.nodes.protocolForm.wireguard.hints.wssCompat') }}</p>
              </div>
              <template v-if="wireGuardForm.wssCompat || wireGuardForm.tunnelType === 'wss'">
                <div class="form-row">
                  <div class="form-group">
                    <label>{{ t('admin.nodes.protocolForm.wireguard.fields.wssPath') }}</label>
                    <input v-model.trim="wireGuardForm.wssPath" type="text" />
                  </div>
                  <div class="form-group" v-if="wireGuardForm.role === 'entry'">
                    <label>{{ t('admin.nodes.protocolForm.wireguard.fields.wssServerName') }}</label>
                    <input v-model.trim="wireGuardForm.wssServerName" type="text" autocomplete="off" />
                  </div>
                </div>
                <div class="form-row" v-if="wireGuardForm.role === 'entry'">
                  <div class="form-group">
                    <label>{{ t('admin.nodes.protocolForm.wireguard.fields.wssCaFile') }}</label>
                    <input v-model.trim="wireGuardForm.wssCaFile" type="text" autocomplete="off" />
                  </div>
                  <div class="form-group">
                    <label class="checkbox-label">
                      <input v-model="wireGuardForm.wssSecure" type="checkbox" disabled />
                      <span>{{ t('admin.nodes.protocolForm.wireguard.fields.wssSecure') }}</span>
                    </label>
                  </div>
                </div>
                <div class="form-row" v-else>
                  <div class="form-group">
                    <label>{{ t('admin.nodes.protocolForm.wireguard.fields.wssCertFile') }}</label>
                    <input v-model.trim="wireGuardForm.wssCertFile" type="text" autocomplete="off" />
                  </div>
                  <div class="form-group">
                    <label>{{ t('admin.nodes.protocolForm.wireguard.fields.wssKeyFile') }}</label>
                    <input v-model.trim="wireGuardForm.wssKeyFile" type="text" autocomplete="off" />
                  </div>
                </div>
              </template>
              <div class="form-row">
                <div class="form-group">
                  <label>{{ t('admin.nodes.protocolForm.wireguard.fields.relayServer') }}</label>
                  <input v-model.trim="wireGuardForm.relayServer" type="text" />
                </div>
                <div class="form-group">
                  <label>{{ t('admin.nodes.protocolForm.wireguard.fields.relayServerPort') }}</label>
                  <input v-model.number="wireGuardForm.relayServerPort" type="number" min="0" max="65535" />
                </div>
              </div>
              <div class="form-row">
                <div class="form-group">
                  <label>{{ t('admin.nodes.protocolForm.wireguard.fields.tunPort') }}</label>
                  <input v-model.number="wireGuardForm.tunPort" type="number" min="1" max="65535" />
                </div>
                <div class="form-group">
                  <label>{{ t('admin.nodes.protocolForm.wireguard.fields.tunName') }}</label>
                  <input v-model.trim="wireGuardForm.tunName" type="text" />
                </div>
              </div>
              <div class="form-row">
                <div class="form-group">
                  <label>{{ t('admin.nodes.protocolForm.wireguard.fields.entryTunAddress') }}</label>
                  <input v-model.trim="wireGuardForm.entryTunAddress" type="text" />
                </div>
                <div class="form-group">
                  <label>{{ t('admin.nodes.protocolForm.wireguard.fields.exitTunAddress') }}</label>
                  <input v-model.trim="wireGuardForm.exitTunAddress" type="text" />
                </div>
              </div>
              <div class="form-row">
                <div class="form-group">
                  <label>{{ t('admin.nodes.protocolForm.wireguard.fields.outboundIface') }}</label>
                  <input v-model.trim="wireGuardForm.outboundIface" type="text" />
                </div>
                <div class="form-group">
                  <label class="checkbox-label">
                    <input v-model="wireGuardForm.exitNat" type="checkbox" />
                    <span>{{ t('admin.nodes.protocolForm.wireguard.fields.exitNat') }}</span>
                  </label>
                </div>
              </div>
              <div class="form-row">
                <div class="form-group">
                  <label>{{ t('admin.nodes.protocolForm.wireguard.fields.routingTable') }}</label>
                  <input v-model.number="wireGuardForm.routingTable" type="number" min="0" />
                </div>
                <div class="form-group">
                  <label>{{ t('admin.nodes.protocolForm.wireguard.fields.routingPriority') }}</label>
                  <input v-model.number="wireGuardForm.routingPriority" type="number" min="0" />
                </div>
              </div>
              <template v-if="wireGuardForm.role === 'entry' && (wireGuardForm.wssCompat || wireGuardForm.tunnelType === 'wss')">
                <div class="section-title network-policy-title">
                  <span>{{ t('admin.nodes.protocolForm.wireguard.sections.networkPolicy') }}</span>
                  <label class="checkbox-label">
                    <input v-model="wireGuardForm.networkPolicyEnabled" type="checkbox" />
                    <span>{{ t('admin.nodes.protocolForm.wireguard.fields.networkPolicyEnabled') }}</span>
                  </label>
                </div>
                <p class="field-hint">{{ t('admin.nodes.protocolForm.wireguard.hints.networkPolicy') }}</p>
                <template v-if="wireGuardForm.networkPolicyEnabled">
                  <div v-for="(path, index) in wireGuardForm.networkPaths" :key="index" class="network-path-card">
                    <div class="network-path-header">
                      <strong>{{ t('admin.nodes.protocolForm.wireguard.fields.networkPath') }} {{ index + 1 }}</strong>
                      <button type="button" class="btn btn-secondary btn-sm" @click="removeWireGuardNetworkPath(index)">
                        {{ t('common.delete') }}
                      </button>
                    </div>
                    <div class="form-row">
                      <div class="form-group">
                        <label>{{ t('admin.nodes.protocolForm.wireguard.fields.pathName') }}</label>
                        <input v-model.trim="path.name" type="text" placeholder="CN2" />
                      </div>
                      <div class="form-group">
                        <label>{{ t('admin.nodes.protocolForm.wireguard.fields.pathInterface') }}</label>
                        <input v-model.trim="path.interface" type="text" placeholder="eth1" />
                      </div>
                    </div>
                    <div class="form-row">
                      <div class="form-group">
                        <label>{{ t('admin.nodes.protocolForm.wireguard.fields.pathSource') }}</label>
                        <input v-model.trim="path.source" type="text" placeholder="10.8.0.112" />
                      </div>
                      <div class="form-group">
                        <label>{{ t('admin.nodes.protocolForm.wireguard.fields.pathGateway') }}</label>
                        <input v-model.trim="path.gateway" type="text" placeholder="10.8.0.1" />
                      </div>
                      <div class="form-group">
                        <label>{{ t('admin.nodes.protocolForm.wireguard.fields.pathPriority') }}</label>
                        <input v-model.number="path.priority" type="number" min="0" />
                      </div>
                    </div>
                  </div>
                  <button type="button" class="btn btn-secondary btn-sm" @click="addWireGuardNetworkPath">
                    {{ t('admin.nodes.protocolForm.wireguard.actions.addNetworkPath') }}
                  </button>
                  <div class="form-row network-health-row">
                    <div class="form-group">
                      <label>{{ t('admin.nodes.protocolForm.wireguard.fields.healthInterval') }}</label>
                      <input v-model.number="wireGuardForm.healthInterval" type="number" min="1" />
                    </div>
                    <div class="form-group">
                      <label>{{ t('admin.nodes.protocolForm.wireguard.fields.healthTimeout') }}</label>
                      <input v-model.number="wireGuardForm.healthTimeout" type="number" min="1" />
                    </div>
                    <div class="form-group">
                      <label>{{ t('admin.nodes.protocolForm.wireguard.fields.failureThreshold') }}</label>
                      <input v-model.number="wireGuardForm.failureThreshold" type="number" min="1" />
                    </div>
                    <div class="form-group">
                      <label>{{ t('admin.nodes.protocolForm.wireguard.fields.failbackDelay') }}</label>
                      <input v-model.number="wireGuardForm.failbackDelay" type="number" min="0" />
                    </div>
                  </div>
                </template>
              </template>
            </div>

            <!-- JSON sub-editors for advanced fields -->
            <details class="advanced-details" v-if="protocolForm.type !== 'wireguard'">
              <summary>{{ t('admin.nodes.protocolForm.fields.settings') }}</summary>
              <textarea
                v-model="protocolForm.settings"
                class="json-textarea"
                rows="4"
                spellcheck="false"
                placeholder='{}'
              ></textarea>
            </details>

            <details class="advanced-details" v-if="protocolForm.tls > 0">
              <summary>{{ t('admin.nodes.protocolForm.fields.tlsSettings') }}</summary>
              <textarea
                v-model="protocolForm.tls_settings"
                class="json-textarea"
                rows="4"
                spellcheck="false"
                placeholder='{}'
              ></textarea>
            </details>

            <details class="advanced-details" v-if="protocolForm.tls === 2">
              <summary>{{ t('admin.nodes.protocolForm.fields.realitySettings') }}</summary>
              <textarea
                v-model="protocolForm.reality_settings"
                class="json-textarea"
                rows="4"
                spellcheck="false"
                placeholder='{}'
              ></textarea>
            </details>

            <details class="advanced-details" v-if="protocolForm.transport !== 'tcp'">
              <summary>{{ t('admin.nodes.protocolForm.fields.transportSettings') }}</summary>
              <textarea
                v-model="protocolForm.transport_settings"
                class="json-textarea"
                rows="4"
                spellcheck="false"
                placeholder='{}'
              ></textarea>
            </details>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary" @click="closeProtocolFormModal">{{ t('admin.nodes.actions.cancel') }}</button>
          <button class="btn btn-primary" @click="saveProtocol" :disabled="savingProtocol">
            {{ savingProtocol ? t('admin.nodes.actions.saving') : t('admin.nodes.actions.save') }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, computed, watch, onMounted } from 'vue'
import {
  getNodes, getNodeStats, getNodeLogs, createNode, updateNode, deleteNode,
  getNodeCredentials,
  syncNodeProtocol,
  getNodeProtocols, createNodeProtocol, updateNodeProtocol, deleteNodeProtocol,
  getProtocolTemplates, generateWireGuardKeypair,
  getAuthKeys
} from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import { AGENT_NAME } from '@/constants/brand'

const { t, formatDateTime } = useAppI18n()

// State
const loading = ref(false)
const saving = ref(false)
const savingProtocol = ref(false)
const syncingNodeIds = reactive(new Set())

const nodes = ref([])
const stats = reactive({ total: 0, online: 0, offline: 0, pending: 0 })
const pagination = reactive({ page: 1, size: 20, total: 0 })

const showNodeModal = ref(false)
const editingNode = ref(null)
const nodeForm = reactive({
  name: '',
  address: '',
  tags: '',
  rate: 1.0,
  sort: 0,
  status: 0,
  parent_id: null,
  monthly_limit_gb: null,
  monthly_reset_day: 1
})

const BYTES_PER_GB = 1024 * 1024 * 1024

// 可选父节点: 排除自己 (编辑时) 及其所有下级(编辑时), 避免手工配置出环
const parentCandidates = computed(() => {
  if (!editingNode.value) return nodes.value
  const excluded = new Set([editingNode.value.id])
  let changed = true
  while (changed) {
    changed = false
    for (const n of nodes.value) {
      if (n.parent_id && excluded.has(n.parent_id) && !excluded.has(n.id)) {
        excluded.add(n.id)
        changed = true
      }
    }
  }
  return nodes.value.filter((n) => !excluded.has(n.id))
})

const getNodeName = (parentId) => {
  if (!parentId) return ''
  const parent = nodes.value.find((n) => n.id === parentId)
  return parent ? parent.name : ''
}

const isOverQuota = (node) => {
  if (!node.monthly_limit) return false
  return (node.monthly_upload || 0) + (node.monthly_download || 0) > node.monthly_limit
}

const overQuotaNodes = computed(() => nodes.value.filter(isOverQuota))
const parentNodes = computed(() => nodes.value.filter((node) => !node.parent_id))

const showAuthKeyModal = ref(false)
const authKey = ref('')
const authKeyUsed = ref(0)
const showDeployModal = ref(false)
const deployLoading = ref(false)
const deployError = ref('')
const deployRows = ref([])
const deploySettings = reactive({
  panelApiHost: typeof window !== 'undefined' ? window.location.origin : 'http://127.0.0.1:18080',
  grpcHost: typeof window !== 'undefined'
    ? `${window.location.hostname}:${window.location.protocol === 'https:' ? '443' : '50051'}`
    : '127.0.0.1:50051',
  grpcUseTLS: typeof window !== 'undefined' ? window.location.protocol === 'https:' : false,
  grpcServerName: typeof window !== 'undefined' ? window.location.hostname : '127.0.0.1',
  amd64BinaryPath: '/home/dev/anixops/anix-agent/build/inventory/anix-agent_linux_amd64',
  arm64BinaryPath: '/home/dev/anixops/anix-agent/build/inventory/anix-agent_linux_arm64',
  coreType: 'xray',
  pluginSupervisorEnabled: false,
  pluginRoot: '/var/lib/anixops/plugins',
  pluginSocketDir: '/run/anixops/plugins',
  pluginOfficialPublicKey: ''
})

function isLoopbackGRPCHost(value) {
  const endpoint = String(value || '').trim().toLowerCase()
  if (!endpoint || endpoint.includes('://') || endpoint.includes('/') || endpoint.includes('?') || endpoint.includes('#')) {
    return false
  }

  let host = endpoint
  if (endpoint.startsWith('[')) {
    const closingBracket = endpoint.indexOf(']')
    if (closingBracket <= 1 || !/^\](?::\d{1,5})?$/.test(endpoint.slice(closingBracket))) {
      return false
    }
    host = endpoint.slice(1, closingBracket)
  } else {
    const firstColon = endpoint.indexOf(':')
    const lastColon = endpoint.lastIndexOf(':')
    if (firstColon === lastColon && firstColon > 0) {
      const port = endpoint.slice(lastColon + 1)
      if (!/^\d{1,5}$/.test(port)) return false
      host = endpoint.slice(0, lastColon)
    }
  }

  if (host === 'localhost' || host === '::1') return true
  const octets = host.split('.')
  return octets.length === 4 && octets.every((octet) => /^\d{1,3}$/.test(octet) && Number(octet) >= 0 && Number(octet) <= 255) && Number(octets[0]) === 127
}

function isEd25519PublicKey(value) {
  const encoded = String(value || '').trim()
  if (!encoded || !/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/.test(encoded) || typeof globalThis.atob !== 'function') return false
  try {
    return globalThis.atob(encoded).length === 32
  } catch {
    return false
  }
}

const agentControlEnabled = computed(() => deploySettings.grpcUseTLS || isLoopbackGRPCHost(deploySettings.grpcHost))

const pluginSupervisorCanaryReady = computed(() => {
  if (!deploySettings.pluginSupervisorEnabled) return true
  return agentControlEnabled.value && [
    deploySettings.pluginRoot,
    deploySettings.pluginSocketDir
  ].every((value) => String(value || '').trim() !== '') && isEd25519PublicKey(deploySettings.pluginOfficialPublicKey)
})

const pluginSupervisorCanaryError = computed(() => {
  if (!agentControlEnabled.value) {
    return t('admin.nodes.deployModal.pluginSupervisorControlRequired')
  }
  if (String(deploySettings.pluginOfficialPublicKey || '').trim() && !isEd25519PublicKey(deploySettings.pluginOfficialPublicKey)) {
    return t('admin.nodes.deployModal.pluginSupervisorKeyInvalid')
  }
  return t('admin.nodes.deployModal.pluginSupervisorKeyRequired')
})

const showLogModal = ref(false)
const logNode = ref(null)
const logLoading = ref(false)
const nodeLogs = ref([])
const logLevels = ['debug', 'info', 'warning', 'error']
const logFilter = reactive({
  level: '',
  source: '',
  search: ''
})
const logPagination = reactive({
  page: 1,
  size: 20,
  total: 0
})

const showProtocolModal = ref(false)
const selectedNode = ref(null)
const protocols = ref([])

const showProtocolFormModal = ref(false)
const editingProtocol = ref(null)
const protocolForm = reactive({
  mode: 'json', // json | visual
  type: 'vless',
  port: 443,
  enable: 1,
  tls: 0,
  transport: 'tcp',
  settings: '{}',
  tls_settings: '{}',
  transport_settings: '{}',
  reality_settings: '{}',
  custom_config: '',
  show: 1
})

const defaultWireGuardForm = () => ({
  cidr: '10.66.0.0/24',
  serverAddress: '10.66.0.1/24',
  serverPrivateKey: '',
  serverPublicKey: '',
  mtu: 1280,
  dns: '1.1.1.1,8.8.8.8',
  allowedIps: '0.0.0.0/0',
  tunnelType: 'quic',
  role: 'entry',
  wssCompat: false,
  wssPath: '/ws',
  wssSecure: true,
  wssServerName: '',
  wssCaFile: '',
  wssCertFile: '',
  wssKeyFile: '',
  relayServer: '',
  relayServerPort: 0,
  tunPort: 8421,
  tunName: '',
  entryTunAddress: '172.31.66.2/24',
  exitTunAddress: '172.31.66.1/24',
  outboundIface: '',
  exitNat: true,
  routingTable: 0,
  routingPriority: 0,
  networkPolicyEnabled: false,
  networkPaths: [],
  healthInterval: 10,
  healthTimeout: 3,
  failureThreshold: 3,
  recoveryThreshold: 2,
  failbackDelay: 300
})
const wireGuardForm = reactive(defaultWireGuardForm())
const protocolTemplates = ref([])
const selectedTemplate = ref('')

// JSON editor state
const jsonEditorContent = ref('')
const jsonParseError = ref('')
const jsonValid = computed(() => jsonParseError.value === '')

const jsonPlaceholder = `{
  "type": "vless",
  "port": 443,
  "tls": 0,
  "transport": "tcp",
  "enable": 1,
  "show": 1,
  "settings": {},
  "tls_settings": {},
  "transport_settings": {},
  "reality_settings": {}
}`

const splitList = (value) => String(value || '')
  .split(',')
  .map(item => item.trim())
  .filter(Boolean)

const asListText = (value, fallback) => {
  if (Array.isArray(value)) return value.join(',')
  if (typeof value === 'string' && value.trim()) return value
  return fallback
}

const asNumber = (value, fallback) => {
  const n = Number(value)
  return Number.isFinite(n) ? n : fallback
}

const resetWireGuardForm = () => {
  Object.assign(wireGuardForm, defaultWireGuardForm())
}

const addWireGuardNetworkPath = () => {
  wireGuardForm.networkPaths.push({ name: '', interface: '', source: '', gateway: '', priority: 100 })
}

const removeWireGuardNetworkPath = (index) => {
  wireGuardForm.networkPaths.splice(index, 1)
}

const readNodeApiError = (error) => {
  const response = error?.response?.data
  return response?.error || response?.message || error?.message || String(error)
}

const createWireGuardKeypair = async () => {
  try {
    const response = await generateWireGuardKeypair()
    const payload = response?.data?.data || response?.data || response
    wireGuardForm.serverPrivateKey = payload?.private_key || ''
    wireGuardForm.serverPublicKey = payload?.public_key || ''
  } catch (error) {
    alert(t('admin.nodes.messages.generateFailed', { message: readNodeApiError(error) }))
  }
}

const buildWireGuardSettings = () => {
  const tunnelType = wireGuardForm.wssCompat ? 'wss' : (wireGuardForm.tunnelType || 'quic')
  const relayMode = tunnelType === 'wss' ? 'relay+wss' : 'relay+quic'
  const isExit = wireGuardForm.role === 'exit'
  return {
    cidr: wireGuardForm.cidr || '10.66.0.0/24',
    server_address: isExit ? '' : (wireGuardForm.serverAddress || '10.66.0.1/24'),
    server_private_key: isExit ? '' : (wireGuardForm.serverPrivateKey || ''),
    server_public_key: isExit ? '' : (wireGuardForm.serverPublicKey || ''),
    mtu: asNumber(wireGuardForm.mtu, 1280),
    dns: isExit ? [] : splitList(wireGuardForm.dns),
    allowed_ips: isExit ? [] : splitList(wireGuardForm.allowedIps),
    tunnel_type: tunnelType,
    relay: {
      backend: 'gost',
      mode: relayMode,
      role: wireGuardForm.role === 'exit' ? 'exit' : 'entry',
      wss_compat: tunnelType === 'wss',
      wss_path: wireGuardForm.wssPath || '/ws',
      wss_secure: !isExit && tunnelType === 'wss',
      wss_server_name: isExit ? '' : (wireGuardForm.wssServerName || ''),
      wss_ca_file: isExit ? '' : (wireGuardForm.wssCaFile || ''),
      wss_cert_file: isExit ? (wireGuardForm.wssCertFile || '') : '',
      wss_key_file: isExit ? (wireGuardForm.wssKeyFile || '') : '',
      exit_nat: Boolean(wireGuardForm.exitNat),
      entry_stats: true,
      server: wireGuardForm.relayServer || '',
      server_port: asNumber(wireGuardForm.relayServerPort, 0),
      tun_port: asNumber(wireGuardForm.tunPort, 8421),
      tun_name: wireGuardForm.tunName || '',
      entry_tun_address: wireGuardForm.entryTunAddress || '172.31.66.2/24',
      exit_tun_address: wireGuardForm.exitTunAddress || '172.31.66.1/24',
      outbound_iface: wireGuardForm.outboundIface || '',
      routing_table: asNumber(wireGuardForm.routingTable, 0),
      routing_priority: asNumber(wireGuardForm.routingPriority, 0),
      ...(wireGuardForm.networkPolicyEnabled && tunnelType === 'wss' ? {
        network_policy: {
          version: 1,
          strategy: 'failover',
          paths: wireGuardForm.networkPaths.map(path => ({
            name: path.name || '',
            interface: path.interface || '',
            source: path.source || '',
            gateway: path.gateway || '',
            priority: asNumber(path.priority, 100),
            routing_table: asNumber(path.routing_table, 0),
            rule_priority: asNumber(path.rule_priority, 0)
          })),
          health_check: {
            interval_seconds: asNumber(wireGuardForm.healthInterval, 10),
            timeout_seconds: asNumber(wireGuardForm.healthTimeout, 3),
            failure_threshold: asNumber(wireGuardForm.failureThreshold, 3),
            recovery_threshold: asNumber(wireGuardForm.recoveryThreshold, 2),
            failback_delay_seconds: asNumber(wireGuardForm.failbackDelay, 300)
          }
        }
      } : {})
    }
  }
}

const hydrateWireGuardForm = (settings = {}) => {
  const relay = settings.relay && typeof settings.relay === 'object' ? settings.relay : {}
  const networkPolicy = relay.network_policy && typeof relay.network_policy === 'object' ? relay.network_policy : {}
  const healthCheck = networkPolicy.health_check && typeof networkPolicy.health_check === 'object' ? networkPolicy.health_check : {}
  const tunnelType = relay.wss_compat || settings.tunnel_type === 'wss' || String(relay.mode || '').includes('wss') ? 'wss' : 'quic'
  Object.assign(wireGuardForm, {
    cidr: settings.cidr || '10.66.0.0/24',
    serverAddress: settings.server_address || '10.66.0.1/24',
    serverPrivateKey: settings.server_private_key || '',
    serverPublicKey: settings.server_public_key || '',
    mtu: asNumber(settings.mtu, 1280),
    dns: asListText(settings.dns, '1.1.1.1,8.8.8.8'),
    allowedIps: asListText(settings.allowed_ips, '0.0.0.0/0'),
    tunnelType,
    role: relay.role === 'exit' ? 'exit' : 'entry',
    wssCompat: tunnelType === 'wss',
    wssPath: relay.wss_path || '/ws',
    wssSecure: relay.wss_secure !== false,
    wssServerName: relay.wss_server_name || '',
    wssCaFile: relay.wss_ca_file || '',
    wssCertFile: relay.wss_cert_file || '',
    wssKeyFile: relay.wss_key_file || '',
    relayServer: relay.server || '',
    relayServerPort: asNumber(relay.server_port, 0),
    tunPort: asNumber(relay.tun_port, 8421),
    tunName: relay.tun_name || '',
    entryTunAddress: relay.entry_tun_address || '172.31.66.2/24',
    exitTunAddress: relay.exit_tun_address || '172.31.66.1/24',
    outboundIface: relay.outbound_iface || '',
    exitNat: relay.exit_nat !== false,
    routingTable: asNumber(relay.routing_table, 0),
    routingPriority: asNumber(relay.routing_priority, 0),
    networkPolicyEnabled: Array.isArray(networkPolicy.paths) && networkPolicy.paths.length > 0,
    networkPaths: Array.isArray(networkPolicy.paths) ? networkPolicy.paths.map(path => ({
      name: path.name || '', interface: path.interface || '', source: path.source || '', gateway: path.gateway || '',
      priority: asNumber(path.priority, 100), routing_table: asNumber(path.routing_table, 0), rule_priority: asNumber(path.rule_priority, 0)
    })) : [],
    healthInterval: asNumber(healthCheck.interval_seconds, 10),
    healthTimeout: asNumber(healthCheck.timeout_seconds, 3),
    failureThreshold: asNumber(healthCheck.failure_threshold, 3),
    recoveryThreshold: asNumber(healthCheck.recovery_threshold, 2),
    failbackDelay: asNumber(healthCheck.failback_delay_seconds, 300)
  })
}

// Convert visual form to JSON object
function visualToJson() {
  const obj = {
    type: protocolForm.type,
    port: protocolForm.port,
    tls: protocolForm.tls,
    transport: protocolForm.transport,
    enable: protocolForm.enable,
    show: protocolForm.show,
  }
  if (protocolForm.type === 'wireguard') {
    obj.port = protocolForm.port || 51820
    obj.tls = 0
    obj.transport = 'udp'
    obj.settings = buildWireGuardSettings()
    if (obj.settings.relay.role === 'exit') obj.show = 0
    obj.tls_settings = {}
    obj.transport_settings = {}
    obj.reality_settings = {}
    return obj
  }
  try { obj.settings = JSON.parse(protocolForm.settings || '{}') } catch { obj.settings = {} }
  try { obj.tls_settings = JSON.parse(protocolForm.tls_settings || '{}') } catch { obj.tls_settings = {} }
  try { obj.transport_settings = JSON.parse(protocolForm.transport_settings || '{}') } catch { obj.transport_settings = {} }
  try { obj.reality_settings = JSON.parse(protocolForm.reality_settings || '{}') } catch { obj.reality_settings = {} }
  return obj
}

// Convert JSON object to visual form fields
function jsonToVisual(json) {
  protocolForm.type = json.type || 'vless'
  protocolForm.port = json.port || 443
  protocolForm.tls = json.tls ?? 0
  protocolForm.transport = json.transport || 'tcp'
  protocolForm.enable = json.enable ?? 1
  protocolForm.show = json.show ?? 1
  protocolForm.settings = json.settings ? (typeof json.settings === 'string' ? json.settings : JSON.stringify(json.settings, null, 2)) : '{}'
  protocolForm.tls_settings = json.tls_settings ? (typeof json.tls_settings === 'string' ? json.tls_settings : JSON.stringify(json.tls_settings, null, 2)) : '{}'
  protocolForm.transport_settings = json.transport_settings ? (typeof json.transport_settings === 'string' ? json.transport_settings : JSON.stringify(json.transport_settings, null, 2)) : '{}'
  protocolForm.reality_settings = json.reality_settings ? (typeof json.reality_settings === 'string' ? json.reality_settings : JSON.stringify(json.reality_settings, null, 2)) : '{}'
  if (protocolForm.type === 'wireguard') {
    let settings = json.settings || {}
    if (typeof settings === 'string') {
      try { settings = JSON.parse(settings) } catch { settings = {} }
    }
    hydrateWireGuardForm(settings)
  }
}

// When switching to JSON mode, sync from visual form
watch(() => protocolForm.mode, (newMode) => {
  if (newMode === 'json' && !editingProtocol.value) {
    const json = visualToJson()
    jsonEditorContent.value = JSON.stringify(json, null, 2)
    jsonParseError.value = ''
  }
})

// When switching to visual mode, sync from JSON
watch(() => protocolForm.mode, (newMode) => {
  if (newMode === 'visual') {
    try {
      const json = JSON.parse(jsonEditorContent.value)
      jsonToVisual(json)
    } catch {
      // keep existing visual values if JSON is invalid
    }
  }
})

watch(() => protocolForm.type, (newType, oldType) => {
  if (newType === 'wireguard' && oldType !== 'wireguard') {
    protocolForm.port = protocolForm.port === 443 ? 51820 : protocolForm.port
    protocolForm.tls = 0
    protocolForm.transport = 'udp'
    resetWireGuardForm()
  }
})

watch(() => wireGuardForm.wssCompat, (enabled) => {
  wireGuardForm.tunnelType = enabled ? 'wss' : 'quic'
})

watch(() => wireGuardForm.tunnelType, (value) => {
  wireGuardForm.wssCompat = value === 'wss'
})

watch(() => wireGuardForm.role, (role) => {
  if (role === 'exit') protocolForm.show = 0
})

function onJsonInput() {
  try {
    JSON.parse(jsonEditorContent.value)
    jsonParseError.value = ''
  } catch (e) {
    jsonParseError.value = e.message
  }
}

function formatJson() {
  try {
    const parsed = JSON.parse(jsonEditorContent.value)
    jsonEditorContent.value = JSON.stringify(parsed, null, 2)
    jsonParseError.value = ''
  } catch (e) {
    jsonParseError.value = e.message
  }
}

async function copyJson() {
  try {
    await navigator.clipboard.writeText(jsonEditorContent.value)
  } catch (e) {
    alert(t('admin.nodes.messages.copyFailed') + ': ' + (e.message || e))
  }
}

function loadTemplateAsJson() {
  if (protocolTemplates.value.length === 0) return
  // Show a simple prompt to pick template
  const names = protocolTemplates.value.map((tpl, i) => `${i + 1}. ${tpl.name}`).join('\n')
  const pick = prompt(`Select template number:\n${names}`)
  const idx = parseInt(pick) - 1
  if (isNaN(idx) || idx < 0 || idx >= protocolTemplates.value.length) return
  const tpl = protocolTemplates.value[idx]
  const json = {
    type: tpl.type,
    port: tpl.default_port,
    tls: tpl.tls || 0,
    transport: tpl.transport || 'tcp',
    enable: 1,
    show: 1,
  }
  try { json.settings = JSON.parse(tpl.settings || '{}') } catch { json.settings = {} }
  try { json.tls_settings = JSON.parse(tpl.tls_settings || '{}') } catch { json.tls_settings = {} }
  try { json.transport_settings = JSON.parse(tpl.transport_settings || '{}') } catch { json.transport_settings = {} }
  try { json.reality_settings = JSON.parse(tpl.reality_settings || '{}') } catch { json.reality_settings = {} }
  jsonEditorContent.value = JSON.stringify(json, null, 2)
  jsonParseError.value = ''
}

// Data loaders
const normalizeNode = (node) => ({
  ...node,
  address: node.address || node.host || '',
  traffic_today: node.traffic_today || (Number(node.total_upload || 0) + Number(node.total_download || 0))
})

const loadNodes = async () => {
  loading.value = true
  try {
    const res = await getNodes({ page: pagination.page, page_size: pagination.size })
    const payload = readNodePage(res)
    nodes.value = (payload.list || []).map(normalizeNode)
    pagination.total = payload.total || 0
    return true
  } catch (e) {
    console.error('Failed to load nodes:', e)
    return false
  } finally {
    loading.value = false
  }
}

const loadStats = async () => {
  try {
    const res = await getNodeStats()
    Object.assign(stats, readNodeStats(res))
  } catch (e) {
    console.error('Failed to load stats:', e)
  }
}

const readNodeStats = (res) => {
  const payload = readNodePayload(res)
  return payload && typeof payload === 'object' ? payload : {}
}

const readNodePayload = (res) => {
  if (!res || typeof res !== 'object') {
    return null
  }
  if (Object.prototype.hasOwnProperty.call(res, 'code')) {
    return res.data ?? null
  }
  if (
    res.data &&
    typeof res.data === 'object' &&
    Object.prototype.hasOwnProperty.call(res.data, 'data')
  ) {
    return res.data.data ?? null
  }
  return res.data ?? res
}

const readNodeList = (res) => {
  const payload = readNodePayload(res)
  if (Array.isArray(payload)) {
    return payload
  }
  if (payload && Array.isArray(payload.list)) {
    return payload.list
  }
  return []
}

const readNodePage = (res) => {
  const payload = readNodePayload(res)
  return payload && typeof payload === 'object' ? payload : {}
}

const loadProtocolTemplates = async () => {
  try {
    const res = await getProtocolTemplates()
    protocolTemplates.value = readNodeList(res)
  } catch (e) {
    console.error('Failed to load templates:', e)
  }
}

const loadAuthKeysPreview = async () => {
  try {
    const res = await getAuthKeys()
    const keys = readNodeList(res)
    if (keys.length > 0) {
      authKey.value = keys[0].key || ''
      authKeyUsed.value = keys[0].used || 0
    } else {
      authKey.value = ''
      authKeyUsed.value = 0
    }
  } catch (e) {
    console.error('Failed to preload auth keys:', e)
    authKey.value = ''
    authKeyUsed.value = 0
  }
}

const slugifyDeployAlias = (name, id) => {
  const normalized = String(name || '')
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
  return normalized || `node-${id}`
}

const buildDeployRow = (node, apiKey = '') => ({
  id: node.id,
  nodeId: node.id,
  name: node.name,
  alias: slugifyDeployAlias(node.name, node.id),
  host: node.host || node.address || '',
  sshPort: 22,
  sshUser: 'root',
  arch: 'amd64',
  authMode: 'password',
  authValue: '',
  apiKey
})

const deployInventoryPreview = computed(() => {
  const lines = [
    '[v2bx_nodes]',
    '# Generated from parent nodes (nodes without parent_id)'
  ]
  for (const row of deployRows.value) {
    const authValue = row.authValue || (row.authMode === 'password' ? '<PASSWORD>' : '~/.ssh/id_ed25519')
    const authField = row.authMode === 'password'
      ? `ansible_ssh_pass=${authValue}`
      : `ansible_ssh_private_key_file=${authValue}`
    const binaryPath = row.arch === 'arm64'
      ? (deploySettings.arm64BinaryPath || '/home/dev/anixops/anix-agent/build/inventory/anix-agent_linux_arm64')
      : (deploySettings.amd64BinaryPath || '/home/dev/anixops/anix-agent/build/inventory/anix-agent_linux_amd64')
    lines.push(
      `${row.alias} ansible_host=${row.host} ansible_port=${row.sshPort || 22} ansible_user=${row.sshUser || 'root'} ${authField} node_id=${row.nodeId} api_key=${row.apiKey || '<API_KEY>'} v2bx_arch=${row.arch} v2bx_binary_local=${binaryPath}`
    )
  }
  return lines.join('\n')
})

const deployGroupVarsPreview = computed(() => {
  const lines = [
    '---',
    `panel_api_host: "${deploySettings.panelApiHost}"`,
    `grpc_host: "${deploySettings.grpcHost}"`,
    `grpc_use_tls: ${deploySettings.grpcUseTLS ? 'true' : 'false'}`,
    `agent_control_enabled: ${agentControlEnabled.value ? 'true' : 'false'}`,
    'agent_control_allow_insecure: false',
    `plugin_supervisor_enabled: ${pluginSupervisorCanaryReady.value ? 'true' : 'false'}`
  ]
  if (deploySettings.grpcUseTLS && deploySettings.grpcServerName) {
    lines.push(`grpc_server_name: "${deploySettings.grpcServerName}"`)
  }
  if (pluginSupervisorCanaryReady.value) {
    lines.push(
      `plugin_root: ${JSON.stringify(deploySettings.pluginRoot)}`,
      `plugin_socket_dir: ${JSON.stringify(deploySettings.pluginSocketDir)}`,
      `plugin_official_public_key: ${JSON.stringify(deploySettings.pluginOfficialPublicKey)}`
    )
  }
  lines.push(
    '',
    'panel_api_base: "http://127.0.0.1:18080"',
    '',
    `v2bx_binary_amd64_local: "${deploySettings.amd64BinaryPath}"`,
    `v2bx_binary_arm64_local: "${deploySettings.arm64BinaryPath}"`,
    '',
    'push_geodata: false',
    'v2bx_geodata_dir: "/home/dev/anixops/anix-agent/example"',
    '',
    `core_type: "${deploySettings.coreType}"`,
    'v2bx_log_level: "info"',
    'listen_ip: "0.0.0.0"',
    'send_ip: "0.0.0.0"',
    'cert_mode: "none"'
  )
  return lines.join('\n')
})

const deployCommandsPreview = computed(() => {
  if (deployRows.value.length === 0) {
    return 'cd config/deploy/ansible/nodes'
  }
  const firstAlias = deployRows.value[0]?.alias || '<node-alias>'
  return [
    'cd config/deploy/ansible/nodes',
    'export ANSIBLE_CONFIG=../ansible.cfg',
    `# Download and verify matching ${AGENT_NAME} artifacts from GitHub Actions into the configured paths`,
    `# AMD64 artifact: ${deploySettings.amd64BinaryPath}`,
    `# ARM64 artifact: ${deploySettings.arm64BinaryPath}`,
    '# No local build is performed by this deployment flow',
    'ansible-playbook -i inventory.ini deploy_v2bx.yml',
    `ansible-playbook -i inventory.ini deploy_v2bx.yml -l ${firstAlias}`,
    `ansible-playbook -i inventory.ini bootstrap_ssh_key.yml -l ${firstAlias}`
  ].join('\n')
})

// Node actions
const openAuthKeyModal = async () => {
  await loadAuthKeysPreview()
  showAuthKeyModal.value = true
}

const closeAuthKeyModal = () => {
  showAuthKeyModal.value = false
}

const copyAuthKey = async () => {
  if (!authKey.value) return
  try {
    await navigator.clipboard.writeText(authKey.value)
    alert(t('admin.nodes.messages.copied'))
  } catch {
    alert(t('admin.nodes.messages.copyFailed'))
  }
}

const copyConfig = async () => {
  if (!pluginSupervisorCanaryReady.value) return
  try {
    await navigator.clipboard.writeText(configSnippet.value)
    alert(t('admin.nodes.messages.copied'))
  } catch {
    alert(t('admin.nodes.messages.copyFailed'))
  }
}

const copyDeployText = async (text) => {
  try {
    await navigator.clipboard.writeText(text)
    alert(t('admin.nodes.messages.copied'))
  } catch {
    alert(t('admin.nodes.messages.copyFailed'))
  }
}

const configSnippet = computed(() => {
  const host = String(deploySettings.panelApiHost || '').trim() || (typeof window !== 'undefined' ? window.location.origin : 'http://127.0.0.1:18080')
  const config = {
    Log: {
      Level: 'info',
      Output: ''
    },
    Cores: [
      {
        Type: deploySettings.coreType || 'xray',
        Log: {
          Level: 'info'
        }
      }
    ],
    Nodes: [
      {
        Core: deploySettings.coreType || 'xray',
        ApiHost: host,
        Transport: 'http',
        GRPCHost: deploySettings.grpcHost,
        GRPCUseTLS: Boolean(deploySettings.grpcUseTLS),
        ...(deploySettings.grpcUseTLS && deploySettings.grpcServerName
          ? { GRPCServerName: deploySettings.grpcServerName }
          : {}),
        GRPCKeepalive: 30,
        AgentControlEnabled: agentControlEnabled.value,
        AgentControlAllowInsecure: false,
        ...(deploySettings.pluginSupervisorEnabled && pluginSupervisorCanaryReady.value
          ? {
              PluginSupervisorEnabled: true,
              PluginRoot: deploySettings.pluginRoot,
              PluginSocketDir: deploySettings.pluginSocketDir,
              PluginOfficialPublicKey: deploySettings.pluginOfficialPublicKey
            }
          : {}),
        AuthKey: authKey.value || '<your-auth-key>',
        NodeID: 0,
        AutoRegister: true,
        Timeout: 30,
        ListenIP: '0.0.0.0',
        SendIP: '0.0.0.0',
        CertConfig: {
          CertMode: 'none'
        }
      }
    ]
  }
  return `# ${AGENT_NAME} config example\n${JSON.stringify(config, null, 2)}`
})

const openDeployModal = async () => {
  showDeployModal.value = true
  deployLoading.value = true
  deployError.value = ''
  try {
    const roots = parentNodes.value
    if (roots.length === 0) {
      deployRows.value = []
      return
    }
    const credentials = await Promise.allSettled(roots.map(async (node) => {
      const res = await getNodeCredentials(node.id)
      const payload = readNodePayload(res)
      return { nodeId: node.id, apiKey: payload?.api_key || '' }
    }))
    const apiKeysByNode = new Map(
      credentials
        .filter((item) => item.status === 'fulfilled')
        .map((item) => [item.value.nodeId, item.value.apiKey])
    )
    deployRows.value = roots.map((node) => buildDeployRow(node, apiKeysByNode.get(node.id) || ''))
  } catch (e) {
    console.error('Failed to load deploy credentials:', e)
    deployError.value = t('admin.nodes.messages.deployLoadFailed')
    deployRows.value = parentNodes.value.map((node) => buildDeployRow(node, ''))
  } finally {
    deployLoading.value = false
  }
}

const closeDeployModal = () => {
  showDeployModal.value = false
}

const openCreateModal = () => {
  editingNode.value = null
  Object.assign(nodeForm, {
    name: '',
    address: '',
    tags: '',
    rate: 1.0,
    sort: 0,
    status: 0,
    parent_id: null,
    monthly_limit_gb: null,
    monthly_reset_day: 1
  })
  showNodeModal.value = true
}

const openEditModal = (node) => {
  editingNode.value = node
  Object.assign(nodeForm, {
    name: node.name,
    address: node.address || node.host || '',
    tags: node.tags || '',
    rate: node.rate || 1.0,
    sort: node.sort || 0,
    status: node.status,
    parent_id: node.parent_id || null,
    monthly_limit_gb: node.monthly_limit ? node.monthly_limit / BYTES_PER_GB : null,
    monthly_reset_day: node.monthly_reset_day || 1
  })
  showNodeModal.value = true
}

const closeNodeModal = () => {
  showNodeModal.value = false
  editingNode.value = null
}

const saveNode = async () => {
  if (!nodeForm.name || !nodeForm.address) {
    alert(t('admin.nodes.messages.requiredFields'))
    return
  }
  saving.value = true
  try {
    const payload = {
      name: nodeForm.name,
      host: nodeForm.address,
      tags: nodeForm.tags,
      rate: Number(nodeForm.rate),
      sort: Number(nodeForm.sort),
      parent_id: nodeForm.parent_id || null,
      monthly_limit: nodeForm.monthly_limit_gb ? Math.round(Number(nodeForm.monthly_limit_gb) * BYTES_PER_GB) : null,
      monthly_reset_day: Number(nodeForm.monthly_reset_day) || 1
    }
    if (editingNode.value) {
      payload.status = Number(nodeForm.status)
      await updateNode(editingNode.value.id, payload)
    } else {
      await createNode(payload)
    }
    closeNodeModal()
    loadNodes()
    loadStats()
  } catch (e) {
    alert(t('admin.nodes.messages.saveFailed', { message: readNodeApiError(e) }))
  } finally {
    saving.value = false
  }
}

const confirmDelete = async (node) => {
  if (!confirm(t('admin.nodes.messages.deleteNodeConfirm', { name: node.name }))) return
  try {
    await deleteNode(node.id)
    loadNodes()
    loadStats()
  } catch (e) {
    alert(t('admin.nodes.messages.deleteFailed', { message: readNodeApiError(e) }))
  }
}

const syncNode = async (node) => {
  if (syncingNodeIds.has(node.id)) return
  syncingNodeIds.add(node.id)
  try {
    await syncNodeProtocol(node.id)
    alert(t('admin.nodes.messages.syncSuccess', { name: node.name }))
  } catch (e) {
    alert(t('admin.nodes.messages.syncFailed', { message: readNodeApiError(e) }))
  } finally {
    syncingNodeIds.delete(node.id)
  }
}

const loadNodeLogs = async () => {
  if (!logNode.value) return
  logLoading.value = true
  try {
    const res = await getNodeLogs(logNode.value.id, {
      page: logPagination.page,
      page_size: logPagination.size,
      level: logFilter.level || undefined,
      source: logFilter.source || undefined,
      search: logFilter.search || undefined
    })
    const payload = readNodePage(res)
    nodeLogs.value = payload.list || []
    logPagination.total = payload.total || 0
  } catch (e) {
    console.error('Failed to load node logs:', e)
    nodeLogs.value = []
    logPagination.total = 0
  } finally {
    logLoading.value = false
  }
}

const refreshLogs = async () => {
  logPagination.page = 1
  await loadNodeLogs()
}

const openLogModal = async (node) => {
  logNode.value = node
  showLogModal.value = true
  logFilter.level = ''
  logFilter.source = ''
  logFilter.search = ''
  logPagination.page = 1
  await loadNodeLogs()
}

const closeLogModal = () => {
  showLogModal.value = false
  logNode.value = null
  nodeLogs.value = []
}

const changeLogPage = async (page) => {
  logPagination.page = page
  await loadNodeLogs()
}

// Protocol actions
const openProtocols = async (node) => {
  selectedNode.value = node
  showProtocolModal.value = true
  try {
    const res = await getNodeProtocols(node.id)
    protocols.value = readNodeList(res)
  } catch (e) {
    console.error('Failed to load protocols:', e)
    protocols.value = []
  }
}

const closeProtocolModal = () => {
  showProtocolModal.value = false
  selectedNode.value = null
  protocols.value = []
}

const openAddProtocol = () => {
  editingProtocol.value = null
  selectedTemplate.value = ''
  resetWireGuardForm()
  Object.assign(protocolForm, {
    mode: 'json',
    type: 'vless',
    port: 443,
    enable: 1,
    tls: 0,
    transport: 'tcp',
    settings: '{}',
    tls_settings: '{}',
    transport_settings: '{}',
    reality_settings: '{}',
    custom_config: '',
    show: 1
  })
  jsonEditorContent.value = JSON.stringify({
    type: 'vless',
    port: 443,
    tls: 0,
    transport: 'tcp',
    enable: 1,
    show: 1,
    settings: {},
    tls_settings: {},
    transport_settings: {},
    reality_settings: {}
  }, null, 2)
  jsonParseError.value = ''
  showProtocolFormModal.value = true
}

const editProtocol = (protocol) => {
  editingProtocol.value = protocol

  // Parse existing protocol data safely
  const settings = protocol.settings || '{}'
  const tlsSettings = protocol.tls_settings || '{}'
  const transportSettings = protocol.transport_settings || '{}'
  const realitySettings = protocol.reality_settings || '{}'

  // Build complete JSON for the editor
  const json = {
    type: protocol.type || 'vless',
    port: protocol.port || 443,
    tls: protocol.tls ?? 0,
    transport: protocol.transport || 'tcp',
    enable: protocol.enable ?? 1,
    show: protocol.show ?? 1,
  }
  try { json.settings = typeof settings === 'string' ? JSON.parse(settings) : settings } catch { json.settings = {} }
  try { json.tls_settings = typeof tlsSettings === 'string' ? JSON.parse(tlsSettings) : tlsSettings } catch { json.tls_settings = {} }
  try { json.transport_settings = typeof transportSettings === 'string' ? JSON.parse(transportSettings) : transportSettings } catch { json.transport_settings = {} }
  try { json.reality_settings = typeof realitySettings === 'string' ? JSON.parse(realitySettings) : realitySettings } catch { json.reality_settings = {} }
  if (protocol.custom_config) {
    try { json.custom_config = typeof protocol.custom_config === 'string' ? JSON.parse(protocol.custom_config) : protocol.custom_config } catch { json.custom_config = {} }
  }

  // Sync to visual form
  jsonToVisual(json)

  // Set JSON editor content
  jsonEditorContent.value = JSON.stringify(json, null, 2)
  jsonParseError.value = ''

  // Start in JSON mode
  protocolForm.mode = 'json'
  showProtocolFormModal.value = true
}

const closeProtocolFormModal = () => {
  showProtocolFormModal.value = false
  editingProtocol.value = null
}

const applyTemplate = (tpl) => {
  selectedTemplate.value = tpl.name

  const json = {
    type: tpl.type,
    port: tpl.default_port,
    tls: tpl.tls || 0,
    transport: tpl.transport || 'tcp',
    enable: 1,
    show: 1,
  }
  try { json.settings = JSON.parse(tpl.settings || '{}') } catch { json.settings = {} }
  try { json.tls_settings = JSON.parse(tpl.tls_settings || '{}') } catch { json.tls_settings = {} }
  try { json.transport_settings = JSON.parse(tpl.transport_settings || '{}') } catch { json.transport_settings = {} }
  try { json.reality_settings = JSON.parse(tpl.reality_settings || '{}') } catch { json.reality_settings = {} }
  if (json.type === 'wireguard') {
    hydrateWireGuardForm(json.settings)
  }

  jsonEditorContent.value = JSON.stringify(json, null, 2)
  jsonParseError.value = ''

  // Also sync to visual
  jsonToVisual(json)
  protocolForm.mode = 'json'
}

const saveProtocol = async () => {
  let payload

  if (protocolForm.mode === 'json') {
    // Parse from JSON editor
    try {
      const json = JSON.parse(jsonEditorContent.value)
      payload = {
        type: json.type || 'vless',
        port: json.port || 443,
        enable: json.enable ?? 1,
        tls: json.tls ?? 0,
        transport: json.transport || 'tcp',
        settings: JSON.stringify(json.settings || {}),
        tls_settings: JSON.stringify(json.tls_settings || {}),
        transport_settings: JSON.stringify(json.transport_settings || {}),
        reality_settings: JSON.stringify(json.reality_settings || {}),
        show: json.show ?? 1,
      }
      if (json.custom_config) {
        payload.custom_config = typeof json.custom_config === 'string' ? json.custom_config : JSON.stringify(json.custom_config)
      }
    } catch (e) {
      alert(t('admin.nodes.messages.invalidJson') + ': ' + e.message)
      return
    }
  } else {
    // Parse from visual form
    if (!protocolForm.type || !protocolForm.port) {
      alert(t('admin.nodes.messages.requiredFields'))
      return
    }
    const json = visualToJson()
    payload = {
      type: json.type,
      port: json.port,
      enable: json.enable,
      tls: json.tls,
      transport: json.transport,
      settings: JSON.stringify(json.settings || {}),
      tls_settings: JSON.stringify(json.tls_settings || {}),
      transport_settings: JSON.stringify(json.transport_settings || {}),
      reality_settings: JSON.stringify(json.reality_settings || {}),
      show: protocolForm.show,
    }
  }

  savingProtocol.value = true
  try {
    if (editingProtocol.value) {
      await updateNodeProtocol(selectedNode.value.id, editingProtocol.value.id, payload)
    } else {
      await createNodeProtocol(selectedNode.value.id, payload)
    }

    closeProtocolFormModal()
    const res = await getNodeProtocols(selectedNode.value.id)
    protocols.value = readNodeList(res)
  } catch (e) {
    alert(t('admin.nodes.messages.saveFailed', { message: readNodeApiError(e) }))
  } finally {
    savingProtocol.value = false
  }
}

const deleteProtocol = async (protocol) => {
  if (!confirm(t('admin.nodes.messages.deleteProtocolConfirm'))) return
  try {
    await deleteNodeProtocol(selectedNode.value.id, protocol.id)
    const res = await getNodeProtocols(selectedNode.value.id)
    protocols.value = readNodeList(res)
  } catch (e) {
    alert(t('admin.nodes.messages.deleteFailed', { message: readNodeApiError(e) }))
  }
}

// Pagination
const changePage = (page) => {
  pagination.page = page
  loadNodes()
}

// Helpers
const getStatusClass = (status) => {
  const classes = ['status-pending', 'status-online', 'status-offline', 'status-disabled']
  return classes[status] || 'status-pending'
}

const getStatusText = (status) => {
  const keys = ['pending', 'online', 'offline', 'disabled']
  const key = keys[status]
  return key ? t(`admin.nodes.statusText.${key}`) : t('admin.nodes.statusText.unknown')
}

const formatBytes = (bytes) => {
  if (!bytes) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  while (bytes >= 1024 && i < units.length - 1) {
    bytes /= 1024
    i++
  }
  return bytes.toFixed(2) + ' ' + units[i]
}

const formatTime = (timestamp) => {
  if (!timestamp) return '-'
  const date = new Date(timestamp * 1000)
  const now = new Date()
  const diff = (now - date) / 1000
  if (diff < 60) return t('admin.nodes.relativeTime.justNow')
  if (diff < 3600) return t('admin.nodes.relativeTime.minutesAgo', { count: Math.floor(diff / 60) })
  if (diff < 86400) return t('admin.nodes.relativeTime.hoursAgo', { count: Math.floor(diff / 3600) })
  return formatDateTime(timestamp)
}

const normalizeLogLevel = (level) => (logLevels.includes(level) ? level : 'info')

const getLogLevelLabel = (level) => {
  if (logLevels.includes(level)) {
    return t(`admin.nodes.logModal.levels.${level}`)
  }
  return String(level || 'INFO').toUpperCase()
}

const formatLogFields = (fields, fallback) => {
  if (fields) {
    return JSON.stringify(fields, null, 2)
  }
  if (!fallback) {
    return '{}'
  }
  try {
    return JSON.stringify(JSON.parse(fallback), null, 2)
  } catch {
    return fallback
  }
}

const formatLogTime = (log) => {
  if (log.logged_at) {
    return formatDateTime(log.logged_at)
  }
  if (log.created_at) {
    return formatDateTime(log.created_at)
  }
  return '-'
}

// Init
onMounted(async () => {
  const nodesLoaded = await loadNodes()
  if (!nodesLoaded) {
    return
  }
  await Promise.all([
    loadStats(),
    loadProtocolTemplates(),
    loadAuthKeysPreview()
  ])
})
</script>

<style scoped>
.nodes-page {
  padding: 0;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
  flex-wrap: wrap;
  gap: 16px;
}

.page-header h1 {
  margin: 0;
  font-size: 24px;
  font-weight: 600;
  color: var(--text-color);
}

.header-actions {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
}

/* Stats cards */
.stats-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 16px;
  margin-bottom: 24px;
}

.stat-card {
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  padding: 20px;
  text-align: center;
  transition: var(--transition);
}

.stat-card:hover {
  border-color: var(--text-secondary);
}

.stat-card.online { border-left: 4px solid var(--success-color); }
.stat-card.warning { border-left: 4px solid var(--warning-color); }
.stat-card.pending { border-left: 4px solid var(--primary-color); }

.stat-value {
  font-size: 32px;
  font-weight: 700;
  color: var(--text-color);
}

.stat-label {
  font-size: 14px;
  color: var(--text-secondary);
  margin-top: 4px;
}

/* Table container */
.table-container {
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  overflow: hidden;
}

.table {
  width: 100%;
  border-collapse: collapse;
}

.table th, .table td {
  padding: 14px 16px;
  text-align: left;
  border-bottom: 1px solid var(--border-color);
}

.table th {
  background: var(--bg-color);
  font-weight: 600;
  color: var(--text-secondary);
  font-size: 13px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.table tr:hover {
  background: var(--surface-hover);
}

.table td code {
  background: var(--bg-color);
  border: 1px solid var(--border-color);
  padding: 4px 8px;
  border-radius: var(--radius-sm);
  font-size: 12px;
  color: var(--text-color);
}

.quota-banner {
  background: rgba(239, 68, 68, 0.12);
  border: 1px solid var(--error-color);
  color: var(--error-color);
  border-radius: var(--radius-md);
  padding: 12px 18px;
  margin-bottom: 20px;
  font-size: 14px;
}

.row-over-quota {
  background: rgba(239, 68, 68, 0.06);
}

.quota-text {
  white-space: nowrap;
}

.quota-exceeded {
  color: var(--error-color);
  font-weight: 600;
}

.quota-tag {
  display: inline-block;
  margin-left: 6px;
  padding: 2px 8px;
  border-radius: 10px;
  background: rgba(239, 68, 68, 0.2);
  color: var(--error-color);
  font-size: 11px;
  font-weight: 500;
}

.field-hint {
  margin: 6px 0 0;
  font-size: 12px;
  color: var(--text-secondary);
}

.field-label-row {
	align-items: center;
	display: flex;
	justify-content: space-between;
	gap: 8px;
}

.wireguard-editor {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.section-title {
  margin-top: 8px;
  padding-top: 12px;
  border-top: 1px solid var(--border-color);
  color: var(--text-color);
  font-size: 14px;
  font-weight: 600;
}

.network-policy-title,
.network-path-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.network-policy-title .checkbox-label {
  margin: 0;
  font-size: 13px;
  text-transform: none;
}

.network-path-card {
  margin: 12px 0;
  padding: 14px;
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  background: var(--bg-color);
}

.network-path-header {
  margin-bottom: 12px;
}

.network-health-row {
  margin-top: 14px;
}

.node-tags {
  display: flex;
  gap: 4px;
  margin-left: 8px;
  flex-wrap: wrap;
}

.tag {
  background: rgba(99, 102, 241, 0.2);
  color: #a5b4fc;
  padding: 2px 8px;
  border-radius: 12px;
  font-size: 11px;
}

/* Status badges */
.status-badge {
  display: inline-block;
  padding: 4px 12px;
  border-radius: 12px;
  font-size: 12px;
  font-weight: 500;
}

.status-pending { background: rgba(99, 102, 241, 0.2); color: #a5b4fc; }
.status-online { background: rgba(34, 197, 94, 0.2); color: var(--success-color); }
.status-offline { background: rgba(239, 68, 68, 0.2); color: var(--error-color); }
.status-disabled { background: rgba(161, 161, 170, 0.2); color: var(--text-secondary); }

.runtime-health-badge {
  display: inline-block;
  margin-top: 4px;
  padding: 2px 6px;
  border-radius: 4px;
  font-size: 11px;
}

.runtime-healthy { background: rgba(34, 197, 94, 0.14); color: var(--success-color); }
.runtime-unhealthy { background: rgba(239, 68, 68, 0.14); color: var(--error-color); }

.actions {
  display: flex;
  gap: 8px;
}

.sync-node-btn {
  min-width: 92px;
  white-space: nowrap;
}

/* Buttons */
.btn {
  padding: 8px 16px;
  border: none;
  border-radius: var(--radius-md);
  cursor: pointer;
  font-weight: 500;
  transition: var(--transition);
  font-size: 14px;
}

.btn-primary { background: var(--primary-color); color: white; }
.btn-primary:hover { background: var(--primary-hover); transform: translateY(-1px); }
.btn-secondary { background: var(--surface-color); color: var(--text-color); border: 1px solid var(--border-color); }
.btn-secondary:hover { background: var(--surface-hover); border-color: var(--text-secondary); }
.btn-info { background: #0ea5e9; color: white; }
.btn-info:hover { background: #0284c7; }
.btn-warning { background: var(--warning-color); color: white; }
.btn-warning:hover { background: #d97706; }
.btn-danger { background: var(--error-color); color: white; }
.btn-danger:hover { background: #dc2626; }

.btn-sm { padding: 6px 12px; font-size: 12px; }

.btn:disabled { opacity: 0.5; cursor: not-allowed; transform: none; }

/* Pagination */
.pagination {
  display: flex;
  justify-content: center;
  align-items: center;
  gap: 16px;
  padding: 20px;
  color: var(--text-secondary);
}

.pagination button {
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  color: var(--text-color);
}

.pagination button:disabled {
  opacity: 0.4;
}

/* Modal */
.modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.7);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  backdrop-filter: blur(4px);
}

.modal {
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  width: 480px;
  max-width: 90%;
  max-height: 90vh;
  overflow-y: auto;
  box-shadow: var(--shadow-lg);
}

.modal-lg {
  width: 720px;
}

.modal-xl {
  width: min(1080px, 96vw);
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 20px 24px;
  border-bottom: 1px solid var(--border-color);
}

.modal-header h3 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  color: var(--text-color);
}

.close-btn {
  background: none;
  border: none;
  font-size: 24px;
  cursor: pointer;
  color: var(--text-secondary);
  padding: 4px;
  line-height: 1;
  transition: var(--transition);
}

.close-btn:hover {
  color: var(--text-color);
}

.modal-body {
  padding: 24px;
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  padding: 20px 24px;
  border-top: 1px solid var(--border-color);
}

/* Form styles */
.form-group {
  margin-bottom: 20px;
}

.form-group label {
  display: block;
  margin-bottom: 8px;
  font-weight: 500;
  color: var(--text-color);
  font-size: 14px;
}

.form-group input,
.form-group select,
.form-group textarea {
  width: 100%;
  padding: 12px 14px;
  background: var(--bg-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  font-size: 14px;
  color: var(--text-color);
  transition: var(--transition);
}

.form-group input:focus,
.form-group select:focus,
.form-group textarea:focus {
  border-color: var(--primary-color);
  outline: none;
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
}

.form-group input::placeholder,
.form-group textarea::placeholder {
  color: var(--text-secondary);
}

.form-group textarea {
  font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;
  resize: vertical;
}

.form-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
}

.checkbox-label {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  color: var(--text-color);
}

.checkbox-label input {
  width: auto;
  accent-color: var(--primary-color);
}

.protocol-header {
  display: flex;
  justify-content: flex-end;
  margin-bottom: 16px;
}

.form-inline {
  display: flex;
  gap: 12px;
  align-items: center;
}

.protocol-type {
  font-weight: 600;
  color: var(--primary-color);
}

.empty-message {
  text-align: center;
  padding: 40px;
  color: var(--text-secondary);
}

.info-box {
  background: rgba(59, 130, 246, 0.1);
  border: 1px solid rgba(59, 130, 246, 0.3);
  border-radius: var(--radius-md);
  padding: 14px 18px;
  margin-bottom: 20px;
  color: var(--primary-color);
  font-size: 14px;
}

.key-text {
  font-size: 12px;
  background: var(--bg-color);
  padding: 4px 8px;
  border-radius: var(--radius-sm);
  word-break: break-all;
  color: var(--text-color);
  font-family: monospace;
}

/* Protocol templates */
.template-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 12px;
  margin-bottom: 20px;
}

.template-card {
  padding: 12px;
  background: var(--bg-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: var(--transition);
}

.template-card:hover {
  border-color: var(--primary-color);
  background: var(--surface-hover);
}

.template-card.active {
  border-color: var(--primary-color);
  background: rgba(59, 130, 246, 0.1);
  box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.2);
}

.tpl-name {
  font-weight: 600;
  font-size: 13px;
  margin-bottom: 4px;
  color: var(--primary-color);
}

.tpl-desc {
  font-size: 11px;
  color: var(--text-secondary);
  line-height: 1.4;
}

/* Tabs */
.tabs {
  display: flex;
  gap: 8px;
  margin-bottom: 20px;
  border-bottom: 1px solid var(--border-color);
  padding-bottom: 12px;
}

.tab-btn {
  background: none;
  border: none;
  color: var(--text-secondary);
  padding: 8px 16px;
  border-radius: var(--radius-md);
  cursor: pointer;
  font-size: 14px;
  transition: var(--transition);
}

.tab-btn:hover {
  color: var(--text-color);
  background: var(--bg-color);
}

.tab-btn.active {
  color: white;
  background: var(--primary-color);
}

.badge {
  display: inline-block;
  padding: 4px 10px;
  border-radius: var(--radius-sm);
  font-size: 12px;
  font-weight: 500;
}

.badge-success {
  background: rgba(34, 197, 94, 0.2);
  color: var(--success-color);
}

.badge-info {
  background: rgba(59, 130, 246, 0.2);
  color: var(--primary-color);
}

.text-center {
  text-align: center;
}

/* JSON editor */
.json-editor-actions {
  display: flex;
  gap: 8px;
  align-items: center;
  margin-bottom: 12px;
}

.json-textarea {
  font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', 'Consolas', monospace !important;
  font-size: 13px;
  line-height: 1.5;
  background: var(--bg-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: 14px;
  color: var(--text-color);
  width: 100%;
  resize: vertical;
  tab-size: 2;
}

.json-textarea:focus {
  border-color: var(--primary-color);
  outline: none;
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
}

.json-status {
  margin-left: auto;
  font-size: 12px;
  font-weight: 500;
  padding: 4px 10px;
  border-radius: var(--radius-sm);
}

.json-status.valid {
  background: rgba(34, 197, 94, 0.15);
  color: var(--success-color);
}

.json-status.invalid {
  background: rgba(239, 68, 68, 0.15);
  color: var(--error-color);
}

/* Advanced details */
.advanced-details {
  margin-bottom: 16px;
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  overflow: hidden;
}

.advanced-details summary {
  padding: 12px 16px;
  background: var(--bg-color);
  cursor: pointer;
  font-weight: 500;
  font-size: 13px;
  color: var(--text-secondary);
  user-select: none;
}

.advanced-details summary:hover {
  background: var(--surface-hover);
}

.advanced-details > .json-textarea {
  border: none;
  border-radius: 0;
  border-top: 1px solid var(--border-color);
}

/* Responsive */
@media (max-width: 768px) {
  .page-header {
    flex-direction: column;
    align-items: flex-start;
  }

  .header-actions {
    width: 100%;
  }

  .header-actions .btn {
    flex: 1;
  }

  .form-row {
    grid-template-columns: 1fr;
  }

  .deploy-summary {
    flex-direction: column;
  }

  .deploy-settings-grid,
  .deploy-output-grid {
    grid-template-columns: 1fr;
  }

  .table-container {
    overflow-x: auto;
  }

  .table {
    min-width: 800px;
  }

  .actions {
    flex-direction: column;
  }

  .modal {
    max-width: 95%;
  }

  .log-toolbar {
    grid-template-columns: 1fr;
  }
}

/* Auth Key Modal */
.auth-key-hint {
  color: var(--text-secondary);
  margin-bottom: 16px;
  font-size: 14px;
}

.auth-key-display {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}

.auth-key-value {
  flex: 1;
  background: var(--bg-color);
  padding: 12px 16px;
  border-radius: var(--radius-md);
  font-family: var(--font-mono);
  font-size: 14px;
  word-break: break-all;
  border: 1px solid var(--border-color);
}

.auth-key-usage {
  color: var(--text-secondary);
  font-size: 13px;
  margin-bottom: 16px;
}

.auth-key-config {
  margin-top: 20px;
}

.auth-key-config label {
  display: block;
  color: var(--text-secondary);
  font-size: 13px;
  margin-bottom: 8px;
}

.config-block {
  background: var(--bg-color);
  padding: 16px;
  border-radius: var(--radius-md);
  font-family: var(--font-mono);
  font-size: 13px;
  white-space: pre-wrap;
  word-break: break-all;
  border: 1px solid var(--border-color);
  max-height: 300px;
  overflow-y: auto;
  margin-bottom: 12px;
}

.deploy-summary {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 18px;
}

.deploy-summary-title {
  margin: 4px 0;
  font-size: 18px;
  font-weight: 600;
}

.deploy-warning {
  max-width: 320px;
}

.deploy-settings-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 16px;
  margin-bottom: 20px;
}

.deploy-table input,
.deploy-table select {
  min-width: 0;
}

.deploy-meta {
  margin-top: 4px;
  font-size: 12px;
  color: var(--text-secondary);
}

.deploy-output-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
  margin-top: 20px;
}

.deploy-output-panel {
  display: grid;
  gap: 8px;
}

.deploy-output-panel textarea {
  width: 100%;
  min-height: 180px;
  resize: vertical;
  font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', 'Consolas', monospace;
  background: var(--bg-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  color: var(--text-color);
  padding: 12px;
}

.deploy-output-panel-full {
  margin-top: 16px;
}

.deploy-output-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.log-toolbar {
  display: grid;
  grid-template-columns: 160px 180px minmax(240px, 1fr) auto;
  gap: 12px;
  margin-bottom: 16px;
}

.log-toolbar input,
.log-toolbar select {
  width: 100%;
  padding: 10px 12px;
  background: var(--bg-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  color: var(--text-color);
  font-size: 14px;
}

.log-toolbar input:focus,
.log-toolbar select:focus {
  border-color: var(--primary-color);
  outline: none;
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
}

.node-log-table {
  table-layout: fixed;
}

.node-log-table th:nth-child(1),
.node-log-table td:nth-child(1) {
  width: 180px;
}

.node-log-table th:nth-child(2),
.node-log-table td:nth-child(2) {
  width: 120px;
}

.node-log-table th:nth-child(3),
.node-log-table td:nth-child(3) {
  width: 160px;
}

.log-time,
.log-source {
  color: var(--text-secondary);
  font-size: 13px;
  vertical-align: top;
}

.log-message-cell {
  vertical-align: top;
}

.log-message {
  white-space: pre-wrap;
  word-break: break-word;
  color: var(--text-color);
}

.log-meta {
  margin-top: 8px;
  color: var(--text-secondary);
  font-size: 12px;
}

.log-fields {
  margin-top: 10px;
}

.log-fields summary {
  cursor: pointer;
  color: var(--text-secondary);
  font-size: 12px;
}

.log-fields pre {
  margin: 8px 0 0;
  padding: 12px;
  background: var(--bg-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  color: var(--text-color);
  font-size: 12px;
  overflow-x: auto;
}

.log-level-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 72px;
  padding: 4px 10px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 600;
  text-transform: uppercase;
}

.log-level-debug {
  background: rgba(148, 163, 184, 0.2);
  color: #cbd5e1;
}

.log-level-info {
  background: rgba(59, 130, 246, 0.16);
  color: #93c5fd;
}

.log-level-warning {
  background: rgba(245, 158, 11, 0.18);
  color: #fbbf24;
}

.log-level-error {
  background: rgba(239, 68, 68, 0.18);
  color: #fca5a5;
}

.modal-pagination {
  padding: 16px 0 0;
}
</style>
