<template>
  <div class="node-wg">
    <section class="dialog-section">
      <h3 class="dialog-section__title">{{ t(`${K}.sections.access`) }}</h3>
      <div class="form-grid">
        <UiTextField v-model.trim="wg.cidr" size="md" :label="t(`${K}.fields.cidr`)" />
        <UiTextField v-if="!isExit" v-model.trim="wg.serverAddress" size="md" :label="t(`${K}.fields.serverAddress`)" />
        <template v-if="!isExit">
          <UiPasswordField v-model.trim="wg.serverPrivateKey" size="md" autocomplete="off" :label="t(`${K}.fields.serverPrivateKey`)" />
          <UiTextField v-model.trim="wg.serverPublicKey" size="md" :label="t(`${K}.fields.serverPublicKey`)" />
          <div class="form-grid__full node-wg__row-end">
            <UiButton size="sm" :icon="KeyRound" @click="emit('generate-keypair')">{{ t(`${K}.actions.generateKeypair`) }}</UiButton>
          </div>
        </template>
        <UiNumberField v-model="wg.mtu" size="md" :min="576" :max="1500" :label="t(`${K}.fields.mtu`)" />
        <UiTextField v-if="!isExit" v-model.trim="wg.dns" size="md" :label="t(`${K}.fields.dns`)" />
        <UiTextField v-if="!isExit" v-model.trim="wg.allowedIps" class="form-grid__full" size="md" :label="t(`${K}.fields.allowedIps`)" />
      </div>
    </section>

    <section class="dialog-section">
      <h3 class="dialog-section__title">{{ t(`${K}.sections.relay`) }}</h3>
      <div class="form-grid">
        <UiSelect v-model="wg.role" size="md" :label="t(`${K}.fields.role`)" :options="roleOptions" />
        <UiSelect v-model="wg.tunnelType" size="md" :label="t(`${K}.fields.tunnelType`)" :options="TUNNEL_OPTIONS" />
        <UiSwitch v-model="wg.wssCompat" class="form-grid__full" :label="t(`${K}.fields.wssCompat`)" :description="t(`${K}.hints.wssCompat`)" />
        <template v-if="usesWss">
          <UiTextField v-model.trim="wg.wssPath" size="md" :label="t(`${K}.fields.wssPath`)" />
          <template v-if="wg.role === 'entry'">
            <UiTextField v-model.trim="wg.wssServerName" size="md" autocomplete="off" :label="t(`${K}.fields.wssServerName`)" />
            <UiTextField v-model.trim="wg.wssCaFile" size="md" autocomplete="off" :label="t(`${K}.fields.wssCaFile`)" />
            <UiCheckbox v-model="wg.wssSecure" class="node-wg__check" disabled :label="t(`${K}.fields.wssSecure`)" />
          </template>
          <template v-else>
            <UiTextField v-model.trim="wg.wssCertFile" size="md" autocomplete="off" :label="t(`${K}.fields.wssCertFile`)" />
            <UiTextField v-model.trim="wg.wssKeyFile" size="md" autocomplete="off" :label="t(`${K}.fields.wssKeyFile`)" />
          </template>
        </template>
        <UiTextField v-model.trim="wg.relayServer" size="md" :label="t(`${K}.fields.relayServer`)" />
        <UiNumberField v-model="wg.relayServerPort" size="md" :min="0" :max="65535" :label="t(`${K}.fields.relayServerPort`)" />
        <UiNumberField v-model="wg.tunPort" size="md" :min="1" :max="65535" :label="t(`${K}.fields.tunPort`)" />
        <UiTextField v-model.trim="wg.tunName" size="md" :label="t(`${K}.fields.tunName`)" />
        <UiTextField v-model.trim="wg.entryTunAddress" size="md" :label="t(`${K}.fields.entryTunAddress`)" />
        <UiTextField v-model.trim="wg.exitTunAddress" size="md" :label="t(`${K}.fields.exitTunAddress`)" />
        <UiTextField v-model.trim="wg.outboundIface" size="md" :label="t(`${K}.fields.outboundIface`)" />
        <UiCheckbox v-model="wg.exitNat" class="node-wg__check" :label="t(`${K}.fields.exitNat`)" />
        <UiNumberField v-model="wg.routingTable" size="md" :min="0" :label="t(`${K}.fields.routingTable`)" />
        <UiNumberField v-model="wg.routingPriority" size="md" :min="0" :label="t(`${K}.fields.routingPriority`)" />
      </div>
    </section>

    <section v-if="wg.role === 'entry' && usesWss" class="dialog-section">
      <h3 class="dialog-section__title">{{ t(`${K}.sections.networkPolicy`) }}</h3>
      <UiSwitch v-model="wg.networkPolicyEnabled" :label="t(`${K}.fields.networkPolicyEnabled`)" :description="t(`${K}.hints.networkPolicy`)" />
      <template v-if="wg.networkPolicyEnabled">
        <fieldset v-for="(path, index) in wg.networkPaths" :key="index" class="node-wg__path">
          <legend class="node-wg__legend">{{ t(`${K}.fields.networkPath`) }} {{ index + 1 }}</legend>
          <div class="form-grid">
            <UiTextField v-model.trim="path.name" size="md" placeholder="CN2" :label="t(`${K}.fields.pathName`)" />
            <UiTextField v-model.trim="path.interface" size="md" placeholder="eth1" :label="t(`${K}.fields.pathInterface`)" />
            <UiTextField v-model.trim="path.source" size="md" placeholder="10.8.0.112" :label="t(`${K}.fields.pathSource`)" />
            <UiTextField v-model.trim="path.gateway" size="md" placeholder="10.8.0.1" :label="t(`${K}.fields.pathGateway`)" />
            <UiNumberField v-model="path.priority" size="md" :min="0" :label="t(`${K}.fields.pathPriority`)" />
          </div>
          <div class="node-wg__row-end">
            <UiButton size="sm" variant="danger-soft" :icon="Trash2" @click="emit('remove-path', index)">{{ t(`${K}.actions.removeNetworkPath`, { index: index + 1 }) }}</UiButton>
          </div>
        </fieldset>
        <div>
          <UiButton size="sm" :icon="Plus" @click="emit('add-path')">{{ t(`${K}.actions.addNetworkPath`) }}</UiButton>
        </div>
        <div class="form-grid">
          <UiNumberField v-model="wg.healthInterval" size="md" :min="1" :label="t(`${K}.fields.healthInterval`)" />
          <UiNumberField v-model="wg.healthTimeout" size="md" :min="1" :label="t(`${K}.fields.healthTimeout`)" />
          <UiNumberField v-model="wg.failureThreshold" size="md" :min="1" :label="t(`${K}.fields.failureThreshold`)" />
          <UiNumberField v-model="wg.failbackDelay" size="md" :min="0" :label="t(`${K}.fields.failbackDelay`)" />
        </div>
      </template>
    </section>
  </div>
</template>

<script setup>
// The WireGuard relay part of the visual protocol editor: access, the
// entry/exit relay over GOST (QUIC or WSS) and the optional failover paths
// of an entry node. Fields map one to one to settings (useProtocolEditor).
import { computed } from 'vue'
import { KeyRound, Plus, Trash2 } from '@lucide/vue'
import UiButton from '@/ui/UiButton.vue'
import UiCheckbox from '@/ui/UiCheckbox.vue'
import UiNumberField from '@/ui/UiNumberField.vue'
import UiPasswordField from '@/ui/UiPasswordField.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiSwitch from '@/ui/UiSwitch.vue'
import UiTextField from '@/ui/UiTextField.vue'
import { useAppI18n } from '@/composables/useAppI18n'

const props = defineProps({
  // The editor's reactive wireGuardForm.
  wg: { type: Object, required: true }
})
const emit = defineEmits(['generate-keypair', 'add-path', 'remove-path'])
const { t } = useAppI18n()
const K = 'admin.nodes.protocolForm.wireguard'
const TUNNEL_OPTIONS = [
  { value: 'quic', label: 'GOST relay+QUIC' },
  { value: 'wss', label: 'GOST relay+WSS' }
]
const roleOptions = computed(() => [
  { value: 'entry', label: t(`${K}.values.entry`) },
  { value: 'exit', label: t(`${K}.values.exit`) }
])
const isExit = computed(() => props.wg.role === 'exit')
const usesWss = computed(() => props.wg.wssCompat || props.wg.tunnelType === 'wss')
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

.node-wg__check {
  align-self: end;
  min-height: var(--size-control-md);
}

.node-wg__path {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  min-width: 0;
  margin: 0;
  padding: var(--space-4);
  border: 1px solid var(--separator);
  border-radius: var(--radius-md);
}

.node-wg__legend {
  padding: 0 var(--space-1);
  color: var(--label-1);
  font-weight: var(--weight-semibold);
}
</style>
