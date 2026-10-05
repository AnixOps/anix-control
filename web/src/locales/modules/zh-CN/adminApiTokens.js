// 安全 → API 令牌（security/ApiTokens.vue、ApiTokenCreate.vue）：管理员用于自动化的个人访问令牌。随 adminPages 加载。
export default {
  adminApiTokens: {
    title: 'API 令牌',
    description: '给脚本、CI 任务和监控探针用的个人访问令牌。令牌以你的身份调用管理员 API，权限不超过它的范围，直到过期或被你吊销。',
    guide: '关于 API 令牌（指南）',
    refresh: '刷新',
    quota: '已使用 {used} / {max} 个有效令牌。',
    quotaReached: '你已持有上限 {max} 个有效令牌。请先吊销一个再创建新的。',
    loadFailed: '无法加载 API 令牌',
    copied: '已复制令牌 ID。',
    copyFailed: '无法复制令牌 ID。',
    filters: {
      label: '同时显示',
      ended: '已吊销和已过期',
      all: '所有管理员',
      superOnly: '只有超级管理员可以查看其他管理员的令牌。'
    },
    table: {
      label: 'API 令牌',
      name: '名称',
      owner: '所有者',
      scope: '范围',
      status: '状态',
      expires: '到期时间',
      lastUsed: '最近使用',
      createdOn: '创建于 {date}',
      hintTitle: '令牌的最后四个字符。令牌本身不会再次显示。'
    },
    owner: {
      you: '你',
      admin: '管理员 #{id}'
    },
    scopes: {
      read: '只读',
      admin: '管理'
    },
    states: {
      active: '有效',
      expiring: '即将到期',
      expired: '已过期',
      revoked: '已吊销'
    },
    expires: {
      never: '永不过期',
      expired: '{when}已过期',
      revoked: '{when}已吊销'
    },
    lastUsed: {
      never: '从未使用'
    },
    revokeReasons: {
      owner_revoked: '已由所有者吊销',
      admin_revoked: '已由超级管理员吊销',
      owner_not_admin: '已吊销：所有者被封禁、降级或删除',
      unknown: '已吊销'
    },
    actions: {
      copyId: '复制令牌 ID',
      revoke: '吊销令牌…'
    },
    empty: {
      title: '还没有 API 令牌',
      description: '创建令牌后，脚本、CI 任务或监控探针无需你的密码就能调用管理员 API。令牌只在创建时显示一次。'
    },
    revoke: {
      title: '吊销“{name}”？',
      message: '使用这个令牌的程序会立即失效：它的下一个请求会被拒绝。此操作无法撤销。请为这个自动化另建一个令牌。',
      messageOther: '它属于管理员 #{id}。使用这个令牌的程序会立即失效：它的下一个请求会被拒绝。此操作无法撤销。',
      action: '吊销令牌',
      done: '已吊销“{name}”。',
      already: '“{name}”早已被吊销。',
      gone: '这个令牌已不存在，或不属于你。列表已刷新。',
      failed: '无法吊销令牌：{message}'
    },
    create: {
      action: '创建令牌',
      title: '创建 API 令牌',
      description: '脚本、CI 任务或监控探针用它代替你的密码。令牌只在创建后显示一次。',
      name: '名称',
      namePlaceholder: '每晚导出',
      nameHelp: '用途，例如“每晚导出”。最多 {max} 个字符。',
      scope: '范围',
      scopeHelp: '优先选只读，设较短的有效期，每个自动化一个令牌。',
      scopeText: {
        read: '只读：只能对管理员 API 发 GET 和 HEAD 请求。可以查看，不能修改任何内容；也不能读取节点的 API 密钥和 Telegram 机器人令牌，这是仍以明文返回密钥的两个读取接口。',
        admin: '你在管理员 API 上能做的一切，包括修改（你是超级管理员时也包括超级管理员操作）。它仍然不能管理 API 令牌：那需要你已登录的会话。'
      },
      expiry: '有效期',
      expiryHelp: '请设置有效期，并把续期记在日历里。90 天是个不错的起点。',
      expiryOptions: {
        d30: '30 天',
        d90: '90 天（推荐）',
        d180: '180 天',
        d365: '1 年（365 天）',
        d730: '2 年（730 天，最长）',
        custom: '自定义…',
        never: '永不过期（不推荐）'
      },
      customDays: '多少天后到期',
      customDaysHelp: '1 到 {max} 之间的整数。',
      daysUnit: '天',
      neverNotice: '没有有效期的令牌在被吊销之前一直有效，泄露了也一样。除非有明确的理由，请设置有效期。',
      confirmTitle: '确认是你本人',
      checking: '正在检查验证方式…',
      passwordNote: '创建令牌需要你当前的密码。',
      password: '当前密码',
      codeNote: '已开启两步验证。请输入身份验证器里的 6 位验证码。',
      code: '验证码',
      useRecovery: '改用恢复码',
      recoveryNote: '输入一个恢复码。每个恢复码只能使用一次。',
      recovery: '恢复码',
      useCode: '改用验证码',
      cancel: '取消',
      submit: '创建令牌',
      signInAgain: '重新登录',
      errors: {
        name: {
          required: '请输入名称。',
          tooLong: '最多 100 个字符。',
          unprintable: '只能使用可打印字符。'
        },
        expiry: '请输入 1 到 730 之间的整数天数。',
        password: '请输入当前密码。',
        code: '请输入 6 位验证码。',
        recoveryFormat: '恢复码的格式是 XXXX-XXXX。',
        passwordWrong: '密码不正确。',
        codeWrong: '验证码不正确。请等下一个再试。',
        recoveryWrong: '恢复码无效，或已被使用。',
        rateLimited: '失败次数过多。请稍后再试。',
        rateLimitedWait: '失败次数过多。请 {minutes} 分钟后再试。',
        signInAgain: '你的登录时间太久，无法完成这一步。请重新登录，并在 10 分钟内创建令牌。',
        too_many_tokens: '你已持有 25 个有效的 API 令牌。请先吊销一个。',
        not_an_administrator: '只有有效的管理员可以创建 API 令牌。',
        invalid_request: '没有创建令牌：{message}',
        failed: '无法创建令牌：{message}'
      }
    },
    result: {
      title: '你的新 API 令牌',
      description: '只显示一次。关闭此对话框后无法再次查看。',
      warning: '现在就复制，放进使用它的程序的密钥存储里。Control 只保存哈希，无法再次显示。弄丢了就吊销它，再创建一个。',
      label: 'API 令牌',
      copy: '复制令牌',
      name: '名称',
      scope: '范围',
      expires: '有效期',
      never: '永不过期',
      usageTitle: '使用令牌',
      usageIntro: '放在 Authorization 请求头里发送，只用于管理员 API（/api/v2/admin、/api/v3 和 /api/v4）。',
      usageLabel: '请求示例',
      usageNever: '不要把令牌放进 URL。查询字符串里带令牌的请求会被拒绝并记入审计，而且 URL 已经进了访问日志：请吊销那个令牌。',
      done: '完成'
    }
  }
}
