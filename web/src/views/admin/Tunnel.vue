<template>
  <div class="tunnel-page">
    <div class="toolbar">
      <div class="toolbar-copy">
        <p class="eyebrow">Flux Compatible</p>
        <h2>隧道管理</h2>
      </div>
      <div class="toolbar-actions">
        <button class="btn btn-primary" @click="openCreateModal">新增</button>
      </div>
    </div>

    <div v-if="feedback.message" :class="['feedback', `feedback-${feedback.type}`]">
      <span>{{ feedback.message }}</span>
      <button class="feedback-close" @click="clearFeedback">×</button>
    </div>

    <div v-if="loading" class="loading-state">
      <div class="spinner"></div>
      <span>正在加载隧道与节点数据...</span>
    </div>

    <template v-else>
      <section v-if="tunnels.length" class="card-grid">
        <article v-for="tunnel in tunnels" :key="tunnel.id" class="tunnel-card">
          <div class="card-head">
            <div class="card-title">
              <h3>{{ tunnel.name }}</h3>
              <p>{{ resolveTypeMeta(tunnel.type).text }}</p>
            </div>
            <div class="card-head-actions">
              <span :class="['tag', resolveTypeMeta(tunnel.type).className]">
                {{ resolveTypeMeta(tunnel.type).text }}
              </span>
              <span :class="['tag', resolveStatusMeta(tunnel.status).className]">
                {{ resolveStatusMeta(tunnel.status).text }}
              </span>
            </div>
          </div>

          <div class="meta-list">
            <div class="meta-item">
              <span class="meta-label">转发入口节点</span>
              <strong>{{ resolveNodeName(tunnel.inNodeId) }}</strong>
              <code>{{ tunnel.inIp || '-' }}</code>
            </div>
            <div class="meta-item">
              <span class="meta-label">转发出口节点</span>
              <strong>{{ resolveNodeName(tunnel.outNodeId || tunnel.inNodeId) }}</strong>
              <code>{{ tunnel.outIp || tunnel.inIp || '-' }}</code>
            </div>
            <div class="meta-item">
              <span class="meta-label">流量计算</span>
              <strong>{{ resolveFlowLabel(tunnel.flow) }}</strong>
            </div>
            <div class="meta-item">
              <span class="meta-label">流量倍率</span>
              <strong>{{ formatTrafficRatio(tunnel.trafficRatio) }}</strong>
            </div>
          </div>

          <div class="card-actions">
            <button class="btn btn-secondary btn-sm" @click="openEditModal(tunnel)">编辑</button>
            <button class="btn btn-secondary btn-sm" @click="openDiagnosisModal(tunnel)">诊断</button>
            <button class="btn btn-secondary btn-sm danger-text" @click="openDeleteModal(tunnel)">删除</button>
          </div>
        </article>
      </section>

      <section v-else class="empty-state">
        <h3>暂无隧道配置</h3>
        <p>先创建入口节点和出口节点，再新增第一个可供转发引用的隧道。</p>
      </section>
    </template>

    <div v-if="modalOpen" class="modal-overlay" @click.self="closeEditorModal">
      <div class="modal modal-lg">
        <div class="modal-header">
          <div>
            <p class="eyebrow">Tunnel</p>
            <h3>{{ isEdit ? '编辑隧道' : '新增隧道' }}</h3>
          </div>
          <button class="modal-close" @click="closeEditorModal">×</button>
        </div>

        <div class="modal-body">
          <div class="form-grid">
            <div class="form-group">
              <label>隧道名称</label>
              <input v-model.trim="form.name" type="text" maxlength="50" placeholder="例如：HK-Tunnel-01" />
              <p v-if="errors.name" class="form-error">{{ errors.name }}</p>
            </div>

            <div class="form-group">
              <label>隧道类型</label>
              <select v-model.number="form.type" :disabled="isEdit">
                <option :value="1">端口转发</option>
                <option :value="2">隧道转发</option>
              </select>
              <p v-if="errors.type" class="form-error">{{ errors.type }}</p>
            </div>
          </div>

          <div class="form-grid">
            <div class="form-group">
              <label>流量计算</label>
              <select v-model.number="form.flow">
                <option :value="1">单向计算</option>
                <option :value="2">双向计算</option>
              </select>
            </div>

            <div class="form-group">
              <label>流量倍率</label>
              <input v-model.number="form.trafficRatio" type="number" min="0.1" max="100" step="0.1" />
              <p v-if="errors.trafficRatio" class="form-error">{{ errors.trafficRatio }}</p>
            </div>
          </div>

          <div class="form-grid">
            <div class="form-group">
              <label>转发入口节点</label>
              <select
                data-test="forward-entry-select"
                v-model.number="form.inNodeId"
                :disabled="isEdit"
              >
                <option :value="0">请选择转发入口节点</option>
                <option v-for="node in relayNodeOptions" :key="node.id" :value="node.id">
                  {{ node.name }} · 转发入口节点 · {{ node.host }}
                </option>
              </select>
              <p v-if="errors.inNodeId" class="form-error">{{ errors.inNodeId }}</p>
            </div>

            <div class="form-group">
              <label>TCP 监听地址</label>
              <input v-model.trim="form.tcpListenAddr" type="text" placeholder="[::]" />
              <p v-if="errors.tcpListenAddr" class="form-error">{{ errors.tcpListenAddr }}</p>
            </div>
          </div>

          <div class="form-grid">
            <div class="form-group">
              <label>UDP 监听地址</label>
              <input v-model.trim="form.udpListenAddr" type="text" placeholder="[::]" />
              <p v-if="errors.udpListenAddr" class="form-error">{{ errors.udpListenAddr }}</p>
            </div>

            <div v-if="form.type === 2" class="form-group">
              <label>出口网卡名或 IP</label>
              <input v-model.trim="form.interfaceName" type="text" placeholder="例如：eth0 / 192.0.2.10" />
            </div>
          </div>

          <div v-if="form.type === 2" class="form-grid">
            <div class="form-group">
              <label>协议类型</label>
              <select v-model="form.protocol">
                <option value="tls">tls</option>
                <option value="tcp">tcp</option>
                <option value="udp">udp</option>
                <option value="ws">ws</option>
                <option value="wss">wss</option>
                <option value="grpc">grpc</option>
                <option value="quic">quic</option>
              </select>
              <p v-if="errors.protocol" class="form-error">{{ errors.protocol }}</p>
            </div>

            <div class="form-group">
              <label>转发出口节点</label>
              <select
                data-test="forward-exit-select"
                v-model.number="form.outNodeId"
                :disabled="isEdit"
              >
                <option :value="0">请选择转发出口节点</option>
                <option
                  v-for="node in exitNodeOptions"
                  :key="`out-${node.id}`"
                  :value="node.id"
                >
                  {{ node.name }} · 转发出口节点 · {{ node.host }}
                </option>
              </select>
              <p v-if="errors.outNodeId" class="form-error">{{ errors.outNodeId }}</p>
            </div>
          </div>
        </div>

        <div class="modal-footer">
          <button class="btn btn-secondary" @click="closeEditorModal">取消</button>
          <button class="btn btn-primary" :disabled="submitLoading" @click="handleSubmit">
            {{ submitLoading ? '提交中...' : (isEdit ? '更新' : '创建') }}
          </button>
        </div>
      </div>
    </div>

    <div v-if="deleteModalOpen" class="modal-overlay" @click.self="deleteModalOpen = false">
      <div class="modal">
        <div class="modal-header">
          <div>
            <p class="eyebrow">Delete</p>
            <h3>确认删除</h3>
          </div>
          <button class="modal-close" @click="deleteModalOpen = false">×</button>
        </div>
        <div class="modal-body">
          <p class="modal-copy">确认删除 <strong>{{ tunnelToDelete?.name }}</strong> 吗？</p>
          <p class="hint">如果该隧道仍被转发规则或用户权限引用，后端会阻止删除。</p>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary" @click="deleteModalOpen = false">取消</button>
          <button class="btn btn-primary danger" :disabled="deleteLoading" @click="confirmDelete">
            {{ deleteLoading ? '删除中...' : '确认删除' }}
          </button>
        </div>
      </div>
    </div>

    <div v-if="diagnosisModalOpen" class="modal-overlay" @click.self="diagnosisModalOpen = false">
      <div class="modal modal-xl">
        <div class="modal-header">
          <div>
            <p class="eyebrow">Diagnosis</p>
            <h3>隧道诊断结果</h3>
            <p v-if="currentDiagnosisTunnel" class="modal-subtitle">{{ currentDiagnosisTunnel.name }}</p>
          </div>
          <button class="modal-close" @click="diagnosisModalOpen = false">×</button>
        </div>

        <div class="modal-body">
          <div v-if="diagnosisLoading" class="loading-state compact">
            <div class="spinner"></div>
            <span>正在诊断隧道连通性...</span>
          </div>

          <div v-else-if="diagnosisResult?.results?.length" class="diagnosis-list">
            <article v-for="(result, index) in diagnosisResult.results" :key="`${result.nodeId}-${index}`" class="diagnosis-card">
              <div class="diagnosis-head">
                <div>
                  <h4>{{ result.description }}</h4>
                  <p>{{ result.nodeName }} · Node {{ result.nodeId }}</p>
                </div>
                <span :class="['tag', result.success ? 'tag-success' : 'tag-danger']">
                  {{ result.success ? '成功' : '失败' }}
                </span>
              </div>

              <div class="diagnosis-meta">
                <div>
                  <span class="meta-label">目标地址</span>
                  <code>{{ formatAddress(result.targetIp, result.targetPort) }}</code>
                </div>
                <div v-if="result.averageTime">
                  <span class="meta-label">耗时</span>
                  <strong>{{ result.averageTime.toFixed(0) }} ms</strong>
                </div>
                <div v-if="result.message">
                  <span class="meta-label">信息</span>
                  <strong>{{ result.message }}</strong>
                </div>
              </div>
            </article>
          </div>

          <div v-else class="empty-state compact">
            <h3>暂无诊断结果</h3>
            <p>当前没有可展示的节点诊断数据。</p>
          </div>
        </div>

        <div class="modal-footer">
          <button class="btn btn-secondary" @click="diagnosisModalOpen = false">关闭</button>
          <button class="btn btn-primary" :disabled="diagnosisLoading" @click="rerunDiagnosis">
            {{ diagnosisLoading ? '诊断中...' : '重新诊断' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import {
  createForwardTunnel,
  deleteForwardTunnel,
  diagnoseForwardTunnel,
  getAdminForwardTunnelList,
  getForwardNodes,
  updateForwardTunnel
} from '@/api/admin'

const loading = ref(true)
const tunnels = ref([])
const nodes = ref([])

const relayNodeOptions = computed(() =>
  nodes.value.filter(
    node => node.id > 0 && String(node.type ?? '').toLowerCase() === 'relay'
  )
)

const exitNodeOptions = computed(() =>
  nodes.value.filter(
    node => node.id > 0 && String(node.type ?? '').toLowerCase() === 'exit'
  )
)

const modalOpen = ref(false)
const deleteModalOpen = ref(false)
const diagnosisModalOpen = ref(false)
const isEdit = ref(false)

const submitLoading = ref(false)
const deleteLoading = ref(false)
const diagnosisLoading = ref(false)

const tunnelToDelete = ref(null)
const currentDiagnosisTunnel = ref(null)
const diagnosisResult = ref(null)

const feedback = reactive({
  type: 'info',
  message: ''
})

const form = reactive(createDefaultForm())
const errors = reactive({
  name: '',
  type: '',
  inNodeId: '',
  outNodeId: '',
  trafficRatio: '',
  protocol: '',
  tcpListenAddr: '',
  udpListenAddr: ''
})

onMounted(async () => {
  await loadData(true)
})

function createDefaultForm() {
  return {
    id: null,
    name: '',
    type: 1,
    inNodeId: 0,
    outNodeId: 0,
    flow: 1,
    trafficRatio: 1,
    protocol: 'tls',
    tcpListenAddr: '[::]',
    udpListenAddr: '[::]',
    interfaceName: ''
  }
}

function resetForm() {
  Object.assign(form, createDefaultForm())
  clearErrors()
}

function clearErrors() {
  errors.name = ''
  errors.type = ''
  errors.inNodeId = ''
  errors.outNodeId = ''
  errors.trafficRatio = ''
  errors.protocol = ''
  errors.tcpListenAddr = ''
  errors.udpListenAddr = ''
}

function setFeedback(type, message) {
  feedback.type = type
  feedback.message = message
}

function clearFeedback() {
  feedback.message = ''
}

function normalizeTunnel(raw) {
  return {
    ...raw,
    id: Number(raw.id ?? 0),
    inNodeId: Number(raw.inNodeId ?? raw.in_node_id ?? 0),
    outNodeId: raw.outNodeId == null && raw.out_node_id == null ? null : Number(raw.outNodeId ?? raw.out_node_id),
    type: Number(raw.type ?? 1),
    flow: Number(raw.flow ?? 1),
    trafficRatio: Number(raw.trafficRatio ?? raw.traffic_ratio ?? 1),
    protocol: raw.protocol ?? '',
    tcpListenAddr: raw.tcpListenAddr ?? raw.tcp_listen_addr ?? '[::]',
    udpListenAddr: raw.udpListenAddr ?? raw.udp_listen_addr ?? '[::]',
    interfaceName: raw.interfaceName ?? raw.interface_name ?? '',
    inIp: raw.inIp ?? raw.in_ip ?? '',
    outIp: raw.outIp ?? raw.out_ip ?? '',
    status: Number(raw.status ?? 1)
  }
}

function normalizeNode(raw) {
  return {
    ...raw,
    id: Number(raw.id ?? 0),
    host: raw.host ?? '',
    status: Number(raw.status ?? 0)
  }
}

function extractNodeList(response) {
  const list = response?.list ?? response?.data?.list ?? response?.data ?? []
  return Array.isArray(list) ? list.map(normalizeNode) : []
}

async function loadData(showLoading = true) {
  if (showLoading) {
    loading.value = true
  }

  clearFeedback()
  try {
    const [tunnelRes, nodeRes] = await Promise.all([getAdminForwardTunnelList(), getForwardNodes({ page_size: 200 })])

    if (tunnelRes.code === 0) {
      tunnels.value = Array.isArray(tunnelRes.data) ? tunnelRes.data.map(normalizeTunnel) : []
    } else {
      setFeedback('error', tunnelRes.msg || '获取隧道列表失败')
    }

    nodes.value = extractNodeList(nodeRes)
  } catch (error) {
    console.error('加载隧道页数据失败:', error)
    setFeedback('error', '加载数据失败')
  } finally {
    loading.value = false
  }
}

function resolveNodeName(nodeId) {
  const match = nodes.value.find(node => node.id === Number(nodeId))
  return match?.name || (nodeId ? `Node #${nodeId}` : '-')
}

function resolveTypeMeta(type) {
  return Number(type) === 2
    ? { text: '隧道转发', className: 'tag-primary' }
    : { text: '端口转发', className: 'tag-neutral' }
}

function resolveStatusMeta(status) {
  return Number(status) === 1
    ? { text: '启用', className: 'tag-success' }
    : { text: '禁用', className: 'tag-danger' }
}

function resolveFlowLabel(flow) {
  return Number(flow) === 2 ? '双向计算' : '单向计算'
}

function formatTrafficRatio(value) {
  const ratio = Number(value ?? 1)
  return `${ratio.toFixed(ratio % 1 === 0 ? 1 : 2)}x`
}

function formatAddress(host, port) {
  if (!host) {
    return '-'
  }
  return port ? `${host}:${port}` : host
}

function openCreateModal() {
  isEdit.value = false
  resetForm()
  modalOpen.value = true
}

function openEditModal(tunnel) {
  isEdit.value = true
  Object.assign(form, {
    id: tunnel.id,
    name: tunnel.name,
    type: tunnel.type,
    inNodeId: tunnel.inNodeId,
    outNodeId: tunnel.outNodeId || 0,
    flow: tunnel.flow,
    trafficRatio: tunnel.trafficRatio,
    protocol: tunnel.protocol || 'tls',
    tcpListenAddr: tunnel.tcpListenAddr || '[::]',
    udpListenAddr: tunnel.udpListenAddr || '[::]',
    interfaceName: tunnel.interfaceName || ''
  })
  clearErrors()
  modalOpen.value = true
}

function closeEditorModal() {
  modalOpen.value = false
}

function validateForm() {
  clearErrors()

  if (!form.name.trim()) {
    errors.name = '请输入隧道名称'
  } else if (form.name.trim().length < 2 || form.name.trim().length > 50) {
    errors.name = '隧道名称长度应在 2-50 个字符之间'
  }

  if (![1, 2].includes(Number(form.type))) {
    errors.type = '请选择有效的隧道类型'
  }

  if (!form.inNodeId) {
    errors.inNodeId = '请选择转发入口节点'
  } else if (!relayNodeOptions.value.some(node => node.id === Number(form.inNodeId))) {
    errors.inNodeId = '入口节点必须是转发中继节点'
  }

  const trafficRatio = Number(form.trafficRatio)
  if (!Number.isFinite(trafficRatio) || trafficRatio <= 0 || trafficRatio > 100) {
    errors.trafficRatio = '流量倍率必须在 0.1-100.0 之间'
  }

  if (!String(form.tcpListenAddr || '').trim()) {
    errors.tcpListenAddr = '请输入 TCP 监听地址'
  }

  if (!String(form.udpListenAddr || '').trim()) {
    errors.udpListenAddr = '请输入 UDP 监听地址'
  }

  if (Number(form.type) === 2) {
    if (!form.outNodeId) {
      errors.outNodeId = '请选择转发出口节点'
    } else if (Number(form.outNodeId) === Number(form.inNodeId)) {
      errors.outNodeId = '转发入口节点和转发出口节点不能相同'
    } else if (!exitNodeOptions.value.some(node => node.id === Number(form.outNodeId))) {
      errors.outNodeId = '出口节点必须是转发出口节点'
    }

    if (!String(form.protocol || '').trim()) {
      errors.protocol = '请选择协议类型'
    }
  }

  return Object.values(errors).every(value => !value)
}

async function handleSubmit() {
  if (!validateForm()) {
    return
  }

  submitLoading.value = true
  try {
    const payload = {
      name: form.name.trim(),
      flow: Number(form.flow),
      trafficRatio: Number(form.trafficRatio),
      protocol: String(form.protocol || 'tls').trim(),
      tcpListenAddr: String(form.tcpListenAddr || '[::]').trim(),
      udpListenAddr: String(form.udpListenAddr || '[::]').trim(),
      interfaceName: String(form.interfaceName || '').trim()
    }

    const response = isEdit.value
      ? await updateForwardTunnel({
          id: form.id,
          ...payload
        })
      : await createForwardTunnel({
          ...payload,
          type: Number(form.type),
          inNodeId: Number(form.inNodeId),
          outNodeId: Number(form.type) === 2 ? Number(form.outNodeId) : null,
          protocol: Number(form.type) === 2 ? payload.protocol : ''
        })

    if (response.code === 0) {
      modalOpen.value = false
      setFeedback('success', isEdit.value ? '隧道更新成功' : '隧道创建成功')
      await loadData(false)
      return
    }

    setFeedback('error', response.msg || '操作失败')
  } catch (error) {
    console.error('提交隧道失败:', error)
    setFeedback('error', '操作失败')
  } finally {
    submitLoading.value = false
  }
}

function openDeleteModal(tunnel) {
  tunnelToDelete.value = tunnel
  deleteModalOpen.value = true
}

async function confirmDelete() {
  if (!tunnelToDelete.value) {
    return
  }

  deleteLoading.value = true
  try {
    const response = await deleteForwardTunnel(tunnelToDelete.value.id)
    if (response.code === 0) {
      deleteModalOpen.value = false
      tunnelToDelete.value = null
      setFeedback('success', '隧道删除成功')
      await loadData(false)
      return
    }
    setFeedback('error', response.msg || '删除失败')
  } catch (error) {
    console.error('删除隧道失败:', error)
    setFeedback('error', '删除失败')
  } finally {
    deleteLoading.value = false
  }
}

async function runDiagnosis(tunnel) {
  diagnosisLoading.value = true
  diagnosisResult.value = null
  try {
    const response = await diagnoseForwardTunnel(tunnel.id)
    if (response.code === 0) {
      diagnosisResult.value = response.data
      return
    }
    diagnosisResult.value = {
      tunnelName: tunnel.name,
      tunnelType: resolveTypeMeta(tunnel.type).text,
      timestamp: Date.now(),
      results: [
        {
          success: false,
          description: '隧道诊断',
          nodeName: '-',
          nodeId: '-',
          targetIp: '-',
          message: response.msg || '诊断失败'
        }
      ]
    }
  } catch (error) {
    console.error('诊断隧道失败:', error)
    diagnosisResult.value = {
      tunnelName: tunnel.name,
      tunnelType: resolveTypeMeta(tunnel.type).text,
      timestamp: Date.now(),
      results: [
        {
          success: false,
          description: '隧道诊断',
          nodeName: '-',
          nodeId: '-',
          targetIp: '-',
          message: '诊断请求失败'
        }
      ]
    }
  } finally {
    diagnosisLoading.value = false
  }
}

async function openDiagnosisModal(tunnel) {
  currentDiagnosisTunnel.value = tunnel
  diagnosisModalOpen.value = true
  await runDiagnosis(tunnel)
}

async function rerunDiagnosis() {
  if (!currentDiagnosisTunnel.value) {
    return
  }
  await runDiagnosis(currentDiagnosisTunnel.value)
}
</script>

<style scoped>
.tunnel-page {
  display: grid;
  gap: 20px;
}

.toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
}

.toolbar-copy h2 {
  margin: 4px 0 0;
  font-size: 28px;
}

.eyebrow {
  margin: 0;
  color: var(--text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.14em;
  font-size: 12px;
}

.toolbar-actions {
  display: flex;
  gap: 12px;
}

.btn {
  border: 1px solid var(--border-color);
  border-radius: 14px;
  padding: 10px 16px;
  font-weight: 600;
  cursor: pointer;
  transition: 0.2s ease;
}

.btn:disabled {
  cursor: not-allowed;
  opacity: 0.65;
}

.btn-primary {
  background: linear-gradient(135deg, #1d4ed8, #2563eb);
  color: #fff;
  border-color: transparent;
}

.btn-secondary {
  background: var(--surface-color);
  color: var(--text-color);
}

.btn-sm {
  padding: 8px 12px;
  border-radius: 12px;
  font-size: 13px;
}

.danger {
  background: linear-gradient(135deg, #b91c1c, #dc2626);
}

.danger-text {
  color: #dc2626;
}

.feedback {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 14px 18px;
  border-radius: 16px;
  border: 1px solid var(--border-color);
}

.feedback-success {
  background: rgba(22, 163, 74, 0.1);
  border-color: rgba(22, 163, 74, 0.25);
}

.feedback-error {
  background: rgba(220, 38, 38, 0.1);
  border-color: rgba(220, 38, 38, 0.25);
}

.feedback-close,
.modal-close {
  border: none;
  background: transparent;
  color: inherit;
  font-size: 22px;
  cursor: pointer;
}

.loading-state,
.empty-state {
  display: grid;
  place-items: center;
  gap: 12px;
  min-height: 220px;
  padding: 32px;
  border-radius: 20px;
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  text-align: center;
}

.loading-state.compact,
.empty-state.compact {
  min-height: 160px;
}

.spinner {
  width: 36px;
  height: 36px;
  border-radius: 50%;
  border: 3px solid rgba(37, 99, 235, 0.18);
  border-top-color: #2563eb;
  animation: spin 0.8s linear infinite;
}

.card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: 18px;
}

.tunnel-card,
.diagnosis-card {
  display: grid;
  gap: 16px;
  padding: 20px;
  border-radius: 20px;
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.05), transparent), var(--surface-color);
  border: 1px solid var(--border-color);
}

.card-head,
.diagnosis-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.card-title h3,
.diagnosis-head h4 {
  margin: 0;
  font-size: 20px;
}

.card-title p,
.diagnosis-head p,
.modal-subtitle,
.hint,
.meta-label {
  margin: 4px 0 0;
  color: var(--text-secondary);
}

.card-head-actions,
.card-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.meta-list,
.diagnosis-meta {
  display: grid;
  gap: 12px;
}

.meta-item {
  display: grid;
  gap: 4px;
  padding: 12px 14px;
  border-radius: 16px;
  background: rgba(148, 163, 184, 0.08);
}

.meta-item code,
.diagnosis-meta code {
  word-break: break-all;
}

.tag {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 999px;
  padding: 6px 10px;
  font-size: 12px;
  font-weight: 700;
  background: rgba(148, 163, 184, 0.12);
}

.tag-primary {
  background: rgba(37, 99, 235, 0.16);
  color: #2563eb;
}

.tag-success {
  background: rgba(22, 163, 74, 0.16);
  color: #16a34a;
}

.tag-danger {
  background: rgba(220, 38, 38, 0.16);
  color: #dc2626;
}

.tag-neutral {
  background: rgba(148, 163, 184, 0.14);
  color: var(--text-color);
}

.modal-overlay {
  position: fixed;
  inset: 0;
  z-index: 50;
  display: grid;
  place-items: center;
  padding: 24px;
  background: rgba(15, 23, 42, 0.68);
}

.modal {
  width: min(100%, 560px);
  max-height: calc(100vh - 48px);
  overflow: auto;
  border-radius: 24px;
  background: var(--surface-color);
  border: 1px solid var(--border-color);
}

.modal-lg {
  width: min(100%, 760px);
}

.modal-xl {
  width: min(100%, 900px);
}

.modal-header,
.modal-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 20px 24px;
  border-bottom: 1px solid var(--border-color);
}

.modal-footer {
  border-bottom: none;
  border-top: 1px solid var(--border-color);
  justify-content: flex-end;
}

.modal-body {
  display: grid;
  gap: 18px;
  padding: 24px;
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}

.form-group {
  display: grid;
  gap: 8px;
}

.form-group input,
.form-group select {
  width: 100%;
  padding: 12px 14px;
  border-radius: 14px;
  border: 1px solid var(--border-color);
  background: var(--bg-color);
  color: var(--text-color);
}

.form-group input:disabled,
.form-group select:disabled {
  opacity: 0.7;
  cursor: not-allowed;
}

.form-error {
  margin: 0;
  color: #dc2626;
  font-size: 13px;
}

.modal-copy {
  margin: 0;
  line-height: 1.7;
}

.diagnosis-list {
  display: grid;
  gap: 14px;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

@media (max-width: 768px) {
  .toolbar,
  .modal-header,
  .modal-footer,
  .card-head,
  .diagnosis-head {
    flex-direction: column;
    align-items: stretch;
  }

  .form-grid {
    grid-template-columns: 1fr;
  }

  .modal-overlay {
    padding: 12px;
  }
}
</style>
