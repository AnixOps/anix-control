<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <h1 class="text-2xl font-bold text-white">Dashboard</h1>
      <button
        @click="refresh"
        class="px-4 py-2 bg-dark-700 hover:bg-dark-600 text-white rounded-lg transition-colors"
      >
        Refresh
      </button>
    </div>

    <!-- Stats Cards -->
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
      <div
        v-for="stat in stats"
        :key="stat.title"
        class="bg-dark-800 rounded-xl p-6 border border-dark-700"
      >
        <div class="flex items-center justify-between">
          <div>
            <p class="text-dark-400 text-sm">{{ stat.title }}</p>
            <p class="text-2xl font-bold text-white mt-1">{{ stat.value }}</p>
          </div>
          <div
            class="w-12 h-12 rounded-lg flex items-center justify-center"
            :class="stat.color"
          >
            <component :is="stat.icon" class="w-6 h-6 text-white" />
          </div>
        </div>
        <div v-if="stat.change" class="mt-2">
          <span
            :class="stat.change > 0 ? 'text-green-400' : 'text-red-400'"
            class="text-sm"
          >
            {{ stat.change > 0 ? '↑' : '↓' }} {{ Math.abs(stat.change) }}%
          </span>
          <span class="text-dark-500 text-sm ml-1">vs last week</span>
        </div>
      </div>
    </div>

    <!-- Main Content Grid -->
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
      <!-- Infrastructure Status -->
      <div class="bg-dark-800 rounded-xl p-6 border border-dark-700">
        <h2 class="text-lg font-semibold text-white mb-4">Infrastructure</h2>
        <div class="space-y-3">
          <div
            v-for="service in services"
            :key="service.name"
            class="flex items-center justify-between p-3 bg-dark-700/50 rounded-lg"
          >
            <div class="flex items-center gap-3">
              <div
                class="w-2 h-2 rounded-full"
                :class="service.status === 'online' ? 'bg-green-400' : 'bg-red-400'"
              />
              <span class="text-white">{{ service.name }}</span>
            </div>
            <div class="text-right">
              <span class="text-dark-400">{{ service.count }}</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Recent Activity -->
      <div class="bg-dark-800 rounded-xl p-6 border border-dark-700">
        <h2 class="text-lg font-semibold text-white mb-4">Recent Activity</h2>
        <div class="space-y-3">
          <div
            v-for="activity in activities"
            :key="activity.id"
            class="flex items-start gap-3 p-3 bg-dark-700/50 rounded-lg"
          >
            <div
              class="w-8 h-8 rounded-full flex items-center justify-center shrink-0"
              :class="getActivityColor(activity.type)"
            >
              <component :is="getActivityIcon(activity.type)" class="w-4 h-4" />
            </div>
            <div class="flex-1 min-w-0">
              <p class="text-white text-sm">{{ activity.message }}</p>
              <p class="text-dark-500 text-xs mt-1">{{ activity.time }}</p>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Quick Actions -->
    <div class="bg-dark-800 rounded-xl p-6 border border-dark-700">
      <h2 class="text-lg font-semibold text-white mb-4">Quick Actions</h2>
      <div class="grid grid-cols-2 md:grid-cols-4 gap-4">
        <button
          v-for="action in quickActions"
          :key="action.name"
          @click="handleAction(action.action)"
          class="flex flex-col items-center gap-2 p-4 bg-dark-700 hover:bg-dark-600 rounded-lg transition-colors"
        >
          <component :is="action.icon" class="w-6 h-6 text-primary-400" />
          <span class="text-sm text-white">{{ action.name }}</span>
        </button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import {
  ServerIcon,
  UsersIcon,
  ComputerDesktopIcon,
  ChartBarIcon,
  CheckCircleIcon,
  XCircleIcon,
  ClockIcon,
  BoltIcon,
  DocumentTextIcon
} from '@heroicons/vue/24/outline'

const router = useRouter()

const stats = ref([
  { title: 'Total Nodes', value: '8', change: 0, icon: ServerIcon, color: 'bg-blue-600' },
  { title: 'Active Users', value: '357', change: 12, icon: UsersIcon, color: 'bg-green-600' },
  { title: 'Online Agents', value: '4', change: 0, icon: ComputerDesktopIcon, color: 'bg-purple-600' },
  { title: 'Traffic Today', value: '1.2TB', change: -5, icon: ChartBarIcon, color: 'bg-orange-600' }
])

const services = ref([
  { name: 'Panel', status: 'online', count: 'Running' },
  { name: 'Nodes', status: 'online', count: '8 / 8' },
  { name: 'Agents', status: 'online', count: '4 / 4' },
  { name: 'Monitoring', status: 'offline', count: 'Disabled' }
])

const activities = ref([
  { id: 1, type: 'deploy', message: 'Node tokyo-01 deployed successfully', time: '2 minutes ago' },
  { id: 2, type: 'user', message: 'User admin logged in', time: '15 minutes ago' },
  { id: 3, type: 'backup', message: 'Database backup completed', time: '1 hour ago' },
  { id: 4, type: 'alert', message: 'Certificate expires in 7 days', time: '2 hours ago' },
  { id: 5, type: 'error', message: 'Agent cache-01 connection failed', time: '3 hours ago' }
])

const quickActions = ref([
  { name: 'Deploy Node', action: 'deploy', icon: ServerIcon },
  { name: 'Manage Users', action: 'users', icon: UsersIcon },
  { name: 'Run Playbook', action: 'playbook', icon: DocumentTextIcon },
  { name: 'View Logs', action: 'logs', icon: ChartBarIcon }
])

function getActivityColor(type) {
  const colors = {
    deploy: 'bg-green-600',
    user: 'bg-blue-600',
    backup: 'bg-purple-600',
    alert: 'bg-yellow-600',
    error: 'bg-red-600'
  }
  return colors[type] || 'bg-gray-600'
}

function getActivityIcon(type) {
  const icons = {
    deploy: CheckCircleIcon,
    user: UsersIcon,
    backup: ClockIcon,
    alert: BoltIcon,
    error: XCircleIcon
  }
  return icons[type] || ClockIcon
}

function handleAction(action) {
  const routes = {
    deploy: '/nodes',
    users: '/users',
    playbook: '/playbooks',
    logs: '/logs'
  }
  router.push(routes[action])
}

function refresh() {
  // TODO: Implement refresh
}

onMounted(() => {
  // TODO: Fetch data from API
})
</script>