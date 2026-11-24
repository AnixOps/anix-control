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
      // The banner for the certificates the panel serves itself.
      certificate: {
        failed: '{name} 证书申请失败，当前使用临时证书，节点无法接入',
        pending: '正在申请 {name} 证书，完成前使用临时证书，节点无法接入',
        renewalFailing: '{name} 证书续期失败，当前证书于 {date} 到期',
        untrusted: '{name} 证书不受公共证书机构信任，节点无法接入',
        reason: '原因：{reason}',
        retry: '重新申请',
        retryStarted: '已重新申请，结果将在此处显示。',
        retryWaiting: '一小时内已申请 3 次，下次申请时间：{date}。',
        retryFailed: '申请未发出。请稍后重试，或在服务器上运行 anix-control tls retry。',
        fixes: {
          dns_unresolved: '为 {name} 添加指向本机的 A（及 AAAA）记录，关闭 CDN 代理后重新申请。',
          dns_elsewhere: '{name} 未解析到本机：请将记录指向本机并关闭 CDN 代理。如需使用 CDN，请设置 tls.mode: external，并将 grpc.public_name 设为仅解析（不代理）到本机的域名。',
          unreachable: '证书机构无法访问 {name} 的 443 和 80 端口：请在防火墙和安全组中放行这两个端口，并停止占用端口的其他程序。',
          challenge_failed: '证书机构访问 {name} 时未收到本面板的应答：请将记录指向本机并关闭代理，或将 /.well-known/acme-challenge/ 转发至本面板。',
          rate_limited: '已触发证书机构频率限制，面板将自动重试。请勿重装：证书和账户保存在数据库中。',
          ca_unreachable: '本机无法连接证书机构：请检查出站 HTTPS（443 端口）和 DNS。',
          ca_error: '面板将自动重试。详情请在服务器上运行 anix-control tls status。',
          file_unreadable: '请检查证书和私钥文件是否存在、可读，且为匹配的 PEM 证书链和私钥。文件更新后自动重新加载。',
          expired: '证书已过期：请更换为续期后的证书，或按上述原因处理后重新申请。',
          name_mismatch: '证书不包含 {name}：请更换为该域名签发的证书。',
          untrusted: '请使用公共证书机构签发的证书。设置 domain 并使用 tls.mode: acme 时由面板自动申请。'
        }
      },
      groups: {
        overview: '概览',
        users: '用户',
        network: '网络',
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
        lines: '线路',
        topology: '拓扑',
        forward: '转发',
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
        traffic: '访问统计',
        tickets: '工单',
        account: '账户',
        plans: '套餐',
        orders: '订单'
      }
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
      source: '源代码',
      revealAddresses: '显示服务器 IP',
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
      unavailable: '无法读取版本信息。',
      close: '完成'
    },
    palette: {
      title: '搜索或跳转',
      description: '输入页面名称、操作或用户邮箱，按回车键打开。',
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
        addNode: '接入节点',
        addUser: '添加用户',
        newForwardRoute: '新建转发路由',
        upgradeNodes: '升级节点',
        themeLight: '切换到浅色外观',
        themeDark: '切换到深色外观',
        themeSystem: '外观跟随系统',
        revealAddresses: '显示服务器 IP',
        hideAddresses: '隐藏服务器 IP',
        language: '切换语言：{language}'
      },
      userResult: '在用户列表中查看'
    },
    status: {
      notFound: {
        title: '页面不存在',
        description: '链接有误或页面已移动。'
      },
      forbidden: {
        title: '无权访问此页面',
        description: '当前账户无访问权限。如需访问，请联系管理员。'
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
        description: '登录时需输入密码及验证器生成的 6 位验证码。',
        status: '状态',
        on: '已开启',
        off: '未开启',
        backupCodes: '恢复码',
        remaining: '剩余 {count} 个',
        none: '无',
        lastUsed: '最近使用',
        enable: '开启两步验证',
        disable: '关闭两步验证…',
        regenerate: '重新生成恢复码…',
        loadFailed: '无法读取两步验证状态。',
        retry: '重试',
        setup: {
          title: '开启两步验证',
          description: '在验证器（如 1Password、Google Authenticator）中添加此账户，然后输入验证码。',
          secret: '密钥',
          secretHelp: '在验证器中选择“手动输入密钥”并粘贴。',
          openApp: '在本机验证器中打开',
          scan: '使用验证器扫描二维码',
          qrLabel: '两步验证设置二维码',
          manual: '手动输入密钥',
          enterCode: '输入 6 位验证码',
          code: '6 位验证码',
          codeHelp: '验证码每 30 秒更新。',
          confirm: '验证并开启',
          preparing: '正在生成密钥…',
          enabled: '两步验证已开启'
        },
        backup: {
          title: '保存恢复码',
          description: '无法使用验证器时，可用恢复码登录。每个恢复码仅可使用一次，请妥善保存。',
          copyAll: '复制全部',
          download: '下载',
          done: '我已保存'
        },
        disableDialog: {
          title: '关闭两步验证？',
          description: '关闭后仅需密码即可登录。请输入当前密码确认。',
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
          failed: '操作未完成：{message}'
        }
      },
      telegram: {
        title: 'Telegram',
        description: '绑定后可通过 Telegram 机器人查看账户、获取订阅，并接收到期和流量提醒。',
        status: '状态',
        bound: '已绑定',
        notBound: '未绑定',
        account: 'Telegram 账号',
        bind: '绑定 Telegram',
        unbind: '解除绑定',
        bindDisabled: '自助绑定已关闭，请联系管理员。',
        loadFailed: '无法读取 Telegram 绑定',
        retry: '重试',
        unbindTitle: '解除 Telegram 绑定？',
        unbindMessage: '解除后将不再接收 Telegram 提醒，机器人也无法访问此账户。',
        unbound: '已解除 Telegram 绑定',
        bindDone: 'Telegram 已绑定',
        failed: '操作失败：{message}',
        dialog: {
          title: '绑定 Telegram',
          description: '扫描二维码或打开 Telegram，在机器人中点击“开始”完成绑定。链接 10 分钟内有效，仅可绑定当前账户。',
          preparing: '正在生成绑定链接…',
          qrLabel: 'Telegram 绑定二维码',
          open: '打开 Telegram',
          manual: '或向机器人发送以下命令',
          waiting: '等待在 Telegram 中点击“开始”…',
          expires: '有效期至 {time}',
          done: '完成'
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
