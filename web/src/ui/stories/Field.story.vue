<script setup>
import UiField from '../UiField.vue'
</script>

<template>
  <Story title="Field" group="forms" :layout="{ type: 'single', iframe: true }">
    <Variant title="Custom control">
      <div class="story">
        <UiField v-slot="field" label="颜色主题" help="自定义控件也通过 UiField 获得标签、帮助与错误。" error="请选择一个主题。" required>
          <input :id="field.id" type="color" :aria-describedby="field.describedBy" :aria-invalid="field.invalid">
        </UiField>
      </div>
    </Variant>
  </Story>
</template>

<docs lang="md">
# Field

The frame every form control uses: a top label, help, an error (in a polite
live region) and a required marker. The default slot gets `{ id, labelId,
describedBy, invalid }` to put on a custom control.
</docs>
