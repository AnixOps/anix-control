// 线路 (src/views/admin/Lines.vue).
export default {
  lines: {
    title: '线路',
    description: '一条线路就是用户能连上的一条路：从入口进，经过中转，从出口上公网。',
    loadFailed: '线路加载失败',
    emptyTitle: '还没有线路',
    emptyDescription: '给设备加一个协议，或建一条转发路由，这里就会出现线路。',
    legend: '圆点颜色分别是入口、中转、出口；描边的设备是协议解开的地方（服务点）；虚线表示这一段由内核转发。',
    rules: '分流 {n}',
    filter: { label: '筛选', relay: '中转', direct: '直连', fixed: '固定目标', attention: '需要关注' },
    kind: { relay: '中转线路', direct: '直连线路', fixed: '固定目标' },
    columns: { line: '线路', path: '入口 → 出口', rate: '倍率', traffic: '近 24 小时流量' },
    skipped: {
      title: '{n} 项配置还读不成线路',
      description: '它们照常工作，只是暂时不在上面的列表里。'
    },
    detail: {
      path: '路径',
      served: '服务点',
      target: '固定目标',
      entryAndExit: '入口和出口',
      protocol: '协议',
      noProtocol: '只转发端口',
      port: '入口端口',
      hostname: '入口域名',
      subscription: '订阅',
      shown: '出现在订阅里',
      hidden: '不进订阅',
      rules: '分流规则',
      noRules: '全部流量从默认出口出。',
      via: '从 {name} 出',
      openRoute: '查看转发路由'
    }
  }
}
