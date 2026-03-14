<template>
  <div class="invite-page">
    <div class="page-header">
      <h1>邀请返利管理</h1>
      <p class="text-secondary">配置邀请奖励和佣金提现</p>
    </div>

    <!-- 标签切换 -->
    <div class="tabs">
      <button :class="['tab', { active: activeTab === 'config' }]" @click="activeTab = 'config'">
        配置设置
      </button>
      <button :class="['tab', { active: activeTab === 'withdrawals' }]" @click="activeTab = 'withdrawals'">
        提现审核
      </button>
      <button :class="['tab', { active: activeTab === 'stats' }]" @click="activeTab = 'stats'">
        统计数据
      </button>
    </div>

    <!-- 配置设置 -->
    <div v-show="activeTab === 'config'">
      <div class="config-section">
        <h3>邀请配置</h3>
        <div class="form-group">
          <label class="checkbox-label">
            <input type="checkbox" v-model="config.enabled" />
            <span>启用邀请系统</span>
          </label>
        </div>
        <div class="form-row">
          <div class="form-group">
            <label>邀请码前缀</label>
            <input v-model="config.code_prefix" type="text" placeholder="如: INV" />
          </div>
          <div class="form-group">
            <label>邀请码长度</label>
            <input v-model.number="config.code_length" type="number" min="4" max="16" />
          </div>
        </div>

        <h4>佣金设置</h4>
        <div class="form-row">
          <div class="form-group">
            <label>佣金比例 (%)</label>
            <input v-model.number="config.commission_rate" type="number" min="0" max="100" step="0.1" />
            <p class="help-text">被邀请人消费时，邀请人获得的佣金比例</p>
          </div>
          <div class="form-group">
            <label>佣金类型</label>
            <select v-model="config.commission_type">
              <option value="percent">按比例</option>
              <option value="fixed">固定金额</option>
            </select>
          </div>
        </div>
        <div class="form-group" v-if="config.commission_type === 'fixed'">
          <label>固定佣金金额</label>
          <input v-model.number="config.commission_fixed" type="number" step="0.01" />
        </div>

        <h4>提现设置</h4>
        <div class="form-row">
          <div class="form-group">
            <label>最低提现金额</label>
            <input v-model.number="config.min_withdraw" type="number" step="0.01" />
          </div>
          <div class="form-group">
            <label>提现手续费 (%)</label>
            <input v-model.number="config.withdraw_fee" type="number" min="0" max="100" step="0.1" />
          </div>
        </div>
        <div class="form-group">
          <label>提现方式</label>
          <div class="checkbox-group">
            <label class="checkbox-label">
              <input type="checkbox" value="alipay" v-model="config.withdraw_methods" />
              <span>支付宝</span>
            </label>
            <label class="checkbox-label">
              <input type="checkbox" value="wechat" v-model="config.withdraw_methods" />
              <span>微信</span>
            </label>
            <label class="checkbox-label">
              <input type="checkbox" value="bank" v-model="config.withdraw_methods" />
              <span>银行卡</span>
            </label>
          </div>
        </div>

        <div class="form-actions">
          <button class="btn-primary" @click="saveConfig">保存配置</button>
        </div>
      </div>
    </div>

    <!-- 提现审核 -->
    <div v-show="activeTab === 'withdrawals'">
      <div class="toolbar">
        <select v-model="withdrawalFilter.status">
          <option value="">全部状态</option>
          <option value="pending">待审核</option>
          <option value="approved">已通过</option>
          <option value="rejected">已拒绝</option>
        </select>
        <button class="btn-secondary" @click="fetchWithdrawals">🔍 搜索</button>
      </div>

      <div class="table-container">
        <table class="data-table">
          <thead>
            <tr>
              <th>ID</th>
              <th>用户ID</th>
              <th>金额</th>
              <th>方式</th>
              <th>账号</th>
              <th>状态</th>
              <th>申请时间</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in withdrawals" :key="item.id">
              <td>{{ item.id }}</td>
              <td>{{ item.user_id }}</td>
              <td>¥{{ item.amount }}</td>
              <td>{{ getMethodLabel(item.method) }}</td>
              <td>{{ maskAccount(item.account) }}</td>
              <td>
                <span :class="['status-badge', 'status-' + item.status]">
                  {{ getStatusLabel(item.status) }}
                </span>
              </td>
              <td>{{ formatTime(item.created_at) }}</td>
              <td>
                <div class="action-buttons" v-if="item.status === 'pending'">
                  <button class="btn-sm btn-primary" @click="processWithdrawal(item, true)" title="通过">✓</button>
                  <button class="btn-sm btn-danger" @click="processWithdrawal(item, false)" title="拒绝">✕</button>
                </div>
                <span v-else class="text-secondary">-</span>
              </td>
            </tr>
            <tr v-if="withdrawals.length === 0">
              <td colspan="8" class="empty-row">暂无提现申请</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 统计数据 -->
    <div v-show="activeTab === 'stats'">
      <div class="stats-grid">
        <div class="stat-card">
          <div class="stat-value">{{ stats.total_invites || 0 }}</div>
          <div class="stat-label">总邀请数</div>
        </div>
        <div class="stat-card">
          <div class="stat-value">¥{{ stats.total_commission?.toFixed(2) || '0.00' }}</div>
          <div class="stat-label">总佣金</div>
        </div>
        <div class="stat-card">
          <div class="stat-value">¥{{ stats.pending_commission?.toFixed(2) || '0.00' }}</div>
          <div class="stat-label">待发放佣金</div>
        </div>
        <div class="stat-card">
          <div class="stat-value">¥{{ stats.withdrawn_commission?.toFixed(2) || '0.00' }}</div>
          <div class="stat-label">已提现佣金</div>
        </div>
      </div>

      <div class="chart-section">
        <h3>邀请排行</h3>
        <div class="table-container">
          <table class="data-table">
            <thead>
              <tr>
                <th>排名</th>
                <th>用户ID</th>
                <th>邀请人数</th>
                <th>累计佣金</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(item, index) in stats.top_inviters" :key="item.user_id">
                <td>{{ index + 1 }}</td>
                <td>{{ item.user_id }}</td>
                <td>{{ item.invite_count }}</td>
                <td>¥{{ item.commission?.toFixed(2) || '0.00' }}</td>
              </tr>
              <tr v-if="!stats.top_inviters || stats.top_inviters.length === 0">
                <td colspan="4" class="empty-row">暂无数据</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import {
  getInviteConfig, updateInviteConfig, getWithdrawals,
  processWithdrawal, getInviteStats
} from '@/api/admin'

const activeTab = ref('config')
const withdrawals = ref([])
const stats = ref({})

const withdrawalFilter = ref({ status: '' })

const config = ref({
  enabled: false,
  code_prefix: 'INV',
  code_length: 8,
  commission_rate: 10,
  commission_type: 'percent',
  commission_fixed: 0,
  min_withdraw: 10,
  withdraw_fee: 0,
  withdraw_methods: ['alipay']
})

const methodLabels = {
  alipay: '支付宝',
  wechat: '微信',
  bank: '银行卡'
}

const statusLabels = {
  pending: '待审核',
  approved: '已通过',
  rejected: '已拒绝'
}

const getMethodLabel = (method) => methodLabels[method] || method
const getStatusLabel = (status) => statusLabels[status] || status

const formatTime = (time) => {
  if (!time) return '-'
  return new Date(time).toLocaleString()
}

const maskAccount = (account) => {
  if (!account) return '-'
  if (account.length <= 4) return account
  return account.substring(0, 2) + '***' + account.substring(account.length - 2)
}

const fetchConfig = async () => {
  try {
    const res = await getInviteConfig()
    if (res.data) {
      config.value = { ...config.value, ...res.data }
    }
  } catch (err) {
    console.error('获取配置失败:', err)
  }
}

const saveConfig = async () => {
  try {
    await updateInviteConfig(config.value)
    alert('保存成功')
  } catch (err) {
    alert('保存失败: ' + (err.response?.data?.error || err.message))
  }
}

const fetchWithdrawals = async () => {
  try {
    const res = await getWithdrawals(withdrawalFilter.value)
    withdrawals.value = res.data?.list || []
  } catch (err) {
    console.error('获取提现列表失败:', err)
  }
}

const processWithdrawalRequest = async (item, approve) => {
  const action = approve ? '通过' : '拒绝'
  if (!confirm(`确定${action}该提现申请?`)) return

  try {
    await processWithdrawal(item.id, { approve })
    alert(`${action}成功`)
    fetchWithdrawals()
    fetchStats()
  } catch (err) {
    alert(`${action}失败: ` + (err.response?.data?.error || err.message))
  }
}

const fetchStats = async () => {
  try {
    const res = await getInviteStats()
    stats.value = res.data || {}
  } catch (err) {
    console.error('获取统计失败:', err)
  }
}

onMounted(() => {
  fetchConfig()
  fetchWithdrawals()
  fetchStats()
})
</script>

<style scoped>
.config-section {
  background: var(--surface-color);
  padding: 24px;
  border-radius: var(--radius-lg);
  border: 1px solid var(--border-color);
}

.config-section h3 {
  margin-bottom: 16px;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--border-color);
}

.config-section h4 {
  margin-top: 24px;
  margin-bottom: 12px;
}

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

.checkbox-group {
  display: flex;
  gap: 16px;
  margin-top: 8px;
}

.help-text {
  color: var(--text-secondary);
  font-size: 12px;
  margin-top: 4px;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 20px;
  margin-bottom: 30px;
}

.stat-card {
  background: var(--surface-color);
  padding: 24px;
  border-radius: var(--radius-lg);
  text-align: center;
  border: 1px solid var(--border-color);
}

.stat-value {
  font-size: 2rem;
  font-weight: 700;
  color: var(--primary-color);
}

.stat-label {
  color: var(--text-secondary);
  margin-top: 8px;
}

.chart-section {
  background: var(--surface-color);
  padding: 24px;
  border-radius: var(--radius-lg);
  border: 1px solid var(--border-color);
}

.chart-section h3 {
  margin-bottom: 16px;
}

.status-pending { background: rgba(251, 191, 36, 0.15); color: #fbbf24; }
.status-approved { background: rgba(34, 197, 94, 0.15); color: #22c55e; }
.status-rejected { background: rgba(239, 68, 68, 0.15); color: #ef4444; }

.btn-danger {
  background: rgba(239, 68, 68, 0.1);
  color: #ef4444;
  border: 1px solid rgba(239, 68, 68, 0.2);
}

.btn-danger:hover {
  background: rgba(239, 68, 68, 0.2);
}
</style>