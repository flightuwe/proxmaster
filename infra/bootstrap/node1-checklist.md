# Node 1 Bootstrap Checklist

1. Create management VM on `node-1` (Ubuntu 24.04 LTS suggested)
2. Attach VM network to management VLAN/bridge (`vmbr0` or dedicated mgmt bridge)
3. Install Docker and Docker Compose in VM
4. Start infra dependencies and control plane:
   - `docker compose -f /opt/proxmaster/infra/docker-compose.yml up -d`
5. Set secure env vars:
   - `PROXMASTER_ADMIN_TOKEN=<strong-random-token>`
   - `PROXMASTER_STORE_BACKEND=postgres`
   - `PROXMASTER_POSTGRES_DSN=<cluster-local-dsn>`
6. Install runner-agent binary on each node and enable systemd service (`runner-agent.service`)
7. Join WireGuard/Tailscale network
8. Restrict API access to overlay network only
9. Validate:
   - `/healthz`
   - `/cluster/overview`
   - hard-block path via `/mcp/call` + `/mcp/approve`
   - `/policy/simulate`
   - `/nodes/heartbeat`
10. Enable Proxmox HA for management VM once shared storage is confirmed
