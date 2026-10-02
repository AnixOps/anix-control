<template>
  <UiSheet
    :open="open"
    size="lg"
    :title="t('admin.nodes.deploy.title')"
    :description="t('admin.nodes.deploy.summaryText')"
    data-testid="node-deploy-sheet"
    @update:open="value => emit('update:open', value)"
  >
    <section class="dialog-section">
      <h3 class="dialog-section__title">{{ t('admin.nodes.deploy.summaryTitle', { count: nodes.length }) }}</h3>
      <p class="node-deploy__note">{{ t('admin.nodes.deploy.warning') }}</p>
    </section>

    <section class="dialog-section">
      <h3 class="dialog-section__title">{{ t('admin.nodes.deploy.settingsTitle') }}</h3>
      <NodeDeploySettings :deploy="deploy" ansible />
    </section>

    <section class="dialog-section">
      <h3 class="dialog-section__title">{{ t('admin.nodes.deploy.hostsTitle') }}</h3>
      <p v-if="deploy.deployError" class="form-error" role="alert">{{ deploy.deployError }}</p>
      <UiSkeleton v-if="showSkeleton" variant="card" :label="t('admin.nodes.deploy.loading')" />
      <p v-else-if="!deploy.deployLoading && deploy.deployRows.length === 0" class="node-deploy__note">{{ t('admin.nodes.deploy.empty') }}</p>
      <template v-else>
        <fieldset v-for="row in deploy.deployRows" :key="row.id" class="node-deploy__host">
          <legend class="node-deploy__legend">
            <span>{{ row.name }}</span>
            <span class="node-deploy__meta">ID {{ row.nodeId }}</span>
          </legend>
          <div class="form-grid">
            <UiTextField v-model.trim="row.alias" size="md" :label="t('admin.nodes.deploy.table.alias')" />
            <UiTextField v-model.trim="row.host" size="md" :label="t('admin.nodes.deploy.table.sshHost')" />
            <UiNumberField v-model="row.sshPort" size="md" :min="1" :max="65535" :label="t('admin.nodes.deploy.table.port')" />
            <UiTextField v-model.trim="row.sshUser" size="md" :label="t('admin.nodes.deploy.table.user')" />
            <UiSelect v-model="row.arch" size="md" :label="t('admin.nodes.deploy.table.arch')" :options="ARCHES" />
            <UiSelect v-model="row.authMode" size="md" :label="t('admin.nodes.deploy.table.authMode')" :options="authModes" />
            <UiPasswordField
              v-if="row.authMode === 'password'"
              v-model.trim="row.authValue"
              class="form-grid__full"
              size="md"
              autocomplete="off"
              :label="t('admin.nodes.deploy.table.authValue')"
              :help="t('admin.nodes.deploy.placeholders.password')"
            />
            <UiTextField
              v-else
              v-model.trim="row.authValue"
              class="form-grid__full"
              size="md"
              :label="t('admin.nodes.deploy.table.authValue')"
              :placeholder="t('admin.nodes.deploy.placeholders.privateKey')"
            />
          </div>
        </fieldset>
      </template>
    </section>

    <section class="dialog-section">
      <h3 class="dialog-section__title">{{ t('admin.nodes.deploy.outputTitle') }}</h3>
      <UiCodeBlock label="inventory.ini" :code="deploy.deployInventoryPreview" max-height="320px" />
      <UiCodeBlock label="group_vars/all.yml" :code="deploy.deployGroupVarsPreview" max-height="320px" />
      <UiCodeBlock :label="t('admin.nodes.deploy.commandsLabel')" :code="deploy.deployCommandsPreview" max-height="320px" />
    </section>

    <template #footer="{ close }">
      <UiButton @click="close">{{ t('common.actions.close') }}</UiButton>
    </template>
  </UiSheet>
</template>

<script setup>
// Ansible deployment helper for parent nodes (nodes without a parent): one
// host per node with its API key, the connection settings, and the
// generated inventory.ini, group_vars/all.yml and playbook commands for
// config/deploy/ansible/nodes. SSH passwords stay in this page only.
import { computed, toRef, watch } from 'vue'
import UiButton from '@/ui/UiButton.vue'
import UiNumberField from '@/ui/UiNumberField.vue'
import UiPasswordField from '@/ui/UiPasswordField.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiSheet from '@/ui/UiSheet.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import UiTextField from '@/ui/UiTextField.vue'
import { useDelayedLoading } from '@/ui/composables/useDelayedLoading'
import { useAppI18n } from '@/composables/useAppI18n'
import UiCodeBlock from '@/ui/UiCodeBlock.vue'
import NodeDeploySettings from './NodeDeploySettings.vue'

const props = defineProps({
  open: { type: Boolean, default: false },
  // Parent nodes to deploy.
  nodes: { type: Array, default: () => [] },
  // reactive(useNodeDeploy())
  deploy: { type: Object, required: true }
})
const emit = defineEmits(['update:open'])
const { t } = useAppI18n()
const ARCHES = ['amd64', 'arm64']
const authModes = computed(() => [
  { value: 'password', label: t('admin.nodes.deploy.authModes.password') },
  { value: 'key', label: t('admin.nodes.deploy.authModes.key') }
])
const showSkeleton = useDelayedLoading(toRef(() => props.deploy.deployLoading))

// Each opening reads the parent nodes' API keys again.
watch(() => props.open, (open) => {
  if (open) props.deploy.loadDeployRows(props.nodes)
}, { immediate: true })
</script>

<style scoped>
.node-deploy__note {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
}

.node-deploy__host {
  min-width: 0;
  margin: 0;
  padding: var(--space-4);
  border: 1px solid var(--separator);
  border-radius: var(--radius-md);
}

.node-deploy__legend {
  display: inline-flex;
  gap: var(--space-2);
  align-items: baseline;
  padding: 0 var(--space-1);
  color: var(--label-1);
  font-weight: var(--weight-semibold);
}

.node-deploy__meta {
  color: var(--label-2);
  font-size: var(--type-caption-size);
  font-weight: var(--weight-regular);
}
</style>
