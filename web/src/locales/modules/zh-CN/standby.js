// 节点区域的「过境应急」页：跨境链路被封时，一键切到公开协议备用通道，再一键切回。
// 与 en/standby.js 键相同。
export default {
  standby: {
    nav: '过境应急',
    page: {
      title: '过境应急通道',
      description: '每条跨境链路（ANX-A）都可以备好两档公开协议通道：第一档 VLESS + REALITY + Vision，第二档 VLESS + XHTTP（走自有域名的真实 TLS 前置）。链路被封时一键切过去，恢复后一键切回。',
      prepare: '准备通道…',
      allReality: '全部切到第一档',
      allXhttp: '全部切到第二档',
      allOff: '全部切回',
      loadFailed: '无法加载过境链路',
      emptyTitle: '网络模型里还没有跨境链路',
      emptyDescription: '在「网络」里加一条 kind 为 anx、非受信的传输后，这里会列出它。',
      note: '切换后两端节点的 Agent 在一分钟内生效：源节点把所有域名流量改走出口节点的 VLESS 中继；只用 IP 访问的流量仍走原链路。演练步骤见 docs/guide/cross-border-standby.md。',
      stale: '已不属于任何跨境链路的通道：{n} 条（下次「准备通道」时删除未启用的）',
      skipped: '有路径的两端不是代理节点，未备通道',
      columns: {
        transport: '链路',
        pairs: '节点对',
        active: '当前走',
        ready: '可切换',
        channels: '通道'
      },
      tier: {
        reality: '第一档 REALITY',
        xhttp: '第二档 XHTTP',
        off: '原链路'
      },
      none: '无',
      toReality: '切到第一档',
      toXhttp: '切到第二档',
      toOff: '切回原链路'
    },
    sheet: {
      title: '准备备用通道',
      description: '为每条跨境链路的每个节点对生成通道（uuid、REALITY 密钥、端口由 Control 生成，不显示）。只准备不切换，节点上不会有变化。',
      serverName: '第一档借用站点（SNI）',
      serverNameHelp: '一个国外大站的域名，例如 www.microsoft.com。留空则不准备第一档。',
      dest: '借用站点地址',
      destHelp: 'host:port，留空用 SNI:443。',
      domains: '第二档自有域名',
      domainsHelp: '每行一个「出口节点 id=域名」，例如 50=edge.example.net。域名需解析到出口节点，并在节点上配好 TLS 前置。没有域名的节点不准备第二档。',
      save: '准备'
    },
    confirm: {
      title: '把 {what} 切到 {tier}？',
      message: '两端节点会在一分钟内切换，正在进行的连接会断开重连。',
      all: '全部跨境链路',
      ok: '切换'
    },
    toast: {
      prepared: '已准备：新建 {created}，更新 {updated}，删除 {removed}',
      switched: '{n} 条链路已切到 {tier}'
    }
  }
}
