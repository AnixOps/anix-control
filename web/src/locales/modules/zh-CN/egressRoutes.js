// 转发区域的「分流」页：把某个节点上用户访问的部分域名，改从另一个节点出去。
// 与 en/egressRoutes.js 键相同。
export default {
  egressRoutes: {
    nav: '分流',
    page: {
      title: '域名分流',
      description: '让一个节点上的用户访问指定域名时，从另一个节点出去（例如香港节点的 AI 域名走日本）。其他流量不变。',
      add: '新建分流',
      loadFailed: '无法加载分流规则',
      emptyTitle: '还没有分流规则',
      emptyDescription: '新建一条规则：选源节点、出口节点和要分流的域名。两端节点的 Agent 在一分钟内自动生效。',
      note: '两个节点之间用 Shadowsocks 2022 中继，密钥由 Control 生成，不会显示，也不会进入订阅。出口节点的防火墙需要放行中继端口。',
      domainCount: '{n} 个域名',
      disabled: '已停用',
      edit: '编辑…',
      enable: '启用',
      disable: '停用',
      delete: '删除…',
      nodeMissing: '节点 #{id} 已删除',
      columns: {
        name: '名称',
        path: '源节点 → 出口节点',
        domains: '域名',
        relay: '中继端口',
        state: '生效状态'
      },
      state: {
        applied: '已生效',
        pending: '下发中',
        failed: '失败'
      },
      stateSource: '源',
      stateExit: '出口'
    },
    sheet: {
      createTitle: '新建分流',
      editTitle: '编辑 {name}',
      description: '源节点上的用户访问这些域名时，流量经出口节点出去。只对 xray 内核承载的协议生效。',
      name: '名称',
      namePlaceholder: 'AI 走日本',
      source: '源节点',
      exit: '出口节点',
      exitHost: '出口地址',
      exitHostHelp: '源节点连接出口节点用的地址，留空则用出口节点的地址。',
      relayPort: '中继端口',
      relayPortHelp: '出口节点上监听的端口，不能和它的协议端口或其他中继重复。',
      domains: '域名',
      domainsHelp: '每行一个。写 openai.com 会同时匹配它的所有子域名；也可写 full:、keyword:、regexp: 开头的规则。',
      enabled: '启用',
      rotateKey: '更换中继密钥',
      rotateKeyHelp: '两端节点会一起换成新密钥。',
      presetAi: '填入常用 AI 域名',
      create: '创建',
      save: '保存',
      required: '必填',
      sameNode: '出口节点不能是源节点'
    },
    delete: {
      title: '删除 {name}？',
      message: '两端节点会在一分钟内撤掉这条分流，这些域名恢复从源节点直接出去。',
      confirm: '删除'
    },
    toast: {
      created: '已创建 {name}',
      saved: '已保存 {name}',
      deleted: '已删除 {name}'
    }
  }
}
