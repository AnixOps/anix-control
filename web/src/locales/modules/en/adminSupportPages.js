export default {
  adminCoupons: {
    title: 'Coupons',
    subtitle: 'Create and manage discount coupons for orders.',
    currencySymbol: '¥',
    actions: {
      createCoupon: 'Create Coupon'
    },
    table: {
      id: 'ID',
      code: 'Code',
      name: 'Name',
      type: 'Type',
      value: 'Value',
      usageCount: 'Usage',
      validity: 'Validity',
      actions: 'Actions',
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
      title: 'Create Coupon'
    },
    placeholders: {
      code: 'SUMMER2026',
      name: 'Summer sale'
    },
    empty: {
      noData: 'No coupons'
    },
    messages: {
      fetchFailed: 'Failed to load coupons',
      requiredFields: 'Please fill in coupon code and coupon name',
      createSuccess: 'Coupon created successfully',
      createFailed: 'Failed to create coupon',
      deleteConfirm: 'Delete coupon {code}?',
      deleteFailed: 'Failed to delete coupon'
    }
  },
  adminInvite: {
    title: 'Invite Rewards',
    subtitle: 'Configure invite commissions, review withdrawals, and track rankings.',
    currencySymbol: '¥',
    tabs: {
      config: 'Configuration',
      withdrawals: 'Withdrawals',
      stats: 'Stats'
    },
    config: {
      inviteTitle: 'Invite Configuration',
      enabled: 'Enable invite rewards',
      codePrefix: 'Invite code prefix',
      codeLength: 'Invite code length',
      commissionTitle: 'Commission Rules',
      commissionRate: 'Commission rate (%)',
      commissionRateHelp: 'Percentage of each qualified order returned to the inviter.',
      commissionType: 'Commission type',
      commissionFixed: 'Fixed commission amount',
      withdrawTitle: 'Withdrawal Rules',
      minWithdraw: 'Minimum withdrawal amount',
      withdrawFee: 'Withdrawal fee (%)',
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
      search: 'Search',
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
      filters: {
        all: 'All statuses'
      },
      table: {
        id: 'ID',
        userId: 'User ID',
        amount: 'Amount',
        method: 'Method',
        account: 'Account',
        status: 'Status',
        createdAt: 'Created at',
        actions: 'Actions'
      },
      empty: 'No withdrawal requests'
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
    messages: {
      fetchConfigFailed: 'Failed to load invite config',
      fetchWithdrawalsFailed: 'Failed to load withdrawal requests',
      fetchStatsFailed: 'Failed to load invite stats',
      saveSuccess: 'Invite config saved',
      saveFailed: 'Failed to save invite config: {message}',
      saveFailedShort: 'Save failed',
      approveConfirm: 'Approve this withdrawal request?',
      approveSuccess: 'Withdrawal approved',
      approveFailed: 'Failed to approve withdrawal: {message}',
      approveFailedShort: 'Approval failed',
      rejectConfirm: 'Reject this withdrawal request?',
      rejectSuccess: 'Withdrawal rejected',
      rejectFailed: 'Failed to reject withdrawal: {message}',
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
    subtitle: 'Review user tickets and reply from one queue.',
    icons: {
      open: '!',
      answered: '✓',
      closed: '×'
    },
    stats: {
      open: 'Open',
      answered: 'Answered',
      closed: 'Closed'
    },
    table: {
      id: 'ID',
      userId: 'User ID',
      subject: 'Subject',
      priority: 'Priority',
      status: 'Status',
      createdAt: 'Created at',
      actions: 'Actions'
    },
    actions: {
      reply: 'Reply',
      closeTicket: 'Close',
      sendReply: 'Send Reply'
    },
    empty: {
      noData: 'No tickets'
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
    replyModal: {
      title: 'Reply to Ticket #{id}',
      subject: 'Subject: ',
      userId: 'User ID: ',
      content: 'Reply content',
      placeholder: 'Write your reply...'
    },
    messages: {
      fetchFailed: 'Failed to load tickets',
      replyRequired: 'Please enter a reply',
      replySuccess: 'Reply sent successfully',
      replyFailed: 'Failed to send reply',
      closeConfirm: 'Close this ticket?',
      closeFailed: 'Failed to close ticket'
    }
  },
  adminKnowledge: {
    title: 'Knowledge Base',
    subtitle: 'Publish announcements, tutorials, FAQs, and support articles.',
    actions: {
      createArticle: 'Create Article',
      publish: 'Publish'
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
      sort: 'Sort',
      visibility: 'Visibility'
    },
    placeholders: {
      title: 'Article title',
      body: 'Write the article content...'
    },
    modal: {
      createTitle: 'Create Article',
      editTitle: 'Edit Article'
    },
    empty: {
      noData: 'No articles'
    },
    messages: {
      fetchFailed: 'Failed to load articles',
      requiredFields: 'Please fill in title and content',
      saveSuccess: 'Article saved successfully',
      publishSuccess: 'Article published successfully',
      actionFailed: 'Article operation failed',
      deleteConfirm: 'Delete article "{title}"?',
      deleteFailed: 'Failed to delete article'
    }
  }
}
