<template>
  <main id="app-main-content" tabindex="-1" class="login-page" aria-labelledby="login-page-title">
    <div class="login-shell">
      <section class="login-aside">
        <div class="aside-top">
          <BrandLockup tile inverse />
          <div class="aside-pill">{{ t('layout.admin.badge') }}</div>
        </div>
        <div class="aside-copy">
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
        <BrandLockup class="panel-brand" tile />
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

          <div v-if="isRegisterMode && requireInvite" class="form-row">
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
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useUserStore } from '@/stores/user'
import { useAppI18n } from '@/composables/useAppI18n'
import { login, register } from '@/api/auth'
import { useEdition } from '@/composables/useEdition'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import BrandLockup from '@/components/common/BrandLockup.vue'

const router = useRouter()
const userStore = useUserStore()
const { t } = useAppI18n()
// Registration asks for an invite code only when auth.registration
// require_invite is on (GET /api/v4/public/config).
const { requireInvite, loadEdition } = useEdition()
onMounted(() => {
  void loadEdition()
})

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

function resolvePermissionFields(payload) {
  const fields = {}
  for (const field of ['permission_mode', 'permissions', 'restricted_plugins']) {
    if (Object.prototype.hasOwnProperty.call(payload || {}, field)) {
      fields[field] = payload[field]
    }
  }
  return fields
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
      ...(requireInvite.value && inviteCode.value ? { invite_code: inviteCode.value } : {})
    })

    const payload = resolveAuthPayload(res, t('login.errors.registerFailed'))
    const { token, is_admin, user_id, email: userEmail } = payload
    userStore.login(token, {
      id: user_id,
      email: userEmail,
      is_admin,
      ...resolvePermissionFields(payload)
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
      is_admin,
      ...resolvePermissionFields(payload)
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
  padding: var(--space-6);
}

.login-shell {
  width: 100%;
  max-width: 1120px;
  display: grid;
  grid-template-columns: minmax(320px, 1.05fr) minmax(360px, 0.95fr);
  gap: var(--space-6);
}

/* Brand moment: the mark tile on slate with a soft glow of the brand
   gradient; text stays on slate so it keeps AA contrast (brand.md §3.1). */
.login-aside {
  display: flex;
  flex-direction: column;
  padding: var(--space-10);
  border-radius: var(--radius-lg);
  background:
    radial-gradient(120% 90% at 100% 0%, color-mix(in srgb, var(--brand-gradient-end) 42%, transparent), transparent 62%),
    radial-gradient(90% 70% at 0% 100%, color-mix(in srgb, var(--brand-gradient-start) 26%, transparent), transparent 60%),
    var(--brand-slate-900);
  color: var(--on-accent);
  box-shadow: var(--shadow-2);
}

.aside-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  flex-wrap: wrap;
}

.aside-copy {
  margin-top: var(--space-12);
}

.aside-pill {
  display: inline-flex;
  align-items: center;
  min-height: 28px;
  padding: 0 var(--space-3);
  border-radius: var(--radius-pill);
  background: color-mix(in srgb, var(--on-accent) 14%, transparent);
  font-size: var(--type-caption-size);
  font-weight: var(--weight-semibold);
}

.login-aside h1 {
  font-size: var(--type-display-size);
  line-height: var(--type-display-line);
  font-weight: var(--type-display-weight);
  letter-spacing: var(--type-display-tracking);
}

.login-aside p {
  margin-top: var(--space-3);
  max-width: 440px;
  color: color-mix(in srgb, var(--on-accent) 78%, transparent);
}

.aside-mode-copy {
  color: color-mix(in srgb, var(--on-accent) 92%, transparent);
  font-weight: var(--weight-semibold);
}

.aside-stats {
  margin-top: auto;
  padding-top: var(--space-10);
  display: grid;
  gap: var(--space-3);
}

.aside-stat {
  padding: var(--space-4) var(--space-5);
  border-radius: var(--radius-md);
  background: color-mix(in srgb, var(--on-accent) 8%, transparent);
  border: 1px solid color-mix(in srgb, var(--on-accent) 10%, transparent);
}

.aside-stat-label {
  display: block;
  font-size: var(--type-caption-size);
  color: color-mix(in srgb, var(--on-accent) 70%, transparent);
  margin-bottom: var(--space-1);
}

.aside-stat strong {
  font-size: var(--type-body-size);
  font-weight: var(--weight-semibold);
}

.login-panel {
  padding: var(--space-8);
  border-radius: var(--radius-lg);
}

.panel-brand {
  display: none;
  margin-bottom: var(--space-6);
}

.login-panel-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-4);
  margin-bottom: var(--space-6);
}

.login-panel-header h2 {
  font-size: var(--type-title-2-size);
  line-height: var(--type-title-2-line);
  font-weight: var(--type-title-2-weight);
  letter-spacing: var(--type-title-2-tracking);
}

.login-panel-header p {
  margin-top: var(--space-2);
  color: var(--label-2);
}

.login-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

/* The global .form-row is a horizontal flex row; login fields stack their
   label on top (plan D5). */
.login-form .form-row {
  display: block;
}

.form-row label {
  display: block;
  margin-bottom: var(--space-2);
  font-size: var(--type-callout-size);
  font-weight: var(--weight-semibold);
  color: var(--label-2);
}

.banner {
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-sm);
  font-size: var(--type-callout-size);
}

.error-banner {
  background: var(--danger-soft);
  border: 1px solid color-mix(in srgb, var(--danger) 24%, transparent);
  color: var(--danger);
}

.success-banner {
  background: var(--success-soft);
  border: 1px solid color-mix(in srgb, var(--success) 24%, transparent);
  color: var(--success);
}

.submit-button {
  width: 100%;
  min-height: var(--size-control-lg);
}

.spinner {
  width: 14px;
  height: 14px;
  border: 2px solid color-mix(in srgb, var(--on-accent) 35%, transparent);
  border-top-color: var(--on-accent);
  border-radius: 50%;
  animation: rotate 1s linear infinite;
}

.login-footer {
  margin-top: var(--space-5);
  display: flex;
  justify-content: space-between;
  gap: var(--space-3);
}

.login-footer a {
  color: var(--accent);
  text-decoration: none;
  font-weight: var(--weight-medium);
}

.dev-actions {
  margin-top: var(--space-6);
}

.dev-divider {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  margin-bottom: var(--space-3);
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.dev-divider::before,
.dev-divider::after {
  content: '';
  flex: 1;
  height: 1px;
  background: var(--separator);
}

.dev-buttons {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--space-3);
}

@keyframes rotate {
  to {
    transform: rotate(360deg);
  }
}

@media (max-width: 920px) {
  .login-shell {
    max-width: 480px;
    grid-template-columns: 1fr;
  }

  .login-aside {
    display: none;
  }

  .panel-brand {
    display: inline-flex;
  }
}

@media (max-width: 520px) {
  .login-page {
    padding: var(--space-4);
  }

  .login-panel {
    padding: var(--space-6);
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
