<template>
  <p v-if="claimable && visible" class="claim-hint" data-auth-claim-hint>
    <UiIcon :icon="Info" :size="16" />
    <i18n-t keypath="claim.signInHint" tag="span" scope="global">
      <template #command><code class="claim-hint__code">sudo anix-control admin claim-link</code></template>
    </i18n-t>
  </p>
</template>

<script setup>
// The sign-in page's "no administrator yet" note. Login.vue loads it after
// the form (its own chunk), so the sign-in page's first visit carries none
// of it (web/bundle-budget.json). It asks GET /api/v5/claim/status once and,
// only while the panel has no administrator, loads the claim messages and
// says how to claim the panel; any other answer shows nothing.
import { onMounted, ref } from 'vue'
import { Info } from '@lucide/vue'
import { getClaimStatus } from '@/api/claim'
import { loadMessageGroup } from '@/i18n'
import UiIcon from '@/ui/UiIcon.vue'

defineProps({
  // The sign-in form is showing (not registration or a second step).
  visible: { type: Boolean, default: true }
})

const claimable = ref(false)

onMounted(async () => {
  try {
    if (!(await getClaimStatus())) return
    await loadMessageGroup('claim')
    claimable.value = true
  } catch {
    // Nothing to say when the status cannot be read.
  }
})
</script>

<style scoped>
.claim-hint {
  display: flex;
  gap: var(--space-2);
  align-items: flex-start;
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-sm);
  background: var(--bg-grouped);
  color: var(--label-1);
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
}

.claim-hint :deep(.ui-icon) {
  flex: none;
  margin-top: 2px;
  color: var(--accent);
}

.claim-hint__code {
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
  overflow-wrap: anywhere;
}
</style>
