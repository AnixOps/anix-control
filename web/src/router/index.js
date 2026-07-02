import { createRouter, createWebHistory } from 'vue-router'
import { useUserStore } from '@/stores/user'

// Layouts
import UserLayout from '@/layouts/UserLayout.vue'
import AdminLayout from '@/layouts/AdminLayout.vue'

// Lazy-loaded views
const Login = () => import('@/views/Login.vue')
const UserDashboard = () => import('@/views/user/Dashboard.vue')
const UserSubscribe = () => import('@/views/user/Subscribe.vue')
const UserKnowledge = () => import('@/views/user/Knowledge.vue')
const UserTickets = () => import('@/views/user/Tickets.vue')
const UserPlans = () => import('@/views/user/Plans.vue')
const UserOrders = () => import('@/views/user/Orders.vue')
const AdminDashboard = () => import('@/views/admin/Dashboard.vue')
const AdminMonitor = () => import('@/views/admin/Monitor.vue')
const AdminTrafficHourly = () => import('@/views/admin/TrafficHourly.vue')
const AdminUsers = () => import('@/views/admin/Users.vue')
const AdminOrders = () => import('@/views/admin/Orders.vue')
const AdminNodes = () => import('@/views/admin/Nodes.vue')
const AdminSubscriptions = () => import('@/views/admin/Subscriptions.vue')
const AdminPlans = () => import('@/views/admin/Plans.vue')
const AdminTickets = () => import('@/views/admin/Tickets.vue')
const AdminCoupons = () => import('@/views/admin/Coupons.vue')
const AdminKnowledge = () => import('@/views/admin/Knowledge.vue')
const AdminForward = () => import('@/views/admin/Forward.vue')
const AdminForwardWizard = () => import('@/views/admin/ForwardWizard.vue')
const AdminTunnel = () => import('@/views/admin/Tunnel.vue')
const AdminLimit = () => import('@/views/admin/Limit.vue')
const AdminAnsibleMachines = () => import('@/views/admin/AnsibleMachines.vue')
const AdminForwardNodes = () => import('@/views/admin/ForwardNodes.vue')
const AdminLocalRuntime = () => import('@/views/admin/LocalRuntime.vue')
const AdminNodeX = () => import('@/views/admin/NodeX.vue')
const AdminObservability = () => import('@/views/admin/Observability.vue')
const AdminPayment = () => import('@/views/admin/Payment.vue')
const AdminTelegram = () => import('@/views/admin/Telegram.vue')
const AdminMFA = () => import('@/views/admin/MFA.vue')
const AdminNotifications = () => import('@/views/admin/Notifications.vue')
const AdminInvite = () => import('@/views/admin/Invite.vue')
const AdminSystem = () => import('@/views/admin/System.vue')
const AdminAgent = () => import('@/views/admin/Agent.vue')

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
        path: 'monitor',
        component: AdminMonitor
      },
      {
        path: 'traffic-hourly',
        component: AdminTrafficHourly
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
        path: 'forward/setup',
        component: AdminForwardWizard
      },
      {
        path: 'forward/tunnel',
        component: AdminTunnel
      },
      {
        path: 'forward/limit',
        component: AdminLimit
      },
      {
        path: 'forward/ansible-machines',
        component: AdminAnsibleMachines
      },
      {
        path: 'forward/nodes',
        component: AdminForwardNodes
      },
      {
        path: 'forward/local',
        component: AdminLocalRuntime
      },
      {
        path: 'forward/nodex',
        component: AdminNodeX
      },
      {
        path: 'forward/observability',
        component: AdminObservability
      },
      {
        path: 'forward/agents',
        component: AdminAgent
      },
      {
        path: 'forward/tunnels',
        redirect: '/admin/forward/tunnel'
      },
      {
        path: 'forward/limits',
        redirect: '/admin/forward/limit'
      },
      {
        path: 'forward/ansible',
        redirect: '/admin/forward/ansible-machines'
      },
      {
        path: 'local',
        redirect: '/admin/forward/local'
      },
      {
        path: 'nodex',
        redirect: '/admin/forward/nodex'
      },
      {
        path: 'tunnel',
        redirect: '/admin/forward/tunnel'
      },
      {
        path: 'limit',
        redirect: '/admin/forward/limit'
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
