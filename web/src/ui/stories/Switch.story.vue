<script setup>
import { ref } from 'vue'
import UiSwitch from '../UiSwitch.vue'

const register = ref(true)
const invite = ref(false)
</script>

<template>
  <Story title="Switch" group="forms" :layout="{ type: 'single', iframe: true }">
    <Variant title="States">
      <div class="story" style="max-width: 480px">
        <UiSwitch v-model="register" label="允许新用户注册" />
        <UiSwitch v-model="invite" label="需要邀请码" description="开启后，注册时必须填写有效的邀请码。" />
        <UiSwitch :model-value="true" label="不可用" disabled />
        <div class="story-row">
          <UiSwitch v-model="register" aria-label="允许新用户注册（无可见标签）" />
          <span class="story-note">无可见标签时传 aria-label。</span>
        </div>
      </div>
    </Variant>
  </Story>
</template>

<docs lang="md">
# Switch

An on/off setting that applies immediately. Reka Switch: `role="switch"`
with `aria-checked`; Space or Enter toggles. Label on the left, switch on
the right, as in a settings row. Settings that apply on submit use a
Checkbox instead.
</docs>
