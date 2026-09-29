<template>
  <div class="space-y-6">
    <h1 class="text-2xl font-bold text-white">Playbooks</h1>

    <!-- Playbooks List -->
    <div class="grid gap-4">
      <div
        v-for="playbook in playbooks"
        :key="playbook.name"
        class="bg-dark-800 rounded-xl p-6 border border-dark-700 hover:border-dark-600 transition-colors"
      >
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-4">
            <div
              class="w-10 h-10 rounded-lg flex items-center justify-center"
              :class="getStatusColor(playbook.status)"
            >
              <component :is="getStatusIcon(playbook.status)" class="w-5 h-5" />
            </div>
            <div>
              <h3 class="text-white font-medium">{{ playbook.name }}</h3>
              <p class="text-dark-400 text-sm">Last run: {{ playbook.lastRun }}</p>
            </div>
          </div>
          <div class="flex items-center gap-2">
            <span
              class="px-2 py-1 text-xs rounded-full"
              :class="getStatusColor(playbook.status)"
            >
              {{ playbook.status }}
            </span>
            <button
              @click="runPlaybook(playbook)"
              class="px-4 py-2 bg-primary-600 hover:bg-primary-700 text-white text-sm rounded-lg transition-colors"
            >
              Run
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import {
  CheckCircleIcon,
  XCircleIcon,
  ClockIcon
} from '@heroicons/vue/24/outline'

const playbooks = ref([
  { name: 'deploy_node.yml', lastRun: '2024-03-15 12:34', status: 'Success' },
  { name: 'update_certificates.yml', lastRun: '2024-03-14 08:00', status: 'Success' },
  { name: 'backup_database.yml', lastRun: '2024-03-15 00:00', status: 'Running' },
  { name: 'cleanup_logs.yml', lastRun: '2024-03-13 06:00', status: 'Failed' },
  { name: 'deploy_agent.yml', lastRun: '2024-03-12 18:30', status: 'Success' }
])

function getStatusColor(status) {
  const colors = {
    'Success': 'bg-green-900/30 text-green-400',
    'Failed': 'bg-red-900/30 text-red-400',
    'Running': 'bg-yellow-900/30 text-yellow-400'
  }
  return colors[status] || 'bg-gray-900/30 text-gray-400'
}

function getStatusIcon(status) {
  const icons = {
    'Success': CheckCircleIcon,
    'Failed': XCircleIcon,
    'Running': ClockIcon
  }
  return icons[status] || ClockIcon
}

function runPlaybook(playbook) {
  console.log('Running playbook:', playbook.name)
}
</script>