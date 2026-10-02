<script setup>
import { ref } from 'vue'
import UiSkeleton from '../UiSkeleton.vue'
import UiCard from '../UiCard.vue'
import UiButton from '../UiButton.vue'
import { useDelayedLoading } from '../composables/useDelayedLoading'

const loading = ref(false)
const showSkeleton = useDelayedLoading(loading)

function load(ms) {
  loading.value = true
  setTimeout(() => { loading.value = false }, ms)
}
</script>

<template>
  <Story title="Skeleton" group="display" :layout="{ type: 'single', iframe: true }">
    <Variant title="Variants">
      <div class="story story-grouped">
        <span class="story-label">text</span>
        <UiCard><UiSkeleton :lines="3" /></UiCard>
        <span class="story-label">card</span>
        <div class="story-grid">
          <UiSkeleton variant="card" />
          <UiSkeleton variant="card" />
        </div>
        <span class="story-label">table-row</span>
        <UiCard :padded="false"><UiSkeleton variant="table-row" :rows="3" :columns="5" /></UiCard>
      </div>
    </Variant>
    <Variant title="300 ms rule">
      <div class="story">
        <div class="story-row">
          <UiButton @click="load(200)">快速请求（200 ms）</UiButton>
          <UiButton @click="load(1500)">慢请求（1.5 s）</UiButton>
        </div>
        <UiCard>
          <UiSkeleton v-if="showSkeleton" :lines="3" />
          <p v-else-if="loading" class="story-note">加载中，300 ms 内不显示骨架。</p>
          <p v-else>数据已加载。</p>
        </UiCard>
      </div>
    </Variant>
  </Story>
</template>

<docs lang="md">
# Skeleton

Loading placeholder for anything slower than 300 ms: gate it with
`useDelayedLoading(loading)`. Variants: text, card, table-row. It is a
`role="status"` with a hidden "加载中…"; the shimmer runs three times and
none under reduced motion.
</docs>
