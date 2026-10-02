<script setup>
import { ref } from 'vue'
import UiRadioGroup from '../UiRadioGroup.vue'

const mode = ref('agent')
const period = ref('month')
const options = [
  { value: 'agent', label: 'Agent', description: '在机器上运行 AnixOps Agent，由控制面下发配置。' },
  { value: 'ansible', label: 'Ansible', description: '通过 SSH 执行 playbook。' },
  { value: 'local', label: '本地运行时', description: '在控制面所在机器上运行。', disabled: true }
]
</script>

<template>
  <Story title="RadioGroup" group="forms" :layout="{ type: 'single', iframe: true }">
    <Variant title="Vertical">
      <div class="story">
        <UiRadioGroup v-model="mode" label="运行方式" :options="options" help="创建后不能更改。" />
      </div>
    </Variant>
    <Variant title="Horizontal">
      <div class="story">
        <UiRadioGroup v-model="period" label="计费周期" orientation="horizontal" :options="[{ value: 'month', label: '按月' }, { value: 'quarter', label: '按季' }, { value: 'year', label: '按年' }]" />
      </div>
    </Variant>
  </Story>
</template>

<docs lang="md">
# RadioGroup

One choice among a few options that should all be visible. Reka RadioGroup:
`role="radiogroup"` named by the label; Tab enters at the checked option,
arrow keys move and select.
</docs>
