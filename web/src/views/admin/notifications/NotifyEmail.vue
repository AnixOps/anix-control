<template>
  <div class="notify-panel" data-notify-panel="email">
    <UiSection :title="t('adminNotify.email.title')" :description="t('adminNotify.email.description')">
      <template #actions>
        <UiButton :icon="Send" data-test="notify-test-send" @click="openTestModal">{{ t('adminNotify.email.testSend') }}</UiButton>
      </template>
      <UiSkeleton v-if="showSkeleton" variant="card" :label="t('adminSettings.loading')" />
      <UiErrorState
        v-else-if="emailLoadError"
        compact
        heading-tag="h3"
        :title="t('adminNotify.email.loadFailed')"
        :error="emailLoadError"
        @retry="fetchEmailSettings"
      />
      <template v-else-if="!emailLoading">
        <UiGroupedList :title="t('adminNotify.email.server')">
          <UiGroupedListRow stacked>
            <div class="form-grid notify-form">
              <UiTextField id="smtp-host" v-model="emailConfig.host" size="md" :label="t('adminNotify.email.host')" placeholder="smtp.example.com" autocomplete="off" />
              <UiNumberField id="smtp-port" v-model="emailConfig.port" size="md" :min="1" :max="65535" :label="t('adminNotify.email.port')" :help="t('adminNotify.email.portHelp')" :error="portError" />
              <UiTextField id="smtp-username" v-model="emailConfig.username" size="md" :label="t('adminNotify.email.username')" autocomplete="off" />
              <UiPasswordField
                id="smtp-password"
                v-model="emailConfig.password"
                size="md"
                autocomplete="new-password"
                :label="t('adminNotify.email.password')"
                :help="emailPasswordStored ? t('adminNotify.email.passwordStored') : ''"
              />
            </div>
          </UiGroupedListRow>
          <UiGroupedListRow :label="t('adminNotify.email.encryption')" :description="t('adminNotify.email.encryptionHelp')" label-for="smtp-encryption">
            <UiSwitch id="smtp-encryption" v-model="emailConfig.encryption" />
          </UiGroupedListRow>
        </UiGroupedList>
        <UiGroupedList :title="t('adminNotify.email.sender')">
          <UiGroupedListRow stacked>
            <div class="form-grid notify-form">
              <UiTextField id="smtp-from-name" v-model="emailConfig.from_name" size="md" :label="t('adminNotify.email.fromName')" :placeholder="t('adminNotify.email.fromNamePlaceholder')" />
              <UiTextField
                id="smtp-from-address"
                v-model="emailConfig.from_address"
                size="md"
                type="email"
                :label="t('adminNotify.email.fromAddress')"
                placeholder="noreply@example.com"
                :error="fromAddressError"
              />
            </div>
          </UiGroupedListRow>
        </UiGroupedList>
      </template>
    </UiSection>

    <SettingsSaveBar
      :visible="emailDirty"
      :saving="emailSaving"
      :invalid="emailInvalid"
      @save="saveEmailSettings"
      @discard="discardEmailSettings"
    />

    <UiDialog v-model:open="showTestModal" size="sm" :title="t('adminNotify.email.testTitle')" :description="t('adminNotify.email.testDescription')" :dismissible="!testSending">
      <UiTextField
        v-model="testEmail"
        type="email"
        autocomplete="email"
        data-test="notification-test-recipient"
        :label="t('adminNotify.email.testRecipient')"
        placeholder="ops@example.com"
        :error="testError"
        required
        @keydown.enter.prevent="sendTestEmail"
      />
      <template #footer="{ close }">
        <UiButton :disabled="testSending" @click="close">{{ t('common.actions.cancel') }}</UiButton>
        <UiButton variant="primary" data-test="notification-test-send" :loading="testSending" @click="sendTestEmail">{{ t('adminNotify.email.testSendAction') }}</UiButton>
      </template>
    </UiDialog>
  </div>
</template>

<script setup>
// 通知 → 邮件: the SMTP settings (saved with the save bar) and 测试发送.
// The API answers a stored password as ********: the field then starts
// empty, and saving it empty keeps the stored password. Endpoints
// unchanged: GET/PUT /admin/notification/email/config,
// POST /admin/notification/test.
import { computed, onMounted, ref } from 'vue'
import { Send } from '@lucide/vue'
import { getEmailConfig, sendTestNotification, updateEmailConfig } from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import { useUnsavedChanges } from '@/composables/useUnsavedChanges'
import { isMaskedSecret } from '@/constants/secrets'
import SettingsSaveBar from '@/components/admin/settings/SettingsSaveBar.vue'
import UiButton from '@/ui/UiButton.vue'
import UiDialog from '@/ui/UiDialog.vue'
import UiErrorState from '@/ui/UiErrorState.vue'
import UiGroupedList from '@/ui/UiGroupedList.vue'
import UiGroupedListRow from '@/ui/UiGroupedListRow.vue'
import UiNumberField from '@/ui/UiNumberField.vue'
import UiPasswordField from '@/ui/UiPasswordField.vue'
import UiSection from '@/ui/UiSection.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import UiSwitch from '@/ui/UiSwitch.vue'
import UiTextField from '@/ui/UiTextField.vue'
import { useDelayedLoading } from '@/ui/composables/useDelayedLoading'
import { useToast } from '@/ui/composables/useToast'
import { ensureNotifySuccess, notifyErrorText, readNotifyPayload } from './notifyResponse'

const { t } = useAppI18n()
const toast = useToast()

const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

const emailConfig = ref(defaultEmailConfig())
const savedEmailConfig = ref(JSON.stringify(emailConfig.value))
const emailPasswordStored = ref(false)
const emailLoading = ref(false)
const emailLoadError = ref(null)
const emailSaving = ref(false)
const showSkeleton = useDelayedLoading(emailLoading)

const showTestModal = ref(false)
const testEmail = ref('')
const testSending = ref(false)
const testError = ref('')

function defaultEmailConfig() {
  return { host: '', port: 465, username: '', password: '', from_name: '', from_address: '', encryption: true }
}

const portError = computed(() => {
  const port = Number(emailConfig.value.port)
  return Number.isInteger(port) && port >= 1 && port <= 65535 ? '' : t('adminNotify.email.portInvalid')
})
const fromAddressError = computed(() => {
  const value = String(emailConfig.value.from_address || '').trim()
  return !value || EMAIL_PATTERN.test(value) ? '' : t('adminNotify.email.addressInvalid')
})
const emailInvalid = computed(() => Boolean(portError.value || fromAddressError.value))
const emailDirty = computed(() => JSON.stringify(emailConfig.value) !== savedEmailConfig.value)

function commitEmailConfig(config) {
  emailConfig.value = config
  savedEmailConfig.value = JSON.stringify(config)
}

const fetchEmailSettings = async () => {
  emailLoading.value = true
  emailLoadError.value = null
  try {
    const res = await getEmailConfig()
    const payload = readNotifyPayload(res, t('adminNotify.email.loadFailed'))
    if (payload && typeof payload === 'object') {
      const next = { ...defaultEmailConfig(), ...payload }
      emailPasswordStored.value = isMaskedSecret(payload.password)
      if (emailPasswordStored.value) next.password = ''
      commitEmailConfig(next)
    }
  } catch (error) {
    emailLoadError.value = notifyErrorText(error) || t('adminNotify.email.loadFailed')
  } finally {
    emailLoading.value = false
  }
}

const saveEmailSettings = async () => {
  if (emailSaving.value || emailInvalid.value) return
  emailSaving.value = true
  try {
    ensureNotifySuccess(await updateEmailConfig(emailConfig.value), t('adminNotify.email.saveFailedShort'))
    if (String(emailConfig.value.password || '').trim()) emailPasswordStored.value = true
    commitEmailConfig({ ...emailConfig.value, password: '' })
    toast.success(t('adminNotify.email.saved'))
  } catch (error) {
    toast.error(t('adminNotify.email.saveFailed', { message: notifyErrorText(error) || t('adminNotify.email.saveFailedShort') }))
  } finally {
    emailSaving.value = false
  }
}

function discardEmailSettings() {
  emailConfig.value = JSON.parse(savedEmailConfig.value)
}

useUnsavedChanges(emailDirty, { discard: discardEmailSettings })

const openTestModal = () => {
  testError.value = ''
  showTestModal.value = true
}

const sendTestEmail = async () => {
  if (testSending.value) return
  const recipient = testEmail.value.trim()
  if (!recipient) {
    testError.value = t('adminNotify.email.testRecipientRequired')
    return
  }
  if (!EMAIL_PATTERN.test(recipient)) {
    testError.value = t('adminNotify.email.addressInvalid')
    return
  }

  testSending.value = true
  testError.value = ''
  try {
    ensureNotifySuccess(
      await sendTestNotification({
        type: 'email',
        recipient,
        subject: t('adminNotify.email.testSubject'),
        content: t('adminNotify.email.testContent')
      }),
      t('adminNotify.email.testFailedShort')
    )
    toast.success(t('adminNotify.email.testSent', { recipient }))
    showTestModal.value = false
    testEmail.value = ''
  } catch (error) {
    testError.value = t('adminNotify.email.testFailed', { message: notifyErrorText(error) || t('adminNotify.email.testFailedShort') })
  } finally {
    testSending.value = false
  }
}

onMounted(fetchEmailSettings)
</script>

<style scoped>
.notify-panel {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
  min-width: 0;
}

.notify-panel :deep(.ui-section) {
  gap: var(--space-5);
}

.notify-form {
  width: 100%;
}
</style>
