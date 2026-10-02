<template>
  <UiDialog
    :open="open"
    :title="t('adminSubscriptionGroups.links.title', { name: group?.name || '' })"
    :description="token ? t('adminSubscriptionGroups.links.description') : ''"
    @update:open="emit('update:open', $event)"
  >
    <p v-if="!token" class="links-note">{{ t('adminSubscriptionGroups.links.noToken') }}</p>
    <div v-else class="links-list">
      <UiCopyField
        v-for="format in SUBSCRIPTION_FORMATS"
        :key="format"
        :value="linkFor(format)"
        :label="t(`adminSubscriptionGroups.formats.${format}`)"
        size="md"
      />
    </div>
  </UiDialog>
</template>

<script setup>
// The group's subscription links in every format, signed with the
// administrator's own subscription token (as before): one copy field each.
import { computed } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { useUserStore } from '@/stores/user'
import UiCopyField from '@/ui/UiCopyField.vue'
import UiDialog from '@/ui/UiDialog.vue'
import { SUBSCRIPTION_FORMATS, groupSubscriptionUrl } from './subscriptionShared'

const props = defineProps({
  open: { type: Boolean, default: false },
  group: { type: Object, default: null }
})
const emit = defineEmits(['update:open'])

const { t } = useAppI18n()
const userStore = useUserStore()
const token = computed(() => userStore.userInfo?.token || '')

function linkFor(format) {
  return groupSubscriptionUrl({ origin: window.location.origin, token: token.value, groupId: props.group?.id, format })
}
</script>

<style scoped>
.links-list {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.links-note {
  margin: 0;
  color: var(--label-2);
}
</style>
