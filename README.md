# Proxmaster V1 Monorepo

Proxmaster is a bootstrap-ready Proxmox management stack:

- `backend/`: Go control plane (`API + MCP + risk/policy gates + audit/jobs`)
- `android/`: Kotlin Android app skeleton for remote administration
- `infra/`: Docker Compose and bootstrap docs for Node 1 management VM

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
  "reauth_token": "reauth-ok"
}
```

## Next hardening steps

- Replace in-memory store with PostgreSQL persistence
- Integrate real Proxmox API auth/session handling
- Wire Vault-backed secrets and short-lived credentials
- Add WireGuard/Tailscale sidecar and policy enforcement

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
