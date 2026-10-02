<template>
  <div class="node-deploy-settings">
    <div class="form-grid">
      <UiTextField v-model.trim="settings.panelApiHost" size="md" :label="t('admin.nodes.deploy.fields.panelApiHost')" />
      <UiTextField v-model.trim="settings.grpcHost" size="md" :label="t('admin.nodes.deploy.fields.grpcHost')" />
      <UiTextField v-model.trim="settings.grpcServerName" size="md" :label="t('admin.nodes.deploy.fields.grpcServerName')" />
      <UiSelect v-model="settings.coreType" size="md" :label="t('admin.nodes.deploy.fields.coreType')" :options="CORE_TYPES" />
      <template v-if="ansible">
        <UiTextField v-model.trim="settings.amd64BinaryPath" class="form-grid__full" size="md" :label="t('admin.nodes.deploy.fields.amd64BinaryPath')" />
        <UiTextField v-model.trim="settings.arm64BinaryPath" class="form-grid__full" size="md" :label="t('admin.nodes.deploy.fields.arm64BinaryPath')" />
      </template>
    </div>
    <UiSwitch v-model="settings.grpcUseTLS" :label="t('admin.nodes.deploy.fields.grpcUseTLS')" data-testid="grpc-use-tls" />
    <UiSwitch
      v-model="settings.pluginSupervisorEnabled"
      :label="t('admin.nodes.deploy.fields.pluginSupervisorEnabled')"
      :description="t('admin.nodes.deploy.pluginSupervisorHint')"
      data-testid="plugin-supervisor-enabled"
    />
    <div v-if="settings.pluginSupervisorEnabled" class="form-grid">
      <UiTextField v-model.trim="settings.pluginRoot" size="md" data-testid="plugin-root" :label="t('admin.nodes.deploy.fields.pluginRoot')" />
      <UiTextField v-model.trim="settings.pluginSocketDir" size="md" data-testid="plugin-socket-dir" :label="t('admin.nodes.deploy.fields.pluginSocketDir')" />
      <UiTextField
        v-model.trim="settings.pluginOfficialPublicKey"
        class="form-grid__full"
        size="md"
        autocomplete="off"
        data-testid="plugin-official-public-key"
        :label="t('admin.nodes.deploy.fields.pluginOfficialPublicKey')"
        :error="deploy.pluginSupervisorCanaryReady ? '' : deploy.pluginSupervisorCanaryError"
      />
    </div>
  </div>
</template>

<script setup>
// Connection settings of an Agent (control API, gRPC, TLS, core) and, for
// the Ansible helper, the binary paths. They only change the generated
// config.json, inventory and group vars; nothing is saved.
import { computed } from 'vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiSwitch from '@/ui/UiSwitch.vue'
import UiTextField from '@/ui/UiTextField.vue'
import { useAppI18n } from '@/composables/useAppI18n'

const props = defineProps({
  // reactive(useNodeDeploy())
  deploy: { type: Object, required: true },
  ansible: { type: Boolean, default: false }
})
const { t } = useAppI18n()
const CORE_TYPES = ['xray', 'sing']
const settings = computed(() => props.deploy.deploySettings)
</script>

<style scoped>
.node-deploy-settings {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  min-width: 0;
}
</style>
