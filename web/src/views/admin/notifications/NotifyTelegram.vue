<template>
  <div class="notify-panel" data-notify-panel="telegram">
    <UiSection :title="t('adminNotify.telegram.bot.title')" :description="t('adminNotify.telegram.bot.description')">
      <UiSkeleton v-if="showSkeleton" variant="card" :label="t('adminSettings.loading')" />
      <UiErrorState
        v-else-if="botLoadError"
        compact
        heading-tag="h3"
        :title="t('adminNotify.telegram.bot.loadFailed')"
        :error="botLoadError"
        @retry="fetchBotConfig"
      />
      <template v-else-if="!botLoading">
        <UiGroupedList>
          <UiGroupedListRow stacked>
            <div class="form-grid notify-form">
              <UiPasswordField
                id="telegram-token"
                v-model="botConfig.token"
                class="form-grid__full"
                size="md"
                autocomplete="off"
                required
                :label="t('adminNotify.telegram.bot.token')"
                :help="t('adminNotify.telegram.bot.tokenHelp')"
              />
              <UiTextField
                id="telegram-admin-ids"
                v-model="adminIdsText"
                class="form-grid__full"
                size="md"
                inputmode="numeric"
                :label="t('adminNotify.telegram.bot.adminIds')"
                placeholder="123456789, 987654321"
                :help="t('adminNotify.telegram.bot.adminIdsHelp')"
                :error="adminIdsError"
              />
              <UiTextarea
                id="telegram-welcome"
                v-model="botConfig.welcome_message"
                class="form-grid__full"
                :rows="3"
                :label="t('adminNotify.telegram.bot.welcome')"
                :placeholder="t('adminNotify.telegram.bot.welcomePlaceholder')"
              />
            </div>
          </UiGroupedListRow>
        </UiGroupedList>

        <UiGroupedList :title="t('adminNotify.telegram.webhook.title')" :footer="t('adminNotify.telegram.webhook.help')">
          <UiGroupedListRow stacked>
            <div class="webhook">
              <UiCopyField
                v-if="webhookUrl"
                :value="webhookUrl"
                size="md"
                :label="t('adminNotify.telegram.webhook.url')"
              />
              <p v-else class="notify-note">{{ t('adminNotify.telegram.webhook.needsToken') }}</p>
              <div class="webhook__actions">
                <UiButton size="sm" :disabled="!webhookUrl || webhookBusy" data-test="telegram-webhook-set" @click="setWebhookConfig">{{ t('adminNotify.telegram.webhook.set') }}</UiButton>
                <UiButton size="sm" variant="danger-soft" :disabled="webhookBusy" data-test="telegram-webhook-delete" @click="deleteWebhookConfig">{{ t('adminNotify.telegram.webhook.delete') }}</UiButton>
              </div>
            </div>
          </UiGroupedListRow>
        </UiGroupedList>
      </template>
    </UiSection>

    <UiSection :title="t('adminNotify.telegram.send.title')" :description="t('adminNotify.telegram.send.description')">
      <form class="send-form" novalidate @submit.prevent="sendNotification">
        <UiSegmentedControl
          v-model="notifyForm.type"
          :aria-label="t('adminNotify.telegram.send.audience')"
          :options="audienceOptions"
        />
        <UiTextField
          v-if="notifyForm.type === 'single'"
          id="telegram-notify-id"
          v-model="notifyForm.telegram_id"
          inputmode="numeric"
          data-test="telegram-notify-id"
          required
          :label="t('adminNotify.telegram.send.telegramId')"
          placeholder="123456789"
          :help="t('adminNotify.telegram.send.telegramIdHelp')"
          :error="notifyErrors.telegram_id"
        />
        <UiTextarea
          id="telegram-notify-message"
          v-model="notifyForm.message"
          :rows="4"
          data-test="telegram-notify-message"
          required
          :label="t('adminNotify.telegram.send.message')"
          :placeholder="t('adminNotify.telegram.send.messagePlaceholder')"
          :help="t('adminNotify.telegram.send.markdown')"
          :error="notifyErrors.message"
        />
        <p v-if="notifyForm.type === 'broadcast'" class="notify-note">{{ t('adminNotify.telegram.send.broadcastNote') }}</p>
        <div class="send-form__actions">
          <UiButton type="submit" variant="primary" :icon="Send" :loading="sending" data-test="telegram-send">
            {{ notifyForm.type === 'broadcast' ? t('adminNotify.telegram.send.broadcast') : t('adminNotify.telegram.send.send') }}
          </UiButton>
        </div>
      </form>
    </UiSection>

    <UiSection :title="t('adminNotify.telegram.users.title')" :description="t('adminNotify.telegram.users.description')">
      <UiDataTable
        :columns="userColumns"
        :rows="filteredUsers"
        :label="t('adminNotify.telegram.users.title')"
        :row-label="user => user.user_email || String(user.telegram_id)"
        storage-key="admin.telegramUsers"
        :page-size="20"
        :loading="usersLoading"
        :error="usersError"
        :error-title="t('adminNotify.telegram.users.loadFailed')"
        :filtered="Boolean(userSearch)"
        :empty-icon="Users"
        :empty-title="t('adminNotify.telegram.users.empty')"
        :empty-description="t('adminNotify.telegram.users.emptyDescription')"
        state-heading-tag="h3"
        @retry="fetchUsers"
        @clear-filters="userSearch = ''"
      >
        <template #toolbar>
          <UiSearchField v-model="userSearch" class="list-page__search" :shortcut="false" :label="t('adminNotify.telegram.users.search')" />
        </template>
        <template #cell-notify_enabled="{ row }">
          <UiSwitch
            :model-value="Boolean(row.notify_enabled)"
            :aria-label="t('adminNotify.telegram.users.notifyNamed', { name: row.user_email || row.telegram_id })"
            @update:model-value="toggleUserNotify(row)"
          />
        </template>
      </UiDataTable>
    </UiSection>

    <UiSection :title="t('adminNotify.telegram.commands.title')" :description="t('adminNotify.telegram.commands.description')">
      <UiGroupedList>
        <UiGroupedListRow v-for="command in commands" :key="command.cmd" :label="command.desc">
          <template #leading><code class="command">{{ command.cmd }}</code></template>
        </UiGroupedListRow>
      </UiGroupedList>
    </UiSection>

    <SettingsSaveBar
      :visible="botDirty"
      :saving="botSaving"
      :invalid="Boolean(adminIdsError)"
      @save="saveBotConfig"
      @discard="discardBotConfig"
    />
  </div>
</template>

<script setup>
// 通知 → Telegram: the bot (token, administrator IDs, welcome message; saved
// with the save bar), its webhook, sending a message to one user (the
// channel's test send) or to every bound user, the bound users with their
// notification switch, and the bot's commands. Endpoints unchanged:
// GET/PUT /admin/telegram/bot, POST/DELETE /admin/telegram/webhook,
// POST /admin/telegram/notify, POST /admin/telegram/broadcast,
// GET /admin/telegram/users, PUT /admin/telegram/users/:id/notify.
import { computed, onMounted, ref, watch } from 'vue'
import { Send, Users } from '@lucide/vue'
import {
  broadcastTelegram,
  deleteTelegramWebhook,
  getTelegramBot,
  getTelegramUsers,
  sendTelegramNotification,
  setTelegramWebhook,
  updateTelegramBot,
  updateTelegramUserNotify
} from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import { useUnsavedChanges } from '@/composables/useUnsavedChanges'
import SettingsSaveBar from '@/components/admin/settings/SettingsSaveBar.vue'
import UiButton from '@/ui/UiButton.vue'
import UiCopyField from '@/ui/UiCopyField.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiErrorState from '@/ui/UiErrorState.vue'
import UiGroupedList from '@/ui/UiGroupedList.vue'
import UiGroupedListRow from '@/ui/UiGroupedListRow.vue'
import UiPasswordField from '@/ui/UiPasswordField.vue'
import UiSearchField from '@/ui/UiSearchField.vue'
import UiSection from '@/ui/UiSection.vue'
import UiSegmentedControl from '@/ui/UiSegmentedControl.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import UiSwitch from '@/ui/UiSwitch.vue'
import UiTextField from '@/ui/UiTextField.vue'
import UiTextarea from '@/ui/UiTextarea.vue'
import { useConfirm } from '@/ui/composables/useConfirm'
import { useDelayedLoading } from '@/ui/composables/useDelayedLoading'
import { useFormat } from '@/ui/composables/useFormat'
import { useToast } from '@/ui/composables/useToast'
import { ensureNotifySuccess, notifyErrorText, readNotifyPayload } from './notifyResponse'

const { t } = useAppI18n()
const format = useFormat()
const toast = useToast()
const confirm = useConfirm()

const botConfig = ref(emptyBotConfig())
const savedBotConfig = ref(JSON.stringify(botConfig.value))
const adminIdsText = ref('')
const botLoading = ref(false)
const botLoadError = ref(null)
const botSaving = ref(false)
const webhookBusy = ref(false)
const showSkeleton = useDelayedLoading(botLoading)

const users = ref([])
const usersLoading = ref(false)
const usersError = ref(null)
const userSearch = ref('')

const notifyForm = ref({ type: 'single', telegram_id: '', message: '' })
const notifyErrors = ref({ telegram_id: '', message: '' })
const sending = ref(false)

function emptyBotConfig() {
  return { token: '', admin_ids: [], welcome_message: '' }
}

const commands = computed(() => ['start', 'bind', 'unbind', 'info', 'sub', 'renew', 'ticket', 'help']
  .map(name => ({ cmd: `/${name}`, desc: t(`adminNotify.telegram.commands.items.${name}`) })))

const audienceOptions = computed(() => [
  { value: 'single', label: t('adminNotify.telegram.send.single') },
  { value: 'broadcast', label: t('adminNotify.telegram.send.everyone') }
])

// Administrator IDs: typed as text, validated as you type, stored as numbers.
const adminIdTokens = computed(() => adminIdsText.value.split(/[,，\s]+/).map(item => item.trim()).filter(Boolean))
const adminIdsError = computed(() => {
  const invalid = adminIdTokens.value.filter(item => !/^-?\d+$/.test(item))
  return invalid.length ? t('adminNotify.telegram.bot.adminIdsInvalid', { ids: invalid.join(', ') }) : ''
})
watch(adminIdTokens, tokens => {
  if (adminIdsError.value) return
  botConfig.value.admin_ids = tokens.map(item => parseInt(item, 10))
})

const botDirty = computed(() => JSON.stringify(botConfig.value) !== savedBotConfig.value)

const webhookUrl = computed(() => {
  if (!botConfig.value.token) return ''
  return `${window.location.origin}/api/v2/telegram/webhook`
})

const filteredUsers = computed(() => {
  if (!userSearch.value) return users.value
  const search = userSearch.value.toLowerCase()
  return users.value.filter(user => (
    user.telegram_id?.toString().includes(search) ||
    user.user_email?.toLowerCase().includes(search)
  ))
})

const userColumns = computed(() => [
  { key: 'user_email', label: t('adminNotify.telegram.users.email'), primary: true, sortable: true, truncate: true, maxWidth: 280, value: user => user.user_email || '—' },
  { key: 'telegram_id', label: t('adminNotify.telegram.users.telegramId'), secondary: true, numeric: true },
  { key: 'user_id', label: t('adminNotify.telegram.users.userId'), numeric: true, breakpoint: 'md' },
  { key: 'created_at', label: t('adminNotify.telegram.users.boundAt'), nowrap: true, breakpoint: 'lg', sortable: true, firstDirection: 'desc', format: value => (value ? format.dateTime(value) : '—') },
  { key: 'notify_enabled', label: t('adminNotify.telegram.users.notify'), hideable: false }
])

function commitBotConfig(config) {
  botConfig.value = config
  savedBotConfig.value = JSON.stringify(config)
  adminIdsText.value = (config.admin_ids || []).join(', ')
}

const fetchBotConfig = async () => {
  botLoading.value = true
  botLoadError.value = null
  try {
    const res = await getTelegramBot()
    const payload = readNotifyPayload(res, t('adminNotify.telegram.bot.loadFailed'))
    commitBotConfig(Object.keys(payload).length > 0
      ? { ...emptyBotConfig(), ...payload, admin_ids: Array.isArray(payload.admin_ids) ? payload.admin_ids : [] }
      : emptyBotConfig())
  } catch (error) {
    botLoadError.value = notifyErrorText(error) || t('adminNotify.telegram.bot.loadFailed')
  } finally {
    botLoading.value = false
  }
}

const saveBotConfig = async () => {
  if (botSaving.value || adminIdsError.value) return
  botSaving.value = true
  try {
    ensureNotifySuccess(await updateTelegramBot(botConfig.value), t('adminNotify.telegram.bot.saveFailedShort'))
    commitBotConfig({ ...botConfig.value })
    toast.success(t('adminNotify.telegram.bot.saved'))
  } catch (error) {
    toast.error(t('adminNotify.telegram.bot.saveFailed', { message: notifyErrorText(error) || t('adminNotify.telegram.bot.saveFailedShort') }))
  } finally {
    botSaving.value = false
  }
}

function discardBotConfig() {
  commitBotConfig(JSON.parse(savedBotConfig.value))
}

useUnsavedChanges(botDirty, { discard: discardBotConfig })

const setWebhookConfig = async () => {
  webhookBusy.value = true
  try {
    ensureNotifySuccess(await setTelegramWebhook(webhookUrl.value || undefined), t('adminNotify.telegram.webhook.setFailedShort'))
    toast.success(t('adminNotify.telegram.webhook.setDone'))
    return true
  } catch (error) {
    toast.error(t('adminNotify.telegram.webhook.setFailed', { message: notifyErrorText(error) || t('adminNotify.telegram.webhook.setFailedShort') }))
    return false
  } finally {
    webhookBusy.value = false
  }
}

const deleteWebhookConfig = async () => {
  webhookBusy.value = true
  try {
    ensureNotifySuccess(await deleteTelegramWebhook(), t('adminNotify.telegram.webhook.deleteFailedShort'))
    // Setting the webhook again is the inverse: offer it as 撤销.
    toast.success(t('adminNotify.telegram.webhook.deleted'), { undo: () => setWebhookConfig() })
  } catch (error) {
    toast.error(t('adminNotify.telegram.webhook.deleteFailed', { message: notifyErrorText(error) || t('adminNotify.telegram.webhook.deleteFailedShort') }))
  } finally {
    webhookBusy.value = false
  }
}

const fetchUsers = async () => {
  usersLoading.value = true
  usersError.value = null
  try {
    const res = await getTelegramUsers({ all: true })
    const payload = readNotifyPayload(res, t('adminNotify.telegram.users.loadFailed'))
    users.value = payload.list || (Array.isArray(payload) ? payload : [])
  } catch (error) {
    users.value = []
    usersError.value = notifyErrorText(error) || t('adminNotify.telegram.users.loadFailed')
  } finally {
    usersLoading.value = false
  }
}

const toggleUserNotify = async (user) => {
  try {
    const nextEnabled = !user.notify_enabled
    const res = await updateTelegramUserNotify(user.id, { notify_enabled: nextEnabled })
    const updated = readNotifyPayload(res, t('adminNotify.telegram.users.toggleFailedShort'))
    user.notify_enabled = !!updated.notify_enabled
    user.notify_expire = !!updated.notify_expire
    user.notify_traffic = !!updated.notify_traffic
    user.notify_ticket = !!updated.notify_ticket
  } catch (error) {
    toast.error(t('adminNotify.telegram.users.toggleFailed', { message: notifyErrorText(error) || t('adminNotify.telegram.users.toggleFailedShort') }))
  }
}

const sendNotification = async () => {
  if (sending.value) return
  const broadcast = notifyForm.value.type === 'broadcast'
  notifyErrors.value = {
    telegram_id: !broadcast && !notifyForm.value.telegram_id ? t('adminNotify.telegram.send.telegramIdRequired') : '',
    message: notifyForm.value.message ? '' : t('adminNotify.telegram.send.messageRequired')
  }
  if (notifyErrors.value.telegram_id || notifyErrors.value.message) return
  // A broadcast reaches every bound user and cannot be recalled: ask first.
  if (broadcast && !(await confirm({
    title: t('adminNotify.telegram.send.broadcastTitle'),
    message: t('adminNotify.telegram.send.broadcastMessage'),
    confirmLabel: t('adminNotify.telegram.send.broadcast')
  }))) return
  sending.value = true
  try {
    if (broadcast) {
      const res = await broadcastTelegram(notifyForm.value.message)
      const payload = readNotifyPayload(res, t('adminNotify.telegram.send.failedShort'))
      const counts = { success: payload.success || 0, failed: payload.failed || 0 }
      if (counts.failed > 0) toast.warning(t('adminNotify.telegram.send.broadcastDone', counts))
      else toast.success(t('adminNotify.telegram.send.broadcastDone', counts))
    } else {
      ensureNotifySuccess(
        await sendTelegramNotification({
          telegram_id: notifyForm.value.telegram_id,
          message: notifyForm.value.message
        }),
        t('adminNotify.telegram.send.failedShort')
      )
      toast.success(t('adminNotify.telegram.send.sent'))
    }
    notifyForm.value.message = ''
  } catch (error) {
    toast.error(t('adminNotify.telegram.send.failed', { message: notifyErrorText(error) || t('adminNotify.telegram.send.failedShort') }))
  } finally {
    sending.value = false
  }
}

onMounted(() => {
  fetchBotConfig()
  fetchUsers()
})
</script>

<style scoped>
.notify-panel {
  display: flex;
  flex-direction: column;
  gap: var(--space-12);
  min-width: 0;
}

.notify-panel :deep(.ui-section) {
  gap: var(--space-5);
}

.notify-form,
.webhook {
  width: 100%;
}

.webhook {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.webhook__actions,
.send-form__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}

.send-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  max-width: 640px;
}

.send-form > :first-child {
  align-self: flex-start;
}

.notify-note {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.command {
  min-width: 72px;
  color: var(--label-1);
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
}
</style>
