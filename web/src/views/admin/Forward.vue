<template>
  <div class="forward-page">
    <div class="toolbar">
      <div class="toolbar-copy">
        <p class="eyebrow">Flux Compatible</p>
        <h2>流量转发管理</h2>
      </div>
      <div class="toolbar-actions">
        <button
          class="btn btn-secondary icon-button"
          :title="viewMode === 'grouped' ? '切换到直连视图' : '切换到分组视图'"
          @click="toggleViewMode"
        >
          <span class="icon-mark">{{ viewMode === 'grouped' ? '直' : '组' }}</span>
          <span>{{ viewMode === 'grouped' ? '直连' : '分组' }}</span>
        </button>
        <button class="btn btn-secondary" @click="openImportModal">导入</button>
        <button class="btn btn-secondary" @click="openExportModal">导出</button>
        <button class="btn btn-primary" @click="openCreateModal">新增</button>
      </div>
    </div>
    <ForwardSuiteNav />

    <div v-if="feedback.message" :class="['feedback', `feedback-${feedback.type}`]">
      <span>{{ feedback.message }}</span>
      <button class="feedback-close" @click="clearFeedback">×</button>
    </div>

    <div v-if="loading" class="loading-state">
      <div class="spinner"></div>
      <span>正在加载转发与隧道数据...</span>
    </div>

    <template v-else>
      <section v-if="viewMode === 'grouped'" class="grouped-stack">
        <article v-for="userGroup in groupedForwards" :key="userGroup.userKey" class="user-group">
          <div class="user-group-head">
            <div>
              <p class="eyebrow">User</p>
              <h3>{{ userGroup.userName }}</h3>
              <p class="group-summary">{{ userGroup.tunnelGroups.length }} 个隧道，{{ userGroup.total }} 个转发</p>
            </div>
            <span class="tag tag-primary">用户</span>
          </div>

          <details v-for="tunnelGroup in userGroup.tunnelGroups" :key="`${userGroup.userKey}-${tunnelGroup.tunnelId}`" class="accordion" open>
            <summary>
              <div>
                <span class="accordion-title">{{ tunnelGroup.tunnelName }}</span>
                <span class="accordion-meta">Tunnel #{{ tunnelGroup.tunnelId }}</span>
              </div>
              <span class="tag">{{ tunnelGroup.running }}/{{ tunnelGroup.forwards.length }}</span>
            </summary>

            <div class="card-grid">
              <article v-for="forward in tunnelGroup.forwards" :key="forward.id" class="forward-card">
                <div class="card-head">
                  <div class="card-title">
                    <h4>{{ forward.name }}</h4>
                    <p>{{ forward.tunnelName }}</p>
                  </div>
                  <div class="card-head-actions">
                    <label class="switch">
                      <input
                        type="checkbox"
                        :checked="forward.serviceRunning"
                        :disabled="Number(forward.status) !== 0 && Number(forward.status) !== 1"
                        @change="handleToggleService(forward)"
                      />
                      <span class="switch-slider"></span>
                    </label>
                    <span :class="['tag', getStatusMeta(forward.status).className]">
                      {{ getStatusMeta(forward.status).text }}
                    </span>
                  </div>
                </div>

                <button class="endpoint" type="button" @click="showAddressModal({ value: forward.inIp, port: forward.inPort, title: '入口地址' })">
                  <span>入口</span>
                  <code>{{ formatInAddress(forward.inIp, forward.inPort) }}</code>
                </button>

                <button class="endpoint" type="button" @click="showAddressModal({ value: forward.remoteAddr, title: '目标地址' })">
                  <span>目标</span>
                  <code>{{ formatRemoteAddress(forward.remoteAddr) }}</code>
                </button>

                <div class="card-stats">
                  <span :class="['tag', getStrategyMeta(forward.strategy).className]">
                    {{ getStrategyMeta(forward.strategy).text }}
                  </span>
                  <span class="tag">入 {{ formatFlow(forward.inFlow || 0) }}</span>
                  <span class="tag tag-success">出 {{ formatFlow(forward.outFlow || 0) }}</span>
                </div>

                <div class="card-actions">
                  <button class="btn btn-secondary btn-sm" @click="openEditModal(forward)">编辑</button>
                  <button class="btn btn-secondary btn-sm" @click="openDiagnosisModal(forward)">诊断</button>
                  <button class="btn btn-secondary btn-sm danger-text" @click="openDeleteModal(forward)">删除</button>
                </div>
              </article>
            </div>
          </details>
        </article>

        <section v-if="!groupedForwards.length" class="empty-state">
          <h3>暂无转发配置</h3>
          <p>当前系统里还没有任何兼容 flux-panel 的转发记录。</p>
        </section>
      </section>

      <section v-else class="direct-stack">
        <div class="card-grid">
          <article
            v-for="forward in sortedDirectForwards"
            :key="forward.id"
            class="forward-card"
            :class="{ dragging: draggingId === forward.id, dragover: dragOverId === forward.id }"
            draggable="true"
            @dragstart="onDragStart($event, forward.id)"
            @dragenter.prevent="onDragEnter(forward.id)"
            @dragover.prevent="onDragEnter(forward.id)"
            @drop.prevent="onDrop(forward.id)"
            @dragend="onDragEnd"
          >
            <div class="card-head">
              <div class="card-title">
                <h4>{{ forward.name }}</h4>
                <p>{{ forward.tunnelName }}</p>
              </div>
              <div class="card-head-actions">
                <span :class="['drag-handle', { visible: isMobile }]" title="拖拽排序">⋮⋮</span>
                <label class="switch">
                  <input
                    type="checkbox"
                    :checked="forward.serviceRunning"
                    :disabled="Number(forward.status) !== 0 && Number(forward.status) !== 1"
                    @change="handleToggleService(forward)"
                  />
                  <span class="switch-slider"></span>
                </label>
                <span :class="['tag', getStatusMeta(forward.status).className]">
                  {{ getStatusMeta(forward.status).text }}
                </span>
              </div>
            </div>

            <button class="endpoint" type="button" @click="showAddressModal({ value: forward.inIp, port: forward.inPort, title: '入口地址' })">
              <span>入口</span>
              <code>{{ formatInAddress(forward.inIp, forward.inPort) }}</code>
            </button>

            <button class="endpoint" type="button" @click="showAddressModal({ value: forward.remoteAddr, title: '目标地址' })">
              <span>目标</span>
              <code>{{ formatRemoteAddress(forward.remoteAddr) }}</code>
            </button>

            <div class="card-stats">
              <span :class="['tag', getStrategyMeta(forward.strategy).className]">
                {{ getStrategyMeta(forward.strategy).text }}
              </span>
              <span class="tag">入 {{ formatFlow(forward.inFlow || 0) }}</span>
              <span class="tag tag-success">出 {{ formatFlow(forward.outFlow || 0) }}</span>
            </div>

            <div class="card-actions">
              <button class="btn btn-secondary btn-sm" @click="openEditModal(forward)">编辑</button>
              <button class="btn btn-secondary btn-sm" @click="openDiagnosisModal(forward)">诊断</button>
              <button class="btn btn-secondary btn-sm danger-text" @click="openDeleteModal(forward)">删除</button>
            </div>
          </article>
        </div>

        <section v-if="!sortedDirectForwards.length" class="empty-state">
          <h3>暂无转发配置</h3>
          <p>创建第一条转发后，这里会显示当前转发的直连卡片视图。</p>
        </section>
      </section>
    </template>

    <div v-if="modalOpen" class="modal-overlay" @click.self="closeEditorModal">
      <div class="modal modal-lg">
        <div class="modal-header">
          <div>
            <p class="eyebrow">Forward</p>
            <h3>{{ isEdit ? '编辑转发' : '新增转发' }}</h3>
          </div>
          <button class="modal-close" @click="closeEditorModal">×</button>
        </div>

        <div class="modal-body">
          <div class="form-grid">
            <div class="form-group">
              <label>转发名称</label>
              <input v-model.trim="form.name" type="text" maxlength="50" placeholder="例如：HK-Web-01" />
              <p v-if="errors.name" class="form-error">{{ errors.name }}</p>
            </div>

            <div class="form-group">
              <label>关联隧道</label>
              <select :value="form.tunnelId ?? ''" @change="handleTunnelChange($event.target.value)">
                <option value="">请选择隧道</option>
                <option v-for="tunnel in tunnels" :key="tunnel.id" :value="tunnel.id">{{ tunnel.name }}</option>
              </select>
              <p v-if="errors.tunnelId" class="form-error">{{ errors.tunnelId }}</p>
            </div>
          </div>

          <div class="form-grid">
            <div class="form-group">
              <label>入口端口</label>
              <input v-model="portInput" type="number" min="1" max="65535" placeholder="留空自动分配" />
              <p v-if="selectedTunnel && selectedTunnel.inNodePortSta && selectedTunnel.inNodePortEnd" class="hint">
                允许范围：{{ selectedTunnel.inNodePortSta }} - {{ selectedTunnel.inNodePortEnd }}
              </p>
              <p v-if="errors.inPort" class="form-error">{{ errors.inPort }}</p>
            </div>

            <div class="form-group">
              <label>网卡名称</label>
              <input v-model.trim="form.interfaceName" type="text" placeholder="可选，例如 eth0" />
            </div>
          </div>

          <div class="form-group">
            <label>目标地址</label>
            <textarea
              v-model="form.remoteAddr"
              rows="7"
              placeholder="每行一个目标，例如：&#10;1.1.1.1:443&#10;example.com:8443&#10;[2001:db8::1]:443"
            ></textarea>
            <p class="hint">支持 IPv4:port、domain:port、[完整 IPv6]:port。多地址请每行一个。</p>
            <p v-if="errors.remoteAddr" class="form-error">{{ errors.remoteAddr }}</p>
          </div>

          <div v-if="addressLineCount > 1" class="form-group">
            <label>调度策略</label>
            <select v-model="form.strategy">
              <option value="fifo">主备</option>
              <option value="round">轮询</option>
              <option value="rand">随机</option>
              <option value="hash">Hash</option>
            </select>
          </div>
        </div>

        <div class="modal-footer">
          <button class="btn btn-secondary" @click="closeEditorModal">取消</button>
          <button class="btn btn-primary" :disabled="submitLoading" @click="handleSubmit">
            {{ submitLoading ? '提交中...' : (isEdit ? '保存修改' : '创建转发') }}
          </button>
        </div>
      </div>
    </div>

    <div v-if="deleteModalOpen" class="modal-overlay" @click.self="deleteModalOpen = false">
      <div class="modal">
        <div class="modal-header">
          <div>
            <p class="eyebrow">Delete</p>
            <h3>删除转发</h3>
          </div>
          <button class="modal-close" @click="deleteModalOpen = false">×</button>
        </div>
        <div class="modal-body">
          <p class="modal-copy">确认删除 <strong>{{ forwardToDelete?.name }}</strong> 吗？</p>
          <p class="hint">常规删除失败时，会继续给出强制删除确认。</p>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary" @click="deleteModalOpen = false">取消</button>
          <button class="btn btn-primary danger" :disabled="deleteLoading" @click="confirmDelete">
            {{ deleteLoading ? '删除中...' : '确认删除' }}
          </button>
        </div>
      </div>
    </div>

    <div v-if="addressModalOpen" class="modal-overlay" @click.self="addressModalOpen = false">
      <div class="modal modal-lg">
        <div class="modal-header">
          <div>
            <p class="eyebrow">Address</p>
            <h3>{{ addressModalTitle }}</h3>
          </div>
          <button class="modal-close" @click="addressModalOpen = false">×</button>
        </div>
        <div class="modal-body">
          <div class="modal-toolbar">
            <button class="btn btn-secondary btn-sm" @click="copyAllAddresses">复制全部</button>
          </div>
          <div class="list-stack">
            <div v-for="item in addressList" :key="item.id" class="list-item">
              <code>{{ item.address }}</code>
              <button class="btn btn-secondary btn-sm" :disabled="item.copying" @click="copyAddress(item)">
                {{ item.copying ? '复制中...' : '复制' }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <div v-if="exportModalOpen" class="modal-overlay" @click.self="exportModalOpen = false">
      <div class="modal modal-lg">
        <div class="modal-header">
          <div>
            <p class="eyebrow">Export</p>
            <h3>导出转发数据</h3>
            <p class="modal-subtitle">格式：remoteAddr|name|inPort</p>
          </div>
          <button class="modal-close" @click="exportModalOpen = false">×</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label>选择导出隧道</label>
            <select :value="selectedTunnelForExport ?? ''" @change="handleExportTunnelChange($event.target.value)">
              <option value="">请选择隧道</option>
              <option v-for="tunnel in tunnels" :key="tunnel.id" :value="tunnel.id">{{ tunnel.name }}</option>
            </select>
          </div>

          <div v-if="exportData" class="modal-toolbar">
            <button class="btn btn-primary btn-sm" :disabled="exportLoading" @click="executeExport">
              {{ exportLoading ? '生成中...' : '重新生成' }}
            </button>
            <button class="btn btn-secondary btn-sm" @click="copyExportData">复制</button>
          </div>

          <div v-else class="modal-toolbar align-end">
            <button class="btn btn-primary btn-sm" :disabled="exportLoading || !selectedTunnelForExport" @click="executeExport">
              {{ exportLoading ? '生成中...' : '生成导出数据' }}
            </button>
          </div>

          <textarea v-if="exportData" :value="exportData" class="mono-area" rows="12" readonly placeholder="暂无导出数据"></textarea>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary" @click="exportModalOpen = false">关闭</button>
        </div>
      </div>
    </div>

    <div v-if="importModalOpen" class="modal-overlay" @click.self="importModalOpen = false">
      <div class="modal modal-xl">
        <div class="modal-header">
          <div>
            <p class="eyebrow">Import</p>
            <h3>导入转发数据</h3>
            <p class="modal-subtitle">格式：remoteAddr|name|inPort，每行一条，inPort 可留空。</p>
            <p class="modal-subtitle muted">目标地址支持单地址或逗号拼接的多地址，例如：3.3.3.3:3,4.4.4.4:4</p>
          </div>
          <button class="modal-close" @click="importModalOpen = false">×</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label>选择导入隧道</label>
            <select :value="selectedTunnelForImport ?? ''" @change="handleImportTunnelChange($event.target.value)">
              <option value="">请选择隧道</option>
              <option v-for="tunnel in tunnels" :key="tunnel.id" :value="tunnel.id">{{ tunnel.name }}</option>
            </select>
          </div>

          <div class="form-group">
            <label>导入数据</label>
            <textarea
              v-model="importData"
              class="mono-area"
              rows="10"
              placeholder="example.com:8080|业务入口|10086"
            ></textarea>
          </div>

          <div v-if="importResults.length" class="result-panel">
            <div class="result-head">
              <h4>导入结果</h4>
              <span>成功：{{ importSuccessCount }} / 总计：{{ importResults.length }}</span>
            </div>
            <div class="result-list">
              <div
                v-for="(result, index) in importResults"
                :key="`${result.line}-${index}`"
                :class="['result-item', result.success ? 'result-success' : 'result-failed']"
              >
                <div class="result-status">{{ result.success ? '成功' : '失败' }}</div>
                <code>{{ result.line }}</code>
                <p>{{ result.message }}</p>
              </div>
            </div>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary" @click="importModalOpen = false">关闭</button>
          <button class="btn btn-primary" :disabled="importLoading || !importData.trim() || !selectedTunnelForImport" @click="executeImport">
            {{ importLoading ? '导入中...' : '开始导入' }}
          </button>
        </div>
      </div>
    </div>

    <div v-if="diagnosisModalOpen" class="modal-overlay" @click.self="diagnosisModalOpen = false">
      <div class="modal modal-xl">
        <div class="modal-header">
          <div>
            <p class="eyebrow">Diagnosis</p>
            <h3>转发诊断结果</h3>
            <p v-if="currentDiagnosisForward" class="modal-subtitle">{{ currentDiagnosisForward.name }}</p>
          </div>
          <button class="modal-close" @click="diagnosisModalOpen = false">×</button>
        </div>
        <div class="modal-body">
          <div v-if="diagnosisLoading" class="loading-state compact">
            <div class="spinner"></div>
            <span>正在诊断转发连接...</span>
          </div>

          <div v-else-if="diagnosisResult && diagnosisResult.results?.length" class="diagnosis-list">
            <article v-for="(result, index) in diagnosisResult.results" :key="`${result.targetIp}-${index}`" class="diagnosis-card">
              <div class="diagnosis-head">
                <div>
                  <h4>{{ result.description }}</h4>
                  <p>{{ result.nodeName }} · Node {{ result.nodeId }}</p>
                </div>
                <span :class="['tag', result.success ? 'tag-success' : 'tag-danger']">
                  {{ result.success ? '连接成功' : '连接失败' }}
                </span>
              </div>

              <div class="diagnosis-body">
                <div class="diagnosis-metric">
                  <span>目标地址</span>
                  <code>{{ result.targetIp }}<template v-if="result.targetPort">:{{ result.targetPort }}</template></code>
                </div>

                <template v-if="result.success">
                  <div class="metric-grid">
                    <div class="metric-card">
                      <span>平均延迟</span>
                      <strong>{{ result.averageTime?.toFixed(0) || '0' }} ms</strong>
                    </div>
                    <div class="metric-card">
                      <span>丢包率</span>
                      <strong>{{ result.packetLoss?.toFixed(1) || '0.0' }}%</strong>
                    </div>
                    <div class="metric-card">
                      <span>质量</span>
                      <strong>{{ getQualityMeta(result.averageTime, result.packetLoss).text }}</strong>
                    </div>
                  </div>
                </template>

                <p v-else class="diagnosis-error">{{ result.message || '诊断失败' }}</p>
              </div>
            </article>
          </div>

          <div v-else class="empty-state compact">
            <h3>暂无诊断数据</h3>
            <p>发起一次诊断后，这里会展示与参考页一致的结果卡片。</p>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary" @click="diagnosisModalOpen = false">关闭</button>
          <button v-if="currentDiagnosisForward" class="btn btn-primary" :disabled="diagnosisLoading" @click="openDiagnosisModal(currentDiagnosisForward)">
            {{ diagnosisLoading ? '诊断中...' : '重新诊断' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useUserStore } from '@/stores/user'
import {
  createForward,
  getForwardList,
  updateForward,
  deleteForward,
  forceDeleteForward,
  pauseForwardService,
  resumeForwardService,
  diagnoseForward,
  updateForwardOrder,
  getForwardTunnels
} from '@/api/admin'
import ForwardSuiteNav from '@/components/admin/ForwardSuiteNav.vue'

const userStore = useUserStore()

const loading = ref(true)
const isMobile = ref(false)
const viewMode = ref(getSavedViewMode())
const forwardOrder = ref(getSavedOrder())
const forwards = ref([])
const tunnels = ref([])

const modalOpen = ref(false)
const deleteModalOpen = ref(false)
const addressModalOpen = ref(false)
const diagnosisModalOpen = ref(false)
const exportModalOpen = ref(false)
const importModalOpen = ref(false)
const isEdit = ref(false)

const submitLoading = ref(false)
const deleteLoading = ref(false)
const diagnosisLoading = ref(false)
const exportLoading = ref(false)
const importLoading = ref(false)

const forwardToDelete = ref(null)
const currentDiagnosisForward = ref(null)
const diagnosisResult = ref(null)
const addressModalTitle = ref('')
const addressList = ref([])
const exportData = ref('')
const selectedTunnelForExport = ref(null)
const importData = ref('')
const selectedTunnelForImport = ref(null)
const importResults = ref([])
const selectedTunnel = ref(null)

const draggingId = ref(null)
const dragOverId = ref(null)
const portInput = ref('')

const feedback = reactive({
  type: 'info',
  message: ''
})

const form = reactive({
  id: null,
  userId: null,
  name: '',
  tunnelId: null,
  inPort: null,
  remoteAddr: '',
  interfaceName: '',
  strategy: 'fifo'
})

const errors = reactive({
  name: '',
  tunnelId: '',
  remoteAddr: '',
  inPort: ''
})

let feedbackTimer = null

const currentUserId = computed(() => resolveCurrentUserId())
const addressLineCount = computed(() => splitLines(form.remoteAddr).length)
const sortedDirectForwards = computed(() => getSortedForwards('direct'))
const groupedForwards = computed(() => buildGroupedForwards())
const importSuccessCount = computed(() => importResults.value.filter(item => item.success).length)

watch(portInput, value => {
  if (value === '' || value === null) {
    form.inPort = null
    return
  }
  const parsed = Number(value)
  form.inPort = Number.isFinite(parsed) ? parsed : null
})

watch(
  () => form.tunnelId,
  value => {
    selectedTunnel.value = tunnels.value.find(item => Number(item.id) === Number(value)) || null
    if (errors.tunnelId) errors.tunnelId = ''
  }
)

watch(
  () => form.name,
  () => {
    if (errors.name) errors.name = ''
  }
)

watch(
  () => form.remoteAddr,
  () => {
    if (errors.remoteAddr) errors.remoteAddr = ''
  }
)

watch(
  () => form.inPort,
  () => {
    if (errors.inPort) errors.inPort = ''
  }
)

onMounted(async () => {
  updateViewport()
  window.addEventListener('resize', updateViewport)

  if (!currentUserId.value && userStore.isLoggedIn) {
    await userStore.getUserInfo()
  }

  await loadData(true)
})

onUnmounted(() => {
  window.removeEventListener('resize', updateViewport)
  clearFeedback()
})

function getSavedViewMode() {
  try {
    const saved = localStorage.getItem('forward-view-mode')
    return saved === 'grouped' || saved === 'direct' ? saved : 'direct'
  } catch {
    return 'direct'
  }
}

function getSavedOrder() {
  try {
    const saved = localStorage.getItem('forward-order')
    if (!saved) return []
    const parsed = JSON.parse(saved)
    return Array.isArray(parsed) ? parsed.map(item => Number(item)).filter(item => Number.isFinite(item)) : []
  } catch {
    return []
  }
}

function saveOrder(order) {
  try {
    localStorage.setItem('forward-order', JSON.stringify(order))
  } catch (error) {
    console.warn('无法保存排序到 localStorage:', error)
  }
}

function resolveCurrentUserId() {
  const storeId = Number(
    userStore.userInfo?.id ??
      userStore.userInfo?.user_id ??
      userStore.userInfo?.ID ??
      0
  )
  if (Number.isFinite(storeId) && storeId > 0) {
    return storeId
  }

  const token = userStore.token || localStorage.getItem('token') || ''
  if (!token || !token.includes('.')) {
    return null
  }

  try {
    const payload = token.split('.')[1]
    const normalized = payload.replace(/-/g, '+').replace(/_/g, '/')
    const decoded = atob(normalized.padEnd(Math.ceil(normalized.length / 4) * 4, '='))
    const parsed = JSON.parse(decoded)
    const id = Number(parsed.user_id ?? parsed.id ?? parsed.uid ?? parsed.sub ?? 0)
    return Number.isFinite(id) && id > 0 ? id : null
  } catch {
    return null
  }
}

function updateViewport() {
  isMobile.value = window.innerWidth < 768
}

function setFeedback(type, message) {
  feedback.type = type
  feedback.message = message
  if (feedbackTimer) {
    clearTimeout(feedbackTimer)
  }
  feedbackTimer = setTimeout(() => {
    feedback.message = ''
    feedbackTimer = null
  }, 3600)
}

function clearFeedback() {
  if (feedbackTimer) {
    clearTimeout(feedbackTimer)
    feedbackTimer = null
  }
  feedback.message = ''
}

function normalizeTunnel(raw) {
  return {
    ...raw,
    inIp: raw.inIp ?? raw.in_ip ?? '',
    inNodePortSta: raw.inNodePortSta ?? raw.in_node_port_sta ?? null,
    inNodePortEnd: raw.inNodePortEnd ?? raw.in_node_port_end ?? null
  }
}

function normalizeForward(raw) {
  return {
    ...raw,
    id: Number(raw.id),
    tunnelId: Number(raw.tunnelId),
    inPort: Number(raw.inPort ?? 0),
    status: Number(raw.status ?? 0),
    inx: raw.inx == null ? 0 : Number(raw.inx),
    userId: raw.userId == null ? null : Number(raw.userId),
    serviceRunning: Number(raw.status) === 1
  }
}

function mergeReferencedTunnels(list, forwardList) {
  const merged = Array.isArray(list) ? [...list] : []
  const tunnelMap = new Map(merged.map(item => [Number(item.id), item]))

  for (const forward of Array.isArray(forwardList) ? forwardList : []) {
    const tunnelId = Number(forward.tunnelId)
    if (!Number.isFinite(tunnelId) || tunnelId <= 0 || tunnelMap.has(tunnelId)) {
      continue
    }

    const fallbackTunnel = normalizeTunnel({
      id: tunnelId,
      name: forward.tunnelName || `Tunnel #${tunnelId}`,
      inIp: forward.inIp || '',
      status: 0
    })
    merged.push(fallbackTunnel)
    tunnelMap.set(tunnelId, fallbackTunnel)
  }

  return merged.sort((a, b) => String(a.name || '').localeCompare(String(b.name || '')))
}

function filterCurrentUserForwards(list) {
  if (!Array.isArray(list)) return []
  if (currentUserId.value == null) return list
  return list.filter(forward => Number(forward.userId) === Number(currentUserId.value))
}

function hasValidInx(forward) {
  return forward?.inx !== undefined && forward?.inx !== null && Number(forward.inx) !== 0
}

function initializeOrder(list) {
  const userForwards = filterCurrentUserForwards(list)
  if (!userForwards.length) {
    forwardOrder.value = []
    saveOrder([])
    return
  }

  const hasDbOrdering = userForwards.some(hasValidInx)
  if (hasDbOrdering) {
    const dbOrder = [...userForwards]
      .sort((a, b) => (a.inx ?? 0) - (b.inx ?? 0))
      .map(item => item.id)
    forwardOrder.value = dbOrder
    saveOrder(dbOrder)
    return
  }

  const savedOrder = getSavedOrder()
  if (savedOrder.length) {
    const validOrder = savedOrder.filter(id => userForwards.some(item => item.id === id))
    userForwards.forEach(forward => {
      if (!validOrder.includes(forward.id)) {
        validOrder.push(forward.id)
      }
    })
    forwardOrder.value = validOrder
    saveOrder(validOrder)
    return
  }

  const order = userForwards.map(item => item.id)
  forwardOrder.value = order
  saveOrder(order)
}

async function loadData(showLoading = true) {
  if (showLoading) {
    loading.value = true
  }

  try {
    const [forwardsRes, tunnelsRes] = await Promise.all([getForwardList(), getForwardTunnels()])
    let items = forwards.value
    let availableTunnels = tunnels.value

    if (forwardsRes.code === 0) {
      items = Array.isArray(forwardsRes.data) ? forwardsRes.data.map(normalizeForward) : []
      forwards.value = items
      if (viewMode.value === 'direct') {
        initializeOrder(items)
      }
    } else {
      setFeedback('error', forwardsRes.msg || '获取转发列表失败')
    }

    if (tunnelsRes.code === 0) {
      availableTunnels = Array.isArray(tunnelsRes.data) ? tunnelsRes.data.map(normalizeTunnel) : []
    } else {
      setFeedback('warning', tunnelsRes.msg || '获取隧道列表失败')
    }
    tunnels.value = mergeReferencedTunnels(availableTunnels, items)
  } catch (error) {
    console.error('加载转发页数据失败:', error)
    setFeedback('error', '加载数据失败')
  } finally {
    loading.value = false
  }
}

function getSortedForwards(mode = viewMode.value) {
  if (!Array.isArray(forwards.value) || !forwards.value.length) {
    return []
  }

  let filtered = forwards.value
  if (mode === 'direct') {
    filtered = filterCurrentUserForwards(forwards.value)
  }

  if (!filtered.length) {
    return []
  }

  const sorted = [...filtered].sort((a, b) => (a.inx ?? 0) - (b.inx ?? 0))

  if (forwardOrder.value.length && sorted.every(item => !hasValidInx(item))) {
    const forwardMap = new Map(filtered.map(item => [item.id, item]))
    const localSorted = []

    forwardOrder.value.forEach(id => {
      const match = forwardMap.get(id)
      if (match) {
        localSorted.push(match)
      }
    })

    filtered.forEach(item => {
      if (!forwardOrder.value.includes(item.id)) {
        localSorted.push(item)
      }
    })

    return localSorted
  }

  return sorted
}

function buildGroupedForwards() {
  const userMap = new Map()
  const sorted = getSortedForwards('grouped')

  sorted.forEach(forward => {
    const userKey = forward.userId ? String(forward.userId) : 'unknown'
    const userName = forward.userName || '未知用户'

    if (!userMap.has(userKey)) {
      userMap.set(userKey, {
        userKey,
        userName,
        total: 0,
        tunnelGroups: []
      })
    }

    const userGroup = userMap.get(userKey)
    userGroup.total += 1

    let tunnelGroup = userGroup.tunnelGroups.find(item => item.tunnelId === forward.tunnelId)
    if (!tunnelGroup) {
      tunnelGroup = {
        tunnelId: forward.tunnelId,
        tunnelName: forward.tunnelName || `Tunnel #${forward.tunnelId}`,
        running: 0,
        forwards: []
      }
      userGroup.tunnelGroups.push(tunnelGroup)
    }

    tunnelGroup.forwards.push(forward)
    if (forward.serviceRunning || Number(forward.status) === 1) {
      tunnelGroup.running += 1
    }
  })

  return Array.from(userMap.values())
    .sort((a, b) => a.userName.localeCompare(b.userName))
    .map(group => ({
      ...group,
      tunnelGroups: [...group.tunnelGroups].sort((a, b) => a.tunnelName.localeCompare(b.tunnelName))
    }))
}

function toggleViewMode() {
  viewMode.value = viewMode.value === 'grouped' ? 'direct' : 'grouped'
  try {
    localStorage.setItem('forward-view-mode', viewMode.value)
  } catch (error) {
    console.warn('无法保存显示模式到 localStorage:', error)
  }

  if (viewMode.value === 'direct') {
    initializeOrder(forwards.value)
  }
}

function handleTunnelChange(value) {
  form.tunnelId = value ? Number(value) : null
}

function handleExportTunnelChange(value) {
  selectedTunnelForExport.value = value ? Number(value) : null
}

function handleImportTunnelChange(value) {
  selectedTunnelForImport.value = value ? Number(value) : null
}

function splitLines(value) {
  return String(value || '')
    .split('\n')
    .map(item => item.trim())
    .filter(Boolean)
}

function validateForm() {
  errors.name = ''
  errors.tunnelId = ''
  errors.remoteAddr = ''
  errors.inPort = ''

  if (!form.name.trim()) {
    errors.name = '请输入转发名称'
  } else if (form.name.length < 2 || form.name.length > 50) {
    errors.name = '转发名称长度应在 2-50 个字符之间'
  }

  if (!form.tunnelId) {
    errors.tunnelId = '请选择关联隧道'
  }

  if (!form.remoteAddr.trim()) {
    errors.remoteAddr = '请输入远程地址'
  } else {
    const addresses = splitLines(form.remoteAddr)
    const ipv4Pattern = /^(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?):\d+$/
    const ipv6FullPattern = /^\[((([0-9a-fA-F]{1,4}:){7}([0-9a-fA-F]{1,4}|:))|(([0-9a-fA-F]{1,4}:){6}(:[0-9a-fA-F]{1,4}|((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3})|:))|(([0-9a-fA-F]{1,4}:){5}(((:[0-9a-fA-F]{1,4}){1,2})|:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3})|:))|(([0-9a-fA-F]{1,4}:){4}(((:[0-9a-fA-F]{1,4}){1,3})|((:[0-9a-fA-F]{1,4})?:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:))|(([0-9a-fA-F]{1,4}:){3}(((:[0-9a-fA-F]{1,4}){1,4})|((:[0-9a-fA-F]{1,4}){0,2}:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:))|(([0-9a-fA-F]{1,4}:){2}(((:[0-9a-fA-F]{1,4}){1,5})|((:[0-9a-fA-F]{1,4}){0,3}:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:))|(([0-9a-fA-F]{1,4}:){1}(((:[0-9a-fA-F]{1,4}){1,6})|((:[0-9a-fA-F]{1,4}){0,4}:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:))|(:(((:[0-9a-fA-F]{1,4}){1,7})|((:[0-9a-fA-F]{1,4}){0,5}:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:)))\]:\d+$/
    const domainPattern = /^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*:\d+$/

    for (let index = 0; index < addresses.length; index += 1) {
      const address = addresses[index]
      if (!ipv4Pattern.test(address) && !ipv6FullPattern.test(address) && !domainPattern.test(address)) {
        errors.remoteAddr = `第 ${index + 1} 行地址格式错误`
        break
      }
    }
  }

  if (form.inPort !== null && (form.inPort < 1 || form.inPort > 65535)) {
    errors.inPort = '端口号必须在 1-65535 之间'
  }

  if (
    selectedTunnel.value &&
    selectedTunnel.value.inNodePortSta &&
    selectedTunnel.value.inNodePortEnd &&
    form.inPort
  ) {
    if (form.inPort < selectedTunnel.value.inNodePortSta || form.inPort > selectedTunnel.value.inNodePortEnd) {
      errors.inPort = `端口号必须在 ${selectedTunnel.value.inNodePortSta}-${selectedTunnel.value.inNodePortEnd} 范围内`
    }
  }

  return !errors.name && !errors.tunnelId && !errors.remoteAddr && !errors.inPort
}

function resetFormState() {
  Object.assign(form, {
    id: null,
    userId: null,
    name: '',
    tunnelId: null,
    inPort: null,
    remoteAddr: '',
    interfaceName: '',
    strategy: 'fifo'
  })
  portInput.value = ''
  selectedTunnel.value = null
  errors.name = ''
  errors.tunnelId = ''
  errors.remoteAddr = ''
  errors.inPort = ''
}

function openCreateModal() {
  isEdit.value = false
  resetFormState()
  modalOpen.value = true
}

function openEditModal(forward) {
  isEdit.value = true
  Object.assign(form, {
    id: forward.id,
    userId: forward.userId,
    name: forward.name,
    tunnelId: forward.tunnelId,
    inPort: forward.inPort,
    remoteAddr: String(forward.remoteAddr || '').split(',').join('\n'),
    interfaceName: forward.interfaceName || '',
    strategy: forward.strategy || 'fifo'
  })
  portInput.value = forward.inPort ? String(forward.inPort) : ''
  selectedTunnel.value = tunnels.value.find(item => Number(item.id) === Number(forward.tunnelId)) || null
  errors.name = ''
  errors.tunnelId = ''
  errors.remoteAddr = ''
  errors.inPort = ''
  modalOpen.value = true
}

function closeEditorModal() {
  modalOpen.value = false
}

async function handleSubmit() {
  if (!validateForm()) {
    return
  }

  submitLoading.value = true
  try {
    const processedRemoteAddr = splitLines(form.remoteAddr).join(',')
    const addressCount = processedRemoteAddr.split(',').map(item => item.trim()).filter(Boolean).length
    const payload = {
      name: form.name,
      tunnelId: form.tunnelId,
      inPort: form.inPort,
      remoteAddr: processedRemoteAddr,
      interfaceName: form.interfaceName,
      strategy: addressCount > 1 ? form.strategy : 'fifo'
    }

    const response = isEdit.value
      ? await updateForward({
          id: form.id,
          userId: form.userId,
          ...payload
        })
      : await createForward(payload)

    if (response.code === 0) {
      modalOpen.value = false
      setFeedback('success', isEdit.value ? '修改成功' : '创建成功')
      await loadData(true)
    } else {
      setFeedback('error', response.msg || '操作失败')
    }
  } catch (error) {
    console.error('提交转发失败:', error)
    setFeedback('error', '操作失败')
  } finally {
    submitLoading.value = false
  }
}

async function handleToggleService(forward) {
  if (Number(forward.status) !== 1 && Number(forward.status) !== 0) {
    setFeedback('error', '转发状态异常，无法操作')
    return
  }

  const targetState = !forward.serviceRunning
  forwards.value = forwards.value.map(item =>
    item.id === forward.id ? { ...item, serviceRunning: targetState } : item
  )

  try {
    const response = targetState ? await resumeForwardService(forward.id) : await pauseForwardService(forward.id)

    if (response.code === 0) {
      forwards.value = forwards.value.map(item =>
        item.id === forward.id
          ? { ...item, serviceRunning: targetState, status: targetState ? 1 : 0 }
          : item
      )
      setFeedback('success', targetState ? '服务已启动' : '服务已暂停')
      return
    }

    forwards.value = forwards.value.map(item =>
      item.id === forward.id ? { ...item, serviceRunning: !targetState } : item
    )
    setFeedback('error', response.msg || '操作失败')
  } catch (error) {
    console.error('切换转发服务失败:', error)
    forwards.value = forwards.value.map(item =>
      item.id === forward.id ? { ...item, serviceRunning: !targetState } : item
    )
    setFeedback('error', '网络错误，操作失败')
  }
}

function openDeleteModal(forward) {
  forwardToDelete.value = forward
  deleteModalOpen.value = true
}

async function confirmDelete() {
  if (!forwardToDelete.value) {
    return
  }

  deleteLoading.value = true
  try {
    const response = await deleteForward(forwardToDelete.value.id)
    if (response.code === 0) {
      deleteModalOpen.value = false
      setFeedback('success', '删除成功')
      await loadData(true)
      return
    }

    const shouldForceDelete = window.confirm(
      `常规删除失败：${response.msg || '删除失败'}\n\n是否需要强制删除？\n\n⚠️ 注意：强制删除不会去验证节点端是否已经删除对应的转发服务。`
    )

    if (!shouldForceDelete) {
      return
    }

    const forceResponse = await forceDeleteForward(forwardToDelete.value.id)
    if (forceResponse.code === 0) {
      deleteModalOpen.value = false
      setFeedback('success', '强制删除成功')
      await loadData(true)
    } else {
      setFeedback('error', forceResponse.msg || '强制删除失败')
    }
  } catch (error) {
    console.error('删除转发失败:', error)
    setFeedback('error', '删除失败')
  } finally {
    deleteLoading.value = false
  }
}

function buildDiagnosisFallback(forward, title, message) {
  return {
    forwardName: forward.name,
    timestamp: Date.now(),
    results: [
      {
        success: false,
        description: title,
        nodeName: '-',
        nodeId: '-',
        targetIp: String(forward.remoteAddr || '').split(',')[0] || '-',
        message
      }
    ]
  }
}

async function openDiagnosisModal(forward) {
  currentDiagnosisForward.value = forward
  diagnosisModalOpen.value = true
  diagnosisLoading.value = true
  diagnosisResult.value = null

  try {
    const response = await diagnoseForward(forward.id)
    if (response.code === 0) {
      diagnosisResult.value = response.data
    } else {
      setFeedback('error', response.msg || '诊断失败')
      diagnosisResult.value = buildDiagnosisFallback(forward, '诊断失败', response.msg || '诊断过程中发生错误')
    }
  } catch (error) {
    console.error('诊断转发失败:', error)
    setFeedback('error', '网络错误，诊断失败')
    diagnosisResult.value = buildDiagnosisFallback(forward, '网络错误', '无法连接到服务器')
  } finally {
    diagnosisLoading.value = false
  }
}

function normalizeInboundAddress(ip, port) {
  if (String(ip).includes(':') && !String(ip).startsWith('[')) {
    return `[${ip}]:${port}`
  }
  return `${ip}:${port}`
}

function formatInAddress(ipString, port) {
  if (!ipString || !port) {
    return ''
  }

  const ips = String(ipString)
    .split(',')
    .map(item => item.trim())
    .filter(Boolean)

  if (!ips.length) {
    return ''
  }

  if (ips.length === 1) {
    return normalizeInboundAddress(ips[0], port)
  }

  return `${normalizeInboundAddress(ips[0], port)} (+${ips.length - 1})`
}

function formatRemoteAddress(addressString) {
  const addresses = String(addressString || '')
    .split(',')
    .map(item => item.trim())
    .filter(Boolean)

  if (!addresses.length) {
    return ''
  }

  if (addresses.length === 1) {
    return addresses[0]
  }

  return `${addresses[0]} (+${addresses.length - 1})`
}

function hasMultipleAddresses(addressString) {
  return String(addressString || '')
    .split(',')
    .map(item => item.trim())
    .filter(Boolean).length > 1
}

async function copyToClipboard(text, label = '内容') {
  try {
    await navigator.clipboard.writeText(text)
    setFeedback('success', `${label}已复制`)
  } catch (error) {
    console.error('复制失败:', error)
    setFeedback('error', '复制失败：HTTP 下无法复制，需 HTTPS/反代')
  }
}

function showAddressModal({ value, port = null, title }) {
  if (!value) {
    return
  }

  let addresses = []
  if (port !== null) {
    const ips = String(value)
      .split(',')
      .map(item => item.trim())
      .filter(Boolean)

    if (ips.length <= 1) {
      copyToClipboard(formatInAddress(value, port), title)
      return
    }

    addresses = ips.map(ip => normalizeInboundAddress(ip, port))
  } else {
    addresses = String(value)
      .split(',')
      .map(item => item.trim())
      .filter(Boolean)

    if (addresses.length <= 1) {
      copyToClipboard(addresses[0], title)
      return
    }
  }

  addressList.value = addresses.map((address, index) => ({
    id: index,
    address,
    copying: false
  }))
  addressModalTitle.value = `${title} (${addresses.length})`
  addressModalOpen.value = true
}

async function copyAddress(item) {
  addressList.value = addressList.value.map(entry =>
    entry.id === item.id ? { ...entry, copying: true } : entry
  )
  try {
    await copyToClipboard(item.address, '地址')
  } finally {
    addressList.value = addressList.value.map(entry =>
      entry.id === item.id ? { ...entry, copying: false } : entry
    )
  }
}

async function copyAllAddresses() {
  if (!addressList.value.length) {
    return
  }
  await copyToClipboard(addressList.value.map(item => item.address).join('\n'), '所有地址')
}

function openExportModal() {
  selectedTunnelForExport.value = null
  exportData.value = ''
  exportModalOpen.value = true
}

function getExportSource() {
  if (!selectedTunnelForExport.value) {
    return []
  }

  if (viewMode.value === 'grouped') {
    return groupedForwards.value.flatMap(userGroup =>
      userGroup.tunnelGroups
        .filter(tunnelGroup => Number(tunnelGroup.tunnelId) === Number(selectedTunnelForExport.value))
        .flatMap(tunnelGroup => tunnelGroup.forwards)
    )
  }

  return getSortedForwards('direct').filter(forward => Number(forward.tunnelId) === Number(selectedTunnelForExport.value))
}

async function executeExport() {
  if (!selectedTunnelForExport.value) {
    setFeedback('error', '请选择要导出的隧道')
    return
  }

  exportLoading.value = true
  try {
    const items = getExportSource()
    if (!items.length) {
      setFeedback('error', '所选隧道没有转发数据')
      return
    }

    exportData.value = items.map(item => `${item.remoteAddr}|${item.name}|${item.inPort}`).join('\n')
  } catch (error) {
    console.error('导出转发失败:', error)
    setFeedback('error', '导出失败')
  } finally {
    exportLoading.value = false
  }
}

async function copyExportData() {
  await copyToClipboard(exportData.value, '转发数据')
}

function openImportModal() {
  importData.value = ''
  importResults.value = []
  selectedTunnelForImport.value = null
  importModalOpen.value = true
}

function appendImportResult(result) {
  importResults.value = [result, ...importResults.value]
}

async function executeImport() {
  if (!importData.value.trim()) {
    setFeedback('error', '请输入要导入的数据')
    return
  }

  if (!selectedTunnelForImport.value) {
    setFeedback('error', '请选择要导入的隧道')
    return
  }

  importLoading.value = true
  importResults.value = []

  try {
    const lines = importData.value
      .trim()
      .split('\n')
      .map(item => item.trim())
      .filter(Boolean)

    for (const line of lines) {
      const parts = line.split('|')

      if (parts.length < 2) {
        appendImportResult({
          line,
          success: false,
          message: '格式错误：至少需要包含目标地址和转发名称'
        })
        continue
      }

      const remoteAddr = String(parts[0] || '').trim()
      const name = String(parts[1] || '').trim()
      const inPortRaw = String(parts[2] || '').trim()

      if (!remoteAddr || !name) {
        appendImportResult({
          line,
          success: false,
          message: '目标地址和转发名称不能为空'
        })
        continue
      }

      const addressPattern = /^[^:]+:\d+$/
      const isValidRemoteAddr = remoteAddr
        .split(',')
        .map(item => item.trim())
        .every(item => addressPattern.test(item))

      if (!isValidRemoteAddr) {
        appendImportResult({
          line,
          success: false,
          message: '目标地址格式错误，应为 host:port，多个地址用逗号分隔'
        })
        continue
      }

      let portNumber = null
      if (inPortRaw) {
        const parsedPort = Number(inPortRaw)
        if (!Number.isFinite(parsedPort) || parsedPort < 1 || parsedPort > 65535) {
          appendImportResult({
            line,
            success: false,
            message: '入口端口格式错误，应为 1-65535 之间的数字'
          })
          continue
        }
        portNumber = parsedPort
      }

      try {
        const response = await createForward({
          name,
          tunnelId: selectedTunnelForImport.value,
          inPort: portNumber,
          remoteAddr,
          strategy: 'fifo'
        })

        if (response.code === 0) {
          appendImportResult({
            line,
            success: true,
            message: '创建成功',
            forwardName: name
          })
        } else {
          appendImportResult({
            line,
            success: false,
            message: response.msg || '创建失败'
          })
        }
      } catch (error) {
        console.error('导入创建失败:', error)
        appendImportResult({
          line,
          success: false,
          message: '网络错误，创建失败'
        })
      }
    }

    setFeedback('success', '导入执行完成')
    await loadData(false)
  } catch (error) {
    console.error('导入转发失败:', error)
    setFeedback('error', '导入过程中发生错误')
  } finally {
    importLoading.value = false
  }
}

function getStatusMeta(status) {
  switch (Number(status)) {
    case 1:
      return { text: '正常', className: 'tag-success' }
    case 0:
      return { text: '暂停', className: 'tag-warning' }
    case -1:
      return { text: '异常', className: 'tag-danger' }
    default:
      return { text: '未知', className: 'tag-muted' }
  }
}

function getStrategyMeta(strategy) {
  switch (strategy) {
    case 'fifo':
      return { text: '主备', className: 'tag-primary' }
    case 'round':
      return { text: '轮询', className: 'tag-success' }
    case 'rand':
      return { text: '随机', className: 'tag-warning' }
    default:
      return { text: '未知', className: 'tag-muted' }
  }
}

function getQualityMeta(averageTime, packetLoss) {
  if (averageTime == null || packetLoss == null) {
    return { text: '未知' }
  }
  if (averageTime < 30 && packetLoss === 0) return { text: '优秀' }
  if (averageTime < 50 && packetLoss === 0) return { text: '很好' }
  if (averageTime < 100 && packetLoss < 1) return { text: '良好' }
  if (averageTime < 150 && packetLoss < 2) return { text: '一般' }
  if (averageTime < 200 && packetLoss < 5) return { text: '较差' }
  return { text: '很差' }
}

function formatFlow(value) {
  const size = Number(value || 0)
  if (size === 0) return '0 B'
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(2)} KB`
  if (size < 1024 * 1024 * 1024) return `${(size / (1024 * 1024)).toFixed(2)} MB`
  return `${(size / (1024 * 1024 * 1024)).toFixed(2)} GB`
}

function arrayMove(list, fromIndex, toIndex) {
  const next = [...list]
  const [item] = next.splice(fromIndex, 1)
  next.splice(toIndex, 0, item)
  return next
}

function onDragStart(event, id) {
  if (viewMode.value !== 'direct') {
    return
  }
  draggingId.value = id
  dragOverId.value = id
  event.dataTransfer.effectAllowed = 'move'
  event.dataTransfer.setData('text/plain', String(id))
}

function onDragEnter(id) {
  if (draggingId.value == null || viewMode.value !== 'direct') {
    return
  }
  dragOverId.value = id
}

async function reorderDirectForwards(activeId, overId) {
  const orderedIds = sortedDirectForwards.value.map(item => item.id)
  const oldIndex = orderedIds.indexOf(activeId)
  const newIndex = orderedIds.indexOf(overId)

  if (oldIndex === -1 || newIndex === -1 || oldIndex === newIndex) {
    return
  }

  const newOrder = arrayMove(orderedIds, oldIndex, newIndex)
  forwardOrder.value = newOrder
  saveOrder(newOrder)

  const inxMap = new Map(newOrder.map((id, index) => [id, index]))
  forwards.value = forwards.value.map(item =>
    inxMap.has(item.id) ? { ...item, inx: inxMap.get(item.id) } : item
  )

  try {
    const response = await updateForwardOrder({
      forwards: newOrder.map((id, index) => ({
        id,
        inx: index
      }))
    })

    if (response.code !== 0) {
      setFeedback('error', `保存排序失败：${response.msg || '未知错误'}`)
    }
  } catch (error) {
    console.error('保存转发排序失败:', error)
    setFeedback('error', '保存排序失败，请重试')
  }
}

async function onDrop(id) {
  if (draggingId.value == null || viewMode.value !== 'direct') {
    onDragEnd()
    return
  }

  const activeId = draggingId.value
  onDragEnd()
  if (activeId === id) {
    return
  }
  await reorderDirectForwards(activeId, id)
}

function onDragEnd() {
  draggingId.value = null
  dragOverId.value = null
}
</script>

<style scoped>
.forward-page {
  --forward-accent: #2563eb;
  --forward-accent-soft: rgba(37, 99, 235, 0.12);
  --forward-success-soft: rgba(16, 185, 129, 0.14);
  --forward-warning-soft: rgba(245, 158, 11, 0.14);
  --forward-danger-soft: rgba(239, 68, 68, 0.14);
  --forward-muted-soft: rgba(148, 163, 184, 0.16);
  display: flex;
  flex-direction: column;
  gap: 20px;
  min-height: calc(100vh - 180px);
}

.toolbar {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 16px;
  padding: 22px 24px;
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  background:
    radial-gradient(circle at top right, rgba(37, 99, 235, 0.14), transparent 32%),
    linear-gradient(180deg, rgba(15, 23, 42, 0.03), transparent 60%),
    var(--surface-color);
}

.toolbar-copy h2 {
  margin: 4px 0 0;
  font-size: 26px;
  line-height: 1.1;
}

.eyebrow {
  margin: 0;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.18em;
  text-transform: uppercase;
  color: var(--text-secondary);
}

.toolbar-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}

.btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  border: 1px solid transparent;
  border-radius: var(--radius-md);
  padding: 10px 16px;
  font-size: 14px;
  font-weight: 600;
  cursor: pointer;
  transition: var(--transition);
}

.btn:hover {
  transform: translateY(-1px);
}

.btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
  transform: none;
}

.btn-primary {
  background: var(--forward-accent);
  color: #fff;
  box-shadow: 0 14px 32px rgba(37, 99, 235, 0.2);
}

.btn-primary:hover {
  background: #1d4ed8;
}

.btn-secondary {
  background: var(--surface-color);
  color: var(--text-color);
  border-color: var(--border-color);
}

.btn-secondary:hover {
  border-color: rgba(37, 99, 235, 0.35);
  background: rgba(37, 99, 235, 0.05);
}

.btn-sm {
  padding: 8px 12px;
  font-size: 12px;
}

.danger {
  background: #dc2626;
}

.danger:hover {
  background: #b91c1c;
}

.danger-text {
  color: #dc2626;
}

.icon-button {
  min-width: 92px;
}

.icon-mark {
  width: 22px;
  height: 22px;
  border-radius: 999px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: rgba(37, 99, 235, 0.1);
  color: var(--forward-accent);
  font-size: 12px;
  font-weight: 700;
}

.feedback {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 14px 16px;
  border-radius: var(--radius-md);
  border: 1px solid transparent;
}

.feedback-success {
  background: var(--forward-success-soft);
  color: #047857;
  border-color: rgba(16, 185, 129, 0.24);
}

.feedback-error {
  background: var(--forward-danger-soft);
  color: #b91c1c;
  border-color: rgba(239, 68, 68, 0.25);
}

.feedback-warning {
  background: var(--forward-warning-soft);
  color: #b45309;
  border-color: rgba(245, 158, 11, 0.28);
}

.feedback-info {
  background: var(--forward-accent-soft);
  color: #1d4ed8;
  border-color: rgba(37, 99, 235, 0.24);
}

.feedback-close,
.modal-close {
  background: transparent;
  border: none;
  color: inherit;
  font-size: 22px;
  cursor: pointer;
}

.loading-state {
  min-height: 300px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  border: 1px dashed rgba(37, 99, 235, 0.2);
  border-radius: var(--radius-lg);
  background:
    linear-gradient(180deg, rgba(37, 99, 235, 0.04), transparent 55%),
    var(--surface-color);
  color: var(--text-secondary);
}

.loading-state.compact {
  min-height: 180px;
}

.spinner {
  width: 22px;
  height: 22px;
  border-radius: 999px;
  border: 3px solid rgba(37, 99, 235, 0.2);
  border-top-color: var(--forward-accent);
  animation: spin 0.8s linear infinite;
}

.grouped-stack,
.direct-stack {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.user-group,
.empty-state {
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  background: var(--surface-color);
  overflow: hidden;
}

.user-group-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding: 22px 24px;
  border-bottom: 1px solid var(--border-color);
  background: linear-gradient(180deg, rgba(37, 99, 235, 0.06), transparent 85%);
}

.user-group-head h3,
.empty-state h3,
.modal-header h3 {
  margin: 4px 0 0;
}

.group-summary,
.modal-subtitle,
.hint {
  margin: 6px 0 0;
  color: var(--text-secondary);
  font-size: 13px;
}

.muted {
  opacity: 0.8;
}

.accordion {
  border-top: 1px solid var(--border-color);
}

.accordion:first-of-type {
  border-top: none;
}

.accordion summary {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  list-style: none;
  cursor: pointer;
  padding: 18px 24px;
  background: rgba(15, 23, 42, 0.03);
}

.accordion summary::-webkit-details-marker {
  display: none;
}

.accordion-title {
  display: block;
  font-weight: 700;
}

.accordion-meta {
  display: block;
  margin-top: 4px;
  font-size: 12px;
  color: var(--text-secondary);
}

.card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 16px;
  padding: 18px;
}

.forward-card {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 18px;
  border-radius: 20px;
  border: 1px solid var(--border-color);
  background:
    radial-gradient(circle at top right, rgba(37, 99, 235, 0.08), transparent 28%),
    linear-gradient(180deg, rgba(15, 23, 42, 0.04), transparent 60%),
    var(--surface-color);
  box-shadow: 0 14px 30px rgba(15, 23, 42, 0.08);
  transition: transform 0.2s ease, border-color 0.2s ease, box-shadow 0.2s ease;
}

.forward-card:hover {
  transform: translateY(-3px);
  border-color: rgba(37, 99, 235, 0.24);
  box-shadow: 0 20px 38px rgba(15, 23, 42, 0.12);
}

.forward-card.dragging {
  opacity: 0.55;
}

.forward-card.dragover {
  border-color: var(--forward-accent);
}

.card-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.card-title {
  min-width: 0;
}

.card-title h4 {
  margin: 0;
  font-size: 15px;
}

.card-title p {
  margin: 6px 0 0;
  color: var(--text-secondary);
  font-size: 12px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.card-head-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.drag-handle {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  border-radius: 10px;
  color: var(--text-secondary);
  background: rgba(15, 23, 42, 0.04);
  cursor: grab;
  opacity: 0;
  transition: opacity 0.2s ease, color 0.2s ease, background 0.2s ease;
}

.forward-card:hover .drag-handle,
.drag-handle.visible {
  opacity: 1;
}

.drag-handle:hover {
  color: var(--forward-accent);
  background: rgba(37, 99, 235, 0.08);
}

.endpoint {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 14px;
  border-radius: 14px;
  border: 1px solid var(--border-color);
  background: rgba(15, 23, 42, 0.03);
  color: var(--text-color);
  cursor: pointer;
  transition: border-color 0.2s ease, background 0.2s ease;
}

.endpoint:hover {
  border-color: rgba(37, 99, 235, 0.28);
  background: rgba(37, 99, 235, 0.05);
}

.endpoint span {
  font-size: 12px;
  font-weight: 700;
  color: var(--text-secondary);
}

.endpoint code,
.list-item code,
.result-item code,
.diagnosis-metric code {
  font-family: Consolas, 'Courier New', monospace;
  font-size: 12px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.card-stats,
.card-actions,
.modal-toolbar,
.result-head {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.card-actions {
  padding-top: 4px;
}

.tag {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 28px;
  padding: 6px 10px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 700;
  background: var(--forward-muted-soft);
  color: var(--text-secondary);
}

.tag-primary {
  background: var(--forward-accent-soft);
  color: #1d4ed8;
}

.tag-success {
  background: var(--forward-success-soft);
  color: #047857;
}

.tag-warning {
  background: var(--forward-warning-soft);
  color: #b45309;
}

.tag-danger {
  background: var(--forward-danger-soft);
  color: #b91c1c;
}

.tag-muted {
  background: var(--forward-muted-soft);
  color: var(--text-secondary);
}

.empty-state {
  padding: 42px 24px;
  text-align: center;
}

.empty-state.compact {
  padding: 26px 20px;
}

.modal-overlay {
  position: fixed;
  inset: 0;
  z-index: 1000;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: rgba(15, 23, 42, 0.7);
  backdrop-filter: blur(6px);
}

.modal {
  width: min(560px, 100%);
  max-height: calc(100vh - 48px);
  display: flex;
  flex-direction: column;
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: 24px;
  overflow: hidden;
  box-shadow: 0 28px 70px rgba(15, 23, 42, 0.28);
}

.modal-lg {
  width: min(760px, 100%);
}

.modal-xl {
  width: min(900px, 100%);
}

.modal-header,
.modal-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 18px 22px;
  border-bottom: 1px solid var(--border-color);
}

.modal-footer {
  justify-content: flex-end;
  border-top: 1px solid var(--border-color);
  border-bottom: none;
}

.modal-body {
  padding: 22px;
  overflow: auto;
}

.modal-copy {
  margin: 0;
  line-height: 1.7;
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}

.form-group {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 18px;
}

.form-group label {
  font-size: 13px;
  font-weight: 700;
}

.form-group input,
.form-group select,
.form-group textarea,
.mono-area {
  width: 100%;
  padding: 12px 14px;
  border: 1px solid var(--border-color);
  border-radius: 14px;
  background: rgba(15, 23, 42, 0.04);
  color: var(--text-color);
  font-size: 14px;
  transition: border-color 0.2s ease, box-shadow 0.2s ease, background 0.2s ease;
}

.form-group input:focus,
.form-group select:focus,
.form-group textarea:focus,
.mono-area:focus {
  outline: none;
  border-color: rgba(37, 99, 235, 0.4);
  box-shadow: 0 0 0 4px rgba(37, 99, 235, 0.12);
  background: rgba(37, 99, 235, 0.03);
}

.form-group textarea,
.mono-area {
  resize: vertical;
  font-family: Consolas, 'Courier New', monospace;
}

.form-error {
  margin: 0;
  font-size: 12px;
  color: #dc2626;
}

.align-end {
  justify-content: flex-end;
}

.list-stack,
.result-list,
.diagnosis-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.list-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 14px;
  border-radius: 14px;
  border: 1px solid var(--border-color);
  background: rgba(15, 23, 42, 0.04);
}

.result-panel {
  border-top: 1px solid var(--border-color);
  padding-top: 18px;
}

.result-head {
  justify-content: space-between;
  margin-bottom: 12px;
}

.result-head h4 {
  margin: 0;
}

.result-item {
  padding: 12px 14px;
  border-radius: 16px;
  border: 1px solid transparent;
}

.result-status {
  margin-bottom: 8px;
  font-size: 12px;
  font-weight: 700;
}

.result-item p {
  margin: 8px 0 0;
  font-size: 13px;
}

.result-success {
  background: var(--forward-success-soft);
  border-color: rgba(16, 185, 129, 0.22);
}

.result-success .result-status {
  color: #047857;
}

.result-failed {
  background: var(--forward-danger-soft);
  border-color: rgba(239, 68, 68, 0.2);
}

.result-failed .result-status {
  color: #b91c1c;
}

.diagnosis-card {
  padding: 18px;
  border-radius: 18px;
  border: 1px solid var(--border-color);
  background:
    linear-gradient(180deg, rgba(37, 99, 235, 0.04), transparent 55%),
    var(--surface-color);
}

.diagnosis-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.diagnosis-head h4 {
  margin: 0;
}

.diagnosis-head p {
  margin: 6px 0 0;
  color: var(--text-secondary);
  font-size: 13px;
}

.diagnosis-body {
  margin-top: 14px;
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.diagnosis-metric {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 14px;
  border-radius: 14px;
  background: rgba(15, 23, 42, 0.04);
}

.metric-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}

.metric-card {
  padding: 14px;
  border-radius: 16px;
  border: 1px solid var(--border-color);
  background: rgba(15, 23, 42, 0.04);
}

.metric-card span {
  display: block;
  font-size: 12px;
  color: var(--text-secondary);
}

.metric-card strong {
  display: block;
  margin-top: 8px;
  font-size: 18px;
}

.diagnosis-error {
  margin: 0;
  padding: 12px 14px;
  border-radius: 14px;
  background: var(--forward-danger-soft);
  color: #b91c1c;
  line-height: 1.6;
}

.switch {
  position: relative;
  display: inline-flex;
  width: 42px;
  height: 24px;
}

.switch input {
  opacity: 0;
  width: 0;
  height: 0;
}

.switch-slider {
  position: absolute;
  inset: 0;
  border-radius: 999px;
  background: rgba(148, 163, 184, 0.45);
  transition: 0.2s ease;
}

.switch-slider::before {
  content: '';
  position: absolute;
  width: 18px;
  height: 18px;
  left: 3px;
  top: 3px;
  border-radius: 50%;
  background: #fff;
  transition: 0.2s ease;
  box-shadow: 0 2px 8px rgba(15, 23, 42, 0.18);
}

.switch input:checked + .switch-slider {
  background: var(--forward-accent);
}

.switch input:checked + .switch-slider::before {
  transform: translateX(18px);
}

.switch input:disabled + .switch-slider {
  opacity: 0.5;
  cursor: not-allowed;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

@media (max-width: 900px) {
  .toolbar {
    flex-direction: column;
    align-items: stretch;
  }

  .toolbar-actions {
    justify-content: stretch;
  }

  .toolbar-actions .btn {
    flex: 1 1 160px;
  }
}

@media (max-width: 768px) {
  .card-grid,
  .form-grid,
  .metric-grid {
    grid-template-columns: 1fr;
  }

  .modal-overlay {
    padding: 12px;
  }

  .modal {
    max-height: calc(100vh - 24px);
  }

  .modal-header,
  .modal-body,
  .modal-footer,
  .toolbar,
  .user-group-head,
  .accordion summary {
    padding-left: 16px;
    padding-right: 16px;
  }

  .endpoint,
  .diagnosis-metric,
  .list-item {
    flex-direction: column;
    align-items: flex-start;
  }

  .card-head {
    align-items: flex-start;
  }

  .card-head-actions {
    align-items: center;
    flex-wrap: wrap;
    justify-content: flex-end;
  }

  .drag-handle {
    opacity: 1;
  }
}
</style>
