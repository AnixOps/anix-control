import runtimePages from './modules/en/runtimePages'
import networkPages from './modules/en/networkPages'
import miscPages from './modules/en/miscPages'
import adminSupportPages from './modules/en/adminSupportPages'

const legacy = {
  'V2Board 管理端': 'V2Board Admin',
  '管理端': 'Admin',
  '概览': 'Overview',
  '仪表盘': 'Dashboard',
  '流量转发': 'Forwards',
  '隧道管理': 'Tunnels',
  '限速管理': 'Limits',
  'Ansible 机器': 'Ansible Machines',
  '本地运行时': 'Local Runtime',
  'NodeX 拓扑': 'NodeX Topology',
  'NodeX 运行时': 'NodeX Runtime',
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
  '退出登录': 'Logout',
  '刷新': 'Refresh',
  '刷新中...': 'Refreshing...',
  '保存': 'Save',
  '取消': 'Cancel',
  '关闭': 'Close',
  '创建': 'Create',
  '编辑': 'Edit',
  '删除': 'Delete',
  '预览': 'Preview',
  '复制': 'Copy',
  '复制内容': 'Copy Content',
  '复制链接': 'Copy Link',
  '下载': 'Download',
  '打开': 'Open',
  '详情': 'Details',
  '返回': 'Back',
  '提交': 'Submit',
  '提交订单': 'Submit Order',
  '立即选购': 'Buy Now',
  '去支付': 'Pay Now',
  '验证': 'Verify',
  '检查中...': 'Checking...',
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
  '剩余天数': 'Remaining Days',
  '流量剩余': 'Traffic Remaining',
  '到期时间': 'Expires At',
  '创建时间': 'Created At',
  '更新时间': 'Updated At',
  '订单详情': 'Order Details',
  '回复': 'Reply',
  '提交工单': 'Submit Ticket',
  '关闭工单': 'Close Ticket',
  '阅读全文': 'Read More',
  '我知道了': 'I Understand',
  '加载中...': 'Loading...',
  '暂无数据': 'No data',
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
  'Doctor 输出': 'Doctor Output',
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
  ...runtimePages,
  ...networkPages,
  ...miscPages,
  ...adminSupportPages,
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
      createdAt: 'Created at',
      updatedAt: 'Updated at',
      subject: 'Subject',
      priority: 'Priority',
      message: 'Message',
      period: 'Billing period',
      plan: 'Plan',
      price: 'Price',
      status: 'Status'
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
      paymentPending: 'Payment is still being integrated. Please try again later.',
      closeTicketConfirm: 'Are you sure you want to close this ticket?'
    }
  },
  pageTitles: {
    auth: {
      login: 'Sign In'
    },
    user: {
      dashboard: 'Dashboard',
      subscribe: 'Subscriptions',
      knowledge: 'Guides',
      tickets: 'Tickets',
      plans: 'Plans',
      orders: 'Orders',
      fallback: 'User Center'
    },
    admin: {
      dashboard: 'Dashboard',
      users: 'Users',
      nodes: 'Nodes',
      subscriptions: 'Subscriptions',
      orders: 'Orders',
      plans: 'Plans',
      tickets: 'Tickets',
      coupons: 'Coupons',
      knowledge: 'Knowledge Base',
      forward: 'Forward Management',
      forwardTunnel: 'Tunnels',
      forwardLimit: 'Limits',
      forwardAnsibleMachines: 'Ansible Machines',
      forwardNodes: 'NodeX Topology',
      forwardLocal: 'Local Runtime',
      forwardNodeX: 'NodeX Runtime',
      forwardAgents: 'NodeX Agents',
      payment: 'Payment Gateways',
      telegram: 'Telegram Bot',
      mfa: 'MFA',
      notifications: 'Notifications',
      invite: 'Invite Rewards',
      system: 'System',
      fallback: 'Admin Console'
    }
  },
  app: {
    meta: {
      defaultDescription: 'Commercial-ready control plane for subscriptions, payments, nodes, NodeX runtime, and local Ansible relay operations.',
      loginDescription: 'Sign in to V2Board AnixOps to manage subscriptions, billing, nodes, and forwarding runtimes.',
      userDescription: 'User portal for subscriptions, tickets, plans, and billing records.',
      adminDescription: 'Administration console for users, billing, nodes, notifications, and system operations.',
      forwardDescription: 'Forwarding operations workspace for Local Runtime, Ansible Machines, NodeX topology, tunnels, and runtime diagnostics.'
    }
  },
  layout: {
    admin: {
      brand: 'V2Board',
      badge: 'Admin',
      mobileTitle: 'V2Board Admin',
      subtitle: 'Control plane, forwarding suite, and operations entry points are centralized in this navigation.',
      adminUser: 'Administrator',
      sections: {
        overview: 'Overview',
        forwardSuite: 'Forward Suite',
        userManagement: 'User Management',
        nodeManagement: 'Node Management',
        marketing: 'Marketing',
        finance: 'Finance',
        notifications: 'Notifications',
        content: 'Content',
        system: 'System'
      },
      nav: {
        dashboard: 'Dashboard',
        users: 'Users',
        orders: 'Orders',
        tickets: 'Tickets',
        nodes: 'Nodes',
        subscriptions: 'Subscriptions',
        plans: 'Plans',
        coupons: 'Coupons',
        invite: 'Invite Rewards',
        payment: 'Payment',
        telegram: 'Telegram Bot',
        notifications: 'Notifications',
        knowledge: 'Knowledge Base',
        mfa: 'MFA',
        system: 'System',
        nodeXAgentsLegacy: 'NodeX Agents Legacy'
      }
    },
    user: {
      brand: 'V2Board',
      nav: {
        dashboard: 'Dashboard',
        subscribe: 'Subscriptions',
        knowledge: 'Guides',
        tickets: 'Tickets',
        plans: 'Plans',
        orders: 'Orders'
      }
    }
  },
  forwardSuite: {
    nav: {
      forwards: 'Forwards',
      tunnels: 'Tunnels',
      limits: 'Limits',
      ansibleMachines: 'Ansible Machines',
      localRuntime: 'Local Runtime',
      nodeXTopology: 'NodeX Topology',
      nodeXRuntime: 'NodeX Runtime',
      nodeXAgents: 'NodeX Agents'
    },
    hints: {
      ansibleMachines: 'Stateless execution machines',
      localRuntime: 'Stateless panel-host executor',
      nodeXTopology: 'Stateful relay/exit topology',
      nodeXRuntime: 'Stateful gost control-plane',
      nodeXAgents: 'Stateful agent task channel'
    }
  },
  login: {
    brandSubtitle: 'High-performance proxy service control panel',
    signInTitle: 'Welcome back',
    registerTitle: 'Create an account',
    signInSubtitle: 'Sign in to continue',
    registerSubtitle: 'Fill in the form below to register',
    emailPlaceholder: 'Enter your email address',
    passwordPlaceholder: 'Enter your password',
    registerPasswordPlaceholder: 'Enter a password (minimum 6 characters)',
    confirmPasswordPlaceholder: 'Re-enter your password',
    forgotPassword: 'Forgot password?',
    switchToRegister: 'No account yet? Register',
    switchToLogin: 'Already have an account? Sign in',
    submitLogin: 'Sign in',
    submitRegister: 'Register',
    loadingLogin: 'Signing in...',
    loadingRegister: 'Registering...',
    mockMode: 'Development mode',
    mockUser: 'Mock user',
    mockAdmin: 'Mock admin',
    errors: {
      emailPasswordRequired: 'Please enter both email and password.',
      passwordMin: 'Password must be at least 6 characters.',
      passwordMismatch: 'The two passwords do not match.',
      registerFailed: 'Registration failed. Please try again later.',
      loginFailed: 'Sign-in failed. Check your email and password.'
    },
    success: {
      registerCompleted: 'Registration completed. Redirecting...'
    }
  },
  user: {
    dashboard: {
      title: 'My Subscription',
      subtitle: 'Review your current plan and usage.',
      refresh: 'Refresh',
      refreshing: 'Refreshing...',
      trafficUsage: 'Traffic usage',
      upload: 'Upload',
      download: 'Download',
      remainingDays: 'Remaining days',
      trafficRemaining: 'Traffic remaining',
      quickActions: {
        subscribe: 'Manage subscriptions',
        orders: 'My orders',
        tickets: 'My tickets',
        knowledge: 'Guides'
      },
      cacheUpdatedAt: 'Last updated at {value}'
    },
    subscribe: {
      title: 'Subscriptions',
      subtitle: 'Manage, copy, preview, and download your subscription links.',
      missingToken: 'No subscription token was detected. Please sign in again or refresh the page.',
      infoTitle: 'Subscription info',
      linksTitle: 'Subscription links',
      previewTitle: 'Subscription preview ({format})',
      usedTraffic: 'Used traffic',
      totalTraffic: 'Total traffic',
      copyLink: 'Copy link',
      refreshCache: 'Refresh subscription cache',
      copyContent: 'Copy content',
      closePreview: 'Close',
      fetchPreviewFailed: 'Failed to fetch preview.',
      noPreviewContent: 'No subscription content returned.',
      refreshCompleted: 'Subscription cache refreshed.',
      formats: {
        auto: 'Auto (by User-Agent)',
        v2ray: 'V2Ray (Base64)',
        clash: 'Clash (YAML)',
        stash: 'Stash (YAML)',
        egern: 'Egern (YAML)',
        surge: 'Surge',
        loon: 'Loon',
        shadowrocket: 'ShadowRocket',
        quantumultx: 'Quantumult X',
        singBox: 'Sing-box (JSON)',
        json: 'Raw JSON',
        base64json: 'Base64 JSON'
      }
    },
    orders: {
      title: 'My Orders',
      subtitle: 'Review and manage your order history.',
      loading: 'Loading your order history...',
      empty: 'You do not have any orders yet. Explore the available plans to get started.',
      buyNow: 'Browse plans',
      unknownPlan: 'Unknown plan',
      detailTitle: 'Order details',
      headers: {
        tradeNo: 'Order #',
        plan: 'Plan',
        period: 'Billing',
        amount: 'Amount',
        status: 'Status',
        createdAt: 'Created at',
        actions: 'Actions'
      },
      labels: {
        tradeNo: 'Order number',
        status: 'Status',
        plan: 'Plan',
        period: 'Billing period',
        totalAmount: 'Total amount',
        discount: 'Discount',
        createdAt: 'Created at',
        paidAt: 'Paid at'
      }
    },
    plans: {
      title: 'Plans',
      subtitle: 'Choose the plan that matches your traffic needs and start instantly.',
      loading: 'Loading plans...',
      permanentBadge: 'Permanent',
      unlimitedFeature: 'Multi-country nodes with full protocol support',
      trafficFeature: '{value} traffic',
      speedLimitFeature: '{value} Mbps speed limit',
      deviceLimitFeature: '{value} devices online at the same time',
      confirmOrder: 'Confirm order',
      selectedPlan: 'Selected plan',
      choosePeriod: 'Choose billing period',
      optionalCoupon: 'Coupon code (optional)',
      couponPlaceholder: 'Enter coupon code',
      couponApplied: 'Coupon applied: {name} (-¥{value})',
      totalAmount: 'Total due',
      backToEdit: 'Back',
      creatingOrder: 'Creating order...'
    },
    tickets: {
      title: 'My Tickets',
      subtitle: 'Submit feedback or request technical support.',
      submitTicket: 'New ticket',
      active: 'Active',
      resolved: 'Resolved',
      loading: 'Loading...',
      empty: 'No tickets yet. Submit one whenever you need help.',
      submitNow: 'Create now',
      newTicketTitle: 'Create a new ticket',
      replyPlaceholder: 'Write your reply...',
      closeWindow: 'Close window',
      closedHint: 'This ticket is already closed. Create a new one if you need more help.',
      assistant: 'Support assistant',
      me: 'Me',
      updated: 'updated',
      ticketId: 'Ticket #{id}'
    },
    knowledge: {
      title: 'Guides',
      subtitle: 'Find setup guides, notices, and frequently asked questions.',
      loading: 'Loading...',
      empty: 'No related articles found.',
      readMore: 'Read more →',
      updated: 'updated',
      all: 'All',
      closeAction: 'Close'
    }
  },
  runtime: {
    shared: {
      panelConfig: 'Panel Config',
      reachability: 'Reachability',
      runtimeReady: 'Runtime Ready',
      executor: 'Executor',
      files: 'Files',
      warnings: 'Warnings',
      powerShell: 'PowerShell',
      bash: 'Bash',
      bootstrapVerify: 'Bootstrap / Verify',
      references: 'References',
      doctorOutput: 'Doctor Output',
      doctorNotExecuted: 'Doctor has not been executed yet.',
      refreshStatus: 'Refresh status',
      runDoctor: 'Run doctor',
      runningDoctor: 'Running...',
      loading: 'Loading...',
      yes: 'Yes',
      no: 'No',
      present: 'Present',
      missing: 'Missing',
      reachable: 'Reachable',
      notReady: 'Not ready',
      ready: 'Ready',
      unavailable: 'Unavailable',
      pending: 'Pending',
      running: 'Running',
      success: 'Success',
      failed: 'Failed',
      unknown: 'Unknown',
      online: 'Online',
      offline: 'Offline',
      enabled: 'Enabled',
      disabled: 'Disabled',
      all: 'All'
    },
    localRuntime: {
      heroEyebrow: 'Stateless Runtime',
      title: 'Local Runtime / Ansible',
      heroTextPrimary: 'This page owns the panel-host Ansible executor only. It is the stateless runtime path for panel-side forwarding and does not require a persistent NodeX control-plane or Node-Agent connection.',
      heroTextSecondary: 'The recommended backend is nftables / Ansible. iptables / Ansible remains available as a legacy compatibility path.',
      refreshLoading: 'Refreshing...',
      saveLoading: 'Saving...',
      saveActivate: 'Save And Activate Local Runtime',
      activeBannerTitle: 'Local runtime is active',
      standbyBannerTitle: 'Local runtime is configured as standby',
      activeBannerText: 'Forward jobs currently use {backend}. SSH transport and privilege escalation are resolved from this Ansible runtime config.',
      standbyBannerText: 'NodeX/gost remains active globally. You can still stage and validate the local Ansible runtime here before switching back.',
      configEyebrow: 'Configuration',
      configTitle: 'Panel-Host Ansible Executor',
      configCopy: 'Ansible mode is stateless: the panel stores execution-node identity on tunnel and forward records, while inventory, playbooks, sudo and SSH behavior live here.',
      recommended: 'Recommended',
      legacy: 'Legacy',
      executorEyebrow: 'Executor',
      defaultsAction: 'Use backend defaults',
      executorHint: 'Saving here keeps {backend} active by writing `forward.runtime_backend={backendKey}`, `forward.runtime.ansible.backend={backendKey}` and `forward.runtime.nodex_mode=false`.',
      fields: {
        inventory: 'Inventory',
        applyPlaybook: 'Apply playbook',
        removePlaybook: 'Remove playbook',
        command: 'Command',
        workingDir: 'Working dir',
        targetPattern: 'Target pattern',
        timeoutSeconds: 'Timeout (seconds)',
        ansibleConfig: 'ANSIBLE_CONFIG',
        useBecome: 'Use sudo / become on the execution node',
        extraVarsJson: 'Extra vars JSON',
        environmentJson: 'Environment JSON',
        generatedJson: 'Generated runtime JSON'
      },
      extraVarsHint: 'Backend-specific fields such as firewall driver are injected automatically by the backend.',
      environmentHint: 'Extra process environment variables for the panel-host executor.',
      generatedHint: 'The JSON payload is generated from the structured fields above and stored in `forward.runtime.ansible.config`.',
      probeEyebrow: 'Local Probe',
      probeTitle: 'Executor Reachability And Runtime Readiness',
      loadingStatus: 'Loading local runtime status...',
      noStatus: 'No local runtime status loaded yet.',
      cards: {
        localActiveValue: 'Local runtime active',
        standbyValue: 'Standby config',
        backend: 'Backend',
        preferredLocalBackend: 'Preferred local backend',
        attachment: 'Attachment',
        runtimeReady: 'Runtime ready',
        firewallDriver: 'Firewall driver',
        commandFound: 'Command found',
        become: 'Become',
        inventory: 'Inventory',
        applyPlaybook: 'Apply playbook',
        removePlaybook: 'Remove playbook',
        workingDir: 'Working dir'
      },
      jobsEyebrow: 'Runtime Jobs',
      latestJobs: 'Latest {backend} Jobs',
      jobMeta: 'forward {forwardId} / tunnel {tunnelId} / node {nodeId}',
      loadingJobs: 'Loading runtime jobs...',
      noJobs: 'No local runtime jobs yet.',
      backends: {
        nftables: {
          label: 'nftables / Ansible',
          description: 'Modern Linux hosts should prefer nftables.'
        },
        iptables: {
          label: 'iptables / Ansible',
          description: 'Legacy compatibility for existing playbooks.'
        }
      },
      errors: {
        savedConfigInvalid: 'Saved local runtime config is invalid. Defaults were loaded; save again to repair it.',
        invalidJson: '{label} must be valid JSON',
        invalidObject: '{label} must be a JSON object',
        invalidPreview: 'Invalid runtime config: {message}',
        invalidRuntimeJson: 'Local runtime JSON is invalid',
        saveFailed: 'Failed to save local runtime config',
        fetchStatusFailed: 'Failed to fetch local runtime status',
        doctorFailed: 'Local runtime doctor failed'
      }
    },
    nodeX: {
      heroEyebrow: 'Private Runtime',
      title: 'NodeX Runtime',
      heroTextPrimary: 'Dedicated operator entry for the stateful NodeX/gost path. This page probes the configured NodeX control plane directly, even when the current global runtime backend is still a local Ansible backend.',
      heroTextSecondary: 'Local Ansible execution now lives under Local Runtime and Ansible Machines. Node "online" still means TCP reachability only and is not proof that NodeX or the relay gost API is already attached.',
      refreshLoading: 'Refreshing...',
      saveLoading: 'Saving...',
      save: 'Save NodeX Config',
      enabledBannerTitle: 'NodeX Mode is enabled',
      disabledBannerTitle: 'NodeX Mode is disabled',
      enabledBannerText: 'Panel forward jobs can route through NodeX/gost, but each runtime job still has to succeed before relay attachment is real.',
      disabledBannerText: 'You can validate the configured NodeX control plane here first, then switch the global backend when you are ready.',
      configEyebrow: 'Configuration',
      configTitle: 'NodeX Control Plane',
      enableModeTitle: 'Enable NodeX Mode',
      enableModeHint: 'Writes `forward.runtime.nodex_mode=true` and `forward.runtime_backend=gost`.',
      fields: {
        baseUrl: 'NodeX Base URL',
        baseUrlHint: 'This must point to the NodeX control-plane, not directly to the relay gost API.',
        token: 'NodeX Token',
        tokenHint: 'Matches the NodeX control-plane `--forward-api-token` value.',
        timeout: 'Timeout (seconds)',
        timeoutHint: 'Used by the panel when probing or executing NodeX runtime requests.'
      },
      probeEyebrow: 'NodeX Probe',
      probeTitle: 'Health And Runtime Status',
      probeCopy: 'These checks always target the configured NodeX control plane. They do not depend on the currently active runtime backend.',
      loadingStatus: 'Loading NodeX runtime status...',
      noStatus: 'No NodeX runtime status loaded yet.',
      cards: {
        modeOn: 'NodeX mode on',
        modeOff: 'NodeX mode off',
        backend: 'Backend',
        baseUrl: 'Base URL',
        tokenConfigured: 'Token configured',
        timeout: 'Timeout',
        health: 'Health',
        http: 'HTTP',
        version: 'Version',
        executePath: 'Execute path'
      },
      jobsEyebrow: 'Runtime Jobs',
      jobsTitle: 'Latest gost Jobs',
      jobsCopy: 'Recent panel-side runtime audit rows filtered to the `gost` backend.',
      jobMeta: 'forward {forwardId} / tunnel {tunnelId} / node {nodeId}',
      loadingJobs: 'Loading runtime jobs...',
      noJobs: 'No gost runtime jobs yet.',
      backends: {
        gost: 'gost / NodeX'
      },
      errors: {
        baseUrlRequired: 'NodeX base URL is required when NodeX mode is enabled',
        tokenRequired: 'NodeX token is required when NodeX mode is enabled',
        saveFailed: 'Failed to save NodeX config',
        fetchStatusFailed: 'Failed to fetch NodeX runtime status',
        doctorFailed: 'NodeX runtime doctor failed'
      }
    },
    ansibleMachines: {
      heroEyebrow: 'Execution Fleet',
      title: 'Ansible Machines',
      heroText: 'This page is only for stateless Ansible execution machines. These hosts do not need Node-Agent and do not need a persistent control-plane connection.',
      refreshLoading: 'Refreshing...',
      addMachine: 'Add Machine',
      stats: {
        machines: 'Machines',
        online: 'Online',
        enabled: 'Enabled'
      },
      sectionEyebrow: 'Machines',
      sectionTitle: 'Execution Targets',
      sectionCopy: 'These records only identify hosts for the local Ansible runtime. They are not NodeX control-plane nodes.',
      inventoryHint: 'SSH user, password, and private key are not stored on this page. Define them in Ansible inventory, playbooks, or Local Runtime environment settings.',
      filterLabel: 'Status',
      filters: {
        all: 'All',
        online: 'Online',
        offline: 'Offline'
      },
      loading: 'Loading Ansible machines...',
      empty: 'No Ansible execution machines yet.',
      machineEyebrow: 'Machine #{id}',
      meta: {
        authSource: 'Auth Source',
        authSourceValue: 'Inventory / Local Runtime',
        regionIsp: 'Region / ISP',
        currentConn: 'Current Conn',
        traffic: 'Traffic'
      },
      actions: {
        edit: 'Edit',
        check: 'Health Check',
        checking: 'Checking...',
        sync: 'Sync Stats',
        syncing: 'Syncing...',
        disable: 'Disable',
        enable: 'Enable',
        delete: 'Delete'
      },
      modal: {
        eyebrow: 'Machine',
        titleEdit: 'Edit Ansible Machine',
        titleAdd: 'Add Ansible Machine',
        deleteEyebrow: 'Delete',
        deleteTitle: 'Delete Machine',
        deleteConfirm: 'Delete {name} from the Ansible execution fleet?',
        saveLoading: 'Saving...',
        save: 'Save',
        cancel: 'Cancel',
        deleteLoading: 'Deleting...'
      },
      fields: {
        name: 'Name',
        host: 'Host',
        reachabilityPort: 'Reachability Port',
        weight: 'Weight',
        region: 'Region',
        isp: 'ISP'
      },
      placeholders: {
        name: 'relay-exec-01',
        host: '1.2.3.4',
        region: 'HK / JP / US',
        isp: 'CMI / NTT / Cogent'
      },
      results: {
        latency: 'Latency {value} ms',
        reachable: 'Machine reachable',
        unavailable: 'Machine unavailable',
        synced: 'Stats synced'
      },
      errors: {
        required: 'Name, host and reachability port are required.',
        saveFailed: 'Failed to save machine',
        loadFailed: 'Failed to load Ansible machines',
        detailFailed: 'Failed to load machine details',
        deleteFailed: 'Failed to delete machine',
        checkFailed: 'Health check failed',
        syncFailed: 'Failed to sync machine stats',
        toggleFailed: 'Failed to change machine status'
      }
    },
    forward: {
      heroEyebrow: 'Flux Compatible',
      title: 'Forward Management',
      note: 'NodeX mode keeps ingress/exit semantics; local Ansible mode only targets execution nodes resolved by inventory. Forward node "online" only checks TCP reachability and does not prove remote attachment or firewall state already exists.',
      modeLabelNodeX: 'Active Runtime: NodeX / gost',
      modeLabelLocal: 'Active Runtime: Local / {backend}',
      modeSummaryNodeX: 'Edit NodeX control-plane URL, token and gost operator checks on the dedicated NodeX Runtime page.',
      modeSummaryLocal: 'Edit inventory, playbooks and panel-host executor settings on the dedicated Local Runtime page.',
      modeCompatibilityHint: 'Forward editor options are filtered by the currently active runtime. Local Ansible runtime only accepts Port Forward tunnels, while NodeX/gost can attach both compatible port-forward and tunnel-forward layouts.',
      modeHintNodeX: 'NodeX/gost mode keeps ingress and exit semantics. A selected tunnel still requires NodeX runtime jobs to succeed before forwarding is really attached.',
      modeHintLocal: 'Local Ansible mode only records the execution node. SSH access comes from the configured ansible inventory and local runtime settings, not from NodeX topology records.',
      tunnelHintNodeX: '{name} will be attached through NodeX/gost. Panel-side "online" or status checks do not prove the remote relay has finished attaching.',
      tunnelHintLocal: '{name} will be applied on the execution node only. This path stays stateless until the queued ansible job finishes successfully.',
      tunnelHintLocalIncompatible: '{name} is a Tunnel Forward tunnel and requires NodeX/gost. Local Ansible runtime cannot attach it directly.',
      portRange: 'Allowed range: {start} - {end}',
      portHintNodeX: 'Leaving the port empty lets the panel allocate one from the tunnel entry-node range.',
      portHintLocal: 'Leaving the port empty lets the panel allocate one on the selected execution node.',
      loading: 'Loading forwards and tunnels...',
      view: {
        switchToDirectTitle: 'Switch to direct view',
        switchToGroupedTitle: 'Switch to grouped view',
        directShort: 'D',
        groupedShort: 'G',
        directLabel: 'Direct',
        groupedLabel: 'Grouped'
      },
      actions: {
        import: 'Import',
        export: 'Export',
        add: 'Create',
        edit: 'Edit',
        diagnose: 'Diagnose',
        delete: 'Delete',
        copyAll: 'Copy All',
        regenerate: 'Regenerate',
        generateExport: 'Generate Export Data',
        startImport: 'Start Import',
        rerunDiagnosis: 'Run Again'
      },
      group: {
        eyebrow: 'User',
        userTag: 'User',
        summary: '{tunnels} tunnels, {forwards} forwards',
        tunnelMeta: 'Tunnel #{id}'
      },
      emptyGroupedTitle: 'No forwards yet',
      emptyGroupedText: 'There are no flux-panel-compatible forward records in the current system yet.',
      emptyDirectTitle: 'No forwards yet',
      emptyDirectText: 'After you create the first forward, the direct card view will appear here.',
      editor: {
        eyebrow: 'Forward',
        titleEdit: 'Edit Forward',
        titleAdd: 'Create Forward',
        fields: {
          name: 'Forward Name',
          tunnel: 'Tunnel',
          ingressPort: 'Ingress Port',
          interfaceName: 'Interface Name',
          remoteAddress: 'Target Address',
          strategy: 'Scheduling Strategy'
        },
        placeholders: {
          name: 'e.g. HK-Web-01',
          tunnel: 'Please select a tunnel',
          ingressPort: 'Leave blank to auto-allocate',
          interfaceName: 'Optional, e.g. eth0',
          remoteAddress: 'One target per line, for example:\n1.1.1.1:443\nexample.com:8443\n[2001:db8::1]:443'
        },
        remoteHint: 'Supports IPv4:port, domain:port, or [full IPv6]:port. Use one address per line for multiple targets.',
        submitLoading: 'Submitting...',
        submitUpdate: 'Save Changes',
        submitCreate: 'Create Forward'
      },
      deleteModal: {
        eyebrow: 'Delete',
        title: 'Delete Forward',
        confirmText: 'Delete {name}?',
        hint: 'If regular deletion fails, a force-delete confirmation will be shown next.',
        deleteLoading: 'Deleting...',
        confirmDelete: 'Confirm Delete',
        forceDeleteIntro: 'Regular delete failed: {message}',
        forceDeleteQuestion: 'Do you want to force delete it?',
        forceDeleteWarning: 'Warning: force delete does not verify whether the node-side forward service was removed.'
      },
      addressModal: {
        eyebrow: 'Address',
        copy: 'Copy',
        copying: 'Copying...',
        titleWithCount: '{title} ({count})'
      },
      exportModal: {
        eyebrow: 'Export',
        title: 'Export Forward Data',
        subtitle: 'Format: remoteAddr|name|inPort',
        tunnelLabel: 'Select Export Tunnel',
        tunnelPlaceholder: 'Please select a tunnel',
        generating: 'Generating...',
        regenerate: 'Regenerate',
        generate: 'Generate Export Data',
        noDataPlaceholder: 'No export data yet'
      },
      importModal: {
        eyebrow: 'Import',
        title: 'Import Forward Data',
        subtitle: 'Format: remoteAddr|name|inPort, one entry per line, inPort may be blank.',
        subtitleSecondary: 'Target addresses may contain a single address or multiple comma-separated addresses, for example: 3.3.3.3:3,4.4.4.4:4',
        tunnelLabel: 'Select Import Tunnel',
        tunnelPlaceholder: 'Please select a tunnel',
        dataLabel: 'Import Data',
        placeholder: 'example.com:8080|Business Entry|10086',
        resultTitle: 'Import Results',
        resultSummary: 'Success: {success} / Total: {total}',
        statusSuccess: 'Success',
        statusFailed: 'Failed',
        importing: 'Importing...',
        startImport: 'Start Import'
      },
      diagnosis: {
        eyebrow: 'Diagnosis',
        title: 'Forward Diagnosis Results',
        loading: 'Diagnosing forward connectivity...',
        connectionSuccess: 'Connection Succeeded',
        connectionFailed: 'Connection Failed',
        nodeMeta: '{name} · {node}',
        nodeMeta: '{name} · {node}',
        nodeMeta: '{name} / {node}',
        targetAddress: 'Target Address',
        averageLatency: 'Average Latency',
        packetLoss: 'Packet Loss',
        quality: 'Quality',
        failedFallback: 'Diagnosis Failed',
        emptyTitle: 'No diagnosis data yet',
        emptyText: 'After you run a diagnosis, results aligned with the reference page will appear here.',
        rerunning: 'Diagnosing...',
        rerun: 'Run Again'
      },
      status: {
        normal: 'Healthy',
        paused: 'Paused',
        error: 'Error',
        unknown: 'Unknown'
      },
      runtimeStatus: {
        pending: 'Pending Push',
        running: 'Running',
        synced: 'Synced',
        applied: 'Applied',
        failed: 'Sync Failed',
        queuedSummary: 'Runtime job has been queued and is waiting for the executor to finish.',
        runningSummary: 'Runtime job is running.'
      },
      strategy: {
        fifo: 'Primary / Backup',
        round: 'Round Robin',
        rand: 'Random',
        hash: 'Hash',
        unknown: 'Unknown'
      },
      quality: {
        unknown: 'Unknown',
        excellent: 'Excellent',
        veryGood: 'Very Good',
        good: 'Good',
        fair: 'Fair',
        poor: 'Poor',
        veryPoor: 'Very Poor'
      },
      labels: {
        inbound: 'In',
        outbound: 'Out'
      },
      references: {
        tunnel: 'Tunnel #{id}',
        node: 'Node #{id}',
        node: 'Node {id}',
        node: 'Node #{id}'
      },
      messages: {
        loadForwardsFailed: 'Failed to load forward list',
        loadTunnelsFailed: 'Failed to load tunnel list',
        loadDataFailed: 'Failed to load data',
        unknownUser: 'Unknown User',
        nameRequired: 'Please enter a forward name',
        nameLength: 'Forward name must be between 2 and 50 characters',
        tunnelRequired: 'Please select an associated tunnel',
        remoteAddrRequired: 'Please enter a remote address',
        remoteAddrLineInvalid: 'Line {line} has an invalid address format',
        portRange: 'Port must be between 1 and 65535',
        portRangeTunnel: 'Port must be within {start}-{end}',
        updated: 'Updated successfully',
        created: 'Created successfully',
        actionFailed: 'Operation failed',
        runtimeBusy: 'A runtime job is still queued or running. Wait for it to finish before trying again.',
        invalidStatus: 'Forward status is abnormal and cannot be operated on.',
        serviceChanged: 'Service change request submitted',
        servicePaused: 'Pause request submitted',
        networkActionFailed: 'Network error, operation failed',
        deleted: 'Deleted successfully',
        forceDeleted: 'Force delete succeeded',
        forceDeleteFailed: 'Force delete failed',
        deleteFailed: 'Delete failed',
        diagnosisFailed: 'Diagnosis failed',
        diagnosisProcessingFailed: 'An error occurred during diagnosis',
        diagnosisNetworkFailed: 'Network error, diagnosis failed',
        unableConnectServer: 'Unable to connect to the server',
        contentCopied: '{label} copied',
        copyFailedHttp: 'Copy failed: clipboard requires HTTPS or a reverse proxy',
        selectExportTunnel: 'Please select a tunnel to export',
        noExportData: 'The selected tunnel has no forward data',
        exportFailed: 'Export failed',
        enterImportData: 'Please enter data to import',
        selectImportTunnel: 'Please select a tunnel to import into',
        importCompleted: 'Import completed',
        importFailed: 'An error occurred during import',
        importFormatError: 'Invalid format: target address and forward name are required at minimum',
        importRequiredFields: 'Target address and forward name cannot be empty',
        importAddressInvalid: 'Invalid target address format. Expected host:port, with commas for multiple addresses',
        importPortInvalid: 'Invalid ingress port. It must be a number between 1 and 65535',
        importCreateSuccess: 'Created successfully',
        importCreateFailed: 'Create failed',
        importNetworkCreateFailed: 'Network error, create failed',
        orderSaveFailed: 'Failed to save order: {message}',
        orderSaveRetry: 'Failed to save order, please try again',
        unknownError: 'Unknown error',
        localRuntimeTunnelForwardUnsupported: 'Local Ansible runtime cannot attach Tunnel Forward tunnels. Switch to NodeX Runtime or choose a Port Forward tunnel.'
      },
      card: {
        dragHandleTitle: 'Drag to reorder',
        ingressAddressTitle: 'Ingress address',
        ingressLabel: 'Ingress',
        targetAddressTitle: 'Target address',
        targetLabel: 'Target',
        status: {
          normal: 'Healthy',
          paused: 'Paused',
          error: 'Error',
          unknown: 'Unknown'
        },
        strategy: {
          round: 'Round robin',
          random: 'Random',
          hash: 'Hash',
          primaryBackup: 'Primary / Backup'
        }
      }
    },
    tunnel: {
      note: 'NodeX mode separates ingress and execution nodes; local Ansible mode only needs the execution node mapped in inventory. Tunnel "online" only checks host:port reachability and does not confirm remote attachment or firewall state is already in place.',
      modeLabelNodeX: 'Active Runtime: NodeX / gost',
      modeLabelLocal: 'Active Runtime: Local / {backend}',
      modeSummaryNodeX: 'Ingress and egress semantics are controlled through NodeX/gost. Use NodeX Runtime for the control-plane URL, token and gost readiness.',
      modeSummaryLocal: 'Only the execution node identity is stored here. Use Local Runtime for inventory, playbooks and the panel-host ansible executor.',
      modeCompatibilityHint: 'Tunnel cards below are evaluated against the currently active runtime. Legacy type-1 tunnels may stay schema-compatible with both runtimes, so use the compatibility badge instead of assuming ownership from stored fields.',
      loading: 'Loading tunnels and nodes...',
      emptyTitle: 'No tunnels yet.',
      emptyText: 'Create the required topology first, then add the first tunnel that forwards can reference.',
      actions: {
        add: 'Create tunnel',
        edit: 'Edit',
        diagnose: 'Diagnose',
        delete: 'Delete'
      },
      meta: {
        ingressNode: 'Ingress node',
        egressNode: 'Egress node',
        executionNode: 'Execution node',
        flowAccounting: 'Flow accounting',
        trafficRatio: 'Traffic ratio'
      },
      modal: {
        eyebrow: 'Tunnel',
        titleEdit: 'Edit tunnel',
        titleAdd: 'Create tunnel',
        deleteEyebrow: 'Delete',
        deleteTitle: 'Confirm deletion',
        deleteConfirmMessage: 'Delete tunnel {name}?',
        deleteHint: 'If this tunnel is still referenced by forward rules or user entitlements, the backend will block deletion.',
        submitLoading: 'Submitting...',
        submitUpdate: 'Update',
        submitCreate: 'Create',
        deleteLoading: 'Deleting...',
        confirmDelete: 'Confirm delete'
      },
      fields: {
        name: 'Tunnel name',
        tunnelType: 'Tunnel type',
        flowAccounting: 'Flow accounting',
        trafficRatio: 'Traffic ratio',
        ingressNode: 'NodeX ingress node',
        executionNode: 'Execution node',
        tcpListenAddr: 'TCP listen address',
        udpListenAddr: 'UDP listen address',
        interfaceName: 'Egress interface or IP',
        protocol: 'Protocol',
        egressNode: 'NodeX egress node'
      },
      placeholders: {
        name: 'HK-Tunnel-01',
        interfaceName: 'eth0 / 192.0.2.10'
      },
      options: {
        portForward: 'Port forward',
        tunnelForward: 'Tunnel forward',
        oneWayAccounting: 'One-way accounting',
        twoWayAccounting: 'Two-way accounting'
      },
      hints: {
        ingressNode: 'Only NodeX/gost mode uses an ingress node here. This is a forward relay role and stays separate from proxy nodes.',
        executionNode: 'Local Ansible mode only needs the execution node identity. SSH access still comes from the configured inventory and local runtime settings.',
        egressNode: 'Exit nodes are only used by NodeX/gost tunnel forwarding. A successful panel save still needs the runtime job to attach remotely.'
      },
      compatibility: {
        nodeXReady: 'Ready for NodeX runtime',
        nodeXNeedsIngress: 'NodeX runtime needs an ingress node',
        nodeXNeedsEgress: 'NodeX tunnel-forward needs an egress node',
        localReady: 'Ready for Local Runtime',
        localNeedsExecution: 'Local Runtime needs an execution node',
        localOnlyPortForward: 'Local Runtime only supports port-forward tunnels'
      },
      messages: {
        loadListFailed: 'Failed to load tunnel list',
        loadDataFailed: 'Failed to load data',
        created: 'Tunnel created successfully',
        updated: 'Tunnel updated successfully',
        actionFailed: 'Operation failed',
        deleted: 'Tunnel deleted successfully',
        deleteFailed: 'Delete failed',
        diagnosis: 'Tunnel diagnosis',
        diagnosisFailed: 'Diagnosis failed',
        diagnosisRequestFailed: 'Diagnosis request failed'
      },
      diagnosis: {
        eyebrow: 'Diagnosis',
        title: 'Tunnel Diagnosis Results',
        loading: 'Diagnosing tunnel connectivity...',
        targetAddress: 'Target Address',
        duration: 'Duration',
        message: 'Message',
        emptyTitle: 'No diagnosis results yet',
        emptyText: 'There is currently no diagnosis data to display.',
        rerun: 'Run Again',
        rerunning: 'Diagnosing...'
      },
      validation: {
        nameRequired: 'Please enter a tunnel name',
        nameLength: 'Tunnel name must be 2-50 characters',
        typeInvalid: 'Please select a valid tunnel type',
        ingressRequired: 'Please select an ingress node',
        ingressMustRelay: 'Ingress node must be a relay node',
        trafficRatio: 'Traffic ratio must be between 0.1 and 100.0',
        tcpListenRequired: 'Please enter a TCP listen address',
        udpListenRequired: 'Please enter a UDP listen address',
        egressRequired: 'Please select an egress node',
        ingressEgressDifferent: 'Ingress and egress nodes cannot be the same',
        egressMustExit: 'Egress node must be an exit node',
        protocolRequired: 'Please select a protocol',
        executionRequired: 'Please select an execution node',
        executionMustRelay: 'Execution node must be a relay node'
      }
    },
    workbench: {
      eyebrow: 'Forward Runtime',
      title: 'Runtime Workbench',
      subtitle: 'Dual runtime control plane',
      actions: {
        refreshJobs: 'Refresh jobs',
        openAnsibleMachines: 'Open Ansible Machines',
        openLocalRuntime: 'Open Local Runtime',
        openNodeXRuntime: 'Open NodeX Runtime',
        refreshActiveRuntime: 'Refresh active runtime',
        runDoctorActiveRuntime: 'Run doctor on active runtime'
      },
      localCard: {
        eyebrow: 'Local Runtime',
        title: 'Local Ansible executor',
        description: 'Stateless panel-host execution. Inventory, playbooks and SSH access are managed separately from NodeX.',
        currentState: 'Current state',
        manage: 'Manage Local Runtime / Ansible'
      },
      nodeXCard: {
        eyebrow: 'NodeX Runtime',
        title: 'Stateful gost control-plane',
        description: 'Panel talks to the internal NodeX control-plane. Real relay attachment only exists after gost runtime jobs succeed.',
        manage: 'Manage NodeX Runtime'
      },
      state: {
        activeBackend: 'Active',
        standby: 'Standby'
      },
      references: {
        panelRuntimeDoc: 'Panel doc: docs/reference/runtime.md',
        panelRelayOnboarding: 'Panel doc: docs/guide/forward-relay-onboarding.md',
        nodeXRepo: 'NodeX repo: https://github.com/zdwtest/NodeX',
        panelNodeXOnboarding: 'Panel doc: docs/forward-runtime-relay-onboarding.md'
      },
      recentJobsTitle: 'Recent runtime jobs',
      recentJobsSubtitle: 'Latest queued and executed actions across the dedicated Local Runtime and NodeX Runtime pages.',
      loadingJobs: 'Loading runtime jobs...',
      noJobs: 'No runtime jobs yet.',
      jobMeta: '{backend} / forward {forwardId} / tunnel {tunnelId} / node {nodeId}',
      doctor: {
        eyebrow: 'Forward Runtime Doctor',
        title: 'Active Runtime Snapshot',
        description: 'Reachability only means the control plane or local executor can be contacted. It is not proof that a relay has already attached or that iptables rules already exist.',
        note: 'This workbench only shows the currently active backend. Use the dedicated Local Runtime and NodeX Runtime pages to edit config and run mode-specific probes.',
        summary: 'This workbench consolidates control-plane health, runtime diagnostics and one-click commands across both NodeX/gost and local Ansible execution paths.',
        loadingStatus: 'Fetching forward runtime status...'
      },
      cards: {
        backend: 'Backend',
        nodeXMode: 'NodeX Mode',
        attachment: 'Attachment',
        panelVerdict: 'Panel Verdict',
        nodeXSnapshot: 'NodeX Snapshot',
        baseUrlConfigured: 'Base URL configured',
        runtimeVersion: 'Runtime version',
        localAnsible: 'Local ansible',
        playbooks: 'Playbooks'
      },
      errors: {
        fetchStatusFailed: 'Failed to fetch forward runtime status',
        doctorFailed: 'Forward runtime doctor failed',
        savedConfigInvalid: 'Saved ansible runtime config is invalid. Defaults were loaded; save again to repair it.',
        nodeXBaseUrlRequired: 'NodeX base URL is required in NodeX Mode',
        nodeXTokenRequired: 'NodeX token is required in NodeX Mode',
        invalidRuntimeJson: 'ansible JSON invalid',
        saveFailed: 'Failed to save runtime config'
      }
    },
    systemPage: {
      title: 'System',
      subtitle: 'System configuration, backups, and load balancers',
      tabs: {
        config: 'System Config',
        backup: 'Backups',
        balancer: 'Load Balancers',
        audit: 'Audit Logs'
      },
      actions: {
        addConfig: 'Add Config',
        createBalancer: 'Create Balancer',
        restore: 'Restore',
        healthCheck: 'Health Check'
      },
      config: {
        searchPlaceholder: 'Search config keys...',
        table: {
          key: 'Key',
          value: 'Value',
          description: 'Description',
          updatedAt: 'Updated At',
          actions: 'Actions'
        },
        empty: 'No config data'
      },
      backup: {
        title: 'Automatic Backup',
        enabled: 'Enable automatic backup',
        intervalHours: 'Backup Interval (hours)',
        keepCount: 'Retention Count',
        backupDatabase: 'Include database',
        backupFiles: 'Include files',
        storageType: 'Storage Type',
        storagePath: 'Local Storage Path',
        storagePathPlaceholder: 'For example: backups',
        s3Bucket: 'S3 Bucket',
        s3BucketPlaceholder: 'For example: panel-backups',
        s3Region: 'S3 Region',
        s3RegionPlaceholder: 'For example: us-east-1',
        s3Endpoint: 'S3 Endpoint (optional)',
        s3EndpointPlaceholder: 'For example: https://s3.amazonaws.com',
        s3AccessKey: 'S3 Access Key',
        s3AccessKeyPlaceholder: 'Leave blank to keep current key',
        s3SecretKey: 'S3 Secret Key',
        s3SecretKeyPlaceholder: 'Leave blank to keep current secret',
        storageTypes: {
          local: 'Local',
          s3: 'S3 Compatible'
        },
        sensitiveHintWithValue: 'Sensitive value is hidden. Leave blank to keep current value, or enter a new value to rotate it.',
        sensitiveHintWithoutValue: 'This sensitive field is not set yet. Enter a value to save it.',
        saveConfig: 'Save Config',
        backupNow: 'Backup Now',
        stats: {
          totalCount: 'Total backups:',
          totalSize: 'Total size:',
          lastBackup: 'Last backup:'
        },
        listTitle: 'Backup List',
        table: {
          id: 'ID',
          filename: 'Filename',
          size: 'Size',
          status: 'Status',
          createdAt: 'Created At',
          actions: 'Actions'
        },
        empty: 'No backup data'
      },
      balancer: {
        table: {
          id: 'ID',
          name: 'Name',
          group: 'Node Group',
          strategy: 'Strategy',
          healthCheck: 'Health Check',
          enabled: 'Status',
          actions: 'Actions'
        },
        empty: 'No load balancers'
      },
      audit: {
        title: 'Operation Audit Logs',
        actions: {
          filter: 'Filter',
          refresh: 'Refresh'
        },
        filters: {
          actionPlaceholder: 'Filter by action',
          targetTypePlaceholder: 'Filter by target type'
        },
        table: {
          id: 'ID',
          action: 'Action',
          module: 'Module',
          targetType: 'Target Type',
          username: 'Username',
          content: 'Content',
          ip: 'IP',
          status: 'Status',
          createdAt: 'Created At'
        },
        pagination: {
          total: 'Total: {total}',
          pageSize: 'Page size',
          page: 'Page {page} / {totalPages}',
          prev: 'Previous',
          next: 'Next'
        },
        empty: 'No audit logs'
      },
      configModal: {
        titleEdit: 'Edit Config',
        titleCreate: 'Create Config',
        key: 'Key',
        value: 'Value',
        description: 'Description',
        keyPlaceholder: 'For example: site.name',
        valuePlaceholder: 'Config value, JSON is supported',
        descriptionPlaceholder: 'Config description',
        sensitiveHintWithValue: 'Sensitive value is hidden. Leave blank to keep the current value, or enter a new value to replace it.',
        sensitiveHintWithoutValue: 'This is a sensitive key. Enter a value to set it.'
      },
      balancerModal: {
        titleEdit: 'Edit Load Balancer',
        titleCreate: 'Create Load Balancer',
        name: 'Name',
        namePlaceholder: 'Load balancer name',
        groupId: 'Node Group ID',
        strategy: 'Strategy',
        healthCheck: 'Enable health check',
        checkInterval: 'Check Interval (seconds)',
        weightsJson: 'Node Weights (JSON)',
        weightsPlaceholder: '{"1": 10, "2": 5}'
      },
      strategy: {
        roundRobin: 'Round Robin',
        leastLoad: 'Least Load',
        latency: 'Lowest Latency',
        weight: 'Weighted',
        random: 'Random'
      },
      status: {
        pending: 'Pending',
        completed: 'Completed',
        failed: 'Failed'
      },
      booleans: {
        enabled: 'Enabled',
        disabled: 'Disabled'
      },
      messages: {
        fetchConfigsFailed: 'Failed to load configs',
        saveConfigFailed: 'Save failed: {message}',
        deleteConfigConfirm: 'Delete config {key}?',
        deleteConfigFailed: 'Delete failed',
        fetchBackupConfigFailed: 'Failed to load backup config',
        backupConfigSaved: 'Saved successfully',
        backupConfigSaveFailed: 'Save failed',
        backupStarted: 'Backup started',
        backupStartFailed: 'Failed to create backup',
        fetchBackupsFailed: 'Failed to load backup list',
        fetchBackupStatsFailed: 'Failed to load backup stats',
        deleteBackupConfirm: 'Delete backup {filename}?',
        deleteBackupFailed: 'Delete failed',
        restoreBackupConfirm: 'Restore backup {filename}? Current data will be overwritten.',
        restoreBackupSuccess: 'Restore completed',
        restoreBackupFailed: 'Restore failed: {message}',
        fetchBalancersFailed: 'Failed to load load balancers',
        weightsJsonInvalid: 'Weights JSON is invalid',
        saveBalancerFailed: 'Save failed: {message}',
        deleteBalancerConfirm: 'Delete load balancer {name}?',
        deleteBalancerFailed: 'Delete failed',
        healthCheckCompleted: 'Health check completed',
        healthCheckFailed: 'Health check failed',
        fetchAuditLogsFailed: 'Failed to load audit logs'
      }
    },
    nodeXTopology: {
      heroEyebrow: 'NodeX Topology',
      title: 'NodeX Topology + Legacy Rules',
      heroText: 'This page owns the stateful NodeX relay/exit topology and the legacy-rules compatibility layer. Stateless execution hosts belong on Ansible Machines.',
      nodesEyebrow: 'Nodes',
      nodesTitle: 'NodeX Relay / Exit Topology',
      nodesText: 'Use this page only for stateful NodeX relay/exit topology, reachability checks, and gost API operations. It does not manage Ansible machines.',
      loading: 'Loading NodeX topology nodes...',
      emptyTitle: 'No NodeX topology nodes yet.',
      emptyText: 'Create relay / exit nodes here for NodeX mode. If you only need stateless execution, use Ansible Machines.',
      legacyText: 'This block mirrors the `/admin/forward/rules*` compatibility endpoints. It preserves legacy rule behavior but does not define the current primary runtime path for NodeX or Ansible.',
      actions: {
        refresh: 'Refresh',
        testConnection: 'Test Connection',
        addLegacyRule: 'Add Legacy Rule',
        addNode: 'Add Node',
        query: 'Query',
        clear: 'Clear',
        addRule: 'Add Rule',
        edit: 'Edit',
        healthCheck: 'Health Check',
        checking: 'Checking...',
        syncStats: 'Sync Stats',
        syncing: 'Syncing...',
        enable: 'Enable',
        disable: 'Disable',
        delete: 'Delete',
        startTest: 'Start Test',
        confirmDelete: 'Confirm Delete',
        cancel: 'Cancel',
        save: 'Save',
        saveChanges: 'Save Changes',
        createNode: 'Create Node',
        createRule: 'Create Rule'
      },
      filters: {
        nodeType: 'Node Type',
        status: 'Status',
        all: 'All',
        online: 'Online',
        offline: 'Offline',
        userId: 'User ID',
        userIdPlaceholder: 'Filter by User ID',
        relay: 'Relay',
        exit: 'Exit'
      },
      status: {
        enabled: 'Enabled',
        disabled: 'Disabled',
        online: 'Online',
        offline: 'Offline',
        success: 'Success',
        failed: 'Failed',
        operationSuccess: 'Operation succeeded',
        operationFailed: 'Operation failed'
      },
      meta: {
        managementApi: 'Management API',
        regionIsp: 'Region / ISP',
        latency: 'Latency',
        currentConnections: 'Current Connections',
        traffic: 'Upload / Download',
        weightMaxConnections: 'Weight / Max Connections',
        lastCheck: 'Last Check',
        uptime: 'Uptime',
        rateLimit: 'Rate',
        trafficLimit: 'Traffic',
        expire: 'Expire',
        upload: 'Upload',
        download: 'Download',
        connections: 'Connections',
        serviceCount: 'Service Count'
      },
      stats: {
        relayNodes: 'Relay Nodes',
        exitNodes: 'Exit Nodes',
        totalNodes: 'Total Nodes',
        onlineNodes: 'Online Nodes',
        totalUpload: 'Total Upload',
        totalDownload: 'Total Download',
        onlineCount: 'Online {count}',
        includesRelayExit: 'Includes Relay / Exit',
        refreshing: 'Refreshing stats...',
        basedOnLastCheck: 'Based on latest health check',
        aggregatedAcrossNodes: 'Aggregated across all topology nodes'
      },
      pagination: {
        prev: 'Previous',
        next: 'Next',
        summary: 'Page {page} / {totalPages}, total {total} items'
      },
      legacy: {
        eyebrow: 'Legacy Rules',
        title: 'Legacy Port Forward Rules',
        text: 'This section mirrors `/admin/forward/rules*` compatibility APIs to preserve legacy relay + exit forwarding behavior.',
        loading: 'Loading legacy rules...',
        emptyTitle: 'No legacy rules yet',
        emptyText: 'Add rules here if you need compatibility for relay + exit port-level forwarding.',
        columns: {
          id: 'ID',
          name: 'Name',
          ingress: 'Ingress',
          egress: 'Egress',
          owner: 'Owner',
          limits: 'Limits',
          traffic: 'Traffic',
          status: 'Status',
          actions: 'Actions'
        }
      },
      nodeModal: {
        eyebrow: 'Node',
        titleEdit: 'Edit Relay/Exit Node',
        titleAdd: 'Add Relay/Exit Node',
        loading: 'Loading node detail...',
        saveLoading: 'Saving...',
        fields: {
          name: 'Node Name',
          type: 'Node Type',
          host: 'Host',
          servicePort: 'Service Port',
          apiPort: 'API Port',
          apiToken: 'API Token',
          region: 'Region',
          isp: 'ISP',
          bandwidth: 'Bandwidth (Mbps)',
          maxConnections: 'Max Connections',
          weight: 'Weight'
        },
        placeholders: {
          name: 'e.g. relay-hk-01',
          host: '1.2.3.4',
          apiToken: 'Leave blank to auto-generate',
          region: 'HK / JP / US',
          isp: 'CMI / NTT / Cogent'
        },
        hints: {
          apiPort: 'Required for NodeX management API health checks, stats sync, and connection tests.'
        }
      },
      ruleModal: {
        eyebrow: 'Rule',
        titleEdit: 'Edit Legacy Rule',
        titleAdd: 'Add Legacy Rule',
        loading: 'Loading rule detail...',
        saveLoading: 'Saving...',
        ownerReadOnlyHint: 'Current backend update API does not allow changing ownership fields. They are read-only in edit mode.',
        fields: {
          name: 'Rule Name',
          protocol: 'Protocol',
          relayNode: 'Ingress Relay Node',
          listenPort: 'Listen Port',
          exitNode: 'Egress Exit Node',
          targetPort: 'Target Port',
          targetHost: 'Target Host',
          userId: 'User ID',
          userGroupId: 'User Group ID',
          speedLimit: 'Speed Limit (KB/s)',
          trafficLimit: 'Traffic Limit (Bytes)',
          expireTime: 'Expire Time',
          remark: 'Remark'
        },
        placeholders: {
          name: 'e.g. tcp-11111-hk',
          relayNode: 'Select Relay Node',
          exitNode: 'Select Exit Node',
          targetHost: '127.0.0.1 or target host at destination side',
          userId: 'Blank means public rule',
          userGroupId: 'Choose either User ID or Group ID',
          remark: 'Record business purpose or maintenance notes'
        }
      },
      connectionModal: {
        eyebrow: 'Gost API',
        title: 'Test Node Connection',
        fields: {
          host: 'Host',
          apiPort: 'API Port',
          apiToken: 'API Token'
        },
        placeholders: {
          host: '127.0.0.1',
          apiToken: 'Leave blank if auth is disabled'
        },
        success: 'Connection succeeded',
        failed: 'Connection failed',
        testing: 'Testing...'
      },
      deleteModal: {
        title: 'Confirm Delete',
        confirmNode: 'Confirm deleting node',
        confirmRule: 'Confirm deleting rule',
        warning: 'Deletion cannot be automatically reverted. Ensure no active forwarding relationships still depend on it.',
        deleting: 'Deleting...'
      },
      validation: {
        requestFailed: 'Request failed',
        nodeNameRequired: 'Node name is required',
        nodeHostRequired: 'Host is required',
        nodePortRange: 'Service port must be between 1 and 65535',
        nodeApiPortRequired: 'NodeX management API port is required',
        nodeApiPortRange: 'API port must be between 1 and 65535',
        ruleNameRequired: 'Rule name is required',
        relayNodeRequired: 'Please select an ingress Relay node',
        exitNodeRequired: 'Please select an egress Exit node',
        listenPortRange: 'Listen port must be between 1 and 65535',
        targetHostRequired: 'Target host is required',
        targetPortRange: 'Target port must be between 1 and 65535',
        ownerConflict: 'User ID and User Group ID cannot both be set',
        connectionHostRequired: 'Host is required',
        connectionApiPortRange: 'API port must be between 1 and 65535',
        userIdPositive: 'User ID must be a positive integer'
      },
      messages: {
        loadStatsFailed: 'Failed to load forward statistics',
        loadNodesFailed: 'Failed to load relay/exit nodes',
        loadNodeOptionsFailed: 'Failed to load node options',
        loadRulesFailed: 'Failed to load forward rules',
        loadNodeDetailFailed: 'Failed to load node detail',
        saveNodeFailed: 'Failed to save relay/exit node',
        nodeUpdated: 'Relay/exit node updated',
        nodeCreated: 'Relay/exit node created',
        nodeDeleted: 'Relay/exit node deleted',
        nodeCheckFailed: 'Health check failed',
        nodeSyncFailed: 'Sync stats failed',
        nodeToggleFailed: 'Failed to toggle node status',
        loadRuleDetailFailed: 'Failed to load rule detail',
        saveRuleFailed: 'Failed to save forward rule',
        ruleUpdated: 'Forward rule updated',
        ruleCreated: 'Forward rule created',
        ruleDeleted: 'Forward rule deleted',
        ruleToggleFailed: 'Failed to toggle rule status',
        connectionFailed: 'Connection check failed',
        deleteFailed: 'Delete failed',
        nodeReachable: 'Node reachable',
        nodeUnavailable: 'Node unavailable',
        syncSuccess: 'Stats synced',
        connectionSuccess: 'Connection succeeded',
        connectionError: 'Connection failed',
        nodeEnabled: '{name} enabled',
        nodeDisabled: '{name} disabled',
        ruleEnabled: '{name} enabled',
        ruleDisabled: '{name} disabled',
        latency: 'Latency {value} ms',
        currentConnectionsSuffix: ', current connections {value}'
      },
      owner: {
        user: 'User #{id}',
        userGroup: 'User Group #{id}',
        public: 'Public Rule'
      },
      protocols: {
        tcp: 'TCP',
        udp: 'UDP',
        both: 'TCP + UDP'
      },
      labels: {
        node: 'Node #{id}',
        route: 'Route',
        none: 'Unlimited',
        neverExpires: 'Never Expires'
      }
    },
    limitPage: {
      heroEyebrow: 'Speed Limit Management',
      title: 'Limits',
      subtitle: 'Maintain rate limit rules per tunnel while keeping the dedicated Flux-style rule page.',
      note: 'Speed limit rules are enforced by the active forwarding runtime. Changes may take effect after a short delay.',
      actions: {
        refresh: 'Refresh',
        create: 'Create',
        createNow: 'Create Now',
        edit: 'Edit',
        delete: 'Delete'
      },
      loading: 'Loading limit rules...',
      empty: {
        title: 'No limit rules yet',
        text: 'No limit rules have been created yet. Use the button above to create the first one.'
      },
      status: {
        active: 'Running',
        error: 'Error'
      },
      cards: {
        speed: 'Speed Limit',
        tunnel: 'Bound Tunnel',
        updatedAt: 'Updated At'
      },
      formModal: {
        titleCreate: 'Create Limit Rule',
        titleEdit: 'Edit Limit Rule',
        fields: {
          name: 'Rule Name',
          speed: 'Speed Limit',
          tunnel: 'Bound Tunnel'
        },
        placeholders: {
          name: 'Enter a limit rule name',
          speed: 'Enter speed limit (Mbps)',
          tunnel: 'Select a tunnel to bind'
        },
        submitting: 'Submitting...',
        submitCreate: 'Create Rule',
        submitUpdate: 'Save Changes'
      },
      deleteModal: {
        title: 'Delete Rule',
        eyebrow: 'Confirm Deletion',
        confirmText: 'Delete limit rule {name}?',
        hint: 'This action cannot be undone. The rule will be permanently removed.',
        deleting: 'Deleting...',
        confirmDelete: 'Confirm Delete'
      },
      values: {
        unlimited: 'Unlimited',
        tunnelFallback: 'Tunnel #{id}',
        ruleFallback: 'Rule #{id}'
      },
      modeLabelNodeX: 'NodeX Mode',
      modeSummaryNodeX: 'Speed limits are synchronized and enforced by NodeX agents.',
      modeLabelLocal: 'Local Mode',
      modeSummaryLocal: 'Speed limits are applied locally via {backend} runtime.',
      messages: {
        fetchTunnelsFailed: 'Failed to load tunnel list',
        fetchRulesFailed: 'Failed to load limit rules',
        loadFailed: 'Failed to load data',
        nameRequired: 'Rule name is required',
        nameLength: 'Rule name must be between 2 and 50 characters',
        speedInvalid: 'Enter a valid speed limit (>= 1 Mbps)',
        tunnelRequired: 'Please select a tunnel to bind',
        tunnelMissing: 'Tunnel name is missing. Refresh and try again.',
        createFailed: 'Failed to create limit rule',
        updateFailed: 'Failed to update limit rule',
        submitFailed: 'Submission failed',
        deleteFailed: 'Failed to delete limit rule',
        created: 'Limit rule created successfully',
        updated: 'Limit rule updated successfully',
        deleted: 'Limit rule deleted successfully'
      }
    },
    nodeXAgents: {
      title: 'NodeX Agents',
      subtitle: 'Only NodeX mode needs agents. Use this page for agent status, remote terminal, and task delivery.',
      tabs: {
        agents: 'Online Agents',
        terminal: 'Remote Terminal',
        tasks: 'Task History'
      },
      actions: {
        refresh: 'Refresh',
        execute: 'Execute',
        send: 'Send',
        cancel: 'Cancel',
        monitor: 'Monitor',
        terminalShort: 'TTY',
        taskShort: 'Task',
        monitorShort: 'Mon'
      },
      table: {
        nodeId: 'Node ID',
        version: 'Version',
        system: 'System',
        lastSeen: 'Last Seen',
        status: 'Status',
        capabilities: 'Capabilities',
        action: 'Actions',
        taskId: 'Task ID',
        node: 'Node',
        type: 'Type',
        command: 'Command / Action',
        duration: 'Duration',
        time: 'Time'
      },
      status: {
        online: 'Online',
        offline: 'Offline',
        connected: 'Connected',
        disconnected: 'Disconnected',
        success: 'Success',
        failed: 'Failed'
      },
      empty: {
        agents: 'No online agents',
        tasks: 'No task history yet.'
      },
      terminal: {
        chooseNode: 'Choose node',
        nodeLabel: 'Node #{id}',
        promptPlaceholder: 'Enter command...'
      },
      taskModal: {
        title: 'Send Task',
        targetNode: 'Target Node',
        taskType: 'Task Type',
        action: 'Action',
        paramsJson: 'Params (JSON)',
        paramsPlaceholder: '{"key": "value"}',
        timeoutSeconds: 'Timeout (seconds)'
      },
      taskTypes: {
        command: 'Execute command',
        file: 'File operation',
        service: 'Service management',
        gost: 'GOST management'
      },
      hints: {
        monitor: 'View monitoring data for node #{id}'
      },
      messages: {
        fetchFailed: 'Failed to fetch agent list',
        taskIncomplete: 'Please fill in the required fields',
        invalidParamsJson: 'Params JSON is invalid',
        taskSent: 'Task sent',
        taskSendFailed: 'Send failed: {message}',
        commandError: 'Error: {message}'
      }
    }
  },
  admin: {
    subscriptions: {
      title: 'Subscription Management',
      subtitle: 'Manage subscription groups, templates, and linked production node protocols.',
      groups: 'Groups',
      createGroup: 'Create Group',
      editGroup: 'Edit Group',
      groupName: 'Group Name',
      groupNamePlaceholder: 'Enter group name',
      description: 'Description',
      priority: 'Priority',
      enabled: 'Enabled',
      disabled: 'Disabled',
      noDescription: 'No description',
      templates: 'Templates',
      templatesFor: 'Templates For',
      createTemplate: 'Create Template',
      editTemplate: 'Edit Template',
      copySubscription: 'Copy Subscription Links',
      copyCombinedSubscription: 'Copy Combined Subscription',
      preview: 'Preview',
      previewTitle: 'Subscription Preview',
      format: 'Format',
      copyContent: 'Copy Content',
      download: 'Download',
      subscriptionLinks: 'Subscription Links',
      nodeName: 'Node Name',
      nodeNamePlaceholder: 'US Node',
      protocol: 'Protocol',
      server: 'Server',
      port: 'Port',
      tls: 'TLS',
      tlsNone: 'None',
      tlsReality: 'Reality',
      tlsEnabled: 'TLS',
      status: 'Status',
      actions: 'Actions',
      productionNodes: 'Production Node Protocols',
      manageRelations: 'Manage Links',
      linkedProtocolsInfo: 'Protocols already linked to this group',
      visibilityShown: 'Shown',
      visibilityHidden: 'Hidden',
      protocolOnline: 'Online',
      protocolOffline: 'Offline',
      goToNode: 'Go to node management',
      productionNodesEmpty: 'No production node protocols are linked to this group yet.',
      manageProtocolsTitle: 'Manage Production Node Protocol Links',
      manageProtocolsDescription: 'Select which node protocols should be included in the "{group}" group. Only protocols marked as visible in Node Management appear here.',
      unknownNode: 'Unknown Node',
      confirmSave: 'Confirm Save',
      transport: 'Transport',
      sni: 'SNI (Server Name)',
      realityPublicKey: 'Reality Public Key',
      realityShortId: 'Reality Short ID',
      tlsFingerprint: 'TLS Fingerprint',
      defaultOption: 'Default',
      websocketPath: 'WebSocket Path',
      flow: 'Flow',
      noneOption: 'None',
      productionTable: {
        node: 'Node',
        protocol: 'Protocol',
        name: 'Name',
        port: 'Port',
        visibility: 'Visibility',
        status: 'Status',
        actions: 'Actions'
      },
      protocolPool: {
        node: 'Node',
        protocolName: 'Protocol / Name',
        port: 'Port',
        linkedGroups: 'Linked Groups'
      },
      formats: {
        auto: 'Auto (By User-Agent)',
        v2ray: 'V2Ray (Base64)',
        clash: 'Clash (YAML)',
        stash: 'Stash (YAML)',
        egern: 'Egern (YAML)',
        surge: 'Surge',
        loon: 'Loon',
        shadowrocket: 'ShadowRocket',
        quantumultx: 'QuantumultX',
        json: 'JSON',
        base64json: 'Base64 JSON'
      },
      loadError: 'Failed to load subscription data',
      availableProtocolsLoadError: 'Failed to load available protocols',
      groupProtocolsUpdated: 'Group protocol links updated',
      groupProtocolsUpdateFailed: 'Failed to update protocol links',
      copyCombinedConfirm: 'Copy the combined subscription content for this group?',
      copied: 'Copied',
      copyError: 'Copy failed',
      copyFallbackNotice: 'Copied merged subscription content using the template-only fallback.',
      previewError: 'Failed to load preview content',
      confirmDeleteGroup: 'Delete this subscription group?',
      groupDeleted: 'Subscription group deleted',
      deleteError: 'Delete failed',
      groupSaved: 'Subscription group saved',
      saveError: 'Save failed',
      confirmDeleteTemplate: 'Delete this subscription template?',
      templateDeleted: 'Subscription template deleted',
      updateError: 'Update failed',
      templateSaved: 'Subscription template saved'
    },
    nodes: {
      title: 'Node Management',
      authKeys: 'Auth Keys',
      addNode: 'Add Node',
      stats: {
        total: 'Total Nodes',
        online: 'Online',
        offline: 'Offline',
        pending: 'Pending Activation'
      },
      table: {
        name: 'Name',
        address: 'Address',
        status: 'Status',
        protocols: 'Protocols',
        traffic: 'Traffic Today',
        lastHeartbeat: 'Last Heartbeat',
        actions: 'Actions',
        loading: 'Loading...',
        empty: 'No nodes yet'
      },
      actions: {
        manageProtocols: 'Manage protocols',
        protocols: 'Protocols',
        edit: 'Edit',
        delete: 'Delete',
        cancel: 'Cancel',
        save: 'Save',
        saving: 'Saving...',
        authKey: 'Auth Key'
      },
      pagination: {
        previous: 'Previous',
        next: 'Next'
      },
      nodeModal: {
        titleCreate: 'Add Node',
        titleEdit: 'Edit Node',
        fields: {
          name: 'Node Name *',
          address: 'Node Address *',
          tags: 'Tags (comma separated)',
          rate: 'Node Rate',
          sort: 'Sort',
          status: 'Status'
        },
        placeholders: {
          name: 'Enter node name',
          address: 'IP or domain',
          tags: 'HK,IEPL,Premium',
          rate: '1.0',
          sort: '0'
        }
      },
      authKeyModal: {
        title: 'Node Auth Key',
        hint: 'Configure this key in V2bX config.json. The node will auto-register on startup. The same key can register unlimited nodes.',
        noKey: 'No auth key generated yet. Please create an auth key first.',
        copy: 'Copy Key',
        copyConfig: 'Copy Config',
        configHint: 'Paste this config into V2bX config.json, replace <auth_key> with the key value above.',
        registeredCount: 'Registered Nodes',
        copied: 'Copied'
      },
      protocolModal: {
        title: 'Protocol Management - {name}',
        addProtocol: 'Add Protocol',
        table: {
          type: 'Protocol Type',
          port: 'Port',
          status: 'Status',
          actions: 'Actions'
        },
        status: {
          enabled: 'Enabled',
          disabled: 'Disabled'
        },
        empty: 'No protocol configuration yet'
      },
      protocolForm: {
        titleCreate: 'Add Protocol',
        titleEdit: 'Edit Protocol',
        templateLibrary: 'Protocol Template Library',
        tabs: {
          json: 'JSON',
          visual: 'Visual Config'
        },
        fields: {
          type: 'Protocol Type *',
          port: 'Listen Port *',
          tls: 'TLS Mode',
          transport: 'Transport',
          settings: 'Protocol Settings (JSON)',
          tlsSettings: 'TLS Settings (JSON)',
          realitySettings: 'Reality Settings (JSON)',
          transportSettings: 'Transport Settings (JSON)'
        },
        placeholders: {
          port: '443'
        },
        jsonActions: {
          format: 'Format',
          copy: 'Copy',
          fromTemplate: 'Load from Template'
        },
        jsonStatus: {
          valid: 'JSON Valid',
          invalid: 'JSON Invalid'
        },
        enable: 'Enable protocol (run on node)',
        show: 'Show in subscription protocol pool'
      },
      statusText: {
        pending: 'Pending Activation',
        online: 'Online',
        offline: 'Offline',
        disabled: 'Disabled',
        unknown: 'Unknown'
      },
      tlsModes: {
        none: 'No TLS',
        standard: 'Standard TLS',
        reality: 'Reality (Recommended)'
      },
      transports: {
        tcp: 'TCP',
        ws: 'WebSocket',
        grpc: 'gRPC',
        quic: 'QUIC',
        h2: 'HTTP/2'
      },
      relativeTime: {
        justNow: 'Just now',
        minutesAgo: '{count} min ago',
        hoursAgo: '{count} hr ago'
      },
      messages: {
        requiredFields: 'Please fill in the required fields',
        saveFailed: 'Save failed: {message}',
        deleteNodeConfirm: 'Delete node "{name}"?',
        deleteFailed: 'Delete failed: {message}',
        deleteProtocolConfirm: 'Delete this protocol?',
        generateFailed: 'Generation failed: {message}',
        deleteAuthKeyConfirm: 'Delete this auth key?',
        copied: 'Copied to clipboard',
        copyFailed: 'Copy failed: {message}',
        invalidJson: 'Invalid JSON'
      }
    }
  },
  adminNotifications: {
    title: 'Notifications',
    subtitle: 'Manage notification templates, SMTP delivery, and send logs.',
    tabs: {
      templates: 'Templates',
      email: 'Email',
      logs: 'Logs'
    },
    actions: {
      createTemplate: 'Create template',
      edit: 'Edit',
      delete: 'Delete',
      search: 'Search',
      sendTest: 'Send test email'
    },
    templates: {
      table: {
        id: 'ID',
        name: 'Name',
        type: 'Type',
        event: 'Trigger Event',
        status: 'Status',
        actions: 'Actions'
      },
      empty: 'No notification templates'
    },
    email: {
      title: 'SMTP Configuration',
      fields: {
        host: 'SMTP Host',
        port: 'Port',
        username: 'Username',
        password: 'Password',
        fromName: 'From Name',
        fromAddress: 'From Address',
        encryption: 'Enable TLS encryption'
      },
      placeholders: {
        host: 'smtp.example.com',
        port: '465',
        username: 'your@email.com',
        password: 'Enter SMTP password',
        fromName: 'V2Board',
        fromAddress: 'noreply@example.com'
      }
    },
    logs: {
      filters: {
        allTypes: 'All types',
        allStatuses: 'All statuses'
      },
      table: {
        id: 'ID',
        type: 'Type',
        recipient: 'Recipient',
        title: 'Title',
        status: 'Status',
        sentAt: 'Sent At'
      },
      empty: 'No notification logs'
    },
    modal: {
      createTitle: 'Create Template',
      editTitle: 'Edit Template',
      fields: {
        name: 'Name',
        type: 'Type',
        event: 'Trigger Event',
        title: 'Title Template',
        content: 'Content Template',
        enabled: 'Enabled'
      },
      placeholders: {
        name: 'Template name',
        title: 'Supports variables: {username}, {site_name}',
        content: 'Supports variables: {username}, {email}, {expire_time}'
      }
    },
    testModal: {
      title: 'Send Test Email',
      fields: {
        recipient: 'Recipient Email'
      },
      placeholders: {
        recipient: 'test@example.com'
      },
      actions: {
        send: 'Send'
      }
    },
    types: {
      email: 'Email',
      telegram: 'Telegram',
      webhook: 'Webhook'
    },
    events: {
      userRegister: 'User Register',
      userLogin: 'User Login',
      userExpire: 'User Expire',
      userTrafficLow: 'Low Traffic',
      orderPaid: 'Order Paid',
      ticketReply: 'Ticket Reply',
      nodeOffline: 'Node Offline',
      nodeOnline: 'Node Online'
    },
    status: {
      enabled: 'Enabled',
      disabled: 'Disabled',
      pending: 'Pending',
      success: 'Success',
      failed: 'Failed'
    },
    testPayload: {
      subject: 'Test Email',
      content: 'This is a test email. If you received it, the email configuration is working correctly.'
    },
    messages: {
      fetchTemplatesFailed: 'Failed to load notification templates',
      fetchLogsFailed: 'Failed to load notification logs',
      fetchEmailConfigFailed: 'Failed to load email configuration',
      templateSaveSuccess: 'Template saved successfully',
      templateSaveFailed: 'Failed to save template: {message}',
      templateSaveFailedShort: 'Save failed',
      deleteConfirm: 'Delete template "{name}"?',
      deleteFailed: 'Failed to delete template: {message}',
      deleteFailedShort: 'Delete failed',
      emailSaveSuccess: 'Email configuration saved',
      emailSaveFailed: 'Failed to save email configuration: {message}',
      emailSaveFailedShort: 'Save failed',
      testRecipientRequired: 'Please enter a recipient email address',
      testSendSuccess: 'Test email sent successfully',
      testSendFailed: 'Failed to send test email: {message}',
      testSendFailedShort: 'Send failed'
    }
  },
  adminPayment: {
    title: 'Payment Gateway Management',
    subtitle: 'Manage gateways, payment records, and aggregated stats.',
    currencySymbol: '¥',
    tabs: {
      gateways: 'Gateways',
      records: 'Records',
      stats: 'Stats'
    },
    actions: {
      createGateway: 'Create gateway',
      enable: 'Enable',
      disable: 'Disable',
      edit: 'Edit',
      delete: 'Delete',
      search: 'Search',
      details: 'Details'
    },
    gateways: {
      table: {
        id: 'ID',
        name: 'Name',
        type: 'Type',
        feeRate: 'Fee Rate',
        minAmount: 'Min Amount',
        maxAmount: 'Max Amount',
        status: 'Status',
        actions: 'Actions'
      },
      empty: 'No payment gateways'
    },
    records: {
      filters: {
        allStatuses: 'All statuses',
        allTypes: 'All types'
      },
      table: {
        id: 'ID',
        tradeNo: 'Trade No.',
        userId: 'User ID',
        gateway: 'Gateway',
        amount: 'Amount',
        status: 'Status',
        createdAt: 'Created At',
        actions: 'Actions'
      },
      detail: {
        title: 'Payment Details',
        tradeNo: 'Trade No.',
        amount: 'Amount',
        status: 'Status'
      },
      empty: 'No payment records'
    },
    stats: {
      totalAmount: 'Total Revenue',
      totalOrders: 'Total Orders',
      successOrders: 'Successful Orders',
      successRate: 'Success Rate',
      gatewayDistribution: 'Gateway Distribution',
      empty: 'No gateway statistics yet'
    },
    modal: {
      createTitle: 'Create Gateway',
      editTitle: 'Edit Gateway',
      fields: {
        name: 'Name',
        type: 'Type',
        feeRate: 'Fee Rate',
        minAmount: 'Min Amount',
        maxAmount: 'Max Amount',
        configJson: 'Configuration (JSON)'
      },
      placeholders: {
        name: 'Gateway name',
        feeRate: 'Example: 0.01 = 1%',
        minAmount: 'Minimum payment amount',
        maxAmount: 'Maximum payment amount',
        configJson: '{"app_id": "", "private_key": ""}'
      }
    },
    types: {
      alipay: 'Alipay',
      wechat: 'WeChat Pay',
      stripe: 'Stripe',
      usdt: 'USDT',
      epay: 'EPay'
    },
    status: {
      enabled: 'Enabled',
      disabled: 'Disabled',
      pending: 'Pending',
      paid: 'Paid',
      failed: 'Failed',
      refunded: 'Refunded'
    },
    messages: {
      fetchGatewaysFailed: 'Failed to load payment gateways',
      fetchRecordsFailed: 'Failed to load payment records',
      fetchStatsFailed: 'Failed to load payment statistics',
      invalidConfigJson: 'Configuration JSON is invalid',
      gatewaySaveSuccess: 'Gateway saved successfully',
      gatewaySaveFailed: 'Failed to save gateway: {message}',
      gatewaySaveFailedShort: 'Save failed',
      toggleFailed: 'Failed to change gateway status: {message}',
      toggleFailedShort: 'Operation failed',
      deleteConfirm: 'Delete gateway "{name}"?',
      deleteFailed: 'Failed to delete gateway: {message}',
      deleteFailedShort: 'Delete failed'
    }
  },
  adminMfa: {
    title: 'Multi-Factor Authentication Management',
    subtitle: 'Configure the global MFA policy.',
    config: {
      title: 'Global Configuration',
      enabled: 'Enable multi-factor authentication',
      enabledHelp: 'Once enabled, users can choose to turn on MFA to protect account security.',
      required: 'Require MFA',
      requiredHelp: 'Require all users to enable MFA, otherwise they cannot use the service.',
      methods: 'Supported authentication methods',
      backupCodesCount: 'Backup code count',
      backupCodesHelp: 'Number of backup codes generated when users enable MFA.',
      maxAttempts: 'Maximum login attempts',
      maxAttemptsHelp: 'Accounts will be temporarily locked after exceeding the limit.',
      lockoutDuration: 'Lockout duration (minutes)',
      lockoutDurationHelp: 'Lockout duration after login failures exceed the limit.'
    },
    methods: {
      totp: 'TOTP (Google Authenticator / Authy)',
      sms: 'SMS verification code',
      email: 'Email verification code'
    },
    info: {
      title: '💡 Usage Guide',
      totpTitle: 'TOTP Authentication',
      totpBody: 'Time-based one-time passwords. Users can scan a QR code with apps such as Google Authenticator or Authy to bind MFA.',
      backupTitle: 'Backup Codes',
      backupBody: 'When users cannot access their authenticator, they can use backup codes to log in. Each backup code can only be used once.',
      lockoutTitle: 'Account Lockout',
      lockoutBody: 'Multiple consecutive MFA failures will trigger an account lockout to prevent brute-force attacks.',
      userOpsTitle: 'User Operations',
      userOpsBody: 'Users can manage MFA on the Security Settings page, including enabling, disabling, and regenerating backup codes.'
    },
    messages: {
      fetchFailed: 'Failed to load MFA configuration',
      saveSuccess: 'Saved successfully',
      saveFailed: 'Save failed: {message}',
      saveFailedShort: 'Save failed'
    }
  },
  adminUsers: {
    title: 'User Management',
    subtitle: 'Manage all registered users.',
    stats: {
      totalUsers: 'Total users',
      activeUsers: 'Active users',
      expiredUsers: 'Expired users',
      bannedUsers: 'Banned users'
    },
    filters: {
      searchEmail: 'Search email...',
      allStatus: 'All status'
    },
    table: {
      id: 'ID',
      email: 'Email',
      plan: 'Plan',
      traffic: 'Traffic',
      expireAt: 'Expires at',
      status: 'Status',
      createdAt: 'Created at',
      actions: 'Actions'
    },
    status: {
      active: 'Active',
      expired: 'Expired',
      banned: 'Banned'
    },
    actions: {
      search: 'Search',
      addUser: 'Add user',
      editUser: 'Edit user',
      manageTunnel: 'Manage tunnel grants',
      manageTunnelShort: 'Tunnel',
      ban: 'Ban',
      unban: 'Unban',
      resetTraffic: 'Reset traffic',
      resetShort: 'Reset'
    },
    empty: {
      noData: 'No data'
    },
    pagination: {
      prev: 'Previous',
      next: 'Next',
      info: 'Page {page} / {totalPages}'
    },
    editModal: {
      title: 'Edit User',
      fields: {
        email: 'Email',
        balance: 'Balance (cents)',
        transfer: 'Traffic limit (bytes)',
        groupId: 'Subscription group',
        expiredAt: 'Expire time (Unix seconds)',
        flowResetTime: 'Flow reset day',
        remark: 'Remark'
      },
      groupOptions: {
        unassigned: 'Unassigned'
      }
    },
    createModal: {
      title: 'Add User',
      creating: 'Creating...',
      fields: {
        email: 'Email',
        password: 'Password',
        userType: 'User type',
        groupId: 'Subscription group',
        transferEnable: 'Traffic limit (bytes)',
        flowResetTime: 'Flow reset day'
      },
      placeholders: {
        email: 'Enter email',
        password: 'Enter password (min 6 chars)',
        transferEnable: 'Leave empty for default 0'
      },
      groupOptions: {
        unassigned: 'Unassigned'
      },
      userTypes: {
        normal: 'Normal user',
        admin: 'Admin'
      }
    },
    tunnelModal: {
      title: 'Tunnel Grants - {email}',
      sections: {
        form: 'Grant form',
        list: 'Current grants'
      },
      fields: {
        tunnel: 'Tunnel',
        tunnelReadonlyHint: ' (read-only while editing)',
        status: 'Status',
        flowQuota: 'Flow quota',
        numQuota: 'Quantity quota',
        expTime: 'Expire time',
        flowResetTime: 'Flow reset time',
        rateLimit: 'Rate limit'
      },
      options: {
        noAssignableTunnel: 'No assignable tunnel',
        selectTunnel: 'Select tunnel',
        selectTunnelFirst: 'Select tunnel first',
        noRateLimitRules: 'No rate limit rules for current tunnel'
      },
      actions: {
        cancelEdit: 'Cancel edit',
        submitting: 'Submitting...',
        updateGrant: 'Update grant',
        addGrant: 'Add grant'
      },
      table: {
        id: 'ID',
        tunnel: 'Tunnel',
        status: 'Status',
        flow: 'Flow',
        num: 'Quantity',
        expireAt: 'Expires at',
        reset: 'Reset',
        usedFlow: 'Used flow',
        rateLimit: 'Rate limit'
      },
      empty: 'No grants'
    },
    resetFlow: {
      userTitle: 'Reset User Traffic',
      userMessage: 'Confirm reset used traffic for user {email}?',
      tunnelTitle: 'Reset Tunnel Grant Traffic',
      tunnelMessage: 'Confirm reset used traffic for tunnel grant #{id}?',
      usedFlow: 'Used flow',
      quota: 'Quota',
      resetting: 'Resetting...',
      confirmAction: 'Confirm reset'
    },
    labels: {
      admin: 'Admin',
      noLimit: 'No limit',
      noReset: 'No reset',
      monthlyDay: 'Day {day} of every month',
      permanent: 'Permanent'
    },
    messages: {
      actionFailed: 'Operation failed',
      fillEmailPassword: 'Please fill in email and password',
      passwordTooShort: 'Password must be at least 6 characters',
      userCreated: 'User created successfully',
      createFailed: 'Create failed',
      fetchUsersFailed: 'Failed to fetch users',
      fetchStatsFailed: 'Failed to fetch statistics',
      saveFailed: 'Save failed: {message}',
      confirmBan: 'Confirm ban for user {email}?',
      confirmUnban: 'Confirm unban for user {email}?',
      fetchTunnelListFailed: 'Failed to fetch tunnel list',
      fetchSpeedLimitFailed: 'Failed to fetch speed limit rules',
      fetchTunnelGrantFailed: 'Failed to fetch tunnel grants',
      selectTunnelFirst: 'Please select a tunnel',
      tunnelAlreadyAssigned: 'This tunnel is already assigned to current user',
      grantUpdateFailed: 'Failed to update grant',
      grantCreateFailed: 'Failed to create grant',
      grantUpdated: 'Grant updated successfully',
      grantCreated: 'Grant created successfully',
      grantActionFailed: 'Grant action failed',
      confirmDeleteGrant: 'Confirm deleting tunnel grant #{id}?',
      grantDeleteFailed: 'Failed to delete grant',
      resetFailed: 'Reset failed',
      userFlowReset: 'User traffic reset successfully',
      tunnelFlowReset: 'Tunnel traffic reset successfully'
    }
  },
  legacy
}

