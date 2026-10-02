// Built-in strings of the component library (web/src/ui/). Written for
// Chinese readers, not translated from the English file; see the voice and
// tone rules in docs/reference/frontend-design.md.
export default {
  ui: {
    actions: {
      close: '关闭',
      cancel: '取消',
      confirm: '确认',
      undo: '撤销',
      copy: '复制',
      copied: '已复制',
      dismiss: '关闭通知'
    },
    loading: '加载中…',
    field: {
      required: '必填'
    },
    password: {
      show: '显示密码'
    },
    secret: {
      show: '显示内容',
      masked: '已隐藏'
    },
    number: {
      increment: '增加',
      decrement: '减少'
    },
    select: {
      placeholder: '请选择',
      search: '搜索',
      empty: '没有匹配的选项'
    },
    otp: {
      digit: '第 {n} 位，共 {total} 位'
    },
    qr: {
      failed: '无法生成二维码'
    },
    copy: {
      failed: '无法自动复制，已选中内容，请按 Ctrl+C 或 ⌘C 复制。'
    },
    toast: {
      region: '通知',
      hotkeyHint: '按 F8 前往通知。'
    },
    confirm: {
      title: '确认操作',
      typeToConfirm: '输入 {name} 以确认',
      failed: '操作没有完成：{reason}'
    },
    status: {
      online: '在线',
      offline: '离线',
      disabled: '已停用',
      pending: '待处理',
      error: '异常'
    },
    format: {
      duration: {
        day: '{n} 天',
        hour: '{n} 小时',
        minute: '{n} 分钟',
        second: '{n} 秒'
      },
      durationShort: {
        day: '{n} 天',
        hour: '{n} 小时',
        minute: '{n} 分',
        second: '{n} 秒'
      }
    }
  }
}
