<template>
  <UiDialog
    :open="open"
    :title="title"
    data-test="forward-address-dialog"
    @update:open="value => emit('update:open', value)"
  >
    <ul class="address-list">
      <li v-for="item in addresses" :key="item.id" class="address-list__item">
        <code class="address-list__value">{{ item.address }}</code>
        <UiButton size="sm" :icon="Copy" :loading="item.copying" :aria-label="`${t('runtime.forward.addressModal.copy')} ${item.address}`" @click="emit('copy', item)">
          {{ t('runtime.forward.addressModal.copy') }}
        </UiButton>
      </li>
    </ul>
    <template #footer="{ close }">
      <UiButton @click="close">{{ t('common.actions.close') }}</UiButton>
      <UiButton variant="primary" :icon="CopyCheck" @click="emit('copy-all')">{{ t('runtime.forward.actions.copyAll') }}</UiButton>
    </template>
  </UiDialog>
</template>

<script setup>
// The addresses of a rule with several ingress IPs or targets, each with
// copy, and 复制全部. A single address is copied straight from the table.
import { Copy, CopyCheck } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'
import UiButton from '@/ui/UiButton.vue'
import UiDialog from '@/ui/UiDialog.vue'

defineProps({
  open: { type: Boolean, default: false },
  title: { type: String, default: '' },
  // [{ id, address, copying }]
  addresses: { type: Array, default: () => [] }
})

const emit = defineEmits(['update:open', 'copy', 'copy-all'])
const { t } = useAppI18n()
</script>

<style scoped>
.address-list {
  display: flex;
  flex-direction: column;
  margin: 0;
  padding: 0;
  border-radius: var(--radius-md);
  background: var(--fill-1);
  list-style: none;
}

.address-list__item {
  display: flex;
  gap: var(--space-3);
  align-items: center;
  justify-content: space-between;
  min-height: 52px;
  padding: var(--space-2) var(--space-3);
}

.address-list__item + .address-list__item {
  border-top: 0.5px solid var(--separator);
}

.address-list__value {
  min-width: 0;
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
  overflow-wrap: anywhere;
}
</style>
