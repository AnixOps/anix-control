<template>
  <span class="shell-avatar" :class="`shell-avatar--${size}`" aria-hidden="true">{{ initial }}</span>
</template>

<script setup>
// The account's initial on a neutral disc (decorative: the menu button that
// holds it carries the name). One grapheme, upper-cased for Latin letters.
import { computed } from 'vue'

const props = defineProps({
  name: { type: String, default: '' },
  size: { type: String, default: 'md', validator: value => ['sm', 'md'].includes(value) }
})

const initial = computed(() => {
  const source = String(props.name || '').trim()
  if (!source) return '?'
  const first = Array.from(source)[0]
  return first.toLocaleUpperCase()
})
</script>

<style scoped>
.shell-avatar {
  display: inline-grid;
  flex: none;
  place-items: center;
  width: 32px;
  height: 32px;
  border-radius: 50%;
  background: var(--fill-2);
  color: var(--label-1);
  font-size: var(--type-callout-size);
  font-weight: var(--weight-semibold);
  line-height: 1;
}

.shell-avatar--sm {
  width: 28px;
  height: 28px;
  font-size: var(--type-caption-size);
}
</style>
