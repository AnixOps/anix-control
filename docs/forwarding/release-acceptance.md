# Forwarding Release Acceptance

This gate records the real-environment evidence required before the signed
`nftables-forward` plugin can be approved for production rollout. Automated CI
proves package and runtime behavior; it does not replace staging, canary, or
operator evidence.

## Prepare The Record

Copy the shipped template into the ignored operator-owned config path:

```bash
cp config/examples/forwarding-acceptance.yaml.example \
  config/forwarding-acceptance.yaml
```

Keep credentials, private keys, tokens, and raw customer traffic out of this
file. Use change-ticket IDs or immutable log references for evidence fields.

The record must identify the exact Control commit, Agent commit, semantic
plugin version, and signed package SHA-256. It then records these gates in
order:

1. staging restore and IPv4/IPv6 TCP/UDP verification
2. partial-node rollback plus Control and Agent restart recovery
3. recovery through the legacy forwarding fallback
4. a healthy canary lasting at least 72 hours with rollback exercised
5. cumulative 1, 5, 25, and 100 percent rollout sets
6. operations-owner approval after the final rollout stage

`rollout.managed_node_count` is the eligible node count after excluding nodes
that are disabled or in maintenance. Each stage must list exactly the ceiling
of that percentage of managed nodes, retain all nodes from the previous stage,
and start after the preceding gate completes.

## Run The Gate

The command loads and validates the normal application config first, then
strictly decodes exactly one evidence document. Unknown fields, placeholders,
duplicate node IDs, incorrect stage sizes, overlapping stages, a short canary,
or premature approval fail the command.

```bash
anix-control -config config/config.yaml \
  -check-forwarding-evidence config/forwarding-acceptance.yaml
```

Do not enable production topology execution or describe 3.2 as production
approved until this command passes against the retained release record. The
command validates evidence and exits; it does not change runtime configuration
or deploy the plugin.
