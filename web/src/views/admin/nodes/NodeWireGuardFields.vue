<template>
  <div class="node-wg">
    <section class="dialog-section">
      <h3 class="dialog-section__title">{{ t(`${K}.sections.access`) }}</h3>
      <div class="form-grid">
        <UiTextField v-model.trim="wg.cidr" size="md" :label="t(`${K}.fields.cidr`)" />
        <UiTextField v-model.trim="wg.serverAddress" size="md" :label="t(`${K}.fields.serverAddress`)" />
        <UiPasswordField v-model.trim="wg.serverPrivateKey" size="md" autocomplete="off" :label="t(`${K}.fields.serverPrivateKey`)" />
        <UiTextField v-model.trim="wg.serverPublicKey" size="md" :label="t(`${K}.fields.serverPublicKey`)" />
        <div class="form-grid__full node-wg__row-end">
          <UiButton size="sm" :icon="KeyRound" @click="emit('generate-keypair')">{{ t(`${K}.actions.generateKeypair`) }}</UiButton>
        </div>
        <UiNumberField v-model="wg.mtu" size="md" :min="576" :max="1500" :label="t(`${K}.fields.mtu`)" />
        <UiTextField v-model.trim="wg.dns" size="md" :label="t(`${K}.fields.dns`)" />
        <UiTextField v-model.trim="wg.allowedIps" class="form-grid__full" size="md" :label="t(`${K}.fields.allowedIps`)" />
      </div>
    </section>
  </div>
</template>

<script setup>
// The WireGuard part of the visual protocol editor: the node terminates its
// users and egresses itself (the relay over GOST was removed with gost).
// Fields map one to one to settings (useProtocolEditor).
import { KeyRound } from '@lucide/vue'
import UiButton from '@/ui/UiButton.vue'
import UiNumberField from '@/ui/UiNumberField.vue'
import UiPasswordField from '@/ui/UiPasswordField.vue'
import UiTextField from '@/ui/UiTextField.vue'
import { useAppI18n } from '@/composables/useAppI18n'

defineProps({
  // The editor's reactive wireGuardForm.
  wg: { type: Object, required: true }
})
const emit = defineEmits(['generate-keypair'])
const { t } = useAppI18n()
const K = 'admin.nodes.protocolForm.wireguard'
</script>

<style scoped>
.node-wg {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
}

.node-wg__row-end {
  display: flex;
  justify-content: flex-end;
}
</style>
