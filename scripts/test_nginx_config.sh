#!/usr/bin/env bash

set -Eeuo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
test_dir="${NGINX_TEST_DIR:-/tmp/anixops-nginx-config-test}"
image="${NGINX_TEST_IMAGE:-nginx:alpine@sha256:62ff2089abf5a9ed33bd232895bef5e22f7bb4b200675cec49a5ebc48e3d4ac8}"

command -v docker >/dev/null 2>&1 || {
  echo "docker is required for the Nginx configuration test" >&2
  exit 1
}
command -v openssl >/dev/null 2>&1 || {
  echo "openssl is required for the Nginx configuration test" >&2
  exit 1
}

mkdir -p "${test_dir}"
openssl req -x509 -nodes -newkey rsa:2048 -days 1 -subj '/CN=localhost' \
  -keyout "${test_dir}/key.pem" -out "${test_dir}/cert.pem" >/dev/null 2>&1

docker run --rm --add-host api:127.0.0.1 \
  -v "${repo_root}/config/docker/nginx/nginx.conf:/etc/nginx/nginx.conf:ro" \
  -v "${test_dir}:/etc/nginx/ssl:ro" \
  "${image}" nginx -t
