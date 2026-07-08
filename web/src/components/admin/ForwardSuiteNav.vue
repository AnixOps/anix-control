<template>
  <nav class="forward-suite-nav" :aria-label="t('layout.admin.sections.forwardSuite')">
    <ul class="forward-suite-list">
      <li v-for="link in coreLinks" :key="link.to" class="forward-suite-item">
        <router-link
          :to="link.to"
          class="forward-suite-link"
          active-class="forward-suite-link-active"
          :aria-label="link.hint ? `${link.label}: ${link.hint}` : link.label"
        >
          <span class="icon" aria-hidden="true">{{ link.icon }}</span>
          <span class="link-copy">
            <span class="label">{{ link.label }}</span>
            <span v-if="link.hint" class="hint">{{ link.hint }}</span>
          </span>
        </router-link>
      </li>
      <li class="forward-suite-item forward-suite-more">
        <details class="forward-suite-details" :open="advancedOpen">
          <summary class="forward-suite-link forward-suite-summary">
            <span class="icon" aria-hidden="true">··</span>
            <span class="link-copy">
              <span class="label">{{ t('forwardSuite.nav.more') }}</span>
              <span class="hint">{{ t('forwardSuite.hints.more') }}</span>
            </span>
          </summary>
          <ul class="forward-suite-sublist">
            <li v-for="link in advancedLinks" :key="link.to" class="forward-suite-subitem">
              <router-link
                :to="link.to"
                class="forward-suite-link forward-suite-sub-link"
                active-class="forward-suite-link-active"
                :aria-label="link.hint ? `${link.label}: ${link.hint}` : link.label"
              >
                <span class="icon" aria-hidden="true">{{ link.icon }}</span>
                <span class="link-copy">
                  <span class="label">{{ link.label }}</span>
                  <span v-if="link.hint" class="hint">{{ link.hint }}</span>
                </span>
              </router-link>
            </li>
          </ul>
        </details>
      </li>
    </ul>
  </nav>
</template>

<script setup>
import { computed, getCurrentInstance, inject } from 'vue'
import { routeLocationKey } from 'vue-router'
import { useAppI18n } from '@/composables/useAppI18n'

const { t } = useAppI18n()
const injectedRoute = inject(routeLocationKey, null)
const instance = getCurrentInstance()

const coreLinks = computed(() => ([
  { label: t('forwardSuite.nav.setupWizard'), to: '/admin/forward/setup', icon: 'WZ', hint: t('forwardSuite.hints.setupWizard') },
  { label: t('forwardSuite.nav.forwards'), to: '/admin/forward', icon: 'FW' },
  { label: t('forwardSuite.nav.tunnels'), to: '/admin/forward/tunnel', icon: 'TN' },
  { label: t('forwardSuite.nav.limits'), to: '/admin/forward/limit', icon: 'LM' },
  { label: t('forwardSuite.nav.nodeXTopology'), to: '/admin/forward/nodes', icon: 'NX', hint: t('forwardSuite.hints.nodeXTopology') }
]))

const advancedLinks = computed(() => ([
  { label: t('forwardSuite.nav.ansibleMachines'), to: '/admin/forward/ansible-machines', icon: 'AM', hint: t('forwardSuite.hints.ansibleMachines') },
  { label: t('forwardSuite.nav.localRuntime'), to: '/admin/forward/local', icon: 'LR', hint: t('forwardSuite.hints.localRuntime') },
  { label: t('forwardSuite.nav.nodeXRuntime'), to: '/admin/forward/nodex', icon: 'RT', hint: t('forwardSuite.hints.nodeXRuntime') },
  { label: t('forwardSuite.nav.nodeXAgents'), to: '/admin/forward/agents', icon: 'AG', hint: t('forwardSuite.hints.nodeXAgents') },
  { label: t('forwardSuite.nav.observability'), to: '/admin/forward/observability', icon: 'OB', hint: t('forwardSuite.hints.observability') }
]))

const advancedOpen = computed(() => {
  const path = injectedRoute?.path || instance?.proxy?.$route?.path || ''
  return advancedLinks.value.some(link => path === link.to || path.startsWith(`${link.to}/`))
})
</script>

<style scoped>
.forward-suite-nav {
  width: 100%;
}

.forward-suite-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.forward-suite-item {
  margin: 0;
}

.forward-suite-link {
  display: flex;
  align-items: center;
  gap: 10px;
}

.forward-suite-summary {
  cursor: pointer;
  list-style: none;
}

.forward-suite-summary::-webkit-details-marker {
  display: none;
}

.forward-suite-link-active {
  font-weight: 700;
}

.icon {
  width: 28px;
  height: 28px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.08);
  font-size: 10px;
  font-weight: 700;
}

.link-copy {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.hint {
  font-size: 12px;
  opacity: 0.76;
}

.forward-suite-sublist {
  list-style: none;
  margin: 6px 0 0;
  padding: 0 0 0 14px;
  display: flex;
  flex-direction: column;
  gap: 6px;
  border-left: 1px solid rgba(255, 255, 255, 0.12);
}

.forward-suite-subitem {
  margin: 0;
}

.forward-suite-sub-link {
  padding-left: 0;
}
</style>
