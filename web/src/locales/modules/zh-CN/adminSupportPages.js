export default {
  adminCoupons: {
    title: '优惠券',
    subtitle: '用户下单时输入的优惠码。',
    actions: {
      createCoupon: '新建优惠券',
      create: '创建优惠券'
    },
    filters: {
      search: '搜索券码或名称',
      type: '按类型筛选'
    },
    table: {
      label: '优惠券列表',
      id: 'ID',
      code: '券码',
      name: '名称',
      type: '类型',
      value: '面值',
      usageCount: '使用情况',
      validity: '有效期',
      unlimited: '不限'
    },
    types: {
      discount: '折扣',
      fixed: '固定金额',
      discountPercent: '折扣 (%)',
      fixedCents: '固定金额 (分)'
    },
    fields: {
      code: '优惠券码',
      name: '优惠券名称',
      type: '优惠类型',
      discountValue: '折扣值',
      fixedValue: '固定金额 (分)',
      startTime: '开始时间',
      endTime: '结束时间',
      limitUse: '使用次数限制'
    },
    modal: {
      title: '新建优惠券'
    },
    help: {
      code: '保存时转为大写。',
      startTime: '留空则从现在开始。',
      endTime: '留空则 30 天后结束。',
      limitUse: '-1 表示不限次数。'
    },
    units: {
      cents: '分'
    },
    placeholders: {
      code: 'SUMMER2026',
      name: '夏季活动'
    },
    empty: {
      title: '暂无优惠券',
      description: '创建优惠码，供用户下单时使用。'
    },
    confirm: {
      deleteTitle: '删除优惠券 {code}？',
      deleteMessage: '删除后该优惠码将无法使用。此操作无法撤销。',
      deleteAction: '删除优惠券'
    },
    messages: {
      fetchFailed: '优惠券加载失败',
      codeRequired: '请填写优惠券码',
      nameRequired: '请填写优惠券名称',
      createSuccess: '优惠券创建成功',
      createFailed: '优惠券创建失败',
      deleted: '已删除优惠券 {code}',
      deleteFailed: '删除优惠券失败'
    }
  },
  adminOrders: {
    title: '订单',
    subtitle: '全部购买、续费与升级订单。选择订单查看支付情况。',
    stats: {
      totalOrders: '订单总数',
      pendingOrders: '待支付订单',
      totalRevenue: '总收入',
      todayRevenue: '今日收入'
    },
    filters: {
      tradeNo: '订单号',
      email: '用户邮箱',
      label: '按状态筛选'
    },
    status: {
      pending: '待支付',
      paid: '已支付',
      cancelled: '已取消',
      completed: '已完成',
      unknown: '未知'
    },
    actions: {
      markPaid: '标记已支付'
    },
    table: {
      label: '订单列表',
      tradeNo: '订单号',
      user: '用户',
      plan: '套餐',
      period: '周期',
      amount: '金额',
      status: '状态',
      createdAt: '创建时间'
    },
    detailModal: {
      title: '订单详情',
      orderSection: '订单',
      paymentSection: '支付',
      tradeNo: '订单编号',
      userEmail: '用户邮箱',
      plan: '套餐',
      period: '购买周期',
      amount: '金额',
      type: '订单类型',
      createdAt: '创建时间',
      paidAt: '支付时间',
      callbackNo: '回调单号'
    },
    types: {
      new: '新购',
      renew: '续费',
      upgrade: '升级',
      resetTraffic: '重置流量',
      unknown: '未知'
    },
    empty: {
      title: '暂无订单',
      description: '显示用户购买或续费的订单。'
    },
    confirm: {
      markPaidTitle: '将订单 {tradeNo} 标记为已支付？',
      markPaidMessage: '订单金额 {amount}，将按已支付处理。此操作无法撤销。',
      cancelTitle: '取消订单 {tradeNo}？',
      cancelMessage: '取消后该订单将无法支付。此操作无法撤销。',
      cancelAction: '取消订单',
      keepOrder: '保留订单'
    },
    messages: {
      fetchOrdersFailed: '订单列表加载失败',
      fetchStatsFailed: '加载订单统计失败',
      markPaidSuccess: '订单已标记为支付成功',
      markPaidFailedShort: '标记失败',
      cancelSuccess: '已取消订单 {tradeNo}',
      cancelFailedShort: '取消失败'
    }
  },
  // 批量操作（用户、邀请码）的结果：接口逐项返回的错误码的说法，以及“重试”。
  adminBulk: {
    retry: '重试 {count} 个',
    list: '{details}。',
    separator: '；',
    errors: {
      not_found: '{count} 个不存在',
      conflict: '{count} 个已被使用，予以保留',
      forbidden_self: '不能封禁自己的账号',
      not_attempted: '{count} 个未执行（请求超时）',
      failed: '{count} 个失败'
    }
  },
  adminInviteCodes: {
    title: '邀请码',
    subtitle: '生成、复制与撤销注册邀请码。',
    rewardsLink: '邀请返佣',
    never: '永不过期',
    empty: {
      title: '暂无邀请码',
      description: '批量生成邀请码用于注册。'
    },
    registration: {
      requiredBadge: '必填',
      optionalBadge: '选填',
      required: '当前注册必须填写邀请码（auth.registration.require_invite）。',
      optional: '当前注册无需邀请码，已填写的邀请码仍将核销。'
    },
    generate: {
      title: '生成邀请码',
      description: '每个邀请码只能注册一次。',
      count: '数量',
      countHelp: '每次 1 到 {max} 个。',
      expireDays: '有效期',
      daysUnit: '天',
      expireDaysPlaceholder: '使用配置的默认值',
      expireDaysHelp: '留空使用默认有效期，0 表示永不过期。',
      submit: '生成',
      created: '已生成 {count} 个邀请码：'
    },
    filters: {
      label: '按状态筛选'
    },
    status: {
      unused: '未使用',
      used: '已使用',
      expired: '已过期'
    },
    owner: {
      admin: '管理员',
      user: '用户 #{id}'
    },
    table: {
      label: '邀请码列表',
      code: '邀请码',
      owner: '创建者',
      status: '状态',
      usedBy: '使用者',
      expiresAt: '过期时间',
      createdAt: '创建时间'
    },
    actions: {
      copy: '复制',
      copyAll: '全部复制',
      revoke: '撤销',
      revokeOne: '撤销邀请码…',
      done: '完成'
    },
    confirm: {
      revokeTitle: '撤销邀请码 {code}？',
      revokeManyTitle: '撤销 {count} 个邀请码？',
      revokeMessage: '撤销后无法用于注册。此操作无法撤销。'
    },
    messages: {
      failed: '请求失败',
      fetchFailed: '邀请码加载失败',
      countRange: '每次可生成 1 到 {max} 个邀请码。',
      generated: '已生成 {count} 个邀请码',
      generateFailed: '生成邀请码失败：{message}',
      revoked: '已撤销邀请码 {code}',
      revokedMany: '已撤销 {count} 个邀请码',
      revokePartial: '{total} 个邀请码中 {done} 个已撤销。',
      revokeNone: '未撤销任何邀请码。',
      revokeFailed: '撤销邀请码失败：{message}',
      copied: '已复制',
      copyFailed: '复制失败'
    }
  },
  adminInvite: {
    title: '邀请返佣',
    subtitle: '佣金提现审核、邀请排行与返佣规则。',
    currencySymbol: '¥',
    tabs: {
      label: '邀请返佣分区',
      config: '规则',
      withdrawals: '提现申请',
      stats: '统计'
    },
    config: {
      inviteTitle: '邀请码',
      enabled: '启用邀请返佣',
      codePrefix: '邀请码前缀',
      codeLength: '邀请码长度',
      codeLengthHelp: '4 到 16 个字符。',
      commissionTitle: '佣金',
      commissionRate: '佣金比例',
      commissionRateHelp: '符合条件的订单按此比例返佣给邀请人。',
      commissionType: '佣金类型',
      commissionFixed: '固定佣金金额',
      withdrawTitle: '提现',
      minWithdraw: '最小提现金额',
      withdrawFee: '提现手续费',
      withdrawMethods: '提现方式'
    },
    types: {
      percent: '比例',
      fixed: '固定金额'
    },
    methods: {
      alipay: '支付宝',
      wechat: '微信支付',
      bank: '银行卡'
    },
    actions: {
      approve: '通过',
      reject: '拒绝'
    },
    placeholders: {
      codePrefix: 'INV'
    },
    status: {
      pending: '待审核',
      approved: '已通过',
      rejected: '已拒绝'
    },
    withdrawals: {
      label: '提现申请列表',
      rowName: '提现申请 #{id}',
      filters: {
        label: '按状态筛选'
      },
      table: {
        id: 'ID',
        userId: '用户 ID',
        amount: '金额',
        method: '方式',
        account: '账号',
        status: '状态',
        createdAt: '申请时间'
      },
      empty: '暂无提现申请',
      emptyDescription: '显示待审核的佣金提现申请。'
    },
    stats: {
      totalInvites: '总邀请数',
      totalCommission: '累计佣金',
      pendingCommission: '待结算佣金',
      withdrawnCommission: '已提现佣金',
      rankingTitle: '邀请排行',
      empty: '暂无排行数据',
      table: {
        rank: '排名',
        userId: '用户 ID',
        inviteCount: '邀请人数',
        commission: '佣金'
      }
    },
    confirm: {
      approveTitle: '通过提现申请 #{id}？',
      approveMessage: '将向 {account} 发放 {amount}。此操作无法撤销。',
      rejectTitle: '拒绝提现申请 #{id}？',
      rejectMessage: '将拒绝金额为 {amount} 的申请。此操作无法撤销。'
    },
    messages: {
      fetchConfigFailed: '加载邀请配置失败',
      fetchWithdrawalsFailed: '加载提现申请失败',
      fetchStatsFailed: '加载邀请统计失败',
      saveSuccess: '邀请配置已保存',
      saveFailed: '保存邀请配置失败：{message}',
      saveFailedShort: '保存失败',
      approveSuccess: '提现申请已通过',
      approveFailedShort: '审核失败',
      rejectSuccess: '提现申请已拒绝',
      rejectFailedShort: '拒绝失败'
    }
  },
  adminTickets: {
    title: '工单',
    subtitle: '全部用户工单。选择工单进行回复或关闭。',
    list: '工单队列',
    loading: '正在加载工单…',
    select: '选择一个工单进行回复。',
    backToList: '全部工单',
    noSubject: '无主题',
    meta: '#{id} · 用户 {user} · {level}',
    priority: '优先级：{level}',
    threadNote: '管理端暂不显示历史对话。回复将显示在用户工单页，工单标记为已回复。',
    closedNote: '工单已关闭，不能再回复。',
    filters: {
      search: '搜索主题、编号或用户 ID',
      label: '按状态筛选',
      clear: '清除筛选'
    },
    facts: {
      number: '工单',
      user: '用户 ID',
      created: '创建于',
      updated: '最后更新'
    },
    actions: {
      closeTicket: '关闭工单…',
      sendReply: '发送回复'
    },
    empty: {
      title: '暂无工单',
      description: '显示用户提交的工单。',
      noMatches: '无符合条件的工单'
    },
    levels: {
      low: '低',
      medium: '中',
      high: '高',
      unknown: '未知'
    },
    status: {
      open: '待处理',
      answered: '已回复',
      closed: '已关闭',
      unknown: '未知'
    },
    reply: {
      content: '回复',
      placeholder: '给用户写回复',
      hint: '按 Ctrl+Enter 或 ⌘Enter 发送。'
    },
    quick: {
      label: '快捷回复',
      receivedLabel: '正在处理',
      received: '感谢反馈，我们正在排查，进展将在此更新。',
      detailsLabel: '询问细节',
      details: '请提供所用客户端、节点及问题出现的时间。',
      fixedLabel: '已修复',
      fixed: '问题已处理。请更新订阅后重试，如仍有问题请继续回复。'
    },
    confirm: {
      closeTitle: '关闭工单 #{id}“{subject}”？',
      closeMessage: '关闭后无法回复或重新打开。',
      closeAction: '关闭工单'
    },
    messages: {
      fetchFailed: '工单加载失败',
      replyRequired: '请先写回复内容',
      replySuccess: '已回复工单 #{id}',
      replyFailed: '回复发送失败',
      closed: '已关闭工单 #{id}',
      closeFailed: '工单关闭失败'
    }
  },
  adminKnowledge: {
    title: '帮助中心内容',
    subtitle: '帮助中心的公告、教程与常见问题。',
    actions: {
      createArticle: '新建文章',
      publish: '发布',
      show: '在帮助中心显示',
      hide: '从帮助中心隐藏',
      delete: '删除文章…'
    },
    table: {
      label: '文章列表',
      updatedAt: '更新时间'
    },
    filters: {
      search: '搜索文章',
      label: '按分类筛选'
    },
    categories: {
      announcement: '公告',
      tutorial: '教程',
      faq: '常见问题',
      other: '其他'
    },
    visibility: {
      visible: '显示',
      hidden: '隐藏'
    },
    fields: {
      title: '标题',
      category: '分类',
      content: '正文',
      sort: '排序',
      sortHelp: '数字越小越靠前。',
      visibility: '在帮助中心显示',
      visibilityHelp: '隐藏后文章保留，但对用户不可见。'
    },
    placeholders: {
      title: '文章标题',
      body: '正文。## 表示小节，- 表示列表项。'
    },
    editor: {
      view: '编辑视图',
      write: '编辑',
      preview: '预览',
      previewEmpty: '边写边在这里预览。',
      syntax: '## 标题，- 列表，1. 编号列表，**粗体**，`代码`，> 引用，[文字](https://…)。'
    },
    modal: {
      createTitle: '新建文章',
      editTitle: '编辑文章',
      description: '与帮助中心显示效果一致。'
    },
    empty: {
      title: '暂无文章',
      description: '文章发布后显示于帮助中心。'
    },
    confirm: {
      deleteTitle: '删除文章“{title}”？',
      deleteMessage: '文章将从帮助中心移除。此操作无法撤销。',
      deleteAction: '删除文章'
    },
    messages: {
      fetchFailed: '文章加载失败',
      requiredFields: '请填写标题和正文',
      titleRequired: '请填写标题',
      bodyRequired: '请填写正文',
      saveSuccess: '文章已保存',
      publishSuccess: '文章已发布',
      actionFailed: '文章保存失败',
      shown: '“{title}”已在帮助中心显示',
      hidden: '“{title}”已隐藏',
      deleted: '已删除文章“{title}”',
      deleteFailed: '文章删除失败'
    }
  }
}
