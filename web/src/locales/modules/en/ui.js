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
    otp: {
      digit: 'Digit {n} of {total}'
    },
    qr: {
      failed: 'QR code unavailable'
    },
    chart: {
      loading: 'Loading {label}…',
      loadFailed: 'Couldn’t load the chart',
      empty: 'No data yet',
      showTable: 'View as table',
      showChart: 'View as chart'
    },
    table: {
      settings: 'Table settings',
      columns: 'Columns',
      density: 'Row height',
      densities: {
        comfortable: 'Comfortable',
        compact: 'Compact'
      },
      actions: 'Actions',
      rowActions: 'Actions for {name}',
      selectAll: 'Select all rows on this page',
      selectRow: 'Select {name}',
      selected: '{count} selected',
      selectAllMatching: 'Select all {count}',
      clearSelection: 'Clear selection',
      bulkActions: 'Actions for the selected rows',
      loading: 'Loading {label}…',
      loadFailed: 'This list didn’t load',
      empty: 'Nothing here yet',
      noMatches: 'No results',
      noMatchesHint: 'Try other words or clear the filters.',
      clearFilters: 'Clear filters'
    },
    pagination: {
      label: 'Pages',
      labelFor: 'Pages of {name}',
      range: '{from}–{to} of {total}',
      previous: 'Previous page',
      next: 'Next page',
      page: 'Page {page}',
      compact: '{page} / {pages}'
    },
    search: {
      clear: 'Clear search'
    },
    error: {
      retry: 'Try again',
      copyDetails: 'Copy error details',
      detailsCopied: 'Error details copied'
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
