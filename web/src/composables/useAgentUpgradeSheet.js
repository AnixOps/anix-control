import { ref } from 'vue'

// The 升级节点 sheet (components/admin/AgentUpgradeSheet.vue) is mounted once
// by the admin shell. Any page opens it with openAgentUpgrade(): the node
// list's button, the ⌘K action 升级节点.
//
//   import { openAgentUpgrade } from '@/composables/useAgentUpgradeSheet'
//   <UiButton @click="openAgentUpgrade">升级节点</UiButton>
const open = ref(false)
// True from the first opening on: the shell loads the sheet's code then,
// not with every admin page.
const requested = ref(false)

export function openAgentUpgrade() {
  requested.value = true
  open.value = true
}

export function closeAgentUpgrade() {
  open.value = false
}

export function useAgentUpgradeSheet() {
  return { open, requested, openAgentUpgrade, closeAgentUpgrade }
}
