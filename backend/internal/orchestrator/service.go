package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"proxmaster/backend/internal/proxmox"
	"proxmaster/backend/internal/runner"
)

type Service struct {
	px     *proxmox.Client
	runner *runner.Controller
}

func New(px *proxmox.Client, runnerCtrl *runner.Controller) *Service {
	return &Service{px: px, runner: runnerCtrl}
}

func (s *Service) Execute(ctx context.Context, tool string, params map[string]any) (map[string]any, error) {
	switch tool {
	case "cluster.get_state":
		state := s.px.GetState(ctx)
		return map[string]any{"state": state}, nil
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
		return s.px.ApplyStoragePool(ctx, name, poolType)
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
