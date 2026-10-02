<script setup>
import { useFormat } from '../composables/useFormat'

const f = useFormat()
const now = Date.now()
const rows = [
  ['bytes(0)', f.bytes(0)],
  ['bytes(1536)', f.bytes(1536)],
  ['bytes(137_975_824_384)', f.bytes(137975824384)],
  ['bytes(137_975_824_384, { precision: 1 })', f.bytes(137975824384, { precision: 1 })],
  ['rate(12_500_000)  // bytes/s', f.rate(12500000)],
  ['rate(500, { input: "mbps" })', f.rate(500, { input: 'mbps' })],
  ['duration(273_600)', f.duration(273600)],
  ['duration(273_600, { style: "long" })', f.duration(273600, { style: 'long' })],
  ['money(128_800)  // cents', f.money(128800)],
  ['number(1_208)', f.number(1208)],
  ['percent(0.64)', f.percent(0.64)],
  ['date(1_798_761_600)', f.date(1798761600)],
  ['dateTime(1_798_761_600)', f.dateTime(1798761600)],
  ['relativeTime(now - 180 s)', f.relativeTime(now - 180000, { now })],
  ['relativeTime(now - 2 d)', f.relativeTime(now - 2 * 86400000, { now })],
  ['date(null)', f.date(null)]
]
</script>

<template>
  <Story title="useFormat()" group="display" :layout="{ type: 'single', iframe: true }">
    <Variant title="Examples">
      <div class="story">
        <table class="story-table">
          <tbody>
            <tr v-for="[call, out] in rows" :key="call">
              <th scope="row"><code>{{ call }}</code></th>
              <td class="tabular-nums">{{ out }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </Variant>
  </Story>
</template>

<style scoped>
.story-table {
  border-collapse: collapse;
  font-size: var(--type-callout-size);
}

.story-table th,
.story-table td {
  padding: var(--space-2) var(--space-4) var(--space-2) 0;
  border-bottom: 1px solid var(--separator);
  text-align: left;
}

.story-table th {
  color: var(--label-2);
  font-weight: var(--weight-regular);
}
</style>

<docs lang="md">
# useFormat()

One formatter on Intl and the current locale: bytes (binary, 2 decimals,
matching the page copies of `formatBytes`), rates (decimal bits), durations,
money (cents, ¥), numbers, percents, dates (2026-11-30), date-times
(24-hour) and relative times (「3 分钟前」, show `dateTime` on hover).
Missing values render as an em dash.
</docs>
