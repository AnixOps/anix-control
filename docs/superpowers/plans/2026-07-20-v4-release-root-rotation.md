# V4 Release Root Rotation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** Rotate V4 to one newly generated Ed25519 official package root, configure matching GitHub repository secrets, and prove formal package signing works without issuing release-approval records.

**Architecture:** Generate the PEM outside Git at /root/.anixops-release/v4-root-20260720/. Its derived Base64 raw public key becomes the only plugins.official_public_key value in Control configuration and the protected GitHub public-root secret.

**Tech Stack:** OpenSSL, Python package builder, Go configuration tests, GitHub CLI.

## Global Constraints

- The PEM stays outside Git with mode 0600; its directory is mode 0700.
- V4 has one active root. Old-root packages require re-signing and re-import.
- Do not create canary or support approval secrets during this rotation.
- Preserve .superpowers/sdd/task-6-report.md as an unstaged user change.

---

### Task 1: Generate The Rotated Root

**Files:**
- Create outside Git: /root/.anixops-release/v4-root-20260720/official-ed25519.pem
- Create outside Git: /root/.anixops-release/v4-root-20260720/official-public-key.raw

**Produces:** a PEM private key and a one-line Base64 raw 32-byte public key.

- [ ] **Step 1: Verify tooling and repository-secret access**

Run:

~~~bash
openssl version
gh auth status --hostname github.com
gh repo view AnixOps/anix-control --json nameWithOwner
~~~

Expected: OpenSSL and authenticated access to AnixOps/anix-control.

- [ ] **Step 2: Generate the new key pair**

Run:

~~~bash
set -Eeuo pipefail
install -d -m 0700 /root/.anixops-release/v4-root-20260720
umask 077
openssl genpkey -algorithm ED25519 -out /root/.anixops-release/v4-root-20260720/official-ed25519.pem
chmod 0600 /root/.anixops-release/v4-root-20260720/official-ed25519.pem
openssl pkey -in /root/.anixops-release/v4-root-20260720/official-ed25519.pem -pubout -outform DER | tail -c 32 | base64 | tr -d '\n' > /root/.anixops-release/v4-root-20260720/official-public-key.raw
printf '\n' >> /root/.anixops-release/v4-root-20260720/official-public-key.raw
~~~

- [ ] **Step 3: Verify permissions and key pairing without printing the PEM**

Run:

~~~bash
test "$(stat -c '%a' /root/.anixops-release/v4-root-20260720)" = 700
test "$(stat -c '%a' /root/.anixops-release/v4-root-20260720/official-ed25519.pem)" = 600
test "$(base64 -d < /root/.anixops-release/v4-root-20260720/official-public-key.raw | wc -c | tr -d ' ')" = 32
test "$(openssl pkey -in /root/.anixops-release/v4-root-20260720/official-ed25519.pem -pubout -outform DER | tail -c 32 | base64 | tr -d '\n')" = "$(tr -d '[:space:]' < /root/.anixops-release/v4-root-20260720/official-public-key.raw)"
~~~

Expected: all assertions exit 0.

### Task 2: Replace The Repository Trust Root

**Files:**
- Modify: config/config.prod.yaml line 108
- Modify: config/config.yaml.example line 91
- Modify: config/config.dev.yaml.example line 68
- Modify: internal/config/config_test.go

**Consumes:** the public root from Task 1.

**Produces:** all three plugins.official_public_key values equal the new Base64 root.

- [ ] **Step 1: Change the existing profile test expectation before changing configuration**

In `TestOfficialPluginAlphaProfiles` in `internal/config/config_test.go`, replace
the existing `officialPublicKey` constant with the exact Base64 value in
`/root/.anixops-release/v4-root-20260720/official-public-key.raw`. That test
already Base64-decodes the expected root, asserts 32 bytes, and loads all three
configuration profiles.

- [ ] **Step 2: Run the focused test and observe the old-root failure**

Run: GOWORK=off go test ./internal/config -run TestOfficialPluginAlphaProfiles -count=1

Expected: failure because each loaded profile still contains the old root.

- [ ] **Step 3: Replace only the quoted root values**

Use apply_patch to replace official_public_key in each configuration file with the output of:

~~~bash
tr -d '[:space:]' < /root/.anixops-release/v4-root-20260720/official-public-key.raw
~~~

- [ ] **Step 4: Verify all configured roots and run the package configuration suite**

Run:

~~~bash
expected="$(tr -d '[:space:]' < /root/.anixops-release/v4-root-20260720/official-public-key.raw)"
for config in config/config.prod.yaml config/config.yaml.example config/config.dev.yaml.example; do
  actual="$(sed -n 's/^[[:space:]]*official_public_key:[[:space:]]*"\([^"]*\)".*/\1/p' "$config")"
  test "$actual" = "$expected"
done
GOWORK=off go test ./internal/config -count=1
~~~

Expected: all assertions and configuration tests pass.

### Task 3: Prove Formal Package Signing With The New Root

**Files:**
- Create outside Git: /tmp/anixops-v4-root-rotation/packages/
- Test: packages/shared/tests/test_manifest_schema.py
- Test: config/scripts/check_release_workflow.sh

**Consumes:** Task 1 key files and Task 2 configuration.

**Produces:** a local-only verified cohort of sixteen formal V4 packages.

- [ ] **Step 1: Build formal packages**

Run:

~~~bash
install -d -m 0700 /tmp/anixops-v4-root-rotation/packages
GOWORK=off python3 packages/shared/build_package.py --all --version 4.0.0 --out /tmp/anixops-v4-root-rotation/packages --formal-release --signing-key /root/.anixops-release/v4-root-20260720/official-ed25519.pem --official-public-key /root/.anixops-release/v4-root-20260720/official-public-key.raw
~~~

Expected: built 16 signed package artifacts and 16 SBOM files.

- [ ] **Step 2: Verify formal artifacts and release guards**

Run:

~~~bash
GOWORK=off python3 packages/shared/build_package.py --all --version 4.0.0 --out /tmp/anixops-v4-root-rotation/packages --verify-release --official-public-key /root/.anixops-release/v4-root-20260720/official-public-key.raw
python3 packages/shared/tests/test_manifest_schema.py -v
bash config/scripts/check_release_workflow.sh --self-test
bash config/scripts/check_release_workflow.sh
~~~

Expected: all commands pass.

### Task 4: Commit The Public Rotation And Set Key Secrets

**Files:**
- Modify: config/config.prod.yaml
- Modify: config/config.yaml.example
- Modify: config/config.dev.yaml.example
- Modify: internal/config/config_test.go

**Consumes:** the verified Task 1 key pair.

**Produces:** a pushed root configuration and two GitHub repository secrets.

- [ ] **Step 1: Commit and push only public-root configuration and its test**

Run:

~~~bash
git add config/config.prod.yaml config/config.yaml.example config/config.dev.yaml.example internal/config/config_test.go
git diff --cached --check
git commit -m "chore(release): rotate v4 official package root"
git push origin go_dev
~~~

Expected: origin/go_dev contains the new public root and the task report remains unstaged.

- [ ] **Step 2: Upload the private and public key as repository secrets**

Run:

~~~bash
gh secret set ANIXOPS_PLUGIN_SIGNING_PRIVATE_KEY --repo AnixOps/anix-control < /root/.anixops-release/v4-root-20260720/official-ed25519.pem
gh secret set ANIXOPS_PLUGIN_OFFICIAL_PUBLIC_KEY --repo AnixOps/anix-control < /root/.anixops-release/v4-root-20260720/official-public-key.raw
gh secret list --repo AnixOps/anix-control | grep -E '^(ANIXOPS_PLUGIN_SIGNING_PRIVATE_KEY|ANIXOPS_PLUGIN_OFFICIAL_PUBLIC_KEY)[[:space:]]'
~~~

Expected: both names appear; no secret values are read or printed.

- [ ] **Step 3: Keep approval secrets unset**

Run:

~~~bash
gh secret list --repo AnixOps/anix-control | grep -E '^(ANIXOPS_V4_CANARY_APPROVAL|ANIXOPS_V4_SUPPORT_APPROVAL)[[:space:]]' || true
~~~

Expected: root rotation does not create operational canary or support approvals.
