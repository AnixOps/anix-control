<template>
  <div class="forward-page">
    <div class="toolbar">
      <div class="toolbar-spacer"></div>
      <div class="toolbar-actions">
        <button class="btn-secondary icon-btn" :title="viewMode === 'grouped' ? '切换到平铺视图' : '切换到分组视图'" @click="toggleViewMode">
          <span v-if="viewMode === 'grouped'">≡</span>
          <span v-else>▥</span>
        </button>
        <button class="btn-secondary" @click="openImportModal">导入</button>
        <button class="btn-secondary" @click="openExportModal">导出</button>
        <button @click="openCreateModal">新增</button>
      </div>
    </div>

    <div v-if="feedback.message" :class="['feedback', feedback.type]">
      <span>{{ feedback.message }}</span>
      <button class="feedback-close" @click="clearFeedback">×</button>
    </div>

    <div v-if="loading" class="loading">
      <div class="spinner"></div>
      <span>正在加载...</span>
    </div>

    <template v-else>
      <div v-if="viewMode === 'grouped'" class="group-stack">
        <section v-for="userGroup in groupedForwards" :key="userGroup.userKey" class="panel">
          <div class="panel-head">
            <div>
              <h2>{{ userGroup.userName }}</h2>
              <p>{{ userGroup.tunnelGroups.length }} 个隧道，{{ userGroup.total }} 个转发</p>
            </div>
            <span class="pill pill-primary">用户</span>
          </div>
          <details v-for="tunnelGroup in userGroup.tunnelGroups" :key="tunnelGroup.tunnelId" class="accordion" open>
            <summary>
              <span>{{ tunnelGroup.tunnelName }}</span>
              <span class="pill">{{ tunnelGroup.running }}/{{ tunnelGroup.forwards.length }}</span>
            </summary>
            <div class="grid">
              <article v-for="forward in tunnelGroup.forwards" :key="forward.id" class="card">
                <div class="card-head">
                  <div class="card-title">
                    <h3>{{ forward.name }}</h3>
                    <p>{{ forward.tunnelName }}</p>
                  </div>
                  <div class="card-head-actions">
                    <label class="switch">
                      <input type="checkbox" :checked="Number(forward.status) === 1" @change="handleToggleService(forward)" />
                      <span class="switch-slider"></span>
                    </label>
                    <span :class="['pill', getStatusMeta(forward.status).className]">{{ getStatusMeta(forward.status).text }}</span>
                  </div>
                </div>
                <button class="endpoint" type="button" @click="showAddressModal({ value: forward.inIp, port: forward.inPort, title: '入口端口' })">
                  <span>入口</span>
                  <code>{{ formatInAddress(forward.inIp, forward.inPort) }}</code>
                </button>
                <button class="endpoint" type="button" @click="showAddressModal({ value: forward.remoteAddr, title: '目标地址' })">
                  <span>目标</span>
                  <code>{{ formatRemoteAddress(forward.remoteAddr) }}</code>
                </button>
                <div class="card-stats">
                  <span :class="['pill', getStrategyMeta(forward.strategy).className]">{{ getStrategyMeta(forward.strategy).text }}</span>
                  <span class="pill">↑{{ formatFlow(forward.inFlow || 0) }}</span>
                  <span class="pill pill-success">↓{{ formatFlow(forward.outFlow || 0) }}</span>
                </div>
                <div class="card-actions">
                  <button class="btn-secondary btn-sm" @click="openEditModal(forward)">编辑</button>
                  <button class="btn-secondary btn-sm" @click="openDiagnosisModal(forward)">诊断</button>
                  <button class="btn-secondary btn-sm danger-text" @click="openDeleteModal(forward)">删除</button>
                </div>
              </article>
            </div>
          </details>
        </section>
        <section v-if="!groupedForwards.length" class="empty">暂无转发配置</section>
      </div>

      <div v-else class="grid">
        <article
          v-for="forward in sortedDirectForwards"
          :key="forward.id"
          class="card"
          :class="{ dragging: draggingId === forward.id, dragover: dragOverId === forward.id }"
          draggable="true"
          @dragstart="onDragStart(forward.id)"
          @dragenter.prevent="onDragEnter(forward.id)"
          @dragover.prevent="onDragEnter(forward.id)"
          @drop.prevent="onDrop(forward.id)"
          @dragend="onDragEnd"
        >
          <div class="card-head">
            <div class="card-title">
              <h3>{{ forward.name }}</h3>
              <p>{{ forward.tunnelName }}</p>
            </div>
            <div class="card-head-actions">
              <span class="drag-handle">⋮⋮</span>
              <label class="switch">
                <input type="checkbox" :checked="Number(forward.status) === 1" @change="handleToggleService(forward)" />
                <span class="switch-slider"></span>
              </label>
              <span :class="['pill', getStatusMeta(forward.status).className]">{{ getStatusMeta(forward.status).text }}</span>
            </div>
          </div>
          <button class="endpoint" type="button" @click="showAddressModal({ value: forward.inIp, port: forward.inPort, title: '入口端口' })">
            <span>入口</span>
            <code>{{ formatInAddress(forward.inIp, forward.inPort) }}</code>
          </button>
          <button class="endpoint" type="button" @click="showAddressModal({ value: forward.remoteAddr, title: '目标地址' })">
            <span>目标</span>
            <code>{{ formatRemoteAddress(forward.remoteAddr) }}</code>
          </button>
          <div class="card-stats">
            <span :class="['pill', getStrategyMeta(forward.strategy).className]">{{ getStrategyMeta(forward.strategy).text }}</span>
            <span class="pill">↑{{ formatFlow(forward.inFlow || 0) }}</span>
            <span class="pill pill-success">↓{{ formatFlow(forward.outFlow || 0) }}</span>
          </div>
          <div class="card-actions">
            <button class="btn-secondary btn-sm" @click="openEditModal(forward)">编辑</button>
            <button class="btn-secondary btn-sm" @click="openDiagnosisModal(forward)">诊断</button>
            <button class="btn-secondary btn-sm danger-text" @click="openDeleteModal(forward)">删除</button>
          </div>
        </article>
        <section v-if="!sortedDirectForwards.length" class="empty">暂无转发配置</section>
      </div>
    </template>
  </div>
</template>

<script setup>
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useUserStore } from '@/stores/user'
import { createForward, getForwardList, updateForward, deleteForward, forceDeleteForward, pauseForwardService, resumeForwardService, diagnoseForward, updateForwardOrder, getForwardTunnels } from '@/api/admin'

const userStore = useUserStore()
const loading = ref(true)
const submitting = ref(false)
const deleting = ref(false)
const importing = ref(false)
const diagnosing = ref(false)
const forwards = ref([])
const tunnels = ref([])
const forwardOrder = ref([])
const draggingId = ref(null)
const dragOverId = ref(null)
const viewMode = ref(localStorage.getItem('forward-view-mode') || 'direct')
const showEditor = ref(false)
const showDeleteModal = ref(false)
const showAddressList = ref(false)
const showExportModal = ref(false)
const showImportModal = ref(false)
const showDiagnosisModal = ref(false)
const isEdit = ref(false)
const targetForward = ref(null)
const diagnosisForward = ref(null)
const diagnosisReport = ref(null)
const addressEntries = ref([])
const addressTitle = ref('')
const selectedTunnelForExport = ref('')
const selectedTunnelForImport = ref('')
const exportData = ref('')
const importData = ref('')
const importResults = ref([])
const feedback = reactive({ type: 'success', message: '' })
const form = reactive({ id: null, userId: null, name: '', tunnelId: null, inPort: null, remoteAddr: '', interfaceName: '', strategy: 'fifo' })
const errors = reactive({ name: '', tunnelId: '', remoteAddr: '', inPort: '' })
const portInput = ref('')
let timer = null

const currentUserId = computed(() => Number(userStore.userInfo?.id || userStore.userInfo?.user_id || 0))
const selectedTunnel = computed(() => tunnels.value.find(item => Number(item.id) === Number(form.tunnelId)) || null)
const addressLineCount = computed(() => splitLines(form.remoteAddr).length)
const sortedDirectForwards = computed(() => getSortedForwards('direct'))
const groupedForwards = computed(() => buildGroupedForwards())

watch(currentUserId, () => { if (viewMode.value === 'direct') initializeOrder(forwards.value) })
watch(portInput, value => { form.inPort = value === '' ? null : Number(value) || null })

onMounted(async () => {
  if (!currentUserId.value && userStore.isLoggedIn) await userStore.getUserInfo()
  await loadData(true)
})

onUnmounted(() => { if (timer) clearTimeout(timer) })
</script>

<style scoped>
</style>
