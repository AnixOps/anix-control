<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <h1 class="text-2xl font-bold text-white">Nodes</h1>
      <button
        @click="showCreateModal = true"
        class="px-4 py-2 bg-primary-600 hover:bg-primary-700 text-white rounded-lg transition-colors"
      >
        Add Node
      </button>
    </div>

    <!-- Filters -->
    <div class="flex gap-4">
      <input
        v-model="nodesStore.searchQuery"
        type="text"
        placeholder="Search nodes..."
        class="flex-1 px-4 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white placeholder-dark-400 focus:outline-none focus:ring-2 focus:ring-primary-500"
      />
      <select
        v-model="nodesStore.statusFilter"
        class="px-4 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:ring-2 focus:ring-primary-500"
      >
        <option value="">All Status</option>
        <option value="online">Online</option>
        <option value="offline">Offline</option>
      </select>
    </div>

    <!-- Loading State -->
    <div v-if="nodesStore.loading" class="flex justify-center py-8">
      <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-500"></div>
    </div>

    <!-- Error State -->
    <div v-else-if="nodesStore.error" class="bg-red-900/30 text-red-400 px-4 py-3 rounded-lg">
      {{ nodesStore.error }}
    </div>

    <!-- Empty State -->
    <div v-else-if="nodesStore.nodes.length === 0" class="text-center py-8 text-dark-400">
      No nodes found. Add your first node to get started.
    </div>

    <!-- Nodes Table -->
    <div v-else class="bg-dark-800 rounded-xl border border-dark-700 overflow-hidden">
      <table class="w-full">
        <thead class="bg-dark-700">
          <tr>
            <th class="px-6 py-3 text-left text-xs font-medium text-dark-300 uppercase tracking-wider">Name</th>
            <th class="px-6 py-3 text-left text-xs font-medium text-dark-300 uppercase tracking-wider">Host</th>
            <th class="px-6 py-3 text-left text-xs font-medium text-dark-300 uppercase tracking-wider">Status</th>
            <th class="px-6 py-3 text-left text-xs font-medium text-dark-300 uppercase tracking-wider">Users</th>
            <th class="px-6 py-3 text-left text-xs font-medium text-dark-300 uppercase tracking-wider">Traffic</th>
            <th class="px-6 py-3 text-right text-xs font-medium text-dark-300 uppercase tracking-wider">Actions</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-dark-700">
          <tr
            v-for="node in nodesStore.filteredNodes"
            :key="node.id"
            class="hover:bg-dark-700/50"
          >
            <td class="px-6 py-4 whitespace-nowrap">
              <div class="flex items-center gap-3">
                <div class="w-8 h-8 rounded-lg bg-primary-600/20 flex items-center justify-center">
                  <ServerIcon class="w-4 h-4 text-primary-400" />
                </div>
                <span class="text-white font-medium">{{ node.name }}</span>
              </div>
            </td>
            <td class="px-6 py-4 whitespace-nowrap text-dark-300">{{ node.host }}</td>
            <td class="px-6 py-4 whitespace-nowrap">
              <span
                class="px-2 py-1 text-xs rounded-full"
                :class="node.status === 'online' ? 'bg-green-900/30 text-green-400' : 'bg-red-900/30 text-red-400'"
              >
                {{ node.status }}
              </span>
            </td>
            <td class="px-6 py-4 whitespace-nowrap text-dark-300">{{ node.users ?? '-' }}</td>
            <td class="px-6 py-4 whitespace-nowrap text-dark-300">{{ formatTraffic(node.traffic) }}</td>
            <td class="px-6 py-4 whitespace-nowrap text-right">
              <div class="flex items-center justify-end gap-2">
                <button
                  @click="viewNode(node)"
                  class="p-2 hover:bg-dark-600 rounded-lg transition-colors"
                  title="View"
                >
                  <EyeIcon class="w-4 h-4 text-dark-400" />
                </button>
                <button
                  @click="editNode(node)"
                  class="p-2 hover:bg-dark-600 rounded-lg transition-colors"
                  title="Edit"
                >
                  <PencilIcon class="w-4 h-4 text-dark-400" />
                </button>
                <button
                  @click="confirmDelete(node)"
                  class="p-2 hover:bg-dark-600 rounded-lg transition-colors"
                  title="Delete"
                >
                  <TrashIcon class="w-4 h-4 text-red-400" />
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Create/Edit Modal -->
    <div v-if="showCreateModal || editingNode" class="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div class="bg-dark-800 rounded-xl p-6 w-full max-w-md border border-dark-700">
        <h2 class="text-xl font-bold text-white mb-4">
          {{ editingNode ? 'Edit Node' : 'Add Node' }}
        </h2>

        <form @submit.prevent="saveNode" class="space-y-4">
          <div>
            <label class="block text-sm text-dark-300 mb-1">Name</label>
            <input
              v-model="formData.name"
              type="text"
              required
              class="w-full px-3 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:ring-2 focus:ring-primary-500"
            />
          </div>

          <div>
            <label class="block text-sm text-dark-300 mb-1">Host</label>
            <input
              v-model="formData.host"
              type="text"
              required
              placeholder="192.168.1.100"
              class="w-full px-3 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:ring-2 focus:ring-primary-500"
            />
          </div>

          <div>
            <label class="block text-sm text-dark-300 mb-1">SSH Port</label>
            <input
              v-model.number="formData.port"
              type="number"
              class="w-full px-3 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:ring-2 focus:ring-primary-500"
            />
          </div>

          <div>
            <label class="block text-sm text-dark-300 mb-1">Username</label>
            <input
              v-model="formData.username"
              type="text"
              class="w-full px-3 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white focus:outline-none focus:ring-2 focus:ring-primary-500"
            />
          </div>

          <div class="flex gap-3 pt-4">
            <button
              type="button"
              @click="closeModal"
              class="flex-1 px-4 py-2 bg-dark-600 hover:bg-dark-500 text-white rounded-lg transition-colors"
            >
              Cancel
            </button>
            <button
              type="submit"
              :disabled="nodesStore.loading"
              class="flex-1 px-4 py-2 bg-primary-600 hover:bg-primary-700 text-white rounded-lg transition-colors disabled:opacity-50"
            >
              {{ editingNode ? 'Update' : 'Create' }}
            </button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import {
  ServerIcon,
  EyeIcon,
  PencilIcon,
  TrashIcon
} from '@heroicons/vue/24/outline'
import { useNodesStore } from '@/stores/nodes'
import { useAuthStore } from '@/stores/auth'

const nodesStore = useNodesStore()
const authStore = useAuthStore()

const showCreateModal = ref(false)
const editingNode = ref(null)
const formData = ref({
  name: '',
  host: '',
  port: 22,
  username: 'root'
})

onMounted(async () => {
  await nodesStore.fetchNodes()

  if (authStore.token) {
    await nodesStore.subscribeToUpdates(authStore.token)
  }
})

onUnmounted(() => {
  nodesStore.unsubscribeFromUpdates()
})

function formatTraffic(bytes) {
  if (!bytes) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let value = bytes
  while (value >= 1024 && i < units.length - 1) {
    value /= 1024
    i++
  }
  return `${value.toFixed(1)} ${units[i]}`
}

function viewNode(node) {
  // Navigate to node detail page
  console.log('View node:', node)
}

function editNode(node) {
  editingNode.value = node
  formData.value = {
    name: node.name,
    host: node.host,
    port: node.port || 22,
    username: node.username || 'root'
  }
}

async function confirmDelete(node) {
  if (confirm(`Are you sure you want to delete "${node.name}"?`)) {
    await nodesStore.deleteNode(node.id)
  }
}

async function saveNode() {
  let result
  if (editingNode.value) {
    result = await nodesStore.updateNode(editingNode.value.id, formData.value)
  } else {
    result = await nodesStore.createNode(formData.value)
  }

  if (result.success) {
    closeModal()
  }
}

function closeModal() {
  showCreateModal.value = false
  editingNode.value = null
  formData.value = {
    name: '',
    host: '',
    port: 22,
    username: 'root'
  }
}
</script>