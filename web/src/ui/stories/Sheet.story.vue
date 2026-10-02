<script setup>
import { ref } from 'vue'
import UiSheet from '../UiSheet.vue'
import UiButton from '../UiButton.vue'
import UiBadge from '../UiBadge.vue'
import UiGroupedList from '../UiGroupedList.vue'
import UiGroupedListRow from '../UiGroupedListRow.vue'
import UiSwitch from '../UiSwitch.vue'

const open = ref(true)
const banned = ref(false)
</script>

<template>
  <Story title="Sheet" group="overlays" :layout="{ type: 'single', iframe: true }">
    <Variant title="User details">
      <div class="story">
        <div class="story-row">
          <UiButton @click="open = true">查看用户</UiButton>
        </div>
        <p class="story-note">宽于 834 px 时从右侧滑出；更窄时为底部弹出。</p>
      </div>
      <UiSheet v-model:open="open" title="lin.xiao@example.com" description="注册于 2026-03-14" grouped>
        <template #header-actions>
          <UiBadge status="online" />
        </template>
        <UiGroupedList title="订阅">
          <UiGroupedListRow label="订阅模板" value="标准 · 200 GB/月" />
          <UiGroupedListRow label="已用流量" value="71.6 GB / 200 GB" />
          <UiGroupedListRow label="到期" value="2026-11-30" />
        </UiGroupedList>
        <UiGroupedList title="账户" footer="封禁后用户无法登录，订阅立即停用。">
          <UiGroupedListRow label="封禁用户" label-for="sheet-ban">
            <UiSwitch id="sheet-ban" v-model="banned" />
          </UiGroupedListRow>
        </UiGroupedList>
        <template #footer="{ close }">
          <UiButton @click="close">关闭</UiButton>
          <UiButton variant="primary" @click="close">保存</UiButton>
        </template>
      </UiSheet>
    </Variant>
  </Story>
</template>

<docs lang="md">
# Sheet

Side panel for quick views and edits that keep the list in context (user
details, node quick view, rule editor). Same behaviour as Dialog (focus
trap and return, Esc, scrim, no page scroll). Right drawer (sm 360, md 440,
lg 600); below 834 px a bottom sheet up to 92 % of the screen height.
`grouped` puts the body on `--bg-grouped` for grouped lists.
</docs>
