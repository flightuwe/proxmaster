#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="${PROXMASTER_GITOPS_REPO_DIR:-/opt/proxmaster}"
BRANCH="${PROXMASTER_GITOPS_BRANCH:-main}"
COMPOSE_FILE="${PROXMASTER_GITOPS_COMPOSE_FILE:-/opt/proxmaster/infra/docker-compose.yml}"
ENV_FILE="${PROXMASTER_GITOPS_ENV_FILE:-/opt/proxmaster/infra/.env}"
HEALTH_URL="${PROXMASTER_GITOPS_HEALTH_URL:-http://127.0.0.1:8080/healthz}"
ROLLBACK_ON_FAIL="${PROXMASTER_GITOPS_ROLLBACK_ON_FAIL:-true}"

log() {
  echo "[gitops] $*"
}

run_compose() {
  docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" "$@"
}

health_check() {
  curl -fsS --max-time 5 "$HEALTH_URL" >/dev/null
}

main() {
  cd "$REPO_DIR"
  before_commit="$(git rev-parse HEAD)"
  log "before commit: $before_commit"

  git fetch origin "$BRANCH"
  git checkout "$BRANCH"
  git pull --ff-only origin "$BRANCH"
  after_commit="$(git rev-parse HEAD)"
  log "after commit: $after_commit"

  if ! run_compose up -d; then
    log "compose up failed"
    if [ "$ROLLBACK_ON_FAIL" = "true" ]; then
      log "rolling back to $before_commit"
      git checkout "$before_commit"
      run_compose up -d || true
    fi
    exit 1
  fi

  if ! health_check; then
    log "health check failed"
    if [ "$ROLLBACK_ON_FAIL" = "true" ]; then
      log "rolling back to $before_commit"
      git checkout "$before_commit"
      run_compose up -d || true
      health_check || true
    fi
    exit 1
  fi

  log "deploy successful"
}

main "$@"

