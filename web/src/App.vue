<script setup>
import { computed, watchEffect } from 'vue'
import { useRoute } from 'vue-router'
import { useAppI18n } from '@/composables/useAppI18n'
import { resolveDocumentTitle, resolveRouteMetaDescription } from '@/utils/pageMeta'

const route = useRoute()
const { t, currentLocale } = useAppI18n()
const appName = computed(() => t('layout.user.brand'))

function upsertMetaDescription(content) {
  if (typeof document === 'undefined') return

  let meta = document.head.querySelector('meta[name="description"]')
  if (!meta) {
    meta = document.createElement('meta')
    meta.setAttribute('name', 'description')
    document.head.appendChild(meta)
  }
  meta.setAttribute('content', content)
}

watchEffect(() => {
  const locale = currentLocale.value
  const path = route.path
  const title = resolveDocumentTitle(t, path, appName.value)
  const description = resolveRouteMetaDescription(t, path, t('app.meta.defaultDescription'))

  if (typeof document !== 'undefined') {
    document.title = title
    document.documentElement.lang = locale
  }
  upsertMetaDescription(description)
})
</script>

<template>
  <a class="skip-link" href="#app-main-content">{{ t('common.a11y.skipToContent') }}</a>
  <router-view></router-view>
</template>
