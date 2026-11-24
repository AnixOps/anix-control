// Entry high availability through DNS in the forwarding pages (L2): the
// DNS providers page, the route editor's binding picker and the route
// page's 入口高可用 card. Same keys as zh-CN/forwardDns.js.
export default {
  forwardDns: {
    nav: 'DNS',
    superOnly: 'Only super administrators can add, change or delete DNS providers.',
    page: {
      title: 'DNS providers',
      description: 'DNS accounts used to point bound entry hostnames at healthy entry nodes.',
      add: 'Add provider',
      readOnly: 'Read-only. Only super administrators can add, change or delete providers.',
      loadFailed: 'Could not load the DNS providers',
      emptyTitle: 'No DNS providers',
      emptyDescription: 'Add a DNS service account, then bind entry hostnames in the route editor.',
      defaultEndpoint: 'Provider default',
      missing: 'Missing: {names}',
      bindingCount: '{n} binding | {n} bindings',
      unused: 'Not used',
      note: 'Credentials are encrypted and never shown again; only names are listed.',
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
      description: 'Grant the credential access to the target zone only.',
      name: 'Name',
      namePlaceholder: 'cloudflare-main',
      kind: 'Provider',
      kindFixed: 'The provider kind cannot be changed after creation.',
      settings: 'Settings',
      credentials: 'Credentials',
      credentialsHelp: 'Stored encrypted and never shown again.',
      credentialsKeep: 'Stored credentials show as ********. Keep them to retain the values, or enter new values.',
      stored: 'Stored. Leave as is to keep it.',
      create: 'Add provider',
      save: 'Save'
    },
    kindHelp: {
      DNS_PROVIDER_KIND_CLOUDFLARE: 'An API token with Zone → DNS → Edit and Zone → Zone → Read on the zone.',
      DNS_PROVIDER_KIND_ALIDNS: 'A RAM user AccessKey limited to alidns record actions on the domain.',
      DNS_PROVIDER_KIND_DNSPOD: 'A CAM sub-user’s SecretId and SecretKey with the dnspod record actions.',
      DNS_PROVIDER_KIND_HUAWEICLOUD: 'An IAM user’s access key (AK/SK) with the dns zone and recordset actions.',
      DNS_PROVIDER_KIND_WEBHOOK: 'Each change is signed with HMAC-SHA256 and POSTed to the HTTPS endpoint.'
    },
    config: {
      endpoint: 'API endpoint',
      url: 'Webhook URL'
    },
    configHelp: {
      endpoint: 'Optional regional API host.',
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
      message: 'Its credentials are deleted. Published records remain at the provider.',
      confirm: 'Delete provider'
    },
    mode: {
      DNS_BINDING_MODE_DDNS: 'DDNS',
      DNS_BINDING_MODE_CNAME: 'CNAME'
    },
    modeHint: {
      DNS_BINDING_MODE_DDNS: 'Writes the entry hostname records directly.',
      DNS_BINDING_MODE_CNAME: 'Writes a managed name; the entry hostname needs a one-time CNAME to it.'
    },
    binding: {
      hostnameHelp: 'Common name clients use for the entries. Bind a DNS provider below, or manage the records manually.',
      enable: 'Point at healthy entries',
      enableHelp: 'A/AAAA records are updated as entry nodes fail and recover.',
      bound: 'Bound to a DNS provider',
      provider: 'DNS provider',
      providerPlaceholder: 'Choose a provider',
      noProviders: 'No DNS provider.',
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
      ttlHelp: 'Use the provider minimum (default 60).',
      vantage: 'Vantage node label',
      vantageHelp: 'Optional key=value label (such as vantage=cn). Nodes with this label probe each entry address; addresses most cannot reach are removed until they recover.',
      vantageFormat: 'Format: key=value, for example vantage=cn',
      paused: 'Paused',
      pausedHelp: 'Keep the published records unchanged.',
      cnameInstruction: 'Create this CNAME in the DNS of {host}: {host} → managed name.',
      cnameTarget: 'CNAME target',
      copy: 'Copy',
      immutable: 'Provider, zone, name and mode cannot be changed. To change them, unbind on the route page and bind again.',
      mismatch: 'This binding writes {name}, which cannot change in DDNS mode. Unbind on the route page and bind again.',
      needsHostname: 'Set the entry hostname first.',
      required: 'Required',
      saveBlocked: 'The DNS binding is incomplete: {fields}. Complete it or turn it off.',
      fieldsSeparator: ', ',
      incompleteShort: 'DNS binding incomplete',
      outsideZone: 'The name must be inside the zone.',
      typeRequired: 'Choose at least one record type.',
      ttlRange: 'From 1 to {max} seconds.',
      saveFailed: 'The route was saved, but the DNS binding was not: {message}'
    },
    card: {
      title: 'Entry high availability',
      description: 'The entry hostname’s DNS records follow the healthy entry nodes.',
      loadFailed: 'Could not load the DNS status',
      edit: 'Edit binding',
      unbind: 'Unbind…',
      unbindSuperOnly: 'Only a super administrator may unbind',
      unboundHost: '{host} is not bound to a DNS provider; records are managed manually.',
      unboundNoHost: 'This route has no entry hostname. Set one in the editor to bind it.',
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
      unreachable: 'Unreachable from vantage nodes: {addresses} ({n} vantage nodes); removed from the records',
      inRotation: 'In rotation',
      outOfRotation: 'Out of rotation',
      joining: 'Healthy {n}/3, joining',
      leaving: 'Unhealthy {n}/3, leaving',
      timing: 'Evaluated every 10 s: removed after 3 unhealthy results, restored after 3 healthy ones. Refreshes every 30 s.'
    },
    unbind: {
      title: 'Unbind {host}?',
      description: 'The hostname records will no longer be maintained.',
      purge: 'Also delete the records Control published',
      purgeHelp: 'Unchecked keeps the records.',
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
      pending: 'Not published: no healthy entry since binding.',
      degraded: 'No healthy entry; the last published records are kept.',
      error: 'The provider refused or failed; retrying with back-off.',
      rate_limited: 'The provider rate limit was reached; the change is delayed.',
      paused: 'The binding or route is paused or enforced; no changes are made.',
      unbound: 'Not bound to a DNS provider.',
      route_missing: 'The route was deleted; unbind the hostname.',
      hostname_mismatch: 'In DDNS mode, the entry hostname no longer matches the binding. Unbind and bind again.'
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
      upstreams_down: 'Upstreams down',
      vantage_unreachable: 'Unreachable from vantage'
    },
    codes: {
      secret_store_unavailable: 'No credential encryption key is configured (module_runtime.ca_kek).',
      provider_in_use: 'This provider is used by a route binding. Unbind it first.',
      binding_exists: 'Another binding already uses this route, or this provider, zone and name.',
      name_taken: 'Another provider has this name.',
      unknown_route: 'The route is not saved.',
      unknown_provider: 'The DNS provider no longer exists.',
      entry_hostname_required: 'The route has no entry hostname.',
      hostname_mismatch: 'In DDNS mode, the record name must be the route entry hostname.',
      immutable: 'Provider, zone, name and mode cannot be changed; unbind and bind again.'
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
