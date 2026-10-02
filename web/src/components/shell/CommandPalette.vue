<template>
  <DialogRoot :open="open" @update:open="setOpen">
    <DialogPortal>
      <DialogOverlay class="palette-overlay">
        <DialogContent
          class="palette"
          aria-modal="true"
          data-command-palette
          @open-auto-focus="onOpenAutoFocus"
        >
          <DialogTitle class="visually-hidden">{{ t('shell.palette.title') }}</DialogTitle>
          <DialogDescription :id="descriptionId" class="visually-hidden">{{ t('shell.palette.description') }}</DialogDescription>

          <ListboxRoot ref="listboxRef" class="palette__root" highlight-on-hover>
            <div class="palette__search">
              <UiIcon :icon="Search" :size="20" class="palette__search-icon" />
              <ListboxFilter
                ref="filterRef"
                v-model="query"
                class="palette__input"
                role="combobox"
                aria-expanded="true"
                aria-autocomplete="list"
                :aria-controls="listId"
                :aria-label="t('shell.palette.title')"
                :aria-describedby="descriptionId"
                :placeholder="t('shell.palette.placeholder')"
                autocomplete="off"
                autocapitalize="off"
                spellcheck="false"
                enterkeyhint="go"
                data-palette-input
              />
            </div>

            <ListboxContent :id="listId" class="palette__list" :class="{ 'is-empty': resultCount === 0 }" :aria-label="t('shell.palette.title')">
              <ListboxGroup v-for="section in sections" :key="section.id" class="palette__group" :data-palette-group="section.id">
                <ListboxGroupLabel class="palette__group-label">{{ section.label }}</ListboxGroupLabel>
                <ListboxItem
                  v-for="entry in section.entries"
                  :key="entry.key"
                  :value="entry.key"
                  :text-value="entry.label"
                  class="palette__item"
                  :data-palette-entry="entry.key"
                  @select="run(entry)"
                >
                  <span class="palette__item-icon"><UiIcon :icon="entry.iconComponent" /></span>
                  <span class="palette__item-label">{{ entry.label }}</span>
                  <span v-if="entry.context" class="palette__item-context">{{ entry.context }}</span>
                </ListboxItem>
              </ListboxGroup>
            </ListboxContent>

            <p class="palette__status" :class="{ 'is-inline': resultCount > 0, 'is-silent': !statusText }" role="status" data-palette-status>{{ statusText }}</p>

            <footer class="palette__footer" aria-hidden="true">
              <span><kbd>↑</kbd><kbd>↓</kbd>{{ t('shell.palette.hints.move') }}</span>
              <span><kbd>↵</kbd>{{ t('shell.palette.hints.open') }}</span>
              <span><kbd>esc</kbd>{{ t('shell.palette.hints.close') }}</span>
            </footer>
          </ListboxRoot>
        </DialogContent>
      </DialogOverlay>
    </DialogPortal>
  </DialogRoot>
</template>

<script setup>
// Command palette (⌘K / Ctrl+K, plan §4.2 and §9). Reka Dialog (focus
// trap, Esc, focus returns to the opener, page scroll locked) around a
// Reka Listbox driven from its filter input, which carries the combobox
// role: type to filter, ↑/↓ move, Enter opens, Esc closes.
//
// It lists what the admin can see: the pages of the menu config
// (navigation/menu.js, already filtered by edition, permissions and
// plugins), the quick actions that open existing create flows, appearance
// and language switches, and, from two characters on, users whose email
// matches (GET /admin/users?email=, the list endpoint's own filter). Nodes
// and forward rules have no list endpoint with a search parameter, so they
// are not searched.
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import {
  DialogContent, DialogDescription, DialogOverlay, DialogPortal, DialogRoot, DialogTitle,
  ListboxContent, ListboxFilter, ListboxGroup, ListboxGroupLabel, ListboxItem, ListboxRoot, useId
} from 'reka-ui'
import { Languages, Monitor, Moon, Search, Sun, UserRound } from '@lucide/vue'
import { getUserList } from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import { useTheme } from '@/composables/useTheme'
import { matchesQuery, paletteEntries } from '@/navigation/menu'
import { navIcon } from '@/navigation/icons'
import UiIcon from '@/ui/UiIcon.vue'
import { usePalette } from './usePalette'

const props = defineProps({
  // Menu groups from useAdminMenu().
  groups: { type: Array, default: () => [] }
})

const USER_SEARCH_MIN = 2
const USER_SEARCH_DELAY = 250
const USER_SEARCH_LIMIT = 5

const router = useRouter()
const { t, currentLocale, localeOptions, switchLocale } = useAppI18n()
const { themePreference, setThemePreference } = useTheme()
const { open } = usePalette()

const listId = useId(undefined, 'palette-list')
const descriptionId = useId(undefined, 'palette-description')
const query = ref('')
const listboxRef = ref(null)
const filterRef = ref(null)
const userResults = ref([])
const userSearch = ref('idle') // idle | loading | failed

function setOpen(value) {
  open.value = value
}

function onOpenAutoFocus(event) {
  // Focus the input ourselves (ListboxFilter's autoFocus runs on a timer).
  event.preventDefault()
  const input = filterRef.value?.$el
  input?.focus?.()
}

watch(open, value => {
  if (value) {
    query.value = ''
    userResults.value = []
    userSearch.value = 'idle'
    nextTick(() => listboxRef.value?.highlightFirstItem?.())
  }
})

const base = computed(() => paletteEntries({ t, groups: props.groups }))

const settingEntries = computed(() => {
  const entries = [
    { id: 'theme-light', label: t('shell.palette.actions.themeLight'), iconComponent: Sun, run: () => setThemePreference('light'), when: themePreference.value !== 'light' },
    { id: 'theme-dark', label: t('shell.palette.actions.themeDark'), iconComponent: Moon, run: () => setThemePreference('dark'), when: themePreference.value !== 'dark' },
    { id: 'theme-system', label: t('shell.palette.actions.themeSystem'), iconComponent: Monitor, run: () => setThemePreference('system'), when: themePreference.value !== 'system' }
  ]
  for (const option of localeOptions.value) {
    if (option.value === currentLocale.value) continue
    entries.push({
      id: `locale-${option.value}`,
      label: t('shell.palette.actions.language', { language: option.nativeLabel }),
      iconComponent: Languages,
      run: () => switchLocale(option.value),
      when: true,
      keywords: 'language locale 语言'
    })
  }
  return entries
    .filter(entry => entry.when)
    .map(entry => ({ ...entry, kind: 'setting', keywords: entry.keywords || 'appearance theme 外观 主题' }))
})

function decorate(entry) {
  return {
    ...entry,
    key: `${entry.kind}:${entry.id}`,
    iconComponent: entry.iconComponent || navIcon(entry.icon)
  }
}

const sections = computed(() => {
  const q = query.value
  const actions = [...base.value.actions, ...settingEntries.value].filter(entry => matchesQuery(entry, q)).map(decorate)
  const pages = base.value.pages.filter(entry => matchesQuery(entry, q)).map(decorate)
  const users = userResults.value.map(decorate)
  const out = []
  // While typing, pages come first: jumping somewhere is the common case.
  if (q.trim()) {
    if (pages.length) out.push({ id: 'pages', label: t('shell.palette.groups.pages'), entries: pages })
    if (actions.length) out.push({ id: 'actions', label: t('shell.palette.groups.actions'), entries: actions })
  } else {
    if (actions.length) out.push({ id: 'actions', label: t('shell.palette.groups.actions'), entries: actions })
    if (pages.length) out.push({ id: 'pages', label: t('shell.palette.groups.pages'), entries: pages })
  }
  if (users.length) out.push({ id: 'users', label: t('shell.palette.groups.users'), entries: users })
  return out
})

const resultCount = computed(() => sections.value.reduce((sum, section) => sum + section.entries.length, 0))

const statusText = computed(() => {
  if (userSearch.value === 'loading') return t('shell.palette.searching')
  if (resultCount.value === 0) {
    return userSearch.value === 'failed' ? t('shell.palette.searchFailed') : t('shell.palette.empty')
  }
  return ''
})

// --- user search -------------------------------------------------------
let searchTimer = null
let searchSeq = 0

function readUserList(body) {
  const payload = body && typeof body.code === 'number' ? (body.code === 0 ? body.data : null) : (body?.data ?? body)
  return Array.isArray(payload?.list) ? payload.list : []
}

watch(query, value => {
  clearTimeout(searchTimer)
  const q = value.trim()
  searchSeq += 1
  if (q.length < USER_SEARCH_MIN) {
    userResults.value = []
    userSearch.value = 'idle'
    return
  }
  const seq = searchSeq
  searchTimer = setTimeout(async () => {
    userSearch.value = 'loading'
    try {
      const list = readUserList(await getUserList({ email: q, page: 1, page_size: USER_SEARCH_LIMIT }))
      if (seq !== searchSeq) return
      userResults.value = list
        .filter(user => user && typeof user.email === 'string')
        .slice(0, USER_SEARCH_LIMIT)
        .map(user => ({
          id: String(user.id ?? user.email),
          kind: 'user',
          label: user.email,
          context: t('shell.palette.userResult'),
          iconComponent: UserRound,
          to: { path: '/admin/users', query: { email: user.email } }
        }))
      userSearch.value = 'idle'
    } catch {
      if (seq !== searchSeq) return
      userResults.value = []
      userSearch.value = 'failed'
    }
  }, USER_SEARCH_DELAY)
})

onBeforeUnmount(() => clearTimeout(searchTimer))

async function run(entry) {
  open.value = false
  if (typeof entry.run === 'function') {
    await entry.run()
    return
  }
  if (entry.to) {
    await router.push(entry.to)
  }
}
</script>

<style scoped>
.palette-overlay {
  position: fixed;
  inset: 0;
  z-index: var(--z-modal);
  display: flex;
  justify-content: center;
  padding: max(12vh, var(--space-4)) var(--space-4) var(--space-4);
  background: var(--scrim);
  animation: palette-fade var(--dur-overlay) var(--ease-standard);
}

.palette {
  display: flex;
  flex-direction: column;
  width: min(640px, 100%);
  max-height: min(520px, 100%);
  overflow: hidden;
  border-radius: var(--radius-lg);
  background: var(--bg-elevated);
  box-shadow: var(--shadow-3), 0 0 0 1px var(--separator);
  color: var(--label-1);
  animation: palette-in var(--dur-overlay) var(--ease-emphasized);
}

.palette:focus {
  outline: none;
}

.palette__root {
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.palette__search {
  display: flex;
  gap: var(--space-3);
  align-items: center;
  padding: 0 var(--space-5);
  border-bottom: 1px solid var(--separator);
}

.palette__search-icon {
  color: var(--label-3);
}

.palette__input {
  flex: 1;
  min-width: 0;
  height: 56px;
  border: 0;
  background: transparent;
  color: var(--label-1);
  font-size: var(--type-title-3-size);
  outline: none;
}

.palette__input::placeholder {
  color: var(--label-3);
}

.palette__list {
  min-height: 0;
  padding: var(--space-2);
  overflow-y: auto;
  overscroll-behavior: contain;
}

.palette__list.is-empty {
  display: none;
}

.palette__group + .palette__group {
  margin-top: var(--space-2);
}

.palette__group-label {
  padding: var(--space-2) var(--space-3) var(--space-1);
  color: var(--label-2);
  font-size: var(--type-caption-size);
  font-weight: var(--weight-semibold);
}

.palette__item {
  display: flex;
  gap: var(--space-3);
  align-items: center;
  min-height: 40px;
  padding: 0 var(--space-3);
  border-radius: var(--radius-sm);
  outline: none;
  cursor: default;
  user-select: none;
}

.palette__item-icon {
  display: inline-grid;
  flex: none;
  place-items: center;
  width: 28px;
  height: 28px;
  border-radius: calc(var(--radius-sm) - 2px);
  background: var(--fill-1);
  color: var(--label-2);
}

.palette__item-label {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.palette__item-context {
  flex: none;
  max-width: 45%;
  overflow: hidden;
  color: var(--label-2);
  font-size: var(--type-callout-size);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.palette__item[data-highlighted] {
  background: var(--accent-fill);
  color: var(--on-accent);
}

.palette__item[data-highlighted] .palette__item-icon {
  background: transparent;
  color: var(--on-accent);
}

.palette__item[data-highlighted] .palette__item-context {
  color: var(--on-accent);
}

.palette__status {
  padding: var(--space-8) var(--space-4);
  color: var(--label-2);
  font-size: var(--type-callout-size);
  text-align: center;
}

.palette__status.is-inline {
  padding: var(--space-2) var(--space-5) var(--space-3);
  text-align: left;
}

.palette__status.is-silent {
  height: 0;
  padding: 0;
  overflow: hidden;
}

.palette__footer {
  display: flex;
  gap: var(--space-5);
  padding: var(--space-2) var(--space-5);
  border-top: 1px solid var(--separator);
  background: var(--bg-grouped);
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

.palette__footer span {
  display: inline-flex;
  gap: var(--space-1);
  align-items: center;
}

.palette__footer kbd {
  display: inline-grid;
  place-items: center;
  min-width: 20px;
  height: 20px;
  padding: 0 var(--space-1);
  border: 1px solid var(--separator);
  border-radius: var(--radius-xs);
  background: var(--bg-elevated);
  font-family: var(--font-sans);
  font-size: var(--type-caption-size);
}

@keyframes palette-fade {
  from { opacity: 0; }
  to { opacity: 1; }
}

@keyframes palette-in {
  from {
    opacity: 0;
    transform: scale(0.98) translateY(-4px);
  }

  to {
    opacity: 1;
    transform: none;
  }
}

@media (max-width: 833px) {
  .palette-overlay {
    padding: var(--space-3) var(--space-3) var(--space-4);
  }

  .palette {
    max-height: min(560px, 100%);
  }

  .palette__footer {
    display: none;
  }
}

@media (pointer: coarse) {
  .palette__item {
    min-height: 44px;
  }
}

@media (forced-colors: active) {
  .palette__item[data-highlighted] {
    outline: 2px solid transparent;
    text-decoration: underline;
  }
}
</style>
