<template>
  <div class="forward-suite-links">
    <router-link
      v-for="link in links"
      :key="link.to"
      :to="link.to"
      class="forward-suite-link"
    >
      <span class="icon">{{ link.icon }}</span>
      <span class="link-copy">
        <span class="label">{{ link.label }}</span>
        <span v-if="link.hint" class="hint">{{ link.hint }}</span>
      </span>
    </router-link>
  </div>
</template>

<script setup>
const links = [
  { label: 'Forwards', to: '/admin/forward', icon: 'F' },
  { label: 'Tunnels', to: '/admin/forward/tunnel', icon: 'T' },
  { label: 'Limits', to: '/admin/forward/limit', icon: 'L' },
  { label: 'Ansible Machines', to: '/admin/forward/ansible-machines', icon: 'A', hint: 'Stateless execution machines' },
  { label: 'Local Runtime', to: '/admin/forward/local', icon: 'R', hint: 'Stateless panel-host executor' },
  { label: 'NodeX Topology', to: '/admin/forward/nodes', icon: 'N', hint: 'Stateful relay/exit topology' },
  { label: 'NodeX Runtime', to: '/admin/forward/nodex', icon: 'X', hint: 'Stateful gost control-plane' },
  { label: 'NodeX Agents', to: '/admin/forward/agents', icon: 'G', hint: 'Stateful agent task channel' }
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
  font-weight: 700;
}

.link-copy {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.hint {
  font-size: 12px;
  opacity: 0.8;
}
</style>
