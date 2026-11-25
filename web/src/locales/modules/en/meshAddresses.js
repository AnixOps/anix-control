// The devices, host groups, intranet names, region changes and settings
// sections of the 组网 page. Same keys as zh-CN/meshAddresses.js.
export default {
  meshAddr: {
    sections: {
      label: 'Mesh sections',
      regions: 'Regions and nodes',
      devices: 'Devices',
      hostGroups: 'Host groups',
      names: 'Intranet names',
      changes: 'Region changes',
      changesPending: 'Region changes ({n})',
      settings: 'Settings'
    },
    common: {
      cancel: 'Cancel',
      save: 'Save',
      delete: 'Delete…',
      deleteConfirm: 'Delete',
      required: 'Required',
      saved: 'Saved',
      deleted: 'Deleted',
      none: 'None',
      region: 'Region',
      address: 'Address',
      name: 'Name'
    },
    devices: {
      title: 'Devices',
      description: 'Phones and personal computers receive an address from their region\'s device pool. The address stays with the device and is reclaimed only after 30 days without use.',
      add: 'Add device',
      empty: 'No devices',
      loadFailed: 'Devices could not be loaded',
      hostname: 'Hostname',
      hostnameHelp: 'One DNS label: letters, digits and inner hyphens.',
      deviceKey: 'Device key',
      deviceKeyHelp: 'The device\'s public identity key. Optional.',
      lastUsed: 'Last used',
      pool: 'Device pool',
      poolUsage: '{used} of {size} in use',
      poolLow: 'Device pool nearly full',
      assign: 'Assign a new address',
      assigned: 'New address assigned',
      move: 'Move to region…',
      moveTitle: 'Move {name}',
      moveDescription: 'The device receives a new address from the target region\'s device pool.',
      addTitle: 'Add device',
      editTitle: 'Edit {name}',
      edit: 'Edit…',
      deleteTitle: 'Delete device {name}',
      deleteMessage: 'The device\'s address is released.',
      warnings: {
        no_address: 'Address reclaimed',
        address_idle: 'Unused for 30 days'
      }
    },
    hostGroups: {
      title: 'Host groups',
      description: 'A host group is the virtual machines or containers behind a gateway node. It uses a host group segment outside the region ranges.',
      add: 'New host group',
      empty: 'No host groups',
      loadFailed: 'Host groups could not be loaded',
      segment: 'Segment',
      segmentHelp: 'Assigned automatically when empty. Cannot be changed after creation.',
      gateway: 'Gateway node',
      standby: 'Standby gateway',
      standbyHelp: 'A node in the gateway\'s region. Optional.',
      addTitle: 'New host group',
      editTitle: 'Edit {name}',
      edit: 'Edit…',
      deleteTitle: 'Delete host group {name}',
      deleteMessage: 'The device names of the host group are deleted with it.'
    },
    deviceNames: {
      title: 'Device names',
      description: 'A gateway node publishes names for the hosts behind it: hosts of a host group or devices of its local network.',
      add: 'Add name',
      empty: 'No device names',
      hostGroup: 'Host group',
      localNetwork: 'Gateway\'s local network',
      kind: 'Location',
      addTitle: 'Add device name',
      editTitle: 'Edit {name}',
      edit: 'Edit…',
      addressHelp: 'A host of a host group uses an address in the group\'s segment. A local device uses a private address outside the region ranges and the host group range.',
      deleteTitle: 'Delete name {name}',
      deleteMessage: 'The name is no longer published.'
    },
    names: {
      title: 'Intranet names',
      description: 'Names take the form name.region-code.{suffix}, in the member\'s region. Duplicate names are all listed and marked.',
      empty: 'No intranet names',
      loadFailed: 'Intranet names could not be loaded',
      fqdn: 'Name',
      source: 'Source',
      duplicate: 'Duplicate',
      unnamed: 'These nodes have no intranet name because their names are not valid DNS labels: {names}',
      sources: {
        node: 'Node',
        device: 'Device',
        host_group: 'Host group',
        local: 'Local device'
      }
    },
    changes: {
      title: 'Region changes',
      description: 'A region change alters the member\'s virtual address and requires administrator approval.',
      empty: 'No region changes',
      loadFailed: 'Region changes could not be loaded',
      member: 'Member',
      move: 'Change',
      reason: 'Reason',
      status: 'Status',
      approve: 'Approve…',
      reject: 'Reject…',
      approveTitle: 'Approve region change',
      rejectTitle: 'Reject region change',
      approveDescription: 'The member moves to the target region and receives a new address.',
      note: 'Note',
      confirmApprove: 'Approve',
      confirmReject: 'Reject',
      approved: 'Approved',
      rejected: 'Rejected',
      device: 'Device {id}',
      sources: {
        member: 'Member request',
        admin: 'Administrator'
      },
      statuses: {
        all: 'All',
        pending: 'Pending',
        approved: 'Approved',
        rejected: 'Rejected'
      }
    },
    settings: {
      title: 'Settings',
      nameSuffix: 'Domain suffix',
      nameSuffixHelp: 'Names take the form name.region-code.suffix. The suffix lies under internal or home.arpa.',
      hostGroupSupernet: 'Host group range',
      hostGroupPrefix: 'Host group segment prefix length',
      hostGroupHelp: 'Cannot be changed while host groups exist.',
      reservedRanges: 'Reserved ranges',
      reservedRangesHelp: 'One range per line. Regions and host groups may not overlap them.',
      loadFailed: 'Mesh settings could not be loaded'
    }
  }
}
