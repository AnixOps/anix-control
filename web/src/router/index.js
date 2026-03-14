import { createRouter, createWebHistory } from 'vue-router'
import { useUserStore } from '@/stores/user'

// Layouts
import UserLayout from '@/layouts/UserLayout.vue'
import AdminLayout from '@/layouts/AdminLayout.vue'

// Views
import Login from '@/views/Login.vue'
import UserDashboard from '@/views/user/Dashboard.vue'
import UserSubscribe from '@/views/user/Subscribe.vue'
import UserKnowledge from '@/views/user/Knowledge.vue'
import UserTickets from '@/views/user/Tickets.vue'
import UserPlans from '@/views/user/Plans.vue'
import UserOrders from '@/views/user/Orders.vue'
import AdminDashboard from '@/views/admin/Dashboard.vue'
import AdminUsers from '@/views/admin/Users.vue'
import AdminOrders from '@/views/admin/Orders.vue'
import AdminNodes from '@/views/admin/Nodes.vue'
import AdminSubscriptions from '@/views/admin/Subscriptions.vue'
import AdminPlans from '@/views/admin/Plans.vue'
import AdminTickets from '@/views/admin/Tickets.vue'
import AdminCoupons from '@/views/admin/Coupons.vue'
import AdminKnowledge from '@/views/admin/Knowledge.vue'
import AdminForward from '@/views/admin/Forward.vue'
import AdminPayment from '@/views/admin/Payment.vue'
import AdminTelegram from '@/views/admin/Telegram.vue'
import AdminMFA from '@/views/admin/MFA.vue'
import AdminNotifications from '@/views/admin/Notifications.vue'
import AdminInvite from '@/views/admin/Invite.vue'
import AdminSystem from '@/views/admin/System.vue'
import AdminAgent from '@/views/admin/Agent.vue'

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
        component: UserKnowledge
      },
      {
        path: 'tickets',
        component: UserTickets
      },
      {
        path: 'plans',
        component: UserPlans
      },
      {
        path: 'orders',
        component: UserOrders
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
        path: 'tickets',
        component: AdminTickets
      },
      {
        path: 'coupons',
        component: AdminCoupons
      },
      {
        path: 'knowledge',
        component: AdminKnowledge
      },
      {
        path: 'forward',
        component: AdminForward
      },
      {
        path: 'payment',
        component: AdminPayment
      },
      {
        path: 'telegram',
        component: AdminTelegram
      },
      {
        path: 'mfa',
        component: AdminMFA
      },
      {
        path: 'notifications',
        component: AdminNotifications
      },
      {
        path: 'invite',
        component: AdminInvite
      },
      {
        path: 'system',
        component: AdminSystem
      },
      {
        path: 'agent',
        component: AdminAgent
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
