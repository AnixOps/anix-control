// agentUpgradeSheet：升级节点面板（5.0）。进度和暂停、中止的文案在 agentUpgrades。
export default {
  agentUpgradeSheet: {
    title: '升级节点',
    description: 'Control 把它固定的 Agent 版本分批推送到在线节点，先升级一小批做灰度。',
    loading: '正在加载升级信息…',
    loadFailed: '无法加载升级信息',
    started: '已开始升级到 {version}',
    kernelStarted: '已开始把内核升级到 anx-node {version}',
    start: {
      title: '开始升级',
      target: '目标版本',
      targetHelp: '默认是 Control 安装脚本固定的 Agent 版本，不能比 Control（{control}）新。',
      batches: '灰度批次（累计百分比）',
      batchesHelp: '用逗号分隔。第一批最多 {first}%，最后一批是 100%。',
      minimum: '每批最短时间',
      minimumValue: '{minutes} 分钟',
      eligible: '会升级',
      eligibleValue: '{n} 个节点',
      rules: '一批里失败的节点超过 {percent}%，或者 {minutes} 分钟内没有以新版本重连，这一批会自动回滚。',
      reason: '备注（可选）',
      reasonPlaceholder: '例如：升级到 v4.2.0',
      nodes: '选择节点',
      nodeVersion: '当前 Agent {version}',
      nodeKernel: '内核 anx-node {version}',
      nodeKernelUnknown: '内核未上报',
      kernelTarget: '将安装 anx-node {version}（Control 上最新的内核）。Agent 版本不变，会重启一次。',
      kernelMissing: 'Control 上没有 anx-node 内核：先把内核发布复制到 agent_install.artifact_dir。',
      kernelSubmit: '升级内核',
      mode: '升级方式',
      modes: {
        staged: '分批灰度',
        manual: '手动选择',
        kernel: '只升级内核',
        stagedHelp: '按批次升级所有可升级的节点，先灰度一小批。',
        manualHelp: '只升级勾选的节点，一次全部推送，不等批次时间。',
        kernelHelp: '只升级勾选节点的 anx-node 内核，Agent 版本不变。勾选的节点要运行同一个 Agent 版本。'
      },
      submit: '开始升级',
      noneEligible: '没有可升级的节点：节点要启用，且它的 Agent 连上过 Control。',
      active: '已有一个升级在进行。结束或中止后才能开始新的。',
      superOnly: '只有超级管理员可以开始升级。',
      unconfigured: '现在还不能升级：{reason}'
    },
    confirm: {
      title: '把 {n} 个节点升级到 {version}？',
      message: '分批推送：{batches}，每批至少 {minutes} 分钟。可以随时暂停、中止或回滚。',
      kernelTitle: '把 {n} 个节点的内核升级到 anx-node {version}？',
      kernelMessage: '立即给这些节点安装内核：{nodes}。每个 Agent 会重启一次；失败或 10 分钟内没有重连算作失败。',
      manualMessage: '立即升级这些节点：{nodes}。失败或 10 分钟内没有以新版本重连会自动回滚。'
    },
    errors: {
      target: '目标版本要写成发布标签，例如 v4.2.0。',
      batchesFormat: '批次写成用逗号分隔的整数，例如 5, 25, 100。',
      batchesCount: '最多 {max} 批。',
      batchesOrder: '百分比要逐批变大，且不超过 100。',
      batchesFirst: '第一批最多 {max}%。',
      batchesLast: '最后一批必须是 100%。',
      super_admin_required: '只有超级管理员可以开始升级。',
      agent_release_unverified: '这个 Agent 版本的文件没有通过校验：{message}',
      agent_upgrade_active: '已有一个升级在进行。结束或中止后才能开始新的。',
      agent_upgrade_no_nodes: '没有可升级的节点。',
      kernel_release_missing: 'Control 上没有可安装的 anx-node 内核。',
      kernel_release_unverified: 'anx-node 内核文件不完整：{message}',
      agent_install_unconfigured: '现在还不能升级：{message}',
      invalid_request: '升级设置不正确：{message}',
      unknown: '没有开始升级。'
    }
  }
}
