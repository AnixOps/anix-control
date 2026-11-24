// Sign-in and the user pages (UI redesign phase U5): 登录, 概览, 订阅,
// 帮助中心, 工单, 套餐, 订单. Written for Chinese readers, not translated from
// the English file (guidelines/voice-and-tone.md).
export default {
  auth: {
    signIn: {
      title: '登录 AnixOps Control',
      subtitle: '订阅、节点与转发管理。',
      email: '邮箱',
      password: '密码',
      submit: '登录',
      noAccount: '没有账户？',
      register: '注册'
    },
    register: {
      title: '创建账户',
      subtitle: '注册后获取订阅链接。',
      email: '邮箱',
      password: '密码',
      passwordHelp: '至少 6 个字符。',
      confirm: '确认密码',
      invite: '邀请码',
      inviteHelp: '邀请码请向管理员索取。',
      submit: '创建账户',
      haveAccount: '已有账户？',
      signIn: '登录',
      done: '账户已创建'
    },
    mfa: {
      title: '两步验证',
      description: '输入 {email} 的 6 位验证码。',
      code: '6 位验证码',
      submit: '验证',
      back: '返回',
      useRecovery: '使用恢复码',
      recoveryTitle: '使用恢复码',
      recoveryDescription: '输入恢复码。每个恢复码仅可使用一次。',
      recovery: '恢复码',
      useCode: '改用验证码'
    },
    enroll: {
      title: '需要开启两步验证',
      description: '所有账户须开启两步验证后方可登录，{email} 尚未开启。',
      stepsTitle: '处理步骤',
      step1: '联系管理员，在“两步验证策略”中暂时关闭强制要求。',
      step2: '登录后在“账户”中开启两步验证并保存恢复码。',
      step3: '通知管理员恢复强制要求。',
      app: '需使用身份验证器，如 1Password、Google Authenticator 或 Microsoft Authenticator。',
      back: '返回登录'
    },
    errors: {
      emailRequired: '请输入邮箱。',
      emailInvalid: "邮箱格式不正确，示例：name{'@'}example.com。",
      passwordRequired: '请输入密码。',
      passwordShort: '密码至少需要 6 个字符。',
      passwordMismatch: '两次输入的密码不一致。',
      inviteRequired: '请输入邀请码。',
      codeIncomplete: '请输入完整的 6 位验证码。',
      codeInvalid: '验证码不正确。请校准设备时间后输入最新验证码。',
      recoveryFormat: '恢复码为 8 位字母或数字，示例：ABCD-1234。',
      recoveryInvalid: '恢复码不正确或已使用。',
      rateLimited: '尝试次数过多，请稍后再试。',
      signInFailed: '无法登录：{message}',
      registerFailed: '无法创建账户：{message}',
      network: '无法连接服务器，请检查网络后重试。'
    },
    dev: {
      title: '开发模式',
      user: '以用户身份进入',
      admin: '以管理员身份进入'
    }
  },
  portal: {
    state: {
      retry: '重试'
    },
    home: {
      greeting: '您好，{name}',
      greetingAnonymous: '您好',
      summary: {
        active: '订阅正常，剩余流量 {percent}。',
        low: '剩余流量 {percent}。',
        exhausted: '流量已用完，服务暂停。',
        expired: '订阅已于 {date} 到期。',
        none: '暂无订阅。'
      },
      next: {
        community: '请联系管理员开通或续期。',
        commercial: '请选购套餐。'
      },
      ring: {
        label: '剩余流量 {remaining}，共 {total}',
        caption: '{unit} 剩余 · 共 {total}',
        empty: '没有可用流量'
      },
      facts: {
        status: '状态',
        expires: '到期',
        never: '长期有效',
        daysLeft: '剩余 {n} 天',
        lastDay: '今天到期',
        ended: '已结束',
        used: '已用',
        upDown: '上传 {up} · 下载 {down}'
      },
      status: {
        active: '有效',
        expired: '已到期',
        exhausted: '流量用完',
        none: '未订阅'
      },
      plan: {
        community: '订阅模板：{name}',
        commercial: '套餐：{name}'
      },
      copyLink: '复制订阅链接',
      copied: '已复制订阅链接',
      copyFailed: '无法自动复制，请在“订阅”页手动复制。',
      importToClient: '导入到客户端',
      buyPlan: '选购套餐',
      orders: '我的订单',
      updatedAt: '数据更新于 {time}',
      refresh: '刷新',
      refreshing: '正在刷新',
      loadFailed: '无法加载订阅信息。',
      help: {
        title: '帮助中心',
        all: '全部文章',
        empty: '暂无帮助文章。'
      },
      tickets: {
        title: '最近工单',
        new: '新建工单',
        all: '全部工单',
        empty: '暂无工单。'
      }
    },
    subscribe: {
      title: '订阅',
      description: '将订阅链接导入客户端。链接等同于密码，请勿分享。',
      link: '订阅链接',
      linkHelp: '客户端按其设置定期更新订阅。',
      domain: '订阅域名',
      copy: '复制',
      facts: {
        remaining: '剩余流量',
        used: '已用',
        expires: '到期'
      },
      qr: {
        label: '订阅链接二维码',
        caption: '使用手机客户端扫码导入'
      },
      missing: '暂无订阅链接。',
      loadFailed: '无法加载订阅信息。',
      anx: {
        title: 'AnixOps Net 客户端（ANX 版）',
        hint: '本站专用客户端 {version}，支持 ANX 协议',
        os: { windows: 'Windows', macos: 'macOS', linux: 'Linux', android: '安卓', ios: 'iPhone / iPad' },
        kind: {
          windows: '安装版',
          'windows-portable': '便携版',
          macos: '安装包',
          'macos-dmg': '磁盘映像',
          'linux-deb': '.deb',
          linux: '.tar.gz',
          android: 'APK'
        },
        downloadLabel: '下载 {os} {name}',
        testflight: '加入 TestFlight',
        testflightAsk: '通过 TestFlight 安装，邀请请联系客服获取。',
        verify: '下载链接包含订阅令牌，请勿分享。校验文件：'
      },
      clients: {
        title: '导入到客户端',
        hint: '在已安装的客户端中打开',
        import: '一键导入',
        copy: '复制链接',
        copied: '已复制 {name} 的订阅链接',
        copyFailed: '无法自动复制，请手动复制上方订阅链接。',
        importLabel: '导入到 {name}',
        copyLabel: '复制 {name} 的订阅链接',
        allPlatforms: '全平台'
      },
      formats: {
        title: '其他格式',
        description: '按客户端支持的格式复制链接或预览内容。',
        format: '格式',
        preview: '预览',
        previewTitle: '{format}订阅内容',
        previewDescription: '客户端将收到的内容，包含节点密码，请勿分享。',
        previewEmpty: '订阅内容为空。',
        previewFailed: '无法获取订阅内容（{reason}）。',
        download: '下载',
        copyContent: '复制内容',
        names: {
          auto: '自动识别（按客户端）',
          v2ray: 'V2Ray（Base64）',
          clash: 'Clash（YAML）',
          stash: 'Stash（YAML）',
          egern: 'Egern（YAML）',
          surge: 'Surge',
          loon: 'Loon',
          shadowrocket: 'Shadowrocket',
          quantumultx: 'Quantumult X',
          singBox: 'sing-box（JSON）',
          wireguard: 'WireGuard（.conf）',
          iwan: 'iWAN（iwand 配置）',
          json: '原始 JSON',
          base64json: 'Base64 JSON'
        }
      },
      danger: {
        title: '危险区',
        resetTitle: '重置订阅链接',
        resetDescription: '重置后旧链接立即失效，所有设备须重新导入。',
        resetAction: '重置链接…',
        dialogTitle: '重置订阅链接？',
        dialogPassword: '旧链接将立即失效。请输入当前密码确认。',
        dialogCode: '旧链接将立即失效。请输入 6 位验证码。',
        dialogRecovery: '旧链接将立即失效。请输入恢复码，每个恢复码仅可使用一次。',
        checking: '正在检查两步验证…',
        password: '当前密码',
        code: '验证码',
        recovery: '恢复码',
        useRecovery: '使用恢复码',
        useCode: '使用验证码',
        cancel: '取消',
        confirm: '重置链接',
        done: '订阅链接已重置，请在所有设备上重新导入',
        doneTitle: '订阅链接已重置',
        doneDescription: '旧链接已失效。请在各设备上导入新链接或扫描二维码。',
        newLink: '新的订阅链接',
        close: '完成',
        errors: {
          password: '请输入当前密码。',
          passwordWrong: '密码不正确。',
          code: '请输入 6 位验证码。',
          codeWrong: '验证码不正确或已过期。',
          recoveryFormat: '恢复码为 8 位字母或数字，示例：ABCD-1234。',
          recoveryWrong: '恢复码不正确或已使用。',
          rateLimited: '尝试次数过多，请一小时后重试。',
          network: '无法连接服务器',
          failed: '链接未重置：{message}'
        }
      }
    },
    help: {
      title: '帮助中心',
      description: '使用说明、常见问题与公告。',
      search: '搜索帮助文章',
      categories: '分类',
      all: '全部',
      uncategorized: '其他',
      articles: '{n} 篇文章',
      results: '“{query}”的搜索结果',
      noResults: '未找到与“{query}”相关的文章。',
      clearSearch: '清除搜索',
      empty: '暂无帮助文章。',
      updated: '更新于 {date}',
      back: '帮助中心',
      prev: '上一篇',
      next: '下一篇',
      toc: '本文内容',
      notFound: '文章不存在',
      notFoundHint: '文章可能已被删除或隐藏。',
      loadFailed: '无法加载帮助文章。',
      contact: '未找到答案？',
      contactAction: '提交工单'
    },
    tickets: {
      title: '工单',
      description: '联系客服及查看回复。',
      new: '新建工单',
      list: '我的工单',
      empty: '暂无工单',
      emptyHint: '如有问题，请新建工单。',
      select: '选择工单以查看对话。',
      status: {
        open: '处理中',
        answered: '已回复',
        closed: '已关闭'
      },
      priority: {
        label: '优先级',
        low: '低',
        medium: '中',
        high: '高'
      },
      subject: '主题',
      subjectPlaceholder: '一句话描述问题',
      message: '详细描述',
      messageHelp: '请注明设备、客户端及问题发生时间。',
      submit: '提交工单',
      created: '工单已提交。',
      reply: '回复',
      replyPlaceholder: '输入回复…',
      send: '发送',
      sendHint: '按 Ctrl + Enter 发送',
      close: '关闭工单…',
      closeTitle: '关闭工单“{subject}”？',
      closeMessage: '关闭后无法回复。如需继续咨询，请新建工单。',
      closeConfirm: '关闭工单',
      closed: '工单已关闭',
      closedNote: '工单已关闭。如需继续咨询，请新建工单。',
      me: '我',
      support: '客服',
      backToList: '我的工单',
      number: '#{id}',
      updated: '{time}更新',
      errors: {
        subject: '请填写主题。',
        message: '请填写详细描述。',
        reply: '请输入回复内容。',
        load: '无法加载工单。',
        detail: '无法打开该工单。',
        send: '回复未发送：{message}',
        create: '工单未提交：{message}'
      }
    },
    plans: {
      title: '套餐',
      description: '套餐选购与续费。',
      period: '计费周期',
      periods: {
        month: '月付',
        quarter: '季付',
        half_year: '半年付',
        year: '年付',
        two_year: '两年付',
        three_year: '三年付',
        onetime: '一次性'
      },
      per: {
        month: '/ 月',
        quarter: '/ 季',
        half_year: '/ 半年',
        year: '/ 年',
        two_year: '/ 两年',
        three_year: '/ 三年',
        onetime: '一次性'
      },
      notOffered: '此周期不提供',
      traffic: '{value} 流量',
      speed: '最高 {value} Mbps',
      devices: '最多 {n} 台设备同时在线',
      buy: '购买',
      empty: '暂无在售套餐',
      emptyHint: '套餐上架后将在此显示。',
      loadFailed: '无法加载套餐。',
      checkout: {
        title: '确认订单',
        plan: '套餐',
        period: '计费周期',
        coupon: '优惠码',
        couponPlaceholder: '选填',
        apply: '使用',
        remove: '移除',
        couponApplied: '已使用优惠码“{name}”',
        subtotal: '原价',
        discount: '优惠',
        total: '应付',
        submit: '提交订单',
        back: '返回',
        created: '订单已创建，请在“订单”中完成支付。',
        couponInvalid: '优惠码无效或不适用于该套餐。',
        failed: '订单未创建：{message}'
      }
    },
    destinationStats: {
      loadFailed: '访问统计加载失败',
      empty: '该时间段内无访问记录',
      summary: {
        label: '访问统计汇总',
        total: '总流量',
        split: '上传 {upload} · 下载 {download}',
        topCategory: '最多的类型',
        domains: '访问的网站',
        connections: '{count} 次连接'
      },
      categories: {
        title: '流量类型',
        description: '按目标域名归类，类型为估算值。'
      },
      domains: {
        title: '访问的网站',
        description: '按主域名汇总，按流量降序排列。',
        label: '访问的网站',
        domain: '网站',
        category: '类型',
        total: '流量',
        upload: '上传',
        download: '下载',
        connections: '连接数',
        other: '其他（超出单次上报上限）'
      },
      category: {
        video: '视频',
        music: '音乐',
        messaging: '通讯',
        social: '社交',
        gaming: '游戏',
        ai: 'AI',
        download: '下载',
        web: '网站',
        unknown: '未知'
      }
    },
    traffic: {
      title: '访问统计',
      description: '经由节点访问的网站及流量类型。仅记录域名，不记录网址和内容。',
      rangeLabel: '时间范围',
      range: {
        '24h': '24 小时',
        '7d': '7 天',
        '30d': '30 天'
      },
      retention: '记录保留 {days} 天。',
      off: '访问统计已关闭，不再记录新数据。',
      empty: '该时间段内无访问记录',
      emptyHint: '记录将在使用后一小时内显示。'
    },
    orders: {
      title: '订单',
      description: '购买记录与支付状态。',
      empty: '暂无订单',
      emptyHint: '选购套餐后，订单将在此显示。',
      browse: '选购套餐',
      loadFailed: '无法加载订单。',
      detailFailed: '无法打开订单详情。',
      status: {
        pending: '待支付',
        paid: '已支付',
        cancelled: '已取消',
        completed: '已完成',
        discounted: '已抵扣',
        unknown: '未知'
      },
      columns: {
        plan: '套餐',
        period: '周期',
        amount: '金额',
        status: '状态',
        createdAt: '下单时间'
      },
      unknownPlan: '已下架的套餐',
      details: '详情',
      pay: '去支付',
      payPending: '在线支付未开放，请联系管理员完成支付。',
      detail: {
        title: '订单详情',
        tradeNo: '订单号',
        status: '状态',
        plan: '套餐',
        period: '周期',
        subtotal: '金额',
        discount: '优惠',
        total: '实付',
        createdAt: '下单时间',
        paidAt: '支付时间'
      }
    }
  }
}
