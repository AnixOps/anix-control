<template>
  <div class="admin-layout">
    <!-- 移动端头部 -->
    <header class="mobile-header">
      <button class="menu-toggle" @click="sidebarOpen = !sidebarOpen">
        <span class="menu-icon">☰</span>
      </button>
      <div class="logo">V2Board Admin</div>
      <button class="btn-ghost btn-sm" @click="logout">退出</button>
    </header>
    
    <!-- 侧边栏遮罩 -->
    <div 
      class="sidebar-overlay" 
      :class="{ active: sidebarOpen }" 
      @click="sidebarOpen = false"
    ></div>
    
    <!-- 侧边栏 -->
    <aside class="sidebar" :class="{ open: sidebarOpen }">
      <div class="sidebar-header">
        <div class="logo">V2Board</div>
        <span class="badge">Admin</span>
        <button class="close-btn" @click="sidebarOpen = false">✕</button>
      </div>
      
      <nav class="sidebar-nav">
        <div class="nav-section">
          <div class="nav-title">概览</div>
          <router-link to="/admin/dashboard" @click="closeSidebar">
            <span class="nav-icon">📊</span> 仪表盘
          </router-link>
        </div>
        
        <div class="nav-section">
          <div class="nav-title">用户管理</div>
          <router-link to="/admin/users" @click="closeSidebar">
            <span class="nav-icon">👥</span> 用户列表
          </router-link>
          <router-link to="/admin/orders" @click="closeSidebar">
            <span class="nav-icon">📋</span> 订单管理
          </router-link>
          <router-link to="/admin/tickets" @click="closeSidebar">
            <span class="nav-icon">🎫</span> 工单管理
          </router-link>
        </div>
        
        <div class="nav-section">
          <div class="nav-title">节点管理</div>
          <router-link to="/admin/nodes" @click="closeSidebar">
            <span class="nav-icon">🖥️</span> 节点列表
          </router-link>
          <router-link to="/admin/subscriptions" @click="closeSidebar">
            <span class="nav-icon">📡</span> 订阅管理
          </router-link>
        </div>

        <div class="nav-section">
          <div class="nav-title">营销管理</div>
          <router-link to="/admin/plans" @click="closeSidebar">
            <span class="nav-icon">💰</span> 套餐管理
          </router-link>
          <router-link to="/admin/coupons" @click="closeSidebar">
            <span class="nav-icon">🎟️</span> 优惠券
          </router-link>
        </div>
        
        <div class="nav-section">
          <div class="nav-title">内容管理</div>
          <router-link to="/admin/knowledge" @click="closeSidebar">
            <span class="nav-icon">📚</span> 知识库
          </router-link>
        </div>
        
        <div class="nav-section">
          <div class="nav-title">系统</div>
          <router-link to="/admin/settings" @click="closeSidebar">
            <span class="nav-icon">⚙️</span> 系统设置
          </router-link>
        </div>
      </nav>
      
      <div class="sidebar-footer">
        <div class="user-info">
          <div class="user-avatar">👤</div>
          <div class="user-details">
            <div class="user-name">管理员</div>
            <div class="user-email">{{ userStore.userInfo?.email }}</div>
          </div>
        </div>
        <button class="btn-ghost btn-sm w-full" @click="logout">退出登录</button>
      </div>
    </aside>
    
    <!-- 主内容区 -->
    <main class="main-content">
      <header class="content-header">
        <div class="header-title">
          <h1>{{ pageTitle }}</h1>
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
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useUserStore } from '@/stores/user'
import { useRouter, useRoute } from 'vue-router'

const userStore = useUserStore()
const router = useRouter()
const route = useRoute()
const sidebarOpen = ref(false)
const currentTime = ref('')

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
  '/admin/settings': '系统设置'
}

const pageTitle = computed(() => pageTitles[route.path] || '管理面板')

const closeSidebar = () => {
  sidebarOpen.value = false
}

const logout = () => {
  userStore.logout()
  router.push('/login')
}

const updateTime = () => {
  const now = new Date()
  currentTime.value = now.toLocaleString('zh-CN', {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit'
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

/* 移动端头部 */
.mobile-header {
  display: flex;
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  height: var(--header-height);
  background: var(--surface-color);
  border-bottom: 1px solid var(--border-color);
  align-items: center;
  padding: 0 16px;
  z-index: 100;
  gap: 12px;
}

.menu-toggle {
  display: flex;
  padding: 8px;
  background: transparent;
  border: none;
  font-size: 20px;
}

.mobile-header .logo {
  flex: 1;
  font-weight: 700;
  font-size: 16px;
}

/* 侧边栏遮罩 */
.sidebar-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.6);
  z-index: 200;
  opacity: 0;
  visibility: hidden;
  transition: var(--transition);
}

.sidebar-overlay.active {
  opacity: 1;
  visibility: visible;
}

/* 侧边栏 */
.sidebar {
  position: fixed;
  top: 0;
  left: 0;
  width: var(--sidebar-width);
  max-width: 85vw;
  height: 100vh;
  background: var(--surface-color);
  border-right: 1px solid var(--border-color);
  display: flex;
  flex-direction: column;
  z-index: 300;
  transform: translateX(-100%);
  transition: transform 0.3s ease;
}

.sidebar.open {
  transform: translateX(0);
}

.sidebar-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 20px;
  border-bottom: 1px solid var(--border-color);
}

.sidebar-header .logo {
  font-weight: 700;
  font-size: 18px;
  background: linear-gradient(135deg, #3b82f6, #8b5cf6);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.badge {
  font-size: 11px;
  padding: 2px 8px;
  background: var(--primary-color);
  border-radius: 10px;
  font-weight: 600;
}

.close-btn {
  margin-left: auto;
  padding: 8px;
  background: transparent;
  border: none;
  font-size: 18px;
  color: var(--text-secondary);
}

.sidebar-nav {
  flex: 1;
  overflow-y: auto;
  padding: 16px 12px;
}

.nav-section {
  margin-bottom: 24px;
}

.nav-title {
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 1px;
  color: var(--text-secondary);
  padding: 0 12px;
  margin-bottom: 8px;
}

.sidebar-nav a {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 16px;
  color: var(--text-secondary);
  text-decoration: none;
  border-radius: var(--radius-md);
  font-size: 14px;
  transition: var(--transition);
  margin-bottom: 2px;
}

.sidebar-nav a:hover {
  background: var(--bg-color);
  color: var(--text-color);
}

.sidebar-nav a.router-link-active {
  background: var(--primary-color);
  color: white;
}

.nav-icon {
  font-size: 16px;
  width: 20px;
  text-align: center;
}

.sidebar-footer {
  padding: 16px;
  border-top: 1px solid var(--border-color);
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
  background: var(--bg-color);
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 18px;
}

.user-details {
  flex: 1;
  min-width: 0;
}

.user-name {
  font-size: 14px;
  font-weight: 600;
}

.user-email {
  font-size: 12px;
  color: var(--text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* 主内容区 */
.main-content {
  flex: 1;
  display: flex;
  flex-direction: column;
  margin-top: var(--header-height);
  min-width: 0;
}

.content-header {
  display: none;
  align-items: center;
  justify-content: space-between;
  padding: 24px 32px;
  border-bottom: 1px solid var(--border-color);
  background: var(--surface-color);
}

.header-title h1 {
  font-size: 24px;
  font-weight: 600;
}

.current-time {
  font-size: 14px;
  color: var(--text-secondary);
}

.page-content {
  flex: 1;
  padding: 20px 16px;
  overflow-y: auto;
}

/* 平板和桌面端 */
@media (min-width: 768px) {
  .mobile-header {
    display: none;
  }
  
  .sidebar-overlay {
    display: none;
  }
  
  .sidebar {
    position: sticky;
    top: 0;
    transform: translateX(0);
    flex-shrink: 0;
  }
  
  .close-btn {
    display: none;
  }
  
  .main-content {
    margin-top: 0;
  }
  
  .content-header {
    display: flex;
  }
  
  .page-content {
    padding: 24px 32px;
  }
}

@media (min-width: 1024px) {
  .page-content {
    padding: 32px 40px;
  }
}
</style>
