<template>
  <section class="forward-card">
    <header class="card-head">
      <div class="title-wrap">
        <h3>{{ forward.name }}</h3>
        <p>{{ forward.tunnelName || '-' }}</p>
      </div>
      <div class="head-actions">
        <span v-if="draggable" class="drag-handle" title="拖拽排序">⋮⋮</span>
        <label class="switch">
          <input type="checkbox" :checked="isRunning" @change="$emit('toggle', forward)" />
          <span class="slider"></span>
        </label>
        <span :class="['status-pill', statusMeta.className]">{{ statusMeta.text }}</span>
      </div>
    </header>

    <button class="endpoint" type="button" @click="$emit('address', { value: forward.inIp, port: forward.inPort, title: '入口地址' })">
      <span class="label">入口</span>
      <code>{{ inAddressText }}</code>
    </button>

    <button class="endpoint" type="button" @click="$emit('address', { value: forward.remoteAddr, title: '目标地址' })">
      <span class="label">目标</span>
      <code>{{ remoteAddressText }}</code>
    </button>

    <div class="meta-row">
      <span :class="['status-pill', strategyMeta.className]">{{ strategyMeta.text }}</span>
      <span class="status-pill">↑{{ formatFlow(forward.inFlow || 0) }}</span>
      <span class="status-pill success">↓{{ formatFlow(forward.outFlow || 0) }}</span>
    </div>

    <footer class="actions">
      <button class="btn-secondary btn-sm" @click="$emit('edit', forward)">编辑</button>
      <button class="btn-secondary btn-sm" @click="$emit('diagnose', forward)">诊断</button>
      <button class="btn-secondary btn-sm danger-text" @click="$emit('delete', forward)">删除</button>
    </footer>
  </section>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  forward: {
    type: Object,
    required: true
  },
  draggable: {
    type: Boolean,
    default: false
  }
})

defineEmits(['toggle', 'edit', 'delete', 'diagnose', 'address'])

const isRunning = computed(() => Number(props.forward.status) === 1)

const statusMeta = computed(() => {
  switch (Number(props.forward.status)) {
    case 1:
      return { text: '正常', className: 'success' }
    case 0:
      return { text: '暂停', className: 'warning' }
    case -1:
      return { text: '异常', className: 'danger' }
    default:
      return { text: '未知', className: '' }
  }
})

const strategyMeta = computed(() => {
  switch (props.forward.strategy) {
    case 'round':
      return { text: '轮询', className: 'success' }
    case 'rand':
      return { text: '随机', className: 'warning' }
    case 'hash':
      return { text: '哈希', className: 'primary' }
    default:
      return { text: '主备', className: 'primary' }
  }
})

const inAddressText = computed(() => formatInAddress(props.forward.inIp, props.forward.inPort))
const remoteAddressText = computed(() => formatRemoteAddress(props.forward.remoteAddr))

function splitComma(value) {
  return String(value || '')
    .split(',')
    .map(item => item.trim())
    .filter(Boolean)
}

function formatInAddress(value, port) {
  const entries = splitComma(value).map(item => {
    if (item.includes(':') && !item.startsWith('[') && item.split(':').length > 2) {
      return `[${item}]:${port}`
    }
    return `${item}:${port}`
  })

  if (!entries.length) return '-'
  if (entries.length === 1) return entries[0]
  return `${entries[0]} (+${entries.length - 1})`
}

function formatRemoteAddress(value) {
  const entries = splitComma(value)
  if (!entries.length) return '-'
  if (entries.length === 1) return entries[0]
  return `${entries[0]} (+${entries.length - 1})`
}

function formatFlow(value) {
  const num = Number(value || 0)
  if (num < 1024) return `${num} B`
  if (num < 1024 * 1024) return `${(num / 1024).toFixed(2)} KB`
  if (num < 1024 * 1024 * 1024) return `${(num / 1024 / 1024).toFixed(2)} MB`
  return `${(num / 1024 / 1024 / 1024).toFixed(2)} GB`
}
</script>

<style scoped>
.forward-card {
  background: rgba(20, 20, 20, 0.96);
  border: 1px solid rgba(255, 255, 255, 0.05);
  border-radius: 14px;
  padding: 14px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-height: 100%;
}

.card-head {
  display: flex;
  justify-content: space-between;
  gap: 10px;
}

.title-wrap {
  min-width: 0;
}

.title-wrap h3 {
  font-size: 15px;
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.title-wrap p {
  font-size: 12px;
  color: var(--text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.head-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.drag-handle {
  color: var(--text-secondary);
  cursor: grab;
  user-select: none;
}

.switch {
  position: relative;
  width: 38px;
  height: 22px;
  display: inline-flex;
}

.switch input {
  opacity: 0;
  width: 0;
  height: 0;
}

.slider {
  position: absolute;
  inset: 0;
  background: rgba(255, 255, 255, 0.12);
  border-radius: 999px;
  transition: all 0.2s ease;
}

.slider::before {
  content: '';
  position: absolute;
  width: 16px;
  height: 16px;
  top: 3px;
  left: 3px;
  border-radius: 50%;
  background: #fff;
  transition: all 0.2s ease;
}

.switch input:checked + .slider {
  background: rgba(34, 197, 94, 0.85);
}

.switch input:checked + .slider::before {
  transform: translateX(16px);
}

.endpoint {
  width: 100%;
  background: rgba(255, 255, 255, 0.02);
  border: 1px solid rgba(255, 255, 255, 0.06);
  border-radius: 10px;
  padding: 8px 10px;
  text-align: left;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.endpoint:hover {
  transform: none;
  background: rgba(255, 255, 255, 0.04);
}

.endpoint .label {
  font-size: 11px;
  color: var(--text-secondary);
}

.endpoint code {
  font-size: 12px;
  color: var(--text-color);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.meta-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.actions {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
  margin-top: auto;
}

.status-pill {
  min-height: 24px;
  padding: 0 8px;
  border-radius: 999px;
  font-size: 11px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: rgba(255, 255, 255, 0.06);
  color: var(--text-secondary);
}

.status-pill.primary {
  background: rgba(59, 130, 246, 0.2);
  color: #93c5fd;
}

.status-pill.success {
  background: rgba(34, 197, 94, 0.2);
  color: #86efac;
}

.status-pill.warning {
  background: rgba(245, 158, 11, 0.2);
  color: #fcd34d;
}

.status-pill.danger {
  background: rgba(239, 68, 68, 0.2);
  color: #fca5a5;
}

.danger-text {
  color: #fca5a5;
}

@media (max-width: 768px) {
  .actions {
    grid-template-columns: 1fr;
  }
}
</style>
