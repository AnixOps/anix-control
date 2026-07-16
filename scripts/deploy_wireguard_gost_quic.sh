#!/usr/bin/env bash
set -euo pipefail

BASE_DIR="/home/dev/anixops/.private/wireguard-rollout"
CRED_FILE="$BASE_DIR/credentials.env"
KNOWN_HOSTS="$BASE_DIR/known_hosts"
GOST_VERSION="3.2.6"

for command in ssh sshpass scp openssl stat; do
  command -v "$command" >/dev/null 2>&1 || {
    echo "required command not found: $command" >&2
    exit 1
  }
done

test -r "$CRED_FILE"
test "$(stat -c '%a' "$CRED_FILE")" = "600"
set -a
# shellcheck source=/dev/null
. "$CRED_FILE"
set +a

for var in ENTRY_HOST ENTRY_PORT ENTRY_USER ENTRY_PASSWORD EXIT_HOST EXIT_PORT EXIT_USER EXIT_PASSWORD; do
  test -n "${!var:-}" || { echo "missing $var" >&2; exit 1; }
done

mkdir -p "$BASE_DIR"
touch "$KNOWN_HOSTS"
chmod 600 "$KNOWN_HOSTS"

ssh_remote() {
  local side="$1"
  shift
  local host port user password
  if [[ "$side" == ENTRY ]]; then
    host="$ENTRY_HOST"; port="$ENTRY_PORT"; user="$ENTRY_USER"; password="$ENTRY_PASSWORD"
  else
    host="$EXIT_HOST"; port="$EXIT_PORT"; user="$EXIT_USER"; password="$EXIT_PASSWORD"
  fi
  sshpass -f <(printf '%s\n' "$password") ssh \
    -o StrictHostKeyChecking=accept-new \
    -o UserKnownHostsFile="$KNOWN_HOSTS" \
    -o ConnectTimeout=12 -o ConnectionAttempts=1 \
    -p "$port" "$user@$host" "$@"
}

scp_to() {
  local side="$1" src="$2" dst="$3"
  local host port user password
  if [[ "$side" == ENTRY ]]; then
    host="$ENTRY_HOST"; port="$ENTRY_PORT"; user="$ENTRY_USER"; password="$ENTRY_PASSWORD"
  else
    host="$EXIT_HOST"; port="$EXIT_PORT"; user="$EXIT_USER"; password="$EXIT_PASSWORD"
  fi
  sshpass -f <(printf '%s\n' "$password") scp \
    -o StrictHostKeyChecking=accept-new \
    -o UserKnownHostsFile="$KNOWN_HOSTS" \
    -o ConnectTimeout=12 -P "$port" "$src" "$user@$host:$dst"
}

run_script() {
  local side="$1"
  shift
  ssh_remote "$side" bash -s -- "$@"
}

install_base() {
  local side="$1"
  run_script "$side" <<'REMOTE'
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y --no-install-recommends ca-certificates curl iproute2 iptables openssl procps tar wireguard-tools
for command in ip iptables ss sysctl wg; do
  command -v "$command" >/dev/null 2>&1 || {
    echo "required remote command not found: $command" >&2
    exit 1
  }
done
REMOTE
}

install_gost() {
  local side="$1"
  run_script "$side" "$GOST_VERSION" <<'REMOTE'
set -euo pipefail
version="$1"
arch=$(dpkg --print-architecture)
case "$arch" in
  amd64) asset="gost_${version}_linux_amd64.tar.gz" ;;
  arm64) asset="gost_${version}_linux_arm64.tar.gz" ;;
  *) echo "unsupported architecture: $arch" >&2; exit 1 ;;
esac
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
curl -fsSL "https://github.com/go-gost/gost/releases/download/v${version}/${asset}" -o "$tmp/$asset"
curl -fsSL "https://github.com/go-gost/gost/releases/download/v${version}/checksums.txt" -o "$tmp/checksums.txt"
grep "  $asset$" "$tmp/checksums.txt" > "$tmp/checksum"
(cd "$tmp" && sha256sum -c checksum)
tar -xzf "$tmp/$asset" -C "$tmp"
install -m 0755 "$tmp/gost" /usr/local/bin/gost
/usr/local/bin/gost -V
REMOTE
}

get_or_create_key() {
  local side="$1" path="$2"
  run_script "$side" "$path" <<'REMOTE'
set -euo pipefail
path="$1"
install -d -m 0700 /etc/wireguard
if [ ! -s "$path" ]; then
  umask 077
  wg genkey > "$path"
  chmod 600 "$path"
fi
wg pubkey < "$path"
REMOTE
}

get_or_create_client_key() {
  run_script ENTRY <<'REMOTE'
set -euo pipefail
path=/root/wg-client-1.key
if [ ! -s "$path" ]; then
  umask 077
  wg genkey > "$path"
  chmod 600 "$path"
fi
wg pubkey < "$path"
REMOTE
}

echo "[1/7] Installing WireGuard dependencies"
install_base EXIT
install_base ENTRY

echo "[2/7] Installing GOST v${GOST_VERSION}"
install_gost EXIT
install_gost ENTRY

echo "[3/7] Generating WireGuard keys"
exit_pub=$(get_or_create_key EXIT /etc/wireguard/wg-exit.key | tail -n 1)
entry_pub=$(get_or_create_key ENTRY /etc/wireguard/wg-exit.key | tail -n 1)
client_pub=$(get_or_create_client_key | tail -n 1)
relay_secret=$(openssl rand -hex 24)

exit_dev=$(ssh_remote EXIT "ip route show default | awk 'NR==1 {print \$5}'")
test -n "$exit_dev"

echo "[4/7] Configuring exit gateway"
run_script EXIT "$relay_secret" "$entry_pub" "$exit_dev" "$exit_pub" <<'REMOTE'
set -euo pipefail
relay_secret="$1"
entry_pub="$2"
exit_dev="$3"
exit_pub="$4"
backup=/var/backups/wireguard-gost-$(date +%Y%m%d%H%M%S)
install -d -m 0700 "$backup" /etc/gost /etc/wireguard
systemctl is-active --quiet V2bX || { echo 'V2bX is not active; refusing to continue' >&2; exit 1; }
for f in /etc/wireguard/wg-exit.conf /etc/gost/exit.yaml /etc/systemd/system/gost-wg-relay.service; do
  [ -e "$f" ] && cp -a "$f" "$backup/"
done
private=$(cat /etc/wireguard/wg-exit.key)
cat > /etc/wireguard/wg-exit.conf <<EOF
[Interface]
Address = 10.78.0.1/30
ListenPort = 51820
PrivateKey = $private
MTU = 1280
PostUp = iptables -A INPUT -i $exit_dev -p udp --dport 51820 -j DROP; iptables -A FORWARD -i wg-exit -o $exit_dev -j ACCEPT; iptables -A FORWARD -i $exit_dev -o wg-exit -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT; iptables -t nat -A POSTROUTING -s 10.78.0.0/30 -o $exit_dev -j MASQUERADE
PostDown = iptables -D INPUT -i $exit_dev -p udp --dport 51820 -j DROP; iptables -D FORWARD -i wg-exit -o $exit_dev -j ACCEPT; iptables -D FORWARD -i $exit_dev -o wg-exit -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT; iptables -t nat -D POSTROUTING -s 10.78.0.0/30 -o $exit_dev -j MASQUERADE

[Peer]
PublicKey = $entry_pub
AllowedIPs = 10.78.0.2/32
PersistentKeepalive = 25
EOF
chmod 600 /etc/wireguard/wg-exit.conf
cat > /etc/gost/exit.yaml <<EOF
services:
  - name: wg-relay-quic
    addr: :443
    handler:
      type: relay
      auth:
        username: wgrelay
        password: $relay_secret
    listener:
      type: quic
      metadata:
        keepalive: true
        ttl: 10s
EOF
chmod 600 /etc/gost/exit.yaml
cat > /etc/systemd/system/gost-wg-relay.service <<'EOF'
[Unit]
Description=GOST QUIC relay for WireGuard
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/gost -C /etc/gost/exit.yaml
Restart=on-failure
RestartSec=3
LimitNOFILE=65535
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
EOF
chmod 644 /etc/systemd/system/gost-wg-relay.service
sysctl -w net.ipv4.ip_forward=1 >/dev/null
install -d -m 0755 /etc/sysctl.d
printf 'net.ipv4.ip_forward=1\n' > /etc/sysctl.d/99-wireguard-gateway.conf
systemctl daemon-reload
systemctl enable --now wg-quick@wg-exit
systemctl enable gost-wg-relay
systemctl restart gost-wg-relay
systemctl is-active --quiet wg-quick@wg-exit
systemctl is-active --quiet gost-wg-relay
ss -lunp | grep -E ':(443|51820)\b'
REMOTE

echo "[5/7] Configuring entry gateway"
run_script ENTRY "$relay_secret" "$exit_pub" "$client_pub" "$EXIT_HOST" <<'REMOTE'
set -euo pipefail
relay_secret="$1"
exit_pub="$2"
client_pub="$3"
exit_host="$4"
entry_dev=$(ip route show default | awk 'NR==1 {print $5}')
test -n "$entry_dev"
backup=/var/backups/wireguard-gost-$(date +%Y%m%d%H%M%S)
install -d -m 0700 "$backup" /etc/gost /etc/wireguard
for f in /etc/wireguard/wg-in.conf /etc/wireguard/wg-exit.conf /etc/gost/entry.yaml /etc/systemd/system/gost-wg-relay.service /root/wg-client-1.conf; do
  [ -e "$f" ] && cp -a "$f" "$backup/"
done
entry_private=$(cat /etc/wireguard/wg-exit.key)
in_private=$(if [ -s /etc/wireguard/wg-in.key ]; then cat /etc/wireguard/wg-in.key; else umask 077; wg genkey | tee /etc/wireguard/wg-in.key; fi)
chmod 600 /etc/wireguard/wg-exit.key /etc/wireguard/wg-in.key /root/wg-client-1.key
cat > /etc/wireguard/wg-exit.conf <<EOF
[Interface]
Address = 10.78.0.2/30
PrivateKey = $entry_private
MTU = 1280
Table = off
PostUp = ip rule add from 10.77.0.0/24 table 51820; ip route add default dev wg-exit table 51820; iptables -A FORWARD -i wg-in -o wg-exit -j ACCEPT; iptables -A FORWARD -i wg-exit -o wg-in -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT; iptables -t nat -A POSTROUTING -s 10.77.0.0/24 -o wg-exit -j MASQUERADE
PostDown = ip rule del from 10.77.0.0/24 table 51820; ip route del default dev wg-exit table 51820; iptables -D FORWARD -i wg-in -o wg-exit -j ACCEPT; iptables -D FORWARD -i wg-exit -o wg-in -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT; iptables -t nat -D POSTROUTING -s 10.77.0.0/24 -o wg-exit -j MASQUERADE

[Peer]
PublicKey = $exit_pub
AllowedIPs = 0.0.0.0/0
Endpoint = 127.0.0.1:51821
PersistentKeepalive = 25
EOF
chmod 600 /etc/wireguard/wg-exit.conf
cat > /etc/wireguard/wg-in.conf <<EOF
[Interface]
Address = 10.77.0.1/24
ListenPort = 51820
PrivateKey = $in_private
MTU = 1280
PostUp = iptables -A INPUT -i $entry_dev -p udp --dport 51820 -j ACCEPT
PostDown = iptables -D INPUT -i $entry_dev -p udp --dport 51820 -j ACCEPT

[Peer]
PublicKey = $client_pub
AllowedIPs = 10.77.0.2/32
EOF
chmod 600 /etc/wireguard/wg-in.conf
cat > /etc/gost/entry.yaml <<EOF
services:
  - name: wg-udp-over-quic
    addr: 127.0.0.1:51821
    handler:
      type: udp
      chain: relay-quic
    listener:
      type: udp
    forwarder:
      nodes:
        - name: exit-wg
          addr: 127.0.0.1:51820
chains:
  - name: relay-quic
    hops:
      - name: exit-hop
        nodes:
          - name: exit-relay
            addr: $exit_host:443
            connector:
              type: relay
              auth:
                username: wgrelay
                password: $relay_secret
            dialer:
              type: quic
              metadata:
                keepalive: true
                ttl: 10s
EOF
chmod 600 /etc/gost/entry.yaml
cat > /etc/systemd/system/gost-wg-relay.service <<'EOF'
[Unit]
Description=GOST QUIC relay for WireGuard
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/gost -C /etc/gost/entry.yaml
Restart=on-failure
RestartSec=3
LimitNOFILE=65535
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
EOF
chmod 644 /etc/systemd/system/gost-wg-relay.service
sysctl -w net.ipv4.ip_forward=1 >/dev/null
install -d -m 0755 /etc/sysctl.d
printf 'net.ipv4.ip_forward=1\n' > /etc/sysctl.d/99-wireguard-gateway.conf
systemctl daemon-reload
systemctl enable gost-wg-relay
systemctl restart gost-wg-relay
systemctl enable --now wg-quick@wg-exit
systemctl enable --now wg-quick@wg-in
systemctl is-active --quiet gost-wg-relay
systemctl is-active --quiet wg-quick@wg-exit
systemctl is-active --quiet wg-quick@wg-in
wg show
REMOTE

echo "[6/7] Writing client profile on entry"
run_script ENTRY "$ENTRY_HOST" <<'REMOTE'
set -euo pipefail
entry_host="$1"
in_pub=$(wg show wg-in public-key)
client_private=$(cat /root/wg-client-1.key)
cat > /root/wg-client-1.conf <<EOF
[Interface]
PrivateKey = $client_private
Address = 10.77.0.2/32
DNS = 1.1.1.1
MTU = 1280

[Peer]
PublicKey = $in_pub
AllowedIPs = 0.0.0.0/0
Endpoint = $entry_host:51820
PersistentKeepalive = 25
EOF
chmod 600 /root/wg-client-1.conf /root/wg-client-1.key
REMOTE

echo "[7/7] Verifying services and path"
run_script ENTRY <<'REMOTE'
set -euo pipefail
systemctl is-active wg-quick@wg-in wg-quick@wg-exit gost-wg-relay
wg show wg-in
wg show wg-exit
sysctl -n net.ipv4.ip_forward
REMOTE
run_script EXIT <<'REMOTE'
set -euo pipefail
systemctl is-active wg-quick@wg-exit gost-wg-relay V2bX
wg show wg-exit
sysctl -n net.ipv4.ip_forward
REMOTE

echo "Deployment complete. Client profile: /root/wg-client-1.conf on ENTRY"
