<template>
  <nav v-if="pageCount > 1 || showSummary" class="ui-pagination" :aria-label="label || t('ui.pagination.label')">
    <p v-if="showSummary && total > 0" class="ui-pagination__summary tabular-nums">
      {{ t('ui.pagination.range', { from: rangeFrom, to: rangeTo, total }) }}
    </p>
    <div v-if="pageCount > 1" class="ui-pagination__pages">
      <UiIconButton
        :icon="ChevronLeft"
        :label="t('ui.pagination.previous')"
        :disabled="current <= 1"
        size="sm"
        data-page-prev
        @click="go(current - 1)"
      />
      <ul class="ui-pagination__list">
        <li v-for="(item, index) in items" :key="`${item}-${index}`">
          <span v-if="item === GAP" class="ui-pagination__gap" aria-hidden="true">…</span>
          <button
            v-else
            type="button"
            class="ui-pagination__page tabular-nums"
            :class="{ 'is-current': item === current }"
            :aria-current="item === current ? 'page' : undefined"
            :aria-label="t('ui.pagination.page', { page: item })"
            @click="go(item)"
          >{{ item }}</button>
        </li>
      </ul>
      <span class="ui-pagination__compact tabular-nums" aria-hidden="true">{{ t('ui.pagination.compact', { page: current, pages: pageCount }) }}</span>
      <UiIconButton
        :icon="ChevronRight"
        :label="t('ui.pagination.next')"
        :disabled="current >= pageCount"
        size="sm"
        data-page-next
        @click="go(current + 1)"
      />
    </div>
  </nav>
</template>

<script setup>
// Pagination for lists (client or server side): "第 21–40 条，共 312 条",
// previous / next and page numbers with gaps (1 … 4 5 6 … 16); on phones
// only previous / "2 / 16" / next. The current page has aria-current.
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { ChevronLeft, ChevronRight } from '@lucide/vue'
import UiIconButton from './UiIconButton.vue'

const GAP = '…'

const props = defineProps({
  page: { type: Number, default: 1 },
  total: { type: Number, default: 0 },
  pageSize: { type: Number, default: 20 },
  showSummary: { type: Boolean, default: true },
  label: { type: String, default: '' }
})

const emit = defineEmits(['update:page'])
const { t } = useI18n()

const pageCount = computed(() => Math.max(1, Math.ceil((props.total || 0) / (props.pageSize || 1))))
const current = computed(() => Math.min(Math.max(1, props.page || 1), pageCount.value))
const rangeFrom = computed(() => (props.total ? (current.value - 1) * props.pageSize + 1 : 0))
const rangeTo = computed(() => Math.min(props.total, current.value * props.pageSize))

const items = computed(() => {
  const count = pageCount.value
  const page = current.value
  if (count <= 7) return Array.from({ length: count }, (_, index) => index + 1)
  const out = [1]
  const start = Math.max(2, Math.min(page - 1, count - 4))
  const end = Math.min(count - 1, Math.max(page + 1, 5))
  if (start > 2) out.push(GAP)
  for (let value = start; value <= end; value += 1) out.push(value)
  if (end < count - 1) out.push(GAP)
  out.push(count)
  return out
})

function go(page) {
  const next = Math.min(Math.max(1, page), pageCount.value)
  if (next !== current.value) emit('update:page', next)
}
</script>

<style scoped>
.ui-pagination {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3);
  align-items: center;
  justify-content: space-between;
  min-height: var(--size-control-md);
}

.ui-pagination__summary {
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.ui-pagination__pages {
  display: flex;
  gap: var(--space-1);
  align-items: center;
  margin-left: auto;
}

.ui-pagination__list {
  display: flex;
  gap: var(--space-1);
  align-items: center;
  padding: 0;
  margin: 0;
  list-style: none;
}

.ui-pagination__page {
  min-width: var(--size-control-sm);
  height: var(--size-control-sm);
  min-height: 0;
  padding: 0 var(--space-2);
  border: 0;
  border-radius: var(--radius-pill);
  background: transparent;
  color: var(--label-1);
  font: inherit;
  font-size: var(--type-callout-size);
  cursor: pointer;
  transition: background-color var(--dur-micro) var(--ease-standard);
}

.ui-pagination__page:hover {
  background: var(--fill-1);
}

.ui-pagination__page:focus-visible {
  outline: var(--focus-ring);
  outline-offset: var(--focus-ring-offset);
}

.ui-pagination__page.is-current {
  background: var(--fill-2);
  font-weight: var(--weight-semibold);
}

.ui-pagination__gap {
  padding: 0 var(--space-1);
  color: var(--label-2);
}

.ui-pagination__compact {
  display: none;
  padding: 0 var(--space-2);
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

@media (max-width: 639.98px) {
  .ui-pagination__list {
    display: none;
  }

  .ui-pagination__compact {
    display: inline;
  }
}

@media (pointer: coarse) {
  .ui-pagination__page {
    min-width: 44px;
    height: 44px;
  }
}
</style>
