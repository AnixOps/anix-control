<template>
  <div class="api-tokens" data-security-panel="api-tokens">
    <UiSection :title="t('adminApiTokens.title')" :description="t('adminApiTokens.description')">
      <template #actions>
        <ApiTokenCreate ref="creator" :disabled="limitReached" @created="onCreated" />
      </template>

      <p v-if="quotaText" class="api-tokens__quota" :class="{ 'is-full': limitReached }" data-testid="api-tokens-quota">{{ quotaText }}</p>

      <UiDataTable
        :columns="columns"
        :rows="rows"
        :label="t('adminApiTokens.table.label')"
        :row-label="row => row.name"
        storage-key="admin.security.apiTokens"
        :page-size="20"
        :loading="loading"
        :error="error"
        :error-title="t('adminApiTokens.loadFailed')"
        :empty-icon="KeySquare"
        :empty-title="t('adminApiTokens.empty.title')"
        :empty-description="t('adminApiTokens.empty.description')"
        :row-actions="rowActions"
        :card-fields="4"
        state-heading-tag="h3"
        data-testid="api-tokens-table"
        @retry="load"
      >
        <template #toolbar>
          <UiFilterChips v-model="filters" multiple :label="t('adminApiTokens.filters.label')" :options="filterChips" data-testid="api-tokens-filters" />
          <p v-if="superAdmin === false" class="api-tokens__note" data-testid="api-tokens-super-only">{{ t('adminApiTokens.filters.superOnly') }}</p>
        </template>
        <template #toolbar-end>
          <UiButton size="md" :icon="RotateCw" :loading="loading" data-testid="api-tokens-refresh" @click="load">{{ t('adminApiTokens.refresh') }}</UiButton>
        </template>
        <template #empty-actions>
          <UiButton variant="primary" :icon="Plus" :disabled="limitReached" data-testid="api-tokens-empty-create" @click="creator?.open()">
            {{ t('adminApiTokens.create.action') }}
          </UiButton>
        </template>
        <template #cell-name="{ row }">
          <span class="api-token-name">
            <span class="api-token-name__text">{{ row.name }}</span>
            <code class="api-token-name__hint" :title="t('adminApiTokens.table.hintTitle')">anixadm_…{{ row.hint }}</code>
            <span class="api-token-name__created" :title="format.dateTime(row.createdAt)">{{ t('adminApiTokens.table.createdOn', { date: format.date(row.createdAt) }) }}</span>
          </span>
        </template>
        <template #cell-owner="{ row }">
          <span :class="{ 'api-token-owner--you': isMine(row) }" :title="row.ownerEmail ? t('adminApiTokens.owner.admin', { id: row.userId }) : undefined">{{ ownerText(row) }}</span>
        </template>
        <template #cell-scope="{ row }">
          <UiBadge :tone="row.scope === 'admin' ? 'warning' : 'info'" :dot="false" :label="t(`adminApiTokens.scopes.${row.scope}`)" />
        </template>
        <template #cell-state="{ row }">
          <UiBadge :tone="STATE_TONES[row.state]" :label="t(`adminApiTokens.states.${row.state}`)" :title="row.state === 'revoked' ? revokeReason(row) : undefined" />
        </template>
        <template #cell-expiresAt="{ row }">
          <span class="api-token-time" :title="row.expiresAt ? format.dateTime(row.expiresAt) : undefined">
            <span>{{ expiryText(row) }}</span>
            <span v-if="expirySubtext(row)" class="api-token-time__sub">{{ expirySubtext(row) }}</span>
          </span>
        </template>
        <template #cell-lastUsedAt="{ row }">
          <span v-if="row.lastUsedAt" class="api-token-time" :title="format.dateTime(row.lastUsedAt)">
            <span>{{ format.relativeTime(row.lastUsedAt, { now }) }}</span>
            <code v-if="row.lastUsedIp" class="api-token-time__sub api-token-time__ip">{{ row.lastUsedIp }}</code>
          </span>
          <span v-else class="api-token-time__sub">{{ t('adminApiTokens.lastUsed.never') }}</span>
        </template>
      </UiDataTable>

      <p class="api-tokens__note api-tokens__guide">
        <a :href="GUIDE_URL" target="_blank" rel="noopener noreferrer" data-testid="api-tokens-guide">{{ t('adminApiTokens.guide') }}</a>
      </p>
    </UiSection>
  </div>
</template>

<script setup>
// 安全 → API tokens: an administrator's personal access tokens for
// automation (docs/guide/admin-api-tokens.md). A table of the caller's active
// tokens (name, last four characters and creation date, scope, status, expiry,
// last use and address) with chips for revoked and expired ones and, for a
// super administrator, everyone's; a "…" menu that revokes a token behind a
// danger confirmation; and a Create button (ApiTokenCreate) whose form takes
// the re-authentication and whose result dialog shows the new token once.
//
// Endpoints: GET|POST /api/v4/kernel/api-tokens, DELETE
// /api/v4/kernel/api-tokens/:id. The rows carry the owner's id only, so
// other administrators' tokens say "Administrator #7".
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { Ban, Copy, KeySquare, Plus, RotateCw } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { useUserStore } from '@/stores/user'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiFilterChips from '@/ui/UiFilterChips.vue'
import UiSection from '@/ui/UiSection.vue'
import { copyText } from '@/ui/composables/useClipboard'
import { useConfirm } from '@/ui/composables/useConfirm'
import { useFormat } from '@/ui/composables/useFormat'
import { useToast } from '@/ui/composables/useToast'
import ApiTokenCreate from './ApiTokenCreate.vue'
import { API_TOKEN_MAX_ACTIVE, tokenState } from './apiTokens'
import { useApiTokens } from './useApiTokens'

const GUIDE_URL = 'https://github.com/AnixOps/anix-control/blob/go_dev/docs/guide/admin-api-tokens.md'
const STATE_TONES = { active: 'success', expiring: 'warning', expired: 'neutral', revoked: 'neutral' }

const { t } = useAppI18n()
const format = useFormat()
const toast = useToast()
const confirm = useConfirm()
const userStore = useUserStore()

// The clock the page reads times against: set when the list loads and every
// minute, so "expires soon" and the relative times stay current.
const now = ref(Date.now())
let ticker = null

const list = useApiTokens({ me: () => userStore.userInfo?.id, now: () => now.value })
const { tokens, loading, error, showAll, showEnded, superAdmin, loadedAll, ownActive, limitReached, isMine } = list
const creator = ref(null)

async function load() {
  now.value = Date.now()
  return list.load()
}

const rows = computed(() => tokens.value.map(token => ({ ...token, state: tokenState(token, now.value) })))

const columns = computed(() => [
  { key: 'name', label: t('adminApiTokens.table.name'), primary: true, hideable: false, sortable: true, minWidth: 150, sortValue: row => row.name.toLowerCase() },
  ...(loadedAll.value ? [{ key: 'owner', label: t('adminApiTokens.table.owner'), hideable: false, nowrap: true, value: row => ownerText(row) }] : []),
  { key: 'scope', label: t('adminApiTokens.table.scope'), nowrap: true, value: row => t(`adminApiTokens.scopes.${row.scope}`) },
  { key: 'state', label: t('adminApiTokens.table.status'), secondary: true, nowrap: true, value: row => t(`adminApiTokens.states.${row.state}`) },
  { key: 'expiresAt', label: t('adminApiTokens.table.expires'), sortable: true, minWidth: 130, sortValue: row => row.expiresAt ?? Number.MAX_SAFE_INTEGER },
  { key: 'lastUsedAt', label: t('adminApiTokens.table.lastUsed'), sortable: true, minWidth: 140, firstDirection: 'desc', sortValue: row => row.lastUsedAt ?? 0 }
])

// The two switches of the list as toggle chips: revoked and expired tokens, and
// (super administrators) everyone's. Turning one on asks the route again.
const filterChips = computed(() => [
  { value: 'ended', label: t('adminApiTokens.filters.ended') },
  ...(superAdmin.value === false ? [] : [{ value: 'all', label: t('adminApiTokens.filters.all') }])
])
const filters = computed({
  get: () => [...(showEnded.value ? ['ended'] : []), ...(showAll.value ? ['all'] : [])],
  set: (value) => {
    showEnded.value = value.includes('ended')
    showAll.value = value.includes('all')
  }
})

function ownerText(row) {
  if (isMine(row)) return t('adminApiTokens.owner.you')
  return row.ownerEmail || t('adminApiTokens.owner.admin', { id: row.userId })
}

function revokeReason(row) {
  return row.revokeReason ? t(`adminApiTokens.revokeReasons.${row.revokeReason}`) : t('adminApiTokens.revokeReasons.unknown')
}

// "in 3 months" / "Expired 2 days ago" / "Revoked 3 days ago", with the exact
// time on hover and, where it adds something, under the relative one.
function expiryText(row) {
  if (row.state === 'revoked') return t('adminApiTokens.expires.revoked', { when: format.relativeTime(row.revokedAt, { now: now.value }) })
  if (row.expiresAt === null) return t('adminApiTokens.expires.never')
  const when = format.relativeTime(row.expiresAt, { now: now.value })
  return row.state === 'expired' ? t('adminApiTokens.expires.expired', { when }) : when
}

function expirySubtext(row) {
  if (row.state === 'revoked') return revokeReason(row)
  return row.expiresAt === null ? '' : format.dateTime(row.expiresAt)
}

const quotaText = computed(() => {
  if (ownActive.value === null) return ''
  return limitReached.value
    ? t('adminApiTokens.quotaReached', { max: API_TOKEN_MAX_ACTIVE })
    : t('adminApiTokens.quota', { used: ownActive.value, max: API_TOKEN_MAX_ACTIVE })
})

const rowActions = row => [
  { key: 'copy-id', label: t('adminApiTokens.actions.copyId'), icon: Copy, onSelect: () => copyId(row) },
  { key: 'revoke', label: t('adminApiTokens.actions.revoke'), icon: Ban, danger: true, separatorBefore: true, hidden: row.state === 'revoked' || row.state === 'expired', onSelect: () => revoke(row) }
]

async function copyId(row) {
  if (await copyText(row.id)) toast.success(t('adminApiTokens.copied'))
  else toast.error(t('adminApiTokens.copyFailed'))
}

// Revoking ends the token at once and cannot be undone, so it asks first; a
// failure shows inside the confirmation.
async function revoke(row) {
  let changed = true
  const confirmed = await confirm({
    title: t('adminApiTokens.revoke.title', { name: row.name }),
    message: isMine(row) ? t('adminApiTokens.revoke.message') : t('adminApiTokens.revoke.messageOther', { id: row.userId }),
    confirmLabel: t('adminApiTokens.revoke.action'),
    tone: 'danger',
    onConfirm: async () => {
      try {
        changed = (await list.revoke(row)).changed
      } catch (cause) {
        const kind = cause?.refusal?.kind
        throw new Error(kind === 'not_found'
          ? t('adminApiTokens.revoke.gone')
          : t('adminApiTokens.revoke.failed', { message: cause?.message || '' }))
      }
    }
  })
  if (!confirmed) return
  toast.success(t(changed ? 'adminApiTokens.revoke.done' : 'adminApiTokens.revoke.already', { name: row.name }))
  await load()
}

// A token was created (the record, never the secret): the list shows it.
function onCreated() {
  load()
}

onMounted(() => {
  load()
  ticker = setInterval(() => { now.value = Date.now() }, 60_000)
})
onBeforeUnmount(() => {
  if (ticker) clearInterval(ticker)
})
</script>

<style scoped>
.api-tokens {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
  min-width: 0;
}

.api-tokens__quota {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.api-tokens__quota.is-full {
  color: var(--label-1);
  font-weight: var(--weight-semibold);
}

.api-tokens__note {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.api-tokens__guide a {
  color: var(--accent);
}

.api-token-name {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  min-width: 0;
}

.api-token-name__text {
  overflow-wrap: break-word;
  font-weight: var(--weight-semibold);
}

.api-token-name__created {
  color: var(--label-2);
  font-size: var(--type-caption-size);
  font-weight: var(--weight-regular);
}

.api-token-name__hint,
.api-token-time__ip {
  color: var(--label-2);
  font-family: var(--font-mono);
  font-size: var(--type-caption-size);
  font-weight: var(--weight-regular);
}

.api-token-time {
  display: inline-flex;
  flex-direction: column;
  gap: var(--space-1);
}

.api-token-time__sub {
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

.api-token-time__ip {
  overflow-wrap: anywhere;
}
</style>
