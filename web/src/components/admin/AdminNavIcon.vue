<template>
  <component
    :is="icon"
    class="admin-nav-icon"
    :data-admin-nav-icon="resolvedName"
    :size="normalizedSize"
    :stroke-width="1.8"
    aria-hidden="true"
    focusable="false"
  />
</template>

<script setup>
import { computed } from 'vue'
import {
  Activity,
  BookOpen,
  Bot,
  Box,
  Cable,
  ChartNoAxesCombined,
  CircleGauge,
  CreditCard,
  Ellipsis,
  Gauge,
  HardDrive,
  LayoutDashboard,
  Network,
  Package,
  PanelsTopLeft,
  Radio,
  Rocket,
  ServerCog,
  Settings,
  ShieldCheck,
  ShoppingCart,
  SlidersHorizontal,
  Ticket,
  Users,
  Workflow
} from '@lucide/vue'

const props = defineProps({
  name: {
    type: String,
    required: true
  },
  size: {
    type: Number,
    default: 18
  }
})

// Navigation metadata crosses a signed extension boundary. Keep the map closed
// so an extension can choose only a known Lucide icon or the neutral fallback.
const icons = Object.freeze({
  dashboard: LayoutDashboard,
  monitor: Gauge,
  traffic: ChartNoAxesCombined,
  users: Users,
  orders: ShoppingCart,
  tickets: Ticket,
  network: Network,
  nodes: Network,
  subscriptions: PanelsTopLeft,
  plans: Package,
  coupons: Ticket,
  invite: Users,
  payment: CreditCard,
  knowledge: BookOpen,
  mfa: ShieldCheck,
  'access-groups': Users,
  system: Settings,
  plugins: Package,
  deployments: Rocket,
  control: Settings,
  agents: Bot,
  more: Ellipsis,
  setup: Workflow,
  forward: Activity,
  tunnel: Cable,
  limits: SlidersHorizontal,
  ansible: ServerCog,
  local: HardDrive,
  nodex: Radio,
  observability: CircleGauge
})

const icon = computed(() => icons[props.name] || Box)
const resolvedName = computed(() => (icons[props.name] ? props.name : 'box'))
const normalizedSize = computed(() => (
  Number.isFinite(props.size) && props.size > 0 && props.size <= 48 ? props.size : 18
))
</script>
