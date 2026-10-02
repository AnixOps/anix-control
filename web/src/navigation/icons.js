import {
  Activity,
  ArrowLeftRight,
  Bell,
  BookOpen,
  Bot,
  Box,
  Cable,
  ChartColumn,
  CreditCard,
  Gauge,
  Gift,
  HardDrive,
  House,
  Inbox,
  KeyRound,
  Layers,
  LayoutDashboard,
  LayoutTemplate,
  LifeBuoy,
  Link,
  MessageSquare,
  Network,
  Package,
  Plug,
  Puzzle,
  Radio,
  Receipt,
  Rocket,
  Send,
  Server,
  ServerCog,
  Settings,
  Shield,
  ShieldCheck,
  SlidersHorizontal,
  TicketCheck,
  TicketPercent,
  UserRound,
  Users,
  Workflow
} from '@lucide/vue'

// Navigation icons by name. The map is closed on purpose: menu metadata
// from signed plugin packages can only pick a known icon (or the neutral
// box), never a component of its own.
export const NAV_ICONS = Object.freeze({
  // admin
  dashboard: LayoutDashboard,
  monitor: Gauge,
  traffic: ChartColumn,
  users: Users,
  'invite-codes': TicketCheck,
  subscriptions: Layers,
  templates: LayoutTemplate,
  tickets: Inbox,
  knowledge: BookOpen,
  nodes: Server,
  forward: ArrowLeftRight,
  'forward-nodes': Network,
  agents: Bot,
  plugins: Puzzle,
  deployments: Rocket,
  settings: Settings,
  mfa: ShieldCheck,
  security: Shield,
  'access-groups': KeyRound,
  notifications: Bell,
  telegram: Send,
  plans: Package,
  orders: Receipt,
  coupons: TicketPercent,
  payment: CreditCard,
  invite: Gift,
  account: UserRound,
  // forward suite
  setup: Workflow,
  tunnel: Cable,
  limits: SlidersHorizontal,
  ansible: ServerCog,
  local: HardDrive,
  nodex: Radio,
  observability: Activity,
  // user
  home: House,
  subscribe: Link,
  help: LifeBuoy,
  conversation: MessageSquare,
  // plugin menus: the two-letter codes extensions/runtime.js derives from
  // the package's icon name
  AC: Activity,
  EX: Box,
  GA: Gauge,
  NW: Network,
  PL: Plug,
  SV: Server,
  ST: Settings,
  SH: Shield
})

export function navIcon(name) {
  return NAV_ICONS[name] || Box
}

export function navIconName(name) {
  return NAV_ICONS[name] ? name : 'box'
}
