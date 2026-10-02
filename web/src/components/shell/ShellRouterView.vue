<template>
  <router-view v-slot="{ Component, route }">
    <Transition name="shell-page">
      <div v-if="Component" :key="pageKey(route)" class="shell-page">
        <component :is="Component" />
      </div>
    </Transition>
  </router-view>
</template>

<script setup>
// The shells' router view with the page transition (plan §5.6): the new
// page fades in over 240 ms while rising 8 px; the old one leaves at once,
// so the two never overlap. Under prefers-reduced-motion only a short fade
// remains (base.css). The wrapper gives every page a single element root,
// which the transition needs. Pages that share a route record (one
// component, different params) are not re-animated.
function pageKey(route) {
  const record = route.matched[route.matched.length - 1]
  return record ? record.path : route.path
}
</script>

<style scoped>
.shell-page-enter-active {
  transition:
    opacity var(--dur-page) var(--ease-standard),
    transform var(--dur-page) var(--ease-emphasized);
}

.shell-page-enter-from {
  opacity: 0;
  transform: translateY(8px);
}

@media (prefers-reduced-motion: reduce) {
  .shell-page-enter-from {
    transform: none;
  }
}
</style>
