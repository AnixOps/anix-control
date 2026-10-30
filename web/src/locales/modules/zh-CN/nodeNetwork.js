// 节点页「协议」标签下的「网络」：自动线路的出口网卡、源地址、网关和中转端口
// （NAT、多网卡机器）。与 en/nodeNetwork.js 键相同。
export default {
  nodeNetwork: {
    title: '网络',
    description: '转发线路自动经过这个节点时用的网卡和中转端口。默认走系统默认路由；NAT 或多网卡机器在这里指定出口网卡、源地址和网关。NAT 机器还要在「入口」里加一个 NAT 入口，端口映射覆盖中转端口，其他节点就会连映射后的公网端口。',
    loading: '正在加载网络设置',
    loadFailed: '无法加载网络设置',
    interface: '出口网卡',
    interfaceHint: '如 eth0、ens18。留空走默认路由。',
    notDetected: '如 eth0、ens18。留空走默认路由。Agent 还没上报网卡（需 Agent 5.0.0-rc.20 及以上），先手动填写。',
    detectedAt: 'Agent 检测到的网卡（{time} 上报）。选默认路由则不指定。',
    defaultRoute: '默认路由',
    none: '不指定',
    manual: '手动填写',
    pick: '从检测到的网卡中选择',
    sourceV4: 'IPv4 源地址',
    gatewayV4: 'IPv4 网关',
    sourceV6: 'IPv6 源地址',
    gatewayV6: 'IPv6 网关',
    hopPort: '中转端口',
    hopPortHint: '其他节点经 ANX 连到这里的监听端口，留空为 24443。',
    save: '保存',
    saved: '已保存，经过这个节点的线路已重新规划。',
    saveFailed: '保存失败',
    invalid: '有字段需要修改'
  }
}
