<script setup>
import { ref } from 'vue'
import UiDialog from '../UiDialog.vue'
import UiButton from '../UiButton.vue'
import UiTextField from '../UiTextField.vue'
import UiNumberField from '../UiNumberField.vue'
import UiSelect from '../UiSelect.vue'

const open = ref(false)
const openOnLoad = ref(true)
const small = ref(false)
const large = ref(false)
const name = ref('hk-hkg-03')
const port = ref(443)
const group = ref('hk')
const saving = ref(false)

function save(close) {
  saving.value = true
  setTimeout(() => {
    saving.value = false
    close()
  }, 1200)
}
</script>

<template>
  <Story title="Dialog" group="overlays" :layout="{ type: 'single', iframe: true }">
    <Variant title="Form (md)">
      <div class="story">
        <div class="story-row">
          <UiButton variant="primary" @click="open = true">添加节点…</UiButton>
          <UiButton @click="small = true">小号 sm</UiButton>
          <UiButton @click="large = true">大号 lg</UiButton>
        </div>
        <p class="story-note">Esc、点击遮罩或 × 关闭；焦点进入对话框并在关闭后回到按钮。</p>
      </div>
      <UiDialog v-model:open="open" title="添加节点" description="节点创建后会出现在所有使用该分组的订阅中。">
        <UiTextField v-model="name" label="名称" required />
        <UiNumberField v-model="port" label="端口" :min="1" :max="65535" />
        <UiSelect v-model="group" label="分组" :options="[{ value: 'hk', label: '香港' }, { value: 'jp', label: '日本' }]" />
        <template #footer="{ close }">
          <UiButton :disabled="saving" @click="close">取消</UiButton>
          <UiButton variant="primary" :loading="saving" @click="save(close)">添加节点</UiButton>
        </template>
      </UiDialog>
      <UiDialog v-model:open="small" size="sm" title="重置订阅链接？" description="旧链接会立即失效，用户需要重新导入。">
        <template #footer="{ close }">
          <UiButton @click="close">取消</UiButton>
          <UiButton variant="primary" @click="close">重置链接</UiButton>
        </template>
      </UiDialog>
      <UiDialog v-model:open="large" size="lg" title="订阅输出预览">
        <p class="story-note">大号对话框用于预览与较宽的表单。</p>
        <template #footer="{ close }">
          <UiButton variant="primary" @click="close">完成</UiButton>
        </template>
      </UiDialog>
    </Variant>

    <Variant title="Open on load">
      <div class="story">
        <div class="story-row">
          <UiButton @click="openOnLoad = true">打开</UiButton>
        </div>
      </div>
      <UiDialog v-model:open="openOnLoad" title="添加节点" description="节点创建后会出现在所有使用该分组的订阅中。">
        <UiTextField v-model="name" label="名称" required />
        <UiNumberField v-model="port" label="端口" :min="1" :max="65535" />
        <template #footer="{ close }">
          <UiButton @click="close">取消</UiButton>
          <UiButton variant="primary" @click="close">添加节点</UiButton>
        </template>
      </UiDialog>
    </Variant>
  </Story>
</template>

<docs lang="md">
# Dialog

Modal dialog on Reka Dialog: `role="dialog"`, `aria-modal`,
`aria-labelledby` (title) and `aria-describedby` (description). Focus moves
in, is trapped and returns to the opener; Esc and the scrim close it unless
`dismissible` is false; the page does not scroll behind it. Sizes sm 420,
md 560, lg 760; below 834 px md and lg fill the screen. Footer buttons are
right-aligned with the primary action last. The close button (×) is last in
tab order.
</docs>
