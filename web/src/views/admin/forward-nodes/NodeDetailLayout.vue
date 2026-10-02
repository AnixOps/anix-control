<template>
  <div class="node-detail">
    <UiPageHeader :title="title" :description="description">
      <template #back>
        <RouterLink :to="backTo" class="node-detail__back" data-test="node-detail-back">
          <UiIcon :icon="ChevronLeft" :size="16" />
          <span>{{ backLabel }}</span>
        </RouterLink>
      </template>
      <template v-if="$slots.meta" #meta><slot name="meta" /></template>
      <template v-if="$slots.actions && ready" #actions><slot name="actions" /></template>
    </UiPageHeader>

    <UiSkeleton v-if="showSkeleton" variant="card" :lines="6" />
    <UiErrorState v-else-if="error" :title="errorTitle" :error="error" @retry="emit('retry')" />
    <UiEmptyState v-else-if="notFound" :icon="SearchX" :title="notFoundText" heading-tag="h2">
      <template #actions>
        <UiButton :as="RouterLink" :to="backTo">{{ backLabel }}</UiButton>
      </template>
    </UiEmptyState>
    <UiTabs
      v-else-if="ready"
      :model-value="tab"
      :items="tabs"
      :aria-label="sectionsLabel"
      data-test="node-detail-tabs"
      @update:model-value="value => emit('update:tab', value)"
    >
      <template v-for="item in tabs" :key="item.value" #[item.value]>
        <div class="node-detail__panel">
          <slot :name="item.value" />
        </div>
      </template>
    </UiTabs>
  </div>
</template>

<script setup>
// Detail page template of plan §7.2 for the forward node pages (UI U7):
// back link + title + status badges + actions, then UiTabs sections
// (概览 / 配置 / 危险操作) holding grouped lists. Loading shows a skeleton
// after 300 ms, a failed load UiErrorState with 重试, a missing record an
// empty state that leads back to the list.
import { computed, toRef } from 'vue'
import { RouterLink } from 'vue-router'
import { ChevronLeft, SearchX } from '@lucide/vue'
import UiButton from '@/ui/UiButton.vue'
import UiEmptyState from '@/ui/UiEmptyState.vue'
import UiErrorState from '@/ui/UiErrorState.vue'
import UiIcon from '@/ui/UiIcon.vue'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import UiTabs from '@/ui/UiTabs.vue'
import { useDelayedLoading } from '@/ui/composables/useDelayedLoading'

const props = defineProps({
  title: { type: String, default: '' },
  description: { type: String, default: '' },
  backTo: { type: String, required: true },
  backLabel: { type: String, required: true },
  sectionsLabel: { type: String, required: true },
  tabs: { type: Array, required: true },
  tab: { type: String, required: true },
  loading: { type: Boolean, default: false },
  loaded: { type: Boolean, default: false },
  error: { type: [Object, String, null], default: null },
  errorTitle: { type: String, default: '' },
  notFound: { type: Boolean, default: false },
  notFoundText: { type: String, default: '' }
})
const emit = defineEmits(['update:tab', 'retry'])

const showSkeleton = useDelayedLoading(toRef(() => props.loading && !props.loaded))
const ready = computed(() => props.loaded && !props.error && !props.notFound)
</script>

<style scoped>
.node-detail {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
  min-width: 0;
}

.node-detail__back {
  display: inline-flex;
  gap: var(--space-1);
  align-items: center;
  border-radius: var(--radius-xs);
  color: var(--accent);
  font-size: var(--type-callout-size);
  text-decoration: none;
}

.node-detail__back:hover {
  text-decoration: underline;
}

.node-detail__back:focus-visible {
  outline: var(--focus-ring);
  outline-offset: var(--focus-ring-offset);
}

.node-detail__panel {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
  max-width: 880px;
}

/* Touch: a 44 px hit area around the link without changing the look. */
@media (pointer: coarse) {
  .node-detail__back {
    position: relative;
  }

  .node-detail__back::after {
    position: absolute;
    top: 50%;
    left: 50%;
    width: max(100%, var(--size-control-lg));
    height: max(100%, var(--size-control-lg));
    transform: translate(-50%, -50%);
    content: '';
  }
}
</style>
