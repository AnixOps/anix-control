<script setup>
import { computed, ref } from 'vue'
import UiNumberField from '../UiNumberField.vue'

const port = ref(30001)
const limit = ref(0)
const traffic = ref(200)
const limitError = computed(() => (limit.value !== null && limit.value <= 0 ? '限速必须大于 0；不限速请关闭此项。' : ''))
</script>

<template>
  <Story title="NumberField" group="forms" :layout="{ type: 'single', iframe: true }">
    <Variant title="States">
      <div class="story">
        <UiNumberField v-model="port" label="入口端口" :min="1" :max="65535" help="1–65535，不能与已有规则重复。" />
        <UiNumberField v-model="limit" label="限速" unit="Mbps" :min="0" :error="limitError" />
        <UiNumberField v-model="traffic" label="每月流量" unit="GB" :min="0" :step="10" stepper help="↑/↓ 每次 10 GB。" />
        <UiNumberField :model-value="443" label="不可用" disabled />
      </div>
    </Variant>
  </Story>
</template>

<docs lang="md">
# NumberField

Reka NumberField: `role="spinbutton"` with `aria-valuenow/min/max`; ↑/↓ step,
PageUp/PageDown step ×10, Home/End jump to min/max; the mouse wheel does not
change the value. The unit is a suffix read as part of the description. No
digit grouping by default (ports read 30001). `stepper` adds − / + buttons
for pointer users. The model is a number, or `null` when empty.
</docs>
