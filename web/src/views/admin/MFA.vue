<template>
  <div class="mfa-page">
    <div class="page-header">
      <h1>{{ t('adminMfa.title') }}</h1>
      <p class="text-secondary">{{ t('adminMfa.subtitle') }}</p>
    </div>

    <div class="config-section">
      <h3>{{ t('adminMfa.config.title') }}</h3>

      <div class="form-group">
        <label class="checkbox-label">
          <input v-model="config.enabled" type="checkbox" />
          <span>{{ t('adminMfa.config.enabled') }}</span>
        </label>
        <p class="help-text">{{ t('adminMfa.config.enabledHelp') }}</p>
      </div>

      <div class="form-group">
        <label class="checkbox-label">
          <input v-model="config.required" type="checkbox" />
          <span>{{ t('adminMfa.config.required') }}</span>
        </label>
        <p class="help-text">{{ t('adminMfa.config.requiredHelp') }}</p>
      </div>

      <div class="form-group">
        <label>{{ t('adminMfa.config.methods') }}</label>
        <div class="checkbox-group">
          <label class="checkbox-label">
            <input v-model="config.methods.totp" type="checkbox" />
            <span>{{ t('adminMfa.methods.totp') }}</span>
          </label>
          <label class="checkbox-label">
            <input v-model="config.methods.sms" type="checkbox" />
            <span>{{ t('adminMfa.methods.sms') }}</span>
          </label>
          <label class="checkbox-label">
            <input v-model="config.methods.email" type="checkbox" />
            <span>{{ t('adminMfa.methods.email') }}</span>
          </label>
        </div>
      </div>

      <div class="form-row">
        <div class="form-group">
          <label>{{ t('adminMfa.config.backupCodesCount') }}</label>
          <input v-model.number="config.backup_codes_count" type="number" min="1" max="20" />
          <p class="help-text">{{ t('adminMfa.config.backupCodesHelp') }}</p>
        </div>
        <div class="form-group">
          <label>{{ t('adminMfa.config.maxAttempts') }}</label>
          <input v-model.number="config.max_attempts" type="number" min="1" max="10" />
          <p class="help-text">{{ t('adminMfa.config.maxAttemptsHelp') }}</p>
        </div>
      </div>

      <div class="form-group">
        <label>{{ t('adminMfa.config.lockoutDuration') }}</label>
        <input v-model.number="config.lockout_duration" type="number" min="1" />
        <p class="help-text">{{ t('adminMfa.config.lockoutDurationHelp') }}</p>
      </div>

      <div class="form-actions">
        <button class="btn-primary" @click="saveConfig">{{ t('common.actions.save') }}</button>
      </div>
    </div>

    <div class="info-section">
      <h3>{{ t('adminMfa.info.title') }}</h3>
      <div class="info-grid">
        <div class="info-item">
          <h4>{{ t('adminMfa.info.totpTitle') }}</h4>
          <p>{{ t('adminMfa.info.totpBody') }}</p>
        </div>
        <div class="info-item">
          <h4>{{ t('adminMfa.info.backupTitle') }}</h4>
          <p>{{ t('adminMfa.info.backupBody') }}</p>
        </div>
        <div class="info-item">
          <h4>{{ t('adminMfa.info.lockoutTitle') }}</h4>
          <p>{{ t('adminMfa.info.lockoutBody') }}</p>
        </div>
        <div class="info-item">
          <h4>{{ t('adminMfa.info.userOpsTitle') }}</h4>
          <p>{{ t('adminMfa.info.userOpsBody') }}</p>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { getMFAConfig, updateMFAConfig } from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'

const { t } = useAppI18n()

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

const resolveApiError = (error, fallbackKey) => (
  error?.response?.data?.error ||
  error?.response?.data?.msg ||
  error?.message ||
  t(fallbackKey)
)

const fetchConfig = async () => {
  try {
    const res = await getMFAConfig()
    if (res.data) {
      config.value = createMfaConfig(res.data)
    }
  } catch (error) {
    console.error(t('adminMfa.messages.fetchFailed'), error)
  }
}

const saveConfig = async () => {
  try {
    await updateMFAConfig(config.value)
    window.alert(t('adminMfa.messages.saveSuccess'))
  } catch (error) {
    window.alert(t('adminMfa.messages.saveFailed', { message: resolveApiError(error, 'adminMfa.messages.saveFailedShort') }))
  }
}

onMounted(() => {
  fetchConfig()
})
</script>

<style scoped>
.config-section,
.info-section {
  background: var(--surface-color);
  padding: 24px;
  border-radius: var(--radius-lg);
  border: 1px solid var(--border-color);
  margin-bottom: 20px;
}

.config-section h3,
.info-section h3 {
  margin-bottom: 16px;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--border-color);
}

.checkbox-label {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
}

.checkbox-label input[type="checkbox"] {
  width: 18px;
  height: 18px;
}

.checkbox-group {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin-top: 8px;
}

.help-text {
  color: var(--text-secondary);
  font-size: 12px;
  margin-top: 4px;
}

.info-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
  gap: 20px;
  margin-top: 16px;
}

.info-item {
  padding: 16px;
  background: var(--background-color);
  border-radius: var(--radius-md);
}

.info-item h4 {
  margin-bottom: 8px;
  color: var(--primary-color);
}

.info-item p {
  color: var(--text-secondary);
  font-size: 13px;
  margin: 0;
}
</style>
