import { computed, reactive } from 'vue'

// Feature flags the forwarding pages follow. enableAnixOps is Control's
// forward.anixops_experimental (forward-sdk.md F6 A1–A5, off by default):
// with it the editor offers the anixops engine and link, labelled
// 「anixops（实验）」 (D12). Control does not serve the flag yet, so it stays
// off; routes that already use anixops show it whatever the flag.
const state = reactive({ enableAnixOps: false })

export function setForwardFlags(flags = {}) {
  if (typeof flags.enableAnixOps === 'boolean') state.enableAnixOps = flags.enableAnixOps
}

export function useForwardFlags() {
  return { enableAnixOps: computed(() => state.enableAnixOps) }
}
