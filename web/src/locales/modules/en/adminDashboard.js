export default {
  adminDashboard: {
    title: 'Dashboard',
    subtitle: 'Users, nodes, traffic and tickets at a glance, and what needs you.',
    updatedAt: 'Data from {time}',
    refresh: 'Refresh',
    loadFailed: 'Couldn’t load the dashboard',
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
      ticketsUnknown: 'Tickets didn’t load',
      revenue: 'Revenue this month',
      revenueDetail: '{today} today · {total} in total',
      orders: 'Orders awaiting payment',
      ordersDetail: '{total} in total · {paid} completed'
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
      loadFailed: 'Couldn’t load traffic',
      hour: 'Hour',
      value: 'Traffic',
      series: 'Traffic'
    },
    commerce: {
      title: 'Revenue and orders'
    },
    alerts: {
      title: 'Needs attention',
      description: 'Offline nodes, tickets waiting for a reply, stalled traffic reports, and certificate and rollout alerts.',
      allClear: 'All clear',
      allClearDescription: 'No node is offline and no ticket is waiting.',
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
      alertsFailed: 'Couldn’t load certificate and rollout alerts',
      noResolved: 'No resolved alerts',
      noResolvedDescription: 'Alerts that cleared in the last 30 days are listed here.',
      resolvedAt: 'Resolved {date}',
      kinds: {
        agentCertificate: {
          ending: 'The Agent certificate of {name} is about to expire',
          ended: 'The Agent certificate of {name} has expired',
          endingHint: 'Ends {date}. The Agent has not renewed it: check that it is running and can reach Control.',
          endedHint: 'Ended {date}. The Agent can’t connect: enroll it again with a new install command.'
        },
        linkCertificate: {
          ending: 'The forward link certificate of {name} is about to expire',
          ended: 'The forward link certificate of {name} has expired',
          endingHint: 'Ends {date}. It renews with the Agent certificate: check the Agent.',
          endedHint: 'Ended {date}. Bring the Agent back, or enroll it again.'
        },
        moduleCertificate: {
          ending: 'The certificate of module {name} is about to expire',
          ended: 'The certificate of module {name} has expired',
          endingHint: 'Ends {date}. Check that the module is running and reaches Control.',
          endedHint: 'Ended {date}. Restart the module, enroll it again, or revoke its enrollment.'
        },
        ca: {
          module: 'module',
          forwardLink: 'forward link',
          ending: 'The current {ca} CA is about to end',
          ended: 'The current {ca} CA has ended',
          staged: 'Ends {date}. A next CA is staged and takes over by itself.',
          notStaged: 'Ends {date}. No next CA is staged: rotate the CA now.'
        },
        nodeSecretsStalled: {
          title: 'The credential split of {table} has stalled in {phase}',
          hint: 'Not touched since {date}. Verify and finalize it, or turn the reminder off.'
        },
        nodeSecretsInterrupted: {
          title: 'Finalizing the credential split of {table} was interrupted',
          hint: 'Started {date}. Run the finalize again: it resumes.'
        },
        identityImport: {
          title: 'The identity import has stalled',
          hint: 'No progress since {date}. Resume the import or start the cutover.'
        },
        identityCutover: {
          title: 'The identity cutover is not finalized',
          hint: 'Identity has handled sign-ins since {date}. Finalize it, or roll back.'
        }
      },
      nodesFailed: 'Couldn’t load node status',
      ticketsFailed: 'Couldn’t load tickets'
    },
    activity: {
      title: 'Recent activity',
      description: 'From the audit log.',
      open: 'Audit log',
      empty: 'No activity yet',
      emptyDescription: 'Administrator actions are recorded here.',
      loadFailed: 'Couldn’t load the audit log',
      actor: '{name}',
      system: 'System'
    }
  }
}
