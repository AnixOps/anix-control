<template>
  <UiSheet
    :open="open"
    size="lg"
    :title="editing ? t('reverseProxies.sheet.editTitle', { name: proxy.name }) : t('reverseProxies.sheet.createTitle')"
    :description="t('reverseProxies.sheet.description')"
    :dismissible="!saving"
    @update:open="value => emit('update:open', value)"
  >
    <form class="rp-sheet" novalidate data-testid="reverse-proxy-form" @submit.prevent="save">
      <UiTextField v-model="form.name" size="md" :label="t('reverseProxies.sheet.name')" required :placeholder="t('reverseProxies.sheet.namePlaceholder')" :error="errors.name" />
      <UiSelect v-model="form.entryNodeId" size="md" :label="t('reverseProxies.sheet.entry')" :options="nodeOptions" required :help="t('reverseProxies.sheet.entryHelp')" :error="errors.entry" data-testid="reverse-proxy-entry" />
      <UiSelect v-model="form.viaNodeId" size="md" :label="t('reverseProxies.sheet.via')" :options="viaOptions" :help="t('reverseProxies.sheet.viaHelp')" :error="errors.via" data-testid="reverse-proxy-via" />
      <p v-if="form.extraEntries.length || form.extraVia.length" class="fwd-note">{{ t('reverseProxies.sheet.extraNodes', { nodes: [...form.extraEntries, ...form.extraVia].join(', ') }) }}</p>
      <UiNumberField v-model="form.listenPort" size="md" :label="t('reverseProxies.sheet.listenPort')" :min="1" :max="65535" required :error="errors.listenPort" />

      <UiSegmentedControl v-model="form.tlsMode" :options="tlsOptions" :aria-label="t('reverseProxies.sheet.tls')" />
      <UiNumberField v-if="form.tlsMode !== 'none'" v-model="form.httpPort" size="md" :label="t('reverseProxies.sheet.httpPort')" :min="0" :max="65535" :help="t('reverseProxies.sheet.httpPortHelp')" :error="errors.httpPort" />
      <template v-if="form.tlsMode === 'acme'">
        <UiSelect v-model="form.dnsProviderId" size="md" :label="t('reverseProxies.sheet.dnsProvider')" :options="dnsProviderOptions" required :help="dnsProviderOptions.length ? t('reverseProxies.sheet.dnsProviderHelp') : t('reverseProxies.sheet.dnsProviderNone')" :error="errors.dnsProvider" data-testid="reverse-proxy-dns-provider" />
        <UiTextField v-model="form.dnsZone" size="md" :label="t('reverseProxies.sheet.dnsZone')" required placeholder="example.com" :help="t('reverseProxies.sheet.dnsZoneHelp')" :error="errors.dnsZone" />
        <p v-if="form.hasCertificate" class="fwd-note">{{ t('reverseProxies.sheet.acmeIssued', { names: (proxy?.certificate_names || []).join(', ') }) }}</p>
        <p v-if="proxy?.certificate_error" class="fwd-note is-danger">{{ t('reverseProxies.sheet.acmeError', { error: proxy.certificate_error }) }}</p>
      </template>
      <template v-if="form.tlsMode === 'manual'">
        <p v-if="form.hasCertificate" class="fwd-note">{{ t('reverseProxies.sheet.certificateKept', { names: (proxy?.certificate_names || []).join(', ') }) }}</p>
        <UiTextarea v-model="form.certificatePem" :rows="4" :label="t('reverseProxies.sheet.certificate')" :required="!form.hasCertificate" placeholder="-----BEGIN CERTIFICATE-----" :help="t('reverseProxies.sheet.certificateHelp')" :error="errors.certificatePem" />
        <UiTextarea v-model="form.privateKeyPem" :rows="4" :label="t('reverseProxies.sheet.privateKey')" :required="!form.hasCertificate" placeholder="-----BEGIN PRIVATE KEY-----" :help="t('reverseProxies.sheet.privateKeyHelp')" :error="errors.privateKeyPem" />
      </template>

      <fieldset v-for="(site, i) in form.sites" :key="`site-${i}`" class="rp-sheet__site" :data-testid="`reverse-proxy-site-${i}`">
        <legend class="rp-sheet__legend">
          {{ t('reverseProxies.sheet.site', { n: i + 1 }) }}
          <UiIconButton v-if="form.sites.length > 1" :icon="Trash2" size="sm" :label="t('reverseProxies.sheet.removeSite', { n: i + 1 })" @click="form.sites.splice(i, 1)" />
        </legend>
        <UiTextarea v-model="site.hosts" :rows="2" :label="t('reverseProxies.sheet.hosts')" required placeholder="example.com&#10;*.example.com" :help="t('reverseProxies.sheet.hostsHelp')" :error="errors[`sites.${i}.hosts`]" />
        <div v-for="(route, j) in site.routes" :key="`route-${i}-${j}`" class="rp-sheet__route">
          <div class="rp-sheet__route-head">
            <span class="rp-sheet__route-title">{{ t('reverseProxies.sheet.route', { n: j + 1 }) }}</span>
            <UiIconButton v-if="site.routes.length > 1" :icon="Trash2" size="sm" :label="t('reverseProxies.sheet.removeRoute', { n: j + 1 })" @click="site.routes.splice(j, 1)" />
          </div>
          <UiTextField v-model="route.path" size="md" :label="t('reverseProxies.sheet.path')" placeholder="/" :help="t('reverseProxies.sheet.pathHelp')" :error="errors[`sites.${i}.routes.${j}.path`]" />
          <UiSwitch v-if="route.path.trim() && route.path.trim() !== '/'" v-model="route.stripPrefix" :label="t('reverseProxies.sheet.stripPrefix')" />
          <UiTextarea v-model="route.upstreams" :rows="2" :label="t('reverseProxies.sheet.upstreams')" required placeholder="10.0.0.5:8080" :help="t('reverseProxies.sheet.upstreamsHelp')" :error="errors[`sites.${i}.routes.${j}.upstreams`]" />
          <UiSelect v-model="route.balance" size="md" :label="t('reverseProxies.sheet.balance')" :options="balanceOptions" />
          <UiSelect v-model="route.hostHeader" size="md" :label="t('reverseProxies.sheet.hostHeader')" :options="hostHeaderOptions" />
          <UiTextField v-if="route.hostHeader === 'custom'" v-model="route.host" size="md" :label="t('reverseProxies.sheet.fixedHost')" placeholder="origin.example.com" :error="errors[`sites.${i}.routes.${j}.host`]" />
        </div>
        <UiButton size="sm" variant="ghost" :icon="Plus" @click="site.routes.push(emptyRoute())">{{ t('reverseProxies.sheet.addRoute') }}</UiButton>
      </fieldset>
      <UiButton size="sm" variant="ghost" :icon="Plus" data-testid="reverse-proxy-add-site" @click="form.sites.push(emptySite())">{{ t('reverseProxies.sheet.addSite') }}</UiButton>

      <UiSwitch v-model="form.enabled" :label="t('reverseProxies.sheet.enabled')" />
      <p v-if="saveError" class="fwd-note is-danger" role="alert">{{ saveError }}</p>
    </form>
    <template #footer="{ close }">
      <UiButton :disabled="saving" @click="close">{{ t('forwardV4.common.cancel') }}</UiButton>
      <UiButton variant="primary" :loading="saving" data-testid="reverse-proxy-save" @click="save">{{ editing ? t('reverseProxies.sheet.save') : t('reverseProxies.sheet.create') }}</UiButton>
    </template>
  </UiSheet>
</template>

<script setup>
// The reverse proxy sheet: the entry node (and, optionally, the node the
// upstreams are dialed from, reached over Control's ANX-A link), the
// listen port, the certificate (obtained by Control over ACME DNS-01
// through a DNS provider, or the administrator's) and the sites with their
// path routes. The private key is never shown; an edit without one keeps
// the stored pair.
import { computed, ref, watch } from 'vue'
import { Plus, Trash2 } from '@lucide/vue'
import { UiButton, UiIconButton, UiNumberField, UiSegmentedControl, UiSelect, UiSheet, UiSwitch, UiTextarea, UiTextField, useToast } from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import { createReverseProxy, updateReverseProxy } from '@/api/kernel'
import { emptyRoute, emptySite, reverseProxyErrorMessage, reverseProxyForm, reverseProxyRequest, validateReverseProxy } from './reverseProxyModel'
import './forward.css'

const props = defineProps({
  open: { type: Boolean, default: false },
  // null for a new proxy.
  proxy: { type: Object, default: null },
  // The proxy nodes: [{ id, name, host }].
  nodes: { type: Array, default: () => [] },
  // The forwarding DNS providers: [{ id, name, kind }].
  dnsProviders: { type: Array, default: () => [] }
})
const emit = defineEmits(['update:open', 'saved'])
const { t } = useAppI18n()
const toast = useToast()

const editing = computed(() => Boolean(props.proxy?.id))
const form = ref(reverseProxyForm(null))
const errors = ref({})
const saving = ref(false)
const saveError = ref('')

watch(() => props.open, value => {
  if (!value) return
  form.value = reverseProxyForm(props.proxy)
  errors.value = {}
  saveError.value = ''
}, { immediate: true })

const nodeOptions = computed(() => props.nodes.map(node => ({
  value: node.id,
  label: `${node.name || `#${node.id}`}${node.host ? ` · ${node.host}` : ''}`
})))
const dnsProviderOptions = computed(() => props.dnsProviders.map(provider => ({
  value: Number(provider.id),
  label: `${provider.name} · ${provider.kind}`
})))
const viaOptions = computed(() => [{ value: 0, label: t('reverseProxies.sheet.viaNone') }, ...nodeOptions.value])
const tlsOptions = computed(() => [
  { value: 'acme', label: t('reverseProxies.sheet.tlsAcme') },
  { value: 'manual', label: t('reverseProxies.sheet.tlsManual') },
  { value: 'none', label: t('reverseProxies.sheet.tlsNone') }
])
const balanceOptions = computed(() => [
  { value: '', label: t('reverseProxies.sheet.balanceDefault') },
  { value: 'least_conn', label: t('reverseProxies.sheet.balanceLeastConn') },
  { value: 'ip_hash', label: t('reverseProxies.sheet.balanceIpHash') },
  { value: 'failover', label: t('reverseProxies.sheet.balanceFailover') },
  { value: 'random', label: t('reverseProxies.sheet.balanceRandom') }
])
const hostHeaderOptions = computed(() => [
  { value: 'client', label: t('reverseProxies.sheet.hostClient') },
  { value: 'upstream', label: t('reverseProxies.sheet.hostUpstream') },
  { value: 'custom', label: t('reverseProxies.sheet.hostCustom') }
])

async function save() {
  if (saving.value) return
  errors.value = validateReverseProxy(form.value, {
    required: t('reverseProxies.sheet.required'),
    sameNode: t('reverseProxies.sheet.sameNode'),
    port: t('reverseProxies.sheet.badPort'),
    httpPort: t('reverseProxies.sheet.badHttpPort'),
    path: t('reverseProxies.sheet.badPath'),
    zone: t('reverseProxies.sheet.badZone'),
    outsideZone: host => t('reverseProxies.sheet.outsideZone', { host }),
    upstream: line => t('reverseProxies.sheet.badUpstream', { line })
  })
  if (Object.keys(errors.value).length) {
    saveError.value = t('reverseProxies.sheet.fixErrors')
    return
  }
  const body = reverseProxyRequest(form.value)
  saving.value = true
  saveError.value = ''
  try {
    const answer = editing.value ? await updateReverseProxy(props.proxy.id, body) : await createReverseProxy(body)
    toast.success(t(editing.value ? 'reverseProxies.toast.saved' : 'reverseProxies.toast.created', { name: body.name }))
    emit('saved', answer || null)
    emit('update:open', false)
  } catch (error) {
    saveError.value = reverseProxyErrorMessage(error)
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.rp-sheet {
  display: grid;
  gap: var(--space-4);
}

.rp-sheet > .ui-button {
  justify-self: start;
}

.rp-sheet__site {
  display: grid;
  gap: var(--space-3);
  margin: 0;
  padding: var(--space-3) var(--space-4) var(--space-4);
  border: 1px solid var(--separator);
  border-radius: var(--radius-md);
}

.rp-sheet__site > .ui-button {
  justify-self: start;
}

.rp-sheet__legend {
  display: inline-flex;
  gap: var(--space-2);
  align-items: center;
  padding: 0 var(--space-1);
  font-weight: var(--weight-semibold);
}

.rp-sheet__route {
  display: grid;
  gap: var(--space-3);
  padding-top: var(--space-3);
  border-top: 1px solid var(--separator);
}

.rp-sheet__route-head {
  display: flex;
  gap: var(--space-2);
  align-items: center;
  justify-content: space-between;
}

.rp-sheet__route-title {
  color: var(--label-2);
  font-size: var(--type-callout-size);
}
</style>
