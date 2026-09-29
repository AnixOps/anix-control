<template>
  <div class="space-y-6">
    <h1 class="text-2xl font-bold text-white">Agents</h1>

    <div class="grid gap-4">
      <div
        v-for="agent in agents"
        :key="agent.id"
        class="bg-dark-800 rounded-xl p-6 border border-dark-700"
      >
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-4">
            <div
              class="w-10 h-10 rounded-lg flex items-center justify-center"
              :class="agent.status === 'online' ? 'bg-green-600' : 'bg-red-600'"
            >
              <ComputerDesktopIcon class="w-5 h-5 text-white" />
            </div>
            <div>
              <h3 class="text-white font-medium">{{ agent.name }}</h3>
              <p class="text-dark-400 text-sm">{{ agent.host }}</p>
            </div>
          </div>
          <div class="flex items-center gap-4">
            <div class="text-right">
              <p class="text-dark-400 text-sm">Uptime</p>
              <p class="text-white">{{ agent.uptime }}</p>
            </div>
            <button
              @click="connectAgent(agent)"
              class="px-4 py-2 bg-primary-600 hover:bg-primary-700 text-white text-sm rounded-lg transition-colors"
            >
              Connect
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { ComputerDesktopIcon } from '@heroicons/vue/24/outline'

const agents = ref([
  { id: 1, name: 'web-01', host: '10.0.0.101', status: 'online', uptime: '2d 4h' },
  { id: 2, name: 'db-01', host: '10.0.0.102', status: 'online', uptime: '5d 12h' },
  { id: 3, name: 'cache-01', host: '10.0.0.103', status: 'offline', uptime: '-' }
])

function connectAgent(agent) {
  console.log('Connecting to agent:', agent.name)
}
</script>