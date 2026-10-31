<template>
  <div class="settings-section" data-settings-panel="general">
    <UiSection :title="t('adminSettings.general.subscription.title')" :description="t('adminSettings.general.subscription.description')">
      <UiSkeleton v-if="showSubscriptionSkeleton" variant="card" :label="t('adminSettings.loading')" />
      <UiErrorState
        v-else-if="subscriptionLoadError"
        compact
        heading-tag="h3"
        :title="t('adminSettings.general.subscription.loadFailed')"
        :error="subscriptionLoadError"
        @retry="loadSubscriptionDomainSettings"
      />
      <template v-else-if="!subscriptionSettingsLoading">
        <UiGroupedList>
          <UiGroupedListRow :label="t('adminSettings.general.subscription.path')" :value="subscriptionPath" />
          <UiGroupedListRow :label="t('adminSettings.general.subscription.currentHost')" :value="subscriptionCurrentHost || '—'" />
        </UiGroupedList>
        <UiGroupedList :footer="t('adminSettings.general.subscription.domainsHelp')">
          <UiGroupedListRow stacked>
            <UiTextarea
              id="settings-subscription-domains"
              v-model="subscriptionDomainsText"
              class="settings-domains"
              :rows="4"
              :label="t('adminSettings.general.subscription.domains')"
              :placeholder="t('adminSettings.general.subscription.domainsPlaceholder')"
              :error="subscriptionDomainFieldError"
              data-test="subscription-domains"
            />
          </UiGroupedListRow>
        </UiGroupedList>
        <UiGroupedList :title="t('adminSettings.general.subscription.preview')">
          <UiGroupedListRow v-if="normalizedSubscriptionDomains.length === 0" :label="t('adminSettings.general.subscription.previewEmpty')" />
          <UiGroupedListRow v-for="domain in normalizedSubscriptionDomains" v-else :key="domain">
            <template #label><code class="settings-mono">{{ `${subscriptionPreviewProtocol}//${domain}${subscriptionPath}` }}</code></template>
          </UiGroupedListRow>
        </UiGroupedList>
        <p v-if="subscriptionDomainError" class="form-error" role="alert" data-test="subscription-domain-error">{{ subscriptionDomainError }}</p>
      </template>
    </UiSection>

    <UiSection :title="t('adminSettings.general.links.title')" :description="t('adminSettings.general.links.description')">
      <UiGroupedList>
        <UiGroupedListRow
          v-for="link in settingsLinks"
          :key="link.to"
          @click="router.push(link.to)"
          :label="t(link.labelKey)"
          :value="t(link.valueKey)"
          :data-test="link.test"
        />
      </UiGroupedList>
    </UiSection>


    <SettingsSaveBar
      :visible="subscriptionDirty"
      :saving="subscriptionSettingsSaving"
      :invalid="Boolean(subscriptionDomainFieldError)"
      @save="saveSubscriptionDomainSettings"
      @discard="discardSubscriptionDomains"
    />
  </div>
</template>

<script setup>
// 系统设置 → 通用: the subscription domains (an edit with the save bar) and
// the links to the pages that hold the other everyday settings (mail,
// sign-in security, invites). The raw configuration keys are in 高级.
// Endpoints: GET /admin/system/subscription-settings,
// GET/PUT/DELETE /admin/system/configs/app.subscribe_domains.
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  deleteSystemConfig,
  getSubscriptionSettings,
  getSystemConfig,
  setSystemConfig
} from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import { useUnsavedChanges } from '@/composables/useUnsavedChanges'
import SettingsSaveBar from '@/components/admin/settings/SettingsSaveBar.vue'
import UiErrorState from '@/ui/UiErrorState.vue'
import UiGroupedList from '@/ui/UiGroupedList.vue'
import UiGroupedListRow from '@/ui/UiGroupedListRow.vue'
import UiSection from '@/ui/UiSection.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import UiTextarea from '@/ui/UiTextarea.vue'
import { useDelayedLoading } from '@/ui/composables/useDelayedLoading'
import { useToast } from '@/ui/composables/useToast'
import { ensureSystemMutation, systemErrorText } from './systemResponse'

const { t, translateLiteral } = useAppI18n()
const router = useRouter()
const toast = useToast()

// The everyday settings that live on their own pages.
const settingsLinks = [
  { to: '/admin/notifications/email', labelKey: 'adminSettings.general.links.mail', valueKey: 'adminSettings.general.links.mailHint', test: 'settings-link-mail' },
  { to: '/admin/security/mfa', labelKey: 'adminSettings.general.links.security', valueKey: 'adminSettings.general.links.securityHint', test: 'settings-link-security' },
]

const subscriptionDomainsConfigKey = 'app.subscribe_domains'
const subscriptionPath = ref('/s')
const subscriptionCurrentHost = ref(typeof window !== 'undefined' ? window.location.host : '')
const subscriptionDomainsText = ref('')
const savedSubscriptionDomainsText = ref('')
const subscriptionSettingsLoading = ref(false)
const subscriptionSettingsSaving = ref(false)
const subscriptionLoadError = ref(null)
const subscriptionDomainError = ref('')
const showSubscriptionSkeleton = useDelayedLoading(subscriptionSettingsLoading)

const translateText = (value, fallback = '-') => {
  const text = String(value ?? '').trim()
  return text ? translateLiteral(text) : fallback
}
const resolveSystemError = (error, fallbackKey) => translateText(systemErrorText(error), t(fallbackKey))

// Subscription domains -------------------------------------------------------

function splitDomainInput(value) {
  const text = String(value || '').trim()
  if (!text) return []
  return text.replace(/\r\n/g, '\n').replace(/\r/g, '\n').replace(/[,;]/g, '\n')
    .split('\n').map(item => item.trim()).filter(Boolean)
}

function hostOf(line) {
  try {
    const url = new URL(line.includes('://') ? line : `https://${line}`)
    return (url.host || line).trim().replace(/\/+$/, '')
  } catch {
    return line.trim().replace(/\/+$/, '')
  }
}

const parseSubscriptionDomainInput = (value) => {
  const unique = []
  const seen = new Set()
  for (const line of splitDomainInput(value)) {
    const host = hostOf(line)
    if (!host || seen.has(host)) continue
    seen.add(host)
    unique.push(host)
  }
  return unique
}

const HOST_PATTERN = /^(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)*[a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?::\d{1,5})?$|^\[[0-9a-f:.]+\](?::\d{1,5})?$/i

const normalizedSubscriptionDomains = computed(() => parseSubscriptionDomainInput(subscriptionDomainsText.value))
const invalidSubscriptionDomains = computed(() => splitDomainInput(subscriptionDomainsText.value).filter(line => !HOST_PATTERN.test(hostOf(line))))
const subscriptionDomainFieldError = computed(() => (
  invalidSubscriptionDomains.value.length
    ? t('adminSettings.general.subscription.invalidDomains', { domains: invalidSubscriptionDomains.value.join(', ') })
    : ''
))
const subscriptionDirty = computed(() => subscriptionDomainsText.value !== savedSubscriptionDomainsText.value)
const subscriptionPreviewProtocol = computed(() => {
  if (typeof window === 'undefined') return 'https:'
  return window.location.protocol || 'https:'
})

const readSubscriptionSettings = (res) => {
  if (!res || typeof res !== 'object') return { subscribe_path: '/s', subscribe_domains: [] }
  const payload = Object.prototype.hasOwnProperty.call(res, 'code') ? res.data : (res.data ?? res)
  if (!payload || typeof payload !== 'object') return { subscribe_path: '/s', subscribe_domains: [] }
  return {
    subscribe_path: payload.subscribe_path || '/s',
    subscribe_domains: Array.isArray(payload.subscribe_domains) ? payload.subscribe_domains : []
  }
}

function setLoadedDomains(text) {
  subscriptionDomainsText.value = text
  savedSubscriptionDomainsText.value = text
}

const loadSubscriptionDomainSettings = async () => {
  subscriptionSettingsLoading.value = true
  subscriptionLoadError.value = null
  subscriptionDomainError.value = ''
  try {
    const [settingsRes, rawConfigRes] = await Promise.allSettled([
      getSubscriptionSettings(),
      getSystemConfig(subscriptionDomainsConfigKey)
    ])

    if (settingsRes.status === 'fulfilled') {
      const payload = readSubscriptionSettings(settingsRes.value)
      subscriptionPath.value = payload.subscribe_path || '/s'
    }

    if (rawConfigRes.status === 'fulfilled') {
      const rawValue = rawConfigRes.value.data?.value
      if (typeof rawValue === 'string' && rawValue.trim()) {
        try {
          const parsed = JSON.parse(rawValue)
          setLoadedDomains(Array.isArray(parsed) ? parsed.join('\n') : rawValue)
        } catch {
          setLoadedDomains(rawValue)
        }
      } else {
        const settingsPayload = settingsRes.status === 'fulfilled' ? readSubscriptionSettings(settingsRes.value) : {}
        const domains = Array.isArray(settingsPayload.subscribe_domains) ? settingsPayload.subscribe_domains : []
        setLoadedDomains(domains.join('\n'))
      }
    } else if (settingsRes.status === 'fulfilled') {
      const settingsPayload = readSubscriptionSettings(settingsRes.value)
      const domains = Array.isArray(settingsPayload.subscribe_domains) ? settingsPayload.subscribe_domains : []
      setLoadedDomains(domains.join('\n'))
    } else {
      throw rawConfigRes.reason || settingsRes.reason || new Error('failed to load subscription settings')
    }
  } catch (err) {
    subscriptionLoadError.value = resolveSystemError(err, 'adminSettings.general.subscription.loadFailed')
  } finally {
    subscriptionSettingsLoading.value = false
  }
}

const saveSubscriptionDomainSettings = async () => {
  if (subscriptionSettingsSaving.value || subscriptionDomainFieldError.value) return
  subscriptionSettingsSaving.value = true
  subscriptionDomainError.value = ''
  const failed = t('adminSettings.general.subscription.saveFailed')
  try {
    const domains = normalizedSubscriptionDomains.value
    if (domains.length === 0) {
      try {
        await ensureSystemMutation(deleteSystemConfig(subscriptionDomainsConfigKey), failed)
      } catch {
        await ensureSystemMutation(
          setSystemConfig(subscriptionDomainsConfigKey, {
            value: '',
            type: 'string',
            group: 'app',
            description: 'Alternate subscription domains'
          }),
          failed
        )
      }
    } else {
      await ensureSystemMutation(
        setSystemConfig(subscriptionDomainsConfigKey, {
          value: JSON.stringify(domains),
          type: 'json',
          group: 'app',
          description: 'Alternate subscription domains'
        }),
        failed
      )
    }
    await loadSubscriptionDomainSettings()
    toast.success(t('adminSettings.general.subscription.saved'))
  } catch (err) {
    subscriptionDomainError.value = resolveSystemError(err, 'adminSettings.general.subscription.saveFailed')
  } finally {
    subscriptionSettingsSaving.value = false
  }
}

function discardSubscriptionDomains() {
  subscriptionDomainsText.value = savedSubscriptionDomainsText.value
  subscriptionDomainError.value = ''
}

useUnsavedChanges(subscriptionDirty, { discard: discardSubscriptionDomains })

onMounted(loadSubscriptionDomainSettings)

</script>

<style scoped>
.settings-section {
  display: flex;
  flex-direction: column;
  gap: var(--space-12);
  min-width: 0;
}

.settings-section :deep(.ui-section) {
  gap: var(--space-5);
}

.settings-domains {
  width: 100%;
}

.settings-mono,
.settings-mono-field :deep(input),
.settings-mono-field :deep(textarea) {
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
  overflow-wrap: anywhere;
}
</style>
