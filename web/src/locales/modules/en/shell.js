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
      // The banner for the certificates the panel serves itself.
      certificate: {
        failed: 'The certificate for {name} could not be obtained. A temporary certificate is in use; nodes cannot join',
        pending: 'Requesting the certificate for {name}. A temporary certificate is in use; nodes cannot join',
        renewalFailing: 'The certificate for {name} could not be renewed. The current certificate expires {date}',
        untrusted: 'The certificate for {name} is not trusted by public certificate authorities. Nodes cannot join',
        reason: 'Reason: {reason}',
        retry: 'Request again',
        retryStarted: 'Requested again. The result will appear here.',
        retryWaiting: 'Three requests were made within the hour. Next request: {date}.',
        retryFailed: 'The request was not sent. Try again later, or run anix-control tls retry on the server.',
        fixes: {
          dns_unresolved: 'Add an A (and AAAA) record for {name} pointing to this machine, disable the CDN proxy, then request again.',
          dns_elsewhere: '{name} does not resolve to this machine. Point the record here and disable the CDN proxy. To keep a CDN, set tls.mode: external and set grpc.public_name to a DNS-only name that resolves here.',
          unreachable: 'The certificate authority cannot reach {name} on ports 443 and 80. Open both ports in the firewall and security group, and stop other programs that use them.',
          challenge_failed: 'The certificate authority did not receive a response from this panel at {name}. Point the record here with the proxy disabled, or forward /.well-known/acme-challenge/ to the panel.',
          rate_limited: 'The certificate authority rate limit was reached. The panel retries automatically. Do not reinstall: the certificate and account are stored in the database.',
          ca_unreachable: 'This machine cannot reach the certificate authority. Check outbound HTTPS (port 443) and DNS.',
          ca_error: 'The panel retries automatically. Run anix-control tls status on the server for details.',
          file_unreadable: 'Check that the certificate and key files exist, are readable and contain a matching PEM chain and key. Changes are reloaded automatically.',
          expired: 'The certificate has expired. Replace it with a renewed certificate, or resolve the reason above and request again.',
          name_mismatch: 'The certificate does not cover {name}. Use a certificate issued for this name.',
          untrusted: 'Use a certificate from a public certificate authority. With domain set and tls.mode: acme, the panel obtains one automatically.'
        }
      },
      groups: {
        overview: 'Overview',
        users: 'Users',
        network: 'Network',
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
        lines: 'Lines',
        topology: 'Topology',
        mesh: 'Mesh',
        forward: 'Forwarding',
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
        traffic: 'Site statistics',
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
      source: 'Source code',
      revealAddresses: 'Show server IPs',
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
      unavailable: 'Version information is unavailable.',
      close: 'Done'
    },
    palette: {
      title: 'Search or jump to',
      description: 'Type a page, an action or a user’s email, then press Return.',
      placeholder: 'Search pages, actions or user emails…',
      empty: 'No results',
      searching: 'Looking up users…',
      searchFailed: 'Could not look up users',
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
        addNode: 'Join a node',
        addUser: 'Add a user',
        newForwardRoute: 'New forwarding route',
        upgradeNodes: 'Upgrade nodes',
        themeLight: 'Use light appearance',
        themeDark: 'Use dark appearance',
        themeSystem: 'Match system appearance',
        revealAddresses: 'Show server IP addresses',
        hideAddresses: 'Hide server IP addresses',
        language: 'Switch language: {language}'
      },
      userResult: 'Show in the user list'
    },
    status: {
      notFound: {
        title: 'Page not found',
        description: 'The link is incorrect or the page has moved.'
      },
      forbidden: {
        title: 'Access denied',
        description: 'This account does not have access to this page. Contact an administrator for access.'
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
        description: 'Sign-in requires a password and a 6-digit code from an authenticator app.',
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
        loadFailed: 'Could not load the two-factor status.',
        retry: 'Try again',
        setup: {
          title: 'Turn on two-factor authentication',
          description: 'Add this account to an authenticator app (such as 1Password or Google Authenticator), then enter the code.',
          secret: 'Setup key',
          secretHelp: 'In the authenticator app, choose “Enter a setup key” and paste this key.',
          openApp: 'Open in an authenticator on this device',
          scan: 'Scan the QR code with an authenticator app',
          qrLabel: 'QR code for setting up two-factor authentication',
          manual: 'Enter the setup key manually',
          enterCode: 'Enter the 6-digit authenticator code',
          code: '6-digit code',
          codeHelp: 'The code changes every 30 seconds.',
          confirm: 'Verify and turn on',
          preparing: 'Creating a setup key…',
          enabled: 'Two-factor authentication is on'
        },
        backup: {
          title: 'Save the recovery codes',
          description: 'Use a recovery code to sign in without the authenticator. Each code can be used once. Store them securely.',
          copyAll: 'Copy all',
          download: 'Download',
          done: 'Saved'
        },
        disableDialog: {
          title: 'Turn off two-factor authentication?',
          description: 'Sign-in will require only the password. Enter the current password to confirm.',
          password: 'Current password',
          confirm: 'Turn off',
          done: 'Two-factor authentication is off'
        },
        regenerateDialog: {
          title: 'Create new recovery codes?',
          description: 'The current recovery codes stop working immediately.',
          confirm: 'Create new codes'
        },
        errors: {
          code: 'Enter the 6-digit code',
          password: 'Enter the current password',
          failed: 'The operation did not complete: {message}'
        }
      },
      telegram: {
        title: 'Telegram',
        description: 'When linked, the Telegram bot shows the account and subscription and sends expiry and traffic reminders.',
        status: 'Status',
        bound: 'Linked',
        notBound: 'Not linked',
        account: 'Telegram account',
        bind: 'Link Telegram',
        unbind: 'Unlink',
        bindDisabled: 'Self-service linking is disabled. Contact an administrator.',
        loadFailed: 'Could not load the Telegram link',
        retry: 'Retry',
        unbindTitle: 'Unlink Telegram?',
        unbindMessage: 'Telegram reminders stop and the bot can no longer access this account.',
        unbound: 'Telegram unlinked',
        bindDone: 'Telegram linked',
        failed: 'Failed: {message}',
        dialog: {
          title: 'Link Telegram',
          description: 'Scan the QR code or open Telegram, then press Start in the bot. The link is valid for 10 minutes and links only the current account.',
          preparing: 'Preparing the link…',
          qrLabel: 'Telegram link QR code',
          open: 'Open Telegram',
          manual: 'Or send this command to the bot',
          waiting: 'Waiting for Start in Telegram…',
          expires: 'Valid until {time}',
          done: 'Done'
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
