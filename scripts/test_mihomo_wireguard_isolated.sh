#!/usr/bin/env bash
set -euo pipefail

# Black-box Mihomo -> WireGuard test in a container with no network device
# shared with the host. The runtime container uses Docker's `none` network,
# publishes no ports, and only receives NET_ADMIN plus /dev/net/tun.

MIHOMO_IMAGE="${MIHOMO_IMAGE:-metacubex/mihomo:latest}"
IMAGE="${MIHOMO_WG_TEST_IMAGE:-v2board-mihomo-wireguard-test:local}"
CONTAINER="v2board-mihomo-wg-test-$$"
WORKDIR=$(mktemp -d)

cleanup() {
  docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
  rm -rf "$WORKDIR"
}
trap cleanup EXIT INT TERM

command -v docker >/dev/null 2>&1 || {
  echo "docker is required" >&2
  exit 1
}
test -c /dev/net/tun || {
  echo "/dev/net/tun is required" >&2
  exit 1
}

cat >"$WORKDIR/Dockerfile" <<'EOF'
ARG MIHOMO_IMAGE
FROM ${MIHOMO_IMAGE} AS mihomo
FROM alpine:3.22
RUN apk add --no-cache bash curl iproute2 python3 wireguard-tools
COPY --from=mihomo /mihomo /usr/local/bin/mihomo
EOF

cat >"$WORKDIR/run-test.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

TEST_DIR=/tmp/mihomo-wireguard
mkdir -p "$TEST_DIR/www"
chmod 700 "$TEST_DIR"
cd "$TEST_DIR"

ip link set lo up
umask 077
wg genkey >server.key
wg pubkey <server.key >server.pub
wg genkey >client.key
wg pubkey <client.key >client.pub
wg genpsk >peer.psk

ip link add wg0 type wireguard
ip address add 10.66.0.1/24 dev wg0
wg set wg0 \
  private-key server.key \
  listen-port 51820 \
  peer "$(cat client.pub)" \
  preshared-key peer.psk \
  allowed-ips 10.66.0.2/32
ip link set wg0 up

cat >config.yaml <<YAML
mixed-port: 7890
allow-lan: false
bind-address: 127.0.0.1
mode: rule
log-level: debug
proxies:
  - name: WireGuard-CN-Entry
    type: wireguard
    server: 127.0.0.1
    port: 51820
    ip: 10.66.0.2
    private-key: $(cat client.key)
    public-key: $(cat server.pub)
    pre-shared-key: $(cat peer.psk)
    allowed-ips:
      - 0.0.0.0/0
    udp: true
    mtu: 1280
proxy-groups:
  - name: PROXY
    type: select
    proxies:
      - WireGuard-CN-Entry
rules:
  - MATCH,PROXY
YAML

printf 'mihomo-wireguard-isolated-ok\n' >www/health.txt
python3 -m http.server 8080 --bind 10.66.0.1 --directory www \
  >http.log 2>&1 &
HTTP_PID=$!
/usr/local/bin/mihomo -d "$TEST_DIR" -f config.yaml >mihomo.log 2>&1 &
MIHOMO_PID=$!

finish_processes() {
  kill "$MIHOMO_PID" "$HTTP_PID" >/dev/null 2>&1 || true
  wait "$MIHOMO_PID" "$HTTP_PID" >/dev/null 2>&1 || true
}
trap finish_processes EXIT INT TERM

for _ in $(seq 1 50); do
  if curl --silent --fail --max-time 1 \
    --proxy http://127.0.0.1:7890 \
    http://10.66.0.1:8080/health.txt >response.txt; then
    break
  fi
  sleep 0.1
done

test "$(cat response.txt 2>/dev/null || true)" = "mihomo-wireguard-isolated-ok" || {
  echo "Mihomo WireGuard request failed" >&2
  cat mihomo.log >&2
  exit 1
}

# Exercise repeated and concurrent client traffic without sharing any host
# route, socket, DNS resolver, or external network connection.
curl_pids=()
for _ in $(seq 1 20); do
  curl --silent --fail --max-time 2 \
    --proxy http://127.0.0.1:7890 \
    http://10.66.0.1:8080/health.txt >/dev/null &
  curl_pids+=("$!")
done
for pid in "${curl_pids[@]}"; do
  wait "$pid"
done

HANDSHAKE=$(wg show wg0 latest-handshakes | awk 'NR == 1 {print $2}')
test "${HANDSHAKE:-0}" -gt 0 || {
  echo "WireGuard handshake was not established" >&2
  cat mihomo.log >&2
  exit 1
}

if grep -q 'handshakeZeroed' mihomo.log; then
  echo "Mihomo emitted handshakeZeroed" >&2
  cat mihomo.log >&2
  exit 1
fi

echo "response=$(cat response.txt)"
echo "latest_handshake=$HANDSHAKE"
wg show wg0 transfer | awk 'NR == 1 {print "server_rx_bytes=" $2; print "server_tx_bytes=" $3}'
echo "mihomo_version=$(/usr/local/bin/mihomo -v | head -n 1)"
echo "network_mode=none"
EOF
chmod 700 "$WORKDIR/run-test.sh"

docker build --quiet --build-arg "MIHOMO_IMAGE=$MIHOMO_IMAGE" -t "$IMAGE" "$WORKDIR" >/dev/null
docker run --name "$CONTAINER" --rm \
  --network none \
  --cap-drop ALL \
  --cap-add NET_ADMIN \
  --device /dev/net/tun:/dev/net/tun \
  --security-opt no-new-privileges \
  --mount "type=bind,src=$WORKDIR/run-test.sh,dst=/run-test.sh,readonly" \
  "$IMAGE" /run-test.sh
