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
import adminApiTokens from './modules/zh-CN/adminApiTokens'
import adminLicenses from './modules/zh-CN/adminLicenses'
import forwardV4 from './modules/zh-CN/forwardV4'
import forwardDns from './modules/zh-CN/forwardDns'
import egressRoutes from './modules/zh-CN/egressRoutes'
import standby from './modules/zh-CN/standby'
import topology from './modules/zh-CN/topology'
import lines from './modules/zh-CN/lines'
import mesh from './modules/zh-CN/mesh'
import reverseProxies from './modules/zh-CN/reverseProxies'
import nodeDiagnostics from './modules/zh-CN/nodeDiagnostics'
import realityTargets from './modules/zh-CN/realityTargets'
import nodeEntries from './modules/zh-CN/nodeEntries'
import nodeNetwork from './modules/zh-CN/nodeNetwork'
import nodeForwardPorts from './modules/zh-CN/nodeForwardPorts'
import nodeExitPolicy from './modules/zh-CN/nodeExitPolicy'
import nodeTemporaryProxies from './modules/zh-CN/nodeTemporaryProxies'
import nodeReach from './modules/zh-CN/nodeReach'
import agentUpgradeSheet from './modules/zh-CN/agentUpgradeSheet'

export default {
  ...adminSubscriptionGroups,
  ...adminApiTokens,
  ...adminLicenses,
  ...forwardV4,
  ...forwardDns,
  ...egressRoutes,
  ...topology,
  ...lines,
  ...mesh,
  ...standby,
  ...reverseProxies,
  ...nodeDiagnostics,
  ...realityTargets,
  ...nodeEntries,
  ...nodeNetwork,
  ...nodeForwardPorts,
  ...nodeExitPolicy,
  ...nodeTemporaryProxies,
  ...nodeReach,
  ...agentUpgradeSheet,
  ...networkPages,
  ...miscPages,
  ...adminSupportPages,
  admin: {
    nodes: adminNodes
  },
  adminPayment: {
    title: '支付',
    subtitle: '支付网关、支付记录与收入统计。',
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
      empty: '暂无支付网关',
      emptyDescription: '添加网关后，用户方可付款购买套餐。'
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
      empty: '暂无支付记录',
      emptyDescription: '用户付款后显示于此。'
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
      deleteMessage: '删除后用户将无法通过该网关付款。此操作无法撤销。',
      deleteAction: '删除网关'
    },
    messages: {
      fetchGatewaysFailed: '支付网关加载失败',
      fetchRecordsFailed: '支付记录加载失败',
      fetchStatsFailed: '支付统计加载失败',
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
      enabledHelp: '启用后用户可开启 MFA',
      required: '\u5f3a\u5236\u542f\u7528 MFA',
      requiredHelp: '所有用户须启用 MFA 方可使用服务',
      methods: '\u652f\u6301\u7684\u8ba4\u8bc1\u65b9\u5f0f',
      backupCodesCount: '\u6062\u590d\u7801\u6570\u91cf',
      backupCodesHelp: '启用 MFA 时生成的恢复码数量',
      maxAttempts: '\u767b\u5f55\u5c1d\u8bd5\u9650\u5236',
      maxAttemptsHelp: '超过限制将临时锁定账户',
      lockoutDuration: '\u9501\u5b9a\u65f6\u957f (\u5206\u949f)',
      lockoutDurationHelp: '登录失败超限后的锁定时长'
    },
    methods: {
      totp: 'TOTP (Google Authenticator / Authy)',
      sms: '\u77ed\u4fe1\u9a8c\u8bc1\u7801',
      email: '\u90ae\u7bb1\u9a8c\u8bc1\u7801'
    },
    info: {
      totpBody: '基于时间的一次性密码，支持 Google Authenticator、Authy 等应用扫码绑定。',
      backupBody: '认证器不可用时可使用恢复码登录，每个恢复码仅可使用一次。',
      lockoutBody: 'MFA 连续验证失败将锁定账户。',
      userOpsBody: '用户在“账户”页开启或关闭两步验证，并重新生成恢复码。'
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
    subtitle: '免费订阅模板：流量额度、速率与设备限制，以及授予的订阅分组。分配给用户后生效。',
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
      title: '暂无订阅模板',
      description: '模板分配给用户后，用户获得相应流量与订阅分组。'
    },
    detail: {
      description: '模板 ID {id}'
    },
    labels: {
      noGroupsHint: '该模板未授予订阅分组。'
    },
    planModal: {
      createTitle: '新建订阅模板',
      editTitle: '编辑订阅模板',
      fields: {
        name: '模板名称'
      },
      placeholders: {
        name: '请输入模板名称'
      },
      help: {
        show: '关闭后用户不可见该模板。'
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
    subtitle: '可购买套餐：流量额度、速率与设备限制、价格，以及授予的订阅分组。',
    filters: {
      search: '搜索套餐'
    },
    table: {
      label: '套餐列表',
      name: '名称',
      transfer: '流量',
      limits: '限制',
      monthPrice: '月付价格',
      subscriptionGroups: '订阅分组',
      status: '状态'
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
      title: '暂无套餐',
      description: '用户购买套餐后获得相应流量与订阅分组。'
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
        monthPrice: '月付价格（分）',
        show: '上架'
      },
      placeholders: {
        name: '请输入套餐名称'
      },
      help: {
        zeroUnlimited: '0 表示不限制。',
        show: '关闭后购买页不显示该套餐，已购用户不受影响。'
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
      noGroupsHint: '该套餐未授予订阅分组。',
      onSale: '在售',
      offSale: '已下架'
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
    subtitle: '账户、订阅与流量。选择用户查看详情。',
    filters: {
      searchEmail: '搜索邮箱',
      label: '按状态筛选',
      exhaustedHint: '“流量用尽”仅筛选本页用户。'
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
      neverHint: '暂无节点上报该用户。',
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
      title: '暂无用户',
      description: '添加用户后可分配订阅。'
    },
    detail: {
      description: 'ID {id} · 注册于 {date}',
      subscription: '订阅',
      flowReset: '流量重置',
      activity: '活动',
      actions: '操作',
      danger: '危险操作',
      dangerFooter: '重置订阅链接后所有客户端须重新导入；重置流量无法撤销。'
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
      resetSubscribeMessage: '原链接立即失效，用户须重新导入订阅。此操作无法撤销。',
      resetSubscribeAction: '重置订阅链接'
    },
    copyDialog: {
      title: '复制订阅链接',
      description: '自动复制失败，请手动复制以下链接。',
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
      banNone: '未封禁任何用户。',
      unbanPartial: '{total} 个用户中 {done} 个已解封。',
      unbanNone: '未解封任何用户。',
      undoPartial: '{count} 个用户未能恢复。',
      resetTitle: '重置 {count} 个用户的流量？',
      resetMessage: '所选用户的已用流量将清零。此操作无法撤销。',
      resetDone: '已重置 {count} 个用户的流量',
      resetPartial: '{total} 个用户中 {done} 个已重置流量。',
      resetNone: '未重置任何用户的流量。'
    },
    messages: {
      actionFailed: '操作失败',
      fillEmailPassword: '请填写邮箱和密码',
      passwordTooShort: '密码长度至少 6 位',
      userCreated: '用户创建成功',
      createFailed: '创建失败',
      fetchUsersFailed: '用户列表加载失败',
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
    config: {
      title: '插件配置', loading: '正在加载配置...', revision: '配置 revision：{revision}', mode: '配置编辑模式', formMode: '表单', jsonMode: 'JSON',
      selectValue: '请选择', addItem: '添加项目', removeItem: '移除项目', emptyArray: '暂无项目', item: '项目', invalidJSON: 'JSON 无效',
      schemaError: '{path} 的值不符合 Schema', requiredError: '{path} 为必填项'
    },
    messages: { configSaved: '{plugin} 配置已保存' },
    errors: { configLoad: '无法加载插件配置', configSave: '无法保存插件配置' }
  },
  accessGroups: {
    subtitle: '管理独立服务作用域的成员关系、资源授权和插件私有配额策略。',
    actions: { refresh: '刷新', newGroup: '新建访问组', open: '打开', editGroup: '编辑名称和说明', enable: '启用', disable: '禁用', add: '添加', removeNamed: '移除 {name}', addGrant: '添加授权', saveQuota: '保存配额', resolve: '解析有效授权' },
    filters: { label: '按服务作用域筛选', allScopes: '全部服务作用域' },
    table: { group: '访问组', scope: '作用域', state: '状态' },
    states: { enabled: '已启用', disabled: '已禁用' },
    groups: { title: '访问组', empty: '该作用域暂无访问组', emptyAll: '暂无访问组', emptyDescription: '访问组在服务作用域内为用户和套餐授予资源权限与配额策略。', noDescription: '暂无说明', directUnion: '直接成员和套餐成员按允许并集计算。' },
    detail: { title: '访问组', loading: '正在加载访问组详情…', group: '访问组', danger: '危险操作' },
    members: { title: '用户成员', userID: '用户 ID', empty: '暂无直接用户成员' },
    plans: { title: '套餐成员', planID: '套餐 ID', empty: '暂无套餐成员' },
    grants: { title: '资源授权', resourceType: '资源类型', resourceID: '资源 ID', permissions: '权限 JSON', empty: '暂无资源授权' },
    quotas: { title: '配额策略', key: '策略键', policy: '策略 JSON', empty: '暂无配额策略' },
    resolver: { title: '有效授权预览', description: '预览指定用户、套餐与服务作用域的有效授权。', userID: '用户 ID', planID: '套餐 ID（可选）', scope: '服务作用域', result: '命中 {count} 个已启用访问组', none: '未命中已启用访问组', policySummary: '当前生效 {grants} 条授权和 {quotas} 条配额策略。' },
    editor: { createTitle: '新建访问组', editTitle: '编辑访问组', name: '组名称', description: '说明', enabled: '启用该访问组', scopeFixed: '已有访问组的作用域不能修改。' },
    messages: { groupCreated: '已创建访问组 {name}', groupSaved: '已保存访问组 {name}', groupEnabled: '已启用访问组 {name}', groupDisabled: '已禁用访问组 {name}', groupDeleted: '已删除访问组 {name}', memberAdded: '已添加用户 #{id}', memberRemoved: '已移除用户 #{id}', planAdded: '已添加套餐 #{id}', planRemoved: '已移除套餐 #{id}', grantAdded: '已添加资源授权', grantRemoved: '已移除资源授权', quotaSaved: '已保存配额策略', quotaRemoved: '已移除配额策略' },
    confirm: { deleteGroupTitle: '删除访问组 {name}？', deleteGroup: '组内的成员关系、资源授权和配额策略会一并删除。此操作无法撤销。', deleteGroupAction: '删除访问组', removeGrantTitle: '移除资源授权 #{id}？', removeGrant: '该组成员将失去对 {resource} 的访问权限。此操作无法撤销。', removeGrantAction: '移除授权', removeQuotaTitle: '移除配额策略 {key}？', removeQuota: '该组将不再受此配额限制。此操作无法撤销。', removeQuotaAction: '移除策略' },
    errors: { load: '无法加载访问控制数据', loadGroups: '无法加载访问组', loadDetail: '无法加载访问组详情', groupRequired: '必须填写服务作用域和组名称', saveGroup: '无法保存访问组', deleteGroup: '无法删除访问组', member: '无法更新用户成员关系', plan: '无法更新套餐成员关系', grant: '无法更新资源授权', quota: '无法更新配额策略', resolve: '无法解析有效授权', invalidID: '{label} 必须为正整数', invalidJSON: '{label} 必须为有效 JSON', scopeRequired: '必须选择服务作用域' }
  },
  agentUpgrades: {
    title: 'Agent 升级',
    description: 'Agent 版本按 5%、25%、100% 分批推送，每批至少 30 分钟；单批失败率超过 5% 时自动回滚该批。',
    empty: '暂无 Agent 升级。超级管理员可执行：',
    emptyShort: '暂无 Agent 升级。',
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
      abort: '停止向其余节点下发升级，已升级节点保留新版本。',
      rollbackTitle: '回滚升级到 {version}？',
      rollback: '当前批次已升级节点恢复至原版本，并结束本次升级。'
    },
    loadFailed: '无法加载 Agent 升级'
  },
  // The channel names of the node page's Agent connection (the page that
  // listed every node's channel is gone).
  agentTransports: {
    transports: {
      'mtls-stream': 'mTLS 流'
    }
  }
}
