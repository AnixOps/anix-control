<template>
  <span v-if="failed" class="user-last-online user-last-online--muted" data-testid="last-online-unavailable">{{ t('adminUsers.lastOnline.unavailable') }}</span>
  <span v-else-if="value === undefined" class="user-last-online user-last-online--muted" data-testid="last-online-loading">
    <span aria-hidden="true">—</span><span class="visually-hidden">{{ t('adminUsers.lastOnline.loading') }}</span>
  </span>
  <span v-else-if="value === null" class="user-last-online user-last-online--muted" :title="t('adminUsers.lastOnline.neverHint')" data-testid="last-online-never">{{ t('adminUsers.lastOnline.never') }}</span>
  <time v-else class="user-last-online" :datetime="iso" :title="format.dateTime(value)" data-testid="last-online">{{ format.relativeTime(value) }}</time>
</template>

<script setup>
// When a user was last seen on a node: a relative time ("3 minutes ago",
// the exact time on hover), "Never" for a user no node has reported yet, a
// dash while the lookup is on its way and "Unavailable" when it failed. The
// value is Unix seconds from GET /api/v4/admin/users/activity: undefined
// (not loaded), null (never seen) or a number.
import { computed } from 'vue'
import { useFormat } from '@/ui/composables/useFormat'
import { useAppI18n } from '@/composables/useAppI18n'

const props = defineProps({
  value: { type: Number, default: undefined },
  failed: { type: Boolean, default: false }
})
const { t } = useAppI18n()
const format = useFormat()
const iso = computed(() => (Number.isFinite(props.value) ? new Date(props.value * 1000).toISOString() : undefined))
</script>

<style scoped>
.user-last-online--muted {
  color: var(--label-2);
}
</style>
