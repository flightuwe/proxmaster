# Backend

Go control plane implementing:

- Auth stubs (`/auth/login`, `/auth/mfa/verify`, `/auth/reauth`)
- Cluster inventory endpoints (`/cluster/overview`, `/nodes`, `/vms`)
- Jobs and audit (`/jobs`, `/jobs/{id}`, `/audit`)
- MCP tool execution (`/mcp/call`) + approval path (`/mcp/approve`)

## Run

```powershell
$env:PROXMASTER_ADMIN_TOKEN="dev-admin-token"
go run ./cmd/api
```

## Supported MCP tools

- `cluster.get_state`
- `node.set_maintenance`
- `vm.migrate`
- `storage.pool.apply`
- `network.apply`
- `updates.rollout_start`
- `updates.rollout_pause`
- `updates.rollout_abort`

## Guarded Auto behavior

- `LOW` and `MEDIUM` risk: auto-executed
- `HIGH` risk: hard-blocked unless explicitly approved (`ApproveNow=true` or `/mcp/approve` with reauth token)

## Tests

```powershell
go test ./...
```