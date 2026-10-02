<template>
  <div class="notify-page">
    <UiPageHeader :title="t('adminNotify.title')" :description="t('adminNotify.subtitle')" />
    <UiTabs
      class="notify-channels"
      variant="segmented"
      :model-value="channel"
      :aria-label="t('adminNotify.channelsLabel')"
      :items="channelOptions"
      data-test="notify-channels"
      @update:model-value="goTo"
    >
      <template v-for="option in channelOptions" :key="option.value" #[option.value]>
        <component :is="PANELS[option.value]" />
      </template>
    </UiTabs>
  </div>
</template>

<script setup>
// 通知 (plan §8.2): the e-mail and Telegram channels, the message templates
// and the send log in one page, switched with segmented tabs whose
// choice is in the path (/admin/notifications/:channel). /admin/telegram
// redirects to the Telegram channel. Each panel loads its own data; e-mail
// and the Telegram bot are settings forms with the save bar, and each
// channel has its test send (测试发送 for e-mail, a message to one Telegram
// user).
import { computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAppI18n } from '@/composables/useAppI18n'
import { ADMIN_PAGE_SECTIONS } from '@/navigation/menu'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import UiTabs from '@/ui/UiTabs.vue'
import NotifyEmail from './notifications/NotifyEmail.vue'
import NotifyLogs from './notifications/NotifyLogs.vue'
import NotifyTelegram from './notifications/NotifyTelegram.vue'
import NotifyTemplates from './notifications/NotifyTemplates.vue'

const PANELS = { email: NotifyEmail, telegram: NotifyTelegram, templates: NotifyTemplates, logs: NotifyLogs }

const { t } = useAppI18n()
const route = useRoute()
const router = useRouter()

const channelOptions = computed(() => ADMIN_PAGE_SECTIONS.notifications.map(entry => ({
  value: entry.id.replace(/^notifications-/, ''),
  label: t(entry.labelKey)
})))
const requested = computed(() => String(route.params.channel || ''))
const channel = computed(() => (PANELS[requested.value] ? requested.value : 'email'))

function goTo(value) {
  if (value !== channel.value) router.push(`/admin/notifications/${value}`)
}

watch(requested, value => {
  if (value && !PANELS[value]) router.replace('/admin/notifications')
}, { immediate: true })
</script>

<style scoped>
.notify-page {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
  min-width: 0;
}

.notify-channels :deep(.ui-tabs__panel) {
  padding-top: var(--space-8);
}
</style>
