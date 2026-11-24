// nodeDiagnostics：节点页的诊断分区（5.0，设计 D7）。
export default {
  nodeDiagnostics: {
    title: '诊断',
    description: '在节点 Agent 上运行检查。',
    refresh: '刷新',
    loading: '正在加载诊断…',
    loadFailed: '无法加载诊断',
    online: 'Agent 在线',
    notConnected: 'Agent 不在线',
    agentVersion: 'Agent {version}',
    supports: '支持：{list}',
    separator: '、',
    offline: {
      title: 'Agent 不在线',
      description: 'Agent 连接 Control 后方可诊断。'
    },
    unsupported: {
      title: '该 Agent 不支持诊断',
      description: '请升级 Agent。',
      descriptionVersion: 'Agent {version} 不支持诊断，请升级。'
    },
    service: {
      title: '服务 {service}',
      label: '服务'
    },
    actions: {
      service_status: '查看服务状态',
      service_restart: '重启服务',
      log_tail: '查看日志',
      forward_listen: '检查监听',
      forward_port_conflict: '检查端口冲突',
      forward_connect: '检查 TCP 连接',
      forward_udp_probe: '探测 UDP'
    },
    lines: {
      label: '日志行数',
      option: '最近 {count} 行'
    },
    restartSuperOnly: '仅超级管理员可重启服务。',
    restart: {
      title: '重启 {name} 上的 {service}？',
      message: '{service} 将短暂中断，现有连接将断开。',
      confirm: '重启服务'
    },
    forward: {
      title: '转发检查',
      hop: '要检查的跳',
      noHops: '该节点未承载转发路由。',
      unsupported: '该 Agent 不支持转发检查，请升级。'
    },
    running: '正在运行“{action}”，最多等待 {seconds} 秒…',
    result: {
      title: '结果',
      success: '成功',
      failed: '失败',
      duration: '耗时 {ms} ms',
      output: '输出',
      noOutput: '（无输出）',
      rtt: '{ms} ms'
    },
    checkStates: {
      ok: '正常',
      failed: '失败',
      inconclusive: '无法判断',
      skipped: '已跳过'
    },
    history: {
      title: '最近的诊断',
      show: '查看',
      pending: '进行中',
      none: '转发节点不保存诊断历史，运行记录见操作日志。'
    },
    errors: {
      node_offline: '节点 Agent 不在线。',
      diagnostics_unsupported: '该 Agent 不支持此诊断，请升级。',
      node_pending_approval: '节点待批准，批准后方可诊断。',
      diagnostic_busy: '该节点正在运行其他诊断，请稍后重试。',
      diagnostic_timeout: '节点 {seconds} 秒内未返回结果。',
      agent_error: 'Agent 拒绝诊断：{message}',
      super_admin_required: '仅超级管理员可重启服务。',
      action_not_allowed: '不允许此诊断。',
      invalid_params: '诊断参数不正确：{message}',
      node_not_found: '节点不存在。',
      unknown: '诊断未完成。'
    }
  }
}
