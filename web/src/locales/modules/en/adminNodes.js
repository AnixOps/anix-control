// admin.nodes: the node list and the node page (UI U7).
import { AGENT_NAME } from '../../../constants/brand'

export default {
  title: 'Nodes',
  subtitle: 'Proxy nodes that serve subscriptions. Open a node for its protocols, credentials, deployment and logs.',
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
    lastSeen: 'Last seen',
    nodeGroup: 'Node group',
    role: 'Role',
    connection: 'Connection',
    certificate: 'Certificate',
    serviceStatus: 'Services',
    never: 'Never',
    empty: 'No nodes yet',
    emptyDescription: 'Choose Join a node and run the command on a machine: it appears here.',
    loadFailed: 'Couldn’t load nodes'
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
    unavailable: 'Service status is unavailable: it needs the enabled machine-telemetry package and its API permission',
    notEnabled: 'Service collection is off on this node'
  },
  actions: {
    open: 'Open node',
    manageProtocols: 'Manage protocols',
    syncReload: 'Sync and reload',
    logs: 'View logs',
    edit: 'Edit',
    delete: 'Delete',
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
    titleEdit: 'Edit node',
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
  install: {
    title: 'Move to a new machine',
    description: 'Install node {node} on another machine with one command.',
    intro: 'Generates a single-use token bound to this node and the install command for each download source. Paste it on the new machine as root (sudo): the script installs the Agent, removes this machine’s legacy forward runtime and waits until the Agent has joined. Running the same command again is safe. The new machine takes the place of the old one.',
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
    description: `Manage this node’s ${AGENT_NAME}. New machines are not added here: use Nodes › Join a node.`,
    move: 'Move to a new machine',
    moveText: 'Move this node to another machine, or join it again after its certificate stopped working. Generates a single-use command to run on the new machine as root.',
    moveAction: 'Generate install command',
    certificate: 'Certificate',
    certificateText: 'The node connects with a certificate; its Agent renews it before it ends.',
    certificateGone: 'The certificate no longer works, so this node’s Agent cannot connect. Join it again with “Move to a new machine”.',
    certificateNone: 'This node has no Agent certificate yet. Use “Move to a new machine” to join a machine.',
    uninstall: 'Uninstall',
    uninstallText: 'Run it on this node’s machine. Add --purge to remove its identity and data too.',
    uninstallLabel: 'Run on the node as root',
    copy: 'Copy command'
  },
  traffic: {
    title: 'Traffic',
    description: 'The bytes this node’s users moved through it, with the node’s traffic rate applied.',
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
      summary: '{up} uploaded and {down} downloaded. Busiest {bucket}: {peak}, starting {time}.',
      summaryEmpty: 'No traffic.',
      bucketHour: 'hour',
      bucketDay: 'day',
      time: 'Time'
    },
    empty: 'No traffic in this range',
    emptyHour: 'Nothing was reported in these hours. Hourly history only goes back as far as the traffic log is kept.',
    emptyDay: 'Nothing was recorded on these days.',
    loadFailed: 'Couldn’t load the traffic',
    noteHour: 'Hourly buckets (UTC hours) from the traffic log, shown in your time zone. History ends where the log was purged.',
    noteDay: 'Daily totals (calendar days of the Control host) from the daily statistics, kept however long the traffic log is.'
  },
  rotate: {
    title: 'Agent credentials',
    row: 'Rotate Agent credentials',
    rowHint: 'Revoke what this node’s Agent holds and issue a one-time credential to enroll it again.',
    footer: 'Super administrators only. The rotation is recorded in the audit log with your reason; the credential never is.',
    action: 'Rotate credentials…',
    superOnly: 'Only super administrators can rotate credentials.',
    disabledHint: 'This node is disabled. Enable it first, then rotate its credentials.',
    confirm: {
      title: 'Rotate the credentials of {name}?',
      message: 'The Agent has to enroll again. You can rotate again at any time; only the last credential works.',
      action: 'Rotate credentials',
      impact: 'What happens',
      revoke: 'Every Agent certificate, enrollment and forward link certificate of this node is revoked. A running Agent loses its connection at its next heartbeat.',
      issue: 'A new one-time credential is issued and shown once. An earlier unused credential stops working.',
      offline: 'Nothing contacts the node, so this works while it is offline: its Agent finds out when it next connects.',
      reason: 'Reason',
      reasonHelp: 'Optional, recorded in the audit log. {count} / {max} bytes.',
      reasonTooLong: 'The reason is too long: {count} of {max} bytes.',
      reasonPlaceholder: 'For example: the host’s disk was stolen',
      ttl: 'Credential lifetime',
      rotateKey: 'Also replace the node’s API key',
      rotateKeyHelp: 'Without this the old API key keeps working, so whoever holds it can still enroll an Agent by itself. Replacing it breaks every process that still polls Control with the old key until it gets the new one (read it under Credentials).',
      rotateKeyNotice: 'The old API key stops working as soon as you confirm. Read the new key under Credentials afterwards.'
    },
    result: {
      title: 'New credential for {name}',
      description: 'Shown once. It can’t be shown again after you close this dialog.',
      warning: 'Treat it like a password. Until it is used or expires, anyone who has it can enroll an Agent as this node: don’t share it, and don’t paste it into chats, tickets or files.',
      label: 'One-time credential',
      copy: 'Copy credential',
      expires: 'Valid until {time}',
      remaining: 'in {remaining}',
      expired: 'This credential has expired. Rotate again to get a new one.',
      single: 'It works once.',
      revoked: 'Revoked',
      certificates: 'Agent certificates',
      enrollments: 'Enrollments',
      linkCertificates: 'Forward link certificates',
      apiKeyReplaced: 'The node’s API key was replaced. Read the new key under Credentials.',
      apiKeyKept: 'The node’s API key was not changed: the old key still works.',
      next: 'Give this credential to the machine and enroll the Agent again. The guide has the steps for a node that already has an identity.',
      guide: 'Rotating a node’s credentials (guide)',
      done: 'Done'
    },
    errors: {
      super_admin_required: 'Only super administrators can rotate credentials.',
      node_not_found: 'This node no longer exists.',
      node_disabled: 'This node is disabled. Enable it first.',
      agent_pki_disabled: 'The built-in Agent CA is off (module_runtime.ca_kek), so Control can’t issue credentials.',
      failed: 'Couldn’t rotate the credentials: {message}'
    }
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
    iwan: {
      title: 'Panabit iWAN',
      unencrypted: 'Not encrypted',
      unencryptedHint: 'iWAN has no encryption or integrity protection: devices on the path can read and change the traffic. Use it only on links you trust (spec §9).',
      fields: {
        pool: 'Address pool',
        mtu: 'MTU',
        dnsUpstream: 'DNS upstream',
        allowEncrypt: 'Allow XOR masking',
        allowClientToClient: 'Allow user-to-user traffic',
        fixedAddresses: 'Fixed addresses'
      },
      hints: {
        pool: 'IPv4 network (/8 to /30); its first address is the gateway.',
        dnsUpstream: 'Where the gateway relays DNS: an IP or IP:port.',
        allowEncrypt: 'XOR only hides traffic from casual inspection; it is not encryption.',
        fixedAddresses: 'One "user id=address" per line, inside the pool and not the gateway.',
        login: 'Users log in with their panel user id and the first 16 hex digits of their UUID; the iWAN format on the subscription page gives them an iwand configuration.'
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
  tabs: {
    label: 'Node sections',
    nodes: 'Nodes',
    groups: 'Node groups',
    hosts: 'Forward hosts'
  },
  // 角色 of a regular node (docs/design/node-unification).
  roles: {
    exit: 'Exit',
    relay: 'Relay',
    hops: '{n} hops',
    unconvertedBanner: '{count} old forward nodes are not regular nodes yet: once converted they are listed here and can carry ANX relays.'
  },
  // Join a node: one entry for a new machine (docs/design/node-onboarding).
  join: {
    title: 'Join a node',
    description: 'Run one command on the machine to join. It becomes a node of the node group you pick.',
    checking: 'Checking that this Control can take nodes…',
    loading: 'Getting ready…',
    loadFailed: 'Couldn’t get ready: {message}',
    retry: 'Try again',
    recheck: 'Check again',
    group: 'Node group',
    newGroup: 'New node group',
    requiresApproval: 'New nodes wait for your approval',
    defaultCreated: 'The default node group “{name}” was created for you: one VLESS Reality protocol on a random port, in the default subscription group. Edit it later under Node groups.',
    blocked: {
      title: 'Nodes can’t join yet',
      description: 'Fix these first. A command copied now would fail on the machine.'
    },
    checks: {
      public_url: 'Control’s address',
      panel_certificate: 'Panel certificate',
      grpc: 'gRPC address',
      grpc_certificate: 'gRPC certificate',
      ca: 'Key that signs node certificates',
      mtls_mode: 'Agent authentication',
      agent_release: 'Agent release',
      script_signature: 'Install script signature'
    },
    fixes: {
      public_url: 'Set agent_install.public_url to the https address nodes reach Control at (for example https://panel.example.com) and restart Control. Behind a reverse proxy, also list the proxy in server.trusted_proxies.',
      panel_certificate: 'Nodes download the install script over https and refuse an untrusted certificate. The banner at the top says why the certificate is not ready; fix that, then use Request again there (or anix-control tls retry on the server).',
      grpc: 'Set domain (it turns the gRPC listener on), or grpc.enabled. When nodes dial a different address than Control’s own, set agent_install.grpc_target. Then restart Control.',
      grpc_certificate: 'Nodes verify the gRPC certificate against their system CAs and refuse anything else. Set domain (and, behind a CDN, grpc.public_name to a DNS-only name) so Control obtains one; the banner at the top says why it is not ready yet.',
      ca: 'Set module_runtime.ca_kek (environment variable ANIX_CONTROL_MODULE_RUNTIME_CA_KEK) to a 32-byte key, for example from openssl rand -base64 32, and restart Control.',
      mtls_mode: 'Remove agent_control.mtls (required is the default) or set it to required, and restart Control.',
      agent_release: 'Set agent_install.agent_version to a published Agent release (for example v4.2.0), or put the Agent release in agent_install.artifact_dir, and restart Control.',
      script_signature: 'Point agent_install.signature_file at this version’s agent-install.sh.sig, or leave it empty.'
    },
    command: {
      title: 'Install command',
      mirror: 'Download source',
      mirrors: {
        control: 'Control',
        cn: 'Mainland mirror',
        github: 'GitHub',
        controlHelp: 'The script and the Agent are downloaded from this Control.',
        cnHelp: 'The script comes from this Control and the Agent from the mainland mirror; checksums always come from Control or GitHub.',
        githubHelp: 'The script and the Agent come from GitHub releases; the node still joins this Control.'
      },
      commandLabel: 'Run on the machine to join, as root',
      copy: 'Copy command',
      fallback: 'This download source is not set up: {note}',
      expires: 'The token ends {time}. The same command joins any number of machines.',
      neverExpires: 'The token does not expire. The same command joins any number of machines.',
      expired: 'The token has expired: set a new validity under Node groups › Edit, or reset the token, to join more machines.',
      history: 'The command contains the token; it stays in the shell history of every machine it ran on.',
      reset: 'Reset token',
      resetConfirmTitle: 'Reset the token of “{name}”?',
      resetConfirmMessage: 'Every command copied before stops working. Nodes that already joined are not affected.',
      resetDone: 'Token reset. Use the new command.',
      uninstall: 'Uninstall command',
      uninstallHelp: 'Run it on the node. Add --purge to remove its identity and data too.',
      forbidden: 'Only a super administrator can see the join command. Sign in as one.',
      failed: 'Couldn’t read the join command: {message}'
    },
    nodes: {
      title: 'Machines that joined',
      waiting: 'Waiting for a machine to join…',
      waitingHint: 'Run the command above on the machine and it shows up here.',
      joined: '{name} joined',
      online: 'Online',
      connecting: 'Connecting',
      more: '{count} more that joined earlier are in the node list.'
    },
    quick: {
      name: 'Node group name',
      namePlaceholder: 'Hong Kong',
      kind: 'Node type',
      kindHelp: {
        proxy: 'Regular nodes serve the protocols in subscriptions.',
        forward: 'Relays are regular nodes without protocols: the entry or relay machines of forward routes (ANX relays).'
      },
      preset: 'Protocols',
      presets: {
        'vless-reality': 'VLESS Reality (recommended)',
        'shadowsocks-2022': 'Shadowsocks 2022',
        'vless-reality-shadowsocks': 'VLESS Reality + Shadowsocks 2022'
      },
      subscriptionGroups: 'Subscription groups',
      create: 'Create node group',
      failed: 'Couldn’t create the node group: {message}'
    }
  },
  approval: {
    pending: 'Awaiting approval',
    pendingHint: 'This node waits for you: it gets no configuration and no users until you approve it.',
    approved: 'Approved',
    rejected: 'Rejected',
    approve: 'Approve',
    reject: 'Reject',
    banner: '{count} machine(s) wait for your approval. They get no configuration and no users until you approve them.',
    view: 'View',
    rejectTitle: 'Reject node {name}?',
    rejectMessage: 'The node is disabled and its certificates are revoked.',
    approvedToast: '{name} approved',
    rejectedToast: '{name} rejected',
    failed: 'Couldn’t do that: {message}'
  },
  // Node groups (the tab and its sheet).
  groups: {
    summary: 'A node group sets the defaults, the protocols and the join token of its nodes.',
    add: 'New node group',
    kinds: {
      proxy: 'Regular',
      forward: 'Relay'
    },
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
    tokenExpiredEdit: 'This group’s token has expired. Set a new validity under Edit first.',
    approvalOn: 'New nodes of “{name}” now need your approval.',
    approvalOff: 'New nodes of “{name}” no longer need approval.',
    empty: 'No node groups yet',
    emptyDescription: 'Choose Join a node: Control makes the default node group and shows the command.',
    loadFailed: 'Couldn’t load node groups',
    created: 'Node group {name} created',
    saved: 'Node group {name} saved',
    deleted: 'Node group {name} deleted',
    deleteTitle: 'Delete node group {name}?',
    deleteMessage: 'Its join token stops working at once. Nodes that already joined are not affected.',
    deleteAction: 'Delete node group',
    deleteInUse: 'The group still has nodes: delete or move them first.',
    nameTaken: 'A node group with this name exists.',
    caKekMissing: 'module_runtime.ca_kek is not set, so no node group can be created.'
  },
  groupForm: {
    titleCreate: 'New node group',
    titleEdit: 'Edit node group',
    createDescription: 'The group’s settings are copied to every node that joins it.',
    create: 'Create node group',
    save: 'Save changes',
    saveFailed: 'Couldn’t save: {message}',
    joining: 'Joining',
    noSubscriptionGroups: 'There are no subscription groups yet. Create one under Subscription groups so nodes can appear in subscriptions.',
    fields: {
      name: 'Name',
      kind: 'Type',
      rate: 'Traffic rate',
      sort: 'Sort',
      tags: 'Tags',
      monthlyLimit: 'Monthly traffic limit',
      resetDay: 'Monthly reset day',
      tokenExpiry: 'Token validity',
      maxNodes: 'Node limit',
      requireApproval: 'New nodes need my approval',
      subscriptionGroups: 'Subscription groups'
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
      requireApproval: 'Until you approve it, a node gets no configuration and no users.',
      subscriptionGroups: 'The protocols of new nodes join these subscription groups. Nodes that already joined stay as they are.'
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
    },
    template: {
      title: 'Protocols',
      description: 'Every new node gets these protocols, with secrets Control generates for each node. A change applies to machines that join afterwards; nodes already there keep theirs.',
      empty: 'No protocols: a node that joins would have nothing to serve.',
      protocol: 'Protocol {n}',
      name: 'Name',
      namePlaceholder: 'Defaults to the protocol',
      type: 'Protocol',
      types: {
        vless: 'VLESS',
        vmess: 'VMess',
        shadowsocks: 'Shadowsocks',
        wireguard: 'WireGuard'
      },
      transport: 'Transport',
      tls: 'TLS',
      portMode: 'Port',
      portModes: {
        range: 'Random port',
        fixed: 'Fixed port'
      },
      port: 'Port',
      portFrom: 'Port from',
      portTo: 'to',
      serverName: 'Reality site',
      serverNameHelp: 'A site that answers TLS 1.3.',
      dest: 'Reality destination',
      destHelp: 'Empty means site:443.',
      advanced: 'Advanced',
      settings: 'Protocol settings (JSON)',
      transportSettings: 'Transport settings (JSON)',
      add: 'Add protocol',
      remove: 'Remove',
      fixProblems: 'Fix the protocol settings marked below first.',
      portErrors: {
        port: 'A port from 1 to 65535.',
        range: 'Ports from 1 to 65535, the start not above the end.'
      },
      jsonErrors: {
        json: 'Not valid JSON.',
        object: 'Must be a JSON object.'
      }
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
    saveFailed: 'Couldn’t save: {message}',
    syncSuccess: 'Node “{name}” accepted the sync',
    syncFailed: 'Sync failed: {message}',
    nodeSaved: 'Node {name} saved',
    nodeDeleted: 'Node {name} deleted',
    nodeDisabled: 'Node {name} disabled',
    nodeEnabled: 'Node {name} enabled',
    protocolCreated: 'Protocol {type} :{port} added',
    protocolSaved: 'Protocol {type} :{port} saved',
    protocolDeleted: 'Protocol {type} :{port} deleted',
    generateFailed: 'Couldn’t generate: {message}',
    copied: 'Copied',
    copyFailedManual: 'Couldn’t copy automatically. Select the text and copy it.',
    invalidJsonDetail: 'Invalid JSON: {message}',
    quotaExceededBanner: 'Over their monthly quota on this page: {count}. Nodes are only flagged; nothing is limited automatically.'
  }
}
