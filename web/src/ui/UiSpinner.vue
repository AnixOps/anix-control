<template>
  <span class="ui-spinner" :style="{ '--ui-spinner-size': `${size}px` }" aria-hidden="true" />
</template>

<script setup>
// Small inline spinner for a busy button. Large areas use UiSkeleton instead
// (guidelines/motion.md). Decorative: the busy state is announced by the
// owner (aria-busy, or the button's own text).
defineProps({
  size: { type: Number, default: 16 }
})
</script>

<style scoped>
.ui-spinner {
  display: inline-block;
  width: var(--ui-spinner-size);
  height: var(--ui-spinner-size);
  border: 2px solid currentColor;
  border-right-color: transparent;
  border-radius: 50%;
  opacity: 0.85;
  animation: ui-spin 0.8s linear infinite;
}

@keyframes ui-spin {
  to {
    transform: rotate(360deg);
  }
}

/* No rotation under reduced motion (base.css also caps animations); a
   static, dimmed ring still marks the busy state. */
@media (prefers-reduced-motion: reduce) {
  .ui-spinner {
    animation: none;
    opacity: 0.5;
  }
}
</style>
