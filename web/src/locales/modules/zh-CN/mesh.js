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
      regions: '地区',
      members: '节点',
      emptyRegions: '暂无地区',
      emptyMembers: '暂无组网节点',
      emptyMembersDescription: '添加节点后，节点获得所在地区服务器段的固定地址。',
      disabled: '未启用',
      enabled: '已启用',
      fewBoundaries: '边界节点少于两台',
      sharedAddressSpace: '位于共享地址段',
      resolverAddress: '解析地址 {address}',
      resolverAddressHeld: '解析地址被占用',
      memberCount: '{n} 个节点',
      boundaryCount: '{n} 台边界节点',
      note: '地区网络强制启用前向保密。地区停用后，其节点停止该网络。',
      edit: '编辑…',
      enable: '启用',
      disable: '停用',
      rotateSecret: '更换网络密钥…',
      delete: '删除…',
      move: '迁移地区…',
      setBoundary: '设为边界节点',
      unsetBoundary: '取消边界节点',
      remove: '移出组网…',
      boundary: '边界节点',
      externalBridge: '外部网络桥接',
      member: '节点',
      columns: {
        region: '地区',
        cidr: '网段',
        members: '节点',
        state: '状态',
        node: '节点',
        address: '地址',
        role: '角色',
        status: '组网状态'
      },
      status: {
        none: '未上报',
        running: '运行中',
        pending: '暂缓',
        stopped: '未运行',
        lost: '控制面失联'
      },
      alerts: {
        address_conflict: '地址冲突',
        address_unmanaged: '地址未受管',
        unsupported: '内核不支持组网',
        start_refused: '拒绝启动',
        takeover_pending: '等待接管现有组网程序',
        peer_not_native: '拒绝未准入节点',
        native_keys_pending: '等待准入清单',
        native_keys_unsupported: '内核不支持准入清单',
        invalid_kind: '网络类型无效',
        static_key_taken: '公钥与其他成员重复',
        external_bridge_unsupported: '内核不支持外部网络桥接',
        external_bridge_apply_failed: '外部网络桥接未能生效',
        external_bridge_ipv6_forwarding_off: '未开启 IPv6 转发',
        external_bridge_downgrade_residue: '外部网络桥接规则可能残留，请清理后删除记录文件',
        external_bridge_lock_held: '本机已有其他进程占用外部网络桥接，未启用桥接能力'
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
      cidrShared: '该网段位于 100.64.0.0/10，可能被同一主机上的其他组网软件整段接管，建议改用其他私有网段。',
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
    delete: {
      regionTitle: '删除地区 {name}',
      regionMessage: '边界节点将退出该地区。仍为节点所属地区时无法删除。',
      memberTitle: '将 {name} 移出组网',
      memberMessage: '节点将停止全部组网网络并释放地址。',
      confirm: '删除',
      confirmRemove: '移出'
    },
    rotate: {
      title: '更换 {name} 的网络密钥',
      message: '新密钥立即推送到该地区全部节点，设备在下次读取配置时获得。已吊销或已删除的设备将无法再以原密钥入网。更换期间该地区的连接会短暂中断。',
      confirm: '更换'
    },
    toast: {
      saved: '已保存',
      rotated: '已更换网络密钥',
      deleted: '已删除',
      removed: '已移出'
    },
    required: '必填'
  }
}
