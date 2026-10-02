<script setup>
import { ref } from 'vue'
import UiTextarea from '../UiTextarea.vue'

const note = ref('')
</script>

<template>
  <Story title="Textarea" group="forms" :layout="{ type: 'single', iframe: true }">
    <Variant title="States">
      <div class="story">
        <UiTextarea v-model="note" label="备注" placeholder="写给其他管理员的说明" help="仅管理员可见。" />
        <UiTextarea model-value="ssh-rsa AAAA" label="公钥" error="公钥格式不正确：应以 ssh-ed25519 开头。" />
        <UiTextarea model-value="" label="不可用" disabled />
      </div>
    </Variant>
  </Story>
</template>

<docs lang="md">
# Textarea

Multi-line text with the same label, help and error wiring as TextField.
Resizes vertically.
</docs>
