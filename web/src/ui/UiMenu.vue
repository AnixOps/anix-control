<template>
  <DropdownMenuRoot v-model:open="open" :modal="false">
    <DropdownMenuTrigger as-child>
      <slot name="trigger">
        <UiIconButton :icon="icon" :label="label" :size="size" v-bind="$attrs" />
      </slot>
    </DropdownMenuTrigger>
    <DropdownMenuPortal>
      <DropdownMenuContent class="ui-menu" :align="align" :side-offset="4" :collision-padding="12">
        <slot :close="close">
          <template v-for="(item, index) in visibleItems" :key="item.key || index">
            <DropdownMenuSeparator v-if="item.separatorBefore && index > 0" class="ui-menu__separator" />
            <DropdownMenuItem
              class="ui-menu__item"
              :class="{ 'is-danger': item.danger }"
              :disabled="item.disabled"
              :data-menu-item="item.key || undefined"
              @select="item.onSelect?.()"
            >
              <UiIcon v-if="item.icon" :icon="item.icon" :size="16" />
              <span class="ui-menu__label">{{ item.label }}</span>
            </DropdownMenuItem>
          </template>
        </slot>
      </DropdownMenuContent>
    </DropdownMenuPortal>
  </DropdownMenuRoot>
</template>

<script setup>
// "…" menu for a row or a toolbar (Reka DropdownMenu: role="menu", arrows,
// typeahead, Esc returns focus to the trigger). Pass `items`
// ({ key, label, icon, danger, disabled, separatorBefore, hidden, onSelect })
// or fill the default slot with Reka menu items using the `ui-menu__*`
// classes. Danger items are red and come last, after a separator.
import { computed, ref } from 'vue'
import { DropdownMenuContent, DropdownMenuItem, DropdownMenuPortal, DropdownMenuRoot, DropdownMenuSeparator, DropdownMenuTrigger } from 'reka-ui'
import { MoreHorizontal } from '@lucide/vue'
import UiIcon from './UiIcon.vue'
import UiIconButton from './UiIconButton.vue'

defineOptions({ inheritAttrs: false })

const props = defineProps({
  // Accessible name of the default "…" trigger.
  label: { type: String, default: '' },
  icon: { type: [Object, Function], default: () => MoreHorizontal },
  items: { type: Array, default: () => [] },
  align: { type: String, default: 'end' },
  size: { type: String, default: 'sm' }
})

const open = ref(false)
const visibleItems = computed(() => props.items.filter(item => item && !item.hidden))

function close() {
  open.value = false
}
</script>

<style src="./internal/menu.css"></style>
