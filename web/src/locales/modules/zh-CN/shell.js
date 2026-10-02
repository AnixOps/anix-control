// App shell strings (UI redesign phase U3): navigation, account menu,
// command palette, status pages and the account page. Written for Chinese
// readers, not translated from the English file.
export default {
  shell: {
    admin: {
      navLabel: '管理导航',
      sidebarLabel: '侧栏',
      collapse: '收起侧栏',
      expand: '展开侧栏',
      breadcrumb: '位置',
      groups: {
        overview: '概览',
        users: '用户',
        network: '网络',
        extensions: '扩展',
        system: '系统',
        commerce: '商业'
      },
      items: {
        dashboard: '仪表盘',
        monitor: '流量与监控',
        users: '用户',
        inviteCodes: '邀请码',
        subscriptions: '订阅分组',
        templates: '订阅模板',
        tickets: '工单',
        knowledge: '帮助中心内容',
        nodes: '节点',
        forward: '转发',
        forwardNodes: '转发节点',
        agents: 'NodeX Agents',
        plugins: '插件中心',
        deployments: '部署编排',
        settings: '系统设置',
        security: '安全',
        notifications: '通知',
        plans: '套餐',
        orders: '订单',
        coupons: '优惠券',
        payment: '支付',
        invite: '邀请返佣',
        account: '账户'
      }
    },
    user: {
      navLabel: '主导航',
      items: {
        dashboard: '概览',
        subscribe: '订阅',
        knowledge: '帮助中心',
        knowledgeShort: '帮助',
        tickets: '工单',
        account: '账户',
        plans: '套餐',
        orders: '订单'
      }
    },
    forwardNav: {
      label: '转发套件',
      more: '更多',
      moreLabel: '更多转发工具'
    },
    search: {
      button: '搜索或跳转…',
      label: '搜索或跳转（{shortcut}）'
    },
    account: {
      menuLabel: '账户菜单：{name}',
      account: '账户',
      appearance: '外观',
      language: '语言',
      about: '关于',
      logout: '退出登录',
      roleAdmin: '管理员',
      roleUser: '用户',
      themes: {
        system: '跟随系统',
        light: '浅色',
        dark: '深色'
      }
    },
    about: {
      title: '关于 AnixOps Control',
      version: '版本',
      backendBuild: '后端构建时间',
      commit: '提交',
      frontendBuild: '前端构建号',
      frontendTime: '前端构建时间',
      unavailable: '暂时无法读取版本信息。',
      close: '完成'
    },
    palette: {
      title: '搜索或跳转',
      description: '输入页面名称、操作或用户邮箱，按回车打开。',
      placeholder: '搜索页面、操作或用户邮箱…',
      empty: '没有匹配的结果',
      searching: '正在查找用户…',
      searchFailed: '无法查找用户',
      groups: {
        actions: '操作',
        pages: '页面',
        users: '用户'
      },
      hints: {
        move: '选择',
        open: '打开',
        close: '关闭'
      },
      actions: {
        addNode: '添加节点',
        addUser: '添加用户',
        forwardWizard: '转发快速向导',
        themeLight: '切换到浅色外观',
        themeDark: '切换到深色外观',
        themeSystem: '外观跟随系统',
        language: '切换语言：{language}'
      },
      userResult: '在用户列表中查看'
    },
    status: {
      notFound: {
        title: '找不到这个页面',
        description: '链接可能有误，或者页面已经移动。'
      },
      forbidden: {
        title: '没有权限访问这个页面',
        description: '当前账户不能打开这里。需要访问时，请联系管理员。'
      },
      home: '返回首页',
      back: '返回上一页',
      code: '错误 {code}'
    },
    accountPage: {
      title: '账户',
      subtitle: '个人资料、两步验证、语言与外观。',
      profile: {
        title: '个人资料',
        email: '邮箱',
        id: '用户 ID',
        role: '身份'
      },
      mfa: {
        title: '两步验证',
        description: '登录时除了密码，还要输入验证器 App 生成的 6 位验证码。',
        status: '状态',
        on: '已开启',
        off: '未开启',
        backupCodes: '恢复码',
        remaining: '剩余 {count} 个',
        none: '没有',
        lastUsed: '最近使用',
        enable: '开启两步验证',
        disable: '关闭两步验证…',
        regenerate: '重新生成恢复码…',
        loadFailed: '无法读取两步验证状态。',
        retry: '重试',
        setup: {
          title: '开启两步验证',
          description: '在验证器 App（如 1Password、Google Authenticator）里添加这个账户，再输入它显示的验证码。',
          secret: '密钥',
          secretHelp: '在验证器里选择「手动输入密钥」，粘贴这串字符。',
          openApp: '在本机验证器中打开',
          scan: '用验证器 App 扫描二维码',
          qrLabel: '两步验证设置二维码',
          manual: '无法扫码？手动输入密钥',
          enterCode: '输入验证器显示的 6 位验证码',
          code: '6 位验证码',
          codeHelp: '验证器每 30 秒更新一次。',
          confirm: '验证并开启',
          preparing: '正在生成密钥…',
          enabled: '两步验证已开启'
        },
        backup: {
          title: '保存恢复码',
          description: '手机丢失时，可以用恢复码登录。每个恢复码只能用一次，请保存到安全的地方。',
          copyAll: '复制全部',
          download: '下载',
          done: '我已保存'
        },
        disableDialog: {
          title: '关闭两步验证？',
          description: '关闭后，登录只需要密码。输入当前密码确认。',
          password: '当前密码',
          confirm: '关闭两步验证',
          done: '两步验证已关闭'
        },
        regenerateDialog: {
          title: '重新生成恢复码？',
          description: '旧的恢复码会立即失效。',
          confirm: '重新生成'
        },
        errors: {
          code: '请输入 6 位数字验证码',
          password: '请输入当前密码',
          failed: '操作没有完成：{message}'
        }
      },
      language: {
        title: '语言',
        label: '界面语言'
      },
      appearance: {
        title: '外观',
        label: '主题'
      }
    }
  }
}
