// 安全 → API 令牌（security/ApiTokens.vue、ApiTokenCreate.vue）：管理员用于自动化的个人访问令牌。随 adminPages 加载。
export default {
  adminApiTokens: {
    title: 'API 令牌',
    description: '供脚本、CI 任务与监控探针使用的个人访问令牌。令牌以创建者身份在其范围内调用管理员 API，至过期或吊销为止。',
    guide: '关于 API 令牌（指南）',
    refresh: '刷新',
    quota: '已使用 {used} / {max} 个有效令牌。',
    quotaReached: '有效令牌已达上限 {max} 个，请先吊销后再创建。',
    loadFailed: '无法加载 API 令牌',
    copied: '已复制令牌 ID。',
    copyFailed: '无法复制令牌 ID。',
    filters: {
      label: '同时显示',
      ended: '已吊销和已过期',
      all: '所有管理员',
      superOnly: '仅超级管理员可查看其他管理员的令牌。'
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
      hintTitle: '令牌末四位。完整令牌不再显示。'
    },
    owner: {
      you: '本人',
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
      title: '暂无 API 令牌',
      description: '脚本、CI 任务或监控探针可使用令牌调用管理员 API。令牌仅在创建时显示一次。'
    },
    revoke: {
      title: '吊销“{name}”？',
      message: '使用该令牌的程序将立即失效。此操作无法撤销。',
      messageOther: '该令牌属于管理员 #{id}，使用该令牌的程序将立即失效。此操作无法撤销。',
      action: '吊销令牌',
      done: '已吊销“{name}”。',
      already: '“{name}”早已被吊销。',
      gone: '令牌不存在或不属于当前管理员，列表已刷新。',
      failed: '无法吊销令牌：{message}'
    },
    create: {
      action: '创建令牌',
      title: '创建 API 令牌',
      description: '供脚本、CI 任务或监控探针代替密码使用。令牌仅在创建后显示一次。',
      name: '名称',
      namePlaceholder: '每晚导出',
      nameHelp: '用途，例如“每晚导出”，最多 {max} 个字符。',
      scope: '范围',
      scopeHelp: '建议选择只读、设置较短有效期，并为每项自动化单独创建令牌。',
      scopeText: {
        read: '只读：仅可对管理员 API 发送 GET 与 HEAD 请求，不可修改内容，不可读取 Telegram 机器人令牌。',
        admin: '与创建者在管理员 API 上的权限相同（含修改及超级管理员操作），但不可管理 API 令牌。'
      },
      expiry: '有效期',
      expiryHelp: '请设置有效期，建议 90 天。',
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
      neverNotice: '无有效期的令牌在吊销前始终有效。如无特殊需要，请设置有效期。',
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
        signInAgain: '登录已超时，请重新登录并在 10 分钟内创建令牌。',
        too_many_tokens: '有效 API 令牌已达 25 个上限，请先吊销。',
        not_an_administrator: '仅有效管理员可创建 API 令牌。',
        invalid_request: '令牌创建失败：{message}',
        failed: '无法创建令牌：{message}'
      }
    },
    result: {
      title: '新 API 令牌',
      description: '仅显示一次，关闭后无法再次查看。',
      warning: '请立即复制并存入密钥存储。如遗失，请吊销后重新创建。',
      label: 'API 令牌',
      copy: '复制令牌',
      name: '名称',
      scope: '范围',
      expires: '有效期',
      never: '永不过期',
      usageTitle: '使用令牌',
      usageIntro: '通过 Authorization 请求头发送，仅适用于管理员 API（/api/v2/admin、/api/v3 与 /api/v4）。',
      usageLabel: '请求示例',
      usageNever: '请勿将令牌放入 URL。查询字符串中携带令牌的请求将被拒绝并记入审计，该令牌应予吊销。',
      done: '完成'
    }
  }
}
