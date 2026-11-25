// Security → API tokens (security/ApiTokens.vue, ApiTokenCreate.vue): an
// administrator's personal access tokens for automation. Loads with adminPages.
export default {
  adminApiTokens: {
    title: 'API tokens',
    description: 'Personal access tokens for scripts, CI jobs and monitoring probes. A token acts as its owner on the administrator APIs, within its scope, until it expires or is revoked.',
    guide: 'About API tokens (guide)',
    refresh: 'Refresh',
    quota: '{used} of {max} active tokens in use.',
    quotaReached: 'The limit of {max} active tokens is reached. Revoke one to create another.',
    loadFailed: 'Could not load the API tokens',
    copied: 'Token ID copied.',
    copyFailed: 'Could not copy the token ID.',
    filters: {
      label: 'Also show',
      ended: 'Revoked and expired',
      all: 'All administrators',
      superOnly: 'Only super administrators can list other administrators’ tokens.'
    },
    table: {
      label: 'API tokens',
      name: 'Name',
      owner: 'Owner',
      scope: 'Scope',
      status: 'Status',
      expires: 'Expires',
      lastUsed: 'Last used',
      createdOn: 'Created {date}',
      hintTitle: 'Last four characters of the token. The full token is not shown again.'
    },
    owner: {
      you: 'You',
      admin: 'Administrator #{id}'
    },
    scopes: {
      read: 'Read',
      admin: 'Admin'
    },
    states: {
      active: 'Active',
      expiring: 'Expires soon',
      expired: 'Expired',
      revoked: 'Revoked'
    },
    expires: {
      never: 'No expiry',
      expired: 'Expired {when}',
      revoked: 'Revoked {when}'
    },
    lastUsed: {
      never: 'Never used'
    },
    revokeReasons: {
      owner_revoked: 'Revoked by its owner',
      admin_revoked: 'Revoked by a super administrator',
      owner_not_admin: 'Revoked: its owner was banned, demoted or deleted',
      unknown: 'Revoked'
    },
    actions: {
      copyId: 'Copy token ID',
      revoke: 'Revoke token…'
    },
    empty: {
      title: 'No API tokens',
      description: 'Scripts, CI jobs and monitoring probes use tokens to call the administrator APIs. A token is shown only once, at creation.'
    },
    revoke: {
      title: 'Revoke “{name}”?',
      message: 'Anything using this token stops working immediately. This cannot be undone.',
      messageOther: 'It belongs to administrator #{id}. Anything using this token stops working immediately. This cannot be undone.',
      action: 'Revoke token',
      done: 'Revoked “{name}”.',
      already: '“{name}” was already revoked.',
      gone: 'The token no longer exists or belongs to another administrator. The list has been refreshed.',
      failed: 'Could not revoke the token: {message}'
    },
    create: {
      action: 'Create token',
      title: 'Create an API token',
      description: 'Used by scripts, CI jobs or monitoring probes instead of a password. Shown only once, after creation.',
      name: 'Name',
      namePlaceholder: 'nightly export',
      nameHelp: 'Purpose, for example “nightly export”. Up to {max} characters.',
      scope: 'Scope',
      scopeHelp: 'Prefer Read, a short expiry and one token per automation.',
      scopeText: {
        read: 'Read only: GET and HEAD requests on the administrator APIs. Cannot change anything or read the Telegram bot token.',
        admin: 'The owner’s full permissions on the administrator APIs, including changes and super administrator actions. Cannot manage API tokens.'
      },
      expiry: 'Expires',
      expiryHelp: 'Set an expiry; 90 days is recommended.',
      expiryOptions: {
        d30: '30 days',
        d90: '90 days (recommended)',
        d180: '180 days',
        d365: '1 year (365 days)',
        d730: '2 years (730 days, the longest)',
        custom: 'Custom…',
        never: 'No expiry (not recommended)'
      },
      customDays: 'Days until it expires',
      customDaysHelp: 'A whole number from 1 to {max}.',
      daysUnit: 'days',
      neverNotice: 'A token without an expiry stays valid until revoked. Set an expiry unless there is a specific reason not to.',
      cancel: 'Cancel',
      submit: 'Create token',
      signInAgain: 'Sign in again',
      errors: {
        name: {
          required: 'Enter a name.',
          tooLong: 'Use at most 100 characters.',
          unprintable: 'Use printable characters only.'
        },
        expiry: 'Enter a whole number of days from 1 to 730.',
        signInAgain: 'The session is too old. Sign in again and create the token within 10 minutes.',
        too_many_tokens: 'The limit of 25 active API tokens is reached. Revoke one first.',
        not_an_administrator: 'Only an active administrator can create API tokens.',
        invalid_request: 'Could not create the token: {message}',
        failed: 'Could not create the token: {message}'
      }
    },
    result: {
      title: 'New API token',
      description: 'Shown only once. It cannot be viewed again after this dialog closes.',
      warning: 'Copy it now into a secret store. If it is lost, revoke it and create another.',
      label: 'API token',
      copy: 'Copy token',
      name: 'Name',
      scope: 'Scope',
      expires: 'Expires',
      never: 'Never',
      usageTitle: 'Using the token',
      usageIntro: 'Send it in the Authorization header, on the administrator APIs only (/api/v2/admin, /api/v3 and /api/v4).',
      usageLabel: 'Example request',
      usageNever: 'Never put it in a URL. A request with a token in its query string is refused and audited; revoke that token.',
      done: 'Done'
    }
  }
}
