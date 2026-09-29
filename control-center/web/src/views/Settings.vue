<template>
  <div class="space-y-6">
    <h1 class="text-2xl font-bold text-white">Settings</h1>

    <div class="grid gap-6">
      <!-- Server Configuration -->
      <div class="bg-dark-800 rounded-xl p-6 border border-dark-700">
        <h2 class="text-lg font-semibold text-white mb-4">Server Configuration</h2>
        <div class="grid gap-4">
          <div>
            <label class="block text-sm text-dark-400 mb-1">Host</label>
            <input
              v-model="settings.host"
              type="text"
              class="w-full px-4 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white"
            />
          </div>
          <div>
            <label class="block text-sm text-dark-400 mb-1">Port</label>
            <input
              v-model="settings.port"
              type="number"
              class="w-full px-4 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white"
            />
          </div>
        </div>
      </div>

      <!-- Plugins -->
      <div class="bg-dark-800 rounded-xl p-6 border border-dark-700">
        <h2 class="text-lg font-semibold text-white mb-4">Plugins</h2>
        <div class="space-y-3">
          <div
            v-for="plugin in plugins"
            :key="plugin.name"
            class="flex items-center justify-between p-3 bg-dark-700/50 rounded-lg"
          >
            <div class="flex items-center gap-3">
              <div
                class="w-2 h-2 rounded-full"
                :class="plugin.status === 'running' ? 'bg-green-400' : 'bg-red-400'"
              />
              <span class="text-white">{{ plugin.name }}</span>
            </div>
            <span class="text-dark-400 text-sm">{{ plugin.version }}</span>
          </div>
        </div>
      </div>

      <div class="flex gap-4">
        <button
          @click="saveSettings"
          class="px-6 py-2 bg-primary-600 hover:bg-primary-700 text-white rounded-lg transition-colors"
        >
          Save Changes
        </button>
        <button
          @click="restartServer"
          class="px-6 py-2 bg-dark-700 hover:bg-dark-600 text-white rounded-lg transition-colors"
        >
          Restart Server
        </button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'

const settings = ref({
  host: '0.0.0.0',
  port: 8080
})

const plugins = ref([
  { name: 'ansible', status: 'running', version: '1.0.0' },
  { name: 'v2board', status: 'running', version: '1.0.0' },
  { name: 'v2bx', status: 'running', version: '1.0.0' },
  { name: 'agent', status: 'running', version: '1.0.0' }
])

function saveSettings() {
  console.log('Saving settings:', settings.value)
}

function restartServer() {
  console.log('Restarting server...')
}
</script>