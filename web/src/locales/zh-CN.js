import runtimePages from './modules/zh-CN/runtimePages'
import networkPages from './modules/zh-CN/networkPages'
import miscPages from './modules/zh-CN/miscPages'
import adminSupportPages from './modules/zh-CN/adminSupportPages'
import adminDashboard from './modules/zh-CN/adminDashboard'
import adminMonitor from './modules/zh-CN/adminMonitor'
import adminTrafficHourly from './modules/zh-CN/adminTrafficHourly'
import { AGENT_NAME, CONTROL_NAME } from '../constants/brand'

const legacy = {
  'V2Board Admin': `${CONTROL_NAME} 管理台`,
  'Admin': '工作室管理台',
  'Overview': '概览',
  'Dashboard': '仪表盘',
  'Forwards': '流量转发',
  'Tunnels': '隧道管理',
  'Limits': '限速管理',
  'Ansible Machines': 'Ansible 机器',
  'Local Runtime': '本地运行时',
  'NodeX Topology': 'NodeX 拓扑',
  'NodeX Runtime': 'NodeX 运行时',
  'NodeX Agents': 'NodeX Agents',
  'Users': '用户管理',
  'Orders': '订单管理',
  'Tickets': '工单管理',
  'Nodes': '节点管理',
  'Subscriptions': '订阅管理',
  'Plans': '套餐管理',
  'Coupons': '优惠券管理',
  'Invite': '邀请返利管理',
  'Payment': '支付网关管理',
  'Notifications': '通知管理',
  'Knowledge': '知识库管理',
  'MFA': 'MFA 设置',
  'System': '系统管理',
  'Logout': '退出登录',
  'Refresh': '刷新',
  'Refreshing...': '刷新中...',
  'Save': '保存',
  'Cancel': '取消',
  'Close': '关闭',
  'Create': '创建',
  'Edit': '编辑',
  'Delete': '删除',
  'Preview': '预览',
  'Copy': '复制',
  'Copy Content': '复制内容',
  'Copy Link': '复制链接',
  'Download': '下载',
  'Open': '打开',
  'Details': '详情',
  'Back': '返回',
  'Submit': '提交',
  'Submit Order': '提交订单',
  'Buy Now': '立即选购',
  'Pay Now': '去支付',
  'Verify': '验证',
  'Checking...': '检查中...',
  'Remove': '移除',
  'Enabled': '启用',
  'Disabled': '禁用',
  'Status': '状态',
  'Description': '描述',
  'Priority': '优先级',
  'Format': '格式',
  'Traffic': '流量',
  'Traffic Usage': '流量使用',
  'Upload': '上传',
  'Download': '下载',
  'Remaining Days': '剩余天数',
  'Traffic Remaining': '流量剩余',
  'Expires At': '到期时间',
  'Created At': '创建时间',
  'Updated At': '更新时间',
  'Order Details': '订单详情',
  'Reply': '回复',
  'Submit Ticket': '提交工单',
  'Close Ticket': '关闭工单',
  'Read More': '阅读全文',
  'I Understand': '我知道了',
  'Loading...': '加载中...',
  'No data': '暂无数据',
  'All': '全部',
  'Permanent': '永久',
  'Active': '有效',
  'Expired': '已过期',
  'Pending': '待支付',
  'Paid': '已支付',
  'Cancelled': '已取消',
  'Completed': '已完成',
  'Unknown': '未知',
  'Open Ticket': '待处理',
  'Answered': '已回复',
  'Closed': '已关闭',
  'Current state': '当前状态',
  'Doctor Output': 'Doctor 输出',
  'Yes': '是',
  'No': '否',
  'ansible-playbook is available on the panel host': '\u9762\u677f\u4e3b\u673a\u5df2\u53ef\u7528 ansible-playbook',
  'Local ansible executor resolved inventory/playbooks and is ready to queue jobs': '\u672c\u5730 Ansible \u6267\u884c\u5668\u5df2\u89e3\u6790 inventory \u4e0e playbook\uff0c\u53ef\u4ee5\u5f00\u59cb\u6392\u961f\u6267\u884c\u4efb\u52a1',
  'Local nftables/Ansible executor is ready.': '\u672c\u5730 nftables/Ansible \u6267\u884c\u5668\u5df2\u5c31\u7eea\u3002',
  'NodeX /health responded with ok from the panel host': '\u9762\u677f\u4e3b\u673a\u8bbf\u95ee NodeX /health \u5df2\u8fd4\u56de ok',
  'NodeX runtime status responded and advertises gost support': 'NodeX \u8fd0\u884c\u65f6\u72b6\u6001\u63a5\u53e3\u5df2\u54cd\u5e94\uff0c\u5e76\u58f0\u660e\u652f\u6301 gost',
  'NodeX control plane is ready.': 'NodeX \u63a7\u5236\u9762\u5df2\u5c31\u7eea\u3002',
  'doctor ok': '\u8bca\u65ad\u5b8c\u6210'
  ,
  'Enable NodeX forward runtime mode': '\u542f\u7528 NodeX \u8f6c\u53d1\u8fd0\u884c\u65f6\u6a21\u5f0f',
  'Forward runtime backend': '\u8f6c\u53d1\u8fd0\u884c\u65f6 backend',
  'Preferred local ansible backend': '\u9996\u9009\u672c\u5730 ansible backend',
  'Forward runtime NodeX base URL': '\u8f6c\u53d1\u8fd0\u884c\u65f6 NodeX Base URL',
  'Forward runtime NodeX token': '\u8f6c\u53d1\u8fd0\u884c\u65f6 NodeX Token',
  'Forward runtime NodeX timeout': '\u8f6c\u53d1\u8fd0\u884c\u65f6 NodeX \u8d85\u65f6',
  'Forward runtime ansible config': '\u8f6c\u53d1\u8fd0\u884c\u65f6 ansible \u914d\u7f6e',
  'Forward ansible inventory path': '\u8f6c\u53d1 ansible inventory \u8def\u5f84',
  'Forward ansible apply playbook path': '\u8f6c\u53d1 ansible \u4e0b\u53d1 playbook \u8def\u5f84',
  'Forward ansible remove playbook path': '\u8f6c\u53d1 ansible \u79fb\u9664 playbook \u8def\u5f84',
  'Forward ansible become flag': '\u8f6c\u53d1 ansible become \u6807\u8bb0',
  'Forward ansible extra vars JSON': '\u8f6c\u53d1 ansible extra vars JSON'
}

export default {
  ...runtimePages,
  ...networkPages,
  ...miscPages,
  ...adminSupportPages,
  ...adminDashboard,
  ...adminMonitor,
  ...adminTrafficHourly,
  common: {
    locale: {
      label: '语言',
      switch: '切换语言',
      zhCN: '简体中文',
      en: 'English',
      zhShort: '中',
      enShort: 'EN'
    },
    theme: {
      switchToDark: '深色模式',
      switchToLight: '浅色模式'
    },
    a11y: {
      skipToContent: '跳转到主内容',
      openNavigation: '打开导航菜单',
      closeNavigation: '关闭导航菜单'
    },
    actions: {
      cancel: '取消',
      close: '关闭',
      copy: '复制',
      download: '下载',
      preview: '预览',
      refresh: '刷新',
      save: '保存',
      submit: '提交',
      create: '创建',
      edit: '编辑',
      delete: '删除',
      logout: '退出登录',
      details: '详情',
      back: '返回',
      buyNow: '立即选购',
      verify: '验证',
      remove: '移除',
      payNow: '去支付'
    },
    states: {
      loading: '加载中...',
      active: '有效',
      expired: '已过期',
      pending: '待支付',
      paid: '已支付',
      cancelled: '已取消',
      completed: '已完成',
      discounted: '已折抵',
      unknown: '未知',
      permanent: '永久',
      open: '待处理',
      answered: '已回复',
      closed: '已关闭',
      none: '未订阅'
    },
    labels: {
      email: '邮箱',
      password: '密码',
      confirmPassword: '确认密码',
      expiresAt: '到期时间',
      createdAt: '创建时间',
      updatedAt: '更新时间',
      subject: '主题',
      priority: '优先级',
      message: '内容',
      period: '周期',
      plan: '套餐',
      price: '价格',
      status: '状态'
    },
    ticketPriority: {
      low: '低（一般建议）',
      medium: '中（使用遇到困难）',
      high: '高（无法使用/紧急故障）'
    },
    periods: {
      month: '月付',
      quarter: '季付',
      halfYear: '半年付',
      year: '年付',
      twoYear: '两年付',
      threeYear: '三年付',
      onetime: '一次性'
    },
    messages: {
      copySuccess: '已复制到剪贴板',
      copyFailed: '复制失败',
      loadFailed: '加载失败',
      submitFailed: '提交失败',
      invalidCoupon: '无效的优惠码',
      paymentPending: '支付功能正在集成中，敬请期待',
      closeTicketConfirm: '确定要关闭此工单吗？'
    }
  },
  pageTitles: {
    auth: {
      login: '登录'
    },
    user: {
      dashboard: '仪表盘',
      subscribe: '订阅',
      knowledge: '使用教程',
      tickets: '我的工单',
      plans: '购买套餐',
      orders: '我的订单',
      fallback: '用户中心'
    },
    admin: {
      dashboard: '仪表盘',
      monitor: '实时监控',
      trafficHourly: '小时流量统计',
      users: '用户管理',
      nodes: '节点管理',
      subscriptions: '订阅管理',
      orders: '订单管理',
      plans: '套餐管理',
      tickets: '工单管理',
      coupons: '优惠券管理',
      knowledge: '知识库管理',
      forward: '流量转发管理',
      forwardTunnel: '隧道管理',
      forwardLimit: '限速管理',
      forwardAnsibleMachines: 'Ansible 机器',
      forwardNodes: 'NodeX 拓扑',
      forwardLocal: '本地运行时',
      forwardNodeX: 'NodeX 运行时',
      forwardAgents: 'NodeX Agents',
      control: '控制内核',
      payment: '支付网关管理',
      telegram: 'Telegram Bot 管理',
      mfa: 'MFA 设置',
      notifications: '通知管理',
      invite: '邀请返利管理',
      system: '系统管理',
      fallback: '管理面板'
    }
  },
  app: {
    meta: {
      defaultDescription: `${CONTROL_NAME} 统一管理订阅交付、支付账单、节点编排、Agent 运行时与转发运维。`,
      loginDescription: `登录 ${CONTROL_NAME}，继续管理订阅、账单、节点与转发运行时。`,
      userDescription: `${CONTROL_NAME} 用户中心，查看订阅、工单、套餐与账单记录。`,
      adminDescription: `${CONTROL_NAME} 管理后台，用于用户、账单、节点、通知与系统运维。`,
      forwardDescription: `${CONTROL_NAME} 转发运维工作台，覆盖 Local Runtime、Ansible Machines、拓扑、隧道与运行时诊断。`
    }
  },
  layout: {
    admin: {
      brand: CONTROL_NAME,
      mobileTitle: `${CONTROL_NAME} 管理台`,
      badge: '工作室管理台',
      subtitle: '订阅交付、节点编排、转发套件与运维入口统一收敛在此导航。',
      adminUser: '管理员',
      sections: {
        overview: '概览',
        forwardSuite: '转发套件',
        userManagement: '用户管理',
        nodeManagement: '节点管理',
        marketing: '营销管理',
        finance: '财务',
        notifications: '通知',
        content: '内容管理',
        extensions: '扩展',
        system: '系统'
      },
      nav: {
        dashboard: '仪表盘',
        monitor: '实时监控',
        trafficHourly: '小时流量',
        users: '用户管理',
        orders: '订单管理',
        tickets: '工单管理',
        nodes: '节点管理',
        subscriptions: '订阅管理',
        plans: '套餐管理',
        coupons: '优惠券管理',
        invite: '邀请返利管理',
        payment: '支付网关',
        telegram: 'Telegram Bot',
        notifications: '通知管理',
        knowledge: '知识库',
        mfa: 'MFA 设置',
        control: '控制内核',
        system: '系统管理',
        nodeXAgentsLegacy: 'NodeX Agents Legacy'
      }
    },
    user: {
      brand: CONTROL_NAME,
      nav: {
        dashboard: '仪表盘',
        subscribe: '订阅',
        knowledge: '使用教程',
        tickets: '我的工单',
        plans: '购买套餐',
        orders: '我的订单'
      }
    }
  },
  forwardSuite: {
    nav: {
      setupWizard: '快速配置向导',
      forwards: '流量转发',
      tunnels: '隧道管理',
      limits: '限速管理',
      ansibleMachines: 'Ansible 机器',
      localRuntime: '本地运行时',
      nodeXTopology: 'NodeX 拓扑',
      nodeXRuntime: 'NodeX 运行时',
      nodeXAgents: 'NodeX Agents',
      observability: '可观测性',
      more: '更多'
    },
    hints: {
      setupWizard: '一步步配置节点、隧道和转发',
      ansibleMachines: '无状态执行机器',
      localRuntime: '无状态面板宿主执行器',
      nodeXTopology: '有状态 relay/exit 拓扑',
      nodeXRuntime: '有状态 gost 控制面',
      nodeXAgents: '有状态 agent 任务通道',
      observability: '网络拓扑与延迟指标',
      more: '高级运行时工具'
    }
  },
  forwardWizard: {
    title: '转发配置向导',
    subtitle: '按步骤创建节点、隧道和转发,不用在多个页面之间来回跳转',
    loading: '正在加载运行模式…',
    shared: {
      existingLabel: '已有可复用的记录',
      useExisting: '使用现有'
    },
    steps: {
      mode: {
        title: '第一步:选择转发方式',
        intro: '选一种转发方式,系统会自动配置好对应的运行时后端和隧道类型,无需分别理解这两个概念。',
        cards: {
          local: {
            label: '本地端口转发',
            description: '直接在 Ansible 管理的机器上转发,无需中转节点。'
          },
          gostSingle: {
            label: '中转 · 单节点转发',
            description: '通过一个 NodeX 节点转发,不做协议封装。'
          },
          gostTunnel: {
            label: '中转 · 隧道转发',
            description: '跨入口/出口两个节点转发,支持协议隐藏(tls/ws/grpc 等)。'
          }
        },
        currentBadge: '当前生效',
        nodeXSetupHint: '首次使用中转转发,需要先填写 NodeX 控制面信息才能继续。',
        confirmAndContinue: '确认并继续'
      },
      machine: {
        title: '第二步:机器/节点',
        intro: '先注册一台执行机器或节点,后面的隧道会用到它。',
        createAndContinue: '创建并继续'
      },
      node: {
        title: '第二步:机器/节点',
        intro: '先注册一个 NodeX 节点,后面的隧道会用到它。',
        createAndContinue: '创建并继续'
      },
      tunnel: {
        title: '第三步:隧道',
        intro: '基于上一步的节点创建隧道,隧道是转发条目的必选项。',
        inheritedNodeHint: '已自动带入上一步创建的节点,无需重复选择。',
        createAndContinue: '创建并继续'
      },
      forward: {
        title: '第四步:转发',
        intro: '基于上一步的隧道创建实际的转发条目,填好目标地址即可生效。',
        inheritedTunnelHint: '已自动带入上一步创建的隧道,无需重复选择。',
        createAndFinish: '创建并完成'
      },
      done: {
        title: '完成',
        summary: '转发「{name}」已创建成功,链路已打通。',
        gotoForward: '前往流量转发管理',
        gotoTunnel: '前往隧道管理',
        gotoNode: '前往机器/节点管理',
        createAnother: '再建一条转发'
      }
    }
  },
  observability: {
    title: '转发可观测性',
    subtitle: '延迟趋势、网络拓扑与运行时作业时间线',
    refresh: '刷新',
    tabs: {
      trend: '延迟趋势',
      topology: '拓扑图',
      multiIngress: '多入口',
      jobs: '作业时间线'
    },
    trend: {
      targetLabel: '目标',
      selectTarget: '选择探测目标',
      noTargets: '暂无探测目标。转发/隧道/节点激活后将自动出现。',
      noData: '所选时间范围内无采样数据。',
      avg: '平均',
      p95: 'P95',
      min: '最小',
      max: '最大',
      loss: '丢包',
      latencyAxis: '延迟 (ms)',
      online: '在线',
      offline: '离线'
    },
    topology: {
      empty: '暂无 relay/exit 节点可展示。',
      relay: '中转',
      exit: '出口',
      proxy: '代理',
      latency: '延迟',
      load: '负载',
      forwards: '转发数',
      online: '在线',
      offline: '离线'
    },
    multiIngress: {
      forwardLabel: '转发',
      selectForward: '选择转发',
      tunnel: '隧道',
      ingress: '入口',
      ingressIp: '入口 IP',
      avgRtt: '平均 RTT (ms)',
      loss: '丢包率 (%)',
      status: '状态',
      empty: '选择一个转发以对比入口路径。',
      noRows: '该转发暂无入口数据。'
    },
    jobs: {
      empty: '暂无运行时作业。',
      action: '动作',
      backend: '后端',
      status: '状态',
      createdAt: '创建时间',
      pending: '等待中',
      running: '运行中',
      success: '成功',
      failed: '失败'
    },
    errors: {
      loadFailed: '加载可观测性数据失败'
    }
  },
  login: {
    brandSubtitle: CONTROL_NAME,
    brandDescription: '面向工作室交付的订阅、节点与转发运维控制台。',
    signInTitle: '欢迎回来',
    registerTitle: '创建账户',
    signInSubtitle: `登录 ${CONTROL_NAME}，继续管理服务。`,
    registerSubtitle: `创建 ${CONTROL_NAME} 账户，开始使用交付与运维能力。`,
    emailPlaceholder: '请输入邮箱地址',
    passwordPlaceholder: '请输入密码',
    registerPasswordPlaceholder: '请输入密码（至少 6 位）',
    confirmPasswordPlaceholder: '请再次输入密码',
    inviteCodeLabel: '邀请码（可选）',
    inviteCodePlaceholder: '如站点要求邀请码，请在此填写',
    mfaCodeLabel: '认证码',
    mfaCodePlaceholder: '输入 TOTP 或备用码',
    mfaRequired: '请输入 MFA 认证码完成登录。',
    forgotPassword: '忘记密码？',
    switchToRegister: '没有账户？去注册',
    switchToLogin: '已有账户？去登录',
    submitLogin: '登录',
    submitMFA: '验证并登录',
    submitRegister: '注册',
    loadingLogin: '登录中...',
    loadingRegister: '注册中...',
    mockMode: '开发模式',
    mockUser: '模拟用户',
    mockAdmin: '模拟管理员',
    errors: {
      emailPasswordRequired: '请输入邮箱和密码',
      passwordMin: '密码长度至少 6 位',
      passwordMismatch: '两次输入的密码不一致',
      registerFailed: '注册失败，请稍后重试',
      loginFailed: '登录失败，请检查邮箱和密码',
      mfaCodeRequired: '请输入 MFA 认证码。',
      mfaEnrollmentRequired: '登录前必须先启用 MFA，请联系站点管理员。'
    },
    success: {
      registerCompleted: '注册成功，正在跳转...'
    }
  },
  user: {
    dashboard: {
      title: '我的订阅',
      subtitle: '查看您的套餐和使用情况',
      refresh: '刷新',
      refreshing: '刷新中...',
      trafficUsage: '流量使用',
      upload: '上传',
      download: '下载',
      remainingDays: '剩余天数',
      trafficRemaining: '流量剩余',
      quickActions: {
        subscribe: '订阅管理',
        orders: '我的订单',
        tickets: '我的工单',
        knowledge: '使用教程'
      },
      cacheUpdatedAt: '数据更新于 {value}'
    },
    subscribe: {
      title: '订阅管理',
      subtitle: '管理与复制你的订阅链接，预览与下载订阅内容。',
      missingToken: '未检测到用户订阅 Token，请先登录或刷新页面。',
      infoTitle: '订阅信息',
      linksTitle: '订阅链接',
      previewTitle: '订阅预览 ({format})',
      usedTraffic: '已用流量',
      totalTraffic: '总流量',
      domainLabel: '订阅域名',
      copyLink: '复制链接',
      refreshCache: '刷新订阅缓存',
      copyContent: '复制内容',
      closePreview: '关闭',
      fetchPreviewFailed: '获取预览失败',
      noPreviewContent: '无订阅内容',
      refreshCompleted: '订阅缓存已刷新',
      formats: {
        auto: '自动（按 User-Agent）',
        v2ray: 'V2Ray（Base64）',
        clash: 'Clash（YAML）',
        stash: 'Stash（YAML）',
        egern: 'Egern（YAML）',
        surge: 'Surge',
        loon: 'Loon',
        shadowrocket: 'ShadowRocket',
        quantumultx: 'Quantumult X',
        singBox: 'Sing-box（JSON）',
        wireguard: 'WireGuard（.conf）',
        json: '原始 JSON',
        base64json: 'Base64 JSON'
      }
    },
    orders: {
      title: '我的订单',
      subtitle: '查看并维护您的历史订单记录',
      loading: '正在拉取订单记录...',
      empty: '您还没有任何订单，前往选购心仪的套餐吧。',
      buyNow: '立即选购',
      unknownPlan: '未知套餐',
      detailTitle: '订单详情',
      headers: {
        tradeNo: '订单号',
        plan: '套餐',
        period: '周期',
        amount: '金额',
        status: '状态',
        createdAt: '创建时间',
        actions: '操作'
      },
      labels: {
        tradeNo: '订单编号',
        status: '订单状态',
        plan: '套餐内容',
        period: '购买周期',
        totalAmount: '订单总额',
        discount: '优惠抵扣',
        createdAt: '创建时间',
        paidAt: '支付时间'
      }
    },
    plans: {
      title: '订阅计划',
      subtitle: '选择最适合您的流量套餐，随时开启高速网络体验。',
      loading: '正在加载精品套餐...',
      permanentBadge: '永久',
      unlimitedFeature: '多国节点全协议支持',
      trafficFeature: '{value} 流量',
      speedLimitFeature: '{value} Mbps 速率限制',
      deviceLimitFeature: '{value} 台设备同时在线',
      confirmOrder: '确认订单',
      selectedPlan: '所选套餐',
      choosePeriod: '选择支付周期',
      optionalCoupon: '使用优惠码（可选）',
      couponPlaceholder: '输入优惠码',
      couponApplied: '已应用优惠：{name} (-¥{value})',
      totalAmount: '应付总额',
      backToEdit: '返回重选',
      creatingOrder: '正在下单...'
    },
    tickets: {
      title: '我的工单',
      subtitle: '提交反馈或寻求技术支持',
      submitTicket: '提交工单',
      active: '进行中',
      resolved: '已处理',
      loading: '加载中...',
      empty: '暂无任何工单，如有疑问欢迎提交反馈。',
      submitNow: '立即提交',
      newTicketTitle: '提交新工单',
      replyPlaceholder: '回复内容...',
      closeWindow: '关闭窗口',
      closedHint: '此工单已关闭，如需进一步支持请提交新工单。',
      assistant: '客服助手',
      me: '我',
      updated: '更新',
      ticketId: '工单 #{id}'
    },
    knowledge: {
      title: '使用教程',
      subtitle: '获取最新的使用说明、公告和常见问题解答。',
      loading: '加载中...',
      empty: '暂无相关教程',
      readMore: '阅读全文 →',
      updated: '更新',
      all: '全部',
      closeAction: '我知道了'
    }
  },
  runtime: {
    shared: {
      panelConfig: '\u9762\u677f\u914d\u7f6e',
      reachability: '\u53ef\u8fde\u901a\u6027',
      runtimeReady: '\u8fd0\u884c\u65f6\u5c31\u7eea',
      executor: '\u6267\u884c\u5668',
      files: '\u6587\u4ef6',
      warnings: '\u8b66\u544a',
      powerShell: 'PowerShell',
      bash: 'Bash',
      bootstrapVerify: '\u5f15\u5bfc / \u6821\u9a8c',
      references: '\u53c2\u8003',
      doctorOutput: 'Doctor \u8f93\u51fa',
      doctorNotExecuted: '\u5c1a\u672a\u6267\u884c Doctor\u3002',
      refreshStatus: '\u5237\u65b0\u72b6\u6001',
      runDoctor: '\u8fd0\u884c Doctor',
      runningDoctor: '\u6267\u884c\u4e2d...',
      loading: '\u52a0\u8f7d\u4e2d...',
      yes: '\u662f',
      no: '\u5426',
      present: '\u5b58\u5728',
      missing: '\u7f3a\u5931',
      reachable: '\u53ef\u8fde\u901a',
      notReady: '\u672a\u5c31\u7eea',
      ready: '\u5c31\u7eea',
      unavailable: '\u4e0d\u53ef\u7528',
      pending: '\u5f85\u5904\u7406',
      running: '\u8fd0\u884c\u4e2d',
      success: '\u6210\u529f',
      failed: '\u5931\u8d25',
      unknown: '\u672a\u77e5',
      online: '\u5728\u7ebf',
      offline: '\u79bb\u7ebf',
      enabled: '\u5df2\u542f\u7528',
      disabled: '\u5df2\u7981\u7528',
      all: '\u5168\u90e8'
    },
    localRuntime: {
      heroEyebrow: '\u65e0\u72b6\u6001\u8fd0\u884c\u65f6',
      title: '\u672c\u5730\u8fd0\u884c\u65f6 / Ansible',
      heroTextPrimary: '\u8be5\u9875\u9762\u53ea\u7ba1\u7406\u9762\u677f\u4e3b\u673a\u4e0a\u7684 Ansible \u6267\u884c\u5668\u3002\u5b83\u662f\u9762\u677f\u4fa7\u8f6c\u53d1\u7684\u65e0\u72b6\u6001\u8fd0\u884c\u65f6\u8def\u5f84\uff0c\u4e0d\u9700\u8981\u6301\u7eed\u7684 NodeX \u63a7\u5236\u9762\u6216 Node-Agent \u8fde\u63a5\u3002',
      heroTextSecondary: '\u540e\u7aef\u4e3a nftables / Ansible\uff0c\u9762\u677f\u4fa7\u8f6c\u53d1\u7684\u65e0\u72b6\u6001\u63a7\u5236\u8def\u5f84\u3002',
      refreshLoading: '\u5237\u65b0\u4e2d...',
      saveLoading: '\u4fdd\u5b58\u4e2d...',
      saveActivate: '\u4fdd\u5b58\u5e76\u542f\u7528\u672c\u5730\u8fd0\u884c\u65f6',
      activeBannerTitle: '\u672c\u5730\u8fd0\u884c\u65f6\u5df2\u542f\u7528',
      standbyBannerTitle: '\u672c\u5730\u8fd0\u884c\u65f6\u5904\u4e8e\u5f85\u547d',
      activeBannerText: '\u5f53\u524d\u8f6c\u53d1\u4efb\u52a1\u4f7f\u7528 {backend}\u3002SSH \u4f20\u8f93\u548c\u63d0\u6743\u7b56\u7565\u90fd\u4ece\u8fd9\u4efd Ansible \u8fd0\u884c\u65f6\u914d\u7f6e\u89e3\u6790\u3002',
      standbyBannerText: 'NodeX/gost \u4ecd\u7136\u662f\u5168\u5c40\u6d3b\u8dc3\u8fd0\u884c\u65f6\u3002\u4f60\u4ecd\u53ef\u5148\u5728\u8fd9\u91cc\u9884\u6f14\u548c\u9a8c\u8bc1\u672c\u5730 Ansible \u8fd0\u884c\u65f6\uff0c\u518d\u5207\u6362\u56de\u53bb\u3002',
      configEyebrow: '\u914d\u7f6e',
      configTitle: '\u9762\u677f\u4e3b\u673a Ansible \u6267\u884c\u5668',
      configCopy: 'Ansible \u6a21\u5f0f\u662f\u65e0\u72b6\u6001\u7684\uff1a\u9762\u677f\u53ea\u5728 tunnel \u548c forward \u8bb0\u5f55\u4e2d\u4fdd\u5b58\u6267\u884c\u8282\u70b9\u6807\u8bc6\uff0cinventory\u3001playbook\u3001sudo \u548c SSH \u884c\u4e3a\u90fd\u5728\u8fd9\u91cc\u914d\u7f6e\u3002',
      recommended: '\u63a8\u8350',
      legacy: '\u65e7\u517c\u5bb9',
      executorEyebrow: '\u6267\u884c\u5668',
      defaultsAction: '\u4f7f\u7528\u540e\u7aef\u9ed8\u8ba4\u503c',
      executorHint: '\u4fdd\u5b58\u540e\u4f1a\u5c06 {backend} \u8bbe\u4e3a\u5f53\u524d\u672c\u5730 runtime\uff0c\u5e76\u5199\u5165 `forward.runtime_backend={backendKey}`\u3001`forward.runtime.ansible.backend={backendKey}` \u4ee5\u53ca `forward.runtime.nodex_mode=false`\u3002',
      fields: {
        inventory: 'Inventory',
        applyPlaybook: '\u4e0b\u53d1 Playbook',
        removePlaybook: '\u79fb\u9664 Playbook',
        command: '\u547d\u4ee4',
        workingDir: '\u5de5\u4f5c\u76ee\u5f55',
        targetPattern: '\u76ee\u6807\u6a21\u5f0f',
        timeoutSeconds: '\u8d85\u65f6\uff08\u79d2\uff09',
        ansibleConfig: 'ANSIBLE_CONFIG',
        useBecome: '\u5728\u6267\u884c\u8282\u70b9\u4e0a\u4f7f\u7528 sudo / become',
        extraVarsJson: '\u989d\u5916 vars JSON',
        environmentJson: '\u73af\u5883\u53d8\u91cf JSON',
        generatedJson: '\u751f\u6210\u7684\u8fd0\u884c\u65f6 JSON'
      },
      extraVarsHint: '\u540e\u7aef\u76f8\u5173\u5b57\u6bb5\uff08\u6bd4\u5982 firewall driver\uff09\u4f1a\u7531\u6240\u9009 backend \u81ea\u52a8\u6ce8\u5165\u3002',
      environmentHint: '\u9762\u677f\u4e3b\u673a\u6267\u884c\u5668\u8fdb\u7a0b\u7684\u989d\u5916\u73af\u5883\u53d8\u91cf\u3002',
      generatedHint: 'JSON \u8d1f\u8f7d\u7531\u4e0a\u9762\u7684\u7ed3\u6784\u5316\u5b57\u6bb5\u751f\u6210\uff0c\u5e76\u5b58\u5165 `forward.runtime.ansible.config`\u3002',
      probeEyebrow: '\u672c\u5730\u63a2\u6d4b',
      probeTitle: '\u6267\u884c\u5668\u53ef\u8fde\u901a\u6027\u4e0e\u8fd0\u884c\u65f6\u5c31\u7eea\u5ea6',
      loadingStatus: '\u52a0\u8f7d\u672c\u5730\u8fd0\u884c\u65f6\u72b6\u6001\u4e2d...',
      noStatus: '\u8fd8\u672a\u52a0\u8f7d\u672c\u5730\u8fd0\u884c\u65f6\u72b6\u6001\u3002',
      cards: {
        localActiveValue: '\u672c\u5730\u8fd0\u884c\u65f6\u5df2\u542f\u7528',
        standbyValue: '\u5f85\u547d\u914d\u7f6e',
        backend: '\u540e\u7aef',
        preferredLocalBackend: '\u9996\u9009\u672c\u5730 backend',
        attachment: '\u6302\u8f7d\u6a21\u5f0f',
        runtimeReady: '\u8fd0\u884c\u65f6\u5c31\u7eea',
        firewallDriver: 'Firewall driver',
        commandFound: '\u627e\u5230\u547d\u4ee4',
        become: 'Become',
        inventory: 'Inventory',
        applyPlaybook: '\u4e0b\u53d1 Playbook',
        removePlaybook: '\u79fb\u9664 Playbook',
        workingDir: '\u5de5\u4f5c\u76ee\u5f55'
      },
      jobsEyebrow: '\u8fd0\u884c\u65f6\u4efb\u52a1',
      latestJobs: '\u6700\u65b0 {backend} \u4efb\u52a1',
      jobMeta: 'forward {forwardId} / tunnel {tunnelId} / node {nodeId}',
      loadingJobs: '\u52a0\u8f7d\u8fd0\u884c\u65f6\u4efb\u52a1\u4e2d...',
      noJobs: '\u6682\u65e0\u672c\u5730\u8fd0\u884c\u65f6\u4efb\u52a1\u3002',
      backends: {
        nftables: {
          label: 'nftables / Ansible',
          description: '\u73b0\u4ee3 Linux \u4e3b\u673a\u5e94\u4f18\u5148\u4f7f\u7528 nftables\u3002'
        },
        iptables: {
          label: 'iptables / Ansible',
          description: '\u7528\u4e8e\u65e7 playbook \u7684\u517c\u5bb9\u8def\u5f84\u3002'
        }
      },
      errors: {
        savedConfigInvalid: '\u5df2\u4fdd\u5b58\u7684\u672c\u5730\u8fd0\u884c\u65f6\u914d\u7f6e\u65e0\u6548\uff0c\u5df2\u56de\u9000\u5230\u9ed8\u8ba4\u503c\uff0c\u8bf7\u91cd\u65b0\u4fdd\u5b58\u4ee5\u4fee\u590d\u3002',
        invalidJson: '{label} \u5fc5\u987b\u662f\u6709\u6548 JSON',
        invalidObject: '{label} \u5fc5\u987b\u662f JSON \u5bf9\u8c61',
        invalidPreview: '\u8fd0\u884c\u65f6\u914d\u7f6e\u65e0\u6548\uff1a{message}',
        invalidRuntimeJson: '\u672c\u5730\u8fd0\u884c\u65f6 JSON \u65e0\u6548',
        saveFailed: '\u4fdd\u5b58\u672c\u5730\u8fd0\u884c\u65f6\u914d\u7f6e\u5931\u8d25',
        fetchStatusFailed: '\u83b7\u53d6\u672c\u5730\u8fd0\u884c\u65f6\u72b6\u6001\u5931\u8d25',
        doctorFailed: '\u672c\u5730\u8fd0\u884c\u65f6 Doctor \u6267\u884c\u5931\u8d25'
      }
    },
    nodeX: {
      heroEyebrow: '\u79c1\u6709\u8fd0\u884c\u65f6',
      title: 'NodeX \u8fd0\u884c\u65f6',
      heroTextPrimary: '\u8fd9\u662f\u7ed9\u6709\u72b6\u6001 NodeX/gost \u8def\u5f84\u7684\u4e13\u7528\u64cd\u4f5c\u5165\u53e3\u3002\u5373\u4f7f\u5168\u5c40 runtime backend \u4ecd\u662f\u672c\u5730 Ansible\uff0c\u8fd9\u4e2a\u9875\u9762\u4e5f\u4f1a\u76f4\u63a5\u63a2\u6d4b\u5df2\u914d\u7f6e\u7684 NodeX \u63a7\u5236\u9762\u3002',
      heroTextSecondary: '\u672c\u5730 Ansible \u6267\u884c\u73b0\u5728\u653e\u5728 Local Runtime \u548c Ansible Machines \u4e0b\u3002\u8282\u70b9\u201c\u5728\u7ebf\u201d\u4ecd\u7136\u53ea\u8868\u793a TCP \u53ef\u8fde\u901a\uff0c\u4e0d\u4ee3\u8868 NodeX \u6216 relay gost API \u5df2\u7ecf\u6302\u8f7d\u6210\u529f\u3002',
      refreshLoading: '\u5237\u65b0\u4e2d...',
      saveLoading: '\u4fdd\u5b58\u4e2d...',
      save: '\u4fdd\u5b58 NodeX \u914d\u7f6e',
      enabledBannerTitle: 'NodeX \u6a21\u5f0f\u5df2\u542f\u7528',
      disabledBannerTitle: 'NodeX \u6a21\u5f0f\u672a\u542f\u7528',
      enabledBannerText: '\u9762\u677f\u8f6c\u53d1\u4efb\u52a1\u53ef\u4ee5\u7ecf\u7531 NodeX/gost\uff0c\u4f46\u6bcf\u4e2a runtime \u4efb\u52a1\u4ecd\u7136\u5fc5\u987b\u6210\u529f\u624d\u4ee3\u8868 relay \u771f\u6b63\u6302\u8f7d\u5b8c\u6210\u3002',
      disabledBannerText: '\u4f60\u53ef\u4ee5\u5148\u5728\u8fd9\u91cc\u9a8c\u8bc1\u5df2\u914d\u7f6e\u7684 NodeX \u63a7\u5236\u9762\uff0c\u51c6\u5907\u597d\u540e\u518d\u5207\u6362\u5168\u5c40 backend\u3002',
      configEyebrow: '\u914d\u7f6e',
      configTitle: 'NodeX \u63a7\u5236\u9762',
      enableModeTitle: '\u542f\u7528 NodeX \u6a21\u5f0f',
      enableModeHint: '\u4f1a\u5199\u5165 `forward.runtime.nodex_mode=true` \u548c `forward.runtime_backend=gost`\u3002',
      fields: {
        baseUrl: 'NodeX Base URL',
        baseUrlHint: '\u8fd9\u91cc\u5fc5\u987b\u6307\u5411 NodeX \u63a7\u5236\u9762\uff0c\u4e0d\u662f relay gost API \u672c\u8eab\u3002',
        token: 'NodeX Token',
        tokenHint: '\u9700\u4e0e NodeX \u63a7\u5236\u9762 `--forward-api-token` \u7684\u503c\u4e00\u81f4\u3002',
        timeout: '\u8d85\u65f6\uff08\u79d2\uff09',
        timeoutHint: '\u9762\u677f\u63a2\u6d4b\u6216\u6267\u884c NodeX runtime \u8bf7\u6c42\u65f6\u4f1a\u4f7f\u7528\u8be5\u8d85\u65f6\u503c\u3002'
      },
      probeEyebrow: 'NodeX \u63a2\u6d4b',
      probeTitle: '\u5065\u5eb7\u5ea6\u4e0e\u8fd0\u884c\u65f6\u72b6\u6001',
      probeCopy: '\u8fd9\u4e9b\u68c0\u67e5\u603b\u662f\u76f4\u63a5\u6307\u5411\u5df2\u914d\u7f6e\u7684 NodeX \u63a7\u5236\u9762\uff0c\u4e0d\u4f9d\u8d56\u5f53\u524d\u5168\u5c40 runtime backend\u3002',
      loadingStatus: '\u52a0\u8f7d NodeX \u8fd0\u884c\u65f6\u72b6\u6001\u4e2d...',
      noStatus: '\u8fd8\u672a\u52a0\u8f7d NodeX \u8fd0\u884c\u65f6\u72b6\u6001\u3002',
      cards: {
        modeOn: 'NodeX \u6a21\u5f0f\u5df2\u5f00',
        modeOff: 'NodeX \u6a21\u5f0f\u5df2\u5173',
        backend: '\u540e\u7aef',
        baseUrl: 'Base URL',
        tokenConfigured: 'Token \u5df2\u914d\u7f6e',
        timeout: '\u8d85\u65f6',
        health: '\u5065\u5eb7\u68c0\u67e5',
        http: 'HTTP',
        version: '\u7248\u672c',
        executePath: '\u6267\u884c\u8def\u5f84'
      },
      jobsEyebrow: '\u8fd0\u884c\u65f6\u4efb\u52a1',
      jobsTitle: '\u6700\u65b0 gost \u4efb\u52a1',
      jobsCopy: '\u6700\u8fd1\u7684\u9762\u677f\u4fa7 runtime \u5ba1\u8ba1\u8bb0\u5f55\uff0c\u5df2\u6309 `gost` backend \u8fc7\u6ee4\u3002',
      jobMeta: 'forward {forwardId} / tunnel {tunnelId} / node {nodeId}',
      loadingJobs: '\u52a0\u8f7d\u8fd0\u884c\u65f6\u4efb\u52a1\u4e2d...',
      noJobs: '\u6682\u65e0 gost \u8fd0\u884c\u65f6\u4efb\u52a1\u3002',
      backends: {
        gost: 'gost / NodeX'
      },
      errors: {
        baseUrlRequired: '\u542f\u7528 NodeX \u6a21\u5f0f\u65f6\u5fc5\u987b\u586b\u5199 NodeX Base URL',
        tokenRequired: '\u542f\u7528 NodeX \u6a21\u5f0f\u65f6\u5fc5\u987b\u586b\u5199 NodeX Token',
        saveFailed: '\u4fdd\u5b58 NodeX \u914d\u7f6e\u5931\u8d25',
        fetchStatusFailed: '\u83b7\u53d6 NodeX \u8fd0\u884c\u65f6\u72b6\u6001\u5931\u8d25',
        doctorFailed: 'NodeX runtime Doctor \u6267\u884c\u5931\u8d25'
      }
    },
    ansibleMachines: {
      heroEyebrow: '\u6267\u884c\u673a\u7fa4',
      title: 'Ansible \u673a\u5668',
      heroText: '\u8fd9\u4e2a\u9875\u9762\u53ea\u7528\u4e8e\u65e0\u72b6\u6001 Ansible \u6267\u884c\u673a\u5668\u3002\u8fd9\u4e9b\u4e3b\u673a\u4e0d\u9700\u8981 Node-Agent\uff0c\u4e5f\u4e0d\u9700\u8981\u6301\u7eed\u63a7\u5236\u9762\u8fde\u63a5\u3002',
      refreshLoading: '\u5237\u65b0\u4e2d...',
      addMachine: '\u6dfb\u52a0\u673a\u5668',
      stats: {
        machines: '\u673a\u5668',
        online: '\u5728\u7ebf',
        enabled: '\u5df2\u542f\u7528'
      },
      sectionEyebrow: '\u673a\u5668',
      sectionTitle: '\u6267\u884c\u76ee\u6807',
      sectionCopy: '\u8fd9\u4e9b\u8bb0\u5f55\u53ea\u7528\u4e8e\u672c\u5730 Ansible \u8fd0\u884c\u65f6\u8bc6\u522b\u6267\u884c\u76ee\u6807\u4e3b\u673a\uff0c\u4e0d\u5c5e\u4e8e NodeX \u63a7\u5236\u9762\u8282\u70b9\u3002',
      inventoryHint: 'SSH \u7528\u6237\u540d\u3001\u5bc6\u7801\u548c\u79c1\u94a5\u4e0d\u5728\u6b64\u9875\u9762\u4fdd\u5b58\uff0c\u8bf7\u5728 Ansible inventory\u3001playbook \u6216 Local Runtime \u73af\u5883\u914d\u7f6e\u4e2d\u63d0\u4f9b\u3002',
      filterLabel: '\u72b6\u6001',
      filters: {
        all: '\u5168\u90e8',
        online: '\u5728\u7ebf',
        offline: '\u79bb\u7ebf'
      },
      loading: '\u52a0\u8f7d Ansible \u673a\u5668\u4e2d...',
      empty: '\u6682\u65e0 Ansible \u6267\u884c\u673a\u5668\u3002',
      machineEyebrow: '\u673a\u5668 #{id}',
      meta: {
        authSource: '\u51ed\u636e\u6765\u6e90',
        authSourceValue: 'Inventory / Local Runtime',
        regionIsp: '\u533a\u57df / ISP',
        currentConn: '\u5f53\u524d\u8fde\u63a5',
        traffic: '\u6d41\u91cf'
      },
      actions: {
        edit: '\u7f16\u8f91',
        check: '\u5065\u5eb7\u68c0\u67e5',
        checking: '\u68c0\u67e5\u4e2d...',
        sync: '\u540c\u6b65\u7edf\u8ba1',
        syncing: '\u540c\u6b65\u4e2d...',
        disable: '\u7981\u7528',
        enable: '\u542f\u7528',
        delete: '\u5220\u9664'
      },
      modal: {
        eyebrow: '\u673a\u5668',
        titleEdit: '\u7f16\u8f91 Ansible \u673a\u5668',
        titleAdd: '\u6dfb\u52a0 Ansible \u673a\u5668',
        deleteEyebrow: '\u5220\u9664',
        deleteTitle: '\u5220\u9664\u673a\u5668',
        deleteConfirm: '\u786e\u5b9a\u5c06 {name} \u4ece Ansible \u6267\u884c\u673a\u7fa4\u4e2d\u5220\u9664\u5417\uff1f',
        saveLoading: '\u4fdd\u5b58\u4e2d...',
        save: '\u4fdd\u5b58',
        cancel: '\u53d6\u6d88',
        deleteLoading: '\u5220\u9664\u4e2d...'
      },
      fields: {
        name: '\u540d\u79f0',
        host: '\u4e3b\u673a',
        reachabilityPort: '\u8fde\u901a\u6027\u7aef\u53e3',
        weight: '\u6743\u91cd',
        region: '\u533a\u57df',
        isp: 'ISP'
      },
      placeholders: {
        name: 'relay-exec-01',
        host: '1.2.3.4',
        region: 'HK / JP / US',
        isp: 'CMI / NTT / Cogent'
      },
      results: {
        latency: '\u5ef6\u8fdf {value} ms',
        reachable: '\u673a\u5668\u53ef\u8fde\u901a',
        unavailable: '\u673a\u5668\u4e0d\u53ef\u7528',
        synced: '\u7edf\u8ba1\u5df2\u540c\u6b65'
      },
      errors: {
        required: '\u540d\u79f0\u3001\u4e3b\u673a\u548c\u8fde\u901a\u6027\u7aef\u53e3\u4e3a\u5fc5\u586b\u9879\u3002',
        saveFailed: '\u4fdd\u5b58\u673a\u5668\u5931\u8d25',
        loadFailed: '\u52a0\u8f7d Ansible \u673a\u5668\u5931\u8d25',
        detailFailed: '\u52a0\u8f7d\u673a\u5668\u8be6\u60c5\u5931\u8d25',
        deleteFailed: '\u5220\u9664\u673a\u5668\u5931\u8d25',
        checkFailed: '\u5065\u5eb7\u68c0\u67e5\u5931\u8d25',
        syncFailed: '\u540c\u6b65\u673a\u5668\u7edf\u8ba1\u5931\u8d25',
        toggleFailed: '\u5207\u6362\u673a\u5668\u72b6\u6001\u5931\u8d25'
      }
    },
    forward: {
      heroEyebrow: 'Flux Compatible',
      title: '流量转发管理',
      note: 'NodeX \u6a21\u5f0f\u4fdd\u7559 ingress/exit \u8bed\u4e49\uff1b\u672c\u5730 Ansible \u6a21\u5f0f\u53ea\u9762\u5411\u7531 inventory \u89e3\u6790\u51fa\u6765\u7684\u6267\u884c\u8282\u70b9\u3002\u8f6c\u53d1\u8282\u70b9\u201c\u5728\u7ebf\u201d\u53ea\u8868\u793a TCP \u53ef\u8fde\u901a\uff0c\u4e0d\u80fd\u8bc1\u660e\u8fdc\u7a0b\u6302\u8f7d\u6216\u9632\u706b\u5899\u72b6\u6001\u5df2\u5b8c\u6210\u3002',
      modeLabelNodeX: '\u5f53\u524d\u8fd0\u884c\u65f6\uff1aNodeX / gost',
      modeLabelLocal: '\u5f53\u524d\u8fd0\u884c\u65f6\uff1aLocal / {backend}',
      modeSummaryNodeX: '\u8bf7\u5728\u4e13\u7528\u7684 NodeX Runtime \u9875\u9762\u7f16\u8f91 NodeX \u63a7\u5236\u9762 URL\u3001Token \u548c gost \u64cd\u4f5c\u68c0\u67e5\u3002',
      modeSummaryLocal: '\u8bf7\u5728\u4e13\u7528\u7684 Local Runtime \u9875\u9762\u7f16\u8f91 inventory\u3001playbook \u548c\u9762\u677f\u4e3b\u673a\u6267\u884c\u5668\u914d\u7f6e\u3002',
      modeCompatibilityHint: '\u8f6c\u53d1\u7f16\u8f91\u5668\u4f1a\u6309\u5f53\u524d runtime \u81ea\u52a8\u8fc7\u6ee4\u53ef\u9009 tunnel\u3002\u672c\u5730 Ansible runtime \u53ea\u63a5\u53d7 Port Forward \u96a7\u9053\uff0cNodeX/gost \u5219\u53ef\u4ee5\u9644\u7740\u517c\u5bb9\u7684 Port Forward \u548c Tunnel Forward \u5e03\u5c40\u3002',
      modeHintNodeX: 'NodeX/gost \u6a21\u5f0f\u4fdd\u7559 ingress \u548c exit \u8bed\u4e49\u3002\u5373\u4f7f\u5df2\u9009\u62e9 tunnel\uff0c\u4e5f\u4ecd\u9700 NodeX runtime \u4efb\u52a1\u6267\u884c\u6210\u529f\uff0c\u8f6c\u53d1\u624d\u7b97\u771f\u6b63\u6302\u8f7d\u3002',
      modeHintLocal: '\u672c\u5730 Ansible \u6a21\u5f0f\u53ea\u8bb0\u5f55\u6267\u884c\u8282\u70b9\u3002SSH \u8bbf\u95ee\u4f9d\u8d56\u5df2\u914d\u7f6e\u7684 ansible inventory \u548c local runtime \u53c2\u6570\uff0c\u4e0d\u6765\u81ea NodeX \u62d3\u6251\u8bb0\u5f55\u3002',
      tunnelHintNodeX: '{name} \u5c06\u901a\u8fc7 NodeX/gost \u6302\u8f7d\u3002\u9762\u677f\u4fa7\u201c\u5728\u7ebf\u201d\u6216\u72b6\u6001\u68c0\u67e5\u4e0d\u80fd\u8bc1\u660e\u8fdc\u7a0b relay \u5df2\u5b8c\u6210\u6302\u8f7d\u3002',
      tunnelHintLocal: '{name} \u53ea\u4f1a\u5728\u6267\u884c\u8282\u70b9\u4e0a\u88ab\u5e94\u7528\u3002\u8be5\u8def\u5f84\u4fdd\u6301\u65e0\u72b6\u6001\uff0c\u76f4\u5230\u6392\u961f\u7684 ansible \u4efb\u52a1\u6210\u529f\u7ed3\u675f\u3002',
      tunnelHintLocalIncompatible: '{name} \u662f Tunnel Forward \u96a7\u9053\uff0c\u53ea\u80fd\u7531 NodeX/gost \u6302\u8f7d\uff0c\u672c\u5730 Ansible runtime \u4e0d\u80fd\u76f4\u63a5\u9644\u7740\u5b83\u3002',
      portRange: '\u53ef\u7528\u7aef\u53e3\u8303\u56f4\uff1a{start} - {end}',
      portHintNodeX: '\u7aef\u53e3\u7559\u7a7a\u65f6\uff0c\u9762\u677f\u4f1a\u4ece tunnel \u5165\u53e3\u8282\u70b9\u7aef\u53e3\u6bb5\u4e2d\u81ea\u52a8\u5206\u914d\u3002',
      portHintLocal: '\u7aef\u53e3\u7559\u7a7a\u65f6\uff0c\u9762\u677f\u4f1a\u5728\u6240\u9009\u6267\u884c\u8282\u70b9\u4e0a\u81ea\u52a8\u5206\u914d\u3002',
      loading: '\u6b63\u5728\u52a0\u8f7d\u8f6c\u53d1\u4e0e\u96a7\u9053\u6570\u636e...',
      view: {
        switchToDirectTitle: '\u5207\u6362\u5230\u76f4\u8fde\u89c6\u56fe',
        switchToGroupedTitle: '\u5207\u6362\u5230\u5206\u7ec4\u89c6\u56fe',
        directShort: '\u76f4',
        groupedShort: '\u7ec4',
        directLabel: '\u76f4\u8fde',
        groupedLabel: '\u5206\u7ec4'
      },
      actions: {
        import: '\u5bfc\u5165',
        export: '\u5bfc\u51fa',
        add: '\u65b0\u589e',
        edit: '\u7f16\u8f91',
        diagnose: '\u8bca\u65ad',
        delete: '\u5220\u9664',
        copyAll: '\u590d\u5236\u5168\u90e8',
        regenerate: '\u91cd\u65b0\u751f\u6210',
        generateExport: '\u751f\u6210\u5bfc\u51fa\u6570\u636e',
        startImport: '\u5f00\u59cb\u5bfc\u5165',
        rerunDiagnosis: '\u91cd\u65b0\u8bca\u65ad'
      },
      bulk: {
        selected: '已选择 {count} 条',
        selectAll: '选择全部转发',
        selectRule: '选择 {name}',
        resume: '恢复',
        pause: '暂停',
        export: '导出',
        delete: '删除',
        clear: '清空'
      },
      filters: {
        search: '搜索',
        searchPlaceholder: '规则、隧道、用户、地址或端口',
        tunnel: '隧道',
        allTunnels: '全部隧道',
        status: '状态',
        allStatuses: '全部状态',
        running: '运行中',
        paused: '已暂停',
        error: '错误',
        clear: '重置',
        emptyTitle: '没有匹配的转发',
        emptyText: '调整搜索、隧道或状态筛选后查看更多规则。'
      },
      table: {
        rule: '\u89c4\u5219 / \u96a7\u9053',
        ingress: '\u5165\u53e3',
        target: '\u76ee\u6807',
        policy: '\u7b56\u7565',
        status: '\u72b6\u6001',
        traffic: '\u6d41\u91cf',
        actions: '\u64cd\u4f5c'
      },
      group: {
        eyebrow: 'User',
        userTag: '\u7528\u6237',
        summary: '{tunnels} \u4e2a\u96a7\u9053\uff0c{forwards} \u4e2a\u8f6c\u53d1',
        tunnelMeta: 'Tunnel #{id}'
      },
      emptyGroupedTitle: '\u6682\u65e0\u8f6c\u53d1\u914d\u7f6e',
      emptyGroupedText: '\u5f53\u524d\u7cfb\u7edf\u91cc\u8fd8\u6ca1\u6709\u4efb\u4f55\u517c\u5bb9 flux-panel \u7684\u8f6c\u53d1\u8bb0\u5f55\u3002',
      emptyDirectTitle: '\u6682\u65e0\u8f6c\u53d1\u914d\u7f6e',
      emptyDirectText: '\u521b\u5efa\u7b2c\u4e00\u6761\u8f6c\u53d1\u540e\uff0c\u8fd9\u91cc\u4f1a\u663e\u793a\u5f53\u524d\u8f6c\u53d1\u7684\u76f4\u8fde\u8868\u683c\u89c6\u56fe\u3002',
      editor: {
        eyebrow: 'Forward',
        titleEdit: '\u7f16\u8f91\u8f6c\u53d1',
        titleAdd: '\u65b0\u589e\u8f6c\u53d1',
        fields: {
          name: '\u8f6c\u53d1\u540d\u79f0',
          tunnel: '\u5173\u8054\u96a7\u9053',
          ingressPort: '\u5165\u53e3\u7aef\u53e3',
          interfaceName: '\u7f51\u5361\u540d\u79f0',
          remoteAddress: '\u76ee\u6807\u5730\u5740',
          strategy: '\u8c03\u5ea6\u7b56\u7565'
        },
        placeholders: {
          name: '\u4f8b\u5982\uff1aHK-Web-01',
          tunnel: '\u8bf7\u9009\u62e9\u96a7\u9053',
          ingressPort: '\u7559\u7a7a\u81ea\u52a8\u5206\u914d',
          interfaceName: '\u53ef\u9009\uff0c\u4f8b\u5982 eth0',
          remoteAddress: '\u6bcf\u884c\u4e00\u4e2a\u76ee\u6807\uff0c\u4f8b\u5982\uff1a\n1.1.1.1:443\nexample.com:8443\n[2001:db8::1]:443'
        },
        remoteHint: '\u652f\u6301 IPv4:port\u3001domain:port\u3001[\u5b8c\u6574 IPv6]:port\u3002\u591a\u5730\u5740\u8bf7\u6bcf\u884c\u4e00\u4e2a\u3002',
        submitLoading: '\u63d0\u4ea4\u4e2d...',
        submitUpdate: '\u4fdd\u5b58\u4fee\u6539',
        submitCreate: '\u521b\u5efa\u8f6c\u53d1'
      },
      deleteModal: {
        eyebrow: 'Delete',
        title: '\u5220\u9664\u8f6c\u53d1',
        confirmText: '\u786e\u8ba4\u5220\u9664 {name} \u5417\uff1f',
        hint: '\u5e38\u89c4\u5220\u9664\u5931\u8d25\u65f6\uff0c\u4f1a\u7ee7\u7eed\u7ed9\u51fa\u5f3a\u5236\u5220\u9664\u786e\u8ba4\u3002',
        deleteLoading: '\u5220\u9664\u4e2d...',
        confirmDelete: '\u786e\u8ba4\u5220\u9664',
        forceDeleteIntro: '\u5e38\u89c4\u5220\u9664\u5931\u8d25\uff1a{message}',
        forceDeleteQuestion: '\u662f\u5426\u9700\u8981\u5f3a\u5236\u5220\u9664\uff1f',
        forceDeleteWarning: '\u8b66\u544a\uff1a\u5f3a\u5236\u5220\u9664\u4e0d\u4f1a\u9a8c\u8bc1\u8282\u70b9\u7aef\u662f\u5426\u5df2\u5220\u9664\u5bf9\u5e94\u7684\u8f6c\u53d1\u670d\u52a1\u3002'
      },
      addressModal: {
        eyebrow: 'Address',
        copy: '\u590d\u5236',
        copying: '\u590d\u5236\u4e2d...',
        titleWithCount: '{title} ({count})'
      },
      exportModal: {
        eyebrow: 'Export',
        title: '\u5bfc\u51fa\u8f6c\u53d1\u6570\u636e',
        subtitle: '格式：兼容 relay-panel 的 JSON：[{ "dest": ["host:port"], "listen_port": 10086, "name": "规则" }]',
        tunnelLabel: '\u9009\u62e9\u5bfc\u51fa\u96a7\u9053',
        tunnelPlaceholder: '\u8bf7\u9009\u62e9\u96a7\u9053',
        generating: '\u751f\u6210\u4e2d...',
        regenerate: '\u91cd\u65b0\u751f\u6210',
        generate: '\u751f\u6210\u5bfc\u51fa\u6570\u636e',
        noDataPlaceholder: '\u6682\u65e0\u5bfc\u51fa\u6570\u636e',
        selectionHint: '正在导出已选择的 {count} 条转发。'
      },
      importModal: {
        eyebrow: 'Import',
        title: '\u5bfc\u5165\u8f6c\u53d1\u6570\u636e',
        subtitle: '支持 relay-panel JSON 和旧格式 remoteAddr|name|inPort，inPort 可留空。',
        subtitleSecondary: 'JSON 示例：[{ "dest": ["3.3.3.3:3", "4.4.4.4:4"], "listen_port": 10086, "name": "业务入口" }]',
        tunnelLabel: '\u9009\u62e9\u5bfc\u5165\u96a7\u9053',
        tunnelPlaceholder: '\u8bf7\u9009\u62e9\u96a7\u9053',
        dataLabel: '\u5bfc\u5165\u6570\u636e',
        placeholder: '[{"dest":["example.com:8080"],"listen_port":10086,"name":"业务入口"}]',
        resultTitle: '\u5bfc\u5165\u7ed3\u679c',
        resultSummary: '\u6210\u529f\uff1a{success} / \u603b\u8ba1\uff1a{total}',
        statusSuccess: '\u6210\u529f',
        statusFailed: '\u5931\u8d25',
        importing: '\u5bfc\u5165\u4e2d...',
        startImport: '\u5f00\u59cb\u5bfc\u5165'
      },
      diagnosis: {
        eyebrow: 'Diagnosis',
        title: '\u8f6c\u53d1\u8bca\u65ad\u7ed3\u679c',
        loading: '\u6b63\u5728\u8bca\u65ad\u8f6c\u53d1\u8fde\u63a5...',
        connectionSuccess: '\u8fde\u63a5\u6210\u529f',
        connectionFailed: '\u8fde\u63a5\u5931\u8d25',
        nodeMeta: '{name} · {node}',
        nodeMeta: '{name} · {node}',
        nodeMeta: '{name} / {node}',
        targetAddress: '\u76ee\u6807\u5730\u5740',
        averageLatency: '\u5e73\u5747\u5ef6\u8fdf',
        packetLoss: '\u4e22\u5305\u7387',
        quality: '\u8d28\u91cf',
        failedFallback: '\u8bca\u65ad\u5931\u8d25',
        emptyTitle: '\u6682\u65e0\u8bca\u65ad\u6570\u636e',
        emptyText: '\u53d1\u8d77\u4e00\u6b21\u8bca\u65ad\u540e\uff0c\u8fd9\u91cc\u4f1a\u5c55\u793a\u4e0e\u53c2\u8003\u9875\u4e00\u81f4\u7684\u7ed3\u679c\u5361\u7247\u3002',
        rerunning: '\u8bca\u65ad\u4e2d...',
        rerun: '\u91cd\u65b0\u8bca\u65ad'
      },
      status: {
        normal: '\u6b63\u5e38',
        paused: '\u6682\u505c',
        error: '\u5f02\u5e38',
        unknown: '\u672a\u77e5'
      },
      runtimeStatus: {
        pending: '\u5f85\u4e0b\u53d1',
        running: '\u6267\u884c\u4e2d',
        synced: '\u5df2\u540c\u6b65',
        applied: '\u5df2\u5e94\u7528',
        failed: '\u540c\u6b65\u5931\u8d25',
        queuedSummary: '\u8fd0\u884c\u65f6\u4efb\u52a1\u5df2\u5165\u961f\uff0c\u7b49\u5f85\u6267\u884c\u5668\u5b8c\u6210\u3002',
        runningSummary: '\u8fd0\u884c\u65f6\u4efb\u52a1\u6b63\u5728\u6267\u884c\u3002'
      },
      strategy: {
        fifo: '\u4e3b\u5907',
        round: '\u8f6e\u8be2',
        rand: '\u968f\u673a',
        hash: 'Hash',
        unknown: '\u672a\u77e5'
      },
      quality: {
        unknown: '\u672a\u77e5',
        excellent: '\u4f18\u79c0',
        veryGood: '\u5f88\u597d',
        good: '\u826f\u597d',
        fair: '\u4e00\u822c',
        poor: '\u8f83\u5dee',
        veryPoor: '\u5f88\u5dee'
      },
      labels: {
        inbound: '\u5165',
        outbound: '\u51fa'
      },
      references: {
        tunnel: '\u96a7\u9053 #{id}',
        node: '\u8282\u70b9 {id}',
        node: '\u8282\u70b9 #{id}'
      },
      messages: {
        loadForwardsFailed: '\u83b7\u53d6\u8f6c\u53d1\u5217\u8868\u5931\u8d25',
        loadTunnelsFailed: '\u83b7\u53d6\u96a7\u9053\u5217\u8868\u5931\u8d25',
        loadDataFailed: '\u52a0\u8f7d\u6570\u636e\u5931\u8d25',
        unknownUser: '\u672a\u77e5\u7528\u6237',
        nameRequired: '\u8bf7\u8f93\u5165\u8f6c\u53d1\u540d\u79f0',
        nameLength: '\u8f6c\u53d1\u540d\u79f0\u957f\u5ea6\u5e94\u5728 2-50 \u4e2a\u5b57\u7b26\u4e4b\u95f4',
        tunnelRequired: '\u8bf7\u9009\u62e9\u5173\u8054\u96a7\u9053',
        remoteAddrRequired: '\u8bf7\u8f93\u5165\u8fdc\u7a0b\u5730\u5740',
        remoteAddrLineInvalid: '\u7b2c {line} \u884c\u5730\u5740\u683c\u5f0f\u9519\u8bef',
        portRange: '\u7aef\u53e3\u53f7\u5fc5\u987b\u5728 1-65535 \u4e4b\u95f4',
        portRangeTunnel: '\u7aef\u53e3\u53f7\u5fc5\u987b\u5728 {start}-{end} \u8303\u56f4\u5185',
        updated: '\u4fee\u6539\u6210\u529f',
        created: '\u521b\u5efa\u6210\u529f',
        actionFailed: '\u64cd\u4f5c\u5931\u8d25',
        runtimeBusy: '\u5f53\u524d\u8fd0\u884c\u65f6\u4efb\u52a1\u4ecd\u5728\u6392\u961f\u6216\u6267\u884c\u4e2d\uff0c\u8bf7\u7b49\u5f85\u5b8c\u6210\u540e\u518d\u64cd\u4f5c',
        invalidStatus: '\u8f6c\u53d1\u72b6\u6001\u5f02\u5e38\uff0c\u65e0\u6cd5\u64cd\u4f5c',
        serviceChanged: '\u670d\u52a1\u53d8\u66f4\u5df2\u63d0\u4ea4',
        servicePaused: '\u6682\u505c\u8bf7\u6c42\u5df2\u63d0\u4ea4',
        networkActionFailed: '\u7f51\u7edc\u9519\u8bef\uff0c\u64cd\u4f5c\u5931\u8d25',
        bulkActionComplete: '批量操作完成：成功 {success} 条，失败 {failed} 条',
        bulkDeleteConfirm: '确认删除已选择的 {count} 条转发吗？批量模式下常规删除失败不会自动强制删除。',
        deleted: '\u5220\u9664\u6210\u529f',
        forceDeleted: '\u5f3a\u5236\u5220\u9664\u6210\u529f',
        forceDeleteFailed: '\u5f3a\u5236\u5220\u9664\u5931\u8d25',
        deleteFailed: '\u5220\u9664\u5931\u8d25',
        diagnosisFailed: '\u8bca\u65ad\u5931\u8d25',
        diagnosisProcessingFailed: '\u8bca\u65ad\u8fc7\u7a0b\u4e2d\u53d1\u751f\u9519\u8bef',
        diagnosisNetworkFailed: '\u7f51\u7edc\u9519\u8bef\uff0c\u8bca\u65ad\u5931\u8d25',
        unableConnectServer: '\u65e0\u6cd5\u8fde\u63a5\u5230\u670d\u52a1\u5668',
        contentCopied: '{label}\u5df2\u590d\u5236',
        copyFailedHttp: '\u590d\u5236\u5931\u8d25\uff1aHTTP \u4e0b\u65e0\u6cd5\u590d\u5236\uff0c\u9700 HTTPS/\u53cd\u4ee3',
        selectExportTunnel: '\u8bf7\u9009\u62e9\u8981\u5bfc\u51fa\u7684\u96a7\u9053',
        noExportData: '\u6240\u9009\u96a7\u9053\u6ca1\u6709\u8f6c\u53d1\u6570\u636e',
        exportFailed: '\u5bfc\u51fa\u5931\u8d25',
        enterImportData: '\u8bf7\u8f93\u5165\u8981\u5bfc\u5165\u7684\u6570\u636e',
        selectImportTunnel: '\u8bf7\u9009\u62e9\u8981\u5bfc\u5165\u7684\u96a7\u9053',
        importCompleted: '\u5bfc\u5165\u6267\u884c\u5b8c\u6210',
        importFailed: '\u5bfc\u5165\u8fc7\u7a0b\u4e2d\u53d1\u751f\u9519\u8bef',
        importFormatError: '\u683c\u5f0f\u9519\u8bef\uff1a\u81f3\u5c11\u9700\u8981\u5305\u542b\u76ee\u6807\u5730\u5740\u548c\u8f6c\u53d1\u540d\u79f0',
        importRequiredFields: '\u76ee\u6807\u5730\u5740\u548c\u8f6c\u53d1\u540d\u79f0\u4e0d\u80fd\u4e3a\u7a7a',
        importAddressInvalid: '\u76ee\u6807\u5730\u5740\u683c\u5f0f\u9519\u8bef\uff0c\u5e94\u4e3a host:port\uff0c\u591a\u4e2a\u5730\u5740\u7528\u9017\u53f7\u5206\u9694',
        importPortInvalid: '\u5165\u53e3\u7aef\u53e3\u683c\u5f0f\u9519\u8bef\uff0c\u5e94\u4e3a 1-65535 \u4e4b\u95f4\u7684\u6570\u5b57',
        importCreateSuccess: '\u521b\u5efa\u6210\u529f',
        importCreateFailed: '\u521b\u5efa\u5931\u8d25',
        importNetworkCreateFailed: '\u7f51\u7edc\u9519\u8bef\uff0c\u521b\u5efa\u5931\u8d25',
        orderSaveFailed: '\u4fdd\u5b58\u6392\u5e8f\u5931\u8d25\uff1a{message}',
        orderSaveRetry: '\u4fdd\u5b58\u6392\u5e8f\u5931\u8d25\uff0c\u8bf7\u91cd\u8bd5',
        unknownError: '\u672a\u77e5\u9519\u8bef',
        localRuntimeTunnelForwardUnsupported: '\u672c\u5730 Ansible runtime \u4e0d\u80fd\u9644\u7740 Tunnel Forward \u96a7\u9053\uff0c\u8bf7\u5207\u6362\u5230 NodeX Runtime \u6216\u6539\u9009 Port Forward \u96a7\u9053'
      },
      card: {
        dragHandleTitle: '\u62d6\u62fd\u6392\u5e8f',
        ingressAddressTitle: '\u5165\u53e3\u5730\u5740',
        ingressLabel: '\u5165\u53e3',
        targetAddressTitle: '\u76ee\u6807\u5730\u5740',
        targetLabel: '\u76ee\u6807',
        status: {
          normal: '\u6b63\u5e38',
          paused: '\u6682\u505c',
          error: '\u5f02\u5e38',
          unknown: '\u672a\u77e5'
        },
        strategy: {
          round: '\u8f6e\u8be2',
          random: '\u968f\u673a',
          hash: '\u54c8\u5e0c',
          primaryBackup: '\u4e3b\u5907'
        }
      }
    },
    tunnel: {
      note: 'NodeX \u6a21\u5f0f\u4f1a\u62c6\u5206 ingress \u548c\u6267\u884c\u8282\u70b9\uff1b\u672c\u5730 Ansible \u6a21\u5f0f\u53ea\u9700\u8981 inventory \u4e2d\u6620\u5c04\u7684\u6267\u884c\u8282\u70b9\u3002\u96a7\u9053\u201c\u5728\u7ebf\u201d\u53ea\u68c0\u67e5 host:port \u53ef\u8fde\u901a\u6027\uff0c\u4e0d\u80fd\u786e\u8ba4\u8fdc\u7a0b\u6302\u8f7d\u6216\u9632\u706b\u5899\u72b6\u6001\u5df2\u7ecf\u5c31\u4f4d\u3002',
      modeLabelNodeX: '\u5f53\u524d\u8fd0\u884c\u65f6\uff1aNodeX / gost',
      modeLabelLocal: '\u5f53\u524d\u8fd0\u884c\u65f6\uff1aLocal / {backend}',
      modeSummaryNodeX: 'ingress \u4e0e egress \u8bed\u4e49\u7531 NodeX/gost \u63a7\u5236\u3002NodeX Runtime \u9875\u9762\u8d1f\u8d23\u63a7\u5236\u9762 URL\u3001Token \u548c gost \u5c31\u7eea\u6027\u3002',
      modeSummaryLocal: '\u8fd9\u91cc\u53ea\u5b58\u50a8\u6267\u884c\u8282\u70b9\u8eab\u4efd\u3002inventory\u3001playbook \u548c\u9762\u677f\u5bbf\u4e3b ansible \u6267\u884c\u5668\u8bf7\u5230 Local Runtime \u9875\u9762\u7ba1\u7406\u3002',
      modeCompatibilityHint: '\u4e0b\u65b9\u96a7\u9053\u5361\u7247\u662f\u6309\u201c\u5f53\u524d\u8fd0\u884c\u65f6\u80fd\u5426\u6267\u884c\u201d\u6765\u6807\u8bb0\u7684\u3002\u4e00\u4e9b\u5386\u53f2 type-1 \u96a7\u9053\u5728\u8868\u7ed3\u6784\u4e0a\u53ef\u80fd\u540c\u65f6\u517c\u5bb9\u4e24\u79cd\u8fd0\u884c\u65f6\uff0c\u4e0d\u8981\u4ec5\u51ed\u5b58\u50a8\u5b57\u6bb5\u63a8\u65ad\u5b83\u5c5e\u4e8e NodeX \u8fd8\u662f Ansible\u3002',
      loading: '\u6b63\u5728\u52a0\u8f7d\u96a7\u9053\u4e0e\u8282\u70b9\u6570\u636e...',
      emptyTitle: '\u6682\u65e0\u96a7\u9053\u914d\u7f6e',
      emptyText: '\u8bf7\u5148\u5b8c\u6210\u6240\u9700\u62d3\u6251\uff0c\u518d\u521b\u5efa\u7b2c\u4e00\u4e2a\u53ef\u88ab\u8f6c\u53d1\u5f15\u7528\u7684\u96a7\u9053\u3002',
      actions: {
        add: '\u65b0\u589e\u96a7\u9053',
        edit: '\u7f16\u8f91',
        diagnose: '\u8bca\u65ad',
        delete: '\u5220\u9664'
      },
      meta: {
        ingressNode: '\u8f6c\u53d1\u5165\u53e3\u8282\u70b9',
        egressNode: '\u8f6c\u53d1\u51fa\u53e3\u8282\u70b9',
        executionNode: '\u4e2d\u8f6c\u6267\u884c\u8282\u70b9',
        flowAccounting: '\u6d41\u91cf\u8ba1\u7b97',
        trafficRatio: '\u6d41\u91cf\u500d\u7387'
      },
      modal: {
        eyebrow: 'Tunnel',
        titleEdit: '\u7f16\u8f91\u96a7\u9053',
        titleAdd: '\u65b0\u589e\u96a7\u9053',
        deleteEyebrow: 'Delete',
        deleteTitle: '\u786e\u8ba4\u5220\u9664',
        deleteConfirmMessage: '\u786e\u8ba4\u5220\u9664\u96a7\u9053 {name} \u5417\uff1f',
        deleteHint: '\u5982\u679c\u8be5\u96a7\u9053\u4ecd\u88ab\u8f6c\u53d1\u89c4\u5219\u6216\u7528\u6237\u6743\u9650\u5f15\u7528\uff0c\u540e\u7aef\u4f1a\u963b\u6b62\u5220\u9664\u3002',
        submitLoading: '\u63d0\u4ea4\u4e2d...',
        submitUpdate: '\u66f4\u65b0',
        submitCreate: '\u521b\u5efa',
        deleteLoading: '\u5220\u9664\u4e2d...',
        confirmDelete: '\u786e\u8ba4\u5220\u9664'
      },
      fields: {
        name: '\u96a7\u9053\u540d\u79f0',
        tunnelType: '\u96a7\u9053\u7c7b\u578b',
        flowAccounting: '\u6d41\u91cf\u8ba1\u7b97',
        trafficRatio: '\u6d41\u91cf\u500d\u7387',
        ingressNode: 'NodeX \u5165\u53e3\u8282\u70b9',
        executionNode: '\u4e2d\u8f6c\u6267\u884c\u8282\u70b9',
        tcpListenAddr: 'TCP \u76d1\u542c\u5730\u5740',
        udpListenAddr: 'UDP \u76d1\u542c\u5730\u5740',
        interfaceName: '\u51fa\u53e3\u7f51\u5361\u540d\u6216 IP',
        protocol: '\u534f\u8bae\u7c7b\u578b',
        egressNode: '\u8f6c\u53d1\u51fa\u53e3\u8282\u70b9'
      },
      placeholders: {
        name: 'HK-Tunnel-01',
        interfaceName: 'eth0 / 192.0.2.10'
      },
      options: {
        portForward: '\u7aef\u53e3\u8f6c\u53d1',
        tunnelForward: '\u96a7\u9053\u8f6c\u53d1',
        oneWayAccounting: '\u5355\u5411\u8ba1\u7b97',
        twoWayAccounting: '\u53cc\u5411\u8ba1\u7b97'
      },
      hints: {
        ingressNode: '\u53ea\u6709 NodeX/gost \u6a21\u5f0f\u4f1a\u5728\u8fd9\u91cc\u4f7f\u7528 ingress \u8282\u70b9\u3002\u8fd9\u662f\u8f6c\u53d1 relay \u89d2\u8272\uff0c\u4e0e\u4ee3\u7406\u8282\u70b9\u4fdd\u6301\u5206\u79bb\u3002',
        executionNode: '\u672c\u5730 Ansible \u6a21\u5f0f\u53ea\u9700\u8981\u6267\u884c\u8282\u70b9\u8eab\u4efd\u3002SSH \u8bbf\u95ee\u4ecd\u7136\u6765\u81ea\u5df2\u914d\u7f6e\u7684 inventory \u548c local runtime \u53c2\u6570\u3002',
        egressNode: '\u51fa\u53e3\u8282\u70b9\u53ea\u7528\u4e8e NodeX/gost \u96a7\u9053\u8f6c\u53d1\u3002\u9762\u677f\u4fdd\u5b58\u6210\u529f\u540e\uff0c\u4ecd\u7136\u9700\u8981 runtime \u4efb\u52a1\u5728\u8fdc\u7a0b\u5b8c\u6210\u6302\u8f7d\u3002'
      },
      compatibility: {
        nodeXReady: '\u53ef\u7528\u4e8e NodeX Runtime',
        nodeXNeedsIngress: 'NodeX Runtime \u9700\u8981\u5165\u53e3\u8282\u70b9',
        nodeXNeedsEgress: 'NodeX \u96a7\u9053\u8f6c\u53d1\u7f3a\u5c11\u51fa\u53e3\u8282\u70b9',
        localReady: '\u53ef\u7528\u4e8e Local Runtime',
        localNeedsExecution: 'Local Runtime \u9700\u8981\u6267\u884c\u8282\u70b9',
        localOnlyPortForward: 'Local Runtime \u53ea\u652f\u6301\u7aef\u53e3\u8f6c\u53d1\u96a7\u9053'
      },
      messages: {
        loadListFailed: '\u83b7\u53d6\u96a7\u9053\u5217\u8868\u5931\u8d25',
        loadDataFailed: '\u52a0\u8f7d\u6570\u636e\u5931\u8d25',
        created: '\u96a7\u9053\u521b\u5efa\u6210\u529f',
        updated: '\u96a7\u9053\u66f4\u65b0\u6210\u529f',
        actionFailed: '\u64cd\u4f5c\u5931\u8d25',
        deleted: '\u96a7\u9053\u5220\u9664\u6210\u529f',
        deleteFailed: '\u5220\u9664\u5931\u8d25',
        diagnosis: '\u96a7\u9053\u8bca\u65ad',
        diagnosisFailed: '\u8bca\u65ad\u5931\u8d25',
        diagnosisRequestFailed: '\u8bca\u65ad\u8bf7\u6c42\u5931\u8d25'
      },
      diagnosis: {
        eyebrow: 'Diagnosis',
        title: '\u96a7\u9053\u8bca\u65ad\u7ed3\u679c',
        loading: '\u6b63\u5728\u8bca\u65ad\u96a7\u9053\u8fde\u901a\u6027...',
        targetAddress: '\u76ee\u6807\u5730\u5740',
        duration: '\u8017\u65f6',
        message: '\u4fe1\u606f',
        emptyTitle: '\u6682\u65e0\u8bca\u65ad\u7ed3\u679c',
        emptyText: '\u5f53\u524d\u6ca1\u6709\u53ef\u5c55\u793a\u7684\u8282\u70b9\u8bca\u65ad\u6570\u636e\u3002',
        rerun: '\u91cd\u65b0\u8bca\u65ad',
        rerunning: '\u8bca\u65ad\u4e2d...'
      },
      validation: {
        nameRequired: '\u8bf7\u8f93\u5165\u96a7\u9053\u540d\u79f0',
        nameLength: '\u96a7\u9053\u540d\u79f0\u957f\u5ea6\u5e94\u5728 2-50 \u4e2a\u5b57\u7b26\u4e4b\u95f4',
        typeInvalid: '\u8bf7\u9009\u62e9\u6709\u6548\u7684\u96a7\u9053\u7c7b\u578b',
        ingressRequired: '\u8bf7\u9009\u62e9\u8f6c\u53d1\u5165\u53e3\u8282\u70b9',
        ingressMustRelay: '\u5165\u53e3\u8282\u70b9\u5fc5\u987b\u662f\u8f6c\u53d1\u4e2d\u7ee7\u8282\u70b9',
        trafficRatio: '\u6d41\u91cf\u500d\u7387\u5fc5\u987b\u5728 0.1-100.0 \u4e4b\u95f4',
        tcpListenRequired: '\u8bf7\u8f93\u5165 TCP \u76d1\u542c\u5730\u5740',
        udpListenRequired: '\u8bf7\u8f93\u5165 UDP \u76d1\u542c\u5730\u5740',
        egressRequired: '\u8bf7\u9009\u62e9\u8f6c\u53d1\u51fa\u53e3\u8282\u70b9',
        ingressEgressDifferent: '\u8f6c\u53d1\u5165\u53e3\u8282\u70b9\u548c\u8f6c\u53d1\u51fa\u53e3\u8282\u70b9\u4e0d\u80fd\u76f8\u540c',
        egressMustExit: '\u51fa\u53e3\u8282\u70b9\u5fc5\u987b\u662f\u8f6c\u53d1\u51fa\u53e3\u8282\u70b9',
        protocolRequired: '\u8bf7\u9009\u62e9\u534f\u8bae\u7c7b\u578b',
        executionRequired: '\u8bf7\u9009\u62e9\u4e2d\u8f6c\u6267\u884c\u8282\u70b9',
        executionMustRelay: '\u4e2d\u8f6c\u6267\u884c\u8282\u70b9\u5fc5\u987b\u662f\u8f6c\u53d1\u4e2d\u7ee7\u8282\u70b9'
      }
    },
    workbench: {
      eyebrow: 'Forward Runtime',
      title: 'Runtime Workbench',
      subtitle: '\u53cc\u8fd0\u884c\u65f6\u63a7\u5236\u9762',
      actions: {
        refreshJobs: '\u5237\u65b0\u4efb\u52a1',
        openAnsibleMachines: '\u6253\u5f00 Ansible Machines',
        openLocalRuntime: '\u6253\u5f00 Local Runtime',
        openNodeXRuntime: '\u6253\u5f00 NodeX Runtime',
        refreshActiveRuntime: '\u5237\u65b0\u5f53\u524d\u8fd0\u884c\u65f6',
        runDoctorActiveRuntime: '\u5bf9\u5f53\u524d\u8fd0\u884c\u65f6\u8fd0\u884c Doctor'
      },
      localCard: {
        eyebrow: 'Local Runtime',
        title: '\u672c\u5730 Ansible \u6267\u884c\u5668',
        description: '\u9762\u677f\u4e3b\u673a\u65e0\u72b6\u6001\u6267\u884c\u3002Inventory\u3001playbook \u548c SSH \u8bbf\u95ee\u72ec\u7acb\u4e8e NodeX \u7ba1\u7406\u3002',
        currentState: '\u5f53\u524d\u72b6\u6001',
        manage: '\u7ba1\u7406 Local Runtime / Ansible'
      },
      nodeXCard: {
        eyebrow: 'NodeX Runtime',
        title: '\u6709\u72b6\u6001 gost \u63a7\u5236\u9762',
        description: '\u9762\u677f\u4f1a\u76f4\u63a5\u8fde\u63a5\u5185\u90e8 NodeX \u63a7\u5236\u9762\u3002\u53ea\u6709 gost runtime \u4efb\u52a1\u6210\u529f\u540e\uff0crelay \u624d\u7b97\u771f\u6b63\u6302\u8f7d\u3002',
        manage: '\u7ba1\u7406 NodeX Runtime'
      },
      state: {
        activeBackend: '\u5df2\u6fc0\u6d3b',
        standby: '\u5f85\u547d'
      },
      references: {
        panelRuntimeDoc: '\u9762\u677f\u6587\u6863: docs/reference/runtime.md',
        panelRelayOnboarding: '\u9762\u677f\u6587\u6863: docs/guide/forward-relay-onboarding.md',
        nodeXRepo: 'NodeX \u4ed3\u5e93: https://github.com/zdwtest/NodeX',
        panelNodeXOnboarding: '\u9762\u677f\u6587\u6863: docs/forward-runtime-relay-onboarding.md'
      },
      recentJobsTitle: '\u6700\u8fd1\u8fd0\u884c\u65f6\u4efb\u52a1',
      recentJobsSubtitle: '\u663e\u793a Local Runtime \u548c NodeX Runtime \u4e13\u7528\u9875\u9762\u6700\u8fd1\u6392\u961f\u6216\u5df2\u6267\u884c\u7684\u52a8\u4f5c\u3002',
      loadingJobs: '\u52a0\u8f7d\u8fd0\u884c\u65f6\u4efb\u52a1\u4e2d...',
      noJobs: '\u6682\u65e0\u8fd0\u884c\u65f6\u4efb\u52a1\u3002',
      jobMeta: '{backend} / forward {forwardId} / tunnel {tunnelId} / node {nodeId}',
      doctor: {
        eyebrow: 'Forward Runtime Doctor',
        title: '\u5f53\u524d\u8fd0\u884c\u65f6\u5feb\u7167',
        description: '\u53ef\u8fde\u901a\u53ea\u80fd\u8bf4\u660e\u63a7\u5236\u9762\u6216\u672c\u5730\u6267\u884c\u5668\u53ef\u4ee5\u8bbf\u95ee\uff0c\u5e76\u4e0d\u80fd\u8bc1\u660e relay \u5df2\u6302\u8f7d\u6210\u529f\uff0c\u4e5f\u4e0d\u4ee3\u8868 iptables \u89c4\u5219\u5df2\u5b58\u5728\u3002',
        note: '\u8fd9\u4e2a workbench \u53ea\u663e\u793a\u5f53\u524d\u6d3b\u8dc3 backend\u3002\u82e5\u8981\u7f16\u8f91\u914d\u7f6e\u6216\u8fd0\u884c\u6a21\u5f0f\u7279\u5b9a\u63a2\u6d4b\uff0c\u8bf7\u524d\u5f80 Local Runtime \u548c NodeX Runtime \u4e13\u9875\u3002',
        summary: '\u8fd9\u4e2a workbench \u6c47\u603b\u63a7\u5236\u9762\u5065\u5eb7\u3001\u8fd0\u884c\u65f6\u8bca\u65ad\u548c\u4e00\u952e\u547d\u4ee4\uff0c\u7edf\u4e00\u8986\u76d6 NodeX/gost \u548c\u672c\u5730 Ansible \u4e24\u79cd\u6267\u884c\u8def\u5f84\u3002',
        loadingStatus: '\u6b63\u5728\u83b7\u53d6 forward runtime \u72b6\u6001...'
      },
      cards: {
        backend: 'Backend',
        nodeXMode: 'NodeX Mode',
        attachment: '\u6302\u8f7d\u6a21\u5f0f',
        panelVerdict: '\u9762\u677f\u5224\u5b9a',
        nodeXSnapshot: 'NodeX Snapshot',
        baseUrlConfigured: 'Base URL \u5df2\u914d\u7f6e',
        runtimeVersion: '\u8fd0\u884c\u65f6\u7248\u672c',
        localAnsible: '\u672c\u5730 ansible',
        playbooks: 'Playbooks'
      },
      errors: {
        fetchStatusFailed: '\u83b7\u53d6 forward runtime \u72b6\u6001\u5931\u8d25',
        doctorFailed: 'Forward runtime Doctor \u6267\u884c\u5931\u8d25',
        savedConfigInvalid: '\u5df2\u4fdd\u5b58\u7684 ansible runtime \u914d\u7f6e\u65e0\u6548\uff0c\u5df2\u56de\u9000\u5230\u9ed8\u8ba4\u503c\uff0c\u8bf7\u91cd\u65b0\u4fdd\u5b58\u4ee5\u4fee\u590d\u3002',
        nodeXBaseUrlRequired: 'NodeX Mode \u4e0b\u5fc5\u987b\u586b\u5199 NodeX base URL',
        nodeXTokenRequired: 'NodeX Mode \u4e0b\u5fc5\u987b\u586b\u5199 NodeX token',
        invalidRuntimeJson: 'ansible JSON \u65e0\u6548',
        saveFailed: '\u4fdd\u5b58 runtime \u914d\u7f6e\u5931\u8d25'
      }
    },
    systemPage: {
      title: '系统管理',
      subtitle: '系统配置、数据备份与负载均衡',
      tabs: {
        config: '系统配置',
        backup: '数据备份',
        balancer: '负载均衡',
        audit: '审计日志'
      },
      actions: {
        addConfig: '新增配置',
        createBalancer: '新建负载均衡器',
        restore: '恢复',
        healthCheck: '健康检查'
      },
      config: {
        searchPlaceholder: '搜索配置项...',
        table: {
          key: '键名',
          value: '值',
          description: '描述',
          updatedAt: '更新时间',
          actions: '操作'
        },
        empty: '暂无配置数据'
      },
      subscription: {
        eyebrow: '订阅',
        title: '订阅域名',
        description: '配置一个或多个用于用户订阅链接和 managed-config 输出的域名。',
        pathLabel: '订阅路径',
        currentDomainLabel: '当前请求 Host',
        domainListLabel: '备用域名列表',
        domainListPlaceholder: 'sub1.example.com\nsub2.example.com',
        domainListHelp: '每行一个域名，也支持逗号或分号分隔。',
        previewLabel: '链接预览',
        previewEmpty: '未配置备用域名时，将使用当前请求 Host。',
        actions: {
          refresh: '刷新',
          save: '保存域名',
          saving: '保存中...'
        },
        messages: {
          loadFailed: '加载订阅域名配置失败',
          saveFailed: '保存订阅域名失败',
          saveSuccess: '订阅域名已保存'
        }
      },
      backup: {
        title: '自动备份配置',
        enabled: '启用自动备份',
        intervalHours: '备份间隔 (小时)',
        keepCount: '保留数量',
        backupDatabase: '包含数据库',
        backupFiles: '包含文件',
        storageType: '存储类型',
        storagePath: '本地存储路径',
        storagePathPlaceholder: '例如：backups',
        s3Bucket: 'S3 Bucket',
        s3BucketPlaceholder: '例如：panel-backups',
        s3Region: 'S3 Region',
        s3RegionPlaceholder: '例如：us-east-1',
        s3Endpoint: 'S3 Endpoint（可选）',
        s3EndpointPlaceholder: '例如：https://s3.amazonaws.com',
        s3AccessKey: 'S3 Access Key',
        s3AccessKeyPlaceholder: '留空则保留当前密钥',
        s3SecretKey: 'S3 Secret Key',
        s3SecretKeyPlaceholder: '留空则保留当前密钥',
        storageTypes: {
          local: '本地',
          s3: 'S3 兼容存储'
        },
        sensitiveHintWithValue: '敏感值已隐藏。留空将保留当前值，输入新值可进行轮换。',
        sensitiveHintWithoutValue: '该敏感字段尚未设置，请输入后保存。',
        saveConfig: '保存配置',
        backupNow: '立即备份',
        stats: {
          totalCount: '总备份数:',
          totalSize: '总大小:',
          lastBackup: '最近备份:'
        },
        listTitle: '备份列表',
        table: {
          id: 'ID',
          filename: '文件名',
          size: '大小',
          status: '状态',
          createdAt: '创建时间',
          actions: '操作'
        },
        empty: '暂无备份数据'
      },
      balancer: {
        table: {
          id: 'ID',
          name: '名称',
          group: '节点组',
          strategy: '策略',
          healthCheck: '健康检查',
          enabled: '状态',
          actions: '操作'
        },
        empty: '暂无负载均衡器'
      },
      audit: {
        title: '操作审计日志',
        actions: {
          filter: '筛选',
          refresh: '刷新'
        },
        filters: {
          actionPlaceholder: '按 action 过滤',
          targetTypePlaceholder: '按 target_type 过滤'
        },
        table: {
          id: 'ID',
          action: '动作',
          module: '模块',
          targetType: '目标类型',
          username: '操作人',
          content: '内容',
          ip: 'IP',
          status: '状态',
          createdAt: '创建时间'
        },
        pagination: {
          total: '总数: {total}',
          pageSize: '每页',
          page: '第 {page} / {totalPages} 页',
          prev: '上一页',
          next: '下一页'
        },
        empty: '暂无审计日志'
      },
      configModal: {
        titleEdit: '编辑配置',
        titleCreate: '新增配置',
        key: '键名',
        value: '值',
        description: '描述',
        keyPlaceholder: '如: site.name',
        valuePlaceholder: '配置值，支持 JSON 格式',
        descriptionPlaceholder: '配置说明',
        sensitiveHintWithValue: '敏感值已隐藏。留空将保留当前值，输入新值将覆盖。',
        sensitiveHintWithoutValue: '该配置为敏感项，请输入值后保存。'
      },
      balancerModal: {
        titleEdit: '编辑负载均衡器',
        titleCreate: '新建负载均衡器',
        name: '名称',
        namePlaceholder: '负载均衡器名称',
        groupId: '节点组ID',
        strategy: '策略',
        healthCheck: '启用健康检查',
        checkInterval: '检查间隔 (秒)',
        weightsJson: '节点权重 (JSON)',
        weightsPlaceholder: '{"1": 10, "2": 5}'
      },
      strategy: {
        roundRobin: '轮询',
        leastLoad: '最少负载',
        latency: '最低延迟',
        weight: '加权',
        random: '随机'
      },
      status: {
        pending: '处理中',
        completed: '已完成',
        failed: '失败'
      },
      booleans: {
        enabled: '启用',
        disabled: '禁用'
      },
      messages: {
        fetchConfigsFailed: '获取配置失败',
        fetchAuditLogsFailed: '获取审计日志失败',
        saveConfigFailed: '保存失败: {message}',
        deleteConfigConfirm: '确定删除配置 {key}?',
        deleteConfigFailed: '删除失败',
        fetchBackupConfigFailed: '获取备份配置失败',
        backupConfigSaved: '保存成功',
        backupConfigSaveFailed: '保存失败',
        backupStarted: '备份已开始',
        backupStartFailed: '创建备份失败',
        fetchBackupsFailed: '获取备份列表失败',
        fetchBackupStatsFailed: '获取备份统计失败',
        deleteBackupConfirm: '确定删除备份 {filename}?',
        deleteBackupFailed: '删除失败',
        restoreBackupConfirm: '确定恢复备份 {filename}? 当前数据将被覆盖。',
        restoreBackupSuccess: '恢复成功',
        restoreBackupFailed: '恢复失败: {message}',
        fetchBalancersFailed: '获取负载均衡器失败',
        weightsJsonInvalid: '权重 JSON 格式错误',
        saveBalancerFailed: '保存失败: {message}',
        deleteBalancerConfirm: '确定删除负载均衡器 {name}?',
        deleteBalancerFailed: '删除失败',
        healthCheckCompleted: '健康检查已完成',
        healthCheckFailed: '健康检查失败'
      }
    },
    nodeXTopology: {
      heroEyebrow: 'NodeX Topology',
      title: 'NodeX Topology + Legacy Rules',
      heroText: '\u8fd9\u4e2a\u9875\u9762\u53ea\u7ba1\u7406 NodeX \u6709\u72b6\u6001 relay/exit \u62d3\u6251\u53ca Legacy \u89c4\u5219\u517c\u5bb9\u5c42\u3002\u65e0\u72b6\u6001\u6267\u884c\u4e3b\u673a\u8bf7\u653e\u5230 Ansible Machines \u9875\u9762\u5355\u72ec\u7ba1\u7406\u3002',
      nodesEyebrow: 'Nodes',
      nodesTitle: 'NodeX Relay / Exit Topology',
      nodesText: '\u8fd9\u91cc\u53ea\u7528\u4e8e NodeX \u6709\u72b6\u6001 relay/exit \u62d3\u6251\u3001\u8fde\u901a\u6027\u68c0\u67e5\u548c gost API \u76f8\u5173\u64cd\u4f5c\uff0c\u4e0d\u627f\u8f7d Ansible \u673a\u5668\u7ba1\u7406\u3002',
      loading: '\u6b63\u5728\u52a0\u8f7d NodeX \u62d3\u6251\u8282\u70b9...',
      emptyTitle: '\u6682\u65e0 NodeX \u62d3\u6251\u8282\u70b9',
      emptyText: '\u8bf7\u5148\u521b\u5efa relay / exit \u8282\u70b9\u7528\u4e8e NodeX \u6a21\u5f0f\u3002\u82e5\u53ea\u505a\u65e0\u72b6\u6001\u6267\u884c\uff0c\u8bf7\u6539\u5230 Ansible Machines \u9875\u9762\u3002',
      legacyText: '\u8fd9\u4e2a\u533a\u5757\u5bf9\u5e94 `/admin/forward/rules*` \u517c\u5bb9\u63a5\u53e3\u3002\u5b83\u53ea\u4fdd\u7559 Legacy \u89c4\u5219\u80fd\u529b\uff0c\u4e0d\u4ee3\u8868 NodeX \u6216 Ansible \u7684\u5f53\u524d\u4e3b\u8fd0\u884c\u8def\u5f84\u3002',
      actions: {
        refresh: '\u5237\u65b0',
        testConnection: '\u6d4b\u8bd5\u8fde\u63a5',
        addLegacyRule: '\u65b0\u589e Legacy \u89c4\u5219',
        addNode: '\u65b0\u589e\u8282\u70b9',
        query: '\u67e5\u8be2',
        clear: '\u6e05\u7a7a',
        addRule: '\u65b0\u589e\u89c4\u5219',
        edit: '\u7f16\u8f91',
        healthCheck: '\u5065\u5eb7\u68c0\u67e5',
        checking: '\u68c0\u6d4b\u4e2d...',
        syncStats: '\u540c\u6b65\u7edf\u8ba1',
        syncing: '\u540c\u6b65\u4e2d...',
        enable: '\u542f\u7528',
        disable: '\u7981\u7528',
        delete: '\u5220\u9664',
        startTest: '\u5f00\u59cb\u68c0\u6d4b',
        confirmDelete: '\u786e\u8ba4\u5220\u9664',
        cancel: '\u53d6\u6d88',
        save: '\u4fdd\u5b58',
        saveChanges: '\u4fdd\u5b58\u4fee\u6539',
        createNode: '\u521b\u5efa\u8282\u70b9',
        createRule: '\u521b\u5efa\u89c4\u5219'
      },
      filters: {
        nodeType: '\u8282\u70b9\u7c7b\u578b',
        status: '\u72b6\u6001',
        all: '\u5168\u90e8',
        online: '\u5728\u7ebf',
        offline: '\u79bb\u7ebf',
        userId: '\u7528\u6237 ID',
        userIdPlaceholder: '\u6309\u7528\u6237 ID \u8fc7\u6ee4',
        relay: 'Relay',
        exit: 'Exit'
      },
      status: {
        enabled: '\u5df2\u542f\u7528',
        disabled: '\u5df2\u7981\u7528',
        online: '\u5728\u7ebf',
        offline: '\u79bb\u7ebf',
        success: '\u6210\u529f',
        failed: '\u5931\u8d25',
        operationSuccess: '\u64cd\u4f5c\u6210\u529f',
        operationFailed: '\u64cd\u4f5c\u5931\u8d25'
      },
      meta: {
        managementApi: '\u7ba1\u7406 API',
        regionIsp: '\u5730\u533a / ISP',
        latency: '\u5ef6\u8fdf',
        currentConnections: '\u5f53\u524d\u8fde\u63a5',
        traffic: '\u4e0a\u884c / \u4e0b\u884c',
        weightMaxConnections: '\u6743\u91cd / \u6700\u5927\u8fde\u63a5',
        lastCheck: '\u6700\u540e\u68c0\u6d4b',
        uptime: '\u5728\u7ebf\u7387',
        rateLimit: '\u901f\u7387',
        trafficLimit: '\u6d41\u91cf',
        expire: '\u8fc7\u671f',
        upload: '\u4e0a\u884c',
        download: '\u4e0b\u884c',
        connections: '\u8fde\u63a5',
        serviceCount: '\u670d\u52a1\u6570\u91cf'
      },
      stats: {
        relayNodes: 'Relay \u8282\u70b9',
        exitNodes: 'Exit \u8282\u70b9',
        totalNodes: '\u8282\u70b9\u603b\u6570',
        onlineNodes: '\u5728\u7ebf\u8282\u70b9',
        totalUpload: '\u7d2f\u8ba1\u4e0a\u884c',
        totalDownload: '\u7d2f\u8ba1\u4e0b\u884c',
        onlineCount: '\u5728\u7ebf {count} \u53f0',
        includesRelayExit: '\u5305\u542b Relay / Exit',
        refreshing: '\u7edf\u8ba1\u5237\u65b0\u4e2d...',
        basedOnLastCheck: '\u57fa\u4e8e\u6700\u8fd1\u4e00\u6b21\u5065\u5eb7\u68c0\u6d4b',
        aggregatedAcrossNodes: '\u6240\u6709\u62d3\u6251\u8282\u70b9\u6c47\u603b'
      },
      pagination: {
        prev: '\u4e0a\u4e00\u9875',
        next: '\u4e0b\u4e00\u9875',
        summary: '\u7b2c {page} / {totalPages} \u9875\uff0c\u5171 {total} \u6761'
      },
      legacy: {
        eyebrow: 'Legacy Rules',
        title: 'Legacy Port Forward Rules',
        text: '\u8be5\u533a\u5757\u5bf9\u5e94 `/admin/forward/rules*` \u517c\u5bb9 API\uff0c\u7528\u4e8e\u4fdd\u7559 relay + exit \u7aef\u53e3\u7ea7\u8f6c\u53d1\u884c\u4e3a\u3002',
        loading: '\u6b63\u5728\u52a0\u8f7d Legacy \u89c4\u5219...',
        emptyTitle: '\u6682\u65e0 Legacy \u89c4\u5219',
        emptyText: '\u5982\u679c\u9700\u8981\u517c\u5bb9 relay + exit \u7aef\u53e3\u7ea7\u8f6c\u53d1\uff0c\u53ef\u5148\u5728\u8fd9\u91cc\u65b0\u589e\u89c4\u5219\u3002',
        columns: {
          id: 'ID',
          name: '\u540d\u79f0',
          ingress: '\u5165\u53e3',
          egress: '\u51fa\u53e3',
          owner: '\u5f52\u5c5e',
          limits: '\u9650\u989d',
          traffic: '\u6d41\u91cf',
          status: '\u72b6\u6001',
          actions: '\u64cd\u4f5c'
        }
      },
      nodeModal: {
        eyebrow: 'Node',
        titleEdit: '\u7f16\u8f91\u4e2d\u8f6c\u8282\u70b9',
        titleAdd: '\u65b0\u589e\u4e2d\u8f6c\u8282\u70b9',
        loading: '\u6b63\u5728\u52a0\u8f7d\u8282\u70b9\u8be6\u60c5...',
        saveLoading: '\u4fdd\u5b58\u4e2d...',
        fields: {
          name: '\u8282\u70b9\u540d\u79f0',
          type: '\u8282\u70b9\u7c7b\u578b',
          host: '\u4e3b\u673a\u5730\u5740',
          servicePort: '\u4e1a\u52a1\u7aef\u53e3',
          apiPort: 'API \u7aef\u53e3',
          apiToken: 'API Token',
          metricsPort: 'Metrics \u7aef\u53e3',
          region: '\u5730\u533a',
          isp: 'ISP',
          bandwidth: '\u5e26\u5bbd (Mbps)',
          maxConnections: '\u6700\u5927\u8fde\u63a5',
          weight: '\u6743\u91cd'
        },
        placeholders: {
          name: '\u4f8b\u5982 relay-hk-01',
          host: '1.2.3.4',
          apiToken: '\u7559\u7a7a\u5219\u540e\u7aef\u81ea\u52a8\u751f\u6210',
          region: 'HK / JP / US',
          isp: 'CMI / NTT / Cogent'
        },
        hints: {
          apiPort: 'NodeX \u7ba1\u7406 API \u7684\u5065\u5eb7\u68c0\u67e5\u3001\u7edf\u8ba1\u540c\u6b65\u548c\u8fde\u901a\u6d4b\u8bd5\u90fd\u9700\u8981\u8be5\u7aef\u53e3\u3002',
          metricsPort: 'gost Prometheus /metrics \u7aef\u53e3\uff0c\u7528\u4e8e\u91c7\u96c6\u8f6c\u53d1\u6d41\u91cf\u7edf\u8ba1\uff0c\u7559\u7a7a\u8868\u793a\u4e0d\u91c7\u96c6\u3002'
        }
      },
      ruleModal: {
        eyebrow: 'Rule',
        titleEdit: '\u7f16\u8f91 Legacy \u89c4\u5219',
        titleAdd: '\u65b0\u589e Legacy \u89c4\u5219',
        loading: '\u6b63\u5728\u52a0\u8f7d\u89c4\u5219\u8be6\u60c5...',
        saveLoading: '\u4fdd\u5b58\u4e2d...',
        ownerReadOnlyHint: '\u5f53\u524d\u540e\u7aef\u66f4\u65b0\u63a5\u53e3\u4e0d\u652f\u6301\u4fee\u6539\u5f52\u5c5e\u5b57\u6bb5\uff0c\u7f16\u8f91\u65f6\u53ea\u8bfb\u3002',
        fields: {
          name: '\u89c4\u5219\u540d\u79f0',
          protocol: '\u534f\u8bae',
          relayNode: '\u5165\u53e3 Relay \u8282\u70b9',
          listenPort: '\u76d1\u542c\u7aef\u53e3',
          exitNode: '\u51fa\u53e3 Exit \u8282\u70b9',
          targetPort: '\u76ee\u6807\u7aef\u53e3',
          targetHost: '\u76ee\u6807\u4e3b\u673a',
          userId: '\u7528\u6237 ID',
          userGroupId: '\u7528\u6237\u7ec4 ID',
          speedLimit: '\u901f\u7387\u4e0a\u9650 (KB/s)',
          trafficLimit: '\u6d41\u91cf\u4e0a\u9650 (Bytes)',
          expireTime: '\u8fc7\u671f\u65f6\u95f4',
          remark: '\u5907\u6ce8'
        },
        placeholders: {
          name: '\u4f8b\u5982 tcp-11111-hk',
          relayNode: '\u8bf7\u9009\u62e9 Relay \u8282\u70b9',
          exitNode: '\u8bf7\u9009\u62e9 Exit \u8282\u70b9',
          targetHost: '127.0.0.1 \u6216\u76ee\u6807\u4e3b\u673a',
          userId: '\u7559\u7a7a\u8868\u793a\u516c\u5171\u89c4\u5219',
          userGroupId: '\u4e0e\u7528\u6237 ID \u4e8c\u9009\u4e00',
          remark: '\u53ef\u8bb0\u5f55\u4e1a\u52a1\u7528\u9014\u6216\u7ef4\u62a4\u8bf4\u660e'
        }
      },
      connectionModal: {
        eyebrow: 'Gost API',
        title: '\u6d4b\u8bd5\u8282\u70b9\u8fde\u63a5',
        fields: {
          host: '\u4e3b\u673a\u5730\u5740',
          apiPort: 'API \u7aef\u53e3',
          apiToken: 'API Token'
        },
        placeholders: {
          host: '127.0.0.1',
          apiToken: '\u5982\u672a\u542f\u7528\u9274\u6743\u53ef\u7559\u7a7a'
        },
        success: '\u8fde\u63a5\u6210\u529f',
        failed: '\u8fde\u63a5\u5931\u8d25',
        testing: '\u68c0\u6d4b\u4e2d...'
      },
      deleteModal: {
        title: '\u786e\u8ba4\u5220\u9664',
        confirmNode: '\u786e\u8ba4\u5220\u9664\u8282\u70b9',
        confirmRule: '\u786e\u8ba4\u5220\u9664\u89c4\u5219',
        warning: '\u5220\u9664\u540e\u65e0\u6cd5\u81ea\u52a8\u6062\u590d\uff0c\u8bf7\u786e\u8ba4\u6ca1\u6709\u4ecd\u5728\u4f7f\u7528\u7684\u8f6c\u53d1\u5173\u7cfb\u3002',
        deleting: '\u5220\u9664\u4e2d...'
      },
      validation: {
        requestFailed: '\u8bf7\u6c42\u5931\u8d25',
        nodeNameRequired: '\u8282\u70b9\u540d\u79f0\u4e0d\u80fd\u4e3a\u7a7a',
        nodeHostRequired: '\u4e3b\u673a\u5730\u5740\u4e0d\u80fd\u4e3a\u7a7a',
        nodePortRange: '\u4e1a\u52a1\u7aef\u53e3\u5fc5\u987b\u5728 1 \u5230 65535 \u4e4b\u95f4',
        nodeApiPortRequired: 'NodeX \u7ba1\u7406 API \u7aef\u53e3\u4e0d\u80fd\u4e3a\u7a7a',
        nodeApiPortRange: 'API \u7aef\u53e3\u5fc5\u987b\u5728 1 \u5230 65535 \u4e4b\u95f4',
        ruleNameRequired: '\u89c4\u5219\u540d\u79f0\u4e0d\u80fd\u4e3a\u7a7a',
        relayNodeRequired: '\u8bf7\u9009\u62e9\u5165\u53e3 Relay \u8282\u70b9',
        exitNodeRequired: '\u8bf7\u9009\u62e9\u51fa\u53e3 Exit \u8282\u70b9',
        listenPortRange: '\u76d1\u542c\u7aef\u53e3\u5fc5\u987b\u5728 1 \u5230 65535 \u4e4b\u95f4',
        targetHostRequired: '\u76ee\u6807\u4e3b\u673a\u4e0d\u80fd\u4e3a\u7a7a',
        targetPortRange: '\u76ee\u6807\u7aef\u53e3\u5fc5\u987b\u5728 1 \u5230 65535 \u4e4b\u95f4',
        ownerConflict: '\u7528\u6237 ID \u4e0e\u7528\u6237\u7ec4 ID \u53ea\u80fd\u586b\u5199\u4e00\u4e2a',
        connectionHostRequired: '\u4e3b\u673a\u5730\u5740\u4e0d\u80fd\u4e3a\u7a7a',
        connectionApiPortRange: 'API \u7aef\u53e3\u5fc5\u987b\u5728 1 \u5230 65535 \u4e4b\u95f4',
        userIdPositive: '\u7528\u6237 ID \u5fc5\u987b\u4e3a\u6b63\u6574\u6570'
      },
      messages: {
        loadStatsFailed: '\u52a0\u8f7d\u8f6c\u53d1\u7edf\u8ba1\u5931\u8d25',
        loadNodesFailed: '\u52a0\u8f7d\u4e2d\u8f6c\u8282\u70b9\u5931\u8d25',
        loadNodeOptionsFailed: '\u52a0\u8f7d\u8282\u70b9\u9009\u9879\u5931\u8d25',
        loadRulesFailed: '\u52a0\u8f7d\u8f6c\u53d1\u89c4\u5219\u5931\u8d25',
        loadNodeDetailFailed: '\u52a0\u8f7d\u8282\u70b9\u8be6\u60c5\u5931\u8d25',
        saveNodeFailed: '\u4fdd\u5b58\u4e2d\u8f6c\u8282\u70b9\u5931\u8d25',
        nodeUpdated: '\u4e2d\u8f6c\u8282\u70b9\u5df2\u66f4\u65b0',
        nodeCreated: '\u4e2d\u8f6c\u8282\u70b9\u5df2\u521b\u5efa',
        nodeDeleted: '\u4e2d\u8f6c\u8282\u70b9\u5df2\u5220\u9664',
        nodeCheckFailed: '\u5065\u5eb7\u68c0\u6d4b\u5931\u8d25',
        nodeSyncFailed: '\u540c\u6b65\u7edf\u8ba1\u5931\u8d25',
        nodeToggleFailed: '\u5207\u6362\u8282\u70b9\u72b6\u6001\u5931\u8d25',
        loadRuleDetailFailed: '\u52a0\u8f7d\u89c4\u5219\u8be6\u60c5\u5931\u8d25',
        saveRuleFailed: '\u4fdd\u5b58\u8f6c\u53d1\u89c4\u5219\u5931\u8d25',
        ruleUpdated: '\u8f6c\u53d1\u89c4\u5219\u5df2\u66f4\u65b0',
        ruleCreated: '\u8f6c\u53d1\u89c4\u5219\u5df2\u521b\u5efa',
        ruleDeleted: '\u8f6c\u53d1\u89c4\u5219\u5df2\u5220\u9664',
        ruleToggleFailed: '\u5207\u6362\u89c4\u5219\u72b6\u6001\u5931\u8d25',
        connectionFailed: '\u8fde\u63a5\u68c0\u6d4b\u5931\u8d25',
        deleteFailed: '\u5220\u9664\u5931\u8d25',
        nodeReachable: '\u8282\u70b9\u53ef\u8fbe',
        nodeUnavailable: '\u8282\u70b9\u4e0d\u53ef\u8fbe',
        syncSuccess: '\u7edf\u8ba1\u540c\u6b65\u6210\u529f',
        connectionSuccess: '\u8fde\u63a5\u6210\u529f',
        connectionError: '\u8fde\u63a5\u5931\u8d25',
        nodeEnabled: '{name} \u5df2\u542f\u7528',
        nodeDisabled: '{name} \u5df2\u7981\u7528',
        ruleEnabled: '{name} \u5df2\u542f\u7528',
        ruleDisabled: '{name} \u5df2\u7981\u7528',
        latency: '\u5ef6\u8fdf {value} ms',
        currentConnectionsSuffix: '\uff0c\u5f53\u524d\u8fde\u63a5 {value}'
      },
      owner: {
        user: '\u7528\u6237 #{id}',
        userGroup: '\u7528\u6237\u7ec4 #{id}',
        public: '\u516c\u5171\u89c4\u5219'
      },
      protocols: {
        tcp: 'TCP',
        udp: 'UDP',
        both: 'TCP + UDP'
      },
      labels: {
        node: 'Node #{id}',
        route: '\u8def',
        none: '\u4e0d\u9650',
        neverExpires: '\u6c38\u4e0d\u8fc7\u671f'
      }
    },
    limitPage: {
      heroEyebrow: '限速管理',
      title: '限速管理',
      subtitle: '按隧道维护限速规则，保持 Flux 风格的独立规则页。',
      note: '限速规则由当前转发运行时强制执行，更改可能需要短暂时间生效。',
      actions: {
        refresh: '刷新',
        create: '新增',
        createNow: '立即创建',
        edit: '编辑',
        delete: '删除'
      },
      loading: '正在加载限速规则...',
      empty: {
        title: '暂无限速规则',
        text: '还没有创建任何限速规则，点击上方按钮开始创建。'
      },
      status: {
        active: '运行',
        error: '异常'
      },
      cards: {
        speed: '速度限制',
        tunnel: '绑定隧道',
        updatedAt: '更新时间'
      },
      formModal: {
        titleCreate: '新增限速规则',
        titleEdit: '编辑限速规则',
        fields: {
          name: '规则名称',
          speed: '速度限制',
          tunnel: '绑定隧道'
        },
        placeholders: {
          name: '请输入限速规则名称',
          speed: '请输入速度限制（Mbps）',
          tunnel: '请选择要绑定的隧道'
        },
        submitting: '提交中...',
        submitCreate: '创建规则',
        submitUpdate: '保存修改'
      },
      deleteModal: {
        title: '确认删除',
        eyebrow: '确认删除',
        confirmText: '确定要删除限速规则 {name} 吗？',
        hint: '此操作无法撤销，删除后该规则将永久消失。',
        deleting: '删除中...',
        confirmDelete: '确认删除'
      },
      values: {
        unlimited: '不限速',
        tunnelFallback: '隧道 #{id}',
        ruleFallback: '规则 #{id}'
      },
      modeLabelNodeX: 'NodeX 模式',
      modeSummaryNodeX: '限速由 NodeX Agent 同步并强制执行。',
      modeLabelLocal: '本地模式',
      modeSummaryLocal: '限速通过本地 {backend} 运行时应用。',
      messages: {
        fetchTunnelsFailed: '获取隧道列表失败',
        fetchRulesFailed: '获取限速规则失败',
        loadFailed: '加载失败',
        nameRequired: '规则名称不能为空',
        nameLength: '规则名称长度应在 2-50 个字符之间',
        speedInvalid: '请输入有效的速度限制（>= 1 Mbps）',
        tunnelRequired: '请选择要绑定的隧道',
        tunnelMissing: '隧道名称不存在，请刷新后重试',
        createFailed: '创建限速规则失败',
        updateFailed: '更新限速规则失败',
        submitFailed: '提交失败',
        deleteFailed: '删除限速规则失败',
        created: '限速规则创建成功',
        updated: '限速规则更新成功',
        deleted: '限速规则删除成功'
      }
    },
    nodeXAgents: {
      title: 'NodeX Agents',
      subtitle: '\u53ea\u6709 NodeX \u6a21\u5f0f\u9700\u8981 agents\u3002\u8fd9\u4e2a\u9875\u9762\u7528\u4e8e agent \u72b6\u6001\u3001\u8fdc\u7a0b\u7ec8\u7aef\u548c\u4efb\u52a1\u4e0b\u53d1\u3002',
      tabs: {
        agents: '\u5728\u7ebf Agent',
        terminal: '\u8fdc\u7a0b\u7ec8\u7aef',
        tasks: '\u4efb\u52a1\u5386\u53f2'
      },
      actions: {
        refresh: '\u5237\u65b0',
        execute: '\u6267\u884c',
        send: '\u53d1\u9001',
        cancel: '\u53d6\u6d88',
        monitor: '\u76d1\u63a7',
        terminalShort: '\u7ec8\u7aef',
        taskShort: '\u4efb\u52a1',
        monitorShort: '\u76d1\u63a7'
      },
      table: {
        nodeId: '\u8282\u70b9 ID',
        version: '\u7248\u672c',
        system: '\u7cfb\u7edf',
        lastSeen: '\u6700\u540e\u5728\u7ebf',
        status: '\u72b6\u6001',
        capabilities: '\u80fd\u529b',
        action: '\u64cd\u4f5c',
        taskId: '\u4efb\u52a1 ID',
        node: '\u8282\u70b9',
        type: '\u7c7b\u578b',
        command: '\u547d\u4ee4 / \u52a8\u4f5c',
        duration: '\u8017\u65f6',
        time: '\u65f6\u95f4'
      },
      status: {
        online: '\u5728\u7ebf',
        offline: '\u79bb\u7ebf',
        connected: '\u5df2\u8fde\u63a5',
        disconnected: '\u672a\u8fde\u63a5',
        success: '\u6210\u529f',
        failed: '\u5931\u8d25'
      },
      empty: {
        agents: '\u6682\u65e0\u5728\u7ebf Agent',
        tasks: '\u6682\u65e0\u4efb\u52a1\u8bb0\u5f55'
      },
      terminal: {
        chooseNode: '\u9009\u62e9\u8282\u70b9',
        nodeLabel: '\u8282\u70b9 #{id}',
        promptPlaceholder: '\u8f93\u5165\u547d\u4ee4...',
        chooseAction: '\u9009\u62e9\u52a8\u4f5c',
        chooseService: '\u9009\u62e9\u670d\u52a1'
      },
      diagnosticActions: {
        service_status: '\u67e5\u770b\u670d\u52a1\u72b6\u6001',
        service_restart: '\u91cd\u542f\u670d\u52a1',
        log_tail: '\u67e5\u770b\u65e5\u5fd7\u5c3e\u90e8'
      },
      fields: {
        service: '\u670d\u52a1',
        lines: '\u884c\u6570'
      },
      services: {
        gost: 'GOST'
      },
      taskModal: {
        title: '\u4e0b\u53d1\u4efb\u52a1',
        targetNode: '\u76ee\u6807\u8282\u70b9',
        taskType: '\u4efb\u52a1\u7c7b\u578b',
        action: '\u52a8\u4f5c',
        paramsJson: '\u53c2\u6570 (JSON)',
        paramsPlaceholder: '{"key": "value"}',
        timeoutSeconds: '\u8d85\u65f6 (\u79d2)'
      },
      taskTypes: {
        command: '\u6267\u884c\u547d\u4ee4',
        file: '\u6587\u4ef6\u64cd\u4f5c',
        service: '\u670d\u52a1\u7ba1\u7406',
        gost: 'GOST \u7ba1\u7406'
      },
      hints: {
        monitor: '\u67e5\u770b\u8282\u70b9 #{id} \u7684\u76d1\u63a7\u6570\u636e'
      },
      messages: {
        fetchFailed: '\u83b7\u53d6 Agent \u5217\u8868\u5931\u8d25',
        taskIncomplete: '\u8bf7\u586b\u5199\u5b8c\u6574\u4fe1\u606f',
        invalidParamsJson: '\u53c2\u6570 JSON \u683c\u5f0f\u65e0\u6548',
        taskSent: '\u4efb\u52a1\u5df2\u53d1\u9001',
        taskSendFailed: '\u53d1\u9001\u5931\u8d25: {message}',
        commandError: '\u9519\u8bef: {message}',
        selectActionFirst: '\u8bf7\u5148\u9009\u62e9\u4e00\u4e2a\u52a8\u4f5c'
      }
    }
  },
  admin: {
    subscriptions: {
      title: '订阅管理',
      subtitle: '管理订阅分组、模板以及关联的物理节点协议。',
      groups: '分组列表',
      createGroup: '创建分组',
      editGroup: '编辑分组',
      groupName: '分组名称',
      groupNamePlaceholder: '请输入分组名称',
      description: '描述',
      priority: '优先级',
      enabled: '启用',
      disabled: '禁用',
      noDescription: '暂无描述',
      templates: '模板数',
      templatesFor: '模板列表',
      createTemplate: '创建模板',
      editTemplate: '编辑模板',
      copySubscription: '复制订阅链接',
      copyCombinedSubscription: '复制合并订阅',
      preview: '预览',
      previewTitle: '订阅预览',
      format: '格式',
      copyContent: '复制内容',
      download: '下载',
      subscriptionLinks: '订阅链接',
      nodeName: '节点名称',
      nodeNamePlaceholder: '美国节点',
      protocol: '协议',
      server: '服务器',
      port: '端口',
      tls: 'TLS',
      tlsNone: '无',
      tlsReality: 'Reality',
      tlsEnabled: 'TLS',
      status: '状态',
      actions: '操作',
      productionNodes: '物理节点协议',
      manageRelations: '管理关联',
      linkedProtocolsInfo: '已关联到此分组的协议',
      visibilityShown: '显示',
      visibilityHidden: '隐藏',
      protocolOnline: '在线',
      protocolOffline: '下线',
      goToNode: '前往节点管理',
      productionNodesEmpty: '此分组暂无关联的物理节点协议。',
      manageProtocolsTitle: '管理物理节点协议关联',
      manageProtocolsDescription: '勾选要包含在“{group}”分组中的节点协议。只有在“节点管理”中开启“显示在订阅中”的协议才会出现在这里。',
      unknownNode: '未知节点',
      confirmSave: '确认保存',
      transport: '传输',
      sni: 'SNI（服务器名称）',
      realityPublicKey: 'Reality 公钥',
      realityShortId: 'Reality Short ID',
      tlsFingerprint: 'TLS 指纹',
      defaultOption: '默认',
      websocketPath: 'WebSocket 路径',
      flow: 'Flow',
      noneOption: '无',
      productionTable: {
        node: '节点',
        protocol: '协议',
        name: '名称',
        port: '端口',
        visibility: '可见',
        status: '状态',
        actions: '操作'
      },
      protocolPool: {
        node: '节点',
        protocolName: '协议 / 名称',
        port: '端口',
        linkedGroups: '已关联分组'
      },
      formats: {
        auto: '自动（按 User-Agent）',
        v2ray: 'V2Ray（Base64）',
        clash: 'Clash（YAML）',
        stash: 'Stash（YAML）',
        egern: 'Egern（YAML）',
        surge: 'Surge',
        loon: 'Loon',
        shadowrocket: 'ShadowRocket',
        quantumultx: 'QuantumultX',
        json: 'JSON',
        base64json: 'Base64 JSON'
      },
      loadError: '加载订阅数据失败',
      availableProtocolsLoadError: '加载可用协议失败',
      groupProtocolsUpdated: '分组协议关联已更新',
      groupProtocolsUpdateFailed: '更新协议关联失败',
      copyCombinedConfirm: '确定复制该分组的合并订阅内容吗？',
      copied: '已复制',
      copyError: '复制失败',
      copyFallbackNotice: '已使用仅模板回退方案复制合并订阅内容。',
      previewError: '加载预览内容失败',
      confirmDeleteGroup: '确定删除这个订阅分组吗？',
      groupDeleted: '订阅分组已删除',
      deleteError: '删除失败',
      groupSaved: '订阅分组已保存',
      saveError: '保存失败',
      confirmDeleteTemplate: '确定删除这个订阅模板吗？',
      templateDeleted: '订阅模板已删除',
      updateError: '更新失败',
      templateSaved: '订阅模板已保存',
      stats: {
        totalGroups: '总分组数',
        totalUsers: '总用户数',
        totalTemplates: '总模板数',
        totalTraffic: '总已用流量',
        groupUsage: '分组使用情况',
        groupName: '分组',
        users: '用户数',
        enabledUsers: '活跃用户',
        templates: '模板数',
        protocols: '协议数',
        onlineNodes: '在线节点',
        trafficUsed: '已用流量',
        plans: '关联套餐',
        view: '查看',
        empty: '暂无订阅分组。'
      }
    },
    nodes: {
      title: '节点管理',
      authKeys: '授权密钥',
      addNode: '添加节点',
      stats: {
        total: '总节点',
        online: '在线',
        offline: '离线',
        pending: '待激活'
      },
      table: {
        name: '名称',
        address: '地址',
        status: '状态',
        runtimeHealthy: '运行时正常',
        runtimeUnhealthy: '运行时异常',
        parent: '父节点',
        protocols: '协议数',
        traffic: '今日流量',
        monthlyQuota: '月流量限额',
        quotaExceeded: '本月已超限',
        lastHeartbeat: '最后心跳',
        actions: '操作',
        loading: '加载中...',
        empty: '暂无节点'
      },
      actions: {
        manageProtocols: '管理协议',
        protocols: '协议',
        syncReload: '\u540c\u6b65 / Reload',
        syncing: '\u540c\u6b65\u4e2d...',
        logs: '日志',
        deployParents: '部署父节点',
        edit: '编辑',
        delete: '删除',
        cancel: '取消',
        save: '保存',
        saving: '保存中...',
        authKey: '授权密钥',
        refreshLogs: '刷新日志',
        loadingLogs: '加载日志中...'
      },
      pagination: {
        previous: '上一页',
        next: '下一页'
      },
      nodeModal: {
        titleCreate: '添加节点',
        titleEdit: '编辑节点',
        parentNone: '无(作为根/落地节点)',
        parentHint: '选择上级节点可组成多级中转链路; 本节点转发的流量会同时累加到自己和每一级上级节点。',
        fields: {
          name: '节点名称 *',
          address: '节点地址 *',
          tags: '标签（逗号分隔）',
          rate: '节点倍率',
          sort: '排序',
          status: '状态',
          parent: '父节点(上级/落地节点)',
          monthlyLimit: '月流量限额 (GB)',
          monthlyResetDay: '每月重置日 (1-28)'
        },
        placeholders: {
          name: '输入节点名称',
          address: 'IP 或域名',
          tags: '香港,IEPL,高速',
          rate: '1.0',
          sort: '0',
          monthlyLimit: '留空则不限',
          monthlyResetDay: '1'
        }
      },
      authKeyModal: {
        title: '节点授权密钥',
        hint: `将此密钥配置到 ${AGENT_NAME} 的 config.json 中，节点启动后会自动注册到控制面。同一密钥可用于多台节点注册。`,
        noKey: '尚未生成授权密钥，请先创建授权密钥。',
        copy: '复制密钥',
        copyConfig: '复制配置',
        configHint: `将此配置粘贴到 ${AGENT_NAME} 的 config.json 中，并将 <auth_key> 替换为上方密钥值。`,
        registeredCount: '已注册节点数',
        copied: '已复制'
      },
      deployModal: {
        title: '父节点部署助手',
        summaryEyebrow: '部署',
        summaryTitle: '已为 {count} 个父节点准备部署信息',
        summaryText: '这里会生成 legacy 父节点的一键部署清单、group_vars 和命令。你输入的 SSH 密码或私钥路径只保留在当前浏览器会话里。',
        warning: '此助手基于 config/deploy/ansible/nodes/deploy_v2bx.yml 生成内容，适合父节点批量接管。子节点如有单独需求，仍建议按需单独处理。',
        loading: '正在加载父节点凭据...',
        empty: '暂无可部署的父节点',
        commandsLabel: '部署命令',
        authModes: {
          password: '密码',
          key: '私钥'
        },
        fields: {
          panelApiHost: '面板 API 地址',
          grpcHost: 'gRPC 地址',
          grpcServerName: 'gRPC ServerName',
          amd64BinaryPath: 'AMD64 二进制路径',
          arm64BinaryPath: 'ARM64 二进制路径',
          coreType: '核心类型',
          grpcUseTLS: 'gRPC 使用 TLS'
        },
        table: {
          alias: '别名',
          node: '父节点',
          sshHost: 'SSH 主机',
          port: 'SSH 端口',
          user: 'SSH 用户',
          arch: '架构',
          authMode: '认证方式',
          authValue: '密码 / 私钥路径'
        },
        placeholders: {
          password: 'sshpass 使用的密码',
          privateKey: '~/.ssh/id_ed25519'
        }
      },
      protocolModal: {
        title: '协议管理 - {name}',
        addProtocol: '添加协议',
        table: {
          type: '协议类型',
          port: '端口',
          status: '状态',
          actions: '操作'
        },
        status: {
          enabled: '启用',
          disabled: '禁用'
        },
        empty: '暂无协议配置'
      },
      logModal: {
        title: '运行日志 - {name}',
        loading: '正在加载节点日志...',
        empty: '该节点暂无运行日志',
        filters: {
          allLevels: '全部级别',
          sourcePlaceholder: '来源模块',
          searchPlaceholder: '搜索日志内容 / 来源 / trace id'
        },
        levels: {
          debug: '调试',
          info: '信息',
          warning: '警告',
          error: '错误'
        },
        table: {
          time: '时间',
          level: '级别',
          source: '来源',
          message: '内容',
          fields: '结构化字段'
        }
      },
      protocolForm: {
        titleCreate: '添加协议',
        titleEdit: '编辑协议',
        templateLibrary: '协议模板库',
        tabs: {
          json: 'JSON',
          visual: '可视化配置'
        },
        fields: {
          type: '协议类型 *',
          port: '监听端口 *',
          tls: 'TLS 模式',
          transport: '传输层协议',
          settings: '协议设置 (JSON)',
          tlsSettings: 'TLS 设置 (JSON)',
          realitySettings: 'Reality 设置 (JSON)',
          transportSettings: '传输层设置 (JSON)'
        },
        placeholders: {
          port: '443'
        },
        jsonActions: {
          format: '格式化',
          copy: '复制',
          fromTemplate: '从模板加载'
        },
        jsonStatus: {
          valid: 'JSON 有效',
          invalid: 'JSON 无效'
        },
        wireguard: {
          sections: {
            access: 'WireGuard 接入',
            relay: '双机中转',
            networkPolicy: '入口网络路径（可选）'
          },
          fields: {
            cidr: 'Peer CIDR',
            serverAddress: '入口接口地址',
            serverPrivateKey: '服务端私钥',
            serverPublicKey: '服务端公钥',
            mtu: 'MTU',
            dns: 'DNS 服务器',
            allowedIps: 'Allowed IPs',
            role: '节点角色',
            tunnelType: '入口到出口隧道',
            wssCompat: 'WSS 兼容模式',
			wssPath: 'WSS 路径',
			wssSecure: '校验出口证书',
			wssServerName: 'WSS 服务名称（SNI）',
			wssCaFile: '入口机 WSS CA 证书文件',
			wssCertFile: '出口机 WSS 证书文件',
			wssKeyFile: '出口机 WSS 私钥文件',
            relayServer: '出口中转主机',
            relayServerPort: '出口中转端口',
            tunPort: 'GOST TUN 端口',
            tunName: 'GOST TUN 名称',
            entryTunAddress: '入口 TUN 地址',
            exitTunAddress: '出口 TUN 地址',
            outboundIface: '出口网卡',
            exitNat: '启用出口 NAT',
            routingTable: '路由表',
            routingPriority: '路由优先级',
            networkPolicyEnabled: '启用多线路故障转移',
            networkPath: '网络路径',
            pathName: '路径名称',
            pathInterface: '网卡',
            pathSource: '源 IP',
            pathGateway: '网关',
            pathPriority: '优先级（越小越优先）',
            healthInterval: '探测间隔（秒）',
            healthTimeout: '探测超时（秒）',
            failureThreshold: '失败阈值',
            failbackDelay: '主线恢复等待（秒）'
          },
          actions: {
            generateKeypair: '生成密钥对',
            addNetworkPath: '添加网络路径'
          },
          values: {
            entry: '国内入口',
            exit: '海外出口'
          },
          hints: {
			wssCompat: '默认使用 QUIC。WSS 会校验出口证书，只作为 UDP 中转受阻或不稳定时的兼容模式。',
            networkPolicy: '仅接管到出口中转 IP 的连接，并为每个源 IP 建立独立回程路由。普通节点无需启用；出口主机必须填写 IP 地址。'
          }
        },
        enable: '启用协议（节点端运行）',
        show: '显示在订阅协议池中'
      },
      statusText: {
        pending: '待激活',
        online: '在线',
        offline: '离线',
        disabled: '禁用',
        unknown: '未知'
      },
      tlsModes: {
        none: '无 TLS',
        standard: '标准 TLS',
        reality: 'Reality（推荐）'
      },
      transports: {
        tcp: 'TCP',
        ws: 'WebSocket',
        grpc: 'gRPC',
        quic: 'QUIC',
        h2: 'HTTP/2'
      },
      relativeTime: {
        justNow: '刚刚',
        minutesAgo: '{count} 分钟前',
        hoursAgo: '{count} 小时前'
      },
      messages: {
        requiredFields: '请填写必填字段',
        saveFailed: '保存失败: {message}',
        syncSuccess: '\u8282\u70b9 "{name}" \u5df2\u63a5\u6536\u540c\u6b65\u64cd\u4f5c',
        syncFailed: '\u540c\u6b65\u5931\u8d25: {message}',
        deleteNodeConfirm: '确定要删除节点 "{name}" 吗？',
        deleteFailed: '删除失败: {message}',
        deleteProtocolConfirm: '确定要删除此协议吗？',
        generateFailed: '生成失败: {message}',
        deleteAuthKeyConfirm: '确定要删除此授权密钥吗？',
        deployLoadFailed: '加载父节点凭据失败',
        copied: '已复制到剪贴板',
        copyFailed: '复制失败: {message}',
        invalidJson: 'JSON 格式无效',
        quotaExceededBanner: '有 {count} 个节点本月流量已超限，仅作提示，不会自动限制'
      }
    }
  },
  adminNotifications: {
    title: '通知管理',
    subtitle: '管理通知模板、SMTP 投递和发送日志。',
    tabs: {
      templates: '通知模板',
      email: '邮件配置',
      logs: '发送日志'
    },
    actions: {
      createTemplate: '新增模板',
      edit: '编辑',
      delete: '删除',
      search: '搜索',
      sendTest: '发送测试邮件'
    },
    templates: {
      table: {
        id: 'ID',
        name: '名称',
        type: '类型',
        event: '触发事件',
        status: '状态',
        actions: '操作'
      },
      empty: '暂无通知模板'
    },
    email: {
      title: 'SMTP 配置',
      fields: {
        host: 'SMTP 服务器',
        port: '端口',
        username: '用户名',
        password: '密码',
        fromName: '发件人名称',
        fromAddress: '发件人地址',
        encryption: '启用 TLS 加密'
      },
      placeholders: {
        host: 'smtp.example.com',
        port: '465',
        username: "your{'@'}email.com",
        password: '请输入 SMTP 密码',
        fromName: CONTROL_NAME,
        fromAddress: "noreply{'@'}example.com"
      }
    },
    logs: {
      filters: {
        allTypes: '全部类型',
        allStatuses: '全部状态'
      },
      table: {
        id: 'ID',
        type: '类型',
        recipient: '接收者',
        title: '标题',
        status: '状态',
        sentAt: '发送时间'
      },
      empty: '暂无发送日志'
    },
    modal: {
      createTitle: '新增模板',
      editTitle: '编辑模板',
      fields: {
        name: '名称',
        type: '类型',
        event: '触发事件',
        title: '标题模板',
        content: '内容模板',
        enabled: '启用'
      },
      placeholders: {
        name: '模板名称',
        title: '支持变量: {username}, {site_name}',
        content: '支持变量: {username}, {email}, {expire_time}'
      }
    },
    testModal: {
      title: '发送测试邮件',
      fields: {
        recipient: '收件人地址'
      },
      placeholders: {
        recipient: "test{'@'}example.com"
      },
      actions: {
        send: '发送'
      }
    },
    types: {
      email: '邮件',
      telegram: 'Telegram',
      webhook: 'Webhook'
    },
    events: {
      userRegister: '用户注册',
      userLogin: '用户登录',
      userExpire: '用户到期',
      userTrafficLow: '流量不足',
      orderPaid: '订单支付',
      ticketReply: '工单回复',
      nodeOffline: '节点离线',
      nodeOnline: '节点上线'
    },
    status: {
      enabled: '启用',
      disabled: '禁用',
      pending: '待发送',
      success: '成功',
      failed: '失败'
    },
    testPayload: {
      subject: '测试邮件',
      content: '这是一封测试邮件。如果您收到此邮件，说明邮件配置正确。'
    },
    messages: {
      fetchTemplatesFailed: '加载通知模板失败',
      fetchLogsFailed: '加载通知日志失败',
      fetchEmailConfigFailed: '加载邮件配置失败',
      templateSaveSuccess: '模板保存成功',
      templateSaveFailed: '模板保存失败: {message}',
      templateSaveFailedShort: '保存失败',
      deleteConfirm: '确定删除模板 "{name}"？',
      deleteFailed: '删除模板失败: {message}',
      deleteFailedShort: '删除失败',
      emailSaveSuccess: '邮件配置已保存',
      emailSaveFailed: '邮件配置保存失败: {message}',
      emailSaveFailedShort: '保存失败',
      testRecipientRequired: '请输入收件人邮箱地址',
      testSendSuccess: '测试邮件发送成功',
      testSendFailed: '测试邮件发送失败: {message}',
      testSendFailedShort: '发送失败'
    }
  },
  adminPayment: {
    title: '支付网关管理',
    subtitle: '管理支付渠道、支付记录和统计数据。',
    currencySymbol: '¥',
    tabs: {
      gateways: '支付网关',
      records: '支付记录',
      stats: '统计数据'
    },
    actions: {
      createGateway: '新增网关',
      enable: '启用',
      disable: '禁用',
      edit: '编辑',
      delete: '删除',
      search: '搜索',
      details: '详情'
    },
    gateways: {
      table: {
        id: 'ID',
        name: '名称',
        type: '类型',
        feeRate: '手续费率',
        minAmount: '最小金额',
        maxAmount: '最大金额',
        status: '状态',
        actions: '操作'
      },
      empty: '暂无支付网关'
    },
    records: {
      filters: {
        allStatuses: '全部状态',
        allTypes: '全部类型'
      },
      table: {
        id: 'ID',
        tradeNo: '交易号',
        userId: '用户 ID',
        gateway: '网关',
        amount: '金额',
        status: '状态',
        createdAt: '创建时间',
        actions: '操作'
      },
      detail: {
        title: '支付详情',
        tradeNo: '交易号',
        amount: '金额',
        status: '状态'
      },
      empty: '暂无支付记录'
    },
    stats: {
      totalAmount: '总收入',
      totalOrders: '总订单数',
      successOrders: '成功订单',
      successRate: '成功率',
      gatewayDistribution: '支付渠道分布',
      empty: '暂无渠道统计'
    },
    modal: {
      createTitle: '新增网关',
      editTitle: '编辑网关',
      fields: {
        name: '名称',
        type: '类型',
        feeRate: '手续费率',
        minAmount: '最小金额',
        maxAmount: '最大金额',
        configJson: '配置 (JSON)'
      },
      placeholders: {
        name: '网关名称',
        feeRate: '例如: 0.01 = 1%',
        minAmount: '最小支付金额',
        maxAmount: '最大支付金额',
        configJson: '{"app_id": "", "private_key": ""}'
      }
    },
    types: {
      alipay: '支付宝',
      wechat: '微信支付',
      stripe: 'Stripe',
      usdt: 'USDT',
      epay: 'EPay'
    },
    status: {
      enabled: '启用',
      disabled: '禁用',
      pending: '待支付',
      paid: '已支付',
      failed: '失败',
      refunded: '已退款'
    },
    messages: {
      fetchGatewaysFailed: '加载支付网关失败',
      fetchRecordsFailed: '加载支付记录失败',
      fetchStatsFailed: '加载支付统计失败',
      invalidConfigJson: '配置 JSON 格式错误',
      gatewaySaveSuccess: '网关保存成功',
      gatewaySaveFailed: '网关保存失败: {message}',
      gatewaySaveFailedShort: '保存失败',
      toggleFailed: '切换网关状态失败: {message}',
      toggleFailedShort: '操作失败',
      deleteConfirm: '确定删除网关 "{name}"？',
      deleteFailed: '删除网关失败: {message}',
      deleteFailedShort: '删除失败'
    }
  },
  adminMfa: {
    title: '\u591a\u56e0\u7d20\u8ba4\u8bc1\u7ba1\u7406',
    subtitle: '\u914d\u7f6e\u5168\u5c40 MFA \u7b56\u7565',
    config: {
      title: '\u5168\u5c40\u914d\u7f6e',
      enabled: '\u542f\u7528\u591a\u56e0\u7d20\u8ba4\u8bc1',
      enabledHelp: '\u542f\u7528\u540e\uff0c\u7528\u6237\u53ef\u9009\u62e9\u5f00\u542f MFA \u4fdd\u62a4\u8d26\u6237\u5b89\u5168',
      required: '\u5f3a\u5236\u542f\u7528 MFA',
      requiredHelp: '\u5f3a\u5236\u6240\u6709\u7528\u6237\u542f\u7528 MFA\uff0c\u5426\u5219\u65e0\u6cd5\u4f7f\u7528\u670d\u52a1',
      methods: '\u652f\u6301\u7684\u8ba4\u8bc1\u65b9\u5f0f',
      backupCodesCount: '\u5907\u7528\u7801\u6570\u91cf',
      backupCodesHelp: '\u7528\u6237\u542f\u7528 MFA \u65f6\u751f\u6210\u7684\u5907\u7528\u7801\u6570\u91cf',
      maxAttempts: '\u767b\u5f55\u5c1d\u8bd5\u9650\u5236',
      maxAttemptsHelp: '\u8d85\u8fc7\u9650\u5236\u5c06\u4e34\u65f6\u9501\u5b9a\u8d26\u6237',
      lockoutDuration: '\u9501\u5b9a\u65f6\u957f (\u5206\u949f)',
      lockoutDurationHelp: '\u767b\u5f55\u5931\u8d25\u8d85\u8fc7\u9650\u5236\u540e\u7684\u9501\u5b9a\u65f6\u95f4'
    },
    methods: {
      totp: 'TOTP (Google Authenticator / Authy)',
      sms: '\u77ed\u4fe1\u9a8c\u8bc1\u7801',
      email: '\u90ae\u7bb1\u9a8c\u8bc1\u7801'
    },
    info: {
      title: '\ud83d\udca1 \u4f7f\u7528\u8bf4\u660e',
      totpTitle: 'TOTP \u8ba4\u8bc1',
      totpBody: '\u57fa\u4e8e\u65f6\u95f4\u7684\u4e00\u6b21\u6027\u5bc6\u7801\uff0c\u7528\u6237\u53ef\u4f7f\u7528 Google Authenticator\u3001Authy \u7b49\u5e94\u7528\u626b\u63cf\u4e8c\u7ef4\u7801\u7ed1\u5b9a\u3002',
      backupTitle: '\u5907\u7528\u7801',
      backupBody: '\u5f53\u7528\u6237\u65e0\u6cd5\u4f7f\u7528\u8ba4\u8bc1\u5668\u65f6\uff0c\u53ef\u4f7f\u7528\u5907\u7528\u7801\u767b\u5f55\u3002\u6bcf\u4e2a\u5907\u7528\u7801\u53ea\u80fd\u4f7f\u7528\u4e00\u6b21\u3002',
      lockoutTitle: '\u8d26\u6237\u9501\u5b9a',
      lockoutBody: '\u8fde\u7eed\u591a\u6b21 MFA \u9a8c\u8bc1\u5931\u8d25\u5c06\u89e6\u53d1\u8d26\u6237\u9501\u5b9a\uff0c\u9632\u6b62\u66b4\u529b\u7834\u89e3\u3002',
      userOpsTitle: '\u7528\u6237\u7aef\u64cd\u4f5c',
      userOpsBody: '\u7528\u6237\u53ef\u5728\u300c\u5b89\u5168\u8bbe\u7f6e\u300d\u9875\u9762\u81ea\u884c\u7ba1\u7406 MFA\uff0c\u5305\u62ec\u542f\u7528\u3001\u7981\u7528\u3001\u91cd\u65b0\u751f\u6210\u5907\u7528\u7801\u3002'
    },
    messages: {
      fetchFailed: '\u83b7\u53d6 MFA \u914d\u7f6e\u5931\u8d25',
      saveSuccess: '\u4fdd\u5b58\u6210\u529f',
      saveFailed: '\u4fdd\u5b58\u5931\u8d25: {message}',
      saveFailedShort: '\u4fdd\u5b58\u5931\u8d25'
    }
  },
  adminPlans: {
    title: '套餐管理',
    subtitle: '管理订阅套餐、流量额度、速率和设备限制。',
    table: {
      name: '名称',
      transfer: '流量（GB）',
      limits: '限制',
      monthPrice: '月付价格（分）',
      subscriptionGroups: '订阅分组',
      actions: '操作'
    },
    actions: {
      create: '新增套餐',
      edit: '编辑',
      delete: '删除',
      assign: '分配',
      manageGroups: '管理分组',
      removeGroup: '移除分组'
    },
    empty: {
      noData: '暂无套餐'
    },
    planModal: {
      createTitle: '新增套餐',
      editTitle: '编辑套餐',
      fields: {
        name: '套餐名称',
        transfer: '流量额度（GB）',
        speedLimit: '速率限制（Mbps，0 为不限）',
        deviceLimit: '设备限制（0 为不限）',
        monthPrice: '月付价格（分）'
      },
      placeholders: {
        name: '请输入套餐名称'
      }
    },
    assignModal: {
      title: '分配套餐',
      fields: {
        userId: '用户 ID',
        expireAt: '到期时间（Unix 秒）'
      },
      placeholders: {
        userId: '请输入用户 ID'
      }
    },
    groupModal: {
      title: '套餐分组 - {name}',
      description: '选择该套餐可访问的订阅分组。',
      empty: '暂无订阅分组',
      noDescription: '无描述',
      selectedShort: '已选'
    },
    labels: {
      noSpeedLimit: '不限速',
      noDeviceLimit: '不限设备',
      speedLimitMbps: '{value} Mbps',
      deviceLimitCount: '{value} 台'
    },
    messages: {
      loadFailed: '加载套餐失败',
      loadGroupsFailed: '加载订阅分组失败',
      deleteConfirm: '确定删除该套餐吗？',
      deleteFailed: '删除失败：{message}',
      deleteFailedShort: '删除失败',
      nameRequired: '请填写套餐名称',
      saveFailed: '保存失败：{message}',
      saveFailedShort: '保存失败',
      userIdRequired: '请填写用户 ID',
      assignSuccess: '分配成功',
      assignFailed: '分配失败：{message}',
      assignFailedShort: '分配失败',
      toggleGroupFailed: '切换分组失败：{message}',
      toggleGroupFailedShort: '切换分组失败',
      removeGroupConfirm: '确定移除该订阅分组吗？',
      removeGroupFailed: '移除分组失败：{message}',
      removeGroupFailedShort: '移除失败'
    }
  },
  adminUsers: {
    title: '用户管理',
    subtitle: '管理所有注册用户。',
    stats: {
      totalUsers: '总用户数',
      activeUsers: '有效用户',
      expiredUsers: '已过期',
      bannedUsers: '已封禁'
    },
    filters: {
      searchEmail: '搜索邮箱...',
      allStatus: '全部状态'
    },
    table: {
      id: 'ID',
      email: '邮箱',
      plan: '套餐',
      traffic: '流量',
      limits: '限制',
      expireAt: '到期时间',
      status: '状态',
      createdAt: '注册时间',
      actions: '操作'
    },
    status: {
      active: '有效',
      expired: '已过期',
      banned: '已封禁'
    },
    actions: {
      search: '搜索',
      addUser: '新增用户',
      editUser: '编辑用户',
      manageTunnel: '管理隧道授权',
      manageTunnelShort: '隧道',
      ban: '封禁',
      unban: '解封',
      resetTraffic: '重置流量',
      resetShort: '重置',
      copySubscribe: '复制订阅链接',
      copySubscribeShort: '复制订阅',
      resetSubscribe: '重置订阅链接 (旧链接失效)',
      resetSubscribeShort: '重置订阅',
      viewTraffic: '查看最近 30 天流量',
      viewTrafficShort: '流量详情'
    },
    empty: {
      noData: '暂无数据'
    },
    pagination: {
      prev: '上一页',
      next: '下一页',
      info: '第 {page} / {totalPages} 页'
    },
    editModal: {
      title: '编辑用户',
      fields: {
        email: '邮箱',
        balance: '余额（分）',
        transfer: '流量限制（字节）',
        speedLimit: '速率限制（Mbps，0 为不限）',
        deviceLimit: '设备限制（0 为不限）',
        groupId: '订阅分组',
        expiredAt: '到期时间（Unix 秒）',
        flowResetTime: '流量重置日',
        remark: '备注'
      },
      groupOptions: {
        unassigned: '未分配'
      }
    },
    createModal: {
      title: '新增用户',
      creating: '创建中...',
      fields: {
        email: '邮箱',
        password: '密码',
        userType: '用户类型',
        groupId: '订阅分组',
        transferEnable: '流量限制（字节）',
        speedLimit: '速率限制（Mbps，0 为不限）',
        deviceLimit: '设备限制（0 为不限）',
        flowResetTime: '流量重置日'
      },
      placeholders: {
        email: '请输入邮箱',
        password: '请输入密码（至少 6 位）',
        transferEnable: '留空则使用默认值 0',
        speedLimit: '0 表示不限速',
        deviceLimit: '0 表示不限设备'
      },
      groupOptions: {
        unassigned: '未分配'
      },
      userTypes: {
        normal: '普通用户',
        admin: '管理员'
      }
    },
    tunnelModal: {
      title: '隧道授权 - {email}',
      sections: {
        form: '授权表单',
        list: '当前授权列表'
      },
      fields: {
        tunnel: '隧道',
        tunnelReadonlyHint: '（编辑时不可修改）',
        status: '状态',
        flowQuota: '流量配额',
        numQuota: '数量配额',
        expTime: '到期时间',
        flowResetTime: '流量重置时间',
        rateLimit: '限速规则'
      },
      options: {
        noAssignableTunnel: '暂无可分配隧道',
        selectTunnel: '请选择隧道',
        selectTunnelFirst: '请先选择隧道',
        noRateLimitRules: '当前隧道暂无限速规则'
      },
      actions: {
        cancelEdit: '取消编辑',
        submitting: '提交中...',
        updateGrant: '更新授权',
        addGrant: '新增授权'
      },
      table: {
        id: 'ID',
        tunnel: '隧道',
        status: '状态',
        flow: '流量',
        num: '数量',
        expireAt: '到期',
        reset: '重置',
        usedFlow: '已用流量',
        rateLimit: '限速'
      },
      empty: '暂无授权'
    },
    resetFlow: {
      userTitle: '重置用户流量',
      userMessage: '确认将用户 {email} 的已用流量清零吗？',
      tunnelTitle: '重置隧道授权流量',
      tunnelMessage: '确认将隧道授权 #{id} 的已用流量清零吗？',
      usedFlow: '当前已用',
      quota: '当前配额',
      resetting: '重置中...',
      confirmAction: '确认重置'
    },
    trafficModal: {
      title: '流量详情 - {email}',
      subtitle: '最近 30 天每日汇总和每小时明细。',
      refresh: '刷新',
      loading: '加载中...',
      dailyTitle: '每日流量',
      hourlyTitle: '每小时流量',
      empty: '暂无流量记录',
      fetchFailed: '加载用户流量失败',
      summary: {
        total30d: '30 天总流量',
        dailyPeak: '单日峰值',
        hourlyPeak: '单小时峰值'
      },
      table: {
        date: '日期',
        hour: '小时',
        traffic: '流量'
      }
    },
    labels: {
      admin: '管理员',
      noLimit: '不限速',
      noSpeedLimit: '不限速',
      noDeviceLimit: '不限设备',
      speedLimitMbps: '{value} Mbps',
      deviceLimitCount: '{value} 台',
      noReset: '不重置',
      monthlyDay: '每月第 {day} 天',
      permanent: '永久'
    },
    messages: {
      actionFailed: '操作失败',
      fillEmailPassword: '请填写邮箱和密码',
      passwordTooShort: '密码长度至少 6 位',
      userCreated: '用户创建成功',
      createFailed: '创建失败',
      fetchUsersFailed: '获取用户列表失败',
      fetchStatsFailed: '获取统计失败',
      saveFailed: '保存失败：{message}',
      confirmBan: '确认封禁用户 {email} 吗？',
      confirmUnban: '确认解封用户 {email} 吗？',
      fetchTunnelListFailed: '获取隧道列表失败',
      fetchSpeedLimitFailed: '获取限速规则失败',
      fetchTunnelGrantFailed: '获取隧道授权失败',
      selectTunnelFirst: '请选择隧道',
      tunnelAlreadyAssigned: '该隧道已授权给当前用户',
      grantUpdateFailed: '授权更新失败',
      grantCreateFailed: '授权创建失败',
      grantUpdated: '授权更新成功',
      grantCreated: '授权创建成功',
      grantActionFailed: '授权操作失败',
      confirmDeleteGrant: '确认删除隧道授权 #{id} 吗？',
      grantDeleteFailed: '删除授权失败',
      resetFailed: '重置失败',
      userFlowReset: '用户流量已重置',
      tunnelFlowReset: '隧道流量已重置',
      noToken: '该用户没有订阅 token',
      subscribeCopied: '订阅链接已复制到剪贴板',
      copyFailed: '复制失败',
      copyManual: '自动复制失败，请手动复制以下订阅链接：',
      resetSubscribeConfirm: '确认重置用户 {email} 的订阅链接吗？旧链接将立即失效，用户需重新导入。',
      resetSubscribeSuccess: '订阅链接已重置',
      resetSubscribeFailed: '重置订阅失败'
    }
  },
  control: {
    actions: { refresh: '刷新', refreshing: '刷新中...' },
    tabs: { plugins: '插件', scopes: '作用域', topologies: '拓扑', operations: '操作' },
    table: {
      plugin: '插件', publisher: '发布者', installation: '安装目标', desiredVersion: '期望版本', observedVersion: '实际版本', state: '状态',
      scope: '作用域', owner: '所有者', description: '说明', topology: '拓扑', activeRevision: '激活 revision',
      operation: '操作', revision: 'revision', deadline: '截止时间'
    },
    states: { catalogued: '已登记', loading: '正在加载控制状态...' },
    empty: { plugins: '暂无插件', scopes: '暂无服务作用域', topologies: '暂无拓扑', operations: '暂无操作' },
    errors: { load: '无法加载控制状态' }
  },
  legacy
}
