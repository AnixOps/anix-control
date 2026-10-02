<template>
  <component
    :is="icon"
    class="admin-nav-icon"
    :data-admin-nav-icon="resolvedName"
    :size="normalizedSize"
    :stroke-width="1.5"
    aria-hidden="true"
    focusable="false"
  />
</template>

<script setup>
// A navigation icon by name (navigation/icons.js). Unknown names, such as a
// plugin menu's icon the closed map does not know, get the neutral box.
import { computed } from 'vue'
import { navIcon, navIconName } from '@/navigation/icons'

const props = defineProps({
  name: {
    type: String,
    required: true
  },
  size: {
    type: Number,
    default: 18
  }
})

const icon = computed(() => navIcon(props.name))
const resolvedName = computed(() => navIconName(props.name))
const normalizedSize = computed(() => (
  Number.isFinite(props.size) && props.size > 0 && props.size <= 48 ? props.size : 18
))
</script>
