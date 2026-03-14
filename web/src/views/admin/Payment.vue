<template>
  <div class="payment-page">
    <div class="page-header">
      <h1>支付网关管理</h1>
      <p class="text-secondary">配置支付渠道和查看支付记录</p>
    </div>

    <!-- 标签切换 -->
    <div class="tabs">
      <button :class="['tab', { active: activeTab === 'gateways' }]" @click="activeTab = 'gateways'">
        支付网关
      </button>
      <button :class="['tab', { active: activeTab === 'records' }]" @click="activeTab = 'records'">
        支付记录
      </button>
      <button :class="['tab', { active: activeTab === 'stats' }]" @click="activeTab = 'stats'">
        统计数据
      </button>
    </div>

    <!-- 网关管理 -->
    <div v-show="activeTab === 'gateways'">
      <div class="toolbar">
        <button class="btn-primary" @click="openGatewayModal()">➕ 新增网关</button>
      </div>

      <div class="table-container">
        <table class="data-table">
          <thead>
            <tr>
              <th>ID</th>
              <th>名称</th>
              <th>类型</th>
              <th>手续费率</th>
              <th>最小金额</th>
              <th>最大金额</th>
              <th>状态</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="gateway in gateways" :key="gateway.id">
              <td>{{ gateway.id }}</td>
              <td>{{ gateway.name }}</td>
              <td>
                <span :class="['type-badge', gateway.type]">
                  {{ getGatewayTypeLabel(gateway.type) }}
                </span>
              </td>
              <td>{{ (gateway.fee_rate * 100).toFixed(2) }}%</td>
              <td>¥{{ gateway.min_amount }}</td>
              <td>¥{{ gateway.max_amount }}</td>
              <td>
                <span :class="['status-badge', gateway.enabled ? 'status-active' : 'status-disabled']">
                  {{ gateway.enabled ? '启用' : '禁用' }}
                </span>
              </td>
              <td>
                <div class="action-buttons">
                  <button class="btn-sm btn-ghost" @click="toggleGateway(gateway)" title="切换状态">
                    {{ gateway.enabled ? '🔴' : '🟢' }}
                  </button>
                  <button class="btn-sm btn-ghost" @click="openGatewayModal(gateway)" title="编辑">✏️</button>
                  <button class="btn-sm btn-ghost" @click="deleteGateway(gateway)" title="删除">🗑️</button>
                </div>
              </td>
            </tr>
            <tr v-if="gateways.length === 0">
              <td colspan="8" class="empty-row">暂无网关数据</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 支付记录 -->
    <div v-show="activeTab === 'records'">
      <div class="toolbar">
        <select v-model="recordFilter.status">
          <option value="">全部状态</option>
          <option value="pending">待支付</option>
          <option value="paid">已支付</option>
          <option value="failed">失败</option>
          <option value="refunded">已退款</option>
        </select>
        <select v-model="recordFilter.gateway_type">
          <option value="">全部类型</option>
          <option value="alipay">支付宝</option>
          <option value="wechat">微信支付</option>
          <option value="stripe">Stripe</option>
          <option value="usdt">USDT</option>
          <option value="epay">EPay</option>
        </select>
        <button class="btn-secondary" @click="fetchRecords">🔍 搜索</button>
      </div>

      <div class="table-container">
        <table class="data-table">
          <thead>
            <tr>
              <th>ID</th>
              <th>交易号</th>
              <th>用户ID</th>
              <th>网关</th>
              <th>金额</th>
              <th>状态</th>
              <th>创建时间</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="record in records" :key="record.id">
              <td>{{ record.id }}</td>
              <td>{{ record.trade_no }}</td>
              <td>{{ record.user_id }}</td>
              <td>{{ getGatewayTypeLabel(record.gateway_type) }}</td>
              <td>¥{{ record.amount }}</td>
              <td>
                <span :class="['status-badge', 'status-' + record.status]">
                  {{ getStatusLabel(record.status) }}
                </span>
              </td>
              <td>{{ formatTime(record.created_at) }}</td>
              <td>
                <div class="action-buttons">
                  <button class="btn-sm btn-ghost" @click="viewRecord(record)" title="详情">👁️</button>
                </div>
              </td>
            </tr>
            <tr v-if="records.length === 0">
              <td colspan="8" class="empty-row">暂无支付记录</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 统计数据 -->
    <div v-show="activeTab === 'stats'">
      <div class="stats-grid">
        <div class="stat-card">
          <div class="stat-value">¥{{ stats.total_amount?.toFixed(2) || '0.00' }}</div>
          <div class="stat-label">总收入</div>
        </div>
        <div class="stat-card">
          <div class="stat-value">{{ stats.total_orders || 0 }}</div>
          <div class="stat-label">总订单数</div>
        </div>
        <div class="stat-card">
          <div class="stat-value">{{ stats.success_orders || 0 }}</div>
          <div class="stat-label">成功订单</div>
        </div>
        <div class="stat-card">
          <div class="stat-value">{{ stats.success_rate?.toFixed(1) || 0 }}%</div>
          <div class="stat-label">成功率</div>
        </div>
      </div>

      <div class="chart-section">
        <h3>支付渠道分布</h3>
        <div class="gateway-stats">
          <div v-for="(item, type) in stats.by_gateway" :key="type" class="gateway-stat-item">
            <span class="gateway-name">{{ getGatewayTypeLabel(type) }}</span>
            <div class="progress-bar">
              <div class="progress-fill" :style="{ width: getGatewayPercent(type) + '%' }"></div>
            </div>
            <span class="gateway-amount">¥{{ item.amount?.toFixed(2) || '0.00' }}</span>
          </div>
        </div>
      </div>
    </div>

    <!-- 网关弹窗 -->
    <div v-if="showGatewayModal" class="modal-overlay" @click.self="showGatewayModal = false">
      <div class="modal">
        <div class="modal-header">
          <h3>{{ editingGateway ? '编辑网关' : '新增网关' }}</h3>
          <button class="close-btn" @click="showGatewayModal = false">✕</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label>名称 <span class="required">*</span></label>
            <input v-model="gatewayForm.name" type="text" placeholder="网关名称" />
          </div>
          <div class="form-row">
            <div class="form-group">
              <label>类型</label>
              <select v-model="gatewayForm.type">
                <option value="alipay">支付宝</option>
                <option value="wechat">微信支付</option>
                <option value="stripe">Stripe</option>
                <option value="usdt">USDT</option>
                <option value="epay">EPay</option>
              </select>
            </div>
            <div class="form-group">
              <label>手续费率</label>
              <input v-model.number="gatewayForm.fee_rate" type="number" step="0.001" placeholder="如: 0.01 = 1%" />
            </div>
          </div>
          <div class="form-row">
            <div class="form-group">
              <label>最小金额</label>
              <input v-model.number="gatewayForm.min_amount" type="number" placeholder="最小支付金额" />
            </div>
            <div class="form-group">
              <label>最大金额</label>
              <input v-model.number="gatewayForm.max_amount" type="number" placeholder="最大支付金额" />
            </div>
          </div>
          <div class="form-group">
            <label>配置 (JSON)</label>
            <textarea v-model="gatewayForm.config_json" rows="4" placeholder='{"app_id": "", "private_key": ""}'></textarea>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn-secondary" @click="showGatewayModal = false">取消</button>
          <button @click="saveGateway">保存</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue'
import {
  getPaymentGateways, createPaymentGateway, updatePaymentGateway,
  deletePaymentGateway, togglePaymentGateway, getPaymentRecords,
  getPaymentStats
} from '@/api/admin'

const activeTab = ref('gateways')
const gateways = ref([])
const records = ref([])
const stats = ref({})

const recordFilter = ref({ status: '', gateway_type: '' })

const showGatewayModal = ref(false)
const editingGateway = ref(null)
const gatewayForm = ref({
  name: '', type: 'alipay', fee_rate: 0, min_amount: 0, max_amount: 0, config_json: ''
})

const gatewayTypes = {
  alipay: '支付宝',
  wechat: '微信支付',
  stripe: 'Stripe',
  usdt: 'USDT',
  epay: 'EPay'
}

const statusLabels = {
  pending: '待支付',
  paid: '已支付',
  failed: '失败',
  refunded: '已退款'
}

const getGatewayTypeLabel = (type) => gatewayTypes[type] || type
const getStatusLabel = (status) => statusLabels[status] || status

const formatTime = (time) => {
  if (!time) return '-'
  return new Date(time).toLocaleString()
}

const getGatewayPercent = (type) => {
  if (!stats.value.by_gateway || !stats.value.total_amount) return 0
  const item = stats.value.by_gateway[type]
  if (!item || !item.amount) return 0
  return (item.amount / stats.value.total_amount * 100).toFixed(1)
}

const fetchGateways = async () => {
  try {
    const res = await getPaymentGateways()
    gateways.value = res.data?.list || []
  } catch (err) {
    console.error('获取网关失败:', err)
  }
}

const fetchRecords = async () => {
  try {
    const res = await getPaymentRecords(recordFilter.value)
    records.value = res.data?.list || []
  } catch (err) {
    console.error('获取记录失败:', err)
  }
}

const fetchStats = async () => {
  try {
    const res = await getPaymentStats()
    stats.value = res.data || {}
  } catch (err) {
    console.error('获取统计失败:', err)
  }
}

const openGatewayModal = (gateway = null) => {
  if (gateway) {
    editingGateway.value = gateway
    gatewayForm.value = {
      name: gateway.name,
      type: gateway.type,
      fee_rate: gateway.fee_rate,
      min_amount: gateway.min_amount,
      max_amount: gateway.max_amount,
      config_json: typeof gateway.config === 'string' ? gateway.config : JSON.stringify(gateway.config || {})
    }
  } else {
    editingGateway.value = null
    gatewayForm.value = {
      name: '', type: 'alipay', fee_rate: 0, min_amount: 0, max_amount: 0, config_json: ''
    }
  }
  showGatewayModal.value = true
}

const saveGateway = async () => {
  try {
    const data = { ...gatewayForm.value }
    if (data.config_json) {
      try {
        data.config = JSON.parse(data.config_json)
      } catch (e) {
        alert('配置 JSON 格式错误')
        return
      }
    }
    delete data.config_json

    if (editingGateway.value) {
      await updatePaymentGateway(editingGateway.value.id, data)
    } else {
      await createPaymentGateway(data)
    }
    showGatewayModal.value = false
    fetchGateways()
  } catch (err) {
    alert('保存失败: ' + (err.response?.data?.error || err.message))
  }
}

const toggleGateway = async (gateway) => {
  try {
    await togglePaymentGateway(gateway.id)
    fetchGateways()
  } catch (err) {
    alert('操作失败')
  }
}

const deleteGateway = async (gateway) => {
  if (!confirm(`确定删除网关 ${gateway.name}?`)) return
  try {
    await deletePaymentGateway(gateway.id)
    fetchGateways()
  } catch (err) {
    alert('删除失败')
  }
}

const viewRecord = (record) => {
  alert(`交易详情:\n交易号: ${record.trade_no}\n金额: ¥${record.amount}\n状态: ${getStatusLabel(record.status)}`)
}

onMounted(() => {
  fetchGateways()
  fetchRecords()
  fetchStats()
})
</script>

<style scoped>
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

.gateway-stats {
  margin-top: 16px;
}

.gateway-stat-item {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 12px;
}

.gateway-name {
  width: 80px;
  color: var(--text-secondary);
}

.progress-bar {
  flex: 1;
  height: 8px;
  background: var(--border-color);
  border-radius: 4px;
  overflow: hidden;
}

.progress-fill {
  height: 100%;
  background: var(--primary-color);
  transition: width 0.3s;
}

.gateway-amount {
  width: 100px;
  text-align: right;
  font-weight: 500;
}

.type-badge {
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 4px;
}

.type-badge.alipay { background: rgba(0, 132, 255, 0.15); color: #0084ff; }
.type-badge.wechat { background: rgba(7, 193, 96, 0.15); color: #07c160; }
.type-badge.stripe { background: rgba(99, 91, 255, 0.15); color: #635bff; }
.type-badge.usdt { background: rgba(38, 161, 123, 0.15); color: #26a17b; }
.type-badge.epay { background: rgba(255, 153, 0, 0.15); color: #ff9900; }

.status-pending { background: rgba(251, 191, 36, 0.15); color: #fbbf24; }
.status-paid { background: rgba(34, 197, 94, 0.15); color: #22c55e; }
.status-failed { background: rgba(239, 68, 68, 0.15); color: #ef4444; }
.status-refunded { background: rgba(107, 114, 128, 0.15); color: #6b7280; }
</style>