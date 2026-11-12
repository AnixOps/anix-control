// State of "create an API token" (POST /api/v4/kernel/api-tokens): the form
// and the one-time token it answers.
//
// The token is the only secret in here. It lives in `result` for as long as
// the result dialog is open and nowhere else: not in storage, not in the URL,
// not in a log, not in an event (`onCreated` gets the stored record, which
// holds the last four characters, never the token). Closing the dialog
// (`dismiss`: Done, Esc, the close button), cancelling the form and leaving
// the page (the end of the scope) all clear it, and an answer that arrives
// after that is dropped.
//
// The route asks for no password: identity holds the credentials, so the kernel
// only checks that the sign-in is at most ten minutes old, and a stale one
// offers a new sign-in. `t` translates the messages (keys under adminApiTokens.create.*).
import { computed, onScopeDispose, reactive, ref, shallowRef } from 'vue'
import { createKernelApiToken } from '@/api/kernel'
import {
  DEFAULT_EXPIRY, checkTokenName, classifyTokenRefusal, expiryDays, readApiToken
} from './apiTokens'

function blankForm() {
  return { name: '', scope: 'read', expiry: DEFAULT_EXPIRY, customDays: '' }
}

export function useApiTokenCreate({ t, onCreated } = {}) {
  const say = (key, params) => (t ? t(`adminApiTokens.create.${key}`, params) : key)

  const stage = ref('idle') // 'idle' | 'form' | 'result'
  const form = reactive(blankForm())
  const busy = ref(false)
  const nameError = ref('')
  const expiryError = ref('')
  // A refusal that belongs to no field, and whether it asks for a new sign-in.
  const error = ref('')
  const signInAgain = ref(false)
  const result = shallowRef(null)
  let attempt = 0

  const days = computed(() => expiryDays(form.expiry, form.customDays))
  const neverExpires = computed(() => form.expiry === 'never')

  function clearErrors() {
    nameError.value = ''
    expiryError.value = ''
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

  function open() {
    clearSecret()
    reset()
    stage.value = 'form'
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

  // What went wrong, said in the place it belongs; returns the field to focus.
  function refused(cause) {
    const refusal = classifyTokenRefusal(cause)
    switch (refusal.kind) {
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
  // `focus` naming the field to move to ('name', 'expiry', '').
  async function submit() {
    const outcome = { ok: false, focus: '' }
    if (busy.value || stage.value !== 'form') return outcome
    clearErrors()
    const nameProblem = checkTokenName(form.name)
    if (nameProblem) nameError.value = say(`errors.name.${nameProblem}`)
    const chosenDays = days.value
    if (Number.isNaN(chosenDays)) expiryError.value = say('errors.expiry')
    if (nameProblem) return { ...outcome, focus: 'name' }
    if (Number.isNaN(chosenDays)) return { ...outcome, focus: 'expiry' }

    const mine = ++attempt
    busy.value = true
    try {
      const answer = await createKernelApiToken({
        name: form.name,
        scope: form.scope,
        expiresInDays: chosenDays
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
    stage, form, busy, nameError, expiryError, error, signInAgain, result, days, neverExpires,
    open, cancel, dismiss, submit
  }
}
