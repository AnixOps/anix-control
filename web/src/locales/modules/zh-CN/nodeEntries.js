// 节点页「协议」标签下的「入口」：订阅里用哪些地址连这个节点（多线路、原生
// IPv6、厂商 NAT 端口映射）。与 en/nodeEntries.js 键相同。
export default {
  nodeEntries: {
    title: '入口',
    description: '用户连这个节点用的地址。机器有多条线路（如 CN2 和 9929），或者 IPv6 直连、IPv4 走厂商 NAT 时，在这里各加一个入口：订阅会按入口给每个协议各生成一条，名字后面带入口名。',
    edit: '编辑入口',
    add: '添加入口',
    tableLabel: '{name} 的入口',
    loadFailed: '无法加载入口',
    emptyTitle: '没有额外入口',
    emptyDescription: '订阅使用节点地址 {host}。',
    enabled: '启用',
    disabled: '停用',
    samePorts: '与协议端口相同',
    saved: '已保存 {name} 的入口，订阅立即生效',
    columns: {
      label: '名称',
      kind: '类型',
      host: '地址',
      family: '地址类型',
      ports: '端口',
      status: '状态'
    },
    kinds: {
      direct: '直连',
      nat: 'NAT 映射'
    },
    families: {
      ipv4: 'IPv4',
      ipv6: 'IPv6',
      domain: '域名'
    },
    sheet: {
      title: '编辑 {name} 的入口',
      description: '保存后整个列表替换原来的入口，订阅立即按新入口生成；节点本身的配置不变。中转机若是 IPv6 进、IPv4 出，转发要用 gost 这类用户态程序监听 [::]（nftables DNAT 不能跨 IPv4/IPv6）。',
      empty: '还没有入口，订阅使用节点地址 {host}。',
      entryN: '入口 {n}',
      remove: '删除这个入口',
      label: '名称',
      labelPlaceholder: 'CN2 v6',
      labelHelp: '显示在订阅节点名后面，例如「CN2 v6」「9929 NAT」。',
      kind: '类型',
      kindHelp: {
        direct: '用户直接连这个地址，端口与协议端口相同（例如机器自带的 IPv6）。',
        nat: '厂商把公网地址的端口映射到机器上，需要填端口映射。'
      },
      host: '地址',
      portMap: '端口映射',
      portMapHelp: '每行一段：公网端口 -> 机器端口，例如 21000-21099 -> 30000-30099。没映射到的协议端口，这个入口下不生成节点。',
      enabled: '启用',
      cancel: '取消',
      save: '保存',
      required: '必填',
      noPipe: "名称里不能有 {'|'}",
      duplicate: '名称重复',
      badLine: '第 {line} 行格式不对，或两边端口数量不一样'
    }
  }
}
