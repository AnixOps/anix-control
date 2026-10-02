<script setup>
import { ref } from 'vue'
import UiConfirmDialog from '../UiConfirmDialog.vue'
import UiButton from '../UiButton.vue'
import UiHost from '../UiHost.vue'
import { useConfirm } from '../composables/useConfirm'
import { useToast } from '../composables/useToast'

const typed = ref(true)
const simple = ref(false)
const deleting = ref(false)
const confirm = useConfirm()
const toast = useToast()

function remove() {
  deleting.value = true
  setTimeout(() => {
    deleting.value = false
    typed.value = false
  }, 1200)
}

async function viaComposable() {
  const ok = await confirm({
    title: '删除用户 lin.xiao@example.com？',
    message: '用户的订阅、工单与流量记录会一并删除。此操作无法撤销。',
    confirmLabel: '删除用户',
    tone: 'danger',
    requireText: 'lin.xiao@example.com',
    onConfirm: () => new Promise(resolve => setTimeout(resolve, 1000))
  })
  if (ok) toast.success('已删除用户 lin.xiao@example.com')
}

async function failing() {
  await confirm({
    title: '停用节点 hk-hkg-01？',
    message: '节点会从所有订阅中移除，直到重新启用。',
    confirmLabel: '停用节点',
    onConfirm: () => new Promise((resolve, reject) => setTimeout(() => reject(new Error('无法连接节点 hk-hkg-01：连接超时。')), 800))
  })
}
</script>

<template>
  <Story title="ConfirmDialog" group="overlays" :layout="{ type: 'single', iframe: true }">
    <Variant title="Typed name (destructive)">
      <div class="story">
        <div class="story-row">
          <UiButton variant="danger-soft" @click="typed = true">删除节点…</UiButton>
          <UiButton @click="simple = true">重置订阅链接…</UiButton>
        </div>
      </div>
      <UiConfirmDialog
        v-model:open="typed"
        title="删除节点 hk-hkg-01？"
        message="节点的 4 个协议与凭据会一并删除，1,208 位用户的订阅中将不再出现此节点。此操作无法撤销。"
        confirm-label="删除节点"
        tone="danger"
        require-text="hk-hkg-01"
        :loading="deleting"
        @confirm="remove"
      />
      <UiConfirmDialog
        v-model:open="simple"
        title="重置订阅链接？"
        message="旧链接会立即失效，用户需要重新导入。"
        confirm-label="重置链接"
        @confirm="simple = false"
      />
    </Variant>

    <Variant title="useConfirm()">
      <div class="story">
        <div class="story-row">
          <UiButton variant="danger-soft" @click="viaComposable">删除用户…</UiButton>
          <UiButton @click="failing">停用节点（失败示例）</UiButton>
        </div>
        <p class="story-note">await confirm({...}) 返回 true / false；onConfirm 运行期间按钮显示加载，失败时错误显示在对话框内。</p>
      </div>
      <UiHost />
    </Variant>
  </Story>
</template>

<docs lang="md">
# ConfirmDialog and useConfirm()

The replacement for `confirm()`, as `role="alertdialog"`. Ask only for
irreversible or wide-reaching actions; use an undo toast for the rest. Name
the object in the title (「删除节点 hk-01？」), say what happens, and make the
confirm button a verb (「删除节点」). Deleting a node or a user requires typing
its name (`requireText`). Initial focus: the typed field, else Cancel for
danger, else the confirm button. Esc cancels; the scrim does not.

```js
const confirm = useConfirm()
if (await confirm({ title, message, confirmLabel, tone: 'danger', requireText, onConfirm })) { … }
```
</docs>
