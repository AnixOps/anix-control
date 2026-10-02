// Sign-in and the user pages (UI redesign phase U5): 登录, 概览, 订阅,
// 帮助中心, 工单, 套餐, 订单. Written for Chinese readers, not translated from
// the English file (guidelines/voice-and-tone.md).
export default {
  auth: {
    signIn: {
      title: '登录 AnixOps Control',
      subtitle: '管理你的订阅、节点与转发。',
      email: '邮箱',
      password: '密码',
      submit: '登录',
      noAccount: '还没有账户？',
      register: '注册'
    },
    register: {
      title: '创建账户',
      subtitle: '注册后即可获取订阅链接。',
      email: '邮箱',
      password: '密码',
      passwordHelp: '至少 6 个字符。',
      confirm: '确认密码',
      invite: '邀请码',
      inviteHelp: '没有邀请码？向管理员索取。',
      submit: '创建账户',
      haveAccount: '已有账户？',
      signIn: '登录',
      done: '账户已创建'
    },
    mfa: {
      title: '两步验证',
      description: '输入身份验证器中为 {email} 生成的 6 位验证码。',
      code: '6 位验证码',
      submit: '验证',
      back: '返回',
      useRecovery: '使用恢复码',
      recoveryTitle: '使用恢复码',
      recoveryDescription: '输入开启两步验证时保存的恢复码。每个恢复码只能使用一次。',
      recovery: '恢复码',
      useCode: '改用验证码'
    },
    enroll: {
      title: '先开启两步验证',
      description: '管理员要求所有账户开启两步验证后才能登录，{email} 还没有开启。',
      stepsTitle: '这样做',
      step1: '联系管理员，请其在「两步验证策略」中暂时关闭强制要求。',
      step2: '登录后打开「账户」，开启两步验证并保存恢复码。',
      step3: '告诉管理员已经开启，管理员即可恢复强制要求。',
      app: '你需要一个身份验证器 App，如 1Password、Google Authenticator 或 Microsoft Authenticator。',
      back: '返回登录'
    },
    errors: {
      emailRequired: '请输入邮箱。',
      emailInvalid: "邮箱格式不正确，例如 name{'@'}example.com。",
      passwordRequired: '请输入密码。',
      passwordShort: '密码至少需要 6 个字符。',
      passwordMismatch: '两次输入的密码不一致。',
      inviteRequired: '请输入邀请码。',
      codeIncomplete: '请输入完整的 6 位验证码。',
      codeInvalid: '验证码不正确。确认手机时间准确后，输入验证器中的最新验证码。',
      recoveryFormat: '恢复码由 8 个字母或数字组成，例如 ABCD-1234。',
      recoveryInvalid: '恢复码不正确，或已经使用过。',
      rateLimited: '尝试次数过多，请稍后再试。',
      signInFailed: '无法登录：{message}',
      registerFailed: '无法创建账户：{message}',
      network: '无法连接服务器。检查网络后重试。'
    },
    dev: {
      title: '开发模式',
      user: '以用户身份进入',
      admin: '以管理员身份进入'
    }
  },
  portal: {
    state: {
      retry: '重试',
      copyDetails: '复制错误详情',
      detailsCopied: '已复制错误详情'
    },
    home: {
      greeting: '你好，{name}',
      greetingAnonymous: '你好',
      summary: {
        active: '订阅运行正常，还剩 {percent} 的流量。',
        low: '流量只剩 {percent}，请留意用量。',
        exhausted: '流量已经用完，暂时无法使用。',
        expired: '订阅已于 {date} 到期。',
        none: '你还没有订阅。'
      },
      next: {
        community: '联系管理员开通或续期。',
        commercial: '选购套餐后即可使用。'
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
        daysLeft: '还有 {n} 天',
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
      copyFailed: '无法自动复制。打开「订阅」页可以手动复制链接。',
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
        empty: '还没有帮助文章。'
      },
      tickets: {
        title: '最近工单',
        new: '新建工单',
        all: '全部工单',
        empty: '还没有工单。遇到问题时，在这里联系客服。'
      }
    },
    subscribe: {
      title: '订阅',
      description: '把订阅链接导入客户端即可使用全部节点。链接等同于密码，请勿分享。',
      link: '订阅链接',
      linkHelp: '客户端会按自己的设置定期更新订阅。',
      domain: '订阅域名',
      copy: '复制',
      facts: {
        remaining: '剩余流量',
        used: '已用',
        expires: '到期'
      },
      qr: {
        label: '订阅链接二维码',
        caption: '用手机客户端扫码导入'
      },
      missing: '还没有订阅链接。',
      loadFailed: '无法加载订阅信息。',
      clients: {
        title: '导入到客户端',
        hint: '点击后在已安装的客户端中打开',
        import: '一键导入',
        copy: '复制链接',
        copied: '已复制 {name} 的订阅链接',
        copyFailed: '无法自动复制，请在上方手动复制订阅链接。',
        importLabel: '导入到 {name}',
        copyLabel: '复制 {name} 的订阅链接',
        allPlatforms: '全平台'
      },
      formats: {
        title: '其他格式',
        description: '客户端不在上面时，选择它支持的格式，复制链接或预览内容。',
        format: '格式',
        preview: '预览',
        previewTitle: '{format}订阅内容',
        previewDescription: '这是客户端会收到的内容，包含节点密码，请勿分享。',
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
          json: '原始 JSON',
          base64json: 'Base64 JSON'
        }
      },
      danger: {
        title: '危险区',
        resetTitle: '重置订阅链接',
        resetDescription: '如果链接泄露，重置后旧链接立即失效，所有设备需要重新导入。重置由管理员完成。',
        resetAction: '申请重置…',
        confirmTitle: '向管理员申请重置订阅链接？',
        confirmDescription: '我们会替你提交一张工单。管理员重置后，所有设备都要重新导入新链接。',
        cancel: '取消',
        confirm: '提交申请',
        submitted: '已提交重置申请，管理员处理后会在工单中回复。',
        failed: '申请没有提交：{message}',
        viewTicket: '查看工单',
        ticketSubject: '申请重置订阅链接',
        ticketMessage: '我的订阅链接可能已经泄露，请帮我重置。'
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
      results: '「{query}」的搜索结果',
      noResults: '没有找到与「{query}」相关的文章。换个关键词试试。',
      clearSearch: '清除搜索',
      empty: '还没有帮助文章。管理员发布后会显示在这里。',
      updated: '更新于 {date}',
      back: '帮助中心',
      prev: '上一篇',
      next: '下一篇',
      toc: '本文内容',
      notFound: '找不到这篇文章',
      notFoundHint: '它可能已被删除或隐藏。',
      loadFailed: '无法加载帮助文章。',
      contact: '没有找到答案？',
      contactAction: '提交工单'
    },
    tickets: {
      title: '工单',
      description: '遇到问题时联系客服，回复会显示在这里。',
      new: '新建工单',
      list: '我的工单',
      empty: '还没有工单',
      emptyHint: '遇到问题时，新建一张工单联系客服。',
      select: '选择一张工单，查看与客服的对话。',
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
      messageHelp: '写明设备、客户端和出现问题的时间，便于更快定位。',
      submit: '提交工单',
      created: '工单已提交，客服会尽快回复。',
      reply: '回复',
      replyPlaceholder: '输入回复…',
      send: '发送',
      sendHint: '按 Ctrl + Enter 发送',
      close: '关闭工单…',
      closeTitle: '关闭工单「{subject}」？',
      closeMessage: '关闭后不能再回复。如需继续咨询，请新建工单。',
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
        detail: '无法打开这张工单。',
        send: '回复没有发出：{message}',
        create: '工单没有提交：{message}'
      }
    },
    plans: {
      title: '套餐',
      description: '选择适合你的套餐，到期前可随时续费。',
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
      empty: '暂时没有在售的套餐',
      emptyHint: '管理员上架后会显示在这里。',
      loadFailed: '无法加载套餐。',
      checkout: {
        title: '确认订单',
        plan: '套餐',
        period: '计费周期',
        coupon: '优惠码',
        couponPlaceholder: '没有可以留空',
        apply: '使用',
        remove: '移除',
        couponApplied: '已使用优惠码「{name}」',
        subtotal: '原价',
        discount: '优惠',
        total: '应付',
        submit: '提交订单',
        back: '返回',
        created: '订单已创建，请在「订单」中完成支付。',
        couponInvalid: '优惠码无效或不适用于这个套餐。',
        failed: '订单没有创建：{message}'
      }
    },
    orders: {
      title: '订单',
      description: '你的购买记录与支付状态。',
      empty: '还没有订单',
      emptyHint: '选购套餐后，订单会显示在这里。',
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
      payPending: '在线支付暂未开放，请联系管理员完成支付。',
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
