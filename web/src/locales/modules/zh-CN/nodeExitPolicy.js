// 节点页协议标签下的出口策略：节点作为出口时是否拒绝中国大陆目标。
// 与 en/nodeExitPolicy.js 键相同。
export default {
  nodeExitPolicy: {
    title: '出口策略',
    description: '客户端经这个节点访问互联网时，节点拒绝哪些目标。',
    loading: '正在加载出口策略',
    loadFailed: '无法加载出口策略',
    blockMainland: '拒绝中国大陆目标',
    blockMainlandHelp: '默认关闭。开启后节点拒绝所有访问中国大陆地址的连接，误发到这里的大陆流量会失败，而不是从境外绕出。需要 5.0.2 及以上的 Agent 和内核，旧版本会忽略。',
    blocked: '已拒绝 {n} 次连接，最近一次在 {date}。',
    none: '还没有上报被拒绝的连接。',
    saveFailed: '无法保存出口策略'
  }
}
