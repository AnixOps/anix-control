<template>
  <component
    :is="icon"
    class="ui-icon"
    :size="size"
    :stroke-width="strokeWidth"
    :aria-hidden="label ? undefined : 'true'"
    :aria-label="label || undefined"
    :role="label ? 'img' : undefined"
    focusable="false"
  />
</template>

<script setup>
// A Lucide icon at the design system's stroke (1.5) and sizes: 16 inside
// buttons and cells, 20 in navigation, 48 in empty states. Decorative unless
// `label` is given (then role="img" with that name).
defineProps({
  icon: { type: [Object, Function], required: true },
  size: { type: Number, default: 16 },
  strokeWidth: { type: Number, default: 1.5 },
  label: { type: String, default: '' }
})
</script>

<style scoped>
.ui-icon {
  flex: none;
}
</style>
