# Mihomo WireGuard compatibility

WireGuard subscriptions rendered for Clash-compatible Mihomo clients omit
`persistent-keepalive`. Mihomo manages its userspace WireGuard handshake state
internally, and forcing the option can cause intermittent
`invalid state for keypair derivation: handshakeZeroed` errors on IPv6-only
endpoints.

The native WireGuard profile and V2Ray-compatible `wireguard://` output retain
their existing formats. IPv6 endpoint literals are normalized before rendering
so clients receive a single pair of brackets where the selected format requires
them.

For a CN-entry/remote-exit relay, the V2bX policy rule must run before Linux's
main routing-table rule at priority `32766`. An omitted `routing_priority` is
therefore generated deterministically in the range `10000-29999`; an explicit
value must be between `1` and `32765`. The routing table ID and rule priority are
separate values and must not be reused interchangeably.

Run `scripts/test_mihomo_wireguard_isolated.sh` to exercise the Mihomo path in a
container with Docker network mode `none`. The test creates both WireGuard ends
inside the container and does not change host routes, DNS, interfaces, or
firewall rules.
