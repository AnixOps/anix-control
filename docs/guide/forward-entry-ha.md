# Forward Entry HA Through DNS

A forwarding route with several entry nodes has one name that clients use,
`listen.entry_hostname`. Control can keep that name's A/AAAA records on the
entry nodes that are healthy right now, through your DNS provider (L2,
[`../architecture/forward-sdk.md`](../architecture/forward-sdk.md) section
7.4). This guide covers:

- the credentials to create at each provider, with the least permissions;
- the web pages (转发 → DNS, the route editor and the route page);
- how to bind a route through the API or the command line;
- how to read the status.

The API is in [`../forwarding/v4-api.md`](../forwarding/v4-api.md), "Entry
HA through DNS".

## Before You Start

- **A key-encryption key.** Control seals provider credentials with
  `module_runtime.ca_kek`, the key that already seals the built-in CA.
  Without it, adding a provider is refused (`secret_store_unavailable`).
  The command line needs the same key in its configuration.
- **A super administrator.** Only a super administrator may add, change or
  delete a provider, or delete a binding. Every administrator may read them
  and create or change bindings.
- **A public address for every entry node.** Control publishes the node's
  forwarding settings' addresses (`PUT /api/v4/forward/nodes/{ref}/settings`),
  else its host when that is an IP address. Private, loopback and
  link-local addresses are never published. A node without a public
  address of the published family stays out (`no_address`).
- **A route with `entry_hostname`.** It is required on a route with
  several entry nodes, and a binding needs it.

## Choosing DDNS Or CNAME

- **DDNS.** The entry name is in a zone your provider account manages.
  Control writes `entry_hostname`'s records itself. The binding's
  `record_name` is `entry_hostname`.
- **CNAME.** The entry name is somewhere Control has no credentials, or you
  want one provider account for many customers' names. Control writes the
  records of a name it manages, for example `r1.ha.example.net`. You create
  the CNAME `hk.customer.example` → `r1.ha.example.net` once. The route's
  status shows the target as `cname_target`.

## Provider Credentials

Give each credential access to the one zone Control writes, and nothing
else. Store it nowhere but Control: Control never shows it again.

### Cloudflare

1. My Profile → API Tokens → Create Token → Create Custom Token.
2. Permissions:
   - Zone → DNS → Edit;
   - Zone → Zone → Read (Control looks the zone up by name).
3. Zone Resources: Include → Specific zone → your zone.
4. Optional: Client IP Address Filtering to Control's egress addresses.
5. Create the provider:

   ```json
   {"provider": {"name": "cloudflare-main", "kind": "DNS_PROVIDER_KIND_CLOUDFLARE"},
    "credentials": {"api_token": "<token>"}}
   ```

Cloudflare records are written unproxied (`proxied: false`): the entries
must receive the clients' connections themselves.

### Alibaba Cloud DNS

1. RAM console → Users → Create User, with OpenAPI access. Keep its
   AccessKey ID and secret.
2. Create a custom policy and attach it to the user only:

   ```json
   {
     "Version": "1",
     "Statement": [{
       "Effect": "Allow",
       "Action": [
         "alidns:DescribeSubDomainRecords",
         "alidns:AddDomainRecord",
         "alidns:UpdateDomainRecord",
         "alidns:DeleteDomainRecord"
       ],
       "Resource": "acs:alidns:*:*:domain/example.com"
     }]
   }
   ```

3. Create the provider with `kind` `DNS_PROVIDER_KIND_ALIDNS` and the
   credentials `access_key_id` and `access_key_secret`. `config.endpoint`
   may name a regional endpoint (`alidns.cn-hangzhou.aliyuncs.com`); the
   default is `alidns.aliyuncs.com`.

Alibaba Cloud DNS's free edition refuses a TTL under 600 seconds. Set the
binding's `ttl` to what your edition allows: the provider's refusal shows
in the route's `last_error`.

### DNSPod (Tencent Cloud)

1. CAM console → Users → Create sub-user, with programmatic access. Keep
   its SecretId and SecretKey.
2. Create a custom policy and attach it to the sub-user only:

   ```json
   {
     "version": "2.0",
     "statement": [{
       "effect": "allow",
       "action": [
         "dnspod:DescribeRecordList",
         "dnspod:CreateRecord",
         "dnspod:ModifyRecord",
         "dnspod:DeleteRecord"
       ],
       "resource": ["*"]
     }]
   }
   ```

   Where the console offers it, narrow `resource` to your domain.
3. Create the provider with `kind` `DNS_PROVIDER_KIND_DNSPOD` and the
   credentials `secret_id` and `secret_key`.

Control writes the default line (默认). DNSPod's free plan has a minimum
TTL of 600 seconds.

### Huawei Cloud DNS

1. IAM console → Users → Create User, with programmatic access. Download its
   access key (AK) and secret key (SK).
2. Create a custom policy for the global DNS service and grant it to the
   user only:

   ```json
   {
     "Version": "1.1",
     "Statement": [{
       "Effect": "Allow",
       "Action": [
         "dns:zone:list",
         "dns:recordset:list",
         "dns:recordset:create",
         "dns:recordset:update",
         "dns:recordset:delete"
       ]
     }]
   }
   ```

3. Create the provider with `kind` `DNS_PROVIDER_KIND_HUAWEICLOUD` and the
   credentials `access_key` and `secret_key`. The default endpoint is
   `dns.myhuaweicloud.com`; `config.endpoint` may name a regional one.

Huawei keeps one record set per name and type, so Control replaces the set
whole.

### Webhook

For a DNS service without a built-in provider, Control POSTs every change
to your HTTPS endpoint:

```text
POST <config.url>
Content-Type: application/json
X-AnixOps-Event: forward.dns
X-AnixOps-Timestamp: 1759579200
X-AnixOps-Signature: sha256=<hex HMAC-SHA256(secret, "1759579200" + "." + body)>

{"version":1,"action":"set","zone":"example.com","name":"hk.example.com",
 "type":"A","values":["203.0.113.11","203.0.113.14"],"ttl":60,"timestamp":1759579200}
```

- `action` `set` makes the name's records of that type exactly `values`;
  `delete` removes them. Make both idempotent.
- Verify the signature over the raw body, and refuse a timestamp more than
  5 minutes old.
- Answer any 2xx for success. Anything else is a failure: Control retries
  with back-off.
- Generate the secret with `openssl rand -hex 32`. Create the provider with
  `kind` `DNS_PROVIDER_KIND_WEBHOOK`, `config.url`, and the credential
  `secret`.

## In The Web App

1. **Add the provider.** 转发 → **DNS** → **添加服务商** (super
   administrator). Pick the kind; the form shows that kind's settings and
   credentials (`GET /api/v4/forward/dns/kinds`). Credentials are
   write-only: after saving, the list shows only their names, and editing a
   provider shows a stored one as `********`. Leave it to keep the stored
   value, or type a new one to replace it. A provider's kind cannot change.
2. **Bind the route.** Open the route's editor. With two or more entry
   nodes, 入口域名 has a binding section under it: turn on
   **让此主机名始终指向健康的入口**, then choose:
   - the provider and the zone (guessed from the hostname, and guessed
     again when you change the hostname, unless you typed a zone yourself).
     A field left empty is flagged "Required" only after you have been in
     it and left it, not as soon as you turn the section on. While the
     binding is incomplete the save button stays off, and one note under
     the switch says for what ("Save waits for the DNS binding: DNS provider.
     Complete it, or turn the binding off."); the save buttons are described
     by that note for screen readers, and the phone bar shows "DNS binding
     incomplete";
   - the mode: DDNS writes the entry hostname itself; CNAME asks for a
     managed name inside the zone and shows the CNAME to create, with a
     copy button;
   - the record types (A, AAAA), the TTL and 暂停.

   Saving writes the route, then the binding (`POST /dns/bindings`, or
   the whole binding on `PUT /dns/bindings/{id}` for a change). If the
   route saved but the binding did not, a message says why and the route
   page shows it as not bound. Once bound, the provider, zone, name and
   mode are shown read-only.
3. **Watch it.** The route page's **入口高可用** card reads
   `GET /routes/{id}/dns` every 30 seconds: the state, the published and
   desired values of each record type, every entry node with its reason
   and whether it is in rotation, the last error and the next attempt. In
   CNAME mode it repeats the CNAME target.
4. **Move or remove it.** **解除绑定…** on the card (super administrator)
   deletes the binding; **同时删除 Control 发布的记录** (on by default)
   deletes the published records first. If the provider refuses
   (`dns_purge_failed`) the binding stays: retry, or unbind without
   deleting the records. Then bind again.

## Binding A Route

```bash
curl -X POST "$CONTROL/api/v4/forward/dns/bindings" -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' -d '{
    "route_id": "01J...", "provider_id": "1", "zone": "example.com",
    "mode": "DNS_BINDING_MODE_DDNS",
    "record_types": ["DNS_RECORD_TYPE_A", "DNS_RECORD_TYPE_AAAA"], "ttl": 60
  }'
```

Or from the command line on Control's host:

```bash
anix-control forward dns bindings create -f binding.json
anix-control forward dns status 01J...
```

- **TTL.** Pick the shortest your provider allows. Clients keep resolving a
  removed entry for up to the TTL, and the entry keeps serving them while
  it can.
- **Moving a binding.** The provider, zone, name and mode cannot change.
  Delete the binding with `purge=true`, which deletes the records Control
  published, then create another.

## Reading The Status

`GET /api/v4/forward/routes/{id}/dns` (or `anix-control forward dns status`)
shows:

- the published and desired values of each record type;
- each entry node with its reason;
- the state.

| `state` | Meaning |
|---|---|
| `pending` | nothing published yet: no entry has been healthy since the binding was made |
| `ok` | the records are the healthy entries' addresses |
| `degraded` | no entry is healthy: Control keeps the last published records rather than an empty set |
| `error` | the provider refused or failed; `last_error` says why, and Control retries at `next_attempt_at_unix_ms` |
| `rate_limited` | the provider's call budget is spent; the change waits seconds |
| `paused` | the binding, or the route, is paused or enforced: nothing changes |
| `hostname_mismatch` | DDNS mode, and the route's `entry_hostname` is no longer the binding's name |
| `route_missing` | the route was deleted; delete the binding |

A node's `reason` is `healthy`, or why it is not:

| `reason` | Meaning |
|---|---|
| `converging` | it has not applied its latest state yet; it neither joins nor leaves |
| `never_reported`, `report_stale` | no forward report, or none for 150 s |
| `offline` | its Agent is not connected |
| `hop_error` | it could not apply the route's entry hop |
| `upstreams_down` | every upstream of its entry hop is down |
| `no_address`, `not_in_inventory` | it has no public address, or it is disabled or not a forwarding node |

A node leaves rotation after 3 unhealthy evaluations (about 30 s) and
rejoins after 3 healthy ones. Every change is in the audit log as
`system/forward-dns` (`forward.dns_publish`, `forward.dns_degraded`,
`forward.dns_recovered`), and in the metrics
`anixops_forward_dns_bindings{state}` and
`anixops_forward_dns_updates_total{provider,result}`. Alert on
`anixops_forward_dns_bindings{state="degraded"} > 0` and
`{state="error"} > 0`.
