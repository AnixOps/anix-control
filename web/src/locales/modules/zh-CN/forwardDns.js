// 转发页面里的入口高可用（L2）：DNS 服务商页面、路由编辑器的绑定选择和
// 路由详情的「入口高可用」卡片。与 en/forwardDns.js 键相同。
export default {
  forwardDns: {
    nav: 'DNS',
    superOnly: '仅超级管理员可添加、修改或删除 DNS 服务商。',
    page: {
      title: 'DNS 服务商',
      description: 'Control 通过这些 DNS 账号写入口记录：让每个已绑定的入口主机名始终指向健康的入口节点。',
      add: '添加服务商',
      readOnly: '你可以查看服务商；只有超级管理员可以添加、修改或删除。',
      loadFailed: '无法加载 DNS 服务商',
      emptyTitle: '还没有 DNS 服务商',
      emptyDescription: '添加托管入口主机名的 DNS 服务账号，然后在路由编辑器中把路由的入口主机名绑定到它。',
      defaultEndpoint: '服务商默认',
      missing: '缺少：{names}',
      bindingCount: '{n} 个绑定',
      unused: '未使用',
      note: '凭据只写不读：Control 加密保存，之后不再显示，这里只列出名称。',
      edit: '编辑…',
      editSuperOnly: '编辑（仅超级管理员）',
      delete: '删除…',
      deleteSuperOnly: '删除（仅超级管理员）',
      deleteInUse: '删除（{n} 个绑定正在使用）',
      columns: {
        name: '服务商',
        bindings: '绑定',
        endpoint: '接口地址',
        credentials: '凭据',
        updated: '更新时间'
      }
    },
    sheet: {
      createTitle: '添加 DNS 服务商',
      editTitle: '编辑 {name}',
      description: '凭据只授予 Control 要写入的那个区域的权限，不要多给。',
      name: '名称',
      namePlaceholder: 'cloudflare-main',
      kind: '服务商类型',
      kindFixed: '服务商类型创建后不能修改，如需更换请另外添加。',
      settings: '设置',
      credentials: '凭据',
      credentialsHelp: '在 Control 上加密保存，之后不再显示。',
      credentialsKeep: '已保存的凭据显示为 ********。保持不变即沿用已保存的值，输入新值则替换。',
      stored: '已保存。保持不变即沿用。',
      create: '添加服务商',
      save: '保存'
    },
    kindHelp: {
      DNS_PROVIDER_KIND_CLOUDFLARE: '一个在你的区域上具有 Zone → DNS → Edit 和 Zone → Zone → Read 权限的 API 令牌。',
      DNS_PROVIDER_KIND_ALIDNS: 'RAM 用户的 AccessKey，仅授予该域名的 alidns 记录操作。',
      DNS_PROVIDER_KIND_DNSPOD: 'CAM 子用户的 SecretId 和 SecretKey，仅授予 dnspod 记录操作。',
      DNS_PROVIDER_KIND_HUAWEICLOUD: 'IAM 用户的访问密钥（AK/SK），仅授予 dns 区域与记录集操作。',
      DNS_PROVIDER_KIND_WEBHOOK: 'Control 把每次变更以 HMAC-SHA256 签名后 POST 到你的 HTTPS 接口。'
    },
    config: {
      endpoint: 'API 地址',
      url: 'Webhook 地址'
    },
    configHelp: {
      endpoint: '可选：用地域 API 主机替代服务商默认地址。',
      url: '一个 https:// 地址。'
    },
    configPlaceholder: {
      DNS_PROVIDER_KIND_CLOUDFLARE: { endpoint: 'api.cloudflare.com' },
      DNS_PROVIDER_KIND_ALIDNS: { endpoint: 'alidns.aliyuncs.com' },
      DNS_PROVIDER_KIND_DNSPOD: { endpoint: 'dnspod.tencentcloudapi.com' },
      DNS_PROVIDER_KIND_HUAWEICLOUD: { endpoint: 'dns.myhuaweicloud.com' },
      DNS_PROVIDER_KIND_WEBHOOK: { url: 'https://dns-hook.example.com/anixops' }
    },
    credential: {
      api_token: 'API 令牌',
      access_key_id: 'AccessKey ID',
      access_key_secret: 'AccessKey Secret',
      secret_id: 'SecretId',
      secret_key: 'SecretKey',
      access_key: '访问密钥（AK）',
      secret: '签名密钥'
    },
    delete: {
      title: '删除服务商 {name}？',
      message: 'Control 会删除它的凭据；已发布到服务商的记录保持不变。',
      confirm: '删除服务商'
    },
    mode: {
      DNS_BINDING_MODE_DDNS: 'DDNS',
      DNS_BINDING_MODE_CNAME: 'CNAME'
    },
    modeHint: {
      DNS_BINDING_MODE_DDNS: 'Control 直接写入口主机名的记录。',
      DNS_BINDING_MODE_CNAME: 'Control 写一个它管理的名称；你把入口主机名 CNAME 到它（只需一次）。'
    },
    binding: {
      hostnameHelp: '客户端访问此路由各入口时使用的统一名称。可在下方绑定 DNS 服务商，或自行维护它的记录。',
      enable: '让此主机名始终指向健康的入口',
      enableHelp: '入口节点故障和恢复时，Control 通过 DNS 服务商更新 A/AAAA 记录。',
      bound: '已绑定 DNS 服务商',
      provider: 'DNS 服务商',
      providerPlaceholder: '选择服务商',
      noProviders: '还没有 DNS 服务商。',
      manageProviders: '管理 DNS 服务商',
      zone: '区域',
      zoneHelp: '记录所在的服务商区域（Zone）。',
      mode: '模式',
      recordName: '托管名称',
      recordNamePlaceholder: 'r1.ha.example.net',
      recordNameHelp: '区域内由 Control 写入的名称。',
      record: '记录',
      ddnsRecord: '记录：',
      recordTypes: '记录类型',
      recordTypesHelp: '取消某个类型会删除该类型的记录。',
      type: {
        A: 'IPv4 地址',
        AAAA: 'IPv6 地址'
      },
      ttl: 'TTL',
      seconds: '秒',
      ttlHelp: '设为服务商允许的最小值（默认 60）。',
      paused: '暂停',
      pausedHelp: '保留已发布的记录，不做任何变更。',
      cnameInstruction: '在 {host} 所在的 DNS 中创建一次 CNAME：{host} → 托管名称。',
      cnameTarget: 'CNAME 目标',
      copy: '复制',
      immutable: '服务商、区域、名称和模式绑定后不能修改。如需更换，请在路由详情中解除绑定（并删除记录）后重新绑定。',
      mismatch: '此绑定写入的是 {name}。DDNS 模式下名称不能修改：请在路由详情中解除绑定，再绑定新的主机名。',
      needsHostname: '请先填写入口主机名。',
      required: '必填',
      outsideZone: '名称必须在该区域内。',
      typeRequired: '至少选择一种记录类型。',
      ttlRange: '1 到 {max} 秒。',
      saveFailed: '路由已保存，但 DNS 绑定未保存：{message}'
    },
    card: {
      title: '入口高可用',
      description: '入口主机名的 DNS 记录跟随健康的入口节点。',
      loadFailed: '无法加载 DNS 状态',
      edit: '编辑绑定',
      unbind: '解除绑定…',
      unbindSuperOnly: '仅超级管理员可解除绑定',
      unboundHost: '{host} 未绑定 DNS 服务商：它的记录由你自行维护。',
      unboundNoHost: '此路由没有入口主机名。在编辑器中填写后即可绑定 DNS 服务商。',
      bind: '绑定 DNS',
      setHostname: '填写入口主机名',
      lastError: '最近错误',
      nextAttempt: '下次尝试：{when}',
      hostname: '入口主机名',
      mode: '模式',
      zone: '区域',
      ttl: 'TTL',
      ttlValue: '{n} 秒',
      published: '发布时间',
      evaluated: '评估时间',
      never: '尚未发布',
      records: '记录',
      type: '类型',
      publishedValues: '已发布',
      desiredValues: '期望',
      none: '无',
      keepPublished: '无健康入口：保留已发布',
      entries: '入口节点',
      noAddress: '无公网地址',
      inRotation: '轮换中',
      outOfRotation: '已移出',
      joining: '健康 {n}/3，即将加入',
      leaving: '异常 {n}/3，即将移出',
      timing: 'Control 每 10 秒评估一次：节点连续 3 次异常后移出，连续 3 次健康后重新加入。此卡片每 30 秒刷新。'
    },
    unbind: {
      title: '解除 {host} 的绑定？',
      description: 'Control 将不再维护该主机名的记录。',
      purge: '同时删除 Control 发布的记录',
      purgeHelp: '不勾选则记录按原样保留在服务商处。',
      purgeFailedHint: '绑定已保留。可重试，或不删除记录直接解除。',
      confirm: '解除绑定'
    },
    state: {
      ok: '正常',
      pending: '等待发布',
      degraded: '降级',
      error: '错误',
      rate_limited: '限流中',
      paused: '已暂停',
      unbound: '未绑定',
      route_missing: '路由不存在',
      hostname_mismatch: '主机名不一致'
    },
    stateHelp: {
      ok: '记录即为健康入口的地址。',
      pending: '尚未发布：绑定后还没有入口健康过。',
      degraded: '没有健康的入口：Control 保留最后发布的记录，而不是发布空集。',
      error: '服务商拒绝或失败，Control 会退避重试。',
      rate_limited: '服务商的调用额度已用完，变更将在几秒后进行。',
      paused: '绑定或路由已暂停或受限：不做任何变更。',
      unbound: '未绑定 DNS 服务商。',
      route_missing: '路由已删除，请解除绑定。',
      hostname_mismatch: 'DDNS 模式下路由的入口主机名已不是绑定的名称。请解除绑定后重新绑定。'
    },
    reason: {
      healthy: '健康',
      converging: '收敛中',
      not_in_inventory: '不在清单中',
      no_address: '无公网地址',
      never_reported: '从未上报',
      report_stale: '上报过期',
      offline: '离线',
      hop_error: '跳错误',
      upstreams_down: '上游全部不可用'
    },
    codes: {
      secret_store_unavailable: 'Control 没有用于加密凭据的密钥（module_runtime.ca_kek）。',
      provider_in_use: '有路由绑定正在使用此服务商，请先解除绑定。',
      binding_exists: '此路由，或此服务商、区域和名称，已有绑定。',
      name_taken: '已有同名服务商。',
      unknown_route: '路由尚未保存。',
      unknown_provider: '该 DNS 服务商已不存在。',
      entry_hostname_required: '路由没有入口主机名。',
      hostname_mismatch: 'DDNS 模式下记录名必须是路由的入口主机名。',
      immutable: '服务商、区域、名称和模式不能修改；请解除绑定后重新绑定。'
    },
    errors: {
      dns_purge_failed: 'DNS 服务商未能删除记录。'
    },
    toast: {
      created: '已添加 {name}',
      saved: '已保存 {name}',
      deleted: '已删除 {name}',
      unbound: '已解除 {host} 的绑定'
    }
  }
}
