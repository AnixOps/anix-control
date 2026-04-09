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
    </div>

    <!-- System config -->
    <div v-show="activeTab === 'config'">
      <div class="toolbar">
        <input v-model="configSearch" type="text" :placeholder="t('runtime.systemPage.config.searchPlaceholder')" class="search-input" />
        <button class="btn-primary" @click="openConfigModal()">{{ t('runtime.systemPage.actions.addConfig') }}</button>
      </div>
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
              <td class="value-cell">{{ truncateValue(config.value) }}</td>
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
  getSystemConfigs, getSystemConfig, setSystemConfig, deleteSystemConfig,
  getBackupConfig, updateBackupConfig, createBackup, getBackups,
  getBackupStats, deleteBackup, restoreBackup,
  getLoadBalancers, createLoadBalancer, updateLoadBalancer,
  deleteLoadBalancer, runHealthCheck, listForwardRuntimeJobs, getForwardRuntimeStatus, runForwardRuntimeDoctor
} from '@/api/admin'

const { t, formatDateTime, translateLiteral } = useAppI18n()

const activeTab = ref('config')
const configSearch = ref('')
const configs = ref([])
const backups = ref([])
const balancers = ref([])

const backupConfig = ref({
  enabled: false,
  interval: 24,
  keep_count: 7
})

const backupStats = ref({})

const showConfigModal = ref(false)
const editingConfig = ref(null)
const configForm = ref({ key: '', value: '', description: '' })

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
const runtimeBackendOptions = [
  { value: 'gost', label: `${t('runtime.nodeX.backends.gost')} (default)` },
  { value: 'nftables_ansible', label: 'nftables_ansible' },
  { value: 'iptables_ansible', label: 'iptables_ansible (legacy)' }
]
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

const filteredConfigs = computed(() => {
  if (!configSearch.value) return configs.value
  const search = configSearch.value.toLowerCase()
  return configs.value.filter(c =>
    c.key?.toLowerCase().includes(search) ||
    c.description?.toLowerCase().includes(search)
  )
})

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

const notify = (message) => window.alert(message)
const confirmAction = (message) => window.confirm(message)
const resolveSystemError = (error, fallbackKey) => (
  translateRuntimeText(error?.response?.data?.msg || error?.response?.data?.error || error?.message, t(fallbackKey))
)

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
  const normalized = String(value ?? '').trim().toLowerCase()
  if (normalized === 'gost') {
    return t('runtime.nodeX.backends.gost')
  }
  if (normalized === 'nftables_ansible') {
    return t('runtime.localRuntime.backends.nftables.label')
  }
  if (normalized === 'iptables_ansible') {
    return t('runtime.localRuntime.backends.iptables.label')
  }
  return value || '-'
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
      setSystemConfig(runtimeNodeXModeKey, {
        value: runtimeNodeXMode.value,
        type: 'bool',
        group: 'forward',
        description: 'Enable NodeX forward runtime mode'
      }),
      setSystemConfig(runtimeBackendKey, {
        value: backendValue,
        type: 'string',
        group: 'forward',
        description: 'Forward runtime backend'
      }),
      setSystemConfig(runtimeAnsibleBackendKey, {
        value: backendValue === 'gost' ? 'nftables_ansible' : backendValue,
        type: 'string',
        group: 'forward',
        description: 'Preferred local ansible backend'
      }),
      setSystemConfig(runtimeNodeXBaseUrlKey, {
        value: trimmedNodeXBaseUrl,
        type: 'string',
        group: 'forward',
        description: 'Forward runtime NodeX base URL'
      }),
      setSystemConfig(runtimeNodeXTokenKey, {
        value: trimmedNodeXToken,
        type: 'string',
        group: 'forward',
        description: 'Forward runtime NodeX token'
      }),
      setSystemConfig(runtimeNodeXTimeoutKey, {
        value: Number.isFinite(timeoutValue) && timeoutValue > 0 ? timeoutValue : 15,
        type: 'number',
        group: 'forward',
        description: 'Forward runtime NodeX timeout'
      })
    ]

    if (!runtimeNodeXMode.value) {
      updates.push(
        setSystemConfig(runtimeAnsibleConfigKey, {
          value: ansiblePayload ? JSON.stringify(ansiblePayload) : '',
          type: 'json',
          group: 'forward',
          description: 'Forward runtime ansible config'
        }),
        setSystemConfig(runtimeAnsibleInventoryKey, {
          value: ansiblePayload?.inventory || '',
          type: 'string',
          group: 'forward',
          description: 'Forward ansible inventory path'
        }),
        setSystemConfig(runtimeAnsibleApplyPlaybookKey, {
          value: ansiblePayload?.playbookApply || '',
          type: 'string',
          group: 'forward',
          description: 'Forward ansible apply playbook path'
        }),
        setSystemConfig(runtimeAnsibleRemovePlaybookKey, {
          value: ansiblePayload?.playbookRemove || '',
          type: 'string',
          group: 'forward',
          description: 'Forward ansible remove playbook path'
        }),
        setSystemConfig(runtimeAnsibleBecomeKey, {
          value: ansiblePayload?.become || false,
          type: 'bool',
          group: 'forward',
          description: 'Forward ansible become flag'
        }),
        setSystemConfig(runtimeAnsibleExtraVarsKey, {
          value: JSON.stringify(ansiblePayload?.extraVars || {}),
          type: 'json',
          group: 'forward',
          description: 'Forward ansible extra vars JSON'
        })
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

// System config
const fetchConfigs = async () => {
  try {
    const res = await getSystemConfigs()
    configs.value = res.data?.list || []
  } catch (err) {
    console.error(t('runtime.systemPage.messages.fetchConfigsFailed'), err)
  }
}

const openConfigModal = (config = null) => {
  if (config) {
    editingConfig.value = config
    configForm.value = { ...config }
  } else {
    editingConfig.value = null
    configForm.value = { key: '', value: '', description: '' }
  }
  showConfigModal.value = true
}

const saveConfig = async () => {
  try {
    await setSystemConfig(configForm.value.key, configForm.value)
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
    await deleteSystemConfig(config.key)
    fetchConfigs()
  } catch (err) {
    notify(resolveSystemError(err, 'runtime.systemPage.messages.deleteConfigFailed'))
  }
}

// Backups
const fetchBackupConfig = async () => {
  try {
    const res = await getBackupConfig()
    if (res.data) {
      backupConfig.value = { ...backupConfig.value, ...res.data }
    }
  } catch (err) {
    console.error(t('runtime.systemPage.messages.fetchBackupConfigFailed'), err)
  }
}

const saveBackupConfig = async () => {
  try {
    await updateBackupConfig(backupConfig.value)
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
    backups.value = res.data?.list || []
  } catch (err) {
    console.error(t('runtime.systemPage.messages.fetchBackupsFailed'), err)
  }
}

const fetchBackupStats = async () => {
  try {
    const res = await getBackupStats()
    backupStats.value = res.data || {}
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
    const res = await getLoadBalancers()
    balancers.value = res.data?.list || []
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
      await updateLoadBalancer(editingBalancer.value.id, data)
    } else {
      await createLoadBalancer(data)
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
    await deleteLoadBalancer(lb.id)
    fetchBalancers()
  } catch (err) {
    notify(resolveSystemError(err, 'runtime.systemPage.messages.deleteBalancerFailed'))
  }
}

const runHealthCheckRequest = async (lb) => {
  try {
    await runHealthCheck(lb.id)
    notify(t('runtime.systemPage.messages.healthCheckCompleted'))
    fetchBalancers()
  } catch (err) {
    notify(resolveSystemError(err, 'runtime.systemPage.messages.healthCheckFailed'))
  }
}

onMounted(async () => {
  fetchConfigs()
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
}
</style>
