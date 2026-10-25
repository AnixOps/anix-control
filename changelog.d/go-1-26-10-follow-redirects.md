### Security

- **Go 1.26.10.** The root, `sdk`, `identity` and Control Center modules, the
  CI and nightly toolchain and the Control and module images
  (`golang:1.26.10-alpine`, pinned by digest) move to Go 1.26.10, which fixes
  GO-2026-6809 in `crypto/x509` (compromised intermediate certificates are
  distrusted). Control reaches it through the Agent certificate check
  (`agentpki.VerifyPeer`).
- **follow-redirects 1.16.1** in the frontend (through `axios`), which fixes
  GHSA-q3jh-wrc6-jgq8 and GHSA-8r9p-f939-6h3c (credential headers kept across
  a cross-origin redirect).
