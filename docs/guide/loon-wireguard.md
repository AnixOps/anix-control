# Loon WireGuard compatibility

Loon subscriptions use Loon's native node syntax and are selected only when
the subscription request User-Agent identifies Loon. Other subscription
formats are not changed by Loon-specific compatibility handling.

For a WireGuard endpoint hostname that has IPv6 addresses but no IPv4 address,
the panel resolves the hostname while rendering the Loon subscription and
emits a bracketed IPv6 endpoint such as `[2001:db8::10]:51820`. This avoids a
Loon bootstrap failure when its local DNS configuration sends only an A query
before opening the UDP transport. Dual-stack and IPv4 endpoint hostnames remain
hostnames, and any panel-side resolver failure falls back to the configured
hostname instead of failing the subscription request.

The endpoint must remain reachable over IPv6/UDP from the client network. The
WireGuard entry should listen on the configured UDP port on every published
IPv6 address.
