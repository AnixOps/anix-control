// realityTargets：节点页的「目标站」分区（Reality 目标站探测）。
export default {
  realityTargets: {
    title: '目标站',
    description: '由节点 Agent 探测 Reality 目标站（dest 与 SNI），按结果排序推荐，可应用至节点的 Reality 协议。',
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
      none: '该节点无 Reality 协议。',
      dest: 'dest {dest}'
    },
    probe: {
      title: '探测',
      offline: 'Agent 连接 Control 后方可探测。',
      unsupported: '该 Agent 不支持目标站探测，请升级。',
      unsupportedVersion: 'Agent {version} 不支持目标站探测，请升级。',
      candidates: '自定义候选',
      candidatesHint: '域名或域名:端口，以空格、逗号或换行分隔，优先于内置列表探测。',
      withDefaults: '同时探测内置列表（{count} 个）',
      run: '开始探测',
      running: '正在探测，最长约需一分钟…'
    },
    recommended: {
      title: '推荐'
    },
    results: {
      title: '探测结果',
      none: '暂无探测结果。',
      handshake: '握手 {ms} ms',
      cdn: 'CDN：{cdn}',
      usedBy: '另有 {count} 个节点使用'
    },
    verdicts: {
      ok: '可用',
      failed: '不可用',
      error: '未完成'
    },
    failures: {
      resolve: '解析失败',
      private_address: '解析到内网地址',
      connect: '无法连接',
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
      description: '每 6 小时复测当前目标站及内置候选；当前目标站连续两次（间隔 15 分钟）探测失败时，自动切换为推荐站。切换记入操作日志。',
      nextRun: '下次复测：{time}',
      nextRunSoon: '下次复测：几分钟内',
      lastRun: '上次复测：{time}',
      outcome: '结果：{outcome}',
      lastSwitch: '上次自动切换：{time}',
      confirmTitle: '开启自动选择？',
      confirmMessage: '自动切换将修改该节点 Reality 协议的 SNI 和 dest，客户端须更新订阅。',
      confirm: '开启'
    },
    apply: {
      button: '应用',
      toProtocol: '应用到 {name}',
      inUse: '使用中',
      title: '将 {name} 的目标站改为 {target}？',
      message: 'SNI 和 dest 将由 {from} 改为 {target}，其他设置不变。客户端须更新订阅。',
      confirm: '应用',
      done: '{name} 的目标站已改为 {target}，请通知用户更新订阅。'
    },
    errors: {
      node_offline: '节点 Agent 不在线。',
      reality_probe_unsupported: '该 Agent 不支持目标站探测。',
      probe_busy: '该节点正在探测，请稍后。',
      target_not_probed: '该目标站未成功探测，请先探测。',
      protocol_not_found: 'Reality 协议不存在。',
      invalid_params: '参数无效：{message}',
      unknown: '操作失败，请稍后重试。'
    }
  }
}
