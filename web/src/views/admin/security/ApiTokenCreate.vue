<template>
  <span class="api-token-create" data-testid="api-token-create">
    <UiButton
      ref="triggerRef"
      variant="primary"
      :icon="Plus"
      :disabled="disabled"
      data-testid="api-token-create-open"
      @click="create.open"
    >{{ t('adminApiTokens.create.action') }}</UiButton>

    <UiDialog
      :open="create.stage.value === 'form'"
      size="md"
      :title="t('adminApiTokens.create.title')"
      :description="t('adminApiTokens.create.description')"
      :dismissible="!create.busy.value"
      data-testid="api-token-form-dialog"
      @update:open="onFormOpenChange"
    >
      <form id="api-token-form" class="api-token-form" novalidate @submit.prevent="submit()">
        <UiTextField
          id="api-token-name"
          ref="nameRef"
          v-model="create.form.name"
          :label="t('adminApiTokens.create.name')"
          :placeholder="t('adminApiTokens.create.namePlaceholder')"
          :help="t('adminApiTokens.create.nameHelp', { max: API_TOKEN_NAME_MAX })"
          :error="create.nameError.value"
          :disabled="create.busy.value"
          autocomplete="off"
          spellcheck="false"
          required
          data-testid="api-token-name"
        />

        <UiRadioGroup
          v-model="create.form.scope"
          :label="t('adminApiTokens.create.scope')"
          :help="t('adminApiTokens.create.scopeHelp')"
          :options="scopeOptions"
          :disabled="create.busy.value"
          data-testid="api-token-scope"
        />

        <div class="api-token-form__group">
          <UiSelect
            id="api-token-expiry"
            v-model="create.form.expiry"
            :label="t('adminApiTokens.create.expiry')"
            :help="t('adminApiTokens.create.expiryHelp')"
            :error="create.form.expiry === 'custom' ? '' : create.expiryError.value"
            :options="expiryOptions"
            :disabled="create.busy.value"
            data-testid="api-token-expiry"
          />
          <UiTextField
            v-if="create.form.expiry === 'custom'"
            id="api-token-custom-days"
            ref="daysRef"
            v-model="create.form.customDays"
            type="number"
            inputmode="numeric"
            min="1"
            :max="API_TOKEN_MAX_DAYS"
            :label="t('adminApiTokens.create.customDays')"
            :help="t('adminApiTokens.create.customDaysHelp', { max: API_TOKEN_MAX_DAYS })"
            :suffix="t('adminApiTokens.create.daysUnit')"
            :error="create.expiryError.value"
            :disabled="create.busy.value"
            required
            data-testid="api-token-custom-days"
          />
          <NodeNotice v-if="create.neverExpires.value" tone="warning" data-testid="api-token-never-notice">
            {{ t('adminApiTokens.create.neverNotice') }}
          </NodeNotice>
        </div>

        <div class="api-token-form__group" role="group" aria-labelledby="api-token-confirm-title" data-testid="api-token-credential">
          <h3 id="api-token-confirm-title" class="api-token-form__heading">{{ t('adminApiTokens.create.confirmTitle') }}</h3>
          <p v-if="create.step.value === 'checking'" class="api-token-form__note" role="status">{{ t('adminApiTokens.create.checking') }}</p>
          <template v-else-if="create.step.value === 'password'">
            <p class="api-token-form__note">{{ t('adminApiTokens.create.passwordNote') }}</p>
            <UiPasswordField
              ref="passwordRef"
              v-model="create.form.password"
              :label="t('adminApiTokens.create.password')"
              :error="create.credentialError.value"
              :disabled="create.busy.value"
              required
              data-testid="api-token-password"
            />
          </template>
          <template v-else-if="create.step.value === 'code'">
            <p class="api-token-form__note">{{ t('adminApiTokens.create.codeNote') }}</p>
            <UiOtpField
              ref="otpRef"
              v-model="create.form.code"
              :label="t('adminApiTokens.create.code')"
              :error="create.credentialError.value"
              :disabled="create.busy.value"
              data-testid="api-token-code"
              @complete="submit"
            />
            <UiButton variant="tertiary" size="sm" class="api-token-form__switch" data-testid="api-token-use-recovery" @click="switchStep('recovery')">
              {{ t('adminApiTokens.create.useRecovery') }}
            </UiButton>
          </template>
          <template v-else>
            <p class="api-token-form__note">{{ t('adminApiTokens.create.recoveryNote') }}</p>
            <UiTextField
              id="api-token-recovery"
              ref="recoveryRef"
              v-model="create.form.recovery"
              :label="t('adminApiTokens.create.recovery')"
              placeholder="XXXX-XXXX"
              :error="create.credentialError.value"
              :disabled="create.busy.value"
              autocomplete="one-time-code"
              autocapitalize="characters"
              spellcheck="false"
              maxlength="9"
              class="api-token-form__recovery"
              aria-required="true"
              data-testid="api-token-recovery"
              @blur="create.form.recovery = formatRecoveryCode(create.form.recovery)"
            />
            <UiButton variant="tertiary" size="sm" class="api-token-form__switch" data-testid="api-token-use-code" @click="switchStep('code')">
              {{ t('adminApiTokens.create.useCode') }}
            </UiButton>
          </template>
        </div>

        <div v-if="create.error.value" class="api-token-form__error" role="alert" data-testid="api-token-error">
          <p>{{ create.error.value }}</p>
          <UiButton v-if="create.signInAgain.value" size="sm" data-testid="api-token-sign-in" @click="signInAgain">
            {{ t('adminApiTokens.create.signInAgain') }}
          </UiButton>
        </div>
      </form>
      <template #footer="{ close }">
        <UiButton :disabled="create.busy.value" data-testid="api-token-cancel" @click="close">{{ t('adminApiTokens.create.cancel') }}</UiButton>
        <UiButton
          variant="primary"
          type="submit"
          form="api-token-form"
          :loading="create.busy.value"
          :disabled="create.step.value === 'checking'"
          data-testid="api-token-submit"
        >{{ t('adminApiTokens.create.submit') }}</UiButton>
      </template>
    </UiDialog>

    <UiDialog
      :open="create.stage.value === 'result'"
      size="md"
      :title="t('adminApiTokens.result.title')"
      :description="t('adminApiTokens.result.description')"
      :close-on-scrim="false"
      initial-focus=".ui-copy__button"
      data-testid="api-token-result"
      @update:open="onResultOpenChange"
      @close-auto-focus="restoreFocus"
    >
      <div v-if="result" class="api-token-form">
        <NodeNotice tone="warning" data-testid="api-token-warning">{{ t('adminApiTokens.result.warning') }}</NodeNotice>
        <UiCopyField
          :value="result.token"
          :label="t('adminApiTokens.result.label')"
          :copy-label="t('adminApiTokens.result.copy')"
          secret
          size="md"
          data-testid="api-token-secret"
        />
        <dl class="api-token-facts" data-testid="api-token-facts">
          <div>
            <dt>{{ t('adminApiTokens.result.name') }}</dt>
            <dd>{{ result.record.name }}</dd>
          </div>
          <div>
            <dt>{{ t('adminApiTokens.result.scope') }}</dt>
            <dd>{{ t(`adminApiTokens.scopes.${result.record.scope}`) }}</dd>
          </div>
          <div>
            <dt>{{ t('adminApiTokens.result.expires') }}</dt>
            <dd>{{ result.record.expiresAt ? format.dateTime(result.record.expiresAt) : t('adminApiTokens.result.never') }}</dd>
          </div>
        </dl>
        <div class="api-token-form__group">
          <h3 class="api-token-form__heading">{{ t('adminApiTokens.result.usageTitle') }}</h3>
          <p class="api-token-form__note">{{ t('adminApiTokens.result.usageIntro') }}</p>
          <UiCodeBlock :code="usageExample" :label="t('adminApiTokens.result.usageLabel')" max-height="" wrap data-testid="api-token-usage" />
          <p class="api-token-form__note" data-testid="api-token-no-url">{{ t('adminApiTokens.result.usageNever') }}</p>
        </div>
      </div>
      <template #footer="{ close }">
        <UiButton variant="primary" data-testid="api-token-done" @click="close">{{ t('adminApiTokens.result.done') }}</UiButton>
      </template>
    </UiDialog>
  </span>
</template>

<script setup>
// "Create token" for the API tokens page: a form (name, scope, expiry and the
// re-authentication the route needs: the password, or with two-step
// verification on a six-digit code or a recovery code), then a dialog that
// shows the new `anixadm_` token once.
//
// The token lives in useApiTokenCreate's `result` while the result dialog is
// open and is cleared when it closes (Done, Esc, ×) and when this component
// goes: it is never stored, put in the URL or logged, and the page is not told
// it (`created` carries the stored record, which has the last four characters
// only). The usage example shows `$ANIXOPS_TOKEN`, never the token, so the
// secret is in one place on screen: the masked, copyable field.
import { computed, inject, nextTick, ref } from 'vue'
import { routerKey } from 'vue-router'
import { Plus } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { useUserStore } from '@/stores/user'
import UiButton from '@/ui/UiButton.vue'
import UiCodeBlock from '@/ui/UiCodeBlock.vue'
import UiCopyField from '@/ui/UiCopyField.vue'
import UiDialog from '@/ui/UiDialog.vue'
import UiOtpField from '@/ui/UiOtpField.vue'
import UiPasswordField from '@/ui/UiPasswordField.vue'
import UiRadioGroup from '@/ui/UiRadioGroup.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiTextField from '@/ui/UiTextField.vue'
import { useFormat } from '@/ui/composables/useFormat'
import NodeNotice from '@/views/admin/nodes/NodeNotice.vue'
import { API_TOKEN_MAX_DAYS, API_TOKEN_NAME_MAX, EXPIRY_CHOICES, SCOPES, formatRecoveryCode } from './apiTokens'
import { useApiTokenCreate } from './useApiTokenCreate'

defineProps({
  // The limit of 25 active tokens is reached (the page knows from the list).
  disabled: { type: Boolean, default: false }
})
const emit = defineEmits(['created'])

const { t } = useAppI18n()
const format = useFormat()
const router = inject(routerKey, null)
const userStore = useUserStore()

const create = useApiTokenCreate({ t, onCreated: record => emit('created', record) })
const result = computed(() => create.result.value)

const scopeOptions = computed(() => SCOPES.map(value => ({
  value,
  label: t(`adminApiTokens.scopes.${value}`),
  description: t(`adminApiTokens.create.scopeText.${value}`)
})))
// The preset days are keyed d30 … d730 (a numeric segment is not a safe message path).
const expiryOptions = computed(() => EXPIRY_CHOICES.map(value => ({
  value,
  label: t(`adminApiTokens.create.expiryOptions.${/^\d+$/.test(value) ? `d${value}` : value}`)
})))

// A request that shows how to call the API without the token in it.
const usageExample = computed(() => {
  const origin = typeof window !== 'undefined' ? window.location.origin : 'https://panel.example.com'
  return `curl -sS ${origin}/api/v3/plugins \\\n  -H "Authorization: Bearer $ANIXOPS_TOKEN"`
})

const nameRef = ref(null)
const daysRef = ref(null)
const passwordRef = ref(null)
const otpRef = ref(null)
const recoveryRef = ref(null)

function focusStep() {
  nextTick(() => {
    if (create.step.value === 'password') passwordRef.value?.focus()
    else if (create.step.value === 'code') otpRef.value?.focus()
    else if (create.step.value === 'recovery') recoveryRef.value?.focus()
  })
}

function focusField(name) {
  if (name === 'name') nameRef.value?.focus()
  else if (name === 'expiry') nextTick(() => (daysRef.value ? daysRef.value.focus() : document.getElementById('api-token-expiry')?.focus()))
  else if (name === 'credential') {
    // A refused code starts over from the first box.
    if (create.step.value === 'code') nextTick(() => otpRef.value?.clear?.())
    else focusStep()
  }
}

async function submit(value) {
  const outcome = await create.submit(value)
  if (!outcome.ok && outcome.focus) focusField(outcome.focus)
}

function switchStep(step) {
  create.switchStep(step)
  focusStep()
}

function onFormOpenChange(open) {
  if (!open) create.cancel()
}

function onResultOpenChange(open) {
  if (!open) create.dismiss()
}

// The sign-in the route asks for: an identity-held credential is checked at
// sign-in, so the way to a fresh one is to sign out and in again.
function signInAgain() {
  create.cancel()
  userStore.logout()
  router?.push('/login')
}

// The result dialog opens from the form, which is gone by then, so the browser
// has nothing to return focus to: give it back to the button.
const triggerRef = ref(null)
function restoreFocus(event) {
  const button = triggerRef.value?.$el
  if (button && typeof button.focus === 'function') {
    event.preventDefault()
    button.focus()
  }
}

// The empty state's button opens the same form; focus comes back to the header
// button when it closes.
function open() {
  create.open()
}
defineExpose({ open })
</script>

<style scoped>
.api-token-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
}

.api-token-form__group {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.api-token-form__heading {
  margin: 0;
  font-size: var(--type-callout-size);
  font-weight: var(--weight-semibold);
}

.api-token-form__note {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
}

.api-token-form__switch {
  align-self: flex-start;
}

.api-token-form__recovery :deep(input) {
  font-family: var(--font-mono);
  letter-spacing: 0.06em;
  text-transform: uppercase;
}

.api-token-form__error {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  align-items: flex-start;
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-md);
  background: var(--danger-soft);
  color: var(--label-1);
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
}

.api-token-form__error p {
  margin: 0;
}

.api-token-facts {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: var(--space-3);
  margin: 0;
}

.api-token-facts div {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  min-width: 0;
  padding: var(--space-3);
  border-radius: var(--radius-sm);
  background: var(--bg-grouped);
}

.api-token-facts dt {
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

.api-token-facts dd {
  margin: 0;
  color: var(--label-1);
  font-size: var(--type-callout-size);
  font-weight: var(--weight-semibold);
  overflow-wrap: anywhere;
}
</style>
