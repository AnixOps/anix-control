<script setup>
import UiBadge from '../UiBadge.vue'
</script>

<template>
  <Story title="Badge" group="display" :layout="{ type: 'single', iframe: true }">
    <Variant title="Status map and tones">
      <div class="story">
        <div class="story-row">
          <UiBadge v-for="status in ['online', 'offline', 'disabled', 'pending', 'error']" :key="status" :status="status" />
        </div>
        <div class="story-row">
          <UiBadge tone="success" label="有效" />
          <UiBadge tone="warning" label="即将到期" />
          <UiBadge tone="danger" label="流量用尽" />
          <UiBadge tone="info" label="待验证" />
          <UiBadge tone="neutral" label="草稿" />
          <UiBadge tone="info" label="v4.1.0" :dot="false" />
        </div>
      </div>
    </Variant>
  </Story>
</template>

<docs lang="md">
# Badge

A pill with a dot and a word (caption type). `status` uses the shared map —
online (success), offline (danger), disabled (neutral), pending (warning),
error (danger) — with i18n words; `tone` + `label` for anything else. The
word is always shown: colour is never the only signal.
</docs>
