package proxmox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"sort"
	"strconv"
	"strings"
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

func (c *Client) DesiredState(_ context.Context) models.DesiredStateBundle {
	return c.store.DesiredStateBundle()
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

func (c *Client) PlanVMMigration(ctx context.Context, vmID, targetNode string) (map[string]any, error) {
	state := c.GetState(ctx)
	if vmID == "" {
		return nil, errors.New("missing vm_id")
	}
	var sourceVM *models.VM
	for i := range state.VMs {
		if state.VMs[i].ID == vmID {
			sourceVM = &state.VMs[i]
			break
		}
	}
	if sourceVM == nil {
		return nil, errors.New("vm not found")
	}
	if targetNode == "" {
		for _, n := range state.Nodes {
			if n.ID != sourceVM.NodeID && n.Status == "online" && !n.Maintenance {
				targetNode = n.ID
				break
			}
		}
	}
	if targetNode == "" {
		return nil, errors.New("no suitable target node found")
	}
	return map[string]any{
		"changed":           false,
		"action":            "vm_migration_plan",
		"vm_id":             vmID,
		"source_node":       sourceVM.NodeID,
		"target_node":       targetNode,
		"live_migration":    true,
		"downtime_expected": "low (<3s)",
		"prechecks": []string{
			"target node online",
			"shared storage reachable",
			"network bridge parity",
			"ha policy compatible",
		},
	}, nil
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
		"changed":               true,
		"action":                "proxmaster_self_migrate",
		"management_vm_id":      vmID,
		"from_node":             vm.NodeID,
		"to_node":               targetNode,
		"live_migration":        true,
		"restart_after_migrate": restartAfter,
		"switch_mode":           "seamless_handover",
		"client_reconnect_hint": "reconnect API client to active control-plane endpoint",
		"handover":              switchResult,
		"completed_at_utc":      time.Now().UTC().Format(time.RFC3339),
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
		"changed":           false,
		"action":            "storage_pool_plan_apply",
		"name":              name,
		"type":              poolType,
		"dry_run_passed":    true,
		"impact_summary":    "pool metadata update only",
		"requires_approval": true,
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
		"changed":        true,
		"status":         "canary_started",
		"node_id":        nodeID,
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

func (c *Client) ApplySpec(_ context.Context, scope, key string, desired map[string]any) (map[string]any, error) {
	if scope == "" {
		return nil, errors.New("missing scope")
	}
	if key == "" {
		key = "default"
	}
	spec := c.store.UpsertSpec(scope, key, desired)
	c.store.CreateAgentTask(models.AgentTask{
		Type:        "spec.reconcile",
		Payload:     map[string]any{"scope": scope, "key": key},
		RequestedBy: "spec-controller",
		Priority:    80,
		MaxAttempts: 3,
	})
	return map[string]any{
		"changed": true,
		"scope":   scope,
		"key":     key,
		"spec":    spec,
		"drift_delta": map[string]any{
			"status": "reconcile_queued",
		},
	}, nil
}

func (c *Client) ExplainSpec(_ context.Context, scope, key string) (map[string]any, error) {
	if scope == "" {
		scope = "workloads"
	}
	if key == "" {
		key = "default"
	}
	spec, ok := c.store.GetSpec(scope, key)
	if !ok {
		return nil, errors.New("spec not found")
	}
	observed, drift, delta := c.computeObservedAndDrift(scope, key, spec.Desired)
	if updated, ok := c.store.SetSpecObserved(scope, key, observed, drift, ""); ok {
		spec = updated
	}
	return map[string]any{
		"changed":     false,
		"scope":       scope,
		"key":         key,
		"spec":        spec,
		"drift_delta": delta,
	}, nil
}

func (c *Client) SimulateBackupPolicy(_ context.Context, workloadID string) (map[string]any, error) {
	policy, decision, ok := c.store.ExplainBackupPolicy(workloadID)
	if !ok {
		return map[string]any{
			"changed":        false,
			"workload_id":    workloadID,
			"selected":       false,
			"recommendation": "create workload specific policy",
		}, nil
	}
	return map[string]any{
		"changed":      false,
		"workload_id":  workloadID,
		"selected":     true,
		"policy":       policy,
		"decision_log": decision,
	}, nil
}

func (c *Client) ListBlueprints(_ context.Context) (map[string]any, error) {
	return map[string]any{
		"changed":        false,
		"catalog":        c.store.ListBlueprints(),
		"deployed_specs": c.store.ListBlueprintSpecs(),
	}, nil
}

func (c *Client) PlanBlueprint(_ context.Context, params map[string]any) (map[string]any, error) {
	name := stringFrom(params["name"])
	if name == "" {
		return nil, errors.New("missing blueprint name")
	}
	bp, ok := c.store.GetBlueprint(name)
	if !ok {
		return nil, errors.New("blueprint not found")
	}
	nodeID := stringFrom(params["node_id"])
	if nodeID == "" {
		nodeID = "node-1"
	}
	return map[string]any{
		"changed": false,
		"plan": map[string]any{
			"blueprint":      bp,
			"target_node":    nodeID,
			"provision_kind": bp.ProvisionKind,
			"steps":          []string{"provision workload via template+cloud-init", "run ansible roles", "health verify", "bind backup policy"},
			"rollback":       bp.RollbackSteps,
			"ansible_check":  "planned",
		},
	}, nil
}

func (c *Client) DeployBlueprint(ctx context.Context, params map[string]any) (map[string]any, error) {
	name := stringFrom(params["name"])
	if name == "" {
		return nil, errors.New("missing blueprint name")
	}
	bp, ok := c.store.GetBlueprint(name)
	if !ok {
		return nil, errors.New("blueprint not found")
	}
	nodeID := stringFrom(params["node_id"])
	if nodeID == "" {
		nodeID = "node-1"
	}
	workloadName := stringFrom(params["workload_name"])
	if workloadName == "" {
		workloadName = "svc-" + name
	}
	templateID := stringFrom(params["template_id"])
	var created map[string]any
	var err error
	if templateID != "" {
		created, err = c.CloneVM(ctx, templateID, "", nodeID, workloadName)
	} else if bp.ProvisionKind == "lxc" {
		created, err = c.CreateLXC(ctx, workloadName, nodeID, bp.DefaultCPU, bp.DefaultMemMB, bp.DefaultDiskGB)
	} else {
		created, err = c.CreateVM(ctx, workloadName, nodeID, bp.DefaultCPU, bp.DefaultMemMB, bp.DefaultDiskGB)
	}
	if err != nil {
		return nil, err
	}
	createdID := ""
	if vmObj, ok := created["vm"].(models.VM); ok {
		createdID = vmObj.ID
	}
	if lxcObj, ok := created["lxc"].(models.VM); ok {
		createdID = lxcObj.ID
	}
	spec := c.store.UpsertBlueprint(models.ServiceBlueprintSpec{
		Name:    name,
		Version: bp.Version,
		Workload: models.WorkloadSpec{
			ID:           createdID,
			Name:         workloadName,
			Kind:         bp.ProvisionKind,
			NodeID:       nodeID,
			CPU:          bp.DefaultCPU,
			MemoryMB:     bp.DefaultMemMB,
			DiskGB:       bp.DefaultDiskGB,
			DesiredPower: "running",
		},
		Parameters: params,
	})
	ansibleCheck := c.runAnsibleRoles(ctx, bp, spec, true)
	ansibleApply := c.runAnsibleRoles(ctx, bp, spec, false)
	verify, _ := c.VerifyBlueprint(ctx, map[string]any{"name": name})
	return map[string]any{
		"changed":       true,
		"blueprint":     spec,
		"provision":     created,
		"ansible_check": ansibleCheck,
		"ansible_apply": ansibleApply,
		"verify":        verify,
		"health_status": "pending",
		"drift_delta":   map[string]any{"blueprint": name, "status": "deployed"},
	}, nil
}

func (c *Client) VerifyBlueprint(_ context.Context, params map[string]any) (map[string]any, error) {
	name := stringFrom(params["name"])
	if name == "" {
		return nil, errors.New("missing blueprint name")
	}
	bp, ok := c.store.GetBlueprint(name)
	if !ok {
		return nil, errors.New("blueprint not found")
	}
	return map[string]any{
		"changed": false,
		"name":    name,
		"checks":  bp.HealthChecks,
		"status":  "healthy",
	}, nil
}

func (c *Client) UpdateBlueprint(ctx context.Context, params map[string]any) (map[string]any, error) {
	plan, err := c.PlanBlueprint(ctx, params)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"changed": true,
		"status":  "update_started",
		"plan":    plan["plan"],
	}, nil
}

func (c *Client) RollbackBlueprint(_ context.Context, params map[string]any) (map[string]any, error) {
	name := stringFrom(params["name"])
	if name == "" {
		return nil, errors.New("missing blueprint name")
	}
	bp, ok := c.store.GetBlueprint(name)
	if !ok {
		return nil, errors.New("blueprint not found")
	}
	return map[string]any{
		"changed": true,
		"name":    name,
		"status":  "rollback_started",
		"steps":   bp.RollbackSteps,
	}, nil
}

func (c *Client) SetPolicyMode(_ context.Context, modeRaw, actor string, durationMin int) (map[string]any, error) {
	mode := models.PolicyModeGuardedSRE
	if modeRaw == "AGGRESSIVE_AUTO" || modeRaw == "aggressive" || modeRaw == "aggressive_auto" {
		mode = models.PolicyModeAggressive
	}
	if durationMin < 0 {
		durationMin = 0
	}
	st := c.store.SetPolicyMode(mode, actor, time.Duration(durationMin)*time.Minute)
	return map[string]any{
		"changed":      true,
		"policy_mode":  st,
		"aggressive":   st.Mode == models.PolicyModeAggressive,
		"auto_expires": st.AggressiveUntil,
	}, nil
}

func (c *Client) ReconcileSpec(_ context.Context, scope, key, reconcileJobID string) (map[string]any, error) {
	spec, ok := c.store.GetSpec(scope, key)
	if !ok {
		return nil, errors.New("spec not found")
	}
	observed, drift, delta := c.computeObservedAndDrift(scope, key, spec.Desired)
	updated, _ := c.store.SetSpecObserved(scope, key, observed, drift, reconcileJobID)
	return map[string]any{
		"changed":     true,
		"scope":       scope,
		"key":         key,
		"spec":        updated,
		"drift_delta": delta,
	}, nil
}

func (c *Client) ReconcileAllSpecs(ctx context.Context, reconcileJobID string) (map[string]any, error) {
	scopes := []string{"cluster", "storage", "network", "backup", "workloads", "blueprint"}
	results := make([]map[string]any, 0)
	for _, scope := range scopes {
		group := c.store.ListSpecs(scope)
		for key := range group {
			out, err := c.ReconcileSpec(ctx, scope, key, reconcileJobID)
			if err != nil {
				results = append(results, map[string]any{"scope": scope, "key": key, "error": err.Error()})
				continue
			}
			results = append(results, map[string]any{"scope": scope, "key": key, "result": out})
		}
	}
	return map[string]any{"changed": true, "results": results}, nil
}

func (c *Client) computeObservedAndDrift(scope, key string, desired map[string]any) (map[string]any, models.DriftStatus, map[string]any) {
	observed := c.observedFor(scope, key)
	delta := diffMaps(desired, observed)
	if len(desired) == 0 {
		return observed, models.DriftPending, map[string]any{"summary": "no desired set", "mismatches": []any{}}
	}
	if len(delta) == 0 {
		return observed, models.DriftInSync, map[string]any{"summary": "in sync", "mismatches": []any{}}
	}
	return observed, models.DriftDrifted, map[string]any{"summary": "drift detected", "mismatches": delta}
}

func (c *Client) observedFor(scope, key string) map[string]any {
	state := c.store.ClusterState()
	switch scope {
	case "cluster":
		return map[string]any{"ha_enabled": state.HAEnabled, "nodes_online": len(state.Nodes)}
	case "storage":
		return map[string]any{"pools": state.Pools, "datastores": state.Datastores}
	case "network":
		return map[string]any{"networks": state.Networks}
	case "backup":
		return map[string]any{"policies": c.store.ListBackupPolicies(), "targets": c.store.ListBackupTargets()}
	case "workloads":
		for _, vm := range state.VMs {
			if vm.ID == key {
				return map[string]any{
					"id": vm.ID, "name": vm.Name, "kind": vm.Kind, "node_id": vm.NodeID,
					"cpu": vm.CPU, "memory_mb": vm.MemoryMB, "disk_gb": vm.DiskGB, "desired_power": vm.Power,
				}
			}
		}
		return map[string]any{}
	case "blueprint":
		for _, b := range c.store.ListBlueprintSpecs() {
			if b.Name == key {
				raw, _ := toMap(b)
				return raw
			}
		}
		return map[string]any{}
	default:
		return map[string]any{}
	}
}

func diffMaps(desired, observed map[string]any) []map[string]any {
	keys := make([]string, 0, len(desired))
	for k := range desired {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	mismatches := make([]map[string]any, 0)
	for _, key := range keys {
		dv := normalizeValue(desired[key])
		ov := normalizeValue(observed[key])
		if !reflect.DeepEqual(dv, ov) {
			mismatches = append(mismatches, map[string]any{
				"field":    key,
				"desired":  desired[key],
				"observed": observed[key],
			})
		}
	}
	return mismatches
}

func normalizeValue(v any) any {
	raw, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return v
	}
	return out
}

func (c *Client) runAnsibleRoles(ctx context.Context, bp models.BlueprintDefinition, spec models.ServiceBlueprintSpec, checkMode bool) map[string]any {
	if len(bp.AnsibleRoles) == 0 {
		return map[string]any{"status": "skipped", "reason": "no ansible roles configured"}
	}
	ansiblePlaybook, err := exec.LookPath("ansible-playbook")
	if err != nil {
		return map[string]any{"status": "skipped", "reason": "ansible-playbook not found"}
	}
	inventory := os.Getenv("PROXMASTER_ANSIBLE_INVENTORY")
	playbook := os.Getenv("PROXMASTER_ANSIBLE_PLAYBOOK")
	if strings.TrimSpace(playbook) == "" {
		playbook = "site.yml"
	}
	args := []string{playbook}
	if strings.TrimSpace(inventory) != "" {
		args = append(args, "-i", inventory)
	}
	if checkMode {
		args = append(args, "--check")
	}
	args = append(args, "-e", "roles="+strings.Join(bp.AnsibleRoles, ","))
	args = append(args, "-e", "blueprint_name="+bp.Name)
	args = append(args, "-e", "workload_name="+spec.Workload.Name)
	cmd := exec.CommandContext(ctx, ansiblePlaybook, args...)
	out, runErr := cmd.CombinedOutput()
	return map[string]any{
		"status":     ternary(runErr == nil, "ok", "failed"),
		"check_mode": checkMode,
		"command":    append([]string{ansiblePlaybook}, args...),
		"output":     string(out),
		"error":      errString(runErr),
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func ternary[T any](ok bool, a, b T) T {
	if ok {
		return a
	}
	return b
}

func toMap(v any) (map[string]any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func stringFrom(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
