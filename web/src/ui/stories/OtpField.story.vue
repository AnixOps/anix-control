<script setup>
import { ref } from 'vue'
import UiOtpField from '../UiOtpField.vue'

const code = ref('')
const partial = ref('482')
const last = ref('')
</script>

<template>
  <Story title="OtpField" group="forms" :layout="{ type: 'single', iframe: true }">
    <Variant title="States">
      <div class="story" style="max-width: 360px">
        <UiOtpField v-model="code" label="6 位验证码" help="验证器每 30 秒更新一次。" @complete="value => (last = value)" />
        <p class="story-note">完成：{{ last || '—' }}</p>
        <UiOtpField v-model="partial" label="6 位验证码" error="验证码不正确。检查手机时间后重试。" />
        <UiOtpField model-value="123456" label="6 位验证码" disabled />
      </div>
    </Variant>
  </Story>
</template>

<docs lang="md">
# OtpField

One box per digit inside a fieldset (the legend names the code). Typing
moves forward, Backspace clears and moves back, arrows / Home / End move,
and a paste or an autofill into the first box spreads the whole code.
Non-digits are ignored. `complete` fires when every box is filled; the
login page submits on it.
</docs>
