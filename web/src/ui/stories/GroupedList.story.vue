<script setup>
import { ref } from 'vue'
import { Bell, Mail, Shield } from '@lucide/vue'
import UiGroupedList from '../UiGroupedList.vue'
import UiGroupedListRow from '../UiGroupedListRow.vue'
import UiSwitch from '../UiSwitch.vue'
import UiSelect from '../UiSelect.vue'
import UiIcon from '../UiIcon.vue'

const register = ref(true)
const lang = ref('zh-CN')
const clicks = ref(0)
</script>

<template>
  <Story title="GroupedList" group="layout" :layout="{ type: 'single', iframe: true }">
    <Variant title="Settings">
      <div class="story story-grouped" style="min-height: 520px">
        <UiGroupedList title="通用" footer="关闭注册后，只有管理员能创建用户。">
          <UiGroupedListRow label="允许新用户注册" label-for="gl-register" description="开放登录页的注册入口。">
            <UiSwitch id="gl-register" v-model="register" />
          </UiGroupedListRow>
          <UiGroupedListRow label="默认语言" label-for="gl-lang">
            <UiSelect id="gl-lang" v-model="lang" size="md" :options="[{ value: 'zh-CN', label: '简体中文' }, { value: 'en', label: 'English' }]" />
          </UiGroupedListRow>
          <UiGroupedListRow label="版本" value="4.1.0-rc.4" />
        </UiGroupedList>
        <UiGroupedList title="更多">
          <UiGroupedListRow label="邮件" description="SMTP 未配置" @click="clicks++">
            <template #leading><UiIcon :icon="Mail" :size="20" /></template>
          </UiGroupedListRow>
          <UiGroupedListRow label="通知" @click="clicks++">
            <template #leading><UiIcon :icon="Bell" :size="20" /></template>
          </UiGroupedListRow>
          <UiGroupedListRow label="安全" href="#security">
            <template #leading><UiIcon :icon="Shield" :size="20" /></template>
          </UiGroupedListRow>
        </UiGroupedList>
        <p class="story-note">点击了 {{ clicks }} 次可导航行。</p>
      </div>
    </Variant>
  </Story>
</template>

<docs lang="md">
# GroupedList

Settings-style rows (System Settings / iCloud): a group title, rows on a
rounded surface with inset hairlines, an optional footnote. A row shows a
label and a sub line on the left, and a value, a control or a chevron on the
right. `label-for` ties the row label to the control. With `@click` or
`href` the whole row is a button or link with a chevron; do not put controls
in such a row.
</docs>
