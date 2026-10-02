<template>
  <div class="node-auth-key">
    <p class="node-auth-key__hint">{{ t('admin.nodes.authKey.hint') }}</p>

    <UiCopyField
      v-if="deploy.authKey"
      :value="deploy.authKey"
      :label="t('admin.nodes.authKey.label')"
      :help="t('admin.nodes.authKey.shownOnce')"
      :copy-label="t('admin.nodes.authKey.copy')"
      secret
      size="md"
      data-testid="auth-key-value"
    />
    <div v-else class="node-auth-key__empty" data-testid="auth-key-empty">
      <span class="node-auth-key__empty-label">{{ t('admin.nodes.authKey.label') }}</span>
      <span class="node-auth-key__empty-value">{{ deploy.authKeyMasked ? t('admin.nodes.authKey.hiddenKey') : t('admin.nodes.authKey.noKey') }}</span>
      <span v-if="deploy.authKeyMasked" class="node-auth-key__empty-help">{{ t('admin.nodes.authKey.hiddenHint') }}</span>
    </div>

    <div class="node-auth-key__actions">
      <span v-if="deploy.authKeyUsed > 0" class="node-auth-key__used">{{ t('admin.nodes.authKey.registeredCount', { count: deploy.authKeyUsed }) }}</span>
      <UiButton
        size="md"
        :icon="KeyRound"
        :loading="deploy.authKeyGenerating"
        data-testid="generate-auth-key"
        @click="deploy.createAuthKey()"
      >{{ deploy.authKey ? t('admin.nodes.authKey.generateAnother') : t('admin.nodes.authKey.generate') }}</UiButton>
    </div>
    <p v-if="deploy.authKeyError" class="form-error" role="alert" data-testid="auth-key-error">{{ deploy.authKeyError }}</p>

    <NodeCodeBlock
      :title="t('admin.nodes.authKey.configTitle')"
      :code="deploy.configSnippet"
      :copy-label="t('admin.nodes.authKey.copyConfig')"
      :disabled="!deploy.pluginSupervisorCanaryReady"
      data-testid="agent-config"
      @copy="copyWithToast"
    />
    <p class="node-auth-key__note">
      {{ t('admin.nodes.authKey.configHint') }}
      <UiButton v-if="settingsLink" variant="tertiary" size="sm" class="node-auth-key__link" @click="emit('edit-settings')">{{ t('admin.nodes.authKey.editSettings') }}</UiButton>
    </p>
    <p v-if="deploy.deploySettings.pluginSupervisorEnabled && !deploy.pluginSupervisorCanaryReady" class="form-error" role="alert">
      {{ deploy.pluginSupervisorCanaryError }}
    </p>
  </div>
</template>

<script setup>
// The Agent registration key and the config.json example that uses it.
// A key is shown once, when generated (the list endpoint masks keys); the
// field keeps it masked with a reveal toggle, and copying never needs it.
import { KeyRound } from '@lucide/vue'
import UiButton from '@/ui/UiButton.vue'
import UiCopyField from '@/ui/UiCopyField.vue'
import { useAppI18n } from '@/composables/useAppI18n'
import NodeCodeBlock from './NodeCodeBlock.vue'
import { useNodeCopy } from './useNodeCopy'

defineProps({
  // reactive(useNodeDeploy())
  deploy: { type: Object, required: true },
  // Offer 修改连接设置 (the list page, where the settings live in the
  // deployment helper).
  settingsLink: { type: Boolean, default: false }
})
const emit = defineEmits(['edit-settings'])
const { t } = useAppI18n()
const copyWithToast = useNodeCopy()
</script>

<style scoped>
.node-auth-key {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  min-width: 0;
}

.node-auth-key__hint,
.node-auth-key__note {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
}

.node-auth-key__link {
  margin-left: var(--space-1);
}

.node-auth-key__empty {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-sm);
  background: var(--bg-grouped);
}

.node-auth-key__empty-label {
  color: var(--label-2);
  font-size: var(--type-caption-size);
  font-weight: var(--weight-medium);
}

.node-auth-key__empty-value {
  color: var(--label-1);
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
}

.node-auth-key__empty-help {
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

.node-auth-key__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3);
  align-items: center;
  justify-content: flex-end;
}

.node-auth-key__used {
  margin-right: auto;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.form-error {
  margin: 0;
}
</style>
