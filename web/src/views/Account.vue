<template>
  <div class="account-page">
    <UiPageHeader :title="t('shell.accountPage.title')" :description="t('shell.accountPage.subtitle')" />

    <UiGroupedList :title="t('shell.accountPage.profile.title')" heading-tag="h2" data-account-section="profile">
      <UiGroupedListRow :label="t('shell.accountPage.profile.email')" :value="profile.email || '—'" />
      <UiGroupedListRow v-if="profile.id" :label="t('shell.accountPage.profile.id')" :value="String(profile.id)" />
      <UiGroupedListRow :label="t('shell.accountPage.profile.role')" :value="profile.isAdmin ? t('shell.account.roleAdmin') : t('shell.account.roleUser')" />
    </UiGroupedList>

    <UiGroupedList
      :title="t('shell.accountPage.mfa.title')"
      :footer="t('shell.accountPage.mfa.description')"
      heading-tag="h2"
      data-account-section="mfa"
    >
      <UiGroupedListRow v-if="mfa.state === 'loading'" :label="t('shell.accountPage.mfa.status')">
        <UiSkeleton variant="text" :lines="1" class="account-page__skeleton" />
      </UiGroupedListRow>
      <UiGroupedListRow v-else-if="mfa.state === 'failed'" :label="t('shell.accountPage.mfa.loadFailed')">
        <UiButton size="sm" @click="loadMfa">{{ t('shell.accountPage.mfa.retry') }}</UiButton>
      </UiGroupedListRow>
      <template v-else>
        <UiGroupedListRow :label="t('shell.accountPage.mfa.status')">
          <template #value>
            <UiBadge
              :tone="mfa.enabled ? 'success' : 'neutral'"
              :label="mfa.enabled ? t('shell.accountPage.mfa.on') : t('shell.accountPage.mfa.off')"
              data-mfa-status
            />
          </template>
          <UiButton v-if="!mfa.enabled" variant="primary" size="sm" data-mfa-enable @click="startSetup">{{ t('shell.accountPage.mfa.enable') }}</UiButton>
          <UiButton v-else variant="danger-soft" size="sm" data-mfa-disable @click="openDisable">{{ t('shell.accountPage.mfa.disable') }}</UiButton>
        </UiGroupedListRow>
        <UiGroupedListRow
          v-if="mfa.enabled"
          :label="t('shell.accountPage.mfa.backupCodes')"
          :value="mfa.remaining > 0 ? t('shell.accountPage.mfa.remaining', { count: mfa.remaining }) : t('shell.accountPage.mfa.none')"
        >
          <UiButton size="sm" data-mfa-regenerate @click="regenerate">{{ t('shell.accountPage.mfa.regenerate') }}</UiButton>
        </UiGroupedListRow>
        <UiGroupedListRow v-if="mfa.enabled && mfa.lastUsed" :label="t('shell.accountPage.mfa.lastUsed')" :value="format.dateTime(mfa.lastUsed)" />
      </template>
    </UiGroupedList>

    <UiGroupedList :title="t('shell.accountPage.language.title')" heading-tag="h2" data-account-section="language">
      <UiGroupedListRow :label="t('shell.accountPage.language.label')">
        <UiSegmentedControl
          :model-value="currentLocale"
          :options="localeOptions.map(option => ({ value: option.value, label: option.nativeLabel }))"
          :aria-label="t('shell.accountPage.language.label')"
          size="sm"
          @update:model-value="switchLocale"
        />
      </UiGroupedListRow>
    </UiGroupedList>

    <UiGroupedList :title="t('shell.accountPage.appearance.title')" heading-tag="h2" data-account-section="appearance">
      <UiGroupedListRow :label="t('shell.accountPage.appearance.label')">
        <UiSegmentedControl
          :model-value="themePreference"
          :options="themeOptions"
          :aria-label="t('shell.accountPage.appearance.label')"
          size="sm"
          @update:model-value="setThemePreference"
        />
      </UiGroupedListRow>
    </UiGroupedList>

    <!-- Turn on: setup key, then the first code. -->
    <UiDialog
      v-model:open="setup.open"
      :title="t('shell.accountPage.mfa.setup.title')"
      :description="t('shell.accountPage.mfa.setup.description')"
      :dismissible="!setup.busy"
      initial-focus="input[autocomplete='one-time-code']"
      data-mfa-setup
    >
      <p v-if="setup.preparing" class="account-page__muted">{{ t('shell.accountPage.mfa.setup.preparing') }}</p>
      <p v-else-if="setup.loadError" class="account-page__error" role="alert">{{ setup.loadError }}</p>
      <form v-else id="mfa-setup-form" class="account-page__form" novalidate @submit.prevent="confirmSetup">
        <UiCopyField :value="setup.secret" :label="t('shell.accountPage.mfa.setup.secret')" :help="t('shell.accountPage.mfa.setup.secretHelp')" size="md" />
        <a v-if="setup.url" class="account-page__link" :href="setup.url">{{ t('shell.accountPage.mfa.setup.openApp') }}</a>
        <UiTextField
          v-model="setup.code"
          :label="t('shell.accountPage.mfa.setup.code')"
          :help="t('shell.accountPage.mfa.setup.codeHelp')"
          :error="setup.error"
          inputmode="numeric"
          autocomplete="one-time-code"
          maxlength="6"
          required
        />
      </form>
      <template #footer="{ close }">
        <UiButton :disabled="setup.busy" @click="close">{{ t('common.actions.cancel') }}</UiButton>
        <UiButton
          variant="primary"
          type="submit"
          form="mfa-setup-form"
          :loading="setup.busy"
          :disabled="setup.preparing || Boolean(setup.loadError)"
        >
          {{ t('shell.accountPage.mfa.setup.confirm') }}
        </UiButton>
      </template>
    </UiDialog>

    <!-- Backup codes, shown once after turning on or regenerating. -->
    <UiDialog
      v-model:open="codes.open"
      :title="t('shell.accountPage.mfa.backup.title')"
      :description="t('shell.accountPage.mfa.backup.description')"
      :close-on-scrim="false"
      data-mfa-codes
    >
      <ul class="account-page__codes" data-mfa-code-list>
        <li v-for="code in codes.list" :key="code">{{ code }}</li>
      </ul>
      <template #footer="{ close }">
        <UiButton @click="copyCodes">{{ codes.copied ? t('ui.actions.copied') : t('shell.accountPage.mfa.backup.copyAll') }}</UiButton>
        <UiButton variant="primary" @click="close">{{ t('shell.accountPage.mfa.backup.done') }}</UiButton>
      </template>
    </UiDialog>

    <!-- Turn off: current password. -->
    <UiDialog
      v-model:open="disable.open"
      size="sm"
      :title="t('shell.accountPage.mfa.disableDialog.title')"
      :description="t('shell.accountPage.mfa.disableDialog.description')"
      :dismissible="!disable.busy"
      data-mfa-disable-dialog
    >
      <form id="mfa-disable-form" novalidate @submit.prevent="confirmDisable">
        <UiPasswordField
          v-model="disable.password"
          :label="t('shell.accountPage.mfa.disableDialog.password')"
          :error="disable.error"
          required
        />
      </form>
      <template #footer="{ close }">
        <UiButton :disabled="disable.busy" @click="close">{{ t('common.actions.cancel') }}</UiButton>
        <UiButton variant="danger" type="submit" form="mfa-disable-form" :loading="disable.busy">
          {{ t('shell.accountPage.mfa.disableDialog.confirm') }}
        </UiButton>
      </template>
    </UiDialog>
  </div>
</template>

<script setup>
// 账户 (plan §8.1, shell phase U3): profile, two-factor authentication,
// language and appearance, for users (/user/account) and admins
// (/admin/account). Only what the backend serves: the profile is read-only
// (GET /user/profile) and two-factor uses /user/mfa/*. Password change and
// signed-in devices have no endpoint yet, so they are not here.
import { computed, onMounted, reactive } from 'vue'
import { Monitor, Moon, Sun } from '@lucide/vue'
import { disableMfa, enableTotp, getMfaStatus, regenerateBackupCodes, setupTotp } from '@/api/user'
import { useUserStore } from '@/stores/user'
import { useAppI18n } from '@/composables/useAppI18n'
import { useTheme } from '@/composables/useTheme'
import {
  UiBadge, UiButton, UiCopyField, UiDialog, UiGroupedList, UiGroupedListRow, UiPageHeader, UiPasswordField,
  UiSegmentedControl, UiSkeleton, UiTextField, copyText, useConfirm, useFormat, useToast
} from '@/ui'

const userStore = useUserStore()
const { t, currentLocale, localeOptions, switchLocale } = useAppI18n()
const { themePreference, setThemePreference } = useTheme()
const format = useFormat()
const toast = useToast()
const confirm = useConfirm()

const profile = computed(() => ({
  email: userStore.userInfo?.email || '',
  id: userStore.userInfo?.id || '',
  isAdmin: userStore.isAdmin
}))

const themeOptions = computed(() => [
  { value: 'system', label: t('shell.account.themes.system'), icon: Monitor },
  { value: 'light', label: t('shell.account.themes.light'), icon: Sun },
  { value: 'dark', label: t('shell.account.themes.dark'), icon: Moon }
])

// The panel envelope: { code, msg, data }; anything else is the body itself.
function unwrap(body) {
  if (body && typeof body.code === 'number') {
    if (body.code !== 0) throw new Error(body.msg || 'error')
    return body.data || {}
  }
  return body?.data ?? body ?? {}
}

function messageOf(error) {
  return error?.response?.data?.msg || error?.message || String(error)
}

const mfa = reactive({ state: 'loading', enabled: false, remaining: 0, lastUsed: null })

async function loadMfa() {
  mfa.state = 'loading'
  try {
    const data = unwrap(await getMfaStatus())
    mfa.enabled = data.enabled === true
    mfa.remaining = Number(data.remaining_codes) || 0
    mfa.lastUsed = data.last_used || null
    mfa.state = 'ready'
  } catch {
    mfa.state = 'failed'
  }
}

const codes = reactive({ open: false, list: [], copied: false })

function showCodes(list) {
  codes.list = Array.isArray(list) ? list.map(String) : []
  codes.copied = false
  codes.open = codes.list.length > 0
}

async function copyCodes() {
  codes.copied = await copyText(codes.list.join('\n'))
}

// --- turn on -------------------------------------------------------------
const setup = reactive({ open: false, preparing: false, busy: false, secret: '', url: '', backupCodes: [], code: '', error: '', loadError: '' })

async function startSetup() {
  Object.assign(setup, { open: true, preparing: true, busy: false, secret: '', url: '', backupCodes: [], code: '', error: '', loadError: '' })
  try {
    const data = unwrap(await setupTotp())
    setup.secret = String(data.secret || '')
    // Only an otpauth: link may become a link (the value comes from the API).
    const url = String(data.url || data.qr_code || '')
    setup.url = url.startsWith('otpauth://') ? url : ''
    setup.backupCodes = Array.isArray(data.backup_codes) ? data.backup_codes : []
  } catch (error) {
    setup.loadError = t('shell.accountPage.mfa.errors.failed', { message: messageOf(error) })
  } finally {
    setup.preparing = false
  }
}

async function confirmSetup() {
  const code = String(setup.code || '').replace(/\s+/g, '')
  if (!/^\d{6}$/.test(code)) {
    setup.error = t('shell.accountPage.mfa.errors.code')
    return
  }
  setup.error = ''
  setup.busy = true
  try {
    unwrap(await enableTotp(code))
    setup.open = false
    toast.success(t('shell.accountPage.mfa.setup.enabled'))
    showCodes(setup.backupCodes)
    await loadMfa()
  } catch (error) {
    setup.error = t('shell.accountPage.mfa.errors.failed', { message: messageOf(error) })
  } finally {
    setup.busy = false
  }
}

// --- turn off ------------------------------------------------------------
const disable = reactive({ open: false, busy: false, password: '', error: '' })

function openDisable() {
  Object.assign(disable, { open: true, busy: false, password: '', error: '' })
}

async function confirmDisable() {
  if (!disable.password) {
    disable.error = t('shell.accountPage.mfa.errors.password')
    return
  }
  disable.error = ''
  disable.busy = true
  try {
    unwrap(await disableMfa(disable.password))
    disable.open = false
    disable.password = ''
    toast.success(t('shell.accountPage.mfa.disableDialog.done'))
    await loadMfa()
  } catch (error) {
    disable.error = t('shell.accountPage.mfa.errors.failed', { message: messageOf(error) })
  } finally {
    disable.busy = false
  }
}

// --- new backup codes ----------------------------------------------------
async function regenerate() {
  let fresh = []
  const done = await confirm({
    title: t('shell.accountPage.mfa.regenerateDialog.title'),
    message: t('shell.accountPage.mfa.regenerateDialog.description'),
    confirmLabel: t('shell.accountPage.mfa.regenerateDialog.confirm'),
    onConfirm: async () => {
      try {
        fresh = unwrap(await regenerateBackupCodes()).backup_codes || []
      } catch (error) {
        throw new Error(t('shell.accountPage.mfa.errors.failed', { message: messageOf(error) }))
      }
    }
  })
  if (done) {
    showCodes(fresh)
    await loadMfa()
  }
}

onMounted(loadMfa)
</script>

<style scoped>
.account-page {
  display: flex;
  flex-direction: column;
  gap: var(--space-10);
  width: 100%;
  max-width: var(--size-content-read);
  margin: 0 auto;
}

.account-page__skeleton {
  width: 96px;
}

.account-page__form {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.account-page__link {
  align-self: flex-start;
  font-size: var(--type-callout-size);
}

.account-page__muted {
  color: var(--label-2);
}

.account-page__error {
  color: var(--danger);
}

.account-page__codes {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--space-2) var(--space-6);
  padding: var(--space-4) var(--space-5);
  border-radius: var(--radius-sm);
  background: var(--bg-grouped);
  font-family: var(--font-mono);
  font-variant-numeric: tabular-nums;
  list-style: none;
}
</style>
