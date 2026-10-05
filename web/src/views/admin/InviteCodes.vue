<template>
  <div class="list-page">
    <UiPageHeader :title="t('adminInviteCodes.title')" :description="t('adminInviteCodes.subtitle')">
      <template #actions>
        <UiButton v-if="isCommercial" as="router-link" to="/admin/invite" :icon-end="ArrowRight" data-testid="invite-rewards-link">
          {{ t('adminInviteCodes.rewardsLink') }}
        </UiButton>
        <UiButton variant="primary" :icon="Plus" data-testid="open-generate" @click="openGenerate">{{ t('adminInviteCodes.generate.title') }}</UiButton>
      </template>
    </UiPageHeader>

    <p class="registration-hint" data-testid="registration-hint">
      <UiBadge :tone="requireInvite ? 'info' : 'neutral'" :dot="false" :label="requireInvite ? t('adminInviteCodes.registration.requiredBadge') : t('adminInviteCodes.registration.optionalBadge')" />
      <span>{{ requireInvite ? t('adminInviteCodes.registration.required') : t('adminInviteCodes.registration.optional') }}</span>
    </p>

    <UiDataTable
      v-model:selected="selectedIds"
      :columns="columns"
      :rows="codes"
      :label="t('adminInviteCodes.table.label')"
      :row-label="item => item.code"
      storage-key="admin.invite-codes"
      manual-pagination
      :page="page"
      :page-size="pageSize"
      :total="total"
      :loading="loading"
      :error="loadError"
      :error-title="t('adminInviteCodes.messages.fetchFailed')"
      :filtered="Boolean(filter)"
      :empty-icon="Ticket"
      :empty-title="t('adminInviteCodes.empty.title')"
      :empty-description="t('adminInviteCodes.empty.description')"
      selectable
      :row-actions="codeActions"
      @update:page="goTo"
      @retry="fetchCodes"
      @clear-filters="filter = ''"
    >
      <template #toolbar>
        <UiFilterChips v-model="filter" :label="t('adminInviteCodes.filters.label')" :options="statusChips" data-testid="invite-code-filter" />
      </template>
      <template #empty-actions>
        <UiButton variant="primary" :icon="Plus" @click="openGenerate">{{ t('adminInviteCodes.generate.title') }}</UiButton>
      </template>
      <template #cell-code="{ row }">
        <code class="invite-code">{{ row.code }}</code>
      </template>
      <template #cell-status="{ row }">
        <UiBadge :tone="STATE_TONES[codeState(row)]" :label="t(`adminInviteCodes.status.${codeState(row)}`)" />
      </template>
      <template #bulk-actions="{ rows: chosen }">
        <UiButton size="sm" :icon="Copy" data-testid="bulk-copy" @click="copy(chosen.map(item => item.code).join('\n'))">{{ t('adminInviteCodes.actions.copy') }}</UiButton>
        <UiButton size="sm" variant="danger-soft" :icon="Ban" :disabled="!chosen.some(item => item.status === 0)" data-testid="bulk-revoke" @click="revokeMany(chosen)">{{ t('adminInviteCodes.actions.revoke') }}</UiButton>
      </template>
    </UiDataTable>

    <UiDialog
      v-model:open="showGenerate"
      size="sm"
      :title="t('adminInviteCodes.generate.title')"
      :description="t('adminInviteCodes.generate.description')"
      :dismissible="!generating"
    >
      <div v-if="generated.length" class="generated" data-testid="generated-codes">
        <p class="generated__lead">{{ t('adminInviteCodes.generate.created', { count: generated.length }) }}</p>
        <ul class="generated__list">
          <li v-for="item in generated" :key="item.id"><code class="invite-code">{{ item.code }}</code></li>
        </ul>
      </div>
      <div v-else class="generate-form">
        <UiTextField
          id="invite-code-count"
          v-model.number="form.count"
          type="number"
          min="1"
          :max="maxBatch"
          :label="t('adminInviteCodes.generate.count')"
          :help="t('adminInviteCodes.generate.countHelp', { max: maxBatch })"
          :error="countError"
        />
        <UiTextField
          id="invite-code-expire"
          v-model="form.expireDays"
          type="number"
          min="0"
          :label="t('adminInviteCodes.generate.expireDays')"
          :placeholder="t('adminInviteCodes.generate.expireDaysPlaceholder')"
          :help="t('adminInviteCodes.generate.expireDaysHelp')"
          :suffix="t('adminInviteCodes.generate.daysUnit')"
        />
      </div>
      <template #footer="{ close }">
        <template v-if="generated.length">
          <UiButton :icon="Copy" @click="copy(generated.map(item => item.code).join('\n'))">{{ t('adminInviteCodes.actions.copyAll') }}</UiButton>
          <UiButton variant="primary" @click="close">{{ t('adminInviteCodes.actions.done') }}</UiButton>
        </template>
        <template v-else>
          <UiButton :disabled="generating" @click="close">{{ t('common.actions.cancel') }}</UiButton>
          <UiButton variant="primary" data-testid="generate-invite-codes" :loading="generating" @click="generate">{{ t('adminInviteCodes.generate.submit') }}</UiButton>
        </template>
      </template>
    </UiDialog>
  </div>
</template>

<script setup>
// 邀请码 (plan §7.1 list page): registration control in every edition
// (identity-platform). Codes as a server-paged list with status chips, a
// "…" menu (copy, revoke), bulk copy / revoke, and a dialog that generates
// a batch and shows it. Commissions, withdrawals and statistics are the
// commercial 邀请返佣 page. Endpoints: GET/POST /admin/invite/codes, DELETE
// /admin/invite/codes/:id, and POST /api/v4/admin/invite-codes/bulk for the
// bulk revoke.
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ArrowRight, Ban, Copy, Plus, Ticket } from '@lucide/vue'
import { bulkInviteCodes, generateInviteCodes, getInviteCodes, revokeInviteCode } from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import { useBulkReport } from '@/composables/useBulkReport'
import { useListQuery } from '@/composables/useListQuery'
import { useEdition } from '@/composables/useEdition'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiDialog from '@/ui/UiDialog.vue'
import UiFilterChips from '@/ui/UiFilterChips.vue'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import UiTextField from '@/ui/UiTextField.vue'
import { copyText } from '@/ui/composables/useClipboard'
import { useConfirm } from '@/ui/composables/useConfirm'
import { useFormat } from '@/ui/composables/useFormat'
import { useToast } from '@/ui/composables/useToast'
import { adminV4ErrorMessage } from '@/utils/adminV4'
import { readBulkResult } from '@/utils/bulkResult'

const { t } = useAppI18n()
const format = useFormat()
const toast = useToast()
const confirm = useConfirm()
const bulkReport = useBulkReport()
const { isCommercial, requireInvite, loadEdition } = useEdition()

const maxBatch = 50
const pageSize = 20
const STATE_TONES = { unused: 'success', used: 'neutral', expired: 'warning' }

// The status chip and the page live in the URL query (plan §9).
const listQuery = useListQuery()
const codes = ref([])
const total = ref(0)
const page = ref(listQuery.readPage())
const filter = ref(listQuery.read('status', { values: ['unused', 'used', 'expired'] }))
const loading = ref(false)
const loadError = ref(null)
const selectedIds = ref([])
const showGenerate = ref(false)
const generating = ref(false)
const generated = ref([])
const countError = ref('')
const form = reactive({ count: 1, expireDays: '' })

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))

const columns = computed(() => [
  { key: 'code', label: t('adminInviteCodes.table.code'), primary: true, hideable: false },
  { key: 'status', label: t('adminInviteCodes.table.status'), secondary: true },
  { key: 'owner', label: t('adminInviteCodes.table.owner'), value: item => (item.user_id ? t('adminInviteCodes.owner.user', { id: item.user_id }) : t('adminInviteCodes.owner.admin')) },
  { key: 'used_by', label: t('adminInviteCodes.table.usedBy'), numeric: true, format: value => (value ? `#${value}` : '—') },
  { key: 'expired_at', label: t('adminInviteCodes.table.expiresAt'), nowrap: true, format: value => (value ? format.dateTime(value) : t('adminInviteCodes.never')) },
  { key: 'created_at', label: t('adminInviteCodes.table.createdAt'), nowrap: true, breakpoint: 'lg', format: value => format.dateTime(value) }
])
const statusChips = computed(() => [
  { value: 'unused', label: t('adminInviteCodes.status.unused') },
  { value: 'used', label: t('adminInviteCodes.status.used') },
  { value: 'expired', label: t('adminInviteCodes.status.expired') }
])
const codeActions = item => [
  { key: 'copy', label: t('adminInviteCodes.actions.copy'), icon: Copy, onSelect: () => copy(item.code) },
  { key: 'revoke', label: t('adminInviteCodes.actions.revokeOne'), icon: Ban, danger: true, separatorBefore: true, hidden: item.status !== 0, onSelect: () => revoke(item) }
]

// readPanel returns the data of a panel answer and throws its message when
// the answer is an error (code other than 0).
function readPanel(res) {
  if (res && typeof res === 'object' && Object.prototype.hasOwnProperty.call(res, 'code')) {
    if (res.code !== 0) {
      throw new Error(res.msg || t('adminInviteCodes.messages.failed'))
    }
    return res.data ?? {}
  }
  return res?.data ?? res ?? {}
}

function errorMessage(error) {
  return error?.response?.data?.msg || error?.response?.data?.error || error?.message || t('adminInviteCodes.messages.failed')
}

function codeState(item) {
  if (item.status === 1) return 'used'
  if (item.expired_at && new Date(item.expired_at).getTime() <= Date.now()) return 'expired'
  return 'unused'
}

async function fetchCodes() {
  loading.value = true
  listQuery.write({ status: filter.value, page: page.value })
  try {
    const params = { page: page.value, page_size: pageSize }
    if (filter.value) params.status = filter.value
    const data = readPanel(await getInviteCodes(params))
    codes.value = Array.isArray(data.list) ? data.list : []
    total.value = Number(data.total || 0)
    loadError.value = null
  } catch (error) {
    loadError.value = error
    codes.value = []
    total.value = 0
  } finally {
    loading.value = false
  }
}

function reload() {
  page.value = 1
  selectedIds.value = []
  return fetchCodes()
}

function goTo(next) {
  page.value = Math.min(Math.max(1, next), totalPages.value)
  selectedIds.value = []
  return fetchCodes()
}

watch(filter, () => { reload() })

function openGenerate() {
  generated.value = []
  countError.value = ''
  showGenerate.value = true
}

async function generate() {
  const count = Number(form.count)
  countError.value = ''
  if (!Number.isInteger(count) || count < 1 || count > maxBatch) {
    countError.value = t('adminInviteCodes.messages.countRange', { max: maxBatch })
    return
  }
  const payload = { count }
  if (form.expireDays !== '' && form.expireDays !== null) {
    payload.expire_days = Number(form.expireDays)
  }
  generating.value = true
  try {
    const data = readPanel(await generateInviteCodes(payload))
    generated.value = Array.isArray(data.codes) ? data.codes : []
    toast.success(t('adminInviteCodes.messages.generated', { count: generated.value.length }))
    await reload()
  } catch (error) {
    countError.value = t('adminInviteCodes.messages.generateFailed', { message: errorMessage(error) })
  } finally {
    generating.value = false
  }
}

async function revoke(item) {
  const confirmed = await confirm({
    title: t('adminInviteCodes.confirm.revokeTitle', { code: item.code }),
    message: t('adminInviteCodes.confirm.revokeMessage'),
    confirmLabel: t('adminInviteCodes.actions.revokeOne'),
    tone: 'danger'
  })
  if (!confirmed) return
  try {
    readPanel(await revokeInviteCode(item.id))
    toast.success(t('adminInviteCodes.messages.revoked', { code: item.code }))
    await fetchCodes()
  } catch (error) {
    toast.error(t('adminInviteCodes.messages.revokeFailed', { message: errorMessage(error) }))
  }
}

// Bulk revoke: one request for the selected unused codes (POST
// /api/v4/admin/invite-codes/bulk) after one confirmation, answered per code.
// A code that was used is kept (conflict) and one that is gone is not found;
// the codes that could not be revoked stay selected and the toast says why,
// with 重试 for those worth another try.
async function revokeMany(chosen) {
  const targets = chosen.filter(item => item.status === 0)
  if (!targets.length) return
  const ids = targets.map(item => item.id)
  let outcome = null
  const confirmed = await confirm({
    title: t('adminInviteCodes.confirm.revokeManyTitle', { count: targets.length }),
    message: t('adminInviteCodes.confirm.revokeMessage'),
    confirmLabel: t('adminInviteCodes.actions.revoke'),
    tone: 'danger',
    onConfirm: async () => {
      try {
        outcome = await runRevoke(ids)
      } catch (error) {
        throw new Error(adminV4ErrorMessage(error, t('adminInviteCodes.messages.failed')))
      }
    }
  })
  if (!confirmed || !outcome) return
  await reportRevoke(outcome, ids.length)
}

const runRevoke = async ids => readBulkResult(await bulkInviteCodes('revoke', ids), ids)

async function reportRevoke(outcome, total) {
  selectedIds.value = outcome.failedIds
  await fetchCodes()
  bulkReport.report(outcome, {
    success: t('adminInviteCodes.messages.revokedMany', { count: outcome.done.length }),
    partial: t('adminInviteCodes.messages.revokePartial', { done: outcome.done.length, total }),
    none: t('adminInviteCodes.messages.revokeNone'),
    retry: () => retryRevoke(outcome.retryable)
  })
}

async function retryRevoke(ids) {
  try {
    await reportRevoke(await runRevoke(ids), ids.length)
  } catch (error) {
    toast.error(t('adminInviteCodes.messages.revokeFailed', { message: adminV4ErrorMessage(error, t('adminInviteCodes.messages.failed')) }))
  }
}

async function copy(text) {
  if (await copyText(text)) toast.success(t('adminInviteCodes.messages.copied'))
  else toast.error(t('adminInviteCodes.messages.copyFailed'))
}

onMounted(() => {
  loadEdition()
  fetchCodes()
})

defineExpose({ fetchCodes, generate, revoke })
</script>

<style scoped>
.registration-hint {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
  margin-top: calc(-1 * var(--space-2));
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.invite-code {
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
  letter-spacing: 0.04em;
}

.generate-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.generated {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.generated__lead {
  color: var(--label-2);
}

.generated__list {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(120px, 1fr));
  gap: var(--space-2);
  max-height: 280px;
  padding: var(--space-3);
  margin: 0;
  overflow-y: auto;
  border-radius: var(--radius-sm);
  background: var(--bg-grouped);
  list-style: none;
}
</style>
