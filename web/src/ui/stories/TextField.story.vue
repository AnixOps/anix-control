<script setup>
import { computed, ref } from 'vue'
import UiTextField from '../UiTextField.vue'

const email = ref('lin.xiao@example.com')
const name = ref('')
const nameTouched = ref(false)
const nameError = computed(() => (nameTouched.value && !name.value.trim() ? '请输入节点名称。' : ''))
</script>

<template>
  <Story title="TextField" group="forms" :layout="{ type: 'single', iframe: true }">
    <Variant title="States">
      <div class="story">
        <UiTextField v-model="email" label="邮箱" type="email" autocomplete="username" help="登录与接收通知用。" />
        <UiTextField
          v-model="name"
          label="节点名称"
          required
          placeholder="例如 hk-hkg-01"
          :error="nameError"
          @blur="nameTouched = true"
        />
        <UiTextField model-value="0" label="限速" suffix="Mbps" error="限速必须大于 0；不限速请关闭此项。" />
        <UiTextField model-value="panel.example.com" label="面板地址" prefix="https://" readonly help="只读。" />
        <UiTextField model-value="" label="不可用" disabled placeholder="禁用状态" />
      </div>
    </Variant>
    <Variant title="Sizes">
      <div class="story">
        <UiTextField model-value="" label="lg 44（默认）" placeholder="表单" />
        <UiTextField model-value="" label="md 36" size="md" placeholder="工具栏搜索" />
        <UiTextField model-value="" label="sm 28" size="sm" placeholder="表格内" />
      </div>
    </Variant>
  </Story>
</template>

<docs lang="md">
# TextField

Top label (decision D5), help under the field, the error under the help with
an icon. Help, error and units are tied to the input with
`aria-describedby`; an error sets `aria-invalid` and is announced through a
polite live region. Validate on blur and on submit; mark required fields.
Default height 44 px (`lg`); `md` 36 for toolbars, `sm` 28 inside tables.
On phones the text is 16 px so iOS does not zoom. `class`/`style` go to the
wrapper, every other attribute (`autocomplete`, `inputmode`, `name`,
`@blur`) to the `<input>`.
</docs>
