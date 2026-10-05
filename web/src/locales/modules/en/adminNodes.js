// admin.nodes: the node list and the node page (UI U7).
import { AGENT_NAME } from '../../../constants/brand'

export default {
  title: 'Nodes',
  subtitle: 'Proxy nodes that serve subscriptions. Open a node for its protocols, credentials, deployment and logs.',
  addNode: 'Add node',
  confirm: {
    deleteNodeTitle: 'Delete node {name}?',
    deleteNodeMessage: 'The node and its credentials are removed. This can’t be undone.',
    deleteNodeAction: 'Delete node',
    disableNodeTitle: 'Disable node {name}?',
    disableNodeMessage: 'Subscriptions stop offering this node and the Agent certificates issued to it are revoked.',
    disableNodeAction: 'Disable node',
    deleteProtocolTitle: 'Delete protocol {type} :{port}?',
    deleteProtocolMessage: 'Node {name} stops offering this protocol. This can’t be undone.',
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
    load: 'Load',
    loadValue: 'CPU {cpu} · Mem {memory}',
    traffic: 'Total traffic',
    monthlyQuota: 'Monthly quota',
    quotaExceeded: 'Over quota',
    lastHeartbeat: 'Last heartbeat',
    connection: 'Connection',
    certificate: 'Certificate',
    never: 'Never',
    empty: 'No nodes yet',
    emptyDescription: `A node appears here when its ${AGENT_NAME} starts with a registration key, or when you add it by hand.`,
    loadFailed: 'Couldn’t load nodes'
  },
  actions: {
    open: 'Open node',
    manageProtocols: 'Manage protocols',
    syncReload: 'Sync and reload',
    logs: 'View logs',
    deployParents: 'Deploy parent nodes',
    edit: 'Edit',
    delete: 'Delete',
    authKey: 'Registration key'
  },
  detail: {
    title: 'Node',
    back: 'Nodes',
    loading: 'Loading node…',
    loadFailed: 'Couldn’t load this node',
    notFound: 'Node #{id} doesn’t exist',
    notFoundHint: 'It may have been deleted. Go back to the node list.',
    sections: 'Node sections',
    sectionNames: {
      overview: 'Overview',
      protocols: 'Protocols',
      credentials: 'Credentials',
      deploy: 'Deployment',
      logs: 'Logs',
      services: 'Services',
      danger: 'Danger zone'
    }
  },
  overview: {
    health: 'Health',
    traffic: 'Traffic',
    config: 'Settings',
    never: 'No heartbeat yet',
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
    quotaFooter: 'A node over its monthly quota is only flagged; nothing is limited automatically.',
    overQuota: 'This node is over its monthly quota. It is only flagged; nothing is limited automatically.',
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
    loadFailed: 'Couldn’t load the Agent connection',
    noRecord: 'No Agent record yet',
    noRecordHint: 'This node has not enrolled an Agent. Install one from the Deployment tab.',
    connectionLabel: 'Connection',
    transport: 'Last transport',
    lastSeen: 'Last seen',
    neverSeen: 'Never',
    certificateLabel: 'Certificate',
    notAfter: 'Not valid after',
    renewAfter: 'Renews after',
    revokedAt: 'Revoked at',
    revokeReason: 'Revoked because',
    overdueNotice: 'The Agent should have renewed its certificate after {date}. The certificate is still valid, but the Agent has not renewed it in time.',
    revokedNotice: 'This Agent’s certificate is revoked. The Agent has to enroll again.',
    footer: 'Read from the Agent transport inventory. A live connection is only known to the Control process that holds it.',
    connection: {
      mtls_stream: 'mTLS stream',
      apikey_stream: 'API key stream',
      legacy: 'Legacy',
      third_party: 'Third-party',
      offline: 'Offline'
    },
    connectionHint: {
      mtls_stream: 'Agent Control stream authenticated by the Agent certificate',
      apikey_stream: 'Agent Control stream authenticated by the node API key',
      legacy: 'Legacy REST, WebSocket or clean-agent channel; refused once agent_control.mtls is required',
      third_party: 'Only third-party node software (UniProxy or v2board gRPC) was seen, not an Agent',
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
    titleCreate: 'Add node',
    titleEdit: 'Edit node',
    createDescription: `Add a node by hand. Nodes whose ${AGENT_NAME} has a registration key add themselves.`,
    create: 'Add node',
    save: 'Save changes',
    required: 'Required',
    parentNone: 'None (root / exit node)',
    parentHint: 'Pick a parent node to build a multi-level relay chain. Traffic this node forwards counts for it and every ancestor.',
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
  authKey: {
    title: 'Registration key',
    label: 'Registration key',
    hint: `An ${AGENT_NAME} with this key in its config.json registers its node when it starts. One key can register any number of nodes.`,
    noKey: 'No key generated yet',
    copy: 'Copy key',
    copyConfig: 'Copy config',
    configTitle: `${AGENT_NAME} config.json`,
    configHint: 'Paste this into the Agent’s config.json. It carries the key above once you generate one.',
    editSettings: 'Change connection settings',
    registeredCount: '{count} nodes registered with this key',
    generate: 'Generate key',
    generateAnother: 'Generate a new key',
    hiddenKey: 'Hidden (********)',
    hiddenHint: 'Keys are shown once, when generated. Generate a new key to copy it; existing keys keep working.',
    shownOnce: 'Copy this key now: it isn’t shown again after you leave this page.',
    defaultName: 'Panel key {date}'
  },
  deploy: {
    title: 'Deploy parent nodes',
    summaryTitle: '{count} parent nodes',
    summaryText: 'Inventory, group vars and commands for config/deploy/ansible/nodes. SSH passwords and key paths stay in this page.',
    warning: 'Use the files with config/deploy/ansible/nodes/deploy_v2bx.yml. Child nodes are deployed separately when needed.',
    settingsTitle: 'Connection',
    hostsTitle: 'Hosts',
    outputTitle: 'Generated files',
    loading: 'Loading the parent nodes’ credentials…',
    empty: 'No parent nodes on this page.',
    commandsLabel: 'Commands',
    authModes: {
      password: 'Password',
      key: 'Private key'
    },
    fields: {
      panelApiHost: 'Control API address',
      grpcHost: 'gRPC address',
      grpcServerName: 'gRPC server name',
      amd64BinaryPath: 'AMD64 binary path',
      arm64BinaryPath: 'ARM64 binary path',
      coreType: 'Core',
      grpcUseTLS: 'Use TLS for gRPC',
      pluginSupervisorEnabled: 'Plugin Supervisor canary',
      pluginRoot: 'Plugin state directory',
      pluginSocketDir: 'Plugin socket directory',
      pluginOfficialPublicKey: 'Official plugin public key'
    },
    pluginSupervisorHint: 'Keeps the data plane as it is and only lets this Agent run signed package lifecycle operations.',
    pluginSupervisorControlRequired: 'The Plugin Supervisor canary needs TLS on the gRPC address, or a loopback gRPC host.',
    pluginSupervisorKeyRequired: 'Enter the official plugin public key before copying a Plugin Supervisor canary configuration.',
    pluginSupervisorKeyInvalid: 'The official plugin public key must be a Base64 Ed25519 public key.',
    table: {
      alias: 'Alias',
      sshHost: 'SSH host',
      port: 'SSH port',
      user: 'SSH user',
      arch: 'Architecture',
      authMode: 'SSH sign-in',
      authValue: 'Password or key path'
    },
    placeholders: {
      password: 'Used by sshpass; stays in this page.',
      privateKey: '~/.ssh/id_ed25519'
    }
  },
  install: {
    title: 'Copy install command',
    description: 'Install and connect the Agent on node {node} with one command.',
    intro: 'Generates a single-use enrollment token bound to this node and the install command for each download source. Paste it on the node as root (sudo): the script installs the Agent, removes this machine’s legacy forward runtime and waits until the Agent has enrolled. Running the same command again is safe.',
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
    once: 'The token is shown only here, works once and expires {time}. It cannot be shown again after you close this sheet.',
    mirror: 'Download source',
    mirrors: {
      control: 'Control',
      cn: 'Mainland mirror',
      github: 'GitHub',
      controlHelp: 'The script and the Agent are downloaded from this Control.',
      cnHelp: 'The script comes from this Control and the Agent from the mainland mirror; checksums always come from Control or GitHub.',
      githubHelp: 'The script and the Agent come from GitHub releases; the token still enrolls with this Control.'
    },
    commandLabel: 'Run on the node as root',
    copy: 'Copy command',
    fallback: 'This download source is not set up: {note}',
    signed: 'The install script is signed: verify it with /install.sh.sig and the official release key before running it.',
    unsigned: 'This Control has no release signature for the install script; to verify it, use agent-install.sh and its .sig from GitHub releases.',
    legacy: 'The install removes this machine’s legacy forward runtime (the nftables tables inet v2b_forward, ip v2b_forward and ip anixops_forward, and the v2forward-agent service) and lists what it removed.',
    failed: 'Could not generate the install command'
  },
  deploySection: {
    title: 'Deployment',
    description: `Install and connect this node’s ${AGENT_NAME}.`,
    registration: 'Agent registration',
    connection: 'Connection settings',
    connectionHint: 'They only change the generated files; nothing is saved.',
    ansible: 'Ansible',
    ansibleRoot: 'Generate the inventory, group vars and commands to deploy this node with Ansible.',
    ansibleChild: 'The Ansible helper covers parent nodes only. Deploy this child node separately.',
    installCommand: 'One-command install',
    installCommandText: 'Copy one command and paste it on the node to install the Agent and enroll it (single-use token).',
    openInstall: 'Copy install command',
    openHelper: 'Open deployment helper'
  },
  credentials: {
    title: 'Credentials',
    description: `The ${AGENT_NAME} signs in to the control plane with this node’s API key.`,
    nodeId: 'Node ID',
    apiKey: 'API key',
    apiKeyHidden: 'Hidden. Reading it is recorded in the audit log.',
    apiKeyShown: 'Shown below, masked.',
    apiKeyHelp: 'Masked; reveal or copy it. It is read again when you come back to this section.',
    reveal: 'Read API key',
    noKey: 'The server returned no key',
    revealFailed: 'Couldn’t read the API key: {message}',
    auditFooter: 'Every read of a node’s credentials is recorded as a reveal in the audit log. The shared secret is never shown here.',
    protocolSecrets: 'Protocol secrets',
    protocolSecretsRow: 'Open protocols',
    protocolSecretsFooter: 'Private keys and passwords in protocol settings show as ********; keep ******** to keep a stored value.'
  },
  danger: {
    title: 'Danger zone',
    description: 'These change what subscriptions offer.',
    disable: 'Disable node',
    disableHint: 'Subscriptions stop offering it and its Agent certificates are revoked.',
    disableAction: 'Disable…',
    enable: 'Enable node',
    enableHint: 'The node goes back to pending and shows online after its next heartbeat.',
    enableAction: 'Enable',
    delete: 'Delete node',
    deleteHint: 'Removes the node, its protocols and credentials. You type its name to confirm.',
    deleteAction: 'Delete…'
  },
  protocols: {
    title: 'Protocols',
    description: 'What this node serves. Changes reach the node with the next sync.',
    tableLabel: 'Protocols of {name}',
    add: 'Add protocol',
    empty: 'No protocols yet',
    emptyDescription: 'Add a protocol, or start from a template, so subscriptions can use this node.',
    loadFailed: 'Couldn’t load the protocols',
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
    description: 'Read-only systemd services on this node: state, CPU over the last 10 minutes and memory.',
    tableLabel: 'Services of {name}',
    refresh: 'Refresh services',
    settings: 'Settings',
    loadFailed: 'Couldn’t load the services',
    totals: "Total {total} {'|'} Failed {failed} {'|'} Updated every 10 minutes",
    observedAt: 'Reported {time}',
    stale: 'The last report is from {time}. The table may be out of date: the node or its collector may be offline.',
    unsupported: 'This node can’t report its services',
    unsupportedReason: 'Reason: {reason}',
    unsupportedNoReason: 'The node needs systemd and cgroup v2.',
    waiting: 'Waiting for the first report',
    waitingDescription: 'The node sends its services table about once a minute after collection is enabled.',
    empty: 'No services match the node’s filters',
    emptyDescription: 'Change the include or exclude patterns in Settings.',
    disabled: {
      title: 'Not enabled on this node',
      description: 'The services table is off by default. Enable it to collect the unit names, states, CPU and memory of this node’s systemd services. Nothing else is collected.',
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
      enabledHelp: 'Read-only: the table never starts, stops or restarts a service.',
      include: 'Include patterns',
      includeHelp: 'One pattern per line, such as nginx*.service. Leave empty to include every service.',
      exclude: 'Exclude patterns',
      excludeHelp: 'One pattern per line. Matching services are left out.',
      invalidGlob: 'Not a valid pattern: {glob}',
      tooManyGlobs: 'At most {max} patterns',
      save: 'Save',
      saved: 'Services settings saved. The node applies them with its next configuration.',
      saveFailed: 'Couldn’t save the services settings',
      conflict: 'The settings changed elsewhere. Reloaded the latest; review and save again.',
      noInstallation: 'Machine Telemetry has no Agent installation. Install it under Plugins first.'
    }
  },
  logs: {
    title: 'Logs',
    description: 'Runtime logs the node reported, newest first.',
    tableLabel: 'Logs of {name}',
    refresh: 'Refresh logs',
    empty: 'No logs from this node',
    emptyDescription: 'Logs appear here once the node reports them.',
    loadFailed: 'Couldn’t load the logs',
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
    templateLibrary: 'Start from a template',
    maskedSecretsHint: 'Stored secrets (private keys, passwords, tokens) show as ********. Leave ******** to keep a stored secret, or replace it with a new value.',
    modeLabel: 'Editor',
    jsonLabel: 'Protocol JSON',
    tabs: {
      visual: 'Form'
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
      copy: 'Copy',
      fromTemplate: 'Load from template'
    },
    templatePicker: {
      title: 'Load from template',
      description: 'Pick a template. It replaces the JSON in the editor.',
      label: 'Template',
      placeholder: 'Choose a template',
      required: 'Choose a template to load.',
      apply: 'Load template'
    },
    jsonStatus: {
      valid: 'Valid JSON',
      invalid: 'Invalid JSON'
    },
    wireguard: {
      sections: {
        access: 'WireGuard access',
        relay: 'Dual-node relay',
        networkPolicy: 'Entry network paths (optional)'
      },
      fields: {
        cidr: 'Peer CIDR',
        serverAddress: 'Entry interface address',
        serverPrivateKey: 'Server private key',
        serverPublicKey: 'Server public key',
        mtu: 'MTU',
        dns: 'DNS servers',
        allowedIps: 'Allowed IPs',
        role: 'Node role',
        tunnelType: 'Entry-to-exit tunnel',
        wssCompat: 'WSS compatibility mode',
        wssPath: 'WSS path',
        wssSecure: 'Verify exit certificate',
        wssServerName: 'WSS server name (SNI)',
        wssCaFile: 'WSS CA certificate file on entry',
        wssCertFile: 'WSS certificate file on exit',
        wssKeyFile: 'WSS private key file on exit',
        relayServer: 'Exit relay host',
        relayServerPort: 'Exit relay port',
        tunPort: 'GOST TUN port',
        tunName: 'GOST TUN name',
        entryTunAddress: 'Entry TUN address',
        exitTunAddress: 'Exit TUN address',
        outboundIface: 'Exit outbound interface',
        exitNat: 'Enable exit NAT',
        routingTable: 'Routing table',
        routingPriority: 'Routing priority',
        networkPolicyEnabled: 'Enable multi-path failover',
        networkPath: 'Network path',
        pathName: 'Path name',
        pathInterface: 'Interface',
        pathSource: 'Source IP',
        pathGateway: 'Gateway',
        pathPriority: 'Priority (lower first)',
        healthInterval: 'Probe interval (seconds)',
        healthTimeout: 'Probe timeout (seconds)',
        failureThreshold: 'Failure threshold',
        failbackDelay: 'Primary failback delay (seconds)'
      },
      actions: {
        generateKeypair: 'Generate keypair',
        addNetworkPath: 'Add network path',
        removeNetworkPath: 'Remove path {index}'
      },
      values: {
        entry: 'Domestic entry',
        exit: 'Overseas exit'
      },
      hints: {
        wssCompat: 'QUIC is the default. WSS verifies the exit certificate and is only for compatibility when UDP relay traffic is blocked or unstable.',
        networkPolicy: 'Only relay-destination traffic is steered, with a separate return route for every source IP. Leave it off on ordinary nodes; the relay host must be an IP address.'
      }
    },
    enable: 'Run on the node',
    show: 'List in subscriptions'
  },
  statusText: {
    pending: 'Pending',
    online: 'Online',
    offline: 'Offline',
    disabled: 'Disabled',
    unknown: 'Unknown'
  },
  tlsModes: {
    none: 'No TLS',
    standard: 'Standard TLS',
    reality: 'Reality (recommended)'
  },
  transports: {
    tcp: 'TCP',
    ws: 'WebSocket',
    grpc: 'gRPC',
    quic: 'QUIC',
    h2: 'HTTP/2'
  },
  messages: {
    requiredFields: 'Fill in the required fields',
    saveFailed: 'Couldn’t save: {message}',
    syncSuccess: 'Node “{name}” accepted the sync',
    syncFailed: 'Sync failed: {message}',
    nodeCreated: 'Node {name} added',
    nodeSaved: 'Node {name} saved',
    nodeDeleted: 'Node {name} deleted',
    nodeDisabled: 'Node {name} disabled',
    nodeEnabled: 'Node {name} enabled',
    protocolCreated: 'Protocol {type} :{port} added',
    protocolSaved: 'Protocol {type} :{port} saved',
    protocolDeleted: 'Protocol {type} :{port} deleted',
    generateFailed: 'Couldn’t generate: {message}',
    deployLoadFailed: 'Couldn’t load the parent nodes’ credentials',
    copied: 'Copied',
    copyFailedManual: 'Couldn’t copy automatically. Select the text and copy it.',
    invalidJsonDetail: 'Invalid JSON: {message}',
    quotaExceededBanner: 'Over their monthly quota on this page: {count}. Nodes are only flagged; nothing is limited automatically.'
  }
}
