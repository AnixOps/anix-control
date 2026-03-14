<template>
  <div class="forward-page">
    <div class="page-header">
      <h1>流量转发管理</h1>
      <p class="text-secondary">管理中转节点和转发规则</p>
    </div>

    <!-- 标签切换 -->
    <div class="tabs">
      <button :class="['tab', { active: activeTab === 'nodes' }]" @click="activeTab = 'nodes'">
        中转节点
      </button>
      <button :class="['tab', { active: activeTab === 'rules' }]" @click="activeTab = 'rules'">
        转发规则
      </button>
    </div>

    <!-- 节点管理 -->
    <div v-show="activeTab === 'nodes'">
      <div class="toolbar">
        <select v-model="nodeFilter.type">
          <option value="">全部类型</option>
          <option value="relay">中转节点</option>
          <option value="exit">落地节点</option>
        </select>
        <select v-model="nodeFilter.status">
          <option value="">全部状态</option>
          <option value="1">在线</option>
          <option value="0">离线</option>
        </select>
        <button class="btn-secondary" @click="fetchNodes">🔍 搜索</button>
        <button class="btn-primary" @click="openNodeModal()">➕ 新增节点</button>
      </div>

      <div class="table-container">
        <table class="data-table">
          <thead>
            <tr>
              <th>ID</th>
              <th>名称</th>
              <th>类型</th>
              <th>地址</th>
              <th>地区</th>
              <th>延迟</th>
              <th>状态</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="node in nodes" :key="node.id">
              <td>{{ node.id }}</td>
              <td>{{ node.name }}</td>
              <td>
                <span :class="['type-badge', node.type]">
                  {{ node.type === 'relay' ? '中转' : '落地' }}
                </span>
              </td>
              <td>{{ node.host }}:{{ node.port }}</td>
              <td>{{ node.region || '-' }}</td>
              <td>{{ node.latency ? node.latency + 'ms' : '-' }}</td>
              <td>
                <span :class="['status-badge', node.status === 1 ? 'status-active' : 'status-offline']">
                  {{ node.status === 1 ? '在线' : '离线' }}
                </span>
              </td>
              <td>
                <div class="action-buttons">
                  <button class="btn-sm btn-ghost" @click="checkNode(node)" title="检查">🔍</button>
                  <button class="btn-sm btn-ghost" @click="openNodeModal(node)" title="编辑">✏️</button>
                  <button class="btn-sm btn-ghost" @click="deleteNode(node)" title="删除">🗑️</button>
                </div>
              </td>
            </tr>
            <tr v-if="nodes.length === 0">
              <td colspan="8" class="empty-row">暂无节点数据</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 规则管理 -->
    <div v-show="activeTab === 'rules'">
      <div class="toolbar">
        <button class="btn-primary" @click="openRuleModal()">➕ 新增规则</button>
      </div>

      <div class="table-container">
        <table class="data-table">
          <thead>
            <tr>
              <th>ID</th>
              <th>名称</th>
              <th>中转节点</th>
              <th>监听端口</th>
              <th>目标地址</th>
              <th>协议</th>
              <th>状态</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="rule in rules" :key="rule.id">
              <td>{{ rule.id }}</td>
              <td>{{ rule.name }}</td>
              <td>{{ rule.relay_node_id }}</td>
              <td>{{ rule.listen_port }}</td>
              <td>{{ rule.target_host }}:{{ rule.target_port }}</td>
              <td>{{ rule.protocol }}</td>
              <td>
                <span :class="['status-badge', rule.enabled ? 'status-active' : 'status-disabled']">
                  {{ rule.enabled ? '启用' : '禁用' }}
                </span>
              </td>
              <td>
                <div class="action-buttons">
                  <button class="btn-sm btn-ghost" @click="toggleRule(rule)" title="切换状态">
                    {{ rule.enabled ? '🔴' : '🟢' }}
                  </button>
                  <button class="btn-sm btn-ghost" @click="openRuleModal(rule)" title="编辑">✏️</button>
                  <button class="btn-sm btn-ghost" @click="deleteRule(rule)" title="删除">🗑️</button>
                </div>
              </td>
            </tr>
            <tr v-if="rules.length === 0">
              <td colspan="8" class="empty-row">暂无规则数据</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 节点弹窗 -->
    <div v-if="showNodeModal" class="modal-overlay" @click.self="showNodeModal = false">
      <div class="modal">
        <div class="modal-header">
          <h3>{{ editingNode ? '编辑节点' : '新增节点' }}</h3>
          <button class="close-btn" @click="showNodeModal = false">✕</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label>名称 <span class="required">*</span></label>
            <input v-model="nodeForm.name" type="text" placeholder="节点名称" />
          </div>
          <div class="form-row">
            <div class="form-group">
              <label>类型</label>
              <select v-model="nodeForm.type">
                <option value="relay">中转节点</option>
                <option value="exit">落地节点</option>
              </select>
            </div>
            <div class="form-group">
              <label>地区</label>
              <input v-model="nodeForm.region" type="text" placeholder="如: HK, US, JP" />
            </div>
          </div>
          <div class="form-row">
            <div class="form-group">
              <label>地址 <span class="required">*</span></label>
              <input v-model="nodeForm.host" type="text" placeholder="IP或域名" />
            </div>
            <div class="form-group">
              <label>端口 <span class="required">*</span></label>
              <input v-model.number="nodeForm.port" type="number" placeholder="端口号" />
            </div>
          </div>
          <div class="form-group">
            <label>带宽 (Mbps)</label>
            <input v-model.number="nodeForm.bandwidth" type="number" />
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn-secondary" @click="showNodeModal = false">取消</button>
          <button @click="saveNode">保存</button>
        </div>
      </div>
    </div>

    <!-- 规则弹窗 -->
    <div v-if="showRuleModal" class="modal-overlay" @click.self="showRuleModal = false">
      <div class="modal">
        <div class="modal-header">
          <h3>{{ editingRule ? '编辑规则' : '新增规则' }}</h3>
          <button class="close-btn" @click="showRuleModal = false">✕</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label>名称 <span class="required">*</span></label>
            <input v-model="ruleForm.name" type="text" placeholder="规则名称" />
          </div>
          <div class="form-row">
            <div class="form-group">
              <label>中转节点ID</label>
              <input v-model.number="ruleForm.relay_node_id" type="number" />
            </div>
            <div class="form-group">
              <label>落地节点ID</label>
              <input v-model.number="ruleForm.exit_node_id" type="number" />
            </div>
          </div>
          <div class="form-row">
            <div class="form-group">
              <label>监听端口</label>
              <input v-model.number="ruleForm.listen_port" type="number" />
            </div>
            <div class="form-group">
              <label>协议</label>
              <select v-model="ruleForm.protocol">
                <option value="tcp">TCP</option>
                <option value="udp">UDP</option>
                <option value="both">TCP+UDP</option>
              </select>
            </div>
          </div>
          <div class="form-row">
            <div class="form-group">
              <label>目标地址</label>
              <input v-model="ruleForm.target_host" type="text" placeholder="目标IP或域名" />
            </div>
            <div class="form-group">
              <label>目标端口</label>
              <input v-model.number="ruleForm.target_port" type="number" />
            </div>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn-secondary" @click="showRuleModal = false">取消</button>
          <button @click="saveRule">保存</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import {
  getForwardNodes, createForwardNode, updateForwardNode, deleteForwardNode,
  checkForwardNode, getForwardRules, createForwardRule, updateForwardRule,
  deleteForwardRule, toggleForwardRule
} from '@/api/admin'

const activeTab = ref('nodes')
const nodes = ref([])
const rules = ref([])
const nodeFilter = ref({ type: '', status: '' })

const showNodeModal = ref(false)
const editingNode = ref(null)
const nodeForm = ref({
  name: '', type: 'relay', host: '', port: 0, region: '', bandwidth: 0
})

const showRuleModal = ref(false)
const editingRule = ref(null)
const ruleForm = ref({
  name: '', relay_node_id: 0, exit_node_id: 0, listen_port: 0,
  protocol: 'tcp', target_host: '', target_port: 0
})

const fetchNodes = async () => {
  try {
    const res = await getForwardNodes(nodeFilter.value)
    nodes.value = res.data?.list || []
  } catch (err) {
    console.error('获取节点失败:', err)
  }
}

const fetchRules = async () => {
  try {
    const res = await getForwardRules({})
    rules.value = res.data?.list || []
  } catch (err) {
    console.error('获取规则失败:', err)
  }
}

const openNodeModal = (node = null) => {
  if (node) {
    editingNode.value = node
    nodeForm.value = { ...node }
  } else {
    editingNode.value = null
    nodeForm.value = { name: '', type: 'relay', host: '', port: 0, region: '', bandwidth: 0 }
  }
  showNodeModal.value = true
}

const saveNode = async () => {
  try {
    if (editingNode.value) {
      await updateForwardNode(editingNode.value.id, nodeForm.value)
    } else {
      await createForwardNode(nodeForm.value)
    }
    showNodeModal.value = false
    fetchNodes()
  } catch (err) {
    alert('保存失败: ' + (err.response?.data?.error || err.message))
  }
}

const checkNode = async (node) => {
  try {
    await checkForwardNode(node.id)
    alert('节点检查完成')
    fetchNodes()
  } catch (err) {
    alert('检查失败')
  }
}

const deleteNode = async (node) => {
  if (!confirm(`确定删除节点 ${node.name}?`)) return
  try {
    await deleteForwardNode(node.id)
    fetchNodes()
  } catch (err) {
    alert('删除失败')
  }
}

const openRuleModal = (rule = null) => {
  if (rule) {
    editingRule.value = rule
    ruleForm.value = { ...rule }
  } else {
    editingRule.value = null
    ruleForm.value = {
      name: '', relay_node_id: 0, exit_node_id: 0, listen_port: 0,
      protocol: 'tcp', target_host: '', target_port: 0
    }
  }
  showRuleModal.value = true
}

const saveRule = async () => {
  try {
    if (editingRule.value) {
      await updateForwardRule(editingRule.value.id, ruleForm.value)
    } else {
      await createForwardRule(ruleForm.value)
    }
    showRuleModal.value = false
    fetchRules()
  } catch (err) {
    alert('保存失败: ' + (err.response?.data?.error || err.message))
  }
}

const toggleRule = async (rule) => {
  try {
    await toggleForwardRule(rule.id, !rule.enabled)
    fetchRules()
  } catch (err) {
    alert('操作失败')
  }
}

const deleteRule = async (rule) => {
  if (!confirm(`确定删除规则 ${rule.name}?`)) return
  try {
    await deleteForwardRule(rule.id)
    fetchRules()
  } catch (err) {
    alert('删除失败')
  }
}

onMounted(() => {
  fetchNodes()
  fetchRules()
})
</script>

<style scoped>
.tabs {
  display: flex;
  gap: 8px;
  margin-bottom: 20px;
}

.tab {
  padding: 10px 20px;
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: var(--transition);
}

.tab.active {
  background: var(--primary-color);
  color: white;
  border-color: var(--primary-color);
}

.toolbar {
  display: flex;
  gap: 12px;
  margin-bottom: 16px;
  flex-wrap: wrap;
}

.toolbar select {
  min-width: 120px;
}

.type-badge {
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 4px;
}

.type-badge.relay {
  background: rgba(59, 130, 246, 0.15);
  color: #3b82f6;
}

.type-badge.exit {
  background: rgba(139, 92, 246, 0.15);
  color: #8b5cf6;
}

.status-offline {
  background: rgba(239, 68, 68, 0.15);
  color: var(--error-color);
}

.status-disabled {
  background: rgba(107, 114, 128, 0.15);
  color: var(--text-secondary);
}

.form-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
}
</style>