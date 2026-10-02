<template>
  <UiSheet
    :open="open"
    size="lg"
    data-testid="plugin-detail-drawer"
    :title="row?.plugin?.name || row?.plugin?.id || ''"
    :dismissible="canClose"
    @update:open="value => { if (!value) requestClose() }"
  >
    <template v-if="row?.plugin?.id" #description>
      <code>{{ row.plugin.id }}</code>
    </template>
    <p v-if="row?.plugin?.description" class="plugin-description">{{ row.plugin.description }}</p>
    <p v-if="row?.health?.error" class="health-error" role="alert">
      <strong>{{ t('control.errors.action') }}</strong>
      {{ row.health.error }}
    </p>

    <div class="target-tabs" role="tablist" :aria-label="t('control.install.target')">
      <button
        v-for="target in targets"
        :id="`plugin-target-${target.target}`"
        :key="target.target"
        class="target-tab"
        :class="{ active: selectedTarget === target.target }"
        :data-target="target.target"
        type="button"
        role="tab"
        :aria-selected="selectedTarget === target.target"
        :aria-controls="`plugin-target-panel-${target.target}`"
        @click="selectedTarget = target.target"
      >
        {{ target.target }}
      </button>
    </div>

    <section
      v-if="currentTarget"
      :id="`plugin-target-panel-${currentTarget.target}`"
      class="target-detail"
      role="tabpanel"
      :aria-labelledby="`plugin-target-${currentTarget.target}`"
    >
      <dl class="detail-list">
        <div>
          <dt>{{ t('control.table.release') }}</dt>
          <dd><code>{{ currentTarget.latestRelease?.version || '-' }}</code></dd>
        </div>
        <div>
          <dt>{{ t('control.labels.desired') }}</dt>
          <dd><code>{{ currentTarget.installation?.desired_version || '-' }}</code></dd>
        </div>
        <div>
          <dt>{{ t('control.labels.observed') }}</dt>
          <dd><code>{{ currentTarget.installation?.observed_version || '-' }}</code></dd>
        </div>
        <div>
          <dt>{{ t('control.table.state') }}</dt>
          <dd>{{ currentTarget.installation?.state || t('control.states.catalogued') }}</dd>
        </div>
        <div v-if="currentTarget.installation">
          <dt>{{ t('control.table.installation') }}</dt>
          <dd>{{ currentTarget.installation.enabled ? t('control.states.enabled') : t('control.states.disabled') }}</dd>
        </div>
      </dl>

      <p v-if="currentTarget.installation?.last_error" class="health-error" role="alert">
        <strong>{{ t('control.errors.action') }}</strong>
        {{ currentTarget.installation.last_error }}
      </p>

      <div class="drawer-actions">
        <UiButton
          v-if="!currentTarget.installation"
          variant="primary"
          data-action="install"
          :disabled="targetBusy || currentTarget.releases.length === 0 || row?.plugin?.official !== true"
          @click="emit('install', currentTarget)"
        >
          {{ row?.plugin?.official === true ? t('control.actions.install') : t('control.actions.installOfficialOnly') }}
        </UiButton>
        <template v-else>
          <UiButton
            data-action="configure"
            :disabled="targetBusy"
            @click="emit('configure', currentTarget)"
          >
            {{ t('control.actions.configure') }}
          </UiButton>
          <UiButton
            :variant="currentTarget.installation.enabled ? 'secondary' : 'primary'"
            :data-action="currentTarget.installation.enabled ? 'disable' : 'enable'"
            :disabled="targetBusy"
            @click="emitLifecycle(currentTarget.installation.enabled ? 'disable' : 'enable')"
          >
            {{ currentTarget.installation.enabled ? t('control.actions.disable') : t('control.actions.enable') }}
          </UiButton>
          <UiButton
            v-if="currentTarget.latestRelease?.version && currentTarget.latestRelease.version !== currentTarget.installation.desired_version"
            data-action="upgrade"
            :disabled="targetBusy"
            @click="emit('install', currentTarget)"
          >
            {{ t('control.actions.upgrade') }}
          </UiButton>
          <UiButton
            v-if="currentTarget.installation.previous_version"
            data-action="rollback"
            :disabled="targetBusy"
            @click="emitLifecycle('rollback')"
          >
            {{ t('control.actions.rollback') }}
          </UiButton>
        </template>
      </div>
    </section>
  </UiSheet>
</template>

<script setup>
// Plugin details as a side sheet (UiSheet: focus trap, Esc, focus return);
// it stays open while a target action is busy.
import { computed, ref, watch } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { UiButton, UiSheet } from '@/ui'

const props = defineProps({
  row: { type: Object, default: null },
  busyTarget: { default: '' },
  open: { type: Boolean, default: false },
})
const emit = defineEmits(['close', 'install', 'configure', 'lifecycle'])
const { t } = useAppI18n()
const selectedTarget = ref('')

const targets = computed(() => Array.isArray(props.row?.targets) ? props.row.targets : [])
const currentTarget = computed(() => targets.value.find(target => target.target === selectedTarget.value) || targets.value[0] || null)
const targetBusy = computed(() => Boolean(
  props.busyTarget === currentTarget.value?.target ||
  props.busyTarget?.target === currentTarget.value?.target
))
const canClose = computed(() => !props.busyTarget)
function requestClose() {
  if (canClose.value) emit('close')
}

watch([() => props.open, targets], () => {
  if (!targets.value.some(target => target.target === selectedTarget.value)) {
    selectedTarget.value = targets.value[0]?.target || ''
  }
}, { immediate: true })

function emitLifecycle(action) {
  if (!currentTarget.value?.installation) return
  emit('lifecycle', { installation: currentTarget.value.installation, action })
}
</script>

<style scoped>
.plugin-description, .health-error { margin: 0; color: var(--label-2); line-height: var(--type-body-line); overflow-wrap: anywhere; }
.health-error { color: var(--danger); }
.health-error strong { display: block; }
.target-tabs { display: flex; gap: var(--space-2); border-bottom: 1px solid var(--separator); }
.target-tab { min-height: var(--size-control-md); border: 0; border-bottom: 2px solid transparent; background: transparent; color: var(--label-2); cursor: pointer; font: inherit; text-transform: capitalize; }
.target-tab.active { border-bottom-color: var(--accent); color: var(--label-1); font-weight: var(--weight-bold); }
.target-tab:focus-visible { outline: var(--focus-ring); outline-offset: var(--focus-ring-offset); }
.target-detail { display: grid; gap: var(--space-5); }
.detail-list { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--space-4); margin: 0; }
.detail-list div { display: grid; gap: var(--space-1); }
.detail-list dt { color: var(--label-2); font-size: var(--type-caption-size); }
.detail-list dd { margin: 0; overflow-wrap: anywhere; }
.drawer-actions { display: flex; flex-wrap: wrap; gap: var(--space-2); }
@media (max-width: 720px) {
  .detail-list { grid-template-columns: 1fr; }
}
</style>
