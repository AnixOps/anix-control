### Fixed

- A node with anx-server reports its network status again. The anx-server
  hybrid key (X25519 and ML-KEM-768) is about 2.5 KB of text, past the
  1 KB text cap, so the Agent refused to build the status and Control never
  got the key: every ANX route into that node stayed pending. Public keys and
  `anx_line.server_public` now take up to 8 KB.
