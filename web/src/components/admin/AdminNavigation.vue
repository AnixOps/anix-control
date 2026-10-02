<template>
  <nav
    ref="navRef"
    class="admin-nav"
    :class="{ 'is-rail': rail }"
    :aria-label="t('shell.admin.navLabel')"
    @keydown="onKeydown"
  >
    <div
      v-for="group in groups"
      :key="group.id"
      class="admin-nav__group"
      :data-nav-group="group.id"
    >
      <div :id="headingId(group.id)" class="admin-nav__heading" :class="{ 'visually-hidden': rail }">{{ group.label }}</div>
      <ul class="admin-nav__list" :aria-labelledby="headingId(group.id)">
        <li v-for="item in group.items" :key="item.id">
          <router-link
            :to="item.to"
            class="admin-nav__link"
            :class="{ 'is-active': item.id === activeId }"
            active-class=""
            exact-active-class=""
            :aria-current="item.id === activeId ? 'page' : undefined"
            :title="rail ? item.label : undefined"
            :data-nav-item="item.id"
            :data-nav-source="item.source"
            @click="emit('navigate')"
          >
            <AdminNavIcon :name="item.icon" class="admin-nav__icon" />
            <span class="admin-nav__label" :class="{ 'visually-hidden': rail }">{{ item.label }}</span>
          </router-link>
        </li>
      </ul>
    </div>
  </nav>
</template>

<script setup>
// The admin sidebar's link list. It renders the groups from
// navigation/menu.js as they come (built-in items, edition, permissions and
// plugin menus are merged there) and marks the one active item.
// The selected item is a rounded fill with an accent icon; hover is a lighter
// fill. As an icon rail the labels stay in the accessibility tree (visually
// hidden) and show as tooltips. Every link is in the Tab order; ↑/↓, Home
// and End also move between links.
import { ref } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import AdminNavIcon from '@/components/admin/AdminNavIcon.vue'

defineProps({
  groups: { type: Array, default: () => [] },
  activeId: { type: String, default: '' },
  rail: { type: Boolean, default: false }
})

const emit = defineEmits(['navigate'])
const { t } = useAppI18n()
const navRef = ref(null)

function headingId(groupId) {
  return `admin-nav-group-${groupId}`
}

function onKeydown(event) {
  if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
  const links = [...(navRef.value?.querySelectorAll('.admin-nav__link') || [])]
  if (links.length === 0) return
  const index = links.indexOf(document.activeElement)
  if (index < 0) return
  event.preventDefault()
  let next = index
  if (event.key === 'ArrowDown') next = (index + 1) % links.length
  if (event.key === 'ArrowUp') next = (index - 1 + links.length) % links.length
  if (event.key === 'Home') next = 0
  if (event.key === 'End') next = links.length - 1
  links[next].focus()
}
</script>

<style scoped>
.admin-nav {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.admin-nav__group {
  display: flex;
  flex-direction: column;
}

.admin-nav__heading {
  padding: var(--space-2) var(--space-3) var(--space-1);
  color: var(--label-2);
  font-size: var(--type-caption-size);
  font-weight: var(--weight-semibold);
  line-height: var(--type-caption-line);
}

.admin-nav__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-0-5);
  list-style: none;
}

.admin-nav__link {
  display: flex;
  gap: var(--space-3);
  align-items: center;
  min-height: 32px;
  padding: var(--space-1) var(--space-3);
  border-radius: var(--radius-sm);
  color: var(--label-1);
  font-size: var(--type-body-size);
  line-height: var(--type-callout-line);
  text-decoration: none;
  transition: background-color var(--dur-micro) var(--ease-standard);
}

.admin-nav__icon {
  flex: none;
  color: var(--label-2);
}

.admin-nav__label {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.admin-nav__link:hover {
  background: var(--fill-1);
}

/* Selected: a stronger fill, medium weight and the accent icon. */
.admin-nav__link.is-active {
  background: var(--fill-2);
  font-weight: var(--weight-medium);
}

.admin-nav__link.is-active .admin-nav__icon {
  color: var(--accent);
}

.admin-nav__link:focus-visible {
  outline: var(--focus-ring);
  outline-offset: calc(var(--focus-ring-offset) * -1);
}

/* Icon rail: one centred icon per row, groups separated by a hairline. */
.admin-nav.is-rail {
  gap: var(--space-2);
}

.admin-nav.is-rail .admin-nav__group + .admin-nav__group {
  padding-top: var(--space-2);
  border-top: 1px solid var(--separator);
}

.admin-nav.is-rail .admin-nav__link {
  justify-content: center;
  width: 40px;
  min-height: 40px;
  margin-inline: auto;
  padding: 0;
}

@media (pointer: coarse) {
  .admin-nav__link {
    min-height: 44px;
  }
}

@media (forced-colors: active) {
  .admin-nav__link.is-active {
    outline: 2px solid transparent;
    text-decoration: underline;
  }
}
</style>
