// 转发区域的「分流」页：把某个节点上用户访问的部分域名，改从另一个节点出去。
// 与 en/egressRoutes.js 键相同。
export default {
  egressRoutes: {
    nav: '分流',
    page: {
      title: '域名分流',
      description: '源节点用户访问指定域名时，经另一节点出口。其他流量不变。',
      add: '新建分流',
      loadFailed: '无法加载分流规则',
      emptyTitle: '暂无分流规则',
      emptyDescription: '新建规则：选择源节点、出口节点和域名。一分钟内生效。',
      note: '节点间使用 Shadowsocks 2022 中继，密钥自动生成且不显示。出口节点防火墙须放行中继端口。',
      domainCount: '{n} 个域名',
      disabled: '已停用',
      edit: '编辑…',
      enable: '启用',
      disable: '停用',
      delete: '删除…',
      nodeMissing: '节点 #{id} 已删除',
      fallbacks: '（备用 {names}）',
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
      description: '源节点用户访问以下域名时，流量经出口节点。',
      name: '名称',
      namePlaceholder: 'AI 走日本',
      source: '源节点',
      exit: '出口节点',
      exitHost: '出口地址',
      exitHostHelp: '源节点连接出口节点的地址，留空使用出口节点地址。',
      relayPort: '中继端口',
      relayPortHelp: '出口节点监听端口，不可与其协议端口或其他中继重复。',
      domains: '域名',
      domainsHelp: '每行一个，匹配其全部子域名；支持 full:、keyword:、regexp: 前缀。',
      enabled: '启用',
      rotateKey: '更换中继密钥',
      rotateKeyHelp: '两端节点同时更换。',
      presetAi: '填入常用 AI 域名',
      create: '创建',
      save: '保存',
      required: '必填',
      sameNode: '出口节点不能为源节点',
      fallbacks: '备用出口',
      fallbacksHelp: '出口不可用时按顺序切换至备用出口，故障出口 30 秒内不重试，恢复后自动切回。各备用出口监听各自的中继端口。',
      fallbackNode: '备用出口 {n}',
      addFallback: '添加备用出口',
      removeFallback: '移除',
      fallbackRepeated: '源节点、出口节点和备用出口不可重复'
    },
    delete: {
      title: '删除 {name}？',
      message: '一分钟内撤销该分流，相关域名恢复由源节点直接出口。',
      confirm: '删除'
    },
    toast: {
      created: '已创建 {name}',
      saved: '已保存 {name}',
      deleted: '已删除 {name}'
    }
  }
}
