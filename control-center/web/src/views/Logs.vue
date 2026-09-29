<template>
  <div class="space-y-6">
    <h1 class="text-2xl font-bold text-white">Logs</h1>

    <div class="bg-dark-800 rounded-xl border border-dark-700 overflow-hidden">
      <div class="p-4 border-b border-dark-700 flex items-center gap-4">
        <select
          v-model="levelFilter"
          class="px-3 py-1 bg-dark-700 border border-dark-600 rounded-lg text-white text-sm"
        >
          <option value="">All Levels</option>
          <option value="INFO">INFO</option>
          <option value="WARN">WARN</option>
          <option value="ERROR">ERROR</option>
        </select>
        <button
          @click="clearLogs"
          class="px-3 py-1 bg-dark-700 hover:bg-dark-600 text-white text-sm rounded-lg transition-colors"
        >
          Clear
        </button>
      </div>

      <div class="font-mono text-sm max-h-[600px] overflow-y-auto">
        <div
          v-for="(log, index) in filteredLogs"
          :key="index"
          class="px-4 py-2 border-b border-dark-700/50 hover:bg-dark-700/50"
        >
          <span class="text-dark-500">{{ log.time }}</span>
          <span
            class="ml-2 px-1 rounded"
            :class="getLevelColor(log.level)"
          >
            {{ log.level }}
          </span>
          <span class="ml-2 text-dark-400">[{{ log.source }}]</span>
          <span class="ml-2 text-white">{{ log.message }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'

const levelFilter = ref('')

const logs = ref([
  { time: '12:34:56', level: 'INFO', source: 'node', message: 'Node tokyo-01 deployed successfully' },
  { time: '12:34:55', level: 'INFO', source: 'ansible', message: 'Running playbook: deploy_node.yml' },
  { time: '12:33:21', level: 'INFO', source: 'auth', message: 'User admin logged in from 192.168.1.1' },
  { time: '12:30:00', level: 'INFO', source: 'backup', message: 'Database backup completed' },
  { time: '12:28:45', level: 'WARN', source: 'cert', message: 'Certificate for node-03 expires in 7 days' },
  { time: '12:20:00', level: 'ERROR', source: 'agent', message: 'Failed to connect to agent cache-01' },
  { time: '12:15:30', level: 'INFO', source: 'traffic', message: 'Traffic report: 1.2TB uploaded' }
])

const filteredLogs = computed(() => {
  if (!levelFilter.value) return logs.value
  return logs.value.filter(log => log.level === levelFilter.value)
})

function getLevelColor(level) {
  const colors = {
    'INFO': 'text-blue-400',
    'WARN': 'text-yellow-400',
    'ERROR': 'text-red-400'
  }
  return colors[level] || 'text-gray-400'
}

function clearLogs() {
  logs.value = []
}
</script>