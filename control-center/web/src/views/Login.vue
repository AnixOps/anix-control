<template>
  <div class="min-h-screen flex items-center justify-center bg-dark-900">
    <div class="w-full max-w-md">
      <div class="text-center mb-8">
        <h1 class="text-3xl font-bold text-white">AnixOps</h1>
        <p class="text-dark-400 mt-2">Control Center</p>
      </div>

      <div class="bg-dark-800 rounded-xl p-8 shadow-xl border border-dark-700">
        <form @submit.prevent="handleLogin">
          <div class="space-y-4">
            <div>
              <label class="block text-sm font-medium text-dark-300 mb-1">
                Email
              </label>
              <input
                v-model="form.email"
                type="email"
                required
                class="w-full px-4 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white placeholder-dark-400 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent"
                placeholder="admin@example.com"
              />
            </div>

            <div>
              <label class="block text-sm font-medium text-dark-300 mb-1">
                Password
              </label>
              <input
                v-model="form.password"
                type="password"
                required
                class="w-full px-4 py-2 bg-dark-700 border border-dark-600 rounded-lg text-white placeholder-dark-400 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent"
                placeholder="••••••••"
              />
            </div>
          </div>

          <div v-if="error" class="mt-4 p-3 bg-red-900/30 border border-red-800 rounded-lg">
            <p class="text-sm text-red-400">{{ error }}</p>
          </div>

          <button
            type="submit"
            :disabled="loading"
            class="w-full mt-6 px-4 py-2 bg-primary-600 hover:bg-primary-700 disabled:bg-primary-800 disabled:cursor-not-allowed text-white font-medium rounded-lg transition-colors"
          >
            <span v-if="loading">Signing in...</span>
            <span v-else>Sign In</span>
          </button>
        </form>
      </div>

      <p class="text-center text-dark-500 text-sm mt-6">
        Default: admin@example.com / admin123456
      </p>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const authStore = useAuthStore()

const form = ref({
  email: 'admin@example.com',
  password: 'admin123456'
})

const loading = ref(false)
const error = ref('')

async function handleLogin() {
  loading.value = true
  error.value = ''

  const result = await authStore.login(form.value.email, form.value.password)

  if (result.success) {
    router.push('/')
  } else {
    error.value = result.error
  }

  loading.value = false
}
</script>