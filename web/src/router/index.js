import { createRouter, createWebHistory } from 'vue-router'
import { useUserStore } from '@/stores/user'

// Layouts
import UserLayout from '@/layouts/UserLayout.vue'
import AdminLayout from '@/layouts/AdminLayout.vue'

// Views
import Login from '@/views/Login.vue'
import UserDashboard from '@/views/user/Dashboard.vue'
import UserSubscribe from '@/views/user/Subscribe.vue'
import AdminDashboard from '@/views/admin/Dashboard.vue'
import AdminUsers from '@/views/admin/Users.vue'
import AdminOrders from '@/views/admin/Orders.vue'
import AdminNodes from '@/views/admin/Nodes.vue'
import AdminSubscriptions from '@/views/admin/Subscriptions.vue'
import AdminPlans from '@/views/admin/Plans.vue'

const routes = [
  {
    path: '/',
    redirect: '/login'
  },
  {
    path: '/login',
    component: Login,
    meta: { guest: true }
  },
  // User Routes
  {
    path: '/user',
    component: UserLayout,
    meta: { requiresAuth: true },
    children: [
      {
        path: 'dashboard',
        component: UserDashboard
      },
      {
        path: 'subscribe',
        component: UserSubscribe
      },
      {
        path: 'knowledge',
        component: { template: '<div class="page"><h1>使用教程</h1><p>教程内容开发中...</p></div>' }
      }
    ]
  },
  // Admin Routes
  {
    path: '/admin',
    component: AdminLayout,
    meta: { requiresAuth: true, requiresAdmin: true },
    children: [
      {
        path: 'dashboard',
        component: AdminDashboard
      },
      {
        path: 'users',
        component: AdminUsers
      },
      {
        path: 'nodes',
        component: AdminNodes
      },
      {
        path: 'orders',
        component: AdminOrders
      },
      {
        path: 'subscriptions',
        component: AdminSubscriptions
      },
      {
        path: 'plans',
        component: AdminPlans
      },
      {
        path: 'settings',
        component: { template: '<div class="page"><h1>系统设置</h1><p>系统设置功能开发中...</p></div>' }
      }
    ]
  }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

// Navigation Guards
router.beforeEach((to, from, next) => {
  const userStore = useUserStore()
  
  if (to.meta.requiresAuth && !userStore.isLoggedIn) {
    next('/login')
  } else if (to.meta.requiresAdmin && !userStore.isAdmin) {
    next('/user/dashboard') // Redirect non-admins to user dashboard
  } else if (to.meta.guest && userStore.isLoggedIn) {
    if (userStore.isAdmin) {
      next('/admin/dashboard')
    } else {
      next('/user/dashboard')
    }
  } else {
    next()
  }
})

export default router
