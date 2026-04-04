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
  getSystemConfigs, setSystemConfig, deleteSystemConfig,
  getBackupConfig, updateBackupConfig, createBackup, getBackups,
  getBackupStats, deleteBackup, restoreBackup,
  getLoadBalancers, createLoadBalancer, updateLoadBalancer,
  deleteLoadBalancer, runHealthCheck
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

onMounted(() => {
  fetchConfigs()
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
</style>
