<template>
  <div class="notifications-page">
    <div class="page-header">
      <h1>Notifications</h1>
      <div class="header-actions">
        <span v-if="unreadCount > 0" class="badge">{{ unreadCount }} unread</span>
        <button v-if="unreadCount > 0" class="btn btn-secondary" @click="markAllRead">
          Mark All Read
        </button>
        <button class="btn btn-secondary" @click="fetchNotifications">
          <span class="icon">🔄</span> Refresh
        </button>
      </div>
    </div>

    <!-- Notification List -->
    <div class="notifications-list">
      <div v-if="loading" class="loading-state">
        <div class="spinner"></div>
        <span>Loading notifications...</span>
      </div>

      <div v-else-if="notifications.length === 0" class="empty-state">
        <div class="empty-icon">📬</div>
        <p>No notifications</p>
      </div>

      <div v-else>
        <div
          v-for="notification in notifications"
          :key="notification.id"
          :class="['notification-item', { unread: !notification.read }]"
          @click="markAsRead(notification)"
        >
          <div :class="['notification-icon', notification.type]">
            {{ getTypeIcon(notification.type) }}
          </div>
          <div class="notification-content">
            <div class="notification-header">
              <span class="notification-title">{{ notification.title }}</span>
              <span class="notification-time">{{ formatTime(notification.created_at) }}</span>
            </div>
            <p class="notification-message">{{ notification.message }}</p>
          </div>
          <div class="notification-actions">
            <button
              v-if="!notification.read"
              class="btn-icon"
              @click.stop="markAsRead(notification)"
              title="Mark as read"
            >
              ✓
            </button>
            <button
              class="btn-icon danger"
              @click.stop="deleteNotification(notification)"
              title="Delete"
            >
              ✕
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useApi } from '@/composables/useApi'

const { get, put, del } = useApi()

const notifications = ref([])
const loading = ref(false)

const unreadCount = computed(() =>
  notifications.value.filter(n => !n.read).length
)

async function fetchNotifications() {
  loading.value = true
  try {
    const res = await get('/notifications')
    notifications.value = res.data?.items || []
  } catch (e) {
    console.error('Failed to fetch notifications:', e)
    // Mock data for demo
    notifications.value = [
      {
        id: '1',
        title: 'Node Offline',
        message: 'Node "US-East-1" has gone offline',
        type: 'error',
        read: false,
        created_at: new Date(Date.now() - 5 * 60000).toISOString()
      },
      {
        id: '2',
        title: 'High CPU Usage',
        message: 'Node "EU-West-2" CPU usage exceeded 90%',
        type: 'warning',
        read: false,
        created_at: new Date(Date.now() - 60 * 60000).toISOString()
      },
      {
        id: '3',
        title: 'Backup Completed',
        message: 'Daily backup completed successfully',
        type: 'success',
        read: true,
        created_at: new Date(Date.now() - 6 * 3600000).toISOString()
      }
    ]
  } finally {
    loading.value = false
  }
}

async function markAsRead(notification) {
  if (notification.read) return
  try {
    await put(`/notifications/${notification.id}/read`)
    notification.read = true
  } catch (e) {
    notification.read = true
  }
}

async function markAllRead() {
  try {
    await put('/notifications/read-all')
    notifications.value.forEach(n => n.read = true)
  } catch (e) {
    notifications.value.forEach(n => n.read = true)
  }
}

async function deleteNotification(notification) {
  try {
    await del(`/notifications/${notification.id}`)
    notifications.value = notifications.value.filter(n => n.id !== notification.id)
  } catch (e) {
    notifications.value = notifications.value.filter(n => n.id !== notification.id)
  }
}

function getTypeIcon(type) {
  switch (type) {
    case 'error': return '❌'
    case 'warning': return '⚠️'
    case 'success': return '✅'
    default: return 'ℹ️'
  }
}

function formatTime(dateStr) {
  const date = new Date(dateStr)
  const now = new Date()
  const diff = Math.floor((now - date) / 1000)

  if (diff < 60) return 'Just now'
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`
  return `${Math.floor(diff / 86400)}d ago`
}

onMounted(() => {
  fetchNotifications()
})
</script>

<style scoped>
.notifications-page {
  padding: 24px;
  max-width: 800px;
  margin: 0 auto;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
}

.page-header h1 {
  margin: 0;
  font-size: 24px;
  display: flex;
  align-items: center;
  gap: 12px;
}

.badge {
  background: var(--primary);
  color: white;
  padding: 4px 12px;
  border-radius: 12px;
  font-size: 12px;
  font-weight: 500;
}

.header-actions {
  display: flex;
  gap: 12px;
  align-items: center;
}

.notifications-list {
  background: var(--card-bg);
  border-radius: 12px;
  overflow: hidden;
}

.loading-state,
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  color: var(--text-secondary);
}

.empty-icon {
  font-size: 48px;
  margin-bottom: 16px;
}

.spinner {
  width: 32px;
  height: 32px;
  border: 3px solid var(--border-color);
  border-top-color: var(--primary);
  border-radius: 50%;
  animation: spin 1s linear infinite;
  margin-bottom: 12px;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.notification-item {
  display: flex;
  align-items: flex-start;
  gap: 16px;
  padding: 16px 20px;
  border-bottom: 1px solid var(--border-color);
  cursor: pointer;
  transition: background 0.2s;
}

.notification-item:last-child {
  border-bottom: none;
}

.notification-item:hover {
  background: var(--hover-bg);
}

.notification-item.unread {
  background: rgba(var(--primary-rgb), 0.05);
}

.notification-item.unread:hover {
  background: rgba(var(--primary-rgb), 0.1);
}

.notification-icon {
  width: 40px;
  height: 40px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 18px;
  flex-shrink: 0;
}

.notification-icon.error {
  background: rgba(244, 67, 54, 0.15);
}

.notification-icon.warning {
  background: rgba(255, 193, 7, 0.15);
}

.notification-icon.success {
  background: rgba(76, 175, 80, 0.15);
}

.notification-icon.info {
  background: rgba(33, 150, 243, 0.15);
}

.notification-content {
  flex: 1;
  min-width: 0;
}

.notification-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
  margin-bottom: 4px;
}

.notification-title {
  font-weight: 600;
  font-size: 14px;
}

.notification-time {
  font-size: 12px;
  color: var(--text-secondary);
  white-space: nowrap;
}

.notification-message {
  margin: 0;
  font-size: 13px;
  color: var(--text-secondary);
  line-height: 1.5;
}

.notification-actions {
  display: flex;
  gap: 4px;
  opacity: 0;
  transition: opacity 0.2s;
}

.notification-item:hover .notification-actions {
  opacity: 1;
}

.btn {
  padding: 8px 16px;
  border-radius: 6px;
  font-weight: 500;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 14px;
}

.btn-primary {
  background: var(--primary);
  color: white;
  border: none;
}

.btn-secondary {
  background: transparent;
  color: var(--text);
  border: 1px solid var(--border-color);
}

.btn-secondary:hover {
  background: var(--hover-bg);
}

.btn-icon {
  background: none;
  border: none;
  cursor: pointer;
  padding: 6px;
  font-size: 14px;
  border-radius: 4px;
  color: var(--text-secondary);
}

.btn-icon:hover {
  background: var(--hover-bg);
  color: var(--text);
}

.btn-icon.danger:hover {
  background: rgba(244, 67, 54, 0.15);
  color: #f44336;
}
</style>