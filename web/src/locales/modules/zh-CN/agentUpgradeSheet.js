// agentUpgradeSheet：升级节点面板（5.0）。进度和暂停、中止的文案在 agentUpgrades。
export default {
  agentUpgradeSheet: {
    title: '升级节点',
    description: '将 Control 固定的 Agent 版本分批推送至在线节点。',
    loading: '正在加载升级信息…',
    loadFailed: '无法加载升级信息',
    started: '已开始升级到 {version}',
    kernelStarted: '已开始将内核升级至 anx-node {version}',
    start: {
      title: '开始升级',
      target: '目标版本',
      targetHelp: '默认为 Control 固定的 Agent 版本，须与 Control（{control}）主次版本号相同。',
      lineMismatch: '默认目标 {target} 与 Control（{control}）版本线不一致，请将同版本线的 Agent 发布放入 agent_install.artifact_dir。',
      nodeIncompatible: '版本不兼容',
      nodeWarning: '版本需关注',
      batches: '灰度批次（累计百分比）',
      batchesHelp: '逗号分隔。首批不超过 {first}%，末批为 100%。',
      minimum: '每批最短时间',
      minimumValue: '{minutes} 分钟',
      eligible: '升级范围',
      eligibleValue: '{n} 个节点',
      rules: '单批失败超过 {percent}% 或 {minutes} 分钟内未以新版本重连时，该批自动回滚。',
      reason: '备注（可选）',
      reasonPlaceholder: '例如：升级到 v4.2.0',
      nodes: '选择节点',
      nodeVersion: '当前 Agent {version}',
      nodeKernel: '内核 anx-node {version}',
      nodeKernelUnknown: '内核未上报',
      kernelTarget: '将安装 anx-node {version}（最新内核），Agent 版本不变，重启一次。',
      kernelMissing: 'Control 无 anx-node 内核，请在下方上传或复制至 agent_install.artifact_dir。',
      kernelSubmit: '升级内核',
      mode: '升级方式',
      modes: {
        staged: '分批灰度',
        manual: '手动选择',
        kernel: '只升级内核',
        stagedHelp: '按批次升级全部可升级节点。',
        manualHelp: '一次性升级所选节点。',
        kernelHelp: '仅升级所选节点的 anx-node 内核，所选节点须运行同一 Agent 版本。'
      },
      submit: '开始升级',
      noneEligible: '无可升级节点：节点须已启用且 Agent 曾连接 Control。',
      active: '已有升级在进行，结束或中止后方可开始新升级。',
      superOnly: '仅超级管理员可开始升级。',
      unconfigured: '无法升级：{reason}'
    },
    upload: {
      label: '手动安装内核：选择同一发布的 SHA256SUMS、SHA256SUMS.sig 和 anx-node 包',
      submit: '上传内核',
      missing: '请选择 SHA256SUMS、SHA256SUMS.sig 及至少一个 anx-node .tar.gz 包。',
      done: '已上传 anx-node {version}'
    },
    confirm: {
      title: '将 {n} 个节点升级至 {version}？',
      message: '分批推送：{batches}，每批至少 {minutes} 分钟。可随时暂停、中止或回滚。',
      kernelTitle: '将 {n} 个节点的内核升级至 anx-node {version}？',
      kernelMessage: '立即为以下节点安装内核：{nodes}。Agent 将重启一次，10 分钟内未重连视为失败。',
      manualMessage: '立即升级以下节点：{nodes}。失败或 10 分钟内未以新版本重连时自动回滚。'
    },
    errors: {
      target: '目标版本须为发布标签，例如 v4.2.0。',
      batchesFormat: '批次须为逗号分隔的整数，例如 5, 25, 100。',
      batchesCount: '最多 {max} 批。',
      batchesOrder: '百分比须逐批递增且不超过 100。',
      batchesFirst: '首批不超过 {max}%。',
      batchesLast: '末批须为 100%。',
      super_admin_required: '仅超级管理员可开始升级。',
      agent_release_unverified: '该 Agent 版本文件校验失败：{message}',
      agent_upgrade_active: '已有升级在进行，结束或中止后方可开始新升级。',
      agent_upgrade_no_nodes: '无可升级节点。',
      kernel_release_missing: 'Control 无可安装的 anx-node 内核。',
      kernel_release_unverified: 'anx-node 内核文件不完整：{message}',
      agent_install_unconfigured: '无法升级：{message}',
      kernel_release_invalid: '内核发布未通过发布公钥校验：{message}',
      kernel_release_conflict: '该 anx-node 版本已存在其他发布：{message}',
      invalid_request: '升级设置有误：{message}',
      unknown: '升级未开始。'
    }
  }
}
