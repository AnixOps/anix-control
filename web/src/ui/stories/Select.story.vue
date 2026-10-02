<script setup>
import { ref } from 'vue'
import UiSelect from '../UiSelect.vue'

const template = ref('standard')
const empty = ref(undefined)
const options = [
  { value: 'basic', label: '基础', description: '100 GB / 月' },
  { value: 'standard', label: '标准', description: '200 GB / 月' },
  { value: 'premium', label: '高级', description: '500 GB / 月' },
  { value: 'legacy', label: '旧版（已停用）', disabled: true }
]
</script>

<template>
  <Story title="Select" group="forms" :layout="{ type: 'single', iframe: true }">
    <Variant title="States">
      <div class="story" style="min-height: 420px">
        <UiSelect v-model="template" label="订阅模板" :options="options" help="新用户默认使用此模板。" />
        <UiSelect v-model="empty" label="分组" :options="['香港', '东京', '新加坡']" placeholder="选择分组" required error="请选择分组。" />
        <UiSelect model-value="standard" label="不可用" :options="options" disabled />
        <UiSelect v-model="template" aria-label="订阅模板" size="md" :options="options" style="max-width: 220px" />
      </div>
    </Variant>
  </Story>
</template>

<docs lang="md">
# Select

Reka Select: a button with `role="combobox"` that opens a listbox.
Space/Enter/↓ open it, ↑/↓ move, typing jumps to a match, Enter selects,
Esc closes and returns focus. Use it for short lists (up to ~10 options);
use **Combobox** for long or searchable ones. Without a visible label pass
`aria-label`. Options: `{ value, label, description?, disabled? }` or
strings.
</docs>
