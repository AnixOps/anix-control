<template>
  <div class="system-page">
    <div class="page-header">
      <h1>系统管理</h1>
      <p class="text-secondary">系统配置、数据备份与负载均衡</p>
    </div>

    <!-- 标签切换 -->
    <div class="tabs">
      <button :class="['tab', { active: activeTab === 'config' }]" @click="activeTab = 'config'">
        系统配置
      </button>
      <button :class="['tab', { active: activeTab === 'backup' }]" @click="activeTab = 'backup'">
        数据备份
      </button>
      <button :class="['tab', { active: activeTab === 'balancer' }]" @click="activeTab = 'balancer'">
        负载均衡
      </button>
    </div>

    <!-- 系统配置 -->
    <div v-show="activeTab === 'config'">
      <div class="toolbar">
        <input v-model="configSearch" type="text" placeholder="搜索配置项..." class="search-input" />
        <button class="btn-primary" @click="openConfigModal()">➕ 新增配置</button>
      </div>
      <section class="runtime-config-card">
        <div class="runtime-config-head">
          <div>
            <p class="eyebrow">Forward Runtime</p>
            <h3>双运行时控制面</h3>
          </div>
          <div class="runtime-config-actions">
            <button class="btn btn-secondary btn-sm" :disabled="runtimeJobsLoading" @click="fetchForwardRuntimeJobs">
              {{ runtimeJobsLoading ? 'Refreshing...' : 'Refresh jobs' }}
            </button>
            <button class="btn btn-secondary btn-sm" :disabled="runtimeSaving" @click="saveForwardRuntimeConfig">
              {{ runtimeSaving ? 'Saving...' : 'Save config' }}
            </button>
          </div>
        </div>
        <div class="runtime-mode-toggle">
          <div>
            <p class="eyebrow">Forward Runtime</p>
            <h3>NodeX Mode</h3>
            <p class="text-secondary mode-description">
              NodeX Mode delegates runtime actions to the gost-backed control plane. When disabled, the system
              falls back to the local iptables/Ansible executor.
            </p>
            <p class="text-secondary mode-description">
              NodeX Mode requires an explicit base URL and token even for local deployments (think
              http://localhost:PORT + your shared token). This mode preserves ingress/egress semantics.
            </p>
          </div>
          <label class="mode-switch">
            <input type="checkbox" v-model="runtimeNodeXMode" aria-label="Toggle NodeX Mode" />
            <span></span>
          </label>
        </div>
        <div class="form-grid runtime-config-grid">
          <div v-if="runtimeNodeXMode" class="node-config">
            <div class="form-group">
              <label for="nodex-base-url">NodeX Base URL</label>
              <input
                id="nodex-base-url"
                name="nodex-base-url"
                type="text"
                v-model="runtimeNodeXBaseUrl"
                placeholder="https://nodex.example.com"
              />
            </div>
            <div class="form-group">
              <label for="nodex-token">NodeX Token</label>
              <input
                id="nodex-token"
                name="nodex-token"
                type="text"
                v-model="runtimeNodeXToken"
                placeholder="X-API-Key or Bearer token"
              />
            </div>
            <div class="form-group">
              <label for="nodex-timeout">Timeout (seconds)</label>
              <input
                id="nodex-timeout"
                name="nodex-timeout"
                type="number"
                min="1"
                v-model.number="runtimeNodeXTimeout"
              />
            </div>
            <p class="text-secondary small">
              NodeX timeout defaults to 15 seconds. The control plane still needs explicit base_url + token so jobs
              can authenticate.
            </p>
          </div>
          <div v-else class="ansible-config">
            <div class="runtime-local-head">
              <div>
                <p class="eyebrow">Local iptables/Ansible</p>
                <h4>Stateless executor config</h4>
                <p class="text-secondary">
                  This path stores only execution-node identity on the tunnel. SSH credentials still come from the
                  inventory or environment variables, not from ForwardNode records.
                </p>
              </div>
              <button class="btn btn-secondary btn-sm" :disabled="runtimeSaving" @click="applyDefaultRuntimeAnsibleConfig">
                Use defaults
              </button>
            </div>

            <div class="form-grid ansible-form-grid">
              <div class="form-group">
                <label for="ansible-inventory">Inventory</label>
                <input
                  id="ansible-inventory"
                  v-model.trim="runtimeAnsibleForm.inventory"
                  type="text"
                  placeholder="config/deploy/ansible/inventory.ini"
                />
              </div>
              <div class="form-group">
                <label for="ansible-apply-playbook">Apply playbook</label>
                <input
                  id="ansible-apply-playbook"
                  v-model.trim="runtimeAnsibleForm.playbookApply"
                  type="text"
                  placeholder="config/deploy/ansible/playbooks/forward_apply.yml"
                />
              </div>
              <div class="form-group">
                <label for="ansible-remove-playbook">Remove playbook</label>
                <input
                  id="ansible-remove-playbook"
                  v-model.trim="runtimeAnsibleForm.playbookRemove"
                  type="text"
                  placeholder="config/deploy/ansible/playbooks/forward_remove.yml"
                />
              </div>
              <div class="form-group">
                <label for="ansible-command">Command</label>
                <input
                  id="ansible-command"
                  v-model.trim="runtimeAnsibleForm.command"
                  type="text"
                  placeholder="ansible-playbook"
                />
              </div>
              <div class="form-group">
                <label for="ansible-working-dir">Working dir</label>
                <input
                  id="ansible-working-dir"
                  v-model.trim="runtimeAnsibleForm.workingDir"
                  type="text"
                  placeholder="config/deploy/ansible"
                />
              </div>
              <div class="form-group">
                <label for="ansible-target-pattern">Target pattern</label>
                <input
                  id="ansible-target-pattern"
                  v-model.trim="runtimeAnsibleForm.targetPattern"
                  type="text"
                  placeholder="{{node.host}}"
                />
              </div>
              <div class="form-group">
                <label for="ansible-timeout-seconds">Timeout (seconds)</label>
                <input
                  id="ansible-timeout-seconds"
                  v-model.number="runtimeAnsibleForm.timeoutSeconds"
                  type="number"
                  min="1"
                  placeholder="120"
                />
              </div>
              <div class="form-group">
                <label for="ansible-config-path">ANSIBLE_CONFIG</label>
                <input
                  id="ansible-config-path"
                  v-model.trim="runtimeAnsibleForm.ansibleConfig"
                  type="text"
                  placeholder="config/deploy/ansible/ansible.cfg"
                />
              </div>
            </div>

            <div class="form-group checkbox-group">
              <label class="checkbox-label">
                <input v-model="runtimeAnsibleForm.become" type="checkbox" />
                <span>Use sudo / become on the execution node</span>
              </label>
            </div>

            <div class="form-grid ansible-form-grid ansible-json-grid">
              <div class="form-group">
                <label for="ansible-extra-vars-json">Extra vars JSON</label>
                <textarea
                  id="ansible-extra-vars-json"
                  v-model="runtimeAnsibleForm.extraVarsJson"
                  rows="6"
                  placeholder='{"manage_with":"iptables"}'
                ></textarea>
                <p class="text-secondary small">Additional Ansible vars merged into the generated payload.</p>
              </div>
              <div class="form-group">
                <label for="ansible-environment-json">Environment JSON</label>
                <textarea
                  id="ansible-environment-json"
                  v-model="runtimeAnsibleForm.environmentJson"
                  rows="6"
                  placeholder='{"ANSIBLE_HOST_KEY_CHECKING":"False"}'
                ></textarea>
                <p class="text-secondary small">Extra environment variables for the local executor process.</p>
              </div>
            </div>

            <div class="form-group">
              <label>Generated runtime JSON</label>
              <textarea :value="runtimeConfigPreview" rows="8" class="runtime-config-preview" readonly></textarea>
              <p class="text-secondary">
                Saved payload is generated from the structured fields above. Use the JSON blocks only for advanced
                vars and environment overrides.
              </p>
            </div>
          </div>
        </div>
        <p v-if="runtimeValidationError" class="form-error">{{ runtimeValidationError }}</p>

        <div class="runtime-jobs-block">
          <div class="runtime-jobs-head">
            <h4>Recent runtime jobs</h4>
            <span class="text-secondary">Latest queued and executed actions for the dual-runtime layer.</span>
          </div>

          <div v-if="runtimeJobsLoading" class="runtime-jobs-empty">Loading runtime jobs...</div>

          <div v-else-if="runtimeJobs.length" class="runtime-jobs-list">
            <article v-for="job in runtimeJobs" :key="job.id" class="runtime-job-item">
              <div class="runtime-job-main">
                <div class="runtime-job-title">
                  <strong>#{{ job.id }} {{ job.action }}</strong>
                  <span>{{ job.backend }} · forward {{ job.forwardId || '-' }} · tunnel {{ job.tunnelId || '-' }} · node {{ job.nodeId || '-' }}</span>
                </div>
                <div class="runtime-job-side">
                  <span :class="['status-badge', `runtime-status-${job.status}`]">{{ getRuntimeJobStatusLabel(job.status) }}</span>
                  <span class="runtime-job-time">{{ formatRuntimeJobTime(job) }}</span>
                </div>
              </div>
              <code v-if="formatRuntimeJobMessage(job)" class="runtime-job-message">{{ formatRuntimeJobMessage(job) }}</code>
            </article>
          </div>

          <div v-else class="runtime-jobs-empty">No runtime jobs yet.</div>
        </div>

        <div class="runtime-operator-panel">
          <div class="operator-head">
            <div>
              <p class="eyebrow">Forward Runtime Doctor</p>
              <h4>运行时状态与命令</h4>
              <p class="text-secondary mode-description">
                Reachability only means the control plane or local executor can be contacted. It is not proof that a
                relay has already attached or that iptables rules already exist.
              </p>
              <p class="text-secondary mode-description">
                汇总控制面健康、运行时诊断与一键命令，统一覆盖 NodeX/gost 与本地 ansible 两种执行路径。
              </p>
            </div>
            <div class="operator-actions">
              <button class="btn btn-secondary btn-sm" :disabled="runtimeStatusLoading" @click="fetchRuntimeStatusSafe">
                {{ runtimeStatusLoading ? 'Loading...' : '刷新状态' }}
              </button>
              <button class="btn btn-secondary btn-sm" :disabled="runtimeDoctorRunning" @click="runRuntimeDoctorCheckSafe">
                {{ runtimeDoctorRunning ? 'Running...' : 'Run Doctor' }}
              </button>
            </div>
          </div>

          <div v-if="runtimeStatusLoading" class="operator-loading">Fetching forward runtime status...</div>
          <div v-else>
            <div v-if="runtimeStatusError" class="form-error">{{ runtimeStatusError }}</div>
            <div v-else class="operator-status-grid">
              <div class="status-card operator-card">
                <p class="metric-label">Backend</p>
                <p class="metric-value">{{ runtimeStatus?.config?.backend || (runtimeNodeXMode ? 'gost' : 'iptables_ansible') }}</p>
                <p class="metric-detail">NodeX Mode: {{ runtimeStatus?.config?.nodeXMode ? 'Enabled' : 'Disabled' }}</p>
                <p class="metric-detail">Attachment: {{ runtimeStatus?.attachment?.model || '-' }}</p>
                <p class="metric-detail">{{ runtimeStatus?.attachment?.description || '-' }}</p>
              </div>
              <div class="status-card operator-card">
                <p class="metric-label">Panel Verdict</p>
                <p class="metric-value">{{ runtimeStatus?.runtimeReady?.ready ? 'Ready' : 'Not ready' }}</p>
                <p class="metric-detail">Reachability: {{ runtimeStatus?.reachability?.ready ? 'Ready' : 'Not ready' }}</p>
                <p class="metric-detail">{{ runtimeStatus?.reachability?.reason || '-' }}</p>
                <p class="metric-detail">{{ runtimeStatus?.runtimeReady?.reason || '-' }}</p>
              </div>
              <div class="status-card operator-card">
                <template v-if="runtimeStatus?.config?.nodeXMode">
                  <p class="metric-label">NodeX Snapshot</p>
                  <p class="metric-detail">Base URL: {{ runtimeStatus?.config?.baseUrl || '-' }}</p>
                  <p class="metric-detail">Base URL configured: {{ runtimeStatus?.config?.baseUrlConfigured ? 'Yes' : 'No' }}</p>
                  <p class="metric-detail">Token configured: {{ runtimeStatus?.config?.tokenConfigured ? 'Yes' : 'No' }}</p>
                  <p class="metric-detail">Health: {{ runtimeStatus?.health?.ok ? 'OK' : 'Unavailable' }}</p>
                  <p class="metric-detail">Runtime version: {{ runtimeStatus?.runtimeStatus?.version || '-' }}</p>
                </template>
                <template v-else>
                  <p class="metric-label">Local ansible</p>
                  <p class="metric-detail">Command: {{ runtimeStatus?.localAnsible?.command || 'ansible-playbook' }}</p>
                  <p class="metric-detail">Command found: {{ runtimeStatus?.localAnsible?.commandFound ? 'Yes' : 'No' }}</p>
                  <p class="metric-detail">Inventory: {{ runtimeStatus?.localAnsible?.inventoryExists ? 'Present' : 'Missing' }}</p>
                  <p class="metric-detail">Playbooks: {{ runtimeStatus?.localAnsible?.applyPlaybookExists && runtimeStatus?.localAnsible?.removePlaybookExists ? 'Ready' : 'Missing' }}</p>
                  <p class="metric-detail">Working dir: {{ runtimeStatus?.localAnsible?.workingDirExists ? 'Present' : 'Missing' }}</p>
                </template>
              </div>
            </div>
          </div>

          <div v-if="runtimeStatus?.warnings?.length" class="operator-commands">
            <p class="metric-label">Warnings</p>
            <code v-for="warning in runtimeStatus.warnings" :key="warning">{{ warning }}</code>
          </div>

          <div class="operator-commands">
            <p class="metric-label">PowerShell</p>
            <code v-for="command in runtimeDisplayedCommands.powerShell" :key="`ps-${command}`">{{ command }}</code>
            <p class="metric-label">Bash</p>
            <code v-for="command in runtimeDisplayedCommands.bash" :key="`bash-${command}`">{{ command }}</code>
            <p class="metric-label">Upgrade / Verify</p>
            <code v-for="command in runtimeDisplayedCommands.upgrade" :key="`upgrade-${command}`">{{ command }}</code>
            <p class="metric-label">Reference</p>
            <code v-for="reference in runtimeDisplayedCommands.references" :key="reference">{{ reference }}</code>
          </div>
          <div class="operator-doctor-output">
            <p class="metric-label">Doctor 输出</p>
            <pre>{{ runtimeDoctorOutput || '尚未运行 doctor' }}</pre>
          </div>
        </div>
      </section>

      <div class="table-container">
        <table class="data-table">
          <thead>
            <tr>
              <th>键名</th>
              <th>值</th>
              <th>描述</th>
              <th>更新时间</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="config in filteredConfigs" :key="config.key">
              <td><code>{{ config.key }}</code></td>
              <td class="value-cell">{{ truncateValue(config.value) }}</td>
              <td>{{ config.description || '-' }}</td>
              <td>{{ formatTime(config.updated_at) }}</td>
              <td>
                <div class="action-buttons">
                  <button class="btn-sm btn-ghost" @click="openConfigModal(config)" title="编辑">✏️</button>
                  <button class="btn-sm btn-ghost" @click="deleteConfig(config)" title="删除">🗑️</button>
                </div>
              </td>
            </tr>
            <tr v-if="filteredConfigs.length === 0">
              <td colspan="5" class="empty-row">暂无配置数据</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 数据备份 -->
    <div v-show="activeTab === 'backup'">
      <div class="backup-config">
        <h3>自动备份配置</h3>
        <div class="form-row">
          <div class="form-group">
            <label class="checkbox-label">
              <input type="checkbox" v-model="backupConfig.enabled" />
              <span>启用自动备份</span>
            </label>
          </div>
          <div class="form-group">
            <label>备份间隔 (小时)</label>
            <input v-model.number="backupConfig.interval" type="number" min="1" />
          </div>
          <div class="form-group">
            <label>保留数量</label>
            <input v-model.number="backupConfig.keep_count" type="number" min="1" />
          </div>
        </div>
        <div class="form-actions">
          <button class="btn-secondary" @click="saveBackupConfig">保存配置</button>
          <button class="btn-primary" @click="createBackupRequest">立即备份</button>
        </div>
      </div>

      <div class="backup-stats">
        <div class="stat-item">
          <span class="stat-label">总备份数:</span>
          <span class="stat-value">{{ backupStats.total_count || 0 }}</span>
        </div>
        <div class="stat-item">
          <span class="stat-label">总大小:</span>
          <span class="stat-value">{{ formatSize(backupStats.total_size) }}</span>
        </div>
        <div class="stat-item">
          <span class="stat-label">最近备份:</span>
          <span class="stat-value">{{ backupStats.last_backup ? formatTime(backupStats.last_backup) : '-' }}</span>
        </div>
      </div>

      <div class="table-container">
        <h3>备份列表</h3>
        <table class="data-table">
          <thead>
            <tr>
              <th>ID</th>
              <th>文件名</th>
              <th>大小</th>
              <th>状态</th>
              <th>创建时间</th>
              <th>操作</th>
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
                  <button class="btn-sm btn-ghost" @click="restoreBackupRequest(backup)" title="恢复" :disabled="backup.status !== 'completed'">📥</button>
                  <button class="btn-sm btn-ghost" @click="deleteBackupRequest(backup)" title="删除">🗑️</button>
                </div>
              </td>
            </tr>
            <tr v-if="backups.length === 0">
              <td colspan="6" class="empty-row">暂无备份数据</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 负载均衡 -->
    <div v-show="activeTab === 'balancer'">
      <div class="toolbar">
        <button class="btn-primary" @click="openBalancerModal()">➕ 新建负载均衡器</button>
      </div>

      <div class="table-container">
        <table class="data-table">
          <thead>
            <tr>
              <th>ID</th>
              <th>名称</th>
              <th>节点组</th>
              <th>策略</th>
              <th>健康检查</th>
              <th>状态</th>
              <th>操作</th>
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
                  {{ lb.health_check ? '启用' : '禁用' }}
                </span>
              </td>
              <td>
                <span :class="['status-badge', lb.enabled ? 'status-active' : 'status-disabled']">
                  {{ lb.enabled ? '启用' : '禁用' }}
                </span>
              </td>
              <td>
                <div class="action-buttons">
                  <button class="btn-sm btn-ghost" @click="runHealthCheckRequest(lb)" title="健康检查">🔍</button>
                  <button class="btn-sm btn-ghost" @click="openBalancerModal(lb)" title="编辑">✏️</button>
                  <button class="btn-sm btn-ghost" @click="deleteBalancer(lb)" title="删除">🗑️</button>
                </div>
              </td>
            </tr>
            <tr v-if="balancers.length === 0">
              <td colspan="7" class="empty-row">暂无负载均衡器</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 配置弹窗 -->
    <div v-if="showConfigModal" class="modal-overlay" @click.self="showConfigModal = false">
      <div class="modal">
        <div class="modal-header">
          <h3>{{ editingConfig ? '编辑配置' : '新增配置' }}</h3>
          <button class="close-btn" @click="showConfigModal = false">✕</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label>键名 <span class="required">*</span></label>
            <input v-model="configForm.key" type="text" placeholder="如: site.name" :disabled="!!editingConfig" />
          </div>
          <div class="form-group">
            <label>值</label>
            <textarea v-model="configForm.value" rows="3" placeholder="配置值，支持 JSON 格式"></textarea>
          </div>
          <div class="form-group">
            <label>描述</label>
            <input v-model="configForm.description" type="text" placeholder="配置说明" />
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn-secondary" @click="showConfigModal = false">取消</button>
          <button @click="saveConfig">保存</button>
        </div>
      </div>
    </div>

    <!-- 负载均衡器弹窗 -->
    <div v-if="showBalancerModal" class="modal-overlay" @click.self="showBalancerModal = false">
      <div class="modal">
        <div class="modal-header">
          <h3>{{ editingBalancer ? '编辑负载均衡器' : '新建负载均衡器' }}</h3>
          <button class="close-btn" @click="showBalancerModal = false">✕</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label>名称 <span class="required">*</span></label>
            <input v-model="balancerForm.name" type="text" placeholder="负载均衡器名称" />
          </div>
          <div class="form-row">
            <div class="form-group">
              <label>节点组ID</label>
              <input v-model.number="balancerForm.group_id" type="number" />
            </div>
            <div class="form-group">
              <label>策略</label>
              <select v-model="balancerForm.strategy">
                <option value="round-robin">轮询</option>
                <option value="least-load">最少负载</option>
                <option value="latency">最低延迟</option>
                <option value="weight">加权</option>
                <option value="random">随机</option>
              </select>
            </div>
          </div>
          <div class="form-row">
            <div class="form-group">
              <label class="checkbox-label">
                <input type="checkbox" v-model="balancerForm.health_check" />
                <span>启用健康检查</span>
              </label>
            </div>
            <div class="form-group">
              <label>检查间隔 (秒)</label>
              <input v-model.number="balancerForm.check_interval" type="number" min="10" />
            </div>
          </div>
          <div class="form-group">
            <label>节点权重 (JSON)</label>
            <textarea v-model="balancerForm.weights_json" rows="3" placeholder='{"1": 10, "2": 5}'></textarea>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn-secondary" @click="showBalancerModal = false">取消</button>
          <button @click="saveBalancer">保存</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import {
  getSystemConfigs, getSystemConfig, setSystemConfig, deleteSystemConfig,
  getBackupConfig, updateBackupConfig, createBackup, getBackups,
  getBackupStats, deleteBackup, restoreBackup,
  getLoadBalancers, createLoadBalancer, updateLoadBalancer,
  deleteLoadBalancer, runHealthCheck, listForwardRuntimeJobs, getForwardRuntimeStatus, runForwardRuntimeDoctor
} from '@/api/admin'

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
const runtimeAnsibleConfigKey = 'forward.runtime.iptables_ansible.config'
const runtimeAnsibleInventoryKey = 'forward.ansible.inventory'
const runtimeAnsibleApplyPlaybookKey = 'forward.ansible.playbook_apply'
const runtimeAnsibleRemovePlaybookKey = 'forward.ansible.playbook_remove'
const runtimeAnsibleBecomeKey = 'forward.ansible.become'
const runtimeAnsibleExtraVarsKey = 'forward.ansible.extra_vars_json'
const runtimeNodeXBaseUrlKey = 'forward.runtime.nodex.base_url'
const runtimeNodeXTokenKey = 'forward.runtime.nodex.token'
const runtimeNodeXTimeoutKey = 'forward.runtime.nodex.timeout_seconds'
const defaultRuntimeAnsibleConfig = Object.freeze({
  inventory: 'config/deploy/ansible/inventory.ini',
  playbookApply: 'config/deploy/ansible/playbooks/forward_apply.yml',
  playbookRemove: 'config/deploy/ansible/playbooks/forward_remove.yml',
  command: 'ansible-playbook',
  workingDir: 'config/deploy/ansible',
  targetPattern: '{{node.host}}',
  timeoutSeconds: 120,
  ansibleConfig: 'config/deploy/ansible/ansible.cfg',
  become: false
})
const runtimeBackendOptions = [
  { value: 'gost', label: 'gost (默认)' },
  { value: 'iptables_ansible', label: 'iptables_ansible' }
]
const runtimeBackend = ref('gost')
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

const defaultNodeXBaseUrl = 'http://127.0.0.1:8080'
const runtimeOperatorBaseUrl = computed(() => runtimeNodeXBaseUrl.value?.trim() || defaultNodeXBaseUrl)
const runtimeOperatorToken = computed(() => runtimeNodeXToken.value?.trim() || '<token>')
const runtimeOperatorDoctorCommand = computed(() => `BASE_URL=${runtimeOperatorBaseUrl.value} FORWARD_API_TOKEN=${runtimeOperatorToken.value} bash ./tools/nodex.sh doctor`)
const runtimeOperatorStatusCommand = computed(() => `BASE_URL=${runtimeOperatorBaseUrl.value} FORWARD_API_TOKEN=${runtimeOperatorToken.value} bash ./tools/nodex.sh runtime-status`)
const runtimeOperatorUpgradeCommand = computed(() => 'git pull --ff-only && powershell -File .\\tools\\nodex.ps1 version')
const runtimeOperatorReferences = [
  'docs/reference/check-version.md',
  'docs/reference/upgrade.md',
  'docs/reference/connect-model.md',
  'docs/reference/nodeclient-faq.md'
]
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
      powerShell: [],
      bash: [runtimeOperatorStatusCommand.value, runtimeOperatorDoctorCommand.value],
      upgrade: [runtimeOperatorUpgradeCommand.value],
      references: runtimeOperatorReferences
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
    throw new Error(`${label} must be valid JSON`)
  }

  if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') {
    throw new Error(`${label} must be a JSON object`)
  }

  return parsed
}

function buildRuntimeAnsiblePayload(source = runtimeAnsibleForm.value) {
  const form = normalizeRuntimeAnsibleForm(source)
  const extraVars = parseRuntimeJsonObject(form.extraVarsJson, 'Extra vars JSON')
  const environmentOverrides = parseRuntimeJsonObject(form.environmentJson, 'Environment JSON')
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
    return `Invalid runtime config: ${err.message}`
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

const strategyLabels = {
  'round-robin': '轮询',
  'least-load': '最少负载',
  'latency': '最低延迟',
  'weight': '加权',
  'random': '随机'
}

const statusLabels = {
  pending: '处理中',
  completed: '已完成',
  failed: '失败'
}

const getStrategyLabel = (strategy) => strategyLabels[strategy] || strategy
const getStatusLabel = (status) => statusLabels[status] || status

const formatTime = (time) => {
  if (!time) return '-'
  return new Date(time).toLocaleString()
}

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

const getRuntimeJobStatusLabel = (status) => {
  switch (Number(status)) {
    case 0:
      return 'Pending'
    case 1:
      return 'Running'
    case 2:
      return 'Success'
    case 3:
      return 'Failed'
    default:
      return 'Unknown'
  }
}

const formatRuntimeJobTime = (job) => {
  const value = job.completedAt || job.startedAt || job.updatedAt || job.createdAt
  return value ? formatTime(value) : '-'
}

const formatRuntimeJobMessage = (job) => {
  const source = job.error || job.result || job.payload || ''
  const text = String(source).trim()
  if (!text) return ''
  return text.length > 220 ? `${text.slice(0, 217)}...` : text
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
    runtimeStatusError.value = err.response?.data?.msg || err.message || 'Failed to fetch forward runtime status'
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
    runtimeDoctorOutput.value = err.response?.data?.msg || err.message || 'Forward runtime doctor failed'
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
    const backendValue = backendRes.data?.value || 'gost'
    runtimeBackend.value = backendValue
  } catch (err) {
    console.error('鑾峰彇 forward runtime backend 澶辫触:', err)
  }
  runtimeNodeXMode.value = explicitNodeXMode === null
    ? runtimeBackend.value === 'gost'
    : explicitNodeXMode
  runtimeBackend.value = runtimeNodeXMode.value ? 'gost' : 'iptables_ansible'

  try {
    const configRes = await getSystemConfig(runtimeAnsibleConfigKey)
    const rawValue = configRes.data?.value || ''
    if (rawValue) {
      try {
        const parsed = JSON.parse(rawValue)
        if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') {
          throw new Error('saved payload must be a JSON object')
        }
        runtimeAnsibleForm.value = normalizeRuntimeAnsibleForm(parsed)
      } catch {
        runtimeValidationError.value = 'Saved ansible runtime config is invalid. Defaults were loaded; save again to repair it.'
        runtimeAnsibleForm.value = createRuntimeAnsibleForm()
      }
    }
  } catch (err) {
    console.error('鑾峰彇 forward runtime ansible 配置澶辫触:', err)
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
      runtimeValidationError.value = 'NodeX base URL is required in NodeX Mode'
      return
    }
    if (!trimmedNodeXToken) {
      runtimeValidationError.value = 'NodeX token is required in NodeX Mode'
      return
    }
  }

  let ansiblePayload = null
  if (!runtimeNodeXMode.value) {
    try {
      runtimeAnsibleForm.value = normalizeRuntimeAnsibleForm(runtimeAnsibleForm.value)
      ansiblePayload = buildRuntimeAnsiblePayload(runtimeAnsibleForm.value)
    } catch (err) {
      runtimeValidationError.value = err.message || 'ansible JSON invalid'
      return
    }
  }

  const backendValue = runtimeNodeXMode.value ? 'gost' : 'iptables_ansible'
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
    runtimeValidationError.value = err.response?.data?.error || err.message || '保存失败'
  } finally {
    runtimeSaving.value = false
  }
}

// 系统配置
const fetchConfigs = async () => {
  try {
    const res = await getSystemConfigs()
    configs.value = res.data?.list || []
  } catch (err) {
    console.error('获取配置失败:', err)
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
    alert('保存失败: ' + (err.response?.data?.error || err.message))
  }
}

const deleteConfig = async (config) => {
  if (!confirm(`确定删除配置 ${config.key}?`)) return
  try {
    await deleteSystemConfig(config.key)
    fetchConfigs()
  } catch (err) {
    alert('删除失败')
  }
}

// 备份
const fetchBackupConfig = async () => {
  try {
    const res = await getBackupConfig()
    if (res.data) {
      backupConfig.value = { ...backupConfig.value, ...res.data }
    }
  } catch (err) {
    console.error('获取备份配置失败:', err)
  }
}

const saveBackupConfig = async () => {
  try {
    await updateBackupConfig(backupConfig.value)
    alert('保存成功')
  } catch (err) {
    alert('保存失败')
  }
}

const createBackupRequest = async () => {
  try {
    await createBackup()
    alert('备份已开始')
    fetchBackups()
    fetchBackupStats()
  } catch (err) {
    alert('创建备份失败')
  }
}

const fetchBackups = async () => {
  try {
    const res = await getBackups()
    backups.value = res.data?.list || []
  } catch (err) {
    console.error('获取备份列表失败:', err)
  }
}

const fetchBackupStats = async () => {
  try {
    const res = await getBackupStats()
    backupStats.value = res.data || {}
  } catch (err) {
    console.error('获取备份统计失败:', err)
  }
}

const deleteBackupRequest = async (backup) => {
  if (!confirm(`确定删除备份 ${backup.filename}?`)) return
  try {
    await deleteBackup(backup.id)
    fetchBackups()
    fetchBackupStats()
  } catch (err) {
    alert('删除失败')
  }
}

const restoreBackupRequest = async (backup) => {
  if (!confirm(`确定恢复备份 ${backup.filename}? 当前数据将被覆盖!`)) return
  try {
    await restoreBackup(backup.id)
    alert('恢复成功')
  } catch (err) {
    alert('恢复失败: ' + (err.response?.data?.error || err.message))
  }
}

// 负载均衡
const fetchBalancers = async () => {
  try {
    const res = await getLoadBalancers()
    balancers.value = res.data?.list || []
  } catch (err) {
    console.error('获取负载均衡器失败:', err)
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
        alert('权重 JSON 格式错误')
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
    alert('保存失败: ' + (err.response?.data?.error || err.message))
  }
}

const deleteBalancer = async (lb) => {
  if (!confirm(`确定删除负载均衡器 ${lb.name}?`)) return
  try {
    await deleteLoadBalancer(lb.id)
    fetchBalancers()
  } catch (err) {
    alert('删除失败')
  }
}

const runHealthCheckRequest = async (lb) => {
  try {
    await runHealthCheck(lb.id)
    alert('健康检查已完成')
    fetchBalancers()
  } catch (err) {
    alert('健康检查失败')
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
.runtime-mode-toggle {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  align-items: center;
  margin-bottom: 16px;
}
.runtime-mode-toggle .mode-description {
  margin: 4px 0;
}
.mode-switch {
  display: inline-flex;
  align-items: center;
  position: relative;
  cursor: pointer;
}
.mode-switch input {
  position: absolute;
  width: 0;
  height: 0;
  opacity: 0;
}
.mode-switch span {
  width: 52px;
  height: 28px;
  border-radius: 999px;
  background: var(--border-color);
  display: block;
  transition: background 0.2s ease;
  position: relative;
}
.mode-switch span::after {
  content: '';
  position: absolute;
  top: 3px;
  left: 3px;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: var(--surface-color);
  transition: transform 0.2s ease;
  box-shadow: 0 2px 4px rgba(15, 23, 42, 0.25);
}
.mode-switch input:checked + span {
  background: var(--primary-color);
}
.mode-switch input:checked + span::after {
  transform: translateX(24px);
}
.runtime-config-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
  gap: 16px;
}
.node-config .form-group {
  margin-bottom: 12px;
}
.runtime-local-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 16px;
}
.runtime-local-head h4 {
  margin: 0 0 6px;
}
.ansible-config {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.ansible-form-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 16px;
}
.ansible-json-grid {
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
}
.ansible-config textarea {
  min-height: 160px;
}
.checkbox-group {
  margin: 0;
}
.runtime-config-preview {
  min-height: 220px;
  background: rgba(15, 23, 42, 0.04);
}
.text-secondary.small {
  font-size: 12px;
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

.checkbox-label {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
}

.checkbox-label input[type="checkbox"] {
  width: 18px;
  height: 18px;
}
@media (max-width: 768px) {
  .runtime-config-head,
  .runtime-mode-toggle,
  .runtime-local-head,
  .operator-head {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>
