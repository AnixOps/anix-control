<script setup>
import { ref } from 'vue'
import UiCombobox from '../UiCombobox.vue'

const node = ref('hk-hkg-02')
const nodes = [
  'hk-hkg-01', 'hk-hkg-02', 'jp-tyo-01', 'jp-tyo-02', 'jp-osa-01', 'sg-sin-01', 'sg-sin-02',
  'us-lax-01', 'us-sjc-01', 'de-fra-01', 'gb-lon-01', 'kr-sel-01'
].map(value => ({ value, label: value, description: value.startsWith('hk') ? '香港' : value.startsWith('jp') ? '日本' : '其他地区' }))
</script>

<template>
  <Story title="Combobox" group="forms" :layout="{ type: 'single', iframe: true }">
    <Variant title="Searchable">
      <div class="story" style="min-height: 460px">
        <UiCombobox v-model="node" label="入口节点" :options="nodes" help="输入名称筛选。" />
        <UiCombobox :model-value="undefined" label="出口节点" :options="nodes" error="请选择出口节点。" />
      </div>
    </Variant>
  </Story>
</template>

<docs lang="md">
# Combobox

Searchable single choice for long lists (nodes, users, templates). Reka
Combobox: the input has `role="combobox"` and filters the listbox as you
type; ↓/↑ move, Enter selects, Esc closes. An unmatched search is reset on
blur, so the field always shows the chosen option.
</docs>
