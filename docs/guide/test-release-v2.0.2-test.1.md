# v2.0.2-test.1

Date: 2026-04-11
Channel: test

This test release packages the current `go_dev` state for operator validation before a stable release.

Included in this build:

- explicit runtime separation between NodeX mode and local nftables/Ansible mode in the admin system workflow
- system audit log query API and System page audit tab
- sensitive system and backup configuration masking plus audit logging
- real backup execution for `database`, `files`, and `full` local backup types
- login rate-limit hardening and baseline security header / request tracing middleware
- broader frontend i18n, layout, and accessibility cleanup already merged in this branch

Recommended test focus:

- NodeX mode end-to-end with a real control-plane target
- nftables/Ansible mode create, update, and delete flow on a Debian host
- System page backup create and restore for `database`, `files`, and `full`
- upgrade validation against an existing SQLite-backed environment

Known limit for this test release:

- backup storage remains production-ready only for `local`; `s3` is configured in UI/API but still returns an explicit not-implemented error
