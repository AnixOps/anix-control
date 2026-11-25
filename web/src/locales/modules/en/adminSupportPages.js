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
      title: 'No coupons',
      description: 'Create a discount code for users to apply at checkout.'
    },
    confirm: {
      deleteTitle: 'Delete coupon {code}?',
      deleteMessage: 'The code can no longer be redeemed. This cannot be undone.',
      deleteAction: 'Delete coupon'
    },
    messages: {
      fetchFailed: 'Could not load the coupons',
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
    subtitle: 'All purchases, renewals and upgrades. Select an order to view its payment.',
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
      title: 'No orders',
      description: 'Plan purchases and renewals appear here.'
    },
    confirm: {
      markPaidTitle: 'Mark order {tradeNo} as paid?',
      markPaidMessage: 'Order amount {amount}. The order is then treated as paid. This cannot be undone.',
      cancelTitle: 'Cancel order {tradeNo}?',
      cancelMessage: 'The order can no longer be paid. This cannot be undone.',
      cancelAction: 'Cancel order',
      keepOrder: 'Keep order'
    },
    messages: {
      fetchOrdersFailed: 'Could not load the orders',
      fetchStatsFailed: 'Failed to load order stats',
      markPaidSuccess: 'Order marked as paid',
      markPaidFailedShort: 'Mark paid failed',
      cancelSuccess: 'Order {tradeNo} cancelled',
      cancelFailedShort: 'Cancel failed'
    }
  },
  // The result of a bulk action (users, invite codes): the wording of the
  // codes the bulk routes answer per item, and 重试.
  adminBulk: {
    retry: 'Retry {count}',
    list: '{details}.',
    separator: '; ',
    errors: {
      not_found: '{count} not found',
      conflict: '{count} already used and kept',
      forbidden_self: 'your own account cannot be banned',
      not_attempted: '{count} not attempted (out of time)',
      failed: '{count} failed'
    }
  },
  adminInviteCodes: {
    title: 'Invite codes',
    subtitle: 'Generate, copy and revoke registration invite codes.',
    rewardsLink: 'Referrals',
    never: 'Never',
    empty: {
      title: 'No invite codes',
      description: 'Generate a batch of codes for registration.'
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
      expireDaysHelp: 'Empty uses the configured expiry; 0 never expires.',
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
      revokeMessage: 'A revoked code can no longer be used to register. This cannot be undone.'
    },
    messages: {
      failed: 'Request failed',
      fetchFailed: 'Could not load the invite codes',
      countRange: 'Generate between 1 and {max} codes at a time.',
      generated: '{count} codes generated',
      generateFailed: 'Failed to generate codes: {message}',
      revoked: 'Code {code} revoked',
      revokedMany: '{count} codes revoked',
      revokePartial: 'Revoked {done} of {total} codes.',
      revokeNone: 'No code was revoked.',
      revokeFailed: 'Failed to revoke the code: {message}',
      copied: 'Copied',
      copyFailed: 'Copy failed'
    }
  },
  adminInvite: {
    title: 'Referrals',
    subtitle: 'Commission withdrawals, inviter ranking and referral rules.',
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
      emptyDescription: 'Commission withdrawal requests awaiting review appear here.'
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
      approveMessage: '{amount} will be paid to {account}. This cannot be undone.',
      rejectTitle: 'Reject withdrawal #{id}?',
      rejectMessage: 'The request for {amount} will be rejected. This cannot be undone.'
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
  adminTickets: {
    title: 'Tickets',
    subtitle: 'All user tickets. Select a ticket to reply or close it.',
    list: 'Ticket queue',
    loading: 'Loading tickets…',
    select: 'Select a ticket to reply to it.',
    backToList: 'All tickets',
    noSubject: 'No subject',
    meta: '#{id} · user {user} · {level}',
    priority: '{level} priority',
    threadNote: 'Earlier messages are not shown here. The reply appears on the user’s Tickets page and marks the ticket as answered.',
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
      description: 'User tickets appear here.',
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
      received: 'Thank you for the report. We are investigating and will post updates here.',
      detailsLabel: 'Ask for details',
      details: 'Please provide the client and node in use and when the problem started.',
      fixedLabel: 'Fixed',
      fixed: 'The issue has been resolved. Please update the subscription and reply if it persists.'
    },
    confirm: {
      closeTitle: 'Close ticket #{id} “{subject}”?',
      closeMessage: 'A closed ticket cannot be replied to or reopened.',
      closeAction: 'Close ticket'
    },
    messages: {
      fetchFailed: 'Could not load the tickets',
      replyRequired: 'Write a reply first',
      replySuccess: 'Reply sent to ticket #{id}',
      replyFailed: 'Could not send the reply',
      closed: 'Ticket #{id} closed',
      closeFailed: 'Could not close the ticket'
    }
  },
  adminKnowledge: {
    title: 'Help Center content',
    subtitle: 'Help Center announcements, tutorials and answers.',
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
      visibilityHelp: 'Hidden articles are kept but not shown to users.'
    },
    placeholders: {
      title: 'Article title',
      body: 'Article body. ## starts a section, - a list item.'
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
      description: 'Shown as in the Help Center.'
    },
    empty: {
      title: 'No articles',
      description: 'Published articles appear in the Help Center.'
    },
    confirm: {
      deleteTitle: 'Delete article "{title}"?',
      deleteMessage: 'The article is removed from the Help Center. This cannot be undone.',
      deleteAction: 'Delete article'
    },
    messages: {
      fetchFailed: 'Could not load the articles',
      requiredFields: 'Add a title and the article text',
      titleRequired: 'Add a title',
      bodyRequired: 'Write the article text',
      saveSuccess: 'Article saved',
      publishSuccess: 'Article published',
      actionFailed: 'Could not save the article',
      shown: '“{title}” is visible in the Help Center',
      hidden: '“{title}” is hidden',
      deleted: 'Article "{title}" deleted',
      deleteFailed: 'Could not delete the article'
    }
  }
}
