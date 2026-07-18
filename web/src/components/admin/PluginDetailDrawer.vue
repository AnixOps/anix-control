<template>
  <div v-if="open" class="plugin-detail-backdrop" @click.self="requestClose">
    <aside
      class="plugin-detail-drawer"
      data-testid="plugin-detail-drawer"
      role="dialog"
      aria-modal="true"
      aria-labelledby="plugin-detail-title"
      @keydown.esc.prevent="requestClose"
    >
      <header class="drawer-header">
        <div>
          <h2 id="plugin-detail-title">{{ row?.plugin?.name || row?.plugin?.id }}</h2>
          <code v-if="row?.plugin?.id">{{ row.plugin.id }}</code>
        </div>
        <button
          ref="closeButton"
          class="icon-button"
          data-testid="plugin-detail-close"
          type="button"
          :aria-label="t('common.actions.close')"
          :title="t('common.actions.close')"
          :disabled="!canClose"
          @click="requestClose"
        >
          <X :size="20" aria-hidden="true" />
          <span class="sr-only">{{ t('common.actions.close') }}</span>
        </button>
      </header>

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
        </dl>

        <p v-if="currentTarget.installation?.last_error" class="health-error" role="alert">
          <strong>{{ t('control.errors.action') }}</strong>
          {{ currentTarget.installation.last_error }}
        </p>

        <div class="drawer-actions">
          <button
            v-if="!currentTarget.installation"
            class="btn btn-primary"
            data-action="install"
            type="button"
            :disabled="targetBusy || currentTarget.releases.length === 0"
            @click="emit('install', currentTarget)"
          >
            {{ t('control.actions.install') }}
          </button>
          <template v-else>
            <button
              class="btn"
              data-action="configure"
              type="button"
              :disabled="targetBusy"
              @click="emit('configure', currentTarget)"
            >
              {{ t('control.actions.configure') }}
            </button>
            <button
              :class="['btn', currentTarget.installation.enabled ? 'btn-danger' : 'btn-primary']"
              :data-action="currentTarget.installation.enabled ? 'disable' : 'enable'"
              type="button"
              :disabled="targetBusy"
              @click="emitLifecycle(currentTarget.installation.enabled ? 'disable' : 'enable')"
            >
              {{ currentTarget.installation.enabled ? t('control.actions.disable') : t('control.actions.enable') }}
            </button>
            <button
              v-if="currentTarget.installation.previous_version"
              class="btn"
              data-action="rollback"
              type="button"
              :disabled="targetBusy"
              @click="emitLifecycle('rollback')"
            >
              {{ t('control.actions.rollback') }}
            </button>
          </template>
        </div>
      </section>
    </aside>
  </div>
</template>

<script setup>
import { computed, nextTick, ref, watch } from 'vue'
import { X } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'

const props = defineProps({
  row: { type: Object, default: null },
  busyTarget: { default: '' },
  open: { type: Boolean, default: false },
})
const emit = defineEmits(['close', 'install', 'configure', 'lifecycle'])
const { t } = useAppI18n()
const closeButton = ref(null)
const selectedTarget = ref('')

const targets = computed(() => Array.isArray(props.row?.targets) ? props.row.targets : [])
const currentTarget = computed(() => targets.value.find(target => target.target === selectedTarget.value) || targets.value[0] || null)
const targetBusy = computed(() => Boolean(
  props.busyTarget === currentTarget.value?.target ||
  props.busyTarget?.target === currentTarget.value?.target
))
const canClose = computed(() => !props.busyTarget)

watch([() => props.open, targets], async ([isOpen]) => {
  if (!targets.value.some(target => target.target === selectedTarget.value)) {
    selectedTarget.value = targets.value[0]?.target || ''
  }
  if (isOpen) {
    await nextTick()
    closeButton.value?.focus()
  }
}, { immediate: true })

function requestClose() {
  if (canClose.value) emit('close')
}

function emitLifecycle(action) {
  if (!currentTarget.value?.installation) return
  emit('lifecycle', { installation: currentTarget.value.installation, action })
}
</script>

<style scoped>
.plugin-detail-backdrop { position: fixed; inset: 0; z-index: 1200; display: flex; justify-content: flex-end; background: rgba(15, 23, 42, .62); }
.plugin-detail-drawer { width: min(100%, 560px); min-height: 100%; overflow-y: auto; background: var(--surface-color); border-left: 1px solid var(--border-color); color: var(--text-color); box-shadow: -12px 0 28px rgba(15, 23, 42, .18); }
.drawer-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; padding: 20px; border-bottom: 1px solid var(--border-color); }
.drawer-header h2 { margin: 0; font-size: 20px; line-height: 1.3; }
.drawer-header code { display: inline-block; margin-top: 4px; color: var(--text-secondary); }
.icon-button { display: inline-grid; width: 36px; height: 36px; place-items: center; flex: 0 0 auto; border: 1px solid var(--border-color); border-radius: 6px; background: transparent; color: var(--text-color); cursor: pointer; }
.icon-button:disabled { cursor: not-allowed; opacity: .55; }
.plugin-description, .health-error { margin: 16px 20px 0; color: var(--text-secondary); line-height: 1.5; overflow-wrap: anywhere; }
.health-error { color: var(--error-color); }
.health-error strong { display: block; }
.target-tabs { display: flex; gap: 6px; padding: 20px 20px 0; border-bottom: 1px solid var(--border-color); }
.target-tab { min-height: 36px; border: 0; border-bottom: 2px solid transparent; background: transparent; color: var(--text-secondary); cursor: pointer; font: inherit; text-transform: capitalize; }
.target-tab.active { border-bottom-color: var(--primary-color); color: var(--text-color); font-weight: 700; }
.target-detail { display: grid; gap: 20px; padding: 20px; }
.detail-list { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; margin: 0; }
.detail-list div { display: grid; gap: 4px; }
.detail-list dt { color: var(--text-secondary); font-size: 12px; }
.detail-list dd { margin: 0; overflow-wrap: anywhere; }
.drawer-actions { display: flex; flex-wrap: wrap; gap: 8px; }
.btn { min-height: 36px; border: 1px solid var(--border-color); border-radius: 6px; background: var(--surface-color); color: var(--text-color); cursor: pointer; padding: 8px 12px; }
.btn-primary { border-color: var(--primary-color); background: var(--primary-color); color: #fff; }
.btn-danger { border-color: var(--error-color); color: var(--error-color); }
.btn:disabled { cursor: not-allowed; opacity: .55; }
.sr-only { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip: rect(0, 0, 0, 0); white-space: nowrap; border: 0; }
@media (max-width: 720px) {
  .plugin-detail-drawer { width: 100vw; min-height: 100dvh; border-left: 0; }
  .detail-list { grid-template-columns: 1fr; }
}
</style>
