<template>
  <div class="admin-layout">
    <header class="mobile-header">
      <button class="menu-toggle" @click="sidebarOpen = !sidebarOpen">
        <span class="menu-icon">☰</span>
      </button>
      <div class="logo">V2Board Admin</div>
      <button class="btn-ghost btn-sm" @click="logout">退出</button>
    </header>

    <div class="sidebar-overlay" :class="{ active: sidebarOpen }" @click="sidebarOpen = false"></div>

    <aside class="sidebar" :class="{ open: sidebarOpen }">
      <div class="sidebar-header">
        <div class="sidebar-brand">
          <div class="logo">V2Board</div>
          <span class="badge">Admin</span>
        </div>
        <button class="close-btn" @click="sidebarOpen = false">×</button>
      </div>

      <nav class="sidebar-nav">
        <div v-for="section in navSections" :key="section.title" class="nav-section">
          <div class="nav-title">{{ section.title }}</div>
          <template v-if="section.kind === 'forward'">
            <ForwardSuiteNav />
          </template>
          <template v-else>
            <router-link v-for="item in section.items" :key="item.to" :to="item.to" @click="closeSidebar">
              <span class="nav-icon">{{ item.icon }}</span>
              <span>{{ item.label }}</span>
            </router-link>
          </template>
        </div>
      </nav>

      <div class="sidebar-footer">
        <div class="user-info">
          <div class="user-avatar">👤</div>
          <div class="user-details">
            <div class="user-name">管理员</div>
            <div class="user-email">{{ userStore.userInfo?.email || '-' }}</div>
          </div>
        </div>
        <button class="btn-ghost btn-sm w-full" @click="logout">退出登录</button>
      </div>
    </aside>

    <main class="main-content">
      <header class="content-header">
        <div class="header-title">
          <h1>{{ pageTitle }}</h1>
          <p class="header-subtitle">控制面、转发套件和运维入口统一收敛在此导航。</p>
        </div>
        <div class="header-actions">
          <span class="current-time">{{ currentTime }}</span>
        </div>
      </header>
      <div class="page-content">
        <router-view></router-view>
      </div>
    </main>
  </div>
</template>

<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useUserStore } from '@/stores/user'
import ForwardSuiteNav from '@/components/admin/ForwardSuiteNav.vue'

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()
const sidebarOpen = ref(false)
const currentTime = ref('')

const navSections = [
  {
    title: '概览',
    items: [{ to: '/admin/dashboard', icon: '🏠', label: '仪表盘' }]
  },
  {
    title: '转发套件',
    kind: 'forward'
  },
  {
    title: '用户管理',
    items: [
      { to: '/admin/users', icon: '👥', label: '用户管理' },
      { to: '/admin/orders', icon: '📦', label: '订单管理' },
      { to: '/admin/tickets', icon: '🎫', label: '工单管理' }
    ]
  },
  {
    title: '节点管理',
    items: [
      { to: '/admin/nodes', icon: '🖥️', label: '节点管理' },
      { to: '/admin/subscriptions', icon: '📚', label: '订阅管理' },
      { to: '/admin/agent', icon: '🤖', label: 'Agent 管理' }
    ]
  },
  {
    title: '营销管理',
    items: [
      { to: '/admin/plans', icon: '🔖', label: '套餐管理' },
      { to: '/admin/coupons', icon: '🎟️', label: '优惠券' },
      { to: '/admin/invite', icon: '✉️', label: '邀请返利' }
    ]
  },
  {
    title: '财务',
    items: [{ to: '/admin/payment', icon: '💰', label: '支付网关' }]
  },
  {
    title: '通知',
    items: [
      { to: '/admin/telegram', icon: '💬', label: 'Telegram Bot' },
      { to: '/admin/notifications', icon: '📢', label: '通知管理' }
    ]
  },
  {
    title: '内容管理',
    items: [{ to: '/admin/knowledge', icon: '📖', label: '知识库' }]
  },
  {
    title: '系统',
    items: [
      { to: '/admin/mfa', icon: '🔐', label: 'MFA 设置' },
      { to: '/admin/system', icon: '⚙️', label: '系统管理' }
    ]
  }
]

const pageTitles = {
  '/admin/dashboard': '仪表盘',
  '/admin/users': '用户管理',
  '/admin/nodes': '节点管理',
  '/admin/subscriptions': '订阅管理',
  '/admin/orders': '订单管理',
  '/admin/plans': '套餐管理',
  '/admin/tickets': '工单管理',
  '/admin/coupons': '优惠券管理',
  '/admin/knowledge': '知识库管理',
  '/admin/forward': '流量转发管理',
  '/admin/forward/tunnel': '隧道管理',
  '/admin/forward/limit': '限速管理',
  '/admin/forward/nodes': '中转节点与规则',
  '/admin/forward/tunnels': '隧道管理',
  '/admin/forward/limits': '限速管理',
  '/admin/tunnel': '隧道管理',
  '/admin/limit': '限速管理',
  '/admin/payment': '支付网关管理',
  '/admin/telegram': 'Telegram Bot 管理',
  '/admin/mfa': 'MFA 设置',
  '/admin/notifications': '通知管理',
  '/admin/invite': '邀请返利管理',
  '/admin/system': '系统管理',
  '/admin/agent': 'Agent 管理'
}

const pageTitle = computed(() => pageTitles[route.path] || '管理面板')

function closeSidebar() {
  sidebarOpen.value = false
}

function logout() {
  userStore.logout()
  router.push('/login')
}

function updateTime() {
  currentTime.value = new Date().toLocaleString('zh-CN', {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false
  })
}

let timer

onMounted(() => {
  updateTime()
  timer = setInterval(updateTime, 60000)
  if (userStore.isLoggedIn) {
    userStore.getUserInfo()
  }
})

onUnmounted(() => {
  clearInterval(timer)
})
</script>

<style scoped>
.admin-layout {
  display: flex;
  min-height: 100vh;
  background: var(--bg-color);
}

.mobile-header {
  display: none;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 16px;
  border-bottom: 1px solid var(--border-color);
  background: var(--surface-color);
  position: sticky;
  top: 0;
  z-index: 1200;
}

.menu-toggle,
.close-btn {
  border: 0;
  background: transparent;
  color: var(--text-color);
  cursor: pointer;
  font-size: 22px;
  line-height: 1;
}

.sidebar-overlay {
  position: fixed;
  inset: 0;
  background: rgba(15, 23, 42, 0.45);
  opacity: 0;
  pointer-events: none;
  transition: opacity 0.2s ease;
  z-index: 1090;
}

.sidebar-overlay.active {
  opacity: 1;
  pointer-events: auto;
}

.sidebar {
  width: 288px;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  border-right: 1px solid var(--border-color);
  background:
    radial-gradient(circle at top right, rgba(37, 99, 235, 0.12), transparent 28%),
    var(--surface-color);
  position: sticky;
  top: 0;
  height: 100vh;
}

.sidebar-header,
.sidebar-footer {
  padding: 18px 20px;
  border-bottom: 1px solid var(--border-color);
}

.sidebar-footer {
  border-top: 1px solid var(--border-color);
  border-bottom: 0;
  margin-top: auto;
}

.sidebar-brand {
  display: flex;
  align-items: center;
  gap: 10px;
}

.logo {
  font-size: 20px;
  font-weight: 700;
}

.badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 4px 10px;
  border-radius: 999px;
  background: rgba(37, 99, 235, 0.12);
  color: #2563eb;
  font-size: 12px;
  font-weight: 700;
}

.sidebar-nav {
  padding: 16px 12px 20px;
  overflow-y: auto;
}

.nav-section + .nav-section {
  margin-top: 16px;
}

.nav-title {
  margin: 0 0 8px;
  padding: 0 12px;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.08em;
  color: var(--text-secondary);
  text-transform: uppercase;
}

.sidebar-nav :deep(a) {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
  border-radius: 12px;
  color: var(--text-secondary);
  text-decoration: none;
}

.sidebar-nav :deep(a:hover) {
  background: var(--bg-color);
  color: var(--text-color);
}

.sidebar-nav :deep(a.router-link-active) {
  background: var(--primary-color);
  color: #fff;
}

.nav-icon {
  width: 20px;
  text-align: center;
  font-size: 16px;
}

.user-info {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}

.user-avatar {
  width: 40px;
  height: 40px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(37, 99, 235, 0.12);
}

.user-name {
  font-weight: 700;
}

.user-email {
  font-size: 13px;
  color: var(--text-secondary);
  word-break: break-all;
}

.main-content {
  min-width: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
}

.content-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 20px 24px;
  border-bottom: 1px solid var(--border-color);
  background: rgba(255, 255, 255, 0.84);
  backdrop-filter: blur(10px);
  position: sticky;
  top: 0;
  z-index: 900;
}

.header-title h1 {
  margin: 0;
  font-size: 28px;
}

.header-subtitle {
  margin: 6px 0 0;
  color: var(--text-secondary);
  font-size: 14px;
}

.current-time {
  display: inline-flex;
  align-items: center;
  padding: 8px 12px;
  border-radius: 999px;
  background: rgba(15, 23, 42, 0.06);
  color: var(--text-secondary);
  font-size: 13px;
}

.page-content {
  padding: 24px;
}

@media (max-width: 960px) {
  .admin-layout {
    display: block;
  }

  .mobile-header {
    display: flex;
  }

  .sidebar {
    position: fixed;
    left: 0;
    top: 0;
    transform: translateX(-100%);
    transition: transform 0.22s ease;
    z-index: 1100;
  }

  .sidebar.open {
    transform: translateX(0);
  }

  .content-header {
    top: 57px;
    padding: 18px 16px;
  }

  .page-content {
    padding: 16px;
  }
}
</style>
