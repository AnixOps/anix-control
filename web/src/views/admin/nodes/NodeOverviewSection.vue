<template>
  <div class="node-overview">
    <NodeNotice v-if="overQuota" tone="warning" data-testid="node-over-quota">
      {{ t('admin.nodes.overview.overQuota') }}
    </NodeNotice>
    <NodeNotice v-if="node.runtime_checked_at && !node.runtime_healthy" tone="danger" data-testid="node-runtime-error">
      {{ t('admin.nodes.overview.runtimeError', { message: node.runtime_error || t('admin.nodes.table.runtimeUnhealthy') }) }}
    </NodeNotice>

    <div class="node-overview__grid">
      <UiGroupedList :title="t('admin.nodes.overview.health')" heading-tag="h2">
        <UiGroupedListRow :label="t('admin.nodes.table.status')">
          <template #value>
            <span class="node-overview__badges">
              <UiBadge :status="statusName(status)" :label="t(`admin.nodes.statusText.${statusName(status)}`)" />
              <UiBadge
                v-if="node.runtime_checked_at"
                :tone="node.runtime_healthy ? 'success' : 'danger'"
                :label="node.runtime_healthy ? t('admin.nodes.table.runtimeHealthy') : t('admin.nodes.table.runtimeUnhealthy')"
              />
            </span>
          </template>
        </UiGroupedListRow>
        <UiGroupedListRow :label="t('admin.nodes.table.lastHeartbeat')">
          <template #value>
            <time v-if="node.last_check_at" :datetime="isoTime(node.last_check_at)" :title="format.dateTime(node.last_check_at)">{{ format.relativeTime(node.last_check_at) }}</time>
            <span v-else>{{ t('admin.nodes.overview.never') }}</span>
          </template>
        </UiGroupedListRow>
        <UiGroupedListRow :label="t('admin.nodes.table.agentVersion')" :value="node.server_version || '—'" />
        <UiGroupedListRow :label="t('admin.nodes.overview.os')" :value="node.server_os || '—'" />
        <UiGroupedListRow :label="t('admin.nodes.overview.serverIp')" :value="node.server_ip || '—'" />
        <UiGroupedListRow :label="t('admin.nodes.overview.uptime')" :value="node.uptime ? format.duration(node.uptime) : '—'" />
        <UiGroupedListRow :label="t('admin.nodes.overview.onlineUsers')" :value="hasReport ? format.number(node.online_users || 0) : '—'" />
        <UiGroupedListRow v-for="metric in metrics" :key="metric.key" :label="metric.label">
          <UiUsageBar v-if="hasReport" class="node-overview__bar" :value="metric.value" :max="100" :text="format.percent(metric.value / 100, { precision: 0 })" />
          <span v-else class="node-overview__muted">—</span>
        </UiGroupedListRow>
      </UiGroupedList>

      <UiGroupedList :title="t('admin.nodes.overview.traffic')" heading-tag="h2" :footer="t('admin.nodes.overview.quotaFooter')">
        <UiGroupedListRow :label="t('admin.nodes.overview.totalUpload')" :value="format.bytes(node.total_upload || 0)" />
        <UiGroupedListRow :label="t('admin.nodes.overview.totalDownload')" :value="format.bytes(node.total_download || 0)" />
        <UiGroupedListRow :label="t('admin.nodes.table.monthlyQuota')">
          <UiUsageBar
            v-if="node.monthly_limit"
            class="node-overview__bar"
            :value="monthlyUsed(node)"
            :max="Number(node.monthly_limit)"
            :text="`${format.bytes(monthlyUsed(node))} / ${format.bytes(node.monthly_limit)}`"
          />
          <span v-else class="node-overview__muted">{{ t('admin.nodes.overview.unlimited', { used: format.bytes(monthlyUsed(node)) }) }}</span>
        </UiGroupedListRow>
        <UiGroupedListRow :label="t('admin.nodes.form.fields.monthlyResetDay')" :value="t('admin.nodes.overview.resetDay', { day: node.monthly_reset_day || 1 })" />
        <UiGroupedListRow :label="t('admin.nodes.form.fields.rate')" :value="`× ${node.rate ?? 1}`" />
      </UiGroupedList>

      <UiGroupedList :title="t('admin.nodes.overview.config')" heading-tag="h2">
        <UiGroupedListRow :label="t('admin.nodes.table.address')">
          <template #value><code class="node-overview__code">{{ node.address || '—' }}</code></template>
        </UiGroupedListRow>
        <UiGroupedListRow :label="t('admin.nodes.table.protocols')" :value="String(protocolCount)" />
        <UiGroupedListRow :label="t('admin.nodes.table.parent')">
          <template #value>
            <RouterLink v-if="node.parent_id" class="node-overview__link" :to="`/admin/nodes/${node.parent_id}`">{{ parentName || `#${node.parent_id}` }}</RouterLink>
            <span v-else>{{ t('admin.nodes.overview.rootNode') }}</span>
          </template>
        </UiGroupedListRow>
        <UiGroupedListRow :label="t('admin.nodes.form.fields.tags')">
          <template #value>
            <span v-if="tags.length" class="node-overview__badges">
              <UiBadge v-for="tag in tags" :key="tag" tone="neutral" :dot="false" :label="tag" />
            </span>
            <span v-else>—</span>
          </template>
        </UiGroupedListRow>
        <UiGroupedListRow :label="t('admin.nodes.form.fields.sort')" :value="String(node.sort ?? 0)" />
        <UiGroupedListRow :label="t('admin.nodes.overview.registered')" :value="node.auto_register ? t('admin.nodes.overview.autoRegistered') : t('admin.nodes.overview.manual')" />
        <UiGroupedListRow :label="t('admin.nodes.overview.createdAt')" :value="format.dateTime(node.created_at)" />
      </UiGroupedList>
    </div>
  </div>
</template>

<script setup>
// 概览 section of the node page: health (status, runtime, heartbeat, the
// Agent's own report), traffic and the node's settings. Everything comes
// from GET /admin/nodes/:id; nothing here writes.
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import UiBadge from '@/ui/UiBadge.vue'
import UiGroupedList from '@/ui/UiGroupedList.vue'
import UiGroupedListRow from '@/ui/UiGroupedListRow.vue'
import UiUsageBar from '@/ui/UiUsageBar.vue'
import { useFormat } from '@/ui/composables/useFormat'
import { useAppI18n } from '@/composables/useAppI18n'
import NodeNotice from './NodeNotice.vue'
import { displayStatus, isOverQuota, monthlyUsed, splitTags, statusName } from './nodeData'

const props = defineProps({
  node: { type: Object, required: true },
  protocolCount: { type: Number, default: 0 },
  parentName: { type: String, default: '' }
})
const { t } = useAppI18n()
const format = useFormat()

const status = computed(() => displayStatus(props.node))
const overQuota = computed(() => isOverQuota(props.node))
const tags = computed(() => splitTags(props.node.tags))
// The Agent's own numbers exist once it has reported (a heartbeat).
const hasReport = computed(() => Boolean(props.node.last_check_at))
const metrics = computed(() => [
  { key: 'cpu', label: t('admin.nodes.overview.cpu'), value: Number(props.node.cpu_usage || 0) },
  { key: 'memory', label: t('admin.nodes.overview.memory'), value: Number(props.node.memory_usage || 0) },
  { key: 'disk', label: t('admin.nodes.overview.disk'), value: Number(props.node.disk_usage || 0) }
])

function isoTime(seconds) {
  const value = Number(seconds)
  return Number.isFinite(value) && value > 0 ? new Date(value * 1000).toISOString() : undefined
}
</script>

<style scoped>
.node-overview {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
}

.node-overview__grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--space-8) var(--space-6);
  align-items: start;
}

.node-overview__grid > :first-child {
  grid-row: span 2;
}

.node-overview__badges {
  display: inline-flex;
  flex-wrap: wrap;
  gap: var(--space-1);
  justify-content: flex-end;
}

.node-overview__bar {
  width: 180px;
  max-width: 100%;
}

.node-overview__muted {
  color: var(--label-2);
}

.node-overview__code {
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
  overflow-wrap: anywhere;
}

.node-overview__link {
  color: var(--accent);
  text-decoration: none;
}

.node-overview__link:hover {
  text-decoration: underline;
}

@media (max-width: 1067.98px) {
  .node-overview__grid {
    grid-template-columns: minmax(0, 1fr);
  }

  .node-overview__grid > :first-child {
    grid-row: auto;
  }
}
</style>
