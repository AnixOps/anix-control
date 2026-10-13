// 领取管理员页面（/claim）：单独的消息组，随页面加载，登录页首次访问不带它
// （web/bundle-budget.json）。为中文读者撰写，不是从英文翻译。
export default {
  claim: {
    title: '创建首个管理员',
    subtitle: '这个面板还没有管理员。设置以后用来登录的邮箱和密码。',
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
    created: '管理员已创建，请用新密码登录。',
    missing: {
      title: '请打开领取链接',
      body: '这个页面需要服务器打印的领取链接。在服务器上运行下面的命令获取新链接：'
    },
    none: {
      title: '没有进行中的领取',
      body: '这里没有需要设置的内容。如果你管理这个面板，请直接登录。'
    },
    expired: {
      title: '链接已过期',
      body: '领取链接 24 小时内有效。在服务器上运行下面的命令，再打开它打印的新链接：'
    },
    invalid: {
      title: '链接无效',
      body: '它可能已被更新的链接替换。在服务器上运行下面的命令，再打开它打印的新链接：'
    },
    toSignIn: '去登录',
    errors: {
      passwordShort: '密码至少需要 {min} 个字符。',
      passwordLong: '密码太长，最多 72 个字节。',
      emailTaken: '这个邮箱已有账户，请换一个邮箱。',
      identity: '登录服务暂时无法访问。没有做任何更改，请稍后重试。',
      failed: '无法创建管理员：{message}'
    }
  }
}
