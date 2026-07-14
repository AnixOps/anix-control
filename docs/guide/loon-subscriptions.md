# Loon subscriptions

Requests whose User-Agent contains `Loon` receive Loon's native node resource
syntax. The same format can be requested explicitly with `?type=loon`; normal
requests should rely on User-Agent detection.

The Loon formatter is independent from the outputs for Clash, V2Ray,
Shadowrocket, Surge, Stash, Egern, Quantumult X, and Sing-box. It supports the
Loon-documented VMess, VLESS, Trojan, Shadowsocks, WireGuard, Hysteria2, and
AnyTLS node forms. Protocols that Loon does not document are omitted only from
the Loon response.

WireGuard-only Loon subscriptions stay in Loon node syntax rather than falling
back to a native WireGuard `.conf`. IPv6 peer endpoints are rendered with one
pair of brackets around the address.
