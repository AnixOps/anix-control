// App shell strings (UI redesign phase U3): navigation, account menu,
// command palette, status pages and the account page. Written for English
// readers, not translated from the Chinese file.
export default {
  shell: {
    admin: {
      navLabel: 'Admin navigation',
      sidebarLabel: 'Sidebar',
      collapse: 'Collapse sidebar',
      expand: 'Expand sidebar',
      breadcrumb: 'Location',
      groups: {
        overview: 'Overview',
        users: 'Users',
        network: 'Network',
        extensions: 'Extensions',
        system: 'System',
        commerce: 'Commerce'
      },
      items: {
        dashboard: 'Dashboard',
        monitor: 'Traffic & monitoring',
        users: 'Users',
        inviteCodes: 'Invite codes',
        subscriptions: 'Subscription groups',
        templates: 'Subscription templates',
        tickets: 'Tickets',
        knowledge: 'Help Center content',
        nodes: 'Nodes',
        forward: 'Forwarding',
        agents: 'NodeX Agents',
        plugins: 'Plugin Center',
        deployments: 'Deployments',
        settings: 'Settings',
        security: 'Security',
        notifications: 'Notifications',
        plans: 'Plans',
        orders: 'Orders',
        coupons: 'Coupons',
        payment: 'Payments',
        invite: 'Referrals',
        account: 'Account'
      }
    },
    user: {
      navLabel: 'Main navigation',
      items: {
        dashboard: 'Overview',
        subscribe: 'Subscription',
        knowledge: 'Help Center',
        knowledgeShort: 'Help',
        tickets: 'Tickets',
        account: 'Account',
        plans: 'Plans',
        orders: 'Orders'
      }
    },
    search: {
      button: 'Search or jump to…',
      label: 'Search or jump to ({shortcut})'
    },
    account: {
      menuLabel: 'Account menu: {name}',
      account: 'Account',
      appearance: 'Appearance',
      language: 'Language',
      about: 'About',
      logout: 'Sign out',
      roleAdmin: 'Administrator',
      roleUser: 'User',
      themes: {
        system: 'System',
        light: 'Light',
        dark: 'Dark'
      }
    },
    about: {
      title: 'About AnixOps Control',
      version: 'Version',
      backendBuild: 'Backend built',
      commit: 'Commit',
      frontendBuild: 'Frontend build',
      frontendTime: 'Frontend built',
      unavailable: 'Version information is not available right now.',
      close: 'Done'
    },
    palette: {
      title: 'Search or jump to',
      description: 'Type a page, an action or a user’s email, then press Return.',
      placeholder: 'Search pages, actions or user emails…',
      empty: 'No results',
      searching: 'Looking up users…',
      searchFailed: 'Couldn’t look up users',
      groups: {
        actions: 'Actions',
        pages: 'Pages',
        users: 'Users'
      },
      hints: {
        move: 'select',
        open: 'open',
        close: 'close'
      },
      actions: {
        addNode: 'Add a node',
        addUser: 'Add a user',
        newForwardRoute: 'New forwarding route',
        themeLight: 'Use light appearance',
        themeDark: 'Use dark appearance',
        themeSystem: 'Match system appearance',
        language: 'Switch language: {language}'
      },
      userResult: 'Show in the user list'
    },
    status: {
      notFound: {
        title: 'Page not found',
        description: 'The link may be wrong, or the page has moved.'
      },
      forbidden: {
        title: 'You don’t have access to this page',
        description: 'Your account can’t open this page. Ask an administrator if you need it.'
      },
      home: 'Go to home',
      back: 'Go back',
      code: 'Error {code}'
    },
    accountPage: {
      title: 'Account',
      subtitle: 'Profile, two-factor authentication, language and appearance.',
      profile: {
        title: 'Profile',
        email: 'Email',
        id: 'User ID',
        role: 'Role'
      },
      mfa: {
        title: 'Two-factor authentication',
        description: 'Signing in asks for a 6-digit code from your authenticator app as well as your password.',
        status: 'Status',
        on: 'On',
        off: 'Off',
        backupCodes: 'Recovery codes',
        remaining: '{count} left',
        none: 'None',
        lastUsed: 'Last used',
        enable: 'Turn on two-factor authentication',
        disable: 'Turn off…',
        regenerate: 'New recovery codes…',
        loadFailed: 'Couldn’t load the two-factor status.',
        retry: 'Try again',
        setup: {
          title: 'Turn on two-factor authentication',
          description: 'Add this account to an authenticator app (such as 1Password or Google Authenticator), then enter the code it shows.',
          secret: 'Setup key',
          secretHelp: 'In the app, choose “Enter a setup key” and paste this key.',
          openApp: 'Open in an authenticator on this device',
          scan: 'Scan the QR code with your authenticator app',
          qrLabel: 'QR code for setting up two-factor authentication',
          manual: 'Can’t scan? Enter the setup key instead',
          enterCode: 'Enter the 6-digit code the app shows',
          code: '6-digit code',
          codeHelp: 'The app shows a new code every 30 seconds.',
          confirm: 'Verify and turn on',
          preparing: 'Creating a setup key…',
          enabled: 'Two-factor authentication is on'
        },
        backup: {
          title: 'Save your recovery codes',
          description: 'If you lose your phone, sign in with a recovery code. Each code works once; keep them somewhere safe.',
          copyAll: 'Copy all',
          download: 'Download',
          done: 'I’ve saved them'
        },
        disableDialog: {
          title: 'Turn off two-factor authentication?',
          description: 'Signing in will only need your password. Enter your current password to confirm.',
          password: 'Current password',
          confirm: 'Turn off',
          done: 'Two-factor authentication is off'
        },
        regenerateDialog: {
          title: 'Create new recovery codes?',
          description: 'Your current recovery codes stop working right away.',
          confirm: 'Create new codes'
        },
        errors: {
          code: 'Enter the 6-digit code',
          password: 'Enter your current password',
          failed: 'That didn’t work: {message}'
        }
      },
      language: {
        title: 'Language',
        label: 'Display language'
      },
      appearance: {
        title: 'Appearance',
        label: 'Theme'
      }
    }
  }
}
