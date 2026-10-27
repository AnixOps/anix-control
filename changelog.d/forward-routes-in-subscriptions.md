### Added

- Forward routes in subscriptions (加入订阅, node-onboarding Phase 10). A
  route whose target is a proxy node's enabled protocol (the node's host or
  the protocol's own host and the protocol's port, or a loopback target of
  a last hop on the node) can be published from the 订阅 card of its page:
  the protocol's subscription groups then offer it again at each entry
  node's address and allocated port (or at the route's entry HA hostname),
  with the protocol's settings and keys, named "<protocol> | <route>" or a
  display name. A paused route, or one that does not forward the protocol's
  TCP or UDP, stays published and is left out until fixed. API:
  `GET`/`PUT /api/v4/kernel/forward-routes/{id}/subscription` (audited,
  module `forward_route_subscription`); migration
  `0012_forward_route_subscriptions` adds `v5_forward_route_subscription`.
  This replaces the child relay node (`parent_id`), which ran no Agent and
  so never reached a subscription.
