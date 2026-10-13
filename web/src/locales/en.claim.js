// The first administrator's claim page (/claim): its own message group,
// loaded with the page, so the sign-in page's first visit does not carry it
// (web/bundle-budget.json). Written for English readers.
export default {
  claim: {
    title: 'Create the first administrator',
    subtitle: 'This panel has no administrator yet. Choose the email and password you will sign in with.',
    loading: 'Checking the link…',
    email: 'Email',
    password: 'Password',
    passwordHelp: 'At least 6 characters.',
    confirm: 'Confirm password',
    submit: 'Create administrator',
    done: {
      title: 'Administrator created',
      subtitle: 'Signing you in…'
    },
    created: 'Administrator created. Sign in with your new password.',
    missing: {
      title: 'Open the claim link',
      body: 'This page needs the link the server printed. For a new link, run this on the server:'
    },
    none: {
      title: 'No claim in progress',
      body: 'There is nothing to set up here. If you manage this panel, sign in.'
    },
    expired: {
      title: 'This link has expired',
      body: 'A claim link works for 24 hours. Run this on the server, then open the new link it prints:'
    },
    invalid: {
      title: 'This link is not valid',
      body: 'A newer link may have replaced it. Run this on the server, then open the new link it prints:'
    },
    toSignIn: 'Go to sign in',
    errors: {
      passwordLong: 'Use a shorter password (at most 72 bytes).',
      emailTaken: 'An account with this email already exists. Use another email.',
      identity: 'The sign-in service is not reachable right now. Nothing was changed; try again in a minute.',
      failed: 'Could not create the administrator: {message}'
    }
  }
}
