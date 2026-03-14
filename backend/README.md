# Backend

Go control plane implementing:

- Auth stubs (`/auth/login`, `/auth/mfa/verify`, `/auth/reauth`)
- Cluster inventory endpoints (`/cluster/overview`, `/nodes`, `/vms`)
- Jobs, audit, incidents (`/jobs`, `/jobs/{id}`, `/audit`, `/incidents`)
- Policy API (`/policy/simulate`, `/policy/explain`)
- Control-plane endpoint (`/controlplane/endpoint`)
- MCP tool execution (`/mcp/call`) + approval path (`/mcp/approve`)
- Node runner heartbeat endpoint (`/nodes/heartbeat`)

## Run

```powershell
$env:PROXMASTER_ADMIN_TOKEN="dev-admin-token"
$env:PROXMASTER_STORE_BACKEND="memory" # or postgres
# $env:PROXMASTER_POSTGRES_DSN="postgres://proxmaster:proxmaster@localhost:5432/proxmaster?sslmode=disable"
$env:PROXMASTER_CONTROLPLANE_MODE="vip" # vip|dns
$env:PROXMASTER_CONTROLPLANE_VIP="100.100.100.10"
$env:PROXMASTER_CONTROLPLANE_DNS_NAME="proxmaster.internal"
$env:PROXMASTER_CONTROLPLANE_API_PORT="8080"
$env:PROXMASTER_NODE_ID="node-1"
go run ./cmd/api
```

## Supported MCP tools

- `cluster.get_state`
- `node.set_maintenance`
- `vm.migrate`
- `proxmaster.self_migrate`
- `vm.create`
- `vm.clone_from_template`
- `lxc.create`
- `storage.pool.apply`
- `storage.plan_apply`
- `network.apply`
- `network.plan_apply`
- `updates.plan`
- `updates.canary_start`
- `updates.rollout_continue`
- `updates.abort`
- `updates.rollout_pause`
- `updates.rollout_start`
- `updates.rollout_abort`
- `policy.simulate`
- `policy.explain`
- `node.runner.exec`

## Guarded Auto behavior

- `LOW` and `MEDIUM` risk: auto-executed
- `HIGH` risk: approval required (`reauth + hardware_mfa + second_approver`)
- Fail-closed health gates block writes if quorum/runner health is unsafe

### Seamless Proxmaster switch (self-migrate)

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

Danach aktiven Endpoint pruefen:

```http
GET /controlplane/endpoint
Authorization: Bearer <token>
```

## Tests

```powershell
go test ./...
```

## Runner agent (per node)

```powershell
go run ./cmd/runner-agent
```

Endpoints: `/healthz`, `/heartbeat`, `/exec` (HMAC-signed envelope).
