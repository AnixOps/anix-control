// Forward nodes (UI U7): the header, run-mode switch, detail pages and
// runtime pages shared by NodeX nodes, Ansible machines, the local runtime
// and the NodeX runtime.
export default {
  forwardNodesPage: {
    title: 'Forward nodes',
    modes: {
      label: 'Run mode'
    },
    descriptions: {
      nodex: 'Stateful relay and exit nodes for NodeX mode, and the legacy port rules.',
      local: 'The Ansible executor on the panel host: stateless, no NodeX control plane needed.',
      nodexRuntime: 'How the panel reaches the NodeX control plane, what the probes say, and the latest gost jobs.'
    },
    nodex: {
      tableLabel: 'NodeX nodes',
      typeFilter: 'Node type',
      statusFilter: 'Reachability',
      onlineNote: '"Online" only means host:port accepts a TCP connection; it does not mean gost or NodeX is ready.',
      columns: {
        name: 'Node',
        type: 'Type',
        status: 'Status',
        managementApi: 'Management API',
        regionIsp: 'Region / ISP',
        latency: 'Latency',
        connections: 'Connections',
        traffic: 'Upload / download',
        weight: 'Weight / max connections',
        lastCheck: 'Last check',
        uptime: 'Uptime',
        result: 'Last result'
      },
      summary: {
        relay: 'Relay',
        exit: 'Exit',
        online: 'online',
        upload: 'Upload',
        download: 'Download'
      }
    },
    rules: {
      tableLabel: 'Legacy rules',
      userFilterLabel: 'Filter by user ID',
      apply: 'Filter',
      columns: {
        name: 'Rule',
        ingress: 'Ingress',
        egress: 'Egress',
        owner: 'Owner',
        limits: 'Limits',
        traffic: 'Traffic',
        status: 'Status'
      }
    },
    detail: {
      sections: 'Node details',
      tabs: {
        overview: 'Overview',
        config: 'Configuration',
        danger: 'Danger zone'
      },
      runtime: 'Runtime',
      actions: 'Actions',
      identity: 'Node',
      endpoints: 'Addresses and ports',
      capacity: 'Capacity',
      editConfig: 'Edit configuration',
      tokenHidden: 'Set (hidden)',
      tokenNone: 'Not set',
      lastResult: 'Last result',
      noResult: 'Not checked yet',
      loadFailed: 'Could not load the node',
      notFound: 'This node no longer exists. It may have been deleted.',
      dangerFooter: 'This cannot be undone. Forwarding that depends on it stops working.',
      disableFooter: 'A disabled node takes no new forwarding jobs. You can enable it again at any time.'
    },
    ansibleDetail: {
      loadFailed: 'Could not load the Ansible machine',
      notFound: 'This machine no longer exists. It may have been deleted.',
      dangerFooter: 'This cannot be undone. The machine leaves the Ansible execution fleet.'
    },
    runtimePage: {
      commandsTitle: 'Troubleshooting commands',
      copyGroup: 'Copy the {label} commands',
      copied: 'Copied',
      doctorTitle: 'Doctor output',
      statusLoadFailed: 'Could not load the runtime status',
      emptyStatus: 'No status yet',
      emptyJobs: 'No runtime jobs yet',
      emptyJobsDescription: 'Runtime jobs the panel writes when forwards, tunnels or nodes change appear here.',
      columns: {
        job: 'Job',
        target: 'Target',
        status: 'Status',
        time: 'Time',
        message: 'Result'
      }
    }
  }
}
