// 节点页协议标签下的临时 SOCKS5 代理：监听内网地址，到期自动关闭。
// 与 en/nodeTemporaryProxies.js 键相同。
export default {
  nodeTemporaryProxies: {
    title: '临时 SOCKS5 代理',
    description: '在节点内网地址上开启 SOCKS5 代理，到期自动关闭。需支持临时代理的 Agent 和内核。',
    loading: '正在加载临时代理',
    loadFailed: '无法加载临时代理',
    listen: '监听地址',
    listenHelp: '节点的内网地址，如组网地址或局域网地址。不接受公网地址和通配地址。',
    port: '端口',
    minutes: '时长',
    minutesUnit: '分钟',
    open: '开启',
    openFailed: '无法开启临时代理',
    passwordOnce: '密码仅显示一次。',
    address: '地址',
    username: '用户名',
    password: '密码',
    none: '暂无开启中的临时代理。',
    row: '用户 {user}，{date} 关闭',
    close: '关闭',
    closeFailed: '无法关闭临时代理'
  }
}
