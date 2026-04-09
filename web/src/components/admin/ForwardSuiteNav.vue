<template>
  <div class="forward-suite-nav">
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
import { computed } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'

const { t } = useAppI18n()

const links = computed(() => ([
  { label: t('forwardSuite.nav.forwards'), to: '/admin/forward', icon: 'F' },
  { label: t('forwardSuite.nav.tunnels'), to: '/admin/forward/tunnel', icon: 'T' },
  { label: t('forwardSuite.nav.limits'), to: '/admin/forward/limit', icon: 'L' },
  { label: t('forwardSuite.nav.ansibleMachines'), to: '/admin/forward/ansible-machines', icon: 'A', hint: t('forwardSuite.hints.ansibleMachines') },
  { label: t('forwardSuite.nav.localRuntime'), to: '/admin/forward/local', icon: 'R', hint: t('forwardSuite.hints.localRuntime') },
  { label: t('forwardSuite.nav.nodeXTopology'), to: '/admin/forward/nodes', icon: 'N', hint: t('forwardSuite.hints.nodeXTopology') },
  { label: t('forwardSuite.nav.nodeXRuntime'), to: '/admin/forward/nodex', icon: 'X', hint: t('forwardSuite.hints.nodeXRuntime') },
  { label: t('forwardSuite.nav.nodeXAgents'), to: '/admin/forward/agents', icon: 'G', hint: t('forwardSuite.hints.nodeXAgents') }
]))
</script>

<style scoped>
.forward-suite-nav {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.forward-suite-link {
  display: flex;
  align-items: center;
  gap: 10px;
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
