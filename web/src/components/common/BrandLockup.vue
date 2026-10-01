<template>
  <span class="brand-lockup" :class="[`brand-lockup-${size}`, { 'brand-lockup-tile': tile, 'brand-lockup-inverse': inverse }]">
    <!-- The glyph markup is the vendored mark-glyph.svg (currentColor), inlined so it follows the theme. -->
    <span class="brand-lockup-mark" aria-hidden="true" v-html="BRAND_GLYPH_SVG"></span>
    <span v-if="!markOnly" class="brand-lockup-name"><span class="brand-lockup-family">AnixOps</span> <span class="brand-lockup-product">{{ product }}</span></span>
  </span>
</template>

<script>
import markGlyph from '@/design/brand/mark-glyph.svg?raw'

// Inline-ready glyph: decorative (the name is written next to it), sized by CSS.
export const BRAND_GLYPH_SVG = markGlyph
  .trim()
  .replace(/<title>[^<]*<\/title>/, '')
  .replace(' width="24" height="24" role="img"', ' focusable="false"')
</script>

<script setup>
// Brand lockup per AnixOps Design brand.md §2.4: the mark followed by
// "AnixOps" (600) and the product word (400) in label-1 / label-2, the gap a
// third of the mark height and the cap height about half of it.
// tile draws the glyph in white on the brand-gradient tile (brand moments
// such as the login page); the default is the bare glyph for UI chrome.
// inverse sets the name in white for the slate brand backdrop.
defineProps({
  product: { type: String, default: 'Control' },
  size: { type: String, default: 'md', validator: value => ['sm', 'md', 'lg'].includes(value) },
  tile: { type: Boolean, default: false },
  inverse: { type: Boolean, default: false },
  markOnly: { type: Boolean, default: false }
})
</script>

<style scoped>
.brand-lockup {
  --lockup-mark: 26px;
  --lockup-text: var(--type-title-3-size);

  display: inline-flex;
  align-items: center;
  gap: calc(var(--lockup-mark) / 3);
  min-width: 0;
  color: var(--label-1);
  line-height: 1;
  white-space: nowrap;
}

.brand-lockup-sm {
  --lockup-mark: 20px;
  --lockup-text: var(--type-body-size);
}

.brand-lockup-lg {
  --lockup-mark: 44px;
  --lockup-text: var(--type-title-1-size);
}

.brand-lockup-mark {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  width: var(--lockup-mark);
  height: var(--lockup-mark);
}

.brand-lockup-mark :deep(svg) {
  display: block;
  width: 100%;
  height: 100%;
}

/* Tile (brand.md §2.1): brand gradient, corner radius 22 % of the side, the
   glyph scaled to 36/64 of the tile and centred (14/64 inset). */
.brand-lockup-tile .brand-lockup-mark {
  border-radius: calc(var(--lockup-mark) * 0.22);
  background: var(--brand-gradient);
  color: var(--on-accent);
  padding: calc(var(--lockup-mark) * 14 / 64);
}

.brand-lockup-name {
  overflow: hidden;
  text-overflow: ellipsis;
  font-family: var(--font-sans);
  font-size: var(--lockup-text);
  letter-spacing: -0.01em;
}

.brand-lockup-family {
  color: var(--label-1);
  font-weight: var(--weight-semibold);
}

.brand-lockup-product {
  color: var(--label-2);
  font-weight: var(--weight-regular);
}

.brand-lockup-inverse,
.brand-lockup-inverse .brand-lockup-family {
  color: var(--on-accent);
}

.brand-lockup-inverse .brand-lockup-product {
  color: color-mix(in srgb, var(--on-accent) 78%, transparent);
}
</style>
