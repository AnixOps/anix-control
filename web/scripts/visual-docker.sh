#!/usr/bin/env sh
# Visual regression in the official Playwright image (the same image as the
# Frontend Visual CI job), so fonts and rendering match the baselines.
#
#   npm run test:visual                      compare with e2e/visual/__screenshots__
#   npm run test:visual:update               rewrite the baselines
#   sh scripts/visual-docker.sh -g login     extra Playwright arguments
#
# node_modules for the container live in a docker volume, apart from the
# host's. The image tag follows the locked @playwright/test version.
set -eu
cd "$(dirname "$0")/.."
version=$(node -p "require('./package-lock.json').packages['node_modules/@playwright/test'].version")
image="mcr.microsoft.com/playwright:v${version}-noble"
volume="anix-web-visual-node-modules-${version}"
docker run --rm --init --ipc=host \
  -v "$PWD:/work" -v "$volume:/work/node_modules" -w /work \
  -e CI=1 -e HOME=/tmp \
  -e OWNER="$(id -u):$(id -g)" \
  "$image" sh -c 'npm ci --no-audit --no-fund --loglevel=error && npx playwright test --config playwright.visual.config.js "$@"; status=$?; chown -R "$OWNER" e2e/visual test-results 2>/dev/null || true; exit $status' visual "$@"
