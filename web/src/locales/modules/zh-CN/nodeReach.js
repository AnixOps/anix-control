// 节点可达性：各地区探测视角看到的节点是否可达。与 en/nodeReach.js 键相同。
export default {
  nodeReach: {
    column: '可达性',
    title: '可达性探测',
    description: '探测视角节点每 10 分钟从所在地区连一次其他节点已启用协议的端口（只建立连接，不发数据），对比境内与境外的结果，区分节点故障和仅部分地区无法访问。',
    loading: '正在加载可达性',
    loadFailed: '无法加载可达性',
    saveFailed: '无法保存探测视角',
    noAnswer: '还没有探测结果，或结果已超过 30 分钟。',
    role: '探测视角',
    roles: { none: '不参与', domestic: '境内视角', overseas: '境外视角' },
    roleHint: {
      none: '这个节点不探测其他节点，只被探测。',
      active: '这个节点的 Agent 每 10 分钟探测一次其他节点。',
      waiting: '已设为探测视角，等这个节点的 Agent 连上并升级到支持可达性探测的版本后开始。'
    },
    vantage: { domestic: '境内视角', overseas: '境外视角' },
    region: { domestic: '境内', overseas: '境外' },
    view: { yes: '{region}可达', no: '{region}不可达', none: '{region}无结果' },
    status: {
      ok: '正常',
      region_unreachable: '境内不可达',
      overseas_unreachable: '境外不可达',
      unreachable: '全部不可达',
      unknown: '无结果'
    }
  }
}
