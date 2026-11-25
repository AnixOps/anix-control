export default {
  adminDashboard: {
    title: 'Dashboard',
    subtitle: 'Overview of users, nodes, traffic and tickets, and pending items.',
    updatedAt: 'Data from {time}',
    refresh: 'Refresh',
    loadFailed: 'Could not load the dashboard',
    metrics: {
      label: 'Key metrics',
      users: 'Users',
      usersToday: '+{count} today',
      usersDetail: '{active} active · {expired} expired · {banned} banned',
      nodes: 'Nodes online',
      nodesDetail: '{count} users online',
      traffic: 'Traffic today',
      trafficDetail: '{total} in total',
      tickets: 'Open tickets',
      ticketsDetail: '{count} answered',
      ticketsUnknown: 'Could not load the tickets',
      revenue: 'Revenue this month',
      revenueDetail: '{today} today · {total} in total',
      orders: 'Orders awaiting payment',
      ordersDetail: '{total} in total · {paid} completed',
      backup: 'Last verified backup',
      backupNone: 'None',
      backupAge: '{age} ago',
      backupTaken: 'Taken {date}',
      backupStale: 'Older than {hours} hours',
      backupUnsupported: 'Backups need SQLite',
      backupUnknown: 'Could not load the backups'
    },
    traffic: {
      title: 'Traffic, last 24 hours',
      description: 'Hourly traffic of all users.',
      open: 'Traffic & Monitoring',
      label: 'Hourly traffic over the last 24 hours',
      summary: '{total} in total, peak {peak} ({hour})',
      summaryEmpty: 'No traffic in the last 24 hours',
      empty: 'No traffic in the last 24 hours',
      emptyDescription: 'Hourly traffic shows here once nodes report it.',
      loadFailed: 'Could not load traffic',
      hour: 'Hour',
      value: 'Traffic',
      series: 'Traffic'
    },
    commerce: {
      title: 'Revenue and orders'
    },
    alerts: {
      title: 'Needs attention',
      description: 'Offline nodes, tickets awaiting a reply, stalled traffic reports, and certificate and rollout alerts.',
      allClear: 'All clear',
      allClearDescription: 'No offline nodes and no pending tickets.',
      offlineNode: '{name} is offline',
      lastSeen: 'Last heartbeat {time}',
      neverSeen: 'Never sent a heartbeat',
      moreOffline: '{count} more offline nodes',
      openTickets: '{count} tickets waiting for a reply',
      openTicketsHint: 'The oldest was opened {time}',
      pendingOrders: '{count} orders awaiting payment',
      trafficStale: 'Traffic reports have stopped',
      trafficStaleHint: 'The last report was {time}. Check the node processes and the report path.',
      view: 'Alert status',
      viewActive: 'Active',
      viewResolved: 'Resolved',
      summary: '{count} active alerts',
      summaryCritical: '{count} active, {critical} critical',
      alertsFailed: 'Could not load certificate and rollout alerts',
      noResolved: 'No resolved alerts',
      noResolvedDescription: 'Alerts that cleared in the last 30 days are listed here.',
      resolvedAt: 'Resolved {date}',
      kinds: {
        agentCertificate: {
          ending: 'The Agent certificate of {name} is about to expire',
          ended: 'The Agent certificate of {name} has expired',
          endingHint: 'Ends {date} and the Agent has not renewed it. Check that the Agent is running and can reach Control.',
          endedHint: 'Ended {date}; the Agent cannot connect. Enroll it again with a new install command.'
        },
        linkCertificate: {
          ending: 'The forward link certificate of {name} is about to expire',
          ended: 'The forward link certificate of {name} has expired',
          endingHint: 'Ends {date}. It renews with the Agent certificate: check the Agent.',
          endedHint: 'Ended {date}. Bring the Agent back, or enroll it again.'
        },
        ca: {
          module: 'module',
          forwardLink: 'forward link',
          ending: 'The current {ca} CA is about to end',
          ended: 'The current {ca} CA has ended',
          staged: 'Ends {date}. The next CA is staged and takes over automatically.',
          notStaged: 'Ends {date} and no next CA is staged. Rotate the CA now.'
        },
        nodeSecretsStalled: {
          title: 'The credential split of {table} has stalled in {phase}',
          hint: 'No activity since {date}. Verify and finalize it, or dismiss the reminder.'
        },
        nodeSecretsInterrupted: {
          title: 'Credential split finalization of {table} interrupted',
          hint: 'Started {date}. Run the finalize step again to resume.'
        },
        builtinPackageInvalid: {
          title: 'The built-in package {name} does not verify',
          hint: 'Not installed; its pages and routes are unavailable. Install a verified release binary.'
        },
        builtinPackageFailed: {
          title: 'The built-in package {name} is not running',
          hint: 'Its pages and routes are unavailable. Resolve the cause shown in the Control log and restart Control.'
        },
        panelCertificate: {
          ending: 'The certificate for {name} is about to expire',
          endingHint: 'Ends {date}; automatic renewal failed. Run anix-control tls status on the server for the cause.',
          endingFilesHint: 'Ends {date}. Replace the certificate files with a renewed certificate; the panel reloads them automatically.',
          failed: 'The panel has no usable certificate for {name}',
          renewalFailing: 'The certificate for {name} could not be renewed',
          hint: 'The panel is serving a temporary certificate. Run anix-control tls status on the server for the cause.'
        },
        backupVerifyFailed: {
          title: 'The backup {name} failed its verification',
          hint: 'Taken {date}; it cannot be restored. Run anix-control backup create on the server and check the Control log.'
        },
        backupStale: {
          title: 'No verified backup in the last {hours} hours',
          hint: 'Newest verified backup: {date}. Check the backup worker in the Control log, or back up now.',
          never: 'No verified backup. Check the backup worker in the Control log, or back up now.'
        },
        exitMainland: {
          title: '{name} refused mainland-China destinations ({n} so far)',
          hint: 'Last refused {date}. Check the entries and client routing; mainland traffic should not reach this exit.'
        },
        crossBorderDown: {
          title: 'Cross-border link {name} down since {date}',
          hint: 'Switch to tier 1 (REALITY) under Nodes → Standby, or to tier 2 (XHTTP) if tier 1 is also unreachable. See docs/guide/cross-border-standby.md.',
          switched: 'Traffic runs on the {tier} standby channel. Switch back under Standby once the link recovers.'
        },
        crossBorderWide: {
          title: '{n} of {total} cross-border links are down at once',
          hint: 'Several links are down at once. Follow the manual plan: switch all to tier 1 or tier 2, then record the symptoms.'
        }
      },
      nodesFailed: 'Could not load node status',
      ticketsFailed: 'Could not load tickets'
    },
    activity: {
      title: 'Recent activity',
      description: 'From the audit log.',
      open: 'Audit log',
      empty: 'No activity',
      emptyDescription: 'Administrator actions appear here.',
      loadFailed: 'Could not load the audit log',
      actor: '{name}',
      system: 'System'
    }
  }
}
