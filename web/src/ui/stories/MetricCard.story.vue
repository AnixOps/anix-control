<script setup>
import { Activity, LifeBuoy, Server, Users } from '@lucide/vue'
import UiMetricCard from '../UiMetricCard.vue'

const spark = [4, 6, 5, 9, 7, 12, 10, 14, 11, 16, 13, 18]
</script>

<template>
  <Story title="MetricCard" group="display" :layout="{ type: 'single', iframe: true }">
    <Variant title="Dashboard row">
      <div class="story story-grouped">
        <div class="story-grid">
          <UiMetricCard label="用户" :icon="Users" value="1,204" trend="今日 +12" trend-direction="up" trend-tone="positive" detail="活跃 980 · 已到期 31 · 已封禁 4" />
          <UiMetricCard label="在线节点" :icon="Server" value="18 / 20" detail="在线用户 342" />
          <UiMetricCard label="今日流量" :icon="Activity" value="128.4 GB" detail="累计 42.1 TB" :sparkline="spark" />
          <UiMetricCard label="待处理工单" :icon="LifeBuoy" value="3" trend="比昨天多 2" trend-direction="up" trend-tone="negative" detail="已回复 5" />
        </div>
      </div>
    </Variant>
    <Variant title="Loading and empty">
      <div class="story story-grouped">
        <div class="story-grid">
          <UiMetricCard label="用户" loading />
          <UiMetricCard label="待处理工单" value="" detail="工单列表未加载" />
        </div>
      </div>
    </Variant>
  </Story>
</template>

<docs lang="md">
# MetricCard

A label, one big number, an optional trend and detail, and an optional
sparkline. The trend carries a word and an arrow, never colour alone;
`trendTone` says whether the change is good news. The sparkline is a small
inline SVG in the accent colour, decorative (`aria-hidden`). Wrap the card in
a link when it opens a page. `loading` shows a card skeleton after 300 ms.
</docs>
