<script setup>
import UiStatusDot from '../UiStatusDot.vue'
</script>

<template>
  <Story title="StatusDot" group="display" :layout="{ type: 'single', iframe: true }">
    <Variant title="Status map">
      <div class="story">
        <div class="story-row">
          <UiStatusDot v-for="status in ['online', 'offline', 'disabled', 'pending', 'error']" :key="status" :status="status" />
        </div>
        <div class="story-row">
          <UiStatusDot tone="success" label="运行中" />
          <UiStatusDot tone="warning" label="降级" />
          <UiStatusDot tone="danger" label="异常" />
          <UiStatusDot label="未知" />
        </div>
      </div>
    </Variant>
  </Story>
</template>

<docs lang="md">
# StatusDot

A dot and a word for table cells and list rows where a badge is too heavy.
Same status map as Badge.
</docs>
