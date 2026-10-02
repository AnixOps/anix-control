import { createRouter, createWebHistory } from 'vue-router'
import { useUserStore } from '@/stores/user'
import { ensureAdminExtensions, resetAdminExtensions } from '@/extensions/runtime'
import { resolveLegacyControlRedirect } from '@/router/controlLegacy'
import { loadEdition, routeAllowedByEdition } from '@/composables/useEdition'
import { scrollBehavior } from '@/router/scroll'

// Layouts: lazy, so the login page does not load the shells (and the
// component library they use) before anyone has signed in.
const UserLayout = () => import('@/layouts/UserLayout.vue')
const AdminLayout = () => import('@/layouts/AdminLayout.vue')

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
const AdminInviteCodes = () => import('@/views/admin/InviteCodes.vue')
const AdminSystem = () => import('@/views/admin/System.vue')
const AdminAgent = () => import('@/views/admin/Agent.vue')
const AdminPlugins = () => import('@/views/admin/Plugins.vue')
const AdminDeployments = () => import('@/views/admin/Deployments.vue')
const AdminAccessGroups = () => import('@/views/admin/AccessGroups.vue')
const Account = () => import('@/views/Account.vue')
const StatusPage = () => import('@/views/StatusPage.vue')

const ACCOUNT_META = Object.freeze({ titleKey: 'shell.accountPage.title' })
const NOT_FOUND_META = Object.freeze({ titleKey: 'shell.status.notFound.title' })
// Pages with wide tables keep to --size-content-wide instead of
// --size-content-admin (AdminLayout).
const WIDE = Object.freeze({ layout: 'wide' })

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
        component: UserPlans,
        meta: { edition: 'commercial' }
      },
      {
        path: 'orders',
        component: UserOrders,
        meta: { edition: 'commercial' }
      },
      {
        path: 'account',
        component: Account,
        meta: ACCOUNT_META
      },
      {
        path: '',
        redirect: '/user/dashboard'
      },
      {
        path: ':pathMatch(.*)*',
        component: StatusPage,
        props: { kind: 'not-found' },
        meta: NOT_FOUND_META
      }
    ]
  },
  // Admin Routes
  {
    path: '/admin',
    name: 'admin',
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
        component: AdminUsers,
        meta: WIDE
      },
      {
        path: 'nodes',
        component: AdminNodes,
        meta: WIDE
      },
      {
        path: 'orders',
        component: AdminOrders,
        meta: { edition: 'commercial', ...WIDE }
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
        component: AdminCoupons,
        meta: { edition: 'commercial' }
      },
      {
        path: 'knowledge',
        component: AdminKnowledge
      },
      {
        path: 'forward',
        component: AdminForward,
        meta: WIDE
      },
      {
        path: 'forward/setup',
        component: AdminForwardWizard
      },
      {
        path: 'forward/tunnel',
        component: AdminTunnel,
        meta: WIDE
      },
      {
        path: 'forward/limit',
        component: AdminLimit,
        meta: WIDE
      },
      {
        path: 'forward/ansible-machines',
        component: AdminAnsibleMachines,
        meta: WIDE
      },
      {
        path: 'forward/nodes',
        component: AdminForwardNodes,
        meta: WIDE
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
        component: AdminPayment,
        meta: { edition: 'commercial' }
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
        component: AdminInvite,
        meta: { edition: 'commercial' }
      },
      {
        path: 'invite-codes',
        component: AdminInviteCodes
      },
      {
        path: 'system',
        component: AdminSystem
      },
      {
        path: 'agent',
        component: AdminAgent
      },
      {
        path: 'plugins',
        component: AdminPlugins
      },
      {
        path: 'deployments',
        component: AdminDeployments,
        meta: WIDE
      },
      {
        path: 'control',
        redirect: resolveLegacyControlRedirect
      },
      {
        path: 'access-groups',
        component: AdminAccessGroups
      },
      {
        path: 'account',
        component: Account,
        meta: ACCOUNT_META
      },
      {
        // Named so the parent's name ('admin', which extensions add their
        // pages under) does not trigger Vue Router's empty-path warning.
        path: '',
        name: 'admin-index',
        redirect: '/admin/dashboard'
      },
      {
        path: ':pathMatch(.*)*',
        component: StatusPage,
        props: { kind: 'not-found' },
        meta: NOT_FOUND_META
      }
    ]
  },
  // Unknown paths outside the shells, and admin pages a signed-in user may
  // not open. Both keep the URL as typed: the guard redirects by name with
  // the path as the parameter.
  {
    path: '/:pathMatch(.*)*',
    name: 'not-found',
    component: StatusPage,
    props: { kind: 'not-found', standalone: true },
    meta: NOT_FOUND_META
  },
  {
    path: '/:forbiddenPath(.*)*',
    name: 'forbidden',
    component: StatusPage,
    props: { kind: 'forbidden', standalone: true },
    meta: { titleKey: 'shell.status.forbidden.title' }
  }
]

const router = createRouter({
  history: createWebHistory(),
  routes,
  scrollBehavior
})

// editionFallback is where a page the edition does not serve leads.
function editionFallback(to) {
  return to.path.startsWith('/admin') ? '/admin/dashboard' : '/user/dashboard'
}

// Navigation Guards
// forbiddenLocation shows 无权限 at the requested URL.
function forbiddenLocation(to) {
  return {
    name: 'forbidden',
    params: { forbiddenPath: to.path.replace(/^\//, '').split('/') },
    query: to.query,
    hash: to.hash,
    replace: true
  }
}

router.beforeEach(async (to, from, next) => {
  const userStore = useUserStore()
  const isAdminTarget = to.path === '/admin' || to.path.startsWith('/admin/')

  // The status pages are terminal: no auth or edition checks (a redirect
  // to them keeps an /admin URL, which would loop otherwise).
  if (to.name === 'forbidden' || to.name === 'not-found') {
    next()
    return
  }

  // Commercial pages (meta.edition) exist only in the commercial edition.
  if (to.matched.some(record => record.meta?.edition || record.meta?.extensionPluginID)) {
    await loadEdition()
    if (!routeAllowedByEdition(to)) {
      next(editionFallback(to))
      return
    }
  }

  if ((to.meta.requiresAuth || isAdminTarget) && !userStore.isLoggedIn) {
    resetAdminExtensions()
    next('/login')
  } else if ((to.meta.requiresAdmin || isAdminTarget) && !userStore.isAdmin) {
    resetAdminExtensions()
    next(forbiddenLocation(to)) // Signed in, but not an administrator: 无权限
  } else if (to.meta.guest && userStore.isLoggedIn) {
    if (userStore.isAdmin) {
      next('/admin/dashboard')
    } else {
      next('/user/dashboard')
    }
  } else {
    if (to.path.startsWith('/admin/') && userStore.isAdmin) {
      // Login responses from older panels may not carry the kernel grants.
      // Resolve the authoritative profile before discovering any extension so
      // the first admin render cannot silently omit authorized plugin UI.
      if (userStore.permissionMode === 'missing') {
        await userStore.getUserInfo()
      }
      const isUnmatchedExtension = to.path.startsWith('/admin/extensions/') && !to.matched.some(record => record.meta.extension)
      if (isUnmatchedExtension) {
        await ensureAdminExtensions(router)
        const resolved = router.resolve(to.fullPath)
        if (resolved.matched.some(record => record.meta.extension)) {
          const extensionPermission = resolved.meta.extensionPermission
          if (!userStore.hasPermission(extensionPermission)) {
            next('/admin/plugins')
          } else {
            next({ path: to.path, query: to.query, hash: to.hash, replace: true })
          }
        } else {
          next('/admin/plugins')
        }
        return
      }
      if (to.meta.extension && !userStore.hasPermission(to.meta.extensionPermission)) {
        next('/admin/plugins')
        return
      }
      if (to.meta.extension) {
        next()
        return
      }
      void ensureAdminExtensions(router).catch(() => {
        // Optional extension discovery must never block core admin routes.
      })
    }
    next()
  }
})

export default router
