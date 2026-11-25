<template>
  <UiSection
    :title="t('nodeTemporaryProxies.title')"
    :description="t('nodeTemporaryProxies.description')"
    heading-tag="h2"
    data-testid="node-temporary-proxies"
  >
    <UiSkeleton v-if="loading" variant="card" :label="t('nodeTemporaryProxies.loading')" />
    <UiErrorState v-else-if="loadError" :title="t('nodeTemporaryProxies.loadFailed')" :error="loadError" @retry="load" />
    <div v-else class="node-tmp">
      <form class="node-tmp__form" novalidate @submit.prevent="open">
        <UiTextField
          v-model="form.listen"
          size="md"
          :label="t('nodeTemporaryProxies.listen')"
          placeholder="10.126.126.1"
          :help="t('nodeTemporaryProxies.listenHelp')"
          data-testid="node-temporary-proxies-listen"
        />
        <div class="node-tmp__pair">
          <UiNumberField
            v-model="form.port"
            size="md"
            :label="t('nodeTemporaryProxies.port')"
            :min="1"
            :max="65535"
            data-testid="node-temporary-proxies-port"
          />
          <UiNumberField
            v-model="form.minutes"
            size="md"
            :label="t('nodeTemporaryProxies.minutes')"
            :unit="t('nodeTemporaryProxies.minutesUnit')"
            :min="1"
            :max="1440"
            data-testid="node-temporary-proxies-minutes"
          />
        </div>
        <p v-if="openError" class="node-tmp__error" role="alert">{{ openError }}</p>
        <div>
          <UiButton type="submit" variant="primary" :loading="opening" :disabled="opening || !canOpen" data-testid="node-temporary-proxies-open">
            {{ t('nodeTemporaryProxies.open') }}
          </UiButton>
        </div>
      </form>

      <div v-if="opened" class="node-tmp__opened" role="status" data-testid="node-temporary-proxies-opened">
        <p class="node-tmp__hint">{{ t('nodeTemporaryProxies.passwordOnce') }}</p>
        <UiCopyField size="md" :label="t('nodeTemporaryProxies.address')" :value="hostPort(opened)" />
        <UiCopyField size="md" :label="t('nodeTemporaryProxies.username')" :value="opened.username" />
        <UiCopyField size="md" :label="t('nodeTemporaryProxies.password')" :value="opened.password" secret />
      </div>

      <p v-if="!proxies.length" class="node-tmp__hint" data-testid="node-temporary-proxies-empty">{{ t('nodeTemporaryProxies.none') }}</p>
      <ul v-else class="node-tmp__list" data-testid="node-temporary-proxies-list">
        <li v-for="proxy in proxies" :key="proxy.id" class="node-tmp__row">
          <div class="node-tmp__text">
            <span class="node-tmp__address">{{ hostPort(proxy) }}</span>
            <span class="node-tmp__meta">{{ t('nodeTemporaryProxies.row', { user: proxy.username, date: format.dateTime(proxy.expires_at) }) }}</span>
          </div>
          <UiButton
            size="sm"
            variant="danger-soft"
            :loading="closing === proxy.id"
            :disabled="closing !== null"
            :data-testid="`node-temporary-proxies-close-${proxy.id}`"
            @click="close(proxy)"
          >
            {{ t('nodeTemporaryProxies.close') }}
          </UiButton>
        </li>
      </ul>
      <p v-if="closeError" class="node-tmp__error" role="alert">{{ closeError }}</p>
    </div>
  </UiSection>
</template>

<script setup>
// 临时 SOCKS5 代理 on the node page's 协议 tab: a SOCKS5 proxy the node
// serves on an internal address (its mesh address or a LAN address) for a
// limited time, so that devices of that network send traffic out through the
// node. Control generates the username and password; the password is shown
// once, in the answer that opened the proxy. The node's kernel closes the
// proxy at its end time by itself.
// GET/POST /api/v4/kernel/nodes/:id/temporary-proxies,
// DELETE /api/v4/kernel/nodes/:id/temporary-proxies/:proxy_id
import { computed, onMounted, reactive, ref } from 'vue'
import {
  closeKernelNodeTemporaryProxy,
  listKernelNodeTemporaryProxies,
  openKernelNodeTemporaryProxy
} from '@/api/kernel'
import { useAppI18n } from '@/composables/useAppI18n'
import { useFormat } from '@/ui/composables/useFormat'
import UiButton from '@/ui/UiButton.vue'
import UiCopyField from '@/ui/UiCopyField.vue'
import UiErrorState from '@/ui/UiErrorState.vue'
import UiNumberField from '@/ui/UiNumberField.vue'
import UiSection from '@/ui/UiSection.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import UiTextField from '@/ui/UiTextField.vue'

const props = defineProps({
  nodeRef: { type: String, required: true }
})
const { t } = useAppI18n()
const format = useFormat()

const form = reactive({ listen: '', port: 1080, minutes: 15 })
const proxies = ref([])
const opened = ref(null)
const loading = ref(true)
const loadError = ref(null)
const opening = ref(false)
const openError = ref('')
const closing = ref(null)
const closeError = ref('')

const canOpen = computed(() => String(form.listen || '').trim() !== '' && Number(form.port) > 0)

function hostPort(proxy) {
  const host = String(proxy.listen || '').includes(':') ? `[${proxy.listen}]` : proxy.listen
  return `${host}:${proxy.port}`
}

function message(error, fallback) {
  return error?.response?.data?.error?.message || error?.message || fallback
}

async function load() {
  loading.value = true
  loadError.value = null
  try {
    proxies.value = (await listKernelNodeTemporaryProxies(props.nodeRef)) || []
  } catch (e) {
    loadError.value = e
  } finally {
    loading.value = false
  }
}

async function refresh() {
  try {
    proxies.value = (await listKernelNodeTemporaryProxies(props.nodeRef)) || []
  } catch {
    // The list keeps what it showed; the next load shows the error.
  }
}

async function open() {
  if (!canOpen.value) return
  opening.value = true
  openError.value = ''
  try {
    opened.value = await openKernelNodeTemporaryProxy(props.nodeRef, { ...form })
    await refresh()
  } catch (e) {
    openError.value = message(e, t('nodeTemporaryProxies.openFailed'))
  } finally {
    opening.value = false
  }
}

async function close(proxy) {
  closing.value = proxy.id
  closeError.value = ''
  try {
    await closeKernelNodeTemporaryProxy(props.nodeRef, proxy.id)
    if (opened.value?.id === proxy.id) opened.value = null
    proxies.value = proxies.value.filter(item => item.id !== proxy.id)
  } catch (e) {
    closeError.value = message(e, t('nodeTemporaryProxies.closeFailed'))
  } finally {
    closing.value = null
  }
}

onMounted(load)
</script>

<style scoped>
.node-tmp,
.node-tmp__form,
.node-tmp__opened {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.node-tmp__pair {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(10rem, 1fr));
  gap: var(--space-3);
}

.node-tmp__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}

.node-tmp__row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
}

.node-tmp__text {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.node-tmp__address {
  font-family: var(--font-mono);
  overflow-wrap: anywhere;
}

.node-tmp__meta,
.node-tmp__hint,
.node-tmp__error {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.node-tmp__error {
  color: var(--danger);
}
</style>
