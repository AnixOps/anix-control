import ui from './modules/en/ui'
import shell from './modules/en/shell'
import userPages from './modules/en/userPages'
import { CONTROL_NAME } from '../constants/brand'

const legacy = {
  '管理端': 'Studio Console',
  '概览': 'Overview',
  '仪表盘': 'Dashboard',
  '流量转发': 'Forwards',
  '用户管理': 'Users',
  '订单管理': 'Orders',
  '工单管理': 'Tickets',
  '节点管理': 'Nodes',
  '订阅管理': 'Subscriptions',
  '套餐管理': 'Plans',
  '优惠券管理': 'Coupons',
  '邀请返利管理': 'Invite',
  '支付网关管理': 'Payment',
  '通知管理': 'Notifications',
  '知识库管理': 'Knowledge',
  '系统管理': 'System',
  '刷新': 'Refresh',
  '保存': 'Save',
  '取消': 'Cancel',
  '关闭': 'Close',
  '创建': 'Create',
  '编辑': 'Edit',
  '删除': 'Delete',
  '预览': 'Preview',
  '复制': 'Copy',
  '下载': 'Download',
  '详情': 'Details',
  '返回': 'Back',
  '提交': 'Submit',
  '验证': 'Verify',
  '移除': 'Remove',
  '启用': 'Enabled',
  '禁用': 'Disabled',
  '状态': 'Status',
  '描述': 'Description',
  '优先级': 'Priority',
  '格式': 'Format',
  '流量': 'Traffic',
  '流量使用': 'Traffic Usage',
  '上传': 'Upload',
  '下载': 'Download',
  '流量剩余': 'Traffic Remaining',
  '到期时间': 'Expires At',
  '更新时间': 'Updated At',
  '订单详情': 'Order Details',
  '回复': 'Reply',
  '提交工单': 'Submit Ticket',
  '关闭工单': 'Close Ticket',
  '全部': 'All',
  '永久': 'Permanent',
  '有效': 'Active',
  '已过期': 'Expired',
  '待支付': 'Pending',
  '已支付': 'Paid',
  '已取消': 'Cancelled',
  '已完成': 'Completed',
  '未知': 'Unknown',
  '待处理': 'Open Ticket',
  '已回复': 'Answered',
  '已关闭': 'Closed',
  '当前状态': 'Current state',
  '是': 'Yes',
  '否': 'No'
  ,
  'Enable NodeX forward runtime mode': 'Enable NodeX forward runtime mode',
  'Forward runtime backend': 'Forward runtime backend',
  'Preferred local ansible backend': 'Preferred local ansible backend',
  'Forward runtime NodeX base URL': 'Forward runtime NodeX base URL',
  'Forward runtime NodeX token': 'Forward runtime NodeX token',
  'Forward runtime NodeX timeout': 'Forward runtime NodeX timeout',
  'Forward runtime ansible config': 'Forward runtime ansible config',
  'Forward ansible inventory path': 'Forward ansible inventory path',
  'Forward ansible apply playbook path': 'Forward ansible apply playbook path',
  'Forward ansible remove playbook path': 'Forward ansible remove playbook path',
  'Forward ansible become flag': 'Forward ansible become flag',
  'Forward ansible extra vars JSON': 'Forward ansible extra vars JSON'
}

export default {
  ...ui,
  ...shell,
  ...userPages,
  common: {
    locale: {
      label: 'Language',
      switch: 'Switch language',
      zhCN: 'Simplified Chinese',
      en: 'English',
      zhShort: '中',
      enShort: 'EN'
    },
    a11y: {
      skipToContent: 'Skip to main content',
      openNavigation: 'Open navigation menu',
      closeNavigation: 'Close navigation menu'
    },
    actions: {
      cancel: 'Cancel',
      close: 'Close',
      copy: 'Copy',
      download: 'Download',
      preview: 'Preview',
      refresh: 'Refresh',
      save: 'Save',
      submit: 'Submit',
      create: 'Create',
      edit: 'Edit',
      delete: 'Delete',
      logout: 'Logout',
      details: 'Details',
      back: 'Back',
      buyNow: 'Buy Now',
      verify: 'Verify',
      remove: 'Remove',
      payNow: 'Pay Now'
    },
    states: {
      loading: 'Loading...',
      active: 'Active',
      expired: 'Expired',
      pending: 'Pending',
      paid: 'Paid',
      cancelled: 'Cancelled',
      completed: 'Completed',
      discounted: 'Discounted',
      unknown: 'Unknown',
      permanent: 'Permanent',
      open: 'Open',
      answered: 'Answered',
      closed: 'Closed',
      none: 'No subscription'
    },
    labels: {
      email: 'Email',
      password: 'Password',
      confirmPassword: 'Confirm password',
      expiresAt: 'Expires at',
      subject: 'Subject',
      priority: 'Priority',
      message: 'Message'
    },
    ticketPriority: {
      low: 'Low (general inquiries)',
      medium: 'Medium (usage issue)',
      high: 'High (service unavailable / urgent issue)'
    },
    periods: {
      month: 'Monthly',
      quarter: 'Quarterly',
      halfYear: 'Half-year',
      year: 'Yearly',
      twoYear: 'Two-year',
      threeYear: 'Three-year',
      onetime: 'One-time'
    },
    messages: {
      copySuccess: 'Copied to clipboard',
      copyFailed: 'Copy failed',
      loadFailed: 'Load failed',
      submitFailed: 'Submit failed',
      invalidCoupon: 'Invalid coupon code',
      paymentPending: 'Payment is still being integrated. Please try again later.'
    }
  },
  pageTitles: {
    auth: {
      login: 'Sign In'
    },
    user: {
      dashboard: 'Overview',
      subscribe: 'Subscription',
      knowledge: 'Help Center',
      tickets: 'Tickets',
      plans: 'Plans',
      orders: 'Orders'
    },
    admin: {
      dashboard: 'Dashboard',
      monitor: 'Traffic & Monitoring',
      users: 'Users',
      nodes: 'Nodes',
      subscriptions: 'Subscription groups',
      orders: 'Orders',
      plans: 'Plans',
      subscriptionTemplates: 'Subscription templates',
      tickets: 'Tickets',
      coupons: 'Coupons',
      knowledge: 'Help Center content',
      forward: 'Forward Management',
      forwardTunnel: 'Tunnels',
      forwardLimit: 'Limits',
      forwardAnsibleMachines: 'Ansible Machines',
      forwardNodes: 'NodeX Topology',
      forwardLocal: 'Local Runtime',
      forwardNodeX: 'NodeX Runtime',
      forwardAgents: 'NodeX Agents',
      control: 'Control Kernel',
      plugins: 'Plugin Center',
      routeModes: 'Route modes',
      deployments: 'Deployments',
      accessGroups: 'Access groups',
      payment: 'Payments',
      notifications: 'Notifications',
      invite: 'Referrals',
      inviteCodes: 'Invite codes',
      system: 'Settings',
      security: 'Security',
      fallback: 'Admin Console'
    }
  },
  app: {
    meta: {
      defaultDescription: `${CONTROL_NAME} centralizes subscription delivery, billing, node orchestration, agent runtime, and forwarding operations.`,
      loginDescription: `Sign in to ${CONTROL_NAME} to manage subscriptions, billing, nodes, and forwarding runtimes.`,
      userDescription: `${CONTROL_NAME} user portal for subscriptions, tickets, plans, and billing records.`,
      adminDescription: `${CONTROL_NAME} administration console for users, billing, nodes, notifications, and system operations.`,
      forwardDescription: `${CONTROL_NAME} forwarding workspace for Local Runtime, Ansible Machines, topology, tunnels, and runtime diagnostics.`
    }
  },
  layout: {
    // The shells' strings live under shell.* (locales/modules/*/shell.js);
    // these remain for the login page until it is redesigned (U5).
    admin: {
      brand: CONTROL_NAME,
      badge: 'Studio Console',
      sections: {
        overview: 'Overview',
        userManagement: 'User Management',
        system: 'System'
      },
      nav: {
        dashboard: 'Dashboard',
        users: 'Users',
        system: 'System'
      }
    },
    user: {
      brand: CONTROL_NAME
    }
  },
  legacy
}
