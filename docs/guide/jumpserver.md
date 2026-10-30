# JumpServer Node Access

The node fleet is reached through one JumpServer (`https://jump.anixops.com`):
people log in to nodes through it, with session recording, and batch commands
and file distribution run in its job center, which drives Ansible. Control
keeps managing the business side (Agents, protocols, forwarding,
subscriptions); JumpServer only manages access to the machines.

`scripts/jumpserver/jms.py` is a small client for the JumpServer API, and the
manual GitHub Actions workflow **JumpServer** (`.github/workflows/jumpserver.yml`)
runs it with the repository's credentials.

## Credentials

The workflow reads them from the repository's `production` environment
(Settings → Environments → production):

| Name | Kind | Value |
| --- | --- | --- |
| `JMS_URL` | variable | `https://jump.anixops.com`, without a trailing slash |
| `JMS_KEY_ID` | secret | the Access Key ID |
| `JMS_SECRET` | secret | the Access Key secret |
| `CLOUDFLARE_API_TOKEN` | secret | optional: a Cloudflare token with only Zone → DNS → Edit, for certbot's DNS-01 renewal on the hosts |

Create the Access Key in JumpServer under the profile menu → 个人信息 →
API Key, preferably for a dedicated user (for example `api-bot`) with only
the roles it needs rather than an administrator. Running commands also needs
command execution enabled (系统设置 → 安全设置) and an account on the hosts
that the key's user may use. Limit the `production` environment's deployment
branches to protected branches, so only merged workflows can read the key.

The workflow runs on a GitHub-hosted runner, so JumpServer must be reachable
from the internet. If it is not, register a self-hosted runner on a machine
that can reach it and change the job's `runs-on`.

## Running It

Actions → JumpServer → Run workflow, then choose the action:

- `check`: shows whom the key belongs to and how many hosts JumpServer has.
  Run it first to confirm the key and the address.
- `hosts`: lists every host with its address, platform and whether it is
  active.
- `run`: runs `command` (default `uptime`) as `runas` (default `root`) on the
  hosts named in `hosts` (comma-separated names or addresses; empty means
  every active host) through the job center, waits for it and prints the
  summary. The run fails when any host fails, except a host without Python,
  where Ansible cannot run the command at all: the run warns and goes on
  (scripts gate on a file of the host they are for). With `show_output` it then
  prints the job's Ansible log, which JumpServer serves only over its task
  log websocket. Ansible would treat `{{ }}` in the command as a template
  (for example `docker ps --format '{{.Names}}'`), so `jms.py` wraps such a
  command in `{% raw %}` and it runs as written.

- `control`: calls Control's administrator API (`api_method`, `api_path`,
  `api_body`) at the `CONTROL_URL` variable (default
  `https://new-x.kalijerry.uk`) with the `CONTROL_ADMIN_TOKEN` secret, an
  Admin-scope token from **Security → API tokens**
  ([Admin API tokens](admin-api-tokens.md)). The answer is printed with every
  secret field (`api_key`, `secret`, `credential`, `token`, `password`,
  `private_key`, `server_key`, `psk`) replaced by `***`. In `api_body`,
  `@@NEW_REALITY_KEY@@` becomes a new X25519 private key (its public key is
  printed, and `@@NEW_REALITY_PUBLIC_KEY@@` in the same body becomes it) and
  `@@NEW_SS2022_KEY_16@@` / `@@NEW_SS2022_KEY_32@@` a new Shadowsocks 2022 key
  of that many bytes, so keys never pass through the inputs.
- `install-agent`: asks Control for a one-hour install token for `node`
  (`proxy-<id>`) and runs Control's install command on the single host named in
  `hosts`, under `systemd-run` so it outlives the job's 60-second limit. The
  token is masked and never printed. The installer logs to
  `/root/anix-agent-install.log`; read it with a later `run`
  (`tail -n 30 /root/anix-agent-install.log`).

Never type a secret into `command`: workflow inputs are shown in the run's
page. Write `@@CLOUDFLARE_API_TOKEN@@` instead and `jms.py` fills in the
secret of that name just before it sends the job, masking it in the log.
Only names listed in `SECRET_PLACEHOLDERS` in `jms.py` are filled in. The
filled-in command is stored with the job in JumpServer and is briefly visible
in the host's process list, so pass the secret to a script that writes it to
a root-only file, for example:

```bash
echo <base64 script> | base64 -d > /root/setup.sh && CLOUDFLARE_API_TOKEN='@@CLOUDFLARE_API_TOKEN@@' bash /root/setup.sh
```

The repository is public, and so are its workflow logs. In GitHub Actions
`jms.py` masks every host address and name (`::add-mask::`), and `run` prints each
host's output only when `show_output` is ticked. Leave it off for commands
whose output may hold anything private, and run those locally instead,
where `jms.py run` always prints the output.

Locally, with the same three variables exported:

```bash
pip install requests websocket-client
python3 scripts/jumpserver/jms.py check
python3 scripts/jumpserver/jms.py run --cmd 'uptime' --hosts node-1,node-2
```

`JMS_ORG` selects another organization; it defaults to the default one,
`00000000-0000-0000-0000-000000000002`.
