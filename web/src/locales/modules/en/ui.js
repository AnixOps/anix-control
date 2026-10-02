// Built-in strings of the component library (web/src/ui/). Written for
// English readers in sentence case, not translated from the Chinese file;
// see the voice and tone rules in docs/reference/frontend-design.md.
export default {
  ui: {
    actions: {
      close: 'Close',
      cancel: 'Cancel',
      confirm: 'Confirm',
      undo: 'Undo',
      copy: 'Copy',
      copied: 'Copied',
      dismiss: 'Dismiss notification'
    },
    loading: 'Loading…',
    field: {
      required: 'Required'
    },
    password: {
      show: 'Show password'
    },
    secret: {
      show: 'Show value',
      masked: 'Hidden'
    },
    number: {
      increment: 'Increase',
      decrement: 'Decrease'
    },
    select: {
      placeholder: 'Choose an option',
      search: 'Search',
      empty: 'No matching options'
    },
    copy: {
      failed: 'Could not copy automatically. The text is selected; press Ctrl+C or ⌘C to copy it.'
    },
    toast: {
      region: 'Notifications',
      hotkeyHint: 'Press F8 to go to notifications.'
    },
    confirm: {
      title: 'Confirm action',
      typeToConfirm: 'Type {name} to confirm',
      failed: 'The action did not complete: {reason}'
    },
    status: {
      online: 'Online',
      offline: 'Offline',
      disabled: 'Disabled',
      pending: 'Pending',
      error: 'Error'
    },
    format: {
      duration: {
        day: '{n} day | {n} days',
        hour: '{n} hour | {n} hours',
        minute: '{n} minute | {n} minutes',
        second: '{n} second | {n} seconds'
      },
      durationShort: {
        day: '{n}d',
        hour: '{n}h',
        minute: '{n}m',
        second: '{n}s'
      }
    }
  }
}
