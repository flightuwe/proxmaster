# PowerShell demo for Proxmaster backend

$base = "http://localhost:8080"
$token = "dev-admin-token"
$headers = @{ Authorization = "Bearer $token" }

Write-Host "1) Cluster state"
Invoke-RestMethod -Method Post -Uri "$base/mcp/call" -Headers $headers -ContentType "application/json" -Body (@{
    tool = "cluster.get_state"
    params = @{}
    actor = "demo"
} | ConvertTo-Json)

Write-Host "2) Hard-blocked network apply"
Invoke-RestMethod -Method Post -Uri "$base/mcp/call" -Headers $headers -ContentType "application/json" -Body (@{
    tool = "network.apply"
    params = @{ name = "vmbr1"; kind = "bridge"; cidr = "10.20.0.0/24" }
    actor = "demo"
    idempotency_key = "demo-net-001"
} | ConvertTo-Json)

Write-Host "3) Reauth token"
$reauth = Invoke-RestMethod -Method Post -Uri "$base/auth/reauth" -Headers $headers -ContentType "application/json" -Body "{}"

Write-Host "4) Approved network apply"
Invoke-RestMethod -Method Post -Uri "$base/mcp/approve" -Headers $headers -ContentType "application/json" -Body (@{
    tool = "network.apply"
    params = @{ name = "vmbr1"; kind = "bridge"; cidr = "10.20.0.0/24" }
    actor = "demo"
    reauth_token = $reauth.reauth_token
    hardware_mfa = $true
    second_approver = "ops-admin"
    idempotency_key = "demo-net-001"
} | ConvertTo-Json)

Write-Host "5) Create VM"
Invoke-RestMethod -Method Post -Uri "$base/mcp/call" -Headers $headers -ContentType "application/json" -Body (@{
    tool = "vm.create"
    params = @{ name = "app-vm-1"; node_id = "node-2"; cpu = 2; memory_mb = 2048; disk_gb = 30 }
    actor = "demo"
    idempotency_key = "demo-vm-001"
} | ConvertTo-Json)

Write-Host "6) Proxmaster self-migrate to node-2"
Invoke-RestMethod -Method Post -Uri "$base/mcp/approve" -Headers $headers -ContentType "application/json" -Body (@{
    tool = "proxmaster.self_migrate"
    params = @{ vm_id = "100"; target_node = "node-2"; restart_after_migrate = $true }
    actor = "demo"
    reauth_token = "reauth-ok"
    hardware_mfa = $true
    second_approver = "ops-admin"
    idempotency_key = "demo-selfmig-001"
} | ConvertTo-Json)

Write-Host "7) Active control-plane endpoint"
Invoke-RestMethod -Method Get -Uri "$base/controlplane/endpoint" -Headers $headers

Write-Host "8) Storage inventory sync"
Invoke-RestMethod -Method Post -Uri "$base/mcp/call" -Headers $headers -ContentType "application/json" -Body (@{
    tool = "storage.inventory.sync"
    params = @{}
    actor = "demo"
    idempotency_key = "demo-storage-sync-001"
} | ConvertTo-Json)

Write-Host "9) Rebuild all pools plan"
$rebuildPlanResp = Invoke-RestMethod -Method Post -Uri "$base/mcp/approve" -Headers $headers -ContentType "application/json" -Body (@{
    tool = "storage.pool.rebuild_all.plan"
    params = @{}
    actor = "demo"
    reauth_token = "reauth-ok"
    hardware_mfa = $true
    second_approver = "ops-admin"
    idempotency_key = "demo-rebuild-plan-001"
} | ConvertTo-Json)

Write-Host "10) Backup policy upsert for VM 101"
Invoke-RestMethod -Method Post -Uri "$base/mcp/approve" -Headers $headers -ContentType "application/json" -Body (@{
    tool = "backup.policy.upsert"
    params = @{
      workload_id = "101"
      workload_kind = "vm"
      priority = 200
      override = $true
      schedule = "0 2 * * *"
      target_id = "target-pbs-1"
      rpo = "24h"
      retention = "30d"
      encryption = $true
      immutability = $true
      verify_restore = $true
    }
    actor = "demo"
    reauth_token = "reauth-ok"
    hardware_mfa = $true
    second_approver = "ops-admin"
    idempotency_key = "demo-policy-upsert-001"
} | ConvertTo-Json -Depth 5)
