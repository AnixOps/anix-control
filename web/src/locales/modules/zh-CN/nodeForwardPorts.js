// 节点页「协议」标签下的「转发端口」：转发规划器在这个节点上分配端口的范围和
// 保留端口。与 en/nodeForwardPorts.js 键相同。
export default {
  nodeForwardPorts: {
    title: '转发端口',
    description: '转发线路在该节点的监听端口从此范围分配，不占用保留端口。修改后重新规划经过该节点的线路。',
    loading: '正在加载转发端口设置',
    loadFailed: '无法加载转发端口设置',
    first: '端口范围起',
    last: '端口范围止',
    sourceOwn: '该节点自定义的范围。',
    sourceBuiltIn: '未设置范围，当前使用内置范围 {first}–{last}。保存后固定为该节点设置。',
    sourceNone: '未设置范围，请填写起止端口。',
    bothEnds: '请填写起止端口',
    reversed: '起始端口不能大于结束端口',
    reserved: '保留端口',
    reservedHelp: '逗号或空格分隔。SSH（22）及节点协议端口始终保留。',
    reservedInvalid: '保留端口必须是 1–65535 的整数',
    save: '保存',
    saved: '已保存，经过该节点的线路已重新规划。'
  }
}
