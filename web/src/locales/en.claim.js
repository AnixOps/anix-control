// The first administrator's claim page (/claim): its own message group,
// loaded with the page, so the sign-in page's first visit does not carry it
// (web/bundle-budget.json). Written for English readers.
export default {
  claim: {
    pageTitle: 'Create the first administrator',
    signInHint: 'No administrator exists. Run {command} on the server and open the printed link.',
    title: 'Create the first administrator',
    subtitle: 'Set the administrator sign-in email and password.',
    loading: 'Checking the link…',
    email: 'Email',
    password: 'Password',
    passwordHelp: 'At least {min} characters.',
    confirm: 'Confirm password',
    submit: 'Create administrator',
    done: {
      title: 'Administrator created',
      subtitle: 'Signing in…'
    },
    created: 'Administrator created. Sign in with the new password.',
    missing: {
      title: 'Open the claim link',
      body: 'This page requires the claim link printed by the server. Run this on the server to obtain one:'
    },
    none: {
      title: 'No claim in progress',
      body: 'Nothing to set up. Administrators can sign in.'
    },
    expired: {
      title: 'This link has expired',
      body: 'A claim link is valid for 24 hours. Run this on the server to obtain a new link:'
    },
    invalid: {
      title: 'This link is not valid',
      body: 'The link may have been replaced. Run this on the server to obtain a new link:'
    },
    toSignIn: 'Go to sign in',
    errors: {
      passwordShort: 'Use at least {min} characters.',
      passwordLong: 'The password must be at most 72 bytes.',
      emailTaken: 'This email is already in use.',
      identity: 'The sign-in service is unavailable. Nothing was changed. Try again later.',
      failed: 'Could not create the administrator: {message}'
    }
  }
}
