<template>
  <div class="min-h-screen bg-dark-900 text-white">
    <div v-if="authStore.isAuthenticated">
      <Sidebar />
      <button
        v-if="mobileMenuOpen"
        type="button"
        class="fixed inset-0 z-40 bg-black/60 md:hidden"
        aria-label="Close navigation"
        @click="mobileMenuOpen = false"
      />
      <Sidebar v-if="mobileMenuOpen" mobile @navigate="mobileMenuOpen = false" />
      <header class="sticky top-0 z-30 flex h-14 items-center justify-between border-b border-dark-700 bg-dark-800 px-4 md:hidden">
        <span class="font-semibold text-primary-400">AnixOps</span>
        <button
          type="button"
          class="rounded-lg p-2 text-dark-200 hover:bg-dark-700"
          title="Open navigation"
          aria-label="Open navigation"
          :aria-expanded="mobileMenuOpen"
          @click="mobileMenuOpen = true"
        >
          <Bars3Icon class="h-5 w-5" />
        </button>
      </header>
      <main class="min-w-0 p-4 md:ml-64 md:p-6">
        <router-view />
      </main>
    </div>
    <div v-else>
      <router-view />
    </div>
  </div>
</template>

<script setup>
import { ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { Bars3Icon } from '@heroicons/vue/24/outline'
import { useAuthStore } from '@/stores/auth'
import Sidebar from '@/components/Sidebar.vue'

const authStore = useAuthStore()
const route = useRoute()
const mobileMenuOpen = ref(false)

watch(() => route.fullPath, () => { mobileMenuOpen.value = false })
</script>
