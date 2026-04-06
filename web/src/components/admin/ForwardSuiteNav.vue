<template>
  <div class="forward-suite-links">
    <router-link
      v-for="link in links"
      :key="link.to"
      :to="link.to"
      class="forward-suite-link"
    >
      <span class="icon">{{ link.icon }}</span>
      <span class="label">{{ link.label }}</span>
    </router-link>
  </div>
</template>

<script setup>
const links = [
  { label: '流量转发', to: '/admin/forward', icon: '⚙️' },
  { label: '隧道管理', to: '/admin/forward/tunnel', icon: '🔌' },
  { label: '限速管理', to: '/admin/forward/limit', icon: '⏱️' },
  { label: '中转节点管理', to: '/admin/forward/nodes', icon: '🛰️' }
]
</script>

<style scoped>
.forward-suite-links {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.forward-suite-link {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 14px;
  border-radius: 12px;
  color: var(--text-secondary);
  text-decoration: none;
}

.forward-suite-link:hover {
  background: var(--bg-color);
  color: var(--text-color);
}

.forward-suite-link.router-link-active {
  background: var(--primary-color);
  color: white;
}

.icon {
  width: 20px;
  text-align: center;
}
</style>
