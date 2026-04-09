<template>
  <div class="login-page">
    <div class="login-shell">
      <div class="login-toolbar">
        <LocaleSwitcher />
      </div>

      <div class="login-container">
        <div class="login-brand">
          <div class="brand-icon">V</div>
          <h1>V2Board</h1>
          <p>{{ t('login.brandSubtitle') }}</p>
        </div>

        <div class="login-box">
          <h2>{{ isRegisterMode ? t('login.registerTitle') : t('login.signInTitle') }}</h2>
          <p class="login-subtitle">{{ isRegisterMode ? t('login.registerSubtitle') : t('login.signInSubtitle') }}</p>

          <form class="login-form" @submit.prevent="isRegisterMode ? handleRegister() : handleLogin()">
            <div class="form-group">
              <label for="email">{{ t('common.labels.email') }}</label>
              <input
                id="email"
                v-model.trim="email"
                type="email"
                :placeholder="t('login.emailPlaceholder')"
                autocomplete="email"
              />
            </div>

            <div class="form-group">
              <label for="password">{{ t('common.labels.password') }}</label>
              <input
                id="password"
                v-model="password"
                type="password"
                :placeholder="isRegisterMode ? t('login.registerPasswordPlaceholder') : t('login.passwordPlaceholder')"
                :autocomplete="isRegisterMode ? 'new-password' : 'current-password'"
              />
            </div>

            <div v-if="isRegisterMode" class="form-group">
              <label for="confirm-password">{{ t('common.labels.confirmPassword') }}</label>
              <input
                id="confirm-password"
                v-model="confirmPassword"
                type="password"
                :placeholder="t('login.confirmPasswordPlaceholder')"
                autocomplete="new-password"
              />
            </div>

            <div v-if="errorMsg" class="error-msg">{{ errorMsg }}</div>
            <div v-if="successMsg" class="success-msg">{{ successMsg }}</div>

            <button type="submit" class="login-btn" :disabled="loading">
              <span v-if="loading" class="spinner"></span>
              {{ loading
                ? (isRegisterMode ? t('login.loadingRegister') : t('login.loadingLogin'))
                : (isRegisterMode ? t('login.submitRegister') : t('login.submitLogin')) }}
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
              <button type="button" class="btn-ghost" @click="mockLogin('user')">{{ t('login.mockUser') }}</button>
              <button type="button" class="btn-ghost" @click="mockLogin('admin')">{{ t('login.mockAdmin') }}</button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
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
const loading = ref(false)
const errorMsg = ref('')
const successMsg = ref('')
const isRegisterMode = ref(false)

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
}

async function handleRegister() {
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
      password: password.value
    })

    if (!res.data?.token) {
      throw new Error('register response missing token')
    }

    const { token, is_admin, user_id, email: userEmail } = res.data
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
    errorMsg.value = err.response?.data?.message || t('login.errors.registerFailed')
  } finally {
    loading.value = false
  }
}

async function handleLogin() {
  if (!email.value || !password.value) {
    errorMsg.value = t('login.errors.emailPasswordRequired')
    return
  }

  loading.value = true
  errorMsg.value = ''
  successMsg.value = ''

  try {
    const res = await login({
      email: email.value,
      password: password.value
    })

    if (!res.data?.token) {
      throw new Error('login response missing token')
    }

    const { token, is_admin, user_id, email: userEmail } = res.data
    userStore.login(token, {
      id: user_id,
      email: userEmail,
      is_admin
    })

    router.push(is_admin ? '/admin/dashboard' : '/user/dashboard')
  } catch (err) {
    errorMsg.value = err.response?.data?.message || t('login.errors.loginFailed')
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
  padding: 20px;
  background:
    radial-gradient(circle at top right, rgba(59, 130, 246, 0.25), transparent 32%),
    linear-gradient(135deg, #08111f 0%, #111827 50%, #0f172a 100%);
}

.login-shell {
  width: 100%;
  max-width: 460px;
}

.login-toolbar {
  display: flex;
  justify-content: flex-end;
  margin-bottom: 18px;
}

.login-container {
  width: 100%;
}

.login-brand {
  text-align: center;
  margin-bottom: 28px;
}

.brand-icon {
  width: 64px;
  height: 64px;
  border-radius: 20px;
  margin: 0 auto 16px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 28px;
  font-weight: 800;
  color: #fff;
  background: linear-gradient(135deg, #2563eb, #0ea5e9);
  box-shadow: 0 18px 35px rgba(37, 99, 235, 0.28);
}

.login-brand h1 {
  font-size: 28px;
  font-weight: 700;
  margin-bottom: 8px;
  color: #fff;
}

.login-brand p {
  color: rgba(226, 232, 240, 0.8);
  margin: 0;
}

.login-box {
  padding: 28px;
  border-radius: 24px;
  border: 1px solid rgba(148, 163, 184, 0.16);
  background: rgba(15, 23, 42, 0.78);
  backdrop-filter: blur(18px);
  box-shadow: 0 30px 60px rgba(2, 6, 23, 0.38);
}

.login-box h2 {
  margin-bottom: 8px;
  color: #fff;
}

.login-subtitle {
  margin-bottom: 24px;
  color: rgba(226, 232, 240, 0.72);
}

.login-form {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.form-group label {
  display: block;
  margin-bottom: 8px;
  color: #e2e8f0;
  font-size: 14px;
}

.form-group input {
  width: 100%;
  border-radius: 14px;
  border: 1px solid rgba(148, 163, 184, 0.2);
  background: rgba(15, 23, 42, 0.65);
  color: #fff;
  padding: 14px 16px;
}

.form-group input::placeholder {
  color: rgba(148, 163, 184, 0.72);
}

.error-msg,
.success-msg {
  border-radius: 12px;
  padding: 12px 14px;
  font-size: 14px;
}

.error-msg {
  background: rgba(239, 68, 68, 0.14);
  color: #fecaca;
}

.success-msg {
  background: rgba(34, 197, 94, 0.14);
  color: #bbf7d0;
}

.login-btn {
  width: 100%;
  min-height: 48px;
  border: 0;
  border-radius: 14px;
  background: linear-gradient(135deg, #2563eb, #0ea5e9);
  color: #fff;
  font-weight: 700;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
}

.login-btn:disabled {
  opacity: 0.72;
  cursor: not-allowed;
}

.spinner {
  width: 16px;
  height: 16px;
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
  color: #93c5fd;
  text-decoration: none;
  font-size: 14px;
}

.dev-actions {
  margin-top: 22px;
}

.dev-divider {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
  color: rgba(226, 232, 240, 0.6);
  font-size: 13px;
}

.dev-divider::before,
.dev-divider::after {
  content: '';
  flex: 1;
  height: 1px;
  background: rgba(148, 163, 184, 0.18);
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

@media (max-width: 520px) {
  .login-box {
    padding: 22px;
  }

  .login-footer {
    flex-direction: column;
  }

  .dev-buttons {
    grid-template-columns: 1fr;
  }
}
</style>
