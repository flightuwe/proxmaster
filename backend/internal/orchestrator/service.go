package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"proxmaster/backend/internal/breakglass"
	"proxmaster/backend/internal/connectivity"
	"proxmaster/backend/internal/gitops"
	"proxmaster/backend/internal/models"
	"proxmaster/backend/internal/proxmox"
	"proxmaster/backend/internal/runner"
	"proxmaster/backend/internal/vpn"
)

type Service struct {
	px           *proxmox.Client
	runner       *runner.Controller
	connectivity *connectivity.Service
	gitops       *gitops.Service
	breakglass   *breakglass.Service
	wireguard    *vpn.WireGuardService
}

func New(px *proxmox.Client, runnerCtrl *runner.Controller, conn *connectivity.Service, gitopsSvc *gitops.Service, breakglassSvc *breakglass.Service, wgSvc *vpn.WireGuardService) *Service {
	return &Service{
		px:           px,
		runner:       runnerCtrl,
		connectivity: conn,
		gitops:       gitopsSvc,
		breakglass:   breakglassSvc,
		wireguard:    wgSvc,
	}
}

func (s *Service) Execute(ctx context.Context, tool string, params map[string]any) (map[string]any, error) {
	switch tool {
	case "cluster.get_state":
		state := s.px.GetState(ctx)
		return map[string]any{"state": state}, nil
	case "connectivity.status":
		if s.connectivity == nil {
			return nil, errors.New("connectivity service not configured")
		}
		return s.connectivity.Status(ctx), nil
	case "vpn.wireguard.status":
		if s.wireguard == nil {
			return nil, errors.New("wireguard service not configured")
		}
		return s.wireguard.Status(ctx)
	case "vpn.wireguard.plan":
		if s.wireguard == nil {
			return nil, errors.New("wireguard service not configured")
		}
		return s.wireguard.Plan(ctx, params)
	case "vpn.wireguard.apply":
		if s.wireguard == nil {
			return nil, errors.New("wireguard service not configured")
		}
		return s.wireguard.Apply(ctx, params)
	case "proxmox.connection.test":
		return s.px.ConnectionTest(ctx)
	case "gitops.status":
		if s.gitops == nil {
			return nil, errors.New("gitops service not configured")
		}
		return s.gitops.Status(), nil
	case "gitops.sync.now":
		if s.gitops == nil {
			return nil, errors.New("gitops service not configured")
		}
		actor := stringFrom(params["actor"])
		return s.gitops.SyncNow(ctx, actor)
	case "gitops.rollback":
		if s.gitops == nil {
			return nil, errors.New("gitops service not configured")
		}
		return s.gitops.RollbackLastStable(ctx)
	case "ssh.breakglass.status":
		if s.breakglass == nil {
			return nil, errors.New("breakglass service not configured")
		}
		return s.breakglass.Status(ctx), nil
	case "ssh.breakglass.enable":
		if s.breakglass == nil {
			return nil, errors.New("breakglass service not configured")
		}
		actor := stringFrom(params["actor"])
		durationMin := intFrom(params["duration_minutes"], 60)
		if durationMin < 1 {
			durationMin = 1
		}
		return s.breakglass.Enable(ctx, time.Duration(durationMin)*time.Minute, actor)
	case "ssh.breakglass.disable":
		if s.breakglass == nil {
			return nil, errors.New("breakglass service not configured")
		}
		actor := stringFrom(params["actor"])
		return s.breakglass.Disable(ctx, actor)
	case "node.set_maintenance":
		nodeID, _ := params["node_id"].(string)
		maintenance, _ := params["maintenance"].(bool)
		if nodeID == "" {
			return nil, errors.New("missing node_id")
		}
		return s.px.SetNodeMaintenance(ctx, nodeID, maintenance)
	case "vm.migrate":
		vmID, _ := params["vm_id"].(string)
		targetNode, _ := params["target_node"].(string)
		if vmID == "" || targetNode == "" {
			return nil, errors.New("missing vm_id or target_node")
		}
		return s.px.MigrateVM(ctx, vmID, targetNode)
	case "proxmaster.self_migrate":
		vmID, _ := params["vm_id"].(string)
		targetNode, _ := params["target_node"].(string)
		restartAfter, _ := params["restart_after_migrate"].(bool)
		if targetNode == "" {
			return nil, errors.New("missing target_node")
		}
		return s.px.SelfMigrateProxmaster(ctx, vmID, targetNode, restartAfter)
	case "vm.create":
		name, _ := params["name"].(string)
		nodeID, _ := params["node_id"].(string)
		return s.px.CreateVM(ctx, name, nodeID, intFrom(params["cpu"], 2), intFrom(params["memory_mb"], 2048), intFrom(params["disk_gb"], 20))
	case "vm.clone_from_template":
		templateID, _ := params["template_id"].(string)
		newID, _ := params["new_id"].(string)
		targetNode, _ := params["target_node"].(string)
		name, _ := params["name"].(string)
		if templateID == "" || targetNode == "" {
			return nil, errors.New("missing template_id or target_node")
		}
		return s.px.CloneVM(ctx, templateID, newID, targetNode, name)
	case "lxc.create":
		name, _ := params["name"].(string)
		nodeID, _ := params["node_id"].(string)
		return s.px.CreateLXC(ctx, name, nodeID, intFrom(params["cpu"], 2), intFrom(params["memory_mb"], 1024), intFrom(params["disk_gb"], 8))
	case "storage.pool.apply", "storage.plan_apply":
		name, _ := params["name"].(string)
		poolType, _ := params["type"].(string)
		if name == "" || poolType == "" {
			return nil, errors.New("missing name or type")
		}
		if tool == "storage.plan_apply" {
			return s.px.PlanApplyStoragePool(ctx, name, poolType)
		}
		return s.px.ApplyStoragePool(ctx, name, poolType)
	case "storage.inventory.sync":
		return s.px.SyncStorageInventory(ctx)
	case "storage.pool.rebuild_all.plan":
		return s.px.PlanRebuildAllPools(ctx)
	case "storage.pool.rebuild_all.execute":
		planID, _ := params["plan_id"].(string)
		if planID == "" {
			return nil, errors.New("missing plan_id")
		}
		return s.px.ExecuteRebuildAllPools(ctx, planID)
	case "storage.replication.plan_apply":
		policyID, _ := params["id"].(string)
		name, _ := params["name"].(string)
		source, _ := params["source_pool"].(string)
		target, _ := params["target_pool"].(string)
		schedule, _ := params["schedule"].(string)
		compression, _ := params["compression"].(string)
		verifyAfter, _ := params["verify_after"].(bool)
		return s.px.ApplyReplicationPolicy(ctx, models.ReplicationPolicy{
			ID:          policyID,
			Name:        name,
			SourcePool:  source,
			TargetPool:  target,
			Schedule:    schedule,
			Compression: compression,
			VerifyAfter: verifyAfter,
			Status:      "active",
		})
	case "storage.health.explain":
		return s.px.SyncStorageInventory(ctx)
	case "network.apply", "network.plan_apply":
		name, _ := params["name"].(string)
		kind, _ := params["kind"].(string)
		cidr, _ := params["cidr"].(string)
		if name == "" || kind == "" || cidr == "" {
			return nil, errors.New("missing name, kind, or cidr")
		}
		return s.px.ApplyNetwork(ctx, name, kind, cidr)
	case "updates.plan":
		strategy, _ := params["strategy"].(string)
		return s.px.UpdatesPlan(ctx, strategy)
	case "updates.canary_start", "updates.rollout_start":
		nodeID, _ := params["node_id"].(string)
		return s.px.CanaryStart(ctx, nodeID)
	case "updates.rollout_continue", "updates.rollout_pause":
		return s.px.RolloutContinue(ctx)
	case "updates.abort", "updates.rollout_abort":
		reason, _ := params["reason"].(string)
		return s.px.RolloutAbort(ctx, reason)
	case "policy.simulate":
		targetTool, _ := params["tool"].(string)
		return s.px.SimulatePolicy(ctx, targetTool, "MEDIUM")
	case "policy.explain":
		targetTool, _ := params["tool"].(string)
		return s.px.ExplainPolicy(ctx, targetTool)
	case "node.runner.exec":
		nodeID, _ := params["node_id"].(string)
		command, _ := params["command"].(string)
		args, _ := params["args"].(map[string]any)
		if nodeID == "" || command == "" {
			return nil, errors.New("missing node_id or command")
		}
		return s.runner.Execute(ctx, nodeID, command, args)
	case "backup.policy.upsert":
		workloadID, _ := params["workload_id"].(string)
		workloadKind, _ := params["workload_kind"].(string)
		schedule, _ := params["schedule"].(string)
		targetID, _ := params["target_id"].(string)
		rpo, _ := params["rpo"].(string)
		retention, _ := params["retention"].(string)
		encryption, _ := params["encryption"].(bool)
		immutability, _ := params["immutability"].(bool)
		verifyRestore, _ := params["verify_restore"].(bool)
		override, _ := params["override"].(bool)
		return s.px.UpsertBackupPolicy(ctx, models.BackupPolicy{
			ID:            stringFrom(params["id"]),
			WorkloadID:    workloadID,
			WorkloadKind:  workloadKind,
			Priority:      intFrom(params["priority"], 100),
			Override:      override,
			Schedule:      schedule,
			TargetID:      targetID,
			RPO:           rpo,
			Retention:     retention,
			Encryption:    encryption,
			Immutability:  immutability,
			VerifyRestore: verifyRestore,
		})
	case "backup.policy.explain":
		workloadID, _ := params["workload_id"].(string)
		return s.px.ExplainBackupPolicy(ctx, workloadID)
	case "backup.run.now":
		workloadID, _ := params["workload_id"].(string)
		return s.px.RunBackupNow(ctx, workloadID)
	case "backup.restore.plan":
		workloadID, _ := params["workload_id"].(string)
		targetID, _ := params["target_id"].(string)
		return s.px.PlanRestore(ctx, workloadID, targetID)
	case "backup.restore.execute":
		planID, _ := params["plan_id"].(string)
		return s.px.ExecuteRestore(ctx, planID)
	case "backup.verify.sample":
		return s.px.VerifyBackupSample(ctx)
	case "backup.policy.list":
		return s.px.ListBackupPolicies(ctx)
	case "backup.target.list":
		return s.px.ListBackupTargets(ctx)
	default:
		return nil, fmt.Errorf("unknown tool: %s", tool)
	}
}

func intFrom(v any, fallback int) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	default:
		return fallback
	}
}

func stringFrom(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
