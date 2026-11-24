// 节点可达性：各地区探测视角看到的节点是否可达。与 en/nodeReach.js 键相同。
export default {
  nodeReach: {
    column: '可达性',
    title: '可达性探测',
    description: '探测视角节点每 10 分钟连接其他节点的已启用协议端口（仅建立连接），对比境内与境外结果，区分节点故障与区域不可达。',
    loading: '正在加载可达性',
    loadFailed: '无法加载可达性',
    saveFailed: '无法保存探测视角',
    noAnswer: '暂无探测结果，或结果已超过 30 分钟。',
    role: '探测视角',
    roles: { none: '不参与', domestic: '境内视角', overseas: '境外视角' },
    roleHint: {
      none: '该节点不探测其他节点，仅被探测。',
      active: '该节点 Agent 每 10 分钟探测一次其他节点。',
      waiting: '已设为探测视角，待 Agent 连接并升级至支持版本后开始。'
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
