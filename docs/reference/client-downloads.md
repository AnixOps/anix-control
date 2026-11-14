# ANX Client Downloads (Reference)

The AnixOps Net client has two editions (owner decision, 2026-11-14):

| Edition | Protocols | Where users get it |
|---|---|---|
| AnixOps Net (open source) | public protocols only | GitHub Releases of `AnixOps/anixops-net` and its download page `https://anixops.github.io/anixops-net/` |
| AnixOps Net, ANX edition | public protocols and ANX | **only from Control**, behind the subscription token (this page) |

The ANX edition is closed source and is never published anywhere public.
The private anx release workflows build it and upload each release to
Control; Control checks it and serves it to its subscribers. iOS has only
the ANX edition, through TestFlight.

Code: `internal/clientrelease` (storage and checks),
`internal/handler/client_releases.go` (routes),
`web/src/views/user/AnxClientDownloads.vue` (subscription page).

## Release files

A release is a version (semver without `v`, for example `0.3.0-beta.6`) and
any of these files, named `anixops-net-anx-<version><suffix>`:

| Platform key | Suffix | Shown as |
|---|---|---|
| `windows` | `-windows-x64.msi` | Windows, installer |
| `windows-portable` | `-windows-x64-portable.zip` | Windows, portable |
| `macos` | `-macos.pkg` | macOS, installer package |
| `macos-dmg` | `-macos.dmg` | macOS, disk image |
| `linux-deb` | `-linux-amd64.deb` | Linux, .deb |
| `linux` | `-linux-x64.tar.gz` | Linux, .tar.gz |
| `android` | `-android-arm64.apk` | Android, APK |
| `ios` | `-ios-testflight.url` | iPhone / iPad: a text file with one `https://` TestFlight invitation |

plus `SHA256SUMS` (`sha256sum` output listing every file) and
`SHA256SUMS.sig`: `ssh-keygen -Y sign -n anx-release` by the anx release key,
the same key and namespace as anx-node releases
(`agentinstall.KernelReleaseKey`). Control trusts no other key.

Each file may be at most 512 MiB. The newest version (semver order,
prereleases included) that holds at least one file is the one subscribers
are offered.

## Storage

`agent_install.artifact_dir` must be set. Releases live in

```text
<artifact_dir>/anixops-net-anx/<version>/<file>
<artifact_dir>/anixops-net-anx/<version>/SHA256SUMS
<artifact_dir>/anixops-net-anx/<version>/SHA256SUMS.sig
```

A release can be uploaded in several requests (one file each) with the same
`SHA256SUMS` and signature. Another `SHA256SUMS` for a stored version is
refused (`409 client_release_conflict`); delete the version first.

## Administrator API

Super administrators, or an admin API token of one (`admin` scope):

| Route | Does |
|---|---|
| `POST /api/v4/kernel/client-releases` | multipart, field `files`: `SHA256SUMS`, `SHA256SUMS.sig` and at least one release file. `201` with the stored release; `400 client_release_invalid` when the signature or a digest does not verify or a name is not a release file; `409 agent_install_unconfigured` without an artifact directory |
| `GET /api/v4/kernel/client-releases` | `configured`, `latest`, `releases` (newest first, each with `files`: name, platform, os, size, sha256, and `testflight_url`), `platforms` |
| `DELETE /api/v4/kernel/client-releases/{version}` | removes a release, `204` |

Upload example (what the anx release workflow does):

```bash
curl -fsS -H "Authorization: Bearer $ANIX_CONTROL_ADMIN_TOKEN" \
  -F files=@SHA256SUMS -F files=@SHA256SUMS.sig \
  -F files=@anixops-net-anx-0.3.0-beta.6-windows-x64-portable.zip \
  https://control.example.com/api/v4/kernel/client-releases
```

The reverse proxy in front of Control must accept a body that large on this
route (nginx: `client_max_body_size 600m;` in its `location`).

## Subscriber routes

On the subscription address (`app.subscribe_path`, default `s`), with the
subscription token, rate limited like public downloads:

| Route | Answers |
|---|---|
| `GET /s/{token}/client` | `data`: `version`, `files` (name, platform, os, size, sha256, `url`), `testflight_url`, `sha256sums_url`, `signature_url` |
| `GET /s/{token}/client/{version}/{file}` | the file, as an attachment |

`404` for an unknown token, an unknown file or when Control holds no
release; `403 subscription_inactive` for a banned or expired subscriber.
The download addresses carry the subscription token: like the subscription
link, they must not be shared.

The subscription page (用户中心 → 订阅) reads `GET /s/{token}/client` from
its own origin and shows the "AnixOps Net (ANX edition)" section only when a
release is there.
