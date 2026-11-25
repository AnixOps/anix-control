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
      regions: 'Regions',
      members: 'Nodes',
      emptyRegions: 'No regions',
      emptyMembers: 'No mesh nodes',
      emptyMembersDescription: 'An added node gets a fixed address from the server segment of its region.',
      disabled: 'Disabled',
      enabled: 'Enabled',
      fewBoundaries: 'Fewer than two boundary nodes',
      sharedAddressSpace: 'In shared address space',
      resolverAddress: 'Resolver {address}',
      resolverAddressHeld: 'Resolver address held',
      memberCount: '{n} node | {n} nodes',
      boundaryCount: '{n} boundary node | {n} boundary nodes',
      note: 'Region networks always use forward secrecy. The nodes of a disabled region stop its network.',
      edit: 'Edit…',
      enable: 'Enable',
      disable: 'Disable',
      rotateSecret: 'Change network key…',
      delete: 'Delete…',
      move: 'Move to region…',
      setBoundary: 'Make boundary node',
      unsetBoundary: 'Remove boundary role',
      remove: 'Remove from mesh…',
      boundary: 'Boundary node',
      externalBridge: 'External bridge',
      member: 'Node',
      columns: {
        region: 'Region',
        cidr: 'Range',
        members: 'Nodes',
        state: 'State',
        node: 'Node',
        address: 'Address',
        role: 'Role',
        status: 'Mesh status'
      },
      status: {
        none: 'Not reported',
        running: 'Running',
        pending: 'On hold',
        stopped: 'Not running',
        lost: 'Control link lost'
      },
      alerts: {
        address_conflict: 'Address conflict',
        address_unmanaged: 'Unmanaged address',
        unsupported: 'Kernel without mesh',
        start_refused: 'Start refused',
        takeover_pending: 'Waiting to take over the existing mesh program',
        peer_not_native: 'Refused a peer not admitted',
        native_keys_pending: 'Waiting for the admission list',
        native_keys_unsupported: 'Kernel without admission list',
        invalid_kind: 'Invalid network kind',
        static_key_taken: 'Static key used by another member'
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
      cidrShared: 'This range lies in 100.64.0.0/10, which other overlay software on the same host may take over entirely. Use another private range.',
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
    delete: {
      regionTitle: 'Delete region {name}',
      regionMessage: 'Boundary nodes leave the region. A region that is still a node\'s home region cannot be deleted.',
      memberTitle: 'Remove {name} from the mesh',
      memberMessage: 'The node stops every mesh network and releases its addresses.',
      confirm: 'Delete',
      confirmRemove: 'Remove'
    },
    rotate: {
      title: 'Change the network key of {name}',
      message: 'The new key goes to every node of the region at once and to devices with their next configuration read. Revoked or deleted devices can no longer join with the old key. Connections in the region are interrupted briefly.',
      confirm: 'Change'
    },
    toast: {
      rotated: 'Network key changed',
      saved: 'Saved',
      deleted: 'Deleted',
      removed: 'Removed'
    },
    required: 'Required'
  }
}
