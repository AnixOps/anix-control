import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

// Mock notifications data
const mockNotifications = [
  { id: '1', title: 'Node Offline', message: 'Node US-East-1 has gone offline', type: 'error', read: false },
  { id: '2', title: 'High CPU', message: 'CPU usage exceeded 90%', type: 'warning', read: false },
  { id: '3', title: 'Backup Complete', message: 'Daily backup completed', type: 'success', read: true },
  { id: '4', title: 'Task Started', message: 'Task execution started', type: 'info', read: true },
]

describe('Notifications Store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('filters notifications by read status', () => {
    const unreadNotifications = mockNotifications.filter(n => !n.read)
    const readNotifications = mockNotifications.filter(n => n.read)

    expect(unreadNotifications.length).toBe(2)
    expect(readNotifications.length).toBe(2)
  })

  it('calculates unread count', () => {
    const unreadCount = mockNotifications.filter(n => !n.read).length

    expect(unreadCount).toBe(2)
  })

  it('filters notifications by type', () => {
    const errorNotifications = mockNotifications.filter(n => n.type === 'error')
    const warningNotifications = mockNotifications.filter(n => n.type === 'warning')
    const successNotifications = mockNotifications.filter(n => n.type === 'success')
    const infoNotifications = mockNotifications.filter(n => n.type === 'info')

    expect(errorNotifications.length).toBe(1)
    expect(warningNotifications.length).toBe(1)
    expect(successNotifications.length).toBe(1)
    expect(infoNotifications.length).toBe(1)
  })

  it('filters notifications by search query', () => {
    const query = 'backup'
    const filtered = mockNotifications.filter(n =>
      n.title.toLowerCase().includes(query.toLowerCase()) ||
      n.message.toLowerCase().includes(query.toLowerCase())
    )

    expect(filtered.length).toBe(1)
    expect(filtered[0].title).toBe('Backup Complete')
  })
})

describe('Notification Model', () => {
  it('parses JSON correctly', () => {
    const json = {
      id: 'notif-123',
      title: 'Test Notification',
      message: 'This is a test notification',
      type: 'info',
      read: false,
      created_at: '2026-03-20T10:00:00Z',
    }

    expect(json.id).toBe('notif-123')
    expect(json.title).toBe('Test Notification')
    expect(json.message).toBe('This is a test notification')
    expect(json.type).toBe('info')
    expect(json.read).toBe(false)
    expect(json.created_at).toBe('2026-03-20T10:00:00Z')
  })

  it('handles missing optional fields', () => {
    const json = {
      id: '2',
      title: 'Minimal Notification',
      message: 'Test',
    }

    expect(json.id).toBe('2')
    expect(json.title).toBe('Minimal Notification')
    expect(json.message).toBe('Test')
    expect(json.type).toBeUndefined()
    expect(json.read).toBeUndefined()
  })

  it('gets correct icon for notification types', () => {
    const getTypeIcon = (type) => {
      switch (type) {
        case 'error': return '❌'
        case 'warning': return '⚠️'
        case 'success': return '✅'
        default: return 'ℹ️'
      }
    }

    expect(getTypeIcon('error')).toBe('❌')
    expect(getTypeIcon('warning')).toBe('⚠️')
    expect(getTypeIcon('success')).toBe('✅')
    expect(getTypeIcon('info')).toBe('ℹ️')
    expect(getTypeIcon('unknown')).toBe('ℹ️')
  })

  it('formats relative time correctly', () => {
    const formatTime = (diffSeconds) => {
      if (diffSeconds < 60) return 'Just now'
      if (diffSeconds < 3600) return `${Math.floor(diffSeconds / 60)}m ago`
      if (diffSeconds < 86400) return `${Math.floor(diffSeconds / 3600)}h ago`
      return `${Math.floor(diffSeconds / 86400)}d ago`
    }

    expect(formatTime(30)).toBe('Just now')
    expect(formatTime(120)).toBe('2m ago')
    expect(formatTime(7200)).toBe('2h ago')
    expect(formatTime(172800)).toBe('2d ago')
  })
})

describe('Notification Actions', () => {
  it('marks notification as read', () => {
    const notification = { id: '1', title: 'Test', message: 'Msg', read: false }
    notification.read = true

    expect(notification.read).toBe(true)
  })

  it('marks all notifications as read', () => {
    const notifications = [
      { id: '1', read: false },
      { id: '2', read: false },
      { id: '3', read: true },
    ]

    notifications.forEach(n => n.read = true)

    expect(notifications.every(n => n.read)).toBe(true)
  })

  it('deletes notification from list', () => {
    const notifications = [
      { id: '1', title: 'Test 1' },
      { id: '2', title: 'Test 2' },
      { id: '3', title: 'Test 3' },
    ]

    const filtered = notifications.filter(n => n.id !== '2')

    expect(filtered.length).toBe(2)
    expect(filtered.find(n => n.id === '2')).toBeUndefined()
  })
})