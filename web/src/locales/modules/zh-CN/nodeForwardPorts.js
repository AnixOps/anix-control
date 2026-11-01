// 节点页「协议」标签下的「转发端口」：转发规划器在这个节点上分配端口的范围和
// 保留端口。与 en/nodeForwardPorts.js 键相同。
export default {
  nodeForwardPorts: {
    title: '转发端口',
    description: '转发线路在这个节点上监听的端口从这个范围里分配，保留端口不会被占用。修改后所有经过这个节点的线路重新规划。',
    loading: '正在加载转发端口设置',
    loadFailed: '无法加载转发端口设置',
    first: '端口范围起',
    last: '端口范围止',
    sourceOwn: '这是这个节点自己设置的范围。',
    sourceBuiltIn: '这个节点还没有设置范围，现在使用内置的 {first}–{last}。改成需要的范围后保存，就会固定为这个节点的设置。',
    sourceNone: '这个节点还没有设置范围。填写起止端口后保存。',
    bothEnds: '起止端口都要填写',
    reversed: '起始端口不能大于结束端口',
    reserved: '保留端口',
    reservedHelp: '逗号或空格分隔。SSH（22）和节点自己的协议端口总是保留。',
    reservedInvalid: '保留端口必须是 1–65535 的整数',
    save: '保存',
    saved: '已保存，经过这个节点的线路已重新规划。'
  }
}
