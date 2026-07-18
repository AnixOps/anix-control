<template>
  <nav
    class="forward-suite-nav"
    :class="{
      'forward-suite-nav-collapsed': collapsed,
      'forward-suite-nav-sidebar': sidebar,
    }"
    :aria-label="t('layout.admin.sections.forwardSuite')"
  >
    <ul class="forward-suite-list">
      <li v-for="link in coreLinks" :key="link.to" class="forward-suite-item">
        <router-link
          :to="link.to"
          class="forward-suite-link"
          active-class="forward-suite-link-active"
          :aria-label="linkDescription(link)"
          :title="linkDescription(link)"
          @click="onNavigate"
        >
          <AdminNavIcon :name="link.icon" />
          <span class="link-copy">
            <span class="label">{{ link.label }}</span>
            <span v-if="link.hint" class="hint">{{ link.hint }}</span>
          </span>
        </router-link>
      </li>
      <li class="forward-suite-item forward-suite-more">
        <details class="forward-suite-details" :open="advancedOpen">
          <summary
            class="forward-suite-link forward-suite-summary"
            :aria-label="t('forwardSuite.nav.more')"
            :title="t('forwardSuite.nav.more')"
          >
            <AdminNavIcon name="more" />
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
                :aria-label="linkDescription(link)"
                :title="linkDescription(link)"
                @click="onNavigate"
              >
                <AdminNavIcon :name="link.icon" />
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
import AdminNavIcon from '@/components/admin/AdminNavIcon.vue'

defineProps({
  collapsed: {
    type: Boolean,
    default: false
  },
  sidebar: {
    type: Boolean,
    default: false
  }
})

const emit = defineEmits(['navigate'])
const { t } = useAppI18n()
const injectedRoute = inject(routeLocationKey, null)
const instance = getCurrentInstance()

const coreLinks = computed(() => ([
  { label: t('forwardSuite.nav.setupWizard'), to: '/admin/forward/setup', icon: 'setup', hint: t('forwardSuite.hints.setupWizard') },
  { label: t('forwardSuite.nav.forwards'), to: '/admin/forward', icon: 'forward' },
  { label: t('forwardSuite.nav.tunnels'), to: '/admin/forward/tunnel', icon: 'tunnel' },
  { label: t('forwardSuite.nav.limits'), to: '/admin/forward/limit', icon: 'limits' },
  { label: t('forwardSuite.nav.nodeXTopology'), to: '/admin/forward/nodes', icon: 'nodes', hint: t('forwardSuite.hints.nodeXTopology') }
]))

const advancedLinks = computed(() => ([
  { label: t('forwardSuite.nav.ansibleMachines'), to: '/admin/forward/ansible-machines', icon: 'ansible', hint: t('forwardSuite.hints.ansibleMachines') },
  { label: t('forwardSuite.nav.localRuntime'), to: '/admin/forward/local', icon: 'local', hint: t('forwardSuite.hints.localRuntime') },
  { label: t('forwardSuite.nav.nodeXRuntime'), to: '/admin/forward/nodex', icon: 'nodex', hint: t('forwardSuite.hints.nodeXRuntime') },
  { label: t('forwardSuite.nav.nodeXAgents'), to: '/admin/forward/agents', icon: 'agents', hint: t('forwardSuite.hints.nodeXAgents') },
  { label: t('forwardSuite.nav.observability'), to: '/admin/forward/observability', icon: 'observability', hint: t('forwardSuite.hints.observability') }
]))

const advancedOpen = computed(() => {
  const path = injectedRoute?.path || instance?.proxy?.$route?.path || ''
  return advancedLinks.value.some(link => path === link.to || path.startsWith(`${link.to}/`))
})

function linkDescription(link) {
  return link.hint ? `${link.label}: ${link.hint}` : link.label
}

function onNavigate() {
  emit('navigate')
}
</script>

<style scoped>
.forward-suite-nav {
  width: 100%;
}

.forward-suite-list,
.forward-suite-sublist {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.forward-suite-item,
.forward-suite-subitem {
  margin: 0;
}

.forward-suite-link {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 40px;
  padding: 9px 10px;
  border-radius: var(--radius-md);
  color: var(--text-color);
  text-decoration: none;
  transition: var(--transition);
}

.forward-suite-link:hover,
.forward-suite-link-active,
.forward-suite-details[open] > .forward-suite-summary {
  background: var(--surface-hover);
  color: var(--text-color);
}

.forward-suite-summary {
  cursor: pointer;
  list-style: none;
}

.forward-suite-summary::-webkit-details-marker {
  display: none;
}

.link-copy {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 2px;
}

.hint {
  color: var(--text-secondary);
  font-size: 12px;
}

.forward-suite-sublist {
  margin: 4px 0 0 20px;
  padding-left: 8px;
  border-left: 1px solid var(--border-color);
}

.forward-suite-nav-sidebar .forward-suite-link {
  color: var(--admin-sidebar-text);
}

.forward-suite-nav-sidebar .forward-suite-link:hover,
.forward-suite-nav-sidebar .forward-suite-link-active,
.forward-suite-nav-sidebar .forward-suite-details[open] > .forward-suite-summary {
  background: var(--admin-sidebar-hover);
  color: var(--admin-sidebar-text-strong);
}

.forward-suite-nav-sidebar .hint {
  color: var(--admin-sidebar-muted);
}

.forward-suite-nav-sidebar .forward-suite-sublist {
  border-left-color: var(--admin-sidebar-divider);
}

.forward-suite-sub-link {
  min-height: 36px;
}

.forward-suite-nav-collapsed .link-copy {
  display: none;
}

.forward-suite-nav-collapsed .forward-suite-link {
  justify-content: center;
  width: 40px;
  padding: 9px;
}

.forward-suite-nav-collapsed .forward-suite-sublist {
  align-items: center;
  margin-left: 0;
  padding-left: 0;
  border-left: 0;
}
</style>
