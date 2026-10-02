<script setup>
import UiCopyField from '../UiCopyField.vue'
import UiCard from '../UiCard.vue'
</script>

<template>
  <Story title="CopyField" group="display" :layout="{ type: 'single', iframe: true }">
    <Variant title="Link and secret">
      <div class="story story-grouped">
        <UiCard>
          <div class="story">
            <UiCopyField label="订阅链接" value="https://panel.example.com/s/7f3k9q2m8x4v" copy-label="复制订阅链接" variant="primary" help="不要分享给他人；泄露后可在下方重置。" />
            <UiCopyField label="Agent 注册令牌" value="anx_reg_9c1f0e7b4d2a6c8e5f3b1a0d" secret help="24 小时内有效，只能使用一次。" />
            <UiCopyField label="安装命令" value="curl -fsSL https://panel.example.com/install.sh | sh -s -- --token anx_reg_9c1f" size="md" />
          </div>
        </UiCard>
      </div>
    </Variant>
  </Story>
</template>

<docs lang="md">
# CopyField

Read-only value and a copy button. "复制" becomes "已复制" for 2 s and is
announced. `secret` masks the value (a read-only password field) with a
reveal toggle; copying does not need revealing. Over plain HTTP the copy
falls back to `execCommand`; if that fails too, the text is selected and the
field says how to copy it.
</docs>
