<template>
  <span class="line-path" :class="{ 'is-stacked': stacked }">
    <template v-for="(hop, index) in line.hops" :key="index">
      <span v-if="index" class="line-path__link" :class="{ 'is-kernel': line.hops[index - 1].devices[0]?.kernel }" aria-hidden="true" />
      <span class="line-path__hop" :class="[`is-${line.hops.length === 1 ? 'exit' : hop.role}`, { 'is-served': hop.devices.some(device => device.ref === line.served) }]">
        <span class="line-path__dot" aria-hidden="true" />
        {{ hop.devices[0]?.name }}<template v-if="hop.devices.length > 1"> +{{ hop.devices.length - 1 }}</template>
      </span>
    </template>
    <template v-if="line.targets.length && line.kind === 'fixed'">
      <span class="line-path__link" :class="{ 'is-kernel': line.hops.at(-1)?.devices[0]?.kernel }" aria-hidden="true" />
      <span class="line-path__hop is-target">{{ line.targets[0] }}<template v-if="line.targets.length > 1"> +{{ line.targets.length - 1 }}</template></span>
    </template>
  </span>
</template>

<script setup>
// A line's way as chips, entry first: a chip per hop (its first device and
// how many more), outlined where the protocol is served, a dashed link
// after a device that forwards in the kernel, and the first target of a
// line that forwards to fixed targets.
defineProps({
  line: { type: Object, required: true },
  stacked: { type: Boolean, default: false }
})
</script>

<style scoped>
.line-path {
  display: inline-flex;
  flex-wrap: wrap;
  gap: var(--space-1) 0;
  align-items: center;
  min-width: 0;
}

.line-path__hop {
  --pin: var(--accent);

  display: inline-flex;
  gap: var(--space-1);
  align-items: center;
  max-width: 100%;
  padding: var(--space-0-5) var(--space-2);
  overflow: hidden;
  border-radius: var(--radius-pill);
  background: var(--fill-1);
  color: var(--label-1);
  font-size: var(--type-callout-size);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.line-path__hop.is-entry {
  --pin: var(--chart-2);
}

.line-path__hop.is-relay {
  --pin: var(--chart-3);
}

.line-path__hop.is-served {
  box-shadow: inset 0 0 0 1.5px var(--accent);
}

.line-path__hop.is-target {
  color: var(--label-2);
  font-family: var(--font-mono);
  font-size: var(--type-caption-size);
}

.line-path__dot {
  flex: none;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--pin);
}

.line-path__link {
  flex: none;
  width: var(--space-6);
  border-top: 1.5px solid var(--accent);
  opacity: 0.6;
}

.line-path__link.is-kernel {
  border-top-style: dashed;
  border-top-color: var(--label-3);
  opacity: 1;
}
</style>
