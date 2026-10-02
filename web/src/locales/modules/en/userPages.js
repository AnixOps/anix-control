// Sign-in and the user pages (UI redesign phase U5): sign in, Overview,
// Subscription, Help Center, Tickets, Plans, Orders. Written for English
// readers, not translated from the Chinese file
// (guidelines/voice-and-tone.md: sentence case, verbs on buttons).
export default {
  auth: {
    signIn: {
      title: 'Sign in to AnixOps Control',
      subtitle: 'Manage your subscription, nodes and forwards.',
      email: 'Email',
      password: 'Password',
      submit: 'Sign in',
      noAccount: 'New here?',
      register: 'Create an account'
    },
    register: {
      title: 'Create your account',
      subtitle: 'Sign up to get your subscription link.',
      email: 'Email',
      password: 'Password',
      passwordHelp: 'At least 6 characters.',
      confirm: 'Confirm password',
      invite: 'Invite code',
      inviteHelp: 'No invite code? Ask your administrator for one.',
      submit: 'Create account',
      haveAccount: 'Already have an account?',
      signIn: 'Sign in',
      done: 'Account created'
    },
    mfa: {
      title: 'Two-factor authentication',
      description: 'Enter the 6-digit code your authenticator app shows for {email}.',
      code: '6-digit code',
      submit: 'Verify',
      back: 'Back',
      useRecovery: 'Use a recovery code',
      recoveryTitle: 'Use a recovery code',
      recoveryDescription: 'Enter one of the recovery codes you saved when you turned on two-factor authentication. Each code works once.',
      recovery: 'Recovery code',
      useCode: 'Use your authenticator instead'
    },
    enroll: {
      title: 'Turn on two-factor authentication first',
      description: 'Your administrator requires two-factor authentication to sign in, and {email} doesn’t have it on yet.',
      stepsTitle: 'What to do',
      step1: 'Ask your administrator to pause the requirement in Two-factor policy.',
      step2: 'Sign in, open Account, turn on two-factor authentication and save your recovery codes.',
      step3: 'Let your administrator know, so they can turn the requirement back on.',
      app: 'You need an authenticator app, such as 1Password, Google Authenticator or Microsoft Authenticator.',
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
      codeInvalid: 'That code is not correct. Check that your phone’s clock is right, then enter the newest code.',
      recoveryFormat: 'A recovery code has 8 letters or digits, like ABCD-1234.',
      recoveryInvalid: 'That recovery code is not correct or was already used.',
      rateLimited: 'Too many attempts. Try again in a few minutes.',
      signInFailed: 'Could not sign in: {message}',
      registerFailed: 'Could not create the account: {message}',
      network: 'Could not reach the server. Check your connection and try again.'
    },
    dev: {
      title: 'Development mode',
      user: 'Continue as a user',
      admin: 'Continue as an administrator'
    }
  },
  portal: {
    state: {
      retry: 'Try again',
      copyDetails: 'Copy error details',
      detailsCopied: 'Error details copied'
    },
    home: {
      greeting: 'Hello, {name}',
      greetingAnonymous: 'Hello',
      summary: {
        active: 'Your subscription is active, with {percent} of your traffic left.',
        low: 'Only {percent} of your traffic is left.',
        exhausted: 'You’ve used all your traffic, so the subscription is paused.',
        expired: 'Your subscription expired on {date}.',
        none: 'You don’t have a subscription yet.'
      },
      next: {
        community: 'Ask your administrator to set one up or renew it.',
        commercial: 'Choose a plan to get started.'
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
      copyFailed: 'Could not copy automatically. Open Subscription to copy the link by hand.',
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
        empty: 'No help articles yet.'
      },
      tickets: {
        title: 'Recent tickets',
        new: 'New ticket',
        all: 'All tickets',
        empty: 'No tickets yet. When something goes wrong, contact support here.'
      }
    },
    subscribe: {
      title: 'Subscription',
      description: 'Import your subscription link into a client to use every node. Treat the link like a password and don’t share it.',
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
      missing: 'You don’t have a subscription link yet.',
      loadFailed: 'Could not load your subscription.',
      clients: {
        title: 'Import to a client',
        hint: 'Opens the client if it’s installed',
        import: 'Import',
        copy: 'Copy link',
        copied: 'Copied the {name} link',
        copyFailed: 'Could not copy automatically. Copy the subscription link above by hand.',
        importLabel: 'Import to {name}',
        copyLabel: 'Copy the {name} subscription link',
        allPlatforms: 'All platforms'
      },
      formats: {
        title: 'Other formats',
        description: 'If your client isn’t listed above, pick a format it supports, then copy the link or preview the content.',
        format: 'Format',
        preview: 'Preview',
        previewTitle: '{format} subscription content',
        previewDescription: 'This is what a client receives. It includes node passwords, so don’t share it.',
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
          json: 'Raw JSON',
          base64json: 'Base64 JSON'
        }
      },
      danger: {
        title: 'Danger zone',
        resetTitle: 'Reset subscription link',
        resetDescription: 'If your link leaks, a reset stops the old link at once, and every device has to import the new one.',
        resetAction: 'Reset link…',
        dialogTitle: 'Reset your subscription link?',
        dialogPassword: 'The old link stops working at once. Enter your current password to confirm it’s you.',
        dialogCode: 'The old link stops working at once. Enter the 6-digit code from your authenticator app.',
        dialogRecovery: 'The old link stops working at once. Enter one of your recovery codes; each code works once.',
        checking: 'Checking two-factor authentication…',
        password: 'Current password',
        code: 'Verification code',
        recovery: 'Recovery code',
        useRecovery: 'Use a recovery code',
        useCode: 'Use a verification code',
        cancel: 'Cancel',
        confirm: 'Reset link',
        done: 'Subscription link reset. Import it again on all your devices.',
        doneTitle: 'Subscription link reset',
        doneDescription: 'The old link no longer works. Import the new link on each device, or scan the QR code.',
        newLink: 'New subscription link',
        close: 'Done',
        errors: {
          password: 'Enter your current password.',
          passwordWrong: 'That password is not correct.',
          code: 'Enter the 6-digit code.',
          codeWrong: 'That code is not correct or has expired.',
          recoveryFormat: 'A recovery code has 8 letters or digits, like ABCD-1234.',
          recoveryWrong: 'That recovery code is not correct or was already used.',
          rateLimited: 'Too many attempts. Try again in an hour.',
          network: 'Can’t reach the server',
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
      noResults: 'No articles match “{query}”. Try another word.',
      clearSearch: 'Clear search',
      empty: 'No help articles yet. They appear here once your administrator publishes them.',
      updated: 'Updated {date}',
      back: 'Help Center',
      prev: 'Previous',
      next: 'Next',
      toc: 'In this article',
      notFound: 'Article not found',
      notFoundHint: 'It may have been removed or hidden.',
      loadFailed: 'Could not load the help articles.',
      contact: 'Didn’t find an answer?',
      contactAction: 'Open a ticket'
    },
    tickets: {
      title: 'Tickets',
      description: 'Contact support when something goes wrong. Replies show up here.',
      new: 'New ticket',
      list: 'My tickets',
      empty: 'No tickets yet',
      emptyHint: 'When something goes wrong, open a ticket to contact support.',
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
      messageHelp: 'Include your device, client and when it happened, so support can find the cause faster.',
      submit: 'Send ticket',
      created: 'Ticket sent. Support will reply soon.',
      reply: 'Reply',
      replyPlaceholder: 'Write a reply…',
      send: 'Send',
      sendHint: 'Press Ctrl + Enter to send',
      close: 'Close ticket…',
      closeTitle: 'Close the ticket “{subject}”?',
      closeMessage: 'You can’t reply after closing it. Open a new ticket if you need more help.',
      closeConfirm: 'Close ticket',
      closed: 'Ticket closed',
      closedNote: 'This ticket is closed. Open a new ticket if you need more help.',
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
      description: 'Pick the plan that fits. You can renew any time before it expires.',
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
      emptyHint: 'Plans appear here once your administrator lists them.',
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
        couponInvalid: 'This coupon is not valid for this plan.',
        failed: 'The order was not placed: {message}'
      }
    },
    orders: {
      title: 'Orders',
      description: 'What you bought and whether it’s paid.',
      empty: 'No orders yet',
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
      payPending: 'Online payment isn’t available yet. Contact your administrator to pay.',
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
