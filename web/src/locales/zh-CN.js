import ui from './modules/zh-CN/ui'
import shell from './modules/zh-CN/shell'
import userPages from './modules/zh-CN/userPages'
import { CONTROL_NAME } from '../constants/brand'

const legacy = {
  'V2Board Admin': `${CONTROL_NAME} 管理台`,
  'Admin': '工作室管理台',
  'Overview': '概览',
  'Dashboard': '仪表盘',
  'Forwards': '流量转发',
  'Tunnels': '隧道管理',
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
  'Refresh': '刷新',
  'Save': '保存',
  'Cancel': '取消',
  'Close': '关闭',
  'Create': '创建',
  'Delete': '删除',
  'Preview': '预览',
  'Copy': '复制',
  'Download': '下载',
  'Open': '打开',
  'Details': '详情',
  'Back': '返回',
  'Verify': '验证',
  'Remove': '移除',
  'Enabled': '启用',
  'Disabled': '禁用',
  'Status': '状态',
  'Description': '描述',
  'Priority': '优先级',
  'Format': '格式',
  'Traffic': '流量',
  'Upload': '上传',
  'Download': '下载',
  'Reply': '回复',
  'Loading...': '加载中...',
  'No data': '暂无数据',
  'All': '全部',
  'Active': '有效',
  'Expired': '已过期',
  'Pending': '待支付',
  'Paid': '已支付',
  'Unknown': '未知',
  'Closed': '已关闭',
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
  ...ui,
  ...shell,
  ...userPages,
  common: {
    locale: {
      label: '语言',
      switch: '切换语言',
      zhCN: '简体中文',
      en: 'English',
      zhShort: '中',
      enShort: 'EN'
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
      subject: '主题',
      priority: '优先级',
      message: '内容'
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
      paymentPending: '支付功能正在集成中，敬请期待'
    }
  },
  pageTitles: {
    auth: {
      login: '登录'
    },
    user: {
      dashboard: '概览',
      subscribe: '订阅',
      knowledge: '帮助中心',
      tickets: '工单',
      plans: '套餐',
      orders: '订单'
    },
    admin: {
      dashboard: '仪表盘',
      monitor: '流量与监控',
      users: '用户',
      nodes: '节点',
      subscriptions: '订阅分组',
      orders: '订单',
      plans: '套餐',
      subscriptionTemplates: '订阅模板',
      tickets: '工单',
      coupons: '优惠券',
      knowledge: '帮助中心内容',
      forward: '流量转发管理',
      forwardTunnel: '隧道管理',
      forwardLimit: '限速管理',
      forwardAnsibleMachines: 'Ansible 机器',
      forwardNodes: 'NodeX 拓扑',
      forwardLocal: '本地运行时',
      forwardNodeX: 'NodeX 运行时',
      forwardAgents: 'NodeX Agents',
      forwardV4Overview: '转发概览',
      forwardV4Routes: '转发路由',
      forwardV4RouteNew: '新建路由',
      forwardV4Route: '路由详情',
      forwardV4RouteEdit: '编辑路由',
      forwardV4Nodes: '转发节点清单',
      forwardV4Node: '节点详情',
      agentTransports: 'Agent 连接方式',
      control: '控制内核',
      plugins: '插件中心',
      routeModes: '路由模式',
      deployments: '部署编排',
      accessGroups: '访问组',
      payment: '支付',
      notifications: '通知',
      invite: '邀请返佣',
      inviteCodes: '邀请码',
      system: '系统设置',
      security: '安全',
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
    // The shells' strings live under shell.* (locales/modules/*/shell.js);
    // these remain for the login page until it is redesigned (U5).
    admin: {
      brand: CONTROL_NAME,
      badge: '工作室管理台',
      sections: {
        overview: '概览',
        userManagement: '用户管理',
        system: '系统'
      },
      nav: {
        dashboard: '仪表盘',
        users: '用户管理',
        system: '系统管理'
      }
    },
    user: {
      brand: CONTROL_NAME
    }
  },
  legacy
}
