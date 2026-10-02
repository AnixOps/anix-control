<template>
  <div class="mfa-section" data-security-panel="mfa">
    <UiSection :title="t('adminSecurity.mfa.title')" :description="t('adminSecurity.mfa.description')">
      <UiSkeleton v-if="showSkeleton" variant="card" :label="t('adminSettings.loading')" />
      <UiErrorState
        v-else-if="loadError"
        compact
        heading-tag="h3"
        :title="t('adminMfa.messages.fetchFailed')"
        :error="loadError"
        @retry="fetchConfig"
      />
      <template v-else-if="!loading">
        <UiGroupedList :footer="t('adminMfa.info.userOpsBody')">
          <UiGroupedListRow :label="t('adminMfa.config.enabled')" :description="t('adminMfa.config.enabledHelp')" label-for="mfa-enabled">
            <UiSwitch id="mfa-enabled" v-model="config.enabled" />
          </UiGroupedListRow>
          <UiGroupedListRow :label="t('adminMfa.config.required')" :description="t('adminMfa.config.requiredHelp')" label-for="mfa-required">
            <UiSwitch id="mfa-required" v-model="config.required" />
          </UiGroupedListRow>
        </UiGroupedList>

        <UiGroupedList :title="t('adminMfa.config.methods')" :footer="t('adminMfa.info.totpBody')">
          <UiGroupedListRow :label="t('adminMfa.methods.totp')" label-for="mfa-method-totp">
            <UiSwitch id="mfa-method-totp" v-model="config.methods.totp" />
          </UiGroupedListRow>
          <UiGroupedListRow :label="t('adminMfa.methods.sms')" label-for="mfa-method-sms">
            <UiSwitch id="mfa-method-sms" v-model="config.methods.sms" />
          </UiGroupedListRow>
          <UiGroupedListRow :label="t('adminMfa.methods.email')" label-for="mfa-method-email">
            <UiSwitch id="mfa-method-email" v-model="config.methods.email" />
          </UiGroupedListRow>
        </UiGroupedList>
        <p v-if="methodsError" class="form-error" role="alert" data-test="mfa-methods-error">{{ methodsError }}</p>

        <UiGroupedList :title="t('adminSecurity.mfa.recovery')" :footer="t('adminMfa.info.backupBody')">
          <UiGroupedListRow :label="t('adminMfa.config.backupCodesCount')" :description="t('adminMfa.config.backupCodesHelp')" label-for="mfa-backup-codes">
            <UiNumberField id="mfa-backup-codes" v-model="config.backup_codes_count" size="md" :min="1" :max="20" :error="errors.backup_codes_count" />
          </UiGroupedListRow>
        </UiGroupedList>

        <UiGroupedList :title="t('adminSecurity.mfa.lockout')" :footer="t('adminMfa.info.lockoutBody')">
          <UiGroupedListRow :label="t('adminMfa.config.maxAttempts')" :description="t('adminMfa.config.maxAttemptsHelp')" label-for="mfa-max-attempts">
            <UiNumberField id="mfa-max-attempts" v-model="config.max_attempts" size="md" :min="1" :max="10" :error="errors.max_attempts" />
          </UiGroupedListRow>
          <UiGroupedListRow :label="t('adminSecurity.mfa.lockoutDuration')" :description="t('adminMfa.config.lockoutDurationHelp')" label-for="mfa-lockout">
            <UiNumberField id="mfa-lockout" v-model="config.lockout_duration" size="md" :min="1" :unit="t('adminSecurity.mfa.minutes')" :error="errors.lockout_duration" />
          </UiGroupedListRow>
        </UiGroupedList>
      </template>
    </UiSection>

    <SettingsSaveBar :visible="dirty" :saving="saving" :invalid="invalid" @save="saveConfig" @discard="discardConfig" />
  </div>
</template>

<script setup>
// 安全 → 两步验证: the administrator's MFA policy for everyone (on, required,
// methods, recovery codes, lockout), as a settings form with the save bar.
// Rendered by Security.vue; /admin/mfa redirects there. Endpoints and the
// payload unchanged: GET/PUT /admin/mfa/config.
import { computed, onMounted, ref } from 'vue'
import { getMFAConfig, updateMFAConfig } from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import { useUnsavedChanges } from '@/composables/useUnsavedChanges'
import SettingsSaveBar from '@/components/admin/settings/SettingsSaveBar.vue'
import UiErrorState from '@/ui/UiErrorState.vue'
import UiGroupedList from '@/ui/UiGroupedList.vue'
import UiGroupedListRow from '@/ui/UiGroupedListRow.vue'
import UiNumberField from '@/ui/UiNumberField.vue'
import UiSection from '@/ui/UiSection.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import UiSwitch from '@/ui/UiSwitch.vue'
import { useDelayedLoading } from '@/ui/composables/useDelayedLoading'
import { useToast } from '@/ui/composables/useToast'

const { t } = useAppI18n()
const toast = useToast()

const defaultConfig = Object.freeze({
  enabled: false,
  required: false,
  methods: {
    totp: true,
    sms: false,
    email: true
  },
  backup_codes_count: 10,
  max_attempts: 5,
  lockout_duration: 15
})

const config = ref(createMfaConfig())
const savedConfig = ref(JSON.stringify(config.value))
const loading = ref(false)
const loadError = ref(null)
const saving = ref(false)
const showSkeleton = useDelayedLoading(loading)

function createMfaConfig(source = {}) {
  return {
    ...defaultConfig,
    ...source,
    methods: {
      ...defaultConfig.methods,
      ...(source.methods || {})
    }
  }
}

const inRange = (value, min, max) => Number.isInteger(Number(value)) && value !== null && value !== '' && Number(value) >= min && (max === undefined || Number(value) <= max)
const errors = computed(() => ({
  backup_codes_count: inRange(config.value.backup_codes_count, 1, 20) ? '' : t('adminSecurity.mfa.range', { min: 1, max: 20 }),
  max_attempts: inRange(config.value.max_attempts, 1, 10) ? '' : t('adminSecurity.mfa.range', { min: 1, max: 10 }),
  lockout_duration: inRange(config.value.lockout_duration, 1) ? '' : t('adminSecurity.mfa.atLeastOne')
}))
const methodsError = computed(() => (
  config.value.enabled && !Object.values(config.value.methods).some(Boolean) ? t('adminSecurity.mfa.methodRequired') : ''
))
const invalid = computed(() => Boolean(methodsError.value || Object.values(errors.value).some(Boolean)))
const dirty = computed(() => JSON.stringify(config.value) !== savedConfig.value)

const resolveApiError = (error, fallbackKey) => (
  error?.response?.data?.error ||
  error?.response?.data?.msg ||
  error?.message ||
  t(fallbackKey)
)

const requirePanelSuccess = (res, fallbackKey) => {
  if (typeof res?.code === 'number' && res.code !== 0) {
    throw new Error(res.msg || t(fallbackKey))
  }
  return res
}

const readMfaConfigPayload = (res) => {
  if (!res || typeof res !== 'object') return {}
  if (Object.prototype.hasOwnProperty.call(res, 'code')) {
    return res.data && typeof res.data === 'object' ? res.data : {}
  }
  if (res.data && typeof res.data === 'object' && Object.prototype.hasOwnProperty.call(res.data, 'data')) {
    return res.data.data && typeof res.data.data === 'object' ? res.data.data : {}
  }
  return res.data && typeof res.data === 'object' ? res.data : res
}

function commitConfig(next) {
  config.value = next
  savedConfig.value = JSON.stringify(next)
}

const fetchConfig = async () => {
  loading.value = true
  loadError.value = null
  try {
    const res = requirePanelSuccess(await getMFAConfig(), 'adminMfa.messages.fetchFailed')
    const payload = readMfaConfigPayload(res)
    if (payload && typeof payload === 'object') {
      commitConfig(createMfaConfig(payload))
    }
  } catch (error) {
    loadError.value = resolveApiError(error, 'adminMfa.messages.fetchFailed')
  } finally {
    loading.value = false
  }
}

const saveConfig = async () => {
  if (saving.value || invalid.value) return
  saving.value = true
  try {
    requirePanelSuccess(await updateMFAConfig(config.value), 'adminMfa.messages.saveFailedShort')
    commitConfig(createMfaConfig(config.value))
    toast.success(t('adminMfa.messages.saveSuccess'))
  } catch (error) {
    toast.error(t('adminMfa.messages.saveFailed', { message: resolveApiError(error, 'adminMfa.messages.saveFailedShort') }))
  } finally {
    saving.value = false
  }
}

function discardConfig() {
  config.value = JSON.parse(savedConfig.value)
}

useUnsavedChanges(dirty, { discard: discardConfig })

onMounted(fetchConfig)
</script>

<style scoped>
.mfa-section {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.mfa-section :deep(.ui-section) {
  gap: var(--space-6);
}

.form-error {
  margin: calc(-1 * var(--space-4)) 0 0;
}
</style>
