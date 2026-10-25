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
  summary. The run fails when any host fails.

The repository is public, and so are its workflow logs. In GitHub Actions
`jms.py` masks every host address and name (`::add-mask::`), and `run` prints each
host's output only when `show_output` is ticked. Leave it off for commands
whose output may hold anything private, and run those locally instead,
where `jms.py run` always prints the output.

Locally, with the same three variables exported:

```bash
pip install requests
python3 scripts/jumpserver/jms.py check
python3 scripts/jumpserver/jms.py run --cmd 'uptime' --hosts node-1,node-2
```

`JMS_ORG` selects another organization; it defaults to the default one,
`00000000-0000-0000-0000-000000000002`.
