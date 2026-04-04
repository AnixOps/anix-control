<template>
  <div class="page subscriptions-page">
    <div class="page-header">
      <h1>订阅管理</h1>
      <p class="subtitle">管理与复制你的订阅链接，预览与下载订阅内容。</p>
    </div>

    <div v-if="!token" class="card">
      <p>未检测到用户订阅 Token，请先登录或刷新页面。</p>
    </div>

    <div v-else>
      <div class="card">
        <h3>订阅信息</h3>
        <p>过期时间: <strong>{{ subscription.ExpireAt ? new Date(subscription.ExpireAt * 1000).toLocaleString() : '永久' }}</strong></p>
        <p>已用流量: <strong>{{ formatBytes(subscription.UsedTraffic || 0) }}</strong></p>
        <p>总流量: <strong>{{ formatBytes(subscription.TotalTraffic || 0) }}</strong></p>
      </div>

      <div class="card">
        <h3>订阅链接</h3>
        <div v-for="fmt in formats" :key="fmt.value" class="link-item">
          <label>{{ fmt.label }}:</label>
          <div class="link-row">
            <input type="text" :value="getSubscribeUrl(fmt.value)" readonly />
            <button class="btn btn-sm" @click="copyText(getSubscribeUrl(fmt.value))">复制链接</button>
            <button class="btn btn-sm" @click="preview(fmt.value)">预览</button>
          </div>
        </div>
        <div class="link-actions">
          <button class="btn btn-primary" @click="refresh">刷新订阅缓存</button>
        </div>
      </div>

      <div v-if="showPreview" class="card preview-card">
        <h3>订阅预览 ({{ previewFormat }})</h3>
        <div class="preview-controls">
          <button class="btn btn-sm" @click="copyText(previewContent)">复制内容</button>
          <button class="btn btn-sm" @click="downloadPreview">下载</button>
          <button class="btn btn-ghost btn-sm" @click="closePreview">关闭</button>
        </div>
        <textarea readonly rows="12">{{ previewContent }}</textarea>
      </div>
    </div>
  </div>
</template>

<script>
import { ref, computed, onMounted } from 'vue'
import { useUserStore } from '@/stores/user'
import { getSubscription } from '@/api/user'

export default {
  name: 'UserSubscribe',
  setup() {
    const userStore = useUserStore()
    const token = computed(() => (userStore.userInfo && userStore.userInfo.token) || '')
    const subscription = ref({})
    const formats = [
      { value: 'auto', label: 'Auto (By User-Agent)' },
      { value: 'v2ray', label: 'V2Ray (Base64)' },
      { value: 'clash', label: 'Clash (YAML)' },
      { value: 'stash', label: 'Stash (YAML)' },
      { value: 'egern', label: 'Egern (YAML)' },
      { value: 'surge', label: 'Surge' },
      { value: 'loon', label: 'Loon' },
      { value: 'shadowrocket', label: 'ShadowRocket' },
      { value: 'quantumultx', label: 'QuantumultX' },
      { value: 'sing-box', label: 'Sing-box (JSON)' },
      { value: 'json', label: 'Raw JSON' },
      { value: 'base64json', label: 'Base64 JSON' }
    ]

    const showPreview = ref(false)
    const previewContent = ref('')
    const previewFormat = ref('')

    const load = async (refresh = false) => {
      try {
        const res = await getSubscription(refresh)
        subscription.value = res.data || {}
      } catch (e) {
        // ignore
      }
    }

    onMounted(() => {
      load(false)
    })

    function getSubscribeUrl(format) {
      const origin = window.location.origin
      const path = '/s' // default subscribe path
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
        alert('已复制到剪贴板')
      } catch (e) {
        alert('复制失败')
      }
    }

    async function preview(format) {
      previewFormat.value = format
      showPreview.value = true
      try {
        const url = getSubscribeUrl(format)
        const res = await fetch(url)
        if (!res.ok) {
          previewContent.value = '获取预览失败'
          return
        }
        const text = await res.text()
        previewContent.value = text || '无订阅内容'
      } catch (e) {
        previewContent.value = '获取预览失败'
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
      const a = document.createElement('a')
      a.href = url
      a.download = `subscription.${ext}`
      a.click()
      URL.revokeObjectURL(url)
    }

    async function refresh() {
      await load(true)
      alert('订阅缓存已刷新')
    }

    function formatBytes(bytes) {
      if (!bytes) return '0 B'
      const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
      const i = Math.floor(Math.log(bytes) / Math.log(1024))
      return (bytes / Math.pow(1024, i)).toFixed(2) + ' ' + sizes[i]
    }

    return {
      token,
      subscription,
      formats,
      getSubscribeUrl,
      copyText,
      preview,
      showPreview,
      previewContent,
      previewFormat,
      closePreview,
      downloadPreview,
      refresh,
      formatBytes
    }
  }
}
</script>

<style scoped>
.subscriptions-page .page-header { margin-bottom: 16px }
.card { background: var(--surface-color); padding: 16px; border-radius: 6px; margin-bottom: 12px }
.link-item { margin-bottom: 8px }
.link-row { display:flex; gap:8px; align-items:center }
.link-row input { flex:1; padding:6px; border-radius:4px; border:1px solid var(--border-color); background:transparent; color:var(--text-color) }
.preview-card textarea { width:100%; background:transparent; color:var(--text-color); border:1px solid var(--border-color); padding:8px; border-radius:6px }
.preview-controls { display:flex; gap:8px; margin-bottom:8px }
</style>
