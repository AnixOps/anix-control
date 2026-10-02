<script setup>
import UiQrCode from '../UiQrCode.vue'
import UiCard from '../UiCard.vue'
</script>

<template>
  <Story title="QrCode" group="display" :layout="{ type: 'single', iframe: true }">
    <Variant title="Subscription link">
      <div class="story story-grouped">
        <UiCard>
          <UiQrCode value="https://panel.example.com/s/7f3k9q2m8x4v" label="订阅链接二维码" caption="用客户端扫码导入" />
        </UiCard>
      </div>
    </Variant>
  </Story>
</template>

<docs lang="md">
# QrCode

A scannable code drawn as one SVG path. The encoder (`uqr`) loads on first
use. The code is dark on a light tile in both themes, because many scanners
cannot read an inverted code. `label` is the accessible name: say what the
code holds, not the secret.
</docs>
