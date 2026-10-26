// realityTargets：节点页的「目标站」分区（Reality 目标站探测）。
export default {
  realityTargets: {
    title: '目标站',
    description: '由节点自己的 Agent 探测候选的 Reality 目标站（dest 与 SNI），按结果排序并推荐，可一键应用到节点的 Reality 协议。',
    refresh: '刷新',
    loading: '正在加载目标站…',
    loadFailed: '无法加载目标站',
    online: 'Agent 在线',
    notConnected: 'Agent 不在线',
    agentVersion: 'Agent {version}',
    nodeAddress: '节点地址 {address}',
    separator: '、',
    score: '{score} 分',
    current: {
      title: '当前目标',
      none: '这个节点没有 Reality 协议。',
      dest: 'dest {dest}'
    },
    probe: {
      title: '探测',
      offline: '节点的 Agent 连上 Control 后才能探测。',
      unsupported: '这个 Agent 不支持目标站探测，升级节点的 Agent 后可以使用。',
      unsupportedVersion: 'Agent {version} 不支持目标站探测，升级节点的 Agent 后可以使用。',
      candidates: '自定义候选',
      candidatesHint: '域名或 域名:端口，用空格、逗号或换行分隔，会先于内置列表探测。',
      withDefaults: '同时探测内置列表（{count} 个）',
      run: '开始探测',
      running: '节点正在逐个探测，最多需要一分多钟…'
    },
    recommended: {
      title: '推荐'
    },
    results: {
      title: '探测结果',
      none: '还没有探测结果。',
      handshake: '握手 {ms} ms',
      cdn: 'CDN：{cdn}',
      usedBy: '另有 {count} 个节点在用'
    },
    verdicts: {
      ok: '可用',
      failed: '不可用',
      error: '未完成'
    },
    failures: {
      resolve: '解析失败',
      private_address: '解析到内网地址',
      connect: '连不上',
      tls_handshake: 'TLS 握手失败',
      tls13: '不支持 TLS 1.3',
      h2: '不支持 h2',
      x25519: '不支持 X25519',
      certificate: '证书无效',
      http: 'HTTP 请求失败',
      redirect: '跳转到其他域名',
      reality: 'REALITY 握手失败'
    },
    auto: {
      title: '自动选择',
      description: '每 6 小时从节点复测一次当前目标站和内置候选；当前目标站连续两次探测失败（第二次在 15 分钟后）就自动换成推荐站。探测没有结果不算失败，还能用的目标站不会因为分数更高而被替换。每次切换都会写入操作日志。',
      nextRun: '下次复测：{time}',
      nextRunSoon: '下次复测：几分钟内',
      lastRun: '上次复测：{time}',
      outcome: '结果：{outcome}',
      lastSwitch: '上次自动切换：{time}',
      confirmTitle: '开启自动选择？',
      confirmMessage: '自动切换会改动这个节点 Reality 协议的 SNI 和 dest，客户端要更新订阅才能连上新的目标站。',
      confirm: '开启'
    },
    apply: {
      button: '应用',
      toProtocol: '应用到 {name}',
      inUse: '使用中',
      title: '把 {name} 的目标站改为 {target}？',
      message: 'SNI 和 dest 会从 {from} 改为 {target}，私钥等其他设置不变。节点会自动拿到新配置；客户端需要更新订阅，否则会连不上。',
      confirm: '应用',
      done: '已把 {name} 的目标站改为 {target}。请让用户更新订阅。'
    },
    errors: {
      node_offline: '节点的 Agent 不在线。',
      reality_probe_unsupported: '这个 Agent 不支持目标站探测。',
      probe_busy: '这个节点正在探测，请等它完成。',
      target_not_probed: '这个节点还没有成功探测过这个目标站，请先探测。',
      protocol_not_found: '找不到这个 Reality 协议。',
      invalid_params: '参数无效：{message}',
      unknown: '操作失败，请稍后重试。'
    }
  }
}
