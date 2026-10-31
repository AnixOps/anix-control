<template>
  <div class="settings-page">
    <UiPageHeader :title="t('adminSettings.title')" :description="t('adminSettings.subtitle')" />
    <SettingsLayout
      :sections="sections"
      :current="current"
      :nav-label="t('adminSettings.navLabel')"
      :back-label="t('adminSettings.title')"
      back-to="/admin/system"
    >
      <component :is="SECTION_COMPONENTS[current]" v-if="current" :key="current" />
    </SettingsLayout>
  </div>
</template>

<script setup>
// 系统设置 (plan §7.3, §8.2): the settings template with the sections in the
// URL (/admin/system/:section): 通用, 数据与保留, 备份, 审计日志,
// 内置包, 高级 (the raw configuration keys) and 关于. Each section is its own component under
// views/admin/system/ that loads its own data when opened, so a section
// change does not reload the others. Sections with a form show the
// 保存 / 放弃 bar while it has changes and ask before leaving it.
// /admin/system shows 通用 on wide screens and the section list on phones.
import { computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Archive, Database, Info, Package, ScrollText, SlidersHorizontal, Wrench } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { NARROW_QUERY, useMediaQuery } from '@/composables/useMediaQuery'
import { ADMIN_PAGE_SECTIONS } from '@/navigation/menu'
import SettingsLayout from '@/components/admin/settings/SettingsLayout.vue'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import SettingsAbout from './system/SettingsAbout.vue'
import SettingsAdvanced from './system/SettingsAdvanced.vue'
import SettingsAudit from './system/SettingsAudit.vue'
import SettingsBackup from './system/SettingsBackup.vue'
import SettingsData from './system/SettingsData.vue'
import SettingsGeneral from './system/SettingsGeneral.vue'
import SettingsPackages from './system/SettingsPackages.vue'

const SECTION_COMPONENTS = {
  general: SettingsGeneral,
  data: SettingsData,
  backup: SettingsBackup,
  audit: SettingsAudit,
  packages: SettingsPackages,
  advanced: SettingsAdvanced,
  about: SettingsAbout
}
const SECTION_ICONS = {
  general: SlidersHorizontal,
  data: Database,
  backup: Archive,
  audit: ScrollText,
  packages: Package,
  advanced: Wrench,
  about: Info
}

const { t } = useAppI18n()
const route = useRoute()
const router = useRouter()
const narrow = useMediaQuery(NARROW_QUERY)

const sections = computed(() => ADMIN_PAGE_SECTIONS.settings.map(entry => {
  const id = entry.id.replace(/^settings-/, '')
  return { id, label: t(entry.labelKey), icon: SECTION_ICONS[id], to: entry.to }
}))

const requested = computed(() => String(route.params.section || ''))
const current = computed(() => {
  if (SECTION_COMPONENTS[requested.value]) return requested.value
  return narrow.value ? '' : 'general'
})

// An unknown section goes to the page itself (no dead end).
watch(requested, value => {
  if (value && !SECTION_COMPONENTS[value]) router.replace('/admin/system')
}, { immediate: true })
</script>

<style scoped>
.settings-page {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
  min-width: 0;
}
</style>
