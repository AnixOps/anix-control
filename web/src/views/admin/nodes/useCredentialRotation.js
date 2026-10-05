// State of "rotate Agent credentials" (POST /api/v4/kernel/agents/rotate-credentials):
// the confirmation form, the request, and the one-time credential it answers.
//
// The credential is the only secret in here. It lives in `result` for as long
// as the result dialog is open and nowhere else: not in storage, not in the
// URL, not in a log, not in an event. Closing the dialog, cancelling, and
// leaving the page all clear it (`dismiss`, `cancel`, unmount), and a request
// that answers after that is dropped.
import { computed, onScopeDispose, reactive, ref, shallowRef } from 'vue'
import { rotateKernelAgentCredentials } from '@/api/kernel'
import { adminV4ErrorMessage } from '@/utils/adminV4'

// The route's bounds: a reason of at most 200 bytes (the server counts the
// UTF-8 bytes of the trimmed text), a lifetime of 60 s to 7 days.
export const ROTATE_REASON_MAX = 200
export const ROTATE_TTL_SECONDS = Object.freeze([3600, 6 * 3600, 24 * 3600, 7 * 24 * 3600])
export const DEFAULT_ROTATE_TTL = 3600

// 'proxy' or 'forward' from the inventory name ("proxy-12", "forward-3").
export function agentNodeKind(node) {
  return String(node || '').startsWith('forward-') ? 'forward' : 'proxy'
}

export function utf8Length(text) {
  return new TextEncoder().encode(String(text ?? '')).length
}

function codeOf(cause) {
  const failure = cause?.response?.data?.error
  return (failure && typeof failure === 'object' ? failure.code : '') || ''
}

// What the page keeps of the answer. The credential is a plain string here;
// `expiresAt` is a time in milliseconds.
function readRotation(answer, fallbackTtl) {
  const expires = Date.parse(answer?.expires_at || '')
  return {
    node: String(answer?.node || ''),
    credential: String(answer?.credential || ''),
    expiresAt: Number.isFinite(expires) ? expires : Date.now() + fallbackTtl * 1000,
    apiKeyRotated: Boolean(answer?.api_key_rotated),
    revoked: {
      certificates: Number(answer?.revoked?.certificates || 0),
      enrollments: Number(answer?.revoked?.enrollments || 0),
      linkCertificates: Number(answer?.revoked?.link_certificates || 0)
    }
  }
}

// `node` is the inventory name or a getter of it; `t` / `te` translate the
// refusals; `onRotated` is told that a rotation happened (never the secret).
export function useCredentialRotation(node, { t, te, onRotated } = {}) {
  const nodeName = () => (typeof node === 'function' ? node() : node)

  const stage = ref('idle') // 'idle' | 'confirm' | 'result'
  const form = reactive({ reason: '', ttlSeconds: DEFAULT_ROTATE_TTL, rotateApiKey: false })
  const busy = ref(false)
  const error = ref('')
  const result = shallowRef(null)
  // null until known; false after the route said 403.
  const superAdmin = ref(null)
  let attempt = 0

  const kind = computed(() => agentNodeKind(nodeName()))
  const reasonBytes = computed(() => utf8Length(form.reason.trim()))
  const reasonTooLong = computed(() => reasonBytes.value > ROTATE_REASON_MAX)
  
  function resetForm() {
    form.reason = ''
    form.ttlSeconds = DEFAULT_ROTATE_TTL
    form.rotateApiKey = false
  }

  function clearSecret() {
    attempt += 1
    result.value = null
  }

  function open() {
    clearSecret()
    resetForm()
    busy.value = false
    error.value = ''
    stage.value = 'confirm'
  }

  // The confirmation closed without rotating.
  function cancel() {
    if (busy.value) return
    clearSecret()
    resetForm()
    error.value = ''
    stage.value = 'idle'
  }

  // The result dialog closed: the credential is gone for good.
  function dismiss() {
    clearSecret()
    resetForm()
    busy.value = false
    error.value = ''
    stage.value = 'idle'
  }

  function describe(cause) {
    const code = codeOf(cause)
    const message = adminV4ErrorMessage(cause, '')
    if (code && te && te(`admin.nodes.rotate.errors.${code}`)) return t(`admin.nodes.rotate.errors.${code}`)
    return t ? t('admin.nodes.rotate.errors.failed', { message: message || cause?.message || '' }) : message
  }

  async function submit() {
    if (busy.value || reasonTooLong.value) return false
    if (superAdmin.value === false) {
      error.value = t ? t('admin.nodes.rotate.errors.super_admin_required') : ''
      return false
    }
    const mine = ++attempt
    busy.value = true
    error.value = ''
    try {
      const answer = await rotateKernelAgentCredentials({
        node: nodeName(),
        rotateApiKey: kind.value === 'proxy' && form.rotateApiKey,
        ttlSeconds: form.ttlSeconds,
        reason: form.reason
      })
      if (mine !== attempt) return false
      const rotation = readRotation(answer, form.ttlSeconds)
      if (!rotation.credential) throw new Error('the answer carried no credential')
      result.value = rotation
      resetForm()
      stage.value = 'result'
      onRotated?.({ node: rotation.node, apiKeyRotated: rotation.apiKeyRotated })
      return true
    } catch (cause) {
      if (mine !== attempt) return false
      if (cause?.response?.status === 403 || codeOf(cause) === 'super_admin_required') superAdmin.value = false
      error.value = describe(cause)
      return false
    } finally {
      if (mine === attempt) busy.value = false
    }
  }

  onScopeDispose(() => {
    clearSecret()
    resetForm()
  })

  return { stage, form, busy, error, result, superAdmin, kind, reasonBytes, reasonTooLong, open, cancel, dismiss, submit }
}
