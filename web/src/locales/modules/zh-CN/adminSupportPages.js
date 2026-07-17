export default {
  adminCoupons: {
    title: '优惠券管理',
    subtitle: '创建和管理订单优惠券。',
    currencySymbol: '¥',
    actions: {
      createCoupon: '新建优惠券'
    },
    table: {
      id: 'ID',
      code: '券码',
      name: '名称',
      type: '类型',
      value: '面值',
      usageCount: '使用情况',
      validity: '有效期',
      actions: '操作',
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
    placeholders: {
      code: 'SUMMER2026',
      name: '夏季活动'
    },
    empty: {
      noData: '暂无优惠券'
    },
    messages: {
      fetchFailed: '加载优惠券失败',
      requiredFields: '请填写优惠券码和优惠券名称',
      createSuccess: '优惠券创建成功',
      createFailed: '优惠券创建失败',
      deleteConfirm: '确定删除优惠券 {code} 吗？',
      deleteFailed: '删除优惠券失败'
    }
  },
  adminOrders: {
    title: '订单管理',
    subtitle: '查看订单、处理待支付订单并取消无效记录。',
    stats: {
      totalOrders: '订单总数',
      pendingOrders: '待支付订单',
      totalRevenue: '总收入',
      todayRevenue: '今日收入'
    },
    filters: {
      tradeNo: '订单号',
      email: '用户邮箱',
      allStatus: '全部状态'
    },
    status: {
      pending: '待支付',
      paid: '已支付',
      cancelled: '已取消',
      completed: '已完成',
      unknown: '未知'
    },
    actions: {
      search: '搜索',
      markPaid: '标记已支付',
      cancelOrder: '取消订单'
    },
    table: {
      tradeNo: '订单号',
      user: '用户',
      plan: '套餐',
      period: '周期',
      amount: '金额',
      status: '状态',
      createdAt: '创建时间',
      actions: '操作'
    },
    detailModal: {
      title: '订单详情',
      tradeNo: '订单编号',
      userEmail: '用户邮箱',
      plan: '套餐',
      period: '购买周期',
      amount: '金额',
      status: '状态',
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
      noData: '暂无订单'
    },
    pagination: {
      prev: '上一页',
      next: '下一页',
      info: '第 {page} / {totalPages} 页'
    },
    messages: {
      fetchOrdersFailed: '加载订单失败',
      fetchStatsFailed: '加载订单统计失败',
      markPaidConfirm: '确定将订单 {tradeNo} 标记为已支付吗？',
      markPaidSuccess: '订单已标记为支付成功',
      markPaidFailed: '标记订单支付失败：{message}',
      markPaidFailedShort: '标记失败',
      cancelConfirm: '确定取消订单 {tradeNo} 吗？',
      cancelFailed: '取消订单失败：{message}',
      cancelFailedShort: '取消失败'
    }
  },
  adminInvite: {
    title: '邀请返利',
    subtitle: '配置邀请佣金、审核提现申请并查看排行。',
    currencySymbol: '¥',
    tabs: {
      config: '配置',
      withdrawals: '提现申请',
      stats: '统计'
    },
    config: {
      inviteTitle: '邀请配置',
      enabled: '启用邀请返利',
      codePrefix: '邀请码前缀',
      codeLength: '邀请码长度',
      commissionTitle: '佣金规则',
      commissionRate: '佣金比例 (%)',
      commissionRateHelp: '每笔符合条件的订单按此比例返给邀请人。',
      commissionType: '佣金类型',
      commissionFixed: '固定佣金金额',
      withdrawTitle: '提现规则',
      minWithdraw: '最小提现金额',
      withdrawFee: '提现手续费 (%)',
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
      search: '搜索',
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
      filters: {
        all: '全部状态'
      },
      table: {
        id: 'ID',
        userId: '用户 ID',
        amount: '金额',
        method: '方式',
        account: '账号',
        status: '状态',
        createdAt: '创建时间',
        actions: '操作'
      },
      empty: '暂无提现申请'
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
    messages: {
      fetchConfigFailed: '加载邀请配置失败',
      fetchWithdrawalsFailed: '加载提现申请失败',
      fetchStatsFailed: '加载邀请统计失败',
      saveSuccess: '邀请配置已保存',
      saveFailed: '保存邀请配置失败：{message}',
      saveFailedShort: '保存失败',
      approveConfirm: '确定通过这笔提现申请吗？',
      approveSuccess: '提现申请已通过',
      approveFailed: '通过提现申请失败：{message}',
      approveFailedShort: '审核失败',
      rejectConfirm: '确定拒绝这笔提现申请吗？',
      rejectSuccess: '提现申请已拒绝',
      rejectFailed: '拒绝提现申请失败：{message}',
      rejectFailedShort: '拒绝失败'
    }
  },
  adminTelegram: {
    title: 'Telegram Bot',
    subtitle: '管理机器人配置、绑定用户和消息发送。',
    tabs: {
      config: '配置',
      users: '用户',
      notify: '通知'
    },
    config: {
      basicTitle: '机器人配置',
      token: '机器人 Token',
      webhookUrl: 'Webhook 地址',
      adminIds: '管理员 Telegram ID',
      welcomeMessage: '欢迎消息'
    },
    actions: {
      setWebhook: '设置 Webhook',
      deleteWebhook: '删除 Webhook',
      refreshUsers: '刷新用户',
      toggleNotify: '切换通知状态',
      enableNotifyLabel: '启用',
      disableNotifyLabel: '停用',
      send: '发送',
      broadcast: '广播'
    },
    commands: {
      title: '支持的命令',
      items: {
        start: '启动机器人',
        bind: '绑定账号',
        unbind: '解绑账号',
        info: '查看账户信息',
        sub: '查看订阅链接',
        renew: '查看续费入口',
        ticket: '创建工单',
        help: '查看帮助'
      }
    },
    users: {
      searchPlaceholder: '按 Telegram ID 或邮箱搜索',
      empty: '暂无已绑定的 Telegram 用户',
      table: {
        id: 'ID',
        telegramId: 'Telegram ID',
        userId: '用户 ID',
        userEmail: '用户邮箱',
        boundAt: '绑定时间',
        notifyStatus: '通知状态',
        actions: '操作'
      }
    },
    status: {
      enabled: '已启用',
      disabled: '已停用'
    },
    notify: {
      type: '发送类型',
      telegramId: 'Telegram ID',
      message: '消息内容',
      types: {
        single: '单用户',
        broadcast: '广播'
      }
    },
    placeholders: {
      token: '123456:ABCDEF...',
      adminIds: '123456789,987654321',
      welcomeMessage: '欢迎使用机器人。',
      telegramId: '输入 Telegram ID',
      message: '输入消息内容'
    },
    tips: {
      title: '广播提示',
      broadcastScope: '广播只会发送给已经绑定 Telegram 的用户。',
      markdown: '在支持的场景下，消息可使用 Telegram Markdown。',
      singleFirst: '建议先单发测试，再执行广播。'
    },
    messages: {
      fetchConfigFailed: '加载 Telegram Bot 配置失败',
      fetchUsersFailed: '加载 Telegram 用户失败',
      saveSuccess: 'Telegram Bot 配置已保存',
      saveFailed: '保存 Telegram Bot 配置失败：{message}',
      saveFailedShort: '保存失败',
      webhookSetSuccess: 'Webhook 设置成功',
      webhookSetFailed: '设置 Webhook 失败：{message}',
      webhookSetFailedShort: '设置失败',
      webhookDeleteSuccess: 'Webhook 已删除',
      webhookDeleteFailed: '删除 Webhook 失败：{message}',
      webhookDeleteFailedShort: '删除失败',
      toggleNotifyFailed: '更新通知状态失败：{message}',
      toggleNotifyFailedShort: '更新失败',
      messageRequired: '请输入消息内容',
      telegramIdRequired: '请输入 Telegram ID',
      sendSuccess: '消息发送成功',
      sendFailed: '消息发送失败：{message}',
      sendFailedShort: '发送失败',
      broadcastComplete: '广播完成。成功：{success}，失败：{failed}'
    }
  },
  adminTickets: {
    title: '工单管理',
    subtitle: '统一查看用户工单并进行回复。',
    icons: {
      open: '!',
      answered: '✓',
      closed: '×'
    },
    stats: {
      open: '待处理',
      answered: '已回复',
      closed: '已关闭'
    },
    table: {
      id: 'ID',
      userId: '用户 ID',
      subject: '主题',
      priority: '优先级',
      status: '状态',
      createdAt: '创建时间',
      actions: '操作'
    },
    actions: {
      reply: '回复',
      closeTicket: '关闭',
      sendReply: '发送回复'
    },
    empty: {
      noData: '暂无工单'
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
    replyModal: {
      title: '回复工单 #{id}',
      subject: '主题：',
      userId: '用户 ID：',
      content: '回复内容',
      placeholder: '输入回复内容...'
    },
    messages: {
      fetchFailed: '加载工单失败',
      replyRequired: '请输入回复内容',
      replySuccess: '回复发送成功',
      replyFailed: '回复发送失败',
      closeConfirm: '确定关闭该工单吗？',
      closeFailed: '关闭工单失败'
    }
  },
  adminKnowledge: {
    title: '知识库管理',
    subtitle: '发布公告、教程、常见问题和支持文章。',
    actions: {
      createArticle: '新建文章',
      publish: '发布'
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
      content: '内容',
      sort: '排序',
      visibility: '可见性'
    },
    placeholders: {
      title: '文章标题',
      body: '填写文章内容...'
    },
    modal: {
      createTitle: '新建文章',
      editTitle: '编辑文章'
    },
    empty: {
      noData: '暂无文章'
    },
    messages: {
      fetchFailed: '加载文章失败',
      requiredFields: '请填写标题和内容',
      saveSuccess: '文章保存成功',
      publishSuccess: '文章发布成功',
      actionFailed: '文章操作失败',
      deleteConfirm: '确认删除文章“{title}”吗？',
      deleteFailed: '删除文章失败'
    }
  }
}
