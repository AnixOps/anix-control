<template>
  <component :is="standalone ? 'div' : 'section'" class="status-page" :class="{ 'is-standalone': standalone }" :data-status-page="kind">
    <header v-if="standalone" class="status-page__bar">
      <router-link :to="homePath" class="status-page__brand">
        <BrandLockup size="sm" />
      </router-link>
    </header>
    <component :is="standalone ? 'main' : 'div'" :id="standalone ? 'app-main-content' : undefined" class="status-page__main" :tabindex="standalone ? -1 : undefined">
      <UiEmptyState
        :icon="copy.icon"
        :title="copy.title"
        :description="copy.description"
        heading-tag="h1"
      >
        <template #actions>
          <UiButton v-if="canGoBack" variant="secondary" data-status-back @click="goBack">{{ t('shell.status.back') }}</UiButton>
          <UiButton variant="primary" :as="RouterLink" :to="homePath" data-status-home>{{ t('shell.status.home') }}</UiButton>
        </template>
      </UiEmptyState>
      <p class="status-page__code">{{ t('shell.status.code', { code: copy.code }) }}</p>
    </component>
  </component>
</template>

<script setup>
// 404 and 无权限 (plan §4.2, §14). Inside a shell (an unknown /admin/… or
// /user/… path) it is the page body; at the top level (an unknown path, or a
// signed-in user opening an admin page) it brings its own lockup bar and
// <main>. The URL stays as typed, so the address bar shows what was asked for.
import { computed } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import { FileQuestion, ShieldX } from '@lucide/vue'
import { useUserStore } from '@/stores/user'
import { useAppI18n } from '@/composables/useAppI18n'
import BrandLockup from '@/components/common/BrandLockup.vue'
import UiEmptyState from '@/ui/UiEmptyState.vue'
import UiButton from '@/ui/UiButton.vue'

const props = defineProps({
  kind: { type: String, default: 'not-found', validator: value => ['not-found', 'forbidden'].includes(value) },
  standalone: { type: Boolean, default: false }
})

const router = useRouter()
const userStore = useUserStore()
const { t } = useAppI18n()

const copy = computed(() => (props.kind === 'forbidden'
  ? { code: 403, icon: ShieldX, title: t('shell.status.forbidden.title'), description: t('shell.status.forbidden.description') }
  : { code: 404, icon: FileQuestion, title: t('shell.status.notFound.title'), description: t('shell.status.notFound.description') }))

const homePath = computed(() => {
  if (!userStore.isLoggedIn) return '/login'
  return userStore.isAdmin ? '/admin/dashboard' : '/user/dashboard'
})

// "返回上一页" only when there is an in-app page to go back to.
const canGoBack = computed(() => Boolean(router.options?.history?.state?.back))

function goBack() {
  router.back()
}
</script>

<style scoped>
.status-page {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 60vh;
}

.status-page.is-standalone {
  min-height: 100vh;
  min-height: 100dvh;
  justify-content: flex-start;
  background: var(--bg);
}

.status-page__bar {
  display: flex;
  align-items: center;
  width: 100%;
  height: 48px;
  padding: 0 var(--space-6);
  border-bottom: 1px solid var(--separator);
}

.status-page__brand {
  display: inline-flex;
  color: inherit;
  text-decoration: none;
}

.status-page__main {
  display: flex;
  flex: 1;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  width: 100%;
  padding: var(--space-10) var(--space-4);
}

.status-page__main:focus {
  outline: none;
}

.status-page__code {
  color: var(--label-2);
  font-size: var(--type-callout-size);
  font-variant-numeric: tabular-nums;
}

.status-page :deep(.ui-empty__title) {
  font-size: var(--type-title-2-size);
  line-height: var(--type-title-2-line);
}

@media (max-width: 833px) {
  .status-page__bar {
    padding: 0 var(--space-4);
  }
}
</style>
