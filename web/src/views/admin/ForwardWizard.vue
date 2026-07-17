<template>
  <div class="wizard-page">
    <div class="wizard-header">
      <h1>{{ t('forwardWizard.title') }}</h1>
      <p class="wizard-subtitle">{{ t('forwardWizard.subtitle') }}</p>
    </div>

    <ol class="wizard-steps" :aria-label="t('forwardWizard.title')">
      <li
        v-for="(step, index) in wizardSteps"
        :key="step.key"
        class="wizard-step"
        :class="{ active: index === stepIndex, complete: index < stepIndex }"
        :aria-current="index === stepIndex ? 'step' : undefined"
      >
        <span class="wizard-step-index">{{ index + 1 }}</span>
        <span class="wizard-step-title">{{ step.title }}</span>
      </li>
    </ol>

    <div class="wizard-body">
      <StepForwardModeForm v-if="stepIndex === 0" @selected="handleModeSelected" />

      <StepMachineForm v-else-if="stepIndex === 1 && !selectedMode.nodeXMode" @created="handleNodeCreated" />
      <StepNodeForm v-else-if="stepIndex === 1 && selectedMode.nodeXMode" @created="handleNodeCreated" />

      <StepTunnelForm
        v-else-if="stepIndex === 2"
        :prefill-node-id="createdNode.id"
        :prefill-node-label="createdNode.name"
        :fixed-type="selectedMode.tunnelType"
        @created="handleTunnelCreated"
      />

      <StepForwardForm
        v-else-if="stepIndex === 3"
        :prefill-tunnel-id="createdTunnel.id"
        :prefill-tunnel-label="createdTunnel.name"
        @created="handleForwardCreated"
      />

      <div v-else-if="stepIndex === 4" class="wizard-done">
        <p class="wizard-done-summary">{{ t('forwardWizard.steps.done.summary', { name: createdForward.name }) }}</p>
        <ul class="wizard-done-links">
          <li><router-link to="/admin/forward">{{ t('forwardWizard.steps.done.gotoForward') }}</router-link></li>
          <li><router-link to="/admin/forward/tunnel">{{ t('forwardWizard.steps.done.gotoTunnel') }}</router-link></li>
          <li>
            <router-link :to="selectedMode.nodeXMode ? '/admin/forward/nodes' : '/admin/forward/ansible-machines'">
              {{ t('forwardWizard.steps.done.gotoNode') }}
            </router-link>
          </li>
        </ul>
        <div class="wizard-done-actions">
          <button class="btn btn-secondary" @click="createAnotherForward">
            {{ t('forwardWizard.steps.done.createAnother') }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, reactive, ref } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import StepForwardModeForm from '@/components/forward/StepForwardModeForm.vue'
import StepMachineForm from '@/components/forward/StepMachineForm.vue'
import StepNodeForm from '@/components/forward/StepNodeForm.vue'
import StepTunnelForm from '@/components/forward/StepTunnelForm.vue'
import StepForwardForm from '@/components/forward/StepForwardForm.vue'

const { t } = useAppI18n()

const stepIndex = ref(0)
const wizardSteps = computed(() => [
  { key: 'mode', title: t('forwardWizard.steps.mode.title') },
  { key: 'machine', title: t('forwardWizard.steps.machine.title') },
  { key: 'tunnel', title: t('forwardWizard.steps.tunnel.title') },
  { key: 'forward', title: t('forwardWizard.steps.forward.title') },
  { key: 'done', title: t('forwardWizard.steps.done.title') }
])

const selectedMode = reactive({ nodeXMode: false, tunnelType: 1 })
const createdNode = reactive({ id: 0, name: '' })
const createdTunnel = reactive({ id: 0, name: '' })
const createdForward = reactive({ id: 0, name: '' })

function handleModeSelected(payload) {
  selectedMode.nodeXMode = Boolean(payload?.nodeXMode)
  selectedMode.tunnelType = Number(payload?.tunnelType) === 2 ? 2 : 1
  stepIndex.value = 1
}

function handleNodeCreated(payload) {
  createdNode.id = Number(payload?.id || 0)
  createdNode.name = payload?.name || ''
  stepIndex.value = 2
}

function handleTunnelCreated(payload) {
  createdTunnel.id = Number(payload?.id || 0)
  createdTunnel.name = payload?.name || ''
  stepIndex.value = 3
}

function handleForwardCreated(payload) {
  createdForward.id = Number(payload?.id || 0)
  createdForward.name = payload?.name || ''
  stepIndex.value = 4
}

function createAnotherForward() {
  createdForward.id = 0
  createdForward.name = ''
  stepIndex.value = 3
}
</script>

<style scoped>
.wizard-page { display: flex; flex-direction: column; gap: 24px; padding: 24px; max-width: 880px; margin: 0 auto; }
.wizard-header h1 { margin: 0 0 8px; font-size: 22px; }
.wizard-subtitle { margin: 0; color: var(--text-secondary); line-height: 1.6; }
.wizard-steps {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 8px;
  margin: 8px 0;
  padding: 0;
  list-style: none;
}
.wizard-step {
  position: relative;
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  padding: 10px 12px;
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  background: var(--surface-muted);
  color: var(--text-secondary);
}
.wizard-step.active {
  border-color: var(--primary-color);
  background: var(--primary-soft);
  color: var(--text-color);
}
.wizard-step.complete {
  border-color: rgba(22, 163, 74, 0.28);
  background: rgba(22, 163, 74, 0.08);
  color: var(--text-color);
}
.wizard-step-index {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  width: 24px;
  height: 24px;
  border-radius: 50%;
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  font-size: 12px;
  font-weight: 700;
}
.wizard-step.active .wizard-step-index {
  background: var(--primary-color);
  border-color: var(--primary-color);
  color: #fff;
}
.wizard-step.complete .wizard-step-index {
  background: var(--success-color);
  border-color: var(--success-color);
  color: #fff;
}
.wizard-step-title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
  font-weight: 600;
}
.wizard-body { border: 1px solid var(--border-color); border-radius: 16px; padding: 20px; background: var(--surface-color); }
.wizard-loading { color: var(--text-secondary); text-align: center; padding: 40px 0; }
.wizard-done { display: flex; flex-direction: column; gap: 16px; }
.wizard-done-summary { margin: 0; font-size: 16px; }
.wizard-done-links { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 8px; }
.wizard-done-actions { display: flex; justify-content: flex-end; }
.btn { display: inline-flex; align-items: center; justify-content: center; border: 1px solid transparent; border-radius: 12px; padding: 10px 16px; cursor: pointer; }
.btn-secondary { background: var(--surface-color); color: var(--text-color); border-color: var(--border-color); }
@media (max-width: 760px) {
  .wizard-page { padding: 16px; }
  .wizard-steps { grid-template-columns: 1fr; }
  .wizard-step-title { white-space: normal; }
}
</style>
