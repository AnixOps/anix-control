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
        failed: '{name} 的证书申请失败，面板正在使用临时证书，节点暂时无法接入',
        pending: '正在为 {name} 申请证书，完成前面板使用临时证书，节点暂时无法接入',
        renewalFailing: '{name} 的证书续期失败，当前证书 {date} 到期',
        untrusted: '{name} 的证书不受公共证书机构信任，节点无法接入',
        reason: '原因：{reason}',
        retry: '重新申请',
        retryStarted: '已重新申请，结果稍后显示在这里。',
        retryWaiting: '一小时内已申请 3 次，下一次在 {date}。',
        retryFailed: '重新申请没有发出，请稍后再试，或在服务器上运行 anix-control tls retry。',
        fixes: {
          dns_unresolved: '为 {name} 添加指向本机的 A（以及 AAAA）记录，不要开启 CDN 代理，然后重新申请。',
          dns_elsewhere: '{name} 没有解析到本机：把记录改为指向本机并关闭 CDN 代理。面板必须放在 CDN 后面时，改用 tls.mode: external，并把 grpc.public_name 设为一个只解析（不代理）到本机的域名供节点连接。',
          unreachable: '证书机构连不上 {name} 的 443 和 80 端口：在防火墙和云服务商的安全组里放行这两个端口，并停掉占用它们的其他程序。',
          challenge_failed: '证书机构访问 {name} 时得到的不是本面板的应答：域名指向了别处，或有 CDN、其他网站在应答。把记录指向本机并关闭代理，或把 /.well-known/acme-challenge/ 转发给本面板。',
          rate_limited: '触发了证书机构的频率限制，面板会稍后自动重试。不要重装：证书和账户保存在数据库里。',
          ca_unreachable: '本机连不上证书机构：检查本机的出站 HTTPS（443 端口）和 DNS。',
          ca_error: '面板会自动重试；也可以在服务器上运行 anix-control tls status 查看详情。',
          file_unreadable: '检查证书文件和私钥文件是否存在、面板能否读取、内容是否为匹配的 PEM 证书链和私钥；文件改好后面板会自动重新加载。',
          expired: '证书已经过期：更换为续期后的证书（证书文件会自动重新加载），或按上面的原因处理后重新申请。',
          name_mismatch: '证书不包含 {name}：换成为这个域名签发的证书。',
          untrusted: '节点和浏览器只接受公共证书机构签发的证书：换成公共证书机构的证书；设置 domain 并使用 tls.mode: acme 时面板会自动申请。'
        }
      },
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
        addNode: '接入节点',
        addUser: '添加用户',
        newForwardRoute: '新建转发路由',
        upgradeNodes: '升级节点',
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
