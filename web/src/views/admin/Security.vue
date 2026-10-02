<template>
  <div class="settings-page">
    <UiPageHeader :title="t('adminSecurity.title')" :description="t('adminSecurity.subtitle')" />
    <SettingsLayout
      :sections="sections"
      :current="current"
      :nav-label="t('adminSecurity.navLabel')"
      :back-label="t('adminSecurity.title')"
      back-to="/admin/security"
    >
      <MFA v-if="current === 'mfa'" key="mfa" />
      <AccessGroups v-else-if="current === 'access-groups'" key="access-groups" embedded />
    </SettingsLayout>
  </div>
</template>

<script setup>
// 安全 (plan §8.2): the MFA policy and the access groups as sections of one
// page (/admin/security/:section), on the settings template. Both sections
// are the existing pages (MFA.vue, AccessGroups.vue) with their own requests,
// unchanged; /admin/mfa and /admin/access-groups redirect here. The API has
// no administrator API tokens, so there is no third section.
import { computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { KeyRound, ShieldCheck } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { NARROW_QUERY, useMediaQuery } from '@/composables/useMediaQuery'
import { ADMIN_PAGE_SECTIONS } from '@/navigation/menu'
import SettingsLayout from '@/components/admin/settings/SettingsLayout.vue'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import AccessGroups from './AccessGroups.vue'
import MFA from './MFA.vue'

const SECTION_ICONS = { mfa: ShieldCheck, 'access-groups': KeyRound }

const { t } = useAppI18n()
const route = useRoute()
const router = useRouter()
const narrow = useMediaQuery(NARROW_QUERY)

const sections = computed(() => ADMIN_PAGE_SECTIONS.security.map(entry => {
  const id = entry.id.replace(/^security-/, '')
  return { id, label: t(entry.labelKey), icon: SECTION_ICONS[id], to: entry.to }
}))
const requested = computed(() => String(route.params.section || ''))
const current = computed(() => {
  if (SECTION_ICONS[requested.value]) return requested.value
  return narrow.value ? '' : 'mfa'
})

watch(requested, value => {
  if (value && !SECTION_ICONS[value]) router.replace('/admin/security')
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
