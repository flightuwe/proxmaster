# Node 1 Bootstrap Checklist

1. Create management VM on `node-1` (Ubuntu 24.04 LTS suggested)
2. Attach VM network to management VLAN/bridge (`vmbr0` or dedicated mgmt bridge)
3. Install Docker and Docker Compose in VM
4. Start infra dependencies:
   - `docker compose -f /opt/proxmaster/infra/docker-compose.yml up -d`
5. Deploy backend service in VM and set env vars:
   - `PROXMASTER_LISTEN_ADDR=:8080`
   - `PROXMASTER_ADMIN_TOKEN=<strong-random-token>`
6. Join WireGuard/Tailscale network
7. Restrict API access to overlay network only
8. Validate:
   - `/healthz`
   - `/cluster/overview`
   - hard-block path via `/mcp/call` + `/mcp/approve`
9. Enable Proxmox HA for management VM once shared storage is confirmed