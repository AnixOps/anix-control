<template>
  <UiSheet
    :open="open"
    size="lg"
    :title="t('admin.nodes.install.title')"
    :description="t('admin.nodes.install.description', { node: nodeLabel || node })"
    data-testid="agent-install-sheet"
    @update:open="onOpenChange"
  >
    <section class="dialog-section">
      <p class="agent-install__text">{{ t('admin.nodes.install.intro') }}</p>
      <div class="agent-install__controls">
        <UiSelect v-model="ttlSeconds" size="md" :label="t('admin.nodes.install.ttl')" :options="ttlOptions" data-testid="agent-install-ttl" />
        <UiButton
          variant="primary"
          :icon="KeyRound"
          :loading="loading"
          data-testid="agent-install-generate"
          @click="generate"
        >
          {{ result ? t('admin.nodes.install.regenerate') : t('admin.nodes.install.generate') }}
        </UiButton>
      </div>
      <p v-if="error" class="form-error" role="alert" data-testid="agent-install-error">{{ error }}</p>
    </section>

    <section v-if="result" class="dialog-section" data-testid="agent-install-result">
      <h3 class="dialog-section__title">{{ t('admin.nodes.install.commandTitle') }}</h3>
      <p class="agent-install__warning" role="note">
        {{ t('admin.nodes.install.once', { time: expiresLabel }) }}
      </p>
      <UiSegmentedControl
        v-model="mirror"
        :aria-label="t('admin.nodes.install.mirror')"
        :options="mirrorOptions"
        data-testid="agent-install-mirror"
      />
      <p class="agent-install__text">{{ t(`admin.nodes.install.mirrors.${mirror}Help`) }}</p>
      <UiCodeBlock
        :label="t('admin.nodes.install.commandLabel')"
        :code="activeCommand.command"
        :copy-label="t('admin.nodes.install.copy')"
        wrap
        max-height="200px"
        data-testid="agent-install-command"
      />
      <p v-if="!activeCommand.available" class="agent-install__text" data-testid="agent-install-fallback">
        {{ t('admin.nodes.install.fallback', { note: activeCommand.note }) }}
      </p>
      <p class="agent-install__text">
        {{ result.script?.signed ? t('admin.nodes.install.signed') : t('admin.nodes.install.unsigned') }}
      </p>
      <p class="agent-install__text">{{ t('admin.nodes.install.legacy') }}</p>
    </section>

    <template #footer="{ close }">
      <UiButton @click="close">{{ t('common.actions.close') }}</UiButton>
    </template>
  </UiSheet>
</template>

<script setup>
// "复制安装命令": issues a single-use enrollment token bound to the node and
// shows the one-line install command for each mirror (forward-sdk.md,
// section 9). The token lives only in this sheet and is dropped on close.
import { computed, ref } from 'vue'
import { KeyRound } from '@lucide/vue'
import UiButton from '@/ui/UiButton.vue'
import UiCodeBlock from '@/ui/UiCodeBlock.vue'
import UiSegmentedControl from '@/ui/UiSegmentedControl.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiSheet from '@/ui/UiSheet.vue'
import { createKernelAgentInstallToken } from '@/api/kernel'
import { useAppI18n } from '@/composables/useAppI18n'

const props = defineProps({
  open: { type: Boolean, default: false },
  // The Agent node name: "proxy-<id>" or "forward-<id>".
  node: { type: String, required: true },
  nodeLabel: { type: String, default: '' }
})
const emit = defineEmits(['update:open'])
const { t, formatDateTime } = useAppI18n()

const MIRRORS = ['control', 'cn', 'github']
const ttlSeconds = ref(3600)
const loading = ref(false)
const error = ref('')
const result = ref(null)
const mirror = ref('control')

const ttlOptions = computed(() => [
  { value: 3600, label: t('admin.nodes.install.ttlOptions.hour') },
  { value: 6 * 3600, label: t('admin.nodes.install.ttlOptions.sixHours') },
  { value: 24 * 3600, label: t('admin.nodes.install.ttlOptions.day') },
  { value: 7 * 24 * 3600, label: t('admin.nodes.install.ttlOptions.week') }
])
const mirrorOptions = computed(() => MIRRORS.map(value => ({ value, label: t(`admin.nodes.install.mirrors.${value}`) })))
const activeCommand = computed(() => (
  result.value?.commands?.find(command => command.mirror === mirror.value) || { command: '', available: true, note: '' }
))
const expiresLabel = computed(() => {
  const value = result.value?.expires_at
  return (value && formatDateTime(value)) || '—'
})

function errorMessage(cause) {
  const response = cause?.response?.data
  return response?.error?.message || response?.message || cause?.message || t('admin.nodes.install.failed')
}

async function generate() {
  loading.value = true
  error.value = ''
  try {
    result.value = await createKernelAgentInstallToken(props.node, ttlSeconds.value)
  } catch (cause) {
    result.value = null
    error.value = errorMessage(cause)
  } finally {
    loading.value = false
  }
}

function onOpenChange(value) {
  if (!value) {
    result.value = null
    error.value = ''
    mirror.value = 'control'
  }
  emit('update:open', value)
}
</script>

<style scoped>
.agent-install__controls {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-4);
  align-items: flex-end;
}

.agent-install__text {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
}

.agent-install__warning {
  margin: 0;
  color: var(--label-1);
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
  font-weight: 600;
}
</style>
