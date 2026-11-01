<template>
  <div class="monitor-section" data-monitor-categories>
    <QosSettingsCard />

    <UiCard :title="t('adminMonitor.categories.test.title')" :description="t('adminMonitor.categories.test.description')" as="section" data-category-test>
      <form class="category-test" @submit.prevent="runTest">
        <UiTextField v-model="test.domain" class="category-test__domain" :label="t('adminMonitor.categories.test.domain')" placeholder="youtube.com" size="md" data-category-test-domain />
        <UiTextField v-model="test.protocol" class="category-test__protocol" :label="t('adminMonitor.categories.test.protocol')" placeholder="tls" size="md" />
        <UiButton type="submit" :loading="test.running" :disabled="!test.domain.trim() && !test.protocol.trim()" data-category-test-run>{{ t('adminMonitor.categories.test.run') }}</UiButton>
      </form>
      <p v-if="test.result" class="category-test__result" role="status" data-category-test-result>{{ test.result }}</p>
    </UiCard>

    <UiSection :title="t('adminMonitor.categories.rules.title')" :description="t('adminMonitor.categories.rules.description')" heading-tag="h2" data-category-rules>
      <template #actions>
        <UiButton size="md" :icon="Plus" :disabled="loading || Boolean(error)" data-category-rule-add @click="openRule(null)">{{ t('adminMonitor.categories.rules.add') }}</UiButton>
      </template>
      <UiDataTable
        :columns="ruleColumns"
        :rows="rules"
        :label="t('adminMonitor.categories.rules.label')"
        :row-label="row => row.value"
        :row-actions="ruleActions"
        storage-key="admin.monitor.category-rules"
        :loading="loading"
        :error="error"
        :error-title="t('adminMonitor.categories.loadFailed')"
        :empty-icon="Tags"
        :empty-title="t('adminMonitor.categories.rules.emptyTitle')"
        :empty-description="t('adminMonitor.categories.rules.emptyDescription')"
        :page-size="50"
        state-heading-tag="h3"
        @retry="load"
      >
        <template #cell-value="{ row }">
          <span class="category-mono">{{ row.value }}</span>
        </template>
      </UiDataTable>
    </UiSection>

    <UiSection :title="t('adminMonitor.categories.sources.title')" :description="t('adminMonitor.categories.sources.description')" heading-tag="h2" data-category-sources>
      <template #actions>
        <UiButton size="md" :icon="Plus" :disabled="loading || Boolean(error)" data-category-source-add @click="openSource(null)">{{ t('adminMonitor.categories.sources.add') }}</UiButton>
      </template>
      <UiDataTable
        :columns="sourceColumns"
        :rows="sources"
        :label="t('adminMonitor.categories.sources.label')"
        :row-label="row => row.name"
        :row-actions="sourceActions"
        storage-key="admin.monitor.category-sources"
        :loading="loading"
        :error="error"
        :error-title="t('adminMonitor.categories.loadFailed')"
        :empty-icon="Globe"
        :empty-title="t('adminMonitor.categories.sources.emptyTitle')"
        :empty-description="t('adminMonitor.categories.sources.emptyDescription')"
        state-heading-tag="h3"
        @retry="load"
      >
        <template #cell-url="{ row }">
          <span class="category-mono">{{ row.url }}</span>
        </template>
        <template #cell-status="{ row }">
          <UiBadge :tone="sourceTone(row)" :label="sourceStatus(row)" :title="row.fetch_error || undefined" />
        </template>
      </UiDataTable>
    </UiSection>

    <UiSection :title="t('adminMonitor.categories.builtin.title')" :description="t('adminMonitor.categories.builtin.description')" heading-tag="h2" data-category-builtin>
      <ul class="category-builtin">
        <li v-for="group in builtin" :key="group.category" class="category-builtin__row">
          <div class="category-builtin__head">
            <span class="category-builtin__name">{{ categoryLabel(group.category) }}</span>
            <span class="category-builtin__counts">{{ t('adminMonitor.categories.builtin.counts', { domains: group.domains.length, networks: group.networks.length, protocols: group.protocols.length }) }}</span>
            <UiButton size="sm" variant="ghost" @click="toggleBuiltin(group.category)">{{ expanded[group.category] ? t('adminMonitor.categories.builtin.hide') : t('adminMonitor.categories.builtin.show') }}</UiButton>
          </div>
          <p v-if="expanded[group.category]" class="category-builtin__list category-mono">{{ [...group.protocols, ...group.domains, ...group.networks].join(' · ') }}</p>
        </li>
      </ul>
    </UiSection>

    <UiDialog v-model:open="ruleDialog.open" :title="ruleDialog.id ? t('adminMonitor.categories.rules.edit') : t('adminMonitor.categories.rules.add')" :dismissible="!ruleDialog.saving" data-category-rule-dialog>
      <form id="category-rule-form" class="category-form" @submit.prevent="saveRule">
        <UiSelect v-model="ruleDialog.kind" :label="t('adminMonitor.categories.rules.kind')" :options="kindOptions" size="md" />
        <UiTextField v-model="ruleDialog.value" :label="t('adminMonitor.categories.rules.value')" :help="t(`adminMonitor.categories.rules.valueHelp.${ruleDialog.kind}`)" required size="md" data-category-rule-value />
        <UiSelect v-model="ruleDialog.category" :label="t('adminMonitor.categories.rules.category')" :options="categoryOptions" size="md" />
        <UiTextField v-model="ruleDialog.note" :label="t('adminMonitor.categories.rules.note')" size="md" />
      </form>
      <template #footer>
        <UiButton variant="ghost" :disabled="ruleDialog.saving" @click="ruleDialog.open = false">{{ t('adminMonitor.categories.cancel') }}</UiButton>
        <UiButton type="submit" form="category-rule-form" variant="primary" :loading="ruleDialog.saving" :disabled="!ruleDialog.value.trim()" data-category-rule-save>{{ t('adminMonitor.categories.save') }}</UiButton>
      </template>
    </UiDialog>

    <UiDialog v-model:open="sourceDialog.open" :title="sourceDialog.id ? t('adminMonitor.categories.sources.edit') : t('adminMonitor.categories.sources.add')" :dismissible="!sourceDialog.saving" data-category-source-dialog>
      <form id="category-source-form" class="category-form" @submit.prevent="saveSource">
        <UiTextField v-model="sourceDialog.name" :label="t('adminMonitor.categories.sources.name')" required size="md" />
        <UiTextField v-model="sourceDialog.url" :label="t('adminMonitor.categories.sources.url')" type="url" required size="md" placeholder="https://" data-category-source-url />
        <UiSelect v-model="sourceDialog.category" :label="t('adminMonitor.categories.sources.category')" :options="categoryOptions" size="md" />
        <UiNumberField v-model="sourceDialog.refresh_hours" :label="t('adminMonitor.categories.sources.refreshHours')" :unit="t('adminMonitor.categories.sources.refreshUnit')" :help="t('adminMonitor.categories.sources.refreshHelp')" :min="1" :max="168" size="md" />
        <UiSwitch v-model="sourceDialog.enabled" :label="t('adminMonitor.categories.sources.enabled')" />
      </form>
      <template #footer>
        <UiButton variant="ghost" :disabled="sourceDialog.saving" @click="sourceDialog.open = false">{{ t('adminMonitor.categories.cancel') }}</UiButton>
        <UiButton type="submit" form="category-source-form" variant="primary" :loading="sourceDialog.saving" :disabled="!sourceDialog.name.trim() || !sourceDialog.url.trim()" data-category-source-save>{{ t('adminMonitor.categories.save') }}</UiButton>
      </template>
    </UiDialog>
  </div>
</template>

<script setup>
// 流量分类 of 流量与监控: the rules 访问统计 judges the categories by
// (GET /api/v4/admin/traffic/categories): the administrator's own rules
// (domain, network or protocol to a category, winning over the rest), the
// remote sources (lists fetched from a URL on a schedule, one category per
// list) and the built-in rules, with a test of where a destination falls
// (GET .../classify). Every change applies to the recorded history at once.
// On top, the QoS settings the categories steer (QosSettingsCard).
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { Globe, Pencil, Plus, RefreshCw, Tags, Trash2 } from '@lucide/vue'
import {
  classifyTrafficDestination,
  createTrafficCategoryRule,
  createTrafficCategorySource,
  deleteTrafficCategoryRule,
  deleteTrafficCategorySource,
  getTrafficCategories,
  refreshTrafficCategorySource,
  updateTrafficCategoryRule,
  updateTrafficCategorySource
} from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiCard from '@/ui/UiCard.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiDialog from '@/ui/UiDialog.vue'
import UiNumberField from '@/ui/UiNumberField.vue'
import UiSection from '@/ui/UiSection.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiSwitch from '@/ui/UiSwitch.vue'
import UiTextField from '@/ui/UiTextField.vue'
import { useConfirm } from '@/ui/composables/useConfirm'
import { useFormat } from '@/ui/composables/useFormat'
import { useToast } from '@/ui/composables/useToast'
import QosSettingsCard from './QosSettingsCard.vue'
import { errorText } from './trafficData'

const RULE_CATEGORIES = ['video', 'music', 'messaging', 'social', 'gaming', 'ai', 'download', 'web']

const { t, te } = useAppI18n()
const toast = useToast()
const confirm = useConfirm()
const format = useFormat()

const rules = ref([])
const sources = ref([])
const builtin = ref([])
const loading = ref(false)
const error = ref(null)
const expanded = reactive({})
const test = reactive({ domain: '', protocol: '', running: false, result: '' })
const ruleDialog = reactive({ open: false, id: 0, kind: 'domain', value: '', category: 'video', note: '', saving: false })
const sourceDialog = reactive({ open: false, id: 0, name: '', url: '', category: 'video', refresh_hours: 24, enabled: true, saving: false })

let isActive = true

function categoryLabel(category) {
  const key = `portal.destinationStats.category.${category}`
  return te(key) ? t(key) : category
}

const categoryOptions = computed(() => RULE_CATEGORIES.map(value => ({ value, label: categoryLabel(value) })))
const kindOptions = computed(() => ['domain', 'network', 'protocol'].map(value => ({ value, label: t(`adminMonitor.categories.rules.kinds.${value}`) })))

const ruleColumns = computed(() => [
  { key: 'value', label: t('adminMonitor.categories.rules.value'), primary: true, hideable: false },
  { key: 'kind', label: t('adminMonitor.categories.rules.kind'), value: row => t(`adminMonitor.categories.rules.kinds.${row.kind}`), secondary: true },
  { key: 'category', label: t('adminMonitor.categories.rules.category'), value: row => categoryLabel(row.category) },
  { key: 'note', label: t('adminMonitor.categories.rules.note'), breakpoint: 'md' }
])

const sourceColumns = computed(() => [
  { key: 'name', label: t('adminMonitor.categories.sources.name'), primary: true, hideable: false },
  { key: 'category', label: t('adminMonitor.categories.sources.category'), value: row => categoryLabel(row.category), secondary: true },
  { key: 'url', label: t('adminMonitor.categories.sources.url'), breakpoint: 'lg' },
  { key: 'entries', label: t('adminMonitor.categories.sources.entries'), numeric: true, align: 'end', value: row => row.entry_count || 0 },
  { key: 'fetched', label: t('adminMonitor.categories.sources.fetchedAt'), breakpoint: 'md', value: row => (row.fetched_at ? format.dateTime(row.fetched_at) : t('adminMonitor.categories.sources.never')) },
  { key: 'status', label: t('adminMonitor.categories.sources.status') }
])

function sourceTone(row) {
  if (!row.enabled) return 'neutral'
  return row.fetch_error ? 'danger' : (row.fetched_at ? 'success' : 'neutral')
}

function sourceStatus(row) {
  if (!row.enabled) return t('adminMonitor.categories.sources.disabled')
  if (row.fetch_error) return t('adminMonitor.categories.sources.failed')
  return row.fetched_at ? t('adminMonitor.categories.sources.ok') : t('adminMonitor.categories.sources.never')
}

const ruleActions = row => [
  { key: 'edit', label: t('adminMonitor.categories.rules.edit'), icon: Pencil, onSelect: () => openRule(row) },
  { key: 'delete', label: t('adminMonitor.categories.rules.deleteConfirm'), icon: Trash2, danger: true, separatorBefore: true, onSelect: () => removeRule(row) }
]

const sourceActions = row => [
  { key: 'refresh', label: t('adminMonitor.categories.sources.refresh'), icon: RefreshCw, onSelect: () => refreshSource(row) },
  { key: 'edit', label: t('adminMonitor.categories.sources.edit'), icon: Pencil, onSelect: () => openSource(row) },
  { key: 'delete', label: t('adminMonitor.categories.sources.deleteConfirm'), icon: Trash2, danger: true, separatorBefore: true, onSelect: () => removeSource(row) }
]

function failure(err, fallback) {
  return err?.response?.data?.error?.message || errorText(err, fallback)
}

async function load() {
  loading.value = true
  error.value = null
  try {
    const answer = await getTrafficCategories()
    if (!isActive) return
    rules.value = Array.isArray(answer?.rules) ? answer.rules : []
    sources.value = Array.isArray(answer?.sources) ? answer.sources : []
    builtin.value = Array.isArray(answer?.builtin) ? answer.builtin : []
  } catch (err) {
    if (!isActive) return
    error.value = err
  } finally {
    if (isActive) loading.value = false
  }
}

async function runTest() {
  test.running = true
  test.result = ''
  try {
    const answer = await classifyTrafficDestination(test.domain.trim(), test.protocol.trim())
    test.result = t('adminMonitor.categories.test.result', { target: answer?.domain || answer?.protocol || '—', category: categoryLabel(answer?.category) })
  } catch (err) {
    toast.error(failure(err, t('adminMonitor.categories.test.failed')))
  } finally {
    test.running = false
  }
}

function toggleBuiltin(category) {
  expanded[category] = !expanded[category]
}

function openRule(row) {
  Object.assign(ruleDialog, {
    open: true,
    id: row?.id || 0,
    kind: row?.kind || 'domain',
    value: row?.value || '',
    category: row?.category || 'video',
    note: row?.note || '',
    saving: false
  })
}

async function saveRule() {
  ruleDialog.saving = true
  const body = { kind: ruleDialog.kind, value: ruleDialog.value.trim(), category: ruleDialog.category, note: ruleDialog.note.trim() }
  try {
    if (ruleDialog.id) await updateTrafficCategoryRule(ruleDialog.id, body)
    else await createTrafficCategoryRule(body)
    ruleDialog.open = false
    toast.success(t('adminMonitor.categories.rules.saved'))
    await load()
  } catch (err) {
    toast.error(failure(err, t('adminMonitor.categories.rules.saveFailed')))
  } finally {
    ruleDialog.saving = false
  }
}

async function removeRule(row) {
  const confirmed = await confirm({
    title: t('adminMonitor.categories.rules.deleteTitle', { value: row.value }),
    message: t('adminMonitor.categories.rules.deleteMessage'),
    confirmLabel: t('adminMonitor.categories.rules.deleteConfirm'),
    tone: 'danger'
  })
  if (!confirmed) return
  try {
    await deleteTrafficCategoryRule(row.id)
    toast.success(t('adminMonitor.categories.rules.deleted'))
    await load()
  } catch (err) {
    toast.error(failure(err, t('adminMonitor.categories.rules.deleteFailed')))
  }
}

function openSource(row) {
  Object.assign(sourceDialog, {
    open: true,
    id: row?.id || 0,
    name: row?.name || '',
    url: row?.url || '',
    category: row?.category || 'video',
    refresh_hours: row?.refresh_hours || 24,
    enabled: row ? Boolean(row.enabled) : true,
    saving: false
  })
}

async function saveSource() {
  sourceDialog.saving = true
  const body = {
    name: sourceDialog.name.trim(),
    url: sourceDialog.url.trim(),
    category: sourceDialog.category,
    refresh_hours: Number(sourceDialog.refresh_hours) || 24,
    enabled: sourceDialog.enabled
  }
  try {
    if (sourceDialog.id) await updateTrafficCategorySource(sourceDialog.id, body)
    else await createTrafficCategorySource(body)
    sourceDialog.open = false
    toast.success(t('adminMonitor.categories.sources.saved'))
    await load()
  } catch (err) {
    toast.error(failure(err, t('adminMonitor.categories.sources.saveFailed')))
  } finally {
    sourceDialog.saving = false
  }
}

async function refreshSource(row) {
  try {
    const answer = await refreshTrafficCategorySource(row.id)
    const source = answer?.source || {}
    if (source.fetch_error) toast.error(t('adminMonitor.categories.sources.refreshFailed', { error: source.fetch_error }))
    else toast.success(t('adminMonitor.categories.sources.refreshed', { count: source.entry_count || 0 }))
    await load()
  } catch (err) {
    toast.error(failure(err, t('adminMonitor.categories.sources.refreshFailed', { error: '' })))
  }
}

async function removeSource(row) {
  const confirmed = await confirm({
    title: t('adminMonitor.categories.sources.deleteTitle', { name: row.name }),
    message: t('adminMonitor.categories.sources.deleteMessage'),
    confirmLabel: t('adminMonitor.categories.sources.deleteConfirm'),
    tone: 'danger'
  })
  if (!confirmed) return
  try {
    await deleteTrafficCategorySource(row.id)
    toast.success(t('adminMonitor.categories.sources.deleted'))
    await load()
  } catch (err) {
    toast.error(failure(err, t('adminMonitor.categories.sources.deleteFailed')))
  }
}

onMounted(() => {
  void load()
})

onUnmounted(() => {
  isActive = false
})

defineExpose({ load, rules, sources })
</script>

<style scoped>
.category-test {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  gap: var(--space-4);
}

.category-test__domain {
  flex: 1 1 260px;
}

.category-test__protocol {
  width: 160px;
}

.category-test__result {
  margin: var(--space-3) 0 0;
  color: var(--label-1);
}

.category-mono {
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
  overflow-wrap: anywhere;
}

.category-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.category-builtin {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  margin: 0;
  padding: 0;
  list-style: none;
}

.category-builtin__head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2) var(--space-4);
}

.category-builtin__name {
  min-width: 64px;
  font-weight: var(--weight-medium);
}

.category-builtin__counts {
  flex: 1 1 auto;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.category-builtin__list {
  margin: var(--space-2) 0 0;
  color: var(--label-2);
}
</style>
