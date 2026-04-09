<template>
  <div class="page subscriptions-page">
    <div class="page-header">
      <h1>{{ t('user.subscribe.title') }}</h1>
      <p class="subtitle">{{ t('user.subscribe.subtitle') }}</p>
    </div>

    <div v-if="!token" class="card">
      <p>{{ t('user.subscribe.missingToken') }}</p>
    </div>

    <div v-else>
      <div class="card">
        <h3>{{ t('user.subscribe.infoTitle') }}</h3>
        <p>{{ t('common.labels.expiresAt') }}: <strong>{{ subscription.ExpireAt ? formatDateTime(subscription.ExpireAt) : t('common.states.permanent') }}</strong></p>
        <p>{{ t('user.subscribe.usedTraffic') }}: <strong>{{ formatBytes(subscription.UsedTraffic || 0) }}</strong></p>
        <p>{{ t('user.subscribe.totalTraffic') }}: <strong>{{ formatBytes(subscription.TotalTraffic || 0) }}</strong></p>
      </div>

      <div class="card">
        <h3>{{ t('user.subscribe.linksTitle') }}</h3>
        <div v-for="fmt in formats" :key="fmt.value" class="link-item">
          <label>{{ fmt.label }}:</label>
          <div class="link-row">
            <input type="text" :value="getSubscribeUrl(fmt.value)" readonly />
            <button class="btn btn-sm" @click="copyText(getSubscribeUrl(fmt.value))">{{ t('user.subscribe.copyLink') }}</button>
            <button class="btn btn-sm" @click="preview(fmt.value)">{{ t('common.actions.preview') }}</button>
          </div>
        </div>
        <div class="link-actions">
          <button class="btn btn-primary" @click="refresh">{{ t('user.subscribe.refreshCache') }}</button>
        </div>
      </div>

      <div v-if="showPreview" class="card preview-card">
        <h3>{{ t('user.subscribe.previewTitle', { format: previewFormat }) }}</h3>
        <div class="preview-controls">
          <button class="btn btn-sm" @click="copyText(previewContent)">{{ t('user.subscribe.copyContent') }}</button>
          <button class="btn btn-sm" @click="downloadPreview">{{ t('common.actions.download') }}</button>
          <button class="btn btn-ghost btn-sm" @click="closePreview">{{ t('user.subscribe.closePreview') }}</button>
        </div>
        <textarea readonly rows="12">{{ previewContent }}</textarea>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useUserStore } from '@/stores/user'
import { getSubscription } from '@/api/user'
import { useAppI18n } from '@/composables/useAppI18n'

const userStore = useUserStore()
const { t, formatDateTime } = useAppI18n()

const subscription = ref({})
const showPreview = ref(false)
const previewContent = ref('')
const previewFormat = ref('')

const token = computed(() => (userStore.userInfo && userStore.userInfo.token) || '')
const formats = computed(() => ([
  { value: 'auto', label: t('user.subscribe.formats.auto') },
  { value: 'v2ray', label: t('user.subscribe.formats.v2ray') },
  { value: 'clash', label: t('user.subscribe.formats.clash') },
  { value: 'stash', label: t('user.subscribe.formats.stash') },
  { value: 'egern', label: t('user.subscribe.formats.egern') },
  { value: 'surge', label: t('user.subscribe.formats.surge') },
  { value: 'loon', label: t('user.subscribe.formats.loon') },
  { value: 'shadowrocket', label: t('user.subscribe.formats.shadowrocket') },
  { value: 'quantumultx', label: t('user.subscribe.formats.quantumultx') },
  { value: 'sing-box', label: t('user.subscribe.formats.singBox') },
  { value: 'json', label: t('user.subscribe.formats.json') },
  { value: 'base64json', label: t('user.subscribe.formats.base64json') }
]))

async function load(refresh = false) {
  try {
    const res = await getSubscription(refresh)
    subscription.value = res.data || {}
  } catch {
    // no-op
  }
}

function getSubscribeUrl(format) {
  const origin = window.location.origin
  const path = '/s'
  if (!format || format === 'auto' || format === 'ua') {
    return `${origin}${path}/${token.value}`
  }
  return `${origin}${path}/${token.value}?type=${encodeURIComponent(format)}`
}

function getFileExt(format) {
  if (format === 'auto' || format === 'ua') return 'txt'
  if (format === 'clash' || format === 'stash' || format === 'egern') return 'yaml'
  if (format === 'json' || format === 'sing-box') return 'json'
  if (format === 'surge') return 'conf'
  return 'txt'
}

async function copyText(text) {
  try {
    await navigator.clipboard.writeText(text)
    alert(t('common.messages.copySuccess'))
  } catch {
    alert(t('common.messages.copyFailed'))
  }
}

async function preview(format) {
  previewFormat.value = format
  showPreview.value = true
  try {
    const res = await fetch(getSubscribeUrl(format))
    if (!res.ok) {
      previewContent.value = t('user.subscribe.fetchPreviewFailed')
      return
    }
    const text = await res.text()
    previewContent.value = text || t('user.subscribe.noPreviewContent')
  } catch {
    previewContent.value = t('user.subscribe.fetchPreviewFailed')
  }
}

function closePreview() {
  showPreview.value = false
  previewContent.value = ''
}

function downloadPreview() {
  const ext = getFileExt(previewFormat.value)
  const blob = new Blob([previewContent.value], { type: 'text/plain' })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = `subscription.${ext}`
  anchor.click()
  URL.revokeObjectURL(url)
}

async function refresh() {
  await load(true)
  alert(t('user.subscribe.refreshCompleted'))
}

function formatBytes(bytes) {
  if (!bytes) return '0 B'
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const index = Math.floor(Math.log(bytes) / Math.log(1024))
  return `${(bytes / (1024 ** index)).toFixed(2)} ${sizes[index]}`
}

onMounted(() => {
  load(false)
})
</script>

<style scoped>
.subscriptions-page .page-header {
  margin-bottom: 16px;
}

.card {
  background: var(--surface-color);
  padding: 16px;
  border-radius: 6px;
  margin-bottom: 12px;
}

.link-item {
  margin-bottom: 8px;
}

.link-row {
  display: flex;
  gap: 8px;
  align-items: center;
}

.link-row input {
  flex: 1;
  padding: 6px;
  border-radius: 4px;
  border: 1px solid var(--border-color);
  background: transparent;
  color: var(--text-color);
}

.preview-card textarea {
  width: 100%;
  background: transparent;
  color: var(--text-color);
  border: 1px solid var(--border-color);
  padding: 8px;
  border-radius: 6px;
}

.preview-controls {
  display: flex;
  gap: 8px;
  margin-bottom: 8px;
}
</style>
