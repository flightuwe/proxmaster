#!/usr/bin/env bash
set -euo pipefail

REPO_URL="${REPO_URL:-https://github.com/flightuwe/proxmaster.git}"
INSTALL_DIR="${INSTALL_DIR:-/opt/proxmaster}"
INFRA_ENV_FILE="${INFRA_ENV_FILE:-$INSTALL_DIR/infra/.env}"

GREEN="\033[0;32m"
YELLOW="\033[1;33m"
RED="\033[0;31m"
NC="\033[0m"

log() { echo -e "${GREEN}[proxmaster]${NC} $*"; }
warn() { echo -e "${YELLOW}[warn]${NC} $*"; }
err() { echo -e "${RED}[error]${NC} $*" >&2; }

require_root() {
  if [ "${EUID:-$(id -u)}" -ne 0 ]; then
    err "Bitte als root ausfuehren (z. B. mit sudo)."
    exit 1
  fi
}

detect_apt() {
  if ! command -v apt-get >/dev/null 2>&1; then
    err "Dieses Install-Skript unterstuetzt aktuell apt-basierte Systeme (Ubuntu/Debian)."
    exit 1
  fi
}

install_dependencies() {
  log "Installiere Abhaengigkeiten (git, curl, jq, docker, compose-plugin) ..."
  apt-get update -y
  apt-get install -y ca-certificates curl gnupg lsb-release git jq

  if ! command -v docker >/dev/null 2>&1; then
    install -m 0755 -d /etc/apt/keyrings
    if [ ! -f /etc/apt/keyrings/docker.gpg ]; then
      curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
      chmod a+r /etc/apt/keyrings/docker.gpg
    fi
    echo \
      "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
      $(. /etc/os-release && echo "$VERSION_CODENAME") stable" > /etc/apt/sources.list.d/docker.list
    apt-get update -y
    apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
    systemctl enable docker
    systemctl start docker
  fi
}

sync_repo() {
  if [ -d "$INSTALL_DIR/.git" ]; then
    log "Aktualisiere vorhandenes Repo in $INSTALL_DIR ..."
    git -C "$INSTALL_DIR" fetch --all --prune
    git -C "$INSTALL_DIR" checkout main
    git -C "$INSTALL_DIR" pull --ff-only origin main
  else
    log "Klone Repo nach $INSTALL_DIR ..."
    mkdir -p "$INSTALL_DIR"
    git clone "$REPO_URL" "$INSTALL_DIR"
  fi
}

rand_token() {
  tr -dc 'A-Za-z0-9' </dev/urandom | head -c 40
}

write_env_file() {
  local admin_token="$1"
  local store_backend="$2"
  local pg_dsn="$3"
  local runner_node_id="$4"
  local runner_secret="$5"

  cat >"$INFRA_ENV_FILE" <<EOF
PROXMASTER_LISTEN_ADDR=:8080
PROXMASTER_ADMIN_TOKEN=$admin_token
PROXMASTER_STORE_BACKEND=$store_backend
PROXMASTER_POSTGRES_DSN=$pg_dsn
PROXMASTER_FAIL_CLOSED=true
PROXMASTER_RUNNER_HEARTBEAT_MAX_SEC=120
RUNNER_LISTEN_ADDR=:9091
RUNNER_NODE_ID=$runner_node_id
RUNNER_SHARED_SECRET=$runner_secret
EOF
}

start_stack() {
  log "Starte Proxmaster-Stack ..."
  docker compose -f "$INSTALL_DIR/infra/docker-compose.yml" --env-file "$INFRA_ENV_FILE" up -d
}

print_summary() {
  local admin_token="$1"
  cat <<EOF

Installation abgeschlossen.

Naechste Schritte:
1) VPN anbinden (Tailscale/WireGuard) und Port 8080 nur im Overlay freigeben
2) Health pruefen:
   curl http://127.0.0.1:8080/healthz
3) Token sicher speichern:
   PROXMASTER_ADMIN_TOKEN=$admin_token

Stack:
- API:    http://<node1-ip>:8080
- Runner: http://<node1-ip>:9091
- Env:    $INFRA_ENV_FILE
EOF
}

quick_install() {
  local admin_token store_backend pg_dsn runner_node_id runner_secret
  admin_token="$(rand_token)"
  store_backend="postgres"
  pg_dsn="postgres://proxmaster:proxmaster@postgres:5432/proxmaster?sslmode=disable"
  runner_node_id="node-1"
  runner_secret="$(rand_token)"

  write_env_file "$admin_token" "$store_backend" "$pg_dsn" "$runner_node_id" "$runner_secret"
  start_stack
  print_summary "$admin_token"
}

custom_install() {
  local admin_token store_backend pg_dsn runner_node_id runner_secret
  read -r -p "Admin Token (leer = automatisch generieren): " admin_token
  if [ -z "$admin_token" ]; then admin_token="$(rand_token)"; fi

  read -r -p "Store Backend [postgres/memory] (default: postgres): " store_backend
  if [ -z "$store_backend" ]; then store_backend="postgres"; fi

  read -r -p "Postgres DSN (default: postgres://proxmaster:proxmaster@postgres:5432/proxmaster?sslmode=disable): " pg_dsn
  if [ -z "$pg_dsn" ]; then pg_dsn="postgres://proxmaster:proxmaster@postgres:5432/proxmaster?sslmode=disable"; fi

  read -r -p "Runner Node ID (default: node-1): " runner_node_id
  if [ -z "$runner_node_id" ]; then runner_node_id="node-1"; fi

  read -r -p "Runner Shared Secret (leer = automatisch generieren): " runner_secret
  if [ -z "$runner_secret" ]; then runner_secret="$(rand_token)"; fi

  write_env_file "$admin_token" "$store_backend" "$pg_dsn" "$runner_node_id" "$runner_secret"
  start_stack
  print_summary "$admin_token"
}

update_only() {
  log "Fuehre Update durch (ohne neue Konfiguration) ..."
  if [ ! -f "$INFRA_ENV_FILE" ]; then
    err "Keine bestehende Env-Datei gefunden: $INFRA_ENV_FILE"
    exit 1
  fi
  start_stack
  log "Update abgeschlossen."
}

menu() {
  echo "Proxmaster Node1 Installer"
  echo "Installationspfad: $INSTALL_DIR"
  echo
  PS3="Bitte Option waehlen (1-4): "
  select opt in \
    "Quick Install (Empfohlen)" \
    "Custom Install" \
    "Update bestehender Stack" \
    "Abbrechen"; do
    case "$REPLY" in
      1) quick_install; break ;;
      2) custom_install; break ;;
      3) update_only; break ;;
      4) log "Abgebrochen."; exit 0 ;;
      *) warn "Ungueltige Auswahl." ;;
    esac
  done
}

main() {
  require_root
  detect_apt
  install_dependencies
  sync_repo
  menu
}

main "$@"
