<template>
  <div class="page-shell invite-codes-page">
    <div class="page-toolbar">
      <div>
        <h1>{{ t('adminInviteCodes.title') }}</h1>
        <p>{{ t('adminInviteCodes.subtitle') }}</p>
      </div>
      <router-link v-if="isCommercial" class="btn btn-secondary" to="/admin/invite" data-testid="invite-rewards-link">
        {{ t('adminInviteCodes.rewardsLink') }}
      </router-link>
    </div>

    <p class="registration-hint" data-testid="registration-hint">
      {{ requireInvite ? t('adminInviteCodes.registration.required') : t('adminInviteCodes.registration.optional') }}
    </p>

    <section class="section-panel generate-panel">
      <h3>{{ t('adminInviteCodes.generate.title') }}</h3>
      <div class="form-row">
        <div class="form-group">
          <label for="invite-code-count">{{ t('adminInviteCodes.generate.count') }}</label>
          <input id="invite-code-count" v-model.number="form.count" type="number" min="1" :max="maxBatch" />
        </div>
        <div class="form-group">
          <label for="invite-code-expire">{{ t('adminInviteCodes.generate.expireDays') }}</label>
          <input
            id="invite-code-expire"
            v-model="form.expireDays"
            type="number"
            min="0"
            :placeholder="t('adminInviteCodes.generate.expireDaysPlaceholder')"
          />
          <p class="help-text">{{ t('adminInviteCodes.generate.expireDaysHelp') }}</p>
        </div>
      </div>
      <div class="form-actions">
        <button class="btn btn-primary" data-testid="generate-invite-codes" :disabled="generating" @click="generate">
          {{ t('adminInviteCodes.generate.submit') }}
        </button>
      </div>
      <div v-if="generated.length" class="generated-codes" data-testid="generated-codes">
        <span>{{ t('adminInviteCodes.generate.created', { count: generated.length }) }}</span>
        <code v-for="item in generated" :key="item.id">{{ item.code }}</code>
        <button class="btn btn-sm btn-secondary" @click="copy(generated.map(item => item.code).join('\n'))">
          {{ t('adminInviteCodes.actions.copyAll') }}
        </button>
      </div>
    </section>

    <section class="section-panel data-panel">
      <div class="toolbar">
        <select v-model="filter" data-testid="invite-code-filter" @change="reload">
          <option value="">{{ t('adminInviteCodes.filters.all') }}</option>
          <option value="unused">{{ t('adminInviteCodes.status.unused') }}</option>
          <option value="used">{{ t('adminInviteCodes.status.used') }}</option>
          <option value="expired">{{ t('adminInviteCodes.status.expired') }}</option>
        </select>
        <button class="btn btn-secondary" @click="fetchCodes">{{ t('adminInviteCodes.actions.refresh') }}</button>
      </div>
      <div class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <th>{{ t('adminInviteCodes.table.code') }}</th>
              <th>{{ t('adminInviteCodes.table.owner') }}</th>
              <th>{{ t('adminInviteCodes.table.status') }}</th>
              <th>{{ t('adminInviteCodes.table.usedBy') }}</th>
              <th>{{ t('adminInviteCodes.table.expiresAt') }}</th>
              <th>{{ t('adminInviteCodes.table.createdAt') }}</th>
              <th>{{ t('adminInviteCodes.table.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in codes" :key="item.id" data-testid="invite-code-row">
              <td><code>{{ item.code }}</code></td>
              <td>{{ item.user_id ? t('adminInviteCodes.owner.user', { id: item.user_id }) : t('adminInviteCodes.owner.admin') }}</td>
              <td>
                <span :class="['status-badge', `status-${codeState(item)}`]">{{ t(`adminInviteCodes.status.${codeState(item)}`) }}</span>
              </td>
              <td>{{ item.used_by ? `#${item.used_by}` : '-' }}</td>
              <td>{{ item.expired_at ? formatDateTime(item.expired_at) : t('adminInviteCodes.never') }}</td>
              <td>{{ item.created_at ? formatDateTime(item.created_at) : '-' }}</td>
              <td>
                <div class="action-buttons">
                  <button class="btn btn-sm btn-secondary" @click="copy(item.code)">
                    {{ t('adminInviteCodes.actions.copy') }}
                  </button>
                  <button
                    v-if="item.status === 0"
                    class="btn btn-sm btn-danger"
                    data-testid="revoke-invite-code"
                    @click="revoke(item)"
                  >
                    {{ t('adminInviteCodes.actions.revoke') }}
                  </button>
                </div>
              </td>
            </tr>
            <tr v-if="codes.length === 0">
              <td colspan="7" class="empty-row">{{ t('adminInviteCodes.empty') }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <div class="pager">
        <button class="btn btn-sm btn-secondary" :disabled="page <= 1" @click="goTo(page - 1)">
          {{ t('adminInviteCodes.pager.previous') }}
        </button>
        <span>{{ t('adminInviteCodes.pager.summary', { page, pages: totalPages, total }) }}</span>
        <button class="btn btn-sm btn-secondary" :disabled="page >= totalPages" @click="goTo(page + 1)">
          {{ t('adminInviteCodes.pager.next') }}
        </button>
      </div>
    </section>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { generateInviteCodes, getInviteCodes, revokeInviteCode } from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import { useEdition } from '@/composables/useEdition'

// Invite codes are registration control and served in every edition
// (identity-platform). Commissions, withdrawals and statistics are the
// commercial Invite Rewards page.
const { t, formatDateTime } = useAppI18n()
const { isCommercial, requireInvite, loadEdition } = useEdition()

const maxBatch = 50
const pageSize = 20

const codes = ref([])
const total = ref(0)
const page = ref(1)
const filter = ref('')
const generating = ref(false)
const generated = ref([])
const form = reactive({ count: 1, expireDays: '' })

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))

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
  try {
    const params = { page: page.value, page_size: pageSize }
    if (filter.value) params.status = filter.value
    const data = readPanel(await getInviteCodes(params))
    codes.value = Array.isArray(data.list) ? data.list : []
    total.value = Number(data.total || 0)
  } catch (error) {
    console.error(t('adminInviteCodes.messages.fetchFailed'), error)
    codes.value = []
    total.value = 0
  }
}

function reload() {
  page.value = 1
  return fetchCodes()
}

function goTo(next) {
  page.value = Math.min(Math.max(1, next), totalPages.value)
  return fetchCodes()
}

async function generate() {
  const count = Number(form.count)
  if (!Number.isInteger(count) || count < 1 || count > maxBatch) {
    window.alert(t('adminInviteCodes.messages.countRange', { max: maxBatch }))
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
    await reload()
  } catch (error) {
    window.alert(t('adminInviteCodes.messages.generateFailed', { message: errorMessage(error) }))
  } finally {
    generating.value = false
  }
}

async function revoke(item) {
  if (!window.confirm(t('adminInviteCodes.messages.revokeConfirm', { code: item.code }))) return
  try {
    readPanel(await revokeInviteCode(item.id))
    await fetchCodes()
  } catch (error) {
    window.alert(t('adminInviteCodes.messages.revokeFailed', { message: errorMessage(error) }))
  }
}

// copy uses the clipboard API where the page is a secure context and a
// hidden textarea otherwise (plain HTTP).
async function copy(text) {
  let copied = false
  if (navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text)
      copied = true
    } catch {
      copied = false
    }
  }
  if (!copied) {
    try {
      const area = document.createElement('textarea')
      area.value = text
      area.style.position = 'fixed'
      area.style.top = '-9999px'
      document.body.appendChild(area)
      area.select()
      copied = document.execCommand('copy')
      document.body.removeChild(area)
    } catch {
      copied = false
    }
  }
  window.alert(copied ? t('adminInviteCodes.messages.copied') : t('adminInviteCodes.messages.copyFailed'))
}

onMounted(() => {
  loadEdition()
  fetchCodes()
})
</script>

<style scoped>
.registration-hint {
  color: var(--text-secondary);
  margin-bottom: 16px;
}

.generate-panel {
  margin-bottom: 20px;
}

.generate-panel h3 {
  margin-bottom: 12px;
}

.help-text {
  color: var(--text-secondary);
  font-size: 12px;
  margin-top: 4px;
}

.generated-codes {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  margin-top: 12px;
}

.toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 12px;
}

.pager {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 12px;
  margin-top: 12px;
}

.status-unused {
  background: rgba(34, 197, 94, 0.15);
  color: #22c55e;
}

.status-used {
  background: rgba(148, 163, 184, 0.15);
  color: var(--text-secondary);
}

.status-expired {
  background: rgba(239, 68, 68, 0.15);
  color: #ef4444;
}
</style>
