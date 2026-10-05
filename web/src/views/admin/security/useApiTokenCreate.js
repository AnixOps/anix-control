// State of "create an API token" (POST /api/v4/kernel/api-tokens): the form,
// the re-authentication it needs, and the one-time token it answers.
//
// The token is the only secret in here. It lives in `result` for as long as
// the result dialog is open and nowhere else: not in storage, not in the URL,
// not in a log, not in an event (`onCreated` gets the stored record, which
// holds the last four characters, never the token). Closing the dialog
// (`dismiss`: Done, Esc, the close button), cancelling the form and leaving
// the page (the end of the scope) all clear it, and an answer that arrives
// after that is dropped. The password and codes the form takes are cleared
// the same way, and after a refusal of the credential.
//
// Re-authentication follows the subscription reset (views/user/Subscribe.vue):
// the account's two-step status picks the password or a six-digit code (with a
// switch to a recovery code), and the route's own refusals move between them.
// `t` translates the messages (keys under adminApiTokens.create.*).
import { computed, onScopeDispose, reactive, ref, shallowRef } from 'vue'
import { createKernelApiToken } from '@/api/kernel'
import { getMfaStatus } from '@/api/user'
import { unwrapPanel } from '@/utils/panelResponse'
import {
  DEFAULT_EXPIRY, checkTokenName, classifyTokenRefusal, expiryDays, formatRecoveryCode, isRecoveryCode, readApiToken
} from './apiTokens'

function blankForm() {
  return { name: '', scope: 'read', expiry: DEFAULT_EXPIRY, customDays: '', password: '', code: '', recovery: '' }
}

export function useApiTokenCreate({ t, onCreated } = {}) {
  const say = (key, params) => (t ? t(`adminApiTokens.create.${key}`, params) : key)

  const stage = ref('idle') // 'idle' | 'form' | 'result'
  // What the credential field asks: 'checking' (reading the account's two-step
  // status), then 'password', 'code' or 'recovery'.
  const step = ref('checking')
  const form = reactive(blankForm())
  const busy = ref(false)
  const nameError = ref('')
  const expiryError = ref('')
  const credentialError = ref('')
  // A refusal that belongs to no field, and whether it asks for a new sign-in.
  const error = ref('')
  const signInAgain = ref(false)
  const result = shallowRef(null)
  let attempt = 0

  const days = computed(() => expiryDays(form.expiry, form.customDays))
  const neverExpires = computed(() => form.expiry === 'never')

  function clearCredentials() {
    form.password = ''
    form.code = ''
    form.recovery = ''
  }

  function clearErrors() {
    nameError.value = ''
    expiryError.value = ''
    credentialError.value = ''
    error.value = ''
    signInAgain.value = false
  }

  function clearSecret() {
    attempt += 1
    result.value = null
  }

  function reset() {
    Object.assign(form, blankForm())
    clearErrors()
    busy.value = false
  }

  async function chooseStep(mine) {
    let next = 'password'
    try {
      // With two-step verification on, the route takes a code, not the password.
      if (unwrapPanel(await getMfaStatus())?.enabled === true) next = 'code'
    } catch {
      // Ask for the password; the route says when it wants a code instead.
    }
    if (mine === attempt && stage.value === 'form' && step.value === 'checking') step.value = next
  }

  function open() {
    clearSecret()
    reset()
    step.value = 'checking'
    stage.value = 'form'
    chooseStep(attempt)
  }

  function switchStep(next) {
    step.value = next
    credentialError.value = ''
    form.code = ''
    form.recovery = ''
  }

  // The form closed without creating a token.
  function cancel() {
    if (busy.value) return
    clearSecret()
    reset()
    stage.value = 'idle'
  }

  // The result dialog closed: the token is gone for good.
  function dismiss() {
    clearSecret()
    reset()
    stage.value = 'idle'
  }

  // The credential of the current step, or null after saying what is missing.
  function credentials(value) {
    if (step.value === 'password') {
      if (form.password) return { password: form.password }
      credentialError.value = say('errors.password')
    } else if (step.value === 'code') {
      const digits = String(typeof value === 'string' ? value : form.code).replace(/\D/g, '')
      if (digits.length === 6) return { code: digits, method: 'totp' }
      credentialError.value = say('errors.code')
    } else if (step.value === 'recovery') {
      form.recovery = formatRecoveryCode(form.recovery)
      if (isRecoveryCode(form.recovery)) return { code: form.recovery, method: 'backup' }
      credentialError.value = say('errors.recoveryFormat')
    }
    return null
  }

  // What went wrong, said in the place it belongs; returns the field to focus.
  function refused(cause) {
    const refusal = classifyTokenRefusal(cause)
    switch (refusal.kind) {
      case 'password_required':
        switchStep('password')
        credentialError.value = say('errors.password')
        return 'credential'
      case 'code_required':
        switchStep('code')
        return 'credential'
      case 'step_up_failed':
        credentialError.value = say(step.value === 'recovery' ? 'errors.recoveryWrong' : step.value === 'code' ? 'errors.codeWrong' : 'errors.passwordWrong')
        clearCredentials()
        return 'credential'
      case 'rate_limited':
        credentialError.value = refusal.retryAfter > 0
          ? say('errors.rateLimitedWait', { minutes: Math.max(1, Math.ceil(refusal.retryAfter / 60)) })
          : say('errors.rateLimited')
        clearCredentials()
        return 'credential'
      case 'sign_in_again':
        error.value = say('errors.signInAgain')
        signInAgain.value = true
        return ''
      case 'too_many_tokens':
        error.value = say('errors.too_many_tokens')
        return ''
      case 'not_an_administrator':
        error.value = say('errors.not_an_administrator')
        return ''
      case 'invalid_request':
        error.value = say('errors.invalid_request', { message: refusal.message })
        return ''
      default:
        error.value = say('errors.failed', { message: refusal.message })
        return ''
    }
  }

  // Validates, sends, and moves to the result. Resolves true when a token was
  // created and shown; otherwise false, with the reason in the errors and
  // `focus` naming the field to move to ('name', 'expiry', 'credential', '').
  async function submit(value) {
    const outcome = { ok: false, focus: '' }
    if (busy.value || stage.value !== 'form' || step.value === 'checking') return outcome
    clearErrors()
    const nameProblem = checkTokenName(form.name)
    if (nameProblem) nameError.value = say(`errors.name.${nameProblem}`)
    const chosenDays = days.value
    if (Number.isNaN(chosenDays)) expiryError.value = say('errors.expiry')
    if (nameProblem) return { ...outcome, focus: 'name' }
    if (Number.isNaN(chosenDays)) return { ...outcome, focus: 'expiry' }
    const proof = credentials(value)
    if (!proof) return { ...outcome, focus: 'credential' }

    const mine = ++attempt
    busy.value = true
    try {
      const answer = await createKernelApiToken({
        name: form.name,
        scope: form.scope,
        expiresInDays: chosenDays,
        ...proof
      })
      if (mine !== attempt) return outcome
      const token = String(answer?.token || '')
      if (!token) throw new Error('the answer carried no token')
      const record = readApiToken(answer?.api_token)
      result.value = { token, record }
      reset()
      stage.value = 'result'
      onCreated?.(record)
      return { ok: true, focus: '' }
    } catch (cause) {
      if (mine !== attempt) return outcome
      return { ...outcome, focus: refused(cause) }
    } finally {
      if (mine === attempt) busy.value = false
    }
  }

  onScopeDispose(() => {
    clearSecret()
    reset()
  })

  return {
    stage, step, form, busy, nameError, expiryError, credentialError, error, signInAgain, result, days, neverExpires,
    open, cancel, dismiss, switchStep, submit
  }
}
