<template>
  <div class="wizard-page">
    <UiPageHeader :title="t('forwardWizard.title')" :description="t('forwardWizard.subtitle')" />

    <div class="wizard-layout">
      <nav class="wizard-nav" :aria-label="t('forwardWizard.stepsLabel')">
        <ol class="wizard-steps">
          <li
            v-for="(step, index) in wizardSteps"
            :key="step.key"
            class="wizard-step"
            :class="{ active: index === stepIndex, complete: index < stepIndex }"
            :aria-current="index === stepIndex ? 'step' : undefined"
          >
            <span class="wizard-step-index" aria-hidden="true">
              <UiIcon v-if="index < stepIndex" :icon="Check" :size="14" />
              <template v-else>{{ index + 1 }}</template>
            </span>
            <span class="wizard-step-title">
              {{ step.title }}
              <span v-if="index < stepIndex" class="visually-hidden">{{ t('forwardWizard.stepDone') }}</span>
            </span>
          </li>
        </ol>
      </nav>

      <section class="wizard-panel" aria-labelledby="wizard-step-heading">
        <header class="wizard-panel-head">
          <p class="wizard-panel-count">{{ t('forwardWizard.stepCount', { current: stepIndex + 1, total: wizardSteps.length }) }}</p>
          <h2 id="wizard-step-heading" ref="headingRef" class="wizard-panel-title" tabindex="-1">{{ wizardSteps[stepIndex].title }}</h2>
        </header>

        <div v-if="createdNode.apiToken" class="wizard-token-notice" data-testid="wizard-node-token">
          <p class="wizard-token-text">{{ t('forwardWizard.steps.node.tokenNotice', { name: createdNode.name }) }}</p>
          <UiCopyField :value="createdNode.apiToken" :label="t('runtime.nodeXTopology.nodeModal.fields.apiToken')" size="md" />
        </div>

        <div class="wizard-body">
          <StepForwardModeForm v-if="stepIndex === 0" ref="stepRef" @selected="handleModeSelected" />

          <StepMachineForm v-else-if="stepIndex === 1 && !selectedMode.nodeXMode" ref="stepRef" @created="handleNodeCreated" />
          <StepNodeForm v-else-if="stepIndex === 1 && selectedMode.nodeXMode" ref="stepRef" @created="handleNodeCreated" />

          <StepTunnelForm
            v-else-if="stepIndex === 2"
            ref="stepRef"
            :prefill-node-id="createdNode.id"
            :prefill-node-label="createdNode.name"
            :fixed-type="selectedMode.tunnelType"
            @created="handleTunnelCreated"
          />

          <StepForwardForm
            v-else-if="stepIndex === 3"
            ref="stepRef"
            :prefill-tunnel-id="createdTunnel.id"
            :prefill-tunnel-label="createdTunnel.name"
            @created="handleForwardCreated"
          />

          <div v-else-if="stepIndex === 4" class="wizard-done">
            <p class="wizard-done-summary">
              <UiIcon class="wizard-done-icon" :icon="CircleCheck" :size="24" />
              <span>{{ t('forwardWizard.steps.done.summary', { name: createdForward.name }) }}</span>
            </p>
            <h3 class="wizard-done-next">{{ t('forwardWizard.steps.done.nextTitle') }}</h3>
            <ul class="wizard-done-links">
              <li><router-link to="/admin/forward">{{ t('forwardWizard.steps.done.gotoForward') }}</router-link></li>
              <li><router-link to="/admin/forward/tunnel">{{ t('forwardWizard.steps.done.gotoTunnel') }}</router-link></li>
              <li>
                <router-link :to="selectedMode.nodeXMode ? '/admin/forward/nodes' : '/admin/forward/ansible-machines'">
                  {{ t('forwardWizard.steps.done.gotoNode') }}
                </router-link>
              </li>
            </ul>
          </div>
        </div>

        <footer class="wizard-footer">
          <UiButton
            v-if="stepIndex > 0 && stepIndex < 4"
            :icon="ArrowLeft"
            :disabled="stepBusy"
            data-test="wizard-back"
            @click="goBack"
          >
            {{ t('forwardWizard.back') }}
          </UiButton>
          <span v-else />
          <UiButton
            v-if="stepIndex < 4"
            type="submit"
            form="wizard-step-form"
            variant="primary"
            :icon-end="ArrowRight"
            :loading="stepBusy"
            :disabled="!stepCanSubmit"
            data-test="wizard-next"
          >
            {{ stepPrimaryLabel }}
          </UiButton>
          <UiButton v-else variant="primary" :icon="Plus" data-test="wizard-create-another" @click="createAnotherForward">
            {{ t('forwardWizard.steps.done.createAnother') }}
          </UiButton>
        </footer>
      </section>
    </div>
  </div>
</template>

<script setup>
// 快速配置向导 (/admin/forward/setup), on the wizard template of the
// redesign (plan §7.5, UI U7): the five steps on the left, the current
// step's form on the right, 上一步 / 下一步 at the bottom. Each step keeps
// its fields and API calls; it exposes submit / busy / canSubmit /
// primaryLabel and its form has the id "wizard-step-form", so the footer's
// primary button submits it (Enter in a field does the same).
import { computed, nextTick, reactive, ref, watch } from 'vue'
import { ArrowLeft, ArrowRight, Check, CircleCheck, Plus } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'
import UiButton from '@/ui/UiButton.vue'
import UiCopyField from '@/ui/UiCopyField.vue'
import UiIcon from '@/ui/UiIcon.vue'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import StepForwardModeForm from '@/components/forward/StepForwardModeForm.vue'
import StepMachineForm from '@/components/forward/StepMachineForm.vue'
import StepNodeForm from '@/components/forward/StepNodeForm.vue'
import StepTunnelForm from '@/components/forward/StepTunnelForm.vue'
import StepForwardForm from '@/components/forward/StepForwardForm.vue'

const { t } = useAppI18n()

const stepIndex = ref(0)
const stepRef = ref(null)
const headingRef = ref(null)
const wizardSteps = computed(() => [
  { key: 'mode', title: t('forwardWizard.steps.mode.title') },
  { key: 'machine', title: t('forwardWizard.steps.machine.title') },
  { key: 'tunnel', title: t('forwardWizard.steps.tunnel.title') },
  { key: 'forward', title: t('forwardWizard.steps.forward.title') },
  { key: 'done', title: t('forwardWizard.steps.done.title') }
])

const selectedMode = reactive({ nodeXMode: false, tunnelType: 1 })
const createdNode = reactive({ id: 0, name: '', apiToken: '' })
const createdTunnel = reactive({ id: 0, name: '' })
const createdForward = reactive({ id: 0, name: '' })

const stepBusy = computed(() => Boolean(stepRef.value?.busy))
const stepCanSubmit = computed(() => Boolean(stepRef.value?.canSubmit))
const stepPrimaryLabel = computed(() => stepRef.value?.primaryLabel || t('forwardWizard.next'))

// A new step moves focus to its heading, so screen readers hear where they are.
watch(stepIndex, async () => {
  await nextTick()
  headingRef.value?.focus?.()
})

function handleModeSelected(payload) {
  selectedMode.nodeXMode = Boolean(payload?.nodeXMode)
  selectedMode.tunnelType = Number(payload?.tunnelType) === 2 ? 2 : 1
  stepIndex.value = 1
}

function handleNodeCreated(payload) {
  createdNode.id = Number(payload?.id || 0)
  createdNode.name = payload?.name || ''
  // A generated API token is shown once, when the node is created.
  createdNode.apiToken = payload?.apiToken || ''
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

// 上一步 shows the previous step again; what was already created stays and
// can be picked from its "使用现有" list.
function goBack() {
  if (stepIndex.value > 0) stepIndex.value -= 1
}

function createAnotherForward() {
  createdForward.id = 0
  createdForward.name = ''
  stepIndex.value = 3
}
</script>

<style scoped>
.wizard-page {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
  min-width: 0;
}

.wizard-layout {
  display: grid;
  grid-template-columns: 240px minmax(0, 1fr);
  gap: var(--space-6);
  align-items: start;
}

.wizard-nav {
  position: sticky;
  top: calc(var(--shell-topbar-height, 52px) + var(--space-4));
}

.wizard-steps {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  margin: 0;
  padding: 0;
  list-style: none;
}

.wizard-step {
  position: relative;
  display: flex;
  gap: var(--space-3);
  align-items: center;
  min-width: 0;
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-md);
  color: var(--label-2);
}

.wizard-step.active {
  background: var(--fill-1);
  color: var(--label-1);
  font-weight: var(--weight-medium);
}

.wizard-step.complete {
  color: var(--label-1);
}

.wizard-step-index {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border-radius: 50%;
  box-shadow: inset 0 0 0 1px var(--label-3);
  font-size: var(--type-caption-size);
  font-variant-numeric: tabular-nums;
  font-weight: var(--weight-semibold);
}

.wizard-step.active .wizard-step-index {
  background: var(--accent-fill);
  box-shadow: none;
  color: var(--on-accent);
}

.wizard-step.complete .wizard-step-index {
  background: var(--success);
  box-shadow: none;
  color: var(--on-accent);
}

.wizard-step-title {
  min-width: 0;
  font-size: var(--type-callout-size);
  overflow-wrap: anywhere;
}

.wizard-panel {
  display: flex;
  flex-direction: column;
  min-width: 0;
  border-radius: var(--radius-lg);
  background: var(--bg-elevated);
  box-shadow: var(--shadow-1), 0 0 0 0.5px var(--separator);
}

.wizard-panel-head {
  padding: var(--space-6) var(--space-6) 0;
}

.wizard-panel-count {
  margin: 0 0 var(--space-1);
  color: var(--label-2);
  font-size: var(--type-caption-size);
  font-variant-numeric: tabular-nums;
}

.wizard-panel-title {
  margin: 0;
  color: var(--label-1);
  font-size: var(--type-title-3-size);
  font-weight: var(--weight-semibold);
  line-height: var(--type-title-3-line);
}

.wizard-panel-title:focus {
  outline: none;
}

.wizard-panel-title:focus-visible {
  border-radius: var(--radius-xs);
  outline: var(--focus-ring);
  outline-offset: var(--focus-ring-offset);
}

.wizard-token-notice {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  margin: var(--space-5) var(--space-6) 0;
  padding: var(--space-4);
  border-radius: var(--radius-md);
  background: var(--warning-soft);
}

.wizard-token-text {
  margin: 0;
  color: var(--label-1);
  font-size: var(--type-callout-size);
  overflow-wrap: anywhere;
}

.wizard-body {
  padding: var(--space-5) var(--space-6) var(--space-6);
}

.wizard-footer {
  position: sticky;
  bottom: 0;
  display: flex;
  gap: var(--space-3);
  align-items: center;
  justify-content: space-between;
  padding: var(--space-4) var(--space-6) calc(var(--space-4) + env(safe-area-inset-bottom, 0px));
  border-top: 0.5px solid var(--separator);
  border-radius: 0 0 var(--radius-lg) var(--radius-lg);
  background: var(--bg-elevated);
}

.wizard-done {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.wizard-done-summary {
  display: flex;
  gap: var(--space-3);
  align-items: center;
  margin: 0;
  color: var(--label-1);
  font-size: var(--type-body-size);
}

.wizard-done-icon {
  flex: none;
  color: var(--success);
}

.wizard-done-next {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
  font-weight: var(--weight-semibold);
}

.wizard-done-links {
  display: flex;
  flex-direction: column;
  margin: 0;
  padding: 0;
  border-radius: var(--radius-md);
  background: var(--fill-1);
  list-style: none;
}

.wizard-done-links li + li {
  border-top: 0.5px solid var(--separator);
}

.wizard-done-links a {
  display: flex;
  align-items: center;
  min-height: 44px;
  padding: 0 var(--space-4);
  color: var(--accent);
  font-weight: var(--weight-medium);
  text-decoration: none;
}

.wizard-done-links a:hover {
  text-decoration: underline;
}

.wizard-done-links a:focus-visible {
  border-radius: var(--radius-md);
  outline: var(--focus-ring);
  outline-offset: calc(-1 * var(--focus-ring-offset, 2px));
}

/* Below 834 px the steps become a compact row above the form. */
@media (max-width: 833.98px) {
  .wizard-layout {
    grid-template-columns: minmax(0, 1fr);
    gap: var(--space-4);
  }

  .wizard-nav {
    position: static;
  }

  .wizard-steps {
    display: grid;
    grid-template-columns: repeat(5, minmax(0, 1fr));
  }

  .wizard-step {
    flex-direction: column;
    gap: var(--space-1);
    padding: var(--space-2) var(--space-1);
    text-align: center;
  }

  .wizard-step-title {
    font-size: var(--type-caption-size);
  }
}

@media (max-width: 639.98px) {
  /* Only the numbers; the current step's name is the panel heading. */
  .wizard-step-title {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip: rect(0 0 0 0);
    white-space: nowrap;
  }

  .wizard-panel-head,
  .wizard-body {
    padding-inline: var(--space-4);
  }

  .wizard-token-notice {
    margin-inline: var(--space-4);
  }

  .wizard-footer {
    padding: var(--space-3) var(--space-4);
  }
}
</style>
