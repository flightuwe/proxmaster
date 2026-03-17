#!/usr/bin/env bash
set -euo pipefail

SSH_PORT="${1:-22}"
WG_SUBNET="${2:-10.13.13.0/24}"

if command -v systemctl >/dev/null 2>&1; then
  systemctl enable ssh || true
  systemctl start ssh || true
fi

if command -v ufw >/dev/null 2>&1; then
  ufw allow from "$WG_SUBNET" to any port "$SSH_PORT" proto tcp comment "proxmaster-breakglass" || true
fi

echo "break-glass ssh enabled for subnet $WG_SUBNET on port $SSH_PORT"

