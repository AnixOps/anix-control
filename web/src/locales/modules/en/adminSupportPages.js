export default {
  adminCoupons: {
    title: 'Coupons',
    subtitle: 'Discount codes users enter at checkout.',
    actions: {
      createCoupon: 'New coupon',
      create: 'Create coupon'
    },
    filters: {
      search: 'Search code or name',
      type: 'Filter by type'
    },
    table: {
      label: 'Coupons',
      id: 'ID',
      code: 'Code',
      name: 'Name',
      type: 'Type',
      value: 'Value',
      usageCount: 'Usage',
      validity: 'Valid',
      unlimited: 'Unlimited'
    },
    types: {
      discount: 'Discount',
      fixed: 'Fixed amount',
      discountPercent: 'Discount (%)',
      fixedCents: 'Fixed amount (cents)'
    },
    fields: {
      code: 'Coupon code',
      name: 'Coupon name',
      type: 'Coupon type',
      discountValue: 'Discount value',
      fixedValue: 'Fixed amount (cents)',
      startTime: 'Start time',
      endTime: 'End time',
      limitUse: 'Usage limit'
    },
    modal: {
      title: 'New coupon'
    },
    help: {
      code: 'Saved in capitals.',
      startTime: 'Empty: from now.',
      endTime: 'Empty: 30 days from now.',
      limitUse: '-1 for unlimited.'
    },
    units: {
      cents: 'cents'
    },
    placeholders: {
      code: 'SUMMER2026',
      name: 'Summer sale'
    },
    empty: {
      title: 'No coupons yet',
      description: 'Create a code to give users a discount on their next order.'
    },
    confirm: {
      deleteTitle: 'Delete coupon {code}?',
      deleteMessage: 'Users can no longer redeem this code. This can’t be undone.',
      deleteAction: 'Delete coupon'
    },
    messages: {
      fetchFailed: 'Coupons didn’t load',
      codeRequired: 'Enter a coupon code',
      nameRequired: 'Enter a coupon name',
      createSuccess: 'Coupon created successfully',
      createFailed: 'Failed to create coupon',
      deleted: 'Coupon {code} deleted',
      deleteFailed: 'Failed to delete coupon'
    }
  },
  adminOrders: {
    title: 'Orders',
    subtitle: 'Every purchase, renewal and upgrade. Select an order to see its payment.',
    stats: {
      totalOrders: 'Total orders',
      pendingOrders: 'Pending orders',
      totalRevenue: 'Total revenue',
      todayRevenue: 'Revenue today'
    },
    filters: {
      tradeNo: 'Order number',
      email: 'User email',
      label: 'Filter by status'
    },
    status: {
      pending: 'Pending',
      paid: 'Paid',
      cancelled: 'Cancelled',
      completed: 'Completed',
      unknown: 'Unknown'
    },
    actions: {
      markPaid: 'Mark paid'
    },
    table: {
      label: 'Orders',
      tradeNo: 'Order #',
      user: 'User',
      plan: 'Plan',
      period: 'Billing',
      amount: 'Amount',
      status: 'Status',
      createdAt: 'Created at'
    },
    detailModal: {
      title: 'Order details',
      orderSection: 'Order',
      paymentSection: 'Payment',
      tradeNo: 'Order number',
      userEmail: 'User email',
      plan: 'Plan',
      period: 'Billing period',
      amount: 'Amount',
      type: 'Order type',
      createdAt: 'Created at',
      paidAt: 'Paid at',
      callbackNo: 'Callback number'
    },
    types: {
      new: 'New purchase',
      renew: 'Renewal',
      upgrade: 'Upgrade',
      resetTraffic: 'Traffic reset',
      unknown: 'Unknown'
    },
    empty: {
      title: 'No orders yet',
      description: 'Orders appear here when users buy or renew a plan.'
    },
    confirm: {
      markPaidTitle: 'Mark order {tradeNo} as paid?',
      markPaidMessage: 'Order amount {amount}. The order is then treated as paid. This can’t be undone.',
      cancelTitle: 'Cancel order {tradeNo}?',
      cancelMessage: 'The user can no longer pay this order. This can’t be undone.',
      cancelAction: 'Cancel order',
      keepOrder: 'Keep order'
    },
    messages: {
      fetchOrdersFailed: 'Orders didn’t load',
      fetchStatsFailed: 'Failed to load order stats',
      markPaidSuccess: 'Order marked as paid',
      markPaidFailedShort: 'Mark paid failed',
      cancelSuccess: 'Order {tradeNo} cancelled',
      cancelFailedShort: 'Cancel failed'
    }
  },
  adminInviteCodes: {
    title: 'Invite codes',
    subtitle: 'Generate, copy and revoke the codes that admit a registration.',
    rewardsLink: 'Referrals',
    never: 'Never',
    empty: {
      title: 'No invite codes',
      description: 'Generate a batch and hand the codes to the people you want to sign up.'
    },
    registration: {
      requiredBadge: 'Required',
      optionalBadge: 'Optional',
      required: 'Registration currently requires an invite code (auth.registration.require_invite).',
      optional: 'Registration does not require an invite code; a code given at sign-up is still consumed.'
    },
    generate: {
      title: 'Generate codes',
      description: 'Each code admits one registration.',
      count: 'How many',
      countHelp: '1 to {max} at a time.',
      expireDays: 'Expires after',
      daysUnit: 'days',
      expireDaysPlaceholder: 'Configured default',
      expireDaysHelp: 'Empty: the configured expiry. 0: never expires.',
      submit: 'Generate',
      created: '{count} codes generated:'
    },
    filters: {
      label: 'Filter by status'
    },
    status: {
      unused: 'Unused',
      used: 'Used',
      expired: 'Expired'
    },
    owner: {
      admin: 'Administrator',
      user: 'User #{id}'
    },
    table: {
      label: 'Invite codes',
      code: 'Code',
      owner: 'Created by',
      status: 'Status',
      usedBy: 'Used by',
      expiresAt: 'Expires',
      createdAt: 'Created'
    },
    actions: {
      copy: 'Copy',
      copyAll: 'Copy all',
      revoke: 'Revoke',
      revokeOne: 'Revoke code…',
      done: 'Done'
    },
    confirm: {
      revokeTitle: 'Revoke invite code {code}?',
      revokeManyTitle: 'Revoke {count} invite codes?',
      revokeMessage: 'A revoked code can no longer be used to register. This can’t be undone.'
    },
    messages: {
      failed: 'Request failed',
      fetchFailed: 'Invite codes didn’t load',
      countRange: 'Generate between 1 and {max} codes at a time.',
      generated: '{count} codes generated',
      generateFailed: 'Failed to generate codes: {message}',
      revoked: 'Code {code} revoked',
      revokedMany: '{count} codes revoked',
      revokeFailed: 'Failed to revoke the code: {message}',
      copied: 'Copied',
      copyFailed: 'Copy failed'
    }
  },
  adminInvite: {
    title: 'Referrals',
    subtitle: 'Review commission withdrawals, see who invites the most, and set the referral rules.',
    currencySymbol: '¥',
    tabs: {
      label: 'Referral sections',
      config: 'Rules',
      withdrawals: 'Withdrawals',
      stats: 'Stats'
    },
    config: {
      inviteTitle: 'Invite codes',
      enabled: 'Enable invite rewards',
      codePrefix: 'Invite code prefix',
      codeLength: 'Invite code length',
      codeLengthHelp: '4 to 16 characters.',
      commissionTitle: 'Commission',
      commissionRate: 'Commission rate',
      commissionRateHelp: 'Percentage of each qualified order returned to the inviter.',
      commissionType: 'Commission type',
      commissionFixed: 'Fixed commission amount',
      withdrawTitle: 'Withdrawals',
      minWithdraw: 'Minimum withdrawal amount',
      withdrawFee: 'Withdrawal fee',
      withdrawMethods: 'Withdrawal methods'
    },
    types: {
      percent: 'Percent',
      fixed: 'Fixed amount'
    },
    methods: {
      alipay: 'Alipay',
      wechat: 'WeChat Pay',
      bank: 'Bank transfer'
    },
    actions: {
      approve: 'Approve',
      reject: 'Reject'
    },
    placeholders: {
      codePrefix: 'INV'
    },
    status: {
      pending: 'Pending',
      approved: 'Approved',
      rejected: 'Rejected'
    },
    withdrawals: {
      label: 'Withdrawal requests',
      rowName: 'withdrawal #{id}',
      filters: {
        label: 'Filter by status'
      },
      table: {
        id: 'ID',
        userId: 'User ID',
        amount: 'Amount',
        method: 'Method',
        account: 'Account',
        status: 'Status',
        createdAt: 'Requested at'
      },
      empty: 'No withdrawal requests',
      emptyDescription: 'When an inviter asks to withdraw their commission, the request appears here for review.'
    },
    stats: {
      totalInvites: 'Total invites',
      totalCommission: 'Total commission',
      pendingCommission: 'Pending commission',
      withdrawnCommission: 'Withdrawn commission',
      rankingTitle: 'Top inviters',
      empty: 'No ranking data',
      table: {
        rank: 'Rank',
        userId: 'User ID',
        inviteCount: 'Invite count',
        commission: 'Commission'
      }
    },
    confirm: {
      approveTitle: 'Approve withdrawal #{id}?',
      approveMessage: '{amount} will be paid to {account}. This can’t be undone.',
      rejectTitle: 'Reject withdrawal #{id}?',
      rejectMessage: 'The request for {amount} will be rejected. This can’t be undone.'
    },
    messages: {
      fetchConfigFailed: 'Failed to load invite config',
      fetchWithdrawalsFailed: 'Failed to load withdrawal requests',
      fetchStatsFailed: 'Failed to load invite stats',
      saveSuccess: 'Invite config saved',
      saveFailed: 'Failed to save invite config: {message}',
      saveFailedShort: 'Save failed',
      approveSuccess: 'Withdrawal approved',
      approveFailedShort: 'Approval failed',
      rejectSuccess: 'Withdrawal rejected',
      rejectFailedShort: 'Rejection failed'
    }
  },
  adminTelegram: {
    title: 'Telegram Bot',
    subtitle: 'Manage bot settings, bound users, and message delivery.',
    tabs: {
      config: 'Configuration',
      users: 'Users',
      notify: 'Notifications'
    },
    config: {
      basicTitle: 'Bot Configuration',
      token: 'Bot token',
      webhookUrl: 'Webhook URL',
      adminIds: 'Admin Telegram IDs',
      welcomeMessage: 'Welcome message'
    },
    actions: {
      setWebhook: 'Set Webhook',
      deleteWebhook: 'Delete Webhook',
      refreshUsers: 'Refresh Users',
      toggleNotify: 'Toggle notifications',
      enableNotifyLabel: 'Enable',
      disableNotifyLabel: 'Disable',
      send: 'Send',
      broadcast: 'Broadcast'
    },
    commands: {
      title: 'Supported Commands',
      items: {
        start: 'Start the bot',
        bind: 'Bind account',
        unbind: 'Unbind account',
        info: 'Show account info',
        sub: 'Show subscription link',
        renew: 'Show renewal entry',
        ticket: 'Create support ticket',
        help: 'Show help'
      }
    },
    users: {
      searchPlaceholder: 'Search by Telegram ID or email',
      empty: 'No bound Telegram users',
      table: {
        id: 'ID',
        telegramId: 'Telegram ID',
        userId: 'User ID',
        userEmail: 'User email',
        boundAt: 'Bound at',
        notifyStatus: 'Notify status',
        actions: 'Actions'
      }
    },
    status: {
      enabled: 'Enabled',
      disabled: 'Disabled'
    },
    notify: {
      type: 'Delivery type',
      telegramId: 'Telegram ID',
      message: 'Message',
      types: {
        single: 'Single user',
        broadcast: 'Broadcast'
      }
    },
    placeholders: {
      token: '123456:ABCDEF...',
      adminIds: '123456789,987654321',
      welcomeMessage: 'Welcome to the bot.',
      telegramId: 'Enter Telegram ID',
      message: 'Enter message content'
    },
    tips: {
      title: 'Broadcast Tips',
      broadcastScope: 'Broadcast only reaches users who have bound Telegram.',
      markdown: 'Messages support Telegram Markdown where available.',
      singleFirst: 'Test with a single user before broadcasting.'
    },
    confirm: {
      broadcastTitle: 'Broadcast this message to every bound user?',
      broadcastMessage: 'It goes to everyone who has bound Telegram and can’t be recalled once sent.'
    },
    messages: {
      fetchConfigFailed: 'Failed to load Telegram bot config',
      fetchUsersFailed: 'Failed to load Telegram users',
      saveSuccess: 'Telegram bot config saved',
      saveFailed: 'Failed to save Telegram bot config: {message}',
      saveFailedShort: 'Save failed',
      webhookSetSuccess: 'Webhook configured successfully',
      webhookSetFailed: 'Failed to set webhook: {message}',
      webhookSetFailedShort: 'Set webhook failed',
      webhookDeleteSuccess: 'Webhook deleted successfully',
      webhookDeleteFailed: 'Failed to delete webhook: {message}',
      webhookDeleteFailedShort: 'Delete webhook failed',
      toggleNotifyFailed: 'Failed to update notify status: {message}',
      toggleNotifyFailedShort: 'Update failed',
      messageRequired: 'Please enter a message',
      telegramIdRequired: 'Please enter a Telegram ID',
      sendSuccess: 'Message sent',
      sendFailed: 'Failed to send message: {message}',
      sendFailedShort: 'Send failed',
      broadcastComplete: 'Broadcast finished. Success: {success}, failed: {failed}'
    }
  },
  adminTickets: {
    title: 'Tickets',
    subtitle: 'Every user ticket in one queue. Pick one to reply or close it.',
    list: 'Ticket queue',
    loading: 'Loading tickets…',
    select: 'Select a ticket to reply to it.',
    backToList: 'All tickets',
    noSubject: 'No subject',
    meta: '#{id} · user {user} · {level}',
    priority: '{level} priority',
    threadNote: 'Earlier messages aren’t available to administrators here. Your reply reaches the user on their Tickets page and marks the ticket as answered.',
    closedNote: 'This ticket is closed and takes no more replies.',
    filters: {
      search: 'Search subject, #number or user ID',
      label: 'Filter by status',
      clear: 'Clear filters'
    },
    facts: {
      number: 'Ticket',
      user: 'User ID',
      created: 'Opened',
      updated: 'Last update'
    },
    actions: {
      closeTicket: 'Close ticket…',
      sendReply: 'Send reply'
    },
    empty: {
      title: 'No tickets',
      description: 'When users ask for help, their tickets show up here.',
      noMatches: 'No tickets match'
    },
    levels: {
      low: 'Low',
      medium: 'Medium',
      high: 'High',
      unknown: 'Unknown'
    },
    status: {
      open: 'Open',
      answered: 'Answered',
      closed: 'Closed',
      unknown: 'Unknown'
    },
    reply: {
      content: 'Reply',
      placeholder: 'Write a reply to the user',
      hint: 'Ctrl+Enter or ⌘Enter sends it.'
    },
    quick: {
      label: 'Quick replies',
      receivedLabel: 'Looking into it',
      received: 'Thanks for reaching out. We’re looking into this and will update you here.',
      detailsLabel: 'Ask for details',
      details: 'Could you tell us which client and node you use, and when the problem started?',
      fixedLabel: 'Fixed',
      fixed: 'This should be fixed now. Please refresh your subscription and let us know if it still happens.'
    },
    confirm: {
      closeTitle: 'Close ticket #{id} “{subject}”?',
      closeMessage: 'A closed ticket takes no more replies and can’t be reopened.',
      closeAction: 'Close ticket'
    },
    messages: {
      fetchFailed: 'Tickets didn’t load',
      replyRequired: 'Write a reply first',
      replySuccess: 'Reply sent to ticket #{id}',
      replyFailed: 'The reply wasn’t sent',
      closed: 'Ticket #{id} closed',
      closeFailed: 'The ticket wasn’t closed'
    }
  },
  adminKnowledge: {
    title: 'Help Center content',
    subtitle: 'Announcements, tutorials and answers your users read in the Help Center.',
    actions: {
      createArticle: 'New article',
      publish: 'Publish',
      show: 'Show in Help Center',
      hide: 'Hide from Help Center',
      delete: 'Delete article…'
    },
    table: {
      label: 'Articles',
      updatedAt: 'Updated'
    },
    filters: {
      search: 'Search articles',
      label: 'Filter by category'
    },
    categories: {
      announcement: 'Announcement',
      tutorial: 'Tutorial',
      faq: 'FAQ',
      other: 'Other'
    },
    visibility: {
      visible: 'Visible',
      hidden: 'Hidden'
    },
    fields: {
      title: 'Title',
      category: 'Category',
      content: 'Content',
      sort: 'Order',
      sortHelp: 'Lower numbers come first.',
      visibility: 'Show in Help Center',
      visibilityHelp: 'Hidden articles stay here but users don’t see them.'
    },
    placeholders: {
      title: 'Article title',
      body: 'Write the article. ## starts a section, - a list item.'
    },
    editor: {
      view: 'Editor view',
      write: 'Write',
      preview: 'Preview',
      previewEmpty: 'The preview appears as you write.',
      syntax: '## heading, - list, 1. numbered list, **bold**, `code`, > quote, [text](https://…).'
    },
    modal: {
      createTitle: 'New article',
      editTitle: 'Edit article',
      description: 'The preview shows the article as users see it in the Help Center.'
    },
    empty: {
      title: 'No articles yet',
      description: 'Write the first article and users will find it in the Help Center.'
    },
    confirm: {
      deleteTitle: 'Delete article "{title}"?',
      deleteMessage: 'The article is removed from the help center and users can no longer see it. This can’t be undone.',
      deleteAction: 'Delete article'
    },
    messages: {
      fetchFailed: 'Articles didn’t load',
      requiredFields: 'Add a title and the article text',
      titleRequired: 'Add a title',
      bodyRequired: 'Write the article text',
      saveSuccess: 'Article saved',
      publishSuccess: 'Article published',
      actionFailed: 'The article wasn’t saved',
      shown: '“{title}” is visible in the Help Center',
      hidden: '“{title}” is hidden',
      deleted: 'Article "{title}" deleted',
      deleteFailed: 'The article wasn’t deleted'
    }
  }
}
