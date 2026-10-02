<template>
  <UiSheet
    :open="open"
    size="lg"
    data-testid="plugin-detail-drawer"
    :title="row?.plugin?.name || row?.plugin?.id || ''"
    :dismissible="canClose"
    grouped
    @update:open="value => { if (!value) requestClose() }"
  >
    <template v-if="row?.plugin?.id" #description>
      <code>{{ row.plugin.id }}</code>
    </template>
    <template v-if="row?.health?.state" #header-actions>
      <UiBadge :tone="healthTone" :label="healthLabel" />
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
      <UiGroupedList :title="t('control.pluginCenter.detail.versions')">
        <UiGroupedListRow :label="t('control.table.release')">
          <code>{{ currentTarget.latestRelease?.version || '—' }}</code>
        </UiGroupedListRow>
        <UiGroupedListRow :label="t('control.labels.desired')">
          <code>{{ currentTarget.installation?.desired_version || '—' }}</code>
        </UiGroupedListRow>
        <UiGroupedListRow :label="t('control.labels.observed')">
          <code>{{ currentTarget.installation?.observed_version || '—' }}</code>
        </UiGroupedListRow>
        <UiGroupedListRow :label="t('control.table.state')" :value="currentTarget.installation?.state || t('control.states.catalogued')" />
        <UiGroupedListRow
          v-if="currentTarget.installation"
          :label="t('control.table.installation')"
          :value="currentTarget.installation.enabled ? t('control.states.enabled') : t('control.states.disabled')"
        />
      </UiGroupedList>

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
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiGroupedList from '@/ui/UiGroupedList.vue'
import UiGroupedListRow from '@/ui/UiGroupedListRow.vue'
import UiSheet from '@/ui/UiSheet.vue'

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
const healthTone = computed(() => ({ healthy: 'success', attention: 'warning' })[props.row?.health?.state] || 'neutral')
const healthLabel = computed(() => {
  const state = props.row?.health?.state
  if (state === 'healthy') return t('control.pluginCenter.states.healthy')
  if (state === 'attention') return t('control.pluginCenter.states.attention')
  return t('control.states.catalogued')
})
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
.health-error { padding: var(--space-3) var(--space-4); border-radius: var(--radius-sm); background: var(--danger-soft); color: color-mix(in srgb, var(--danger) 78%, var(--label-1)); }
.health-error strong { display: block; }
/* Targets as a pill switch (tablist semantics kept). */
.target-tabs { display: inline-flex; align-self: flex-start; gap: var(--space-0-5); padding: var(--space-0-5); border-radius: var(--radius-pill); background: var(--fill-1); }
.target-tab { min-height: var(--size-control-sm); padding: 0 var(--space-4); border: 0; border-radius: var(--radius-pill); background: transparent; color: var(--label-1); cursor: pointer; font: inherit; font-size: var(--type-callout-size); text-transform: capitalize; }
.target-tab:hover { background: var(--fill-2); }
.target-tab.active { background: var(--bg-elevated); box-shadow: var(--shadow-1); font-weight: var(--weight-semibold); }
.target-tab:focus-visible { outline: var(--focus-ring); outline-offset: var(--focus-ring-offset); }

/* Touch: 44 px tall hit areas on the target tabs. */
@media (pointer: coarse) {
  .target-tab { position: relative; }
  .target-tab::after { position: absolute; top: 50%; left: 0; width: 100%; height: var(--size-control-lg); transform: translateY(-50%); content: ''; }
}
.target-detail { display: grid; gap: var(--space-5); }
.target-detail code { font-family: var(--font-mono); }
.drawer-actions { display: flex; flex-wrap: wrap; gap: var(--space-2); }
</style>
