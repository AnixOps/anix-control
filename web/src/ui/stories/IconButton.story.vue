<script setup>
import { ref } from 'vue'
import { Ellipsis, RefreshCw, Trash2, X } from '@lucide/vue'
import UiIconButton from '../UiIconButton.vue'

const pressed = ref(false)
</script>

<template>
  <Story title="IconButton" group="actions" :layout="{ type: 'single', iframe: true }">
    <Variant title="Sizes and states">
      <div class="story">
        <div class="story-row">
          <UiIconButton label="更多操作" :icon="Ellipsis" size="sm" />
          <UiIconButton label="更多操作" :icon="Ellipsis" />
          <UiIconButton label="更多操作" :icon="Ellipsis" size="lg" />
          <UiIconButton label="关闭" :icon="X" variant="secondary" />
          <UiIconButton label="自动刷新" :icon="RefreshCw" :pressed="pressed" @click="pressed = !pressed" />
          <UiIconButton label="删除" :icon="Trash2" disabled />
        </div>
      </div>
    </Variant>
  </Story>
</template>

<docs lang="md">
# IconButton

Square, round-cornered icon-only button. `label` is required: it is the
`aria-label` and the native tooltip. A toggle passes `pressed`
(`aria-pressed`) and keeps the same label. Sizes 28 / 36 / 44 px, with a
44 px hit area on touch screens.
</docs>
