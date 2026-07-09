<template>
  <div class="system-page">
    <div class="page-header">
      <h1>{{ t('runtime.systemPage.title') }}</h1>
      <p class="text-secondary">{{ t('runtime.systemPage.subtitle') }}</p>
    </div>

    <!-- Tab switcher -->
    <div class="tabs">
      <button :class="['tab', { active: activeTab === 'config' }]" @click="activeTab = 'config'">
        {{ t('runtime.systemPage.tabs.config') }}
      </button>
      <button :class="['tab', { active: activeTab === 'backup' }]" @click="activeTab = 'backup'">
        {{ t('runtime.systemPage.tabs.backup') }}
      </button>
      <button :class="['tab', { active: activeTab === 'balancer' }]" @click="activeTab = 'balancer'">
        {{ t('runtime.systemPage.tabs.balancer') }}
      </button>
      <button :class="['tab', { active: activeTab === 'audit' }]" @click="activeTab = 'audit'">
        {{ t('runtime.systemPage.tabs.audit') }}
      </button>
    </div>

    <!-- System config -->
    <div v-show="activeTab === 'config'">
      <div class="toolbar">
        <input v-model="configSearch" type="text" :placeholder="t('runtime.systemPage.config.searchPlaceholder')" class="search-input" />
        <button class="btn-primary" @click="openConfigModal()">{{ t('runtime.systemPage.actions.addConfig') }}</button>
      </div>
      <section class="subscription-settings-card">
        <div class="subscription-settings-head">
          <div>
            <p class="eyebrow">{{ t('runtime.systemPage.subscription.eyebrow') }}</p>
            <h3>{{ t('runtime.systemPage.subscription.title') }}</h3>
            <p class="text-secondary">{{ t('runtime.systemPage.subscription.description') }}</p>
          </div>
          <div class="subscription-settings-actions">
            <button class="btn btn-secondary btn-sm" :disabled="subscriptionSettingsLoading" @click="loadSubscriptionDomainSettings">
              {{ subscriptionSettingsLoading ? t('runtime.shared.loading') : t('runtime.systemPage.subscription.actions.refresh') }}
            </button>
            <button class="btn btn-primary btn-sm" :disabled="subscriptionSettingsSaving" @click="saveSubscriptionDomainSettings">
              {{ subscriptionSettingsSaving ? t('runtime.systemPage.subscription.actions.saving') : t('runtime.systemPage.subscription.actions.save') }}
            </button>
          </div>
        </div>
        <div class="subscription-settings-grid">
          <div class="form-group">
            <label>{{ t('runtime.systemPage.subscription.pathLabel') }}</label>
            <input :value="subscriptionPath" type="text" readonly />
          </div>
          <div class="form-group">
            <label>{{ t('runtime.systemPage.subscription.currentDomainLabel') }}</label>
            <input :value="subscriptionCurrentHost" type="text" readonly />
          </div>
        </div>
        <div class="form-group">
          <label>{{ t('runtime.systemPage.subscription.domainListLabel') }}</label>
          <textarea
            v-model="subscriptionDomainsText"
            rows="5"
            :placeholder="t('runtime.systemPage.subscription.domainListPlaceholder')"
          ></textarea>
          <p class="field-help">{{ t('runtime.systemPage.subscription.domainListHelp') }}</p>
        </div>
        <div v-if="subscriptionDomainError" class="form-error">{{ subscriptionDomainError }}</div>
        <div class="subscription-settings-preview">
          <p class="metric-label">{{ t('runtime.systemPage.subscription.previewLabel') }}</p>
          <code v-if="normalizedSubscriptionDomains.length === 0">{{ t('runtime.systemPage.subscription.previewEmpty') }}</code>
          <code v-for="domain in normalizedSubscriptionDomains" :key="domain">
            {{ `${subscriptionPreviewProtocol}//${domain}${subscriptionPath}` }}
          </code>
        </div>
      </section>
      <section class="runtime-config-card">
        <div class="runtime-config-head">
          <div>
            <p class="eyebrow">{{ t('runtime.workbench.eyebrow') }}</p>
            <p class="runtime-workbench-title">{{ t('runtime.workbench.title') }}</p>
            <h3>{{ t('runtime.workbench.subtitle') }}</h3>
          </div>
          <div class="runtime-config-actions">
            <button class="btn btn-secondary btn-sm" :disabled="runtimeStatusLoading" @click="fetchRuntimeStatusSafe">
              {{ runtimeStatusLoading ? t('runtime.shared.loading') : t('runtime.shared.refreshStatus') }}
            </button>
            <button class="btn btn-secondary btn-sm" :disabled="runtimeJobsLoading" @click="fetchForwardRuntimeJobs">
              {{ runtimeJobsLoading ? t('runtime.shared.loading') : t('runtime.workbench.actions.refreshJobs') }}
            </button>
            <router-link class="btn btn-secondary btn-sm" to="/admin/forward/ansible-machines">{{ t('runtime.workbench.actions.openAnsibleMachines') }}</router-link>
            <router-link class="btn btn-secondary btn-sm" to="/admin/forward/local">{{ t('runtime.workbench.actions.openLocalRuntime') }}</router-link>
            <router-link class="btn btn-secondary btn-sm" to="/admin/forward/nodex">{{ t('runtime.workbench.actions.openNodeXRuntime') }}</router-link>
          </div>
        </div>
        <div class="runtime-mode-overview">
          <article :class="['runtime-mode-card', { active: !runtimeNodeXMode }]">
            <p class="eyebrow">{{ t('runtime.workbench.localCard.eyebrow') }}</p>
            <h4>{{ t('runtime.workbench.localCard.title') }}</h4>
            <p class="text-secondary mode-description">
              {{ t('runtime.workbench.localCard.description') }}
            </p>
            <p class="metric-detail">{{ t('runtime.workbench.localCard.currentState') }}: {{ runtimeNodeXMode ? t('runtime.workbench.state.standby') : t('runtime.workbench.state.activeBackend') }}</p>
            <p class="metric-detail">{{ t('runtime.localRuntime.fields.inventory') }}: {{ runtimeAnsibleForm.inventory || '-' }}</p>
            <p class="metric-detail">{{ t('runtime.localRuntime.fields.command') }}: {{ runtimeAnsibleForm.command || defaultRuntimeAnsibleConfig.command }}</p>
            <router-link class="btn btn-secondary btn-sm" to="/admin/forward/local">{{ t('runtime.workbench.localCard.manage') }}</router-link>
          </article>

          <article :class="['runtime-mode-card', { active: runtimeNodeXMode }]">
            <p class="eyebrow">{{ t('runtime.workbench.nodeXCard.eyebrow') }}</p>
            <h4>{{ t('runtime.workbench.nodeXCard.title') }}</h4>
            <p class="text-secondary mode-description">
              {{ t('runtime.workbench.nodeXCard.description') }}
            </p>
            <p class="metric-detail">{{ t('runtime.workbench.localCard.currentState') }}: {{ runtimeNodeXMode ? t('runtime.workbench.state.activeBackend') : t('runtime.workbench.state.standby') }}</p>
            <p class="metric-detail">{{ t('runtime.nodeX.cards.baseUrl') }}: {{ runtimeNodeXBaseUrl || '-' }}</p>
            <p class="metric-detail">{{ t('runtime.nodeX.cards.tokenConfigured') }}: {{ runtimeNodeXToken ? t('runtime.shared.yes') : t('runtime.shared.no') }}</p>
            <router-link class="btn btn-secondary btn-sm" to="/admin/forward/nodex">{{ t('runtime.workbench.nodeXCard.manage') }}</router-link>
          </article>
        </div>

        <div class="runtime-jobs-block">
          <div class="runtime-jobs-head">
            <h4>{{ t('runtime.workbench.recentJobsTitle') }}</h4>
            <span class="text-secondary">{{ t('runtime.workbench.recentJobsSubtitle') }}</span>
          </div>

          <div v-if="runtimeJobsLoading" class="runtime-jobs-empty">{{ t('runtime.workbench.loadingJobs') }}</div>

          <div v-else-if="runtimeJobs.length" class="runtime-jobs-list">
            <article v-for="job in runtimeJobs" :key="job.id" class="runtime-job-item">
              <div class="runtime-job-main">
                <div class="runtime-job-title">
                  <strong>#{{ job.id }} {{ job.action }}</strong>
                  <span>{{ t('runtime.workbench.jobMeta', { backend: runtimeBackendLabel(job.backend), forwardId: job.forwardId || '-', tunnelId: job.tunnelId || '-', nodeId: job.nodeId || '-' }) }}</span>
                </div>
                <div class="runtime-job-side">
                  <span :class="['status-badge', `runtime-status-${job.status}`]">{{ getRuntimeJobStatusLabel(job.status) }}</span>
                  <span class="runtime-job-time">{{ formatRuntimeJobTime(job) }}</span>
                </div>
              </div>
              <code v-if="formatRuntimeJobMessage(job)" class="runtime-job-message">{{ formatRuntimeJobMessage(job) }}</code>
            </article>
          </div>

          <div v-else class="runtime-jobs-empty">{{ t('runtime.workbench.noJobs') }}</div>
        </div>

        <div class="runtime-operator-panel">
          <div class="operator-head">
            <div>
              <p class="eyebrow">{{ t('runtime.workbench.doctor.eyebrow') }}</p>
              <h4>{{ t('runtime.workbench.doctor.title') }}</h4>
              <p class="text-secondary mode-description">
                {{ t('runtime.workbench.doctor.description') }}
              </p>
              <p class="text-secondary mode-description">
                {{ t('runtime.workbench.doctor.note') }}
              </p>
              <p class="text-secondary mode-description runtime-operator-note">
                {{ t('runtime.workbench.doctor.summary') }}
              </p>
            </div>
            <div class="operator-actions">
              <button class="btn btn-secondary btn-sm" :disabled="runtimeStatusLoading" @click="fetchRuntimeStatusSafe">
                {{ runtimeStatusLoading ? t('runtime.shared.loading') : t('runtime.workbench.actions.refreshActiveRuntime') }}
              </button>
              <button class="btn btn-secondary btn-sm" :disabled="runtimeDoctorRunning" @click="runRuntimeDoctorCheckSafe">
                {{ runtimeDoctorRunning ? t('runtime.shared.runningDoctor') : t('runtime.workbench.actions.runDoctorActiveRuntime') }}
              </button>
            </div>
          </div>

          <div v-if="runtimeStatusLoading" class="operator-loading">{{ t('runtime.workbench.doctor.loadingStatus') }}</div>
          <div v-else>
            <div v-if="runtimeStatusError" class="form-error">{{ runtimeStatusError }}</div>
            <div v-else class="operator-status-grid">
              <div class="status-card operator-card">
                <p class="metric-label">{{ t('runtime.workbench.cards.backend') }}</p>
                <p class="metric-value">{{ runtimeBackendLabel(runtimeStatus?.config?.backend || (runtimeNodeXMode ? 'gost' : runtimeBackend)) }}</p>
                <p class="metric-detail">{{ t('runtime.workbench.cards.nodeXMode') }}: {{ runtimeStatus?.config?.nodeXMode ? t('runtime.shared.enabled') : t('runtime.shared.disabled') }}</p>
                <p class="metric-detail">{{ t('runtime.workbench.cards.attachment') }}: {{ runtimeStatus?.attachment?.model || '-' }}</p>
                <p class="metric-detail">{{ translateRuntimeText(runtimeStatus?.attachment?.description) }}</p>
              </div>
              <div class="status-card operator-card">
                <p class="metric-label">{{ t('runtime.workbench.cards.panelVerdict') }}</p>
                <p class="metric-value">{{ runtimeStatus?.runtimeReady?.ready ? t('runtime.shared.ready') : t('runtime.shared.notReady') }}</p>
                <p class="metric-detail">{{ t('runtime.shared.reachability') }}: {{ runtimeStatus?.reachability?.ready ? t('runtime.shared.ready') : t('runtime.shared.notReady') }}</p>
                <p class="metric-detail">{{ translateRuntimeText(runtimeStatus?.reachability?.reason) }}</p>
                <p class="metric-detail">{{ translateRuntimeText(runtimeStatus?.runtimeReady?.reason) }}</p>
              </div>
              <div class="status-card operator-card">
                <template v-if="runtimeStatus?.config?.nodeXMode">
                  <p class="metric-label">{{ t('runtime.workbench.cards.nodeXSnapshot') }}</p>
                  <p class="metric-detail">{{ t('runtime.nodeX.cards.baseUrl') }}: {{ runtimeStatus?.config?.baseUrl || '-' }}</p>
                  <p class="metric-detail">{{ t('runtime.workbench.cards.baseUrlConfigured') }}: {{ runtimeStatus?.config?.baseUrlConfigured ? t('runtime.shared.yes') : t('runtime.shared.no') }}</p>
                  <p class="metric-detail">{{ t('runtime.nodeX.cards.tokenConfigured') }}: {{ runtimeStatus?.config?.tokenConfigured ? t('runtime.shared.yes') : t('runtime.shared.no') }}</p>
                  <p class="metric-detail">{{ t('runtime.nodeX.cards.health') }}: {{ runtimeStatus?.health?.ok ? t('runtime.shared.ready') : t('runtime.shared.unavailable') }}</p>
                  <p class="metric-detail">{{ t('runtime.workbench.cards.runtimeVersion') }}: {{ runtimeStatus?.runtimeStatus?.version || '-' }}</p>
                </template>
                <template v-else>
                  <p class="metric-label">{{ t('runtime.workbench.cards.localAnsible') }}</p>
                  <p class="metric-detail">{{ t('runtime.localRuntime.fields.command') }}: {{ runtimeStatus?.localAnsible?.command || defaultRuntimeAnsibleConfig.command }}</p>
                  <p class="metric-detail">{{ t('runtime.localRuntime.cards.commandFound') }}: {{ runtimeStatus?.localAnsible?.commandFound ? t('runtime.shared.yes') : t('runtime.shared.no') }}</p>
                  <p class="metric-detail">{{ t('runtime.localRuntime.fields.inventory') }}: {{ runtimeStatus?.localAnsible?.inventoryExists ? t('runtime.shared.present') : t('runtime.shared.missing') }}</p>
                  <p class="metric-detail">{{ t('runtime.workbench.cards.playbooks') }}: {{ runtimeStatus?.localAnsible?.applyPlaybookExists && runtimeStatus?.localAnsible?.removePlaybookExists ? t('runtime.shared.ready') : t('runtime.shared.missing') }}</p>
                  <p class="metric-detail">{{ t('runtime.localRuntime.fields.workingDir') }}: {{ runtimeStatus?.localAnsible?.workingDirExists ? t('runtime.shared.present') : t('runtime.shared.missing') }}</p>
                </template>
              </div>
            </div>
          </div>

          <div v-if="runtimeStatus?.warnings?.length" class="operator-commands">
            <p class="metric-label">{{ t('runtime.shared.warnings') }}</p>
            <code v-for="warning in runtimeStatus.warnings" :key="warning">{{ translateRuntimeText(warning) }}</code>
          </div>

          <div class="operator-commands">
            <p class="metric-label">{{ t('runtime.shared.powerShell') }}</p>
            <code v-for="command in runtimeDisplayedCommands.powerShell" :key="`ps-${command}`">{{ command }}</code>
            <p class="metric-label">{{ t('runtime.shared.bash') }}</p>
            <code v-for="command in runtimeDisplayedCommands.bash" :key="`bash-${command}`">{{ command }}</code>
            <p class="metric-label">{{ t('runtime.shared.bootstrapVerify') }}</p>
            <code v-for="command in runtimeDisplayedCommands.upgrade" :key="`upgrade-${command}`">{{ command }}</code>
            <p class="metric-label">{{ t('runtime.shared.references') }}</p>
            <code v-for="reference in runtimeDisplayedCommands.references" :key="reference">{{ reference }}</code>
          </div>
          <div class="operator-doctor-output">
            <p class="metric-label">{{ t('runtime.shared.doctorOutput') }}</p>
            <pre>{{ runtimeDoctorOutput || t('runtime.shared.doctorNotExecuted') }}</pre>
          </div>
        </div>
      </section>

      <div class="table-container">
        <table class="data-table">
          <thead>
            <tr>
              <th>{{ t('runtime.systemPage.config.table.key') }}</th>
              <th>{{ t('runtime.systemPage.config.table.value') }}</th>
              <th>{{ t('runtime.systemPage.config.table.description') }}</th>
              <th>{{ t('runtime.systemPage.config.table.updatedAt') }}</th>
              <th>{{ t('runtime.systemPage.config.table.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="config in filteredConfigs" :key="config.key">
              <td><code>{{ config.key }}</code></td>
              <td class="value-cell">{{ truncateValue(getConfigDisplayValue(config)) }}</td>
              <td>{{ translateRuntimeText(config.description, config.description || '-') }}</td>
              <td>{{ formatTime(config.updated_at) }}</td>
              <td>
                <div class="action-buttons">
                  <button class="btn-sm btn-ghost" @click="openConfigModal(config)">{{ t('common.actions.edit') }}</button>
                  <button class="btn-sm btn-ghost" @click="deleteConfig(config)">{{ t('common.actions.delete') }}</button>
                </div>
              </td>
            </tr>
            <tr v-if="filteredConfigs.length === 0">
              <td colspan="5" class="empty-row">{{ t('runtime.systemPage.config.empty') }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Backups -->
    <div v-show="activeTab === 'backup'">
      <div class="backup-config">
        <h3>{{ t('runtime.systemPage.backup.title') }}</h3>
        <div class="form-row">
          <div class="form-group">
            <label class="checkbox-label">
              <input type="checkbox" v-model="backupConfig.enabled" />
              <span>{{ t('runtime.systemPage.backup.enabled') }}</span>
            </label>
          </div>
          <div class="form-group">
            <label>{{ t('runtime.systemPage.backup.intervalHours') }}</label>
            <input v-model.number="backupConfig.interval" type="number" min="1" />
          </div>
          <div class="form-group">
            <label>{{ t('runtime.systemPage.backup.keepCount') }}</label>
            <input v-model.number="backupConfig.keep_count" type="number" min="1" />
          </div>
        </div>
        <div class="form-row">
          <div class="form-group">
            <label class="checkbox-label">
              <input type="checkbox" v-model="backupConfig.backup_database" />
              <span>{{ t('runtime.systemPage.backup.backupDatabase') }}</span>
            </label>
          </div>
          <div class="form-group">
            <label class="checkbox-label">
              <input type="checkbox" v-model="backupConfig.backup_files" />
              <span>{{ t('runtime.systemPage.backup.backupFiles') }}</span>
            </label>
          </div>
        </div>
        <div class="form-row">
          <div class="form-group">
            <label>{{ t('runtime.systemPage.backup.storageType') }}</label>
            <select v-model="backupConfig.storage_type">
              <option value="local">{{ t('runtime.systemPage.backup.storageTypes.local') }}</option>
              <option value="s3">{{ t('runtime.systemPage.backup.storageTypes.s3') }}</option>
            </select>
          </div>
          <div class="form-group" v-if="backupConfig.storage_type === 'local'">
            <label>{{ t('runtime.systemPage.backup.storagePath') }}</label>
            <input v-model="backupConfig.storage_path" type="text" :placeholder="t('runtime.systemPage.backup.storagePathPlaceholder')" />
          </div>
        </div>
        <div v-if="backupConfig.storage_type === 's3'" class="form-grid">
          <div class="form-group">
            <label>{{ t('runtime.systemPage.backup.s3Bucket') }}</label>
            <input v-model="backupConfig.s3_bucket" type="text" :placeholder="t('runtime.systemPage.backup.s3BucketPlaceholder')" />
          </div>
          <div class="form-group">
            <label>{{ t('runtime.systemPage.backup.s3Region') }}</label>
            <input v-model="backupConfig.s3_region" type="text" :placeholder="t('runtime.systemPage.backup.s3RegionPlaceholder')" />
          </div>
          <div class="form-group">
            <label>{{ t('runtime.systemPage.backup.s3Endpoint') }}</label>
            <input v-model="backupConfig.s3_endpoint" type="text" :placeholder="t('runtime.systemPage.backup.s3EndpointPlaceholder')" />
          </div>
          <div class="form-group">
            <label>{{ t('runtime.systemPage.backup.s3AccessKey') }}</label>
            <input v-model="backupConfig.s3_access_key" type="text" :placeholder="t('runtime.systemPage.backup.s3AccessKeyPlaceholder')" />
            <p v-if="backupConfig.s3_access_key_sensitive" class="text-secondary">
              {{
                backupConfig.s3_access_key_has_value
                  ? t('runtime.systemPage.backup.sensitiveHintWithValue')
                  : t('runtime.systemPage.backup.sensitiveHintWithoutValue')
              }}
            </p>
          </div>
          <div class="form-group">
            <label>{{ t('runtime.systemPage.backup.s3SecretKey') }}</label>
            <input v-model="backupConfig.s3_secret_key" type="password" :placeholder="t('runtime.systemPage.backup.s3SecretKeyPlaceholder')" autocomplete="new-password" />
            <p v-if="backupConfig.s3_secret_key_sensitive" class="text-secondary">
              {{
                backupConfig.s3_secret_key_has_value
                  ? t('runtime.systemPage.backup.sensitiveHintWithValue')
                  : t('runtime.systemPage.backup.sensitiveHintWithoutValue')
              }}
            </p>
          </div>
        </div>
        <div class="form-actions">
          <button class="btn-secondary" @click="saveBackupConfig">{{ t('runtime.systemPage.backup.saveConfig') }}</button>
          <button class="btn-primary" @click="createBackupRequest">{{ t('runtime.systemPage.backup.backupNow') }}</button>
        </div>
      </div>

      <div class="backup-stats">
        <div class="stat-item">
          <span class="stat-label">{{ t('runtime.systemPage.backup.stats.totalCount') }}</span>
          <span class="stat-value">{{ backupStats.total_count || 0 }}</span>
        </div>
        <div class="stat-item">
          <span class="stat-label">{{ t('runtime.systemPage.backup.stats.totalSize') }}</span>
          <span class="stat-value">{{ formatSize(backupStats.total_size) }}</span>
        </div>
        <div class="stat-item">
          <span class="stat-label">{{ t('runtime.systemPage.backup.stats.lastBackup') }}</span>
          <span class="stat-value">{{ backupStats.last_backup ? formatTime(backupStats.last_backup) : '-' }}</span>
        </div>
      </div>

      <div class="table-container">
        <h3>{{ t('runtime.systemPage.backup.listTitle') }}</h3>
        <table class="data-table">
          <thead>
            <tr>
              <th>{{ t('runtime.systemPage.backup.table.id') }}</th>
              <th>{{ t('runtime.systemPage.backup.table.filename') }}</th>
              <th>{{ t('runtime.systemPage.backup.table.size') }}</th>
              <th>{{ t('runtime.systemPage.backup.table.status') }}</th>
              <th>{{ t('runtime.systemPage.backup.table.createdAt') }}</th>
              <th>{{ t('runtime.systemPage.backup.table.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="backup in backups" :key="backup.id">
              <td>{{ backup.id }}</td>
              <td>{{ backup.filename }}</td>
              <td>{{ formatSize(backup.size) }}</td>
              <td>
                <span :class="['status-badge', 'status-' + backup.status]">
                  {{ getStatusLabel(backup.status) }}
                </span>
              </td>
              <td>{{ formatTime(backup.created_at) }}</td>
              <td>
                <div class="action-buttons">
                  <button class="btn-sm btn-ghost" @click="restoreBackupRequest(backup)" :disabled="backup.status !== 'completed'">{{ t('runtime.systemPage.actions.restore') }}</button>
                  <button class="btn-sm btn-ghost" @click="deleteBackupRequest(backup)">{{ t('common.actions.delete') }}</button>
                </div>
              </td>
            </tr>
            <tr v-if="backups.length === 0">
              <td colspan="6" class="empty-row">{{ t('runtime.systemPage.backup.empty') }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Load balancing -->
    <div v-show="activeTab === 'balancer'">
      <div class="toolbar">
        <button class="btn-primary" @click="openBalancerModal()">{{ t('runtime.systemPage.actions.createBalancer') }}</button>
      </div>

      <div class="table-container">
        <table class="data-table">
          <thead>
            <tr>
              <th>{{ t('runtime.systemPage.balancer.table.id') }}</th>
              <th>{{ t('runtime.systemPage.balancer.table.name') }}</th>
              <th>{{ t('runtime.systemPage.balancer.table.group') }}</th>
              <th>{{ t('runtime.systemPage.balancer.table.strategy') }}</th>
              <th>{{ t('runtime.systemPage.balancer.table.healthCheck') }}</th>
              <th>{{ t('runtime.systemPage.balancer.table.enabled') }}</th>
              <th>{{ t('runtime.systemPage.balancer.table.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="lb in balancers" :key="lb.id">
              <td>{{ lb.id }}</td>
              <td>{{ lb.name }}</td>
              <td>{{ lb.group_name || '-' }}</td>
              <td>
                <span class="strategy-badge">{{ getStrategyLabel(lb.strategy) }}</span>
              </td>
              <td>
                <span :class="['status-badge', lb.health_check ? 'status-active' : 'status-disabled']">
                  {{ lb.health_check ? t('runtime.systemPage.booleans.enabled') : t('runtime.systemPage.booleans.disabled') }}
                </span>
              </td>
              <td>
                <span :class="['status-badge', lb.enabled ? 'status-active' : 'status-disabled']">
                  {{ lb.enabled ? t('runtime.systemPage.booleans.enabled') : t('runtime.systemPage.booleans.disabled') }}
                </span>
              </td>
              <td>
                <div class="action-buttons">
                  <button class="btn-sm btn-ghost" @click="runHealthCheckRequest(lb)">{{ t('runtime.systemPage.actions.healthCheck') }}</button>
                  <button class="btn-sm btn-ghost" @click="openBalancerModal(lb)">{{ t('common.actions.edit') }}</button>
                  <button class="btn-sm btn-ghost" @click="deleteBalancer(lb)">{{ t('common.actions.delete') }}</button>
                </div>
              </td>
            </tr>
            <tr v-if="balancers.length === 0">
              <td colspan="7" class="empty-row">{{ t('runtime.systemPage.balancer.empty') }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Audit logs -->
    <div v-show="activeTab === 'audit'">
      <div class="audit-toolbar">
        <h3>{{ t('runtime.systemPage.audit.title') }}</h3>
        <div class="audit-toolbar-actions">
          <input
            v-model="auditFilters.action"
            type="text"
            class="search-input"
            :placeholder="t('runtime.systemPage.audit.filters.actionPlaceholder')"
            @keyup.enter="applyAuditFilters"
          />
          <input
            v-model="auditFilters.target_type"
            type="text"
            class="search-input"
            :placeholder="t('runtime.systemPage.audit.filters.targetTypePlaceholder')"
            @keyup.enter="applyAuditFilters"
          />
          <button class="btn-secondary" :disabled="auditLoading" @click="applyAuditFilters">
            {{ t('runtime.systemPage.audit.actions.filter') }}
          </button>
          <button class="btn-primary" :disabled="auditLoading" @click="refreshAuditLogs">
            {{ t('runtime.systemPage.audit.actions.refresh') }}
          </button>
        </div>
      </div>

      <div class="table-container">
        <table class="data-table">
          <thead>
            <tr>
              <th>{{ t('runtime.systemPage.audit.table.id') }}</th>
              <th>{{ t('runtime.systemPage.audit.table.action') }}</th>
              <th>{{ t('runtime.systemPage.audit.table.module') }}</th>
              <th>{{ t('runtime.systemPage.audit.table.targetType') }}</th>
              <th>{{ t('runtime.systemPage.audit.table.username') }}</th>
              <th>{{ t('runtime.systemPage.audit.table.content') }}</th>
              <th>{{ t('runtime.systemPage.audit.table.ip') }}</th>
              <th>{{ t('runtime.systemPage.audit.table.status') }}</th>
              <th>{{ t('runtime.systemPage.audit.table.createdAt') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="log in auditLogs" :key="log.id">
              <td>{{ log.id ?? '-' }}</td>
              <td>{{ log.action || '-' }}</td>
              <td>{{ log.module || '-' }}</td>
              <td>{{ log.target_type || '-' }}</td>
              <td>{{ log.username || '-' }}</td>
              <td class="audit-content-cell">{{ log.content || '-' }}</td>
              <td>{{ log.ip || '-' }}</td>
              <td>{{ log.status || '-' }}</td>
              <td>{{ formatTime(log.created_at) }}</td>
            </tr>
            <tr v-if="!auditLoading && auditLogs.length === 0">
              <td colspan="9" class="empty-row">{{ t('runtime.systemPage.audit.empty') }}</td>
            </tr>
            <tr v-if="auditLoading">
              <td colspan="9" class="empty-row">{{ t('runtime.shared.loading') }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <div class="audit-pagination">
        <div class="text-secondary">
          {{ t('runtime.systemPage.audit.pagination.total', { total: auditTotal }) }}
        </div>
        <div class="audit-pagination-controls">
          <label>
            {{ t('runtime.systemPage.audit.pagination.pageSize') }}
            <select v-model.number="auditFilters.page_size" @change="changeAuditPageSize">
              <option v-for="size in auditPageSizeOptions" :key="size" :value="size">{{ size }}</option>
            </select>
          </label>
          <button class="btn-sm btn-ghost" :disabled="auditLoading || !auditHasPrev" @click="changeAuditPage(auditFilters.page - 1)">
            {{ t('runtime.systemPage.audit.pagination.prev') }}
          </button>
          <span>{{ t('runtime.systemPage.audit.pagination.page', { page: auditFilters.page, totalPages: auditTotalPages }) }}</span>
          <button class="btn-sm btn-ghost" :disabled="auditLoading || !auditHasNext" @click="changeAuditPage(auditFilters.page + 1)">
            {{ t('runtime.systemPage.audit.pagination.next') }}
          </button>
        </div>
      </div>
    </div>

    <!-- Config modal -->
    <div v-if="showConfigModal" class="modal-overlay" @click.self="showConfigModal = false">
      <div class="modal">
        <div class="modal-header">
          <h3>{{ editingConfig ? t('runtime.systemPage.configModal.titleEdit') : t('runtime.systemPage.configModal.titleCreate') }}</h3>
          <button class="close-btn" :aria-label="t('common.actions.close')" :title="t('common.actions.close')" @click="showConfigModal = false">x</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label>{{ t('runtime.systemPage.configModal.key') }} <span class="required">*</span></label>
            <input v-model="configForm.key" type="text" :placeholder="t('runtime.systemPage.configModal.keyPlaceholder')" :disabled="!!editingConfig" />
          </div>
          <div class="form-group">
            <label>{{ t('runtime.systemPage.configModal.value') }}</label>
            <textarea v-model="configForm.value" rows="3" :placeholder="t('runtime.systemPage.configModal.valuePlaceholder')"></textarea>
            <p v-if="editingConfig && configForm.sensitive" class="text-secondary">
              {{
                configForm.has_value
                  ? t('runtime.systemPage.configModal.sensitiveHintWithValue')
                  : t('runtime.systemPage.configModal.sensitiveHintWithoutValue')
              }}
            </p>
          </div>
          <div class="form-group">
            <label>{{ t('runtime.systemPage.configModal.description') }}</label>
            <input v-model="configForm.description" type="text" :placeholder="t('runtime.systemPage.configModal.descriptionPlaceholder')" />
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn-secondary" @click="showConfigModal = false">{{ t('common.actions.cancel') }}</button>
          <button @click="saveConfig">{{ t('common.actions.save') }}</button>
        </div>
      </div>
    </div>

    <!-- Balancer modal -->
    <div v-if="showBalancerModal" class="modal-overlay" @click.self="showBalancerModal = false">
      <div class="modal">
        <div class="modal-header">
          <h3>{{ editingBalancer ? t('runtime.systemPage.balancerModal.titleEdit') : t('runtime.systemPage.balancerModal.titleCreate') }}</h3>
          <button class="close-btn" :aria-label="t('common.actions.close')" :title="t('common.actions.close')" @click="showBalancerModal = false">x</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label>{{ t('runtime.systemPage.balancerModal.name') }} <span class="required">*</span></label>
            <input v-model="balancerForm.name" type="text" :placeholder="t('runtime.systemPage.balancerModal.namePlaceholder')" />
          </div>
          <div class="form-row">
            <div class="form-group">
              <label>{{ t('runtime.systemPage.balancerModal.groupId') }}</label>
              <input v-model.number="balancerForm.group_id" type="number" />
            </div>
            <div class="form-group">
              <label>{{ t('runtime.systemPage.balancerModal.strategy') }}</label>
              <select v-model="balancerForm.strategy">
                <option value="round-robin">{{ t('runtime.systemPage.strategy.roundRobin') }}</option>
                <option value="least-load">{{ t('runtime.systemPage.strategy.leastLoad') }}</option>
                <option value="latency">{{ t('runtime.systemPage.strategy.latency') }}</option>
                <option value="weight">{{ t('runtime.systemPage.strategy.weight') }}</option>
                <option value="random">{{ t('runtime.systemPage.strategy.random') }}</option>
              </select>
            </div>
          </div>
          <div class="form-row">
            <div class="form-group">
              <label class="checkbox-label">
                <input type="checkbox" v-model="balancerForm.health_check" />
                <span>{{ t('runtime.systemPage.balancerModal.healthCheck') }}</span>
              </label>
            </div>
            <div class="form-group">
              <label>{{ t('runtime.systemPage.balancerModal.checkInterval') }}</label>
              <input v-model.number="balancerForm.check_interval" type="number" min="10" />
            </div>
          </div>
          <div class="form-group">
            <label>{{ t('runtime.systemPage.balancerModal.weightsJson') }}</label>
            <textarea v-model="balancerForm.weights_json" rows="3" :placeholder="t('runtime.systemPage.balancerModal.weightsPlaceholder')"></textarea>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn-secondary" @click="showBalancerModal = false">{{ t('common.actions.cancel') }}</button>
          <button @click="saveBalancer">{{ t('common.actions.save') }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import {
  getSystemConfigs, getSystemConfig, getSubscriptionSettings, setSystemConfig, deleteSystemConfig,
  getSystemAuditLogs,
  getBackupConfig, updateBackupConfig, createBackup, getBackups,
  getBackupStats, deleteBackup, restoreBackup,
  getLoadBalancers, createLoadBalancer, updateLoadBalancer,
  deleteLoadBalancer, runHealthCheck, listForwardRuntimeJobs, getForwardRuntimeStatus, runForwardRuntimeDoctor
} from '@/api/admin'
import { humanizeForwardRuntimeBackend } from '@/utils/forwardRuntime'

const { t, formatDateTime, translateLiteral } = useAppI18n()

const activeTab = ref('config')
const configSearch = ref('')
const subscriptionDomainsConfigKey = 'app.subscribe_domains'
const subscriptionPath = ref('/s')
const subscriptionCurrentHost = ref(typeof window !== 'undefined' ? window.location.host : '')
const subscriptionDomainsText = ref('')
const subscriptionSettingsLoading = ref(false)
const subscriptionSettingsSaving = ref(false)
const subscriptionDomainError = ref('')
const configs = ref([])
const backups = ref([])
const balancers = ref([])
const auditLogs = ref([])
const auditLoading = ref(false)
const auditTotal = ref(0)
const auditFilters = ref({
  action: '',
  target_type: '',
  page: 1,
  page_size: 20
})
const auditPageSizeOptions = [20, 50, 100]

const createBackupConfigForm = (source = {}) => ({
  enabled: Boolean(source.enabled),
  interval: Number.isFinite(Number(source.interval)) && Number(source.interval) > 0
    ? Number(source.interval)
    : Number.isFinite(Number(source.retention_days)) && Number(source.retention_days) > 0
      ? Number(source.retention_days)
      : 24,
  keep_count: Number.isFinite(Number(source.keep_count)) && Number(source.keep_count) > 0
    ? Number(source.keep_count)
    : Number.isFinite(Number(source.retention_days)) && Number(source.retention_days) > 0
      ? Number(source.retention_days)
      : 7,
  backup_database: source.backup_database !== false,
  backup_files: Boolean(source.backup_files),
  storage_type: String(source.storage_type || 'local').trim().toLowerCase() || 'local',
  storage_path: String(source.storage_path || 'backups'),
  s3_bucket: String(source.s3_bucket || ''),
  s3_region: String(source.s3_region || ''),
  s3_endpoint: String(source.s3_endpoint || ''),
  s3_access_key: Boolean(source.s3_access_key_sensitive)
    ? ''
    : String(source.s3_access_key || ''),
  s3_secret_key: Boolean(source.s3_secret_key_sensitive)
    ? ''
    : String(source.s3_secret_key || ''),
  s3_access_key_display_value: String(source.s3_access_key_display_value || ''),
  s3_secret_key_display_value: String(source.s3_secret_key_display_value || ''),
  s3_access_key_sensitive: Boolean(source.s3_access_key_sensitive),
  s3_secret_key_sensitive: Boolean(source.s3_secret_key_sensitive),
  s3_access_key_has_value: Boolean(source.s3_access_key_has_value),
  s3_secret_key_has_value: Boolean(source.s3_secret_key_has_value)
})

const backupConfig = ref(createBackupConfigForm())

const backupStats = ref({})

const showConfigModal = ref(false)
const editingConfig = ref(null)
const configForm = ref({
  key: '',
  value: '',
  description: '',
  type: 'string',
  group: '',
  sensitive: false,
  has_value: false
})

const showBalancerModal = ref(false)
const editingBalancer = ref(null)
const balancerForm = ref({
  name: '', group_id: 0, strategy: 'round-robin',
  health_check: true, check_interval: 60, weights_json: ''
})

const runtimeNodeXModeKey = 'forward.runtime.nodex_mode'
const runtimeBackendKey = 'forward.runtime_backend'
const runtimeAnsibleBackendKey = 'forward.runtime.ansible.backend'
const runtimeAnsibleConfigKey = 'forward.runtime.ansible.config'
const runtimeLegacyAnsibleConfigKey = 'forward.runtime.iptables_ansible.config'
const runtimeAnsibleInventoryKey = 'forward.runtime.ansible.inventory'
const runtimeAnsibleApplyPlaybookKey = 'forward.runtime.ansible.apply_playbook'
const runtimeAnsibleRemovePlaybookKey = 'forward.runtime.ansible.remove_playbook'
const runtimeAnsibleBecomeKey = 'forward.runtime.ansible.become'
const runtimeAnsibleExtraVarsKey = 'forward.runtime.ansible.extra_vars_json'
const runtimeLegacyAnsibleInventoryKey = 'forward.ansible.inventory'
const runtimeLegacyAnsibleApplyPlaybookKey = 'forward.ansible.playbook_apply'
const runtimeLegacyAnsibleRemovePlaybookKey = 'forward.ansible.playbook_remove'
const runtimeLegacyAnsibleBecomeKey = 'forward.ansible.become'
const runtimeLegacyAnsibleExtraVarsKey = 'forward.ansible.extra_vars_json'
const runtimeNodeXBaseUrlKey = 'forward.runtime.nodex.base_url'
const runtimeNodeXTokenKey = 'forward.runtime.nodex.token'
const runtimeNodeXTimeoutKey = 'forward.runtime.nodex.timeout_seconds'
const defaultRuntimeAnsibleConfig = Object.freeze({
  inventory: 'config/deploy/ansible/inventory.ini',
  playbookApply: 'config/deploy/ansible/playbooks/forward_apply_nftables.yml',
  playbookRemove: 'config/deploy/ansible/playbooks/forward_remove_nftables.yml',
  command: 'ansible-playbook',
  workingDir: 'config/deploy/ansible',
  targetPattern: '{{node.host}}',
  timeoutSeconds: 120,
  ansibleConfig: 'config/deploy/ansible/ansible.cfg',
  become: false
})
const runtimeBackend = ref('nftables_ansible')
const runtimeNodeXMode = ref(false)
const runtimeNodeXBaseUrl = ref('')
const runtimeNodeXToken = ref('')
const runtimeNodeXTimeout = ref(15)
const runtimeAnsibleForm = ref(createRuntimeAnsibleForm())
const runtimeSaving = ref(false)
const runtimeValidationError = ref('')
const runtimeJobs = ref([])
const runtimeJobsLoading = ref(false)
const runtimeStatus = ref(null)
const runtimeStatusLoading = ref(false)
const runtimeStatusError = ref('')
const runtimeDoctorOutput = ref('')
const runtimeDoctorRunning = ref(false)
const runtimeDoctorSummary = ref(null)

const defaultNodeXBaseUrl = 'http://127.0.0.1:18081'
const runtimeOperatorBaseUrl = computed(() => runtimeNodeXBaseUrl.value?.trim() || defaultNodeXBaseUrl)
const runtimeOperatorToken = computed(() => runtimeNodeXToken.value?.trim() || '<FORWARD_API_TOKEN>')
const runtimeOperatorReferences = computed(() => ([
  t('runtime.workbench.references.panelRuntimeDoc'),
  t('runtime.workbench.references.panelRelayOnboarding'),
  t('runtime.workbench.references.nodeXRepo'),
  t('runtime.workbench.references.panelNodeXOnboarding')
]))
const runtimeDisplayedCommands = computed(() => {
  const source = runtimeDoctorSummary.value?.commands
  if (source) {
    return {
      powerShell: Array.isArray(source.powerShell) ? source.powerShell : [],
      bash: Array.isArray(source.bash) ? source.bash : [],
      upgrade: Array.isArray(source.upgrade) ? source.upgrade : [],
      references: Array.isArray(source.references) ? source.references : []
    }
  }

  if (runtimeNodeXMode.value) {
    return {
      powerShell: [
        `Invoke-WebRequest '${runtimeOperatorBaseUrl.value}/health' | Select-Object -ExpandProperty Content`,
        `Invoke-WebRequest '${runtimeOperatorBaseUrl.value}/api/v2/internal/forward/runtime/status' -Headers @{ Authorization = 'Bearer ${runtimeOperatorToken.value}' } | Select-Object -ExpandProperty Content`
      ],
      bash: [
        `curl -fsSL '${runtimeOperatorBaseUrl.value}/health'`,
        `curl -fsSL -H 'Authorization: Bearer ${runtimeOperatorToken.value}' '${runtimeOperatorBaseUrl.value}/api/v2/internal/forward/runtime/status'`
      ],
      upgrade: [
        'git clone https://github.com/zdwtest/NodeX.git',
        'cd NodeX/control-plane && go run ./cmd/control-plane --version',
        'cd NodeX/control-plane && go run ./cmd/control-plane --config ../deploy/config/control-plane.yaml --addr :18081 --forward-api-token <FORWARD_API_TOKEN>'
      ],
      references: runtimeOperatorReferences.value
    }
  }

  return {
    powerShell: ['Get-Command ansible-playbook', 'ansible-playbook --version'],
    bash: ['command -v ansible-playbook', 'ansible-playbook --version'],
    upgrade: ['git pull --ff-only', 'go test ./internal/service/... -run ForwardRuntime'],
    references: [
      'docs/guide/forward-relay-onboarding.md',
      'docs/guide/forward-tunnel-runtime-ops.md',
      'docs/guide/forward-tunnel-smoke-test.md'
    ]
  }
})

function parseRuntimeBoolean(value) {
  if (typeof value === 'boolean') {
    return value
  }

  if (typeof value === 'number') {
    return value !== 0
  }

  const normalized = String(value ?? '').trim().toLowerCase()
  if (!normalized) {
    return null
  }
  if (['1', 'true', 'yes', 'on'].includes(normalized)) {
    return true
  }
  if (['0', 'false', 'no', 'off'].includes(normalized)) {
    return false
  }
  return null
}

function normalizeJsonObjectText(value) {
  const trimmed = String(value ?? '').trim()
  if (!trimmed) {
    return ''
  }

  try {
    const parsed = JSON.parse(trimmed)
    if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') {
      return trimmed
    }
    return JSON.stringify(parsed, null, 2)
  } catch {
    return trimmed
  }
}

function createRuntimeAnsibleForm(source = {}) {
  const extraVars = source.extraVars && typeof source.extraVars === 'object' && !Array.isArray(source.extraVars)
    ? source.extraVars
    : {}
  const rawEnvironment = source.environment && typeof source.environment === 'object' && !Array.isArray(source.environment)
    ? source.environment
    : {}
  const environment = { ...rawEnvironment }
  const ansibleConfig = String(source.ansibleConfig ?? environment.ANSIBLE_CONFIG ?? defaultRuntimeAnsibleConfig.ansibleConfig).trim() ||
    defaultRuntimeAnsibleConfig.ansibleConfig
  delete environment.ANSIBLE_CONFIG

  return {
    inventory: String(source.inventory ?? defaultRuntimeAnsibleConfig.inventory).trim() || defaultRuntimeAnsibleConfig.inventory,
    playbookApply: String(source.playbookApply ?? source.applyPlaybook ?? defaultRuntimeAnsibleConfig.playbookApply).trim() || defaultRuntimeAnsibleConfig.playbookApply,
    playbookRemove: String(source.playbookRemove ?? source.removePlaybook ?? defaultRuntimeAnsibleConfig.playbookRemove).trim() || defaultRuntimeAnsibleConfig.playbookRemove,
    command: String(source.command ?? defaultRuntimeAnsibleConfig.command).trim() || defaultRuntimeAnsibleConfig.command,
    workingDir: String(source.workingDir ?? defaultRuntimeAnsibleConfig.workingDir).trim() || defaultRuntimeAnsibleConfig.workingDir,
    targetPattern: String(source.targetPattern ?? defaultRuntimeAnsibleConfig.targetPattern).trim() || defaultRuntimeAnsibleConfig.targetPattern,
    timeoutSeconds: Number.isFinite(Number(source.timeoutSeconds)) && Number(source.timeoutSeconds) > 0
      ? Number(source.timeoutSeconds)
      : defaultRuntimeAnsibleConfig.timeoutSeconds,
    ansibleConfig,
    become: parseRuntimeBoolean(source.become) ?? defaultRuntimeAnsibleConfig.become,
    extraVarsJson: Object.keys(extraVars).length ? JSON.stringify(extraVars, null, 2) : '',
    environmentJson: Object.keys(environment).length ? JSON.stringify(environment, null, 2) : ''
  }
}

function normalizeRuntimeAnsibleForm(source = {}) {
  const form = createRuntimeAnsibleForm(source)
  return {
    ...form,
    extraVarsJson: normalizeJsonObjectText(source.extraVarsJson ?? form.extraVarsJson),
    environmentJson: normalizeJsonObjectText(source.environmentJson ?? form.environmentJson)
  }
}

function parseRuntimeJsonObject(value, label) {
  const trimmed = String(value ?? '').trim()
  if (!trimmed) {
    return {}
  }

  let parsed
  try {
    parsed = JSON.parse(trimmed)
  } catch {
    throw new Error(t('runtime.localRuntime.errors.invalidJson', { label }))
  }

  if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') {
    throw new Error(t('runtime.localRuntime.errors.invalidObject', { label }))
  }

  return parsed
}

function buildRuntimeAnsiblePayload(source = runtimeAnsibleForm.value) {
  const form = normalizeRuntimeAnsibleForm(source)
  const extraVars = parseRuntimeJsonObject(form.extraVarsJson, t('runtime.localRuntime.fields.extraVarsJson'))
  const environmentOverrides = parseRuntimeJsonObject(form.environmentJson, t('runtime.localRuntime.fields.environmentJson'))
  const environment = {}

  if (form.ansibleConfig) {
    environment.ANSIBLE_CONFIG = form.ansibleConfig
  }

  Object.entries(environmentOverrides).forEach(([key, value]) => {
    const normalizedKey = String(key ?? '').trim()
    if (!normalizedKey) {
      return
    }
    environment[normalizedKey] = value == null ? '' : String(value)
  })

  const normalizedExtraVars = {}
  Object.entries(extraVars).forEach(([key, value]) => {
    const normalizedKey = String(key ?? '').trim()
    if (!normalizedKey) {
      return
    }
    normalizedExtraVars[normalizedKey] = value
  })

  return {
    inventory: form.inventory,
    playbookApply: form.playbookApply,
    playbookRemove: form.playbookRemove,
    become: !!form.become,
    extraVars: normalizedExtraVars,
    command: form.command,
    workingDir: form.workingDir,
    targetPattern: form.targetPattern,
    environment,
    timeoutSeconds: Number.isFinite(Number(form.timeoutSeconds)) && Number(form.timeoutSeconds) > 0
      ? Number(form.timeoutSeconds)
      : defaultRuntimeAnsibleConfig.timeoutSeconds
  }
}

const runtimeConfigPreview = computed(() => {
  try {
    return JSON.stringify(buildRuntimeAnsiblePayload(runtimeAnsibleForm.value), null, 2)
  } catch (err) {
    return t('runtime.localRuntime.errors.invalidPreview', { message: err.message })
  }
})

function applyDefaultRuntimeAnsibleConfig() {
  runtimeValidationError.value = ''
  runtimeAnsibleForm.value = createRuntimeAnsibleForm(defaultRuntimeAnsibleConfig)
}

const parseSubscriptionDomainInput = (value) => {
  const text = String(value || '').trim()
  if (!text) return []
  const normalized = text.replace(/\r\n/g, '\n').replace(/\r/g, '\n').replace(/,/g, '\n').replace(/;/g, '\n')
  const lines = normalized.split('\n').map(item => item.trim()).filter(Boolean)
  const unique = []
  const seen = new Set()
  for (const line of lines) {
    let host = line
    try {
      const url = new URL(line.includes('://') ? line : `https://${line}`)
      host = url.host || line
    } catch {
      host = line
    }
    host = host.trim().replace(/\/+$/, '')
    if (!host || seen.has(host)) continue
    seen.add(host)
    unique.push(host)
  }
  return unique
}

const normalizedSubscriptionDomains = computed(() => parseSubscriptionDomainInput(subscriptionDomainsText.value))
const subscriptionPreviewProtocol = computed(() => {
  if (typeof window === 'undefined') return 'https:'
  return window.location.protocol || 'https:'
})

const readSubscriptionSettings = (res) => {
  if (!res || typeof res !== 'object') return { subscribe_path: '/s', subscribe_domains: [] }
  const payload = Object.prototype.hasOwnProperty.call(res, 'code') ? res.data : (res.data ?? res)
  if (!payload || typeof payload !== 'object') return { subscribe_path: '/s', subscribe_domains: [] }
  return {
    subscribe_path: payload.subscribe_path || '/s',
    subscribe_domains: Array.isArray(payload.subscribe_domains) ? payload.subscribe_domains : []
  }
}

const filteredConfigs = computed(() => {
  if (!configSearch.value) return configs.value
  const search = configSearch.value.toLowerCase()
  return configs.value.filter(c =>
    c.key?.toLowerCase().includes(search) ||
    c.description?.toLowerCase().includes(search)
  )
})
const auditTotalPages = computed(() => {
  const pageSize = Number(auditFilters.value.page_size) || 20
  const total = Number(auditTotal.value) || 0
  return Math.max(1, Math.ceil(total / pageSize))
})
const auditHasPrev = computed(() => Number(auditFilters.value.page) > 1)
const auditHasNext = computed(() => Number(auditFilters.value.page) < auditTotalPages.value)

const getStrategyLabel = (strategy) => {
  switch (strategy) {
    case 'round-robin':
      return t('runtime.systemPage.strategy.roundRobin')
    case 'least-load':
      return t('runtime.systemPage.strategy.leastLoad')
    case 'latency':
      return t('runtime.systemPage.strategy.latency')
    case 'weight':
      return t('runtime.systemPage.strategy.weight')
    case 'random':
      return t('runtime.systemPage.strategy.random')
    default:
      return strategy
  }
}

const getStatusLabel = (status) => {
  switch (status) {
    case 'pending':
      return t('runtime.systemPage.status.pending')
    case 'completed':
      return t('runtime.systemPage.status.completed')
    case 'failed':
      return t('runtime.systemPage.status.failed')
    default:
      return status
  }
}

const formatTime = (time) => {
  if (!time) return '-'
  return formatDateTime(time) || String(time)
}

const notify = (message) => {
  if (typeof window !== 'undefined' && typeof window.alert === 'function') {
    window.alert(message)
    return
  }
  console.warn(message)
}
const confirmAction = (message) => window.confirm(message)
const resolveSystemError = (error, fallbackKey) => (
  translateRuntimeText(error?.response?.data?.msg || error?.response?.data?.error || error?.msg || error?.message, t(fallbackKey))
)

const readPanelEnvelopeError = (res) => {
  const candidates = [res, res?.data]
  for (const candidate of candidates) {
    if (!candidate || typeof candidate !== 'object') continue
    if (!Object.prototype.hasOwnProperty.call(candidate, 'code')) continue
    if (Number(candidate.code) === 0) return null
    return candidate.msg || candidate.error || ''
  }
  return null
}

const ensureSystemSuccess = (res, fallbackKey) => {
  const message = readPanelEnvelopeError(res)
  if (message !== null) {
    throw new Error(message || t(fallbackKey))
  }
  return res
}

const readSystemPayload = (res, fallbackKey) => {
  const payload = ensureSystemSuccess(res, fallbackKey)
  if (!payload || typeof payload !== 'object') return {}
  if (Object.prototype.hasOwnProperty.call(payload, 'code')) {
    return payload.data && typeof payload.data === 'object' ? payload.data : {}
  }
  if (payload.data && typeof payload.data === 'object') {
    if (Object.prototype.hasOwnProperty.call(payload.data, 'code')) {
      return payload.data.data && typeof payload.data.data === 'object' ? payload.data.data : {}
    }
    return payload.data.data && typeof payload.data.data === 'object' ? payload.data.data : payload.data
  }
  return payload
}

const ensureSystemMutation = async (promise, fallbackKey) => ensureSystemSuccess(await promise, fallbackKey)

const formatSize = (bytes) => {
  if (!bytes) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  let i = 0
  while (bytes >= 1024 && i < units.length - 1) {
    bytes /= 1024
    i++
  }
  return bytes.toFixed(2) + ' ' + units[i]
}

const truncateValue = (value) => {
  if (!value) return '-'
  const str = String(value)
  return str.length > 50 ? str.substring(0, 50) + '...' : str
}

const getConfigDisplayValue = (config) => {
  if (!config || typeof config !== 'object') return ''
  return config.display_value ?? config.value
}

const normalizeRuntimeJob = (raw) => ({
  ...raw,
  id: Number(raw.id),
  status: Number(raw.status ?? 0),
  forwardId: raw.forwardId ?? raw.forward_id ?? null,
  tunnelId: raw.tunnelId ?? raw.tunnel_id ?? null,
  nodeId: raw.nodeId ?? raw.node_id ?? null,
  createdAt: raw.createdAt ?? raw.created_at ?? null,
  updatedAt: raw.updatedAt ?? raw.updated_at ?? null,
  startedAt: raw.startedAt ?? raw.started_at ?? null,
  completedAt: raw.completedAt ?? raw.completed_at ?? null
})

const translateRuntimeText = (value, fallback = '-') => {
  const text = String(value ?? '').trim()
  if (!text) return fallback
  return translateLiteral(text)
}

const resolveRuntimeError = (error, fallbackKey) => (
  translateRuntimeText(error?.response?.data?.msg || error?.message, t(fallbackKey))
)

const runtimeBackendLabel = (value) => {
  return humanizeForwardRuntimeBackend(t, value)
}

const getRuntimeJobStatusLabel = (status) => {
  switch (Number(status)) {
    case 0:
      return t('runtime.shared.pending')
    case 1:
      return t('runtime.shared.running')
    case 2:
      return t('runtime.shared.success')
    case 3:
      return t('runtime.shared.failed')
    default:
      return t('runtime.shared.unknown')
  }
}

const formatRuntimeJobTime = (job) => {
  const value = job.completedAt || job.startedAt || job.updatedAt || job.createdAt
  return value ? (formatDateTime(value) || String(value)) : '-'
}

const formatRuntimeJobMessage = (job) => {
  const source = job.error || job.result || job.payload || ''
  const text = String(source).trim()
  if (!text) return ''
  const translated = translateLiteral(text)
  return translated.length > 220 ? `${translated.slice(0, 217)}...` : translated
}

const fetchForwardRuntimeJobs = async () => {
  runtimeJobsLoading.value = true
  try {
    const res = await listForwardRuntimeJobs({ limit: 10 })
    runtimeJobs.value = Array.isArray(res.data?.list) ? res.data.list.map(normalizeRuntimeJob) : []
  } catch (err) {
    console.error('get forward runtime jobs failed:', err)
    runtimeJobs.value = []
  } finally {
    runtimeJobsLoading.value = false
  }
}

const fetchRuntimeStatusSafe = async () => {
  runtimeStatusLoading.value = true
  runtimeStatusError.value = ''
  try {
    const res = await getForwardRuntimeStatus()
    runtimeStatus.value = res.data?.data || res.data || null
    runtimeDoctorSummary.value = null
  } catch (err) {
    runtimeStatusError.value = resolveRuntimeError(err, 'runtime.workbench.errors.fetchStatusFailed')
    runtimeStatus.value = null
  } finally {
    runtimeStatusLoading.value = false
  }
}

const runRuntimeDoctorCheckSafe = async () => {
  runtimeDoctorRunning.value = true
  runtimeDoctorOutput.value = ''
  try {
    const res = await runForwardRuntimeDoctor()
    const payload = res.data?.data || res.data || res
    runtimeDoctorSummary.value = payload
    runtimeDoctorOutput.value = JSON.stringify(payload, null, 2)
  } catch (err) {
    runtimeDoctorOutput.value = resolveRuntimeError(err, 'runtime.workbench.errors.doctorFailed')
  } finally {
    runtimeDoctorRunning.value = false
  }
}

const fetchForwardRuntimeConfig = async () => {
  let explicitNodeXMode = null
  runtimeValidationError.value = ''
  runtimeAnsibleForm.value = createRuntimeAnsibleForm()
  try {
    const nodeXModeRes = await getSystemConfig(runtimeNodeXModeKey)
    explicitNodeXMode = parseRuntimeBoolean(nodeXModeRes.data?.value)
  } catch (err) {
    console.error('get forward runtime NodeX mode config failed:', err)
  }

  try {
    const backendRes = await getSystemConfig(runtimeBackendKey)
    const backendValue = String(backendRes.data?.value || 'nftables_ansible').trim().toLowerCase()
    runtimeBackend.value = backendValue || 'nftables_ansible'
  } catch (err) {
    console.error('get forward runtime backend config failed:', err)
  }
  runtimeNodeXMode.value = explicitNodeXMode === null
    ? runtimeBackend.value === 'gost'
    : explicitNodeXMode
  if (runtimeNodeXMode.value) {
    runtimeBackend.value = 'gost'
  } else if (runtimeBackend.value === 'gost') {
    try {
      const localBackendRes = await getSystemConfig(runtimeAnsibleBackendKey)
      runtimeBackend.value = String(localBackendRes.data?.value || 'nftables_ansible').trim().toLowerCase() || 'nftables_ansible'
    } catch {
      runtimeBackend.value = 'nftables_ansible'
    }
  }

  try {
    const configRes = await getSystemConfig(runtimeAnsibleConfigKey)
    let rawValue = configRes.data?.value || ''
    if (!rawValue) {
      try {
        const legacyConfigRes = await getSystemConfig(runtimeLegacyAnsibleConfigKey)
        rawValue = legacyConfigRes.data?.value || ''
      } catch {
        rawValue = ''
      }
    }
    if (rawValue) {
      try {
        const parsed = JSON.parse(rawValue)
        if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') {
          throw new Error('saved payload must be a JSON object')
        }
        runtimeAnsibleForm.value = normalizeRuntimeAnsibleForm(parsed)
      } catch {
        runtimeValidationError.value = t('runtime.workbench.errors.savedConfigInvalid')
        runtimeAnsibleForm.value = createRuntimeAnsibleForm()
      }
    }
  } catch (err) {
    console.error('get forward runtime ansible config failed:', err)
  }
  try {
    const baseUrlRes = await getSystemConfig(runtimeNodeXBaseUrlKey)
    runtimeNodeXBaseUrl.value = baseUrlRes.data?.value || ''
  } catch (err) {
    console.error('get forward runtime NodeX base URL config failed:', err)
  }
  try {
    const tokenRes = await getSystemConfig(runtimeNodeXTokenKey)
    runtimeNodeXToken.value = tokenRes.data?.value || ''
  } catch (err) {
    console.error('get forward runtime NodeX token config failed:', err)
  }
  try {
    const timeoutRes = await getSystemConfig(runtimeNodeXTimeoutKey)
    const timeoutValue = Number(timeoutRes.data?.value)
    runtimeNodeXTimeout.value = Number.isFinite(timeoutValue) && timeoutValue > 0 ? timeoutValue : 15
  } catch (err) {
    console.error('get forward runtime NodeX timeout config failed:', err)
  }
}

const saveForwardRuntimeConfig = async () => {
  runtimeValidationError.value = ''
  const trimmedNodeXBaseUrl = runtimeNodeXBaseUrl.value?.trim() || ''
  const trimmedNodeXToken = runtimeNodeXToken.value?.trim() || ''

  if (runtimeNodeXMode.value) {
    if (!trimmedNodeXBaseUrl) {
      runtimeValidationError.value = t('runtime.workbench.errors.nodeXBaseUrlRequired')
      return
    }
    if (!trimmedNodeXToken) {
      runtimeValidationError.value = t('runtime.workbench.errors.nodeXTokenRequired')
      return
    }
  }

  let ansiblePayload = null
  if (!runtimeNodeXMode.value) {
    try {
      runtimeAnsibleForm.value = normalizeRuntimeAnsibleForm(runtimeAnsibleForm.value)
      ansiblePayload = buildRuntimeAnsiblePayload(runtimeAnsibleForm.value)
    } catch {
      runtimeValidationError.value = t('runtime.workbench.errors.invalidRuntimeJson')
      return
    }
  }

  const backendValue = runtimeNodeXMode.value ? 'gost' : (runtimeBackend.value && runtimeBackend.value !== 'gost' ? runtimeBackend.value : 'nftables_ansible')
  runtimeBackend.value = backendValue
  const timeoutValue = Number(runtimeNodeXTimeout.value)

  runtimeSaving.value = true
  try {
    const updates = [
      ensureSystemMutation(setSystemConfig(runtimeNodeXModeKey, {
        value: runtimeNodeXMode.value,
        type: 'bool',
        group: 'forward',
        description: 'Enable NodeX forward runtime mode'
      }), 'runtime.workbench.errors.saveFailed'),
      ensureSystemMutation(setSystemConfig(runtimeBackendKey, {
        value: backendValue,
        type: 'string',
        group: 'forward',
        description: 'Forward runtime backend'
      }), 'runtime.workbench.errors.saveFailed'),
      ensureSystemMutation(setSystemConfig(runtimeAnsibleBackendKey, {
        value: backendValue === 'gost' ? 'nftables_ansible' : backendValue,
        type: 'string',
        group: 'forward',
        description: 'Preferred local ansible backend'
      }), 'runtime.workbench.errors.saveFailed'),
      ensureSystemMutation(setSystemConfig(runtimeNodeXBaseUrlKey, {
        value: trimmedNodeXBaseUrl,
        type: 'string',
        group: 'forward',
        description: 'Forward runtime NodeX base URL'
      }), 'runtime.workbench.errors.saveFailed'),
      ensureSystemMutation(setSystemConfig(runtimeNodeXTokenKey, {
        value: trimmedNodeXToken,
        type: 'string',
        group: 'forward',
        description: 'Forward runtime NodeX token'
      }), 'runtime.workbench.errors.saveFailed'),
      ensureSystemMutation(setSystemConfig(runtimeNodeXTimeoutKey, {
        value: Number.isFinite(timeoutValue) && timeoutValue > 0 ? timeoutValue : 15,
        type: 'number',
        group: 'forward',
        description: 'Forward runtime NodeX timeout'
      }), 'runtime.workbench.errors.saveFailed')
    ]

    if (!runtimeNodeXMode.value) {
      updates.push(
        ensureSystemMutation(setSystemConfig(runtimeAnsibleConfigKey, {
          value: ansiblePayload ? JSON.stringify(ansiblePayload) : '',
          type: 'json',
          group: 'forward',
          description: 'Forward runtime ansible config'
        }), 'runtime.workbench.errors.saveFailed'),
        ensureSystemMutation(setSystemConfig(runtimeAnsibleInventoryKey, {
          value: ansiblePayload?.inventory || '',
          type: 'string',
          group: 'forward',
          description: 'Forward ansible inventory path'
        }), 'runtime.workbench.errors.saveFailed'),
        ensureSystemMutation(setSystemConfig(runtimeAnsibleApplyPlaybookKey, {
          value: ansiblePayload?.playbookApply || '',
          type: 'string',
          group: 'forward',
          description: 'Forward ansible apply playbook path'
        }), 'runtime.workbench.errors.saveFailed'),
        ensureSystemMutation(setSystemConfig(runtimeAnsibleRemovePlaybookKey, {
          value: ansiblePayload?.playbookRemove || '',
          type: 'string',
          group: 'forward',
          description: 'Forward ansible remove playbook path'
        }), 'runtime.workbench.errors.saveFailed'),
        ensureSystemMutation(setSystemConfig(runtimeAnsibleBecomeKey, {
          value: ansiblePayload?.become || false,
          type: 'bool',
          group: 'forward',
          description: 'Forward ansible become flag'
        }), 'runtime.workbench.errors.saveFailed'),
        ensureSystemMutation(setSystemConfig(runtimeAnsibleExtraVarsKey, {
          value: JSON.stringify(ansiblePayload?.extraVars || {}),
          type: 'json',
          group: 'forward',
          description: 'Forward ansible extra vars JSON'
        }), 'runtime.workbench.errors.saveFailed')
      )
    }

    await Promise.all(updates)
    await fetchForwardRuntimeConfig()
    await fetchForwardRuntimeJobs()
    await fetchRuntimeStatusSafe()
    fetchConfigs()
  } catch (err) {
    runtimeValidationError.value = translateRuntimeText(
      err.response?.data?.error || err.message,
      t('runtime.workbench.errors.saveFailed')
    )
  } finally {
    runtimeSaving.value = false
  }
}

const loadSubscriptionDomainSettings = async () => {
  subscriptionSettingsLoading.value = true
  subscriptionDomainError.value = ''
  try {
    const [settingsRes, rawConfigRes] = await Promise.allSettled([
      getSubscriptionSettings(),
      getSystemConfig(subscriptionDomainsConfigKey)
    ])

    if (settingsRes.status === 'fulfilled') {
      const payload = readSubscriptionSettings(settingsRes.value)
      subscriptionPath.value = payload.subscribe_path || '/s'
    }

    if (rawConfigRes.status === 'fulfilled') {
      const rawValue = rawConfigRes.value.data?.value
      if (typeof rawValue === 'string' && rawValue.trim()) {
        try {
          const parsed = JSON.parse(rawValue)
          if (Array.isArray(parsed)) {
            subscriptionDomainsText.value = parsed.join('\n')
          } else {
            subscriptionDomainsText.value = rawValue
          }
        } catch {
          subscriptionDomainsText.value = rawValue
        }
      } else {
        const settingsPayload = settingsRes.status === 'fulfilled' ? readSubscriptionSettings(settingsRes.value) : {}
        const domains = Array.isArray(settingsPayload.subscribe_domains)
          ? settingsPayload.subscribe_domains
          : []
        subscriptionDomainsText.value = domains.join('\n')
      }
    } else if (settingsRes.status === 'fulfilled') {
      const settingsPayload = readSubscriptionSettings(settingsRes.value)
      const domains = Array.isArray(settingsPayload.subscribe_domains) ? settingsPayload.subscribe_domains : []
      subscriptionDomainsText.value = domains.join('\n')
    } else {
      throw rawConfigRes.reason || settingsRes.reason || new Error('failed to load subscription settings')
    }
  } catch (err) {
    subscriptionDomainError.value = resolveSystemError(err, 'runtime.systemPage.subscription.messages.loadFailed')
  } finally {
    subscriptionSettingsLoading.value = false
  }
}

const saveSubscriptionDomainSettings = async () => {
  subscriptionSettingsSaving.value = true
  subscriptionDomainError.value = ''
  try {
    const domains = normalizedSubscriptionDomains.value
    if (domains.length === 0) {
      try {
        await ensureSystemMutation(
          deleteSystemConfig(subscriptionDomainsConfigKey),
          'runtime.systemPage.subscription.messages.saveFailed'
        )
      } catch {
        await ensureSystemMutation(
          setSystemConfig(subscriptionDomainsConfigKey, {
            value: '',
            type: 'string',
            group: 'app',
            description: 'Alternate subscription domains'
          }),
          'runtime.systemPage.subscription.messages.saveFailed'
        )
      }
    } else {
      await ensureSystemMutation(
        setSystemConfig(subscriptionDomainsConfigKey, {
          value: JSON.stringify(domains),
          type: 'json',
          group: 'app',
          description: 'Alternate subscription domains'
        }),
        'runtime.systemPage.subscription.messages.saveFailed'
      )
    }
    await Promise.all([
      loadSubscriptionDomainSettings(),
      fetchConfigs()
    ])
    notify(t('runtime.systemPage.subscription.messages.saveSuccess'))
  } catch (err) {
    subscriptionDomainError.value = resolveSystemError(err, 'runtime.systemPage.subscription.messages.saveFailed')
  } finally {
    subscriptionSettingsSaving.value = false
  }
}

// System config
const fetchConfigs = async () => {
  try {
    const payload = readSystemPayload(
      await getSystemConfigs(),
      'runtime.systemPage.messages.fetchConfigsFailed'
    )
    const list = Array.isArray(payload.list) ? payload.list : []
    configs.value = list.map((config) => ({
      ...config,
      description: config.description || config.remark || '',
      sensitive: Boolean(config.sensitive),
      has_value: typeof config.has_value === 'boolean'
        ? config.has_value
        : String(config.value || '').trim() !== ''
    }))
  } catch (err) {
    console.error(t('runtime.systemPage.messages.fetchConfigsFailed'), err)
  }
}

const fetchAuditLogs = async () => {
  auditLoading.value = true
  try {
    const params = {
      page: Number(auditFilters.value.page) || 1,
      page_size: Number(auditFilters.value.page_size) || 20
    }
    const action = String(auditFilters.value.action || '').trim()
    const targetType = String(auditFilters.value.target_type || '').trim()
    if (action) {
      params.action = action
    }
    if (targetType) {
      params.target_type = targetType
    }

    const res = ensureSystemSuccess(
      await getSystemAuditLogs(params),
      'runtime.systemPage.messages.fetchAuditLogsFailed'
    )
    const payload = res.data?.data || res.data || {}
    const list = Array.isArray(payload.list) ? payload.list : []
    auditLogs.value = list.map((item) => ({
      id: item?.id ?? null,
      action: item?.action ?? '',
      module: item?.module ?? '',
      target_type: item?.target_type ?? '',
      username: item?.username ?? '',
      content: item?.content ?? '',
      ip: item?.ip ?? '',
      status: item?.status ?? '',
      created_at: item?.created_at ?? ''
    }))
    auditTotal.value = Number(payload.total) || 0
    if (Number.isFinite(Number(payload.page)) && Number(payload.page) > 0) {
      auditFilters.value.page = Number(payload.page)
    }
    if (Number.isFinite(Number(payload.page_size)) && Number(payload.page_size) > 0) {
      auditFilters.value.page_size = Number(payload.page_size)
    }
  } catch (err) {
    auditLogs.value = []
    auditTotal.value = 0
    notify(resolveSystemError(err, 'runtime.systemPage.messages.fetchAuditLogsFailed'))
  } finally {
    auditLoading.value = false
  }
}

const refreshAuditLogs = async () => {
  await fetchAuditLogs()
}

const applyAuditFilters = async () => {
  auditFilters.value.page = 1
  await fetchAuditLogs()
}

const changeAuditPage = async (page) => {
  const next = Number(page)
  if (!Number.isFinite(next)) return
  const bounded = Math.min(Math.max(1, next), auditTotalPages.value)
  if (bounded === auditFilters.value.page) return
  auditFilters.value.page = bounded
  await fetchAuditLogs()
}

const changeAuditPageSize = async () => {
  const next = Number(auditFilters.value.page_size)
  auditFilters.value.page_size = Number.isFinite(next) && next > 0 ? next : 20
  auditFilters.value.page = 1
  await fetchAuditLogs()
}

const openConfigModal = (config = null) => {
  if (config) {
    editingConfig.value = config
    const sensitive = Boolean(config.sensitive)
    const rawValue = config.value ?? ''
    configForm.value = {
      key: config.key || '',
      value: sensitive ? '' : rawValue,
      description: config.description || config.remark || '',
      type: config.type || 'string',
      group: config.group || '',
      sensitive,
      has_value: typeof config.has_value === 'boolean'
        ? config.has_value
        : String(rawValue).trim() !== ''
    }
  } else {
    editingConfig.value = null
    configForm.value = {
      key: '',
      value: '',
      description: '',
      type: 'string',
      group: '',
      sensitive: false,
      has_value: false
    }
  }
  showConfigModal.value = true
}

const saveConfig = async () => {
  try {
    const value = typeof configForm.value.value === 'string'
      ? configForm.value.value
      : String(configForm.value.value ?? '')
    const preserveExisting = Boolean(
      editingConfig.value &&
      configForm.value.sensitive &&
      configForm.value.has_value &&
      value.trim() === ''
    )

    await ensureSystemMutation(
      setSystemConfig(configForm.value.key, {
        value,
        type: configForm.value.type || 'string',
        group: configForm.value.group || '',
        description: configForm.value.description || '',
        preserve_existing: preserveExisting
      }),
      'runtime.systemPage.messages.fetchConfigsFailed'
    )
    showConfigModal.value = false
    fetchConfigs()
  } catch (err) {
    notify(t('runtime.systemPage.messages.saveConfigFailed', {
      message: resolveSystemError(err, 'runtime.systemPage.messages.fetchConfigsFailed')
    }))
  }
}

const deleteConfig = async (config) => {
  if (!confirmAction(t('runtime.systemPage.messages.deleteConfigConfirm', { key: config.key }))) return
  try {
    await ensureSystemMutation(
      deleteSystemConfig(config.key),
      'runtime.systemPage.messages.deleteConfigFailed'
    )
    fetchConfigs()
  } catch (err) {
    notify(resolveSystemError(err, 'runtime.systemPage.messages.deleteConfigFailed'))
  }
}

// Backups
const hasEmptySensitiveBackupField = (form) => {
  if (!form || typeof form !== 'object') return false
  const accessKeyEmpty = Boolean(form.s3_access_key_sensitive && form.s3_access_key_has_value && String(form.s3_access_key || '').trim() === '')
  const secretKeyEmpty = Boolean(form.s3_secret_key_sensitive && form.s3_secret_key_has_value && String(form.s3_secret_key || '').trim() === '')
  return accessKeyEmpty || secretKeyEmpty
}

const buildBackupConfigPayload = (form) => {
  const normalizedStorageType = String(form.storage_type || 'local').trim().toLowerCase() || 'local'
  const preserveExistingSensitive = hasEmptySensitiveBackupField(form)
  return {
    enabled: !!form.enabled,
    auto_backup: !!form.enabled,
    interval: Number.isFinite(Number(form.interval)) && Number(form.interval) > 0 ? Number(form.interval) : 24,
    keep_count: Number.isFinite(Number(form.keep_count)) && Number(form.keep_count) > 0 ? Number(form.keep_count) : 7,
    backup_database: !!form.backup_database,
    backup_files: !!form.backup_files,
    storage_type: normalizedStorageType,
    storage_path: normalizedStorageType === 'local' ? String(form.storage_path || '').trim() : String(form.storage_path || ''),
    s3_bucket: normalizedStorageType === 's3' ? String(form.s3_bucket || '').trim() : String(form.s3_bucket || ''),
    s3_region: normalizedStorageType === 's3' ? String(form.s3_region || '').trim() : String(form.s3_region || ''),
    s3_endpoint: normalizedStorageType === 's3' ? String(form.s3_endpoint || '').trim() : String(form.s3_endpoint || ''),
    s3_access_key: normalizedStorageType === 's3' ? String(form.s3_access_key || '').trim() : String(form.s3_access_key || ''),
    s3_secret_key: normalizedStorageType === 's3' ? String(form.s3_secret_key || '').trim() : String(form.s3_secret_key || ''),
    preserve_existing_sensitive: preserveExistingSensitive
  }
}

const readBackupPayload = (res) => {
  if (!res || typeof res !== 'object') return {}
  if (Object.prototype.hasOwnProperty.call(res, 'code')) {
    return res.data && typeof res.data === 'object' ? res.data : {}
  }
  if (res.data && typeof res.data === 'object' && Object.prototype.hasOwnProperty.call(res.data, 'data')) {
    return res.data.data && typeof res.data.data === 'object' ? res.data.data : {}
  }
  return res.data && typeof res.data === 'object' ? res.data : res
}

const readBackupList = (res) => {
  const payload = readBackupPayload(res)
  return Array.isArray(payload?.list) ? payload.list : []
}

const fetchBackupConfig = async () => {
  try {
    const res = await getBackupConfig()
    const payload = readBackupPayload(res)
    if (payload && typeof payload === 'object') {
      backupConfig.value = createBackupConfigForm(payload)
    }
  } catch (err) {
    console.error(t('runtime.systemPage.messages.fetchBackupConfigFailed'), err)
  }
}

const saveBackupConfig = async () => {
  try {
    await updateBackupConfig(buildBackupConfigPayload(backupConfig.value))
    notify(t('runtime.systemPage.messages.backupConfigSaved'))
  } catch (err) {
    notify(resolveSystemError(err, 'runtime.systemPage.messages.backupConfigSaveFailed'))
  }
}

const createBackupRequest = async () => {
  try {
    await createBackup()
    notify(t('runtime.systemPage.messages.backupStarted'))
    fetchBackups()
    fetchBackupStats()
  } catch (err) {
    notify(resolveSystemError(err, 'runtime.systemPage.messages.backupStartFailed'))
  }
}

const fetchBackups = async () => {
  try {
    const res = await getBackups()
    backups.value = readBackupList(res)
  } catch (err) {
    console.error(t('runtime.systemPage.messages.fetchBackupsFailed'), err)
  }
}

const readBackupStats = (res) => {
  if (!res || typeof res !== 'object') return {}
  const payload = Object.prototype.hasOwnProperty.call(res, 'code') ? res.data : (res.data ?? res)
  return payload && typeof payload === 'object' && !Array.isArray(payload) ? payload : {}
}

const fetchBackupStats = async () => {
  try {
    const res = await getBackupStats()
    backupStats.value = readBackupStats(res)
  } catch (err) {
    console.error(t('runtime.systemPage.messages.fetchBackupStatsFailed'), err)
  }
}

const deleteBackupRequest = async (backup) => {
  if (!confirmAction(t('runtime.systemPage.messages.deleteBackupConfirm', { filename: backup.filename }))) return
  try {
    await deleteBackup(backup.id)
    fetchBackups()
    fetchBackupStats()
  } catch (err) {
    notify(resolveSystemError(err, 'runtime.systemPage.messages.deleteBackupFailed'))
  }
}

const restoreBackupRequest = async (backup) => {
  if (!confirmAction(t('runtime.systemPage.messages.restoreBackupConfirm', { filename: backup.filename }))) return
  try {
    await restoreBackup(backup.id)
    notify(t('runtime.systemPage.messages.restoreBackupSuccess'))
  } catch (err) {
    notify(t('runtime.systemPage.messages.restoreBackupFailed', {
      message: resolveSystemError(err, 'runtime.systemPage.messages.restoreBackupFailed')
    }))
  }
}

// Load balancing
const fetchBalancers = async () => {
  try {
    const payload = readSystemPayload(
      await getLoadBalancers(),
      'runtime.systemPage.messages.fetchBalancersFailed'
    )
    balancers.value = Array.isArray(payload.list) ? payload.list : []
  } catch (err) {
    console.error(t('runtime.systemPage.messages.fetchBalancersFailed'), err)
  }
}

const openBalancerModal = (lb = null) => {
  if (lb) {
    editingBalancer.value = lb
    balancerForm.value = {
      name: lb.name,
      group_id: lb.group_id,
      strategy: lb.strategy,
      health_check: lb.health_check,
      check_interval: lb.check_interval,
      weights_json: typeof lb.weights === 'string' ? lb.weights : JSON.stringify(lb.weights || {})
    }
  } else {
    editingBalancer.value = null
    balancerForm.value = {
      name: '', group_id: 0, strategy: 'round-robin',
      health_check: true, check_interval: 60, weights_json: ''
    }
  }
  showBalancerModal.value = true
}

const saveBalancer = async () => {
  try {
    const data = { ...balancerForm.value }
    if (data.weights_json) {
      try {
        data.weights = JSON.parse(data.weights_json)
      } catch (e) {
        notify(t('runtime.systemPage.messages.weightsJsonInvalid'))
        return
      }
    }
    delete data.weights_json

    if (editingBalancer.value) {
      await ensureSystemMutation(
        updateLoadBalancer(editingBalancer.value.id, data),
        'runtime.systemPage.messages.saveBalancerFailed'
      )
    } else {
      await ensureSystemMutation(
        createLoadBalancer(data),
        'runtime.systemPage.messages.saveBalancerFailed'
      )
    }
    showBalancerModal.value = false
    fetchBalancers()
  } catch (err) {
    notify(t('runtime.systemPage.messages.saveBalancerFailed', {
      message: resolveSystemError(err, 'runtime.systemPage.messages.saveBalancerFailed')
    }))
  }
}

const deleteBalancer = async (lb) => {
  if (!confirmAction(t('runtime.systemPage.messages.deleteBalancerConfirm', { name: lb.name }))) return
  try {
    await ensureSystemMutation(
      deleteLoadBalancer(lb.id),
      'runtime.systemPage.messages.deleteBalancerFailed'
    )
    fetchBalancers()
  } catch (err) {
    notify(resolveSystemError(err, 'runtime.systemPage.messages.deleteBalancerFailed'))
  }
}

const runHealthCheckRequest = async (lb) => {
  try {
    await ensureSystemMutation(
      runHealthCheck(lb.id),
      'runtime.systemPage.messages.healthCheckFailed'
    )
    notify(t('runtime.systemPage.messages.healthCheckCompleted'))
    fetchBalancers()
  } catch (err) {
    notify(resolveSystemError(err, 'runtime.systemPage.messages.healthCheckFailed'))
  }
}

onMounted(async () => {
  fetchConfigs()
  fetchAuditLogs()
  loadSubscriptionDomainSettings()
  await fetchForwardRuntimeConfig()
  fetchForwardRuntimeJobs()
  fetchRuntimeStatusSafe()
  fetchBackupConfig()
  fetchBackups()
  fetchBackupStats()
  fetchBalancers()
})
</script>

<style scoped>
.backup-config, .backup-stats {
  background: var(--surface-color);
  padding: 24px;
  border-radius: var(--radius-lg);
  border: 1px solid var(--border-color);
  margin-bottom: 20px;
}

.subscription-settings-card {
  background: var(--surface-color);
  padding: 20px;
  border-radius: var(--radius-lg);
  border: 1px solid var(--border-color);
  margin-bottom: 20px;
}

.subscription-settings-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 16px;
}

.subscription-settings-head h3 {
  margin: 4px 0;
}

.subscription-settings-actions {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}

.subscription-settings-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}

.field-help {
  margin-top: 8px;
  color: var(--text-secondary);
  font-size: 13px;
}

.subscription-settings-preview {
  margin-top: 18px;
  display: grid;
  gap: 8px;
}

.subscription-settings-preview code {
  display: block;
  padding: 10px 12px;
  border-radius: var(--radius-md);
  background: var(--surface-muted);
  border: 1px solid var(--border-color);
  overflow-x: auto;
}

.runtime-config-card {
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  padding: 20px;
  margin-bottom: 20px;
  box-shadow: 0 10px 30px rgba(15, 23, 42, 0.05);
}
.runtime-config-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
  gap: 12px;
}

@media (max-width: 900px) {
  .subscription-settings-head {
    flex-direction: column;
  }

  .subscription-settings-grid {
    grid-template-columns: 1fr;
  }
}
.runtime-config-head h3 {
  display: none;
}
.runtime-workbench-title {
  margin: 6px 0 0;
  font-size: 28px;
  font-weight: 700;
  color: var(--text-color);
}
.runtime-config-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.runtime-config-card .form-grid {
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
}
.runtime-config-card textarea {
  min-height: 120px;
  font-family: Consolas, 'Courier New', monospace;
}
.runtime-config-card .btn {
  min-width: 120px;
}
.runtime-mode-overview {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
  gap: 16px;
  margin-bottom: 18px;
}
.runtime-mode-card {
  border: 1px solid var(--border-color);
  border-radius: 18px;
  background: var(--bg-color);
  padding: 18px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.runtime-mode-card.active {
  border-color: var(--primary-color);
  box-shadow: 0 10px 24px rgba(59, 130, 246, 0.12);
}
.runtime-mode-card h4 {
  margin: 0;
}
.runtime-mode-card .btn {
  align-self: flex-start;
}
.runtime-jobs-block {
  margin-top: 20px;
  padding-top: 18px;
  border-top: 1px solid var(--border-color);
}
.runtime-jobs-head,
.runtime-job-main,
.runtime-job-side {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.runtime-jobs-head {
  margin-bottom: 12px;
  flex-wrap: wrap;
}
.runtime-jobs-head h4 {
  margin: 0;
}
.runtime-jobs-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.runtime-job-item {
  padding: 14px 16px;
  border-radius: 16px;
  border: 1px solid var(--border-color);
  background: rgba(15, 23, 42, 0.03);
}
.runtime-job-main {
  align-items: flex-start;
}
.runtime-job-title {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.runtime-job-title span,
.runtime-job-time {
  color: var(--text-secondary);
  font-size: 12px;
}
.runtime-job-side {
  flex-shrink: 0;
}
.runtime-job-message {
  display: block;
  margin-top: 12px;
  padding: 10px 12px;
  border-radius: 12px;
  background: rgba(15, 23, 42, 0.04);
  font-family: Consolas, 'Courier New', monospace;
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-word;
}
.runtime-jobs-empty {
  padding: 14px 16px;
  border-radius: 14px;
  background: rgba(15, 23, 42, 0.03);
  color: var(--text-secondary);
}
.runtime-operator-panel {
  margin-top: 24px;
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  background: var(--surface-color);
  padding: 20px;
}
.operator-head {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 16px;
  align-items: center;
}
.operator-actions {
  display: flex;
  gap: 8px;
}
.operator-status-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 12px;
  margin-bottom: 16px;
}
.operator-card {
  padding: 12px;
  border: 1px dashed var(--border-color);
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.02);
}
.operator-loading {
  padding: 12px;
  border-radius: 10px;
  background: rgba(253, 230, 138, 0.2);
  color: #92400e;
}
.operator-commands code {
  display: block;
  margin-bottom: 8px;
  background: rgba(224, 224, 224, 0.15);
  padding: 8px;
  border-radius: 8px;
  font-family: 'Courier New', monospace;
}
.operator-doctor-output pre {
  background: rgba(15, 23, 42, 0.05);
  border-radius: 8px;
  padding: 12px;
  max-height: 160px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-word;
  margin: 0;
}
.metric-label {
  font-size: 12px;
  color: var(--text-secondary);
}
.metric-value {
  font-size: 18px;
  font-weight: 600;
}
.metric-detail {
  font-size: 12px;
  color: var(--text-secondary);
  margin: 2px 0;
}
.runtime-status-0 {
  background: rgba(245, 158, 11, 0.16);
  color: #b45309;
}
.runtime-status-1 {
  background: rgba(59, 130, 246, 0.14);
  color: #1d4ed8;
}
.runtime-status-2 {
  background: rgba(16, 185, 129, 0.14);
  color: #047857;
}
.runtime-status-3 {
  background: rgba(239, 68, 68, 0.14);
  color: #b91c1c;
}

.backup-config h3 {
  margin-bottom: 16px;
}

.backup-stats {
  display: flex;
  gap: 40px;
}

.stat-item {
  display: flex;
  align-items: center;
  gap: 8px;
}

.stat-item .stat-label {
  color: var(--text-secondary);
}

.stat-item .stat-value {
  font-weight: 600;
}

.search-input {
  min-width: 200px;
}

.audit-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}

.audit-toolbar h3 {
  margin: 0;
}

.audit-toolbar-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.audit-content-cell {
  max-width: 320px;
  white-space: pre-wrap;
  word-break: break-word;
}

.audit-pagination {
  margin-top: 12px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}

.audit-pagination-controls {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.audit-pagination-controls label {
  display: flex;
  align-items: center;
  gap: 6px;
}

.value-cell {
  font-family: monospace;
  font-size: 13px;
}

.strategy-badge {
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 4px;
  background: rgba(59, 130, 246, 0.15);
  color: #3b82f6;
}

.status-pending { background: rgba(251, 191, 36, 0.15); color: #fbbf24; }
.status-completed { background: rgba(34, 197, 94, 0.15); color: #22c55e; }
.status-failed { background: rgba(239, 68, 68, 0.15); color: #ef4444; }

@media (max-width: 768px) {
  .runtime-config-head,
  .operator-head {
    flex-direction: column;
    align-items: flex-start;
  }

  .audit-toolbar {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>
