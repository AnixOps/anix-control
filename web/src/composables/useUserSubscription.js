// The signed-in user's subscription for 概览 and 订阅: the summary from
// GET /user/subscription (plan, traffic, expiry, link settings; the kernel
// caches it for 30 s and ?refresh=true rebuilds it) and the subscription
// token from GET /user/profile (the user store keeps it). No new endpoints.
import { computed, ref, watch } from 'vue'
import { getSubscription } from '@/api/user'
import { useUserStore } from '@/stores/user'
import { unwrapPanel } from '@/utils/panelResponse'

const DAY = 86400

function currentHost() {
  return typeof window !== 'undefined' ? window.location.host : ''
}

function currentProtocol() {
  return typeof window !== 'undefined' ? window.location.protocol : 'https:'
}

function toNumber(value) {
  const n = Number(value)
  return Number.isFinite(n) ? n : 0
}

// subscriptionStatus: 'expired' wins, then 'none' (no plan), 'exhausted'
// (a plan whose traffic is used up, or a plan without traffic, which serves
// nothing: packages/plan/native/assign.go), else 'active'.
export function subscriptionStatus(summary) {
  if (!summary) return 'none'
  if (summary.is_expired) return 'expired'
  if (summary.plan_id === null || summary.plan_id === undefined || summary.plan_id === 0) return 'none'
  const total = toNumber(summary.transfer_enable)
  const used = toNumber(summary.used_traffic)
  if (total <= 0 || used >= total) return 'exhausted'
  return 'active'
}

export function useUserSubscription() {
  const userStore = useUserStore()
  const summary = ref(null)
  const loading = ref(true)
  const refreshing = ref(false)
  const error = ref(null)
  const selectedDomain = ref(currentHost())

  const token = computed(() => String(userStore.userInfo?.token || '').trim())

  async function ensureToken() {
    if (token.value || !userStore.isLoggedIn) return
    await userStore.getUserInfo()
  }

  async function load({ refresh = false } = {}) {
    if (refresh) refreshing.value = true
    else loading.value = true
    error.value = null
    try {
      const [body] = await Promise.all([getSubscription(refresh), ensureToken()])
      summary.value = unwrapPanel(body) || {}
      return true
    } catch (failure) {
      // A failed refresh keeps the numbers already on screen.
      if (!refresh || !summary.value) error.value = failure
      return false
    } finally {
      loading.value = false
      refreshing.value = false
    }
  }

  const total = computed(() => Math.max(0, toNumber(summary.value?.transfer_enable)))
  const used = computed(() => Math.max(0, toNumber(summary.value?.used_traffic)))
  const upload = computed(() => Math.max(0, toNumber(summary.value?.upload_traffic)))
  const download = computed(() => Math.max(0, toNumber(summary.value?.download_traffic)))
  const remaining = computed(() => Math.max(0, total.value - used.value))
  // Share of the traffic left, 0…1.
  const remainingRatio = computed(() => (total.value > 0 ? Math.min(1, remaining.value / total.value) : 0))
  const status = computed(() => subscriptionStatus(summary.value))
  const hasPlan = computed(() => status.value !== 'none')
  const planName = computed(() => (hasPlan.value ? String(summary.value?.plan_name || '') : ''))
  const expiresAt = computed(() => toNumber(summary.value?.expired_at))
  // Whole days left (the kernel's days_remaining, recomputed when absent).
  const daysLeft = computed(() => {
    if (expiresAt.value <= 0) return null
    const fromServer = summary.value?.days_remaining
    if (typeof fromServer === 'number' && fromServer >= 0) return fromServer
    return Math.max(0, Math.floor((expiresAt.value - Date.now() / 1000) / DAY))
  })
  const cachedAt = computed(() => summary.value?.cached_at || null)

  const path = computed(() => {
    const value = String(summary.value?.subscribe_path || summary.value?.SubscribePath || '/s').trim() || '/s'
    return value.startsWith('/') ? value.replace(/\/+$/, '') : `/${value.replace(/\/+$/, '')}`
  })

  const domains = computed(() => {
    const configured = summary.value?.subscribe_domains || summary.value?.SubscribeDomains
    if (Array.isArray(configured) && configured.length > 0) return configured.map(String).filter(Boolean)
    return [currentHost()]
  })

  watch(domains, (list) => {
    if (!list.length || list.includes(selectedDomain.value)) return
    selectedDomain.value = list.includes(currentHost()) ? currentHost() : list[0]
  }, { immediate: true })

  // The link a client imports; `format` adds ?type= (the kernel otherwise
  // picks the format from the client's User-Agent).
  function linkFor(format = '', host = selectedDomain.value) {
    if (!token.value) return ''
    const base = `${currentProtocol()}//${host || currentHost()}${path.value}/${token.value}`
    if (!format || format === 'auto' || format === 'ua') return base
    return `${base}?type=${encodeURIComponent(format)}`
  }

  const link = computed(() => linkFor())

  return {
    summary,
    loading,
    refreshing,
    error,
    token,
    load,
    total,
    used,
    upload,
    download,
    remaining,
    remainingRatio,
    status,
    hasPlan,
    planName,
    expiresAt,
    daysLeft,
    cachedAt,
    domains,
    selectedDomain,
    link,
    linkFor
  }
}
