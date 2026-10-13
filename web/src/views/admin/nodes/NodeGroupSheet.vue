<template>
  <UiSheet
    :open="open"
    size="lg"
    :title="group ? t('admin.nodes.groupForm.titleEdit') : t('admin.nodes.groupForm.titleCreate')"
    :description="group ? group.name : t('admin.nodes.groupForm.createDescription')"
    :dismissible="!saving"
    data-testid="node-group-sheet"
    @update:open="value => { if (!value) close() }"
  >
    <form class="group-form" novalidate @submit.prevent="save">
      <section class="dialog-section">
        <div class="form-grid">
          <UiTextField
            v-model="form.name"
            class="form-grid__full"
            size="md"
            required
            :label="t('admin.nodes.groupForm.fields.name')"
            :placeholder="t('admin.nodes.groupForm.placeholders.name')"
            :error="errors.name"
            data-testid="group-name"
            @blur="validateName"
          />
          <UiSegmentedControl
            v-if="!group"
            v-model="form.kind"
            class="form-grid__full"
            :aria-label="t('admin.nodes.groupForm.fields.kind')"
            :options="kindOptions"
            data-testid="group-kind"
          />
          <UiNumberField
            v-model="form.rate"
            size="md"
            :min="0"
            :step="0.1"
            :format-options="{ useGrouping: false, maximumFractionDigits: 2 }"
            :label="t('admin.nodes.groupForm.fields.rate')"
            :help="t('admin.nodes.groupForm.help.rate')"
            data-testid="group-rate"
          />
          <UiNumberField v-model="form.sort" size="md" :label="t('admin.nodes.groupForm.fields.sort')" :help="t('admin.nodes.groupForm.help.sort')" />
          <UiTextField
            v-model="form.tagsText"
            class="form-grid__full"
            size="md"
            :label="t('admin.nodes.groupForm.fields.tags')"
            :placeholder="t('admin.nodes.groupForm.placeholders.tags')"
          />
          <UiNumberField
            v-model="form.monthlyLimitGb"
            size="md"
            :min="0"
            :step="0.1"
            :format-options="{ useGrouping: false, maximumFractionDigits: 2 }"
            unit="GB"
            :label="t('admin.nodes.groupForm.fields.monthlyLimit')"
            :help="t('admin.nodes.groupForm.help.monthlyLimit')"
          />
          <UiNumberField
            v-model="form.resetDay"
            size="md"
            :min="1"
            :max="28"
            :label="t('admin.nodes.groupForm.fields.resetDay')"
            :help="t('admin.nodes.groupForm.help.resetDay')"
          />
        </div>
      </section>

      <section class="dialog-section">
        <h3 class="dialog-section__title">{{ t('admin.nodes.groupForm.joining') }}</h3>
        <div class="form-grid">
          <UiSelect
            v-model="form.expiry"
            size="md"
            :label="t('admin.nodes.groupForm.fields.tokenExpiry')"
            :options="expiryOptions"
            :error="errors.expiry"
            data-testid="group-expiry"
          />
          <UiNumberField
            v-model="form.maxNodes"
            size="md"
            :min="0"
            :label="t('admin.nodes.groupForm.fields.maxNodes')"
            :help="t('admin.nodes.groupForm.help.maxNodes')"
            data-testid="group-max-nodes"
          />
        </div>
        <UiSwitch
          v-model="form.requireApproval"
          :label="t('admin.nodes.groupForm.fields.requireApproval')"
          :description="t('admin.nodes.groupForm.help.requireApproval')"
          data-testid="group-require-approval"
        />
      </section>

      <template v-if="form.kind === 'proxy'">
        <section class="dialog-section">
          <h3 class="dialog-section__title">{{ t('admin.nodes.groupForm.fields.subscriptionGroups') }}</h3>
          <p class="group-form__text">{{ t('admin.nodes.groupForm.help.subscriptionGroups') }}</p>
          <p v-if="subscriptionGroups.length === 0" class="group-form__text" data-testid="group-no-subscription-groups">
            {{ t('admin.nodes.groupForm.noSubscriptionGroups') }}
          </p>
          <div v-else class="group-form__checks" role="group" :aria-label="t('admin.nodes.groupForm.fields.subscriptionGroups')">
            <UiCheckbox
              v-for="item in subscriptionGroups"
              :key="item.id"
              :model-value="form.subscriptionGroupIds.includes(item.id)"
              :label="item.name"
              @update:model-value="checked => toggleSubscriptionGroup(item.id, checked)"
            />
          </div>
        </section>

        <section class="dialog-section">
          <h3 class="dialog-section__title">{{ t('admin.nodes.groupForm.template.title') }}</h3>
          <p class="group-form__text">{{ t('admin.nodes.groupForm.template.description') }}</p>
          <NodeGroupTemplateEditor v-model="form.protocols" :validate="validated" />
        </section>
      </template>

      <p v-if="formError" class="form-error" role="alert" data-testid="group-form-error">{{ formError }}</p>
    </form>

    <template #footer>
      <UiButton :disabled="saving" @click="close">{{ t('common.actions.cancel') }}</UiButton>
      <UiButton variant="primary" data-testid="save-group" :loading="saving" @click="save">
        {{ group ? t('admin.nodes.groupForm.save') : t('admin.nodes.groupForm.create') }}
      </UiButton>
    </template>
  </UiSheet>
</template>

<script setup>
// Create or edit a node group (POST /kernel/node-groups, PUT
// /kernel/node-groups/:id): the defaults every new node copies, how machines
// join (token validity, node cap, approval), the subscription groups its
// protocols join and, for a proxy group, the protocol template. A changed
// template applies to machines that join afterwards; nodes already there keep
// their protocols. The API's validation errors (they name the field, such as
// "protocol_template[0]: ...") show in the sheet.
import { computed, reactive, ref, watch } from 'vue'
import { getSubscriptionGroups } from '@/api/admin'
import { createKernelNodeGroup, updateKernelNodeGroup } from '@/api/kernel'
import UiButton from '@/ui/UiButton.vue'
import UiCheckbox from '@/ui/UiCheckbox.vue'
import UiNumberField from '@/ui/UiNumberField.vue'
import UiSegmentedControl from '@/ui/UiSegmentedControl.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiSheet from '@/ui/UiSheet.vue'
import UiSwitch from '@/ui/UiSwitch.vue'
import UiTextField from '@/ui/UiTextField.vue'
import { useToast } from '@/ui/composables/useToast'
import { useAppI18n } from '@/composables/useAppI18n'
import NodeGroupTemplateEditor from './NodeGroupTemplateEditor.vue'
import { readNodeList } from './nodeData'
import {
  NODE_GROUP_KINDS, TOKEN_EXPIRY_DAYS, buildGroupInput, groupFormFrom, hasProblems, pickDefaultSubscriptionGroup,
  protocolProblems, readKernelError
} from './nodeGroupData'

const props = defineProps({
  open: { type: Boolean, default: false },
  // The group to edit (as GET /kernel/node-groups lists it); null creates one.
  group: { type: Object, default: null }
})
const emit = defineEmits(['update:open', 'saved'])
const { t, formatDateTime } = useAppI18n()
const toast = useToast()

const form = reactive(groupFormFrom(null))
const errors = reactive({ name: '', expiry: '' })
const formError = ref('')
const saving = ref(false)
const validated = ref(false)
const subscriptionGroups = ref([])

const kindOptions = computed(() => NODE_GROUP_KINDS.map(value => ({ value, label: t(`admin.nodes.groups.kinds.${value}`) })))
const expiryOptions = computed(() => {
  const options = [{ value: 'never', label: t('admin.nodes.groupForm.expiry.never') }]
  for (const value of Object.keys(TOKEN_EXPIRY_DAYS)) options.push({ value, label: t(`admin.nodes.groupForm.expiry.${value}`) })
  if (form.expiry === 'keep') {
    options.push({ value: 'keep', label: t('admin.nodes.groupForm.expiry.keep', { time: formatDateTime(form.currentExpiry) || form.currentExpiry }) })
  }
  if (form.expiry === 'expired') {
    options.push({ value: 'expired', label: t('admin.nodes.groupForm.expiry.expired'), disabled: true })
  }
  return options
})

function toggleSubscriptionGroup(id, checked) {
  const ids = new Set(form.subscriptionGroupIds)
  if (checked) ids.add(id)
  else ids.delete(id)
  form.subscriptionGroupIds = [...ids]
}

async function loadSubscriptionGroups() {
  try {
    subscriptionGroups.value = readNodeList(await getSubscriptionGroups()).map(item => ({ id: Number(item.id), name: item.name }))
    // A new group joins the default subscription group, as the first one does.
    if (!props.group) {
      const preferred = pickDefaultSubscriptionGroup(subscriptionGroups.value)
      if (preferred && form.subscriptionGroupIds.length === 0) form.subscriptionGroupIds = [preferred.id]
    }
  } catch (cause) {
    console.error('Failed to load the subscription groups:', cause)
  }
}

watch(() => props.open, (open) => {
  if (!open) return
  Object.assign(form, groupFormFrom(props.group))
  errors.name = ''
  errors.expiry = ''
  formError.value = ''
  validated.value = false
  void loadSubscriptionGroups()
}, { immediate: true })

function validateName() {
  errors.name = String(form.name || '').trim() ? '' : t('admin.nodes.form.required')
  return !errors.name
}

function close() {
  if (saving.value) return
  emit('update:open', false)
}

async function save() {
  if (saving.value) return
  formError.value = ''
  validated.value = true
  const nameOk = validateName()
  errors.expiry = form.expiry === 'expired' ? t('admin.nodes.groupForm.expiry.expiredError') : ''
  const protocolsOk = form.kind !== 'proxy' || form.protocols.every(protocol => !hasProblems(protocolProblems(protocol)))
  if (!nameOk || errors.expiry) return
  if (!protocolsOk) {
    formError.value = t('admin.nodes.groupForm.template.fixProblems')
    return
  }
  const editing = Boolean(props.group)
  const input = buildGroupInput(form)
  saving.value = true
  try {
    if (editing) await updateKernelNodeGroup(props.group.id, input)
    else await createKernelNodeGroup(input)
    saving.value = false
    emit('update:open', false)
    toast.success(t(editing ? 'admin.nodes.groups.saved' : 'admin.nodes.groups.created', { name: input.name }))
    emit('saved', { input, editing })
  } catch (cause) {
    const problem = readKernelError(cause)
    if (problem.code === 'name_taken') errors.name = t('admin.nodes.groups.nameTaken')
    else if (problem.code === 'ca_kek_missing') formError.value = t('admin.nodes.groups.caKekMissing')
    else formError.value = t('admin.nodes.groupForm.saveFailed', { message: problem.message })
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.group-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
}

.group-form__text {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
}

.group-form__checks {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
</style>
