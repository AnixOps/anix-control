<template>
  <div class="ansible-machines-page">
    <section class="hero-card">
      <div>
        <p class="eyebrow">Execution Fleet</p>
        <h2>Ansible Machines</h2>
        <p class="hero-text">
          This page is only for stateless Ansible execution machines. These hosts do not need Node-Agent and do not
          need a persistent control-plane connection.
        </p>
      </div>
      <div class="hero-actions">
        <router-link class="btn btn-secondary" to="/admin/forward/local">Local Runtime</router-link>
        <router-link class="btn btn-secondary" to="/admin/forward/nodes">NodeX Topology</router-link>
        <router-link class="btn btn-secondary" to="/admin/forward/agents">NodeX Agents</router-link>
        <button class="btn btn-secondary" :disabled="loading" @click="refreshAll">{{ loading ? 'Refreshing...' : 'Refresh' }}</button>
        <button class="btn btn-primary" @click="openEditor()">Add Machine</button>
      </div>
    </section>

    <ForwardSuiteNav />

    <section class="stats-grid">
      <article class="stat-card">
        <p class="stat-label">Machines</p>
        <strong class="stat-value">{{ machines.length }}</strong>
      </article>
      <article class="stat-card">
        <p class="stat-label">Online</p>
        <strong class="stat-value">{{ onlineCount }}</strong>
      </article>
      <article class="stat-card">
        <p class="stat-label">Enabled</p>
        <strong class="stat-value">{{ enabledCount }}</strong>
      </article>
    </section>

    <section class="panel-card">
      <div class="section-head">
        <div>
          <p class="eyebrow">Machines</p>
          <h3>Relay Execution Nodes</h3>
          <p class="section-copy">These records are used by the local Ansible runtime as execution-node identity only.</p>
        </div>
        <label class="filter-group">
          <span>Status</span>
          <select v-model="statusFilter" @change="refreshAll">
            <option value="all">All</option>
            <option value="1">Online</option>
            <option value="0">Offline</option>
          </select>
        </label>
      </div>

      <div v-if="loading" class="state-card">Loading Ansible machines...</div>
      <div v-else-if="!machines.length" class="state-card">No Ansible execution machines yet.</div>
      <div v-else class="machine-grid">
        <article v-for="machine in machines" :key="machine.id" class="machine-card">
          <div class="machine-head">
            <div>
              <p class="eyebrow">Machine #{{ machine.id }}</p>
              <h4>{{ machine.name }}</h4>
              <p class="machine-meta">{{ machine.host }}:{{ machine.port }}</p>
            </div>
            <div class="status-stack">
              <span :class="['tag', machine.enabled ? 'tag-success' : 'tag-muted']">{{ machine.enabled ? 'Enabled' : 'Disabled' }}</span>
              <span :class="['tag', machine.status === 1 ? 'tag-success' : 'tag-danger']">{{ machine.status === 1 ? 'Online' : 'Offline' }}</span>
            </div>
          </div>

          <div class="meta-grid">
            <div class="meta-item">
              <span class="meta-label">API Port</span>
              <strong>{{ machine.apiPort || '-' }}</strong>
            </div>
            <div class="meta-item">
              <span class="meta-label">Region / ISP</span>
              <strong>{{ machine.region || '-' }} / {{ machine.isp || '-' }}</strong>
            </div>
            <div class="meta-item">
              <span class="meta-label">Current Conn</span>
              <strong>{{ machine.currentConn }}</strong>
            </div>
            <div class="meta-item">
              <span class="meta-label">Traffic</span>
              <strong>{{ formatBytes(machine.totalUpload) }} / {{ formatBytes(machine.totalDownload) }}</strong>
            </div>
          </div>

          <p v-if="results[machine.id]" :class="['result-text', results[machine.id].success ? 'ok' : 'fail']">
            {{ results[machine.id].message }}
          </p>

          <div class="card-actions">
            <button class="btn btn-secondary btn-sm" @click="openEditor(machine)">Edit</button>
            <button class="btn btn-secondary btn-sm" :disabled="pendingAction === `${machine.id}:check`" @click="checkMachine(machine)">
              {{ pendingAction === `${machine.id}:check` ? 'Checking...' : 'Health Check' }}
            </button>
            <button class="btn btn-secondary btn-sm" :disabled="pendingAction === `${machine.id}:sync`" @click="syncMachine(machine)">
              {{ pendingAction === `${machine.id}:sync` ? 'Syncing...' : 'Sync Stats' }}
            </button>
            <button class="btn btn-secondary btn-sm" :disabled="pendingAction === `${machine.id}:toggle`" @click="toggleMachine(machine)">
              {{ machine.enabled ? 'Disable' : 'Enable' }}
            </button>
            <button class="btn btn-secondary btn-sm danger-text" @click="openDelete(machine)">Delete</button>
          </div>
        </article>
      </div>
    </section>

    <div v-if="editorOpen" class="modal-overlay" @click.self="closeEditor">
      <div class="modal">
        <div class="modal-header">
          <div>
            <p class="eyebrow">Machine</p>
            <h3>{{ editorMode ? 'Edit Ansible Machine' : 'Add Ansible Machine' }}</h3>
          </div>
          <button class="modal-close" @click="closeEditor">×</button>
        </div>
        <div class="modal-body">
          <div class="form-grid">
            <label class="form-group">
              <span>Name</span>
              <input v-model.trim="form.name" type="text" placeholder="relay-exec-01" />
            </label>
            <label class="form-group">
              <span>Host</span>
              <input v-model.trim="form.host" type="text" placeholder="1.2.3.4" />
            </label>
          </div>
          <div class="form-grid">
            <label class="form-group">
              <span>Service Port</span>
              <input v-model.trim="form.port" type="number" min="1" max="65535" />
            </label>
            <label class="form-group">
              <span>API Port</span>
              <input v-model.trim="form.apiPort" type="number" min="1" max="65535" />
            </label>
          </div>
          <div class="form-grid">
            <label class="form-group">
              <span>API Token</span>
              <input v-model.trim="form.apiToken" type="text" placeholder="optional" />
            </label>
            <label class="form-group">
              <span>Weight</span>
              <input v-model.trim="form.weight" type="number" min="1" />
            </label>
          </div>
          <div class="form-grid">
            <label class="form-group">
              <span>Region</span>
              <input v-model.trim="form.region" type="text" placeholder="HK / JP / US" />
            </label>
            <label class="form-group">
              <span>ISP</span>
              <input v-model.trim="form.isp" type="text" placeholder="CMI / NTT / Cogent" />
            </label>
          </div>
          <p v-if="formError" class="form-error">{{ formError }}</p>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary" @click="closeEditor">Cancel</button>
          <button class="btn btn-primary" :disabled="saving" @click="submitForm">{{ saving ? 'Saving...' : 'Save' }}</button>
        </div>
      </div>
    </div>

    <div v-if="deleteTarget" class="modal-overlay" @click.self="deleteTarget = null">
      <div class="modal modal-sm">
        <div class="modal-header">
          <div>
            <p class="eyebrow">Delete</p>
            <h3>Delete Machine</h3>
          </div>
          <button class="modal-close" @click="deleteTarget = null">×</button>
        </div>
        <div class="modal-body">
          <p>Delete <strong>{{ deleteTarget?.name }}</strong> from the Ansible execution fleet?</p>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary" @click="deleteTarget = null">Cancel</button>
          <button class="btn btn-primary" :disabled="saving" @click="confirmDelete">{{ saving ? 'Deleting...' : 'Delete' }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import {
  checkForwardNode,
  createForwardNode,
  deleteForwardNode,
  getForwardNode,
  getForwardNodes,
  syncForwardNodeStats,
  toggleForwardNode,
  updateForwardNode
} from '@/api/admin'
import ForwardSuiteNav from '@/components/admin/ForwardSuiteNav.vue'

const loading = ref(false)
const saving = ref(false)
const statusFilter = ref('all')
const machines = ref([])
const results = reactive({})
const pendingAction = ref('')
const editorOpen = ref(false)
const editorMode = ref(false)
const deleteTarget = ref(null)
const formError = ref('')
const form = reactive(createForm())

const onlineCount = computed(() => machines.value.filter(item => item.status === 1).length)
const enabledCount = computed(() => machines.value.filter(item => item.enabled).length)

function createForm() {
  return { id: null, name: '', host: '', port: '', apiPort: '', apiToken: '', region: '', isp: '', weight: '1' }
}

function resetForm() {
  Object.assign(form, createForm())
  formError.value = ''
}

function unwrapResponse(response) {
  return response?.data?.data ?? response?.data ?? response
}

function normalizeMachine(node) {
  return {
    id: Number(node?.id || 0),
    name: node?.name || '-',
    host: node?.host || '-',
    port: Number(node?.port || 0),
    apiPort: Number(node?.api_port || node?.apiPort || 0),
    region: node?.region || '',
    isp: node?.isp || '',
    weight: Number(node?.weight || 1),
    enabled: Boolean(node?.enabled ?? true),
    status: Number(node?.status ?? 0),
    currentConn: Number(node?.current_conn || node?.currentConn || 0),
    totalUpload: Number(node?.total_upload || node?.totalUpload || 0),
    totalDownload: Number(node?.total_download || node?.totalDownload || 0),
    apiToken: node?.api_token || node?.apiToken || ''
  }
}

function formatBytes(value) {
  const size = Number(value || 0)
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(2)} KB`
  if (size < 1024 * 1024 * 1024) return `${(size / (1024 * 1024)).toFixed(2)} MB`
  return `${(size / (1024 * 1024 * 1024)).toFixed(2)} GB`
}

async function refreshAll() {
  loading.value = true
  try {
    const params = { page: 1, page_size: 200, type: 'relay' }
    if (statusFilter.value !== 'all') params.status = Number(statusFilter.value)
    const payload = unwrapResponse(await getForwardNodes(params))
    const list = Array.isArray(payload?.list) ? payload.list : Array.isArray(payload) ? payload : []
    machines.value = list.map(normalizeMachine)
  } catch (error) {
    console.error('load ansible machines failed:', error)
    machines.value = []
  } finally {
    loading.value = false
  }
}

async function openEditor(machine = null) {
  resetForm()
  editorMode.value = Boolean(machine?.id)
  editorOpen.value = true
  if (!machine?.id) return
  const detail = normalizeMachine(unwrapResponse(await getForwardNode(machine.id)))
  Object.assign(form, {
    id: detail.id,
    name: detail.name,
    host: detail.host,
    port: detail.port ? String(detail.port) : '',
    apiPort: detail.apiPort ? String(detail.apiPort) : '',
    apiToken: detail.apiToken || '',
    region: detail.region,
    isp: detail.isp,
    weight: String(detail.weight || 1)
  })
}

function closeEditor() {
  if (saving.value) return
  editorOpen.value = false
  editorMode.value = false
  resetForm()
}

async function submitForm() {
  formError.value = ''
  if (!form.name || !form.host || !Number(form.port)) {
    formError.value = 'Name, host and service port are required.'
    return
  }
  const payload = {
    name: form.name.trim(),
    type: 'relay',
    host: form.host.trim(),
    port: Number(form.port),
    weight: Number(form.weight || 1)
  }
  if (Number(form.apiPort)) payload.api_port = Number(form.apiPort)
  if (form.apiToken.trim()) payload.api_token = form.apiToken.trim()
  if (form.region.trim()) payload.region = form.region.trim()
  if (form.isp.trim()) payload.isp = form.isp.trim()

  saving.value = true
  try {
    if (editorMode.value && form.id) {
      await updateForwardNode(form.id, payload)
    } else {
      await createForwardNode(payload)
    }
    closeEditor()
    await refreshAll()
  } catch (error) {
    formError.value = error.response?.data?.msg || error.message || 'Failed to save machine'
  } finally {
    saving.value = false
  }
}

function openDelete(machine) {
  deleteTarget.value = machine
}

async function confirmDelete() {
  if (!deleteTarget.value?.id) return
  saving.value = true
  try {
    await deleteForwardNode(deleteTarget.value.id)
    deleteTarget.value = null
    await refreshAll()
  } finally {
    saving.value = false
  }
}

async function checkMachine(machine) {
  pendingAction.value = `${machine.id}:check`
  try {
    const payload = unwrapResponse(await checkForwardNode(machine.id))
    const success = Number(payload?.status ?? 0) === 1 && !payload?.error
    results[machine.id] = { success, message: success ? (payload?.latency ? `Latency ${payload.latency} ms` : 'Machine reachable') : (payload?.error || 'Machine unavailable') }
    await refreshAll()
  } finally {
    pendingAction.value = ''
  }
}

async function syncMachine(machine) {
  pendingAction.value = `${machine.id}:sync`
  try {
    const payload = unwrapResponse(await syncForwardNodeStats(machine.id))
    results[machine.id] = { success: true, message: payload?.message || 'Stats synced' }
    await refreshAll()
  } finally {
    pendingAction.value = ''
  }
}

async function toggleMachine(machine) {
  pendingAction.value = `${machine.id}:toggle`
  try {
    await toggleForwardNode(machine.id, !machine.enabled)
    await refreshAll()
  } finally {
    pendingAction.value = ''
  }
}

onMounted(async () => {
  await refreshAll()
})
</script>

<style scoped>
.ansible-machines-page { display: flex; flex-direction: column; gap: 20px; }
.hero-card, .panel-card, .stat-card { border: 1px solid var(--border-color); border-radius: 20px; background: var(--surface-color); }
.hero-card { display: flex; justify-content: space-between; gap: 20px; padding: 24px; background: radial-gradient(circle at top right, rgba(16,185,129,.14), transparent 28%), var(--surface-color); }
.hero-text, .section-copy, .machine-meta, .meta-label { color: var(--text-secondary); line-height: 1.6; }
.hero-actions, .section-head, .status-stack, .card-actions { display: flex; gap: 10px; flex-wrap: wrap; }
.btn { display: inline-flex; align-items: center; justify-content: center; border: 1px solid transparent; border-radius: 12px; padding: 10px 16px; text-decoration: none; cursor: pointer; }
.btn-primary { background: var(--primary-color); color: #fff; }
.btn-secondary { background: var(--surface-color); color: var(--text-color); border-color: var(--border-color); }
.btn-sm { padding: 8px 12px; font-size: 12px; }
.eyebrow { margin: 0; text-transform: uppercase; letter-spacing: .08em; font-size: 12px; color: var(--text-secondary); }
.stats-grid, .machine-grid, .meta-grid, .form-grid { display: grid; gap: 16px; }
.stats-grid { grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); }
.stat-card, .panel-card { padding: 20px; }
.stat-label { margin: 0; color: var(--text-secondary); }
.stat-value { font-size: 28px; font-weight: 700; }
.section-head { justify-content: space-between; align-items: flex-start; margin-bottom: 18px; }
.filter-group, .form-group { display: flex; flex-direction: column; gap: 8px; }
.filter-group select, .form-group input { border: 1px solid var(--border-color); border-radius: 12px; background: var(--bg-color); color: var(--text-color); padding: 12px 14px; }
.machine-grid { grid-template-columns: repeat(auto-fit, minmax(300px, 1fr)); }
.machine-card { border: 1px solid var(--border-color); border-radius: 18px; background: var(--bg-color); padding: 18px; display: flex; flex-direction: column; gap: 14px; }
.machine-head { display: flex; justify-content: space-between; gap: 12px; }
.status-stack { flex-direction: column; align-items: flex-end; }
.meta-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
.meta-item { display: flex; flex-direction: column; gap: 4px; }
.tag { display: inline-flex; align-items: center; justify-content: center; padding: 4px 10px; border-radius: 999px; font-size: 12px; font-weight: 700; }
.tag-success { background: rgba(16,185,129,.14); color: #047857; }
.tag-danger { background: rgba(239,68,68,.14); color: #b91c1c; }
.tag-muted { background: rgba(148,163,184,.16); color: #475569; }
.result-text { margin: 0; font-size: 13px; }
.result-text.ok { color: #047857; }
.result-text.fail, .danger-text, .form-error { color: #b91c1c; }
.state-card { padding: 16px; border: 1px solid var(--border-color); border-radius: 16px; background: var(--bg-color); }
.modal-overlay { position: fixed; inset: 0; display: flex; align-items: center; justify-content: center; padding: 20px; background: rgba(15,23,42,.62); z-index: 1100; }
.modal { width: min(100%, 720px); border-radius: 18px; border: 1px solid var(--border-color); background: var(--surface-color); }
.modal-sm { width: min(100%, 420px); }
.modal-header, .modal-footer { display: flex; justify-content: space-between; gap: 12px; padding: 18px 22px; border-bottom: 1px solid var(--border-color); }
.modal-footer { border-top: 1px solid var(--border-color); border-bottom: 0; justify-content: flex-end; }
.modal-body { padding: 20px 22px; display: flex; flex-direction: column; gap: 14px; }
.modal-close { border: 0; background: transparent; color: var(--text-secondary); cursor: pointer; font-size: 24px; }
.form-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
@media (max-width: 900px) {
  .hero-card, .section-head, .machine-head, .modal-header, .modal-footer { flex-direction: column; }
  .meta-grid, .form-grid { grid-template-columns: 1fr; }
  .status-stack { align-items: flex-start; }
}
</style>
