<template>
  <div class="ansible-machines-page">
    <section class="hero-card">
      <div>
        <p class="eyebrow">{{ t('runtime.ansibleMachines.heroEyebrow') }}</p>
        <h2>{{ t('runtime.ansibleMachines.title') }}</h2>
        <p class="hero-text">
          {{ t('runtime.ansibleMachines.heroText') }}
        </p>
      </div>
      <div class="hero-actions">
        <router-link class="btn btn-secondary" to="/admin/forward/local">{{ t('forwardSuite.nav.localRuntime') }}</router-link>
        <router-link class="btn btn-secondary" to="/admin/forward/nodes">{{ t('forwardSuite.nav.nodeXTopology') }}</router-link>
        <router-link class="btn btn-secondary" to="/admin/forward/agents">{{ t('forwardSuite.nav.nodeXAgents') }}</router-link>
        <button class="btn btn-secondary" :disabled="loading" @click="refreshAll">{{ loading ? t('runtime.ansibleMachines.refreshLoading') : t('common.actions.refresh') }}</button>
        <button class="btn btn-primary" @click="openEditor()">{{ t('runtime.ansibleMachines.addMachine') }}</button>
      </div>
    </section>

    <ForwardSuiteNav />

    <section class="stats-grid">
      <article class="stat-card">
        <p class="stat-label">{{ t('runtime.ansibleMachines.stats.machines') }}</p>
        <strong class="stat-value">{{ machines.length }}</strong>
      </article>
      <article class="stat-card">
        <p class="stat-label">{{ t('runtime.ansibleMachines.stats.online') }}</p>
        <strong class="stat-value">{{ onlineCount }}</strong>
      </article>
      <article class="stat-card">
        <p class="stat-label">{{ t('runtime.ansibleMachines.stats.enabled') }}</p>
        <strong class="stat-value">{{ enabledCount }}</strong>
      </article>
    </section>

    <section class="panel-card">
      <div class="section-head">
        <div>
          <p class="eyebrow">{{ t('runtime.ansibleMachines.sectionEyebrow') }}</p>
          <h3>{{ t('runtime.ansibleMachines.sectionTitle') }}</h3>
          <p class="section-copy">{{ t('runtime.ansibleMachines.sectionCopy') }}</p>
        </div>
        <label class="filter-group">
          <span>{{ t('runtime.ansibleMachines.filterLabel') }}</span>
          <select v-model="statusFilter" @change="refreshAll">
            <option value="all">{{ t('runtime.ansibleMachines.filters.all') }}</option>
            <option value="1">{{ t('runtime.ansibleMachines.filters.online') }}</option>
            <option value="0">{{ t('runtime.ansibleMachines.filters.offline') }}</option>
          </select>
        </label>
      </div>
      <p class="inventory-hint">{{ t('runtime.ansibleMachines.inventoryHint') }}</p>

      <div v-if="pageError" class="state-card state-error">{{ pageError }}</div>
      <div v-else-if="loading" class="state-card">{{ t('runtime.ansibleMachines.loading') }}</div>
      <div v-else-if="!machines.length" class="state-card">{{ t('runtime.ansibleMachines.empty') }}</div>
      <div v-else class="machine-grid">
        <article v-for="machine in machines" :key="machine.id" class="machine-card">
          <div class="machine-head">
            <div>
              <p class="eyebrow">{{ t('runtime.ansibleMachines.machineEyebrow', { id: machine.id }) }}</p>
              <h4>{{ machine.name }}</h4>
              <p class="machine-meta">{{ machine.host }}:{{ machine.port }}</p>
            </div>
            <div class="status-stack">
              <span :class="['tag', machine.enabled ? 'tag-success' : 'tag-muted']">{{ machine.enabled ? t('runtime.shared.enabled') : t('runtime.shared.disabled') }}</span>
              <span :class="['tag', machine.status === 1 ? 'tag-success' : 'tag-danger']">{{ machine.status === 1 ? t('runtime.shared.online') : t('runtime.shared.offline') }}</span>
            </div>
          </div>

          <div class="meta-grid">
            <div class="meta-item">
              <span class="meta-label">{{ t('runtime.ansibleMachines.meta.authSource') }}</span>
              <strong>{{ t('runtime.ansibleMachines.meta.authSourceValue') }}</strong>
            </div>
            <div class="meta-item">
              <span class="meta-label">{{ t('runtime.ansibleMachines.meta.regionIsp') }}</span>
              <strong>{{ machine.region || '-' }} / {{ machine.isp || '-' }}</strong>
            </div>
            <div class="meta-item">
              <span class="meta-label">{{ t('runtime.ansibleMachines.meta.currentConn') }}</span>
              <strong>{{ machine.currentConn }}</strong>
            </div>
            <div class="meta-item">
              <span class="meta-label">{{ t('runtime.ansibleMachines.meta.traffic') }}</span>
              <strong>{{ formatBytes(machine.totalUpload) }} / {{ formatBytes(machine.totalDownload) }}</strong>
            </div>
          </div>

          <p v-if="results[machine.id]" :class="['result-text', results[machine.id].success ? 'ok' : 'fail']">
            {{ results[machine.id].message }}
          </p>

          <div class="card-actions">
            <button class="btn btn-secondary btn-sm" @click="openEditor(machine)">{{ t('runtime.ansibleMachines.actions.edit') }}</button>
            <button class="btn btn-secondary btn-sm" :disabled="pendingAction === `${machine.id}:check`" @click="checkMachine(machine)">
              {{ pendingAction === `${machine.id}:check` ? t('runtime.ansibleMachines.actions.checking') : t('runtime.ansibleMachines.actions.check') }}
            </button>
            <button class="btn btn-secondary btn-sm" :disabled="pendingAction === `${machine.id}:sync`" @click="syncMachine(machine)">
              {{ pendingAction === `${machine.id}:sync` ? t('runtime.ansibleMachines.actions.syncing') : t('runtime.ansibleMachines.actions.sync') }}
            </button>
            <button class="btn btn-secondary btn-sm" :disabled="pendingAction === `${machine.id}:toggle`" @click="toggleMachine(machine)">
              {{ machine.enabled ? t('runtime.ansibleMachines.actions.disable') : t('runtime.ansibleMachines.actions.enable') }}
            </button>
            <button class="btn btn-secondary btn-sm danger-text" @click="openDelete(machine)">{{ t('runtime.ansibleMachines.actions.delete') }}</button>
          </div>
        </article>
      </div>
    </section>

    <div v-if="editorOpen" class="modal-overlay" @click.self="closeEditor">
      <div class="modal">
        <div class="modal-header">
          <div>
            <p class="eyebrow">{{ t('runtime.ansibleMachines.modal.eyebrow') }}</p>
            <h3>{{ editorMode ? t('runtime.ansibleMachines.modal.titleEdit') : t('runtime.ansibleMachines.modal.titleAdd') }}</h3>
          </div>
          <button class="modal-close" :aria-label="t('common.actions.close')" :title="t('common.actions.close')" @click="closeEditor">&times;</button>
        </div>
        <div class="modal-body">
          <p class="inventory-hint compact">{{ t('runtime.ansibleMachines.inventoryHint') }}</p>
          <div class="form-grid">
            <label class="form-group">
              <span>{{ t('runtime.ansibleMachines.fields.name') }}</span>
              <input v-model.trim="form.name" type="text" :placeholder="t('runtime.ansibleMachines.placeholders.name')" />
            </label>
            <label class="form-group">
              <span>{{ t('runtime.ansibleMachines.fields.host') }}</span>
              <input v-model.trim="form.host" type="text" :placeholder="t('runtime.ansibleMachines.placeholders.host')" />
            </label>
          </div>
          <div class="form-grid">
            <label class="form-group">
              <span>{{ t('runtime.ansibleMachines.fields.reachabilityPort') }}</span>
              <input v-model.trim="form.port" type="number" min="1" max="65535" />
            </label>
            <label class="form-group">
              <span>{{ t('runtime.ansibleMachines.fields.weight') }}</span>
              <input v-model.trim="form.weight" type="number" min="1" />
            </label>
          </div>
          <div class="form-grid">
            <label class="form-group">
              <span>{{ t('runtime.ansibleMachines.fields.region') }}</span>
              <input v-model.trim="form.region" type="text" :placeholder="t('runtime.ansibleMachines.placeholders.region')" />
            </label>
            <label class="form-group">
              <span>{{ t('runtime.ansibleMachines.fields.isp') }}</span>
              <input v-model.trim="form.isp" type="text" :placeholder="t('runtime.ansibleMachines.placeholders.isp')" />
            </label>
          </div>
          <p v-if="formError" class="form-error">{{ formError }}</p>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary" @click="closeEditor">{{ t('runtime.ansibleMachines.modal.cancel') }}</button>
          <button class="btn btn-primary" :disabled="saving" @click="submitForm">{{ saving ? t('runtime.ansibleMachines.modal.saveLoading') : t('runtime.ansibleMachines.modal.save') }}</button>
        </div>
      </div>
    </div>

    <div v-if="deleteTarget" class="modal-overlay" @click.self="deleteTarget = null">
      <div class="modal modal-sm">
        <div class="modal-header">
          <div>
            <p class="eyebrow">{{ t('runtime.ansibleMachines.modal.deleteEyebrow') }}</p>
            <h3>{{ t('runtime.ansibleMachines.modal.deleteTitle') }}</h3>
          </div>
          <button class="modal-close" :aria-label="t('common.actions.close')" :title="t('common.actions.close')" @click="deleteTarget = null">&times;</button>
        </div>
        <div class="modal-body">
          <p>{{ t('runtime.ansibleMachines.modal.deleteConfirm', { name: deleteTarget?.name || '' }) }}</p>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary" @click="deleteTarget = null">{{ t('runtime.ansibleMachines.modal.cancel') }}</button>
          <button class="btn btn-primary" :disabled="saving" @click="confirmDelete">{{ saving ? t('runtime.ansibleMachines.modal.deleteLoading') : t('runtime.ansibleMachines.actions.delete') }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import {
  checkAnsibleMachine,
  createAnsibleMachine,
  deleteAnsibleMachine,
  getAnsibleMachine,
  getAnsibleMachines,
  syncAnsibleMachineStats,
  toggleAnsibleMachine,
  updateAnsibleMachine
} from '@/api/admin'
import ForwardSuiteNav from '@/components/admin/ForwardSuiteNav.vue'

const { t, translateLiteral } = useAppI18n()

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
const pageError = ref('')
const form = reactive(createForm())

const onlineCount = computed(() => machines.value.filter(item => item.status === 1).length)
const enabledCount = computed(() => machines.value.filter(item => item.enabled).length)

function translateRuntimeText(value, fallback = '-') {
  const text = String(value ?? '').trim()
  if (!text) {
    return fallback
  }
  return translateLiteral(text)
}

function resolveRuntimeError(error, fallbackKey) {
  return translateRuntimeText(error?.response?.data?.msg || error?.message, t(fallbackKey))
}

function createForm() {
  return { id: null, name: '', host: '', port: '', region: '', isp: '', weight: '1' }
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
    region: node?.region || '',
    isp: node?.isp || '',
    weight: Number(node?.weight || 1),
    enabled: Boolean(node?.enabled ?? true),
    status: Number(node?.status ?? 0),
    currentConn: Number(node?.current_conn || node?.currentConn || 0),
    totalUpload: Number(node?.total_upload || node?.totalUpload || 0),
    totalDownload: Number(node?.total_download || node?.totalDownload || 0)
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
  pageError.value = ''
  try {
    const params = { page: 1, page_size: 200, type: 'relay' }
    if (statusFilter.value !== 'all') params.status = Number(statusFilter.value)
    const payload = unwrapResponse(await getAnsibleMachines(params))
    const list = Array.isArray(payload?.list) ? payload.list : Array.isArray(payload) ? payload : []
    machines.value = list.map(normalizeMachine)
  } catch (error) {
    pageError.value = resolveRuntimeError(error, 'runtime.ansibleMachines.errors.loadFailed')
    machines.value = []
  } finally {
    loading.value = false
  }
}

async function openEditor(machine = null) {
  resetForm()
  pageError.value = ''
  editorMode.value = Boolean(machine?.id)
  editorOpen.value = true
  if (!machine?.id) return
  try {
    const detail = normalizeMachine(unwrapResponse(await getAnsibleMachine(machine.id)))
    Object.assign(form, {
      id: detail.id,
      name: detail.name,
      host: detail.host,
      port: detail.port ? String(detail.port) : '',
      region: detail.region,
      isp: detail.isp,
      weight: String(detail.weight || 1)
    })
  } catch (error) {
    formError.value = resolveRuntimeError(error, 'runtime.ansibleMachines.errors.detailFailed')
  }
}

function closeEditor(force = false) {
  if (saving.value && !force) return
  editorOpen.value = false
  editorMode.value = false
  resetForm()
}

async function submitForm() {
  formError.value = ''
  if (!form.name || !form.host || !Number(form.port)) {
    formError.value = t('runtime.ansibleMachines.errors.required')
    return
  }
  const payload = {
    name: form.name.trim(),
    type: 'relay',
    host: form.host.trim(),
    port: Number(form.port),
    weight: Number(form.weight || 1)
  }
  if (form.region.trim()) payload.region = form.region.trim()
  if (form.isp.trim()) payload.isp = form.isp.trim()

  saving.value = true
  try {
    if (editorMode.value && form.id) {
      await updateAnsibleMachine(form.id, payload)
    } else {
      await createAnsibleMachine(payload)
    }
    await refreshAll()
    closeEditor(true)
  } catch (error) {
    formError.value = resolveRuntimeError(error, 'runtime.ansibleMachines.errors.saveFailed')
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
    await deleteAnsibleMachine(deleteTarget.value.id)
    deleteTarget.value = null
    await refreshAll()
  } catch (error) {
    pageError.value = resolveRuntimeError(error, 'runtime.ansibleMachines.errors.deleteFailed')
  } finally {
    saving.value = false
  }
}

async function checkMachine(machine) {
  pendingAction.value = `${machine.id}:check`
  try {
    const payload = unwrapResponse(await checkAnsibleMachine(machine.id))
    const success = Number(payload?.status ?? 0) === 1 && !payload?.error
    results[machine.id] = {
      success,
      message: success
        ? (payload?.latency
          ? t('runtime.ansibleMachines.results.latency', { value: payload.latency })
          : t('runtime.ansibleMachines.results.reachable'))
        : translateRuntimeText(payload?.error, t('runtime.ansibleMachines.results.unavailable'))
    }
    await refreshAll()
  } catch (error) {
    results[machine.id] = { success: false, message: resolveRuntimeError(error, 'runtime.ansibleMachines.errors.checkFailed') }
  } finally {
    pendingAction.value = ''
  }
}

async function syncMachine(machine) {
  pendingAction.value = `${machine.id}:sync`
  try {
    const payload = unwrapResponse(await syncAnsibleMachineStats(machine.id))
    results[machine.id] = { success: true, message: translateRuntimeText(payload?.message, t('runtime.ansibleMachines.results.synced')) }
    await refreshAll()
  } catch (error) {
    results[machine.id] = { success: false, message: resolveRuntimeError(error, 'runtime.ansibleMachines.errors.syncFailed') }
  } finally {
    pendingAction.value = ''
  }
}

async function toggleMachine(machine) {
  pendingAction.value = `${machine.id}:toggle`
  try {
    await toggleAnsibleMachine(machine.id, !machine.enabled)
    await refreshAll()
  } catch (error) {
    results[machine.id] = { success: false, message: resolveRuntimeError(error, 'runtime.ansibleMachines.errors.toggleFailed') }
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
.inventory-hint { margin: 0 0 18px; padding: 12px 14px; border: 1px solid rgba(16,185,129,.24); border-radius: 14px; background: rgba(16,185,129,.08); color: var(--text-secondary); line-height: 1.6; }
.inventory-hint.compact { margin: 0; }
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
.state-error { color: #b91c1c; border-color: rgba(239,68,68,.24); background: rgba(239,68,68,.08); }
.modal-overlay { position: fixed; inset: 0; display: flex; align-items: center; justify-content: center; padding: 20px; background: rgba(15,23,42,.62); z-index: 1100; }
.modal { width: min(100%, 720px); border-radius: 18px; border: 1px solid var(--border-color); background: var(--surface-color); }
.modal-sm { width: min(100%, 420px); }
.modal-header, .modal-footer { display: flex; justify-content: space-between; gap: 12px; padding: 18px 22px; border-bottom: 1px solid var(--border-color); }
.modal-footer { border-top: 1px solid var(--border-color); border-bottom: 0; justify-content: flex-end; }
.modal-body { padding: 20px 22px; display: flex; flex-direction: column; gap: 14px; }
.modal-close { border: 0; background: transparent; color: var(--text-secondary); cursor: pointer; font-size: 0; line-height: 1; }
.modal-close::before { content: '\00d7'; font-size: 24px; }
.form-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
@media (max-width: 900px) {
  .hero-card, .section-head, .machine-head, .modal-header, .modal-footer { flex-direction: column; }
  .meta-grid, .form-grid { grid-template-columns: 1fr; }
  .status-stack { align-items: flex-start; }
}
</style>
