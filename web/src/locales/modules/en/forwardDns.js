// Entry high availability through DNS in the forwarding pages (L2): the
// DNS providers page, the route editor's binding picker and the route
// page's 入口高可用 card. Same keys as zh-CN/forwardDns.js.
export default {
  forwardDns: {
    nav: 'DNS',
    superOnly: 'Only a super administrator may add, change or delete DNS providers.',
    page: {
      title: 'DNS providers',
      description: 'The DNS accounts Control writes entry records through: it keeps each bound entry hostname on the healthy entry nodes.',
      add: 'Add provider',
      readOnly: 'You can view the providers. Only a super administrator may add, change or delete them.',
      loadFailed: 'Could not load the DNS providers',
      emptyTitle: 'No DNS providers yet',
      emptyDescription: 'Add the account of the DNS service that holds your entry hostnames, then bind a route’s entry hostname to it in the route editor.',
      defaultEndpoint: 'Provider default',
      missing: 'Missing: {names}',
      bindingCount: '{n} binding | {n} bindings',
      unused: 'Not used',
      note: 'Credentials are write-only: Control seals them and never shows them again. Only their names are listed.',
      edit: 'Edit…',
      editSuperOnly: 'Edit (super administrators only)',
      delete: 'Delete…',
      deleteSuperOnly: 'Delete (super administrators only)',
      deleteInUse: 'Delete (used by {n} binding) | Delete (used by {n} bindings)',
      columns: {
        name: 'Provider',
        bindings: 'Bindings',
        endpoint: 'Endpoint',
        credentials: 'Credentials',
        updated: 'Updated'
      }
    },
    sheet: {
      createTitle: 'Add a DNS provider',
      editTitle: 'Edit {name}',
      description: 'Give the credential access to the one zone Control writes, and nothing else.',
      name: 'Name',
      namePlaceholder: 'cloudflare-main',
      kind: 'Provider',
      kindFixed: 'A provider’s kind cannot change. Add another provider instead.',
      settings: 'Settings',
      credentials: 'Credentials',
      credentialsHelp: 'Stored sealed on Control and never shown again.',
      credentialsKeep: 'Stored credentials show as ********. Leave them to keep the stored values, or type a new value to replace one.',
      stored: 'Stored. Leave as is to keep it.',
      create: 'Add provider',
      save: 'Save'
    },
    kindHelp: {
      DNS_PROVIDER_KIND_CLOUDFLARE: 'An API token with Zone → DNS → Edit and Zone → Zone → Read on your zone.',
      DNS_PROVIDER_KIND_ALIDNS: 'A RAM user’s AccessKey with the alidns record actions on your domain.',
      DNS_PROVIDER_KIND_DNSPOD: 'A CAM sub-user’s SecretId and SecretKey with the dnspod record actions.',
      DNS_PROVIDER_KIND_HUAWEICLOUD: 'An IAM user’s access key (AK/SK) with the dns zone and recordset actions.',
      DNS_PROVIDER_KIND_WEBHOOK: 'Control POSTs every change, signed with HMAC-SHA256, to your HTTPS endpoint.'
    },
    config: {
      endpoint: 'API endpoint',
      url: 'Webhook URL'
    },
    configHelp: {
      endpoint: 'Optional: a regional API host instead of the provider’s default.',
      url: 'An https:// URL.'
    },
    configPlaceholder: {
      DNS_PROVIDER_KIND_CLOUDFLARE: { endpoint: 'api.cloudflare.com' },
      DNS_PROVIDER_KIND_ALIDNS: { endpoint: 'alidns.aliyuncs.com' },
      DNS_PROVIDER_KIND_DNSPOD: { endpoint: 'dnspod.tencentcloudapi.com' },
      DNS_PROVIDER_KIND_HUAWEICLOUD: { endpoint: 'dns.myhuaweicloud.com' },
      DNS_PROVIDER_KIND_WEBHOOK: { url: 'https://dns-hook.example.com/anixops' }
    },
    credential: {
      api_token: 'API token',
      access_key_id: 'AccessKey ID',
      access_key_secret: 'AccessKey secret',
      secret_id: 'SecretId',
      secret_key: 'Secret key',
      access_key: 'Access key (AK)',
      secret: 'Signing secret'
    },
    delete: {
      title: 'Delete the provider {name}?',
      message: 'Control forgets its credentials. Records it published stay at the provider.',
      confirm: 'Delete provider'
    },
    mode: {
      DNS_BINDING_MODE_DDNS: 'DDNS',
      DNS_BINDING_MODE_CNAME: 'CNAME'
    },
    modeHint: {
      DNS_BINDING_MODE_DDNS: 'Control writes the entry hostname’s records itself.',
      DNS_BINDING_MODE_CNAME: 'Control writes a name it manages; you point the entry hostname at it once.'
    },
    binding: {
      hostnameHelp: 'The one name clients use for this route’s entries. Bind it to a DNS provider below, or manage its records yourself.',
      enable: 'Keep this hostname on the healthy entries',
      enableHelp: 'Control updates the A/AAAA records through a DNS provider as entry nodes go down and come back.',
      bound: 'Bound to a DNS provider',
      provider: 'DNS provider',
      providerPlaceholder: 'Choose a provider',
      noProviders: 'No DNS provider yet.',
      manageProviders: 'Manage DNS providers',
      zone: 'Zone',
      zoneHelp: 'The provider’s zone the record is in.',
      mode: 'Mode',
      recordName: 'Managed name',
      recordNamePlaceholder: 'r1.ha.example.net',
      recordNameHelp: 'A name inside the zone that Control writes.',
      record: 'Record',
      ddnsRecord: 'Record:',
      recordTypes: 'Record types',
      recordTypesHelp: 'Dropping a type deletes its records.',
      type: {
        A: 'IPv4 addresses',
        AAAA: 'IPv6 addresses'
      },
      ttl: 'TTL',
      seconds: 's',
      ttlHelp: 'The shortest your provider allows (60 by default).',
      paused: 'Paused',
      pausedHelp: 'Keep the published records and change nothing.',
      cnameInstruction: 'Create this CNAME once where {host} is hosted: {host} → the managed name.',
      cnameTarget: 'CNAME target',
      copy: 'Copy',
      immutable: 'Provider, zone, name and mode cannot change. To move the binding, unbind it on the route page (deleting the records) and bind again.',
      mismatch: 'The binding writes {name}. In DDNS mode its name cannot change: unbind on the route page and bind the new hostname.',
      needsHostname: 'Set the entry hostname first.',
      required: 'Required',
      outsideZone: 'The name must be inside the zone.',
      typeRequired: 'Choose at least one record type.',
      ttlRange: 'From 1 to {max} seconds.',
      saveFailed: 'The route is saved, but its DNS binding was not: {message}'
    },
    card: {
      title: 'Entry high availability',
      description: 'The entry hostname’s DNS records follow the healthy entry nodes.',
      loadFailed: 'Could not load the DNS status',
      edit: 'Edit binding',
      unbind: 'Unbind…',
      unbindSuperOnly: 'Only a super administrator may unbind',
      unboundHost: '{host} is not bound to a DNS provider: you manage its records.',
      unboundNoHost: 'This route has no entry hostname. Set one in the editor to bind it to a DNS provider.',
      bind: 'Bind to DNS',
      setHostname: 'Set entry hostname',
      lastError: 'Last error',
      nextAttempt: 'Next attempt {when}',
      hostname: 'Entry hostname',
      mode: 'Mode',
      zone: 'Zone',
      ttl: 'TTL',
      ttlValue: '{n} s',
      published: 'Published',
      evaluated: 'Evaluated',
      never: 'Never',
      records: 'Records',
      type: 'Type',
      publishedValues: 'Published',
      desiredValues: 'Desired',
      none: 'None',
      keepPublished: 'None healthy: published kept',
      entries: 'Entry nodes',
      noAddress: 'No public address',
      inRotation: 'In rotation',
      outOfRotation: 'Out of rotation',
      joining: 'Healthy {n}/3, joining',
      leaving: 'Unhealthy {n}/3, leaving',
      timing: 'Control evaluates every 10 s. A node leaves after 3 unhealthy evaluations and rejoins after 3 healthy ones. This card refreshes every 30 s.'
    },
    unbind: {
      title: 'Unbind {host}?',
      description: 'Control stops maintaining the hostname’s records.',
      purge: 'Also delete the records Control published',
      purgeHelp: 'Leave unchecked to keep the records at the provider as they are.',
      purgeFailedHint: 'The binding is kept. Retry, or unbind without deleting the records.',
      confirm: 'Unbind'
    },
    state: {
      ok: 'OK',
      pending: 'Pending',
      degraded: 'Degraded',
      error: 'Error',
      rate_limited: 'Rate limited',
      paused: 'Paused',
      unbound: 'Not bound',
      route_missing: 'Route missing',
      hostname_mismatch: 'Hostname mismatch'
    },
    stateHelp: {
      ok: 'The records are the healthy entries’ addresses.',
      pending: 'Nothing published yet: no entry has been healthy since the binding was made.',
      degraded: 'No entry is healthy: Control keeps the last published records rather than an empty set.',
      error: 'The provider refused or failed. Control retries with back-off.',
      rate_limited: 'The provider’s call budget is spent; the change waits a few seconds.',
      paused: 'The binding, or the route, is paused or enforced: nothing changes.',
      unbound: 'Not bound to a DNS provider.',
      route_missing: 'The route was deleted; unbind the hostname.',
      hostname_mismatch: 'DDNS mode, and the route’s entry hostname is no longer the binding’s name. Unbind and bind again.'
    },
    reason: {
      healthy: 'Healthy',
      converging: 'Converging',
      not_in_inventory: 'Not in inventory',
      no_address: 'No public address',
      never_reported: 'Never reported',
      report_stale: 'Report stale',
      offline: 'Offline',
      hop_error: 'Hop error',
      upstreams_down: 'Upstreams down'
    },
    codes: {
      secret_store_unavailable: 'Control has no key-encryption key (module_runtime.ca_kek) to seal credentials with.',
      provider_in_use: 'A route binding uses this provider. Unbind it first.',
      binding_exists: 'Another binding already uses this route, or this provider, zone and name.',
      name_taken: 'Another provider has this name.',
      unknown_route: 'The route is not stored yet.',
      unknown_provider: 'The DNS provider no longer exists.',
      entry_hostname_required: 'The route has no entry hostname.',
      hostname_mismatch: 'In DDNS mode the record is the route’s entry hostname.',
      immutable: 'Provider, zone, name and mode cannot change; unbind and bind again.'
    },
    errors: {
      dns_purge_failed: 'The DNS provider did not delete the records.'
    },
    toast: {
      created: 'Added {name}',
      saved: 'Saved {name}',
      deleted: 'Deleted {name}',
      unbound: 'Unbound {host}'
    }
  }
}
