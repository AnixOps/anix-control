<template>
  <nav
    class="admin-navigation"
    :class="{
      'admin-navigation-collapsed': collapsed && !mobile,
      'admin-navigation-mobile': mobile,
    }"
    :aria-label="t('layout.admin.mobileTitle')"
  >
    <section
      v-for="section in sections"
      :key="section.id"
      class="nav-section"
      :class="{ 'nav-section-forward': section.kind === 'forward' }"
      :data-nav-group="section.id"
    >
      <div class="nav-section-title">{{ section.label }}</div>

      <router-link
        v-for="item in section.items"
        :key="item.to"
        :to="item.to"
        class="nav-link"
        active-class="nav-link-active"
        :aria-label="itemDescription(item)"
        :title="itemDescription(item)"
        @click="onNavigate"
      >
        <AdminNavIcon :name="item.icon" />
        <span class="nav-link-label">{{ item.label }}</span>
      </router-link>

      <template v-if="section.kind === 'forward'">
        <div class="nav-subsection-title">{{ section.forwardLabel }}</div>
        <ForwardSuiteNav sidebar :collapsed="collapsed && !mobile" @navigate="onNavigate" />
      </template>

      <details
        v-if="section.advancedItems?.length"
        class="nav-details"
        :open="containsCurrentRoute(section.advancedItems)"
      >
        <summary
          class="nav-disclosure"
          :aria-label="t('layout.admin.sections.more')"
          :title="t('layout.admin.sections.more')"
        >
          <AdminNavIcon name="more" />
          <span class="nav-disclosure-label">{{ t('layout.admin.sections.more') }}</span>
          <ChevronDown class="nav-disclosure-chevron" :size="16" aria-hidden="true" />
        </summary>
        <div class="nav-advanced-list">
          <router-link
            v-for="item in section.advancedItems"
            :key="item.to"
            :to="item.to"
            class="nav-link nav-link-advanced"
            active-class="nav-link-active"
            :aria-label="itemDescription(item)"
            :title="itemDescription(item)"
            @click="onNavigate"
          >
            <AdminNavIcon :name="item.icon" />
            <span class="nav-link-label">{{ item.label }}</span>
          </router-link>
        </div>
      </details>

      <div v-if="section.extensionGroups?.length" class="extension-menu-groups">
        <section
          v-for="group in section.extensionGroups"
          :key="group.parent"
          class="extension-menu-group"
          :data-extension-parent="group.parent"
        >
          <div class="extension-menu-heading">{{ group.label }}</div>
          <router-link
            v-for="item in group.items"
            :key="item.to"
            :to="item.to"
            class="nav-link nav-link-extension"
            active-class="nav-link-active"
            :aria-label="itemDescription(item)"
            :title="itemDescription(item)"
            @click="onNavigate"
          >
            <AdminNavIcon :name="item.icon" />
            <span class="nav-link-label">{{ item.label }}</span>
          </router-link>
        </section>
      </div>
    </section>

    <button
      class="navigation-collapse-control"
      type="button"
      :aria-label="collapseLabel"
      :title="collapseLabel"
      @click="emit('toggle-collapse')"
    >
      <PanelLeftOpen v-if="collapsed" :size="18" aria-hidden="true" />
      <PanelLeftClose v-else :size="18" aria-hidden="true" />
    </button>
  </nav>
</template>

<script setup>
import { computed } from 'vue'
import { ChevronDown, PanelLeftClose, PanelLeftOpen } from '@lucide/vue'
import { useRoute } from 'vue-router'
import { useAppI18n } from '@/composables/useAppI18n'
import AdminNavIcon from '@/components/admin/AdminNavIcon.vue'
import ForwardSuiteNav from '@/components/admin/ForwardSuiteNav.vue'

const props = defineProps({
  sections: {
    type: Array,
    default: () => []
  },
  collapsed: {
    type: Boolean,
    default: false
  },
  mobile: {
    type: Boolean,
    default: false
  }
})

const emit = defineEmits(['navigate', 'toggle-collapse'])
const route = useRoute()
const { t } = useAppI18n()

const collapseLabel = computed(() => (
  props.collapsed
    ? t('layout.admin.expandNavigation')
    : t('layout.admin.collapseNavigation')
))

function itemDescription(item) {
  return item.hint ? `${item.label}: ${item.hint}` : item.label
}

function containsCurrentRoute(items) {
  return items.some(item => route.path === item.to || route.path.startsWith(`${item.to}/`))
}

function onNavigate() {
  emit('navigate')
}
</script>

<style scoped>
.admin-navigation {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 18px;
  min-height: 0;
  overflow-y: auto;
  padding-right: 4px;
}

.nav-section {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.nav-section-title,
.nav-subsection-title,
.extension-menu-heading {
  padding: 0 8px;
  color: var(--admin-sidebar-muted);
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0;
  text-transform: uppercase;
}

.nav-section-title {
  margin-bottom: 4px;
}

.nav-subsection-title {
  margin: 12px 0 4px;
}

.nav-link,
.nav-disclosure {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 40px;
  padding: 9px 10px;
  border-radius: var(--radius-md);
  color: var(--admin-sidebar-text);
  text-decoration: none;
  transition: var(--transition);
}

.nav-link:hover,
.nav-link-active,
.nav-disclosure:hover,
.nav-details[open] > .nav-disclosure {
  background: var(--admin-sidebar-hover);
  color: var(--admin-sidebar-text-strong);
}

.nav-link :deep(.admin-nav-icon),
.nav-disclosure :deep(.admin-nav-icon) {
  flex: 0 0 auto;
}

.nav-link-label,
.nav-disclosure-label {
  min-width: 0;
}

.nav-details {
  margin: 0;
}

.nav-disclosure {
  cursor: pointer;
  list-style: none;
}

.nav-disclosure::-webkit-details-marker {
  display: none;
}

.nav-disclosure-chevron {
  flex: 0 0 auto;
  margin-left: auto;
  transition: transform 0.2s ease;
}

.nav-details[open] .nav-disclosure-chevron {
  transform: rotate(180deg);
}

.nav-advanced-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin: 4px 0 0 20px;
  padding-left: 8px;
  border-left: 1px solid var(--admin-sidebar-divider);
}

.nav-link-advanced {
  min-height: 36px;
}

.extension-menu-groups {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin-top: 10px;
  padding-top: 10px;
  border-top: 1px solid var(--admin-sidebar-divider);
}

.extension-menu-group {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.extension-menu-heading {
  padding-top: 2px;
}

.navigation-collapse-control {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  min-height: 40px;
  margin-top: auto;
  border: 1px solid var(--admin-sidebar-divider);
  border-radius: var(--radius-md);
  background: transparent;
  color: var(--admin-sidebar-muted);
  cursor: pointer;
}

.navigation-collapse-control:hover {
  background: var(--admin-sidebar-hover);
  color: var(--admin-sidebar-text-strong);
}

.admin-navigation-collapsed {
  align-items: center;
  overflow-x: hidden;
  padding-right: 0;
}

.admin-navigation-collapsed .nav-section {
  width: 100%;
  align-items: center;
}

.admin-navigation-collapsed .nav-section-title,
.admin-navigation-collapsed .nav-subsection-title,
.admin-navigation-collapsed .extension-menu-heading,
.admin-navigation-collapsed .nav-link-label,
.admin-navigation-collapsed .nav-disclosure-label,
.admin-navigation-collapsed .nav-disclosure-chevron,
.admin-navigation-collapsed :deep(.forward-suite-link .link-copy) {
  display: none;
}

.admin-navigation-collapsed .nav-link,
.admin-navigation-collapsed .nav-disclosure,
.admin-navigation-collapsed :deep(.forward-suite-link) {
  justify-content: center;
  width: 40px;
  padding: 9px;
}

.admin-navigation-collapsed .nav-advanced-list {
  align-items: center;
  margin-left: 0;
  padding-left: 0;
  border-left: 0;
}

.admin-navigation-collapsed .extension-menu-groups {
  width: 100%;
  align-items: center;
}

.admin-navigation-collapsed .extension-menu-group {
  align-items: center;
}

@media (max-width: 1024px) {
  .navigation-collapse-control {
    display: none;
  }
}
</style>
