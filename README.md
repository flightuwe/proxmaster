# Proxmaster V1 Monorepo

Proxmaster is a bootstrap-ready Proxmox management stack:

- `backend/`: Go control plane (`API + MCP + policy/risk + fail-closed gates + durable event log`)
- `android/`: Kotlin Android app skeleton for remote administration
- `infra/`: Docker Compose and bootstrap docs for Node 1 management VM and runner agent

## One-command Node1 installation

Auf der Node1-Management-VM (Ubuntu, als `root`/`sudo`) nur diesen Befehl ausführen:

```bash
curl -fsSL https://raw.githubusercontent.com/flightuwe/proxmaster/main/infra/bootstrap/install-node1.sh | sudo bash
```

Danach führt dich ein Menü:
- `Quick Install (Empfohlen)`
- `Custom Install`
- `Update bestehender Stack`

`Custom Install` fragt auch den Handover-Typ:
- `vip` (feste Virtual IP)
- `dns` (stabiler DNS-Name mit niedriger TTL)

`Custom Install` kann jetzt auch direkt die echte Proxmox API anbinden
(`PROXMASTER_PROXMOX_*` Variablen).

Neu fuer Remote-Cluster-Betrieb:
- WireGuard Interface Status in API (`/connectivity/status`)
- Native WireGuard control (`/vpn/wireguard/status`, `/vpn/wireguard/plan`, `/vpn/wireguard/apply`)
- GitOps Pull Deploy (`/gitops/status`, `/gitops/sync`, `/gitops/rollback`)
- SSH Break-Glass Toggle mit Timebox (`/access/breakglass/*`)
- Ops-Runbooks/Skripte unter `infra/ops/*`
- CLI Shortcuts: `proxmaster update`, `proxmaster proof`, `proxmaster status`
- Superkurz: `pm up`, `pm pf`, `pm st`, `pm lg`, `pm rs`, `pm tk`
- WireGuard CLI: `pm wg-status`, `pm wg-plan`, `pm wg-apply <client_pubkey> <endpoint>`
- Declarative control CLI:
  - `pm spec-put <scope> [key] '<json>'`
  - `pm state [all|cluster|storage|network|backup|workloads]`
  - `pm blueprint <list|plan|deploy|verify|update|rollback> '<json>'`
  - `pm mode <get|guarded|aggressive [minutes]>`
  - `pm wg-vm-deploy [node_id] [name] [template_id]`
  - `pm term [node_id] '<command>'` (runner-executed, audited)
- `pm up` auto-heals local git drift (backs up diff to `/tmp/*.patch`, then syncs hard to `origin/main`)
- `pm up` repariert bei Bedarf auch Repo-/Docker-Rechte automatisch via `sudo`
- `pm up` installiert fehlende WireGuard-Tools automatisch (`wireguard`, `wireguard-tools`)
- `pm up` macht immer `fetch --all --prune`, `reset --hard origin/main`, `clean -fd` und refreshed die Wrapper (`/usr/local/bin/pm`, `proxmaster`)
- `pm up` wartet auf apt-locks, retryt die WireGuard-Installation und bricht klar ab, falls `wg` am Ende fehlt
- One-command bootstrap: `pm bootstrap quick` (non-interactive)
- Network helper: `pm ip` (zeigt lokale IPv4 + direkte Test-URLs)

## Quick start (backend)

```powershell
cd backend
go test ./...
go run ./cmd/api
```

Default admin bearer token is `dev-admin-token`.

## Minimal flow

1. Get token:
```http
POST /auth/mfa/verify
```
2. Call MCP tool:
```http
POST /mcp/call
Authorization: Bearer dev-admin-token
{
  "tool": "cluster.get_state",
  "params": {},
  "actor": "android-admin"
}
```
3. High risk tool gets blocked without approval:
```http
POST /mcp/call
{
  "tool": "network.apply",
  "params": {"name":"vmbr1","kind":"bridge","cidr":"10.20.0.0/24"}
}
```
4. Approve with re-auth token:
```http
POST /auth/reauth
POST /mcp/approve
{
  "tool": "network.apply",
  "params": {"name":"vmbr1","kind":"bridge","cidr":"10.20.0.0/24"},
  "reauth_token": "reauth-ok",
  "hardware_mfa": true,
  "second_approver": "oncall-admin"
}
```

5. Proxmaster nahtlos auf andere Node verschieben:
```http
POST /mcp/approve
Authorization: Bearer <token>
{
  "tool": "proxmaster.self_migrate",
  "params": {
    "vm_id": "100",
    "target_node": "node-2",
    "restart_after_migrate": true
  },
  "reauth_token": "reauth-ok",
  "hardware_mfa": true,
  "second_approver": "oncall-admin"
}
```

## SRE-mode capabilities implemented

- Job FSM lifecycle (`planned -> approved -> running -> verified -> completed|failed|blocked|aborted|rolled_back`)
- Idempotency key support for mutating calls
- Fail-closed health gates (quorum + runner heartbeat checks)
- Guarded high-risk operations with dual approval metadata
- PostgreSQL-backed durable logs (jobs, audits, incidents) via `PROXMASTER_STORE_BACKEND=postgres`
- Node runner-agent binary for allowlisted host operations (`backend/cmd/runner-agent`)
- Storage control plane for mixed pools (`ZFS + Ceph + NFS/SMB`)
- Rebuild-all-pools planning/execution hooks with guarded approvals
- Per-workload backup policies and restore plan flows for VM/LXC
- Declarative spec APIs (`/spec/*`) + observed state APIs (`/state/*`)
- Blueprint catalog + deploy/verify/update/rollback flows (`/blueprints/*`)
- Policy mode switching (`/policy/mode`) with time-boxed aggressive auto mode
- Autonomy task queue with retries/priority/dead-letter + timeline (`/autonomy/tasks`, `/jobs/timeline`)
- Minimal integrated browser UI at `/webui` for state/spec/blueprint/policy actions
- Blueprint catalog includes `pfsense-gateway` bootstrap flow for test deployments
- Blueprint catalog includes `wireguard-server` and MCP tool `service.wireguard.vm.deploy`

## Remaining production hardening

- Replace stub auth with real OIDC + hardware MFA attestation
- Integrate real Proxmox session/token handling and retries per API category
- Add Vault transit/signing and credential rotation controller
- Add Prometheus/tracing exporters and alert rules
- Add full Android approval UX (second approver evidence flow)

## GitHub Project automation

This repository includes GitHub Actions that keep the project board in sync:

- New issue -> add to project + set status to `Todo`
- PR opened/reopened/synchronize -> linked issues set to `In Progress`
- PR merged -> linked issues set to `Done`

### One-time setup

1. Create a personal access token (classic) with scopes: `repo`, `project`.
2. Add it as repository secret: `GH_PROJECT_TOKEN`.
3. Confirm project settings in workflows:
   - `PROJECT_OWNER=flightuwe`
   - `PROJECT_NUMBER=1`
