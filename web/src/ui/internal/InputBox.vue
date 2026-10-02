<template>
  <component
    :is="as"
    class="ui-box"
    :class="[`ui-box--${size}`, { 'is-invalid': invalid, 'is-disabled': disabled, 'is-readonly': readonly, 'is-multiline': multiline }]"
  >
    <slot />
  </component>
</template>

<script setup>
// The visual box shared by text fields, the select trigger and the combobox:
// 1 px separator-strong border, radius-sm, elevated background, and a focus
// ring (2 px accent + soft halo) on :focus-within. Not exported.
defineProps({
  as: { type: [String, Object], default: 'div' },
  size: { type: String, default: 'lg' },
  invalid: { type: Boolean, default: false },
  disabled: { type: Boolean, default: false },
  readonly: { type: Boolean, default: false },
  multiline: { type: Boolean, default: false }
})
</script>

<style scoped>
.ui-box {
  --ui-box-height: var(--size-control-lg);
  --ui-box-pad: var(--space-3);

  position: relative;
  display: flex;
  align-items: center;
  width: 100%;
  min-width: 0;
  min-height: var(--ui-box-height);
  padding: 0;
  border: 1px solid var(--separator-strong);
  border-radius: var(--radius-sm);
  background: var(--bg-elevated);
  color: var(--label-1);
  font-size: var(--type-body-size);
  text-align: left;
  transition:
    border-color var(--dur-micro) var(--ease-standard),
    box-shadow var(--dur-micro) var(--ease-standard);
}

.ui-box--md {
  --ui-box-height: var(--size-control-md);
}

.ui-box--sm {
  --ui-box-height: var(--size-control-sm);
  --ui-box-pad: var(--space-2);

  font-size: var(--type-callout-size);
}

.ui-box:hover:not(.is-disabled, .is-readonly) {
  border-color: var(--label-3);
}

/* The ring replaces the browser outline on the control inside. A
   transparent outline keeps a visible ring in forced-colors mode. */
.ui-box:focus-within {
  border-color: var(--accent);
  outline: 2px solid transparent;
  box-shadow: 0 0 0 1px var(--accent), 0 0 0 4px var(--accent-soft);
}

.ui-box.is-invalid {
  border-color: var(--danger);
}

.ui-box.is-invalid:focus-within {
  box-shadow: 0 0 0 1px var(--danger), 0 0 0 4px var(--danger-soft);
}

.ui-box.is-disabled {
  background: var(--fill-1);
  color: var(--label-2);
  cursor: not-allowed;
}

.ui-box.is-readonly {
  background: var(--bg-grouped);
}

.ui-box.is-multiline {
  align-items: stretch;
}

/* Slotted control: fills the box, no own border or outline. */
.ui-box :slotted(input),
.ui-box :slotted(textarea) {
  flex: 1;
  width: 100%;
  min-width: 0;
  height: calc(var(--ui-box-height) - 2px);
  padding: 0 var(--ui-box-pad);
  border: 0;
  border-radius: inherit;
  background: transparent;
  color: inherit;
  font: inherit;
  outline: none;
  appearance: none;
}

.ui-box :slotted(textarea) {
  height: auto;
  min-height: 88px;
  padding: var(--space-2) var(--ui-box-pad);
  line-height: var(--type-body-line);
  resize: vertical;
}

.ui-box :slotted(input)::placeholder,
.ui-box :slotted(textarea)::placeholder {
  color: var(--label-3);
  opacity: 1;
}

.ui-box :slotted(input:disabled),
.ui-box :slotted(textarea:disabled) {
  cursor: not-allowed;
}

/* Number inputs: no browser spinners (arrow keys still step). */
.ui-box :slotted(input[type='number'])::-webkit-inner-spin-button,
.ui-box :slotted(input[type='number'])::-webkit-outer-spin-button {
  margin: 0;
  appearance: none;
}

/* Units and adornments */
.ui-box :slotted(.ui-box__affix) {
  flex: none;
  padding: 0 var(--ui-box-pad);
  color: var(--label-2);
  font-size: var(--type-callout-size);
  white-space: nowrap;
  pointer-events: none;
}

.ui-box :slotted(.ui-box__affix--start) {
  padding-right: 0;
}

.ui-box :slotted(.ui-box__affix--end) {
  padding-left: 0;
}

.ui-box :slotted(.ui-box__end) {
  display: flex;
  flex: none;
  gap: var(--space-0-5);
  align-items: center;
  padding-right: var(--space-1);
}

/* Phones: 16 px text so iOS Safari does not zoom on focus. */
@media (max-width: 833px) {
  .ui-box {
    font-size: 16px;
  }
}
</style>
