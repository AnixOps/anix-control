#!/usr/bin/env bash
# Manage one panel forward in the nftables "inet v2b_forward" table.
#
#   v2b_forward_nft.sh apply  <forward-id> <ruleset-file>
#   v2b_forward_nft.sh remove <forward-id> [delete-counters]
#   v2b_forward_nft.sh stats  <forward-id>
#
# The ruleset file is the nft text the panel renders for the forward
# (internal/service/forward_nftables_plan.go). apply and remove build one nft
# transaction: delete the forward's rules (found by comment) and chains, delete
# its rules from the legacy "ip v2b_forward" table (or the whole legacy table
# once no other forward uses it), then add the new ruleset. nft -f applies it
# atomically, so a failed apply leaves the old rules in place.
#
# The named counters fwd_<id>_<proto>_up / _down are kept across apply and
# pause; only "remove <id> delete-counters" deletes them.
set -euo pipefail

NFT=${NFT:-nft}
FAMILY=inet
TABLE=v2b_forward
LEGACY_FAMILY=ip
LEGACY_TABLE=v2b_forward

die() {
  echo "v2b_forward_nft: $*" >&2
  exit 2
}

mode=${1:-}
id=${2:-}
[[ "$id" =~ ^[0-9]+$ ]] || die "forward id must be a number, got '${id}'"

table_exists() {
  "$NFT" list table "$1" "$2" >/dev/null 2>&1
}

# Print "delete rule" commands for the rules in <chain> whose comment starts
# with <prefix>.
rule_deletes() {
  local family=$1 table=$2 chain=$3 prefix=$4 listing handle
  listing=$("$NFT" -a list chain "$family" "$table" "$chain" 2>/dev/null) || return 0
  while read -r handle; do
    [[ "$handle" =~ ^[0-9]+$ ]] && echo "delete rule $family $table $chain handle $handle"
  done < <(printf '%s\n' "$listing" | awk -v p="comment \"${prefix}" 'index($0, p) { print $NF }')
  return 0
}

# Print the names of the chains (or counters) of a table matching a regex.
table_objects() {
  local family=$1 table=$2 kind=$3 regex=$4 listing
  listing=$("$NFT" list table "$family" "$table" 2>/dev/null) || return 0
  printf '%s\n' "$listing" | awk -v kind="$kind" -v re="$regex" '$1 == kind && $3 == "{" && $2 ~ re { print $2 }'
}

# Remove the forward from the inet table. $1 = 1 deletes its counters too.
emit_new_cleanup() {
  local delete_counters=$1 chain c chains counters
  table_exists "$FAMILY" "$TABLE" || return 0
  for chain in prerouting postrouting forward; do
    rule_deletes "$FAMILY" "$TABLE" "$chain" "v2b-forward-${id}-"
  done
  chains=$(table_objects "$FAMILY" "$TABLE" chain "^v2b_(fwd|acct)_${id}_")
  for c in $chains; do echo "flush chain $FAMILY $TABLE $c"; done
  for c in $chains; do echo "delete chain $FAMILY $TABLE $c"; done
  if [ "$delete_counters" = 1 ]; then
    counters=$(table_objects "$FAMILY" "$TABLE" counter "^fwd_${id}_")
    for c in $counters; do echo "delete counter $FAMILY $TABLE $c"; done
  fi
  return 0
}

# One-time migration: drop the forward from the legacy IPv4-only table, and
# the table itself once no other forward is left in it.
emit_legacy_cleanup() {
  local chain c chains others
  table_exists "$LEGACY_FAMILY" "$LEGACY_TABLE" || return 0
  others=$(table_objects "$LEGACY_FAMILY" "$LEGACY_TABLE" chain '^v2b_fwd_' | grep -Ev "^v2b_fwd_${id}_" || true)
  if [ -z "$others" ]; then
    echo "delete table $LEGACY_FAMILY $LEGACY_TABLE"
    return 0
  fi
  for chain in prerouting postrouting; do
    rule_deletes "$LEGACY_FAMILY" "$LEGACY_TABLE" "$chain" "v2b-forward-${id}-"
  done
  chains=$(table_objects "$LEGACY_FAMILY" "$LEGACY_TABLE" chain "^v2b_fwd_${id}_")
  for c in $chains; do echo "flush chain $LEGACY_FAMILY $LEGACY_TABLE $c"; done
  for c in $chains; do echo "delete chain $LEGACY_FAMILY $LEGACY_TABLE $c"; done
  return 0
}

# True when the inet table holds nothing but this forward (and, unless its
# counters are deleted, no counters at all).
only_forward_left() {
  local delete_counters=$1 chains counters
  chains=$(table_objects "$FAMILY" "$TABLE" chain '^v2b_(fwd|acct)_' | grep -Ev "^v2b_(fwd|acct)_${id}_" || true)
  if [ "$delete_counters" = 1 ]; then
    counters=$(table_objects "$FAMILY" "$TABLE" counter '.' | grep -Ev "^fwd_${id}_" || true)
  else
    counters=$(table_objects "$FAMILY" "$TABLE" counter '.')
  fi
  [ -z "$chains" ] && [ -z "$counters" ]
}

run_tx() {
  local tx=$1
  if [ -s "$tx" ]; then
    "$NFT" -f "$tx"
  fi
}

counter_bytes() {
  local listing
  listing=$("$NFT" list counters table "$FAMILY" "$TABLE" 2>/dev/null) || return 0
  printf '%s\n' "$listing" | awk -v re="^fwd_${id}_(tcp|udp)_(up|down)$" '
    $1 == "counter" && $2 ~ re { name = $2; next }
    name != "" && $1 == "packets" { print name, $4; name = "" }
  '
}

legacy_chain_bytes() {
  local listing
  listing=$("$NFT" list chain "$LEGACY_FAMILY" "$LEGACY_TABLE" "v2b_fwd_${id}_$1" 2>/dev/null) || return 1
  printf '%s\n' "$listing" | awk '{ for (i = 1; i < NF; i++) if ($i == "bytes") s += $(i + 1) } END { print s + 0 }'
}

tx=$(mktemp)
trap 'rm -f "$tx"' EXIT

case "$mode" in
  apply)
    ruleset=${3:-}
    [ -r "$ruleset" ] || die "ruleset file '${ruleset}' is not readable"
    {
      emit_new_cleanup 0
      emit_legacy_cleanup
      cat "$ruleset"
    } >"$tx"
    run_tx "$tx"
    echo "V2B_NFT applied forward ${id}"
    ;;
  remove)
    delete_counters=0
    [ "${3:-}" = "delete-counters" ] && delete_counters=1
    {
      if table_exists "$FAMILY" "$TABLE" && only_forward_left "$delete_counters"; then
        echo "delete table $FAMILY $TABLE"
      else
        emit_new_cleanup "$delete_counters"
      fi
      emit_legacy_cleanup
    } >"$tx"
    run_tx "$tx"
    echo "V2B_NFT removed forward ${id}"
    ;;
  stats)
    declare -A bytes=()
    while read -r name value; do
      [ -n "${name:-}" ] && bytes[$name]=$value
    done < <(counter_bytes)
    found=0
    for proto in tcp udp; do
      up=${bytes[fwd_${id}_${proto}_up]:-}
      down=${bytes[fwd_${id}_${proto}_down]:-}
      if [ -n "$up" ] || [ -n "$down" ]; then
        found=1
        echo "STATS_JSON {\"protocol\":\"${proto}\",\"upload\":${up:-0},\"download\":${down:-0}}"
      fi
    done
    if [ "$found" = 0 ]; then
      # Not migrated yet: report the legacy nat-chain counter (upload only).
      for proto in tcp udp; do
        if legacy=$(legacy_chain_bytes "$proto"); then
          echo "STATS_JSON {\"protocol\":\"${proto}\",\"bytes\":${legacy},\"legacy\":true}"
        fi
      done
    fi
    ;;
  *)
    die "unknown mode '${mode}' (want apply, remove or stats)"
    ;;
esac
