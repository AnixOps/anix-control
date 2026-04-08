<template>
  <div class="forward-nodes-page">
    <div class="toolbar">
      <div class="toolbar-copy">
        <p class="eyebrow">NodeX Topology</p>
        <h2>NodeX Topology + Legacy Rules</h2>
        <p class="toolbar-subtitle">
          这里聚合 NodeX 的 relay/exit 拓扑与 Legacy 规则兼容层。无状态 Ansible 执行机器请在 Ansible Machines 页面单独管理。
        </p>
      </div>
      <div class="toolbar-actions">
        <button class="btn btn-secondary" :disabled="pageBusy" @click="refreshAll">刷新</button>
        <button class="btn btn-secondary" @click="openConnectionModal()">测试连接</button>
        <button class="btn btn-secondary" @click="openRuleEditor()">新增 Legacy 规则</button>
        <router-link class="btn btn-secondary" to="/admin/forward/ansible-machines">Ansible Machines</router-link>
        <router-link class="btn btn-secondary" to="/admin/forward/nodex">NodeX Runtime</router-link>
        <button class="btn btn-primary" @click="openNodeEditor()">新增节点</button>
      </div>
    </div>
    <ForwardSuiteNav />

    <div v-if="feedback.message" :class="['feedback', `feedback-${feedback.type}`]">
      <span>{{ feedback.message }}</span>
      <button class="feedback-close" @click="clearFeedback">×</button>
    </div>

    <section class="stats-grid">
      <article v-for="item in statsCards" :key="item.key" class="stat-card">
        <p class="stat-label">{{ item.label }}</p>
        <strong class="stat-value">{{ item.value }}</strong>
        <span class="stat-note">{{ item.note }}</span>
      </article>
    </section>

    <section class="panel">
      <div class="section-header">
        <div>
          <p class="eyebrow">Nodes</p>
          <h3>NodeX Relay / Exit Topology</h3>
          <p class="section-subtitle">
            只用于 NodeX 有状态运行时的 relay/exit 拓扑、连通性检测和 gost API 相关操作，不承载 Ansible 机器管理。
          </p>
        </div>
        <div class="section-actions">
          <label class="filter-group">
            <span>节点类型</span>
            <select v-model="nodeTypeFilter">
              <option value="">全部</option>
              <option value="relay">Relay</option>
              <option value="exit">Exit</option>
            </select>
          </label>
          <label class="filter-group">
            <span>状态</span>
            <select v-model="nodeStatusFilter">
              <option value="all">全部</option>
              <option value="1">在线</option>
              <option value="0">离线</option>
            </select>
          </label>
        </div>
      </div>

      <div v-if="nodeLoading" class="loading-state">
        <div class="spinner"></div>
        <span>正在加载 NodeX 拓扑节点...</span>
      </div>

      <div v-else-if="!nodes.length" class="empty-state">
        <h4>暂无 NodeX 拓扑节点</h4>
        <p>请先创建 relay / exit 节点用于 NodeX 模式。若只做无状态执行，请改到 Ansible Machines 页面。</p>
      </div>

      <div v-else class="nodes-grid">
        <article v-for="node in nodes" :key="node.id" class="node-card">
          <div class="node-header">
            <div>
              <p class="eyebrow">Node #{{ node.id }}</p>
              <h4>{{ node.name }}</h4>
              <p class="node-meta">{{ node.typeLabel }} · {{ node.host }}:{{ node.port }}</p>
            </div>
            <div class="status-stack">
              <span :class="['tag', `tag-${node.type}`]">{{ node.typeLabel }}</span>
              <span :class="['tag', node.enabled ? 'tag-success' : 'tag-muted']">
                {{ node.enabled ? '已启用' : '已禁用' }}
              </span>
              <span :class="['tag', node.status === 1 ? 'tag-success' : 'tag-danger']">
                {{ node.status === 1 ? '在线' : '离线' }}
              </span>
            </div>
          </div>

          <div class="meta-grid">
            <div class="meta-item">
              <span class="meta-label">管理 API</span>
              <code>{{ node.host }}:{{ node.apiPort || '-' }}</code>
            </div>
            <div class="meta-item">
              <span class="meta-label">地区 / 运营商</span>
              <strong>{{ node.region || '-' }} / {{ node.isp || '-' }}</strong>
            </div>
            <div class="meta-item">
              <span class="meta-label">延迟</span>
              <strong>{{ node.latency ? `${node.latency} ms` : '-' }}</strong>
            </div>
            <div class="meta-item">
              <span class="meta-label">当前连接</span>
              <strong>{{ node.currentConn }}</strong>
            </div>
            <div class="meta-item">
              <span class="meta-label">上行 / 下行</span>
              <strong>{{ formatBytes(node.totalUpload) }} / {{ formatBytes(node.totalDownload) }}</strong>
            </div>
            <div class="meta-item">
              <span class="meta-label">权重 / 最大连接</span>
              <strong>{{ node.weight }} / {{ node.maxConn || '-' }}</strong>
            </div>
            <div class="meta-item">
              <span class="meta-label">最后检测</span>
              <strong>{{ formatTime(node.lastCheck) }}</strong>
            </div>
            <div class="meta-item">
              <span class="meta-label">在线率</span>
              <strong>{{ formatPercent(node.uptime) }}</strong>
            </div>
          </div>

          <div v-if="nodeResults[node.id]" class="node-result">
            <span :class="['tag', nodeResults[node.id].success ? 'tag-success' : 'tag-danger']">
              {{ nodeResults[node.id].success ? '操作成功' : '操作失败' }}
            </span>
            <p>{{ nodeResults[node.id].message }}</p>
          </div>

          <div class="node-actions">
            <button class="btn btn-secondary btn-sm" @click="openNodeEditor(node)">编辑</button>
            <button
              class="btn btn-secondary btn-sm"
              :disabled="isNodeActionPending(node.id, 'check')"
              @click="runNodeCheck(node)"
            >
              {{ isNodeActionPending(node.id, 'check') ? '检测中...' : '健康检测' }}
            </button>
            <button
              class="btn btn-secondary btn-sm"
              :disabled="isNodeActionPending(node.id, 'sync')"
              @click="syncNodeStatsAction(node)"
            >
              {{ isNodeActionPending(node.id, 'sync') ? '同步中...' : '同步统计' }}
            </button>
            <button class="btn btn-secondary btn-sm" @click="openConnectionModal(node)">测试连接</button>
            <button
              class="btn btn-secondary btn-sm"
              :disabled="isNodeActionPending(node.id, 'toggle')"
              @click="toggleNodeStatus(node)"
            >
              {{ node.enabled ? '禁用' : '启用' }}
            </button>
            <button class="btn btn-danger btn-sm" @click="openDeleteDialog('node', node)">删除</button>
          </div>
        </article>
      </div>

      <div class="pagination-bar">
        <button class="btn btn-ghost btn-sm" :disabled="nodePage === 1" @click="changeNodePage(nodePage - 1)">
          上一页
        </button>
        <span>第 {{ nodePage }} / {{ nodePageCount }} 页，共 {{ nodeTotal }} 条</span>
        <button
          class="btn btn-ghost btn-sm"
          :disabled="nodePage >= nodePageCount"
          @click="changeNodePage(nodePage + 1)"
        >
          下一页
        </button>
      </div>
    </section>

    <section class="panel">
      <div class="section-header">
        <div>
          <p class="eyebrow">Legacy Rules</p>
          <h3>Legacy Port Forward Rules</h3>
          <p class="section-subtitle">
            对应 `/admin/forward/rules*` 兼容接口。该区块仅用于保留旧规则能力，不代表 NodeX 或 Ansible 当前主执行路径。
          </p>
        </div>
        <div class="section-actions">
          <label class="filter-group filter-wide">
            <span>用户 ID</span>
            <input
              v-model.trim="ruleUserFilterInput"
              type="text"
              inputmode="numeric"
              placeholder="按用户 ID 过滤"
              @keyup.enter="applyRuleFilter"
            />
          </label>
          <button class="btn btn-secondary" @click="applyRuleFilter">查询</button>
          <button class="btn btn-secondary" @click="resetRuleFilter">清空</button>
          <button class="btn btn-primary" @click="openRuleEditor()">新增规则</button>
        </div>
      </div>

      <div v-if="ruleLoading" class="loading-state">
        <div class="spinner"></div>
        <span>正在加载 Legacy 规则...</span>
      </div>

      <div v-else-if="!rules.length" class="empty-state">
        <h4>暂无 Legacy 规则</h4>
        <p>如果需要兼容 relay + exit 的端口级转发，可以先在这里新增规则。</p>
      </div>

      <div v-else class="rules-table-wrap">
        <table class="rules-table">
          <thead>
            <tr>
              <th>ID</th>
              <th>名称</th>
              <th>入口</th>
              <th>出口</th>
              <th>归属</th>
              <th>限额</th>
              <th>流量</th>
              <th>状态</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="rule in rules" :key="rule.id">
              <td>#{{ rule.id }}</td>
              <td>
                <div class="rule-name-cell">
                  <strong>{{ rule.name }}</strong>
                  <span>{{ formatTime(rule.updatedAt) }}</span>
                </div>
              </td>
              <td>
                <div class="rule-endpoint">
                  <strong>{{ rule.relayName }}</strong>
                  <span>{{ rule.protocolLabel }} / :{{ rule.listenPort }}</span>
                </div>
              </td>
              <td>
                <div class="rule-endpoint">
                  <strong>{{ rule.exitName }}</strong>
                  <span>{{ rule.targetHost }}:{{ rule.targetPort }}</span>
                </div>
              </td>
              <td>{{ rule.ownerLabel }}</td>
              <td>
                <div class="rule-limit-cell">
                  <span>速率 {{ formatSpeedLimit(rule.speedLimit) }}</span>
                  <span>流量 {{ formatTrafficLimit(rule.trafficLimit) }}</span>
                  <span>过期 {{ formatExpireTime(rule.expireTime) }}</span>
                </div>
              </td>
              <td>
                <div class="rule-limit-cell">
                  <span>上 {{ formatBytes(rule.upload) }}</span>
                  <span>下 {{ formatBytes(rule.download) }}</span>
                  <span>连接 {{ rule.connections }}</span>
                </div>
              </td>
              <td>
                <div class="status-stack">
                  <span :class="['tag', rule.enabled ? 'tag-success' : 'tag-muted']">
                    {{ rule.enabled ? '启用' : '禁用' }}
                  </span>
                  <span class="tag tag-outline">{{ rule.protocolLabel }}</span>
                </div>
              </td>
              <td>
                <div class="table-actions">
                  <button class="btn btn-secondary btn-sm" @click="openRuleEditor(rule)">编辑</button>
                  <button
                    class="btn btn-secondary btn-sm"
                    :disabled="isRuleActionPending(rule.id, 'toggle')"
                    @click="toggleRuleStatus(rule)"
                  >
                    {{ rule.enabled ? '禁用' : '启用' }}
                  </button>
                  <button class="btn btn-danger btn-sm" @click="openDeleteDialog('rule', rule)">删除</button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <div class="pagination-bar">
        <button class="btn btn-ghost btn-sm" :disabled="rulePage === 1" @click="changeRulePage(rulePage - 1)">
          上一页
        </button>
        <span>第 {{ rulePage }} / {{ rulePageCount }} 页，共 {{ ruleTotal }} 条</span>
        <button
          class="btn btn-ghost btn-sm"
          :disabled="rulePage >= rulePageCount"
          @click="changeRulePage(rulePage + 1)"
        >
          下一页
        </button>
      </div>
    </section>

    <div v-if="nodeModalOpen" class="modal-overlay" @click.self="closeNodeModal">
      <div class="modal modal-lg">
        <div class="modal-header">
          <div>
            <p class="eyebrow">Node</p>
            <h3>{{ nodeEditMode ? '编辑中转节点' : '新增中转节点' }}</h3>
          </div>
          <button class="modal-close" @click="closeNodeModal">×</button>
        </div>
        <div class="modal-body">
          <div v-if="nodeModalLoading" class="modal-loading">正在加载节点详情...</div>
          <template v-else>
            <div class="form-grid">
              <label class="form-group">
                <span>节点名称</span>
                <input v-model.trim="nodeForm.name" type="text" placeholder="例如 relay-hk-01" />
                <small v-if="nodeFormErrors.name">{{ nodeFormErrors.name }}</small>
              </label>
              <label class="form-group">
                <span>节点类型</span>
                <select v-model="nodeForm.type">
                  <option value="relay">Relay</option>
                  <option value="exit">Exit</option>
                </select>
              </label>
            </div>
            <div class="form-grid">
              <label class="form-group">
                <span>主机地址</span>
                <input v-model.trim="nodeForm.host" type="text" placeholder="1.2.3.4" />
                <small v-if="nodeFormErrors.host">{{ nodeFormErrors.host }}</small>
              </label>
              <label class="form-group">
                <span>业务端口</span>
                <input v-model.trim="nodeForm.port" type="number" min="1" max="65535" />
                <small v-if="nodeFormErrors.port">{{ nodeFormErrors.port }}</small>
              </label>
            </div>
            <div class="form-grid">
              <label class="form-group">
                <span>API 端口</span>
                <input v-model.trim="nodeForm.apiPort" type="number" min="1" max="65535" />
                <small v-if="nodeFormErrors.apiPort">{{ nodeFormErrors.apiPort }}</small>
              </label>
              <label class="form-group">
                <span>API Token</span>
                <input v-model.trim="nodeForm.apiToken" type="text" placeholder="留空则后端自动生成" />
              </label>
            </div>
            <div class="form-grid">
              <label class="form-group">
                <span>地区</span>
                <input v-model.trim="nodeForm.region" type="text" placeholder="HK / JP / US" />
              </label>
              <label class="form-group">
                <span>运营商</span>
                <input v-model.trim="nodeForm.isp" type="text" placeholder="CMI / NTT / Cogent" />
              </label>
            </div>
            <div class="form-grid">
              <label class="form-group">
                <span>带宽 (Mbps)</span>
                <input v-model.trim="nodeForm.bandwidth" type="number" min="0" />
              </label>
              <label class="form-group">
                <span>最大连接</span>
                <input v-model.trim="nodeForm.maxConn" type="number" min="0" />
              </label>
            </div>
            <div class="form-grid">
              <label class="form-group">
                <span>权重</span>
                <input v-model.trim="nodeForm.weight" type="number" min="1" />
              </label>
            </div>
          </template>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary" :disabled="nodeModalSaving" @click="closeNodeModal">取消</button>
          <button class="btn btn-primary" :disabled="nodeModalSaving || nodeModalLoading" @click="submitNodeForm">
            {{ nodeModalSaving ? '保存中...' : nodeEditMode ? '保存修改' : '创建节点' }}
          </button>
        </div>
      </div>
    </div>

    <div v-if="ruleModalOpen" class="modal-overlay" @click.self="closeRuleModal">
      <div class="modal modal-xl">
        <div class="modal-header">
          <div>
            <p class="eyebrow">Rule</p>
            <h3>{{ ruleEditMode ? '编辑 Legacy 规则' : '新增 Legacy 规则' }}</h3>
          </div>
          <button class="modal-close" @click="closeRuleModal">×</button>
        </div>
        <div class="modal-body">
          <div v-if="ruleModalLoading" class="modal-loading">正在加载规则详情...</div>
          <template v-else>
            <div class="form-grid">
              <label class="form-group">
                <span>规则名称</span>
                <input v-model.trim="ruleForm.name" type="text" placeholder="例如 tcp-11111-hk" />
                <small v-if="ruleFormErrors.name">{{ ruleFormErrors.name }}</small>
              </label>
              <label class="form-group">
                <span>协议</span>
                <select v-model="ruleForm.protocol">
                  <option value="tcp">TCP</option>
                  <option value="udp">UDP</option>
                  <option value="both">TCP + UDP</option>
                </select>
              </label>
            </div>
            <div class="form-grid">
              <label class="form-group">
                <span>入口 Relay 节点</span>
                <select v-model.trim="ruleForm.relayNodeId">
                  <option value="">请选择 Relay 节点</option>
                  <option v-for="node in relayNodeOptions" :key="node.id" :value="String(node.id)">
                    {{ node.name }} ({{ node.host }})
                  </option>
                </select>
                <small v-if="ruleFormErrors.relayNodeId">{{ ruleFormErrors.relayNodeId }}</small>
              </label>
              <label class="form-group">
                <span>监听端口</span>
                <input v-model.trim="ruleForm.listenPort" type="number" min="1" max="65535" />
                <small v-if="ruleFormErrors.listenPort">{{ ruleFormErrors.listenPort }}</small>
              </label>
            </div>
            <div class="form-grid">
              <label class="form-group">
                <span>出口 Exit 节点</span>
                <select v-model.trim="ruleForm.exitNodeId">
                  <option value="">请选择 Exit 节点</option>
                  <option v-for="node in exitNodeOptions" :key="node.id" :value="String(node.id)">
                    {{ node.name }} ({{ node.host }})
                  </option>
                </select>
                <small v-if="ruleFormErrors.exitNodeId">{{ ruleFormErrors.exitNodeId }}</small>
              </label>
              <label class="form-group">
                <span>目标端口</span>
                <input v-model.trim="ruleForm.targetPort" type="number" min="1" max="65535" />
                <small v-if="ruleFormErrors.targetPort">{{ ruleFormErrors.targetPort }}</small>
              </label>
            </div>
            <div class="form-grid">
              <label class="form-group form-group-span">
                <span>目标主机</span>
                <input v-model.trim="ruleForm.targetHost" type="text" placeholder="127.0.0.1 或落地节点目标地址" />
                <small v-if="ruleFormErrors.targetHost">{{ ruleFormErrors.targetHost }}</small>
              </label>
            </div>
            <div class="form-grid">
              <label class="form-group">
                <span>用户 ID</span>
                <input
                  v-model.trim="ruleForm.userId"
                  type="text"
                  inputmode="numeric"
                  placeholder="留空表示公共规则"
                  :disabled="ruleEditMode"
                />
              </label>
              <label class="form-group">
                <span>用户组 ID</span>
                <input
                  v-model.trim="ruleForm.userGroupId"
                  type="text"
                  inputmode="numeric"
                  placeholder="与用户 ID 二选一"
                  :disabled="ruleEditMode"
                />
              </label>
            </div>
            <p v-if="ruleEditMode" class="helper-text">当前后端更新接口不支持修改用户归属字段，编辑时保持只读。</p>
            <small v-if="ruleFormErrors.owner" class="form-error">{{ ruleFormErrors.owner }}</small>
            <div class="form-grid">
              <label class="form-group">
                <span>速率上限 (KB/s)</span>
                <input v-model.trim="ruleForm.speedLimit" type="number" min="0" />
              </label>
              <label class="form-group">
                <span>流量上限 (Bytes)</span>
                <input v-model.trim="ruleForm.trafficLimit" type="number" min="0" />
              </label>
            </div>
            <div class="form-grid">
              <label class="form-group">
                <span>过期时间</span>
                <input v-model="ruleForm.expireTime" type="datetime-local" />
              </label>
              <label class="form-group form-group-span">
                <span>备注</span>
                <textarea v-model.trim="ruleForm.remark" rows="3" placeholder="可记录业务用途或维护说明"></textarea>
              </label>
            </div>
          </template>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary" :disabled="ruleModalSaving" @click="closeRuleModal">取消</button>
          <button class="btn btn-primary" :disabled="ruleModalSaving || ruleModalLoading" @click="submitRuleForm">
            {{ ruleModalSaving ? '保存中...' : ruleEditMode ? '保存修改' : '创建规则' }}
          </button>
        </div>
      </div>
    </div>

    <div v-if="connectionModalOpen" class="modal-overlay" @click.self="closeConnectionModal">
      <div class="modal modal-md">
        <div class="modal-header">
          <div>
            <p class="eyebrow">Gost API</p>
            <h3>测试节点连接</h3>
          </div>
          <button class="modal-close" @click="closeConnectionModal">×</button>
        </div>
        <div class="modal-body">
          <div class="form-grid">
            <label class="form-group">
              <span>主机地址</span>
              <input v-model.trim="connectionForm.host" type="text" placeholder="127.0.0.1" />
              <small v-if="connectionErrors.host">{{ connectionErrors.host }}</small>
            </label>
            <label class="form-group">
              <span>API 端口</span>
              <input v-model.trim="connectionForm.apiPort" type="number" min="1" max="65535" />
              <small v-if="connectionErrors.apiPort">{{ connectionErrors.apiPort }}</small>
            </label>
          </div>
          <div class="form-grid">
            <label class="form-group form-group-span">
              <span>API Token</span>
              <input v-model.trim="connectionForm.apiToken" type="text" placeholder="如未启用鉴权可留空" />
            </label>
          </div>
          <div v-if="connectionResult" :class="['connection-result', connectionResult.success ? 'connection-ok' : 'connection-fail']">
            <strong>{{ connectionResult.success ? '连接成功' : '连接失败' }}</strong>
            <p>{{ connectionResult.message }}</p>
            <p v-if="connectionResult.serviceCount !== null">服务数量: {{ connectionResult.serviceCount }}</p>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary" :disabled="connectionLoading" @click="closeConnectionModal">取消</button>
          <button class="btn btn-primary" :disabled="connectionLoading" @click="submitConnectionTest">
            {{ connectionLoading ? '检测中...' : '开始检测' }}
          </button>
        </div>
      </div>
    </div>

    <div v-if="deleteModalOpen" class="modal-overlay" @click.self="closeDeleteDialog">
      <div class="modal modal-sm">
        <div class="modal-header">
          <h3>确认删除</h3>
          <button class="modal-close" @click="closeDeleteDialog">×</button>
        </div>
        <div class="modal-body">
          <p>
            确认删除{{ deleteState.kind === 'node' ? '节点' : '规则' }}
            <strong>{{ deleteState.name || `#${deleteState.id || '-'}` }}</strong>？
          </p>
          <p class="helper-text">删除后无法自动恢复，请确认没有仍在使用的转发关系。</p>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary btn-sm" :disabled="deleteLoading" @click="closeDeleteDialog">取消</button>
          <button class="btn btn-danger btn-sm" :disabled="deleteLoading" @click="confirmDelete">
            {{ deleteLoading ? '删除中...' : '确认删除' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import ForwardSuiteNav from '@/components/admin/ForwardSuiteNav.vue'
import {
  checkForwardNode,
  createForwardNode,
  createForwardRule,
  deleteForwardNode,
  deleteForwardRule,
  getForwardNode,
  getForwardNodes,
  getForwardRule,
  getForwardRules,
  getForwardStats,
  syncForwardNodeStats,
  testForwardConnection,
  toggleForwardNode,
  toggleForwardRule,
  updateForwardNode,
  updateForwardRule
} from '@/api/admin'

const typeLabels = {
  relay: 'Relay',
  exit: 'Exit'
}

const protocolLabels = {
  tcp: 'TCP',
  udp: 'UDP',
  both: 'TCP + UDP'
}

const feedback = reactive({
  type: 'info',
  message: ''
})

const stats = reactive({
  relay_nodes: 0,
  exit_nodes: 0,
  online_relay: 0,
  online_exit: 0,
  total_upload: 0,
  total_download: 0
})
const statsLoading = ref(false)

const nodes = ref([])
const nodeOptions = ref([])
const nodeLoading = ref(false)
const nodeOptionsLoading = ref(false)
const nodePage = ref(1)
const nodePageSize = ref(12)
const nodeTotal = ref(0)
const nodeTypeFilter = ref('')
const nodeStatusFilter = ref('all')
const pendingNodeAction = ref('')
const nodeResults = reactive({})

const rules = ref([])
const ruleLoading = ref(false)
const rulePage = ref(1)
const rulePageSize = ref(10)
const ruleTotal = ref(0)
const ruleUserFilter = ref('')
const ruleUserFilterInput = ref('')
const pendingRuleAction = ref('')

const nodeModalOpen = ref(false)
const nodeModalLoading = ref(false)
const nodeModalSaving = ref(false)
const nodeEditMode = ref(false)
const nodeForm = reactive({
  id: null,
  name: '',
  type: 'relay',
  host: '',
  port: '',
  apiPort: '',
  apiToken: '',
  region: '',
  isp: '',
  bandwidth: '',
  weight: '1',
  maxConn: ''
})
const nodeFormErrors = reactive({
  name: '',
  host: '',
  port: '',
  apiPort: ''
})

const ruleModalOpen = ref(false)
const ruleModalLoading = ref(false)
const ruleModalSaving = ref(false)
const ruleEditMode = ref(false)
const ruleForm = reactive({
  id: null,
  name: '',
  relayNodeId: '',
  exitNodeId: '',
  listenPort: '',
  protocol: 'tcp',
  targetHost: '',
  targetPort: '',
  userId: '',
  userGroupId: '',
  speedLimit: '',
  trafficLimit: '',
  expireTime: '',
  remark: ''
})
const ruleFormErrors = reactive({
  name: '',
  relayNodeId: '',
  exitNodeId: '',
  listenPort: '',
  targetHost: '',
  targetPort: '',
  owner: ''
})

const connectionModalOpen = ref(false)
const connectionLoading = ref(false)
const connectionResult = ref(null)
const connectionForm = reactive({
  host: '',
  apiPort: '',
  apiToken: ''
})
const connectionErrors = reactive({
  host: '',
  apiPort: ''
})

const deleteModalOpen = ref(false)
const deleteLoading = ref(false)
const deleteState = reactive({
  kind: 'node',
  id: null,
  name: ''
})

const pageBusy = computed(() => {
  return statsLoading.value || nodeLoading.value || ruleLoading.value || nodeOptionsLoading.value
})

const nodePageCount = computed(() => Math.max(1, Math.ceil(nodeTotal.value / nodePageSize.value)))
const rulePageCount = computed(() => Math.max(1, Math.ceil(ruleTotal.value / rulePageSize.value)))

const relayNodeOptions = computed(() => nodeOptions.value.filter(node => node.type === 'relay'))
const exitNodeOptions = computed(() => nodeOptions.value.filter(node => node.type === 'exit'))

const statsCards = computed(() => {
  const totalNodes = Number(stats.relay_nodes || 0) + Number(stats.exit_nodes || 0)
  const onlineNodes = Number(stats.online_relay || 0) + Number(stats.online_exit || 0)

  return [
    {
      key: 'relay',
      label: 'Relay 节点',
      value: String(stats.relay_nodes || 0),
      note: `在线 ${stats.online_relay || 0} 台`
    },
    {
      key: 'exit',
      label: 'Exit 节点',
      value: String(stats.exit_nodes || 0),
      note: `在线 ${stats.online_exit || 0} 台`
    },
    {
      key: 'total',
      label: '节点总数',
      value: String(totalNodes),
      note: '包含 Relay / Exit'
    },
    {
      key: 'online',
      label: '在线节点',
      value: String(onlineNodes),
      note: statsLoading.value ? '统计刷新中' : '基于最近一次健康检测'
    },
    {
      key: 'upload',
      label: '累计上行',
      value: formatBytes(stats.total_upload),
      note: '所有中转节点汇总'
    },
    {
      key: 'download',
      label: '累计下行',
      value: formatBytes(stats.total_download),
      note: '所有中转节点汇总'
    }
  ]
})

function clearFeedback() {
  feedback.message = ''
}

function setFeedback(type, message) {
  feedback.type = type
  feedback.message = message
}

function unwrapResponse(response) {
  if (!response) {
    return {}
  }

  if (typeof response.code === 'number') {
    if (response.code !== 0) {
      throw new Error(response.msg || '请求失败')
    }
    return response.data ?? response
  }

  if (response.data && typeof response.data === 'object' && !Array.isArray(response.data)) {
    return response.data
  }

  return response
}

function extractErrorMessage(error, fallback = '请求失败') {
  return (
    error?.response?.data?.error ||
    error?.response?.data?.message ||
    error?.response?.data?.msg ||
    error?.message ||
    fallback
  )
}

function parsePositiveInt(value) {
  const text = String(value ?? '').trim()
  if (!text) {
    return null
  }
  const parsed = Number(text)
  if (!Number.isInteger(parsed) || parsed <= 0) {
    return null
  }
  return parsed
}

function parseNonNegativeInt(value) {
  const text = String(value ?? '').trim()
  if (!text) {
    return null
  }
  const parsed = Number(text)
  if (!Number.isFinite(parsed) || parsed < 0) {
    return null
  }
  return Math.trunc(parsed)
}

function formatTime(value) {
  if (!value) {
    return '-'
  }
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return '-'
  }
  return date.toLocaleString('zh-CN', { hour12: false })
}

function formatDateTimeLocal(value) {
  if (!value) {
    return ''
  }
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return ''
  }

  const pad = input => String(input).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`
}

function toISOStringOrNull(value) {
  if (!value) {
    return null
  }
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return null
  }
  return date.toISOString()
}

function formatBytes(value) {
  const size = Number(value || 0)
  if (!Number.isFinite(size) || size <= 0) {
    return '0 B'
  }

  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let next = size
  let index = 0

  while (next >= 1024 && index < units.length - 1) {
    next /= 1024
    index += 1
  }

  return `${next.toFixed(next >= 10 || index === 0 ? 0 : 1)} ${units[index]}`
}

function formatPercent(value) {
  const percent = Number(value)
  if (!Number.isFinite(percent)) {
    return '-'
  }
  return `${percent.toFixed(1)}%`
}

function formatSpeedLimit(value) {
  if (value === null || value === undefined || value === '') {
    return '不限'
  }
  return `${value} KB/s`
}

function formatTrafficLimit(value) {
  if (value === null || value === undefined || value === '') {
    return '不限'
  }
  return formatBytes(value)
}

function formatExpireTime(value) {
  if (!value) {
    return '永不过期'
  }
  return formatTime(value)
}

function normalizeNode(item = {}) {
  const type = item.type || 'relay'
  return {
    id: Number(item.id ?? 0) || 0,
    name: item.name || `node-${item.id ?? 'new'}`,
    type,
    typeLabel: typeLabels[type] || 'Relay',
    host: item.host || '-',
    port: Number(item.port ?? 0) || 0,
    apiPort: Number(item.api_port ?? item.apiPort ?? 0) || '',
    apiToken: item.api_token ?? item.apiToken ?? '',
    region: item.region || '',
    isp: item.isp || '',
    bandwidth: Number(item.bandwidth ?? 0) || 0,
    weight: Number(item.weight ?? 1) || 1,
    maxConn: Number(item.max_conn ?? item.maxConn ?? 0) || 0,
    enabled: typeof item.enabled === 'boolean' ? item.enabled : Number(item.enabled ?? 0) === 1,
    status: Number(item.status ?? 0) || 0,
    lastCheck: item.last_check ?? item.lastCheck ?? null,
    latency: Number(item.latency ?? 0) || 0,
    currentConn: Number(item.current_conn ?? item.currentConn ?? 0) || 0,
    totalUpload: Number(item.total_upload ?? item.totalUpload ?? 0) || 0,
    totalDownload: Number(item.total_download ?? item.totalDownload ?? 0) || 0,
    uptime: Number(item.uptime ?? 0) || 0
  }
}

function normalizeRule(item = {}) {
  const protocol = String(item.protocol || 'tcp').toLowerCase()
  const relayNodeId = Number(item.relay_node_id ?? item.relayNodeID ?? item.relayNodeId ?? 0) || 0
  const exitNodeId = Number(item.exit_node_id ?? item.exitNodeID ?? item.exitNodeId ?? 0) || 0
  const userId = Number(item.user_id ?? item.userId ?? 0) || null
  const userGroupId = Number(item.user_group_id ?? item.userGroupId ?? 0) || null

  return {
    id: Number(item.id ?? 0) || 0,
    name: item.name || `rule-${item.id ?? 'new'}`,
    relayNodeId,
    exitNodeId,
    relayName: item.relay_node?.name || item.relayNode?.name || (relayNodeId ? `#${relayNodeId}` : '-'),
    exitName: item.exit_node?.name || item.exitNode?.name || (exitNodeId ? `#${exitNodeId}` : '-'),
    listenPort: Number(item.listen_port ?? item.listenPort ?? 0) || 0,
    protocol,
    protocolLabel: protocolLabels[protocol] || protocol.toUpperCase(),
    targetHost: item.target_host ?? item.targetHost ?? '',
    targetPort: Number(item.target_port ?? item.targetPort ?? 0) || 0,
    enabled: typeof item.enabled === 'boolean' ? item.enabled : Number(item.enabled ?? 0) === 1,
    userId,
    userGroupId,
    ownerLabel: userId ? `用户 #${userId}` : userGroupId ? `用户组 #${userGroupId}` : '公共规则',
    speedLimit: item.speed_limit ?? item.speedLimit ?? null,
    trafficLimit: item.traffic_limit ?? item.trafficLimit ?? null,
    expireTime: item.expire_time ?? item.expireTime ?? null,
    upload: Number(item.upload ?? 0) || 0,
    download: Number(item.download ?? 0) || 0,
    connections: Number(item.connections ?? 0) || 0,
    updatedAt: item.updated_at ?? item.updatedAt ?? item.created_at ?? item.createdAt ?? null,
    remark: item.remark || ''
  }
}

function resetNodeForm() {
  nodeForm.id = null
  nodeForm.name = ''
  nodeForm.type = 'relay'
  nodeForm.host = ''
  nodeForm.port = ''
  nodeForm.apiPort = ''
  nodeForm.apiToken = ''
  nodeForm.region = ''
  nodeForm.isp = ''
  nodeForm.bandwidth = ''
  nodeForm.weight = '1'
  nodeForm.maxConn = ''

  nodeFormErrors.name = ''
  nodeFormErrors.host = ''
  nodeFormErrors.port = ''
  nodeFormErrors.apiPort = ''
}

function fillNodeForm(node) {
  nodeForm.id = node.id
  nodeForm.name = node.name
  nodeForm.type = node.type
  nodeForm.host = node.host
  nodeForm.port = node.port ? String(node.port) : ''
  nodeForm.apiPort = node.apiPort ? String(node.apiPort) : ''
  nodeForm.apiToken = node.apiToken || ''
  nodeForm.region = node.region || ''
  nodeForm.isp = node.isp || ''
  nodeForm.bandwidth = node.bandwidth ? String(node.bandwidth) : ''
  nodeForm.weight = node.weight ? String(node.weight) : '1'
  nodeForm.maxConn = node.maxConn ? String(node.maxConn) : ''
}

function validateNodeForm() {
  nodeFormErrors.name = ''
  nodeFormErrors.host = ''
  nodeFormErrors.port = ''
  nodeFormErrors.apiPort = ''

  if (!nodeForm.name.trim()) {
    nodeFormErrors.name = '节点名称不能为空'
  }
  if (!nodeForm.host.trim()) {
    nodeFormErrors.host = '主机地址不能为空'
  }
  if (!parsePositiveInt(nodeForm.port) || parsePositiveInt(nodeForm.port) > 65535) {
    nodeFormErrors.port = '业务端口必须在 1 到 65535 之间'
  }
  if (String(nodeForm.apiPort).trim() && (!parsePositiveInt(nodeForm.apiPort) || parsePositiveInt(nodeForm.apiPort) > 65535)) {
    nodeFormErrors.apiPort = 'API 端口必须在 1 到 65535 之间'
  }

  return !nodeFormErrors.name && !nodeFormErrors.host && !nodeFormErrors.port && !nodeFormErrors.apiPort
}

function resetRuleForm() {
  ruleForm.id = null
  ruleForm.name = ''
  ruleForm.relayNodeId = ''
  ruleForm.exitNodeId = ''
  ruleForm.listenPort = ''
  ruleForm.protocol = 'tcp'
  ruleForm.targetHost = ''
  ruleForm.targetPort = ''
  ruleForm.userId = ''
  ruleForm.userGroupId = ''
  ruleForm.speedLimit = ''
  ruleForm.trafficLimit = ''
  ruleForm.expireTime = ''
  ruleForm.remark = ''

  ruleFormErrors.name = ''
  ruleFormErrors.relayNodeId = ''
  ruleFormErrors.exitNodeId = ''
  ruleFormErrors.listenPort = ''
  ruleFormErrors.targetHost = ''
  ruleFormErrors.targetPort = ''
  ruleFormErrors.owner = ''
}

function fillRuleForm(rule) {
  ruleForm.id = rule.id
  ruleForm.name = rule.name
  ruleForm.relayNodeId = rule.relayNodeId ? String(rule.relayNodeId) : ''
  ruleForm.exitNodeId = rule.exitNodeId ? String(rule.exitNodeId) : ''
  ruleForm.listenPort = rule.listenPort ? String(rule.listenPort) : ''
  ruleForm.protocol = rule.protocol
  ruleForm.targetHost = rule.targetHost
  ruleForm.targetPort = rule.targetPort ? String(rule.targetPort) : ''
  ruleForm.userId = rule.userId ? String(rule.userId) : ''
  ruleForm.userGroupId = rule.userGroupId ? String(rule.userGroupId) : ''
  ruleForm.speedLimit = rule.speedLimit !== null && rule.speedLimit !== undefined ? String(rule.speedLimit) : ''
  ruleForm.trafficLimit = rule.trafficLimit !== null && rule.trafficLimit !== undefined ? String(rule.trafficLimit) : ''
  ruleForm.expireTime = formatDateTimeLocal(rule.expireTime)
  ruleForm.remark = rule.remark || ''
}

function validateRuleForm() {
  ruleFormErrors.name = ''
  ruleFormErrors.relayNodeId = ''
  ruleFormErrors.exitNodeId = ''
  ruleFormErrors.listenPort = ''
  ruleFormErrors.targetHost = ''
  ruleFormErrors.targetPort = ''
  ruleFormErrors.owner = ''

  if (!ruleForm.name.trim()) {
    ruleFormErrors.name = '规则名称不能为空'
  }
  if (!parsePositiveInt(ruleForm.relayNodeId)) {
    ruleFormErrors.relayNodeId = '请选择入口 Relay 节点'
  }
  if (!parsePositiveInt(ruleForm.exitNodeId)) {
    ruleFormErrors.exitNodeId = '请选择出口 Exit 节点'
  }
  if (!parsePositiveInt(ruleForm.listenPort) || parsePositiveInt(ruleForm.listenPort) > 65535) {
    ruleFormErrors.listenPort = '监听端口必须在 1 到 65535 之间'
  }
  if (!ruleForm.targetHost.trim()) {
    ruleFormErrors.targetHost = '目标主机不能为空'
  }
  if (!parsePositiveInt(ruleForm.targetPort) || parsePositiveInt(ruleForm.targetPort) > 65535) {
    ruleFormErrors.targetPort = '目标端口必须在 1 到 65535 之间'
  }
  if (ruleForm.userId.trim() && ruleForm.userGroupId.trim()) {
    ruleFormErrors.owner = '用户 ID 与用户组 ID 只能填写一个'
  }

  return (
    !ruleFormErrors.name &&
    !ruleFormErrors.relayNodeId &&
    !ruleFormErrors.exitNodeId &&
    !ruleFormErrors.listenPort &&
    !ruleFormErrors.targetHost &&
    !ruleFormErrors.targetPort &&
    !ruleFormErrors.owner
  )
}

function resetConnectionForm() {
  connectionForm.host = ''
  connectionForm.apiPort = ''
  connectionForm.apiToken = ''
  connectionErrors.host = ''
  connectionErrors.apiPort = ''
  connectionResult.value = null
}

function validateConnectionForm() {
  connectionErrors.host = ''
  connectionErrors.apiPort = ''

  if (!connectionForm.host.trim()) {
    connectionErrors.host = '主机地址不能为空'
  }
  if (!parsePositiveInt(connectionForm.apiPort) || parsePositiveInt(connectionForm.apiPort) > 65535) {
    connectionErrors.apiPort = 'API 端口必须在 1 到 65535 之间'
  }

  return !connectionErrors.host && !connectionErrors.apiPort
}

function isNodeActionPending(id, action) {
  return pendingNodeAction.value === `${id}:${action}`
}

function isRuleActionPending(id, action) {
  return pendingRuleAction.value === `${id}:${action}`
}

function setNodeResult(nodeId, success, message) {
  nodeResults[nodeId] = {
    success,
    message
  }
}

async function loadStats() {
  statsLoading.value = true
  try {
    const payload = unwrapResponse(await getForwardStats())
    stats.relay_nodes = Number(payload.relay_nodes ?? 0)
    stats.exit_nodes = Number(payload.exit_nodes ?? 0)
    stats.online_relay = Number(payload.online_relay ?? 0)
    stats.online_exit = Number(payload.online_exit ?? 0)
    stats.total_upload = Number(payload.total_upload ?? 0)
    stats.total_download = Number(payload.total_download ?? 0)
  } catch (error) {
    stats.relay_nodes = 0
    stats.exit_nodes = 0
    stats.online_relay = 0
    stats.online_exit = 0
    stats.total_upload = 0
    stats.total_download = 0
    setFeedback('error', extractErrorMessage(error, '加载转发统计失败'))
  } finally {
    statsLoading.value = false
  }
}

async function loadNodes() {
  nodeLoading.value = true
  try {
    const params = {
      page: nodePage.value,
      page_size: nodePageSize.value
    }
    if (nodeTypeFilter.value) {
      params.type = nodeTypeFilter.value
    }
    if (nodeStatusFilter.value !== 'all') {
      params.status = Number(nodeStatusFilter.value)
    }

    const payload = unwrapResponse(await getForwardNodes(params))
    const list = Array.isArray(payload?.list) ? payload.list : Array.isArray(payload) ? payload : []
    nodes.value = list.map(normalizeNode)
    nodeTotal.value = Number(payload?.total ?? list.length)
  } catch (error) {
    nodes.value = []
    nodeTotal.value = 0
    setFeedback('error', extractErrorMessage(error, '加载中转节点失败'))
  } finally {
    nodeLoading.value = false
  }
}

async function loadNodeOptions() {
  nodeOptionsLoading.value = true
  try {
    const payload = unwrapResponse(await getForwardNodes({ page: 1, page_size: 500 }))
    const list = Array.isArray(payload?.list) ? payload.list : Array.isArray(payload) ? payload : []
    nodeOptions.value = list
      .map(normalizeNode)
      .sort((left, right) => {
        if (left.type === right.type) {
          return left.name.localeCompare(right.name)
        }
        return left.type.localeCompare(right.type)
      })
  } catch (error) {
    nodeOptions.value = []
    setFeedback('error', extractErrorMessage(error, '加载节点选项失败'))
  } finally {
    nodeOptionsLoading.value = false
  }
}

async function loadRules() {
  ruleLoading.value = true
  try {
    const params = {
      page: rulePage.value,
      page_size: rulePageSize.value
    }
    const userID = parsePositiveInt(ruleUserFilter.value)
    if (userID) {
      params.user_id = userID
    }

    const payload = unwrapResponse(await getForwardRules(params))
    const list = Array.isArray(payload?.list) ? payload.list : Array.isArray(payload) ? payload : []
    rules.value = list.map(normalizeRule)
    ruleTotal.value = Number(payload?.total ?? list.length)
  } catch (error) {
    rules.value = []
    ruleTotal.value = 0
    setFeedback('error', extractErrorMessage(error, '加载转发规则失败'))
  } finally {
    ruleLoading.value = false
  }
}

async function refreshAll() {
  await Promise.all([
    loadStats(),
    loadNodes(),
    loadRules(),
    loadNodeOptions()
  ])
}

function changeNodePage(nextPage) {
  if (nextPage < 1 || nextPage > nodePageCount.value) {
    return
  }
  nodePage.value = nextPage
  loadNodes()
}

function changeRulePage(nextPage) {
  if (nextPage < 1 || nextPage > rulePageCount.value) {
    return
  }
  rulePage.value = nextPage
  loadRules()
}

function applyRuleFilter() {
  if (ruleUserFilterInput.value.trim() && !parsePositiveInt(ruleUserFilterInput.value)) {
    setFeedback('error', '用户 ID 必须为正整数')
    return
  }
  ruleUserFilter.value = ruleUserFilterInput.value.trim()
  rulePage.value = 1
  loadRules()
}

function resetRuleFilter() {
  ruleUserFilter.value = ''
  ruleUserFilterInput.value = ''
  rulePage.value = 1
  loadRules()
}

async function openNodeEditor(node = null) {
  resetNodeForm()
  nodeEditMode.value = Boolean(node?.id)
  nodeModalOpen.value = true

  if (!node?.id) {
    return
  }

  nodeModalLoading.value = true
  try {
    const detail = normalizeNode(unwrapResponse(await getForwardNode(node.id)))
    fillNodeForm(detail)
  } catch (error) {
    setFeedback('error', extractErrorMessage(error, '加载节点详情失败'))
    closeNodeModal(true)
  } finally {
    nodeModalLoading.value = false
  }
}

function closeNodeModal(force = false) {
  if (!force && nodeModalSaving.value) {
    return
  }
  nodeModalOpen.value = false
  nodeModalLoading.value = false
  nodeEditMode.value = false
  resetNodeForm()
}

async function submitNodeForm() {
  if (!validateNodeForm()) {
    return
  }

  nodeModalSaving.value = true
  try {
    const payload = {
      name: nodeForm.name.trim(),
      type: nodeForm.type,
      host: nodeForm.host.trim(),
      port: parsePositiveInt(nodeForm.port),
      weight: parsePositiveInt(nodeForm.weight) || 1
    }

    const apiPort = parsePositiveInt(nodeForm.apiPort)
    const bandwidth = parseNonNegativeInt(nodeForm.bandwidth)
    const maxConn = parseNonNegativeInt(nodeForm.maxConn)

    if (apiPort) {
      payload.api_port = apiPort
    }
    if (nodeForm.apiToken.trim()) {
      payload.api_token = nodeForm.apiToken.trim()
    }
    if (nodeForm.region.trim()) {
      payload.region = nodeForm.region.trim()
    }
    if (nodeForm.isp.trim()) {
      payload.isp = nodeForm.isp.trim()
    }
    if (bandwidth !== null) {
      payload.bandwidth = bandwidth
    }
    if (maxConn !== null) {
      payload.max_conn = maxConn
    }

    if (nodeEditMode.value && nodeForm.id) {
      await updateForwardNode(nodeForm.id, payload)
      setFeedback('success', '中转节点已更新')
    } else {
      await createForwardNode(payload)
      setFeedback('success', '中转节点已创建')
    }

    closeNodeModal(true)
    await Promise.all([loadNodes(), loadNodeOptions(), loadStats()])
  } catch (error) {
    setFeedback('error', extractErrorMessage(error, '保存中转节点失败'))
  } finally {
    nodeModalSaving.value = false
  }
}

async function runNodeCheck(node) {
  pendingNodeAction.value = `${node.id}:check`
  try {
    const payload = unwrapResponse(await checkForwardNode(node.id))
    const success = Number(payload.status ?? 0) === 1 && !payload.error
    const message = success
      ? payload.latency ? `延迟 ${payload.latency} ms` : '节点可达'
      : payload.error || '节点不可达'

    setNodeResult(node.id, success, message)
    setFeedback(success ? 'success' : 'error', `${node.name}: ${message}`)
    await Promise.all([loadNodes(), loadNodeOptions(), loadStats()])
  } catch (error) {
    const message = extractErrorMessage(error, '健康检测失败')
    setNodeResult(node.id, false, message)
    setFeedback('error', `${node.name}: ${message}`)
  } finally {
    pendingNodeAction.value = ''
  }
}

async function syncNodeStatsAction(node) {
  pendingNodeAction.value = `${node.id}:sync`
  try {
    const payload = unwrapResponse(await syncForwardNodeStats(node.id))
    const statsPayload = payload?.stats || null
    const message = payload?.message || '统计同步成功'
    const suffix = statsPayload?.current_conn !== undefined ? `，当前连接 ${statsPayload.current_conn}` : ''

    setNodeResult(node.id, true, `${message}${suffix}`)
    setFeedback('success', `${node.name}: ${message}`)
    await Promise.all([loadNodes(), loadStats()])
  } catch (error) {
    const message = extractErrorMessage(error, '同步统计失败')
    setNodeResult(node.id, false, message)
    setFeedback('error', `${node.name}: ${message}`)
  } finally {
    pendingNodeAction.value = ''
  }
}

async function toggleNodeStatus(node) {
  pendingNodeAction.value = `${node.id}:toggle`
  try {
    await toggleForwardNode(node.id, !node.enabled)
    setFeedback('success', `${node.name} 已${node.enabled ? '禁用' : '启用'}`)
    await Promise.all([loadNodes(), loadNodeOptions(), loadStats()])
  } catch (error) {
    setFeedback('error', extractErrorMessage(error, '切换节点状态失败'))
  } finally {
    pendingNodeAction.value = ''
  }
}

async function openRuleEditor(rule = null) {
  resetRuleForm()
  ruleEditMode.value = Boolean(rule?.id)
  ruleModalOpen.value = true
  ruleModalLoading.value = true

  try {
    await loadNodeOptions()
    if (rule?.id) {
      const detail = normalizeRule(unwrapResponse(await getForwardRule(rule.id)))
      fillRuleForm(detail)
    }
  } catch (error) {
    setFeedback('error', extractErrorMessage(error, '加载规则详情失败'))
    closeRuleModal(true)
  } finally {
    ruleModalLoading.value = false
  }
}

function closeRuleModal(force = false) {
  if (!force && ruleModalSaving.value) {
    return
  }
  ruleModalOpen.value = false
  ruleModalLoading.value = false
  ruleEditMode.value = false
  resetRuleForm()
}

async function submitRuleForm() {
  if (!validateRuleForm()) {
    return
  }

  ruleModalSaving.value = true
  try {
    const payload = {
      name: ruleForm.name.trim(),
      relay_node_id: parsePositiveInt(ruleForm.relayNodeId),
      exit_node_id: parsePositiveInt(ruleForm.exitNodeId),
      listen_port: parsePositiveInt(ruleForm.listenPort),
      protocol: ruleForm.protocol,
      target_host: ruleForm.targetHost.trim(),
      target_port: parsePositiveInt(ruleForm.targetPort),
      remark: ruleForm.remark.trim()
    }

    const speedLimit = parseNonNegativeInt(ruleForm.speedLimit)
    const trafficLimit = parseNonNegativeInt(ruleForm.trafficLimit)
    const expireTime = toISOStringOrNull(ruleForm.expireTime)

    if (speedLimit !== null) {
      payload.speed_limit = speedLimit
    }
    if (trafficLimit !== null) {
      payload.traffic_limit = trafficLimit
    }
    if (expireTime) {
      payload.expire_time = expireTime
    }

    if (ruleEditMode.value && ruleForm.id) {
      await updateForwardRule(ruleForm.id, payload)
      setFeedback('success', '转发规则已更新')
    } else {
      const userId = parsePositiveInt(ruleForm.userId)
      const userGroupId = parsePositiveInt(ruleForm.userGroupId)

      if (userId) {
        payload.user_id = userId
      }
      if (userGroupId) {
        payload.user_group_id = userGroupId
      }

      await createForwardRule(payload)
      setFeedback('success', '转发规则已创建')
    }

    closeRuleModal(true)
    await Promise.all([loadRules(), loadNodes(), loadStats()])
  } catch (error) {
    setFeedback('error', extractErrorMessage(error, '保存转发规则失败'))
  } finally {
    ruleModalSaving.value = false
  }
}

async function toggleRuleStatus(rule) {
  pendingRuleAction.value = `${rule.id}:toggle`
  try {
    await toggleForwardRule(rule.id, !rule.enabled)
    setFeedback('success', `${rule.name} 已${rule.enabled ? '禁用' : '启用'}`)
    await loadRules()
  } catch (error) {
    setFeedback('error', extractErrorMessage(error, '切换规则状态失败'))
  } finally {
    pendingRuleAction.value = ''
  }
}

function openConnectionModal(node = null) {
  resetConnectionForm()
  if (node) {
    connectionForm.host = node.host || ''
    connectionForm.apiPort = node.apiPort ? String(node.apiPort) : ''
    connectionForm.apiToken = node.apiToken || ''
  }
  connectionModalOpen.value = true
}

function closeConnectionModal() {
  if (connectionLoading.value) {
    return
  }
  connectionModalOpen.value = false
  resetConnectionForm()
}

async function submitConnectionTest() {
  if (!validateConnectionForm()) {
    return
  }

  connectionLoading.value = true
  try {
    const payload = unwrapResponse(await testForwardConnection({
      host: connectionForm.host.trim(),
      api_port: parsePositiveInt(connectionForm.apiPort),
      api_token: connectionForm.apiToken.trim() || undefined
    }))

    connectionResult.value = {
      success: payload.success !== false,
      message: payload.message || (payload.success === false ? '连接失败' : '连接成功'),
      serviceCount: payload.service_count ?? null
    }

    setFeedback(connectionResult.value.success ? 'success' : 'error', connectionResult.value.message)
  } catch (error) {
    connectionResult.value = {
      success: false,
      message: extractErrorMessage(error, '连接检测失败'),
      serviceCount: null
    }
    setFeedback('error', connectionResult.value.message)
  } finally {
    connectionLoading.value = false
  }
}

function openDeleteDialog(kind, item) {
  deleteState.kind = kind
  deleteState.id = item?.id ?? null
  deleteState.name = item?.name || ''
  deleteModalOpen.value = true
}

function closeDeleteDialog(force = false) {
  if (!force && deleteLoading.value) {
    return
  }
  deleteModalOpen.value = false
  deleteState.kind = 'node'
  deleteState.id = null
  deleteState.name = ''
}

async function confirmDelete() {
  if (!deleteState.id) {
    return
  }

  deleteLoading.value = true
  try {
    if (deleteState.kind === 'node') {
      await deleteForwardNode(deleteState.id)
      setFeedback('success', '中转节点已删除')
      await Promise.all([loadNodes(), loadNodeOptions(), loadRules(), loadStats()])
    } else {
      await deleteForwardRule(deleteState.id)
      setFeedback('success', '转发规则已删除')
      await loadRules()
    }
    closeDeleteDialog(true)
  } catch (error) {
    setFeedback('error', extractErrorMessage(error, '删除失败'))
  } finally {
    deleteLoading.value = false
  }
}

watch([nodeTypeFilter, nodeStatusFilter], () => {
  nodePage.value = 1
  loadNodes()
})

onMounted(() => {
  refreshAll()
})
</script>

<style scoped>
.forward-nodes-page {
  display: flex;
  flex-direction: column;
  gap: 18px;
  min-height: calc(100vh - 200px);
}

.toolbar,
.panel,
.stat-card {
  border: 1px solid var(--border-color);
  background: var(--surface-color);
  box-shadow: var(--shadow-sm);
}

.toolbar {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 16px;
  padding: 22px 24px;
  border-radius: 18px;
  background:
    radial-gradient(circle at top right, rgba(37, 99, 235, 0.14), transparent 35%),
    var(--surface-color);
}

.toolbar-copy h2 {
  margin: 6px 0 4px;
  font-size: 28px;
}

.toolbar-subtitle,
.section-subtitle,
.stat-note,
.helper-text,
.meta-label {
  color: var(--text-secondary);
}

.toolbar-subtitle {
  margin: 0;
  max-width: 720px;
  line-height: 1.6;
}

.toolbar-actions,
.section-actions,
.status-stack,
.node-actions,
.table-actions,
.pagination-bar {
  display: flex;
  gap: 10px;
}

.toolbar-actions,
.section-actions,
.node-actions,
.table-actions {
  flex-wrap: wrap;
}

.feedback {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 16px;
  border-radius: 12px;
  border: 1px solid transparent;
}

.feedback-success {
  color: #047857;
  background: rgba(16, 185, 129, 0.08);
  border-color: rgba(16, 185, 129, 0.2);
}

.feedback-error {
  color: #b91c1c;
  background: rgba(239, 68, 68, 0.08);
  border-color: rgba(239, 68, 68, 0.2);
}

.feedback-close {
  border: 0;
  background: transparent;
  color: inherit;
  cursor: pointer;
  font-size: 18px;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 14px;
}

.stat-card {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 18px;
  border-radius: 16px;
}

.stat-label {
  margin: 0;
  font-size: 13px;
  color: var(--text-secondary);
}

.stat-value {
  font-size: 28px;
  font-weight: 700;
}

.stat-note {
  font-size: 12px;
}

.panel {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 20px;
  border-radius: 18px;
}

.section-header {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  align-items: flex-start;
}

.section-header h3 {
  margin: 6px 0 4px;
  font-size: 24px;
}

.section-subtitle {
  margin: 0;
  max-width: 720px;
  line-height: 1.6;
}

.filter-group,
.form-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.filter-group {
  min-width: 140px;
}

.filter-wide {
  min-width: 180px;
}

.filter-group span,
.form-group span {
  font-size: 13px;
  font-weight: 600;
}

.filter-group select,
.filter-group input,
.form-group input,
.form-group select,
.form-group textarea {
  width: 100%;
  padding: 10px 12px;
  border: 1px solid var(--border-color);
  border-radius: 10px;
  background: var(--bg-color);
  color: var(--text-color);
}

.form-group textarea {
  resize: vertical;
}

.nodes-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
  gap: 16px;
}

.node-card {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 18px;
  border-radius: 18px;
  border: 1px solid var(--border-color);
  background: linear-gradient(180deg, rgba(248, 250, 252, 0.75), rgba(255, 255, 255, 1));
}

.node-header {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  align-items: flex-start;
}

.node-header h4 {
  margin: 6px 0 4px;
  font-size: 20px;
}

.node-meta {
  margin: 0;
  color: var(--text-secondary);
}

.status-stack {
  align-items: flex-end;
  flex-direction: column;
}

.meta-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.meta-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.meta-label {
  font-size: 12px;
}

.meta-item code {
  padding: 2px 6px;
  border-radius: 8px;
  background: rgba(15, 23, 42, 0.06);
}

.node-result,
.connection-result {
  padding: 12px 14px;
  border-radius: 12px;
  border: 1px solid transparent;
}

.node-result p,
.connection-result p {
  margin: 8px 0 0;
}

.connection-ok,
.node-result {
  background: rgba(37, 99, 235, 0.05);
  border-color: rgba(37, 99, 235, 0.18);
}

.connection-fail {
  background: rgba(239, 68, 68, 0.08);
  border-color: rgba(239, 68, 68, 0.18);
}

.node-actions button,
.table-actions button {
  flex: 1 1 auto;
}

.rules-table-wrap {
  overflow-x: auto;
}

.rules-table {
  width: 100%;
  min-width: 960px;
  border-collapse: collapse;
}

.rules-table th,
.rules-table td {
  padding: 14px 12px;
  border-bottom: 1px solid var(--border-color);
  text-align: left;
  vertical-align: top;
}

.rules-table th {
  color: var(--text-secondary);
  font-size: 12px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.rule-name-cell,
.rule-endpoint,
.rule-limit-cell {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.rule-name-cell span,
.rule-endpoint span,
.rule-limit-cell span {
  color: var(--text-secondary);
  font-size: 13px;
}

.pagination-bar {
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
  color: var(--text-secondary);
}

.loading-state,
.empty-state,
.modal-loading {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 180px;
  text-align: center;
}

.loading-state,
.empty-state {
  gap: 12px;
  border-radius: 14px;
  border: 1px dashed var(--border-color);
  background: rgba(148, 163, 184, 0.06);
  padding: 24px;
}

.empty-state {
  flex-direction: column;
}

.empty-state h4 {
  margin: 0;
  font-size: 20px;
}

.empty-state p {
  margin: 0;
  max-width: 640px;
  color: var(--text-secondary);
}

.spinner {
  width: 28px;
  height: 28px;
  border: 3px solid rgba(37, 99, 235, 0.25);
  border-top-color: #2563eb;
  border-radius: 50%;
  animation: spin 0.9s linear infinite;
}

.tag {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 4px 10px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 600;
  white-space: nowrap;
}

.tag-relay {
  background: rgba(37, 99, 235, 0.1);
  color: #2563eb;
}

.tag-exit {
  background: rgba(249, 115, 22, 0.12);
  color: #c2410c;
}

.tag-success {
  background: rgba(16, 185, 129, 0.12);
  color: #047857;
}

.tag-danger {
  background: rgba(239, 68, 68, 0.12);
  color: #b91c1c;
}

.tag-muted {
  background: rgba(148, 163, 184, 0.14);
  color: var(--text-secondary);
}

.tag-outline {
  background: transparent;
  color: var(--text-secondary);
  border: 1px solid var(--border-color);
}

.modal-overlay {
  position: fixed;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
  background: rgba(15, 23, 42, 0.62);
  z-index: 1100;
}

.modal {
  width: min(100%, 640px);
  max-height: min(90vh, 960px);
  overflow: hidden;
  border-radius: 18px;
  border: 1px solid var(--border-color);
  background: var(--surface-color);
  box-shadow: 0 30px 70px rgba(15, 23, 42, 0.25);
  display: flex;
  flex-direction: column;
}

.modal-sm {
  width: min(100%, 420px);
}

.modal-md {
  width: min(100%, 560px);
}

.modal-lg {
  width: min(100%, 820px);
}

.modal-xl {
  width: min(100%, 920px);
}

.modal-header,
.modal-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 18px 22px;
  border-bottom: 1px solid var(--border-color);
}

.modal-footer {
  border-top: 1px solid var(--border-color);
  border-bottom: 0;
}

.modal-header h3 {
  margin: 6px 0 0;
  font-size: 22px;
}

.modal-close {
  border: 0;
  background: transparent;
  color: var(--text-secondary);
  cursor: pointer;
  font-size: 24px;
  line-height: 1;
}

.modal-body {
  padding: 20px 22px;
  overflow: auto;
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.modal-loading {
  color: var(--text-secondary);
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}

.form-group-span {
  grid-column: 1 / -1;
}

.form-group small,
.form-error {
  color: var(--error-color, #dc2626);
}

.eyebrow {
  margin: 0;
  font-size: 11px;
  letter-spacing: 0.18em;
  text-transform: uppercase;
  color: var(--text-secondary);
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

@media (max-width: 900px) {
  .toolbar,
  .section-header,
  .modal-header,
  .modal-footer {
    flex-direction: column;
    align-items: stretch;
  }

  .meta-grid,
  .form-grid {
    grid-template-columns: 1fr;
  }

  .status-stack {
    align-items: flex-start;
  }
}
</style>
