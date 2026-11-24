#!/usr/bin/env bash
# Fails when a tracked file describes ANX internals. This repository is
# public: ANX may be named and driven only through the fields, flags and
# names of the anix-agent contracts/anx/BOUNDARY.md, never explained (spec
# references, private documents, mechanism or rationale). Sensitive words
# are checked by .github/scripts/neutral_terms.py.
#
#   config/scripts/check_public_wording.sh             check every tracked file
#   config/scripts/check_public_wording.sh --self-test check the matcher itself
set -euo pipefail

# Extended regular expressions, matched case-insensitively.
PATTERNS=(
  'ANX-LINE-(SPEC|REVERSE|ONBOARDING)' # private spec documents
  'TBD-L[0-9]'                         # private spec decisions
  'anx (spec |docs )?§'                # private spec sections
  'FUTURE-IMPROVEMENTS'                # private roadmap
  'anx #[0-9]'                         # private pull requests
  'anx-lab|dest-probe'                 # private tools
  'cover[- ]site|looks unlike|random-looking|wireguard-shaped'
  'camouflage|伪装'
  'borrowed[- ]handshake|借用握手'
)

# Merged migrations are immutable (their checksums are recorded), and this
# script holds the patterns.
EXCLUDE=':!internal/database/migrations/sql/0037_entitlement_devices.*.sql :!config/scripts/check_public_wording.sh :!web/package-lock.json'

matcher() {
  local IFS='|'
  printf '%s' "${PATTERNS[*]}"
}

if [[ "${1:-}" == "--self-test" ]]; then
  bad=$'see anx ANX-LINE-REVERSE §6.4\nthe cover site\ncamouflage target\nTBD-L14\npaste what anx-lab prints'
  good=$'anx-server --reverse-a2-listen\nkind anx, the A2 jacket\nfallback target (dest)'
  [[ "$(grep -icE "$(matcher)" <<<"${bad}")" == 5 ]] || { echo "self-test: missed a bad line" >&2; exit 1; }
  ! grep -iqE "$(matcher)" <<<"${good}" || { echo "self-test: flagged a good line" >&2; exit 1; }
  echo "self-test ok"
  exit 0
fi

# shellcheck disable=SC2086
if git grep -nIiE "$(matcher)" -- . ${EXCLUDE}; then
  echo "public wording check failed: describe ANX only through BOUNDARY.md names" >&2
  exit 1
fi
echo "public wording ok"
