# Node 1 Bootstrap Checklist

1. Create management VM on `node-1` (Ubuntu 24.04 LTS suggested)
2. Attach VM network to management VLAN/bridge (`vmbr0` or dedicated mgmt bridge)
3. Run one command:
   - `curl -fsSL https://raw.githubusercontent.com/flightuwe/proxmaster/main/infra/bootstrap/install-node1.sh | sudo bash`
4. In installer menu choose:
   - `Quick Install (Empfohlen)` or `Custom Install`
5. Install runner-agent binary on each node and enable systemd service (`runner-agent.service`)
6. Join WireGuard/Tailscale network
7. Restrict API access to overlay network only
8. Validate:
   - `/healthz`
   - `/cluster/overview`
   - `/controlplane/endpoint`
   - `mcp tool: proxmox.connection.test`
   - hard-block path via `/mcp/call` + `/mcp/approve`
   - `/policy/simulate`
   - `/nodes/heartbeat`
9. Enable Proxmox HA for management VM once shared storage is confirmed
