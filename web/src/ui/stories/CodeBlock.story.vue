<script setup>
import UiCodeBlock from '../UiCodeBlock.vue'

const clash = `proxies:
  - name: hk-01
    type: vless
    server: hk-01.example.com
    port: 443
    uuid: 00000000-0000-0000-0000-000000000000
    tls: true
    servername: hk-01.example.com
proxy-groups:
  - name: Proxy
    type: select
    proxies: [hk-01]`
</script>

<template>
  <Story title="CodeBlock" group="display" :layout="{ type: 'single', iframe: true }">
    <Variant title="Subscription output">
      <div class="story">
        <UiCodeBlock label="Clash" :code="clash" />
      </div>
    </Variant>
    <Variant title="Command, wrapped">
      <div class="story">
        <UiCodeBlock label="Bash" wrap code="curl -fsSL -H 'Authorization: Bearer <FORWARD_API_TOKEN>' 'http://127.0.0.1:18081/api/v2/internal/forward/runtime/status'" />
      </div>
    </Variant>
  </Story>
</template>

<docs lang="md">
# CodeBlock

Generated text in a monospace block: subscription previews, commands,
diagnostic output. It scrolls inside itself (`maxHeight`, default 360 px;
`wrap` wraps long lines instead of scrolling sideways), so the page never
scrolls sideways. The label names the block; 复制 becomes 已复制 for 2 s and
is announced. The block is focusable so it can be scrolled from the keyboard.
</docs>
