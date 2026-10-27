<template>
  <UiCard :title="t('forwardV4.subscription.title')" :description="t('forwardV4.subscription.description')" data-testid="forward-route-subscription">
    <UiErrorState v-if="error && !status" compact :title="t('forwardV4.subscription.loadFailed')" :error="error" @retry="load" />
    <UiSkeleton v-else-if="!status" variant="text" :lines="3" />

    <template v-else>
      <p v-if="!status.protocol" class="fwd-note is-warning" role="status" data-testid="forward-route-subscription-none">{{ t('forwardV4.subscription.noProtocol') }}</p>
      <template v-else>
        <p class="sub-text">{{ t('forwardV4.subscription.protocol', { node: status.protocol.node_name, name: status.protocol.name, type: status.protocol.type, port: status.protocol.port }) }}</p>
        <UiSwitch
          v-model="published"
          :label="t('forwardV4.subscription.publish')"
          :description="t('forwardV4.subscription.publishHelp')"
          :disabled="saving"
          data-testid="forward-route-subscription-switch"
        />
        <UiTextField
          v-model="displayName"
          :label="t('forwardV4.subscription.displayName')"
          :help="t('forwardV4.subscription.displayNameHelp', { route: status.route_name })"
          :placeholder="status.route_name"
          :disabled="saving"
          size="md"
        />
        <div class="sub-actions">
          <UiButton variant="primary" size="sm" :loading="saving" :disabled="!dirty" data-testid="forward-route-subscription-save" @click="save">{{ t('forwardV4.subscription.save') }}</UiButton>
        </div>
        <p v-if="status.published && status.reason" class="fwd-note is-warning" role="status" data-testid="forward-route-subscription-reason">{{ t(`forwardV4.subscription.reasons.${status.reason}`) }}</p>
        <ul v-if="status.published && !status.reason" class="sub-entries" data-testid="forward-route-subscription-entries">
          <li v-for="entry in status.entries" :key="`${entry.host}:${entry.port}`">
            <span class="fwd-mono">{{ hostPort(entry) }}</span>
            <span class="fwd-muted">{{ entry.node_name }}</span>
          </li>
        </ul>
      </template>
    </template>
  </UiCard>
</template>

<script setup>
// 订阅 on a route's page (Phase 10): when the route's target is a proxy
// node's protocol, 加入订阅 offers that protocol again in its subscription
// groups at the route's entries (GET/PUT /api/v4/kernel/forward-routes/
// {id}/subscription). It replaces the child relay node.
import { computed, onMounted, ref } from 'vue'
import { UiButton, UiCard, UiErrorState, UiSkeleton, UiSwitch, UiTextField, useToast } from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import { getRouteSubscription, setRouteSubscription } from '@/api/kernel'

const props = defineProps({
  routeId: { type: String, required: true }
})
const { t } = useAppI18n()
const toast = useToast()

const status = ref(null)
const error = ref(null)
const saving = ref(false)
const published = ref(false)
const displayName = ref('')

const dirty = computed(() => Boolean(status.value) && (published.value !== status.value.published || displayName.value.trim() !== (status.value.display_name || '')))

function apply(answer) {
  status.value = answer
  published.value = Boolean(answer?.published)
  displayName.value = answer?.display_name || ''
}

async function load() {
  error.value = null
  try {
    apply(await getRouteSubscription(props.routeId))
  } catch (e) {
    error.value = e
  }
}

async function save() {
  saving.value = true
  try {
    apply(await setRouteSubscription(props.routeId, published.value, displayName.value.trim()))
    toast.success(t(published.value ? 'forwardV4.subscription.published' : 'forwardV4.subscription.withdrawn'))
  } catch (e) {
    toast.error(e?.response?.data?.error?.message || e?.message || t('forwardV4.subscription.saveFailed'))
  } finally {
    saving.value = false
  }
}

function hostPort(entry) {
  return entry.host.includes(':') ? `[${entry.host}]:${entry.port}` : `${entry.host}:${entry.port}`
}

onMounted(load)

defineExpose({ load, status })
</script>

<style scoped>
.sub-text {
  margin: 0 0 var(--space-3);
  color: var(--label-2);
}

.sub-actions {
  display: flex;
  justify-content: flex-end;
  margin-top: var(--space-3);
}

.sub-entries {
  display: grid;
  gap: var(--space-2);
  margin: var(--space-3) 0 0;
  padding: 0;
  list-style: none;
}

.sub-entries li {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3);
  overflow-wrap: anywhere;
}
</style>
