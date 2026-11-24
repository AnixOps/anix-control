// 节点页协议标签下的出口策略：节点作为出口时是否拒绝中国大陆目标。
// 与 en/nodeExitPolicy.js 键相同。
export default {
  nodeExitPolicy: {
    title: '出口策略',
    description: '经该节点访问时拒绝的目标。',
    loading: '正在加载出口策略',
    loadFailed: '无法加载出口策略',
    blockMainland: '拒绝中国大陆目标',
    blockMainlandHelp: '默认关闭。开启后拒绝访问中国大陆地址的连接。需 Agent 和内核 5.0.2 及以上。',
    blocked: '已拒绝 {n} 次连接，最近一次在 {date}。',
    none: '暂无被拒绝的连接。',
    saveFailed: '无法保存出口策略'
  }
}
