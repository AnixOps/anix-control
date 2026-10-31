<template>
  <section class="dialog-section" data-testid="anx-fields">
    <h3 class="dialog-section__title">{{ t(`${K}.title`) }}</h3>
    <div class="form-grid">
      <UiTextField v-model.trim="anx.serverNames" size="md" placeholder="www.apple.com" :label="t(`${K}.fields.serverNames`)" :help="t(`${K}.hints.serverNames`)" />
      <UiTextField v-model.trim="anx.dest" size="md" placeholder="www.apple.com:443" :label="t(`${K}.fields.dest`)" :help="t(`${K}.hints.dest`)" />
      <UiTextField v-model.trim="anx.alpn" size="md" placeholder="h2,http/1.1" :label="t(`${K}.fields.alpn`)" />
    </div>
    <p class="node-anx__hint">{{ t(`${K}.hints.login`) }}</p>
  </section>
</template>

<script setup>
// The ANX-A part of the visual protocol editor. Fields map one to one to
// settings (useProtocolEditor buildAnxSettings).
import UiTextField from '@/ui/UiTextField.vue'
import { useAppI18n } from '@/composables/useAppI18n'

defineProps({
  // The editor's reactive anxForm.
  anx: { type: Object, required: true }
})
const { t } = useAppI18n()
const K = 'admin.nodes.protocolForm.anx'
</script>

<style scoped>
.node-anx__hint {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}
</style>
