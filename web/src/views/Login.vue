<template>
  <main id="app-main-content" tabindex="-1" class="auth" :aria-labelledby="titleId">
    <div class="auth__stack">
      <section class="auth__card" :data-auth-step="step" :aria-busy="loading ? 'true' : 'false'">
        <header class="auth__header">
          <BrandLockup class="auth__tile" tile mark-only size="lg" style="--lockup-mark: 56px" />
          <h1 :id="titleId" ref="headingRef" class="auth__title" tabindex="-1">{{ heading.title }}</h1>
          <p class="auth__subtitle">
            <template v-if="step === 'mfa' || step === 'enroll'">
              <i18n-t :keypath="step === 'mfa' ? 'auth.mfa.description' : 'auth.enroll.description'" scope="global">
                <template #email><strong class="auth__email">{{ challengeEmail }}</strong></template>
              </i18n-t>
            </template>
            <template v-else>{{ heading.subtitle }}</template>
          </p>
        </header>

        <!-- Email and password (sign in), or the registration form. -->
        <form
          v-if="step === 'password'"
          class="auth__form"
          novalidate
          data-auth-form="password"
          @submit.prevent="mode === 'register' ? submitRegister() : submitPassword()"
        >
          <UiTextField
            id="email"
            ref="emailRef"
            v-model.trim="email"
            type="email"
            :label="mode === 'register' ? t('auth.register.email') : t('auth.signIn.email')"
            :autocomplete="mode === 'register' ? 'email' : 'username'"
            :error="errors.email"
            autocapitalize="off"
            spellcheck="false"
            aria-required="true"
            @blur="validateEmail"
          />
          <UiPasswordField
            id="password"
            v-model="password"
            :label="mode === 'register' ? t('auth.register.password') : t('auth.signIn.password')"
            :help="mode === 'register' ? t('auth.register.passwordHelp') : ''"
            :autocomplete="mode === 'register' ? 'new-password' : 'current-password'"
            :error="errors.password"
            aria-required="true"
          />
          <template v-if="mode === 'register'">
            <UiPasswordField
              id="confirm-password"
              v-model="confirmPassword"
              :label="t('auth.register.confirm')"
              autocomplete="new-password"
              :error="errors.confirm"
              aria-required="true"
            />
            <UiTextField
              v-if="requireInvite"
              id="invite-code"
              v-model.trim="inviteCode"
              :label="t('auth.register.invite')"
              :help="t('auth.register.inviteHelp')"
              :error="errors.invite"
              autocomplete="off"
              autocapitalize="characters"
              spellcheck="false"
              aria-required="true"
            />
          </template>

          <p v-if="formError" class="auth__alert" role="alert" data-auth-error>
            <UiIcon :icon="CircleAlert" :size="16" />
            <span>{{ formError }}</span>
          </p>

          <UiButton type="submit" variant="primary" size="lg" block :loading="loading" data-auth-submit>
            {{ mode === 'register' ? t('auth.register.submit') : t('auth.signIn.submit') }}
          </UiButton>

          <p v-if="mode === 'register' || registrationEnabled" class="auth__switch">
            <span>{{ mode === 'register' ? t('auth.register.haveAccount') : t('auth.signIn.noAccount') }}</span>
            <UiButton variant="tertiary" size="sm" data-auth-switch @click="switchMode">
              {{ mode === 'register' ? t('auth.register.signIn') : t('auth.signIn.register') }}
            </UiButton>
          </p>
        </form>

        <!-- Second step: the 6-digit code. -->
        <form v-else-if="step === 'mfa'" class="auth__form" novalidate data-auth-form="mfa" @submit.prevent="submitCode()">
          <UiOtpField
            ref="otpRef"
            v-model="code"
            :label="t('auth.mfa.code')"
            hide-label
            :error="errors.code"
            :disabled="loading"
            @complete="submitCode"
          />
          <UiButton type="submit" variant="primary" size="lg" block :loading="loading" data-auth-submit>
            {{ t('auth.mfa.submit') }}
          </UiButton>
          <div class="auth__row">
            <UiButton variant="tertiary" size="sm" :icon="ChevronLeft" data-auth-back @click="backToPassword">{{ t('auth.mfa.back') }}</UiButton>
            <UiButton v-if="canUseRecovery" variant="tertiary" size="sm" data-auth-recovery @click="goTo('recovery')">{{ t('auth.mfa.useRecovery') }}</UiButton>
          </div>
        </form>

        <!-- Second step with a recovery (backup) code. -->
        <form v-else-if="step === 'recovery'" class="auth__form" novalidate data-auth-form="recovery" @submit.prevent="submitRecovery">
          <UiTextField
            id="recovery-code"
            ref="recoveryRef"
            v-model="recoveryCode"
            :label="t('auth.mfa.recovery')"
            placeholder="XXXX-XXXX"
            :error="errors.recovery"
            autocomplete="one-time-code"
            autocapitalize="characters"
            spellcheck="false"
            maxlength="9"
            class="auth__recovery"
            aria-required="true"
            @blur="recoveryCode = formatRecovery(recoveryCode)"
          />
          <UiButton type="submit" variant="primary" size="lg" block :loading="loading" data-auth-submit>
            {{ t('auth.mfa.submit') }}
          </UiButton>
          <div class="auth__row">
            <UiButton variant="tertiary" size="sm" :icon="ChevronLeft" data-auth-back @click="backToPassword">{{ t('auth.mfa.back') }}</UiButton>
            <UiButton variant="tertiary" size="sm" data-auth-use-code @click="goTo('mfa')">{{ t('auth.mfa.useCode') }}</UiButton>
          </div>
        </form>

        <!-- The administrator requires two-step verification and this account has none. -->
        <div v-else-if="step === 'enroll'" class="auth__form" data-auth-form="enroll">
          <div class="auth__enroll">
            <h2 class="auth__enroll-title">{{ t('auth.enroll.stepsTitle') }}</h2>
            <ol class="auth__steps">
              <li>{{ t('auth.enroll.step1') }}</li>
              <li>{{ t('auth.enroll.step2') }}</li>
              <li>{{ t('auth.enroll.step3') }}</li>
            </ol>
            <p class="auth__note">
              <UiIcon :icon="Smartphone" :size="16" />
              <span>{{ t('auth.enroll.app') }}</span>
            </p>
          </div>
          <UiButton size="lg" block :icon="ChevronLeft" data-auth-back @click="backToPassword">{{ t('auth.enroll.back') }}</UiButton>
        </div>

        <div v-if="enableMockLogin && step === 'password' && mode === 'login'" class="auth__dev">
          <p class="auth__dev-title">{{ t('auth.dev.title') }}</p>
          <div class="auth__row">
            <UiButton size="sm" @click="mockLogin('user')">{{ t('auth.dev.user') }}</UiButton>
            <UiButton size="sm" @click="mockLogin('admin')">{{ t('auth.dev.admin') }}</UiButton>
          </div>
        </div>
      </section>

      <LocaleSwitcher class="auth__locale" compact />
    </div>
  </main>
</template>

<script setup>
// Sign-in (plan §4.3): one centred card on a faint brand-tinted backdrop.
// Steps: email and password → (when the account has it) the 6-digit code,
// or a recovery code when the server lists the "backup" method → home.
// When the administrator requires two-step verification and the account has
// none, the server answers mfa_enrollment_required without a token; the
// /user/mfa/* setup endpoints need a signed-in session, so the card explains
// what to do instead of starting a setup it cannot finish.
// "忘记密码" is not offered: the backend has no password-reset endpoint.
// Registration (when the public config allows it) asks for an invite code
// only when auth.registration.require_invite is on.
import { computed, nextTick, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ChevronLeft, CircleAlert, Smartphone } from '@lucide/vue'
import { useUserStore } from '@/stores/user'
import { useAppI18n } from '@/composables/useAppI18n'
import { login, register } from '@/api/auth'
import { useEdition } from '@/composables/useEdition'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import BrandLockup from '@/components/common/BrandLockup.vue'
import UiButton from '@/ui/UiButton.vue'
import UiIcon from '@/ui/UiIcon.vue'
import UiOtpField from '@/ui/UiOtpField.vue'
import UiPasswordField from '@/ui/UiPasswordField.vue'
import UiTextField from '@/ui/UiTextField.vue'
import { useToast } from '@/ui/composables/useToast'

const router = useRouter()
const userStore = useUserStore()
const toast = useToast()
const { t } = useAppI18n()
const { requireInvite, registrationEnabled, loadEdition } = useEdition()

const titleId = 'login-page-title'
const mode = ref('login') // 'login' | 'register'
const step = ref('password') // 'password' | 'mfa' | 'recovery' | 'enroll'
const email = ref('')
const password = ref('')
const confirmPassword = ref('')
const inviteCode = ref('')
const code = ref('')
const recoveryCode = ref('')
const loading = ref(false)
const formError = ref('')
const errors = reactive({ email: '', password: '', confirm: '', invite: '', code: '', recovery: '' })
const challenge = reactive({ email: '', methods: [] })

const headingRef = ref(null)
const emailRef = ref(null)
const otpRef = ref(null)
const recoveryRef = ref(null)

const challengeEmail = computed(() => challenge.email || email.value)
const canUseRecovery = computed(() => challenge.methods.includes('backup'))

const heading = computed(() => {
  if (step.value === 'mfa') return { title: t('auth.mfa.title') }
  if (step.value === 'recovery') return { title: t('auth.mfa.recoveryTitle'), subtitle: t('auth.mfa.recoveryDescription') }
  if (step.value === 'enroll') return { title: t('auth.enroll.title') }
  if (mode.value === 'register') return { title: t('auth.register.title'), subtitle: t('auth.register.subtitle') }
  return { title: t('auth.signIn.title'), subtitle: t('auth.signIn.subtitle') }
})

const enableMockLogin = computed(() => {
  const flag = String(import.meta.env.VITE_ENABLE_MOCK_LOGIN || '').trim().toLowerCase()
  return flag === 'true' || flag === '1' || flag === 'yes'
})

// Registration closed after the page loaded: go back to signing in.
watch(registrationEnabled, (enabled) => {
  if (!enabled && mode.value === 'register') switchMode()
})

onMounted(() => {
  void loadEdition()
})

function clearErrors() {
  formError.value = ''
  for (const key of Object.keys(errors)) errors[key] = ''
}

// Move focus to what the new step needs: the code boxes, the recovery
// field, or the heading (so a screen reader hears the new title).
function focusStep() {
  nextTick(() => {
    if (step.value === 'mfa') otpRef.value?.focus()
    else if (step.value === 'recovery') recoveryRef.value?.focus()
    else if (step.value === 'enroll') headingRef.value?.focus()
    else emailRef.value?.focus()
  })
}

function goTo(next) {
  clearErrors()
  code.value = ''
  recoveryCode.value = ''
  step.value = next
  focusStep()
}

function backToPassword() {
  challenge.email = ''
  challenge.methods = []
  password.value = ''
  goTo('password')
}

function switchMode() {
  mode.value = mode.value === 'register' ? 'login' : 'register'
  password.value = ''
  confirmPassword.value = ''
  inviteCode.value = ''
  goTo('password')
}

const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

function validateEmail() {
  if (!email.value) {
    // Only complain on blur once the user typed something; submit checks again.
    return true
  }
  errors.email = EMAIL_PATTERN.test(email.value) ? '' : t('auth.errors.emailInvalid')
  return !errors.email
}

function requireEmailAndPassword() {
  errors.email = !email.value ? t('auth.errors.emailRequired') : (EMAIL_PATTERN.test(email.value) ? '' : t('auth.errors.emailInvalid'))
  errors.password = password.value ? '' : t('auth.errors.passwordRequired')
  return !errors.email && !errors.password
}

// --- server answers -------------------------------------------------------
function payloadOf(res, fallback) {
  if (typeof res?.code === 'number' && res.code !== 0) {
    throw new Error(res.msg || fallback)
  }
  return res?.data && typeof res.data === 'object' ? res.data : res
}

function permissionFields(payload) {
  const fields = {}
  for (const field of ['permission_mode', 'permissions', 'restricted_plugins']) {
    if (Object.prototype.hasOwnProperty.call(payload || {}, field)) fields[field] = payload[field]
  }
  return fields
}

function serverMessage(error) {
  return error?.response?.data?.msg || error?.response?.data?.message || error?.message || ''
}

function isRateLimited(error, message) {
  return error?.response?.status === 429 || /too many/i.test(message)
}

// axios: no response at all (offline, refused, timed out).
function isNetworkError(error) {
  return Boolean(error?.isAxiosError || error?.request) && !error?.response
}

function signInWith(payload) {
  const { token, is_admin: isAdmin, user_id: id, email: userEmail } = payload
  if (!token) throw new Error(t('auth.errors.network'))
  userStore.login(token, { id, email: userEmail, is_admin: isAdmin, ...permissionFields(payload) })
  router.push(isAdmin ? '/admin/dashboard' : '/user/dashboard')
}

function credentials() {
  return { email: email.value, password: password.value }
}

// --- sign in ----------------------------------------------------------------
async function submitPassword() {
  clearErrors()
  if (!requireEmailAndPassword()) return
  loading.value = true
  try {
    const payload = payloadOf(await login(credentials()), t('auth.errors.network'))
    if (payload?.mfa_enrollment_required) {
      challenge.email = payload.email || email.value
      goTo('enroll')
      return
    }
    if (payload?.mfa_required) {
      challenge.email = payload.email || email.value
      challenge.methods = Array.isArray(payload.methods) ? payload.methods : (Array.isArray(payload.mfa_methods) ? payload.mfa_methods : [])
      goTo('mfa')
      return
    }
    signInWith(payload)
  } catch (error) {
    const message = serverMessage(error)
    if (isRateLimited(error, message)) formError.value = t('auth.errors.rateLimited')
    else if (isNetworkError(error)) formError.value = t('auth.errors.network')
    else formError.value = t('auth.errors.signInFailed', { message: message || t('auth.errors.network') })
  } finally {
    loading.value = false
  }
}

async function submitSecondFactor(fields, field) {
  loading.value = true
  try {
    const payload = payloadOf(await login({ ...credentials(), ...fields }), t('auth.errors.network'))
    if (payload?.mfa_required || payload?.mfa_enrollment_required) {
      errors[field] = field === 'code' ? t('auth.errors.codeInvalid') : t('auth.errors.recoveryInvalid')
      return
    }
    signInWith(payload)
  } catch (error) {
    const message = serverMessage(error)
    if (isRateLimited(error, message)) errors[field] = t('auth.errors.rateLimited')
    else if (/invalid mfa code/i.test(message)) errors[field] = field === 'code' ? t('auth.errors.codeInvalid') : t('auth.errors.recoveryInvalid')
    else if (isNetworkError(error)) errors[field] = t('auth.errors.network')
    else errors[field] = t('auth.errors.signInFailed', { message: message || t('auth.errors.network') })
    if (field === 'code') {
      otpRef.value?.clear()
    } else {
      nextTick(() => recoveryRef.value?.select?.())
    }
  } finally {
    loading.value = false
  }
}

async function submitCode(value) {
  if (loading.value) return
  const digits = String(typeof value === 'string' ? value : code.value).replace(/\D/g, '')
  if (digits.length !== 6) {
    errors.code = t('auth.errors.codeIncomplete')
    otpRef.value?.focus()
    return
  }
  errors.code = ''
  await submitSecondFactor({ mfa_code: digits, mfa_method: 'totp' }, 'code')
}

// Recovery codes are XXXX-XXXX from A-Z and 0-9 (identity/account/mfa.go)
// and compared exactly: upper-case them and restore the hyphen.
function formatRecovery(value) {
  const clean = String(value || '').toUpperCase().replace(/[^A-Z0-9]/g, '')
  if (clean.length !== 8) return String(value || '').toUpperCase().trim()
  return `${clean.slice(0, 4)}-${clean.slice(4)}`
}

async function submitRecovery() {
  const formatted = formatRecovery(recoveryCode.value)
  recoveryCode.value = formatted
  if (!/^[A-Z0-9]{4}-[A-Z0-9]{4}$/.test(formatted)) {
    errors.recovery = t('auth.errors.recoveryFormat')
    recoveryRef.value?.focus()
    return
  }
  errors.recovery = ''
  await submitSecondFactor({ mfa_code: formatted, mfa_method: 'backup' }, 'recovery')
}

// --- register ---------------------------------------------------------------
async function submitRegister() {
  clearErrors()
  const valid = requireEmailAndPassword()
  if (password.value && password.value.length < 6) errors.password = t('auth.errors.passwordShort')
  if (password.value !== confirmPassword.value) errors.confirm = t('auth.errors.passwordMismatch')
  if (requireInvite.value && !inviteCode.value) errors.invite = t('auth.errors.inviteRequired')
  if (!valid || errors.password || errors.confirm || errors.invite) return

  loading.value = true
  try {
    const payload = payloadOf(await register({
      ...credentials(),
      ...(requireInvite.value && inviteCode.value ? { invite_code: inviteCode.value } : {})
    }), t('auth.errors.network'))
    toast.success(t('auth.register.done'))
    signInWith(payload)
  } catch (error) {
    const message = serverMessage(error)
    if (isNetworkError(error)) formError.value = t('auth.errors.network')
    else formError.value = t('auth.errors.registerFailed', { message: message || t('auth.errors.network') })
  } finally {
    loading.value = false
  }
}

function mockLogin(role) {
  if (!enableMockLogin.value) return
  userStore.login(`mock-token-${role}`, {
    id: 1,
    email: role === 'admin' ? 'admin@example.com' : 'user@example.com',
    is_admin: role === 'admin'
  })
  router.push(role === 'admin' ? '/admin/dashboard' : '/user/dashboard')
}

defineExpose({ mode, step })
</script>

<style scoped>
/* Brand moment (guidelines/color.md rule 4): the only gradient is the
   backdrop wash and the mark tile; text and controls stay neutral. */
.auth {
  display: grid;
  min-height: 100vh;
  min-height: 100dvh;
  place-items: center;
  padding: var(--space-10) var(--space-4);
  background:
    radial-gradient(60% 50% at 20% 0%, color-mix(in srgb, var(--brand-gradient-start) 14%, transparent), transparent 70%),
    radial-gradient(50% 50% at 90% 10%, color-mix(in srgb, var(--brand-gradient-end) 12%, transparent), transparent 70%),
    var(--bg);
}

.auth:focus {
  outline: none;
}

.auth__stack {
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
  align-items: center;
  width: 100%;
}

.auth__card {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
  width: min(400px, 100%);
  padding: var(--space-10) var(--space-8);
  border-radius: var(--radius-lg);
  background: var(--bg-elevated);
  box-shadow: 0 0 0 0.5px var(--separator), var(--shadow-2);
}

.auth__header {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  align-items: center;
  text-align: center;
}

.auth__tile {
  margin-bottom: var(--space-1);
  filter: drop-shadow(0 8px 20px color-mix(in srgb, var(--brand-gradient-end) 30%, transparent));
}

.auth__title {
  font-size: var(--type-title-2-size);
  font-weight: var(--type-title-2-weight);
  line-height: var(--type-title-2-line);
  letter-spacing: var(--type-title-2-tracking);
}

.auth__title:focus {
  outline: none;
}

.auth__title:focus-visible {
  border-radius: var(--radius-xs);
  outline: 2px solid var(--accent);
  outline-offset: 4px;
}

.auth__subtitle {
  color: var(--label-2);
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
  overflow-wrap: anywhere;
}

.auth__email {
  color: var(--label-1);
  font-weight: var(--weight-medium);
}

.auth__form {
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
}

.auth__alert {
  display: flex;
  gap: var(--space-2);
  align-items: flex-start;
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-sm);
  background: var(--danger-soft);
  /* 4.5:1 on the tinted fill in both themes. */
  color: color-mix(in srgb, var(--danger) 80%, var(--label-1));
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
}

.auth__alert :deep(.ui-icon) {
  flex: none;
  margin-top: 2px;
}

.auth__switch {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1);
  align-items: center;
  justify-content: center;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.auth__row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3);
  justify-content: space-between;
}

.auth__recovery :deep(input) {
  font-family: var(--font-mono);
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.auth__enroll {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-5);
  border-radius: var(--radius-md);
  background: var(--bg-grouped);
}

.auth__enroll-title {
  font-size: var(--type-body-size);
  font-weight: var(--weight-semibold);
}

.auth__steps {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding-left: var(--space-5);
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
}

.auth__note {
  display: flex;
  gap: var(--space-2);
  align-items: flex-start;
  color: var(--label-2);
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
}

.auth__note :deep(.ui-icon) {
  flex: none;
  margin-top: 2px;
}

.auth__dev {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding-top: var(--space-5);
  border-top: 1px solid var(--separator);
}

.auth__dev-title {
  color: var(--label-2);
  font-size: var(--type-caption-size);
  text-align: center;
}

@media (max-width: 639px) {
  .auth {
    align-items: start;
    padding: var(--space-8) var(--space-4);
  }

  .auth__card {
    padding: var(--space-8) var(--space-5);
  }
}
</style>
