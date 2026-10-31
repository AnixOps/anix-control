// Node groups (5.0 node onboarding): how a group is edited in the console and
// sent to /api/v4/kernel/node-groups. A group owns a join token and the
// defaults of its nodes; the nodes join clean, without protocols. No
// requests here. The server checks everything again (internal/nodegroup);
// these helpers only shape the form and the body.

export const BYTES_PER_GB = 1024 * 1024 * 1024

// Options of the token validity select, in days; 'never' and 'keep' are not.
export const TOKEN_EXPIRY_DAYS = Object.freeze({ '7d': 7, '30d': 30, '90d': 90, '365d': 365 })

// The form of a group (the edit sheet), or of a new one.
export function groupFormFrom(group, { now = Date.now() } = {}) {
  if (!group) {
    return {
      name: '',
      rate: 1,
      tagsText: '',
      monthlyLimitGb: null,
      resetDay: 1,
      sort: 0,
      maxNodes: 0,
      requireApproval: false,
      expiry: 'never',
      currentExpiry: null
    }
  }
  const expires = group.token_expires_at ? Date.parse(group.token_expires_at) : NaN
  const hasExpiry = Number.isFinite(expires)
  let expiry = 'never'
  if (hasExpiry) expiry = group.token_expired || expires <= now ? 'expired' : 'keep'
  return {
    name: group.name || '',
    rate: Number(group.rate) || 1,
    tagsText: Array.isArray(group.tags) ? group.tags.join(', ') : '',
    monthlyLimitGb: group.monthly_limit_bytes ? group.monthly_limit_bytes / BYTES_PER_GB : null,
    resetDay: Number(group.reset_day) || 1,
    sort: Number(group.sort) || 0,
    maxNodes: Number(group.max_nodes) || 0,
    requireApproval: Boolean(group.require_approval),
    expiry,
    currentExpiry: hasExpiry ? group.token_expires_at : null
  }
}

function splitTags(text) {
  return [...new Set(String(text || '').split(/[,，]/).map(tag => tag.trim()).filter(Boolean))]
}

// The token's new expiry for the form's choice: null (never), the group's own
// when kept, or now plus the chosen number of days. An expired token must be
// given a new one: the API refuses a past time.
export function expiryFor(form, now = Date.now()) {
  if (form.expiry === 'never') return null
  if (form.expiry === 'keep') return form.currentExpiry
  const days = TOKEN_EXPIRY_DAYS[form.expiry]
  return days ? new Date(now + days * 86_400_000).toISOString() : null
}

// The body of POST/PUT /kernel/node-groups.
export function buildGroupInput(form, { now = Date.now() } = {}) {
  const input = {
    name: String(form.name || '').trim(),
    max_nodes: Number(form.maxNodes) || 0,
    require_approval: Boolean(form.requireApproval),
    rate: Number(form.rate),
    tags: splitTags(form.tagsText),
    monthly_limit_bytes: form.monthlyLimitGb ? Math.round(Number(form.monthlyLimitGb) * BYTES_PER_GB) : 0,
    reset_day: Number(form.resetDay) || 1,
    sort: Number(form.sort) || 0,
    token_expires_at: expiryFor(form, now)
  }
  return input
}

// A group as an unchanged update body, for the toggles of the list: the same
// fields the API replaces, taken from the group as it was listed.
export function inputFromGroup(group, overrides = {}) {
  return {
    name: group.name,
    max_nodes: group.max_nodes || 0,
    require_approval: Boolean(group.require_approval),
    rate: group.rate,
    tags: Array.isArray(group.tags) ? group.tags : [],
    monthly_limit_bytes: group.monthly_limit_bytes || 0,
    reset_day: group.reset_day || 1,
    sort: group.sort || 0,
    token_expires_at: group.token_expires_at || null,
    ...overrides
  }
}

// The body of the inline "new group" form of the join dialog: a name.
export function buildQuickGroupInput({ name }) {
  return { name: String(name || '').trim() }
}

// The groups of an answer of GET /kernel/node-groups.
export function readNodeGroups(answer) {
  if (Array.isArray(answer)) return answer
  return Array.isArray(answer?.groups) ? answer.groups : []
}

export function readNodeGroupNodes(answer) {
  if (Array.isArray(answer)) return answer
  return Array.isArray(answer?.nodes) ? answer.nodes : []
}

// The API's error as one line: kernel errors are { error: { code, message } }.
export function readKernelError(error, fallback = '') {
  const body = error?.response?.data
  const detail = body?.error
  return {
    status: error?.response?.status || 0,
    code: (detail && typeof detail === 'object' ? detail.code : '') || '',
    message: (detail && typeof detail === 'object' ? detail.message : (typeof detail === 'string' ? detail : '')) || body?.message || error?.message || fallback
  }
}
