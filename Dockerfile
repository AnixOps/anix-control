# syntax=docker/dockerfile:1
#
# AnixOps Control image.
#
# Targets:
#   source  (default, last stage) builds the frontend and binary from this
#           checkout: local development, the PR smoke test and edge images.
#   release copies the exact GitHub Release artifacts from the named build
#           context "artifacts" (see the docker job in .github/workflows/ci.yml):
#             artifacts/anix-control-linux-<arch>
#             artifacts/web/public/
#             artifacts/bootstrap/identity-platform/  (signed package, 3 files)
#
# The image runs as uid 10001 without a config file: configure it with
# ANIX_CONTROL_* environment variables (anix-control -print-env) or mount a
# config file and set ANIX_CONTROL_CONFIG. It works with a read-only root
# filesystem when /tmp is a writable, executable tmpfs or volume.

ARG GO_IMAGE=golang:1.26.8-alpine@sha256:8ac98ca534ac3f51e1f420a1dd2c15e74c75cfa0f23f3ad27eb5d7236c349a0c
ARG NODE_IMAGE=node:22-alpine@sha256:0a7108bf6c7bf5de370ffb1a3ed6be93d405b43ff159f681a8d18c0e2bc2e402
ARG RUNTIME_IMAGE=debian:bookworm-slim@sha256:3783cc01769c7b2b1b83a5c5ad96c815348e28ed7da68e2e3687004faa906251

# Frontend build, on the build platform (the output is architecture-neutral).
FROM --platform=$BUILDPLATFORM ${NODE_IMAGE} AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# Go build, cross-compiled on the build platform for the target platform.
FROM --platform=$BUILDPLATFORM ${GO_IMAGE} AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGETOS=linux
ARG TARGETARCH
ARG VERSION=dev
ARG BUILD_TIME
ARG BUILD_CODE
ARG COMMIT=unknown
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath \
    -ldflags="-s -w -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME} -X main.buildCode=${BUILD_CODE} -X main.commit=${COMMIT}" \
    -o /out/anix-control ./cmd/server

# Runtime base shared by both targets. /var/lib/anixops is the mount point for
# the package artifact volume; a new named volume inherits its owner (uid 10001)
# and mode from the image.
FROM ${RUNTIME_IMAGE} AS runtime-base
# ansible, openssh-client and sshpass serve the local-ansible forward runtime
# (forward_runtime.backend: nftables_ansible); tini reaps the ssh and ansible
# processes that runtime spawns.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ansible ca-certificates curl openssh-client sshpass tini tzdata \
    && rm -rf /var/lib/apt/lists/*
RUN groupadd --gid 10001 anixops \
    && useradd --uid 10001 --gid 10001 --home-dir /home/anixops --create-home --shell /usr/sbin/nologin anixops \
    && install -d -o anixops -g anixops -m 0700 /home/anixops/.ssh \
    && install -d -o anixops -g anixops -m 0700 /var/lib/anixops
WORKDIR /app
# Code, docs and playbooks are owned by root and read-only for the app user.
COPY docs/swagger.json docs/swagger.yaml ./docs/
COPY config/deploy/ansible/ansible.cfg config/deploy/ansible/README.md ./config/deploy/ansible/
COPY config/deploy/ansible/playbooks/ ./config/deploy/ansible/playbooks/
COPY config/config.yaml.example ./config/config.yaml.example
ENV HOME=/home/anixops \
    TZ=Asia/Shanghai \
    TMPDIR=/tmp \
    GIN_MODE=release \
    ANSIBLE_CONFIG=/app/config/deploy/ansible/ansible.cfg \
    ANSIBLE_HOME=/tmp/.ansible \
    ANSIBLE_LOCAL_TEMP=/tmp/.ansible/tmp \
    ANSIBLE_SSH_CONTROL_PATH_DIR=/tmp/.ansible/cp
USER 10001:10001
EXPOSE 8080 3000 50051
HEALTHCHECK --interval=15s --timeout=5s --start-period=60s --retries=3 \
    CMD curl -fsS http://127.0.0.1:8080/readyz >/dev/null || exit 1
ENTRYPOINT ["/usr/bin/tini", "--", "/app/anix-control"]

# Release image: the binary, frontend and identity bootstrap package attached
# to the GitHub Release, byte for byte.
FROM runtime-base AS release
ARG TARGETARCH
COPY --from=artifacts --chmod=0755 anix-control-linux-${TARGETARCH} /app/anix-control
COPY --from=artifacts web/public/ /app/web/public/
COPY --from=artifacts bootstrap/identity-platform/ /app/bootstrap/identity-platform/
# A fresh database imports the signed identity package on first start.
ENV ANIX_CONTROL_PLUGINS_IDENTITY_BOOTSTRAP_PACKAGE_DIR=/app/bootstrap/identity-platform

# Source image (default target).
FROM runtime-base AS source
COPY --from=builder /out/anix-control /app/anix-control
COPY --from=web /src/web/public/ /app/web/public/
