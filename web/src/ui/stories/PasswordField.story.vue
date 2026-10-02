<script setup>
import { ref } from 'vue'
import UiPasswordField from '../UiPasswordField.vue'

const password = ref('correct-horse-battery')
const next = ref('')
</script>

<template>
  <Story title="PasswordField" group="forms" :layout="{ type: 'single', iframe: true }">
    <Variant title="States">
      <div class="story">
        <UiPasswordField v-model="password" label="密码" />
        <UiPasswordField v-model="next" label="新密码" autocomplete="new-password" required help="至少 12 个字符。" />
        <UiPasswordField model-value="secret" label="密码" error="密码不正确。" />
      </div>
    </Variant>
  </Story>
</template>

<docs lang="md">
# PasswordField

A text field with a reveal toggle. The toggle keeps one name ("显示密码")
and reports its state with `aria-pressed`, so screen readers hear
"pressed / not pressed". `autocomplete` defaults to `current-password`; use
`new-password` when setting one.
</docs>
