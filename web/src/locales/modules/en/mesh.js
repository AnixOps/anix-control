// The node area's Mesh page: regions, the nodes' fixed addresses and
// boundary nodes, and the mesh status each node reports. Same keys as
// zh-CN/mesh.js.
export default {
  mesh: {
    nav: 'Mesh',
    page: {
      title: 'Mesh',
      description: 'The intranet is divided into regions, each a network of its own. Nodes have fixed addresses; boundary nodes connect the regions.',
      unavailable: 'This version does not provide the mesh.',
      loadFailed: 'Could not load the mesh configuration',
      addRegion: 'New region',
      addMember: 'Add node',
      issueEnrollment: 'Issue enrollment credential',
      devices: 'Devices',
      emptyDevices: 'No enrolled devices',
      emptyDevicesDescription: 'Devices without an Agent join the mesh with a one-time enrollment credential.',
      regionChanges: 'Pending region changes',
      enrollments: 'Enrollment credentials',
      noAddress: 'No address',
      moveTo: 'Move to {name}',
      revoke: 'Revoke…',
      approve: 'Approve',
      reject: 'Reject',
      withdraw: 'Withdraw',
      regions: 'Regions',
      members: 'Nodes',
      emptyRegions: 'No regions',
      emptyMembers: 'No mesh nodes',
      emptyMembersDescription: 'An added node gets a fixed address from the server segment of its region.',
      disabled: 'Disabled',
      enabled: 'Enabled',
      fewBoundaries: 'Fewer than two boundary nodes',
      memberCount: '{n} node | {n} nodes',
      boundaryCount: '{n} boundary node | {n} boundary nodes',
      note: 'Region networks always use forward secrecy. The nodes of a disabled region stop its network.',
      edit: 'Edit…',
      enable: 'Enable',
      disable: 'Disable',
      delete: 'Delete…',
      move: 'Move to region…',
      setBoundary: 'Make boundary node',
      unsetBoundary: 'Remove boundary role',
      remove: 'Remove from mesh…',
      boundary: 'Boundary node',
      member: 'Node',
      columns: {
        region: 'Region',
        cidr: 'Range',
        members: 'Nodes',
        state: 'State',
        node: 'Node',
        address: 'Address',
        role: 'Role',
        status: 'Mesh status',
        device: 'Device',
        move: 'Change',
        reason: 'Reason',
        name: 'Name'
      },
      status: {
        none: 'Not reported',
        running: 'Running',
        stopped: 'Not running',
        lost: 'Control link lost'
      },
      alerts: {
        address_conflict: 'Address conflict',
        address_unmanaged: 'Unmanaged address',
        unsupported: 'Kernel without mesh',
        start_refused: 'Start refused',
        takeover_pending: 'Waiting to take over the existing mesh program'
      }
    },
    region: {
      createTitle: 'New region',
      editTitle: 'Edit {name}',
      description: 'The first half of the range is the server segment, the second half the device pool. Region ranges must not overlap.',
      code: 'Code',
      codeHelp: '2 to 16 lowercase letters, digits or dashes, starting with a letter.',
      name: 'Name',
      cidr: 'Range',
      cidrHelp: 'A private IPv4 range with a prefix length of 8 to 24. The range of a region with nodes cannot change.',
      sort: 'Order',
      enabled: 'Enabled',
      create: 'Create',
      save: 'Save'
    },
    member: {
      addTitle: 'Add node',
      moveTitle: 'Move {name}',
      description: 'The node gets the lowest free address of its region\'s server segment. Moving changes the node\'s address.',
      node: 'Node',
      region: 'Region',
      boundary: 'Boundary node',
      boundaryHelp: 'A boundary node joins every region and advertises the other regions\' ranges in each. Each region needs at least two.',
      add: 'Add',
      save: 'Save'
    },
    device: {
      kinds: {
        node: 'Node device',
        personal: 'Personal device'
      }
    },
    enrollment: {
      title: 'Issue enrollment credential',
      description: 'A credential can be used once. The device exchanges it for a device token, then reads its mesh configuration and reports its status periodically.',
      name: 'Device name',
      kind: 'Type',
      kindHelp: {
        node: 'A node device has a fixed address from the server segment and runs as a full node.',
        personal: 'A personal device has an address from the device pool and runs in access mode; the address is released after 30 days without use.'
      },
      region: 'Region',
      anyRegion: 'Chosen by latency',
      hours: 'Validity (hours)',
      hoursHelp: '1 to 168 hours.',
      issue: 'Issue',
      token: 'Enrollment credential',
      tokenHelp: 'The credential is shown only once. Copy it now and give it to the device user.',
      done: 'Done',
      states: {
        valid: 'Unused',
        used: 'Used',
        expired: 'Expired'
      }
    },
    delete: {
      regionTitle: 'Delete region {name}',
      regionMessage: 'Boundary nodes leave the region. A region that is still a node\'s home region cannot be deleted.',
      memberTitle: 'Remove {name} from the mesh',
      memberMessage: 'The node stops every mesh network and releases its addresses.',
      confirm: 'Delete',
      confirmRemove: 'Remove',
      deviceTitle: 'Revoke device {name}',
      deviceMessage: 'The device token stops working at once and its address is released. Enrolling again needs a new credential.',
      confirmRevoke: 'Revoke'
    },
    toast: {
      saved: 'Saved',
      deleted: 'Deleted',
      removed: 'Removed',
      revoked: 'Revoked'
    },
    required: 'Required'
  }
}
