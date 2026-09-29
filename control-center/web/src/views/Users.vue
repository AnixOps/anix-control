<template>
  <div class="space-y-6">
    <h1 class="text-2xl font-bold text-white">Users</h1>

    <!-- Users Table -->
    <div class="bg-dark-800 rounded-xl border border-dark-700 overflow-hidden">
      <table class="w-full">
        <thead class="bg-dark-700">
          <tr>
            <th class="px-6 py-3 text-left text-xs font-medium text-dark-300 uppercase">Email</th>
            <th class="px-6 py-3 text-left text-xs font-medium text-dark-300 uppercase">Plan</th>
            <th class="px-6 py-3 text-left text-xs font-medium text-dark-300 uppercase">Status</th>
            <th class="px-6 py-3 text-left text-xs font-medium text-dark-300 uppercase">Used</th>
            <th class="px-6 py-3 text-right text-xs font-medium text-dark-300 uppercase">Actions</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-dark-700">
          <tr v-for="user in users" :key="user.id" class="hover:bg-dark-700/50">
            <td class="px-6 py-4 text-white">{{ user.email }}</td>
            <td class="px-6 py-4 text-dark-300">{{ user.plan }}</td>
            <td class="px-6 py-4">
              <span
                class="px-2 py-1 text-xs rounded-full"
                :class="user.status === 'active' ? 'bg-green-900/30 text-green-400' : 'bg-red-900/30 text-red-400'"
              >
                {{ user.status }}
              </span>
            </td>
            <td class="px-6 py-4 text-dark-300">{{ user.used }}</td>
            <td class="px-6 py-4 text-right">
              <button
                @click="toggleBan(user)"
                class="px-3 py-1 text-sm rounded-lg transition-colors"
                :class="user.status === 'active' ? 'bg-red-900/30 text-red-400 hover:bg-red-900/50' : 'bg-green-900/30 text-green-400 hover:bg-green-900/50'"
              >
                {{ user.status === 'active' ? 'Ban' : 'Unban' }}
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'

const users = ref([
  { id: 1, email: 'admin@example.com', plan: 'Pro', status: 'active', used: '12.4GB' },
  { id: 2, email: 'user1@example.com', plan: 'Basic', status: 'active', used: '5.2GB' },
  { id: 3, email: 'user2@example.com', plan: 'Pro', status: 'banned', used: '0B' },
  { id: 4, email: 'user3@example.com', plan: 'Enterprise', status: 'active', used: '45.6GB' }
])

function toggleBan(user) {
  user.status = user.status === 'active' ? 'banned' : 'active'
}
</script>