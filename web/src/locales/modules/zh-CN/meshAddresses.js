// 「组网」页的设备、宿主组、内网域名、地区变更与设置分区。与 en/meshAddresses.js 键相同。
export default {
  meshAddr: {
    sections: {
      label: '组网分区',
      regions: '地区与节点',
      devices: '设备',
      hostGroups: '宿主组',
      names: '内网域名',
      changes: '地区变更',
      changesPending: '地区变更（{n}）',
      settings: '设置'
    },
    common: {
      cancel: '取消',
      save: '保存',
      delete: '删除…',
      deleteConfirm: '删除',
      required: '必填',
      saved: '已保存',
      deleted: '已删除',
      none: '无',
      region: '地区',
      address: '地址',
      name: '名称'
    },
    devices: {
      title: '设备',
      description: '手机与个人电脑从所在地区的设备地址池获得地址，地址按设备保持不变，连续 30 天未使用才可回收。',
      add: '添加设备',
      empty: '暂无设备',
      loadFailed: '无法加载设备',
      hostname: '主机名',
      hostnameHelp: '一个 DNS 标签：字母、数字与中间的连字符。',
      deviceKey: '设备密钥',
      deviceKeyHelp: '设备的身份公钥，可留空。',
      lastUsed: '最近使用',
      pool: '设备地址池',
      poolUsage: '已用 {used} / {size}',
      poolLow: '地址池余量不足',
      assign: '分配新地址',
      assigned: '已分配新地址',
      move: '迁移地区…',
      moveTitle: '迁移 {name}',
      moveDescription: '设备将获得目标地区设备地址池中的新地址。',
      addTitle: '添加设备',
      editTitle: '编辑 {name}',
      edit: '编辑…',
      deleteTitle: '删除设备 {name}',
      deleteMessage: '设备地址随即释放。',
      warnings: {
        no_address: '地址已回收',
        address_idle: '30 天未使用'
      }
    },
    hostGroups: {
      title: '宿主组',
      description: '宿主组为网关节点之后的虚拟机或容器，使用宿主组地址段，不占用地区网段。',
      add: '新建宿主组',
      empty: '暂无宿主组',
      loadFailed: '无法加载宿主组',
      segment: '地址段',
      segmentHelp: '留空则自动分配；创建后不可更改。',
      gateway: '网关节点',
      standby: '备用网关',
      standbyHelp: '与网关节点同一地区，可留空。',
      addTitle: '新建宿主组',
      editTitle: '编辑 {name}',
      edit: '编辑…',
      deleteTitle: '删除宿主组 {name}',
      deleteMessage: '该宿主组的设备命名一并删除。'
    },
    deviceNames: {
      title: '设备命名',
      description: '网关节点为其后的主机发布名称：宿主组中的主机，或网关所在局域网中的设备。',
      add: '添加命名',
      empty: '暂无设备命名',
      hostGroup: '宿主组',
      localNetwork: '网关所在局域网',
      kind: '位置',
      addTitle: '添加设备命名',
      editTitle: '编辑 {name}',
      edit: '编辑…',
      addressHelp: '宿主组中的主机使用该组地址段内的地址；局域网设备使用地区网段与宿主组地址段之外的私有地址。',
      deleteTitle: '删除命名 {name}',
      deleteMessage: '该名称不再发布。'
    },
    names: {
      title: '内网域名',
      description: '名称格式为 名称.地区代码.{suffix}，与所在地区一致。重名的条目全部列出并标记。',
      empty: '暂无内网域名',
      loadFailed: '无法加载内网域名',
      fqdn: '域名',
      source: '来源',
      duplicate: '重名',
      unnamed: '以下节点的名称不是有效的 DNS 标签，未生成域名：{names}',
      sources: {
        node: '节点',
        device: '设备',
        host_group: '宿主组',
        local: '局域网设备'
      }
    },
    changes: {
      title: '地区变更',
      description: '地区变更会改变成员的虚拟地址，须经管理员批准。',
      empty: '暂无地区变更',
      loadFailed: '无法加载地区变更',
      member: '成员',
      move: '变更',
      reason: '原因',
      status: '状态',
      approve: '批准…',
      reject: '拒绝…',
      approveTitle: '批准地区变更',
      rejectTitle: '拒绝地区变更',
      approveDescription: '成员将迁入目标地区并获得新地址。',
      note: '备注',
      confirmApprove: '批准',
      confirmReject: '拒绝',
      approved: '已批准',
      rejected: '已拒绝',
      device: '设备 {id}',
      sources: {
        member: '成员申请',
        admin: '管理员'
      },
      statuses: {
        all: '全部',
        pending: '待处理',
        approved: '已批准',
        rejected: '已拒绝'
      }
    },
    settings: {
      title: '设置',
      nameSuffix: '域名后缀',
      nameSuffixHelp: '名称格式为 名称.地区代码.后缀。后缀须位于 internal 或 home.arpa 之下。',
      hostGroupSupernet: '宿主组地址范围',
      hostGroupPrefix: '宿主组地址段前缀长度',
      hostGroupHelp: '存在宿主组时不可更改。',
      reservedRanges: '保留网段',
      reservedRangesHelp: '每行一个网段，地区与宿主组均不得与其重叠。',
      loadFailed: '无法加载组网设置'
    }
  }
}
