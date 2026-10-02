<template>
  <UiToastRegion />
  <UiConfirmHost />
</template>

<script setup>
// Mount once at the app root (App.vue, from U4 on): the toast region and the
// confirmation host that useToast() and useConfirm() talk to.
import UiToastRegion from './UiToastRegion.vue'
import UiConfirmHost from './UiConfirmHost.vue'
</script>
