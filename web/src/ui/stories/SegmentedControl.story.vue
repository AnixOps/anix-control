<script setup>
import { ref } from 'vue'
import { LayoutGrid, List } from '@lucide/vue'
import UiSegmentedControl from '../UiSegmentedControl.vue'

const range = ref('24h')
const view = ref('list')
</script>

<template>
  <Story title="SegmentedControl" group="forms" :layout="{ type: 'single', iframe: true }">
    <Variant title="States">
      <div class="story">
        <UiSegmentedControl
          v-model="range"
          aria-label="时间范围"
          :options="[{ value: '1h', label: '1 小时' }, { value: '24h', label: '24 小时' }, { value: '7d', label: '7 天' }, { value: '30d', label: '30 天' }]"
        />
        <UiSegmentedControl
          v-model="view"
          aria-label="视图"
          size="sm"
          :options="[{ value: 'list', label: '直列', icon: List }, { value: 'group', label: '分组', icon: LayoutGrid }]"
        />
        <UiSegmentedControl v-model="range" aria-label="时间范围" block :options="['1h', '24h', '7d']" />
        <p class="story-note">选中值：{{ range }} · {{ view }}</p>
      </div>
    </Variant>
  </Story>
</template>

<docs lang="md">
# SegmentedControl

Pick one of 2–5 values that change the current view in place (time range,
list or grouped view). Reka ToggleGroup: a named `group` of buttons with
`aria-pressed`; Tab enters at the selected segment, ←/→ move, Space/Enter
select; there is always one selection. To switch between **content
panels** (转发规则 / 隧道 / 限速) use **Tabs** with `variant="segmented"`,
which has tab and tabpanel semantics.
</docs>
