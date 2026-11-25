// 节点区域的「组网」页：地区、节点固定地址与边界节点，以及各节点上报的组网状态。
// 与 en/mesh.js 键相同。
export default {
  mesh: {
    nav: '组网',
    page: {
      title: '组网',
      description: '按地区划分内网，每个地区为独立网络。节点使用固定地址，边界节点连通各地区。',
      unavailable: '当前版本未提供组网功能。',
      loadFailed: '无法加载组网配置',
      addRegion: '新建地区',
      addMember: '添加节点',
      issueEnrollment: '签发接入凭证',
      devices: '设备',
      emptyDevices: '暂无接入设备',
      emptyDevicesDescription: '未安装 Agent 的设备凭一次性接入凭证加入组网。',
      regionChanges: '待审批的地区变更',
      enrollments: '接入凭证',
      noAddress: '无地址',
      moveTo: '迁移至 {name}',
      revoke: '吊销…',
      approve: '批准',
      reject: '拒绝',
      withdraw: '撤回',
      regions: '地区',
      members: '节点',
      emptyRegions: '暂无地区',
      emptyMembers: '暂无组网节点',
      emptyMembersDescription: '添加节点后，节点获得所在地区服务器段的固定地址。',
      disabled: '未启用',
      enabled: '已启用',
      fewBoundaries: '边界节点少于两台',
      memberCount: '{n} 个节点',
      boundaryCount: '{n} 台边界节点',
      note: '地区网络强制启用前向保密。地区停用后，其节点停止该网络。',
      edit: '编辑…',
      enable: '启用',
      disable: '停用',
      delete: '删除…',
      move: '迁移地区…',
      setBoundary: '设为边界节点',
      unsetBoundary: '取消边界节点',
      remove: '移出组网…',
      boundary: '边界节点',
      member: '节点',
      columns: {
        region: '地区',
        cidr: '网段',
        members: '节点',
        state: '状态',
        node: '节点',
        address: '地址',
        role: '角色',
        status: '组网状态',
        device: '设备',
        move: '变更',
        reason: '原因',
        name: '名称'
      },
      status: {
        none: '未上报',
        running: '运行中',
        stopped: '未运行',
        lost: '控制面失联'
      },
      alerts: {
        address_conflict: '地址冲突',
        address_unmanaged: '地址未受管',
        unsupported: '内核不支持组网',
        start_refused: '拒绝启动',
        takeover_pending: '等待接管现有组网程序'
      }
    },
    region: {
      createTitle: '新建地区',
      editTitle: '编辑 {name}',
      description: '网段前半为服务器段，后半为设备地址池。各地区网段不得重叠。',
      code: '代码',
      codeHelp: '2 至 16 位小写字母、数字或连字符，以字母开头。',
      name: '名称',
      cidr: '网段',
      cidrHelp: '私有 IPv4 网段，前缀长度 8 至 24。已有节点的地区不可更改网段。',
      sort: '排序',
      enabled: '启用',
      create: '创建',
      save: '保存'
    },
    member: {
      addTitle: '添加节点',
      moveTitle: '迁移 {name}',
      description: '节点获得所在地区服务器段的最小空闲地址。迁移会改变节点地址。',
      node: '节点',
      region: '地区',
      boundary: '边界节点',
      boundaryHelp: '边界节点加入全部地区，并向各地区通告其他地区网段。每个地区至少两台。',
      add: '添加',
      save: '保存'
    },
    device: {
      kinds: {
        node: '节点设备',
        personal: '个人设备'
      }
    },
    enrollment: {
      title: '签发接入凭证',
      description: '凭证仅可使用一次。设备凭证换取设备令牌后，定期读取组网配置并上报状态。',
      name: '设备名称',
      kind: '类型',
      kindHelp: {
        node: '节点设备使用服务器段固定地址，作为完整节点运行。',
        personal: '个人设备使用设备地址池地址，以接入方式运行；连续 30 天未使用时释放地址。'
      },
      region: '地区',
      anyRegion: '按延迟自动选择',
      hours: '有效期（小时）',
      hoursHelp: '1 至 168 小时。',
      issue: '签发',
      token: '接入凭证',
      tokenHelp: '凭证仅显示一次，请立即复制并交给设备使用者。',
      done: '完成',
      states: {
        valid: '未使用',
        used: '已使用',
        expired: '已过期'
      }
    },
    delete: {
      regionTitle: '删除地区 {name}',
      regionMessage: '边界节点将退出该地区。仍为节点所属地区时无法删除。',
      memberTitle: '将 {name} 移出组网',
      memberMessage: '节点将停止全部组网网络并释放地址。',
      confirm: '删除',
      confirmRemove: '移出',
      deviceTitle: '吊销设备 {name}',
      deviceMessage: '设备令牌立即失效，地址随之释放。重新接入需签发新凭证。',
      confirmRevoke: '吊销'
    },
    toast: {
      saved: '已保存',
      deleted: '已删除',
      removed: '已移出',
      revoked: '已吊销'
    },
    required: '必填'
  }
}
