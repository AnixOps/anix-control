<template>
  <aside class="fixed left-0 top-0 flex h-dvh w-64 flex-col border-r border-dark-700 bg-dark-800" :class="mobile ? 'z-50 md:hidden' : 'hidden md:flex'">
    <div class="flex shrink-0 items-start justify-between border-b border-dark-700 p-4">
      <div>
        <h1 class="text-xl font-bold text-primary-400">AnixOps</h1>
        <p class="text-xs text-dark-400">Control Center v1.0</p>
      </div>
      <button v-if="mobile" type="button" class="rounded-lg p-1 text-dark-300 hover:bg-dark-700 hover:text-white" title="Close navigation" aria-label="Close navigation" @click="emit('navigate')">
        <XMarkIcon class="h-5 w-5" />
      </button>
    </div>

    <nav class="min-h-0 flex-1 overflow-y-auto p-4">
      <ul class="space-y-1">
        <li v-for="item in menuItems" :key="item.path">
          <router-link
            :to="item.path"
            class="flex items-center gap-3 px-3 py-2 rounded-lg transition-colors"
            :class="[
              isActive(item.path)
                ? 'bg-primary-600 text-white'
                : 'text-dark-300 hover:bg-dark-700 hover:text-white'
            ]"
            @click="emit('navigate')"
          >
            <component :is="item.icon" class="w-5 h-5" />
            <span>{{ item.name }}</span>
          </router-link>
        </li>
      </ul>
    </nav>

    <div class="shrink-0 border-t border-dark-700 p-4">
      <div class="flex items-center gap-3">
        <div class="w-8 h-8 rounded-full bg-primary-600 flex items-center justify-center">
          <span class="text-sm font-medium">{{ userInitials }}</span>
        </div>
        <div class="flex-1 min-w-0">
          <p class="text-sm font-medium truncate">{{ authStore.user?.email }}</p>
          <p class="text-xs text-dark-400">{{ authStore.user?.role }}</p>
        </div>
        <button
          @click="logout"
          class="p-1 text-dark-400 hover:text-white"
          title="Logout"
        >
          <ArrowRightOnRectangleIcon class="w-5 h-5" />
        </button>
      </div>
    </div>
  </aside>
</template>

<script setup>
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import {
  HomeIcon,
  ServerIcon,
  ComputerDesktopIcon,
  UsersIcon,
  DocumentTextIcon,
  ClipboardDocumentListIcon,
  CalendarIcon,
  BellIcon,
  Cog6ToothIcon,
  PuzzlePieceIcon,
  ArrowRightOnRectangleIcon,
  XMarkIcon
} from '@heroicons/vue/24/outline'

defineProps({ mobile: { type: Boolean, default: false } })
const emit = defineEmits(['navigate'])
const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()

const menuItems = [
  { name: 'Dashboard', path: '/', icon: HomeIcon },
  { name: 'Nodes', path: '/nodes', icon: ServerIcon },
  { name: 'Agents', path: '/agents', icon: ComputerDesktopIcon },
  { name: 'Users', path: '/users', icon: UsersIcon },
  { name: 'Playbooks', path: '/playbooks', icon: DocumentTextIcon },
  { name: 'Tasks', path: '/tasks', icon: ClipboardDocumentListIcon },
  { name: 'Schedules', path: '/schedules', icon: CalendarIcon },
  { name: 'Logs', path: '/logs', icon: ClipboardDocumentListIcon },
  { name: 'Notifications', path: '/notifications', icon: BellIcon },
  { name: 'Kernel Plugins', path: '/plugins', icon: PuzzlePieceIcon },
  { name: 'Settings', path: '/settings', icon: Cog6ToothIcon }
]

const userInitials = computed(() => {
  const email = authStore.user?.email || ''
  return email.substring(0, 2).toUpperCase()
})

function isActive(path) {
  if (path === '/') {
    return route.path === '/'
  }
  return route.path.startsWith(path)
}

function logout() {
  authStore.logout()
  emit('navigate')
  router.push('/login')
}
</script>
