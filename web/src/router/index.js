import { createRouter, createWebHistory } from 'vue-router'
import { useUserStore } from '@/stores/user'
import { resolveLegacyControlRedirect } from '@/router/controlLegacy'
import { loadEdition, routeAllowedByEdition } from '@/composables/useEdition'
import { scrollBehavior } from '@/router/scroll'
import { loadMessageGroup } from '@/i18n'

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
const AdminUsers = () => import('@/views/admin/Users.vue')
const AdminOrders = () => import('@/views/admin/Orders.vue')
const AdminNodes = () => import('@/views/admin/Nodes.vue')
const AdminNodeDetail = () => import('@/views/admin/NodeDetail.vue')
const AdminSubscriptions = () => import('@/views/admin/Subscriptions.vue')
const AdminSubscriptionGroup = () => import('@/views/admin/SubscriptionGroup.vue')
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
const AdminForwardNodeDetail = () => import('@/views/admin/forward-nodes/ForwardNodeDetail.vue')
const AdminAnsibleMachineDetail = () => import('@/views/admin/forward-nodes/AnsibleMachineDetail.vue')
const AdminPayment = () => import('@/views/admin/Payment.vue')
const AdminSecurity = () => import('@/views/admin/Security.vue')
const AdminNotifications = () => import('@/views/admin/Notifications.vue')
const AdminInvite = () => import('@/views/admin/Invite.vue')
const AdminInviteCodes = () => import('@/views/admin/InviteCodes.vue')
const AdminSystem = () => import('@/views/admin/System.vue')
const AdminAgent = () => import('@/views/admin/Agent.vue')
const AdminAgentTransports = () => import('@/views/admin/AgentTransports.vue')
const AdminPlugins = () => import('@/views/admin/Plugins.vue')
const AdminRouteModes = () => import('@/views/admin/RouteModes.vue')
const AdminDeployments = () => import('@/views/admin/Deployments.vue')
// The v4.2 forwarding area (F5b): lazy chunks, opened only with the
// forward package's v4 API (meta.capability, checked in the guard).
const ForwardOverview = () => import('@/views/admin/forward/Overview.vue')
const ForwardRoutes = () => import('@/views/admin/forward/Routes.vue')
const ForwardRouteEditor = () => import('@/views/admin/forward/RouteEditor.vue')
const ForwardRouteDetail = () => import('@/views/admin/forward/RouteDetail.vue')
const ForwardNodes = () => import('@/views/admin/forward/Nodes.vue')
const ForwardNodeView = () => import('@/views/admin/forward/NodeDetail.vue')
const Account = () => import('@/views/Account.vue')
const StatusPage = () => import('@/views/StatusPage.vue')

const ACCOUNT_META = Object.freeze({ titleKey: 'shell.accountPage.title' })
const NOT_FOUND_META = Object.freeze({ titleKey: 'shell.status.notFound.title' })
// Pages with wide tables keep to --size-content-wide instead of
// --size-content-admin (AdminLayout).
const WIDE = Object.freeze({ layout: 'wide' })
// The forwarding area's pages need the forward package's v4 API.
const FORWARD_V4 = 'forward.v4'
const forwardMeta = titleKey => Object.freeze({ layout: 'wide', capability: FORWARD_V4, titleKey })

// The extension runtime (and the kernel API it calls) is admin-only: it loads
// with the first admin route, so the sign-in page does not carry it.
let extensionRuntime = null

function loadExtensionRuntime() {
  return import('@/extensions/runtime').then((module) => {
    extensionRuntime = module
    return module
  })
}

async function ensureAdminExtensions(targetRouter) {
  return (await loadExtensionRuntime()).ensureAdminExtensions(targetRouter)
}

// Nothing to reset before the runtime has loaded.
function resetAdminExtensions() {
  extensionRuntime?.resetAdminExtensions()
}

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
        // 流量与监控 (UI U8): live nodes, user traffic, node latency and
        // forwards as sections in the path; the old pages redirect.
        path: 'monitor/:section?',
        component: AdminMonitor
      },
      {
        path: 'traffic-hourly',
        redirect: to => ({ path: '/admin/monitor/traffic', query: to.query })
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
        // A node's page (UI U7); ?section= picks the section.
        path: 'nodes/:id(\\d+)',
        component: AdminNodeDetail,
        meta: { ...WIDE, titleKey: 'admin.nodes.detail.title' }
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
        // One subscription group, its section in the path (UI U7).
        path: 'subscriptions/:id(\\d+)/:section?',
        component: AdminSubscriptionGroup
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
      // The v4.2 forwarding area (F5b, docs/design/forward-ui): 概览, 路由
      // and 节点, on paths the flux-clone pages above never used.
      {
        path: 'forward/overview',
        component: ForwardOverview,
        meta: forwardMeta('pageTitles.admin.forwardV4Overview')
      },
      {
        path: 'forward/routes',
        component: ForwardRoutes,
        meta: forwardMeta('pageTitles.admin.forwardV4Routes')
      },
      {
        path: 'forward/routes/new',
        component: ForwardRouteEditor,
        meta: forwardMeta('pageTitles.admin.forwardV4RouteNew')
      },
      {
        path: 'forward/routes/:id',
        component: ForwardRouteDetail,
        props: true,
        meta: forwardMeta('pageTitles.admin.forwardV4Route')
      },
      {
        path: 'forward/routes/:id/edit',
        component: ForwardRouteEditor,
        props: true,
        meta: forwardMeta('pageTitles.admin.forwardV4RouteEdit')
      },
      {
        path: 'forward/inventory',
        component: ForwardNodes,
        meta: forwardMeta('pageTitles.admin.forwardV4Nodes')
      },
      {
        path: 'forward/inventory/:nodeRef',
        component: ForwardNodeView,
        props: true,
        meta: forwardMeta('pageTitles.admin.forwardV4Node')
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
      // Detail pages of 转发节点 (UI U7); the ids are numbers.
      {
        path: 'forward/nodes/:id(\\d+)',
        component: AdminForwardNodeDetail,
        props: route => ({ id: Number(route.params.id) }),
        meta: { titleKey: 'forwardNodesPage.detail.sections' }
      },
      {
        path: 'forward/ansible-machines/:id(\\d+)',
        component: AdminAnsibleMachineDetail,
        props: route => ({ id: Number(route.params.id) }),
        meta: { titleKey: 'forwardNodesPage.detail.sections' }
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
        // 转发可观测性 is the 转发 section of 流量与监控 now (UI U8).
        path: 'forward/observability',
        redirect: to => ({ path: '/admin/monitor/forward', query: to.query })
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
        // Telegram is a channel of 通知 now (UI U7).
        path: 'telegram',
        redirect: '/admin/notifications/telegram'
      },
      {
        // 安全 holds the MFA policy and the access groups (UI U7).
        path: 'security/:section?',
        component: AdminSecurity
      },
      {
        path: 'mfa',
        redirect: '/admin/security/mfa'
      },
      {
        // 通知: the channel is in the path (UI U7).
        path: 'notifications/:channel?',
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
        // 系统设置: the section is in the path (UI U7).
        path: 'system/:section?',
        component: AdminSystem
      },
      {
        path: 'agent',
        component: AdminAgent
      },
      {
        // Agent transport inventory (A2-6), below NodeX Agents.
        path: 'agent/transports',
        component: AdminAgentTransports,
        meta: WIDE
      },
      {
        path: 'plugins',
        component: AdminPlugins
      },
      {
        // Per-package v2 route modes (legacy / shadow / native), below 插件中心.
        path: 'plugins/route-modes',
        component: AdminRouteModes,
        meta: WIDE
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
        redirect: '/admin/security/access-groups'
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

// Admin paths that need only the `admin` message group.
const ADMIN_SHELL_PATHS = new Set(['/admin', '/admin/', '/admin/dashboard'])

function whenIdle(task) {
  if (typeof window !== 'undefined' && typeof window.requestIdleCallback === 'function') {
    window.requestIdleCallback(task, { timeout: 3000 })
  } else {
    setTimeout(task, 1000)
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

  // Admin messages load with the first admin page; an admin on a user page
  // gets them too (⌘K and the account menu list admin pages). A failed load
  // leaves the keys untranslated rather than blocking the navigation.
  if (userStore.isAdmin) {
    await loadMessageGroup('admin').catch(() => {})
    // The other admin pages' messages load before the first of them opens;
    // the dashboard, an admin's first visit, renders without them and they
    // follow once the browser is idle.
    if (isAdminTarget && !ADMIN_SHELL_PATHS.has(to.path)) {
      await loadMessageGroup('adminPages').catch(() => {})
    } else {
      whenIdle(() => loadMessageGroup('adminPages').catch(() => {}))
    }
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
      // A page that needs a package capability (the forwarding area needs
      // the forward package's v4 API) waits for the catalog; without the
      // capability it leads to the flux-clone forwarding page.
      if (to.meta.capability) {
        const snapshot = await ensureAdminExtensions(router).catch(() => null)
        if (!snapshot?.capabilities?.includes(to.meta.capability)) {
          next({ path: '/admin/forward', replace: true })
          return
        }
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
