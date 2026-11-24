// Sign-in and the user pages (UI redesign phase U5): sign in, Overview,
// Subscription, Help Center, Tickets, Plans, Orders. Written for English
// readers, not translated from the Chinese file
// (guidelines/voice-and-tone.md: sentence case, verbs on buttons).
export default {
  auth: {
    signIn: {
      title: 'Sign in to AnixOps Control',
      subtitle: 'Subscriptions, nodes and forwarding.',
      email: 'Email',
      password: 'Password',
      submit: 'Sign in',
      noAccount: 'No account?',
      register: 'Create an account'
    },
    register: {
      title: 'Create your account',
      subtitle: 'Register to obtain a subscription link.',
      email: 'Email',
      password: 'Password',
      passwordHelp: 'At least 6 characters.',
      confirm: 'Confirm password',
      invite: 'Invite code',
      inviteHelp: 'Invite codes are issued by the administrator.',
      submit: 'Create account',
      haveAccount: 'Already have an account?',
      signIn: 'Sign in',
      done: 'Account created'
    },
    mfa: {
      title: 'Two-factor authentication',
      description: 'Enter the 6-digit code for {email}.',
      code: '6-digit code',
      submit: 'Verify',
      back: 'Back',
      useRecovery: 'Use a recovery code',
      recoveryTitle: 'Use a recovery code',
      recoveryDescription: 'Enter a recovery code. Each code can be used once.',
      recovery: 'Recovery code',
      useCode: 'Use your authenticator instead'
    },
    enroll: {
      title: 'Two-factor authentication required',
      description: 'Two-factor authentication is required to sign in. It is not enabled for {email}.',
      stepsTitle: 'Steps',
      step1: 'Ask the administrator to pause the requirement in Two-factor policy.',
      step2: 'Sign in, open Account, turn on two-factor authentication and save the recovery codes.',
      step3: 'Ask the administrator to restore the requirement.',
      app: 'An authenticator app is required, such as 1Password, Google Authenticator or Microsoft Authenticator.',
      back: 'Back to sign in'
    },
    errors: {
      emailRequired: 'Enter your email.',
      emailInvalid: "Enter an email like name{'@'}example.com.",
      passwordRequired: 'Enter your password.',
      passwordShort: 'Use at least 6 characters.',
      passwordMismatch: 'The passwords do not match.',
      inviteRequired: 'Enter your invite code.',
      codeIncomplete: 'Enter all 6 digits.',
      codeInvalid: 'The code is incorrect. Check the device clock and enter the latest code.',
      recoveryFormat: 'A recovery code has 8 letters or digits, for example ABCD-1234.',
      recoveryInvalid: 'The recovery code is incorrect or already used.',
      rateLimited: 'Too many attempts. Try again in a few minutes.',
      signInFailed: 'Could not sign in: {message}',
      registerFailed: 'Could not create the account: {message}',
      network: 'Could not reach the server. Check the network connection and try again.'
    },
    dev: {
      title: 'Development mode',
      user: 'Continue as a user',
      admin: 'Continue as an administrator'
    }
  },
  portal: {
    state: {
      retry: 'Try again'
    },
    home: {
      greeting: 'Hello, {name}',
      greetingAnonymous: 'Hello',
      summary: {
        active: 'Subscription active. {percent} of traffic remaining.',
        low: '{percent} of traffic remaining.',
        exhausted: 'Traffic exhausted. The subscription is paused.',
        expired: 'Subscription expired on {date}.',
        none: 'No subscription.'
      },
      next: {
        community: 'Contact the administrator to activate or renew.',
        commercial: 'Choose a plan.'
      },
      ring: {
        label: '{remaining} of {total} traffic left',
        caption: '{unit} left · {total} total',
        empty: 'No traffic available'
      },
      facts: {
        status: 'Status',
        expires: 'Expires',
        never: 'Never',
        daysLeft: 'In {n} day | In {n} days',
        lastDay: 'Today',
        ended: 'Ended',
        used: 'Used',
        upDown: '{up} up · {down} down'
      },
      status: {
        active: 'Active',
        expired: 'Expired',
        exhausted: 'Out of traffic',
        none: 'None'
      },
      plan: {
        community: 'Subscription template: {name}',
        commercial: 'Plan: {name}'
      },
      copyLink: 'Copy subscription link',
      copied: 'Subscription link copied',
      copyFailed: 'Could not copy automatically. Copy the link on the Subscription page.',
      importToClient: 'Import to a client',
      buyPlan: 'Choose a plan',
      orders: 'My orders',
      updatedAt: 'Updated {time}',
      refresh: 'Refresh',
      refreshing: 'Refreshing',
      loadFailed: 'Could not load your subscription.',
      help: {
        title: 'Help Center',
        all: 'All articles',
        empty: 'No help articles.'
      },
      tickets: {
        title: 'Recent tickets',
        new: 'New ticket',
        all: 'All tickets',
        empty: 'No tickets.'
      }
    },
    subscribe: {
      title: 'Subscription',
      description: 'Import the subscription link into a client. The link is a credential; do not share it.',
      link: 'Subscription link',
      linkHelp: 'Clients update the subscription on their own schedule.',
      domain: 'Subscription domain',
      copy: 'Copy',
      facts: {
        remaining: 'Traffic left',
        used: 'Used',
        expires: 'Expires'
      },
      qr: {
        label: 'QR code of your subscription link',
        caption: 'Scan with a client on your phone'
      },
      missing: 'No subscription link.',
      loadFailed: 'Could not load your subscription.',
      anx: {
        title: 'AnixOps Net (ANX edition)',
        hint: 'This service’s own client {version}, with the ANX protocol',
        os: { windows: 'Windows', macos: 'macOS', linux: 'Linux', android: 'Android', ios: 'iPhone / iPad' },
        kind: {
          windows: 'Installer',
          'windows-portable': 'Portable',
          macos: 'Installer',
          'macos-dmg': 'Disk image',
          'linux-deb': '.deb',
          linux: '.tar.gz',
          android: 'APK'
        },
        downloadLabel: 'Download {os} {name}',
        testflight: 'Join TestFlight',
        testflightAsk: 'Installed through TestFlight. Contact support for an invitation.',
        verify: 'Download links carry your subscription token. Do not share them. Checksums:'
      },
      clients: {
        title: 'Import to a client',
        hint: 'Opens the installed client',
        import: 'Import',
        copy: 'Copy link',
        copied: 'Copied the {name} link',
        copyFailed: 'Could not copy automatically. Copy the subscription link above manually.',
        importLabel: 'Import to {name}',
        copyLabel: 'Copy the {name} subscription link',
        allPlatforms: 'All platforms'
      },
      formats: {
        title: 'Other formats',
        description: 'Copy the link or preview the content in a format the client supports.',
        format: 'Format',
        preview: 'Preview',
        previewTitle: '{format} subscription content',
        previewDescription: 'The content a client receives. It includes node passwords; do not share it.',
        previewEmpty: 'The subscription is empty.',
        previewFailed: 'Could not load the subscription ({reason}).',
        download: 'Download',
        copyContent: 'Copy content',
        names: {
          auto: 'Automatic (by client)',
          v2ray: 'V2Ray (Base64)',
          clash: 'Clash (YAML)',
          stash: 'Stash (YAML)',
          egern: 'Egern (YAML)',
          surge: 'Surge',
          loon: 'Loon',
          shadowrocket: 'Shadowrocket',
          quantumultx: 'Quantumult X',
          singBox: 'sing-box (JSON)',
          wireguard: 'WireGuard (.conf)',
          iwan: 'iWAN (iwand config)',
          json: 'Raw JSON',
          base64json: 'Base64 JSON'
        }
      },
      danger: {
        title: 'Danger zone',
        resetTitle: 'Reset subscription link',
        resetDescription: 'The old link stops working immediately. Every device must import the new link.',
        resetAction: 'Reset link…',
        dialogTitle: 'Reset your subscription link?',
        dialogPassword: 'The old link stops working immediately. Enter the current password to confirm.',
        dialogCode: 'The old link stops working immediately. Enter the 6-digit code.',
        dialogRecovery: 'The old link stops working immediately. Enter a recovery code; each code can be used once.',
        checking: 'Checking two-factor authentication…',
        password: 'Current password',
        code: 'Verification code',
        recovery: 'Recovery code',
        useRecovery: 'Use a recovery code',
        useCode: 'Use a verification code',
        cancel: 'Cancel',
        confirm: 'Reset link',
        done: 'Subscription link reset. Import it again on all devices.',
        doneTitle: 'Subscription link reset',
        doneDescription: 'The old link is no longer valid. Import the new link on each device or scan the QR code.',
        newLink: 'New subscription link',
        close: 'Done',
        errors: {
          password: 'Enter your current password.',
          passwordWrong: 'The password is incorrect.',
          code: 'Enter the 6-digit code.',
          codeWrong: 'The code is incorrect or has expired.',
          recoveryFormat: 'A recovery code has 8 letters or digits, for example ABCD-1234.',
          recoveryWrong: 'The recovery code is incorrect or already used.',
          rateLimited: 'Too many attempts. Try again in an hour.',
          network: 'Cannot reach the server',
          failed: 'The link was not reset: {message}'
        }
      }
    },
    help: {
      title: 'Help Center',
      description: 'Guides, answers and announcements.',
      search: 'Search help articles',
      categories: 'Categories',
      all: 'All',
      uncategorized: 'Other',
      articles: '{n} article | {n} articles',
      results: 'Results for “{query}”',
      noResults: 'No articles match “{query}”.',
      clearSearch: 'Clear search',
      empty: 'No help articles.',
      updated: 'Updated {date}',
      back: 'Help Center',
      prev: 'Previous',
      next: 'Next',
      toc: 'In this article',
      notFound: 'Article not found',
      notFoundHint: 'The article may have been removed or hidden.',
      loadFailed: 'Could not load the help articles.',
      contact: 'No answer found?',
      contactAction: 'Open a ticket'
    },
    tickets: {
      title: 'Tickets',
      description: 'Support requests and replies.',
      new: 'New ticket',
      list: 'My tickets',
      empty: 'No tickets',
      emptyHint: 'Open a ticket to contact support.',
      select: 'Select a ticket to read the conversation.',
      status: {
        open: 'Open',
        answered: 'Answered',
        closed: 'Closed'
      },
      priority: {
        label: 'Priority',
        low: 'Low',
        medium: 'Medium',
        high: 'High'
      },
      subject: 'Subject',
      subjectPlaceholder: 'Describe the problem in one line',
      message: 'Details',
      messageHelp: 'Include the device, client and time of the problem.',
      submit: 'Send ticket',
      created: 'Ticket sent.',
      reply: 'Reply',
      replyPlaceholder: 'Write a reply…',
      send: 'Send',
      sendHint: 'Press Ctrl + Enter to send',
      close: 'Close ticket…',
      closeTitle: 'Close the ticket “{subject}”?',
      closeMessage: 'A closed ticket cannot be replied to. Open a new ticket for further help.',
      closeConfirm: 'Close ticket',
      closed: 'Ticket closed',
      closedNote: 'This ticket is closed. Open a new ticket for further help.',
      me: 'You',
      support: 'Support',
      backToList: 'My tickets',
      number: '#{id}',
      updated: 'Updated {time}',
      errors: {
        subject: 'Enter a subject.',
        message: 'Describe the problem.',
        reply: 'Write a reply first.',
        load: 'Could not load your tickets.',
        detail: 'Could not open this ticket.',
        send: 'The reply was not sent: {message}',
        create: 'The ticket was not sent: {message}'
      }
    },
    plans: {
      title: 'Plans',
      description: 'Plans and renewal.',
      period: 'Billing period',
      periods: {
        month: 'Monthly',
        quarter: 'Quarterly',
        half_year: 'Every 6 months',
        year: 'Yearly',
        two_year: 'Every 2 years',
        three_year: 'Every 3 years',
        onetime: 'One-time'
      },
      per: {
        month: '/ month',
        quarter: '/ quarter',
        half_year: '/ 6 months',
        year: '/ year',
        two_year: '/ 2 years',
        three_year: '/ 3 years',
        onetime: 'one-time'
      },
      notOffered: 'Not offered for this period',
      traffic: '{value} of traffic',
      speed: 'Up to {value} Mbps',
      devices: 'Up to {n} device at a time | Up to {n} devices at a time',
      buy: 'Buy',
      empty: 'No plans on sale',
      emptyHint: 'Plans appear here once listed.',
      loadFailed: 'Could not load the plans.',
      checkout: {
        title: 'Review your order',
        plan: 'Plan',
        period: 'Billing period',
        coupon: 'Coupon code',
        couponPlaceholder: 'Optional',
        apply: 'Apply',
        remove: 'Remove',
        couponApplied: 'Coupon “{name}” applied',
        subtotal: 'Price',
        discount: 'Discount',
        total: 'Total',
        submit: 'Place order',
        back: 'Back',
        created: 'Order placed. Pay for it in Orders.',
        couponInvalid: 'The coupon is invalid or does not apply to this plan.',
        failed: 'The order was not placed: {message}'
      }
    },
    destinationStats: {
      loadFailed: 'Could not load the site statistics',
      empty: 'No sites visited in this period',
      summary: {
        label: 'Site statistics summary',
        total: 'Total traffic',
        split: 'Upload {upload} · Download {download}',
        topCategory: 'Top category',
        domains: 'Sites visited',
        connections: '{count} connections'
      },
      categories: {
        title: 'Traffic types',
        description: 'Classified by destination domain. Types are estimates.'
      },
      domains: {
        title: 'Sites visited',
        description: 'By registrable domain, most traffic first.',
        label: 'Sites visited',
        domain: 'Site',
        category: 'Type',
        total: 'Traffic',
        upload: 'Upload',
        download: 'Download',
        connections: 'Connections',
        other: 'Other (beyond the per-report limit)'
      },
      category: {
        video: 'Video',
        music: 'Music',
        messaging: 'Messaging',
        social: 'Social',
        gaming: 'Gaming',
        ai: 'AI',
        download: 'Downloads',
        web: 'Websites',
        unknown: 'Unknown'
      }
    },
    traffic: {
      title: 'Site statistics',
      description: 'Sites reached through the nodes and traffic by type. Only domains are recorded, not addresses or content.',
      rangeLabel: 'Period',
      range: {
        '24h': '24 hours',
        '7d': '7 days',
        '30d': '30 days'
      },
      retention: 'Records are kept for {days} days.',
      off: 'Site statistics are disabled. No new data is recorded.',
      empty: 'No sites visited in this period',
      emptyHint: 'Records appear within an hour of use.'
    },
    orders: {
      title: 'Orders',
      description: 'Purchases and payment status.',
      empty: 'No orders',
      emptyHint: 'Orders appear here after you choose a plan.',
      browse: 'Choose a plan',
      loadFailed: 'Could not load your orders.',
      detailFailed: 'Could not open the order.',
      status: {
        pending: 'Unpaid',
        paid: 'Paid',
        cancelled: 'Cancelled',
        completed: 'Completed',
        discounted: 'Credited',
        unknown: 'Unknown'
      },
      columns: {
        plan: 'Plan',
        period: 'Period',
        amount: 'Amount',
        status: 'Status',
        createdAt: 'Ordered'
      },
      unknownPlan: 'Plan no longer offered',
      details: 'Details',
      pay: 'Pay',
      payPending: 'Online payment is not available. Contact the administrator to pay.',
      detail: {
        title: 'Order details',
        tradeNo: 'Order number',
        status: 'Status',
        plan: 'Plan',
        period: 'Period',
        subtotal: 'Amount',
        discount: 'Discount',
        total: 'Paid',
        createdAt: 'Ordered',
        paidAt: 'Paid on'
      }
    }
  }
}
