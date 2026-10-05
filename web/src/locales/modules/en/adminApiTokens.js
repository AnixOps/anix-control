// Security → API tokens (security/ApiTokens.vue, ApiTokenCreate.vue): an
// administrator's personal access tokens for automation. Loads with adminPages.
export default {
  adminApiTokens: {
    title: 'API tokens',
    description: 'Personal access tokens for scripts, CI jobs and monitoring probes. A token acts as you on the administrator APIs, within its scope, until it expires or you revoke it.',
    guide: 'About API tokens (guide)',
    refresh: 'Refresh',
    quota: '{used} of {max} active tokens in use.',
    quotaReached: 'You hold the maximum of {max} active tokens. Revoke one to create another.',
    loadFailed: 'Couldn’t load the API tokens',
    copied: 'Token ID copied.',
    copyFailed: 'Couldn’t copy the token ID.',
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
      hintTitle: 'The last four characters of the token. The token itself is never shown again.'
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
      description: 'Create a token to let a script, CI job or monitoring probe call the administrator APIs without your password. It is shown once, when you create it.'
    },
    revoke: {
      title: 'Revoke “{name}”?',
      message: 'Anything using this token stops working at once: its next request is refused. This can’t be undone. Create a new token for the automation instead.',
      messageOther: 'It belongs to administrator #{id}. Anything using this token stops working at once: its next request is refused. This can’t be undone.',
      action: 'Revoke token',
      done: 'Revoked “{name}”.',
      already: '“{name}” was already revoked.',
      gone: 'This token no longer exists, or it isn’t yours. The list has been refreshed.',
      failed: 'Couldn’t revoke the token: {message}'
    },
    create: {
      action: 'Create token',
      title: 'Create an API token',
      description: 'A script, CI job or monitoring probe sends it instead of your password. It is shown once, right after you create it.',
      name: 'Name',
      namePlaceholder: 'nightly export',
      nameHelp: 'What it is for, for example “nightly export”. Up to {max} characters.',
      scope: 'Scope',
      scopeHelp: 'Prefer Read, a short expiry and one token per automation.',
      scopeText: {
        read: 'Read only: GET and HEAD requests on the administrator APIs. It can list and inspect but never change anything, and it can’t read a node’s API key or the Telegram bot token, the two reads that still answer a secret in clear.',
        admin: 'Everything you may do on the administrator APIs, changes included (and super administrator actions if you are one). It still can’t manage API tokens: that needs your signed-in session.'
      },
      expiry: 'Expires',
      expiryHelp: 'Set one and put the renewal in your calendar. 90 days is a good start.',
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
      neverNotice: 'A token without an expiry works until someone revokes it, even if it leaks. Choose an expiry unless you have a reason not to.',
      confirmTitle: 'Confirm it’s you',
      checking: 'Checking how to confirm…',
      passwordNote: 'Creating a token needs your current password.',
      password: 'Current password',
      codeNote: 'Two-step verification is on. Enter the 6-digit code from your authenticator app.',
      code: 'Authenticator code',
      useRecovery: 'Use a recovery code instead',
      recoveryNote: 'Enter one of your recovery codes. Each one works once.',
      recovery: 'Recovery code',
      useCode: 'Use an authenticator code instead',
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
        password: 'Enter your current password.',
        code: 'Enter the 6-digit code.',
        recoveryFormat: 'A recovery code looks like XXXX-XXXX.',
        passwordWrong: 'That password isn’t right.',
        codeWrong: 'That code isn’t right. Try the next one.',
        recoveryWrong: 'That recovery code isn’t valid, or was already used.',
        rateLimited: 'Too many failed attempts. Try again later.',
        rateLimitedWait: 'Too many failed attempts. Try again in {minutes} min.',
        signInAgain: 'Your sign-in is too old for this. Sign in again, then create the token within 10 minutes.',
        too_many_tokens: 'You already hold 25 active API tokens. Revoke one first.',
        not_an_administrator: 'Only an active administrator can create API tokens.',
        invalid_request: 'The token wasn’t created: {message}',
        failed: 'Couldn’t create the token: {message}'
      }
    },
    result: {
      title: 'Your new API token',
      description: 'Shown once. When you close this dialog it can’t be shown again.',
      warning: 'Copy it now and put it in the secret store of whatever will use it. Control keeps only a hash and can’t show it again. If you lose it, revoke it and create another.',
      label: 'API token',
      copy: 'Copy token',
      name: 'Name',
      scope: 'Scope',
      expires: 'Expires',
      never: 'Never',
      usageTitle: 'Using the token',
      usageIntro: 'Send it in the Authorization header, on the administrator APIs only (/api/v2/admin, /api/v3 and /api/v4).',
      usageLabel: 'Example request',
      usageNever: 'Never put it in a URL. A request with a token in its query string is refused and audited, and the URL has already reached the access logs: revoke that token.',
      done: 'Done'
    }
  }
}
