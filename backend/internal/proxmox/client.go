package proxmox

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"proxmaster/backend/internal/controlplane"
	"proxmaster/backend/internal/models"
	"proxmaster/backend/internal/proxmoxapi"
	"proxmaster/backend/internal/store"
)

type Client struct {
	store        store.Store
	controlPlane *controlplane.Manager
	realAPI      *proxmoxapi.Client
}

func NewClient(s store.Store, cp *controlplane.Manager, realAPI *proxmoxapi.Client) *Client {
	return &Client{store: s, controlPlane: cp, realAPI: realAPI}
}

func (c *Client) GetState(ctx context.Context) models.ClusterState {
	if c.realAPI != nil && c.realAPI.Enabled() {
		state := c.store.ClusterState()
		res, err := c.realAPI.ClusterResources(ctx, "")
		if err == nil {
			nodes := make([]models.Node, 0)
			vms := make([]models.VM, 0)
			for _, r := range res {
				switch r.Type {
				case "node":
					nodes = append(nodes, models.Node{
						ID:            r.Node,
						Name:          r.Node,
						Status:        r.Status,
						Maintenance:   false,
						LastHeartbeat: time.Now().UTC(),
						RunnerHealthy: true,
					})
				case "qemu", "lxc":
					vmID := strconv.Itoa(r.VMID)
					name := r.Name
					if name == "" {
						name = vmID
					}
					vms = append(vms, models.VM{
						ID:       vmID,
						NodeID:   r.Node,
						Name:     name,
						Power:    r.Status,
						Priority: 80,
						CPU:      int(r.CPUs),
						MemoryMB: int(r.MaxMem / (1024 * 1024)),
						DiskGB:   int(r.MaxDisk / (1024 * 1024 * 1024)),
						Kind:     r.Type,
					})
				}
			}
			if len(nodes) > 0 {
				state.Nodes = nodes
			}
			if len(vms) > 0 {
				state.VMs = vms
			}
			storages, serr := c.realAPI.StorageList(ctx)
			if serr == nil && len(storages) > 0 {
				pools := make([]models.StoragePool, 0, len(storages))
				for _, s := range storages {
					status := "unknown"
					if s.Enabled == 1 {
						status = "healthy"
					}
					pools = append(pools, models.StoragePool{
						Name:       s.Storage,
						Type:       s.Type,
						Backend:    s.Type,
						Status:     status,
						CapacityGB: int(s.Total / (1024 * 1024 * 1024)),
						UsedGB:     int(s.Used / (1024 * 1024 * 1024)),
						Tier:       "balanced",
					})
				}
				state.Pools = pools
			}
			state.UpdatedAt = time.Now().UTC()
			return state
		}
	}
	return c.store.ClusterState()
}

func (c *Client) SetNodeMaintenance(_ context.Context, nodeID string, maintenance bool) (map[string]any, error) {
	ok := c.store.SetNodeMaintenance(nodeID, maintenance)
	if !ok {
		return nil, errors.New("node not found")
	}
	return map[string]any{"changed": true, "node_id": nodeID, "maintenance": maintenance}, nil
}

func (c *Client) MigrateVM(ctx context.Context, vmID, targetNode string) (map[string]any, error) {
	if c.realAPI != nil && c.realAPI.Enabled() {
		sourceNode := ""
		for _, vm := range c.GetState(ctx).VMs {
			if vm.ID == vmID {
				sourceNode = vm.NodeID
				break
			}
		}
		parsedID, ok := proxmoxapi.ParseVMID(vmID)
		if !ok {
			return nil, errors.New("invalid vm_id")
		}
		if sourceNode == "" {
			return nil, errors.New("source node for vm not found")
		}
		if err := c.realAPI.MigrateQemuVM(ctx, sourceNode, parsedID, targetNode, true); err != nil {
			return nil, err
		}
	}
	ok := c.store.MigrateVM(vmID, targetNode)
	if !ok {
		return map[string]any{"changed": true, "vm_id": vmID, "target_node": targetNode, "live_api_only": true}, nil
	}
	return map[string]any{"changed": true, "vm_id": vmID, "target_node": targetNode}, nil
}

func (c *Client) SelfMigrateProxmaster(ctx context.Context, vmID, targetNode string, restartAfter bool) (map[string]any, error) {
	if vmID == "" {
		vmID = "100"
	}
	if targetNode == "" {
		return nil, errors.New("missing target_node")
	}

	state := c.store.ClusterState()
	var vm *models.VM
	for i := range state.VMs {
		if state.VMs[i].ID == vmID {
			vm = &state.VMs[i]
			break
		}
	}
	if vm == nil {
		return nil, errors.New("proxmaster management vm not found")
	}
	if vm.NodeID == targetNode {
		return nil, errors.New("proxmaster already running on target node")
	}

	targetOnline := false
	for _, n := range state.Nodes {
		if n.ID == targetNode && n.Status == "online" {
			targetOnline = true
			break
		}
	}
	if !targetOnline {
		return nil, errors.New("target node is not online")
	}

	_, err := c.MigrateVM(ctx, vmID, targetNode)
	if err != nil {
		return nil, err
	}
	switchResult := c.controlPlane.SwitchTo(targetNode)

	return map[string]any{
		"changed":                true,
		"action":                 "proxmaster_self_migrate",
		"management_vm_id":       vmID,
		"from_node":              vm.NodeID,
		"to_node":                targetNode,
		"live_migration":         true,
		"restart_after_migrate":  restartAfter,
		"switch_mode":            "seamless_handover",
		"client_reconnect_hint":  "reconnect API client to active control-plane endpoint",
		"handover":               switchResult,
		"completed_at_utc":       time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (c *Client) ConnectionTest(ctx context.Context) (map[string]any, error) {
	if c.realAPI == nil || !c.realAPI.Enabled() {
		return map[string]any{
			"connected": false,
			"mode":      "mock",
			"message":   "real proxmox api disabled (set PROXMASTER_PROXMOX_USE_REAL_API=true)",
		}, nil
	}
	if err := c.realAPI.Ping(ctx); err != nil {
		return map[string]any{
			"connected": false,
			"mode":      "real",
			"error":     err.Error(),
		}, nil
	}
	return map[string]any{
		"connected": true,
		"mode":      "real",
		"message":   "proxmox api reachable and token accepted",
	}, nil
}

func (c *Client) CreateVM(_ context.Context, name, nodeID string, cpu, memoryMB, diskGB int) (map[string]any, error) {
	if name == "" || nodeID == "" {
		return nil, errors.New("missing name or node_id")
	}
	vm := c.store.CreateVM(models.VM{
		Name:     name,
		NodeID:   nodeID,
		CPU:      cpu,
		MemoryMB: memoryMB,
		DiskGB:   diskGB,
		Priority: 80,
		Kind:     "vm",
	})
	return map[string]any{"changed": true, "vm": vm}, nil
}

func (c *Client) CloneVM(_ context.Context, templateID, newID, targetNode, name string) (map[string]any, error) {
	vm, ok := c.store.CloneVM(templateID, newID, targetNode, name)
	if !ok {
		return nil, errors.New("template vm not found")
	}
	return map[string]any{"changed": true, "vm": vm}, nil
}

func (c *Client) CreateLXC(_ context.Context, name, nodeID string, cpu, memoryMB, diskGB int) (map[string]any, error) {
	if name == "" || nodeID == "" {
		return nil, errors.New("missing name or node_id")
	}
	ct := c.store.CreateLXC(models.VM{
		Name:     name,
		NodeID:   nodeID,
		CPU:      cpu,
		MemoryMB: memoryMB,
		DiskGB:   diskGB,
		Priority: 70,
		Kind:     "lxc",
	})
	return map[string]any{"changed": true, "lxc": ct}, nil
}

func (c *Client) ApplyStoragePool(_ context.Context, name, poolType string) (map[string]any, error) {
	c.store.ApplyPool(name, poolType)
	return map[string]any{"changed": true, "pool_name": name, "pool_type": poolType}, nil
}

func (c *Client) SyncStorageInventory(_ context.Context) (map[string]any, error) {
	out := c.store.SyncStorageInventory()
	out["changed"] = false
	return out, nil
}

func (c *Client) PlanApplyStoragePool(_ context.Context, name, poolType string) (map[string]any, error) {
	if name == "" || poolType == "" {
		return nil, errors.New("missing name or type")
	}
	return map[string]any{
		"changed":            false,
		"action":             "storage_pool_plan_apply",
		"name":               name,
		"type":               poolType,
		"dry_run_passed":     true,
		"impact_summary":     "pool metadata update only",
		"requires_approval":  true,
	}, nil
}

func (c *Client) PlanRebuildAllPools(_ context.Context) (map[string]any, error) {
	plan := c.store.PlanRebuildAllPools()
	return map[string]any{
		"changed": false,
		"plan":    plan,
		"phase":   "plan",
	}, nil
}

func (c *Client) ExecuteRebuildAllPools(_ context.Context, planID string) (map[string]any, error) {
	plan, ok := c.store.ExecuteRebuildAllPools(planID)
	if !ok {
		return nil, errors.New("rebuild plan not found")
	}
	return map[string]any{
		"changed":         true,
		"phase":           "execute",
		"plan":            plan,
		"execution_mode":  "canary_then_rolling",
		"post_verify":     "scheduled",
		"reconcile_state": "pending",
	}, nil
}

func (c *Client) ApplyReplicationPolicy(_ context.Context, policy models.ReplicationPolicy) (map[string]any, error) {
	updated := c.store.ApplyReplicationPolicy(policy)
	return map[string]any{"changed": true, "replication_policy": updated}, nil
}

func (c *Client) ApplyNetwork(_ context.Context, name, kind, cidr string) (map[string]any, error) {
	c.store.ApplyNetwork(name, kind, cidr)
	return map[string]any{"changed": true, "name": name, "kind": kind, "cidr": cidr}, nil
}

func (c *Client) UpdatesPlan(_ context.Context, strategy string) (map[string]any, error) {
	if strategy == "" {
		strategy = "canary_then_rolling"
	}
	return map[string]any{
		"changed":            false,
		"strategy":           strategy,
		"canary_node":        "node-1",
		"estimated_duration": "45m",
	}, nil
}

func (c *Client) CanaryStart(_ context.Context, nodeID string) (map[string]any, error) {
	if nodeID == "" {
		nodeID = "node-1"
	}
	return map[string]any{
		"changed":       true,
		"status":        "canary_started",
		"node_id":       nodeID,
		"started_at_utc": time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (c *Client) RolloutContinue(_ context.Context) (map[string]any, error) {
	return map[string]any{"changed": true, "status": "rolling_update_continued"}, nil
}

func (c *Client) RolloutAbort(_ context.Context, reason string) (map[string]any, error) {
	if reason == "" {
		reason = "manual abort"
	}
	return map[string]any{"changed": true, "status": "rolling_update_aborted", "reason": reason}, nil
}

func (c *Client) SimulatePolicy(_ context.Context, tool string, risk string) (map[string]any, error) {
	return map[string]any{
		"tool":               tool,
		"risk_level":         risk,
		"recommended_action": "require gate check and approval for high risk",
	}, nil
}

func (c *Client) ExplainPolicy(_ context.Context, tool string) (map[string]any, error) {
	return map[string]any{
		"tool":         tool,
		"policy_scope": "SRE-mode guarded autonomy",
		"details":      fmt.Sprintf("tool %s is evaluated by risk + hard blocks + fail-closed health gates", tool),
	}, nil
}

func (c *Client) UpsertBackupPolicy(_ context.Context, policy models.BackupPolicy) (map[string]any, error) {
	if policy.WorkloadID == "" {
		return nil, errors.New("missing workload_id")
	}
	if policy.WorkloadKind == "" {
		policy.WorkloadKind = "vm"
	}
	updated := c.store.UpsertBackupPolicy(policy)
	return map[string]any{
		"changed": true,
		"policy":  updated,
	}, nil
}

func (c *Client) ExplainBackupPolicy(_ context.Context, workloadID string) (map[string]any, error) {
	policy, logEntry, ok := c.store.ExplainBackupPolicy(workloadID)
	if !ok {
		return nil, errors.New("no policy for workload")
	}
	return map[string]any{
		"changed":      false,
		"policy":       policy,
		"decision_log": logEntry,
	}, nil
}

func (c *Client) RunBackupNow(_ context.Context, workloadID string) (map[string]any, error) {
	if workloadID == "" {
		return nil, errors.New("missing workload_id")
	}
	return c.store.RunBackupNow(workloadID), nil
}

func (c *Client) PlanRestore(_ context.Context, workloadID, targetID string) (map[string]any, error) {
	if workloadID == "" {
		return nil, errors.New("missing workload_id")
	}
	plan := c.store.PlanRestore(workloadID, targetID)
	return map[string]any{
		"changed": false,
		"plan":    plan,
	}, nil
}

func (c *Client) ExecuteRestore(_ context.Context, planID string) (map[string]any, error) {
	plan, ok := c.store.ExecuteRestore(planID)
	if !ok {
		return nil, errors.New("restore plan not found")
	}
	return map[string]any{
		"changed": true,
		"plan":    plan,
		"status":  "restore_started",
	}, nil
}

func (c *Client) VerifyBackupSample(_ context.Context) (map[string]any, error) {
	return c.store.VerifyBackupSample(), nil
}

func (c *Client) ListBackupPolicies(_ context.Context) (map[string]any, error) {
	return map[string]any{
		"changed":  false,
		"policies": c.store.ListBackupPolicies(),
	}, nil
}

func (c *Client) ListBackupTargets(_ context.Context) (map[string]any, error) {
	return map[string]any{
		"changed": false,
		"targets": c.store.ListBackupTargets(),
	}, nil
}
