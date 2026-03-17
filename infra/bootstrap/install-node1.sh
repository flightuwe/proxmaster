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
  apt-get install -y ca-certificates curl gnupg lsb-release git jq wireguard wireguard-tools

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
  chmod +x "$INSTALL_DIR/infra/bootstrap/install-node1.sh" || true
  chmod +x "$INSTALL_DIR/infra/ops/gitops/proxmaster-deploy.sh" || true
  chmod +x "$INSTALL_DIR/infra/ops/breakglass/ssh-breakglass-enable.sh" || true
  chmod +x "$INSTALL_DIR/infra/ops/breakglass/ssh-breakglass-disable.sh" || true
  chmod +x "$INSTALL_DIR/infra/ops/cli/proxmaster" || true
  ln -sf "$INSTALL_DIR/infra/ops/cli/proxmaster" /usr/local/bin/proxmaster || true
  ln -sf "$INSTALL_DIR/infra/ops/cli/proxmaster" /usr/local/bin/pm || true
}

rand_token() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 24
    return
  fi
  cat /proc/sys/kernel/random/uuid | tr -d '-'
}

write_env_file() {
  local admin_token="$1"
  local store_backend="$2"
  local pg_dsn="$3"
  local runner_node_id="$4"
  local runner_secret="$5"
  local cp_mode="$6"
  local cp_vip="$7"
  local cp_dns="$8"
  local cp_port="$9"
  local pve_real_api="${10}"
  local pve_api_base="${11}"
  local pve_token_id="${12}"
  local pve_token_secret="${13}"
  local pve_insecure_tls="${14}"
  local wg_interface="${15}"
  local gitops_repo_dir="${16}"
  local gitops_branch="${17}"
  local gitops_compose_file="${18}"
  local gitops_env_file="${19}"
  local gitops_health_url="${20}"
  local gitops_rollback_on_fail="${21}"
  local breakglass_enable_cmd="${22}"
  local breakglass_disable_cmd="${23}"
  local breakglass_default_min="${24}"
  local wg_config_path="${25}"
  local wg_keys_dir="${26}"
  local wg_listen_port="${27}"

  cat >"$INFRA_ENV_FILE" <<EOF
PROXMASTER_LISTEN_ADDR=:8080
PROXMASTER_ADMIN_TOKEN=$admin_token
PROXMASTER_STORE_BACKEND=$store_backend
PROXMASTER_POSTGRES_DSN=$pg_dsn
PROXMASTER_FAIL_CLOSED=true
PROXMASTER_RUNNER_HEARTBEAT_MAX_SEC=120
PROXMASTER_CONTROLPLANE_MODE=$cp_mode
PROXMASTER_CONTROLPLANE_VIP=$cp_vip
PROXMASTER_CONTROLPLANE_DNS_NAME=$cp_dns
PROXMASTER_CONTROLPLANE_API_PORT=$cp_port
PROXMASTER_NODE_ID=$runner_node_id
PROXMASTER_PROXMOX_USE_REAL_API=$pve_real_api
PROXMASTER_PROXMOX_API_BASE_URL=$pve_api_base
PROXMASTER_PROXMOX_API_TOKEN_ID=$pve_token_id
PROXMASTER_PROXMOX_API_TOKEN_SECRET=$pve_token_secret
PROXMASTER_PROXMOX_INSECURE_TLS=$pve_insecure_tls
PROXMASTER_WIREGUARD_INTERFACE=$wg_interface
PROXMASTER_GITOPS_REPO_DIR=$gitops_repo_dir
PROXMASTER_GITOPS_BRANCH=$gitops_branch
PROXMASTER_GITOPS_COMPOSE_FILE=$gitops_compose_file
PROXMASTER_GITOPS_ENV_FILE=$gitops_env_file
PROXMASTER_GITOPS_HEALTH_URL=$gitops_health_url
PROXMASTER_GITOPS_ROLLBACK_ON_FAIL=$gitops_rollback_on_fail
PROXMASTER_BREAKGLASS_ENABLE_CMD=$breakglass_enable_cmd
PROXMASTER_BREAKGLASS_DISABLE_CMD=$breakglass_disable_cmd
PROXMASTER_BREAKGLASS_DEFAULT_MIN=$breakglass_default_min
PROXMASTER_WIREGUARD_CONFIG_PATH=$wg_config_path
PROXMASTER_WIREGUARD_KEYS_DIR=$wg_keys_dir
PROXMASTER_WIREGUARD_LISTEN_PORT=$wg_listen_port
PROXMASTER_AGENT_TOKEN=$(rand_token)
PROXMASTER_AGENT_POLL_SEC=5
RUNNER_LISTEN_ADDR=:9091
RUNNER_NODE_ID=$runner_node_id
RUNNER_SHARED_SECRET=$runner_secret
RUNNER_API_BASE_URL=http://proxmaster-api:8080
RUNNER_ADMIN_TOKEN=$admin_token
RUNNER_HEARTBEAT_NODES=node-1,node-2,node-3,node-4
RUNNER_HEARTBEAT_INTERVAL_SEC=30
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

Remote Ops:
- Connectivity: GET /connectivity/status
- GitOps:       GET /gitops/status, POST /gitops/sync, POST /gitops/rollback
- Break-Glass:  GET /access/breakglass, POST /access/breakglass/enable|disable
EOF
}

quick_install() {
  local admin_token store_backend pg_dsn runner_node_id runner_secret cp_mode cp_vip cp_dns cp_port pve_real_api pve_api_base pve_token_id pve_token_secret pve_insecure_tls wg_interface gitops_repo_dir gitops_branch gitops_compose_file gitops_env_file gitops_health_url gitops_rollback_on_fail breakglass_enable_cmd breakglass_disable_cmd breakglass_default_min wg_config_path wg_keys_dir wg_listen_port
  admin_token="$(rand_token)"
  store_backend="postgres"
  pg_dsn="postgres://proxmaster:proxmaster@postgres:5432/proxmaster?sslmode=disable"
  runner_node_id="node-1"
  runner_secret="$(rand_token)"
  cp_mode="vip"
  cp_vip="100.100.100.10"
  cp_dns="proxmaster.internal"
  cp_port="8080"
  pve_real_api="false"
  pve_api_base="https://proxmox-node1:8006/api2/json"
  pve_token_id=""
  pve_token_secret=""
  pve_insecure_tls="false"
  wg_interface="wg0"
  gitops_repo_dir="$INSTALL_DIR"
  gitops_branch="main"
  gitops_compose_file="$INSTALL_DIR/infra/docker-compose.yml"
  gitops_env_file="$INFRA_ENV_FILE"
  gitops_health_url="http://127.0.0.1:8080/healthz"
  gitops_rollback_on_fail="true"
  breakglass_enable_cmd="$INSTALL_DIR/infra/ops/breakglass/ssh-breakglass-enable.sh 22 10.13.13.0/24"
  breakglass_disable_cmd="$INSTALL_DIR/infra/ops/breakglass/ssh-breakglass-disable.sh 22 10.13.13.0/24"
  breakglass_default_min="60"
  wg_config_path="/etc/wireguard/wg0.conf"
  wg_keys_dir="/etc/proxmaster/wireguard"
  wg_listen_port="51820"

  write_env_file "$admin_token" "$store_backend" "$pg_dsn" "$runner_node_id" "$runner_secret" "$cp_mode" "$cp_vip" "$cp_dns" "$cp_port" "$pve_real_api" "$pve_api_base" "$pve_token_id" "$pve_token_secret" "$pve_insecure_tls" "$wg_interface" "$gitops_repo_dir" "$gitops_branch" "$gitops_compose_file" "$gitops_env_file" "$gitops_health_url" "$gitops_rollback_on_fail" "$breakglass_enable_cmd" "$breakglass_disable_cmd" "$breakglass_default_min" "$wg_config_path" "$wg_keys_dir" "$wg_listen_port"
  start_stack
  print_summary "$admin_token"
}

custom_install() {
  local admin_token store_backend pg_dsn runner_node_id runner_secret cp_mode cp_vip cp_dns cp_port pve_real_api pve_api_base pve_token_id pve_token_secret pve_insecure_tls wg_interface gitops_repo_dir gitops_branch gitops_compose_file gitops_env_file gitops_health_url gitops_rollback_on_fail breakglass_enable_cmd breakglass_disable_cmd breakglass_default_min wg_config_path wg_keys_dir wg_listen_port
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

  read -r -p "ControlPlane Modus [vip/dns] (default: vip): " cp_mode
  if [ -z "$cp_mode" ]; then cp_mode="vip"; fi

  read -r -p "ControlPlane VIP (default: 100.100.100.10): " cp_vip
  if [ -z "$cp_vip" ]; then cp_vip="100.100.100.10"; fi

  read -r -p "ControlPlane DNS Name (default: proxmaster.internal): " cp_dns
  if [ -z "$cp_dns" ]; then cp_dns="proxmaster.internal"; fi

  read -r -p "ControlPlane API Port (default: 8080): " cp_port
  if [ -z "$cp_port" ]; then cp_port="8080"; fi

  read -r -p "Proxmox Real API aktivieren? [true/false] (default: true): " pve_real_api
  if [ -z "$pve_real_api" ]; then pve_real_api="true"; fi

  read -r -p "Proxmox API Base URL (default: https://<node>:8006/api2/json): " pve_api_base
  if [ -z "$pve_api_base" ]; then pve_api_base="https://proxmox-node1:8006/api2/json"; fi

  read -r -p "Proxmox API Token ID (z. B. root@pam!proxmaster): " pve_token_id
  read -r -p "Proxmox API Token Secret: " pve_token_secret

  read -r -p "Proxmox Insecure TLS [true/false] (default: false): " pve_insecure_tls
  if [ -z "$pve_insecure_tls" ]; then pve_insecure_tls="false"; fi

  read -r -p "WireGuard Interface (default: wg0): " wg_interface
  if [ -z "$wg_interface" ]; then wg_interface="wg0"; fi

  read -r -p "GitOps Repo Pfad (default: $INSTALL_DIR): " gitops_repo_dir
  if [ -z "$gitops_repo_dir" ]; then gitops_repo_dir="$INSTALL_DIR"; fi

  read -r -p "GitOps Branch (default: main): " gitops_branch
  if [ -z "$gitops_branch" ]; then gitops_branch="main"; fi

  read -r -p "GitOps Compose File (default: $INSTALL_DIR/infra/docker-compose.yml): " gitops_compose_file
  if [ -z "$gitops_compose_file" ]; then gitops_compose_file="$INSTALL_DIR/infra/docker-compose.yml"; fi

  read -r -p "GitOps Env File (default: $INFRA_ENV_FILE): " gitops_env_file
  if [ -z "$gitops_env_file" ]; then gitops_env_file="$INFRA_ENV_FILE"; fi

  read -r -p "GitOps Health URL (default: http://127.0.0.1:8080/healthz): " gitops_health_url
  if [ -z "$gitops_health_url" ]; then gitops_health_url="http://127.0.0.1:8080/healthz"; fi

  read -r -p "GitOps Rollback bei Fehler [true/false] (default: true): " gitops_rollback_on_fail
  if [ -z "$gitops_rollback_on_fail" ]; then gitops_rollback_on_fail="true"; fi

  read -r -p "Break-Glass Enable Hook Command (default: $INSTALL_DIR/infra/ops/breakglass/ssh-breakglass-enable.sh 22 10.13.13.0/24): " breakglass_enable_cmd
  if [ -z "$breakglass_enable_cmd" ]; then breakglass_enable_cmd="$INSTALL_DIR/infra/ops/breakglass/ssh-breakglass-enable.sh 22 10.13.13.0/24"; fi
  read -r -p "Break-Glass Disable Hook Command (default: $INSTALL_DIR/infra/ops/breakglass/ssh-breakglass-disable.sh 22 10.13.13.0/24): " breakglass_disable_cmd
  if [ -z "$breakglass_disable_cmd" ]; then breakglass_disable_cmd="$INSTALL_DIR/infra/ops/breakglass/ssh-breakglass-disable.sh 22 10.13.13.0/24"; fi
  read -r -p "Break-Glass Default Minuten (default: 60): " breakglass_default_min
  if [ -z "$breakglass_default_min" ]; then breakglass_default_min="60"; fi
  read -r -p "WireGuard Config Path (default: /etc/wireguard/wg0.conf): " wg_config_path
  if [ -z "$wg_config_path" ]; then wg_config_path="/etc/wireguard/wg0.conf"; fi
  read -r -p "WireGuard Keys Dir (default: /etc/proxmaster/wireguard): " wg_keys_dir
  if [ -z "$wg_keys_dir" ]; then wg_keys_dir="/etc/proxmaster/wireguard"; fi
  read -r -p "WireGuard Listen Port (default: 51820): " wg_listen_port
  if [ -z "$wg_listen_port" ]; then wg_listen_port="51820"; fi

  write_env_file "$admin_token" "$store_backend" "$pg_dsn" "$runner_node_id" "$runner_secret" "$cp_mode" "$cp_vip" "$cp_dns" "$cp_port" "$pve_real_api" "$pve_api_base" "$pve_token_id" "$pve_token_secret" "$pve_insecure_tls" "$wg_interface" "$gitops_repo_dir" "$gitops_branch" "$gitops_compose_file" "$gitops_env_file" "$gitops_health_url" "$gitops_rollback_on_fail" "$breakglass_enable_cmd" "$breakglass_disable_cmd" "$breakglass_default_min" "$wg_config_path" "$wg_keys_dir" "$wg_listen_port"
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
  local mode=""
  local non_interactive="false"
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --quick) mode="quick" ;;
      --custom) mode="custom" ;;
      --update) mode="update" ;;
      --non-interactive) non_interactive="true" ;;
      *)
        err "Unbekannte Option: $1"
        exit 1
        ;;
    esac
    shift
  done

  require_root
  detect_apt
  install_dependencies
  sync_repo
  if [ "$mode" = "quick" ]; then
    quick_install
    return
  fi
  if [ "$mode" = "update" ]; then
    update_only
    return
  fi
  if [ "$mode" = "custom" ] && [ "$non_interactive" = "true" ]; then
    err "--custom kann nicht non-interactive ausgefuehrt werden."
    exit 1
  fi
  if [ "$mode" = "custom" ]; then
    custom_install
    return
  fi
  if [ "$non_interactive" = "true" ]; then
    quick_install
    return
  fi
  menu
}

main "$@"
