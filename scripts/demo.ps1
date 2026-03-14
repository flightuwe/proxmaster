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
