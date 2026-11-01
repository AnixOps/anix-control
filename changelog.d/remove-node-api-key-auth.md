### Removed

- The node API key and request-signature middleware of the removed node
  HTTP APIs, `POST /api/v2/internal/auth-keys` and the node registration
  code behind the removed `/api/v2/node/register`. `NODE_DEFAULT_AUTH_KEY`
  no longer seeds a registration key at startup.
- `app.api_token` (`ANIX_CONTROL_APP_API_TOKEN`) is ignored; remove it.
  Automation uses admin API tokens (`anixadm_...`).
