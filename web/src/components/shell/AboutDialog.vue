<template>
  <UiDialog
    :open="open"
    size="sm"
    :title="t('shell.about.title')"
    @update:open="emit('update:open', $event)"
  >
    <p v-if="failed && !rows.length" class="about-note">{{ t('shell.about.unavailable') }}</p>
    <dl v-else class="about-list" data-about-list>
      <div v-for="row in rows" :key="row.key" class="about-row" :data-about-row="row.key">
        <dt>{{ row.label }}</dt>
        <dd>{{ row.value }}</dd>
      </div>
    </dl>
    <template #footer="{ close }">
      <UiButton variant="primary" @click="close">{{ t('shell.about.close') }}</UiButton>
    </template>
  </UiDialog>
</template>

<script setup>
// 关于: product version and build details. They used to sit at the bottom
// of the admin sidebar; 系统设置 → 关于 shows the same rows (aboutRows).
import { computed, ref, watch } from 'vue'
import { getSystemInfo } from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import { FRONTEND_BUILD, aboutRows, readSystemInfo } from '@/utils/systemInfo'
import UiDialog from '@/ui/UiDialog.vue'
import UiButton from '@/ui/UiButton.vue'

const props = defineProps({
  open: { type: Boolean, default: false }
})

const emit = defineEmits(['update:open'])
const { t } = useAppI18n()

const info = ref(null)
const failed = ref(false)

async function load() {
  failed.value = false
  try {
    info.value = readSystemInfo(await getSystemInfo())
  } catch {
    failed.value = true
  }
}

watch(() => props.open, value => {
  if (value && !info.value) void load()
}, { immediate: true })

const rows = computed(() => aboutRows(t, info.value, FRONTEND_BUILD))
</script>

<style scoped>
.about-list {
  display: flex;
  flex-direction: column;
  margin: 0;
  border-radius: var(--radius-sm);
  background: var(--bg-grouped);
}

.about-row {
  display: flex;
  gap: var(--space-4);
  justify-content: space-between;
  padding: var(--space-2) var(--space-4);
  font-size: var(--type-callout-size);
}

.about-row + .about-row {
  border-top: 1px solid var(--separator);
}

.about-row dt {
  color: var(--label-2);
}

.about-row dd {
  margin: 0;
  overflow-wrap: anywhere;
  font-family: var(--font-mono);
  text-align: right;
}

.about-note {
  color: var(--label-2);
}
</style>
