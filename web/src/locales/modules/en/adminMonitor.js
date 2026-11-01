export default {
  adminMonitor: {
    title: 'Traffic & Monitoring',
    subtitle: 'Live node status, traffic by user, node latency and forward paths.',
    sectionsLabel: 'Traffic and monitoring sections',
    sections: {
      live: 'Live nodes',
      traffic: 'User traffic',
      destinations: 'Sites',
      categories: 'Categories'
    },
    range: {
      label: 'Time range',
      '1h': '1 h',
      '24h': '24 h',
      '7d': '7 d',
      '30d': '30 d'
    },
    destinations: {
      user: 'User',
      allUsers: 'All users',
      settings: {
        title: 'Site statistics',
        description: 'Nodes report each user\'s traffic by destination domain. Users see their own statistics on their Site statistics page. Off by default; turning it off stops recording at once.',
        enabled: 'Record site statistics',
        enabledHint: 'Needs anix-agent with destinations.v1 on the nodes.',
        retention: 'Keep for',
        retentionUnit: 'days',
        retentionHelp: '7 to 180 days. Each active user takes about 25–50 KB a day.',
        save: 'Save',
        saved: 'Site statistics settings saved',
        saveFailed: 'Could not save the settings',
        loadFailed: 'Could not load the settings'
      },
      off: 'Site statistics are off: nothing new is recorded. The history kept is still shown.'
    },
    categories: {
      test: {
        title: 'Test a destination',
        description: 'Which category the current rules put a domain, address or network in.',
        domain: 'Domain, address or network',
        protocol: 'Protocol (optional)',
        run: 'Classify',
        result: '{target} is “{category}”',
        failed: 'Classification failed'
      },
      rules: {
        title: 'Custom rules',
        description: 'Custom rules win over the remote sources and the built-in rules, apply at once and to the recorded history too. Point a domain at “Web” to take it out of a built-in category.',
        label: 'Custom category rules',
        add: 'Add rule',
        edit: 'Edit rule',
        emptyTitle: 'No custom rules yet',
        emptyDescription: 'Traffic is classified by the built-in rules and the remote sources.',
        loadFailed: 'Could not load the rules',
        kind: 'Kind',
        value: 'Value',
        category: 'Category',
        note: 'Note',
        kinds: { domain: 'Domain', network: 'Network', protocol: 'Protocol' },
        valueHelp: {
          domain: 'For example example.com; its subdomains match too.',
          network: 'For example 203.0.113.0/24 or one address.',
          protocol: 'A protocol the nodes sniff, such as bittorrent.'
        },
        saved: 'Rule saved',
        saveFailed: 'Could not save the rule',
        deleteTitle: 'Delete the rule {value}?',
        deleteMessage: 'The destination falls back to the remote sources or the built-in rules.',
        deleteConfirm: 'Delete rule',
        deleted: 'Rule deleted',
        deleteFailed: 'Could not delete the rule'
      },
      sources: {
        title: 'Remote sources',
        description: 'Lists of domains and networks fetched from a URL on a schedule, the whole list in one category. Plain lists, v2fly domain-list-community (domain:, full:), Clash rules (DOMAIN-SUFFIX, IP-CIDR) and Clash rule providers are read. A failed fetch keeps the previous list.',
        label: 'Remote sources',
        add: 'Add source',
        edit: 'Edit source',
        emptyTitle: 'No remote sources yet',
        emptyDescription: 'For example https://raw.githubusercontent.com/v2fly/domain-list-community/master/data/netflix as Video.',
        name: 'Name',
        url: 'URL',
        category: 'Category',
        enabled: 'Enabled',
        refreshHours: 'Refresh every',
        refreshUnit: 'hours',
        refreshHelp: '1 to 168 hours.',
        entries: 'Entries',
        fetchedAt: 'Last fetch',
        never: 'Not fetched yet',
        status: 'Status',
        ok: 'OK',
        failed: 'Failed',
        disabled: 'Disabled',
        refresh: 'Fetch now',
        refreshed: 'Fetched {count} entries',
        refreshFailed: 'Fetch failed: {error}',
        saved: 'Source saved; it is fetched within 10 minutes',
        saveFailed: 'Could not save the source',
        deleteTitle: 'Delete the source {name}?',
        deleteMessage: 'Its list goes too; its destinations fall back to the other rules.',
        deleteConfirm: 'Delete source',
        deleted: 'Source deleted',
        deleteFailed: 'Could not delete the source'
      },
      builtin: {
        title: 'Built-in rules',
        description: 'The rules built into Control, updated with each release; custom rules and remote sources override them.',
        counts: '{domains} domains · {networks} networks · {protocols} protocols',
        show: 'Show',
        hide: 'Hide'
      },
      cancel: 'Cancel',
      save: 'Save',
      loadFailed: 'Could not load the category rules'
    },
    refresh: 'Refresh',
    live: {
      connection: {
        connecting: 'Connecting…',
        connected: 'Live',
        disconnected: 'Disconnected',
        reconnecting: 'Reconnecting in {seconds} s…'
      },
      overview: {
        label: 'Node overview',
        totalNodes: 'Nodes',
        onlineNodes: 'Online',
        offlineNodes: 'Offline',
        pendingNodes: 'Pending',
        upload: '{value} uploaded',
        download: '{value} downloaded'
      },
      table: {
        label: 'Live node status',
        name: 'Name',
        host: 'Address',
        status: 'Status',
        cpu: 'CPU',
        memory: 'Memory',
        disk: 'Disk',
        users: 'Users online',
        uptime: 'Uptime'
      },
      status: {
        online: 'Online',
        offline: 'Offline',
        pending: 'Pending'
      },
      traffic: {
        view: 'View traffic',
        openNode: 'Open node page',
        title: 'Traffic of {name}',
        description: 'Upload and download over time. The node page has its protocols, credentials and logs.'
      },
      waiting: 'Waiting for the first node snapshot',
      waitingDescription: 'Node status shows here in real time once connected.',
      empty: 'No nodes yet',
      emptyDescription: 'Once you add a node, its CPU, memory and online users show here live.',
      offlineTitle: 'The live connection dropped',
      offlineDescription: 'Couldn’t reach the live monitor. It reconnects on its own.',
      reconnectNow: 'Reconnect now',
      unsupported: 'This browser doesn’t support WebSocket'
    },
    traffic: {
      user: 'User',
      allUsers: 'All users',
      showing: 'Showing {user}',
      clearUser: 'Show all users',
      summary: {
        label: 'Traffic summary',
        total: 'Traffic in range',
        peak: 'Peak hour',
        latestReport: 'Last report',
        latestReportHint: 'When a node last wrote a traffic log',
        noReport: 'No traffic reports yet'
      },
      chart: {
        title: 'Hourly traffic',
        label: 'Hourly traffic of {user} over {range}',
        summary: '{total} in total, peak {peak} ({hour})',
        summaryEmpty: 'No traffic in the selected range',
        series: 'Traffic',
        hour: 'Hour',
        value: 'Traffic'
      },
      empty: 'No traffic data in the selected range',
      emptyWithLatest: 'The last report was {time}. Check the node processes and the report path.',
      emptyNever: 'No node has reported traffic yet.',
      loadFailed: 'Couldn’t load traffic',
      ranking: {
        title: 'Top users by traffic',
        description: 'Pick a user to see their traffic in the chart above.',
        label: 'Users by traffic',
        rank: 'Rank',
        user: 'User',
        traffic: 'Traffic',
        empty: 'No user traffic in the selected range',
        loadFailed: 'Couldn’t load the user ranking'
      }
    }
  }
}
