<template>
  <section class="dialog-section" data-testid="join-command">
    <h3 class="dialog-section__title">{{ t('admin.nodes.join.command.title') }}</h3>

    <NodeNotice v-if="group.token_expired" tone="danger" data-testid="join-token-expired">
      {{ t('admin.nodes.join.command.expired') }}
    </NodeNotice>

    <NodeNotice v-if="problem === 'forbidden'" tone="warning" data-testid="join-command-forbidden">
      {{ t('admin.nodes.join.command.forbidden') }}
    </NodeNotice>
    <template v-else-if="problem === 'failed'">
      <p class="form-error" role="alert" data-testid="join-command-failed">{{ t('admin.nodes.join.command.failed', { message: problemMessage }) }}</p>
      <div>
        <UiButton :icon="RotateCw" data-testid="join-command-retry" @click="emit('retry')">{{ t('admin.nodes.join.retry') }}</UiButton>
      </div>
    </template>
    <p v-else-if="!command" class="join-command__text" role="status">{{ t('admin.nodes.join.loading') }}</p>

    <template v-else>
      <UiSegmentedControl
        v-model="mirror"
        :aria-label="t('admin.nodes.join.command.mirror')"
        :options="mirrorOptions"
        data-testid="join-mirror"
      />
      <p class="join-command__text">{{ t(`admin.nodes.join.command.mirrors.${mirror}Help`) }}</p>
      <UiCodeBlock
        :label="t('admin.nodes.join.command.commandLabel')"
        :code="activeCommand.command"
        :copy-label="t('admin.nodes.join.command.copy')"
        wrap
        max-height="200px"
        data-testid="join-command-text"
      />
      <p v-if="!activeCommand.available" class="join-command__text" data-testid="join-command-fallback">
        {{ t('admin.nodes.join.command.fallback', { note: activeCommand.note }) }}
      </p>
      <p class="join-command__text" data-testid="join-token-expiry">
        {{ expiryText }}
      </p>
      <p class="join-command__text">{{ t('admin.nodes.join.command.history') }}</p>

      <div class="join-command__actions">
        <UiButton :icon="RotateCcw" data-testid="join-reset-token" @click="emit('reset')">{{ t('admin.nodes.join.command.reset') }}</UiButton>
      </div>
    </template>
  </section>

  <!-- The machines that joined go right under the command. -->
  <slot />

  <section v-if="command" class="dialog-section" data-testid="join-uninstall">
    <UiCodeBlock
      :label="t('admin.nodes.join.command.uninstall')"
      :code="command.uninstall_command || DEFAULT_UNINSTALL"
      :copy-label="t('admin.nodes.join.command.copy')"
      wrap
      max-height="120px"
      data-testid="join-uninstall-command"
    />
    <p class="join-command__text">{{ t('admin.nodes.join.command.uninstallHelp') }}</p>
  </section>
</template>

<script setup>
// The command of the selected node group: one per download source with
// copy, when the token ends and 重置令牌 (a confirmation owned by the dialog);
// then, in the default slot, the machines that joined; then the uninstall
// command. The token lives only in this dialog's state.
import { computed, ref, watch } from 'vue'
import { RotateCcw, RotateCw } from '@lucide/vue'
import UiButton from '@/ui/UiButton.vue'
import UiCodeBlock from '@/ui/UiCodeBlock.vue'
import UiSegmentedControl from '@/ui/UiSegmentedControl.vue'
import { useAppI18n } from '@/composables/useAppI18n'
import NodeNotice from './NodeNotice.vue'

const DEFAULT_UNINSTALL = 'sudo anix-agent uninstall'
const MIRRORS = ['control', 'cn', 'github']

const props = defineProps({
  group: { type: Object, required: true },
  command: { type: Object, default: null },
  // '' | 'forbidden' | 'failed'
  problem: { type: String, default: '' },
  problemMessage: { type: String, default: '' }
})
const emit = defineEmits(['reset', 'retry'])
const { t, formatDateTime } = useAppI18n()

const mirror = ref('control')
const mirrorOptions = computed(() => MIRRORS.map(value => ({ value, label: t(`admin.nodes.join.command.mirrors.${value}`) })))
const activeCommand = computed(() => (
  props.command?.commands?.find(item => item.mirror === mirror.value) || { command: '', available: true, note: '' }
))
const expiryText = computed(() => {
  const value = props.command?.token_expires_at || props.group.token_expires_at
  if (!value) return t('admin.nodes.join.command.neverExpires')
  return t('admin.nodes.join.command.expires', { time: formatDateTime(value) || value })
})

// Another group's command starts on the default source again.
watch(() => props.group.id, () => { mirror.value = 'control' })
</script>

<style scoped>
.join-command__text {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
}

.join-command__actions {
  display: flex;
  gap: var(--space-2);
}
</style>
