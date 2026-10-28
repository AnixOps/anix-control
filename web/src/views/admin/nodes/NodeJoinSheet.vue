<template>
  <UiSheet
    :open="open"
    size="lg"
    :title="t('admin.nodes.join.title')"
    :description="t('admin.nodes.join.description')"
    data-testid="node-join-sheet"
    @update:open="onOpenChange"
  >
    <p v-if="join.stage.value === 'checking' || join.stage.value === 'loading'" class="join__status" role="status" data-testid="join-loading">
      <UiSpinner :size="16" />
      <span>{{ join.stage.value === 'checking' ? t('admin.nodes.join.checking') : t('admin.nodes.join.loading') }}</span>
    </p>

    <section v-else-if="join.stage.value === 'blocked'" class="dialog-section" data-testid="join-blocked">
      <h3 class="dialog-section__title">{{ t('admin.nodes.join.blocked.title') }}</h3>
      <p class="join__text">{{ t('admin.nodes.join.blocked.description') }}</p>
      <ul class="join__checks">
        <li v-for="check in join.blockers.value" :key="check.id" class="join__check" :data-check="check.id">
          <strong>{{ checkTitle(check) }}</strong>
          <span class="join__check-fix">{{ checkFix(check) }}</span>
          <span v-if="check.message" class="join__check-detail">{{ check.message }}</span>
        </li>
      </ul>
      <div class="join__actions">
        <UiButton variant="primary" :icon="RotateCw" data-testid="join-recheck" @click="join.recheck()">{{ t('admin.nodes.join.recheck') }}</UiButton>
      </div>
    </section>

    <section v-else-if="join.stage.value === 'error'" class="dialog-section" data-testid="join-error">
      <p class="form-error" role="alert">{{ t('admin.nodes.join.loadFailed', { message: join.errorMessage.value }) }}</p>
      <div class="join__actions">
        <UiButton :icon="RotateCw" data-testid="join-retry" @click="join.recheck()">{{ t('admin.nodes.join.retry') }}</UiButton>
      </div>
    </section>

    <template v-else-if="join.stage.value === 'ready'">
      <NodeNotice v-for="check in join.warnings.value" :key="check.id" tone="warning" :data-testid="`join-warning-${check.id}`">
        <strong>{{ checkTitle(check) }}</strong>
        {{ checkFix(check) }}
      </NodeNotice>

      <NodeNotice v-if="join.createdDefault.value" tone="info" data-testid="join-default-created">
        {{ t('admin.nodes.join.defaultCreated', { name: join.group.value?.name || '' }) }}
      </NodeNotice>

      <section class="dialog-section">
        <div class="join__group-row">
          <UiSelect
            v-if="join.groups.value.length"
            :model-value="join.groupId.value ?? undefined"
            size="md"
            class="join__group-select"
            :label="t('admin.nodes.join.group')"
            :options="groupOptions"
            data-testid="join-group"
            @update:model-value="value => join.selectGroup(value)"
          />
          <UiButton :icon="Plus" data-testid="join-new-group" @click="creating = !creating">{{ t('admin.nodes.join.newGroup') }}</UiButton>
        </div>
        <NodeGroupQuickCreate v-if="creating" :create="createGroup" @cancel="creating = false" />
        <p v-if="join.group.value" class="join__text" data-testid="join-group-summary">{{ groupSummary }}</p>
      </section>

      <NodeJoinCommand
        v-if="join.group.value"
        :group="join.group.value"
        :command="join.command.value"
        :problem="join.commandProblem.value"
        :problem-message="join.commandMessage.value"
        @reset="resetToken"
        @retry="join.selectGroup(join.groupId.value)"
      >
        <NodeJoinNodes
          :nodes="join.nodes.value"
          :loaded="join.nodesLoaded.value"
          :busy="join.busyNodes.value"
          @approve="decide('approve', $event)"
          @reject="decide('reject', $event)"
        />
      </NodeJoinCommand>
    </template>

    <template #footer="{ close }">
      <UiButton @click="close">{{ t('common.actions.close') }}</UiButton>
    </template>
  </UiSheet>
</template>

<script setup>
// 接入节点: the one entry point for a new machine (docs/design/node-onboarding,
// "The Administrator's Flow"). Pick a node group or make one here; copy the
// command; watch the machine arrive. An empty panel gets its default group
// without any input, and a Control that cannot hand out a working command
// says what to fix instead of showing one that would fail.
import { computed, ref, watch } from 'vue'
import { Plus, RotateCw } from '@lucide/vue'
import UiButton from '@/ui/UiButton.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiSheet from '@/ui/UiSheet.vue'
import UiSpinner from '@/ui/UiSpinner.vue'
import { useConfirm } from '@/ui/composables/useConfirm'
import { useToast } from '@/ui/composables/useToast'
import { useAppI18n } from '@/composables/useAppI18n'
import NodeGroupQuickCreate from './NodeGroupQuickCreate.vue'
import NodeJoinCommand from './NodeJoinCommand.vue'
import NodeJoinNodes from './NodeJoinNodes.vue'
import NodeNotice from './NodeNotice.vue'
import { groupKind, readKernelError } from './nodeGroupData'
import { useNodeJoin } from './useNodeJoin'

const props = defineProps({
  open: { type: Boolean, default: false },
  // The group to show first (the groups tab's row action); the first group
  // otherwise.
  groupId: { type: Number, default: null }
})
const emit = defineEmits(['update:open', 'changed'])

const { t, te } = useAppI18n()
const toast = useToast()
const confirm = useConfirm()

// A join, an approval or a new group changes what the page behind shows.
const join = useNodeJoin({ onChanged: () => emit('changed') })
const creating = ref(false)

const groupOptions = computed(() => join.groups.value.map(group => ({
  value: group.id,
  label: `${group.name} · ${t(`admin.nodes.groups.kinds.${groupKind(group)}`)}`
})))

const groupSummary = computed(() => {
  const group = join.group.value
  if (!group) return ''
  const counts = group.nodes || { live: 0, pending: 0 }
  const parts = [t('admin.nodes.groups.nodesCount', { live: counts.live || 0, pending: counts.pending || 0 })]
  if (group.require_approval) parts.push(t('admin.nodes.join.requiresApproval'))
  return parts.join(' · ')
})

// The check's own name and the fix in plain words, by its id; an id this
// version does not know shows the server's words.
function checkTitle(check) {
  const key = `admin.nodes.join.checks.${check.id}`
  return te(key) ? t(key) : check.id
}

function checkFix(check) {
  const key = `admin.nodes.join.fixes.${check.id}`
  return te(key) ? t(key) : (check.fix || check.message || '')
}

async function createGroup(input) {
  await join.createGroup(input)
  creating.value = false
}

async function resetToken() {
  const group = join.group.value
  if (!group) return
  const confirmed = await confirm({
    title: t('admin.nodes.join.command.resetConfirmTitle', { name: group.name }),
    message: t('admin.nodes.join.command.resetConfirmMessage'),
    confirmLabel: t('admin.nodes.join.command.reset'),
    tone: 'danger',
    onConfirm: async () => {
      try {
        await join.resetToken()
      } catch (error) {
        throw new Error(readKernelError(error).message)
      }
    }
  })
  if (confirmed) toast.success(t('admin.nodes.join.command.resetDone'))
}

async function decide(action, node) {
  try {
    if (action === 'approve') await join.approve(node)
    else {
      const confirmed = await confirm({
        title: t('admin.nodes.approval.rejectTitle', { name: node.name }),
        message: t('admin.nodes.approval.rejectMessage'),
        confirmLabel: t('admin.nodes.approval.reject'),
        tone: 'danger'
      })
      if (!confirmed) return
      await join.reject(node)
    }
    toast.success(t(action === 'approve' ? 'admin.nodes.approval.approvedToast' : 'admin.nodes.approval.rejectedToast', { name: node.name }))
  } catch (error) {
    toast.error(t('admin.nodes.approval.failed', { message: readKernelError(error).message }))
  }
}

function onOpenChange(value) {
  if (!value) {
    join.close()
    creating.value = false
  }
  emit('update:open', value)
}

watch(() => props.open, (value) => {
  if (value) {
    creating.value = false
    void join.open(props.groupId)
  } else {
    // Closed from outside (the page sets it): the token goes with the dialog.
    join.close()
  }
}, { immediate: true })

defineExpose({ join })
</script>

<style scoped>
.join__status {
  display: flex;
  gap: var(--space-2);
  align-items: center;
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.join__text {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
}

.join__checks {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  margin: 0;
  padding: 0;
  list-style: none;
}

.join__check {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-md);
  background: var(--danger-soft);
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
}

.join__check-fix {
  color: var(--label-1);
}

.join__check-detail {
  color: var(--label-1);
  font-size: var(--type-caption-size);
  overflow-wrap: anywhere;
}

.join__actions {
  display: flex;
  gap: var(--space-2);
}

.join__group-row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3);
  align-items: flex-end;
}

.join__group-select {
  flex: 1 1 240px;
  min-width: 0;
}
</style>
