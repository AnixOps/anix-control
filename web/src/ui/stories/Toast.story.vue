<script setup>
import { onMounted } from 'vue'
import UiButton from '../UiButton.vue'
import UiHost from '../UiHost.vue'
import { useToast } from '../composables/useToast'

const toast = useToast()

function showAll() {
  toast.success('已复制订阅链接')
  toast.success('已封禁 2 位用户', { undo: () => toast.info('已撤销封禁') })
  toast.error('无法保存设置：网络连接中断。')
}

// Show one of each on load so the preview (and its screenshot) has content.
onMounted(showAll)
</script>

<template>
  <Story title="Toast" group="overlays" :layout="{ type: 'single', iframe: true }">
    <Variant title="useToast()">
      <div class="story">
        <div class="story-row">
          <UiButton @click="toast.success('已复制订阅链接')">成功（3 秒）</UiButton>
          <UiButton @click="toast.success('已删除转发规则 relay-2', { undo: () => toast.info('已恢复转发规则 relay-2') })">可撤销（5 秒）</UiButton>
          <UiButton @click="toast.error('无法保存设置：网络连接中断。')">错误（需手动关闭）</UiButton>
          <UiButton @click="toast.info('备份已开始，完成后会通知你。')">信息</UiButton>
          <UiButton @click="showAll">全部</UiButton>
        </div>
        <p class="story-note">鼠标悬停或焦点在通知内时计时暂停；F8 跳到通知，Esc 返回。</p>
      </div>
      <UiHost />
    </Variant>
  </Story>
</template>

<docs lang="md">
# Toast and useToast()

The replacement for `alert()`. Success disappears after 3 s; an undoable
action shows "撤销" for 5 s; errors stay until dismissed (inline form errors
come first; system-wide problems use a banner). At most three toasts; timers
pause on hover, on focus and while the page is hidden. One polite live region
announces each toast; F8 moves focus to the toasts and Esc returns it.

```js
const toast = useToast()
toast.success('已复制订阅链接')
toast.success('已删除转发规则 relay-2', { undo: () => restore() })
toast.error('无法保存设置：网络连接中断。')
```
</docs>
