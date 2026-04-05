#!/usr/bin/env bash
set -euo pipefail

APP_NAME="v2board-anixops"
REPO_SLUG="${REPO_SLUG:-anixops/v2board_AnixOps}"
REPO_REF="${REPO_REF:-go_dev}"
GIT_URL="https://github.com/${REPO_SLUG}.git"
ARCHIVE_URL="https://codeload.github.com/${REPO_SLUG}/tar.gz/refs/heads/${REPO_REF}"
INSTALL_STATE_FILE=".panel-install.env"

USE_WHIPTAIL=0
COUNTRY_CODE=""
SUDO=""
DOCKER_CMD=()

log_info() {
  printf '[INFO] %s\n' "$*"
}

log_warn() {
  printf '[WARN] %s\n' "$*" >&2
}

log_error() {
  printf '[ERROR] %s\n' "$*" >&2
}

die() {
  log_error "$*"
  exit 1
}

detect_sudo() {
  if [ "$(id -u)" -eq 0 ]; then
    SUDO=""
  elif command -v sudo >/dev/null 2>&1; then
    SUDO="sudo"
  else
    SUDO=""
  fi
}

init_ui() {
  if command -v whiptail >/dev/null 2>&1 && [ -t 0 ] && [ -t 1 ]; then
    USE_WHIPTAIL=1
  fi
}

ui_header() {
  if [ -t 1 ]; then
    clear || true
  fi
  printf '========================================\n'
  printf '  V2Board AnixOps Installer / Manager\n'
  printf '========================================\n'
  printf 'Repo: %s\n' "$REPO_SLUG"
  printf 'Ref:  %s\n\n' "$REPO_REF"
}

ui_menu() {
  local title=$1
  local prompt=$2
  shift 2

  if [ "$USE_WHIPTAIL" -eq 1 ]; then
    whiptail --title "$title" --menu "$prompt" 20 78 10 "$@" 3>&1 1>&2 2>&3
    return
  fi

  printf '%s\n%s\n' "$title" "$prompt"
  while [ "$#" -gt 0 ]; do
    printf '  %s) %s\n' "$1" "$2"
    shift 2
  done
  printf '> '
  read -r value
  printf '%s\n' "$value"
}

ui_input() {
  local title=$1
  local prompt=$2
  local default_value=${3:-}

  if [ "$USE_WHIPTAIL" -eq 1 ]; then
    whiptail --title "$title" --inputbox "$prompt" 12 78 "$default_value" 3>&1 1>&2 2>&3
    return
  fi

  if [ -n "$default_value" ]; then
    printf '%s [%s]: ' "$prompt" "$default_value"
  else
    printf '%s: ' "$prompt"
  fi
  read -r value
  printf '%s\n' "${value:-$default_value}"
}

ui_password() {
  local title=$1
  local prompt=$2
  local default_value=${3:-}

  if [ "$USE_WHIPTAIL" -eq 1 ]; then
    whiptail --title "$title" --passwordbox "$prompt" 12 78 "$default_value" 3>&1 1>&2 2>&3
    return
  fi

  printf '%s: ' "$prompt"
  read -r -s value
  printf '\n'
  printf '%s\n' "${value:-$default_value}"
}

ui_confirm() {
  local title=$1
  local prompt=$2

  if [ "$USE_WHIPTAIL" -eq 1 ]; then
    whiptail --title "$title" --yesno "$prompt" 12 78
    return
  fi

  printf '%s [y/N]: ' "$prompt"
  read -r value
  case "${value:-n}" in
    y|Y|yes|YES) return 0 ;;
    *) return 1 ;;
  esac
}

random_secret() {
  LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c 24
}

detect_country() {
  if [ -n "$COUNTRY_CODE" ]; then
    printf '%s\n' "$COUNTRY_CODE"
    return
  fi

  COUNTRY_CODE="$(curl -fsSL --connect-timeout 5 https://ipinfo.io/country 2>/dev/null || true)"
  COUNTRY_CODE="${COUNTRY_CODE//$'\n'/}"
  printf '%s\n' "$COUNTRY_CODE"
}

github_url() {
  local url=$1
  if [ -n "${GH_PROXY:-}" ]; then
    printf '%s%s\n' "$GH_PROXY" "$url"
    return
  fi

  if [ "$(detect_country)" = "CN" ]; then
    printf 'https://ghfast.top/%s\n' "$url"
    return
  fi

  printf '%s\n' "$url"
}

default_install_dir() {
  if [ "$(id -u)" -eq 0 ] || [ -w /opt ] || [ -n "$SUDO" ]; then
    printf '/opt/%s\n' "$APP_NAME"
  else
    printf '%s/%s\n' "$HOME" "$APP_NAME"
  fi
}

require_commands() {
  local missing=()
  local cmd
  for cmd in curl tar; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
      missing+=("$cmd")
    fi
  done
  if [ "${#missing[@]}" -gt 0 ]; then
    die "Missing required commands: ${missing[*]}"
  fi
}

detect_docker() {
  if command -v docker-compose >/dev/null 2>&1; then
    DOCKER_CMD=(docker-compose)
    return
  fi

  if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
    DOCKER_CMD=(docker compose)
    return
  fi

  die "Docker Compose was not found. Install docker compose or docker-compose first."
}

download_archive_overlay() {
  local install_dir=$1
  local archive
  local tmp_dir
  local src_dir

  tmp_dir="$(mktemp -d)"
  archive="$tmp_dir/repo.tgz"

  log_info "Downloading source archive"
  curl -fsSL "$(github_url "$ARCHIVE_URL")" -o "$archive"
  tar -xzf "$archive" -C "$tmp_dir"
  src_dir="$(find "$tmp_dir" -mindepth 1 -maxdepth 1 -type d | head -n 1)"
  [ -n "$src_dir" ] || die "Failed to unpack source archive"

  mkdir -p "$install_dir"
  cp -a "$src_dir"/. "$install_dir"/
  rm -rf "$tmp_dir"
}

sync_source() {
  local install_dir=$1

  mkdir -p "$install_dir"
  if command -v git >/dev/null 2>&1; then
    if [ -d "$install_dir/.git" ]; then
      log_info "Updating repository checkout"
      git -C "$install_dir" fetch --depth 1 origin "$REPO_REF"
      git -C "$install_dir" checkout "$REPO_REF"
      if ! git -C "$install_dir" pull --ff-only origin "$REPO_REF"; then
        die "git pull failed. Resolve local changes in $install_dir and re-run the installer."
      fi
      return
    fi

    if [ -z "$(find "$install_dir" -mindepth 1 -maxdepth 1 -print -quit 2>/dev/null)" ]; then
      log_info "Cloning repository"
      git clone --depth 1 --branch "$REPO_REF" "$(github_url "$GIT_URL")" "$install_dir"
      return
    fi
  fi

  download_archive_overlay "$install_dir"
}

prepare_install_layout() {
  local install_dir=$1
  local ssh_dir="$install_dir/config/deploy/ssh"
  local ansible_dir="$install_dir/config/deploy/ansible"

  mkdir -p \
    "$install_dir/config/data" \
    "$install_dir/logs" \
    "$ssh_dir" \
    "$ansible_dir/playbooks"

  if [ ! -f "$ansible_dir/inventory.ini" ] && [ -f "$ansible_dir/inventory.ini.example" ]; then
    cp "$ansible_dir/inventory.ini.example" "$ansible_dir/inventory.ini"
  fi

  chmod 700 "$ssh_dir" || true
  find "$ssh_dir" -type f ! -name '.gitignore' -exec chmod 600 {} \; 2>/dev/null || true

  if [ -n "$SUDO" ] || [ "$(id -u)" -eq 0 ]; then
    $SUDO chown -R 1000:1000 "$ssh_dir" 2>/dev/null || true
  fi
}

seed_inventory() {
  local install_dir=$1
  local host_alias=$2
  local ssh_user=$3
  local ssh_port=$4
  local ssh_key=$5
  local inventory_path="$install_dir/config/deploy/ansible/inventory.ini"

  if [ -z "$host_alias" ] || [ -z "$ssh_user" ]; then
    return
  fi

  cat >"$inventory_path" <<EOF
[forward_nodes]
${host_alias} ansible_host=${host_alias} ansible_user=${ssh_user} ansible_port=${ssh_port} ansible_ssh_private_key_file=/home/v2board/.ssh/${ssh_key}
EOF
}

write_install_state() {
  local install_dir=$1
  local deploy_mode=$2
  local monitoring_enabled=$3
  local proxy_enabled=$4

  cat >"$install_dir/$INSTALL_STATE_FILE" <<EOF
DEPLOY_MODE=${deploy_mode}
MONITORING_ENABLED=${monitoring_enabled}
PROXY_ENABLED=${proxy_enabled}
EOF
}

load_install_state() {
  local install_dir=$1
  if [ -f "$install_dir/$INSTALL_STATE_FILE" ]; then
    # shellcheck disable=SC1090
    . "$install_dir/$INSTALL_STATE_FILE"
  else
    DEPLOY_MODE="quick"
    MONITORING_ENABLED=0
    PROXY_ENABLED=0
  fi
}

compose_args() {
  local deploy_mode=$1
  local monitoring_enabled=$2
  local proxy_enabled=$3

  COMPOSE_ARGS=()
  if [ "$deploy_mode" = "production" ]; then
    COMPOSE_ARGS=(-f docker-compose.prod.yml)
    if [ "$monitoring_enabled" = "1" ]; then
      COMPOSE_ARGS+=(--profile monitoring)
    fi
    if [ "$proxy_enabled" = "1" ]; then
      COMPOSE_ARGS+=(--profile proxy)
    fi
  else
    COMPOSE_ARGS=(-f docker-compose.yml)
  fi
}

run_compose() {
  local install_dir=$1
  shift
  (
    cd "$install_dir"
    "${DOCKER_CMD[@]}" "${COMPOSE_ARGS[@]}" "$@"
  )
}

forward_runtime_ansible_json() {
  printf '%s' '{"inventory":"/app/config/deploy/ansible/inventory.ini","playbookApply":"/app/config/deploy/ansible/playbooks/forward_apply.yml","playbookRemove":"/app/config/deploy/ansible/playbooks/forward_remove.yml","workingDir":"/app/config/deploy/ansible","targetPattern":"{{node.host}}","timeoutSeconds":120,"become":true,"environment":{"ANSIBLE_CONFIG":"/app/config/deploy/ansible/ansible.cfg","ANSIBLE_HOST_KEY_CHECKING":"False"}}'
}

write_quick_env() {
  local install_dir=$1
  local timezone=$2
  local frontend_port=$3
  local api_port=$4
  local grpc_port=$5
  local runtime_backend=${6:-gost}

  cat >"$install_dir/.env" <<EOF
TZ=${timezone}
VERSION=latest
PANEL_FRONTEND_PORT=${frontend_port}
PANEL_API_PORT=${api_port}
PANEL_GRPC_PORT=${grpc_port}
GRAFANA_PORT=3001
NGINX_HTTP_PORT=80
NGINX_HTTPS_PORT=443
DOCKER_IMAGE=v2board:latest
FORWARD_RUNTIME_BACKEND=${runtime_backend}
FORWARD_RUNTIME_ANSIBLE_CONFIG_JSON=$(forward_runtime_ansible_json)
FORWARD_RUNTIME_ANSIBLE_INVENTORY=
FORWARD_RUNTIME_ANSIBLE_BECOME=
FORWARD_RUNTIME_ANSIBLE_HOST_ALIAS=
FORWARD_RUNTIME_ANSIBLE_HOST=
FORWARD_RUNTIME_ANSIBLE_PORT=22
FORWARD_RUNTIME_ANSIBLE_USER=root
FORWARD_RUNTIME_ANSIBLE_PASSWORD=
FORWARD_RUNTIME_ANSIBLE_BECOME_PASSWORD=
EOF
}

write_prod_env() {
  local install_dir=$1
  local timezone=$2
  local frontend_port=$3
  local api_port=$4
  local grpc_port=$5
  local http_port=$6
  local https_port=$7
  local db_user=$8
  local db_password=$9
  local db_name=${10}
  local redis_password=${11}
  local jwt_secret=${12}
  local runtime_backend=${13:-gost}

  cat >"$install_dir/.env" <<EOF
TZ=${timezone}
VERSION=latest
PANEL_FRONTEND_PORT=${frontend_port}
PANEL_API_PORT=${api_port}
PANEL_GRPC_PORT=${grpc_port}
GRAFANA_PORT=3001
NGINX_HTTP_PORT=${http_port}
NGINX_HTTPS_PORT=${https_port}
DB_USER=${db_user}
DB_PASSWORD=${db_password}
DB_NAME=${db_name}
REDIS_PASSWORD=${redis_password}
JWT_SECRET=${jwt_secret}
API_TOKEN=
DOCKER_IMAGE=v2board:latest
GRAFANA_ADMIN=admin
GRAFANA_PASSWORD=$(random_secret)
DOMAIN=panel.example.com
EMAIL=admin@example.com
FORWARD_RUNTIME_BACKEND=${runtime_backend}
FORWARD_RUNTIME_ANSIBLE_CONFIG_JSON=$(forward_runtime_ansible_json)
FORWARD_RUNTIME_ANSIBLE_INVENTORY=
FORWARD_RUNTIME_ANSIBLE_BECOME=
FORWARD_RUNTIME_ANSIBLE_HOST_ALIAS=
FORWARD_RUNTIME_ANSIBLE_HOST=
FORWARD_RUNTIME_ANSIBLE_PORT=22
FORWARD_RUNTIME_ANSIBLE_USER=root
FORWARD_RUNTIME_ANSIBLE_PASSWORD=
FORWARD_RUNTIME_ANSIBLE_BECOME_PASSWORD=
EOF
}

write_quick_config() {
  local install_dir=$1
  local admin_email=$2
  local admin_password=$3
  local jwt_secret=$4

  cat >"$install_dir/config/config.yaml" <<EOF
env: "production"

server:
  host: "0.0.0.0"
  port: 8080
  mode: "release"
  read_timeout: 30
  write_timeout: 30

frontend:
  enable: true
  port: 3000
  path: "web/public"

database:
  driver: "sqlite"
  database: "config/data/v2board.db"
  log_level: "info"

cache:
  driver: "memory"

log:
  level: "info"
  output: "stdout"
  file_path: "./logs/v2board.log"
  max_size: 100
  max_backups: 30
  max_age: 7

jwt:
  secret: "${jwt_secret}"
  expire: 86400

app:
  name: "V2Board"
  version: "2.0.0"
  api_token: ""
  traffic_log_enable: true
  subscribe_path: "s"

admin:
  email: "${admin_email}"
  password: "${admin_password}"

tls:
  enable: false
  cert_file: ""
  key_file: ""
  domain: ""
EOF
}

write_prod_config() {
  local install_dir=$1
  local admin_email=$2
  local admin_password=$3
  local jwt_secret=$4
  local db_user=$5
  local db_password=$6
  local db_name=$7
  local redis_password=$8

  cat >"$install_dir/config/config.yaml" <<EOF
env: "production"

server:
  host: "0.0.0.0"
  port: 8080
  mode: "release"
  read_timeout: 30
  write_timeout: 30

frontend:
  enable: true
  port: 3000
  path: "web/public"

database:
  driver: "postgres"
  host: "db"
  port: 5432
  username: "${db_user}"
  password: "${db_password}"
  database: "${db_name}"
  log_level: "info"
  max_idle_conns: 10
  max_open_conns: 100
  conn_max_lifetime: 3600

cache:
  driver: "redis"
  redis_host: "redis"
  redis_port: 6379
  redis_password: "${redis_password}"
  redis_db: 0

log:
  level: "info"
  output: "stdout"
  file_path: "./logs/v2board.log"
  max_size: 100
  max_backups: 30
  max_age: 7

jwt:
  secret: "${jwt_secret}"
  expire: 86400

app:
  name: "V2Board"
  version: "2.0.0"
  api_token: ""
  traffic_log_enable: true
  subscribe_path: "s"

admin:
  email: "${admin_email}"
  password: "${admin_password}"

tls:
  enable: false
  cert_file: ""
  key_file: ""
  domain: ""
EOF
}

install_panel() {
  local install_dir
  local mode_choice
  local deploy_mode
  local timezone
  local admin_email
  local admin_password
  local generated_password=""
  local frontend_port
  local api_port
  local grpc_port
  local jwt_secret
  local db_user
  local db_password
  local db_name
  local redis_password
  local monitoring_enabled=0
  local proxy_enabled=0
  local runtime_backend="gost"
  local http_port=80
  local https_port=443
  local seed_host=""
  local seed_user=""
  local seed_port="22"
  local seed_key="id_ed25519"

  detect_docker

  install_dir="$(ui_input "Install" "Installation directory" "$(default_install_dir)")"
  mode_choice="$(ui_menu "Install" "Select deployment mode" "1" "Quick deploy (SQLite + memory cache)" "2" "Production deploy (PostgreSQL + Redis)")"
  case "$mode_choice" in
    2) deploy_mode="production" ;;
    *) deploy_mode="quick" ;;
  esac

  timezone="$(ui_input "Install" "Timezone" "Asia/Shanghai")"
  admin_email="$(ui_input "Install" "Admin email" "admin@panel.local")"
  admin_password="$(ui_password "Install" "Admin password (leave blank to auto-generate)")"
  if [ -z "$admin_password" ]; then
    admin_password="$(random_secret)"
    generated_password="$admin_password"
  fi

  if [ "$(ui_menu "Install" "Default forward runtime backend" "1" "gost" "2" "iptables_ansible")" = "2" ]; then
    runtime_backend="iptables_ansible"
  fi

  frontend_port="$(ui_input "Install" "Frontend port" "3000")"
  api_port="$(ui_input "Install" "API port" "8080")"
  grpc_port="$(ui_input "Install" "gRPC port" "50051")"
  jwt_secret="$(random_secret)"

  if [ "$deploy_mode" = "production" ]; then
    db_name="$(ui_input "Install" "PostgreSQL database name" "v2board")"
    db_user="$(ui_input "Install" "PostgreSQL user" "v2board")"
    db_password="$(ui_password "Install" "PostgreSQL password")"
    [ -n "$db_password" ] || db_password="$(random_secret)"
    redis_password="$(ui_password "Install" "Redis password")"
    [ -n "$redis_password" ] || redis_password="$(random_secret)"

    if ui_confirm "Install" "Enable Grafana/Prometheus profile?"; then
      monitoring_enabled=1
    fi

    if ui_confirm "Install" "Enable Nginx reverse proxy profile?"; then
      proxy_enabled=1
      http_port="$(ui_input "Install" "Nginx HTTP port" "80")"
      https_port="$(ui_input "Install" "Nginx HTTPS port" "443")"
    fi
  fi

  if ui_confirm "Install" "Seed an ansible inventory entry now?"; then
    seed_host="$(ui_input "Install" "Forward node host or IP" "")"
    seed_user="$(ui_input "Install" "SSH user" "root")"
    seed_port="$(ui_input "Install" "SSH port" "22")"
    seed_key="$(ui_input "Install" "SSH private key filename under config/deploy/ssh" "id_ed25519")"
  fi

  sync_source "$install_dir"
  prepare_install_layout "$install_dir"

  if [ "$deploy_mode" = "production" ]; then
    write_prod_env "$install_dir" "$timezone" "$frontend_port" "$api_port" "$grpc_port" "$http_port" "$https_port" "$db_user" "$db_password" "$db_name" "$redis_password" "$jwt_secret" "$runtime_backend"
    write_prod_config "$install_dir" "$admin_email" "$admin_password" "$jwt_secret" "$db_user" "$db_password" "$db_name" "$redis_password"
  else
    write_quick_env "$install_dir" "$timezone" "$frontend_port" "$api_port" "$grpc_port" "$runtime_backend"
    write_quick_config "$install_dir" "$admin_email" "$admin_password" "$jwt_secret"
  fi

  seed_inventory "$install_dir" "$seed_host" "$seed_user" "$seed_port" "$seed_key"
  write_install_state "$install_dir" "$deploy_mode" "$monitoring_enabled" "$proxy_enabled"
  compose_args "$deploy_mode" "$monitoring_enabled" "$proxy_enabled"

  log_info "Starting containers"
  run_compose "$install_dir" up -d --build

  ui_header
  printf 'Install completed.\n\n'
  printf 'Install dir: %s\n' "$install_dir"
  printf 'Frontend:    http://SERVER_IP:%s\n' "$frontend_port"
  printf 'API:         http://SERVER_IP:%s\n' "$api_port"
  printf 'gRPC:        SERVER_IP:%s\n' "$grpc_port"
  if [ "$proxy_enabled" = "1" ]; then
    printf 'Proxy HTTP:   http://SERVER_IP:%s\n' "$http_port"
    printf 'Proxy HTTPS:  https://SERVER_IP:%s\n' "$https_port"
  fi
  printf '\n'
  printf 'Admin email:    %s\n' "$admin_email"
  printf 'Admin password: %s\n' "$admin_password"
  if [ -n "$generated_password" ]; then
    printf '\nA random admin password was generated because you left it blank.\n'
  fi
  printf '\n'
  printf 'Ansible inventory: %s\n' "$install_dir/config/deploy/ansible/inventory.ini"
  printf 'Ansible apply:     %s\n' "$install_dir/config/deploy/ansible/playbooks/forward_apply.yml"
  printf 'Ansible remove:    %s\n' "$install_dir/config/deploy/ansible/playbooks/forward_remove.yml"
  printf 'SSH key directory: %s\n' "$install_dir/config/deploy/ssh"
  printf 'Default runtime:   %s\n' "$runtime_backend"
  printf '\n'
  printf 'Use config/deploy/ssh + inventory.ini for key auth, or fill FORWARD_RUNTIME_ANSIBLE_HOST/USER/PASSWORD in .env for password auth.\n'
  printf 'The runtime backend and ansible config are seeded from .env during service startup.\n'
}

resolve_existing_install_dir() {
  local suggested
  suggested="$(default_install_dir)"
  ui_input "Manage" "Installation directory" "$suggested"
}

update_panel() {
  local install_dir

  detect_docker
  install_dir="$(resolve_existing_install_dir)"
  [ -d "$install_dir" ] || die "Installation directory does not exist: $install_dir"

  sync_source "$install_dir"
  prepare_install_layout "$install_dir"
  load_install_state "$install_dir"
  compose_args "$DEPLOY_MODE" "$MONITORING_ENABLED" "$PROXY_ENABLED"

  log_info "Rebuilding and restarting containers"
  run_compose "$install_dir" up -d --build
}

restart_panel() {
  local install_dir

  detect_docker
  install_dir="$(resolve_existing_install_dir)"
  [ -d "$install_dir" ] || die "Installation directory does not exist: $install_dir"

  load_install_state "$install_dir"
  compose_args "$DEPLOY_MODE" "$MONITORING_ENABLED" "$PROXY_ENABLED"
  run_compose "$install_dir" restart
}

status_panel() {
  local install_dir

  detect_docker
  install_dir="$(resolve_existing_install_dir)"
  [ -d "$install_dir" ] || die "Installation directory does not exist: $install_dir"

  load_install_state "$install_dir"
  compose_args "$DEPLOY_MODE" "$MONITORING_ENABLED" "$PROXY_ENABLED"
  run_compose "$install_dir" ps
}

logs_panel() {
  local install_dir

  detect_docker
  install_dir="$(resolve_existing_install_dir)"
  [ -d "$install_dir" ] || die "Installation directory does not exist: $install_dir"

  load_install_state "$install_dir"
  compose_args "$DEPLOY_MODE" "$MONITORING_ENABLED" "$PROXY_ENABLED"
  run_compose "$install_dir" logs --tail=200
}

uninstall_panel() {
  local install_dir

  detect_docker
  install_dir="$(resolve_existing_install_dir)"
  [ -d "$install_dir" ] || die "Installation directory does not exist: $install_dir"

  if ! ui_confirm "Uninstall" "Stop containers, remove volumes, and delete $install_dir ?"; then
    return
  fi

  load_install_state "$install_dir"
  compose_args "$DEPLOY_MODE" "$MONITORING_ENABLED" "$PROXY_ENABLED"
  run_compose "$install_dir" down --volumes --remove-orphans
  rm -rf "$install_dir"
}

main_menu() {
  ui_menu \
    "V2Board AnixOps" \
    "Select an action" \
    "1" "Install or redeploy" \
    "2" "Update" \
    "3" "Restart" \
    "4" "Status" \
    "5" "Logs" \
    "6" "Uninstall" \
    "7" "Exit"
}

main() {
  detect_sudo
  init_ui
  require_commands

  while true; do
    ui_header
    case "$(main_menu)" in
      1) install_panel ;;
      2) update_panel ;;
      3) restart_panel ;;
      4) status_panel ;;
      5) logs_panel ;;
      6) uninstall_panel ;;
      7) exit 0 ;;
      *) log_warn "Invalid selection" ;;
    esac

    if [ "$USE_WHIPTAIL" -eq 1 ]; then
      if ! ui_confirm "Continue" "Return to the main menu?"; then
        exit 0
      fi
    else
      printf '\nPress Enter to continue...'
      read -r _
    fi
  done
}

main "$@"
