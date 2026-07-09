<template>
  <main id="app-main-content" tabindex="-1" class="login-page" aria-labelledby="login-page-title">
    <div class="login-shell">
      <section class="login-aside">
        <div class="aside-pill">{{ t('layout.admin.badge') }}</div>
        <div class="aside-copy">
          <div class="aside-kicker">{{ t('login.brandSubtitle') }}</div>
          <h1>{{ t('layout.user.brand') }}</h1>
          <p>{{ t('login.brandDescription') }}</p>
          <p class="aside-mode-copy">{{ isRegisterMode ? t('login.registerSubtitle') : t('login.signInSubtitle') }}</p>
        </div>
        <div class="aside-stats">
          <div class="aside-stat">
            <span class="aside-stat-label">{{ t('layout.admin.sections.overview') }}</span>
            <strong>{{ t('layout.admin.nav.dashboard') }}</strong>
          </div>
          <div class="aside-stat">
            <span class="aside-stat-label">{{ t('layout.admin.sections.userManagement') }}</span>
            <strong>{{ t('layout.admin.nav.users') }}</strong>
          </div>
          <div class="aside-stat">
            <span class="aside-stat-label">{{ t('layout.admin.sections.system') }}</span>
            <strong>{{ t('layout.admin.nav.system') }}</strong>
          </div>
        </div>
      </section>

      <section class="login-panel card">
        <div class="login-panel-header">
          <div>
            <h2 id="login-page-title">{{ isRegisterMode ? t('login.registerTitle') : t('login.signInTitle') }}</h2>
            <p>{{ isRegisterMode ? t('login.registerSubtitle') : t('login.signInSubtitle') }}</p>
          </div>
          <LocaleSwitcher />
        </div>

        <form class="login-form" :aria-busy="loading ? 'true' : 'false'" @submit.prevent="isRegisterMode ? handleRegister() : handleLogin()">
          <div class="form-row">
            <label for="email">{{ t('common.labels.email') }}</label>
            <input
              id="email"
              v-model.trim="email"
              type="email"
              :placeholder="t('login.emailPlaceholder')"
              autocomplete="email"
            />
          </div>

          <div class="form-row">
            <label for="password">{{ t('common.labels.password') }}</label>
            <input
              id="password"
              v-model="password"
              type="password"
              :placeholder="isRegisterMode ? t('login.registerPasswordPlaceholder') : t('login.passwordPlaceholder')"
              :autocomplete="isRegisterMode ? 'new-password' : 'current-password'"
            />
          </div>

          <div v-if="mfaRequired && !isRegisterMode" class="form-row">
            <label for="mfa-code">{{ t('login.mfaCodeLabel') }}</label>
            <input
              id="mfa-code"
              v-model.trim="mfaCode"
              type="text"
              :placeholder="t('login.mfaCodePlaceholder')"
              autocomplete="one-time-code"
            />
          </div>

          <div v-if="isRegisterMode" class="form-row">
            <label for="confirm-password">{{ t('common.labels.confirmPassword') }}</label>
            <input
              id="confirm-password"
              v-model="confirmPassword"
              type="password"
              :placeholder="t('login.confirmPasswordPlaceholder')"
              autocomplete="new-password"
            />
          </div>

          <div v-if="isRegisterMode" class="form-row">
            <label for="invite-code">{{ t('login.inviteCodeLabel') }}</label>
            <input
              id="invite-code"
              v-model.trim="inviteCode"
              type="text"
              :placeholder="t('login.inviteCodePlaceholder')"
              autocomplete="off"
            />
          </div>

          <div v-if="errorMsg" class="banner error-banner" role="alert">{{ errorMsg }}</div>
          <div v-if="successMsg" class="banner success-banner" role="status" aria-live="polite">{{ successMsg }}</div>

          <button type="submit" class="btn btn-primary submit-button" :disabled="loading">
            <span v-if="loading" class="spinner"></span>
            {{ loading
              ? (isRegisterMode ? t('login.loadingRegister') : t('login.loadingLogin'))
              : submitLabel }}
          </button>
        </form>

        <div class="login-footer">
          <a v-if="!isRegisterMode" href="#" @click.prevent>{{ t('login.forgotPassword') }}</a>
          <a href="#" @click.prevent="toggleMode">
            {{ isRegisterMode ? t('login.switchToLogin') : t('login.switchToRegister') }}
          </a>
        </div>

        <div v-if="enableMockLogin && !isRegisterMode" class="dev-actions">
          <div class="dev-divider">
            <span>{{ t('login.mockMode') }}</span>
          </div>
          <div class="dev-buttons">
            <button type="button" class="btn" @click="mockLogin('user')">{{ t('login.mockUser') }}</button>
            <button type="button" class="btn" @click="mockLogin('admin')">{{ t('login.mockAdmin') }}</button>
          </div>
        </div>
      </section>
    </div>
  </main>
</template>

<script setup>
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useUserStore } from '@/stores/user'
import { useAppI18n } from '@/composables/useAppI18n'
import { login, register } from '@/api/auth'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'

const router = useRouter()
const userStore = useUserStore()
const { t } = useAppI18n()

const email = ref('')
const password = ref('')
const confirmPassword = ref('')
const inviteCode = ref('')
const loading = ref(false)
const errorMsg = ref('')
const successMsg = ref('')
const isRegisterMode = ref(false)
const mfaRequired = ref(false)
const mfaCode = ref('')
const mfaMethods = ref([])

const submitLabel = computed(() => {
  if (isRegisterMode.value) {
    return t('login.submitRegister')
  }
  return mfaRequired.value ? t('login.submitMFA') : t('login.submitLogin')
})

const enableMockLogin = computed(() => {
  const flag = String(import.meta.env.VITE_ENABLE_MOCK_LOGIN || '').trim().toLowerCase()
  return flag === 'true' || flag === '1' || flag === 'yes'
})

function toggleMode() {
  isRegisterMode.value = !isRegisterMode.value
  errorMsg.value = ''
  successMsg.value = ''
  password.value = ''
  confirmPassword.value = ''
  inviteCode.value = ''
  resetMFAChallenge()
}

function resetMFAChallenge() {
  mfaRequired.value = false
  mfaCode.value = ''
  mfaMethods.value = []
}

function resolveAuthPayload(res, fallbackMessage, options = {}) {
  if (typeof res?.code === 'number' && res.code !== 0) {
    throw new Error(res.msg || fallbackMessage)
  }

  const payload = res?.data && typeof res.data === 'object' ? res.data : res
  if (options.allowMFAChallenge && (payload?.mfa_required || payload?.mfa_enrollment_required)) {
    return payload
  }
  if (!payload?.token) {
    throw new Error(fallbackMessage)
  }
  return payload
}

function resolveAuthErrorMessage(err, fallbackMessage) {
  return err?.response?.data?.msg || err?.response?.data?.message || err?.message || fallbackMessage
}

async function handleRegister() {
  resetMFAChallenge()
  if (!email.value || !password.value) {
    errorMsg.value = t('login.errors.emailPasswordRequired')
    return
  }
  if (password.value.length < 6) {
    errorMsg.value = t('login.errors.passwordMin')
    return
  }
  if (password.value !== confirmPassword.value) {
    errorMsg.value = t('login.errors.passwordMismatch')
    return
  }

  loading.value = true
  errorMsg.value = ''
  successMsg.value = ''

  try {
    const res = await register({
      email: email.value,
      password: password.value,
      ...(inviteCode.value ? { invite_code: inviteCode.value } : {})
    })

    const { token, is_admin, user_id, email: userEmail } = resolveAuthPayload(res, t('login.errors.registerFailed'))
    userStore.login(token, {
      id: user_id,
      email: userEmail,
      is_admin
    })

    successMsg.value = t('login.success.registerCompleted')
    setTimeout(() => {
      router.push(is_admin ? '/admin/dashboard' : '/user/dashboard')
    }, 1000)
  } catch (err) {
    errorMsg.value = resolveAuthErrorMessage(err, t('login.errors.registerFailed'))
  } finally {
    loading.value = false
  }
}

async function handleLogin() {
  if (!email.value || !password.value) {
    errorMsg.value = t('login.errors.emailPasswordRequired')
    return
  }
  if (mfaRequired.value && !mfaCode.value) {
    errorMsg.value = t('login.errors.mfaCodeRequired')
    return
  }

  loading.value = true
  errorMsg.value = ''
  successMsg.value = ''

  try {
    const res = await login({
      email: email.value,
      password: password.value,
      ...(mfaRequired.value ? { mfa_code: mfaCode.value } : {})
    })

    const payload = resolveAuthPayload(res, t('login.errors.loginFailed'), { allowMFAChallenge: true })
    if (payload.mfa_enrollment_required) {
      resetMFAChallenge()
      errorMsg.value = t('login.errors.mfaEnrollmentRequired')
      return
    }
    if (payload.mfa_required) {
      mfaRequired.value = true
      mfaCode.value = ''
      mfaMethods.value = Array.isArray(payload.methods) ? payload.methods : []
      successMsg.value = t('login.mfaRequired')
      return
    }

    const { token, is_admin, user_id, email: userEmail } = payload
    userStore.login(token, {
      id: user_id,
      email: userEmail,
      is_admin
    })

    resetMFAChallenge()
    router.push(is_admin ? '/admin/dashboard' : '/user/dashboard')
  } catch (err) {
    errorMsg.value = resolveAuthErrorMessage(err, t('login.errors.loginFailed'))
  } finally {
    loading.value = false
  }
}

function mockLogin(role) {
  if (!enableMockLogin.value) {
    return
  }

  const mockUser = {
    id: 1,
    email: role === 'admin' ? 'admin@example.com' : 'user@example.com',
    is_admin: role === 'admin'
  }

  resetMFAChallenge()
  userStore.login(`mock-token-${role}`, mockUser)
  router.push(role === 'admin' ? '/admin/dashboard' : '/user/dashboard')
}
</script>

<style scoped>
.login-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
}

.login-shell {
  width: 100%;
  max-width: 1120px;
  display: grid;
  grid-template-columns: minmax(320px, 1.05fr) minmax(360px, 0.95fr);
  gap: 24px;
}

.login-aside {
  padding: 36px;
  border-radius: 8px;
  background:
    linear-gradient(180deg, rgba(0, 100, 250, 0.94), rgba(8, 47, 135, 0.94)),
    #0f172a;
  color: #fff;
  box-shadow: var(--shadow-lg);
}

.aside-copy {
  margin-top: 24px;
}

.aside-pill {
  display: inline-flex;
  align-items: center;
  min-height: 28px;
  padding: 0 12px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.14);
  font-size: 12px;
  font-weight: 700;
}

.login-aside h1 {
  font-size: 36px;
  line-height: 1.1;
}

.aside-kicker {
  display: inline-flex;
  align-items: center;
  min-height: 24px;
  padding: 0 10px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.1);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.login-aside p {
  margin-top: 12px;
  max-width: 440px;
  color: rgba(255, 255, 255, 0.78);
}

.aside-mode-copy {
  color: rgba(255, 255, 255, 0.92);
  font-weight: 600;
}

.aside-stats {
  margin-top: 36px;
  display: grid;
  gap: 14px;
}

.aside-stat {
  padding: 16px 18px;
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.1);
}

.aside-stat-label {
  display: block;
  font-size: 12px;
  color: rgba(255, 255, 255, 0.66);
  margin-bottom: 6px;
}

.aside-stat strong {
  font-size: 16px;
}

.login-panel {
  padding: 28px;
}

.login-panel-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 24px;
}

.login-panel-header h2 {
  font-size: 28px;
  line-height: 1.15;
}

.login-panel-header p {
  margin-top: 8px;
  color: var(--text-secondary);
}

.login-form {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.form-row label {
  display: block;
  margin-bottom: 8px;
  font-size: 13px;
  font-weight: 700;
  color: var(--text-secondary);
}

.banner {
  padding: 12px 14px;
  border-radius: 8px;
  font-size: 14px;
}

.error-banner {
  background: rgba(220, 38, 38, 0.08);
  border: 1px solid rgba(220, 38, 38, 0.18);
  color: var(--error-color);
}

.success-banner {
  background: rgba(22, 163, 74, 0.08);
  border: 1px solid rgba(22, 163, 74, 0.18);
  color: var(--success-color);
}

.submit-button {
  width: 100%;
  min-height: 44px;
}

.spinner {
  width: 14px;
  height: 14px;
  border: 2px solid rgba(255, 255, 255, 0.35);
  border-top-color: #fff;
  border-radius: 50%;
  animation: rotate 1s linear infinite;
}

.login-footer {
  margin-top: 18px;
  display: flex;
  justify-content: space-between;
  gap: 12px;
}

.login-footer a {
  color: var(--primary-color);
  text-decoration: none;
  font-weight: 600;
}

.dev-actions {
  margin-top: 22px;
}

.dev-divider {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
  color: var(--text-secondary);
  font-size: 13px;
}

.dev-divider::before,
.dev-divider::after {
  content: '';
  flex: 1;
  height: 1px;
  background: var(--border-color);
}

.dev-buttons {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}

@keyframes rotate {
  to {
    transform: rotate(360deg);
  }
}

@media (max-width: 920px) {
  .login-shell {
    grid-template-columns: 1fr;
  }

  .login-aside {
    display: none;
  }
}

@media (max-width: 520px) {
  .login-page {
    padding: 16px;
  }

  .login-panel {
    padding: 22px;
  }

  .login-panel-header,
  .login-footer {
    flex-direction: column;
  }

  .dev-buttons {
    grid-template-columns: 1fr;
  }
}
</style>
