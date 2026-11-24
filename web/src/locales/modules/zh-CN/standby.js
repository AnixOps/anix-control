// 节点区域的「过境应急」页：跨境链路不可达时，一键切到公开协议备用通道，再一键切回。
// 与 en/standby.js 键相同。
export default {
  standby: {
    nav: '过境应急',
    page: {
      title: '过境应急通道',
      description: '跨境链路（ANX-A）的两档备用通道：第一档 VLESS + REALITY + Vision，第二档 VLESS + XHTTP（自有域名 TLS 前置）。链路不可达时切换，恢复后切回。',
      prepare: '准备通道…',
      allReality: '全部切换至第一档',
      allXhttp: '全部切换至第二档',
      allOff: '全部切回',
      loadFailed: '无法加载过境链路',
      emptyTitle: '暂无跨境链路',
      emptyDescription: '在“网络”中添加 kind 为 anx 的非受信传输后显示。',
      note: '切换一分钟内生效：域名流量改经出口节点 VLESS 中继，IP 直连流量仍走原链路。演练步骤见 docs/guide/cross-border-standby.md。',
      stale: '无所属跨境链路的通道：{n} 条（下次“准备通道”时删除未启用者）',
      skipped: '部分路径两端非代理节点，未准备通道',
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
      toReality: '切换至第一档',
      toXhttp: '切换至第二档',
      toOff: '切回原链路'
    },
    sheet: {
      title: '准备备用通道',
      description: '为各跨境链路的节点对生成通道，凭据自动生成且不显示。仅准备，不切换。',
      serverName: '第一档借用站点（SNI）',
      serverNameHelp: '境外站点域名，例如 www.microsoft.com。留空则不准备第一档。',
      dest: '借用站点地址',
      destHelp: 'host:port，留空使用 SNI:443。',
      domains: '第二档自有域名',
      domainsHelp: '每行一条“出口节点 id=域名”，例如 50=edge.example.net。域名须解析至出口节点并配置 TLS 前置；无域名的节点不准备第二档。',
      save: '准备'
    },
    confirm: {
      title: '将 {what} 切换至 {tier}？',
      message: '一分钟内切换，现有连接将断开重连。',
      all: '全部跨境链路',
      ok: '切换'
    },
    toast: {
      prepared: '已准备：新建 {created}，更新 {updated}，删除 {removed}',
      switched: '{n} 条链路已切换至 {tier}'
    }
  }
}
