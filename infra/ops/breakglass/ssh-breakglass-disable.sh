#!/usr/bin/env bash
set -euo pipefail

SSH_PORT="${1:-22}"
WG_SUBNET="${2:-10.13.13.0/24}"

if command -v ufw >/dev/null 2>&1; then
  ufw delete allow from "$WG_SUBNET" to any port "$SSH_PORT" proto tcp || true
fi

if command -v systemctl >/dev/null 2>&1; then
  systemctl stop ssh || true
fi

echo "break-glass ssh disabled for subnet $WG_SUBNET on port $SSH_PORT"

