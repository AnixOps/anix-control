<script setup>
import UiCard from '../UiCard.vue'
import UiButton from '../UiButton.vue'
import UiSection from '../UiSection.vue'
</script>

<template>
  <Story title="Card and Section" group="layout" :layout="{ type: 'single', iframe: true }">
    <Variant title="Card">
      <div class="story story-grouped">
        <UiCard>
          <p>纯卡片：24 px 内边距，radius-md，shadow-1 与发丝边。</p>
        </UiCard>
        <UiCard title="公告" description="来自帮助中心的置顶文章。">
          <template #actions>
            <UiButton variant="tertiary" size="sm">全部公告</UiButton>
          </template>
          <p>10 月 8 日凌晨 2:00–3:00 维护香港节点。</p>
        </UiCard>
      </div>
    </Variant>
    <Variant title="Section">
      <div class="story">
        <UiSection title="最近活动" description="来自审计日志的最近 20 条操作。">
          <template #actions>
            <UiButton size="sm">查看审计日志</UiButton>
          </template>
          <UiCard><p>admin 封禁了 2 位用户 · 3 分钟前</p></UiCard>
        </UiSection>
      </div>
    </Variant>
  </Story>
</template>

<docs lang="md">
# Card and Section

**Card**: an elevated surface (radius-md, shadow-1, hairline edge). With a
`title` it gets a header row with actions. **Section**: a titled block of a
page (Title 2) that holds cards, tables or grouped lists; 48 px apart.
</docs>
