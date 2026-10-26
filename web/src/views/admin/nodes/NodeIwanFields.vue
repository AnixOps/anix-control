<template>
  <section class="dialog-section node-iwan" data-testid="iwan-fields">
    <h3 class="dialog-section__title">{{ t(`${K}.title`) }}</h3>
    <p class="node-iwan__warning" role="note" data-testid="iwan-unencrypted">
      <UiBadge tone="warning" :label="t(`${K}.unencrypted`)" />
      <span>{{ t(`${K}.unencryptedHint`) }}</span>
    </p>
    <div class="form-grid">
      <UiTextField v-model.trim="iwan.pool" size="md" placeholder="10.66.0.0/16" :label="t(`${K}.fields.pool`)" :help="t(`${K}.hints.pool`)" />
      <UiNumberField v-model="iwan.mtu" size="md" :min="576" :max="1600" :label="t(`${K}.fields.mtu`)" />
      <UiTextField v-model.trim="iwan.dnsUpstream" size="md" placeholder="1.1.1.1" :label="t(`${K}.fields.dnsUpstream`)" :help="t(`${K}.hints.dnsUpstream`)" />
      <UiSwitch v-model="iwan.allowEncrypt" class="form-grid__full" :label="t(`${K}.fields.allowEncrypt`)" :description="t(`${K}.hints.allowEncrypt`)" />
      <UiSwitch v-model="iwan.allowClientToClient" class="form-grid__full" :label="t(`${K}.fields.allowClientToClient`)" />
      <UiTextarea
        v-model="iwan.fixedAddresses"
        class="form-grid__full"
        :rows="4"
        placeholder="7=10.66.0.9"
        spellcheck="false"
        :label="t(`${K}.fields.fixedAddresses`)"
        :help="t(`${K}.hints.fixedAddresses`)"
      />
    </div>
    <p class="node-iwan__hint">{{ t(`${K}.hints.login`) }}</p>
  </section>
</template>

<script setup>
// The Panabit iWAN part of the visual protocol editor. Fields map one to
// one to settings (useProtocolEditor buildIwanSettings). iWAN does not
// encrypt traffic, which the form says up front.
import UiBadge from '@/ui/UiBadge.vue'
import UiNumberField from '@/ui/UiNumberField.vue'
import UiSwitch from '@/ui/UiSwitch.vue'
import UiTextField from '@/ui/UiTextField.vue'
import UiTextarea from '@/ui/UiTextarea.vue'
import { useAppI18n } from '@/composables/useAppI18n'

defineProps({
  // The editor's reactive iwanForm.
  iwan: { type: Object, required: true }
})
const { t } = useAppI18n()
const K = 'admin.nodes.protocolForm.iwan'
</script>

<style scoped>
.node-iwan__warning {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2);
  margin: 0;
  color: var(--label-2);
}

.node-iwan__hint {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}
</style>
