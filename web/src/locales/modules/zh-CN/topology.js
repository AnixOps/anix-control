// 网络拓扑 (src/views/admin/Topology.vue).
export default {
  topology: {
    title: '网络拓扑',
    description: '所有设备和它们之间的线路。点一台设备或一条线路，看它经过哪里。',
    loadFailed: '拓扑加载失败',
    emptyTitle: '还没有设备',
    emptyDescription: '接入第一台设备后，这里会画出它的位置和线路。',
    openNodes: '去接入设备',
    close: '关闭',
    unplaced: '{n} 台设备认不出位置，在名称或标签里写上地区（如 JP、东京）即可上图：',
    view: { label: '视图', map: '全球地图', graph: '抽象拓扑' },
    map: { zoomIn: '放大', zoomOut: '缩小', fit: '聚焦到设备', world: '看全球' },
    roles: { entry: '入口', relay: '中转', exit: '出口', direct: '直连' },
    status: { online: '在线', offline: '离线', pending: '待接入', disabled: '已停用' },
    tone: { ok: '正常', warn: '需要关注', bad: '中断', off: '已暂停' },
    legend: { label: '图例', line: '线路', egress: '分流' },
    metrics: {
      devices: '在线设备',
      devicesDetail: '在线 / 全部',
      lines: '线路',
      linesDetail: '{n} 条需要关注',
      places: '覆盖地区',
      placesDetail: '{n} 台未定位',
      users: '在线用户',
      usersDetail: '各设备上报之和'
    },
    lines: { title: '线路', empty: '还没有中转线路。设备目前都是直连。' },
    line: { kind: '类型', route: '中转线路', egress: '分流', traffic: '近 24 小时流量', open: '查看线路' },
    device: {
      place: '位置',
      noPlace: '未识别',
      role: '角色',
      host: '地址',
      usersLabel: '在线用户',
      users: '{n} 人在线',
      lines: '经过的线路（{n}）',
      noLines: '没有线路经过，用户直连这台设备。',
      open: '打开设备页'
    }
  }
}
