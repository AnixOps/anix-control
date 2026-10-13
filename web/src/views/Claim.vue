<template>
  <main id="app-main-content" tabindex="-1" class="claim" :aria-labelledby="titleId">
    <div class="claim__stack">
      <section class="claim__card" :data-claim-state="state" :aria-busy="busy || state === 'loading' ? 'true' : 'false'">
        <header class="claim__header">
          <BrandLockup class="claim__tile" tile mark-only size="lg" style="--lockup-mark: 56px" />
          <h1 :id="titleId" ref="headingRef" class="claim__title" tabindex="-1">{{ heading.title }}</h1>
          <p v-if="heading.subtitle" class="claim__subtitle">{{ heading.subtitle }}</p>
        </header>

        <p v-if="state === 'loading'" class="claim__loading" role="status">
          <UiSpinner :size="20" />
          <span class="visually-hidden">{{ t('claim.loading') }}</span>
        </p>

        <!-- The first administrator's email and password. -->
        <form v-else-if="state === 'form'" class="claim__form" novalidate data-claim-form @submit.prevent="submit">
          <UiTextField
            id="claim-email"
            ref="emailRef"
            v-model.trim="email"
            type="email"
            :label="t('claim.email')"
            autocomplete="username"
            :error="errors.email"
            autocapitalize="off"
            spellcheck="false"
            aria-required="true"
          />
          <UiPasswordField
            id="claim-password"
            v-model="password"
            :label="t('claim.password')"
            :help="t('claim.passwordHelp')"
            autocomplete="new-password"
            :error="errors.password"
            aria-required="true"
          />
          <UiPasswordField
            id="claim-confirm"
            v-model="confirm"
            :label="t('claim.confirm')"
            autocomplete="new-password"
            :error="errors.confirm"
            aria-required="true"
          />

          <p v-if="formError" class="claim__alert" role="alert" data-claim-error>
            <UiIcon :icon="CircleAlert" :size="16" />
            <span>{{ formError }}</span>
          </p>

          <UiButton type="submit" variant="primary" size="lg" block :loading="busy" data-claim-submit>
            {{ t('claim.submit') }}
          </UiButton>
        </form>

        <p v-else-if="state === 'done'" class="claim__loading" role="status">
          <UiSpinner :size="20" />
        </p>

        <!-- Nothing to claim here: why, and what to do. -->
        <div v-else class="claim__note" data-claim-note>
          <code v-if="needsNewLink" class="claim__command" data-claim-command>sudo anix-control admin claim-link</code>
          <UiButton v-else size="lg" block data-claim-sign-in @click="router.push('/login')">
            {{ t('claim.toSignIn') }}
          </UiButton>
        </div>
      </section>
    </div>
  </main>
</template>

<script setup>
// 领取管理员 (/claim): the first administrator's one-time claim link
// (docs/design/single-binary, N10). `anix-control admin claim-link` prints
// https://<domain>/claim#anixclaim_…; the token sits in the fragment, which
// the browser never sends. The page takes it once, removes it from the
// address bar and the history entry, and sends it only in the body of POST
// /api/v5/claim with the email and password. Then it signs in with the
// same email and password through the ordinary sign-in.
//
// States: loading; form (no administrator yet and a link); missing (no
// administrator, but no link in the address); none (nothing to claim,
// GET /api/v5/claim/status says so or the claim answers 404); expired and
// invalid (the claim's answers); done (signing in).
import { computed, nextTick, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { CircleAlert } from '@lucide/vue'
import { getClaimStatus, submitClaim } from '@/api/claim'
import { login } from '@/api/auth'
import { useUserStore } from '@/stores/user'
import { useAppI18n } from '@/composables/useAppI18n'
import BrandLockup from '@/components/common/BrandLockup.vue'
import UiButton from '@/ui/UiButton.vue'
import UiIcon from '@/ui/UiIcon.vue'
import UiPasswordField from '@/ui/UiPasswordField.vue'
import UiSpinner from '@/ui/UiSpinner.vue'
import UiTextField from '@/ui/UiTextField.vue'
import { useToast } from '@/ui/composables/useToast'

const router = useRouter()
const userStore = useUserStore()
const toast = useToast()
const { t } = useAppI18n()

const TOKEN_PATTERN = /^anixclaim_[A-Za-z0-9_-]{43}$/
const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/
const MIN_PASSWORD = 6
const MAX_PASSWORD_BYTES = 72

// takeToken reads the claim token from the fragment once and removes it
// from the address bar and the history entry; it is kept only here, never
// rendered and never put in a URL.
function takeToken() {
  if (typeof window === 'undefined') return ''
  let raw = ''
  try {
    raw = decodeURIComponent((window.location.hash || '').replace(/^#/, '').trim())
  } catch {
    raw = ''
  }
  if (window.location.hash) {
    window.history.replaceState(window.history.state, '', `${window.location.pathname}${window.location.search}`)
  }
  return TOKEN_PATTERN.test(raw) ? raw : ''
}

const token = takeToken()

const titleId = 'claim-page-title'
const state = ref('loading') // loading | form | missing | none | expired | invalid | done
const email = ref('')
const password = ref('')
const confirm = ref('')
const busy = ref(false)
const formError = ref('')
const errors = reactive({ email: '', password: '', confirm: '' })
const headingRef = ref(null)
const emailRef = ref(null)

const needsNewLink = computed(() => ['missing', 'expired', 'invalid'].includes(state.value))

const heading = computed(() => {
  switch (state.value) {
    case 'form':
      return { title: t('claim.title'), subtitle: t('claim.subtitle') }
    case 'done':
      return { title: t('claim.done.title'), subtitle: t('claim.done.subtitle') }
    case 'missing':
    case 'none':
    case 'expired':
    case 'invalid':
      return { title: t(`claim.${state.value}.title`), subtitle: t(`claim.${state.value}.body`) }
    default:
      return { title: t('claim.title'), subtitle: '' }
  }
})

function show(next) {
  state.value = next
  nextTick(() => {
    if (next === 'form') emailRef.value?.focus?.()
    else headingRef.value?.focus()
  })
}

onMounted(async () => {
  let claimable = null
  try {
    claimable = await getClaimStatus()
  } catch {
    // Unknown: a link still gets the form, and the claim answers for itself.
  }
  if (claimable === false) show('none')
  else show(token ? 'form' : 'missing')
})

function clearErrors() {
  formError.value = ''
  for (const key of Object.keys(errors)) errors[key] = ''
}

function utf8Length(value) {
  return new TextEncoder().encode(value).length
}

function validate() {
  errors.email = !email.value ? t('auth.errors.emailRequired') : (EMAIL_PATTERN.test(email.value) ? '' : t('auth.errors.emailInvalid'))
  if (!password.value) errors.password = t('auth.errors.passwordRequired')
  else if ([...password.value].length < MIN_PASSWORD) errors.password = t('auth.errors.passwordShort')
  else if (utf8Length(password.value) > MAX_PASSWORD_BYTES) errors.password = t('claim.errors.passwordLong')
  if (!errors.password && password.value !== confirm.value) errors.confirm = t('auth.errors.passwordMismatch')
  return !errors.email && !errors.password && !errors.confirm
}

// refuse turns the claim's answer into the page's words.
function refuse(error) {
  const response = error?.response
  if (!response) {
    formError.value = t('auth.errors.network')
    return
  }
  const code = response.data?.error?.code || ''
  switch (code) {
    case 'weak_password':
      errors.password = t('auth.errors.passwordShort')
      return
    case 'password_too_long':
      errors.password = t('claim.errors.passwordLong')
      return
    case 'invalid_email':
      errors.email = t('auth.errors.emailInvalid')
      return
    case 'email_taken':
      errors.email = t('claim.errors.emailTaken')
      return
    case 'claim_expired':
      show('expired')
      return
    case 'claim_invalid':
      show('invalid')
      return
    case 'identity_unavailable':
      formError.value = t('claim.errors.identity')
      return
  }
  if (response.status === 404) show('none')
  else if (response.status === 429) formError.value = t('auth.errors.rateLimited')
  else formError.value = t('claim.errors.failed', { message: response.data?.error?.message || response.statusText || response.status })
}

function panelPayload(res) {
  if (typeof res?.code === 'number' && res.code !== 0) return null
  return res?.data && typeof res.data === 'object' ? res.data : res
}

// signIn uses the ordinary sign-in with what was just claimed; anything
// but a token (a network error, a second step) leads to the sign-in page.
async function signIn(credentials) {
  try {
    const payload = panelPayload(await login(credentials))
    if (payload?.token && !payload.mfa_required && !payload.mfa_enrollment_required) {
      userStore.login(payload.token, { id: payload.user_id, email: payload.email, is_admin: payload.is_admin })
      router.push(payload.is_admin ? '/admin/dashboard' : '/user/dashboard')
      return
    }
  } catch {
    // The account exists; signing in by hand works.
  }
  toast.success(t('claim.created'))
  router.push('/login')
}

async function submit() {
  if (busy.value) return
  clearErrors()
  if (!validate()) return
  busy.value = true
  const credentials = { email: email.value, password: password.value }
  try {
    await submitClaim({ token, ...credentials })
  } catch (error) {
    refuse(error)
    busy.value = false
    return
  }
  password.value = ''
  confirm.value = ''
  show('done')
  await signIn(credentials)
  busy.value = false
}

defineExpose({ state })
</script>

<style scoped>
/* The sign-in page's card (Login.vue): one centred card on the faint
   brand-tinted backdrop. */
.claim {
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

.claim:focus {
  outline: none;
}

.claim__stack {
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
  align-items: center;
  width: 100%;
}

.claim__card {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
  width: min(420px, 100%);
  padding: var(--space-10) var(--space-8);
  border-radius: var(--radius-lg);
  background: var(--bg-elevated);
  box-shadow: 0 0 0 0.5px var(--separator), var(--shadow-2);
}

.claim__header {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  align-items: center;
  text-align: center;
}

.claim__tile {
  margin-bottom: var(--space-1);
  filter: drop-shadow(0 8px 20px color-mix(in srgb, var(--brand-gradient-end) 30%, transparent));
}

.claim__title {
  font-size: var(--type-title-2-size);
  font-weight: var(--type-title-2-weight);
  line-height: var(--type-title-2-line);
  letter-spacing: var(--type-title-2-tracking);
}

.claim__title:focus {
  outline: none;
}

.claim__title:focus-visible {
  border-radius: var(--radius-xs);
  outline: 2px solid var(--accent);
  outline-offset: 4px;
}

.claim__subtitle {
  color: var(--label-2);
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
  overflow-wrap: anywhere;
}

.claim__form,
.claim__note {
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
}

.claim__loading {
  display: flex;
  justify-content: center;
  color: var(--label-2);
}

.claim__command {
  display: block;
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-sm);
  background: var(--bg-grouped);
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
  overflow-wrap: anywhere;
  user-select: all;
}

.claim__alert {
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

.claim__alert :deep(.ui-icon) {
  flex: none;
  margin-top: 2px;
}

@media (max-width: 639px) {
  .claim {
    align-items: start;
    padding: var(--space-8) var(--space-4);
  }

  .claim__card {
    padding: var(--space-8) var(--space-5);
  }
}
</style>
