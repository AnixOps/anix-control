<script setup>
import { reactive, ref } from 'vue'
import { Download, Plus, RefreshCw, Trash2 } from '@lucide/vue'
import UiButton from '../UiButton.vue'

const variants = ['primary', 'secondary', 'tertiary', 'danger', 'danger-soft', 'ghost']
const saving = ref(false)
const playground = reactive({ variant: 'primary', size: 'md', label: '添加节点', loading: false, disabled: false })

function save() {
  saving.value = true
  setTimeout(() => { saving.value = false }, 1500)
}
</script>

<template>
  <Story title="Button" group="actions" :layout="{ type: 'single', iframe: true }">
    <Variant title="Variants">
      <div class="story">
        <div v-for="size in ['sm', 'md', 'lg']" :key="size" class="story-row">
          <UiButton v-for="variant in variants" :key="variant" :variant="variant" :size="size">
            {{ variant }} {{ size }}
          </UiButton>
        </div>
      </div>
    </Variant>

    <Variant title="With icons">
      <div class="story">
        <div class="story-row">
          <UiButton variant="primary" :icon="Plus">添加节点</UiButton>
          <UiButton :icon="Download">导出</UiButton>
          <UiButton variant="tertiary" :icon="RefreshCw">刷新</UiButton>
          <UiButton variant="danger-soft" :icon="Trash2">删除节点…</UiButton>
          <UiButton variant="primary" size="lg" :icon="Plus">添加第一个节点</UiButton>
        </div>
      </div>
    </Variant>

    <Variant title="Loading and disabled">
      <div class="story">
        <p class="story-note">Loading keeps the width, sets aria-busy and ignores clicks; focus stays on the button.</p>
        <div class="story-row">
          <UiButton variant="primary" :loading="saving" @click="save">保存更改</UiButton>
          <UiButton variant="primary" loading>保存更改</UiButton>
          <UiButton variant="danger" loading>删除节点</UiButton>
          <UiButton variant="primary" disabled>不可用</UiButton>
          <UiButton disabled>不可用</UiButton>
        </div>
      </div>
    </Variant>

    <Variant title="Playground">
      <div class="story">
        <div class="story-row">
          <UiButton :variant="playground.variant" :size="playground.size" :loading="playground.loading" :disabled="playground.disabled">
            {{ playground.label }}
          </UiButton>
        </div>
      </div>
      <template #controls>
        <HstSelect v-model="playground.variant" title="variant" :options="variants" />
        <HstSelect v-model="playground.size" title="size" :options="['sm', 'md', 'lg']" />
        <HstText v-model="playground.label" title="label" />
        <HstCheckbox v-model="playground.loading" title="loading" />
        <HstCheckbox v-model="playground.disabled" title="disabled" />
      </template>
    </Variant>
  </Story>
</template>

<docs lang="md">
# Button

Pill buttons. One **primary** per view (the main action, rightmost in a
footer); **secondary** for the rest; **tertiary** for low-emphasis text
actions; **danger** only for the confirming button of a destructive action,
**danger-soft** for the button that starts one ("删除节点…"); **ghost** in
toolbars. Labels are verbs that say what happens.

Sizes: `sm` 28 (tables), `md` 36 (default), `lg` 44 (touch, page actions).
On touch screens every size has a 44 px hit area.
</docs>
