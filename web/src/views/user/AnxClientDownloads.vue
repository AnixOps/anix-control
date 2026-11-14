<template>
  <section
    v-if="release"
    class="subscribe-section"
    aria-labelledby="subscribe-anx-title"
    data-subscribe-anx
  >
    <div class="subscribe-section__head">
      <h2 id="subscribe-anx-title" class="subscribe-section__title">{{ t('portal.subscribe.anx.title') }}</h2>
      <span class="subscribe-section__hint">{{ t('portal.subscribe.anx.hint', { version: release.version }) }}</span>
    </div>
    <ul class="anx-downloads">
      <li v-for="group in groups" :key="group.os" class="anx-download" :data-anx-os="group.os">
        <span class="anx-download__name">{{ t(`portal.subscribe.anx.os.${group.os}`) }}</span>
        <template v-if="group.os === 'ios'">
          <UiButton
            v-if="release.testflight_url"
            :href="release.testflight_url"
            size="sm"
            block
            data-anx-testflight
          >
            {{ t('portal.subscribe.anx.testflight') }}
          </UiButton>
          <span v-else class="anx-download__note">{{ t('portal.subscribe.anx.testflightAsk') }}</span>
        </template>
        <template v-else>
          <UiButton
            v-for="file in group.files"
            :key="file.name"
            :href="file.url"
            :icon="Download"
            size="sm"
            block
            variant="secondary"
            :aria-label="t('portal.subscribe.anx.downloadLabel', { os: t(`portal.subscribe.anx.os.${group.os}`), name: t(`portal.subscribe.anx.kind.${file.platform}`) })"
            :data-anx-file="file.platform"
          >
            {{ t(`portal.subscribe.anx.kind.${file.platform}`) }} · {{ shortBytes(file.size) }}
          </UiButton>
        </template>
      </li>
    </ul>
    <p class="anx-downloads__foot">
      {{ t('portal.subscribe.anx.verify') }}
      <a :href="release.sha256sums_url" data-anx-sums>SHA256SUMS</a>
    </p>
  </section>
</template>

<script setup>
// The ANX edition of AnixOps Net for this subscriber (GET
// /{subscribe_path}/{token}/client, docs/reference/client-downloads.md):
// the newest release Control holds, grouped by system, downloaded with the
// subscription token. Hidden when Control holds none or the request fails,
// so a panel without client releases looks as before.
import { computed, ref, watch } from 'vue'
import { Download } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'
import UiButton from '@/ui/UiButton.vue'
import { useFormat } from '@/ui/composables/useFormat'

const props = defineProps({
  // The subscription link on this page's own origin, without a format.
  link: { type: String, default: '' }
})

const { t } = useAppI18n()
const format = useFormat()
const release = ref(null)

const OS_ORDER = ['windows', 'macos', 'linux', 'android', 'ios']

function shortBytes(value) {
  return format.bytes(value, { precision: 1, empty: '0 B' }).replace(/[.,]0(?= )/, '')
}

const groups = computed(() => {
  if (!release.value) return []
  const files = Array.isArray(release.value.files) ? release.value.files : []
  return OS_ORDER
    .map(os => ({ os, files: files.filter(file => file.os === os && file.platform !== 'ios') }))
    .filter(group => group.files.length > 0 || (group.os === 'ios' && files.some(file => file.os === 'ios')))
})

async function load(link) {
  release.value = null
  if (!link) return
  try {
    const response = await fetch(`${link}/client`, { headers: { Accept: 'application/json' } })
    if (!response?.ok) return
    const body = await response.json()
    if (body?.data?.version && Array.isArray(body.data.files) && body.data.files.length > 0) {
      release.value = body.data
    }
  } catch {
    release.value = null
  }
}

watch(() => props.link, load, { immediate: true })
</script>

<style scoped>
.anx-downloads {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: var(--space-4);
  list-style: none;
}

.anx-download {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-5);
  border-radius: var(--radius-md);
  background: var(--bg-elevated);
  box-shadow: 0 0 0 0.5px var(--separator), var(--shadow-1);
}

.anx-download__name {
  font-weight: 600;
}

.anx-download__note,
.anx-downloads__foot {
  color: var(--label-2);
  font-size: var(--type-caption-size);
}
</style>
