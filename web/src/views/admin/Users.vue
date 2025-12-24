<template>
  <div class="users-page">
    <div class="page-header">
      <h1>用户管理</h1>
      <p class="text-secondary">管理所有注册用户</p>
    </div>

    <!-- 统计卡片 -->
    <div class="stats-grid">
      <div class="stat-card">
        <div class="stat-icon">👥</div>
        <div class="stat-info">
          <div class="stat-value">{{ stats.total_users || 0 }}</div>
          <div class="stat-label">总用户数</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon">✅</div>
        <div class="stat-info">
          <div class="stat-value">{{ stats.active_users || 0 }}</div>
          <div class="stat-label">有效用户</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon">⏰</div>
        <div class="stat-info">
          <div class="stat-value">{{ stats.expired_users || 0 }}</div>
          <div class="stat-label">已过期</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon">🚫</div>
        <div class="stat-info">
          <div class="stat-value">{{ stats.banned_users || 0 }}</div>
          <div class="stat-label">已封禁</div>
        </div>
      </div>
    </div>

    <!-- 筛选栏 -->
    <div class="filter-bar">
      <input
        v-model="filters.email"
        type="text"
        placeholder="搜索邮箱..."
        class="search-input"
        @keyup.enter="fetchUsers"
      />
      <select v-model="filters.status" @change="fetchUsers">
        <option value="">全部状态</option>
        <option value="active">有效</option>
        <option value="expired">已过期</option>
        <option value="banned">已封禁</option>
      </select>
      <button class="btn-secondary" @click="fetchUsers">
        🔍 搜索
      </button>
      <button class="btn-primary" @click="showCreateModal = true">
        ➕ 新增用户
      </button>
    </div>

    <!-- 用户列表 -->
    <div class="table-container">
      <table class="data-table">
        <thead>
          <tr>
            <th>ID</th>
            <th>邮箱</th>
            <th>套餐</th>
            <th>流量</th>
            <th>到期时间</th>
            <th>状态</th>
            <th>注册时间</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="user in users" :key="user.id">
            <td>{{ user.id }}</td>
            <td>
              <div class="user-email">
                <span v-if="user.is_admin === 1" class="admin-badge">管理员</span>
                {{ user.email }}
              </div>
            </td>
            <td>{{ user.plan?.name || '-' }}</td>
            <td>
              <div class="traffic-info">
                {{ formatBytes(user.u + user.d) }} / {{ formatBytes(user.transfer_enable) }}
              </div>
            </td>
            <td>{{ formatDate(user.expired_at) }}</td>
            <td>
              <span :class="['status-badge', getStatusClass(user)]">
                {{ getStatusText(user) }}
              </span>
            </td>
            <td>{{ formatDateTime(user.created_at) }}</td>
            <td>
              <div class="action-buttons">
                <button class="btn-sm btn-ghost" @click="editUser(user)" title="编辑">
                  ✏️
                </button>
                <button 
                  v-if="user.banned === 0"
                  class="btn-sm btn-ghost" 
                  @click="handleBan(user)" 
                  title="封禁"
                >
                  🚫
                </button>
                <button 
                  v-else
                  class="btn-sm btn-ghost" 
                  @click="handleUnban(user)" 
                  title="解封"
                >
                  ✅
                </button>
                <button class="btn-sm btn-ghost" @click="handleResetTraffic(user)" title="重置流量">
                  🔄
                </button>
              </div>
            </td>
          </tr>
          <tr v-if="users.length === 0">
            <td colspan="8" class="empty-row">暂无数据</td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- 分页 -->
    <div class="pagination">
      <button 
        class="btn-sm btn-secondary" 
        :disabled="page <= 1"
        @click="page--; fetchUsers()"
      >
        上一页
      </button>
      <span class="page-info">第 {{ page }} 页 / 共 {{ totalPages }} 页</span>
      <button 
        class="btn-sm btn-secondary"
        :disabled="page >= totalPages"
        @click="page++; fetchUsers()"
      >
        下一页
      </button>
    </div>

    <!-- 编辑弹窗 -->
    <div v-if="showEditModal" class="modal-overlay" @click.self="showEditModal = false">
      <div class="modal">
        <div class="modal-header">
          <h3>编辑用户</h3>
          <button class="close-btn" @click="showEditModal = false">✕</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label>邮箱</label>
            <input v-model="editingUser.email" type="email" />
          </div>
          <div class="form-group">
            <label>余额 (分)</label>
            <input v-model.number="editingUser.balance" type="number" />
          </div>
          <div class="form-group">
            <label>流量限制 (字节)</label>
            <input v-model.number="editingUser.transfer_enable" type="number" />
          </div>
          <div class="form-group">
            <label>到期时间 (Unix时间戳)</label>
            <input v-model.number="editingUser.expired_at" type="number" />
          </div>
          <div class="form-group">
            <label>备注</label>
            <textarea v-model="editingUser.remark_content" rows="3"></textarea>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn-secondary" @click="showEditModal = false">取消</button>
          <button @click="saveUser">保存</button>
        </div>
      </div>
    </div>

    <!-- 新增用户弹窗 -->
    <div v-if="showCreateModal" class="modal-overlay" @click.self="showCreateModal = false">
      <div class="modal">
        <div class="modal-header">
          <h3>新增用户</h3>
          <button class="close-btn" @click="showCreateModal = false">✕</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label>邮箱 <span class="required">*</span></label>
            <input v-model="newUser.email" type="email" placeholder="请输入邮箱" />
          </div>
          <div class="form-group">
            <label>密码 <span class="required">*</span></label>
            <input v-model="newUser.password" type="password" placeholder="请输入密码（至少6位）" />
          </div>
          <div class="form-group">
            <label>用户类型</label>
            <select v-model="newUser.is_admin">
              <option :value="0">普通用户</option>
              <option :value="1">管理员</option>
            </select>
          </div>
          <div v-if="createError" class="error-msg">{{ createError }}</div>
        </div>
        <div class="modal-footer">
          <button class="btn-secondary" @click="showCreateModal = false">取消</button>
          <button @click="handleCreateUser" :disabled="createLoading">
            {{ createLoading ? '创建中...' : '创建' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { getUserList, getUserStats, updateUser, banUser, unbanUser, resetUserTraffic, createUser } from '@/api/admin'

const users = ref([])
const stats = ref({})
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const filters = ref({
  email: '',
  status: ''
})
const showEditModal = ref(false)
const editingUser = ref({})

// 新增用户相关
const showCreateModal = ref(false)
const createLoading = ref(false)
const createError = ref('')
const newUser = ref({
  email: '',
  password: '',
  is_admin: 0
})

const totalPages = computed(() => Math.ceil(total.value / pageSize.value) || 1)

// 创建用户
const handleCreateUser = async () => {
  if (!newUser.value.email || !newUser.value.password) {
    createError.value = '请填写邮箱和密码'
    return
  }
  if (newUser.value.password.length < 6) {
    createError.value = '密码长度至少6位'
    return
  }
  
  createLoading.value = true
  createError.value = ''
  
  try {
    await createUser(newUser.value)
    showCreateModal.value = false
    newUser.value = { email: '', password: '', is_admin: 0 }
    fetchUsers()
    fetchStats()
    alert('用户创建成功')
  } catch (err) {
    createError.value = err.response?.data?.message || '创建失败'
  } finally {
    createLoading.value = false
  }
}

const fetchUsers = async () => {
  try {
    const res = await getUserList({
      page: page.value,
      page_size: pageSize.value,
      email: filters.value.email,
      status: filters.value.status
    })
    users.value = res.data?.list || []
    total.value = res.data?.total || 0
  } catch (err) {
    console.error('获取用户列表失败:', err)
  }
}

const fetchStats = async () => {
  try {
    const res = await getUserStats()
    stats.value = res.data || {}
  } catch (err) {
    console.error('获取统计失败:', err)
  }
}

const editUser = (user) => {
  editingUser.value = { ...user }
  showEditModal.value = true
}

const saveUser = async () => {
  try {
    await updateUser(editingUser.value.id, {
      email: editingUser.value.email,
      balance: editingUser.value.balance,
      transfer_enable: editingUser.value.transfer_enable,
      expired_at: editingUser.value.expired_at,
      remark_content: editingUser.value.remark_content
    })
    showEditModal.value = false
    fetchUsers()
  } catch (err) {
    alert('保存失败: ' + (err.response?.data?.message || err.message))
  }
}

const handleBan = async (user) => {
  if (!confirm(`确定要封禁用户 ${user.email} 吗?`)) return
  try {
    await banUser(user.id)
    fetchUsers()
    fetchStats()
  } catch (err) {
    alert('操作失败')
  }
}

const handleUnban = async (user) => {
  if (!confirm(`确定要解封用户 ${user.email} 吗?`)) return
  try {
    await unbanUser(user.id)
    fetchUsers()
    fetchStats()
  } catch (err) {
    alert('操作失败')
  }
}

const handleResetTraffic = async (user) => {
  if (!confirm(`确定要重置用户 ${user.email} 的流量吗?`)) return
  try {
    await resetUserTraffic(user.id)
    fetchUsers()
  } catch (err) {
    alert('操作失败')
  }
}

const getStatusClass = (user) => {
  if (user.banned === 1) return 'status-banned'
  if (user.expired_at && user.expired_at < Date.now() / 1000) return 'status-expired'
  return 'status-active'
}

const getStatusText = (user) => {
  if (user.banned === 1) return '已封禁'
  if (user.expired_at && user.expired_at < Date.now() / 1000) return '已过期'
  return '有效'
}

const formatBytes = (bytes) => {
  if (!bytes) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  while (bytes >= 1024 && i < units.length - 1) {
    bytes /= 1024
    i++
  }
  return `${bytes.toFixed(2)} ${units[i]}`
}

const formatDate = (timestamp) => {
  if (!timestamp) return '永久'
  return new Date(timestamp * 1000).toLocaleDateString('zh-CN')
}

const formatDateTime = (datetime) => {
  if (!datetime) return '-'
  return new Date(datetime).toLocaleString('zh-CN')
}

onMounted(() => {
  fetchUsers()
  fetchStats()
})
</script>

<style scoped>
.users-page {
  max-width: 1400px;
}

.page-header {
  margin-bottom: 24px;
}

.page-header h1 {
  font-size: 24px;
  margin-bottom: 4px;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 16px;
  margin-bottom: 24px;
}

@media (max-width: 768px) {
  .stats-grid {
    grid-template-columns: repeat(2, 1fr);
  }
}

.stat-card {
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  padding: 20px;
  display: flex;
  align-items: center;
  gap: 16px;
}

.stat-icon {
  font-size: 32px;
}

.stat-value {
  font-size: 24px;
  font-weight: 700;
}

.stat-label {
  font-size: 13px;
  color: var(--text-secondary);
}

.filter-bar {
  display: flex;
  gap: 12px;
  margin-bottom: 16px;
  flex-wrap: wrap;
}

.search-input {
  flex: 1;
  min-width: 200px;
}

.filter-bar select {
  min-width: 120px;
}

.table-container {
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  overflow-x: auto;
}

.data-table {
  width: 100%;
  border-collapse: collapse;
}

.data-table th,
.data-table td {
  padding: 14px 16px;
  text-align: left;
  border-bottom: 1px solid var(--border-color);
}

.data-table th {
  background: var(--bg-color);
  font-weight: 600;
  font-size: 13px;
  color: var(--text-secondary);
  text-transform: uppercase;
}

.data-table tr:hover {
  background: var(--bg-color);
}

.user-email {
  display: flex;
  align-items: center;
  gap: 8px;
}

.admin-badge {
  font-size: 10px;
  padding: 2px 6px;
  background: var(--primary-color);
  border-radius: 4px;
  font-weight: 600;
}

.status-badge {
  font-size: 12px;
  padding: 4px 10px;
  border-radius: 20px;
  font-weight: 500;
}

.status-active {
  background: rgba(34, 197, 94, 0.15);
  color: var(--success-color);
}

.status-expired {
  background: rgba(245, 158, 11, 0.15);
  color: var(--warning-color);
}

.status-banned {
  background: rgba(239, 68, 68, 0.15);
  color: var(--error-color);
}

.action-buttons {
  display: flex;
  gap: 4px;
}

.empty-row {
  text-align: center;
  color: var(--text-secondary);
  padding: 40px !important;
}

.pagination {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 16px;
  margin-top: 20px;
}

.page-info {
  font-size: 14px;
  color: var(--text-secondary);
}

/* 弹窗样式 */
.modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.7);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  padding: 20px;
}

.modal {
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  width: 100%;
  max-width: 500px;
  max-height: 90vh;
  overflow-y: auto;
}

.modal-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 20px;
  border-bottom: 1px solid var(--border-color);
}

.modal-header h3 {
  font-size: 18px;
  font-weight: 600;
}

.close-btn {
  background: transparent;
  border: none;
  font-size: 18px;
  color: var(--text-secondary);
  cursor: pointer;
  padding: 4px;
}

.modal-body {
  padding: 20px;
}

.modal-body .form-group {
  margin-bottom: 16px;
}

.modal-body label {
  display: block;
  margin-bottom: 6px;
  font-size: 14px;
  font-weight: 500;
}

.modal-body textarea {
  resize: vertical;
}

.modal-body .error-msg {
  margin-top: 12px;
  padding: 10px 14px;
  background: rgba(239, 68, 68, 0.1);
  border: 1px solid rgba(239, 68, 68, 0.3);
  border-radius: var(--radius-sm);
  color: var(--error-color);
  font-size: 14px;
}

.modal-body .required {
  color: var(--error-color);
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  padding: 16px 20px;
  border-top: 1px solid var(--border-color);
}

.modal-footer button {
  min-width: 80px;
}
</style>
