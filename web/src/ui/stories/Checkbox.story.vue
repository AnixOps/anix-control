<script setup>
import { ref } from 'vue'
import UiCheckbox from '../UiCheckbox.vue'

const terms = ref(false)
const all = ref('indeterminate')
const notify = ref(true)
</script>

<template>
  <Story title="Checkbox" group="forms" :layout="{ type: 'single', iframe: true }">
    <Variant title="States">
      <div class="story">
        <UiCheckbox v-model="terms" label="我已阅读并同意服务条款" />
        <UiCheckbox v-model="notify" label="到期前 3 天提醒" description="通过邮件和 Telegram 通知用户。" />
        <UiCheckbox v-model="all" label="全选（部分选中）" />
        <UiCheckbox :model-value="true" label="不可用" disabled />
      </div>
    </Variant>
  </Story>
</template>

<docs lang="md">
# Checkbox

Reka Checkbox: `role="checkbox"` with `aria-checked` true, false or mixed;
Space toggles. `modelValue` can be `'indeterminate'` (table "select all").
The box border is `--label-3` so it keeps 3:1 against the surface.
</docs>
