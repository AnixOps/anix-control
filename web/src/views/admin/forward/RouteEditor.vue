<template>
  <section class="fwd-page editor" data-testid="forward-editor">
    <UiPageHeader :title="pageTitle">
      <template #back>
        <RouterLink class="fwd-back" :to="backTo"><ChevronLeft :size="16" aria-hidden="true" /> {{ editing ? t('forwardV4.editor.backToRoute') : t('forwardV4.nav.routes') }}</RouterLink>
      </template>
      <template #meta>
        <UiBadge v-if="editing && draft" tone="neutral" :dot="false" :label="t('forwardV4.detail.revision', { n: draft.revision })" />
      </template>
      <template #actions>
        <UiButton @click="router.push(backTo)">{{ t('forwardV4.common.cancel') }}</UiButton>
        <UiButton variant="primary" :icon="Check" :loading="saving" :disabled="!canSave" data-testid="forward-save" @click="save">{{ editing ? t('forwardV4.editor.save') : t('forwardV4.editor.create') }}</UiButton>
      </template>
    </UiPageHeader>

    <UiErrorState v-if="loadError" :title="t('forwardV4.editor.loadFailed')" :error="loadError" @retry="load" />
    <UiSkeleton v-else-if="!draft" variant="card" />

    <template v-else>
      <p v-if="enforced" class="fwd-note is-warning" role="status">{{ t(`forwardV4.editor.enforced.${enforced}`) }}</p>

      <div v-if="conflict" class="editor-conflict" role="alert" data-testid="forward-conflict">
        <p class="editor-conflict__title"><AlertTriangle :size="16" aria-hidden="true" /> {{ t('forwardV4.conflict.title', { mine: conflict.mine, theirs: conflict.theirs }) }}</p>
        <p class="fwd-muted">{{ t('forwardV4.conflict.help') }}</p>
        <span class="editor-conflict__actions">
          <UiButton size="sm" variant="primary" data-testid="forward-conflict-review" @click="diffOpen = true">{{ t('forwardV4.conflict.review') }}</UiButton>
          <UiButton size="sm" data-testid="forward-conflict-discard" @click="discardMine">{{ t('forwardV4.conflict.discard') }}</UiButton>
        </span>
      </div>

      <p v-if="saveError" class="fwd-note is-danger" role="alert" data-testid="forward-save-error">{{ saveError }}</p>

      <div v-if="violations.length" class="editor-summary" role="alert" data-testid="forward-violations">
        <p class="editor-summary__title"><AlertCircle :size="16" aria-hidden="true" /> {{ t('forwardV4.editor.problems', { n: violations.length }) }}</p>
        <ul class="editor-summary__list">
          <li v-for="(item, index) in violations" :key="index">
            <button type="button" class="editor-summary__item" :data-violation-field="item.field" @click="focusField(item.field)">
              <span class="fwd-mono editor-summary__field">{{ item.field || 'route' }}</span>
              <span>{{ violationText(t, te, item) }}</span>
              <span class="fwd-mono editor-summary__code">{{ item.code }}</span>
            </button>
          </li>
        </ul>
      </div>

      <div class="editor-layout">
        <form class="editor-form" novalidate @submit.prevent="save">
          <UiCard :title="t('forwardV4.editor.basics')" heading-tag="h2">
            <div class="editor-grid">
              <UiTextField :id="fieldId('name')" v-model="draft.name" size="md" :label="t('forwardV4.editor.name')" required :error="errorText('name')" />
              <UiField :label="t('forwardV4.editor.labels')" label-tag="span" :help="t('forwardV4.editor.labelsHelp')" :error="errorText('labels')" data-field="labels">
                <template #default="{ labelId }">
                  <div class="editor-labels" role="group" :aria-labelledby="labelId">
                    <span v-for="(item, index) in draft.labels" :key="item._key" class="editor-label">
                      <UiTextField v-model="item.key" size="sm" :aria-label="t('forwardV4.editor.labelKey', { n: index + 1 })" placeholder="key" />
                      <span aria-hidden="true">=</span>
                      <UiTextField v-model="item.value" size="sm" :aria-label="t('forwardV4.editor.labelValue', { n: index + 1 })" placeholder="value" />
                      <UiIconButton size="sm" :icon="X" :label="t('forwardV4.editor.removeLabel', { n: index + 1 })" @click="draft.labels.splice(index, 1)" />
                    </span>
                    <UiButton size="sm" variant="tertiary" :icon="Plus" @click="addLabel">{{ t('forwardV4.editor.addLabel') }}</UiButton>
                  </div>
                </template>
              </UiField>
            </div>
          </UiCard>

          <UiCard :title="t('forwardV4.editor.listen')" heading-tag="h2" :description="t('forwardV4.editor.listenDescription')">
            <div class="editor-grid editor-grid--3">
              <UiTextField :id="fieldId('listen.address')" v-model="draft.listen.address" size="md" :label="t('forwardV4.editor.listenAddress')" placeholder="0.0.0.0" :error="errorText('listen.address')" />
              <UiField :label="t('forwardV4.editor.port')" label-tag="span" :error="errorText('listen.port')" :help="draft.listen.portMode === 'auto' ? t('forwardV4.editor.portAutoHelp') : t('forwardV4.editor.portExplicitHelp')" data-field="listen.port">
                <template #default="{ labelId }">
                  <span class="editor-port" role="group" :aria-labelledby="labelId">
                    <UiSegmentedControl v-model="draft.listen.portMode" size="sm" :aria-label="t('forwardV4.editor.portMode')" :options="portModes" />
                    <UiNumberField v-if="draft.listen.portMode === 'explicit'" :id="fieldId('listen.port')" v-model="draft.listen.port" size="md" :aria-label="t('forwardV4.editor.portValueEntry')" :min="1" :max="65535" />
                  </span>
                </template>
              </UiField>
              <UiSelect :id="fieldId('listen.protocol')" v-model="draft.listen.protocol" size="md" :label="t('forwardV4.editor.protocol')" :options="protocolOptions" :error="errorText('listen.protocol')" />
            </div>
            <UiTextField
              v-if="draft.hops[0]?.node_refs.length > 1"
              :id="fieldId('listen.entry_hostname')"
              v-model="draft.listen.entry_hostname"
              class="editor-mt"
              size="md"
              :label="t('forwardV4.editor.entryHostname')"
              placeholder="edge.example.net"
              :help="t('forwardDns.binding.hostnameHelp')"
              :error="errorText('listen.entry_hostname')"
            />
            <DnsBindingPicker
              v-if="draft.hops[0]?.node_refs.length > 1 && dnsReady"
              v-model="dns"
              :stored="storedBinding"
              :hostname="draft.listen.entry_hostname"
              :errors="dnsErrors"
            />
          </UiCard>

          <UiCard :title="t('forwardV4.editor.chain')" heading-tag="h2" :description="chainDescription">
            <p class="visually-hidden" aria-live="polite">{{ announcement }}</p>
            <ol class="editor-chain" data-field="hops">
              <template v-for="(hop, index) in draft.hops" :key="hop._key">
                <li v-if="index > 0" class="editor-link" aria-hidden="true">
                  <ArrowDown :size="16" />
                  <span class="fwd-mono">{{ linkLabel(hop, ' · mux') }}</span>
                </li>
                <li>
                  <HopCard
                    :ref="el => setHopRef(hop._key, el)"
                    :hop="hop"
                    :previous="draft.hops[index - 1] || null"
                    :index="index"
                    :total="draft.hops.length"
                    :nodes="nodes"
                    :violations="violations"
                    :error-text="errorText"
                    :enable-anix-ops="enableAnixOps"
                    @update="patch => updateHop(index, patch)"
                    @add-node="ref => hop.node_refs.push(ref)"
                    @remove-node="ref => hop.node_refs.splice(hop.node_refs.indexOf(ref), 1)"
                    @move="delta => moveHop(index, delta)"
                    @remove="removeHop(index)"
                    @insert-relay="insertRelay(index)"
                  />
                </li>
              </template>
              <li class="editor-link" aria-hidden="true"><ArrowDown :size="16" /><span class="fwd-mono">RAW</span></li>
              <li class="editor-targets-anchor fwd-muted">{{ t('forwardV4.editor.targetsBelow') }}</li>
            </ol>
            <UiButton class="editor-mt" :icon="Plus" :disabled="draft.hops.length >= MAX_HOPS" data-testid="forward-add-hop" @click="addHop">{{ t('forwardV4.editor.addHop') }}</UiButton>
          </UiCard>

          <UiCard :title="t('forwardV4.editor.targets')" heading-tag="h2" :description="t('forwardV4.editor.targetsDescription')">
            <div class="editor-targets" data-field="targets">
              <div class="editor-targets__head" aria-hidden="true">
                <span>{{ t('forwardV4.editor.host') }}</span><span>{{ t('forwardV4.editor.port') }}</span><span>{{ t('forwardV4.editor.weight') }}</span><span>{{ t('forwardV4.editor.priorityShort') }}</span><span />
              </div>
              <div v-for="(target, index) in draft.targets" :key="target._key" class="editor-target">
                <UiTextField :id="fieldId(`targets[${index}].host`)" v-model="target.host" size="md" :aria-label="t('forwardV4.editor.targetHost', { n: index + 1 })" :placeholder="t('forwardV4.editor.hostPlaceholder')" :error="errorText(`targets[${index}].host`)" />
                <UiNumberField :id="fieldId(`targets[${index}].port`)" v-model="target.port" size="md" :aria-label="t('forwardV4.editor.targetPort', { n: index + 1 })" :min="1" :max="65535" :error="errorText(`targets[${index}].port`)" />
                <UiNumberField :id="fieldId(`targets[${index}].weight`)" v-model="target.weight" size="md" :aria-label="t('forwardV4.editor.targetWeight', { n: index + 1 })" :min="0" placeholder="1" :error="errorText(`targets[${index}].weight`)" />
                <UiNumberField :id="fieldId(`targets[${index}].priority`)" v-model="target.priority" size="md" :aria-label="t('forwardV4.editor.targetPriority', { n: index + 1 })" :min="0" placeholder="0" :error="errorText(`targets[${index}].priority`)" />
                <UiIconButton :icon="Trash2" :label="t('forwardV4.editor.removeTarget', { n: index + 1 })" :disabled="draft.targets.length === 1" @click="draft.targets.splice(index, 1)" />
              </div>
            </div>
            <div class="editor-grid editor-mt">
              <UiButton :icon="Plus" @click="draft.targets.push(blankTarget())">{{ t('forwardV4.editor.addTarget') }}</UiButton>
              <UiSelect :id="fieldId('policy.target_policy')" v-model="draft.policy.target_policy" size="md" :label="t('forwardV4.editor.targetPolicy')" :options="targetPolicyOptions" :error="errorText('policy.target_policy')" />
            </div>
          </UiCard>

          <UiCard :title="t('forwardV4.editor.balance')" heading-tag="h2">
            <div class="editor-grid">
              <UiSelect :id="fieldId('policy.next_hop')" v-model="draft.policy.next_hop" size="md" :label="t('forwardV4.editor.nextHopStrategy')" :options="strategyOptions" :help="nextHopHelp" :error="errorText('policy.next_hop')" />
              <UiSelect :id="fieldId('policy.target')" v-model="draft.policy.target" size="md" :label="t('forwardV4.editor.targetStrategy')" :options="strategyOptions" :help="targetHelp" :error="errorText('policy.target')" />
            </div>
            <UiRadioGroup :id="fieldId('policy.direct')" v-model="draft.policy.direct" class="editor-mt" :label="t('forwardV4.editor.direct')" orientation="horizontal" :options="directOptions" :error="errorText('policy.direct')" />
            <div class="editor-subhead">
              <h3 class="editor-subhead__title">{{ t('forwardV4.editor.healthTitle') }}</h3>
              <span class="fwd-muted">{{ t('forwardV4.editor.healthHelp') }}</span>
            </div>
            <div class="editor-grid editor-grid--4">
              <UiNumberField :id="fieldId('policy.health.interval_ms')" v-model="draft.policy.health.interval_ms" size="md" :label="t('forwardV4.editor.interval')" unit="ms" :min="0" :placeholder="String(H21_DEFAULTS.interval_ms)" :help="t('forwardV4.editor.defaultIs', { v: H21_DEFAULTS.interval_ms })" :error="errorText('policy.health.interval_ms')" :disabled="draft.policy.health.disabled" />
              <UiNumberField :id="fieldId('policy.health.timeout_ms')" v-model="draft.policy.health.timeout_ms" size="md" :label="t('forwardV4.editor.timeout')" unit="ms" :min="0" :placeholder="String(H21_DEFAULTS.timeout_ms)" :help="t('forwardV4.editor.defaultIs', { v: H21_DEFAULTS.timeout_ms })" :error="errorText('policy.health.timeout_ms')" :disabled="draft.policy.health.disabled" />
              <UiNumberField :id="fieldId('policy.circuit_breaker.failure_threshold')" v-model="draft.policy.circuit_breaker.failure_threshold" size="md" :label="t('forwardV4.editor.threshold')" :unit="t('forwardV4.editor.times')" :min="0" :placeholder="String(H21_DEFAULTS.failure_threshold)" :help="t('forwardV4.editor.thresholdHelp', { v: H21_DEFAULTS.failure_threshold })" :error="errorText('policy.circuit_breaker.failure_threshold')" />
              <UiNumberField :id="fieldId('policy.circuit_breaker.open_ms')" v-model="draft.policy.circuit_breaker.open_ms" size="md" :label="t('forwardV4.editor.openFor')" unit="ms" :min="0" :placeholder="String(H21_DEFAULTS.open_ms)" :help="t('forwardV4.editor.openForHelp', { v: H21_DEFAULTS.open_ms })" :error="errorText('policy.circuit_breaker.open_ms')" />
            </div>
            <UiSwitch :id="fieldId('policy.health.disabled')" v-model="draft.policy.health.disabled" class="editor-mt" :label="t('forwardV4.editor.healthOff')" :description="t('forwardV4.editor.healthOffHelp')" />
          </UiCard>

          <UiCard id="limits" :title="t('forwardV4.editor.limits')" heading-tag="h2" :description="t('forwardV4.editor.limitsDescription')">
            <div class="editor-grid editor-grid--4">
              <UiNumberField :id="fieldId('limits.bandwidth_bps')" v-model="draft.limits.bandwidthMbps" size="md" :label="t('forwardV4.editor.bandwidth')" unit="Mbps" :min="0" :step="0.1" :placeholder="t('forwardV4.editor.unlimited')" :error="errorText('limits.bandwidth_bps')" />
              <UiNumberField :id="fieldId('limits.quota_bytes')" v-model="draft.limits.quotaGB" size="md" :label="t('forwardV4.editor.quota')" unit="GB" :min="0" :step="0.1" :placeholder="t('forwardV4.editor.unlimited')" :error="errorText('limits.quota_bytes')" />
              <UiNumberField :id="fieldId('limits.max_conns')" v-model="draft.limits.max_conns" size="md" :label="t('forwardV4.editor.maxConns')" :min="0" :placeholder="t('forwardV4.editor.unlimited')" :error="errorText('limits.max_conns')" />
              <UiTextField :id="fieldId('limits.expires_at_unix_ms')" v-model="draft.limits.expires" type="datetime-local" size="md" :label="t('forwardV4.editor.expires')" :help="t('forwardV4.editor.expiresHelp')" :error="errorText('limits.expires_at_unix_ms')" />
            </div>
          </UiCard>
        </form>

        <aside id="preview" class="editor-preview" :aria-label="t('forwardV4.preview.title')">
          <PreviewPanel
            :result="preview.result.value"
            :loading="preview.loading.value"
            :skipped="preview.skipped.value"
            :error="preview.error.value"
            :route="preview.requestBody.value"
            :route-id="draft.id"
            :nodes="nodes"
            :extra-warnings="uiWarnings"
            :error-message-for="error => forwardErrorMessage(t, error)"
            @refresh="preview.refresh"
          />
        </aside>
      </div>

      <div class="editor-phonebar">
        <UiBadge v-if="violations.length" tone="danger" :label="t('forwardV4.editor.problemsShort', { n: violations.length })" />
        <UiBadge v-else-if="preview.result.value" tone="success" :label="t('forwardV4.preview.ok')" />
        <UiBadge v-else tone="neutral" :label="t('forwardV4.preview.waiting')" />
        <a class="editor-phonebar__link" href="#preview">{{ t('forwardV4.editor.viewPreview') }}</a>
        <UiButton size="sm" variant="primary" :loading="saving" :disabled="!canSave" @click="save">{{ editing ? t('forwardV4.editor.save') : t('forwardV4.editor.createShort') }}</UiButton>
      </div>
    </template>

    <UiDialog v-model:open="diffOpen" size="lg" :title="t('forwardV4.conflict.diffTitle')" :description="t('forwardV4.conflict.diffDescription', { theirs: conflict?.theirs || '' })">
      <p v-if="!diff.length" class="fwd-muted">{{ t('forwardV4.conflict.noDiff') }}</p>
      <ul v-else class="editor-diff">
        <li v-for="row in diff" :key="row.field" class="editor-diff__row" :class="{ 'is-overlap': row.overlap }">
          <span class="fwd-mono">{{ row.field }}</span>
          <span class="editor-diff__values">
            <span><span class="fwd-muted">{{ t('forwardV4.conflict.before') }}</span> <code>{{ row.before || '—' }}</code></span>
            <span><span class="fwd-muted">{{ t('forwardV4.conflict.theirs') }}</span> <code>{{ row.theirs || '—' }}</code></span>
            <span><span class="fwd-muted">{{ t('forwardV4.conflict.mine') }}</span> <code>{{ row.mine || '—' }}</code></span>
          </span>
          <UiBadge v-if="row.overlap" tone="warning" :label="t('forwardV4.conflict.overlap')" />
        </li>
      </ul>
      <template #footer="{ close }">
        <UiButton @click="close">{{ t('forwardV4.common.cancel') }}</UiButton>
        <UiButton variant="primary" data-testid="forward-conflict-reapply" @click="reapply">{{ t('forwardV4.conflict.reapply') }}</UiButton>
      </template>
    </UiDialog>
  </section>
</template>

<script setup>
// 新建 / 编辑路由 (F5b, D2): a full page. The form maps each violation of
// POST /routes/preview (D4) or of a refused save to its field by the
// violation's `field` path; save stays disabled while the preview finds
// problems. One Idempotency-Key per save attempt is reused for retries of
// the same body; 409 revision_conflict keeps the form (D10).
import { computed, nextTick, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { AlertCircle, AlertTriangle, ArrowDown, Check, ChevronLeft, Plus, Trash2, X } from '@lucide/vue'
import {
  UiBadge, UiButton, UiCard, UiDialog, UiErrorState, UiField, UiIconButton, UiNumberField, UiPageHeader, UiRadioGroup,
  UiSegmentedControl, UiSelect, UiSkeleton, UiSwitch, UiTextField, useToast
} from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import { useUnsavedChanges } from '@/composables/useUnsavedChanges'
import { createDnsBinding, createRoute, getRoute, listDnsBindings, listNodes, newIdempotencyKey, routeBody, updateDnsBinding, updateRoute } from '@/api/forwardV4'
import DnsBindingPicker from '@/components/forward/DnsBindingPicker.vue'
import { bindingChanged, bindingDraft, bindingErrors, bindingRequest, dnsErrorMessage } from '@/components/forward/dnsModel'
import HopCard from '@/components/forward/HopCard.vue'
import PreviewPanel from '@/components/forward/PreviewPanel.vue'
import { forwardErrorMessage, violationText } from '@/components/forward/messages'
import { useForwardFlags } from '@/components/forward/useForwardFlags'
import { useRoutePreview } from '@/components/forward/useRoutePreview'
import {
  DIRECT_MODES, H21_DEFAULTS, PROTOCOLS, STRATEGIES, TARGET_POLICIES, blankHop, blankTarget, draftToRoute, fieldId, linkLabel,
  missingRequired, routeDiff, routeToDraft, syncRoles
} from '@/components/forward/routeModel'
import '@/components/forward/forward.css'

const props = defineProps({
  id: { type: String, default: '' }
})

const MAX_HOPS = 8
const router = useRouter()
const currentRoute = useRoute()
const { t, te } = useAppI18n()
const toast = useToast()
const { enableAnixOps } = useForwardFlags()

const editing = computed(() => Boolean(props.id))
const draft = ref(null)
const nodes = ref([])
const enforced = ref('')
const loadError = ref(null)
const saving = ref(false)
const saveError = ref('')
const serverViolations = ref([])
const conflict = ref(null)
const diffOpen = ref(false)
const announcement = ref('')
// The body the form started from (or was last saved as), for dirty and
// for the conflict diff.
const baseBody = ref('')
const baseRoute = ref(null)
// D10: the key of the current save attempt and the body it was sent with.
const attempt = { key: newIdempotencyKey(), body: null }

const backTo = computed(() => (editing.value ? `/admin/forward/routes/${props.id}` : '/admin/forward/routes'))
const pageTitle = computed(() => (editing.value
  ? t('forwardV4.editor.editTitle', { name: draft.value?.name || props.id })
  : t('forwardV4.editor.newTitle')))

async function load() {
  loadError.value = null
  try {
    const sourceId = props.id || String(currentRoute.query.from || '')
    const [nodeAnswer, routeAnswer, bindings] = await Promise.all([
      listNodes(),
      sourceId ? getRoute(sourceId) : Promise.resolve(null),
      // An older package has no DNS API: the editor then has no picker.
      props.id ? listDnsBindings({ routeId: props.id }).catch(() => null) : Promise.resolve([])
    ])
    nodes.value = nodeAnswer.nodes
    storedBinding.value = bindings?.find(item => item.route_id === props.id) || null
    dnsReady.value = Array.isArray(bindings)
    let route = routeAnswer?.route || null
    if (route && !props.id) {
      // 复制为新路由: the copy has no identity and plans its own ports.
      route = { ...route, id: '', revision: '', owner: '', name: t('forwardV4.editor.copyName', { name: route.name }), paused: false }
      if (route.listen) route.listen = { ...route.listen, port: 0 }
    }
    enforced.value = props.id ? (routeAnswer?.enforced || '') : ''
    setDraft(route)
    await nextTick()
    if (currentRoute.hash === '#limits') document.getElementById('limits')?.scrollIntoView({ block: 'start' })
  } catch (error) {
    loadError.value = forwardErrorMessage(t, error)
  }
}

function setDraft(route) {
  draft.value = reactive(routeToDraft(route))
  dns.value = bindingDraft(storedBinding.value, route?.listen?.entry_hostname || '')
  baseRoute.value = route
  baseBody.value = JSON.stringify(routeBody(draftToRoute(draft.value)))
  serverViolations.value = []
}

onMounted(load)

const currentRouteValue = () => (draft.value ? draftToRoute(draft.value) : null)
const preview = useRoutePreview(() => currentRouteValue(), {
  missing: () => (draft.value ? missingRequired(draft.value) : ['route'])
})
// The first preview runs once the draft is loaded.
watch(draft, value => { if (value) preview.schedule() })

// Edits clear a refused save's violations: the preview takes over again.
watch(() => draft.value && JSON.stringify(routeBody(draftToRoute(draft.value))), () => {
  serverViolations.value = []
  saveError.value = ''
})

// ---------------------------------------------------------------------------
// The entry hostname's DNS binding (L2, D14)
// ---------------------------------------------------------------------------

const storedBinding = ref(null)
const dnsReady = ref(false)
const dns = ref(bindingDraft(null))
// The picker shows, and its binding is written, only while the entry has
// several nodes (draftToRoute drops entry_hostname otherwise).
const dnsActive = computed(() => Boolean(draft.value) && dnsReady.value && draft.value.hops[0]?.node_refs.length > 1)
const dnsErrors = computed(() => (dnsActive.value ? bindingErrors(dns.value, draft.value.listen.entry_hostname, t, storedBinding.value) : {}))
const dnsDirty = computed(() => dnsActive.value && bindingChanged(dns.value, storedBinding.value))

// applyBinding writes the binding once the route is stored. A failure is a
// toast: the route is saved, and its page shows the binding state.
async function applyBinding(routeId, hostname) {
  if (!dnsDirty.value) return
  const body = bindingRequest(dns.value, routeId, hostname, storedBinding.value)
  try {
    if (storedBinding.value) await updateDnsBinding(storedBinding.value.id, body, { idempotencyKey: newIdempotencyKey() })
    else await createDnsBinding(body, { idempotencyKey: newIdempotencyKey() })
  } catch (error) {
    toast.error(t('forwardDns.binding.saveFailed', { message: dnsErrorMessage(t, te, error) }))
  }
}

const violations = computed(() => (serverViolations.value.length ? serverViolations.value : (preview.result.value?.violations || [])))
const routeDirty = computed(() => Boolean(draft.value) && JSON.stringify(routeBody(draftToRoute(draft.value))) !== baseBody.value)
// leaving: the save went through, the page navigates without asking.
const leaving = ref(false)
const dirty = computed(() => !leaving.value && (routeDirty.value || dnsDirty.value))
const canSave = computed(() => Boolean(draft.value) && !saving.value && !conflict.value && !Object.keys(dnsErrors.value).length &&
  !missingRequired(draft.value).length && !violations.value.length && !preview.loading.value && !preview.error.value)

useUnsavedChanges(dirty, { discard: () => {} })

function errorText(field) {
  const hit = violations.value.find(item => item.field === field)
  return hit ? violationText(t, te, hit) : ''
}

const FOCUSABLE = 'input, button, select, textarea, [tabindex]:not([tabindex="-1"]), [role="combobox"]'
async function focusField(field) {
  await nextTick()
  let path = String(field || '')
  while (path) {
    const el = document.getElementById(fieldId(path)) || document.querySelector(`[data-field="${CSS.escape ? CSS.escape(path) : path}"]`)
    if (el) {
      const target = el.matches?.(FOCUSABLE) ? el : el.querySelector(FOCUSABLE)
      el.scrollIntoView?.({ block: 'center' })
      ;(target || el).focus?.()
      return
    }
    path = path.replace(/(\.[^.[\]]+|\[\d+\])$/, '')
  }
}

// ---------------------------------------------------------------------------
// The hop chain
// ---------------------------------------------------------------------------

const hopRefs = new Map()
function setHopRef(key, el) {
  if (el) hopRefs.set(key, el)
  else hopRefs.delete(key)
}

function updateHop(index, patch) {
  const hop = draft.value.hops[index]
  Object.assign(hop, patch)
  if (patch.ingress) hop.ingress = { ...patch.ingress }
}

async function moveHop(index, delta) {
  const to = index + delta
  if (to < 0 || to >= draft.value.hops.length) return
  const [hop] = draft.value.hops.splice(index, 1)
  draft.value.hops.splice(to, 0, hop)
  syncRoles(draft.value.hops)
  announcement.value = t('forwardV4.editor.moved', { from: index + 1, to: to + 1 })
  await nextTick()
  // Focus follows the moved hop.
  hopRefs.get(hop._key)?.focusTool(delta < 0 && to > 0 ? 'up' : 'down')
}

function removeHop(index) {
  draft.value.hops.splice(index, 1)
  syncRoles(draft.value.hops)
  announcement.value = t('forwardV4.editor.removed', { n: index + 1 })
}

async function addHop() {
  draft.value.hops.push(blankHop(draft.value.hops.length, draft.value.hops.length + 1))
  syncRoles(draft.value.hops)
  await nextTick()
  focusField(`hops[${draft.value.hops.length - 1}].node_refs`)
}

// insertRelay puts a gost relay before hop `index`: gost originates every
// encrypted link the next hop may terminate (the link_unsupported fix).
async function insertRelay(index) {
  const relay = blankHop(index, draft.value.hops.length + 1, 'ENGINE_GOST')
  draft.value.hops.splice(index, 0, relay)
  syncRoles(draft.value.hops)
  announcement.value = t('forwardV4.editor.inserted', { n: index + 1 })
  await nextTick()
  focusField(`hops[${index}].node_refs`)
}

function addLabel() {
  draft.value.labels.push({ _key: `l${Date.now()}${draft.value.labels.length}`, key: '', value: '' })
}

// ---------------------------------------------------------------------------
// Options and help
// ---------------------------------------------------------------------------

const portModes = computed(() => [{ value: 'auto', label: t('forwardV4.editor.auto') }, { value: 'explicit', label: t('forwardV4.editor.explicit') }])
const protocolOptions = computed(() => PROTOCOLS.map(value => ({ value, label: t(`forwardV4.protocol.${value}`) })))
const strategyOptions = computed(() => STRATEGIES.map(value => ({ value, label: t(`forwardV4.strategy.${value}`), description: t(`forwardV4.strategyHint.${value}`) })))
const directOptions = computed(() => DIRECT_MODES.map(value => ({ value, label: t(`forwardV4.direct.${value}`), description: t(`forwardV4.directHint.${value}`) })))
const targetPolicyOptions = computed(() => TARGET_POLICIES.map(value => ({ value, label: t(`forwardV4.targetPolicy.${value}`), description: t(`forwardV4.targetPolicyHint.${value}`) })))
const chainDescription = computed(() => (enableAnixOps.value ? t('forwardV4.editor.chainDescriptionAnixOps') : t('forwardV4.editor.chainDescription')))

// D11: LEAST_CONN on nftables is approximate until the Agent reports
// conntrack counts. The hop that balances is the one before the choice.
function leastConnOnNft(strategy, hop) {
  return strategy === 'BALANCE_STRATEGY_LEAST_CONN' && hop?.engine === 'ENGINE_NFTABLES'
}
const nextHopHelp = computed(() => (draft.value?.hops.slice(0, -1).some(hop => leastConnOnNft(draft.value.policy.next_hop, hop))
  ? t('forwardV4.editor.leastConnNft')
  : t('forwardV4.editor.nextHopHelp')))
const targetHelp = computed(() => (leastConnOnNft(draft.value?.policy.target, draft.value?.hops.at(-1))
  ? t('forwardV4.editor.leastConnNft')
  : t('forwardV4.editor.targetHelp')))

const uiWarnings = computed(() => {
  if (!draft.value) return []
  const out = []
  const hops = draft.value.hops
  hops.forEach((hop, index) => {
    const strategy = index === hops.length - 1 ? draft.value.policy.target : draft.value.policy.next_hop
    if (leastConnOnNft(strategy, hop)) out.push(t('forwardV4.preview.leastConnWarning', { n: index + 1 }))
  })
  if (hops[0]?.node_refs.length > 1 && !String(draft.value.listen.entry_hostname || '').trim()) out.push(t('forwardV4.preview.entryHaWarning'))
  return out
})

// ---------------------------------------------------------------------------
// Save (D10)
// ---------------------------------------------------------------------------

async function save() {
  if (!canSave.value) return
  const route = draftToRoute(draft.value)
  const body = JSON.stringify(routeBody(route))
  // A retry of the same body reuses its key, so a double click or a network
  // retry never applies twice; a different body is a new request.
  if (attempt.body !== null && attempt.body !== body) attempt.key = newIdempotencyKey()
  attempt.body = body
  saving.value = true
  saveError.value = ''
  try {
    // Only the binding changed: no route write (and no new revision).
    const answer = editing.value && body === baseBody.value && dnsDirty.value
      ? { route: { ...route, id: props.id } }
      : await (editing.value
        ? updateRoute(props.id, route, { idempotencyKey: attempt.key })
        : createRoute(route, { idempotencyKey: attempt.key }))
    attempt.key = newIdempotencyKey()
    attempt.body = null
    const saved = answer?.route || {}
    baseBody.value = body
    const savedId = saved.id || props.id
    await applyBinding(savedId, route.listen?.entry_hostname || '')
    toast.success(t(editing.value ? 'forwardV4.toast.saved' : 'forwardV4.toast.created', { name: saved.name || route.name }))
    leaving.value = true
    router.push(`/admin/forward/routes/${savedId}`)
  } catch (error) {
    if (error.code === 'revision_conflict') {
      await loadConflict()
    } else if (error.violations?.length) {
      serverViolations.value = error.violations
      saveError.value = forwardErrorMessage(t, error)
    } else {
      saveError.value = forwardErrorMessage(t, error)
    }
  } finally {
    saving.value = false
  }
}

async function loadConflict() {
  try {
    const latest = await getRoute(props.id)
    conflict.value = { mine: draft.value.revision, theirs: latest?.route?.revision || '?', latest: latest?.route || null, enforced: latest?.enforced || '' }
  } catch (error) {
    saveError.value = forwardErrorMessage(t, error)
  }
}

const diff = computed(() => {
  if (!conflict.value?.latest || !draft.value) return []
  return routeDiff(JSON.parse(baseBody.value || '{}'), routeBody(conflict.value.latest), routeBody(draftToRoute(draft.value)))
})

// 重新应用: keep the form, at the revision now stored.
async function reapply() {
  const latest = conflict.value?.latest
  if (!latest) return
  draft.value.revision = latest.revision
  baseRoute.value = latest
  baseBody.value = JSON.stringify(routeBody(latest))
  enforced.value = conflict.value.enforced
  conflict.value = null
  diffOpen.value = false
  await nextTick()
  await save()
}

// 放弃我的修改: load what is stored now.
function discardMine() {
  const latest = conflict.value?.latest
  enforced.value = conflict.value?.enforced || ''
  conflict.value = null
  if (latest) setDraft(latest)
}
</script>

<style scoped>
.editor-summary,
.editor-conflict {
  min-width: 0;
  padding: var(--space-3) var(--space-4);
  border: 1px solid var(--danger);
  border-radius: var(--radius-sm);
  background: var(--danger-soft);
}

.editor-conflict {
  display: grid;
  gap: var(--space-2);
  border-color: var(--warning);
  background: var(--warning-soft);
}

.editor-conflict p {
  margin: 0;
}

.editor-conflict__title,
.editor-summary__title {
  display: flex;
  gap: var(--space-2);
  align-items: center;
  font-weight: var(--weight-semibold);
}

.editor-summary__title {
  margin: 0 0 var(--space-2);
}

.editor-conflict__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}

.editor-summary__list {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: var(--space-1);
  margin: 0;
  padding: 0;
  list-style: none;
}

.editor-summary__item {
  display: flex;
  min-width: 0;
  white-space: normal;
  max-width: 100%;
  flex-wrap: wrap;
  gap: var(--space-1) var(--space-3);
  align-items: baseline;
  padding: var(--space-1) 0;
  border: 0;
  background: none;
  color: var(--label-1);
  text-align: left;
  cursor: pointer;
}

.editor-summary__item > span {
  min-width: 0;
  overflow-wrap: anywhere;
}

.editor-summary__item:hover span:nth-child(2) {
  text-decoration: underline;
}

.editor-summary__item:focus-visible {
  border-radius: var(--radius-xs);
  outline: var(--focus-ring);
  outline-offset: var(--focus-ring-offset);
}

.editor-summary__field,
.editor-summary__code {
  color: var(--label-1);
}

.editor-layout {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(320px, 400px);
  gap: var(--space-4);
  align-items: start;
}

.editor-form {
  display: grid;
  gap: var(--space-4);
  min-width: 0;
}

.editor-preview {
  position: sticky;
  top: calc(var(--shell-topbar-height, 52px) + var(--space-4));
  min-width: 0;
}

.editor-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: var(--space-3);
  align-items: start;
}

.editor-grid--3 {
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
}

.editor-grid--4 {
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
}

.editor-mt {
  margin-top: var(--space-3);
}

.editor-port {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
}

.editor-labels {
  display: grid;
  gap: var(--space-2);
  justify-items: start;
}

.editor-label {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr) auto;
  gap: var(--space-1);
  align-items: center;
  width: 100%;
  color: var(--label-2);
}

.editor-chain {
  display: grid;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}

.editor-link {
  display: flex;
  gap: var(--space-2);
  align-items: center;
  padding-left: var(--space-4);
  color: var(--label-2);
}

.editor-targets-anchor {
  padding-left: var(--space-4);
}

.editor-targets {
  display: grid;
  gap: var(--space-2);
}

.editor-targets__head,
.editor-target {
  display: grid;
  grid-template-columns: minmax(0, 3fr) minmax(0, 1.2fr) minmax(0, 1fr) minmax(0, 1fr) 36px;
  gap: var(--space-2);
  align-items: start;
}

.editor-targets__head {
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

.editor-subhead {
  display: grid;
  gap: var(--space-1);
  margin: var(--space-5) 0 var(--space-3);
}

.editor-subhead__title {
  margin: 0;
  font-size: var(--type-body-size);
  font-weight: var(--weight-semibold);
}

.editor-diff {
  display: grid;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}

.editor-diff__row {
  display: grid;
  gap: var(--space-1);
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-sm);
  background: var(--fill-1);
}

.editor-diff__row.is-overlap {
  box-shadow: inset 2px 0 0 var(--warning);
}

.editor-diff__values {
  display: grid;
  gap: 2px;
  overflow-wrap: anywhere;
}

.editor-phonebar {
  display: none;
}

@media (max-width: 1099.98px) {
  .editor-layout {
    grid-template-columns: minmax(0, 1fr);
  }

  .editor-preview {
    position: static;
  }
}

@media (max-width: 639.98px) {
  .editor-phonebar {
    position: sticky;
    bottom: 0;
    z-index: var(--z-sticky);
    display: flex;
    gap: var(--space-3);
    align-items: center;
    margin: 0;
    padding: var(--space-2) var(--space-3) calc(var(--space-2) + env(safe-area-inset-bottom));
    border-top: 1px solid var(--separator);
    background: var(--material);
    backdrop-filter: blur(20px);
  }

  .editor-phonebar__link {
    margin-left: auto;
    color: var(--accent);
    text-decoration: none;
  }

  .editor-targets__head {
    display: none;
  }

  .editor-target {
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr) minmax(0, 1fr) 36px;
    padding-bottom: var(--space-2);
    border-bottom: 1px solid var(--separator);
  }

  .editor-target > :first-child {
    grid-column: 1 / -1;
  }
}
</style>
