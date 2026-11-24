// admin.nodes: the node list and the node page (UI U7).
import { AGENT_NAME } from '../../../constants/brand'

export default {
  title: 'Nodes',
  subtitle: 'Proxy nodes that serve subscriptions.',
  confirm: {
    deleteNodeTitle: 'Delete node {name}?',
    deleteNodeMessage: 'The node and its credentials will be removed. This cannot be undone.',
    deleteNodeAction: 'Delete node',
    disableNodeTitle: 'Disable node {name}?',
    disableNodeMessage: 'Subscriptions stop offering this node and its Agent certificates are revoked.',
    disableNodeAction: 'Disable node',
    deleteProtocolTitle: 'Delete protocol {type} :{port}?',
    deleteProtocolMessage: 'Node {name} stops offering this protocol. This cannot be undone.',
    deleteProtocolAction: 'Delete protocol'
  },
  stats: {
    total: 'Total',
    online: 'Online',
    offline: 'Offline',
    pending: 'Pending'
  },
  filters: {
    label: 'Filter by status',
    search: 'Search nodes',
    searchPlaceholder: 'Name or address'
  },
  table: {
    label: 'Nodes',
    id: 'ID',
    name: 'Name',
    address: 'Address',
    status: 'Status',
    runtimeHealthy: 'Runtime OK',
    runtimeUnhealthy: 'Runtime error',
    parent: 'Parent node',
    protocols: 'Protocols',
    agentVersion: 'Agent version',
    kernelVersion: 'Kernel version',
    compatibility: {
      warning: 'Check version',
      incompatible: 'Incompatible',
      problems: {
        agent_protocol_too_old: 'The Agent protocol revision is below the minimum Control supports. Upgrade the Agent.',
        agent_protocol_too_new: 'The Agent protocol revision is newer than Control; new features are disabled. Upgrade Control.',
        version_line_mismatch: 'The Agent and Control major.minor versions differ. Upgrade to the same version line.',
        kernel_too_old: 'anx-node is below the version this Agent requires. Upgrade the kernel.'
      }
    },
    kernelUnknown: 'Not reported',
    kernelUnknownHint: 'Reported by an online Agent; older Agents do not report it.',
    load: 'Load',
    loadValue: 'CPU {cpu} · Mem {memory}',
    traffic: 'Total traffic',
    monthlyQuota: 'Monthly quota',
    quotaExceeded: 'Over quota',
    lastHeartbeat: 'Last heartbeat',
    lastSeen: 'Last seen',
    nodeGroup: 'Node group',
    connection: 'Connection',
    certificate: 'Certificate',
    serviceStatus: 'Services',
    never: 'Never',
    empty: 'No nodes',
    emptyDescription: 'Choose Join a node to get the install command.',
    loadFailed: 'Could not load nodes'
  },
  // The Services column: the machine-telemetry services summary of a node.
  serviceStatus: {
    states: {
      healthy: '{active}/{total} OK',
      partial: '{active}/{total} running',
      failed: '{failed} failed',
      empty: 'No services',
      waiting: 'Waiting',
      unsupported: 'Unsupported',
      stale: 'Stale'
    },
    counts: '{total} services: {active} running, {failed} failed',
    loading: 'Loading service status',
    unavailable: 'Service status unavailable: requires the machine-telemetry package and its API permission',
    notEnabled: 'Service collection is off on this node'
  },
  actions: {
    open: 'Open node',
    manageProtocols: 'Manage protocols',
    syncReload: 'Sync and reload',
    upgradeKernel: 'Upgrade kernel',
    logs: 'View logs',
    edit: 'Edit',
    delete: 'Delete'
  },
  detail: {
    title: 'Node',
    back: 'Nodes',
    loading: 'Loading node…',
    loadFailed: 'Could not load this node',
    notFound: 'Node #{id} does not exist',
    notFoundHint: 'It may have been deleted. Go back to the node list.',
    sections: 'Node sections',
    sectionNames: {
      overview: 'Overview',
      protocols: 'Protocols',
      credentials: 'Credentials',
      deploy: 'Deployment',
      traffic: 'Traffic',
      diagnostics: 'Diagnostics',
      reality: 'Targets',
      logs: 'Logs',
      services: 'Services',
      danger: 'Danger zone'
    }
  },
  overview: {
    health: 'Health',
    traffic: 'Traffic',
    config: 'Settings',
    never: 'No heartbeat',
    os: 'Operating system',
    serverIp: 'Server IP',
    uptime: 'Uptime',
    onlineUsers: 'Online users',
    cpu: 'CPU',
    memory: 'Memory',
    disk: 'Disk',
    totalUpload: 'Total upload',
    totalDownload: 'Total download',
    unlimited: 'No limit · {used} this month',
    resetDay: 'Day {day} of each month',
    quotaFooter: 'Nodes over the monthly quota are flagged only, not limited.',
    overQuota: 'Over the monthly quota. Flagged only, not limited.',
    runtimeError: 'The node reports a runtime error: {message}',
    rootNode: 'None (exit node)',
    registered: 'Added',
    autoRegistered: `By its ${AGENT_NAME}`,
    manual: 'By hand',
    createdAt: 'Created'
  },
  agent: {
    title: 'Agent connection',
    retry: 'Retry',
    loading: 'Loading the Agent connection…',
    unavailable: 'Unavailable',
    loadFailed: 'Could not load the Agent connection',
    noRecord: 'No Agent record',
    noRecordHint: 'No Agent is enrolled. Install one from the Deployment tab.',
    connectionLabel: 'Connection',
    transport: 'Last transport',
    lastSeen: 'Last seen',
    neverSeen: 'Never',
    certificateLabel: 'Certificate',
    notAfter: 'Not valid after',
    renewAfter: 'Renews after',
    revokedAt: 'Revoked at',
    revokeReason: 'Revoked because',
    overdueNotice: 'The Agent did not renew its certificate after {date}. The certificate is still valid.',
    revokedNotice: 'The Agent certificate is revoked. The Agent must enroll again.',
    footer: 'From the Agent transport inventory.',
    connection: {
      mtls_stream: 'mTLS stream',
      offline: 'Offline'
    },
    connectionHint: {
      mtls_stream: 'Control stream authenticated by the Agent certificate',
      offline: 'Nothing seen in the last five minutes'
    },
    certificate: {
      valid: 'Valid',
      overdue: 'Renewal overdue',
      revoked: 'Revoked',
      expired: 'Expired',
      none: 'No certificate',
      until: 'until {date}',
      expiredOn: 'expired {date}',
      revokedOn: 'revoked {date}'
    }
  },
  form: {
    titleEdit: 'Edit node',
    save: 'Save changes',
    required: 'Required',
    parentNone: 'None (root / exit node)',
    parentHint: 'Builds a multi-level relay chain. Forwarded traffic counts for this node and every ancestor.',
    fields: {
      name: 'Name',
      address: 'Address',
      tags: 'Tags',
      rate: 'Traffic rate',
      sort: 'Sort order',
      status: 'Status',
      parent: 'Parent node',
      monthlyLimit: 'Monthly quota',
      monthlyResetDay: 'Monthly reset day'
    },
    placeholders: {
      name: 'hk-01',
      address: 'IP or domain',
      tags: 'HK,IEPL,Premium'
    },
    help: {
      rate: 'Usage is multiplied by this rate.',
      sort: 'Lower numbers come first.',
      monthlyLimit: 'Empty for no limit.',
      monthlyResetDay: '1 to 28.'
    }
  },
  install: {
    title: 'Move to a new machine',
    description: 'Move node {node} to another machine.',
    intro: 'Generates a single-use token bound to this node and the install commands. Run one on the new machine as root. The new machine replaces the old one; the command can be run again safely.',
    ttl: 'Token lifetime',
    ttlOptions: {
      hour: '1 hour (default)',
      sixHours: '6 hours',
      day: '24 hours',
      week: '7 days (maximum)'
    },
    generate: 'Generate install command',
    regenerate: 'Generate again',
    commandTitle: 'Install command',
    once: 'The token is shown once, works once and expires {time}.',
    mirror: 'Download source',
    mirrors: {
      control: 'Control',
      cn: 'Mainland mirror',
      github: 'GitHub',
      controlHelp: 'The script and the Agent download from this Control.',
      cnHelp: 'The script downloads from this Control and the Agent from the mainland mirror; checksums come from Control or GitHub.',
      githubHelp: 'The script and the Agent download from GitHub releases; the token enrolls with this Control.'
    },
    commandLabel: 'Run on the node as root',
    copy: 'Copy command',
    fallback: 'This download source is not set up: {note}',
    signed: 'The install script is signed. Verify it with /install.sh.sig and the official release key.',
    unsigned: 'No install script signature on this Control. To verify, use agent-install.sh and its .sig from GitHub releases.',
    legacy: 'The install removes the legacy forward runtime (nftables tables inet v2b_forward, ip v2b_forward and ip anixops_forward, and the v2forward-agent service) and lists what was removed.',
    failed: 'Could not generate the install command'
  },

  deploySection: {
    title: 'Deployment',
    description: `Manage the ${AGENT_NAME} of this node. Add new machines under Nodes › Join a node.`,
    move: 'Move to a new machine',
    moveText: 'Move this node to another machine, or join it again after its certificate stops working.',
    moveAction: 'Generate install command',
    certificate: 'Certificate',
    certificateText: 'The node connects with a certificate that is renewed before expiry.',
    certificateGone: 'The certificate is invalid and the Agent cannot connect. Join again with “Move to a new machine”.',
    certificateNone: 'No Agent certificate. Use “Move to a new machine” to join a machine.',
    uninstall: 'Uninstall',
    uninstallText: 'Run on the node. Add --purge to also remove its identity and data.',
    uninstallLabel: 'Run on the node as root',
    copy: 'Copy command'
  },
  traffic: {
    title: 'Traffic',
    description: 'User traffic through this node, with the traffic rate applied.',
    range: 'Time range',
    ranges: { '24h': '24 h', '7d': '7 d', '30d': '30 d', '90d': '90 d', '1y': '1 y' },
    rangeNames: {
      '24h': 'the last 24 hours',
      '7d': 'the last 7 days',
      '30d': 'the last 30 days',
      '90d': 'the last 90 days',
      '1y': 'the last year'
    },
    refresh: 'Refresh',
    summaryLabel: 'Traffic totals',
    upload: 'Upload',
    download: 'Download',
    total: 'Total',
    window: '{from} to {to}',
    chart: {
      title: 'Traffic over time',
      label: 'Traffic of {node}, {range}',
      summary: '{up} uploaded, {down} downloaded. Peak {bucket}: {peak}, starting {time}.',
      summaryEmpty: 'No traffic.',
      bucketHour: 'hour',
      bucketDay: 'day',
      time: 'Time'
    },
    empty: 'No traffic in this range',
    emptyHour: 'No reports in this period. Hourly data follows the traffic log retention.',
    emptyDay: 'Nothing was recorded on these days.',
    loadFailed: 'Could not load the traffic',
    noteHour: 'Hourly totals (UTC hours), shown in the local time zone.',
    noteDay: 'Daily totals (calendar days of the Control host), independent of traffic log retention.'
  },
  rotate: {
    title: 'Agent credentials',
    row: 'Rotate Agent credentials',
    rowHint: 'Revoke the Agent credentials of this node and issue a one-time credential to enroll again.',
    footer: 'Super administrators only. The rotation and reason are recorded in the audit log; the credential is not.',
    action: 'Rotate credentials…',
    superOnly: 'Only super administrators can rotate credentials.',
    disabledHint: 'This node is disabled. Enable it first.',
    confirm: {
      title: 'Rotate the credentials of {name}?',
      message: 'The Agent must enroll again. Only the latest credential is valid.',
      action: 'Rotate credentials',
      impact: 'Impact',
      revoke: 'All Agent certificates, enrollments and forward link certificates of this node are revoked. A running Agent disconnects at its next heartbeat.',
      issue: 'A new one-time credential is issued and shown once. Unused earlier credentials stop working.',
      offline: 'The node is not contacted; rotation works while it is offline.',
      reason: 'Reason',
      reasonHelp: 'Optional, recorded in the audit log. {count} / {max} bytes.',
      reasonTooLong: 'The reason is too long: {count} of {max} bytes.',
      reasonPlaceholder: 'For example: the host’s disk was stolen',
      ttl: 'Credential lifetime'
    },
    result: {
      title: 'New credential for {name}',
      description: 'Shown once. It cannot be shown again after this dialog closes.',
      warning: 'Treat it as a password. Until used or expired, it can enroll an Agent as this node. Do not share it.',
      label: 'One-time credential',
      copy: 'Copy credential',
      expires: 'Valid until {time}',
      remaining: 'in {remaining}',
      expired: 'This credential has expired. Rotate again.',
      single: 'It works once.',
      revoked: 'Revoked',
      certificates: 'Agent certificates',
      enrollments: 'Enrollments',
      linkCertificates: 'Forward link certificates',
      next: 'Use this credential to enroll the Agent again. See the guide for the steps.',
      guide: 'Rotating a node’s credentials (guide)',
      done: 'Done'
    },
    errors: {
      super_admin_required: 'Only super administrators can rotate credentials.',
      node_not_found: 'This node no longer exists.',
      node_disabled: 'This node is disabled. Enable it first.',
      agent_pki_disabled: 'The built-in Agent CA is disabled (module_runtime.ca_kek). Credentials cannot be issued.',
      failed: 'Could not rotate the credentials: {message}'
    }
  },
  credentials: {
    title: 'Credentials',
    description: `The ${AGENT_NAME} authenticates to the control plane with an mTLS certificate; the node has no API key.`,
    nodeId: 'Node ID',
    protocolSecrets: 'Protocol secrets',
    protocolSecretsRow: 'Open protocols',
    protocolSecretsFooter: 'Private keys and passwords show as ********; keep it to retain the stored value.'
  },
  danger: {
    title: 'Danger zone',
    description: 'These change what subscriptions offer.',
    disable: 'Disable node',
    disableHint: 'Subscriptions stop offering it; its Agent certificates are revoked.',
    disableAction: 'Disable…',
    enable: 'Enable node',
    enableHint: 'The node returns to pending and shows online after the next heartbeat.',
    enableAction: 'Enable',
    delete: 'Delete node',
    deleteHint: 'Removes the node, its protocols and credentials. Requires the node name to confirm.',
    deleteAction: 'Delete…'
  },
  protocols: {
    title: 'Protocols',
    description: 'Protocols served by this node. Changes apply at the next sync.',
    tableLabel: 'Protocols of {name}',
    add: 'Add protocol',
    empty: 'No protocols',
    emptyDescription: 'Add a protocol so subscriptions can use this node.',
    loadFailed: 'Could not load the protocols',
    enabled: 'Enabled',
    disabled: 'Disabled',
    listed: 'Listed',
    hidden: 'Hidden',
    columns: {
      type: 'Protocol',
      port: 'Port',
      transport: 'Transport',
      tls: 'TLS',
      status: 'Status',
      show: 'In subscriptions'
    }
  },
  services: {
    title: 'Services',
    description: 'systemd services on this node (read-only): state, 10-minute CPU and memory.',
    tableLabel: 'Services of {name}',
    refresh: 'Refresh services',
    settings: 'Settings',
    loadFailed: 'Could not load the services',
    totals: "Total {total} {'|'} Failed {failed} {'|'} Updated every 10 minutes",
    observedAt: 'Reported {time}',
    stale: 'Last report: {time}. The data may be out of date.',
    unsupported: 'This node cannot report its services',
    unsupportedReason: 'Reason: {reason}',
    unsupportedNoReason: 'The node needs systemd and cgroup v2.',
    waiting: 'Waiting for the first report',
    waitingDescription: 'Once enabled, the node reports about once a minute.',
    empty: 'No services match the node’s filters',
    emptyDescription: 'Change the include or exclude patterns in Settings.',
    disabled: {
      title: 'Not enabled on this node',
      description: 'Off by default. When enabled, only unit names, states, CPU and memory of systemd services are collected.',
      enable: 'Enable for this node'
    },
    filters: {
      label: 'Filter by state',
      all: 'All',
      search: 'Search services',
      searchPlaceholder: 'Unit name'
    },
    states: {
      active: 'Active',
      failed: 'Failed',
      inactive: 'Inactive',
      activating: 'Activating',
      deactivating: 'Deactivating',
      reloading: 'Reloading'
    },
    columns: {
      name: 'Service',
      state: 'State',
      cpuAvg: 'CPU (10 min avg)',
      cpuPeak: 'CPU peak',
      memory: 'Memory',
      memoryPeak: 'Memory peak'
    },
    form: {
      title: 'Services settings',
      description: 'Node {name}',
      enabled: 'Collect services on this node',
      enabledHelp: 'Read-only. Services are never started, stopped or restarted.',
      include: 'Include patterns',
      includeHelp: 'One pattern per line, such as nginx*.service. Empty includes all services.',
      exclude: 'Exclude patterns',
      excludeHelp: 'One pattern per line. Matching services are left out.',
      invalidGlob: 'Not a valid pattern: {glob}',
      tooManyGlobs: 'At most {max} patterns',
      save: 'Save',
      saved: 'Services settings saved. They apply with the next node configuration.',
      saveFailed: 'Could not save the services settings',
      conflict: 'The settings were changed elsewhere and have been reloaded. Review and save again.',
      noInstallation: 'Machine Telemetry has no Agent installation. Install it under Plugins.'
    }
  },
  logs: {
    title: 'Logs',
    description: 'Runtime logs the node reported, newest first.',
    tableLabel: 'Logs of {name}',
    refresh: 'Refresh logs',
    empty: 'No logs from this node',
    emptyDescription: 'Logs appear here once the node reports them.',
    loadFailed: 'Could not load the logs',
    filters: {
      allLevels: 'All levels',
      search: 'Search logs',
      sourcePlaceholder: 'Source',
      searchPlaceholder: 'Message, source or trace ID'
    },
    levels: {
      debug: 'Debug',
      info: 'Info',
      warning: 'Warning',
      error: 'Error'
    },
    columns: {
      time: 'Time',
      level: 'Level',
      source: 'Source',
      message: 'Message',
      fields: 'Structured fields'
    }
  },
  protocolForm: {
    titleCreate: 'Add protocol',
    titleEdit: 'Edit protocol',
    description: 'Node {name}',
    create: 'Add protocol',
    save: 'Save protocol',
    pickHint: 'Choose a template. All settings can be changed in the next step.',
    portSpec: 'Port {port}',
    recommended: 'Recommended',
    change: 'Change',
    fixed: '{value} (fixed for this protocol)',
    blank: {
      name: 'Custom',
      description: 'Start from a blank configuration'
    },
    sections: {
      basics: 'Basics',
      connection: 'Transport and security',
      publish: 'Publishing'
    },
    enableHint: 'When off, the configuration is kept but not run on the node.',
    showHint: 'Include this inbound in user subscriptions.',
    maskedSecretsHint: 'Stored secrets show as ********. Keep it to retain the value, or enter a new value.',
    modeLabel: 'Editor',
    jsonLabel: 'Protocol JSON',
    tabs: {
      visual: 'Form',
      json: 'Advanced JSON'
    },
    fields: {
      type: 'Protocol',
      port: 'Listen port',
      tls: 'TLS',
      transport: 'Transport',
      settings: 'Protocol settings (JSON)',
      tlsSettings: 'TLS settings (JSON)',
      realitySettings: 'Reality settings (JSON)',
      transportSettings: 'Transport settings (JSON)'
    },
    jsonActions: {
      format: 'Format',
      copy: 'Copy'
    },
    jsonStatus: {
      valid: 'Valid JSON',
      invalid: 'Invalid JSON'
    },
    iwan: {
      title: 'Panabit iWAN',
      unencrypted: 'Not encrypted',
      unencryptedHint: 'iWAN has no encryption or integrity protection. Use it on trusted links only.',
      fields: {
        pool: 'Address pool',
        mtu: 'MTU',
        dnsUpstream: 'DNS upstream',
        allowEncrypt: 'Allow XOR masking',
        allowClientToClient: 'Allow user-to-user traffic',
        fixedAddresses: 'Fixed addresses'
      },
      hints: {
        pool: 'IPv4 network (/8 to /30); the first address is the gateway.',
        dnsUpstream: 'Where the gateway relays DNS: an IP or IP:port.',
        allowEncrypt: 'XOR masking is not encryption.',
        fixedAddresses: 'One "user id=address" per line, inside the pool and not the gateway.',
        login: 'Login: user id and the first 16 hex digits of the UUID. The iwand configuration is available in the iWAN subscription format.'
      }
    },
    wireguard: {
      sections: {
        access: 'WireGuard access'
      },
      fields: {
        cidr: 'Peer CIDR',
        serverAddress: 'Interface address',
        serverPrivateKey: 'Server private key',
        serverPublicKey: 'Server public key',
        mtu: 'MTU',
        dns: 'DNS servers',
        allowedIps: 'Allowed IPs'
      },
      actions: {
        generateKeypair: 'Generate keypair'
      }
    },
    enable: 'Run on the node',
    show: 'List in subscriptions'
  },
  // Protocol form fields (protocolFields.js): 3x-ui style, shown by protocol, transport and TLS.
  protocolFields: {
    groups: {
      protocol: 'Protocol',
      transport: 'Transport',
      security: 'Security',
      anxCover: 'Fallback target',
      anxEntry: 'User entry',
      anxTuning: 'Handshake tuning',
      anxAdvanced: 'Advanced'
    },
    groupIntros: {
      anxCover: 'Non-ANX connections are forwarded to this site.',
      anxEntry: 'Only for nodes users connect to directly; keep the defaults on transit nodes.',
      anxTuning: 'Optional: a general preset applies when empty.'
    },
    none: 'None',
    needsAttention: 'See hint',
    generate: 'Generate',
    labels: {
      server_names: 'Allowed SNI',
      alpn: 'ALPN',
      flow: 'Flow',
      method: 'Cipher',
      server_key: 'Server key',
      up_mbps: 'Upload (Mbps)',
      down_mbps: 'Download (Mbps)',
      obfs: 'Obfuscation',
      obfs_password: 'Obfuscation password',
      congestion_control: 'Congestion control',
      psk: 'PSK',
      version: 'Version',
      mode: 'Mode',
      path: 'Path',
      host: 'Host',
      serviceName: 'gRPC service name',
      server_name: 'SNI (server name)',
      fingerprint: 'uTLS fingerprint',
      allowInsecure: 'Allow insecure certificates',
      dest: 'Target (Dest)',
      short_id: 'Short ID',
      private_key: 'Private key',
      public_key: 'Public key',
      allow_no_sni: 'No-SNI mode',
      shape_plain: 'Turn off traffic shaping',
      max_streams: 'Max streams per session',
      dest_stack: 'TLS stack (dest_stack)',
      dest_behavior: 'Dest behavior (dest_behavior)',
      dest_behaviors: 'Per-dest behavior (dest_behaviors)',
      wg_listen_port: 'UDP port (wg jacket)',
      bare_listen_port: 'TCP port (bare jacket)',
      wire_gen: 'Published handshake generation',
      min_wire_gen: 'Lowest handshake generation accepted'
    },
    hints: {
      anxServerNames: 'Comma separated; clients use the first. Must be names the fallback target serves.',
      anxDest: 'Forward address for non-ANX connections, as host:port.',
      anxAllowNoSni: 'Also accept connections without SNI; subscriptions then carry no SNI. Requires anx-node newer than alpha.7.',
      anxShapePlain: 'For interoperability debugging only.',
      anxMaxStreams: 'Concurrent streams per session. Default 256.',
      anxWireGen: 'Set to 2 to publish the server key with "{\'@\'}2"; clients then use the second handshake generation. Enable only after anx-node supports it and all clients are updated; older clients cannot use the new key. Default 1.',
      anxMinWireGen: 'Set to 2 to refuse first-generation handshakes. Enable only after the published generation is 2 and Stats auth_gen1 stops growing. Default 1. Requires anx-node with second-generation support.',
      anxDestBehavior: 'Paste the complete "dest_behavior": {…} line or a JSON object, including session_tickets and flight_records. Empty uses the stack preset.',
      anxDestBehaviorMissing: 'Warning: a dest lacks session_tickets or flight_records in dest_behavior. Paste the complete line ("none" counts as set).',
      anxDestBehaviors: 'For multiple dests: a JSON object mapping each dest ("host:port") to its overrides of dest_behavior. Requires a newer anx-node.',
      anxDestBehaviorsStale: 'Some entries name a dest no longer listed; they are kept but not sent.',
      anxWgListenPort: 'Serve the wg jacket on this UDP port; subscriptions add wg_port. Open the port in the firewall. Empty disables it. Requires a newer anx-node.',
      anxBareListenPort: 'Serve the bare jacket on this TCP port; requires no-SNI mode. Subscriptions add bare_port. Open the port in the firewall. Empty disables it. Requires a newer anx-node.',
      serverKey: 'Required for 2022 ciphers; generated when switching to one.',
      snellPsk: 'Shared by all users; each user also authenticates with a UUID. Listed in the Surge format only. Requires Agent protocol revision 3 or later.',
      snellVersion: '1 to 6. Version 4 nodes also serve version 5 clients. Obfuscation applies to versions 1 to 4; mode applies to version 6, which requires a PSK of at least 12 bytes.',
      privateKey: 'Empty generates a key pair when saved.',
      publicKey: 'Public key sent to clients; empty derives it from the private key.'
    }
  },
  tabs: {
    label: 'Node sections',
    nodes: 'Nodes',
    groups: 'Node groups'
  },
  // Join a node: one entry for a new machine (docs/design/node-onboarding).
  join: {
    title: 'Join a node',
    description: 'Run the install command on the target machine to join the selected node group.',
    checking: 'Checking that this Control can take nodes…',
    loading: 'Getting ready…',
    loadFailed: 'Could not get ready: {message}',
    retry: 'Try again',
    recheck: 'Check again',
    group: 'Node group',
    newGroup: 'New node group',
    requiresApproval: 'New nodes require approval',
    defaultCreated: 'Default node group “{name}” created: VLESS Reality on a random port, in the default subscription group.',
    blocked: {
      title: 'Nodes cannot join',
      description: 'Resolve the following first.'
    },
    checks: {
      public_url: 'Control’s address',
      panel_certificate: 'Panel certificate',
      grpc: 'gRPC address',
      grpc_certificate: 'gRPC certificate',
      ca: 'Key that signs node certificates',
      agent_release: 'Agent release',
      script_signature: 'Install script signature'
    },
    fixes: {
      public_url: 'Set agent_install.public_url to the Control https address (for example https://panel.example.com) and restart Control. Behind a reverse proxy, add the proxy to server.trusted_proxies.',
      panel_certificate: 'The panel certificate is not ready; the banner at the top shows why. Resolve it, then choose Request again or run anix-control tls retry on the server.',
      grpc: 'Set domain or grpc.enabled. If nodes connect to a different address, set agent_install.grpc_target. Then restart Control.',
      grpc_certificate: 'The gRPC certificate must come from a public CA. Set domain (behind a CDN, also grpc.public_name to a DNS-only name) to obtain one; the banner at the top shows why it is not ready.',
      ca: 'Set module_runtime.ca_kek (environment variable ANIX_CONTROL_MODULE_RUNTIME_CA_KEK) to a 32-byte key (openssl rand -base64 32) and restart Control.',
      agent_release: 'Set agent_install.agent_version to a published Agent release (for example v4.2.0), or place the release in agent_install.artifact_dir, and restart Control.',
      script_signature: 'Point agent_install.signature_file to agent-install.sh.sig of this version, or leave it empty.'
    },
    command: {
      title: 'Install command',
      mirror: 'Download source',
      mirrors: {
        control: 'Control',
        cn: 'Mainland mirror',
        github: 'GitHub',
        controlHelp: 'The script and the Agent download from this Control.',
        cnHelp: 'The script downloads from this Control and the Agent from the mainland mirror; checksums come from Control or GitHub.',
        githubHelp: 'The script and the Agent download from GitHub releases; the node joins this Control.'
      },
      commandLabel: 'Run on the machine to join, as root',
      copy: 'Copy command',
      fallback: 'This download source is not set up: {note}',
      expires: 'The token expires {time}. The command can join multiple machines.',
      neverExpires: 'The token does not expire. The command can join multiple machines.',
      expired: 'The token has expired. Set a new validity under Node groups › Edit, or reset the token.',
      history: 'The command contains the token and remains in the shell history of the machine.',
      reset: 'Reset token',
      resetConfirmTitle: 'Reset the token of “{name}”?',
      resetConfirmMessage: 'Previously copied commands stop working. Joined nodes are not affected.',
      resetDone: 'Token reset. Use the new command.',
      uninstall: 'Uninstall command',
      uninstallHelp: 'Run on the node. Add --purge to also remove its identity and data.',
      forbidden: 'Only super administrators can view the join command.',
      failed: 'Could not read the join command: {message}'
    },
    nodes: {
      title: 'Machines that joined',
      waiting: 'Waiting for a machine to join…',
      waitingHint: 'Machines appear here after running the command above.',
      joined: '{name} joined',
      online: 'Online',
      connecting: 'Connecting',
      more: '{count} more that joined earlier are in the node list.'
    },
    quick: {
      name: 'Node group name',
      namePlaceholder: 'Hong Kong',
      create: 'Create node group',
      failed: 'Could not create the node group: {message}'
    }
  },
  approval: {
    pending: 'Awaiting approval',
    pendingHint: 'Awaiting approval. No configuration or users are sent until approved.',
    approved: 'Approved',
    rejected: 'Rejected',
    approve: 'Approve',
    reject: 'Reject',
    banner: '{count} machine(s) awaiting approval. No configuration or users are sent until approved.',
    view: 'View',
    rejectTitle: 'Reject node {name}?',
    rejectMessage: 'The node is disabled and its certificates are revoked.',
    approvedToast: '{name} approved',
    rejectedToast: '{name} rejected',
    failed: 'Could not process: {message}'
  },
  // Node groups (the tab and its sheet).
  groups: {
    summary: 'A node group defines the defaults, protocols and join token of its nodes.',
    add: 'New node group',
    table: {
      label: 'Node groups',
      name: 'Name',
      nodes: 'Nodes',
      requireApproval: 'Needs approval',
      token: 'Join token',
      rate: 'Traffic rate',
      sort: 'Sort'
    },
    nodesLive: '{count}',
    nodesCount: '{live} node(s), {pending} awaiting approval',
    nodesPending: '{count} awaiting',
    toggleApproval: 'New nodes of “{name}” need approval',
    tokenNever: 'Does not expire',
    tokenUntil: 'Ends {date}',
    tokenExpired: 'Expired',
    tokenExpiredEdit: 'The token has expired. Set a new validity under Edit.',
    approvalOn: 'New nodes of “{name}” require approval.',
    approvalOff: 'New nodes of “{name}” no longer need approval.',
    empty: 'No node groups',
    emptyDescription: 'Choose Join a node to create the default node group.',
    loadFailed: 'Could not load node groups',
    created: 'Node group {name} created',
    saved: 'Node group {name} saved',
    deleted: 'Node group {name} deleted',
    deleteTitle: 'Delete node group {name}?',
    deleteMessage: 'The join token stops working immediately. Joined nodes are not affected.',
    deleteAction: 'Delete node group',
    deleteInUse: 'The group still has nodes: delete or move them first.',
    nameTaken: 'A node group with this name exists.',
    caKekMissing: 'module_runtime.ca_kek is not set. Node groups cannot be created.'
  },
  groupForm: {
    titleCreate: 'New node group',
    titleEdit: 'Edit node group',
    createDescription: 'Settings apply to nodes that join later.',
    create: 'Create node group',
    save: 'Save changes',
    saveFailed: 'Could not save: {message}',
    joining: 'Joining',
    fields: {
      name: 'Name',
      rate: 'Traffic rate',
      sort: 'Sort',
      tags: 'Tags',
      monthlyLimit: 'Monthly traffic limit',
      resetDay: 'Monthly reset day',
      tokenExpiry: 'Token validity',
      maxNodes: 'Node limit',
      requireApproval: 'Require approval for new nodes'
    },
    placeholders: {
      name: 'Hong Kong',
      tags: 'hk,iepl,fast'
    },
    help: {
      rate: 'Usage is counted at this rate.',
      sort: 'Lower numbers come first.',
      monthlyLimit: 'Leave empty for no limit.',
      resetDay: '1 to 28.',
      maxNodes: '0 means no limit.',
      requireApproval: 'No configuration or users are sent until approved.'
    },
    expiry: {
      never: 'Does not expire',
      '7d': '7 days',
      '30d': '30 days',
      '90d': '90 days',
      '365d': '1 year',
      keep: 'Keep (ends {time})',
      expired: 'Expired (set a new one)',
      expiredError: 'The token has expired: choose a new validity.'
    }
  },
  statusText: {
    pending: 'Pending',
    online: 'Online',
    offline: 'Offline',
    disabled: 'Disabled',
    unknown: 'Unknown'
  },
  tlsModes: {
    anxA2: 'A2 jacket',
    none: 'No TLS',
    standard: 'Standard TLS',
    reality: 'Reality (recommended)'
  },
  transports: {
    tcp: 'TCP',
    ws: 'WebSocket',
    grpc: 'gRPC',
    quic: 'QUIC',
    h2: 'HTTP/2',
    httpupgrade: 'HTTPUpgrade',
    udp: 'UDP'
  },
  messages: {
    transportUnsupported: '{type} does not run over {value}',
    tlsUnsupported: '{type} does not take {value}',
    requiredFields: 'Fill in the required fields',
    saveFailed: 'Could not save: {message}',
    syncSuccess: 'Node “{name}” accepted the sync',
    syncFailed: 'Sync failed: {message}',
    nodeSaved: 'Node {name} saved',
    nodeDeleted: 'Node {name} deleted',
    nodeDisabled: 'Node {name} disabled',
    nodeEnabled: 'Node {name} enabled',
    protocolCreated: 'Protocol {type} :{port} added',
    protocolSaved: 'Protocol {type} :{port} saved',
    protocolDeleted: 'Protocol {type} :{port} deleted',
    generateFailed: 'Could not generate: {message}',
    copied: 'Copied',
    copyFailedManual: 'Could not copy automatically. Copy the text manually.',
    invalidJsonDetail: 'Invalid JSON: {message}',
    quotaExceededBanner: 'Nodes over the monthly quota on this page: {count}. Flagged only, not limited.'
  }
}
