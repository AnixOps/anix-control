<template>
  <figure class="ui-qr" :style="{ '--ui-qr-size': `${size}px` }">
    <svg
      v-if="path"
      class="ui-qr__code"
      :viewBox="`0 0 ${modules} ${modules}`"
      role="img"
      :aria-label="label"
      shape-rendering="crispEdges"
      data-qr-ready
    >
      <rect class="ui-qr__light" :width="modules" :height="modules" />
      <path class="ui-qr__dark" :d="path" />
    </svg>
    <div v-else-if="failed" class="ui-qr__placeholder" role="img" :aria-label="t('ui.qr.failed')">
      <UiIcon :icon="QrCode" :size="32" />
    </div>
    <div v-else class="ui-qr__placeholder is-loading" role="status">
      <span class="visually-hidden">{{ t('ui.loading') }}</span>
    </div>
    <figcaption v-if="caption || $slots.caption" class="ui-qr__caption">
      <slot name="caption">{{ caption }}</slot>
    </figcaption>
  </figure>
</template>

<script setup>
// A scannable QR code of `value` (a subscription link, an otpauth:// key).
// The encoder (uqr, MIT, about 4 KB gzip) loads on first use, so pages
// without a code do not carry it. Modules are drawn as one SVG path with a
// four-module quiet zone. The code stays dark on a light tile in both themes
// (--brand-slate-900 on --on-accent): many scanners cannot read an inverted
// code. `label` is the accessible name; say what the code holds, never the
// secret itself.
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { QrCode } from '@lucide/vue'
import UiIcon from './UiIcon.vue'

const props = defineProps({
  value: { type: String, default: '' },
  label: { type: String, required: true },
  size: { type: Number, default: 168 },
  caption: { type: String, default: '' },
  // Error correction: M suits links shown on a screen.
  ecc: { type: String, default: 'M', validator: value => ['L', 'M', 'Q', 'H'].includes(value) }
})

const QUIET_ZONE = 4
const { t } = useI18n()
const path = ref('')
const modules = ref(0)
const failed = ref(false)
let generation = 0

async function render(value) {
  const current = ++generation
  failed.value = false
  if (!value) {
    path.value = ''
    return
  }
  try {
    const { encode } = await import('uqr')
    if (current !== generation) return
    const result = encode(value, { ecc: props.ecc, border: QUIET_ZONE })
    let d = ''
    result.data.forEach((row, y) => {
      row.forEach((dark, x) => {
        if (dark) d += `M${x} ${y}h1v1h-1z`
      })
    })
    modules.value = result.size
    path.value = d
  } catch {
    if (current !== generation) return
    path.value = ''
    failed.value = true
  }
}

watch(() => [props.value, props.ecc], ([value]) => { render(value) }, { immediate: true })
</script>

<style scoped>
.ui-qr {
  display: inline-flex;
  flex-direction: column;
  gap: var(--space-2);
  align-items: center;
  width: var(--ui-qr-size);
  margin: 0;
}

.ui-qr__code,
.ui-qr__placeholder {
  display: block;
  width: var(--ui-qr-size);
  height: var(--ui-qr-size);
  border-radius: var(--radius-md);
  box-shadow: 0 0 0 1px var(--separator);
}

.ui-qr__light {
  fill: var(--on-accent);
}

.ui-qr__dark {
  fill: var(--brand-slate-900);
}

.ui-qr__placeholder {
  display: grid;
  place-items: center;
  background: var(--fill-1);
  color: var(--label-3);
}

.ui-qr__caption {
  color: var(--label-2);
  font-size: var(--type-caption-size);
  line-height: var(--type-caption-line);
  text-align: center;
}
</style>
