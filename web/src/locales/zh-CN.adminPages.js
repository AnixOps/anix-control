// Messages for the admin pages other than the dashboard (src/views/admin
// and the forwarding area). src/i18n.js loads them before the first such
// page opens (router/index.js), so an admin's first visit, the
// dashboard, does not download them. The shell's messages are in
// src/locales/zh-CN.admin.js; the two groups share no message key.
import networkPages from './modules/zh-CN/networkPages'
import miscPages from './modules/zh-CN/miscPages'
import adminSupportPages from './modules/zh-CN/adminSupportPages'
import adminNodes from './modules/zh-CN/adminNodes'
import adminSubscriptionGroups from './modules/zh-CN/adminSubscriptionGroups'
import forwardV4 from './modules/zh-CN/forwardV4'
import forwardDns from './modules/zh-CN/forwardDns'

export default {
  ...adminSubscriptionGroups,
  ...forwardV4,
  ...forwardDns,
  ...networkPages,
  ...miscPages,
  ...adminSupportPages,
  runtime: {
    shared: {
      reachability: '\u53ef\u8fde\u901a\u6027',
      warnings: '\u8b66\u544a',
      powerShell: 'PowerShell',
      bash: 'Bash',
      bootstrapVerify: '\u5f15\u5bfc / \u6821\u9a8c',
      references: '\u53c2\u8003',
      doctorOutput: 'Doctor \u8f93\u51fa',
      doctorNotExecuted: '\u5c1a\u672a\u6267\u884c Doctor\u3002',
      refreshStatus: '\u5237\u65b0\u72b6\u6001',
      runningDoctor: '\u6267\u884c\u4e2d...',
      loading: '\u52a0\u8f7d\u4e2d...',
      yes: '\u662f',
      no: '\u5426',
      present: '\u5b58\u5728',
      missing: '\u7f3a\u5931',
      notReady: '\u672a\u5c31\u7eea',
      ready: '\u5c31\u7eea',
      unavailable: '\u4e0d\u53ef\u7528',
      enabled: '\u5df2\u542f\u7528',
      disabled: '\u5df2\u7981\u7528'
    },
    localRuntime: {
      fields: {
        inventory: 'Inventory',
        command: '\u547d\u4ee4',
        workingDir: '\u5de5\u4f5c\u76ee\u5f55'
      },
      cards: {
        commandFound: '\u627e\u5230\u547d\u4ee4'
      },
      backends: {
        nftables: {
          label: 'nftables / Ansible'
        }
      }
    },
    nodeX: {
      saveLoading: '\u4fdd\u5b58\u4e2d...',
      save: '\u4fdd\u5b58 NodeX \u914d\u7f6e',
      cards: {
        baseUrl: 'Base URL',
        tokenConfigured: 'Token \u5df2\u914d\u7f6e',
        health: '\u5065\u5eb7\u68c0\u67e5'
      },
      backends: {
        gost: 'gost / NodeX'
      }
    },
    ansibleMachines: {
      modal: {
        saveLoading: '\u4fdd\u5b58\u4e2d...'
      }
    },
    workbench: {
      actions: {
        runDoctorActiveRuntime: '\u5bf9\u5f53\u524d\u8fd0\u884c\u65f6\u8fd0\u884c Doctor'
      },
      localCard: {
        title: '\u672c\u5730 Ansible \u6267\u884c\u5668',
        description: '\u9762\u677f\u4e3b\u673a\u65e0\u72b6\u6001\u6267\u884c\u3002Inventory\u3001playbook \u548c SSH \u8bbf\u95ee\u72ec\u7acb\u4e8e NodeX \u7ba1\u7406\u3002',
        currentState: '\u5f53\u524d\u72b6\u6001'
      },
      nodeXCard: {
        title: '\u6709\u72b6\u6001 gost \u63a7\u5236\u9762',
        description: '\u9762\u677f\u4f1a\u76f4\u63a5\u8fde\u63a5\u5185\u90e8 NodeX \u63a7\u5236\u9762\u3002\u53ea\u6709 gost runtime \u4efb\u52a1\u6210\u529f\u540e\uff0crelay \u624d\u7b97\u771f\u6b63\u6302\u8f7d\u3002'
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
      doctor: {
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
        doctorFailed: 'Forward runtime Doctor \u6267\u884c\u5931\u8d25'
      }
    },
    nodeXTopology: {
      meta: {
        traffic: '\u4e0a\u884c / \u4e0b\u884c'
      },
      nodeModal: {
        saveLoading: '\u4fdd\u5b58\u4e2d...'
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
        execute: '\u8fd0\u884c',
        send: '\u53d1\u9001',
        cancel: '\u53d6\u6d88',
        monitor: '\u67e5\u770b\u76d1\u63a7',
        openTerminal: '\u5728\u7ec8\u7aef\u4e2d\u6253\u5f00'
      },
      table: {
        nodeId: '\u8282\u70b9',
        version: '\u7248\u672c',
        system: '\u7cfb\u7edf',
        lastSeen: '\u6700\u540e\u5728\u7ebf',
        status: '\u72b6\u6001',
        capabilities: '\u80fd\u529b',
        taskId: '\u4efb\u52a1 ID',
        node: '\u8282\u70b9',
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
        agents: '\u6ca1\u6709\u5728\u7ebf\u7684 Agent',
        agentsDescription: 'NodeX \u8282\u70b9\u8fde\u4e0a\u63a7\u5236\u9762\u540e\uff0c\u5b83\u7684 Agent \u4f1a\u51fa\u73b0\u5728\u8fd9\u91cc\u3002',
        tasks: '\u8fd8\u6ca1\u6709\u4efb\u52a1\u8bb0\u5f55',
        tasksDescription: '\u5728\u7ec8\u7aef\u91cc\u8fd0\u884c\u7684\u547d\u4ee4\u548c\u4e0b\u53d1\u7684\u4efb\u52a1\u4f1a\u5217\u5728\u8fd9\u91cc\u3002'
      },
      terminal: {
        chooseNode: '\u9009\u62e9\u8282\u70b9',
        nodeLabel: '\u8282\u70b9 #{id}',
        chooseAction: '\u9009\u62e9\u52a8\u4f5c',
        output: '\u7ec8\u7aef\u8f93\u51fa',
        hint: '\u9009\u62e9\u4e00\u4e2a\u5728\u7ebf\u8282\u70b9\u548c\u52a8\u4f5c\u540e\u8fd0\u884c\u3002\u53ea\u80fd\u8fd0\u884c\u63a7\u5236\u9762\u5141\u8bb8\u7684\u8bca\u65ad\u52a8\u4f5c\u3002'
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
        action: '\u52a8\u4f5c',
        timeoutSeconds: '\u8d85\u65f6\uff08\u79d2\uff09'
      },
      hints: {
        monitor: '\u67e5\u770b\u8282\u70b9 #{id} \u7684\u76d1\u63a7\u6570\u636e'
      },
      messages: {
        fetchFailed: 'Agent \u5217\u8868\u6ca1\u6709\u52a0\u8f7d\u51fa\u6765',
        tasksFetchFailed: '\u4efb\u52a1\u5386\u53f2\u6ca1\u6709\u52a0\u8f7d\u51fa\u6765',
        taskIncomplete: '\u8bf7\u586b\u5199\u5b8c\u6574\u4fe1\u606f',
        taskSent: '\u4efb\u52a1\u5df2\u53d1\u9001',
        taskSendFailed: '\u53d1\u9001\u5931\u8d25\uff1a{message}',
        commandError: '\u9519\u8bef\uff1a{message}',
        selectActionFirst: '\u8bf7\u5148\u9009\u62e9\u4e00\u4e2a\u52a8\u4f5c'
      }
    }
  },
  admin: {
    nodes: adminNodes
  },
  adminPayment: {
    title: '支付',
    subtitle: '用户付款用的网关、每一笔支付和收入统计。',
    tabs: {
      label: '支付分区',
      gateways: '支付网关',
      records: '支付记录',
      stats: '统计数据'
    },
    actions: {
      createGateway: '新建网关',
      enable: '启用',
      disable: '禁用',
      edit: '编辑',
      details: '详情'
    },
    gateways: {
      label: '支付网关',
      table: {
        id: 'ID',
        name: '名称',
        type: '类型',
        feeRate: '手续费率',
        minAmount: '最小金额',
        maxAmount: '最大金额',
        status: '状态'
      },
      empty: '还没有支付网关',
      emptyDescription: '添加一个网关，用户才能付款购买套餐。'
    },
    records: {
      label: '支付记录',
      filters: {
        status: '按状态筛选',
        type: '按网关筛选'
      },
      table: {
        id: 'ID',
        tradeNo: '交易号',
        userId: '用户 ID',
        gateway: '网关',
        amount: '金额',
        status: '状态',
        createdAt: '创建时间'
      },
      detail: {
        title: '支付详情',
        tradeNo: '交易号',
        amount: '金额',
        status: '状态'
      },
      empty: '还没有支付记录',
      emptyDescription: '用户为订单付款后，记录会出现在这里。'
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
      createTitle: '新建网关',
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
        configJson: "{'{'}\"app_id\": \"\", \"private_key\": \"\"{'}'}"
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
      enabled: '已启用',
      disabled: '已停用',
      pending: '待支付',
      paid: '已支付',
      failed: '失败',
      refunded: '已退款'
    },
    confirm: {
      deleteTitle: '删除支付网关 {name}？',
      deleteMessage: '用户将无法再通过这个网关付款。此操作无法撤销。',
      deleteAction: '删除网关'
    },
    messages: {
      fetchGatewaysFailed: '支付网关没有加载出来',
      fetchRecordsFailed: '支付记录没有加载出来',
      fetchStatsFailed: '支付统计没有加载出来',
      invalidConfigJson: '配置 JSON 格式错误',
      gatewaySaveSuccess: '网关保存成功',
      gatewaySaveFailed: '网关保存失败: {message}',
      gatewaySaveFailedShort: '保存失败',
      toggleFailed: '切换网关状态失败: {message}',
      toggleFailedShort: '操作失败',
      gatewayEnabled: '已启用 {name}',
      gatewayDisabled: '已禁用 {name}',
      gatewayDeleted: '已删除网关 {name}',
      deleteFailedShort: '删除失败'
    }
  },
  adminMfa: {
    config: {
      enabled: '\u542f\u7528\u591a\u56e0\u7d20\u8ba4\u8bc1',
      enabledHelp: '\u542f\u7528\u540e\uff0c\u7528\u6237\u53ef\u9009\u62e9\u5f00\u542f MFA \u4fdd\u62a4\u8d26\u6237\u5b89\u5168',
      required: '\u5f3a\u5236\u542f\u7528 MFA',
      requiredHelp: '\u5f3a\u5236\u6240\u6709\u7528\u6237\u542f\u7528 MFA\uff0c\u5426\u5219\u65e0\u6cd5\u4f7f\u7528\u670d\u52a1',
      methods: '\u652f\u6301\u7684\u8ba4\u8bc1\u65b9\u5f0f',
      backupCodesCount: '\u6062\u590d\u7801\u6570\u91cf',
      backupCodesHelp: '\u7528\u6237\u542f\u7528 MFA \u65f6\u751f\u6210\u7684\u6062\u590d\u7801\u6570\u91cf',
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
      totpBody: '\u57fa\u4e8e\u65f6\u95f4\u7684\u4e00\u6b21\u6027\u5bc6\u7801\uff0c\u7528\u6237\u53ef\u4f7f\u7528 Google Authenticator\u3001Authy \u7b49\u5e94\u7528\u626b\u63cf\u4e8c\u7ef4\u7801\u7ed1\u5b9a\u3002',
      backupBody: '\u5f53\u7528\u6237\u65e0\u6cd5\u4f7f\u7528\u8ba4\u8bc1\u5668\u65f6\uff0c\u53ef\u4f7f\u7528\u6062\u590d\u7801\u767b\u5f55\u3002\u6bcf\u4e2a\u6062\u590d\u7801\u53ea\u80fd\u4f7f\u7528\u4e00\u6b21\u3002',
      lockoutBody: '\u8fde\u7eed\u591a\u6b21 MFA \u9a8c\u8bc1\u5931\u8d25\u5c06\u89e6\u53d1\u8d26\u6237\u9501\u5b9a\uff0c\u9632\u6b62\u66b4\u529b\u7834\u89e3\u3002',
      userOpsBody: '用户在自己的「账户」页开启、关闭两步验证，并重新生成恢复码。'
    },
    messages: {
      fetchFailed: '\u83b7\u53d6 MFA \u914d\u7f6e\u5931\u8d25',
      saveSuccess: '\u4fdd\u5b58\u6210\u529f',
      saveFailed: '\u4fdd\u5b58\u5931\u8d25: {message}',
      saveFailedShort: '\u4fdd\u5b58\u5931\u8d25'
    }
  },
  adminTemplates: {
    title: '订阅模板',
    subtitle: '免费的订阅模板：流量额度、速率和设备限制，以及授予的订阅分组。分配给用户即可生效。',
    filters: {
      search: '搜索模板'
    },
    table: {
      label: '订阅模板列表'
    },
    actions: {
      create: '新建模板'
    },
    empty: {
      title: '还没有订阅模板',
      description: '创建模板后分配给用户，用户就能获得流量和订阅分组。'
    },
    detail: {
      description: '模板 ID {id}'
    },
    labels: {
      noGroupsHint: '这个模板还没有授予订阅分组。'
    },
    planModal: {
      createTitle: '新建订阅模板',
      editTitle: '编辑订阅模板',
      fields: {
        name: '模板名称'
      },
      placeholders: {
        name: '请输入模板名称'
      }
    },
    assignModal: {
      title: '分配订阅模板'
    },
    groupModal: {
      title: '模板分组 - {name}',
      description: '选择该模板授予的订阅分组。'
    },
    confirm: {
      deleteTitle: '删除订阅模板 {name}？',
      deleteAction: '删除模板'
    },
    messages: {
      loadFailed: '加载订阅模板失败',
      nameRequired: '请输入模板名称',
      deleted: '已删除订阅模板 {name}',
      groupRemoved: '已从模板 {plan} 移除分组 {group}'
    }
  },
  adminPlans: {
    title: '套餐',
    subtitle: '用户可以购买的套餐：流量额度、速率和设备限制、价格，以及授予的订阅分组。',
    filters: {
      search: '搜索套餐'
    },
    table: {
      label: '套餐列表',
      name: '名称',
      transfer: '流量',
      limits: '限制',
      monthPrice: '月付价格',
      subscriptionGroups: '订阅分组'
    },
    actions: {
      create: '新建套餐',
      edit: '编辑',
      delete: '删除',
      assign: '分配',
      manageGroups: '管理分组',
      removeGroupNamed: '移除分组 {name}'
    },
    empty: {
      title: '还没有套餐',
      description: '创建套餐后，用户就能购买流量并获得订阅分组。'
    },
    detail: {
      description: '套餐 ID {id}',
      limits: '额度和限制',
      actions: '操作'
    },
    planModal: {
      createTitle: '新建套餐',
      editTitle: '编辑套餐',
      fields: {
        name: '套餐名称',
        transfer: '流量额度',
        speedLimit: '速率限制',
        deviceLimit: '设备限制',
        monthPrice: '月付价格（分）'
      },
      placeholders: {
        name: '请输入套餐名称'
      },
      help: {
        zeroUnlimited: '0 表示不限制。'
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
      },
      help: {
        expireAt: '可选，Unix 时间（秒）。'
      }
    },
    groupModal: {
      title: '套餐分组 - {name}',
      description: '选择该套餐可访问的订阅分组。',
      empty: '暂无订阅分组',
      noDescription: '无描述'
    },
    labels: {
      noSpeedLimit: '不限速',
      noDeviceLimit: '不限设备',
      speedLimitMbps: '{value} Mbps',
      deviceLimitCount: '{value} 台',
      noGroups: '无',
      noGroupsHint: '这个套餐还没有授予订阅分组。'
    },
    confirm: {
      deleteTitle: '删除套餐 {name}？',
      deleteMessage: '此操作无法撤销。',
      deleteAction: '删除套餐'
    },
    messages: {
      loadFailed: '加载套餐失败',
      loadGroupsFailed: '加载订阅分组失败',
      deleted: '已删除套餐 {name}',
      deleteFailedShort: '删除失败',
      nameRequired: '请填写套餐名称',
      saveFailed: '保存失败：{message}',
      saveFailedShort: '保存失败',
      saved: '已保存 {name}',
      userIdRequired: '请填写用户 ID',
      assignSuccess: '分配成功',
      assignFailed: '分配失败：{message}',
      assignFailedShort: '分配失败',
      toggleGroupFailed: '切换分组失败：{message}',
      toggleGroupFailedShort: '切换分组失败',
      groupRemoved: '已从套餐 {plan} 移除分组 {group}',
      removeGroupFailed: '移除分组失败：{message}',
      removeGroupFailedShort: '移除失败'
    }
  },
  adminUsers: {
    title: '用户',
    subtitle: '账户、订阅和流量。选中一个用户查看详情。',
    filters: {
      searchEmail: '搜索邮箱',
      label: '按状态筛选',
      exhaustedHint: '「流量用尽」只筛选本页的用户，服务器没有这个筛选条件。'
    },
    table: {
      label: '用户列表',
      id: 'ID',
      email: '邮箱',
      plan: '套餐',
      subscriptionTemplate: '订阅模板',
      traffic: '已用 / 总流量',
      limits: '限制',
      expireAt: '到期时间',
      status: '状态',
      lastOnline: '最近在线',
      createdAt: '注册时间'
    },
    lastOnline: {
      never: '从未在线',
      neverHint: '还没有任何节点上报过该用户。',
      loading: '加载中',
      unavailable: '暂不可用'
    },
    status: {
      active: '正常',
      expired: '已到期',
      banned: '已封禁',
      exhausted: '流量用尽'
    },
    actions: {
      addUser: '新建用户',
      editUser: '编辑用户',
      ban: '封禁',
      unban: '解封',
      resetTraffic: '重置流量',
      copySubscribe: '复制订阅链接',
      resetSubscribe: '重置订阅链接',
      viewTraffic: '最近 30 天流量'
    },
    empty: {
      title: '还没有用户',
      description: '添加第一个用户后，就能给他分配订阅。'
    },
    detail: {
      description: 'ID {id} · 注册于 {date}',
      subscription: '订阅',
      flowReset: '流量重置',
      activity: '活动',
      actions: '操作',
      danger: '危险操作',
      dangerFooter: '重置订阅链接后，所有客户端都要重新导入；重置流量无法撤销。'
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
      title: '新建用户',
      submit: '创建用户',
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
      userTypes: {
        normal: '普通用户',
        admin: '管理员'
      }
    },
    confirm: {
      resetSubscribeTitle: '重置 {email} 的订阅链接？',
      resetSubscribeMessage: '旧链接会立即失效，用户需要重新导入订阅。此操作无法撤销。',
      resetSubscribeAction: '重置订阅链接'
    },
    copyDialog: {
      title: '复制订阅链接',
      description: '无法自动复制，请手动复制下面的链接。',
      label: '订阅链接'
    },
    resetFlow: {
      userTitle: '重置 {email} 的流量？',
      userMessage: '已用流量将清零。此操作无法撤销。',
      usedFlow: '当前已用',
      quota: '当前配额',
      confirmAction: '重置流量'
    },
    trafficModal: {
      title: '流量详情 - {email}',
      subtitle: '最近 30 天每日汇总和每小时明细。',
      refresh: '刷新',
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
      noSpeedLimit: '不限速',
      noDeviceLimit: '不限设备',
      speedLimitMbps: '{value} Mbps',
      deviceLimitCount: '{value} 台',
      noReset: '不重置',
      monthlyDay: '每月第 {day} 天',
      permanent: '永久',
      trafficUnlimited: '已用 {used} · 不限'
    },
    bulk: {
      requestFailed: '批量操作失败：{message}',
      banPartial: '{total} 个用户中 {done} 个已封禁。',
      banNone: '没有用户被封禁。',
      unbanPartial: '{total} 个用户中 {done} 个已解封。',
      unbanNone: '没有用户被解封。',
      undoPartial: '{count} 个用户未能恢复。',
      resetTitle: '重置 {count} 个用户的流量？',
      resetMessage: '所选用户的已用流量都会归零，此操作无法撤销。',
      resetDone: '已重置 {count} 个用户的流量',
      resetPartial: '{total} 个用户中 {done} 个已重置流量。',
      resetNone: '没有用户的流量被重置。'
    },
    messages: {
      actionFailed: '操作失败',
      fillEmailPassword: '请填写邮箱和密码',
      passwordTooShort: '密码长度至少 6 位',
      userCreated: '用户创建成功',
      createFailed: '创建失败',
      fetchUsersFailed: '用户列表没有加载出来',
      bulkBanned: '已封禁 {count} 个用户',
      bulkUnbanned: '已解封 {count} 个用户',
      fetchStatsFailed: '获取统计失败',
      saveFailed: '保存失败：{message}',
      userSaved: '已保存 {email}',
      userBanned: '已封禁 {email}',
      userUnbanned: '已解封 {email}',
      resetFailed: '重置失败',
      userFlowReset: '用户流量已重置',
      fetchUserFailed: '获取用户详情失败',
      noToken: '该用户没有订阅 token',
      subscribeCopied: '订阅链接已复制到剪贴板',
      resetSubscribeSuccess: '订阅链接已重置',
      resetSubscribeFailed: '重置订阅失败'
    }
  },
  control: {
    subtitle: '管理官方签名软件包、Control WebUI 扩展与生命周期操作。',
    actions: {
      refresh: '刷新', refreshing: '刷新中...', importRelease: '导入发行版', install: '安装',
      configure: '配置', enable: '启用', disable: '禁用', upgrade: '升级', update: '升级', rollback: '回滚', cancel: '取消操作', installOfficialOnly: '需要官方发行版',
      saving: '保存中...', newAssignment: '新建角色'
    },
    pluginCenter: {
      filters: { search: '搜索插件', health: '按健康状态筛选', target: '按运行目标筛选' },
      states: { healthy: '健康', attention: '需要关注' },
      listLabel: '插件',
      official: '官方签名软件包',
      loadFailed: '插件目录没有加载出来',
      empty: '没有符合当前筛选条件的插件',
      emptyCatalog: { title: '还没有插件', description: '导入一个官方签名发行版，它的插件就会出现在这里。' },
      detail: { versions: '版本与状态' },
      operations: { title: '最近插件操作', empty: '暂无最近插件操作' }
    },
    tabs: { assignments: '节点角色', topologies: '拓扑' },
    table: {
      plugin: '插件', release: '发行版', installation: '安装目标', version: '版本', state: '状态', actions: '操作',
      scope: '作用域', description: '说明', topology: '拓扑', activeRevision: '激活 revision', deployment: '部署',
      operation: '操作', chain: '链', revision: 'revision', deadline: '截止时间', target: '目标', version: '版本', role: '服务角色', configRevision: '配置 revision', rolloutGroup: '灰度组'
    },
    labels: { releases: '{count} 个发行版', desired: '期望', observed: '实际' },
    states: { catalogued: '已登记', enabled: '已启用', disabled: '已禁用', loading: '正在加载控制状态...' },
    empty: { assignments: '该节点暂无服务角色', topologies: '暂无拓扑', operations: '暂无操作' },
    activity: { scoped: '当前范围活动', all: '全部活动', showAll: '显示全部活动', showScoped: '显示当前范围活动', empty: '当前范围暂无活动' },
    topology: {
      select: '拓扑', new: '新建拓扑', newTitle: '新建拓扑', create: '创建拓扑', name: '名称', edit: '编辑修订', status: '查看状态', noDeployment: '暂无部署', editorTitle: '拓扑修订编辑器', revision: '修订',
      noRevisions: '暂无修订', failurePolicy: '失败策略', stopAndRollback: '停止并回滚', message: '修订说明', graphJSON: '拓扑图 JSON',
      graphHelp: '使用 vertices 和 edges 描述拓扑。秘密必须通过 secret_id 引用，禁止直接写入秘密值。', diagnose: '校验 / 诊断', validating: '校验中...',
      unsavedChanges: '当前修订存在未保存修改。请先保存新的不可变修订，再进行预览或规划。',
      valid: '拓扑校验通过', invalid: '拓扑存在校验问题', invalidJSON: '拓扑 JSON 无效', saveRevision: '保存修订', plan: '规划部署',
      apply: '应用部署', rollback: '回滚部署', deployment: '部署', applyConfirm: '确认应用 {topology}（部署 #{deployment}）吗？', rollbackConfirm: '确认请求回滚 {topology}（部署 #{deployment}）吗？',
      preview: '只读部署预览', previewAction: '预览', previewSteps: '{count} 个计划步骤',
      emptyHint: '新建一个拓扑，写入顶点与连接，保存修订后即可规划部署。',
      viewLabel: '编辑方式', viewJSON: 'JSON', viewGraph: '图示', graphLabel: '拓扑图示', graphSummary: '{nodes} 个顶点，{edges} 条连接',
      graphEmpty: '还没有顶点', graphEmptyHint: '在 JSON 的 vertices 中添加顶点后，这里会画出拓扑。', graphInvalidHint: '修正 JSON 后这里会画出拓扑。', graphFailed: '拓扑图示加载失败'
    },
    extensions: { errorsTitle: 'WebUI 扩展加载失败' },
    assignments: {
      node: '节点', noNodes: '暂无可用节点', agentPlugin: 'Agent 插件', enabled: '启用该节点角色',
      createTitle: '新建节点角色', editTitle: '编辑节点角色', deleteTitle: '从该节点删除 {plugin} / {role}？', deleteMessage: '该节点会停止运行这个插件角色。此操作无法撤销。', deleteAction: '删除节点角色',
      emptyHint: '为该节点新建一个角色，指定插件、作用域与版本。', roleHelp: '常用角色：{roles}'
    },
    install: { title: '安装官方插件', target: '运行目标', version: '发行版本', enableAfterInstall: '安装后立即启用' },
    update: { title: '升级官方插件' },
    config: {
      title: '插件配置', loading: '正在加载配置...', revision: '配置 revision：{revision}', mode: '配置编辑模式', formMode: '表单', jsonMode: 'JSON',
      selectValue: '请选择', addItem: '添加项目', removeItem: '移除项目', emptyArray: '暂无项目', item: '项目', invalidJSON: 'JSON 无效',
      schemaError: '{path} 的值不符合 Schema', requiredError: '{path} 为必填项'
    },
    releaseImport: {
      title: '导入官方签名发行版', manifest: 'Manifest JSON', signature: '签名', artifact: '软件包制品',
      artifactOptional: '可选；未上传制品的发行版不能安装。'
    },
    messages: {
      actionQueued: '{plugin} 的“{action}”操作已提交', operationStatus: '操作 {id} 当前为 {state}，链：{chain}。', installed: '{plugin} 安装意图已保存', configSaved: '{plugin} 配置已保存',
      releaseImported: '{plugin} {version} 已导入', cancelRequested: '已请求取消操作',
      assignmentSaved: '{plugin} 节点角色已保存', assignmentStateSaved: '{plugin} 节点角色状态已保存', assignmentDeleted: '{plugin} 节点角色已删除',
      topologyRevisionSaved: '拓扑修订 {revision} 已保存', topologyPlanned: '部署 #{id} 已规划',
      topologyApplyRequested: '已请求应用拓扑部署', topologyRollbackRequested: '已请求回滚拓扑部署', topologyCreated: '拓扑 {name} 已创建'
    },
    errors: {
      load: '无法加载控制状态', action: '插件操作失败', install: '插件安装失败', configLoad: '无法加载插件配置',
      configSave: '无法保存插件配置', releaseImport: '发行版导入失败', cancel: '无法取消操作', poll: '无法刷新操作状态',
      nodesLoad: '无法加载节点', assignmentsLoad: '无法加载节点角色', assignmentSave: '无法保存节点角色', assignmentDelete: '无法删除节点角色',
      topologyLoad: '无法加载拓扑修订', topologyValidate: '无法校验拓扑', topologySave: '无法保存拓扑修订', topologyPlan: '无法规划拓扑部署',
      topologyPreview: '无法预览拓扑部署', topologyStatus: '无法加载部署状态', topologyApply: '无法应用拓扑部署', topologyRollback: '无法回滚拓扑部署', topologyCreate: '无法创建拓扑'
    }
  },
  routeModes: {
    open: '路由模式',
    subtitle: '按软件包切换 v2 路由的旧版、影子和原生模式，并可回滚到旧版。',
    back: '插件中心',
    package: '软件包',
    packageMeta: '版本 {version} · 配置修订 {revision}',
    packageDisabled: '已停用',
    superAdminOnly: '仅超级管理员可以切换路由模式。',
    loadFailed: '路由模式加载失败',
    empty: { title: '没有声明 v2 路由的软件包', description: '安装并启用声明了 v2 路由的软件包后，即可在这里管理其路由模式。' },
    modes: { legacy: '旧版', shadow: '影子', native: '原生' },
    modeDefault: '{mode}（默认）',
    sources: {
      stored: '已明确设置',
      default: '演练通过的默认值：未保存模式',
      'kill-switch': '默认值已关闭：package_routes.default_mode 为 legacy',
      'package-too-old': '默认值未生效：软件包早于演练版本',
      'identity-authority': '跟随身份权威状态',
      unset: '未保存模式'
    },
    defaults: {
      'kill-switch': '默认原生已关闭（package_routes.default_mode: legacy），未保存模式的路由都走旧版。',
      'package-too-old': '版本 {version} 早于演练版本 {min}，其路由默认走旧版。'
    },
    catalog: { 'native-flagged': '可切原生', bridged: '桥接', 'kernel-owned': '内核接管', native: '原生', none: '未声明' },
    locked: { kernel_owned: '内核接管', identity_group_a: '身份切换', websocket: 'WebSocket', not_declared: '未声明' },
    columns: { route: '路由', endpoint: '方法 / 路径', catalog: '资格', configured: '配置模式', effective: '生效模式', shadow: '影子（总数 / 不一致 / 错误）', mismatch: '不一致率', mode: '切换为' },
    routesLabel: '{package} 的路由',
    routeMode: '{route} 的模式',
    effectiveDiffers: '与配置模式不同',
    hostEffective: '宿主：{mode}',
    packageMode: '整个软件包的模式',
    setPackage: '设置整个软件包',
    rollback: '回滚到旧版',
    wholePackage: '整个软件包',
    wholePackageTarget: '{package}（整个软件包）',
    dialog: {
      setTitle: '将 {target} 切换为{mode}模式？',
      setMessage: '不支持该模式的路由会被跳过。此次变更会记入修订历史。',
      nativeMessage: '原生模式下，请求由软件包自身的实现处理，不再经过内核的旧版处理器。请确认并填写原因，以便审计。',
      rollbackTitle: '将 {package} 回滚到旧版？',
      rollbackMessage: '该软件包的所有路由都会恢复为旧版模式，包括默认走原生的路由。',
      reason: '原因',
      reasonHelp: '选填，会随修订一并保存。',
      reasonRequired: '切换到原生模式时必须填写。',
      confirmSet: '切换模式',
      confirmNative: '切换到原生',
      confirmRollback: '回滚'
    },
    result: '已变更 {changed} 条路由，跳过 {skipped} 条（配置修订 {revision}）',
    errors: { switch: '路由模式切换失败', rollback: '回滚失败', revisions: '修订历史加载失败' },
    revisions: {
      title: '修订历史',
      label: '路由模式修订',
      empty: '暂无路由模式变更',
      time: '时间', route: '路由', change: '变更', actor: '操作人', reason: '原因', revision: '修订',
      actions: { set: '设置', rollback: '回滚' }
    },
    mismatches: {
      title: '影子不一致样本',
      label: '不一致样本',
      open: '样本（{count}）',
      lastSeen: '最近 {time}',
      privacy: '样本在保存前已脱敏：令牌、密码、密钥、UUID 和订阅链接全部隐藏，邮箱只保留首字母和域名，IP 只保留前两段。每条路由保留最近 {max} 条，保存 {days} 天。',
      loadFailed: '不一致样本加载失败',
      empty: '没有保存的样本',
      emptyDescription: '影子运行的结果与旧版不同时，样本会出现在这里。',
      observed: '发生时间',
      status: '状态码（旧版 → 原生）',
      diffColumn: '差异',
      fields: '{count} 个字段',
      detail: '样本详情',
      request: '请求',
      requestID: '请求 ID',
      version: '包版本',
      truncated: '差异已截断',
      diff: '差异（已脱敏）',
      copyDiff: '复制差异'
    }
  },
  accessGroups: {
    subtitle: '管理独立服务作用域的成员关系、资源授权和插件私有配额策略。',
    actions: { refresh: '刷新', newGroup: '新建访问组', open: '打开', editGroup: '编辑名称和说明', enable: '启用', disable: '禁用', add: '添加', removeNamed: '移除 {name}', addGrant: '添加授权', saveQuota: '保存配额', resolve: '解析有效授权' },
    filters: { label: '按服务作用域筛选', allScopes: '全部服务作用域' },
    table: { group: '访问组', scope: '作用域', state: '状态' },
    states: { enabled: '已启用', disabled: '已禁用' },
    groups: { title: '访问组', empty: '这个作用域还没有访问组', emptyAll: '还没有访问组', emptyDescription: '访问组在一个服务作用域内，为其中的用户和套餐授予资源权限和配额策略。', noDescription: '暂无说明', directUnion: '直接成员和套餐成员按允许并集计算。' },
    detail: { title: '访问组', loading: '正在加载访问组详情…', group: '访问组', danger: '危险操作' },
    members: { title: '用户成员', userID: '用户 ID', empty: '暂无直接用户成员' },
    plans: { title: '套餐成员', planID: '套餐 ID', empty: '暂无套餐成员' },
    grants: { title: '资源授权', resourceType: '资源类型', resourceID: '资源 ID', permissions: '权限 JSON', empty: '暂无资源授权' },
    quotas: { title: '配额策略', key: '策略键', policy: '策略 JSON', empty: '暂无配额策略' },
    resolver: { title: '有效授权预览', description: '预览一个用户、可选套餐和服务作用域在服务端的允许并集结果。', userID: '用户 ID', planID: '套餐 ID（可选）', scope: '服务作用域', result: '命中 {count} 个已启用访问组', none: '没有命中已启用访问组', policySummary: '当前生效 {grants} 条授权和 {quotas} 条配额策略。' },
    editor: { createTitle: '新建访问组', editTitle: '编辑访问组', name: '组名称', description: '说明', enabled: '启用该访问组', scopeFixed: '已有访问组的作用域不能修改。' },
    messages: { groupCreated: '已创建访问组 {name}', groupSaved: '已保存访问组 {name}', groupEnabled: '已启用访问组 {name}', groupDisabled: '已禁用访问组 {name}', groupDeleted: '已删除访问组 {name}', memberAdded: '已添加用户 #{id}', memberRemoved: '已移除用户 #{id}', planAdded: '已添加套餐 #{id}', planRemoved: '已移除套餐 #{id}', grantAdded: '已添加资源授权', grantRemoved: '已移除资源授权', quotaSaved: '已保存配额策略', quotaRemoved: '已移除配额策略' },
    confirm: { deleteGroupTitle: '删除访问组 {name}？', deleteGroup: '组内的成员关系、资源授权和配额策略会一并删除。此操作无法撤销。', deleteGroupAction: '删除访问组', removeGrantTitle: '移除资源授权 #{id}？', removeGrant: '该组成员将失去对 {resource} 的访问权限。此操作无法撤销。', removeGrantAction: '移除授权', removeQuotaTitle: '移除配额策略 {key}？', removeQuota: '该组将不再受这条配额限制。此操作无法撤销。', removeQuotaAction: '移除策略' },
    errors: { load: '无法加载访问控制数据', loadGroups: '无法加载访问组', loadDetail: '无法加载访问组详情', groupRequired: '必须填写服务作用域和组名称', saveGroup: '无法保存访问组', deleteGroup: '无法删除访问组', member: '无法更新用户成员关系', plan: '无法更新套餐成员关系', grant: '无法更新资源授权', quota: '无法更新配额策略', resolve: '无法解析有效授权', invalidID: '{label} 必须为正整数', invalidJSON: '{label} 必须为有效 JSON', scopeRequired: '必须选择服务作用域' }
  },
  agentUpgrades: {
    title: 'Agent 升级',
    description: 'Control 按灰度批次推送 Agent 版本（5%、25%、100%，每批至少 30 分钟）；某一批失败超过 5% 时自动回滚该批。',
    empty: '尚无 Agent 升级。超级管理员可执行：',
    target: 'Agent {version}',
    batchOf: '第 {batch} / {total} 批',
    batchEnds: '最早 {time} 进入下一批',
    finished: '结束于 {time}',
    batch: '第 {batch} 批 · {percent}%',
    progress: '已完成 {done} / {total}',
    statuses: { running: '进行中', paused: '已暂停', rolling_back: '回滚中', succeeded: '已完成', rolled_back: '已回滚', aborted: '已中止' },
    states: { pending: '待升级', offered: '已下发', upgrading: '升级中', succeeded: '已升级', failed: '失败', rolled_back: '已回滚', skipped: '已跳过' },
    actions: { pause: '暂停', resume: '继续', abort: '中止', rollback: '中止并回滚' },
    confirm: {
      abortTitle: '中止升级到 {version}？',
      abort: '不再向其他节点下发升级。已升级的节点保留新版本。',
      rollbackTitle: '回滚升级到 {version}？',
      rollback: '当前批次中已升级的节点将恢复到之前的版本，然后结束本次升级。'
    },
    loadFailed: '无法加载 Agent 升级'
  },
  agentTransports: {
    open: 'Agent 连接方式',
    subtitle: '查看每个节点的 Agent 如何连接 Control：mTLS 流，或 v4.2 将拒绝的旧版通道。',
    back: 'NodeX Agents',
    mode: 'agent_control.mtls：{mode}',
    modes: {
      off: '已关闭客户端证书；旧版 Agent 照常服务，不发出提示。',
      optional: '提供客户端证书时会校验；旧版 Agent 照常服务，不发出提示。',
      preferred: '旧版 Agent 仍可连接，但会收到弃用提示。',
      required: 'AnixOps Agent 通道只接受已注册（持有客户端证书）的 Agent。'
    },
    sunset: '旧版停用日期：{date}',
    notice: 'v4.2 起 agent_control.mtls 默认为 required。升级前，所有标记为“旧版”的节点都必须运行已完成注册的 Agent；使用 API 密钥的旧版 Agent 将被拒绝。UniProxy 和 v2board gRPC（第三方节点软件）不受影响。',
    guide: '升级指南',
    filterLabel: '按状态筛选',
    statuses: { all: '全部', mtls: 'mTLS', legacy: '旧版', 'third-party': '第三方', unseen: '未出现' },
    legacyHint: 'required 模式下会被拒绝',
    transports: {
      'mtls-stream': 'mTLS 流',
      'apikey-stream': 'API 密钥流',
      'http-legacy': '旧版 HTTP',
      websocket: 'WebSocket',
      'clean-agent': 'Clean Agent',
      uniproxy: 'UniProxy',
      'v2board-grpc': 'v2board gRPC'
    },
    columns: { node: '节点', status: '状态', transport: '最近通道', version: 'Agent 版本', certificate: '证书', lastSeen: '最近出现', seenOn: '出现过的通道' },
    certificateUntil: '有效至 {date}',
    noCertificate: '未注册',
    disabled: '已停用',
    tableLabel: '各节点的 Agent 连接方式',
    empty: { title: '暂无节点', description: '创建代理节点或转发节点后会显示在这里。' },
    loadFailed: 'Agent 连接方式加载失败'
  }
}
