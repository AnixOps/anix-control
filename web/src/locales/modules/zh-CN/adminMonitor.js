export default {
  adminMonitor: {
    title: '实时监控',
    subtitle: '通过 WebSocket 实时显示节点状态',
    connecting: '连接中...',
    connected: '已连接',
    disconnected: '已断开',
    reconnecting: '{seconds} 秒后重连...',
    overview: {
      title: '概览',
      totalNodes: '总节点',
      onlineNodes: '在线',
      offlineNodes: '离线',
      pendingNodes: '待激活',
      totalUpload: '总上传',
      totalDownload: '总下载'
    },
    nodeTable: {
      title: '节点列表',
      name: '名称',
      host: '地址',
      status: '状态',
      cpu: 'CPU',
      memory: '内存',
      disk: '磁盘',
      users: '用户',
      upload: '上传',
      download: '下载',
      uptime: '运行时间'
    },
    status: {
      online: '在线',
      offline: '离线',
      pending: '待激活'
    },
    empty: '暂无节点',
    errors: {
      connectFailed: '无法连接监控 WebSocket',
      unsupported: '当前浏览器不支持 WebSocket'
    }
  }
}
