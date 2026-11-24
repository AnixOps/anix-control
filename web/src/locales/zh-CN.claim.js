// 领取管理员页面（/claim）：单独的消息组，随页面加载，登录页首次访问不带它
// （web/bundle-budget.json）。为中文读者撰写，不是从英文翻译。
export default {
  claim: {
    pageTitle: '创建首个管理员',
    signInHint: '尚未创建管理员。请在服务器上运行 {command}，并打开输出的链接。',
    title: '创建首个管理员',
    subtitle: '设置管理员登录邮箱和密码。',
    loading: '正在检查链接…',
    email: '邮箱',
    password: '密码',
    passwordHelp: '至少 {min} 个字符。',
    confirm: '确认密码',
    submit: '创建管理员',
    done: {
      title: '管理员已创建',
      subtitle: '正在登录…'
    },
    created: '管理员已创建，请使用新密码登录。',
    missing: {
      title: '请打开领取链接',
      body: '需使用服务器输出的领取链接。在服务器上运行以下命令获取：'
    },
    none: {
      title: '没有进行中的领取',
      body: '无需设置。管理员请直接登录。'
    },
    expired: {
      title: '链接已过期',
      body: '领取链接 24 小时内有效。在服务器上运行以下命令获取新链接：'
    },
    invalid: {
      title: '链接无效',
      body: '链接可能已被替换。在服务器上运行以下命令获取新链接：'
    },
    toSignIn: '去登录',
    errors: {
      passwordShort: '密码至少 {min} 个字符。',
      passwordLong: '密码最长 72 字节。',
      emailTaken: '该邮箱已被使用。',
      identity: '登录服务不可用，未做任何更改。请稍后重试。',
      failed: '无法创建管理员：{message}'
    }
  }
}
