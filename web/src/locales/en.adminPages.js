// Messages for the admin pages other than the dashboard (src/views/admin
// and the forward suite). src/i18n.js loads them before the first such
// page opens (router/index.js), so an admin's first visit, the
// dashboard, does not download them. The shell's messages are in
// src/locales/en.admin.js; the two groups share no message key.
import runtimePages from './modules/en/runtimePages'
import networkPages from './modules/en/networkPages'
import miscPages from './modules/en/miscPages'
import adminSupportPages from './modules/en/adminSupportPages'
import adminNodes from './modules/en/adminNodes'
import forwardNodesPage from './modules/en/forwardNodesPage'
import adminSubscriptionGroups from './modules/en/adminSubscriptionGroups'

export default {
  ...adminSubscriptionGroups,
  ...runtimePages,
  ...networkPages,
  ...miscPages,
  ...adminSupportPages,
  ...forwardNodesPage,
  forwardWizard: {
    title: 'Forward Setup Wizard',
    subtitle: 'Create a node, tunnel and forward step by step without jumping between pages',
    loading: 'Loading runtime mode…',
    stepsLabel: 'Setup steps',
    stepCount: 'Step {current} of {total}',
    stepDone: ' (done)',
    back: 'Back',
    next: 'Next',
    shared: {
      existingLabel: 'Existing records you can reuse',
      useExisting: 'Use existing'
    },
    steps: {
      mode: {
        title: 'Choose forward mode',
        chooseLabel: 'Forward mode',
        intro: 'Pick a forward mode — the runtime backend and tunnel type are configured automatically, so you don\'t need to understand either concept separately.',
        cards: {
          local: {
            label: 'Local port forward',
            description: 'Forward directly on an Ansible-managed machine, no relay node needed.'
          },
          gostSingle: {
            label: 'Relay · single-node forward',
            description: 'Forward through one NodeX node, without protocol wrapping.'
          },
          gostTunnel: {
            label: 'Relay · tunnel forward',
            description: 'Forward across ingress/egress nodes, with protocol obfuscation (tls/ws/grpc, etc).'
          }
        },
        currentBadge: 'Currently active',
        nodeXSetupHint: 'First time using relay forwarding requires NodeX control-plane details before continuing.',
        confirmAndContinue: 'Confirm and continue'
      },
      machine: {
        title: 'Machine or node',
        intro: 'Register an execution machine or node first — the tunnel step will use it.',
        createAndContinue: 'Create and continue'
      },
      node: {
        intro: 'Register a NodeX node first — the tunnel step will use it.',
        createAndContinue: 'Create and continue',
        tokenNotice: 'This is the API token of node "{name}". It is shown only once: copy it now.'
      },
      tunnel: {
        title: 'Tunnel',
        intro: 'Create a tunnel based on the node from the previous step. A tunnel is required for forwards.',
        inheritedNodeHint: 'The node from the previous step is filled in automatically.',
        createAndContinue: 'Create and continue'
      },
      forward: {
        title: 'Forward',
        intro: 'Create the actual forward entry based on the tunnel from the previous step. Just fill in the remote address.',
        inheritedTunnelHint: 'The tunnel from the previous step is filled in automatically.',
        createAndFinish: 'Create and finish'
      },
      done: {
        title: 'Done',
        summary: 'Forward "{name}" was created successfully — the chain is now live.',
        nextTitle: 'What next',
        gotoForward: 'Go to forward management',
        gotoTunnel: 'Go to tunnel management',
        gotoNode: 'Go to machine/node management',
        createAnother: 'Create another forward'
      }
    }
  },
  runtime: {
    shared: {
      panelConfig: 'Panel Config',
      reachability: 'Reachability',
      runtimeReady: 'Runtime Ready',
      executor: 'Executor',
      files: 'Files',
      warnings: 'Warnings',
      powerShell: 'PowerShell',
      bash: 'Bash',
      bootstrapVerify: 'Bootstrap / Verify',
      references: 'References',
      doctorOutput: 'Doctor Output',
      doctorNotExecuted: 'Doctor has not been executed yet.',
      refreshStatus: 'Refresh status',
      runDoctor: 'Run doctor',
      runningDoctor: 'Running...',
      loading: 'Loading...',
      yes: 'Yes',
      no: 'No',
      present: 'Present',
      missing: 'Missing',
      reachable: 'Reachable',
      notReady: 'Not ready',
      ready: 'Ready',
      unavailable: 'Unavailable',
      pending: 'Pending',
      running: 'Running',
      success: 'Success',
      failed: 'Failed',
      unknown: 'Unknown',
      online: 'Online',
      offline: 'Offline',
      enabled: 'Enabled',
      disabled: 'Disabled'
    },
    localRuntime: {
      heroTextPrimary: 'This page owns the panel-host Ansible executor only. It is the stateless runtime path for panel-side forwarding and does not require a persistent NodeX control-plane or Node-Agent connection.',
      heroTextSecondary: 'The backend is nftables / Ansible — a stateless control path for panel-side forwarding.',
      saveActivate: 'Save And Activate Local Runtime',
      activeBannerTitle: 'Local runtime is active',
      standbyBannerTitle: 'Local runtime is configured as standby',
      activeBannerText: 'Forward jobs currently use {backend}. SSH transport and privilege escalation are resolved from this Ansible runtime config.',
      standbyBannerText: 'NodeX/gost remains active globally. You can still stage and validate the local Ansible runtime here before switching back.',
      configTitle: 'Panel-Host Ansible Executor',
      configCopy: 'Ansible mode is stateless: the panel stores execution-node identity on tunnel and forward records, while inventory, playbooks, sudo and SSH behavior live here.',
      recommended: 'Recommended',
      legacy: 'Legacy',
      executorEyebrow: 'Executor',
      defaultsAction: 'Use backend defaults',
      executorHint: 'Saving here keeps {backend} active by writing `forward.runtime_backend={backendKey}`, `forward.runtime.ansible.backend={backendKey}` and `forward.runtime.nodex_mode=false`.',
      fields: {
        inventory: 'Inventory',
        applyPlaybook: 'Apply playbook',
        removePlaybook: 'Remove playbook',
        command: 'Command',
        workingDir: 'Working dir',
        targetPattern: 'Target pattern',
        timeoutSeconds: 'Timeout (seconds)',
        ansibleConfig: 'ANSIBLE_CONFIG',
        useBecome: 'Use sudo / become on the execution node',
        extraVarsJson: 'Extra vars JSON',
        environmentJson: 'Environment JSON',
        generatedJson: 'Generated runtime JSON'
      },
      extraVarsHint: 'Backend-specific fields such as firewall driver are injected automatically by the backend.',
      environmentHint: 'Extra process environment variables for the panel-host executor.',
      generatedHint: 'The JSON payload is generated from the structured fields above and stored in `forward.runtime.ansible.config`.',
      probeTitle: 'Executor Reachability And Runtime Readiness',
      noStatus: 'No local runtime status loaded yet.',
      cards: {
        localActiveValue: 'Local runtime active',
        standbyValue: 'Standby config',
        backend: 'Backend',
        preferredLocalBackend: 'Preferred local backend',
        attachment: 'Attachment',
        runtimeReady: 'Runtime ready',
        firewallDriver: 'Firewall driver',
        commandFound: 'Command found',
        become: 'Become',
        inventory: 'Inventory',
        applyPlaybook: 'Apply playbook',
        removePlaybook: 'Remove playbook',
        workingDir: 'Working dir'
      },
      latestJobs: 'Latest {backend} Jobs',
      noJobs: 'No local runtime jobs yet.',
      backends: {
        nftables: {
          label: 'nftables / Ansible',
          description: 'Modern Linux hosts should prefer nftables.'
        }
      },
      errors: {
        savedConfigInvalid: 'Saved local runtime config is invalid. Defaults were loaded; save again to repair it.',
        invalidJson: '{label} must be valid JSON',
        invalidObject: '{label} must be a JSON object',
        invalidPreview: 'Invalid runtime config: {message}',
        invalidRuntimeJson: 'Local runtime JSON is invalid',
        saveFailed: 'Failed to save local runtime config',
        fetchStatusFailed: 'Failed to fetch local runtime status',
        doctorFailed: 'Local runtime doctor failed'
      }
    },
    nodeX: {
      heroTextPrimary: 'Dedicated operator entry for the stateful NodeX/gost path. This page probes the configured NodeX control plane directly, even when the current global runtime backend is still a local Ansible backend.',
      heroTextSecondary: 'Local Ansible execution now lives under Local Runtime and Ansible Machines. Node "online" still means TCP reachability only and is not proof that NodeX or the relay gost API is already attached.',
      saveLoading: 'Saving...',
      save: 'Save NodeX Config',
      enabledBannerTitle: 'NodeX Mode is enabled',
      disabledBannerTitle: 'NodeX Mode is disabled',
      enabledBannerText: 'Panel forward jobs can route through NodeX/gost, but each runtime job still has to succeed before relay attachment is real.',
      disabledBannerText: 'You can validate the configured NodeX control plane here first, then switch the global backend when you are ready.',
      configTitle: 'NodeX Control Plane',
      enableModeTitle: 'Enable NodeX Mode',
      enableModeHint: 'Writes `forward.runtime.nodex_mode=true` and `forward.runtime_backend=gost`.',
      fields: {
        baseUrl: 'NodeX Base URL',
        baseUrlHint: 'This must point to the NodeX control-plane, not directly to the relay gost API.',
        token: 'NodeX Token',
        tokenHint: 'Matches the NodeX control-plane `--forward-api-token` value.',
        timeout: 'Timeout (seconds)',
        timeoutHint: 'Used by the panel when probing or executing NodeX runtime requests.'
      },
      probeTitle: 'Health And Runtime Status',
      probeCopy: 'These checks always target the configured NodeX control plane. They do not depend on the currently active runtime backend.',
      noStatus: 'No NodeX runtime status loaded yet.',
      cards: {
        modeOn: 'NodeX mode on',
        modeOff: 'NodeX mode off',
        backend: 'Backend',
        baseUrl: 'Base URL',
        tokenConfigured: 'Token configured',
        timeout: 'Timeout',
        health: 'Health',
        http: 'HTTP',
        version: 'Version',
        executePath: 'Execute path'
      },
      jobsTitle: 'Latest gost Jobs',
      jobsCopy: 'Recent panel-side runtime audit rows filtered to the `gost` backend.',
      noJobs: 'No gost runtime jobs yet.',
      backends: {
        gost: 'gost / NodeX'
      },
      errors: {
        baseUrlRequired: 'NodeX base URL is required when NodeX mode is enabled',
        tokenRequired: 'NodeX token is required when NodeX mode is enabled',
        saveFailed: 'Failed to save NodeX config',
        fetchStatusFailed: 'Failed to fetch NodeX runtime status',
        doctorFailed: 'NodeX runtime doctor failed'
      }
    },
    ansibleMachines: {
      heroText: 'This page is only for stateless Ansible execution machines. These hosts do not need Node-Agent and do not need a persistent control-plane connection.',
      addMachine: 'Add machine',
      table: {
        reachability: 'Reachability',
        lastResult: 'Last result'
      },
      stats: {
        machines: 'Machines',
        online: 'Online',
        enabled: 'Enabled'
      },
      sectionTitle: 'Execution Targets',
      sectionCopy: 'These records only identify hosts for the local Ansible runtime. They are not NodeX control-plane nodes.',
      inventoryHint: 'SSH user, password, and private key are not stored on this page. Define them in Ansible inventory, playbooks, or Local Runtime environment settings.',
      filterLabel: 'Status',
      filters: {
        online: 'Online',
        offline: 'Offline'
      },
      empty: 'No Ansible execution machines yet',
      meta: {
        regionIsp: 'Region / ISP',
        currentConn: 'Current Conn',
        traffic: 'Traffic'
      },
      actions: {
        edit: 'Edit',
        check: 'Health Check',
        checking: 'Checking...',
        sync: 'Sync Stats',
        syncing: 'Syncing...',
        disable: 'Disable',
        enable: 'Enable',
        delete: 'Delete',
        updating: 'Updating {name}…'
      },
      modal: {
        titleEdit: 'Edit Ansible Machine',
        titleAdd: 'Add Ansible Machine',
        deleteTitle: 'Delete Ansible machine {name}?',
        deleteConfirm: 'It is removed from the Ansible execution fleet. This can’t be undone.',
        deleteAction: 'Delete machine',
        saveLoading: 'Saving...',
        save: 'Save',
        cancel: 'Cancel'
      },
      messages: {
        saved: '{name} saved',
        deleted: '{name} deleted'
      },
      fields: {
        name: 'Name',
        host: 'Host',
        reachabilityPort: 'Reachability Port',
        reachabilityHelp: 'Control only checks that host:port accepts TCP connections.',
        weight: 'Weight',
        region: 'Region',
        isp: 'ISP'
      },
      placeholders: {
        name: 'relay-exec-01',
        host: '1.2.3.4',
        region: 'HK / JP / US',
        isp: 'CMI / NTT / Cogent'
      },
      results: {
        latency: 'Latency {value} ms',
        reachable: 'Machine reachable',
        unavailable: 'Machine unavailable',
        synced: 'Stats synced'
      },
      errors: {
        required: 'Name, host and reachability port are required.',
        saveFailed: 'Failed to save machine',
        loadFailed: 'Failed to load Ansible machines',
        detailFailed: 'Failed to load machine details',
        deleteFailed: 'Failed to delete machine',
        checkFailed: 'Health check failed',
        syncFailed: 'Failed to sync machine stats',
        toggleFailed: 'Failed to change machine status'
      }
    },
    forward: {
      title: 'Forward Management',
      modeLabelNodeX: 'Active Runtime: NodeX / gost',
      modeLabelLocal: 'Active Runtime: Local / {backend}',
      modeSummaryNodeX: 'Edit NodeX control-plane URL, token and gost operator checks on the dedicated NodeX Runtime page.',
      modeSummaryLocal: 'Edit inventory, playbooks and panel-host executor settings on the dedicated Local Runtime page.',
      modeCompatibilityHint: 'Forward editor options are filtered by the currently active runtime. Local Ansible runtime only accepts Port Forward tunnels, while NodeX/gost can attach both compatible port-forward and tunnel-forward layouts.',
      nftablesHint: 'The runtime is nftables / Ansible: Primary / Backup and Hash send traffic only to the first target, and speed limits are not enforced. Use Round Robin or Random to balance across targets, or the NodeX runtime for speed limits.',
      nftablesStrategyHint: 'On the nftables / Ansible runtime, Primary / Backup and Hash use only the first target.',
      modeHintNodeX: 'NodeX/gost mode keeps ingress and exit semantics. A selected tunnel still requires NodeX runtime jobs to succeed before forwarding is really attached.',
      modeHintLocal: 'Local Ansible mode only records the execution node. SSH access comes from the configured ansible inventory and local runtime settings, not from NodeX topology records.',
      tunnelHintNodeX: '{name} will be attached through NodeX/gost. Panel-side "online" or status checks do not prove the remote relay has finished attaching.',
      tunnelHintLocal: '{name} will be applied on the execution node only. This path stays stateless until the queued ansible job finishes successfully.',
      tunnelHintLocalIncompatible: '{name} is a Tunnel Forward tunnel and requires NodeX/gost. Local Ansible runtime cannot attach it directly.',
      portRange: 'Allowed range: {start} - {end}',
      portHintNodeX: 'Leaving the port empty lets the panel allocate one from the tunnel entry-node range.',
      portHintLocal: 'Leaving the port empty lets the panel allocate one on the selected execution node.',
      runtimeLinks: 'Runtime pages',
      loading: 'Loading forwards and tunnels...',
      view: {
        label: 'View',
        directLabel: 'Direct',
        groupedLabel: 'Grouped'
      },
      actions: {
        import: 'Import',
        export: 'Export',
        add: 'Create',
        edit: 'Edit',
        diagnose: 'Diagnose',
        delete: 'Delete',
        copyAll: 'Copy All',
        more: 'More actions',
        moveUp: 'Move up',
        moveDown: 'Move down'
      },
      bulk: {
        resume: 'Resume',
        pause: 'Pause',
        export: 'Export',
        delete: 'Delete'
      },
      filters: {
        search: 'Search',
        searchPlaceholder: 'Rule, tunnel, user, address or port',
        tunnel: 'Tunnel',
        allTunnels: 'All tunnels',
        status: 'Status',
        running: 'Running',
        paused: 'Paused',
        error: 'Error'
      },
      table: {
        label: 'Forward rules',
        ruleName: 'Rule',
        tunnel: 'Tunnel',
        ingress: 'Ingress',
        target: 'Target',
        policy: 'Policy',
        status: 'Status',
        traffic: 'Traffic',
        copyAddress: 'Copy {title}: {address}',
        toggleService: 'Forwarding service of {name}'
      },
      group: {
        userTag: 'User',
        summary: '{tunnels} tunnels, {forwards} forwards',
        tunnelMeta: 'Tunnel #{id}',
        runningCount: '{running} of {total} running'
      },
      emptyGroupedTitle: 'No forwards yet',
      emptyGroupedText: 'There are no flux-panel-compatible forward records in the current system yet.',
      emptyDirectTitle: 'No forwards yet',
      emptyDirectText: 'After you create the first forward, the direct table view will appear here.',
      editor: {
        titleEdit: 'Edit Forward',
        titleAdd: 'Create Forward',
        description: 'Traffic enters through the tunnel and goes to the targets below.',
        fields: {
          name: 'Forward Name',
          tunnel: 'Tunnel',
          ingressPort: 'Ingress Port',
          interfaceName: 'Interface Name',
          remoteAddress: 'Target Address',
          strategy: 'Scheduling Strategy'
        },
        placeholders: {
          name: 'e.g. HK-Web-01',
          tunnel: 'Please select a tunnel',
          ingressPort: 'Leave blank to auto-allocate',
          interfaceName: 'Optional, e.g. eth0',
          remoteAddress: 'One target per line, for example:\n1.1.1.1:443\nexample.com:8443\n[2001:db8::1]:443'
        },
        remoteHint: 'Supports IPv4:port, domain:port, or [full IPv6]:port. Use one address per line for multiple targets.',
        submitUpdate: 'Save Changes',
        submitCreate: 'Create Forward'
      },
      deleteModal: {
        confirmText: 'Delete forward {name}?',
        hint: 'If the regular delete fails, you are asked whether to force delete. This can’t be undone.',
        confirmDelete: 'Delete forward',
        forceDeleteTitle: 'Force delete {name}?',
        forceDeleteAction: 'Force delete',
        forceDeleteMessage: 'The regular delete failed ({message}). A force delete does not check that the forward service was removed from the node.'
      },
      addressModal: {
        copy: 'Copy',
        titleWithCount: '{title} ({count})'
      },
      exportModal: {
        title: 'Export Forward Data',
        subtitle: 'Format: relay-panel compatible JSON: {\'[{ "dest": ["host:port"], "listen_port": 10086, "name": "Rule" }\'}]',
        tunnelLabel: 'Select Export Tunnel',
        tunnelPlaceholder: 'Please select a tunnel',
        regenerate: 'Regenerate',
        generate: 'Generate Export Data',
        selectionHint: 'Exporting {count} selected forwards.',
        dataLabel: 'Export data'
      },
      importModal: {
        title: 'Import Forward Data',
        subtitle: 'Supports relay-panel JSON and legacy remoteAddr{\'|\'}name{\'|\'}inPort lines. inPort may be blank.',
        subtitleSecondary: 'JSON example: {\'[{ "dest": ["3.3.3.3:3", "4.4.4.4:4"], "listen_port": 10086, "name": "Business Entry" }\'}]',
        tunnelLabel: 'Select Import Tunnel',
        tunnelPlaceholder: 'Please select a tunnel',
        dataLabel: 'Import Data',
        placeholder: '{\'[{"dest":["example.com:8080"],"listen_port":10086,"name":"Business Entry"}]\'}',
        resultTitle: 'Import Results',
        resultSummary: 'Success: {success} / Total: {total}',
        statusSuccess: 'Success',
        statusFailed: 'Failed',
        startImport: 'Start Import'
      },
      diagnosis: {
        title: 'Forward Diagnosis Results',
        loading: 'Diagnosing forward connectivity...',
        connectionSuccess: 'Connection Succeeded',
        connectionFailed: 'Connection Failed',
        nodeMeta: '{name} · {node}',
        nodeMeta: '{name} · {node}',
        nodeMeta: '{name} / {node}',
        targetAddress: 'Target Address',
        averageLatency: 'Average Latency',
        packetLoss: 'Packet Loss',
        quality: 'Quality',
        failedFallback: 'Diagnosis Failed',
        emptyTitle: 'No diagnosis data yet',
        emptyText: 'After you run a diagnosis, results aligned with the reference page will appear here.',
        rerun: 'Run Again',
        summary: '{passed} of {total} checks passed'
      },
      status: {
        normal: 'Healthy',
        paused: 'Paused',
        error: 'Error',
        unknown: 'Unknown'
      },
      runtimeStatus: {
        pending: 'Pending Push',
        running: 'Running',
        synced: 'Synced',
        applied: 'Applied',
        failed: 'Sync Failed',
        queuedSummary: 'Runtime job has been queued and is waiting for the executor to finish.',
        runningSummary: 'Runtime job is running.'
      },
      strategy: {
        fifo: 'Primary / Backup',
        round: 'Round Robin',
        rand: 'Random',
        hash: 'Hash',
        unknown: 'Unknown'
      },
      quality: {
        unknown: 'Unknown',
        excellent: 'Excellent',
        veryGood: 'Very Good',
        good: 'Good',
        fair: 'Fair',
        poor: 'Poor',
        veryPoor: 'Very Poor'
      },
      labels: {
        inbound: 'In',
        outbound: 'Out'
      },
      references: {
        tunnel: 'Tunnel #{id}',
        node: 'Node #{id}',
        node: 'Node {id}',
        node: 'Node #{id}'
      },
      messages: {
        loadForwardsFailed: 'Failed to load forward list',
        loadTunnelsFailed: 'Failed to load tunnel list',
        loadDataFailed: 'Failed to load data',
        unknownUser: 'Unknown User',
        nameRequired: 'Please enter a forward name',
        nameLength: 'Forward name must be between 2 and 50 characters',
        tunnelRequired: 'Please select an associated tunnel',
        remoteAddrRequired: 'Please enter a remote address',
        remoteAddrLineInvalid: 'Line {line} has an invalid address format',
        portRange: 'Port must be between 1 and 65535',
        portRangeTunnel: 'Port must be within {start}-{end}',
        updated: 'Updated successfully',
        created: 'Created successfully',
        actionFailed: 'Operation failed',
        runtimeBusy: 'A runtime job is still queued or running. Wait for it to finish before trying again.',
        invalidStatus: 'Forward status is abnormal and cannot be operated on.',
        serviceChanged: 'Service change request submitted',
        servicePaused: 'Pause request submitted',
        networkActionFailed: 'Network error, operation failed',
        bulkActionComplete: 'Batch completed: {success} succeeded, {failed} failed',
        bulkDeleteTitle: 'Delete {count} selected forwards?',
        bulkDeleteMessage: 'In batch mode, forwards whose regular delete fails are not force deleted. This can’t be undone.',
        bulkDeleteAction: 'Delete forwards',
        deleted: 'Deleted successfully',
        forceDeleted: 'Force delete succeeded',
        forceDeleteFailed: 'Force delete failed',
        deleteFailed: 'Delete failed',
        diagnosisFailed: 'Diagnosis failed',
        diagnosisProcessingFailed: 'An error occurred during diagnosis',
        diagnosisNetworkFailed: 'Network error, diagnosis failed',
        unableConnectServer: 'Unable to connect to the server',
        contentCopied: '{label} copied',
        copyFailedHttp: 'Copy failed: clipboard requires HTTPS or a reverse proxy',
        selectExportTunnel: 'Please select a tunnel to export',
        noExportData: 'The selected tunnel has no forward data',
        exportFailed: 'Export failed',
        enterImportData: 'Please enter data to import',
        selectImportTunnel: 'Please select a tunnel to import into',
        importCompleted: 'Import completed',
        importFailed: 'An error occurred during import',
        importFormatError: 'Invalid format: target address and forward name are required at minimum',
        importRequiredFields: 'Target address and forward name cannot be empty',
        importAddressInvalid: 'Invalid target address format. Expected host:port, with commas for multiple addresses',
        importPortInvalid: 'Invalid ingress port. It must be a number between 1 and 65535',
        importCreateSuccess: 'Created successfully',
        importCreateFailed: 'Create failed',
        importNetworkCreateFailed: 'Network error, create failed',
        orderSaveFailed: 'Failed to save order: {message}',
        orderSaveRetry: 'Failed to save order, please try again',
        unknownError: 'Unknown error',
        localRuntimeTunnelForwardUnsupported: 'Local Ansible runtime cannot attach Tunnel Forward tunnels. Switch to NodeX Runtime or choose a Port Forward tunnel.'
      },
      card: {
        dragHandleTitle: 'Drag to reorder',
        ingressAddressTitle: 'Ingress address',
        targetAddressTitle: 'Target address',
        targetLabel: 'Target'
      }
    },
    tunnel: {
      note: 'NodeX mode separates ingress and execution nodes; local Ansible mode only needs the execution node mapped in inventory. Tunnel "online" only checks host:port reachability and does not confirm remote attachment or firewall state is already in place.',
      modeLabelNodeX: 'Active Runtime: NodeX / gost',
      modeLabelLocal: 'Active Runtime: Local / {backend}',
      modeSummaryNodeX: 'Ingress and egress semantics are controlled through NodeX/gost. Use NodeX Runtime for the control-plane URL, token and gost readiness.',
      modeSummaryLocal: 'Only the execution node identity is stored here. Use Local Runtime for inventory, playbooks and the panel-host ansible executor.',
      modeCompatibilityHint: 'Tunnels below are evaluated against the currently active runtime. Legacy type-1 tunnels may stay schema-compatible with both runtimes, so use the compatibility badge instead of assuming ownership from stored fields.',
      runtimeLinks: 'Runtime pages',
      emptyTitle: 'No tunnels yet.',
      emptyText: 'Create the required topology first, then add the first tunnel that forwards can reference.',
      actions: {
        add: 'Create tunnel',
        edit: 'Edit',
        diagnose: 'Diagnose',
        delete: 'Delete'
      },
      table: {
        label: 'Tunnels',
        compatibility: 'Runtime compatibility',
        status: 'Status'
      },
      meta: {
        ingressNode: 'Ingress node',
        egressNode: 'Egress node',
        executionNode: 'Execution node',
        flowAccounting: 'Flow accounting',
        trafficRatio: 'Traffic ratio'
      },
      modal: {
        titleEdit: 'Edit tunnel',
        titleAdd: 'Create tunnel',
        deleteConfirmMessage: 'Delete tunnel {name}?',
        deleteHint: 'If forward rules or user grants still use this tunnel, the delete is refused. This can’t be undone.',
        submitUpdate: 'Update',
        submitCreate: 'Create',
        confirmDelete: 'Delete tunnel'
      },
      fields: {
        name: 'Tunnel name',
        tunnelType: 'Tunnel type',
        flowAccounting: 'Flow accounting',
        trafficRatio: 'Traffic ratio',
        ingressNode: 'NodeX ingress node',
        executionNode: 'Execution node',
        tcpListenAddr: 'TCP listen address',
        udpListenAddr: 'UDP listen address',
        interfaceName: 'Egress interface or IP',
        protocol: 'Protocol',
        egressNode: 'NodeX egress node'
      },
      placeholders: {
        name: 'HK-Tunnel-01',
        interfaceName: 'eth0 / 192.0.2.10'
      },
      options: {
        portForward: 'Port forward',
        tunnelForward: 'Tunnel forward',
        oneWayAccounting: 'One-way accounting',
        twoWayAccounting: 'Two-way accounting'
      },
      hints: {
        ingressNode: 'Only NodeX/gost mode uses an ingress node here. This is a forward relay role and stays separate from proxy nodes.',
        executionNode: 'Local Ansible mode only needs the execution node identity. SSH access still comes from the configured inventory and local runtime settings.',
        egressNode: 'Exit nodes are only used by NodeX/gost tunnel forwarding. A successful panel save still needs the runtime job to attach remotely.'
      },
      compatibility: {
        nodeXReady: 'Ready for NodeX runtime',
        nodeXNeedsIngress: 'NodeX runtime needs an ingress node',
        nodeXNeedsEgress: 'NodeX tunnel-forward needs an egress node',
        localReady: 'Ready for Local Runtime',
        localNeedsExecution: 'Local Runtime needs an execution node',
        localOnlyPortForward: 'Local Runtime only supports port-forward tunnels'
      },
      messages: {
        loadListFailed: 'Failed to load tunnel list',
        loadDataFailed: 'Failed to load data',
        created: 'Tunnel created successfully',
        updated: 'Tunnel updated successfully',
        actionFailed: 'Operation failed',
        deleted: 'Tunnel deleted successfully',
        deleteFailed: 'Delete failed',
        diagnosis: 'Tunnel diagnosis',
        diagnosisFailed: 'Diagnosis failed',
        diagnosisRequestFailed: 'Diagnosis request failed'
      },
      diagnosis: {
        title: 'Tunnel Diagnosis Results',
        loading: 'Diagnosing tunnel connectivity...',
        targetAddress: 'Target Address',
        duration: 'Duration',
        emptyTitle: 'No diagnosis results yet',
        emptyText: 'There is currently no diagnosis data to display.',
        rerun: 'Run Again',
        summary: '{passed} of {total} checks passed'
      },
      validation: {
        nameRequired: 'Please enter a tunnel name',
        nameLength: 'Tunnel name must be 2-50 characters',
        typeInvalid: 'Please select a valid tunnel type',
        ingressRequired: 'Please select an ingress node',
        ingressMustRelay: 'Ingress node must be a relay node',
        trafficRatio: 'Traffic ratio must be between 0.1 and 100.0',
        tcpListenRequired: 'Please enter a TCP listen address',
        udpListenRequired: 'Please enter a UDP listen address',
        egressRequired: 'Please select an egress node',
        ingressEgressDifferent: 'Ingress and egress nodes cannot be the same',
        egressMustExit: 'Egress node must be an exit node',
        protocolRequired: 'Please select a protocol',
        executionRequired: 'Please select an execution node',
        executionMustRelay: 'Execution node must be a relay node'
      }
    },
    workbench: {
      actions: {
        refreshJobs: 'Refresh jobs',
        openAnsibleMachines: 'Open Ansible Machines',
        runDoctorActiveRuntime: 'Run doctor on active runtime'
      },
      localCard: {
        title: 'Local Ansible executor',
        description: 'Stateless panel-host execution. Inventory, playbooks and SSH access are managed separately from NodeX.',
        currentState: 'Current state',
        manage: 'Manage Local Runtime / Ansible'
      },
      nodeXCard: {
        title: 'Stateful gost control-plane',
        description: 'Panel talks to the internal NodeX control-plane. Real relay attachment only exists after gost runtime jobs succeed.',
        manage: 'Manage NodeX Runtime'
      },
      state: {
        activeBackend: 'Active',
        standby: 'Standby'
      },
      references: {
        panelRuntimeDoc: 'Panel doc: docs/reference/runtime.md',
        panelRelayOnboarding: 'Panel doc: docs/guide/forward-relay-onboarding.md',
        nodeXRepo: 'NodeX repo: https://github.com/zdwtest/NodeX',
        panelNodeXOnboarding: 'Panel doc: docs/forward-runtime-relay-onboarding.md'
      },
      recentJobsTitle: 'Recent runtime jobs',
      recentJobsSubtitle: 'Latest queued and executed actions across the dedicated Local Runtime and NodeX Runtime pages.',
      noJobs: 'No runtime jobs yet.',
      jobMeta: '{backend} / forward {forwardId} / tunnel {tunnelId} / node {nodeId}',
      doctor: {
        title: 'Active Runtime Snapshot',
        description: 'Reachability only means the control plane or local executor can be contacted. It is not proof that a relay has already attached or that iptables rules already exist.',
        note: 'This workbench only shows the currently active backend. Use the dedicated Local Runtime and NodeX Runtime pages to edit config and run mode-specific probes.',
        summary: 'This workbench consolidates control-plane health, runtime diagnostics and one-click commands across both NodeX/gost and local Ansible execution paths.',
        loadingStatus: 'Fetching forward runtime status...'
      },
      cards: {
        backend: 'Backend',
        nodeXMode: 'NodeX Mode',
        attachment: 'Attachment',
        panelVerdict: 'Panel Verdict',
        nodeXSnapshot: 'NodeX Snapshot',
        baseUrlConfigured: 'Base URL configured',
        runtimeVersion: 'Runtime version',
        localAnsible: 'Local ansible',
        playbooks: 'Playbooks'
      },
      errors: {
        fetchStatusFailed: 'Failed to fetch forward runtime status',
        doctorFailed: 'Forward runtime doctor failed'
      }
    },
    nodeXTopology: {
      emptyTitle: 'No NodeX topology nodes yet.',
      emptyText: 'Create relay / exit nodes here for NodeX mode. If you only need stateless execution, use Ansible Machines.',
      legacyText: 'This block mirrors the `/admin/forward/rules*` compatibility endpoints. It preserves legacy rule behavior but does not define the current primary runtime path for NodeX or Ansible.',
      actions: {
        refresh: 'Refresh',
        testConnection: 'Test Connection',
        addNode: 'Add Node',
        clear: 'Clear',
        addRule: 'Add Rule',
        edit: 'Edit',
        healthCheck: 'Health Check',
        checking: 'Checking...',
        syncStats: 'Sync Stats',
        syncing: 'Syncing...',
        enable: 'Enable',
        disable: 'Disable',
        delete: 'Delete',
        startTest: 'Start Test',
        cancel: 'Cancel',
        saveChanges: 'Save Changes',
        createNode: 'Create Node',
        createRule: 'Create Rule'
      },
      filters: {
        online: 'Online',
        offline: 'Offline',
        userId: 'User ID',
        userIdPlaceholder: 'Filter by User ID',
        relay: 'Relay',
        exit: 'Exit'
      },
      status: {
        enabled: 'Enabled',
        disabled: 'Disabled',
        online: 'Online',
        offline: 'Offline'
      },
      meta: {
        latency: 'Latency',
        currentConnections: 'Current Connections',
        traffic: 'Upload / Download',
        lastCheck: 'Last Check',
        uptime: 'Uptime',
        rateLimit: 'Rate',
        trafficLimit: 'Traffic',
        expire: 'Expire',
        upload: 'Upload',
        download: 'Download',
        connections: 'Connections',
        serviceCount: 'Service Count'
      },
      legacy: {
        title: 'Legacy Port Forward Rules',
        emptyTitle: 'No legacy rules yet',
        emptyText: 'Add rules here if you need compatibility for relay + exit port-level forwarding.'
      },
      nodeModal: {
        titleEdit: 'Edit Relay/Exit Node',
        titleAdd: 'Add Relay/Exit Node',
        loading: 'Loading node detail...',
        saveLoading: 'Saving...',
        fields: {
          name: 'Node Name',
          type: 'Node Type',
          host: 'Host',
          servicePort: 'Service Port',
          apiPort: 'API Port',
          apiToken: 'API Token',
          metricsPort: 'Metrics Port',
          region: 'Region',
          isp: 'ISP',
          bandwidth: 'Bandwidth (Mbps)',
          maxConnections: 'Max Connections',
          weight: 'Weight'
        },
        placeholders: {
          name: 'e.g. relay-hk-01',
          host: '1.2.3.4',
          apiToken: 'Leave blank to auto-generate',
          apiTokenKept: 'Stored token hidden; leave blank to keep it',
          region: 'HK / JP / US',
          isp: 'CMI / NTT / Cogent'
        },
        hints: {
          apiPort: 'Required for NodeX management API health checks, stats sync, and connection tests.',
          metricsPort: 'gost Prometheus /metrics port, used to collect traffic stats. Leave blank to skip collection.',
          apiTokenKept: 'The API token is shown only once, when the node is created. Leave this blank to keep it, or enter a new token to replace it.'
        }
      },
      ruleModal: {
        titleEdit: 'Edit Legacy Rule',
        titleAdd: 'Add Legacy Rule',
        loading: 'Loading rule detail...',
        ownerReadOnlyHint: 'Current backend update API does not allow changing ownership fields. They are read-only in edit mode.',
        fields: {
          name: 'Rule Name',
          protocol: 'Protocol',
          relayNode: 'Ingress Relay Node',
          listenPort: 'Listen Port',
          exitNode: 'Egress Exit Node',
          targetPort: 'Target Port',
          targetHost: 'Target Host',
          userId: 'User ID',
          userGroupId: 'User Group ID',
          speedLimit: 'Speed Limit (KB/s)',
          trafficLimit: 'Traffic Limit (Bytes)',
          expireTime: 'Expire Time',
          remark: 'Remark'
        },
        placeholders: {
          name: 'e.g. tcp-11111-hk',
          relayNode: 'Select Relay Node',
          exitNode: 'Select Exit Node',
          targetHost: '127.0.0.1 or target host at destination side',
          userId: 'Blank means public rule',
          userGroupId: 'Choose either User ID or Group ID',
          remark: 'Record business purpose or maintenance notes'
        }
      },
      connectionModal: {
        title: 'Test Node Connection',
        fields: {
          host: 'Host',
          apiPort: 'API Port',
          apiToken: 'API Token'
        },
        placeholders: {
          host: '127.0.0.1',
          apiToken: 'Leave blank if auth is disabled',
          apiTokenHidden: 'The stored token is hidden; enter it to test'
        },
        success: 'Connection succeeded',
        failed: 'Connection failed'
      },
      deleteModal: {
        titleNode: 'Delete forward node {name}?',
        titleRule: 'Delete forward rule {name}?',
        deleteNode: 'Delete forward node',
        deleteRule: 'Delete rule',
        warning: 'Deletion cannot be automatically reverted. Ensure no active forwarding relationships still depend on it.'
      },
      validation: {
        requestFailed: 'Request failed',
        nodeNameRequired: 'Node name is required',
        nodeHostRequired: 'Host is required',
        nodePortRange: 'Service port must be between 1 and 65535',
        nodeApiPortRequired: 'NodeX management API port is required',
        nodeApiPortRange: 'API port must be between 1 and 65535',
        ruleNameRequired: 'Rule name is required',
        relayNodeRequired: 'Please select an ingress Relay node',
        exitNodeRequired: 'Please select an egress Exit node',
        listenPortRange: 'Listen port must be between 1 and 65535',
        targetHostRequired: 'Target host is required',
        targetPortRange: 'Target port must be between 1 and 65535',
        ownerConflict: 'User ID and User Group ID cannot both be set',
        connectionHostRequired: 'Host is required',
        connectionApiPortRange: 'API port must be between 1 and 65535',
        userIdPositive: 'User ID must be a positive integer'
      },
      messages: {
        loadStatsFailed: 'Failed to load forward statistics',
        loadNodesFailed: 'Failed to load relay/exit nodes',
        loadNodeOptionsFailed: 'Failed to load node options',
        loadRulesFailed: 'Failed to load forward rules',
        loadNodeDetailFailed: 'Failed to load node detail',
        saveNodeFailed: 'Failed to save relay/exit node',
        nodeUpdated: 'Relay/exit node updated',
        nodeCreated: 'Relay/exit node created',
        nodeCreatedWithToken: 'Relay/exit node created. Copy its API token now, it is not shown again: {token}',
        nodeDeleted: 'Relay/exit node deleted',
        nodeCheckFailed: 'Health check failed',
        nodeSyncFailed: 'Sync stats failed',
        nodeToggleFailed: 'Failed to toggle node status',
        loadRuleDetailFailed: 'Failed to load rule detail',
        saveRuleFailed: 'Failed to save forward rule',
        ruleUpdated: 'Forward rule updated',
        ruleCreated: 'Forward rule created',
        ruleDeleted: 'Forward rule deleted',
        ruleToggleFailed: 'Failed to toggle rule status',
        connectionFailed: 'Connection check failed',
        deleteFailed: 'Delete failed',
        nodeReachable: 'Node reachable',
        nodeUnavailable: 'Node unavailable',
        syncSuccess: 'Stats synced',
        connectionSuccess: 'Connection succeeded',
        connectionError: 'Connection failed',
        nodeEnabled: '{name} enabled',
        nodeDisabled: '{name} disabled',
        ruleEnabled: '{name} enabled',
        ruleDisabled: '{name} disabled',
        latency: 'Latency {value} ms',
        currentConnectionsSuffix: ', current connections {value}'
      },
      owner: {
        user: 'User #{id}',
        userGroup: 'User Group #{id}',
        public: 'Public Rule'
      },
      protocols: {
        tcp: 'TCP',
        udp: 'UDP',
        both: 'TCP + UDP'
      },
      labels: {
        none: 'Unlimited',
        neverExpires: 'Never Expires'
      }
    },
    limitPage: {
      heroEyebrow: 'Speed Limit Management',
      title: 'Limits',
      note: 'Speed limit rules are enforced by the active forwarding runtime. Changes may take effect after a short delay.',
      actions: {
        create: 'Create',
        createNow: 'Create Now',
        edit: 'Edit',
        delete: 'Delete'
      },
      runtimeLinks: 'Runtime pages',
      table: {
        label: 'Limit rules',
        status: 'Status'
      },
      empty: {
        title: 'No limit rules yet',
        text: 'No limit rules have been created yet. Use the button above to create the first one.'
      },
      status: {
        active: 'Running',
        error: 'Error'
      },
      cards: {
        speed: 'Speed Limit',
        updatedAt: 'Updated At'
      },
      formModal: {
        titleCreate: 'Create Limit Rule',
        titleEdit: 'Edit Limit Rule',
        fields: {
          name: 'Rule Name',
          speed: 'Speed Limit',
          tunnel: 'Bound Tunnel'
        },
        placeholders: {
          name: 'Enter a limit rule name',
          speed: 'Enter speed limit (Mbps)',
          tunnel: 'Select a tunnel to bind'
        },
        submitCreate: 'Create Rule',
        submitUpdate: 'Save Changes'
      },
      deleteModal: {
        title: 'Delete limit rule {name}?',
        hint: 'This can’t be undone. The rule is removed for good.',
        confirmDelete: 'Delete rule'
      },
      values: {
        unlimited: 'Unlimited',
        tunnelFallback: 'Tunnel #{id}',
        ruleFallback: 'Rule #{id}'
      },
      modeLabelNodeX: 'NodeX Mode',
      modeSummaryNodeX: 'Speed limits are synchronized and enforced by NodeX agents.',
      modeLabelLocal: 'Local Mode',
      modeSummaryLocal: 'Speed limits are applied locally via {backend} runtime.',
      messages: {
        fetchTunnelsFailed: 'Failed to load tunnel list',
        fetchRulesFailed: 'Failed to load limit rules',
        loadFailed: 'Failed to load data',
        nameRequired: 'Rule name is required',
        nameLength: 'Rule name must be between 2 and 50 characters',
        speedInvalid: 'Enter a valid speed limit (>= 1 Mbps)',
        tunnelRequired: 'Please select a tunnel to bind',
        tunnelMissing: 'Tunnel name is missing. Refresh and try again.',
        submitFailed: 'Submission failed',
        deleteFailed: 'Failed to delete limit rule',
        created: 'Limit rule created successfully',
        updated: 'Limit rule updated successfully',
        deleted: 'Limit rule deleted successfully'
      }
    },
    nodeXAgents: {
      title: 'NodeX Agents',
      subtitle: 'Only NodeX mode needs agents. Use this page for agent status, remote terminal, and task delivery.',
      tabs: {
        agents: 'Online Agents',
        terminal: 'Remote Terminal',
        tasks: 'Task History'
      },
      actions: {
        refresh: 'Refresh',
        execute: 'Run',
        send: 'Send',
        cancel: 'Cancel',
        monitor: 'Monitoring',
        openTerminal: 'Open in terminal'
      },
      table: {
        nodeId: 'Node',
        version: 'Version',
        system: 'System',
        lastSeen: 'Last Seen',
        status: 'Status',
        capabilities: 'Capabilities',
        taskId: 'Task ID',
        node: 'Node',
        command: 'Command / Action',
        duration: 'Duration',
        time: 'Time'
      },
      status: {
        online: 'Online',
        offline: 'Offline',
        connected: 'Connected',
        disconnected: 'Disconnected',
        success: 'Success',
        failed: 'Failed'
      },
      empty: {
        agents: 'No agents online',
        agentsDescription: 'An agent appears here when a NodeX node connects to Control.',
        tasks: 'No task history yet',
        tasksDescription: 'Commands you run in the terminal and tasks you send are listed here.'
      },
      terminal: {
        chooseNode: 'Choose node',
        nodeLabel: 'Node #{id}',
        chooseAction: 'Choose action',
        output: 'Terminal output',
        hint: 'Choose an online node and an action, then run it. Only the diagnostic actions Control allows can run.'
      },
      diagnosticActions: {
        service_status: 'Check service status',
        service_restart: 'Restart service',
        log_tail: 'Tail service log'
      },
      fields: {
        service: 'Service',
        lines: 'Lines'
      },
      services: {
        gost: 'GOST'
      },
      taskModal: {
        title: 'Send Task',
        action: 'Action',
        timeoutSeconds: 'Timeout (seconds)'
      },
      hints: {
        monitor: 'View monitoring data for node #{id}'
      },
      messages: {
        fetchFailed: 'Agents didn’t load',
        tasksFetchFailed: 'Task history didn’t load',
        taskIncomplete: 'Please fill in the required fields',
        taskSent: 'Task sent',
        taskSendFailed: 'Send failed: {message}',
        commandError: 'Error: {message}',
        selectActionFirst: 'Choose an action first'
      }
    }
  },
  admin: {
    nodes: adminNodes
  },
  adminPayment: {
    title: 'Payments',
    subtitle: 'Gateways users pay through, every payment, and how much came in.',
    tabs: {
      label: 'Payment sections',
      gateways: 'Gateways',
      records: 'Records',
      stats: 'Stats'
    },
    actions: {
      createGateway: 'New gateway',
      enable: 'Enable',
      disable: 'Disable',
      edit: 'Edit',
      details: 'Details'
    },
    gateways: {
      label: 'Payment gateways',
      table: {
        id: 'ID',
        name: 'Name',
        type: 'Type',
        feeRate: 'Fee Rate',
        minAmount: 'Min Amount',
        maxAmount: 'Max Amount',
        status: 'Status'
      },
      empty: 'No payment gateways yet',
      emptyDescription: 'Add a gateway so users can pay for plans.'
    },
    records: {
      label: 'Payment records',
      filters: {
        status: 'Filter by status',
        type: 'Filter by gateway'
      },
      table: {
        id: 'ID',
        tradeNo: 'Trade No.',
        userId: 'User ID',
        gateway: 'Gateway',
        amount: 'Amount',
        status: 'Status',
        createdAt: 'Created at'
      },
      detail: {
        title: 'Payment Details',
        tradeNo: 'Trade No.',
        amount: 'Amount',
        status: 'Status'
      },
      empty: 'No payments yet',
      emptyDescription: 'Payments appear here when users pay for an order.'
    },
    stats: {
      totalAmount: 'Total Revenue',
      totalOrders: 'Total Orders',
      successOrders: 'Successful Orders',
      successRate: 'Success Rate',
      gatewayDistribution: 'Gateway Distribution',
      empty: 'No gateway statistics yet'
    },
    modal: {
      createTitle: 'New gateway',
      editTitle: 'Edit Gateway',
      fields: {
        name: 'Name',
        type: 'Type',
        feeRate: 'Fee Rate',
        minAmount: 'Min Amount',
        maxAmount: 'Max Amount',
        configJson: 'Configuration (JSON)'
      },
      placeholders: {
        name: 'Gateway name',
        feeRate: 'Example: 0.01 = 1%',
        minAmount: 'Minimum payment amount',
        maxAmount: 'Maximum payment amount',
        configJson: "{'{'}\"app_id\": \"\", \"private_key\": \"\"{'}'}"
      }
    },
    types: {
      alipay: 'Alipay',
      wechat: 'WeChat Pay',
      stripe: 'Stripe',
      usdt: 'USDT',
      epay: 'EPay'
    },
    status: {
      enabled: 'Enabled',
      disabled: 'Disabled',
      pending: 'Pending',
      paid: 'Paid',
      failed: 'Failed',
      refunded: 'Refunded'
    },
    confirm: {
      deleteTitle: 'Delete payment gateway {name}?',
      deleteMessage: 'Users can no longer pay through this gateway. This can’t be undone.',
      deleteAction: 'Delete gateway'
    },
    messages: {
      fetchGatewaysFailed: 'Payment gateways didn’t load',
      fetchRecordsFailed: 'Payment records didn’t load',
      fetchStatsFailed: 'Payment statistics didn’t load',
      invalidConfigJson: 'Configuration JSON is invalid',
      gatewaySaveSuccess: 'Gateway saved successfully',
      gatewaySaveFailed: 'Failed to save gateway: {message}',
      gatewaySaveFailedShort: 'Save failed',
      toggleFailed: 'Failed to change gateway status: {message}',
      toggleFailedShort: 'Operation failed',
      gatewayEnabled: '{name} enabled',
      gatewayDisabled: '{name} disabled',
      gatewayDeleted: 'Gateway {name} deleted',
      deleteFailedShort: 'Delete failed'
    }
  },
  adminMfa: {
    config: {
      enabled: 'Enable multi-factor authentication',
      enabledHelp: 'Once enabled, users can choose to turn on MFA to protect account security.',
      required: 'Require MFA',
      requiredHelp: 'Require all users to enable MFA, otherwise they cannot use the service.',
      methods: 'Supported authentication methods',
      backupCodesCount: 'Recovery code count',
      backupCodesHelp: 'Number of recovery codes generated when users enable MFA.',
      maxAttempts: 'Maximum login attempts',
      maxAttemptsHelp: 'Accounts will be temporarily locked after exceeding the limit.',
      lockoutDuration: 'Lockout duration (minutes)',
      lockoutDurationHelp: 'Lockout duration after login failures exceed the limit.'
    },
    methods: {
      totp: 'TOTP (Google Authenticator / Authy)',
      sms: 'SMS verification code',
      email: 'Email verification code'
    },
    info: {
      totpBody: 'Time-based one-time passwords. Users can scan a QR code with apps such as Google Authenticator or Authy to bind MFA.',
      backupBody: 'When users cannot access their authenticator, they can use recovery codes to log in. Each recovery code can only be used once.',
      lockoutBody: 'Multiple consecutive MFA failures will trigger an account lockout to prevent brute-force attacks.',
      userOpsBody: 'Users turn two-factor authentication on or off and make new recovery codes on their own Account page.'
    },
    messages: {
      fetchFailed: 'Failed to load MFA configuration',
      saveSuccess: 'Saved successfully',
      saveFailed: 'Save failed: {message}',
      saveFailedShort: 'Save failed'
    }
  },
  adminTemplates: {
    title: 'Subscription templates',
    subtitle: 'Free subscription templates: traffic quota, speed and device limits, and the subscription groups they grant. Assign one to a user to apply it.',
    filters: {
      search: 'Search templates'
    },
    table: {
      label: 'Subscription templates'
    },
    actions: {
      create: 'New template'
    },
    empty: {
      title: 'No subscription templates yet',
      description: 'Create a template, then assign it to users to give them traffic and subscription groups.'
    },
    detail: {
      description: 'Template ID {id}'
    },
    labels: {
      noGroupsHint: 'This template grants no subscription groups yet.'
    },
    planModal: {
      createTitle: 'New subscription template',
      editTitle: 'Edit subscription template',
      fields: {
        name: 'Template name'
      },
      placeholders: {
        name: 'Enter template name'
      }
    },
    assignModal: {
      title: 'Assign subscription template'
    },
    groupModal: {
      title: 'Template groups - {name}',
      description: 'Select the subscription groups this template grants.'
    },
    confirm: {
      deleteTitle: 'Delete subscription template {name}?',
      deleteAction: 'Delete template'
    },
    messages: {
      loadFailed: 'Failed to load subscription templates',
      nameRequired: 'Please enter a template name',
      deleted: 'Subscription template {name} deleted',
      groupRemoved: 'Group {group} removed from template {plan}'
    }
  },
  adminPlans: {
    title: 'Plans',
    subtitle: 'What users can buy: traffic quota, speed and device limits, price, and the subscription groups each plan grants.',
    filters: {
      search: 'Search plans'
    },
    table: {
      label: 'Plans',
      name: 'Name',
      transfer: 'Traffic',
      limits: 'Limits',
      monthPrice: 'Monthly price',
      subscriptionGroups: 'Subscription groups'
    },
    actions: {
      create: 'Create plan',
      edit: 'Edit',
      delete: 'Delete',
      assign: 'Assign',
      manageGroups: 'Manage groups',
      removeGroupNamed: 'Remove group {name}'
    },
    empty: {
      title: 'No plans yet',
      description: 'Create a plan so users can buy traffic and get subscription groups.'
    },
    detail: {
      description: 'Plan ID {id}',
      limits: 'Quota and limits',
      actions: 'Actions'
    },
    planModal: {
      createTitle: 'Create Plan',
      editTitle: 'Edit Plan',
      fields: {
        name: 'Plan name',
        transfer: 'Traffic quota',
        speedLimit: 'Speed limit',
        deviceLimit: 'Device limit',
        monthPrice: 'Monthly price (cents)'
      },
      placeholders: {
        name: 'Enter plan name'
      },
      help: {
        zeroUnlimited: '0 means no limit.'
      }
    },
    assignModal: {
      title: 'Assign Plan',
      fields: {
        userId: 'User ID',
        expireAt: 'Expire time (Unix seconds)'
      },
      placeholders: {
        userId: 'Enter user ID'
      },
      help: {
        expireAt: 'Optional. Unix time in seconds.'
      }
    },
    groupModal: {
      title: 'Plan Groups - {name}',
      description: 'Select the subscription groups this plan can access.',
      empty: 'No subscription groups',
      noDescription: 'No description'
    },
    labels: {
      noSpeedLimit: 'No speed limit',
      noDeviceLimit: 'No device limit',
      speedLimitMbps: '{value} Mbps',
      deviceLimitCount: '{value} devices',
      noGroups: 'None',
      noGroupsHint: 'This plan grants no subscription groups yet.'
    },
    confirm: {
      deleteTitle: 'Delete plan {name}?',
      deleteMessage: 'This can’t be undone.',
      deleteAction: 'Delete plan'
    },
    messages: {
      loadFailed: 'Failed to load plans',
      loadGroupsFailed: 'Failed to load subscription groups',
      deleted: 'Plan {name} deleted',
      deleteFailedShort: 'Delete failed',
      nameRequired: 'Please enter a plan name',
      saveFailed: 'Save failed: {message}',
      saveFailedShort: 'Save failed',
      saved: '{name} saved',
      userIdRequired: 'Please enter a user ID',
      assignSuccess: 'Assigned successfully',
      assignFailed: 'Assign failed: {message}',
      assignFailedShort: 'Assign failed',
      toggleGroupFailed: 'Failed to toggle group: {message}',
      toggleGroupFailedShort: 'Failed to toggle group',
      groupRemoved: 'Group {group} removed from plan {plan}',
      removeGroupFailed: 'Failed to remove group: {message}',
      removeGroupFailedShort: 'Remove failed'
    }
  },
  adminUsers: {
    title: 'Users',
    subtitle: 'Accounts, subscriptions and traffic. Select a user to see the details.',
    filters: {
      searchEmail: 'Search by email',
      label: 'Filter by status',
      exhaustedHint: '“Out of traffic” narrows the users on this page only; the server has no such filter.'
    },
    table: {
      label: 'Users',
      id: 'ID',
      email: 'Email',
      plan: 'Plan',
      subscriptionTemplate: 'Subscription template',
      traffic: 'Used / total',
      limits: 'Limits',
      expireAt: 'Expires at',
      status: 'Status',
      createdAt: 'Created at'
    },
    status: {
      active: 'Active',
      expired: 'Expired',
      banned: 'Banned',
      exhausted: 'Out of traffic'
    },
    actions: {
      addUser: 'New user',
      editUser: 'Edit user',
      manageTunnel: 'Manage tunnel grants',
      ban: 'Ban',
      unban: 'Unban',
      resetTraffic: 'Reset traffic',
      copySubscribe: 'Copy subscription link',
      resetSubscribe: 'Reset subscription link',
      viewTraffic: 'Traffic in the last 30 days'
    },
    empty: {
      title: 'No users yet',
      description: 'Add the first user to give them a subscription.'
    },
    detail: {
      description: 'ID {id} · joined {date}',
      subscription: 'Subscription',
      flowReset: 'Traffic resets',
      actions: 'Actions',
      danger: 'Danger zone',
      dangerFooter: 'A new subscription link means every client has to import it again. Reset traffic can’t be undone.'
    },
    editModal: {
      title: 'Edit User',
      fields: {
        email: 'Email',
        balance: 'Balance (cents)',
        transfer: 'Traffic limit (bytes)',
        speedLimit: 'Speed limit (Mbps, 0 for unlimited)',
        deviceLimit: 'Device limit (0 for unlimited)',
        groupId: 'Subscription group',
        expiredAt: 'Expire time (Unix seconds)',
        flowResetTime: 'Flow reset day',
        remark: 'Remark'
      },
      groupOptions: {
        unassigned: 'Unassigned'
      }
    },
    createModal: {
      title: 'New user',
      submit: 'Create user',
      fields: {
        email: 'Email',
        password: 'Password',
        userType: 'User type',
        groupId: 'Subscription group',
        transferEnable: 'Traffic limit (bytes)',
        speedLimit: 'Speed limit (Mbps, 0 for unlimited)',
        deviceLimit: 'Device limit (0 for unlimited)',
        flowResetTime: 'Flow reset day'
      },
      placeholders: {
        email: 'Enter email',
        password: 'Enter password (min 6 chars)',
        transferEnable: 'Leave empty for default 0',
        speedLimit: '0 means no speed limit',
        deviceLimit: '0 means no device limit'
      },
      userTypes: {
        normal: 'Normal user',
        admin: 'Admin'
      }
    },
    tunnelModal: {
      title: 'Tunnel Grants - {email}',
      sections: {
        form: 'New grant',
        editForm: 'Edit grant #{id}',
        list: 'Current grants'
      },
      fields: {
        tunnel: 'Tunnel',
        tunnelReadonlyHint: 'The tunnel can’t be changed while editing a grant.',
        status: 'Status',
        flowQuota: 'Flow quota',
        numQuota: 'Quantity quota',
        expTime: 'Expire time',
        flowResetTime: 'Flow reset time',
        rateLimit: 'Rate limit'
      },
      options: {
        noAssignableTunnel: 'No assignable tunnel',
        selectTunnel: 'Select tunnel',
        selectTunnelFirst: 'Select tunnel first',
        noRateLimitRules: 'No rate limit rules for current tunnel'
      },
      actions: {
        cancelEdit: 'Cancel edit',
        updateGrant: 'Update grant',
        addGrant: 'Add grant'
      },
      table: {
        id: 'ID',
        tunnel: 'Tunnel',
        status: 'Status',
        flow: 'Flow',
        num: 'Quantity',
        expireAt: 'Expires at',
        reset: 'Reset',
        usedFlow: 'Used flow',
        rateLimit: 'Rate limit'
      },
      empty: 'No grants'
    },
    confirm: {
      resetSubscribeTitle: 'Reset the subscription link of {email}?',
      resetSubscribeMessage: 'The old link stops working at once and the user has to import the subscription again. This can’t be undone.',
      resetSubscribeAction: 'Reset link',
      deleteGrantTitle: 'Delete tunnel grant #{id}?',
      deleteGrantMessage: 'The user can no longer use tunnel {tunnel}. This can’t be undone.',
      deleteGrantAction: 'Delete grant'
    },
    copyDialog: {
      title: 'Copy subscription link',
      description: 'The link could not be copied automatically. Copy it below.',
      label: 'Subscription link'
    },
    resetFlow: {
      userTitle: 'Reset the traffic of {email}?',
      userMessage: 'Used traffic goes back to zero. This can’t be undone.',
      tunnelTitle: 'Reset the traffic of tunnel grant #{id}?',
      tunnelMessage: 'Used traffic of this grant goes back to zero. This can’t be undone.',
      usedFlow: 'Used flow',
      quota: 'Quota',
      confirmAction: 'Reset traffic'
    },
    trafficModal: {
      title: 'Traffic Detail - {email}',
      subtitle: 'Daily totals and hourly detail for the last 30 days.',
      refresh: 'Refresh',
      dailyTitle: 'Daily Traffic',
      hourlyTitle: 'Hourly Traffic',
      empty: 'No traffic records',
      fetchFailed: 'Failed to load user traffic',
      summary: {
        total30d: '30-day total',
        dailyPeak: 'Daily peak',
        hourlyPeak: 'Hourly peak'
      },
      table: {
        date: 'Date',
        hour: 'Hour',
        traffic: 'Traffic'
      }
    },
    labels: {
      admin: 'Admin',
      noLimit: 'No limit',
      noSpeedLimit: 'No speed limit',
      noDeviceLimit: 'No device limit',
      speedLimitMbps: '{value} Mbps',
      deviceLimitCount: '{value} devices',
      noReset: 'No reset',
      monthlyDay: 'Day {day} of every month',
      permanent: 'Permanent',
      trafficUnlimited: '{used} used · no limit'
    },
    messages: {
      actionFailed: 'Operation failed',
      fillEmailPassword: 'Please fill in email and password',
      passwordTooShort: 'Password must be at least 6 characters',
      userCreated: 'User created successfully',
      createFailed: 'Create failed',
      fetchUsersFailed: 'Users didn’t load',
      bulkBanned: '{count} users banned',
      bulkUnbanned: '{count} users unbanned',
      bulkPartial: '{done} of {total} users changed. {message}',
      fetchStatsFailed: 'Failed to fetch statistics',
      saveFailed: 'Save failed: {message}',
      userSaved: '{email} saved',
      userBanned: '{email} banned',
      userUnbanned: '{email} unbanned',
      fetchTunnelListFailed: 'Failed to fetch tunnel list',
      fetchSpeedLimitFailed: 'Failed to fetch speed limit rules',
      fetchTunnelGrantFailed: 'Failed to fetch tunnel grants',
      selectTunnelFirst: 'Please select a tunnel',
      tunnelAlreadyAssigned: 'This tunnel is already assigned to current user',
      grantUpdateFailed: 'Failed to update grant',
      grantCreateFailed: 'Failed to create grant',
      grantUpdated: 'Grant updated successfully',
      grantCreated: 'Grant created successfully',
      grantActionFailed: 'Grant action failed',
      grantDeleted: 'Tunnel grant #{id} deleted',
      grantDeleteFailed: 'Failed to delete grant',
      resetFailed: 'Reset failed',
      userFlowReset: 'User traffic reset successfully',
      tunnelFlowReset: 'Tunnel traffic reset successfully',
      fetchUserFailed: 'Failed to load the user',
      noToken: 'This user has no subscription token',
      subscribeCopied: 'Subscription link copied to clipboard',
      resetSubscribeSuccess: 'Subscription link reset',
      resetSubscribeFailed: 'Failed to reset subscription'
    }
  },
  control: {
    subtitle: 'Manage official signed packages, Control WebUI extensions, and lifecycle operations.',
    actions: {
      refresh: 'Refresh', refreshing: 'Refreshing...', importRelease: 'Import release', install: 'Install',
      configure: 'Configure', enable: 'Enable', disable: 'Disable', upgrade: 'Upgrade', update: 'Upgrade', rollback: 'Rollback', cancel: 'Cancel operation', installOfficialOnly: 'Official release required',
      saving: 'Saving...', newAssignment: 'New assignment'
    },
    pluginCenter: {
      filters: { search: 'Search plugins', health: 'Filter by health', target: 'Filter by target' },
      states: { healthy: 'Healthy', attention: 'Needs attention' },
      listLabel: 'Plugins',
      official: 'Official signed package',
      loadFailed: 'The plugin catalog didn’t load',
      empty: 'No plugins match the current filters',
      emptyCatalog: { title: 'No plugins yet', description: 'Import an official signed release to add its plugin to the catalog.' },
      detail: { versions: 'Versions and state' },
      operations: { title: 'Recent plugin operations', empty: 'No recent plugin operations' }
    },
    tabs: { assignments: 'Assignments', topologies: 'Topologies' },
    table: {
      plugin: 'Plugin', release: 'Release', installation: 'Installation', version: 'Version', state: 'State', actions: 'Actions',
      scope: 'Scope', description: 'Description', topology: 'Topology', activeRevision: 'Active revision', deployment: 'Deployment',
      operation: 'Operation', chain: 'Chain', revision: 'Revision', deadline: 'Deadline', target: 'Target', version: 'Version', role: 'Role', configRevision: 'Config revision', rolloutGroup: 'Rollout group'
    },
    labels: { releases: '{count} releases', desired: 'Desired', observed: 'Observed' },
    states: { catalogued: 'Catalogued', enabled: 'Enabled', disabled: 'Disabled', loading: 'Loading control state...' },
    empty: { assignments: 'No assignments for this node', topologies: 'No topologies', operations: 'No operations' },
    activity: { scoped: 'Selected activity', all: 'All activity', showAll: 'Show all activity', showScoped: 'Show selected activity', empty: 'No activity for the selected scope' },
    topology: {
      select: 'Topology', new: 'New topology', newTitle: 'Create topology', create: 'Create topology', name: 'Name', edit: 'Edit revision', status: 'View status', noDeployment: 'No deployment', editorTitle: 'Topology revision editor', revision: 'Revision',
      noRevisions: 'No revisions', failurePolicy: 'Failure policy', stopAndRollback: 'Stop and rollback', message: 'Revision message', graphJSON: 'Graph JSON',
      graphHelp: 'Use vertices and edges. Secrets must be referenced by secret_id; inline secret values are rejected.', diagnose: 'Validate / diagnose', validating: 'Validating...',
      unsavedChanges: 'This revision has unsaved changes. Save a new immutable revision before previewing or planning.',
      valid: 'Topology is valid', invalid: 'Topology has validation issues', invalidJSON: 'Topology JSON is invalid', saveRevision: 'Save revision', plan: 'Plan deployment',
      apply: 'Apply deployment', rollback: 'Rollback deployment', deployment: 'Deployment', applyConfirm: 'Apply {topology} (deployment #{deployment})?', rollbackConfirm: 'Request rollback for {topology} (deployment #{deployment})?',
      preview: 'Read-only deployment preview', previewAction: 'Preview', previewSteps: '{count} planned steps',
      emptyHint: 'Create a topology, describe its vertices and links, and save a revision to plan a deployment.',
      viewLabel: 'Edit as', viewJSON: 'JSON', viewGraph: 'Graph', graphLabel: 'Topology graph', graphSummary: '{nodes} vertices, {edges} links',
      graphEmpty: 'No vertices yet', graphEmptyHint: 'Add vertices to the JSON and the topology is drawn here.', graphInvalidHint: 'Fix the JSON and the topology is drawn here.', graphFailed: 'Couldn’t draw the topology'
    },
    extensions: { errorsTitle: 'WebUI extension loading failed' },
    assignments: {
      node: 'Node', noNodes: 'No nodes available', agentPlugin: 'Agent plugin', enabled: 'Assignment enabled',
      createTitle: 'Create node assignment', editTitle: 'Edit node assignment', deleteTitle: 'Delete {plugin} / {role} from this node?', deleteMessage: 'The node stops running this plugin role. This can’t be undone.', deleteAction: 'Delete assignment',
      emptyHint: 'Give this node a role: pick the plugin, scope and version.', roleHelp: 'Common roles: {roles}'
    },
    install: { title: 'Install official plugin', target: 'Runtime target', version: 'Release version', enableAfterInstall: 'Enable immediately after installation' },
    update: { title: 'Upgrade official plugin' },
    config: {
      title: 'Plugin configuration', loading: 'Loading configuration...', revision: 'Configuration revision: {revision}', mode: 'Configuration editor mode', formMode: 'Form', jsonMode: 'JSON',
      selectValue: 'Select a value', addItem: 'Add item', removeItem: 'Remove item', emptyArray: 'No items', item: 'Item', invalidJSON: 'Invalid JSON',
      schemaError: '{path} does not satisfy the schema', requiredError: '{path} is required'
    },
    releaseImport: {
      title: 'Import official signed release', manifest: 'Manifest JSON', signature: 'Signature', artifact: 'Package artifact',
      artifactOptional: 'Optional; a release without an uploaded artifact cannot be installed.'
    },
    messages: {
      actionQueued: '{action} was submitted for {plugin}', operationStatus: 'Operation {id} is {state}. Chain: {chain}.', installed: 'Installation intent was saved for {plugin}', configSaved: 'Configuration was saved for {plugin}',
      releaseImported: '{plugin} {version} was imported', cancelRequested: 'Operation cancellation was requested',
      assignmentSaved: '{plugin} assignment was saved', assignmentStateSaved: '{plugin} assignment state was saved', assignmentDeleted: '{plugin} assignment was deleted',
      topologyRevisionSaved: 'Topology revision {revision} was saved', topologyPlanned: 'Deployment #{id} was planned',
      topologyApplyRequested: 'Deployment apply was requested', topologyRollbackRequested: 'Deployment rollback was requested', topologyCreated: 'Topology {name} was created'
    },
    errors: {
      load: 'Unable to load control state', action: 'Plugin operation failed', install: 'Plugin installation failed', configLoad: 'Unable to load plugin configuration',
      configSave: 'Unable to save plugin configuration', releaseImport: 'Release import failed', cancel: 'Unable to cancel operation', poll: 'Unable to refresh operation state',
      nodesLoad: 'Unable to load nodes', assignmentsLoad: 'Unable to load node assignments', assignmentSave: 'Unable to save node assignment', assignmentDelete: 'Unable to delete node assignment',
      topologyLoad: 'Unable to load topology revisions', topologyValidate: 'Unable to validate topology', topologySave: 'Unable to save topology revision', topologyPlan: 'Unable to plan topology deployment',
      topologyPreview: 'Unable to preview topology deployment', topologyStatus: 'Unable to load deployment status', topologyApply: 'Unable to apply topology deployment', topologyRollback: 'Unable to roll back topology deployment', topologyCreate: 'Unable to create topology'
    }
  },
  routeModes: {
    open: 'Route modes',
    subtitle: 'Switch each package’s v2 routes between legacy, shadow and native, and roll back to legacy.',
    back: 'Plugin Center',
    package: 'Package',
    packageMeta: 'Version {version} · config revision {revision}',
    packageDisabled: 'Disabled',
    superAdminOnly: 'Only super admins can switch route modes.',
    loadFailed: 'Route modes didn’t load',
    empty: { title: 'No packages with v2 routes', description: 'Install and enable a package that declares v2 routes to manage its route modes.' },
    modes: { legacy: 'Legacy', shadow: 'Shadow', native: 'Native' },
    modeDefault: '{mode} (default)',
    sources: {
      stored: 'Set explicitly',
      default: 'Rehearsed default: no mode is stored',
      'kill-switch': 'Default off: package_routes.default_mode is legacy',
      'package-too-old': 'Default off: the package is older than the rehearsed release',
      'identity-authority': 'Follows the identity authority',
      unset: 'No mode stored'
    },
    defaults: {
      'kill-switch': 'Native defaults are off (package_routes.default_mode: legacy). Routes without a stored mode run legacy.',
      'package-too-old': 'Version {version} is older than {min}, the rehearsed release: its routes default to legacy.'
    },
    catalog: { 'native-flagged': 'Native-flagged', bridged: 'Bridged', 'kernel-owned': 'Kernel-owned', native: 'Native', none: 'Undeclared' },
    locked: { kernel_owned: 'Kernel-owned', identity_group_a: 'Identity cutover', websocket: 'WebSocket', not_declared: 'Not declared' },
    columns: { route: 'Route', endpoint: 'Method / path', catalog: 'Eligibility', configured: 'Configured', effective: 'Effective', shadow: 'Shadow (total / mismatch / errors)', mismatch: 'Mismatch rate', mode: 'Switch to' },
    routesLabel: 'Routes of {package}',
    routeMode: 'Mode of {route}',
    effectiveDiffers: 'Differs from configured',
    hostEffective: 'Host: {mode}',
    packageMode: 'Mode for the whole package',
    setPackage: 'Set whole package',
    rollback: 'Roll back to legacy',
    wholePackage: 'Whole package',
    wholePackageTarget: '{package} (whole package)',
    dialog: {
      setTitle: 'Switch {target} to {mode}?',
      setMessage: 'Routes that can’t take this mode are skipped. The change is recorded in the revision history.',
      nativeMessage: 'In native mode the package implementation answers instead of the legacy kernel handler. Confirm and give a reason for the audit trail.',
      rollbackTitle: 'Roll {package} back to legacy?',
      rollbackMessage: 'Every route of the package returns to legacy, routes native by default included.',
      reason: 'Reason',
      reasonHelp: 'Optional; stored with the revision.',
      reasonRequired: 'Required to switch to native.',
      confirmSet: 'Switch mode',
      confirmNative: 'Switch to native',
      confirmRollback: 'Roll back'
    },
    result: '{changed} routes changed, {skipped} skipped (config revision {revision})',
    errors: { switch: 'Couldn’t switch the route mode', rollback: 'Couldn’t roll back', revisions: 'Revision history didn’t load' },
    revisions: {
      title: 'Revision history',
      label: 'Route mode revisions',
      empty: 'No route mode changes yet',
      time: 'Time', route: 'Route', change: 'Change', actor: 'Actor', reason: 'Reason', revision: 'Revision',
      actions: { set: 'Set', rollback: 'Rollback' }
    },
    mismatches: {
      title: 'Shadow mismatch samples',
      label: 'Mismatch samples',
      open: 'Samples ({count})',
      lastSeen: 'Last {time}',
      privacy: 'Samples are masked before they are stored: tokens, passwords, keys, UUIDs and subscription links are hidden, e-mail addresses keep their first letter and domain, IP addresses their first two parts. Each route keeps its latest {max} samples for {days} days.',
      loadFailed: 'Mismatch samples didn’t load',
      empty: 'No samples stored',
      emptyDescription: 'Samples appear here when a shadow run answers differently from legacy.',
      observed: 'Observed',
      status: 'Status (legacy → native)',
      diffColumn: 'Differences',
      fields: '{count} fields',
      detail: 'Sample details',
      request: 'Request',
      requestID: 'Request ID',
      version: 'Package version',
      truncated: 'Diff shortened',
      diff: 'Differences (masked)',
      copyDiff: 'Copy diff'
    }
  },
  accessGroups: {
    subtitle: 'Manage independent service-scope memberships, resource grants, and plugin-owned quota policies.',
    actions: { refresh: 'Refresh', newGroup: 'New group', open: 'Open', editGroup: 'Edit name and description', enable: 'Enable', disable: 'Disable', add: 'Add', removeNamed: 'Remove {name}', addGrant: 'Add grant', saveQuota: 'Save quota', resolve: 'Resolve access' },
    filters: { label: 'Filter by service scope', allScopes: 'All service scopes' },
    table: { group: 'Access group', scope: 'Scope', state: 'State' },
    states: { enabled: 'Enabled', disabled: 'Disabled' },
    groups: { title: 'Access groups', empty: 'No access groups in this scope', emptyAll: 'No access groups yet', emptyDescription: 'An access group gives its users and plans resource grants and quota policies within one service scope.', noDescription: 'No description', directUnion: 'Direct and plan membership are combined as an allow-union.' },
    detail: { title: 'Access group', loading: 'Loading access group details…', group: 'Group', danger: 'Danger zone' },
    members: { title: 'User members', userID: 'User ID', empty: 'No direct user members' },
    plans: { title: 'Plan memberships', planID: 'Plan ID', empty: 'No plan memberships' },
    grants: { title: 'Resource grants', resourceType: 'Resource type', resourceID: 'Resource ID', permissions: 'Permissions JSON', empty: 'No resource grants' },
    quotas: { title: 'Quota policies', key: 'Policy key', policy: 'Policy JSON', empty: 'No quota policies' },
    resolver: { title: 'Effective access preview', description: 'Preview the server-side allow-union for one user, optional plan, and service scope.', userID: 'User ID', planID: 'Plan ID (optional)', scope: 'Service scope', result: '{count} enabled groups apply', none: 'No enabled groups apply', policySummary: '{grants} grants and {quotas} quota policies are effective.' },
    editor: { createTitle: 'Create access group', editTitle: 'Edit access group', name: 'Group name', description: 'Description', enabled: 'Group is enabled', scopeFixed: 'The scope of an existing group can’t change.' },
    messages: { groupCreated: 'Created access group {name}', groupSaved: 'Saved access group {name}', groupEnabled: 'Enabled access group {name}', groupDisabled: 'Disabled access group {name}', groupDeleted: 'Deleted access group {name}', memberAdded: 'Added user #{id}', memberRemoved: 'Removed user #{id}', planAdded: 'Added plan #{id}', planRemoved: 'Removed plan #{id}', grantAdded: 'Added resource grant', grantRemoved: 'Removed resource grant', quotaSaved: 'Saved quota policy', quotaRemoved: 'Removed quota policy' },
    confirm: { deleteGroupTitle: 'Delete access group {name}?', deleteGroup: 'Its memberships, resource grants and quota policies are deleted with it. This can’t be undone.', deleteGroupAction: 'Delete group', removeGrantTitle: 'Remove resource grant #{id}?', removeGrant: 'Members of this group lose access to {resource}. This can’t be undone.', removeGrantAction: 'Remove grant', removeQuotaTitle: 'Remove quota policy {key}?', removeQuota: 'The group is no longer limited by this quota. This can’t be undone.', removeQuotaAction: 'Remove policy' },
    errors: { load: 'Unable to load access control data', loadGroups: 'Unable to load access groups', loadDetail: 'Unable to load access group details', groupRequired: 'A service scope and group name are required', saveGroup: 'Unable to save access group', deleteGroup: 'Unable to delete access group', member: 'Unable to update user membership', plan: 'Unable to update plan membership', grant: 'Unable to update resource grant', quota: 'Unable to update quota policy', resolve: 'Unable to resolve effective access', invalidID: '{label} must be a positive integer', invalidJSON: '{label} must be valid JSON', scopeRequired: 'A service scope is required' }
  },
  agentUpgrades: {
    title: 'Agent upgrades',
    description: 'Control pushes Agent releases in canary batches (5%, 25%, 100%, at least 30 minutes each) and rolls a batch back when more than 5% of it fails.',
    empty: 'No Agent upgrade yet. A super administrator starts one with:',
    target: 'Agent {version}',
    batchOf: 'batch {batch} of {total}',
    batchEnds: 'next batch at {time} at the earliest',
    finished: 'ended {time}',
    batch: 'Batch {batch} · {percent}%',
    progress: '{done} / {total} settled',
    statuses: { running: 'Running', paused: 'Paused', rolling_back: 'Rolling back', succeeded: 'Succeeded', rolled_back: 'Rolled back', aborted: 'Aborted' },
    states: { pending: 'Pending', offered: 'Offered', upgrading: 'Upgrading', succeeded: 'Upgraded', failed: 'Failed', rolled_back: 'Rolled back', skipped: 'Skipped' },
    actions: { pause: 'Pause', resume: 'Resume', abort: 'Abort', rollback: 'Abort and roll back' },
    confirm: {
      abortTitle: 'Abort the upgrade to {version}?',
      abort: 'No further node is offered the upgrade. Nodes already upgraded keep the new release.',
      rollbackTitle: 'Roll back the upgrade to {version}?',
      rollback: 'The upgraded nodes of the current batch are told to reinstate their previous release, then the campaign ends.'
    },
    loadFailed: 'Unable to load Agent upgrades'
  },
  agentTransports: {
    open: 'Agent transports',
    subtitle: 'How each node’s agent reaches Control: the mTLS stream, or a legacy channel that v4.2 will refuse.',
    back: 'NodeX Agents',
    mode: 'agent_control.mtls: {mode}',
    modes: {
      off: 'Client certificates are off; legacy agents are served silently.',
      optional: 'Client certificates are verified when presented; legacy agents are served silently.',
      preferred: 'Legacy agents are still served, with deprecation signals.',
      required: 'Only enrolled agents (client certificates) are accepted on AnixOps Agent channels.'
    },
    sunset: 'Legacy sunset: {date}',
    notice: 'v4.2 makes agent_control.mtls: required the default. Before upgrading, every node marked Legacy must run an Agent that has enrolled; legacy API-key agents will be refused. UniProxy and v2board gRPC (third-party node software) are not affected.',
    guide: 'Upgrade guide',
    filterLabel: 'Filter by status',
    statuses: { all: 'All', mtls: 'mTLS', legacy: 'Legacy', 'third-party': 'Third-party', unseen: 'Not seen' },
    legacyHint: 'Refused under required',
    transports: {
      'mtls-stream': 'mTLS stream',
      'apikey-stream': 'API key stream',
      'http-legacy': 'Legacy HTTP',
      websocket: 'WebSocket',
      'clean-agent': 'Clean agent',
      uniproxy: 'UniProxy',
      'v2board-grpc': 'v2board gRPC'
    },
    columns: { node: 'Node', status: 'Status', transport: 'Last transport', version: 'Agent version', certificate: 'Certificate', lastSeen: 'Last seen', seenOn: 'Seen on' },
    certificateUntil: 'until {date}',
    noCertificate: 'Not enrolled',
    disabled: 'Disabled',
    tableLabel: 'Agent transports by node',
    empty: { title: 'No nodes', description: 'Proxy and forward nodes appear here once they exist.' },
    loadFailed: 'Unable to load agent transports'
  }
}
