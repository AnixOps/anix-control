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
    viaLine: '经由 {name}',
    uncovered: '订阅里不会出现：{protocols}。这个节点有入口后只按入口输出，这些协议端口没有被任何启用的入口覆盖（NAT 映射的机器端口要填协议端口）。补上映射，或加一个直连入口。',
    listSeparator: '、',
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
      nat: 'NAT 映射',
      iwan: '经 iWAN 线路'
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
        nat: '厂商把公网地址的端口映射到机器上，需要填端口映射。',
        iwan: 'AnixOps Net 先登录 Panabit iWAN 线路，再经由线路连这个节点。和节点地址、其他入口同时提供，只对 VMess、VLESS、Trojan、Shadowsocks 生效，也只发给 AnixOps Net。'
      },
      line: 'iWAN 线路',
      linePlaceholder: '选择线路',
      noLines: '先在「iWAN 线路」里添加线路',
      lineHelp: '线路的共用账号会发给这个节点的所有订阅用户。',
      iwanHostHelp: '线路出口连这个节点用的地址，一般就是节点地址。',
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
    },
    lines: {
      open: 'iWAN 线路',
      title: 'Panabit iWAN 线路',
      description: '第三方的 iWAN 入口（比如 Panabit 设备和它的 IPLC 专线），AnixOps Net 用线路的共用账号登录，我们不在上面装任何东西。iWAN 本身不加密，整条路径由节点自己的协议加密。',
      empty: '还没有线路。',
      used: '{n} 个入口在用',
      add: '添加线路',
      edit: '编辑 {name}',
      remove: '删除 {name}',
      close: '完成',
      name: '名称',
      nameHelp: '在订阅里作为一个节点显示，不能重名。',
      host: '服务器',
      port: '端口',
      username: '用户名',
      password: '口令',
      passwordKeep: '留空则保留原口令。',
      mtu: 'MTU',
      mtuHelp: '留空或 0 用客户端默认值，否则 576 到 1600。',
      encrypt: 'XOR 掩码',
      encryptHelp: '只是让内容不那么一眼可见，不是加密。',
      badPort: '端口是 1 到 65535',
      badMtu: '0，或 576 到 1600',
      saved: '已保存线路 {name}，订阅立即生效',
      removeTitle: '删除线路 {name}？',
      removeMessage: '订阅里不再提供这条线路，无法撤销。',
      removeAction: '删除线路'
    }
  }
}
